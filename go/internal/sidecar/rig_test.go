package sidecar

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"multiverse/internal/archive"
	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/relay"
)

// testToken is the shared LAN token every rig uses. contract-b-m4.md §3.1 wants
// 16 to 256 bytes of printable ASCII; 32 random bytes hex-encoded is the
// RECOMMENDED shape, and a fixed one keeps the tests reproducible.
const testToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// TestMain doubles as the helper-process entry point for the crash-custody
// test: the test binary re-executes itself as a real sidecar process so a test
// can SIGKILL it mid-migration.
func TestMain(m *testing.M) {
	if os.Getenv("MULTIVERSE_TEST_HELPER") == "sidecar" {
		args := strings.Split(os.Getenv("MULTIVERSE_TEST_ARGS"), "\x1f")
		os.Exit(Main(args, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

func testLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(newTestWriter(t), &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// testWriter routes a component's slog output into the test log, and STOPS
// doing so once the test is over.
//
// A relay connection handler, a sidecar tick and a mod pinger all outlive the
// test function by a few milliseconds, and testing.T.Logf after a test has
// finished is a data race in the harness rather than in the code under test.
// The flag is set by a cleanup registered before every component's own, so it
// runs last: everything shut down by a cleanup still logs, and anything after
// that is dropped.
type testWriter struct {
	mu   sync.Mutex
	t    *testing.T
	done bool
}

func newTestWriter(t *testing.T) *testWriter {
	w := &testWriter{t: t}
	t.Cleanup(func() {
		w.mu.Lock()
		w.done = true
		w.mu.Unlock()
	})
	return w
}

func (w *testWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.done {
		return len(p), nil
	}
	w.t.Logf("%s", strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// trackingListener remembers every accepted connection so the relay-drop test
// can sever them outright. A hijacked WebSocket leaves net/http's connection
// tracking, so http.Server.Close cannot reach it.
type trackingListener struct {
	net.Listener
	mu    sync.Mutex
	conns []net.Conn
}

func (l *trackingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.conns = append(l.conns, c)
	l.mu.Unlock()
	return c, nil
}

func (l *trackingListener) severAll() {
	l.mu.Lock()
	conns := l.conns
	l.conns = nil
	l.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

// testRelay is an in-process relay that can be killed and restarted on the same
// port. Its map is durable in a temp directory, so a restart reclaims the same
// slots and the same positions (contract-b-m4.md §7.4) — and mints a NEW
// relaySessionId, which is exactly what invalidates every outstanding proof
// (§5.2).
type testRelay struct {
	t       *testing.T
	addr    string
	dataDir string
	token   string
	srv     *http.Server
	ln      *trackingListener
	relay   *relay.Server
}

func startRelay(t *testing.T) *testRelay {
	t.Helper()
	return startRelayToken(t, testToken)
}

func startRelayToken(t *testing.T, token string) *testRelay {
	t.Helper()
	// 127.0.0.1:0 only. The rig never binds a fixed port and never touches the
	// ports the running game owns.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("relay listen: %v", err)
	}
	r := &testRelay{t: t, addr: ln.Addr().String(), dataDir: t.TempDir(), token: token}
	r.serve(ln)
	t.Cleanup(r.kill)
	return r
}

func (r *testRelay) serve(ln net.Listener) {
	srv, err := relay.New(relay.Options{
		Logger:  testLogger(r.t),
		DataDir: r.dataDir,
		Token:   r.token,
		// The contract's own behaviour on a short clock: the coalescing window
		// and the stats republish move, and no rule they gate does.
		PingInterval:   200 * time.Millisecond,
		PeerTimeout:    3 * time.Second,
		StatusCoalesce: 40 * time.Millisecond,
		StatsBroadcast: 500 * time.Millisecond,
	})
	if err != nil {
		r.t.Fatalf("relay new: %v", err)
	}
	r.relay = srv
	r.srv = &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	r.ln = &trackingListener{Listener: ln}
	tracked := r.ln
	go func() { _ = r.srv.Serve(tracked) }()
}

func (r *testRelay) url() string { return "ws://" + r.addr + contractb.ContractBPath }

// kill drops the relay and every connection on it, the way kill -9 would: no
// close frame, no drain.
func (r *testRelay) kill() {
	if r.srv == nil {
		return
	}
	_ = r.srv.Close()
	r.ln.severAll()
	r.relay.Close()
	r.srv = nil
	r.ln = nil
}

// restart brings the relay back on the same address and the same ring.json,
// with a NEW session id.
func (r *testRelay) restart() {
	r.t.Helper()
	var ln net.Listener
	var err error
	for i := 0; i < 50; i++ {
		ln, err = net.Listen("tcp", r.addr)
		if err == nil {
			r.serve(ln)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.t.Fatalf("relay restart on %s: %v", r.addr, err)
}

// fastConfig is the contract's own behaviour on a short clock, so the suite
// finishes in seconds instead of a day. Only the timers move; every rule the
// tests exercise is the shipped one.
func fastConfig(t *testing.T, relayURL, peerID string) Config {
	cfg := DefaultConfig()
	cfg.Listen = "127.0.0.1:0"
	cfg.RelayURL = relayURL
	cfg.PeerID = peerID
	cfg.Token = testToken
	cfg.DataDir = t.TempDir()
	cfg.Logger = testLogger(t)
	cfg.HeartbeatTimeout = 1500 * time.Millisecond
	cfg.WSPingInterval = 2 * time.Second
	cfg.WSPongTimeout = 2 * time.Second
	cfg.MigrateInAckTimeout = 30 * time.Second
	cfg.RelayBackoffMin = 30 * time.Millisecond
	cfg.RelayBackoffMax = 150 * time.Millisecond
	cfg.ForwardRetry = 300 * time.Millisecond
	cfg.BounceTimeout = 700 * time.Millisecond
	cfg.StatsInterval = 200 * time.Millisecond
	cfg.TickInterval = 40 * time.Millisecond
	// The pacing default is 100.0 per SIMULATED minute since §18 A40's second
	// raise — sized two orders below A29's ingest ceiling, and meant to be
	// invisible to ordinary traffic. A test rig moves in seconds, so even that
	// would gate every test on the burst; the burst is what "ordinary traffic is
	// never delayed" means, and the tests that assert PACING set the rate
	// themselves.
	cfg.InboundRatePerSimMinute = 6000
	cfg.InboundRateBurst = 64
	return cfg
}

func startSidecar(t *testing.T, cfg Config) *Sidecar {
	t.Helper()
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("sidecar %s: new: %v", cfg.PeerID, err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("sidecar %s: start: %v", cfg.PeerID, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// node is one map member: a sidecar, its fake mod and that mod's world.
type node struct {
	side  *Sidecar
	mod   *fakeMod
	world *fakeWorld
	cfg   Config
	slot  int
	pos   contractb.Position
}

// grid is the standard M4 rig: one relay and a rectangle of slots, every sim
// declaring ALL FOUR edges and therefore both exporting and receiving on each
// of them (D17, contract-a.md §18 A38). A test that wants D13's two-edge world
// — still legal, and the mixed-rig case §18 A41 checks — names it in
// gridOptions.exportEdges.
type grid struct {
	t     *testing.T
	relay *testRelay
	nodes []*node
}

type gridOptions struct {
	// layout names each peer's preferredPosition, in start order. A rig that
	// wants a SPECIFIC map names it (§7.2); an empty layout leaves every claim
	// opinion-free and exercises auto-placement instead.
	layout []contractb.Position
	// tune adjusts the config of node i before it starts.
	tune func(i int, c *Config)
	// heartbeat is each mod's HEARTBEAT cadence.
	heartbeat time.Duration
	// exportEdges overrides the mod's declared export edges.
	exportEdges []string
	// noMods leaves the sidecars without mods.
	noMods bool
	// skipEdgeCheck skips waiting for the east lane of every peer to open.
	skipEdgeCheck bool
}

// layout3x2 is the exit test's map: row 0 holds slots 1-3, row 1 holds slots 4-6.
func layout3x2() []contractb.Position {
	return []contractb.Position{
		{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 2, Row: 0},
		{Col: 0, Row: 1}, {Col: 1, Row: 1}, {Col: 2, Row: 1},
	}
}

// layoutRow is a one-row map of n slots: the M3 ring, which §2 keeps as the
// height-1 specialization of the grid.
func layoutRow(n int) []contractb.Position {
	out := make([]contractb.Position, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, contractb.Position{Col: i, Row: 0})
	}
	return out
}

func newGrid(t *testing.T, n int, opts gridOptions) *grid {
	t.Helper()
	if opts.heartbeat == 0 {
		opts.heartbeat = 200 * time.Millisecond
	}
	g := &grid{t: t, relay: startRelay(t)}
	for i := 0; i < n; i++ {
		var at *contractb.Position
		if i < len(opts.layout) {
			p := opts.layout[i]
			at = &p
		}
		g.addPeer(fmt.Sprintf("peer-slot%d", i+1), at, opts)
	}
	if !opts.noMods && !opts.skipEdgeCheck {
		for _, nd := range g.nodes {
			nd.mod.waitEdge(contracta.EdgeE, true, 10*time.Second)
		}
	}
	return g
}

// addPeer starts one more sidecar and waits for its slot, so placement is
// deterministic in start order.
func (g *grid) addPeer(peerID string, at *contractb.Position, opts gridOptions) *node {
	g.t.Helper()
	if opts.heartbeat == 0 {
		opts.heartbeat = 200 * time.Millisecond
	}
	cfg := fastConfig(g.t, g.relay.url(), peerID)
	cfg.PreferredPosition = at
	if opts.tune != nil {
		opts.tune(len(g.nodes), &cfg)
	}
	nd := &node{cfg: cfg, world: newWorld()}
	nd.side = startSidecar(g.t, cfg)
	nd.slot = waitSlotAny(g.t, nd.side)
	if !opts.noMods {
		slot := nd.slot
		nd.mod = dialFakeMod(g.t, fakeModOptions{
			url: nd.side.URL(), world: nd.world, ringSlot: &slot,
			exportEdges: opts.exportEdges, heartbeat: opts.heartbeat})
	}
	// The position arrives with the grant that follows the claim.
	waitFor(g.t, 10*time.Second, "a position for "+peerID, func() bool {
		nd.pos = nd.side.Position()
		return nd.side.Slot() > 0
	})
	g.nodes = append(g.nodes, nd)
	return nd
}

func (g *grid) node(i int) *node { return g.nodes[i] }

// addPeerWithSplice starts one more sidecar carrying §7.2 rule 5's advisory
// splice: "place me immediately after this slot on this axis".
func (g *grid) addPeerWithSplice(peerID string, afterSlot int, axis string) *node {
	g.t.Helper()
	cfg := fastConfig(g.t, g.relay.url(), peerID)
	cfg.InsertAfterSlot = afterSlot
	cfg.InsertAxis = axis
	nd := &node{cfg: cfg, world: newWorld()}
	nd.side = startSidecar(g.t, cfg)
	nd.slot = waitSlotAny(g.t, nd.side)
	slot := nd.slot
	nd.mod = dialFakeMod(g.t, fakeModOptions{
		url: nd.side.URL(), world: nd.world, ringSlot: &slot, heartbeat: 200 * time.Millisecond})
	nd.pos = nd.side.Position()
	g.nodes = append(g.nodes, nd)
	return nd
}

// bySlot finds a node by the slot the relay granted it.
func (g *grid) bySlot(slot int) *node {
	for _, nd := range g.nodes {
		if nd.slot == slot {
			return nd
		}
	}
	g.t.Fatalf("no node holds slot %d", slot)
	return nil
}

func waitSlotAny(t *testing.T, s *Sidecar) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if got := s.Slot(); got > 0 {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("sidecar %s never got a slot", s.PeerID())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitSlot(t *testing.T, s *Sidecar, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if got := s.Slot(); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sidecar %s did not get slot %d (has %d)", s.PeerID(), want, s.Slot())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitLane waits until the effective neighbour on one export edge is the wanted
// slot. It is the M4 replacement for M3's waitEastSlot, once per axis.
func waitLane(t *testing.T, s *Sidecar, edge string, want int) *contractb.Neighbour {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if n := s.Neighbour(edge); n != nil && n.Slot == want {
			return n
		}
		if time.Now().After(deadline) {
			t.Fatalf("sidecar %s %s lane is %d, want slot %d",
				s.PeerID(), edge, laneSlotOf(s, edge), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitLaneClosed waits until one export edge has no effective neighbour, which
// is what a degenerate axis produces (§2.1).
func waitLaneClosed(t *testing.T, s *Sidecar, edge string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if s.Neighbour(edge) == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sidecar %s %s lane is still slot %d, want closed",
				s.PeerID(), edge, laneSlotOf(s, edge))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func laneSlotOf(s *Sidecar, edge string) int {
	if n := s.Neighbour(edge); n != nil {
		return n.Slot
	}
	return 0
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", timeout, what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// custodyOf finds one migration's journal state, or "absent".
func custodyOf(s *Sidecar, migrationID string) string {
	for _, st := range s.CustodySnapshot() {
		if st.Entry.MigrationID == migrationID {
			return fmt.Sprintf("%s/%s", st.Direction, st.Status)
		}
	}
	return "absent"
}

// handoffOf is the M4 half of the same question: §9.2's durable handoff state.
func handoffOf(s *Sidecar, migrationID string) journal.Handoff {
	for _, st := range s.CustodySnapshot() {
		if st.Entry.MigrationID == migrationID {
			return st.Handoff
		}
	}
	return ""
}

// journalEntry returns one migration's journal state, for the annex and handoff
// assertions.
func journalEntry(t *testing.T, s *Sidecar, migrationID string) *journal.State {
	t.Helper()
	for _, st := range s.CustodySnapshot() {
		if st.Entry.MigrationID == migrationID {
			return st
		}
	}
	t.Fatalf("sidecar %s has no journal entry for %s", s.PeerID(), migrationID)
	return nil
}

// startArchive brings up an in-process multiverse-archive against the rig's
// relay, on a short retry ladder so a test does not wait a minute for the first
// re-ask, with the status page on an ephemeral port.
func startArchive(t *testing.T, relayURL string) *archive.Archive {
	t.Helper()
	a, err := archive.New(archive.Config{
		RelayURL:          relayURL,
		Token:             testToken,
		PeerID:            "archive-test",
		DataDir:           t.TempDir(),
		Logger:            testLogger(t),
		RelayBackoffMin:   30 * time.Millisecond,
		RelayBackoffMax:   150 * time.Millisecond,
		FirstAttemptDelay: 0,
		RetrySchedule:     []time.Duration{200 * time.Millisecond, 500 * time.Millisecond},
		TickInterval:      50 * time.Millisecond,
		HTTPListen:        "127.0.0.1:0",
		MetricsInterval:   200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("archive: new: %v", err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("archive: start: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}
