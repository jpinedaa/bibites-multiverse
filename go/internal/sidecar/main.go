package sidecar

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
)

// Main is the multiverse-sidecar entry point, factored out of package main so
// the crash-custody test can run the same code path in a subprocess and SIGKILL
// it.
func Main(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("multiverse-sidecar", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", env("MULTIVERSE_LISTEN", fmt.Sprintf("127.0.0.1:%d", contracta.DefaultPort)),
		"Contract A listen address; loopback only")
	relayURL := fs.String("relay", env("MULTIVERSE_RELAY",
		fmt.Sprintf("ws://127.0.0.1:%d%s", contractb.DefaultRelayPort, contractb.ContractBPath)),
		"Contract B relay URL")
	peerID := fs.String("peer-id", env("MULTIVERSE_PEER_ID", ""),
		"stable peer identity; sector reclaim keys on it")
	dataDir := fs.String("data-dir", env("MULTIVERSE_DATA_DIR", "multiverse-data"),
		"directory for the migration journal")
	sector := fs.String("sector", env("MULTIVERSE_SECTOR", ""),
		"preferred sector A or B; advisory, the relay arbitrates")
	logLevel := fs.String("log-level", env("MULTIVERSE_LOG_LEVEL", "info"), "debug, info, warn or error")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg := DefaultConfig()
	cfg.Listen = *listen
	cfg.RelayURL = *relayURL
	cfg.PeerID = *peerID
	cfg.DataDir = *dataDir
	cfg.PreferredSector = *sector
	cfg.Logger = newLogger(stderr, *logLevel)

	s, err := New(cfg)
	if err != nil {
		cfg.Logger.Error("sidecar: startup failed", "err", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := s.Start(ctx); err != nil {
		cfg.Logger.Error("sidecar: start failed", "err", err)
		return 1
	}
	<-ctx.Done()
	cfg.Logger.Info("sidecar: shutting down")
	done := make(chan struct{})
	go func() { _ = s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cfg.Logger.Warn("sidecar: shutdown timed out")
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
