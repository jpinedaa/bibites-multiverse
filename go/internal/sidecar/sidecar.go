// Package sidecar implements multiverse-sidecar: the Contract A server for the
// mod, the Contract B client for the relay, and the durable custody chain of
// decision D2 that joins them.
//
// M4 adds four things and nothing else. The map is two-dimensional, so the
// sidecar runs ONE LANE PER DECLARED EXPORT EDGE and tells its mod one
// EDGE_STATUS entry for each (contract-a.md §15, A18) — FOUR of each under D17,
// where every declared edge is both an export edge and an entry edge and an
// `open: false` edge refuses MIGRATE_OUT through it while never refusing
// MIGRATE_IN on it (§18, A38, §5.4). Inbound delivery is
// PACED out of the journal in simulated minutes, because a dam released at wake
// is what T1 measured (A20, raised to 12.0 and given a knob by A40). An
// outbound entry carries a durable HANDOFF STATE,
// which is what lets a journaled hop be re-routed ONLY under a proof of
// non-delivery (contract-b-m4.md §9.2). And an entry whose destination went
// silent with no such proof is RECORDED LOST: since §25's B37 the frame is
// forwarded once, never re-forwarded, and never brought home on a timeout.
// MIGRATION IS AT-MOST-ONCE WITH NO EXCEPTION, and the price is stated where a
// participant reads it — a forwarded organism that never spawns is gone (§9.3).
//
// Under M3 the sidecar already owned the lineage annex (D11): it hashes the
// migrant's genome and every parent blob the mod ships, caches them by hash,
// strips the blobs from the wire, and answers a later GENOME_REQUEST out of that
// cache. D4 stays intact — the mod never parses a genome, and the sidecar never
// trusts a mod-supplied hash.
package sidecar

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"multiverse/internal/bb8"
	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/mapwalk"
	"multiverse/internal/modtoken"
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

// Sidecar is one peer node's network brain.
type Sidecar struct {
	cfg     Config
	log     *slog.Logger
	jr      *journal.Journal
	genomes *bb8.Store

	ln      net.Listener
	httpSrv *http.Server

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu sync.Mutex

	// Contract A state.
	mod       *modSession
	modGen    uint64
	edgeEpoch int64
	lastEdges []contracta.EdgeState
	// contractAAuthFailures counts CONSECUTIVE refused upgrades on this wire
	// (contract-a.md §21, A47). The mod owns the backoff ladder and its own
	// ceiling; this is the sidecar's half of the same fact, so a person reading
	// the sidecar log can tell a wrong token from a mod that never dialled.
	contractAAuthFailures int
	// admission owns the pre-custody population gate. admittedSinceHeartbeat
	// covers successful MIGRATE_IN_ACKs that have left the pending journal but
	// are not necessarily included in the mod's next population heartbeat yet.
	admission              admissionController
	admittedSinceHeartbeat int
	admissionSaveMu        sync.Mutex

	// Contract B state: the map. A sidecar needs its own slot, its position, and
	// ONE EFFECTIVE NEIGHBOUR PER EXPORT EDGE (D8, D12, D13, §6.4).
	relayConn  *wsutil.Conn
	relayReady bool
	// relaySessionID is the scope of every non-delivery proof this connection
	// can produce (§5.2). A link flap keeps it; a relay restart changes it, and
	// every outstanding proof goes with it.
	relaySessionID string
	slot           int
	position       contractb.Position
	mapShape       contractb.MapShape
	slotCount      int
	neighbours     map[string]*contractb.Neighbour
	// status is the last PEER_STATUS. It exists for two jobs: deciding whether a
	// journaled entry's recorded destSlot has a live peer right now (§9.2, §9.3),
	// and reproducing §8's walk for an axis the grant carries no key for, which
	// is the only way to name WHY a closed edge is closed.
	status    contractb.PeerStatus
	peerEpoch int64
	// sendPace is B24's client half: this connection's own outbound rate, taken
	// as a fraction of the ceiling the relay published on HANDSHAKE_ACK
	// (contract-b-m4.md §3.3, §6.2, §22 B24). It is reset on every session.
	sendPace sendPace
	// capacityShedTotal counts every close 4007 this process has taken. It is
	// monotonic and it is the measure the rejoin-burst finding is read against:
	// a drain that spends frames and sheds nothing leaves it at zero.
	capacityShedTotal int
	// The observation record of observe.go: what a diagnostic reads instead of
	// dialling the relay a second time (docs/sidecar-diagnose-spec.md §1).
	relayFault       *relayFault
	relayConnectedAt time.Time
	lastGrant        *grantRecord
	// publishedLimits and minContractVersion are the relay's own configuration
	// as it published it, kept verbatim. Absence is UNKNOWN and never "no
	// ceiling" (contract-b-m4.md §6.2).
	publishedLimits    map[string]int64
	minContractVersion string
	sent               sentMeter
	claims             claimMeter
	achieved           achievedRate
	// startedAt is this process's own start, reported beside its pid so a stale
	// process record can be told from a live one.
	startedAt time.Time

	// Custody scheduling, in memory. The durable half lives in the journal.
	sched        map[string]*sched
	seenSessions map[string]bool
	lastPurge    time.Time
	lastSweep    time.Time
	lastCompact  time.Time
	// pace is the delivery rate limit of contract-a.md §7.5.
	pace pacer
	// lostForwardTotal is monotonic and reset only by losing the journal: the
	// forwards this sidecar committed to one relay enqueue and never heard an answer to
	// (§6.3.1, §9.3). lateAckTotal is the half that says whether
	// forwardTimeoutMs is set too short — an answer that arrived after the entry
	// was already recorded lost. It is process-local and off the wire.
	lostForwardTotal int
	lateAckTotal     int
	// receiptsRecorded counts the FORWARD_RECEIPTs this process has journaled
	// (contract-b-m4.md §6.12, §22 B26). It is process-local and deliberately NOT
	// on the stats block: §6.3.1 is a published shape and B26 added no field to
	// it. It exists for this sidecar's own log line and for the cost harness.
	receiptsRecorded int64
	// genomeServed counts GENOME_REQUESTs answered per requester in the current
	// minute (contract-b-m4.md §10's rate limit, answering side).
	genomeServed map[string]*rateWindow
	// Test-only interleaving hooks. Production leaves them nil. They let a
	// regression stop at the custody boundaries that must be deterministic:
	// after Create returns a clone, after the handler's immediate custody tick,
	// and after frame preparation but before the durable sent transition.
	afterOutboundCreate        func()
	afterOutboundImmediateTick func()
	afterForwardPrepare        func()
	closed                     bool
}

type sched struct {
	// nextForward paces the offer of an entry that has NOT reached a live relay
	// connection yet, and the retry of an upstream MIGRATION_ACK. The ACK retry
	// runs on TickInterval because the shared wire pacer now owns its rate. It
	// never paces a re-forward, because there is no re-forward (§25, B37).
	nextForward time.Time
	bounceAt    time.Time
	nextDeliver time.Time
}

type rateWindow struct {
	windowStart time.Time
	count       int
}

// New builds a sidecar and opens its journal, replaying whatever custody the
// previous process left behind (D2).
func New(cfg Config) (*Sidecar, error) {
	cfg.applyDefaults()
	if err := validateAdmissionConfig(cfg); err != nil {
		return nil, err
	}
	// §33 B49's bounds, applied where the participant's two strings enter this
	// process and nowhere else. A bad one is clipped or dropped and never
	// refuses a start: a typo in a display name must not keep a world off the
	// map, and a dropped value simply publishes nothing.
	sanitizePublicNames(&cfg)
	if cfg.Fault == "" {
		cfg.Fault = os.Getenv("MULTIVERSE_FAULT")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, err
	}
	// contract-a.md §21 A47: the sidecar MINTS THE TOKEN AT FIRST START when the
	// file does not exist, mode 0600, and both processes then read the same path
	// on the same machine (D9). It happens here, before the listener binds, so
	// the file exists before any mod can dial — which is the ordering A52's
	// migration note asks a rollout to keep.
	if cfg.ContractAToken == "" && !cfg.InsecureNoContractAToken {
		tok, err := modtoken.EnsureFile(cfg.ContractATokenFile)
		if err != nil {
			return nil, fmt.Errorf("sidecar: contract A token: %w", err)
		}
		cfg.ContractAToken = tok
	}
	jr, err := journal.Open(filepath.Join(cfg.DataDir, "journal"))
	if err != nil {
		return nil, fmt.Errorf("sidecar: open journal: %w", err)
	}
	genomes, err := bb8.OpenStore(filepath.Join(cfg.DataDir, "genomes"))
	if err != nil {
		return nil, fmt.Errorf("sidecar: open genome cache: %w", err)
	}
	s := &Sidecar{
		cfg:          cfg,
		jr:           jr,
		genomes:      genomes,
		neighbours:   map[string]*contractb.Neighbour{},
		sched:        map[string]*sched{},
		seenSessions: map[string]bool{},
		genomeServed: map[string]*rateWindow{},
	}
	// journal.Open has just compacted, so the periodic compaction's clock
	// starts here rather than at the epoch.
	s.lastCompact = cfg.Clock()
	s.startedAt = time.Now()
	s.sent = newSentMeter()
	s.pace = newPacer(cfg.InboundRatePerSimMinute, cfg.InboundRateBurst)
	s.admission = newAdmissionController(cfg)
	// §7.4: peerId is persisted outside the journal. Losing it makes the peer a
	// stranger that takes a second slot and strands its old one — which is why
	// §7.5 gives the operator a release and a handover command.
	s.cfg.PeerID = s.resolvePeerID(cfg.PeerID)
	s.log = s.cfg.Logger.With("peer", s.cfg.PeerID)
	s.loadAdmissionState()
	if s.cfg.PreferredSlot == 0 {
		s.cfg.PreferredSlot = s.readSlot()
	}
	if s.cfg.PreferredPosition == nil {
		s.cfg.PreferredPosition = s.readPosition()
	}
	if lost := jr.Discarded(); lost > 0 {
		// A journal damaged before this process opened it. On 2026-08-08 this
		// was eight hours of custody, discarded in silence by five sidecars at
		// once, because a full disk had left a half-written record in the middle
		// of each log. It is an error, not a warning: history that D2 promised
		// is durable has been lost, and only an operator can judge what it held.
		s.log.Error("sidecar: the journal was damaged and replay stopped early; "+
			"custody history after the torn record is GONE",
			"discardedBytes", lost, "journal", filepath.Join(cfg.DataDir, "journal"))
	}
	pendingOut := jr.CountPending(journal.Out)
	pendingIn := jr.CountPending(journal.In)
	if pendingOut+pendingIn > 0 {
		s.log.Warn("sidecar: recovered custody from the journal",
			"outbound", pendingOut, "inbound", pendingIn)
	}
	// §9.3: an entry that was already forwarded keeps the deadline it was
	// forwarded under, and the restart neither restarts it nor resolves it. Say
	// so out loud: these are organisms whose fate this sidecar cannot learn, and
	// a `held` entry left by a pre-§25 sidecar lands here too — its journal
	// record replays as `sent`, which is what it always meant.
	for _, st := range jr.List() {
		if st.Direction == journal.Out && st.Handoff == journal.HandoffSent &&
			(st.Status == journal.StatusOpen || st.Status == journal.StatusInFlight) {
			s.log.Warn("sidecar: an already-committed organism is unresolved and will not be re-sent",
				"migrationId", st.Entry.MigrationID, "destSlot", st.Entry.DestSlot,
				"sentAt", time.UnixMilli(st.SentAtMs).UTC(),
				"forwardTimeout", s.cfg.ForwardTimeout)
		}
	}
	return s, nil
}

// Start binds the Contract A listener, dials the relay, and starts the custody
// scheduler. It returns once the listener is bound.
func (s *Sidecar) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	if err := checkLoopback(s.cfg.Listen); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		return fmt.Errorf("sidecar: listen on %s: %w", s.cfg.Listen, err)
	}
	s.ln = ln
	mux := http.NewServeMux()
	mux.HandleFunc(contracta.ContractAPath, s.serveContractA)
	// contract-a.md §15 A23: the sidecar MUST keep serving /contract-a/v1 and
	// MUST close every connection on it immediately with 4000. A bare HTTP 404
	// would be a socket error in a BepInEx log and half an evening of diagnosis.
	mux.HandleFunc(contracta.RetiredContractAPath, s.serveRetiredContractA)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	// WP7's own-slot view (ownslot.go). Read-only, loopback, and unauthenticated
	// for the reasons written there.
	mux.HandleFunc(OwnSlotPath, s.serveOwnSlot)
	s.httpSrv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	// Publish the resolved address so a caller that asked for port 0 can find
	// it without parsing logs.
	_ = os.WriteFile(filepath.Join(s.cfg.DataDir, "listen.addr"), []byte(ln.Addr().String()+"\n"), 0o644)
	// And the process record `--diagnose`'s stale-process check reads. It is
	// written after the listener binds, so a record that exists names a process
	// that got as far as serving (ownslot.go).
	s.writeProcessRecord(ln.Addr().String())

	s.wg.Add(1)
	go func() { defer s.wg.Done(); _ = s.httpSrv.Serve(ln) }()
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.relayLoop() }()
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.tickLoop() }()

	// The token file's PATH is logged and its value never is (contract-a.md §21,
	// A47, §11.1). The path is what a person needs: A47's whole failure mode is
	// two processes pointed at two different files, and the remedy names a path.
	contractAAuth := s.cfg.ContractATokenFile
	if s.cfg.InsecureNoContractAToken {
		contractAAuth = "DISABLED by --insecure-no-contract-a-token"
	}
	s.log.Info("sidecar: listening", "addr", ln.Addr().String(), "path", contracta.ContractAPath,
		"relay", s.cfg.RelayURL, "dataDir", s.cfg.DataDir, "preferredSlot", s.cfg.PreferredSlot,
		"preferredPosition", s.cfg.PreferredPosition,
		"contractATokenFile", contractAAuth,
		"relayCredential", credentialState(s.cfg.Secret))
	return nil
}

// Addr is the resolved Contract A address.
func (s *Sidecar) Addr() string {
	if s.ln == nil {
		return s.cfg.Listen
	}
	return s.ln.Addr().String()
}

// URL is the Contract A WebSocket URL a mod connects to.
func (s *Sidecar) URL() string { return "ws://" + s.Addr() + contracta.ContractAPath }

// PeerID is this sidecar's stable identity.
func (s *Sidecar) PeerID() string { return s.cfg.PeerID }

// RelayURL is the Contract B endpoint this sidecar dials, and it never changes
// while the process runs. That is a RULE and not an implementation detail
// (contract-b-m4.md §3.1, §22 B23): a sidecar whose credential is refused, or
// whose relay presents a certificate it cannot verify, MUST NOT fall back to
// ws://, MUST NOT try another port, and MUST NOT go looking for a relay that
// will take it. It keeps dialling exactly what it was given and waits for a
// person.
func (s *Sidecar) RelayURL() string { return s.cfg.RelayURL }

// CustodySnapshot returns a copy of every journal state, in journal order.
func (s *Sidecar) CustodySnapshot() []*journal.State { return s.jr.List() }

// Slot is the slot the relay granted, or 0 when none is held.
func (s *Sidecar) Slot() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.slot
}

// Position is this peer's coordinate in the map.
func (s *Sidecar) Position() contractb.Position {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.position
}

// MapShape is the rectangle as of the last grant or status.
func (s *Sidecar) MapShape() contractb.MapShape {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mapShape
}

// Neighbour is the effective neighbour on one export edge, or nil when that
// axis has no deliverable target and the edge is therefore closed.
func (s *Sidecar) Neighbour(edge string) *contractb.Neighbour {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.neighbours[edge]
	if !ok || n == nil {
		return nil
	}
	c := *n
	return &c
}

// RelayConnected reports whether the Contract B link is up. §9.3's second
// condition turns on it: a sender with no link is BLIND, and a blind sender
// never accrues hold time.
func (s *Sidecar) RelayConnected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.relayReady
}

// RelaySessionID is the relay session this link is under (§5.2). It changes
// when the relay process changes, and with it every proof it could give.
func (s *Sidecar) RelaySessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.relaySessionID
}

// ReceiptsRecorded is how many FORWARD_RECEIPTs this process has journaled
// (§6.12, §22 B26). It is the sender's own count of forwards the relay
// acknowledged, and it is a measurement rather than a state: nothing routes,
// refuses, holds or re-routes on it.
func (s *Sidecar) ReceiptsRecorded() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.receiptsRecorded
}

// PacedFramesPerSecond is the outbound rate this sidecar is holding itself to
// on the current session, and 0 when the relay published no ceiling to hold
// itself to (contract-b-m4.md §3.3, §6.2, §22 B24). It is a FRACTION of the
// published maxFramesPerSecond, never a compiled rate, so an operator who moves
// the relay's knob moves this with it.
func (s *Sidecar) PacedFramesPerSecond() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendPace.framesPerSecond()
}

// PacedDeferrals is how many journal-backed frames the pacer has held back on
// the current session. Rising with no capacity shed is the fix working; see
// CapacitySheds.
func (s *Sidecar) PacedDeferrals() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendPace.deferred
}

// CapacitySheds is how many times this process has been closed with 4007 for a
// published capacity limit (§3.2, §3.3).
func (s *Sidecar) CapacitySheds() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.capacityShedTotal
}

// Stats is the peer stats block of §6.3.1 as this sidecar would send it.
func (s *Sidecar) Stats() contractb.PeerStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statsLocked()
}

// Genomes is the on-disk genome cache, for tests and operator tools.
func (s *Sidecar) Genomes() *bb8.Store { return s.genomes }

// Close drains and stops the sidecar. In-flight custody stays in the journal.
func (s *Sidecar) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	mod := s.mod
	relay := s.relayConn
	s.mu.Unlock()

	if mod != nil {
		mod.conn.Close(contracta.CloseShuttingDown, "sidecar draining")
	}
	if relay != nil {
		relay.Close(contractb.CloseShuttingDown, "sidecar draining")
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = s.httpSrv.Shutdown(ctx)
		cancel()
	}
	s.wg.Wait()
	// A CLEAN shutdown removes its own process record. What is left behind after
	// a kill is exactly the stale record `--diagnose` warns about, and leaving it
	// is the honest outcome: pid numbers are reused, so a record nobody removed
	// is a record nobody should trust.
	s.removeProcessRecord()
	return s.jr.Close()
}

func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("sidecar: --listen %q is not host:port: %w", addr, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "*" {
		// contract-a.md §2 and §14 A17: the sidecar MUST NOT bind 0.0.0.0.
		return fmt.Errorf("sidecar: --listen %q binds a wildcard address; contract A is loopback only", addr)
	}
	return nil
}

func (s *Sidecar) now() time.Time { return s.cfg.Clock() }

// ---------------------------------------------------------------- scheduling

func (s *Sidecar) tickLoop() {
	t := time.NewTicker(s.cfg.TickInterval)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			s.tick(s.now())
		}
	}
}

func (s *Sidecar) tick(now time.Time) {
	s.mu.Lock()
	states := s.jr.List()
	// A completed inbound entry is a durable MIGRATION_ACK waiting to release
	// the sender's custody. Drain those answers before new outbound payloads use
	// this tick's shared wire budget. tickInbound marks AckedUpstream only after
	// the writer accepts the frame, so a deferral stays safe and retryable.
	for _, st := range states {
		if st.Direction == journal.In && st.Status == journal.StatusDone {
			s.tickInbound(st, now)
		}
	}
	// A retry time can land a few microseconds after this tick. Do not let an
	// outbound payload spend the newly refilled token in that gap. The durable
	// ACK queue owns the deferred budget until every eligible reply has left.
	pendingAck := s.hasPendingAckLocked()
	for _, st := range states {
		if st.Direction == journal.In && st.Status == journal.StatusDone {
			continue
		}
		switch st.Direction {
		case journal.Out:
			if pendingAck {
				continue
			}
			s.tickOutbound(st, now)
		case journal.In:
			s.tickInbound(st, now)
		}
	}

	if now.Sub(s.lastPurge) > time.Minute {
		s.lastPurge = now
		if n, err := s.jr.PurgeExpired(s.cfg.ExportRetention, now); err != nil {
			s.log.Warn("sidecar: tombstone purge failed", "err", err)
		} else if n > 0 {
			s.log.Info("sidecar: purged expired tombstones", "count", n)
		}
	}
	sweep := now.Sub(s.lastSweep) > time.Minute
	if sweep {
		s.lastSweep = now
	}
	// The first compaction is scheduled a full interval after startup, because
	// Open has just compacted: s.lastCompact is set at construction.
	compact := now.Sub(s.lastCompact) >= s.cfg.JournalCompactInterval
	if compact {
		s.lastCompact = now
	}
	s.mu.Unlock()

	if sweep {
		// contract-b-m4.md §10: the cache expires by genomeCacheRetentionDays,
		// least recently served, bounded by genomeCacheMaxBytes.
		if n, err := s.genomes.Sweep(s.cfg.GenomeCacheRetention, s.cfg.GenomeCacheMaxBytes); err != nil {
			s.log.Warn("sidecar: genome cache sweep failed", "err", err)
		} else if n > 0 {
			s.log.Info("sidecar: evicted genomes from the cache", "count", n)
		}
	}

	if compact {
		// Outside s.mu on purpose. The journal takes its own lock and the
		// rewrite is short, but the custody scheduler has no reason to wait on
		// a disk write, and holding both locks across one would invent a stall
		// where none is needed (contract-b-m4.md §12, journalCompactMinutes).
		before, after, err := s.jr.Compact()
		if err != nil {
			s.log.Error("sidecar: journal compaction failed", "err", err,
				"bytes", before)
		} else if before != after {
			s.log.Info("sidecar: compacted the journal", "live", s.jr.Live(),
				"bytesBefore", before, "bytesAfter", after, "reclaimed", before-after)
		}
	}
}

// hasPendingAckLocked reports whether the deferred send budget belongs to a
// durable ACK. Every outbound entry point uses this gate, including a fresh
// Contract A export that arrives between custody ticks. Sidecar.mu makes the
// check and the following send decision one scheduler action.
func (s *Sidecar) hasPendingAckLocked() bool {
	for _, st := range s.jr.List() {
		if st.AwaitsUpstreamAck() {
			return true
		}
	}
	return false
}

// tickOutbound is §9.2's state machine, in one place.
//
// SINCE §25's B37 IT HAS ONE FEWER BRANCH AND ONE FEWER CLOCK. An entry that
// reached a live relay connection is `sent`, and this function does exactly one
// thing with it: it waits for an answer until forwardTimeoutMs, then records the
// organism LOST. It does not re-forward it, does not accrue anything against it,
// and does not bring it home. The branches that still act — pending and refused
// — are the ones where NO CUSTODY CAN HAVE MOVED, so what they do cannot
// duplicate an organism.
func (s *Sidecar) tickOutbound(st *journal.State, now time.Time) {
	// Journal Create/List/Get return snapshots. A caller can release Sidecar.mu
	// after taking one, and a custody tick can durably advance the real entry
	// before that caller resumes. Always decide from the current journal state
	// while the caller holds Sidecar.mu; a stale pending clone must never commit
	// a second send after the scheduler already committed the first.
	current, ok := s.jr.Get(st.Entry.MigrationID)
	if !ok {
		return
	}
	st = current
	if st.Status != journal.StatusOpen && st.Status != journal.StatusInFlight {
		return
	}
	sc := s.schedFor(st.Entry.MigrationID)

	switch st.Handoff {
	case journal.HandoffSent, "":
		// CUSTODY MAY HAVE MOVED, unknowably, and nothing this sidecar can do
		// will settle it. Silence is not proof (§9.2), so the entry is never
		// re-routed on its own account; and a second copy of the frame is not
		// free, so it is never sent (§25, B37). Only an answer resolves this
		// entry — MIGRATION_ACK to `done`, a proving MIGRATION_NACK to `refused`
		// — or the deadline resolves it to `lost`.
		if now.Sub(s.sentAt(st)) >= s.cfg.ForwardTimeout {
			s.loseLocked(st, now)
		}

	case journal.HandoffPending:
		// A never-sent or a refused entry. NO CUSTODY HAS MOVED — the frame
		// reached nobody, or the receiver said in as many words that it took
		// none — so offering it to a destination is not a second copy of
		// anything and cannot duplicate an organism.
		if s.provenRefusalDeadlineExpiredLocked(st, now) {
			return
		}
		if s.destLiveLocked(st.Entry.DestSlot) {
			sc.bounceAt = time.Time{}
			if now.Before(sc.nextForward) {
				return
			}
			if s.forwardLocked(st, now) {
				sc.nextForward = now.Add(s.cfg.ForwardRetry)
			}
			return
		}
		// An exact relay NOT_FORWARDED chain keeps walking forward from the
		// selected destination. Returning to the source's current effective
		// neighbour can select a transport queue that this migration already
		// tried. The durable deadline and refused set remain in force across this
		// never-sent alternate.
		if st.RefusalDeadlineMs != 0 {
			if s.rerouteLocked(st, contractb.ProofNeverSent, now) {
				sc.bounceAt = time.Time{}
				sc.nextForward = time.Time{}
			}
			return
		}
		// No custody can have moved, so the entry may be re-routed along the
		// same axis to the current effective neighbour (§9.2).
		if s.rerouteLocked(st, contractb.ProofNeverSent, now) {
			sc.bounceAt = time.Time{}
			sc.nextForward = time.Time{}
			return
		}
		// It needs a lane. With no effective neighbour on that axis there is
		// nowhere to re-route to: the entry waits, and bounces home after
		// bounceTimeoutMs — M3's rule, unchanged, and now the only automatic
		// bounce there is.
		if sc.bounceAt.IsZero() {
			sc.bounceAt = now.Add(s.cfg.BounceTimeout)
			return
		}
		if now.After(sc.bounceAt) {
			s.bounceLocked(st, "no deliverable slot on the "+st.Entry.Edge+
				" axis within bounceTimeoutMs, and no custody was ever taken")
		}

	case journal.HandoffRefused:
		proof := st.RerouteProof
		if proof == "" {
			proof = contractb.ProofPeerRefused
		}
		if st.RefusalDeadlineMs != 0 {
			// This is a bounded transport-refusal chain, even if a later
			// destination supplied a peer-local proof. Never retry a destination
			// from the durable tried set, and never start a new deadline.
			if s.rerouteLocked(st, proof, now) {
				sc.bounceAt = time.Time{}
				sc.nextForward = time.Time{}
			}
			return
		}
		if proof != contractb.ProofPeerRefused {
			// A relay-generated NOT_FORWARDED says its bounded destination
			// transport queue did not accept the frame. It is not the live
			// destination refusing admission. Preserve the original behavior:
			// retry that destination while it stays live, and use the ordinary
			// effective-neighbour reroute only if it goes dark. In particular,
			// do not start the no-lane bounce clock against a live queue that is
			// already draining.
			if s.destLiveLocked(st.Entry.DestSlot) {
				sc.bounceAt = time.Time{}
				if now.Before(sc.nextForward) {
					return
				}
				if s.forwardLocked(st, now) {
					sc.nextForward = now.Add(s.cfg.ForwardRetry)
				}
				return
			}
			if s.rerouteLocked(st, proof, now) {
				sc.bounceAt = time.Time{}
				sc.nextForward = time.Time{}
				return
			}
			if sc.bounceAt.IsZero() {
				sc.bounceAt = now.Add(s.cfg.BounceTimeout)
				return
			}
			if now.After(sc.bounceAt) {
				s.bounceLocked(st, "no deliverable slot on the "+st.Entry.Edge+
					" axis within bounceTimeoutMs, and the relay proved custody never moved")
			}
			return
		}
		// A live receiver's NACK proves that custody did not move, but it also
		// proves that sending straight back to that same live receiver is not a
		// route. Continue the axis walk after it and exclude every world that has
		// already refused this migration.
		if s.rerouteLocked(st, proof, now) {
			sc.bounceAt = time.Time{}
			sc.nextForward = time.Time{}
			return
		}
		// If every other world is unavailable or has refused, periodically try
		// the current destination again: a population refusal is transient. The
		// original bounce deadline remains bounded and is not reset by retries.
		if sc.bounceAt.IsZero() {
			sc.bounceAt = now.Add(s.cfg.BounceTimeout)
		}
		if now.After(sc.bounceAt) {
			s.bounceLocked(st, "every deliverable slot on the "+st.Entry.Edge+
				" axis refused or was unavailable within bounceTimeoutMs")
			return
		}
		if s.destLiveLocked(st.Entry.DestSlot) && !now.Before(sc.nextForward) {
			if s.forwardLocked(st, now) {
				sc.nextForward = now.Add(s.cfg.ForwardRetry)
			}
		}
	}
}

// sentAt is when the forward-resolution deadline of §9.3 started running for
// this entry. SentAtMs is written at the durable commitment to the current socket enqueue;
// a journal an older sidecar left behind has no such field, so the entry's own
// journaling time stands in. It is earlier than the forward it stands for, which
// is the safe direction for a deadline that only ever closes a record.
func (s *Sidecar) sentAt(st *journal.State) time.Time {
	if st.SentAtMs > 0 {
		return time.UnixMilli(st.SentAtMs)
	}
	if st.Entry.JournaledAt > 0 {
		return time.UnixMilli(st.Entry.JournaledAt)
	}
	return s.now()
}

// loseLocked is §9.3's whole terminal action, and it is bookkeeping. The
// organism was committed to one relay enqueue and no answer ever came; this sidecar
// cannot tell a delivery whose acknowledgement was lost from a delivery that
// never happened, and it will not guess in either direction. The entry becomes a
// tombstone in the `lost` state so a late MIGRATION_ACK is still recognised, and
// the loss is counted where the map can read it (§6.3.1, lostForwardTotal).
//
// NOTHING IS RE-SENT AND NOTHING COMES HOME. That is the whole of B37: an
// automatic bounce here would be the duplication at-most-once refuses, and a
// re-forward would be the retry that made the bounce necessary.
func (s *Sidecar) loseLocked(st *journal.State, now time.Time) {
	id := st.Entry.MigrationID
	completed := now.UnixMilli()
	if _, err := s.jr.Apply(id, journal.Update{
		Status: journal.StatusDone, CompletedAt: &completed, Handoff: journal.HandoffLost,
		Note: fmt.Sprintf("lost: one relay enqueue committed for slot %d and never answered within forwardTimeoutMs (%s)",
			st.Entry.DestSlot, s.cfg.ForwardTimeout)}); err != nil {
		s.log.Error("sidecar: lost-forward journal update failed", "migrationId", id, "err", err)
		return
	}
	delete(s.sched, id)
	s.lostForwardTotal++
	// A LOSS IS A FACT THE OPERATOR READS, not a silent repair — the same rule
	// the timeout bounce carried, applied to the outcome that replaced it.
	s.log.Error("sidecar: FORWARD LOST — an organism was committed to one relay enqueue and never answered",
		"migrationId", id, "entityId", st.Entry.EntityID, "destSlot", st.Entry.DestSlot,
		"exitEdge", st.Entry.Edge, "sentAt", s.sentAt(st).UTC(),
		"forwardTimeout", s.cfg.ForwardTimeout, "forwardReceipts", st.ForwardReceipts,
		"lostForwardTotal", s.lostForwardTotal,
		"meaning", "migration is at-most-once and this is what that costs: the organism was "+
			"not re-sent and was not brought home (contract-b-m4.md §9.3, §25 B37)")
}

// destLiveLocked answers, from the latest PEER_STATUS, whether the destination
// slot is live. A slot ABSENT FROM THE MAP counts as dark, and so does a slot
// this sidecar has no status for yet. It gates the OFFER of an entry no peer has
// taken custody of (§9.2), and nothing else: it no longer gates a re-forward,
// because there is no re-forward (§25, B37).
func (s *Sidecar) destLiveLocked(slot int) bool {
	if !s.relayReady || s.slot == 0 || slot == 0 {
		return false
	}
	si, ok := mapwalk.Find(s.status, slot)
	if !ok || !si.Live {
		return false
	}
	return true
}

func (s *Sidecar) setHandoffLocked(st *journal.State, h journal.Handoff, note string) {
	if st.Handoff == h {
		return
	}
	u := journal.Update{Handoff: h}
	if note != "" {
		u.Note = note
	}
	if _, err := s.jr.Apply(st.Entry.MigrationID, u); err != nil {
		s.log.Error("sidecar: handoff update failed", "migrationId", st.Entry.MigrationID, "err", err)
		return
	}
	st.Handoff = h
}

// provenRefusalDeadlineExpiredLocked applies the durable first-refusal bound
// only while custody provably has not moved. A sent entry can retain the
// historical timestamp, but it can only become lost or receive an answer.
func (s *Sidecar) provenRefusalDeadlineExpiredLocked(st *journal.State, now time.Time) bool {
	if st.RefusalDeadlineMs == 0 || st.Handoff.CustodyMayHaveMoved() {
		return false
	}
	if now.Before(time.UnixMilli(st.RefusalDeadlineMs)) {
		return false
	}
	s.bounceLocked(st, "the first proven relay refusal reached bounceTimeoutMs before an alternate took custody")
	return true
}

// rerouteLocked is §7.3's ONE exception to the no-rewrite rule, and it carries
// its own evidence. Only destSlot is rewritten; the migrationId, the axis, the
// exit geometry, the annex and the body are the same bytes.
func (s *Sidecar) rerouteLocked(st *journal.State, proof string, now time.Time) bool {
	if st.Handoff.CustodyMayHaveMoved() {
		// Belt and braces: the caller already checked, and a re-route from here
		// would be the duplication D2 refuses.
		return false
	}
	boundedTransportRefusal := st.RefusalDeadlineMs != 0
	if boundedTransportRefusal && s.provenRefusalDeadlineExpiredLocked(st, now) {
		return true
	}
	if s.cfg.MaxReroutes < 0 {
		// Re-routing is off by configuration (§9.2, maxReroutes). The entry takes
		// the no-lane path instead and bounces home after bounceTimeoutMs.
		if boundedTransportRefusal {
			s.bounceLocked(st, "re-routing is disabled and the relay proved that custody never moved")
			return true
		}
		return false
	}
	if st.RerouteCount >= s.cfg.MaxReroutes {
		s.bounceLocked(st, fmt.Sprintf(
			"maxReroutes (%d) reached; an organism circling a broken axis is a symptom, not a delivery strategy",
			s.cfg.MaxReroutes))
		return true
	}
	var dest int
	walkKnown := false
	if proof == contractb.ProofPeerRefused || boundedTransportRefusal {
		me, ok := mapwalk.Find(s.status, s.slot)
		excluded := make(map[int]bool, len(st.RefusedSlots))
		for _, slot := range st.RefusedSlots {
			excluded[slot] = true
		}
		if ok {
			after, found := mapwalk.Find(s.status, st.Entry.DestSlot)
			if found {
				if contracta.Vertical(st.Entry.Edge) {
					walkKnown = after.Position.Col == me.Position.Col
				} else {
					walkKnown = after.Position.Row == me.Position.Row
				}
			}
			if next, found := mapwalk.WalkAfter(s.status, me, st.Entry.Edge,
				st.Entry.DestSlot, excluded); found {
				dest = next.Slot
			}
		}
		// If the refused slot disappeared between the NACK and this status
		// frame, its position is unknowable. The relay's current effective
		// neighbour remains safe so long as it is neither the current nor an
		// already-refused destination.
		if dest == 0 && (!boundedTransportRefusal || !walkKnown) {
			if n := s.neighbours[st.Entry.Edge]; n != nil &&
				n.Slot != st.Entry.DestSlot && !excluded[n.Slot] {
				dest = n.Slot
			}
		}
	} else if n := s.neighbours[st.Entry.Edge]; n != nil && n.Slot != st.Entry.DestSlot {
		dest = n.Slot
	}
	if dest == 0 {
		if boundedTransportRefusal && walkKnown {
			s.bounceLocked(st, "every compatible same-axis destination was tried after an exact relay NOT_FORWARDED proof")
			return true
		}
		return false
	}
	from := st.RerouteFrom
	if st.RerouteCount == 0 {
		from = st.Entry.DestSlot
	}
	count := st.RerouteCount + 1
	at := now.UnixMilli()
	empty := ""
	zero := int64(0)
	if _, err := s.jr.Apply(st.Entry.MigrationID, journal.Update{
		Handoff:        journal.HandoffPending,
		DestSlot:       &dest,
		RerouteCount:   &count,
		RerouteFrom:    &from,
		RerouteProof:   &proof,
		RerouteAtMs:    &at,
		RelaySessionID: &empty,
		SentAtMs:       &zero,
		Note: fmt.Sprintf("re-routed from slot %d to slot %d on the %s axis under %s",
			from, dest, st.Entry.Edge, proof),
	}); err != nil {
		s.log.Error("sidecar: re-route journal update failed", "migrationId", st.Entry.MigrationID, "err", err)
		return false
	}
	st.Handoff = journal.HandoffPending
	st.Entry.DestSlot = dest
	st.RerouteCount = count
	st.RerouteFrom = from
	st.RerouteProof = proof
	st.RerouteAtMs = at
	st.RelaySessionID = ""
	st.SentAtMs = 0
	s.log.Warn("sidecar: re-routed a journaled hop under a proof of non-delivery",
		"migrationId", st.Entry.MigrationID, "fromSlot", from, "destSlot", dest,
		"axis", st.Entry.Edge, "proof", proof, "rerouteCount", count)
	return true
}

func (s *Sidecar) tickInbound(st *journal.State, now time.Time) {
	id := st.Entry.MigrationID
	if st.Status == journal.StatusDone {
		// The mod ACKed, but the upstream MIGRATION_ACK has not gone out yet —
		// the process may have died between the two. Re-send it.
		if !st.AwaitsUpstreamAck() {
			return
		}
		sc := s.schedFor(id)
		if now.Before(sc.nextForward) {
			return
		}
		// The send pacer decides when this durable ACK can leave. Retry on the
		// next custody tick rather than ForwardRetry: the latter is the network
		// retry for a never-sent payload, and its production value would leave an
		// ACK backlog idle for seconds after tokens became available.
		sc.nextForward = now.Add(s.cfg.TickInterval)
		s.ackUpstreamLocked(st)
		return
	}
	if st.Status != journal.StatusOpen && st.Status != journal.StatusInFlight {
		return
	}
	if s.mod == nil || !s.mod.handshaked {
		return
	}
	sc := s.schedFor(id)
	if st.Status == journal.StatusOpen {
		if now.Before(sc.nextDeliver) {
			// A replay batch waits behind its escalating delay before the pacer
			// takes over (§7.5). An ordinary fresh arrival carries a zero
			// nextDeliver, so now.Before is false and it is released at once.
			return
		}
		s.deliverLocked(st, now)
		return
	}
	if s.mod.paused || s.mod.timeScale == 0 {
		// contract-a.md §8: a missing MIGRATE_IN_ACK is not counted against a
		// paused mod.
		sc.nextDeliver = now.Add(s.cfg.MigrateInAckTimeout)
		return
	}
	if now.After(sc.nextDeliver) {
		s.deliverLocked(st, now)
	}
}

func (s *Sidecar) schedFor(id string) *sched {
	sc, ok := s.sched[id]
	if !ok {
		sc = &sched{}
		s.sched[id] = sc
	}
	return sc
}

// ---------------------------------------------------------------- edges

// sameSize compares two half-extents. contract-a.md §4.1 forbids exact float
// equality, so this is a relative comparison (§13, A10).
func sameSize(a, b float64) bool { return mapwalk.SameSize(a, b) }

// meLocked is this peer's own row in the last PEER_STATUS, filled in from the
// live mod where the status is stale. It is the "me" of §8's walk.
func (s *Sidecar) meLocked() (contractb.SlotInfo, bool) {
	me, ok := mapwalk.Find(s.status, s.slot)
	if !ok {
		if s.slot == 0 {
			return contractb.SlotInfo{}, false
		}
		me = contractb.SlotInfo{Slot: s.slot, Position: s.position, PeerID: s.cfg.PeerID}
	}
	if s.mod != nil && s.mod.handshaked {
		me.GameVersion = s.mod.gameVersion
		me.SimulationSize = s.mod.simSize
	}
	return me, true
}

// edgeStateLocked is contract-b-m4.md §8's table for ONE export edge: the exact
// mapping from the relay's map view to what this sidecar tells its mod. The
// order of the tests is the order of the table.
//
// An export edge closes for ONE REASON ONLY: no slot on that axis is
// deliverable. Every other M3 close reason has been demoted to a SKIP reason,
// and the aggregate of the skips is what names the closure.
func (s *Sidecar) edgeStateLocked(edge string) (open bool, reason string, peerSize float64) {
	switch {
	case !s.relayReady:
		return false, contracta.ReasonPeerUnreachable, 0
	case s.slot == 0:
		return false, contracta.ReasonNoPeer, 0
	}
	if n := s.neighbours[edge]; n != nil {
		return true, contracta.ReasonPeerLive, n.SimulationSize
	}
	// The grant carries no key for this axis, so nothing on it was deliverable.
	// Reproduce §8's walk over PEER_STATUS to name which reason the skips share
	// — the grant cannot carry a skip list for a lane it does not publish.
	me, ok := s.meLocked()
	if !ok {
		return false, contracta.ReasonNoPeer, 0
	}
	_, skipped, found := mapwalk.Walk(s.status, me, edge)
	if found {
		// The status is ahead of the grant. The grant is the authority, so the
		// edge stays closed until the grant catches up.
		return false, contracta.ReasonNoPeer, 0
	}
	return false, mapwalk.EdgeReason(skipped), 0
}

// computeEdgesLocked builds EDGE_STATUS.edges: ONE ENTRY PER DECLARED EXPORT
// EDGE (contract-a.md §15, A18) — FOUR of them for a conformant D17 mod (§18,
// A38). Each edge opens and closes independently: a peer whose whole row went
// dark still exports north, a peer alone in the map closes all of them with
// no_peer, and on an axis of length 2 the pair on that axis opens and closes
// TOGETHER because both walks name the same peer. An empty array closes every
// edge and is the correct frame when this sidecar holds no slot.
//
// `open` GOVERNS EXPORTS ONLY (§5.4). Nothing computed here reaches the
// inbound delivery path, by construction: see onMigrationPayload, where an
// arrival is journaled and delivered without ever asking what this list says.
func (s *Sidecar) computeEdgesLocked() []contracta.EdgeState {
	if s.mod == nil || !s.mod.handshaked || len(s.mod.exportEdges) == 0 {
		return nil
	}
	out := make([]contracta.EdgeState, 0, len(s.mod.exportEdges))
	for _, edge := range s.mod.exportEdges {
		open, reason, peerSize := s.edgeStateLocked(edge)
		st := contracta.EdgeState{Edge: edge, Open: open, Reason: reason}
		if open {
			size := peerSize
			st.PeerSimulationSize = &size
		}
		out = append(out, st)
	}
	return out
}

func edgesEqual(a, b []contracta.EdgeState) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Edge != b[i].Edge || a[i].Open != b[i].Open || a[i].Reason != b[i].Reason {
			return false
		}
		switch {
		case a[i].PeerSimulationSize == nil && b[i].PeerSimulationSize == nil:
		case a[i].PeerSimulationSize == nil || b[i].PeerSimulationSize == nil:
			return false
		case *a[i].PeerSimulationSize != *b[i].PeerSimulationSize:
			return false
		}
	}
	return true
}

// publishEdgesLocked sends EDGE_STATUS when the full state changed, or always
// when force is set (the handshake case).
func (s *Sidecar) publishEdgesLocked(force bool) {
	if s.mod == nil || !s.mod.handshaked {
		return
	}
	edges := s.computeEdgesLocked()
	if !force && edgesEqual(edges, s.lastEdges) {
		return
	}
	s.lastEdges = edges
	s.edgeEpoch++
	if edges == nil {
		edges = []contracta.EdgeState{}
	}
	s.sendModLocked(contracta.TypeEdgeStatus, contracta.EdgeStatus{Epoch: s.edgeEpoch, Edges: edges})
}

// exportOpenLocked reports whether migration out of edge is permitted right
// now, and the destination slot. A closed edge comes back with the
// MIGRATE_OUT_NACK code contract-a.md §9.1 assigns.
func (s *Sidecar) exportOpenLocked(edge string, simSize float64) (destSlot int, code string, message string) {
	if s.mod == nil || !s.mod.handshaked {
		return 0, contracta.OutEdgeClosed, "no mod session"
	}
	declared := false
	for _, e := range s.mod.exportEdges {
		if e == edge {
			declared = true
			break
		}
	}
	if !declared {
		// §15 A18: the sidecar MUST NOT open an undeclared edge, whatever
		// borderEdges says.
		return 0, contracta.OutEdgeClosed,
			"edge " + edge + " is not one of this sim's exportEdges " + strings.Join(s.mod.exportEdges, ",")
	}
	if !s.relayReady {
		return 0, contracta.OutNoRoute, "no relay link"
	}
	if s.slot == 0 {
		return 0, contracta.OutNoRoute, "no slot granted yet"
	}
	open, reason, _ := s.edgeStateLocked(edge)
	if !open {
		switch reason {
		case contracta.ReasonSimSizeMismatch:
			return 0, contracta.OutSimSizeMismatch,
				"no slot on the " + edge + " axis agrees about simulationSize"
		case contracta.ReasonPeerIncompatible:
			return 0, contracta.OutPeerIncompatible,
				"no slot on the " + edge + " axis runs a compatible gameVersion"
		case contracta.ReasonPeerUnreachable:
			return 0, contracta.OutNoRoute, "no relay link"
		default:
			return 0, contracta.OutEdgeClosed, "the " + edge + " export edge is closed: " + reason
		}
	}
	n := s.neighbours[edge]
	if !sameSize(n.SimulationSize, simSize) {
		return 0, contracta.OutSimSizeMismatch,
			fmt.Sprintf("the %s neighbour's simulationSize %v differs from %v", edge, n.SimulationSize, simSize)
	}
	return n.Slot, "", ""
}

// ---------------------------------------------------------------- custody

func (s *Sidecar) forwardLocked(st *journal.State, now time.Time) bool {
	// Older journals can have an empty handoff, and §9.2 treats that value as
	// sent. No state for which custody might have moved can enter the outbound
	// writer a second time, even if a caller offers it by mistake.
	if st.Handoff != journal.HandoffPending && st.Handoff != journal.HandoffRefused {
		return false
	}
	// B24's client half, asked before the payload is built (contract-b-m4.md
	// §3.3, §6.2, §22 B24). A drain offers every open entry on every tick, so
	// this gate is what keeps a deferred forward from costing a multi-megabyte
	// encode; prepareRelayFrameLocked still makes the real decision. THE ENTRY IS LEFT
	// EXACTLY AS IT WAS — no journal write, no handoff change, no schedule move —
	// so the next tick offers it again and the backlog drains slower rather than
	// shorter. The wall clock is the one the relay's meter runs on.
	if !s.sendPace.readyForBulk(time.Now()) {
		return false
	}
	payload := contractb.MigrationPayload{
		MigrationID: st.Entry.MigrationID,
		Kind:        st.Entry.Kind,
		Body:        contractb.Body{Version: st.Entry.GameVersion, BB8: st.Entry.Payload},
		Lineage:     lineageOf(st.Entry),
		// COPY, NEVER AUTHOR (§6.6). The block goes out exactly as the journal
		// holds it, which is exactly as MIGRATE_OUT delivered it. A re-route and
		// an offer of an entry no relay ever took reproduce the same bytes,
		// because they read the same journal entry.
		Species:    st.Entry.Species,
		SourcePeer: s.cfg.PeerID,
		SourceSlot: st.Entry.SourceSlot,
		DestSlot:   st.Entry.DestSlot,
		ExitEdge:   st.Entry.Edge,
		// contract-a.md §4.3: the sidecar copies exitPosition and never
		// converts it. The mod owns the geometry.
		ExitPosition: st.Entry.Position,
		Velocity:     contractb.Vec{X: st.Entry.VelocityX, Y: st.Entry.VelocityY},
		Heading:      st.Entry.Heading,
		EntityID:     st.Entry.EntityID,
		// A re-route is a change of destination, not a new migration: timestamp
		// still names the original journal write, because the organism left its
		// world once.
		Timestamp: st.Entry.JournaledAt,
	}
	if st.RerouteCount > 0 {
		payload.Reroute = &contractb.Reroute{
			FromSlot: st.RerouteFrom, Count: st.RerouteCount,
			Proof: st.RerouteProof, AtMs: st.RerouteAtMs}
	}
	frame, ok := s.prepareRelayFrameLocked(contractb.TypeMigrationPayload, payload, true)
	if !ok {
		return false
	}
	if s.afterForwardPrepare != nil {
		s.afterForwardPrepare()
	}
	// Encoding and pace admission can take time. Re-read the sidecar clock after
	// both and enforce the absolute first-refusal bound before the durable sent
	// transition. Crossing the bound at this boundary bounces once; it never
	// commits or enqueues an out-of-bounds alternate.
	if st.RefusalDeadlineMs != 0 {
		now = s.now()
		if s.provenRefusalDeadlineExpiredLocked(st, now) {
			return false
		}
	}
	// §9.2: pending/refused -> sent, RECORDING THE relaySessionId IN FORCE BEFORE
	// THIS ATTEMPT'S ENQUEUE. A safe reroute cleared the preceding attempt's
	// session and sentAt, so every later proof is scoped to this attempt.
	//
	// THE DURABLE TRANSITION PRECEDES THE SOCKET QUEUE. If the process crashes,
	// or the local queue rejects the prepared frame, replay sees `sent` and can
	// only wait for an answer or record a loss. Marking after Send would leave a
	// crash window in which the relay could have the organism while the journal
	// still invited a retry.
	session := s.relaySessionID
	sentAt := now.UnixMilli()
	u := journal.Update{
		Handoff: journal.HandoffSent, SentAtMs: &sentAt, RelaySessionID: &session,
	}
	if st.Status != journal.StatusInFlight {
		u.Status = journal.StatusInFlight
	}
	if _, err := s.jr.Apply(st.Entry.MigrationID, u); err != nil {
		s.log.Error("sidecar: journal update failed", "migrationId", st.Entry.MigrationID, "err", err)
		return false
	}
	st.Handoff = journal.HandoffSent
	st.SentAtMs = sentAt
	st.RelaySessionID = session

	s.faultPoint(FaultPreForward)
	if !s.sendPreparedRelayFrameLocked(contractb.TypeMigrationPayload, frame) {
		s.log.Error("sidecar: prepared migration was not enqueued after its durable sent transition; "+
			"it will not be retried",
			"migrationId", st.Entry.MigrationID, "destSlot", st.Entry.DestSlot,
			"relaySessionId", st.RelaySessionID)
		return false
	}
	s.log.Info("sidecar: forwarded MIGRATION_PAYLOAD",
		"migrationId", st.Entry.MigrationID, "destSlot", st.Entry.DestSlot,
		"exitEdge", st.Entry.Edge, "relaySessionId", st.RelaySessionID,
		"reroutes", st.RerouteCount, "genomeHash", st.Entry.GenomeHash,
		"parents", len(st.Entry.Parents), "species", wire.SpeciesName(st.Entry.Species))
	s.faultPoint(FaultPostForward)
	return true
}

// lineageOf builds the wire annex from the journal entry. Parents is never nil:
// §6.6 requires the array, and an empty one is normal.
func lineageOf(e journal.Entry) contractb.Lineage {
	parents := make([]contractb.Parent, 0, len(e.Parents))
	for _, p := range e.Parents {
		parents = append(parents, contractb.Parent{
			EntityID: p.EntityID, GenomeHash: p.GenomeHash, GapReason: p.GapReason})
	}
	return contractb.Lineage{GenomeHash: e.GenomeHash, Parents: parents}
}

// bounceLocked re-injects an outbound organism into the local sim. The journal
// entry keeps its migrationId and flips direction, so dedup still holds end to
// end (custody chain step 6). It comes home ON THE EDGE IT LEFT FROM — the
// origin's own exitEdge, any of the four (§9.4).
//
// Under two-way lanes that edge is also a normal ENTRY edge, and the bounce is
// unchanged anyway (contract-a.md §18, A38): same inset, same immunity window,
// same outward velocity. Ordinary arrivals on that edge come in moving INWARD
// and a bounce comes in moving OUTWARD, so the two populations are separated by
// the mod's own outward-velocity test rather than by a rule here.
// EVERY CALLER IS A CASE WHERE NO CUSTODY MOVED (§9.4). Since §25's B37 there is
// no ambiguous timeout bounce. B45's first-refusal deadline is safe because an
// exact relay proof says custody did not move. A bounce is never a guess unless
// an operator explicitly takes that risk by hand.
func (s *Sidecar) bounceLocked(st *journal.State, why string) {
	id := st.Entry.MigrationID
	bounce := true
	attempt := 0
	if _, err := s.jr.Apply(id, journal.Update{
		Status:     journal.StatusOpen,
		Direction:  journal.In,
		BounceBack: &bounce,
		Attempt:    &attempt,
		Handoff:    journal.HandoffDone,
		Note:       "bounced: " + why,
	}); err != nil {
		s.log.Error("sidecar: bounce journal update failed", "migrationId", id, "err", err)
		return
	}
	delete(s.sched, id)
	s.log.Warn("sidecar: bouncing organism back into the local sim",
		"migrationId", id, "entityId", st.Entry.EntityID, "why", why)
}

// deliverLocked emits one MIGRATE_IN THROUGH THE DELIVERY RATE LIMIT
// (contract-a.md §7.5, §15 A20). Pacing sits AFTER the durable journal write,
// never before it: custody is taken at the speed of the wire and released at
// the speed of the world.
func (s *Sidecar) deliverLocked(st *journal.State, now time.Time) {
	if !s.pace.allow(now, s.cfg.PacingIdleGrace) {
		return
	}
	// Hold delivery into a mod that has gone quiet at the application layer. The
	// pacer's own idle grace (pacingIdleGraceMs, 10 s) used to be LONGER than the
	// 3.5 s heartbeat timeout, so during a stall the socket was closed with 4004
	// before that branch could trip — which left this function releasing
	// MIGRATE_IN frames right up to the close, into a mod whose main thread was
	// already drowning. §20 A45 raised the timeout to 13 s and so inverted that
	// pair: the idle grace now trips first on a stall past ten seconds. Nothing
	// here changes, because both branches release nothing and this one trips at
	// 1.5 s regardless — the gate below is still what makes a stalling mod paused
	// into rather than flooded into.
	// Gate on heartbeat freshness: a mod whose last app-level HEARTBEAT
	// is older than heartbeatDeliveryGraceMs (~1.5x the interval) is not keeping
	// up, so the entry stays scheduled and delivers when heartbeats resume. This
	// changes WHEN, not WHETHER (§7.5); the token is spent only after the journal
	// write below, so a held delivery costs nothing. lastHeartbeat is s.mu-owned
	// and every caller holds s.mu, so this read is race-free.
	if s.mod != nil && now.Sub(s.mod.lastHeartbeat) > s.cfg.HeartbeatDeliveryGrace {
		return
	}
	id := st.Entry.MigrationID
	attempt := st.Attempt + 1
	updated, err := s.jr.Apply(id, journal.Update{Status: journal.StatusInFlight, Attempt: &attempt})
	if err != nil {
		s.log.Error("sidecar: delivery journal update failed", "migrationId", id, "err", err)
		return
	}
	s.pace.take()
	msg := contracta.MigrateIn{
		MigrationID: id,
		EntityID:    updated.Entry.EntityID,
		Kind:        updated.Entry.Kind,
		GameVersion: updated.Entry.GameVersion,
		Payload:     updated.Entry.Payload,
		// Handed through VERBATIM when the entry carried one, and OMITTED when it
		// did not (contract-b-m4.md §6.6 step 7). Nothing here resolves, translates
		// or annotates it: the registry the name resolves against lives inside the
		// game process. A BOUNCE-BACK takes this path too, so an organism that
		// comes home comes home under the name it left with.
		Species:       updated.Entry.Species,
		EntryEdge:     updated.Entry.Edge,
		EntryPosition: updated.Entry.Position,
		Velocity:      contracta.Vec{X: updated.Entry.VelocityX, Y: updated.Entry.VelocityY},
		Heading:       updated.Entry.Heading,
		BounceBack:    updated.BounceBack,
		Attempt:       attempt,
		AckDeadlineMs: int(s.cfg.MigrateInAckTimeout / time.Millisecond),
	}
	s.sendModLocked(contracta.TypeMigrateIn, msg)
	s.schedFor(id).nextDeliver = now.Add(s.cfg.MigrateInAckTimeout)
	s.log.Info("sidecar: delivered MIGRATE_IN", "migrationId", id, "attempt", attempt,
		"entryEdge", updated.Entry.Edge, "bounceBack", updated.BounceBack,
		"species", wire.SpeciesName(updated.Entry.Species), "pacedDepth", s.pacedDepthLocked())
}

func (s *Sidecar) ackUpstreamLocked(st *journal.State) {
	ack := contractb.MigrationAck{
		MigrationID: st.Entry.MigrationID,
		SourcePeer:  s.cfg.PeerID,
		DestPeer:    st.Entry.SourcePeer,
		EntityID:    st.Entry.EntityID,
		Duplicate:   st.Duplicate,
		DeliveredAt: s.now().UnixMilli(),
	}
	if !s.sendRelayLocked(contractb.TypeMigrationAck, ack) {
		return
	}
	acked := true
	if _, err := s.jr.Apply(st.Entry.MigrationID, journal.Update{Acked: &acked}); err != nil {
		s.log.Error("sidecar: journal update failed", "migrationId", st.Entry.MigrationID, "err", err)
	}
}

// cacheGenome stores one blob under its hash. A cache write failure is logged
// and never fails a migration (contract-b-m4.md §6.6 step 6).
func (s *Sidecar) cacheGenome(hash, version, blob string) {
	if hash == "" || blob == "" {
		return
	}
	if err := s.genomes.Put(hash, version, blob); err != nil {
		s.log.Warn("sidecar: genome cache write failed", "genomeHash", hash, "err", err)
	}
}

// faultPoint is the test-only crash hook. It touches a marker and blocks, so a
// test can SIGKILL the process at an exactly known point in the custody chain.
func (s *Sidecar) faultPoint(point string) {
	if s.cfg.Fault != point {
		return
	}
	_ = os.WriteFile(filepath.Join(s.cfg.DataDir, "fault.hit"), []byte(point+"\n"), 0o644)
	s.log.Error("sidecar: fault injection point reached, blocking", "point", point)
	select {}
}

// ---------------------------------------------------------------- stats

// statsLocked builds the peer stats block of §6.3.1. EVERY FIELD IS OPTIONAL
// AND ABSENCE IS A VALUE: a stat this sidecar does not know is omitted, never
// defaulted, because a slot that reports nothing is unknown, not empty.
func (s *Sidecar) statsLocked() contractb.PeerStats {
	committed, known := s.committedPopulationLocked()
	admission := s.admission
	admission.refresh(committed, known)
	a := admission.snapshot()
	st := contractb.PeerStats{
		CustodyDepth:     contractb.IntPtr(s.jr.CountPending(journal.Out) + s.jr.CountPending(journal.In)),
		PacedDepth:       contractb.IntPtr(s.pacedDepthLocked()),
		LostForwardTotal: contractb.IntPtr(s.lostForwardTotal),
		// The delivery rate limit this sidecar is CONFIGURED with (§18, B16).
		// Always known, because it is this process's own setting and not a
		// reading of anything else — and it is what makes pacedDepth readable:
		// a queue is only deep against the cap it is queued behind. The default
		// has moved three times, so a reader that assumes one is wrong.
		InboundRatePerSimMinute: contractb.Float64Ptr(s.cfg.InboundRatePerSimMinute),
		InboundRateBurst:        contractb.Float64Ptr(s.cfg.InboundRateBurst),
		// The participant's own two public strings (§33, B49). Config-sourced
		// like the two above, so they are published WHETHER OR NOT A MOD IS
		// CONNECTED: a world whose game is shut down still says whose it is,
		// and a slot that goes dark keeps a name an operator can read it by.
		// Empty is ABSENT — there is nothing here to invent one from.
		Keeper:                   s.cfg.Keeper,
		WorldName:                s.cfg.WorldName,
		AdmissionMode:            a.Mode,
		AdmissionTargetTimeScale: contractb.Float64Ptr(a.TargetTimeScale),
		AdmissionClosed:          contractb.BoolPtr(a.Closed),
		AdmissionEnforcing:       contractb.BoolPtr(a.Enforcing),
		AdmissionSampleCount:     contractb.IntPtr(a.SampleCount),
		AdmissionRejectedTotal:   contractb.IntPtr(a.RejectedTotal),
	}
	if a.EffectiveLimit > 0 {
		st.AdmissionPopulationLimit = contractb.IntPtr(a.EffectiveLimit)
	}
	if a.EstimatedLimit > 0 {
		st.AdmissionEstimatedLimit = contractb.IntPtr(a.EstimatedLimit)
	}
	if a.PopulationKnown {
		st.AdmissionCommitted = contractb.IntPtr(a.Committed)
	}
	if s.mod != nil && s.mod.handshaked {
		if s.mod.haveTimeScale {
			// Copied from the HEARTBEAT, never computed. 0 is a world standing
			// still, which is a reading; absent is a world that has not said.
			st.TimeScale = contractb.Float64Ptr(s.mod.timeScale)
		}
		if s.mod.haveTargetTimeScale {
			st.TargetTimeScale = contractb.Float64Ptr(s.mod.targetTimeScale)
		}
		if s.mod.havePopulation {
			st.Population = contractb.IntPtr(s.mod.population)
		}
		if s.mod.haveEggCount {
			st.EggCount = contractb.IntPtr(s.mod.eggCount)
		}
		if s.mod.haveSimTime {
			st.SimulatedTime = contractb.Float64Ptr(s.mod.simulatedTime)
		}
		if s.mod.lastSave != nil {
			save := *s.mod.lastSave
			st.LastSave = &save
		}
		// COPY, NEVER AUTHOR (contract-b-m4.md §16, B11). The census that
		// reaches the relay is the one the last HEARTBEAT carried, entry for
		// entry, byte for byte, in the order the mod sorted it. This sidecar
		// does not synthesize one, does not re-sort one, does not merge two
		// entries, does not fill a missing count, and does not derive one from
		// its journal, its genome cache or the migration blocks that pass
		// through it — those describe migrants, this describes a population.
		//
		// Clone because the decoded frame is transient and this block outlives
		// it. nil stays nil: absent is unknown, and unknown is a value.
		if s.mod.census != nil {
			st.Species = s.mod.census.Clone()
			st.Truncated = contractb.TruncatedFlag(s.mod.censusTruncated)
		}

		// THE SEVEN SETTINGS AND VERSION FIELDS OF §19, B18.
		//
		// Two of them this sidecar has held since M2 and never published:
		// modVersion, and the contract-a identifier THIS MOD'S SESSION IS
		// SPEAKING. The second is not this build's wire.ProtocolA — a sidecar
		// MUST publish what the peer actually sent, because those differ on
		// exactly the rig this field exists to describe, and a slot reporting
		// contract-a/2.2 has no settings BECAUSE its mod cannot send any.
		//
		// The other five are COPIED, NEVER AUTHORED, exactly as the census is:
		// this sidecar does not default, repair, re-normalize or infer one, and
		// an absent one stays absent all the way to the page, where it renders
		// unknown rather than the value the mod happens to ship with.
		st.ModVersion = s.mod.modVersion
		st.ContractAVersion = s.mod.contractAVersion
		// Clone: the decoded handshake is transient and this block outlives it.
		// nil stays nil (unknown); a present empty list stays present (the
		// exclusion policy is off), and those are different facts.
		st.MigrationExclude = s.mod.settings.MigrationExclude.Clone()
		st.SaveMinutes = copyFloat(s.mod.settings.SaveMinutes)
		st.SaveKeep = copyInt(s.mod.settings.SaveKeep)
		st.SaveOnQuit = copyBool(s.mod.settings.SaveOnQuit)
		st.WorldWrapping = copyBool(s.mod.settings.WorldWrapping)
	}
	return st
}

// The three copy helpers keep a pointer into the session's own state out of a
// block that is about to be encoded on another goroutine. nil stays nil, which
// is the whole distinction these fields carry.
func copyFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func copyInt(v *int) *int {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func copyBool(v *bool) *bool {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

// pacedDepthLocked is the number of inbound entries waiting on the delivery
// rate limit. A depth that never falls names a limit set too low.
func (s *Sidecar) pacedDepthLocked() int {
	n := 0
	for _, st := range s.jr.List() {
		if st.Direction == journal.In && st.Status == journal.StatusOpen {
			n++
		}
	}
	return n
}

// unresolvedDepthLocked is the number of outbound entries in the `sent` state of
// §9.2 — forwarded once, no answer yet, and nothing this sidecar can do about
// it. It is a LOCAL diagnostic and not a wire stat: `custodyDepth` already
// counts these on §6.3.1, and B37 added one field to that block and no more.
func (s *Sidecar) unresolvedDepthLocked() int {
	n := 0
	for _, st := range s.jr.List() {
		if st.Direction == journal.Out && st.Handoff == journal.HandoffSent &&
			(st.Status == journal.StatusOpen || st.Status == journal.StatusInFlight) {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------- operator

// InflightEntry is one row of --list-inflight (§7.5).
type InflightEntry struct {
	MigrationID string
	EntityID    int32
	Direction   string
	Status      string
	Handoff     string
	DestSlot    int
	ExitEdge    string
	// SentAt is when the sidecar durably committed to its one socket enqueue, and
	// LostIn is what is left of forwardTimeoutMs before the entry is recorded
	// lost (§9.3). Both are zero on an entry that has never reached that commit.
	SentAt       time.Time
	LostIn       time.Duration
	Reroutes     int
	RerouteFrom  int
	RerouteProof string
	RelaySession string
	JournaledAt  time.Time
	Note         string
	// The FORWARD_RECEIPT block (§6.12, §22 B26). It is the reason this report
	// can now say WHETHER the relay ever wrote this frame instead of leaving the
	// operator to infer it from a handoff state that means "unknowably".
	//
	// ForwardReceipts of 0 is NOT a statement that nothing was forwarded. A
	// missing receipt is silence, and silence is never proof in this contract;
	// the printed report says so in as many words, because this is the one
	// surface where a person is about to act on the difference.
	ForwardReceipts  int
	ReceiptSession   string
	ReceiptDestSlot  int
	ReceiptForwarded time.Time
}

// ListInflight answers the question the relay CANNOT: which entries name this
// slot, and what are they (§7.5). It runs on the machine that owns the journal,
// because D2 keeps custody local.
func ListInflight(dataDir string, destSlot int, forwardTimeout time.Duration) ([]InflightEntry, error) {
	jr, err := journal.Open(filepath.Join(dataDir, "journal"))
	if err != nil {
		return nil, err
	}
	defer jr.Close()
	out := []InflightEntry{}
	for _, st := range jr.List() {
		if st.Status != journal.StatusOpen && st.Status != journal.StatusInFlight &&
			st.Status != journal.StatusHeld {
			continue
		}
		if destSlot > 0 && st.Entry.DestSlot != destSlot {
			continue
		}
		var sentAt time.Time
		var lostIn time.Duration
		if st.SentAtMs > 0 {
			sentAt = time.UnixMilli(st.SentAtMs)
			lostIn = forwardTimeout - time.Since(sentAt)
		}
		out = append(out, InflightEntry{
			MigrationID:  st.Entry.MigrationID,
			EntityID:     st.Entry.EntityID,
			Direction:    string(st.Direction),
			Status:       string(st.Status),
			Handoff:      string(st.Handoff),
			DestSlot:     st.Entry.DestSlot,
			ExitEdge:     st.Entry.Edge,
			SentAt:       sentAt,
			LostIn:       lostIn,
			Reroutes:     st.RerouteCount,
			RerouteFrom:  st.RerouteFrom,
			RerouteProof: st.RerouteProof,
			RelaySession: st.RelaySessionID,
			JournaledAt:  time.UnixMilli(st.Entry.JournaledAt),
			Note:         st.Note,

			ForwardReceipts: st.ForwardReceipts,
			ReceiptSession:  st.ReceiptSessionID,
			ReceiptDestSlot: st.ReceiptDestSlot,
			ReceiptForwarded: func() time.Time {
				if st.ReceiptForwardedAtMs == 0 {
					return time.Time{}
				}
				return time.UnixMilli(st.ReceiptForwardedAtMs)
			}(),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].JournaledAt.Before(out[j].JournaledAt) })
	return out, nil
}

// ReleaseInflight is the operator's escape hatch of §7.5: it resolves one
// unresolved outbound entry by hand. It is the custody twin of --release-slot,
// and it runs on the machine that holds the journal.
//
// SINCE §25's B37 IT IS THE ONLY WAY LEFT TO DUPLICATE AN ORGANISM. Nothing in
// this sidecar bounces a forwarded entry any more; `bounce` on an entry in
// `sent` is a person deciding, with the receipt evidence in front of them, that
// the far side never took custody. InflightRisk is printed before it acts.
//
// The sidecar for this data directory MUST be stopped: the journal is a
// single-writer file.
func ReleaseInflight(dataDir, migrationID, action string) (string, error) {
	if action != "bounce" && action != "drop" {
		return "", fmt.Errorf("sidecar: --release-inflight takes bounce or drop, not %q", action)
	}
	jr, err := journal.Open(filepath.Join(dataDir, "journal"))
	if err != nil {
		return "", err
	}
	defer jr.Close()
	st, ok := jr.Get(migrationID)
	if !ok {
		return "", fmt.Errorf("sidecar: no journal entry for %s", migrationID)
	}
	if st.Direction != journal.Out {
		return "", fmt.Errorf("sidecar: %s is an inbound entry; --release-inflight releases outbound custody",
			migrationID)
	}
	if action == "bounce" {
		bounce := true
		attempt := 0
		if _, err := jr.Apply(migrationID, journal.Update{
			Status: journal.StatusOpen, Direction: journal.In, BounceBack: &bounce,
			Attempt: &attempt, Handoff: journal.HandoffDone,
			Note: "bounced by operator command --release-inflight"}); err != nil {
			return "", err
		}
		return fmt.Sprintf(
			"released %s (entity %d, destSlot %d, handoff %s): it will be delivered to THIS world's mod\n"+
				"on the edge it left by (%s) the next time this sidecar starts.",
			migrationID, st.Entry.EntityID, st.Entry.DestSlot, st.Handoff, st.Entry.Edge), nil
	}
	completed := time.Now().UnixMilli()
	if _, err := jr.Apply(migrationID, journal.Update{
		Status: journal.StatusDone, CompletedAt: &completed, Handoff: journal.HandoffDone,
		Note: "dropped by operator command --release-inflight"}); err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"dropped %s (entity %d, destSlot %d, handoff %s): the organism is GONE from this map.\n"+
			"D2 accepts loss; it never accepts duplication, and a drop is a loss you chose.",
		migrationID, st.Entry.EntityID, st.Entry.DestSlot, st.Handoff), nil
}

// InflightRisk is the duplication warning §9.3 REQUIRES --release-inflight to
// print in its own output before acting.
const InflightRisk = `
RISK, and it is the reason this command asks.

An entry in handoff "sent" was durably committed to one socket enqueue, so the
relay may have accepted it and the far sidecar may hold custody. If it does, and it returns
and replays its own journal after you bounce this one home, THE MAP HOLDS TWO
COPIES.

At-most-once now carries NO automatic exception: this sidecar never bounces a
forwarded organism by itself, and an unanswered forward is recorded LOST rather
than brought home (§9.3, §25 B37). THIS COMMAND IS THE ONLY WAY LEFT TO
DUPLICATE AN ORGANISM ON THIS MAP, and you are the one firing it.

An entry in handoff "pending" or "refused" was never handed to anybody, or was
refused before custody moved. Bouncing one of those cannot duplicate anything.
`

// ---------------------------------------------------------------- persistence

// resolvePeerID returns the identity this sidecar keeps for life. An explicit
// --peer-id wins and is written down; otherwise the persisted one is reused;
// otherwise one is generated on first start (contract-b-m4.md §7.4).
func (s *Sidecar) resolvePeerID(explicit string) string {
	path := filepath.Join(s.cfg.DataDir, "peer-id")
	stored := ""
	if b, err := os.ReadFile(path); err == nil {
		stored = strings.TrimSpace(string(b))
	}
	id := explicit
	if id == "" {
		id = stored
	}
	if id == "" {
		id = "peer-" + wire.NewUUID()[:8]
	}
	if id != stored {
		if err := os.WriteFile(path, []byte(id+"\n"), 0o644); err != nil {
			s.cfg.Logger.Error("sidecar: could not persist the peer id; "+
				"a restart would take a second slot and strand this one", "err", err)
		}
	}
	return id
}

func (s *Sidecar) readSlot() int {
	b, err := os.ReadFile(filepath.Join(s.cfg.DataDir, "slot"))
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || n < 1 {
		// §7.4: an unreadable value is treated as absent.
		return 0
	}
	return n
}

func (s *Sidecar) writeSlot(slot int) {
	if slot < 1 {
		return
	}
	_ = os.WriteFile(filepath.Join(s.cfg.DataDir, "slot"), []byte(strconv.Itoa(slot)+"\n"), 0o644)
}

// readPosition reads <data-dir>/position, "col,row". It is only the
// preferredPosition hint, and only useful to a peer that lost its peerId too.
func (s *Sidecar) readPosition() *contractb.Position {
	b, err := os.ReadFile(filepath.Join(s.cfg.DataDir, "position"))
	if err != nil {
		return nil
	}
	colStr, rowStr, ok := strings.Cut(strings.TrimSpace(string(b)), ",")
	if !ok {
		return nil
	}
	col, err1 := strconv.Atoi(strings.TrimSpace(colStr))
	row, err2 := strconv.Atoi(strings.TrimSpace(rowStr))
	if err1 != nil || err2 != nil || col < 0 || row < 0 {
		return nil
	}
	return &contractb.Position{Col: col, Row: row}
}

func (s *Sidecar) writePosition(pos contractb.Position) {
	_ = os.WriteFile(filepath.Join(s.cfg.DataDir, "position"),
		[]byte(strconv.Itoa(pos.Col)+","+strconv.Itoa(pos.Row)+"\n"), 0o644)
}

// credentialState says whether a peer credential is configured WITHOUT saying
// anything about its value. A log line that named a prefix would be a log line
// that leaked one, and §3.1 keeps the secret off every surface but the HTTP
// upgrade it rides.
func credentialState(secret string) string {
	if secret == "" {
		return "NOT CONFIGURED — the relay will answer 401 (--credential-file)"
	}
	return "configured"
}
