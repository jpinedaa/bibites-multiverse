package archive

// The BRAIN SHAPE the genealogy draws on each species, and the one place this
// archive reads a stored genome for anything other than serving it back.
//
// WHERE IT COMES FROM. Every migration record carries a genomeHash beside the
// species block, and the archive already fetches and stores the blob behind that
// hash in its content-addressed store (bb8.Store, §10). species.go keeps the
// LATEST such hash per species — one string, maintained by the same single
// writer as every other counter there — so naming a genome for a species costs
// nothing at request time. This file turns that one hash into two numbers.
//
// AND THE ANSWER IS NOW KEPT. The measurement used to live only in the two maps
// below, which is to say only for as long as this process ran, and that made
// three defects out of one design:
//
//	A RESTART EMPTIED THE PICTURE. Against the running rig's ~160 000-entry
//	fetch backlog, 2 of 14 drawn rows carried a ring twelve minutes after a
//	restart and 5 of 13 after hours of uptime — because the ring depended on
//	this process having happened to read that species' genome since it booted.
//
//	AN EXTINCT ANCESTOR COULD NEVER GAIN ONE. Nothing is fetching a dead
//	species' genomes any more, so the blob either arrived long ago (and the
//	reading died with the last restart) or never will. The dimmed ancestor rows
//	— the branch points, which are the whole reason the tree has interior nodes
//	— were structurally ringless.
//
//	AND THE HORIZON MADE IT PERMANENT. Once an extinct species' blobs age past
//	the 720 h retention horizon (§23, B33) the measurement is unobtainable
//	forever, however long the archive runs.
//
// So the last measured shape of each species is PERSISTED (brainhist.go,
// brainsave.go), folded at the same two write points as the time series, and
// this file answers from it. A MEASUREMENT NOW OUTLIVES THE BLOB IT WAS READ
// FROM, which is the same argument as capturing at arrival: the horizon prunes
// the genome and does not prune the number.
//
// THAT CHANGES WHAT THE RING MEANS, and the page's words are changed with it.
// The ring is no longer "the newest genome of that species this archive HOLDS A
// COPY OF"; it is "the newest genome of it this archive EVER READ", which for a
// long-extinct species may be days old and may describe a genome that no longer
// exists anywhere. The measurement's own age travels with it so the row can say
// so, and the tooltip says it in words.
//
// THREE RULES, and they are still the whole design:
//
//  1. ONE PARSE PER HASH, EVER, NOT ONE PER REQUEST. The genealogy is polled
//     every two seconds while its tab is open. A parse per poll would put a file
//     read and a JSON decode per species on a two-second cadence forever, which
//     is the shape species.go rule 1 refuses for the ledger and this file has no
//     business reintroducing for the genome store. The cache below is keyed on
//     the hash, which is CONTENT — the same hash is the same bytes is the same
//     answer, for as long as this process runs.
//
//  2. AN ABSENT ANSWER IS AN ANSWER, AND NEVER AN ERROR. The blob may have been
//     pruned by the retention horizon (§23, B33), may never have arrived (a
//     genome gap), or may be a shape this parser cannot read. All three render as
//     NOTHING on the page — no figure, no zero, no error banner — because a brain
//     this archive cannot see is not a brain of size zero, and a view that failed
//     because a genome aged out would be a view that breaks by design. The MISS
//     IS CACHED TOO, which is what stops a pruned hash costing a stat() every
//     two seconds. A species this archive has NEVER read a genome for still draws
//     no ring, persisted record or not.
//
//  3. IT NEVER TOUCHES THE LEDGER. Everything here is a map lookup plus, at most
//     once per hash, one read of one small file the store already holds.

import (
	"sync"

	"multiverse/internal/bb8"
)

// brainCacheMax bounds the cache. It is keyed on genome hashes, and a species'
// latest hash moves whenever a new genome of it crosses — so this grows with
// TIME as well as with species, and is the one thing here that needs a bound.
// The reduced tree holds at most treeNodeMax nodes, so this is several turnovers
// of the whole drawn set.
const brainCacheMax = 2048

type brainCache struct {
	mu sync.Mutex
	// byHash holds both answers and misses; known says which.
	byHash map[string]brainEntry
	// order is insertion order, for the oldest-out eviction. A cache with no
	// eviction is the unbounded thing this bound exists to prevent.
	order []string
}

type brainEntry struct {
	brain bb8.Brain
	known bool
}

// brainFor returns the brain shape behind one genome hash, parsing it at most
// once. The bool is false for every absence in rule 2, and the caller draws
// nothing.
func (a *Archive) brainFor(hash string) (bb8.Brain, bool) {
	if hash == "" || a.genomes == nil {
		return bb8.Brain{}, false
	}
	a.brains.mu.Lock()
	if e, ok := a.brains.byHash[hash]; ok {
		a.brains.mu.Unlock()
		// A CACHED MISS on a cold hash still asks for a restore. Without this, the
		// very first poll caches the miss and no later poll ever re-triggers the
		// restore — a genome that failed to restore once (cold copy briefly down,
		// the worker's buffer full) would stay invisible forever. The restore queue
		// is a dedup set, so re-asking every poll is free until the blob is home,
		// at which point doColdRestore invalidates this entry.
		if !e.known && a.cold.has(hash) {
			a.enqueueColdRestore(hash)
		}
		return e.brain, e.known
	}
	a.brains.mu.Unlock()

	// The read and the parse happen OUTSIDE the cache lock and outside a.mu: a
	// reader of this view must never make the migration path wait, and two
	// readers racing on the same cold hash cost one duplicated parse rather than
	// a queue.
	entry := brainEntry{}
	if stored, ok := a.genomes.Get(hash); ok {
		entry.brain, entry.known = bb8.BrainStats(stored.BB8)
	} else if a.cold.has(hash) {
		// The hot store has aged this blob out to cold. Ask for it back in the
		// background; until it lands the answer is ABSENT (rule 2), and the restore
		// invalidates this cached miss when the bytes are home. The queue dedups, so
		// a two-second poll enqueues one restore rather than one per poll.
		a.enqueueColdRestore(hash)
	}

	a.brains.mu.Lock()
	defer a.brains.mu.Unlock()
	a.putBrainLocked(hash, entry)
	return entry.brain, entry.known
}

// invalidateBrainMiss drops a CACHED MISS for one hash, so the next brainFor
// re-reads the store. It is called by the cold-restore worker when a blob comes
// home: without it, a restored genome would stay invisible on the genealogy until
// the bounded cache turned over. A cached HIT is left alone — the bytes have not
// changed, so the parse is still right.
func (a *Archive) invalidateBrainMiss(hash string) {
	if hash == "" {
		return
	}
	a.brains.mu.Lock()
	defer a.brains.mu.Unlock()
	if e, ok := a.brains.byHash[hash]; ok && !e.known {
		delete(a.brains.byHash, hash)
		for i, h := range a.brains.order {
			if h == hash {
				a.brains.order = append(a.brains.order[:i], a.brains.order[i+1:]...)
				break
			}
		}
	}
}

// fillBrainCache records a shape read from bytes the process was already holding
// — the fetch that has just landed. It is what stops rule 2's cached MISS
// outliving the arrival it was waiting for: without it, a hash looked up while
// its blob was still outstanding stays a miss until the cache turns over, and
// the genealogy stays on an older genome of that species for no reason but the
// order two things happened in.
func (a *Archive) fillBrainCache(hash string, b bb8.Brain) {
	if hash == "" {
		return
	}
	a.brains.mu.Lock()
	defer a.brains.mu.Unlock()
	a.putBrainLocked(hash, brainEntry{brain: b, known: true})
}

// putBrainLocked inserts or replaces one entry, oldest-out at the bound. The
// caller holds the cache lock.
func (a *Archive) putBrainLocked(hash string, e brainEntry) {
	if a.brains.byHash == nil {
		a.brains.byHash = map[string]brainEntry{}
	}
	if _, ok := a.brains.byHash[hash]; !ok {
		if len(a.brains.order) >= brainCacheMax {
			oldest := a.brains.order[0]
			a.brains.order = append(a.brains.order[:0], a.brains.order[1:]...)
			delete(a.brains.byHash, oldest)
		}
		a.brains.order = append(a.brains.order, hash)
	}
	a.brains.byHash[hash] = e
}

// brainForSpecies is what the genealogy actually draws: THE NEWEST GENOME OF
// THIS SPECIES THIS ARCHIVE HAS EVER READ, with the crossing it was read from.
//
// ONE SOURCE, AND IT IS THE PERSISTED RECORD. The species' latest named hash is
// still parsed when the store holds it — that is the freshest reading there can
// be — but the reading is CONTRIBUTED to the record rather than returned
// directly, and the record is what answers. Two things follow, and both are the
// point:
//
//	THE RING SURVIVES A RESTART, and an extinct ancestor keeps the last brain
//	this archive ever managed to read for it, forever.
//
//	AND OUT-OF-ORDER READINGS CANNOT GO BACKWARDS. The record is latest-writer-
//	wins on the CROSSING's clock (brainhist.go), so a blob fetched today for a
//	crossing three days old never displaces a newer measurement, whichever order
//	the two were read in.
//
// WHY IT IS NOT "THE LATEST HASH OR NOTHING". The hash species.go keeps moves
// with every crossing, and on a rig carrying a genome-fetch backlog — 154 000
// gaps when this was measured — an actively-travelling species rotates its latest
// hash faster than the archive fetches the blob behind it. Ring membership then
// changed from poll to poll and the mark BLINKED, which reads as a brain
// appearing and disappearing rather than as a fetch that has not landed yet.
//
// A MISS ON BOTH IS STILL AN ABSENCE, and an absence still draws nothing (rule
// 2). Nothing here invents a shape, and nothing here holds one for a species this
// archive has never once read a readable genome of.
//
// The int64 is WHEN the returned measurement was recorded — the crossing its
// genome was named on, not the moment it was read — because a measurement that
// outlives its blob can be days old, and a row that showed it beside a living
// species' current one without saying which was which would invite exactly the
// wrong comparison.
func (a *Archive) brainForSpecies(key, hash string, hashAtMs int64) (bb8.Brain, int64, bool) {
	if key == "" {
		// No species to file a reading under. The hash alone still answers, for a
		// caller that has one — but nothing is remembered, because a record with
		// no key is a record nothing could ever read back.
		b, ok := a.brainFor(hash)
		return b, 0, ok
	}
	if b, ok := a.brainFor(hash); ok && hashAtMs > 0 {
		a.observeSpeciesBrain(key, hashAtMs, hash, b)
	}
	r, ok := a.brainAgg.record(key)
	if !ok {
		return bb8.Brain{}, 0, false
	}
	return bb8.Brain{Neurons: r.Neurons, Synapses: r.Synapses}, r.AtMs, true
}
