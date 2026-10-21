package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"midwing/internal/midwing"
)

type requestTransport func(*http.Request) (*http.Response, error)

func (f requestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPOSTOutputFile(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
	d := midwing.CustomService{
		ID: "audio", Name: "Audio", BaseURL: "https://audio.example.test", AuthKind: "token", Header: "X-API-Key",
		Endpoints: []midwing.CustomEndpoint{{Path: "/audio", Methods: []string{"POST"}}},
	}
	if err := store.CreateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveService(d.ID, "secret-token", []string{"endpoint-1-post"}); err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	for _, tc := range []struct {
		name, response string
		requestBody    string
		status         int
		existing       bool
		wantError      bool
	}{
		{name: "binary", response: "RIFF\x00\xffaudio"},
		{name: "credential", response: "error: secret-token", wantError: true},
		{name: "escaped credential", response: `{"value":"secret\u002dtoken"}`, wantError: true},
		{name: "HTTP error", status: http.StatusInternalServerError, wantError: true},
		{name: "invalid request", requestBody: `{`, wantError: true},
		{name: "existing file", existing: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			http.DefaultTransport = requestTransport(func(*http.Request) (*http.Response, error) {
				calls++
				status := tc.status
				if status == 0 {
					status = http.StatusOK
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(tc.response))}, nil
			})
			path := filepath.Join(t.TempDir(), "audio.bin")
			if tc.existing {
				if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			body := tc.requestBody
			if body == "" {
				body = `{}`
			}
			err := runCLI([]string{"post", d.ID, "/audio", body, "--output", path}, store, &out)
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
			data, readErr := os.ReadFile(path)
			switch {
			case tc.existing:
				if readErr != nil || string(data) != "keep" || calls != 0 {
					t.Fatal("existing file was changed or request sent")
				}
			case tc.wantError:
				if !os.IsNotExist(readErr) {
					t.Fatal("failed response left an output file")
				}
			default:
				info, statErr := os.Stat(path)
				if readErr != nil || statErr != nil || string(data) != tc.response || info.Mode().Perm() != 0600 {
					t.Fatal("incorrect output bytes or file permissions")
				}
				if !strings.Contains(out.String(), path) {
					t.Fatal("missing output confirmation")
				}
			}
			if tc.wantError && out.Len() != 0 {
				t.Fatal("error wrote to stdout")
			}
			if tc.requestBody != "" && calls != 0 {
				t.Fatal("invalid request reached the service")
			}
		})
	}
}

func TestPOSTOutputArguments(t *testing.T) {
	for _, tail := range [][]string{{"--output"}, {"--output", ""}, {"--unknown", "file"}, {"--output", "file", "extra"}} {
		args := append([]string{"post", "audio", "/audio", `{}`}, tail...)
		var out bytes.Buffer
		if err := runCLI(args, nil, &out); err == nil || !strings.Contains(err.Error(), "[--output FILE]") || out.Len() != 0 {
			t.Fatalf("incorrect usage handling for %v: %v", args, err)
		}
	}
}
