// Package clisetup manages an explicitly requested, per-user CLI link.
package clisetup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
)

type Status struct {
	Available      bool   `json:"available"`
	Installed      bool   `json:"installed"`
	CanInstall     bool   `json:"canInstall"`
	CanRepair      bool   `json:"canRepair"`
	CanRemove      bool   `json:"canRemove"`
	Destination    string `json:"destination"`
	Target         string `json:"target"`
	ExistingTarget string `json:"existingTarget"`
	Message        string `json:"message"`
	Warning        string `json:"warning"`
	PathCommand    string `json:"pathCommand"`
}

type Manager struct {
	Home       string
	Executable string
	SystemLink string
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

func linkTarget(path string) string {
	target, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return filepath.Clean(target)
}

func pointsTo(path, target string) bool {
	if linkTarget(path) == "" {
		return false
	}
	real, err := filepath.EvalSymlinks(path)
	return err == nil && real == target
}

func (m Manager) owned(path string) bool {
	target := linkTarget(path)
	if target == "" {
		return false
	}
	var record struct{ Target string }
	data, err := os.ReadFile(m.record())
	if err == nil && json.Unmarshal(data, &record) == nil && record.Target == target {
		return true
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(target)))
	return target == filepath.Join(bundle, "Contents", "MacOS", "midwing") &&
		strings.HasSuffix(bundle, ".app") && m.VerifyBundle(bundle)
}

func (m Manager) Status() (Status, error) {
	s := Status{Destination: m.destination(), PathCommand: `export PATH="$HOME/.local/bin:$PATH"`}
	target, err := m.target()
	if err != nil {
		s.Message = err.Error()
		return s, nil
	}
	s.Available, s.Target = true, target
	userCorrect, systemCorrect := pointsTo(s.Destination, target), pointsTo(m.SystemLink, target)
	s.Installed = userCorrect || systemCorrect
	if systemCorrect && !userCorrect {
		s.PathCommand = `export PATH="/usr/local/bin:$PATH"`
	}
	info, err := os.Lstat(s.Destination)
	if err != nil && !os.IsNotExist(err) {
		return s, err
	}
	exists := err == nil
	s.ExistingTarget = linkTarget(s.Destination)
	s.CanRemove = exists && m.owned(s.Destination)
	s.CanRepair = s.CanRemove && !userCorrect
	s.CanInstall = !exists && !systemCorrect
	switch {
	case userCorrect:
		s.Message = "CLI link is installed for this app. Verify command discovery in a fresh terminal."
	case systemCorrect && !exists:
		s.Message = "The existing /usr/local/bin/midwing link works with this app. No additional link is needed."
	case s.CanRepair:
		s.Message = "The user CLI link points to another Midwing location. Repair will replace that link with the current app's executable."
	case exists:
		s.Message = "An unrelated command occupies the user CLI destination. It has been preserved; move it yourself before installing."
	default:
		s.Message = "Install CLI to create a link to this app. No administrator access is needed for this step."
	}
	if exists && info.Mode()&os.ModeSymlink == 0 {
		s.ExistingTarget = ""
	}
	if systemCorrect && exists && !userCorrect {
		s.Warning = "The user command may shadow the working /usr/local/bin/midwing link. Run type -a midwing in a fresh terminal and review the first result."
	}
	if _, err := os.Lstat(m.SystemLink); err == nil && !systemCorrect {
		s.Warning = "Another command exists at /usr/local/bin/midwing. It is preserved. Run type -a midwing in a fresh terminal and resolve old installations deliberately."
	}
	return s, nil
}

// Change serializes cooperating app instances; exclusive symlink creation and
// checked removal also preserve commands created by another installer.
func (m Manager) Change(action string) (Status, error) {
	if _, err := m.target(); err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(filepath.Dir(m.record()), 0700); err != nil {
		return Status{}, err
	}
	lock := flock.New(m.record() + ".lock")
	if err := lock.Lock(); err != nil {
		return Status{}, err
	}
	defer lock.Unlock()
	s, err := m.Status()
	if err != nil {
		return s, err
	}
	switch action {
	case "install":
		if s.Installed {
			return s, nil
		}
		if !s.CanInstall {
			return s, errors.New(s.Message)
		}
		if err = os.MkdirAll(filepath.Dir(s.Destination), 0755); err == nil {
			err = os.Symlink(s.Target, s.Destination)
		}
	case "repair", "remove":
		if !s.CanRemove {
			return s, errors.New("Only a verified Midwing user link can be changed")
		}
		replacement := ""
		if action == "repair" {
			replacement = s.Target
		}
		err = changeLink(s.Destination, replacement, s.ExistingTarget)
	default:
		return s, fmt.Errorf("unknown CLI setup action %q", action)
	}
	if err != nil {
		return s, err
	}
	if action != "remove" {
		data, _ := json.Marshal(struct{ Target string }{s.Target})
		if err := writeRecord(m.record(), data); err != nil {
			return s, fmt.Errorf("CLI installed, but could not save move/repair information: %w", err)
		}
	}
	return m.Status()
}

func writeRecord(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".cli-setup-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
