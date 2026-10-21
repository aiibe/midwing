package midwing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func assertLatestActivity(t *testing.T, s *Store, last int) {
	t.Helper()
	rows, err := s.History()
	if err != nil || len(rows) != activityHistoryLimit {
		t.Fatalf("history: %d entries, %v", len(rows), err)
	}
	for i, row := range rows {
		if want := last - activityHistoryLimit + 1 + i; row.Status != want {
			t.Fatalf("entry %d: status %d, want %d", i, row.Status, want)
		}
	}
}

func assertActivityFileBound(t *testing.T, s *Store, name string) {
	t.Helper()
	info, err := os.Stat(filepath.Join(s.Dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > activityFileLimit || info.Mode().Perm() != 0600 {
		t.Fatalf("%s: size %d, permissions %o", name, info.Size(), info.Mode().Perm())
	}
}

func TestActivityRotation(t *testing.T) {
	s := testStore(t)
	operation := strings.Repeat("x", 7000)
	for i := 0; i < 470; i++ {
		if err := s.Record(Activity{Operation: operation, Status: i}); err != nil {
			t.Fatal(err)
		}
		// Each point is shortly after a rotation: history must span both files.
		if i == 150 || i == 310 || i == 469 {
			assertLatestActivity(t, s, i)
			assertActivityFileBound(t, s, "activity.jsonl")
			assertActivityFileBound(t, s, "activity.1.jsonl")
		}
	}
	files, err := filepath.Glob(filepath.Join(s.Dir, "activity.jsonl-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary files left behind: %v, %v", files, err)
	}
}

func TestActivityLegacyOversizedLog(t *testing.T) {
	s := testStore(t)
	path := filepath.Join(s.Dir, "activity.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(f)
	for i := 0; i < 400; i++ {
		if err := encoder.Encode(Activity{Operation: strings.Repeat("x", 7000), Status: i}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	assertLatestActivity(t, s, 399)
	if err := s.Record(Activity{Status: 400}); err != nil {
		t.Fatal(err)
	}
	assertLatestActivity(t, s, 400)
	assertActivityFileBound(t, s, "activity.jsonl")
	assertActivityFileBound(t, s, "activity.1.jsonl")
}

func TestActivityTailBoundary(t *testing.T) {
	for _, boundary := range []string{"\n", "x"} {
		t.Run(fmt.Sprintf("boundary %q", boundary), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "activity.jsonl")
			// The byte before the budget determines whether the first line is whole.
			data := "prefix" + boundary + "{\"status\":1}\n"
			data += strings.Repeat(" ", activityFileLimit-len("{\"status\":1}\n"))
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			tail, err := readActivityTail(path)
			if err != nil || len(tail) > activityFileLimit {
				t.Fatalf("tail length %d: %v", len(tail), err)
			}
			if got := strings.Contains(string(tail), "status"); got != (boundary == "\n") {
				t.Fatalf("incorrect partial-line handling: contains status = %v", got)
			}
		})
	}
}

func TestActivityRejectsOversizedRecord(t *testing.T) {
	s := testStore(t)
	if err := s.Record(Activity{Status: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.Record(Activity{Operation: strings.Repeat("x", activityRecordLimit)}); err == nil {
		t.Fatal("oversized record accepted")
	}
	rows, err := s.History()
	if err != nil || len(rows) != 1 || rows[0].Status != 1 {
		t.Fatalf("rejected record changed history: %v, %v", rows, err)
	}
}

func TestActivityInterruptedRotation(t *testing.T) {
	s := testStore(t)
	for i := 0; i < activityHistoryLimit; i++ {
		if err := s.Record(Activity{Status: i}); err != nil {
			t.Fatal(err)
		}
	}
	// A process may stop after moving the old log, before creating the new one.
	if err := os.Rename(filepath.Join(s.Dir, "activity.jsonl"), filepath.Join(s.Dir, "activity.1.jsonl")); err != nil {
		t.Fatal(err)
	}
	assertLatestActivity(t, s, 99)
	if err := s.Record(Activity{Status: 100}); err != nil {
		t.Fatal(err)
	}
	assertLatestActivity(t, s, 100)
}

func TestActivityRotationFailurePreservesCurrentLog(t *testing.T) {
	s := testStore(t)
	path := filepath.Join(s.Dir, "activity.jsonl")
	data := []byte(strings.Repeat(" ", activityFileLimit))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(s.Dir, "activity.1.jsonl"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.Record(Activity{}); err == nil {
		t.Fatal("rotation failure ignored")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatalf("failed rotation changed current log: %v", err)
	}
}

func TestActivityDoesNotAcquireSettingsLock(t *testing.T) {
	s := testStore(t)
	err := s.locked(func() error {
		done := make(chan error, 1)
		go func() {
			if err := s.Record(Activity{}); err != nil {
				done <- err
				return
			}
			_, err := s.History()
			done <- err
		}()
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			return fmt.Errorf("activity blocked by settings lock")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestActivityConcurrentStores(t *testing.T) {
	s := testStore(t)
	// Fill most of the first file so concurrent writers also exercise rotation.
	for i := 0; i < 130; i++ {
		if err := s.Record(Activity{Operation: strings.Repeat("x", 7000), Status: -i - 1}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errors := make(chan error, 6)
	for worker := 0; worker < 6; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Separate stores model CLI and desktop instances sharing one directory.
			store := &Store{Dir: s.Dir}
			for i := 0; i < 20; i++ {
				if err := store.Record(Activity{Operation: strings.Repeat("y", 3000), Status: worker*20 + i}); err != nil {
					errors <- err
					return
				}
				rows, err := store.History()
				if err != nil {
					errors <- err
					return
				}
				if len(rows) != activityHistoryLimit {
					errors <- fmt.Errorf("concurrent history returned %d entries", len(rows))
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	count := 0
	for _, name := range []string{"activity.1.jsonl", "activity.jsonl"} {
		assertActivityFileBound(t, s, name)
		data, err := os.ReadFile(filepath.Join(s.Dir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			var row Activity
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				t.Fatal(err)
			}
			seen[row.Status] = true
			count++
		}
	}
	if len(seen) != 250 || count != 250 {
		t.Fatalf("concurrent rotation changed entries: %d unique, %d total, want 250", len(seen), count)
	}
}
