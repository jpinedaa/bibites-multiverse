package bb8

// Incremental, resumable eviction over the content-addressed store.
//
// Sweep (store.go) is the SIDECAR's expiry: it materialises the whole store in
// one slice, sorts it by mtime and walks it, because a sidecar cache is capped
// at genomeCacheMaxBytes (2 GiB) and that shape costs nothing at that size. The
// ARCHIVE's store has no cap and never had an expiry at all — it is the record
// (contract-b-m4.md §10, §20 B20) — and the owner's Decision 3 of 2026-08-12
// gives it a retention horizon instead of a cap (§23, B33). At the archive's
// scale the sidecar's shape is the wrong one twice over: it holds the whole
// store's metadata live, and it does all of its work in one breath.
//
// So this is the same job written to §21 B21's discipline, which the archive
// learned the expensive way on 2026-08-10: A PASS IS BOUNDED IN WORK AND IT
// YIELDS. One pass examines at most a few shards and a few thousand files,
// removes at most a few hundred, resumes exactly where the last one stopped,
// and releases the store's own mutex every EvictBudget.Chunk removals.
//
// Why the yield matters here and is not decoration: the archive's fetch pump
// calls Has — an os.Stat under this store's mutex — while holding the ARCHIVE's
// mutex, which is the lock its relay read loop needs. A sweep that held this
// mutex across a whole pass would therefore stall the read loop through the
// other end of the same chain, and a subscriber that stops reading is one the
// relay closes (§5.1, §21). The incident that rule comes from cost 7,789
// crossings.
//
// THE TIME AXIS IS THE FILE'S MTIME, and that is a deliberate choice of the two
// available. Entry.StoredAt inside the file is when the blob was FIRST written
// and it never moves, so evicting on it would delete a genome served an hour
// ago; reading it also costs an open and a parse per file. The mtime is
// refreshed by Put on a re-store and by Get on every read (store.go), so it is
// "last stored or last served" — the same least-recently-served clock §10
// already gives the sidecar cache — and it arrives free with the directory
// walk. A blob served inside the horizon therefore keeps its whole horizon.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// EvictShards is how many shard directories a store has: the first two hex
// digits of the digest, 00 to ff.
const EvictShards = 256

// EvictCursor is where the next pass resumes. Its zero value starts at the
// first shard, which is what a fresh process wants.
type EvictCursor struct {
	// Shard is the next shard directory to examine, 0 to EvictShards-1.
	Shard int
	// After is the last file name examined in that shard, "" for its start.
	// os.ReadDir returns names in sorted order, so a name comparison is a
	// stable resume point — the same property §21 B21 requires of the fetch
	// queue's round-robin, and for the same reason: a bounded prefix of an
	// unstable order starves whatever it never reaches.
	After string
	// Cycles counts completed sweeps of all EvictShards shards.
	Cycles int
}

// EvictBudget bounds one pass. Every field falls back to a default when it is
// not positive, so a caller may set only what it cares about.
type EvictBudget struct {
	// Shards is how many shard directories one pass may open.
	Shards int
	// Examine is how many files one pass may stat.
	Examine int
	// Remove is how many files one pass may delete.
	Remove int
	// Chunk is how many deletions happen under one acquisition of the store's
	// mutex. It is the yield.
	Chunk int
}

func (b EvictBudget) withDefaults() EvictBudget {
	if b.Shards <= 0 {
		b.Shards = 8
	}
	if b.Examine <= 0 {
		b.Examine = 4096
	}
	if b.Remove <= 0 {
		b.Remove = 1024
	}
	if b.Chunk <= 0 {
		b.Chunk = 64
	}
	return b
}

// EvictResult is what one pass did.
type EvictResult struct {
	// Cursor is where the next pass must resume.
	Cursor EvictCursor
	// Examined is how many stored genomes this pass stat'ed.
	Examined int
	// Removed and Bytes are the blobs this pass deleted.
	Removed int
	Bytes   int64
	// Scratch is how many abandoned <hash>.json.tmp files this pass collected.
	// A Put cleans up its own failures; a process killed between the write and
	// the rename cannot, and §20 B20 makes collecting those the sweep's job.
	// The archive had no sweep at all until this one.
	Scratch int
	// Wrapped is true when this pass finished the last shard and the cursor
	// came back round to the first.
	Wrapped bool
}

// EvictOlderThan removes every stored genome whose mtime is before cutoff,
// bounded by budget and resumed from cur. It returns what it did, including the
// cursor the next pass must be given.
//
// A ZERO OR FUTURE CUTOFF IS REFUSED rather than obeyed: this function deletes
// the archive's record, and "the horizon is off" must never arrive here as
// "delete everything". Callers gate on their own horizon and this is the second
// gate, because the first one is one `if` away from being wrong.
func (s *Store) EvictOlderThan(cutoff time.Time, cur EvictCursor, budget EvictBudget) (EvictResult, error) {
	res := EvictResult{Cursor: cur}
	if cutoff.IsZero() {
		return res, errors.New("bb8: refusing to evict with no cutoff")
	}
	if res.Cursor.Shard < 0 || res.Cursor.Shard >= EvictShards {
		res.Cursor.Shard, res.Cursor.After = 0, ""
	}
	b := budget.withDefaults()
	// Scratch files are collected on the same walk, which is the only walk of
	// this store there is. staleTmpAge guarantees no live Put owns one.
	tmpCutoff := time.Now().Add(-staleTmpAge)

	opened := 0
	for opened < b.Shards && res.Examined < b.Examine && res.Removed < b.Remove {
		dir := filepath.Join(s.dir, fmt.Sprintf("%02x", res.Cursor.Shard))
		finished, err := s.evictShard(dir, cutoff, tmpCutoff, b, &res)
		if err != nil {
			return res, err
		}
		if !finished {
			// The budget ran out inside the shard. The cursor names the last
			// file examined, so the next pass takes up at the next one.
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

// evictShard walks one shard directory from the cursor. It reports whether it
// reached the end of the shard; false means the budget stopped it and the
// cursor now names where to resume.
func (s *Store) evictShard(dir string, cutoff, tmpCutoff time.Time, b EvictBudget, res *EvictResult) (bool, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// A shard nothing has ever hashed into. Nothing to resume from.
			return true, nil
		}
		return false, err
	}
	after := res.Cursor.After
	var batch []string
	var scratch []string
	flush := func() {
		if len(batch) > 0 {
			n, bytes := s.removeOlderThan(batch, cutoff)
			res.Removed += n
			res.Bytes += bytes
			batch = batch[:0]
		}
		if len(scratch) > 0 {
			n, _ := s.removeOlderThan(scratch, tmpCutoff)
			res.Scratch += n
			scratch = scratch[:0]
		}
	}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(name)
		if ext != ".json" && ext != ".tmp" {
			continue
		}
		if after != "" && name <= after {
			continue // examined by an earlier pass
		}
		if res.Examined >= b.Examine || res.Removed+len(batch) >= b.Remove {
			flush()
			return false, nil
		}
		res.Examined++
		res.Cursor.After = name
		info, err := e.Info()
		if err != nil {
			// It vanished between the listing and the stat, which on a
			// content-addressed store means somebody else already removed it.
			continue
		}
		switch {
		case ext == ".tmp":
			if info.ModTime().Before(tmpCutoff) {
				scratch = append(scratch, filepath.Join(dir, name))
			}
		case info.ModTime().Before(cutoff):
			batch = append(batch, filepath.Join(dir, name))
			if len(batch) >= b.Chunk {
				flush()
			}
		}
	}
	flush()
	return true, nil
}

// removeOlderThan deletes the named files UNDER THE STORE'S OWN MUTEX, with the
// mtime read again inside it.
//
// Both halves are load-bearing. Put and Get are the only other writers of a
// blob's mtime and both hold this mutex for the whole of what they do — Put for
// its stat-then-Chtimes on a re-store, Get for its read-then-Chtimes — so
// taking it here is what stops a delete landing between another goroutine's
// decision and its act. Re-reading the mtime is what honours the horizon
// itself: a blob served between the listing and this call has a fresh mtime and
// MUST survive, or "fetched inside the horizon is never evicted mid-horizon"
// would be true only of blobs nobody wanted.
//
// The caller batches, so the mutex is held for Chunk removals and not for a
// pass.
func (s *Store) removeOlderThan(paths []string, cutoff time.Time) (int, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed, bytes := 0, int64(0)
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue // touched since the listing: it keeps its whole horizon
		}
		if err := os.Remove(p); err != nil {
			continue
		}
		removed++
		bytes += info.Size()
	}
	return removed, bytes
}
