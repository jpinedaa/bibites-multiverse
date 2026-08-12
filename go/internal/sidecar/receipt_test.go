package sidecar

// WP3's sidecar-side tests for contract-b-m4.md §6.12, §7.4, §9.2 and §22, B26 —
// THE SENDER'S HALF OF THE FORWARD RECEIPT.
//
// B26's sentence is that the relay's forwarding record moves into the sender's
// own journal, so these tests are about the journal rather than about the frame.
// Three properties carry the whole amendment on this side:
//
//	IT IS RECORDED, durably, against the entry, and survives a restart and a
//	    compaction — otherwise the fact still dies with the relay process and
//	    nothing was bought.
//	IT CHANGES NO STATE. An entry that is `sent` stays `sent`. The receipt is
//	    the evidence that the state is right, never a reason to move it.
//	ITS ONLY DIRECTION IS TOWARD HOLDING. It can make the safe answer more
//	    certain and it can NEVER authorize a re-route, which is the one way a
//	    new frame on this wire could have cost an organism.
//
// A STUB RELAY IS USED FOR THE ADVERSARIAL CASES and a real one for the happy
// path. That split is deliberate: the interesting receipts — one for a migration
// this peer never sent, one that contradicts the same relay's own
// neverForwarded, one that arrives after the chain completed — are frames NO
// CONFORMING RELAY EMITS, so a real relay cannot produce them and a sender that
// only ever met a conforming relay is a sender nobody has tested.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/wire"
)

// receiptsOf is journalEntry's non-fatal twin: how many FORWARD_RECEIPTs one
// entry holds, or -1 when the entry does not exist yet. A waitFor condition
// cannot call journalEntry, which fatals on the first poll — and the first poll
// is exactly the one that races the journal's own create.
func receiptsOf(s *Sidecar, migrationID string) int {
	for _, st := range s.CustodySnapshot() {
		if st.Entry.MigrationID == migrationID {
			return st.ForwardReceipts
		}
	}
	return -1
}

// ---------------------------------------------------------------- happy path

// TestB26AReceiptArrivesPerForwardAndBecomesTheSendersOwnEvidence is the
// amendment's own sentence, end to end over a real relay: one forward, one
// receipt, recorded against the sender's journal entry with the relay session it
// was written under.
//
// The destination's mod is SILENT, so no MIGRATION_ACK ever comes back and the
// entry stays in `sent` — which is where the receipt has to be readable, because
// `sent` is the state B26 exists to put evidence beside.
func TestB26AReceiptArrivesPerForwardAndBecomesTheSendersOwnEvidence(t *testing.T) {
	g := newGrid(t, 2, gridOptions{
		layout: layoutRow(2),
		tune: func(i int, c *Config) {
			// One forward and then a long wait: the retry cadence must not turn
			// this into a count of receipts nobody asked about.
			c.ForwardRetry = 30 * time.Second
		},
	})
	a, b := g.node(0), g.node(1)
	b.mod.setAckMode(ackSilent)

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "the sender to record the relay's FORWARD_RECEIPT", func() bool {
		return receiptsOf(a.side, migrationID) > 0
	})

	st := journalEntry(t, a.side, migrationID)
	if st.ForwardReceipts != 1 {
		t.Fatalf("the sender recorded %d receipts for one forward, want 1 (§6.12)", st.ForwardReceipts)
	}
	session := g.relay.relay.SessionID()
	if st.ReceiptSessionID != session {
		t.Fatalf("the recorded receipt names session %q, want the relay's %q — the SCOPE of the "+
			"fact travels with the fact (§5.2)", st.ReceiptSessionID, session)
	}
	if st.RelaySessionID != session {
		t.Fatalf("the entry's own hand-off session is %q, want %q; the receipt and the hand-off "+
			"must agree or ForwardedUnder can never fire", st.RelaySessionID, session)
	}
	if st.ReceiptDestSlot != b.slot {
		t.Fatalf("the recorded receipt names destSlot %d, want %d", st.ReceiptDestSlot, b.slot)
	}
	if st.ReceiptForwardedAtMs == 0 {
		t.Fatal("the recorded receipt carries no forwardedAt; §6.12 makes it REQUIRED")
	}
	// THE RECEIPT CHANGED NOTHING ELSE. This is the assertion that would catch an
	// implementation that treated an acknowledgement as a transition.
	if st.Handoff != journal.HandoffSent {
		t.Fatalf("handoff is %q after a receipt; §6.12 says an entry that is `sent` STAYS `sent` "+
			"and the receipt is the evidence that the state is right", st.Handoff)
	}
	if st.RerouteCount != 0 || st.AccruedHoldMs != 0 {
		t.Fatalf("the receipt moved the re-route count (%d) or the hold accrual (%d ms)",
			st.RerouteCount, st.AccruedHoldMs)
	}
	if n := a.side.ReceiptsRecorded(); n != 1 {
		t.Fatalf("the sidecar counted %d recorded receipts, want 1", n)
	}
	// And the destination holds the organism exactly once: a receipt is not a
	// delivery and it did not become one.
	waitFor(t, 10*time.Second, "the destination to take custody once", func() bool {
		return custodyOf(b.side, migrationID) != "absent"
	})
}

// TestB26TwoForwardsOfOneMigrationAreTwoReceipts is §6.12's *One per forward*
// row on the sender's side. A retry into a silent destination forwards again,
// and the count in the journal is a count of THIS SENDER'S OWN WRITES — never a
// count of organisms, because the migrationId is preserved and the destination
// deduplicates (§6.6).
func TestB26TwoForwardsOfOneMigrationAreTwoReceipts(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	b.mod.setAckMode(ackSilent)

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 20*time.Second, "a second FORWARD_RECEIPT from the retry", func() bool {
		return receiptsOf(a.side, migrationID) >= 2
	})

	st := journalEntry(t, a.side, migrationID)
	if st.Handoff != journal.HandoffSent {
		t.Fatalf("handoff is %q after two receipts, want sent", st.Handoff)
	}
	// The organism did not double. The destination's dedup is what guarantees it
	// and the receipt count is what would tempt a reader to think otherwise.
	if got := b.world.spawnCount(migrationID); got > 1 {
		t.Fatalf("the destination spawned the organism %d times; two receipts are two WRITES, "+
			"not two organisms", got)
	}
}

// ---------------------------------------------------------------- durability

// TestB26TheRecordedReceiptSurvivesARestartAndACompaction is §7.4's rule applied
// to B26's block. The whole amendment is "the fact moves into the sender's own
// journal", and a fact that a routine compaction erased would leave the sender
// exactly where §13 item 6 found it.
func TestB26TheRecordedReceiptSurvivesARestartAndACompaction(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "journal")
	jr, err := journal.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	id := wire.NewUUID()
	session := wire.NewUUID()
	if _, err := jr.Create(journal.Out, journal.Entry{
		MigrationID: id, EntityID: testEntityID, Kind: contracta.KindBibite,
		GameVersion: "0.6.3.1", Payload: makePayload(testEntityID), Edge: contracta.EdgeE,
		DestSlot: 2, JournaledAt: time.Now().UnixMilli(),
	}, false); err != nil {
		t.Fatalf("create: %v", err)
	}
	one := 1
	forwardedAt := int64(1785693732103)
	destSlot := 2
	if _, err := jr.Apply(id, journal.Update{
		Status: journal.StatusInFlight, Handoff: journal.HandoffSent, RelaySessionID: &session,
	}); err != nil {
		t.Fatalf("hand-off: %v", err)
	}
	if _, err := jr.Apply(id, journal.Update{
		ForwardReceipts: &one, ReceiptSessionID: &session, ReceiptDestSlot: &destSlot,
		ReceiptForwardedAtMs: &forwardedAt,
	}); err != nil {
		t.Fatalf("receipt: %v", err)
	}
	// A compaction rewrites the live set as one create plus one status record. It
	// is routine (§20, B20), so it is the likeliest place a new field is lost.
	if _, _, err := jr.Compact(); err != nil {
		t.Fatalf("compact: %v", err)
	}
	if err := jr.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// A restart replays the compacted log, which is what a sidecar does on every
	// start.
	again, err := journal.Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer again.Close()
	st, ok := again.Get(id)
	if !ok {
		t.Fatal("the entry did not survive the restart")
	}
	if st.ForwardReceipts != 1 || st.ReceiptSessionID != session ||
		st.ReceiptDestSlot != destSlot || st.ReceiptForwardedAtMs != forwardedAt {
		t.Fatalf("the receipt block did not survive: %+v", st)
	}
	if !st.ForwardedUnder(session) {
		t.Fatal("a replayed receipt no longer answers ForwardedUnder for its own session")
	}
	if st.ForwardedUnder(wire.NewUUID()) {
		t.Fatal("a receipt answered ForwardedUnder for a session it was never issued in")
	}
	if st.ForwardedUnder("") {
		t.Fatal("a receipt answered ForwardedUnder for NO session; an empty session is not a " +
			"session two statements can be about")
	}
}

// TestB26ARecordedReceiptIsNotEvidenceUnderANewRelaySession is the asymmetry
// B26 states in its own summary and refuses to hide: the sender no longer needs
// the relay to still be the same process to know its entry WAS forwarded — and a
// relay restart still takes away every statement about what was NOT.
func TestB26ARecordedReceiptIsNotEvidenceUnderANewRelaySession(t *testing.T) {
	st := &journal.State{
		ForwardReceipts:  1,
		ReceiptSessionID: "5f0b9c31-77ad-4e26-9a4c-1b83d206ef95",
	}
	if !st.ForwardedUnder("5f0b9c31-77ad-4e26-9a4c-1b83d206ef95") {
		t.Fatal("a receipt is not evidence under its own session")
	}
	if st.ForwardedUnder("c8177a34-90bd-4f51-8e02-4d6b915ca7e3") {
		t.Fatal("a receipt from a DEAD session answered for a new one; a relay cannot speak for " +
			"a process it did not run, and neither can its receipt (§5.2)")
	}
	// And a receipt-free entry answers nothing, whatever session it is asked
	// about. A MISSING RECEIPT IS SILENCE.
	none := &journal.State{ReceiptSessionID: "5f0b9c31-77ad-4e26-9a4c-1b83d206ef95"}
	if none.ForwardedUnder("5f0b9c31-77ad-4e26-9a4c-1b83d206ef95") {
		t.Fatal("an entry with zero receipts claimed to hold one")
	}
}

// ---------------------------------------------------------------- the stub relay

// stubRelay is a Contract B relay reduced to what a sidecar needs to stay
// connected, plus one thing a real relay will not do: PUSH AN ARBITRARY FRAME.
// It exists for the receipts a conforming relay never sends.
type stubRelay struct {
	t       *testing.T
	ts      *httptest.Server
	session string

	mu       sync.Mutex
	conn     *websocket.Conn
	ctx      context.Context
	payloads []contractb.MigrationPayload
	closeNow int // when >0, close the next connection with this code
}

func newStubRelay(t *testing.T) *stubRelay {
	t.Helper()
	s := &stubRelay{t: t, session: wire.NewUUID()}
	mux := http.NewServeMux()
	mux.HandleFunc(contractb.ContractBPath, s.serve)
	s.ts = httptest.NewServer(mux)
	t.Cleanup(s.ts.Close)
	return s
}

func (s *stubRelay) url() string {
	return "ws" + s.ts.URL[len("http"):] + contractb.ContractBPath
}

func (s *stubRelay) serve(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled, InsecureSkipVerify: true})
	if err != nil {
		return
	}
	ws.SetReadLimit(wire.MaxFrameBytes)
	ctx := r.Context()
	s.mu.Lock()
	s.conn = ws
	s.ctx = ctx
	shed := s.closeNow
	if shed > 0 {
		s.closeNow--
	}
	s.mu.Unlock()
	if shed > 0 {
		// §3.2's 4007, named the way a real relay names it: the limit and its
		// value, because a peer told only "capacity" cannot be rebuilt to fit.
		_ = ws.Close(contractb.CloseCapacity, "maxFramesPerSecond 50 exceeded (peak 412/s)")
		return
	}
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			return
		}
		env, decodeErr := wire.Decode(data)
		if decodeErr != nil {
			continue
		}
		switch env.Type {
		case contractb.TypeHandshake:
			slot := 1
			pos := contractb.Position{Col: 0, Row: 0}
			s.write(ws, ctx, contractb.TypeHandshakeAck, contractb.HandshakeAck{
				RelayVersion: "stub", ProtocolVersion: wire.ProtocolB,
				RelaySessionID: s.session, AssignedSlot: &slot, AssignedPosition: &pos,
				Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
				ReceivedAt: time.Now().UnixMilli(),
				Limits:     contractb.Limits{}.Published(),
			})
		case contractb.TypeSectorClaim:
			pos := contractb.Position{Col: 0, Row: 0}
			s.write(ws, ctx, contractb.TypeSectorGrant, contractb.SectorGrant{
				Granted: true, Slot: 1, Position: &pos,
				Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
				Reason: contractb.GrantGranted,
			})
		case contractb.TypeMigrationPayload:
			var p contractb.MigrationPayload
			if json.Unmarshal(env.Data, &p) == nil {
				s.mu.Lock()
				s.payloads = append(s.payloads, p)
				s.mu.Unlock()
			}
		case contractb.TypePing:
			var ping contractb.Ping
			if json.Unmarshal(env.Data, &ping) == nil {
				s.write(ws, ctx, contractb.TypePong, contractb.Pong{Nonce: ping.Nonce})
			}
		}
	}
}

func (s *stubRelay) write(ws *websocket.Conn, ctx context.Context, typ string, data any) {
	frame, err := wire.Encode(wire.ProtocolB, typ, time.Now().UnixMilli(), data)
	if err != nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = ws.Write(writeCtx, websocket.MessageText, frame)
}

// push writes one frame of the test's choosing to the connected sidecar.
func (s *stubRelay) push(typ string, data any) {
	s.t.Helper()
	s.mu.Lock()
	ws, ctx := s.conn, s.ctx
	s.mu.Unlock()
	if ws == nil {
		s.t.Fatal("stub relay: nothing is connected")
	}
	s.write(ws, ctx, typ, data)
}

// pushRaw writes a frame the typed helpers cannot express — a data object that
// is not a valid receipt, for instance.
func (s *stubRelay) pushRaw(typ, data string) {
	s.t.Helper()
	s.mu.Lock()
	ws, ctx := s.conn, s.ctx
	s.mu.Unlock()
	if ws == nil {
		s.t.Fatal("stub relay: nothing is connected")
	}
	frame, err := json.Marshal(wire.Envelope{
		Protocol: wire.ProtocolB, Type: typ, MessageID: wire.NewUUID(),
		SentAt: time.Now().UnixMilli(), Data: json.RawMessage(data)})
	if err != nil {
		s.t.Fatalf("stub relay: encode: %v", err)
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = ws.Write(writeCtx, websocket.MessageText, frame)
}

func (s *stubRelay) waitConnected() {
	s.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		ok := s.conn != nil
		s.mu.Unlock()
		if ok {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatal("no sidecar connected to the stub relay")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// stubConfig is a sidecar pointed at a stub relay, with no mod and a long retry
// so the test drives the traffic rather than the tick loop.
func stubConfig(t *testing.T, s *stubRelay) Config {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Listen = "127.0.0.1:0"
	cfg.RelayURL = s.url()
	cfg.PeerID = "peer-sender"
	cfg.Secret = "stub-secret"
	cfg.DataDir = t.TempDir()
	cfg.ContractAToken = testContractAToken
	cfg.Logger = testLogger(t)
	cfg.RelayBackoffMin = 30 * time.Millisecond
	cfg.RelayBackoffMax = 150 * time.Millisecond
	cfg.ForwardRetry = 30 * time.Second
	cfg.BounceTimeout = 30 * time.Second
	cfg.StatsInterval = 500 * time.Millisecond
	cfg.TickInterval = 40 * time.Millisecond
	return cfg
}

// waitSession waits until the sidecar has taken the stub's relaySessionId, which
// is the moment every proof and every receipt on this connection becomes
// comparable with the journal.
func waitSession(t *testing.T, s *Sidecar, want string) {
	t.Helper()
	waitFor(t, 10*time.Second, "the sidecar to adopt the relay session", func() bool {
		return s.RelaySessionID() == want
	})
}

// ---------------------------------------------------------------- adversarial receipts

// TestB26AReceiptForAnUnknownOrReplayedMigrationChangesNothing walks every
// receipt a conforming relay does not send. NONE of them may write anything, and
// NONE of them may end the connection: §4's rule is that a frame a receiver
// cannot use is ignored, and a receipt is the frame with the least standing of
// any on this wire to close a session over.
func TestB26AReceiptForAnUnknownOrReplayedMigrationChangesNothing(t *testing.T) {
	stub := newStubRelay(t)
	cfg := stubConfig(t, stub)

	// One outbound entry in `sent`, one inbound entry, and one completed
	// outbound entry — the three journal shapes a stray receipt can land on.
	outbound := seedSentEntry(t, cfg.DataDir, 2, stub.session)
	inbound, completed := seedInboundAndCompleted(t, cfg.DataDir)

	side := startSidecar(t, cfg)
	stub.waitConnected()
	waitSession(t, side, stub.session)

	before := side.ReceiptsRecorded()
	unknown := wire.NewUUID()
	cases := []struct {
		name string
		push func()
	}{
		{"a migration this sender never journaled", func() {
			stub.push(contractb.TypeForwardReceipt, contractb.ForwardReceipt{
				MigrationID: unknown, DestSlot: 2, RelaySessionID: stub.session,
				ForwardedAt: time.Now().UnixMilli()})
		}},
		{"an INBOUND entry, which this peer never forwarded", func() {
			stub.push(contractb.TypeForwardReceipt, contractb.ForwardReceipt{
				MigrationID: inbound, DestSlot: 2, RelaySessionID: stub.session,
				ForwardedAt: time.Now().UnixMilli()})
		}},
		{"a migration whose chain already completed", func() {
			stub.push(contractb.TypeForwardReceipt, contractb.ForwardReceipt{
				MigrationID: completed, DestSlot: 2, RelaySessionID: stub.session,
				ForwardedAt: time.Now().UnixMilli()})
		}},
		{"a receipt with no migrationId at all", func() {
			stub.push(contractb.TypeForwardReceipt, contractb.ForwardReceipt{
				DestSlot: 2, RelaySessionID: stub.session})
		}},
		{"a receipt naming slot 0", func() {
			stub.push(contractb.TypeForwardReceipt, contractb.ForwardReceipt{
				MigrationID: outbound, DestSlot: 0, RelaySessionID: stub.session})
		}},
		{"a receipt with no session", func() {
			stub.push(contractb.TypeForwardReceipt, contractb.ForwardReceipt{
				MigrationID: outbound, DestSlot: 2})
		}},
		{"a data object that is not a receipt", func() {
			stub.pushRaw(contractb.TypeForwardReceipt, `{"migrationId":41,"destSlot":"two"}`)
		}},
	}
	for _, c := range cases {
		c.push()
	}
	// A PING answered after all of them proves the connection survived every one.
	waitFor(t, 10*time.Second, "the link to still be alive after the stray receipts", func() bool {
		return side.RelayConnected() && side.RelaySessionID() == stub.session
	})
	time.Sleep(300 * time.Millisecond)

	if got := side.ReceiptsRecorded(); got != before {
		t.Fatalf("%d stray receipts were journaled; none of them names a forward this sender made",
			got-before)
	}
	if st := journalEntry(t, side, outbound); st.ForwardReceipts != 0 {
		t.Fatalf("the seeded entry recorded %d receipts from malformed frames", st.ForwardReceipts)
	}
	if st := journalEntry(t, side, inbound); st.ForwardReceipts != 0 {
		t.Fatalf("an INBOUND entry recorded %d receipts", st.ForwardReceipts)
	}
	if st := journalEntry(t, side, completed); st.ForwardReceipts != 0 {
		t.Fatalf("a completed entry recorded %d receipts; the tombstone already says more than "+
			"a receipt could", st.ForwardReceipts)
	}

	// And a WELL-FORMED receipt for the seeded entry still lands, which is what
	// makes every zero above an observation rather than a broken reader.
	stub.push(contractb.TypeForwardReceipt, contractb.ForwardReceipt{
		MigrationID: outbound, DestSlot: 2, RelaySessionID: stub.session,
		ForwardedAt: time.Now().UnixMilli()})
	waitFor(t, 10*time.Second, "the well-formed receipt to be recorded", func() bool {
		return receiptsOf(side, outbound) == 1
	})
}

// seedInboundAndCompleted writes the two journal shapes a stray receipt must
// bounce off: an inbound entry and an outbound one whose chain completed.
func seedInboundAndCompleted(t *testing.T, dataDir string) (inbound, completed string) {
	t.Helper()
	jr, err := journal.Open(filepath.Join(dataDir, "journal"))
	if err != nil {
		t.Fatalf("seed journal: %v", err)
	}
	defer jr.Close()
	inbound = wire.NewUUID()
	completed = wire.NewUUID()
	base := journal.Entry{
		EntityID: testEntityID, Kind: contracta.KindBibite, GameVersion: "0.6.3.1",
		Payload: makePayload(testEntityID), Edge: contracta.EdgeW, DestSlot: 1,
		JournaledAt: time.Now().UnixMilli(),
	}
	in := base
	in.MigrationID = inbound
	if _, err := jr.Create(journal.In, in, false); err != nil {
		t.Fatalf("seed inbound: %v", err)
	}
	out := base
	out.MigrationID = completed
	out.DestSlot = 2
	out.Edge = contracta.EdgeE
	if _, err := jr.Create(journal.Out, out, false); err != nil {
		t.Fatalf("seed completed: %v", err)
	}
	at := time.Now().UnixMilli()
	if _, err := jr.Apply(completed, journal.Update{
		Status: journal.StatusDone, Handoff: journal.HandoffDone, CompletedAt: &at}); err != nil {
		t.Fatalf("seed completed status: %v", err)
	}
	return inbound, completed
}

// TestB26AReceiptCanOnlyEverMoveAnEntryTowardHolding is the safety property the
// whole amendment turns on, and the one case where a receipt decides an outcome.
//
// THE SETUP IS A DEFECTIVE RELAY, deliberately: it receipts a forward and then,
// in the SAME session, answers `neverForwarded: true` for the same migration.
// Those two statements cannot both be true. §9.2 would re-route on the second
// one alone, and a re-route after a real forward is the duplication D2 refuses —
// so B26's direction rule decides it: A RECEIPT CAN ONLY EVER MOVE AN ENTRY
// TOWARD HOLDING, NEVER TOWARD RE-ROUTING.
//
// The control arm is the same relay without the receipt, which must still
// re-route: this is a NARROWING of what may re-route and never a new refusal.
func TestB26AReceiptCanOnlyEverMoveAnEntryTowardHolding(t *testing.T) {
	t.Run("a receipt contradicts the proof and the entry holds", func(t *testing.T) {
		runReceiptContradiction(t, true)
	})
	t.Run("without a receipt the same proof still frees the entry", func(t *testing.T) {
		runReceiptContradiction(t, false)
	})
}

func runReceiptContradiction(t *testing.T, withReceipt bool) {
	stub := newStubRelay(t)
	cfg := stubConfig(t, stub)
	id := seedSentEntry(t, cfg.DataDir, 2, stub.session)
	side := startSidecar(t, cfg)
	stub.waitConnected()
	waitSession(t, side, stub.session)

	if withReceipt {
		stub.push(contractb.TypeForwardReceipt, contractb.ForwardReceipt{
			MigrationID: id, DestSlot: 2, RelaySessionID: stub.session,
			ForwardedAt: time.Now().UnixMilli()})
		waitFor(t, 10*time.Second, "the receipt to be recorded", func() bool {
			return receiptsOf(side, id) == 1
		})
	}

	// The relay's proof of non-delivery, perfectly formed and matching the
	// entry's own session — everything §9.2 asks for.
	stub.push(contractb.TypeMigrationNack, contractb.MigrationNack{
		MigrationID:    id,
		SourcePeer:     "", // relay-generated
		DestPeer:       cfg.PeerID,
		Code:           contractb.NackPeerOffline,
		Class:          contractb.ClassTransient,
		Message:        "slot 2 is reserved to peer-dest, which is not connected",
		NeverForwarded: contractb.BoolPtr(true),
		RelaySessionID: stub.session,
	})

	if withReceipt {
		// It must NOT become `refused`, ever. Waiting is the only honest way to
		// assert an absence, and the entry is checked repeatedly rather than once.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			st := journalEntry(t, side, id)
			if st.Handoff == journal.HandoffRefused {
				t.Fatal("a matched neverForwarded freed an entry whose own journal holds the " +
					"SAME RELAY'S receipt for a forward in the SAME session — a re-route from " +
					"here is the duplication D2 refuses (§9.2, §6.12, §22 B26)")
			}
			if st.RerouteCount != 0 {
				t.Fatalf("the entry re-routed %d times against a receipt it holds", st.RerouteCount)
			}
			time.Sleep(20 * time.Millisecond)
		}
		st := journalEntry(t, side, id)
		if st.ForwardReceipts != 1 {
			t.Fatalf("the entry holds %d receipts after the contradiction, want 1", st.ForwardReceipts)
		}
		if st.Handoff != journal.HandoffSent && st.Handoff != journal.HandoffHeld {
			t.Fatalf("handoff is %q; a contradicted proof must leave the entry where it was — "+
				"sent, or held once the destination is observed dark", st.Handoff)
		}
		return
	}

	// The control: no receipt, so the proof is the only statement in the room and
	// §9.2 is untouched in every particular.
	waitFor(t, 10*time.Second, "the unreceipted entry to be freed by the relay's proof", func() bool {
		st := journalEntry(t, side, id)
		return st.Handoff == journal.HandoffRefused || st.Handoff == journal.HandoffPending
	})
}

// ---------------------------------------------------------------- mixed fleet

// TestB26AnUnknownMessageTypeIsIgnoredAndTheSessionSurvives is the MIXED-FLEET
// case, run from the older peer's side of it.
//
// B32's compatibility table rates the new message type "additive — a receiver
// that does not know it ignores it (§4)", and a sidecar built before B26 is
// exactly that receiver. What it does with a FORWARD_RECEIPT is what THIS build
// does with any type it has no case for, so the test drives an unknown type
// through the same path and requires the same three things: no close, no
// journal write, and no interruption of the traffic around it.
func TestB26AnUnknownMessageTypeIsIgnoredAndTheSessionSurvives(t *testing.T) {
	stub := newStubRelay(t)
	cfg := stubConfig(t, stub)
	id := seedSentEntry(t, cfg.DataDir, 2, stub.session)
	side := startSidecar(t, cfg)
	stub.waitConnected()
	waitSession(t, side, stub.session)

	// The shape of a frame a FUTURE amendment adds, which is the shape B26's own
	// frame has for every peer that predates it.
	for i := 0; i < 5; i++ {
		stub.pushRaw("FUTURE_MESSAGE", `{"migrationId":"`+id+`","whatever":true}`)
	}
	time.Sleep(300 * time.Millisecond)

	if !side.RelayConnected() {
		t.Fatal("an unknown message type ended the session; §4 says a receiver ignores one")
	}
	if side.RelaySessionID() != stub.session {
		t.Fatal("the session was re-established, so the link did drop")
	}
	st := journalEntry(t, side, id)
	if st.ForwardReceipts != 0 {
		t.Fatalf("an unknown type was journaled as a receipt: %d", st.ForwardReceipts)
	}
	// `held` is allowed and expected: the stub publishes no map, so the tick loop
	// observes slot 2 dark and holds — which is §9.2 working, and is what the
	// entry would be doing whether the unknown frames had arrived or not. What
	// must NOT have happened is a re-route or a refusal, because nothing in those
	// five frames is evidence of anything.
	if st.Handoff != journal.HandoffSent && st.Handoff != journal.HandoffHeld {
		t.Fatalf("an unknown type moved the handoff to %q", st.Handoff)
	}
	if st.RerouteCount != 0 {
		t.Fatalf("an unknown type re-routed the entry %d times", st.RerouteCount)
	}
}

// TestB26ASenderMeetingARelayThatSendsNoReceiptsIsUnchanged is the other half of
// the mixed fleet: a CURRENT sidecar against a relay that never receipts
// anything — an older relay, or a current one whose receipt was dropped from a
// full outbound queue. The two are indistinguishable, which is §9.2's new row,
// and both must leave M4's behaviour exactly as it was.
func TestB26ASenderMeetingARelayThatSendsNoReceiptsIsUnchanged(t *testing.T) {
	stub := newStubRelay(t)
	cfg := stubConfig(t, stub)
	id := seedSentEntry(t, cfg.DataDir, 2, stub.session)
	side := startSidecar(t, cfg)
	stub.waitConnected()
	waitSession(t, side, stub.session)

	// The relay's proof, with no receipt anywhere in the picture. M4's answer was
	// to free the entry, and B26 changes nothing about it.
	stub.push(contractb.TypeMigrationNack, contractb.MigrationNack{
		MigrationID: id, DestPeer: cfg.PeerID, Code: contractb.NackPeerOffline,
		Class: contractb.ClassTransient, Message: "not connected",
		NeverForwarded: contractb.BoolPtr(true), RelaySessionID: stub.session,
	})
	waitFor(t, 10*time.Second, "the entry to be freed exactly as it was before B26", func() bool {
		h := journalEntry(t, side, id).Handoff
		return h == journal.HandoffRefused || h == journal.HandoffPending
	})
	if st := journalEntry(t, side, id); st.ForwardReceipts != 0 {
		t.Fatalf("a relay that sent no receipts left %d on the entry", st.ForwardReceipts)
	}
	if n := side.ReceiptsRecorded(); n != 0 {
		t.Fatalf("the sidecar recorded %d receipts against a relay that sent none", n)
	}
}

// ---------------------------------------------------------------- the 4007 path

// TestB26ReceiptsAndTheTwoCapacityShedsPin covers the interaction §3.2's 4007
// rule has with a frame that is explicitly allowed to go missing.
//
// A shed connection loses whatever receipts were in flight, and a peer that
// takes two 4007s in a row holds at the backoff ceiling — so during that hold NO
// receipt can arrive for anything. The requirement is that none of it moves the
// journal: what was recorded stays recorded, what was not is silence, and the
// pin is reached on the shed count exactly as it was before B26.
func TestB26ReceiptsAndTheTwoCapacityShedsPin(t *testing.T) {
	stub := newStubRelay(t)
	cfg := stubConfig(t, stub)
	id := seedSentEntry(t, cfg.DataDir, 2, stub.session)
	side := startSidecar(t, cfg)
	stub.waitConnected()
	waitSession(t, side, stub.session)

	stub.push(contractb.TypeForwardReceipt, contractb.ForwardReceipt{
		MigrationID: id, DestSlot: 2, RelaySessionID: stub.session,
		ForwardedAt: time.Now().UnixMilli()})
	waitFor(t, 10*time.Second, "the receipt to be recorded before the shed", func() bool {
		return receiptsOf(side, id) == 1
	})

	// The next two connections are shed with 4007, which is the sequence §3.2
	// pins the backoff on.
	stub.mu.Lock()
	stub.closeNow = 2
	stub.conn.Close(contractb.CloseCapacity, "maxFramesPerSecond 50 exceeded (peak 412/s)")
	stub.mu.Unlock()

	waitFor(t, 15*time.Second, "the sidecar to take two capacity sheds", func() bool {
		return side.CapacitySheds() >= 2
	})

	// THE JOURNAL DID NOT MOVE. The recorded receipt is exactly where it was, and
	// the entry is still `sent`: a shed is a liveness event and B26 gave it no
	// say over custody.
	st := journalEntry(t, side, id)
	if st.ForwardReceipts != 1 {
		t.Fatalf("the recorded receipt did not survive two capacity sheds: %d", st.ForwardReceipts)
	}
	if st.ReceiptSessionID != stub.session {
		t.Fatalf("the recorded receipt's session changed across a shed: %q", st.ReceiptSessionID)
	}
	if st.Handoff != journal.HandoffSent && st.Handoff != journal.HandoffHeld {
		t.Fatalf("handoff is %q after two sheds, want sent or held", st.Handoff)
	}
	if st.RerouteCount != 0 {
		t.Fatalf("a capacity shed re-routed an entry %d times", st.RerouteCount)
	}
}

// ---------------------------------------------------------------- the operator's view

// TestB26TheOperatorsReportSaysWhetherTheFrameWasEverForwarded is §7.5's
// printed report, which is where B26's evidence is finally read by a person.
// --release-inflight is the command B26's own text names — "the receipt is what
// reduces how often anybody needs it" — and it is the moment a person decides
// whether a bounce may duplicate an organism.
//
// BOTH RENDERINGS ARE ASSERTED, and the negative one carries the weight: an
// absent receipt must never read as "so it was never forwarded".
func TestB26TheOperatorsReportSaysWhetherTheFrameWasEverForwarded(t *testing.T) {
	receipted := InflightEntry{
		ForwardReceipts:  2,
		ReceiptSession:   "5f0b9c31-77ad-4e26-9a4c-1b83d206ef95",
		ReceiptDestSlot:  5,
		ReceiptForwarded: time.UnixMilli(1785693732103),
	}
	got := renderReceiptEvidence(receipted, "  ")
	for _, want := range []string{"FORWARDED", "2 forward(s)", "slot 5",
		"5f0b9c31-77ad-4e26-9a4c-1b83d206ef95", "duplication"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the receipted report does not mention %q:\n%s", want, got)
		}
	}
	none := renderReceiptEvidence(InflightEntry{}, "  ")
	for _, want := range []string{"no FORWARD_RECEIPT", "proves NOTHING"} {
		if !strings.Contains(none, want) {
			t.Fatalf("the unreceipted report does not mention %q:\n%s", want, none)
		}
	}
	if strings.Contains(none, "FORWARDED:") {
		t.Fatalf("the unreceipted report claims a forward:\n%s", none)
	}
}

// ---------------------------------------------------------------- the sender's cost

// TestB26TheSendersDurableCostOfARecordedReceipt is the other half of WP3's
// measured cost. The relay's half is one 287 B frame it encodes and forgets
// (relay/receipt_cost_test.go); the SENDER's half is a durable write, and a
// durable write on this project's journal is an appended record AND AN FSYNC.
//
// That is the honest cost to name, because it is the one that does not shrink
// with a cheaper frame format: B26 buys durability, and durability is the fsync.
func TestB26TheSendersDurableCostOfARecordedReceipt(t *testing.T) {
	n := 200
	if testing.Short() {
		n = 20
	}
	dir := filepath.Join(t.TempDir(), "journal")
	jr, err := journal.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer jr.Close()

	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := wire.NewUUID()
		if _, err := jr.Create(journal.Out, journal.Entry{
			MigrationID: id, EntityID: testEntityID, Kind: contracta.KindBibite,
			GameVersion: "0.6.3.1", Edge: contracta.EdgeE, DestSlot: 2,
			JournaledAt: time.Now().UnixMilli(),
		}, false); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	sizeBefore := jr.Size()

	session := wire.NewUUID()
	one := 1
	destSlot := 2
	forwardedAt := time.Now().UnixMilli()
	start := time.Now()
	for _, id := range ids {
		if _, err := jr.Apply(id, journal.Update{
			ForwardReceipts: &one, ReceiptSessionID: &session, ReceiptDestSlot: &destSlot,
			ReceiptForwardedAtMs: &forwardedAt,
		}); err != nil {
			t.Fatalf("receipt apply: %v", err)
		}
	}
	elapsed := time.Since(start)
	grew := jr.Size() - sizeBefore

	t.Logf("B26 FORWARD_RECEIPT — the SENDER's measured cost per migration")
	t.Logf("  durability: one journal Apply per receipt = one appended record + one fsync")
	t.Logf("  time:       %s per recorded receipt over %d receipts (%s total) on this host's "+
		"test filesystem", elapsed/time.Duration(n), n, elapsed.Truncate(time.Millisecond))
	t.Logf("  journal:    %d B per record, %d B for %d receipts", grew/int64(n), grew, n)
	t.Logf("  shape:      the sender pays 4 journal writes per outbound migration instead of 3 " +
		"(create, hand-off, RECEIPT, ack) — a 33%% rise in journal records on the outbound path, " +
		"against a journal already bounded by §20 B20 and compacted to its live set")

	// The structural assertion: the record has to stay small, because it rides
	// the same disk budget §20 B20 bounds and it is written once per crossing.
	if per := grew / int64(n); per > 512 {
		t.Fatalf("a recorded receipt costs %d B of journal; it is four scalars against an entry "+
			"that already exists", per)
	}
	// And every one of them is readable afterwards, which is the point of paying
	// for the fsync at all.
	for _, id := range ids {
		st, ok := jr.Get(id)
		if !ok || !st.ForwardedUnder(session) {
			t.Fatalf("%s does not answer ForwardedUnder after its receipt was recorded", id)
		}
	}
}
