package sidecar

import (
	"bufio"
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

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/logging"
	"multiverse/internal/modtoken"
	"multiverse/internal/peercred"
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
	// contract-b-m4.md §3.1: --credential-file or MULTIVERSE_PEER_SECRET, and NO
	// FLAG THAT TAKES THE SECRET LITERALLY — it would put it in every process
	// listing. The peerId half is not a secret and comes from <data-dir>/peer-id.
	credentialFile := fs.String("credential-file", env("MULTIVERSE_CREDENTIAL_FILE", ""),
		"file whose first line is THE SECRET HALF of this peer's credential, from the join "+
			"string the relay operator handed over ("+peercred.SecretEnvVar+" is the "+
			"alternative). The peerId half comes from <data-dir>/peer-id")
	contractATokenFile := fs.String("contract-a-token-file", env(modtoken.FileEnvVar, ""),
		"where the Contract A bearer token lives (contract-a.md §21, A47). Defaults to "+
			"<data-dir>/"+modtoken.DefaultFileName+", minted 0600 at first start. THE MOD MUST "+
			"READ THE SAME PATH. It is NOT the relay credential")
	insecureContractA := fs.Bool("insecure-no-contract-a-token", envBool(modtoken.InsecureEnvVar),
		"accept a mod connection with no bearer token, and log one loud warning per accepted "+
			"connection. For a single-machine rehearsal and for nothing else (contract-a.md §21, A47)")
	// holdTimeoutMs is a contract-b-m4.md §12 tunable and had no knob. Its
	// default is 24 hours, which is a policy, not a measurement (§9.3) — and a
	// rig that wants to SEE the automatic bounce cannot wait a day for it.
	holdTimeout := fs.Duration("hold-timeout", envDuration("MULTIVERSE_HOLD_TIMEOUT", 0),
		"accrued dark time before a held entry bounces home by itself (contract-b-m4.md §9.3, "+
			"holdTimeoutMs). 0 keeps the 24-hour default. The clock runs only while the "+
			"destination is dark and this sidecar can see it")
	// inboundRatePerSimMinute was a compiled Go constant with no flag and no
	// environment variable, reachable only by editing source — and it has now
	// needed retuning three times (contract-a.md §18, A40). A tunable an operator
	// cannot retune from the metric that measures it is not a tunable.
	inboundRate := fs.Float64("inbound-rate", envFloat("MULTIVERSE_INBOUND_RATE", 0),
		"MIGRATE_IN deliveries released per SIMULATED minute of this world "+
			"(contract-a.md §7.5, inboundRatePerSimMinute). 0 keeps the default "+
			"(100.0). Raise it when metrics.jsonl shows a pacedDepth that never "+
			"falls; lower it to spread a dam harder")
	// The burst is the other half of the same knob, and without it --inbound-rate
	// cannot actually be exercised: a bucket of 50 swallows any test burst small
	// enough to force by hand, so a low rate with the shipped burst never dams
	// and the pacing never runs at all.
	inboundBurst := fs.Float64("inbound-burst", envFloat("MULTIVERSE_INBOUND_BURST", 0),
		"token-bucket capacity for --inbound-rate (contract-a.md §7.5, "+
			"inboundRateBurst), the largest clump ever released at once. 0 keeps "+
			"the default (50.0)")
	// heartbeatTimeoutMs was a compiled constant with no knob, and §20 A45 raised
	// it from 3500 to 13000 because a periodic world save blocks the thread that
	// sends the heartbeat. The number is sized from a save-stall tail that has
	// moved with every regime change this rig has made, so A40's rule applies:
	// a tunable an operator cannot retune from the metric that measures it is not
	// a tunable.
	heartbeatTimeout := fs.Duration("heartbeat-timeout", envDuration("MULTIVERSE_HEARTBEAT_TIMEOUT", 0),
		"HEARTBEAT silence before this sidecar closes Contract A with 4004 "+
			"(contract-a.md §8, heartbeatTimeoutMs). 0 keeps the 13-second default. "+
			"Raise it when [M4-SAVE] stalls approach it; lower it to detect a dead "+
			"mod sooner, at the cost of a 4004 for every save that overruns")
	listInflight := fs.Bool("list-inflight", false,
		"print the journal entries this sidecar still holds custody of, then exit "+
			"(contract-b-m4.md §7.5). Answers what the relay cannot.")
	destSlot := fs.Int("dest-slot", 0, "with --list-inflight: only entries addressed to this slot")
	releaseInflight := fs.String("release-inflight", "",
		"<migrationId>: release one held entry by hand, then exit. Needs bounce|drop as the "+
			"next argument (contract-b-m4.md §9.3)")
	yes := fs.Bool("yes", false, "skip the confirmation prompt for --release-inflight")
	// journalCompactMinutes is contract-b-m4.md §12's disk-budget tunable. Its
	// default is 15 minutes; 0 keeps it. See internal/journal's Compact.
	journalCompact := fs.Int("journal-compact-minutes", envInt("MULTIVERSE_JOURNAL_COMPACT_MINUTES", 0),
		"how often the journal is rewritten to its live entries (contract-b-m4.md §12, "+
			"journalCompactMinutes). 0 keeps the 15-minute default. Raise it on a rig with "+
			"disk to spare; lower it on one without")
	// maxGenomeRPM is §3.3's maxGenomeRequestsPerMinute on the ANSWERING side.
	// It shipped as the compiled constant contractb.GenomeRequestsPerMinute,
	// which is the worked example D20's knob rule was written about, and B24
	// moves it into the published table and makes it a knob on every party that
	// enforces it. The relay PUBLISHES the value; this side enforces it.
	maxGenomeRPM := fs.Int("max-genome-requests-per-minute",
		envInt("MULTIVERSE_MAX_GENOME_REQUESTS_PER_MINUTE", 0),
		"GENOME_REQUESTs this sidecar will answer per requester per minute (contract-b-m4.md "+
			"§3.3, §10). 0 keeps the contract default. Read the relay's published limits object "+
			"before moving it: a peer answering below the published ceiling looks broken")
	logLevel := fs.String("log-level", env("MULTIVERSE_LOG_LEVEL", "info"), "debug, info, warn or error")
	logFile, logRotateMB, logKeep := logging.Flags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	logger, logCloser, err := logging.New(stderr, logging.Options{
		Level: *logLevel, File: *logFile,
		RotateBytes: int64(*logRotateMB) << 20, Keep: *logKeep,
	})
	if err != nil {
		fmt.Fprintf(stderr, "sidecar: %v\n", err)
		return 1
	}
	defer logCloser.Close()

	if *listInflight {
		return listInflightCommand(*dataDir, *destSlot, stdout, stderr)
	}
	if *releaseInflight != "" {
		action := fs.Arg(0)
		return releaseInflightCommand(*dataDir, *releaseInflight, action, *yes, stdout, stderr)
	}

	// contract-b-m4.md §3.1: a missing credential is not fatal for a client — the
	// relay answers 401 and the backoff ladder pins itself — but it is worth one
	// loud line, because the alternative is a silent failure to join.
	secret, err := peercred.LoadSecret(*credentialFile)
	if err != nil && !errors.Is(err, peercred.ErrNoSecret) {
		logger.Error("sidecar: the credential file is unusable", "err", err,
			"file", *credentialFile)
		return 1
	}
	if secret == "" {
		logger.Warn("sidecar: no peer credential configured; the relay will answer 401 unless it " +
			"runs --insecure-no-token. Put the SECRET half of this peer's join string in a file " +
			"and pass --credential-file, or set " + peercred.SecretEnvVar)
	}

	cfg := DefaultConfig()
	cfg.Listen = *listen
	cfg.RelayURL = *relayURL
	cfg.PeerID = *peerID
	cfg.DataDir = *dataDir
	cfg.PreferredSlot = *slot
	cfg.InsertAfterSlot = *insertAfter
	cfg.InsertAxis = *insertAxis
	cfg.Secret = secret
	cfg.ContractATokenFile = *contractATokenFile
	cfg.InsecureNoContractAToken = *insecureContractA
	cfg.Logger = logger
	if *insecureContractA {
		logger.Warn("sidecar: --insecure-no-contract-a-token is set; ANY local process can drive " +
			"this world's migrations and impersonate this sidecar to the mod. It exists for a " +
			"single-machine rehearsal and no document this project ships may tell a player to " +
			"pass it (contract-a.md §21, A47)")
	}
	if *holdTimeout > 0 {
		cfg.HoldTimeout = *holdTimeout
		logger.Warn("sidecar: holdTimeoutMs overridden; a held entry bounces home sooner than the "+
			"contract default, and §9.3's accepted duplication case widens with it",
			"holdTimeout", *holdTimeout, "default", DefaultConfig().HoldTimeout)
	}
	if *heartbeatTimeout > 0 {
		cfg.HeartbeatTimeout = *heartbeatTimeout
		logger.Warn("sidecar: heartbeatTimeoutMs overridden; a save stall longer than this "+
			"still closes with 4004, and a dead mod is detected at this deadline instead "+
			"of the contract's",
			"heartbeatTimeout", *heartbeatTimeout, "default", DefaultConfig().HeartbeatTimeout)
	}
	if *journalCompact > 0 {
		cfg.JournalCompactInterval = time.Duration(*journalCompact) * time.Minute
	}
	if *maxGenomeRPM > 0 {
		cfg.GenomeRequestsPerMinute = *maxGenomeRPM
		logger.Info("sidecar: maxGenomeRequestsPerMinute overridden on the answering side",
			"maxGenomeRequestsPerMinute", *maxGenomeRPM,
			"default", DefaultConfig().GenomeRequestsPerMinute,
			"note", "the relay publishes the map's value on HANDSHAKE_ACK.limits; this is what "+
				"THIS peer will answer (contract-b-m4.md §3.3)")
	}
	if *inboundBurst > 0 {
		cfg.InboundRateBurst = *inboundBurst
	}
	if *inboundRate > 0 || *inboundBurst > 0 {
		if *inboundRate > 0 {
			cfg.InboundRatePerSimMinute = *inboundRate
		}
		cfg.Logger.Info("sidecar: the delivery rate limit is overridden",
			"inboundRate", cfg.InboundRatePerSimMinute,
			"defaultRate", DefaultConfig().InboundRatePerSimMinute,
			"burst", cfg.InboundRateBurst,
			"defaultBurst", DefaultConfig().InboundRateBurst)
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

// envFloat reads a float knob from the environment. A value that does not parse
// is IGNORED rather than fatal, for the same reason envDuration ignores one: a
// typo in a service-manager unit must not stop a world joining the map, and the
// startup log line names the value actually in force.
func envFloat(name string, fallback float64) float64 {
	if v := os.Getenv(name); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
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

// envBool reads an off switch from the environment. Only an explicit truthy
// value turns one on: a security control that a typo can disable is not a
// security control.
func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
