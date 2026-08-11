package archive

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/logging"
	"multiverse/internal/peercred"
	"multiverse/internal/wire"
)

// Main is the multiverse-archive entry point. Two subcommands: the default
// runs the recorder and the status page, and `list` is the ledger read path.
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
		"directory for migrations.jsonl, metrics.jsonl and the content-addressed genome store")
	// 8796, not 8791: contract-a.md §10 gives the six-slot rig 8787-8792, and
	// 8791 is slot 5's Contract A port (contract-b-m4.md §3, §12).
	httpListen := fs.String("http", env("MULTIVERSE_ARCHIVE_HTTP", "127.0.0.1:8796"),
		"bind address for the live status page and its JSON endpoint; empty disables it")
	metricsInterval := fs.Duration("metrics-interval", time.Minute,
		"how often a PEER_STATUS sample is appended to metrics.jsonl")
	credentialFile := fs.String("credential-file", env("MULTIVERSE_CREDENTIAL_FILE", ""),
		"file whose first line is THE SECRET HALF of this archive's credential, from the join "+
			"string the relay operator handed over ("+peercred.SecretEnvVar+" is the "+
			"alternative). Its credential must carry the SUBSCRIBE grant (contract-b-m4.md §5.1)")
	logLevel := fs.String("log-level", env("MULTIVERSE_LOG_LEVEL", "info"), "debug, info, warn or error")
	logFile, logRotateMB, logKeep := logging.Flags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	log, logCloser, err := logging.New(stderr, logging.Options{
		Level: *logLevel, File: *logFile,
		RotateBytes: int64(*logRotateMB) << 20, Keep: *logKeep,
	})
	if err != nil {
		fmt.Fprintf(stderr, "archive: %v\n", err)
		return 1
	}
	defer logCloser.Close()

	secret, err := peercred.LoadSecret(*credentialFile)
	if err != nil && !errors.Is(err, peercred.ErrNoSecret) {
		log.Error("archive: the credential file is unusable", "err", err, "file", *credentialFile)
		return 1
	}
	if secret == "" {
		log.Warn("archive: no credential configured; the relay will answer 401 unless it runs " +
			"--insecure-no-token. Put the SECRET half of this archive's join string in a file " +
			"and pass --credential-file, or set " + peercred.SecretEnvVar)
	}

	a, err := New(Config{
		RelayURL:        *relayURL,
		Secret:          secret,
		PeerID:          *peerID,
		DataDir:         *dataDir,
		Logger:          log,
		HTTPListen:      *httpListen,
		MetricsInterval: *metricsInterval,
	})
	if err != nil {
		log.Error("archive: startup failed", "err", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := a.Start(ctx); err != nil {
		log.Error("archive: start failed", "err", err)
		return 1
	}
	if addr := a.HTTPAddr(); addr != "" {
		log.Info("archive: status page", "url", "http://"+addr+"/", "json", "http://"+addr+"/api/status",
			"terminal", "ringstat --url http://"+addr)
	}
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
// gap report of contract-b-m4.md §10 — the archive's honest statement of what
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
	migrations, damage, err := List(*dataDir)
	if err != nil {
		fmt.Fprintf(stderr, "archive: %v\n", err)
		return 1
	}
	shown, gaps, unhashable := 0, 0, 0
	for _, m := range migrations {
		// contract-b-m4.md §10: the gap report names "a hash that no peer can
		// serve" — an UNRESOLVED hash. A lineage entry with no hash at all is a
		// different thing: `gapReason: "parent_gone"` means the genome never
		// existed to be recorded, and no retry can produce one. Counting those
		// here put every ordinary migration in the gap report — every hop after
		// the first carries a parent_gone entry (contract-a.md §5.3) — and
		// buried the entries an operator can actually act on.
		missing := false
		if m.Lineage != nil {
			if m.Lineage.GenomeHash == "" {
				unhashable++
			} else if !m.GenomeHeld {
				missing = true
			}
			for i, p := range m.Lineage.Parents {
				if p.GenomeHash != "" && (i >= len(m.ParentsHeld) || !m.ParentsHeld[i]) {
					missing = true
				}
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
		if m.Species != nil {
			// Recorded, never resolved (§15, B10). An absent block prints nothing
			// at all, because absent is absent and not "unknown".
			line := "    species " + wire.SpeciesName(m.Species)
			if m.Species.ParentGenericName != "" {
				line += "  (parent " + m.Species.ParentGenericName + " " +
					m.Species.ParentSpecificName + ")"
			}
			fmt.Fprintln(stdout, line)
		}
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
	fmt.Fprintf(stdout,
		"\n%d migration(s) shown, %d with an unresolved genome hash, %d whose migrant genome would not hash\n",
		shown, gaps, unhashable)
	// A listing that silently omits a crossing is the 2026-08-08 loss wearing a
	// friendlier face. Replay reads past a damaged line (store.go) and this is
	// where the read path admits to it, on stderr so a piped listing still says
	// it and grep over the listing cannot hide it.
	if damage.Any() {
		fmt.Fprintf(stderr,
			"archive: %d ledger line(s) (%d bytes) do not parse and were SKIPPED; "+
				"every record behind them is in this listing\n", damage.Lines, damage.Bytes)
	}
	if damage.TornTail > 0 {
		fmt.Fprintf(stderr,
			"archive: the ledger ends in an unfinished %d-byte record, which was never durable and is ignored\n",
			damage.TornTail)
	}
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
