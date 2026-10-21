package midwing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsRequireObject(t *testing.T) {
	for _, data := range []string{"null", "[]", "true", "\"text\""} {
		t.Run(data, func(t *testing.T) {
			s := testStore(t)
			if err := os.WriteFile(filepath.Join(s.Dir, "settings.json"), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.RegistrySnapshot(); err == nil {
				t.Fatal("non-object settings accepted")
			}
		})
	}
	s := testStore(t)
	cfg, err := s.read()
	if err != nil || cfg.Version != 1 || cfg.Connections == nil {
		t.Fatalf("missing file defaults: %+v, %v", cfg, err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "settings.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = s.read()
	if err != nil || cfg.Version != 1 || cfg.Connections == nil {
		t.Fatalf("object defaults: %+v, %v", cfg, err)
	}
}

func TestTokenSaveRejectsPasswordService(t *testing.T) {
	registerPasswordFixture(t)
	s := testStore(t)
	if err := saveTestService(s, "sample-auth", "token", []string{"sheets"}); err == nil {
		t.Fatal("password service accepted a direct token")
	}
	if len(s.Vault.(memoryVault)) != 0 {
		t.Fatal("invalid token save wrote a credential")
	}
	rows, err := s.List()
	if err != nil || len(rows) != 0 {
		t.Fatal("invalid token save wrote a connection")
	}
}

func TestDuplicateConnectionsRejectedBeforeMutation(t *testing.T) {
	s := testStore(t)
	if err := saveTestService(s, "sample-api", "old", []string{"profile"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.read()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Connections = append(cfg.Connections, cfg.Connections[0])
	if err := s.write(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RegistrySnapshot(); err == nil {
		t.Fatal("duplicate connections accepted")
	}
	if err := saveTestService(s, "sample-api", "replacement", []string{"profile"}); err == nil {
		t.Fatal("duplicate connections mutated")
	}
	if len(s.Vault.(memoryVault)) != 1 {
		t.Fatal("invalid settings changed credentials")
	}
}

func TestRefreshRejectsDefinitionChange(t *testing.T) {
	s := testStore(t)
	d := tokenDefinition()
	if err := s.CreateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := saveTestService(s, d.ID, "old-token", []string{"endpoint-1"}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := s.read()
	if err != nil {
		t.Fatal(err)
	}
	cfg.CustomServices[0].BaseURL = "https://changed.test"
	if err := s.write(cfg); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateToken(rows[0], "old-token", "new-token"); err == nil {
		t.Fatal("stale refresh persisted")
	}
	token, err := s.Vault.Get(rows[0].CredentialRef)
	if err != nil || token != "old-token" {
		t.Fatal("stale refresh changed credential")
	}
}
