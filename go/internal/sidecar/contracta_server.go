package sidecar

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/bb8"
	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

// modSession is one live mod connection and everything its handshake told us.
type modSession struct {
	gen  uint64
	conn *wsutil.Conn

	handshaked  bool
	sessionID   string
	gameVersion string
	modVersion  string
	simSize     float64
	borderWidth float64
	// borderEdges are the edges the mod will accept an inbound organism on — all
	// four since A18. exportEdges are the edges it runs a capture band on, one
	// entry each in EDGE_STATUS. Under D17 the two sets are EQUAL rather than one
	// being a superset of the other: every declared edge is both an export edge
	// and an entry edge, and "out and in are different doors" is retired
	// (contract-a.md §15 A18, §18 A38).
	borderEdges []string
	exportEdges []string
	ringSlot    *int
	worldName   string

	lastHeartbeat time.Time
	paused        bool
	// timeScale is seeded at 1 so a session with no heartbeat yet reads as
	// RUNNING rather than paused, which is what the pacer wants. haveTimeScale
	// is what the stats block wants, and it is a different question: until a
	// HEARTBEAT arrives the world's speed is UNKNOWN, and 1 would be a confident
	// guess about a world that may well be at 5 or at 0.
	timeScale     float64
	haveTimeScale bool

	// The HEARTBEAT-derived half of the peer stats block (contract-b-m4.md
	// §6.3.1). "have" flags keep absence a value: a stat the mod has not
	// reported is OMITTED, never defaulted to zero.
	havePopulation bool
	population     int
	haveEggCount   bool
	eggCount       int
	haveSimTime    bool
	simulatedTime  float64
	// lastSave is the mod's save receipt, copied verbatim and never computed
	// (§15, A21). A mod that omits it is conformant and the status page shows
	// that world's save state as unknown.
	lastSave *contracta.SaveReceipt

	// census is the species census the LAST HEARTBEAT carried, after §5.2's
	// strip table, and censusTruncated is that heartbeat's flag after the
	// monotonic OR (§17, A35; contract-b-m4.md §16 B11).
	//
	// IT TRACKS THE LAST HEARTBEAT AND IT IS NOT STICKY. A census absent from
	// this beat leaves this field nil, and the stats block then reports the
	// census as unknown rather than repeating an older one. The census is built
	// in the same Update as population, eggCount and simulatedTime, so one
	// heartbeat describes one instant — and one stats block carries one
	// statsAsOfMs. Holding a census over from an earlier beat would put a
	// species list of one age beside a population of another under a single
	// timestamp, which is precisely the disagreement §16 B11 keeps this on one
	// block to avoid.
	//
	// nil is ABSENT/UNKNOWN. A non-nil census with no entries is the stronger,
	// different fact: a reporting mod whose world has nothing alive in it.
	census          *wire.Census
	censusTruncated bool
	// censusWarn is the last set of strip reasons this session logged, so a mod
	// with a permanently broken census says so ONCE rather than once a second
	// for a week. §5.2 asks for one line per rule that fires; a rule that fires
	// on every heartbeat of a 24/7 rig is one fact, not 86 400 of them, and a
	// log an operator scrolls past is the same as no log at all. A CHANGED
	// defect logs again, and so does the same defect on a new session.
	censusWarn string
}

// serveRetiredContractA implements contract-a.md §15 A23: an M3 mod dials
// /contract-a/v1, and the M4 sidecar MUST close that connection immediately
// with 4000 PROTOCOL_UNSUPPORTED. A bare HTTP 404 would be a socket error in a
// BepInEx log and half an evening of diagnosis; 4000 is a defined close the mod
// already handles by logging one loud error and not reconnecting.
func (s *Sidecar) serveRetiredContractA(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode:    websocket.CompressionDisabled,
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	s.log.Error("contract A: refusing a connection on the retired /contract-a/v1 path",
		"hint", "this sidecar speaks "+wire.ProtocolA+" on "+contracta.ContractAPath+
			"; update the mod")
	_ = ws.Close(contracta.CloseProtocolUnsupported,
		"contract-a/v1 is retired; this sidecar serves "+contracta.ContractAPath)
}

func (s *Sidecar) serveContractA(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// contract-a.md §2: permessage-deflate MUST NOT be negotiated.
		CompressionMode:    websocket.CompressionDisabled,
		InsecureSkipVerify: true, // loopback only; auth is the M5 item of §12 (§15, A24)
	})
	if err != nil {
		s.log.Warn("contract A: upgrade failed", "err", err)
		return
	}
	conn := wsutil.New(ws, 256)

	s.mu.Lock()
	s.modGen++
	sess := &modSession{gen: s.modGen, conn: conn, lastHeartbeat: s.now(), timeScale: 1}
	old := s.mod
	s.mod = sess
	// The pacing bucket is deliberately NOT reset here. A socket reconnect under
	// the SAME sessionId is the same world clock: simulated time kept advancing
	// while the socket was down, the burst was never refunded, and refilling it
	// now would let the un-ACKed replay of §7.5 arrive as the very flood pacing
	// exists to prevent — the slot-6 livelock. The reset belongs to a genuinely
	// new sessionId, and onConfigUpdate does it there once the handshake is read.
	s.edgeEpoch = 0 // contract-a.md §5.4: the epoch counter resets per connection
	s.lastEdges = nil
	s.mu.Unlock()

	if old != nil {
		// contract-a.md §2: a newer connection takes over, the older one gets
		// 4006. That makes a crashed-and-restarted game self-healing.
		old.conn.Close(contracta.CloseReplaced, "a newer mod connection took over")
	}
	s.log.Info("contract A: mod connected", "gen", sess.gen)

	go s.modHeartbeatMonitor(sess)
	go s.modPinger(sess)

	s.readModLoop(r.Context(), sess)

	s.mu.Lock()
	if s.mod == sess {
		s.mod = nil
		s.lastEdges = nil
	}
	s.mu.Unlock()
	// contract-a.md §8 step 2: with no mod, every local edge is closed and the
	// neighbour must learn about it.
	s.refreshClaim()
	s.log.Info("contract A: mod disconnected", "gen", sess.gen)
	<-conn.Done()
}

func (s *Sidecar) readModLoop(ctx context.Context, sess *modSession) {
	for {
		select {
		case <-s.ctx.Done():
			sess.conn.Close(contracta.CloseShuttingDown, "sidecar draining")
			return
		case <-sess.conn.Done():
			return
		default:
		}
		readCtx, cancel := context.WithCancel(ctx)
		go func() {
			select {
			case <-sess.conn.Done():
				cancel()
			case <-s.ctx.Done():
				cancel()
			case <-readCtx.Done():
			}
		}()
		frame, err := sess.conn.Read(readCtx)
		cancel()
		if err != nil {
			// One line, once per session end. A swallowed read error hid the
			// evidence during the slot-6 stall diagnosis: whether the mod sent a
			// close (and with what reason), the peer reset, or the monitor's own
			// 4004 cancelled the read is exactly what the next diagnosis needs.
			s.log.Info("contract A: read ended", "gen", sess.gen, "err", err)
			return
		}
		if !s.handleModFrame(sess, frame) {
			return
		}
	}
}

// handleModFrame processes one Contract A frame. It returns false when the
// session must end.
func (s *Sidecar) handleModFrame(sess *modSession, frame []byte) bool {
	env, err := wire.Decode(frame)
	if err != nil {
		// contract-a.md §3.2: never guess a field.
		sess.conn.Close(contracta.CloseMalformedFrame, "malformed frame")
		return false
	}
	if err := wire.CheckProtocol(env.Protocol, wire.ProtocolA); err != nil {
		sess.conn.Close(contracta.CloseProtocolUnsupported, "unsupported protocol major version")
		return false
	}

	s.mu.Lock()
	handshaked := sess.handshaked
	s.mu.Unlock()

	if !handshaked && env.Type != contracta.TypeConfigUpdate {
		// contract-a.md §5.1: CONFIG_UPDATE is the handshake and must come first.
		sess.conn.Close(contracta.CloseMalformedFrame, "first frame is not CONFIG_UPDATE")
		return false
	}

	switch env.Type {
	case contracta.TypeConfigUpdate:
		return s.onConfigUpdate(sess, env)
	case contracta.TypeHeartbeat:
		return s.onHeartbeat(sess, env)
	case contracta.TypeMigrateOut:
		return s.onMigrateOut(sess, env)
	case contracta.TypeMigrateInAck:
		return s.onMigrateInAck(sess, env)
	case contracta.TypeMigrateInNack:
		return s.onMigrateInNack(sess, env)
	default:
		// contract-a.md §3.1: an unknown type is ignored with one warning and
		// never closes the connection.
		s.log.Warn("contract A: ignoring unknown type", "type", env.Type)
		return true
	}
}

func (s *Sidecar) onConfigUpdate(sess *modSession, env wire.Envelope) bool {
	var cfg contracta.ConfigUpdate
	if err := contracta.DecodeData(env.Data, &cfg); err != nil {
		sess.conn.Close(contracta.CloseMalformedFrame, "malformed CONFIG_UPDATE")
		return false
	}
	if err := cfg.Validate(); err != nil {
		// CONFIG_UPDATE has no NACK channel. contract-a.md §5.2 sets the
		// precedent for this class of fault by closing 4003 on an unusable
		// handshake-bound frame, so the same answer is used here.
		sess.conn.Close(contracta.CloseMalformedFrame, err.Error())
		return false
	}
	if !bb8.GameVersionSupported(cfg.GameVersion) {
		sess.conn.Close(contracta.CloseGameVersionUnsupport, "bb8-schema has no dialect for "+cfg.GameVersion)
		return false
	}
	// §15 A18: exportEdges is REQUIRED, has at least one member, no duplicates,
	// and every member is also in borderEdges. Validate already applied the same
	// rule, so this cannot fail here — it is read for the value.
	exportEdges, err := cfg.ResolveExportEdges()
	if err != nil {
		sess.conn.Close(contracta.CloseMalformedFrame, err.Error())
		return false
	}

	s.mu.Lock()
	first := !sess.handshaked
	if !first && !wire.SameUUID(cfg.SessionID, sess.sessionID) {
		s.log.Info("contract A: mod reported a new sessionId", "old", sess.sessionID, "new", cfg.SessionID)
	}
	sess.handshaked = true
	sess.sessionID = cfg.SessionID
	sess.gameVersion = cfg.GameVersion
	sess.modVersion = cfg.ModVersion
	sess.simSize = *cfg.SimulationSize
	sess.borderWidth = *cfg.BorderWidth
	sess.borderEdges = append([]string(nil), cfg.BorderEdges...)
	sess.exportEdges = exportEdges
	sess.ringSlot = cfg.RingSlot
	sess.worldName = cfg.WorldName
	sess.lastHeartbeat = s.now()

	if mismatch := s.slotMismatchLocked(sess); mismatch != "" {
		s.mu.Unlock()
		sess.conn.Close(contracta.CloseSlotMismatch, mismatch)
		return false
	}

	newSession := !s.seenSessions[lowerUUID(cfg.SessionID)]
	s.seenSessions[lowerUUID(cfg.SessionID)] = true

	// contract-a.md §6.1: exactly one EDGE_STATUS follows the handshake.
	s.publishEdgesLocked(true)
	if newSession {
		// A new sessionId is a new world clock (§7.5): the pacing bucket starts
		// cold rather than carrying a stale simulated time across the gap, and its
		// internal lastSim/haveSim are cleared so the first HEARTBEAT re-anchors
		// them. A same-session reconnect keeps all of it — see serveContractA.
		s.pace.reset()
		s.reassertCustodyLocked()
	}
	s.replayInboundLocked()
	s.mu.Unlock()

	s.log.Info("contract A: handshake", "sessionId", cfg.SessionID, "reason", cfg.Reason,
		"gameVersion", cfg.GameVersion, "modVersion", cfg.ModVersion,
		"simulationSize", sess.simSize, "borderEdges", sess.borderEdges,
		"exportEdges", sess.exportEdges, "ringSlot", derefIntPtr(cfg.RingSlot), "world", cfg.WorldName)
	s.refreshClaim()
	return true
}

// slotMismatchLocked implements the advisory CONFIG_UPDATE.ringSlot check of
// contract-a.md §5.1 and §14 A14. It can only fire once a slot has been
// granted, and it catches a mis-wired three-instance rig in one second instead
// of one hour.
func (s *Sidecar) slotMismatchLocked(sess *modSession) string {
	if sess.ringSlot == nil || s.slot == 0 {
		return ""
	}
	if *sess.ringSlot != s.slot {
		return "mod claims ring slot " + strconv.Itoa(*sess.ringSlot) +
			" but this sidecar holds " + strconv.Itoa(s.slot)
	}
	return ""
}

func derefIntPtr(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func lowerUUID(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func (s *Sidecar) onHeartbeat(sess *modSession, env wire.Envelope) bool {
	var hb contracta.Heartbeat
	if err := contracta.DecodeData(env.Data, &hb); err != nil {
		sess.conn.Close(contracta.CloseMalformedFrame, "malformed HEARTBEAT")
		return false
	}
	if err := hb.Validate(); err != nil {
		sess.conn.Close(contracta.CloseMalformedFrame, err.Error())
		return false
	}
	s.mu.Lock()
	if !wire.SameUUID(hb.SessionID, sess.sessionID) {
		// contract-a.md §5.2: a mismatch means the mod skipped a handshake.
		s.mu.Unlock()
		sess.conn.Close(contracta.CloseMalformedFrame, "HEARTBEAT sessionId does not match the handshake")
		return false
	}
	now := s.now()
	sess.lastHeartbeat = now
	sess.paused = *hb.Paused
	sess.timeScale = *hb.TimeScale
	sess.haveTimeScale = true
	resized := !sameSize(sess.simSize, *hb.SimulationSize)
	sess.simSize = *hb.SimulationSize
	// The stats block of contract-b-m4.md §6.3.1 comes from here, and so does
	// the pacing clock: simulated, never wall (§7.5).
	sess.havePopulation, sess.population = true, *hb.Population
	if hb.EggCount != nil {
		sess.haveEggCount, sess.eggCount = true, *hb.EggCount
	}
	sess.haveSimTime, sess.simulatedTime = true, *hb.SimulatedTime
	if hb.LastSave != nil {
		save := *hb.LastSave
		sess.lastSave = &save
	}
	// §5.2's strip table, the ONE place it is applied. It never returns an
	// error and it never closes: a bad entry costs that entry, a `species` that
	// is not an array costs the field, and neither costs the heartbeat, the
	// connection, or the liveness this heartbeat carries (§8). The log lines it
	// produces are emitted after the lock, with the resize warning below.
	census, truncated, stripped := wire.CarryCensus(hb.Species, bool(hb.Truncated))
	sess.census, sess.censusTruncated = census, truncated
	if fresh := strings.Join(stripped, "; "); fresh != sess.censusWarn {
		sess.censusWarn = fresh
	} else {
		stripped = nil
	}
	if sess == s.mod {
		s.pace.beat(*hb.SimulatedTime, !*hb.Paused && *hb.TimeScale > 0, now)
	}
	if resized {
		// contract-a.md §5.2: a changed S re-checks peer agreement exactly as a
		// CONFIG_UPDATE would. SimulationSize is live-mutable.
		s.publishEdgesLocked(false)
	}
	s.mu.Unlock()
	for _, why := range stripped {
		// One line per rule that fired, and then nothing else happens: the
		// heartbeat was processed normally, and the world whose census had to be
		// stripped reports everything else exactly as before.
		s.log.Warn("contract A: stripped part of a HEARTBEAT species census", "reason", why)
	}
	if resized {
		s.log.Warn("contract A: mod reported a new simulationSize", "simulationSize", *hb.SimulationSize)
		s.refreshClaim()
	}
	return true
}

// onMigrateOut implements contract-a.md §5.3's six receiver obligations in
// their stated order.
func (s *Sidecar) onMigrateOut(sess *modSession, env wire.Envelope) bool {
	var out contracta.MigrateOut
	if err := contracta.DecodeData(env.Data, &out); err != nil {
		s.nackOut(sess, out.MigrationID, 0, contracta.OutMalformedMessage, err.Error(), 0)
		return true
	}
	if err := out.Validate(); err != nil {
		var entityID int32
		if out.EntityID != nil {
			entityID = *out.EntityID
		}
		s.nackOut(sess, out.MigrationID, entityID, contracta.OutMalformedMessage, err.Error(), 0)
		return true
	}
	entityID := *out.EntityID

	// 1. Field validation, then bb8-schema validation.
	if out.Kind != contracta.KindBibite {
		s.nackOut(sess, out.MigrationID, entityID, contracta.OutKindUnsupported,
			"kind "+out.Kind+" is not supported in M2", 0)
		return true
	}
	if !contracta.ValidEdge(out.ExitEdge) {
		// §15 A18: exitEdge is always a member of the declared exportEdges. Under
		// D17 (§18, A38) all four values occur, so this is an ENUM check and
		// nothing more — whether THIS mod may leave by THIS edge is
		// exportOpenLocked's question, decided against the set the mod declared.
		s.nackOut(sess, out.MigrationID, entityID, contracta.OutEdgeClosed,
			"exitEdge "+out.ExitEdge+" is not one of E, N, W, S", 0)
		return true
	}
	if err := bb8.Validate(out.GameVersion, out.Payload); err != nil {
		s.log.Error("contract A: payload rejected", "migrationId", out.MigrationID,
			"bytes", len(out.Payload), "head", head(out.Payload), "err", err)
		s.nackOut(sess, out.MigrationID, entityID, contracta.OutInvalidPayload, err.Error(), 0)
		return true
	}
	hash := bb8.Hash(out.Payload)

	// Still step 1, and it is the FIRST GO HOP THE BLOCK REACHES: the species
	// block is schema-validated and nothing more (§16, A30). A block that breaks
	// the shape is stripped, logged ONCE, and the migration proceeds without it —
	// never a NACK, never a reason to hold an organism. Nothing here synthesizes,
	// translates, normalizes or case-folds a name, and nothing derives one from
	// the payload: the blob holds a world-local integer and no name at all.
	species, stripped := wire.CarrySpecies(out.Species)
	if stripped != "" {
		s.log.Warn("contract A: stripping a malformed species block; the migration proceeds without it",
			"migrationId", out.MigrationID, "entityId", entityID, "reason", stripped)
	}

	s.mu.Lock()
	// 2 and 3. migrationId dedup against the journal.
	//nolint:nestif // the two dedup outcomes of §5.3 steps 2 and 3
	if st, ok := s.jr.Get(out.MigrationID); ok {
		same := st.Entry.PayloadHash == hash
		journaledAt := st.Entry.JournaledAt
		s.mu.Unlock()
		if same {
			// An idempotent repeat after a lost ACK.
			s.sendMod(contracta.TypeMigrateOutAck, contracta.MigrateOutAck{
				MigrationID: out.MigrationID, EntityID: entityID, JournaledAt: journaledAt})
			s.log.Info("contract A: idempotent MIGRATE_OUT re-ACKed", "migrationId", out.MigrationID)
		} else {
			s.nackOut(sess, out.MigrationID, entityID, contracta.OutDuplicateMigationID,
				"migrationId is journaled against a different payload", 0)
			s.log.Error("contract A: DUPLICATE_MIGRATION_ID — this is a mod defect",
				"migrationId", out.MigrationID, "entityId", entityID)
		}
		return true
	}

	// 4. Edge and peer checks.
	destSlot, code, message := s.exportOpenLocked(out.ExitEdge, *out.SimulationSize)
	if code != "" {
		s.mu.Unlock()
		s.nackOut(sess, out.MigrationID, entityID, code, message, 15000)
		return true
	}
	if s.jr.CountPending(journal.Out) >= s.cfg.InboundQueueMax {
		s.mu.Unlock()
		s.nackOut(sess, out.MigrationID, entityID, contracta.OutJournalFull,
			"too many outbound migrations already in custody", 15000)
		return true
	}
	sourceSlot := s.slot
	s.mu.Unlock()

	// 5. Build the lineage annex (§14, A12). This step MUST NOT fail the
	// migration: a gap is a normal outcome, and D11 already treats a missing
	// parent as normal.
	genomeHash, parents := s.buildAnnex(out)

	// 6. Durable journal write. Only after this may the sidecar ACK.
	entry := journal.Entry{
		MigrationID:    out.MigrationID,
		EntityID:       entityID,
		Kind:           out.Kind,
		GameVersion:    out.GameVersion,
		Payload:        out.Payload,
		PayloadHash:    hash,
		Edge:           out.ExitEdge,
		Position:       *out.ExitPosition,
		VelocityX:      out.Velocity.X,
		VelocityY:      out.Velocity.Y,
		Heading:        *out.Heading,
		SimulationSize: *out.SimulationSize,
		SimTick:        *out.SimTick,
		SourceSlot:     sourceSlot,
		DestSlot:       destSlot,
		GenomeHash:     genomeHash,
		Parents:        parents,
		Species:        species,
		JournaledAt:    time.Now().UnixMilli(),
	}
	s.mu.Lock()
	st, err := s.jr.Create(journal.Out, entry, false)
	s.mu.Unlock()
	if err != nil {
		s.log.Error("contract A: journal write failed", "migrationId", out.MigrationID, "err", err)
		s.nackOut(sess, out.MigrationID, entityID, contracta.OutJournalError, err.Error(), 5000)
		return true
	}
	s.log.Info("contract A: took custody", "migrationId", out.MigrationID, "entityId", entityID,
		"exitEdge", out.ExitEdge, "destSlot", destSlot, "genomeHash", genomeHash,
		"parents", len(parents), "species", wire.SpeciesName(species))

	s.faultPoint(FaultPostJournal)

	// 7. Custody has moved. Tell the mod to destroy the organism.
	s.sendMod(contracta.TypeMigrateOutAck, contracta.MigrateOutAck{
		MigrationID: out.MigrationID, EntityID: entityID, JournaledAt: st.Entry.JournaledAt})

	// 8. Forward, with the annex attached and every parent blob stripped.
	// Never before the flush.
	//
	// This goes through the custody scheduler rather than forwarding directly:
	// the journal write above released the lock, so the tick may already have
	// picked the entry up, and forwarding again here would put two identical
	// frames on the wire. The receiver deduplicates either way, but a
	// gratuitous duplicate turns every ACK into duplicate: true and makes the
	// archive's record read as a re-delivery.
	s.mu.Lock()
	s.tickOutbound(st, time.Now())
	s.mu.Unlock()
	return true
}

// buildAnnex implements contract-a.md §14 A12 and contract-b-m4.md §6.6: the
// mod ships opaque blobs, the sidecar hashes them.
//
// It hashes the migrant's genome and every present parent blob with the
// canonical projection, caches each blob under its hash so a later
// GENOME_REQUEST can be answered, and records a gap for every parent entry
// that carries no blob or whose blob will not hash. Nothing in here may fail a
// migration — the annex is bookkeeping; the organism is custody.
//
// The blobs are never returned: the caller journals the annex and forwards it,
// and the parent blobs go no further than the cache.
func (s *Sidecar) buildAnnex(out contracta.MigrateOut) (string, []journal.ParentRef) {
	genomeHash, err := bb8.GenomeHash(out.Payload, out.GameVersion)
	if err != nil {
		// genome-hash.md §8: a payload that passed validation and then fails to
		// hash is a bb8-schema defect and MUST be logged loudly. It is not a
		// reason to refuse the organism.
		s.log.Error("contract A: the migrant's own genome will not hash — this is a bb8-schema defect",
			"migrationId", out.MigrationID, "entityId", derefInt32(out.EntityID), "err", err)
	} else {
		s.cacheGenome(genomeHash, out.GameVersion, out.Payload)
	}

	parents := out.Parents
	if len(parents) > contracta.MaxParentBlobs {
		// §10: a longer array is truncated with one warning, never a NACK.
		s.log.Warn("contract A: MIGRATE_OUT carried more than maxParentBlobs parents, truncating",
			"migrationId", out.MigrationID, "parents", len(parents))
		parents = parents[:contracta.MaxParentBlobs]
	}
	refs := make([]journal.ParentRef, 0, len(parents))
	for _, p := range parents {
		if p.EntityID == nil || *p.EntityID == 0 {
			// §5.3: 0 is the game's "unassigned" sentinel and such an entry
			// MUST be omitted by the sender. Drop it rather than record a
			// lineage node that names nobody.
			s.log.Warn("contract A: dropping a parent entry with no entityId",
				"migrationId", out.MigrationID)
			continue
		}
		ref := journal.ParentRef{EntityID: *p.EntityID}
		if p.Payload == "" {
			// The usual case: BibiteGenes drops the parentage once the parent
			// GameObject is gone. A blob the mod dropped for frame size arrives
			// the same way and is indistinguishable from here.
			ref.GapReason = contractb.GapParentGone
			refs = append(refs, ref)
			continue
		}
		version := p.GameVersion
		if version == "" {
			// §5.3: absent means "the same as the migrant's gameVersion",
			// which is always true in practice — both were serialized in the
			// same tick.
			version = out.GameVersion
		}
		hash, err := bb8.GenomeHash(p.Payload, version)
		if err != nil {
			ref.GapReason = contractb.GapBlobInvalid
			s.log.Warn("contract A: a parent blob will not hash, recording a gap",
				"migrationId", out.MigrationID, "parentEntityId", *p.EntityID, "err", err)
			refs = append(refs, ref)
			continue
		}
		ref.GenomeHash = hash
		s.cacheGenome(hash, version, p.Payload)
		refs = append(refs, ref)
	}
	return genomeHash, refs
}

func derefInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

func (s *Sidecar) onMigrateInAck(sess *modSession, env wire.Envelope) bool {
	var ack contracta.MigrateInAck
	if err := contracta.DecodeData(env.Data, &ack); err != nil {
		sess.conn.Close(contracta.CloseMalformedFrame, "malformed MIGRATE_IN_ACK")
		return false
	}
	if err := ack.Validate(); err != nil {
		sess.conn.Close(contracta.CloseMalformedFrame, err.Error())
		return false
	}

	s.mu.Lock()
	st, ok := s.jr.Get(ack.MigrationID)
	if !ok {
		s.mu.Unlock()
		s.log.Warn("contract A: MIGRATE_IN_ACK for an unknown migration", "migrationId", ack.MigrationID)
		return true
	}
	if st.Status == journal.StatusDone {
		s.mu.Unlock()
		return true
	}
	if st.Entry.EntityID != *ack.EntityID {
		s.log.Error("contract A: MIGRATE_IN_ACK entityId differs from the delivery — this is a defect",
			"migrationId", ack.MigrationID, "delivered", st.Entry.EntityID, "acked", *ack.EntityID)
	}
	dup := *ack.Duplicate
	completed := time.Now().UnixMilli()
	done, err := s.jr.Apply(ack.MigrationID, journal.Update{
		Status: journal.StatusDone, Duplicate: &dup, CompletedAt: &completed})
	if err != nil {
		s.mu.Unlock()
		s.log.Error("contract A: journal update failed", "migrationId", ack.MigrationID, "err", err)
		return true
	}
	delete(s.sched, ack.MigrationID)
	// contract-a.md §5.8: the sidecar clears its entry and sends MIGRATION_ACK
	// upstream, which lets the origin peer clear its own journal.
	if !done.BounceBack && done.Entry.SourcePeer != "" {
		s.ackUpstreamLocked(done)
	}
	s.mu.Unlock()
	s.log.Info("contract A: organism delivered", "migrationId", ack.MigrationID,
		"entityId", *ack.EntityID, "duplicate", dup,
		"relinkedParents", derefInt(ack.RelinkedParents), "relinkedChildren", derefInt(ack.RelinkedChildren))
	return true
}

func (s *Sidecar) onMigrateInNack(sess *modSession, env wire.Envelope) bool {
	var nack contracta.MigrateInNack
	if err := contracta.DecodeData(env.Data, &nack); err != nil {
		sess.conn.Close(contracta.CloseMalformedFrame, "malformed MIGRATE_IN_NACK")
		return false
	}
	if err := nack.Validate(); err != nil {
		sess.conn.Close(contracta.CloseMalformedFrame, err.Error())
		return false
	}

	s.mu.Lock()
	st, ok := s.jr.Get(nack.MigrationID)
	if !ok {
		s.mu.Unlock()
		s.log.Warn("contract A: MIGRATE_IN_NACK for an unknown migration", "migrationId", nack.MigrationID)
		return true
	}
	if nack.Class == contracta.ClassTransient {
		retry := s.cfg.MigrateInAckTimeout
		if nack.RetryAfterMs != nil && *nack.RetryAfterMs > 0 {
			retry = time.Duration(*nack.RetryAfterMs) * time.Millisecond
		}
		s.schedFor(nack.MigrationID).nextDeliver = time.Now().Add(retry)
		s.mu.Unlock()
		s.log.Warn("contract A: transient MIGRATE_IN_NACK, keeping custody",
			"migrationId", nack.MigrationID, "code", nack.Code, "retryAfter", retry)
		return true
	}
	// Permanent. contract-b-m4.md §9.4: the destination sidecar HOLDS the entry
	// for an operator and logs it loudly. It does not return the organism over
	// Contract B, because there is no two-phase return in M4's message list and
	// a lost return frame would lose the organism outright.
	if _, err := s.jr.Apply(nack.MigrationID, journal.Update{
		Status: journal.StatusHeld, Note: "MIGRATE_IN_NACK " + nack.Code}); err != nil {
		s.log.Error("contract A: journal update failed", "migrationId", nack.MigrationID, "err", err)
	}
	delete(s.sched, nack.MigrationID)
	s.mu.Unlock()
	s.log.Error("contract A: permanent MIGRATE_IN_NACK, organism held in the journal for an operator",
		"migrationId", nack.MigrationID, "entityId", st.Entry.EntityID,
		"code", nack.Code, "message", nack.Message)
	return true
}

// reassertCustodyLocked implements contract-a.md §7.4. A new sessionId means
// the world may have rolled back to a save that still contains an organism the
// sidecar already exported, so every outbound custody it holds or completed
// within the retention window is re-asserted.
func (s *Sidecar) reassertCustodyLocked() {
	n := 0
	for _, st := range s.jr.List() {
		if st.Direction != journal.Out {
			continue
		}
		switch st.Status {
		case journal.StatusOpen, journal.StatusInFlight, journal.StatusDone, journal.StatusHeld:
		default:
			continue
		}
		s.sendModLocked(contracta.TypeMigrateOutAck, contracta.MigrateOutAck{
			MigrationID: st.Entry.MigrationID,
			EntityID:    st.Entry.EntityID,
			JournaledAt: st.Entry.JournaledAt,
			Unsolicited: true,
		})
		n++
	}
	if n > 0 {
		s.log.Warn("contract A: re-asserted custody after a session change", "count", n)
	}
}

// replayInboundLocked implements contract-a.md §7.5: every journaled inbound
// organism with no MIGRATE_IN_ACK is replayed in journal order, unconditionally
// — and THROUGH THE DELIVERY RATE LIMIT (§15, A20). Replay is exactly the case
// pacing exists for: a slot that slept accumulated a dam, and releasing it all
// at wake is the failure T1 measured.
//
// The batch is scheduled behind ONE escalating, order-preserving delay
// (replayBatchDelay) and the paced tick then releases the entries as tokens
// allow — it is never delivered synchronously here. The delay is the sidecar's
// answer to a reconnect livelock: a batch that keeps stalling its mod and
// forcing a 4004 backs off a little more each generation instead of re-flooding
// the instant the mod reconnects. PACING MUST NOT REORDER, and the tick walks
// the journal in order, so it does not.
func (s *Sidecar) replayInboundLocked() {
	now := s.now()
	batch := make([]*journal.State, 0)
	minAttempt := -1
	for _, st := range s.jr.List() {
		if st.Direction != journal.In {
			continue
		}
		if st.Status != journal.StatusOpen && st.Status != journal.StatusInFlight {
			continue
		}
		if minAttempt < 0 || st.Attempt < minAttempt {
			minAttempt = st.Attempt
		}
		batch = append(batch, st)
	}
	if len(batch) == 0 {
		return
	}
	// One due time for the whole batch keeps journal order: the previous
	// delivery's ACK deadline is overwritten on purpose — the mod that owed that
	// ACK is gone — and the entries become due together, so the tick releases
	// them in journal order at the pacer's rate.
	delay := replayBatchDelay(minAttempt)
	due := now.Add(delay)
	for _, st := range batch {
		s.schedFor(st.Entry.MigrationID).nextDeliver = due
	}
	s.log.Info("contract A: queued un-ACKed inbound deliveries for replay",
		"count", len(batch), "replayDelay", delay, "pacedDepth", s.pacedDepthLocked())
}

// The escalating replay delay of §7.5. A batch that keeps forcing a 4004 climbs
// one step per delivery generation, capped, so a livelock decays instead of
// spinning; ordinary traffic never reaches it because a fresh arrival is not a
// replay.
const (
	replayDelayStep = 500 * time.Millisecond
	replayDelayCap  = 5 * time.Second
)

// replayBatchDelay is the order-preserving delay a reconnect's un-ACKed inbound
// batch waits before the pacer takes over. It escalates with the LEAST-delivered
// entry's attempt count (incremented in deliverLocked), so the livelock —
// deliver, stall, 4004, replay — backs off a little more each generation. A
// first-attempt replay (a delivered-once batch, or a sidecar-restart recovery
// that never delivered, minAttempt 0 or 1) stays prompt; only a repeatedly
// replayed batch is held, up to replayDelayCap.
func replayBatchDelay(minAttempt int) time.Duration {
	d := time.Duration(minAttempt-1) * replayDelayStep
	if d < 0 {
		return 0
	}
	if d > replayDelayCap {
		return replayDelayCap
	}
	return d
}

// modHeartbeatMonitor implements contract-a.md §8. Missed heartbeats close the
// socket with 4004 and close every edge, but custody is never released.
func (s *Sidecar) modHeartbeatMonitor(sess *modSession) {
	t := time.NewTicker(s.cfg.HeartbeatTimeout / 4)
	defer t.Stop()
	for {
		select {
		case <-sess.conn.Done():
			return
		case <-s.ctx.Done():
			return
		case <-t.C:
			// The sidecar's own clock, never the ticker's: every deadline in this
			// process reads one time source, so a test that controls it controls
			// all of them (§9.3's injectable clock).
			now := s.now()
			s.mu.Lock()
			last := sess.lastHeartbeat
			s.mu.Unlock()
			if now.Sub(last) > s.cfg.HeartbeatTimeout {
				s.log.Warn("contract A: heartbeat timeout, closing with 4004",
					"silentFor", now.Sub(last).String())
				sess.conn.Close(contracta.CloseHeartbeatTimeout, "no HEARTBEAT within heartbeatTimeoutMs")
				return
			}
		}
	}
}

// modPinger runs the sidecar-to-mod half of contract-a.md §8's liveness table.
func (s *Sidecar) modPinger(sess *modSession) {
	t := time.NewTicker(s.cfg.WSPingInterval)
	defer t.Stop()
	for {
		select {
		case <-sess.conn.Done():
			return
		case <-s.ctx.Done():
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), s.cfg.WSPongTimeout)
			err := sess.conn.Ping(ctx)
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				s.log.Warn("contract A: pong timeout", "err", err)
				sess.conn.Close(contracta.CloseHeartbeatTimeout, "no pong within wsPongTimeoutMs")
				return
			}
		}
	}
}

// ---------------------------------------------------------------- send helpers

func (s *Sidecar) sendMod(typ string, data any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sendModLocked(typ, data)
}

func (s *Sidecar) sendModLocked(typ string, data any) {
	if s.mod == nil {
		return
	}
	frame, err := wire.Encode(wire.ProtocolA, typ, time.Now().UnixMilli(), data)
	if err != nil {
		s.log.Error("contract A: encode failed", "type", typ, "err", err)
		return
	}
	if err := s.mod.conn.Send(frame); err != nil {
		s.log.Warn("contract A: send failed", "type", typ, "err", err)
	}
}

func (s *Sidecar) nackOut(sess *modSession, migrationID string, entityID int32, code, message string, retryAfterMs int) {
	nack := contracta.MigrateOutNack{
		MigrationID: migrationID,
		EntityID:    entityID,
		Code:        code,
		Class:       contracta.ClassOfOutCode(code),
		Message:     message,
	}
	if nack.Class == contracta.ClassTransient && retryAfterMs > 0 {
		nack.RetryAfterMs = &retryAfterMs
	}
	frame, err := wire.Encode(wire.ProtocolA, contracta.TypeMigrateOutNack, time.Now().UnixMilli(), nack)
	if err != nil {
		s.log.Error("contract A: encode failed", "type", contracta.TypeMigrateOutNack, "err", err)
		return
	}
	if err := sess.conn.Send(frame); err != nil {
		s.log.Warn("contract A: send failed", "type", contracta.TypeMigrateOutNack, "err", err)
	}
	s.log.Warn("contract A: MIGRATE_OUT_NACK", "migrationId", migrationID, "code", code, "message", message)
}

func head(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
