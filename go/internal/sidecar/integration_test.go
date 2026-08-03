package sidecar

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"multiverse/internal/bb8"
	"multiverse/internal/contracta"
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

// TestHappyPath is contract test (a), carried from M2 and re-pointed at the
// ring: slot 1 exports an organism east, slot 2's mod receives MIGRATE_IN and
// ACKs it, sidecar 1 gets MIGRATION_ACK and clears its journal to a tombstone.
func TestHappyPath(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	a, b := r.node(0), r.node(1)

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
	// contract-a.md §14 A11: ordinary ring traffic enters through the passive
	// west edge, and the receiver derives it — the wire never carries entryEdge.
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

// --------------------------------------------------------------------- M3

// TestRingCircumnavigation is the M3 exit test's first phase in miniature: one
// organism travels slot 1 -> 2 -> 3 -> 1 eastward, the blob is byte-identical
// at every hop, and every hop's lineage annex carries the genome hash the
// canonical projection produces for that blob (contract-b-m3.md §2, §6.6).
func TestRingCircumnavigation(t *testing.T) {
	r := newRing(t, 3, ringOptions{})

	// The ring order is [1, 2, 3]; east(1)=2, east(2)=3, east(3)=1.
	waitEastSlot(t, r.node(0).side, 2)
	waitEastSlot(t, r.node(1).side, 3)
	waitEastSlot(t, r.node(2).side, 1)

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
		src, dst := r.node(hop.from), r.node(hop.to)
		before := dst.mod.frameCount()

		hop.migration = src.mod.migrateOutPayload(wire.NewUUID(), testEntityID,
			contracta.EdgeE, 0.5, payload)

		env, _ := dst.mod.waitFrom(before, 10*time.Second,
			func(e wire.Envelope) bool { return e.Type == contracta.TypeMigrateIn })
		in := decodeAs[contracta.MigrateIn](t, env)
		if in.MigrationID != hop.migration {
			t.Fatalf("hop %d delivered migrationId %s, want %s", i+1, in.MigrationID, hop.migration)
		}
		// The blob is opaque and must arrive byte-identical (D4).
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

		// The annex: the sending sidecar hashed the migrant's genome, and the
		// receiver recorded the same value it was sent.
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
		// §6.6 step 6: each peer caches the migrant's genome, so any of them
		// can answer a later GENOME_REQUEST.
		if !dst.side.Genomes().Has(wantHash) {
			t.Fatalf("hop %d receiver did not cache the migrant's genome", i+1)
		}
		// The organism exists exactly once across the whole ring.
		total := 0
		for _, nd := range r.nodes {
			if nd.world.isAlive(testEntityID) {
				total++
			}
		}
		if total != 1 {
			t.Fatalf("after hop %d the organism is alive in %d sims, want exactly 1", i+1, total)
		}
	}

	// It came home: the last hop landed in slot 1's world.
	if !r.node(0).world.isAlive(testEntityID) {
		t.Fatal("the organism did not come back round the ring to slot 1")
	}
}

// TestRingInsertionWhileRunning covers contract-b-m3.md §7.2 and Risk 4: a new
// peer is appended at the tail, exactly one existing lane changes, and no
// surviving slot is renumbered or reordered.
func TestRingInsertionWhileRunning(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	one, two := r.node(0), r.node(1)

	waitEastSlot(t, one.side, 2)
	waitEastSlot(t, two.side, 1)
	one.mod.waitEdge(contracta.EdgeE, true, 5*time.Second)
	two.mod.waitEdge(contracta.EdgeE, true, 5*time.Second)
	oneEpochBefore := one.mod.edgeEpochNow()

	three := r.addPeer("peer-slot3", ringOptions{heartbeat: 200 * time.Millisecond})
	if three.slot != 3 {
		t.Fatalf("the new peer took slot %d, want 3 (maxSlotEverIssued + 1)", three.slot)
	}

	// Exactly one lane moved: the old tail's. Slot 1 still exports into slot 2.
	waitEastSlot(t, two.side, 3)
	waitEastSlot(t, three.side, 1)
	if got := eastSlotOf(one.side); got != 2 {
		t.Fatalf("slot 1's lane changed to %d; an append must move exactly one lane", got)
	}

	// Surviving slots keep their numbers and their relative order.
	res := r.relay.relay.RingSnapshot()
	if len(res) != 3 {
		t.Fatalf("ring has %d slots, want 3", len(res))
	}
	for i, want := range []int{1, 2, 3} {
		if res[i].Slot != want {
			t.Fatalf("ring order is %v; slots must keep their numbers and order", res)
		}
	}

	// Slot 1 was undisturbed: its export edge never closed across the insertion.
	for _, st := range one.mod.edgeHistorySince(oneEpochBefore) {
		for _, e := range st.Edges {
			if !e.Open {
				t.Fatalf("slot 1's export edge closed during the insertion (reason %q)", e.Reason)
			}
		}
	}
	// And it still works.
	migrationID := one.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "a migration into the undisturbed lane", func() bool {
		return two.world.spawnCount(migrationID) == 1
	})
}

// TestOfflineSlotKeepsItsReservation covers D8 and contract-b-m3.md §8: a slot
// belongs to a peer identity, not to a connection. Its lane closes while the
// peer is away — one-way, so only its west neighbour notices — and reopens when
// the same peerId comes back to the same slot.
func TestOfflineSlotKeepsItsReservation(t *testing.T) {
	r := newRing(t, 3, ringOptions{})
	one, two, three := r.node(0), r.node(1), r.node(2)
	waitEastSlot(t, one.side, 2)

	two.mod.abort()
	_ = two.side.Close()

	// The west neighbour loses its export target. A dying peer takes its mod
	// with it, so the edge may pass through peer_mod_absent before it settles
	// on no_peer; what matters is where it settles.
	one.mod.waitEdgeReason(contracta.EdgeE, false, contracta.ReasonNoPeer, 10*time.Second)
	// The ripple is one-way: slot 3 exports into slot 1 and is untouched.
	if st := three.mod.edgeNow(contracta.EdgeE); !st.Open {
		t.Fatalf("slot 3's edge closed with %q; the ripple must be one-way", st.Reason)
	}
	// The reservation survives: three slots, slot 2 still reserved.
	res := r.relay.relay.RingSnapshot()
	if len(res) != 3 || res[1].Slot != 2 || res[1].PeerID != two.cfg.PeerID {
		t.Fatalf("ring after the peer left is %v; a reservation never expires", res)
	}

	// The same peerId and the same data dir come back and reclaim slot 2.
	revived := startSidecar(t, two.cfg)
	waitSlot(t, revived, 2)
	revivedMod := dialFakeMod(t, fakeModOptions{
		url: revived.URL(), world: two.world, heartbeat: 200 * time.Millisecond})

	one.mod.waitEdge(contracta.EdgeE, true, 10*time.Second)
	revivedMod.waitEdge(contracta.EdgeE, true, 10*time.Second)
	if got := len(r.relay.relay.RingSnapshot()); got != 3 {
		t.Fatalf("ring grew to %d slots on a reclaim; the peer must not take a second slot", got)
	}

	migrationID := one.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "a migration into the returned slot", func() bool {
		return two.world.spawnCount(migrationID) == 1
	})
}

// TestArchiveRecordsAndFetchesAGenome covers contract-b-m3.md §5.1 and §10: the
// archive is copied every routed envelope, records it with its annex, and
// fetches by hash a parent genome that never travelled the wire.
func TestArchiveRecordsAndFetchesAGenome(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	one, two := r.node(0), r.node(1)
	arc := startArchive(t, r.relay.url())

	// A parent that is still alive locally: the mod ships its opaque blob, the
	// sidecar hashes it, caches it and strips it (contract-a.md §14, A12).
	const livingParent int32 = -1180911975
	const goneParent int32 = 204418833
	parentBlob := makePayload(livingParent)
	parentHash := genomeHashOf(t, parentBlob)
	migrantHash := genomeHashOf(t, makePayload(testEntityID))

	// The archive has never seen either genome.
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

	// The archive recorded the envelope with its annex.
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

	// And it fetched both genomes by hash, from a peer, over GENOME_REQUEST.
	waitFor(t, 15*time.Second, "the archive to fetch the migrant genome by hash", func() bool {
		return arc.Genomes().Has(migrantHash)
	})
	waitFor(t, 15*time.Second, "the archive to fetch the parent genome by hash", func() bool {
		return arc.Genomes().Has(parentHash)
	})
	// §6.10: the fetched bytes hash to the label they were served under.
	e, ok := arc.Genomes().Get(parentHash)
	if !ok {
		t.Fatal("the parent genome is not in the archive store")
	}
	if e.BB8 != parentBlob {
		t.Fatal("the archive stored bytes that are not the parent blob")
	}

	// The read path shows the migration with its lineage.
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
		// A re-forward after a lost ACK is legal and answers duplicate: true,
		// which §6.7 says is treated exactly like a plain ACK.
		if !strings.HasPrefix(m.Outcome, "delivered") {
			t.Fatalf("the read path reports outcome %q, want a delivered outcome", m.Outcome)
		}
	}
	if !shown {
		t.Fatal("the read path did not list the migration")
	}
}

// TestArchiveMayNotSendMigrations covers contract-b-m3.md §5.1: a
// MIGRATION_PAYLOAD from a subscriber is answered NOT_A_MEMBER and is not
// forwarded.
func TestArchiveMayNotSendMigrations(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	sub := dialSubscriber(t, r.relay.url(), testToken)

	nack := sub.sendMigrationAndWaitNack(t, 10*time.Second)
	if nack.Code != "NOT_A_MEMBER" {
		t.Fatalf("NACK code = %s, want NOT_A_MEMBER", nack.Code)
	}
	if nack.Class != "permanent" {
		t.Fatalf("NACK class = %s, want permanent", nack.Class)
	}
	// Nothing was delivered to the ring.
	time.Sleep(300 * time.Millisecond)
	for i, nd := range r.nodes {
		if len(nd.side.CustodySnapshot()) != 0 {
			t.Fatalf("slot %d journaled something a subscriber sent", i+1)
		}
	}
	// And a claim from a subscriber never gets a slot.
	grant := sub.claimAndWaitGrant(t, 10*time.Second)
	if grant.Granted || grant.Reason != "role_has_no_slot" {
		t.Fatalf("subscriber claim returned %+v, want granted=false role_has_no_slot", grant)
	}
}

// TestWrongLANTokenIsRejected covers contract-b-m3.md §3.1: a missing or wrong
// token gets HTTP 401 and no upgrade, so there is no WebSocket and no close
// code, and the peer never joins the ring.
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

	// A sidecar with the wrong token never gets a ring slot, and the ring stays
	// empty rather than reserving one for a stranger.
	cfg := fastConfig(t, rl.url(), "peer-with-a-bad-token")
	cfg.Token = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	s := startSidecar(t, cfg)
	dialFakeMod(t, fakeModOptions{url: s.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond})
	time.Sleep(1500 * time.Millisecond)
	if got := s.Slot(); got != 0 {
		t.Fatalf("a sidecar with a wrong token got slot %d", got)
	}
	if got := len(rl.relay.RingSnapshot()); got != 0 {
		t.Fatalf("the ring has %d slots; an unauthenticated peer must never reserve one", got)
	}

	// The right token joins.
	good := fastConfig(t, rl.url(), "peer-with-the-right-token")
	goodSide := startSidecar(t, good)
	waitSlot(t, goodSide, 1)
}

// TestExportEdgeFallbackAndNarrowing covers contract-a.md §14 A11 and A16: a
// contract-a/1 mod with no exportEdge is served from a single-entry
// borderEdges, EDGE_STATUS carries exactly one entry, and an ambiguous
// borderEdges with no exportEdge closes 4003.
func TestExportEdgeFallbackAndNarrowing(t *testing.T) {
	r := newRing(t, 2, ringOptions{})

	// EDGE_STATUS is narrowed to the single export edge, even though the mod
	// declared two border edges.
	st := r.node(0).mod.lastEdgeStatus()
	if len(st.Edges) != 1 {
		t.Fatalf("EDGE_STATUS carried %d entries, want exactly one", len(st.Edges))
	}
	if st.Edges[0].Edge != contracta.EdgeE {
		t.Fatalf("EDGE_STATUS reported edge %s, want the export edge E", st.Edges[0].Edge)
	}

	// The A11 fallback: no exportEdge and one border edge.
	fallback := startSidecar(t, fastConfig(t, r.relay.url(), "peer-old-mod"))
	waitSlotAny(t, fallback)
	oldMod := dialFakeMod(t, fakeModOptions{
		url: fallback.URL(), world: newWorld(), omitExportEdge: true,
		borderEdges: []string{contracta.EdgeE}, heartbeat: 200 * time.Millisecond})
	got := oldMod.waitTypeStatus(5 * time.Second)
	if len(got.Edges) != 1 || got.Edges[0].Edge != contracta.EdgeE {
		t.Fatalf("the fallback did not resolve the export edge: %+v", got.Edges)
	}

	// Ambiguous: no exportEdge and two border edges is unusable, and
	// CONFIG_UPDATE has no NACK channel, so the only answer left is 4003.
	ambiguous := startSidecar(t, fastConfig(t, r.relay.url(), "peer-ambiguous-mod"))
	bad := dialRawMod(t, ambiguous.URL())
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
}

// TestRetiredSectorFieldIsIgnored covers contract-a.md §14 A14: a
// contract-a/1.1 sidecar ignores the retired {x,y} sector an older mod sends
// and MUST NOT close on it.
func TestRetiredSectorFieldIsIgnored(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	side := startSidecar(t, fastConfig(t, r.relay.url(), "peer-with-a-retired-sector"))
	waitSlotAny(t, side)

	old := dialRawMod(t, side.URL())
	size, width := 2000.0, 60.0
	body := map[string]any{
		"sessionId": wire.NewUUID(), "reason": "connect", "gameVersion": "0.6.3.1",
		"modVersion": "0.2.0", "simulationSize": size,
		"borderEdges": []string{contracta.EdgeE}, "borderWidth": width,
		// The retired field, exactly as a contract-a/1 mod would send it.
		"sector": map[string]int{"x": 0, "y": 0},
	}
	frame, err := wire.Encode(wire.ProtocolA, contracta.TypeConfigUpdate, time.Now().UnixMilli(), body)
	if err != nil {
		t.Fatal(err)
	}
	old.write(frame)
	// It must not close. The sidecar answers with the one EDGE_STATUS §6.1
	// promises instead.
	time.Sleep(500 * time.Millisecond)
	if old.closedNow() {
		t.Fatal("the sidecar closed on a retired sector field; A14 says ignore it")
	}
}

// TestSlotMismatchClosesWith4001 covers contract-a.md §14 A14: ringSlot is
// advisory, and a disagreement is a mis-wired rig worth one second of
// diagnosis.
func TestSlotMismatchClosesWith4001(t *testing.T) {
	r := newRing(t, 2, ringOptions{noMods: true})
	wrong := 99
	m := dialFakeMod(t, fakeModOptions{
		url: r.node(0).side.URL(), world: newWorld(), ringSlot: &wrong,
		heartbeat: 200 * time.Millisecond})
	if code := m.waitClosed(5 * time.Second); int(code) != contracta.CloseSlotMismatch {
		t.Fatalf("close code = %d, want %d (SLOT_MISMATCH)", code, contracta.CloseSlotMismatch)
	}
}

// TestPeerModAbsentClosesTheWestLane covers contract-a.md §14 A11's new reason
// and contract-b-m3.md §8: a sidecar that loses its mod publishes that, and its
// west neighbour closes its export edge with peer_mod_absent. A dead sim must
// not keep receiving organisms.
func TestPeerModAbsentClosesTheWestLane(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	one, two := r.node(0), r.node(1)
	one.mod.waitEdge(contracta.EdgeE, true, 5*time.Second)

	two.mod.close()

	closed := one.mod.waitEdge(contracta.EdgeE, false, 10*time.Second)
	if closed.Reason != contracta.ReasonPeerModAbsent {
		t.Fatalf("slot 1's edge closed with %q, want peer_mod_absent", closed.Reason)
	}
}

// --------------------------------------------------------------------- (b)

// TestReconnectReplay is contract test (b): the destination mod disconnects
// before it ACKs, reconnects with CONFIG_UPDATE, and gets EDGE_STATUS followed
// by the replayed MIGRATE_IN with attempt = 2 (contract-a.md §7.5).
func TestReconnectReplay(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	a, b := r.node(0), r.node(1)
	b.mod.setAckMode(ackSilent)

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.25)

	first := decodeAs[contracta.MigrateIn](t, b.mod.waitType(contracta.TypeMigrateIn, 5*time.Second))
	if first.Attempt != 1 {
		t.Fatalf("first delivery attempt = %d, want 1", first.Attempt)
	}
	if b.world.spawnCount(migrationID) != 0 {
		t.Fatal("a silent mod must not have spawned anything")
	}

	// The game dies without a close handshake.
	b.mod.abort()

	modB2 := dialFakeMod(t, fakeModOptions{
		url: b.side.URL(), world: b.world, heartbeat: 200 * time.Millisecond})

	// contract-a.md §6.1: exactly one EDGE_STATUS follows the handshake, and
	// the replay comes after it.
	firstFrame, _ := modB2.waitFrom(0, 5*time.Second, func(wire.Envelope) bool { return true })
	if firstFrame.Type != contracta.TypeEdgeStatus {
		t.Fatalf("first frame after reconnect was %s, want EDGE_STATUS", firstFrame.Type)
	}
	replayEnv, idx := modB2.waitFrom(0, 5*time.Second,
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

// TestDuplicateMigrateOutIsIdempotent is contract test (c): a repeated
// MIGRATE_OUT under the same migrationId is re-ACKed without a second journal
// entry and without a second delivery (contract-a.md §5.3 steps 2 and 3).
func TestDuplicateMigrateOutIsIdempotent(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	a, b := r.node(0), r.node(1)

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	a.mod.waitType(contracta.TypeMigrateOutAck, 5*time.Second)

	// The identical message again — the lost-ACK retry of contract-a.md §6.3.
	a.mod.resend(migrationID)
	a.mod.resend(migrationID)

	waitFor(t, 5*time.Second, "three MIGRATE_OUT_ACKs for one migrationId", func() bool {
		return countAcks(a.mod, migrationID) >= 3
	})
	waitFor(t, 5*time.Second, "the delivery to complete", func() bool {
		return b.world.spawnCount(migrationID) == 1
	})
	// Give any spurious second delivery time to appear.
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

	// A different payload under the same key is a mod defect and must be loud.
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
// and its west neighbour's export edge closes, because a sim with no mod cannot
// spawn anything (contract-a.md §8, contract-b-m3.md §8).
func TestHeartbeatTimeout(t *testing.T) {
	r := newRing(t, 2, ringOptions{
		// Long enough that the rig reaches "edges open" before the timeout,
		// short enough that the test does not crawl.
		tune: func(i int, c *Config) {
			if i == 1 {
				c.HeartbeatTimeout = 2 * time.Second
			}
		},
	})
	one, two := r.node(0), r.node(1)
	one.mod.waitEdge(contracta.EdgeE, true, 5*time.Second)

	// Slot 2's mod goes silent. Under the ring it is slot 1's east neighbour.
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
// export edge and the closure reaches the mods; restarting it restores the ring
// from ring.json, with the same slots, and migration works again.
func TestRelayDropAndRecovery(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	one, two := r.node(0), r.node(1)

	r.relay.kill()

	closedA := one.mod.waitEdge(contracta.EdgeE, false, 8*time.Second)
	closedB := two.mod.waitEdge(contracta.EdgeE, false, 8*time.Second)
	// contract-b-m3.md §8: with the relay link down a sidecar knows nothing
	// about its east neighbour.
	if closedA.Reason != contracta.ReasonPeerUnreachable {
		t.Fatalf("slot 1 edge reason = %q, want peer_unreachable", closedA.Reason)
	}
	if closedB.Reason != contracta.ReasonPeerUnreachable {
		t.Fatalf("slot 2 edge reason = %q, want peer_unreachable", closedB.Reason)
	}

	r.relay.restart()

	one.mod.waitEdge(contracta.EdgeE, true, 15*time.Second)
	two.mod.waitEdge(contracta.EdgeE, true, 15*time.Second)
	// §7.4: ring.json is durable, so the reservations survive the restart.
	if got := one.side.Slot(); got != 1 {
		t.Fatalf("slot 1 reclaimed %d after the relay restart", got)
	}
	if got := two.side.Slot(); got != 2 {
		t.Fatalf("slot 2 reclaimed %d after the relay restart", got)
	}
	if got := len(r.relay.relay.RingSnapshot()); got != 2 {
		t.Fatalf("the ring has %d slots after a restart, want 2", got)
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

// TestBounceBackOnNoPeer covers the timeout half of contract-b-m3.md §9: an
// outbound organism that never reached a live peer comes home as a MIGRATE_IN
// with bounceBack = true, on the edge it left from — its own exportEdge.
//
// The journal is seeded the way a SIGKILLed sidecar leaves it — custody taken,
// nothing forwarded — because that is the only state from which a bounce is
// reachable without a race: once a peer is live enough to accept a MIGRATE_OUT
// it is also live enough to receive the forward.
func TestBounceBackOnNoPeer(t *testing.T) {
	relaySrv := startRelay(t)
	dataDir := t.TempDir()
	migrationID := seedOutboundCustody(t, dataDir)

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
	// Custody chain step 6 and §14 A11: it comes home through the door it left
	// by, which under the ring is the sim's own export edge.
	if in.EntryEdge != contracta.EdgeE {
		t.Fatalf("bounce entryEdge = %s, want the origin's own exportEdge E", in.EntryEdge)
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
// one outbound organism, durably in custody, never forwarded.
func seedOutboundCustody(t *testing.T, dataDir string) string {
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
		// A slot nobody holds: the relay answers SLOT_VACANT and the organism
		// bounces home.
		DestSlot:    2,
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
// of an edge with no live neighbour is refused and the mod keeps the organism.
func TestEdgeClosedNack(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	a, b := r.node(0), r.node(1)
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

// TestNonExportEdgeIsRefused covers contract-a.md §14 A11: the sidecar MUST NOT
// open any edge but exportEdge, whatever borderEdges says. The west edge is a
// declared border edge here and still cannot be exported through.
func TestNonExportEdgeIsRefused(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	r.node(0).mod.migrateOut(testEntityID, contracta.EdgeW, 0.4)
	nack := decodeAs[contracta.MigrateOutNack](t,
		r.node(0).mod.waitType(contracta.TypeMigrateOutNack, 5*time.Second))
	if nack.Code != contracta.OutEdgeClosed {
		t.Fatalf("NACK code = %s, want EDGE_CLOSED", nack.Code)
	}
}

// TestInvalidPayloadNack covers contract-a.md §9.1's INVALID_PAYLOAD: nothing
// invalid ever reaches a mod, in either direction.
func TestInvalidPayloadNack(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	r.node(0).mod.migrateOutPayload(wire.NewUUID(), testEntityID, contracta.EdgeE, 0.4, "not json at all")
	nack := decodeAs[contracta.MigrateOutNack](t,
		r.node(0).mod.waitType(contracta.TypeMigrateOutNack, 5*time.Second))
	if nack.Code != contracta.OutInvalidPayload {
		t.Fatalf("NACK code = %s, want INVALID_PAYLOAD", nack.Code)
	}
	if nack.Class != contracta.ClassPermanent {
		t.Fatalf("NACK class = %s, want permanent", nack.Class)
	}
}

// TestUnhashableGenomeStillMigrates covers genome-hash.md §8 and
// contract-b-m3.md §6.8: the annex is bookkeeping, the organism is custody. A
// blob that passes validation but will not hash still crosses.
func TestUnhashableGenomeStillMigrates(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	a, b := r.node(0), r.node(1)
	// Structurally valid JSON, but neither genome dialect matches.
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
	r := newRing(t, 2, ringOptions{})
	// exitPosition outside [0,1]: a valid sender always clamps (§4.3).
	r.node(0).mod.migrateOut(testEntityID, contracta.EdgeE, 1.5)
	nack := decodeAs[contracta.MigrateOutNack](t,
		r.node(0).mod.waitType(contracta.TypeMigrateOutNack, 5*time.Second))
	if nack.Code != contracta.OutMalformedMessage {
		t.Fatalf("NACK code = %s, want MALFORMED_MESSAGE", nack.Code)
	}
	if code := r.node(0).mod.closeCodeNow(); code != -1 {
		t.Fatalf("the connection was closed with %d; a data-level fault must keep it open", code)
	}
}

// TestTransientMigrateInNackKeepsCustody covers contract-a.md §9.2: a transient
// MIGRATE_IN_NACK keeps custody and re-delivers.
func TestTransientMigrateInNackKeepsCustody(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	a, b := r.node(0), r.node(1)
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

// TestPermanentMigrateInNackHoldsCustody covers contract-b-m3.md §9: a
// permanently rejected inbound organism is held in the journal for an operator,
// never silently dropped and never returned over a wire with no safe return.
func TestPermanentMigrateInNackHoldsCustody(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	a, b := r.node(0), r.node(1)
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

// TestCustodyReassertionOnSessionChange covers contract-a.md §7.4: a new
// sessionId makes the sidecar re-assert every outbound custody it holds, so a
// rolled-back world cannot keep a copy of an exported organism.
func TestCustodyReassertionOnSessionChange(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	a, b := r.node(0), r.node(1)
	b.mod.setAckMode(ackSilent) // keep the migration in flight

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.4)
	a.mod.waitType(contracta.TypeMigrateOutAck, 5*time.Second)
	waitFor(t, 5*time.Second, "the mod to destroy its copy", func() bool {
		return a.world.destroyCount(migrationID) == 1
	})

	// kill -9 on the game: the world rolls back to an autosave that still
	// holds the organism, and the mod mints a fresh sessionId.
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

// TestSecondModConnectionReplacesTheFirst covers contract-a.md §2: a newer mod
// connection takes over and the older one is closed with 4006.
func TestSecondModConnectionReplacesTheFirst(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	second := dialFakeMod(t, fakeModOptions{
		url: r.node(0).side.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond})
	code := r.node(0).mod.waitClosed(5 * time.Second)
	if int(code) != contracta.CloseReplaced {
		t.Fatalf("close code = %d, want %d (REPLACED)", code, contracta.CloseReplaced)
	}
	second.waitType(contracta.TypeEdgeStatus, 5*time.Second)
}

// TestFirstFrameMustBeConfigUpdate covers contract-a.md §5.1.
func TestFirstFrameMustBeConfigUpdate(t *testing.T) {
	r := newRing(t, 2, ringOptions{noMods: true})
	bad := dialRawMod(t, r.node(1).side.URL())
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
	r := newRing(t, 2, ringOptions{noMods: true})
	bad := dialRawMod(t, r.node(1).side.URL())
	bad.write([]byte(`{"protocol":"contract-a/1.1","type":"CONFIG_UPDATE"}`))
	if code := bad.waitClosed(5 * time.Second); code != contracta.CloseMalformedFrame {
		t.Fatalf("close code = %d, want %d (MALFORMED_FRAME)", code, contracta.CloseMalformedFrame)
	}
}

// TestUnusableConfigUpdateClosesWith4003 covers contract-a.md §13 amendment A8:
// the envelope is well-formed, so §3.2 does not apply, but CONFIG_UPDATE has no
// NACK channel — the only answer left is a close.
func TestUnusableConfigUpdateClosesWith4003(t *testing.T) {
	r := newRing(t, 2, ringOptions{noMods: true})
	bad := dialRawMod(t, r.node(1).side.URL())
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
			ExportEdge:     contracta.EdgeE,
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

// TestUnknownMigrationIDOnMigrateInAckIsIgnored covers the exception in
// contract-a.md §13 amendment A8: a well-formed reply naming a migration the
// sidecar no longer knows is a late race, not a defect.
func TestUnknownMigrationIDOnMigrateInAckIsIgnored(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	a, b := r.node(0), r.node(1)
	entityID := testEntityID
	dup, tick := false, int64(1)
	b.mod.sendFrame(contracta.TypeMigrateInAck, contracta.MigrateInAck{
		MigrationID: wire.NewUUID(), EntityID: &entityID, Duplicate: &dup, SimTick: &tick})

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.42)
	waitFor(t, 5*time.Second, "the migration to complete after a stray MIGRATE_IN_ACK", func() bool {
		return b.world.spawnCount(migrationID) == 1
	})
}

// TestUnsupportedProtocolClosesWith4000 covers contract-a.md §3.1 and §14 A16:
// compatibility is on the major only.
func TestUnsupportedProtocolClosesWith4000(t *testing.T) {
	r := newRing(t, 2, ringOptions{noMods: true})
	bad := dialRawMod(t, r.node(1).side.URL())
	bad.write([]byte(`{"protocol":"contract-a/9","type":"CONFIG_UPDATE","messageId":"` +
		wire.NewUUID() + `","sentAt":1,"data":{}}`))
	if code := bad.waitClosed(5 * time.Second); code != contracta.CloseProtocolUnsupported {
		t.Fatalf("close code = %d, want %d (PROTOCOL_UNSUPPORTED)", code, contracta.CloseProtocolUnsupported)
	}
}

// TestOlderMinorIsAccepted covers contract-a.md §14 A16: the minor is never a
// rejection reason, so a contract-a/1 mod keeps working against a
// contract-a/1.1 sidecar.
func TestOlderMinorIsAccepted(t *testing.T) {
	r := newRing(t, 2, ringOptions{noMods: true})
	m := dialRawMod(t, r.node(0).side.URL())
	size, width := 2000.0, 60.0
	body := map[string]any{
		"sessionId": wire.NewUUID(), "reason": "connect", "gameVersion": "0.6.3.1",
		"modVersion": "0.2.0", "simulationSize": size,
		"borderEdges": []string{contracta.EdgeE}, "borderWidth": width,
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	m.write([]byte(`{"protocol":"contract-a/1","type":"CONFIG_UPDATE","messageId":"` +
		wire.NewUUID() + `","sentAt":1,"data":` + string(data) + `}`))
	time.Sleep(500 * time.Millisecond)
	if m.closedNow() {
		t.Fatal("a contract-a/1 mod was closed; the minor is never a rejection reason")
	}
}

// TestUnknownTypeIsIgnored covers contract-a.md §3.1: an unknown type is a
// forward-compatible addition, not a fault.
func TestUnknownTypeIsIgnored(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	a, b := r.node(0), r.node(1)
	a.mod.sendRaw([]byte(`{"protocol":"contract-a/1.1","type":"SOMETHING_NEW","messageId":"` +
		wire.NewUUID() + `","sentAt":1,"data":{"x":1}}`))
	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.3)
	waitFor(t, 5*time.Second, "the migration to complete after an unknown type", func() bool {
		return b.world.spawnCount(migrationID) == 1
	})
}

// TestJournalSurvivesSidecarRestartInProcess is the in-process half of the
// crash-custody story: the journal is replayed and custody is not lost.
func TestJournalSurvivesSidecarRestartInProcess(t *testing.T) {
	r := newRing(t, 2, ringOptions{})
	a, b := r.node(0), r.node(1)
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
