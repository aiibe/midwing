package midwing

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Activity struct {
	Time      time.Time `json:"time"`
	Operation string    `json:"operation"`
	Outcome   string    `json:"outcome"`
	Status    int       `json:"status,omitempty"`
}

func (s *Store) Record(a Activity) error {
	return s.locked(func() error {
		f, e := os.OpenFile(filepath.Join(s.Dir, "activity.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		defer f.Close()
		return json.NewEncoder(f).Encode(a)
	})
}

func (s *Store) History() ([]Activity, error) {
	out := []Activity{}
	e := s.locked(func() error {
		f, e := os.Open(filepath.Join(s.Dir, "activity.jsonl"))
		if errors.Is(e, os.ErrNotExist) {
			return nil
		}
		if e != nil {
			return e
		}
		defer f.Close()
		d := json.NewDecoder(f)
		for {
			var a Activity
			if e = d.Decode(&a); errors.Is(e, io.EOF) {
				return nil
			} else if e != nil {
				return fmt.Errorf("invalid activity history: %w", e)
			}
			out = append(out, a)
			if len(out) > 100 {
				out = out[1:]
			}
		}
	})
	return out, e
}
