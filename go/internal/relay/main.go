package relay

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/lantoken"
)

// Main is the multiverse-relay entry point, factored out of package main so a
// test can run the same code path in a subprocess.
func Main(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("multiverse-relay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", env("MULTIVERSE_RELAY_LISTEN",
		fmt.Sprintf("0.0.0.0:%d", contractb.DefaultRelayPort)),
		"host:port for the Contract B WebSocket server; M3 binds a LAN-reachable address")
	dataDir := fs.String("data-dir", env("MULTIVERSE_RELAY_DATA_DIR", "multiverse-relay-data"),
		"directory for ring.json, the durable slot reservations")
	tokenFile := fs.String("token-file", env("MULTIVERSE_TOKEN_FILE", ""),
		"file whose first line is the shared LAN token; MULTIVERSE_TOKEN is the alternative")
	insecure := fs.Bool("insecure-no-token", false,
		"accept unauthenticated connections; for a single-machine test rig only, never on the LAN")
	releaseSlot := fs.Int("release-slot", 0,
		"release ring slot n at startup and exit; the operator escape hatch of contract-b-m3.md §7.5")
	var reserveSlots peerList
	fs.Var(&reserveSlots, "reserve-slot",
		"reserve the next ring slot for this peer id at startup and exit; repeat once per peer, in ring order")
	logLevel := fs.String("log-level", env("MULTIVERSE_LOG_LEVEL", "info"), "debug, info, warn or error")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	log := newLogger(stderr, *logLevel)

	// There is no flag that takes the token literally: it would put the secret
	// in every process listing (contract-b-m3.md §3.1).
	token, err := lantoken.Load(*tokenFile)
	if err != nil && !*insecure {
		log.Error("relay: no LAN token", "err", err,
			"hint", "set MULTIVERSE_TOKEN or pass --token-file; --insecure-no-token is test-rig only")
		return 1
	}

	srv, err := New(Options{
		Logger:          log,
		DataDir:         *dataDir,
		Token:           token,
		InsecureNoToken: *insecure,
	})
	if err != nil {
		log.Error("relay: startup failed", "err", err)
		return 1
	}

	if *releaseSlot > 0 {
		if err := srv.ReleaseSlot(*releaseSlot); err != nil {
			log.Error("relay: release failed", "slot", *releaseSlot, "err", err)
			return 1
		}
		log.Info("relay: slot released; the number is retired and will not be reused",
			"slot", *releaseSlot)
		return 0
	}

	if len(reserveSlots) > 0 {
		for _, id := range reserveSlots {
			res, created, err := srv.ReserveSlot(id)
			if err != nil {
				log.Error("relay: reservation failed", "peer", id, "err", err)
				return 1
			}
			if created {
				log.Info("relay: reserved a ring slot before the peer connected",
					"slot", res.Slot, "peer", res.PeerID)
			} else {
				log.Info("relay: this peer already holds a ring slot; left alone",
					"slot", res.Slot, "peer", res.PeerID)
			}
		}
		log.Info("relay: ring pre-seeded; start it again to serve", "ring", srv.RingSnapshot())
		return 0
	}

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Error("relay: listen failed", "addr", *listen, "err", err)
		return 1
	}
	log.Info("relay: listening", "addr", ln.Addr().String(), "path", contractb.ContractBPath,
		"dataDir", *dataDir, "ring", srv.RingSnapshot())

	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- httpSrv.Serve(ln) }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("relay: serve failed", "err", err)
			return 1
		}
	case <-ctx.Done():
		log.Info("relay: shutting down")
		srv.Drain()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}
	return 0
}

// peerList collects a repeated --reserve-slot flag, in the order it was given,
// which is the ring order the reservations are appended in.
type peerList []string

func (p *peerList) String() string     { return strings.Join(*p, ",") }
func (p *peerList) Set(v string) error { *p = append(*p, v); return nil }

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func newLogger(w io.Writer, level string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: l}))
}
