package bb8

// The COLD TIER's two store-level primitives, written to the same discipline as
// evict.go and for the same reason (§21 B21): a pass is bounded in work and it
// yields, so a store the size of the archive's never stalls the relay read loop
// through the mutex chain the eviction comment describes.
//
// They are TWO halves of one sequence and they are deliberately not one call:
//
//	BundleOlderThan   COLLECTS blobs past the hot window WITHOUT deleting them,
//	                  so a dated bundle and its manifest can be written and then
//	                  copied off-host. It NEVER touches an mtime — a bundling
//	                  pass that refreshed the clock it reads would postpone the
//	                  hot window for everything it looked at.
//	RetireHashes      DELETES named blobs from the hot store, once and only once
//	                  their bundle's off-host copy is confirmed (the archive
//	                  holds that gate; this only carries it out). It re-reads the
//	                  mtime under the store's own mutex, so a blob SERVED SINCE it
//	                  was listed keeps its whole window and is spared — the same
//	                  guarantee removeOlderThan gives the horizon sweep.
//
// The gate itself — "confirmed off-host copy" — lives in the archive, exactly as
// the ledger's retirement gate does (coldcopy.go). This file cannot open it and
// does not know it exists: it collects, and it deletes what it is told to.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// errBundleNoCutoff is the second gate, mirroring EvictOlderThan's: a bundling
// pass with no cutoff is a bug, never an instruction to bundle the whole store.
var errBundleNoCutoff = errors.New("bb8: refusing to bundle with no cutoff")

// BundleResult is what one bundling pass gathered.
type BundleResult struct {
	// Cursor is where the next pass must resume, exactly as EvictResult.Cursor.
	Cursor EvictCursor
	// Examined is how many stored genomes this pass stat'ed.
	Examined int
	// Entries are the blobs this pass selected: older than the hot window and not
	// refused by the caller's skip. Reading them cost one open each and NO mtime
	// touch, because the mtime is the hot window's clock.
	Entries []Entry
	// Wrapped is true when this pass finished the last shard and the cursor came
	// back round to the first.
	Wrapped bool
}

// BundleOlderThan walks the store from cur, bounded by budget, and collects every
// stored genome whose mtime is before cutoff and whose hex digest skip does not
// refuse. It DELETES NOTHING and REFRESHES NO MTIME. skip may be nil.
//
// A ZERO OR FUTURE CUTOFF IS REFUSED for the same reason EvictOlderThan refuses
// one: "the hot window is off" must never arrive here as "bundle everything".
func (s *Store) BundleOlderThan(cutoff time.Time, cur EvictCursor, budget EvictBudget,
	skip func(hexDigest string) bool) (BundleResult, error) {

	res := BundleResult{Cursor: cur}
	if cutoff.IsZero() {
		return res, errBundleNoCutoff
	}
	if res.Cursor.Shard < 0 || res.Cursor.Shard >= EvictShards {
		res.Cursor.Shard, res.Cursor.After = 0, ""
	}
	b := budget.withDefaults()

	opened := 0
	// Remove is reused as the collection bound: how many blobs one pass may pull
	// into a bundle. Examine bounds the stats, exactly as the sweep's does.
	for opened < b.Shards && res.Examined < b.Examine && len(res.Entries) < b.Remove {
		dir := filepath.Join(s.dir, fmt.Sprintf("%02x", res.Cursor.Shard))
		finished, err := s.bundleShard(dir, cutoff, b, skip, &res)
		if err != nil {
			return res, err
		}
		if !finished {
			return res, nil
		}
		opened++
		res.Cursor.Shard++
		res.Cursor.After = ""
		if res.Cursor.Shard >= EvictShards {
			res.Cursor.Shard = 0
			res.Cursor.Cycles++
			res.Wrapped = true
		}
	}
	return res, nil
}

func (s *Store) bundleShard(dir string, cutoff time.Time, b EvictBudget,
	skip func(string) bool, res *BundleResult) (bool, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	after := res.Cursor.After
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		if after != "" && name <= after {
			continue
		}
		if res.Examined >= b.Examine || len(res.Entries) >= b.Remove {
			return false, nil
		}
		res.Examined++
		res.Cursor.After = name
		info, err := e.Info()
		if err != nil {
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}
		hexDigest := name[:len(name)-len(".json")]
		if skip != nil && skip(hexDigest) {
			continue
		}
		// Read the bytes WITHOUT the store mutex and WITHOUT Chtimes: a
		// content-addressed file's bytes never change, and the mtime is the very
		// clock this pass selected on — refreshing it would reset the hot window
		// for the blob being retired. A file removed between the listing and this
		// read simply drops out of the bundle.
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var ent Entry
		if json.Unmarshal(raw, &ent) != nil || ent.GenomeHash == "" {
			continue
		}
		res.Entries = append(res.Entries, ent)
	}
	return true, nil
}

// RetireHashes deletes the named genomes from the hot store, UNDER THE STORE'S
// OWN MUTEX and with the mtime re-read inside it. A hash whose blob was SERVED
// SINCE the bundle was written has a fresh mtime and is SPARED — its bytes are in
// the off-host bundle too, so sparing it costs a duplicated copy and never a
// record. A hash whose blob is already gone (the horizon pruned it, or a prior
// pass retired it) counts as neither deleted nor an error: it is simply not here.
//
// IT YIELDS EVERY retireChunk DELETIONS, exactly as removeOlderThan does and for
// exactly the same reason (evict.go's header): heldOrColdLocked runs the fetch
// pump's Has under a.mu and blocks on THIS mutex, so a bundle of up to a few
// thousand hashes held across one lock would stall the relay read loop through
// the same chain the 2026-08-10 incident cost 7,789 crossings to. The lock is
// released between chunks and the caller already tolerates the partial totals.
//
// cutoff is the hot window recomputed at retirement time. removed and bytes are
// the blobs this call actually deleted; the caller counts them as retired-to-cold.
func (s *Store) RetireHashes(hashes []string, cutoff time.Time) (removed int, bytes int64) {
	const chunk = 64 // EvictBudget.withDefaults()'s Chunk: the yield.
	for i := 0; i < len(hashes); i += chunk {
		end := i + chunk
		if end > len(hashes) {
			end = len(hashes)
		}
		r, b := s.retireChunk(hashes[i:end], cutoff)
		removed += r
		bytes += b
	}
	return removed, bytes
}

// retireChunk deletes one bounded batch under one acquisition of the store mutex.
func (s *Store) retireChunk(hashes []string, cutoff time.Time) (removed int, bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, hash := range hashes {
		p, err := s.path(hash)
		if err != nil {
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			continue // already gone: pruned or retired by an earlier pass
		}
		if !info.ModTime().Before(cutoff) {
			continue // served since the bundle was written: keep its whole window
		}
		if err := os.Remove(p); err != nil {
			continue
		}
		removed++
		bytes += info.Size()
	}
	return removed, bytes
}

// GetNoTouch reads one genome WITHOUT refreshing its mtime. It is what a restore
// verifies against and what a bundle read uses: the ordinary Get is a "serve",
// and a serve is exactly what must not happen while deciding whether a blob is
// cold. The bool reports whether the blob is held HOT.
func (s *Store) GetNoTouch(hash string) (Entry, bool) {
	p, err := s.path(hash)
	if err != nil {
		return Entry{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Entry{}, false
	}
	var e Entry
	if json.Unmarshal(b, &e) != nil {
		return Entry{}, false
	}
	return e, true
}
