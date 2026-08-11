package sidecar

// The slot-6 livelock tests. A morning flood filled slot 6's paced inbound
// backlog, the game stalled ingesting it and missed HEARTBEATs, the sidecar
// closed 4004, and on reconnect it refilled the burst and marked the whole batch
// due-now — so the mod re-ingested, stalled, and closed again, forever, while the
// upstream sender re-forwarded every stuck entry every forwardRetryMs and paid a
// full multi-MiB decode on each duplicate. These tests pin the five sidecar-side
// defences: no burst refill on a same-session reconnect (G1), an escalating
// replay delay (G2), delivery held into a silent mod (G3), dedup before decode
// (G4), and exponential backoff on re-forwards to a LIVE destination (G5).

import (
	"encoding/json"
	"testing"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/wire"
)

// ---------------------------------------------------------------- test helpers

// These reach into the sidecar's in-memory scheduler and pacer, which is exactly
// where the livelock lived: the rule under test is a decision NOT to release
// tokens or frames, and an absence on the wire is also what a merely-slow rig
// produces. Reading the state directly is the honest way to tell them apart.

func pacerTokens(s *Sidecar) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pace.tokens
}

func pacerHaveSim(s *Sidecar) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pace.haveSim
}

func nextDeliverOf(s *Sidecar, id string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sc, ok := s.sched[id]; ok {
		return sc.nextDeliver
	}
	return time.Time{}
}

func nextForwardOf(s *Sidecar, id string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sc, ok := s.sched[id]; ok {
		return sc.nextForward
	}
	return time.Time{}
}

func liveForwardCountOf(s *Sidecar, id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sc, ok := s.sched[id]; ok {
		return sc.liveForwardCount
	}
	return 0
}

func destLiveOf(s *Sidecar, slot int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.destLiveLocked(slot)
}

// resetSchedForward clears one entry's forward schedule so a test can measure a
// backoff run from a known-clean start, after the desired liveness is settled.
func resetSchedForward(s *Sidecar, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sc, ok := s.sched[id]; ok {
		sc.nextForward = time.Time{}
		sc.liveForwardCount = 0
	}
}

func journalStateOf(s *Sidecar, id string) *journal.State {
	for _, st := range s.CustodySnapshot() {
		if st.Entry.MigrationID == id {
			return st
		}
	}
	return nil
}

func approxDur(got, want, tol time.Duration) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d <= tol
}

func mustJSONData(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal test frame: %v", err)
	}
	return b
}

// ---------------------------------------------------------------- G2 formula

// TestReplayBatchDelayFormula pins the escalating-delay schedule of §7.5: a
// first-attempt replay is prompt, and each further generation adds a step up to
// the cap.
func TestReplayBatchDelayFormula(t *testing.T) {
	cases := []struct {
		minAttempt int
		want       time.Duration
	}{
		{0, 0},                       // journaled, never delivered: sidecar-restart recovery, prompt
		{1, 0},                       // delivered once, first replay: prompt
		{2, 500 * time.Millisecond},  // second generation
		{3, time.Second},             // third
		{4, 1500 * time.Millisecond}, // fourth
		{11, 5 * time.Second},        // capped
		{50, 5 * time.Second},        // still capped
	}
	for _, c := range cases {
		if got := replayBatchDelay(c.minAttempt); got != c.want {
			t.Errorf("replayBatchDelay(%d) = %v, want %v", c.minAttempt, got, c.want)
		}
	}
}

// TestBackoffIntervalFormula pins the live-destination re-forward backoff of
// §9.3/B5: doubling from the base, capped, with no overflow at large n.
func TestBackoffIntervalFormula(t *testing.T) {
	const base = 5 * time.Second
	const max = 60 * time.Second
	want := []time.Duration{
		5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second,
		60 * time.Second, 60 * time.Second,
	}
	for n, w := range want {
		if got := backoffInterval(base, max, n); got != w {
			t.Errorf("backoffInterval(5s,60s,%d) = %v, want %v", n, got, w)
		}
	}
	if got := backoffInterval(base, max, 1000); got != max {
		t.Errorf("backoffInterval at a large n = %v, want the cap %v (no overflow)", got, max)
	}
}

// ---------------------------------------------------------------- G1

// TestSameSessionReconnectKeepsThePacingBucket is G1. A socket reconnect under
// the SAME sessionId must NOT refill the burst: simulated time kept advancing,
// the burst was never refunded, and refilling it is what let the un-ACKed replay
// re-flood the mod. A genuinely NEW sessionId is a new world clock and DOES reset
// the bucket — the case the old unconditional reset was written for.
func TestSameSessionReconnectKeepsThePacingBucket(t *testing.T) {
	const burst = 3.0
	const organisms = 8
	g := newGrid(t, 2, gridOptions{
		layout:    layoutRow(2),
		heartbeat: 100 * time.Millisecond,
		tune: func(i int, c *Config) {
			if i == 1 {
				// A rate near zero so no token accrues over the test: the bucket
				// only ever loses tokens, which makes "was it refilled?" decisive.
				c.InboundRatePerSimMinute = 0.001
				c.InboundRateBurst = burst
			}
		},
	})
	a, b := g.node(0), g.node(1)
	b.mod.setAckMode(ackSilent) // journaled, never completed, so a reconnect replays

	ids := make([]string, 0, organisms)
	for i := 0; i < organisms; i++ {
		ids = append(ids, a.mod.migrateOut(int32(-95000-i), contracta.EdgeE, 0.5))
	}
	waitFor(t, 15*time.Second, "slot 2 to journal the whole batch", func() bool {
		n := 0
		for _, id := range ids {
			if custodyOf(b.side, id) != "absent" {
				n++
			}
		}
		return n == organisms
	})
	// Exactly the burst drains; the rest wait on an empty bucket that never refills.
	waitFor(t, 10*time.Second, "the burst to drain the bucket", func() bool {
		return deliveredCount(b.mod, ids) == int(burst)
	})
	time.Sleep(300 * time.Millisecond)
	if got := deliveredCount(b.mod, ids); got != int(burst) {
		t.Fatalf("delivered %d, want exactly the burst of %d (rate is ~0, so nothing else accrues)", got, int(burst))
	}
	if !pacerHaveSim(b.side) {
		t.Fatal("precondition: the pacer should have anchored its simulated clock by now")
	}

	// Same-session reconnect: the same fakeWorld carries the same sessionId.
	b.mod.abort()
	modB2 := dialFakeMod(t, fakeModOptions{
		url: b.side.URL(), world: b.world, heartbeat: 100 * time.Millisecond, ackMode: ackSilent})
	modB2.waitTypeStatus(5 * time.Second)

	if !pacerHaveSim(b.side) {
		t.Fatal("a same-session reconnect cleared the pacer's simulated clock; G1 keeps it")
	}
	if got := pacerTokens(b.side); got >= 1 {
		t.Fatalf("the pacing bucket refilled to %.2f tokens on a same-session reconnect; the burst must not be refunded", got)
	}
	// The replay of an empty bucket releases nothing: no burst.
	time.Sleep(500 * time.Millisecond)
	if got := deliveredCount(modB2, ids); got != 0 {
		t.Fatalf("a same-session reconnect released %d MIGRATE_IN; with an empty bucket only a refill could burst", got)
	}

	// The contrast: a genuinely NEW sessionId IS a new world clock and refills the
	// burst, so the reset still fires when it should.
	modB2.abort()
	modC := dialFakeMod(t, fakeModOptions{
		url: b.side.URL(), world: newWorld(), heartbeat: 100 * time.Millisecond, ackMode: ackSilent})
	modC.waitTypeStatus(5 * time.Second)
	waitFor(t, 10*time.Second, "a new-session reconnect to refill the burst and replay it", func() bool {
		return deliveredCount(modC, ids) >= int(burst)
	})
}

// ---------------------------------------------------------------- G2

// TestReplayDelayEscalatesAndDrains is G2. Across repeated heartbeat-timeout
// reconnects the un-ACKed batch is replayed behind a delay that escalates with
// its delivery generation, instead of the old mark-all-due-now that let the mod
// re-ingest and stall again at once. Once the mod starts ACKing, the batch drains.
func TestReplayDelayEscalatesAndDrains(t *testing.T) {
	const organisms = 4
	g := newGrid(t, 2, gridOptions{
		layout:    layoutRow(2),
		heartbeat: 100 * time.Millisecond,
		tune: func(i int, c *Config) {
			if i == 1 {
				c.HeartbeatTimeout = 300 * time.Millisecond // a quick 4004 per generation
			}
		},
	})
	a, b := g.node(0), g.node(1)
	b.mod.setAckMode(ackSilent)

	ids := make([]string, 0, organisms)
	for i := 0; i < organisms; i++ {
		ids = append(ids, a.mod.migrateOut(int32(-94000-i), contracta.EdgeE, 0.5))
	}
	waitFor(t, 15*time.Second, "the batch to be delivered once", func() bool {
		for _, id := range ids {
			st := journalStateOf(b.side, id)
			if st == nil || st.Attempt < 1 {
				return false
			}
		}
		return true
	})

	// Each generation: force a 4004, reconnect the SAME session, and time how long
	// the batch waits before it re-delivers. The wait grows by ~500 ms each round.
	elapsed := make([]time.Duration, 0, 3)
	prevAttempt := 1
	for gen := 0; gen < 3; gen++ {
		b.mod.stopHeartbeats()
		if code := b.mod.waitClosed(5 * time.Second); int(code) != contracta.CloseHeartbeatTimeout {
			t.Fatalf("generation %d: close code %d, want 4004", gen, int(code))
		}
		target := prevAttempt + 1
		t0 := time.Now()
		b.mod = dialFakeMod(t, fakeModOptions{
			url: b.side.URL(), world: b.world, heartbeat: 100 * time.Millisecond, ackMode: ackSilent})
		waitFor(t, 20*time.Second, "the whole batch to re-deliver on this generation", func() bool {
			for _, id := range ids {
				st := journalStateOf(b.side, id)
				if st == nil || st.Attempt < target {
					return false
				}
			}
			return true
		})
		elapsed = append(elapsed, time.Since(t0))
		prevAttempt = target
	}
	// Prompt first replay, then an escalating hold.
	if elapsed[1] <= elapsed[0]+250*time.Millisecond {
		t.Fatalf("replay delay did not escalate: gen0=%v gen1=%v (each generation must back off ~500ms more)",
			elapsed[0], elapsed[1])
	}
	if elapsed[2] <= elapsed[1]+250*time.Millisecond {
		t.Fatalf("replay delay did not keep escalating: gen1=%v gen2=%v", elapsed[1], elapsed[2])
	}

	// Once the mod ACKs again, the whole batch drains — the delay slows a livelock,
	// it never drops an organism.
	b.mod.stopHeartbeats()
	b.mod.waitClosed(5 * time.Second)
	b.mod = dialFakeMod(t, fakeModOptions{
		url: b.side.URL(), world: b.world, heartbeat: 100 * time.Millisecond}) // ackNormal
	waitFor(t, 25*time.Second, "the batch to drain once the mod ACKs", func() bool {
		for _, id := range ids {
			if custodyOf(b.side, id) != "in/done" {
				return false
			}
		}
		return true
	})
}

// ---------------------------------------------------------------- G3

// TestDeliveryPausesIntoASilentMod is G3. When a mod goes quiet at the
// application layer, delivery is held after the freshness grace and before the
// 4004 close — the window the old code kept releasing frames into a drowning main
// thread. Delivery resumes, with the same entries, once heartbeats return.
func TestDeliveryPausesIntoASilentMod(t *testing.T) {
	g := newGrid(t, 2, gridOptions{
		layout:    layoutRow(2),
		heartbeat: 100 * time.Millisecond,
		tune: func(i int, c *Config) {
			if i == 1 {
				c.HeartbeatDeliveryGrace = 300 * time.Millisecond
				c.HeartbeatTimeout = 5 * time.Second // far beyond the gate, so the socket stays open
			}
		},
	})
	a, b := g.node(0), g.node(1)

	// A fresh mod delivers normally.
	warm := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "the warm-up delivery to land", func() bool {
		return b.world.spawnCount(warm) == 1
	})

	// The mod goes app-silent. After the freshness grace, delivery is gated.
	b.mod.suppressHeartbeats(true)
	time.Sleep(600 * time.Millisecond) // > grace (300ms), << timeout (5s)

	ids := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		ids = append(ids, a.mod.migrateOut(int32(-96000-i), contracta.EdgeE, 0.5))
	}
	waitFor(t, 10*time.Second, "b to take custody of the gated batch", func() bool {
		n := 0
		for _, id := range ids {
			if custodyOf(b.side, id) != "absent" {
				n++
			}
		}
		return n == len(ids)
	})
	// Custody was taken (§7.5 forbids dropping) but NOTHING is released into the
	// silent mod, and the socket is still open — this is the window before 4004.
	time.Sleep(500 * time.Millisecond)
	if got := deliveredCount(b.mod, ids); got != 0 {
		t.Fatalf("%d MIGRATE_IN were released into an app-silent mod; the freshness gate must hold them", got)
	}
	if b.mod.isClosed() {
		t.Fatal("the mod socket closed before the test could observe the gated window")
	}

	// Heartbeats resume: delivery resumes, the same entries, nothing dropped.
	b.mod.suppressHeartbeats(false)
	waitFor(t, 10*time.Second, "delivery to resume once heartbeats return", func() bool {
		n := 0
		for _, id := range ids {
			if b.world.spawnCount(id) == 1 {
				n++
			}
		}
		return n == len(ids)
	})
}

// ---------------------------------------------------------------- G5

// TestLiveDestinationReforwardBacksOff is G5. A re-forward to a LIVE destination
// that has not answered climbs an exponential backoff (a live peer with a deep
// paced backlog is slow, not gone) up to the cap, instead of hammering every
// forwardRetryMs — the shape that produced 203 735 duplicate deliveries. A DARK
// destination is exempt: the dark branch resets the backoff so the retry keeps
// its own cadence and the hold clock is untouched (§9.3, B5).
func TestLiveDestinationReforwardBacksOff(t *testing.T) {
	clock := newFakeClock()
	rl := startRelay(t)
	ids := []string{"peer-a", "peer-b"}
	for i, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}} {
		p := pos
		if _, _, err := rl.relay.ReserveSlot(ids[i], &p); err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
	}
	cfgA := fastConfig(t, rl, "peer-a")
	cfgA.Clock = clock.Now
	cfgA.HeartbeatTimeout = time.Hour // fake-clock jumps must not read as a silent mod
	cfgA.ForwardRetry = time.Second
	cfgA.ForwardRetryMax = 4 * time.Second

	cfgB := fastConfig(t, rl, "peer-b")
	sideB := startSidecar(t, cfgB)
	waitSlot(t, sideB, 2)
	modB := dialFakeMod(t, fakeModOptions{
		url: sideB.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond, ackMode: ackSilent})

	migrationID := seedSentEntry(t, cfgA.DataDir, 2, rl.relay.SessionID())

	sideA := startSidecar(t, cfgA)
	waitSlot(t, sideA, 1)
	dialFakeMod(t, fakeModOptions{url: sideA.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond})
	waitLane(t, sideA, contracta.EdgeE, 2)
	waitFor(t, 10*time.Second, "sideA to see slot 2 live", func() bool { return destLiveOf(sideA, 2) })

	// Measure a clean backoff run: 1s, 2s, 4s, then the cap.
	resetSchedForward(sideA, migrationID)
	base := clock.Now()
	waitFor(t, 10*time.Second, "the first live re-forward", func() bool {
		return nextForwardOf(sideA, migrationID).After(base)
	})
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second}
	prev := base
	for k, w := range want {
		nf := nextForwardOf(sideA, migrationID)
		if got := nf.Sub(prev); !approxDur(got, w, 100*time.Millisecond) {
			t.Fatalf("live re-forward interval %d = %v, want ~%v (5→10→20→cap, scaled)", k, got, w)
		}
		prev = nf
		if k < len(want)-1 {
			clock.Advance(nf.Sub(clock.Now())) // step to the scheduled re-forward
			target := nf
			waitFor(t, 10*time.Second, "the next live re-forward", func() bool {
				return nextForwardOf(sideA, migrationID).After(target)
			})
		}
	}
	if liveForwardCountOf(sideA, migrationID) == 0 {
		t.Fatal("the live-forward backoff counter never advanced")
	}

	// Dark destination: killing the peer must switch the backoff OFF. The dark
	// branch resets the counter, so the retry keeps its own cadence (§9.3, B5).
	modB.abort()
	_ = sideB.Close()
	waitFor(t, 15*time.Second, "sideA to observe slot 2 dark", func() bool {
		return sideA.RelayConnected() && !destLiveOf(sideA, 2)
	})
	waitFor(t, 10*time.Second, "the dark branch to reset the live-forward backoff", func() bool {
		return liveForwardCountOf(sideA, migrationID) == 0
	})
	if h := handoffOf(sideA, migrationID); h != journal.HandoffHeld && h != journal.HandoffSent {
		t.Fatalf("after going dark the entry is %q, want held/sent — a dark destination is held, not re-routed", h)
	}
}

// ---------------------------------------------------------------- G4

// TestDedupPrecedesTheBodyDecode is G4. A journaled duplicate is recognised from
// a minimal header decode and answered with nothing, WITHOUT the full body
// decode — proven by a body that is structurally unparseable garbage, which a
// full decode would reject. A NEW id carrying the same garbage still fails
// validation exactly as before.
func TestDedupPrecedesTheBodyDecode(t *testing.T) {
	spy := newLogSpy(t)
	rl := startRelay(t)
	cfgB := fastConfig(t, rl, "peer-b")
	cfgB.Logger = spy.logger()
	sideB := startSidecar(t, cfgB)
	waitSlot(t, sideB, 1)
	// ackSilent so the journaled entry stays un-tombstoned: the "journaled, not
	// yet delivered/ACKed" duplicate case, which is answered with nothing.
	dialFakeMod(t, fakeModOptions{
		url: sideB.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond, ackMode: ackSilent})

	// Journal one valid inbound organism by feeding a well-formed MIGRATION_PAYLOAD.
	dupID := wire.NewUUID()
	valid := contractb.MigrationPayload{
		MigrationID:  dupID,
		Kind:         contracta.KindBibite,
		Body:         contractb.Body{Version: "0.6.3.1", BB8: makePayload(testEntityID)},
		Lineage:      contractb.Lineage{Parents: []contractb.Parent{}},
		SourcePeer:   "peer-remote",
		SourceSlot:   2,
		DestSlot:     1,
		ExitEdge:     contracta.EdgeE,
		ExitPosition: 0.5,
		EntityID:     testEntityID,
		Timestamp:    time.Now().UnixMilli(),
	}
	sideB.onMigrationPayload(wire.Envelope{
		Protocol: wire.ProtocolB, Type: contractb.TypeMigrationPayload, Data: mustJSONData(t, valid)})
	waitFor(t, 5*time.Second, "the valid payload to be journaled", func() bool {
		return custodyOf(sideB, dupID) != "absent"
	})

	// A DUPLICATE whose body is garbage (bb8 is a number, not a string): a full
	// decode would error, so a success here proves the decode was skipped.
	garbage := func(id string) json.RawMessage {
		return json.RawMessage(`{"migrationId":"` + id + `","kind":"bibite","sourcePeer":"peer-remote",` +
			`"destSlot":1,"exitEdge":"E","exitPosition":0.5,"body":{"version":"0.6.3.1","bb8":98765}}`)
	}
	before := spy.count("duplicate MIGRATION_PAYLOAD")
	if !sideB.onMigrationPayload(wire.Envelope{
		Protocol: wire.ProtocolB, Type: contractb.TypeMigrationPayload, Data: garbage(dupID)}) {
		t.Fatal("onMigrationPayload returned false for a duplicate")
	}
	if got := spy.count("duplicate MIGRATION_PAYLOAD"); got <= before {
		t.Fatal("a garbage-body duplicate was not recognised; the full body decode was not skipped")
	}
	if got := countJournalEntries(sideB, dupID); got != 1 {
		t.Fatalf("the duplicate created %d journal entries, want 1 (it must deliver nothing)", got)
	}

	// A NEW id carrying the SAME garbage still fails validation exactly as before:
	// nothing is journaled, and the malformed branch runs (no destPeer, so no NACK
	// goes out — the pre-existing behaviour).
	newID := wire.NewUUID()
	beforeDup := spy.count("duplicate MIGRATION_PAYLOAD")
	beforeMalformed := spy.count("cannot NACK, no sourcePeer")
	sideB.onMigrationPayload(wire.Envelope{
		Protocol: wire.ProtocolB, Type: contractb.TypeMigrationPayload, Data: garbage(newID)})
	if custodyOf(sideB, newID) != "absent" {
		t.Fatal("a new garbage payload was journaled; the full decode must reject it")
	}
	if got := spy.count("duplicate MIGRATION_PAYLOAD"); got != beforeDup {
		t.Fatal("a new id was mistaken for a duplicate")
	}
	if got := spy.count("cannot NACK, no sourcePeer"); got <= beforeMalformed {
		t.Fatal("a new garbage payload did not take the malformed branch; it must fail validation as before")
	}
}
