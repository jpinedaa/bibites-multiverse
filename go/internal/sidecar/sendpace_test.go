package sidecar

// The client half of §3.3, tested against the episode that asked for it
// (contract-b-m4.md §3.3, §6.2, §22 B24).

import (
	"path/filepath"
	"testing"
	"time"

	"multiverse/internal/bb8"
	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/wire"
)

// TestSlot6RejoinBacklogDrainsWithoutASingleCapacityShed IS THE EPISODE OF
// 2026-08-11 ~21:40, reproduced.
//
// Slot 6 rejoined after four and a half hours dark, drained its journal backlog
// at the relay as fast as its tick could hand frames over, and was shed with
// close 4007 EIGHTEEN TIMES — every measured peak exactly 51/s against
// maxFramesPerSecond 50 — in a cycle of reclaim, burst, shed, ~30 s backoff,
// reclaim. It converged only because each connection window drained some frames
// before the shed fired, and a stranger returning after days away would cycle
// exactly the same way.
//
// The rig here is that peer: a journal holding a backlog LARGER THAN THE
// PUBLISHED CEILING, two live same-axis destinations, and a relay running a
// ceiling of its own choosing rather than the shipped 50. The first destination
// cannot admit the whole burst to its paced transport queue. Exact
// NOT_FORWARDED proofs move those entries to the second destination. The bar is
// the one WP8's churn story needs: the whole backlog arrives once, and no 4007
// occurs.
func TestSlot6RejoinBacklogDrainsWithoutASingleCapacityShed(t *testing.T) {
	// A published ceiling that is NOT the shipped default, so a sidecar that
	// paced itself against a compiled 50 would fail this test.
	const publishedFrames = 40
	// The backlog is deliberately bigger than the ceiling: unpaced, the first
	// tick puts all of it on the wire and the relay's meter sheds on frame 41.
	const backlog = 64

	rl := startRelayWithLimits(t, contractb.Limits{MaxFramesPerSecond: publishedFrames})
	for i, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 2, Row: 0}} {
		p := pos
		if _, _, err := rl.relay.ReserveSlot([]string{"peer-a", "peer-b", "peer-c"}[i], &p); err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
	}

	// The destination. Its inbound queue is widened because this test is about
	// the SENDER's rate: an inboundQueueMax refusal is a different rule (§6.6
	// step 2) and it would mask the one under test.
	cfgB := fastConfig(t, rl, "peer-b")
	cfgB.StatsInterval = time.Second
	cfgB.InboundQueueMax = 4 * backlog
	sideB := startSidecar(t, cfgB)
	waitSlot(t, sideB, 2)
	worldB := newWorld()
	dialFakeMod(t, fakeModOptions{
		url: sideB.URL(), world: worldB, heartbeat: 200 * time.Millisecond})
	cfgC := fastConfig(t, rl, "peer-c")
	cfgC.StatsInterval = time.Second
	cfgC.InboundQueueMax = 4 * backlog
	sideC := startSidecar(t, cfgC)
	waitSlot(t, sideC, 3)
	worldC := newWorld()
	dialFakeMod(t, fakeModOptions{
		url: sideC.URL(), world: worldC, heartbeat: 200 * time.Millisecond})

	// The peer that was dark: its journal already holds the backlog before its
	// process exists, which is what a rejoin after hours away looks like.
	cfgA := fastConfig(t, rl, "peer-a")
	cfgA.StatsInterval = time.Second
	ids := seedOutboundBacklog(t, cfgA.DataDir, 2, backlog)

	sideA := startSidecar(t, cfgA)
	waitSlot(t, sideA, 1)
	waitFor(t, 10*time.Second, "peer-a to see slot 2 live", func() bool { return destLiveOf(sideA, 2) })

	if got := sideA.PacedFramesPerSecond(); got != publishedFrames*sendPaceRateFraction {
		t.Fatalf("peer-a paced itself at %v frames/s against a PUBLISHED %d; want %v — "+
			"the rate must be a fraction of what the relay published, never a constant",
			got, publishedFrames, publishedFrames*sendPaceRateFraction)
	}

	// The drain lasts longer than this rig's peer timeout. A starved PONG would
	// therefore close the link with 4004. A link that stays up proves that
	// control frames move through the deferred backlog rather than behind it.
	started := time.Now()
	waitFor(t, 60*time.Second, "the whole backlog to drain", func() bool {
		if !sideA.RelayConnected() {
			if sheds := sideA.CapacitySheds(); sheds > 0 {
				// This IS the episode: the burst went over the published ceiling
				// and the relay shed the connection, leaving the same backlog to
				// burst again after the backoff.
				t.Fatalf("the relay shed peer-a with close 4007 (%d times) %s into the drain; "+
					"the backlog is going out faster than the published %d frames/s",
					sheds, time.Since(started), publishedFrames)
			}
			t.Fatalf("peer-a's link dropped %s into the drain with no capacity shed; a PONG or "+
				"the stats PING starved behind the deferred backlog and the relay timed this peer out",
				time.Since(started))
		}
		return doneCount(sideA, ids) == len(ids)
	})
	elapsed := time.Since(started)

	if got := sideA.CapacitySheds(); got != 0 {
		t.Fatalf("the drain took %d capacity sheds (close 4007); the whole point of pacing "+
			"under the published ceiling is that it takes NONE", got)
	}
	spawnedB, spawnedC := spawnedOf(worldB, ids), spawnedOf(worldC, ids)
	if spawnedB+spawnedC != len(ids) {
		t.Fatalf("the two destinations spawned %d + %d of %d organisms", spawnedB, spawnedC, len(ids))
	}
	if spawnedC == 0 {
		t.Fatal("the 64-entry burst never filled slot 2's transport queue, so the alternate path was not exercised")
	}
	for _, id := range ids {
		if got := worldB.spawnCount(id) + worldC.spawnCount(id); got != 1 {
			t.Fatalf("migration %s spawned %d times across the two destinations, want exactly 1", id, got)
		}
	}
	// A drain that finished instantly did not pace. The bucket admits its burst
	// and then refills at half the published rate, so 64 frames cannot leave
	// this process in under (64 - burst) / 20 seconds however fast the disk is.
	if elapsed < 1500*time.Millisecond {
		t.Fatalf("the backlog drained in %s, which is faster than the published ceiling of "+
			"%d/s allows; nothing paced it", elapsed, publishedFrames)
	}
	if got := sideA.PacedDeferrals(); got == 0 {
		t.Fatal("the pacer deferred no frame at all while draining a backlog bigger than the " +
			"ceiling; it is not on the send path")
	}
	t.Logf("drained %d journalled organisms in %s under a published %d frames/s "+
		"(slot 2: %d, slot 3: %d, paced %v/s, %d deferrals, 0 sheds)",
		backlog, elapsed.Round(time.Millisecond), publishedFrames,
		spawnedB, spawnedC, sideA.PacedFramesPerSecond(), sideA.PacedDeferrals())
}

// TestCompletedInboundBurstDrainsWithoutASingleCapacityShed reproduces the
// second backlog shape. A paused world can release its full Contract A burst,
// and each completed inbound entry then owes its sender one MIGRATION_ACK. The
// acknowledgements are durable, so they must drain under the same published
// ceiling as outbound payloads instead of leaving in one 50-frame breath.
func TestCompletedInboundBurstDrainsWithoutASingleCapacityShed(t *testing.T) {
	const publishedFrames = 40
	const backlog = 50
	const outbound = 20

	rl := startRelayWithLimits(t, contractb.Limits{MaxFramesPerSecond: publishedFrames})
	for i, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}} {
		p := pos
		if _, _, err := rl.relay.ReserveSlot([]string{"peer-a", "peer-b"}[i], &p); err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
	}
	cfgB := fastConfig(t, rl, "peer-b")
	cfgB.StatsInterval = time.Second
	cfgB.InboundQueueMax = 4 * outbound
	sideB := startSidecar(t, cfgB)
	worldB := newWorld()
	dialFakeMod(t, fakeModOptions{
		url: sideB.URL(), world: worldB, heartbeat: 200 * time.Millisecond,
	})
	waitSlot(t, sideB, 2)

	cfg := fastConfig(t, rl, "peer-a")
	cfg.StatsInterval = time.Second
	ids := seedCompletedInboundBacklog(t, cfg.DataDir, "source-that-is-offline", backlog)
	outboundIDs := seedOutboundBacklog(t, cfg.DataDir, 2, outbound)

	side := startSidecar(t, cfg)
	waitSlot(t, side, 1)
	waitFor(t, 5*time.Second, "peer-a to see slot 2 live", func() bool { return destLiveOf(side, 2) })
	side.mu.Lock()
	initialPending := side.custodyStateLocked(side.now()).PendingAckDepth
	side.mu.Unlock()
	if initialPending == 0 {
		t.Fatal("the pending ACK gauge did not see the seeded durable backlog")
	}

	started := time.Now()
	waitFor(t, 10*time.Second, "the completed inbound ACK backlog to drain", func() bool {
		if sheds := side.CapacitySheds(); sheds > 0 {
			t.Fatalf("the relay shed peer-a with close 4007 (%d times) %s into the ACK drain; "+
				"journal-backed MIGRATION_ACKs bypassed the published %d frames/s ceiling",
				sheds, time.Since(started), publishedFrames)
		}
		side.mu.Lock()
		custody := side.custodyStateLocked(side.now())
		spawned := spawnedOf(worldB, outboundIDs)
		side.mu.Unlock()
		if custody.PendingAckDepth > 0 && spawned > 0 {
			t.Fatalf("%d outbound payloads left while %d older durable ACKs were pending; "+
				"the custody scheduler did not give ACKs priority", spawned, custody.PendingAckDepth)
		}
		return custody.PendingAckDepth == 0 && ackedUpstreamCount(side, ids) == len(ids)
	})

	if !side.RelayConnected() {
		t.Fatal("peer-a disconnected while its journal-backed MIGRATION_ACKs drained")
	}
	if got := side.CapacitySheds(); got != 0 {
		t.Fatalf("the ACK drain took %d capacity sheds, want none", got)
	}
	if got := side.PacedDeferrals(); got == 0 {
		t.Fatal("the ACK backlog produced no pacing deferral; it did not use the shared send budget")
	}
	if elapsed := time.Since(started); elapsed < time.Second {
		t.Fatalf("the %d-entry ACK backlog drained in %s; it left as an unpaced burst", backlog, elapsed)
	}
	waitFor(t, 10*time.Second, "the outbound backlog to follow the ACK backlog", func() bool {
		return doneCount(side, outboundIDs) == len(outboundIDs)
	})
	if got := spawnedOf(worldB, outboundIDs); got != len(outboundIDs) {
		t.Fatalf("slot 2 spawned %d of %d outbound organisms after the ACK drain", got, outbound)
	}
}

// TestFreshExportWaitsBehindPendingDurableAck covers the Contract A entry point,
// not only the periodic journal walk. A new MIGRATE_OUT is journaled and ACKed
// to the game immediately, but it cannot take a deferred wire token while an
// older completed arrival still owes its source a durable MIGRATION_ACK.
func TestFreshExportWaitsBehindPendingDurableAck(t *testing.T) {
	const publishedFrames = 40

	rl := startRelayWithLimits(t, contractb.Limits{MaxFramesPerSecond: publishedFrames})
	for i, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}} {
		p := pos
		if _, _, err := rl.relay.ReserveSlot([]string{"peer-a", "peer-b"}[i], &p); err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
	}

	cfgB := fastConfig(t, rl, "peer-b")
	sideB := startSidecar(t, cfgB)
	worldB := newWorld()
	dialFakeMod(t, fakeModOptions{
		url: sideB.URL(), world: worldB, heartbeat: 200 * time.Millisecond,
	})
	waitSlot(t, sideB, 2)

	cfgA := fastConfig(t, rl, "peer-a")
	// Keep the periodic scheduler out of the assertion. The test invokes one
	// custody tick explicitly after it proves that fresh-export dispatch waited.
	cfgA.TickInterval = time.Hour
	seeded := seedCompletedInboundBacklog(t, cfgA.DataDir, "source-that-is-offline", 1)
	sideA := startSidecar(t, cfgA)
	waitSlot(t, sideA, 1)
	worldA := newWorld()
	modA := dialFakeMod(t, fakeModOptions{
		url: sideA.URL(), world: worldA, heartbeat: 200 * time.Millisecond,
	})
	modA.waitEdge(contracta.EdgeE, true, 5*time.Second)

	sideA.mu.Lock()
	if !sideA.hasPendingAckLocked() {
		sideA.mu.Unlock()
		t.Fatal("the seeded durable ACK was not pending before the fresh export")
	}
	sideA.mu.Unlock()

	fresh := modA.migrateOut(-12100, contracta.EdgeE, 0.5)
	modA.waitType(contracta.TypeMigrateOutAck, 5*time.Second)
	// The mod ACK proves the direct onMigrateOut path finished. With the periodic
	// ticker parked, any remote spawn here came only from that direct path.
	time.Sleep(100 * time.Millisecond)
	if got := worldB.spawnCount(fresh); got != 0 {
		t.Fatalf("fresh export spawned %d times while a durable ACK was pending; want 0", got)
	}
	st, ok := sideA.jr.Get(fresh)
	if !ok || st.Handoff != journal.HandoffPending {
		t.Fatalf("fresh export handoff = %+v, present %v; want pending behind durable ACK", st, ok)
	}

	sideA.tick(sideA.now())
	waitFor(t, 5*time.Second, "the durable ACK to leave before the fresh export", func() bool {
		return ackedUpstreamCount(sideA, seeded) == 1 && worldB.spawnCount(fresh) == 1
	})
}

// TestAbsentLimitsLeaveTheSendPathUnpaced is §6.2's other reader: a peer talking
// to a relay that PREDATES B24. Absence of the limits object reads as UNKNOWN,
// never as "no ceilings" and never as an invitation to invent one — the
// behaviour stays exactly M4's, with §3.2's two-4007s pin underneath it.
func TestAbsentLimitsLeaveTheSendPathUnpaced(t *testing.T) {
	var p sendPace
	if p.configure(nil, time.Now()) {
		t.Fatal("an absent limits object configured a pacer; §6.2's omitempty means UNKNOWN")
	}
	// A relay that published a table WITHOUT the frame ceiling is the same case
	// for this limit, and must not be turned into a guess either.
	if p.configure(map[string]int64{contractb.LimitMaxClaimsPerMinute: 12}, time.Now()) {
		t.Fatal("a limits object with no maxFramesPerSecond configured a frame pacer")
	}
	if p.framesPerSecond() != 0 {
		t.Fatalf("an unconfigured pacer reports %v frames/s, want 0", p.framesPerSecond())
	}
	now := time.Now()
	for i := 0; i < 5000; i++ {
		if !p.admit(now, 4096, true) {
			t.Fatalf("bulk frame %d was deferred with no published ceiling to defer it under", i)
		}
	}
	if p.deferred != 0 {
		t.Fatalf("an unconfigured pacer deferred %d frames", p.deferred)
	}

	// And once a ceiling IS published, a later frame that omits the object must
	// not silently unthrottle a session that is already pacing.
	if !p.configure(map[string]int64{contractb.LimitMaxFramesPerSecond: 40}, now) {
		t.Fatal("a published maxFramesPerSecond did not configure the pacer")
	}
	if p.configure(nil, now) {
		t.Fatal("an absent object on a later frame re-configured a running pacer")
	}
	if p.framesPerSecond() != 20 {
		t.Fatalf("pacing at %v/s under a published 40/s, want 20/s", p.framesPerSecond())
	}
}

// TestThePaceFollowsThePublishedNumberAndNotAConstant is D20's rule read from
// the client side: the relay publishes THE VALUES IT IS RUNNING WITH, so a peer
// built against the shipped 50 is a peer that will be shed by any operator who
// turned the knob down. Two relays, two ceilings, two rates.
func TestThePaceFollowsThePublishedNumberAndNotAConstant(t *testing.T) {
	for _, tc := range []struct {
		published int64
		wantRate  float64
		wantBurst float64
	}{
		{contractb.DefaultMaxFramesPerSecond, 25, 12},
		{40, 20, 10},
		{500, 250, 125},
		// A ceiling so low that a fraction of it rounds to nothing still has to
		// leave the peer able to send: one frame a second, one frame of burst.
		{1, 1, 1},
	} {
		var p sendPace
		if !p.configure(map[string]int64{contractb.LimitMaxFramesPerSecond: tc.published}, time.Now()) {
			t.Fatalf("published %d did not configure the pacer", tc.published)
		}
		if p.frames.rate != tc.wantRate || p.frames.capacity != tc.wantBurst {
			t.Fatalf("published %d gave rate %v burst %v, want rate %v burst %v",
				tc.published, p.frames.rate, p.frames.capacity, tc.wantRate, tc.wantBurst)
		}
		// The reason for the headroom, restated as an assertion: a token bucket
		// admits at most capacity+rate in any one second, and relay/capacity.go's
		// fixed window sheds on the FIRST frame over. Whatever the published
		// number, the worst second this pacer can produce stays under it.
		if worst := p.frames.rate + p.frames.capacity; tc.published > 2 && worst >= float64(tc.published) {
			t.Fatalf("published %d: the worst second this pacer allows is %v, which is not "+
				"under the ceiling it is pacing against", tc.published, worst)
		}
	}
}

// TestNoBulkFrameLeavesBeforeTheHandshakeIsAcknowledged closes the window the
// custody scheduler opens. The tick runs on its own clock and does not wait for
// a handshake, so on a rejoin its first pass can land in the millisecond between
// the HANDSHAKE going out and the HANDSHAKE_ACK coming back — with the ceiling
// still unknown and the whole backlog ready to go. §5.2 wants the same gate for
// its own reason: relaySessionId arrives ON that ack, and an entry handed over
// before it is stamped with an empty session no later proof can match.
func TestNoBulkFrameLeavesBeforeTheHandshakeIsAcknowledged(t *testing.T) {
	var p sendPace
	now := time.Now()
	if p.readyForBulk(now) {
		t.Fatal("the pacer offered a bulk slot before HANDSHAKE_ACK; a rejoin's first tick " +
			"would burst the backlog at a ceiling it has not read yet")
	}
	if p.admit(now, 1024, true) {
		t.Fatal("a MIGRATION_PAYLOAD was admitted before HANDSHAKE_ACK")
	}
	// The HANDSHAKE itself is a control frame and must not be held by its own
	// acknowledgement.
	if !p.admit(now, 1024, false) {
		t.Fatal("a control frame was held waiting for HANDSHAKE_ACK; the HANDSHAKE is one of them")
	}

	p.configure(map[string]int64{contractb.LimitMaxFramesPerSecond: 40}, now)
	if !p.readyForBulk(now) || !p.admit(now, 1024, true) {
		t.Fatal("bulk did not resume once the handshake was acknowledged")
	}

	// A NEW session is a new relay, a new session id and possibly a new table,
	// so the gate closes again.
	p.reset()
	if p.admit(now, 1024, true) {
		t.Fatal("the gate stayed open across a session reset")
	}
}

// TestControlFramesAreNotStarvedBehindADrain is the reserve. A drain is bulk
// traffic and bulk traffic may spend the bucket down to a floor and no further,
// because the frames that keep a connection ALIVE — the PONG of §6.11, the
// stats PING, or the immediate reply to a relay-paced migration arrival — are
// not queued behind anything and must not have to wait for a backlog. A
// journal-backed MIGRATION_ACK is different: its tombstone makes deferral safe.
func TestControlFramesAreNotStarvedBehindADrain(t *testing.T) {
	var p sendPace
	p.configure(map[string]int64{contractb.LimitMaxFramesPerSecond: 40}, time.Now())
	now := time.Now()

	// Saturate: keep offering bulk at one instant until the pacer says no.
	sent := 0
	for p.admit(now, 512, true) {
		sent++
		if sent > 1000 {
			t.Fatal("the pacer never refused a bulk frame; the bucket has no bottom")
		}
	}
	if sent == 0 {
		t.Fatal("the pacer refused the FIRST bulk frame on a full bucket")
	}
	// At the exact moment bulk is refused, control still goes — and keeps going
	// past the reserve bulk was stopped at, and past the bottom of the bucket.
	for i := 0; i < 8; i++ {
		if !p.admit(now, 512, false) {
			t.Fatalf("control frame %d was refused behind a saturated drain; a starved PONG "+
				"trades close 4007 for close 4004", i)
		}
	}
	// Control spending is CHARGED, so it holds the drain back rather than being
	// invisible to it — every frame the relay counts is a frame counted here.
	if p.frames.tokens >= 0 {
		t.Fatalf("eight control frames past a saturated bucket left %v tokens; control traffic "+
			"is not being charged, so the pacer's arithmetic is not the relay's", p.frames.tokens)
	}
	// And the drain resumes on its own as the bucket refills. Nothing is dropped
	// and nothing needs an operator.
	if !p.admit(now.Add(time.Second), 512, true) {
		t.Fatal("bulk did not resume a second after the bucket was drained")
	}
}

// TestJournalBackedMigrationAcksUseTheDeferredBudget is the regression for a
// paused world releasing a full inbound burst. Every completed journal entry
// can offer one MIGRATION_ACK in the same tick. Those ACKs must share the
// payload budget, while an immediate NACK or tombstone re-ACK uses the control
// reserve for the relay-paced arrival that caused it.
func TestJournalBackedMigrationAcksUseTheDeferredBudget(t *testing.T) {
	if !paceDeferred(contractb.TypeMigrationPayload) {
		t.Fatal("MIGRATION_PAYLOAD is not on the deferred send path")
	}
	if !paceDeferred(contractb.TypeMigrationAck) {
		t.Fatal("a journal-backed MIGRATION_ACK can bypass the deferred send path")
	}
	for _, typ := range []string{
		contractb.TypeMigrationNack,
		contractb.TypePong,
		contractb.TypePing,
	} {
		if paceDeferred(typ) {
			t.Fatalf("%s was classified as a durable retry, but its immediate send path has no journal transition", typ)
		}
	}
}

func TestImmediateMigrationRepliesBypassAnExhaustedDeferredBudget(t *testing.T) {
	rl := startRelayWithLimits(t, contractb.Limits{MaxFramesPerSecond: 40})
	if _, _, err := rl.relay.ReserveSlot("peer-a", nil); err != nil {
		t.Fatalf("ReserveSlot: %v", err)
	}
	cfg := fastConfig(t, rl, "peer-a")
	cfg.TickInterval = time.Hour
	cfg.StatsInterval = time.Hour
	side := startSidecar(t, cfg)
	waitSlot(t, side, 1)

	side.mu.Lock()
	side.sendPace.frames.tokens = side.sendPace.frames.reserve
	side.sendPace.bytes.tokens = side.sendPace.bytes.reserve
	side.sendPace.last = time.Now()
	deferredBefore := side.sendPace.deferred
	sentBefore := side.sent.totalFrames
	side.mu.Unlock()

	side.nackUpstream(wire.NewUUID(), "peer-offline", contractb.NackOverloaded, "test")
	side.ackUpstreamNow(wire.NewUUID(), "peer-offline", 42, true)

	side.mu.Lock()
	deferredAfter := side.sendPace.deferred
	sentAfter := side.sent.totalFrames
	side.mu.Unlock()
	if deferredAfter != deferredBefore {
		t.Fatalf("immediate ACK/NACK increased deferred count from %d to %d; "+
			"a reply used the journal path", deferredBefore, deferredAfter)
	}
	if sentAfter < sentBefore+2 {
		t.Fatalf("only %d of 2 immediate replies left after the deferred budget was exhausted",
			sentAfter-sentBefore)
	}
}

// ------------------------------------------------------------------ helpers

// seedOutboundBacklog writes the journal of a sidecar whose world kept exporting
// while its relay link was down: every entry is journaled, durable, and PENDING
// — handed to nobody. It is the state slot 6 came back to, and since §25's B37
// it is the only backlog a conforming sender can put on the wire at all.
func seedOutboundBacklog(t *testing.T, dataDir string, destSlot, n int) []string {
	t.Helper()
	jr, err := journal.Open(filepath.Join(dataDir, "journal"))
	if err != nil {
		t.Fatalf("seed journal: %v", err)
	}
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		entityID := int32(9000 + i)
		payload := makePayload(entityID)
		migrationID := wire.NewUUID()
		if _, err := jr.Create(journal.Out, journal.Entry{
			MigrationID:    migrationID,
			EntityID:       entityID,
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
			t.Fatalf("seed journal entry %d: %v", i, err)
		}
		// PENDING, not sent: the frame reached nobody. That is what a rejoin
		// backlog IS since §25's B37 — a world that kept exporting while its
		// relay link was down — and it is the only backlog a conforming sender
		// can still put on the wire, because a frame it already wrote is never
		// written again.
		if _, err := jr.Apply(migrationID, journal.Update{
			Handoff: journal.HandoffPending}); err != nil {
			t.Fatalf("seed handoff %d: %v", i, err)
		}
		ids = append(ids, migrationID)
	}
	if err := jr.Close(); err != nil {
		t.Fatalf("seed journal close: %v", err)
	}
	return ids
}

// seedCompletedInboundBacklog writes the durable state left when a mod accepted
// a burst and the sidecar stopped before it could answer the remote senders.
// Every entry is done but has AckedUpstream false, so startup must retry it.
func seedCompletedInboundBacklog(t *testing.T, dataDir, sourcePeer string, n int) []string {
	t.Helper()
	jr, err := journal.Open(filepath.Join(dataDir, "journal"))
	if err != nil {
		t.Fatalf("seed inbound ACK backlog: %v", err)
	}
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := wire.NewUUID()
		entityID := int32(12000 + i)
		if _, err := jr.Create(journal.In, journal.Entry{
			MigrationID: id,
			EntityID:    entityID,
			Kind:        contracta.KindBibite,
			GameVersion: "0.6.3.1",
			SourcePeer:  sourcePeer,
			SourceSlot:  2,
			DestSlot:    1,
			Edge:        contracta.EdgeW,
			JournaledAt: time.Now().UnixMilli(),
		}, false); err != nil {
			t.Fatalf("seed inbound ACK entry %d: %v", i, err)
		}
		completed := time.Now().UnixMilli()
		if _, err := jr.Apply(id, journal.Update{
			Status: journal.StatusDone, Handoff: journal.HandoffDone, CompletedAt: &completed,
		}); err != nil {
			t.Fatalf("complete inbound ACK entry %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	if err := jr.Close(); err != nil {
		t.Fatalf("seed inbound ACK backlog close: %v", err)
	}
	return ids
}

func ackedUpstreamCount(s *Sidecar, ids []string) int {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	n := 0
	for _, st := range s.CustodySnapshot() {
		if want[st.Entry.MigrationID] && st.AckedUpstream {
			n++
		}
	}
	return n
}

// doneCount is how many of the seeded migrations have been acknowledged and
// become tombstones.
func doneCount(s *Sidecar, ids []string) int {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	n := 0
	for _, st := range s.CustodySnapshot() {
		if want[st.Entry.MigrationID] && st.Status == journal.StatusDone {
			n++
		}
	}
	return n
}

// spawnedOf is how many of the seeded organisms actually reached the far world.
// It is the "delayed, never dropped" half of the bar: a pacer that shortened the
// backlog instead of slowing it would pass every rate assertion and lose
// organisms.
func spawnedOf(w *fakeWorld, ids []string) int {
	n := 0
	for _, id := range ids {
		if w.spawnCount(id) > 0 {
			n++
		}
	}
	return n
}
