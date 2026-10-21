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

func TestOutputFile(t *testing.T) {
	for _, method := range []string{"get", "post"} {
		store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
		d := midwing.CustomService{
			ID: "audio", Name: "Audio", BaseURL: "https://audio.example.test", AuthKind: "token", Header: "X-API-Key",
			Endpoints: []midwing.CustomEndpoint{{Path: "/audio", Methods: []string{"GET", "POST"}}},
		}
		if err := store.CreateCustomService(d); err != nil {
			t.Fatal(err)
		}
		if err := saveTestService(store, d.ID, "secret-token", []string{"endpoint-1", "endpoint-1-post"}); err != nil {
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
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				if method == "get" && tc.requestBody != "" {
					t.Skip("GET has no request body")
				}
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
				args := []string{method, d.ID, "/audio"}
				if method == "post" {
					args = append(args, body)
				}
				args = append(args, "--output", path)
				err := runCLI(args, store, &out)
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

func TestOutputOptionsAndEmptyResponses(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
	d := midwing.CustomService{ID: "files", Name: "Files", BaseURL: "https://files.example.test", AuthKind: "token", Header: "X-API-Key", Endpoints: []midwing.CustomEndpoint{{Path: "/file", Methods: []string{"GET", "POST"}}}}
	if err := store.CreateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := saveTestService(store, d.ID, "secret-token", []string{"endpoint-1", "endpoint-1-post"}); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"get", "post"} {
		for _, download := range []bool{false, true} {
			args := []string{method, d.ID, "/file"}
			if method == "post" {
				args = append(args, `{}`)
			}
			path := filepath.Join(t.TempDir(), "file")
			if download {
				args = append(args, "--max-bytes", "4", "--accept", "audio/wav", "--output", path)
			}
			http.DefaultTransport = requestTransport(func(r *http.Request) (*http.Response, error) {
				status, body := 204, ""
				if download {
					if r.Header.Get("Accept") != "audio/wav" {
						t.Fatal("missing media type")
					}
					status, body = 200, "wave"
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			var out bytes.Buffer
			if err := runCLI(args, store, &out); err != nil {
				t.Fatal(err)
			}
			if download {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "wave" {
					t.Fatalf("incorrect download: %v", err)
				}
			} else if out.Len() != 0 {
				t.Fatal("empty response printed output")
			}
		}
		for _, tail := range [][]string{{"--accept", "audio/wav"}, {"--max-bytes", "12"}, {"--output", ""}, {"--output", "file", "--max-bytes", "0"}, {"--output", "file", "--max-bytes", "-1"}, {"--output", "file", "--max-bytes", "invalid"}, {"--output", "file", "--max-bytes", "9223372036854775807"}} {
			args := []string{method, d.ID, "/file"}
			if method == "post" {
				args = append(args, `{}`)
			}
			var out bytes.Buffer
			if err := runCLI(append(args, tail...), nil, &out); err == nil || !strings.Contains(err.Error(), "usage:") || out.Len() != 0 {
				t.Fatalf("accepted invalid options: %v (%v)", tail, err)
			}
		}
	}
}
