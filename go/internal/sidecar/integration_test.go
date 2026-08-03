package sidecar

import (
	"encoding/json"
	"path/filepath"
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

// TestHappyPath is contract test (a): sector A exports an organism, sector B's
// mod receives MIGRATE_IN and ACKs it, sidecar A gets MIGRATION_ACK and clears
// its journal to a tombstone.
func TestHappyPath(t *testing.T) {
	r := newRig(t, rigOptions{})

	migrationID := r.modA.migrateOut(testEntityID, contracta.EdgeE, 0.6031925)

	ackEnv := r.modA.waitType(contracta.TypeMigrateOutAck, 5*time.Second)
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

	inEnv := r.modB.waitType(contracta.TypeMigrateIn, 5*time.Second)
	in := decodeAs[contracta.MigrateIn](t, inEnv)
	if in.MigrationID != migrationID {
		t.Fatalf("MIGRATE_IN migrationId = %s, want %s", in.MigrationID, migrationID)
	}
	if in.EntityID != testEntityID {
		t.Fatalf("MIGRATE_IN entityId = %d, want %d", in.EntityID, testEntityID)
	}
	// contract-a.md §4.2: E pairs with W. §5.7: the sidecar copies
	// exitPosition into entryPosition unchanged in M2.
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

	waitFor(t, 5*time.Second, "the organism to be alive in sector B", func() bool {
		return r.wB.spawnCount(migrationID) == 1 && r.wB.isAlive(testEntityID)
	})
	waitFor(t, 5*time.Second, "sidecar A's journal entry to become a tombstone", func() bool {
		return custodyOf(r.a, migrationID) == "out/done"
	})
	waitFor(t, 5*time.Second, "sidecar B's journal entry to become a tombstone", func() bool {
		return custodyOf(r.b, migrationID) == "in/done"
	})

	if got := r.wA.destroyCount(migrationID); got != 1 {
		t.Fatalf("sector A destroyed the organism %d times, want 1", got)
	}
	if r.wA.isAlive(testEntityID) {
		t.Fatal("the organism is still alive in sector A after custody transferred")
	}
	if got := r.wA.spawnCount(migrationID) + r.wB.spawnCount(migrationID); got != 1 {
		t.Fatalf("the organism exists %d times across both sims, want exactly 1", got)
	}
}

// --------------------------------------------------------------------- (b)

// TestReconnectReplay is contract test (b): sector B's mod disconnects before
// it ACKs, reconnects with CONFIG_UPDATE, and gets EDGE_STATUS followed by the
// replayed MIGRATE_IN with attempt = 2 (contract-a.md §7.5).
func TestReconnectReplay(t *testing.T) {
	r := newRig(t, rigOptions{})
	r.modB.setAckMode(ackSilent)

	migrationID := r.modA.migrateOut(testEntityID, contracta.EdgeE, 0.25)

	first := decodeAs[contracta.MigrateIn](t, r.modB.waitType(contracta.TypeMigrateIn, 5*time.Second))
	if first.Attempt != 1 {
		t.Fatalf("first delivery attempt = %d, want 1", first.Attempt)
	}
	if r.wB.spawnCount(migrationID) != 0 {
		t.Fatal("a silent mod must not have spawned anything")
	}

	// The game dies without a close handshake.
	r.modB.abort()

	modB2 := dialFakeMod(t, fakeModOptions{
		url: r.b.URL(), world: r.wB, borderEdges: []string{contracta.EdgeW},
		sector: &contracta.Sector{X: 1, Y: 0}, heartbeat: 200 * time.Millisecond})

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

	// The reconnected mod ACKs, so the chain completes exactly once.
	waitFor(t, 5*time.Second, "the replayed organism to land", func() bool {
		return r.wB.spawnCount(migrationID) == 1
	})
	waitFor(t, 5*time.Second, "sidecar A's journal to clear", func() bool {
		return custodyOf(r.a, migrationID) == "out/done"
	})
	if got := r.wA.spawnCount(migrationID) + r.wB.spawnCount(migrationID); got != 1 {
		t.Fatalf("the organism exists %d times across both sims, want exactly 1", got)
	}
}

// --------------------------------------------------------------------- (c)

// TestDuplicateMigrateOutIsIdempotent is contract test (c): a repeated
// MIGRATE_OUT under the same migrationId is re-ACKed without a second journal
// entry and without a second delivery (contract-a.md §5.3 steps 2 and 3).
func TestDuplicateMigrateOutIsIdempotent(t *testing.T) {
	r := newRig(t, rigOptions{})

	migrationID := r.modA.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	r.modA.waitType(contracta.TypeMigrateOutAck, 5*time.Second)

	// The identical message again — the lost-ACK retry of contract-a.md §6.3.
	r.modA.resend(migrationID)
	r.modA.resend(migrationID)

	waitFor(t, 5*time.Second, "three MIGRATE_OUT_ACKs for one migrationId", func() bool {
		return countAcks(r.modA, migrationID) >= 3
	})

	waitFor(t, 5*time.Second, "the delivery to complete", func() bool {
		return r.wB.spawnCount(migrationID) == 1
	})
	// Give any spurious second delivery time to appear.
	time.Sleep(500 * time.Millisecond)
	if got := r.wB.spawnCount(migrationID); got != 1 {
		t.Fatalf("sector B spawned the organism %d times, want 1", got)
	}
	if got := countJournalEntries(r.a, migrationID); got != 1 {
		t.Fatalf("sidecar A has %d journal entries for one migrationId, want 1", got)
	}
	if got := countJournalEntries(r.b, migrationID); got != 1 {
		t.Fatalf("sidecar B has %d journal entries for one migrationId, want 1", got)
	}
	if got := r.wA.destroyCount(migrationID); got != 1 {
		t.Fatalf("sector A destroyed the organism %d times, want 1", got)
	}

	// A different payload under the same key is a mod defect and must be loud.
	r.modA.migrateOutPayload(migrationID, testEntityID, contracta.EdgeE, 0.5, makePayload(12345))
	nackEnv := r.modA.waitType(contracta.TypeMigrateOutNack, 5*time.Second)
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
// and every edge closes, here observed as the neighbour's edge going shut
// (contract-a.md §8).
func TestHeartbeatTimeout(t *testing.T) {
	r := newRig(t, rigOptions{
		// Long enough that the rig reaches "edges open" before the timeout,
		// short enough that the test does not crawl.
		tuneA:      func(c *Config) { c.HeartbeatTimeout = 2 * time.Second },
		heartbeatA: -1, // no HEARTBEAT at all
	})

	code := r.modA.waitClosed(6 * time.Second)
	if int(code) != contracta.CloseHeartbeatTimeout {
		t.Fatalf("close code = %d, want %d (HEARTBEAT_TIMEOUT)", code, contracta.CloseHeartbeatTimeout)
	}

	// contract-a.md §8 step 2: the dead sim must stop receiving organisms, and
	// the neighbour's mod learns about it through EDGE_STATUS.
	closed := r.modB.waitEdge(contracta.EdgeW, false, 6*time.Second)
	if closed.Open {
		t.Fatal("the neighbour's edge should be closed")
	}
	if closed.Reason == contracta.ReasonPeerLive {
		t.Fatalf("edge closed with reason %q", closed.Reason)
	}
}

// --------------------------------------------------------------------- (f)

// TestRelayDropAndRecovery is contract test (f): killing the relay closes both
// sidecars' edges and the closure reaches both mods; restarting it restores the
// rig and migration works again.
func TestRelayDropAndRecovery(t *testing.T) {
	r := newRig(t, rigOptions{})

	r.relay.kill()

	closedA := r.modA.waitEdge(contracta.EdgeE, false, 6*time.Second)
	closedB := r.modB.waitEdge(contracta.EdgeW, false, 6*time.Second)
	// contract-b-m2.md §5.5: with the relay link down a sidecar knows nothing
	// about its neighbour.
	if closedA.Reason != contracta.ReasonPeerUnreachable {
		t.Fatalf("sector A edge reason = %q, want peer_unreachable", closedA.Reason)
	}
	if closedB.Reason != contracta.ReasonPeerUnreachable {
		t.Fatalf("sector B edge reason = %q, want peer_unreachable", closedB.Reason)
	}

	r.relay.restart()

	r.modA.waitEdge(contracta.EdgeE, true, 10*time.Second)
	r.modB.waitEdge(contracta.EdgeW, true, 10*time.Second)
	if got := r.a.Sector(); got != "A" {
		t.Fatalf("sector A reclaimed %q after the relay restart, want A", got)
	}
	if got := r.b.Sector(); got != "B" {
		t.Fatalf("sector B reclaimed %q after the relay restart, want B", got)
	}

	migrationID := r.modA.migrateOut(testEntityID, contracta.EdgeE, 0.75)
	waitFor(t, 10*time.Second, "a migration to complete after recovery", func() bool {
		return r.wB.spawnCount(migrationID) == 1
	})
	if got := r.wA.spawnCount(migrationID) + r.wB.spawnCount(migrationID); got != 1 {
		t.Fatalf("the organism exists %d times across both sims, want exactly 1", got)
	}
}

// --------------------------------------------------------- extra contract rules

// TestBounceBackOnNoPeer covers the timeout half of contract-b-m2.md §7: an
// outbound organism that never reached a live peer comes home as a MIGRATE_IN
// with bounceBack = true.
//
// The journal is seeded the way a SIGKILLed sidecar leaves it — custody taken,
// nothing forwarded — because that is the only state from which a bounce is
// reachable without a race: once a peer is live enough to accept a MIGRATE_OUT
// it is also live enough to receive the forward.
func TestBounceBackOnNoPeer(t *testing.T) {
	relaySrv := startRelay(t)
	dataDir := t.TempDir()
	migrationID := seedOutboundCustody(t, dataDir)

	cfg := fastConfig(t, relaySrv.url(), "peer-a", contractb.SectorA)
	cfg.DataDir = dataDir
	sideA := startSidecar(t, cfg)
	waitSector(t, sideA, contractb.SectorA)

	world := newWorld()
	modA := dialFakeMod(t, fakeModOptions{
		url: sideA.URL(), world: world, borderEdges: []string{contracta.EdgeE},
		sector: &contracta.Sector{X: 0, Y: 0}, heartbeat: 200 * time.Millisecond})

	env, _ := modA.waitFrom(0, 10*time.Second,
		func(e wire.Envelope) bool { return e.Type == contracta.TypeMigrateIn })
	in := decodeAs[contracta.MigrateIn](t, env)
	if in.MigrationID != migrationID {
		t.Fatalf("bounced migrationId = %s, want %s", in.MigrationID, migrationID)
	}
	if !in.BounceBack {
		t.Fatal("a bounced delivery must carry bounceBack = true")
	}
	// Custody chain step 6: it comes home through the edge it left by.
	if in.EntryEdge != contracta.EdgeE {
		t.Fatalf("bounce entryEdge = %s, want E", in.EntryEdge)
	}
	if in.EntityID != testEntityID {
		t.Fatalf("bounced entityId = %d, want %d", in.EntityID, testEntityID)
	}
	waitFor(t, 5*time.Second, "the bounced organism to be alive again in sector A", func() bool {
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
		Edge:           contracta.EdgeE,
		Position:       0.4,
		VelocityX:      6.12,
		VelocityY:      0.44,
		Heading:        274.11,
		SimulationSize: 2000,
		SourceSector:   contractb.SectorA,
		DestSector:     contractb.SectorB,
		JournaledAt:    time.Now().UnixMilli(),
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
	r := newRig(t, rigOptions{})
	_ = r.b.Close()
	r.modA.waitEdge(contracta.EdgeE, false, 6*time.Second)

	migrationID := r.modA.migrateOut(testEntityID, contracta.EdgeE, 0.4)
	nack := decodeAs[contracta.MigrateOutNack](t, r.modA.waitType(contracta.TypeMigrateOutNack, 5*time.Second))
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
	if custodyOf(r.a, migrationID) != "absent" {
		t.Fatal("a refused migration must not be journaled")
	}
	if !r.wA.isAlive(testEntityID) {
		t.Fatal("the mod must keep an organism the sidecar refused")
	}
}

// TestUnsupportedEdgeIsRefused covers contract-a.md §5.1: the sidecar must not
// open an edge the mod never declared.
func TestUnsupportedEdgeIsRefused(t *testing.T) {
	r := newRig(t, rigOptions{})
	r.modA.migrateOut(testEntityID, contracta.EdgeN, 0.4)
	nack := decodeAs[contracta.MigrateOutNack](t, r.modA.waitType(contracta.TypeMigrateOutNack, 5*time.Second))
	if nack.Code != contracta.OutEdgeClosed {
		t.Fatalf("NACK code = %s, want EDGE_CLOSED", nack.Code)
	}
}

// TestInvalidPayloadNack covers contract-a.md §9.1's INVALID_PAYLOAD: nothing
// invalid ever reaches a mod, in either direction.
func TestInvalidPayloadNack(t *testing.T) {
	r := newRig(t, rigOptions{})
	r.modA.migrateOutPayload(wire.NewUUID(), testEntityID, contracta.EdgeE, 0.4, "not json at all")
	nack := decodeAs[contracta.MigrateOutNack](t, r.modA.waitType(contracta.TypeMigrateOutNack, 5*time.Second))
	if nack.Code != contracta.OutInvalidPayload {
		t.Fatalf("NACK code = %s, want INVALID_PAYLOAD", nack.Code)
	}
	if nack.Class != contracta.ClassPermanent {
		t.Fatalf("NACK class = %s, want permanent", nack.Class)
	}
}

// TestMalformedMessageNack covers contract-a.md §9.3: a data field that fails
// validation is a NACK, not a close.
func TestMalformedMessageNack(t *testing.T) {
	r := newRig(t, rigOptions{})
	// exitPosition outside [0,1]: a valid sender always clamps (§4.3).
	r.modA.migrateOut(testEntityID, contracta.EdgeE, 1.5)
	nack := decodeAs[contracta.MigrateOutNack](t, r.modA.waitType(contracta.TypeMigrateOutNack, 5*time.Second))
	if nack.Code != contracta.OutMalformedMessage {
		t.Fatalf("NACK code = %s, want MALFORMED_MESSAGE", nack.Code)
	}
	if code := r.modA.closeCodeNow(); code != -1 {
		t.Fatalf("the connection was closed with %d; a data-level fault must keep it open", code)
	}
}

// TestTransientMigrateInNackKeepsCustody covers contract-a.md §9.2: a transient
// MIGRATE_IN_NACK keeps custody and re-delivers.
func TestTransientMigrateInNackKeepsCustody(t *testing.T) {
	r := newRig(t, rigOptions{})
	r.modB.setAckMode(ackNackTransient)

	migrationID := r.modA.migrateOut(testEntityID, contracta.EdgeE, 0.4)
	r.modB.waitType(contracta.TypeMigrateIn, 5*time.Second)
	waitFor(t, 5*time.Second, "a re-delivery after the transient NACK", func() bool {
		return countMigrateIn(r.modB, migrationID) >= 2
	})
	if custodyOf(r.b, migrationID) != "in/in_flight" {
		t.Fatalf("sidecar B custody = %s, want in/in_flight", custodyOf(r.b, migrationID))
	}

	// Once the mod is ready, the organism lands exactly once.
	r.modB.setAckMode(ackNormal)
	waitFor(t, 10*time.Second, "the organism to land after the mod recovers", func() bool {
		return r.wB.spawnCount(migrationID) == 1
	})
}

// TestPermanentMigrateInNackHoldsCustody covers contract-b-m2.md §7: a
// permanently rejected inbound organism is held in the journal for an operator,
// never silently dropped and never returned over a wire with no safe return.
func TestPermanentMigrateInNackHoldsCustody(t *testing.T) {
	r := newRig(t, rigOptions{})
	r.modB.setAckMode(ackNackPermanent)

	migrationID := r.modA.migrateOut(testEntityID, contracta.EdgeE, 0.4)
	waitFor(t, 5*time.Second, "sidecar B to hold the organism", func() bool {
		return custodyOf(r.b, migrationID) == "in/held"
	})
	time.Sleep(400 * time.Millisecond)
	if got := r.wB.spawnCount(migrationID); got != 0 {
		t.Fatalf("sector B spawned %d organisms, want 0", got)
	}
	if got := r.wA.spawnCount(migrationID); got != 0 {
		t.Fatalf("sector A spawned %d organisms, want 0", got)
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
	r := newRig(t, rigOptions{})
	r.modB.setAckMode(ackSilent) // keep the migration in flight

	migrationID := r.modA.migrateOut(testEntityID, contracta.EdgeE, 0.4)
	r.modA.waitType(contracta.TypeMigrateOutAck, 5*time.Second)
	waitFor(t, 5*time.Second, "the mod to destroy its copy", func() bool {
		return r.wA.destroyCount(migrationID) == 1
	})

	// kill -9 on the game: the world rolls back to an autosave that still
	// holds the organism, and the mod mints a fresh sessionId.
	r.modA.abort()
	rolledBack := newWorld()
	rolledBack.put(testEntityID)

	modA2 := dialFakeMod(t, fakeModOptions{
		url: r.a.URL(), world: rolledBack, borderEdges: []string{contracta.EdgeE},
		sector: &contracta.Sector{X: 0, Y: 0}, heartbeat: 200 * time.Millisecond})

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
	r := newRig(t, rigOptions{})
	second := dialFakeMod(t, fakeModOptions{
		url: r.a.URL(), world: newWorld(), borderEdges: []string{contracta.EdgeE},
		heartbeat: 200 * time.Millisecond})
	code := r.modA.waitClosed(5 * time.Second)
	if int(code) != contracta.CloseReplaced {
		t.Fatalf("close code = %d, want %d (REPLACED)", code, contracta.CloseReplaced)
	}
	second.waitType(contracta.TypeEdgeStatus, 5*time.Second)
}

// TestFirstFrameMustBeConfigUpdate covers contract-a.md §5.1.
func TestFirstFrameMustBeConfigUpdate(t *testing.T) {
	r := newRig(t, rigOptions{skipEdgeCheck: true})
	bad := dialRawMod(t, r.b.URL())
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
	r := newRig(t, rigOptions{skipEdgeCheck: true})
	bad := dialRawMod(t, r.b.URL())
	bad.write([]byte(`{"protocol":"contract-a/1","type":"CONFIG_UPDATE"}`))
	if code := bad.waitClosed(5 * time.Second); code != contracta.CloseMalformedFrame {
		t.Fatalf("close code = %d, want %d (MALFORMED_FRAME)", code, contracta.CloseMalformedFrame)
	}
}

// TestUnusableConfigUpdateClosesWith4003 covers contract-a.md §13 amendment A8:
// the envelope is well-formed, so §3.2 does not apply, but CONFIG_UPDATE has no
// NACK channel — the only answer left is a close.
func TestUnusableConfigUpdateClosesWith4003(t *testing.T) {
	r := newRig(t, rigOptions{skipEdgeCheck: true})
	bad := dialRawMod(t, r.b.URL())
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
// sidecar no longer knows is a late race, not a defect. Closing on it would turn
// a race into an outage.
func TestUnknownMigrationIDOnMigrateInAckIsIgnored(t *testing.T) {
	r := newRig(t, rigOptions{})
	entityID := testEntityID
	dup, tick := false, int64(1)
	r.modB.sendFrame(contracta.TypeMigrateInAck, contracta.MigrateInAck{
		MigrationID: wire.NewUUID(), EntityID: &entityID, Duplicate: &dup, SimTick: &tick})

	// The connection stays open and the custody chain still runs end to end.
	migrationID := r.modA.migrateOut(testEntityID, contracta.EdgeE, 0.42)
	waitFor(t, 5*time.Second, "the migration to complete after a stray MIGRATE_IN_ACK", func() bool {
		return r.wB.spawnCount(migrationID) == 1
	})
}

// TestUnsupportedProtocolClosesWith4000 covers contract-a.md §3.1.
func TestUnsupportedProtocolClosesWith4000(t *testing.T) {
	r := newRig(t, rigOptions{skipEdgeCheck: true})
	bad := dialRawMod(t, r.b.URL())
	bad.write([]byte(`{"protocol":"contract-a/9","type":"CONFIG_UPDATE","messageId":"` +
		wire.NewUUID() + `","sentAt":1,"data":{}}`))
	if code := bad.waitClosed(5 * time.Second); code != contracta.CloseProtocolUnsupported {
		t.Fatalf("close code = %d, want %d (PROTOCOL_UNSUPPORTED)", code, contracta.CloseProtocolUnsupported)
	}
}

// TestUnknownTypeIsIgnored covers contract-a.md §3.1: an unknown type is a
// forward-compatible addition, not a fault.
func TestUnknownTypeIsIgnored(t *testing.T) {
	r := newRig(t, rigOptions{})
	r.modA.sendRaw([]byte(`{"protocol":"contract-a/1","type":"SOMETHING_NEW","messageId":"` +
		wire.NewUUID() + `","sentAt":1,"data":{"x":1}}`))
	// The connection stays open and still works.
	migrationID := r.modA.migrateOut(testEntityID, contracta.EdgeE, 0.3)
	waitFor(t, 5*time.Second, "the migration to complete after an unknown type", func() bool {
		return r.wB.spawnCount(migrationID) == 1
	})
}

// TestJournalSurvivesSidecarRestartInProcess is the in-process half of the
// crash-custody story: the journal is replayed and custody is not lost.
func TestJournalSurvivesSidecarRestartInProcess(t *testing.T) {
	r := newRig(t, rigOptions{})
	r.modB.setAckMode(ackSilent)
	migrationID := r.modA.migrateOut(testEntityID, contracta.EdgeE, 0.4)
	waitFor(t, 5*time.Second, "sidecar B to journal the inbound organism", func() bool {
		return custodyOf(r.b, migrationID) == "in/in_flight"
	})

	cfg := r.b.cfg
	_ = r.b.Close()

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
		}
	}
	if !found {
		t.Fatal("custody was lost across the restart")
	}
}
