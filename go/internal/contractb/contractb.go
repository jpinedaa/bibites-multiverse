// Package contractb holds Contract B exactly as contracts/contract-b-m3.md
// specifies it (`contract-b/2.0`): sidecar -> relay -> sidecar, a ring of slots,
// a read-only archive subscriber, a JSON envelope, and an opaque bb8 body.
//
// This is a major bump from M2's contract-b/1. The ["A","B"] sector set is
// gone, sourceSector and destSector became integer ring slots, SECTOR_GRANT
// carries the east neighbour, and two message types are new. A contract-b/1
// sidecar and a contract-b/2 relay are incompatible by design and say so with
// close 4000 rather than misrouting an organism (contract-b-m3.md §4).
package contractb

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"multiverse/internal/wire"
)

// Message types (contract-b-m3.md §6). Twelve, two new in M3.
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

// Close codes (contract-b-m3.md §3.2).
const (
	CloseNormal              = 1000
	CloseTooBig              = 1009
	CloseProtocolUnsupported = 4000
	CloseMalformedFrame      = 4003
	CloseLivenessTimeout     = 4004
	CloseShuttingDown        = 4005
	CloseReplaced            = 4006
)

// Roles (contract-b-m3.md §6.1). A peer owns a world and a ring slot; an
// archive is a read-only subscriber that owns neither.
const (
	RolePeer    = "peer"
	RoleArchive = "archive"
)

// ValidRole reports whether r is one of the two roles.
func ValidRole(r string) bool { return r == RolePeer || r == RoleArchive }

// MIGRATION_NACK codes (contract-b-m3.md §6.8).
const (
	NackSlotVacant         = "SLOT_VACANT" // renamed from M2's SECTOR_VACANT
	NackPeerUnknown        = "PEER_UNKNOWN"
	NackNotAMember         = "NOT_A_MEMBER" // new in M3: a subscriber may not send
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
	NackNotAMember:         true,
	NackInvalidPayload:     true,
	NackKindUnsupported:    true,
	NackVersionUnsupported: true,
	NackMalformedMessage:   true,
}

// ClassOf returns the class contract-b-m3.md §6.8 assigns to code. An unknown
// code is transient, the safe default a receiver must have anyway: §6.8 says
// never switch on code without a default branch.
func ClassOf(code string) string {
	if permanentCodes[code] {
		return ClassPermanent
	}
	return ClassTransient
}

// SECTOR_GRANT reasons (contract-b-m3.md §6.4).
const (
	GrantGranted             = "granted"   // a new slot was inserted at the tail
	GrantReclaimed           = "reclaimed" // the reservation for this peerId was still held
	GrantUpdated             = "updated"   // a repeat claim from a peer that holds a slot
	GrantRoleHasNoSlot       = "role_has_no_slot"
	GrantProtocolMismatch    = "protocol_mismatch"
	GrantVersionIncompatible = "version_incompatible"
)

// Lineage gap reasons (contract-b-m3.md §6.6).
const (
	GapParentGone         = "parent_gone" // no blob was shipped: the usual case
	GapBlobInvalid        = "blob_invalid"
	GapBlobDroppedForSize = "blob_dropped_for_size"
)

// GENOME_RESPONSE failure reasons (contract-b-m3.md §6.10).
const (
	GenomeUnknownHash  = "unknown_hash"
	GenomeRateLimited  = "rate_limited"
	GenomePeerOffline  = "peer_offline" // relay-generated
	GenomeTooLarge     = "too_large"
	GenomeShuttingDown = "shutting_down"
)

// Tunable defaults (contract-b-m3.md §12).
const (
	DefaultRelayPort          = 8790
	ContractBPath             = "/contract-b/v2"
	RelayPingInterval         = 5 * time.Second
	PeerTimeout               = 15 * time.Second
	RelayBackoffMin           = 1 * time.Second
	RelayBackoffMax           = 30 * time.Second
	StableSession             = 5 * time.Second
	AuthFailuresBeforeCeiling = 5
	ForwardRetry              = 5 * time.Second
	BounceTimeout             = 20 * time.Second
	MigrationAckTimeout       = 30 * time.Second
	ArchiveQueueMax           = 1024
	GenomeRequestTimeout      = 15 * time.Second
	GenomeRequestsPerMinute   = 30
	GenomeCacheRetention      = 30 * 24 * time.Hour
	GenomeCacheMaxBytes       = int64(2147483648)
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

// Handshake is HANDSHAKE (contract-b-m3.md §6.1), the first frame on every
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

// HandshakeAck is HANDSHAKE_ACK (contract-b-m3.md §6.2).
type HandshakeAck struct {
	RelayVersion    string `json:"relayVersion"`
	ProtocolVersion string `json:"protocolVersion"`
	AssignedSlot    *int   `json:"assignedSlot,omitempty"`
	RingSize        int    `json:"ringSize"`
	ReceivedAt      int64  `json:"receivedAt"`
}

// SectorClaim is SECTOR_CLAIM (contract-b-m3.md §6.3) — a ring claim. A repeat
// claim from a peer that already holds a slot is an update, never a second
// claim.
type SectorClaim struct {
	PreferredSlot  int      `json:"preferredSlot,omitempty"`
	SimulationSize float64  `json:"simulationSize"`
	ExportEdge     string   `json:"exportEdge"`
	BorderEdges    []string `json:"borderEdges"`
	GameVersion    string   `json:"gameVersion,omitempty"`
	ModConnected   bool     `json:"modConnected"`
}

func (c *SectorClaim) Validate() error {
	if c.PreferredSlot < 0 {
		return invalid("preferredSlot %d is negative", c.PreferredSlot)
	}
	if !wire.Finite(c.SimulationSize) || c.SimulationSize < 0 {
		return invalid("simulationSize %v is not a non-negative finite number", c.SimulationSize)
	}
	return nil
}

// Neighbour is SECTOR_GRANT.eastNeighbour (contract-b-m3.md §6.4). The slot and
// the east neighbour together are the entire topology a sidecar needs (D8).
type Neighbour struct {
	Slot           int     `json:"slot"`
	PeerID         string  `json:"peerId"`
	Live           bool    `json:"live"`
	ModConnected   bool    `json:"modConnected"`
	GameVersion    string  `json:"gameVersion"`
	SimulationSize float64 `json:"simulationSize"`
}

// SectorGrant is SECTOR_GRANT (contract-b-m3.md §6.4).
type SectorGrant struct {
	Granted       bool       `json:"granted"`
	Slot          int        `json:"slot,omitempty"`
	RingSize      int        `json:"ringSize"`
	Reason        string     `json:"reason"`
	EastNeighbour *Neighbour `json:"eastNeighbour,omitempty"`
}

// SlotInfo is one entry of PEER_STATUS.slots (contract-b-m3.md §6.5). A slot
// with no live peer stays in the ring with live:false; it does not disappear.
type SlotInfo struct {
	Slot           int     `json:"slot"`
	PeerID         string  `json:"peerId"`
	Live           bool    `json:"live"`
	ModConnected   bool    `json:"modConnected"`
	GameVersion    string  `json:"gameVersion"`
	SimulationSize float64 `json:"simulationSize"`
	LastSeenMs     int64   `json:"lastSeenMs,omitempty"`
	LastRefusal    string  `json:"lastRefusal,omitempty"`
}

// You is the receiving client's own position in the ring. Both fields are null
// for a subscriber and at ringSize 1.
type You struct {
	Slot              *int `json:"slot"`
	EastNeighbourSlot *int `json:"eastNeighbourSlot"`
}

// PeerStatus is PEER_STATUS (contract-b-m3.md §6.5). Full state, not a delta,
// and it reports the ring order rather than a peer list.
type PeerStatus struct {
	Epoch     int64      `json:"epoch"`
	RingSize  int        `json:"ringSize"`
	Slots     []SlotInfo `json:"slots"`
	You       You        `json:"you"`
	Observers int        `json:"observers"`
}

// Body is the kind=bibite body of the Contract C MigrationEnvelope.
type Body struct {
	Version string `json:"version"`
	BB8     string `json:"bb8"`
}

// Parent is one entry of the lineage annex (contract-b-m3.md §6.6). An absent
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

// MigrationPayload is MIGRATION_PAYLOAD (contract-b-m3.md §6.6): the Contract C
// MigrationEnvelope with the lineage annex. The wire never carries a parent
// blob — the source sidecar hashes them, caches them and strips them.
type MigrationPayload struct {
	MigrationID  string  `json:"migrationId"`
	Kind         string  `json:"kind"`
	Body         Body    `json:"body"`
	Lineage      Lineage `json:"lineage"`
	SourcePeer   string  `json:"sourcePeer"`
	SourceSlot   int     `json:"sourceSlot"`
	DestSlot     int     `json:"destSlot"`
	ExitEdge     string  `json:"exitEdge"`
	ExitPosition float64 `json:"exitPosition"`
	Velocity     Vec     `json:"velocity"`
	Heading      float64 `json:"heading"`
	EntityID     int32   `json:"entityId"`
	Timestamp    int64   `json:"timestamp"`
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
		return invalid("destSlot %d is not a ring slot", p.DestSlot)
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

// MigrationAck is MIGRATION_ACK (contract-b-m3.md §6.7). It is sent only after
// the receiving mod's Contract A MIGRATE_IN_ACK.
type MigrationAck struct {
	MigrationID string `json:"migrationId"`
	SourcePeer  string `json:"sourcePeer"`
	DestPeer    string `json:"destPeer"`
	EntityID    int32  `json:"entityId"`
	Duplicate   bool   `json:"duplicate"`
	DeliveredAt int64  `json:"deliveredAt"`
}

// MigrationNack is MIGRATION_NACK (contract-b-m3.md §6.8). It is never sent
// after durable custody, which is what makes the origin's bounce-back safe.
type MigrationNack struct {
	MigrationID  string `json:"migrationId"`
	SourcePeer   string `json:"sourcePeer"`
	DestPeer     string `json:"destPeer"`
	Code         string `json:"code"`
	Class        string `json:"class"`
	Message      string `json:"message"`
	RetryAfterMs int    `json:"retryAfterMs,omitempty"`
}

// GenomeContext is GENOME_REQUEST.context: the annex the hash came from.
type GenomeContext struct {
	MigrationID string `json:"migrationId,omitempty"`
	EntityID    int32  `json:"entityId,omitempty"`
}

// GenomeRequest is GENOME_REQUEST (contract-b-m3.md §6.9), new in M3.
type GenomeRequest struct {
	RequestID  string         `json:"requestId"`
	SourcePeer string         `json:"sourcePeer"`
	DestPeer   string         `json:"destPeer"`
	GenomeHash string         `json:"genomeHash"`
	Context    *GenomeContext `json:"context,omitempty"`
}

// GenomeResponse is GENOME_RESPONSE (contract-b-m3.md §6.10). The relay
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

// Ping and Pong carry a nonce (contract-b-m3.md §6.11).
type Ping struct {
	Nonce string `json:"nonce"`
}

type Pong struct {
	Nonce string `json:"nonce"`
}

// Routing is the only part of data the relay decodes (contract-b-m3.md §5). It
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
