package sidecar

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/bb8"
	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/lantoken"
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

// relayLoop keeps a Contract B link to the relay up, reconnecting with
// exponential backoff and full jitter (contract-b-m3.md §3).
//
// A run of HTTP 401s pins the backoff at the ceiling: a wrong token is an
// operator problem and hammering the relay will not fix it (§3.1).
func (s *Sidecar) relayLoop() {
	attempt := 0
	authFailures := 0
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		started := time.Now()
		err := s.relaySession()
		if err != nil && !errors.Is(err, context.Canceled) {
			s.log.Warn("contract B: relay session ended", "err", err)
		}
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		if isUnauthorized(err) {
			authFailures++
			s.log.Error("contract B: the relay rejected our LAN token with HTTP 401",
				"consecutiveFailures", authFailures,
				"hint", "check MULTIVERSE_TOKEN or --token-file on both sides")
		} else {
			authFailures = 0
		}
		if time.Since(started) >= contractb.StableSession {
			// §3, contract-a.md §13 A8: the ladder resets only after a session
			// that stayed up.
			attempt = 0
		}
		attempt++
		wait := fullJitter(s.cfg.RelayBackoffMin, s.cfg.RelayBackoffMax, attempt)
		if authFailures >= contractb.AuthFailuresBeforeCeiling {
			wait = s.cfg.RelayBackoffMax
		}
		time.Sleep(wait)
	}
}

// isUnauthorized recognises the relay's HTTP 401. coder/websocket carries the
// status only in the dial error's text, so the string is the signal available.
func isUnauthorized(err error) bool {
	return err != nil && strings.Contains(err.Error(), "401")
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
	opts := &websocket.DialOptions{CompressionMode: websocket.CompressionDisabled}
	if s.cfg.Token != "" {
		// §3.1: the token rides the HTTP upgrade and nothing token-related ever
		// appears in a frame.
		opts.HTTPHeader = http.Header{"Authorization": []string{lantoken.Header(s.cfg.Token)}}
	}
	ws, _, err := websocket.Dial(dialCtx, s.cfg.RelayURL, opts)
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
	s.east = nil
	s.ring = nil
	hs := contractb.Handshake{
		PeerID:          s.cfg.PeerID,
		Role:            contractb.RolePeer,
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
	s.east = nil
	s.ring = nil
	// §8: with the link down the sidecar knows nothing about its east
	// neighbour, so the export edge closes as peer_unreachable.
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
			slot := 0
			if ack.AssignedSlot != nil {
				slot = *ack.AssignedSlot
			}
			s.log.Info("contract B: handshake accepted", "relayVersion", ack.RelayVersion,
				"assignedSlot", slot, "ringSize", ack.RingSize, "relayClock", ack.ReceivedAt)
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
	case contractb.TypeGenomeRequest:
		return s.onGenomeRequest(env)
	case contractb.TypeGenomeResponse:
		// A sidecar does not fetch genomes in M3; the archive does. A stray
		// answer is ignored rather than treated as a fault.
		return true
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
		s.log.Error("contract B: ring claim refused", "reason", grant.Reason)
		s.mu.Lock()
		s.slot = 0
		s.east = nil
		s.ringSize = grant.RingSize
		s.publishEdgesLocked(false)
		s.mu.Unlock()
		return true
	}
	s.mu.Lock()
	newSlot := s.slot != grant.Slot
	s.slot = grant.Slot
	s.ringSize = grant.RingSize
	s.east = grant.EastNeighbour
	s.cfg.PreferredSlot = grant.Slot
	var mismatch string
	if s.mod != nil && s.mod.handshaked {
		mismatch = s.slotMismatchLocked(s.mod)
	}
	mod := s.mod
	s.publishEdgesLocked(false)
	s.mu.Unlock()
	s.writeSlot(grant.Slot)
	east := 0
	eastPeer := ""
	if grant.EastNeighbour != nil {
		east = grant.EastNeighbour.Slot
		eastPeer = grant.EastNeighbour.PeerID
	}
	if newSlot || grant.Reason != contractb.GrantUpdated {
		s.log.Info("contract B: ring slot granted", "slot", grant.Slot, "reason", grant.Reason,
			"ringSize", grant.RingSize, "eastSlot", east, "eastPeer", eastPeer)
	}
	if mismatch != "" && mod != nil {
		// contract-a.md §14 A14: a mis-wired rig is caught in one second.
		mod.conn.Close(contracta.CloseSlotMismatch, mismatch)
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
		// §6.5: ignore an epoch lower than or equal to the last applied one.
		s.mu.Unlock()
		return true
	}
	s.peerEpoch = status.Epoch
	s.ringSize = status.RingSize
	s.ring = status.Slots
	if status.You.Slot != nil {
		s.slot = *status.You.Slot
	}
	// The east neighbour is read out of the ring order, which is the whole
	// topology a sidecar needs (D8). PEER_STATUS is full state, so the previous
	// view is discarded outright.
	s.east = nil
	if status.You.EastNeighbourSlot != nil {
		for _, slot := range status.Slots {
			if slot.Slot != *status.You.EastNeighbourSlot {
				continue
			}
			s.east = &contractb.Neighbour{
				Slot:           slot.Slot,
				PeerID:         slot.PeerID,
				Live:           slot.Live,
				ModConnected:   slot.ModConnected,
				GameVersion:    slot.GameVersion,
				SimulationSize: slot.SimulationSize,
			}
			break
		}
	}
	s.publishEdgesLocked(false)
	s.mu.Unlock()
	s.log.Info("contract B: ring status", "epoch", status.Epoch, "ringSize", status.RingSize,
		"observers", status.Observers)
	return true
}

// onMigrationPayload implements contract-b-m3.md §6.6's seven receiver
// obligations, in order.
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
			"kind "+payload.Kind+" is not supported in M3")
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
		hasMod := s.mod != nil && s.mod.handshaked
		s.mu.Unlock()
		// §6.8: MOD_ABSENT is the code for a full queue behind a sim that has
		// no mod to drain it; OVERLOADED is the code when the mod is simply
		// behind. Both are transient and the sender keeps custody either way.
		code := contractb.NackOverloaded
		message := "inboundQueueMax reached"
		if !hasMod {
			code = contractb.NackModAbsent
			message = "no mod is connected and inboundQueueMax is reached"
		}
		s.nackUpstream(payload.MigrationID, payload.SourcePeer, code, message)
		return true
	}
	// 3. Check S against our own, by the relative test of contract-a.md §13 A10.
	// The sender is a slot in the ring — this peer's west neighbour under
	// ordinary traffic — so the size comes out of the ring order, not out of
	// the east-neighbour view.
	if s.mod != nil && s.mod.handshaked {
		if si, ok := s.slotInfoLocked(payload.SourceSlot); ok && si.SimulationSize > 0 &&
			!sameSize(si.SimulationSize, s.mod.simSize) {
			s.mu.Unlock()
			s.nackUpstream(payload.MigrationID, payload.SourcePeer, contractb.NackSimSizeMismatch,
				"the two sims disagree about simulationSize")
			return true
		}
	}
	// The entry edge is derived, never carried: sending it would let a sender
	// dictate a receiver's geometry (§11 item 7). Under the ring it is the
	// passive edge facing this sim's export edge.
	entryEdge := contracta.EdgeW
	if s.mod != nil && s.mod.exportEdge != "" {
		if opp, ok := contracta.Opposite(s.mod.exportEdge); ok {
			entryEdge = opp
		}
	}
	s.mu.Unlock()

	// 4. bb8-schema validation. Nothing invalid ever reaches a mod.
	if err := bb8.Validate(payload.Body.Version, payload.Body.BB8); err != nil {
		s.log.Error("contract B: payload rejected", "migrationId", payload.MigrationID, "err", err)
		s.nackUpstream(payload.MigrationID, payload.SourcePeer, contractb.NackInvalidPayload, err.Error())
		return true
	}

	// contract-a.md §5.7: entityId comes out of the blob when bb8-schema can
	// read one; the wire value is the fallback (contract-b-m3.md §11 item 3).
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

	// 5. Durable journal write. Custody moves here.
	entry := journal.Entry{
		MigrationID: payload.MigrationID,
		EntityID:    entityID,
		Kind:        payload.Kind,
		GameVersion: payload.Body.Version,
		Payload:     payload.Body.BB8,
		PayloadHash: bb8.Hash(payload.Body.BB8),
		Edge:        entryEdge,
		Position:    payload.ExitPosition,
		VelocityX:   payload.Velocity.X,
		VelocityY:   payload.Velocity.Y,
		Heading:     heading,
		SourcePeer:  payload.SourcePeer,
		SourceSlot:  payload.SourceSlot,
		DestSlot:    payload.DestSlot,
		GenomeHash:  payload.Lineage.GenomeHash,
		Parents:     parentRefs(payload.Lineage.Parents),
		JournaledAt: time.Now().UnixMilli(),
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
		"migrationId", payload.MigrationID, "entityId", entityID, "entryEdge", entryEdge,
		"genomeHash", payload.Lineage.GenomeHash)
	// 7. Deliver, and keep replaying until the mod ACKs.
	if s.mod != nil && s.mod.handshaked {
		s.deliverLocked(st, time.Now())
	}
	s.mu.Unlock()

	// 6. Cache the migrant's genome so this peer can serve it later (§10).
	// The receiver never recomputes lineage.genomeHash as a gate — a mismatch
	// is a bb8-schema defect to shout about, not a reason to refuse an
	// organism, because custody rules outrank bookkeeping.
	if payload.Lineage.GenomeHash != "" {
		if got, err := bb8.GenomeHash(payload.Body.BB8, payload.Body.Version); err == nil &&
			got != payload.Lineage.GenomeHash {
			s.log.Error("contract B: lineage.genomeHash disagrees with the blob — this is a bb8-schema defect",
				"migrationId", payload.MigrationID, "annex", payload.Lineage.GenomeHash, "computed", got)
		}
		s.cacheGenome(payload.Lineage.GenomeHash, payload.Body.Version, payload.Body.BB8)
	}
	return true
}

func parentRefs(in []contractb.Parent) []journal.ParentRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]journal.ParentRef, 0, len(in))
	for _, p := range in {
		out = append(out, journal.ParentRef{
			EntityID: p.EntityID, GenomeHash: p.GenomeHash, GapReason: p.GapReason})
	}
	return out
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
	// §6.7: the tombstone keeps the genome hash, because the archive may ask
	// for that genome long after the migration completed.
	s.log.Info("contract B: MIGRATION_ACK, journal entry became a tombstone",
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
		// §9: a NACK proves custody never moved, so the bounce cannot duplicate.
		s.bounceLocked(st, "MIGRATION_NACK "+nack.Code)
		return true
	}
	sc := s.schedFor(nack.MigrationID)
	if nack.Code == contractb.NackSlotVacant || nack.Code == contractb.NackPeerUnknown {
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

// onGenomeRequest answers exactly one GENOME_RESPONSE from the genome cache
// (contract-b-m3.md §6.9). It never blocks a migration to serve one, and a
// request it cannot serve is a normal answer rather than an error.
func (s *Sidecar) onGenomeRequest(env wire.Envelope) bool {
	var req contractb.GenomeRequest
	if err := contractb.DecodeData(env.Data, &req); err != nil {
		return true
	}
	if req.DestPeer != s.cfg.PeerID {
		return true
	}
	resp := contractb.GenomeResponse{
		RequestID:  req.RequestID,
		SourcePeer: s.cfg.PeerID,
		DestPeer:   req.SourcePeer,
		GenomeHash: req.GenomeHash,
	}
	if !s.allowGenomeRequest(req.SourcePeer) {
		// §10: at most genomeRequestsPerMinute from one requester to one peer,
		// enforced on both sides.
		resp.Found = false
		resp.Reason = contractb.GenomeRateLimited
		resp.RetryAfterMs = 60000
	} else if e, ok := s.genomes.Get(req.GenomeHash); ok {
		resp.Found = true
		resp.Body = &contractb.Body{Version: e.Version, BB8: e.BB8}
	} else {
		// unknown_hash is a normal answer, not an error (§6.10).
		resp.Found = false
		resp.Reason = contractb.GenomeUnknownHash
	}
	s.mu.Lock()
	s.sendRelayLocked(contractb.TypeGenomeResponse, resp)
	s.mu.Unlock()
	s.log.Info("contract B: answered GENOME_REQUEST", "requester", req.SourcePeer,
		"genomeHash", req.GenomeHash, "found", resp.Found, "reason", resp.Reason)
	return true
}

func (s *Sidecar) allowGenomeRequest(requester string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	w, ok := s.genomeServed[requester]
	if !ok || now.Sub(w.windowStart) > time.Minute {
		s.genomeServed[requester] = &rateWindow{windowStart: now, count: 1}
		return true
	}
	if w.count >= s.cfg.GenomeRequestsPerMinute {
		return false
	}
	w.count++
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

// refreshClaim re-sends SECTOR_CLAIM so the relay — and through it the west
// neighbour — learns the current simulationSize, export edge, border edges and
// mod presence (contract-b-m3.md §6.3).
func (s *Sidecar) refreshClaim() {
	s.mu.Lock()
	defer s.mu.Unlock()
	claim := contractb.SectorClaim{PreferredSlot: s.cfg.PreferredSlot}
	if s.mod != nil && s.mod.handshaked {
		claim.SimulationSize = s.mod.simSize
		claim.ExportEdge = s.mod.exportEdge
		claim.BorderEdges = append([]string(nil), s.mod.borderEdges...)
		claim.GameVersion = s.mod.gameVersion
		claim.ModConnected = true
	} else {
		// §6.3: empty while no mod is connected. A sidecar with no mod cannot
		// spawn an organism, and its west neighbour's export edge closes.
		claim.BorderEdges = []string{}
	}
	s.sendRelayLocked(contractb.TypeSectorClaim, claim)
}
