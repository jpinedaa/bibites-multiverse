// Package contractb holds Contract B exactly as contracts/contract-b-m4.md
// specifies it (`contract-b/3.0`): sidecar -> relay -> sidecar, a rectangular
// map of slots, a read-only archive subscriber, a JSON envelope, and an opaque
// bb8 body.
//
// This is a major bump from M3's contract-b/2. The ring became a grid, so a
// reservation gained a coordinate; SECTOR_GRANT returns one effective neighbour
// per export edge instead of one east neighbour; SECTOR_CLAIM carries an array
// of export edges; ringSize became slotCount; MIGRATION_NACK gained the
// non-delivery proof and two new codes, and SLOT_VACANT changed class from
// transient to permanent. A contract-b/2 sidecar and a contract-b/3 relay are
// incompatible by design and say so with close 4000 rather than misrouting an
// organism (contract-b-m4.md §4).
package contractb

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/wire"
)

// Message types (contract-b-m4.md §6). Twelve, none new in M4.
const (
	TypeHandshake        = "HANDSHAKE"
	TypeHandshakeAck     = "HANDSHAKE_ACK"
	TypeSectorClaim      = "SECTOR_CLAIM"
	TypeSectorGrant      = "SECTOR_GRANT"
	TypePeerStatus       = "PEER_STATUS"
	TypeMigrationPayload = "MIGRATION_PAYLOAD"
	TypeMigrationAck     = "MIGRATION_ACK"
	TypeMigrationNack    = "MIGRATION_NACK"
	TypeGenomeRequest    = "GENOME_REQUEST"
	TypeGenomeResponse   = "GENOME_RESPONSE"
	TypePing             = "PING"
	TypePong             = "PONG"
)

// Close codes (contract-b-m4.md §3.2).
const (
	CloseNormal              = 1000
	CloseTooBig              = 1009
	CloseProtocolUnsupported = 4000
	CloseMalformedFrame      = 4003
	CloseLivenessTimeout     = 4004
	CloseShuttingDown        = 4005
	CloseReplaced            = 4006
)

// Roles (contract-b-m4.md §6.1). A peer owns a world and a slot; an archive is
// a read-only subscriber that owns neither.
const (
	RolePeer    = "peer"
	RoleArchive = "archive"
)

// ValidRole reports whether r is one of the two roles.
func ValidRole(r string) bool { return r == RolePeer || r == RoleArchive }

// MIGRATION_NACK codes (contract-b-m4.md §6.8).
const (
	// NackSlotVacant is PERMANENT from M4. destSlot names no reservation at all,
	// and slot numbers are never reused, so that world never returns and no
	// retry can ever succeed. It was transient in M3, when a vacant slot and an
	// offline peer were the same answer.
	NackSlotVacant = "SLOT_VACANT"
	// NackPeerOffline is M3's SLOT_VACANT case, given its own name so the
	// permanent one can mean what it says.
	NackPeerOffline = "PEER_OFFLINE"
	// NackNotForwarded is the relay declining to hand the frame over for a
	// reason of its own: outbound queue full, write failed before any byte left,
	// or the relay is draining.
	NackNotForwarded       = "NOT_FORWARDED"
	NackPeerUnknown        = "PEER_UNKNOWN"
	NackNotAMember         = "NOT_A_MEMBER"
	NackOverloaded         = "OVERLOADED"
	NackSimSizeMismatch    = "SIM_SIZE_MISMATCH"
	NackModAbsent          = "MOD_ABSENT"
	NackInvalidPayload     = "INVALID_PAYLOAD"
	NackKindUnsupported    = "KIND_UNSUPPORTED"
	NackVersionUnsupported = "VERSION_UNSUPPORTED"
	NackMalformedMessage   = "MALFORMED_MESSAGE"
	NackShuttingDown       = "SHUTTING_DOWN"
)

const (
	ClassTransient = "transient"
	ClassPermanent = "permanent"
)

var permanentCodes = map[string]bool{
	NackSlotVacant:         true, // reclassified in M4 (§6.8)
	NackNotAMember:         true,
	NackInvalidPayload:     true,
	NackKindUnsupported:    true,
	NackVersionUnsupported: true,
	NackMalformedMessage:   true,
}

// ClassOf returns the class contract-b-m4.md §6.8 assigns to code. An unknown
// code is transient, the safe default a receiver must have anyway: §6.8 says
// never switch on code without a default branch.
func ClassOf(code string) string {
	if permanentCodes[code] {
		return ClassPermanent
	}
	return ClassTransient
}

// PeerLocalRefusal reports whether code is a receiving SIDECAR's statement that
// it took no custody for a reason of its own (§9.2). §6.8 forbids a sidecar to
// NACK after a durable journal write, so these three codes are proof that
// custody never moved and the entry may be re-routed to another slot.
func PeerLocalRefusal(code string) bool {
	switch code {
	case NackOverloaded, NackSimSizeMismatch, NackModAbsent:
		return true
	}
	return false
}

// PayloadRefusal reports whether code says every slot would refuse this
// organism, so the map is not the answer and it bounces home (§9.2).
func PayloadRefusal(code string) bool {
	switch code {
	case NackInvalidPayload, NackKindUnsupported, NackVersionUnsupported, NackMalformedMessage:
		return true
	}
	return false
}

// SECTOR_GRANT reasons (contract-b-m4.md §6.4).
const (
	GrantGranted             = "granted"      // a new slot was placed
	GrantReclaimed           = "reclaimed"    // the reservation for this peerId was still held
	GrantUpdated             = "updated"      // a repeat claim
	GrantRepositioned        = "repositioned" // same slot, new coordinate, because the map grew
	GrantHandover            = "handover"     // this peer inherited a slot by operator command
	GrantRoleHasNoSlot       = "role_has_no_slot"
	GrantProtocolMismatch    = "protocol_mismatch"
	GrantVersionIncompatible = "version_incompatible"
)

// Skip reasons for SECTOR_GRANT.neighbours.<edge>.skipped[] (§6.4). These are
// the M3 edge-close reasons, demoted: each one now skips a slot instead of
// closing a lane (D12).
const (
	SkipHole             = "hole"
	SkipPeerOffline      = "peer_offline"
	SkipPeerModAbsent    = "peer_mod_absent"
	SkipPeerIncompatible = "peer_incompatible"
	SkipSimSizeMismatch  = "sim_size_mismatch"
)

// Re-route proofs (§6.6, §9.2). Nothing else may appear, because nothing else
// is proof.
const (
	ProofNeverSent           = "never_sent"
	ProofRelayNeverForwarded = "relay_never_forwarded"
	ProofPeerRefused         = "peer_refused"
)

// Lineage gap reasons (contract-b-m4.md §6.6).
const (
	GapParentGone         = "parent_gone" // no blob was shipped: the usual case
	GapBlobInvalid        = "blob_invalid"
	GapBlobDroppedForSize = "blob_dropped_for_size"
)

// GENOME_RESPONSE failure reasons (contract-b-m4.md §6.10).
const (
	GenomeUnknownHash  = "unknown_hash"
	GenomeRateLimited  = "rate_limited"
	GenomePeerOffline  = "peer_offline" // relay-generated
	GenomeTooLarge     = "too_large"
	GenomeShuttingDown = "shutting_down"
)

// Tunable defaults (contract-b-m4.md §12).
const (
	DefaultRelayPort = 8790
	// ContractBPath moved with the major (§3). The relay MUST keep serving
	// RetiredContractBPath and MUST close every connection on it immediately
	// with 4000.
	ContractBPath        = "/contract-b/v3"
	RetiredContractBPath = "/contract-b/v2"

	RelayPingInterval         = 5 * time.Second
	PeerTimeout               = 15 * time.Second
	RelayBackoffMin           = 1 * time.Second
	RelayBackoffMax           = 30 * time.Second
	StableSession             = 5 * time.Second
	AuthFailuresBeforeCeiling = 5
	StatusCoalesce            = 250 * time.Millisecond
	StatsInterval             = 5 * time.Second
	StatsStale                = 30 * time.Second
	ForwardRetry              = 5 * time.Second
	BounceTimeout             = 20 * time.Second
	MigrationAckTimeout       = 30 * time.Second
	// HoldTimeout is the accrued dark time before a held entry bounces home by
	// itself — 24 hours (D2, signed off 2026-08-05). The clock runs only while
	// the destination is dark and the sender can see it (§9.3).
	HoldTimeout = 24 * time.Hour
	// HoldAccrualFlush is how often the accrued hold time is flushed to the
	// journal entry. A crash loses at most this much, in the safe direction.
	HoldAccrualFlush = 60 * time.Second
	// MaxReroutes bounds the re-routes one entry may take before it bounces home
	// instead. An organism circling a broken axis is a symptom, not a delivery
	// strategy.
	MaxReroutes = 4
	// ForwardRecordRetention is how long the relay remembers a forwarded
	// migrationId, in memory, for the neverForwarded proof — 48 hours, twice the
	// default hold. Keep it at least twice HoldTimeout.
	ForwardRecordRetention = 48 * time.Hour
	// StatsBroadcastInterval is the §6.5 timer that republishes PEER_STATUS
	// because stats change without the registry changing. §6.5 names
	// statsBroadcastIntervalMs but §12 does not give it a default; 5000 ms
	// matches statsIntervalMs, the cadence at which the stats it carries arrive.
	StatsBroadcastInterval  = 5 * time.Second
	ArchiveQueueMax         = 1024
	GenomeRequestTimeout    = 15 * time.Second
	GenomeRequestsPerMinute = 30
	GenomeCacheRetention    = 30 * 24 * time.Hour
	GenomeCacheMaxBytes     = int64(2147483648)
)

// ErrInvalid marks a data-level validation failure.
var ErrInvalid = errors.New("contractb: invalid message data")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Vec mirrors contracta.Vec on the sidecar-to-sidecar wire.
type Vec struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Position is a coordinate in the map (§2). It decides who a peer's neighbours
// are, and nothing else. A position moves when the map grows; an address never
// moves, which is why no migration frame carries one.
type Position struct {
	Col int `json:"col"`
	Row int `json:"row"`
}

// MapShape is the rectangle: every col in [0,width) and every row in [0,height)
// is a position that exists (§2).
type MapShape struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Handshake is HANDSHAKE (contract-b-m4.md §6.1), the first frame on every
// connection.
type Handshake struct {
	PeerID          string  `json:"peerId"`
	Role            string  `json:"role"`
	ProtocolVersion string  `json:"protocolVersion"`
	GameVersion     string  `json:"gameVersion"`
	SidecarVersion  string  `json:"sidecarVersion"`
	SimulationSize  float64 `json:"simulationSize,omitempty"`
}

func (h *Handshake) Validate() error {
	if h.PeerID == "" {
		return invalid("peerId is empty")
	}
	if len(h.PeerID) > 64 {
		return invalid("peerId is %d characters, over the 64 limit", len(h.PeerID))
	}
	for i := 0; i < len(h.PeerID); i++ {
		c := h.PeerID[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '.', c == '_', c == '-':
		default:
			return invalid("peerId contains %q, outside [A-Za-z0-9._-]", string(c))
		}
	}
	if !ValidRole(h.Role) {
		return invalid("role %q is not peer/archive", h.Role)
	}
	if h.ProtocolVersion == "" {
		return invalid("protocolVersion is empty")
	}
	return nil
}

// HandshakeAck is HANDSHAKE_ACK (contract-b-m4.md §6.2).
type HandshakeAck struct {
	RelayVersion    string `json:"relayVersion"`
	ProtocolVersion string `json:"protocolVersion"`
	// RelaySessionID is minted once at relay start and constant for the life of
	// the process. It is the scope of the forwarding record (§5.2), and a
	// sidecar MUST persist it against every journal entry it hands over while
	// this connection is live (§9.2).
	RelaySessionID   string    `json:"relaySessionId"`
	AssignedSlot     *int      `json:"assignedSlot,omitempty"`
	AssignedPosition *Position `json:"assignedPosition,omitempty"`
	Map              MapShape  `json:"map"`
	SlotCount        int       `json:"slotCount"`
	ReceivedAt       int64     `json:"receivedAt"`
}

// SaveReceipt is the mod's save receipt, copied verbatim from
// HEARTBEAT.lastSave (contract-a.md §5.2, §15 A21).
type SaveReceipt = contracta.SaveReceipt

// PeerStats is the peer stats block of §6.3.1. One shape, three carriers: a
// sidecar sends it on SECTOR_CLAIM and on PING, and the relay republishes the
// latest value it holds in PEER_STATUS.
//
// EVERY FIELD IS OPTIONAL, AND ABSENCE IS A VALUE. A stat the sidecar does not
// know is omitted, never defaulted: a slot that reports nothing is unknown, not
// empty, and an honest gap beats a confident zero (Risk 4).
type PeerStats struct {
	Population *int `json:"population,omitempty"`
	EggCount   *int `json:"eggCount,omitempty"`
	// CustodyDepth is outbound entries awaiting MIGRATION_ACK plus inbound
	// entries awaiting MIGRATE_IN_ACK.
	CustodyDepth *int `json:"custodyDepth,omitempty"`
	// PacedDepth is inbound entries waiting on the delivery rate limit
	// (contract-a.md §7.5). A depth that never falls names a limit set too low.
	PacedDepth *int `json:"pacedDepth,omitempty"`
	// HeldDepth is outbound entries in the held state of §9.2 — forwarded,
	// unproven, destination dark.
	HeldDepth *int `json:"heldDepth,omitempty"`
	// BouncedTimeoutTotal is cumulative and monotonic. An automatic bounce is a
	// fact the operator reads, not a silent repair (§9.3).
	BouncedTimeoutTotal *int         `json:"bouncedTimeoutTotal,omitempty"`
	SimulatedTime       *float64     `json:"simulatedTime,omitempty"`
	LastSave            *SaveReceipt `json:"lastSave,omitempty"`
}

// SectorClaim is SECTOR_CLAIM (contract-b-m4.md §6.3) — a placement claim. A
// repeat claim from a peer that already holds a slot is an update, never a
// second claim. Every part of it is advisory and it never fails for a lost race.
type SectorClaim struct {
	PreferredSlot int `json:"preferredSlot,omitempty"`
	// PreferredPosition may name a hole inside the current rectangle, or a
	// position exactly one column or one row outside it, which asks the relay to
	// extend the map on that axis (§7.2 rule 4).
	PreferredPosition *Position `json:"preferredPosition,omitempty"`
	// InsertAfterSlot is the advisory splice of §7.2 rule 5.
	InsertAfterSlot int     `json:"insertAfterSlot,omitempty"`
	InsertAxis      string  `json:"insertAxis,omitempty"`
	SimulationSize  float64 `json:"simulationSize"`
	// ExportEdges replaces M3's singular exportEdge (§15, A18).
	ExportEdges  []string   `json:"exportEdges"`
	BorderEdges  []string   `json:"borderEdges"`
	GameVersion  string     `json:"gameVersion,omitempty"`
	ModConnected bool       `json:"modConnected"`
	Stats        *PeerStats `json:"stats,omitempty"`
}

func (c *SectorClaim) Validate() error {
	if c.PreferredSlot < 0 {
		return invalid("preferredSlot %d is negative", c.PreferredSlot)
	}
	if c.PreferredPosition != nil && (c.PreferredPosition.Col < 0 || c.PreferredPosition.Row < 0) {
		return invalid("preferredPosition %+v has a negative coordinate", *c.PreferredPosition)
	}
	if c.InsertAfterSlot < 0 {
		return invalid("insertAfterSlot %d is negative", c.InsertAfterSlot)
	}
	if c.InsertAxis != "" && c.InsertAxis != contracta.EdgeE && c.InsertAxis != contracta.EdgeN {
		return invalid("insertAxis %q is not E/N", c.InsertAxis)
	}
	if !wire.Finite(c.SimulationSize) || c.SimulationSize < 0 {
		return invalid("simulationSize %v is not a non-negative finite number", c.SimulationSize)
	}
	for _, e := range c.ExportEdges {
		if !contracta.ValidEdge(e) {
			return invalid("exportEdges contains %q", e)
		}
	}
	return nil
}

// Skip is one entry of SECTOR_GRANT.neighbours.<edge>.skipped[] (§6.4). Slot is
// nil for a hole — a position with no reservation at all.
type Skip struct {
	Slot     *int     `json:"slot"`
	Position Position `json:"position"`
	Reason   string   `json:"reason"`
}

// Neighbour is one entry of SECTOR_GRANT.neighbours (contract-b-m4.md §6.4):
// the EFFECTIVE target on that export edge, after skipping holes and
// undeliverable slots (§8).
type Neighbour struct {
	Slot           int      `json:"slot"`
	PeerID         string   `json:"peerId"`
	Position       Position `json:"position"`
	Live           bool     `json:"live"`
	ModConnected   bool     `json:"modConnected"`
	GameVersion    string   `json:"gameVersion"`
	SimulationSize float64  `json:"simulationSize"`
	// Skipped is the bypass list, in walk order. Empty when the lane is direct.
	Skipped []Skip `json:"skipped"`
}

// SectorGrant is SECTOR_GRANT (contract-b-m4.md §6.4). The slot, the position,
// the map and one effective neighbour per export edge are the entire topology a
// sidecar needs.
//
// A key absent from Neighbours is what closes that export edge with no_peer
// (§8): a target is deliverable by construction, so an entry is never a closed
// lane.
type SectorGrant struct {
	Granted    bool                  `json:"granted"`
	Slot       int                   `json:"slot,omitempty"`
	Position   *Position             `json:"position,omitempty"`
	Map        MapShape              `json:"map"`
	SlotCount  int                   `json:"slotCount"`
	Reason     string                `json:"reason"`
	Neighbours map[string]*Neighbour `json:"neighbours,omitempty"`
}

// SlotInfo is one entry of PEER_STATUS.slots (contract-b-m4.md §6.5). A slot
// with no live peer stays in the map with live:false; it does not disappear.
type SlotInfo struct {
	Slot           int      `json:"slot"`
	Position       Position `json:"position"`
	PeerID         string   `json:"peerId"`
	Live           bool     `json:"live"`
	ModConnected   bool     `json:"modConnected"`
	GameVersion    string   `json:"gameVersion"`
	SimulationSize float64  `json:"simulationSize"`
	ExportEdges    []string `json:"exportEdges"`
	LastSeenMs     int64    `json:"lastSeenMs,omitempty"`
	// DarkSinceMs is the relay clock at the moment this peer's connection was
	// lost. Present exactly when Live is false and the relay saw it go. This is
	// the field Risk 5 needs: a healed map hides a dead world, and "bypassed
	// since 04:12" is what stops an operator missing it for a day.
	DarkSinceMs int64      `json:"darkSinceMs,omitempty"`
	LastRefusal string     `json:"lastRefusal,omitempty"`
	Stats       *PeerStats `json:"stats,omitempty"`
	// StatsAsOfMs is the relay clock when that block arrived. A reader MUST use
	// it to age the stats: a population from a peer that went dark an hour ago
	// is history, not state.
	StatsAsOfMs int64 `json:"statsAsOfMs,omitempty"`
}

// You is the receiving client's own place in the map. All null for a subscriber.
type You struct {
	Slot       *int            `json:"slot"`
	Position   *Position       `json:"position"`
	Neighbours map[string]*int `json:"neighbours"`
}

// PeerStatus is PEER_STATUS (contract-b-m4.md §6.5). Full state, not a delta,
// and it reports THE STRUCTURE, NOT THE EFFECT: the map as it is reserved, in
// structural order (row ascending, then col ascending). The lanes as they
// currently run are in each peer's own SECTOR_GRANT.
type PeerStatus struct {
	Epoch     int64      `json:"epoch"`
	Map       MapShape   `json:"map"`
	SlotCount int        `json:"slotCount"`
	Slots     []SlotInfo `json:"slots"`
	You       You        `json:"you"`
	Observers int        `json:"observers"`
}

// Holes derives the positions inside the rectangle that no slot names (§6.5).
// They are never sent: a second copy of a fact already on the wire is a second
// thing to get out of step.
func (p *PeerStatus) Holes() []Position {
	taken := map[Position]bool{}
	for _, s := range p.Slots {
		taken[s.Position] = true
	}
	out := []Position{}
	for row := 0; row < p.Map.Height; row++ {
		for col := 0; col < p.Map.Width; col++ {
			pos := Position{Col: col, Row: row}
			if !taken[pos] {
				out = append(out, pos)
			}
		}
	}
	return out
}

// Body is the kind=bibite body of the Contract C MigrationEnvelope.
type Body struct {
	Version string `json:"version"`
	BB8     string `json:"bb8"`
}

// Parent is one entry of the lineage annex (contract-b-m4.md §6.6). An absent
// GenomeHash is a gap, and GapReason then says why.
type Parent struct {
	EntityID   int32  `json:"entityId"`
	GenomeHash string `json:"genomeHash,omitempty"`
	GapReason  string `json:"gapReason,omitempty"`
}

// Lineage is the annex (D11). It is always present; Parents may be empty.
type Lineage struct {
	GenomeHash string   `json:"genomeHash"`
	Parents    []Parent `json:"parents"`
}

// Reroute is MIGRATION_PAYLOAD.reroute (§6.6), present exactly when this
// frame's destSlot is not the one the entry was first journaled with.
// Informational: the relay does not read it and the receiver does not act on
// it. It is what lets the archive and the status page say WHY an organism took
// the lane it took.
type Reroute struct {
	FromSlot int    `json:"fromSlot"`
	Count    int    `json:"count"`
	Proof    string `json:"proof"`
	AtMs     int64  `json:"atMs"`
}

// MigrationPayload is MIGRATION_PAYLOAD (contract-b-m4.md §6.6): the Contract C
// MigrationEnvelope with the lineage annex. The wire never carries a parent
// blob — the source sidecar hashes them, caches them and strips them.
type MigrationPayload struct {
	MigrationID string  `json:"migrationId"`
	Kind        string  `json:"kind"`
	Body        Body    `json:"body"`
	Lineage     Lineage `json:"lineage"`
	SourcePeer  string  `json:"sourcePeer"`
	SourceSlot  int     `json:"sourceSlot"`
	DestSlot    int     `json:"destSlot"`
	// ExitEdge is "E" or "N" under the grid. It is the axis the organism left
	// by, and it is what the receiver turns into an entry edge.
	ExitEdge     string   `json:"exitEdge"`
	ExitPosition float64  `json:"exitPosition"`
	Velocity     Vec      `json:"velocity"`
	Heading      float64  `json:"heading"`
	EntityID     int32    `json:"entityId"`
	Timestamp    int64    `json:"timestamp"`
	Reroute      *Reroute `json:"reroute,omitempty"`
}

func (p *MigrationPayload) Validate() error {
	if !wire.ValidUUID(p.MigrationID) {
		return invalid("migrationId %q is not a uuid", p.MigrationID)
	}
	if p.Kind == "" {
		return invalid("kind is empty")
	}
	if p.Body.Version == "" {
		return invalid("body.version is empty")
	}
	if p.Body.BB8 == "" {
		return invalid("body.bb8 is empty")
	}
	if p.SourcePeer == "" {
		return invalid("sourcePeer is empty")
	}
	if p.DestSlot < 1 {
		return invalid("destSlot %d is not a slot", p.DestSlot)
	}
	if p.ExitEdge != contracta.EdgeE && p.ExitEdge != contracta.EdgeN {
		return invalid("exitEdge %q is not E/N; the grid exports east and north", p.ExitEdge)
	}
	if !wire.Finite(p.ExitPosition) || p.ExitPosition < 0 || p.ExitPosition > 1 {
		return invalid("exitPosition %v is outside [0,1]", p.ExitPosition)
	}
	if !wire.Finite(p.Velocity.X) || !wire.Finite(p.Velocity.Y) {
		return invalid("velocity is not finite")
	}
	if !wire.Finite(p.Heading) {
		return invalid("heading is not finite")
	}
	// The annex is never a reason to refuse an organism (§6.8), so nothing
	// about Lineage is validated here.
	return nil
}

// MigrationAck is MIGRATION_ACK (contract-b-m4.md §6.7). It is sent only after
// the receiving mod's Contract A MIGRATE_IN_ACK — pacing does not change that
// rule, and Risk 9 is the reason to say so out loud.
type MigrationAck struct {
	MigrationID string `json:"migrationId"`
	SourcePeer  string `json:"sourcePeer"`
	DestPeer    string `json:"destPeer"`
	EntityID    int32  `json:"entityId"`
	Duplicate   bool   `json:"duplicate"`
	DeliveredAt int64  `json:"deliveredAt"`
}

// MigrationNack is MIGRATION_NACK (contract-b-m4.md §6.8). It is never sent by
// a sidecar after durable custody, which is what makes both the bounce-back and
// the M4 re-route safe.
type MigrationNack struct {
	MigrationID  string `json:"migrationId"`
	SourcePeer   string `json:"sourcePeer"`
	DestPeer     string `json:"destPeer"`
	Code         string `json:"code"`
	Class        string `json:"class"`
	Message      string `json:"message"`
	RetryAfterMs int    `json:"retryAfterMs,omitempty"`
	// NeverForwarded is present exactly on RELAY-generated NACKs. true means:
	// this relay process has forwarded no frame carrying this migrationId to any
	// peer, ever, during RelaySessionID. It is a PROOF, NOT A HINT (§5.2, §9.2),
	// and a missing field is no proof at all.
	NeverForwarded *bool `json:"neverForwarded,omitempty"`
	// RelaySessionID is present exactly when NeverForwarded is. A sender MUST
	// compare it against the session recorded on the journal entry before
	// treating NeverForwarded: true as proof.
	RelaySessionID string `json:"relaySessionId,omitempty"`
}

// ProvesNoCustody implements §9.2's evidence table for a RELAY-generated NACK:
// neverForwarded true AND a relaySessionId equal to the one recorded on the
// entry. Every ambiguity resolves toward "no proof" — toward holding — because
// holding costs a delay and re-routing on a bad proof costs a duplicated
// organism.
func (n *MigrationNack) ProvesNoCustody(entrySession string) bool {
	if n.NeverForwarded == nil || !*n.NeverForwarded {
		return false
	}
	if n.RelaySessionID == "" || entrySession == "" {
		return false
	}
	return n.RelaySessionID == entrySession
}

// GenomeContext is GENOME_REQUEST.context: the annex the hash came from.
type GenomeContext struct {
	MigrationID string `json:"migrationId,omitempty"`
	EntityID    int32  `json:"entityId,omitempty"`
}

// GenomeRequest is GENOME_REQUEST (contract-b-m4.md §6.9).
type GenomeRequest struct {
	RequestID  string         `json:"requestId"`
	SourcePeer string         `json:"sourcePeer"`
	DestPeer   string         `json:"destPeer"`
	GenomeHash string         `json:"genomeHash"`
	Context    *GenomeContext `json:"context,omitempty"`
}

// GenomeResponse is GENOME_RESPONSE (contract-b-m4.md §6.10). The relay
// generates one when it cannot route the request.
type GenomeResponse struct {
	RequestID    string `json:"requestId"`
	SourcePeer   string `json:"sourcePeer"`
	DestPeer     string `json:"destPeer"`
	GenomeHash   string `json:"genomeHash"`
	Found        bool   `json:"found"`
	Body         *Body  `json:"body,omitempty"`
	Reason       string `json:"reason,omitempty"`
	RetryAfterMs int    `json:"retryAfterMs,omitempty"`
}

// Ping carries a nonce and, from a peer, MAY carry the stats block (§6.11).
// This is where a fresh population comes from: a SECTOR_CLAIM is sent on change
// and would leave the map view showing the population each world had when it
// last resized.
type Ping struct {
	Nonce string     `json:"nonce"`
	Stats *PeerStats `json:"stats,omitempty"`
}

type Pong struct {
	Nonce string `json:"nonce"`
}

// Routing is the only part of data the relay decodes (contract-b-m4.md §5). It
// never touches body.bb8 and never decodes the lineage annex.
type Routing struct {
	SourcePeer string `json:"sourcePeer"`
	DestPeer   string `json:"destPeer"`
	DestSlot   int    `json:"destSlot"`
}

// Identity is the little the relay reads to answer a frame it cannot route.
type Identity struct {
	MigrationID string `json:"migrationId"`
	RequestID   string `json:"requestId"`
	GenomeHash  string `json:"genomeHash"`
}

// DecodeData decodes a frame body into v.
func DecodeData(raw json.RawMessage, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return invalid("%v", err)
	}
	return nil
}

// IntPtr and friends keep the "absence is a value" rule readable at call sites.
func IntPtr(v int) *int             { return &v }
func Float64Ptr(v float64) *float64 { return &v }
func BoolPtr(v bool) *bool          { return &v }
