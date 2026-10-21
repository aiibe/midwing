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

func TestAllowlist(t *testing.T) {
	for _, p := range []string{"https://evil.test/profile", "//evil.test/profile", "/profile#fragment", "/profile?token=secret", "/items/../profile", "/items/a/details", "/%70rofile", "/profile?per_page=101", "/profile?page=1&page=2", "/profile?bad=%zz"} {
		if _, _, e := tokenFixture.allowed("GET", p); e == nil {
			t.Errorf("accepted %s", p)
		}
	}
	if _, _, e := tokenFixture.allowed("POST", "/profile"); e == nil {
		t.Fatal("accepted write")
	}
	for _, p := range []string{"/profile", "/items?page=2&per_page=100", "/items/example", "/items/123", "/items/abc"} {
		if _, _, e := tokenFixture.allowed("GET", p); e != nil {
			t.Errorf("rejected %s: %v", p, e)
		}
	}
}

func TestPermissionsAndCredentialPrivacy(t *testing.T) {
	s := testStore(t)
	if e := saveTestService(s, "sample-api", "secret-token", []string{"profile"}); e != nil {
		t.Fatal(e)
	}
	called := 0
	c := Client{Store: s, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		called++
		if r.URL.Host != "api.example.test" || r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Fatal("incorrect destination or credential")
		}
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})}}
	if _, e := c.Call(context.Background(), "sample-api", "GET", "/items"); e == nil || called != 0 {
		t.Fatal("permission bypass")
	}
	cfg, e := s.List()
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(cfg)
	if strings.Contains(string(b), "secret-token") {
		t.Fatal("token in settings")
	}
	history, e := s.History()
	if e != nil || len(history) != 1 || history[0].Outcome != "denied" {
		t.Fatalf("history: %v %v", history, e)
	}
}

func TestResponseAndRedirect(t *testing.T) {
	for _, tc := range []struct {
		body      string
		status    int
		wantError bool
	}{{`{"login":"octocat"}`, 200, false}, {`{"message":"secret-token"}`, 200, true}, {`{"token":"secret-token"}`, 401, true}, {`not json`, 200, true}, {``, 204, false}, {``, 200, false}, {` `, 200, true}, {``, 302, true}} {
		s := testStore(t)
		if e := saveTestService(s, "sample-api", "secret-token", []string{"profile"}); e != nil {
			t.Fatal(e)
		}
		c := Client{Store: s, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Accept") != "application/json" {
				t.Fatal("JSON request must accept JSON responses")
			}
			return &http.Response{StatusCode: tc.status, Header: http.Header{"Location": []string{"https://evil.test"}}, Body: ioBody(tc.body)}, nil
		})}}
		result, e := c.Call(context.Background(), "sample-api", "GET", "/profile?page=1")
		if (e != nil) != tc.wantError {
			t.Fatalf("status %d: %s %v", tc.status, result, e)
		}
		history, e := s.History()
		if e != nil || len(history) != 1 {
			t.Fatal(e)
		}
		wantOutcome := "success"
		if tc.wantError {
			wantOutcome = "failed"
		}
		if history[0].Outcome != wantOutcome {
			t.Fatalf("body %q: outcome = %q, want %q", tc.body, history[0].Outcome, wantOutcome)
		}
		b, _ := json.Marshal(history)
		if strings.Contains(string(b), "secret-token") || strings.Contains(string(b), "page=") {
			t.Fatal("sensitive history")
		}
	}
}

func TestRotateAndRemove(t *testing.T) {
	s := testStore(t)
	if e := saveTestService(s, "sample-api", "first", []string{"profile"}); e != nil {
		t.Fatal(e)
	}
	if e := saveTestService(s, "sample-api", "second", []string{"items"}); e != nil {
		t.Fatal(e)
	}
	if len(s.Vault.(memoryVault)) != 1 {
		t.Fatal("old credential remains")
	}
	if e := s.RemoveService("sample-api"); e != nil {
		t.Fatal(e)
	}
	c, e := s.List()
	if e != nil || len(c) != 0 || len(s.Vault.(memoryVault)) != 0 {
		t.Fatal("connection not removed")
	}
}

func TestActivityFailurePreservesSanitizedCallError(t *testing.T) {
	for _, status := range []int{200, 401} {
		s := testStore(t)
		if err := saveTestService(s, "sample-api", "private-token", []string{"profile"}); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(s.Dir, "activity.jsonl"), 0700); err != nil {
			t.Fatal(err)
		}
		client := Client{Store: s, HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: ioBody(`{"value":"safe"}`)}, nil
		})}}
		body, err := client.Call(context.Background(), "sample-api", "GET", "/profile")
		if len(body) != 0 || err == nil || !strings.Contains(err.Error(), "could not record activity") {
			t.Fatal("activity failure exposed output")
		}
		if status == 401 && !strings.Contains(err.Error(), "unauthorized") {
			t.Fatal("call error was lost")
		}
		if strings.Contains(err.Error(), "private-token") {
			t.Fatal("error exposed credential")
		}
	}
}

func TestInvalidRequestBodiesNeverReachService(t *testing.T) {
	store := testStore(t)
	d := tokenDefinition()
	d.Endpoints = []CustomEndpoint{{Path: "/items", Methods: []string{"GET", "POST"}}}
	if err := store.CreateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := saveTestService(store, d.ID, "secret-token", []string{"endpoint-1", "endpoint-1-post"}); err != nil {
		t.Fatal(err)
	}
	client := Client{Store: store, HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid request reached service")
		return nil, nil
	})}}
	for _, tc := range []struct{ name, method, body string }{
		{"GET body", "GET", "{}"},
		{"missing POST body", "POST", ""},
		{"invalid JSON", "POST", "not-json"},
		{"multiple JSON values", "POST", "{} {}"},
		{"oversized JSON", "POST", `"` + strings.Repeat("a", 1024*1024) + `"`},
		{"unsupported method", "DELETE", "{}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := client.CallWithBody(context.Background(), d.ID, tc.method, "/items", json.RawMessage(tc.body)); err == nil {
				t.Fatal("accepted invalid request")
			}
		})
	}
}

func TestPOSTOnlyEndpointRejectsGET(t *testing.T) {
	d := tokenDefinition()
	d.Endpoints = []CustomEndpoint{{Path: "/items", Methods: []string{"POST"}}}
	service, err := d.compile()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.allowed("GET", "/items"); err == nil {
		t.Fatal("POST-only endpoint allowed GET")
	}
	permission, _, err := service.allowed("POST", "/items")
	if err != nil || permission != "endpoint-1-post" {
		t.Fatalf("POST permission = %q, error = %v", permission, err)
	}
}
