package midwing

import (
	"errors"
	"strings"
)

func definitions(cfg Settings) ([]Service, error) {
	out := append([]Service{}, services...)
	seen := map[string]bool{}
	for _, s := range out {
		seen[s.ID] = true
	}
	for _, d := range cfg.CustomServices {
		if seen[d.ID] {
			return nil, errors.New("duplicate or reserved custom service ID in settings")
		}
		s, e := d.compile()
		if e != nil {
			return nil, e
		}
		out = append(out, s)
		seen[d.ID] = true
	}
	return out, nil
}

func resolve(cfg Settings, id string) (Service, error) {
	all, e := definitions(cfg)
	if e != nil {
		return Service{}, e
	}
	return findService(all, id)
}

func findService(all []Service, id string) (Service, error) {
	for _, s := range all {
		if s.ID == id {
			return s, nil
		}
	}
	return Service{}, errors.New("unsupported service")
}

func (s *Store) Services() ([]Service, error) {
	var out []Service
	e := s.locked(func() error {
		cfg, e := s.read()
		if e != nil {
			return e
		}
		out, e = definitions(cfg)
		return e
	})
	return out, e
}

func (s *Store) Service(id string) (Service, error) {
	var out Service
	e := s.locked(func() error {
		cfg, e := s.read()
		if e != nil {
			return e
		}
		out, e = resolve(cfg, id)
		return e
	})
	return out, e
}

// InvalidServiceDefinitionError distinguishes validation from persistence errors.
type InvalidServiceDefinitionError struct{ Err error }

func (e *InvalidServiceDefinitionError) Error() string { return e.Err.Error() }

func (e *InvalidServiceDefinitionError) Unwrap() error { return e.Err }

func (s *Store) CreateCustomService(d CustomService) error {
	if _, e := d.compile(); e != nil {
		return &InvalidServiceDefinitionError{Err: e}
	}
	return s.locked(func() error {
		cfg, e := s.read()
		if e != nil {
			return e
		}
		all, e := definitions(cfg)
		if e != nil {
			return e
		}
		for _, known := range all {
			if known.ID == d.ID {
				return errors.New("service ID already exists; choose another ID")
			}
		}
		cfg.CustomServices = append(cfg.CustomServices, d)
		return s.write(cfg)
	})
}

func (s *Store) DeleteCustomService(id string) error {
	return s.locked(func() error {
		cfg, e := s.read()
		if e != nil {
			return e
		}
		index := -1
		for i, d := range cfg.CustomServices {
			if d.ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			return errors.New("custom service not found")
		}
		removeConnections(&cfg, id)
		cfg.CustomServices = append(cfg.CustomServices[:index], cfg.CustomServices[index+1:]...)
		if e = s.write(cfg); e != nil {
			return e
		}
		return s.finishCredentialCleanup(cfg)
	})
}

// UpdateCustomService accepts endpoint and static header changes. Request
// behavior changes invalidate the connection hash until the user reconnects.
func (s *Store) UpdateCustomService(d CustomService) error {
	if _, err := d.compile(); err != nil {
		return &InvalidServiceDefinitionError{Err: err}
	}
	return s.locked(func() error {
		cfg, err := s.read()
		if err != nil {
			return err
		}
		for i := range cfg.CustomServices {
			if cfg.CustomServices[i].ID != d.ID {
				continue
			}
			previous, updated := cfg.CustomServices[i], d
			previous.Headers, updated.Headers = nil, nil
			previous.Endpoints, updated.Endpoints = nil, nil
			if previous.definitionHash() != updated.definitionHash() {
				return errors.New("update only supports endpoint and static header changes; keep all other definition fields unchanged")
			}
			cfg.CustomServices[i] = d
			return s.write(cfg)
		}
		return errors.New("custom service not found")
	})
}

// UpdatePermissionDescriptions only changes display metadata. Paths identify the
// endpoints so a stale editor cannot overwrite another endpoint's description.
func (s *Store) UpdatePermissionDescriptions(id string, descriptions map[string]string) error {
	return s.locked(func() error {
		cfg, err := s.read()
		if err != nil {
			return err
		}
		for i := range cfg.CustomServices {
			d := &cfg.CustomServices[i]
			if d.ID != id {
				continue
			}
			remaining := len(descriptions)
			for j := range d.Endpoints {
				if description, ok := descriptions[d.Endpoints[j].Path]; ok {
					d.Endpoints[j].Description = strings.TrimSpace(description)
					remaining--
				}
			}
			if remaining != 0 {
				return errors.New("endpoints changed; reload the service and try again")
			}
			if _, err := d.compile(); err != nil {
				return err
			}
			return s.write(cfg)
		}
		return errors.New("custom service not found")
	})
}
