package archive

// THE ACHIEVED TIME SCALE — what a world's clock actually did, as against the
// timeScale it reports applying.
//
// This is contract-b-m4.md §10.1 rule 2 — DERIVED, AND MARKED AS DERIVED — and
// it adds NO WIRE FIELD. `stats.simulatedTime` and `statsAsOfMs` have been on
// every PEER_STATUS since M4 (§6.3.1, §6.5), the archive already receives both
// as a subscriber and already publishes `simulatedTime` per slot. All that is
// new here is the join: Δ simulatedTime / Δ wall over a sliding window. It is
// the same precedent §10.1 set for the recent-hops feed — "the inputs are not
// new … what is new is the join, and no wire field is added".
//
// WHY THE PAGE NEEDED IT. Three different numbers get called "the speed of a
// world" and only the third is a measurement:
//
//   - the TARGET is what the operator asked for (`timescale 100`);
//   - the APPLIED scale is what the game's governor allowed — the mod copies
//     `Time.timeScale` into the heartbeat and the sidecar copies that into
//     `stats.timeScale`, never computing it;
//   - the ACHIEVED rate is how much simulated time the world actually produced
//     per wall second, and nothing on the wire carries it.
//
// The two come apart precisely when the host cannot keep up, which is when an
// operator most needs to know: `Time.maximumDeltaTime` is pinned to one fixed
// step, so the achieved rate cannot exceed `fps / simTPS` however high the
// target goes. On the living rig all six worlds report an applied ×100 and
// advance 5–45×. A page that prints the applied figure alone is silent about
// the gap, and the gap is where the news is (dev_environment.md, *A world can
// be at the wrong time scale*).
//
// §10.1 rule 3 binds this as hard as everything else on the page. Every guard
// below resolves to UNKNOWN, and the cell then shows the applied value ALONE
// rather than a number it cannot stand behind. There is no such thing here as
// a negative rate, a rate measured across a seat that changed worlds, or a
// rate over a span too short to mean anything.

import (
	"time"

	"multiverse/internal/contractb"
)

const (
	// achievedWindow is how far back the rate looks. PEER_STATUS republishes on
	// contractb.StatsBroadcastInterval (5 s) and more often when the registry
	// is dirty, so a minute holds at least a dozen pairs — enough to smooth the
	// jitter of which frame boundary a heartbeat happened to sample, without
	// smearing a real change of speed across a whole reading.
	achievedWindow = time.Minute
	// achievedMinSpan is the shortest span a rate may be computed over. Two
	// pairs five seconds apart turn a 100 ms wobble in when the mod read
	// TimeKeeper into a 2% error; ten seconds halves it, and the cost of
	// waiting is one extra poll showing the applied value alone.
	achievedMinSpan = 10 * time.Second
	// achievedMaxGap is the longest two consecutive pairs may straddle and
	// still describe one continuous run. A stats block stops carrying
	// simulatedTime the instant its mod disconnects, which restarts the
	// measurement on its own; this catches the quieter case where blocks keep
	// arriving from a sidecar whose world is not advancing.
	achievedMaxGap = 20 * time.Second
	// achievedMaxSamples bounds what one slot can cost if the relay publishes
	// far faster than its timer. The WINDOW is what makes the rate honest; this
	// only stops the slice being unbounded.
	achievedMaxSamples = 512
)

// simSample is one (wall clock, simulated clock) pair for one slot.
//
// The wall half is `statsAsOfMs` — the RELAY's clock when that stats block
// arrived (§6.5) — and not the archive's own and not the origin's. That matters
// twice: it is ONE clock for all six slots, so the far end's reading is
// comparable with a local one without trusting the second computer's clock, and
// it is already the clock §10.1's freshness rule is measured against, so the
// rate and the staleness of the block behind it cannot disagree.
type simSample struct {
	atMs int64
	sim  float64
}

// achievedRate is one slot's sliding window of pairs.
type achievedRate struct {
	peerID  string
	samples []simSample
}

// observe records one pair, or throws the history away when the new pair cannot
// honestly be compared with what came before. A nil sim is a stats block with
// no simulatedTime on it, which is what a block looks like the moment a mod
// disconnects — the mod-quiet gap — and it starts the measurement over.
func (r *achievedRate) observe(peerID string, atMs int64, sim *float64) {
	if peerID != r.peerID {
		// A different peer in this seat is a different world, whatever the two
		// clocks happen to read. Nothing before this belongs to it.
		r.peerID, r.samples = peerID, nil
	}
	if sim == nil || atMs <= 0 {
		r.samples = nil
		return
	}
	if n := len(r.samples); n > 0 {
		last := r.samples[n-1]
		switch {
		case atMs <= last.atMs:
			// The same block republished on the §6.5 timer, or a clock that
			// stepped back. Neither is a new measurement, and a zero Δ wall is
			// not a rate.
			return
		case *sim < last.sim:
			// simulatedTime runs CONTINUOUSLY across a restart of the same
			// world — proven on slot 6 across two mode flips — so it only ever
			// falls when the seat has been given a different simulation. A
			// negative rate is never a reading; the window starts again.
			r.samples = nil
		case atMs-last.atMs > achievedMaxGap.Milliseconds():
			// The two pairs do not describe one continuous run, so the
			// simulated time between them is not something this archive
			// watched happen.
			r.samples = nil
		}
	}
	r.samples = append(r.samples, simSample{atMs: atMs, sim: *sim})
	if len(r.samples) > achievedMaxSamples {
		r.samples = append(r.samples[:0], r.samples[len(r.samples)-achievedMaxSamples:]...)
	}
}

// rate is Δ simulatedTime / Δ wall over the window, with the span it was
// actually measured over — because a rate with no window on it is not a
// measurement, which is the same reason Status carries flowWindowMs.
//
// It reports UNKNOWN rather than a number it cannot stand behind: fewer than
// two pairs (archive startup), a window that has aged out from under it (a mod
// gone quiet for longer than a minute), or a span too short to smooth the
// jitter.
func (r *achievedRate) rate(nowMs int64) (float64, int64, bool) {
	r.trim(nowMs)
	if len(r.samples) < 2 {
		return 0, 0, false
	}
	first, last := r.samples[0], r.samples[len(r.samples)-1]
	span := last.atMs - first.atMs
	if span < achievedMinSpan.Milliseconds() {
		return 0, 0, false
	}
	advanced := last.sim - first.sim
	if advanced < 0 {
		// observe drops the history on a fall, so this is unreachable by the
		// ordinary path. It stays because the alternative to a redundant guard
		// here is a negative time scale on the operator's map.
		return 0, 0, false
	}
	return advanced / (float64(span) / 1000), span, true
}

// trim drops the pairs that have aged out of the window. When the newest one
// has aged out too the slice empties, and the slot reads unknown — which is the
// right answer for a world nothing has been heard from.
func (r *achievedRate) trim(nowMs int64) {
	cut := nowMs - achievedWindow.Milliseconds()
	i := 0
	for i < len(r.samples) && r.samples[i].atMs < cut {
		i++
	}
	if i > 0 {
		r.samples = append(r.samples[:0], r.samples[i:]...)
	}
}

// observeSimRatesLocked feeds one accepted PEER_STATUS into every slot's window.
// It is called from the frame handler rather than from StatusView on purpose:
// §10.1 rule 1 is ONE SOURCE, NO POLLING, and a rate that only advanced when
// somebody loaded the page would be a measurement of the reader.
func (a *Archive) observeSimRatesLocked(status contractb.PeerStatus) {
	present := make(map[int]bool, len(status.Slots))
	for _, si := range status.Slots {
		present[si.Slot] = true
		r := a.simRates[si.Slot]
		if r == nil {
			r = &achievedRate{}
			a.simRates[si.Slot] = r
		}
		var sim *float64
		if si.Stats != nil {
			sim = si.Stats.SimulatedTime
		}
		r.observe(si.PeerID, si.StatsAsOfMs, sim)
	}
	// A seat that has left the registry keeps no window. It is not a slot the
	// page draws, and its pairs could only ever be compared against a world
	// that is not there.
	for slot := range a.simRates {
		if !present[slot] {
			delete(a.simRates, slot)
		}
	}
}
