package relay

// WP3's relay-side tests for contract-b-m4.md §6.12 and §22, B26 — THE FORWARD
// RECEIPT.
//
// The amendment is one frame, and almost all of its content is about what that
// frame must NOT do. So the cases below are weighted the same way: one proves
// the receipt arrives with the right four fields, and the rest prove it never
// reaches a subscriber, never reaches the destination, never appears for a frame
// that was not forwarded, never closes a connection it could not be written to,
// and never enters any counter §3.3 publishes.
//
// The cost this frame adds is measured in receipt_cost_test.go, which is WP3's
// own done-when clause: "the forward receipt ships WITH ITS MEASURED
// PER-MIGRATION COST AT RATE".

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contractb"
	"multiverse/internal/wire"
)

// ---------------------------------------------------------------- harness

// receiptRig is the smallest map a forward needs: a sender on slot 1 and a
// destination on slot 2, both live, both claimed.
type receiptRig struct {
	t      *testing.T
	r      *credRelay
	sender *credPeer
	dest   *credPeer
	// compression is what BOTH peers of this rig OFFER on their upgrade (§24,
	// B35). The zero value is CompressionDisabled, which is what every test in
	// this file wants: a receipt is measured in frames and fields, not bytes.
	compression websocket.CompressionMode
}

func startReceiptRig(t *testing.T, opts credRelayOptions) *receiptRig {
	t.Helper()
	return startReceiptRigWith(t, opts, websocket.CompressionDisabled)
}

// startReceiptRigWith is startReceiptRig with the peers' offered compression
// chosen by the caller, so compression_test.go can drive a real crossing over a
// deflated wire without a second copy of this rig.
func startReceiptRigWith(t *testing.T, opts credRelayOptions,
	compression websocket.CompressionMode) *receiptRig {
	t.Helper()
	r := startCredRelay(t, opts)
	rig := &receiptRig{t: t, r: r, compression: compression}
	rig.sender = rig.join("peer-sender", contractb.Position{Col: 0, Row: 0}, 1)
	rig.dest = rig.join("peer-dest", contractb.Position{Col: 1, Row: 0}, 2)
	return rig
}

func (rig *receiptRig) join(peerID string, at contractb.Position, wantSlot int) *credPeer {
	rig.t.Helper()
	rig.r.mint(peerID, "peer")
	pos := at
	if _, _, err := rig.r.srv.ReserveSlot(peerID, &pos); err != nil {
		rig.t.Fatalf("reserve %s: %v", peerID, err)
	}
	p := rig.r.dial(dialSpec{credentialPeer: peerID, claimPeer: peerID, sendHandshake: true,
		compression: rig.compression})
	p.wait(contractb.TypeHandshakeAck, 2*time.Second)
	p.claim()
	deadline := time.Now().Add(5 * time.Second)
	for {
		for _, env := range p.findAll(contractb.TypeSectorGrant) {
			var g contractb.SectorGrant
			if json.Unmarshal(env.Data, &g) == nil && g.Granted && g.Slot == wantSlot {
				return p
			}
		}
		if time.Now().After(deadline) {
			rig.t.Fatalf("%s never got slot %d", peerID, wantSlot)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// forward sends one MIGRATION_PAYLOAD from the sender to destSlot and returns
// its migrationId. The frame is built by hand so the test owns every byte and
// the payload is a realistic size.
func (rig *receiptRig) forward(destSlot int) string {
	rig.t.Helper()
	id := wire.NewUUID()
	rig.sender.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
		MigrationID: id,
		Kind:        "bibite",
		Body:        contractb.Body{Version: "0.6.3.1", BB8: `{"version":"0.6.3.1"}`},
		Lineage:     contractb.Lineage{Parents: []contractb.Parent{}},
		SourcePeer:  "peer-sender",
		SourceSlot:  1,
		DestSlot:    destSlot,
		ExitEdge:    "E",
		Timestamp:   time.Now().UnixMilli(),
	})
	return id
}

// forwardSized is forward with a caller-supplied bb8 body, so a test can put a
// realistically sized organism on the wire rather than a placeholder.
func (rig *receiptRig) forwardSized(destSlot int, bb8 string) string {
	rig.t.Helper()
	id := wire.NewUUID()
	rig.sender.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
		MigrationID: id,
		Kind:        "bibite",
		Body:        contractb.Body{Version: "0.6.3.1", BB8: bb8},
		Lineage:     contractb.Lineage{Parents: []contractb.Parent{}},
		SourcePeer:  "peer-sender",
		SourceSlot:  1,
		DestSlot:    destSlot,
		ExitEdge:    "E",
		Timestamp:   time.Now().UnixMilli(),
	})
	return id
}

// forwardAgain re-sends an EXISTING migrationId, which is what a retry and a
// re-route both look like on this wire.
func (rig *receiptRig) forwardAgain(id string, destSlot int) {
	rig.t.Helper()
	rig.sender.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
		MigrationID: id,
		Kind:        "bibite",
		Body:        contractb.Body{Version: "0.6.3.1", BB8: `{"version":"0.6.3.1"}`},
		Lineage:     contractb.Lineage{Parents: []contractb.Parent{}},
		SourcePeer:  "peer-sender",
		SourceSlot:  1,
		DestSlot:    destSlot,
		ExitEdge:    "E",
		Timestamp:   time.Now().UnixMilli(),
	})
}

// receipts decodes every FORWARD_RECEIPT this client has seen.
func (p *credPeer) receipts() []contractb.ForwardReceipt {
	out := []contractb.ForwardReceipt{}
	for _, env := range p.findAll(contractb.TypeForwardReceipt) {
		var r contractb.ForwardReceipt
		if json.Unmarshal(env.Data, &r) == nil {
			out = append(out, r)
		}
	}
	return out
}

func (p *credPeer) waitReceipts(t *testing.T, n int, timeout time.Duration) []contractb.ForwardReceipt {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if got := p.receipts(); len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("saw %d FORWARD_RECEIPTs within %s, want %d", len(p.receipts()), timeout, n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// ---------------------------------------------------------------- §6.12

// TestOneReceiptPerForwardWithTheContractsFourFields is §6.12's table, field by
// field. The receipt carries the migrationId, the destSlot it was written to,
// THE RELAY SESSION IN FORCE AT THE WRITE, and the relay's own clock — and
// nothing else, because it has nothing else to say.
func TestOneReceiptPerForwardWithTheContractsFourFields(t *testing.T) {
	rig := startReceiptRig(t, credRelayOptions{})
	before := time.Now().UnixMilli()
	id := rig.forward(2)

	got := rig.sender.waitReceipts(t, 1, 3*time.Second)
	if len(got) != 1 {
		t.Fatalf("one forward produced %d receipts, want exactly 1", len(got))
	}
	r := got[0]
	if r.MigrationID != id {
		t.Fatalf("receipt names migrationId %q, want %q — it is the sender's JOIN KEY into its "+
			"own journal and a wrong one files the fact under another organism", r.MigrationID, id)
	}
	if r.DestSlot != 2 {
		t.Fatalf("receipt destSlot = %d, want 2; §6.12 echoes it so a sender that re-routed can "+
			"tell two attempts apart", r.DestSlot)
	}
	if r.RelaySessionID != rig.r.srv.SessionID() {
		t.Fatalf("receipt relaySessionId = %q, want this relay's %q — A RECEIPT IS A STATEMENT "+
			"ABOUT ONE SESSION AND NOTHING ELSE (§5.2)", r.RelaySessionID, rig.r.srv.SessionID())
	}
	if r.ForwardedAt < before || r.ForwardedAt > time.Now().UnixMilli() {
		t.Fatalf("receipt forwardedAt %d is outside the window the forward happened in", r.ForwardedAt)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("the relay emitted a receipt that fails its own shape check: %v", err)
	}

	// And the destination got the payload it was owed. The receipt is beside the
	// forward, never instead of it.
	rig.dest.wait(contractb.TypeMigrationPayload, 3*time.Second)
}

// TestAReForwardProducesAnotherReceipt is §6.12's *One per forward* row. Two
// receipts under one migrationId means the sender FORWARDED TWICE — a retry or
// a re-route — and never a duplicated organism, because the migrationId is
// preserved and the destination deduplicates (§6.6).
func TestAReForwardProducesAnotherReceipt(t *testing.T) {
	rig := startReceiptRig(t, credRelayOptions{})
	id := rig.forward(2)
	rig.sender.waitReceipts(t, 1, 3*time.Second)
	rig.forwardAgain(id, 2)

	got := rig.sender.waitReceipts(t, 2, 3*time.Second)
	if len(got) != 2 {
		t.Fatalf("two forwards produced %d receipts, want 2", len(got))
	}
	for i, r := range got {
		if r.MigrationID != id {
			t.Fatalf("receipt %d names %q, want %q", i, r.MigrationID, id)
		}
	}
	// The §5.2 record still holds ONE id: it is a set of migrations, and the
	// receipt is a count of writes. The two are different facts and the relay
	// keeps them apart.
	if n := rig.r.srv.ForwardedCount(); n != 1 {
		t.Fatalf("the forwarding record holds %d ids after two forwards of one migration, want 1", n)
	}
	sent, dropped := rig.r.srv.ReceiptCounts()
	if sent != 2 || dropped != 0 {
		t.Fatalf("relay counted %d receipts sent and %d dropped, want 2 and 0", sent, dropped)
	}
}

// TestTheReceiptGoesToTheSenderAndNowhereElse covers two rules at once: §6.12's
// direction (relay -> SENDER) and §5.1's fan-out set, which B26 leaves alone.
//
// A SUBSCRIBER IS NOT COPIED, and the reason is worth stating because every
// other frame on this path is: a receipt is a fact about ONE SENDER'S JOURNAL,
// not about the migration. A subscriber that received one would be reading
// another peer's bookkeeping, and the archive has nothing to do with it.
func TestTheReceiptGoesToTheSenderAndNowhereElse(t *testing.T) {
	rig := startReceiptRig(t, credRelayOptions{})
	rig.r.mint("archive-one", "subscribe")
	sub := rig.r.dial(dialSpec{credentialPeer: "archive-one", claimPeer: "archive-one",
		role: contractb.RoleArchive, sendHandshake: true})
	sub.wait(contractb.TypeHandshakeAck, 2*time.Second)

	rig.forward(2)
	rig.sender.waitReceipts(t, 1, 3*time.Second)
	// The subscriber's copy of the MIGRATION_PAYLOAD proves it was subscribed and
	// reading at the moment the receipt was written, so its zero receipts are an
	// observation rather than a race.
	sub.wait(contractb.TypeMigrationPayload, 3*time.Second)

	if got := sub.receipts(); len(got) != 0 {
		t.Fatalf("a read-only subscriber received %d FORWARD_RECEIPTs; §5.1's fan-out set is "+
			"unchanged by B26 and a receipt is not about the migration", len(got))
	}
	if got := rig.dest.receipts(); len(got) != 0 {
		t.Fatalf("the DESTINATION received %d FORWARD_RECEIPTs; the receipt goes to the sender, "+
			"which is the only party with a journal entry to file it against", len(got))
	}
}

// TestNoReceiptForAFrameThatWasNotForwarded is §5.2's *What does not count* row,
// which the receipt must agree with exactly. A frame the relay REFUSED BEFORE
// WRITING created no forwarding record, so it must create no receipt either — a
// receipt for an unforwarded frame would be a durable lie in the sender's own
// journal, and the safe direction of §9.2 would be lost with it.
func TestNoReceiptForAFrameThatWasNotForwarded(t *testing.T) {
	rig := startReceiptRig(t, credRelayOptions{})

	// Slot 9 names no reservation at all: SLOT_VACANT, permanent, never written.
	rig.forward(9)
	nack := rig.sender.wait(contractb.TypeMigrationNack, 3*time.Second)
	var n contractb.MigrationNack
	if err := json.Unmarshal(nack.Data, &n); err != nil {
		t.Fatalf("decode MIGRATION_NACK: %v", err)
	}
	if n.Code != contractb.NackSlotVacant {
		t.Fatalf("a payload for an unreserved slot answered %q, want SLOT_VACANT", n.Code)
	}
	if n.NeverForwarded == nil || !*n.NeverForwarded {
		t.Fatal("the relay's own proof says it forwarded a frame it never wrote")
	}
	if got := rig.sender.receipts(); len(got) != 0 {
		t.Fatalf("the relay receipted %d frames it never forwarded", len(got))
	}
	sent, _ := rig.r.srv.ReceiptCounts()
	if sent != 0 {
		t.Fatalf("the relay counted %d receipts for a frame it refused before writing", sent)
	}
}

// TestADroppedReceiptNeverClosesTheSenderOrFailsTheForward is §6.12's *Best
// effort* and *Bounded* rows together, and it is the row that made TrySend
// necessary: wsutil.Send CLOSES a connection whose outbound queue is full, which
// is right for every frame a peer needs and wrong for the one frame that is
// explicitly allowed to go missing.
//
// The relay's answer to a receipt it cannot write is to count it and carry on.
func TestADroppedReceiptNeverClosesTheSenderOrFailsTheForward(t *testing.T) {
	rig := startReceiptRig(t, credRelayOptions{})
	rig.forward(2)
	rig.sender.waitReceipts(t, 1, 3*time.Second)

	// Take the relay's own peer object for the sender and close its connection
	// underneath the receipt path, which is the state a shed or a drained peer is
	// in at exactly the wrong moment.
	rig.r.srv.mu.Lock()
	p := rig.r.srv.peers["peer-sender"]
	rig.r.srv.mu.Unlock()
	if p == nil {
		t.Fatal("the relay has no peer object for the sender")
	}
	p.conn.Close(contractb.CloseNormal, "test")
	<-p.conn.Done()

	before, beforeDropped := rig.r.srv.ReceiptCounts()
	rig.r.srv.sendForwardReceipt(p, wire.NewUUID(), 2)
	after, afterDropped := rig.r.srv.ReceiptCounts()
	if after != before {
		t.Fatalf("a receipt to a closed connection counted as sent (%d -> %d)", before, after)
	}
	if afterDropped != beforeDropped+1 {
		t.Fatalf("a dropped receipt was not counted: dropped %d -> %d", beforeDropped, afterDropped)
	}

	// THE MAP DID NOT MOVE. The destination is still connected and still routable,
	// which is the property that makes a best-effort frame safe to have at all.
	if rig.dest.isClosed() {
		t.Fatal("a receipt the relay could not write closed another peer's connection")
	}
}

// TestAnEmptyMigrationIdIsNeverReceipted mirrors §5.2's record, which skips the
// same case: a frame the relay cannot name is a frame no journal can join a
// receipt to.
func TestAnEmptyMigrationIdIsNeverReceipted(t *testing.T) {
	rig := startReceiptRig(t, credRelayOptions{})
	rig.r.srv.mu.Lock()
	p := rig.r.srv.peers["peer-sender"]
	rig.r.srv.mu.Unlock()
	before, beforeDropped := rig.r.srv.ReceiptCounts()
	rig.r.srv.sendForwardReceipt(p, "", 2)
	after, afterDropped := rig.r.srv.ReceiptCounts()
	if after != before || afterDropped != beforeDropped {
		t.Fatalf("an unnameable migration was receipted: sent %d -> %d, dropped %d -> %d",
			before, after, beforeDropped, afterDropped)
	}
}

// ---------------------------------------------------------------- B24's arithmetic

// TestReceiptsEnterNoPublishedLimit is the B24 check B26's cost paragraph
// implies and does not spell out: does one more frame per migration move any
// ceiling §3.3 publishes?
//
// IT CANNOT, AND THE REASON IS STRUCTURAL. Every limit in §3.3 is counted on the
// relay's INBOUND path — frames and bytes a peer sends, claims it makes,
// connections it opens — and a receipt is relay-authored and outbound. The
// contract's own arithmetic therefore already holds with receipts flowing, and
// this test pins the structure rather than the number: a sender under a
// deliberately small supported maxFramesPerSecond forwards, is receipted, and
// is NOT shed.
func TestReceiptsEnterNoPublishedLimit(t *testing.T) {
	// Eight inbound frames a second. The rig below sends four (handshake, claim,
	// two payloads) and receives more than that back, receipts included.
	rig := startReceiptRig(t, credRelayOptions{limits: contractb.Limits{MaxFramesPerSecond: 8}})
	id := rig.forward(2)
	rig.forwardAgain(id, 2)
	rig.sender.waitReceipts(t, 2, 3*time.Second)

	if rig.sender.isClosed() {
		t.Fatal("a peer was shed for capacity while the relay was writing IT frames; every §3.3 " +
			"limit counts the relay's INBOUND path and a receipt is outbound")
	}
	if rig.dest.isClosed() {
		t.Fatal("the destination was shed while receiving forwards")
	}
	// And the published table still says what it always said: B26 adds no limit,
	// renames none, and moves none.
	ack := rig.sender.wait(contractb.TypeHandshakeAck, 2*time.Second)
	var got contractb.HandshakeAck
	if err := json.Unmarshal(ack.Data, &got); err != nil {
		t.Fatalf("decode HANDSHAKE_ACK: %v", err)
	}
	if len(got.Limits) != len(contractb.PublishedLimitKeys) {
		t.Fatalf("the published table carries %d keys and §3.3 names %d; B26 adds no limit",
			len(got.Limits), len(contractb.PublishedLimitKeys))
	}
	if got.Limits[contractb.LimitMaxFramesPerSecond] != 8 {
		t.Fatalf("maxFramesPerSecond published as %d, want the 8 this relay runs with",
			got.Limits[contractb.LimitMaxFramesPerSecond])
	}
}

// TestTheReceiptIsInTheCatalogueExactlyOnce guards the one thing a thirteenth
// message type can quietly break: a name that collides with something else on
// this wire, or a type string that is not a legal discriminator (§4).
func TestTheReceiptIsInTheCatalogueExactlyOnce(t *testing.T) {
	catalogue := []string{
		contractb.TypeHandshake, contractb.TypeHandshakeAck, contractb.TypeSectorClaim,
		contractb.TypeSectorGrant, contractb.TypePeerStatus, contractb.TypeMigrationPayload,
		contractb.TypeMigrationAck, contractb.TypeMigrationNack, contractb.TypeGenomeRequest,
		contractb.TypeGenomeResponse, contractb.TypePing, contractb.TypePong,
		contractb.TypeForwardReceipt,
	}
	if len(catalogue) != 13 {
		t.Fatalf("the catalogue holds %d types; §6 says thirteen since contract-b/4.0", len(catalogue))
	}
	seen := map[string]bool{}
	for _, typ := range catalogue {
		if !wire.ValidType(typ) {
			t.Fatalf("%q is not a legal message discriminator (§4)", typ)
		}
		if seen[typ] {
			t.Fatalf("%q appears twice in the catalogue", typ)
		}
		seen[typ] = true
	}
	if !strings.EqualFold(contractb.TypeForwardReceipt, "FORWARD_RECEIPT") {
		t.Fatalf("the receipt is named %q; §6.12 names it FORWARD_RECEIPT",
			contractb.TypeForwardReceipt)
	}
	// It is also not an admin act (B28): the admin path is a separate listener
	// and the catalogue grew by exactly one, which is this.
	for _, act := range []string{ActReleaseSlot, ActHandoverSlot, ActEvictPeer} {
		if strings.EqualFold(act, contractb.TypeForwardReceipt) {
			t.Fatalf("%q is both an admin act and a Contract B message type", act)
		}
	}
}

// receiptFrameBytes encodes one receipt exactly as the relay does, with
// realistic identifiers, and returns the frame. It is the measurement input the
// cost harness and the egress table both read.
func receiptFrameBytes(t *testing.T, destSlot int) []byte {
	t.Helper()
	frame, err := wire.Encode(wire.ProtocolB, contractb.TypeForwardReceipt, time.Now().UnixMilli(),
		contractb.ForwardReceipt{
			MigrationID:    wire.NewUUID(),
			DestSlot:       destSlot,
			RelaySessionID: wire.NewUUID(),
			ForwardedAt:    time.Now().UnixMilli(),
		})
	if err != nil {
		t.Fatalf("encode receipt: %v", err)
	}
	if len(frame) == 0 || !strings.Contains(string(frame), strconv.Itoa(destSlot)) {
		t.Fatalf("encoded receipt does not carry its destSlot: %s", frame)
	}
	return frame
}
