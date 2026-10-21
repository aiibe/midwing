package midwing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	activityFileLimit    = 1024 * 1024
	activityRecordLimit  = 8 * 1024
	activityHistoryLimit = 100
)

type Activity struct {
	Time      time.Time `json:"time"`
	Operation string    `json:"operation"`
	Outcome   string    `json:"outcome"`
	Status    int       `json:"status,omitempty"`
}

func (s *Store) Record(a Activity) error {
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > activityRecordLimit {
		return errors.New("activity record exceeds size limit")
	}
	return s.lockedFile("activity.lock", func() error {
		path := filepath.Join(s.Dir, "activity.jsonl")
		info, err := os.Stat(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if info != nil && info.Size()+int64(len(data)) > activityFileLimit {
			if info.Size() > activityFileLimit {
				// Upgrade an unbounded log without reading or archiving it in full.
				previous, err := readActivityTail(path)
				if err != nil {
					return err
				}
				if err := s.writeFile("activity.jsonl", previous); err != nil {
					return err
				}
			}
			// Move rather than copy so an interrupted rotation cannot duplicate history.
			if err := os.Rename(path, filepath.Join(s.Dir, "activity.1.jsonl")); err != nil {
				return err
			}
			return s.writeFile("activity.jsonl", data)
		}
		f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, err = f.Write(data)
		return errors.Join(err, f.Close())
	})
}

func (s *Store) History() ([]Activity, error) {
	out := []Activity{}
	var snapshots [][]byte
	err := s.lockedFile("activity.lock", func() error {
		for _, name := range []string{"activity.1.jsonl", "activity.jsonl"} {
			data, err := readActivityTail(filepath.Join(s.Dir, name))
			if err != nil {
				return err
			}
			snapshots = append(snapshots, data)
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	// Decode the bounded snapshots after releasing the activity lock.
	for _, data := range snapshots {
		d := json.NewDecoder(bytes.NewReader(data))
		for {
			var a Activity
			if err := d.Decode(&a); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				return out, fmt.Errorf("invalid activity history: %w", err)
			}
			out = append(out, a)
			if len(out) > activityHistoryLimit {
				out = out[1:]
			}
		}
	}
	return out, nil
}

// readActivityTail never scans more than one log's budget plus a boundary byte.
// Old oversized JSONL logs may start mid-record; discard that partial record.
func readActivityTail(path string) ([]byte, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	offset := max(int64(0), info.Size()-activityFileLimit-1)
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, activityFileLimit+1))
	if info.Size() > activityFileLimit {
		if boundary := bytes.IndexByte(data, '\n'); boundary >= 0 {
			data = data[boundary+1:]
		} else {
			data = nil
		}
	}
	return data, err
}
