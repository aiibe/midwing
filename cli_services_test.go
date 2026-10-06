package main

import (
	"bytes"
	"encoding/json"
	"midwing/internal/midwing"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const serviceJSON = `{"id":"example","name":"Example","baseUrl":"https://api.example.com/v1","authKind":"token","header":"Authorization","prefix":"Bearer ","endpoints":[{"path":"/items/{id}","queryParameters":["limit"]}]}`

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

func TestSchemaAndHelp(t *testing.T) {
	var schema struct {
		Examples []json.RawMessage `json:"examples"`
	}
	data, err := json.Marshal(serviceSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &schema); err != nil || len(schema.Examples) != 2 {
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
	for _, args := range [][]string{{"--help"}, {"get", "--help"}, {"services", "--help"}, {"services", "schema", "--help"}, {"services", "create", "--help"}, {"services", "validate", "--help"}, {"services", "example", "--help"}} {
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
