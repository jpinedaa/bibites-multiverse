package relay

// WP2's relay-side tests: contract-b-m4.md §22, B22 (the per-peer credential
// bound to the peerId), B25 (the minimum contract version at the handshake) and
// B27 (the archive as an AUTHORISED subscriber).
//
// The test this file exists for is TestRisk1_ValidCredentialWithAnotherPeersID,
// and it is named for the risk rather than for the mechanism because
// m5_considerations.md's Risk 1 is a claim, not a bug: A RELAY WITH TLS AND A
// SHARED TOKEN LOOKS SECURED IN EVERY SCREENSHOT AND IS NOT. §13 item 1
// anticipated it and made the pairing a rule; §3.1 states the property so it is
// testable rather than described; and B22 says in as many words that the
// sentence is the acceptance test WP2 reports against.
//
// A test that only proves the happy path proves the wrong thing here, so the
// adversarial one asserts BOTH HALVES: the impostor is refused at the handshake,
// AND the legitimate peer observes nothing at all.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

// ---------------------------------------------------------------- harness

// credRelay is a relay with a real credential store, plus the secrets a test
// needs in order to speak to it. Every secret in it was minted the way the
// operator's console mints one, so nothing here exercises a path production
// does not have.
type credRelay struct {
	t    *testing.T
	srv  *Server
	url  string
	addr string
	// wireBytes is every byte this relay's listener read or wrote, TCP-side and
	// after any compression the upgrade negotiated (§24, B35). It is the only
	// place a test can see what permessage-deflate actually did: a frame counted
	// after wire.Decode has already been inflated.
	wireBytes atomic.Int64
	store     *peercred.Store
	mu        sync.Mutex
	secrets   map[string]string
}

type credRelayOptions struct {
	minContractVersion string
	dataDir            string
	// limits is §3.3's capacity table (§22, B24). A test that wants to prove a
	// ceiling sets it far below the shipped default, because the shipped one is
	// sized for a migration burst and a test must not have to produce one.
	limits contractb.Limits
	// statsBroadcast is §6.5's timer, which republishes PEER_STATUS BECAUSE
	// STATS CHANGE WITHOUT THE REGISTRY CHANGING. A test that has to prove a
	// peer observed NOTHING sets it out of reach, so that any frame arriving in
	// its window is a registry change and not a heartbeat of the map.
	statsBroadcast time.Duration
	// B29's coalescing window (§22, B29). A churn test sets the floor and the
	// ceiling far below the shipped 250/2000 ms so that a storm fits inside a
	// test's patience; the RATIO is what the bound is about, and it is preserved.
	statusCoalesce    time.Duration
	statusCoalesceMax time.Duration
	churnThreshold    int
	// inMemoryGrid keeps ring.json out of it. The credential store still lives on
	// disk, because that is what a real relay authenticates against; the MAP does
	// not, because Grid.Save fsyncs twice per registry change and a test that
	// wants to measure a CHURN RATE would be measuring this host's disk. §7.4
	// requires the durability in production and says so; nothing about placement
	// arbitration depends on it.
	inMemoryGrid bool
	// noWireCompression stops the relay OFFERING permessage-deflate (§24, B35),
	// which is the operator's kill switch and, for a test, the uncompressed
	// baseline to measure the compressed arm against.
	noWireCompression bool
	// peerTimeout overrides the harness's relaxed 30s liveness window. A focused
	// liveness test can shorten it without changing the production default.
	peerTimeout time.Duration
}

func startCredRelay(t *testing.T, opts credRelayOptions) *credRelay {
	t.Helper()
	dir := opts.dataDir
	if dir == "" {
		dir = t.TempDir()
	}
	store, err := peercred.OpenStore(dir)
	if err != nil {
		t.Fatalf("peercred.OpenStore: %v", err)
	}
	r := &credRelay{t: t, store: store, secrets: map[string]string{}}
	// A relay refuses to start with an empty store (§3.1), so the harness mints
	// the same first credential an operator would.
	r.mint("bootstrap-peer", peercred.GrantPeer)

	statsBroadcast := opts.statsBroadcast
	if statsBroadcast == 0 {
		statsBroadcast = 250 * time.Millisecond
	}
	statusCoalesce := opts.statusCoalesce
	if statusCoalesce == 0 {
		statusCoalesce = 10 * time.Millisecond
	}
	statusCoalesceMax := opts.statusCoalesceMax
	if statusCoalesceMax == 0 {
		// A test that did not ask about B29's widening gets a window that does
		// not widen, so a burst of registry changes cannot silently stretch some
		// other test's timing. The churn tests set both ends deliberately.
		statusCoalesceMax = statusCoalesce
	}
	gridDir := dir
	if opts.inMemoryGrid {
		gridDir = ""
	}
	peerTimeout := opts.peerTimeout
	if peerTimeout == 0 {
		peerTimeout = 30 * time.Second
	}
	srv, err := New(Options{
		Logger:                    slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})),
		DataDir:                   gridDir,
		Credentials:               store,
		MinContractVersion:        opts.minContractVersion,
		Limits:                    opts.limits,
		PingInterval:              time.Second,
		PeerTimeout:               peerTimeout,
		StatusCoalesce:            statusCoalesce,
		StatusCoalesceMax:         statusCoalesceMax,
		StatusChurnBurstThreshold: opts.churnThreshold,
		StatsBroadcast:            statsBroadcast,
		NoWireCompression:         opts.noWireCompression,
	})
	if err != nil {
		t.Fatalf("relay: new: %v", err)
	}
	// 127.0.0.1 on an EPHEMERAL port, always: this suite never touches a port the
	// running rig owns. The listener is wrapped so a test can read the TCP-side
	// byte count; httptest owns the socket, so the wrap has to go on before Start.
	ts := httptest.NewUnstartedServer(srv.Handler())
	ts.Listener = &countingListener{Listener: ts.Listener, n: &r.wireBytes}
	ts.Start()
	t.Cleanup(func() { ts.Close(); srv.Close() })
	r.srv = srv
	r.addr = strings.TrimPrefix(ts.URL, "http://")
	r.url = "ws" + ts.URL[len("http"):] + contractb.ContractBPath
	return r
}

func (r *credRelay) mint(peerID, grant string) string {
	r.t.Helper()
	secret, err := r.store.Mint(peerID, grant)
	if err != nil {
		r.t.Fatalf("mint %s: %v", peerID, err)
	}
	r.mu.Lock()
	r.secrets[peerID] = secret
	r.mu.Unlock()
	return secret
}

func (r *credRelay) secret(peerID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.secrets[peerID]
}

// credPeer is a bare Contract B client that lets a test choose its CREDENTIAL
// and its CLAIMED peerId independently. That separation is the whole point: no
// honest client can produce the pair, and the relay must refuse it anyway.
type credPeer struct {
	t      *testing.T
	ws     *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
	// extensions is Sec-WebSocket-Extensions off the 101, which is the ONLY
	// place the negotiated permessage-deflate is observable: coder/websocket
	// exposes the negotiated subprotocol on the Conn and not the negotiated
	// extension (§24, B35).
	extensions string

	mu     sync.Mutex
	frames []wire.Envelope
	closed bool
	code   int
	reason string
}

type dialSpec struct {
	credentialPeer string // whose credential to present
	credentialFor  string // whose secret to use; defaults to credentialPeer
	claimPeer      string // what the HANDSHAKE frame says
	role           string
	protocol       string
	gameVersion    string
	noCredential   bool
	sendHandshake  bool
	// compression is what this client OFFERS on the upgrade (§24, B35). The zero
	// value is CompressionDisabled, so every test in this package that does not
	// name it dials as an UN-UPDATED SIDECAR does — which is the property B35
	// promises and the reason the default was left where it was.
	compression websocket.CompressionMode
}

func (r *credRelay) dial(spec dialSpec) *credPeer {
	r.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	header := http.Header{}
	if !spec.noCredential {
		who := spec.credentialFor
		if who == "" {
			who = spec.credentialPeer
		}
		header.Set("Authorization", peercred.Header(spec.credentialPeer, r.secret(who)))
	}
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	ws, resp, err := websocket.Dial(dialCtx, r.url, &websocket.DialOptions{
		CompressionMode:      spec.compression,
		CompressionThreshold: wsutil.PeerCompressionThreshold,
		HTTPHeader:           header,
	})
	dialCancel()
	if err != nil {
		cancel()
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		r.t.Fatalf("dial as %s: %v (HTTP %d)", spec.claimPeer, err, status)
	}
	ws.SetReadLimit(wire.MaxFrameBytes)
	p := &credPeer{t: r.t, ws: ws, ctx: ctx, cancel: cancel, code: -1,
		extensions: resp.Header.Get("Sec-WebSocket-Extensions")}
	go p.readLoop()
	r.t.Cleanup(func() { cancel(); _ = ws.CloseNow() })

	if spec.sendHandshake {
		protocol := spec.protocol
		if protocol == "" {
			protocol = wire.ProtocolB
		}
		role := spec.role
		if role == "" {
			role = contractb.RolePeer
		}
		p.send(contractb.TypeHandshake, contractb.Handshake{
			PeerID:          spec.claimPeer,
			Role:            role,
			ProtocolVersion: protocol,
			GameVersion:     spec.gameVersion,
			SidecarVersion:  "test",
		})
	}
	return p
}

func (p *credPeer) readLoop() {
	for {
		_, data, err := p.ws.Read(p.ctx)
		if err != nil {
			p.mu.Lock()
			p.closed = true
			p.code = int(websocket.CloseStatus(err))
			p.reason = err.Error()
			p.mu.Unlock()
			return
		}
		if env, decodeErr := wire.Decode(data); decodeErr == nil {
			p.mu.Lock()
			p.frames = append(p.frames, env)
			p.mu.Unlock()
		}
	}
}

func (p *credPeer) send(typ string, data any) {
	p.t.Helper()
	frame, err := wire.Encode(wire.ProtocolB, typ, time.Now().UnixMilli(), data)
	if err != nil {
		p.t.Fatalf("encode %s: %v", typ, err)
	}
	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()
	_ = p.ws.Write(ctx, websocket.MessageText, frame)
}

func (p *credPeer) waitClosed(timeout time.Duration) (int, string) {
	p.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		p.mu.Lock()
		closed, code, reason := p.closed, p.code, p.reason
		p.mu.Unlock()
		if closed {
			return code, reason
		}
		if time.Now().After(deadline) {
			p.t.Fatalf("the connection was not closed within %s", timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (p *credPeer) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func (p *credPeer) wait(typ string, timeout time.Duration) wire.Envelope {
	p.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if env, ok := p.find(typ); ok {
			return env
		}
		if time.Now().After(deadline) {
			p.t.Fatalf("no %s within %s", typ, timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// findAll returns EVERY frame of a type, which is what a test wanting "did the
// relay ever publish X" must ask. find() returns the first, and a test that
// used it to look for a LATER property — a stats block that only exists after
// the peer has claimed, say — is a test that races the coalescing window and
// loses whenever the window closes between the handshake and the claim.
func (p *credPeer) findAll(typ string) []wire.Envelope {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := []wire.Envelope{}
	for _, env := range p.frames {
		if env.Type == typ {
			out = append(out, env)
		}
	}
	return out
}

func (p *credPeer) find(typ string) (wire.Envelope, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, env := range p.frames {
		if env.Type == typ {
			return env, true
		}
	}
	return wire.Envelope{}, false
}

// waitGrantReason waits for the SECTOR_GRANT carrying a particular reason.
//
// A test that wants the grant ANSWERING ITS OWN CLAIM must ask for it this way
// whenever the peer already holds a reservation. The publisher re-sends grants
// to every live slot holder whose grant body changed (§7.2 ordering step 4) and
// spells every one of them reason:"updated", so a peer with a pre-seeded slot
// routinely receives one of those between its HANDSHAKE_ACK and the answer to
// its claim. Taking the first grant and asserting its reason is a race with the
// coalescing window, and it is a race that gets easier to lose the wider the
// window is allowed to grow (§22, B29).
func (p *credPeer) waitGrantReason(reason string, timeout time.Duration) contractb.SectorGrant {
	p.t.Helper()
	deadline := time.Now().Add(timeout)
	seen := []string{}
	for {
		seen = seen[:0]
		for _, env := range p.findAll(contractb.TypeSectorGrant) {
			var grant contractb.SectorGrant
			if json.Unmarshal(env.Data, &grant) != nil {
				continue
			}
			if grant.Reason == reason {
				return grant
			}
			seen = append(seen, grant.Reason)
		}
		if time.Now().After(deadline) {
			p.t.Fatalf("no SECTOR_GRANT with reason %q within %s; saw %v", reason, timeout, seen)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (p *credPeer) frameCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.frames)
}

func (p *credPeer) statuses() []contractb.PeerStatus {
	p.mu.Lock()
	frames := append([]wire.Envelope(nil), p.frames...)
	p.mu.Unlock()
	out := []contractb.PeerStatus{}
	for _, env := range frames {
		if env.Type != contractb.TypePeerStatus {
			continue
		}
		var st contractb.PeerStatus
		if json.Unmarshal(env.Data, &st) == nil {
			out = append(out, st)
		}
	}
	return out
}

func (p *credPeer) claim() {
	p.send(contractb.TypeSectorClaim, contractb.SectorClaim{
		SimulationSize: 2000, ExportEdges: []string{"E", "N", "W", "S"},
		BorderEdges: []string{"E", "N", "W", "S"}, ModConnected: true,
	})
}

func probe(t *testing.T, addr, credential string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://"+addr+contractb.ContractBPath, nil)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, strings.TrimSpace(resp.Header.Get("WWW-Authenticate"))
}

// ------------------------------------------------------------ B22, Risk 1

// TestRisk1_ValidCredentialWithAnotherPeersID is THE ADVERSARIAL ACCEPTANCE
// TEST of m5_considerations.md, Risk 1, and contract-b-m4.md §3.1's own words:
//
//	"A valid credential for peer-A presented with peerId: "peer-B" is refused AT
//	 THE HANDSHAKE, and peer-B OBSERVES NOTHING AT ALL: no close, no 4006, no
//	 PEER_STATUS change, no lastRefusal on its slot."
//
// Under M4's shared token this exact sequence took the victim off the map in one
// frame, and every peerId it needs is printed on the public status page.
//
// BOTH HALVES ARE ASSERTED, because either one alone proves the wrong thing. A
// refusal that still evicts the victim is the M4 defect wearing a 4003; a
// victim left alone by a relay that admitted the impostor is worse.
func TestRisk1_ValidCredentialWithAnotherPeersID(t *testing.T) {
	// §6.5's stats timer is put out of reach for this test, so "the victim
	// received a frame" means "the registry changed" and nothing else. With the
	// ordinary 5-second republish the assertion would be about a clock.
	r := startCredRelay(t, credRelayOptions{statsBroadcast: time.Hour})
	r.mint("peer-victim", peercred.GrantPeer)
	r.mint("peer-attacker", peercred.GrantPeer)

	// The legitimate peer joins and takes its slot.
	victim := r.dial(dialSpec{credentialPeer: "peer-victim", claimPeer: "peer-victim",
		sendHandshake: true})
	victim.wait(contractb.TypeHandshakeAck, 5*time.Second)
	victim.claim()
	grantEnv := victim.wait(contractb.TypeSectorGrant, 5*time.Second)
	var grant contractb.SectorGrant
	if err := json.Unmarshal(grantEnv.Data, &grant); err != nil {
		t.Fatalf("decode grant: %v", err)
	}
	if !grant.Granted || grant.Slot < 1 {
		t.Fatalf("the legitimate peer did not get a slot: %+v", grant)
	}
	victim.wait(contractb.TypePeerStatus, 5*time.Second)

	// Let the map settle, then record exactly what the victim has seen. Anything
	// after this line that reaches the victim is a violation.
	time.Sleep(300 * time.Millisecond)
	framesBefore := victim.frameCount()
	statusesBefore := victim.statuses()
	if len(statusesBefore) == 0 {
		t.Fatal("the victim received no PEER_STATUS to compare against")
	}
	lastBefore := statusesBefore[len(statusesBefore)-1]

	// THE ATTACK. A VALID credential — the attacker's own, minted by this relay,
	// verifying perfectly — presented with the victim's peerId, which the
	// attacker read off the public status page.
	attacker := r.dial(dialSpec{
		credentialPeer: "peer-attacker",
		claimPeer:      "peer-victim",
		sendHandshake:  true,
	})

	// HALF ONE: refused at the handshake, with 4003 and a reason that names the
	// cause rather than a generic malformed-frame message.
	code, reason := attacker.waitClosed(5 * time.Second)
	if code != contractb.CloseMalformedFrame {
		t.Fatalf("the impostor was closed with %d, want %d (§3.2's 4003 row, added by B22)",
			code, contractb.CloseMalformedFrame)
	}
	if !strings.Contains(reason, "peerId does not match the authenticated credential") {
		t.Fatalf("the close reason was %q; §6.1's worked example names the credential mismatch", reason)
	}
	if _, ok := attacker.find(contractb.TypeHandshakeAck); ok {
		t.Fatal("the impostor was sent a HANDSHAKE_ACK before it was refused")
	}

	// HALF TWO, AND IT IS THE HALF THE M4 TOKEN FAILED. The victim observes
	// NOTHING: it is not closed, it is not replaced with 4006, it receives no new
	// frame at all, and no lastRefusal appears anywhere on the map.
	time.Sleep(500 * time.Millisecond)

	if victim.isClosed() {
		t.Fatal("the legitimate peer's connection was closed by somebody else's handshake; " +
			"this is the M4 one-frame denial of service that contract-b/4 exists to close")
	}
	if got := victim.frameCount(); got != framesBefore {
		after := victim.statuses()
		t.Fatalf("the legitimate peer received %d new frame(s) because of a refusal that had "+
			"nothing to do with it (%d -> %d); §3.1 says it observes NOTHING AT ALL. Last "+
			"status: %+v", got-framesBefore, framesBefore, got, after[len(after)-1])
	}

	// The refusal must not have reached the registry either, so a subscriber's
	// view of the map is unchanged too — checked through a fresh connection, so
	// this is the relay's state and not one client's cached copy.
	r.mint("archive-probe", peercred.GrantSubscribe)
	observer := r.dial(dialSpec{credentialPeer: "archive-probe", claimPeer: "archive-probe",
		role: contractb.RoleArchive, sendHandshake: true})
	observer.wait(contractb.TypeHandshakeAck, 5*time.Second)
	env := observer.wait(contractb.TypePeerStatus, 5*time.Second)
	var now contractb.PeerStatus
	if err := json.Unmarshal(env.Data, &now); err != nil {
		t.Fatalf("decode PEER_STATUS: %v", err)
	}
	if len(now.Slots) != len(lastBefore.Slots) {
		t.Fatalf("the map changed shape across the attack: %d slots -> %d", len(lastBefore.Slots), len(now.Slots))
	}
	for _, slot := range now.Slots {
		if slot.PeerID != "peer-victim" {
			continue
		}
		if !slot.Live {
			t.Fatal("the victim's slot went live:false because of somebody else's refused handshake")
		}
		if slot.LastRefusal != "" {
			t.Fatalf("lastRefusal on the victim's slot is %q. §6.5, B22: A CREDENTIAL FAILURE "+
				"NEVER APPEARS HERE — writing it would tell an innocent peer it had been "+
				"attacked and hand the attacker a confirmation surface for a guessed peerId",
				slot.LastRefusal)
		}
	}
}

// TestBadCredentialIsRefusedBeforeTheUpgrade is §3.1's *Missing, malformed or
// wrong* row: HTTP 401 with WWW-Authenticate: Bearer and NO UPGRADE, so there is
// no WebSocket and therefore no close code. A refusal before the upgrade is the
// one refusal that costs the relay nothing.
func TestBadCredentialIsRefusedBeforeTheUpgrade(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{})
	good := r.mint("peer-one", peercred.GrantPeer)

	for _, c := range []struct {
		name       string
		credential string
	}{
		{"no credential at all", ""},
		{"a known peer with the wrong secret", "peer-one.0000000000000000000000000000000000000000000000000000000000000000"},
		{"an unknown peer with a real secret", "peer-nobody." + good},
		{"no separator", "peer-one"},
		{"an empty secret", "peer-one."},
		{"an empty peerId", "." + good},
	} {
		status, challenge := probe(t, r.addr, c.credential)
		if status != http.StatusUnauthorized {
			t.Fatalf("%s: HTTP %d, want 401", c.name, status)
		}
		if challenge != "Bearer" {
			t.Fatalf("%s: WWW-Authenticate is %q, want \"Bearer\"", c.name, challenge)
		}
	}

	// And the credential that IS right still works, so the test is measuring the
	// check rather than a relay that refuses everything.
	status, _ := probe(t, r.addr, "peer-one."+good)
	if status == http.StatusUnauthorized {
		t.Fatal("a valid credential was refused; the check refuses everything and proves nothing")
	}
}

// TestCredentialStoreSurvivesARelayRestart is the durability half of §12's
// `credentialVerifierStore`. A credential the relay forgets on restart is a peer
// that can never come back, against a reservation that NEVER EXPIRES (D8) and a
// journal full of entries addressed to its slot — so this is a custody property
// wearing an authentication costume.
func TestCredentialStoreSurvivesARelayRestart(t *testing.T) {
	dir := t.TempDir()
	first := startCredRelay(t, credRelayOptions{dataDir: dir})
	secret := first.mint("peer-durable", peercred.GrantPeer)

	// A second relay over the same data directory is what a restart is.
	reopened, err := peercred.OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	peerID, grant, ok := reopened.Verify(peercred.Join("peer-durable", secret))
	if !ok {
		t.Fatal("the credential did not survive a restart; the peer holding this join string " +
			"could never come back, and its slot never expires")
	}
	if peerID != "peer-durable" || grant != peercred.GrantPeer {
		t.Fatalf("reopened credential is (%q,%q), want (peer-durable,peer)", peerID, grant)
	}

	// THE SECRET ITSELF MUST NOT BE IN THE FILE (§3.1): a relay whose store is
	// read must not thereby hand over every peer's identity.
	raw, err := readFileString(reopened.Path())
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if strings.Contains(raw, secret) {
		t.Fatal("the credential store contains the secret in recoverable form; §3.1 requires a " +
			"salted VERIFIER from which the join string cannot be recovered")
	}
}

// TestRelayRefusesToStartWithNoCredentials is §3.1's *No credential store
// configured* row. A relay with an empty store can admit nobody, so serving is a
// listener that 401s every peer on the map — a failure that looks like a network
// problem and is a configuration one.
func TestRelayRefusesToStartWithNoCredentials(t *testing.T) {
	empty, err := New(Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer empty.Close()
	if err := empty.CheckServable(); err == nil {
		t.Fatal("a relay with an empty credential store would serve, and 401 every peer on the map")
	}
	// The check is on SERVING and not on construction, deliberately: minting the
	// first join string needs the same Server, and a relay that could not be
	// built before it had a credential could never be given one.
	if _, mintErr := empty.Credentials().Mint("peer-first", peercred.GrantPeer); mintErr != nil {
		t.Fatalf("the first credential could not be minted: %v", mintErr)
	}
	if err := empty.CheckServable(); err != nil {
		t.Fatalf("a relay with one credential still refuses to serve: %v", err)
	}

	insecure, err := New(Options{InsecureNoToken: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer insecure.Close()
	if err := insecure.CheckServable(); err != nil {
		t.Fatalf("--insecure-no-token should serve a single-machine test rig: %v", err)
	}
}

// TestOnlyTheAuthenticatedPeerCanReplaceItself is §3.2's 4006 row as B22
// narrowed it. The self-healing rule survives — a crashed-and-restarted sidecar
// still takes its own slot back — and the impersonation it permitted does not.
func TestOnlyTheAuthenticatedPeerCanReplaceItself(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{})
	r.mint("peer-self", peercred.GrantPeer)
	r.mint("peer-other", peercred.GrantPeer)

	first := r.dial(dialSpec{credentialPeer: "peer-self", claimPeer: "peer-self", sendHandshake: true})
	first.wait(contractb.TypeHandshakeAck, 5*time.Second)

	// The impersonation attempt does NOT produce a 4006 on the live connection.
	impostor := r.dial(dialSpec{credentialPeer: "peer-other", claimPeer: "peer-self", sendHandshake: true})
	if code, _ := impostor.waitClosed(5 * time.Second); code != contractb.CloseMalformedFrame {
		t.Fatalf("the impostor got close %d, want 4003", code)
	}
	time.Sleep(300 * time.Millisecond)
	if first.isClosed() {
		t.Fatal("a 4006 fired for a connection that did NOT authenticate as this peerId")
	}

	// The peer replacing ITSELF still works: this is contract-a.md §2's
	// self-healing rule, kept.
	second := r.dial(dialSpec{credentialPeer: "peer-self", claimPeer: "peer-self", sendHandshake: true})
	second.wait(contractb.TypeHandshakeAck, 5*time.Second)
	if code, _ := first.waitClosed(5 * time.Second); code != contractb.CloseReplaced {
		t.Fatalf("the old connection got close %d, want 4006 REPLACED; a crashed-and-restarted "+
			"sidecar must still be able to come back", code)
	}
}

// ------------------------------------------------------------ B27

// TestSubscriberNeedsTheSubscribeGrant is B27's worked example. role "archive"
// is no longer a self-declaration: under M4 anyone holding the one token could
// open a socket, declare themselves a subscriber and receive a byte-identical
// copy of EVERY envelope on the map — which since §16 carries every world's
// species census and since §19 its mod version, save policy and exclusion list.
func TestSubscriberNeedsTheSubscribeGrant(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{})
	r.mint("peer-lan-slot5", peercred.GrantPeer)
	r.mint("archive-main", peercred.GrantSubscribe)

	// A PEER credential asking to subscribe is refused, and the reason names the
	// missing grant.
	wrong := r.dial(dialSpec{credentialPeer: "peer-lan-slot5", claimPeer: "peer-lan-slot5",
		role: contractb.RoleArchive, sendHandshake: true})
	code, reason := wrong.waitClosed(5 * time.Second)
	if code != contractb.CloseMalformedFrame {
		t.Fatalf("a peer credential subscribing got close %d, want 4003", code)
	}
	if !strings.Contains(reason, "subscribe grant") {
		t.Fatalf("the close reason is %q; B27's example names the missing grant", reason)
	}

	// THE GRANTS ARE DISJOINT IN BOTH DIRECTIONS: a subscribe credential cannot
	// claim a slot either, so neither compromise becomes the other.
	alsoWrong := r.dial(dialSpec{credentialPeer: "archive-main", claimPeer: "archive-main",
		role: contractb.RolePeer, sendHandshake: true})
	code, reason = alsoWrong.waitClosed(5 * time.Second)
	if code != contractb.CloseMalformedFrame {
		t.Fatalf("a subscribe credential taking role peer got close %d, want 4003", code)
	}
	if !strings.Contains(reason, "peer grant") {
		t.Fatalf("the close reason is %q; it must name the grant the role needs", reason)
	}

	// An admin credential holds NEITHER role: the third grant reaches B28's
	// off-wire path and nothing on this one.
	r.mint("ops-admin", peercred.GrantAdmin)
	for _, role := range []string{contractb.RolePeer, contractb.RoleArchive} {
		adm := r.dial(dialSpec{credentialPeer: "ops-admin", claimPeer: "ops-admin",
			role: role, sendHandshake: true})
		if code, _ := adm.waitClosed(5 * time.Second); code != contractb.CloseMalformedFrame {
			t.Fatalf("an admin credential took role %q with close %d, want 4003", role, code)
		}
	}

	// And the credential that carries the grant subscribes normally.
	right := r.dial(dialSpec{credentialPeer: "archive-main", claimPeer: "archive-main",
		role: contractb.RoleArchive, sendHandshake: true})
	right.wait(contractb.TypeHandshakeAck, 5*time.Second)
	if right.isClosed() {
		t.Fatal("a subscribe credential was refused role archive; the gate refuses everything")
	}
}

// TestARoleErrorIsNotARefusalOfThatPeer is the second half of B27's example, and
// it is a distinction the taxonomy depends on: "nothing appears on slot 5's
// lastRefusal, because slot 5's PEER connection was refused nothing — this is a
// role error on ONE CONNECTION, not a refusal of that peer."
func TestARoleErrorIsNotARefusalOfThatPeer(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{})
	r.mint("peer-lan-slot5", peercred.GrantPeer)
	r.mint("archive-main", peercred.GrantSubscribe)

	peer := r.dial(dialSpec{credentialPeer: "peer-lan-slot5", claimPeer: "peer-lan-slot5",
		sendHandshake: true})
	peer.wait(contractb.TypeHandshakeAck, 5*time.Second)
	peer.claim()
	peer.wait(contractb.TypeSectorGrant, 5*time.Second)

	// The same peer, on a SECOND connection, asks for a role its grant does not
	// carry.
	bad := r.dial(dialSpec{credentialPeer: "peer-lan-slot5", claimPeer: "peer-lan-slot5",
		role: contractb.RoleArchive, sendHandshake: true})
	bad.waitClosed(5 * time.Second)
	time.Sleep(300 * time.Millisecond)

	if peer.isClosed() {
		t.Fatal("a role error on one connection closed the peer's other connection")
	}
	sub := r.dial(dialSpec{credentialPeer: "archive-main", claimPeer: "archive-main",
		role: contractb.RoleArchive, sendHandshake: true})
	env := sub.wait(contractb.TypePeerStatus, 5*time.Second)
	var st contractb.PeerStatus
	if err := json.Unmarshal(env.Data, &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, slot := range st.Slots {
		if slot.PeerID == "peer-lan-slot5" && slot.LastRefusal != "" {
			t.Fatalf("lastRefusal is %q after a ROLE error; B27 says nothing appears there", slot.LastRefusal)
		}
	}
}

// TestSubscriberSeesNothingAPeerDoesNot is B27's *not a privileged view* row,
// which is what makes the visibility boundary describable in one sentence:
// every field a subscriber reads is a field the relay already broadcasts to
// every sidecar on the map. There is no subscriber-only field, no private
// channel and no back door — and the peer's own rule follows from it: NOTHING ON
// THIS WIRE IS CONFIDENTIAL, so a sidecar that must not publish a value must not
// put it on the stats block.
func TestSubscriberSeesNothingAPeerDoesNot(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{})
	r.mint("peer-a", peercred.GrantPeer)
	r.mint("archive-main", peercred.GrantSubscribe)

	peer := r.dial(dialSpec{credentialPeer: "peer-a", claimPeer: "peer-a", sendHandshake: true})
	peer.wait(contractb.TypeHandshakeAck, 5*time.Second)
	peer.claim()
	peer.wait(contractb.TypeSectorGrant, 5*time.Second)

	sub := r.dial(dialSpec{credentialPeer: "archive-main", claimPeer: "archive-main",
		role: contractb.RoleArchive, sendHandshake: true})
	sub.wait(contractb.TypeHandshakeAck, 5*time.Second)

	time.Sleep(400 * time.Millisecond)
	peerStatuses := peer.statuses()
	subStatuses := sub.statuses()
	if len(peerStatuses) == 0 || len(subStatuses) == 0 {
		t.Fatalf("no PEER_STATUS to compare (peer %d, subscriber %d)", len(peerStatuses), len(subStatuses))
	}
	fromPeer := peerStatuses[len(peerStatuses)-1]
	fromSub := subStatuses[len(subStatuses)-1]

	// The slot array is the payload B27 is about, and it must be identical.
	peerSlots, _ := json.Marshal(fromPeer.Slots)
	subSlots, _ := json.Marshal(fromSub.Slots)
	if string(peerSlots) != string(subSlots) {
		t.Fatalf("a subscriber's slot view differs from a peer's.\npeer: %s\nsub:  %s", peerSlots, subSlots)
	}
	// §6.5: `you` is ALL NULL for a subscriber, and that is the one difference —
	// a subscriber owns no world, so it has no place in the map.
	if fromSub.You.Slot != nil || fromSub.You.Position != nil {
		t.Fatalf("a subscriber's `you` is %+v, want all null (§6.5)", fromSub.You)
	}
	if fromSub.Observers != 1 {
		t.Fatalf("observers is %d, want 1 — the count of subscribe grants an operator DECIDED "+
			"to issue, not of sockets that knew a token (§6.5, B27)", fromSub.Observers)
	}
}

// ------------------------------------------------------------ B25

// TestMinContractVersionDefaultsToNoMinimum is B25's *It defaults to nothing*
// row: unset means no minimum, and a relay that has not made a deployment
// decision must not enforce a guess.
func TestMinContractVersionDefaultsToNoMinimum(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{})
	if got := r.srv.MinContractVersion(); got != "" {
		t.Fatalf("minContractVersion defaults to %q, want unset", got)
	}
	r.mint("peer-a", peercred.GrantPeer)
	p := r.dial(dialSpec{credentialPeer: "peer-a", claimPeer: "peer-a", sendHandshake: true})
	env := p.wait(contractb.TypeHandshakeAck, 5*time.Second)

	// Absent, not empty-string-present: a reader must be able to tell "no floor"
	// from "a floor I could not parse".
	var raw map[string]any
	if err := json.Unmarshal(env.Data, &raw); err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	if _, present := raw["minContractVersion"]; present {
		t.Fatal("HANDSHAKE_ACK carries minContractVersion when no floor is set; absent means no minimum")
	}
}

// TestContractVersionFloorRefusesWithTheRightAxis is B25's gate and §6.5's
// lastRefusal axis together, because the refusal is only actionable beside the
// minimum that produced it. It also pins the half an implementer forgets: the
// remedy names WHO MUST ACT, and it is not the relay's operator.
func TestContractVersionFloorRefusesWithTheRightAxis(t *testing.T) {
	// A floor one minor above what this build speaks, so the peer under test is
	// this build itself — the honest shape of "a peer one release behind".
	const floor = "contract-b/4.2"
	r := startCredRelay(t, credRelayOptions{minContractVersion: floor})
	r.mint("peer-stale", peercred.GrantPeer)
	r.mint("peer-current", peercred.GrantPeer)
	r.mint("archive-main", peercred.GrantSubscribe)

	// The stale peer must already hold a slot, or there is no slot for the
	// refusal to appear on.
	stale := r.dial(dialSpec{credentialPeer: "peer-stale", claimPeer: "peer-stale",
		protocol: floor, sendHandshake: true})
	stale.wait(contractb.TypeHandshakeAck, 5*time.Second)
	stale.claim()
	stale.wait(contractb.TypeSectorGrant, 5*time.Second)
	stale.cancel()
	_ = stale.ws.CloseNow()

	// Now it comes back on the version it actually ships.
	refused := r.dial(dialSpec{credentialPeer: "peer-stale", claimPeer: "peer-stale",
		protocol: "contract-b/4.0", sendHandshake: true})
	code, reason := refused.waitClosed(5 * time.Second)
	if code != contractb.CloseMalformedFrame {
		t.Fatalf("a peer below the floor was closed with %d, want 4003 (§3.2, B25)", code)
	}
	if !strings.Contains(reason, "contract-b/4.0") || !strings.Contains(reason, floor) {
		t.Fatalf("the close reason is %q; §6.1's example names BOTH versions", reason)
	}

	// The refusal reaches the slot, so a stale peer does not read as a dead one —
	// which is the same reason §6.1's game-version refusal has always written
	// lastRefusal.
	// The subscriber dials AT THE FLOOR, because the floor is an admission policy
	// of the whole map and the archive is a client of the same wire: an operator
	// who raises it upgrades their own archive with everything else (§5.1, B25).
	sub := r.dial(dialSpec{credentialPeer: "archive-main", claimPeer: "archive-main",
		role: contractb.RoleArchive, protocol: floor, sendHandshake: true})
	sub.wait(contractb.TypeHandshakeAck, 5*time.Second)
	deadline := time.Now().Add(5 * time.Second)
	var found string
	for time.Now().Before(deadline) {
		for _, st := range sub.statuses() {
			for _, slot := range st.Slots {
				if slot.PeerID == "peer-stale" && slot.LastRefusal != "" {
					found = slot.LastRefusal
				}
			}
			if st.MinContractVersion != floor {
				t.Fatalf("PEER_STATUS publishes minContractVersion %q, want %q (§6.5, B25)",
					st.MinContractVersion, floor)
			}
		}
		if found != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if found == "" {
		t.Fatal("no lastRefusal appeared on the refused peer's slot; a stale peer would read as a dead one")
	}
	if !strings.HasPrefix(found, contractb.RefusalContractVersion) {
		t.Fatalf("lastRefusal is %q, want the %q axis (§6.5, B25)", found, contractb.RefusalContractVersion)
	}
	if !strings.Contains(found, "contract-b/4.0 < "+floor) {
		t.Fatalf("lastRefusal is %q; the string carries both versions so the remedy is legible", found)
	}

	// A peer AT the floor is admitted, so the gate is a floor and not a wall.
	ok := r.dial(dialSpec{credentialPeer: "peer-current", claimPeer: "peer-current",
		protocol: floor, sendHandshake: true})
	ack := ok.wait(contractb.TypeHandshakeAck, 5*time.Second)
	var decoded contractb.HandshakeAck
	if err := json.Unmarshal(ack.Data, &decoded); err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	if decoded.MinContractVersion != floor {
		t.Fatalf("HANDSHAKE_ACK publishes minContractVersion %q, want %q (§6.2, B25)",
			decoded.MinContractVersion, floor)
	}
}

// TestTheFloorIsNotASecurityControl is the sentence B25 insists must sit in the
// same paragraph as the gate itself, written as a test so it cannot be read as
// decoration: A PEER THAT EDITS ONE STRING WALKS THROUGH THIS GATE. The gate
// keeps HONEST stale peers off a map they would degrade and stops nobody else,
// and an implementer who assumes the first half implies the second has built a
// security control out of attacker-chosen text.
func TestTheFloorIsNotASecurityControl(t *testing.T) {
	const floor = "contract-b/4.2"
	r := startCredRelay(t, credRelayOptions{minContractVersion: floor})
	r.mint("peer-liar", peercred.GrantPeer)

	// A peer that simply CLAIMS the floor is admitted. Nothing about the frames
	// it then sends has to match the claim, because protocolVersion is a string
	// the sender chooses (§13 item 7's rule, applied to the second axis).
	liar := r.dial(dialSpec{credentialPeer: "peer-liar", claimPeer: "peer-liar",
		protocol: "contract-b/4.9", sendHandshake: true})
	liar.wait(contractb.TypeHandshakeAck, 5*time.Second)
	if liar.isClosed() {
		t.Fatal("a peer claiming a version above the floor was refused; the gate is a " +
			"COMPATIBILITY control and has no way to check a claim")
	}
}

// TestAMajorMismatchIsStill4000 pins the boundary between B25's new refusal and
// the one §3.2 has always had. A different MAJOR is 4000 PROTOCOL_UNSUPPORTED
// and the client must not reconnect until it is restarted; a minor below the
// floor is 4003 and the client reconnects with backoff. Collapsing the two would
// make an upgradeable peer give up and a doomed one hammer.
func TestAMajorMismatchIsStill4000(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{minContractVersion: "contract-b/4.0"})
	r.mint("peer-old", peercred.GrantPeer)
	old := r.dial(dialSpec{credentialPeer: "peer-old", claimPeer: "peer-old",
		protocol: "contract-b/3.5", sendHandshake: true})
	if code, _ := old.waitClosed(5 * time.Second); code != contractb.CloseProtocolUnsupported {
		t.Fatalf("a contract-b/3 peer got close %d, want 4000", code)
	}
}

// ------------------------------------------------------------ B32

// TestEveryRetiredPathAnswersWith4000 is §3 and B32: a relay MUST keep serving
// every retired path and MUST close every connection on one immediately with
// 4000. /contract-b/v3 joins /contract-b/v2 on that list with this major, and
// the reason is unchanged — A BARE HTTP 404 IS A SOCKET ERROR IN A LOG AND HALF
// AN EVENING OF DIAGNOSIS.
func TestEveryRetiredPathAnswersWith4000(t *testing.T) {
	r := startCredRelay(t, credRelayOptions{})
	if len(contractb.RetiredContractBPaths) < 2 {
		t.Fatalf("the retired path list is %v; /contract-b/v3 must be on it after B32",
			contractb.RetiredContractBPaths)
	}
	for _, path := range contractb.RetiredContractBPaths {
		url := "ws://" + r.addr + path
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ws, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
			CompressionMode: websocket.CompressionDisabled,
		})
		cancel()
		if err != nil {
			t.Fatalf("%s: dial failed (%v); a retired path must ANSWER, because the point of a "+
				"retired path is that it teaches", path, err)
		}
		readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _, err = ws.Read(readCtx)
		readCancel()
		if got := websocket.CloseStatus(err); int(got) != contractb.CloseProtocolUnsupported {
			t.Fatalf("%s closed with %d, want 4000 PROTOCOL_UNSUPPORTED", path, got)
		}
		_ = ws.CloseNow()
	}

	// A retired path takes NO credential, deliberately: a peer that cannot
	// complete a handshake should still learn why, and a 401 would teach it the
	// wrong lesson.
	if contractb.ContractBPath != "/contract-b/v4" {
		t.Fatalf("the live path is %q, want /contract-b/v4 (B32)", contractb.ContractBPath)
	}
	// THE MAJOR IS WHAT THE PATH IS BOUND TO, and the minor deliberately is not
	// (§4). Pinning the whole identifier here made a minor bump — §25's
	// contract-b/4.1, which changes no frame — fail a test about retired paths.
	if !strings.HasPrefix(wire.ProtocolB, "contract-b/4.") {
		t.Fatalf("the protocol identifier is %q, want a contract-b/4.x on /contract-b/v4 (B32)",
			wire.ProtocolB)
	}
}

// readFileString is a one-line helper so the store-durability test can assert on
// the file's bytes rather than on a re-parse of them: a verifier that round-trips
// is not the property §3.1 asks for.
func readFileString(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}
