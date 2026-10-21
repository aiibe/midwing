package midwing

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
)

type Connection struct {
	DefinitionHash string   `json:"definitionHash,omitempty"`
	Service        string   `json:"service"`
	CredentialRef  string   `json:"credentialRef"`
	Permissions    []string `json:"permissions"`
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

// SaveServiceRevision saves token approvals for the definition reviewed by the user.
func (s *Store) SaveServiceRevision(service, revision, token string, permissions []string) error {
	definition, err := s.serviceAtRevision(service, revision)
	if err != nil {
		return err
	}
	if definition.AuthKind != "token" {
		return errors.New("service requires email/password login")
	}
	return s.saveToken(service, token, permissions, &definition)
}

func (s *Store) serviceAtRevision(service, revision string) (Service, error) {
	definition, err := s.Service(service)
	if err != nil {
		return Service{}, err
	}
	if revision != definition.DefinitionHash {
		return Service{}, errors.New("service changed; reload and review permissions before reconnecting")
	}
	return definition, nil
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
			return errors.New("service changed; reload and review permissions before reconnecting")
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
