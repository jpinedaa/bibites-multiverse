package sidecar

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

const refusalTestSession = "5f0b9c31-77ad-4e26-9a4c-1b83d206ef95"

type refusalHarness struct {
	side  *Sidecar
	cfg   Config
	clock *fakeClock
}

func newRefusalHarness(t *testing.T, slots int) *refusalHarness {
	t.Helper()
	clock := newFakeClock()
	cfg := DefaultConfig()
	cfg.Listen = "127.0.0.1:0"
	cfg.DataDir = t.TempDir()
	cfg.PeerID = "peer-source"
	cfg.ContractAToken = testContractAToken
	cfg.Clock = clock.Now
	cfg.Logger = testLogger(t)
	side, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = side.Close() })
	setRefusalMap(side, slots)
	return &refusalHarness{side: side, cfg: cfg, clock: clock}
}

func setRefusalMap(side *Sidecar, count int) {
	slots := make([]contractb.SlotInfo, 0, count)
	for i := 0; i < count; i++ {
		slots = append(slots, contractb.SlotInfo{
			Slot:           i + 1,
			Position:       contractb.Position{Col: i, Row: 0},
			PeerID:         "peer-" + string(rune('a'+i)),
			Live:           true,
			ModConnected:   true,
			GameVersion:    "0.6.3.1",
			SimulationSize: 2000,
		})
	}
	side.mu.Lock()
	side.slot = 1
	side.position = contractb.Position{Col: 0, Row: 0}
	side.mapShape = contractb.MapShape{Width: count, Height: 1}
	side.status = contractb.PeerStatus{
		Map: side.mapShape, SlotCount: count, Slots: slots,
	}
	side.relayReady = true
	side.relaySessionID = refusalTestSession
	if count > 1 {
		side.neighbours[contracta.EdgeE] = &contractb.Neighbour{Slot: 2}
	}
	side.mu.Unlock()
}

func seedRefusalEntry(t *testing.T, h *refusalHarness, dest int) string {
	t.Helper()
	id := wire.NewUUID()
	now := h.clock.Now().UnixMilli()
	if _, err := h.side.jr.Create(journal.Out, journal.Entry{
		MigrationID: id,
		EntityID:    int32(dest),
		Kind:        contracta.KindBibite,
		GameVersion: "0.6.3.1",
		Payload:     `{"body":"unchanged"}`,
		PayloadHash: "payload-hash",
		Edge:        contracta.EdgeE,
		Position:    0.5,
		VelocityX:   4,
		VelocityY:   -2,
		Heading:     90,
		SourceSlot:  1,
		DestSlot:    dest,
		JournaledAt: now,
	}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.side.jr.Apply(id, journal.Update{
		Status:         journal.StatusInFlight,
		Handoff:        journal.HandoffSent,
		RelaySessionID: stringPointer(refusalTestSession),
		SentAtMs:       &now,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func stringPointer(v string) *string { return &v }

func markAlternateSent(t *testing.T, h *refusalHarness, id string) {
	t.Helper()
	now := h.clock.Now().UnixMilli()
	if _, err := h.side.jr.Apply(id, journal.Update{
		Status:         journal.StatusInFlight,
		Handoff:        journal.HandoffSent,
		RelaySessionID: stringPointer(refusalTestSession),
		SentAtMs:       &now,
	}); err != nil {
		t.Fatal(err)
	}
}

func refusalNackEnvelope(t *testing.T, id, code, session string, never *bool,
	attempt *contractb.MigrationAttempt) wire.Envelope {
	t.Helper()
	data, err := json.Marshal(contractb.MigrationNack{
		MigrationID:    id,
		DestPeer:       "peer-source",
		Code:           code,
		Class:          contractb.ClassOf(code),
		Message:        "the destination transport queue is full",
		RetryAfterMs:   15000,
		NeverForwarded: never,
		RelaySessionID: session,
		RefusedAttempt: attempt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return wire.Envelope{Data: data}
}

func sendExactTransportRefusal(t *testing.T, h *refusalHarness, id string) {
	t.Helper()
	st := journalEntry(t, h.side, id)
	attempt := &contractb.MigrationAttempt{
		DestSlot: st.Entry.DestSlot, RerouteCount: st.RerouteCount,
	}
	if !h.side.onMigrationNack(refusalNackEnvelope(t, id, contractb.NackNotForwarded,
		st.RelaySessionID, contractb.BoolPtr(true), attempt)) {
		t.Fatal("the sidecar did not consume the transport refusal")
	}
}

func sendPeerRefusal(t *testing.T, h *refusalHarness, id string) {
	t.Helper()
	st := journalEntry(t, h.side, id)
	sourcePeer := ""
	for _, slot := range h.side.status.Slots {
		if slot.Slot == st.Entry.DestSlot {
			sourcePeer = slot.PeerID
			break
		}
	}
	if sourcePeer == "" {
		t.Fatal("current destination has no peer identity")
	}
	data, err := json.Marshal(contractb.MigrationNack{
		MigrationID: id,
		SourcePeer:  sourcePeer,
		DestPeer:    "peer-source",
		Code:        contractb.NackOverloaded,
		Class:       contractb.ClassTransient,
		Message:     "test destination refused before custody",
	})
	if err != nil {
		t.Fatal(err)
	}
	h.side.onMigrationNack(wire.Envelope{Data: data})
}

func recordReceipt(t *testing.T, h *refusalHarness, id string, dest int) {
	t.Helper()
	st := journalEntry(t, h.side, id)
	count := st.ForwardReceipts + 1
	at := h.clock.Now().UnixMilli()
	if _, err := h.side.jr.Apply(id, journal.Update{
		ForwardReceipts:      &count,
		ReceiptSessionID:     stringPointer(st.RelaySessionID),
		ReceiptDestSlot:      &dest,
		ReceiptForwardedAtMs: &at,
	}); err != nil {
		t.Fatal(err)
	}
}

func completeLocalBounce(t *testing.T, side *Sidecar, id string, entityID int32) {
	t.Helper()
	duplicate := false
	tick := int64(1)
	data, err := json.Marshal(contracta.MigrateInAck{
		MigrationID: id,
		EntityID:    &entityID,
		Duplicate:   &duplicate,
		SimTick:     &tick,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !side.onMigrateInAck(&modSession{}, wire.Envelope{Data: data}) {
		t.Fatal("the sidecar did not consume the local bounce acknowledgement")
	}
}

func openRelayWriter(t *testing.T, stub *stubRelay) *wsutil.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, stub.url(), &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	conn := wsutil.New(ws, 1)
	t.Cleanup(conn.CloseNow)
	return conn
}

func closedRelayWriter(t *testing.T) *wsutil.Conn {
	t.Helper()
	conn := openRelayWriter(t, newStubRelay(t))
	conn.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case <-conn.Done():
	case <-ctx.Done():
		t.Fatal("the closed relay writer did not stop")
	}
	return conn
}

// TestSixtyFourTransportRefusalsUseTheAlternateThenBounceOnce models a source
// whose full outbound custody set hits full transport queues. Every migration
// tries the distinct same-axis alternate. If that queue also refuses it, the
// migration comes home once and cannot be duplicated by a repeated NACK.
func TestSixtyFourTransportRefusalsUseTheAlternateThenBounceOnce(t *testing.T) {
	h := newRefusalHarness(t, 3)
	const count = 64
	ids := make([]string, 0, count)
	deadlines := make(map[string]int64, count)
	for i := 0; i < count; i++ {
		id := seedRefusalEntry(t, h, 2)
		ids = append(ids, id)
		sendExactTransportRefusal(t, h, id)
		st := journalEntry(t, h.side, id)
		if st.Entry.DestSlot != 3 || st.Handoff != journal.HandoffPending {
			t.Fatalf("entry %d progress = dest %d handoff %q, want alternate 3 pending",
				i, st.Entry.DestSlot, st.Handoff)
		}
		if !reflect.DeepEqual(st.RefusedSlots, []int{2}) {
			t.Fatalf("entry %d refused slots = %v, want [2]", i, st.RefusedSlots)
		}
		if st.RefusalDeadlineMs == 0 {
			t.Fatalf("entry %d has no durable first-refusal deadline", i)
		}
		deadlines[id] = st.RefusalDeadlineMs
	}

	for _, id := range ids {
		markAlternateSent(t, h, id)
		sendExactTransportRefusal(t, h, id)
		st := journalEntry(t, h.side, id)
		if st.Direction != journal.In || !st.BounceBack || st.Status != journal.StatusOpen {
			t.Fatalf("exhausted entry = %s/%s bounce=%v, want in/open bounce",
				st.Direction, st.Status, st.BounceBack)
		}
		if st.Entry.Edge != contracta.EdgeE || st.Entry.Payload != `{"body":"unchanged"}` {
			t.Fatal("the bounded walk changed the migration axis or body")
		}
		if st.RefusalDeadlineMs != deadlines[id] {
			t.Fatalf("deadline reset from %d to %d", deadlines[id], st.RefusalDeadlineMs)
		}
		if !reflect.DeepEqual(st.RefusedSlots, []int{2, 3}) {
			t.Fatalf("final refused slots = %v, want [2 3]", st.RefusedSlots)
		}

		// A delayed duplicate of the terminal NACK cannot create another entry or
		// another bounce. The direction guard consumes it without mutation.
		sendExactTransportRefusal(t, h, id)
	}
	if got := h.side.jr.CountPending(journal.In); got != count {
		t.Fatalf("inbound bounce depth = %d, want %d unique migrations", got, count)
	}
	if got := h.side.jr.CountPending(journal.Out); got != 0 {
		t.Fatalf("outbound custody depth stayed at %d after all queues refused, want 0", got)
	}
	if got := len(h.side.jr.List()); got != count {
		t.Fatalf("journal entries = %d, want %d; a bounce duplicated a record", got, count)
	}

	// A valid local MIGRATE_IN_ACK completes each bounce. This is the aggregate
	// condition that releases a source stuck at the 64-entry custody cap.
	for _, id := range ids {
		completeLocalBounce(t, h.side, id, 2)
	}
	if got := h.side.jr.CountPending(journal.Out) + h.side.jr.CountPending(journal.In); got != 0 {
		t.Fatalf("custody depth after local bounce completion = %d, want below the 64-entry cap", got)
	}
}

func TestTransportRefusalProofSafetyMatrix(t *testing.T) {
	otherSession := "c8177a34-90bd-4f51-8e02-4d6b915ca7e3"
	cases := []struct {
		name            string
		code            string
		never           *bool
		session         string
		attempt         *contractb.MigrationAttempt
		receipt         bool
		wantBoundedWalk bool
	}{
		{"exact proof", contractb.NackNotForwarded, contractb.BoolPtr(true),
			refusalTestSession, &contractb.MigrationAttempt{DestSlot: 2}, false, true},
		{"migration-wide false but exact attempt", contractb.NackNotForwarded, contractb.BoolPtr(false),
			refusalTestSession, &contractb.MigrationAttempt{DestSlot: 2}, false, true},
		{"missing neverForwarded", contractb.NackNotForwarded, nil,
			refusalTestSession, &contractb.MigrationAttempt{DestSlot: 2}, false, false},
		{"missing attempt from older relay", contractb.NackNotForwarded, contractb.BoolPtr(true),
			refusalTestSession, nil, false, false},
		{"stale destination attempt", contractb.NackNotForwarded, contractb.BoolPtr(true),
			refusalTestSession, &contractb.MigrationAttempt{DestSlot: 3}, false, false},
		{"stale reroute count", contractb.NackNotForwarded, contractb.BoolPtr(true),
			refusalTestSession, &contractb.MigrationAttempt{DestSlot: 2, RerouteCount: 1}, false, false},
		{"different relay session", contractb.NackNotForwarded, contractb.BoolPtr(true),
			otherSession, &contractb.MigrationAttempt{DestSlot: 2}, false, false},
		{"contradictory same-attempt receipt", contractb.NackNotForwarded, contractb.BoolPtr(true),
			refusalTestSession, &contractb.MigrationAttempt{DestSlot: 2}, true, false},
		{"relay drain has no attempt proof", contractb.NackNotForwarded, contractb.BoolPtr(true),
			refusalTestSession, nil, false, false},
		{"different relay code", contractb.NackPeerOffline, contractb.BoolPtr(true),
			refusalTestSession, &contractb.MigrationAttempt{DestSlot: 2}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newRefusalHarness(t, 3)
			id := seedRefusalEntry(t, h, 2)
			if tc.receipt {
				one, dest := 1, 2
				at := h.clock.Now().UnixMilli()
				if _, err := h.side.jr.Apply(id, journal.Update{
					ForwardReceipts:      &one,
					ReceiptSessionID:     stringPointer(refusalTestSession),
					ReceiptDestSlot:      &dest,
					ReceiptForwardedAtMs: &at,
				}); err != nil {
					t.Fatal(err)
				}
			}
			h.side.onMigrationNack(refusalNackEnvelope(
				t, id, tc.code, tc.session, tc.never, tc.attempt))
			st := journalEntry(t, h.side, id)
			gotBoundedWalk := st.RefusalDeadlineMs != 0 || len(st.RefusedSlots) != 0
			if gotBoundedWalk != tc.wantBoundedWalk {
				t.Fatalf("bounded transport walk = %v, want %v; state = %+v",
					gotBoundedWalk, tc.wantBoundedWalk, st)
			}
			if tc.wantBoundedWalk && (st.Entry.DestSlot != 3 || st.RerouteCount != 1) {
				t.Fatalf("exact proof progress = dest %d reroutes %d, want 3 and 1",
					st.Entry.DestSlot, st.RerouteCount)
			}
			if tc.receipt && st.Handoff != journal.HandoffSent {
				t.Fatalf("a contradicted proof changed handoff to %q", st.Handoff)
			}
		})
	}
}

func TestAttemptProofProgressesAcrossMixedPeerAndTransportRefusals(t *testing.T) {
	t.Run("peer then transport", func(t *testing.T) {
		h := newRefusalHarness(t, 4)
		id := seedRefusalEntry(t, h, 2)
		recordReceipt(t, h, id, 2)
		sendPeerRefusal(t, h, id)
		if st := journalEntry(t, h.side, id); st.Entry.DestSlot != 3 || st.RerouteCount != 1 {
			t.Fatalf("peer refusal progress = %+v, want destination 3 reroute 1", st)
		}
		markAlternateSent(t, h, id)
		st := journalEntry(t, h.side, id)
		attempt := &contractb.MigrationAttempt{DestSlot: 3, RerouteCount: 1}
		h.side.onMigrationNack(refusalNackEnvelope(t, id, contractb.NackNotForwarded,
			st.RelaySessionID, contractb.BoolPtr(false), attempt))
		got := journalEntry(t, h.side, id)
		if got.Entry.DestSlot != 4 || got.RerouteCount != 2 || got.RefusalDeadlineMs == 0 ||
			!reflect.DeepEqual(got.RefusedSlots, []int{2, 3}) {
			t.Fatalf("peer->transport progress = %+v, want distinct destination 4", got)
		}
	})

	t.Run("transport then peer then transport", func(t *testing.T) {
		h := newRefusalHarness(t, 5)
		id := seedRefusalEntry(t, h, 2)
		seenAttempts := map[[2]int]bool{}
		remember := func(dest, count int) {
			key := [2]int{dest, count}
			if seenAttempts[key] {
				t.Fatalf("bounded refusal chain repeated attempt %v", key)
			}
			seenAttempts[key] = true
		}

		remember(2, 0)
		sendExactTransportRefusal(t, h, id)
		markAlternateSent(t, h, id)
		recordReceipt(t, h, id, 3)
		sendPeerRefusal(t, h, id)
		middle := journalEntry(t, h.side, id)
		if middle.Entry.DestSlot != 4 || middle.RerouteCount != 2 {
			t.Fatalf("transport->peer progress = %+v, want destination 4 reroute 2", middle)
		}
		remember(3, 1)
		markAlternateSent(t, h, id)
		current := journalEntry(t, h.side, id)
		remember(current.Entry.DestSlot, current.RerouteCount)
		attempt := &contractb.MigrationAttempt{
			DestSlot: current.Entry.DestSlot, RerouteCount: current.RerouteCount,
		}
		h.side.onMigrationNack(refusalNackEnvelope(t, id, contractb.NackNotForwarded,
			current.RelaySessionID, contractb.BoolPtr(false), attempt))
		got := journalEntry(t, h.side, id)
		if got.Entry.DestSlot != 5 || got.RerouteCount != 3 ||
			!reflect.DeepEqual(got.RefusedSlots, []int{2, 3, 4}) {
			t.Fatalf("transport->peer->transport progress = %+v, want destination 5", got)
		}
	})
}

func TestMaxReroutesBoundsTransportRefusalProgress(t *testing.T) {
	h := newRefusalHarness(t, 4)
	h.side.cfg.MaxReroutes = 1
	h.cfg.MaxReroutes = 1
	id := seedRefusalEntry(t, h, 2)
	sendExactTransportRefusal(t, h, id)
	if st := journalEntry(t, h.side, id); st.Entry.DestSlot != 3 || st.RerouteCount != 1 {
		t.Fatalf("first progress = dest %d reroutes %d, want 3 and 1",
			st.Entry.DestSlot, st.RerouteCount)
	}
	markAlternateSent(t, h, id)
	if err := h.side.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(h.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	setRefusalMap(reopened, 4)
	h.side = reopened
	sendExactTransportRefusal(t, h, id)
	st := journalEntry(t, h.side, id)
	if st.Direction != journal.In || !st.BounceBack {
		t.Fatalf("maxReroutes did not bounce the safe entry: %+v", st)
	}
	if st.Entry.DestSlot == 4 {
		t.Fatal("the entry exceeded maxReroutes and reached slot 4")
	}
}

func TestRerouteClearsAttemptClockAndRestampsRotatedRelaySession(t *testing.T) {
	h := newRefusalHarness(t, 3)
	id := seedRefusalEntry(t, h, 2)
	sendExactTransportRefusal(t, h, id)
	pending := journalEntry(t, h.side, id)
	if pending.RelaySessionID != "" || pending.SentAtMs != 0 {
		t.Fatalf("pending alternate retained prior attempt metadata: session=%q sentAt=%d",
			pending.RelaySessionID, pending.SentAtMs)
	}
	if _, _, err := h.side.jr.Compact(); err != nil {
		t.Fatal(err)
	}
	if err := h.side.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(h.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	setRefusalMap(reopened, 3)
	replayed := journalEntry(t, reopened, id)
	if replayed.RelaySessionID != "" || replayed.SentAtMs != 0 ||
		replayed.Handoff != journal.HandoffPending {
		t.Fatalf("replayed alternate metadata = session %q sentAt %d handoff %q",
			replayed.RelaySessionID, replayed.SentAtMs, replayed.Handoff)
	}

	rotated := "a3cdb4a8-4eeb-474e-9b27-aa2cb92363f9"
	stub := newStubRelay(t)
	reopened.mu.Lock()
	reopened.relayConn = openRelayWriter(t, stub)
	reopened.relayReady = true
	reopened.relaySessionID = rotated
	reopened.sendPace.acked = true
	if !reopened.forwardLocked(replayed, h.clock.Now()) {
		reopened.mu.Unlock()
		t.Fatal("the pending alternate was not enqueued under the rotated session")
	}
	reopened.mu.Unlock()
	sent := journalEntry(t, reopened, id)
	if sent.RelaySessionID != rotated || sent.SentAtMs == 0 || sent.Handoff != journal.HandoffSent {
		t.Fatalf("alternate attempt metadata = session %q sentAt %d handoff %q",
			sent.RelaySessionID, sent.SentAtMs, sent.Handoff)
	}

	// A duplicate NACK for the prior destination/session is stale after the
	// alternate is committed. It cannot consume another destination or bounce.
	oldAttempt := &contractb.MigrationAttempt{DestSlot: 2, RerouteCount: 0}
	reopened.onMigrationNack(refusalNackEnvelope(t, id, contractb.NackNotForwarded,
		refusalTestSession, contractb.BoolPtr(true), oldAttempt))
	after := journalEntry(t, reopened, id)
	if after.Handoff != journal.HandoffSent || after.Entry.DestSlot != 3 ||
		after.RerouteCount != 1 || after.RelaySessionID != rotated {
		t.Fatalf("stale prior-attempt NACK moved the committed alternate: %+v", after)
	}
	peerData, err := json.Marshal(contractb.MigrationNack{
		MigrationID: id, SourcePeer: "peer-b", DestPeer: "peer-source",
		Code: contractb.NackOverloaded, Class: contractb.ClassTransient,
		Message: "duplicate peer refusal from the prior destination",
	})
	if err != nil {
		t.Fatal(err)
	}
	reopened.onMigrationNack(wire.Envelope{Data: peerData})
	after = journalEntry(t, reopened, id)
	if after.Handoff != journal.HandoffSent || after.Entry.DestSlot != 3 ||
		after.RerouteCount != 1 || after.RelaySessionID != rotated {
		t.Fatalf("stale peer NACK moved the committed alternate: %+v", after)
	}
}

func TestRefusalDeadlineIsRecheckedAfterForwardPreparation(t *testing.T) {
	h := newRefusalHarness(t, 3)
	id := seedRefusalEntry(t, h, 2)
	sendExactTransportRefusal(t, h, id)
	pending := journalEntry(t, h.side, id)
	stub := newStubRelay(t)

	h.side.mu.Lock()
	h.side.relayConn = openRelayWriter(t, stub)
	h.side.relayReady = true
	h.side.sendPace.acked = true
	h.side.afterForwardPrepare = func() {
		h.clock.Advance(h.cfg.BounceTimeout + time.Millisecond)
	}
	forwarded := h.side.forwardLocked(pending, h.clock.Now())
	h.side.mu.Unlock()
	if forwarded {
		t.Fatal("an alternate crossed the first-refusal deadline and was enqueued")
	}
	st := journalEntry(t, h.side, id)
	if st.Direction != journal.In || !st.BounceBack || st.Handoff == journal.HandoffSent {
		t.Fatalf("post-preparation deadline state = %+v, want one safe bounce", st)
	}
	time.Sleep(50 * time.Millisecond)
	stub.mu.Lock()
	gotPayloads := len(stub.payloads)
	stub.mu.Unlock()
	if gotPayloads != 0 {
		t.Fatalf("relay received %d payloads after the refusal deadline, want 0", gotPayloads)
	}
}

// TestAResentAlternateCannotBounceAfterRestart covers the crash boundary. The
// first-refusal deadline survives compaction, but a durable sent handoff is
// stronger. After restart and deadline expiry, the entry still waits for its
// answer and never comes home.
func TestAResentAlternateCannotBounceAfterRestart(t *testing.T) {
	h := newRefusalHarness(t, 3)
	id := seedRefusalEntry(t, h, 2)
	sendExactTransportRefusal(t, h, id)
	deadline := journalEntry(t, h.side, id).RefusalDeadlineMs
	markAlternateSent(t, h, id)
	if err := h.side.Close(); err != nil {
		t.Fatal(err)
	}

	h.clock.Advance(h.cfg.BounceTimeout + time.Second)
	reopened, err := New(h.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	setRefusalMap(reopened, 3)
	st, ok := reopened.jr.Get(id)
	if !ok {
		t.Fatal("the resent entry disappeared on restart")
	}
	if st.RefusalDeadlineMs != deadline || st.Handoff != journal.HandoffSent {
		t.Fatalf("replayed state = deadline %d handoff %q, want %d sent",
			st.RefusalDeadlineMs, st.Handoff, deadline)
	}
	reopened.mu.Lock()
	reopened.tickOutbound(st, h.clock.Now())
	reopened.mu.Unlock()
	st, _ = reopened.jr.Get(id)
	if st.Direction != journal.Out || st.Handoff != journal.HandoffSent || st.BounceBack {
		t.Fatalf("expired historical refusal deadline moved a sent entry: %+v", st)
	}
}

// TestAlternateEnqueueFailureStaysSentConservatively closes the local writer
// after pace admission but before the prepared alternate is enqueued. The
// durable sent transition wins: no tick can retry or bounce the migration, and
// the ordinary forward timeout records the possible loss once.
func TestAlternateEnqueueFailureStaysSentConservatively(t *testing.T) {
	h := newRefusalHarness(t, 3)
	id := seedRefusalEntry(t, h, 2)
	sendExactTransportRefusal(t, h, id)
	before := journalEntry(t, h.side, id)
	deadline := before.RefusalDeadlineMs

	h.side.mu.Lock()
	h.side.relayConn = closedRelayWriter(t)
	h.side.relayReady = true
	h.side.sendPace.acked = true
	forwarded := h.side.forwardLocked(before, h.clock.Now())
	h.side.mu.Unlock()
	if forwarded {
		t.Fatal("a closed socket writer reported that it enqueued the alternate")
	}

	sent := journalEntry(t, h.side, id)
	if sent.Handoff != journal.HandoffSent || sent.Entry.DestSlot != 3 {
		t.Fatalf("enqueue failure state = handoff %q dest %d, want sent at alternate 3",
			sent.Handoff, sent.Entry.DestSlot)
	}
	if sent.RefusalDeadlineMs != deadline || sent.SentAtMs == 0 {
		t.Fatalf("enqueue failure lost its durable bounds: deadline %d sentAt %d",
			sent.RefusalDeadlineMs, sent.SentAtMs)
	}

	h.clock.Advance(h.cfg.BounceTimeout + time.Second)
	h.side.mu.Lock()
	h.side.tickOutbound(sent, h.clock.Now())
	retried := h.side.forwardLocked(sent, h.clock.Now())
	h.side.mu.Unlock()
	if retried {
		t.Fatal("a durable sent entry was offered to the writer a second time")
	}
	stillSent := journalEntry(t, h.side, id)
	if stillSent.Handoff != journal.HandoffSent || stillSent.Direction != journal.Out || stillSent.BounceBack {
		t.Fatalf("the old refusal deadline moved a conservatively sent entry: %+v", stillSent)
	}

	h.clock.Advance(h.cfg.ForwardTimeout)
	h.side.mu.Lock()
	h.side.tickOutbound(stillSent, h.clock.Now())
	h.side.mu.Unlock()
	lost := journalEntry(t, h.side, id)
	if lost.Handoff != journal.HandoffLost || lost.Status != journal.StatusDone || lost.BounceBack {
		t.Fatalf("enqueue failure terminal state = %+v, want one non-bounced loss", lost)
	}
	if got := len(h.side.jr.List()); got != 1 {
		t.Fatalf("enqueue failure created %d journal records, want one", got)
	}
}

func TestFirstTransportRefusalDeadlineDoesNotReset(t *testing.T) {
	h := newRefusalHarness(t, 3)
	// With no current map, the proof is durable but alternate exhaustion is not
	// knowable. The scheduler waits on the one durable deadline.
	h.side.mu.Lock()
	h.side.status = contractb.PeerStatus{}
	h.side.neighbours = map[string]*contractb.Neighbour{}
	h.side.mu.Unlock()
	id := seedRefusalEntry(t, h, 2)
	sendExactTransportRefusal(t, h, id)
	first := journalEntry(t, h.side, id).RefusalDeadlineMs

	h.clock.Advance(h.cfg.BounceTimeout - time.Millisecond)
	h.side.mu.Lock()
	h.side.tickOutbound(journalEntry(t, h.side, id), h.clock.Now())
	h.side.mu.Unlock()
	if st := journalEntry(t, h.side, id); st.Direction != journal.Out {
		t.Fatal("the entry bounced before the first deadline")
	}

	h.clock.Advance(time.Millisecond)
	h.side.mu.Lock()
	h.side.tickOutbound(journalEntry(t, h.side, id), h.clock.Now())
	h.side.mu.Unlock()
	st := journalEntry(t, h.side, id)
	if st.Direction != journal.In || !st.BounceBack {
		t.Fatalf("the deadline did not bounce the proven-safe entry: %+v", st)
	}
	if st.RefusalDeadlineMs != first {
		t.Fatalf("deadline reset from %d to %d", first, st.RefusalDeadlineMs)
	}
}
