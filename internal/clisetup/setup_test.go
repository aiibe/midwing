package clisetup

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func fixture(t *testing.T) Manager {
	t.Helper()
	temporary, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(temporary, "User With Spaces")
	root := filepath.Join(home, "Applications")
	binary := filepath.Join(root, "Midwing.app", "Contents", "MacOS", "midwing")
	if err := os.MkdirAll(filepath.Dir(binary), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("app"), 0755); err != nil {
		t.Fatal(err)
	}
	return Manager{Home: home, Executable: binary, Roots: []string{root},
		SystemLink: filepath.Join(home, "usr", "local", "bin", "midwing"),
		VerifyBundle: func(bundle string) bool {
			_, err := os.Stat(filepath.Join(bundle, "Contents", "MacOS", "midwing"))
			return err == nil
		},
	}
}

func symlink(t *testing.T, path, target string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func TestInstallIdempotenceAndRemoval(t *testing.T) {
	m := fixture(t)
	s, err := m.Status()
	if err != nil || !s.CanInstall || s.Installed {
		t.Fatalf("fresh: %+v %v", s, err)
	}
	for range 2 {
		s, err = m.Change("install")
		if err != nil || !s.Installed || !s.CanRemove {
			t.Fatalf("install: %+v %v", s, err)
		}
	}
	if linkTarget(m.destination()) != m.Executable {
		t.Fatal("incorrect target")
	}
	s, err = m.Change("remove")
	if err != nil || s.Installed || !s.CanInstall {
		t.Fatalf("remove: %+v %v", s, err)
	}
	if _, err := os.Stat(m.Executable); err != nil {
		t.Fatal("app removed", err)
	}
}

func TestSetupPreservesExistingSettingsAndActivity(t *testing.T) {
	m := fixture(t)
	directory := filepath.Dir(m.record())
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"settings.json", "activity.jsonl"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("existing data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, action := range []string{"install", "remove"} {
		if _, err := m.Change(action); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"settings.json", "activity.jsonl"} {
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil || string(data) != "existing data" {
			t.Fatalf("%s changed: %v", name, err)
		}
	}
}

func TestLegacySystemLinkNeedsNoDuplicate(t *testing.T) {
	m := fixture(t)
	symlink(t, m.SystemLink, m.Executable)
	s, err := m.Change("install")
	if err != nil || !s.Installed || s.CanRemove || s.CanInstall {
		t.Fatalf("legacy: %+v %v", s, err)
	}
	if s.PathCommand != `export PATH="/usr/local/bin:$PATH"` {
		t.Fatalf("legacy PATH guidance: %q", s.PathCommand)
	}
	if _, err := os.Lstat(m.destination()); !os.IsNotExist(err) {
		t.Fatal("created duplicate")
	}
	if _, err := m.Change("remove"); err == nil {
		t.Fatal("offered to remove system link")
	}
}

func TestUserConflictWarnsWhenLegacyLinkWorks(t *testing.T) {
	m := fixture(t)
	symlink(t, m.SystemLink, m.Executable)
	symlink(t, m.destination(), "/unrelated-command")
	s, err := m.Status()
	if err != nil || !s.Installed || s.Warning == "" || s.CanInstall || s.CanRemove {
		t.Fatalf("shadowed legacy link: %+v %v", s, err)
	}
}

func TestUnrelatedCommandsArePreserved(t *testing.T) {
	for _, target := range []string{"file", "/missing/unrelated", "../unrelated", "/missing/Midwing.app/Contents/MacOS/midwing"} {
		t.Run(target, func(t *testing.T) {
			m := fixture(t)
			if target == "file" {
				os.MkdirAll(filepath.Dir(m.destination()), 0755)
				os.WriteFile(m.destination(), []byte("unrelated"), 0644)
			} else {
				symlink(t, m.destination(), target)
			}
			for _, action := range []string{"install", "repair", "remove"} {
				if _, err := m.Change(action); err == nil {
					t.Fatalf("allowed %s", action)
				}
			}
			if target == "file" {
				data, _ := os.ReadFile(m.destination())
				if string(data) != "unrelated" {
					t.Fatal("file changed")
				}
			} else if actual, _ := os.Readlink(m.destination()); actual != target {
				t.Fatal("link changed")
			}
		})
	}
}

func TestMovedAppRepairsRecordedDanglingLink(t *testing.T) {
	m := fixture(t)
	if _, err := m.Change("install"); err != nil {
		t.Fatal(err)
	}
	oldBundle := filepath.Dir(filepath.Dir(filepath.Dir(m.Executable)))
	newBundle := filepath.Join(filepath.Dir(oldBundle), "Midwing Renamed.app")
	if err := os.Rename(oldBundle, newBundle); err != nil {
		t.Fatal(err)
	}
	m.Executable = filepath.Join(newBundle, "Contents", "MacOS", "midwing")
	s, err := m.Status()
	if err != nil || !s.CanRepair || s.Installed {
		t.Fatalf("moved: %+v %v", s, err)
	}
	if _, err := m.Change("install"); err == nil {
		t.Fatal("repair must be explicit")
	}
	s, err = m.Change("repair")
	if err != nil || !s.Installed || s.CanRepair {
		t.Fatalf("repair: %+v %v", s, err)
	}
}

func TestOldSourceLinkAndSystemAmbiguity(t *testing.T) {
	m := fixture(t)
	old := m.Executable
	renamed := filepath.Join(m.Roots[0], "Current.app")
	os.MkdirAll(filepath.Join(renamed, "Contents", "MacOS"), 0755)
	m.Executable = filepath.Join(renamed, "Contents", "MacOS", "midwing")
	os.WriteFile(m.Executable, []byte("current"), 0755)
	symlink(t, m.destination(), old)
	symlink(t, m.SystemLink, old)
	s, err := m.Status()
	if err != nil || !s.CanRepair || s.Warning == "" {
		t.Fatalf("source migration: %+v %v", s, err)
	}
	s, err = m.Change("repair")
	if err != nil || !s.Installed || s.Warning == "" {
		t.Fatalf("repair: %+v %v", s, err)
	}
	if linkTarget(m.SystemLink) != old {
		t.Fatal("system command changed")
	}
}

func TestTransientAndDevelopmentLocations(t *testing.T) {
	for _, path := range []string{"Volumes/Midwing", "AppTranslocation/random/d", "build/bin"} {
		t.Run(path, func(t *testing.T) {
			m := fixture(t)
			bundle := filepath.Dir(filepath.Dir(filepath.Dir(m.Executable)))
			destination := filepath.Join(m.Home, path, "Midwing.app")
			os.MkdirAll(filepath.Dir(destination), 0755)
			os.Rename(bundle, destination)
			m.Executable = filepath.Join(destination, "Contents", "MacOS", "midwing")
			s, err := m.Status()
			if err != nil || s.Available || !strings.Contains(s.Message, "installed copy") {
				t.Fatalf("transient: %+v %v", s, err)
			}
			if _, err := m.Change("install"); err == nil {
				t.Fatal("installed transient link")
			}
		})
	}
}

func TestConcurrentInstallAndChangedDestination(t *testing.T) {
	m := fixture(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := m.Change("install"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if linkTarget(m.destination()) != m.Executable {
		t.Fatal("concurrent install failed")
	}
	os.Remove(m.destination())
	symlink(t, m.destination(), "/other-command")
	if err := changeLink(m.destination(), m.Executable, m.Executable); err == nil {
		t.Fatal("replaced changed link")
	}
	if linkTarget(m.destination()) != "/other-command" {
		t.Fatal("concurrent command lost")
	}
}

func TestPermissionsAndInvalidBundle(t *testing.T) {
	m := fixture(t)
	os.MkdirAll(filepath.Dir(m.destination()), 0755)
	os.Chmod(filepath.Dir(m.destination()), 0500)
	t.Cleanup(func() { os.Chmod(filepath.Dir(m.destination()), 0755) })
	if _, err := m.Change("install"); err == nil {
		t.Fatal("ignored permissions")
	}
	m.VerifyBundle = func(string) bool { return false }
	s, err := m.Status()
	if err != nil || s.Available {
		t.Fatalf("invalid bundle: %+v %v", s, err)
	}
}
