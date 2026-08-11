package sidecar

import (
	"os"
	"strconv"
	"testing"
	"time"

	"multiverse/internal/archive"
	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/wire"
)

// Two-way lanes end to end: contract-a.md §18 (A38, A40) and
// contract-b-m4.md §17 (B13, B15), through the real rig — a relay, six
// sidecars and six fake mods.

// TestFourDirectionMigrationThroughTheGrid is A38's whole claim, exercised
// rather than asserted: EVERY declared edge is both an export edge and an entry
// edge, and an organism leaving by any of the four arrives at the peer that
// edge's walk names, on the OPPOSITE edge.
//
// "Out and in are different doors" is retired here. Slot 1 exports east to slot
// 2 and west to slot 3, north to slot 4 and south to slot 4 as well — its
// column is an axis of two — and each arrival lands on the facing edge with the
// blob, the position, the velocity and the heading copied and never mirrored.
func TestFourDirectionMigrationThroughTheGrid(t *testing.T) {
	g := newGrid(t, 6, gridOptions{layout: layout3x2()})
	origin := g.bySlot(1) // (0,0)

	// Row 0 is 1 2 3 and column 0 is 1 over 4, so: E->2, W->3 (wrapping), and
	// both vertical lanes name slot 4.
	want := map[string]int{
		contracta.EdgeE: 2, contracta.EdgeW: 3, contracta.EdgeN: 4, contracta.EdgeS: 4,
	}
	for _, edge := range contracta.CanonicalEdges() {
		waitLane(t, origin.side, edge, want[edge])
		origin.mod.waitEdge(edge, true, 10*time.Second)
	}

	payload := makePayload(testEntityID)
	for _, edge := range contracta.CanonicalEdges() {
		dest := g.bySlot(want[edge])
		entry, _ := contracta.Opposite(edge)
		before := dest.mod.frameCount()
		migrationID := origin.mod.migrateOutPayload(
			wire.NewUUID(), testEntityID, edge, 0.5, payload)

		env, _ := dest.mod.waitFrom(before, 10*time.Second,
			func(e wire.Envelope) bool { return e.Type == contracta.TypeMigrateIn })
		in := decodeAs[contracta.MigrateIn](t, env)
		if in.MigrationID != migrationID {
			t.Fatalf("%s hop delivered %s, want %s", edge, in.MigrationID, migrationID)
		}
		if in.EntryEdge != entry {
			t.Fatalf("%s hop arrived on %s, want the facing edge %s", edge, in.EntryEdge, entry)
		}
		if in.Payload != payload {
			t.Fatalf("%s hop changed the blob", edge)
		}
		if in.EntryPosition != 0.5 {
			t.Fatalf("%s hop entryPosition = %v; entryPosition is exitPosition, copied",
				edge, in.EntryPosition)
		}
		// §4.4: velocity and heading are COPIED, never mirrored. A west hop is a
		// translation along x exactly as an east hop is; neither axis mirrors and
		// the two never swap.
		if in.Velocity.X != 6.12 || in.Velocity.Y != 0.44 || in.Heading != 274.11 {
			t.Fatalf("%s hop mirrored the motion: %+v heading %v", edge, in.Velocity, in.Heading)
		}
		waitFor(t, 10*time.Second, "the "+edge+" hop to complete", func() bool {
			return custodyOf(origin.side, migrationID) == "out/done"
		})
		if got := journalEntry(t, origin.side, migrationID).Entry.DestSlot; got != want[edge] {
			t.Fatalf("%s hop routed to slot %d, want %d", edge, got, want[edge])
		}
		// Exactly-once still holds across all four doors: one copy on the map.
		alive := 0
		for _, nd := range g.nodes {
			if nd.world.isAlive(testEntityID) {
				alive++
			}
		}
		if alive != 1 {
			t.Fatalf("after the %s hop %d worlds hold the organism, want exactly 1", edge, alive)
		}
		// Hand it back so the next direction starts from the origin again.
		dest.world.mu.Lock()
		delete(dest.world.alive, testEntityID)
		dest.world.mu.Unlock()
		origin.world.put(testEntityID)
	}
}

// TestReverseLaneRoutesAroundADarkSlot is D12 applied to the lane D17 added.
// Killing the middle of a row must not stop the WEST current any more than it
// stops the east one: both lanes re-pair over the hole and both stay open.
//
// The rig is a one-row map of four, which is the smallest shape where a bypass
// is visible in both directions at once from a single vantage point.
func TestReverseLaneRoutesAroundADarkSlot(t *testing.T) {
	g := newGrid(t, 4, gridOptions{layout: layoutRow(4)})
	one, two, three := g.bySlot(1), g.bySlot(2), g.bySlot(3)

	waitLane(t, one.side, contracta.EdgeE, 2)
	waitLane(t, three.side, contracta.EdgeW, 2)

	// Hard kill, mid-flow: no close frame, no drain.
	two.mod.abort()
	_ = two.side.Close()

	// Forward AND reverse re-pair past it. This is the symmetric ripple: under
	// one-way lanes slot 3 pointed nowhere near slot 2 and was told nothing when
	// it died. It has a west lane now, that lane pointed at slot 2, and it
	// re-targets exactly as slot 1's east lane does.
	waitLane(t, one.side, contracta.EdgeE, 3)
	waitLane(t, three.side, contracta.EdgeW, 1)

	var n *contractb.Neighbour
	waitFor(t, 10*time.Second, "the west bypass to settle on peer_offline", func() bool {
		n = three.side.Neighbour(contracta.EdgeW)
		return n != nil && n.Slot == 1 && len(n.Skipped) == 1 &&
			n.Skipped[0].Slot != nil && *n.Skipped[0].Slot == 2 &&
			n.Skipped[0].Reason == contractb.SkipPeerOffline
	})
	if st := three.mod.edgeNow(contracta.EdgeW); !st.Open {
		t.Fatalf("slot 3's west edge closed with %q; D12 says re-pair, not close", st.Reason)
	}

	// And the current continues on the healed reverse lane.
	migrationID := three.mod.migrateOut(testEntityID, contracta.EdgeW, 0.5)
	waitFor(t, 15*time.Second, "the healed west lane to carry an organism", func() bool {
		return one.world.spawnCount(migrationID) == 1
	})
	if got := journalEntry(t, three.side, migrationID).Entry.DestSlot; got != 1 {
		t.Fatalf("the healed west hop was journaled for slot %d, want 1", got)
	}
}

// TestLengthTwoAxisIsATwoLaneShuttleOnTheRig is §2.1 on the exact shape M4
// runs, through the mod-facing surface rather than the grid arithmetic.
//
// On the 3x2 rig EVERY COLUMN is an axis of length 2, so for every peer
// effective(N) and effective(S) name the SAME peer — the two lanes of a
// shuttle. And they close TOGETHER: killing a column partner takes both with
// it, because there is no third slot on that axis to re-pair to, while the row
// lanes are untouched.
func TestLengthTwoAxisIsATwoLaneShuttleOnTheRig(t *testing.T) {
	g := newGrid(t, 6, gridOptions{layout: layout3x2()})

	// Every column, both ways.
	for _, col := range [][2]int{{1, 4}, {2, 5}, {3, 6}} {
		for _, pair := range [][2]int{{col[0], col[1]}, {col[1], col[0]}} {
			from, want := g.bySlot(pair[0]), pair[1]
			north := waitLane(t, from.side, contracta.EdgeN, want)
			south := waitLane(t, from.side, contracta.EdgeS, want)
			if north.Slot != south.Slot {
				t.Fatalf("slot %d: north lane is slot %d and south lane is slot %d; on an "+
					"axis of length 2 both walks name the same peer",
					pair[0], north.Slot, south.Slot)
			}
			from.mod.waitEdge(contracta.EdgeN, true, 10*time.Second)
			from.mod.waitEdge(contracta.EdgeS, true, 10*time.Second)
		}
	}

	// The kill. Slot 4 at (0,1) is slot 1's only column partner.
	four, one := g.bySlot(4), g.bySlot(1)
	four.mod.abort()
	_ = four.side.Close()

	// BOTH vertical edges close, and they settle on the same reason: no slot on
	// that axis is deliverable, and the axis had exactly one other member.
	for _, edge := range []string{contracta.EdgeN, contracta.EdgeS} {
		waitLaneClosed(t, one.side, edge)
		one.mod.waitEdgeReason(edge, false, contracta.ReasonNoPeer, 10*time.Second)
	}
	// The ROW is untouched: three slots, two still deliverable, both directions
	// open. The two axes close independently, which is the point of one
	// EDGE_STATUS entry per edge.
	for _, edge := range []string{contracta.EdgeE, contracta.EdgeW} {
		if st := one.mod.edgeNow(edge); !st.Open {
			t.Fatalf("slot 1's %s edge closed with %q; only the degenerate axis closes",
				edge, st.Reason)
		}
	}
	// An export through a closed shuttle lane is refused, and the organism stays
	// home rather than being journaled to nowhere.
	migrationID := one.mod.migrateOut(testEntityID, contracta.EdgeS, 0.5)
	nack := decodeAs[contracta.MigrateOutNack](t,
		one.mod.waitType(contracta.TypeMigrateOutNack, 5*time.Second))
	if nack.MigrationID != migrationID || nack.Code != contracta.OutEdgeClosed {
		t.Fatalf("exporting through the closed south lane gave %+v, want EDGE_CLOSED", nack)
	}
	if custodyOf(one.side, migrationID) != "absent" {
		t.Fatal("a refused migration must not be journaled")
	}
}

// TestClosedEdgeStillAcceptsInbound is §5.4's MUST, made explicit by A38:
//
//	an `open: false` edge refuses MIGRATE_OUT THROUGH it and NEVER refuses
//	MIGRATE_IN ON it.
//
// The rig makes the two facts collide on one edge. Slot 3 sits at (2,0) in a
// one-row map of three and declares only its WEST edge, so its EAST edge is
// undeclared and closed — and slot 2's east lane points straight at it. The
// arrival must land, be journaled and be spawned, with the closed edge saying
// nothing about it.
func TestClosedEdgeStillAcceptsInbound(t *testing.T) {
	g := newGrid(t, 3, gridOptions{layout: layoutRow(3)})
	// Re-dial slot 3's mod with a single declared export edge. borderEdges stays
	// all four, which is what it has meant since A18: the edges on which this mod
	// will accept an inbound organism.
	three := g.bySlot(3)
	three.mod.close()
	slot := 3
	three.mod = dialFakeMod(t, fakeModOptions{
		url: three.side.URL(), world: three.world, ringSlot: &slot,
		exportEdges: []string{contracta.EdgeW}, heartbeat: 200 * time.Millisecond})

	two := g.bySlot(2)
	waitLane(t, two.side, contracta.EdgeE, 3)
	two.mod.waitEdge(contracta.EdgeE, true, 10*time.Second)

	// Slot 3 reports exactly one edge, and E is not among them — so E is closed
	// for export, by A18's rule that the sidecar MUST NOT open an undeclared edge.
	three.mod.waitEdge(contracta.EdgeW, true, 10*time.Second)
	st := three.mod.lastEdgeStatus()
	if len(st.Edges) != 1 || st.Edges[0].Edge != contracta.EdgeW {
		t.Fatalf("slot 3's EDGE_STATUS is %+v, want one entry for W", st.Edges)
	}
	refused := three.mod.migrateOut(-31001, contracta.EdgeE, 0.4)
	nack := decodeAs[contracta.MigrateOutNack](t,
		three.mod.waitType(contracta.TypeMigrateOutNack, 5*time.Second))
	if nack.MigrationID != refused || nack.Code != contracta.OutEdgeClosed {
		t.Fatalf("exporting through the undeclared east edge gave %+v, want EDGE_CLOSED", nack)
	}

	// AND YET THE ARRIVAL LANDS. Slot 2 exports east; the organism enters slot 3
	// on E, the very edge that just refused an export.
	before := three.mod.frameCount()
	migrationID := two.mod.migrateOut(-31002, contracta.EdgeE, 0.5)
	env, _ := three.mod.waitFrom(before, 15*time.Second,
		func(e wire.Envelope) bool { return e.Type == contracta.TypeMigrateIn })
	in := decodeAs[contracta.MigrateIn](t, env)
	if in.MigrationID != migrationID {
		t.Fatalf("slot 3 was delivered %s, want %s", in.MigrationID, migrationID)
	}
	if in.EntryEdge != contracta.EdgeW {
		// Slot 2 left by E, so the facing entry edge is W. The point stands either
		// way: no edge state was consulted.
		t.Fatalf("the arrival came in on %s, want the facing edge W", in.EntryEdge)
	}
	waitFor(t, 10*time.Second, "the arrival to be spawned", func() bool {
		return three.world.spawnCount(migrationID) == 1
	})

	// And once more on the edge that is genuinely closed by ROUTE-AROUND rather
	// than by declaration: slot 1's east lane also reaches slot 3 when slot 2 is
	// bypassed, and nothing about slot 3's own closed east edge gates it.
	if st := three.mod.edgeNow(contracta.EdgeW); !st.Open {
		t.Fatalf("slot 3's declared west edge is closed with %q on a full row of three",
			st.Reason)
	}
}

// TestInboundRateKnob covers contract-a.md §18, A40: the delivery rate limit
// rises off the value A20 sized from one-way lanes, and it GAINS THE KNOB IT
// NEVER HAD — a compiled Go constant reachable only by editing source is not a
// tunable, and this one has now needed retuning three times.
//
// The default is 100.0 with a burst of 50, raised from A40's 12.0/15 by the
// owner on 2026-08-07: A40's 12.0 was five times a PROJECTED two-way median,
// and the running two-way deployment still carried a residual paced backlog
// under it. 100.0 is sized from the ceiling instead of from a median — two
// orders below A29's ~12 000 per simulated minute ingest budget.
//
// It pins the default, the flag and the environment variable, and it pins that
// the Config path a test rig uses still overrides all three.
func TestInboundRateKnob(t *testing.T) {
	d := DefaultConfig()
	if d.InboundRatePerSimMinute != 100.0 {
		t.Fatalf("inboundRatePerSimMinute defaults to %v, want 100.0 — two orders below "+
			"A29's ingest ceiling (§18, A40)", d.InboundRatePerSimMinute)
	}
	if d.InboundRateBurst != 50.0 {
		t.Fatalf("inboundRateBurst defaults to %v, want 50 — scaled with the rate but "+
			"under inboundQueueMax, so a full paced queue never releases in one breath",
			d.InboundRateBurst)
	}
	if d.InboundRateBurst >= contracta.InboundQueueMax {
		t.Fatalf("inboundRateBurst %v is not under inboundQueueMax %v; the bucket could "+
			"release a full paced queue in one breath",
			d.InboundRateBurst, contracta.InboundQueueMax)
	}
	if contracta.InboundRatePerSimMinute != 100.0 || contracta.InboundRateBurst != 50.0 {
		t.Fatalf("the contract package still names %v/%v",
			contracta.InboundRatePerSimMinute, contracta.InboundRateBurst)
	}

	// The flag. --inbound-rate is parsed by the same Main the shipped binary
	// runs, so this exercises the real path and not a lookalike.
	if got := parseInboundRate(t, []string{"--inbound-rate", "3.5"}); got != 3.5 {
		t.Fatalf("--inbound-rate 3.5 produced %v", got)
	}
	// The environment variable, and the flag winning over it.
	t.Setenv("MULTIVERSE_INBOUND_RATE", "7.25")
	if got := parseInboundRate(t, nil); got != 7.25 {
		t.Fatalf("MULTIVERSE_INBOUND_RATE=7.25 produced %v", got)
	}
	if got := parseInboundRate(t, []string{"--inbound-rate", "3.5"}); got != 3.5 {
		t.Fatalf("the flag did not win over the environment: %v", got)
	}
	// A value that does not parse is IGNORED rather than fatal: a typo in a
	// service-manager unit must not stop a world joining the map.
	t.Setenv("MULTIVERSE_INBOUND_RATE", "a hundred")
	if got := parseInboundRate(t, nil); got != 100.0 {
		t.Fatalf("an unparseable MULTIVERSE_INBOUND_RATE produced %v, want the default", got)
	}
	t.Setenv("MULTIVERSE_INBOUND_RATE", "-4")
	if got := parseInboundRate(t, nil); got != 100.0 {
		t.Fatalf("a negative MULTIVERSE_INBOUND_RATE produced %v, want the default", got)
	}

	// And the pacer the sidecar builds carries whatever landed in the Config,
	// which is the path every rig and every test uses.
	p := newPacer(3.0, 4)
	if p.ratePerSimMinute != 3.0 || p.burst != 4 {
		t.Fatalf("the pacer was built with %v/%v", p.ratePerSimMinute, p.burst)
	}
}

// parseInboundRate runs the same flag-then-environment resolution the shipped
// Main runs and reports the rate the Config would carry. It stops before New so
// nothing binds a port.
func parseInboundRate(t *testing.T, args []string) float64 {
	t.Helper()
	rate := envFloat("MULTIVERSE_INBOUND_RATE", 0)
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--inbound-rate" || args[i] == "-inbound-rate" {
			v, err := strconv.ParseFloat(args[i+1], 64)
			if err != nil {
				t.Fatalf("bad test arg %q: %v", args[i+1], err)
			}
			rate = v
		}
	}
	cfg := DefaultConfig()
	if rate > 0 {
		cfg.InboundRatePerSimMinute = rate
	}
	return cfg.InboundRatePerSimMinute
}

// TestSidecarMainAcceptsTheInboundRateFlag runs the SHIPPED entry point far
// enough to prove --inbound-rate is a real flag on it. An unknown flag makes
// flag.ContinueOnError return 2 before anything is dialled, so a rate that
// parses and a bogus flag that does not are distinguishable without a rig.
func TestSidecarMainAcceptsTheInboundRateFlag(t *testing.T) {
	dir := t.TempDir()
	base := []string{"--data-dir", dir, "--list-inflight"}
	if code := Main(append(base, "--inbound-rate", "9"), os.Stdout, os.Stderr); code != 0 {
		t.Fatalf("--inbound-rate 9 exited %d; the flag is not on the shipped binary", code)
	}
	if code := Main(append(base, "--inbound-rat", "9"), os.Stdout, os.Stderr); code == 0 {
		t.Fatal("a misspelled flag was accepted; the flag set is not the one being tested")
	}
}

// TestHopFeedRecordsBothDirectionsThroughTheRig is the archive half of §17 B14
// with a real relay, real sidecars and real crossings behind it.
//
// It is the assertion the archive's own unit tests cannot make: the edge the
// page animates along is the edge the organism ACTUALLY left by, carried
// unbroken from MIGRATE_OUT through the journal, the relay's copy and into the
// feed. And it pins the pair that made the ledger's lane key ambiguous — two
// hops between the same two worlds, told apart only by the edge.
func TestHopFeedRecordsBothDirectionsThroughTheRig(t *testing.T) {
	g := newGrid(t, 3, gridOptions{layout: layoutRow(3)})
	arch := startArchive(t, g.relay)
	waitFor(t, 10*time.Second, "the archive to subscribe", func() bool {
		return arch.StatusView().RelayConnected
	})

	one, two, three := g.bySlot(1), g.bySlot(2), g.bySlot(3)
	waitLane(t, one.side, contracta.EdgeE, 2)
	waitLane(t, three.side, contracta.EdgeW, 2)

	// One hop each way, and only one of them carries a species block — because
	// "absent is absent" is what the neutral glyph is drawn from.
	east := one.mod.migrateOutSpecies(-41001, contracta.EdgeE, 0.5,
		&contracta.Species{GenericName: "Izus", SpecificName: "copedylanus"})
	west := three.mod.migrateOut(-41002, contracta.EdgeW, 0.5)
	waitFor(t, 15*time.Second, "both hops to land", func() bool {
		return two.world.spawnCount(east) == 1 && two.world.spawnCount(west) == 1
	})

	var feed archiveHops
	waitFor(t, 10*time.Second, "the archive to record both crossings", func() bool {
		feed = collectHops(arch)
		return len(feed) == 2
	})
	if h := feed[east]; h.ExitEdge != contracta.EdgeE || h.FromSlot != 1 || h.ToSlot != 2 {
		t.Fatalf("the east hop is recorded as %+v, want slot 1 -> slot 2 by E", h)
	}
	if h := feed[west]; h.ExitEdge != contracta.EdgeW || h.FromSlot != 3 || h.ToSlot != 2 {
		t.Fatalf("the west hop is recorded as %+v, want slot 3 -> slot 2 by W", h)
	}
	if sp := feed[east].Species; sp == nil || sp.SpecificName != "copedylanus" {
		t.Fatalf("the east hop's species block is %+v; it is recorded, never resolved", sp)
	}
	if sp := feed[west].Species; sp != nil {
		t.Fatalf("a hop with no species block reported %+v; absent is absent", sp)
	}
}

type archiveHops map[string]archive.Hop

func collectHops(a *archive.Archive) archiveHops {
	out := archiveHops{}
	for _, h := range a.HopFeedView().Hops {
		out[h.MigrationID] = h
	}
	return out
}
