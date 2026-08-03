// Package relay implements multiverse-relay: the deliberately dumb M3
// transport (D1). It forwards Contract B frames around a ring of slots,
// arbitrates ring insertion, copies every routed migration to the archive, and
// never parses a bb8 body or a lineage annex (D4).
package relay

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contractb"
	"multiverse/internal/lantoken"
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

// Version is reported in HANDSHAKE_ACK.
const Version = "m3.0"

// Options configures a Server.
type Options struct {
	Logger *slog.Logger
	// DataDir holds ring.json. Empty keeps the ring in memory, which is what a
	// test rig wants and what production must never do (§7.4).
	DataDir         string
	Token           string
	InsecureNoToken bool
	PingInterval    time.Duration
	PeerTimeout     time.Duration
	// ArchiveQueue is the per-subscriber copy queue (§5.1).
	ArchiveQueue int
}

// Server is the relay. The zero value is not usable; call New.
type Server struct {
	log             *slog.Logger
	token           string
	insecureNoToken bool
	pingInterval    time.Duration
	peerTimeout     time.Duration
	archiveQueue    int

	mu   sync.Mutex
	ring *Ring
	// live connections by peer id, role "peer".
	peers map[string]*peer
	// live connections by peer id, role "archive".
	subscribers map[string]*peer
	// what the relay knows about a peer id, live or not. PEER_STATUS keeps
	// reporting a reserved slot after its peer goes away (§6.5).
	meta map[string]*peerMeta
}

type peerMeta struct {
	gameVersion  string
	simSize      float64
	modConnected bool
	lastSeenMs   int64
	lastRefusal  string
}

type peer struct {
	id      string
	role    string
	conn    *wsutil.Conn
	claimed bool // a SECTOR_CLAIM was already answered on this connection

	mu        sync.Mutex
	epoch     int64
	lastGrant string // the last SECTOR_GRANT body, so a repeat is not re-sent
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
	if opts.Token == "" && !opts.InsecureNoToken {
		// §3.1: no token configured means the relay MUST refuse to start.
		return nil, errors.New("relay: no MULTIVERSE_TOKEN and no --token-file; " +
			"pass --insecure-no-token only for a single-machine test rig")
	}
	ring, err := LoadRing(opts.DataDir)
	if err != nil {
		return nil, err
	}
	return &Server{
		log:             opts.Logger,
		token:           opts.Token,
		insecureNoToken: opts.InsecureNoToken,
		pingInterval:    opts.PingInterval,
		peerTimeout:     opts.PeerTimeout,
		archiveQueue:    opts.ArchiveQueue,
		ring:            ring,
		peers:           map[string]*peer{},
		subscribers:     map[string]*peer{},
		meta:            map[string]*peerMeta{},
	}, nil
}

// ReleaseSlot implements the operator escape hatch of §7.5. It is a startup
// command, not a wire message: giving release a network surface in the
// milestone that first leaves loopback is a poor trade.
func (s *Server) ReleaseSlot(slot int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	peerID := s.ring.PeerOfSlot(slot)
	if peerID == "" {
		return errors.New("relay: no such ring slot")
	}
	if _, live := s.peers[peerID]; live {
		// §7.5: releasing a slot whose peer is live is a mis-operation.
		return errors.New("relay: slot " + itoa(slot) + " is held by a live peer; stop it first")
	}
	res, _ := s.ring.Release(slot)
	if err := s.ring.Save(); err != nil {
		return err
	}
	s.log.Warn("relay: released a ring slot by operator command",
		"slot", res.Slot, "peer", res.PeerID, "ringSize", s.ring.Size())
	return nil
}

// ReserveSlot pre-seeds one ring reservation for a peer that has not connected
// yet. Like ReleaseSlot it is a startup command, not a wire message.
//
// Why the ring needs it. §7.2 rule 4 appends a new peer at the tail, so slot
// order is start order, and the rig on one machine simply starts its sidecars in
// the order it wants. Across a LAN that is not available: the second computer is
// started by a person, and demanding that they start it after slot 1 and before
// slot 3 makes the ring order depend on human timing. Pre-seeding the
// reservations in ring order removes the ordering constraint completely — rule 1
// then hands each peer the slot that is already keyed to its peerId, whenever it
// arrives.
//
// It is idempotent: a peerId that already holds a slot keeps it, and created is
// false. Re-running a pre-seed must never insert a second reservation.
func (s *Server) ReserveSlot(peerID string) (Reservation, bool, error) {
	if peerID == "" {
		return Reservation{}, false, errors.New("relay: --reserve-slot needs a peer id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if held := s.ring.SlotOfPeer(peerID); held > 0 {
		return Reservation{Slot: held, PeerID: peerID}, false, nil
	}
	res := s.ring.Append(peerID)
	if err := s.ring.Save(); err != nil {
		s.ring.Release(res.Slot)
		return Reservation{}, false, err
	}
	return res, true, nil
}

// RingSnapshot returns the current reservation order.
func (s *Server) RingSnapshot() []Reservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Reservation(nil), s.ring.Order...)
}

// Drain closes every connection with 4005. A hijacked WebSocket is not tracked
// by net/http, so http.Server.Shutdown alone would leave peers hanging.
func (s *Server) Drain() {
	s.mu.Lock()
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
}

// Handler returns the relay's HTTP handler, serving contract-b-m3.md's path.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(contractb.ContractBPath, s.serveWS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
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
		InsecureSkipVerify: true, // LAN rig; per-peer identity and TLS are M4 (D9)
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
	// version mismatch is indistinguishable from a dead peer — both end with a
	// closed export edge — and M3 crosses two independently updated installs
	// (m3_considerations.md Risk 5). An empty gameVersion is not a mismatch: it
	// means no mod is connected yet.
	if p.role == contractb.RolePeer {
		if ringVersion := s.ringVersionLocked(); ringVersion != "" && hs.GameVersion != "" &&
			ringVersion != hs.GameVersion {
			s.metaLocked(p.id).lastRefusal = "gameVersion " + hs.GameVersion +
				" is incompatible with the ring's " + ringVersion
			s.mu.Unlock()
			s.log.Error("relay: refusing a peer on gameVersion grounds",
				"peer", p.id, "peerGameVersion", hs.GameVersion, "ringGameVersion", ringVersion)
			conn.Close(contractb.CloseMalformedFrame,
				"gameVersion "+hs.GameVersion+" is incompatible with the ring's "+ringVersion)
			s.broadcastPeerStatus()
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
		RingSize:        s.ring.Size(),
		ReceivedAt:      time.Now().UnixMilli(),
	}
	if p.role == contractb.RolePeer {
		if slot := s.ring.SlotOfPeer(p.id); slot > 0 {
			ack.AssignedSlot = &slot
		}
	}
	s.mu.Unlock()

	s.send(p, contractb.TypeHandshakeAck, ack)
	s.log.Info("relay: client connected", "peer", p.id, "role", p.role,
		"assignedSlot", derefSlot(ack.AssignedSlot), "ringSize", ack.RingSize)
	s.broadcastPeerStatus()
	return p, nil
}

// ringVersionLocked is the game version the ring is running. It is the first
// non-empty version among live peers.
func (s *Server) ringVersionLocked() string {
	for _, res := range s.ring.Order {
		if _, live := s.peers[res.PeerID]; !live {
			continue
		}
		if m, ok := s.meta[res.PeerID]; ok && m.gameVersion != "" {
			return m.gameVersion
		}
	}
	for id := range s.peers {
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

	// §7.2 rule 5: a claim from a role:"archive" client never gets a slot.
	if p.role == contractb.RoleArchive {
		s.mu.Lock()
		ringSize := s.ring.Size()
		s.mu.Unlock()
		s.send(p, contractb.TypeSectorGrant, contractb.SectorGrant{
			Granted: false, RingSize: ringSize, Reason: contractb.GrantRoleHasNoSlot})
		return true
	}

	s.mu.Lock()
	// §7.2 rule 6: a peer whose gameVersion disagrees with the ring's is
	// refused. This is the ordinary path for the check, because a sidecar
	// usually connects before its mod and only learns a version here.
	if ringVersion := s.ringVersionOtherThanLocked(p.id); ringVersion != "" &&
		claim.GameVersion != "" && ringVersion != claim.GameVersion {
		s.metaLocked(p.id).lastRefusal = "gameVersion " + claim.GameVersion +
			" is incompatible with the ring's " + ringVersion
		ringSize := s.ring.Size()
		s.mu.Unlock()
		s.log.Error("relay: refusing a ring claim on gameVersion grounds",
			"peer", p.id, "peerGameVersion", claim.GameVersion, "ringGameVersion", ringVersion)
		s.send(p, contractb.TypeSectorGrant, contractb.SectorGrant{
			Granted: false, RingSize: ringSize, Reason: contractb.GrantVersionIncompatible})
		s.broadcastPeerStatus()
		return true
	}

	m := s.metaLocked(p.id)
	m.simSize = claim.SimulationSize
	m.modConnected = claim.ModConnected
	m.lastRefusal = ""
	if claim.GameVersion != "" {
		m.gameVersion = claim.GameVersion
	}

	slot, reason, inserted := s.assignLocked(p, claim.PreferredSlot)
	if inserted {
		// §7.4: the ring is on disk before the grant goes out. An answered
		// grant that is not durable can hand the same slot to two peers across
		// a restart.
		if err := s.ring.Save(); err != nil {
			s.log.Error("relay: ring.json write failed; refusing the claim", "err", err)
			s.ring.Release(slot)
			ringSize := s.ring.Size()
			s.mu.Unlock()
			s.send(p, contractb.TypeSectorGrant, contractb.SectorGrant{
				Granted: false, RingSize: ringSize, Reason: contractb.GrantProtocolMismatch})
			return true
		}
	}
	p.claimed = true
	s.mu.Unlock()

	s.log.Info("relay: ring claim", "peer", p.id, "slot", slot, "reason", reason,
		"simulationSize", claim.SimulationSize, "modConnected", claim.ModConnected,
		"exportEdge", claim.ExportEdge)
	// §7.2: after any change, broadcast PEER_STATUS and send a fresh
	// SECTOR_GRANT to every peer whose east neighbour changed. sendGrants does
	// the second half and suppresses a grant whose content did not move.
	s.broadcastPeerStatus()
	s.sendGrants(p.id, reason)
	return true
}

// ringVersionOtherThanLocked is the ring's game version ignoring one peer's own
// contribution, so a peer never refuses itself.
func (s *Server) ringVersionOtherThanLocked(self string) string {
	for id, m := range s.meta {
		if id == self || m.gameVersion == "" {
			continue
		}
		if _, live := s.peers[id]; !live {
			continue
		}
		return m.gameVersion
	}
	return ""
}

// assignLocked implements contract-b-m3.md §7.2's arbitration, in order.
func (s *Server) assignLocked(p *peer, preferred int) (slot int, reason string, inserted bool) {
	// Rules 1 and 2: this peerId already holds a slot, or names one reserved to
	// itself. The reservation is keyed on peerId, never on a connection, and it
	// never expires.
	if held := s.ring.SlotOfPeer(p.id); held > 0 {
		if p.claimed {
			return held, contractb.GrantUpdated, false
		}
		return held, contractb.GrantReclaimed, false
	}
	// Rule 3: a preferredSlot reserved to somebody else is ignored, never
	// honoured by eviction. A preference never evicts anybody.
	if preferred > 0 && s.ring.PeerOfSlot(preferred) != "" {
		s.log.Warn("relay: ignoring a preferredSlot that belongs to another peer",
			"peer", p.id, "preferredSlot", preferred, "owner", s.ring.PeerOfSlot(preferred))
	}
	// Rule 4: insert at the tail.
	res := s.ring.Append(p.id)
	return res.Slot, contractb.GrantGranted, true
}

// sendGrants pushes a SECTOR_GRANT to every live slot holder whose grant body
// changed. reason applies to the claiming peer; everyone else is an update.
func (s *Server) sendGrants(claimant, claimReason string) {
	s.mu.Lock()
	type item struct {
		p     *peer
		grant contractb.SectorGrant
	}
	items := make([]item, 0, len(s.peers))
	for _, res := range s.ring.Order {
		p, live := s.peers[res.PeerID]
		if !live {
			continue
		}
		g := contractb.SectorGrant{
			Granted:  true,
			Slot:     res.Slot,
			RingSize: s.ring.Size(),
			Reason:   contractb.GrantUpdated,
		}
		if res.PeerID == claimant {
			g.Reason = claimReason
		}
		if east, ok := s.ring.East(res.PeerID); ok {
			m := s.metaLocked(east.PeerID)
			_, eastLive := s.peers[east.PeerID]
			g.EastNeighbour = &contractb.Neighbour{
				Slot:           east.Slot,
				PeerID:         east.PeerID,
				Live:           eastLive,
				ModConnected:   eastLive && m.modConnected,
				GameVersion:    m.gameVersion,
				SimulationSize: m.simSize,
			}
		}
		items = append(items, item{p: p, grant: g})
	}
	s.mu.Unlock()

	for _, it := range items {
		body, err := json.Marshal(it.grant)
		if err != nil {
			continue
		}
		it.p.mu.Lock()
		same := it.p.lastGrant == string(body)
		it.p.lastGrant = string(body)
		it.p.mu.Unlock()
		if same {
			continue
		}
		s.send(it.p, contractb.TypeSectorGrant, it.grant)
	}
}

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
			Message:     "this connection is a read-only archive subscriber and holds no ring slot",
		})
		s.log.Warn("relay: refused a migration from a read-only subscriber", "peer", p.id)
		return true
	}
	if routing.DestSlot < 1 {
		p.conn.Close(contractb.CloseMalformedFrame, "destSlot is not a ring slot")
		return false
	}

	s.mu.Lock()
	owner := s.ring.PeerOfSlot(routing.DestSlot)
	dest := s.peers[owner]
	lastSeen := int64(0)
	if m, ok := s.meta[owner]; ok {
		lastSeen = m.lastSeenMs
	}
	s.mu.Unlock()

	if dest == nil {
		// §5: answer the sender rather than drop the frame. A dropped frame
		// turns a bounded failure into a stall.
		msg := "ring slot " + itoa(routing.DestSlot) + " has no live peer"
		if owner != "" {
			msg = "ring slot " + itoa(routing.DestSlot) + " is reserved to " + owner +
				", which is not connected"
			if lastSeen > 0 {
				msg += " (last seen " + itoa(int((time.Now().UnixMilli()-lastSeen)/1000)) + "s ago)"
			}
		}
		s.send(p, contractb.TypeMigrationNack, contractb.MigrationNack{
			MigrationID:  id.MigrationID,
			SourcePeer:   "",
			DestPeer:     p.id,
			Code:         contractb.NackSlotVacant,
			Class:        contractb.ClassTransient,
			Message:      msg,
			RetryAfterMs: 15000,
		})
		return true
	}
	s.forward(dest, frame)
	s.fanOut(frame)
	return true
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
			// Bounded: on overflow drop the oldest copy, count it, and log at
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
	}
	slot := s.ring.SlotOfPeer(p.id)
	s.mu.Unlock()
	s.log.Info("relay: client gone", "peer", p.id, "role", p.role, "slot", slot,
		"reservationKept", slot > 0)
	// §8: the vacancy ripples one way. The dead peer's west neighbour closes
	// its export edge; the slot itself stays in the ring, reserved, live:false.
	s.broadcastPeerStatus()
	s.sendGrants("", contractb.GrantUpdated)
}

func (s *Server) broadcastPeerStatus() {
	s.mu.Lock()
	slots := make([]contractb.SlotInfo, 0, s.ring.Size())
	for _, res := range s.ring.Order {
		m := s.metaLocked(res.PeerID)
		_, live := s.peers[res.PeerID]
		slots = append(slots, contractb.SlotInfo{
			Slot:           res.Slot,
			PeerID:         res.PeerID,
			Live:           live,
			ModConnected:   live && m.modConnected,
			GameVersion:    m.gameVersion,
			SimulationSize: m.simSize,
			LastSeenMs:     m.lastSeenMs,
			LastRefusal:    m.lastRefusal,
		})
	}
	observers := len(s.subscribers)
	type target struct {
		p  *peer
		me contractb.You
	}
	targets := make([]target, 0, len(s.peers)+len(s.subscribers))
	for _, p := range s.peers {
		var me contractb.You
		if slot := s.ring.SlotOfPeer(p.id); slot > 0 {
			mine := slot
			me.Slot = &mine
		}
		if east, ok := s.ring.East(p.id); ok {
			e := east.Slot
			me.EastNeighbourSlot = &e
		}
		targets = append(targets, target{p: p, me: me})
	}
	for _, p := range s.subscribers {
		// §6.5: both are null for a subscriber.
		targets = append(targets, target{p: p})
	}
	ringSize := s.ring.Size()
	s.mu.Unlock()

	for _, t := range targets {
		t.p.mu.Lock()
		t.p.epoch++
		epoch := t.p.epoch
		t.p.mu.Unlock()
		s.send(t.p, contractb.TypePeerStatus, contractb.PeerStatus{
			Epoch:     epoch,
			RingSize:  ringSize,
			Slots:     slots,
			You:       t.me,
			Observers: observers,
		})
	}
}

func (s *Server) send(p *peer, typ string, data any) {
	frame, err := wire.Encode(wire.ProtocolB, typ, time.Now().UnixMilli(), data)
	if err != nil {
		s.log.Error("relay: encode failed", "type", typ, "err", err)
		return
	}
	if err := p.conn.Send(frame); err != nil {
		s.log.Warn("relay: send failed", "peer", p.id, "type", typ, "err", err)
	}
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

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
