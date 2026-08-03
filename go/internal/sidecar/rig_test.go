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

// testToken is the shared LAN token every rig uses. contract-b-m3.md §3.1 wants
// 16 to 256 bytes of printable ASCII; 32 random bytes hex-encoded is the
// RECOMMENDED shape, and a fixed one keeps the tests reproducible.
const testToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// TestMain doubles as the helper-process entry point for the crash-custody
// test: the test binary re-executes itself as a real sidecar process so a test
// can SIGKILL it mid-migration.
func TestMain(m *testing.M) {
	if os.Getenv("MULTIVERSE_TEST_HELPER") == "sidecar" {
		args := strings.Split(os.Getenv("MULTIVERSE_TEST_ARGS"), "\x1f")
		os.Exit(Main(args, os.Stderr))
	}
	os.Exit(m.Run())
}

func testLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(&testWriter{t: t}, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

type testWriter struct {
	mu sync.Mutex
	t  *testing.T
}

func (w *testWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
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
// port, which is what the relay-drop test needs. Its ring is durable in a temp
// directory, so a restart reclaims the same slots (contract-b-m3.md §7.4).
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
		Logger:       testLogger(r.t),
		DataDir:      r.dataDir,
		Token:        r.token,
		PingInterval: 200 * time.Millisecond,
		PeerTimeout:  3 * time.Second,
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
	r.srv = nil
	r.ln = nil
}

// restart brings the relay back on the same address and the same ring.json.
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
// finishes in seconds instead of minutes. Only the timers move; every rule the
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
	cfg.TickInterval = 40 * time.Millisecond
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

// node is one ring member: a sidecar, its fake mod and that mod's world.
type node struct {
	side  *Sidecar
	mod   *fakeMod
	world *fakeWorld
	cfg   Config
	slot  int
}

// ring is the standard M3 rig: one relay and n slots, every sim exporting east
// and receiving west (D8). With n = 3 the ring is the exit test's topology.
type ring struct {
	t     *testing.T
	relay *testRelay
	nodes []*node
}

type ringOptions struct {
	// tune adjusts the config of node i before it starts.
	tune func(i int, c *Config)
	// heartbeat is each mod's HEARTBEAT cadence; negative disables it.
	heartbeat time.Duration
	// noMods leaves the sidecars without mods.
	noMods bool
	// skipEdgeCheck skips waiting for every export edge to open.
	skipEdgeCheck bool
}

func newRing(t *testing.T, n int, opts ringOptions) *ring {
	t.Helper()
	if opts.heartbeat == 0 {
		opts.heartbeat = 200 * time.Millisecond
	}
	r := &ring{t: t, relay: startRelay(t)}
	for i := 0; i < n; i++ {
		r.addPeer(fmt.Sprintf("peer-slot%d", i+1), opts)
	}
	if !opts.noMods && !opts.skipEdgeCheck {
		for _, nd := range r.nodes {
			nd.mod.waitEdge(contracta.EdgeE, true, 10*time.Second)
		}
	}
	return r
}

// addPeer starts one more sidecar and waits for its ring slot, so slots are
// granted in a deterministic order. §7.2 rule 4 appends at the tail, so peer i
// takes slot i+1.
func (r *ring) addPeer(peerID string, opts ringOptions) *node {
	r.t.Helper()
	cfg := fastConfig(r.t, r.relay.url(), peerID)
	if opts.tune != nil {
		opts.tune(len(r.nodes), &cfg)
	}
	nd := &node{cfg: cfg, world: newWorld()}
	nd.side = startSidecar(r.t, cfg)
	nd.slot = waitSlotAny(r.t, nd.side)
	if !opts.noMods {
		slot := nd.slot
		nd.mod = dialFakeMod(r.t, fakeModOptions{
			url: nd.side.URL(), world: nd.world, ringSlot: &slot, heartbeat: opts.heartbeat})
	}
	r.nodes = append(r.nodes, nd)
	return nd
}

func (r *ring) node(i int) *node { return r.nodes[i] }

func waitSlotAny(t *testing.T, s *Sidecar) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if got := s.Slot(); got > 0 {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("sidecar %s never got a ring slot", s.PeerID())
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

func waitEastSlot(t *testing.T, s *Sidecar, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if east := s.EastNeighbour(); east != nil && east.Slot == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sidecar %s east neighbour is %v, want slot %d",
				s.PeerID(), eastSlotOf(s), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func eastSlotOf(s *Sidecar) int {
	if east := s.EastNeighbour(); east != nil {
		return east.Slot
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

// journalEntry returns one migration's journal state, for the annex assertions.
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
// re-ask.
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
	})
	if err != nil {
		t.Fatalf("archive: new: %v", err)
	}
	a.Start(context.Background())
	t.Cleanup(func() { _ = a.Close() })
	return a
}
