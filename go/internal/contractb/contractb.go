// Package contractb holds the M2 subset of Contract B exactly as
// contracts/contract-b-m2.md specifies it: sidecar -> relay -> sidecar, two
// sectors, JSON envelope, opaque bb8 body.
package contractb

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"multiverse/internal/wire"
)

// Message types (contract-b-m2.md §5).
const (
	TypeHandshake        = "HANDSHAKE"
	TypeHandshakeAck     = "HANDSHAKE_ACK"
	TypeSectorClaim      = "SECTOR_CLAIM"
	TypeSectorGrant      = "SECTOR_GRANT"
	TypePeerStatus       = "PEER_STATUS"
	TypeMigrationPayload = "MIGRATION_PAYLOAD"
	TypeMigrationAck     = "MIGRATION_ACK"
	TypeMigrationNack    = "MIGRATION_NACK"
	TypePing             = "PING"
	TypePong             = "PONG"
)

// Close codes (contract-b-m2.md §2.1).
const (
	CloseNormal              = 1000
	CloseProtocolUnsupported = 4000
	CloseMalformedFrame      = 4003
	CloseLivenessTimeout     = 4004
	CloseShuttingDown        = 4005
	CloseReplaced            = 4006
)

// The M2 sector set (contract-b-m2.md §1).
const (
	SectorA = "A"
	SectorB = "B"
)

// Sectors is the fixed assignment order: the first peer gets A, the second B.
var Sectors = []string{SectorA, SectorB}

// ValidSector reports whether s is one of the two M2 sectors.
func ValidSector(s string) bool { return s == SectorA || s == SectorB }

// MIGRATION_NACK codes (contract-b-m2.md §5.8).
const (
	NackSectorVacant       = "SECTOR_VACANT"
	NackPeerUnknown        = "PEER_UNKNOWN"
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
	NackInvalidPayload:     true,
	NackKindUnsupported:    true,
	NackVersionUnsupported: true,
	NackMalformedMessage:   true,
}

// ClassOf returns the class contract-b-m2.md §5.8 assigns to code. An unknown
// code is transient, the safe default.
func ClassOf(code string) string {
	if permanentCodes[code] {
		return ClassPermanent
	}
	return ClassTransient
}

// SECTOR_GRANT reasons (contract-b-m2.md §5.4).
const (
	GrantGranted           = "granted"
	GrantReclaimed         = "reclaimed"
	GrantNoSectorAvailable = "no_sector_available"
	GrantProtocolMismatch  = "protocol_mismatch"
)

// Tunable defaults (contract-b-m2.md §9).
const (
	DefaultRelayPort    = 8790
	ContractBPath       = "/contract-b/v1"
	RelayPingInterval   = 5 * time.Second
	PeerTimeout         = 15 * time.Second
	RelayBackoffMin     = 1 * time.Second
	RelayBackoffMax     = 30 * time.Second
	ForwardRetry        = 5 * time.Second
	BounceTimeout       = 20 * time.Second
	MigrationAckTimeout = 30 * time.Second
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

// Handshake is HANDSHAKE (contract-b-m2.md §5.1).
type Handshake struct {
	PeerID          string  `json:"peerId"`
	ProtocolVersion string  `json:"protocolVersion"`
	GameVersion     string  `json:"gameVersion"`
	SidecarVersion  string  `json:"sidecarVersion"`
	SimulationSize  float64 `json:"simulationSize,omitempty"`
}

func (h *Handshake) Validate() error {
	if h.PeerID == "" {
		return invalid("peerId is empty")
	}
	if h.ProtocolVersion == "" {
		return invalid("protocolVersion is empty")
	}
	return nil
}

// HandshakeAck is HANDSHAKE_ACK (contract-b-m2.md §5.2).
type HandshakeAck struct {
	RelayVersion    string `json:"relayVersion"`
	ProtocolVersion string `json:"protocolVersion"`
	AssignedSector  string `json:"assignedSector,omitempty"`
}

// SectorClaim is SECTOR_CLAIM (contract-b-m2.md §5.3).
type SectorClaim struct {
	PreferredSector string   `json:"preferredSector,omitempty"`
	SimulationSize  float64  `json:"simulationSize"`
	BorderEdges     []string `json:"borderEdges"`
	GameVersion     string   `json:"gameVersion,omitempty"`
	ModConnected    bool     `json:"modConnected"`
}

func (c *SectorClaim) Validate() error {
	if c.PreferredSector != "" && !ValidSector(c.PreferredSector) {
		return invalid("preferredSector %q is not A/B", c.PreferredSector)
	}
	if !wire.Finite(c.SimulationSize) || c.SimulationSize < 0 {
		return invalid("simulationSize %v is not a non-negative finite number", c.SimulationSize)
	}
	return nil
}

// SectorGrant is SECTOR_GRANT (contract-b-m2.md §5.4).
type SectorGrant struct {
	Granted bool   `json:"granted"`
	Sector  string `json:"sector,omitempty"`
	Reason  string `json:"reason"`
}

// PeerInfo is one entry of PEER_STATUS.peers (contract-b-m2.md §5.5).
type PeerInfo struct {
	PeerID         string  `json:"peerId"`
	Sector         string  `json:"sector"`
	GameVersion    string  `json:"gameVersion"`
	SimulationSize float64 `json:"simulationSize"`
	ModConnected   bool    `json:"modConnected"`
}

// PeerStatus is PEER_STATUS (contract-b-m2.md §5.5). Full state, not a delta.
// A sector absent from Peers is vacant.
type PeerStatus struct {
	Epoch int64      `json:"epoch"`
	Peers []PeerInfo `json:"peers"`
}

// Body is the kind=bibite body of the Contract C MigrationEnvelope.
type Body struct {
	Version string `json:"version"`
	BB8     string `json:"bb8"`
}

// MigrationPayload is MIGRATION_PAYLOAD, the Contract C MigrationEnvelope
// (contract-b-m2.md §5.6). EntityID and Heading are the two documented M2
// additions (§8, item 2).
type MigrationPayload struct {
	MigrationID  string  `json:"migrationId"`
	Kind         string  `json:"kind"`
	Body         Body    `json:"body"`
	SourcePeer   string  `json:"sourcePeer"`
	SourceSector string  `json:"sourceSector"`
	DestSector   string  `json:"destSector"`
	ExitEdge     string  `json:"exitEdge"`
	ExitPosition float64 `json:"exitPosition"`
	Velocity     Vec     `json:"velocity"`
	Timestamp    int64   `json:"timestamp"`
	EntityID     int32   `json:"entityId"`
	Heading      float64 `json:"heading"`
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
	if !ValidSector(p.DestSector) {
		return invalid("destSector %q is not A/B", p.DestSector)
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
	return nil
}

// MigrationAck is MIGRATION_ACK (contract-b-m2.md §5.7). It is sent only after
// the receiving mod's Contract A MIGRATE_IN_ACK.
type MigrationAck struct {
	MigrationID string `json:"migrationId"`
	SourcePeer  string `json:"sourcePeer"`
	DestPeer    string `json:"destPeer"`
	EntityID    int32  `json:"entityId"`
	Duplicate   bool   `json:"duplicate"`
	DeliveredAt int64  `json:"deliveredAt"`
}

// MigrationNack is MIGRATION_NACK (contract-b-m2.md §5.8). It is never sent
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

// Ping and Pong carry a nonce (contract-b-m2.md §5.9).
type Ping struct {
	Nonce string `json:"nonce"`
}

type Pong struct {
	Nonce string `json:"nonce"`
}

// Routing is the only part of data the relay decodes (contract-b-m2.md §4).
// It never touches body.bb8.
type Routing struct {
	SourcePeer string `json:"sourcePeer"`
	DestPeer   string `json:"destPeer"`
	DestSector string `json:"destSector"`
}

// DecodeData decodes a frame body into v.
func DecodeData(raw json.RawMessage, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return invalid("%v", err)
	}
	return nil
}
