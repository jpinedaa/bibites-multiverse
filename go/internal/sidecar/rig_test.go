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

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/relay"
)

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

type testWriter struct{ t *testing.T }

func (w *testWriter) Write(p []byte) (int, error) {
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
// port, which is what the relay-drop test needs.
type testRelay struct {
	t    *testing.T
	addr string
	srv  *http.Server
	ln   *trackingListener
}

func startRelay(t *testing.T) *testRelay {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("relay listen: %v", err)
	}
	r := &testRelay{t: t, addr: ln.Addr().String()}
	r.serve(ln)
	t.Cleanup(r.kill)
	return r
}

func (r *testRelay) serve(ln net.Listener) {
	srv := relay.New(relay.Options{
		Logger:       testLogger(r.t),
		PingInterval: 200 * time.Millisecond,
		PeerTimeout:  3 * time.Second,
	})
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

// restart brings the relay back on the same address.
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
func fastConfig(t *testing.T, relayURL, peerID, sector string) Config {
	cfg := DefaultConfig()
	cfg.Listen = "127.0.0.1:0"
	cfg.RelayURL = relayURL
	cfg.PeerID = peerID
	cfg.PreferredSector = sector
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

// rig is the standard two-sector M2 rig: one relay, two sidecars, two fake mods.
// Sector A opens its east edge; sector B opens its west edge.
type rig struct {
	t     *testing.T
	relay *testRelay
	a, b  *Sidecar
	modA  *fakeMod
	modB  *fakeMod
	wA    *fakeWorld
	wB    *fakeWorld
}

type rigOptions struct {
	tuneA         func(*Config)
	tuneB         func(*Config)
	heartbeatA    time.Duration
	heartbeatB    time.Duration
	skipEdgeCheck bool
}

func newRig(t *testing.T, opts rigOptions) *rig {
	t.Helper()
	if opts.heartbeatA == 0 {
		opts.heartbeatA = 200 * time.Millisecond
	}
	if opts.heartbeatB == 0 {
		opts.heartbeatB = 200 * time.Millisecond
	}
	r := &rig{t: t, relay: startRelay(t), wA: newWorld(), wB: newWorld()}

	cfgA := fastConfig(t, r.relay.url(), "peer-a", contractb.SectorA)
	if opts.tuneA != nil {
		opts.tuneA(&cfgA)
	}
	cfgB := fastConfig(t, r.relay.url(), "peer-b", contractb.SectorB)
	if opts.tuneB != nil {
		opts.tuneB(&cfgB)
	}
	r.a = startSidecar(t, cfgA)
	r.b = startSidecar(t, cfgB)

	waitSector(t, r.a, contractb.SectorA)
	waitSector(t, r.b, contractb.SectorB)

	r.modA = dialFakeMod(t, fakeModOptions{
		url: r.a.URL(), world: r.wA, borderEdges: []string{contracta.EdgeE},
		sector: &contracta.Sector{X: 0, Y: 0}, heartbeat: opts.heartbeatA})
	r.modB = dialFakeMod(t, fakeModOptions{
		url: r.b.URL(), world: r.wB, borderEdges: []string{contracta.EdgeW},
		sector: &contracta.Sector{X: 1, Y: 0}, heartbeat: opts.heartbeatB})

	if !opts.skipEdgeCheck {
		r.modA.waitEdge(contracta.EdgeE, true, 5*time.Second)
		r.modB.waitEdge(contracta.EdgeW, true, 5*time.Second)
	}
	return r
}

func waitSector(t *testing.T, s *Sidecar, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := s.Sector(); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sidecar did not get sector %s (has %q)", want, s.Sector())
		}
		time.Sleep(10 * time.Millisecond)
	}
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

// custodyOf finds one migration's journal state, or nil.
func custodyOf(s *Sidecar, migrationID string) string {
	for _, st := range s.CustodySnapshot() {
		if st.Entry.MigrationID == migrationID {
			return fmt.Sprintf("%s/%s", st.Direction, st.Status)
		}
	}
	return "absent"
}
