package sidecar

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/lantoken"
)

// Main is the multiverse-sidecar entry point, factored out of package main so
// the crash-custody test can run the same code path in a subprocess and SIGKILL
// it.
func Main(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("multiverse-sidecar", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", env("MULTIVERSE_LISTEN", fmt.Sprintf("127.0.0.1:%d", contracta.DefaultPort)),
		"Contract A listen address; loopback only")
	relayURL := fs.String("relay", env("MULTIVERSE_RELAY",
		fmt.Sprintf("ws://127.0.0.1:%d%s", contractb.DefaultRelayPort, contractb.ContractBPath)),
		"Contract B relay URL")
	peerID := fs.String("peer-id", env("MULTIVERSE_PEER_ID", ""),
		"stable peer identity; slot reclaim keys on it. Persisted in <data-dir>/peer-id")
	dataDir := fs.String("data-dir", env("MULTIVERSE_DATA_DIR", "multiverse-data"),
		"directory for the migration journal, the peer id, the slot, the position and the genome cache")
	slot := fs.Int("slot", envInt("MULTIVERSE_SLOT", 0),
		"preferred slot; advisory, the relay arbitrates. Overrides <data-dir>/slot")
	position := fs.String("position", env("MULTIVERSE_POSITION", ""),
		"preferred map position <col>,<row>; advisory. It may name a hole or one column/row "+
			"beyond the current rectangle. Overrides <data-dir>/position")
	insertAfter := fs.Int("insert-after-slot", 0,
		"advisory splice: place me immediately after this slot on --insert-axis")
	insertAxis := fs.String("insert-axis", "",
		"E or N; the axis --insert-after-slot splices on. Default E")
	tokenFile := fs.String("token-file", env("MULTIVERSE_TOKEN_FILE", ""),
		"file whose first line is the shared LAN token; MULTIVERSE_TOKEN is the alternative")
	// holdTimeoutMs is a contract-b-m4.md §12 tunable and had no knob. Its
	// default is 24 hours, which is a policy, not a measurement (§9.3) — and a
	// rig that wants to SEE the automatic bounce cannot wait a day for it.
	holdTimeout := fs.Duration("hold-timeout", envDuration("MULTIVERSE_HOLD_TIMEOUT", 0),
		"accrued dark time before a held entry bounces home by itself (contract-b-m4.md §9.3, "+
			"holdTimeoutMs). 0 keeps the 24-hour default. The clock runs only while the "+
			"destination is dark and this sidecar can see it")
	listInflight := fs.Bool("list-inflight", false,
		"print the journal entries this sidecar still holds custody of, then exit "+
			"(contract-b-m4.md §7.5). Answers what the relay cannot.")
	destSlot := fs.Int("dest-slot", 0, "with --list-inflight: only entries addressed to this slot")
	releaseInflight := fs.String("release-inflight", "",
		"<migrationId>: release one held entry by hand, then exit. Needs bounce|drop as the "+
			"next argument (contract-b-m4.md §9.3)")
	yes := fs.Bool("yes", false, "skip the confirmation prompt for --release-inflight")
	logLevel := fs.String("log-level", env("MULTIVERSE_LOG_LEVEL", "info"), "debug, info, warn or error")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	logger := newLogger(stderr, *logLevel)

	if *listInflight {
		return listInflightCommand(*dataDir, *destSlot, stdout, stderr)
	}
	if *releaseInflight != "" {
		action := fs.Arg(0)
		return releaseInflightCommand(*dataDir, *releaseInflight, action, *yes, stdout, stderr)
	}

	// contract-b-m4.md §3.1: a missing token is not fatal for a client — the
	// relay answers 401 and the backoff ladder pins itself — but it is worth
	// one loud line, because the alternative is a silent failure to join.
	token, err := lantoken.Load(*tokenFile)
	if err != nil && !errors.Is(err, lantoken.ErrNoToken) {
		logger.Error("sidecar: token file is unusable", "err", err)
		return 1
	}
	if token == "" {
		logger.Warn("sidecar: no LAN token configured; the relay will answer 401 unless it runs " +
			"--insecure-no-token. Set MULTIVERSE_TOKEN or pass --token-file")
	}

	cfg := DefaultConfig()
	cfg.Listen = *listen
	cfg.RelayURL = *relayURL
	cfg.PeerID = *peerID
	cfg.DataDir = *dataDir
	cfg.PreferredSlot = *slot
	cfg.InsertAfterSlot = *insertAfter
	cfg.InsertAxis = *insertAxis
	cfg.Token = token
	cfg.Logger = logger
	if *holdTimeout > 0 {
		cfg.HoldTimeout = *holdTimeout
		logger.Warn("sidecar: holdTimeoutMs overridden; a held entry bounces home sooner than the "+
			"contract default, and §9.3's accepted duplication case widens with it",
			"holdTimeout", *holdTimeout, "default", DefaultConfig().HoldTimeout)
	}
	if *position != "" {
		pos, err := parsePosition(*position)
		if err != nil {
			logger.Error("sidecar: bad --position", "value", *position, "err", err)
			return 1
		}
		cfg.PreferredPosition = pos
	}

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

// listInflightCommand answers §7.5's third question — WHICH entries name this
// slot, and what are they — on the machine that owns them.
func listInflightCommand(dataDir string, destSlot int, stdout, stderr io.Writer) int {
	entries, err := ListInflight(dataDir, destSlot, DefaultConfig().HoldTimeout)
	if err != nil {
		fmt.Fprintf(stderr, "sidecar: %v\n"+
			"(the sidecar for this data directory must be stopped: the journal is a single-writer file)\n", err)
		return 1
	}
	if destSlot > 0 {
		fmt.Fprintf(stdout, "in-flight entries addressed to slot %d, in %s:\n\n", destSlot, dataDir)
	} else {
		fmt.Fprintf(stdout, "in-flight entries in %s:\n\n", dataDir)
	}
	for _, e := range entries {
		fmt.Fprintf(stdout, "%s  entity %d  %s/%s\n", e.MigrationID, e.EntityID, e.Direction, e.Status)
		if e.Direction == "out" {
			fmt.Fprintf(stdout, "    destSlot %d via %s   handoff %s\n", e.DestSlot, e.ExitEdge, e.Handoff)
			fmt.Fprintf(stdout, "    accrued hold %s   deadline in %s   (the clock runs only while the\n",
				e.AccruedHold.Truncate(time.Second), e.Deadline.Truncate(time.Second))
			fmt.Fprintf(stdout, "    destination is dark AND this sidecar can see it)\n")
			if e.Reroutes > 0 {
				fmt.Fprintf(stdout, "    re-routed %d time(s) from slot %d under %s\n",
					e.Reroutes, e.RerouteFrom, e.RerouteProof)
			}
			if e.RelaySession != "" {
				fmt.Fprintf(stdout, "    relaySessionId %s\n", e.RelaySession)
			}
		}
		if e.Note != "" {
			fmt.Fprintf(stdout, "    %s\n", e.Note)
		}
	}
	fmt.Fprintf(stdout, "\n%d entr(y|ies). Release one with:\n"+
		"    multiverse-sidecar --data-dir %s --release-inflight <migrationId> bounce|drop\n",
		len(entries), dataDir)
	return 0
}

func releaseInflightCommand(dataDir, migrationID, action string, yes bool, stdout, stderr io.Writer) int {
	if action == "" {
		fmt.Fprintf(stderr, "sidecar: --release-inflight needs bounce or drop as the next argument\n")
		return 2
	}
	entries, err := ListInflight(dataDir, 0, DefaultConfig().HoldTimeout)
	if err != nil {
		fmt.Fprintf(stderr, "sidecar: %v\n", err)
		return 1
	}
	for _, e := range entries {
		if e.MigrationID != migrationID {
			continue
		}
		fmt.Fprintf(stdout, "\n%s  entity %d  destSlot %d via %s  handoff %s  accrued hold %s\n",
			e.MigrationID, e.EntityID, e.DestSlot, e.ExitEdge, e.Handoff,
			e.AccruedHold.Truncate(time.Second))
	}
	fmt.Fprint(stdout, InflightRisk)
	if !yes {
		fmt.Fprintf(stdout, "\nType YES to %s %s: ", action, migrationID)
		in := bufio.NewReader(os.Stdin)
		line, err := in.ReadString('\n')
		if err != nil || strings.TrimSpace(line) != "YES" {
			fmt.Fprintln(stdout, "aborted; nothing changed")
			return 1
		}
	}
	msg, err := ReleaseInflight(dataDir, migrationID, action)
	if err != nil {
		fmt.Fprintf(stderr, "sidecar: %v\n"+
			"(the sidecar for this data directory must be stopped: the journal is a single-writer file)\n", err)
		return 1
	}
	fmt.Fprintln(stdout, msg)
	return 0
}

func parsePosition(v string) (*contractb.Position, error) {
	colStr, rowStr, ok := strings.Cut(v, ",")
	if !ok {
		return nil, errors.New("a position is <col>,<row>")
	}
	col, err := strconv.Atoi(strings.TrimSpace(colStr))
	if err != nil || col < 0 {
		return nil, fmt.Errorf("col %q is not a non-negative integer", colStr)
	}
	row, err := strconv.Atoi(strings.TrimSpace(rowStr))
	if err != nil || row < 0 {
		return nil, fmt.Errorf("row %q is not a non-negative integer", rowStr)
	}
	return &contractb.Position{Col: col, Row: row}, nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func envDuration(name string, fallback time.Duration) time.Duration {
	if v := os.Getenv(name); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func envInt(name string, fallback int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
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
