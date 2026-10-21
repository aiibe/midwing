package midwing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryJSONStream(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    string
		count   int
		invalid bool
	}{
		{name: "empty"},
		{name: "whitespace", data: " \n\t"},
		{name: "records", data: "{\"operation\":\"first\"}\n{\"operation\":\"second\"}\n", count: 2},
		{name: "truncated", data: "{\"operation\":\"first\"}\n{", invalid: true},
		{name: "closing bracket", data: "{\"operation\":\"first\"}\n]", invalid: true},
		{name: "closing brace", data: "{\"operation\":\"first\"}\n}", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			if err := os.WriteFile(filepath.Join(s.Dir, "activity.jsonl"), []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			rows, err := s.History()
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid trailing data accepted")
				}
				return
			}
			if err != nil || rows == nil || len(rows) != tc.count {
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
