package relay

// B29's broadcast bound: the coalescing window that WIDENS under churn
// (contract-b-m4.md §7.2 *No storm*, §12, §22 B29).
//
// WHY A FIXED FLOOR STOPS BEING A BOUND. §7.2 has always required at most one
// PEER_STATUS broadcast per statusCoalesceMs, and on a six-slot map that was
// the whole answer. A broadcast costs `slotCount` stats blocks, so BOTH TERMS
// OF THE COST GROW WITH THE MAP: a public map with forty peers pays forty
// blocks per frame and can be made to pay them four times a second by one
// flapping link. B29's answer is to bound the RATE rather than the spacing —
// the window doubles from statusCoalesceMs toward statusCoalesceMaxMs while a
// window sees more than statusChurnBurstThreshold REGISTRY changes, and narrows
// one step after a quieter one.
//
// THE ARITHMETIC IS THE POINT, and it is arithmetic a reader can check without
// running anything: 60000/statusCoalesceMs broadcasts a minute at the floor,
// 60000/statusCoalesceMaxMs under a storm. 240 and 30 on the shipped defaults.
//
// WHAT COUNTS AS A CHANGE. §12 says "registry changes inside one window", and
// the two words are load-bearing. A placement, a release, a handover, a peer
// arriving, a peer going dark, an eviction and a refusal written to a slot are
// registry changes: they widen the window and they schedule a broadcast.
//
// A STATS BLOCK ARRIVING ON A PING IS NEITHER (amended — §24, B36). It changes
// what the next broadcast SAYS and not what the map IS. It never widened the
// window, for the reason above — a bound that fired on a quiet map's heartbeat
// is not a bound on anything — and since §24 it does not schedule a broadcast
// either: §6.5 gives stats the statsBroadcastIntervalMs timer, that timer was
// always going to carry them (§14, B4), and scheduling on the arrival as well
// turned seven peers' unaligned PINGs into 1.32 broadcasts a second of the whole
// map. The two counters below therefore hold the same number today; §7.2's
// "must be published" and B29's "may widen" remain two rules and keep two.
//
// THE LAST FRAME OF A BURST IS ALWAYS SENT. Coalescing may drop intermediate
// states; it MUST NOT drop the last one, because every one of these messages is
// full state and the last one is the truth. The pending counter is what
// guarantees it: it is cleared only by a publish that actually happened, so no
// widening, no narrowing and no timer reset can lose the final state.

import (
	"time"

	"multiverse/internal/contractb"
)

// churnWindow is B29's adaptive coalescing window. It is not safe for
// concurrent use; the publish loop owns it and nothing else touches it.
type churnWindow struct {
	// floor is statusCoalesceMs and ceiling is statusCoalesceMaxMs.
	floor   time.Duration
	ceiling time.Duration
	// threshold is statusChurnBurstThreshold: MORE than this many registry
	// changes inside one window widens it.
	threshold int

	cur time.Duration
}

func newChurnWindow(floor, ceiling time.Duration, threshold int) *churnWindow {
	if floor <= 0 {
		// A zero floor would be a timer that fires continuously, which is a
		// publisher that never coalesces and a relay that spins. New() already
		// applies the shipped default, so this is the belt on that braces.
		floor = contractb.StatusCoalesce
	}
	if ceiling < floor {
		// A ceiling below the floor would make "widen" mean "narrow". An
		// operator who sets one is told nothing here and gets the floor, which is
		// the pre-B29 behaviour and the safe reading of a contradictory pair.
		ceiling = floor
	}
	if threshold < 1 {
		threshold = 1
	}
	return &churnWindow{floor: floor, ceiling: ceiling, threshold: threshold, cur: floor}
}

// current is the window the next round will wait.
func (w *churnWindow) current() time.Duration { return w.cur }

// observe closes a window that saw `changes` registry changes and returns the
// width of the next one. MORE than the threshold doubles, up to the ceiling;
// anything else narrows one step, down to the floor.
//
// The ladder is symmetric on purpose — doubling up and halving down — so a
// storm that ends is paid for once rather than for as long as it lasted. A
// window that only ever widened would leave a quiet map publishing at the
// ceiling for the rest of the process's life, and an operator reading a
// two-second-old map would have no way to tell that from a hung publisher.
func (w *churnWindow) observe(changes int) time.Duration {
	switch {
	case changes > w.threshold && w.cur < w.ceiling:
		w.cur *= 2
		if w.cur > w.ceiling {
			w.cur = w.ceiling
		}
	case changes <= w.threshold && w.cur > w.floor:
		w.cur /= 2
		if w.cur < w.floor {
			w.cur = w.floor
		}
	}
	return w.cur
}

// maxBroadcastsPerMinute is the ceiling B29 asks a reader to be able to check:
// the rate this relay can broadcast at with its window at the floor. It is what
// the startup line prints and what the churn harness asserts against.
func maxBroadcastsPerMinute(floor time.Duration) float64 {
	if floor <= 0 {
		return 0
	}
	return float64(time.Minute) / float64(floor)
}

// claimShape is the seven fields B29's second rule compares. A SECTOR_CLAIM
// answered reason:"updated" whose shape is unchanged MUST NOT raise the epoch
// or trigger a broadcast — THE CLAIMANT IS STILL OWED ITS SECTOR_GRANT, and
// what the relay stops doing is telling everybody else.
//
// THIS IS THE EPOCH RATE'S MEASURED CAUSE. The living deployment's slot 6
// issued 64 placement claims in one day against two or three from each local
// slot, every one a re-claim with reason:"updated" as its measured time scale
// wandered (m5_considerations.md, DQ3). Not a reconnect and not a defect — 64
// epochs, on a six-slot map, from one peer, for nothing. On a public map that
// term is multiplied by peer count on both sides: more claimants, and more
// stats blocks in every frame each one provokes.
//
// simulationSize is compared with SameSize's relative epsilon rather than with
// ==, because §4.1 forbids exact float equality on this wire and a peer
// republishing the same size as a slightly different float is precisely the
// wandering-measurement case this rule exists for.
type claimShape struct {
	slot         int
	col, row     int
	exportEdges  string
	borderEdges  string
	modConnected bool
	gameVersion  string
	simSize      float64
	// refusal is an EIGHTH field and it is not one of B29's seven. It is here
	// because §6.5 publishes lastRefusal per slot and a claim that CLEARS one is
	// a change every reader can see: a peer that was refused on version grounds
	// and is now accepted must not stay refused on the map until the next stats
	// timer. Every ambiguity in this comparison resolves toward broadcasting.
	refusal string
}

// same answers whether nothing structural moved.
func (c claimShape) same(o claimShape) bool {
	return c.slot == o.slot && c.col == o.col && c.row == o.row &&
		c.exportEdges == o.exportEdges && c.borderEdges == o.borderEdges &&
		c.modConnected == o.modConnected && c.gameVersion == o.gameVersion &&
		c.refusal == o.refusal && sameSize(c.simSize, o.simSize)
}

// edgeKey flattens an edge list for comparison. The ORDER IS SIGNIFICANT and
// deliberately so: exportEdges is published verbatim on PEER_STATUS (§6.5), so
// a peer that reordered its list changed the frame and is owed a broadcast.
func edgeKey(edges []string) string {
	if len(edges) == 0 {
		return ""
	}
	out := edges[0]
	for _, e := range edges[1:] {
		out += "," + e
	}
	return out
}
