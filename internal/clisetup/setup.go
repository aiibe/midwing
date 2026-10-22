// Package clisetup manages the per-user CLI link for the installed app.
package clisetup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
)

type Status struct {
	Available         bool   `json:"available"`
	Installed         bool   `json:"installed"`
	AutoSetupDisabled bool   `json:"autoSetupDisabled"`
	SetupError        string `json:"setupError"`
	Destination       string `json:"destination"`
	Target            string `json:"target"`
	ExistingTarget    string `json:"existingTarget"`
	Message           string `json:"message"`
	PathCommand       string `json:"pathCommand"`
}

type Manager struct {
	Home       string
	Executable string
	// Roots and VerifyBundle are supplied by the platform adapter.
	Roots        []string
	VerifyBundle func(string) bool
	Unavailable  string
}

func (m Manager) destination() string { return filepath.Join(m.Home, ".local", "bin", "midwing") }
func (m Manager) record() string {
	return filepath.Join(m.Home, "Library", "Application Support", "midwing", "cli-setup.json")
}

func (m Manager) target() (string, error) {
	if m.Unavailable != "" {
		return "", errors.New(m.Unavailable)
	}
	executable, err := filepath.EvalSymlinks(m.Executable)
	if err != nil {
		return "", err
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	if executable != filepath.Join(bundle, "Contents", "MacOS", "midwing") || !strings.HasSuffix(bundle, ".app") {
		return "", errors.New("CLI setup requires the installed Midwing app")
	}
	for _, root := range m.Roots {
		realRoot, err := filepath.EvalSymlinks(root)
		if err == nil && filepath.Dir(bundle) == realRoot && m.VerifyBundle(bundle) {
			return executable, nil
		}
	}
	return "", errors.New("Drag Midwing to /Applications or ~/Applications, eject the disk image, and launch the installed copy to set up the CLI")
}

// ErrBusy means automatic startup setup yielded to another setup operation.
var ErrBusy = errors.New("CLI setup is already running")

func (m Manager) Status() (Status, error) {
	target, targetErr := m.target()
	if targetErr != nil {
		s, _ := m.status("", setupRecord{})
		s.Message = targetErr.Error()
		return s, nil
	}
	record, recordErr := m.readRecord()
	s, err := m.status(target, record)
	// Keep preferences and controls available when inspecting the link fails.
	if recordErr != nil {
		s.SetupError = recordErr.Error()
	} else if err != nil {
		s.SetupError = "Could not inspect CLI link: " + err.Error()
	}
	return s, nil
}

func (m Manager) status(target string, record setupRecord) (Status, error) {
	s := Status{Destination: m.destination(), AutoSetupDisabled: record.Disabled,
		Available: target != "", Target: target, PathCommand: `export PATH="$HOME/.local/bin:$PATH"`}
	if !s.Available {
		return s, nil
	}
	s.ExistingTarget = linkTarget(s.Destination)
	s.Installed = s.ExistingTarget == target
	_, err := os.Lstat(s.Destination)
	if err != nil && !os.IsNotExist(err) {
		return s, err
	}
	switch {
	case record.Disabled:
		s.Message = "Automatic CLI setup is disabled."
		if owned(s.ExistingTarget, target, record) {
			s.SetupError = "The CLI link is still present. Retry setup to finish disabling the CLI."
		}
	case s.Installed:
		s.Message = "CLI link installed. Check command discovery in your terminal."
	case err == nil && !owned(s.ExistingTarget, target, record):
		s.Message = "Another command occupies the CLI location. It has been preserved; move it before retrying setup."
	default:
		s.Message = "The CLI link needs setup. Retry to create or update it."
	}
	return s, nil
}

// TryEnsure sets up the CLI at startup without waiting for another operation.
func (m Manager) TryEnsure() (Status, error) { return m.change("ensure", true) }

// Change waits for cooperating app instances and installers before changing the CLI.
func (m Manager) Change(action string) (Status, error) { return m.change(action, false) }

func (m Manager) change(action string, nonblocking bool) (Status, error) {
	if action != "ensure" && action != "enable" && action != "disable" {
		return Status{}, fmt.Errorf("unknown CLI setup action %q", action)
	}
	target, err := m.target()
	if err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(filepath.Dir(m.record()), 0700); err != nil {
		return Status{}, err
	}
	lock := flock.New(m.record() + ".lock")
	if nonblocking {
		locked, err := lock.TryLock()
		if err != nil {
			return Status{}, err
		}
		if !locked {
			return Status{}, ErrBusy
		}
	} else if err := lock.Lock(); err != nil {
		return Status{}, err
	}
	defer lock.Unlock()
	record, err := m.readRecord()
	if err != nil {
		return Status{}, err
	}
	if action != "ensure" {
		record.Disabled = action == "disable"
		// Persist the preference even if inspecting or changing the link fails.
		if err := m.saveRecord(record); err != nil {
			return Status{}, err
		}
	}
	s, err := m.status(target, record)
	if err != nil {
		return s, err
	}
	if record.Disabled {
		if owned(s.ExistingTarget, s.Target, record) {
			if err := changeLink(s.Destination, "", s.ExistingTarget); err != nil {
				return s, err
			}
		}
		return m.status(target, record)
	}
	if !s.Installed {
		if err := m.ensureLink(s, record); err != nil {
			return s, err
		}
	}
	if record.Target != s.Target {
		record.Target = s.Target
		if err := m.saveRecord(record); err != nil {
			return s, fmt.Errorf("CLI installed, but could not save setup preferences: %w", err)
		}
	}
	return m.status(target, record)
}

func (m Manager) ensureLink(s Status, record setupRecord) error {
	_, err := os.Lstat(s.Destination)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(s.Destination), 0755); err != nil {
			return err
		}
		return os.Symlink(s.Target, s.Destination)
	}
	if err != nil {
		return err
	}
	if !owned(s.ExistingTarget, s.Target, record) {
		return errors.New(s.Message)
	}
	return changeLink(s.Destination, s.Target, s.ExistingTarget)
}
