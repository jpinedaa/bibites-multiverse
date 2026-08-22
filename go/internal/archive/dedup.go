package archive

import (
	"hash/maphash"
	"time"
)

// The §5.1 duplicate set: the structure that answers "has this record already
// been recorded" for every MIGRATION, ACK and NACK the archive ever sees.
//
// IT WAS THE ONE RETAINED STRUCTURE THAT GREW WITH THE LEDGER, AND SINCE §25's
// B38 IT IS NOT. The reason it could never forget was the re-forward: a sender
// that re-sent a frame a year later had to be refused a year later, or the
// archive appended it to migrations.jsonl a second time and the permanent record
// gained a duplicate nothing can remove — the file is never rewritten
// (store.go). B37 removed the re-forward. A conforming sender hands each
// migration to the relay exactly once, so a SECOND copy of one record can now
// only come from a sidecar older than B37 still running its retry, or from a
// peer with a defect; both of those arrive within seconds or minutes of the
// first, never a year later.
//
// So the set became a WINDOW (dedupWindow, below), and with it every retained
// thing in this package is bounded by construction: species at 65,536, genome
// fingerprints per species at 8192, brain buckets at a year, lanes at the grid,
// gaps at the retention horizon, and this at its own window. What it costs PER
// KEY still decides how big the window may be.
//
// WHAT IT COSTS. Measured on this dev box over 5,408,123 distinct keys, which
// is the record count of the 2026-08-16 production ledger:
//
//	map[string]bool over "TYPE/" + dedupKey(...)   460.9 MiB   89.4 B/key
//	map[dedupFP]struct{} over the same keys        213.3 MiB   41.4 B/key
//	this set, sized from the ledger                128.0 MiB   24.8 B/key
//
// The string form was 88% of the archive's resident set. A Go map of the
// fingerprint alone already halves it — the 46-byte key, its allocation header
// and the pointer to it all go away, and the garbage collector stops walking
// the set entirely — but a large Go map runs at a load factor around 0.3 to
// 0.45, so 16 bytes of key still cost 41 to 56 bytes of table. This set is the
// same idea at a load factor it controls: flat arrays of fingerprints, open
// addressing, linear probing, no per-entry allocation and no pointer anywhere
// in it.
//
// IT CAN DO THAT BECAUSE NOTHING IS EVER DELETED FROM IT. Linear probing needs
// tombstones to support deletion, and tombstones are what make an
// open-addressed table degrade until it has to be rebuilt. A set that only ever
// grows never has one. The property that makes this set expensive is the same
// property that makes the cheap structure correct for it.
//
// THE COLLISION BOUND. A fingerprint is 128 bits, so n distinct keys collide
// with probability about n²/2^129:
//
//	n = 10^7  keys      1.5e-25
//	n = 10^9  keys      1.5e-21     (about 300 years at 3.7M records a day)
//	n = 10^10 keys      1.5e-19
//
// A 64-bit fingerprint — which is what speciesAgg.genomes uses, because a
// collision there miscounts a number on a page — would be 2.7e-2 at 10^9 keys.
// A collision HERE means treating a NEW migration as a duplicate and leaving it
// out of the permanent record. That is the whole reason there are two hashes.
//
// AND THE SEEDS ARE DRAWN PER PROCESS. The window is rebuilt from the ledger on
// every start and is never persisted, so the fingerprints only ever have to
// agree with themselves inside one run — and with the other generation, which is
// why a rotation keeps the seeds it has. A participant who chooses migrationIds
// therefore cannot search offline for a pair that collides and hold it against
// the archive: it would have to find one under a seed it cannot see, and that
// seed stops existing at the next restart.

// dedupFP is one entry: a 128-bit fingerprint of a duplicate key, as two
// independent 64-bit hashes of it. There is no pointer in it, which is what
// takes the whole set out of the garbage collector's reach.
type dedupFP [2]uint64

// dedupParts is what gets hashed, in the two halves the key is made of. Hashing
// the halves rather than the string they concatenate to is what keeps the
// replay allocation-free: the old set built that string once per record and
// then held it forever, and building one per record only to throw it away would
// make a restart five million pieces of garbage instead.
type dedupParts struct {
	typ string
	key string
}

const (
	// dedupLoadNum/dedupLoadDen is the load factor each table is kept under.
	// Linear probing costs about 2.5 probes for a hit at 3/4 and about 4.5 at
	// 7/8; the replay does one insert per ledger record, so three quarters is
	// where the probe count stops being free and the table is still dense.
	dedupLoadNum = 3
	dedupLoadDen = 4

	// dedupShardBits is why the set is SHARDED rather than one flat table, and
	// this is a lock-hold decision rather than a memory one. A rehash moves
	// every key it holds, under the same mutex the migration path takes: one
	// flat table measured 0.40 to 0.55 s to rehash at the size the current
	// ledger reaches, and 0.91 to 1.17 s at twice that. B21 is the episode where
	// 0.3 to 1.0 s of held lock per tick filled the relay's outbound queue to
	// this archive and cost 7,789 crossings the ledger will never hold — the
	// same mistake in a different function. Sixty-four independent tables make
	// one rehash 1/64 of the work, measured at 5.9 ms at today's size, and they
	// do not all cross the threshold on the same insert, so the set's memory
	// grows in sixty-four small steps instead of one doubling that briefly holds
	// both copies.
	//
	// It costs nothing in density: the shard is chosen from the TOP bits of the
	// fingerprint and the slot from the bottom bits, so a set sized from the
	// ledger lands at exactly the same total slot count a single table would.
	dedupShardBits  = 6
	dedupShards     = 1 << dedupShardBits
	dedupShardShift = 64 - dedupShardBits

	// dedupMinSlots is the floor for one shard's table, and it is small because
	// every archive in the test suite pays it.
	dedupMinSlots = 1 << 6
)

// dedupSet is the set.
//
// The zero fingerprint marks an empty slot, so the one real key that could ever
// fingerprint to it — a 2^-128 event — is held in zero instead. The structure
// is therefore exact, rather than exact with overwhelming probability.
type dedupSet struct {
	seedA  maphash.Seed
	seedB  maphash.Seed
	shards [dedupShards]dedupShard
	zero   bool
}

// dedupShard is one open-addressed table. slots is a power of two and mask is
// len(slots)-1.
type dedupShard struct {
	slots []dedupFP
	mask  uint64
	n     int
}

// newDedupSet builds the set, sized for hint keys when the caller has a usable
// estimate. Sizing it once matters twice: the replay never stops to rehash, and
// a shard that grows holds its old array and its new one at the same time.
func newDedupSet(hint int) *dedupSet {
	return newDedupSetWithSeeds(hint, maphash.MakeSeed(), maphash.MakeSeed())
}

// newDedupSetWithSeeds is the same, under seeds the caller already holds. It is
// what lets a dedupWindow rotate: two generations that hashed a key differently
// could not answer one question between them.
func newDedupSetWithSeeds(hint int, seedA, seedB maphash.Seed) *dedupSet {
	per := int64(hint) / dedupShards
	capacity := int64(dedupMinSlots)
	want := per * dedupLoadDen / dedupLoadNum
	for capacity < want && capacity < 1<<40 {
		capacity <<= 1
	}
	s := &dedupSet{seedA: seedA, seedB: seedB}
	for i := range s.shards {
		s.shards[i] = dedupShard{slots: make([]dedupFP, capacity), mask: uint64(capacity - 1)}
	}
	return s
}

// fingerprint is the hash of one duplicate key. It reads the seeds and nothing
// else, and the seeds are written once when the set is built and never again —
// which is why the caller may compute this OUTSIDE the lock that guards the
// tables, and should.
func (s *dedupSet) fingerprint(typ, key string) dedupFP {
	p := dedupParts{typ: typ, key: key}
	return dedupFP{
		maphash.Comparable(s.seedA, p),
		maphash.Comparable(s.seedB, p),
	}
}

// add records one fingerprint and reports whether it was ALREADY THERE. The
// caller holds the lock that guards the set.
func (s *dedupSet) add(fp dedupFP) bool {
	if fp == (dedupFP{}) {
		if s.zero {
			return true
		}
		s.zero = true
		return false
	}
	return s.shards[fp[0]>>dedupShardShift].add(fp)
}

// has reports whether a fingerprint is in the set, without adding it.
func (s *dedupSet) has(fp dedupFP) bool {
	if fp == (dedupFP{}) {
		return s.zero
	}
	return s.shards[fp[0]>>dedupShardShift].has(fp)
}

// len is how many distinct keys the set holds.
func (s *dedupSet) len() int {
	n := 0
	for i := range s.shards {
		n += s.shards[i].n
	}
	if s.zero {
		n++
	}
	return n
}

// slots is how many table slots the set is holding, across every shard. It is
// what the set costs, divided by 16 bytes.
func (s *dedupSet) slots() int {
	n := 0
	for i := range s.shards {
		n += len(s.shards[i].slots)
	}
	return n
}

// add walks the probe sequence until it finds the fingerprint or the first
// empty slot behind it. There is no third case: nothing is ever deleted, so no
// slot is ever a tombstone, and a walk that reaches an empty slot has seen
// every place this fingerprint could have been put.
func (d *dedupShard) add(fp dedupFP) bool {
	for i := fp[0] & d.mask; ; i = (i + 1) & d.mask {
		switch d.slots[i] {
		case fp:
			return true
		case dedupFP{}:
			d.slots[i] = fp
			d.n++
			if int64(d.n)*dedupLoadDen > int64(len(d.slots))*dedupLoadNum {
				d.grow()
			}
			return false
		}
	}
}

// has is add without the insert.
func (d *dedupShard) has(fp dedupFP) bool {
	for i := fp[0] & d.mask; ; i = (i + 1) & d.mask {
		switch d.slots[i] {
		case fp:
			return true
		case dedupFP{}:
			return false
		}
	}
}

// grow doubles one shard's table and rehashes it. The shard index comes from
// the top bits of the fingerprint and the slot from the bottom bits, so a key
// never moves between shards and this only ever has to look at its own.
func (d *dedupShard) grow() {
	old := d.slots
	d.slots = make([]dedupFP, len(old)*2)
	d.mask = uint64(len(d.slots) - 1)
	for _, fp := range old {
		if fp == (dedupFP{}) {
			continue
		}
		i := fp[0] & d.mask
		for d.slots[i] != (dedupFP{}) {
			i = (i + 1) & d.mask
		}
		d.slots[i] = fp
	}
}

// dedupWindow is the §5.1 duplicate set, bounded in time (§25, B38).
//
// IT IS TWO GENERATIONS AND A ROTATION, and the shape is chosen by what the set
// underneath it is good at. dedupSet is an open-addressed table with no
// tombstones, which is the whole reason it costs 25 bytes a key instead of 41 to
// 90; adding deletion to it would give back exactly what it bought. Expiring by
// generation keeps it: nothing is ever deleted from a generation, the oldest
// generation is DROPPED WHOLE, and the memory it held is returned in one piece.
//
//	add     inserts into cur
//	has     checks cur, then prev
//	rotate  prev = cur, cur = a fresh table, once every window
//
// WHAT THAT RETAINS. A key inserted just before a rotation survives one more
// window; one inserted just after survives two. So the guarantee is AT LEAST
// window and AT MOST twice it, and the memory is at most two windows of keys.
// Sizing the guarantee from below is what matters: a duplicate that arrives
// inside `window` of the original is always refused.
//
// WHY NOT MORE GENERATIONS. Four generations rotating at window/3 would hold at
// most 4/3 of a window instead of 2, for one more array and one more probe per
// miss. The saving is real and small, and this set is no longer the archive's
// dominant term once it is bounded at all; two generations is the version a
// reader can hold in their head.
type dedupWindow struct {
	window time.Duration
	cur    *dedupSet
	prev   *dedupSet
	// rotateAt is when cur stops taking new keys and becomes prev. It moves on
	// the clock the caller passes in, so a replay of an old ledger rotates on the
	// LEDGER's timestamps and a live archive rotates on the wall clock.
	rotateAt time.Time
	// hint sizes the next generation. It starts as the caller's estimate and
	// afterwards is whatever the generation being retired actually held, so the
	// second and every later generation is sized from measurement.
	hint int
}

// newDedupWindow builds the window. hint is the caller's estimate of how many
// keys ONE generation will hold; window is the retention floor.
func newDedupWindow(hint int, window time.Duration, now time.Time) *dedupWindow {
	if window <= 0 {
		window = 48 * time.Hour
	}
	return &dedupWindow{
		window:   window,
		cur:      newDedupSet(hint),
		hint:     hint,
		rotateAt: now.Add(window),
	}
}

// fingerprint is dedupSet's, and it is taken from the CURRENT generation's
// seeds. Both generations must agree about a key or a rotation would forget
// everything it kept, so the seeds are drawn once for the window and every
// generation is built with them.
func (w *dedupWindow) fingerprint(typ, key string) dedupFP {
	return w.cur.fingerprint(typ, key)
}

// tick rotates the window when its time has come. The caller holds the lock that
// guards the tables.
//
// A GAP OF SEVERAL WINDOWS IS ONE ROTATION, not several. An archive that was
// stopped for a week, or a replay that walks a month of ledger, must not be made
// to allocate a generation per window it slept through — and it must not keep
// one either, because every generation it slept through is empty.
func (w *dedupWindow) tick(now time.Time) {
	if now.Before(w.rotateAt) {
		return
	}
	if n := w.cur.len(); n > w.hint {
		w.hint = n
	}
	seedA, seedB := w.cur.seedA, w.cur.seedB
	if now.Sub(w.rotateAt) >= w.window {
		// Two or more rotations are due, so the generation that would have
		// become `prev` is one this window never filled. Both go.
		w.prev = nil
		w.cur = newDedupSetWithSeeds(w.hint, seedA, seedB)
		w.rotateAt = now.Add(w.window)
		return
	}
	w.prev = w.cur
	w.cur = newDedupSetWithSeeds(w.hint, seedA, seedB)
	// The grid is kept rather than restarted, so a rotation that runs late does
	// not push every later one late with it.
	w.rotateAt = w.rotateAt.Add(w.window)
}

// add records one fingerprint and reports whether it was ALREADY THERE, in
// either generation. The caller holds the lock that guards the tables.
func (w *dedupWindow) add(fp dedupFP, now time.Time) bool {
	w.tick(now)
	if w.prev != nil && w.prev.has(fp) {
		// Already known, and known from the older generation. It is deliberately
		// NOT copied forward: a key that keeps arriving is a peer that keeps
		// re-sending, and refreshing its lease would let it hold a slot for as
		// long as it kept doing so.
		return true
	}
	return w.cur.add(fp)
}

// has reports whether a fingerprint is in either generation, without adding it.
func (w *dedupWindow) has(fp dedupFP) bool {
	if w.cur.has(fp) {
		return true
	}
	return w.prev != nil && w.prev.has(fp)
}

// len is how many distinct keys the window holds, across both generations. A key
// in both is counted twice, which is the honest answer to "how many keys is this
// paying for".
func (w *dedupWindow) len() int {
	n := w.cur.len()
	if w.prev != nil {
		n += w.prev.len()
	}
	return n
}

// slots is what the window costs, divided by 16 bytes.
func (w *dedupWindow) slots() int {
	n := w.cur.slots()
	if w.prev != nil {
		n += w.prev.slots()
	}
	return n
}

// dedupHint estimates how many duplicate keys a ledger of this many bytes
// holds. It is a stat and not a second pass over the file: the replay is the
// only full read of the ledger there is, and adding a counting pass to size a
// table would double the one cost a restart is measured on.
//
// The reference workload is 337 bytes per record (deploy/SIZING.md) and 2
// records in every 2.14 carry a migrationId — one MIGRATION and one ACK per
// crossing, while a GENOME record carries none — so one duplicate key per about
// 360 bytes of ledger. The 2026-08-16 production copy measured 349.5 bytes of
// ledger per key, so the estimate lands about 3% LOW, which is the direction
// that costs nothing: it still sizes every shard clear of its own threshold.
//
// AN ESTIMATE IS ENOUGH BECAUSE BOTH ERRORS ARE CHEAP. Too low costs a shard or
// two a rehash near the end of the replay; too high rounds each shard's table
// up to the next power of two and reserves slots the run never fills. Neither
// is a cliff, and neither can make an answer wrong.
func dedupHint(ledgerBytes int64) int {
	const bytesPerDedupKey = 360
	if ledgerBytes <= 0 {
		return 0
	}
	return int(ledgerBytes / bytesPerDedupKey)
}
