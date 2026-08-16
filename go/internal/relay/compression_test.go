package relay

// contract-b-m4.md §3 Transport and §24, B35 — PERMESSAGE-DEFLATE ON THE PEER
// WIRE.
//
// The amendment is one negotiated extension, and almost all of its content is
// about what must NOT change, so the cases below are weighted the same way. One
// proves the extension is negotiated and that it moves far fewer bytes for the
// same crossings. Two prove that nothing breaks for the halves of the map that
// have not been updated: a client that does not offer it is served uncompressed
// and is never refused, and a relay whose operator has turned it off serves
// exactly the pre-§24 wire to a client that DOES offer it.
//
// THE MEASUREMENT IS TCP-SIDE AND IT HAS TO BE. A frame counted after
// wire.Decode has already been inflated by the library, so a byte count taken
// anywhere above the socket would report the same number in every arm. The
// relay's listener is wrapped instead (see countingListener), which is the only
// place the bytes a hoster is billed for are visible.
//
// WHY THE FLOOR IS 50% AND NOT THE MEASURED 8.5x. The 2026-08-16 capture
// measured 9.0x on MIGRATION_PAYLOAD with zlib level 6; coder/websocket
// compresses at flate.BestSpeed, and this test's synthetic organism is not the
// capture's. A test that asserted a specific ratio would be asserting a
// compression library's tuning. 50% is the bar that separates "the extension is
// doing work" from "the extension is not on", which is what the code here can
// honestly promise. THE ACHIEVED RATIO IS IN THE LOG.

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
	"multiverse/internal/wsutil"
)

// ---------------------------------------------------------------- the counter

// countingListener wraps every accepted connection so the test can read what
// this relay actually put on and took off the wire.
type countingListener struct {
	net.Listener
	n *atomic.Int64
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &countingConn{Conn: c, n: l.n}, nil
}

type countingConn struct {
	net.Conn
	n *atomic.Int64
}

func (c *countingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.n.Add(int64(n))
	return n, err
}

func (c *countingConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.n.Add(int64(n))
	return n, err
}

// ---------------------------------------------------------------- an organism

// bb8Like builds a body with the SHAPE of a real bb8 blob — a fixed key set
// repeated over hundreds of neurons and synapses, with values that differ —
// rather than a run of one character.
//
// The shape is the whole point. A padded blob of repeated bytes compresses to
// nothing and would let this test pass on a wire that compressed only its own
// padding; the 2026-08-16 capture's 9.0x came from exactly this structure, where
// the KEYS repeat and the numbers do not. The generator is a deterministic LCG,
// so two arms of the same test compare the same bytes.
func bb8Like(target int) string {
	var b strings.Builder
	b.Grow(target + 512)
	b.WriteString(`{"version":"0.6.3.1","speciesID":41,"genes":{"SizeRatio":1.0399,` +
		`"SpeedRatio":0.9812,"Diet":0.6041,"ColorR":0.31,"ColorG":0.77,"ColorB":0.12},` +
		`"brain":{"Nodes":[`)
	seed := uint32(0x5EED1E)
	next := func() uint32 {
		seed = seed*1664525 + 1013904223
		return seed
	}
	kinds := []string{"Input", "Hidden", "Output"}
	types := []string{"TanH", "Sigmoid", "Linear", "ReLu", "Gaussian", "Latch", "Differential"}
	for i := 0; b.Len() < target*2/3; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"Index":%d,"Type":%q,"TypeName":%q,"Desc":"n%d","Value":%.5f,"Bias":%.5f}`,
			i, kinds[int(next())%len(kinds)], types[int(next())%len(types)], i,
			float64(next()%100000)/100000, float64(next()%100000)/50000-1)
	}
	b.WriteString(`],"Synapses":[`)
	for i := 0; b.Len() < target; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"Inno":%d,"NeuronIn":%d,"NeuronOut":%d,"Weight":%.6f,"En":true}`,
			i, next()%180, next()%180, float64(next()%2000000)/1000000-1)
	}
	b.WriteString(`]}}`)
	return b.String()
}

// ---------------------------------------------------------------- one arm

// b35Arm is one (relay setting, client offer) pair driven over the same
// crossings, and what it cost.
type b35Arm struct {
	name string
	// extensions is Sec-WebSocket-Extensions off the sender's 101 — the ONE
	// observable that says whether the extension was negotiated.
	extensions string
	// bytes is every TCP byte the relay's listener moved for the crossings
	// alone; the joins are excluded by taking the baseline after them.
	bytes int64
	// payloadBytes is the uncompressed size of what was asked for, so the arms
	// can be compared against the same denominator.
	payloadBytes int
}

// b35Crossings is how many organisms each arm sends. Enough that context
// takeover has a dictionary to work with by the end — which is the property
// being tested — and few enough to stay under §3.3's shipped frame rate if this
// test is ever run with the default table.
const b35Crossings = 32

func runB35Arm(t *testing.T, name string, relayOffersCompression bool,
	clientOffers websocket.CompressionMode, body string) b35Arm {
	t.Helper()
	rig := startReceiptRigWith(t, credRelayOptions{
		noWireCompression: !relayOffersCompression,
		// Out of reach: a PEER_STATUS heartbeat landing inside the measured
		// window would be charged to a crossing it has nothing to do with.
		statsBroadcast: time.Hour,
		statusCoalesce: 10 * time.Millisecond,
		// The ceilings are lifted because this test measures BYTES PER CROSSING
		// and not admission control, which capacity_test.go owns. Thirty-two
		// 18 KB frames in a tight loop is above the shipped maxFramesPerSecond.
		limits: contractb.Limits{
			MaxFramesPerSecond: 10000, MaxBytesPerSecond: 1 << 26, MaxClaimsPerMinute: 1000,
		},
		inMemoryGrid: true,
	}, clientOffers)

	// Settle the joins, then take the baseline: the handshake, the claim and the
	// grants are the same in every arm and are not what is being measured.
	time.Sleep(150 * time.Millisecond)
	baseline := rig.r.wireBytes.Load()

	ids := make([]string, 0, b35Crossings)
	for i := 0; i < b35Crossings; i++ {
		ids = append(ids, rig.forwardSized(2, body))
	}

	deadline := time.Now().Add(20 * time.Second)
	for len(rig.dest.findAll(contractb.TypeMigrationPayload)) < b35Crossings {
		if time.Now().After(deadline) {
			t.Fatalf("%s: only %d of %d crossings arrived (dest closed=%v)", name,
				len(rig.dest.findAll(contractb.TypeMigrationPayload)), b35Crossings,
				rig.dest.isClosed())
		}
		time.Sleep(5 * time.Millisecond)
	}
	spent := rig.r.wireBytes.Load() - baseline

	// EVERY ORGANISM ARRIVED WHOLE. Compression is a transport concern and this
	// is what makes that claim testable: the body the destination decoded is the
	// body the sender encoded, byte for byte, in every arm.
	got := map[string]string{}
	for _, env := range rig.dest.findAll(contractb.TypeMigrationPayload) {
		var pay contractb.MigrationPayload
		if err := json.Unmarshal(env.Data, &pay); err != nil {
			t.Fatalf("%s: a forwarded payload did not decode: %v", name, err)
		}
		got[pay.MigrationID] = pay.Body.BB8
	}
	for _, id := range ids {
		if got[id] != body {
			t.Fatalf("%s: migration %s arrived with a body of %d bytes, want the %d sent",
				name, id, len(got[id]), len(body))
		}
	}

	// One frame per crossing, both ways: the sender's payload in and the
	// destination's copy out. That is the denominator the ratio is honest about.
	payloadFrame := 0
	for _, env := range rig.dest.findAll(contractb.TypeMigrationPayload) {
		payloadFrame = len(env.Data) + 160 // the five-field envelope around it
		break
	}
	return b35Arm{
		name:         name,
		extensions:   rig.sender.extensions,
		bytes:        spent,
		payloadBytes: payloadFrame * b35Crossings * 2,
	}
}

// ---------------------------------------------------------------- the cases

// TestB35TheWireCompressesWhenBothEndsOfferItAndNeverWhenOneDoesNot is §24
// B35's whole promise in one table: the saving where both ends have it, and
// today's exact behaviour everywhere else.
func TestB35TheWireCompressesWhenBothEndsOfferItAndNeverWhenOneDoesNot(t *testing.T) {
	// ~17.9 KB, the 2026-08-16 capture's mean MIGRATION_PAYLOAD.
	body := bb8Like(17400)

	// THE UN-UPDATED SIDECAR. It offers nothing, so nothing is negotiated and
	// the frames on this wire are the frames of every release before §24.
	oldClient := runB35Arm(t, "relay offers, client does not",
		true, websocket.CompressionDisabled, body)
	// THE OPERATOR'S KILL SWITCH. The client offers, the relay does not answer,
	// and the map is back on uncompressed frames without the client changing.
	relayOff := runB35Arm(t, "client offers, relay does not",
		false, wsutil.PeerCompressionMode, body)
	// BOTH ENDS.
	both := runB35Arm(t, "both ends offer", true, wsutil.PeerCompressionMode, body)

	for _, arm := range []b35Arm{oldClient, relayOff} {
		if arm.extensions != "" {
			t.Errorf("%s: the 101 carried Sec-WebSocket-Extensions %q; the extension needs BOTH "+
				"ends and exactly one offered it (§24, B35)", arm.name, arm.extensions)
		}
	}
	if !strings.Contains(both.extensions, "permessage-deflate") {
		t.Fatalf("both ends offered permessage-deflate and the 101 answered %q (§24, B35)",
			both.extensions)
	}
	// CONTEXT TAKEOVER IS THE MODE, and RFC 7692 spells its ABSENCE: a peer that
	// wanted the cheap mode would have sent client_no_context_takeover or
	// server_no_context_takeover, and the ratio below depends on neither being
	// there.
	if strings.Contains(both.extensions, "no_context_takeover") {
		t.Fatalf("the negotiated extension is %q; §24 B35 asks for CONTEXT TAKEOVER, which is "+
			"the difference between ~8.5x and ~4.5x on this traffic", both.extensions)
	}

	// The two uncompressed arms must agree with each other: they are the same
	// wire reached two different ways, so a gap between them would mean the
	// measurement, not the extension, is what changed.
	if ratio := float64(oldClient.bytes) / float64(relayOff.bytes); ratio < 0.9 || ratio > 1.1 {
		t.Errorf("the two uncompressed arms moved %d and %d bytes (ratio %.2f); they are the "+
			"same wire and should cost the same", oldClient.bytes, relayOff.bytes, ratio)
	}

	// THE SAVING.
	share := float64(both.bytes) / float64(oldClient.bytes)
	if share >= 0.5 {
		t.Fatalf("the compressed arm moved %d bytes against %d uncompressed (%.1f%%); §24 B35 is "+
			"on this wire to move a lot less than half", both.bytes, oldClient.bytes, share*100)
	}
	for _, arm := range []b35Arm{oldClient, relayOff, both} {
		t.Logf("%-32s %8d bytes on the wire for %d crossings of ~%d bytes (%.2fx)",
			arm.name, arm.bytes, b35Crossings, arm.payloadBytes/(b35Crossings*2),
			float64(arm.payloadBytes)/float64(arm.bytes))
	}
	t.Logf("compressed / uncompressed = %.1f%%", share*100)
}

// TestB35AClientThatDoesNotOfferDeflateIsServedAndNeverRefused is the
// compatibility half on its own, over the ordinary claim path rather than a
// crossing, because "the relay MUST keep accepting uncompressed connections
// forever" is the rule the whole fleet's un-updated half depends on.
func TestB35AClientThatDoesNotOfferDeflateIsServedAndNeverRefused(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{statsBroadcast: time.Hour})
	r.mint("peer-old", peercred.GrantPeer)
	old := r.dial(dialSpec{credentialPeer: "peer-old", claimPeer: "peer-old", sendHandshake: true})
	if old.extensions != "" {
		t.Fatalf("a client that offered no extension was answered %q", old.extensions)
	}
	old.wait(contractb.TypeHandshakeAck, 2*time.Second)
	old.claim()
	grant := old.waitGrantReason(contractb.GrantGranted, 5*time.Second)
	if !grant.Granted {
		t.Fatalf("an uncompressed client was not placed: %+v", grant)
	}
	if old.isClosed() {
		t.Fatal("the relay closed a connection whose only difference was that it offered no " +
			"permessage-deflate; §24 B35 forbids refusing one")
	}

	// And the same relay, same process, same moment, serves a client that DOES
	// offer it — which is what "per connection" means.
	r.mint("peer-new", peercred.GrantPeer)
	fresh := r.dial(dialSpec{credentialPeer: "peer-new", claimPeer: "peer-new",
		sendHandshake: true, compression: wsutil.PeerCompressionMode})
	if !strings.Contains(fresh.extensions, "permessage-deflate") {
		t.Fatalf("the second client offered the extension and was answered %q", fresh.extensions)
	}
	fresh.wait(contractb.TypeHandshakeAck, 2*time.Second)
	fresh.claim()
	if g := fresh.waitGrantReason(contractb.GrantGranted, 5*time.Second); !g.Granted {
		t.Fatalf("a compressed client was not placed: %+v", g)
	}
}
