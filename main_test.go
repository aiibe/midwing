package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"midwing/internal/midwing"
	"strings"
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
	if _, err := app.ConnectPassword("service", "revision", "user@example.com", "password", []string{"read"}); err == nil {
		t.Fatal("login accepted before startup")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	app.ctx = ctx
	if _, err := app.ConnectPassword("service", "revision", "user@example.com", "password", []string{"read"}); !errors.Is(err, context.Canceled) {
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

func TestDesktopConnectionApprovalsRequireReviewedRevision(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
	var definition midwing.CustomService
	if err := json.Unmarshal([]byte(serviceJSON), &definition); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateCustomService(definition); err != nil {
		t.Fatal(err)
	}
	app := &App{store: store, ctx: context.Background()}
	reviewed, err := app.State()
	if err != nil || len(reviewed.Services) != 1 {
		t.Fatalf("could not read reviewed service: %v", err)
	}
	revision := reviewed.Services[0].Revision
	definition.Endpoints[0].Path = "/admin/export"
	if err := store.UpdateCustomService(definition); err != nil {
		t.Fatal(err)
	}
	for _, revision := range []string{revision, ""} {
		if _, err := app.SaveConnection(definition.ID, revision, "secret-token", []string{"endpoint-1"}); err == nil || !strings.Contains(err.Error(), "review permissions") {
			t.Fatalf("desktop token approval ignored revision: %v", err)
		}
		// Revision validation must also precede authentication protocol selection.
		if _, err := app.ConnectPassword(definition.ID, revision, "me@example.com", "password", []string{"endpoint-1"}); err == nil || !strings.Contains(err.Error(), "review permissions") {
			t.Fatalf("desktop password approval ignored revision: %v", err)
		}
	}
	if len(store.Vault.(cliVault)) != 0 {
		t.Fatal("stale desktop approval saved credentials")
	}
	updated, err := app.State()
	if err != nil || len(updated.Connections) != 0 {
		t.Fatalf("stale desktop approval created a connection: %v", err)
	}
	if _, err := app.SaveConnection(definition.ID, updated.Services[0].Revision, "secret-token", []string{"endpoint-1"}); err != nil {
		t.Fatalf("fresh desktop approval failed: %v", err)
	}
}
