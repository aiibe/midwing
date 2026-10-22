package clisetup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestEnsureInstallsRepairsAndRespectsRemoval(t *testing.T) {
	m := fixture(t)
	for range 2 {
		s, err := m.Change("ensure")
		if err != nil || !s.Installed {
			t.Fatalf("ensure: %+v %v", s, err)
		}
	}
	m.Executable = filepath.Join(m.Roots[0], "New.app", "Contents", "MacOS", "midwing")
	if err := os.MkdirAll(filepath.Dir(m.Executable), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.Executable, []byte("new version"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Change("ensure"); err != nil {
		t.Fatal(err)
	}
	if linkTarget(m.destination()) != m.Executable {
		t.Fatal("did not update CLI to current app")
	}
	if _, err := m.Change("disable"); err != nil {
		t.Fatal(err)
	}
	s, err := m.Change("ensure")
	if err != nil || s.Installed {
		t.Fatalf("removal not respected: %+v %v", s, err)
	}
	if s, err = m.Change("enable"); err != nil || !s.Installed {
		t.Fatalf("manual reinstall: %+v %v", s, err)
	}
}

func TestTryEnsureSkipsBusySetup(t *testing.T) {
	m := fixture(t)
	if err := os.MkdirAll(filepath.Dir(m.record()), 0700); err != nil {
		t.Fatal(err)
	}
	lock := flock.New(m.record() + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer lock.Unlock()
	if _, err := m.TryEnsure(); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected busy setup, got %v", err)
	}
	if _, err := os.Lstat(m.destination()); !os.IsNotExist(err) {
		t.Fatal("changed CLI while setup was locked")
	}
}

func TestExplicitEnsureWaitsForBusySetup(t *testing.T) {
	m := fixture(t)
	if err := os.MkdirAll(filepath.Dir(m.record()), 0700); err != nil {
		t.Fatal(err)
	}
	lock := flock.New(m.record() + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer lock.Unlock()
	done := make(chan error, 1)
	go func() {
		s, err := m.Change("ensure")
		if err == nil && !s.Installed {
			err = errors.New("setup returned before the CLI was installed")
		}
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("setup did not wait for the lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("setup did not resume after the lock was released")
	}
}

func TestEnsurePreservesMalformedPreferences(t *testing.T) {
	m := fixture(t)
	if err := os.MkdirAll(filepath.Dir(m.record()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.record(), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Change("ensure"); err == nil {
		t.Fatal("expected invalid preferences error")
	}
	if _, err := os.Lstat(m.destination()); !os.IsNotExist(err) {
		t.Fatal("installed CLI despite unreadable preferences")
	}
}

func TestMovedAppRepairsRecordedDanglingLink(t *testing.T) {
	m := fixture(t)
	if _, err := m.Change("ensure"); err != nil {
		t.Fatal(err)
	}
	oldBundle := filepath.Dir(filepath.Dir(filepath.Dir(m.Executable)))
	newBundle := filepath.Join(m.Roots[0], "Moved.app")
	if err := os.Rename(oldBundle, newBundle); err != nil {
		t.Fatal(err)
	}
	m.Executable = filepath.Join(newBundle, "Contents", "MacOS", "midwing")
	if s, err := m.Change("ensure"); err != nil || !s.Installed {
		t.Fatalf("moved app: %+v %v", s, err)
	}
	if linkTarget(m.destination()) != m.Executable {
		t.Fatal("CLI does not point to moved app")
	}
}

func TestDisablePersistsWhenLinkRemovalFails(t *testing.T) {
	m := fixture(t)
	if _, err := m.Change("ensure"); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(m.destination())
	if err := os.Chmod(directory, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(directory, 0755) })
	if _, err := m.Change("disable"); err == nil {
		t.Fatal("expected removal to fail")
	}
	s, err := m.Status()
	if err != nil || !s.AutoSetupDisabled || !s.Installed {
		t.Fatalf("opt-out lost after failure: %+v %v", s, err)
	}
	if err := os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if s, err := m.Change("ensure"); err != nil || !s.AutoSetupDisabled || s.Installed {
		t.Fatalf("retry removal: %+v %v", s, err)
	}
}

func TestDisablePersistsWhenLinkCannotBeInspected(t *testing.T) {
	m := fixture(t)
	if _, err := m.Change("ensure"); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(m.destination())
	if err := os.Chmod(directory, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(directory, 0755) })
	if _, err := m.Change("disable"); err == nil {
		t.Fatal("expected link inspection to fail")
	}
	s, err := m.Status()
	if err != nil || !s.Available || !s.AutoSetupDisabled || !strings.Contains(s.SetupError, "inspect") {
		t.Fatalf("opt-out or status controls lost: %+v %v", s, err)
	}
	if err := os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if s, err := m.Change("ensure"); err != nil || s.Installed || !s.AutoSetupDisabled {
		t.Fatalf("retry removal: %+v %v", s, err)
	}
}

func TestDisabledStatusReportsOwnedLinkToPreviousApp(t *testing.T) {
	m := fixture(t)
	if _, err := m.Change("ensure"); err != nil {
		t.Fatal(err)
	}
	record, err := m.readRecord()
	if err != nil {
		t.Fatal(err)
	}
	record.Disabled = true
	if err := m.saveRecord(record); err != nil {
		t.Fatal(err)
	}
	oldBundle := filepath.Dir(filepath.Dir(filepath.Dir(m.Executable)))
	newBundle := filepath.Join(m.Roots[0], "Moved.app")
	if err := os.Rename(oldBundle, newBundle); err != nil {
		t.Fatal(err)
	}
	m.Executable = filepath.Join(newBundle, "Contents", "MacOS", "midwing")
	s, err := m.Status()
	if err != nil || s.Installed || s.SetupError == "" || !s.AutoSetupDisabled {
		t.Fatalf("unfinished removal was hidden: %+v %v", s, err)
	}
	if s, err := m.Change("ensure"); err != nil || s.SetupError != "" {
		t.Fatalf("retry removal: %+v %v", s, err)
	}
}
