package sidecar

// The transition tests of contract-b-m4.md §25, B37.
//
// B37 removes the re-forward from the SENDER and nothing else. Every sidecar
// already deployed still holds and retries, this project moves its fleet by
// publication and cannot make anybody upgrade (D25), and a relay that refused an
// old peer would evict a world for running last month's build. So the two
// defences that absorbed a re-forward stay exactly where they were:
//
//	the destination   deduplicates on migrationId against its journal AND its
//	                  tombstones (§6.6 step 1), and answers a merely-journaled
//	                  duplicate with NOTHING (§14, B6)
//	the archive       deduplicates on the record's own key (§5.1), now inside a
//	                  bounded window (§25, B38) — archive/dedup_test.go
//
// legacyPeer is what tests them: a bare Contract B peer that sends the same
// MIGRATION_PAYLOAD as many times as it likes. It is an old sidecar's retry
// loop with the sidecar taken away, and it is also every defective peer this
// map will ever meet.

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
	"multiverse/internal/wire"
)

type legacyPeer struct {
	t      *testing.T
	ws     *websocket.Conn
	ctx    context.Context
	peerID string

	mu     sync.Mutex
	frames []wire.Envelope
	slot   int
}

// dialLegacyPeer connects as an ordinary peer and claims a slot. It declares no
// mod, which is honest: nothing here simulates a world, only a sender.
func dialLegacyPeer(t *testing.T, rl *testRelay, peerID string, at contractb.Position) *legacyPeer {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	ws, _, err := websocket.Dial(dialCtx, rl.url(), &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
		HTTPHeader: http.Header{
			"Authorization": []string{peercred.Header(peerID, rl.secret(peerID))},
		},
	})
	dialCancel()
	if err != nil {
		cancel()
		t.Fatalf("legacy peer: dial: %v", err)
	}
	ws.SetReadLimit(wire.MaxFrameBytes)
	p := &legacyPeer{t: t, ws: ws, ctx: ctx, peerID: peerID}
	go p.readLoop()
	t.Cleanup(func() { cancel(); _ = ws.CloseNow() })

	p.send(contractb.TypeHandshake, contractb.Handshake{
		PeerID:          peerID,
		Role:            contractb.RolePeer,
		ProtocolVersion: wire.ProtocolB,
		SidecarVersion:  "legacy",
	})
	p.waitType(contractb.TypeHandshakeAck, 10*time.Second)
	pos := at
	p.send(contractb.TypeSectorClaim, contractb.SectorClaim{
		SimulationSize:    2000,
		ExportEdges:       []string{"E", "N", "W", "S"},
		BorderEdges:       []string{"E", "N", "W", "S"},
		ModConnected:      true,
		PreferredPosition: &pos,
	})
	env := p.waitType(contractb.TypeSectorGrant, 10*time.Second)
	var grant contractb.SectorGrant
	if err := json.Unmarshal(env.Data, &grant); err != nil {
		t.Fatalf("legacy peer: decode grant: %v", err)
	}
	if !grant.Granted {
		t.Fatalf("legacy peer: no slot: %s", grant.Reason)
	}
	p.mu.Lock()
	p.slot = grant.Slot
	p.mu.Unlock()
	return p
}

func (p *legacyPeer) readLoop() {
	for {
		_, data, err := p.ws.Read(p.ctx)
		if err != nil {
			return
		}
		env, decodeErr := wire.Decode(data)
		if decodeErr != nil {
			continue
		}
		p.mu.Lock()
		p.frames = append(p.frames, env)
		p.mu.Unlock()
	}
}

func (p *legacyPeer) send(typ string, data any) {
	frame, err := wire.Encode(wire.ProtocolB, typ, time.Now().UnixMilli(), data)
	if err != nil {
		p.t.Fatalf("legacy peer: encode %s: %v", typ, err)
	}
	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()
	if err := p.ws.Write(ctx, websocket.MessageText, frame); err != nil {
		p.t.Fatalf("legacy peer: write %s: %v", typ, err)
	}
}

// forward is the whole point: the same migrationId, the same bytes, again. A
// conforming sender since B37 does this once per destination; this one does it
// as often as the caller asks.
func (p *legacyPeer) forward(migrationID string, destSlot int, entityID int32) {
	p.t.Helper()
	p.mu.Lock()
	from := p.slot
	p.mu.Unlock()
	p.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
		MigrationID:  migrationID,
		Kind:         "bibite",
		Body:         contractb.Body{Version: "0.6.3.1", BB8: makePayload(entityID)},
		Lineage:      contractb.Lineage{Parents: []contractb.Parent{}},
		SourcePeer:   p.peerID,
		SourceSlot:   from,
		DestSlot:     destSlot,
		ExitEdge:     "E",
		ExitPosition: 0.5,
		Velocity:     contractb.Vec{X: 6.12, Y: 0.44},
		Heading:      274.11,
		EntityID:     entityID,
		Timestamp:    time.Now().UnixMilli(),
	})
}

func (p *legacyPeer) waitType(typ string, timeout time.Duration) wire.Envelope {
	p.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		p.mu.Lock()
		for _, env := range p.frames {
			if env.Type == typ {
				p.mu.Unlock()
				return env
			}
		}
		p.mu.Unlock()
		if time.Now().After(deadline) {
			p.t.Fatalf("legacy peer: no %s within %s", typ, timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// countType is the negative half, and it never waits: a rule that says a frame
// MUST NOT be sent can only be asserted by looking.
func (p *legacyPeer) countType(typ string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, e := range p.frames {
		if e.Type == typ {
			n++
		}
	}
	return n
}

// TestAnOldSidecarsRetryIsStillAbsorbedExactlyOnce is the transition guarantee
// stated as a test: a peer that re-forwards the same migration — which is every
// sidecar built before §25's B37 — costs the destination one organism, one
// spawn and one acknowledgement, exactly as it did before.
//
// It also carries §14 B6 forward. That rule was written for the sender's retry
// and the sender no longer retries, so B6 would have lost its subject and its
// test along with it. It has not: A DUPLICATE THAT ARRIVES AGAINST A
// JOURNALED-BUT-UNDELIVERED ENTRY IS ANSWERED WITH NOTHING, because an early
// ACK releases the sender's custody before the delivery it claims, and both
// sides would then have let go of one organism.
func TestAnOldSidecarsRetryIsStillAbsorbedExactlyOnce(t *testing.T) {
	spy := newLogSpy(t)
	rl := startRelay(t)
	if _, _, err := rl.relay.ReserveSlot("peer-legacy", &contractb.Position{Col: 0, Row: 0}); err != nil {
		t.Fatalf("ReserveSlot: %v", err)
	}
	cfgB := fastConfig(t, rl, "peer-b")
	cfgB.Logger = spy.logger()
	sideB := startSidecar(t, cfgB)
	waitSlot(t, sideB, 2)
	worldB := newWorld()
	modB := dialFakeMod(t, fakeModOptions{
		url: sideB.URL(), world: worldB, heartbeat: 100 * time.Millisecond})

	old := dialLegacyPeer(t, rl, "peer-legacy", contractb.Position{Col: 0, Row: 0})

	// The destination's world is stopped, so the payload is JOURNALED and not
	// yet delivered — B6's window, and the state pacing makes ordinary.
	modB.setPaused(true)
	time.Sleep(200 * time.Millisecond)

	migrationID := wire.NewUUID()
	old.forward(migrationID, 2, testEntityID)
	waitFor(t, 10*time.Second, "slot 2 to journal the payload", func() bool {
		return custodyOf(sideB, migrationID) != "absent"
	})
	// Four more, on a retry cadence an old sidecar would have used.
	for i := 0; i < 4; i++ {
		old.forward(migrationID, 2, testEntityID)
		time.Sleep(150 * time.Millisecond)
	}
	waitFor(t, 15*time.Second, "the duplicates to be recognised", func() bool {
		return spy.count("duplicate MIGRATION_PAYLOAD") >= 2
	})

	if reAcked := spy.count("reAcked=true"); reAcked != 0 {
		t.Fatalf("the receiver re-ACKed %d duplicate(s) against a journaled, un-delivered "+
			"entry; §14 B6 answers those with NOTHING", reAcked)
	}
	if n := old.countType(contractb.TypeMigrationAck); n != 0 {
		t.Fatalf("the sender was sent %d MIGRATION_ACK(s) before the spawn; an early ACK "+
			"releases custody before the delivery it claims", n)
	}

	// The world comes back and the spawn is what releases it — once.
	modB.setPaused(false)
	old.waitType(contractb.TypeMigrationAck, 20*time.Second)
	time.Sleep(500 * time.Millisecond)
	if got := worldB.spawnCount(migrationID); got != 1 {
		t.Fatalf("five forwards of one migrationId spawned %d organisms, want exactly 1: the "+
			"destination's dedup is what makes an old sidecar's retry harmless", got)
	}
	if n := old.countType(contractb.TypeMigrationAck); n != 1 {
		t.Fatalf("the sender was sent %d MIGRATION_ACKs for one delivery, want 1", n)
	}

	// And a retry AFTER the tombstone is re-ACKed rather than re-delivered
	// (§6.6 step 1, §14 B6): delivery is proven, so re-stating it costs nothing.
	old.forward(migrationID, 2, testEntityID)
	waitFor(t, 10*time.Second, "the tombstoned duplicate to be re-ACKed", func() bool {
		return old.countType(contractb.TypeMigrationAck) == 2
	})
	if got := worldB.spawnCount(migrationID); got != 1 {
		t.Fatalf("a retry against a tombstone spawned the organism again (%d)", got)
	}
}
