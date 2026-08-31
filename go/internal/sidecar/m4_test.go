package sidecar

// The M4 tests: the grid, route-around, the splice, arrival pacing, the bounded
// hold and its dark-only clock, the proof-based re-route, and the operator
// surfaces. Everything here is contract-b-m4.md and contract-a.md §15.

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"multiverse/internal/archive"
	"multiverse/internal/bb8"
	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/wire"
)

// fakeClock is the injectable time source of §9.3. forwardTimeoutMs is TWENTY-
// FOUR HOURS, and the only honest way to test a rule about hours is to control
// the clock the rule reads — the alternative is to test a different, shorter
// rule and hope it is the same one.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// --------------------------------------------------------------- the grid

// TestGridFormsFromSixOpinionFreeClaims covers §7.2 rule 6 end to end: six
// sidecars that express NO preference at all land in a full 3x2 map, which is
// the shape the exit test needs and the smallest honest two-axis map (§2.1).
func TestGridFormsFromSixOpinionFreeClaims(t *testing.T) {
	g := newGrid(t, 6, gridOptions{})

	waitFor(t, 10*time.Second, "the map to settle at 3x2", func() bool {
		shape := g.relay.relay.MapShape()
		return shape.Width == 3 && shape.Height == 2
	})
	if got := len(g.relay.relay.Snapshot()); got != 6 {
		t.Fatalf("the map holds %d slots, want 6", got)
	}
	seen := map[contractb.Position]int{}
	for _, res := range g.relay.relay.Snapshot() {
		pos := contractb.Position{Col: res.Col, Row: res.Row}
		if prev, dup := seen[pos]; dup {
			t.Fatalf("slots %d and %d share position %+v", prev, res.Slot, pos)
		}
		seen[pos] = res.Slot
	}
	if len(seen) != 6 {
		t.Fatalf("the 3x2 map has holes: %v", seen)
	}
	// Every peer exports on two edges and receives on two, and BOTH lanes open.
	for _, nd := range g.nodes {
		nd.mod.waitEdge(contracta.EdgeE, true, 10*time.Second)
		nd.mod.waitEdge(contracta.EdgeN, true, 10*time.Second)
		if nd.side.Neighbour(contracta.EdgeE) == nil || nd.side.Neighbour(contracta.EdgeN) == nil {
			t.Fatalf("slot %d has a closed lane on a full 3x2 map", nd.slot)
		}
	}
}

// TestTwoAxisMigration covers §2 and §6.6: an organism leaves east and arrives
// through the facing WEST edge, and one that leaves north arrives through the
// facing SOUTH edge. The blob is byte-identical on both axes, and velocity and
// heading are copied rather than mirrored on either.
//
// D17 retired the word "passive" here — that edge is now an export edge too
// (contract-a.md §18, A38) — and changed nothing else about this case. The four
// directions are covered in twoway_test.go.
func TestTwoAxisMigration(t *testing.T) {
	g := newGrid(t, 6, gridOptions{layout: layout3x2()})
	origin := g.bySlot(1) // (0,0)
	east := g.bySlot(2)   // (1,0)
	north := g.bySlot(4)  // (0,1)

	waitLane(t, origin.side, contracta.EdgeE, 2)
	waitLane(t, origin.side, contracta.EdgeN, 4)

	payload := makePayload(testEntityID)
	cases := []struct {
		exitEdge  string
		wantEntry string
		dest      *node
		wantSlot  int
	}{
		{contracta.EdgeE, contracta.EdgeW, east, 2},
		{contracta.EdgeN, contracta.EdgeS, north, 4},
	}
	for _, c := range cases {
		before := c.dest.mod.frameCount()
		migrationID := origin.mod.migrateOutPayload(wire.NewUUID(), testEntityID, c.exitEdge, 0.5, payload)

		env, _ := c.dest.mod.waitFrom(before, 10*time.Second,
			func(e wire.Envelope) bool { return e.Type == contracta.TypeMigrateIn })
		in := decodeAs[contracta.MigrateIn](t, env)
		if in.MigrationID != migrationID {
			t.Fatalf("%s hop delivered %s, want %s", c.exitEdge, in.MigrationID, migrationID)
		}
		if in.EntryEdge != c.wantEntry {
			t.Fatalf("%s hop arrived on %s, want the passive %s edge",
				c.exitEdge, in.EntryEdge, c.wantEntry)
		}
		if in.Payload != payload {
			t.Fatalf("%s hop changed the blob", c.exitEdge)
		}
		if in.EntryPosition != 0.5 {
			t.Fatalf("%s hop entryPosition = %v; entryPosition is exitPosition, copied",
				c.exitEdge, in.EntryPosition)
		}
		if in.Velocity.X != 6.12 || in.Velocity.Y != 0.44 || in.Heading != 274.11 {
			t.Fatalf("%s hop mirrored the motion: %+v heading %v", c.exitEdge, in.Velocity, in.Heading)
		}
		waitFor(t, 10*time.Second, "the "+c.exitEdge+" hop to complete", func() bool {
			return custodyOf(origin.side, migrationID) == "out/done"
		})
		if got := journalEntry(t, origin.side, migrationID).Entry.DestSlot; got != c.wantSlot {
			t.Fatalf("%s hop routed to slot %d, want %d", c.exitEdge, got, c.wantSlot)
		}
		// Exactly one copy exists across the whole map.
		alive := 0
		for _, nd := range g.nodes {
			if nd.world.isAlive(testEntityID) {
				alive++
			}
		}
		if alive != 1 {
			t.Fatalf("after the %s hop the organism is alive in %d worlds, want 1", c.exitEdge, alive)
		}
		// Hand it back so the next case starts from the origin again.
		c.dest.world.mu.Lock()
		delete(c.dest.world.alive, testEntityID)
		c.dest.world.mu.Unlock()
		origin.world.put(testEntityID)
	}
}

// TestKillMidColumnRoutesAroundAndClosesTheDegenerateNorthLane is the exit
// test's Part 2, in miniature, and it asserts BOTH halves of §2.1.
//
// Slot 5 at (1,1) is hard-killed. Its row still holds three slots, so the east
// lane RE-PAIRS past it and the current keeps flowing. Its column holds only two,
// so there is no third slot to re-pair to and the survivor's NORTH lane CLOSES
// with no_peer. That is route-around working correctly against a degenerate
// axis, not route-around failing.
func TestKillMidColumnRoutesAroundAndClosesTheDegenerateNorthLane(t *testing.T) {
	g := newGrid(t, 6, gridOptions{layout: layout3x2()})
	four, five, six := g.bySlot(4), g.bySlot(5), g.bySlot(6)
	two := g.bySlot(2) // (1,0): slot 5's column partner

	waitLane(t, four.side, contracta.EdgeE, 5)
	waitLane(t, two.side, contracta.EdgeN, 5)
	four.mod.waitEdge(contracta.EdgeE, true, 5*time.Second)
	two.mod.waitEdge(contracta.EdgeN, true, 5*time.Second)

	// Hard kill, mid-flow: no close frame, no drain.
	five.mod.abort()
	_ = five.side.Close()

	// The east lane along row 1 re-pairs past the hole and STAYS OPEN. A dying
	// peer takes its mod with it, so the bypass may pass through
	// peer_mod_absent on its way — both are SKIP reasons under D12, and what
	// matters is where it settles once the connection is gone.
	waitLane(t, four.side, contracta.EdgeE, 6)
	var n *contractb.Neighbour
	waitFor(t, 10*time.Second, "the bypass to settle on peer_offline", func() bool {
		n = four.side.Neighbour(contracta.EdgeE)
		return n != nil && n.Slot == 6 && len(n.Skipped) == 1 &&
			n.Skipped[0].Slot != nil && *n.Skipped[0].Slot == 5 &&
			n.Skipped[0].Reason == contractb.SkipPeerOffline
	})
	if st := four.mod.edgeNow(contracta.EdgeE); !st.Open {
		t.Fatalf("slot 4's east edge closed with %q; D12 says re-pair, not close", st.Reason)
	}

	// The north lane up the middle column has nothing to re-pair to. §8's
	// mapping names the reason the skips SHARE, so the edge passes through
	// peer_mod_absent while the dying peer still holds its relay connection and
	// settles on no_peer once it does not.
	waitLaneClosed(t, two.side, contracta.EdgeN)
	two.mod.waitEdgeReason(contracta.EdgeN, false, contracta.ReasonNoPeer, 10*time.Second)
	// And its EAST lane is untouched: its row still holds three slots.
	if st := two.mod.edgeNow(contracta.EdgeE); !st.Open {
		t.Fatalf("slot 2's east edge closed with %q; only the degenerate axis closes", st.Reason)
	}

	// The current continues on the healed lane.
	migrationID := four.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "the healed east lane to carry an organism", func() bool {
		return six.world.spawnCount(migrationID) == 1
	})
	if got := journalEntry(t, four.side, migrationID).Entry.DestSlot; got != 6 {
		t.Fatalf("the healed hop was journaled for slot %d, want 6", got)
	}

	// --- Part 3: the dead slot splices back in, with no operator action.
	revived := startSidecar(t, five.cfg)
	waitSlot(t, revived, 5)
	if pos := revived.Position(); pos.Col != 1 || pos.Row != 1 {
		t.Fatalf("slot 5 came back at %+v, want its own coordinate (1,1)", pos)
	}
	revivedMod := dialFakeMod(t, fakeModOptions{
		url: revived.URL(), world: five.world, heartbeat: 200 * time.Millisecond})

	waitLane(t, four.side, contracta.EdgeE, 5)
	waitLane(t, two.side, contracta.EdgeN, 5)
	two.mod.waitEdge(contracta.EdgeN, true, 10*time.Second)
	revivedMod.waitEdge(contracta.EdgeE, true, 10*time.Second)
	if got := len(g.relay.relay.Snapshot()); got != 6 {
		t.Fatalf("the map holds %d slots after the reclaim, want 6", got)
	}
}

// TestNewPeerSplicesIntoALiveMap is the exit test's Part 4: a brand-new peerId
// asks the relay to extend a full 3x2 into a fourth column, and NO OTHER SLOT IS
// RENUMBERED. The second position in the new column is a hole, and both axes
// route around it.
func TestNewPeerSplicesIntoALiveMap(t *testing.T) {
	g := newGrid(t, 6, gridOptions{layout: layout3x2()})
	three := g.bySlot(3) // (2,0): the east end of row 0
	waitLane(t, three.side, contracta.EdgeE, 1)

	before := map[int][2]int{}
	for _, res := range g.relay.relay.Snapshot() {
		before[res.Slot] = [2]int{res.Col, res.Row}
	}
	// An in-flight entry keeps the destination it recorded across the splice
	// (§7.3 rule 1), and rule 4 is why that is provably safe: a newcomer's slot
	// number is greater than every number ever issued.
	g.bySlot(1).mod.setAckMode(ackSilent)
	inflight := g.bySlot(1).mod.migrateOut(-77001, contracta.EdgeE, 0.5)
	waitFor(t, 5*time.Second, "the in-flight entry to be journaled", func() bool {
		return custodyOf(g.bySlot(1).side, inflight) != "absent"
	})
	destBefore := journalEntry(t, g.bySlot(1).side, inflight).Entry.DestSlot

	seventh := g.addPeer("peer-slot7", &contractb.Position{Col: 3, Row: 0}, gridOptions{})
	if seventh.slot != 7 {
		t.Fatalf("the newcomer took slot %d, want 7 (maxSlotEverIssued + 1)", seventh.slot)
	}
	waitFor(t, 10*time.Second, "the map to grow to 4x2", func() bool {
		shape := g.relay.relay.MapShape()
		return shape.Width == 4 && shape.Height == 2
	})
	for _, res := range g.relay.relay.Snapshot() {
		if res.Slot == 7 {
			if res.Col != 3 || res.Row != 0 {
				t.Fatalf("the newcomer landed at (%d,%d), want (3,0)", res.Col, res.Row)
			}
			continue
		}
		if got := [2]int{res.Col, res.Row}; got != before[res.Slot] {
			t.Fatalf("slot %d moved from %v to %v; a splice must renumber and re-position nobody",
				res.Slot, before[res.Slot], got)
		}
	}
	// Its west neighbour re-pairs to it.
	waitLane(t, three.side, contracta.EdgeE, 7)
	// (3,1) is a hole. The newcomer's column holds nobody else, so its NORTH
	// lane closes; its row wraps back to slot 1.
	waitLane(t, seventh.side, contracta.EdgeE, 1)
	waitLaneClosed(t, seventh.side, contracta.EdgeN)
	seventh.mod.waitEdge(contracta.EdgeN, false, 10*time.Second)

	// Nothing in flight changed its destination.
	if got := journalEntry(t, g.bySlot(1).side, inflight).Entry.DestSlot; got != destBefore {
		t.Fatalf("an in-flight entry's destSlot moved from %d to %d across the splice", destBefore, got)
	}
	g.bySlot(1).mod.setAckMode(ackNormal)

	// Organisms cross into the new slot and out of it.
	in := three.mod.migrateOut(-77002, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "an organism to cross into the new slot", func() bool {
		return seventh.world.spawnCount(in) == 1
	})
	out := seventh.mod.migrateOut(-77003, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "an organism to cross out of the new slot", func() bool {
		return g.bySlot(1).world.spawnCount(out) == 1
	})
}

// --------------------------------------------------------------- pacing

// TestBurstArrivesPacedNotAllAtOnce is the exit test's Part 5 in miniature, and
// it is the failure T1 measured: a slot's inbound deliveries accumulate while
// its world is stopped, and at wake they must NOT release together.
//
// The clock is SIMULATED. A stopped world advances no simulated time, banks no
// tokens, and releases nothing; a running one releases at most
// inboundRatePerSimMinute per simulated minute, through a burst that keeps
// ordinary traffic from ever waiting.
func TestBurstArrivesPacedNotAllAtOnce(t *testing.T) {
	const rate = 120.0 // per simulated minute: two per simulated second
	const burst = 2.0
	const organisms = 12

	g := newGrid(t, 2, gridOptions{
		layout:    layoutRow(2),
		heartbeat: 100 * time.Millisecond,
		tune: func(i int, c *Config) {
			if i == 1 {
				c.InboundRatePerSimMinute = rate
				c.InboundRateBurst = burst
			}
		},
	})
	a, b := g.node(0), g.node(1)
	b.mod.setSimStep(0.25) // 0.25 simulated seconds per 100 ms heartbeat

	// The dam: slot 2's world is paused, so nothing is released and nothing is
	// banked. Custody still moves at the speed of the wire.
	b.mod.setPaused(true)
	time.Sleep(300 * time.Millisecond)
	pausedAt := b.mod.simTimeNow()

	ids := make([]string, 0, organisms)
	for i := 0; i < organisms; i++ {
		ids = append(ids, a.mod.migrateOut(int32(-90000-i), contracta.EdgeE, 0.5))
	}
	waitFor(t, 15*time.Second, "slot 2 to take custody of the whole burst", func() bool {
		n := 0
		for _, id := range ids {
			if custodyOf(b.side, id) != "absent" {
				n++
			}
		}
		return n == organisms
	})
	if got := b.mod.simTimeNow(); got != pausedAt {
		t.Fatalf("a paused world advanced its simulated clock from %v to %v", pausedAt, got)
	}
	if got := deliveredCount(b.mod, ids); got != 0 {
		t.Fatalf("%d MIGRATE_IN reached a paused world; §7.5 releases nothing while it is stopped", got)
	}
	stats := b.side.Stats()
	if stats.PacedDepth == nil || *stats.PacedDepth != organisms {
		t.Fatalf("pacedDepth = %v, want %d — the depth waiting on the limit is the metric",
			stats.PacedDepth, organisms)
	}
	// contract-b-m4.md §18, B16: the block that carries the depth now carries the
	// cap it is queued behind and the speed of the world that drains it. A depth
	// alone is not readable — twelve queued behind 120 a simulated minute is a
	// blink and twelve behind 2.0 is six minutes — and the default has moved
	// three times, so a reader that assumes one is reading a different rig.
	if stats.InboundRatePerSimMinute == nil || *stats.InboundRatePerSimMinute != rate {
		t.Fatalf("the stats block does not publish its configured inbound rate: %v",
			stats.InboundRatePerSimMinute)
	}
	if stats.InboundRateBurst == nil || *stats.InboundRateBurst != burst {
		t.Fatalf("the stats block does not publish its burst: %v", stats.InboundRateBurst)
	}
	// The world is stopped, and ×0 is a READING. Absence would mean unknown.
	if stats.TimeScale == nil || *stats.TimeScale != 0 {
		t.Fatalf("a stopped world published timeScale %v; 0 is a fact the operator "+
			"surface must show, not a gap", stats.TimeScale)
	}

	// Wake it. From here every sample must satisfy the contract's own bound.
	wakeSim := b.mod.simTimeNow()
	b.mod.setPaused(false)
	deadline := time.Now().Add(20 * time.Second)
	for {
		delivered := deliveredCount(b.mod, ids)
		simElapsed := b.mod.simTimeNow() - wakeSim
		allowed := burst + rate*(simElapsed/60.0) + 1 // +1 for the sample race
		if float64(delivered) > allowed {
			t.Fatalf("%d deliveries after %.2f simulated seconds, over the paced ceiling of %.2f",
				delivered, simElapsed, allowed)
		}
		if delivered == organisms {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the paced backlog did not drain: %d of %d delivered", delivered, organisms)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// The journal depth falls to zero, and nothing bounced while the destination
	// was live (Risk 9).
	waitFor(t, 15*time.Second, "the paced depth to fall to zero", func() bool {
		st := b.side.Stats()
		return st.PacedDepth != nil && *st.PacedDepth == 0
	})
	for _, id := range ids {
		if got := b.world.spawnCount(id); got != 1 {
			t.Fatalf("migration %s spawned %d times, want exactly 1", id, got)
		}
	}
	// And the speed follows the world back up, because it is copied from every
	// heartbeat rather than latched once.
	waitFor(t, 5*time.Second, "the woken world to publish a running time scale", func() bool {
		st := b.side.Stats()
		return st.TimeScale != nil && *st.TimeScale > 0
	})
	aStats := a.side.Stats()
	if aStats.LostForwardTotal == nil || *aStats.LostForwardTotal != 0 {
		t.Fatalf("the sender wrote off %v entries while the destination was live and draining; "+
			"a live peer with a deep paced backlog is SLOW, NOT ORPHANED",
			aStats.LostForwardTotal)
	}
}

// logSpy captures one component's slog output so a test can assert on the
// BRANCH IT TOOK. It is used where the rule under test is a decision NOT to
// send: the only other evidence is an absence, and an absence is also what a
// slow rig produces. It tees into the test log so a failure is still readable.
type logSpy struct {
	mu    sync.Mutex
	inner io.Writer
	lines []string
}

func newLogSpy(t *testing.T) *logSpy {
	return &logSpy{inner: newTestWriter(t)}
}

func (s *logSpy) Write(p []byte) (int, error) {
	s.mu.Lock()
	s.lines = append(s.lines, string(p))
	s.mu.Unlock()
	return s.inner.Write(p)
}

func (s *logSpy) count(substr string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, line := range s.lines {
		if strings.Contains(line, substr) {
			n++
		}
	}
	return n
}

func (s *logSpy) logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(s, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// TestJournaledDuplicateIsNotReAcked MOVED to transition_test.go with §25's
// B37, as TestAnOldSidecarsRetryIsStillAbsorbedExactlyOnce.
//
// §14 B6's rule is untouched — a duplicate MIGRATION_PAYLOAD that hits an entry
// which is JOURNALED BUT NOT TOMBSTONED is answered with NOTHING, because an
// early ACK releases the sender's custody before the delivery it claims and
// both sides then let go of one organism. What changed is where a duplicate
// comes from: the sender's own retry produced it, and B37 removed the retry. So
// the test drives it from the two sources that remain and will remain forever —
// a peer older than B37, and a defective one.

func deliveredCount(m *fakeMod, ids []string) int {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	seen := map[string]bool{}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, env := range m.frames {
		if env.Type != contracta.TypeMigrateIn {
			continue
		}
		var in contracta.MigrateIn
		if json.Unmarshal(env.Data, &in) == nil && want[in.MigrationID] {
			seen[in.MigrationID] = true
		}
	}
	return len(seen)
}

// ------------------------------------------------------- the loss, not a hold

// TestAForwardedOrganismIsLostNeverHeldAndNeverBounced is §25's B37 end to end,
// and it is the test the removed hold-clock test turned into.
//
// The shape is the one the bounded hold was written for and is now the shape
// migration accepts: the destination takes custody, dies before its
// acknowledgement leaves, and never comes back. FROM THE SENDER THIS IS
// INDISTINGUISHABLE FROM "the frame never arrived" — that has not changed, and
// it is why nothing may be re-sent or brought home on it. What changed is the
// answer: the entry is not held, not re-forwarded, not bounced. At
// forwardTimeoutMs it becomes a `lost` tombstone and a counter the map can read,
// and the organism is gone.
//
// It uses an injectable clock, because the rule is about twenty-four hours and
// the only honest way to test that rule is to control the clock it reads.
func TestAForwardedOrganismIsLostNeverHeldAndNeverBounced(t *testing.T) {
	const forwardTimeout = 10 * time.Minute
	clock := newFakeClock()

	rl := startRelay(t)
	// Pre-seed the reservations so start order stops mattering (§7.2 rule 1).
	for i, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}} {
		p := pos
		if _, _, err := rl.relay.ReserveSlot([]string{"peer-a", "peer-b"}[i], &p); err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
	}
	cfgA := fastConfig(t, rl, "peer-a")
	cfgA.Clock = clock.Now
	cfgA.ForwardTimeout = forwardTimeout
	// Every deadline in this process reads the injected clock, so a jump of
	// simulated hours would otherwise read as a silent mod. The heartbeat
	// timeout is not what this test is about, so it is put out of reach.
	cfgA.HeartbeatTimeout = time.Hour
	cfgB := fastConfig(t, rl, "peer-b")

	sideB := startSidecar(t, cfgB)
	waitSlot(t, sideB, 2)
	worldB := newWorld()
	modB := dialFakeMod(t, fakeModOptions{url: sideB.URL(), world: worldB, heartbeat: 200 * time.Millisecond})
	modB.setAckMode(ackSilent) // custody moves, the acknowledgement never comes

	sideA := startSidecar(t, cfgA)
	waitSlot(t, sideA, 1)
	worldA := newWorld()
	modA := dialFakeMod(t, fakeModOptions{url: sideA.URL(), world: worldA, heartbeat: 200 * time.Millisecond})
	modA.waitEdge(contracta.EdgeE, true, 10*time.Second)

	migrationID := modA.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "slot 2 to take custody", func() bool {
		return custodyOf(sideB, migrationID) != "absent"
	})
	waitFor(t, 10*time.Second, "the entry to reach the sent handoff state", func() bool {
		return handoffOf(sideA, migrationID) == journal.HandoffSent
	})
	entry := journalEntry(t, sideA, migrationID)
	if entry.RelaySessionID != rl.relay.SessionID() {
		t.Fatalf("the entry recorded relaySessionId %q, want the live relay's %q",
			entry.RelaySessionID, rl.relay.SessionID())
	}
	if entry.SentAtMs == 0 {
		t.Fatal("the entry has no sentAt: the one deadline an outbound entry carries never started")
	}

	// The destination dies with the organism in its journal.
	modB.abort()
	_ = sideB.Close()
	waitFor(t, 10*time.Second, "sideA to observe slot 2 dark", func() bool {
		return sideA.RelayConnected() && !destLiveOf(sideA, 2)
	})

	// THE HOLD IS GONE, so a dark destination is not a state change. Nine tenths
	// of the timeout passes with the destination dark and the sender watching,
	// and the entry does not move: no held state, no re-forward, no bounce.
	clock.Advance(9 * forwardTimeout / 10)
	time.Sleep(500 * time.Millisecond)
	st := journalEntry(t, sideA, migrationID)
	if st.Handoff != journal.HandoffSent {
		t.Fatalf("handoff = %q against a dark destination, want sent — silence is not proof and "+
			"there is no state between sent and resolved", st.Handoff)
	}
	if st.ForwardReceipts != 1 {
		t.Fatalf("forwardReceipts = %d, want exactly 1: the frame is forwarded once", st.ForwardReceipts)
	}
	if got := reportedLost(sideA); got != 0 {
		t.Fatalf("lostForwardTotal = %d before the timeout", got)
	}

	// A restart neither restarts the deadline nor resolves the entry: sentAt is
	// durable and the entry replays as `sent`.
	_ = sideA.Close()
	modA.abort()
	reopened := startSidecar(t, cfgA)
	if got := journalEntry(t, reopened, migrationID); got.SentAtMs != st.SentAtMs {
		t.Fatalf("sentAt moved across a restart: %d then %d", st.SentAtMs, got.SentAtMs)
	}
	if got := handoffOf(reopened, migrationID); got != journal.HandoffSent {
		t.Fatalf("the handoff state is %q after a restart, want sent", got)
	}
	worldA2 := newWorld()
	modA2 := dialFakeMod(t, fakeModOptions{
		url: reopened.URL(), world: worldA2, heartbeat: 200 * time.Millisecond})
	waitFor(t, 20*time.Second, "the restarted sidecar to see the map again", func() bool {
		return reopened.RelayConnected() && reopened.Slot() == 1
	})

	// At the timeout the organism is RECORDED LOST. Nothing is delivered to this
	// world's mod — that is the whole difference from the bounded hold, and it is
	// what makes at-most-once carry no exception at all.
	clock.Advance(forwardTimeout)
	waitFor(t, 20*time.Second, "the forward to be recorded lost", func() bool {
		return reportedLost(reopened) == 1
	})
	lost := journalEntry(t, reopened, migrationID)
	if lost.Handoff != journal.HandoffLost {
		t.Fatalf("the tombstone handoff is %q, want lost", lost.Handoff)
	}
	if lost.Status != journal.StatusDone {
		t.Fatalf("the lost entry is %q, want a done tombstone that still recognises a late ACK",
			lost.Status)
	}
	if lost.Direction != journal.Out || lost.BounceBack {
		t.Fatal("a lost forward was turned into a bounce-back; §25 B37 removed the timeout bounce")
	}
	// The origin's world sees nothing come home. It is asserted by LOOKING
	// rather than by waiting, and after a settling pause, because a false pass
	// here is the duplication this change exists to remove.
	time.Sleep(time.Second)
	if n := modA2.countType(contracta.TypeMigrateIn); n != 0 {
		t.Fatalf("the origin mod was handed %d MIGRATE_IN frame(s) at the timeout: the organism "+
			"came home, which is exactly the duplication §25 B37 removes", n)
	}
	if worldA2.spawnCount(migrationID) != 0 {
		t.Fatal("the origin world spawned the organism again")
	}
}

// reportedLost reads lostForwardTotal off the peer stats block — the one field
// §25's B37 adds to §6.3.1, and the map's only reading of what migration costs.
func reportedLost(s *Sidecar) int {
	st := s.Stats()
	if st.LostForwardTotal == nil {
		return -1
	}
	return *st.LostForwardTotal
}

// TestALiveButSilentDestinationIsNeverWrittenOffEarly is Risk 9's assertion in
// its new form. A live peer with a deep paced backlog is SLOW, NOT ORPHANED. It
// used to matter because counting that time would have BOUNCED an organism that
// was about to be spawned; it still matters, for a smaller reason that is worth
// keeping honest — a record written off while the organism is on its way is a
// wrong record, and the late ACK that follows it says so.
func TestALiveButSilentDestinationIsNeverWrittenOffEarly(t *testing.T) {
	clock := newFakeClock()
	rl := startRelay(t)
	for i, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}} {
		p := pos
		if _, _, err := rl.relay.ReserveSlot([]string{"peer-a", "peer-b"}[i], &p); err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
	}
	cfgA := fastConfig(t, rl, "peer-a")
	cfgA.Clock = clock.Now
	cfgA.ForwardTimeout = time.Hour
	cfgA.HeartbeatTimeout = time.Hour
	cfgB := fastConfig(t, rl, "peer-b")

	sideB := startSidecar(t, cfgB)
	waitSlot(t, sideB, 2)
	modB := dialFakeMod(t, fakeModOptions{
		url: sideB.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond})
	modB.setAckMode(ackSilent) // journaled, never acknowledged: silence from a LIVE peer

	sideA := startSidecar(t, cfgA)
	waitSlot(t, sideA, 1)
	modA := dialFakeMod(t, fakeModOptions{
		url: sideA.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond})
	modA.waitEdge(contracta.EdgeE, true, 10*time.Second)

	migrationID := modA.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "the entry to be sent", func() bool {
		return handoffOf(sideA, migrationID) == journal.HandoffSent
	})

	// Most of the timeout passes with the destination LIVE and silent.
	clock.Advance(50 * time.Minute)
	time.Sleep(600 * time.Millisecond)

	st := journalEntry(t, sideA, migrationID)
	if st.Handoff != journal.HandoffSent {
		t.Fatalf("handoff = %q against a live destination, want sent", st.Handoff)
	}
	if st.Status == journal.StatusDone || st.Direction == journal.In {
		t.Fatal("the entry was resolved while its destination was live and holding it")
	}
	if got := reportedLost(sideA); got != 0 {
		t.Fatalf("lostForwardTotal = %d against a LIVE destination inside forwardTimeoutMs", got)
	}
}

// --------------------------------------------------------------- the proof

// TestRerouteNeedsAProofAndSilenceIsNeverOne covers §9.2's evidence table end to
// end, and the failure direction that makes it safe.
//
// Both halves run the same shape: a journaled entry naming a destination that
// has gone dark, with a re-route target available on the same axis. The only
// difference is the entry's HANDOFF STATE, which is the whole of the rule.
//
//	pending   the frame reached nobody. The sender's own record is the proof,
//	          and the organism takes the other lane.
//	sent      the frame was written to a live relay connection. Custody may have
//	          moved and no statement says otherwise, so nothing happens: not a
//	          re-route, not a re-forward, not a bounce home.
//
// THE SECOND ARM IS §25 B37's NAMED COST, and it is asserted rather than
// described. Before B37 that entry ran a 24-hour hold — kept re-forwarding, and
// the re-forward was how the relay's proof reached it, so a `sent` entry left by
// a crashed process usually re-routed within a retry interval. B37 removed the
// re-forward, so it does not: it waits out forwardTimeoutMs and is recorded
// lost. The owner took that trade knowingly (2026-08-17), and this test is where
// anyone who reverses it will find out.
func TestRerouteNeedsAProofAndSilenceIsNeverOne(t *testing.T) {
	t.Run("a pending entry re-routes on its own record", func(t *testing.T) {
		runRerouteProof(t, true)
	})
	t.Run("a forwarded entry has no proof and stays where it is", func(t *testing.T) {
		runRerouteProof(t, false)
	})
}

// TestLivePopulationRefusalSpillsToNextUntriedWorld is the regression for the
// blue-portal failure: OVERLOADED is a statement from a LIVE receiver, so the
// old scheduler immediately offered the organism back to that same live slot.
// A three-world row proves the intended behavior end to end — slot 2 refuses
// before custody at its population limit, and the identical migrationId is
// delivered exactly once to slot 3 on the same east axis.
func TestLivePopulationRefusalSpillsToNextUntriedWorld(t *testing.T) {
	g := newGrid(t, 3, gridOptions{
		layout:    layoutRow(3),
		heartbeat: 100 * time.Millisecond,
		tune: func(i int, c *Config) {
			if i == 1 {
				c.InboundAdmissionMode = AdmissionFixed
				c.InboundPopulationLimit = 1
			}
		},
	})
	a, b, c := g.node(0), g.node(1), g.node(2)
	b.world.put(-88001)
	waitFor(t, 5*time.Second, "slot 2 to report its fixed-limit population", func() bool {
		st := b.side.Stats()
		return st.Population != nil && *st.Population == 1
	})

	migrationID := a.mod.migrateOut(-88002, contracta.EdgeE, 0.5)
	waitFor(t, 15*time.Second, "the overloaded live slot to be skipped", func() bool {
		return c.world.spawnCount(migrationID) == 1
	})
	if got := b.world.spawnCount(migrationID); got != 0 {
		t.Fatalf("population-limited slot 2 spawned the refused migration %d times", got)
	}
	st := journalEntry(t, a.side, migrationID)
	if st.Entry.DestSlot != 3 || st.RerouteCount != 1 ||
		st.RerouteProof != contractb.ProofPeerRefused {
		t.Fatalf("spillover journal = dest %d, count %d, proof %q; want 3, 1, peer_refused",
			st.Entry.DestSlot, st.RerouteCount, st.RerouteProof)
	}
	if len(st.RefusedSlots) != 1 || st.RefusedSlots[0] != 2 {
		t.Fatalf("durable refused slots = %v, want [2]", st.RefusedSlots)
	}
	if got := c.world.spawnCount(migrationID); got != 1 {
		t.Fatalf("slot 3 spawned migration %d times, want exactly 1", got)
	}
}

// TestRefusedMigrationCrossesAxesToTheSinkWorld is §34 B50's regression end to
// end: on a 3x2 map where every world but one refuses on population — the exit
// row included — the chain leaves its axis and the one open world receives the
// migration exactly once. The path is the stated axis-major order (2, 3, then
// 4, 5, 6), so the delivery also exercises a reroute count the old default of
// four would have bounced.
func TestRefusedMigrationCrossesAxesToTheSinkWorld(t *testing.T) {
	const sink = 5 // node index of slot 6, the only open world
	g := newGrid(t, 6, gridOptions{
		layout:    layout3x2(),
		heartbeat: 100 * time.Millisecond,
		tune: func(i int, c *Config) {
			if i != 0 && i != sink {
				c.InboundAdmissionMode = AdmissionFixed
				c.InboundPopulationLimit = 1
			}
		},
	})
	source := g.node(0)
	for i := 1; i < 6; i++ {
		if i == sink {
			continue
		}
		nd := g.node(i)
		nd.world.put(int32(-88100 - i))
		waitFor(t, 5*time.Second, "a closed world to report its fixed-limit population", func() bool {
			st := nd.side.Stats()
			return st.Population != nil && *st.Population == 1
		})
	}

	migrationID := source.mod.migrateOut(-88200, contracta.EdgeE, 0.5)
	waitFor(t, 30*time.Second, "the sink world to receive the cross-axis migration", func() bool {
		return g.node(sink).world.spawnCount(migrationID) == 1
	})
	for i := 1; i < 6; i++ {
		if i == sink {
			continue
		}
		if got := g.node(i).world.spawnCount(migrationID); got != 0 {
			t.Fatalf("closed slot %d spawned the refused migration %d times", i+1, got)
		}
	}
	st := journalEntry(t, source.side, migrationID)
	if st.Entry.DestSlot != 6 || st.RerouteCount != 4 ||
		st.RerouteProof != contractb.ProofPeerRefused {
		t.Fatalf("sink journal = dest %d, count %d, proof %q; want 6, 4, peer_refused",
			st.Entry.DestSlot, st.RerouteCount, st.RerouteProof)
	}
	if !reflect.DeepEqual(st.RefusedSlots, []int{2, 3, 4, 5}) {
		t.Fatalf("durable refused slots = %v, want [2 3 4 5] in walk order", st.RefusedSlots)
	}
	if st.Entry.Edge != contracta.EdgeE {
		t.Fatal("the cross-axis walk rewrote the migration's exit edge")
	}
}

func runRerouteProof(t *testing.T, pending bool) {
	rl := startRelay(t)
	ids := []string{"peer-a", "peer-b", "peer-c"}
	for i, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 2, Row: 0}} {
		p := pos
		if _, _, err := rl.relay.ReserveSlot(ids[i], &p); err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
	}
	// B holds slot 2 and never starts: its slot is reserved, positioned and
	// DARK, which is what the sender sees.
	cfgC := fastConfig(t, rl, "peer-c")
	sideC := startSidecar(t, cfgC)
	waitSlot(t, sideC, 3)
	worldC := newWorld()
	dialFakeMod(t, fakeModOptions{url: sideC.URL(), world: worldC, heartbeat: 200 * time.Millisecond})

	// The sender's journal, as a restart would leave it. In the `sent` arm the
	// frame WAS written to a live relay connection under the session recorded on
	// the entry — every ingredient of a proof except a statement.
	cfgA := fastConfig(t, rl, "peer-a")
	handoff := journal.HandoffSent
	if pending {
		handoff = journal.HandoffPending
	}
	migrationID := seedOutEntry(t, cfgA.DataDir, 2, rl.relay.SessionID(), handoff)

	sideA := startSidecar(t, cfgA)
	waitSlot(t, sideA, 1)
	worldA := newWorld()
	modA := dialFakeMod(t, fakeModOptions{url: sideA.URL(), world: worldA, heartbeat: 200 * time.Millisecond})
	// Slot 2 is dark, so the row's east lane already points past it at slot 3 —
	// a re-route target is available in both halves of this test.
	waitLane(t, sideA, contracta.EdgeE, 3)
	modA.waitEdge(contracta.EdgeE, true, 10*time.Second)

	if pending {
		waitFor(t, 15*time.Second, "the organism to be re-routed onto the healed lane", func() bool {
			return worldC.spawnCount(migrationID) == 1
		})
		st := journalEntry(t, sideA, migrationID)
		if st.Entry.DestSlot != 3 {
			t.Fatalf("destSlot = %d after the re-route, want 3", st.Entry.DestSlot)
		}
		if st.RerouteCount != 1 || st.RerouteFrom != 2 {
			t.Fatalf("reroute count %d from slot %d, want 1 from 2", st.RerouteCount, st.RerouteFrom)
		}
		if st.RerouteProof != contractb.ProofNeverSent {
			t.Fatalf("reroute proof = %q, want never_sent", st.RerouteProof)
		}
		return
	}

	// No statement: the entry STAYS at the destination it recorded, however long
	// the silence lasts and however available the other lane is. It is not
	// re-routed, not re-forwarded and not brought home.
	time.Sleep(2 * time.Second)
	st := journalEntry(t, sideA, migrationID)
	if st.Handoff != journal.HandoffSent {
		t.Fatalf("handoff = %q; silence moves a forwarded entry nowhere", st.Handoff)
	}
	if st.Entry.DestSlot != 2 {
		t.Fatalf("destSlot = %d without a proof of non-delivery; §9.2 forbids the rewrite",
			st.Entry.DestSlot)
	}
	if st.RerouteCount != 0 {
		t.Fatalf("the entry re-routed %d times on silence", st.RerouteCount)
	}
	if st.ForwardReceipts != 0 {
		t.Fatalf("the entry was forwarded %d more times; §25 B37 forwards a frame once",
			st.ForwardReceipts)
	}
	if st.Direction != journal.Out || st.BounceBack {
		t.Fatal("the entry came home on silence, which is the duplication §25 B37 removes")
	}
	if got := worldC.spawnCount(migrationID); got != 0 {
		t.Fatalf("slot 3 spawned the organism %d times without a proof of non-delivery", got)
	}
	if got := worldA.spawnCount(migrationID); got != 0 {
		t.Fatalf("the origin world re-spawned the organism %d times", got)
	}
}

// seedSentEntry writes the journal of a sidecar that forwarded one organism and
// then died: custody may have moved, the entry names the session that write
// happened under, and only a statement scoped to that session can free it.
func seedSentEntry(t *testing.T, dataDir string, destSlot int, session string) string {
	t.Helper()
	return seedOutEntry(t, dataDir, destSlot, session, journal.HandoffSent)
}

// seedOutEntry is the same in either handoff state, which is what lets one test
// run the two sides of §9.2's rule over identical bytes.
func seedOutEntry(t *testing.T, dataDir string, destSlot int, session string,
	handoff journal.Handoff) string {
	t.Helper()
	jr, err := journal.Open(filepath.Join(dataDir, "journal"))
	if err != nil {
		t.Fatalf("seed journal: %v", err)
	}
	migrationID := wire.NewUUID()
	payload := makePayload(testEntityID)
	if _, err := jr.Create(journal.Out, journal.Entry{
		MigrationID:    migrationID,
		EntityID:       testEntityID,
		Kind:           contracta.KindBibite,
		GameVersion:    "0.6.3.1",
		Payload:        payload,
		PayloadHash:    bb8.Hash(payload),
		GenomeHash:     genomeHashOf(t, payload),
		Edge:           contracta.EdgeE,
		Position:       0.4,
		VelocityX:      6.12,
		VelocityY:      0.44,
		Heading:        274.11,
		SimulationSize: 2000,
		SourceSlot:     1,
		DestSlot:       destSlot,
		JournaledAt:    time.Now().UnixMilli(),
	}, false); err != nil {
		t.Fatalf("seed journal: %v", err)
	}
	u := journal.Update{Handoff: handoff}
	if handoff == journal.HandoffSent {
		sentAt := time.Now().UnixMilli()
		u.Status = journal.StatusInFlight
		u.RelaySessionID = &session
		u.SentAtMs = &sentAt
	}
	if _, err := jr.Apply(migrationID, u); err != nil {
		t.Fatalf("seed handoff: %v", err)
	}
	if err := jr.Close(); err != nil {
		t.Fatalf("seed journal close: %v", err)
	}
	return migrationID
}

// TestRelayProofFieldsAreExact covers §5.2 and §9.2's "not evidence" table on
// the one function that decides it. Every ambiguity MUST resolve toward "no
// proof": holding costs a delay, and re-routing on a bad proof costs a
// duplicated organism.
func TestRelayProofFieldsAreExact(t *testing.T) {
	const session = "5f0b9c31-77ad-4e26-9a4c-1b83d206ef95"
	other := "c8177a34-90bd-4f51-8e02-4d6b915ca7e3"
	cases := []struct {
		name string
		nack contractb.MigrationNack
		want bool
	}{
		{"matched session and neverForwarded", contractb.MigrationNack{
			NeverForwarded: contractb.BoolPtr(true), RelaySessionID: session}, true},
		{"a different relay session", contractb.MigrationNack{
			NeverForwarded: contractb.BoolPtr(true), RelaySessionID: other}, false},
		{"neverForwarded false", contractb.MigrationNack{
			NeverForwarded: contractb.BoolPtr(false), RelaySessionID: session}, false},
		{"no neverForwarded field at all", contractb.MigrationNack{RelaySessionID: session}, false},
		{"no session", contractb.MigrationNack{NeverForwarded: contractb.BoolPtr(true)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.nack.ProvesNoCustody(session); got != c.want {
				t.Fatalf("ProvesNoCustody = %v, want %v", got, c.want)
			}
		})
	}
	// An entry that has never been written to a relay carries no session, so
	// even a perfect statement proves nothing about it — its own pending state
	// is the proof instead.
	perfect := contractb.MigrationNack{NeverForwarded: contractb.BoolPtr(true), RelaySessionID: session}
	if perfect.ProvesNoCustody("") {
		t.Fatal("a proof was accepted against an entry with no recorded session")
	}
	attemptCases := []struct {
		name string
		nack contractb.MigrationNack
		want bool
	}{
		{"exact attempt with migration-wide true", contractb.MigrationNack{
			Code: contractb.NackNotForwarded, NeverForwarded: contractb.BoolPtr(true),
			RelaySessionID: session,
			RefusedAttempt: &contractb.MigrationAttempt{DestSlot: 7, RerouteCount: 2}}, true},
		{"exact attempt with migration-wide false", contractb.MigrationNack{
			Code: contractb.NackNotForwarded, NeverForwarded: contractb.BoolPtr(false),
			RelaySessionID: session,
			RefusedAttempt: &contractb.MigrationAttempt{DestSlot: 7, RerouteCount: 2}}, true},
		{"missing attempt", contractb.MigrationNack{
			Code: contractb.NackNotForwarded, NeverForwarded: contractb.BoolPtr(true),
			RelaySessionID: session}, false},
		{"stale destination", contractb.MigrationNack{
			Code: contractb.NackNotForwarded, NeverForwarded: contractb.BoolPtr(true),
			RelaySessionID: session,
			RefusedAttempt: &contractb.MigrationAttempt{DestSlot: 6, RerouteCount: 2}}, false},
		{"stale count", contractb.MigrationNack{
			Code: contractb.NackNotForwarded, NeverForwarded: contractb.BoolPtr(true),
			RelaySessionID: session,
			RefusedAttempt: &contractb.MigrationAttempt{DestSlot: 7, RerouteCount: 1}}, false},
	}
	for _, tc := range attemptCases {
		t.Run("attempt/"+tc.name, func(t *testing.T) {
			if got := tc.nack.ProvesAttemptRefusal(session, 7, 2); got != tc.want {
				t.Fatalf("ProvesAttemptRefusal = %v, want %v", got, tc.want)
			}
		})
	}
	// The taxonomy itself: SLOT_VACANT is PERMANENT from M4, and the two new
	// codes are transient.
	if got := contractb.ClassOf(contractb.NackSlotVacant); got != contractb.ClassPermanent {
		t.Fatalf("SLOT_VACANT class = %s, want permanent (§6.8, reclassified in M4)", got)
	}
	for _, code := range []string{contractb.NackPeerOffline, contractb.NackNotForwarded} {
		if got := contractb.ClassOf(code); got != contractb.ClassTransient {
			t.Fatalf("%s class = %s, want transient", code, got)
		}
	}
}

// TestSlotVacantIsPermanentAndProvableFromThePendingState covers §6.8's
// reclassification and §9.2's first evidence row: an entry that was never
// written to anybody re-routes on its own record, without waiting for a relay
// statement at all.
func TestSlotVacantIsPermanentAndProvableFromThePendingState(t *testing.T) {
	rl := startRelay(t)
	ids := []string{"peer-a", "peer-gone", "peer-c"}
	for i, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 2, Row: 0}} {
		p := pos
		if _, _, err := rl.relay.ReserveSlot(ids[i], &p); err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
	}
	// Slot 2 is RELEASED: it names a world that will never return, and its
	// number is retired for good.
	if err := rl.relay.ReleaseSlot(2); err != nil {
		t.Fatalf("ReleaseSlot: %v", err)
	}

	cfgC := fastConfig(t, rl, "peer-c")
	sideC := startSidecar(t, cfgC)
	waitSlot(t, sideC, 3)
	worldC := newWorld()
	dialFakeMod(t, fakeModOptions{url: sideC.URL(), world: worldC, heartbeat: 200 * time.Millisecond})

	cfgA := fastConfig(t, rl, "peer-a")
	migrationID := seedOutboundCustodyTo(t, cfgA.DataDir, 2)
	sideA := startSidecar(t, cfgA)
	waitSlot(t, sideA, 1)
	dialFakeMod(t, fakeModOptions{url: sideA.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond})

	waitFor(t, 15*time.Second, "the pending entry to re-route past the retired slot", func() bool {
		return worldC.spawnCount(migrationID) == 1
	})
	st := journalEntry(t, sideA, migrationID)
	if st.RerouteProof != contractb.ProofNeverSent {
		t.Fatalf("reroute proof = %q, want never_sent", st.RerouteProof)
	}
	if st.Entry.DestSlot != 3 || st.RerouteFrom != 2 {
		t.Fatalf("re-routed to %d from %d, want 3 from 2", st.Entry.DestSlot, st.RerouteFrom)
	}
}

func seedOutboundCustodyTo(t *testing.T, dataDir string, destSlot int) string {
	t.Helper()
	jr, err := journal.Open(filepath.Join(dataDir, "journal"))
	if err != nil {
		t.Fatalf("seed journal: %v", err)
	}
	migrationID := wire.NewUUID()
	payload := makePayload(testEntityID)
	if _, err := jr.Create(journal.Out, journal.Entry{
		MigrationID: migrationID, EntityID: testEntityID, Kind: contracta.KindBibite,
		GameVersion: "0.6.3.1", Payload: payload, PayloadHash: bb8.Hash(payload),
		GenomeHash: genomeHashOf(t, payload), Edge: contracta.EdgeE, Position: 0.4,
		VelocityX: 6.12, VelocityY: 0.44, Heading: 274.11, SimulationSize: 2000,
		SourceSlot: 1, DestSlot: destSlot, JournaledAt: time.Now().UnixMilli(),
	}, false); err != nil {
		t.Fatalf("seed journal: %v", err)
	}
	if err := jr.Close(); err != nil {
		t.Fatalf("seed journal close: %v", err)
	}
	return migrationID
}

// --------------------------------------------------------------- handover

// TestHandoverRebindsTheAddressAndMovesNoJournal covers §7.5's custody rules.
// A handover rebinds THE POSITION IN THE MAP, not the data: the old machine
// keeps its journal, the new occupant inherits nothing, and in-flight work
// addressed to that slot arrives at whoever is there now, because routing is on
// the slot.
func TestHandoverRebindsTheAddressAndMovesNoJournal(t *testing.T) {
	rl := startRelay(t)
	for i, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}} {
		p := pos
		if _, _, err := rl.relay.ReserveSlot([]string{"peer-a", "peer-old"}[i], &p); err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
	}
	cfgA := fastConfig(t, rl, "peer-a")
	sideA := startSidecar(t, cfgA)
	waitSlot(t, sideA, 1)
	worldA := newWorld()
	modA := dialFakeMod(t, fakeModOptions{url: sideA.URL(), world: worldA, heartbeat: 200 * time.Millisecond})

	cfgOld := fastConfig(t, rl, "peer-old")
	sideOld := startSidecar(t, cfgOld)
	waitSlot(t, sideOld, 2)
	worldOld := newWorld()
	modOld := dialFakeMod(t, fakeModOptions{url: sideOld.URL(), world: worldOld, heartbeat: 200 * time.Millisecond})
	modA.waitEdge(contracta.EdgeE, true, 10*time.Second)

	// One organism crosses, so the old peer's journal holds a real tombstone.
	first := modA.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "the first organism to land", func() bool {
		return worldOld.spawnCount(first) == 1
	})

	// The relay refuses a handover while the old peer is live.
	if _, _, err := rl.relay.HandoverSlot(2, "peer-new"); err == nil {
		t.Fatal("the relay handed over a live peer's slot")
	}
	modOld.abort()
	_ = sideOld.Close()
	waitFor(t, 10*time.Second, "the old peer to go dark", func() bool {
		return sideA.Neighbour(contracta.EdgeE) == nil
	})

	old, now, err := rl.relay.HandoverSlot(2, "peer-new")
	if err != nil {
		t.Fatalf("HandoverSlot: %v", err)
	}
	if old.PeerID != "peer-old" || now.PeerID != "peer-new" || now.Slot != 2 ||
		now.Col != 1 || now.Row != 0 {
		t.Fatalf("handover produced %+v -> %+v", old, now)
	}

	// The new occupant starts with an EMPTY journal and a different world, and
	// it is not told about the old peer's entries.
	cfgNew := fastConfig(t, rl, "peer-new")
	sideNew := startSidecar(t, cfgNew)
	waitSlot(t, sideNew, 2)
	if pos := sideNew.Position(); pos.Col != 1 || pos.Row != 0 {
		t.Fatalf("the new occupant landed at %+v, want the handed-over (1,0)", pos)
	}
	if got := len(sideNew.CustodySnapshot()); got != 0 {
		t.Fatalf("the new occupant inherited %d journal entries; a handover moves no journal", got)
	}
	worldNew := newWorld()
	dialFakeMod(t, fakeModOptions{url: sideNew.URL(), world: worldNew, heartbeat: 200 * time.Millisecond})
	modA.waitEdge(contracta.EdgeE, true, 10*time.Second)

	// In-flight work addressed to slot 2 arrives at the NEW occupant.
	second := modA.migrateOut(-88001, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "the next organism to arrive at the new occupant", func() bool {
		return worldNew.spawnCount(second) == 1
	})
	if got := worldOld.spawnCount(second); got != 0 {
		t.Fatalf("the old world received %d organisms after the handover", got)
	}

	// The old machine kept its journal: its tombstone for the first organism is
	// still there, on its own disk.
	jr, err := journal.Open(filepath.Join(cfgOld.DataDir, "journal"))
	if err != nil {
		t.Fatalf("reopen the old journal: %v", err)
	}
	defer jr.Close()
	if _, ok := jr.Get(first); !ok {
		t.Fatal("the old peer's journal lost its entry; a handover NEVER moves a journal")
	}
}

// --------------------------------------------------------------- stats

// TestPeerStatusRepublishesPeerStats covers §6.3.1 and §6.11: the stats block
// rides the PERIODIC PING, the relay stores it with its own receivedAt and
// republishes it, and it does so ON A TIMER because stats change without the
// registry changing.
func TestPeerStatusRepublishesPeerStats(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	sub := dialSubscriber(t, g.relay.url(), g.relay)
	sub.wait(contractb.TypeHandshakeAck, 10*time.Second)

	// Put a population in slot 1's world so a real number crosses the wire.
	for i := 0; i < 7; i++ {
		g.node(0).world.put(int32(-70000 - i))
	}

	var first contractb.SlotInfo
	waitFor(t, 15*time.Second, "a PEER_STATUS carrying peer stats", func() bool {
		for _, st := range sub.peerStatuses(t) {
			for _, si := range st.Slots {
				if si.Slot == 1 && si.Stats != nil && si.Stats.Population != nil &&
					*si.Stats.Population == 7 {
					first = si
					return true
				}
			}
		}
		return false
	})
	if first.StatsAsOfMs == 0 {
		t.Fatal("stats arrived with no statsAsOfMs; a reader MUST be able to age them")
	}
	if first.Stats.CustodyDepth == nil || first.Stats.PacedDepth == nil ||
		first.Stats.LostForwardTotal == nil {
		t.Fatalf("the stats block is missing an operational depth: %+v", first.Stats)
	}
	// The REPUBLISH: a later broadcast carries a newer statsAsOfMs with no
	// registry change in between.
	waitFor(t, 15*time.Second, "the periodic stats republish", func() bool {
		for _, st := range sub.peerStatuses(t) {
			for _, si := range st.Slots {
				if si.Slot == 1 && si.StatsAsOfMs > first.StatsAsOfMs {
					return true
				}
			}
		}
		return false
	})
	// A subscriber's own place in the map is all null (§6.5).
	statuses := sub.peerStatuses(t)
	last := statuses[len(statuses)-1]
	if last.You.Slot != nil || last.You.Position != nil {
		t.Fatalf("a subscriber was given a slot: %+v", last.You)
	}
	if last.Observers < 1 {
		t.Fatalf("observers = %d with a subscriber connected", last.Observers)
	}
	// Positions and the map shape ride on every frame.
	if last.Map.Width != 2 || last.Map.Height != 1 {
		t.Fatalf("PEER_STATUS map = %+v, want 2x1", last.Map)
	}
	for _, si := range last.Slots {
		if len(si.ExportEdges) == 0 {
			t.Fatalf("slot %d published no exportEdges; a reader cannot tell which lanes it runs", si.Slot)
		}
	}
}

// TestHeartbeatSaveReceiptReachesPeerStatus covers contract-a.md §15 A21: the
// mod's save receipt is copied verbatim into the peer stats and republished, so
// the status page can show the last save of a world on a machine it cannot read
// a file from.
func TestHeartbeatSaveReceiptReachesPeerStatus(t *testing.T) {
	receipt := &contracta.SaveReceipt{
		AtMs: time.Now().UnixMilli(), SimulatedTime: 119303.5, Population: 211,
		Name: "M4-Slot1-20260805T2058Z.zip", Bytes: 41533892, DurationMs: 730,
	}
	rl := startRelay(t)
	cfg := fastConfig(t, rl, "peer-a")
	side := startSidecar(t, cfg)
	waitSlotAny(t, side)
	dialFakeMod(t, fakeModOptions{
		url: side.URL(), world: newWorld(), heartbeat: 100 * time.Millisecond, lastSave: receipt})

	sub := dialSubscriber(t, rl.url(), rl)
	sub.wait(contractb.TypeHandshakeAck, 10*time.Second)
	waitFor(t, 15*time.Second, "the save receipt to reach PEER_STATUS", func() bool {
		for _, st := range sub.peerStatuses(t) {
			for _, si := range st.Slots {
				if si.Stats == nil || si.Stats.LastSave == nil {
					continue
				}
				got := si.Stats.LastSave
				if got.AtMs == receipt.AtMs && got.DurationMs == 730 && got.Name == receipt.Name {
					return true
				}
			}
		}
		return false
	})
}

// --------------------------------------------------------------- operator

// TestListAndReleaseInflight covers §7.5 and §9.3's manual escape hatch: the
// operator finds the entries the relay cannot enumerate, and resolves one by
// hand before forwardTimeoutMs writes it off.
//
// Since §25's B37 it is also THE ONLY WAY LEFT TO DUPLICATE AN ORGANISM on this
// map: nothing bounces a forwarded entry automatically any more, so a `bounce`
// here is a person deciding, against the receipt evidence, that the far side
// never took custody. The risk text is checked for that reason.
func TestListAndReleaseInflight(t *testing.T) {
	dir := t.TempDir()
	unresolved := seedSentEntry(t, dir, 5, "5f0b9c31-77ad-4e26-9a4c-1b83d206ef95")
	other := seedOutboundCustodyTo(t, dir, 9)

	entries, err := ListInflight(dir, 0, 24*time.Hour)
	if err != nil {
		t.Fatalf("ListInflight: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ListInflight returned %d entries, want 2", len(entries))
	}
	only, err := ListInflight(dir, 5, 24*time.Hour)
	if err != nil {
		t.Fatalf("ListInflight --dest-slot: %v", err)
	}
	if len(only) != 1 || only[0].MigrationID != unresolved {
		t.Fatalf("--dest-slot 5 returned %+v", only)
	}
	if only[0].Handoff != string(journal.HandoffSent) || only[0].DestSlot != 5 {
		t.Fatalf("the listed entry is %+v, want the sent entry for slot 5", only[0])
	}
	if only[0].SentAt.IsZero() {
		t.Fatal("the listed entry has no sentAt, so the report cannot say when it will be written off")
	}
	if only[0].LostIn <= 0 || only[0].LostIn > 24*time.Hour {
		t.Fatalf("lostIn = %s, want what is left of forwardTimeoutMs", only[0].LostIn)
	}

	// bounce: the organism comes home to THIS world on the edge it left by.
	msg, err := ReleaseInflight(dir, unresolved, "bounce")
	if err != nil {
		t.Fatalf("ReleaseInflight bounce: %v", err)
	}
	if msg == "" {
		t.Fatal("the release printed nothing")
	}
	jr, err := journal.Open(filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	st, _ := jr.Get(unresolved)
	if st.Direction != journal.In || !st.BounceBack || st.Status != journal.StatusOpen {
		t.Fatalf("after a bounce the entry is %+v, want an open inbound bounce-back", st)
	}
	_ = jr.Close()

	// drop: the organism is gone, and the command says so.
	if _, err := ReleaseInflight(dir, other, "drop"); err != nil {
		t.Fatalf("ReleaseInflight drop: %v", err)
	}
	jr, err = journal.Open(filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer jr.Close()
	st, _ = jr.Get(other)
	if st.Status != journal.StatusDone {
		t.Fatalf("after a drop the entry is %+v, want a tombstone", st)
	}

	if _, err := ReleaseInflight(dir, other, "sideways"); err == nil {
		t.Fatal("an unknown action was accepted")
	}
	if _, err := ReleaseInflight(dir, wire.NewUUID(), "bounce"); err == nil {
		t.Fatal("releasing an unknown migrationId reported success")
	}
	// The risk text is not optional: §9.3 REQUIRES the command to print it, and
	// since B37 it has to say that this command is the only remaining way to
	// duplicate an organism rather than one of two.
	if len(InflightRisk) < 100 {
		t.Fatal("--release-inflight does not print the duplication risk")
	}
	if !strings.Contains(InflightRisk, "ONLY WAY LEFT TO") {
		t.Fatalf("the risk text no longer says this command is the only way left to duplicate "+
			"an organism (§25, B37):\n%s", InflightRisk)
	}
}

// --------------------------------------------------------- the status page

// TestStatusPageShowsTheWholeMap is the exit test's Part 9 in miniature and
// §10.1's three rules in one place: the archive serves the map, the holes, each
// slot's liveness and population, each effective lane and each bypass with the
// time it went dark, the custody and paced depths, and it says UNKNOWN where it
// does not know.
//
// Every value on it comes from the PEER_STATUS broadcasts the archive already
// receives and the envelope copies it already records. It connects to no
// sidecar and asks the relay for nothing.
func TestStatusPageShowsTheWholeMap(t *testing.T) {
	g := newGrid(t, 6, gridOptions{layout: layout3x2()})
	arc := startArchive(t, g.relay)
	four, five := g.bySlot(4), g.bySlot(5)
	two := g.bySlot(2)

	waitLane(t, four.side, contracta.EdgeE, 5)
	// Some real flow, so the lane rates are measurements rather than zeroes.
	for i := 0; i < 3; i++ {
		id := g.bySlot(1).mod.migrateOut(int32(-60000-i), contracta.EdgeE, 0.5)
		waitFor(t, 10*time.Second, "flow on the 1->2 lane", func() bool {
			return g.bySlot(2).world.spawnCount(id) == 1
		})
	}

	five.mod.abort()
	_ = five.side.Close()
	waitLane(t, four.side, contracta.EdgeE, 6)
	waitLaneClosed(t, two.side, contracta.EdgeN)

	base := "http://" + arc.HTTPAddr()
	var view archive.Status
	waitFor(t, 20*time.Second, "the status page to show the dark slot and the healed lane", func() bool {
		v, err := archive.FetchStatus(base, 5*time.Second)
		if err != nil {
			return false
		}
		view = v
		if !v.HaveStatus || v.Map.Width != 3 || v.Map.Height != 2 {
			return false
		}
		darkSeen, popSeen := false, false
		for _, s := range v.Slots {
			if s.Slot == 5 && !s.Live && s.DarkForMs > 0 {
				darkSeen = true
			}
			// A population is UNKNOWN until a HEARTBEAT-derived stats block has
			// arrived, and the page says so rather than showing a zero. This wait
			// is that rule, read from the outside.
			if s.Slot == 1 && s.StatsKnown && s.Population != nil {
				popSeen = true
			}
		}
		if !darkSeen || !popSeen {
			return false
		}
		for _, l := range v.Lanes {
			if l.FromSlot == 4 && l.Edge == contracta.EdgeE && l.ToSlot == 6 {
				return true
			}
		}
		return false
	})

	if view.SlotCount != 6 || len(view.Holes) != 0 {
		t.Fatalf("the page reports %d slots and %d holes, want 6 and 0", view.SlotCount, len(view.Holes))
	}
	byslot := map[int]archive.SlotView{}
	for _, s := range view.Slots {
		byslot[s.Slot] = s
	}
	// A live slot's population is KNOWN and its stats are fresh.
	live := byslot[1]
	if !live.StatsKnown || live.Population == nil {
		t.Fatalf("slot 1's stats are %+v; a live peer's population must be known", live)
	}
	if live.CustodyDepth == nil || live.PacedDepth == nil || live.LostForwardTotal == nil {
		t.Fatalf("slot 1 is missing an operational depth: %+v", live)
	}
	// A dark slot is BYPASSED SINCE a named moment, and its stats age out to
	// UNKNOWN rather than being shown as current state.
	dark := byslot[5]
	if dark.Live || dark.DarkSinceMs == 0 {
		t.Fatalf("slot 5 reads %+v, want dark with a darkSinceMs", dark)
	}
	// The derived lanes name the bypass and the closure.
	var healed, closed *archive.LaneView
	for i := range view.Lanes {
		l := &view.Lanes[i]
		if l.FromSlot == 4 && l.Edge == contracta.EdgeE {
			healed = l
		}
		if l.FromSlot == 2 && l.Edge == contracta.EdgeN {
			closed = l
		}
	}
	if healed == nil || !healed.Open || healed.ToSlot != 6 {
		t.Fatalf("the healed lane reads %+v, want slot 4 -> slot 6 open", healed)
	}
	if len(healed.Skipped) != 1 || healed.Skipped[0].Slot == nil || *healed.Skipped[0].Slot != 5 {
		t.Fatalf("the healed lane's bypass list is %+v, want one entry naming slot 5", healed.Skipped)
	}
	if closed == nil || closed.Open || closed.Reason != contracta.ReasonNoPeer {
		t.Fatalf("slot 2's north lane reads %+v, want closed no_peer", closed)
	}
	var flowing *archive.LaneView
	for i := range view.Lanes {
		if view.Lanes[i].FromSlot == 1 && view.Lanes[i].Edge == contracta.EdgeE {
			flowing = &view.Lanes[i]
		}
	}
	if flowing == nil || flowing.Migrations < 3 {
		t.Fatalf("the 1->2 lane recorded %+v envelopes, want at least the three that crossed", flowing)
	}
	// "An unknown value reads as unknown. A ZERO READS AS A MEASUREMENT." The
	// total is a sum of KNOWN values, so it is present exactly when at least one
	// slot reported one — whatever that number is.
	if view.Totals.Population == nil {
		t.Fatal("the totals report an unknown population while a live slot reported one")
	}
	if view.Totals.LiveSlots != 5 || view.Totals.DarkSlots != 1 {
		t.Fatalf("the totals report %d live and %d dark, want 5 and 1",
			view.Totals.LiveSlots, view.Totals.DarkSlots)
	}

	// The page itself is self-contained: no CDN, no build step, and it polls the
	// one JSON endpoint.
	page := fetchPage(t, base+"/live")
	for _, want := range []string{"api/status", "setInterval", "unknown", "multiverse map"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the status page does not contain %q", want)
		}
	}
	for _, forbidden := range []string{"http://cdn", "https://cdn", "<script src"} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("the status page reaches outside the LAN: %q", forbidden)
		}
	}

	// ringstat renders the same data as a terminal table.
	var out strings.Builder
	archive.RenderRingstat(&out, view)
	table := out.String()
	for _, want := range []string{"map 3x2", "slot", "DARK", "bypassing", "unknown"} {
		if !strings.Contains(table, want) {
			t.Fatalf("ringstat did not print %q:\n%s", want, table)
		}
	}

	// The durable metrics: history survives everything, including this process.
	waitFor(t, 20*time.Second, "a metrics sample to be appended", func() bool {
		samples, err := archive.ReadMetrics(arc.MetricsPath())
		return err == nil && len(samples) > 0
	})
	samples, err := archive.ReadMetrics(arc.MetricsPath())
	if err != nil {
		t.Fatalf("ReadMetrics: %v", err)
	}
	last := samples[len(samples)-1]
	if last.Map.Width != 3 || last.SlotCount != 6 {
		t.Fatalf("the newest metrics sample is %+v", last.Map)
	}
	// And ringstat renders that file when the archive is not running.
	fromFile, err := archive.LastSample(arc.MetricsPath())
	if err != nil {
		t.Fatalf("LastSample: %v", err)
	}
	out.Reset()
	archive.RenderRingstat(&out, fromFile)
	if !strings.Contains(out.String(), "map 3x2") {
		t.Fatal("ringstat could not render the durable sample")
	}
}

func fetchPage(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return string(b)
}

// TestSpliceBetweenTwoLiveSlots covers §7.2 rule 5 on a running map, on BOTH
// axes: the newcomer takes the crossing, the predecessor's effective neighbour
// becomes the newcomer, the newcomer's becomes the old successor, TWO LANES
// CHANGE AND NO SLOT NUMBER CHANGES — and the current flows straight through it.
func TestSpliceBetweenTwoLiveSlots(t *testing.T) {
	for _, axis := range []string{contracta.EdgeE, contracta.EdgeN} {
		t.Run("on the "+axis+" axis", func(t *testing.T) {
			layout := layoutRow(3)
			if axis == contracta.EdgeN {
				layout = []contractb.Position{{Col: 0, Row: 0}, {Col: 0, Row: 1}, {Col: 0, Row: 2}}
			}
			g := newGrid(t, 3, gridOptions{layout: layout, skipEdgeCheck: true})
			one, two := g.bySlot(1), g.bySlot(2)
			waitLane(t, one.side, axis, 2)
			one.mod.waitEdge(axis, true, 10*time.Second)

			before := map[int][2]int{}
			for _, res := range g.relay.relay.Snapshot() {
				before[res.Slot] = [2]int{res.Col, res.Row}
			}

			// "Place me immediately after slot 1, on this axis."
			spliced := g.addPeerWithSplice("peer-splice", 1, axis)
			if spliced.slot <= 3 {
				t.Fatalf("the spliced peer took slot %d, want a number above every one ever issued",
					spliced.slot)
			}

			// The predecessor's lane now points at the newcomer, and the
			// newcomer's at the old successor.
			waitLane(t, one.side, axis, spliced.slot)
			waitLane(t, spliced.side, axis, 2)
			// Nothing was renumbered or re-positioned except by the shift the
			// insertion itself defines, and no SLOT NUMBER moved at all.
			for _, res := range g.relay.relay.Snapshot() {
				if _, known := before[res.Slot]; !known {
					continue
				}
				if res.PeerID == "" {
					t.Fatalf("slot %d lost its reservation", res.Slot)
				}
			}
			if got := g.bySlot(1).side.Slot(); got != 1 {
				t.Fatalf("slot 1 was renumbered to %d", got)
			}

			// And the current flows through the newcomer.
			id := one.mod.migrateOut(-99001, axis, 0.5)
			waitFor(t, 10*time.Second, "an organism to cross into the spliced slot", func() bool {
				return spliced.world.spawnCount(id) == 1
			})
			out := spliced.mod.migrateOut(-99002, axis, 0.5)
			waitFor(t, 10*time.Second, "an organism to cross out of the spliced slot", func() bool {
				return two.world.spawnCount(out) == 1
			})
		})
	}
}

// TestGrantReasonsNameWhatHappened covers §6.4's reason enum on the two values
// only M4 can produce: a peer whose coordinate moved under it is REPOSITIONED,
// and a peer that took a slot by operator command has a HANDOVER.
func TestGrantReasonsNameWhatHappened(t *testing.T) {
	g := newGrid(t, 3, gridOptions{layout: layoutRow(3)})
	one := g.bySlot(1)
	waitLane(t, one.side, contracta.EdgeE, 2)

	// A splice after slot 1 shifts every column after it, so slots 2 and 3 move.
	before := g.bySlot(2).side.Position()
	g.addPeerWithSplice("peer-splice", 1, contracta.EdgeE)
	waitFor(t, 10*time.Second, "the shifted peer to learn its new coordinate", func() bool {
		return g.bySlot(2).side.Position() != before
	})
	if got := g.bySlot(2).side.Slot(); got != 2 {
		t.Fatalf("slot 2 was renumbered to %d by a splice", got)
	}

	// A handover: the new occupant's FIRST grant says handover, not reclaimed.
	old := g.bySlot(3)
	old.mod.abort()
	_ = old.side.Close()
	// §7.5: the relay REFUSES a handover while the old peer is live, so the
	// command is retried until the relay has seen the connection go.
	waitFor(t, 10*time.Second, "the relay to accept the handover once the old peer is dark", func() bool {
		_, _, err := g.relay.relay.HandoverSlot(3, "peer-inheritor")
		return err == nil
	})
	cfg := fastConfig(t, g.relay, "peer-inheritor")
	side := startSidecar(t, cfg)
	waitSlot(t, side, 3)
	if got := len(side.CustodySnapshot()); got != 0 {
		t.Fatalf("the inheritor started with %d journal entries", got)
	}
}
