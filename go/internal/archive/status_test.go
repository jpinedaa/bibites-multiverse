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
