package bb8

// The cold tier's two store-level primitives (coldbundle.go). The archive layers
// the receipt gate on top; these tests hold the primitives underneath it: a
// bundling pass COLLECTS without deleting, a retirement DELETES what it is told
// to, and a blob served since it was listed keeps its whole window.

import (
	"fmt"
	"testing"
	"time"
)

// TestBundleOlderThanCollectsAndNeverDeletes is the "bundled not deleted" half of
// the gate: the expired blobs come back as entries and every one is still in the
// store afterwards.
func TestBundleOlderThanCollectsAndNeverDeletes(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const window = 7 * 24 * time.Hour
	old1 := mustPut(t, s, "well-past-the-window-1")
	old2 := mustPut(t, s, "well-past-the-window-2")
	fresh := mustPut(t, s, "inside-the-window")
	age(t, s, old1, window+time.Hour)
	age(t, s, old2, 30*24*time.Hour)

	res, err := bundleAll(t, s, time.Now().Add(-window), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("collected %d entries, want 2", len(res.Entries))
	}
	got := map[string]bool{}
	for _, e := range res.Entries {
		got[e.GenomeHash] = true
		if e.BB8 == "" {
			t.Fatalf("a bundled entry carries no blob: %+v", e)
		}
	}
	if !got[old1] || !got[old2] {
		t.Fatalf("the wrong blobs were bundled: %v", got)
	}
	// NOTHING is deleted: bundling never removes a blob.
	for _, h := range []string{old1, old2, fresh} {
		if !s.Has(h) {
			t.Fatalf("bundling deleted %s", h)
		}
	}
}

// TestBundleOlderThanRespectsSkip: a blob the caller already has bundled or cold
// is not collected a second time.
func TestBundleOlderThanRespectsSkip(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const window = 7 * 24 * time.Hour
	a := mustPut(t, s, "aaa")
	b := mustPut(t, s, "bbb")
	age(t, s, a, 30*24*time.Hour)
	age(t, s, b, 30*24*time.Hour)
	skipHex := HashHex(a)
	res, err := bundleAll(t, s, time.Now().Add(-window), func(h string) bool { return h == skipHex })
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 1 || res.Entries[0].GenomeHash != b {
		t.Fatalf("skip was not honoured: %+v", res.Entries)
	}
}

// TestRetireHashesDeletesPastCutoffAndSparesServedSince is the whole retirement
// contract: a blob still past the window is deleted, and one served since — a
// fresh mtime — is spared, exactly as the horizon sweep spares one.
func TestRetireHashesDeletesPastCutoffAndSparesServedSince(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const window = 7 * 24 * time.Hour
	retire := mustPut(t, s, "to-be-retired")
	served := mustPut(t, s, "served-since-bundling")
	age(t, s, retire, 30*24*time.Hour)
	age(t, s, served, 30*24*time.Hour)

	// `served` is fetched between the bundle and the retirement: Get refreshes its
	// mtime, which is the clock the retirement re-reads.
	if _, ok := s.Get(served); !ok {
		t.Fatal("Get missed a stored genome")
	}
	cutoff := time.Now().Add(-window)
	removed, bytes := s.RetireHashes([]string{retire, served}, cutoff)
	if removed != 1 || bytes <= 0 {
		t.Fatalf("removed %d / %d bytes, want 1 and >0", removed, bytes)
	}
	if s.Has(retire) {
		t.Fatal("a blob past the window survived retirement")
	}
	if !s.Has(served) {
		t.Fatal("a blob served since the bundle was written was retired anyway")
	}
	// A hash already gone is neither an error nor a second deletion.
	if r, _ := s.RetireHashes([]string{retire}, cutoff); r != 0 {
		t.Fatalf("retiring an already-gone hash removed %d", r)
	}
}

// TestRetireHashesChunksAndStillSparesServedSince: RetireHashes now yields the
// store mutex every 64 deletions (so it cannot starve the relay read loop), and
// the mid-batch served-since guarantee must survive that chunking — a blob served
// between chunks keeps its whole window.
func TestRetireHashesChunksAndStillSparesServedSince(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const window = 7 * 24 * time.Hour
	var all []string
	for i := 0; i < 200; i++ { // three-plus chunks of 64
		h := mustPut(t, s, fmt.Sprintf("chunked-blob-%d", i))
		age(t, s, h, 30*24*time.Hour)
		all = append(all, h)
	}
	// One blob past the first chunk boundary is served, refreshing its mtime.
	served := all[130]
	if _, ok := s.Get(served); !ok {
		t.Fatal("Get missed a stored genome")
	}
	removed, bytes := s.RetireHashes(all, time.Now().Add(-window))
	if removed != 199 {
		t.Fatalf("removed %d, want 199 (all but the served one)", removed)
	}
	if bytes <= 0 {
		t.Fatalf("removed %d blobs and accounted for %d bytes", removed, bytes)
	}
	if !s.Has(served) {
		t.Fatal("a blob served since the listing was retired across a chunk boundary")
	}
	if s.Count() != 1 {
		t.Fatalf("%d blobs left, want 1 (the served one)", s.Count())
	}
}

func TestBundleOlderThanRefusesZeroCutoff(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := mustPut(t, s, "kept")
	if _, err := s.BundleOlderThan(time.Time{}, EvictCursor{}, EvictBudget{}, nil); err == nil {
		t.Fatal("a zero cutoff was accepted")
	}
	if !s.Has(h) {
		t.Fatal("a refused bundling still touched the store")
	}
}

// bundleAll cycles the bundling cursor until it wraps, so a test can collect a
// whole store without caring how the budget divides it.
func bundleAll(t *testing.T, s *Store, cutoff time.Time, skip func(string) bool) (BundleResult, error) {
	t.Helper()
	total := BundleResult{}
	cur := EvictCursor{}
	for i := 0; i < EvictShards+1; i++ {
		res, err := s.BundleOlderThan(cutoff, cur, EvictBudget{Shards: EvictShards}, skip)
		if err != nil {
			return total, err
		}
		total.Entries = append(total.Entries, res.Entries...)
		total.Examined += res.Examined
		cur = res.Cursor
		if res.Wrapped {
			break
		}
	}
	return total, nil
}
