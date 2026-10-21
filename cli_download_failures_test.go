package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"midwing/internal/midwing"
)

func TestOutputFileFailures(t *testing.T) {
	for _, failure := range []string{"write", "close", "cleanup"} {
		t.Run(failure, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "output")
			err := writeResponseFile(path, func(file *os.File) error {
				if failure == "cleanup" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(path, "child"), nil, 0600); err != nil {
						t.Fatal(err)
					}
					return errors.New("request failed")
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
				if failure == "write" {
					_, err := file.Write([]byte("data"))
					return err
				}
				return file.Close()
			})
			if err == nil {
				t.Fatal("missing output error")
			}
			if failure == "cleanup" {
				if !strings.Contains(err.Error(), "request failed") || !strings.Contains(err.Error(), "could not remove failed output file") {
					t.Fatalf("cleanup error lost: %v", err)
				}
			} else if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("failed file remains")
			}
		})
	}
}

func TestDownloadActivityFailureRemovesOutput(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
	d := midwing.CustomService{ID: "files", Name: "Files", BaseURL: "https://files.example.test", AuthKind: "token", Header: "X-API-Key", Endpoints: []midwing.CustomEndpoint{{Path: "/file"}}}
	if err := store.CreateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveService(d.ID, "secret-token", []string{"endpoint-1"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(store.Dir, "activity.jsonl"), 0700); err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	http.DefaultTransport = requestTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("safe"))}, nil
	})
	path := filepath.Join(t.TempDir(), "download")
	var out bytes.Buffer
	err := runRequestCLI([]string{"get", d.ID, "/file", "--output", path}, store, &out)
	if err == nil || !strings.Contains(err.Error(), "could not record activity") || out.Len() != 0 {
		t.Fatalf("unexpected output or error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("activity failure left an output file")
	}
}
