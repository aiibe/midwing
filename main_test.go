package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"midwing/internal/midwing"
	"testing"
)

func TestCLI(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir()}
	var out bytes.Buffer
	if e := runCLI([]string{"services"}, store, &out); e != nil {
		t.Fatal(e)
	}
	var rows []serviceSummary
	if e := json.Unmarshal(out.Bytes(), &rows); e != nil || rows == nil || len(rows) != 0 {
		t.Fatalf("invalid services JSON: %s", out.String())
	}
	for _, args := range [][]string{{"get", "sample-api", "https://evil.test/user"}, {"get", "sample-api", "/unsupported"}, {"get", "sample-api", "/user"}, {"capabilities"}, {"connections", "list"}, {"api", "sample-api", "GET", "/user"}, {"unknown"}} {
		out.Reset()
		if e := runCLI(args, store, &out); e == nil || out.Len() != 0 {
			t.Fatalf("expected silent stdout on error for %v", args)
		}
	}
	out.Reset()
	if e := runCLI([]string{"--help"}, store, &out); e != nil || out.Len() == 0 {
		t.Fatal("missing help")
	}
}

func TestDesktopLoginRequiresActiveContext(t *testing.T) {
	app := &App{}
	if _, err := app.ConnectPassword("service", "user@example.com", "password", []string{"read"}); err == nil {
		t.Fatal("login accepted before startup")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	app.ctx = ctx
	if _, err := app.ConnectPassword("service", "user@example.com", "password", []string{"read"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("login ignored app cancellation: %v", err)
	}
}

func TestCommittedChangesReturnStructuredWarnings(t *testing.T) {
	result, err := changeResult(&midwing.CommittedChangeError{})
	if err != nil || result.Warning == "" {
		t.Fatal("committed change reported as failure")
	}
	failure := errors.New("settings unavailable")
	result, err = changeResult(failure)
	if !errors.Is(err, failure) || result.Warning != "" {
		t.Fatal("uncommitted failure reported as success")
	}
}
