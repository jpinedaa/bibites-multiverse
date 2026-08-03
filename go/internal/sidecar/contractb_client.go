package sidecar

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/bb8"
	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

// relayLoop keeps a Contract B link to the relay up, reconnecting with
// exponential backoff and full jitter (contract-b-m2.md §2).
func (s *Sidecar) relayLoop() {
	attempt := 0
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		err := s.relaySession()
		if err != nil && !errors.Is(err, context.Canceled) {
			s.log.Warn("contract B: relay session ended", "err", err)
		}
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		attempt++
		time.Sleep(fullJitter(s.cfg.RelayBackoffMin, s.cfg.RelayBackoffMax, attempt))
	}
}

func fullJitter(min, max time.Duration, attempt int) time.Duration {
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

func (s *Sidecar) relaySession() error {
	dialCtx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	ws, _, err := websocket.Dial(dialCtx, s.cfg.RelayURL, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	cancel()
	if err != nil {
		s.dropRelay()
		return err
	}
	conn := wsutil.New(ws, 128)
	defer func() {
		conn.Close(contractb.CloseNormal, "sidecar closing the relay link")
		<-conn.Done()
		s.dropRelay()
	}()

	s.mu.Lock()
	s.relayConn = conn
	s.relayReady = true
	s.peerEpoch = 0
	s.peers = map[string]contractb.PeerInfo{}
	hs := contractb.Handshake{
		PeerID:          s.cfg.PeerID,
		ProtocolVersion: wire.ProtocolB,
		SidecarVersion:  Version,
	}
	if s.mod != nil && s.mod.handshaked {
		hs.GameVersion = s.mod.gameVersion
		hs.SimulationSize = s.mod.simSize
	}
	ok := s.sendRelayLocked(contractb.TypeHandshake, hs)
	s.mu.Unlock()
	if !ok {
		return errors.New("contract B: HANDSHAKE send failed")
	}
	s.log.Info("contract B: connected to the relay", "url", s.cfg.RelayURL)
	s.refreshClaim()

	for {
		readCtx, readCancel := context.WithCancel(s.ctx)
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
		if !s.handleRelayFrame(conn, frame) {
			return errors.New("contract B: closing the relay link")
		}
	}
}

func (s *Sidecar) dropRelay() {
	s.mu.Lock()
	s.relayConn = nil
	s.relayReady = false
	s.peers = map[string]contractb.PeerInfo{}
	// contract-b-m2.md §5.5: with the link down the sidecar knows nothing
	// about its neighbour, so every edge closes as peer_unreachable.
	s.publishEdgesLocked(false)
	s.mu.Unlock()
}

func (s *Sidecar) handleRelayFrame(conn *wsutil.Conn, frame []byte) bool {
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
		var ack contractb.HandshakeAck
		if contractb.DecodeData(env.Data, &ack) == nil {
			s.log.Info("contract B: handshake accepted", "relayVersion", ack.RelayVersion,
				"assignedSector", ack.AssignedSector)
		}
		return true
	case contractb.TypeSectorGrant:
		return s.onSectorGrant(env)
	case contractb.TypePeerStatus:
		return s.onPeerStatus(env)
	case contractb.TypeMigrationPayload:
		return s.onMigrationPayload(env)
	case contractb.TypeMigrationAck:
		return s.onMigrationAck(env)
	case contractb.TypeMigrationNack:
		return s.onMigrationNack(env)
	case contractb.TypePing:
		var ping contractb.Ping
		if contractb.DecodeData(env.Data, &ping) == nil {
			s.mu.Lock()
			s.sendRelayLocked(contractb.TypePong, contractb.Pong{Nonce: ping.Nonce})
			s.mu.Unlock()
		}
		return true
	case contractb.TypePong:
		return true
	default:
		s.log.Warn("contract B: ignoring unknown type", "type", env.Type)
		return true
	}
}

func (s *Sidecar) onSectorGrant(env wire.Envelope) bool {
	var grant contractb.SectorGrant
	if err := contractb.DecodeData(env.Data, &grant); err != nil {
		return true
	}
	if !grant.Granted {
		s.log.Error("contract B: sector claim refused", "reason", grant.Reason)
		s.mu.Lock()
		s.sector = ""
		s.publishEdgesLocked(false)
		s.mu.Unlock()
		return true
	}
	s.mu.Lock()
	s.sector = grant.Sector
	s.cfg.PreferredSector = grant.Sector
	var mismatch string
	if s.mod != nil && s.mod.handshaked {
		mismatch = s.sectorMismatchLocked(s.mod)
	}
	mod := s.mod
	s.publishEdgesLocked(false)
	s.mu.Unlock()
	s.writeSector(grant.Sector)
	s.log.Info("contract B: sector granted", "sector", grant.Sector, "reason", grant.Reason)
	if mismatch != "" && mod != nil {
		// contract-a.md §5.1: a mis-wired rig is caught in one second.
		mod.conn.Close(contracta.CloseSectorMismatch, mismatch)
	}
	return true
}

func (s *Sidecar) onPeerStatus(env wire.Envelope) bool {
	var status contractb.PeerStatus
	if err := contractb.DecodeData(env.Data, &status); err != nil {
		return true
	}
	s.mu.Lock()
	if status.Epoch <= s.peerEpoch {
		s.mu.Unlock()
		return true
	}
	s.peerEpoch = status.Epoch
	peers := map[string]contractb.PeerInfo{}
	for _, p := range status.Peers {
		if p.PeerID == s.cfg.PeerID {
			continue
		}
		peers[p.Sector] = p
	}
	s.peers = peers
	s.publishEdgesLocked(false)
	s.mu.Unlock()
	s.log.Info("contract B: peer status", "epoch", status.Epoch, "peers", len(peers))
	return true
}

// onMigrationPayload implements contract-b-m2.md §5.6's five receiver
// obligations.
func (s *Sidecar) onMigrationPayload(env wire.Envelope) bool {
	var payload contractb.MigrationPayload
	if err := contractb.DecodeData(env.Data, &payload); err != nil {
		s.nackUpstream(payload.MigrationID, "", contractb.NackMalformedMessage, err.Error())
		return true
	}
	if err := payload.Validate(); err != nil {
		s.nackUpstream(payload.MigrationID, payload.SourcePeer, contractb.NackMalformedMessage, err.Error())
		return true
	}
	if payload.Kind != contracta.KindBibite {
		s.nackUpstream(payload.MigrationID, payload.SourcePeer, contractb.NackKindUnsupported,
			"kind "+payload.Kind+" is not supported in M2")
		return true
	}

	s.mu.Lock()
	// 1. Dedup on migrationId against the journal and its tombstones.
	if st, ok := s.jr.Get(payload.MigrationID); ok {
		s.mu.Unlock()
		s.log.Info("contract B: duplicate MIGRATION_PAYLOAD, re-ACKing without delivery",
			"migrationId", payload.MigrationID, "status", st.Status)
		s.ackUpstreamNow(payload.MigrationID, payload.SourcePeer, st.Entry.EntityID, true)
		return true
	}
	// 2. Admission control.
	if s.jr.CountPending(journal.In) >= s.cfg.InboundQueueMax {
		s.mu.Unlock()
		s.nackUpstream(payload.MigrationID, payload.SourcePeer, contractb.NackOverloaded,
			"inboundQueueMax reached")
		return true
	}
	// The peer's S is known from PEER_STATUS; refuse a transfer across a
	// resize (contract-a.md §5.3 step 4, mirrored at the receiving end).
	if s.mod != nil && s.mod.handshaked {
		if peer, ok := s.peers[payload.SourceSector]; ok && peer.SimulationSize > 0 &&
			!sameSize(peer.SimulationSize, s.mod.simSize) {
			s.mu.Unlock()
			s.nackUpstream(payload.MigrationID, payload.SourcePeer, contractb.NackSimSizeMismatch,
				"the two sims disagree about simulationSize")
			return true
		}
	}
	s.mu.Unlock()

	// 3. bb8-schema validation. Nothing invalid ever reaches a mod.
	if err := bb8.Validate(payload.Body.Version, payload.Body.BB8); err != nil {
		s.log.Error("contract B: payload rejected", "migrationId", payload.MigrationID, "err", err)
		s.nackUpstream(payload.MigrationID, payload.SourcePeer, contractb.NackInvalidPayload, err.Error())
		return true
	}

	entryEdge, ok := contracta.Opposite(payload.ExitEdge)
	if !ok {
		s.nackUpstream(payload.MigrationID, payload.SourcePeer, contractb.NackMalformedMessage,
			"exitEdge "+payload.ExitEdge+" is not N/S/E/W")
		return true
	}

	// contract-a.md §5.7: entityId comes out of the blob. The wire value is the
	// fallback while bb8-schema is a skeleton (contract-b-m2.md §8 item 2).
	entityID := payload.EntityID
	heading := payload.Heading
	info := bb8.Inspect(payload.Body.BB8)
	if info.HasEntityID {
		if entityID != 0 && info.EntityID != entityID {
			s.log.Warn("contract B: entityId in the blob differs from the wire field",
				"blob", info.EntityID, "wire", entityID)
		}
		entityID = info.EntityID
	}
	if info.HasHeading && heading == 0 {
		heading = info.Heading
	}

	// 4. Durable journal write. Custody moves here.
	entry := journal.Entry{
		MigrationID:    payload.MigrationID,
		EntityID:       entityID,
		Kind:           payload.Kind,
		GameVersion:    payload.Body.Version,
		Payload:        payload.Body.BB8,
		PayloadHash:    bb8.Hash(payload.Body.BB8),
		Edge:           entryEdge,
		Position:       payload.ExitPosition, // M2 sectors are pure translations
		VelocityX:      payload.Velocity.X,
		VelocityY:      payload.Velocity.Y,
		Heading:        heading,
		SimulationSize: 0,
		SourcePeer:     payload.SourcePeer,
		SourceSector:   payload.SourceSector,
		DestSector:     payload.DestSector,
		JournaledAt:    time.Now().UnixMilli(),
	}
	s.mu.Lock()
	st, err := s.jr.Create(journal.In, entry, false)
	if err != nil {
		s.mu.Unlock()
		s.log.Error("contract B: inbound journal write failed", "migrationId", payload.MigrationID, "err", err)
		s.nackUpstream(payload.MigrationID, payload.SourcePeer, contractb.NackOverloaded, err.Error())
		return true
	}
	s.log.Info("contract B: took custody of an inbound organism",
		"migrationId", payload.MigrationID, "entityId", entityID, "entryEdge", entryEdge)
	// 5. Deliver, and keep replaying until the mod ACKs.
	if s.mod != nil && s.mod.handshaked {
		s.deliverLocked(st, time.Now())
	}
	s.mu.Unlock()
	return true
}

func (s *Sidecar) onMigrationAck(env wire.Envelope) bool {
	var ack contractb.MigrationAck
	if err := contractb.DecodeData(env.Data, &ack); err != nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.jr.Get(ack.MigrationID)
	if !ok || st.Direction != journal.Out {
		return true
	}
	if st.Status == journal.StatusDone {
		return true
	}
	completed := time.Now().UnixMilli()
	acked := true
	if _, err := s.jr.Apply(ack.MigrationID, journal.Update{
		Status: journal.StatusDone, CompletedAt: &completed, Acked: &acked,
		Duplicate: &ack.Duplicate}); err != nil {
		s.log.Error("contract B: journal update failed", "migrationId", ack.MigrationID, "err", err)
		return true
	}
	delete(s.sched, ack.MigrationID)
	s.log.Info("contract B: MIGRATION_ACK, journal entry cleared",
		"migrationId", ack.MigrationID, "entityId", ack.EntityID, "duplicate", ack.Duplicate)
	return true
}

func (s *Sidecar) onMigrationNack(env wire.Envelope) bool {
	var nack contractb.MigrationNack
	if err := contractb.DecodeData(env.Data, &nack); err != nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.jr.Get(nack.MigrationID)
	if !ok || st.Direction != journal.Out {
		return true
	}
	if st.Status != journal.StatusOpen && st.Status != journal.StatusInFlight {
		return true
	}
	s.log.Warn("contract B: MIGRATION_NACK", "migrationId", nack.MigrationID,
		"code", nack.Code, "class", nack.Class, "message", nack.Message)

	class := nack.Class
	if class == "" {
		class = contractb.ClassOf(nack.Code)
	}
	if class == contractb.ClassPermanent {
		// contract-b-m2.md §7: a NACK proves custody never moved, so the
		// bounce cannot duplicate.
		s.bounceLocked(st, "MIGRATION_NACK "+nack.Code)
		return true
	}
	sc := s.schedFor(nack.MigrationID)
	if nack.Code == contractb.NackSectorVacant || nack.Code == contractb.NackPeerUnknown {
		// The relay is authoritative that nobody received the frame, so the
		// bounce timer may run again.
		sc.reachedPeer = false
		if sc.bounceAt.IsZero() {
			sc.bounceAt = time.Now().Add(s.cfg.BounceTimeout)
		}
	}
	retry := s.cfg.ForwardRetry
	if nack.RetryAfterMs > 0 {
		retry = time.Duration(nack.RetryAfterMs) * time.Millisecond
	}
	sc.nextForward = time.Now().Add(retry)
	return true
}

// ---------------------------------------------------------------- send helpers

func (s *Sidecar) sendRelayLocked(typ string, data any) bool {
	if s.relayConn == nil || !s.relayReady {
		return false
	}
	frame, err := wire.Encode(wire.ProtocolB, typ, time.Now().UnixMilli(), data)
	if err != nil {
		s.log.Error("contract B: encode failed", "type", typ, "err", err)
		return false
	}
	if err := s.relayConn.Send(frame); err != nil {
		s.log.Warn("contract B: send failed", "type", typ, "err", err)
		return false
	}
	return true
}

func (s *Sidecar) nackUpstream(migrationID, destPeer, code, message string) {
	if destPeer == "" {
		s.log.Warn("contract B: cannot NACK, no sourcePeer", "migrationId", migrationID, "code", code)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sendRelayLocked(contractb.TypeMigrationNack, contractb.MigrationNack{
		MigrationID: migrationID,
		SourcePeer:  s.cfg.PeerID,
		DestPeer:    destPeer,
		Code:        code,
		Class:       contractb.ClassOf(code),
		Message:     message,
	})
}

func (s *Sidecar) ackUpstreamNow(migrationID, destPeer string, entityID int32, duplicate bool) {
	if destPeer == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sendRelayLocked(contractb.TypeMigrationAck, contractb.MigrationAck{
		MigrationID: migrationID,
		SourcePeer:  s.cfg.PeerID,
		DestPeer:    destPeer,
		EntityID:    entityID,
		Duplicate:   duplicate,
		DeliveredAt: time.Now().UnixMilli(),
	})
}

// refreshClaim re-sends SECTOR_CLAIM so the relay — and through it the
// neighbour — learns the current simulationSize, border edges and mod presence
// (contract-b-m2.md §5.3).
func (s *Sidecar) refreshClaim() {
	s.mu.Lock()
	defer s.mu.Unlock()
	claim := contractb.SectorClaim{PreferredSector: s.cfg.PreferredSector}
	if s.mod != nil && s.mod.handshaked {
		claim.SimulationSize = s.mod.simSize
		claim.BorderEdges = append([]string(nil), s.mod.borderEdges...)
		claim.GameVersion = s.mod.gameVersion
		claim.ModConnected = true
	} else {
		claim.BorderEdges = []string{}
	}
	s.sendRelayLocked(contractb.TypeSectorClaim, claim)
}
