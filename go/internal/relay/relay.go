// Package relay implements multiverse-relay: the deliberately dumb M2
// transport (D1). It forwards Contract B frames between two sidecars, arbitrates
// the two-sector map, and never parses a bb8 body (D4).
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
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

// Version is reported in HANDSHAKE_ACK.
const Version = "m2.0"

// Options configures a Server.
type Options struct {
	Logger       *slog.Logger
	PingInterval time.Duration
	PeerTimeout  time.Duration
}

// Server is the relay. The zero value is not usable; call New.
type Server struct {
	log          *slog.Logger
	pingInterval time.Duration
	peerTimeout  time.Duration

	mu sync.Mutex
	// live peers by peer id.
	peers map[string]*peer
	// sector -> peer id. Sticky across a disconnect so a reconnecting peer
	// reclaims what it held (contract-b-m2.md §5.3 rule 1).
	owner map[string]string
}

type peer struct {
	id           string
	conn         *wsutil.Conn
	sector       string
	gameVersion  string
	simSize      float64
	modConnected bool
	epoch        int64
	lastSeen     time.Time
	mu           sync.Mutex
}

// New builds a relay.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.PingInterval <= 0 {
		opts.PingInterval = contractb.RelayPingInterval
	}
	if opts.PeerTimeout <= 0 {
		opts.PeerTimeout = contractb.PeerTimeout
	}
	return &Server{
		log:          opts.Logger,
		pingInterval: opts.PingInterval,
		peerTimeout:  opts.PeerTimeout,
		peers:        map[string]*peer{},
		owner:        map[string]string{},
	}
}

// Drain closes every peer connection with 4005. A hijacked WebSocket is not
// tracked by net/http, so http.Server.Shutdown alone would leave peers hanging.
func (s *Server) Drain() {
	s.mu.Lock()
	peers := make([]*peer, 0, len(s.peers))
	for _, p := range s.peers {
		peers = append(peers, p)
	}
	s.mu.Unlock()
	for _, p := range peers {
		p.conn.Close(contractb.CloseShuttingDown, "relay draining")
	}
}

// Handler returns the relay's HTTP handler, serving contract-b-m2.md's path.
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
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode:    websocket.CompressionDisabled,
		InsecureSkipVerify: true, // loopback rig; origin checks arrive with M3 auth
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
		p.touch()
		if !s.dispatch(p, frame) {
			<-conn.Done()
			return
		}
	}
}

// handshake reads the mandatory first frame and registers the peer.
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

	p := &peer{id: hs.PeerID, conn: conn, gameVersion: hs.GameVersion,
		simSize: hs.SimulationSize, lastSeen: time.Now()}

	s.mu.Lock()
	if old, ok := s.peers[hs.PeerID]; ok {
		// contract-b-m2.md §5.1: a newer connection with a live peer id takes
		// over. This makes a crashed-and-restarted sidecar self-healing.
		old.conn.Close(contractb.CloseReplaced, "a newer connection claimed this peerId")
	}
	s.peers[hs.PeerID] = p
	p.sector = s.sectorOfLocked(hs.PeerID)
	s.mu.Unlock()

	s.send(p, contractb.TypeHandshakeAck, contractb.HandshakeAck{
		RelayVersion:    Version,
		ProtocolVersion: wire.ProtocolB,
		AssignedSector:  p.sector,
	})
	s.log.Info("relay: peer connected", "peer", p.id, "sector", p.sector)
	s.broadcastPeerStatus()
	return p, nil
}

func (s *Server) sectorOfLocked(peerID string) string {
	for sector, owner := range s.owner {
		if owner == peerID {
			return sector
		}
	}
	return ""
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
		return s.onDirected(p, env, frame)
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

	s.mu.Lock()
	sector, reason := s.assignLocked(p.id, claim.PreferredSector)
	s.mu.Unlock()

	p.mu.Lock()
	p.sector = sector
	p.simSize = claim.SimulationSize
	p.modConnected = claim.ModConnected
	if claim.GameVersion != "" {
		p.gameVersion = claim.GameVersion
	}
	p.mu.Unlock()

	s.send(p, contractb.TypeSectorGrant, contractb.SectorGrant{
		Granted: sector != "",
		Sector:  sector,
		Reason:  reason,
	})
	s.log.Info("relay: sector claim", "peer", p.id, "sector", sector, "reason", reason,
		"simulationSize", claim.SimulationSize, "modConnected", claim.ModConnected)
	s.broadcastPeerStatus()
	return true
}

// assignLocked implements contract-b-m2.md §5.3's arbitration.
func (s *Server) assignLocked(peerID, preferred string) (string, string) {
	if sector := s.sectorOfLocked(peerID); sector != "" {
		return sector, contractb.GrantReclaimed
	}
	live := func(sector string) bool {
		owner, ok := s.owner[sector]
		if !ok {
			return false
		}
		_, connected := s.peers[owner]
		return connected
	}
	take := func(sector string) (string, string) {
		s.owner[sector] = peerID
		return sector, contractb.GrantGranted
	}
	if preferred != "" {
		if _, owned := s.owner[preferred]; !owned {
			return take(preferred)
		}
	}
	for _, sector := range contractb.Sectors {
		if _, owned := s.owner[sector]; !owned {
			return take(sector)
		}
	}
	// Every sector has an owner on record. Reuse one whose owner is offline,
	// preferring the requested sector.
	if preferred != "" && !live(preferred) {
		return take(preferred)
	}
	for _, sector := range contractb.Sectors {
		if !live(sector) {
			return take(sector)
		}
	}
	return "", contractb.GrantNoSectorAvailable
}

// onMigrationPayload routes on data.destSector and nothing else. The frame is
// forwarded byte for byte; body.bb8 is never decoded (contract-b-m2.md §4).
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
	if !contractb.ValidSector(routing.DestSector) {
		p.conn.Close(contractb.CloseMalformedFrame, "destSector is not A/B")
		return false
	}

	dest := s.peerInSector(routing.DestSector)
	if dest == nil {
		var migrationID struct {
			MigrationID string `json:"migrationId"`
		}
		_ = json.Unmarshal(env.Data, &migrationID)
		s.send(p, contractb.TypeMigrationNack, contractb.MigrationNack{
			MigrationID:  migrationID.MigrationID,
			SourcePeer:   "",
			DestPeer:     p.id,
			Code:         contractb.NackSectorVacant,
			Class:        contractb.ClassTransient,
			Message:      "no live peer holds sector " + routing.DestSector,
			RetryAfterMs: 5000,
		})
		return true
	}
	s.forward(dest, frame)
	return true
}

// onDirected routes MIGRATION_ACK and MIGRATION_NACK on data.destPeer.
func (s *Server) onDirected(p *peer, env wire.Envelope, frame []byte) bool {
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
		s.log.Warn("relay: dropping frame for absent peer", "type", env.Type, "destPeer", routing.DestPeer)
		return true
	}
	s.forward(dest, frame)
	return true
}

func (s *Server) forward(dest *peer, frame []byte) {
	if err := dest.conn.Send(frame); err != nil {
		s.log.Warn("relay: forward failed", "peer", dest.id, "err", err)
	}
}

func (s *Server) peerInSector(sector string) *peer {
	s.mu.Lock()
	defer s.mu.Unlock()
	owner, ok := s.owner[sector]
	if !ok {
		return nil
	}
	return s.peers[owner]
}

func (s *Server) drop(p *peer) {
	s.mu.Lock()
	if cur, ok := s.peers[p.id]; ok && cur == p {
		delete(s.peers, p.id)
	}
	s.mu.Unlock()
	s.log.Info("relay: peer gone, sector vacant", "peer", p.id, "sector", p.sector)
	// contract-b-m2.md §5.5: the vacancy ripples to the survivor, which closes
	// the paired edge and pushes EDGE_STATUS to its own mod.
	s.broadcastPeerStatus()
}

func (s *Server) broadcastPeerStatus() {
	s.mu.Lock()
	peers := make([]*peer, 0, len(s.peers))
	for _, p := range s.peers {
		peers = append(peers, p)
	}
	infos := make([]contractb.PeerInfo, 0, len(peers))
	for _, sector := range contractb.Sectors {
		owner, ok := s.owner[sector]
		if !ok {
			continue
		}
		p, live := s.peers[owner]
		if !live {
			continue
		}
		p.mu.Lock()
		infos = append(infos, contractb.PeerInfo{
			PeerID:         p.id,
			Sector:         sector,
			GameVersion:    p.gameVersion,
			SimulationSize: p.simSize,
			ModConnected:   p.modConnected,
		})
		p.mu.Unlock()
	}
	s.mu.Unlock()

	for _, p := range peers {
		p.mu.Lock()
		p.epoch++
		epoch := p.epoch
		p.mu.Unlock()
		s.send(p, contractb.TypePeerStatus, contractb.PeerStatus{Epoch: epoch, Peers: infos})
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

func (p *peer) touch() {
	p.mu.Lock()
	p.lastSeen = time.Now()
	p.mu.Unlock()
}
