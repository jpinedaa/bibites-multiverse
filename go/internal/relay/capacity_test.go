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
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contracta"
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

func TestSourceKeyTrustsOneAddressFromALoopbackProxy(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "127.0.0.1:49152",
		Header:     http.Header{"X-Forwarded-For": []string{"203.0.113.12"}},
	}
	if got := sourceKey(r); got != "203.0.113.12" {
		t.Fatalf("sourceKey() = %q, want the client address from the local proxy", got)
	}
}

func TestSourceKeyIgnoresForwardingFromANonLoopbackPeer(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "198.51.100.8:49152",
		Header:     http.Header{"X-Forwarded-For": []string{"203.0.113.12"}},
	}
	if got := sourceKey(r); got != "198.51.100.8" {
		t.Fatalf("sourceKey() = %q, want the direct peer address", got)
	}
}

func TestSourceKeyRejectsAForwardedAddressChain(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "[::1]:49152",
		Header:     http.Header{"X-Forwarded-For": []string{"203.0.113.12, 198.51.100.8"}},
	}
	if got := sourceKey(r); got != "::1" {
		t.Fatalf("sourceKey() = %q, want the direct loopback address", got)
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
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxFramesPerSecond: 8}})
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
	if !strings.Contains(reason, "maxFramesPerSecond 8 exceeded") {
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

func TestMigrationFanInRateAndBoundarySpacing(t *testing.T) {
	for _, tc := range []struct {
		maxFrames int64
		wantRate  int64
		wantSpace time.Duration
	}{
		{50, 6, 166666667 * time.Nanosecond},
		{40, 5, 200 * time.Millisecond},
		{80, 10, 100 * time.Millisecond},
		{8, 1, time.Second},
	} {
		rate := contractb.MigrationFanInRate(tc.maxFrames)
		space := contractb.MigrationFanInInterval(tc.maxFrames)
		if rate != tc.wantRate || space != tc.wantSpace {
			t.Fatalf("maxFramesPerSecond %d gave migration fan-in %d/s at %s, want %d/s at %s",
				tc.maxFrames, rate, space, tc.wantRate, tc.wantSpace)
		}

		pace := migrationForwardPace{interval: space}
		var meter rateMeter
		meter.window = time.Second
		start := time.Unix(0, 0)
		for i := int64(0); i <= rate; i++ {
			at := start.Add(time.Duration(i) * space)
			if delay := pace.Reserve(at); delay != 0 {
				t.Fatalf("maxFramesPerSecond %d refused scheduled migration %d at %s",
					tc.maxFrames, i+1, at.Sub(start))
			}
			if total, _ := meter.observe(at, 1); total > rate {
				t.Fatalf("maxFramesPerSecond %d admitted %d migrations in one relay window; "+
					"derived fan-in rate is %d", tc.maxFrames, total, rate)
			}
		}
	}
}

func TestRelayRejectsAFrameLimitWithoutControlHeadroom(t *testing.T) {
	srv, err := New(Options{
		DataDir:         t.TempDir(),
		InsecureNoToken: true,
		Limits: contractb.Limits{
			MaxFramesPerSecond: contractb.MinimumMaxFramesPerSecond - 1,
		},
	})
	if srv != nil {
		srv.Close()
		t.Fatal("a relay started without the minimum control-frame headroom")
	}
	if err == nil || !strings.Contains(err.Error(), "maxFramesPerSecond must be at least 8") {
		t.Fatalf("low frame limit returned %v, want the minimum and the named limit", err)
	}
}

func TestMigrationPaceReservesOnlyPhysicalWriteSlots(t *testing.T) {
	interval := contractb.MigrationFanInInterval(40)
	first := migrationForwardPace{interval: interval}
	second := migrationForwardPace{interval: interval}
	now := time.Unix(0, 0)
	if delay := first.Reserve(now); delay != 0 {
		t.Fatalf("first physical write was delayed by %s", delay)
	}
	if delay := first.Reserve(now); delay != interval {
		t.Fatalf("second physical write delay is %s, want %s", delay, interval)
	}
	// Asking early does not reserve another future slot.
	if delay := first.Reserve(now.Add(interval)); delay != 0 {
		t.Fatalf("an early retry left a phantom physical-write slot: %s", delay)
	}
	// A different destination identity owns an independent schedule.
	if delay := second.Reserve(now); delay != 0 {
		t.Fatalf("one destination delayed another by %s", delay)
	}
}

// TestAggregateMigrationFanInCannotShedTheDestination reproduces the live
// failure. Honest source connections each stay below their own published frame
// ceiling, but together they can make a full destination answer above that
// ceiling. The relay spaces the forwards per destination, so all immediate
// OVERLOADED replies fit and no connection is shed.
func TestAggregateMigrationFanInCannotShedTheDestination(t *testing.T) {
	const maxFrames int64 = 40
	const sourceCount = 8
	const perSource = 6
	const total = sourceCount * perSource

	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{
		MaxFramesPerSecond:       maxFrames,
		MaxConnectionsPerAddress: 16,
	}})
	r.mint("peer-dest", peercred.GrantPeer)
	reservation, _, err := r.srv.ReserveSlot("peer-dest", nil)
	if err != nil {
		t.Fatalf("reserve destination: %v", err)
	}

	dest := r.dial(dialSpec{credentialPeer: "peer-dest", claimPeer: "peer-dest", sendHandshake: true})
	dest.wait(contractb.TypeHandshakeAck, 2*time.Second)
	type sourcePeer struct {
		id   string
		peer *credPeer
	}
	sources := make([]sourcePeer, 0, sourceCount)
	for i := 0; i < sourceCount; i++ {
		id := fmt.Sprintf("peer-source-%d", i)
		r.mint(id, peercred.GrantPeer)
		p := r.dial(dialSpec{credentialPeer: id, claimPeer: id, sendHandshake: true})
		p.wait(contractb.TypeHandshakeAck, 2*time.Second)
		sources = append(sources, sourcePeer{id: id, peer: p})
	}

	for i := 0; i < perSource; i++ {
		for _, source := range sources {
			source.peer.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
				MigrationID: wire.NewUUID(),
				Kind:        contracta.KindBibite,
				Body:        contractb.Body{Version: "0.6.3.1", BB8: "test"},
				SourcePeer:  source.id,
				SourceSlot:  2,
				DestSlot:    reservation.Slot,
				ExitEdge:    contracta.EdgeE,
			})
		}
	}

	// A control round trip must not wait behind the accepted migration queue.
	dest.send(contractb.TypePing, contractb.Ping{Nonce: wire.NewUUID()})
	dest.wait(contractb.TypePong, time.Second)

	answered := map[string]bool{}
	arrivals := []time.Time{}
	replies := map[string]contractb.MigrationNack{}
	deadline := time.Now().Add(15 * time.Second)
	for len(replies) < total {
		for _, env := range dest.findAll(contractb.TypeMigrationPayload) {
			var payload contractb.MigrationPayload
			if err := json.Unmarshal(env.Data, &payload); err != nil {
				t.Fatalf("decode forwarded migration: %v", err)
			}
			if answered[payload.MigrationID] {
				continue
			}
			answered[payload.MigrationID] = true
			arrivals = append(arrivals, time.Now())
			dest.send(contractb.TypeMigrationNack, contractb.MigrationNack{
				MigrationID: payload.MigrationID,
				SourcePeer:  "peer-dest",
				DestPeer:    payload.SourcePeer,
				Code:        contractb.NackOverloaded,
				Class:       contractb.ClassTransient,
				Message:     "test destination is full",
			})
		}
		for _, source := range sources {
			for _, env := range source.peer.findAll(contractb.TypeMigrationNack) {
				var nack contractb.MigrationNack
				if json.Unmarshal(env.Data, &nack) == nil {
					replies[nack.MigrationID] = nack
				}
			}
			if source.peer.isClosed() {
				t.Fatalf("aggregate fan-in shed source connection %s", source.id)
			}
		}
		if dest.isClosed() {
			t.Fatal("aggregate immediate migration replies shed the destination connection")
		}
		if time.Now().After(deadline) {
			t.Fatalf("sources received %d of %d terminal attempt replies; destination accepted %d",
				len(replies), total, len(answered))
		}
		time.Sleep(5 * time.Millisecond)
	}

	interval := contractb.MigrationFanInInterval(maxFrames)
	for i := 1; i < len(arrivals); i++ {
		if gap := arrivals[i].Sub(arrivals[i-1]); gap < interval-30*time.Millisecond {
			t.Fatalf("destination migration arrivals %d and %d were only %s apart; want about %s",
				i, i+1, gap, interval)
		}
	}
	rejected := 0
	for _, nack := range replies {
		if nack.Code == contractb.NackNotForwarded {
			rejected++
			if nack.NeverForwarded == nil || !*nack.NeverForwarded {
				t.Fatal("a unique full-queue refusal lacked matched neverForwarded:true")
			}
		}
	}
	if rejected == 0 {
		t.Fatal("aggregate burst did not exercise the bounded migration queue")
	}
	if got := r.srv.ForwardedCount(); got != len(answered) {
		t.Fatalf("relay recorded %d forwards, want its %d accepted transport enqueues", got, len(answered))
	}
	if len(answered)+rejected != total {
		t.Fatalf("%d accepted + %d queue refusals != %d attempts", len(answered), rejected, total)
	}
}

func TestFullMigrationQueueRefusesAtomicallyWithoutClosingDestination(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxFramesPerSecond: 8}})
	for _, id := range []string{"peer-dest", "peer-source-a", "peer-source-b"} {
		r.mint(id, peercred.GrantPeer)
	}
	r.mint("archive", peercred.GrantSubscribe)
	reservation, _, err := r.srv.ReserveSlot("peer-dest", nil)
	if err != nil {
		t.Fatalf("reserve destination: %v", err)
	}
	dest := r.dial(dialSpec{credentialPeer: "peer-dest", claimPeer: "peer-dest", sendHandshake: true})
	sourceA := r.dial(dialSpec{credentialPeer: "peer-source-a", claimPeer: "peer-source-a", sendHandshake: true})
	sourceB := r.dial(dialSpec{credentialPeer: "peer-source-b", claimPeer: "peer-source-b", sendHandshake: true})
	archive := r.dial(dialSpec{credentialPeer: "archive", claimPeer: "archive",
		role: contractb.RoleArchive, sendHandshake: true})
	for _, p := range []*credPeer{dest, sourceA, sourceB, archive} {
		p.wait(contractb.TypeHandshakeAck, 2*time.Second)
	}

	r.srv.mu.Lock()
	pace := r.srv.migrationPaceLocked("peer-dest")
	pace.mu.Lock()
	pace.nextAllowed = time.Now().Add(30 * time.Second)
	pace.mu.Unlock()
	r.srv.mu.Unlock()

	ids := make([]string, 9)
	for i := range ids {
		ids[i] = wire.NewUUID()
	}
	for i := 0; i < 4; i++ {
		sourceA.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
			MigrationID: ids[i],
			Kind:        contracta.KindBibite,
			Body:        contractb.Body{Version: "0.6.3.1", BB8: "test"},
			SourcePeer:  "peer-source-a",
			SourceSlot:  2,
			DestSlot:    reservation.Slot,
			ExitEdge:    contracta.EdgeE,
		})
	}
	// The sources are separate WebSocket readers. Sending A's frames before B's
	// does not by itself order their processing, so wait until A has filled the
	// first half of the destination queue before asking B to fill the rest.
	// Without this barrier, any of B's five frames could race ahead and A's last
	// frame—not ids[8]—would correctly receive the full-queue refusal.
	deadline := time.Now().Add(5 * time.Second)
	for r.srv.ForwardedCount() != 4 {
		if time.Now().After(deadline) {
			t.Fatalf("source A produced %d forwarding records, want 4 before source B",
				r.srv.ForwardedCount())
		}
		time.Sleep(5 * time.Millisecond)
	}
	for i := 4; i < 9; i++ {
		sourceB.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
			MigrationID: ids[i],
			Kind:        contracta.KindBibite,
			Body:        contractb.Body{Version: "0.6.3.1", BB8: "test"},
			SourcePeer:  "peer-source-b",
			SourceSlot:  2,
			DestSlot:    reservation.Slot,
			ExitEdge:    contracta.EdgeE,
		})
	}

	findNack := func(p *credPeer, migrationID string) (contractb.MigrationNack, bool) {
		for _, env := range p.findAll(contractb.TypeMigrationNack) {
			var nack contractb.MigrationNack
			if json.Unmarshal(env.Data, &nack) == nil && nack.MigrationID == migrationID {
				return nack, true
			}
		}
		return contractb.MigrationNack{}, false
	}
	deadline = time.Now().Add(5 * time.Second)
	var refused contractb.MigrationNack
	for {
		if got, ok := findNack(sourceB, ids[8]); ok {
			refused = got
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the ninth frame did not receive a full-queue refusal")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if refused.Code != contractb.NackNotForwarded || refused.NeverForwarded == nil ||
		!*refused.NeverForwarded {
		t.Fatalf("unique full-queue refusal = %+v, want NOT_FORWARDED with neverForwarded:true", refused)
	}
	if got := r.srv.ForwardedCount(); got != 8 {
		t.Fatalf("full queue produced %d forwarding records, want exactly 8 accepted enqueues", got)
	}
	deadline = time.Now().Add(5 * time.Second)
	for len(archive.findAll(contractb.TypeMigrationPayload)) < 8 {
		if time.Now().After(deadline) {
			t.Fatalf("archive received %d accepted payload copies, want 8",
				len(archive.findAll(contractb.TypeMigrationPayload)))
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := len(archive.findAll(contractb.TypeMigrationPayload)); got != 8 {
		t.Fatalf("rejected migration leaked into archive fan-out: got %d payloads", got)
	}

	// The same migrationId already has an accepted forwarding record. A later
	// full-queue refusal must not manufacture proof that the organism is safe to
	// reroute.
	sourceA.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
		MigrationID: ids[0],
		Kind:        contracta.KindBibite,
		Body:        contractb.Body{Version: "0.6.3.1", BB8: "test"},
		SourcePeer:  "peer-source-a",
		SourceSlot:  2,
		DestSlot:    reservation.Slot,
		ExitEdge:    contracta.EdgeE,
	})
	deadline = time.Now().Add(5 * time.Second)
	for {
		if got, ok := findNack(sourceA, ids[0]); ok {
			if got.NeverForwarded == nil || *got.NeverForwarded {
				t.Fatalf("already-recorded queue refusal carried neverForwarded=%v, want false",
					got.NeverForwarded)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("repeat migrationId did not receive its full-queue refusal")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := r.srv.ForwardedCount(); got != 8 {
		t.Fatalf("refused duplicate changed forwarding record count to %d", got)
	}
	if dest.isClosed() {
		t.Fatal("full paced migration queue closed the destination")
	}
	dest.send(contractb.TypePing, contractb.Ping{Nonce: wire.NewUUID()})
	dest.wait(contractb.TypePong, time.Second)
}

func TestPacedDestinationQueueNeverStallsTheSourceReader(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{
		MaxFramesPerSecond: 16,
		MaxBytesPerSecond:  512 << 10,
	}})
	for _, id := range []string{"peer-source", "peer-migration-dest", "peer-response-dest"} {
		r.mint(id, peercred.GrantPeer)
	}
	reservation, _, err := r.srv.ReserveSlot("peer-migration-dest", nil)
	if err != nil {
		t.Fatalf("reserve migration destination: %v", err)
	}
	source := r.dial(dialSpec{credentialPeer: "peer-source", claimPeer: "peer-source", sendHandshake: true})
	migrationDest := r.dial(dialSpec{
		credentialPeer: "peer-migration-dest", claimPeer: "peer-migration-dest", sendHandshake: true,
	})
	responseDest := r.dial(dialSpec{
		credentialPeer: "peer-response-dest", claimPeer: "peer-response-dest", sendHandshake: true,
	})
	source.wait(contractb.TypeHandshakeAck, 2*time.Second)
	migrationDest.wait(contractb.TypeHandshakeAck, 2*time.Second)
	responseDest.wait(contractb.TypeHandshakeAck, 2*time.Second)

	r.srv.mu.Lock()
	pace := r.srv.migrationPaceLocked("peer-migration-dest")
	pace.mu.Lock()
	pace.nextAllowed = time.Now().Add(2 * time.Second)
	pace.mu.Unlock()
	r.srv.mu.Unlock()

	source.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
		MigrationID: wire.NewUUID(),
		Kind:        contracta.KindBibite,
		Body:        contractb.Body{Version: "0.6.3.1", BB8: "test"},
		SourcePeer:  "peer-source",
		SourceSlot:  2,
		DestSlot:    reservation.Slot,
		ExitEdge:    contracta.EdgeE,
	})
	// Queue acceptance is synchronous. It must not stop this source handler
	// while the destination writer waits for its physical-send slot.
	source.wait(contractb.TypeForwardReceipt, time.Second)

	message := strings.Repeat("x", 32<<10)
	for i := 0; i < 6; i++ {
		source.send(contractb.TypeMigrationNack, contractb.MigrationNack{
			MigrationID: wire.NewUUID(),
			SourcePeer:  "peer-source",
			DestPeer:    "peer-response-dest",
			Code:        contractb.NackOverloaded,
			Class:       contractb.ClassTransient,
			Message:     message,
		})
		time.Sleep(80 * time.Millisecond)
	}
	deadline := time.Now().Add(time.Second)
	for len(responseDest.findAll(contractb.TypeMigrationNack)) < 6 {
		if source.isClosed() {
			t.Fatal("destination pacing stalled the source reader and caused a false capacity close")
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of 6 directed control frames routed while a migration waited",
				len(responseDest.findAll(contractb.TypeMigrationNack)))
		}
		time.Sleep(5 * time.Millisecond)
	}
	source.send(contractb.TypePing, contractb.Ping{Nonce: wire.NewUUID()})
	source.wait(contractb.TypePong, time.Second)
	if source.isClosed() {
		t.Fatal("source closed while its accepted migration waited on another connection's writer")
	}
	migrationDest.wait(contractb.TypeMigrationPayload, 3*time.Second)
}

func TestAcceptedPacedMigrationSurvivesItsSourceDisconnect(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxFramesPerSecond: 8}})
	for _, id := range []string{"peer-dest", "peer-source"} {
		r.mint(id, peercred.GrantPeer)
	}
	reservation, _, err := r.srv.ReserveSlot("peer-dest", nil)
	if err != nil {
		t.Fatalf("reserve destination: %v", err)
	}
	dest := r.dial(dialSpec{credentialPeer: "peer-dest", claimPeer: "peer-dest", sendHandshake: true})
	source := r.dial(dialSpec{credentialPeer: "peer-source", claimPeer: "peer-source", sendHandshake: true})
	dest.wait(contractb.TypeHandshakeAck, 2*time.Second)
	source.wait(contractb.TypeHandshakeAck, 2*time.Second)

	r.srv.mu.Lock()
	pace := r.srv.migrationPaceLocked("peer-dest")
	pace.mu.Lock()
	pace.nextAllowed = time.Now().Add(time.Second)
	pace.mu.Unlock()
	r.srv.mu.Unlock()

	id := wire.NewUUID()
	source.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
		MigrationID: id,
		Kind:        contracta.KindBibite,
		Body:        contractb.Body{Version: "0.6.3.1", BB8: "test"},
		SourcePeer:  "peer-source",
		SourceSlot:  2,
		DestSlot:    reservation.Slot,
		ExitEdge:    contracta.EdgeE,
	})
	source.wait(contractb.TypeForwardReceipt, time.Second)
	_ = source.ws.CloseNow()

	deadline := time.Now().Add(3 * time.Second)
	for {
		for _, env := range dest.findAll(contractb.TypeMigrationPayload) {
			var payload contractb.MigrationPayload
			if json.Unmarshal(env.Data, &payload) == nil && payload.MigrationID == id {
				if got := r.srv.ForwardedCount(); got != 1 {
					t.Fatalf("relay recorded %d forwards, want the accepted transport enqueue", got)
				}
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("source disconnect canceled an already accepted destination transport frame")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDrainPhysicallyPacesAcceptedMigrationsBeforeClose4005(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxFramesPerSecond: 8}})
	for _, id := range []string{"peer-dest", "peer-source"} {
		r.mint(id, peercred.GrantPeer)
	}
	reservation, _, err := r.srv.ReserveSlot("peer-dest", nil)
	if err != nil {
		t.Fatalf("reserve destination: %v", err)
	}
	dest := r.dial(dialSpec{credentialPeer: "peer-dest", claimPeer: "peer-dest", sendHandshake: true})
	source := r.dial(dialSpec{credentialPeer: "peer-source", claimPeer: "peer-source", sendHandshake: true})
	dest.wait(contractb.TypeHandshakeAck, 2*time.Second)
	source.wait(contractb.TypeHandshakeAck, 2*time.Second)

	r.srv.mu.Lock()
	pace := r.srv.migrationPaceLocked("peer-dest")
	pace.mu.Lock()
	pace.nextAllowed = time.Now().Add(time.Second)
	pace.mu.Unlock()
	r.srv.mu.Unlock()

	ids := []string{wire.NewUUID(), wire.NewUUID()}
	for _, id := range ids {
		source.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
			MigrationID: id,
			Kind:        contracta.KindBibite,
			Body:        contractb.Body{Version: "0.6.3.1", BB8: "test"},
			SourcePeer:  "peer-source",
			SourceSlot:  2,
			DestSlot:    reservation.Slot,
			ExitEdge:    contracta.EdgeE,
		})
	}
	deadline := time.Now().Add(time.Second)
	for len(source.findAll(contractb.TypeForwardReceipt)) < len(ids) {
		if time.Now().After(deadline) {
			t.Fatal("both migrations were not accepted before the drain boundary")
		}
		time.Sleep(5 * time.Millisecond)
	}

	drained := make(chan struct{})
	go func() {
		r.srv.Drain()
		close(drained)
	}()
	arrivals := []time.Time{}
	seen := map[string]bool{}
	deadline = time.Now().Add(4 * time.Second)
	for len(seen) < len(ids) {
		for _, env := range dest.findAll(contractb.TypeMigrationPayload) {
			var payload contractb.MigrationPayload
			if json.Unmarshal(env.Data, &payload) != nil || seen[payload.MigrationID] {
				continue
			}
			seen[payload.MigrationID] = true
			arrivals = append(arrivals, time.Now())
		}
		if time.Now().After(deadline) {
			t.Fatalf("drain delivered %d of %d accepted migrations", len(seen), len(ids))
		}
		time.Sleep(5 * time.Millisecond)
	}
	if gap := arrivals[1].Sub(arrivals[0]); gap < 900*time.Millisecond {
		t.Fatalf("graceful drain flushed paced migrations only %s apart", gap)
	}
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("relay drain did not finish after the paced queue emptied")
	}
	if got := r.srv.ForwardedCount(); got != len(ids) {
		t.Fatalf("relay recorded %d forwards, want %d accepted frames", got, len(ids))
	}
}

func TestAtomicForwardUsesTheReplacementForTheSameDestinationIdentity(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxFramesPerSecond: 8}})
	r.mint("peer-dest", peercred.GrantPeer)
	reservation, _, err := r.srv.ReserveSlot("peer-dest", nil)
	if err != nil {
		t.Fatalf("reserve destination: %v", err)
	}
	old := r.dial(dialSpec{credentialPeer: "peer-dest", claimPeer: "peer-dest", sendHandshake: true})
	old.wait(contractb.TypeHandshakeAck, 2*time.Second)
	r.srv.mu.Lock()
	oldPeer := r.srv.peers["peer-dest"]
	r.srv.mu.Unlock()
	replacement := r.dial(dialSpec{
		credentialPeer: "peer-dest", claimPeer: "peer-dest", sendHandshake: true,
	})
	replacement.wait(contractb.TypeHandshakeAck, 2*time.Second)

	id := wire.NewUUID()
	frame, err := wire.Encode(wire.ProtocolB, contractb.TypeMigrationPayload,
		time.Now().UnixMilli(), contractb.MigrationPayload{
			MigrationID: id,
			Kind:        contracta.KindBibite,
			Body:        contractb.Body{Version: "0.6.3.1", BB8: "test"},
			SourcePeer:  "peer-source",
			DestSlot:    reservation.Slot,
			ExitEdge:    contracta.EdgeE,
		})
	if err != nil {
		t.Fatalf("encode migration: %v", err)
	}
	result, gotReservation, dest, err := r.srv.enqueueMigration(reservation.Slot, id, frame)
	if err != nil || result != migrationEnqueued || dest == oldPeer ||
		gotReservation.PeerID != "peer-dest" {
		t.Fatalf("replacement result=%v dest=%p old=%p reservation=%+v err=%v, want current connection",
			result, dest, oldPeer, gotReservation, err)
	}
	replacement.wait(contractb.TypeMigrationPayload, 2*time.Second)
	if got := r.srv.ForwardedCount(); got != 1 {
		t.Fatalf("replacement target wrote %d forwarding records, want 1", got)
	}
}

func TestReplacementUsesTheSharedIdentityMigrationPace(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxFramesPerSecond: 8}})
	for _, id := range []string{"peer-dest", "peer-source"} {
		r.mint(id, peercred.GrantPeer)
	}
	reservation, _, err := r.srv.ReserveSlot("peer-dest", nil)
	if err != nil {
		t.Fatalf("reserve destination: %v", err)
	}
	old := r.dial(dialSpec{credentialPeer: "peer-dest", claimPeer: "peer-dest", sendHandshake: true})
	source := r.dial(dialSpec{credentialPeer: "peer-source", claimPeer: "peer-source", sendHandshake: true})
	old.wait(contractb.TypeHandshakeAck, 2*time.Second)
	source.wait(contractb.TypeHandshakeAck, 2*time.Second)

	replacement := r.dial(dialSpec{
		credentialPeer: "peer-dest", claimPeer: "peer-dest", sendHandshake: true,
	})
	replacement.wait(contractb.TypeHandshakeAck, 2*time.Second)

	// Model a physical slot that the overlapping old writer just reserved. The
	// replacement was already bound during its handshake, so its next migration
	// must consult this same identity-owned schedule rather than a fresh one.
	r.srv.mu.Lock()
	pace := r.srv.migrationPaceLocked("peer-dest")
	pace.mu.Lock()
	due := time.Now().Add(time.Second)
	pace.nextAllowed = due
	pace.mu.Unlock()
	r.srv.mu.Unlock()

	source.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
		MigrationID: wire.NewUUID(),
		Kind:        contracta.KindBibite,
		Body:        contractb.Body{Version: "0.6.3.1", BB8: "test"},
		SourcePeer:  "peer-source",
		SourceSlot:  2,
		DestSlot:    reservation.Slot,
		ExitEdge:    contracta.EdgeE,
	})
	source.wait(contractb.TypeForwardReceipt, time.Second)
	replacement.wait(contractb.TypeMigrationPayload, 2*time.Second)
	if arrived := time.Now(); arrived.Before(due.Add(-100 * time.Millisecond)) {
		t.Fatalf("replacement bypassed the old connection's physical-write slot by %s",
			due.Sub(arrived))
	}
}

func TestReplacementDropsItsOldAcceptedPacedBacklogWithoutTransferOrNack(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{limits: contractb.Limits{MaxFramesPerSecond: 8}})
	for _, id := range []string{"peer-dest", "peer-source"} {
		r.mint(id, peercred.GrantPeer)
	}
	reservation, _, err := r.srv.ReserveSlot("peer-dest", nil)
	if err != nil {
		t.Fatalf("reserve destination: %v", err)
	}
	old := r.dial(dialSpec{credentialPeer: "peer-dest", claimPeer: "peer-dest", sendHandshake: true})
	source := r.dial(dialSpec{credentialPeer: "peer-source", claimPeer: "peer-source", sendHandshake: true})
	old.wait(contractb.TypeHandshakeAck, 2*time.Second)
	source.wait(contractb.TypeHandshakeAck, 2*time.Second)

	r.srv.mu.Lock()
	pace := r.srv.migrationPaceLocked("peer-dest")
	pace.mu.Lock()
	pace.nextAllowed = time.Now().Add(2 * time.Second)
	pace.mu.Unlock()
	r.srv.mu.Unlock()

	id := wire.NewUUID()
	source.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
		MigrationID: id,
		Kind:        contracta.KindBibite,
		Body:        contractb.Body{Version: "0.6.3.1", BB8: "test"},
		SourcePeer:  "peer-source",
		SourceSlot:  2,
		DestSlot:    reservation.Slot,
		ExitEdge:    contracta.EdgeE,
	})
	source.wait(contractb.TypeForwardReceipt, time.Second)

	replacement := r.dial(dialSpec{
		credentialPeer: "peer-dest", claimPeer: "peer-dest", sendHandshake: true,
	})
	replacement.wait(contractb.TypeHandshakeAck, 2*time.Second)
	if code, reason := old.waitClosed(2 * time.Second); code != contractb.CloseReplaced {
		t.Fatalf("old destination closed %d %q, want replacement close", code, reason)
	}

	// A prompt replacement close makes the accepted attempt conservative. It
	// neither transfers organism bytes to the new socket nor creates later proof
	// of non-delivery for a write the relay already recorded.
	time.Sleep(2200 * time.Millisecond)
	if _, ok := replacement.find(contractb.TypeMigrationPayload); ok {
		t.Fatal("replacement inherited an organism accepted by the old connection")
	}
	for _, env := range source.findAll(contractb.TypeMigrationNack) {
		var nack contractb.MigrationNack
		if json.Unmarshal(env.Data, &nack) == nil && nack.MigrationID == id {
			t.Fatalf("replacement loss produced a later NACK: %+v", nack)
		}
	}
	if got := r.srv.ForwardedCount(); got != 1 {
		t.Fatalf("replacement loss changed %d forwarding records, want one conservative attempt", got)
	}
}

func TestMalformedMigrationIdentityNeverEntersThePacedQueue(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{})
	for _, id := range []string{"peer-dest", "peer-source"} {
		r.mint(id, peercred.GrantPeer)
	}
	reservation, _, err := r.srv.ReserveSlot("peer-dest", nil)
	if err != nil {
		t.Fatalf("reserve destination: %v", err)
	}
	dest := r.dial(dialSpec{credentialPeer: "peer-dest", claimPeer: "peer-dest", sendHandshake: true})
	source := r.dial(dialSpec{credentialPeer: "peer-source", claimPeer: "peer-source", sendHandshake: true})
	dest.wait(contractb.TypeHandshakeAck, 2*time.Second)
	source.wait(contractb.TypeHandshakeAck, 2*time.Second)

	source.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
		MigrationID: "",
		Kind:        contracta.KindBibite,
		Body:        contractb.Body{Version: "0.6.3.1", BB8: "test"},
		SourcePeer:  "peer-source",
		SourceSlot:  2,
		DestSlot:    reservation.Slot,
		ExitEdge:    contracta.EdgeE,
	})
	if code, reason := source.waitClosed(2 * time.Second); code != contractb.CloseMalformedFrame ||
		!strings.Contains(reason, "migrationId") {
		t.Fatalf("malformed identity closed %d %q, want malformed-frame close", code, reason)
	}
	if _, ok := dest.find(contractb.TypeMigrationPayload); ok {
		t.Fatal("migration without a valid identity entered the destination queue")
	}
	if got := r.srv.ForwardedCount(); got != 0 {
		t.Fatalf("malformed identity created %d forwarding records", got)
	}
}

func TestHandshakeCannotPublishAfterTheDrainBoundary(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{})
	r.mint("peer-late", peercred.GrantPeer)
	r.srv.Drain()

	late := r.dial(dialSpec{
		credentialPeer: "peer-late", claimPeer: "peer-late", sendHandshake: true,
	})
	code, reason := late.waitClosed(2 * time.Second)
	if code != contractb.CloseShuttingDown || !strings.Contains(reason, "relay draining") {
		t.Fatalf("post-drain handshake closed %d %q, want 4005 relay draining", code, reason)
	}
	if _, ok := late.find(contractb.TypeHandshakeAck); ok {
		t.Fatal("post-drain connection received HANDSHAKE_ACK and became routable")
	}
	r.srv.mu.Lock()
	_, published := r.srv.peers["peer-late"]
	r.srv.mu.Unlock()
	if published {
		t.Fatal("post-drain handshake appeared in the live peer registry")
	}
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
// data.species, to index anything, or to keep decoded or durable per-organism
// state. D1 is why the archive is a separate service and why M6 can replace
// this relay with libp2p, and an abuse limit is not worth spending it.
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
