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
				if err := saveTestService(s, id, "token", []string{permission}); err != nil {
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

func TestReconnectCredentialCleanup(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "missing"}[missing], func(t *testing.T) {
			s := testStore(t)
			if err := saveTestService(s, "sample-api", "old", []string{"profile"}); err != nil {
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
			err = saveTestService(s, "sample-api", "new", []string{"items"})
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
			if err := saveTestService(s, id, "private-token", []string{permission}); err != nil {
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
	if err := saveTestService(s, "sample-api", "token", []string{"profile"}); err != nil {
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
