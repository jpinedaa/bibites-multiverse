package archive

// The recent-hops feed of contract-b-m4.md §17, B14 (D19).
//
// The page already draws an ambient pulse per lane, paced from LaneView's
// RecentHops. That says HOW FAST a lane is running. It cannot say WHO just
// crossed it, and the feed below is what does: lane, endpoints, timestamp, and
// the species block the envelope carried, so the page can animate that species'
// own glyph travelling the lane's arrow from one world to the next.
//
// FOUR RULES, and every one of them is load-bearing:
//
//  1. ONE SOURCE, NO POLLING. It is built from the MIGRATION_PAYLOAD copies the
//     archive already receives as a subscriber (§5.1). Nothing on the migration
//     path ever waits for a reader (Risk 4): observeHopLocked runs inside the
//     lock onMigration already holds and appends to a slice.
//
//  2. BOUNDED TWICE — BY TIME AND BY COUNT, not either. See below for why the
//     count bound is not belt-and-braces.
//
//  3. IT IS LEDGER, NOT CENSUS. The feed answers *what crossed*. It is never
//     summed into an abundance claim and never joined to the census: a database
//     built from migrations holds migrants and their ancestors, never a resident
//     population (D11, §10.1, B12). The glyph on the lane says "this one
//     crossed"; the glyphs in a cell say "these live here".
//
//  4. AN ABSENT SPECIES BLOCK IS THE NEUTRAL GLYPH. Never a guessed name, never
//     omitted, never "unknown" as a species VALUE — §10.1's unknown rule applied
//     to a new view rather than bent for it. That is why Species is a pointer
//     that stays nil and never a placeholder string.
//
// WHY IT IS NOT A FIELD ON Status, which is the design decision this file
// exists to record. Status is serialized VERBATIM into metrics.jsonl once a
// minute, forever, and that file is the thing an operator tails and jq's months
// later. Hanging a per-organism feed off it would write every hop to disk twice
// — once in the ledger, where it belongs and is durable, and again inside a
// status sample, where it is neither. So the feed gets its OWN endpoint,
// /api/hops, its own view type, and no route into MetricsLog.Append at all. The
// count bound then protects a live reader's payload rather than the disk, and
// the two bounds have genuinely different jobs.

import (
	"time"

	"multiverse/internal/contractb"
)

// hopWindow is how far back the feed reaches. The page polls every two seconds
// and animates a hop once, so the window only has to cover a reader that
// blinked, a tab that was backgrounded, or a poll that lost a round trip.
const hopWindow = 60 * time.Second

// hopMax is the count bound. At the measured peak of ~18 arrivals per simulated
// minute across six slots the window holds well under this; hopMax is what
// stops a genuine dam — a far-end dropout releasing a two-hour backlog — from
// putting thousands of entries in one HTTP response.
const hopMax = 200

// Hop is one recorded migration as the feed reports it. It carries the two
// endpoints AND the edge, because under two-way lanes the pair (fromSlot,
// toSlot) no longer names one lane: on an axis of length 2 the forward and
// reverse lanes join the SAME two worlds, and the page has to know which of the
// two arrows to animate.
type Hop struct {
	MigrationID string `json:"migrationId"`
	AtMs        int64  `json:"atMs"`
	FromSlot    int    `json:"fromSlot"`
	ToSlot      int    `json:"toSlot"`
	// ExitEdge is the edge the organism LEFT its own world by, which is the lane
	// key the page animates: lanes are keyed fromSlot+edge everywhere on this
	// surface. All four values occur under D17.
	ExitEdge string `json:"exitEdge"`
	// Species is the block the envelope carried, recorded and never resolved
	// (§15, B10). NIL IS THE ANSWER when the envelope carried none, and the page
	// draws the neutral glyph for it.
	Species *contractb.Species `json:"species,omitempty"`
}

// HopFeed is /api/hops. It states its own bounds, because a feed whose limits a
// reader cannot see is a feed a reader will mistake for a total.
type HopFeed struct {
	GeneratedAtMs int64 `json:"generatedAtMs"`
	// WindowMs and MaxEntries are rule 2, published. A client that wants to say
	// "12 hops in the last minute" can; one that wants to say "12 hops" cannot.
	WindowMs   int64 `json:"windowMs"`
	MaxEntries int   `json:"maxEntries"`
	// Truncated is true when the count bound cut the window short — the honest
	// statement that the feed is a SAMPLE of the last minute and not all of it.
	Truncated bool `json:"truncated"`
	// Hops is oldest first, so a page can animate them in the order they
	// happened. Never nil: an empty feed is an empty array, not a null.
	Hops []Hop `json:"hops"`
}

// observeHopLocked appends one crossing to the feed and trims it. The caller
// holds a.mu, and this is the only writer.
func (a *Archive) observeHopLocked(h Hop) {
	a.hops = append(a.hops, h)
	a.trimHopsLocked(h.AtMs)
}

// trimHopsLocked applies BOTH bounds, time first and then count. Time first
// matters: it means the count bound only ever fires during a burst, so a
// long-quiet archive is not holding hopMax entries from yesterday.
func (a *Archive) trimHopsLocked(nowMs int64) {
	cut := nowMs - hopWindow.Milliseconds()
	i := 0
	for i < len(a.hops) && a.hops[i].AtMs < cut {
		i++
	}
	if i > 0 {
		a.hops = append(a.hops[:0], a.hops[i:]...)
	}
	if n := len(a.hops); n > hopMax {
		// Keep the NEWEST. An animation of what just happened is worthless if the
		// entries it drops are the recent ones.
		a.hops = append(a.hops[:0], a.hops[n-hopMax:]...)
		a.hopsTruncated = true
	}
}

// HopFeedView is the read path. It holds the archive's lock for a slice copy
// and touches nothing on the migration path.
func (a *Archive) HopFeedView() HopFeed {
	nowMs := time.Now().UnixMilli()
	a.mu.Lock()
	defer a.mu.Unlock()
	// Trim on read as well as on write: a lane that stopped an hour ago must
	// report an empty feed, not the last thing it carried.
	truncatedBefore := a.hopsTruncated
	a.hopsTruncated = false
	a.trimHopsLocked(nowMs)
	out := HopFeed{
		GeneratedAtMs: nowMs,
		WindowMs:      hopWindow.Milliseconds(),
		MaxEntries:    hopMax,
		Truncated:     truncatedBefore || a.hopsTruncated,
		Hops:          append(make([]Hop, 0, len(a.hops)), a.hops...),
	}
	a.hopsTruncated = false
	return out
}
