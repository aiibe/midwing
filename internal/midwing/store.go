package midwing

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/zalando/go-keyring"
)

type Settings struct {
	PendingCredentials []string        `json:"pendingCredentials,omitempty"`
	CustomServices     []CustomService `json:"customServices,omitempty"`
	Version            int             `json:"version"`
	Connections        []Connection    `json:"connections"`
}

type Vault interface {
	Set(string, string) error
	Get(string) (string, error)
	Delete(string) error
}

var errCredentialNotFound = keyring.ErrNotFound

type OSVault struct{}

func (OSVault) Set(ref, token string) error { return keyring.Set("midwing", ref, token) }

func (OSVault) Get(ref string) (string, error) { return keyring.Get("midwing", ref) }

func (OSVault) Delete(ref string) error { return keyring.Delete("midwing", ref) }

type Store struct {
	Dir   string
	Vault Vault
}

func NewStore() (*Store, error) {
	d, e := os.UserConfigDir()
	if e != nil {
		return nil, e
	}
	return &Store{Dir: filepath.Join(d, "midwing"), Vault: OSVault{}}, nil
}

func (s *Store) locked(fn func() error) error {
	if e := os.MkdirAll(s.Dir, 0700); e != nil {
		return e
	}
	l := flock.New(filepath.Join(s.Dir, "store.lock"))
	if e := l.Lock(); e != nil {
		return e
	}
	defer l.Unlock()
	return fn()
}

func (s *Store) read() (Settings, error) {
	cfg := Settings{Version: 1, Connections: []Connection{}}
	b, e := os.ReadFile(filepath.Join(s.Dir, "settings.json"))
	if errors.Is(e, os.ErrNotExist) {
		return cfg, nil
	}
	if e != nil {
		return cfg, e
	}
	value := &cfg
	e = json.Unmarshal(b, &value)
	if e == nil && value == nil {
		return cfg, errors.New("settings must be a JSON object")
	}
	if e == nil && cfg.Version != 1 {
		e = errors.New("unsupported settings version")
	}
	if e == nil {
		seen := map[string]bool{}
		for _, conn := range cfg.Connections {
			if seen[conn.Service] {
				return cfg, errors.New("duplicate connection service in settings")
			}
			seen[conn.Service] = true
		}
	}
	return cfg, e
}

func (s *Store) write(cfg Settings) error {
	b, e := json.MarshalIndent(cfg, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.Dir, "settings-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), filepath.Join(s.Dir, "settings.json"))
}
