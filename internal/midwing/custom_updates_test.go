package midwing

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestEndpointUpdatesRequireReconnect(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*CustomService)
		method string
		path   string
	}{
		{"add", func(d *CustomService) { d.Endpoints = append(d.Endpoints, CustomEndpoint{Path: "/models"}) }, "GET", "/models"},
		{"remove", func(d *CustomService) { d.Endpoints = d.Endpoints[1:] }, "GET", "/items/123"},
		{"path", func(d *CustomService) { d.Endpoints[0].Path = "/models" }, "GET", "/models"},
		{"add method", func(d *CustomService) { d.Endpoints[0].Methods = []string{"GET", "POST"} }, "POST", "/items"},
		{"replace method", func(d *CustomService) { d.Endpoints[0].Methods = []string{"POST"} }, "POST", "/items"},
		{"add query", func(d *CustomService) {
			d.Endpoints[0].QueryParameters = append(d.Endpoints[0].QueryParameters, "cursor")
		}, "GET", "/items?cursor=next"},
		{"remove query", func(d *CustomService) { d.Endpoints[0].QueryParameters = nil }, "GET", "/items"},
		{"reorder", func(d *CustomService) { d.Endpoints[0], d.Endpoints[1] = d.Endpoints[1], d.Endpoints[0] }, "GET", "/items/123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			d := tokenDefinition()
			if err := s.CreateCustomService(d); err != nil {
				t.Fatal(err)
			}
			if err := s.SaveService(d.ID, "private-api-key", []string{"endpoint-1"}); err != nil {
				t.Fatal(err)
			}
			before, err := s.read()
			if err != nil {
				t.Fatal(err)
			}
			tc.change(&d)
			if err := s.UpdateCustomService(d); err != nil {
				t.Fatal(err)
			}
			reopened := &Store{Dir: s.Dir, Vault: s.Vault}
			state, err := reopened.RegistrySnapshot()
			if err != nil || len(state.Connections) != 0 {
				t.Fatalf("endpoint update retained an active connection: %+v (%v)", state, err)
			}
			stored, err := reopened.read()
			if err != nil || !reflect.DeepEqual(stored.Connections, before.Connections) || !reflect.DeepEqual(stored.CustomServices[0], d) {
				t.Fatalf("update did not persist endpoints and retain saved connection: %+v (%v)", stored, err)
			}
			if token, err := s.Vault.Get(before.Connections[0].CredentialRef); err != nil || token != "private-api-key" {
				t.Fatal("endpoint update lost saved credential")
			}
			service, err := reopened.Service(d.ID)
			if err != nil {
				t.Fatal(err)
			}
			permission, _, err := service.allowed(tc.method, tc.path)
			if err != nil {
				t.Fatalf("updated endpoint is not allowed: %v", err)
			}
			calls := 0
			client := Client{Store: reopened, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != tc.method || r.URL.RequestURI() != "/v1"+tc.path || r.Header.Get("X-API-Key") != "private-api-key" {
					t.Fatal("incorrect updated request")
				}
				return &http.Response{StatusCode: 200, Body: ioBody(`{"ok":true}`)}, nil
			})}}
			var body json.RawMessage
			if tc.method == "POST" {
				body = json.RawMessage(`{"name":"test"}`)
			}
			if _, err := client.CallWithBody(context.Background(), d.ID, tc.method, tc.path, body); err == nil || !strings.Contains(err.Error(), "connect") || calls != 0 {
				t.Fatalf("updated endpoint bypassed reconnection: %v (%d calls)", err, calls)
			}
			if err := reopened.SaveService(d.ID, "private-api-key", []string{permission}); err != nil {
				t.Fatal(err)
			}
			if _, err := client.CallWithBody(context.Background(), d.ID, tc.method, tc.path, body); err != nil || calls != 1 {
				t.Fatalf("reconnected endpoint failed: %v (%d calls)", err, calls)
			}
		})
	}
}

func TestServiceUpdateRejectsInvalidOrFixedFields(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*CustomService)
	}{
		{"id", func(d *CustomService) { d.ID = "missing" }},
		{"name", func(d *CustomService) { d.Name = "Other name" }},
		{"base URL", func(d *CustomService) { d.BaseURL = "https://other.test" }},
		{"credential header", func(d *CustomService) { d.Header = "Authorization" }},
		{"prefix", func(d *CustomService) { d.Prefix = "Bearer " }},
		{"auth kind", func(d *CustomService) {
			d.AuthKind = "password"
			d.Password = &PasswordAuth{LoginPath: "/login", EmailField: "email", PasswordField: "password", TokenField: "token"}
		}},
		{"empty endpoints", func(d *CustomService) { d.Endpoints = nil }},
		{"overlapping endpoints", func(d *CustomService) { d.Endpoints = append(d.Endpoints, CustomEndpoint{Path: "/items/123"}) }},
		{"invalid path", func(d *CustomService) { d.Endpoints[0].Path = "//other.test" }},
		{"invalid method", func(d *CustomService) { d.Endpoints[0].Methods = []string{"DELETE"} }},
		{"invalid query", func(d *CustomService) { d.Endpoints[0].QueryParameters = []string{"bad name"} }},
		{"invalid header", func(d *CustomService) { d.Headers = StaticHeaders{"Authorization": "override"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			d := tokenDefinition()
			if err := s.CreateCustomService(d); err != nil {
				t.Fatal(err)
			}
			if err := s.SaveService(d.ID, "private-api-key", []string{"endpoint-1"}); err != nil {
				t.Fatal(err)
			}
			before, err := s.read()
			if err != nil {
				t.Fatal(err)
			}
			tc.change(&d)
			if err := s.UpdateCustomService(d); err == nil {
				t.Fatal("accepted invalid or unsupported update")
			}
			after, err := s.read()
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("rejected update changed settings")
			}
		})
	}
}

func TestEndpointUpdateRemovesOldAccess(t *testing.T) {
	s := testStore(t)
	d := tokenDefinition()
	if err := s.CreateCustomService(d); err != nil {
		t.Fatal(err)
	}
	d.Endpoints = []CustomEndpoint{{Path: "/models", Methods: []string{"POST"}, QueryParameters: []string{"cursor"}}}
	if err := s.UpdateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveService(d.ID, "private-api-key", []string{"endpoint-1-post"}); err != nil {
		t.Fatal(err)
	}
	client := Client{Store: s, HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		t.Fatal("removed access reached the network")
		return nil, nil
	})}}
	for _, request := range []struct{ method, path string }{
		{"GET", "/items"},
		{"GET", "/items/123"},
		{"GET", "/models"},
		{"POST", "/models?filter=test"},
	} {
		var body json.RawMessage
		if request.method == "POST" {
			body = json.RawMessage(`{}`)
		}
		if _, err := client.CallWithBody(context.Background(), d.ID, request.method, request.path, body); err == nil {
			t.Errorf("removed access succeeded: %s %s", request.method, request.path)
		}
	}
}
