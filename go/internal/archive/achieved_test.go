package archive

import (
	"testing"
	"time"

	"multiverse/internal/contractb"
)

// feed pushes a run of pairs into one window: n samples, every step apart,
// with the simulated clock advancing at `rate` simulated seconds per wall
// second. It returns the wall clock of the last one.
func feed(r *achievedRate, peer string, startMs int64, step time.Duration, n int, rate float64) int64 {
	sim := 1000.0
	at := startMs
	for i := 0; i < n; i++ {
		s := sim
		r.observe(peer, at, &s)
		at += step.Milliseconds()
		sim += rate * step.Seconds()
	}
	return at - step.Milliseconds()
}

// TestAchievedRateMeasuresTheClock is the whole point of the thing: the applied
// time scale is what the game says it is doing, and this is what its clock
// actually did.
func TestAchievedRateMeasuresTheClock(t *testing.T) {
	var r achievedRate
	last := feed(&r, "slot-1", 1_000_000, 5*time.Second, 13, 12.5)
	got, span, ok := r.rate(last)
	if !ok {
		t.Fatalf("rate: unknown after a full minute of samples")
	}
	if span != 60_000 {
		t.Errorf("span = %d ms, want 60000", span)
	}
	if got < 12.4 || got > 12.6 {
		t.Errorf("achieved = %v, want ~12.5", got)
	}
}

// TestAchievedRateNeedsTwoSamples is archive startup. Until a second comparable
// pair has arrived there is nothing to divide, and §10.1 rule 3 says the answer
// to that is UNKNOWN — the page then shows the applied value alone.
func TestAchievedRateNeedsTwoSamples(t *testing.T) {
	var r achievedRate
	sim := 500.0
	r.observe("slot-1", 1_000_000, &sim)
	if _, _, ok := r.rate(1_000_000); ok {
		t.Fatalf("rate: reported a number from one sample")
	}
}

// TestAchievedRateNeedsASpan guards the other half of startup: two samples one
// heartbeat apart would turn the jitter of which frame the mod sampled into a
// number that jumps every poll.
func TestAchievedRateNeedsASpan(t *testing.T) {
	var short achievedRate
	last := feed(&short, "slot-1", 1_000_000, time.Second, 3, 40)
	if _, _, ok := short.rate(last); ok {
		t.Fatalf("rate: reported a number over a 2 s span")
	}
	// A run that crosses achievedMinSpan answers, over the span it really used.
	var long achievedRate
	last = feed(&long, "slot-1", 1_000_000, time.Second, 12, 40)
	got, span, ok := long.rate(last)
	if !ok {
		t.Fatalf("rate: still unknown past the minimum span")
	}
	if span != 11_000 {
		t.Errorf("span = %d ms, want the 11000 actually measured", span)
	}
	if got < 39.9 || got > 40.1 {
		t.Errorf("achieved = %v, want ~40", got)
	}
}

// TestAchievedRateNeverGoesNegative is the restart rule. simulatedTime runs
// continuously across a restart of the SAME world, so a fall means the seat is
// running a different simulation — and the honest answer to that is UNKNOWN for
// a moment, never a negative time scale and never an absurd one.
func TestAchievedRateNeverGoesNegative(t *testing.T) {
	var r achievedRate
	last := feed(&r, "slot-1", 1_000_000, 5*time.Second, 13, 12)
	if _, _, ok := r.rate(last); !ok {
		t.Fatalf("rate: unknown before the reset")
	}
	fresh := 3.0 // a brand new world: its clock starts again
	r.observe("slot-1", last+5000, &fresh)
	if _, _, ok := r.rate(last + 5000); ok {
		t.Fatalf("rate: reported a number across a world change")
	}
	after := feed(&r, "slot-1", last+10_000, 5*time.Second, 13, 30)
	got, _, ok := r.rate(after)
	if !ok {
		t.Fatalf("rate: never recovered after the reset")
	}
	if got < 29 || got > 31 {
		t.Errorf("achieved = %v, want ~30 from the new world alone", got)
	}
}

// TestAchievedRateForgetsAnotherPeersWorld: a different peer in the seat is a
// different world whatever the two clocks happen to read, so the pairs before
// it can never be differenced against the pairs after it.
func TestAchievedRateForgetsAnotherPeersWorld(t *testing.T) {
	var r achievedRate
	last := feed(&r, "slot-1", 1_000_000, 5*time.Second, 13, 12)
	sim := 9_000_000.0 // a much older world, so the delta would be enormous
	r.observe("slot-1-replacement", last+5000, &sim)
	if _, _, ok := r.rate(last + 5000); ok {
		t.Fatalf("rate: differenced two peers' clocks")
	}
}

// TestAchievedRateResetsOnAQuietMod: the stats block stops carrying
// simulatedTime the moment a mod disconnects, and the world's clock then stands
// still while the wall clock does not. Differencing across that gap would
// report a world running at a fraction of its real rate, and then — when the
// mod returns and the clock jumps — at a wild multiple of it.
func TestAchievedRateResetsOnAQuietMod(t *testing.T) {
	var r achievedRate
	last := feed(&r, "slot-1", 1_000_000, 5*time.Second, 13, 12)
	r.observe("slot-1", last+5000, nil)
	if _, _, ok := r.rate(last + 5000); ok {
		t.Fatalf("rate: reported a number across a mod-quiet gap")
	}
}

// TestAchievedRateAgesOut: a window with nothing new in it for longer than
// achievedWindow empties, rather than answering forever with the last rate it
// happened to measure. An honest gap beats a stale confident number.
func TestAchievedRateAgesOut(t *testing.T) {
	var r achievedRate
	last := feed(&r, "slot-1", 1_000_000, 5*time.Second, 13, 12)
	if _, _, ok := r.rate(last); !ok {
		t.Fatalf("rate: unknown while fresh")
	}
	if _, _, ok := r.rate(last + achievedWindow.Milliseconds() + 1); ok {
		t.Fatalf("rate: still answering a minute after the last sample")
	}
}

// TestAchievedRateSkipsARepublishedBlock: PEER_STATUS is rebroadcast on a timer
// whether or not a new stats block arrived (§6.5), so the same statsAsOfMs
// arrives many times. A zero Δ wall is not a rate.
func TestAchievedRateSkipsARepublishedBlock(t *testing.T) {
	var r achievedRate
	sim := 100.0
	r.observe("slot-1", 1_000_000, &sim)
	for i := 0; i < 5; i++ {
		same := sim
		r.observe("slot-1", 1_000_000, &same)
	}
	if len(r.samples) != 1 {
		t.Fatalf("samples = %d, want 1 — a republished block is not a measurement", len(r.samples))
	}
}

// TestAchievedRateBoundsItsMemory: the relay coalesces to 250 ms and can publish
// far faster than its 5 s timer, so the slice needs a ceiling as well as a
// window.
func TestAchievedRateBoundsItsMemory(t *testing.T) {
	var r achievedRate
	feed(&r, "slot-1", 1_000_000, 10*time.Millisecond, achievedMaxSamples*3, 1)
	if len(r.samples) > achievedMaxSamples {
		t.Fatalf("samples = %d, want at most %d", len(r.samples), achievedMaxSamples)
	}
}

// TestStatusViewPublishesTheAchievedScale wires it to the operator surface: the
// applied scale and the measured one arrive on the same slot, and the measured
// one carries the span it was measured over — because a rate with no window on
// it is not a measurement.
func TestStatusViewPublishesTheAchievedScale(t *testing.T) {
	stats := &contractb.PeerStats{
		Population: contractb.IntPtr(50),
		TimeScale:  contractb.Float64Ptr(100),
	}
	status := contractb.PeerStatus{
		Epoch: 7, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, stats), slot(2, 1, 0, true, stats)},
	}
	a := newViewFixture(t, status, time.Second)

	// Slot 1 gets a minute of pairs ending now; slot 2 gets none, which is what
	// a slot looks like while the archive is still filling its window.
	now := time.Now().UnixMilli()
	a.mu.Lock()
	r := &achievedRate{}
	feed(r, "peer", now-60_000, 5*time.Second, 13, 12.5)
	a.simRates[1] = r
	a.mu.Unlock()

	view := a.StatusView()
	if view.AchievedWindowMs != achievedWindow.Milliseconds() {
		t.Errorf("achievedWindowMs = %d, want %d",
			view.AchievedWindowMs, achievedWindow.Milliseconds())
	}
	one, two := view.Slots[0], view.Slots[1]
	if one.TimeScale == nil || *one.TimeScale != 100 {
		t.Fatalf("slot 1 timeScale = %v, want the applied 100 unchanged", one.TimeScale)
	}
	if one.AchievedTimeScale == nil {
		t.Fatalf("slot 1 achievedTimeScale: unknown with a full window")
	}
	if *one.AchievedTimeScale < 12.4 || *one.AchievedTimeScale > 12.6 {
		t.Errorf("slot 1 achievedTimeScale = %v, want ~12.5", *one.AchievedTimeScale)
	}
	if one.AchievedSpanMs != 60_000 {
		t.Errorf("slot 1 achievedSpanMs = %d, want 60000", one.AchievedSpanMs)
	}
	if two.AchievedTimeScale != nil || two.AchievedSpanMs != 0 {
		t.Errorf("slot 2 reported a measurement it never made: %v over %d ms",
			two.AchievedTimeScale, two.AchievedSpanMs)
	}
	if two.TimeScale == nil {
		t.Errorf("slot 2 lost its applied scale along with its unmeasured one")
	}
}

// TestObserveSimRatesForgetsADepartedSeat: a slot that leaves the registry keeps
// no window, or the archive would hold a growing set of clocks for worlds that
// are not on the map.
func TestObserveSimRatesForgetsADepartedSeat(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	withSim := func(sim float64) *contractb.PeerStats {
		return &contractb.PeerStats{SimulatedTime: contractb.Float64Ptr(sim)}
	}
	now := time.Now().UnixMilli()
	two := contractb.PeerStatus{Slots: []contractb.SlotInfo{
		slot(1, 0, 0, true, withSim(10)), slot(2, 1, 0, true, withSim(20)),
	}}
	for i := range two.Slots {
		two.Slots[i].StatsAsOfMs = now
	}
	a.mu.Lock()
	a.observeSimRatesLocked(two)
	if len(a.simRates) != 2 {
		a.mu.Unlock()
		t.Fatalf("simRates = %d windows, want 2", len(a.simRates))
	}
	one := contractb.PeerStatus{Slots: []contractb.SlotInfo{slot(1, 0, 0, true, withSim(30))}}
	one.Slots[0].StatsAsOfMs = now + 5000
	a.observeSimRatesLocked(one)
	n, kept := len(a.simRates), a.simRates[1] != nil
	a.mu.Unlock()
	if n != 1 || !kept {
		t.Fatalf("simRates = %d windows (slot 1 kept: %v), want only slot 1", n, kept)
	}
}
