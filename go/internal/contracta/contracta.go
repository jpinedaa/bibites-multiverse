// Package contracta holds the Contract A message types, enums, close codes and
// NACK taxonomy exactly as contracts/contract-a.md specifies them.
//
// Required scalar fields are pointers so the decoder can tell "absent" from
// "zero". contract-a.md §9.3 makes that distinction load-bearing: a missing
// envelope field closes the connection, a missing data field is a NACK.
package contracta

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"multiverse/internal/wire"
)

// Message types (contract-a.md §5).
const (
	TypeConfigUpdate   = "CONFIG_UPDATE"
	TypeHeartbeat      = "HEARTBEAT"
	TypeMigrateOut     = "MIGRATE_OUT"
	TypeMigrateInAck   = "MIGRATE_IN_ACK"
	TypeMigrateInNack  = "MIGRATE_IN_NACK"
	TypeEdgeStatus     = "EDGE_STATUS"
	TypeMigrateIn      = "MIGRATE_IN"
	TypeMigrateOutAck  = "MIGRATE_OUT_ACK"
	TypeMigrateOutNack = "MIGRATE_OUT_NACK"
)

// WebSocket close codes (contract-a.md §2.1).
const (
	CloseNormal              = 1000
	CloseTooBig              = 1009
	CloseProtocolUnsupported = 4000
	// CloseSlotMismatch keeps 4001's number and behaviour. It was
	// SECTOR_MISMATCH over the retired {x,y} grid and is now read as
	// SLOT_MISMATCH (amended — §14, A14).
	CloseSlotMismatch         = 4001
	CloseGameVersionUnsupport = 4002
	CloseMalformedFrame       = 4003
	CloseHeartbeatTimeout     = 4004
	CloseShuttingDown         = 4005
	CloseReplaced             = 4006
)

// MIGRATE_OUT_NACK codes (contract-a.md §9.1).
const (
	OutEdgeClosed          = "EDGE_CLOSED"
	OutNoRoute             = "NO_ROUTE"
	OutSimSizeMismatch     = "SIM_SIZE_MISMATCH"
	OutPeerIncompatible    = "PEER_INCOMPATIBLE"
	OutKindUnsupported     = "KIND_UNSUPPORTED"
	OutInvalidPayload      = "INVALID_PAYLOAD"
	OutDuplicateMigationID = "DUPLICATE_MIGRATION_ID"
	OutRateLimited         = "RATE_LIMITED"
	OutJournalFull         = "JOURNAL_FULL"
	OutJournalError        = "JOURNAL_ERROR"
	OutMalformedMessage    = "MALFORMED_MESSAGE"
	OutShuttingDown        = "SHUTTING_DOWN"
)

// MIGRATE_IN_NACK codes (contract-a.md §9.2).
const (
	InSimNotReady        = "SIM_NOT_READY"
	InSimOverloaded      = "SIM_OVERLOADED"
	InEdgeClosed         = "EDGE_CLOSED"
	InDeserializeFailed  = "DESERIALIZE_FAILED"
	InRelinkFailed       = "RELINK_FAILED"
	InVersionUnsupported = "VERSION_UNSUPPORTED"
	InKindUnsupported    = "KIND_UNSUPPORTED"
	InMalformedMessage   = "MALFORMED_MESSAGE"
	InShuttingDown       = "SHUTTING_DOWN"
)

// NACK classes (contract-a.md §5.6).
const (
	ClassTransient = "transient"
	ClassPermanent = "permanent"
)

// PermanentOutCodes is the class lookup for §9.1. A code that is absent is
// handled as transient, which is the safe default for a receiver.
var permanentOutCodes = map[string]bool{
	OutPeerIncompatible:    true,
	OutKindUnsupported:     true,
	OutInvalidPayload:      true,
	OutDuplicateMigationID: true,
	OutMalformedMessage:    true,
}

// ClassOfOutCode returns the class contract-a.md §9.1 assigns to code.
func ClassOfOutCode(code string) string {
	if permanentOutCodes[code] {
		return ClassPermanent
	}
	return ClassTransient
}

var permanentInCodes = map[string]bool{
	InDeserializeFailed:  true,
	InRelinkFailed:       true,
	InVersionUnsupported: true,
	InKindUnsupported:    true,
	InMalformedMessage:   true,
}

// ClassOfInCode returns the class contract-a.md §9.2 assigns to code.
func ClassOfInCode(code string) string {
	if permanentInCodes[code] {
		return ClassPermanent
	}
	return ClassTransient
}

// Edge enum (contract-a.md §4.2).
const (
	EdgeN = "N"
	EdgeS = "S"
	EdgeE = "E"
	EdgeW = "W"
)

// ValidEdge reports whether e is one of the four edges.
func ValidEdge(e string) bool {
	return e == EdgeN || e == EdgeS || e == EdgeE || e == EdgeW
}

// Opposite returns the facing edge. The sidecar owns this function
// (contract-a.md §4.2).
func Opposite(e string) (string, bool) {
	switch e {
	case EdgeN:
		return EdgeS, true
	case EdgeS:
		return EdgeN, true
	case EdgeE:
		return EdgeW, true
	case EdgeW:
		return EdgeE, true
	}
	return "", false
}

// KindBibite is the only kind M2 accepts (contract-a.md §4.5).
const KindBibite = "bibite"

// EDGE_STATUS reasons (contract-a.md §5.4).
const (
	ReasonPeerLive = "peer_live"
	ReasonNoPeer   = "no_peer"
	// ReasonPeerModAbsent is new in contract-a/1.1 (§14, A11): the east
	// neighbour's sidecar is live but has no mod, so it cannot spawn anything.
	ReasonPeerModAbsent    = "peer_mod_absent"
	ReasonPeerIncompatible = "peer_incompatible"
	ReasonPeerUnreachable  = "peer_unreachable"
	ReasonPeerOverloaded   = "peer_overloaded"
	ReasonAdminClosed      = "admin_closed"
	ReasonSimSizeMismatch  = "sim_size_mismatch"
)

// Tunable defaults (contract-a.md §10).
const (
	DefaultPort              = 8787
	HeartbeatIntervalMs      = 1000
	HeartbeatTimeoutMs       = 3500
	WSPingIntervalMs         = 15000
	WSPongTimeoutMs          = 10000
	MigrateInAckTimeoutMs    = 10000
	InboundQueueMax          = 64
	ExportRetentionSeconds   = 3600
	ReconnectBackoffMinMs    = 1000
	ReconnectBackoffMaxMs    = 30000
	MigrateOutTimeoutMs      = 5000
	MigrationCooldownSeconds = 5
	EntryImmunitySeconds     = 5
	// MaxParentBlobs bounds MIGRATE_OUT.parents (§10, §14 A12). A longer array
	// is truncated by the sidecar with one warning, never a NACK.
	MaxParentBlobs      = 2
	HeartbeatTimeout    = HeartbeatTimeoutMs * time.Millisecond
	MigrateInAckTimeout = MigrateInAckTimeoutMs * time.Millisecond
	ExportRetention     = ExportRetentionSeconds * time.Second
	WSPingInterval      = WSPingIntervalMs * time.Millisecond
	WSPongTimeout       = WSPongTimeoutMs * time.Millisecond
	ContractAPath       = "/contract-a/v1"
)

// Vec is a 2D vector in the source sim's world axes (contract-a.md §4.4).
type Vec struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// The {x, y} sector of contract-a/1 is retired (§14, A14) and has no Go type
// here. D8 retires the grid: a contract-a/1.1 mod MUST NOT send `sector`, and a
// contract-a/1.1 sidecar MUST ignore it when an older mod does. Deleting the
// field is that ignore — encoding/json drops an unknown field, so an old mod's
// `sector` reaches nothing and closes nothing. `ringSlot` replaces it.

// ErrInvalid marks a data-level validation failure. It is answered with the
// matching NACK, never with a close (contract-a.md §3.2, §9.3).
var ErrInvalid = errors.New("contracta: invalid message data")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// ---------------------------------------------------------------- mod → sidecar

// ConfigUpdate is CONFIG_UPDATE (contract-a.md §5.1).
type ConfigUpdate struct {
	SessionID   string `json:"sessionId"`
	Reason      string `json:"reason"`
	GameVersion string `json:"gameVersion"`
	ModVersion  string `json:"modVersion"`
	// SimulationSize is S, the playable half-extent.
	SimulationSize *float64 `json:"simulationSize"`
	// BorderEdges are the edges the mod has a strip on and will accept an
	// inbound organism through — ["E","W"] under the ring (§14, A11).
	BorderEdges []string `json:"borderEdges"`
	// ExportEdge is the one edge this sim exports through, REQUIRED from
	// contract-a/1.1 (§14, A11). Absent is not a close: see ResolveExportEdge.
	ExportEdge  string   `json:"exportEdge,omitempty"`
	BorderWidth *float64 `json:"borderWidth"`
	// RingSlot is the mod's configured ring slot (§14, A14). Advisory; a
	// disagreement with the slot the sidecar holds closes with 4001.
	RingSlot  *int   `json:"ringSlot,omitempty"`
	WorldName string `json:"worldName,omitempty"`
}

// ResolveExportEdge implements the A16-compatible fallback of §14, A11: an
// absent exportEdge is supplied by a single-entry borderEdges, and only an
// ambiguous borderEdges is unusable. That fallback is what keeps the new field
// additive under §3.1's minor rule instead of a breaking change.
func (c *ConfigUpdate) ResolveExportEdge() (string, error) {
	if c.ExportEdge != "" {
		if !ValidEdge(c.ExportEdge) {
			return "", invalid("exportEdge %q is not N/S/E/W", c.ExportEdge)
		}
		for _, e := range c.BorderEdges {
			if e == c.ExportEdge {
				return c.ExportEdge, nil
			}
		}
		return "", invalid("exportEdge %q is not a member of borderEdges %v", c.ExportEdge, c.BorderEdges)
	}
	if len(c.BorderEdges) == 1 {
		return c.BorderEdges[0], nil
	}
	return "", invalid("no exportEdge and borderEdges %v does not have exactly one entry", c.BorderEdges)
}

func (c *ConfigUpdate) Validate() error {
	if !wire.ValidUUID(c.SessionID) {
		return invalid("sessionId %q is not a uuid", c.SessionID)
	}
	switch c.Reason {
	case "connect", "world_loaded", "settings_changed", "sim_size_changed":
	default:
		return invalid("reason %q is not a known value", c.Reason)
	}
	if c.GameVersion == "" {
		return invalid("gameVersion is empty")
	}
	if c.ModVersion == "" {
		return invalid("modVersion is empty")
	}
	if c.SimulationSize == nil {
		return invalid("simulationSize is missing")
	}
	if !wire.Finite(*c.SimulationSize) || *c.SimulationSize <= 0 {
		return invalid("simulationSize %v is not a positive finite number", *c.SimulationSize)
	}
	if c.BorderEdges == nil {
		return invalid("borderEdges is missing")
	}
	seen := map[string]bool{}
	for _, e := range c.BorderEdges {
		if !ValidEdge(e) {
			return invalid("borderEdges contains %q", e)
		}
		if seen[e] {
			return invalid("borderEdges repeats %q", e)
		}
		seen[e] = true
	}
	if c.BorderWidth == nil {
		return invalid("borderWidth is missing")
	}
	if !wire.Finite(*c.BorderWidth) || *c.BorderWidth < 0 {
		return invalid("borderWidth %v is not a non-negative finite number", *c.BorderWidth)
	}
	if c.RingSlot != nil && *c.RingSlot < 1 {
		return invalid("ringSlot %d is not a ring slot", *c.RingSlot)
	}
	if _, err := c.ResolveExportEdge(); err != nil {
		return err
	}
	return nil
}

// Heartbeat is HEARTBEAT (contract-a.md §5.2).
type Heartbeat struct {
	SessionID      string   `json:"sessionId"`
	SimTick        *int64   `json:"simTick"`
	SimulatedTime  *float64 `json:"simulatedTime"`
	Population     *int     `json:"population"`
	EggCount       *int     `json:"eggCount,omitempty"`
	Paused         *bool    `json:"paused"`
	TimeScale      *float64 `json:"timeScale"`
	SimulationSize *float64 `json:"simulationSize"`
	InFlightOut    *int     `json:"inFlightOut,omitempty"`
	PendingIn      *int     `json:"pendingIn,omitempty"`
}

func (h *Heartbeat) Validate() error {
	if !wire.ValidUUID(h.SessionID) {
		return invalid("sessionId %q is not a uuid", h.SessionID)
	}
	if h.SimTick == nil {
		return invalid("simTick is missing")
	}
	if h.SimulatedTime == nil || !wire.Finite(*h.SimulatedTime) {
		return invalid("simulatedTime is missing or not finite")
	}
	if h.Population == nil || *h.Population < 0 {
		return invalid("population is missing or negative")
	}
	if h.Paused == nil {
		return invalid("paused is missing")
	}
	if h.TimeScale == nil || !wire.Finite(*h.TimeScale) {
		return invalid("timeScale is missing or not finite")
	}
	if h.SimulationSize == nil || !wire.Finite(*h.SimulationSize) || *h.SimulationSize <= 0 {
		return invalid("simulationSize is missing or not a positive finite number")
	}
	return nil
}

// ParentBlob is one entry of MIGRATE_OUT.parents (contract-a.md §14, A12).
//
// The mod ships; the sidecar hashes. Payload is opaque to the mod — D4 forbids
// it to parse a genome — and the sidecar hashes it, caches it under that hash
// and strips it before the envelope goes on Contract B. An absent Payload means
// the parent is gone, which is the normal case and never an error.
type ParentBlob struct {
	EntityID    *int32 `json:"entityId"`
	Payload     string `json:"payload,omitempty"`
	GameVersion string `json:"gameVersion,omitempty"`
}

// MigrateOut is MIGRATE_OUT (contract-a.md §5.3).
type MigrateOut struct {
	MigrationID string `json:"migrationId"`
	EntityID    *int32 `json:"entityId"`
	Kind        string `json:"kind"`
	GameVersion string `json:"gameVersion"`
	Payload     string `json:"payload"`
	// Parents are the lineage inputs, in genes.parent1 then genes.parent2
	// order (§14, A12). Nothing here may ever fail a migration.
	Parents        []ParentBlob `json:"parents,omitempty"`
	ExitEdge       string       `json:"exitEdge"`
	ExitPosition   *float64     `json:"exitPosition"`
	Velocity       *Vec         `json:"velocity"`
	Heading        *float64     `json:"heading"`
	SimulationSize *float64     `json:"simulationSize"`
	SimTick        *int64       `json:"simTick"`
}

func (m *MigrateOut) Validate() error {
	if !wire.ValidUUID(m.MigrationID) {
		return invalid("migrationId %q is not a uuid", m.MigrationID)
	}
	if m.EntityID == nil {
		return invalid("entityId is missing")
	}
	if *m.EntityID == 0 {
		// 0 is the game's "unassigned" sentinel and never appears
		// (contract-a.md §4.1).
		return invalid("entityId is 0, the unassigned sentinel")
	}
	if m.Kind == "" {
		return invalid("kind is missing")
	}
	if m.GameVersion == "" {
		return invalid("gameVersion is missing")
	}
	if m.Payload == "" {
		return invalid("payload is empty")
	}
	if !ValidEdge(m.ExitEdge) {
		return invalid("exitEdge %q is not N/S/E/W", m.ExitEdge)
	}
	if m.ExitPosition == nil {
		return invalid("exitPosition is missing")
	}
	// A valid sender always clamps, so a receiver rejects out of range
	// (contract-a.md §4.3).
	if !wire.Finite(*m.ExitPosition) || *m.ExitPosition < 0 || *m.ExitPosition > 1 {
		return invalid("exitPosition %v is outside [0,1]", *m.ExitPosition)
	}
	if m.Velocity == nil {
		return invalid("velocity is missing")
	}
	if !wire.Finite(m.Velocity.X) || !wire.Finite(m.Velocity.Y) {
		return invalid("velocity is not finite")
	}
	if m.Heading == nil || !wire.Finite(*m.Heading) {
		return invalid("heading is missing or not finite")
	}
	if m.SimulationSize == nil || !wire.Finite(*m.SimulationSize) || *m.SimulationSize <= 0 {
		return invalid("simulationSize is missing or not a positive finite number")
	}
	if m.SimTick == nil {
		return invalid("simTick is missing")
	}
	return nil
}

// MigrateInAck is MIGRATE_IN_ACK (contract-a.md §5.8).
type MigrateInAck struct {
	MigrationID      string `json:"migrationId"`
	EntityID         *int32 `json:"entityId"`
	Duplicate        *bool  `json:"duplicate"`
	SimTick          *int64 `json:"simTick"`
	RelinkedParents  *int   `json:"relinkedParents,omitempty"`
	RelinkedChildren *int   `json:"relinkedChildren,omitempty"`
}

func (m *MigrateInAck) Validate() error {
	if !wire.ValidUUID(m.MigrationID) {
		return invalid("migrationId %q is not a uuid", m.MigrationID)
	}
	if m.EntityID == nil {
		return invalid("entityId is missing")
	}
	if m.Duplicate == nil {
		return invalid("duplicate is missing")
	}
	if m.SimTick == nil {
		return invalid("simTick is missing")
	}
	return nil
}

// MigrateInNack is MIGRATE_IN_NACK (contract-a.md §5.9).
type MigrateInNack struct {
	MigrationID  string `json:"migrationId"`
	EntityID     *int32 `json:"entityId"`
	Code         string `json:"code"`
	Class        string `json:"class"`
	Message      string `json:"message"`
	RetryAfterMs *int   `json:"retryAfterMs,omitempty"`
}

func (m *MigrateInNack) Validate() error {
	if !wire.ValidUUID(m.MigrationID) {
		return invalid("migrationId %q is not a uuid", m.MigrationID)
	}
	if m.EntityID == nil {
		return invalid("entityId is missing")
	}
	if m.Code == "" {
		return invalid("code is missing")
	}
	if m.Class != ClassTransient && m.Class != ClassPermanent {
		return invalid("class %q is not transient/permanent", m.Class)
	}
	return nil
}

// ---------------------------------------------------------------- sidecar → mod

// EdgeState is one entry of EDGE_STATUS.edges (contract-a.md §5.4).
type EdgeState struct {
	Edge               string   `json:"edge"`
	Open               bool     `json:"open"`
	Reason             string   `json:"reason"`
	PeerSimulationSize *float64 `json:"peerSimulationSize,omitempty"`
}

// EdgeStatus is EDGE_STATUS (contract-a.md §5.4). Full state, not a delta.
type EdgeStatus struct {
	Epoch int64       `json:"epoch"`
	Edges []EdgeState `json:"edges"`
}

// MigrateIn is MIGRATE_IN (contract-a.md §5.7).
type MigrateIn struct {
	MigrationID   string  `json:"migrationId"`
	EntityID      int32   `json:"entityId"`
	Kind          string  `json:"kind"`
	GameVersion   string  `json:"gameVersion"`
	Payload       string  `json:"payload"`
	EntryEdge     string  `json:"entryEdge"`
	EntryPosition float64 `json:"entryPosition"`
	Velocity      Vec     `json:"velocity"`
	Heading       float64 `json:"heading"`
	BounceBack    bool    `json:"bounceBack"`
	Attempt       int     `json:"attempt"`
	AckDeadlineMs int     `json:"ackDeadlineMs,omitempty"`
}

// MigrateOutAck is MIGRATE_OUT_ACK (contract-a.md §5.5). It is a custody
// assertion, solicited or not (§7.4).
type MigrateOutAck struct {
	MigrationID string `json:"migrationId"`
	EntityID    int32  `json:"entityId"`
	JournaledAt int64  `json:"journaledAt"`
	Unsolicited bool   `json:"unsolicited,omitempty"`
}

// MigrateOutNack is MIGRATE_OUT_NACK (contract-a.md §5.6).
type MigrateOutNack struct {
	MigrationID  string `json:"migrationId"`
	EntityID     int32  `json:"entityId"`
	Code         string `json:"code"`
	Class        string `json:"class"`
	Message      string `json:"message"`
	RetryAfterMs *int   `json:"retryAfterMs,omitempty"`
}

// DecodeData decodes a frame body into v. A decode failure is an ErrInvalid,
// not a malformed frame: the envelope already parsed, so contract-a.md §3.2
// keeps the connection open and answers with a NACK.
func DecodeData(raw json.RawMessage, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return invalid("%v", err)
	}
	return nil
}
