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
// THREE RULES, and they are the whole design:
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
//     two seconds.
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
	}

	a.brains.mu.Lock()
	defer a.brains.mu.Unlock()
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
	a.brains.byHash[hash] = entry
	return entry.brain, entry.known
}
