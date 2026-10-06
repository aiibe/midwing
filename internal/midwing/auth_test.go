package midwing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestPasswordLoginAndRefresh(t *testing.T) {
	registerPasswordFixture(t)
	s := testStore(t)
	if e := s.SaveService("sample-api", "sample-api-secret", []string{"profile"}); e != nil {
		t.Fatal(e)
	}
	calls := 0
	c := Client{Store: s, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "sample.test" {
			t.Fatal("wrong host")
		}
		body := ""
		switch r.URL.Path {
		case "/api/collections/users/auth-with-password":
			if r.Method != "POST" || r.Header.Get("Authorization") != "" {
				t.Fatal("wrong login request")
			}
			var payload map[string]string
			if e := json.NewDecoder(r.Body).Decode(&payload); e != nil {
				t.Fatal(e)
			}
			if payload["identity"] != "me@example.com" || payload["password"] != "private-password" {
				t.Fatal("wrong login fields")
			}
			body = `{"token":"first-token","record":{"email":"me@example.com"}}`
		case "/api/collections/users/auth-refresh":
			if r.Method != "POST" || r.Header.Get("Authorization") != "first-token" {
				t.Fatal("wrong refresh header")
			}
			body = `{"token":"refreshed-token"}`
		case "/api/collections/sheets/records":
			if r.Method != "GET" || r.Header.Get("Authorization") != "refreshed-token" {
				t.Fatal("wrong API header")
			}
			body = `{"items":[]}`
		default:
			t.Fatal("unexpected endpoint")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: ioBody(body)}, nil
	})}}
	if e := c.ConnectPassword(context.Background(), "sample-auth", "me@example.com", "private-password", []string{"sheets"}); e != nil {
		t.Fatal(e)
	}
	rows, e := s.List()
	if e != nil || len(rows) != 2 {
		t.Fatalf("connections %v %v", rows, e)
	}
	data, e := c.Call(context.Background(), "sample-auth", "GET", "/api/collections/sheets/records?perPage=10")
	if e != nil || string(data) != `{"items":[]}` || calls != 3 {
		t.Fatalf("call %s %v %d", data, e, calls)
	}
	if _, e = c.Call(context.Background(), "sample-auth", "GET", "/api/collections/charges/records"); e == nil || calls != 3 {
		t.Fatal("denied permission made network call")
	}
	for _, file := range []string{"settings.json", "activity.jsonl"} {
		b, e := os.ReadFile(filepath.Join(s.Dir, file))
		if e != nil {
			t.Fatal(e)
		}
		for _, secret := range []string{"private-password", "first-token", "refreshed-token", "me@example.com"} {
			if strings.Contains(string(b), secret) {
				t.Fatalf("secret persisted in %s", file)
			}
		}
	}
	if e = s.RemoveService("sample-auth"); e != nil {
		t.Fatal(e)
	}
	rows, e = s.List()
	if e != nil || len(rows) != 1 || rows[0].Service != "sample-api" || len(s.Vault.(memoryVault)) != 1 {
		t.Fatal("disconnect removed another account")
	}
}
func TestDifferentJSONLoginProtocol(t *testing.T) {
	// A different API uses email/secret fields, a nested token, a custom header,
	// and no refresh endpoint. No framework-specific code is involved.
	definition := Service{ID: "example", Name: "Example", Origin: "https://example.test", AuthKind: "password", Header: "X-Session", Prefix: "Session ", Password: &PasswordAuth{LoginPath: "/login", EmailField: "email", PasswordField: "secret", TokenField: "session.accessToken"}, Permissions: []Permission{{"read", "Read", "Read things"}}, Endpoints: []Endpoint{{nil, nil, "read", regexp.MustCompile(`^/things$`), nil}}}
	registerServiceFixture(t, definition)
	s := testStore(t)
	requests := 0
	c := Client{Store: s, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Host != "example.test" {
			t.Fatal("wrong origin")
		}
		body := `{"ok":true}`
		if r.URL.Path == "/login" {
			var payload map[string]string
			json.NewDecoder(r.Body).Decode(&payload)
			if payload["email"] != "me@example.com" || payload["secret"] != "pass" {
				t.Fatal("wrong fields")
			}
			body = `{"session":{"accessToken":"example-token"}}`
		} else if r.URL.Path != "/things" || r.Header.Get("X-Session") != "Session example-token" {
			t.Fatal("wrong API request")
		}
		return &http.Response{StatusCode: 200, Body: ioBody(body)}, nil
	})}}
	if e := c.ConnectPassword(context.Background(), "example", "me@example.com", "pass", []string{"read"}); e != nil {
		t.Fatal(e)
	}
	if _, e := c.Call(context.Background(), "example", "GET", "/things"); e != nil {
		t.Fatal(e)
	}
	if requests != 2 {
		t.Fatal("unexpected refresh")
	}
}
func TestLoginFailuresKeepConnectionAndSuppressPayload(t *testing.T) {
	registerPasswordFixture(t)
	for _, tc := range []struct {
		status int
		body   string
	}{{401, `{"message":"private-password"}`}, {302, `{}`}, {200, `{"token":""}`}, {200, `{"token":"private-password"}`}, {200, `{"token":"bad\r\nheader"}`}, {200, `invalid`}} {
		s := testStore(t)
		seedPasswordConnection(t, s, "sample-auth", "existing-token", []string{"sheets"})
		c := Client{Store: s, HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Header: http.Header{"Location": []string{"https://other.test"}}, Body: ioBody(tc.body)}, nil
		})}}
		e := c.ConnectPassword(context.Background(), "sample-auth", "me@example.com", "private-password", []string{"sheets"})
		if e == nil || strings.Contains(e.Error(), "private-password") {
			t.Fatalf("unsafe failure %v", e)
		}
		rows, _ := s.List()
		token, _ := s.Vault.Get(rows[0].CredentialRef)
		if token != "existing-token" {
			t.Fatal("failed login replaced existing token")
		}
	}
}
func TestPermissionValidationBeforeLogin(t *testing.T) {
	registerPasswordFixture(t)
	c := Client{Store: testStore(t), HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid login made network request")
		return nil, nil
	})}}
	for _, permissions := range [][]string{nil, {"profile"}} {
		if e := c.ConnectPassword(context.Background(), "sample-auth", "me@example.com", "pass", permissions); e == nil {
			t.Fatal("accepted invalid permissions")
		}
	}
}
func TestRefreshDoesNotResurrectConnection(t *testing.T) {
	registerPasswordFixture(t)
	s := testStore(t)
	seedPasswordConnection(t, s, "sample-auth", "old-token", []string{"sheets"})
	rows, _ := s.List()
	s.RemoveService("sample-auth")
	if e := s.UpdateToken(rows[0], "old-token", "new-token"); e == nil {
		t.Fatal("updated disconnected connection")
	}
	if len(s.Vault.(memoryVault)) != 0 {
		t.Fatal("credential resurrected")
	}
}
func TestPasswordServiceAllowlist(t *testing.T) {
	registerPasswordFixture(t)
	s, _ := findService(services, "sample-auth")
	for _, path := range []string{"/api/collections/users/records", "/api/collections/users/auth-refresh", "/api/collections/sheets/records?expand=user", "/api/collections/sheets/records?perPage=101", "/api/collections/sheets/records/..", "//evil.test/api/collections/sheets/records", "/api/collections/sheets/records?per_page=5"} {
		if _, _, e := s.allowed("GET", path); e == nil {
			t.Errorf("accepted %s", path)
		}
	}
	for _, path := range []string{"/api/collections/sheets/records", "/api/collections/charges/records?page=2&perPage=25", "/api/collections/sheets/records/123456789012345"} {
		if _, _, e := s.allowed("GET", path); e != nil {
			t.Errorf("rejected %s: %v", path, e)
		}
	}
}
func TestExpiredRefreshDoesNotCallAPI(t *testing.T) {
	registerPasswordFixture(t)
	s := testStore(t)
	seedPasswordConnection(t, s, "sample-auth", "expired-token", []string{"sheets"})
	calls := 0
	c := Client{Store: s, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.HasSuffix(r.URL.Path, "/auth-refresh") {
			t.Fatal("expired auth reached API")
		}
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{"token":"expired-token"}`))}, nil
	})}}
	_, e := c.Call(context.Background(), "sample-auth", "GET", "/api/collections/sheets/records")
	if e == nil || !strings.Contains(e.Error(), "reconnect") || calls != 1 {
		t.Fatalf("wrong expired behavior %v %d", e, calls)
	}
}

func TestEscapedCredentialSuppression(t *testing.T) {
	if !responseContainsSecret([]byte(`{"value":"prefix-secret\u002dtoken"}`), "secret-token") {
		t.Fatal("escaped credential was not detected")
	}
	if responseContainsSecret([]byte(`{"items":[{"name":"public"}]}`), "secret-token") {
		t.Fatal("safe response rejected")
	}
}

func registerPasswordFixture(t *testing.T) {
	t.Helper()
	registerServiceFixture(t, Service{ID: "sample-auth", Name: "Sample API", Description: "Read your monthly sheets and charges.", Origin: "https://sample.test", AuthKind: "password", Header: "Authorization", Permissions: []Permission{{"sheets", "Sheets", "Read monthly sheets"}, {"charges", "Charges", "Read charges"}}, Examples: []string{"/api/collections/sheets/records", "/api/collections/charges/records"}, Password: &PasswordAuth{LoginPath: "/api/collections/users/auth-with-password", EmailField: "identity", PasswordField: "password", TokenField: "token", RefreshPath: "/api/collections/users/auth-refresh"}, Endpoints: []Endpoint{
		{nil, []string{"/api/collections/sheets/records", "/api/collections/sheets/records/RECORD_ID"}, "sheets", regexp.MustCompile(`^/api/collections/sheets/records(/[A-Za-z0-9]{15})?$`), map[string]int{"page": 0, "perPage": 100}},
		{nil, []string{"/api/collections/charges/records", "/api/collections/charges/records/RECORD_ID"}, "charges", regexp.MustCompile(`^/api/collections/charges/records(/[A-Za-z0-9]{15})?$`), map[string]int{"page": 0, "perPage": 100}},
	}})
}

// Seed an already-authenticated connection without making a login request.
func seedPasswordConnection(t *testing.T, store *Store, id, token string, permissions []string) error {
	t.Helper()
	service, err := store.Service(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.saveToken(id, token, permissions, &service); err != nil {
		t.Fatal(err)
	}
	return nil
}
