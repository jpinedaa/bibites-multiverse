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
// PUBLISHED CEILING, a live destination to drain it at, and a relay running a
// ceiling of its own choosing rather than the shipped 50. The bar is the one
// WP8's churn story needs — the whole backlog arrives, and NOT ONE 4007.
func TestSlot6RejoinBacklogDrainsWithoutASingleCapacityShed(t *testing.T) {
	// A published ceiling that is NOT the shipped default, so a sidecar that
	// paced itself against a compiled 50 would fail this test.
	const publishedFrames = 40
	// The backlog is deliberately bigger than the ceiling: unpaced, the first
	// tick puts all of it on the wire and the relay's meter sheds on frame 41.
	const backlog = 64

	rl := startRelayWithLimits(t, contractb.Limits{MaxFramesPerSecond: publishedFrames})
	for i, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}} {
		p := pos
		if _, _, err := rl.relay.ReserveSlot([]string{"peer-a", "peer-b"}[i], &p); err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
	}

	// The destination. Its inbound queue is widened because this test is about
	// the SENDER's rate: an inboundQueueMax refusal is a different rule (§6.6
	// step 2) and it would mask the one under test.
	cfgB := fastConfig(t, rl, "peer-b")
	cfgB.InboundQueueMax = 4 * backlog
	sideB := startSidecar(t, cfgB)
	waitSlot(t, sideB, 2)
	worldB := newWorld()
	dialFakeMod(t, fakeModOptions{
		url: sideB.URL(), world: worldB, heartbeat: 200 * time.Millisecond})

	// The peer that was dark: its journal already holds the backlog before its
	// process exists, which is what a rejoin after hours away looks like.
	cfgA := fastConfig(t, rl, "peer-a")
	ids := seedOutboundBacklog(t, cfgA.DataDir, 2, backlog, rl.relay.SessionID())

	sideA := startSidecar(t, cfgA)
	waitSlot(t, sideA, 1)
	waitFor(t, 10*time.Second, "peer-a to see slot 2 live", func() bool { return destLiveOf(sideA, 2) })

	if got := sideA.PacedFramesPerSecond(); got != publishedFrames*sendPaceRateFraction {
		t.Fatalf("peer-a paced itself at %v frames/s against a PUBLISHED %d; want %v — "+
			"the rate must be a fraction of what the relay published, never a constant",
			got, publishedFrames, publishedFrames*sendPaceRateFraction)
	}

	// The drain, watched while it runs. A starved PONG would be as bad as a
	// shed: the relay closes a silent peer with 4004 inside peerTimeoutMs (3 s
	// on this rig), and this drain is longer than that, so a link that is still
	// up at the end is proof that control frames went out THROUGH the backlog
	// rather than behind it.
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
				"the stats PING starved behind the backlog and the relay timed this peer out",
				time.Since(started))
		}
		return doneCount(sideA, ids) == len(ids)
	})
	elapsed := time.Since(started)

	if got := sideA.CapacitySheds(); got != 0 {
		t.Fatalf("the drain took %d capacity sheds (close 4007); the whole point of pacing "+
			"under the published ceiling is that it takes NONE", got)
	}
	if got := spawnedOf(worldB, ids); got != len(ids) {
		t.Fatalf("slot 2 spawned %d of %d organisms; frames are DELAYED by the pacer, never "+
			"dropped", got, len(ids))
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
		"(paced %v/s, %d deferrals, 0 sheds)",
		backlog, elapsed.Round(time.Millisecond), publishedFrames,
		sideA.PacedFramesPerSecond(), sideA.PacedDeferrals())
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
// stats PING, a MIGRATION_ACK that releases somebody else's custody — are not
// queued behind anything and must not have to wait for a backlog.
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

// ------------------------------------------------------------------ helpers

// seedOutboundBacklog writes the journal of a sidecar that forwarded a crowd of
// organisms and then went dark: every entry is InFlight and handed over, under a
// named relay session, exactly as seedSentEntry writes one. It is the state slot
// 6 came back to.
func seedOutboundBacklog(t *testing.T, dataDir string, destSlot, n int, session string) []string {
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
		if _, err := jr.Apply(migrationID, journal.Update{
			Status: journal.StatusInFlight, Handoff: journal.HandoffSent,
			RelaySessionID: &session}); err != nil {
			t.Fatalf("seed handoff %d: %v", i, err)
		}
		ids = append(ids, migrationID)
	}
	if err := jr.Close(); err != nil {
		t.Fatalf("seed journal close: %v", err)
	}
	return ids
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
