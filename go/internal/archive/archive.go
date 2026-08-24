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
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"path/filepath"
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
	// MonitorStateDir and HostMetricsFile are the two read-only inputs for the
	// public production-health dashboard. Empty means unavailable, which is the
	// portable default: an archive used on a participant LAN has neither file.
	//
	// The serving boundary never returns either path, arbitrary files from the
	// monitor directory, or the monitor's alert text. health.go projects a fixed
	// allow-list of verdicts and numeric host fields so enabling this surface does
	// not turn an operator file into a public file browser.
	MonitorStateDir string
	HostMetricsFile string
	// StatsStale is §10.1's honesty threshold: a stats block older than this
	// renders as UNKNOWN rather than as state.
	StatsStale time.Duration
	// DedupWindow is how long the §5.1 duplicate set remembers a record's key
	// (§25, B38). It is the archive's one unbounded structure made bounded: a
	// legitimate duplicate stopped existing when the re-forward did (§25, B37),
	// so what is left to absorb is an old sidecar's retry and a defective peer,
	// both of which arrive within minutes. 0 takes the contract default of 48 h.
	//
	// The retention it buys is AT LEAST this and at most twice it, and the memory
	// is at most two windows of keys — deploy/SIZING.md has the arithmetic.
	DedupWindow time.Duration
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

	// LedgerWindow is how long a CLOSED LEDGER SEGMENT is kept on this host
	// (segments.go, "THE WINDOW"). It is a raw-lines rule and never an answers
	// rule: every aggregate the archive publishes is kept forever either way.
	//
	//	> 0  that duration
	//	  0  THE DEFAULT: whatever GenomeHorizon is, which is itself 0 — off.
	//	     One horizon, three mechanisms (§23, B34, extended to the raw
	//	     ledger): a raw window equal to the genome horizon holds exactly the
	//	     crossings whose gaps can still be fetched.
	//	< 0  OFF explicitly, whatever the horizon is: rotate and compress, retire
	//	     nothing.
	//
	// A SEGMENT PAST THE WINDOW IS STILL NOT REMOVED WITHOUT A CONFIRMED
	// OFF-HOST COPY. See coldcopy.go: no receipt, no retirement, forever if need
	// be.
	LedgerWindow time.Duration
	// LedgerMaintenanceInterval is how often the segment pass runs — compress
	// what is closed, retire what is past the window and confirmed off-host,
	// refresh the status counters. Defaults to five minutes. NEGATIVE disables
	// the loop entirely, for a test that drives LedgerMaintenanceNow by hand.
	LedgerMaintenanceInterval time.Duration
	// DisableLedgerCompression leaves closed segments in their plain form. It
	// exists for tests and for a host with no CPU to spare; a plain segment is
	// never retired, because the only thing that leaves this host is a verified
	// .jsonl.gz with a receipt.
	DisableLedgerCompression bool

	// BroadcastPeerID is the peer id of the world the shared camera at /watch is
	// pointed at. NOTHING ON THE WIRE CARRIES THIS: a world does not announce
	// that it is being filmed, so the deployment that runs the publisher is the
	// only party that knows, and it tells this archive here. Empty — the default
	// — means no world is named, and both pages then say so rather than guess.
	// It changes no placement, no routing and no record; it is display only.
	BroadcastPeerID string

	// HomepageRepo is the GitHub organization and repository for links from the
	// landing page. The landing page's download links carry no release number:
	// they address GitHub's /releases/latest, so the newest published release is
	// what a visitor gets without anything here being changed or redeployed.
	HomepageRepo string
	// HomepageGameVersion is the game build label shown in the landing copy.
	HomepageGameVersion string
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
	if c.DedupWindow <= 0 {
		c.DedupWindow = contractb.ArchiveDedupWindow
	}
	if c.MetricsInterval <= 0 {
		c.MetricsInterval = time.Minute
	}
	if c.HomepageRepo == "" {
		c.HomepageRepo = defaultHomepageRepo()
	}
	if c.HomepageGameVersion == "" {
		c.HomepageGameVersion = defaultHomepageGameVersion()
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
	// migrant and speciesKey are what the BRAIN AGGREGATE needs when this blob
	// lands, and they are carried here for the same reason crossedAt is: the
	// arrival is the only moment the bytes are free, and by then the record that
	// asked for them is long gone (brainhist.go rule 1).
	//
	// migrant is true when this hash is the MIGRANT'S OWN genome rather than a
	// parent's. Only the migrant's is measured into the time series: a parent
	// hash is the genome of the migrant's mother or father, which described an
	// EARLIER organism, and folding it at the child's crossing time would drag
	// older brains forward and flatten the very trend the panel exists to show.
	// speciesKey is that migrant's A34 comparison key, and is empty for a parent
	// hash — the species block names a TAXONOMIC parent species, not the species
	// of the individual whose genome this is, and attributing one to the other
	// would be a measurement no record supports.
	migrant    bool
	speciesKey string
	// asked records the peers that answered unknown_hash for this hash, so the
	// ring is walked one peer at a time rather than re-asking the same one.
	//
	// IT IS NIL UNTIL THE FIRST PEER IS ASKED, and it goes back to nil whenever
	// the ladder resets. A restart discovers every missing hash the ledger names
	// in a single pass and the pump asks a bounded few of them per tick, so most
	// entries in a fresh pending set have never been asked anything; a map
	// allocated for each of them is a header per gap for a set that stays empty.
	// Reading a nil map is legal and answers false, which is exactly "nobody has
	// been asked yet", and deleting from one is a no-op.
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
	// releases is the landing page's answer to "which release do these buttons
	// hand me". It is on the serving side too, it touches no record, and the
	// page is complete without it — see release.go. Non-nil from New; it simply
	// answers "" until its background lookup has succeeded, which is a state the
	// page renders correctly.
	releases *releaseTracker

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
	// hopPending and hopEarlyAcks correlate an offered MIGRATION_PAYLOAD with
	// the receiver's eventual MIGRATION_ACK. The payload is not proof that the
	// destination spawned the organism: a population admission NACK can follow
	// it, and the same migrationId can then be offered to another slot. Only the
	// matched ACK promotes an attempt into hops. hopPending also carries the
	// ordered slots that explicitly refused before that ACK so the page can draw
	// the reroute honestly. Both maps are ephemeral and bounded in hops.go; the
	// durable ledger remains unchanged.
	hopPending   map[string]pendingHop
	hopEarlyAcks map[string]earlyHopAck
	hopTrimAtMs  int64
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
	//
	// SINCE THE ROLL-UP SIDECAR it is also PERSISTED (rollup.go): a tail replay
	// does not re-read the region the sidecar covers, so it cannot re-discover
	// the damage in it, and a number that fell to zero because the archive
	// stopped looking would be worse than no number at all.
	ledgerSkipped int
	// ---- the roll-up state sidecar (rollup.go), the record roll-up.
	//
	// ledgerPos is THE CURSOR: the position in the segmented ledger, and the
	// record number, that the aggregates cover. It is exact because every live
	// append writes first and folds second under one lock (store.go, AppendAt),
	// so the fold and the offset can never disagree. tally is the per-type,
	// per-peer and first/last record accounting the M5 evidence needs.
	// rollupIndex maps an hour of ledger time to the position its first record
	// sits at, so a replay that needs only the newest N hours of RAW records can
	// seek to where they start. rollupDirty is what has changed since the last
	// save, per key.
	ledgerPos       LedgerPos
	tally           recordTally
	rollup          *rollupSidecar
	rollupIndex     map[int64]LedgerPos
	rollupDirty     rollupDirty
	rollupCovered   LedgerPos
	rollupSavedAtMs int64
	rollupLoaded    bool
	rollupLost      bool
	rollupSavedAt   time.Time
	// ---- what the LAST START cost, measured rather than modelled, and what the
	// duplicate guard has refused since the archive began.
	//
	// replayRawRecords and replayRawSeconds are the raw scan: how many records it
	// parsed and how long it took. They are published because the whole roll-up
	// rests on a claim about restart time, and a claim about restart time that
	// nobody measures is a claim nobody can check — monitor.sh gates on these
	// rather than on a model of the record count.
	//
	// replayFromRetired says the saved cursor named a segment that has left this
	// host, so the scan restarted at the oldest segment still present and the
	// aggregates have a hole between the two. replayConverted says the saved
	// position could not be used as it stood and was rebuilt by one walk.
	//
	// duplicatesRefused is ALL-TIME and persisted (§25, B38, and decision 0006):
	// during the transition a non-zero value is a peer still running a build that
	// re-forwards, and afterwards it is a defect report. It is the number that
	// says whether the 48 h duplicate window can safely come down to an hour, and
	// a guard whose refusals nobody counts is a guard nobody can trust.
	replayRawRecords  int
	replayRawSeconds  float64
	replayFromRetired bool
	replayConverted   bool
	duplicatesRefused int
	// seen is the §5.1 duplicate rule: one entry for every duplicate key the
	// record holds WITHIN THE LAST cfg.DedupWindow, so a re-copied envelope is
	// refused rather than appended a second time. It used to remember every key
	// forever, because a re-forward could arrive a year later; §25's B37 removed
	// the re-forward, and B38 made this a rotating window whose memory is a
	// function of the window and not of the record. It holds a 128-bit
	// fingerprint of each key rather than the key: dedup.go has the measurement
	// that motivated that, the collision bound, the rotation, and why the seeds
	// are per process.
	seen    *dedupWindow
	pending map[string]*fetch
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
	// seg is the segmented ledger's counters: how many closed segments are on
	// this host, how many bytes they hold, where the raw window starts, how many
	// are waiting for a cold-copy receipt, and how many have been retired. It is
	// refreshed by the maintenance pass rather than by the status request, so a
	// page load never walks a directory. See segments.go.
	seg segState
	// evict is the retention horizon's cursor and its counters (§23, B33/B34).
	// It is the zero value, and every path that reads it returns early, on an
	// archive with no horizon — which is every archive by default. See
	// eviction.go.
	evict evictState

	// The history strip's cache. It is deliberately NOT under mu: building a
	// history reads a file, and nothing that reads a file may hold the lock the
	// migration path takes (Risk 4).
	historyMu     sync.Mutex
	historyKey    string
	historyAt     time.Time
	historyVal    History
	historyAllAt  time.Time
	historyAllVal History
	// The health dashboard tails at most 1 MiB of host samples. Cache that public
	// projection briefly so several open dashboards do not turn a one-minute
	// instrument into repeated disk reads. This lock is separate from mu: a file
	// read must never hold the migration-path lock.
	healthMu     sync.Mutex
	healthAt     time.Time
	healthCached productionHealthView
	// The species detail's sparkline cache, kept apart from the strip's for the
	// same reason and bounded in entries as well as in age: a reader clicking
	// through species must not re-read the sample file per click.
	speciesHistory speciesHistoryCache
	// The genealogy's brain-shape cache, kept apart from both for the third time
	// and the same reason: it reads the genome store, and it is keyed on content
	// so one hash is parsed once for the life of the process. See brain.go.
	brains brainCache
	// brainAgg is the MAINTAINED brain-complexity aggregate: the five-minute
	// series the panel under the genealogy draws, and the persisted per-species
	// measurement its rings are drawn from. It carries its own lock — a view
	// copies a window of histograms out of it, and nothing that walks thousands
	// of map entries may hold the lock the migration path takes — and brainSave
	// is its durable half. See brainhist.go and brainsave.go.
	brainAgg     *brainAgg
	brainSave    *brainSidecar
	brainSavedAt time.Time
	brainHistMu  sync.Mutex
	brainHist    map[string]brainHistoryEntry
}

type rateWindow struct {
	start time.Time
	count int
}

// New opens the archive's store and returns it. Nothing is dialled yet.
func New(cfg Config) (*Archive, error) {
	cfg.applyDefaults()
	// OpenLedgerLog, not OpenLedger: the first start on a whole-file ledger
	// renames it into one legacy segment and a crash reconciliation may finish a
	// compression the last process died inside. Both happen once and both belong
	// in the log rather than in a directory listing nobody reads.
	ledger, err := OpenLedgerLog(cfg.DataDir, func(msg string, kv ...any) {
		cfg.Logger.Warn(msg, kv...)
	})
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
		cfg:          cfg,
		log:          cfg.Logger.With("archive", cfg.PeerID),
		ledger:       ledger,
		genomes:      genomes,
		metrics:      metrics,
		deny:         deny,
		releases:     newReleaseTracker(cfg.HomepageRepo, cfg.Logger),
		lanes:        map[lanePair]*lane{},
		simRates:     map[int]*achievedRate{},
		hopPending:   map[string]pendingHop{},
		hopEarlyAcks: map[string]earlyHopAck{},
		seen:         newDedupWindow(dedupHint(ledger.HintBytes()), cfg.DedupWindow, time.Now()),
		pending:      map[string]*fetch{},
		sentWindow:   map[string]*rateWindow{},
		inFlight:     map[string]int{},
		outstanding:  map[string]*fetch{},
		species:      newSpeciesLedger(),
		brainAgg:     newBrainAgg(),
		tally:        newRecordTally(),
		rollupIndex:  map[int64]LedgerPos{},
		rollupDirty:  newRollupDirty(),
	}
	// THE ROLL-UP STATE IS LOADED BEFORE ONE RECORD IS FOLDED (rollup.go). It is
	// the durable half of the fold below: with it, the replay reads only what the
	// sidecar does not already cover; without it, the replay is exactly what it
	// has always been and the first save writes the state it produced.
	rollupPath := filepath.Join(cfg.DataDir, rollupSidecarName)
	rolled, usable, err := loadRollupState(rollupPath)
	if err != nil {
		return nil, err
	}
	if !usable {
		// A file exists and this build cannot use it. THAT IS A LOSS AND IT IS
		// SAID SO — but never a reason to refuse to run. The archive rebuilds what
		// the on-host raw window still contains. If older segments have retired,
		// their aggregates need the confirmed cold copies; rollupLost keeps that
		// difference visible instead of claiming the shorter replay is complete.
		a.rollupLost = true
		a.log.Error("archive: the roll-up state sidecar could not be read; "+
			"the aggregates are being rebuilt by a FULL replay and the old file is "+
			"kept as .unreadable", "path", rollupPath)
		if err := keepUnreadable(rollupPath); err != nil {
			return nil, err
		}
		rolled = nil
	}
	a.applyRollupState(rolled)
	// "Loaded" means the sidecar covered something. A file holding only a header
	// — a save that was interrupted before its first floor line — is a full
	// replay and a first roll-up, exactly as no file at all is.
	a.rollupLoaded = rolled != nil && rolled.cursor.Record > 0
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
	//
	// AND IT IS NOW A TAIL REPLAY WHEN A SIDECAR SAYS IT CAN BE. planReplay
	// (rollup.go) returns TWO cut points over one pass: Floor is where the
	// aggregate fold resumes, because everything before it is already in the
	// sidecar, and From is where the RAW-derived state — the duplicate guard,
	// whose fingerprint seeds are per process and cannot be persisted — has to be
	// rebuilt from. THE DISTANCE BETWEEN THEM IS EXACTLY cfg.DedupWindow, or the
	// raw window when that is shorter, and nothing else reaches back: the
	// genome-gap queue used to and is persisted now.
	//
	// THE RESTART-TIME MODEL, and it is a model of ONE number:
	//
	//	raw records parsed = min(age of the raw window, cfg.DedupWindow) of records
	//	wall time          = that / ~62 000-72 000 records a second on this host
	//
	// which is about 100 s at the shipped 48 h window on a 7 M-record ledger, and
	// 2-3 s at the 1 h the guard needs once the fleet has crossed. Both halves are
	// measured below and published (replayRawRecords, replayRawSeconds), because
	// the deployment gates on them.
	now := time.Now()
	plan := a.planReplay(rolled, now)
	// THE FLOOR IS RESOLVED AGAINST WHAT IS ACTUALLY ON THE HOST before anything
	// is read, because a floor naming a segment that has been retired is a state
	// older than the raw window and the two possible answers are very different.
	// resolveFloor (rollup.go) picks between them and says which.
	if plan.FromSidecar {
		floor, missing, foldAll := a.resolveFloor(cfg.DataDir, plan.Floor)
		if missing {
			a.replayFromRetired = true
			note := "the aggregates are KEPT and NOTHING is re-folded: the cursor is not " +
				"older than the raw lines still here, so re-folding them would double every " +
				"counter"
			if foldAll {
				note = "the aggregates are KEPT and every record still present is folded on " +
					"top; the records between the cursor and the oldest segment still here " +
					"are NOT in the aggregates and cannot be recovered without " +
					"deploy/coldcopy.sh --restore"
			}
			a.log.Error("archive: the roll-up cursor names a raw segment that is no longer "+
				"on this host", "cursorSegment", plan.Floor.Segment,
				"cursorRecord", plan.Floor.Record, "foldWhatIsLeft", foldAll, "note", note)
			plan.Floor = floor
		}
	}
	rawStart := time.Now()
	prev := plan.From
	raw := 0
	// past is the scan's own verdict, taken by POSITION rather than by record
	// number: false means the roll-up state already holds the fold of this
	// record and only the duplicate guard is rebuilt from it.
	fold := func(rec Record, pos LedgerPos, past bool) {
		raw++
		if past {
			a.countRecordLocked(rec, prev)
		}
		a.replayRawLocked(rec, past, now)
		prev = pos
	}
	damage, end, err := scanLedgerFromFloor(cfg.DataDir, plan.From, plan.Floor, fold)
	if errors.Is(err, ErrPositionRetired) {
		// The RAW SCAN's start named a segment that has gone — an hour-index
		// entry older than the window. The floor is unaffected and has already
		// been resolved, so the scan simply starts at the beginning of what is
		// here and the duplicate guard is rebuilt from a little less than the
		// window asked for. Nothing is folded twice: the floor decides that, and
		// it decides it by position.
		a.log.Warn("archive: the raw scan wanted to start in a segment that is no longer on "+
			"this host; it starts at the oldest segment still here",
			"wantedSegment", plan.From.Segment,
			"note", "the duplicate guard covers a little less than --dedup-window for this "+
				"one start; no aggregate is affected")
		plan.From = LedgerPos{}
		prev, raw = LedgerPos{}, 0
		rawStart = time.Now()
		damage, end, err = scanLedgerFromFloor(cfg.DataDir, plan.From, plan.Floor, fold)
	}
	if err != nil {
		return nil, err
	}
	a.replayRawRecords = raw
	a.replayRawSeconds = time.Since(rawStart).Seconds()
	a.replayConverted = plan.Converted
	a.ledgerSkipped += damage.Lines
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
	// THE CURSOR IS WHERE THE SCAN ENDED, with the fold's own record count on it.
	// The two agree by construction — the fold counts exactly the records the
	// scan delivered past the floor — and writing the count in rather than
	// trusting the scan's is what keeps them together after the retired-position
	// fallback above, where the scan renumbers from zero over what is left.
	a.ledgerPos = LedgerPos{Segment: end.Segment, Offset: end.Offset,
		Record: int64(a.recordCount)}
	// THE GAP QUEUE A SIDECAR RESTORED IS DRAINED OF WHAT AGED OUT while this
	// archive was down, so a loaded queue is in the state a full replay would
	// have produced rather than in the state it was saved in (rollup.go).
	if gone, held := a.expireLoadedGapsLocked(now); gone > 0 || held > 0 {
		a.log.Info("archive: drained the restored genome-gap queue",
			"retiredPastHorizon", gone, "alreadyHeld", held, "remaining", len(a.pending),
			"horizon", a.cfg.GenomeHorizon.String())
	}
	// The roll-up sidecar is opened for APPENDING only; loadRollupState above has
	// already read it, because the state has to be in the aggregates before one
	// record is folded on top of it.
	rollupSave, err := openRollupSidecar(cfg.DataDir)
	if err != nil {
		// Same rule as the brain sidecar: an aggregate that cannot be written is
		// an aggregate being lost as it is made, and it is still not a reason to
		// refuse to run. The next start replays in full.
		a.log.Error("archive: the roll-up state sidecar could not be opened; "+
			"the fold will not be kept across this run and the next start replays in full",
			"path", rollupPath, "err", err)
	} else {
		a.rollup = rollupSave
	}
	if !a.rollupLoaded {
		// NO SIDECAR, SO THIS REPLAY IS THE FIRST ROLL-UP. Write it whole and at
		// once rather than waiting for the first tick: an archive killed in its
		// first thirty seconds would otherwise pay for the whole replay twice.
		a.mu.Lock()
		a.rollupDirty.everything = true
		a.mu.Unlock()
		if err := a.rollup.Save(a); err != nil {
			a.log.Error("archive: the first roll-up state could not be written", "err", err)
		}
	}
	rate := 0.0
	if a.replayRawSeconds > 0 {
		rate = float64(a.replayRawRecords) / a.replayRawSeconds
	}
	a.log.Info("archive: replayed the record",
		"records", a.recordCount, "skippedLines", a.ledgerSkipped,
		"fromRollup", plan.FromSidecar, "rollupCoveredRecords", plan.Floor.Record,
		"rawRecordsParsed", a.replayRawRecords,
		"rawSeconds", fmt.Sprintf("%.2f", a.replayRawSeconds),
		"rawRecordsPerSecond", fmt.Sprintf("%.0f", rate),
		"rawRebuildSpan", plan.RawSpan.String(),
		"scannedFromSegment", plan.From.Segment, "scannedFromOffset", plan.From.Offset,
		"positionConverted", plan.Converted, "positionRetired", a.replayFromRetired,
		"genomeGaps", len(a.pending), "genomeGapsExpired", a.evict.gapsExpired,
		"duplicatesRefused", a.duplicatesRefused,
		"rollup", a.rollup.Path())
	// THE BRAIN SIDECAR IS LOADED AFTER THE LEDGER REPLAY, and the order is a
	// correctness requirement rather than a preference. The replay rebuilds the
	// COVERAGE DENOMINATOR by walking the record forwards, which the aggregate
	// deduplicates against a window at its own write frontier; loading the
	// sidecar first would put that frontier at today and make every replayed
	// crossing look like a late arrival, which is the one case the window cannot
	// deduplicate. Loaded second, the sidecar only ever fills in the half the
	// ledger cannot answer: what was inside each genome.
	save, err := openBrainSidecar(cfg.DataDir, a.brainAgg)
	if err != nil {
		// A sidecar that cannot be opened is not a reason to refuse to run: the
		// panel loses its history and every other thing this archive does is
		// unaffected. It is logged at ERROR because a measurement that cannot be
		// written is one that is being lost as it is made.
		a.log.Error("archive: the brain history sidecar could not be opened; "+
			"brain history will not be kept across this run",
			"path", filepath.Join(cfg.DataDir, brainSidecarName), "err", err)
	} else {
		a.brainSave = save
		a.brainAgg.mu.Lock()
		// MEASURED buckets, not buckets: the ledger replay above has already put
		// a bucket in the map for every five minutes that holds a crossing, and
		// counting those would report a sidecar load that never happened.
		loaded, lost := 0, a.brainAgg.lost
		for _, b := range a.brainAgg.buckets {
			if b.held > 0 {
				loaded++
			}
		}
		records := len(a.brainAgg.species)
		a.brainAgg.mu.Unlock()
		if lost {
			// IT IS A LOSS AND IT IS SAID SO. The history starts now — never at
			// zero — and the unreadable bytes are kept beside the new file.
			a.log.Error("archive: the brain history sidecar could not be read; "+
				"brain history STARTS NOW and the old file is kept as .unreadable",
				"path", save.Path())
		} else if loaded > 0 || records > 0 {
			a.log.Info("archive: replayed the brain history sidecar",
				"buckets", loaded, "speciesMeasured", records, "path", save.Path())
		}
	}
	a.logRetiredAtStartup()
	return a, nil
}

// replayRawLocked is the fold ONE ledger record contributes to during a replay.
//
// agg says whether the record is behind the roll-up sidecar's cursor. When it is
// false the record is already in the aggregates — the sidecar holds the fold of
// it — and ONE THING ONLY is rebuilt from it: the duplicate guard, whose
// per-process fingerprint seeds make it unpersistable by design (dedup.go).
// Every aggregate the sidecar owns is skipped, and that skip is the whole
// saving.
//
// THE GENOME-GAP QUEUE IS NOW ON THE agg SIDE, and that is phase 3's change. It
// used to be rebuilt from the raw record inside the retention horizon, which is
// what forced a restart to read 720 h — or, on an archive with no horizon, the
// whole record. It is persisted now (rollup.go, the "gq" lines), so it is folded
// exactly once per crossing like every other aggregate, and the raw scan is
// bounded by the duplicate window alone.
//
// It runs on New's goroutine before anything else is started, so it takes no
// lock; the Locked suffix says which lock its callers would need.
func (a *Archive) replayRawLocked(rec Record, agg bool, now time.Time) {
	if rec.MigrationID != "" {
		// Rebuild the key the live path uses, not a lookalike. A legacy NACK
		// dedups on migrationId+code (§14, B7). A field-present 4.2 queue refusal
		// also includes destSlot and rerouteCount (§31, B46). Replaying either
		// under migrationId alone would record a re-copied NACK after restart.
		// THE REPLAY ONLY REBUILDS THE WINDOW, not the record (§25, B38). A key
		// whose record is older than the window is not inserted at all: the live
		// path would not have refused a copy of it either, and inserting it would
		// make the set the size of the ledger again for the first window after
		// every restart. recordedAt is the LEDGER's clock, so the rotation below
		// walks the ledger's own time.
		//
		// IT IS FOLDED WHETHER OR NOT agg, and that is the reason a tail replay
		// cannot be shorter than cfg.DedupWindow: this set's fingerprint seeds
		// are drawn per process on purpose (dedup.go), so no sidecar can hold it
		// and only the raw record can rebuild it.
		if at := time.UnixMilli(rec.RecordedAt); rec.RecordedAt > 0 &&
			now.Sub(at) < a.cfg.DedupWindow {
			a.seen.add(a.seen.fingerprint(rec.Type,
				dedupKey(rec.Type, rec.MigrationID, rec.Code, rec.RefusedAttempt)), at)
		}
	}
	if rec.Type != RecordMigration {
		return
	}
	if agg {
		// The lane counters are rebuilt from the ledger, so a restart does not
		// reset the flow the operator was reading.
		a.observeLaneLocked(rec.SourceSlot, rec.DestSlot, rec.ExitEdge, rec.RecordedAt)
		// And so is the species aggregate, ON THIS ONE PASS. It is the whole
		// reason the species tab can answer "how often has this species ever
		// crossed" without reading the ledger again: the replay the archive
		// already performs at startup is the only full scan there is, and every
		// later answer is a map lookup (species.go, rule 1).
		a.observeSpeciesLocked(rec)
	}
	if rec.Lineage == nil {
		return
	}
	migrantHash, speciesKey := migrantHashOf(rec)
	if agg {
		// THE COVERAGE DENOMINATOR, and the ONE half of the brain aggregate the
		// replay rebuilds. It costs a map lookup per record and no disk at all: a
		// crossing's genome hash is a fact the ledger already holds. The other
		// half — what was inside the genome — is NOT re-derived here, because
		// deriving it means reading and parsing every blob in the store; it comes
		// out of brains.jsonl (brainhist.go rule 3), and an hour this archive
		// holds no measurement for is a gap, never a zero.
		//
		// SINCE THE ROLL-UP SIDECAR THE DENOMINATOR IS PERSISTED TOO (rollup.go).
		// brainsave.go argued it should not be, correctly, because the full replay
		// re-derived it for free — and that argument ends the day the raw record
		// stops being replayed in full. A denominator nobody was keeping while a
		// segment was on the host cannot be computed for the period it covers
		// once the segment has gone.
		a.observeBrainSeen(rec.RecordedAt, migrantHash)
	}
	if !agg {
		return
	}
	for _, h := range hashesOf(rec) {
		// The CROSSING's own time, not the replay's: a horizon that measured from
		// the restart would re-queue a backlog it retired yesterday, and pay for
		// it in resident memory as well as in work (§23, B34).
		//
		// AN AGGREGATE LIKE THE REST OF THEM. What the sidecar covers is not
		// re-queued and not re-declined, so genomeGapsExpired counts each crossing
		// once for the life of the archive rather than once per replay — which is
		// the one number phase 1's validation could not make identical across a
		// restart, because it was a fact about the replay and not about the
		// record.
		a.trackLocked(h, speciesKey, rec.SourcePeer, rec.MigrationID, rec.EntityID,
			time.UnixMilli(rec.RecordedAt), now)
	}
}

// lineageHash is one wanted genome and WHOSE it is. The distinction costs a bool
// and buys the brain aggregate its honesty: see fetch.migrant.
type lineageHash struct {
	hash string
	// own is true for the migrant's own genome and false for a parent's.
	own bool
}

func hashesOf(rec Record) []lineageHash {
	if rec.Lineage == nil {
		return nil
	}
	out := make([]lineageHash, 0, 1+len(rec.Lineage.Parents))
	if rec.Lineage.GenomeHash != "" {
		out = append(out, lineageHash{hash: rec.Lineage.GenomeHash, own: true})
	}
	for _, p := range rec.Lineage.Parents {
		if p.GenomeHash != "" {
			out = append(out, lineageHash{hash: p.GenomeHash})
		}
	}
	return out
}

// migrantHashOf is the migrant's own genome hash and the species that carried
// it, which is the pair the brain aggregate folds. Both are empty when the record
// names neither.
func migrantHashOf(rec Record) (hash, speciesKey string) {
	if rec.Lineage == nil {
		return "", ""
	}
	hash = rec.Lineage.GenomeHash
	if rec.Species != nil {
		speciesKey = wire.SpeciesKey(rec.Species.GenericName, rec.Species.SpecificName)
	}
	return hash, speciesKey
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
		// Only when there is a page to put it on, and only ever off the request
		// path (release.go). A failure here changes nothing about the site.
		a.wg.Add(1)
		go func() { defer a.wg.Done(); a.releases.run(a.ctx) }()
	}
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.relayLoop() }()
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.tickLoop() }()
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.metricsLoop() }()
	// Nothing at all unless a horizon was configured (§23, B33). See eviction.go.
	a.startEviction()
	// Rotation, compression of what is closed, and retirement of what is past
	// the window AND confirmed off-host. See segments.go.
	a.startLedgerMaintenance()
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
	// The last save, so an orderly shutdown loses nothing at all. A hard kill
	// loses at most brainSaveInterval of arrivals, and loses them as a GAP in the
	// newest buckets rather than as a run of zeroes.
	if err := a.brainSave.Close(a.brainAgg); err != nil {
		a.log.Warn("archive: brain history close failed", "err", err)
	}
	// And the ledger fold's, so an orderly shutdown leaves a sidecar that covers
	// every record the ledger holds and the next start reads nothing but the
	// raw window it needs for the duplicate guard and the gap queue.
	if err := a.rollup.Close(a); err != nil {
		a.log.Warn("archive: roll-up state close failed", "err", err)
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
	// §3 Transport, §24 B35: the archive OFFERS permessage-deflate like every
	// other Contract B client. It is the subscriber that receives a COPY of every
	// forwarded frame plus every PEER_STATUS (§5.1), so it is the single
	// connection the relay spends the most bytes on. It is also the process this
	// project sizes its machine on, so the mode's fixed per-connection cost is
	// worth naming: ~1.25 MB for this one socket, against the deflate states
	// compress.go already pools for the status page.
	opts := wsutil.PeerDialOptions(&websocket.DialOptions{})
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
	// Sanitize before staging the delivery attempt. The temporary feed state and
	// the durable record must make the same decision about an untrusted species
	// block, including on a rerouted copy the durable ledger deduplicates below.
	species, stripped := wire.CarrySpecies(p.Species)
	receivedAt := time.Now().UnixMilli()
	a.observeHopAttempt(p, species, receivedAt)
	if a.markSeen(RecordMigration, p.MigrationID) {
		// A rerouted or (on an old peer) re-forwarded migration produces a second
		// copy. The durable record deduplicates on migrationId exactly as a
		// sidecar does (§5.1), but observeHopAttempt above must see every attempt:
		// its destination can change before one receiver acknowledges delivery.
		return true
	}
	lineage := p.Lineage
	// §15 B9's opacity rule reaches the archive too: SCHEMA VALIDATION ONLY. A
	// block that breaks the shape is stripped from the record with one log line —
	// recording a malformed one would put a fact in the ledger that no conformant
	// sidecar could have sent, and refusing to record the migration over a label
	// would be worse still.
	if stripped != "" {
		a.log.Warn("archive: stripping a malformed species block from a copied envelope; "+
			"the migration is still recorded",
			"migrationId", p.MigrationID, "sourcePeer", p.SourcePeer, "reason", stripped)
	}
	rec := Record{
		Type:        RecordMigration,
		RecordedAt:  receivedAt,
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
	off, rotated, err := a.ledger.AppendAt(rec)
	if err != nil {
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
	at := a.notePositionLocked(off, rotated)
	a.countRecordLocked(rec, at)
	a.observeLaneLocked(p.SourceSlot, p.DestSlot, p.ExitEdge, rec.RecordedAt)
	// The same envelope, kept another way for another question, and it costs one
	// map lookup. The lane counter answers HOW MANY OFFERS, the ACK-confirmed feed
	// later answers WHO ARRIVED, and the aggregate answers HOW OFTEN, EVER — the one question that
	// would otherwise need the whole ledger read back per poll. It folds the
	// record the archive has just written, so the live path and the startup
	// replay pass identical input to identical code.
	a.observeSpeciesLocked(rec)
	migrantHash, speciesKey := migrantHashOf(rec)
	for _, h := range hashesOf(rec) {
		a.trackLocked(h, speciesKey, p.SourcePeer, p.MigrationID, p.EntityID,
			time.UnixMilli(rec.RecordedAt), now)
	}
	// Whether the store ALREADY holds this crossing's genome, decided while the
	// lock is held and acted on after it is released. A hash already in the store
	// has no arrival left to fold at (brainhist.go rule 1), so this is the second
	// of the aggregate's two write points.
	heldAlready := migrantHash != "" && a.genomes.Has(migrantHash)
	a.mu.Unlock()

	// THE COVERAGE DENOMINATOR, folded off the lock: one more distinct genome the
	// record says crossed in this five minutes, whether or not this archive can
	// see inside it.
	a.observeBrainSeen(rec.RecordedAt, migrantHash)
	if heldAlready {
		// The parse is cached on the hash for the life of the process (brain.go
		// rule 1), so a genome that crosses two hundred times is read once — and
		// it is read OUTSIDE a.mu, because nothing that reads a file may hold the
		// lock the migration path takes (Risk 4).
		//
		// THIS ONE PATH DOES TOUCH AN MTIME, and brainhist.go rule 2 is emphatic
		// about mtimes, so the difference is worth stating. bb8.Store.Get
		// refreshes the blob's mtime, and eviction is ordered by it; what rule 2
		// refuses is a SWEEP, which would refresh all 702 000 blobs and postpone
		// the horizon for the whole store including everything long dead. This
		// refreshes exactly the blob of a genome that is crossing right now, which
		// is precisely what "last stored or last served" is for (bb8/evict.go).
		// It is bounded by first-sighting of a hash — about 1.4 reads a second at
		// the measured distinct-genome rate — and never by the size of the store.
		if br, ok := a.brainFor(migrantHash); ok {
			a.observeBrainHeld(rec.RecordedAt, speciesKey, migrantHash, br)
		}
	}
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
	// The lane's durable half (rollup.go). It is marked here rather than at the
	// call sites because this is the aggregate's ONE writer, which is what makes
	// the dirty set auditable at all.
	a.rollupDirty.lanes[key] = true
}

func (a *Archive) onAck(env wire.Envelope) bool {
	var ack contractb.MigrationAck
	if contractb.DecodeData(env.Data, &ack) != nil {
		return true
	}
	if a.markSeen(RecordAck, ack.MigrationID) {
		return true
	}
	recordedAt := time.Now().UnixMilli()
	// A MIGRATION_ACK is emitted only after the destination mod's
	// MIGRATE_IN_ACK. This is the first point at which the archive can honestly
	// show the creature entering that world.
	a.confirmHop(ack, recordedAt)
	// The record is built before the counters so BOTH see the same one: the
	// per-type and per-peer tallies of rollup.go are folded from the record
	// itself, exactly as the replay folds it, so the live path and the restart
	// pass identical input to identical code.
	rec := Record{
		Type:        RecordAck,
		RecordedAt:  recordedAt,
		MigrationID: ack.MigrationID,
		SourcePeer:  ack.SourcePeer,
		DestPeer:    ack.DestPeer,
		EntityID:    ack.EntityID,
		Duplicate:   ack.Duplicate,
	}
	a.appendAndCount(rec, ack.MigrationID)
	return true
}

func (a *Archive) onNack(env wire.Envelope) bool {
	var nack contractb.MigrationNack
	if contractb.DecodeData(env.Data, &nack) != nil {
		return true
	}
	// Do this even for a ledger-duplicate NACK. A later reroute can receive the
	// same code from another peer; peer matching in rejectHopAttempt ensures an
	// old rejection cannot erase the newer destination attempt.
	a.rejectHopAttempt(nack, time.Now().UnixMilli())
	if a.markSeen(RecordNack,
		dedupKey(RecordNack, nack.MigrationID, nack.Code, nack.RefusedAttempt)) {
		return true
	}
	rec := Record{
		Type:           RecordNack,
		RecordedAt:     time.Now().UnixMilli(),
		MigrationID:    nack.MigrationID,
		SourcePeer:     nack.SourcePeer,
		DestPeer:       nack.DestPeer,
		Code:           nack.Code,
		Message:        nack.Message,
		RefusedAttempt: nack.RefusedAttempt,
	}
	a.appendAndCount(rec, nack.MigrationID)
	return true
}

// dedupKey builds the §5.1 duplicate key for one record type. It is the single
// definition both the live path and the restart replay use, so the two cannot
// drift apart. A legacy MIGRATION_NACK dedups on migrationId + code. From
// contract-b/4.2, each field-present NOT_FORWARDED attempt is a distinct fact,
// so its destination and reroute count extend that key. Everything else dedups
// on migrationId alone.
func dedupKey(typ, migrationID, code string, attempt ...*contractb.MigrationAttempt) string {
	if typ == RecordNack {
		key := migrationID + "/" + code
		if code == contractb.NackNotForwarded && len(attempt) > 0 && attempt[0] != nil {
			key += fmt.Sprintf("/%d/%d", attempt[0].DestSlot, attempt[0].RerouteCount)
		}
		return key
	}
	return migrationID
}

// markSeen records one §5.1 duplicate key and reports whether the record was
// already in the set. AN EMPTY KEY IS NEVER SEEN: a record with no migrationId
// cannot be deduplicated at all, and answering "duplicate" for it would drop
// every one of them after the first.
func (a *Archive) markSeen(typ, key string) bool {
	if key == "" {
		return false
	}
	// Fingerprinted before the lock on purpose: the seeds are read-only for the
	// life of the process and the table is the only shared thing below.
	fp := a.seen.fingerprint(typ, key)
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.seen.add(fp, now) {
		return false
	}
	// A REFUSAL IS A REAL EVENT AND IT LEAVES NO OTHER TRACE. Nothing is
	// appended, nothing is logged per occurrence, and the record the archive
	// keeps is by construction the record without it — so without this counter
	// the guard's whole working is invisible. §25's B38 and decision 0006 both
	// rest on it: during the transition a non-zero value names a peer still
	// running a build that re-forwards, afterwards it is a defect report, and it
	// is the evidence that says whether the 48 h window may come down to an hour.
	//
	// ALL-TIME AND PERSISTED (rollup.go's floor line), because a counter that
	// resets at every restart cannot answer "has anything been refused since the
	// participant release".
	a.duplicatesRefused++
	a.rollupDirty.counts = true
	return true
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
func (a *Archive) trackLocked(h lineageHash, speciesKey, sourcePeer, migrationID string,
	entityID int32, crossedAt, now time.Time) {

	hash := h.hash
	if hash == "" || a.genomes.Has(hash) {
		return
	}
	if f, ok := a.pending[hash]; ok {
		// AN EXISTING ENTRY CAN STILL MOVE, and every move has to reach the
		// sidecar: the queue is durable now, so a field upgraded after its "gq"
		// line was written and never marked again would come back from the file
		// as the older answer. The whole-fold audit in rollup_test.go caught
		// exactly that, which is the reason both marks are here rather than only
		// at the creation site below.
		moved := false
		if f.sourcePeer == "" && sourcePeer != "" {
			f.sourcePeer, moved = sourcePeer, true
		}
		// A HASH NAMED AS A MIGRANT ANYWHERE IS A MIGRANT'S. The same genome can
		// be a parent's on one record and the migrant's own on another — a
		// creature whose parent also travelled — and the measurable one is the
		// answer that wins, because the other is not an answer at all.
		if h.own && !f.migrant {
			f.migrant, f.speciesKey, moved = true, speciesKey, true
		}
		if moved {
			a.markGapNewLocked(hash)
		}
		return
	}
	// Checked AFTER the pending lookup on purpose: a hash an old crossing shares
	// with a recent one is still wanted, and the recent crossing's entry keeps it.
	if a.gapPastHorizonLocked(crossedAt, now) {
		a.evict.gapsExpired++
		a.rollupDirty.counts = true
		return
	}
	f := &fetch{
		hash:        hash,
		sourcePeer:  sourcePeer,
		migrationID: migrationID,
		entityID:    entityID,
		firstSeen:   now,
		crossedAt:   crossedAt,
		nextAt:      now.Add(a.cfg.FirstAttemptDelay),
		migrant:     h.own,
	}
	if h.own {
		f.speciesKey = speciesKey
	}
	a.pending[hash] = f
	a.pendingOrder = append(a.pendingOrder, hash)
	// The queue's durable half (rollup.go). This is its ONE creation site, which
	// is what makes the dirty set auditable.
	a.markGapNewLocked(hash)
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
			// The brain aggregate's durable half, on the SAME timer and behind its
			// own interval: an append of what moved since the last one, which at
			// the measured arrival rate is the frontier bucket, whatever late
			// arrivals landed, and a handful of species records. It rides this
			// loop rather than a timer of its own because it is the same cadence
			// of bounded, yielding maintenance work the pump already is.
			if now.Sub(a.brainSavedAt) >= brainSaveInterval {
				a.brainSavedAt = now
				if err := a.brainSave.Save(a.brainAgg); err != nil {
					a.log.Warn("archive: brain history save failed", "err", err)
				}
			}
			// The LEDGER FOLD's durable half, on the same timer and behind its own
			// interval, and for the same reason: an append of what moved since the
			// last one, which at the measured rate is a handful of species, the
			// lanes that carried something, the counters and one floor line. What a
			// hard kill costs is thirty seconds of records, and the tail replay
			// reads exactly those back. See rollup.go.
			if now.Sub(a.rollupSavedAt) >= rollupSaveInterval {
				a.rollupSavedAt = now
				if err := a.rollup.Save(a); err != nil {
					a.log.Warn("archive: roll-up state save failed", "err", err)
				}
			}
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
			a.markGapGoneLocked(hash)
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
			// Back to nil rather than to an empty map: this hash may now wait
			// hours for its next turn, and an empty set and no set at all are
			// the same answer to every reader below.
			f.asked = nil
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
		if f.asked == nil {
			f.asked = map[string]bool{}
		}
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
	// The three facts the BRAIN AGGREGATE needs, copied out here — under the one
	// lock that guards a fetch entry — because the decision they drive comes
	// before the parse below. crossedAt is the recordedAt of the migration that
	// wanted this genome, which is the bucket key, and it has been durable since
	// the retention horizon needed it (§23, B34).
	var crossedAtMs int64
	var foldSpecies string
	foldSeries := false
	if f != nil {
		a.clearInFlightLocked(f)
		crossedAtMs = f.crossedAt.UnixMilli()
		foldSeries = f.migrant
		foldSpecies = f.speciesKey
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
	// THE BRAIN, MEASURED HERE AND NOWHERE ELSE, and the reason this is the fold
	// point is that the bytes are already in memory: no store read, no store
	// mutex, no mtime touched, and therefore no effect at all on the retention
	// horizon that orders eviction by mtime (brainhist.go rules 1 and 2). It runs
	// OUTSIDE a.mu because BrainStats parses JSON — 470 µs on the measured corpus
	// — and a parse under that lock is a parse the relay read loop waits behind.
	//
	// AND ONLY FOR A MIGRANT'S OWN GENOME. A parent's is measured for nothing (see
	// fetch.migrant), so parsing it would be work spent to be discarded — and
	// caching it would spend a slot of a bounded cache nothing ever reads.
	brain, brainOK := bb8.Brain{}, false
	if foldSeries {
		brain, brainOK = bb8.BrainStats(resp.Body.BB8)
	}
	rec := Record{
		Type:       RecordGenome,
		RecordedAt: now.UnixMilli(),
		GenomeHash: resp.GenomeHash,
		ServedBy:   resp.SourcePeer,
	}
	a.mu.Lock()
	delete(a.pending, resp.GenomeHash)
	// The gap is closed, so its "gq" line gets its tombstone (rollup.go). A queue
	// entry that outlived the blob it was waiting for would be re-asked at every
	// restart for as long as the horizon allows.
	a.markGapGoneLocked(resp.GenomeHash)
	a.mu.Unlock()
	if brainOK {
		// THE PER-HASH PARSE CACHE IS FILLED, not merely consulted, and that
		// closes brain.go's named lag: a miss cached while this blob was still
		// outstanding would otherwise keep the genealogy on an older genome of the
		// species until a LATER hash of it was first seen present. The bytes have
		// just been read and hashed; there is nothing to re-read.
		a.fillBrainCache(resp.GenomeHash, brain)
		a.observeBrainHeld(crossedAtMs, foldSpecies, resp.GenomeHash, brain)
	}
	// A GENOME line is a ledger record and is counted like one. It was not until
	// 2026-08-12, and the omission made `ledgerRecords` drift BELOW `wc -l` from
	// the first fetch after every boot — 8,060,891 against 8,156,869 lines on the
	// deployment, 1.2% low — because the startup replay counts these lines and
	// the live path did not. The counter and the file now answer the same
	// question, and `ledgerSkipped` is once again the whole of the difference.
	a.appendAndCount(rec, resp.GenomeHash)
	a.log.Info("archive: stored a genome fetched by hash",
		"genomeHash", resp.GenomeHash, "servedBy", resp.SourcePeer)
	return true
}

// appendAndCount is the live path's WRITE FIRST, FOLD SECOND for the three
// records that carry no other fold: an ACK, a NACK and a GENOME line.
//
// THE ORDER IS THE WHOLE POINT. The roll-up cursor is a position now (rollup.go),
// and a position is only exact if the fold that produced it and the write that
// placed it move under one lock. Folding first would let a save land between the
// two and write a cursor covering a record the file does not hold yet, and the
// next tail replay would fold that record a second time — a doubled counter,
// which is the one failure this design cannot tolerate because it looks like
// data.
//
// A FAILED APPEND IS NOT COUNTED, and that is a change: the error used to be
// discarded, so a record the disk refused still moved ledgerRecords and the
// counter disagreed with the file forever afterwards. The crossing is still
// recorded by the MIGRATION line that has already landed; what is lost is the
// ACK or the GENOME line, and it is said out loud.
func (a *Archive) appendAndCount(rec Record, id string) {
	off, rotated, err := a.ledger.AppendAt(rec)
	if err != nil {
		a.log.Error("archive: ledger append failed", "type", rec.Type, "id", id, "err", err)
		return
	}
	a.mu.Lock()
	at := a.notePositionLocked(off, rotated)
	a.countRecordLocked(rec, at)
	a.mu.Unlock()
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
