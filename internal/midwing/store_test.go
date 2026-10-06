package midwing

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type deletionVault struct {
	Vault
	err error
}

func (v deletionVault) Delete(string) error { return v.err }

func TestConnectionRemovalCredentialErrors(t *testing.T) {
	for _, custom := range []bool{false, true} {
		for _, missing := range []bool{false, true} {
			name := "disconnect"
			if custom {
				name = "delete-service"
			}
			if missing {
				name += "/missing"
			} else {
				name += "/failure"
			}
			t.Run(name, func(t *testing.T) {
				s := testStore(t)
				id, permission := "sample-api", "profile"
				if custom {
					d := tokenDefinition()
					if err := s.CreateCustomService(d); err != nil {
						t.Fatal(err)
					}
					id, permission = d.ID, "endpoint-1"
				}
				if err := s.SaveService(id, "token", []string{permission}); err != nil {
					t.Fatal(err)
				}
				var err error
				deletionErr := errors.New("vault unavailable")
				if missing {
					deletionErr = errCredentialNotFound
				}
				s.Vault = deletionVault{Vault: s.Vault, err: deletionErr}
				if custom {
					err = s.DeleteCustomService(id)
				} else {
					err = s.RemoveService(id)
				}
				if !missing {
					if err == nil {
						t.Fatal("credential deletion failure ignored")
					}
					var committed *CommittedChangeError
					if !errors.As(err, &committed) {
						t.Fatalf("expected committed cleanup warning: %v", err)
					}
					cfg, readErr := s.read()
					if readErr != nil || len(cfg.Connections) != 0 || len(cfg.PendingCredentials) != 1 {
						t.Fatal("cleanup was not queued after disconnect")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				state, err := s.RegistrySnapshot()
				if err != nil || len(state.Connections) != 0 {
					t.Fatal("missing credential blocked removal")
				}
				if custom && len(state.CustomServices) != 0 {
					t.Fatal("deleted service retained")
				}
			})
		}
	}
}

func TestHistoryJSONStream(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    string
		count   int
		invalid bool
	}{
		{name: "empty"},
		{name: "whitespace", data: " \n\t"},
		{name: "records", data: "{\"operation\":\"first\"}\n{\"operation\":\"second\"}\n", count: 2},
		{name: "truncated", data: "{\"operation\":\"first\"}\n{", invalid: true},
		{name: "closing bracket", data: "{\"operation\":\"first\"}\n]", invalid: true},
		{name: "closing brace", data: "{\"operation\":\"first\"}\n}", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			if err := os.WriteFile(filepath.Join(s.Dir, "activity.jsonl"), []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			rows, err := s.History()
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid trailing data accepted")
				}
				return
			}
			if err != nil || rows == nil || len(rows) != tc.count {
				t.Fatalf("history: %v, %v", rows, err)
			}
		})
	}
	t.Run("latest 100", func(t *testing.T) {
		s := testStore(t)
		for i := 0; i < 105; i++ {
			if err := s.Record(Activity{Status: i}); err != nil {
				t.Fatal(err)
			}
		}
		rows, err := s.History()
		if err != nil || len(rows) != 100 || rows[0].Status != 5 || rows[99].Status != 104 {
			t.Fatalf("history: %v, %v", rows, err)
		}
	})
}

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
	if err := s.SaveService("sample-auth", "token", []string{"sheets"}); err == nil {
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

func TestReconnectCredentialCleanup(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "missing"}[missing], func(t *testing.T) {
			s := testStore(t)
			if err := s.SaveService("sample-api", "old", []string{"profile"}); err != nil {
				t.Fatal(err)
			}
			old, err := s.List()
			if err != nil {
				t.Fatal(err)
			}
			cleanupErr := errors.New("vault unavailable")
			if missing {
				cleanupErr = errCredentialNotFound
			}
			s.Vault = deletionVault{Vault: s.Vault, err: cleanupErr}
			err = s.SaveService("sample-api", "new", []string{"items"})
			if missing && err != nil {
				t.Fatal(err)
			}
			if !missing && err == nil {
				t.Fatal("cleanup failure ignored")
			}
			rows, err := s.List()
			if err != nil || len(rows) != 1 || rows[0].CredentialRef == old[0].CredentialRef {
				t.Fatal("replacement connection was not saved")
			}
			token, err := s.Vault.Get(rows[0].CredentialRef)
			if err != nil || token != "new" {
				t.Fatal("replacement credential was not saved")
			}
		})
	}
}

func TestDuplicateConnectionsRejectedBeforeMutation(t *testing.T) {
	s := testStore(t)
	if err := s.SaveService("sample-api", "old", []string{"profile"}); err != nil {
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
	if err := s.SaveService("sample-api", "replacement", []string{"profile"}); err == nil {
		t.Fatal("duplicate connections mutated")
	}
	if len(s.Vault.(memoryVault)) != 1 {
		t.Fatal("invalid settings changed credentials")
	}
}

func TestRemovalWriteFailurePreservesCredentials(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(map[bool]string{false: "disconnect", true: "delete"}[custom], func(t *testing.T) {
			s := testStore(t)
			id, permission := "sample-api", "profile"
			if custom {
				d := tokenDefinition()
				if err := s.CreateCustomService(d); err != nil {
					t.Fatal(err)
				}
				id, permission = d.ID, "endpoint-1"
			}
			if err := s.SaveService(id, "private-token", []string{permission}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(s.Dir, "settings.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(s.Dir, 0500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(s.Dir, 0700) })
			// Skip on systems where this process can bypass directory permissions.
			probe, probeErr := os.CreateTemp(s.Dir, "probe-*")
			if probeErr == nil {
				probe.Close()
				os.Remove(probe.Name())
				t.Skip("directory permissions cannot force a write failure")
			}
			if custom {
				err = s.DeleteCustomService(id)
			} else {
				err = s.RemoveService(id)
			}
			if err == nil {
				t.Fatal("settings write failure ignored")
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil || string(after) != string(before) {
				t.Fatal("settings changed on failed write")
			}
			if len(s.Vault.(memoryVault)) != 1 {
				t.Fatal("credential removed before settings commit")
			}
		})
	}
}

func TestPendingCredentialCleanupRetriesAfterRestart(t *testing.T) {
	s := testStore(t)
	if err := s.SaveService("sample-api", "token", []string{"profile"}); err != nil {
		t.Fatal(err)
	}
	vault := s.Vault
	s.Vault = deletionVault{Vault: vault, err: errors.New("unavailable")}
	var committed *CommittedChangeError
	if err := s.RemoveService("sample-api"); !errors.As(err, &committed) {
		t.Fatalf("missing committed warning: %v", err)
	}
	cfg, err := s.read()
	if err != nil || len(cfg.Connections) != 0 || len(cfg.PendingCredentials) != 1 {
		t.Fatal("cleanup retry was not persisted")
	}
	data, _ := json.Marshal(cfg)
	if strings.Contains(string(data), "token") {
		t.Fatal("cleanup persisted credential value")
	}
	restarted := &Store{Dir: s.Dir, Vault: vault}
	if err := restarted.RemoveService("sample-api"); err != nil {
		t.Fatal(err)
	}
	cfg, err = restarted.read()
	if err != nil || len(cfg.PendingCredentials) != 0 || len(vault.(memoryVault)) != 0 {
		t.Fatal("cleanup was not retried")
	}
}

func TestRefreshRejectsDefinitionChange(t *testing.T) {
	s := testStore(t)
	d := tokenDefinition()
	if err := s.CreateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveService(d.ID, "old-token", []string{"endpoint-1"}); err != nil {
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
