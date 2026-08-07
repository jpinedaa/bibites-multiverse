package sidecar

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"multiverse/internal/bb8"
	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/wire"
)

const testEntityID int32 = -843827577

func decodeAs[T any](t *testing.T, env wire.Envelope) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(env.Data, &v); err != nil {
		t.Fatalf("decode %s: %v", env.Type, err)
	}
	return v
}

// --------------------------------------------------------------------- (a)
//
// The contract tests carried from M2 and M3 run on a ONE-ROW map, which §2
// keeps as the height-1 specialization of the grid: every M3 rule survives
// there unchanged, and the north edge stays closed with no_peer for the life of
// the map (§2.1).

// TestHappyPath is contract test (a): slot 1 exports an organism east, slot 2's
// mod receives MIGRATE_IN and ACKs it, sidecar 1 gets MIGRATION_ACK and clears
// its journal to a tombstone.
func TestHappyPath(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.6031925)

	ackEnv := a.mod.waitType(contracta.TypeMigrateOutAck, 5*time.Second)
	ack := decodeAs[contracta.MigrateOutAck](t, ackEnv)
	if ack.MigrationID != migrationID {
		t.Fatalf("MIGRATE_OUT_ACK migrationId = %s, want %s", ack.MigrationID, migrationID)
	}
	if ack.EntityID != testEntityID {
		t.Fatalf("MIGRATE_OUT_ACK entityId = %d, want %d", ack.EntityID, testEntityID)
	}
	if ack.Unsolicited {
		t.Fatal("MIGRATE_OUT_ACK should be solicited here")
	}

	inEnv := b.mod.waitType(contracta.TypeMigrateIn, 5*time.Second)
	in := decodeAs[contracta.MigrateIn](t, inEnv)
	if in.MigrationID != migrationID {
		t.Fatalf("MIGRATE_IN migrationId = %s, want %s", in.MigrationID, migrationID)
	}
	if in.EntityID != testEntityID {
		t.Fatalf("MIGRATE_IN entityId = %d, want %d", in.EntityID, testEntityID)
	}
	// §6.6: the entry edge is DERIVED from the sender's exitEdge — W off an east
	// lane — and the wire never carries it.
	if in.EntryEdge != contracta.EdgeW {
		t.Fatalf("entryEdge = %s, want W", in.EntryEdge)
	}
	if in.EntryPosition != 0.6031925 {
		t.Fatalf("entryPosition = %v, want 0.6031925", in.EntryPosition)
	}
	// contract-a.md §4.4: velocity and heading are copied, never mirrored.
	if in.Velocity.X != 6.12 || in.Velocity.Y != 0.44 {
		t.Fatalf("velocity = %+v, want {6.12 0.44}", in.Velocity)
	}
	if in.Heading != 274.11 {
		t.Fatalf("heading = %v, want 274.11", in.Heading)
	}
	if in.BounceBack {
		t.Fatal("bounceBack should be false on a normal delivery")
	}
	if in.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1", in.Attempt)
	}
	// D4: the blob crossed the whole chain untouched.
	if in.Payload != makePayload(testEntityID) {
		t.Fatalf("payload changed in flight:\n got %s\nwant %s", in.Payload, makePayload(testEntityID))
	}

	waitFor(t, 5*time.Second, "the organism to be alive in slot 2", func() bool {
		return b.world.spawnCount(migrationID) == 1 && b.world.isAlive(testEntityID)
	})
	waitFor(t, 5*time.Second, "sidecar 1's journal entry to become a tombstone", func() bool {
		return custodyOf(a.side, migrationID) == "out/done"
	})
	waitFor(t, 5*time.Second, "sidecar 2's journal entry to become a tombstone", func() bool {
		return custodyOf(b.side, migrationID) == "in/done"
	})
	// §9.2: the completed entry ends in the done handoff state.
	if got := handoffOf(a.side, migrationID); got != journal.HandoffDone {
		t.Fatalf("handoff = %q after MIGRATION_ACK, want done", got)
	}

	if got := a.world.destroyCount(migrationID); got != 1 {
		t.Fatalf("slot 1 destroyed the organism %d times, want 1", got)
	}
	if a.world.isAlive(testEntityID) {
		t.Fatal("the organism is still alive in slot 1 after custody transferred")
	}
	if got := a.world.spawnCount(migrationID) + b.world.spawnCount(migrationID); got != 1 {
		t.Fatalf("the organism exists %d times across both sims, want exactly 1", got)
	}
}

// TestRowCircumnavigation is M3's ring circuit, kept as the height-1 case: one
// organism travels slot 1 -> 2 -> 3 -> 1 eastward, the blob is byte-identical
// at every hop, and every hop's annex carries the genome hash the canonical
// projection produces for that blob (§2, §6.6).
func TestRowCircumnavigation(t *testing.T) {
	g := newGrid(t, 3, gridOptions{layout: layoutRow(3)})

	waitLane(t, g.node(0).side, contracta.EdgeE, 2)
	waitLane(t, g.node(1).side, contracta.EdgeE, 3)
	waitLane(t, g.node(2).side, contracta.EdgeE, 1)
	// §2.1: a one-row map has NO north lanes at all, and the north edge stays
	// closed with no_peer for the life of the map.
	for i, nd := range g.nodes {
		if n := nd.side.Neighbour(contracta.EdgeN); n != nil {
			t.Fatalf("slot %d got a north lane on a one-row map: %+v", i+1, n)
		}
		st := nd.mod.waitEdge(contracta.EdgeN, false, 10*time.Second)
		if st.Reason != contracta.ReasonNoPeer {
			t.Fatalf("slot %d north edge closed with %q, want no_peer", i+1, st.Reason)
		}
	}

	payload := makePayload(testEntityID)
	wantHash := genomeHashOf(t, payload)

	hops := []struct {
		from, to  int
		wantDest  int
		migration string
	}{
		{from: 0, to: 1, wantDest: 2},
		{from: 1, to: 2, wantDest: 3},
		{from: 2, to: 0, wantDest: 1},
	}
	for i := range hops {
		hop := &hops[i]
		src, dst := g.node(hop.from), g.node(hop.to)
		before := dst.mod.frameCount()

		hop.migration = src.mod.migrateOutPayload(wire.NewUUID(), testEntityID,
			contracta.EdgeE, 0.5, payload)

		env, _ := dst.mod.waitFrom(before, 10*time.Second,
			func(e wire.Envelope) bool { return e.Type == contracta.TypeMigrateIn })
		in := decodeAs[contracta.MigrateIn](t, env)
		if in.MigrationID != hop.migration {
			t.Fatalf("hop %d delivered migrationId %s, want %s", i+1, in.MigrationID, hop.migration)
		}
		if in.Payload != payload {
			t.Fatalf("hop %d changed the blob:\n got %s\nwant %s", i+1, in.Payload, payload)
		}
		if in.EntryEdge != contracta.EdgeW {
			t.Fatalf("hop %d entryEdge = %s, want the passive west edge", i+1, in.EntryEdge)
		}

		waitFor(t, 10*time.Second, "the hop to complete", func() bool {
			return custodyOf(src.side, hop.migration) == "out/done" &&
				custodyOf(dst.side, hop.migration) == "in/done"
		})

		sent := journalEntry(t, src.side, hop.migration)
		if sent.Entry.GenomeHash != wantHash {
			t.Fatalf("hop %d source annex genomeHash = %q, want %q", i+1, sent.Entry.GenomeHash, wantHash)
		}
		if sent.Entry.DestSlot != hop.wantDest {
			t.Fatalf("hop %d routed to slot %d, want %d", i+1, sent.Entry.DestSlot, hop.wantDest)
		}
		got := journalEntry(t, dst.side, hop.migration)
		if got.Entry.GenomeHash != wantHash {
			t.Fatalf("hop %d receiver annex genomeHash = %q, want %q", i+1, got.Entry.GenomeHash, wantHash)
		}
		if !dst.side.Genomes().Has(wantHash) {
			t.Fatalf("hop %d receiver did not cache the migrant's genome", i+1)
		}
		total := 0
		for _, nd := range g.nodes {
			if nd.world.isAlive(testEntityID) {
				total++
			}
		}
		if total != 1 {
			t.Fatalf("after hop %d the organism is alive in %d sims, want exactly 1", i+1, total)
		}
	}

	if !g.node(0).world.isAlive(testEntityID) {
		t.Fatal("the organism did not come back round the row to slot 1")
	}
}

// TestOfflineSlotKeepsItsReservationAndItsPosition covers D8 and §7.2 rule 1: a
// slot belongs to a peer IDENTITY, not to a connection. Its reservation and its
// coordinate survive the outage, and both come back to the same peerId.
func TestOfflineSlotKeepsItsReservationAndItsPosition(t *testing.T) {
	g := newGrid(t, 3, gridOptions{layout: layoutRow(3)})
	one, two := g.node(0), g.node(1)
	waitLane(t, one.side, contracta.EdgeE, 2)

	two.mod.abort()
	_ = two.side.Close()

	// M4, and this is the whole of D12: the lane RE-PAIRS instead of closing.
	waitLane(t, one.side, contracta.EdgeE, 3)
	if st := one.mod.edgeNow(contracta.EdgeE); !st.Open {
		t.Fatalf("slot 1's east edge closed with %q; route-around must keep it open", st.Reason)
	}
	// The reservation survives, with its position.
	res := g.relay.relay.Snapshot()
	if len(res) != 3 || res[1].Slot != 2 || res[1].PeerID != two.cfg.PeerID ||
		res[1].Col != 1 || res[1].Row != 0 {
		t.Fatalf("map after the peer left is %v; a reservation never expires and never moves", res)
	}

	// The same peerId and the same data dir come back and reclaim slot 2 at (1,0).
	revived := startSidecar(t, two.cfg)
	waitSlot(t, revived, 2)
	if pos := revived.Position(); pos.Col != 1 || pos.Row != 0 {
		t.Fatalf("the returning peer landed at %+v, want (1,0)", pos)
	}
	revivedMod := dialFakeMod(t, fakeModOptions{
		url: revived.URL(), world: two.world, heartbeat: 200 * time.Millisecond})

	// Both lanes re-pair back to it as a liveness event, with no operator action.
	waitLane(t, one.side, contracta.EdgeE, 2)
	revivedMod.waitEdge(contracta.EdgeE, true, 10*time.Second)
	if got := len(g.relay.relay.Snapshot()); got != 3 {
		t.Fatalf("the map grew to %d slots on a reclaim; the peer must not take a second slot", got)
	}

	migrationID := one.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "a migration into the returned slot", func() bool {
		return two.world.spawnCount(migrationID) == 1
	})
}

// TestArchiveRecordsAndFetchesAGenome covers §5.1 and §10: the archive is
// copied every routed envelope, records it with its annex, and fetches by hash
// a parent genome that never travelled the wire.
func TestArchiveRecordsAndFetchesAGenome(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	one, two := g.node(0), g.node(1)
	arc := startArchive(t, g.relay.url())

	const livingParent int32 = -1180911975
	const goneParent int32 = 204418833
	parentBlob := makePayload(livingParent)
	parentHash := genomeHashOf(t, parentBlob)
	migrantHash := genomeHashOf(t, makePayload(testEntityID))

	if arc.Genomes().Has(parentHash) {
		t.Fatal("the archive already holds the parent genome; the test proves nothing")
	}

	living, gone := livingParent, goneParent
	migrationID := one.mod.migrateOutParents(testEntityID, contracta.EdgeE, 0.5,
		[]contracta.ParentBlob{
			{EntityID: &living, Payload: parentBlob, GameVersion: "0.6.3.1"},
			{EntityID: &gone}, // BibiteGenes dropped the parentage: a normal gap
		})
	waitFor(t, 10*time.Second, "the migration to complete", func() bool {
		return two.world.spawnCount(migrationID) == 1
	})

	// The wire never carried the parent blob, but the source sidecar cached it.
	if !one.side.Genomes().Has(parentHash) {
		t.Fatal("the source sidecar did not cache the parent blob it was shipped")
	}
	if two.side.Genomes().Has(parentHash) {
		t.Fatal("the parent blob reached the destination; §6.6 says it is stripped")
	}

	waitFor(t, 10*time.Second, "the archive to record the migration", func() bool {
		list, err := arc.Records()
		if err != nil {
			return false
		}
		for _, rec := range list {
			if rec.MigrationID == migrationID && rec.Lineage != nil {
				return true
			}
		}
		return false
	})
	list, err := arc.Records()
	if err != nil {
		t.Fatalf("archive records: %v", err)
	}
	var found bool
	for _, rec := range list {
		if rec.MigrationID != migrationID || rec.Lineage == nil {
			continue
		}
		found = true
		if rec.Lineage.GenomeHash != migrantHash {
			t.Fatalf("archive recorded genomeHash %q, want %q", rec.Lineage.GenomeHash, migrantHash)
		}
		if rec.SourceSlot != 1 || rec.DestSlot != 2 {
			t.Fatalf("archive recorded slots %d -> %d, want 1 -> 2", rec.SourceSlot, rec.DestSlot)
		}
		if len(rec.Lineage.Parents) != 2 {
			t.Fatalf("archive recorded %d parents, want 2", len(rec.Lineage.Parents))
		}
		if rec.Lineage.Parents[0].GenomeHash != parentHash {
			t.Fatalf("parent 1 hash = %q, want %q", rec.Lineage.Parents[0].GenomeHash, parentHash)
		}
		if rec.Lineage.Parents[1].GenomeHash != "" || rec.Lineage.Parents[1].GapReason != "parent_gone" {
			t.Fatalf("parent 2 should be a parent_gone gap, got %+v", rec.Lineage.Parents[1])
		}
	}
	if !found {
		t.Fatal("the archive has no record for the migration")
	}

	waitFor(t, 15*time.Second, "the archive to fetch the migrant genome by hash", func() bool {
		return arc.Genomes().Has(migrantHash)
	})
	waitFor(t, 15*time.Second, "the archive to fetch the parent genome by hash", func() bool {
		return arc.Genomes().Has(parentHash)
	})
	e, ok := arc.Genomes().Get(parentHash)
	if !ok {
		t.Fatal("the parent genome is not in the archive store")
	}
	if e.BB8 != parentBlob {
		t.Fatal("the archive stored bytes that are not the parent blob")
	}

	// The ACK record is a second ledger line and it lands after the spawn, so the
	// read path is polled rather than sampled once.
	waitFor(t, 10*time.Second, "the read path to show the migration as delivered", func() bool {
		list, err := arc.List()
		if err != nil {
			return false
		}
		for _, m := range list {
			if m.MigrationID == migrationID && strings.HasPrefix(m.Outcome, "delivered") {
				return true
			}
		}
		return false
	})
	migrations, err := arc.List()
	if err != nil {
		t.Fatalf("archive list: %v", err)
	}
	var shown bool
	for _, m := range migrations {
		if m.MigrationID != migrationID {
			continue
		}
		shown = true
		if !m.GenomeHeld {
			t.Fatal("the read path reports the migrant genome as missing")
		}
		if len(m.ParentsHeld) != 2 || !m.ParentsHeld[0] {
			t.Fatalf("the read path reports parents held = %v", m.ParentsHeld)
		}
		if !strings.HasPrefix(m.Outcome, "delivered") {
			t.Fatalf("the read path reports outcome %q, want a delivered outcome", m.Outcome)
		}
	}
	if !shown {
		t.Fatal("the read path did not list the migration")
	}
}

// TestArchiveMayNotSendMigrations covers §5.1: a MIGRATION_PAYLOAD from a
// subscriber is answered NOT_A_MEMBER and is not forwarded.
func TestArchiveMayNotSendMigrations(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	sub := dialSubscriber(t, g.relay.url(), testToken)

	nack := sub.sendMigrationAndWaitNack(t, 10*time.Second)
	if nack.Code != "NOT_A_MEMBER" {
		t.Fatalf("NACK code = %s, want NOT_A_MEMBER", nack.Code)
	}
	if nack.Class != "permanent" {
		t.Fatalf("NACK class = %s, want permanent", nack.Class)
	}
	time.Sleep(300 * time.Millisecond)
	for i, nd := range g.nodes {
		if len(nd.side.CustodySnapshot()) != 0 {
			t.Fatalf("slot %d journaled something a subscriber sent", i+1)
		}
	}
	grant := sub.claimAndWaitGrant(t, 10*time.Second)
	if grant.Granted || grant.Reason != "role_has_no_slot" {
		t.Fatalf("subscriber claim returned %+v, want granted=false role_has_no_slot", grant)
	}
}

// TestWrongLANTokenIsRejected covers §3.1: a missing or wrong token gets HTTP
// 401 and no upgrade, so there is no WebSocket and no close code, and the peer
// never joins the map.
func TestWrongLANTokenIsRejected(t *testing.T) {
	rl := startRelayToken(t, testToken)

	for name, token := range map[string]string{
		"wrong token": "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		"no token":    "",
	} {
		t.Run(name, func(t *testing.T) {
			status, authenticate := probeRelay(t, rl.addr, token)
			if status != 401 {
				t.Fatalf("HTTP status = %d, want 401", status)
			}
			if authenticate != "Bearer" {
				t.Fatalf("WWW-Authenticate = %q, want Bearer", authenticate)
			}
		})
	}

	cfg := fastConfig(t, rl.url(), "peer-with-a-bad-token")
	cfg.Token = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	s := startSidecar(t, cfg)
	dialFakeMod(t, fakeModOptions{url: s.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond})
	time.Sleep(1500 * time.Millisecond)
	if got := s.Slot(); got != 0 {
		t.Fatalf("a sidecar with a wrong token got slot %d", got)
	}
	if got := len(rl.relay.Snapshot()); got != 0 {
		t.Fatalf("the map has %d slots; an unauthenticated peer must never reserve one", got)
	}

	good := fastConfig(t, rl.url(), "peer-with-the-right-token")
	goodSide := startSidecar(t, good)
	waitSlot(t, goodSide, 1)
}

// TestEdgeStatusCarriesOneEntryPerExportEdge covers contract-a.md §15 A18. It
// REPLACES M3's TestExportEdgeFallbackAndNarrowing, whose subject — a
// single-entry EDGE_STATUS and the single-entry-borderEdges fallback for an
// absent exportEdge — A18 removed outright: "a compatibility path that only an
// already-rejected peer can take is dead code that reads like a supported
// configuration".
// Under D17 (contract-a.md §18, A38) a conformant mod declares FOUR, and the
// count is the whole of A38 on this wire: no field changed, no type changed,
// no enum moved — only which values a conformant mod puts in a field that has
// accepted them since A18.
func TestEdgeStatusCarriesOneEntryPerExportEdge(t *testing.T) {
	g := newGrid(t, 6, gridOptions{layout: layout3x2()})

	st := g.node(0).mod.lastEdgeStatus()
	if len(st.Edges) != 4 {
		t.Fatalf("EDGE_STATUS carried %d entries, want one per declared export edge", len(st.Edges))
	}
	seen := map[string]bool{}
	for _, e := range st.Edges {
		seen[e.Edge] = true
	}
	for _, want := range contracta.CanonicalEdges() {
		if !seen[want] {
			t.Fatalf("EDGE_STATUS reported edges %+v, want all four", st.Edges)
		}
	}
	// Each edge opens and closes independently, and on a full 3x2 all four are
	// open.
	for _, edge := range contracta.CanonicalEdges() {
		g.node(0).mod.waitEdge(edge, true, 10*time.Second)
	}

	// A18 removed the fallback: a CONFIG_UPDATE with NO exportEdges is
	// unusable, and CONFIG_UPDATE has no NACK channel, so the only answer is a
	// close.
	orphan := startSidecar(t, fastConfig(t, g.relay.url(), "peer-no-export-edges"))
	bad := dialRawMod(t, orphan.URL())
	size, width := 2000.0, 60.0
	frame, err := wire.Encode(wire.ProtocolA, contracta.TypeConfigUpdate, time.Now().UnixMilli(),
		contracta.ConfigUpdate{
			SessionID: wire.NewUUID(), Reason: "connect", GameVersion: "0.6.3.1",
			ModVersion: "0.2.0", SimulationSize: &size,
			BorderEdges: []string{contracta.EdgeE, contracta.EdgeW}, BorderWidth: &width,
		})
	if err != nil {
		t.Fatal(err)
	}
	bad.write(frame)
	if code := bad.waitClosed(5 * time.Second); code != contracta.CloseMalformedFrame {
		t.Fatalf("close code = %d, want %d (MALFORMED_FRAME)", code, contracta.CloseMalformedFrame)
	}

	// An exportEdges member that is not also a borderEdge is equally unusable.
	orphan2 := startSidecar(t, fastConfig(t, g.relay.url(), "peer-undeclared-band"))
	bad2 := dialRawMod(t, orphan2.URL())
	frame, err = wire.Encode(wire.ProtocolA, contracta.TypeConfigUpdate, time.Now().UnixMilli(),
		contracta.ConfigUpdate{
			SessionID: wire.NewUUID(), Reason: "connect", GameVersion: "0.6.3.1",
			ModVersion: "0.2.0", SimulationSize: &size,
			BorderEdges: []string{contracta.EdgeW}, ExportEdges: []string{contracta.EdgeE},
			BorderWidth: &width,
		})
	if err != nil {
		t.Fatal(err)
	}
	bad2.write(frame)
	if code := bad2.waitClosed(5 * time.Second); code != contracta.CloseMalformedFrame {
		t.Fatalf("close code = %d, want %d", code, contracta.CloseMalformedFrame)
	}
}

// TestRetiredSectorFieldIsIgnored covers contract-a.md §14 A14, still true at
// contract-a/2.0: the sidecar ignores the retired {x,y} sector an older mod
// sends and MUST NOT close on it.
func TestRetiredSectorFieldIsIgnored(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	side := startSidecar(t, fastConfig(t, g.relay.url(), "peer-with-a-retired-sector"))
	waitSlotAny(t, side)

	old := dialRawMod(t, side.URL())
	size, width := 2000.0, 60.0
	body := map[string]any{
		"sessionId": wire.NewUUID(), "reason": "connect", "gameVersion": "0.6.3.1",
		"modVersion": "0.2.0", "simulationSize": size,
		"borderEdges": []string{contracta.EdgeE, contracta.EdgeW},
		"exportEdges": []string{contracta.EdgeE}, "borderWidth": width,
		// The retired field, exactly as a contract-a/1 mod would send it.
		"sector": map[string]int{"x": 0, "y": 0},
	}
	frame, err := wire.Encode(wire.ProtocolA, contracta.TypeConfigUpdate, time.Now().UnixMilli(), body)
	if err != nil {
		t.Fatal(err)
	}
	old.write(frame)
	time.Sleep(500 * time.Millisecond)
	if old.closedNow() {
		t.Fatal("the sidecar closed on a retired sector field; A14 says ignore it")
	}
}

// TestSlotMismatchClosesWith4001 covers contract-a.md §14 A14: ringSlot is
// advisory, and a disagreement is a mis-wired rig worth one second of
// diagnosis.
func TestSlotMismatchClosesWith4001(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2), noMods: true})
	wrong := 99
	m := dialFakeMod(t, fakeModOptions{
		url: g.node(0).side.URL(), world: newWorld(), ringSlot: &wrong,
		heartbeat: 200 * time.Millisecond})
	if code := m.waitClosed(5 * time.Second); int(code) != contracta.CloseSlotMismatch {
		t.Fatalf("close code = %d, want %d (SLOT_MISMATCH)", code, contracta.CloseSlotMismatch)
	}
}

// TestPeerModAbsentClosesADegenerateAxis is M3's TestPeerModAbsentClosesTheWestLane,
// REWRITTEN for D12. A sidecar that loses its mod is no longer DELIVERABLE, and
// under M4 that is a SKIP reason rather than a close reason — so on an axis with
// a third slot the lane re-pairs. This test therefore uses the one shape where
// the closure survives: an axis of two, where §2.1 says there is nothing to
// re-pair to, and §8's mapping turns a shared peer_mod_absent skip into exactly
// that EDGE_STATUS reason.
func TestPeerModAbsentClosesADegenerateAxis(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	one, two := g.node(0), g.node(1)
	one.mod.waitEdge(contracta.EdgeE, true, 5*time.Second)

	two.mod.close()

	closed := one.mod.waitEdgeReason(contracta.EdgeE, false, contracta.ReasonPeerModAbsent, 10*time.Second)
	if closed.Open {
		t.Fatal("the edge stayed open with no deliverable slot on the axis")
	}
}

// TestModAbsentIsRoutedAroundOnALongerAxis is the other half of the same rule,
// and it is the one D12 exists for: with a third slot on the axis the lane
// re-pairs and the current keeps flowing.
func TestModAbsentIsRoutedAroundOnALongerAxis(t *testing.T) {
	g := newGrid(t, 3, gridOptions{layout: layoutRow(3)})
	one, two := g.node(0), g.node(1)
	waitLane(t, one.side, contracta.EdgeE, 2)

	two.mod.close()

	waitLane(t, one.side, contracta.EdgeE, 3)
	if st := one.mod.edgeNow(contracta.EdgeE); !st.Open {
		t.Fatalf("slot 1's edge closed with %q; a mod-less slot is skipped, not fatal", st.Reason)
	}
	n := one.side.Neighbour(contracta.EdgeE)
	if len(n.Skipped) != 1 || n.Skipped[0].Reason != "peer_mod_absent" {
		t.Fatalf("the bypass list is %+v, want one peer_mod_absent entry", n.Skipped)
	}
	migrationID := one.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "the current to keep flowing past the mod-less slot", func() bool {
		return g.node(2).world.spawnCount(migrationID) == 1
	})
}

// --------------------------------------------------------------------- (b)

// TestReconnectReplay is contract test (b): the destination mod disconnects
// before it ACKs, reconnects with CONFIG_UPDATE, and gets EDGE_STATUS followed
// by the replayed MIGRATE_IN with attempt = 2 (contract-a.md §7.5).
func TestReconnectReplay(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	b.mod.setAckMode(ackSilent)

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.25)

	first := decodeAs[contracta.MigrateIn](t, b.mod.waitType(contracta.TypeMigrateIn, 5*time.Second))
	if first.Attempt != 1 {
		t.Fatalf("first delivery attempt = %d, want 1", first.Attempt)
	}
	if b.world.spawnCount(migrationID) != 0 {
		t.Fatal("a silent mod must not have spawned anything")
	}

	b.mod.abort()

	modB2 := dialFakeMod(t, fakeModOptions{
		url: b.side.URL(), world: b.world, heartbeat: 200 * time.Millisecond})

	firstFrame, _ := modB2.waitFrom(0, 5*time.Second, func(wire.Envelope) bool { return true })
	if firstFrame.Type != contracta.TypeEdgeStatus {
		t.Fatalf("first frame after reconnect was %s, want EDGE_STATUS", firstFrame.Type)
	}
	replayEnv, idx := modB2.waitFrom(0, 10*time.Second,
		func(e wire.Envelope) bool { return e.Type == contracta.TypeMigrateIn })
	if idx == 0 {
		t.Fatal("MIGRATE_IN arrived before EDGE_STATUS")
	}
	replay := decodeAs[contracta.MigrateIn](t, replayEnv)
	if replay.MigrationID != migrationID {
		t.Fatalf("replayed migrationId = %s, want %s", replay.MigrationID, migrationID)
	}
	if replay.Attempt != 2 {
		t.Fatalf("replay attempt = %d, want 2", replay.Attempt)
	}

	waitFor(t, 5*time.Second, "the replayed organism to land", func() bool {
		return b.world.spawnCount(migrationID) == 1
	})
	waitFor(t, 5*time.Second, "the origin's journal to clear", func() bool {
		return custodyOf(a.side, migrationID) == "out/done"
	})
	if got := a.world.spawnCount(migrationID) + b.world.spawnCount(migrationID); got != 1 {
		t.Fatalf("the organism exists %d times across both sims, want exactly 1", got)
	}
}

// --------------------------------------------------------------------- (c)

// TestDuplicateMigrateOutIsIdempotent is contract test (c).
func TestDuplicateMigrateOutIsIdempotent(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	a.mod.waitType(contracta.TypeMigrateOutAck, 5*time.Second)

	a.mod.resend(migrationID)
	a.mod.resend(migrationID)

	waitFor(t, 5*time.Second, "three MIGRATE_OUT_ACKs for one migrationId", func() bool {
		return countAcks(a.mod, migrationID) >= 3
	})
	waitFor(t, 5*time.Second, "the delivery to complete", func() bool {
		return b.world.spawnCount(migrationID) == 1
	})
	time.Sleep(500 * time.Millisecond)
	if got := b.world.spawnCount(migrationID); got != 1 {
		t.Fatalf("slot 2 spawned the organism %d times, want 1", got)
	}
	if got := countJournalEntries(a.side, migrationID); got != 1 {
		t.Fatalf("sidecar 1 has %d journal entries for one migrationId, want 1", got)
	}
	if got := countJournalEntries(b.side, migrationID); got != 1 {
		t.Fatalf("sidecar 2 has %d journal entries for one migrationId, want 1", got)
	}
	if got := a.world.destroyCount(migrationID); got != 1 {
		t.Fatalf("slot 1 destroyed the organism %d times, want 1", got)
	}

	a.mod.migrateOutPayload(migrationID, testEntityID, contracta.EdgeE, 0.5, makePayload(12345))
	nackEnv := a.mod.waitType(contracta.TypeMigrateOutNack, 5*time.Second)
	nack := decodeAs[contracta.MigrateOutNack](t, nackEnv)
	if nack.Code != contracta.OutDuplicateMigationID {
		t.Fatalf("NACK code = %s, want DUPLICATE_MIGRATION_ID", nack.Code)
	}
	if nack.Class != contracta.ClassPermanent {
		t.Fatalf("NACK class = %s, want permanent", nack.Class)
	}
}

func countAcks(m *fakeMod, migrationID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, env := range m.frames {
		if env.Type != contracta.TypeMigrateOutAck {
			continue
		}
		var ack contracta.MigrateOutAck
		if json.Unmarshal(env.Data, &ack) == nil && ack.MigrationID == migrationID {
			n++
		}
	}
	return n
}

func countJournalEntries(s *Sidecar, migrationID string) int {
	n := 0
	for _, st := range s.CustodySnapshot() {
		if st.Entry.MigrationID == migrationID {
			n++
		}
	}
	return n
}

// --------------------------------------------------------------------- (e)

// TestHeartbeatTimeout is contract test (e): a silent mod is closed with 4004
// and, on a degenerate axis, its west neighbour's export edge closes — because
// a sim with no mod cannot spawn anything (contract-a.md §8, §8).
func TestHeartbeatTimeout(t *testing.T) {
	g := newGrid(t, 2, gridOptions{
		layout: layoutRow(2),
		tune: func(i int, c *Config) {
			if i == 1 {
				c.HeartbeatTimeout = 2 * time.Second
			}
		},
	})
	one, two := g.node(0), g.node(1)
	one.mod.waitEdge(contracta.EdgeE, true, 5*time.Second)

	two.mod.stopHeartbeats()

	code := two.mod.waitClosed(8 * time.Second)
	if int(code) != contracta.CloseHeartbeatTimeout {
		t.Fatalf("close code = %d, want %d (HEARTBEAT_TIMEOUT)", code, contracta.CloseHeartbeatTimeout)
	}
	closed := one.mod.waitEdge(contracta.EdgeE, false, 10*time.Second)
	if closed.Reason == contracta.ReasonPeerLive {
		t.Fatalf("edge closed with reason %q", closed.Reason)
	}
}

// --------------------------------------------------------------------- (f)

// TestRelayDropAndRecovery is contract test (f): killing the relay closes every
// export edge and the closure reaches the mods; restarting it restores the map
// from ring.json, with the same slots AND the same positions, and migration
// works again.
func TestRelayDropAndRecovery(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	one, two := g.node(0), g.node(1)

	g.relay.kill()

	closedA := one.mod.waitEdge(contracta.EdgeE, false, 8*time.Second)
	closedB := two.mod.waitEdge(contracta.EdgeE, false, 8*time.Second)
	if closedA.Reason != contracta.ReasonPeerUnreachable {
		t.Fatalf("slot 1 edge reason = %q, want peer_unreachable", closedA.Reason)
	}
	if closedB.Reason != contracta.ReasonPeerUnreachable {
		t.Fatalf("slot 2 edge reason = %q, want peer_unreachable", closedB.Reason)
	}

	g.relay.restart()

	one.mod.waitEdge(contracta.EdgeE, true, 15*time.Second)
	two.mod.waitEdge(contracta.EdgeE, true, 15*time.Second)
	if got := one.side.Slot(); got != 1 {
		t.Fatalf("slot 1 reclaimed %d after the relay restart", got)
	}
	if got := two.side.Slot(); got != 2 {
		t.Fatalf("slot 2 reclaimed %d after the relay restart", got)
	}
	if pos := two.side.Position(); pos.Col != 1 || pos.Row != 0 {
		t.Fatalf("slot 2 came back at %+v, want (1,0); §7.4 persists the position too", pos)
	}
	if got := len(g.relay.relay.Snapshot()); got != 2 {
		t.Fatalf("the map has %d slots after a restart, want 2", got)
	}

	migrationID := one.mod.migrateOut(testEntityID, contracta.EdgeE, 0.75)
	waitFor(t, 15*time.Second, "a migration to complete after recovery", func() bool {
		return two.world.spawnCount(migrationID) == 1
	})
	if got := one.world.spawnCount(migrationID) + two.world.spawnCount(migrationID); got != 1 {
		t.Fatalf("the organism exists %d times across both sims, want exactly 1", got)
	}
}

// --------------------------------------------------------- extra contract rules

// TestBounceBackWhenRouteAroundHasNoAnswer covers §9.4 row 2 and §9.2's last
// re-route rule: an outbound entry that never reached a live peer AND has no
// lane to re-route to comes home as a MIGRATE_IN with bounceBack = true, on the
// edge it left from.
//
// The journal is seeded the way a SIGKILLed sidecar leaves it — custody taken,
// nothing forwarded — because that is the only state from which a bounce is
// reachable without a race.
func TestBounceBackWhenRouteAroundHasNoAnswer(t *testing.T) {
	relaySrv := startRelay(t)
	dataDir := t.TempDir()
	migrationID := seedOutboundCustody(t, dataDir, nil)

	cfg := fastConfig(t, relaySrv.url(), "peer-a")
	cfg.DataDir = dataDir
	sideA := startSidecar(t, cfg)
	waitSlot(t, sideA, 1)

	world := newWorld()
	modA := dialFakeMod(t, fakeModOptions{
		url: sideA.URL(), world: world, heartbeat: 200 * time.Millisecond})

	env, _ := modA.waitFrom(0, 10*time.Second,
		func(e wire.Envelope) bool { return e.Type == contracta.TypeMigrateIn })
	in := decodeAs[contracta.MigrateIn](t, env)
	if in.MigrationID != migrationID {
		t.Fatalf("bounced migrationId = %s, want %s", in.MigrationID, migrationID)
	}
	if !in.BounceBack {
		t.Fatal("a bounced delivery must carry bounceBack = true")
	}
	// §9.4: it comes home through the door it left by — the origin's own
	// exitEdge, not a passive entry edge.
	if in.EntryEdge != contracta.EdgeE {
		t.Fatalf("bounce entryEdge = %s, want the origin's own exit edge E", in.EntryEdge)
	}
	if in.EntityID != testEntityID {
		t.Fatalf("bounced entityId = %d, want %d", in.EntityID, testEntityID)
	}
	waitFor(t, 5*time.Second, "the bounced organism to be alive again in slot 1", func() bool {
		return world.spawnCount(migrationID) == 1
	})
	waitFor(t, 5*time.Second, "the bounce to be journaled as complete", func() bool {
		return custodyOf(sideA, migrationID) == "in/done"
	})
	if got := world.spawnCount(migrationID); got != 1 {
		t.Fatalf("the organism came home %d times, want exactly 1", got)
	}
}

// seedOutboundCustody writes the journal a killed sidecar would leave behind:
// one outbound organism, durably in custody, never forwarded — handoff pending.
// species rides the entry when the caller gives one, which is what lets the
// bounce-back test prove the block comes home with the organism (§16, A30).
func seedOutboundCustody(t *testing.T, dataDir string, species *contracta.Species) string {
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
		// A slot nobody holds: the relay answers SLOT_VACANT, permanently, and
		// with only one peer in the map there is no lane to re-route to.
		DestSlot:    2,
		Species:     species,
		JournaledAt: time.Now().UnixMilli(),
	}, false); err != nil {
		t.Fatalf("seed journal: %v", err)
	}
	if err := jr.Close(); err != nil {
		t.Fatalf("seed journal close: %v", err)
	}
	return migrationID
}

// TestEdgeClosedNack covers contract-a.md §9.1's EDGE_CLOSED: a migration out
// of an edge with no deliverable slot on its axis is refused and the mod keeps
// the organism.
func TestEdgeClosedNack(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	b.mod.abort()
	_ = b.side.Close()
	a.mod.waitEdge(contracta.EdgeE, false, 8*time.Second)

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.4)
	nack := decodeAs[contracta.MigrateOutNack](t, a.mod.waitType(contracta.TypeMigrateOutNack, 5*time.Second))
	if nack.MigrationID != migrationID {
		t.Fatalf("NACK migrationId = %s, want %s", nack.MigrationID, migrationID)
	}
	if nack.Code != contracta.OutEdgeClosed {
		t.Fatalf("NACK code = %s, want EDGE_CLOSED", nack.Code)
	}
	if nack.Class != contracta.ClassTransient {
		t.Fatalf("NACK class = %s, want transient", nack.Class)
	}
	if nack.RetryAfterMs == nil {
		t.Fatal("a transient NACK must carry retryAfterMs")
	}
	if custodyOf(a.side, migrationID) != "absent" {
		t.Fatal("a refused migration must not be journaled")
	}
	if !a.world.isAlive(testEntityID) {
		t.Fatal("the mod must keep an organism the sidecar refused")
	}
}

// TestNonExportEdgeIsRefused covers contract-a.md §15 A18: the sidecar MUST NOT
// open an edge the mod did not declare, whatever borderEdges says. W and S are
// declared BORDER edges here and still cannot be exported through.
//
// Under D17 that rule is unchanged and its subject is now the interesting case
// §18 A41 calls a mixed rig: a TWO-EDGE mod on a two-way map. It exports east
// and north only, and it still RECEIVES on all four, because arrivals were
// never gated by exportEdges. Legal, lossless, and visible on the page as a
// slot with two dead lanes — so this test pins the declaration explicitly
// rather than riding the rig's four-edge default.
func TestNonExportEdgeIsRefused(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2),
		exportEdges: []string{contracta.EdgeE, contracta.EdgeN}})
	for _, edge := range []string{contracta.EdgeW, contracta.EdgeS} {
		g.node(0).mod.migrateOut(testEntityID, edge, 0.4)
		nack := decodeAs[contracta.MigrateOutNack](t,
			g.node(0).mod.waitType(contracta.TypeMigrateOutNack, 5*time.Second))
		if nack.Code != contracta.OutEdgeClosed {
			t.Fatalf("exporting through %s gave NACK code %s, want EDGE_CLOSED", edge, nack.Code)
		}
	}
}

// TestInvalidPayloadNack covers contract-a.md §9.1's INVALID_PAYLOAD.
func TestInvalidPayloadNack(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	g.node(0).mod.migrateOutPayload(wire.NewUUID(), testEntityID, contracta.EdgeE, 0.4, "not json at all")
	nack := decodeAs[contracta.MigrateOutNack](t,
		g.node(0).mod.waitType(contracta.TypeMigrateOutNack, 5*time.Second))
	if nack.Code != contracta.OutInvalidPayload {
		t.Fatalf("NACK code = %s, want INVALID_PAYLOAD", nack.Code)
	}
	if nack.Class != contracta.ClassPermanent {
		t.Fatalf("NACK class = %s, want permanent", nack.Class)
	}
}

// TestUnhashableGenomeStillMigrates covers genome-hash.md §8 and §6.6: the
// annex is bookkeeping, the organism is custody.
func TestUnhashableGenomeStillMigrates(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	blob := `{"version":"0.6.3.1","body":{"id":{"id":-843827577}},"notAGenome":true}`
	migrationID := a.mod.migrateOutPayload(wire.NewUUID(), testEntityID, contracta.EdgeE, 0.4, blob)
	waitFor(t, 10*time.Second, "the unhashable organism to land anyway", func() bool {
		return b.world.spawnCount(migrationID) == 1
	})
	if got := journalEntry(t, b.side, migrationID).Entry.GenomeHash; got != "" {
		t.Fatalf("an unhashable genome produced hash %q; §8 forbids a placeholder", got)
	}
}

// TestMalformedMessageNack covers contract-a.md §9.3: a data field that fails
// validation is a NACK, not a close.
func TestMalformedMessageNack(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	g.node(0).mod.migrateOut(testEntityID, contracta.EdgeE, 1.5)
	nack := decodeAs[contracta.MigrateOutNack](t,
		g.node(0).mod.waitType(contracta.TypeMigrateOutNack, 5*time.Second))
	if nack.Code != contracta.OutMalformedMessage {
		t.Fatalf("NACK code = %s, want MALFORMED_MESSAGE", nack.Code)
	}
	if code := g.node(0).mod.closeCodeNow(); code != -1 {
		t.Fatalf("the connection was closed with %d; a data-level fault must keep it open", code)
	}
}

// TestTransientMigrateInNackKeepsCustody covers contract-a.md §9.2.
func TestTransientMigrateInNackKeepsCustody(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	b.mod.setAckMode(ackNackTransient)

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.4)
	b.mod.waitType(contracta.TypeMigrateIn, 5*time.Second)
	waitFor(t, 5*time.Second, "a re-delivery after the transient NACK", func() bool {
		return countMigrateIn(b.mod, migrationID) >= 2
	})
	if custodyOf(b.side, migrationID) != "in/in_flight" {
		t.Fatalf("sidecar 2 custody = %s, want in/in_flight", custodyOf(b.side, migrationID))
	}

	b.mod.setAckMode(ackNormal)
	waitFor(t, 10*time.Second, "the organism to land after the mod recovers", func() bool {
		return b.world.spawnCount(migrationID) == 1
	})
}

// TestPermanentMigrateInNackHoldsCustody covers §9.4's mirror-image rule: a
// permanently rejected inbound organism is held in the journal for an operator,
// never silently dropped and never returned over a wire with no safe return.
func TestPermanentMigrateInNackHoldsCustody(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	b.mod.setAckMode(ackNackPermanent)

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.4)
	waitFor(t, 5*time.Second, "sidecar 2 to hold the organism", func() bool {
		return custodyOf(b.side, migrationID) == "in/held"
	})
	time.Sleep(400 * time.Millisecond)
	if got := b.world.spawnCount(migrationID); got != 0 {
		t.Fatalf("slot 2 spawned %d organisms, want 0", got)
	}
	if got := a.world.spawnCount(migrationID); got != 0 {
		t.Fatalf("slot 1 spawned %d organisms, want 0", got)
	}
}

func countMigrateIn(m *fakeMod, migrationID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, env := range m.frames {
		if env.Type != contracta.TypeMigrateIn {
			continue
		}
		var in contracta.MigrateIn
		if json.Unmarshal(env.Data, &in) == nil && in.MigrationID == migrationID {
			n++
		}
	}
	return n
}

func (m *fakeMod) closeCodeNow() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int(m.closeCode)
}

// TestCustodyReassertionOnSessionChange covers contract-a.md §7.4.
func TestCustodyReassertionOnSessionChange(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	b.mod.setAckMode(ackSilent) // keep the migration in flight

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.4)
	a.mod.waitType(contracta.TypeMigrateOutAck, 5*time.Second)
	waitFor(t, 5*time.Second, "the mod to destroy its copy", func() bool {
		return a.world.destroyCount(migrationID) == 1
	})

	// kill -9 on the game: the world rolls back to an autosave that still holds
	// the organism, and the mod mints a fresh sessionId.
	a.mod.abort()
	rolledBack := newWorld()
	rolledBack.put(testEntityID)

	modA2 := dialFakeMod(t, fakeModOptions{
		url: a.side.URL(), world: rolledBack, heartbeat: 200 * time.Millisecond})

	env, _ := modA2.waitFrom(0, 5*time.Second, func(e wire.Envelope) bool {
		if e.Type != contracta.TypeMigrateOutAck {
			return false
		}
		var ack contracta.MigrateOutAck
		return json.Unmarshal(e.Data, &ack) == nil && ack.Unsolicited
	})
	ack := decodeAs[contracta.MigrateOutAck](t, env)
	if ack.MigrationID != migrationID {
		t.Fatalf("reasserted migrationId = %s, want %s", ack.MigrationID, migrationID)
	}
	if ack.EntityID != testEntityID {
		t.Fatalf("reasserted entityId = %d, want %d", ack.EntityID, testEntityID)
	}
	waitFor(t, 5*time.Second, "the rollback artefact to be destroyed", func() bool {
		return !rolledBack.isAlive(testEntityID)
	})
}

// TestSecondModConnectionReplacesTheFirst covers contract-a.md §2.
func TestSecondModConnectionReplacesTheFirst(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	second := dialFakeMod(t, fakeModOptions{
		url: g.node(0).side.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond})
	code := g.node(0).mod.waitClosed(5 * time.Second)
	if int(code) != contracta.CloseReplaced {
		t.Fatalf("close code = %d, want %d (REPLACED)", code, contracta.CloseReplaced)
	}
	second.waitType(contracta.TypeEdgeStatus, 5*time.Second)
}

// TestFirstFrameMustBeConfigUpdate covers contract-a.md §5.1.
func TestFirstFrameMustBeConfigUpdate(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2), noMods: true})
	bad := dialRawMod(t, g.node(1).side.URL())
	tick := int64(1)
	simTime, pop, paused, scale, size := 1.0, 1, false, 1.0, 2000.0
	frame, err := wire.Encode(wire.ProtocolA, contracta.TypeHeartbeat, time.Now().UnixMilli(),
		contracta.Heartbeat{SessionID: wire.NewUUID(), SimTick: &tick, SimulatedTime: &simTime,
			Population: &pop, Paused: &paused, TimeScale: &scale, SimulationSize: &size})
	if err != nil {
		t.Fatal(err)
	}
	bad.write(frame)
	if code := bad.waitClosed(5 * time.Second); code != contracta.CloseMalformedFrame {
		t.Fatalf("close code = %d, want %d (MALFORMED_FRAME)", code, contracta.CloseMalformedFrame)
	}
}

// TestMalformedFrameClosesWith4003 covers contract-a.md §3.2.
func TestMalformedFrameClosesWith4003(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2), noMods: true})
	bad := dialRawMod(t, g.node(1).side.URL())
	bad.write([]byte(`{"protocol":"contract-a/2.0","type":"CONFIG_UPDATE"}`))
	if code := bad.waitClosed(5 * time.Second); code != contracta.CloseMalformedFrame {
		t.Fatalf("close code = %d, want %d (MALFORMED_FRAME)", code, contracta.CloseMalformedFrame)
	}
}

// TestUnusableConfigUpdateClosesWith4003 covers contract-a.md §13 amendment A8.
func TestUnusableConfigUpdateClosesWith4003(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2), noMods: true})
	bad := dialRawMod(t, g.node(1).side.URL())
	size, width := 2000.0, 40.0
	frame, err := wire.Encode(wire.ProtocolA, contracta.TypeConfigUpdate, time.Now().UnixMilli(),
		contracta.ConfigUpdate{
			SessionID: wire.NewUUID(),
			// Not one of §5.1's four reasons. Every other field is valid.
			Reason:         "because",
			GameVersion:    "0.6.3.1",
			ModVersion:     "0.2.0",
			SimulationSize: &size,
			BorderEdges:    []string{contracta.EdgeE},
			ExportEdges:    []string{contracta.EdgeE},
			BorderWidth:    &width,
		})
	if err != nil {
		t.Fatal(err)
	}
	bad.write(frame)
	if code := bad.waitClosed(5 * time.Second); code != contracta.CloseMalformedFrame {
		t.Fatalf("close code = %d, want %d (MALFORMED_FRAME)", code, contracta.CloseMalformedFrame)
	}
}

// TestEmptyOrMissingExportEdgesClosesWith4003 pins contract-a.md §15 A18 on
// BOTH of its unusable shapes.
//
// A18 made exportEdges REQUIRED with at least one member and REMOVED §14 A11's
// fallback, which let a single-entry borderEdges supply the value. CONFIG_UPDATE
// has no NACK channel (§9.3, §13 A8), so the only answer is a close, and
// contract-a.md §3.2 makes that close 4003.
//
// The two shapes are different frames on the wire — `"exportEdges": null` from a
// mod that stopped sending the field, and `"exportEdges": []` from one that
// computed an empty set — and only the first was covered. An implementation that
// reads them through different branches is exactly the one that answers one of
// them by falling back to borderEdges, so both frames below name a borderEdges
// the retired fallback would have accepted.
func TestEmptyOrMissingExportEdgesClosesWith4003(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2), noMods: true})
	size, width := 2000.0, 60.0
	cases := []struct {
		name  string
		edges any
	}{
		{"field absent", nil},
		{"empty array", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := map[string]any{
				"sessionId": wire.NewUUID(), "reason": "connect",
				"gameVersion": "0.6.3.1", "modVersion": "0.2.0",
				"simulationSize": size, "borderWidth": width,
				// Exactly one export-capable edge: the shape the RETIRED
				// fallback resolved. If it ever comes back, this frame is
				// accepted and this test fails.
				"borderEdges": []string{contracta.EdgeE, contracta.EdgeW},
			}
			if c.edges != nil {
				body["exportEdges"] = c.edges
			}
			frame, err := wire.Encode(wire.ProtocolA, contracta.TypeConfigUpdate,
				time.Now().UnixMilli(), body)
			if err != nil {
				t.Fatal(err)
			}
			bad := dialRawMod(t, g.node(1).side.URL())
			bad.write(frame)
			if code := bad.waitClosed(5 * time.Second); code != contracta.CloseMalformedFrame {
				t.Fatalf("close code = %d, want %d (MALFORMED_FRAME)", code, contracta.CloseMalformedFrame)
			}
		})
	}
}

// TestUnknownMigrationIDOnMigrateInAckIsIgnored covers §13 amendment A8.
func TestUnknownMigrationIDOnMigrateInAckIsIgnored(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	entityID := testEntityID
	dup, tick := false, int64(1)
	b.mod.sendFrame(contracta.TypeMigrateInAck, contracta.MigrateInAck{
		MigrationID: wire.NewUUID(), EntityID: &entityID, Duplicate: &dup, SimTick: &tick})

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.42)
	waitFor(t, 5*time.Second, "the migration to complete after a stray MIGRATE_IN_ACK", func() bool {
		return b.world.spawnCount(migrationID) == 1
	})
}

// TestUnsupportedProtocolClosesWith4000 covers contract-a.md §3.1 and §15 A23:
// compatibility is on the major only, and an M3 mod is now on the wrong side of
// it — which is the point of the bump.
func TestUnsupportedProtocolClosesWith4000(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2), noMods: true})
	for _, proto := range []string{"contract-a/9", "contract-a/1.1"} {
		bad := dialRawMod(t, g.node(1).side.URL())
		bad.write([]byte(`{"protocol":"` + proto + `","type":"CONFIG_UPDATE","messageId":"` +
			wire.NewUUID() + `","sentAt":1,"data":{}}`))
		if code := bad.waitClosed(5 * time.Second); code != contracta.CloseProtocolUnsupported {
			t.Fatalf("%s close code = %d, want %d (PROTOCOL_UNSUPPORTED)",
				proto, code, contracta.CloseProtocolUnsupported)
		}
	}
}

// TestOlderMinorIsAccepted covers contract-a.md §14 A16: the minor is never a
// rejection reason, so a mod that sends the bare major keeps working.
func TestOlderMinorIsAccepted(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2), noMods: true})
	m := dialRawMod(t, g.node(0).side.URL())
	size, width := 2000.0, 60.0
	body := map[string]any{
		"sessionId": wire.NewUUID(), "reason": "connect", "gameVersion": "0.6.3.1",
		"modVersion": "0.2.0", "simulationSize": size,
		"borderEdges": []string{contracta.EdgeE, contracta.EdgeW},
		"exportEdges": []string{contracta.EdgeE}, "borderWidth": width,
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	m.write([]byte(`{"protocol":"contract-a/2","type":"CONFIG_UPDATE","messageId":"` +
		wire.NewUUID() + `","sentAt":1,"data":` + string(data) + `}`))
	time.Sleep(500 * time.Millisecond)
	if m.closedNow() {
		t.Fatal("a contract-a/2 mod was closed; the minor is never a rejection reason")
	}
}

// TestUnknownTypeIsIgnored covers contract-a.md §3.1.
func TestUnknownTypeIsIgnored(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	a.mod.sendRaw([]byte(`{"protocol":"contract-a/2.0","type":"SOMETHING_NEW","messageId":"` +
		wire.NewUUID() + `","sentAt":1,"data":{"x":1}}`))
	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.3)
	waitFor(t, 5*time.Second, "the migration to complete after an unknown type", func() bool {
		return b.world.spawnCount(migrationID) == 1
	})
}

// TestJournalSurvivesSidecarRestartInProcess is the in-process half of the
// crash-custody story: the journal is replayed and custody is not lost.
func TestJournalSurvivesSidecarRestartInProcess(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	b.mod.setAckMode(ackSilent)
	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.4)
	waitFor(t, 5*time.Second, "sidecar 2 to journal the inbound organism", func() bool {
		return custodyOf(b.side, migrationID) == "in/in_flight"
	})

	cfg := b.cfg
	_ = b.side.Close()

	reopened, err := New(cfg)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	found := false
	for _, st := range reopened.CustodySnapshot() {
		if st.Entry.MigrationID == migrationID {
			found = true
			if st.Direction != journal.In {
				t.Fatalf("direction = %s, want in", st.Direction)
			}
			if st.Entry.Payload != makePayload(testEntityID) {
				t.Fatal("the payload did not survive the journal round trip")
			}
			if st.Entry.GenomeHash != genomeHashOf(t, makePayload(testEntityID)) {
				t.Fatal("the lineage annex did not survive the journal round trip")
			}
		}
	}
	if !found {
		t.Fatal("custody was lost across the restart")
	}
}

// TestRetiredContractAPathClosesWith4000 covers contract-a.md §15 A23: an M3
// mod dials /contract-a/v1, and a bare HTTP 404 would be a socket error in a
// BepInEx log and half an evening of diagnosis.
func TestRetiredContractAPathClosesWith4000(t *testing.T) {
	g := newGrid(t, 1, gridOptions{noMods: true})
	url := "ws://" + g.node(0).side.Addr() + contracta.RetiredContractAPath
	old := dialRawMod(t, url)
	if code := old.waitClosed(5 * time.Second); code != contracta.CloseProtocolUnsupported {
		t.Fatalf("close code on the retired path = %d, want %d", code, contracta.CloseProtocolUnsupported)
	}
}

// TestRetiredContractBPathClosesWith4000 covers contract-b-m4.md §3: a relay
// MUST keep serving /contract-b/v2 and MUST close every connection on it
// immediately with 4000, so an M3 sidecar gets the defined loud error instead
// of a bare HTTP 404.
func TestRetiredContractBPathClosesWith4000(t *testing.T) {
	rl := startRelay(t)
	sub := dialRawRelay(t, "ws://"+rl.addr+contractb.RetiredContractBPath, testToken)
	if code := sub.waitClosed(5 * time.Second); code != contractb.CloseProtocolUnsupported {
		t.Fatalf("close code on the retired path = %d, want %d", code, contractb.CloseProtocolUnsupported)
	}
}

// TestAnM3SidecarIsRefusedOnTheCurrentPath covers §4: a contract-b/2 sidecar and
// a contract-b/3 relay are incompatible BY DESIGN and say so with close 4000
// rather than misrouting an organism.
func TestAnM3SidecarIsRefusedOnTheCurrentPath(t *testing.T) {
	rl := startRelay(t)
	old := dialRawRelay(t, rl.url(), testToken)
	old.write([]byte(`{"protocol":"contract-b/2.0","type":"HANDSHAKE","messageId":"` +
		wire.NewUUID() + `","sentAt":1,"data":{"peerId":"m3-peer","role":"peer",` +
		`"protocolVersion":"contract-b/2.0","gameVersion":"0.6.3.1","sidecarVersion":"m3.0"}}`))
	if code := old.waitClosed(5 * time.Second); code != contractb.CloseProtocolUnsupported {
		t.Fatalf("close code = %d, want %d (PROTOCOL_UNSUPPORTED)", code, contractb.CloseProtocolUnsupported)
	}
	if got := len(rl.relay.Snapshot()); got != 0 {
		t.Fatalf("an M3 sidecar reserved %d slots", got)
	}
}
