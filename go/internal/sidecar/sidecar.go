// Package sidecar implements multiverse-sidecar: the Contract A server for the
// mod, the Contract B client for the relay, and the durable custody chain of
// decision D2 that joins them.
//
// Under M3 the sidecar also owns the lineage annex (D11): it hashes the
// migrant's genome and every parent blob the mod ships, caches them by hash,
// strips the blobs from the wire, and answers a later GENOME_REQUEST out of
// that cache. D4 stays intact — the mod never parses a genome, and the sidecar
// never trusts a mod-supplied hash.
package sidecar

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"multiverse/internal/bb8"
	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
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

	// Contract B state: the ring. A sidecar needs exactly two facts — its own
	// slot and its east neighbour (D8, contract-b-m3.md §6.4).
	relayConn  *wsutil.Conn
	relayReady bool
	slot       int
	ringSize   int
	east       *contractb.Neighbour
	// ring is the last PEER_STATUS's slot list, in ring order. It exists for
	// one job: deciding whether a journaled entry's recorded destSlot has a
	// live peer right now (§7.3, §9).
	ring      []contractb.SlotInfo
	peerEpoch int64

	// Custody scheduling, in memory. The durable half lives in the journal.
	sched        map[string]*sched
	seenSessions map[string]bool
	lastPurge    time.Time
	lastSweep    time.Time
	// genomeServed counts GENOME_REQUESTs answered per requester in the
	// current minute (contract-b-m3.md §10's rate limit, answering side).
	genomeServed map[string]*rateWindow
	closed       bool
}

type sched struct {
	nextForward time.Time
	bounceAt    time.Time
	nextDeliver time.Time
	reachedPeer bool
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
		sched:        map[string]*sched{},
		seenSessions: map[string]bool{},
		genomeServed: map[string]*rateWindow{},
	}
	// §7.4: peerId is persisted outside the journal. Losing it makes the peer a
	// stranger that takes a second slot and strands its old one — which is why
	// §7.5 gives the operator a release command.
	s.cfg.PeerID = s.resolvePeerID(cfg.PeerID)
	s.log = s.cfg.Logger.With("peer", s.cfg.PeerID)
	if s.cfg.PreferredSlot == 0 {
		s.cfg.PreferredSlot = s.readSlot()
	}
	pendingOut := jr.CountPending(journal.Out)
	pendingIn := jr.CountPending(journal.In)
	if pendingOut+pendingIn > 0 {
		s.log.Warn("sidecar: recovered custody from the journal",
			"outbound", pendingOut, "inbound", pendingIn)
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
		"relay", s.cfg.RelayURL, "dataDir", s.cfg.DataDir, "preferredSlot", s.cfg.PreferredSlot)
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

// CustodySnapshot returns a copy of every journal state, in journal order. It
// is the read model behind an operator status view, and it is what the contract
// tests assert custody against.
func (s *Sidecar) CustodySnapshot() []*journal.State { return s.jr.List() }

// Slot is the ring slot the relay granted, or 0 when none is held.
func (s *Sidecar) Slot() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.slot
}

// EastNeighbour is the one slot this sidecar exports into, or nil.
func (s *Sidecar) EastNeighbour() *contractb.Neighbour {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.east == nil {
		return nil
	}
	e := *s.east
	return &e
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

// ---------------------------------------------------------------- scheduling

func (s *Sidecar) tickLoop() {
	t := time.NewTicker(s.cfg.TickInterval)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-t.C:
			s.tick(now)
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
	s.mu.Unlock()

	if sweep {
		// contract-b-m3.md §10: the cache expires by genomeCacheRetentionDays,
		// least recently served, bounded by genomeCacheMaxBytes.
		if n, err := s.genomes.Sweep(s.cfg.GenomeCacheRetention, s.cfg.GenomeCacheMaxBytes); err != nil {
			s.log.Warn("sidecar: genome cache sweep failed", "err", err)
		} else if n > 0 {
			s.log.Info("sidecar: evicted genomes from the cache", "count", n)
		}
	}
}

func (s *Sidecar) tickOutbound(st *journal.State, now time.Time) {
	if st.Status != journal.StatusOpen && st.Status != journal.StatusInFlight {
		return
	}
	sc := s.schedFor(st.Entry.MigrationID)
	if s.canForwardLocked(st) {
		sc.bounceAt = time.Time{}
		if now.Before(sc.nextForward) {
			return
		}
		if s.forwardLocked(st) {
			sc.reachedPeer = true
			sc.nextForward = now.Add(s.cfg.ForwardRetry)
			if st.Status != journal.StatusInFlight {
				if _, err := s.jr.Apply(st.Entry.MigrationID, journal.Update{Status: journal.StatusInFlight}); err != nil {
					s.log.Error("sidecar: journal update failed", "migrationId", st.Entry.MigrationID, "err", err)
				}
			}
		}
		return
	}
	// contract-b-m3.md §9: bounce only when the payload never reached a live
	// peer. Anything else is held and re-forwarded, because a lost ACK plus a
	// bounce is a duplicated organism, which D2 rules out.
	if sc.reachedPeer {
		return
	}
	if sc.bounceAt.IsZero() {
		sc.bounceAt = now.Add(s.cfg.BounceTimeout)
		return
	}
	if now.After(sc.bounceAt) {
		s.bounceLocked(st, "no live peer for the destination ring slot within bounceTimeoutMs")
	}
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
func sameSize(a, b float64) bool {
	if !wire.Finite(a) || !wire.Finite(b) {
		return false
	}
	return math.Abs(a-b) <= 1e-6*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

// edgeStateLocked is contract-b-m3.md §8's table: the exact mapping from the
// relay's ring view to the one export edge this sidecar tells its mod about.
// The order of the tests is the order of the table.
func (s *Sidecar) edgeStateLocked() (open bool, reason string, peerSize float64) {
	switch {
	case !s.relayReady:
		return false, contracta.ReasonPeerUnreachable, 0
	case s.slot == 0 || s.ringSize <= 1:
		// At ringSize 1 a peer's east neighbour would be itself, and §2 forbids
		// granting that.
		return false, contracta.ReasonNoPeer, 0
	case s.east == nil:
		return false, contracta.ReasonNoPeer, 0
	case !s.east.Live:
		return false, contracta.ReasonNoPeer, 0
	case !s.east.ModConnected:
		// A dead sim must not keep receiving organisms (§8).
		return false, contracta.ReasonPeerModAbsent, 0
	case s.mod != nil && s.mod.handshaked && s.east.GameVersion != "" &&
		s.east.GameVersion != s.mod.gameVersion:
		return false, contracta.ReasonPeerIncompatible, 0
	case s.mod != nil && s.mod.handshaked && !sameSize(s.east.SimulationSize, s.mod.simSize):
		return false, contracta.ReasonSimSizeMismatch, 0
	default:
		return true, contracta.ReasonPeerLive, s.east.SimulationSize
	}
}

// computeEdgesLocked builds EDGE_STATUS.edges. Under the ring it has exactly
// one entry, for the mod's exportEdge; the entry edge is passive and never
// appears (contract-a.md §14, A11). An empty array closes the export edge and
// is the correct frame when this sidecar holds no ring slot.
func (s *Sidecar) computeEdgesLocked() []contracta.EdgeState {
	if s.mod == nil || !s.mod.handshaked || s.mod.exportEdge == "" {
		return nil
	}
	open, reason, peerSize := s.edgeStateLocked()
	st := contracta.EdgeState{Edge: s.mod.exportEdge, Open: open, Reason: reason}
	if open {
		size := peerSize
		st.PeerSimulationSize = &size
	}
	return []contracta.EdgeState{st}
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
// now, and the destination ring slot. A closed edge comes back with the
// MIGRATE_OUT_NACK code contract-a.md §9.1 assigns.
func (s *Sidecar) exportOpenLocked(edge string, simSize float64) (destSlot int, code string, message string) {
	if s.mod == nil || !s.mod.handshaked {
		return 0, contracta.OutEdgeClosed, "no mod session"
	}
	if edge != s.mod.exportEdge {
		// §14 A11: the sidecar MUST NOT open any edge but exportEdge, whatever
		// borderEdges says.
		return 0, contracta.OutEdgeClosed,
			"edge " + edge + " is not this sim's exportEdge " + s.mod.exportEdge
	}
	if !s.relayReady {
		return 0, contracta.OutNoRoute, "no relay link"
	}
	if s.slot == 0 {
		return 0, contracta.OutNoRoute, "no ring slot granted yet"
	}
	open, reason, _ := s.edgeStateLocked()
	if !open {
		switch reason {
		case contracta.ReasonSimSizeMismatch:
			return 0, contracta.OutSimSizeMismatch,
				fmt.Sprintf("east neighbour simulationSize %v differs from %v", s.east.SimulationSize, simSize)
		case contracta.ReasonPeerIncompatible:
			return 0, contracta.OutPeerIncompatible,
				"east neighbour runs gameVersion " + s.east.GameVersion
		case contracta.ReasonPeerUnreachable:
			return 0, contracta.OutNoRoute, "no relay link"
		default:
			return 0, contracta.OutEdgeClosed, "the export edge is closed: " + reason
		}
	}
	if !sameSize(s.east.SimulationSize, simSize) {
		return 0, contracta.OutSimSizeMismatch,
			fmt.Sprintf("east neighbour simulationSize %v differs from %v", s.east.SimulationSize, simSize)
	}
	return s.east.Slot, "", ""
}

// ---------------------------------------------------------------- custody

// slotInfoLocked looks one ring slot up in the last PEER_STATUS.
func (s *Sidecar) slotInfoLocked(slot int) (contractb.SlotInfo, bool) {
	for _, si := range s.ring {
		if si.Slot == slot {
			return si, true
		}
	}
	return contractb.SlotInfo{}, false
}

// canForwardLocked reports whether an outbound entry can go out right now.
//
// It tests the recorded DestSlot against the ring, never the current east
// neighbour: §7.3 says an insertion applies to new migrations only, a journaled
// entry keeps the destination it recorded, and routing is on the slot. A
// destination that is vacant or gone is what starts the bounce timer (§9).
func (s *Sidecar) canForwardLocked(st *journal.State) bool {
	if !s.relayReady || s.slot == 0 || st.Entry.DestSlot == 0 {
		return false
	}
	si, ok := s.slotInfoLocked(st.Entry.DestSlot)
	if !ok || !si.Live {
		return false
	}
	if si.SimulationSize > 0 && st.Entry.SimulationSize > 0 &&
		!sameSize(si.SimulationSize, st.Entry.SimulationSize) {
		return false
	}
	return true
}

func (s *Sidecar) forwardLocked(st *journal.State) bool {
	payload := contractb.MigrationPayload{
		MigrationID: st.Entry.MigrationID,
		Kind:        st.Entry.Kind,
		Body:        contractb.Body{Version: st.Entry.GameVersion, BB8: st.Entry.Payload},
		Lineage:     lineageOf(st.Entry),
		SourcePeer:  s.cfg.PeerID,
		SourceSlot:  st.Entry.SourceSlot,
		DestSlot:    st.Entry.DestSlot,
		ExitEdge:    st.Entry.Edge,
		// contract-a.md §4.3: the sidecar copies exitPosition and never
		// converts it. The mod owns the geometry.
		ExitPosition: st.Entry.Position,
		Velocity:     contractb.Vec{X: st.Entry.VelocityX, Y: st.Entry.VelocityY},
		Heading:      st.Entry.Heading,
		EntityID:     st.Entry.EntityID,
		Timestamp:    time.Now().UnixMilli(),
	}
	if !s.sendRelayLocked(contractb.TypeMigrationPayload, payload) {
		return false
	}
	s.log.Info("sidecar: forwarded MIGRATION_PAYLOAD",
		"migrationId", st.Entry.MigrationID, "destSlot", st.Entry.DestSlot,
		"genomeHash", st.Entry.GenomeHash, "parents", len(st.Entry.Parents))
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
// entry keeps its migrationId and flips direction, so dedup still holds
// end to end (custody chain step 6). It comes home on the edge it left from —
// which under the ring is this sim's own exportEdge, not its passive entry edge
// (contract-a.md §14, A11).
func (s *Sidecar) bounceLocked(st *journal.State, why string) {
	id := st.Entry.MigrationID
	bounce := true
	attempt := 0
	_, err := s.jr.Apply(id, journal.Update{
		Status:     journal.StatusOpen,
		Direction:  journal.In,
		BounceBack: &bounce,
		Attempt:    &attempt,
		Note:       "bounced: " + why,
	})
	if err != nil {
		s.log.Error("sidecar: bounce journal update failed", "migrationId", id, "err", err)
		return
	}
	delete(s.sched, id)
	s.log.Warn("sidecar: bouncing organism back into the local sim",
		"migrationId", id, "entityId", st.Entry.EntityID, "why", why)
}

func (s *Sidecar) deliverLocked(st *journal.State, now time.Time) {
	id := st.Entry.MigrationID
	attempt := st.Attempt + 1
	updated, err := s.jr.Apply(id, journal.Update{Status: journal.StatusInFlight, Attempt: &attempt})
	if err != nil {
		s.log.Error("sidecar: delivery journal update failed", "migrationId", id, "err", err)
		return
	}
	msg := contracta.MigrateIn{
		MigrationID:   id,
		EntityID:      updated.Entry.EntityID,
		Kind:          updated.Entry.Kind,
		GameVersion:   updated.Entry.GameVersion,
		Payload:       updated.Entry.Payload,
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
		"entryEdge", updated.Entry.Edge, "bounceBack", updated.BounceBack)
}

func (s *Sidecar) ackUpstreamLocked(st *journal.State) {
	ack := contractb.MigrationAck{
		MigrationID: st.Entry.MigrationID,
		SourcePeer:  s.cfg.PeerID,
		DestPeer:    st.Entry.SourcePeer,
		EntityID:    st.Entry.EntityID,
		Duplicate:   st.Duplicate,
		DeliveredAt: time.Now().UnixMilli(),
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
// and never fails a migration (contract-b-m3.md §6.6 step 6).
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

// ---------------------------------------------------------------- persistence

// resolvePeerID returns the identity this sidecar keeps for life. An explicit
// --peer-id wins and is written down; otherwise the persisted one is reused;
// otherwise one is generated on first start (contract-b-m3.md §7.4).
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
				"a restart would take a second ring slot and strand this one", "err", err)
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
