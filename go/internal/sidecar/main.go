package sidecar

import (
	"bufio"
	"context"
	"encoding/json"
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
	// The two participant-chosen public strings of contract-b-m4.md §33, B49.
	// EMPTY IS THE DEFAULT AND NOTHING DERIVES ONE: no OS username, no save file,
	// no fallback to the peer id. What is not set here is not published, and the
	// join prompts are where a person is offered a value and answers.
	keeper := fs.String("keeper", env("MULTIVERSE_KEEPER", ""),
		"the handle this world is published under, shown beside it on the map "+
			"(contract-b-m4.md §33, B49). It is PUBLIC: every world on the map and every "+
			"authorised subscriber sees it. Up to 64 UTF-8 bytes, trimmed and clipped here. "+
			"Unset publishes nothing, and nothing invents one for you")
	worldName := fs.String("world-name", env("MULTIVERSE_WORLD_NAME", ""),
		"the display name for this world, shown on the map beside its slot "+
			"(contract-b-m4.md §33, B49). PUBLIC and bounded on the same terms as --keeper. "+
			"It is a label and never an address: routing uses the slot and identity uses "+
			"the peer id")
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
	// forwardTimeoutMs is a contract-b-m4.md §12 tunable. Its default is 5
	// minutes (§27 B42; 24 hours until 2026-08-19), which is a policy, not a
	// measurement (§9.3) — and a rig that wants to SEE a forward written off
	// still may not want to wait even that long.
	forwardTimeout := fs.Duration("forward-timeout", envDuration("MULTIVERSE_FORWARD_TIMEOUT", 0),
		"how long a forwarded organism waits for its answer before this sidecar records it "+
			"LOST (contract-b-m4.md §9.3, forwardTimeoutMs). 0 keeps the 5-minute default. "+
			"Nothing is re-sent at the deadline and nothing comes home: migration is "+
			"at-most-once")
	// maxReroutes is §9.2's bound, and a NEGATIVE value turns re-routing off.
	// The owner asked for the switch to be a knob rather than a release.
	maxReroutes := fs.Int("max-reroutes", envInt("MULTIVERSE_MAX_REROUTES", 0),
		"how many times an organism REFUSED at its destination may be offered to another slot "+
			"on the same axis (contract-b-m4.md §9.2, maxReroutes). 0 keeps the default of 4. "+
			"A NEGATIVE value turns re-routing off, so a refused organism bounces home instead "+
			"of trying a second slot. A re-route needs a proof that no custody moved, so it can "+
			"never duplicate an organism")
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
	inboundAdmission := fs.String("inbound-admission",
		env("MULTIVERSE_INBOUND_ADMISSION", AdmissionAdaptive),
		"pre-custody population admission: off, fixed, adaptive-shadow, or adaptive. "+
			"adaptive is the default: it fails open while learning, then enforces the learned "+
			"limit. adaptive-shadow publishes the same decision but refuses nothing")
	inboundPopulationLimit := fs.Int("inbound-population-limit",
		envInt("MULTIVERSE_INBOUND_POPULATION_LIMIT", 0),
		"hard living-population limit used by --inbound-admission=fixed; must be positive")
	inboundTargetScale := fs.Float64("inbound-target-time-scale",
		envFloat("MULTIVERSE_INBOUND_TARGET_TIME_SCALE", defaultAdmissionTarget),
		"reference simulation speed the adaptive population limit is sized for: the learned "+
			"machine budget (population x achieved speed) is divided by this value. The world "+
			"never has to request or reach it — the default x10 prices every world's limit on "+
			"one shared scale whatever speed its game runs at")
	inboundPopulationMin := fs.Int("inbound-population-min",
		envInt("MULTIVERSE_INBOUND_POPULATION_MIN", defaultAdmissionMin),
		"lowest population limit the adaptive estimator may learn")
	inboundPopulationMax := fs.Int("inbound-population-max",
		envInt("MULTIVERSE_INBOUND_POPULATION_MAX", defaultAdmissionMax),
		"highest population limit the adaptive estimator may learn")
	inboundPopulationHysteresis := fs.Int("inbound-population-hysteresis",
		envInt("MULTIVERSE_INBOUND_POPULATION_HYSTERESIS", defaultAdmissionHysteresis),
		"population fall below a closed limit required before inbound admission reopens")
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
	// WP7's support surface. Both are READ-ONLY and both exit without starting
	// anything: --diagnose runs the twenty-one checks of
	// docs/sidecar-diagnose-spec.md, and --my-slot prints what this world's own
	// sidecar and the map say about it. Either runs beside a sidecar that is
	// already up, and neither disturbs it.
	diagnose := fs.Bool("diagnose", false,
		"run the support checks against this data directory and exit. Read-only, prints no "+
			"secret, and works with no map to reach: exit 0 when nothing failed, 1 when "+
			"something did, 2 when the diagnostic itself could not run")
	mySlot := fs.Bool("my-slot", false,
		"print what your own slot's liveness, lanes, queue depths, speed and last save are, "+
			"then exit. It reads the running sidecar on this machine and asks the map for "+
			"nothing")
	asJSON := fs.Bool("json", false,
		"with --diagnose or --my-slot: emit the machine-readable form instead of the human one. "+
			"Its shape is stable across releases, so a report from an old build is still readable")
	only := fs.String("check", "",
		"with --diagnose: report only these checks, comma-separated. It filters the REPORT and "+
			"not the work — a check's precondition is still evaluated, or its answer would mean "+
			"nothing — and the exit code then reflects what was reported")
	timeout := fs.Duration("timeout", DefaultDiagnoseTimeout,
		"with --diagnose: the bound on EACH probe it makes — the local read, the relay connect "+
			"and the TLS handshake. A diagnostic that hangs is a diagnostic nobody runs twice")
	gameDir := fs.String("game-dir", env("MULTIVERSE_GAME_DIR", ""),
		"with --diagnose: the game folder, for the mod log and the game build. A packaged "+
			"install names it in install-record.json and needs no flag")
	matrixFile := fs.String("support-matrix", env("MULTIVERSE_SUPPORT_MATRIX", ""),
		"with --diagnose: the support-matrix.json to look this machine's game build up in. A "+
			"packaged install keeps its copy in the folder it was installed from")
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
	if stray := strayArgs(fs, *releaseInflight != ""); len(stray) > 0 {
		fmt.Fprintf(stderr, "sidecar: %s\n", strayArgsMessage(stray))
		return 2
	}
	// THE TWO READ-ONLY COMMANDS RUN BEFORE THE LOGGER IS OPENED, and that
	// ordering is a rule rather than a tidy-up. --log-file has an environment
	// default (MULTIVERSE_LOG_FILE), so a participant who runs --diagnose with
	// the environment their start script sets would otherwise have the
	// diagnostic OPEN AND POSSIBLY ROTATE the running sidecar's own log — a
	// write, on a file another process owns, from a command whose first promise
	// is that it changes nothing. Neither command logs; both write to stdout.
	if *diagnose {
		return diagnoseCommand(diagnoseArgs{
			dataDir:            *dataDir,
			relayURL:           *relayURL,
			contractATokenFile: *contractATokenFile,
			credentialFile:     *credentialFile,
			gameDir:            *gameDir,
			matrixFile:         *matrixFile,
			only:               *only,
			timeout:            *timeout,
			asJSON:             *asJSON,
		}, stdout, stderr)
	}
	if *mySlot {
		return mySlotCommand(*dataDir, *timeout, *asJSON, stdout, stderr)
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
	cfg.Keeper = *keeper
	cfg.WorldName = *worldName
	cfg.Secret = secret
	cfg.ContractATokenFile = *contractATokenFile
	cfg.InsecureNoContractAToken = *insecureContractA
	cfg.InboundAdmissionMode = strings.ToLower(strings.TrimSpace(*inboundAdmission))
	cfg.InboundPopulationLimit = *inboundPopulationLimit
	cfg.InboundTargetTimeScale = *inboundTargetScale
	cfg.InboundPopulationMin = *inboundPopulationMin
	cfg.InboundPopulationMax = *inboundPopulationMax
	cfg.InboundPopulationHysteresis = *inboundPopulationHysteresis
	cfg.Logger = logger
	logger.Info("sidecar: inbound population admission configured",
		"mode", cfg.InboundAdmissionMode, "fixedLimit", cfg.InboundPopulationLimit,
		"targetTimeScale", cfg.InboundTargetTimeScale,
		"adaptiveMin", cfg.InboundPopulationMin, "adaptiveMax", cfg.InboundPopulationMax,
		"hysteresis", cfg.InboundPopulationHysteresis,
		"note", "the target is a reference divisor, not a speed the world must reach; "+
			"adaptive fails open while learning and enforces once ready; "+
			"adaptive-shadow learns and reports but never refuses")
	if *insecureContractA {
		logger.Warn("sidecar: --insecure-no-contract-a-token is set; ANY local process can drive " +
			"this world's migrations and impersonate this sidecar to the mod. It exists for a " +
			"single-machine rehearsal and no document this project ships may tell a player to " +
			"pass it (contract-a.md §21, A47)")
	}
	if *forwardTimeout > 0 {
		cfg.ForwardTimeout = *forwardTimeout
		logger.Warn("sidecar: forwardTimeoutMs overridden; an unanswered forward is written off "+
			"sooner than the contract default. It costs no organism — nothing is re-sent or "+
			"returned either way — but a value below this map's slowest honest answer turns "+
			"late acknowledgements into lost records",
			"forwardTimeout", *forwardTimeout, "default", DefaultConfig().ForwardTimeout)
	}
	if *maxReroutes != 0 {
		cfg.MaxReroutes = *maxReroutes
		if *maxReroutes < 0 {
			logger.Warn("sidecar: re-routing is OFF; an organism refused at its destination " +
				"bounces home instead of being offered to another slot on the same axis")
		} else {
			logger.Warn("sidecar: maxReroutes overridden", "maxReroutes", *maxReroutes,
				"default", DefaultConfig().MaxReroutes)
		}
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

// diagnoseArgs is what the flag surface resolved for --diagnose.
type diagnoseArgs struct {
	dataDir            string
	relayURL           string
	contractATokenFile string
	credentialFile     string
	gameDir            string
	matrixFile         string
	only               string
	timeout            time.Duration
	asJSON             bool
}

// diagnoseCommand runs the checks and maps the report onto the exit codes.
//
// EXIT 2 IS FOR THE DIAGNOSTIC ITSELF, never for anything it found: a run that
// exits 2 has told the caller nothing, so the only things that produce it are a
// missing data directory argument and a --check naming something that is not a
// check. Everything a machine's state can do to this command comes back as 0 or
// 1 with a report attached.
func diagnoseCommand(a diagnoseArgs, stdout, stderr io.Writer) int {
	if strings.TrimSpace(a.dataDir) == "" {
		fmt.Fprintf(stderr, "sidecar: --diagnose needs --data-dir (or MULTIVERSE_DATA_DIR)\n")
		return ExitCannotRun
	}
	var checks []string
	for _, id := range strings.Split(a.only, ",") {
		if id = strings.TrimSpace(id); id != "" {
			checks = append(checks, id)
		}
	}
	if bad := UnknownCheckIDs(checks); len(bad) > 0 {
		fmt.Fprintf(stderr, "sidecar: --check names no such check: %s\n", strings.Join(bad, ", "))
		fmt.Fprintf(stderr, "the checks are: %s\n", strings.Join(CheckIDs, ", "))
		return ExitCannotRun
	}
	// Whether a credential is configured, and NEVER what it is. LoadSecret is
	// the same resolution the running sidecar does — the file, then the
	// environment variable — so the check reports on the credential this install
	// would actually present.
	secret, err := peercred.LoadSecret(a.credentialFile)
	configured := err == nil && secret != ""

	rep := Diagnose(DiagnoseOptions{
		DataDir:            a.dataDir,
		RelayURL:           a.relayURL,
		ContractATokenFile: a.contractATokenFile,
		CredentialFile:     a.credentialFile,
		SecretConfigured:   configured,
		GameDir:            a.gameDir,
		MatrixFile:         a.matrixFile,
		Only:               checks,
		Timeout:            a.timeout,
	})
	if a.asJSON {
		if err := WriteDiagnosisJSON(stdout, rep); err != nil {
			fmt.Fprintf(stderr, "sidecar: %v\n", err)
			return ExitCannotRun
		}
		return rep.Exit
	}
	RenderDiagnosis(stdout, rep)
	return rep.Exit
}

// mySlotCommand prints the participant's own-slot view (ownslot.go).
//
// It exits 0 whenever it printed a view and 1 when there was none to print, and
// it deliberately makes NO judgement: judging is --diagnose's job, and a view
// that graded itself would be a second, quieter diagnostic that nobody
// specified.
func mySlotCommand(dataDir string, timeout time.Duration, asJSON bool, stdout, stderr io.Writer) int {
	if strings.TrimSpace(dataDir) == "" {
		fmt.Fprintf(stderr, "sidecar: --my-slot needs --data-dir (or MULTIVERSE_DATA_DIR)\n")
		return ExitCannotRun
	}
	if timeout <= 0 {
		timeout = DefaultDiagnoseTimeout
	}
	res := fetchOwnSlot(dataDir, timeout)
	if !res.OK {
		fmt.Fprintf(stderr, "sidecar: there is no live view to read for %s.\n%s\n", dataDir, res.Why)
		fmt.Fprintf(stderr, "This view is your own sidecar's answer about your own world, so it "+
			"needs that sidecar to be running.\nRun `multiverse-sidecar --diagnose --data-dir %s` "+
			"for the checks that answer without it.\n", dataDir)
		return ExitFail
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res.View); err != nil {
			fmt.Fprintf(stderr, "sidecar: %v\n", err)
			return ExitCannotRun
		}
		return ExitOK
	}
	RenderOwnSlot(stdout, res.View)
	return ExitOK
}

// listInflightCommand answers §7.5's third question — WHICH entries name this
// slot, and what are they — on the machine that owns them.
func listInflightCommand(dataDir string, destSlot int, stdout, stderr io.Writer) int {
	entries, err := ListInflight(dataDir, destSlot, DefaultConfig().ForwardTimeout)
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
			if e.SentAt.IsZero() {
				fmt.Fprintf(stdout, "    never written to a live relay connection: no custody has moved\n")
			} else {
				fmt.Fprintf(stdout, "    send committed %s   recorded lost in %s   (it is NOT re-sent and\n",
					e.SentAt.UTC().Format(time.RFC3339), e.LostIn.Truncate(time.Second))
				fmt.Fprintf(stdout, "    does not come home: migration is at-most-once)\n")
			}
			if e.Reroutes > 0 {
				fmt.Fprintf(stdout, "    re-routed %d time(s) from slot %d under %s\n",
					e.Reroutes, e.RerouteFrom, e.RerouteProof)
			}
			if e.RelaySession != "" {
				fmt.Fprintf(stdout, "    relaySessionId %s\n", e.RelaySession)
			}
			fmt.Fprint(stdout, renderReceiptEvidence(e, "    "))
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

// renderReceiptEvidence is the sender's own journal answering "was this frame
// ever forwarded?" — the question §5.2's forwarding record could only answer
// while the relay that made it was still the same process (§6.12, §22 B26).
//
// BOTH ANSWERS ARE PRINTED, and the negative one is printed carefully. Holding a
// receipt means the relay wrote the bytes. Holding none means NOTHING AT ALL: it
// is indistinguishable from a receipt that was never sent, was dropped from a
// full outbound queue, or was lost with the session. An operator who read the
// absence as "so it was never forwarded" would be reading silence as proof,
// which is the one mistake this contract is built around (§9.2).
func renderReceiptEvidence(e InflightEntry, indent string) string {
	if e.ForwardReceipts == 0 {
		return indent + "no FORWARD_RECEIPT: this proves NOTHING either way — a receipt that was\n" +
			indent + "never sent, was dropped, or was lost with the relay's session looks exactly\n" +
			indent + "like a forward that never happened (§6.12)\n"
	}
	when := "an unrecorded time"
	if !e.ReceiptForwarded.IsZero() {
		when = e.ReceiptForwarded.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("%sFORWARDED: the relay acknowledged %d forward(s); the last wrote this frame\n"+
		"%sto slot %d at %s under relay session %s. Custody MAY have\n"+
		"%smoved, and a bounce from here is the duplication case below.\n",
		indent, e.ForwardReceipts, indent, e.ReceiptDestSlot, when, e.ReceiptSession, indent)
}

func releaseInflightCommand(dataDir, migrationID, action string, yes bool, stdout, stderr io.Writer) int {
	if action == "" {
		fmt.Fprintf(stderr, "sidecar: --release-inflight needs bounce or drop as the next argument\n")
		return 2
	}
	entries, err := ListInflight(dataDir, 0, DefaultConfig().ForwardTimeout)
	if err != nil {
		fmt.Fprintf(stderr, "sidecar: %v\n", err)
		return 1
	}
	for _, e := range entries {
		if e.MigrationID != migrationID {
			continue
		}
		fmt.Fprintf(stdout, "\n%s  entity %d  destSlot %d via %s  handoff %s  recorded lost in %s\n",
			e.MigrationID, e.EntityID, e.DestSlot, e.ExitEdge, e.Handoff,
			e.LostIn.Truncate(time.Second))
		// B26's whole operator payoff, printed where a person is about to decide
		// (§6.12, §7.5). The receipt is what tells a bounce that MIGHT duplicate
		// from one that IS known to have been forwarded.
		fmt.Fprint(stdout, renderReceiptEvidence(e, "  "))
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

// strayArgs is every positional argument this invocation cannot explain.
//
// WHY A SIDECAR THAT IS HANDED ONE REFUSES TO START. Go's flag package stops at
// the first non-flag word and hands the rest back, and nothing here used to
// collect them — so `--world-name Alice's world` written into a start script
// without quotes started a world called "Alice's" and dropped "world" on the
// floor, silently, with a healthy log and a wrong name published to every peer
// on the map for as long as it ran. The truncation is the loud half; the quiet
// half is worse, because a stray word can equally be a flag somebody misspelt or
// half of a path with a space in it, and the process it starts is then configured
// as nobody asked. A refusal costs a restart; a silent start costs whatever it
// published in the meantime, and there is nothing in a log to look for.
//
// releasing is --release-inflight, the ONE command here that takes a positional
// argument of its own: bounce|drop. Its own handler refuses a missing or unknown
// action with its own message, so exactly one word is left to it and anything
// past that is stray on the same terms as everywhere else.
func strayArgs(fs *flag.FlagSet, releasing bool) []string {
	rest := fs.Args()
	if releasing && len(rest) > 0 {
		rest = rest[1:]
	}
	return rest
}

// strayArgsMessage NAMES WHAT IT DID NOT UNDERSTAND, because the whole failure
// this guards is one nobody can see: a person reading it is looking at a start
// script, and the word this quotes back is the second half of the value that
// lost its quotes.
func strayArgsMessage(stray []string) string {
	quoted := make([]string, 0, len(stray))
	for _, arg := range stray {
		quoted = append(quoted, strconv.Quote(arg))
	}
	what := "an argument that is not a flag: " + quoted[0]
	if len(quoted) > 1 {
		what = "arguments that are not flags: " + strings.Join(quoted, ", ")
	}
	return "refusing to start with " + what + ".\n" +
		"Every setting this program takes is a --flag, so a bare word is either a flag that " +
		"was misspelt or\npart of a value that lost its quotes — a value with a space in it " +
		"(a world name, an apostrophe'd handle,\na path) must be quoted as one argument: " +
		`--world-name "The Tidepool".` + "\n" +
		"Nothing was started, so nothing has been published under a half of a name."
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
