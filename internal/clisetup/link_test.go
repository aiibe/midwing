package clisetup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFailedReplacementRestoresOriginalSymlink(t *testing.T) {
	for _, target := range []string{"existing", "missing", "../relative-command"} {
		t.Run(target, func(t *testing.T) {
			directory := t.TempDir()
			if target == "existing" {
				target = filepath.Join(directory, target)
				if err := os.WriteFile(target, []byte("command"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(directory, "midwing")
			symlink(t, path, target)
			// A rejected replacement must restore even a dangling or relative link.
			if err := changeLink(path, "\x00", linkTarget(path)); err == nil {
				t.Fatal("invalid replacement succeeded")
			}
			if actual, err := os.Readlink(path); err != nil || actual != target {
				t.Fatalf("original symlink not restored: target=%q err=%v", actual, err)
			}
			entries, err := os.ReadDir(directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					t.Fatalf("backup left behind: %s", entry.Name())
				}
			}
		})
	}
}

func TestRestoreCommandPreservesOccupiedDestination(t *testing.T) {
	for _, kind := range []string{"file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			backup := filepath.Join(directory, "previous")
			if kind == "symlink" {
				symlink(t, backup, "../original-command")
			} else if err := os.WriteFile(backup, []byte("original"), 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "midwing")
			if err := os.WriteFile(path, []byte("new command"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := restoreCommand(backup, path); !os.IsExist(err) {
				t.Fatalf("expected occupied destination error, got %v", err)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != "new command" {
				t.Fatalf("destination changed: %q %v", data, err)
			}
			if _, err := os.Lstat(backup); err != nil {
				t.Fatalf("backup lost: %v", err)
			}
		})
	}
}
