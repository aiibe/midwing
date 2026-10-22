package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"
	"midwing/internal/clisetup"
)

func cliSetupFixture(t *testing.T) (clisetup.Manager, string) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, "Applications")
	executable := filepath.Join(root, "Midwing.app", "Contents", "MacOS", "midwing")
	if err := os.MkdirAll(filepath.Dir(executable), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("app"), 0755); err != nil {
		t.Fatal(err)
	}
	m := clisetup.Manager{Home: home, Executable: executable, Roots: []string{root},
		VerifyBundle: func(string) bool { return true }}
	record := filepath.Join(home, "Library", "Application Support", "midwing", "cli-setup.json")
	if err := os.MkdirAll(filepath.Dir(record), 0700); err != nil {
		t.Fatal(err)
	}
	return m, record
}

func TestCLISetupFailureSurvivesRefreshUntilSuccessfulRetry(t *testing.T) {
	m, record := cliSetupFixture(t)
	if err := os.WriteFile(record, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	var state cliSetupState
	if _, err := state.change(m, "ensure"); err == nil {
		t.Fatal("expected startup failure")
	}
	if s, err := state.status(m); err != nil || !strings.Contains(s.SetupError, record) {
		t.Fatalf("preferences error has no recovery path: %+v %v", s, err)
	}
	// A status refresh must not erase the failure after its cause disappears.
	if err := os.Remove(record); err != nil {
		t.Fatal(err)
	}
	s, err := state.status(m)
	if err != nil || !strings.Contains(s.SetupError, "invalid") {
		t.Fatalf("startup failure lost: %+v %v", s, err)
	}
	s, err = state.change(m, "ensure")
	if err != nil || !s.Installed || s.SetupError != "" {
		t.Fatalf("retry: %+v %v", s, err)
	}
	s, err = state.status(m)
	if err != nil || s.SetupError != "" {
		t.Fatalf("failure not cleared: %+v %v", s, err)
	}
}

func TestBusyStartupDoesNotClearPreviousFailure(t *testing.T) {
	m, record := cliSetupFixture(t)
	lock := flock.New(record + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer lock.Unlock()
	state := cliSetupState{failure: "previous setup failed"}
	if _, err := state.run(m.TryEnsure); !errors.Is(err, clisetup.ErrBusy) {
		t.Fatalf("expected busy setup, got %v", err)
	}
	s, err := state.status(m)
	if err != nil || s.SetupError != "previous setup failed" {
		t.Fatalf("failure cleared without setup: %+v %v", s, err)
	}
}
