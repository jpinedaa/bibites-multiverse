// Package archive implements multiverse-archive: the read-only subscriber of
// contract-b-m4.md §5.1, the genome recorder of §10, and M4's operator surface
// — the live status page of §10.1 and the durable metrics behind it.
//
// It connects to the relay with role "archive", records every migration
// envelope and its lineage annex durably, fetches by hash any genome it has
// never seen, and serves the whole map from the PEER_STATUS broadcasts it
// already receives. Everything it does is off the migration path: a subscriber
// that is absent, slow or dead changes nothing about a migration, a missing
// genome is a gap on a record rather than a reason to delay one, and NOTHING ON
// THE MIGRATION PATH EVER WAITS FOR A READER (Risk 4).
//
// UNDER contract-b/4 IT IS AN AUTHORISED SUBSCRIBER (§5.1, §22 B27). It is still
// a recorder with no write interface and no query engine, and what changed is
// the door: role "archive" was a SELF-DECLARATION under M4, so anyone holding
// the one shared token could open a socket, declare themselves a subscriber and
// receive a byte-identical copy of every envelope on the map. It now
// authenticates as a peerId like everybody else, with a credential carrying the
// SUBSCRIBE grant — the same mechanism as a peer's, a different permission — and
// the grant is issued deliberately by the relay's operator, because what it
// grants is a fairly complete profile of every participant's machine.
//
// THE BOUNDARY IS NOT A PRIVILEGED VIEW, and that is what makes it describable
// in one sentence: every field this program reads is a field the relay already
// broadcasts to every sidecar on the map. There is no subscriber-only field, no
// private channel and no back door. The peer's own rule follows from it —
// nothing on this wire is confidential, so a sidecar that must not publish a
// value must not put it on the stats block (§6.3.1).
package archive

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/bb8"
	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

// Version is reported to the relay in HANDSHAKE.sidecarVersion.
const Version = "m5.0"

// DefaultRetrySchedule is contract-b-m4.md §10's ladder: 1 minute, 5 minutes,
// 30 minutes, 6 hours, then daily.
var DefaultRetrySchedule = []time.Duration{
	time.Minute, 5 * time.Minute, 30 * time.Minute, 6 * time.Hour, 24 * time.Hour,
}

// Config is the archive's runtime configuration.
type Config struct {
	RelayURL string
	// Secret is the SECRET HALF of this subscriber's own credential
	// (contract-b-m4.md §3.1, §22 B22/B27). The archive is a client of the same
	// wire with a DIFFERENT GRANT, not a second auth system: it authenticates as
	// a peerId like everybody else, and role "archive" requires the SUBSCRIBE
	// grant. Under M4 role "archive" was a self-declaration and the shared token
	// was the only gate, so anyone who could open a socket could subscribe to
	// every envelope on the map.
	Secret  string
	PeerID  string
	DataDir string
	Logger  *slog.Logger

	RelayBackoffMin time.Duration
	RelayBackoffMax time.Duration
	// FirstAttemptDelay is how long a newly seen hash waits before its first
	// fetch. It exists so the ordinary case — ask the source peer at once — is
	// not slowed by the retry ladder.
	FirstAttemptDelay time.Duration
	RetrySchedule     []time.Duration
	RequestsPerMinute int
	TickInterval      time.Duration

	// The bounds of §21, B21, all of them on the pump and none of them on the
	// ladder. RequestsPerMinute above is a rate; these bound the WORK and the
	// BURST of one pass, which is what a 64,000-entry backlog turned into a
	// self-sustaining session flap on 2026-08-10. The fourth, the chunk size the
	// lock is released on, is contractb.GenomeScanChunk and is not tunable here.
	//
	// ScanPerTick is how many pending entries one pump examines. The walk is
	// round-robin over a stable order and resumes where the last one stopped, so
	// every entry is still visited and nothing is skipped — a full cycle simply
	// takes len(pending)/ScanPerTick ticks instead of one.
	ScanPerTick int
	// MaxRequestsPerTick is how many GENOME_REQUESTs one pump may issue.
	MaxRequestsPerTick int
	// MaxInFlightPerPeer is how many requests may be outstanding to one peer at
	// once, independent of the rate.
	MaxInFlightPerPeer int

	// HTTPListen is the status page's bind address, "" for no page. The rig
	// uses 127.0.0.1:8796; a test uses 127.0.0.1:0.
	HTTPListen string
	// StatsStale is §10.1's honesty threshold: a stats block older than this
	// renders as UNKNOWN rather than as state.
	StatsStale time.Duration
	// MetricsInterval is how often a PEER_STATUS sample is appended to
	// <data-dir>/metrics.jsonl, so history survives everything (WP3, WP5).
	MetricsInterval time.Duration
	// DenyListFile is DQ7's operator-side render deny list (§22, B30): a file of
	// species names and peer ids whose text this archive's surfaces refuse to
	// render. Empty for none, which is every archive that has never needed one.
	// It suppresses THE VIEW AND NEVER THE RECORD — see denylist.go.
	DenyListFile string

	// GenomeHorizon is Decision 3's retention horizon (§23, B33): how long a
	// genome BLOB is kept after it was last stored or last served. ZERO IS OFF
	// AND IS THE DEFAULT — nothing evicts, which is M4's behaviour exactly — and
	// the deployment sets 720h. It never touches the ledger: the record of what
	// crossed is kept forever, and the same horizon retires a gap whose crossing
	// has aged past it (§23, B34). See eviction.go.
	GenomeHorizon time.Duration
	// EvictionInterval is how often one bounded eviction pass runs. Defaults to
	// a minute; it exists so a test does not have to wait one.
	EvictionInterval time.Duration
}

func (c *Config) applyDefaults() {
	if c.PeerID == "" {
		c.PeerID = "archive-" + wire.NewUUID()[:8]
	}
	if c.DataDir == "" {
		c.DataDir = "multiverse-archive-data"
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.RelayBackoffMin <= 0 {
		c.RelayBackoffMin = contractb.RelayBackoffMin
	}
	if c.RelayBackoffMax <= 0 {
		c.RelayBackoffMax = contractb.RelayBackoffMax
	}
	if c.FirstAttemptDelay < 0 {
		c.FirstAttemptDelay = 0
	}
	if len(c.RetrySchedule) == 0 {
		c.RetrySchedule = DefaultRetrySchedule
	}
	if c.RequestsPerMinute <= 0 {
		c.RequestsPerMinute = contractb.GenomeRequestsPerMinute
	}
	if c.TickInterval <= 0 {
		c.TickInterval = time.Second
	}
	if c.ScanPerTick <= 0 {
		c.ScanPerTick = contractb.GenomeScanPerTick
	}
	if c.MaxRequestsPerTick <= 0 {
		c.MaxRequestsPerTick = contractb.GenomeRequestsPerTick
	}
	if c.MaxInFlightPerPeer <= 0 {
		c.MaxInFlightPerPeer = contractb.GenomeInFlightPerPeer
	}
	if c.StatsStale <= 0 {
		c.StatsStale = contractb.StatsStale
	}
	if c.MetricsInterval <= 0 {
		c.MetricsInterval = time.Minute
	}
}

// fetch is one outstanding genome hunt.
type fetch struct {
	hash        string
	sourcePeer  string
	migrationID string
	entityID    int32
	firstSeen   time.Time
	// crossedAt is the recordedAt of the migration that needs this genome, and
	// it is the ONLY clock the retention horizon may retire a gap on (§23, B34).
	// firstSeen above is this process's own clock — the replay sets it to the
	// moment of the restart — so a gap measured on it would grow younger every
	// time the archive is restarted and would never age out at all.
	crossedAt time.Time
	attempts  int
	nextAt    time.Time
	// asked records the peers that answered unknown_hash for this hash, so the
	// ring is walked one peer at a time rather than re-asking the same one.
	asked map[string]bool
	// inFlight is the requestId of an unanswered GENOME_REQUEST.
	inFlight string
	deadline time.Time
	// inFlightPeer and inFlightGen carry the per-peer concurrency accounting of
	// §21, B21. The generation is the relay session the request left on: a
	// session that ended can never answer, so its outstanding requests must not
	// go on occupying a live session's in-flight budget. Only the ACCOUNTING is
	// reset that way — inFlight, deadline, attempts and nextAt are untouched, so
	// the ladder retries at exactly the moment it always did.
	inFlightPeer string
	inFlightGen  int64
}

// Archive is the subscriber.
type Archive struct {
	cfg     Config
	log     *slog.Logger
	ledger  *Ledger
	genomes *bb8.Store

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	httpLn  net.Listener
	httpSrv *http.Server
	metrics *MetricsLog
	// deny is DQ7's render deny list (§22, B30). It sits on the SERVING side of
	// this struct and nowhere near the ledger: suppression is a fact about the
	// view, and the record goes on holding what happened.
	deny *DenyList

	mu        sync.Mutex
	conn      *wsutil.Conn
	ready     bool
	peerEpoch int64
	// status is the last PEER_STATUS, and it is the ONLY source the status page
	// has for the map (§10.1 rule 1).
	status   contractb.PeerStatus
	statusAt time.Time
	// lanes counts the envelopes copied on each ordered slot pair, which is what
	// turns the ledger into a per-lane flow rate.
	lanes map[lanePair]*lane
	// simRates is the sliding window of (statsAsOfMs, simulatedTime) pairs per
	// slot behind achievedTimeScale — how fast each world's clock ACTUALLY ran,
	// as against the timeScale it reports applying. See achieved.go.
	simRates map[int]*achievedRate
	// hops is the bounded recent-hops feed of §17 B14. It is DELIBERATELY not a
	// field of Status: see hops.go for why the durable metrics file must not
	// carry it.
	hops          []Hop
	hopsTruncated bool
	// species is the LEDGER AGGREGATE behind the species tab: crossings, first
	// and last sighting, distinct genomes and recent lanes, per species. It is
	// built once during the replay below and maintained by onMigration, because
	// the ledger is half a million lines and growing and a per-poll scan of it
	// would make the cost of watching grow with the age of the record. See
	// species.go.
	species     *speciesLedger
	recordCount int
	// ledgerSkipped is how many ledger lines the startup replay could not parse
	// and read past. It is 0 for every healthy archive, it never falls once a
	// damaged line exists — the ledger is never rewritten — and it is the
	// difference between recordCount and `wc -l` on the file.
	ledgerSkipped int
	seen          map[string]bool // "TYPE/" + dedupKey(...), the §5.1 duplicate rule
	pending       map[string]*fetch
	// pendingOrder is the stable round-robin order pumpFetches walks, and
	// pendingHead is how far this cycle has got. Go's map iteration is random, so
	// a bounded walk over the map alone could starve a hash indefinitely; a FIFO
	// that pops from the front and pushes the still-pending entry back gives
	// every gap its turn in a fixed number of ticks. Stale hashes — ones the map
	// no longer holds because the genome arrived — are dropped as they are popped
	// and the consumed prefix is reclaimed in one copy when it is half the slice.
	pendingOrder []string
	pendingHead  int
	sentWindow   map[string]*rateWindow
	// inFlight counts unanswered GENOME_REQUESTs per peer for the current relay
	// session (sessionGen). It is emptied when a session starts, because nothing
	// sent on a dead session will ever be answered.
	inFlight map[string]int
	// outstanding is every fetch with an unanswered request, so a timeout is
	// reaped on the tick it falls due rather than whenever the round-robin
	// cursor next passes that entry. It is bounded by MaxInFlightPerPeer per
	// peer plus whatever a just-ended session left behind, and those age out at
	// their own deadlines.
	outstanding map[string]*fetch
	sessionGen  int64
	closed      bool
	// evict is the retention horizon's cursor and its counters (§23, B33/B34).
	// It is the zero value, and every path that reads it returns early, on an
	// archive with no horizon — which is every archive by default. See
	// eviction.go.
	evict evictState

	// The history strip's cache. It is deliberately NOT under mu: building a
	// history reads a file, and nothing that reads a file may hold the lock the
	// migration path takes (Risk 4).
	historyMu  sync.Mutex
	historyKey string
	historyAt  time.Time
	historyVal History
	// The species detail's sparkline cache, kept apart from the strip's for the
	// same reason and bounded in entries as well as in age: a reader clicking
	// through species must not re-read the sample file per click.
	speciesHistory speciesHistoryCache
}

type rateWindow struct {
	start time.Time
	count int
}

// New opens the archive's store and returns it. Nothing is dialled yet.
func New(cfg Config) (*Archive, error) {
	cfg.applyDefaults()
	ledger, err := OpenLedger(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	genomes, err := bb8.OpenStore(cfg.DataDir + "/genomes")
	if err != nil {
		return nil, err
	}
	metrics, err := OpenMetrics(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	deny, err := NewDenyList(cfg.DenyListFile)
	if err != nil {
		return nil, err
	}
	a := &Archive{
		cfg:         cfg,
		log:         cfg.Logger.With("archive", cfg.PeerID),
		ledger:      ledger,
		genomes:     genomes,
		metrics:     metrics,
		deny:        deny,
		lanes:       map[lanePair]*lane{},
		simRates:    map[int]*achievedRate{},
		seen:        map[string]bool{},
		pending:     map[string]*fetch{},
		sentWindow:  map[string]*rateWindow{},
		inFlight:    map[string]int{},
		outstanding: map[string]*fetch{},
		species:     newSpeciesLedger(),
	}
	// Replay what is already recorded, so a restart neither duplicates a record
	// nor forgets a gap. "Keep the hash forever" (§10): a hash with no genome
	// is still a lineage node, and a fetch that failed for a year can succeed
	// tomorrow.
	//
	// THE REPLAY STREAMS. Each record is applied as it is parsed and then
	// dropped, so the cost of a restart is the aggregates below plus one record
	// — not the ledger. Reading the file into a []Record first and walking it
	// afterwards is what made the peak the size of the file (ScanLedger's
	// comment has the measurement); everything in this loop was already
	// fold-shaped, so the fix was to stop holding what it had finished with.
	//
	// now is the replay's clock, taken once. It was taken between the read and
	// the apply loop when those were two passes, which on a multi-million-record
	// ledger was already minutes into the restart; the gaps a replay discovers
	// are all due on the first tick either way.
	now := time.Now()
	damage, err := ScanLedger(cfg.DataDir, func(rec Record) {
		a.recordCount++
		if rec.MigrationID != "" {
			// Rebuild the key the live path uses, not a lookalike. A NACK
			// dedups on migrationId+code (§14, B7), so replaying it under
			// migrationId alone would never match and every re-copied NACK
			// would be recorded a second time after a restart.
			a.seen[rec.Type+"/"+dedupKey(rec.Type, rec.MigrationID, rec.Code)] = true
		}
		if rec.Type != RecordMigration {
			return
		}
		// The lane counters are rebuilt from the ledger, so a restart does not
		// reset the flow the operator was reading.
		a.observeLaneLocked(rec.SourceSlot, rec.DestSlot, rec.ExitEdge, rec.RecordedAt)
		// And so is the species aggregate, ON THIS ONE PASS. It is the whole
		// reason the species tab can answer "how often has this species ever
		// crossed" without reading the ledger again: the replay the archive
		// already performs at startup is the only full scan there is, and every
		// later answer is a map lookup (species.go, rule 1).
		a.observeSpeciesLocked(rec)
		if rec.Lineage == nil {
			return
		}
		for _, h := range hashesOf(rec) {
			// The CROSSING's own time, not the replay's: a horizon that measured
			// from the restart would re-queue a backlog it retired yesterday, and
			// pay for it in resident memory as well as in work (§23, B34).
			a.trackLocked(h, rec.SourcePeer, rec.MigrationID, rec.EntityID,
				time.UnixMilli(rec.RecordedAt), now)
		}
	})
	if err != nil {
		return nil, err
	}
	a.ledgerSkipped = damage.Lines
	if n := ledger.Repaired(); n > 0 {
		// A write the previous process never finished. Nothing durable was lost
		// — the record was never ACKed — but an operator reading `wc -l` against
		// the record count deserves to know the file changed at startup.
		a.log.Warn("archive: dropped an unfinished record from the end of the ledger",
			"bytes", n, "ledger", ledger.Path())
	}
	if damage.Any() {
		// The ledger is damaged and STAYS damaged: it is append-only, so unlike
		// the sidecar journal there is no compaction that quietly heals it. Every
		// restart reads past the same line and every restart must say so. It is
		// an error rather than a warning because a whole record — a crossing that
		// happened — is missing from the archive's account of what happened.
		a.log.Error("archive: the ledger holds line(s) that do not parse; replay SKIPPED them "+
			"and kept every record behind them",
			"skippedLines", damage.Lines, "skippedBytes", damage.Bytes,
			"records", a.recordCount, "ledger", ledger.Path())
	}
	if damage.TornTail > 0 {
		a.log.Warn("archive: ignored an unfinished record at the end of the ledger",
			"bytes", damage.TornTail, "ledger", ledger.Path())
	}
	if n := len(a.pending); n > 0 {
		a.log.Warn("archive: resumed with genomes still missing", "gaps", n,
			"records", a.recordCount)
	}
	a.logRetiredAtStartup()
	return a, nil
}

func hashesOf(rec Record) []string {
	if rec.Lineage == nil {
		return nil
	}
	out := make([]string, 0, 1+len(rec.Lineage.Parents))
	if rec.Lineage.GenomeHash != "" {
		out = append(out, rec.Lineage.GenomeHash)
	}
	for _, p := range rec.Lineage.Parents {
		if p.GenomeHash != "" {
			out = append(out, p.GenomeHash)
		}
	}
	return out
}

// Start dials the relay, starts the fetch scheduler, and — when HTTPListen is
// set — serves the status page.
func (a *Archive) Start(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)
	if a.cfg.HTTPListen != "" {
		ln, err := net.Listen("tcp", a.cfg.HTTPListen)
		if err != nil {
			return err
		}
		a.httpLn = ln
		a.httpSrv = &http.Server{Handler: a.httpHandler(), ReadHeaderTimeout: 10 * time.Second}
		a.wg.Add(1)
		go func() { defer a.wg.Done(); a.serveHTTP() }()
	}
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.relayLoop() }()
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.tickLoop() }()
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.metricsLoop() }()
	// Nothing at all unless a horizon was configured (§23, B33). See eviction.go.
	a.startEviction()
	a.log.Info("archive: started", "relay", a.cfg.RelayURL, "dataDir", a.cfg.DataDir,
		"ledger", a.ledger.Path(), "metrics", a.metrics.Path(), "statusPage", a.HTTPAddr())
	return nil
}

// metricsLoop appends a PEER_STATUS sample to the durable metrics file, so the
// history an operator reads survives a restart of everything — including the
// browser tab (WP3, WP5).
func (a *Archive) metricsLoop() {
	t := time.NewTicker(a.cfg.MetricsInterval)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			// One last sample on the way out, so the file ends with the state the
			// map was in when the archive stopped.
			a.sampleMetrics()
			return
		case <-t.C:
			a.sampleMetrics()
		}
	}
}

func (a *Archive) sampleMetrics() {
	view := a.StatusView()
	if !view.HaveStatus {
		return
	}
	if err := a.metrics.Append(view); err != nil {
		a.log.Warn("archive: metrics append failed", "err", err)
	}
}

// Close stops the archive and flushes its ledger.
func (a *Archive) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	conn := a.conn
	a.mu.Unlock()
	if conn != nil {
		conn.Close(contractb.CloseNormal, "archive closing")
	}
	if a.cancel != nil {
		a.cancel()
	}
	if a.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = a.httpSrv.Shutdown(ctx)
		cancel()
	}
	a.wg.Wait()
	if err := a.metrics.Close(); err != nil {
		a.log.Warn("archive: metrics close failed", "err", err)
	}
	return a.ledger.Close()
}

// MetricsPath is the durable sample file, for tests and operator tools.
func (a *Archive) MetricsPath() string { return a.metrics.Path() }

// Genomes is the content-addressed store, for tests and operator tools.
func (a *Archive) Genomes() *bb8.Store { return a.genomes }

// Records replays the ledger. What the replay skipped is reported where it can
// be acted on — the startup log and Status.LedgerSkipped — rather than to every
// test and tool that only wants the records.
func (a *Archive) Records() ([]Record, error) {
	recs, _, err := ReadLedger(a.cfg.DataDir)
	return recs, err
}

// List is the read path: every recorded migration joined with the genome store.
func (a *Archive) List() ([]Migration, LedgerDamage, error) { return List(a.cfg.DataDir) }

// PendingGaps is the number of hashes no peer has served yet.
func (a *Archive) PendingGaps() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pending)
}

// ---------------------------------------------------------------- relay link

func (a *Archive) relayLoop() {
	attempt := 0
	authFailures := 0
	for {
		select {
		case <-a.ctx.Done():
			return
		default:
		}
		started := time.Now()
		err := a.session()
		if err != nil && !errors.Is(err, context.Canceled) {
			a.log.Warn("archive: relay session ended", "err", err)
		}
		select {
		case <-a.ctx.Done():
			return
		default:
		}
		if err != nil && strings.Contains(err.Error(), "401") {
			authFailures++
			a.log.Error("archive: the relay refused THIS SUBSCRIBER'S CREDENTIAL with HTTP 401",
				"consecutiveFailures", authFailures, "peer", a.cfg.PeerID,
				"remedy", "re-apply this archive's join string (--credential-file or "+
					peercred.SecretEnvVar+"); the relay operator issues the SUBSCRIBE grant "+
					"deliberately, at its own console, and no wire message asks for one")
		} else {
			authFailures = 0
		}
		if time.Since(started) >= contractb.StableSession {
			attempt = 0
		}
		attempt++
		wait := jitter(a.cfg.RelayBackoffMin, a.cfg.RelayBackoffMax, attempt)
		if authFailures >= contractb.AuthFailuresBeforeCeiling {
			wait = a.cfg.RelayBackoffMax
		}
		time.Sleep(wait)
	}
}

func jitter(min, max time.Duration, attempt int) time.Duration {
	ceiling := min
	for i := 1; i < attempt && ceiling < max; i++ {
		ceiling *= 2
	}
	if ceiling > max {
		ceiling = max
	}
	if ceiling <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(ceiling)))
}

func (a *Archive) session() error {
	dialCtx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	opts := &websocket.DialOptions{CompressionMode: websocket.CompressionDisabled}
	if a.cfg.Secret != "" {
		// §3.1: on the HTTP upgrade, bound to this subscriber's own peerId, and
		// never in a frame. No TLS knob is set and none may be — B23 gives a client
		// no way to skip verification, and that includes this one.
		opts.HTTPHeader = http.Header{
			"Authorization": []string{peercred.Header(a.cfg.PeerID, a.cfg.Secret)},
		}
	}
	ws, _, err := websocket.Dial(dialCtx, a.cfg.RelayURL, opts)
	cancel()
	if err != nil {
		return err
	}
	conn := wsutil.New(ws, 256)
	defer func() {
		conn.Close(contractb.CloseNormal, "archive closing the relay link")
		<-conn.Done()
		a.mu.Lock()
		a.conn, a.ready = nil, false
		a.mu.Unlock()
	}()

	a.mu.Lock()
	a.conn = conn
	a.ready = true
	a.peerEpoch = 0
	// §21, B21: a new session inherits no in-flight requests. Anything sent on
	// the session that just ended can never be answered, so holding its
	// concurrency slots would throttle this one for a whole request timeout.
	// Only the accounting resets — every fetch's own inFlight, deadline and
	// nextAt are left exactly as they were, so no gap's retry moves.
	a.sessionGen++
	a.inFlight = map[string]int{}
	// §6.1: gameVersion is always empty for an archive, and it never holds a
	// slot, so it never sends SECTOR_CLAIM either.
	ok := a.sendLocked(contractb.TypeHandshake, contractb.Handshake{
		PeerID:          a.cfg.PeerID,
		Role:            contractb.RoleArchive,
		ProtocolVersion: wire.ProtocolB,
		SidecarVersion:  Version,
	})
	a.mu.Unlock()
	if !ok {
		return errors.New("archive: HANDSHAKE send failed")
	}
	a.log.Info("archive: subscribed to the relay", "url", a.cfg.RelayURL)

	for {
		readCtx, readCancel := context.WithCancel(a.ctx)
		go func() {
			select {
			case <-conn.Done():
				readCancel()
			case <-readCtx.Done():
			}
		}()
		frame, err := conn.Read(readCtx)
		readCancel()
		if err != nil {
			return err
		}
		if !a.handle(conn, frame) {
			return errors.New("archive: closing the relay link")
		}
	}
}

func (a *Archive) handle(conn *wsutil.Conn, frame []byte) bool {
	env, err := wire.Decode(frame)
	if err != nil {
		conn.Close(contractb.CloseMalformedFrame, "malformed frame")
		return false
	}
	if err := wire.CheckProtocol(env.Protocol, wire.ProtocolB); err != nil {
		conn.Close(contractb.CloseProtocolUnsupported, "unsupported protocol major version")
		return false
	}
	switch env.Type {
	case contractb.TypeHandshakeAck:
		return true
	case contractb.TypePeerStatus:
		var status contractb.PeerStatus
		if contractb.DecodeData(env.Data, &status) == nil {
			a.mu.Lock()
			if status.Epoch > a.peerEpoch {
				a.peerEpoch = status.Epoch
				a.status = status
				a.statusAt = time.Now()
				a.observeSimRatesLocked(status)
			}
			a.mu.Unlock()
		}
		return true
	case contractb.TypeMigrationPayload:
		return a.onMigration(env)
	case contractb.TypeMigrationAck:
		return a.onAck(env)
	case contractb.TypeMigrationNack:
		return a.onNack(env)
	case contractb.TypeGenomeResponse:
		return a.onGenomeResponse(env)
	case contractb.TypeSectorGrant:
		// §5.1: a claim from a subscriber is refused with role_has_no_slot. The
		// archive never claims, so this only arrives if a relay volunteers it.
		return true
	case contractb.TypePing:
		var ping contractb.Ping
		if contractb.DecodeData(env.Data, &ping) == nil {
			a.mu.Lock()
			a.sendLocked(contractb.TypePong, contractb.Pong{Nonce: ping.Nonce})
			a.mu.Unlock()
		}
		return true
	case contractb.TypePong:
		return true
	default:
		a.log.Warn("archive: ignoring unknown type", "type", env.Type)
		return true
	}
}

// onMigration records one copied envelope. The record is written when the
// envelope arrives; a missing genome is a gap on that record and never a reason
// to delay or refuse one (§10).
func (a *Archive) onMigration(env wire.Envelope) bool {
	var p contractb.MigrationPayload
	if err := contractb.DecodeData(env.Data, &p); err != nil {
		a.log.Warn("archive: undecodable MIGRATION_PAYLOAD copy", "err", err)
		return true
	}
	if a.markSeen(RecordMigration, p.MigrationID) {
		// A re-forwarded migration produces a second copy; the archive
		// deduplicates on migrationId exactly as a sidecar does (§5.1).
		return true
	}
	lineage := p.Lineage
	// §15 B9's opacity rule reaches the archive too: SCHEMA VALIDATION ONLY. A
	// block that breaks the shape is stripped from the record with one log line —
	// recording a malformed one would put a fact in the ledger that no conformant
	// sidecar could have sent, and refusing to record the migration over a label
	// would be worse still.
	species, stripped := wire.CarrySpecies(p.Species)
	if stripped != "" {
		a.log.Warn("archive: stripping a malformed species block from a copied envelope; "+
			"the migration is still recorded",
			"migrationId", p.MigrationID, "sourcePeer", p.SourcePeer, "reason", stripped)
	}
	rec := Record{
		Type:        RecordMigration,
		RecordedAt:  time.Now().UnixMilli(),
		MigrationID: p.MigrationID,
		SourcePeer:  p.SourcePeer,
		SourceSlot:  p.SourceSlot,
		DestSlot:    p.DestSlot,
		EntityID:    p.EntityID,
		Kind:        p.Kind,
		GameVersion: p.Body.Version,
		Lineage:     &lineage,
		Species:     species,
		ExitEdge:    p.ExitEdge,
		Timestamp:   p.Timestamp,
	}
	if err := a.ledger.Append(rec); err != nil {
		a.log.Error("archive: ledger append failed", "migrationId", p.MigrationID, "err", err)
		return true
	}
	reroute := ""
	if p.Reroute != nil {
		// §6.6: a re-routed copy carries a different destSlot with the SAME
		// migrationId. It is not a duplicate organism, it is the same organism on
		// a new lane, and the block says why it took it.
		reroute = p.Reroute.Proof
	}
	a.log.Info("archive: recorded a migration", "migrationId", p.MigrationID,
		"sourcePeer", p.SourcePeer, "sourceSlot", p.SourceSlot, "destSlot", p.DestSlot,
		"exitEdge", p.ExitEdge, "reroute", reroute, "species", wire.SpeciesName(species),
		"genomeHash", lineage.GenomeHash, "parents", len(lineage.Parents))

	now := time.Now()
	a.mu.Lock()
	a.recordCount++
	a.observeLaneLocked(p.SourceSlot, p.DestSlot, p.ExitEdge, rec.RecordedAt)
	// The same envelope, kept a second way for a second question. The lane
	// counter answers HOW FAST; the feed answers WHO, just now (§17, B14). Both
	// read the copy the archive already has, and neither asks anybody for
	// anything.
	a.observeHopLocked(Hop{
		MigrationID: p.MigrationID,
		AtMs:        rec.RecordedAt,
		FromSlot:    p.SourceSlot,
		ToSlot:      p.DestSlot,
		ExitEdge:    p.ExitEdge,
		// The STRIPPED block, not the raw one: a malformed species never reaches
		// the page, exactly as it never reaches the ledger.
		Species: species,
	})
	// The same envelope, kept a THIRD way for a third question, and it costs one
	// map lookup. The lane counter answers HOW FAST, the feed answers WHO JUST
	// NOW, and the aggregate answers HOW OFTEN, EVER — the one question that
	// would otherwise need the whole ledger read back per poll. It folds the
	// record the archive has just written, so the live path and the startup
	// replay pass identical input to identical code.
	a.observeSpeciesLocked(rec)
	for _, h := range hashesOf(rec) {
		a.trackLocked(h, p.SourcePeer, p.MigrationID, p.EntityID,
			time.UnixMilli(rec.RecordedAt), now)
	}
	a.mu.Unlock()
	return true
}

// observeLaneLocked counts one envelope on the DIRECTED lane it crossed: the
// ordered slot pair AND the edge it left by. The edge is part of the key
// because two-way lanes made the pair ambiguous on an axis of length 2 (§17,
// B13) — slot 1's north lane and slot 1's south lane both end at slot 4 on the
// 3x2 rig, and they are two lanes carrying two flows.
//
// An empty edge is a record written before D17. It keeps its own bucket and is
// re-attributed at display time, where the map is known; see StatusView.
func (a *Archive) observeLaneLocked(from, to int, edge string, atMs int64) {
	if from == 0 && to == 0 {
		return
	}
	key := lanePair{from: from, to: to, edge: edge}
	l, ok := a.lanes[key]
	if !ok {
		l = &lane{}
		a.lanes[key] = l
	}
	l.observe(atMs)
}

func (a *Archive) onAck(env wire.Envelope) bool {
	var ack contractb.MigrationAck
	if contractb.DecodeData(env.Data, &ack) != nil {
		return true
	}
	if a.markSeen(RecordAck, ack.MigrationID) {
		return true
	}
	a.mu.Lock()
	a.recordCount++
	a.mu.Unlock()
	_ = a.ledger.Append(Record{
		Type:        RecordAck,
		RecordedAt:  time.Now().UnixMilli(),
		MigrationID: ack.MigrationID,
		SourcePeer:  ack.SourcePeer,
		DestPeer:    ack.DestPeer,
		EntityID:    ack.EntityID,
		Duplicate:   ack.Duplicate,
	})
	return true
}

func (a *Archive) onNack(env wire.Envelope) bool {
	var nack contractb.MigrationNack
	if contractb.DecodeData(env.Data, &nack) != nil {
		return true
	}
	if a.markSeen(RecordNack, dedupKey(RecordNack, nack.MigrationID, nack.Code)) {
		return true
	}
	a.mu.Lock()
	a.recordCount++
	a.mu.Unlock()
	_ = a.ledger.Append(Record{
		Type:        RecordNack,
		RecordedAt:  time.Now().UnixMilli(),
		MigrationID: nack.MigrationID,
		SourcePeer:  nack.SourcePeer,
		DestPeer:    nack.DestPeer,
		Code:        nack.Code,
		Message:     nack.Message,
	})
	return true
}

// dedupKey builds the §5.1 duplicate key for one record type. It is the single
// definition both the live path and the restart replay use, so the two cannot
// drift apart. A MIGRATION_NACK dedups on the pair migrationId + code, because
// one migration legitimately produces several different refusals on its way to
// a lane and each one is a separate fact (§14, B7). Everything else dedups on
// migrationId alone.
func dedupKey(typ, migrationID, code string) string {
	if typ == RecordNack {
		return migrationID + "/" + code
	}
	return migrationID
}

func (a *Archive) markSeen(typ, key string) bool {
	if key == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	k := typ + "/" + key
	if a.seen[k] {
		return true
	}
	a.seen[k] = true
	return false
}

// ---------------------------------------------------------------- genome fetch

// trackLocked notes a hash the archive may not hold. A hash it already has is
// not tracked; a hash it does not is a gap until some peer serves it.
//
// crossedAt is the recordedAt of the migration that wants the genome, and it is
// what the retention horizon measures (§23, B34). A crossing already past the
// horizon is NOT QUEUED AT ALL: the only blob a fetch could win is one the next
// eviction pass would delete, so the work is spent by construction. That check
// is what keeps a restart from re-queueing a backlog this archive has already
// drained — the replay hands every hash in the ledger to this function.
func (a *Archive) trackLocked(hash, sourcePeer, migrationID string, entityID int32, crossedAt, now time.Time) {
	if hash == "" || a.genomes.Has(hash) {
		return
	}
	if f, ok := a.pending[hash]; ok {
		if f.sourcePeer == "" {
			f.sourcePeer = sourcePeer
		}
		return
	}
	// Checked AFTER the pending lookup on purpose: a hash an old crossing shares
	// with a recent one is still wanted, and the recent crossing's entry keeps it.
	if a.gapPastHorizonLocked(crossedAt, now) {
		a.evict.gapsExpired++
		return
	}
	a.pending[hash] = &fetch{
		hash:        hash,
		sourcePeer:  sourcePeer,
		migrationID: migrationID,
		entityID:    entityID,
		firstSeen:   now,
		crossedAt:   crossedAt,
		nextAt:      now.Add(a.cfg.FirstAttemptDelay),
		asked:       map[string]bool{},
	}
	a.pendingOrder = append(a.pendingOrder, hash)
}

func (a *Archive) tickLoop() {
	t := time.NewTicker(a.cfg.TickInterval)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case now := <-t.C:
			a.pumpFetches(now)
		}
	}
}

// pumpFetches issues at most one outstanding GENOME_REQUEST per hash, asking
// the envelope's sourcePeer first because that sidecar hashed and cached the
// blob, then the other live peers in ring order, one at a time (§10).
//
// IT IS BOUNDED IN WORK AND IN BURST, AND THAT IS §21, B21. Before that bound
// existed the pump walked the WHOLE pending map on every tick, under the one
// lock the read loop also needs, calling the genome store's Has — an os.Stat —
// on every entry. At the 64,736-entry backlog of 2026-08-10 that was ~0.3-1.0 s
// of held lock per one-second tick: the read loop was admitted about once per
// pass, the relay's 128-frame outbound queue to the archive filled in about four
// seconds, and the relay closed the session with 1011 "outbound queue full".
// The resubscribe changed nothing about the backlog, so it happened again — 26
// drops in 30 minutes, and 7,789 crossings the ledger will never hold.
//
// Nothing here changes WHEN a gap is retried. The ladder lives in nextAt and
// retryDelay and is untouched; what is bounded is how much of the backlog one
// tick may examine and how many requests it may put on the wire.
func (a *Archive) pumpFetches(now time.Time) {
	// Never examine more entries than there are: a small backlog is walked
	// exactly once per tick, which is what it always was.
	a.mu.Lock()
	a.reapExpiredLocked(now)
	total := len(a.pending)
	a.mu.Unlock()
	if total > a.cfg.ScanPerTick {
		total = a.cfg.ScanPerTick
	}
	scanned, issued := 0, 0
	for scanned < total && issued < a.cfg.MaxRequestsPerTick {
		budget := total - scanned
		if budget > contractb.GenomeScanChunk {
			budget = contractb.GenomeScanChunk
		}
		s, i, more := a.pumpChunk(now, budget, a.cfg.MaxRequestsPerTick-issued)
		scanned += s
		issued += i
		if !more {
			return
		}
	}
}

// pumpChunk examines at most scanBudget entries under ONE acquisition of the
// archive's lock and returns what it did and whether the caller should come
// back for more. The chunk is the yield: whatever the size of the backlog, the
// read loop waits at most one chunk for the lock, so the heartbeat is answered
// and the relay's queue drains while the pump is still working.
func (a *Archive) pumpChunk(now time.Time, scanBudget, sendBudget int) (scanned, issued int, more bool) {
	a.mu.Lock()
	// LIFO: compact under the lock, then release it.
	defer a.mu.Unlock()
	defer a.compactPendingOrderLocked()
	if !a.ready {
		return 0, 0, false
	}
	for scanned < scanBudget && issued < sendBudget {
		hash, ok := a.nextPendingLocked()
		if !ok {
			// Nothing left in the backlog to examine.
			return scanned, issued, false
		}
		scanned++
		f := a.pending[hash]
		if a.genomes.Has(hash) {
			// Resolved by some other path: retire it and do NOT push it back.
			a.clearInFlightLocked(f)
			delete(a.pending, hash)
			continue
		}
		if a.gapPastHorizonLocked(f.crossedAt, now) {
			// Aged out (§23, B34). Also not pushed back: this is the drain the
			// queue never had, and it runs inside the same bounded, yielding walk
			// as everything else in this pass.
			a.retireGapLocked(f, now)
			continue
		}
		a.pendingOrder = append(a.pendingOrder, hash)
		if f.inFlight != "" {
			if now.Before(f.deadline) {
				continue
			}
			// genomeRequestTimeoutMs elapsed: the attempt failed.
			a.clearInFlightLocked(f)
			f.nextAt = now.Add(a.retryDelay(f.attempts))
			continue
		}
		if now.Before(f.nextAt) {
			continue
		}
		target := a.nextPeerLocked(f)
		if target == "" {
			// Nobody left to ask on this pass. Reset the ladder when the ring's
			// membership changes — a peer that just came back may hold what
			// nobody had. This log line is the gap report of §10: the hash, how
			// long it has been missing, and how often the archive has asked.
			f.asked = map[string]bool{}
			f.attempts++
			f.nextAt = now.Add(a.retryDelay(f.attempts))
			a.log.Warn("archive: no peer has served this genome",
				"genomeHash", f.hash, "firstSeen", f.firstSeen.UTC().Format(time.RFC3339),
				"attempts", f.attempts, "retryIn", a.retryDelay(f.attempts).String())
			continue
		}
		if a.inFlight[target] >= a.cfg.MaxInFlightPerPeer {
			continue
		}
		if !a.allowSendLocked(target, now) {
			continue
		}
		req := contractb.GenomeRequest{
			RequestID:  wire.NewUUID(),
			SourcePeer: a.cfg.PeerID,
			DestPeer:   target,
			GenomeHash: hash,
			Context: &contractb.GenomeContext{
				MigrationID: f.migrationID,
				EntityID:    f.entityID,
			},
		}
		if !a.sendLocked(contractb.TypeGenomeRequest, req) {
			// The session is going or gone. Walking on would log one failure per
			// entry and put nothing on the wire; the next session's pump picks up
			// from the same place in the cycle.
			return scanned, issued, false
		}
		issued++
		f.inFlight = req.RequestID
		f.deadline = now.Add(contractb.GenomeRequestTimeout)
		f.inFlightPeer = target
		f.inFlightGen = a.sessionGen
		a.inFlight[target]++
		a.outstanding[hash] = f
		f.asked[target] = true
		f.attempts++
		a.log.Info("archive: asking for a genome by hash",
			"genomeHash", hash, "peer", target, "attempt", f.attempts)
	}
	return scanned, issued, true
}

// nextPendingLocked pops the next hash in round-robin order, dropping entries
// the map no longer holds as it passes them. An entry that is still a gap is
// pushed back by the caller, so the cursor can only run off the end when there
// is nothing live left — which is the one case ok is false.
func (a *Archive) nextPendingLocked() (string, bool) {
	for a.pendingHead < len(a.pendingOrder) {
		hash := a.pendingOrder[a.pendingHead]
		a.pendingHead++
		if _, live := a.pending[hash]; live {
			return hash, true
		}
	}
	return "", false
}

// compactPendingOrderLocked reclaims the consumed prefix, once per pass over
// the backlog rather than per entry, so the ring costs O(1) amortised and never
// grows without bound behind the cursor.
func (a *Archive) compactPendingOrderLocked() {
	if a.pendingHead == 0 {
		return
	}
	if a.pendingHead < len(a.pendingOrder) && (a.pendingHead < 1024 || a.pendingHead*2 < len(a.pendingOrder)) {
		return
	}
	a.pendingOrder = append(a.pendingOrder[:0], a.pendingOrder[a.pendingHead:]...)
	a.pendingHead = 0
}

// reapExpiredLocked retires every request whose genomeRequestTimeoutMs has
// elapsed. It walks only the outstanding set, which is bounded by the in-flight
// cap, so it costs the same whether the backlog is 60 gaps or 60,000 — and it
// keeps the timeout falling due at exactly the moment it always did, instead of
// waiting for the round-robin cursor to come back round to that entry.
func (a *Archive) reapExpiredLocked(now time.Time) {
	for _, f := range a.outstanding {
		if f.inFlight == "" || now.Before(f.deadline) {
			continue
		}
		// genomeRequestTimeoutMs elapsed: the attempt failed.
		a.clearInFlightLocked(f)
		f.nextAt = now.Add(a.retryDelay(f.attempts))
	}
}

// clearInFlightLocked forgets one outstanding request and gives the peer its
// concurrency slot back. A request from an ended session was already forgotten
// when that session's accounting was dropped, so it does not decrement twice.
func (a *Archive) clearInFlightLocked(f *fetch) {
	if f.inFlight == "" {
		return
	}
	if f.inFlightPeer != "" && f.inFlightGen == a.sessionGen {
		if n := a.inFlight[f.inFlightPeer]; n > 1 {
			a.inFlight[f.inFlightPeer] = n - 1
		} else {
			delete(a.inFlight, f.inFlightPeer)
		}
	}
	delete(a.outstanding, f.hash)
	f.inFlight = ""
	f.inFlightPeer = ""
	f.inFlightGen = 0
}

func (a *Archive) retryDelay(attempts int) time.Duration {
	if attempts <= 0 {
		return a.cfg.RetrySchedule[0]
	}
	if attempts > len(a.cfg.RetrySchedule) {
		return a.cfg.RetrySchedule[len(a.cfg.RetrySchedule)-1]
	}
	return a.cfg.RetrySchedule[attempts-1]
}

// nextPeerLocked picks who to ask: the source peer first, then live peers in
// STRUCTURAL ORDER (§6.5), skipping anyone already asked in this round.
func (a *Archive) nextPeerLocked(f *fetch) string {
	if f.sourcePeer != "" && !f.asked[f.sourcePeer] && a.liveLocked(f.sourcePeer) {
		return f.sourcePeer
	}
	for _, slot := range a.status.Slots {
		if slot.Live && !f.asked[slot.PeerID] {
			return slot.PeerID
		}
	}
	return ""
}

func (a *Archive) liveLocked(peerID string) bool {
	for _, slot := range a.status.Slots {
		if slot.PeerID == peerID {
			return slot.Live
		}
	}
	// Before the first PEER_STATUS the archive knows nothing about the map.
	// Asking is cheap and the relay answers peer_offline if it is wrong.
	return len(a.status.Slots) == 0
}

func (a *Archive) allowSendLocked(peerID string, now time.Time) bool {
	w, ok := a.sentWindow[peerID]
	if !ok || now.Sub(w.start) > time.Minute {
		a.sentWindow[peerID] = &rateWindow{start: now, count: 1}
		return true
	}
	if w.count >= a.cfg.RequestsPerMinute {
		return false
	}
	w.count++
	return true
}

// onGenomeResponse verifies and stores one fetched genome.
//
// §6.10: on found the requester MUST recompute the hash of the bytes and
// discard the answer if it differs. This is content addressing: a store that
// trusts the label instead of the bytes is not content-addressed, and one wrong
// genome silently poisons every lineage query that touches it.
func (a *Archive) onGenomeResponse(env wire.Envelope) bool {
	var resp contractb.GenomeResponse
	if contractb.DecodeData(env.Data, &resp) != nil {
		return true
	}
	now := time.Now()
	a.mu.Lock()
	f := a.pending[resp.GenomeHash]
	if f != nil {
		a.clearInFlightLocked(f)
	}
	a.mu.Unlock()

	if !resp.Found {
		// unknown_hash is a normal answer, not an error.
		a.log.Info("archive: genome not served", "genomeHash", resp.GenomeHash,
			"peer", resp.SourcePeer, "reason", resp.Reason)
		a.mu.Lock()
		if f != nil {
			switch resp.Reason {
			case contractb.GenomeRateLimited:
				// Do not count a rate limit as an answer from that peer.
				delete(f.asked, resp.SourcePeer)
				wait := time.Duration(resp.RetryAfterMs) * time.Millisecond
				if wait <= 0 {
					wait = time.Minute
				}
				f.nextAt = now.Add(wait)
			default:
				f.nextAt = now
			}
		}
		a.mu.Unlock()
		return true
	}
	if resp.Body == nil {
		return true
	}
	got, err := bb8.GenomeHash(resp.Body.BB8, resp.Body.Version)
	if err != nil || got != resp.GenomeHash {
		a.log.Error("archive: DISCARDING a fetched genome whose bytes do not hash to its label",
			"genomeHash", resp.GenomeHash, "computed", got, "peer", resp.SourcePeer, "err", err)
		a.mu.Lock()
		if f != nil {
			f.nextAt = now.Add(a.retryDelay(f.attempts))
		}
		a.mu.Unlock()
		return true
	}
	if err := a.genomes.Put(resp.GenomeHash, resp.Body.Version, resp.Body.BB8); err != nil {
		a.log.Error("archive: genome store write failed", "genomeHash", resp.GenomeHash, "err", err)
		return true
	}
	a.mu.Lock()
	delete(a.pending, resp.GenomeHash)
	// A GENOME line is a ledger record and is counted like one. It was not until
	// 2026-08-12, and the omission made `ledgerRecords` drift BELOW `wc -l` from
	// the first fetch after every boot — 8,060,891 against 8,156,869 lines on the
	// deployment, 1.2% low — because the startup replay counts these lines and
	// the live path did not. The counter and the file now answer the same
	// question, and `ledgerSkipped` is once again the whole of the difference.
	a.recordCount++
	a.mu.Unlock()
	_ = a.ledger.Append(Record{
		Type:       RecordGenome,
		RecordedAt: now.UnixMilli(),
		GenomeHash: resp.GenomeHash,
		ServedBy:   resp.SourcePeer,
	})
	a.log.Info("archive: stored a genome fetched by hash",
		"genomeHash", resp.GenomeHash, "servedBy", resp.SourcePeer)
	return true
}

func (a *Archive) sendLocked(typ string, data any) bool {
	if a.conn == nil || !a.ready {
		return false
	}
	frame, err := wire.Encode(wire.ProtocolB, typ, time.Now().UnixMilli(), data)
	if err != nil {
		a.log.Error("archive: encode failed", "type", typ, "err", err)
		return false
	}
	if err := a.conn.Send(frame); err != nil {
		a.log.Warn("archive: send failed", "type", typ, "err", err)
		return false
	}
	return true
}

// ---------------------------------------------------------------- read path

// Migration is one migration as the read path presents it: the envelope's own
// fields, its lineage, and whether the archive actually holds each genome.
type Migration struct {
	Record
	Outcome     string
	GenomeHeld  bool
	ParentsHeld []bool
}

// List replays the ledger in dir and joins each migration with the genome
// store. It is deliberately a full replay: M3 records and reads only, and an
// index is the first thing a query engine would need (see store.go).
//
// It returns what the replay skipped alongside the migrations, because a
// listing that quietly omits a crossing is the shape of the 2026-08-08 loss:
// the caller prints it.
func List(dir string) ([]Migration, LedgerDamage, error) {
	store, err := bb8.OpenStore(dir + "/genomes")
	if err != nil {
		return nil, LedgerDamage{}, err
	}
	byID := map[string]*Migration{}
	var order []*Migration
	// Streamed for the same reason New is (ScanLedger): the join below keeps
	// only the migrations, so holding the ACKs and NACKs — half the file — in a
	// second slice while it runs bought nothing. `list` on a live ledger is a
	// command an operator types on the box that is also serving the map.
	damage, err := ScanLedger(dir, func(rec Record) {
		switch rec.Type {
		case RecordMigration:
			m := &Migration{Record: rec, Outcome: "pending"}
			if rec.Lineage != nil {
				m.GenomeHeld = rec.Lineage.GenomeHash != "" && store.Has(rec.Lineage.GenomeHash)
				for _, p := range rec.Lineage.Parents {
					m.ParentsHeld = append(m.ParentsHeld, p.GenomeHash != "" && store.Has(p.GenomeHash))
				}
			}
			byID[rec.MigrationID] = m
			order = append(order, m)
		case RecordAck:
			if m, ok := byID[rec.MigrationID]; ok {
				m.Outcome = "delivered"
				m.DestPeer = rec.DestPeer
				if rec.Duplicate {
					m.Outcome = "delivered (duplicate)"
				}
			}
		case RecordNack:
			if m, ok := byID[rec.MigrationID]; ok && m.Outcome == "pending" {
				m.Outcome = "refused " + rec.Code
			}
		}
	})
	if err != nil {
		return nil, damage, err
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].RecordedAt < order[j].RecordedAt })
	out := make([]Migration, 0, len(order))
	for _, m := range order {
		out = append(out, *m)
	}
	return out, damage, nil
}
