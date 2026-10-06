package midwing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSettingsWatcherAtomicWritesAndShutdown(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "settings")}
	changed := make(chan struct{}, 20)
	stop, err := s.WatchSettings(context.Background(), func() { changed <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	// Ignore unrelated activity and temporary files.
	if err := os.WriteFile(filepath.Join(s.Dir, "activity.jsonl"), []byte("record"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
		t.Fatal("unrelated event emitted")
	case <-time.After(250 * time.Millisecond):
	}
	definition := CustomService{ID: "watched", Name: "Watched", BaseURL: "https://example.com", AuthKind: "token", Header: "Authorization", Endpoints: []CustomEndpoint{{Path: "/items"}}}
	if err := s.CreateCustomService(definition); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case <-time.After(3 * time.Second):
		t.Fatal("atomic creation not detected")
	}
	state, err := s.Snapshot()
	if err != nil || len(state.Services) != 1 {
		t.Fatalf("CLI change not loaded: %v", err)
	}
	// A burst of atomic replacements produces a single debounced notification.
	for i := 0; i < 5; i++ {
		if err := s.locked(func() error {
			cfg, err := s.read()
			if err != nil {
				return err
			}
			return s.write(cfg)
		}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-changed:
	case <-time.After(3 * time.Second):
		t.Fatal("replacement not detected")
	}
	select {
	case <-changed:
		t.Fatal("burst was not debounced")
	case <-time.After(250 * time.Millisecond):
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "settings.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case <-time.After(3 * time.Second):
		t.Fatal("invalid change not detected")
	}
	if _, err := s.Snapshot(); err == nil {
		t.Fatal("invalid settings accepted")
	}
	stop()
	if err := os.WriteFile(filepath.Join(s.Dir, "settings.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
		t.Fatal("notification after shutdown")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestSnapshotNoCredentialsAndDefinitionValidation(t *testing.T) {
	s := testStore(t)
	if err := s.SaveService("sample-api", "never-export-this", []string{"profile"}); err != nil {
		t.Fatal(err)
	}
	s.Vault = inaccessibleVault{}
	state, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(state)
	if strings.Contains(string(data), "never-export-this") || len(state.Connections) != 1 || strings.Contains(string(data), "credentialRef") {
		t.Fatal("snapshot exposed credential reference")
	}
	cfg, err := s.read()
	if err != nil {
		t.Fatal(err)
	}
	cfg.CustomServices = []CustomService{{ID: "invalid"}}
	if err := s.write(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Snapshot(); err == nil {
		t.Fatal("invalid definition accepted")
	}
}

func TestRegistrySnapshotFiltersStaleAndUnknownConnections(t *testing.T) {
	s := testStore(t)
	if err := s.SaveService("sample-api", "private-token", []string{"profile"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.read()
	if err != nil {
		t.Fatal(err)
	}
	d := tokenDefinition()
	cfg.CustomServices = append(cfg.CustomServices, d)
	cfg.Connections = append(cfg.Connections,
		Connection{Service: d.ID, DefinitionHash: "stale", CredentialRef: "stale-ref"},
		Connection{Service: "unknown", CredentialRef: "unknown-ref"},
	)
	if err := s.write(cfg); err != nil {
		t.Fatal(err)
	}
	s.Vault = inaccessibleVault{}
	state, err := s.RegistrySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Services) == 0 || len(state.Connections) != 1 || state.Connections[0].CredentialRef != cfg.Connections[0].CredentialRef {
		t.Fatalf("unexpected registry snapshot: %+v", state)
	}
	cfg.CustomServices = []CustomService{{ID: "invalid"}}
	if err := s.write(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RegistrySnapshot(); err == nil {
		t.Fatal("invalid definition accepted")
	}
}
