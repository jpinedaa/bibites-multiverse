package archive

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
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
	// §3.3's maxGenomeRequestsPerMinute on the REQUESTING side. B24 moves the
	// compiled constant into the published table and makes it a knob on every
	// party that enforces it; the relay publishes the value and the archive
	// paces itself to it (§10).
	maxGenomeRPM := fs.Int("max-genome-requests-per-minute",
		envInt("MULTIVERSE_MAX_GENOME_REQUESTS_PER_MINUTE", 0),
		"GENOME_REQUESTs this archive will send per answering peer per minute (contract-b-m4.md "+
			"§3.3, §10). 0 keeps the contract default. It is the limit a public archive is most "+
			"likely to need to move, and it used to need a rebuild")
	// DQ7's operator-side render deny list (§22, B30).
	denyList := fs.String("deny-list", env("MULTIVERSE_ARCHIVE_DENY_LIST", ""),
		"file of species names and peer:<peerId> entries this archive's PAGE AND JSON refuse to "+
			"render, one per line, # for a comment. It suppresses THE VIEW AND NOT THE RECORD: "+
			"the ledger goes on holding what happened, and nothing here evicts from it (D11, §10). "+
			"The file is re-read in place, so moderating costs an edit and never a restart")
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
		RelayURL:          *relayURL,
		Secret:            secret,
		PeerID:            *peerID,
		DataDir:           *dataDir,
		Logger:            log,
		HTTPListen:        *httpListen,
		MetricsInterval:   *metricsInterval,
		RequestsPerMinute: *maxGenomeRPM,
		DenyListFile:      *denyList,
	})
	if err != nil {
		log.Error("archive: startup failed", "err", err)
		return 1
	}
	if *denyList != "" {
		log.Warn("archive: an operator-side render deny list is loaded",
			"file", *denyList, "entries", a.deny.Len(),
			"scope", "the status page, its JSON and ringstat",
			"note", "this suppresses THE VIEW ONLY. The ledger and the genome store are never "+
				"evicted from, so removal from the record is NOT promised and must not be "+
				"promised to anybody (D11, contract-b-m4.md §10, §22 B30)")
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

// envInt is the environment half of a §3.3 knob. An unusable value falls back
// to the flag's own default rather than to zero, so a typo in a service unit
// cannot silently switch a limit off.
func envInt(name string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
