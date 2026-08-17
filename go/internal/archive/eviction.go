package archive

// THE RETENTION HORIZON. Decision 3, answered by the owner on 2026-08-12, and
// contract-b-m4.md §23, B33 and B34.
//
// The archive kept everything, on purpose and by contract: §10, §20 B20 and
// §22 B27 all say that nothing evicts from the ledger or the genome store, and
// §10 says in as many words that a retention rule contradicting that is a
// change to D11 rather than a configuration of it. The owner made exactly that
// change, on exactly one of the two halves:
//
//	THE LEDGER IS KEPT FOREVER. The genome BLOBS are pruned to a horizon.
//
// So this file evicts blobs and never records. The lineage graph keeps every
// node, every crossing, every hash and every GENOME line naming who served what
// — the record of a blob outlives the blob — and what ages out is the bytes.
// Risk 7 becomes a stated policy instead of an accident: a genome nobody
// fetched inside the horizon becomes permanently unfetchable, and the archive
// says so in a counter rather than discovering it in M7.
//
// IT IS OFF BY DEFAULT, and that is the contract's default and not a courtesy.
// GenomeHorizon of 0 means every path here returns before it does anything:
// no goroutine, no cursor, no retirement, no counter. An archive that is not
// told a horizon behaves exactly as it did in M4, which is what makes the rest
// of the suite the proof of that claim. The M5 hosted deployment sets 720h.
//
// TWO MECHANISMS, ONE HORIZON, AND THEY ARE NOT SEPARABLE (§23, B34). The
// eviction pass deletes blobs older than the horizon; the fetch queue retires
// gaps whose CROSSING is older than the horizon. Run either alone and the
// archive fights itself — a queue with no horizon re-fetches a blob the next
// pass deletes, forever, and a horizon with no queue rule leaves §21's backlog
// with no drain at all at the ×100 crossing rate. Run both on one number and
// the queue finally has a bottom: work whose only possible outcome is a blob
// the store must immediately delete is work that stops.
//
// THE GAP'S CLOCK IS THE CROSSING'S recordedAt, not the moment this process
// first noticed the gap. fetch.firstSeen is taken from the replay's clock, so
// it resets at every restart, and a durable retention rule may not be built on
// a value that process uptime moves. The ledger already records recordedAt on
// every migration; that is the crossing's own time and it survives everything.

import (
	"time"

	"multiverse/internal/bb8"
)

// The pass's bounds, in the spirit of §21 B21's: a pass costs what the archive
// chose, never what the store grew to. At one pass a minute the cursor crosses
// all 256 shards in about half an hour, which is a rounding error against a
// horizon measured in days, and the removal budget clears far more per minute
// than the deployment's measured fetch rate puts in.
const (
	defaultEvictionInterval = time.Minute
	evictShardsPerPass      = 8
	evictExaminePerPass     = 4096
	evictRemovePerPass      = 1024
	evictRemoveChunk        = 64
)

// evictState is the horizon's whole accounting. It lives under Archive.mu with
// the rest of the counters the status view reads.
type evictState struct {
	cursor bb8.EvictCursor
	passes int
	// evicted and bytes are what this process has pruned. They are a process
	// total like every other counter on the status page, and they are the
	// operator's only evidence that a knob measured in days is doing anything.
	evicted int
	bytes   int64
	// scratch counts abandoned <hash>.json.tmp files collected on the way past
	// (§20, B20). The archive's store never had a sweep to collect them.
	scratch int
	// gapsExpired counts gap entries retired from retry because their crossing
	// is older than the horizon — the ones a running pump retires, the ones a
	// replay declines to queue at all, and the ones a restored queue is drained
	// of at load.
	//
	// IT IS ALL-TIME AND PERSISTED (rollup.go's floor line), unlike the three
	// counters above it. It had to become so: it used to count what THE REPLAY
	// declined, which made it a fact about how much raw a restart happened to
	// read rather than about the record, and it was the only published number
	// that moved when the raw scan got shorter.
	gapsExpired int
	lastAt      time.Time
}

// horizonOn reports whether this archive prunes anything at all.
func (a *Archive) horizonOn() bool { return a.cfg.GenomeHorizon > 0 }

// startEviction launches the pass when a horizon is configured, and does
// nothing at all when one is not. It is called from Start.
func (a *Archive) startEviction() {
	if !a.horizonOn() {
		return
	}
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.evictionLoop() }()
}

func (a *Archive) evictionLoop() {
	interval := a.cfg.EvictionInterval
	if interval <= 0 {
		interval = defaultEvictionInterval
	}
	a.log.Warn("archive: the genome retention horizon is ON; blobs older than it are PRUNED",
		"horizon", a.cfg.GenomeHorizon.String(), "pass", interval.String(),
		"store", a.genomes.Dir(),
		"scope", "the content-addressed genome store ONLY",
		"note", "this pass NEVER removes a migration ledger line (D11, contract-b-m4.md §10, "+
			"§23 B33); the record of what crossed is kept forever, and the ledger's own window "+
			"is a separate knob (§26, B40). A pruned hash stays a lineage node and answers "+
			"exactly like a hash no peer ever served")
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case now := <-t.C:
			a.evictPass(now)
		}
	}
}

// evictPass runs one bounded, resumable sweep of the genome store.
//
// NO FILE IS TOUCHED UNDER THE ARCHIVE'S OWN LOCK. The lock is taken twice, to
// read the cursor and to fold the result, and the walk happens between them —
// the same rule §10.1 states for the history strip and §21 B21 states for the
// pump, and for the same reason: nothing that reads a directory may hold the
// lock the relay read loop needs.
func (a *Archive) evictPass(now time.Time) bb8.EvictResult {
	if !a.horizonOn() {
		return bb8.EvictResult{}
	}
	a.mu.Lock()
	cur := a.evict.cursor
	a.mu.Unlock()

	res, err := a.genomes.EvictOlderThan(now.Add(-a.cfg.GenomeHorizon), cur, bb8.EvictBudget{
		Shards:  evictShardsPerPass,
		Examine: evictExaminePerPass,
		Remove:  evictRemovePerPass,
		Chunk:   evictRemoveChunk,
	})

	a.mu.Lock()
	a.evict.cursor = res.Cursor
	a.evict.passes++
	a.evict.evicted += res.Removed
	a.evict.bytes += res.Bytes
	a.evict.scratch += res.Scratch
	a.evict.lastAt = now
	totalEvicted, totalBytes, totalGaps := a.evict.evicted, a.evict.bytes, a.evict.gapsExpired
	a.mu.Unlock()

	if err != nil {
		a.log.Warn("archive: the genome eviction pass failed", "err", err,
			"horizon", a.cfg.GenomeHorizon.String())
	}
	if res.Removed > 0 || res.Scratch > 0 {
		a.log.Info("archive: pruned genome blobs past the retention horizon",
			"removed", res.Removed, "bytes", res.Bytes, "scratchCollected", res.Scratch,
			"examined", res.Examined, "horizon", a.cfg.GenomeHorizon.String(),
			"totalRemoved", totalEvicted, "totalBytes", totalBytes,
			"gapsRetired", totalGaps,
			"ledger", "untouched by this pass: the record of what crossed is kept forever "+
				"and the ledger's window is a separate knob (§23, B33; §26, B40)")
	}
	return res
}

// gapPastHorizonLocked reports whether a gap for a crossing recorded at
// crossedAt has aged past the horizon. A zero crossedAt — a record written
// before the field existed, or a caller with nothing to offer — is never past
// it: a missing timestamp is not evidence of age.
func (a *Archive) gapPastHorizonLocked(crossedAt, now time.Time) bool {
	if !a.horizonOn() || crossedAt.IsZero() {
		return false
	}
	return now.Sub(crossedAt) > a.cfg.GenomeHorizon
}

// retireGapLocked drops one aged gap out of the fetch queue and counts it.
//
// The hash is NOT forgotten — §10's "keep the hash forever" is a rule about the
// ledger and it is untouched. The record still names it, the gap report still
// lists it, and `archive list --gaps` still shows the crossing as missing its
// genome. What ends is the asking, because the answer could no longer be kept.
func (a *Archive) retireGapLocked(f *fetch, now time.Time) {
	a.clearInFlightLocked(f)
	delete(a.pending, f.hash)
	// The queue is durable now (rollup.go): a retirement that left the entry in
	// the sidecar would be undone by the next restart, and the counter beside it
	// would climb by the same entry every time.
	a.markGapGoneLocked(f.hash)
	a.evict.gapsExpired++
	a.rollupDirty.counts = true
	// Debug, not Info, and the pass's summary line carries the total. §21's
	// incident included 231 log lines from one pass over a backlog; a retirement
	// that logs per entry would produce one per gap at exactly the moment a
	// 64,000-entry queue finally drains.
	a.log.Debug("archive: retiring a genome gap past the retention horizon",
		"genomeHash", f.hash, "crossedAt", f.crossedAt.UTC().Format(time.RFC3339),
		"attempts", f.attempts, "horizon", a.cfg.GenomeHorizon.String())
}

// logRetiredAtStartup says how much of the replayed backlog was never queued.
// It is called once, from New, and only when the horizon is on.
func (a *Archive) logRetiredAtStartup() {
	if !a.horizonOn() || a.evict.gapsExpired == 0 {
		return
	}
	a.log.Warn("archive: gaps whose crossing is older than the retention horizon were NOT re-queued",
		"gaps", a.evict.gapsExpired, "horizon", a.cfg.GenomeHorizon.String(),
		"note", "their hashes are still in the ledger and still in the gap report; what stopped "+
			"is the retrying (contract-b-m4.md §23, B34)")
}

// EvictionCounters is the horizon's accounting, for tests and for the status
// view. Zero everywhere on an archive with no horizon.
func (a *Archive) EvictionCounters() (evicted int, bytes int64, gapsExpired int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.evict.evicted, a.evict.bytes, a.evict.gapsExpired
}
