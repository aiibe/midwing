package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"midwing/internal/midwing"
)

type unreadableVault struct{}

func (unreadableVault) Set(string, string) error { panic("discovery must not access credentials") }

func (unreadableVault) Get(string) (string, error) { panic("discovery must not access credentials") }

func (unreadableVault) Delete(string) error { panic("discovery must not access credentials") }

func TestDiscoveryGrantedPermissions(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
	definition := midwing.CustomService{ID: "sample-api", Name: "Sample API", BaseURL: "https://api.example.test", AuthKind: "token", Header: "Authorization", Prefix: "Bearer ", Endpoints: []midwing.CustomEndpoint{{Path: "/profile", QueryParameters: []string{"page"}}, {Path: "/items"}}}
	if err := store.CreateCustomService(definition); err != nil {
		t.Fatal(err)
	}
	if err := saveTestService(store, "sample-api", "private-token", []string{"endpoint-1"}); err != nil {
		t.Fatal(err)
	}
	store.Vault = unreadableVault{}
	var out bytes.Buffer
	if err := runCLIWithInput([]string{"services", "sample-api"}, store, nil, &out); err != nil {
		t.Fatal(err)
	}
	var view serviceDiscovery
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Connected || len(view.Endpoints) != 2 || !view.Endpoints[0].Enabled || view.Endpoints[1].Enabled || len(view.Endpoints[0].QueryParameters) != 1 {
		t.Fatalf("incorrect grants: %s", out.String())
	}
	for _, secret := range []string{"private-token", "credentialRef", "Authorization", "definition"} {
		if strings.Contains(out.String(), secret) {
			t.Fatalf("discovery includes %s", secret)
		}
	}
	out.Reset()
	if err := runCLIWithInput([]string{"services"}, store, nil, &out); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil || len(rows) != 1 || len(rows[0]) != 2 || rows[0]["connected"] != true {
		t.Fatalf("list is not compact: %s", out.String())
	}
}

func TestDiscoveryPOSTOnlyGrant(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
	definition := midwing.CustomService{ID: "write-api", Name: "Write API", BaseURL: "https://api.example.test", AuthKind: "token", Header: "Authorization", Endpoints: []midwing.CustomEndpoint{{Path: "/items", Methods: []string{"POST"}}}}
	if err := store.CreateCustomService(definition); err != nil {
		t.Fatal(err)
	}
	if err := saveTestService(store, definition.ID, "private-token", []string{"endpoint-1-post"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runCLIWithInput([]string{"services", definition.ID}, store, nil, &out); err != nil {
		t.Fatal(err)
	}
	var view serviceDiscovery
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Endpoints) != 1 || !view.Endpoints[0].Enabled || len(view.Endpoints[0].Requests) != 1 || view.Endpoints[0].Requests[0] != (requestView{Method: "POST", Enabled: true}) {
		t.Fatalf("incorrect POST grant: %s", out.String())
	}
}

func TestSchemaAndHelp(t *testing.T) {
	var schema struct {
		Examples []json.RawMessage `json:"examples"`
	}
	data, err := json.Marshal(serviceSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &schema); err != nil || len(schema.Examples) != 3 {
		t.Fatal("missing schema examples")
	}
	for _, input := range schema.Examples {
		store := &midwing.Store{Dir: filepath.Join(t.TempDir(), "not-created")}
		var out bytes.Buffer
		if err := runCLIWithInput([]string{"services", "validate", "-"}, store, bytes.NewReader(input), &out); err != nil {
			t.Fatal(err)
		}
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil || len(result) != 2 || result["valid"] != true {
			t.Fatalf("validation not compact: %s", out.String())
		}
		if _, err := os.Stat(store.Dir); !os.IsNotExist(err) {
			t.Fatal("validation persisted settings")
		}
	}
	for _, args := range [][]string{{"--help"}, {"get", "--help"}, {"post", "--help"}, {"services", "--help"}, {"services", "schema", "--help"}, {"services", "create", "--help"}, {"services", "validate", "--help"}, {"services", "example", "--help"}} {
		var out bytes.Buffer
		if err := runCLIWithInput(args, nil, nil, &out); err != nil || out.Len() == 0 {
			t.Fatalf("help failed: %v: %v", args, err)
		}
	}
	for _, id := range []string{"schema", "create", "validate"} {
		var definition midwing.CustomService
		json.Unmarshal([]byte(serviceJSON), &definition)
		definition.ID = id
		if _, err := midwing.ValidateCustomService(definition); err == nil {
			t.Fatalf("reserved ID accepted: %s", id)
		}
	}
}
