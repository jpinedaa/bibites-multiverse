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
// exponential backoff and full jitter (contract-b-m4.md §3).
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
	s.neighbours = map[string]*contractb.Neighbour{}
	s.status = contractb.PeerStatus{}
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

	sessionCtx, stopPing := context.WithCancel(s.ctx)
	go s.statsPingLoop(sessionCtx, conn)
	defer stopPing()

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

// statsPingLoop is §6.11's stats-bearing PING: at most one per statsIntervalMs,
// and it is where a FRESH population comes from. A SECTOR_CLAIM is sent on
// change and would leave the map view showing the population each world had
// when it last resized, which is worse than no number at all.
func (s *Sidecar) statsPingLoop(ctx context.Context, conn *wsutil.Conn) {
	t := time.NewTicker(s.cfg.StatsInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-conn.Done():
			return
		case <-t.C:
			s.mu.Lock()
			stats := s.statsLocked()
			s.sendRelayLocked(contractb.TypePing, contractb.Ping{
				Nonce: wire.NewUUID(), Stats: &stats})
			s.mu.Unlock()
		}
	}
}

func (s *Sidecar) dropRelay() {
	s.mu.Lock()
	s.relayConn = nil
	s.relayReady = false
	s.relaySessionID = ""
	s.neighbours = map[string]*contractb.Neighbour{}
	s.status = contractb.PeerStatus{}
	// §8: with the link down the sidecar knows nothing about its neighbours, so
	// every export edge closes as peer_unreachable. §9.3: the hold clock also
	// stops, because this sidecar is now blind and never observed anything.
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
			s.mu.Lock()
			// §5.2: the session id scopes every non-delivery proof this link can
			// produce. It is recorded against an entry at the first write, not
			// here, but the connection has to know it.
			s.relaySessionID = ack.RelaySessionID
			s.mapShape = ack.Map
			s.slotCount = ack.SlotCount
			s.mu.Unlock()
			slot := 0
			if ack.AssignedSlot != nil {
				slot = *ack.AssignedSlot
			}
			s.log.Info("contract B: handshake accepted", "relayVersion", ack.RelayVersion,
				"assignedSlot", slot, "assignedPosition", ack.AssignedPosition,
				"map", ack.Map, "slotCount", ack.SlotCount,
				"relaySessionId", ack.RelaySessionID, "relayClock", ack.ReceivedAt)
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
		// A sidecar does not fetch genomes in M4; the archive does. A stray
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

// onSectorGrant applies the authority on this peer's routing (§6.4): its slot,
// its position, the map, and ONE EFFECTIVE NEIGHBOUR PER EXPORT EDGE. A key
// absent from neighbours closes that edge.
func (s *Sidecar) onSectorGrant(env wire.Envelope) bool {
	var grant contractb.SectorGrant
	if err := contractb.DecodeData(env.Data, &grant); err != nil {
		return true
	}
	if !grant.Granted {
		s.log.Error("contract B: placement claim refused", "reason", grant.Reason)
		s.mu.Lock()
		s.slot = 0
		s.neighbours = map[string]*contractb.Neighbour{}
		s.mapShape = grant.Map
		s.slotCount = grant.SlotCount
		s.publishEdgesLocked(false)
		s.mu.Unlock()
		return true
	}
	s.mu.Lock()
	newSlot := s.slot != grant.Slot
	moved := grant.Position != nil && s.position != *grant.Position
	s.slot = grant.Slot
	if grant.Position != nil {
		s.position = *grant.Position
	}
	s.mapShape = grant.Map
	s.slotCount = grant.SlotCount
	s.neighbours = map[string]*contractb.Neighbour{}
	for edge, n := range grant.Neighbours {
		if n != nil {
			c := *n
			s.neighbours[edge] = &c
		}
	}
	s.cfg.PreferredSlot = grant.Slot
	if grant.Position != nil {
		p := *grant.Position
		s.cfg.PreferredPosition = &p
	}
	var mismatch string
	if s.mod != nil && s.mod.handshaked {
		mismatch = s.slotMismatchLocked(s.mod)
	}
	mod := s.mod
	position := s.position
	s.publishEdgesLocked(false)
	s.mu.Unlock()

	// §7.4: the slot and the position are written on EVERY granted grant, and
	// read once at startup. Losing them costs a placement mix-up, never an
	// organism.
	s.writeSlot(grant.Slot)
	s.writePosition(position)

	if newSlot || moved || grant.Reason != contractb.GrantUpdated {
		s.log.Info("contract B: slot granted", "slot", grant.Slot, "position", position,
			"reason", grant.Reason, "map", grant.Map, "slotCount", grant.SlotCount,
			"lanes", describeLanes(grant.Neighbours))
	}
	if mismatch != "" && mod != nil {
		// contract-a.md §14 A14: a mis-wired rig is caught in one second.
		mod.conn.Close(contracta.CloseSlotMismatch, mismatch)
	}
	return true
}

// describeLanes renders the effective lanes for a log line a human reads while
// looking at a map, bypasses included.
func describeLanes(n map[string]*contractb.Neighbour) string {
	if len(n) == 0 {
		return "none (every export edge is closed)"
	}
	parts := make([]string, 0, len(n))
	for _, edge := range []string{contracta.EdgeE, contracta.EdgeN} {
		lane, ok := n[edge]
		if !ok || lane == nil {
			continue
		}
		part := edge + "->slot " + itoa(lane.Slot)
		if len(lane.Skipped) > 0 {
			part += " (bypassing"
			for _, sk := range lane.Skipped {
				if sk.Slot != nil {
					part += " slot " + itoa(*sk.Slot) + ":" + sk.Reason
				} else {
					part += " hole(" + itoa(sk.Position.Col) + "," + itoa(sk.Position.Row) + ")"
				}
			}
			part += ")"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " ")
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

// onPeerStatus applies the STRUCTURE (§6.5). The sidecar routes on the grant,
// not on this frame; the structure is what tells it whether a journaled
// destSlot has a live peer right now, and what lets it name the reason a closed
// axis is closed.
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
	s.mapShape = status.Map
	s.slotCount = status.SlotCount
	s.status = status
	if status.You.Slot != nil {
		s.slot = *status.You.Slot
	}
	if status.You.Position != nil {
		s.position = *status.You.Position
	}
	s.publishEdgesLocked(false)
	s.mu.Unlock()
	s.log.Debug("contract B: map status", "epoch", status.Epoch, "map", status.Map,
		"slotCount", status.SlotCount, "observers", status.Observers)
	return true
}

// migrationHead is the minimal projection of a MIGRATION_PAYLOAD the dedup fast
// path needs: the idempotency key, plus sourcePeer (for a tombstone re-ACK) and
// the informational reroute flag (for the duplicate log line). Decoding into it
// SKIPS body.bb8 — up to 4 MiB — so a duplicate never pays that decode.
type migrationHead struct {
	MigrationID string             `json:"migrationId"`
	SourcePeer  string             `json:"sourcePeer"`
	Reroute     *contractb.Reroute `json:"reroute,omitempty"`
}

// onMigrationPayload implements contract-b-m4.md §6.6's seven receiver
// obligations, in order.
func (s *Sidecar) onMigrationPayload(env wire.Envelope) bool {
	// 1. Dedup on migrationId against the journal AND its tombstones, BEFORE the
	// full body decode. A duplicate — the retry hammer of §9.2/B5 — then costs an
	// O(1) journal lookup and a tiny header parse instead of a multi-MiB body.bb8
	// unmarshal followed by validation and a genome hash: the shape that paid
	// 203 735 full decodes into the stalled slot 6. A RE-ROUTED frame
	// deduplicates on exactly the same key, which is what makes a re-route that
	// races a late delivery safe.
	var head migrationHead
	if err := contractb.DecodeData(env.Data, &head); err == nil && head.MigrationID != "" {
		s.mu.Lock()
		if st, ok := s.jr.Get(head.MigrationID); ok {
			s.mu.Unlock()
			// §6.7: a sidecar MUST NOT send MIGRATION_ACK when it has MERELY
			// JOURNALED the payload. The one exception is a migrationId that is
			// already TOMBSTONED — it was ACKed once before, so re-ACKing it
			// carries the same meaning.
			//
			// An entry still awaiting its mod is NOT that case, and pacing makes it
			// a long-lived one: a paced delivery can sit in this journal for
			// minutes of simulated time, and ACKing it here would release the
			// sender's custody before the spawn that is supposed to prove the
			// delivery. The sender's retry is free — it lands right back in this
			// branch — and its hold clock does not run against a live peer (Risk 9).
			acked := st.Status == journal.StatusDone || st.AckedUpstream
			s.log.Info("contract B: duplicate MIGRATION_PAYLOAD, delivering nothing",
				"migrationId", head.MigrationID, "status", st.Status,
				"reAcked", acked, "reroute", head.Reroute != nil)
			if acked {
				s.ackUpstreamNow(head.MigrationID, head.SourcePeer, st.Entry.EntityID, true)
			}
			return true
		}
		s.mu.Unlock()
	}

	// A NEW migrationId (or a frame too malformed to yield one): only now pay for
	// the full body decode and the identical validation the receiver has always
	// run before taking custody. A duplicate never reaches any of this.
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
			"kind "+payload.Kind+" is not supported in M4")
		return true
	}

	s.mu.Lock()
	// 2. Admission control. Under M4 this is also what a PACED BACKLOG produces
	// when it stops draining (contract-a.md §7.5): a peer-local refusal, and
	// therefore proof no custody moved, so the sender re-routes instead of
	// piling more onto a saturated peer. The OVERLOADED check stays AFTER dedup,
	// so it only ever fires for a NEW migrationId (§6.6 step 2).
	if s.jr.CountPending(journal.In) >= s.cfg.InboundQueueMax {
		hasMod := s.mod != nil && s.mod.handshaked
		s.mu.Unlock()
		code := contractb.NackOverloaded
		message := "inboundQueueMax reached; the paced backlog is not draining"
		if !hasMod {
			code = contractb.NackModAbsent
			message = "no mod is connected and inboundQueueMax is reached"
		}
		s.nackUpstream(payload.MigrationID, payload.SourcePeer, code, message)
		return true
	}
	// 3. Check S against our own, by the relative test of contract-a.md §13 A10.
	if s.mod != nil && s.mod.handshaked {
		if si, ok := findSlot(s.status, payload.SourceSlot); ok && si.SimulationSize > 0 &&
			!sameSize(si.SimulationSize, s.mod.simSize) {
			s.mu.Unlock()
			s.nackUpstream(payload.MigrationID, payload.SourcePeer, contractb.NackSimSizeMismatch,
				"the two sims disagree about simulationSize")
			return true
		}
	}
	s.mu.Unlock()

	// The entry edge is DERIVED, never carried: sending it would let a sender
	// dictate a receiver's geometry (§11 item 7). Under the grid it is the
	// opposite of the sender's exitEdge — W off an east lane, S off a north one.
	entryEdge := contracta.EdgeW
	if opp, ok := contracta.Opposite(payload.ExitEdge); ok {
		entryEdge = opp
	}

	// 4. bb8-schema validation. Nothing invalid ever reaches a mod.
	if err := bb8.Validate(payload.Body.Version, payload.Body.BB8); err != nil {
		s.log.Error("contract B: payload rejected", "migrationId", payload.MigrationID, "err", err)
		s.nackUpstream(payload.MigrationID, payload.SourcePeer, contractb.NackInvalidPayload, err.Error())
		return true
	}

	// contract-a.md §5.7: entityId comes out of the blob when bb8-schema can
	// read one; the wire value is the fallback (contract-b-m4.md §11 item 3).
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

	// Schema-validate the species block and nothing more (§6.6). This is the
	// second and last Go hop that takes the block off a wire, and it applies the
	// identical rule the source sidecar applied: a malformed block is stripped
	// and logged, and the organism still crosses. A conformant sender already
	// stripped a bad one, so a strip here names a non-conformant peer — which is
	// exactly why the log line exists.
	species, stripped := wire.CarrySpecies(payload.Species)
	if stripped != "" {
		s.log.Warn("contract B: stripping a malformed species block from an inbound envelope; "+
			"the migration proceeds without it",
			"migrationId", payload.MigrationID, "sourcePeer", payload.SourcePeer, "reason", stripped)
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
		// contract-a.md §4.3: entryPosition is exitPosition, copied. The
		// transverse continuity D3 buys is unchanged on both axes.
		Position:    payload.ExitPosition,
		VelocityX:   payload.Velocity.X,
		VelocityY:   payload.Velocity.Y,
		Heading:     heading,
		SourcePeer:  payload.SourcePeer,
		SourceSlot:  payload.SourceSlot,
		DestSlot:    payload.DestSlot,
		GenomeHash:  payload.Lineage.GenomeHash,
		Parents:     parentRefs(payload.Lineage.Parents),
		Species:     species,
		JournaledAt: s.now().UnixMilli(),
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
		"exitEdge", payload.ExitEdge, "genomeHash", payload.Lineage.GenomeHash,
		"species", wire.SpeciesName(species))
	// 7. Deliver, THROUGH THE DELIVERY RATE LIMIT, and keep replaying until the
	// mod ACKs. Pacing sits after step 5, never before it.
	if s.mod != nil && s.mod.handshaked {
		s.deliverLocked(st, s.now())
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

func findSlot(status contractb.PeerStatus, slot int) (contractb.SlotInfo, bool) {
	for _, si := range status.Slots {
		if si.Slot == slot {
			return si, true
		}
	}
	return contractb.SlotInfo{}, false
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
	if !ok {
		// §9.2: an implementation that swallows an ACK for an unknown
		// migrationId silently throws away the only automatic signal the map
		// will ever give.
		s.log.Warn("contract B: MIGRATION_ACK for a migration this sidecar does not know",
			"migrationId", ack.MigrationID, "from", ack.SourcePeer)
		return true
	}
	if st.BouncedTimeout {
		// THE ACCEPTED DUPLICATION CASE OF §9.3, ANNOUNCING ITSELF. The far peer
		// took custody, died before its acknowledgement, this sidecar's hold
		// expired and bounced the organism home, and the far peer has now
		// returned and delivered it. Two organisms exist.
		s.duplicateSuspected++
		s.log.Error("contract B: MIGRATION_ACK arrived for an entry a hold timeout already bounced — "+
			"THE MAP NOW HOLDS TWO COPIES OF THIS ORGANISM",
			"migrationId", ack.MigrationID, "entityId", ack.EntityID,
			"deliveredBy", ack.SourcePeer, "duplicateSuspected", s.duplicateSuspected)
		return true
	}
	if st.Direction != journal.Out {
		return true
	}
	if st.Status == journal.StatusDone {
		return true
	}
	completed := s.now().UnixMilli()
	acked := true
	if _, err := s.jr.Apply(ack.MigrationID, journal.Update{
		Status: journal.StatusDone, CompletedAt: &completed, Acked: &acked,
		Handoff: journal.HandoffDone, Duplicate: &ack.Duplicate}); err != nil {
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

// onMigrationNack applies §9.2's evidence table. It is the most load-bearing
// switch in the sidecar: SILENCE IS NEVER PROOF, and every ambiguity resolves
// toward holding, because holding costs a delay and re-routing on a bad proof
// costs a duplicated organism.
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
	relayGenerated := nack.SourcePeer == ""
	s.log.Warn("contract B: MIGRATION_NACK", "migrationId", nack.MigrationID,
		"code", nack.Code, "class", nack.Class, "relayGenerated", relayGenerated,
		"neverForwarded", nack.NeverForwarded, "relaySessionId", nack.RelaySessionID,
		"entrySession", st.RelaySessionID, "handoff", st.Handoff, "message", nack.Message)

	now := s.now()
	sc := s.schedFor(nack.MigrationID)
	retry := s.cfg.ForwardRetry
	if nack.RetryAfterMs > 0 {
		retry = time.Duration(nack.RetryAfterMs) * time.Millisecond
	}
	sc.nextForward = now.Add(retry)

	switch {
	case !relayGenerated && contractb.PayloadRefusal(nack.Code):
		// Refused for a payload reason: every slot refuses this organism, so the
		// map is not the answer. A sidecar NACK is only ever sent BEFORE durable
		// custody (§6.8), so the bounce cannot duplicate.
		s.bounceLocked(st, "MIGRATION_NACK "+nack.Code+" — every slot would refuse this payload", false)
		return true

	case !relayGenerated && contractb.PeerLocalRefusal(nack.Code):
		// The receiver STATED it took no custody. Re-route: another slot accepts
		// the same organism.
		s.markRefusedLocked(st, contractb.ProofPeerRefused,
			"peer-local refusal "+nack.Code+" proves no custody moved")
		if s.rerouteLocked(st, contractb.ProofPeerRefused, now) {
			sc.bounceAt = time.Time{}
			sc.nextForward = time.Time{}
		}
		return true

	case relayGenerated && nack.ProvesNoCustody(st.RelaySessionID):
		// The relay has forwarded no frame with this migrationId to anyone
		// during a session that covers the entry's WHOLE LIFE (§5.2).
		s.markRefusedLocked(st, contractb.ProofRelayNeverForwarded,
			"relay proof: neverForwarded under the recorded relaySessionId")
		if s.rerouteLocked(st, contractb.ProofRelayNeverForwarded, now) {
			sc.bounceAt = time.Time{}
			sc.nextForward = time.Time{}
		}
		return true

	case relayGenerated && st.Handoff == journal.HandoffPending:
		// The sender's own record is sufficient: the frame was never handed to
		// anybody. tickOutbound re-routes it on the next pass.
		return true

	case relayGenerated && nack.NeverForwarded == nil:
		// A conforming relay always sets it on a NACK it generated (§6.8), so a
		// missing field is a defect or a frame that came from somewhere else.
		// Treat a missing proof as NO PROOF, and log it.
		s.log.Error("contract B: a relay-generated MIGRATION_NACK carried no neverForwarded field — "+
			"treating it as no proof and holding", "migrationId", nack.MigrationID, "code", nack.Code)
		return true

	case relayGenerated && nack.NeverForwarded != nil && *nack.NeverForwarded &&
		nack.RelaySessionID != st.RelaySessionID:
		// The relay restarted; it cannot speak for what the previous process
		// forwarded, so the sender falls back to holding.
		s.log.Warn("contract B: neverForwarded arrived under a DIFFERENT relaySessionId — "+
			"the relay restarted and cannot speak for this entry; holding",
			"migrationId", nack.MigrationID, "nackSession", nack.RelaySessionID,
			"entrySession", st.RelaySessionID)
		return true
	}
	// Everything else: no proof. The entry keeps its recorded destination and
	// tickOutbound decides between a re-forward and the bounded hold.
	return true
}

func (s *Sidecar) markRefusedLocked(st *journal.State, proof, note string) {
	if _, err := s.jr.Apply(st.Entry.MigrationID, journal.Update{
		Handoff: journal.HandoffRefused, Note: note}); err != nil {
		s.log.Error("contract B: journal update failed", "migrationId", st.Entry.MigrationID, "err", err)
		return
	}
	st.Handoff = journal.HandoffRefused
}

// onGenomeRequest answers exactly one GENOME_RESPONSE from the genome cache
// (contract-b-m4.md §6.9). It never blocks a migration to serve one, and a
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
	now := s.now()
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
		DeliveredAt: s.now().UnixMilli(),
	})
}

// refreshClaim re-sends SECTOR_CLAIM so the relay — and through it every peer
// whose lane points here — learns the current simulationSize, export edges,
// border edges, mod presence and stats (contract-b-m4.md §6.3).
func (s *Sidecar) refreshClaim() {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := s.statsLocked()
	claim := contractb.SectorClaim{
		PreferredSlot:     s.cfg.PreferredSlot,
		PreferredPosition: s.cfg.PreferredPosition,
		InsertAfterSlot:   s.cfg.InsertAfterSlot,
		InsertAxis:        s.cfg.InsertAxis,
		Stats:             &stats,
	}
	if s.mod != nil && s.mod.handshaked {
		claim.SimulationSize = s.mod.simSize
		claim.ExportEdges = append([]string(nil), s.mod.exportEdges...)
		claim.BorderEdges = append([]string(nil), s.mod.borderEdges...)
		claim.GameVersion = s.mod.gameVersion
		claim.ModConnected = true
	} else {
		// §6.3: empty while no mod is connected. A sidecar with no mod cannot
		// spawn an organism, so it is not DELIVERABLE and every lane pointed at
		// it re-pairs around it.
		claim.ExportEdges = []string{}
		claim.BorderEdges = []string{}
	}
	s.sendRelayLocked(contractb.TypeSectorClaim, claim)
}
