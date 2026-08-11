package relay

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
	"multiverse/internal/wire"
)

// speciesTestCreds is the credential store this file's relay runs on. Under
// contract-b/4 there is no shared token to hand every peer, so the harness mints
// one credential PER PEER the way an operator's console does, and dialPeer looks
// its own up by peerId (contract-b-m4.md §22, B22).
var speciesTestCreds struct {
	sync.Mutex
	store   *peercred.Store
	secrets map[string]string
}

func speciesSecret(t *testing.T, peerID string) string {
	t.Helper()
	speciesTestCreds.Lock()
	defer speciesTestCreds.Unlock()
	if secret, ok := speciesTestCreds.secrets[peerID]; ok {
		return secret
	}
	secret, err := speciesTestCreds.store.Mint(peerID, peercred.GrantPeer)
	if err != nil {
		t.Fatalf("mint %s: %v", peerID, err)
	}
	speciesTestCreds.secrets[peerID] = secret
	return secret
}

// TestRelayNeverParsesTheSpeciesBlock is contract-b-m4.md §15, B9: §5's
// prohibition list now names `data.species` beside `data.body.bb8` and
// `data.lineage`, and the relay's answer to the whole amendment is NOTHING.
//
// The assertion is deliberately at the BYTE level rather than at the field
// level. A relay that decoded the block and re-encoded it would pass a
// field-by-field comparison while quietly re-ordering keys, re-escaping a
// non-ASCII name or dropping a field it did not know — and the destination mod
// matches on an exact ordinal string comparison, so any of those is a silent
// mis-identification. Byte equality is the only test that says "it did not look".
//
// The frames carry shapes no conformant sender would emit — a species that is a
// NUMBER, and a block with an unknown extra field — because the point is that
// the relay does not have an opinion about any of them. A relay with an opinion
// about biology is a relay that can refuse an organism over a label.
func TestRelayNeverParsesTheSpeciesBlock(t *testing.T) {
	url := startTestRelay(t)

	sender := dialPeer(t, url, "peer-sender")
	sender.claim(1)
	// The sender needs slot 1 to be its own before it names itself sourcePeer.
	sender.waitGrant(1)
	receiver := dialPeer(t, url, "peer-receiver")
	receiver.claim(2)
	receiver.waitGrant(2)

	frames := []string{
		// The contract's own example, with the key order shuffled, a non-ASCII
		// name, an escaped quote and one \u escape the encoder would normalize.
		`{"genericName":"Cyanëa<&>","parentSpecificName":"prīma",` +
			`"specificName":"velox\"issima","parentGenericName":"Cyanëa"}`,
		// A block a sidecar would strip. The relay still forwards it untouched:
		// stripping is the SIDECAR's job at both ends, never the relay's.
		`{"genericName":"Cyanea"}`,
		// Not an object at all.
		`41`,
		// An unknown field the relay must not notice.
		`{"genericName":"Cyanea","specificName":"velox","grandparentGenericName":"Cyanea"}`,
	}
	for _, species := range frames {
		sent := payloadFrameWithSpecies(t, "peer-sender", 1, 2, species)
		sender.send(sent)
		got := receiver.readPayload()
		if string(got) != string(sent) {
			t.Fatalf("the relay rewrote a forwarded frame.\nsent %s\ngot  %s", sent, got)
		}
	}
}

// payloadFrameWithSpecies builds a MIGRATION_PAYLOAD frame by hand so the exact
// bytes are the test's to compare, `species` included.
func payloadFrameWithSpecies(t *testing.T, sourcePeer string, sourceSlot, destSlot int,
	species string) []byte {
	t.Helper()
	data := `{"migrationId":"` + wire.NewUUID() + `","kind":"bibite",` +
		`"body":{"version":"0.6.3.1","bb8":"{\"version\":\"0.6.3.1\"}"},` +
		`"lineage":{"genomeHash":"","parents":[]},` +
		`"species":` + species + `,` +
		`"sourcePeer":"` + sourcePeer + `","sourceSlot":` + strconv.Itoa(sourceSlot) +
		`,"destSlot":` + strconv.Itoa(destSlot) + `,"exitEdge":"E","exitPosition":0.5,` +
		`"velocity":{"x":6.12,"y":0.44},"heading":274.11,"entityId":-843827577,` +
		`"timestamp":1785693600149}`
	frame, err := json.Marshal(wire.Envelope{
		Protocol: wire.ProtocolB, Type: contractb.TypeMigrationPayload,
		MessageID: wire.NewUUID(), SentAt: time.Now().UnixMilli(),
		Data: json.RawMessage(data),
	})
	if err != nil {
		t.Fatalf("build frame: %v", err)
	}
	return frame
}

// ---------------------------------------------------------------- harness

func startTestRelay(t *testing.T) string {
	t.Helper()
	store, err := peercred.OpenStore("")
	if err != nil {
		t.Fatalf("peercred.OpenStore: %v", err)
	}
	speciesTestCreds.Lock()
	speciesTestCreds.store = store
	speciesTestCreds.secrets = map[string]string{}
	speciesTestCreds.Unlock()
	// A relay refuses to start on an empty store (§3.1), and every peer this file
	// dials mints its own on the way in.
	speciesSecret(t, "bootstrap-peer")

	srv, err := New(Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn})),
		Credentials:    store,
		PingInterval:   time.Second,
		PeerTimeout:    30 * time.Second,
		StatusCoalesce: 10 * time.Millisecond,
		StatsBroadcast: time.Second,
	})
	if err != nil {
		t.Fatalf("relay: new: %v", err)
	}
	// 127.0.0.1 on an EPHEMERAL port, always: this suite never touches a port the
	// running rig owns.
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })
	return "ws" + ts.URL[len("http"):] + contractb.ContractBPath
}

// testPeer is a bare Contract B client: enough handshake to be routable, and
// nothing else.
type testPeer struct {
	t      *testing.T
	id     string
	ws     *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
}

func dialPeer(t *testing.T, url, id string) *testPeer {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	ws, _, err := websocket.Dial(dialCtx, url, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
		HTTPHeader:      http.Header{"Authorization": []string{peercred.Header(id, speciesSecret(t, id))}},
	})
	dialCancel()
	if err != nil {
		cancel()
		t.Fatalf("peer %s: dial: %v", id, err)
	}
	ws.SetReadLimit(wire.MaxFrameBytes)
	p := &testPeer{t: t, id: id, ws: ws, ctx: ctx, cancel: cancel}
	t.Cleanup(func() { cancel(); _ = ws.CloseNow() })
	p.sendTyped(contractb.TypeHandshake, contractb.Handshake{
		PeerID: id, Role: contractb.RolePeer, ProtocolVersion: wire.ProtocolB,
		GameVersion: "0.6.3.1", SidecarVersion: "test", SimulationSize: 2000,
	})
	return p
}

func (p *testPeer) claim(slot int) {
	p.sendTyped(contractb.TypeSectorClaim, contractb.SectorClaim{
		PreferredSlot:     slot,
		PreferredPosition: &contractb.Position{Col: slot - 1, Row: 0},
		SimulationSize:    2000,
		ExportEdges:       []string{contracta.EdgeE, contracta.EdgeN},
		BorderEdges:       []string{contracta.EdgeE, contracta.EdgeN, contracta.EdgeW, contracta.EdgeS},
		GameVersion:       "0.6.3.1",
		ModConnected:      true,
	})
}

// waitGrant reads until this peer's own SECTOR_GRANT names the wanted slot.
func (p *testPeer) waitGrant(slot int) {
	p.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		env, err := wire.Decode(p.readFrame())
		if err != nil || env.Type != contractb.TypeSectorGrant {
			continue
		}
		var grant contractb.SectorGrant
		if json.Unmarshal(env.Data, &grant) == nil && grant.Granted && grant.Slot == slot {
			return
		}
	}
	p.t.Fatalf("peer %s never got slot %d", p.id, slot)
}

func (p *testPeer) sendTyped(typ string, data any) {
	p.t.Helper()
	frame, err := wire.Encode(wire.ProtocolB, typ, time.Now().UnixMilli(), data)
	if err != nil {
		p.t.Fatalf("peer %s: encode %s: %v", p.id, typ, err)
	}
	p.send(frame)
}

func (p *testPeer) send(frame []byte) {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()
	if err := p.ws.Write(ctx, websocket.MessageText, frame); err != nil {
		p.t.Fatalf("peer %s: write: %v", p.id, err)
	}
}

// readFrame returns the next frame, answering the relay's liveness PING so the
// connection is not dropped mid-test.
func (p *testPeer) readFrame() []byte {
	p.t.Helper()
	for {
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		_, frame, err := p.ws.Read(ctx)
		cancel()
		if err != nil {
			p.t.Fatalf("peer %s: read: %v", p.id, err)
		}
		env, decodeErr := wire.Decode(frame)
		if decodeErr == nil && env.Type == contractb.TypePing {
			var ping contractb.Ping
			if json.Unmarshal(env.Data, &ping) == nil {
				p.sendTyped(contractb.TypePong, contractb.Pong{Nonce: ping.Nonce})
			}
			continue
		}
		return frame
	}
}

// readPayload skips the map traffic — grants, PEER_STATUS, pings — so the byte
// comparison sees only the frame under test.
func (p *testPeer) readPayload() []byte {
	p.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		frame := p.readFrame()
		if env, err := wire.Decode(frame); err == nil &&
			env.Type == contractb.TypeMigrationPayload {
			return frame
		}
	}
	p.t.Fatalf("peer %s never received a MIGRATION_PAYLOAD", p.id)
	return nil
}
