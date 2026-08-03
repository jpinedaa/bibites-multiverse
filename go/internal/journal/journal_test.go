package journal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sampleEntry(id string) Entry {
	return Entry{
		MigrationID:    id,
		EntityID:       -843827577,
		Kind:           "bibite",
		GameVersion:    "0.6.3.1",
		Payload:        `{"body":{"id":{"id":-843827577}},"version":"0.6.3.1"}`,
		PayloadHash:    "cafebabe",
		Edge:           "E",
		Position:       0.6031925,
		VelocityX:      6.12,
		VelocityY:      0.44,
		Heading:        274.11,
		SimulationSize: 2000,
		SimTick:        4820344,
		DestSector:     "B",
		JournaledAt:    time.Now().UnixMilli(),
	}
}

func TestCreateAndGet(t *testing.T) {
	j, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	st, err := j.Create(Out, sampleEntry("m1"), false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != StatusOpen || st.Direction != Out {
		t.Fatalf("new state = %+v", st)
	}
	if _, err := j.Create(Out, sampleEntry("m1"), false); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second Create error = %v, want ErrDuplicate", err)
	}
	got, ok := j.Get("m1")
	if !ok || got.Entry.Payload != sampleEntry("m1").Payload {
		t.Fatalf("Get = %+v ok=%v", got, ok)
	}
	if _, ok := j.Get("nope"); ok {
		t.Fatal("Get invented an entry")
	}
}

func TestApplyIsDurableAndReplayed(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Create(In, sampleEntry("m1"), true); err != nil {
		t.Fatal(err)
	}
	attempt := 3
	if _, err := j.Apply("m1", Update{Status: StatusInFlight, Attempt: &attempt}); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	st, ok := reopened.Get("m1")
	if !ok {
		t.Fatal("the entry did not survive a reopen")
	}
	if st.Status != StatusInFlight || st.Attempt != 3 || !st.BounceBack || st.Direction != In {
		t.Fatalf("replayed state = %+v", st)
	}
	if st.Entry.Payload != sampleEntry("m1").Payload {
		t.Fatal("the payload did not survive a reopen")
	}
}

func TestApplyUnknownMigration(t *testing.T) {
	j, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if _, err := j.Apply("nope", Update{Status: StatusDone}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Apply error = %v, want ErrNotFound", err)
	}
}

func TestDoneDropsThePayloadButKeepsTheIdentity(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Create(Out, sampleEntry("m1"), false); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UnixMilli()
	st, err := j.Apply("m1", Update{Status: StatusDone, CompletedAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	if st.Entry.Payload != "" {
		t.Fatal("a tombstone must not keep the payload")
	}
	// contract-a.md §7.2: the tombstone is what makes a late retry safe, so the
	// hash and the entity id must survive.
	if st.Entry.PayloadHash != "cafebabe" || st.Entry.EntityID != -843827577 {
		t.Fatalf("tombstone lost its identity: %+v", st.Entry)
	}
	_ = j.Close()

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, ok := reopened.Get("m1"); !ok || got.Status != StatusDone || got.Entry.PayloadHash != "cafebabe" {
		t.Fatalf("tombstone after reopen = %+v ok=%v", got, ok)
	}
}

func TestListIsInJournalOrder(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"m1", "m2", "m3"} {
		if _, err := j.Create(In, sampleEntry(id), false); err != nil {
			t.Fatal(err)
		}
	}
	assertOrder := func(j *Journal) {
		t.Helper()
		got := []string{}
		for _, st := range j.List() {
			got = append(got, st.Entry.MigrationID)
		}
		if strings.Join(got, ",") != "m1,m2,m3" {
			t.Fatalf("List order = %v", got)
		}
	}
	assertOrder(j)
	_ = j.Close()

	// contract-a.md §7.5 replays in journal order, so compaction must preserve it.
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	assertOrder(reopened)
}

func TestTornTailIsDiscarded(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Create(Out, sampleEntry("m1"), false); err != nil {
		t.Fatal(err)
	}
	_ = j.Close()

	// Simulate a kill -9 in the middle of the next append.
	path := filepath.Join(dir, fileName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"op":"create","migrationId":"m2","entry":{"migrat`); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("Open after a torn write: %v", err)
	}
	defer reopened.Close()
	if _, ok := reopened.Get("m1"); !ok {
		t.Fatal("the complete record was lost")
	}
	if _, ok := reopened.Get("m2"); ok {
		t.Fatal("a torn record was accepted as durable")
	}
	// The log must be usable again straight away.
	if _, err := reopened.Create(Out, sampleEntry("m3"), false); err != nil {
		t.Fatalf("Create after a torn tail: %v", err)
	}
	if _, ok := reopened.Get("m3"); !ok {
		t.Fatal("the post-recovery record was lost")
	}
}

func TestCountPending(t *testing.T) {
	j, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	for _, id := range []string{"m1", "m2"} {
		if _, err := j.Create(In, sampleEntry(id), false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := j.Create(Out, sampleEntry("m3"), false); err != nil {
		t.Fatal(err)
	}
	if got := j.CountPending(In); got != 2 {
		t.Fatalf("CountPending(In) = %d, want 2", got)
	}
	at := time.Now().UnixMilli()
	if _, err := j.Apply("m1", Update{Status: StatusDone, CompletedAt: &at}); err != nil {
		t.Fatal(err)
	}
	if got := j.CountPending(In); got != 1 {
		t.Fatalf("CountPending(In) after a tombstone = %d, want 1", got)
	}
	if got := j.CountPending(Out); got != 1 {
		t.Fatalf("CountPending(Out) = %d, want 1", got)
	}
}

func TestPurgeExpiredOnlyTakesOldTombstones(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"old", "fresh", "live"} {
		if _, err := j.Create(Out, sampleEntry(id), false); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	oldAt := now.Add(-2 * time.Hour).UnixMilli()
	freshAt := now.UnixMilli()
	if _, err := j.Apply("old", Update{Status: StatusDone, CompletedAt: &oldAt}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Apply("fresh", Update{Status: StatusDone, CompletedAt: &freshAt}); err != nil {
		t.Fatal(err)
	}
	n, err := j.PurgeExpired(time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("purged %d entries, want 1", n)
	}
	if _, ok := j.Get("old"); ok {
		t.Fatal("the expired tombstone survived")
	}
	if _, ok := j.Get("fresh"); !ok {
		t.Fatal("a fresh tombstone was purged")
	}
	if _, ok := j.Get("live"); !ok {
		t.Fatal("a live entry was purged")
	}
	_ = j.Close()

	// The purge must be durable too, or §7.4 breaks after a restart.
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, ok := reopened.Get("old"); ok {
		t.Fatal("the purge was not durable")
	}
}

func TestCompactionShrinksTheLog(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Create(Out, sampleEntry("m1"), false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		attempt := i
		if _, err := j.Apply("m1", Update{Status: StatusInFlight, Attempt: &attempt}); err != nil {
			t.Fatal(err)
		}
	}
	_ = j.Close()
	before := fileSize(t, filepath.Join(dir, fileName))

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after := fileSize(t, filepath.Join(dir, fileName))
	if after >= before {
		t.Fatalf("compaction did not shrink the log: %d -> %d", before, after)
	}
	st, ok := reopened.Get("m1")
	if !ok || st.Attempt != 39 {
		t.Fatalf("compaction lost state: %+v ok=%v", st, ok)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}
