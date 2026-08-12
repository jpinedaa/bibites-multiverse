package bb8

// The incremental eviction of contract-b-m4.md §23, B33 — the store's half of
// the owner's Decision 3 of 2026-08-12. These tests hold the horizon boundary,
// the bounds on one pass, the resume, and the one property that makes a horizon
// mean anything at all: a blob served inside it keeps its whole horizon.

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// age backdates a stored genome's mtime, which is the clock the horizon is
// measured on (evict.go: "last stored or last served").
func age(t *testing.T, s *Store, hash string, d time.Duration) {
	t.Helper()
	p, err := s.path(hash)
	if err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-d)
	if err := os.Chtimes(p, when, when); err != nil {
		t.Fatal(err)
	}
}

func mustPut(t *testing.T, s *Store, seed string) string {
	t.Helper()
	h := hashOf(t, seed)
	if err := s.Put(h, "0.6.3.1", "blob-"+seed); err != nil {
		t.Fatal(err)
	}
	return h
}

// TestEvictOlderThanRespectsTheHorizonBoundary is the rule in one line: older
// than the cutoff goes, everything else stays. A full sweep is forced by giving
// the budget the whole store.
func TestEvictOlderThanRespectsTheHorizonBoundary(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const horizon = 30 * 24 * time.Hour
	old := mustPut(t, s, "well-past-the-horizon")
	edgeOld := mustPut(t, s, "just-past-the-horizon")
	edgeNew := mustPut(t, s, "just-inside-the-horizon")
	fresh := mustPut(t, s, "stored-now")
	age(t, s, old, 90*24*time.Hour)
	age(t, s, edgeOld, horizon+time.Minute)
	age(t, s, edgeNew, horizon-time.Minute)

	res := sweepAll(t, s, time.Now().Add(-horizon))
	if res.Removed != 2 {
		t.Fatalf("removed %d, want 2 (the two past the horizon)", res.Removed)
	}
	if res.Bytes <= 0 {
		t.Fatalf("removed %d blobs and accounted for %d bytes", res.Removed, res.Bytes)
	}
	for _, h := range []string{old, edgeOld} {
		if s.Has(h) {
			t.Fatalf("a blob past the horizon survived: %s", h)
		}
	}
	for _, h := range []string{edgeNew, fresh} {
		if !s.Has(h) {
			t.Fatalf("a blob inside the horizon was evicted: %s", h)
		}
	}
}

// TestAPrunedHashAnswersLikeAnUnknownHash is §6.10's half, at the only place
// this store can hold it: the responder's answer is built from Get, and Get is
// what decides between a body and reason "unknown_hash". A pruned hash and a
// hash this store never held are the SAME two return values, which is why
// pruning costs no wire change (§23, B33).
func TestAPrunedHashAnswersLikeAnUnknownHash(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pruned := mustPut(t, s, "will-be-pruned")
	neverHeld := hashOf(t, "never-stored-anywhere")
	age(t, s, pruned, 40*24*time.Hour)
	sweepAll(t, s, time.Now().Add(-30*24*time.Hour))

	for _, h := range []string{pruned, neverHeld} {
		if e, ok := s.Get(h); ok {
			t.Fatalf("Get answered found for %s: %+v", h, e)
		}
		if s.Has(h) {
			t.Fatalf("Has answered true for %s", h)
		}
	}
}

// TestEvictionSparesABlobServedSinceTheListing is the mid-horizon guarantee.
// The listing decides a blob is old; a Get between the listing and the delete
// refreshes its mtime; the delete re-reads the mtime under the store's own lock
// and must let it live. Without the re-read, "fetched inside the horizon is
// never evicted" would be true only of blobs nobody wanted.
func TestEvictionSparesABlobServedSinceTheListing(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := mustPut(t, s, "served-just-in-time")
	p, err := s.path(h)
	if err != nil {
		t.Fatal(err)
	}
	age(t, s, h, 40*24*time.Hour)

	// The listing's decision, taken by hand: this file is past the cutoff.
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	if info, err := os.Stat(p); err != nil || !info.ModTime().Before(cutoff) {
		t.Fatalf("the fixture is wrong: %v", err)
	}
	// The read that saves it. Get refreshes the mtime, which is the whole point
	// of measuring the horizon on "last stored or last served".
	if _, ok := s.Get(h); !ok {
		t.Fatal("Get missed a stored genome")
	}
	if n, _ := s.removeOlderThan([]string{p}, cutoff); n != 0 {
		t.Fatal("a blob served since the listing was evicted anyway")
	}
	if !s.Has(h) {
		t.Fatal("the blob is gone")
	}
}

// TestOnePassIsBoundedAndResumes is §21 B21's discipline applied to the sweep:
// the cost of a pass is a property of the caller, not of the store's size, and
// the cursor guarantees the whole store is still visited.
func TestOnePassIsBoundedAndResumes(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const n = 120
	hashes := make([]string, n)
	for i := range hashes {
		hashes[i] = mustPut(t, s, string(rune('a'+i%26))+string(rune('0'+i/26))+"-blob")
		age(t, s, hashes[i], 60*24*time.Hour)
	}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)

	// One shard per pass, so a store spread over many shards cannot be cleared
	// in one: the pass is bounded and the cursor is what finishes the job.
	budget := EvictBudget{Shards: 1, Examine: 4, Remove: 4, Chunk: 2}
	res, err := s.EvictOlderThan(cutoff, EvictCursor{}, budget)
	if err != nil {
		t.Fatal(err)
	}
	if res.Examined > budget.Examine || res.Removed > budget.Remove {
		t.Fatalf("a pass exceeded its budget: examined %d, removed %d, budget %+v",
			res.Examined, res.Removed, budget)
	}
	if s.Count() == 0 {
		t.Fatal("one bounded pass cleared the whole store")
	}

	// Cycled to completion, every entry past the cutoff is gone and nothing is
	// starved. The bound is on the pass, never on the backlog.
	cur := res.Cursor
	for i := 0; i < 4096 && s.Count() > 0; i++ {
		r, err := s.EvictOlderThan(cutoff, cur, budget)
		if err != nil {
			t.Fatal(err)
		}
		cur = r.Cursor
	}
	if left := s.Count(); left != 0 {
		t.Fatalf("%d blob(s) past the horizon were never reached", left)
	}
}

// TestEvictionCollectsAbandonedScratchFilesOnTheSameWalk is §20 B20's second
// clause, which the archive's store could not satisfy before because it had no
// sweep at all. A .tmp young enough for a live Put to own is left alone.
func TestEvictionCollectsAbandonedScratchFilesOnTheSameWalk(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	shard := filepath.Join(dir, "ab")
	if err := os.MkdirAll(shard, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(shard, "abcdef.json.tmp")
	fresh := filepath.Join(shard, "fedcba.json.tmp")
	for _, p := range []string{stale, fresh} {
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	res := sweepAll(t, s, time.Now().Add(-30*24*time.Hour))
	if res.Scratch != 1 {
		t.Fatalf("collected %d scratch file(s), want 1", res.Scratch)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("the abandoned scratch file survived: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("a scratch file a live write could still own was taken: %v", err)
	}
}

// TestEvictionRefusesAZeroCutoff is the second gate. "The horizon is off" must
// never reach this store as "delete everything".
func TestEvictionRefusesAZeroCutoff(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := mustPut(t, s, "kept")
	if _, err := s.EvictOlderThan(time.Time{}, EvictCursor{}, EvictBudget{}); err == nil {
		t.Fatal("a zero cutoff was accepted")
	}
	if !s.Has(h) {
		t.Fatal("a refused eviction still deleted something")
	}
}

// sweepAll cycles the cursor until it wraps, so a test can assert on a whole
// store without caring how the budget divides it.
func sweepAll(t *testing.T, s *Store, cutoff time.Time) EvictResult {
	t.Helper()
	total := EvictResult{}
	cur := EvictCursor{}
	for i := 0; i < EvictShards+1; i++ {
		res, err := s.EvictOlderThan(cutoff, cur, EvictBudget{Shards: EvictShards})
		if err != nil {
			t.Fatal(err)
		}
		total.Examined += res.Examined
		total.Removed += res.Removed
		total.Bytes += res.Bytes
		total.Scratch += res.Scratch
		cur = res.Cursor
		if res.Wrapped {
			break
		}
	}
	return total
}
