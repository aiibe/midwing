package midwing

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofrs/flock"
	"github.com/zalando/go-keyring"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Connection struct {
	DefinitionHash string   `json:"definitionHash,omitempty"`
	Service        string   `json:"service"`
	CredentialRef  string   `json:"credentialRef"`
	Permissions    []string `json:"permissions"`
}
type Settings struct {
	PendingCredentials []string        `json:"pendingCredentials,omitempty"`
	CustomServices     []CustomService `json:"customServices,omitempty"`
	Version            int             `json:"version"`
	Connections        []Connection    `json:"connections"`
}
type Activity struct {
	Time      time.Time `json:"time"`
	Operation string    `json:"operation"`
	Outcome   string    `json:"outcome"`
	Status    int       `json:"status,omitempty"`
}
type Vault interface {
	Set(string, string) error
	Get(string) (string, error)
	Delete(string) error
}

var errCredentialNotFound = keyring.ErrNotFound

type OSVault struct{}

func (OSVault) Set(ref, token string) error    { return keyring.Set("midwing", ref, token) }
func (OSVault) Get(ref string) (string, error) { return keyring.Get("midwing", ref) }
func (OSVault) Delete(ref string) error        { return keyring.Delete("midwing", ref) }

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

// Matches binds a connection to the current service definition.
func (c Connection) Matches(service Service) bool {
	return c.Service == service.ID && c.DefinitionHash == service.DefinitionHash
}

// RegistryState contains definitions and valid connections from one settings read.
// It contains credential references, but never reads credentials from the vault.
type RegistryState struct {
	Services       []Service
	Connections    []Connection
	CustomServices []CustomService
}

func (s *Store) RegistrySnapshot() (RegistryState, error) {
	var state RegistryState
	err := s.locked(func() error {
		cfg, err := s.read()
		if err != nil {
			return err
		}
		state.Services, err = definitions(cfg)
		if err != nil {
			return err
		}
		state.CustomServices = cfg.CustomServices
		state.Connections = validConnections(cfg.Connections, state.Services)
		return nil
	})
	if err != nil {
		return RegistryState{}, err
	}
	return state, nil
}

func validConnections(connections []Connection, services []Service) []Connection {
	byID := make(map[string]Service, len(services))
	for _, service := range services {
		byID[service.ID] = service
	}
	out := []Connection{}
	for _, conn := range connections {
		if service, ok := byID[conn.Service]; ok && conn.Matches(service) {
			out = append(out, conn)
		}
	}
	return out
}

func (s *Store) List() ([]Connection, error) {
	state, err := s.RegistrySnapshot()
	return state.Connections, err
}

func (s *Store) SaveService(service, token string, permissions []string) error {
	return s.saveToken(service, token, permissions, nil)
}
func (s *Store) saveToken(service, token string, permissions []string, expected *Service) error {
	if strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n") {
		return errors.New("invalid credential")
	}
	return s.locked(func() error {
		cfg, e := s.read()
		if e != nil {
			return e
		}
		definition, e := resolve(cfg, service)
		if e != nil {
			return e
		}
		if expected == nil && definition.AuthKind != "token" {
			return errors.New("service requires email/password login")
		}
		if expected != nil && (definition.DefinitionHash != expected.DefinitionHash || definition.Origin != expected.Origin) {
			return errors.New("service changed during login; reconnect")
		}
		if e = validatePermissions(definition, permissions); e != nil {
			return e
		}
		id := make([]byte, 16)
		if _, e = rand.Read(id); e != nil {
			return e
		}
		ref := service + "-" + hex.EncodeToString(id)
		if e = s.Vault.Set(ref, token); e != nil {
			return errors.New("could not save token in OS credential store")
		}
		old := ""
		index := -1
		for i, c := range cfg.Connections {
			if c.Service == service {
				old = c.CredentialRef
				index = i
				break
			}
		}
		conn := Connection{DefinitionHash: definition.DefinitionHash, Service: service, CredentialRef: ref, Permissions: permissions}
		if index < 0 {
			cfg.Connections = append(cfg.Connections, conn)
		} else {
			cfg.Connections[index] = conn
		}
		if old != "" {
			cfg.PendingCredentials = append(cfg.PendingCredentials, old)
		}
		if e = s.write(cfg); e != nil {
			s.Vault.Delete(ref)
			return e
		}
		return s.finishCredentialCleanup(cfg)
	})
}
func (s *Store) RemoveService(service string) error {
	return s.locked(func() error {
		cfg, e := s.read()
		if e != nil {
			return e
		}
		if _, e = resolve(cfg, service); e != nil {
			return e
		}
		removeConnections(&cfg, service)
		if e = s.write(cfg); e != nil {
			return e
		}
		return s.finishCredentialCleanup(cfg)
	})
}

// ChangeResult reports a committed change even when credential cleanup needs retrying.
type ChangeResult struct {
	Warning string `json:"warning,omitempty"`
}

type CommittedChangeError struct{}

func (*CommittedChangeError) Error() string {
	return "change saved, but credential cleanup is pending; it will retry on the next connection save, disconnect, or service deletion"
}

// Queue cleanup in the same atomic write that removes the connection. A failed
// settings write leaves credentials intact; a vault failure leaves a retry record.
func removeConnections(cfg *Settings, service string) {
	remaining := []Connection{}
	for _, conn := range cfg.Connections {
		if conn.Service == service {
			cfg.PendingCredentials = append(cfg.PendingCredentials, conn.CredentialRef)
		} else {
			remaining = append(remaining, conn)
		}
	}
	cfg.Connections = remaining
}

// Runs under the settings lock after committing the user-visible change.
func (s *Store) finishCredentialCleanup(cfg Settings) error {
	if len(cfg.PendingCredentials) == 0 {
		return nil
	}
	for _, ref := range cfg.PendingCredentials {
		// Never delete a credential that an edited settings file made active again.
		for _, conn := range cfg.Connections {
			if conn.CredentialRef == ref {
				return &CommittedChangeError{}
			}
		}
		if err := s.Vault.Delete(ref); err != nil && !errors.Is(err, errCredentialNotFound) {
			return &CommittedChangeError{}
		}
	}
	cfg.PendingCredentials = nil
	if err := s.write(cfg); err != nil {
		return &CommittedChangeError{}
	}
	return nil
}

// UpdateToken does not recreate a disconnected or replaced connection. Concurrent
// refreshes must not overwrite a newer credential with a stale refresh response.
func (s *Store) UpdateToken(conn Connection, previous, token string) error {
	return s.locked(func() error {
		cfg, e := s.read()
		if e != nil {
			return e
		}
		definition, e := resolve(cfg, conn.Service)
		if e != nil {
			return e
		}
		if !conn.Matches(definition) {
			return errors.New("service changed during refresh; reconnect")
		}
		for _, current := range cfg.Connections {
			if current.Matches(definition) && current.CredentialRef == conn.CredentialRef {
				existing, e := s.Vault.Get(current.CredentialRef)
				if e != nil {
					return errors.New("could not retrieve credential")
				}
				if existing != previous {
					return errors.New("connection changed; retry the command")
				}
				if e = s.Vault.Set(current.CredentialRef, token); e != nil {
					return errors.New("could not save refreshed credential")
				}
				return nil
			}
		}
		return errors.New("connection changed; reconnect or retry")
	})
}
func (s *Store) Record(a Activity) error {
	return s.locked(func() error {
		f, e := os.OpenFile(filepath.Join(s.Dir, "activity.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		defer f.Close()
		return json.NewEncoder(f).Encode(a)
	})
}
func (s *Store) History() ([]Activity, error) {
	out := []Activity{}
	e := s.locked(func() error {
		f, e := os.Open(filepath.Join(s.Dir, "activity.jsonl"))
		if errors.Is(e, os.ErrNotExist) {
			return nil
		}
		if e != nil {
			return e
		}
		defer f.Close()
		d := json.NewDecoder(f)
		for {
			var a Activity
			if e = d.Decode(&a); errors.Is(e, io.EOF) {
				return nil
			} else if e != nil {
				return fmt.Errorf("invalid activity history: %w", e)
			}
			out = append(out, a)
			if len(out) > 100 {
				out = out[1:]
			}
		}
	})
	return out, e
}
