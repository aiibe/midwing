package clisetup

import (
	"fmt"
	"os"
	"path/filepath"
)

// Move the old link aside, then verify its identity before removing it. A
// concurrent replacement is restored without overwriting a newer command.
func changeLink(path, replacement, expected string) error {
	before, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if before.Mode()&os.ModeSymlink == 0 || linkTarget(path) != expected {
		return fmt.Errorf("CLI destination changed during setup; refresh and retry")
	}
	directory, err := os.MkdirTemp(filepath.Dir(path), ".midwing-cli-*")
	if err != nil {
		return err
	}
	backup := filepath.Join(directory, "previous")
	if err := os.Rename(path, backup); err != nil {
		os.Remove(directory)
		return err
	}
	restore := func(cause error) error {
		if err := restoreCommand(backup, path); err != nil {
			return fmt.Errorf("%w; previous command preserved at %s (restore failed: %v)", cause, backup, err)
		}
		os.RemoveAll(directory)
		return cause
	}
	after, err := os.Lstat(backup)
	if err != nil || !os.SameFile(before, after) {
		return restore(fmt.Errorf("CLI destination changed during setup; retry after other installers finish"))
	}
	if replacement != "" {
		if err := os.Symlink(replacement, path); err != nil {
			return restore(err)
		}
	}
	return os.RemoveAll(directory)
}

// Recreate symlinks explicitly: os.Link follows them on macOS. Both operations
// fail if another installer has occupied the destination in the meantime.
func restoreCommand(backup, path string) error {
	info, err := os.Lstat(backup)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(backup)
		if err != nil {
			return err
		}
		return os.Symlink(target, path)
	}
	return os.Link(backup, path)
}
