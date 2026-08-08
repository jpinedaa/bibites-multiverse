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
// non-delivery (contract-b-m4.md §9.2). And an entry whose destination went dark
// with no such proof runs a BOUNDED HOLD whose clock advances only while the
// destination is dark and this sidecar can see it (§9.3).
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

	// Custody scheduling, in memory. The durable half lives in the journal.
	sched        map[string]*sched
	seenSessions map[string]bool
	lastPurge    time.Time
	lastSweep    time.Time
	lastCompact  time.Time
	// pace is the delivery rate limit of contract-a.md §7.5.
	pace pacer
	// bouncedTimeoutTotal is monotonic and reset only by losing the journal.
	bouncedTimeoutTotal int
	duplicateSuspected  int
	// genomeServed counts GENOME_REQUESTs answered per requester in the current
	// minute (contract-b-m4.md §10's rate limit, answering side).
	genomeServed map[string]*rateWindow
	closed       bool
}

type sched struct {
	nextForward time.Time
	bounceAt    time.Time
	nextDeliver time.Time
	// liveForwardCount is how many times this entry has been re-forwarded to a
	// LIVE destination on the current run, driving the exponential backoff of
	// §9.3/B5. It resets on a liveness flip and on any state change — a dark
	// destination (dark uses the flat cadence), a re-route, and an ACK or bounce
	// that drops the sched entry entirely. In memory only, like the rest here.
	liveForwardCount int
	// holdSince is when the hold clock last started running for this entry. The
	// clock is not a deadline: it accrues, it stops, and it resumes.
	holdSince      time.Time
	pendingAccrual time.Duration
	lastFlush      time.Time
}

type rateWindow struct {
	windowStart time.Time
	count       int
}

// New builds a sidecar and opens its journal, replaying whatever custody the
// previous process left behind (D2).
func New(cfg Config) (*Sidecar, error) {
	cfg.applyDefaults()
	if cfg.Fault == "" {
		cfg.Fault = os.Getenv("MULTIVERSE_FAULT")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, err
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
	s.pace = newPacer(cfg.InboundRatePerSimMinute, cfg.InboundRateBurst)
	// §7.4: peerId is persisted outside the journal. Losing it makes the peer a
	// stranger that takes a second slot and strands its old one — which is why
	// §7.5 gives the operator a release and a handover command.
	s.cfg.PeerID = s.resolvePeerID(cfg.PeerID)
	s.log = s.cfg.Logger.With("peer", s.cfg.PeerID)
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
	// §9.3: on restart the sidecar RESUMES the accrual and MUST NOT start a
	// fresh timer. Say so out loud, because the alternative is a silent reset of
	// a 24-hour clock.
	for _, st := range jr.List() {
		if st.Direction == journal.Out && st.Handoff == journal.HandoffHeld {
			s.log.Warn("sidecar: resuming a bounded hold from the journal",
				"migrationId", st.Entry.MigrationID, "destSlot", st.Entry.DestSlot,
				"accruedHold", time.Duration(st.AccruedHoldMs)*time.Millisecond,
				"holdTimeout", s.cfg.HoldTimeout)
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
	s.httpSrv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	// Publish the resolved address so a caller that asked for port 0 can find
	// it without parsing logs.
	_ = os.WriteFile(filepath.Join(s.cfg.DataDir, "listen.addr"), []byte(ln.Addr().String()+"\n"), 0o644)

	s.wg.Add(1)
	go func() { defer s.wg.Done(); _ = s.httpSrv.Serve(ln) }()
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.relayLoop() }()
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.tickLoop() }()

	s.log.Info("sidecar: listening", "addr", ln.Addr().String(), "path", contracta.ContractAPath,
		"relay", s.cfg.RelayURL, "dataDir", s.cfg.DataDir, "preferredSlot", s.cfg.PreferredSlot,
		"preferredPosition", s.cfg.PreferredPosition)
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
	// §9.3: the accrued hold is flushed on CLEAN SHUTDOWN, so a planned restart
	// loses none of it.
	s.flushAccrualsLocked()
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
	for _, st := range s.jr.List() {
		switch st.Direction {
		case journal.Out:
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

// tickOutbound is §9.2's state machine, in one place.
func (s *Sidecar) tickOutbound(st *journal.State, now time.Time) {
	if st.Status != journal.StatusOpen && st.Status != journal.StatusInFlight {
		return
	}
	sc := s.schedFor(st.Entry.MigrationID)
	live := s.destLiveLocked(st.Entry.DestSlot)

	switch st.Handoff {
	case journal.HandoffSent, journal.HandoffHeld, "":
		// CUSTODY MAY HAVE MOVED, unknowably. This entry is never re-routed on
		// its own account: silence is not proof, and only a statement from the
		// relay or the receiving peer can change that (§9.2).
		if live {
			if st.Handoff == journal.HandoffHeld {
				// held -> sent: the clock stops and THE ACCRUAL IS KEPT.
				s.stopHoldLocked(st, sc, now)
				s.setHandoffLocked(st, journal.HandoffSent,
					"destination came back; hold clock stopped, accrual kept")
			} else {
				s.stopHoldLocked(st, sc, now)
			}
		} else {
			// A dark destination keeps the flat retry cadence, so the live-forward
			// backoff resets here: the next time this destination is live, the
			// backoff climbs from forwardRetryMs again.
			sc.liveForwardCount = 0
			if st.Handoff != journal.HandoffHeld {
				s.setHandoffLocked(st, journal.HandoffHeld, fmt.Sprintf(
					"destination slot %d is dark and no proof of non-delivery arrived",
					st.Entry.DestSlot))
				s.log.Warn("sidecar: holding a forwarded organism whose destination went dark",
					"migrationId", st.Entry.MigrationID, "destSlot", st.Entry.DestSlot,
					"accruedHold", time.Duration(st.AccruedHoldMs)*time.Millisecond,
					"holdTimeout", s.cfg.HoldTimeout)
			}
			if s.accrueHoldLocked(st, sc, now) {
				// The bounded hold expired and the organism went home.
				return
			}
		}
		// Retry the RECORDED destSlot on the retry cadence, live or dark (§9.2
		// row 4). The retry is idempotent because the destination deduplicates —
		// and while the destination is dark the retry is also the only way the
		// relay's answer, and with it the only possible proof of non-delivery,
		// ever reaches this sender.
		if now.Before(sc.nextForward) {
			return
		}
		if s.forwardLocked(st, now) {
			if live {
				// A LIVE peer that has not answered is slow — a deep paced backlog
				// (Risk 9) — not gone. Back the re-forward off exponentially so a
				// silent-but-live destination is not hammered every forwardRetryMs
				// forever, which is the shape that produced 203 735 duplicate
				// deliveries into the stalled slot 6.
				interval := backoffInterval(s.cfg.ForwardRetry, s.cfg.ForwardRetryMax, sc.liveForwardCount)
				sc.nextForward = now.Add(interval)
				if interval < s.cfg.ForwardRetryMax {
					sc.liveForwardCount++
				}
			} else {
				// The dark cadence is flat and deliberately untouched (§9.3, B5):
				// the hold clock accrues against exactly this retry, and it is the
				// only conduit the non-delivery proof has.
				sc.nextForward = now.Add(s.cfg.ForwardRetry)
			}
		}

	case journal.HandoffPending, journal.HandoffRefused:
		// A never-sent or re-routed entry is a state change: reset the live-forward
		// backoff so the destination it is (re-)sent to starts from forwardRetryMs.
		sc.liveForwardCount = 0
		s.stopHoldLocked(st, sc, now)
		if live {
			sc.bounceAt = time.Time{}
			if now.Before(sc.nextForward) {
				return
			}
			if s.forwardLocked(st, now) {
				sc.nextForward = now.Add(s.cfg.ForwardRetry)
			}
			return
		}
		// No custody can have moved, so the entry may be re-routed along the
		// same axis to the current effective neighbour (§9.2).
		proof := contractb.ProofNeverSent
		if st.Handoff == journal.HandoffRefused {
			proof = st.RerouteProof
			if proof == "" {
				proof = contractb.ProofPeerRefused
			}
		}
		if s.rerouteLocked(st, proof, now) {
			sc.bounceAt = time.Time{}
			sc.nextForward = time.Time{}
			return
		}
		// It needs a lane. With no effective neighbour on that axis there is
		// nowhere to re-route to: the entry waits, and bounces home after
		// bounceTimeoutMs — M3's rule, unchanged, and now reached only when
		// route-around has no answer either.
		if sc.bounceAt.IsZero() {
			sc.bounceAt = now.Add(s.cfg.BounceTimeout)
			return
		}
		if now.After(sc.bounceAt) {
			s.bounceLocked(st, "no deliverable slot on the "+st.Entry.Edge+
				" axis within bounceTimeoutMs, and no custody was ever taken", false)
		}
	}
}

// backoffInterval doubles base, up to max, n times — the exponential backoff of
// §9.3/B5 for a re-forward to a live-but-silent destination. n is how many live
// re-forwards this entry has already spent. The loop stops at max, so a large n
// never overflows the Duration.
func backoffInterval(base, max time.Duration, n int) time.Duration {
	d := base
	for i := 0; i < n && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	return d
}

// destLiveLocked answers §9.3's third condition from the latest PEER_STATUS:
// is the destination slot live? A slot ABSENT FROM THE MAP counts as dark, and
// so does a slot this sidecar has no status for yet.
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

// accrueHoldLocked runs §9.3's clock. It advances ONLY while all three hold:
// the entry is in held, this sidecar has a live relay connection, and the
// destination is not live or is absent from the map.
//
// It never counts time while the destination is LIVE — a live peer with a deep
// paced backlog is slow, not orphaned (Risk 9) — and it never counts time while
// the SENDER is blind, because a sender whose machine slept for a night never
// observed the destination dark, it only failed to observe anything.
// It returns true when the timeout expired and the organism was bounced home.
func (s *Sidecar) accrueHoldLocked(st *journal.State, sc *sched, now time.Time) bool {
	if !s.relayReady || s.destLiveLocked(st.Entry.DestSlot) {
		// A condition stopped holding. Bank what was served and stop the clock.
		s.stopHoldLocked(st, sc, now)
		return false
	}
	if sc.holdSince.IsZero() {
		sc.holdSince = now
		if sc.lastFlush.IsZero() {
			sc.lastFlush = now
		}
		return false
	}
	sc.pendingAccrual += now.Sub(sc.holdSince)
	sc.holdSince = now

	total := time.Duration(st.AccruedHoldMs)*time.Millisecond + sc.pendingAccrual
	if total >= s.cfg.HoldTimeout {
		s.commitAccrualLocked(st, sc, total)
		s.bounceLocked(st, fmt.Sprintf(
			"the bounded hold expired: %s of accrued dark time against slot %d, over holdTimeoutMs (%s)",
			total.Truncate(time.Second), st.Entry.DestSlot, s.cfg.HoldTimeout), true)
		return true
	}
	if now.Sub(sc.lastFlush) >= s.cfg.HoldAccrualFlush {
		s.commitAccrualLocked(st, sc, total)
		sc.lastFlush = now
	}
	return false
}

// stopHoldLocked banks any un-flushed accrual and stops the clock.
func (s *Sidecar) stopHoldLocked(st *journal.State, sc *sched, now time.Time) {
	if !sc.holdSince.IsZero() {
		sc.pendingAccrual += now.Sub(sc.holdSince)
		sc.holdSince = time.Time{}
	}
	if sc.pendingAccrual > 0 {
		s.commitAccrualLocked(st, sc, time.Duration(st.AccruedHoldMs)*time.Millisecond+sc.pendingAccrual)
		sc.lastFlush = now
	}
}

func (s *Sidecar) commitAccrualLocked(st *journal.State, sc *sched, total time.Duration) {
	ms := total.Milliseconds()
	if _, err := s.jr.Apply(st.Entry.MigrationID, journal.Update{AccruedHoldMs: &ms}); err != nil {
		s.log.Error("sidecar: hold accrual flush failed", "migrationId", st.Entry.MigrationID, "err", err)
		return
	}
	st.AccruedHoldMs = ms
	sc.pendingAccrual = 0
}

// flushAccrualsLocked is the clean-shutdown half of §9.3's durability rule.
func (s *Sidecar) flushAccrualsLocked() {
	now := s.now()
	for _, st := range s.jr.List() {
		if st.Direction != journal.Out || st.Handoff != journal.HandoffHeld {
			continue
		}
		sc, ok := s.sched[st.Entry.MigrationID]
		if !ok {
			continue
		}
		s.stopHoldLocked(st, sc, now)
	}
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

// rerouteLocked is §7.3's ONE exception to the no-rewrite rule, and it carries
// its own evidence. Only destSlot is rewritten; the migrationId, the axis, the
// exit geometry, the annex and the body are the same bytes.
func (s *Sidecar) rerouteLocked(st *journal.State, proof string, now time.Time) bool {
	if st.Handoff.CustodyMayHaveMoved() {
		// Belt and braces: the caller already checked, and a re-route from here
		// would be the duplication D2 refuses.
		return false
	}
	if st.RerouteCount >= s.cfg.MaxReroutes {
		s.bounceLocked(st, fmt.Sprintf(
			"maxReroutes (%d) reached; an organism circling a broken axis is a symptom, not a delivery strategy",
			s.cfg.MaxReroutes), false)
		return true
	}
	n := s.neighbours[st.Entry.Edge]
	if n == nil || n.Slot == st.Entry.DestSlot {
		return false
	}
	from := st.RerouteFrom
	if st.RerouteCount == 0 {
		from = st.Entry.DestSlot
	}
	count := st.RerouteCount + 1
	dest := n.Slot
	at := now.UnixMilli()
	empty := ""
	if _, err := s.jr.Apply(st.Entry.MigrationID, journal.Update{
		Handoff:        journal.HandoffPending,
		DestSlot:       &dest,
		RerouteCount:   &count,
		RerouteFrom:    &from,
		RerouteProof:   &proof,
		RerouteAtMs:    &at,
		RelaySessionID: &empty,
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
		if st.AckedUpstream || st.BounceBack || st.Entry.SourcePeer == "" {
			return
		}
		sc := s.schedFor(id)
		if now.Before(sc.nextForward) {
			return
		}
		sc.nextForward = now.Add(s.cfg.ForwardRetry)
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
	payload := contractb.MigrationPayload{
		MigrationID: st.Entry.MigrationID,
		Kind:        st.Entry.Kind,
		Body:        contractb.Body{Version: st.Entry.GameVersion, BB8: st.Entry.Payload},
		Lineage:     lineageOf(st.Entry),
		// COPY, NEVER AUTHOR (§6.6). The block goes out exactly as the journal
		// holds it, which is exactly as MIGRATE_OUT delivered it. A re-forward, a
		// re-route and a retry all reproduce the same bytes, because they all read
		// the same journal entry.
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
	if !s.sendRelayLocked(contractb.TypeMigrationPayload, payload) {
		return false
	}
	// §9.2: pending -> sent, RECORDING THE relaySessionId IN FORCE AT THIS FIRST
	// WRITE. That id is what scopes every later proof: a link flap keeps it and
	// keeps the proof, a relay restart changes it and the sender falls back to
	// holding.
	//
	// A HELD entry keeps its state through a retry. The retry is how the relay's
	// answer reaches this sender at all, and treating it as a fresh hand-off
	// would reset the very clock §9.3 exists to accrue.
	if st.Handoff != journal.HandoffSent && st.Handoff != journal.HandoffHeld {
		session := s.relaySessionID
		u := journal.Update{Handoff: journal.HandoffSent}
		if st.RelaySessionID == "" {
			u.RelaySessionID = &session
		}
		if st.Status != journal.StatusInFlight {
			u.Status = journal.StatusInFlight
		}
		if _, err := s.jr.Apply(st.Entry.MigrationID, u); err != nil {
			s.log.Error("sidecar: journal update failed", "migrationId", st.Entry.MigrationID, "err", err)
		} else {
			st.Handoff = journal.HandoffSent
			if st.RelaySessionID == "" {
				st.RelaySessionID = session
			}
		}
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
func (s *Sidecar) bounceLocked(st *journal.State, why string, timeout bool) {
	id := st.Entry.MigrationID
	bounce := true
	attempt := 0
	u := journal.Update{
		Status:     journal.StatusOpen,
		Direction:  journal.In,
		BounceBack: &bounce,
		Attempt:    &attempt,
		Handoff:    journal.HandoffDone,
		Note:       "bounced: " + why,
	}
	if timeout {
		u.BouncedTimeout = contractb.BoolPtr(true)
	}
	if _, err := s.jr.Apply(id, u); err != nil {
		s.log.Error("sidecar: bounce journal update failed", "migrationId", id, "err", err)
		return
	}
	delete(s.sched, id)
	if timeout {
		s.bouncedTimeoutTotal++
		// §9.3: an automatic bounce is a FACT THE OPERATOR READS, not a silent
		// repair.
		s.log.Error("sidecar: BOUNDED HOLD EXPIRED, bouncing the organism home by itself",
			"migrationId", id, "entityId", st.Entry.EntityID, "destSlot", st.Entry.DestSlot,
			"accruedHold", time.Duration(st.AccruedHoldMs)*time.Millisecond,
			"bouncedTimeoutTotal", s.bouncedTimeoutTotal, "why", why)
		return
	}
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
	// pacer's own idle grace (pacingIdleGraceMs, 10 s) is LONGER than the 3.5 s
	// heartbeat timeout, so during a stall the socket is closed with 4004 before
	// that branch can trip — which left this function releasing MIGRATE_IN frames
	// right up to the close, into a mod whose main thread was already drowning.
	// Gate on heartbeat freshness instead: a mod whose last app-level HEARTBEAT
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
	st := contractb.PeerStats{
		CustodyDepth:        contractb.IntPtr(s.jr.CountPending(journal.Out) + s.jr.CountPending(journal.In)),
		PacedDepth:          contractb.IntPtr(s.pacedDepthLocked()),
		HeldDepth:           contractb.IntPtr(s.heldDepthLocked()),
		BouncedTimeoutTotal: contractb.IntPtr(s.bouncedTimeoutTotal),
		// The delivery rate limit this sidecar is CONFIGURED with (§18, B16).
		// Always known, because it is this process's own setting and not a
		// reading of anything else — and it is what makes pacedDepth readable:
		// a queue is only deep against the cap it is queued behind. The default
		// has moved three times, so a reader that assumes one is wrong.
		InboundRatePerSimMinute: contractb.Float64Ptr(s.cfg.InboundRatePerSimMinute),
		InboundRateBurst:        contractb.Float64Ptr(s.cfg.InboundRateBurst),
	}
	if s.mod != nil && s.mod.handshaked {
		if s.mod.haveTimeScale {
			// Copied from the HEARTBEAT, never computed. 0 is a world standing
			// still, which is a reading; absent is a world that has not said.
			st.TimeScale = contractb.Float64Ptr(s.mod.timeScale)
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

// heldDepthLocked is the number of outbound entries in the held state of §9.2 —
// forwarded, unproven, destination dark.
func (s *Sidecar) heldDepthLocked() int {
	n := 0
	for _, st := range s.jr.List() {
		if st.Direction == journal.Out && st.Handoff == journal.HandoffHeld &&
			(st.Status == journal.StatusOpen || st.Status == journal.StatusInFlight) {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------- operator

// InflightEntry is one row of --list-inflight (§7.5).
type InflightEntry struct {
	MigrationID  string
	EntityID     int32
	Direction    string
	Status       string
	Handoff      string
	DestSlot     int
	ExitEdge     string
	AccruedHold  time.Duration
	Deadline     time.Duration
	Reroutes     int
	RerouteFrom  int
	RerouteProof string
	RelaySession string
	JournaledAt  time.Time
	Note         string
}

// ListInflight answers the question the relay CANNOT: which entries name this
// slot, and what are they (§7.5). It runs on the machine that owns the journal,
// because D2 keeps custody local.
func ListInflight(dataDir string, destSlot int, holdTimeout time.Duration) ([]InflightEntry, error) {
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
		accrued := time.Duration(st.AccruedHoldMs) * time.Millisecond
		out = append(out, InflightEntry{
			MigrationID:  st.Entry.MigrationID,
			EntityID:     st.Entry.EntityID,
			Direction:    string(st.Direction),
			Status:       string(st.Status),
			Handoff:      string(st.Handoff),
			DestSlot:     st.Entry.DestSlot,
			ExitEdge:     st.Entry.Edge,
			AccruedHold:  accrued,
			Deadline:     holdTimeout - accrued,
			Reroutes:     st.RerouteCount,
			RerouteFrom:  st.RerouteFrom,
			RerouteProof: st.RerouteProof,
			RelaySession: st.RelaySessionID,
			JournaledAt:  time.UnixMilli(st.Entry.JournaledAt),
			Note:         st.Note,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].JournaledAt.Before(out[j].JournaledAt) })
	return out, nil
}

// ReleaseInflight is the manual escape hatch of §9.3: it releases one held
// entry by hand, before the timeout expires. It is the custody twin of
// --release-slot, and it runs on the machine that holds the journal.
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

An entry in handoff "sent" or "held" WAS written to a live relay connection, so
the far sidecar may already hold custody of this organism. If it does, and it
returns and replays its own journal after you bounce this one home, THE MAP
HOLDS TWO COPIES. That is the one exception at-most-once carries (§9.3), and
this command is one of the two ways to fire it deliberately.

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
