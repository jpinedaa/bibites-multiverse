package sidecar

// The slot-6 livelock tests. A morning flood filled slot 6's paced inbound
// backlog, the game stalled ingesting it and missed HEARTBEATs, the sidecar
// closed 4004, and on reconnect it refilled the burst and marked the whole batch
// due-now — so the mod re-ingested, stalled, and closed again, forever, while the
// upstream sender re-forwarded every stuck entry every forwardRetryMs and paid a
// full multi-MiB decode on each duplicate. These tests pin the five sidecar-side
// defences: no burst refill on a same-session reconnect (G1), an escalating
// replay delay (G2), delivery held into a silent mod (G3), dedup before decode
// (G4), and — since §25's B37 replaced G5's exponential backoff with the
// stronger rule it was approximating — NO re-forward at all (G5).
//
// G5 is worth reading as a lesson rather than a fix. The livelock's 203,735
// duplicate deliveries came from a retry loop, B8 bounded that loop, and B37
// removed it: a frame handed to the relay is handed over once. The hammer cannot
// come back, because the code that swung it is gone.

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

func destLiveOf(s *Sidecar, slot int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.destLiveLocked(slot)
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

// TestAForwardedEntryIsNeverReForwarded is G5 as §25's B37 leaves it. A frame
// written to a live relay connection is written ONCE — to a destination that is
// live and silent, to one that has gone dark, and to one that never answers at
// all. There is no cadence to measure any more, which is the point: the retry
// that produced 203,735 duplicate deliveries into the stalled slot 6 no longer
// exists, so no bound on it has to be correct.
//
// The instrument is the relay's own FORWARD_RECEIPT (§6.12, §22 B26): one
// receipt per write, recorded durably in the sender's journal. A second write of
// any kind would put a second receipt there.
func TestAForwardedEntryIsNeverReForwarded(t *testing.T) {
	rl := startRelay(t)
	ids := []string{"peer-a", "peer-b"}
	for i, pos := range []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}} {
		p := pos
		if _, _, err := rl.relay.ReserveSlot(ids[i], &p); err != nil {
			t.Fatalf("ReserveSlot: %v", err)
		}
	}
	cfgA := fastConfig(t, rl, "peer-a")
	// Short enough that several would have fired inside this test, and far short
	// of the forward timeout, so nothing is written off either.
	cfgA.ForwardRetry = 100 * time.Millisecond
	cfgA.ForwardTimeout = time.Hour

	cfgB := fastConfig(t, rl, "peer-b")
	sideB := startSidecar(t, cfgB)
	waitSlot(t, sideB, 2)
	// ackSilent: slot 2 takes the frame and never answers. Under B5 this was the
	// case that re-forwarded forever; it is now the case that waits.
	modB := dialFakeMod(t, fakeModOptions{
		url: sideB.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond, ackMode: ackSilent})

	sideA := startSidecar(t, cfgA)
	waitSlot(t, sideA, 1)
	modA := dialFakeMod(t, fakeModOptions{
		url: sideA.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond})
	waitLane(t, sideA, contracta.EdgeE, 2)

	migrationID := modA.migrateOut(testEntityID, contracta.EdgeE, 0.4)
	waitFor(t, 10*time.Second, "the one forward and its receipt", func() bool {
		st := journalStateOf(sideA, migrationID)
		return st != nil && st.ForwardReceipts >= 1
	})

	// Twenty forwardRetry intervals. Under the old rule this window held four
	// backed-off re-forwards; under B37 it holds none.
	time.Sleep(20 * cfgA.ForwardRetry)
	st := journalStateOf(sideA, migrationID)
	if st == nil {
		t.Fatal("the entry vanished")
	}
	if st.ForwardReceipts != 1 {
		t.Fatalf("forwardReceipts = %d after 20 retry intervals against a LIVE silent "+
			"destination, want exactly 1: a forwarded frame is forwarded once (§25, B37)",
			st.ForwardReceipts)
	}
	if st.Handoff != journal.HandoffSent {
		t.Fatalf("handoff = %q, want sent — custody may have moved and nothing may change that "+
			"but an answer", st.Handoff)
	}

	// And the destination going dark changes nothing. There is no dark cadence
	// to fall back to, because there is no cadence.
	modB.abort()
	_ = sideB.Close()
	waitFor(t, 15*time.Second, "sideA to observe slot 2 dark", func() bool {
		return sideA.RelayConnected() && !destLiveOf(sideA, 2)
	})
	time.Sleep(20 * cfgA.ForwardRetry)
	if st := journalStateOf(sideA, migrationID); st.ForwardReceipts != 1 {
		t.Fatalf("forwardReceipts = %d after the destination went DARK, want 1: a dark "+
			"destination used to be the one the retry kept hammering (§14, B5), and B37 "+
			"removed the retry rather than re-timing it", st.ForwardReceipts)
	}
	if h := handoffOf(sideA, migrationID); h != journal.HandoffSent {
		t.Fatalf("after going dark the entry is %q, want sent — silence is not proof, so it "+
			"is neither re-routed nor brought home", h)
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
