package midwing

import (
	"context"
	"github.com/fsnotify/fsnotify"
	"os"
	"path/filepath"
	"time"
)

// WatchSettings watches the directory because settings are replaced atomically.
// Notifications carry no data; callers reload through the validated store.
func (s *Store) WatchSettings(ctx context.Context, changed func()) (func(), error) {
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return nil, err
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err = watcher.Add(s.Dir); err != nil {
		watcher.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer watcher.Close()
		var timer *time.Timer
		var tick <-chan time.Time
		defer func() {
			if timer != nil {
				timer.Stop()
			}
		}()
		schedule := func() {
			if timer == nil {
				timer = time.NewTimer(150 * time.Millisecond)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(150 * time.Millisecond)
			}
			tick = timer.C
		}
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) == "settings.json" && event.Has(fsnotify.Create|fsnotify.Write|fsnotify.Rename|fsnotify.Remove) {
					schedule()
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
				// Overflow can lose events: reload; focus is an additional fallback.
				schedule()
			case <-tick:
				tick = nil
				if ctx.Err() == nil {
					changed()
				}
			}
		}
	}()
	return func() { cancel(); <-done }, nil
}

type DesktopConnection struct {
	Service     string   `json:"service"`
	Permissions []string `json:"permissions"`
}

type DesktopService struct {
	Service
	Revision string `json:"revision"`
}

type DesktopState struct {
	Services    []DesktopService    `json:"services"`
	Connections []DesktopConnection `json:"connections"`
}

// Snapshot reads and validates one consistent UI state without accessing the vault.
func (s *Store) Snapshot() (DesktopState, error) {
	registry, err := s.RegistrySnapshot()
	if err != nil {
		return DesktopState{}, err
	}
	state := DesktopState{Services: []DesktopService{}, Connections: []DesktopConnection{}}
	for _, service := range registry.Services {
		state.Services = append(state.Services, DesktopService{service, service.DefinitionHash})
	}
	for _, conn := range registry.Connections {
		state.Connections = append(state.Connections, DesktopConnection{Service: conn.Service, Permissions: conn.Permissions})
	}
	return state, nil
}
