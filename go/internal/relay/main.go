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
)

// Main is the multiverse-relay entry point, factored out of package main so a
// test can run the same code path in a subprocess.
func Main(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("multiverse-relay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", env("MULTIVERSE_RELAY_LISTEN",
		fmt.Sprintf("127.0.0.1:%d", contractb.DefaultRelayPort)),
		"host:port for the Contract B WebSocket server")
	logLevel := fs.String("log-level", env("MULTIVERSE_LOG_LEVEL", "info"), "debug, info, warn or error")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	log := newLogger(stderr, *logLevel)
	srv := New(Options{Logger: log})

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Error("relay: listen failed", "addr", *listen, "err", err)
		return 1
	}
	log.Info("relay: listening", "addr", ln.Addr().String(), "path", contractb.ContractBPath)

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
