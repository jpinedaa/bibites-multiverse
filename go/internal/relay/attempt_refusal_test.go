package relay

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
	"multiverse/internal/wire"
)

func attemptPayload(id, source string, dest, rerouteCount int) contractb.MigrationPayload {
	p := contractb.MigrationPayload{
		MigrationID: id,
		Kind:        contracta.KindBibite,
		Body:        contractb.Body{Version: "0.6.3.1", BB8: "test"},
		SourcePeer:  source,
		SourceSlot:  1,
		DestSlot:    dest,
		ExitEdge:    contracta.EdgeE,
	}
	if rerouteCount > 0 {
		p.Reroute = &contractb.Reroute{
			FromSlot: dest - 1, Count: rerouteCount,
			Proof: contractb.ProofPeerRefused, AtMs: time.Now().UnixMilli(),
		}
	}
	return p
}

func waitMigration(t *testing.T, p *credPeer, id string) contractb.MigrationPayload {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		for _, env := range p.findAll(contractb.TypeMigrationPayload) {
			var payload contractb.MigrationPayload
			if json.Unmarshal(env.Data, &payload) == nil && payload.MigrationID == id {
				return payload
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("peer did not receive migration %s", id)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitNack(t *testing.T, p *credPeer, id, code string) contractb.MigrationNack {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		for _, env := range p.findAll(contractb.TypeMigrationNack) {
			var nack contractb.MigrationNack
			if json.Unmarshal(env.Data, &nack) == nil &&
				nack.MigrationID == id && nack.Code == code {
				return nack
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("peer did not receive %s for migration %s", code, id)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitAttemptNack(t *testing.T, p *credPeer, id string, destSlot, rerouteCount int) contractb.MigrationNack {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		for _, env := range p.findAll(contractb.TypeMigrationNack) {
			var nack contractb.MigrationNack
			if json.Unmarshal(env.Data, &nack) == nil && nack.MigrationID == id &&
				nack.Code == contractb.NackNotForwarded && nack.RefusedAttempt != nil &&
				nack.RefusedAttempt.DestSlot == destSlot &&
				nack.RefusedAttempt.RerouteCount == rerouteCount {
				return nack
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("peer did not receive NOT_FORWARDED for attempt slot %d reroute %d",
				destSlot, rerouteCount)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func freezeAndFillMigrationQueue(t *testing.T, r *credRelay, destPeer string, destSlot int,
	fillers ...*credPeer) {
	t.Helper()
	r.srv.mu.Lock()
	pace := r.srv.migrationPaceLocked(destPeer)
	pace.mu.Lock()
	pace.nextAllowed = time.Now().Add(30 * time.Second)
	pace.mu.Unlock()
	r.srv.mu.Unlock()

	baseline := r.srv.ForwardedCount()
	for i := 0; i < 8; i++ {
		filler := fillers[i%len(fillers)]
		filler.send(contractb.TypeMigrationPayload,
			attemptPayload(wire.NewUUID(), fmt.Sprintf("peer-filler-%d", i%len(fillers)),
				destSlot, 0))
	}
	deadline := time.Now().Add(3 * time.Second)
	for r.srv.ForwardedCount() != baseline+8 {
		if time.Now().After(deadline) {
			t.Fatalf("queue for slot %d accepted %d of 8 filler frames",
				destSlot, r.srv.ForwardedCount()-baseline)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestAttemptProofSurvivesMixedPeerAndTransportRefusals exercises the real
// relay transaction, including its migration-wide forwarding memory. An exact
// queue refusal remains provable after an earlier attempt reached a peer, while
// the legacy neverForwarded field remains false for older clients.
func TestAttemptProofSurvivesMixedPeerAndTransportRefusals(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxFramesPerSecond: 8}})
	for _, id := range []string{
		"peer-source", "peer-refuse", "peer-full-a", "peer-full-b", "peer-filler-0", "peer-filler-1",
	} {
		r.mint(id, peercred.GrantPeer)
	}
	refuseRes, _, err := r.srv.ReserveSlot("peer-refuse", nil)
	if err != nil {
		t.Fatal(err)
	}
	fullARes, _, err := r.srv.ReserveSlot("peer-full-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	fullBRes, _, err := r.srv.ReserveSlot("peer-full-b", nil)
	if err != nil {
		t.Fatal(err)
	}
	dial := func(id string) *credPeer {
		p := r.dial(dialSpec{credentialPeer: id, claimPeer: id, sendHandshake: true})
		p.wait(contractb.TypeHandshakeAck, 2*time.Second)
		return p
	}
	source := dial("peer-source")
	refuser := dial("peer-refuse")
	_ = dial("peer-full-a")
	_ = dial("peer-full-b")
	fillers := []*credPeer{dial("peer-filler-0"), dial("peer-filler-1")}
	// Start the rate-meter window after handshakes, so each filler stays below
	// its own eight-frame limit while their aggregate fills a destination.
	time.Sleep(1100 * time.Millisecond)

	// Peer refusal, then a queue refusal for reroute 1.
	peerThenTransport := wire.NewUUID()
	source.send(contractb.TypeMigrationPayload,
		attemptPayload(peerThenTransport, "peer-source", refuseRes.Slot, 0))
	waitMigration(t, refuser, peerThenTransport)
	refuser.send(contractb.TypeMigrationNack, contractb.MigrationNack{
		MigrationID: peerThenTransport, SourcePeer: "peer-refuse", DestPeer: "peer-source",
		Code: contractb.NackOverloaded, Class: contractb.ClassTransient, Message: "test refusal",
	})
	waitNack(t, source, peerThenTransport, contractb.NackOverloaded)
	freezeAndFillMigrationQueue(t, r, "peer-full-a", fullARes.Slot, fillers...)
	source.send(contractb.TypeMigrationPayload,
		attemptPayload(peerThenTransport, "peer-source", fullARes.Slot, 1))
	nack := waitAttemptNack(t, source, peerThenTransport, fullARes.Slot, 1)
	if nack.NeverForwarded == nil || *nack.NeverForwarded || nack.RefusedAttempt == nil ||
		nack.RefusedAttempt.DestSlot != fullARes.Slot || nack.RefusedAttempt.RerouteCount != 1 {
		t.Fatalf("peer->transport refusal = %+v, want legacy false and exact reroute-1 proof", nack)
	}

	// A transport refusal, then a peer refusal, then another transport refusal.
	// The second queue uses a separate frozen destination. The first queue stays
	// full and supplies the final attempt.
	time.Sleep(1100 * time.Millisecond)
	freezeAndFillMigrationQueue(t, r, "peer-full-b", fullBRes.Slot, fillers...)
	transportPeerTransport := wire.NewUUID()
	source.send(contractb.TypeMigrationPayload,
		attemptPayload(transportPeerTransport, "peer-source", fullBRes.Slot, 0))
	first := waitAttemptNack(t, source, transportPeerTransport, fullBRes.Slot, 0)
	if first.NeverForwarded == nil || !*first.NeverForwarded || first.RefusedAttempt == nil ||
		first.RefusedAttempt.DestSlot != fullBRes.Slot || first.RefusedAttempt.RerouteCount != 0 {
		t.Fatalf("first transport refusal = %+v, want exact original-attempt proof", first)
	}
	source.send(contractb.TypeMigrationPayload,
		attemptPayload(transportPeerTransport, "peer-source", refuseRes.Slot, 1))
	waitMigration(t, refuser, transportPeerTransport)
	refuser.send(contractb.TypeMigrationNack, contractb.MigrationNack{
		MigrationID: transportPeerTransport, SourcePeer: "peer-refuse", DestPeer: "peer-source",
		Code: contractb.NackOverloaded, Class: contractb.ClassTransient, Message: "test refusal",
	})
	waitNack(t, source, transportPeerTransport, contractb.NackOverloaded)
	source.send(contractb.TypeMigrationPayload,
		attemptPayload(transportPeerTransport, "peer-source", fullARes.Slot, 2))
	last := waitAttemptNack(t, source, transportPeerTransport, fullARes.Slot, 2)
	if last.NeverForwarded == nil || *last.NeverForwarded || last.RefusedAttempt == nil ||
		last.RefusedAttempt.DestSlot != fullARes.Slot || last.RefusedAttempt.RerouteCount != 2 {
		t.Fatalf("transport->peer->transport final refusal = %+v, want legacy false and exact reroute-2 proof", last)
	}
}

func TestRelayDrainRefusalCarriesNoAttemptProof(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{})
	for _, id := range []string{"peer-source", "peer-dest"} {
		r.mint(id, peercred.GrantPeer)
	}
	res, _, err := r.srv.ReserveSlot("peer-dest", nil)
	if err != nil {
		t.Fatal(err)
	}
	dest := r.dial(dialSpec{credentialPeer: "peer-dest", claimPeer: "peer-dest", sendHandshake: true})
	source := r.dial(dialSpec{credentialPeer: "peer-source", claimPeer: "peer-source", sendHandshake: true})
	dest.wait(contractb.TypeHandshakeAck, 2*time.Second)
	source.wait(contractb.TypeHandshakeAck, 2*time.Second)

	r.srv.mu.Lock()
	r.srv.draining = true
	r.srv.mu.Unlock()
	id := wire.NewUUID()
	source.send(contractb.TypeMigrationPayload, attemptPayload(id, "peer-source", res.Slot, 0))
	nack := waitNack(t, source, id, contractb.NackNotForwarded)
	if nack.RefusedAttempt != nil || nack.NeverForwarded != nil || nack.RelaySessionID != "" {
		t.Fatalf("relay drain exposed non-delivery proof %+v; drain must wait/reconnect, not walk destinations",
			nack)
	}
}

func TestRelayRejectsInvalidAttemptCorrelationInput(t *testing.T) {
	negative := attemptPayload(wire.NewUUID(), "peer-source", 1, 0)
	negative.Reroute = &contractb.Reroute{Count: -1}
	zero := attemptPayload(wire.NewUUID(), "peer-source", 1, 0)
	zero.Reroute = &contractb.Reroute{}
	for _, tc := range []struct {
		name string
		data any
	}{
		{
			name: "negative reroute count",
			data: negative,
		},
		{
			name: "zero reroute count",
			data: zero,
		},
		{
			name: "malformed reroute count",
			data: json.RawMessage(fmt.Sprintf(
				`{"migrationId":%q,"sourcePeer":"peer-source","destSlot":1,"reroute":{"count":"one"}}`,
				wire.NewUUID())),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := startCredRelay(t, credRelayOptions{})
			r.mint("peer-source", peercred.GrantPeer)
			p := r.dial(dialSpec{
				credentialPeer: "peer-source", claimPeer: "peer-source", sendHandshake: true,
			})
			p.wait(contractb.TypeHandshakeAck, 2*time.Second)
			p.send(contractb.TypeMigrationPayload, tc.data)
			code, reason := p.waitClosed(2 * time.Second)
			if code != contractb.CloseMalformedFrame || reason == "" {
				t.Fatalf("invalid reroute closed %d %q, want 4003 with reason", code, reason)
			}
		})
	}
}

func TestMigrationRoutingKeepsOtherRerouteFieldsOpaque(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{})
	r.mint("peer-source", peercred.GrantPeer)
	p := r.dial(dialSpec{
		credentialPeer: "peer-source", claimPeer: "peer-source", sendHandshake: true,
	})
	p.wait(contractb.TypeHandshakeAck, 2*time.Second)
	id := wire.NewUUID()
	p.send(contractb.TypeMigrationPayload, json.RawMessage(fmt.Sprintf(
		`{"migrationId":%q,"sourcePeer":"peer-source","destSlot":999,`+
			`"reroute":{"count":1,"fromSlot":"opaque","proof":[],"atMs":{}}}`,
		id)))
	nack := waitNack(t, p, id, contractb.NackSlotVacant)
	if nack.Code != contractb.NackSlotVacant || p.isClosed() {
		t.Fatalf("opaque reroute metadata changed routing: nack=%+v closed=%v", nack, p.isClosed())
	}
}
