package relay

import (
	"testing"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
)

// grid3x2 is the exit test's map and §2.1's smallest honest two-axis map:
// row 0 holds slots 1-3, row 1 holds slots 4-6. Under two-way lanes its ROWS
// are real cycles of three and its COLUMNS are two-lane shuttles.
func grid3x2(t *testing.T) *Grid {
	t.Helper()
	g := &Grid{}
	for i, pos := range []contractb.Position{
		{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 2, Row: 0},
		{Col: 0, Row: 1}, {Col: 1, Row: 1}, {Col: 2, Row: 1},
	} {
		p := pos
		place(t, g, "peer-"+string(rune('a'+i)), Preference{Position: &p})
	}
	return g
}

func effSlot(t *testing.T, g *Grid, slot int, edge string, ok Deliverable) int {
	t.Helper()
	res, found := g.ResOfSlot(slot)
	if !found {
		t.Fatalf("no slot %d", slot)
	}
	target, _, hit := g.Effective(res, edge, ok)
	if !hit {
		return 0
	}
	return target.Slot
}

// TestReverseWalksAreTheForwardWalksNegated is contract-b-m4.md §17, B13:
// effective(W) and effective(S) are effective(E) and effective(N) with the step
// negated, and that is the whole of D17 on this wire.
//
// It also pins the two things a reverse walk gets wrong if it reaches for Go's
// % on a negative dividend: a west lane off column 0 must wrap to the LAST
// column, not address column -1.
func TestReverseWalksAreTheForwardWalksNegated(t *testing.T) {
	g := grid3x2(t)
	// Row 0: 1 2 3. Row 1: 4 5 6.
	for _, c := range []struct {
		slot int
		edge string
		want int
	}{
		{1, contracta.EdgeE, 2}, {1, contracta.EdgeW, 3}, // W off col 0 wraps
		{2, contracta.EdgeE, 3}, {2, contracta.EdgeW, 1},
		{3, contracta.EdgeE, 1}, {3, contracta.EdgeW, 2}, // E off the last col wraps
		{4, contracta.EdgeE, 5}, {4, contracta.EdgeW, 6},
		{6, contracta.EdgeE, 4}, {6, contracta.EdgeW, 5},
		// The columns are axes of length 2, so both vertical walks name the same
		// peer. That is the shuttle, and it has its own test below.
		{1, contracta.EdgeN, 4}, {1, contracta.EdgeS, 4},
		{5, contracta.EdgeN, 2}, {5, contracta.EdgeS, 2},
	} {
		if got := effSlot(t, g, c.slot, c.edge, allDeliverable); got != c.want {
			t.Fatalf("effective(slot %d, %s) = %d, want slot %d", c.slot, c.edge, got, c.want)
		}
	}
	// A walk never returns to its own source, in either direction.
	for _, edge := range contracta.CanonicalEdges() {
		for slot := 1; slot <= 6; slot++ {
			if got := effSlot(t, g, slot, edge, allDeliverable); got == slot {
				t.Fatalf("slot %d's %s lane points at itself", slot, edge)
			}
		}
	}
}

// TestSkipListsArePerWalkNotPerAxis is B13's row of the same name. East and
// west cross ONE row from opposite ends, meet its dark slots in a different
// order and stop at the first live one, so their skipped lists routinely differ
// even when they name the same target.
//
// This is the case a per-AXIS skip list would silently get wrong: it would
// report one bypass for both directions, and one of the two would be a lie
// about which world was reached over.
func TestSkipListsArePerWalkNotPerAxis(t *testing.T) {
	g := &Grid{}
	// A row of four: 1 2 3 4, with 2 and 4 dark.
	for i := 0; i < 4; i++ {
		p := contractb.Position{Col: i, Row: 0}
		place(t, g, "peer-"+string(rune('a'+i)), Preference{Position: &p})
	}
	one, _ := g.ResOfSlot(1)
	filter := dark(2, 4)

	east, eastSkip, ok := g.Effective(one, contracta.EdgeE, filter)
	if !ok || east.Slot != 3 {
		t.Fatalf("slot 1's east lane is %v (ok=%v), want slot 3 over the dark slot 2", east, ok)
	}
	west, westSkip, ok := g.Effective(one, contracta.EdgeW, filter)
	if !ok || west.Slot != 3 {
		t.Fatalf("slot 1's west lane is %v (ok=%v), want slot 3 over the dark slot 4", west, ok)
	}
	// SAME TARGET, DIFFERENT BYPASS. East reached over slot 2; west wrapped and
	// reached over slot 4.
	if len(eastSkip) != 1 || eastSkip[0].Slot == nil || *eastSkip[0].Slot != 2 {
		t.Fatalf("the east bypass list is %+v, want one entry for slot 2", eastSkip)
	}
	if len(westSkip) != 1 || westSkip[0].Slot == nil || *westSkip[0].Slot != 4 {
		t.Fatalf("the west bypass list is %+v, want one entry for slot 4", westSkip)
	}
}

// TestReverseRouteAroundBypassesADarkSlot is D12 applied to the lane D17 added:
// a dark slot mid-row must not stop the WEST current any more than it stops the
// east one. Killing slot 2 out of row 0 re-pairs slot 3's west lane onto slot 1
// and slot 1's east lane onto slot 3, and neither edge closes.
func TestReverseRouteAroundBypassesADarkSlot(t *testing.T) {
	g := grid3x2(t)
	filter := dark(2)

	if got := effSlot(t, g, 3, contracta.EdgeW, filter); got != 1 {
		t.Fatalf("slot 3's west lane is %d, want slot 1 over the dark slot 2", got)
	}
	if got := effSlot(t, g, 1, contracta.EdgeE, filter); got != 3 {
		t.Fatalf("slot 1's east lane is %d, want slot 3 over the dark slot 2", got)
	}
	// And the lanes that pointed AT slot 2 from the other side re-pair too. This
	// is the symmetric ripple: under one-way lanes slot 3 was slot 2's east
	// neighbour and was told nothing, because nothing pointed at slot 2 from
	// there. Now it does.
	if got := effSlot(t, g, 1, contracta.EdgeW, filter); got != 3 {
		t.Fatalf("slot 1's west lane is %d, want slot 3 (its row still has two live slots)", got)
	}
	three, _ := g.ResOfSlot(3)
	_, skipped, ok := g.Effective(three, contracta.EdgeW, filter)
	if !ok || len(skipped) != 1 || *skipped[0].Slot != 2 ||
		skipped[0].Reason != contractb.SkipPeerOffline {
		t.Fatalf("the west bypass list is %+v (ok=%v), want one peer_offline entry for slot 2",
			skipped, ok)
	}
}

// TestLengthTwoAxisIsATwoLaneShuttle is §2.1's arithmetic, on the exact shape
// the M4 rig runs: on an axis of length 2 one step forward and one step back
// are the same position mod 2, so effective(N) and effective(S) name the SAME
// peer — both lanes of a two-lane shuttle. On the 3x2 map EVERY COLUMN is such
// an axis.
//
// And they therefore close TOGETHER. Killing a column partner closes both N and
// S with no candidates left, because there is no third slot on that axis to
// re-pair to. That is route-around working correctly against a degenerate axis,
// not route-around failing.
func TestLengthTwoAxisIsATwoLaneShuttle(t *testing.T) {
	g := grid3x2(t)
	columns := [][2]int{{1, 4}, {2, 5}, {3, 6}}
	for _, col := range columns {
		lower, upper := col[0], col[1]
		for _, pair := range [][2]int{{lower, upper}, {upper, lower}} {
			from, want := pair[0], pair[1]
			north := effSlot(t, g, from, contracta.EdgeN, allDeliverable)
			south := effSlot(t, g, from, contracta.EdgeS, allDeliverable)
			if north != want || south != want {
				t.Fatalf("slot %d: north lane %d, south lane %d, want both to be slot %d — "+
					"on an axis of length 2 the forward and reverse walks name the same peer",
					from, north, south, want)
			}
		}
	}
	// The kill: slot 4 goes dark and slot 1 loses BOTH vertical lanes at once.
	filter := dark(4)
	for _, edge := range []string{contracta.EdgeN, contracta.EdgeS} {
		one, _ := g.ResOfSlot(1)
		_, skipped, ok := g.Effective(one, edge, filter)
		if ok {
			t.Fatalf("slot 1's %s lane survived the death of its only column partner", edge)
		}
		if len(skipped) != 1 || skipped[0].Slot == nil || *skipped[0].Slot != 4 {
			t.Fatalf("slot 1's %s bypass list is %+v, want one entry for slot 4", edge, skipped)
		}
	}
	// Its ROW is untouched: three slots, two of them still deliverable, so both
	// horizontal lanes stay open. The two axes close independently.
	if got := effSlot(t, g, 1, contracta.EdgeE, filter); got != 2 {
		t.Fatalf("slot 1's east lane is %d, want slot 2 — the row is not degenerate", got)
	}
	if got := effSlot(t, g, 1, contracta.EdgeW, filter); got != 3 {
		t.Fatalf("slot 1's west lane is %d, want slot 3 — the row is not degenerate", got)
	}
}

// TestDegenerateShapesHaveNoReverseLanesEither extends §2.1's table to the two
// walks D17 added: an axis of length 1 has no candidates in EITHER direction,
// so a lone peer and a one-row map close their reverse edges exactly as they
// close their forward ones.
func TestDegenerateShapesHaveNoReverseLanesEither(t *testing.T) {
	g := &Grid{}
	place(t, g, "alone", Preference{})
	one, _ := g.ResOfSlot(1)
	for _, edge := range contracta.CanonicalEdges() {
		if _, skipped, ok := g.Effective(one, edge, allDeliverable); ok || len(skipped) != 0 {
			t.Fatalf("a lone peer got a %s lane (%v)", edge, skipped)
		}
	}
	ring := &Grid{}
	for i := 0; i < 3; i++ {
		p := contractb.Position{Col: i, Row: 0}
		place(t, ring, "peer-"+string(rune('a'+i)), Preference{Position: &p})
	}
	for slot, wantWest := range map[int]int{1: 3, 2: 1, 3: 2} {
		res, _ := ring.ResOfSlot(slot)
		west, _, ok := ring.Effective(res, contracta.EdgeW, allDeliverable)
		if !ok || west.Slot != wantWest {
			t.Fatalf("ring west(%d) = %v, want slot %d", slot, west, wantWest)
		}
		if _, _, ok := ring.Effective(res, contracta.EdgeS, allDeliverable); ok {
			t.Fatalf("slot %d got a south lane on a one-row map", slot)
		}
	}
}

// TestGrantCarriesOnlyTheDeclaredEdges is B13's "which keys the relay emits".
// The relay emits one key per edge the sidecar DECLARED and found a target for,
// and for no other — which is what keeps a two-edge sidecar's grant
// byte-identical to what a contract-b/3.2 relay produced for it.
func TestGrantCarriesOnlyTheDeclaredEdges(t *testing.T) {
	s, err := New(Options{InsecureNoToken: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	s.mu.Lock()
	for i, pos := range []contractb.Position{
		{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 2, Row: 0},
		{Col: 0, Row: 1}, {Col: 1, Row: 1}, {Col: 2, Row: 1},
	} {
		p := pos
		res := s.grid.Place("peer-"+string(rune('a'+i)), Preference{Position: &p})
		m := s.metaLocked(res.PeerID)
		m.modConnected = true
		m.gameVersion = "0.6.3.1"
		m.simSize = 2000
		m.exportEdges = contracta.CanonicalEdges()
		s.meta[res.PeerID] = m
		s.peers[res.PeerID] = &peer{id: res.PeerID}
	}
	four, _ := s.grid.ResOfSlot(4)
	full := s.grantForLocked(four, contractb.GrantGranted)
	// Two-edge peer: same map, half the keys.
	m := s.meta["peer-d"]
	m.exportEdges = []string{contracta.EdgeE, contracta.EdgeN}
	s.meta["peer-d"] = m
	narrow := s.grantForLocked(four, contractb.GrantGranted)
	s.mu.Unlock()

	if len(full.Neighbours) != 4 {
		t.Fatalf("a four-edge peer's grant carries %d neighbour keys, want 4: %+v",
			len(full.Neighbours), full.Neighbours)
	}
	for _, edge := range contracta.CanonicalEdges() {
		if full.Neighbours[edge] == nil {
			t.Fatalf("the grant has no %s key on a full 3x2 map", edge)
		}
	}
	// Slot 4 sits at (0,1): east is slot 5, west wraps to slot 6, and its column
	// holds only slot 1, so north and south are both slot 1.
	for edge, want := range map[string]int{
		contracta.EdgeE: 5, contracta.EdgeW: 6, contracta.EdgeN: 1, contracta.EdgeS: 1,
	} {
		if got := full.Neighbours[edge].Slot; got != want {
			t.Fatalf("grant %s = slot %d, want slot %d", edge, got, want)
		}
	}
	if len(narrow.Neighbours) != 2 ||
		narrow.Neighbours[contracta.EdgeW] != nil || narrow.Neighbours[contracta.EdgeS] != nil {
		t.Fatalf("a two-edge peer's grant carries %+v; the relay MUST NOT invent a lane a "+
			"sidecar did not declare", narrow.Neighbours)
	}
}
