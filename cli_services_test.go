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
		strings.Replace(updated, "api.example.com", "other.example.com", 1),
	} {
		if err := runServicesCLI([]string{"update", "-"}, store, strings.NewReader(invalid), &out); err == nil {
			t.Fatal("accepted missing service or destination change")
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

func TestPostCLIUsesMethodSpecificGrant(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
	definition := midwing.CustomService{ID: "write-api", Name: "Write API", BaseURL: "https://api.example.test", AuthKind: "token", Header: "Authorization", Endpoints: []midwing.CustomEndpoint{{Path: "/items", Methods: []string{"GET", "POST"}}}}
	if err := store.CreateCustomService(definition); err != nil {
		t.Fatal(err)
	}
	if err := saveTestService(store, "write-api", "private-token", []string{"endpoint-1"}); err != nil {
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
