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

func TestCustomTokenServiceSurvivesRestart(t *testing.T) {
	s := testStore(t)
	definition := tokenDefinition()
	definition.Headers = StaticHeaders{"X-API-Version": "2026-08-14"}
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
		if r.Header.Get("X-API-Version") != "2026-08-14" {
			t.Fatal("missing static request header")
		}
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

func TestCustomPOSTRequiresItsOwnPermissionAndSendsJSON(t *testing.T) {
	s := testStore(t)
	d := tokenDefinition()
	d.Headers = StaticHeaders{"X-API-Version": "2026-08-14"}
	d.Endpoints = []CustomEndpoint{{Path: "/items", Methods: []string{"GET", "POST"}}}
	if err := s.CreateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveService(d.ID, "private-api-key", []string{"endpoint-1"}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	c := Client{Store: s, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("X-API-Version") != "2026-08-14" {
			t.Fatal("missing static request header")
		}
		if r.Method != "POST" || r.URL.Path != "/v1/items" || r.Header.Get("X-API-Key") != "private-api-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatal("incorrect POST request")
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload["name"] == "" {
			t.Fatal("incorrect request JSON")
		}
		if payload["name"] == "private-api-key" {
			return &http.Response{StatusCode: 200, Body: ioBody(`{"created":"private-api-key"}`)}, nil
		}
		return &http.Response{StatusCode: 200, Body: ioBody(`{"created":true}`)}, nil
	})}}
	if _, err := c.CallWithBody(context.Background(), d.ID, "POST", "/items", json.RawMessage(`{"name":"private-body-value"}`)); err == nil || calls != 0 {
		t.Fatal("POST bypassed its separate permission")
	}
	if err := s.SaveService(d.ID, "private-api-key", []string{"endpoint-1", "endpoint-1-post"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CallWithBody(context.Background(), d.ID, "POST", "/items", json.RawMessage(`{"name":"private-body-value"}`)); err != nil || calls != 1 {
		t.Fatalf("authorized POST failed: %v", err)
	}
	if _, err := c.CallWithBody(context.Background(), d.ID, "POST", "/items", json.RawMessage(`not-json`)); err == nil || calls != 1 {
		t.Fatal("invalid JSON reached service")
	}
	if _, err := c.CallWithBody(context.Background(), d.ID, "POST", "/items", json.RawMessage(`{"name":"private-api-key"}`)); err == nil || calls != 2 {
		t.Fatal("credential echo was not suppressed")
	}
	history, err := s.History()
	if err != nil || len(history) != 3 || history[0].Operation != "my-api POST /items" {
		t.Fatalf("unexpected activity: %#v %v", history, err)
	}
	for _, row := range history {
		if strings.Contains(row.Operation, "private-body-value") || strings.Contains(row.Operation, "private-api-key") {
			t.Fatal("request body leaked into activity")
		}
	}
}

func TestCustomPasswordService(t *testing.T) {
	s := testStore(t)
	d := tokenDefinition()
	d.Headers = StaticHeaders{"X-API-Version": "2026-08-14"}
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
		if r.Header.Get("X-API-Version") != "2026-08-14" {
			t.Fatal("missing static request header")
		}
		body := `{"items":[]}`
		switch r.URL.Path {
		case "/v1/login":
			if r.Header.Get("Authorization") != "" {
				t.Fatal("login sent an authentication header")
			}
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
