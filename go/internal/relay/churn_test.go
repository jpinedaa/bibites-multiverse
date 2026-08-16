package relay

// WP5's tests for contract-b-m4.md §7.2 as amended, §7.5 and §22 B29 —
// PLACEMENT AND ROUTE-AROUND UNDER CHURN — and the synthetic churn harness the
// amendment asks for by name.
//
// WHY THIS FILE EXISTS RATHER THAN A FEW MORE CASES IN grid_test.go. B29 ends
// with a sentence that is unusual for a contract amendment: "What this
// amendment does not do is test itself. maxSlotEverIssued growth has NEVER been
// run at any scale, and neither the widened window nor the suppressed re-claim
// has been measured under churn." M4 placed six known peers and spliced one
// newcomer, by hand, once each. A public map does nothing but the case M4 never
// ran, and Risk 2's accepted cost is that the exit test cannot be re-run
// cheaply — it spends other people's goodwill each time. So the harness runs
// join, leave, return and never-return at rates no human rig can produce, and
// it runs BEFORE WP8 involves anybody.
//
// THE HARNESS OPENS NO SOCKETS FOR THE EXHAUSTION RUN, deliberately. Placement
// is a registry decision: it is decided under one mutex by §7.2's arbitration
// and it is the same decision whether the claim arrived over TLS or over a
// function call. A harness that dialled a WebSocket per event would measure the
// dialer. The frames themselves — the grant a claimant is still owed, the last
// frame of a burst, the broadcast that must not happen — are proved over a real
// wire in the handful of tests at the bottom of this file, with a handful of
// peers.
//
// SCALE. `go test -short` runs a few thousand events, which is the CI mode.
// The default is a few hundred thousand. MULTIVERSE_CHURN_EVENTS overrides
// both, and is how the exhaustion run is driven.

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
	"multiverse/internal/wire"
)

// ---------------------------------------------------------------- B29's placement rules

// TestB29APreferenceThatWouldExtendAnAxisFallsThroughToTheHole is the narrowing
// of §7.2 rule 4, which is the whole of B29's first row.
//
// A preference is an operator's layout on the rig and AN ORDINARY STRANGER'S
// CONFIGURATION FILE on a public map, and one newcomer that extends an axis
// creates `height` (or `width`) positions and fills exactly one of them. A map
// that honoured every such preference would grow a row of holes per join and
// route-around would walk all of them.
func TestB29APreferenceThatWouldExtendAnAxisFallsThroughToTheHole(t *testing.T) {
	g := &Grid{}
	// A 2x2 with one hole at (1,1): three peers, positions named.
	for _, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 0, Row: 1}} {
		p := pos
		place(t, g, "peer-"+fmt.Sprint(pos.Col, pos.Row), Preference{Position: &p})
	}
	if got := g.Holes(); len(got) != 1 || got[0] != (contractb.Position{Col: 1, Row: 1}) {
		t.Fatalf("holes are %v, want exactly (1,1)", got)
	}

	// A newcomer asks to extend the map by a whole column. Under contract-b/3.x
	// it would have been granted (2,0) and the map would be 3x2 with THREE
	// holes. Under B29 it is ignored and rule 6 fills the hole.
	extend := contractb.Position{Col: 2, Row: 0}
	res := place(t, g, "newcomer", Preference{Position: &extend})
	if res.Col != 1 || res.Row != 1 {
		t.Fatalf("the newcomer landed at (%d,%d), want the hole at (1,1): a preference that "+
			"would extend an axis while a hole exists must fall through to rule 6 (§22, B29)",
			res.Col, res.Row)
	}
	if g.Width != 2 || g.Height != 2 {
		t.Fatalf("the map grew to %dx%d; holes come before growth", g.Width, g.Height)
	}
	if len(g.Holes()) != 0 {
		t.Fatalf("the map still has holes: %v", g.Holes())
	}

	// And with the rectangle full, the SAME preference is honoured. The rule
	// narrows growth, it does not forbid it.
	res = place(t, g, "next", Preference{Position: &extend})
	if res.Col != 2 || res.Row != 0 || g.Width != 3 {
		t.Fatalf("a growth preference was refused on a FULL map: landed (%d,%d) in %dx%d",
			res.Col, res.Row, g.Width, g.Height)
	}

	// The hole case inside the rectangle is unchanged and always usable: the new
	// column left (2,1) empty, and a peer may name it.
	inside := contractb.Position{Col: 2, Row: 1}
	res = place(t, g, "into-the-hole", Preference{Position: &inside})
	if res.Col != 2 || res.Row != 1 {
		t.Fatalf("naming an existing hole landed at (%d,%d)", res.Col, res.Row)
	}
}

// TestB29TheJoinKitLayoutSurvivesTheNarrowing is the contract's own test of the
// narrowing, quoted from §7.2: "The join kit still builds the rig's layout
// under B29's narrowing, and THAT IS THE TEST OF THE NARROWING." At every
// moment one of the six sidecars asks to extend, the rectangle is full.
//
// This is the case that would have caught the narrowing if it had been written
// carelessly — a rule that refused every growth preference would silently
// re-shape the living deployment on its next relay restart.
func TestB29TheJoinKitLayoutSurvivesTheNarrowing(t *testing.T) {
	g := &Grid{}
	layout := []contractb.Position{
		{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 2, Row: 0},
		{Col: 0, Row: 1}, {Col: 1, Row: 1}, {Col: 2, Row: 1},
	}
	for i, pos := range layout {
		p := pos
		res := place(t, g, "peer-main-slot"+strconv.Itoa(i+1), Preference{Position: &p})
		if res.Slot != i+1 || res.Col != pos.Col || res.Row != pos.Row {
			t.Fatalf("the join kit's claim %d landed at slot %d (%d,%d), want slot %d %+v — "+
				"B29 must not refuse a claim in this sequence",
				i+1, res.Slot, res.Col, res.Row, i+1, pos)
		}
	}
	if g.Width != 3 || g.Height != 2 || len(g.Holes()) != 0 {
		t.Fatalf("the rig's layout came out %dx%d with holes %v, want a full 3x2",
			g.Width, g.Height, g.Holes())
	}
	// Row 0 holds slots 1-3 and row 1 holds slots 4-6, exactly as before.
	for slot, want := range map[int][2]int{1: {0, 0}, 2: {1, 0}, 3: {2, 0}, 4: {0, 1}, 5: {1, 1}, 6: {2, 1}} {
		res, _ := g.ResOfSlot(slot)
		if res.Col != want[0] || res.Row != want[1] {
			t.Fatalf("slot %d is at (%d,%d), want (%d,%d)", slot, res.Col, res.Row, want[0], want[1])
		}
	}
}

// TestB29GrowsTheShorterAxisAndStaysNearSquare covers the axis rule and the
// reason B29 finally writes down for it: THE CYCLE LENGTH ON EACH AXIS IS WHAT
// GENETIC MIXING DEPENDS ON, so a map stretched along one axis is a map whose
// other axis has nothing to route around (§2.1).
//
// The assertion is the property rather than the sequence: at no point during
// forty opinion-free joins do the two axes differ by more than one.
func TestB29GrowsTheShorterAxisAndStaysNearSquare(t *testing.T) {
	g := &Grid{}
	for i := 0; i < 40; i++ {
		place(t, g, "peer-"+strconv.Itoa(i), Preference{})
		if d := g.Width - g.Height; d > 1 || d < -1 {
			t.Fatalf("after %d joins the map is %dx%d: growing the shorter axis must keep the "+
				"rectangle within one of square", i+1, g.Width, g.Height)
		}
		if g.Width*g.Height < g.Size() {
			t.Fatalf("after %d joins the rectangle %dx%d cannot hold %d slots",
				i+1, g.Width, g.Height, g.Size())
		}
	}
	// Forty peers in a near-square map: every row is a cycle worth routing
	// around, which is the property the rule exists for.
	if g.Width < 6 || g.Height < 6 {
		t.Fatalf("forty peers produced a %dx%d map", g.Width, g.Height)
	}
}

// ---------------------------------------------------------------- the churn harness

// churnRig drives §7.2's arbitration directly. It is the relay's own registry
// path — assignLocked, ReleaseSlot, ReserveSlot — with the publisher stopped,
// so a million events cost no goroutines, no timers and no sockets.
//
// STOPPING THE PUBLISHER IS THE ONE TRICK HERE and it is worth naming: this rig
// registers fake peers in s.peers so that ReleaseSlot's "is it live" rule is
// exercised for real, and those peers have no connection to send to. Close()
// stops the publish loop before the first one is registered, so nothing ever
// tries.
type churnRig struct {
	t   *testing.T
	srv *Server
	rng *rand.Rand

	// live is who the relay would call connected; claimed is whether that
	// identity has already answered a claim ON ITS CURRENT CONNECTION, which is
	// what tells §7.2 rule 1's "reclaimed" from "updated".
	live    map[string]bool
	claimed map[string]bool
	// home is the reservation an identity was last granted, so a return can be
	// checked against it rather than merely observed.
	home map[string]Reservation
	// retired is every slot number an operator released. Its address must never
	// be handed out again and ResOfSlot must answer no for the life of the map.
	retired map[int]string
	// placements counts every NEW reservation minted. It is the exact predicted
	// value of maxSlotEverIssued, which is the strongest form the bound can take:
	// slot space grows by one per new placement and by nothing else, ever.
	placements int

	joins, leaves, returns, neverReturns, reclaims int
	// quietReclaims counts re-claims the relay answered "updated" with nothing
	// structural changed — B29's suppressed broadcast, counted at the source.
	quietReclaims int
}

func newChurnRig(t *testing.T, seed int64) *churnRig {
	t.Helper()
	srv, err := New(Options{
		InsecureNoToken: true,
		// A million registry changes would otherwise write a million operator
		// warnings, and the one thing a churn harness must not do is make its own
		// output unreadable.
		Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Before anything registers a connectionless peer.
	srv.Close()
	t.Cleanup(srv.Close)
	return &churnRig{
		t: t, srv: srv, rng: rand.New(rand.NewSource(seed)),
		live: map[string]bool{}, claimed: map[string]bool{},
		home: map[string]Reservation{}, retired: map[int]string{},
	}
}

// claim runs one SECTOR_CLAIM through §7.2's arbitration and returns what the
// peer would have been granted.
func (r *churnRig) claim(id string, claim contractb.SectorClaim) (Reservation, string) {
	r.t.Helper()
	p := &peer{id: id, claimed: r.claimed[id]}
	r.srv.mu.Lock()
	res, reason, placed := r.srv.assignLocked(p, claim)
	r.srv.mu.Unlock()
	r.claimed[id] = true
	if placed {
		r.placements++
	}
	// NO CLAIM IS EVER LOST. §7.2 has no position_taken refusal: a refusal would
	// leave a peer with no world in the map and nothing useful to do about it,
	// so every claim that reaches arbitration comes back with a slot.
	if res.Slot < 1 {
		r.t.Fatalf("a claim from %s was answered with no slot (%+v, reason %s)", id, res, reason)
	}
	return res, reason
}

// join brings a NEW identity onto the map, optionally with a preference.
func (r *churnRig) join(id string, pref *contractb.Position) Reservation {
	r.t.Helper()
	r.srv.mu.Lock()
	r.srv.peers[id] = &peer{id: id}
	r.srv.mu.Unlock()
	r.live[id], r.claimed[id] = true, false
	res, reason := r.claim(id, contractb.SectorClaim{
		PreferredPosition: pref, SimulationSize: 2000, ModConnected: true,
		ExportEdges: []string{"E", "N", "W", "S"}, BorderEdges: []string{"E", "N", "W", "S"},
	})
	if reason != contractb.GrantGranted {
		r.t.Fatalf("a new identity %s was answered %q, want granted", id, reason)
	}
	r.home[id] = res
	r.joins++
	return res
}

// leave takes a peer off the wire. THE RESERVATION IS UNTOUCHED: it never
// expires, and that is the whole of "return needs no insertion".
func (r *churnRig) leave(id string) {
	r.t.Helper()
	r.srv.mu.Lock()
	delete(r.srv.peers, id)
	r.srv.mu.Unlock()
	r.live[id], r.claimed[id] = false, false
	r.leaves++
}

// back is a returning peer. It must land on its own slot AND its own position,
// with reason "reclaimed" on the first claim of the new connection.
func (r *churnRig) back(id string) {
	r.t.Helper()
	r.srv.mu.Lock()
	r.srv.peers[id] = &peer{id: id}
	r.srv.mu.Unlock()
	r.live[id], r.claimed[id] = true, false
	res, reason := r.claim(id, contractb.SectorClaim{
		SimulationSize: 2000, ModConnected: true,
		ExportEdges: []string{"E", "N", "W", "S"}, BorderEdges: []string{"E", "N", "W", "S"},
	})
	if reason != contractb.GrantReclaimed {
		r.t.Fatalf("%s returned and was answered %q, want reclaimed", id, reason)
	}
	was := r.home[id]
	if res.Slot != was.Slot || res.Col != was.Col || res.Row != was.Row {
		r.t.Fatalf("%s returned to slot %d (%d,%d) but left from slot %d (%d,%d): a reservation "+
			"is keyed on peerId and NEVER EXPIRES (§7.2 rule 1)",
			id, res.Slot, res.Col, res.Row, was.Slot, was.Col, was.Row)
	}
	r.returns++
}

// reclaim is the live peer that claims again — DQ3's slot 6, whose measured
// time scale wandered and which issued 64 claims in one day for nothing.
func (r *churnRig) reclaim(id string, mutate bool) {
	r.t.Helper()
	claim := contractb.SectorClaim{
		SimulationSize: 2000, ModConnected: true,
		ExportEdges: []string{"E", "N", "W", "S"}, BorderEdges: []string{"E", "N", "W", "S"},
	}
	if mutate {
		claim.ExportEdges = []string{"E", "N"}
	}
	r.srv.mu.Lock()
	before := r.srv.claimShapeLocked(id)
	r.srv.mu.Unlock()

	res, reason := r.claim(id, claim)
	if reason != contractb.GrantUpdated {
		r.t.Fatalf("a live peer's repeat claim was answered %q, want updated", reason)
	}
	was := r.home[id]
	if res.Slot != was.Slot || res.Col != was.Col || res.Row != was.Row {
		r.t.Fatalf("a repeat claim moved %s from slot %d (%d,%d) to slot %d (%d,%d)",
			id, was.Slot, was.Col, was.Row, res.Slot, res.Col, res.Row)
	}
	// The harness applies the meta the claim carries, because assignLocked does
	// not: onSectorClaim owns that half, and the shape comparison is about it.
	r.srv.mu.Lock()
	m := r.srv.metaLocked(id)
	m.simSize, m.modConnected = claim.SimulationSize, claim.ModConnected
	m.exportEdges = append([]string(nil), claim.ExportEdges...)
	m.borderEdges = append([]string(nil), claim.BorderEdges...)
	after := r.srv.claimShapeLocked(id)
	r.srv.mu.Unlock()
	if before.same(after) {
		r.quietReclaims++
	}
	r.reclaims++
}

// neverReturn is the fourth pattern and the one the map cannot heal by itself:
// a peer that left and will not come back, and the operator's answer for it.
func (r *churnRig) neverReturn(id string) {
	r.t.Helper()
	if r.live[id] {
		r.leave(id)
	}
	was := r.home[id]
	if err := r.srv.ReleaseSlot(was.Slot); err != nil {
		r.t.Fatalf("releasing %s's slot %d: %v", id, was.Slot, err)
	}
	r.retired[was.Slot] = id
	delete(r.home, id)
	delete(r.claimed, id)
	delete(r.live, id)
	r.neverReturns++
}

// population is how many identities hold a reservation right now.
func (r *churnRig) population() int {
	r.srv.mu.Lock()
	defer r.srv.mu.Unlock()
	return r.srv.grid.Size()
}

// sweep is THE INVARIANT SWEEP, and it is the reason a churn harness is worth
// building at all: the events are only interesting because something is
// checking the map after every one of them.
//
// Seven invariants, and every one of them is a property some other part of the
// system has already been built to rely on:
//
//  1. NO TWO PEERS HOLD ONE SLOT. Routing is on the slot (§5).
//  2. No peer holds two slots. A peer holds at most one (§7.5).
//  3. NO TWO SLOTS HOLD ONE POSITION. The walk would visit one of them twice
//     and never the other (§8).
//  4. Every reservation is inside the rectangle. A ragged map is never legal
//     (§7.2 rule 4), and Grid.HasHole's arithmetic depends on it.
//  5. maxSlotEverIssued is above every live slot number and NEVER DECREASES.
//  6. A released address is never re-issued: SLOT_VACANT is a permanent answer
//     and therefore a valid proof of non-delivery (§6.8).
//  7. The reservation list is in structural order, because everything published
//     reads from it (§6.5).
func (r *churnRig) sweep(where string) {
	r.t.Helper()
	r.srv.mu.Lock()
	defer r.srv.mu.Unlock()
	g := r.srv.grid

	bySlot := map[int]string{}
	byPeer := map[string]int{}
	byPos := map[contractb.Position]int{}
	prev := contractb.Position{Col: -1, Row: -1}
	for _, res := range g.Slots {
		if other, dup := bySlot[res.Slot]; dup {
			r.t.Fatalf("%s: slot %d is held by both %s and %s — TWO PEERS HOLD ONE SLOT",
				where, res.Slot, other, res.PeerID)
		}
		bySlot[res.Slot] = res.PeerID
		if other, dup := byPeer[res.PeerID]; dup {
			r.t.Fatalf("%s: %s holds slots %d and %d; a peer holds at most one",
				where, res.PeerID, other, res.Slot)
		}
		byPeer[res.PeerID] = res.Slot
		if other, dup := byPos[res.Position()]; dup {
			r.t.Fatalf("%s: slots %d and %d both sit at (%d,%d)",
				where, other, res.Slot, res.Col, res.Row)
		}
		byPos[res.Position()] = res.Slot
		if res.Col < 0 || res.Row < 0 || res.Col >= g.Width || res.Row >= g.Height {
			r.t.Fatalf("%s: slot %d is at (%d,%d), outside the %dx%d rectangle",
				where, res.Slot, res.Col, res.Row, g.Width, g.Height)
		}
		if res.Slot < 1 || res.Slot > g.MaxSlotEverIssued {
			r.t.Fatalf("%s: slot %d is outside 1..maxSlotEverIssued (%d)",
				where, res.Slot, g.MaxSlotEverIssued)
		}
		if who, retired := r.retired[res.Slot]; retired {
			r.t.Fatalf("%s: slot %d was released (it was %s) and has been re-issued to %s — "+
				"SLOT_VACANT would become a lie (§6.8, §7.5)", where, res.Slot, who, res.PeerID)
		}
		if res.Row < prev.Row || (res.Row == prev.Row && res.Col <= prev.Col) {
			r.t.Fatalf("%s: the reservation list is not in structural order at slot %d (%d,%d) "+
				"after (%d,%d)", where, res.Slot, res.Col, res.Row, prev.Col, prev.Row)
		}
		prev = res.Position()
	}
	// maxSlotEverIssued is exactly the number of placements this rig made.
	if g.MaxSlotEverIssued != r.placements {
		r.t.Fatalf("%s: maxSlotEverIssued is %d after %d placements; slot space must grow by "+
			"exactly one per NEW reservation and by nothing else",
			where, g.MaxSlotEverIssued, r.placements)
	}
	// The cheap hole test and the expensive one must agree, because rule 4's
	// narrowing is decided by the cheap one on every claim.
	if g.HasHole() != (len(g.Holes()) > 0) {
		r.t.Fatalf("%s: HasHole()=%v but Holes()=%v", where, g.HasHole(), g.Holes())
	}
}

// sweepRetired is the same invariant as the loop above, walked from the other
// end: EVERY released address answers nothing, for ever, which is what makes an
// orphaned journal entry's SLOT_VACANT a proof rather than a guess.
//
// IT IS SEPARATE FROM sweep() BECAUSE IT IS O(RELEASED ADDRESSES) AND sweep()
// IS O(LIVE SLOTS). The retired set grows for the whole run — 50 million events
// retire millions of addresses — so running this inside the per-event sweep
// makes the harness quadratic in its own length, and the first attempt at a
// long run died of exactly that. The per-reservation check inside sweep()
// already catches a re-issued address the moment it is issued; this one is the
// end-of-run proof over the whole history.
func (r *churnRig) sweepRetired(where string) {
	r.t.Helper()
	r.srv.mu.Lock()
	defer r.srv.mu.Unlock()
	for slot, who := range r.retired {
		if _, ok := r.srv.grid.ResOfSlot(slot); ok {
			r.t.Fatalf("%s: released slot %d (was %s) is reserved again", where, slot, who)
		}
		if slot > r.srv.grid.MaxSlotEverIssued {
			r.t.Fatalf("%s: released slot %d is above maxSlotEverIssued %d",
				where, slot, r.srv.grid.MaxSlotEverIssued)
		}
	}
}

// churnEvents is the harness's scale. -short is the CI mode; the default is a
// run a developer will sit through; MULTIVERSE_CHURN_EVENTS is the exhaustion
// dial and is what the WP5 run was driven with.
func churnEvents(t *testing.T) int {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv("MULTIVERSE_CHURN_EVENTS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			t.Fatalf("MULTIVERSE_CHURN_EVENTS=%q is not a positive integer", v)
		}
		return n
	}
	if testing.Short() {
		return 5000
	}
	return 200000
}

// churnPeers is the top of the population wave. Forty is a public map several
// times the size of the living deployment and small enough that the default run
// stays a few seconds; MULTIVERSE_CHURN_PEERS raises it for an exhaustion run
// that wants a bigger rectangle.
func churnPeers(t *testing.T) int {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv("MULTIVERSE_CHURN_PEERS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 8 {
			t.Fatalf("MULTIVERSE_CHURN_PEERS=%q is not an integer of at least 8", v)
		}
		return n
	}
	return 40
}

// TestChurnHarnessRunsJoinLeaveReturnAndNeverReturnToExhaustion is WP5's
// deliverable: the four patterns §13 item 3 names, plus mixed storms, at rates
// no human rig can produce, with the invariant sweep after every event.
//
// The population is held between a floor and a ceiling on purpose. A harness
// that only ever joined would measure a growing map and nothing else; a public
// map's interesting state is a POPULATION THAT CHURNS AROUND A SIZE, which is
// what makes holes, returns and re-fills collide.
func TestChurnHarnessRunsJoinLeaveReturnAndNeverReturnToExhaustion(t *testing.T) {
	events := churnEvents(t)
	r := newChurnRig(t, 20260811)

	// THE POPULATION IS NOT HELD AT ONE SIZE, because a map that only ever sits
	// at forty peers proves the axis rule at exactly one shape. The ceiling
	// walks a triangular wave between the floor and churnPeers, so a single run
	// grows the rectangle, fills it, starves it back down through a long stretch
	// of holes, and grows it again. Growth is where the axis rule is decided;
	// contraction is where holes-before-growth is.
	const floorPop = 6
	maxPop := churnPeers(t)
	const phases = 8
	ceilingAt := func(n int) int {
		tri := math.Abs(math.Mod(float64(n)/float64(events)*phases, 2) - 1)
		return floorPop + int(float64(maxPop-floorPop)*(1-tri))
	}
	// sweepEvery keeps the sweep affordable at exhaustion scale without ever
	// letting a run go unswept: every event up to 50k, then every hundredth,
	// and always the last one.
	sweepEvery := 1
	if events > 50000 {
		sweepEvery = 100
	}

	dark := []string{}    // identities with a reservation and no connection
	liveIDs := []string{} // identities with a reservation and a connection
	next := 0
	maxSlotSamples := []int{}
	storms := 0

	pick := func(from []string) (string, []string, bool) {
		if len(from) == 0 {
			return "", from, false
		}
		i := r.rng.Intn(len(from))
		id := from[i]
		from[i] = from[len(from)-1]
		return id, from[:len(from)-1], true
	}

	// The mix is a WEIGHTED PICK OVER FEASIBLE ACTIONS rather than a cascade of
	// guarded cases, and the difference matters: a cascade quietly turns every
	// infeasible roll into whichever branch is next, so the first branch whose
	// guard fails often — "join, unless the map is full" — silently doubles the
	// weight of the branch after it. The first version of this harness did
	// exactly that and drove the population to 90% dark, which is a map, but not
	// the map a public relay has.
	const (
		aJoin = iota
		aLeave
		aReturn
		aReclaim
		aNever
		aStorm
	)
	weight := map[int]int{aJoin: 26, aLeave: 22, aReturn: 26, aReclaim: 14, aNever: 8, aStorm: 4}

	start := time.Now()
	peakPop, peakW, peakH, peakHoles := 0, 0, 0, 0
	for n := 1; n <= events; n++ {
		pop := len(liveIDs) + len(dark)
		ceilPop := ceilingAt(n)
		feasible := []int{}
		total := 0
		add := func(a int) { feasible = append(feasible, a); total += weight[a] }
		switch {
		case pop < floorPop:
			// A map below its floor takes newcomers and nothing else, so a run
			// can never collapse to an empty map and stop proving anything.
			add(aJoin)
		default:
			if pop < ceilPop {
				add(aJoin)
			}
			if len(liveIDs) > floorPop/2 {
				add(aLeave)
				add(aReclaim)
				add(aStorm)
			}
			if len(dark) > 0 {
				add(aReturn)
				if pop > floorPop {
					add(aNever)
				}
			}
		}
		if total == 0 {
			add(aJoin)
		}
		roll := r.rng.Intn(total)
		action := feasible[len(feasible)-1]
		for _, a := range feasible {
			if roll < weight[a] {
				action = a
				break
			}
			roll -= weight[a]
		}

		switch action {
		case aJoin:
			// JOIN. One in four newcomers arrives with a preferred position it
			// is content to lose — a stranger's configuration file, which is
			// exactly the case B29 narrowed rule 4 for.
			id := "peer-" + strconv.Itoa(next)
			next++
			var pref *contractb.Position
			if r.rng.Intn(4) == 0 {
				r.srv.mu.Lock()
				w, h := r.srv.grid.Width, r.srv.grid.Height
				r.srv.mu.Unlock()
				p := contractb.Position{Col: w, Row: r.rng.Intn(h + 1)}
				if r.rng.Intn(2) == 0 {
					p = contractb.Position{Col: r.rng.Intn(w + 1), Row: h}
				}
				pref = &p
			}
			r.join(id, pref)
			liveIDs = append(liveIDs, id)
		case aLeave:
			// LEAVE. The reservation stays; the peer becomes a bypassed position
			// its neighbours route around.
			id, rest, ok := pick(liveIDs)
			liveIDs = rest
			if ok {
				r.leave(id)
				dark = append(dark, id)
			}
		case aReturn:
			// RETURN. Rule 1, and the whole of "return needs no insertion".
			id, rest, ok := pick(dark)
			dark = rest
			if ok {
				r.back(id)
				liveIDs = append(liveIDs, id)
			}
		case aReclaim:
			// RE-CLAIM. One in five carries a structural change, so the harness
			// exercises both sides of B29's suppression rather than only the
			// quiet one.
			id := liveIDs[r.rng.Intn(len(liveIDs))]
			r.reclaim(id, r.rng.Intn(5) == 0)
		case aNever:
			// NEVER RETURN. The operator's answer, and the only way a position
			// comes back (§7.5).
			id, rest, ok := pick(dark)
			dark = rest
			if ok {
				r.neverReturn(id)
			}
		case aStorm:
			// A STORM: as many registry changes back to back as the map can be
			// made to produce, which is the mixed case §7.2's "No storm"
			// paragraph is written about.
			storms++
			for i := 0; i < 12 && len(liveIDs) > 0; i++ {
				id, rest, ok := pick(liveIDs)
				liveIDs = rest
				if !ok {
					break
				}
				r.leave(id)
				r.back(id)
				liveIDs = append(liveIDs, id)
			}
		}
		if n%sweepEvery == 0 {
			r.sweep(fmt.Sprintf("event %d", n))
		}
		if n%(events/10+1) == 0 {
			maxSlotSamples = append(maxSlotSamples, r.srv.MaxSlotEverIssued())
		}
		if n%97 == 0 {
			r.srv.mu.Lock()
			w, h, size := r.srv.grid.Width, r.srv.grid.Height, r.srv.grid.Size()
			r.srv.mu.Unlock()
			if size > peakPop {
				peakPop = size
			}
			if w > peakW {
				peakW = w
			}
			if h > peakH {
				peakH = h
			}
			if holes := w*h - size; holes > peakHoles {
				peakHoles = holes
			}
		}
	}
	elapsed := time.Since(start)
	r.sweep("final")
	r.sweepRetired("final")

	// ------------------------------------------------------------ the numbers
	shape := r.srv.MapShape()
	holes := r.srv.Holes()
	maxSlot := r.srv.MaxSlotEverIssued()
	t.Logf("churn harness: %d events in %s (%.0f events/s)",
		events, elapsed.Truncate(time.Millisecond), float64(events)/elapsed.Seconds())
	t.Logf("  patterns: %d joins, %d leaves, %d returns, %d never-returns, %d re-claims "+
		"(%d of them structurally quiet), %d storms",
		r.joins, r.leaves, r.returns, r.neverReturns, r.reclaims, r.quietReclaims, storms)
	t.Logf("  map: %dx%d, %d slots, %d holes, population %d live + %d dark",
		shape.Width, shape.Height, r.population(), len(holes), len(liveIDs), len(dark))
	t.Logf("  peaks over the run: %d slots, %dx%d rectangle, %d simultaneous holes "+
		"(population ceiling walked %d..%d over %d phases)",
		peakPop, peakW, peakH, peakHoles, floorPop, maxPop, phases)
	t.Logf("  maxSlotEverIssued: %d after %d placements; trajectory %v",
		maxSlot, r.placements, maxSlotSamples)
	t.Logf("  retired addresses: %d, and none was re-issued", len(r.retired))

	// ------------------------------------------------------------ the bounds
	//
	// THE BOUND ON SLOT SPACE, stated as arithmetic rather than as a hope.
	// maxSlotEverIssued is exactly the number of NEW placements: a join costs
	// one, and a leave, a return, a re-claim and a storm cost NOTHING. Only a
	// release-then-rejoin buys a second number for one identity, and the harness
	// never rejoins a released identity.
	if maxSlot != r.placements || maxSlot != r.joins {
		t.Fatalf("maxSlotEverIssued %d, placements %d, joins %d: they must be the same number",
			maxSlot, r.placements, r.joins)
	}
	if r.returns == 0 || r.leaves == 0 || r.neverReturns == 0 || r.reclaims == 0 {
		t.Fatalf("the mix did not exercise all four patterns: %d leaves, %d returns, "+
			"%d never-returns, %d re-claims", r.leaves, r.returns, r.neverReturns, r.reclaims)
	}
	// The map stayed near square through all of it: the axis rule is not a
	// property of a quiet map.
	if d := shape.Width - shape.Height; d > 1 || d < -1 {
		t.Logf("note: the final map is %dx%d — a release can leave the rectangle wider than "+
			"the population needs, which is holes-before-growth working", shape.Width, shape.Height)
	}
	// Every position the map holds is either occupied or a hole a newcomer would
	// be given before any axis grew.
	if shape.Width*shape.Height != r.population()+len(holes) {
		t.Fatalf("the %dx%d rectangle holds %d slots and %d holes, which is not %d positions",
			shape.Width, shape.Height, r.population(), len(holes), shape.Width*shape.Height)
	}
}

// TestChurnFillsHolesBeforeExtendingAnAxis is the property B29 asks for, driven
// by the harness rather than by a hand-built map: after any event, if the map
// has a hole, the NEXT newcomer takes one — whatever it asked for.
func TestChurnFillsHolesBeforeExtendingAnAxis(t *testing.T) {
	r := newChurnRig(t, 4242)
	for i := 0; i < 12; i++ {
		r.join("peer-"+strconv.Itoa(i), nil)
	}
	tested := 0
	for i := 0; i < 200; i++ {
		// Punch a hole by retiring somebody, then let a newcomer in with a
		// preference that would have extended an axis.
		victim := "peer-" + strconv.Itoa(i)
		if _, held := r.home[victim]; !held {
			continue
		}
		r.neverReturn(victim)
		r.srv.mu.Lock()
		w, h := r.srv.grid.Width, r.srv.grid.Height
		holesBefore := len(r.srv.grid.Holes())
		r.srv.mu.Unlock()
		if holesBefore == 0 {
			continue
		}
		grow := contractb.Position{Col: w, Row: 0}
		id := "newcomer-" + strconv.Itoa(i)
		res := r.join(id, &grow)
		r.srv.mu.Lock()
		nowW, nowH := r.srv.grid.Width, r.srv.grid.Height
		r.srv.mu.Unlock()
		if nowW != w || nowH != h {
			t.Fatalf("the map grew from %dx%d to %dx%d while %d hole(s) were open",
				w, h, nowW, nowH, holesBefore)
		}
		if res.Col >= w || res.Row >= h {
			t.Fatalf("%s landed at (%d,%d), outside the %dx%d rectangle it was supposed to fill",
				id, res.Col, res.Row, w, h)
		}
		tested++
		r.sweep("hole-fill " + strconv.Itoa(i))
	}
	if tested < 5 {
		t.Fatalf("the case was only exercised %d times", tested)
	}
}

// TestANeverReturningPeersAddressIsRetiredAndItsPositionIsOffered is DQ3's
// split, end to end: THE POSITION COMES BACK AND THE ADDRESS DOES NOT.
//
// It is the pair of facts an operator has to hold at once, and the pair the
// slot-space policy is written to state, so it is asserted as a pair.
func TestANeverReturningPeersAddressIsRetiredAndItsPositionIsOffered(t *testing.T) {
	r := newChurnRig(t, 7)
	for i := 0; i < 6; i++ {
		r.join("peer-"+strconv.Itoa(i), nil)
	}
	gone := "peer-2"
	was := r.home[gone]

	// While it is merely dark, NOTHING happens to its position: the reservation
	// never expires and a newcomer must not be given it.
	r.leave(gone)
	newcomer := r.join("newcomer-while-dark", nil)
	if newcomer.Col == was.Col && newcomer.Row == was.Row {
		t.Fatal("a newcomer was given a dark peer's position; the reservation never expires " +
			"and a return needs no insertion (§7.2 rule 1)")
	}
	// And the dark peer comes back to exactly where it was.
	r.back(gone)
	r.leave(gone)

	// Now the operator answers for a peer that will not come back.
	r.neverReturn(gone)
	if _, ok := r.srv.grid.ResOfSlot(was.Slot); ok {
		t.Fatal("the released slot still names a reservation")
	}
	before := r.srv.MaxSlotEverIssued()

	// THE POSITION IS OFFERED to the next newcomer, before any axis grows.
	filler := r.join("newcomer-after-release", nil)
	if filler.Col != was.Col || filler.Row != was.Row {
		t.Fatalf("the next newcomer landed at (%d,%d), want the released position (%d,%d)",
			filler.Col, filler.Row, was.Col, was.Row)
	}
	// THE ADDRESS IS NOT. It got the next number, not the released one.
	if filler.Slot == was.Slot {
		t.Fatalf("the released slot number %d was re-issued; SLOT_VACANT would become a lie "+
			"about non-delivery (§6.8, §7.5)", was.Slot)
	}
	if filler.Slot != before+1 || r.srv.MaxSlotEverIssued() != before+1 {
		t.Fatalf("the newcomer took slot %d with maxSlotEverIssued %d, want %d",
			filler.Slot, r.srv.MaxSlotEverIssued(), before+1)
	}
	// And the departed identity, if it ever did come back, is a NEW slot at
	// whatever the map has left — which is what makes a release irreversible and
	// is why the operator's report has to say so.
	r.claimed[gone] = false
	r.srv.mu.Lock()
	r.srv.peers[gone] = &peer{id: gone}
	r.srv.mu.Unlock()
	res, reason := r.claim(gone, contractb.SectorClaim{SimulationSize: 2000, ModConnected: true})
	if reason != contractb.GrantGranted || res.Slot == was.Slot {
		t.Fatalf("a released identity returned as slot %d with reason %q, want a NEW slot",
			res.Slot, reason)
	}
	r.home[gone] = res
	r.sweep("after the released identity returned")
}

// TestAContractingMapKeepsItsRectangleAndPaysForItInSkipLists pins a behaviour
// the churn harness surfaced and the contract does not name.
//
// THE RECTANGLE IS MONOTONE TOO. §7.2 grows it and §7.5's release explicitly
// leaves it alone — "the map stays WxH and the position becomes a hole" — so a
// public map that grew to forty peers and churned down to six keeps a
// forty-peer rectangle full of holes. maxSlotEverIssued's monotonicity is
// stated, argued and accepted (D8, D12, B29); the rectangle's is neither stated
// nor argued anywhere, it simply falls out of there being no rule that shrinks
// it.
//
// WHAT IT COSTS IS NOT THE WALK — §8's walk stops at the first deliverable slot
// and visits at most `axis length` positions either way. IT IS THE SKIP LIST:
// §6.4 puts every bypassed position in the grant, so a peer alone in a wide row
// carries a skip entry per hole on every edge of every grant, and PEER_STATUS
// carries the whole registry beside it. That is a frame that grows with the
// map's HISTORY rather than with its population.
//
// This test asserts the behaviour as it is, so that changing it is a decision
// somebody makes rather than a side effect. It is not a claim that the
// behaviour is wrong.
func TestAContractingMapKeepsItsRectangleAndPaysForItInSkipLists(t *testing.T) {
	r := newChurnRig(t, 99)
	for i := 0; i < 30; i++ {
		r.join("peer-"+strconv.Itoa(i), nil)
	}
	grown := r.srv.MapShape()
	if grown.Width < 5 || grown.Height < 5 {
		t.Fatalf("thirty peers produced a %dx%d map", grown.Width, grown.Height)
	}
	// Everybody but three leaves for good.
	for i := 0; i < 27; i++ {
		r.neverReturn("peer-" + strconv.Itoa(i))
	}
	r.sweep("after the contraction")

	shrunk := r.srv.MapShape()
	if shrunk != grown {
		t.Fatalf("the rectangle changed from %dx%d to %dx%d on contraction; §7.5 says a release "+
			"leaves the map's shape alone, so a change here is a rule somebody added",
			grown.Width, grown.Height, shrunk.Width, shrunk.Height)
	}
	holes := r.srv.Holes()
	if len(holes) != grown.Width*grown.Height-3 {
		t.Fatalf("a %dx%d map holding 3 slots has %d holes, want %d",
			grown.Width, grown.Height, len(holes), grown.Width*grown.Height-3)
	}

	// The measurable consequence, so the size of it is on the record rather than
	// asserted in prose: the longest skip list a survivor's grant would carry.
	r.srv.mu.Lock()
	worst, worstEdge := 0, ""
	for _, res := range r.srv.grid.Slots {
		for _, edge := range contracta.CanonicalEdges() {
			_, skipped, _ := r.srv.grid.Effective(res, edge, allDeliverable)
			if len(skipped) > worst {
				worst, worstEdge = len(skipped), edge
			}
		}
	}
	r.srv.mu.Unlock()
	t.Logf("a %dx%d map that churned from 30 peers down to 3 carries %d holes; the longest "+
		"bypass list on any grant is %d entries (edge %s). The walk cost is bounded by the axis "+
		"length; the FRAME cost is not bounded by the population (§6.4, §8)",
		grown.Width, grown.Height, len(holes), worst, worstEdge)
	if worst == 0 {
		t.Fatal("a map with holes produced no bypass entries at all")
	}
}

// ---------------------------------------------------------------- the broadcast bound

// TestTheCoalescingWindowWidensUnderChurnAndNarrowsAfterIt is B29's first
// broadcast rule as arithmetic: DOUBLE while a window sees more than
// statusChurnBurstThreshold registry changes, up to statusCoalesceMaxMs, and
// narrow one step after a quieter one.
func TestTheCoalescingWindowWidensUnderChurnAndNarrowsAfterIt(t *testing.T) {
	w := newChurnWindow(250*time.Millisecond, 2*time.Second, 8)
	if w.current() != 250*time.Millisecond {
		t.Fatalf("a fresh window is %s, want the floor", w.current())
	}
	// Exactly the threshold does NOT widen: §12 says MORE than it.
	if got := w.observe(8); got != 250*time.Millisecond {
		t.Fatalf("8 changes widened the window to %s; the rule is MORE than the threshold", got)
	}
	for _, want := range []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second} {
		if got := w.observe(9); got != want {
			t.Fatalf("widening gave %s, want %s", got, want)
		}
	}
	// The ceiling holds however long the storm lasts.
	for i := 0; i < 10; i++ {
		if got := w.observe(1000); got != 2*time.Second {
			t.Fatalf("the window passed its ceiling: %s", got)
		}
	}
	// And it narrows one step at a time, back to the floor and no further.
	for _, want := range []time.Duration{time.Second, 500 * time.Millisecond, 250 * time.Millisecond} {
		if got := w.observe(0); got != want {
			t.Fatalf("narrowing gave %s, want %s", got, want)
		}
	}
	for i := 0; i < 5; i++ {
		if got := w.observe(0); got != 250*time.Millisecond {
			t.Fatalf("the window narrowed below its floor: %s", got)
		}
	}
	// The published arithmetic, which is what B29 asks a reader to be able to
	// check: 240 broadcasts a minute at the floor, 30 under a storm.
	if got := maxBroadcastsPerMinute(250 * time.Millisecond); got != 240 {
		t.Fatalf("60000/250 = %v, want 240", got)
	}
	if got := maxBroadcastsPerMinute(2 * time.Second); got != 30 {
		t.Fatalf("60000/2000 = %v, want 30", got)
	}
	// A ceiling below the floor is a configuration that cannot be obeyed, and it
	// resolves to "no widening" rather than to a window that shrinks.
	bad := newChurnWindow(250*time.Millisecond, 100*time.Millisecond, 8)
	if got := bad.observe(1000); got != 250*time.Millisecond {
		t.Fatalf("a ceiling under the floor produced a %s window", got)
	}
}

// TestBroadcastStormIsBoundedByTheWindowAndNotByTheChurn is the storm case B29
// promises a bound for, measured rather than reasoned about.
//
// N near-simultaneous registry changes must produce a broadcast count bounded
// by the WINDOW — elapsed/window — and not by N. The distinction is the whole
// amendment: without coalescing this is one broadcast per change, and each one
// costs slotCount stats blocks to every peer AND every subscriber, so the
// uncoalesced cost is quadratic in the thing a public map grows.
func TestBroadcastStormIsBoundedByTheWindowAndNotByTheChurn(t *testing.T) {
	// The floor and the ceiling are scaled down from the shipped 250/2000 ms so a
	// storm fits inside a test's patience, and THE THRESHOLD IS LEFT AT ITS
	// SHIPPED 8 on purpose: the thing being proved is that a real churn rate
	// crosses a real threshold, and moving the threshold to meet the harness
	// would prove only that the harness can be tuned.
	const floor = 100 * time.Millisecond
	const ceiling = 800 * time.Millisecond
	r := startCredRelay(t, credRelayOptions{
		statusCoalesce:    floor,
		statusCoalesceMax: ceiling,
		churnThreshold:    contractb.StatusChurnBurstThreshold,
		// Out of reach: every broadcast in the measurement is the coalescing
		// window's, not §6.5's freshness timer.
		statsBroadcast: time.Hour,
		// THE MAP IS IN MEMORY, and that is what makes this test a measurement
		// rather than a coin toss. Grid.Save fsyncs twice per registry change; on
		// this host under a full-suite run that costs about 40 ms, which throttles
		// the storm to ~24 changes a second — BELOW the widening threshold, so the
		// test would conclude "the window did not widen" about a disk. §7.4's
		// durability is a production requirement and has its own test
		// (TestGridIsDurable); the churn rate is this one's subject.
		inMemoryGrid: true,
	})
	srv := r.srv

	// The storm is real registry churn — a reservation minted and released, over
	// and over — so the map stays 1x1 while maxSlotEverIssued climbs. That is
	// DQ3's arithmetic on a bench: the ADDRESS space grows with churn and the
	// POSITION space does not.
	//
	// It is PACED rather than run flat out. A loop with no pause holds the
	// registry mutex almost continuously and starves the publisher it is trying
	// to measure; 2 ms between pairs still produces ~1000 changes a second, which
	// is two orders of magnitude over the threshold and leaves the test correct
	// even when the host is ten times slower than it is now.
	start := time.Now()
	base := srv.BroadcastCount()
	changes := 0
	peak := 0.0
	lastCount, lastAt := base, start
	deadline := start.Add(2 * time.Second)
	for time.Now().Before(deadline) {
		id := "storm-" + strconv.Itoa(changes)
		res, _, err := srv.ReserveSlot(id, nil)
		if err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
		if err := srv.ReleaseSlot(res.Slot); err != nil {
			t.Fatalf("ReleaseSlot: %v", err)
		}
		changes += 2
		if now := time.Now(); now.Sub(lastAt) >= 200*time.Millisecond {
			if rate := float64(srv.BroadcastCount()-lastCount) / now.Sub(lastAt).Seconds(); rate > peak {
				peak = rate
			}
			lastCount, lastAt = srv.BroadcastCount(), now
		}
		time.Sleep(2 * time.Millisecond)
	}
	elapsed := time.Since(start)
	broadcasts := srv.BroadcastCount() - base

	// THE BOUND. At most one broadcast per window, plus one for the window that
	// was already running when the storm started.
	ceilingRounds := int64(elapsed/floor) + 2
	t.Logf("storm: %d registry changes in %s produced %d PEER_STATUS rounds "+
		"(bound %d at a %s floor); peak observed %.0f/s, bound %.0f/s; window ended at %s; "+
		"maxSlotEverIssued %d on a %dx%d map",
		changes, elapsed.Truncate(time.Millisecond), broadcasts, ceilingRounds, floor,
		peak, float64(time.Second)/float64(floor), srv.CoalesceWindow(),
		srv.MaxSlotEverIssued(), srv.MapShape().Width, srv.MapShape().Height)
	if broadcasts > ceilingRounds {
		t.Fatalf("%d broadcasts in %s exceeds the %d the %s window allows",
			broadcasts, elapsed, ceilingRounds, floor)
	}
	if int64(changes) <= broadcasts*2 {
		t.Fatalf("the storm produced %d changes and %d broadcasts; the harness did not "+
			"generate enough churn to prove anything", changes, broadcasts)
	}
	// THE WIDENING HAPPENED. A storm this dense must have pushed the window off
	// its floor, or the bound above was met by an idle relay rather than by the
	// mechanism.
	if srv.CoalesceWindow() <= floor {
		t.Fatalf("the window is still at its %s floor after %d registry changes; B29's widening "+
			"did not fire", floor, changes)
	}
	// THE MAP IS STILL 1x1. Every release left a hole and every reservation
	// filled it, which is holes-before-growth under the densest churn a bench
	// can make.
	if shape := srv.MapShape(); shape.Width != 1 || shape.Height != 1 {
		t.Fatalf("the storm grew the map to %dx%d", shape.Width, shape.Height)
	}
	if srv.MaxSlotEverIssued() < changes/2 {
		t.Fatalf("maxSlotEverIssued is %d after %d reservations; it must count every one",
			srv.MaxSlotEverIssued(), changes/2)
	}
}

// TestTheLastFrameOfABurstIsAlwaysSent is the rule coalescing must not break:
// intermediate states may be dropped, THE LAST ONE MAY NOT, because every one
// of these messages is full state and the last one is the truth.
func TestTheLastFrameOfABurstIsAlwaysSent(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{
		statusCoalesce: 30 * time.Millisecond, statusCoalesceMax: 240 * time.Millisecond,
		churnThreshold: 4, statsBroadcast: time.Hour,
		limits: contractb.Limits{MaxClaimsPerMinute: 10000},
	})
	r.mint("watcher", peercred.GrantPeer)
	watcher := r.dial(dialSpec{credentialPeer: "watcher", claimPeer: "watcher", sendHandshake: true})
	watcher.claim()
	watcher.wait(contractb.TypeSectorGrant, 2*time.Second)

	// A burst: eight reservations minted and released as fast as the registry
	// takes them, well past the widening threshold.
	for i := 0; i < 8; i++ {
		res, _, err := r.srv.ReserveSlot("burst-"+strconv.Itoa(i), nil)
		if err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
		if i%2 == 0 {
			if err := r.srv.ReleaseSlot(res.Slot); err != nil {
				t.Fatalf("ReleaseSlot: %v", err)
			}
		}
	}
	want := len(r.srv.Snapshot())

	// The window may have widened to its ceiling, so wait generously — the rule
	// is that the last frame ARRIVES, not that it arrives quickly.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		statuses := watcher.statuses()
		if len(statuses) > 0 && statuses[len(statuses)-1].SlotCount == want {
			// And the frame is FULL STATE: every reservation the relay holds.
			if got := len(statuses[len(statuses)-1].Slots); got != want {
				t.Fatalf("the last frame carries %d slots and claims slotCount %d", got, want)
			}
			t.Logf("the burst produced %d PEER_STATUS frames at this peer for %d registry "+
				"changes; the last one carries the final state (%d slots)",
				len(statuses), 12, want)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	statuses := watcher.statuses()
	last := 0
	if len(statuses) > 0 {
		last = statuses[len(statuses)-1].SlotCount
	}
	t.Fatalf("the last frame of the burst never arrived: %d frames, last slotCount %d, want %d",
		len(statuses), last, want)
}

// TestARepeatClaimThatChangesNothingBroadcastsNothing is B29's second broadcast
// rule, over a real wire, with both halves asserted.
//
// THE CLAIMANT IS STILL OWED ITS ANSWER: it gets a SECTOR_GRANT for every claim
// it makes. WHAT STOPS IS TELLING EVERYBODY ELSE: the epoch does not move and no
// PEER_STATUS goes out. This is the epoch rate's measured cause — 64 claims from
// one slot in one day, every one a re-claim as its time scale wandered, and 64
// broadcasts of the whole map for nothing (DQ3).
func TestARepeatClaimThatChangesNothingBroadcastsNothing(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{
		statusCoalesce: 10 * time.Millisecond,
		statsBroadcast: time.Hour, // no heartbeat can be mistaken for a broadcast
		// §3.3's ceilings are lifted for this test and it is worth saying why:
		// sixty-four claims in a tight loop is ABOVE maxFramesPerSecond (50) and
		// above maxClaimsPerMinute (12), so a relay running the shipped table
		// would shed the connection long before B29's rule was reached. The
		// suppression and the capacity limits are two different mechanisms
		// answering the same peer, and this test is about the second one only.
		limits: contractb.Limits{
			MaxClaimsPerMinute: 10000, MaxFramesPerSecond: 10000, MaxBytesPerSecond: 1 << 24,
		},
	})
	r.mint("wanderer", peercred.GrantPeer)
	r.mint("observer", peercred.GrantPeer)
	wanderer := r.dial(dialSpec{credentialPeer: "wanderer", claimPeer: "wanderer", sendHandshake: true})
	observer := r.dial(dialSpec{credentialPeer: "observer", claimPeer: "observer", sendHandshake: true})
	wanderer.claim()
	observer.claim()
	wanderer.wait(contractb.TypeSectorGrant, 2*time.Second)
	observer.wait(contractb.TypeSectorGrant, 2*time.Second)
	// Let the joins settle so the baseline is a quiet map.
	time.Sleep(150 * time.Millisecond)

	baseline := r.srv.BroadcastCount()
	observerFrames := len(observer.statuses())

	// DQ3's slot 6: sixty-four claims that say exactly the same thing.
	for i := 0; i < 64; i++ {
		wanderer.claim()
	}
	// Every one is answered. The claimant is owed that, whatever the map is told.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if grants(wanderer) >= 65 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := grants(wanderer); got < 65 {
		t.Fatalf("the claimant got %d SECTOR_GRANTs for 65 claims (closed=%v); every claim is "+
			"answered (§7.2, §22 B29)", got, wanderer.isClosed())
	}
	time.Sleep(200 * time.Millisecond)

	if got := r.srv.BroadcastCount() - baseline; got != 0 {
		t.Fatalf("64 structurally identical re-claims produced %d PEER_STATUS broadcasts, want 0 "+
			"(§22, B29: a repeat claim that changes nothing structural broadcasts nothing)", got)
	}
	if got := len(observer.statuses()) - observerFrames; got != 0 {
		t.Fatalf("the other peer was told %d times about a claim that changed nothing", got)
	}
	t.Logf("64 wandering re-claims: 65 grants to the claimant, 0 broadcasts to the map")

	// And a claim that DOES change something structural is published, so the
	// suppression is a narrow rule rather than a silenced path.
	wanderer.send(contractb.TypeSectorClaim, contractb.SectorClaim{
		SimulationSize: 2000, ModConnected: true,
		ExportEdges: []string{"E"}, BorderEdges: []string{"E"},
	})
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r.srv.BroadcastCount() > baseline {
			t.Logf("a claim that changed exportEdges was broadcast, as it must be")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("a claim that changed exportEdges produced no broadcast; the suppression is too wide")
}

func grants(p *credPeer) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, env := range p.frames {
		if env.Type == contractb.TypeSectorGrant {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------- B36: the stats cadence

// statsPing sends one stats-bearing PING, which is the frame §6.11 lets a peer
// carry its stats block on and the frame §24 B36 stops treating as a broadcast
// trigger.
func statsPing(p *credPeer, population int) {
	p.send(contractb.TypePing, contractb.Ping{
		Nonce: wire.NewUUID(),
		Stats: &contractb.PeerStats{Population: contractb.IntPtr(population)},
	})
}

// TestB36AStatsBearingPingBroadcastsNothing is §24 B36's first half, and it is
// the shape TestARepeatClaimThatChangesNothingBroadcastsNothing above already
// proved for a re-claim: A FRAME THAT CHANGES NO REGISTRY STATE TELLS NOBODY.
//
// The bug it locks down was measured on the living deployment on 2026-08-16.
// markStatsLocked bumped `pending` on every stats-bearing PING; seven peers
// PINGing once per statsIntervalMs land in seven DIFFERENT coalescing windows,
// so the relay broadcast the whole map at 1.32/s where §6.5 designs for one per
// statsBroadcastIntervalMs. That was ~246 GiB a month of PEER_STATUS on a
// 3,072 GiB allowance, for a frame the timer was going to send anyway.
//
// statsBroadcast is OUT OF REACH here, so any broadcast inside the window is a
// stats-triggered one and nothing else.
func TestB36AStatsBearingPingBroadcastsNothing(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{
		statusCoalesce: 10 * time.Millisecond,
		statsBroadcast: time.Hour,
		limits: contractb.Limits{
			MaxFramesPerSecond: 10000, MaxBytesPerSecond: 1 << 24, MaxClaimsPerMinute: 10000,
		},
	})
	r.mint("breather", peercred.GrantPeer)
	r.mint("observer", peercred.GrantPeer)
	breather := r.dial(dialSpec{credentialPeer: "breather", claimPeer: "breather", sendHandshake: true})
	observer := r.dial(dialSpec{credentialPeer: "observer", claimPeer: "observer", sendHandshake: true})
	breather.claim()
	observer.claim()
	breather.wait(contractb.TypeSectorGrant, 2*time.Second)
	observer.wait(contractb.TypeSectorGrant, 2*time.Second)
	time.Sleep(150 * time.Millisecond)

	baseline := r.srv.BroadcastCount()
	observerFrames := len(observer.statuses())

	// Twelve arrivals, each landing in a coalescing window of its own — which is
	// exactly the shape that made the measured rate track the number of peers
	// rather than the timer.
	const pings = 12
	last := 0
	for i := 0; i < pings; i++ {
		last = 100 + i
		statsPing(breather, last)
		time.Sleep(25 * time.Millisecond)
	}
	// Every PING is still answered: the peer is owed its PONG whatever the map
	// is told, exactly as a re-claiming peer is owed its grant.
	deadline := time.Now().Add(3 * time.Second)
	for len(breather.findAll(contractb.TypePong)) < pings && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := len(breather.findAll(contractb.TypePong)); got < pings {
		t.Fatalf("%d PINGs were answered with %d PONGs (closed=%v)", pings, got, breather.isClosed())
	}
	time.Sleep(200 * time.Millisecond)

	if got := r.srv.BroadcastCount() - baseline; got != 0 {
		t.Fatalf("%d stats-bearing PINGs produced %d PEER_STATUS broadcasts, want 0 (§6.5, §14 "+
			"B4, §24 B36: stats ride the statsBroadcastIntervalMs timer and schedule nothing)",
			pings, got)
	}
	if got := len(observer.statuses()) - observerFrames; got != 0 {
		t.Fatalf("the other peer was told %d times about a heartbeat that changed no registry "+
			"state", got)
	}
	t.Logf("%d stats-bearing PINGs: %d PONGs to the sender, 0 broadcasts to the map", pings, pings)

	// AND THE STATS ARE NOT LOST, which is the half that makes the suppression
	// safe. The block was stored on arrival; the next broadcast the map makes for
	// its own reasons carries it.
	breather.send(contractb.TypeSectorClaim, contractb.SectorClaim{
		SimulationSize: 2000, ModConnected: true,
		ExportEdges: []string{"E"}, BorderEdges: []string{"E"},
	})
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, st := range observer.statuses()[observerFrames:] {
			for _, slot := range st.Slots {
				if slot.PeerID != "breather" || slot.Stats == nil || slot.Stats.Population == nil {
					continue
				}
				if *slot.Stats.Population != last {
					continue
				}
				if slot.StatsAsOfMs == 0 {
					t.Fatal("the republished stats block carried no statsAsOfMs; §6.5 requires a " +
						"reader to age it from the moment it ARRIVED")
				}
				t.Logf("the next registry-driven broadcast carried population=%d, "+
					"statsAsOfMs=%d", last, slot.StatsAsOfMs)
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no broadcast after the claim carried the last stats block (population %d); the "+
		"suppression dropped the stats instead of deferring them", last)
}

// TestB36StatsRideTheTimerAndOnlyTheTimer is the other half: the broadcast rate
// on a map whose only traffic is stats must track statsBroadcastIntervalMs and
// NOT the arrival rate.
//
// It is the assertion the measured defect would have failed by a factor of the
// peer count, and it is written as a RATE rather than a count for the reason
// B29's own harness gives: a bound a test can assert is worth more than an
// estimate a comment makes.
func TestB36StatsRideTheTimerAndOnlyTheTimer(t *testing.T) {
	const interval = 200 * time.Millisecond
	r := startCredRelay(t, credRelayOptions{
		statusCoalesce: 10 * time.Millisecond,
		statsBroadcast: interval,
		limits: contractb.Limits{
			MaxFramesPerSecond: 10000, MaxBytesPerSecond: 1 << 24, MaxClaimsPerMinute: 10000,
		},
	})
	r.mint("breather", peercred.GrantPeer)
	breather := r.dial(dialSpec{credentialPeer: "breather", claimPeer: "breather", sendHandshake: true})
	breather.claim()
	breather.wait(contractb.TypeSectorGrant, 2*time.Second)
	time.Sleep(2 * interval)

	baseline := r.srv.BroadcastCount()
	started := time.Now()
	// One arrival every 20 ms for a second: ten times the timer's rate, and the
	// cadence a busy map's peers collectively produce.
	const pings = 50
	for i := 0; i < pings; i++ {
		statsPing(breather, 100+i)
		time.Sleep(20 * time.Millisecond)
	}
	elapsed := time.Since(started)
	got := r.srv.BroadcastCount() - baseline

	// THE CEILING IS THE TIMER'S RATE, with one tick of slack for a busy host.
	// It is a ceiling and not an equality on purpose: publishLoop skips a tick
	// whose own last publish is younger than the interval (§14 B4's drift
	// bound), so a run can legitimately land anywhere between one publish per
	// interval and one per two.
	ceiling := int64(elapsed/interval) + 1
	if got > ceiling {
		t.Fatalf("%d stats arrivals over %s produced %d broadcasts; the "+
			"statsBroadcastIntervalMs timer bounds it at %d (§6.5, §14 B4, §24 B36) — the "+
			"arrivals are scheduling frames again", pings, elapsed.Round(time.Millisecond),
			got, ceiling)
	}
	// AND THE TIMER STILL FIRES. §6.5 makes it a floor on freshness, so a
	// suppression that also stopped the timer would leave the map's stats
	// frozen until the next registry change.
	if got < 1 {
		t.Fatalf("over %s with statsBroadcastIntervalMs=%s the relay broadcast nothing; the "+
			"timer is a FLOOR ON FRESHNESS and §24 B36 does not remove it",
			elapsed.Round(time.Millisecond), interval)
	}
	t.Logf("%d stats arrivals over %s produced %d broadcasts (the timer's ceiling is %d), "+
		"not %d", pings, elapsed.Round(time.Millisecond), got, ceiling, pings)
}

// ---------------------------------------------------------------- the slot-space policy

// TestTheSlotSpacePolicyIsTheSameWordsAtBothDoors is the operator half of DQ3.
//
// An operator reads a consequence report and then acts, and after M5 those two
// things can happen at different doors — a console on the relay's own machine
// and an HTTP call from wherever the operator is. If the two described the act
// differently, the confirmation B28 requires would be a confirmation of
// whichever text that operator happened to read.
func TestTheSlotSpacePolicyIsTheSameWordsAtBothDoors(t *testing.T) {
	a := startAdminRig(t)
	if _, _, err := a.srv.ReserveSlot("peer-departed", nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, _, err := a.srv.ReserveSlot("peer-stays", nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	console, err := a.srv.consequenceOfRelease(1)
	if err != nil {
		t.Fatalf("consequenceOfRelease: %v", err)
	}
	// The operator's own view, logged so the words the documentation quotes can
	// be read out of a test run rather than reconstructed from source.
	t.Logf("the console's release report, verbatim:\n%s", console)
	status, report := a.post(t, "/admin/release-slot", "ops",
		map[string]any{"slot": 1, "reason": "the operator's answer for a position nobody will fill"})
	if status != http.StatusOK {
		t.Fatalf("the consequence call answered HTTP %d: %v", status, report)
	}

	notes, _ := report["notes"].([]any)
	if len(notes) == 0 {
		t.Fatalf("the release report carries no notes: %v", report)
	}
	// EVERY SENTENCE THE PATH RETURNS IS A SENTENCE THE CONSOLE PRINTS. The
	// console wraps for a terminal, so the comparison is on the words.
	flat := strings.Join(strings.Fields(console), " ")
	for _, note := range notes {
		s, _ := note.(string)
		if !strings.Contains(flat, strings.Join(strings.Fields(s), " ")) {
			t.Fatalf("the admin path says something the console does not:\n  %s\n\nconsole:\n%s",
				s, console)
		}
	}

	// The four facts the policy has to state, whichever door it came through.
	for _, must := range []string{
		"becomes a HOLE",                  // the position comes back
		"NEXT newcomer",                   // and it is offered before any axis grows
		"retired for good",                // the address does not come back
		"SLOT_VACANT",                     // which is what makes non-delivery provable
		"IF YOU DO NOTHING INSTEAD",       // leaving it is the other real option
		"never expires",                   // ... and what that costs
		"cannot be undone",                // release is irreversible
		"a release never moves a journal", // custody is local
	} {
		if !strings.Contains(flat, must) {
			t.Fatalf("the slot-space policy does not state %q:\n%s", must, console)
		}
	}

	// The machine-readable half: the holes this act offers the map, and the
	// address counter that does not move back.
	holes, _ := report["holesAfter"].([]any)
	if len(holes) != 1 {
		t.Fatalf("holesAfter is %v, want the one position this release offers", report["holesAfter"])
	}
	if report["maxSlotEverIssued"] == nil {
		t.Fatalf("the report does not carry maxSlotEverIssued: %v", report)
	}
	if report["addressRetiredForever"] != true {
		t.Fatalf("the report does not say the address is retired: %v", report)
	}
}

// TestReleaseOverThePathLeavesAHoleTheNextNewcomerFills is the policy as
// BEHAVIOUR rather than as text: the act an operator confirms has to do what
// the report they read said it would.
func TestReleaseOverThePathLeavesAHoleTheNextNewcomerFills(t *testing.T) {
	a := startAdminRig(t)
	// A 2x2 with four peers, so releasing one leaves a hole rather than an edge.
	for i := 1; i <= 4; i++ {
		if _, _, err := a.srv.ReserveSlot("peer-"+strconv.Itoa(i), nil); err != nil {
			t.Fatalf("reserve: %v", err)
		}
	}
	before := a.srv.MaxSlotEverIssued()
	target, _ := a.srv.ResOfSlot(2)

	status, report := a.post(t, "/admin/release-slot", "ops",
		map[string]any{"slot": 2, "reason": "departed and not coming back"})
	if status != http.StatusOK {
		t.Fatalf("report: HTTP %d %v", status, report)
	}
	status, applied := a.post(t, "/admin/release-slot/confirm", "ops", map[string]any{
		"confirmToken": report["confirmToken"], "ringStateHash": report["ringStateHash"]})
	if status != http.StatusOK || applied["applied"] != true {
		t.Fatalf("confirm: HTTP %d %v", status, applied)
	}

	holes := a.srv.Holes()
	if len(holes) != 1 || holes[0] != target.Position() {
		t.Fatalf("holes after the release are %v, want exactly %+v", holes, target.Position())
	}
	if a.srv.MaxSlotEverIssued() != before {
		t.Fatalf("maxSlotEverIssued moved from %d to %d over a release; it never decreases and "+
			"a release issues nothing", before, a.srv.MaxSlotEverIssued())
	}
	// The next newcomer fills it, and takes a NEW address.
	res, _, err := a.srv.ReserveSlot("stranger", nil)
	if err != nil {
		t.Fatalf("ReserveSlot: %v", err)
	}
	if res.Position() != target.Position() {
		t.Fatalf("the newcomer landed at %+v, want the released position %+v",
			res.Position(), target.Position())
	}
	if res.Slot != before+1 {
		t.Fatalf("the newcomer took slot %d, want %d; the released address is retired forever",
			res.Slot, before+1)
	}
	if _, ok := a.srv.ResOfSlot(2); ok {
		t.Fatal("slot 2 answers again after being released")
	}
}
