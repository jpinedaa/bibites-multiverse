// Package relay implements multiverse-relay: the deliberately dumb M4
// transport (D1). It forwards Contract B frames across a rectangular map of
// slots, arbitrates placement, computes the effective neighbour on each export
// edge, proves what it did and did not forward, copies every routed migration
// to the archive, and never parses a bb8 body or a lineage annex (D4).
//
// M4 gives the relay exactly two rules that are not frame forwarding: the
// effective-neighbour walk (§8) and the forwarding record behind the
// non-delivery proof (§5.2). It stays dumb in the sense that matters — it never
// parses a body, never validates a payload, never indexes a genome and never
// stores an organism. The walk reads the registry it already keeps; the record
// is a set of ids it already routed.
package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/lantoken"
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

// Version is reported in HANDSHAKE_ACK.
const Version = "m4.0"

// Options configures a Server.
type Options struct {
	Logger *slog.Logger
	// DataDir holds ring.json. Empty keeps the map in memory, which is what a
	// test rig wants and what production must never do (§7.4).
	DataDir         string
	Token           string
	InsecureNoToken bool
	PingInterval    time.Duration
	PeerTimeout     time.Duration
	// ArchiveQueue is the per-subscriber copy queue (§5.1).
	ArchiveQueue int
	// StatusCoalesce is the minimum spacing between PEER_STATUS broadcasts and
	// between grants to one peer (§7.2). The last frame of a burst is always
	// sent.
	StatusCoalesce time.Duration
	// StatsBroadcast republishes PEER_STATUS on a timer, because stats change
	// without the registry changing (§6.5).
	StatsBroadcast time.Duration
	// ForwardRecordRetention is how long a forwarded migrationId is remembered
	// for the neverForwarded proof (§5.2).
	ForwardRecordRetention time.Duration
}

// Server is the relay. The zero value is not usable; call New.
type Server struct {
	log             *slog.Logger
	token           string
	insecureNoToken bool
	pingInterval    time.Duration
	peerTimeout     time.Duration
	archiveQueue    int
	statusCoalesce  time.Duration
	statsBroadcast  time.Duration
	forwardRetain   time.Duration

	// sessionID is minted once at process start and constant for the life of it.
	// It is the scope of the forwarding record: a relay restart is exactly the
	// event that invalidates the proof, so nothing about it is durable (§5.2).
	sessionID string

	stopOnce sync.Once
	stop     chan struct{}
	wg       sync.WaitGroup

	mu   sync.Mutex
	grid *Grid
	// live connections by peer id, role "peer".
	peers map[string]*peer
	// live connections by peer id, role "archive".
	subscribers map[string]*peer
	// what the relay knows about a peer id, live or not. PEER_STATUS keeps
	// reporting a reserved slot after its peer goes away (§6.5).
	meta map[string]*peerMeta
	// dirty is set by any registry change and drained by the publisher, which is
	// what bounds the storm six peers, two axes and a flapping link can make.
	dirty    bool
	draining bool
	// forwarded is the §5.2 record: the migrationIds this PROCESS has attempted
	// to write to some peer's connection, with the time of the first attempt.
	forwarded map[string]time.Time
	// inherited names the peer ids that took a slot by operator handover, so
	// their first grant says so (§6.4's "handover" reason). It is consumed once.
	inherited map[string]bool
}

type peerMeta struct {
	gameVersion  string
	simSize      float64
	modConnected bool
	exportEdges  []string
	lastSeenMs   int64
	darkSinceMs  int64
	lastRefusal  string
	stats        *contractb.PeerStats
	statsAsOfMs  int64
}

type peer struct {
	id      string
	role    string
	conn    *wsutil.Conn
	claimed bool // a SECTOR_CLAIM was already answered on this connection

	mu        sync.Mutex
	epoch     int64
	lastGrant string // the last SECTOR_GRANT body, so a repeat is not re-sent
	// lastPos is the coordinate the last grant carried. A grant whose position
	// moved is a "repositioned", not an "updated": the map grew under this peer
	// and its neighbours changed with it (§6.4).
	lastPos *contractb.Position
	// out is the bounded copy queue of a subscriber (§5.1). It is nil for a
	// peer.
	out         chan []byte
	dropped     int64
	lastDropLog time.Time
}

// New builds a relay.
func New(opts Options) (*Server, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.PingInterval <= 0 {
		opts.PingInterval = contractb.RelayPingInterval
	}
	if opts.PeerTimeout <= 0 {
		opts.PeerTimeout = contractb.PeerTimeout
	}
	if opts.ArchiveQueue <= 0 {
		opts.ArchiveQueue = contractb.ArchiveQueueMax
	}
	if opts.StatusCoalesce <= 0 {
		opts.StatusCoalesce = contractb.StatusCoalesce
	}
	if opts.StatsBroadcast <= 0 {
		opts.StatsBroadcast = contractb.StatsBroadcastInterval
	}
	if opts.ForwardRecordRetention <= 0 {
		opts.ForwardRecordRetention = contractb.ForwardRecordRetention
	}
	if opts.Token == "" && !opts.InsecureNoToken {
		// §3.1: no token configured means the relay MUST refuse to start.
		return nil, errors.New("relay: no MULTIVERSE_TOKEN and no --token-file; " +
			"pass --insecure-no-token only for a single-machine test rig")
	}
	grid, err := LoadGrid(opts.DataDir)
	if err != nil {
		return nil, err
	}
	s := &Server{
		log:             opts.Logger,
		token:           opts.Token,
		insecureNoToken: opts.InsecureNoToken,
		pingInterval:    opts.PingInterval,
		peerTimeout:     opts.PeerTimeout,
		archiveQueue:    opts.ArchiveQueue,
		statusCoalesce:  opts.StatusCoalesce,
		statsBroadcast:  opts.StatsBroadcast,
		forwardRetain:   opts.ForwardRecordRetention,
		sessionID:       wire.NewUUID(),
		stop:            make(chan struct{}),
		grid:            grid,
		peers:           map[string]*peer{},
		subscribers:     map[string]*peer{},
		meta:            map[string]*peerMeta{},
		forwarded:       map[string]time.Time{},
		inherited:       map[string]bool{},
	}
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.publishLoop() }()
	return s, nil
}

// SessionID is the relay's forwarding-record scope (§5.2). It is exported for
// tests and for an operator who has to reason about a proof.
func (s *Server) SessionID() string { return s.sessionID }

// ReleaseSlot implements the operator escape hatch of §7.5. It is a startup
// command, not a wire message: an operator command is a rare, deliberate,
// physical act on the machine that owns the data, and giving it a network
// surface in a milestone with one shared token is a poor trade.
func (s *Server) ReleaseSlot(slot int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, ok := s.grid.ResOfSlot(slot)
	if !ok {
		return errors.New("relay: no such slot")
	}
	if _, live := s.peers[res.PeerID]; live {
		// §7.5: releasing a slot whose peer is live is a mis-operation.
		return fmt.Errorf("relay: slot %d is held by a live peer (%s); stop it first", slot, res.PeerID)
	}
	s.grid.Release(slot)
	if err := s.grid.Save(); err != nil {
		return err
	}
	s.log.Warn("relay: released a slot by operator command",
		"slot", res.Slot, "position", res.Position(), "peer", res.PeerID,
		"slotCount", s.grid.Size(), "positionIsNowAHole", res.Position())
	s.dirty = true
	return nil
}

// HandoverSlot rebinds a reservation — slot number AND position — to a
// different peerId (§7.5, new in M4). The map does not change shape and no lane
// moves.
//
// The relay refuses a handover while the old peer is live: a live peer with its
// slot taken out from under it would keep claiming, keep being refused, and
// keep a world running with nowhere to export.
func (s *Server) HandoverSlot(slot int, newPeerID string) (Reservation, Reservation, error) {
	if newPeerID == "" {
		return Reservation{}, Reservation{}, errors.New("relay: --handover-slot needs a new peer id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, ok := s.grid.ResOfSlot(slot)
	if !ok {
		return Reservation{}, Reservation{}, errors.New("relay: no such slot")
	}
	if _, live := s.peers[res.PeerID]; live {
		return Reservation{}, Reservation{}, fmt.Errorf(
			"relay: slot %d is held by a live peer (%s); stop it first", slot, res.PeerID)
	}
	if held := s.grid.SlotOfPeer(newPeerID); held > 0 {
		return Reservation{}, Reservation{}, fmt.Errorf(
			"relay: %s already holds slot %d; a peer holds at most one", newPeerID, held)
	}
	old, now, _ := s.grid.Handover(slot, newPeerID)
	if err := s.grid.Save(); err != nil {
		s.grid.Handover(slot, old.PeerID)
		return Reservation{}, Reservation{}, err
	}
	// The old identity keeps its journal and its genome cache; nothing about the
	// handover moves data. Its meta is dropped so the new occupant does not
	// inherit a stale version, size or stats block.
	delete(s.meta, old.PeerID)
	s.inherited[newPeerID] = true
	s.log.Warn("relay: handed a slot to a new peer identity by operator command",
		"slot", now.Slot, "position", now.Position(), "from", old.PeerID, "to", newPeerID)
	s.dirty = true
	return old, now, nil
}

// ReserveSlot pre-seeds one reservation for a peer that has not connected yet,
// optionally at a named position. Like ReleaseSlot it is a startup command.
//
// Why the map needs it. Auto-placement (§7.2 rule 6) makes slot order start
// order, and the rig on one machine simply starts its sidecars in the order it
// wants. Across a LAN that is not available: the second computer is started by
// a person, and demanding that they start it after slot 1 and before slot 3
// makes the map depend on human timing. Pre-seeding removes the ordering
// constraint completely — rule 1 then hands each peer the slot that is already
// keyed to its peerId, whenever it arrives.
//
// It is idempotent: a peerId that already holds a slot keeps it, and created is
// false.
func (s *Server) ReserveSlot(peerID string, at *contractb.Position) (Reservation, bool, error) {
	if peerID == "" {
		return Reservation{}, false, errors.New("relay: --reserve-slot needs a peer id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if res, ok := s.grid.ResOfPeer(peerID); ok {
		return res, false, nil
	}
	res := s.grid.Place(peerID, Preference{Position: at})
	if err := s.grid.Save(); err != nil {
		s.grid.Release(res.Slot)
		return Reservation{}, false, err
	}
	return res, true, nil
}

// Snapshot returns the current reservations in structural order.
func (s *Server) Snapshot() []Reservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.grid.Order()
}

// MapShape is the rectangle right now.
func (s *Server) MapShape() contractb.MapShape {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.grid.Shape()
}

// Drain closes every connection with 4005. A hijacked WebSocket is not tracked
// by net/http, so http.Server.Shutdown alone would leave peers hanging.
func (s *Server) Drain() {
	s.mu.Lock()
	s.draining = true
	conns := make([]*peer, 0, len(s.peers)+len(s.subscribers))
	for _, p := range s.peers {
		conns = append(conns, p)
	}
	for _, p := range s.subscribers {
		conns = append(conns, p)
	}
	s.mu.Unlock()
	for _, p := range conns {
		p.conn.Close(contractb.CloseShuttingDown, "relay draining")
	}
	s.Close()
}

// Close stops the publisher. It is idempotent.
func (s *Server) Close() {
	s.stopOnce.Do(func() { close(s.stop) })
	s.wg.Wait()
}

// Handler returns the relay's HTTP handler, serving contract-b-m4.md's path and
// the retired one.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(contractb.ContractBPath, s.serveWS)
	// §3: a relay MUST keep serving /contract-b/v2 and MUST close every
	// connection on it immediately with 4000, so an M3 sidecar gets the defined
	// loud error instead of a bare HTTP 404.
	mux.HandleFunc(contractb.RetiredContractBPath, s.serveRetired)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

func (s *Server) serveRetired(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode:    websocket.CompressionDisabled,
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	s.log.Error("relay: refusing a connection on the retired contract-b/v2 path",
		"remote", r.RemoteAddr, "path", contractb.RetiredContractBPath,
		"hint", "this relay speaks "+wire.ProtocolB+" on "+contractb.ContractBPath)
	_ = ws.Close(contractb.CloseProtocolUnsupported,
		"contract-b/v2 is retired; this relay serves "+contractb.ContractBPath)
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	// §3.1: the token is checked on the HTTP upgrade. A missing or wrong token
	// gets 401 and no upgrade, so there is no WebSocket and no close code.
	if s.insecureNoToken {
		s.log.Warn("relay: accepting a connection with NO TOKEN CHECK; " +
			"--insecure-no-token is for a single-machine test rig and never for the LAN")
	} else if !lantoken.Equal(lantoken.FromRequest(r), s.token) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		s.log.Error("relay: rejected an unauthenticated connection", "remote", r.RemoteAddr)
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode:    websocket.CompressionDisabled,
		InsecureSkipVerify: true, // LAN rig; per-peer identity and TLS are M5 (D9)
	})
	if err != nil {
		s.log.Warn("relay: websocket upgrade failed", "err", err)
		return
	}
	s.handle(r.Context(), wsutil.New(ws, 128))
}

func (s *Server) handle(ctx context.Context, conn *wsutil.Conn) {
	p, err := s.handshake(ctx, conn)
	if err != nil {
		s.log.Warn("relay: handshake failed", "err", err)
		<-conn.Done()
		return
	}
	defer s.drop(p)

	go s.pingLoop(p)
	if p.role == contractb.RoleArchive {
		go s.copyLoop(p)
	}

	for {
		readCtx, cancel := context.WithTimeout(ctx, s.peerTimeout)
		frame, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				s.log.Warn("relay: peer silent, dropping", "peer", p.id)
				conn.Close(contractb.CloseLivenessTimeout, "no traffic within peerTimeoutMs")
			}
			<-conn.Done()
			return
		}
		s.touch(p.id)
		if !s.dispatch(p, frame) {
			<-conn.Done()
			return
		}
	}
}

// handshake reads the mandatory first frame and registers the client.
func (s *Server) handshake(ctx context.Context, conn *wsutil.Conn) (*peer, error) {
	readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	frame, err := conn.Read(readCtx)
	if err != nil {
		conn.Close(contractb.CloseMalformedFrame, "no HANDSHAKE")
		return nil, err
	}
	env, err := wire.Decode(frame)
	if err != nil {
		conn.Close(contractb.CloseMalformedFrame, "malformed first frame")
		return nil, err
	}
	if err := wire.CheckProtocol(env.Protocol, wire.ProtocolB); err != nil {
		conn.Close(contractb.CloseProtocolUnsupported, "unsupported protocol major")
		return nil, err
	}
	if env.Type != contractb.TypeHandshake {
		conn.Close(contractb.CloseMalformedFrame, "first frame is not HANDSHAKE")
		return nil, errors.New("relay: first frame is not HANDSHAKE")
	}
	var hs contractb.Handshake
	if err := contractb.DecodeData(env.Data, &hs); err != nil {
		conn.Close(contractb.CloseMalformedFrame, "malformed HANDSHAKE")
		return nil, err
	}
	if err := hs.Validate(); err != nil {
		conn.Close(contractb.CloseMalformedFrame, err.Error())
		return nil, err
	}
	if err := wire.CheckProtocol(hs.ProtocolVersion, wire.ProtocolB); err != nil {
		conn.Close(contractb.CloseProtocolUnsupported, "unsupported protocolVersion")
		return nil, err
	}

	p := &peer{id: hs.PeerID, role: hs.Role, conn: conn}
	if p.role == contractb.RoleArchive {
		p.out = make(chan []byte, s.archiveQueue)
	}

	s.mu.Lock()
	// §6.1: compatibility enforcement at connect, and it must be loud. A silent
	// version mismatch is indistinguishable from a dead peer — under M4 both end
	// with a lane routed around them — and M4 crosses two independently updated
	// installs, so this is the failure most likely to waste an evening.
	if p.role == contractb.RolePeer {
		if mapVersion := s.mapVersionLocked(""); mapVersion != "" && hs.GameVersion != "" &&
			mapVersion != hs.GameVersion {
			s.metaLocked(p.id).lastRefusal = "gameVersion " + hs.GameVersion +
				" is incompatible with the map's " + mapVersion
			s.dirty = true
			s.mu.Unlock()
			s.log.Error("relay: refusing a peer on gameVersion grounds",
				"peer", p.id, "peerGameVersion", hs.GameVersion, "mapGameVersion", mapVersion)
			conn.Close(contractb.CloseMalformedFrame,
				"gameVersion "+hs.GameVersion+" is incompatible with the map's "+mapVersion)
			return nil, errors.New("relay: incompatible gameVersion")
		}
	}

	registry := s.peers
	if p.role == contractb.RoleArchive {
		registry = s.subscribers
	}
	if old, ok := registry[p.id]; ok {
		// §6.1: a newer connection with a live peer id takes over. This makes a
		// crashed-and-restarted sidecar self-healing.
		old.conn.Close(contractb.CloseReplaced, "a newer connection claimed this peerId")
	}
	registry[p.id] = p

	m := s.metaLocked(p.id)
	m.lastRefusal = ""
	m.lastSeenMs = time.Now().UnixMilli()
	m.darkSinceMs = 0
	if p.role == contractb.RolePeer {
		if hs.GameVersion != "" {
			m.gameVersion = hs.GameVersion
		}
		if hs.SimulationSize > 0 {
			m.simSize = hs.SimulationSize
		}
	}
	ack := contractb.HandshakeAck{
		RelayVersion:    Version,
		ProtocolVersion: wire.ProtocolB,
		RelaySessionID:  s.sessionID,
		Map:             s.grid.Shape(),
		SlotCount:       s.grid.Size(),
		ReceivedAt:      time.Now().UnixMilli(),
	}
	if p.role == contractb.RolePeer {
		if res, ok := s.grid.ResOfPeer(p.id); ok {
			slot, pos := res.Slot, res.Position()
			ack.AssignedSlot = &slot
			ack.AssignedPosition = &pos
		}
	}
	s.dirty = true
	s.mu.Unlock()

	s.send(p, contractb.TypeHandshakeAck, ack)
	s.log.Info("relay: client connected", "peer", p.id, "role", p.role,
		"assignedSlot", derefSlot(ack.AssignedSlot), "map", ack.Map, "slotCount", ack.SlotCount,
		"relaySessionId", s.sessionID)
	return p, nil
}

// mapVersionLocked is the game version the map is running: the first non-empty
// version among live peers other than self, in structural order.
func (s *Server) mapVersionLocked(self string) string {
	for _, res := range s.grid.Slots {
		if res.PeerID == self {
			continue
		}
		if _, live := s.peers[res.PeerID]; !live {
			continue
		}
		if m, ok := s.meta[res.PeerID]; ok && m.gameVersion != "" {
			return m.gameVersion
		}
	}
	for id := range s.peers {
		if id == self {
			continue
		}
		if m, ok := s.meta[id]; ok && m.gameVersion != "" {
			return m.gameVersion
		}
	}
	return ""
}

func (s *Server) metaLocked(peerID string) *peerMeta {
	m, ok := s.meta[peerID]
	if !ok {
		m = &peerMeta{}
		s.meta[peerID] = m
	}
	return m
}

func (s *Server) touch(peerID string) {
	s.mu.Lock()
	s.metaLocked(peerID).lastSeenMs = time.Now().UnixMilli()
	s.mu.Unlock()
}

// dispatch handles one frame. It returns false when the connection must end.
func (s *Server) dispatch(p *peer, frame []byte) bool {
	env, err := wire.Decode(frame)
	if err != nil {
		p.conn.Close(contractb.CloseMalformedFrame, "malformed frame")
		return false
	}
	if err := wire.CheckProtocol(env.Protocol, wire.ProtocolB); err != nil {
		p.conn.Close(contractb.CloseProtocolUnsupported, "unsupported protocol major")
		return false
	}

	switch env.Type {
	case contractb.TypeSectorClaim:
		return s.onSectorClaim(p, env)
	case contractb.TypeMigrationPayload:
		return s.onMigrationPayload(p, env, frame)
	case contractb.TypeMigrationAck, contractb.TypeMigrationNack:
		if p.role == contractb.RoleArchive {
			// §5.1: a subscriber MUST NOT answer a copied frame.
			s.log.Warn("relay: ignoring an answer from a read-only subscriber",
				"peer", p.id, "type", env.Type)
			return true
		}
		return s.onDirected(p, env, frame, true)
	case contractb.TypeGenomeRequest:
		return s.onGenomeRequest(p, env, frame)
	case contractb.TypeGenomeResponse:
		if p.role == contractb.RoleArchive {
			s.log.Warn("relay: ignoring a GENOME_RESPONSE from a subscriber", "peer", p.id)
			return true
		}
		return s.onDirected(p, env, frame, false)
	case contractb.TypePing:
		var ping contractb.Ping
		if contractb.DecodeData(env.Data, &ping) == nil {
			// §6.11: a PING from a peer MAY carry the stats block. The relay
			// stores it against that peer with its own receivedAt, republishes
			// it, and never routes, refuses, schedules or filters on a stat.
			if p.role == contractb.RolePeer && ping.Stats != nil {
				s.mu.Lock()
				m := s.metaLocked(p.id)
				m.stats = ping.Stats
				m.statsAsOfMs = time.Now().UnixMilli()
				s.dirty = true
				s.mu.Unlock()
			}
			s.send(p, contractb.TypePong, contractb.Pong{Nonce: ping.Nonce})
		}
		return true
	case contractb.TypePong:
		return true
	case contractb.TypeHandshake:
		p.conn.Close(contractb.CloseMalformedFrame, "duplicate HANDSHAKE")
		return false
	default:
		// contract-a.md §3.1, mirrored here: an unknown type is a
		// forward-compatible addition, not a fault.
		s.log.Warn("relay: ignoring unknown type", "peer", p.id, "type", env.Type)
		return true
	}
}

func (s *Server) onSectorClaim(p *peer, env wire.Envelope) bool {
	var claim contractb.SectorClaim
	if err := contractb.DecodeData(env.Data, &claim); err != nil {
		p.conn.Close(contractb.CloseMalformedFrame, "malformed SECTOR_CLAIM")
		return false
	}
	if err := claim.Validate(); err != nil {
		p.conn.Close(contractb.CloseMalformedFrame, err.Error())
		return false
	}

	// §7.2 rule 7: a claim from a role:"archive" client never gets a slot.
	if p.role == contractb.RoleArchive {
		s.mu.Lock()
		grant := contractb.SectorGrant{
			Granted: false, Map: s.grid.Shape(), SlotCount: s.grid.Size(),
			Reason: contractb.GrantRoleHasNoSlot}
		s.mu.Unlock()
		s.answerClaim(p, grant)
		return true
	}

	s.mu.Lock()
	// §7.2 rule 8: a peer whose gameVersion disagrees with the map's is refused.
	// This is the ordinary path for the check, because a sidecar usually
	// connects before its mod and only learns a version here.
	if mapVersion := s.mapVersionLocked(p.id); mapVersion != "" &&
		claim.GameVersion != "" && mapVersion != claim.GameVersion {
		s.metaLocked(p.id).lastRefusal = "gameVersion " + claim.GameVersion +
			" is incompatible with the map's " + mapVersion
		grant := contractb.SectorGrant{
			Granted: false, Map: s.grid.Shape(), SlotCount: s.grid.Size(),
			Reason: contractb.GrantVersionIncompatible}
		s.dirty = true
		s.mu.Unlock()
		s.log.Error("relay: refusing a claim on gameVersion grounds",
			"peer", p.id, "peerGameVersion", claim.GameVersion, "mapGameVersion", mapVersion)
		s.answerClaim(p, grant)
		return true
	}

	m := s.metaLocked(p.id)
	m.simSize = claim.SimulationSize
	m.modConnected = claim.ModConnected
	m.exportEdges = append([]string(nil), claim.ExportEdges...)
	m.lastRefusal = ""
	if claim.GameVersion != "" {
		m.gameVersion = claim.GameVersion
	}
	if claim.Stats != nil {
		m.stats = claim.Stats
		m.statsAsOfMs = time.Now().UnixMilli()
	}

	res, reason, placed := s.assignLocked(p, claim)
	if placed {
		// §7.2 ordering step 1: the map is on disk BEFORE the grant goes out. An
		// answered grant that is not durable can hand the same slot to two peers
		// across a restart.
		if err := s.grid.Save(); err != nil {
			s.log.Error("relay: ring.json write failed; refusing the claim", "err", err)
			s.grid.Release(res.Slot)
			grant := contractb.SectorGrant{
				Granted: false, Map: s.grid.Shape(), SlotCount: s.grid.Size(),
				Reason: contractb.GrantProtocolMismatch}
			s.mu.Unlock()
			s.answerClaim(p, grant)
			return true
		}
	}
	p.claimed = true
	grant := s.grantForLocked(res, reason)
	s.dirty = true
	s.mu.Unlock()

	s.log.Info("relay: placement claim", "peer", p.id, "slot", res.Slot,
		"position", res.Position(), "reason", reason,
		"simulationSize", claim.SimulationSize, "modConnected", claim.ModConnected,
		"exportEdges", claim.ExportEdges)
	// §7.2 ordering step 2: answer the claim. Steps 3 and 4 — the PEER_STATUS
	// broadcast and the fresh grants to every peer whose effective neighbour
	// changed — belong to the publisher, which coalesces them.
	s.answerClaim(p, grant)
	return true
}

// assignLocked implements §7.2's arbitration rules 1 to 6, in order.
func (s *Server) assignLocked(p *peer, claim contractb.SectorClaim) (Reservation, string, bool) {
	// Rules 1 and 2: this peerId already holds a reservation, or names one
	// reserved to itself. The reservation is keyed on peerId, never on a
	// connection, and it NEVER EXPIRES. This is the whole of "return needs no
	// insertion": a peer that comes back after two hours or two weeks lands
	// where it was.
	if res, ok := s.grid.ResOfPeer(p.id); ok {
		if p.claimed {
			return res, contractb.GrantUpdated, false
		}
		if s.inherited[p.id] {
			// §7.5: this peer inherited a slot by operator command. It inherits
			// the address and nothing else, and its first grant says which.
			delete(s.inherited, p.id)
			return res, contractb.GrantHandover, false
		}
		return res, contractb.GrantReclaimed, false
	}
	// Rule 3: a preferredSlot reserved to somebody else is ignored, never
	// honoured by eviction.
	if claim.PreferredSlot > 0 {
		if owner := s.grid.PeerOfSlot(claim.PreferredSlot); owner != "" {
			s.log.Warn("relay: ignoring a preferredSlot that belongs to another peer",
				"peer", p.id, "preferredSlot", claim.PreferredSlot, "owner", owner)
		}
	}
	res := s.grid.Place(p.id, Preference{
		Slot:            claim.PreferredSlot,
		Position:        claim.PreferredPosition,
		InsertAfterSlot: claim.InsertAfterSlot,
		InsertAxis:      claim.InsertAxis,
	})
	return res, contractb.GrantGranted, true
}

// deliverableLocked is §8's filter from one peer's point of view. It returns
// the empty string when the candidate is deliverable and the §6.4 skip reason
// otherwise.
func (s *Server) deliverableLocked(me Reservation) Deliverable {
	mine := s.metaLocked(me.PeerID)
	return func(res Reservation) string {
		if res.Slot == me.Slot {
			return contractb.SkipPeerOffline // unreachable: the walk never visits self
		}
		if _, live := s.peers[res.PeerID]; !live {
			return contractb.SkipPeerOffline
		}
		m := s.metaLocked(res.PeerID)
		if !m.modConnected {
			// A dead sim must not keep receiving organisms (contract-a.md §8).
			return contractb.SkipPeerModAbsent
		}
		if mine.gameVersion != "" && m.gameVersion != "" && mine.gameVersion != m.gameVersion {
			return contractb.SkipPeerIncompatible
		}
		if mine.simSize > 0 && m.simSize > 0 && !sameSize(mine.simSize, m.simSize) {
			return contractb.SkipSimSizeMismatch
		}
		return ""
	}
}

// sameSize is contract-a.md §13 A10's relative epsilon. §4.1 forbids exact
// float equality.
func sameSize(a, b float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return false
	}
	return math.Abs(a-b) <= 1e-6*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

// exportEdgesOf is the set of edges a peer declared. A peer that has not
// claimed yet, or has no mod, declares nothing and gets no lanes.
func (s *Server) exportEdgesLocked(peerID string) []string {
	if m, ok := s.meta[peerID]; ok {
		return m.exportEdges
	}
	return nil
}

// grantForLocked builds one peer's SECTOR_GRANT: its slot, its position, the
// map, and ONE EFFECTIVE NEIGHBOUR PER EXPORT EDGE.
//
// A key is absent when that axis has no deliverable target, and its absence is
// what closes that export edge with no_peer (§8).
func (s *Server) grantForLocked(res Reservation, reason string) contractb.SectorGrant {
	pos := res.Position()
	grant := contractb.SectorGrant{
		Granted:   true,
		Slot:      res.Slot,
		Position:  &pos,
		Map:       s.grid.Shape(),
		SlotCount: s.grid.Size(),
		Reason:    reason,
	}
	ok := s.deliverableLocked(res)
	edges := s.exportEdgesLocked(res.PeerID)
	if len(edges) == 0 {
		// Nothing declared: no lanes to publish. The sidecar's own EDGE_STATUS
		// is empty too, which closes every edge it might have had.
		return grant
	}
	grant.Neighbours = map[string]*contractb.Neighbour{}
	for _, edge := range edges {
		if edge != contracta.EdgeE && edge != contracta.EdgeN {
			// The grid exports east and north. An edge the mod declared that the
			// map has no axis for simply has no lane.
			continue
		}
		target, skipped, found := s.grid.Effective(res, edge, ok)
		if !found {
			continue
		}
		m := s.metaLocked(target.PeerID)
		grant.Neighbours[edge] = &contractb.Neighbour{
			Slot:           target.Slot,
			PeerID:         target.PeerID,
			Position:       target.Position(),
			Live:           true,
			ModConnected:   true,
			GameVersion:    m.gameVersion,
			SimulationSize: m.simSize,
			Skipped:        skipped,
		}
	}
	return grant
}

func (s *Server) answerClaim(p *peer, grant contractb.SectorGrant) {
	body, err := json.Marshal(grant)
	if err == nil {
		p.mu.Lock()
		p.lastGrant = string(body)
		p.lastPos = grant.Position
		p.mu.Unlock()
	}
	s.send(p, contractb.TypeSectorGrant, grant)
}

// ---------------------------------------------------------------- publishing

// publishLoop is §7.2's "no storm" rule. Six peers, two axes and a flapping
// link can generate more registry changes than anybody can read, so at most one
// PEER_STATUS broadcast per statusCoalesceMs and at most one SECTOR_GRANT per
// peer in that window. Coalescing may drop intermediate states; it MUST NOT
// drop the last one, because every one of these messages is full state and the
// last one is the truth — which the dirty flag guarantees, since it stays set
// until a publish actually happens.
func (s *Server) publishLoop() {
	coalesce := time.NewTicker(s.statusCoalesce)
	defer coalesce.Stop()
	stats := time.NewTicker(s.statsBroadcast)
	defer stats.Stop()
	sweep := time.NewTicker(time.Minute)
	defer sweep.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-coalesce.C:
			s.mu.Lock()
			dirty := s.dirty
			s.dirty = false
			s.mu.Unlock()
			if dirty {
				s.publish()
			}
		case <-stats.C:
			// §6.5: also sent on a timer, because stats change without the
			// registry changing.
			s.publish()
		case <-sweep.C:
			s.sweepForwardRecord()
		}
	}
}

func (s *Server) publish() {
	s.broadcastPeerStatus()
	s.sendGrants()
}

func (s *Server) markDirty() {
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
}

// sendGrants pushes a SECTOR_GRANT to every live slot holder whose grant body
// changed — which after a splice or a liveness change is usually more than one
// peer, and after a column insertion may be most of them.
func (s *Server) sendGrants() {
	s.mu.Lock()
	type item struct {
		p     *peer
		grant contractb.SectorGrant
	}
	items := make([]item, 0, len(s.peers))
	for _, res := range s.grid.Slots {
		p, live := s.peers[res.PeerID]
		if !live {
			continue
		}
		items = append(items, item{p: p, grant: s.grantForLocked(res, contractb.GrantUpdated)})
	}
	s.mu.Unlock()

	for _, it := range items {
		it.p.mu.Lock()
		// §6.4: "repositioned" is the same slot at a NEW coordinate, because the
		// map grew. A splice moves most of a row or a column, and a peer that
		// learns its neighbours changed for that reason deserves to be told which
		// reason it was.
		if it.p.lastPos != nil && it.grant.Position != nil && *it.p.lastPos != *it.grant.Position {
			it.grant.Reason = contractb.GrantRepositioned
		}
		body, err := json.Marshal(it.grant)
		if err != nil {
			it.p.mu.Unlock()
			continue
		}
		same := it.p.lastGrant == string(body)
		if !same {
			it.p.lastGrant = string(body)
			it.p.lastPos = it.grant.Position
		}
		it.p.mu.Unlock()
		if same {
			continue
		}
		s.send(it.p, contractb.TypeSectorGrant, it.grant)
	}
}

func (s *Server) broadcastPeerStatus() {
	s.mu.Lock()
	slots := make([]contractb.SlotInfo, 0, s.grid.Size())
	for _, res := range s.grid.Slots {
		m := s.metaLocked(res.PeerID)
		_, live := s.peers[res.PeerID]
		info := contractb.SlotInfo{
			Slot:           res.Slot,
			Position:       res.Position(),
			PeerID:         res.PeerID,
			Live:           live,
			ModConnected:   live && m.modConnected,
			GameVersion:    m.gameVersion,
			SimulationSize: m.simSize,
			ExportEdges:    append([]string{}, m.exportEdges...),
			LastSeenMs:     m.lastSeenMs,
			LastRefusal:    m.lastRefusal,
			Stats:          m.stats,
		}
		if !live {
			// Risk 5: a healed map hides a dead world. "Bypassed since 04:12" is
			// what stops an operator missing it for a day.
			info.DarkSinceMs = m.darkSinceMs
		}
		if m.stats != nil {
			info.StatsAsOfMs = m.statsAsOfMs
		}
		slots = append(slots, info)
	}
	observers := len(s.subscribers)
	shape := s.grid.Shape()
	count := s.grid.Size()

	type target struct {
		p  *peer
		me contractb.You
	}
	targets := make([]target, 0, len(s.peers)+len(s.subscribers))
	for _, p := range s.peers {
		me := contractb.You{Neighbours: map[string]*int{}}
		if res, ok := s.grid.ResOfPeer(p.id); ok {
			slot, pos := res.Slot, res.Position()
			me.Slot = &slot
			me.Position = &pos
			ok := s.deliverableLocked(res)
			for _, edge := range []string{contracta.EdgeE, contracta.EdgeN} {
				if target, _, found := s.grid.Effective(res, edge, ok); found {
					n := target.Slot
					me.Neighbours[edge] = &n
				} else {
					me.Neighbours[edge] = nil
				}
			}
		}
		targets = append(targets, target{p: p, me: me})
	}
	for _, p := range s.subscribers {
		// §6.5: all null for a subscriber.
		targets = append(targets, target{p: p, me: contractb.You{Neighbours: map[string]*int{}}})
	}
	s.mu.Unlock()

	for _, t := range targets {
		t.p.mu.Lock()
		t.p.epoch++
		epoch := t.p.epoch
		t.p.mu.Unlock()
		s.send(t.p, contractb.TypePeerStatus, contractb.PeerStatus{
			Epoch:     epoch,
			Map:       shape,
			SlotCount: count,
			Slots:     slots,
			You:       t.me,
			Observers: observers,
		})
	}
}

// ---------------------------------------------------------------- routing

// onMigrationPayload routes on data.destSlot and nothing else. The frame is
// forwarded byte for byte; body.bb8 and data.lineage are never decoded (§5).
func (s *Server) onMigrationPayload(p *peer, env wire.Envelope, frame []byte) bool {
	var routing contractb.Routing
	if err := json.Unmarshal(env.Data, &routing); err != nil {
		p.conn.Close(contractb.CloseMalformedFrame, "malformed routing fields")
		return false
	}
	if routing.SourcePeer != p.id {
		p.conn.Close(contractb.CloseMalformedFrame, "sourcePeer does not match this connection")
		return false
	}
	var id contractb.Identity
	_ = json.Unmarshal(env.Data, &id)

	// §5.1: a MIGRATION_PAYLOAD from a subscriber is answered NOT_A_MEMBER and
	// is not forwarded.
	if p.role == contractb.RoleArchive {
		s.send(p, contractb.TypeMigrationNack, contractb.MigrationNack{
			MigrationID: id.MigrationID,
			SourcePeer:  "",
			DestPeer:    p.id,
			Code:        contractb.NackNotAMember,
			Class:       contractb.ClassPermanent,
			Message:     "this connection is a read-only archive subscriber and holds no slot",
		})
		s.log.Warn("relay: refused a migration from a read-only subscriber", "peer", p.id)
		return true
	}
	if routing.DestSlot < 1 {
		p.conn.Close(contractb.CloseMalformedFrame, "destSlot is not a slot")
		return false
	}

	s.mu.Lock()
	res, reserved := s.grid.ResOfSlot(routing.DestSlot)
	var dest *peer
	darkSince := int64(0)
	if reserved {
		dest = s.peers[res.PeerID]
		if m, ok := s.meta[res.PeerID]; ok {
			darkSince = m.darkSinceMs
		}
	}
	draining := s.draining
	s.mu.Unlock()

	switch {
	case !reserved:
		// §6.8: destSlot names NO RESERVATION AT ALL — released, handed to
		// nobody, or never issued. Slot numbers are never reused, so this world
		// never returns and no retry can ever succeed. PERMANENT.
		s.nackNoDelivery(p, id.MigrationID, contractb.NackSlotVacant,
			fmt.Sprintf("slot %d names no reservation; slot numbers are never reused, so it never returns",
				routing.DestSlot))
		return true
	case dest == nil:
		msg := fmt.Sprintf("slot %d (%d,%d) is reserved to %s, which is not connected",
			res.Slot, res.Col, res.Row, res.PeerID)
		if darkSince > 0 {
			msg += fmt.Sprintf(", dark for %ds", (time.Now().UnixMilli()-darkSince)/1000)
		}
		s.nackNoDelivery(p, id.MigrationID, contractb.NackPeerOffline, msg)
		return true
	case draining:
		s.nackNoDelivery(p, id.MigrationID, contractb.NackNotForwarded,
			"the relay is draining and declined to hand this frame over")
		return true
	}

	// §5.2: ANY ATTEMPTED WRITE of the frame to a destination peer's connection
	// counts as forwarded, whether or not that write later fails. A partial
	// write and a peer that dies with bytes in its receive buffer are
	// indistinguishable from a complete delivery, so both count.
	//
	// The record is written BEFORE the attempt, so no interleaving can produce a
	// neverForwarded: true for a frame that did go out. Every ambiguity resolves
	// toward "no proof".
	s.recordForward(id.MigrationID)
	if err := dest.conn.Send(frame); err != nil {
		s.log.Warn("relay: forward failed after the attempt was recorded",
			"peer", dest.id, "migrationId", id.MigrationID, "err", err)
	}
	s.fanOut(frame)
	return true
}

// nackNoDelivery answers the SENDER rather than dropping the frame. A dropped
// frame turns a bounded failure into a stall, and under M4 it also withholds
// the evidence a sender needs to re-route (§5.2).
func (s *Server) nackNoDelivery(p *peer, migrationID, code, message string) {
	never := !s.hasForwarded(migrationID)
	nack := contractb.MigrationNack{
		MigrationID:    migrationID,
		SourcePeer:     "",
		DestPeer:       p.id,
		Code:           code,
		Class:          contractb.ClassOf(code),
		Message:        message,
		NeverForwarded: &never,
		RelaySessionID: s.sessionID,
	}
	if nack.Class == contractb.ClassTransient {
		nack.RetryAfterMs = 15000
	}
	if never {
		nack.Message += "; this relay has never forwarded this migration"
	} else {
		nack.Message += "; this relay has already handed this migration to a peer at least once"
	}
	s.send(p, contractb.TypeMigrationNack, nack)
	s.fanOut(mustFrame(s.log, contractb.TypeMigrationNack, nack))
}

func (s *Server) recordForward(migrationID string) {
	if migrationID == "" {
		return
	}
	s.mu.Lock()
	if _, ok := s.forwarded[migrationID]; !ok {
		s.forwarded[migrationID] = time.Now()
	}
	s.mu.Unlock()
}

func (s *Server) hasForwarded(migrationID string) bool {
	if migrationID == "" {
		// An answer about a migration the relay cannot name is no proof at all.
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.forwarded[migrationID]
	return ok
}

func (s *Server) sweepForwardRecord() {
	cutoff := time.Now().Add(-s.forwardRetain)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, at := range s.forwarded {
		if at.Before(cutoff) {
			delete(s.forwarded, id)
		}
	}
}

// ForwardedCount is the size of the §5.2 record, for tests and operator logs.
func (s *Server) ForwardedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.forwarded)
}

// onDirected routes MIGRATION_ACK, MIGRATION_NACK and GENOME_RESPONSE on
// data.destPeer. copyToArchive marks the frames §5.1 fans out.
func (s *Server) onDirected(p *peer, env wire.Envelope, frame []byte, copyToArchive bool) bool {
	var routing contractb.Routing
	if err := json.Unmarshal(env.Data, &routing); err != nil {
		p.conn.Close(contractb.CloseMalformedFrame, "malformed routing fields")
		return false
	}
	if routing.SourcePeer != p.id {
		p.conn.Close(contractb.CloseMalformedFrame, "sourcePeer does not match this connection")
		return false
	}
	s.mu.Lock()
	dest := s.peers[routing.DestPeer]
	if dest == nil {
		dest = s.subscribers[routing.DestPeer]
	}
	s.mu.Unlock()
	if dest == nil {
		if env.Type == contractb.TypeMigrationAck || env.Type == contractb.TypeMigrationNack {
			// §6.8: PEER_UNKNOWN, rather than a drop. These route on a peer id
			// rather than a slot.
			var id contractb.Identity
			_ = json.Unmarshal(env.Data, &id)
			s.send(p, contractb.TypeMigrationNack, contractb.MigrationNack{
				MigrationID:  id.MigrationID,
				SourcePeer:   "",
				DestPeer:     p.id,
				Code:         contractb.NackPeerUnknown,
				Class:        contractb.ClassTransient,
				Message:      "destPeer " + routing.DestPeer + " is not connected",
				RetryAfterMs: 15000,
			})
			return true
		}
		s.log.Warn("relay: dropping frame for absent peer", "type", env.Type, "destPeer", routing.DestPeer)
		return true
	}
	s.forward(dest, frame)
	if copyToArchive {
		s.fanOut(frame)
	}
	return true
}

// onGenomeRequest routes on destPeer and answers the requester itself when the
// asked-for peer is not connected (§5, §6.10).
func (s *Server) onGenomeRequest(p *peer, env wire.Envelope, frame []byte) bool {
	var routing contractb.Routing
	if err := json.Unmarshal(env.Data, &routing); err != nil {
		p.conn.Close(contractb.CloseMalformedFrame, "malformed routing fields")
		return false
	}
	if routing.SourcePeer != p.id {
		p.conn.Close(contractb.CloseMalformedFrame, "sourcePeer does not match this connection")
		return false
	}
	s.mu.Lock()
	dest := s.peers[routing.DestPeer]
	s.mu.Unlock()
	if dest == nil {
		var id contractb.Identity
		_ = json.Unmarshal(env.Data, &id)
		s.send(p, contractb.TypeGenomeResponse, contractb.GenomeResponse{
			RequestID:  id.RequestID,
			SourcePeer: "",
			DestPeer:   p.id,
			GenomeHash: id.GenomeHash,
			Found:      false,
			Reason:     contractb.GenomePeerOffline,
		})
		return true
	}
	s.forward(dest, frame)
	return true
}

// fanOut sends a byte-identical copy of a routed frame to every subscriber
// (§5.1). It never delays, blocks or fails a migration.
func (s *Server) fanOut(frame []byte) {
	if frame == nil {
		return
	}
	s.mu.Lock()
	subs := make([]*peer, 0, len(s.subscribers))
	for _, sub := range s.subscribers {
		subs = append(subs, sub)
	}
	s.mu.Unlock()
	for _, sub := range subs {
		select {
		case sub.out <- frame:
		default:
			// Bounded: on overflow drop the OLDEST copy, count it, and log at
			// most one line a minute. Never disconnect the migration.
			select {
			case <-sub.out:
			default:
			}
			select {
			case sub.out <- frame:
			default:
			}
			sub.mu.Lock()
			sub.dropped++
			dropped := sub.dropped
			shout := time.Since(sub.lastDropLog) > time.Minute
			if shout {
				sub.lastDropLog = time.Now()
			}
			sub.mu.Unlock()
			if shout {
				s.log.Warn("relay: archive subscriber is behind, dropping the oldest copies",
					"peer", sub.id, "droppedTotal", dropped)
			}
		}
	}
}

func (s *Server) copyLoop(sub *peer) {
	for {
		select {
		case <-sub.conn.Done():
			return
		case frame := <-sub.out:
			if err := sub.conn.Send(frame); err != nil {
				return
			}
		}
	}
}

func (s *Server) forward(dest *peer, frame []byte) {
	if err := dest.conn.Send(frame); err != nil {
		s.log.Warn("relay: forward failed", "peer", dest.id, "err", err)
	}
}

func (s *Server) drop(p *peer) {
	s.mu.Lock()
	registry := s.peers
	if p.role == contractb.RoleArchive {
		registry = s.subscribers
	}
	if cur, ok := registry[p.id]; ok && cur == p {
		delete(registry, p.id)
	}
	if m, ok := s.meta[p.id]; ok {
		m.modConnected = false
		if p.role == contractb.RolePeer {
			m.darkSinceMs = time.Now().UnixMilli()
		}
	}
	slot := s.grid.SlotOfPeer(p.id)
	s.dirty = true
	s.mu.Unlock()
	s.log.Info("relay: client gone", "peer", p.id, "role", p.role, "slot", slot,
		"reservationKept", slot > 0)
	// §8: the vacancy is announced BACKWARDS along each lane that pointed at it.
	// Every peer whose effective neighbour was that slot re-targets and KEEPS
	// ITS EXPORT EDGE OPEN; the slot itself stays in the map, live:false, with
	// darkSinceMs set.
}

func (s *Server) send(p *peer, typ string, data any) {
	frame := mustFrame(s.log, typ, data)
	if frame == nil {
		return
	}
	if err := p.conn.Send(frame); err != nil {
		s.log.Warn("relay: send failed", "peer", p.id, "type", typ, "err", err)
	}
}

func mustFrame(log *slog.Logger, typ string, data any) []byte {
	frame, err := wire.Encode(wire.ProtocolB, typ, time.Now().UnixMilli(), data)
	if err != nil {
		log.Error("relay: encode failed", "type", typ, "err", err)
		return nil
	}
	return frame
}

func (s *Server) pingLoop(p *peer) {
	t := time.NewTicker(s.pingInterval)
	defer t.Stop()
	for {
		select {
		case <-p.conn.Done():
			return
		case <-t.C:
			s.send(p, contractb.TypePing, contractb.Ping{Nonce: wire.NewUUID()})
		}
	}
}

func derefSlot(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
