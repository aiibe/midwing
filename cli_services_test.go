package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"midwing/internal/midwing"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const serviceJSON = `{"id":"example","name":"Example","baseUrl":"https://api.example.com/v1","authKind":"token","header":"Authorization","prefix":"Bearer ","endpoints":[{"path":"/items/{id}","queryParameters":["limit"]}]}`

type cleanupFailureVault struct{ cliVault }

func (cleanupFailureVault) Delete(string) error { return errors.New("vault unavailable") }

func TestServicesDelete(t *testing.T) {
	for _, tc := range []struct {
		name           string
		connected      bool
		cleanupFailure bool
	}{
		{name: "disconnected"},
		{name: "connected", connected: true},
		{name: "cleanup pending", connected: true, cleanupFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
			var definition midwing.CustomService
			if err := json.Unmarshal([]byte(serviceJSON), &definition); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"example", "other"} {
				definition.ID = id
				if err := store.CreateCustomService(definition); err != nil {
					t.Fatal(err)
				}
				if tc.connected {
					if err := store.SaveService(id, id+"-token", []string{"endpoint-1"}); err != nil {
						t.Fatal(err)
					}
				}
			}
			vault := store.Vault.(cliVault)
			if tc.cleanupFailure {
				store.Vault = cleanupFailureVault{vault}
			}
			var out bytes.Buffer
			if err := runCLIWithInput([]string{"services", "delete", "example"}, store, nil, &out); err != nil {
				t.Fatal(err)
			}
			var result struct {
				Deleted bool   `json:"deleted"`
				ID      string `json:"id"`
				Warning string `json:"warning"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil || !result.Deleted || result.ID != "example" || (result.Warning != "") != tc.cleanupFailure {
				t.Fatalf("unexpected deletion result: %s (%v)", out.String(), err)
			}
			state, err := (&midwing.Store{Dir: store.Dir}).RegistrySnapshot()
			if err != nil || len(state.CustomServices) != 1 || state.CustomServices[0].ID != "other" {
				t.Fatalf("deletion did not preserve other service: %+v (%v)", state, err)
			}
			wantConnections, wantCredentials := 0, 0
			if tc.connected {
				wantConnections, wantCredentials = 1, 1
				if tc.cleanupFailure {
					wantCredentials++
				}
			}
			if len(state.Connections) != wantConnections || len(vault) != wantCredentials {
				t.Fatalf("unexpected connections or credentials: %d, %d", len(state.Connections), len(vault))
			}
			if tc.connected {
				conn := state.Connections[0]
				if conn.Service != "other" || vault[conn.CredentialRef] != "other-token" {
					t.Fatal("deletion changed the other service's connection or credential")
				}
			}
			out.Reset()
			if err := runCLIWithInput([]string{"services", "delete", "example"}, store, nil, &out); err == nil || out.Len() != 0 {
				t.Fatal("missing service deletion succeeded or wrote output")
			}
		})
	}
}

func TestServicesDeleteArguments(t *testing.T) {
	for _, args := range [][]string{{"delete"}, {"delete", ""}, {"delete", "--unknown"}, {"delete", "example", "extra"}} {
		var out bytes.Buffer
		if err := runServicesCLI(args, nil, nil, &out); err == nil || !strings.Contains(err.Error(), "usage:") || out.Len() != 0 {
			t.Fatalf("unexpected argument handling for %v: %v, %s", args, err, out.String())
		}
	}
	var out bytes.Buffer
	if err := runServicesCLI([]string{"delete", "--help"}, nil, nil, &out); err != nil || !strings.Contains(out.String(), "midwing services delete ID") {
		t.Fatalf("delete help failed: %v, %s", err, out.String())
	}
}

func TestServicesDeleteExistingCommandName(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir()}
	input := strings.Replace(serviceJSON, `"id":"example"`, `"id":"delete"`, 1)
	var out bytes.Buffer
	if err := runServicesCLI([]string{"create", "-"}, store, strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runServicesCLI([]string{"delete", "--config"}, store, nil, &out); err != nil || !strings.Contains(out.String(), `"id":"delete"`) {
		t.Fatalf("existing delete service cannot be inspected: %v, %s", err, out.String())
	}
	out.Reset()
	if err := runServicesCLI([]string{"delete", "delete"}, store, nil, &out); err != nil || !strings.Contains(out.String(), `"deleted":true`) {
		t.Fatalf("existing delete service cannot be deleted: %v, %s", err, out.String())
	}
}

func TestServicesEndpointDescription(t *testing.T) {
	input := strings.Replace(serviceJSON, `"path":"/items/{id}"`, `"path":"/items/{id}","description":"Read item details"`, 1)
	store := &midwing.Store{Dir: t.TempDir()}
	var out bytes.Buffer
	for _, command := range []string{"validate", "create"} {
		if err := runServicesCLI([]string{command, "-"}, store, strings.NewReader(input), &out); err != nil {
			t.Fatalf("%s rejected endpoint description: %v", command, err)
		}
	}
	out.Reset()
	if err := runServicesCLI([]string{"example", "--config"}, store, nil, &out); err != nil {
		t.Fatal(err)
	}
	var config serviceConfig
	if err := json.Unmarshal(out.Bytes(), &config); err != nil {
		t.Fatal(err)
	}
	if config.Definition.Endpoints[0].Description != "Read item details" || config.Permissions[0].Description != "Read item details" {
		t.Fatalf("description was not preserved: %s", out.String())
	}
	updated := strings.Replace(input, "Read item details", "Read updated details", 1)
	if err := runServicesCLI([]string{"update", "-"}, store, strings.NewReader(updated), &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runServicesCLI([]string{"example", "--config"}, store, nil, &out); err != nil || !strings.Contains(out.String(), "Read updated details") {
		t.Fatalf("update was not saved: %v %s", err, out.String())
	}
	for _, invalid := range []string{
		strings.Replace(updated, `"id":"example"`, `"id":"missing"`, 1),
		strings.Replace(updated, "/items/{id}", "/other/{id}", 1),
		strings.Replace(updated, "api.example.com", "other.example.com", 1),
	} {
		if err := runServicesCLI([]string{"update", "-"}, store, strings.NewReader(invalid), &out); err == nil {
			t.Fatal("accepted missing service or non-description update")
		}
	}
	for _, value := range []string{`null`, `42`, `"` + strings.Repeat("x", 501) + `"`} {
		invalid := strings.Replace(input, `"Read item details"`, value, 1)
		if err := runServicesCLI([]string{"validate", "-"}, store, strings.NewReader(invalid), &out); err == nil {
			t.Fatalf("invalid description accepted: %s", value)
		}
	}
}

func TestServicesWorkflow(t *testing.T) {
	store := &midwing.Store{Dir: filepath.Join(t.TempDir(), "settings")}
	var out bytes.Buffer
	run := func(args []string, input string) error {
		out.Reset()
		return runCLIWithInput(args, store, strings.NewReader(input), &out)
	}
	if err := run([]string{"services", "validate", "-"}, serviceJSON); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.Dir); !os.IsNotExist(err) {
		t.Fatal("validation wrote settings")
	}
	file := filepath.Join(t.TempDir(), "service.json")
	if err := os.WriteFile(file, []byte(serviceJSON), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"services", "create", file}, ""); err != nil {
		t.Fatal(err)
	}
	var created serviceSummary
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Connected || created.ID != "example" || strings.Contains(out.String(), "definition") {
		t.Fatalf("invalid creation result: %s", out.String())
	}
	// A fresh store sees the definition without an account or vault access.
	store = &midwing.Store{Dir: store.Dir}
	if err := run([]string{"services", "example", "--config"}, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"path":"/items/{id}"`) {
		t.Fatal("missing definition")
	}
	if err := run([]string{"services"}, ""); err != nil {
		t.Fatal(err)
	}
	var views []serviceSummary
	if err := json.Unmarshal(out.Bytes(), &views); err != nil || len(views) != 1 {
		t.Fatalf("invalid list: %s", out.String())
	}
	if err := run([]string{"services", "example"}, ""); err != nil || !strings.Contains(out.String(), `"enabled":false`) || strings.Contains(out.String(), "definition") {
		t.Fatal("invalid disconnected discovery")
	}
	if err := run([]string{"services", "create", "-"}, serviceJSON); err == nil || out.Len() != 0 {
		t.Fatal("duplicate creation accepted")
	}
	password := strings.Replace(serviceJSON, `"authKind":"token"`, `"authKind":"password","password":{"loginPath":"/login","emailField":"email","passwordField":"password","tokenField":"session.token","refreshPath":"/refresh"}`, 1)
	password = strings.Replace(password, `"id":"example"`, `"id":"password-example"`, 1)
	if err := run([]string{"services", "create", "-"}, password); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"services", "password-example", "--config"}, ""); err != nil || !strings.Contains(out.String(), `"tokenField":"session.token"`) {
		t.Fatal("password contract missing")
	}
}

func TestServicesRejectInvalidInput(t *testing.T) {
	for _, input := range []string{
		`null`, `{`, `[]`, serviceJSON + `{}`, serviceJSON + ` trailing`,
		strings.Replace(serviceJSON, `"name":"Example"`, `"name":"Example","token":"secret"`, 1),
		strings.Replace(serviceJSON, `"queryParameters":["limit"]`, `"queryParameters":["limit"],"method":"POST"`, 1),
		strings.Replace(serviceJSON, "https://", "http://", 1),
		strings.Repeat(" ", 1024*1024+1),
	} {
		store := &midwing.Store{Dir: t.TempDir()}
		var out bytes.Buffer
		if err := runCLIWithInput([]string{"services", "create", "-"}, store, strings.NewReader(input), &out); err == nil || out.Len() != 0 {
			t.Fatal("invalid input accepted or printed to stdout")
		}
		if _, err := os.Stat(filepath.Join(store.Dir, "settings.json")); !os.IsNotExist(err) {
			t.Fatal("invalid input persisted")
		}
	}
	for _, args := range [][]string{{"create"}, {"create", "-", "--file", "x"}, {"create", "--file"}, {"create", "--file", "missing.json"}, {"show", "missing"}, {"list", "extra"}} {
		var out bytes.Buffer
		if err := runServicesCLI(args, &midwing.Store{Dir: t.TempDir()}, strings.NewReader(serviceJSON), &out); err == nil || out.Len() != 0 {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
}

type cliVault map[string]string

func (v cliVault) Set(ref, token string) error    { v[ref] = token; return nil }
func (v cliVault) Get(ref string) (string, error) { return v[ref], nil }
func (v cliVault) Delete(ref string) error        { delete(v, ref); return nil }

type unreadableVault struct{}

func (unreadableVault) Set(string, string) error   { panic("discovery must not access credentials") }
func (unreadableVault) Get(string) (string, error) { panic("discovery must not access credentials") }
func (unreadableVault) Delete(string) error        { panic("discovery must not access credentials") }

func TestDiscoveryGrantedPermissions(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
	definition := midwing.CustomService{ID: "sample-api", Name: "Sample API", BaseURL: "https://api.example.test", AuthKind: "token", Header: "Authorization", Prefix: "Bearer ", Endpoints: []midwing.CustomEndpoint{{Path: "/profile", QueryParameters: []string{"page"}}, {Path: "/items"}}}
	if err := store.CreateCustomService(definition); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveService("sample-api", "private-token", []string{"endpoint-1"}); err != nil {
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

func TestPostCLIUsesMethodSpecificGrant(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
	definition := midwing.CustomService{ID: "write-api", Name: "Write API", BaseURL: "https://api.example.test", AuthKind: "token", Header: "Authorization", Endpoints: []midwing.CustomEndpoint{{Path: "/items", Methods: []string{"GET", "POST"}}}}
	if err := store.CreateCustomService(definition); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveService("write-api", "private-token", []string{"endpoint-1"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := runCLIWithInput([]string{"post", "write-api", "/items", `{"name":"test"}`}, store, nil, &out)
	if err == nil || !strings.Contains(err.Error(), "grant the required permission") || out.Len() != 0 {
		t.Fatalf("POST did not enforce its own grant: %v %s", err, out.String())
	}
	state, err := store.RegistrySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Services[0].Permissions) < 2 || state.Services[0].Permissions[1].Label != "POST /items" {
		t.Fatalf("POST permission missing: %#v", state.Services[0].Permissions)
	}
}

func TestDiscoveryPOSTOnlyGrant(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
	definition := midwing.CustomService{ID: "write-api", Name: "Write API", BaseURL: "https://api.example.test", AuthKind: "token", Header: "Authorization", Endpoints: []midwing.CustomEndpoint{{Path: "/items", Methods: []string{"POST"}}}}
	if err := store.CreateCustomService(definition); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveService(definition.ID, "private-token", []string{"endpoint-1-post"}); err != nil {
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

func TestDefinitionJSONContract(t *testing.T) {
	for _, input := range []string{
		strings.Replace(serviceJSON, `"prefix":"Bearer "`, `"prefix":null`, 1),
		strings.Replace(serviceJSON, `"prefix":"Bearer "`, `"prefix":"Bearer ","password":null`, 1),
		strings.Replace(serviceJSON, `"queryParameters":["limit"]`, `"queryParameters":null`, 1),
		strings.Replace(serviceJSON, `"queryParameters":["limit"]`, `"queryParameters":[null]`, 1),
		strings.Replace(serviceJSON, `"queryParameters":["limit"]`, `"methods":null`, 1),
		strings.Replace(serviceJSON, `"queryParameters":["limit"]`, `"methods":[null]`, 1),
		strings.Replace(serviceJSON, `"queryParameters":["limit"]`, `"methods":[]`, 1),
		strings.Replace(serviceJSON, `"id":`, `"ID":`, 1),
		strings.Replace(serviceJSON, `"path":"/items/{id}"`, `"path":null`, 1),
		strings.Replace(serviceJSON, `"authKind":"token"`, `"authKind":"password","password":{"loginPath":"/login","emailField":"email","passwordField":"password","tokenField":"token","refreshPath":null}`, 1),
	} {
		for _, command := range []string{"create", "validate"} {
			var out bytes.Buffer
			if err := runCLIWithInput([]string{"services", command, "-"}, &midwing.Store{Dir: t.TempDir()}, strings.NewReader(input), &out); err == nil || out.Len() != 0 {
				t.Fatalf("invalid contract accepted: %s", input)
			}
		}
	}
	omitted := strings.Replace(serviceJSON, `,"prefix":"Bearer "`, "", 1)
	omitted = strings.Replace(omitted, `,"queryParameters":["limit"]`, "", 1)
	var out bytes.Buffer
	if err := runCLIWithInput([]string{"services", "validate", "-"}, nil, strings.NewReader(omitted), &out); err != nil {
		t.Fatal("omission defaults rejected:", err)
	}
}

func TestAPICommandsRejectMalformedArguments(t *testing.T) {
	for _, args := range [][]string{
		{"get"},
		{"get", "example"},
		{"get", "example", "/items", "{}"},
		{"post"},
		{"post", "example"},
		{"post", "example", "/items"},
		{"post", "example", "/items", "{}", "{}"},
	} {
		var out bytes.Buffer
		err := runCLIWithInput(args, nil, nil, &out)
		if err == nil || !strings.Contains(err.Error(), "usage: midwing "+args[0]) || out.Len() != 0 {
			t.Fatalf("malformed command %q: error = %v, output = %q", args, err, out.String())
		}
	}
}
