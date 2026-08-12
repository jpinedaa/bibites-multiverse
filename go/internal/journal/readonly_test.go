package journal

// OpenReadOnly is what a diagnostic opens, and these are its two promises: it
// reads the same state the owning process holds, and it cannot touch the file
// while that process is writing it.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAReadOnlyJournalNeitherCompactsNorWrites is the whole reason the mode
// exists. The ordinary Open COMPACTS — it rewrites the log to its live set and
// keeps a write handle on it — and a tool that promised to change nothing could
// not use it.
func TestAReadOnlyJournalNeitherCompactsNorWrites(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "journal")
	jr, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// A log with more history than live state, so a compaction would visibly
	// shrink it: every entry is created and then completed.
	for i := 0; i < 20; i++ {
		id := "m" + string(rune('a'+i))
		if _, err := jr.Create(Out, Entry{MigrationID: id, EntityID: int32(i), DestSlot: 2,
			JournaledAt: time.Now().UnixMilli()}, false); err != nil {
			t.Fatalf("create: %v", err)
		}
		if i%2 == 0 {
			continue
		}
		if _, err := jr.Apply(id, Update{Status: StatusDone}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}
	if err := jr.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	path := filepath.Join(dir, fileName)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	ro, err := OpenReadOnly(dir)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer ro.Close()

	// It read the same state.
	if got := ro.Live(); got == 0 {
		t.Fatal("the read-only replay found no live entries")
	}
	if _, ok := ro.Get("ma"); !ok {
		t.Fatal("the read-only replay lost an entry the writer had journaled")
	}
	if ro.Discarded() != 0 {
		t.Fatalf("a healthy journal replayed with %d discarded bytes", ro.Discarded())
	}
	if ro.Size() != before.Size() {
		t.Fatalf("Size reports %d and the file is %d", ro.Size(), before.Size())
	}

	// And it changed nothing. A compaction would have rewritten this file.
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("the file moved: %d bytes at %s became %d at %s",
			before.Size(), before.ModTime(), after.Size(), after.ModTime())
	}

	// Every mutating path refuses rather than corrupting a file another process
	// owns, or dereferencing a write handle that does not exist.
	if _, err := ro.Create(Out, Entry{MigrationID: "new", JournaledAt: 1}, false); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Create on a read-only journal returned %v, want ErrReadOnly", err)
	}
	if _, err := ro.Apply("ma", Update{Status: StatusDone}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Apply on a read-only journal returned %v, want ErrReadOnly", err)
	}
	if _, _, err := ro.Compact(); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Compact on a read-only journal returned %v, want ErrReadOnly", err)
	}
	final, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat final: %v", err)
	}
	if final.Size() != before.Size() {
		t.Fatalf("a refused write still moved the file: %d became %d", before.Size(), final.Size())
	}
}

// TestAReadOnlyJournalOnADirectoryThatDoesNotExistCreatesNothing: the ordinary
// Open makes the directory, which is a change, and a diagnostic run against a
// mistyped path must report an empty journal rather than leave a new one behind.
func TestAReadOnlyJournalOnADirectoryThatDoesNotExistCreatesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nothing", "here")
	ro, err := OpenReadOnly(dir)
	if err != nil {
		t.Fatalf("open read-only on a missing directory: %v", err)
	}
	defer ro.Close()
	if ro.Live() != 0 {
		t.Fatalf("a missing journal replayed %d entries", ro.Live())
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("opening a journal read-only created its directory")
	}
}
