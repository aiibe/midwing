package midwing

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryJSONLines(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    string
		count   int
		invalid bool
	}{
		{name: "empty"},
		{name: "whitespace", data: " \n\t"},
		{name: "records", data: "{\"operation\":\"first\"}\n{\"operation\":\"second\"}\n", count: 2},
		{name: "truncated", data: "{\"operation\":\"first\"}\n{", count: 1, invalid: true},
		{name: "closing bracket", data: "{\"operation\":\"first\"}\n]", count: 1, invalid: true},
		{name: "closing brace", data: "{\"operation\":\"first\"}\n}", count: 1, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			if err := os.WriteFile(filepath.Join(s.Dir, "activity.jsonl"), []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			rows, err := s.History()
			var wantErr error
			if tc.invalid {
				wantErr = ErrInvalidActivityHistory
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("history error: %v, want %v", err, wantErr)
			}
			if rows == nil || len(rows) != tc.count {
				t.Fatalf("history: %v, %v", rows, err)
			}
		})
	}
	t.Run("latest 100", func(t *testing.T) {
		s := testStore(t)
		for i := 0; i < 105; i++ {
			if err := s.Record(Activity{Status: i}); err != nil {
				t.Fatal(err)
			}
		}
		rows, err := s.History()
		if err != nil || len(rows) != 100 || rows[0].Status != 5 || rows[99].Status != 104 {
			t.Fatalf("history: %v, %v", rows, err)
		}
	})
}

func TestActivityTimeJSONName(t *testing.T) {
	b, e := json.Marshal(Activity{})
	if e != nil || !strings.Contains(string(b), `"time"`) {
		t.Fatal("activity time field must be stable")
	}
}

func TestHistoryRecoversValidRecordsAcrossFiles(t *testing.T) {
	s := testStore(t)
	for name, data := range map[string]string{
		"activity.1.jsonl": "{\"status\":1}\n{",
		"activity.jsonl":   "{\"status\":2}\ninvalid\n{\"status\":3}\n",
	} {
		if err := os.WriteFile(filepath.Join(s.Dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.History()
	if !errors.Is(err, ErrInvalidActivityHistory) {
		t.Fatalf("expected recoverable history warning, got %v", err)
	}
	if len(rows) != 3 || rows[0].Status != 1 || rows[1].Status != 2 || rows[2].Status != 3 {
		t.Fatalf("valid history was lost: %v, %v", rows, err)
	}
	// Reading damaged history must not rewrite either log.
	data, err := os.ReadFile(filepath.Join(s.Dir, "activity.1.jsonl"))
	if err != nil || string(data) != "{\"status\":1}\n{" {
		t.Fatalf("reading history changed the archive: %q, %v", data, err)
	}
}
