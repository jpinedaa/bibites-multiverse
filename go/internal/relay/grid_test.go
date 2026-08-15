package relay

import (
	"os"
	"testing"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
)

// allDeliverable is the walk's filter with nothing filtered, so a grid test can
// exercise the geometry on its own.
func allDeliverable(Reservation) string { return "" }

// dark makes the named slots undeliverable, which is what a dead peer, a
// mod-less sidecar or an incompatible install all look like to §8's walk.
func dark(slots ...int) Deliverable {
	set := map[int]bool{}
	for _, s := range slots {
		set[s] = true
	}
	return func(res Reservation) string {
		if set[res.Slot] {
			return contractb.SkipPeerOffline
		}
		return ""
	}
}

func place(t *testing.T, g *Grid, peerID string, pref Preference) Reservation {
	t.Helper()
	return g.Place(peerID, pref)
}

// TestAutoPlacementBuildsThe3x2FromSixOpinionFreeClaims covers §7.2 rule 6 and
// its worked table: fill the first hole in structural order, else GROW THE
// SHORTER AXIS. Six peers with no opinion land in a full 3x2 — the shape the
// exit test needs and the smallest honest two-axis map (§2.1).
func TestAutoPlacementBuildsThe3x2FromSixOpinionFreeClaims(t *testing.T) {
	g := &Grid{}
	want := []struct {
		col, row      int
		width, height int
	}{
		{0, 0, 1, 1}, // empty map
		{1, 0, 2, 1}, // width(1) <= height(1): add a column
		{0, 1, 2, 2}, // width(2) <= height(1) is false: add a row, hole at (1,1)
		{1, 1, 2, 2}, // the first hole
		{2, 0, 3, 2}, // width(2) <= height(2): add a column, hole at (2,1)
		{2, 1, 3, 2}, // the first hole
	}
	for i, w := range want {
		res := place(t, g, "peer-"+string(rune('a'+i)), Preference{})
		if res.Slot != i+1 {
			t.Fatalf("claim %d took slot %d, want %d", i+1, res.Slot, i+1)
		}
		if res.Col != w.col || res.Row != w.row {
			t.Fatalf("claim %d landed at (%d,%d), want (%d,%d)", i+1, res.Col, res.Row, w.col, w.row)
		}
		if g.Width != w.width || g.Height != w.height {
			t.Fatalf("after claim %d the map is %dx%d, want %dx%d",
				i+1, g.Width, g.Height, w.width, w.height)
		}
	}
	if len(g.Holes()) != 0 {
		t.Fatalf("the 3x2 map has holes: %v", g.Holes())
	}
	// Structural order is row ascending, then col ascending (§6.5).
	order := g.Order()
	for i, want := range [][2]int{{0, 0}, {1, 0}, {2, 0}, {0, 1}, {1, 1}, {2, 1}} {
		if order[i].Col != want[0] || order[i].Row != want[1] {
			t.Fatalf("structural order is %v", order)
		}
	}
}

// TestPreferredPositionBuildsTheExitTestLayout covers §7.2 rule 4 and the
// paragraph that follows the worked table: auto-placement reproduces the SHAPE
// but not the assignment, so A RIG THAT WANTS A SPECIFIC LAYOUT NAMES IT.
func TestPreferredPositionBuildsTheExitTestLayout(t *testing.T) {
	g := &Grid{}
	for i, pos := range []contractb.Position{
		{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 2, Row: 0},
		{Col: 0, Row: 1}, {Col: 1, Row: 1}, {Col: 2, Row: 1},
	} {
		p := pos
		res := place(t, g, "peer-slot"+string(rune('1'+i)), Preference{Position: &p})
		if res.Slot != i+1 || res.Col != pos.Col || res.Row != pos.Row {
			t.Fatalf("slot %d landed at (%d,%d), want %+v", res.Slot, res.Col, res.Row, pos)
		}
	}
	if g.Width != 3 || g.Height != 2 {
		t.Fatalf("map is %dx%d, want 3x2", g.Width, g.Height)
	}
	// Row 0 holds slots 1-3 and row 1 holds slots 4-6, which is what the exit
	// test describes and what auto-placement does NOT guarantee.
	for slot, want := range map[int][2]int{1: {0, 0}, 2: {1, 0}, 3: {2, 0}, 4: {0, 1}, 5: {1, 1}, 6: {2, 1}} {
		res, _ := g.ResOfSlot(slot)
		if res.Col != want[0] || res.Row != want[1] {
			t.Fatalf("slot %d is at (%d,%d), want (%d,%d)", slot, res.Col, res.Row, want[0], want[1])
		}
	}
}

// TestUnusablePreferredPositionFallsThrough covers §7.2 rule 4's "anything else
// is ignored": A RAGGED MAP IS NEVER LEGAL, and a lost race is never an error.
func TestUnusablePreferredPositionFallsThrough(t *testing.T) {
	g := &Grid{}
	origin := contractb.Position{Col: 0, Row: 0}
	place(t, g, "a", Preference{Position: &origin})

	// A gap of two columns.
	far := contractb.Position{Col: 3, Row: 0}
	res := place(t, g, "b", Preference{Position: &far})
	if res.Col == 3 {
		t.Fatal("a two-column gap was honoured; growth is always by a whole column or row")
	}
	if g.Width != 2 || g.Height != 1 {
		t.Fatalf("map is %dx%d, want the 2x1 rule 6 produces", g.Width, g.Height)
	}
	// A taken position: the second peer to ask for it falls through and lands
	// somewhere, and the grant names where.
	taken := contractb.Position{Col: 0, Row: 0}
	res = place(t, g, "c", Preference{Position: &taken})
	if res.Col == 0 && res.Row == 0 {
		t.Fatal("a taken position was granted twice")
	}
	if res.Slot != 3 {
		t.Fatalf("the loser of the race took slot %d, want 3; a lost race is never an error", res.Slot)
	}
}

// TestSpliceInsertsAColumnAndRenumbersNobody covers §7.2 rule 5 and §7.3 rule 4:
// a splice shifts COORDINATES and no slot number changes, so no journal entry
// anywhere can be invalidated by it.
func TestSpliceInsertsAColumnAndRenumbersNobody(t *testing.T) {
	g := &Grid{}
	for i := 0; i < 3; i++ {
		p := contractb.Position{Col: i, Row: 0}
		place(t, g, "peer-"+string(rune('a'+i)), Preference{Position: &p})
	}
	// Splice a newcomer immediately after slot 1, on the east axis.
	res := place(t, g, "newcomer", Preference{InsertAfterSlot: 1, InsertAxis: contracta.EdgeE})
	if res.Slot != 4 {
		t.Fatalf("the newcomer took slot %d, want 4 (maxSlotEverIssued + 1)", res.Slot)
	}
	if res.Col != 1 || res.Row != 0 {
		t.Fatalf("the newcomer landed at (%d,%d), want (1,0)", res.Col, res.Row)
	}
	if g.Width != 4 {
		t.Fatalf("the map is %d wide, want 4", g.Width)
	}
	// Slot numbers are untouched; only the coordinates after the anchor moved.
	for slot, wantCol := range map[int]int{1: 0, 4: 1, 2: 2, 3: 3} {
		got, ok := g.ResOfSlot(slot)
		if !ok || got.Col != wantCol {
			t.Fatalf("slot %d is at col %d, want %d (%v)", slot, got.Col, wantCol, g.Order())
		}
	}
	// The predecessor's effective neighbour is the newcomer, and the newcomer's
	// is the old successor: TWO LANES CHANGE.
	one, _ := g.ResOfSlot(1)
	east, _, _ := g.Effective(one, contracta.EdgeE, allDeliverable)
	if east.Slot != 4 {
		t.Fatalf("slot 1 exports east to slot %d, want the newcomer 4", east.Slot)
	}
	four, _ := g.ResOfSlot(4)
	east, _, _ = g.Effective(four, contracta.EdgeE, allDeliverable)
	if east.Slot != 2 {
		t.Fatalf("the newcomer exports east to slot %d, want the old successor 2", east.Slot)
	}
}

// TestSpliceOnTheNorthAxis covers rule 5's other half.
func TestSpliceOnTheNorthAxis(t *testing.T) {
	g := &Grid{}
	for i := 0; i < 2; i++ {
		p := contractb.Position{Col: 0, Row: i}
		place(t, g, "peer-"+string(rune('a'+i)), Preference{Position: &p})
	}
	res := place(t, g, "newcomer", Preference{InsertAfterSlot: 1, InsertAxis: contracta.EdgeN})
	if res.Col != 0 || res.Row != 1 {
		t.Fatalf("the newcomer landed at (%d,%d), want (0,1)", res.Col, res.Row)
	}
	if g.Height != 3 {
		t.Fatalf("the map is %d tall, want 3", g.Height)
	}
	two, _ := g.ResOfSlot(2)
	if two.Row != 2 {
		t.Fatalf("slot 2 was not shifted up: row %d", two.Row)
	}
}

// TestRouteAroundOnTheWidthAxis covers §8: deliverability, not liveness, selects
// the target. A dark slot's lane RE-PAIRS instead of closing, and the bypass is
// reported in walk order.
func TestRouteAroundOnTheWidthAxis(t *testing.T) {
	g := &Grid{}
	for _, pos := range []contractb.Position{
		{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 2, Row: 0},
		{Col: 0, Row: 1}, {Col: 1, Row: 1}, {Col: 2, Row: 1},
	} {
		p := pos
		place(t, g, "peer", Preference{Position: &p})
	}
	four, _ := g.ResOfSlot(4)
	// Slot 5 at (1,1) goes dark. Slot 4's east lane must re-pair to slot 6.
	east, skipped, ok := g.Effective(four, contracta.EdgeE, dark(5))
	if !ok || east.Slot != 6 {
		t.Fatalf("slot 4's east lane is %v (ok=%v), want slot 6", east, ok)
	}
	if len(skipped) != 1 || skipped[0].Slot == nil || *skipped[0].Slot != 5 ||
		skipped[0].Reason != contractb.SkipPeerOffline {
		t.Fatalf("the bypass list is %+v, want one peer_offline entry for slot 5", skipped)
	}
	// §2.1: on an axis of length 2 there is no third slot to re-pair to, so the
	// survivor's export edge on that axis CLOSES. Slot 2 at (1,0) shares its
	// column with slot 5 and nobody else.
	two, _ := g.ResOfSlot(2)
	_, skipped, ok = g.Effective(two, contracta.EdgeN, dark(5))
	if ok {
		t.Fatal("a column of two re-paired around its only other member")
	}
	if len(skipped) != 1 || *skipped[0].Slot != 5 {
		t.Fatalf("the north bypass list is %+v", skipped)
	}
	// And its east lane is untouched: its row still holds three slots, two of
	// them deliverable. That is route-around working correctly against a
	// degenerate axis, not route-around failing.
	east, _, ok = g.Effective(two, contracta.EdgeE, dark(5))
	if !ok || east.Slot != 3 {
		t.Fatalf("slot 2's east lane is %v (ok=%v), want slot 3", east, ok)
	}
}

// TestAHoleIsSkippedLikeADarkSlot covers §2: a hole and a dark slot look
// identical to a router, which is why route-around is what makes a map with
// holes viable.
func TestAHoleIsSkippedLikeADarkSlot(t *testing.T) {
	g := &Grid{}
	for _, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 2, Row: 0}} {
		p := pos
		place(t, g, "peer", Preference{Position: &p})
	}
	// Grow a second row with one occupant, leaving two holes.
	p := contractb.Position{Col: 0, Row: 1}
	place(t, g, "upper", Preference{Position: &p})
	if len(g.Holes()) != 2 {
		t.Fatalf("holes are %v, want (1,1) and (2,1)", g.Holes())
	}
	four, _ := g.ResOfSlot(4)
	_, skipped, ok := g.Effective(four, contracta.EdgeE, allDeliverable)
	if ok {
		t.Fatal("a row whose only other positions are holes produced a target")
	}
	if len(skipped) != 2 || skipped[0].Slot != nil || skipped[0].Reason != contractb.SkipHole {
		t.Fatalf("the bypass list is %+v, want two hole entries with a null slot", skipped)
	}
}

// TestDegenerateShapesHaveNoLanes covers §2.1's table: 1x1 exports nowhere on
// either axis, and a w x 1 map (the M3 ring) has no north lane at all.
func TestDegenerateShapesHaveNoLanes(t *testing.T) {
	g := &Grid{}
	place(t, g, "alone", Preference{})
	one, _ := g.ResOfSlot(1)
	for _, edge := range []string{contracta.EdgeE, contracta.EdgeN} {
		if _, skipped, ok := g.Effective(one, edge, allDeliverable); ok || len(skipped) != 0 {
			t.Fatalf("a lone peer got a %s lane (%v)", edge, skipped)
		}
	}
	// The ring: three slots in one row. Every M3 rule survives as the height-1
	// specialization, and the north edge stays closed with no_peer for the life
	// of the map.
	ring := &Grid{}
	for i := 0; i < 3; i++ {
		p := contractb.Position{Col: i, Row: 0}
		place(t, ring, "peer-"+string(rune('a'+i)), Preference{Position: &p})
	}
	for slot, wantEast := range map[int]int{1: 2, 2: 3, 3: 1} {
		res, _ := ring.ResOfSlot(slot)
		east, _, ok := ring.Effective(res, contracta.EdgeE, allDeliverable)
		if !ok || east.Slot != wantEast {
			t.Fatalf("ring east(%d) = %v, want slot %d", slot, east, wantEast)
		}
		if _, _, ok := ring.Effective(res, contracta.EdgeN, allDeliverable); ok {
			t.Fatalf("slot %d got a north lane on a one-row map", slot)
		}
	}
}

// TestReleaseLeavesAHoleAndNeverReusesTheNumber covers §7.5: surviving slots
// keep their numbers, their positions and their relative order, and
// maxSlotEverIssued never decreases.
func TestReleaseLeavesAHoleAndNeverReusesTheNumber(t *testing.T) {
	g := &Grid{}
	for i := 0; i < 3; i++ {
		p := contractb.Position{Col: i, Row: 0}
		place(t, g, "peer-"+string(rune('a'+i)), Preference{Position: &p})
	}
	res, ok := g.Release(2)
	if !ok || res.PeerID != "peer-b" {
		t.Fatalf("Release(2) = %+v, %v", res, ok)
	}
	if g.Width != 3 || g.Size() != 2 {
		t.Fatalf("the map is %dx%d with %d slots; a release must not reshape it",
			g.Width, g.Height, g.Size())
	}
	holes := g.Holes()
	if len(holes) != 1 || holes[0] != (contractb.Position{Col: 1, Row: 0}) {
		t.Fatalf("holes are %v, want exactly (1,0)", holes)
	}
	one, _ := g.ResOfSlot(1)
	east, skipped, _ := g.Effective(one, contracta.EdgeE, allDeliverable)
	if east.Slot != 3 || len(skipped) != 1 || skipped[0].Reason != contractb.SkipHole {
		t.Fatalf("slot 1 routes to %v past %v; it must skip the hole", east, skipped)
	}
	// A returning peer with the released identity is a NEW slot, not slot 2.
	back := place(t, g, "peer-b", Preference{})
	if back.Slot != 4 {
		t.Fatalf("the returning peer took slot %d, want 4; a released number is retired", back.Slot)
	}
	if back.Col != 1 || back.Row != 0 {
		t.Fatalf("the returning peer filled (%d,%d), want the hole at (1,0)", back.Col, back.Row)
	}
	if _, ok := g.Release(99); ok {
		t.Fatal("Release of an unknown slot reported success")
	}
}

// TestHandoverKeepsTheSlotAndThePosition covers §7.5's new command: a handover
// rebinds the RESERVATION, not the data. The map does not change shape and no
// lane moves.
func TestHandoverKeepsTheSlotAndThePosition(t *testing.T) {
	g := &Grid{}
	for i := 0; i < 3; i++ {
		p := contractb.Position{Col: i, Row: 0}
		place(t, g, "peer-"+string(rune('a'+i)), Preference{Position: &p})
	}
	before := g.Order()
	old, now, ok := g.Handover(2, "peer-replacement")
	if !ok || old.PeerID != "peer-b" || now.PeerID != "peer-replacement" {
		t.Fatalf("Handover = %+v -> %+v (%v)", old, now, ok)
	}
	if now.Slot != 2 || now.Col != 1 || now.Row != 0 {
		t.Fatalf("the handover moved the slot or the position: %+v", now)
	}
	after := g.Order()
	if len(before) != len(after) || g.Width != 3 || g.Height != 1 {
		t.Fatalf("the handover reshaped the map to %dx%d", g.Width, g.Height)
	}
	for i := range before {
		if before[i].Slot != after[i].Slot || before[i].Col != after[i].Col {
			t.Fatalf("a slot moved: %v -> %v", before, after)
		}
	}
	if _, _, ok := g.Handover(99, "nobody"); ok {
		t.Fatal("a handover of an unknown slot reported success")
	}
}

// TestGridIsDurable covers §7.4 and §11 item 5: a reservation that never expires
// is worthless if it lives only in RAM. M4 adds the positions and the map to
// what M3 already persisted.
func TestGridIsDurable(t *testing.T) {
	dir := t.TempDir()
	g, err := LoadGrid(dir)
	if err != nil {
		t.Fatalf("LoadGrid: %v", err)
	}
	for i, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 0, Row: 1}} {
		p := pos
		place(t, g, "peer-slot"+string(rune('1'+i)), Preference{Position: &p})
	}
	g.Release(1)
	if err := g.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	again, err := LoadGrid(dir)
	if err != nil {
		t.Fatalf("LoadGrid: %v", err)
	}
	if again.Width != 2 || again.Height != 2 || again.Size() != 2 {
		t.Fatalf("reloaded map is %dx%d with %d slots", again.Width, again.Height, again.Size())
	}
	if again.MaxSlotEverIssued != 3 {
		t.Fatalf("reloaded maxSlotEverIssued = %d, want 3", again.MaxSlotEverIssued)
	}
	res, ok := again.ResOfSlot(2)
	if !ok || res.Col != 1 || res.Row != 0 || res.PeerID != "peer-slot2" {
		t.Fatalf("reloaded slot 2 is %+v", res)
	}
	if got := place(t, again, "peer-slot4", Preference{}); got.Slot != 4 {
		t.Fatalf("the next slot is %d, want 4", got.Slot)
	}
}

// TestAnM3RingLoadsAsTheHeightOneMap covers §2's claim that the ring is the
// one-row case of the map. The rig's existing ring.json must not read as a
// stack of reservations all at (0,0).
func TestAnM3RingLoadsAsTheHeightOneMap(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"ring":[{"slot":1,"peerId":"a"},{"slot":2,"peerId":"b"},` +
		`{"slot":3,"peerId":"c"}],"maxSlotEverIssued":3}`
	if err := writeFile(dir+"/ring.json", legacy); err != nil {
		t.Fatal(err)
	}
	g, err := LoadGrid(dir)
	if err != nil {
		t.Fatalf("LoadGrid: %v", err)
	}
	if g.Width != 3 || g.Height != 1 {
		t.Fatalf("the M3 ring loaded as %dx%d, want 3x1", g.Width, g.Height)
	}
	for slot, wantCol := range map[int]int{1: 0, 2: 1, 3: 2} {
		res, _ := g.ResOfSlot(slot)
		if res.Col != wantCol || res.Row != 0 {
			t.Fatalf("slot %d loaded at (%d,%d)", slot, res.Col, res.Row)
		}
	}
}

// TestRelayStartsOnAStoreThatHoldsACredential is §3.1's start rule from the
// other side: the relay refuses an empty store (credential_test.go holds that
// half) and starts once one credential exists, because one credential is one
// peer that can join.
func TestRelayStartsOnAStoreThatHoldsACredential(t *testing.T) {
	store, err := peercred.OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if _, err := store.Mint("peer-a", peercred.GrantPeer); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	s, err := New(Options{Credentials: store})
	if err != nil {
		t.Fatalf("a store with a credential in it should start: %v", err)
	}
	if s.SessionID() == "" {
		t.Fatal("the relay minted no relaySessionId; §5.2 needs one per process")
	}
	s.Close()
}

// TestRelayPersistsAnEmptyProductionGrid covers the day-zero half of §7.4:
// ring.json must exist before the first reservation, or the first identity
// backup is necessarily incomplete and cannot be restore-rehearsed safely.
func TestRelayPersistsAnEmptyProductionGrid(t *testing.T) {
	dir := t.TempDir()
	if _, err := os.Stat(dir + "/ring.json"); !os.IsNotExist(err) {
		t.Fatalf("ring.json exists before relay startup: %v", err)
	}

	s, err := New(Options{DataDir: dir, InsecureNoToken: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(dir + "/ring.json"); err != nil {
		t.Fatalf("ring.json was not persisted at startup: %v", err)
	}
	again, err := LoadGrid(dir)
	if err != nil {
		t.Fatalf("LoadGrid: %v", err)
	}
	if again.Width != 0 || again.Height != 0 || again.Size() != 0 || again.MaxSlotEverIssued != 0 {
		t.Fatalf("persisted empty map is %dx%d with %d slots (max %d)",
			again.Width, again.Height, again.Size(), again.MaxSlotEverIssued)
	}
}

// TestReleaseAndHandoverRefuseALivePeersSlot covers §7.5: a live peer with its
// slot taken out from under it would keep claiming, keep being refused, and
// keep a world running with nowhere to export.
func TestReleaseAndHandoverRefuseALivePeersSlot(t *testing.T) {
	s, err := New(Options{InsecureNoToken: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	s.grid.Place("peer-a", Preference{})
	s.peers["peer-a"] = &peer{id: "peer-a"}
	if err := s.ReleaseSlot(1); err == nil {
		t.Fatal("the relay released a live peer's slot")
	}
	if _, _, err := s.HandoverSlot(1, "peer-b"); err == nil {
		t.Fatal("the relay handed over a live peer's slot")
	}
	delete(s.peers, "peer-a")
	if _, _, err := s.HandoverSlot(1, "peer-b"); err != nil {
		t.Fatalf("handing over an offline peer's slot: %v", err)
	}
	if got := s.grid.PeerOfSlot(1); got != "peer-b" {
		t.Fatalf("slot 1 is held by %q after the handover", got)
	}
	if err := s.ReleaseSlot(1); err != nil {
		t.Fatalf("releasing an offline peer's slot: %v", err)
	}
	if s.grid.Size() != 0 {
		t.Fatalf("slot count = %d after the release", s.grid.Size())
	}
}

// TestReserveSlotPreSeedsTheMapInAnyStartOrder covers the LAN case of §7.2: the
// reservations are created before any peer connects, so rule 1 hands each peer
// its slot whenever it arrives and start order stops mattering.
func TestReserveSlotPreSeedsTheMapInAnyStartOrder(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Options{InsecureNoToken: true, DataDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	layout := []contractb.Position{
		{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 2, Row: 0},
		{Col: 0, Row: 1}, {Col: 1, Row: 1}, {Col: 2, Row: 1},
	}
	for i, pos := range layout {
		p := pos
		res, created, err := s.ReserveSlot("slot-"+string(rune('1'+i)), &p)
		if err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
		if !created || res.Slot != i+1 || res.Col != pos.Col || res.Row != pos.Row {
			t.Fatalf("ReserveSlot %d = %+v created=%v", i+1, res, created)
		}
	}
	// Idempotent: a re-run of the pre-seed must not insert a second entry.
	res, created, err := s.ReserveSlot("slot-2", nil)
	if err != nil || created || res.Slot != 2 {
		t.Fatalf("re-reserving slot-2 gave %+v created=%v err=%v", res, created, err)
	}
	if s.grid.Size() != 6 {
		t.Fatalf("slot count = %d after a repeated pre-seed, want 6", s.grid.Size())
	}
	if _, _, err := s.ReserveSlot("", nil); err == nil {
		t.Fatal("an empty peer id was reserved a slot")
	}

	again, err := LoadGrid(dir)
	if err != nil {
		t.Fatalf("LoadGrid: %v", err)
	}
	if again.Width != 3 || again.Height != 2 || again.MaxSlotEverIssued != 6 {
		t.Fatalf("reloaded map is %dx%d (max %d)", again.Width, again.Height, again.MaxSlotEverIssued)
	}
}

// TestReserveThenClaimGivesTheReservedSlot covers rule 1 over a pre-seeded map:
// the peer that connects FIRST must still get the slot its peerId names.
func TestReserveThenClaimGivesTheReservedSlot(t *testing.T) {
	s, err := New(Options{InsecureNoToken: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	for _, id := range []string{"slot-1", "slot-2", "slot-3"} {
		if _, _, err := s.ReserveSlot(id, nil); err != nil {
			t.Fatalf("ReserveSlot(%s): %v", id, err)
		}
	}
	p := &peer{id: "slot-3"}
	res, reason, placed := s.assignLocked(p, contractb.SectorClaim{})
	if res.Slot != 3 || placed || reason != contractb.GrantReclaimed {
		t.Fatalf("assign(slot-3) = %+v reason %s placed %v, want slot 3 reclaimed", res, reason, placed)
	}
}

// TestForwardRecordIsPerSessionAndNotDurable covers §5.2: the record covers ONE
// SESSION only, and a new process cannot prove what an old one forwarded.
func TestForwardRecordIsPerSessionAndNotDurable(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Options{InsecureNoToken: true, DataDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const id = "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12"
	if s.hasForwarded(id) {
		t.Fatal("a fresh relay claims to have forwarded something")
	}
	s.recordForward(id)
	if !s.hasForwarded(id) {
		t.Fatal("the record did not remember an attempted write")
	}
	if s.ForwardedCount() != 1 {
		t.Fatalf("record size = %d, want 1", s.ForwardedCount())
	}
	first := s.SessionID()
	s.Close()

	// A restart on the same data directory: the MAP survives, the RECORD does
	// not, and the session id changes so every outstanding proof goes with it.
	again, err := New(Options{InsecureNoToken: true, DataDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer again.Close()
	if again.SessionID() == first {
		t.Fatal("a restarted relay reused its session id; the proof would outlive the process")
	}
	if again.hasForwarded(id) {
		t.Fatal("the forwarding record survived a restart; §5.2 says it MUST NOT")
	}
	// An empty migrationId is never proof of anything.
	if !again.hasForwarded("") {
		t.Fatal("an unnamed migration was treated as never forwarded")
	}
}

func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}
