package clisetup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type setupRecord struct {
	Target   string
	Disabled bool
}

func (m Manager) readRecord() (setupRecord, error) {
	var record setupRecord
	data, err := os.ReadFile(m.record())
	if os.IsNotExist(err) {
		return record, nil
	}
	if err == nil {
		err = json.Unmarshal(data, &record)
	}
	if err != nil {
		return record, fmt.Errorf("Could not read CLI setup preferences at %s: %w", m.record(), err)
	}
	return record, nil
}

func (m Manager) saveRecord(record setupRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return writeRecord(m.record(), data)
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
