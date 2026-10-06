package midwing

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tokenDefinition() CustomService {
	return CustomService{ID: "my-api", Name: "My API", BaseURL: "https://example.test/v1", AuthKind: "token", Header: "X-API-Key", Endpoints: []CustomEndpoint{{Path: "/items", QueryParameters: []string{"filter", "limit"}}, {Path: "/items/{id}"}}}
}
func TestCustomTokenServiceSurvivesRestart(t *testing.T) {
	s := testStore(t)
	definition := tokenDefinition()
	if e := s.CreateCustomService(definition); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveService(definition.ID, "private-api-key", []string{"endpoint-1"}); e != nil {
		t.Fatal(e)
	}
	reopened := &Store{Dir: s.Dir, Vault: s.Vault}
	list, e := reopened.Services()
	if e != nil || len(list) != 2 || !list[1].Custom {
		t.Fatalf("services %v %v", list, e)
	}
	calls := 0
	c := Client{Store: reopened, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "example.test" || r.URL.Path != "/v1/items" || r.Header.Get("X-API-Key") != "private-api-key" || r.URL.Query().Get("filter") != "hello world" {
			t.Fatalf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: ioBody(`{"items":[]}`)}, nil
	})}}
	if _, e = c.Call(context.Background(), definition.ID, "GET", "/items?filter=hello%20world&limit=25"); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/items/123", "/items?unknown=1", "/items?filter=a&filter=b", "/items/../items", "https://other.test/items"} {
		if _, e = c.Call(context.Background(), definition.ID, "GET", path); e == nil {
			t.Errorf("accepted %s", path)
		}
	}
	if calls != 1 {
		t.Fatal("denied operation reached network")
	}
	state, e := reopened.RegistrySnapshot()
	if e != nil || len(state.Connections) != 1 || len(state.Connections[0].Permissions) != 1 || len(state.Services[1].Endpoints[0].Parameters) != 2 {
		t.Fatalf("registry %v %v", state, e)
	}
	for _, file := range []string{"settings.json", "activity.jsonl"} {
		b, e := os.ReadFile(filepath.Join(s.Dir, file))
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(b), "private-api-key") {
			t.Fatal("credential persisted outside vault")
		}
	}
}
func TestCustomPasswordService(t *testing.T) {
	s := testStore(t)
	d := tokenDefinition()
	d.AuthKind = "password"
	d.Header = "Authorization"
	d.Prefix = "Bearer "
	d.Password = &PasswordAuth{LoginPath: "/login", EmailField: "email", PasswordField: "secret", TokenField: "session.token", RefreshPath: "/refresh"}
	if e := s.CreateCustomService(d); e != nil {
		t.Fatal(e)
	}
	s = &Store{Dir: s.Dir, Vault: s.Vault}
	calls := 0
	c := Client{Store: s, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		body := `{"items":[]}`
		switch r.URL.Path {
		case "/v1/login":
			var payload map[string]string
			if e := json.NewDecoder(r.Body).Decode(&payload); e != nil {
				t.Fatal(e)
			}
			if payload["email"] != "me@example.com" || payload["secret"] != "password-secret" || r.Method != "POST" {
				t.Fatal("wrong login payload")
			}
			body = `{"session":{"token":"initial-token"}}`
		case "/v1/refresh":
			if r.Header.Get("Authorization") != "Bearer initial-token" || r.Method != "POST" {
				t.Fatal("wrong refresh")
			}
			body = `{"session":{"token":"renewed-token"}}`
		case "/v1/items":
			if r.Header.Get("Authorization") != "Bearer renewed-token" {
				t.Fatal("wrong API credential")
			}
		default:
			t.Fatal("unexpected endpoint")
		}
		return &http.Response{StatusCode: 200, Body: ioBody(body)}, nil
	})}}
	if e := c.ConnectPassword(context.Background(), d.ID, "me@example.com", "password-secret", []string{"endpoint-1"}); e != nil {
		t.Fatal(e)
	}
	if _, e := c.Call(context.Background(), d.ID, "GET", "/items"); e != nil || calls != 3 {
		t.Fatalf("call failed %v %d", e, calls)
	}
	bytes, e := os.ReadFile(filepath.Join(s.Dir, "settings.json"))
	if e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"password-secret", "initial-token", "renewed-token", "me@example.com"} {
		if strings.Contains(string(bytes), secret) {
			t.Fatal("login credential leaked into settings")
		}
	}
}
func TestCustomDefinitionValidation(t *testing.T) {
	cases := []struct {
		name   string
		modify func(*CustomService)
	}{
		{"HTTP", func(d *CustomService) { d.BaseURL = "http://example.test" }},
		{"embedded credential", func(d *CustomService) { d.BaseURL = "https://user:secret@example.test" }},
		{"query", func(d *CustomService) { d.BaseURL = "https://example.test?token=secret" }},
		{"fragment", func(d *CustomService) { d.BaseURL = "https://example.test#" }},
		{"base traversal", func(d *CustomService) { d.BaseURL = "https://example.test/a/../b" }},
		{"endpoint traversal", func(d *CustomService) { d.Endpoints[0].Path = "/items/../auth" }},
		{"arbitrary host", func(d *CustomService) { d.Endpoints[0].Path = "//other.test/items" }},
		{"encoded path", func(d *CustomService) { d.Endpoints[0].Path = "/%69tems" }},
		{"invalid template", func(d *CustomService) { d.Endpoints[0].Path = "/items/{a.b}" }},
		{"duplicate template", func(d *CustomService) { d.Endpoints[0].Path = "/items/{id}/{id}" }},
		{"duplicate pattern", func(d *CustomService) { d.Endpoints = []CustomEndpoint{{Path: "/items/{id}"}, {Path: "/items/{name}"}} }},
		{"invalid header", func(d *CustomService) { d.Header = "X-Key\r\nInjected" }},
		{"host header", func(d *CustomService) { d.Header = "Host" }},
		{"invalid prefix", func(d *CustomService) { d.Prefix = "Bearer\r\nInjected" }},
		{"unknown auth", func(d *CustomService) { d.AuthKind = "cookie" }},
		{"missing login", func(d *CustomService) { d.AuthKind = "password" }},
		{"no endpoints", func(d *CustomService) { d.Endpoints = nil }},
		{"invalid query name", func(d *CustomService) { d.Endpoints[0].QueryParameters = []string{"redirect URL"} }},
		{"invalid ID", func(d *CustomService) { d.ID = "my api" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := tokenDefinition()
			tc.modify(&d)
			if e := testStore(t).CreateCustomService(d); e == nil {
				t.Fatal("invalid definition saved")
			}
		})
	}
	for _, id := range []string{"sample-api"} {
		d := tokenDefinition()
		d.ID = id
		if e := testStore(t).CreateCustomService(d); e == nil {
			t.Fatal("built-in service overwritten")
		}
	}
	d := tokenDefinition()
	d.AuthKind = "password"
	d.Password = &PasswordAuth{LoginPath: "/items/anything", EmailField: "email", PasswordField: "password", TokenField: "token"}
	d.Endpoints = []CustomEndpoint{{Path: "/items/{id}"}}
	if e := testStore(t).CreateCustomService(d); e == nil {
		t.Fatal("auth path exposed")
	}
}
func TestCustomDeletionPreservesOtherServices(t *testing.T) {
	s := testStore(t)
	s.SaveService("sample-api", "sample-api-secret", []string{"profile"})
	d := tokenDefinition()
	if e := s.CreateCustomService(d); e != nil {
		t.Fatal(e)
	}
	s.SaveService(d.ID, "custom-secret", []string{"endpoint-1"})
	if e := s.DeleteCustomService(d.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Service(d.ID); e == nil {
		t.Fatal("deleted definition remains")
	}
	connections, e := s.List()
	if e != nil || len(connections) != 1 || connections[0].Service != "sample-api" || len(s.Vault.(memoryVault)) != 1 {
		t.Fatal("unrelated account changed")
	}
	if e := s.DeleteCustomService("sample-api"); e == nil {
		t.Fatal("built-in deleted")
	}
}
func TestDefinitionChangeCannotReuseCredential(t *testing.T) {
	s := testStore(t)
	d := tokenDefinition()
	s.CreateCustomService(d)
	s.SaveService(d.ID, "old-secret", []string{"endpoint-1"})
	old, e := s.Service(d.ID)
	if e != nil {
		t.Fatal(e)
	}
	s.locked(func() error {
		cfg, e := s.read()
		if e != nil {
			return e
		}
		cfg.CustomServices[0].BaseURL = "https://other.test"
		return s.write(cfg)
	})
	c := Client{Store: s, HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		t.Fatal("stale credential reached changed service")
		return nil, nil
	})}}
	if _, e := c.Call(context.Background(), d.ID, "GET", "/items"); e == nil {
		t.Fatal("changed definition reused credential")
	}
	if e := s.saveToken(d.ID, "stale-login-token", []string{"endpoint-1"}, &old); e == nil {
		t.Fatal("old login token saved for changed definition")
	}
}

func TestRemovedBuiltInIsNotListed(t *testing.T) {
	s := testStore(t)
	if e := s.locked(func() error {
		cfg, e := s.read()
		if e != nil {
			return e
		}
		cfg.Connections = append(cfg.Connections, Connection{Service: "fiduro", CredentialRef: "legacy-ref", Permissions: []string{"sheets"}})
		return s.write(cfg)
	}); e != nil {
		t.Fatal(e)
	}
	all, e := s.Services()
	if e != nil {
		t.Fatal(e)
	}
	for _, service := range all {
		if service.ID == "fiduro" {
			t.Fatal("removed built-in still advertised")
		}
	}
	connections, e := s.List()
	if e != nil || len(connections) != 0 {
		t.Fatal("removed connection still listed")
	}
	d := tokenDefinition()
	d.ID = "fiduro"
	if e := s.CreateCustomService(d); e != nil {
		t.Fatal("removed built-in ID cannot be used for custom service")
	}
}

func TestOverlappingEndpointsRejected(t *testing.T) {
	for _, paths := range [][]string{
		{"/items/{id}", "/items/special"},
		{"/items/special", "/items/{id}"},
		{"/{group}/items", "/teams/{kind}"},
		{"/items/{id}/{part}", "/items/fixed/{name}"},
	} {
		d := tokenDefinition()
		d.Endpoints = []CustomEndpoint{{Path: paths[0]}, {Path: paths[1]}}
		if _, err := ValidateCustomService(d); err == nil {
			t.Errorf("overlap accepted: %v", paths)
		}
	}
	for _, paths := range [][]string{{"/items/{id}", "/items/.hidden"}, {"/items/{id}", "/items/{id}/detail"}, {"/items/a", "/items/b"}} {
		d := tokenDefinition()
		d.Endpoints = []CustomEndpoint{{Path: paths[0]}, {Path: paths[1]}}
		if _, err := ValidateCustomService(d); err != nil {
			t.Errorf("disjoint paths rejected: %v: %v", paths, err)
		}
	}
}

func TestPermissionDescriptionsPreserveConnection(t *testing.T) {
	s := testStore(t)
	d := tokenDefinition()
	// The original JSON shape must retain its hash when descriptions are absent.
	legacy, _ := json.Marshal(d)
	if strings.Contains(string(legacy), "description") {
		t.Fatal("empty description changed legacy JSON")
	}
	if err := s.CreateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveService(d.ID, "private-api-key", []string{"endpoint-1"}); err != nil {
		t.Fatal(err)
	}
	before, err := s.RegistrySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	d.Endpoints[0].Description = "Updated through CLI"
	if err := s.UpdateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePermissionDescriptions(d.ID, map[string]string{"/items": "  Read your items  ", "/items/{id}": " "}); err != nil {
		t.Fatal(err)
	}
	reopened := &Store{Dir: s.Dir, Vault: s.Vault}
	after, err := reopened.RegistrySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Connections) != 1 || after.Connections[0].CredentialRef != before.Connections[0].CredentialRef || after.Connections[0].DefinitionHash != before.Connections[0].DefinitionHash || len(after.Connections[0].Permissions) != 1 {
		t.Fatal("description update changed connection")
	}
	services, err := reopened.Services()
	if err != nil {
		t.Fatal(err)
	}
	custom := services[len(services)-1]
	if custom.Permissions[0].Description != "Read your items" || custom.Permissions[1].Description != "Read /items/{id}" {
		t.Fatalf("unexpected descriptions: %v", custom.Permissions)
	}
	for _, descriptions := range []map[string]string{{"/unknown": "Unknown"}, {"/items": strings.Repeat("x", 501)}} {
		if err := s.UpdatePermissionDescriptions(d.ID, descriptions); err == nil {
			t.Fatal("accepted invalid description update")
		}
	}
	d.Endpoints[0].Description = "Different wording"
	if d.definitionHash() != tokenDefinition().definitionHash() {
		t.Fatal("description changes hash")
	}
	d.Endpoints[0].Description = strings.Repeat("x", 501)
	if _, err := d.compile(); err == nil {
		t.Fatal("accepted excessive description on creation")
	}
}
