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
	for _, action := range []string{"ensure", "disable", "enable"} {
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
			for _, action := range []string{"ensure", "enable"} {
				if _, err := m.Change(action); err == nil || !strings.Contains(err.Error(), "Another command") {
					t.Fatalf("conflict missing for %s: %v", action, err)
				}
			}
			// Opting out must preserve a command that Midwing does not own.
			if s, err := m.Change("disable"); err != nil || !s.AutoSetupDisabled {
				t.Fatalf("disable: %+v %v", s, err)
			}
			if _, err := m.Change("ensure"); err != nil {
				t.Fatal(err)
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
			if _, err := m.Change("ensure"); err == nil {
				t.Fatal("installed transient link")
			}
		})
	}
}

func TestUnrecordedAppLinksAndCommandChainsArePreserved(t *testing.T) {
	for _, name := range []string{"other app", "command chain"} {
		t.Run(name, func(t *testing.T) {
			m := fixture(t)
			target := filepath.Join(m.Roots[0], "Other.app", "Contents", "MacOS", "midwing")
			if name == "command chain" {
				target = filepath.Join(m.Home, "another-command")
				symlink(t, target, m.Executable)
			} else {
				os.MkdirAll(filepath.Dir(target), 0755)
				os.WriteFile(target, []byte("other app"), 0755)
			}
			symlink(t, m.destination(), target)
			if _, err := m.Change("ensure"); err == nil {
				t.Fatal("adopted an unrecorded command")
			}
			if s, err := m.Change("disable"); err != nil || s.Installed {
				t.Fatalf("unrecorded command treated as owned: %+v %v", s, err)
			}
			if linkTarget(m.destination()) != target {
				t.Fatal("unrecorded command changed")
			}
		})
	}
}

func TestConcurrentEnableAndChangedDestination(t *testing.T) {
	m := fixture(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := m.Change("enable"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if linkTarget(m.destination()) != m.Executable {
		t.Fatal("concurrent setup failed")
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
	if _, err := m.Change("ensure"); err == nil {
		t.Fatal("ignored permissions")
	}
	m.VerifyBundle = func(string) bool { return false }
	s, err := m.Status()
	if err != nil || s.Available {
		t.Fatalf("invalid bundle: %+v %v", s, err)
	}
}
