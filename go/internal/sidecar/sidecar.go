// Package sidecar implements multiverse-sidecar: the Contract A server for the
// mod, the Contract B client for the relay, and the durable custody chain of
// decision D2 that joins them.
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
	"strings"
	"sync"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

// Sidecar is one peer node's network brain.
type Sidecar struct {
	cfg Config
	log *slog.Logger
	jr  *journal.Journal

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

	// Contract B state.
	relayConn  *wsutil.Conn
	relayReady bool
	sector     string
	peers      map[string]contractb.PeerInfo // by sector
	peerEpoch  int64

	// Custody scheduling, in memory. The durable half lives in the journal.
	sched        map[string]*sched
	seenSessions map[string]bool
	lastPurge    time.Time
	closed       bool
}

type sched struct {
	nextForward time.Time
	bounceAt    time.Time
	nextDeliver time.Time
	reachedPeer bool
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
	s := &Sidecar{
		cfg:          cfg,
		log:          cfg.Logger.With("peer", cfg.PeerID),
		jr:           jr,
		peers:        map[string]contractb.PeerInfo{},
		sched:        map[string]*sched{},
		seenSessions: map[string]bool{},
	}
	if cfg.PreferredSector == "" {
		s.cfg.PreferredSector = s.readSector()
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
		"relay", s.cfg.RelayURL, "dataDir", s.cfg.DataDir)
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

// CustodySnapshot returns a copy of every journal state, in journal order. It
// is the read model behind an operator status view, and it is what the contract
// tests assert custody against.
func (s *Sidecar) CustodySnapshot() []*journal.State { return s.jr.List() }

// Sector is the sector the relay granted, or "" when none is held.
func (s *Sidecar) Sector() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sector
}

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
		// contract-a.md §2: the sidecar MUST NOT bind 0.0.0.0.
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
	defer s.mu.Unlock()

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
	// contract-b-m2.md §7: bounce only when the payload never reached a live
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
		s.bounceLocked(st, "no live peer for the destination sector within bounceTimeoutMs")
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

// routeSector maps one of this sidecar's edges to the destination sector.
// M2's map is two sectors side by side: A's east edge pairs with B's west edge
// (contract-b-m2.md §1).
func routeSector(mySector, edge string) (string, bool) {
	switch {
	case mySector == contractb.SectorA && edge == contracta.EdgeE:
		return contractb.SectorB, true
	case mySector == contractb.SectorB && edge == contracta.EdgeW:
		return contractb.SectorA, true
	}
	return "", false
}

// sectorXY is the {x,y} form of a sector, for the advisory CONFIG_UPDATE.sector
// check of contract-a.md §5.1.
func sectorXY(sector string) (contracta.Sector, bool) {
	switch sector {
	case contractb.SectorA:
		return contracta.Sector{X: 0, Y: 0}, true
	case contractb.SectorB:
		return contracta.Sector{X: 1, Y: 0}, true
	}
	return contracta.Sector{}, false
}

// sameSize compares two half-extents. contract-a.md §4.1 forbids exact float
// equality, so this is a relative comparison.
func sameSize(a, b float64) bool {
	if !wire.Finite(a) || !wire.Finite(b) {
		return false
	}
	return math.Abs(a-b) <= 1e-6*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

func (s *Sidecar) computeEdgesLocked() []contracta.EdgeState {
	if s.mod == nil || !s.mod.handshaked {
		return nil
	}
	out := make([]contracta.EdgeState, 0, len(s.mod.borderEdges))
	for _, edge := range s.mod.borderEdges {
		st := contracta.EdgeState{Edge: edge, Open: false, Reason: contracta.ReasonNoPeer}
		dest, routable := routeSector(s.sector, edge)
		switch {
		case !s.relayReady:
			st.Reason = contracta.ReasonPeerUnreachable
		case s.sector == "":
			st.Reason = contracta.ReasonPeerUnreachable
		case !routable:
			// The mod declared an edge the M2 sector map has no neighbour for.
			st.Reason = contracta.ReasonNoPeer
		default:
			peer, live := s.peers[dest]
			switch {
			case !live:
				st.Reason = contracta.ReasonNoPeer
			case !peer.ModConnected:
				st.Reason = contracta.ReasonPeerUnreachable
			case !sameSize(peer.SimulationSize, s.mod.simSize):
				st.Reason = contracta.ReasonSimSizeMismatch
			default:
				size := peer.SimulationSize
				st.Open = true
				st.Reason = contracta.ReasonPeerLive
				st.PeerSimulationSize = &size
			}
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

// edgeOpenLocked reports whether migration out of edge is permitted right now,
// and the NACK code to use when it is not.
func (s *Sidecar) edgeOpenLocked(edge string, simSize float64) (dest string, code string, message string) {
	if s.mod == nil {
		return "", contracta.OutEdgeClosed, "no mod session"
	}
	declared := false
	for _, e := range s.mod.borderEdges {
		if e == edge {
			declared = true
			break
		}
	}
	if !declared {
		// contract-a.md §5.1: the sidecar MUST NOT open an edge absent from
		// borderEdges.
		return "", contracta.OutEdgeClosed, "edge " + edge + " is not in the mod's borderEdges"
	}
	if !s.relayReady || s.sector == "" {
		return "", contracta.OutNoRoute, "no relay link or no sector assignment yet"
	}
	dest, routable := routeSector(s.sector, edge)
	if !routable {
		return "", contracta.OutEdgeClosed, "sector " + s.sector + " has no neighbour on edge " + edge
	}
	peer, live := s.peers[dest]
	if !live {
		return "", contracta.OutEdgeClosed, "sector " + dest + " is vacant"
	}
	if !peer.ModConnected {
		return "", contracta.OutEdgeClosed, "peer " + peer.PeerID + " has no mod connected"
	}
	if !sameSize(peer.SimulationSize, simSize) {
		return "", contracta.OutSimSizeMismatch,
			fmt.Sprintf("peer simulationSize %v differs from %v", peer.SimulationSize, simSize)
	}
	return dest, "", ""
}

// ---------------------------------------------------------------- custody

// canForwardLocked reports whether an outbound entry can go out right now.
func (s *Sidecar) canForwardLocked(st *journal.State) bool {
	if !s.relayReady || s.sector == "" {
		return false
	}
	peer, live := s.peers[st.Entry.DestSector]
	if !live {
		return false
	}
	return sameSize(peer.SimulationSize, st.Entry.SimulationSize)
}

func (s *Sidecar) forwardLocked(st *journal.State) bool {
	payload := contractb.MigrationPayload{
		MigrationID:  st.Entry.MigrationID,
		Kind:         st.Entry.Kind,
		Body:         contractb.Body{Version: st.Entry.GameVersion, BB8: st.Entry.Payload},
		SourcePeer:   s.cfg.PeerID,
		SourceSector: s.sector,
		DestSector:   st.Entry.DestSector,
		ExitEdge:     st.Entry.Edge,
		ExitPosition: st.Entry.Position,
		Velocity:     contractb.Vec{X: st.Entry.VelocityX, Y: st.Entry.VelocityY},
		Timestamp:    time.Now().UnixMilli(),
		EntityID:     st.Entry.EntityID,
		Heading:      st.Entry.Heading,
	}
	if !s.sendRelayLocked(contractb.TypeMigrationPayload, payload) {
		return false
	}
	s.log.Info("sidecar: forwarded MIGRATION_PAYLOAD",
		"migrationId", st.Entry.MigrationID, "destSector", st.Entry.DestSector)
	s.faultPoint(FaultPostForward)
	return true
}

// bounceLocked re-injects an outbound organism into the local sim. The journal
// entry keeps its migrationId and flips direction, so dedup still holds
// end to end (custody chain step 6).
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
		"bounceBack", updated.BounceBack)
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

func (s *Sidecar) readSector() string {
	b, err := os.ReadFile(filepath.Join(s.cfg.DataDir, "sector"))
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(b))
	if contractb.ValidSector(v) {
		return v
	}
	return ""
}

func (s *Sidecar) writeSector(sector string) {
	if sector == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(s.cfg.DataDir, "sector"), []byte(sector+"\n"), 0o644)
}
