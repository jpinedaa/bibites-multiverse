package relay

// WP4's relay-side tests for contract-b-m4.md §3.3 and §22, B24: the published
// capacity table.
//
// FOUR CLAIMS ARE UNDER TEST, and they are B24's own four rules:
//
//	EVERY LIMIT IS COUNTABLE AT THE FRAME LEVEL, so D1 survives the package.
//	EVERY LIMIT IS A KNOB, so a test can set one and watch it bind.
//	EVERY PEER-VISIBLE LIMIT IS PUBLISHED, on HANDSHAKE_ACK and on PEER_STATUS,
//	    BESIDE the peer stats blocks and never inside one.
//	THE RELAY SHEDS THE CONNECTION, NEVER THE MAP — which is the property that
//	    makes having a capacity limit safe at all, and the only one of the four
//	    that needs a second peer in the room to prove.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
	"multiverse/internal/wire"
)

// tryDial dials without failing the test, so a test can assert on the HTTP
// status of a REFUSED upgrade. r.dial cannot: it is built for the connections
// that succeed.
func (r *credRelay) tryDial(peerID string) (*websocket.Conn, int, error) {
	r.t.Helper()
	header := http.Header{}
	header.Set("Authorization", peercred.Header(peerID, r.secret(peerID)))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, resp, err := websocket.Dial(ctx, r.url, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
		HTTPHeader:      header,
	})
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	if err == nil {
		r.t.Cleanup(func() { _ = ws.CloseNow() })
	}
	return ws, status, err
}

// TestEveryPublishedLimitIsOnBothFramesAndIsWhatTheRelayRuns is D20's rule in
// one assertion. The relay is configured with values that are NOT the shipped
// defaults, because a test against the defaults cannot tell "published" from
// "hard-coded".
func TestEveryPublishedLimitIsOnBothFramesAndIsWhatTheRelayRuns(t *testing.T) {
	running := contractb.Limits{
		MaxConnectionsPerPeer:      3,
		MaxConnectionsPerAddress:   9,
		MaxFramesPerSecond:         51,
		MaxFrameBytes:              7654321,
		MaxBytesPerSecond:          1234567,
		MaxClaimsPerMinute:         13,
		MaxGenomeRequestsPerMinute: 31,
		MaxSubscribers:             5,
	}
	r := startCredRelay(t, credRelayOptions{limits: running})
	r.mint("peer-a", "peer")
	p := r.dial(dialSpec{credentialPeer: "peer-a", claimPeer: "peer-a", sendHandshake: true})

	ack := p.wait(contractb.TypeHandshakeAck, 2*time.Second)
	var got contractb.HandshakeAck
	if err := json.Unmarshal(ack.Data, &got); err != nil {
		t.Fatalf("decode HANDSHAKE_ACK: %v", err)
	}
	assertLimits(t, "HANDSHAKE_ACK", got.Limits, running)

	p.claim()
	status := p.wait(contractb.TypePeerStatus, 2*time.Second)
	var st contractb.PeerStatus
	if err := json.Unmarshal(status.Data, &st); err != nil {
		t.Fatalf("decode PEER_STATUS: %v", err)
	}
	assertLimits(t, "PEER_STATUS", st.Limits, running)

	// And nothing here is a default: if the shipped table had leaked through,
	// every value above would be one off from what this relay was told.
	if got.Limits[contractb.LimitMaxFramesPerSecond] == contractb.DefaultMaxFramesPerSecond {
		t.Fatal("the relay published the SHIPPED default rather than the value it is running with")
	}
}

func assertLimits(t *testing.T, where string, got map[string]int64, want contractb.Limits) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s carries no limits object at all; §6.2 and §6.5 make it REQUIRED", where)
	}
	for _, key := range contractb.PublishedLimitKeys {
		if _, ok := got[key]; !ok {
			t.Fatalf("%s.limits is missing %q; a published table with a hole in it is not a table",
				where, key)
		}
	}
	if len(got) != len(contractb.PublishedLimitKeys) {
		t.Fatalf("%s.limits carries %d keys and §3.3 names %d: %v",
			where, len(got), len(contractb.PublishedLimitKeys), got)
	}
	for key, value := range want.Published() {
		if got[key] != value {
			t.Fatalf("%s.limits[%q] is %d, want the %d this relay runs with", where, key, got[key], value)
		}
	}
}

// TestLimitsRideBesideTheStatsBlockAndNeverInsideIt is §6.3.1's discipline,
// which is the reason DQ3's "published on the stats block" became "published
// beside it": that block is PEER-AUTHORED END TO END, and a relay-authored key
// inside it would be the first value on it the peer did not write.
func TestLimitsRideBesideTheStatsBlockAndNeverInsideIt(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{})
	r.mint("peer-a", "peer")
	p := r.dial(dialSpec{credentialPeer: "peer-a", claimPeer: "peer-a", sendHandshake: true})
	p.wait(contractb.TypeHandshakeAck, 2*time.Second)
	p.send(contractb.TypeSectorClaim, contractb.SectorClaim{
		SimulationSize: 2000, ExportEdges: []string{"E", "N"}, BorderEdges: []string{"E", "N"},
		ModConnected: true,
		Stats:        &contractb.PeerStats{Population: contractb.IntPtr(7)},
	})

	// EVERY PEER_STATUS is examined, not only the first. The first one a peer
	// receives is published for its HANDSHAKE and names no slot at all — it
	// has not claimed yet — so a test that looked only at frame one was racing
	// the coalescing window and asserting nothing whenever it lost.
	deadline := time.Now().Add(3 * time.Second)
	for {
		for _, status := range p.findAll(contractb.TypePeerStatus) {
			var raw struct {
				Slots []struct {
					Stats map[string]json.RawMessage `json:"stats"`
				} `json:"slots"`
				Limits map[string]int64 `json:"limits"`
			}
			if err := json.Unmarshal(status.Data, &raw); err != nil {
				t.Fatalf("decode PEER_STATUS: %v", err)
			}
			if len(raw.Slots) == 0 || raw.Slots[0].Stats == nil {
				continue
			}
			if len(raw.Limits) == 0 {
				t.Fatal("PEER_STATUS carries a stats block and no limits object beside it")
			}
			for _, key := range contractb.PublishedLimitKeys {
				if _, leaked := raw.Slots[0].Stats[key]; leaked {
					t.Fatalf("the relay wrote %q INSIDE a peer's stats block; §6.3.1 is "+
						"peer-authored end to end", key)
				}
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no PEER_STATUS carrying a stats block arrived")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestMaxConnectionsPerAddressRefusesTheUpgradeWith429 is the one limit
// answered with an HTTP status rather than a close code, and §3.3 says why:
// there is no WebSocket yet to close.
func TestMaxConnectionsPerAddressRefusesTheUpgradeWith429(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{
		MaxConnectionsPerAddress: 2, MaxConnectionsPerPeer: 8,
	}})
	r.mint("peer-a", "peer")
	for i := 0; i < 2; i++ {
		if _, status, err := r.tryDial("peer-a"); err != nil {
			t.Fatalf("connection %d was refused with HTTP %d: %v", i+1, status, err)
		}
	}
	_, status, err := r.tryDial("peer-a")
	if err == nil {
		t.Fatal("the third connection from this address was accepted; maxConnectionsPerAddress is 2")
	}
	if status != http.StatusTooManyRequests {
		t.Fatalf("the refused upgrade answered HTTP %d, want %d", status, http.StatusTooManyRequests)
	}
}

// TestAThirdConnectionForOnePeerIsShedWith4007 is §3.3's first row: the second
// connection is the 4006 overlap during a reconnect, and a third is capacity.
//
// It also asserts the SHED IS VISIBLE (§6.5): the peer's own slot carries the
// capacity axis of lastRefusal, naming which limit fired.
func TestAThirdConnectionForOnePeerIsShedWith4007(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxConnectionsPerPeer: 2}})
	r.mint("peer-a", "peer")
	r.mint("watcher", "peer")
	if _, _, err := r.srv.ReserveSlot("peer-a", nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	// Two sockets that authenticated and are simply holding.
	for i := 0; i < 2; i++ {
		if _, _, err := r.tryDial("peer-a"); err != nil {
			t.Fatalf("connection %d refused: %v", i+1, err)
		}
	}
	third := r.dial(dialSpec{credentialPeer: "peer-a", claimPeer: "peer-a", sendHandshake: true})
	code, reason := third.waitClosed(3 * time.Second)
	if code != contractb.CloseCapacity {
		t.Fatalf("the third connection closed %d %q, want 4007 CAPACITY", code, reason)
	}
	if !strings.Contains(reason, "maxConnectionsPerPeer") {
		t.Fatalf("the close reason %q does not name which limit fired", reason)
	}

	watcher := r.dial(dialSpec{credentialPeer: "watcher", claimPeer: "watcher", sendHandshake: true})
	watcher.claim()
	deadline := time.Now().Add(3 * time.Second)
	for {
		for _, st := range watcher.statuses() {
			for _, slot := range st.Slots {
				if slot.PeerID != "peer-a" {
					continue
				}
				if strings.HasPrefix(slot.LastRefusal, contractb.RefusalCapacity+":") &&
					strings.Contains(slot.LastRefusal, "maxConnectionsPerPeer") {
					return
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("no slot ever carried the capacity axis of lastRefusal for the shed peer")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestFrameRateShedsTheConnectionAndNeverTheMap is B24's fourth rule, and it is
// the one that needs two peers in the room: a limit that took the map down with
// the peer would not be safe to have.
func TestFrameRateShedsTheConnectionAndNeverTheMap(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxFramesPerSecond: 5}})
	r.mint("peer-loud", "peer")
	r.mint("peer-quiet", "peer")

	quiet := r.dial(dialSpec{credentialPeer: "peer-quiet", claimPeer: "peer-quiet", sendHandshake: true})
	quiet.wait(contractb.TypeHandshakeAck, 2*time.Second)
	quiet.claim()
	quiet.wait(contractb.TypeSectorGrant, 2*time.Second)

	loud := r.dial(dialSpec{credentialPeer: "peer-loud", claimPeer: "peer-loud", sendHandshake: true})
	loud.wait(contractb.TypeHandshakeAck, 2*time.Second)
	loud.claim()
	for i := 0; i < 40; i++ {
		loud.send(contractb.TypePing, contractb.Ping{Nonce: wire.NewUUID()})
	}
	code, reason := loud.waitClosed(3 * time.Second)
	if code != contractb.CloseCapacity {
		t.Fatalf("the loud peer closed %d %q, want 4007", code, reason)
	}
	if !strings.Contains(reason, "maxFramesPerSecond 5 exceeded") {
		t.Fatalf("the close reason %q names neither the limit nor its value", reason)
	}
	if !strings.Contains(reason, "peak") {
		t.Fatalf("the close reason %q states no measurement to hold the limit against", reason)
	}

	// THE MAP DID NOT MOVE. The quiet peer is still connected, was never closed,
	// and goes on being answered.
	if quiet.isClosed() {
		t.Fatal("shedding one peer for capacity closed another peer's connection")
	}
	quiet.send(contractb.TypePing, contractb.Ping{Nonce: wire.NewUUID()})
	quiet.wait(contractb.TypePong, 2*time.Second)
}

// TestByteRateIsCountedApartFromTheFrameRate covers the row that exists so
// maxFramesPerSecond cannot be evaded with maximum frames.
func TestByteRateIsCountedApartFromTheFrameRate(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{
		MaxFramesPerSecond: 10000, MaxBytesPerSecond: 8192,
	}})
	r.mint("peer-fat", "peer")
	p := r.dial(dialSpec{credentialPeer: "peer-fat", claimPeer: "peer-fat", sendHandshake: true})
	p.wait(contractb.TypeHandshakeAck, 2*time.Second)
	fat := strings.Repeat("x", 4096)
	for i := 0; i < 6; i++ {
		p.send(contractb.TypePing, contractb.Ping{Nonce: fat})
	}
	code, reason := p.waitClosed(3 * time.Second)
	if code != contractb.CloseCapacity {
		t.Fatalf("closed %d %q, want 4007", code, reason)
	}
	if !strings.Contains(reason, "maxBytesPerSecond 8192 exceeded") {
		t.Fatalf("the close reason %q does not name the byte ceiling", reason)
	}
}

// TestClaimStormIsRefusedAndTheConnectionStaysUp is §3.3's one refusal that is
// not a close, and §6.4's rate_limited reason. A claim storm is usually a peer
// whose measured time scale is wandering, and a refusal it can read beats a
// close it must recover from.
func TestClaimStormIsRefusedAndTheConnectionStaysUp(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxClaimsPerMinute: 3}})
	r.mint("peer-a", "peer")
	p := r.dial(dialSpec{credentialPeer: "peer-a", claimPeer: "peer-a", sendHandshake: true})
	p.wait(contractb.TypeHandshakeAck, 2*time.Second)

	for i := 0; i < 6; i++ {
		p.claim()
		time.Sleep(20 * time.Millisecond)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		refused, granted := 0, 0
		p.mu.Lock()
		frames := append([]wire.Envelope(nil), p.frames...)
		p.mu.Unlock()
		for _, env := range frames {
			if env.Type != contractb.TypeSectorGrant {
				continue
			}
			var g contractb.SectorGrant
			if json.Unmarshal(env.Data, &g) != nil {
				continue
			}
			if g.Granted {
				granted++
				continue
			}
			if g.Reason != contractb.GrantRateLimited {
				t.Fatalf("a claim was refused for %q rather than rate_limited", g.Reason)
			}
			refused++
		}
		if refused > 0 && granted > 0 {
			if p.isClosed() {
				t.Fatal("a rate-limited claim closed the connection; §3.3 says it must not")
			}
			// And the peer still holds the slot the earlier claims won.
			if len(r.srv.Snapshot()) == 0 {
				t.Fatal("the rate limit cost this peer its reservation")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("saw %d granted and %d rate-limited claims; expected both", granted, refused)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestMaxSubscribersShedsTheOneTooMany bounds §5.1's fan-out cost. B27's grant
// is what bounds the trust; this bounds the copy queues.
func TestMaxSubscribersShedsTheOneTooMany(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxSubscribers: 1}})
	r.mint("archive-one", "subscribe")
	r.mint("archive-two", "subscribe")

	first := r.dial(dialSpec{credentialPeer: "archive-one", claimPeer: "archive-one",
		role: contractb.RoleArchive, sendHandshake: true})
	first.wait(contractb.TypeHandshakeAck, 2*time.Second)

	second := r.dial(dialSpec{credentialPeer: "archive-two", claimPeer: "archive-two",
		role: contractb.RoleArchive, sendHandshake: true})
	code, reason := second.waitClosed(3 * time.Second)
	if code != contractb.CloseCapacity {
		t.Fatalf("the second subscriber closed %d %q, want 4007", code, reason)
	}
	if !strings.Contains(reason, "maxSubscribers 1 exceeded") {
		t.Fatalf("the close reason %q does not name the limit", reason)
	}
	if first.isClosed() {
		t.Fatal("shedding the second subscriber closed the first one")
	}
}

// TestASubscriberReplacingItselfDoesNotEatItsOwnCeiling: a flapping archive
// reconnects as the same peerId, which is §6.1's replacement and not a new
// subscriber. Without this the first reconnect after a ceiling of 1 would shed
// the archive permanently.
func TestASubscriberReplacingItselfDoesNotEatItsOwnCeiling(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxSubscribers: 1}})
	r.mint("archive-one", "subscribe")
	first := r.dial(dialSpec{credentialPeer: "archive-one", claimPeer: "archive-one",
		role: contractb.RoleArchive, sendHandshake: true})
	first.wait(contractb.TypeHandshakeAck, 2*time.Second)

	again := r.dial(dialSpec{credentialPeer: "archive-one", claimPeer: "archive-one",
		role: contractb.RoleArchive, sendHandshake: true})
	ack := again.wait(contractb.TypeHandshakeAck, 2*time.Second)
	if ack.Type != contractb.TypeHandshakeAck {
		t.Fatal("the reconnecting archive was not admitted")
	}
	if again.isClosed() {
		t.Fatal("an archive reconnecting as itself was shed for capacity")
	}
}

// TestMaxFrameBytesIsAKnobAndOverItIs1009 keeps the two answers apart: a frame
// too big is a SHAPE fault with close 1009 and a shape remedy; a rate is a
// CAPACITY fault with close 4007 and a rate remedy (§3.2, §3.3).
func TestMaxFrameBytesIsAKnobAndOverItIs1009(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxFrameBytes: 4096}})
	r.mint("peer-a", "peer")
	p := r.dial(dialSpec{credentialPeer: "peer-a", claimPeer: "peer-a", sendHandshake: true})
	p.wait(contractb.TypeHandshakeAck, 2*time.Second)
	p.send(contractb.TypePing, contractb.Ping{Nonce: strings.Repeat("y", 8192)})
	code, reason := p.waitClosed(3 * time.Second)
	if code != contractb.CloseTooBig {
		t.Fatalf("an oversized frame closed %d %q, want 1009 TOO_BIG", code, reason)
	}
}

// TestNoCapacityLimitReadsABody is D1, asserted as a property of the file that
// implements the limits rather than as a hope about it. §3.3 is blunt: no limit
// on this wire may require the relay to decode data.body.bb8, data.lineage or
// data.species, to index anything, or to keep per-organism state. D1 is why the
// archive is a separate service and why M6 can replace this relay with libp2p,
// and an abuse limit is not worth spending it.
func TestNoCapacityLimitReadsABody(t *testing.T) {
	src, err := os.ReadFile("capacity.go")
	if err != nil {
		t.Fatalf("read capacity.go: %v", err)
	}
	body := string(src)
	// The comment block at the top names these in order to forbid them, so the
	// search starts after it.
	code := body[strings.Index(body, "import ("):]
	for _, forbidden := range []string{"Lineage", "Species", "BB8", "MigrationPayload", "Unmarshal"} {
		if strings.Contains(code, forbidden) {
			t.Fatalf("the capacity limits touch %q; every one of them must be countable at the "+
				"frame level (D1, contract-b-m4.md §3.3)", forbidden)
		}
	}
}
