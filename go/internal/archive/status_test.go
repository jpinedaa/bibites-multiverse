package archive

import (
	"strings"
	"testing"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
)

// newViewFixture builds an archive with a hand-made PEER_STATUS, so the honesty
// rules of §10.1 can be tested without a rig.
func newViewFixture(t *testing.T, status contractb.PeerStatus, statsAge time.Duration) *Archive {
	t.Helper()
	a, err := New(Config{DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	now := time.Now().UnixMilli()
	for i := range status.Slots {
		if status.Slots[i].Stats != nil {
			status.Slots[i].StatsAsOfMs = now - statsAge.Milliseconds()
		}
	}
	a.mu.Lock()
	a.status = status
	a.statusAt = time.Now()
	a.ready = true
	a.mu.Unlock()
	return a
}

func slot(n, col, row int, live bool, stats *contractb.PeerStats) contractb.SlotInfo {
	return contractb.SlotInfo{
		Slot: n, Position: contractb.Position{Col: col, Row: row},
		PeerID: "peer", Live: live, ModConnected: live,
		GameVersion: "0.6.3.1", SimulationSize: 2000,
		ExportEdges: []string{contracta.EdgeE, contracta.EdgeN},
		Stats:       stats,
	}
}

// TestStaleStatsRenderAsUnknown covers §10.1 rule 3, which is Risk 4's whole
// point: a statsAsOfMs older than statsStaleMs is HISTORY, NOT STATE. A
// population from a peer that went dark an hour ago must not be rendered as
// current, and an honest gap beats a confident zero.
func TestStaleStatsRenderAsUnknown(t *testing.T) {
	fresh := &contractb.PeerStats{
		Population: contractb.IntPtr(231), CustodyDepth: contractb.IntPtr(1),
		PacedDepth: contractb.IntPtr(0), HeldDepth: contractb.IntPtr(2),
		BouncedTimeoutTotal: contractb.IntPtr(0),
	}
	status := contractb.PeerStatus{
		Epoch: 41, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, fresh), slot(2, 1, 0, true, fresh)},
	}

	live := newViewFixture(t, status, time.Second).StatusView()
	if !live.Slots[0].StatsKnown || live.Slots[0].Population == nil {
		t.Fatalf("a one-second-old stats block reads as unknown: %+v", live.Slots[0])
	}
	if live.Totals.Population == nil || *live.Totals.Population != 462 {
		t.Fatalf("the totals sum known populations to %v, want 462", live.Totals.Population)
	}
	if live.Totals.HeldDepth == nil || *live.Totals.HeldDepth != 4 {
		t.Fatalf("held depth totals %v, want 4", live.Totals.HeldDepth)
	}

	stale := newViewFixture(t, status, 5*time.Minute).StatusView()
	for _, s := range stale.Slots {
		if s.StatsKnown {
			t.Fatalf("a five-minute-old stats block reads as state: %+v", s)
		}
		if s.Population != nil || s.HeldDepth != nil {
			t.Fatalf("a stale slot published numbers: %+v", s)
		}
		if s.StatsAgeMs == 0 {
			t.Fatal("a stale slot did not report its age")
		}
	}
	if stale.Totals.Population != nil {
		t.Fatalf("the totals invented a population from stale blocks: %v", stale.Totals.Population)
	}
	if stale.Totals.UnknownSlots != 2 {
		t.Fatalf("unknownSlots = %d, want 2", stale.Totals.UnknownSlots)
	}

	// A slot that never sent a stats block at all is unknown too, and never zero.
	silent := contractb.PeerStatus{
		Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, nil)},
	}
	view := newViewFixture(t, silent, 0).StatusView()
	if view.Slots[0].StatsKnown || view.Slots[0].Population != nil {
		t.Fatalf("a silent slot reported %+v; a slot that reports nothing is unknown, not empty",
			view.Slots[0])
	}
	out := &strings.Builder{}
	RenderRingstat(out, view)
	if !strings.Contains(out.String(), "unknown") {
		t.Fatalf("ringstat rendered a silent slot without saying unknown:\n%s", out)
	}
}

// TestDerivedLanesAndHoles covers §10.1 rule 2 and §6.5's derivation: the page
// recomputes the effective lanes by §8's walk from PEER_STATUS, and a hole is
// derived rather than sent.
func TestDerivedLanesAndHoles(t *testing.T) {
	stats := &contractb.PeerStats{Population: contractb.IntPtr(10)}
	status := contractb.PeerStatus{
		Epoch: 7, Map: contractb.MapShape{Width: 3, Height: 2}, SlotCount: 5,
		Slots: []contractb.SlotInfo{
			slot(1, 0, 0, true, stats), slot(2, 1, 0, true, stats), slot(3, 2, 0, true, stats),
			slot(4, 0, 1, true, stats), slot(5, 1, 1, false, stats),
			// (2,1) is a HOLE: no slot names it.
		},
	}
	view := newViewFixture(t, status, time.Second).StatusView()
	if len(view.Holes) != 1 || view.Holes[0] != (contractb.Position{Col: 2, Row: 1}) {
		t.Fatalf("holes = %v, want exactly (2,1)", view.Holes)
	}
	lanes := map[string]LaneView{}
	for _, l := range view.Lanes {
		lanes[l.Edge+string(rune('0'+l.FromSlot))] = l
	}
	// Slot 4's east lane skips the dark slot 5 AND the hole at (2,1), and wraps
	// back to itself — so nothing on that axis is deliverable and the lane
	// closes with no_peer.
	four := lanes["E4"]
	if four.Open {
		t.Fatalf("slot 4's east lane is open to %d; every other position on its row is dark or a hole",
			four.ToSlot)
	}
	if len(four.Skipped) != 2 {
		t.Fatalf("slot 4's bypass list is %+v, want the dark slot and the hole", four.Skipped)
	}
	if four.Reason != contracta.ReasonNoPeer {
		t.Fatalf("slot 4's closed lane reads %q, want no_peer", four.Reason)
	}
	// Slot 1's north lane finds slot 4 directly.
	one := lanes["N1"]
	if !one.Open || one.ToSlot != 4 || len(one.Skipped) != 0 {
		t.Fatalf("slot 1's north lane is %+v, want a direct lane to slot 4", one)
	}
	// Slot 3's north lane hits the hole at (2,1) and closes.
	three := lanes["N3"]
	if three.Open || len(three.Skipped) != 1 || three.Skipped[0].Slot != nil {
		t.Fatalf("slot 3's north lane is %+v, want closed past one hole", three)
	}
}

// TestMetricsRoundTrip covers WP3 and WP5's durable half: the samples the
// archive appends survive the process, and ringstat renders one when the
// archive is not running.
func TestMetricsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m, err := OpenMetrics(dir)
	if err != nil {
		t.Fatalf("OpenMetrics: %v", err)
	}
	for i := 1; i <= 3; i++ {
		if err := m.Append(Status{
			GeneratedAtMs: int64(i), HaveStatus: true, Epoch: int64(i),
			Map:       contractb.MapShape{Width: 3, Height: 2},
			SlotCount: 6,
		}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	samples, err := ReadMetrics(m.Path())
	if err != nil || len(samples) != 3 {
		t.Fatalf("ReadMetrics = %d samples, %v", len(samples), err)
	}
	last, err := LastSample(m.Path())
	if err != nil || last.Epoch != 3 {
		t.Fatalf("LastSample = %+v, %v", last, err)
	}
	out := &strings.Builder{}
	RenderRingstat(out, last)
	if !strings.Contains(out.String(), "map 3x2") {
		t.Fatalf("ringstat did not render the sample:\n%s", out)
	}
	if _, err := LastSample(dir + "/nothing.jsonl"); err == nil {
		t.Fatal("LastSample invented a sample from a file that does not exist")
	}
}

// TestCensusIsAStatAndObeysEveryRuleForOne is contract-b-m4.md §16, B12's first
// resolution: `stats.species` is what the page's species view is built from,
// and EVERY RULE §10.1 already had applies to it rather than being bent for it.
//
// Four states, four different answers, and only one of them is a list:
//
//	ABSENT    — the block carries no census: UNKNOWN. Never "no species", never
//	            an empty list, never a zero.
//	EMPTY     — the block carries `[]`: the stronger, different fact, and the
//	            page may say so.
//	PRESENT   — the entries, in the census's own order, names untouched.
//	STALE     — a block older than statsStaleMs is HISTORY, NOT STATE, and its
//	            census goes back to unknown with the population beside it.
func TestCensusIsAStatAndObeysEveryRuleForOne(t *testing.T) {
	census := &contractb.Census{Entries: []contractb.CensusEntry{
		// Raw, in the sender's order, with the whitespace that makes this lane
		// different from the migration lane (contract-a.md §17, A36).
		{GenericName: "Izus ", SpecificName: "copedylanus", Bibites: 96, Eggs: 14},
		{GenericName: "Izus", SpecificName: "copedylanus", Bibites: 61, Eggs: 9},
		{GenericName: " ", SpecificName: "anonymus", Bibites: 17, Eggs: 3},
	}}
	present := &contractb.PeerStats{
		Population: contractb.IntPtr(200), EggCount: contractb.IntPtr(30),
		Species: census, Truncated: true,
	}
	empty := &contractb.PeerStats{
		Population: contractb.IntPtr(0), Species: &contractb.Census{},
	}
	absent := &contractb.PeerStats{Population: contractb.IntPtr(41), Truncated: true}

	status := contractb.PeerStatus{
		Epoch: 9, Map: contractb.MapShape{Width: 3, Height: 1}, SlotCount: 3,
		Slots: []contractb.SlotInfo{
			slot(1, 0, 0, true, present), slot(2, 1, 0, true, empty),
			slot(3, 2, 0, true, absent),
		},
	}

	view := newViewFixture(t, status, time.Second).StatusView()
	byslot := map[int]SlotView{}
	for _, v := range view.Slots {
		byslot[v.Slot] = v
	}

	one := byslot[1]
	if !one.SpeciesKnown || len(one.Species) != 3 {
		t.Fatalf("slot 1 published %v / %+v, want a three-entry census",
			one.SpeciesKnown, one.Species)
	}
	for i, want := range census.Entries {
		if one.Species[i] != want {
			t.Fatalf("entry %d is %+v, want %+v — the view copies, it does not repair",
				i, one.Species[i], want)
		}
	}
	if !one.SpeciesTruncated {
		t.Fatal("slot 1 lost the truncated flag; the page MUST be able to say the rest is unreported")
	}

	two := byslot[2]
	if !two.SpeciesKnown {
		t.Fatal("a present `[]` read as unknown; it is the stronger fact, not the absent one")
	}
	if len(two.Species) != 0 {
		t.Fatalf("a present `[]` gained entries: %+v", two.Species)
	}

	three := byslot[3]
	if three.SpeciesKnown || three.Species != nil {
		t.Fatalf("an absent census read as %v / %+v; absent is UNKNOWN",
			three.SpeciesKnown, three.Species)
	}
	if three.SpeciesTruncated {
		t.Fatal("truncated survived with no census to qualify; it is ignored when species is absent")
	}
	if three.Population == nil || *three.Population != 41 {
		t.Fatal("the missing census cost the population; a census is not an input to anything")
	}

	// STALE. Past statsStaleMs the whole block is history, and the census ages
	// with it — it has no clock of its own, because a fresh species list beside
	// a stale population from the same frame is the one thing one timestamp per
	// block exists to prevent (§16, B11).
	stale := newViewFixture(t, status, 5*time.Minute).StatusView()
	for _, v := range stale.Slots {
		if v.SpeciesKnown || v.Species != nil {
			t.Fatalf("slot %d served a census out of a stale block: %+v", v.Slot, v.Species)
		}
	}

	// The terminal tool renders the SAME Status, so the two operator surfaces
	// cannot disagree about which species live where — and it prints the raw
	// spelling, doubled space and all.
	out := &strings.Builder{}
	RenderRingstat(out, view)
	got := out.String()
	for _, want := range []string{
		"Izus  copedylanus ×96+14egg", // the trailing space, still doubled
		"Izus copedylanus ×61+9egg",   // its whitespace twin, still a separate entry
		"UNREPORTED",                  // slot 1 is truncated and says so
		"none alive",                  // slot 2's present []
		"unknown",                     // slot 3's absent census
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ringstat never printed %q:\n%s", want, got)
		}
	}
}

// TestSpeedAndPacingCarryOrStayUnknown covers contract-b-m4.md §18, B16 on the
// archive's side of the wire, and it is §10.1 rule 3 applied to three new
// fields: timeScale is the world's own speed, and inboundRatePerSimMinute /
// inboundRateBurst are the cap arrivals are queued behind.
//
// THE UNKNOWN HALF IS THE POINT. This rig runs one peer whose build predates
// the fields, and the shipped default it would have used has been changed three
// times. Rendering the default in its place would be a confident wrong number
// about the one slot nobody can check by hand.
func TestSpeedAndPacingCarryOrStayUnknown(t *testing.T) {
	reporting := &contractb.PeerStats{
		Population:              contractb.IntPtr(42),
		PacedDepth:              contractb.IntPtr(3),
		TimeScale:               contractb.Float64Ptr(5),
		InboundRatePerSimMinute: contractb.Float64Ptr(100),
		InboundRateBurst:        contractb.Float64Ptr(50),
	}
	// A world that is standing still reports 0, and 0 is a READING: paused is a
	// state the page must show, not a gap it must hide.
	paused := &contractb.PeerStats{
		Population: contractb.IntPtr(7), PacedDepth: contractb.IntPtr(0),
		TimeScale:               contractb.Float64Ptr(0),
		InboundRatePerSimMinute: contractb.Float64Ptr(2),
	}
	// The far end: an older sidecar. It publishes what it knows and omits what
	// it has never heard of.
	older := &contractb.PeerStats{
		Population: contractb.IntPtr(12), PacedDepth: contractb.IntPtr(0),
	}
	status := contractb.PeerStatus{
		Epoch: 3, Map: contractb.MapShape{Width: 3, Height: 1}, SlotCount: 3,
		Slots: []contractb.SlotInfo{
			slot(1, 0, 0, true, reporting), slot(2, 1, 0, true, paused),
			slot(3, 2, 0, true, older),
		},
	}
	view := newViewFixture(t, status, time.Second).StatusView()

	v := view.Slots[0]
	if v.TimeScale == nil || *v.TimeScale != 5 {
		t.Fatalf("slot 1 timeScale = %v, want 5", v.TimeScale)
	}
	if v.InboundRatePerSimMinute == nil || *v.InboundRatePerSimMinute != 100 {
		t.Fatalf("slot 1 inboundRatePerSimMinute = %v, want 100", v.InboundRatePerSimMinute)
	}
	if v.InboundRateBurst == nil || *v.InboundRateBurst != 50 {
		t.Fatalf("slot 1 inboundRateBurst = %v, want 50", v.InboundRateBurst)
	}

	if p := view.Slots[1].TimeScale; p == nil || *p != 0 {
		t.Fatalf("a paused world's timeScale = %v; 0 is a reading, not an absence", p)
	}

	old := view.Slots[2]
	if !old.StatsKnown {
		t.Fatal("the older peer's block is fresh; it must still count as known")
	}
	if old.TimeScale != nil || old.InboundRatePerSimMinute != nil || old.InboundRateBurst != nil {
		t.Fatalf("a peer that published none of the new fields got values invented for it: %+v", old)
	}
	if old.PacedDepth == nil || *old.PacedDepth != 0 {
		t.Fatalf("the older peer's pacedDepth was lost with the fields it does not send: %v",
			old.PacedDepth)
	}

	// And a stale block takes all three with it, exactly as it takes population.
	stale := newViewFixture(t, status, time.Hour).StatusView()
	if stale.Slots[0].TimeScale != nil || stale.Slots[0].InboundRatePerSimMinute != nil {
		t.Fatalf("a stale block still reported a speed and a cap: %+v", stale.Slots[0])
	}
}
