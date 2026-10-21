package midwing

import (
	"context"
	"net/http"
	"testing"
)

func tokenDefinition() CustomService {
	return CustomService{ID: "my-api", Name: "My API", BaseURL: "https://example.test/v1", AuthKind: "token", Header: "X-API-Key", Endpoints: []CustomEndpoint{{Path: "/items", QueryParameters: []string{"filter", "limit"}}, {Path: "/items/{id}"}}}
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
		{"unsupported method", func(d *CustomService) { d.Endpoints[0].Methods = []string{"DELETE"} }},
		{"duplicate method", func(d *CustomService) { d.Endpoints[0].Methods = []string{"POST", "POST"} }},
		{"lowercase method", func(d *CustomService) { d.Endpoints[0].Methods = []string{"post"} }},
		{"empty methods", func(d *CustomService) { d.Endpoints[0].Methods = []string{} }},
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
