package midwing

import (
	"bytes"
	"encoding/json"
	"errors"
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

type ActivityHistory struct {
	Entries []Activity `json:"entries"`
	Warning string     `json:"warning,omitempty"`
}

// ErrInvalidActivityHistory accompanies recovered entries when damaged records were skipped.
var ErrInvalidActivityHistory = errors.New("some activity records were damaged and skipped")

func (a Activity) marshal() ([]byte, error) {
	data, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if len(data) > activityRecordLimit {
		return nil, errors.New("activity record exceeds size limit")
	}
	return data, nil
}

func (a Activity) validateRequestSize() error {
	// Reserve room for the longest outcome and a three-digit HTTP status.
	a.Outcome, a.Status = "success", 999
	_, err := a.marshal()
	return err
}

func (s *Store) Record(a Activity) error {
	data, err := a.marshal()
	if err != nil {
		return err
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
	// Decode bounded JSONL snapshots after releasing the activity lock. A damaged
	// record must not hide valid later records or entries in the other log.
	var warning error
	for _, data := range snapshots {
		for line := range bytes.SplitSeq(data, []byte{'\n'}) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var a Activity
			if err := json.Unmarshal(line, &a); err != nil {
				warning = ErrInvalidActivityHistory
				continue
			}
			out = append(out, a)
			if len(out) > activityHistoryLimit {
				out = out[1:]
			}
		}
	}
	return out, warning
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
