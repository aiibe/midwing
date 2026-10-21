package midwing

import (
	"os"
	"path/filepath"
)

// writeFile atomically replaces a private file. Callers hold the relevant lock.
func (s *Store) writeFile(name string, data []byte) error {
	f, err := os.CreateTemp(s.Dir, name+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(s.Dir, name))
}
