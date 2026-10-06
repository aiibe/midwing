package midwing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryVault map[string]string

func (v memoryVault) Set(k, s string) error { v[k] = s; return nil }
func (v memoryVault) Get(k string) (string, error) {
	s, ok := v[k]
	if !ok {
		return "", errors.New("missing")
	}
	return s, nil
}
func (v memoryVault) Delete(k string) error { delete(v, k); return nil }

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func testStore(t *testing.T) *Store {
	registerTokenFixture(t)
	return &Store{Dir: t.TempDir(), Vault: memoryVault{}}
}
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
	if e := s.SaveService("sample-api", "secret-token", []string{"profile"}); e != nil {
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
	}{{`{"login":"octocat"}`, 200, false}, {`{"message":"secret-token"}`, 200, true}, {`{"token":"secret-token"}`, 401, true}, {`not json`, 200, true}, {``, 302, true}} {
		s := testStore(t)
		if e := s.SaveService("sample-api", "secret-token", []string{"profile"}); e != nil {
			t.Fatal(e)
		}
		c := Client{Store: s, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
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
		b, _ := json.Marshal(history)
		if strings.Contains(string(b), "secret-token") || strings.Contains(string(b), "page=") {
			t.Fatal("sensitive history")
		}
	}
}

type stringBody struct{ *strings.Reader }

func (stringBody) Close() error  { return nil }
func ioBody(s string) stringBody { return stringBody{strings.NewReader(s)} }
func TestRotateAndRemove(t *testing.T) {
	s := testStore(t)
	if e := s.SaveService("sample-api", "first", []string{"profile"}); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveService("sample-api", "second", []string{"items"}); e != nil {
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

func TestCredentialSuppressionInspectsAllJSONStrings(t *testing.T) {
	for _, body := range []string{
		`{"value":"private-token","value":"safe"}`,
		`{"value":"private\u002dtoken","value":"safe"}`,
		`{"nested":{"value":"private-token"},"nested":{}}`,
		`{"private\u002dtoken":"safe"}`,
		`[{"value":"prefix-private-token-suffix"}]`,
		`"private-token"`,
		`{"value":`,
		`{} {}`,
	} {
		if !responseContainsSecret([]byte(body), "", "old-token", "private-token") {
			t.Errorf("unsafe JSON passed: %s", body)
		}
	}
	for _, body := range []string{`{"value":"safe","value":"still safe"}`, `{"items":[null,true,123,1e1000]}`, `"safe"`} {
		if responseContainsSecret([]byte(body), "", "private-token") {
			t.Errorf("safe JSON rejected: %s", body)
		}
	}
}

func TestDuplicateKeyCredentialNeverReachesOutput(t *testing.T) {
	s := testStore(t)
	if err := s.SaveService("sample-api", "private-token", []string{"profile"}); err != nil {
		t.Fatal(err)
	}
	client := Client{Store: s, HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: ioBody(`{"value":"private\u002dtoken","value":"safe"}`)}, nil
	})}}
	result, err := client.Call(context.Background(), "sample-api", "GET", "/profile")
	if err == nil || len(result) != 0 {
		t.Fatal("credential reached API output")
	}
	rows, err := s.History()
	if err != nil || len(rows) != 1 || rows[0].Outcome != "failed" {
		t.Fatal("credential suppression was not recorded as failure")
	}
}

func TestActivityFailurePreservesSanitizedCallError(t *testing.T) {
	for _, status := range []int{200, 401} {
		s := testStore(t)
		if err := s.SaveService("sample-api", "private-token", []string{"profile"}); err != nil {
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
