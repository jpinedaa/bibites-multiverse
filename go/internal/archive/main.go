package archive

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/lantoken"
)

// Main is the multiverse-archive entry point. Two subcommands: the default
// runs the recorder, and `list` is the one read path M3 has.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "list" {
		return listMain(args[1:], stdout, stderr)
	}
	return runMain(args, stderr)
}

func runMain(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("multiverse-archive", flag.ContinueOnError)
	fs.SetOutput(stderr)
	relayURL := fs.String("relay", env("MULTIVERSE_RELAY",
		fmt.Sprintf("ws://127.0.0.1:%d%s", contractb.DefaultRelayPort, contractb.ContractBPath)),
		"Contract B relay URL")
	peerID := fs.String("peer-id", env("MULTIVERSE_ARCHIVE_PEER_ID", "archive-main"),
		"this subscriber's identity on the ring")
	dataDir := fs.String("data-dir", env("MULTIVERSE_ARCHIVE_DATA_DIR", "multiverse-archive-data"),
		"directory for migrations.jsonl and the content-addressed genome store")
	tokenFile := fs.String("token-file", env("MULTIVERSE_TOKEN_FILE", ""),
		"file whose first line is the shared LAN token; MULTIVERSE_TOKEN is the alternative")
	logLevel := fs.String("log-level", env("MULTIVERSE_LOG_LEVEL", "info"), "debug, info, warn or error")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	log := newLogger(stderr, *logLevel)
	token, err := lantoken.Load(*tokenFile)
	if err != nil && !errors.Is(err, lantoken.ErrNoToken) {
		log.Error("archive: token file is unusable", "err", err)
		return 1
	}
	if token == "" {
		log.Warn("archive: no LAN token configured; the relay will answer 401 unless it runs " +
			"--insecure-no-token")
	}

	a, err := New(Config{
		RelayURL: *relayURL,
		Token:    token,
		PeerID:   *peerID,
		DataDir:  *dataDir,
		Logger:   log,
	})
	if err != nil {
		log.Error("archive: startup failed", "err", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a.Start(ctx)
	<-ctx.Done()
	log.Info("archive: shutting down", "pendingGaps", a.PendingGaps())
	done := make(chan struct{})
	go func() { _ = a.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		log.Warn("archive: shutdown timed out")
	}
	return 0
}

// listMain is the read path: every recorded migration with its lineage, and
// whether the archive actually holds each genome. A hash with no genome is the
// gap report of contract-b-m3.md §10 — the archive's honest statement of what
// it does not have.
func listMain(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("multiverse-archive list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dataDir := fs.String("data-dir", env("MULTIVERSE_ARCHIVE_DATA_DIR", "multiverse-archive-data"),
		"directory holding migrations.jsonl")
	gapsOnly := fs.Bool("gaps", false, "list only migrations with at least one genome the archive lacks")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	migrations, err := List(*dataDir)
	if err != nil {
		fmt.Fprintf(stderr, "archive: %v\n", err)
		return 1
	}
	shown, gaps := 0, 0
	for _, m := range migrations {
		missing := !m.GenomeHeld
		for _, held := range m.ParentsHeld {
			if !held {
				missing = true
			}
		}
		if missing {
			gaps++
		}
		if *gapsOnly && !missing {
			continue
		}
		shown++
		when := time.UnixMilli(m.RecordedAt).UTC().Format(time.RFC3339)
		fmt.Fprintf(stdout, "%s  %s  entity %d  slot %d -> slot %d  (%s)  %s\n",
			when, m.MigrationID, m.EntityID, m.SourceSlot, m.DestSlot, m.SourcePeer, m.Outcome)
		if m.Lineage == nil {
			continue
		}
		fmt.Fprintf(stdout, "    genome %s %s\n", m.Lineage.GenomeHash, held(m.GenomeHeld))
		for i, p := range m.Lineage.Parents {
			if p.GenomeHash == "" {
				fmt.Fprintf(stdout, "    parent %d  gap: %s\n", p.EntityID, p.GapReason)
				continue
			}
			ok := i < len(m.ParentsHeld) && m.ParentsHeld[i]
			fmt.Fprintf(stdout, "    parent %d  %s %s\n", p.EntityID, p.GenomeHash, held(ok))
		}
	}
	fmt.Fprintf(stdout, "\n%d migration(s) shown, %d with a genome the archive does not hold\n",
		shown, gaps)
	return 0
}

func held(ok bool) string {
	if ok {
		return "[held]"
	}
	return "[MISSING]"
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
