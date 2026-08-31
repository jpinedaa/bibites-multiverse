package archive

// The delivery-confirmed recent-hops feed of contract-b-m4.md §17, B14 (D19).
//
// The page already labels every lane with its measured rate, from LaneView's
// PerMinute. That says HOW FAST a lane is running. It cannot say WHO just
// arrived, and the feed below is what does: lane, endpoints, timestamp, and the
// species block the envelope carried, so the page can animate that species' own
// glyph travelling the lane's arrow from one world to the next.
//
// FOUR RULES, and every one of them is load-bearing:
//
//  1. ONE SOURCE, NO POLLING. It correlates the MIGRATION_PAYLOAD and
//     MIGRATION_ACK copies the archive already receives as a subscriber (§5.1).
//     An offer is not an arrival: population admission can NACK it and the same
//     migrationId can be rerouted. Nothing on the migration path ever waits for
//     a reader (Risk 4); this is archive-local observation only.
//
//  2. BOUNDED TWICE — BY TIME AND BY COUNT, not either. See below for why the
//     count bound is not belt-and-braces.
//
//  3. IT IS DELIVERY, NOT CENSUS. The feed answers *what a receiver acknowledged
//     spawning*. It is never
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

// Correlation is deliberately much larger and longer-lived than the public
// feed. A receiver can accept custody and then pace a deep queue for well over
// an hour before its mod acknowledges the spawn. Neither a silent peer nor a
// defective stream may grow archive memory without bound, so attempts and the
// rare ACK observed before its payload copy are capped as well as aged.
const (
	hopCorrelationWindow = 24 * time.Hour
	hopPendingMax        = 8192
	hopEarlyAckMax       = 1024
	// The protocol default permits twenty-four reroutes (§34 B50), but an
	// operator can raise it and a hostile copied stream must still not grow one
	// pending map value without bound. This is a visualization sample, so
	// retain a generous chain and say when it was cut.
	hopRefusedMax = 64
)

type pendingHop struct {
	hop      Hop
	destPeer string
	seenAtMs int64
}

type earlyHopAck struct {
	sourcePeer string
	seenAtMs   int64
}

// Hop is one acknowledged delivery as the feed reports it. It carries the two
// endpoints AND the edge, because under two-way lanes the pair (fromSlot,
// toSlot) no longer names one lane: on an axis of length 2 the forward and
// reverse lanes join the SAME two worlds, and the page has to know which of the
// two arrows to animate.
type Hop struct {
	MigrationID string `json:"migrationId"`
	AtMs        int64  `json:"atMs"`
	FromSlot    int    `json:"fromSlot"`
	ToSlot      int    `json:"toSlot"`
	// RefusedSlots is the ordered chain of receivers that explicitly NACKed
	// this migration before ToSlot acknowledged it. It is ephemeral display
	// evidence, not a second migration ledger: a refusal is retained only in the
	// bounded correlator and is published only if the same migration eventually
	// has a receiver-confirmed delivery. The page can therefore show the blocked
	// attempt and the successful reroute without animating a rejected offer as an
	// arrival.
	RefusedSlots []int `json:"refusedSlots,omitempty"`
	// RefusalsTruncated is true only if hopRefusedMax cut a pathological chain.
	// The final confirmed destination remains exact either way.
	RefusalsTruncated bool `json:"refusalsTruncated,omitempty"`
	// ExitEdge is the edge the organism LEFT its own world by, which is the lane
	// key the page animates: lanes are keyed fromSlot+edge everywhere on this
	// surface. All four values occur under D17.
	ExitEdge string `json:"exitEdge"`
	// Species is the block the envelope carried, recorded and never resolved
	// (§15, B10). NIL IS THE ANSWER when the envelope carried none, and the page
	// draws the neutral glyph for it.
	Species *contractb.Species `json:"species,omitempty"`
}

// observeHopAttempt remembers every destination offer, including a rerouted
// copy whose migrationId the durable ledger has already seen. It emits nothing:
// MIGRATION_PAYLOAD means "offered", not "spawned".
func (a *Archive) observeHopAttempt(p contractb.MigrationPayload,
	species *contractb.Species, nowMs int64) {
	if p.MigrationID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.trimHopCorrelationLocked(nowMs)
	// A reroute keeps its migration ID. Preserve the ordered refusal evidence
	// from the preceding attempt while replacing only the destination currently
	// eligible to match an ACK.
	var refused []int
	var refusalsTruncated bool
	if previous, ok := a.hopPending[p.MigrationID]; ok {
		refused = append(refused, previous.hop.RefusedSlots...)
		refusalsTruncated = previous.hop.RefusalsTruncated
	}
	destPeer := a.peerForSlotLocked(p.DestSlot)
	a.hopPending[p.MigrationID] = pendingHop{
		hop: Hop{
			MigrationID:       p.MigrationID,
			FromSlot:          p.SourceSlot,
			ToSlot:            p.DestSlot,
			ExitEdge:          p.ExitEdge,
			Species:           species,
			RefusedSlots:      refused,
			RefusalsTruncated: refusalsTruncated,
		},
		destPeer: destPeer,
		seenAtMs: nowMs,
	}
	if early, ok := a.hopEarlyAcks[p.MigrationID]; ok &&
		hopPeerMatches(destPeer, early.sourcePeer) {
		h := a.hopPending[p.MigrationID].hop
		h.AtMs = nowMs
		delete(a.hopPending, p.MigrationID)
		delete(a.hopEarlyAcks, p.MigrationID)
		a.observeHopLocked(h)
	}
	a.capHopCorrelationLocked()
}

// confirmHop promotes the CURRENT destination attempt into the public feed.
// ACK.SourcePeer is the receiver, so it distinguishes a late response from an
// earlier rejected destination from the eventual reroute destination.
func (a *Archive) confirmHop(ack contractb.MigrationAck, nowMs int64) {
	if ack.MigrationID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.trimHopCorrelationLocked(nowMs)
	if p, ok := a.hopPending[ack.MigrationID]; ok &&
		hopPeerMatches(p.destPeer, ack.SourcePeer) {
		h := p.hop
		h.AtMs = nowMs
		delete(a.hopPending, ack.MigrationID)
		delete(a.hopEarlyAcks, ack.MigrationID)
		a.observeHopLocked(h)
		return
	}
	// Cross-peer frames are normally ordered payload then ACK, but the archive
	// does not make correctness depend on that scheduling detail. A later payload
	// copy completes this only if its destination peer matches the ACK sender.
	a.hopEarlyAcks[ack.MigrationID] = earlyHopAck{
		sourcePeer: ack.SourcePeer,
		seenAtMs:   nowMs,
	}
	a.capHopCorrelationLocked()
}

// rejectHopAttempt closes only the attempt rejected by this receiver and keeps
// its slot as bounded evidence for a later reroute animation. A NACK from slot
// B arriving after the migration has been rerouted to slot C must not erase or
// alter C's pending attempt. An unmatchable relay-generated NACK is left to the
// time/count bounds; it cannot produce a public hop without an ACK.
func (a *Archive) rejectHopAttempt(nack contractb.MigrationNack, nowMs int64) {
	if nack.MigrationID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.trimHopCorrelationLocked(nowMs)
	if p, ok := a.hopPending[nack.MigrationID]; ok && p.destPeer != "" &&
		nack.SourcePeer == p.destPeer {
		if n := len(p.hop.RefusedSlots); n == 0 || p.hop.RefusedSlots[n-1] != p.hop.ToSlot {
			if n < hopRefusedMax {
				p.hop.RefusedSlots = append(p.hop.RefusedSlots, p.hop.ToSlot)
			} else {
				p.hop.RefusalsTruncated = true
			}
		}
		// Empty binding means a late ACK from the rejected receiver cannot
		// promote this attempt. The next copied payload replaces it with the
		// alternate destination while carrying RefusedSlots forward.
		p.destPeer = ""
		p.seenAtMs = nowMs
		a.hopPending[nack.MigrationID] = p
	}
}

func hopPeerMatches(want, got string) bool {
	// No binding means no proof. The archive normally receives PEER_STATUS before
	// migration copies; if it did not, suppressing one animation is honest while
	// attaching an ACK to a guessed reroute destination is not.
	return want != "" && got != "" && want == got
}

func (a *Archive) peerForSlotLocked(slot int) string {
	for _, s := range a.status.Slots {
		if s.Slot == slot {
			return s.PeerID
		}
	}
	return ""
}

func (a *Archive) trimHopCorrelationLocked(nowMs int64) {
	// Expiry is maintenance, not work every copied frame must pay. The count cap
	// below still applies immediately; this throttle only avoids repeatedly
	// scanning a healthy, small map whose oldest entry cannot yet have expired.
	if a.hopTrimAtMs != 0 && nowMs-a.hopTrimAtMs < time.Minute.Milliseconds() &&
		len(a.hopPending) <= hopPendingMax && len(a.hopEarlyAcks) <= hopEarlyAckMax {
		return
	}
	a.hopTrimAtMs = nowMs
	cut := nowMs - hopCorrelationWindow.Milliseconds()
	for id, p := range a.hopPending {
		if p.seenAtMs < cut {
			delete(a.hopPending, id)
		}
	}
	for id, ack := range a.hopEarlyAcks {
		if ack.seenAtMs < cut {
			delete(a.hopEarlyAcks, id)
		}
	}
}

func (a *Archive) capHopCorrelationLocked() {
	// Shed a quarter at once when a cap fires. Correlation is best-effort under
	// pathological overflow, and batching makes the scan amortized rather than
	// turning every subsequent copied payload into an O(max) oldest search.
	if len(a.hopPending) > hopPendingMax {
		for id := range a.hopPending {
			delete(a.hopPending, id)
			if len(a.hopPending) <= hopPendingMax*3/4 {
				break
			}
		}
	}
	if len(a.hopEarlyAcks) > hopEarlyAckMax {
		for id := range a.hopEarlyAcks {
			delete(a.hopEarlyAcks, id)
			if len(a.hopEarlyAcks) <= hopEarlyAckMax*3/4 {
				break
			}
		}
	}
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
