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
// M4's archive is still a recorder. It has no write interface, no query engine
// and no authentication of its own — it is a subscriber that trusts the shared
// token (§13 item 4).
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
	"multiverse/internal/lantoken"
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

// Version is reported to the relay in HANDSHAKE.sidecarVersion.
const Version = "m4.0"

// DefaultRetrySchedule is contract-b-m4.md §10's ladder: 1 minute, 5 minutes,
// 30 minutes, 6 hours, then daily.
var DefaultRetrySchedule = []time.Duration{
	time.Minute, 5 * time.Minute, 30 * time.Minute, 6 * time.Hour, 24 * time.Hour,
}

// Config is the archive's runtime configuration.
type Config struct {
	RelayURL string
	Token    string
	PeerID   string
	DataDir  string
	Logger   *slog.Logger

	RelayBackoffMin time.Duration
	RelayBackoffMax time.Duration
	// FirstAttemptDelay is how long a newly seen hash waits before its first
	// fetch. It exists so the ordinary case — ask the source peer at once — is
	// not slowed by the retry ladder.
	FirstAttemptDelay time.Duration
	RetrySchedule     []time.Duration
	RequestsPerMinute int
	TickInterval      time.Duration

	// HTTPListen is the status page's bind address, "" for no page. The rig
	// uses 127.0.0.1:8796; a test uses 127.0.0.1:0.
	HTTPListen string
	// StatsStale is §10.1's honesty threshold: a stats block older than this
	// renders as UNKNOWN rather than as state.
	StatsStale time.Duration
	// MetricsInterval is how often a PEER_STATUS sample is appended to
	// <data-dir>/metrics.jsonl, so history survives everything (WP3, WP5).
	MetricsInterval time.Duration
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
	attempts    int
	nextAt      time.Time
	// asked records the peers that answered unknown_hash for this hash, so the
	// ring is walked one peer at a time rather than re-asking the same one.
	asked map[string]bool
	// inFlight is the requestId of an unanswered GENOME_REQUEST.
	inFlight string
	deadline time.Time
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
	sentWindow    map[string]*rateWindow
	closed        bool

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
	a := &Archive{
		cfg:        cfg,
		log:        cfg.Logger.With("archive", cfg.PeerID),
		ledger:     ledger,
		genomes:    genomes,
		metrics:    metrics,
		lanes:      map[lanePair]*lane{},
		simRates:   map[int]*achievedRate{},
		seen:       map[string]bool{},
		pending:    map[string]*fetch{},
		sentWindow: map[string]*rateWindow{},
		species:    newSpeciesLedger(),
	}
	// Replay what is already recorded, so a restart neither duplicates a record
	// nor forgets a gap. "Keep the hash forever" (§10): a hash with no genome
	// is still a lineage node, and a fetch that failed for a year can succeed
	// tomorrow.
	records, damage, err := ReadLedger(cfg.DataDir)
	if err != nil {
		return nil, err
	}
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
			"records", len(records), "ledger", ledger.Path())
	}
	if damage.TornTail > 0 {
		a.log.Warn("archive: ignored an unfinished record at the end of the ledger",
			"bytes", damage.TornTail, "ledger", ledger.Path())
	}
	now := time.Now()
	a.recordCount = len(records)
	a.ledgerSkipped = damage.Lines
	for _, rec := range records {
		if rec.MigrationID != "" {
			// Rebuild the key the live path uses, not a lookalike. A NACK
			// dedups on migrationId+code (§14, B7), so replaying it under
			// migrationId alone would never match and every re-copied NACK
			// would be recorded a second time after a restart.
			a.seen[rec.Type+"/"+dedupKey(rec.Type, rec.MigrationID, rec.Code)] = true
		}
		if rec.Type != RecordMigration {
			continue
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
			continue
		}
		for _, h := range hashesOf(rec) {
			a.trackLocked(h, rec.SourcePeer, rec.MigrationID, rec.EntityID, now)
		}
	}
	if n := len(a.pending); n > 0 {
		a.log.Warn("archive: resumed with genomes still missing", "gaps", n,
			"records", len(records))
	}
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
			a.log.Error("archive: the relay rejected our LAN token with HTTP 401",
				"consecutiveFailures", authFailures)
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
	if a.cfg.Token != "" {
		opts.HTTPHeader = http.Header{"Authorization": []string{lantoken.Header(a.cfg.Token)}}
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
		a.trackLocked(h, p.SourcePeer, p.MigrationID, p.EntityID, now)
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
func (a *Archive) trackLocked(hash, sourcePeer, migrationID string, entityID int32, now time.Time) {
	if hash == "" || a.genomes.Has(hash) {
		return
	}
	if f, ok := a.pending[hash]; ok {
		if f.sourcePeer == "" {
			f.sourcePeer = sourcePeer
		}
		return
	}
	a.pending[hash] = &fetch{
		hash:        hash,
		sourcePeer:  sourcePeer,
		migrationID: migrationID,
		entityID:    entityID,
		firstSeen:   now,
		nextAt:      now.Add(a.cfg.FirstAttemptDelay),
		asked:       map[string]bool{},
	}
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
func (a *Archive) pumpFetches(now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.ready {
		return
	}
	for hash, f := range a.pending {
		if a.genomes.Has(hash) {
			delete(a.pending, hash)
			continue
		}
		if f.inFlight != "" {
			if now.Before(f.deadline) {
				continue
			}
			// genomeRequestTimeoutMs elapsed: the attempt failed.
			f.inFlight = ""
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
			continue
		}
		f.inFlight = req.RequestID
		f.deadline = now.Add(contractb.GenomeRequestTimeout)
		f.asked[target] = true
		f.attempts++
		a.log.Info("archive: asking for a genome by hash",
			"genomeHash", hash, "peer", target, "attempt", f.attempts)
	}
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
		f.inFlight = ""
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
	records, damage, err := ReadLedger(dir)
	if err != nil {
		return nil, damage, err
	}
	store, err := bb8.OpenStore(dir + "/genomes")
	if err != nil {
		return nil, damage, err
	}
	byID := map[string]*Migration{}
	order := make([]*Migration, 0, len(records))
	for _, rec := range records {
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
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].RecordedAt < order[j].RecordedAt })
	out := make([]Migration, 0, len(order))
	for _, m := range order {
		out = append(out, *m)
	}
	return out, damage, nil
}
