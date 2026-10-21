package midwing

import (
	"context"
	"net/http"
	"testing"
)

func TestCredentialSuppressionInspectsAllJSONStrings(t *testing.T) {
	for _, body := range []string{
		`{"value":"private-token","value":"safe"}`,
		`{"value":"private\u002dtoken","value":"safe"}`,
		`{"nested":{"value":"private-token"},"nested":{}}`,
		`{"private\u002dtoken":"safe"}`,
		`[{"value":"prefix-private-token-suffix"}]`,
		`"private-token"`,
		`{"value":"private\u002dtoken`,
		"{\"value\":\"private\\u002dtoken\"}\n{}",
	} {
		if !responseContainsSecret([]byte(body), "", "old-token", "private-token") {
			t.Errorf("unsafe JSON passed: %s", body)
		}
	}
	for _, body := range []string{`{"value":"safe","value":"still safe"}`, `{"items":[null,true,123,1e1000]}`, `"safe"`, `{"value":`, `{} {}`, `{"value":"private\\u002dtoken"}`} {
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

func TestCredentialSuppressionEscapeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		body, secret string
		want         bool
	}{
		{`{"value":"prefix\uD83D\uDE00suffix`, "prefix😀suffix", true},
		{`{"value":"\u0061\uD83D\uDE00`, "a😀", true},
		{`{"value":"private\u002dtoken\`, "private-token", true},
		{`{"value":"private\\u002dtoken"}`, "private-token", false},
		{`{"value":"quote\"slash\\tab\t`, "quote\"slash\\tab\t", true},
		{"{\"value\":\"private\xff\\u002dtoken\"}", "private\ufffd-token", true},
		{"{\"value\":\"private\xff\xfftoken\"}", "private\ufffd\ufffdtoken", true},
		{"RIFF\x00\xffaudio", "secret-token", false},
	} {
		if got := responseContainsSecret([]byte(tc.body), tc.secret); got != tc.want {
			t.Errorf("body %q: got %v, want %v", tc.body, got, tc.want)
		}
	}
}
