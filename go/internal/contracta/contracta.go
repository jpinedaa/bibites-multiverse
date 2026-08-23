// Package contracta holds the Contract A message types, enums, close codes and
// NACK taxonomy exactly as contracts/contract-a.md specifies them.
//
// Required scalar fields are pointers so the decoder can tell "absent" from
// "zero". contract-a.md §9.3 makes that distinction load-bearing: a missing
// envelope field closes the connection, a missing data field is a NACK.
package contracta

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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
	// CloseExportEdgesUnusable is EXPORT_EDGES_UNUSABLE (added — §21, A50): NO
	// EDGE OF THE DECLARED exportEdges LIES ON AN AXIS THIS MAP HAS, on a map
	// that has at least one axis. It is a configuration error on THIS machine —
	// not a map state, not a peer's fault — and the mod MUST NOT reconnect
	// automatically, because reconnecting would re-read the same environment
	// variable and reach the same answer.
	//
	// The reason string names the declared set and the map's shape; the
	// sidecar's log names the remedy AND WHO MUST ACT, which is the one thing a
	// close code cannot say.
	CloseExportEdgesUnusable = 4007
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

// CanonicalEdges is §4.3.2's tie order, E N W S, and it is the order every
// four-edge iteration in this system uses so a log line, a grant and an
// EDGE_STATUS list read the same way. Under D17 (§18, A38) a conformant mod
// declares all four, and the two that came last — W and S — are the two the
// map learned in contract-b/3.3.
func CanonicalEdges() []string { return []string{EdgeE, EdgeN, EdgeW, EdgeS} }

// Reverse reports whether an edge's walk runs with the step NEGATED. West and
// south are east and north backwards, and that is the whole of D17's routing
// change (contract-b-m4.md §17, B13).
func Reverse(e string) bool { return e == EdgeW || e == EdgeS }

// Vertical reports whether an edge runs along the COLUMN rather than the row.
func Vertical(e string) bool { return e == EdgeN || e == EdgeS }

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
	DefaultPort         = 8787
	HeartbeatIntervalMs = 1000
	// HeartbeatTimeoutMs was 3500 — three missed heartbeats plus slack — and the
	// living deployment turned that into a routine disconnect (§20, A45). The
	// heartbeat is composed on the Unity main thread (§11.2, §13 A4) and a
	// periodic world save blocks that thread, so a long save IS heartbeat
	// silence: 50.8% of saves over 3.5 s were followed by a 4004 within twelve
	// seconds, and thirteen hours at x100 measured per-slot save maxima of
	// 4 311 / 5 386 / 5 870 / 9 115 / 9 427 ms. 13 000 clears the worst of those
	// by 3 573 ms, stays strictly under WSPingIntervalMs so the app-level
	// detector always fires before the transport one, and keeps §8's own
	// arithmetic: twelve missed heartbeats plus a second of slack.
	//
	// The cost is paid by a mod that is genuinely gone: it now keeps
	// modConnected true for up to this long plus one monitor tick
	// (HeartbeatTimeout/4), and the quiet-mod gate holds its arrivals in the
	// journal for that window instead of closing the edges. Holding is lossless
	// and bounded by InboundQueueMax; the 4004 it replaces costs a session
	// churn, an edge-close broadcast and a replay (§8, §15 A29).
	HeartbeatTimeoutMs       = 13000
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

	// ContractAPath moved with the major (§15, A23). The sidecar MUST keep
	// serving RetiredContractAPath and MUST close every connection on it
	// immediately with 4000, so an M3 mod gets the defined loud error instead of
	// a bare HTTP 404.
	// ContractAPath DOES NOT MOVE for contract-a/2.4 (§21, A52): the path is
	// major-scoped, and A47's bearer token is a transport precondition below the
	// envelope — one request header, one HTTP status on a request that never
	// became a session. Contract B took a major in the same wave because it
	// REPLACED a rule with an installed base; this one ADDS a check where there
	// was none.
	ContractAPath        = "/contract-a/v2"
	RetiredContractAPath = "/contract-a/v1"

	// AuthFailuresBeforeCeiling is `authFailuresBeforeCeiling` (§10, added — §21,
	// A47): consecutive HTTP 401s on the upgrade after which the reconnect ladder
	// holds at reconnectBackoffMaxMs and the client logs ONCE, naming the remedy
	// and who must act. It exists so a misconfigured install is a quiet,
	// diagnosable loop rather than a redial storm.
	//
	// The MOD owns the ladder; the sidecar owns the count in its own log, so a
	// person reading one end can tell a wrong token from a dead process.
	AuthFailuresBeforeCeiling = 5

	// The delivery rate limit (§7.5, §15 A20, RAISED by §18 A40). It paces
	// MIGRATE_IN out of the journal in SIMULATED minutes of the receiving world,
	// so a dam released at wake becomes a trickle the world can absorb.
	//
	// A20 shipped 2.0 and 5, sized at five times T1's measured one-lane rate of
	// 0.4 arrivals per simulated minute. D13 gave every slot a second inbound
	// lane and D17 gives it four, and 35 hours of the living deployment measured
	// the consequence: a median offered load of 1.19 per simulated minute, 12% of
	// all slot-samples ABOVE the 2.0 limit, and a paced backlog at inboundQueueMax
	// on three of six slots that never fell. §7.5's own observability rule — a
	// depth that never falls names a limit set too low — is what condemned it.
	//
	// A40 then set 12.0, five times a PROJECTED two-way median of ~2.4. The
	// projection was made before two-way lanes ran. They now run, and the living
	// deployment still carries a residual paced backlog under 12.0 (slot 3 held a
	// pacedDepth of 16 after the two-way rollout), so the same observability rule
	// condemns 12.0 for the same reason it condemned 2.0. The owner raised the
	// default to 100.0 on 2026-08-07.
	//
	// 100.0 gives up on sizing the limit from a projected median and sizes it from
	// the ceiling instead: pacing exists to stop a dam arriving in one breath, and
	// the hard mod-side ceiling is A29's ingest budget of 4 applications per
	// FixedUpdate, ~12 000 per simulated minute. 100.0 sits two orders below that,
	// so it still spreads a dam — a 900-organism backlog takes 9 simulated minutes
	// rather than one instant — while no longer throttling steady traffic.
	// inboundQueueMax and the OVERLOADED backpressure behind it are untouched.
	//
	// The rate is a DEFAULT and no longer only a constant: --inbound-rate and
	// MULTIVERSE_INBOUND_RATE override it (§18, A40), because a tunable an
	// operator cannot retune from the metric that measures it is not a tunable,
	// and this one has now needed retuning three times.
	InboundRatePerSimMinute = 100.0
	// InboundRateBurst is the token-bucket capacity, raised from 15 with the rate:
	// the bucket exists so a CLUMP is never delayed, and it bounds the largest
	// clump the pacer can ever release at once. 50 scales with the rate but stays
	// under inboundQueueMax (64), so the bucket can never release a full paced
	// queue in one breath, and its worst case costs ~13 FixedUpdates of A29's
	// 4-applications-per-frame ingest budget — about a quarter of a real second,
	// two orders below the ~12 000/simulated-minute mod-side ceiling.
	InboundRateBurst  = 50.0
	PacingIdleGraceMs = 10000
	PacingIdleGrace   = PacingIdleGraceMs * time.Millisecond
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
	// inbound organism through — ["E","N","W","S"] under the grid (§15, A18).
	BorderEdges []string `json:"borderEdges"`
	// ExportEdges are the edges this sim runs a capture band on, REQUIRED from
	// contract-a/2.0 (§15, A18). It declares GEOMETRY — "I run a capture band on
	// these edges" — never topology. The singular exportEdge is REMOVED, with no
	// fallback: an M3 mod cannot reach field validation because its protocol
	// major is rejected first (A23).
	ExportEdges []string `json:"exportEdges"`
	BorderWidth *float64 `json:"borderWidth"`
	// RingSlot is the mod's configured ring slot (§14, A14). Advisory; a
	// disagreement with the slot the sidecar holds closes with 4001.
	RingSlot  *int   `json:"ringSlot,omitempty"`
	WorldName string `json:"worldName,omitempty"`

	// THE FIVE SETTINGS OF §19, A42, AND THEY ARE RAW ON PURPOSE.
	//
	// Every other field on this message is typed, because a bad value there is a
	// close 4003 and the type system is how that is enforced. These five are the
	// third named exception to §9.3: a malformed one is STRIPPED and the
	// handshake proceeds, because a field nobody decides anything on must never
	// be able to end a session that is carrying organisms.
	//
	// json.RawMessage is how that rule is made STRUCTURALLY true rather than
	// remembered: a raw field cannot fail the enclosing decode whatever a mod
	// puts in it, so there is no path on which `"saveMinutes": "soon"` reaches
	// Validate at all. Settings() is the one place they are parsed, checked and
	// dropped, and it returns reasons instead of an error for the same reason.
	MigrationExclude json.RawMessage `json:"migrationExclude,omitempty"`
	SaveMinutes      json.RawMessage `json:"saveMinutes,omitempty"`
	SaveKeep         json.RawMessage `json:"saveKeep,omitempty"`
	SaveOnQuit       json.RawMessage `json:"saveOnQuit,omitempty"`
	WorldWrapping    json.RawMessage `json:"worldWrapping,omitempty"`
}

// Settings is the parsed form of §19 A42's five fields: what a world was told
// to do, as opposed to every other fact this system carries, which is what a
// world is doing.
//
// EVERY FIELD IS A POINTER AND nil IS UNKNOWN, with two readings that are
// emphatically not gaps: SaveMinutes of 0 is a save timer that is OFF, and a
// non-nil MigrationExclude with no entries is an exclusion policy that is OFF.
// A reader that folds either into absence loses the one fact that explains an
// absent `lastSave` or a quiet lane (§19, A42; contract-b-m4.md §19, B18).
//
// NOTHING IN THIS SYSTEM ACTS ON ANY OF THEM. They take no part in validation,
// admission control, pacing, custody, dedup, edge state, the S check or
// liveness, and there is no path by which one travels back toward a mod: they
// are a REPORT, and a control surface is a separate design rather than an
// extension of these (§19, A43).
type Settings struct {
	MigrationExclude *wire.ExcludeList
	SaveMinutes      *float64
	SaveKeep         *int
	SaveOnQuit       *bool
	WorldWrapping    *bool
}

// Settings parses the five settings fields, STRIPS WHAT FAILS AND KEEPS THE
// REST, and returns one line per rule that fired. It never returns an error and
// there is deliberately no way for it to: see the field comments above.
//
// A bad ENTRY costs that entry; a `migrationExclude` that is not an array costs
// the field; a `saveMinutes` that is not a number costs that number. None of
// them costs the handshake, and the handshake is the whole session (§19, A42).
func (c *ConfigUpdate) Settings() (Settings, []string) {
	var out Settings
	var why []string

	if len(c.MigrationExclude) > 0 && !isJSONNull(c.MigrationExclude) {
		var list wire.ExcludeList
		// ExcludeList.UnmarshalJSON is permissive and records rather than
		// returns; CarryExclude is what reads what it recorded.
		_ = json.Unmarshal(c.MigrationExclude, &list)
		carried, reasons := wire.CarryExclude(&list)
		out.MigrationExclude = carried
		why = append(why, reasons...)
	}
	if v, reason := lenientFloat("saveMinutes", c.SaveMinutes); reason != "" {
		why = append(why, reason)
	} else if v != nil {
		// A NEGATIVE interval is not a reading of anything — 0 already means
		// "off" — so it is stripped rather than published as a duration nobody
		// could act on.
		if *v < 0 {
			why = append(why, "saveMinutes is negative")
		} else {
			out.SaveMinutes = v
		}
	}
	if v, reason := lenientFloat("saveKeep", c.SaveKeep); reason != "" {
		why = append(why, reason)
	} else if v != nil {
		if *v != math.Trunc(*v) || *v < 0 {
			why = append(why, "saveKeep is not a non-negative whole number")
		} else {
			n := int(*v)
			out.SaveKeep = &n
		}
	}
	if v, reason := lenientBool("saveOnQuit", c.SaveOnQuit); reason != "" {
		why = append(why, reason)
	} else {
		out.SaveOnQuit = v
	}
	if v, reason := lenientBool("worldWrapping", c.WorldWrapping); reason != "" {
		why = append(why, reason)
	} else {
		out.WorldWrapping = v
	}
	return out, why
}

// SetSettings fills the five raw fields from a parsed Settings. It is the
// SENDING half — the fake mod and the tests use it — and it is the only place
// in Go that writes these fields, because the real sender is a C# plugin.
func (c *ConfigUpdate) SetSettings(s Settings) {
	c.MigrationExclude, c.SaveMinutes = nil, nil
	c.SaveKeep, c.SaveOnQuit, c.WorldWrapping = nil, nil, nil
	if s.MigrationExclude != nil {
		b, err := json.Marshal(s.MigrationExclude)
		if err == nil {
			c.MigrationExclude = b
		}
	}
	if s.SaveMinutes != nil {
		c.SaveMinutes = mustRaw(*s.SaveMinutes)
	}
	if s.SaveKeep != nil {
		c.SaveKeep = mustRaw(*s.SaveKeep)
	}
	if s.SaveOnQuit != nil {
		c.SaveOnQuit = mustRaw(*s.SaveOnQuit)
	}
	if s.WorldWrapping != nil {
		c.WorldWrapping = mustRaw(*s.WorldWrapping)
	}
}

func mustRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

func isJSONNull(b json.RawMessage) bool { return string(bytes.TrimSpace(b)) == "null" }

func lenientFloat(field string, raw json.RawMessage) (*float64, string) {
	if len(raw) == 0 || isJSONNull(raw) {
		return nil, ""
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, field + " is not a number"
	}
	if !wire.Finite(v) {
		return nil, field + " is not a finite number"
	}
	return &v, ""
}

func lenientBool(field string, raw json.RawMessage) (*bool, string) {
	if len(raw) == 0 || isJSONNull(raw) {
		return nil, ""
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, field + " is not a boolean"
	}
	return &v, ""
}

// ResolveExportEdges validates CONFIG_UPDATE.exportEdges under §15, A18: an
// array, REQUIRED, at least one member, no duplicates, every member also in
// borderEdges.
//
// A18 removed the singular exportEdge and its single-entry borderEdges
// fallback deliberately: "a compatibility path that only an already-rejected
// peer can take is dead code that reads like a supported configuration".
func (c *ConfigUpdate) ResolveExportEdges() ([]string, error) {
	if len(c.ExportEdges) == 0 {
		return nil, invalid("exportEdges is missing or empty; contract-a/2.0 REQUIRES at least one member")
	}
	seen := map[string]bool{}
	border := map[string]bool{}
	for _, e := range c.BorderEdges {
		border[e] = true
	}
	out := make([]string, 0, len(c.ExportEdges))
	for _, e := range c.ExportEdges {
		if !ValidEdge(e) {
			return nil, invalid("exportEdges contains %q, which is not N/S/E/W", e)
		}
		if seen[e] {
			return nil, invalid("exportEdges repeats %q", e)
		}
		seen[e] = true
		if !border[e] {
			return nil, invalid("exportEdges member %q is not in borderEdges %v", e, c.BorderEdges)
		}
		out = append(out, e)
	}
	return out, nil
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
	if _, err := c.ResolveExportEdges(); err != nil {
		return err
	}
	return nil
}

// SaveReceipt is HEARTBEAT.lastSave (§5.2, §15 A21). It is a receipt, not a
// request: no component asks for a save, schedules one, or reacts to a missing
// one, and no correctness decision reads atMs (D5). The sidecar copies it into
// its peer stats verbatim and never computes it.
type SaveReceipt struct {
	AtMs          int64   `json:"atMs"`
	SimulatedTime float64 `json:"simulatedTime"`
	Population    int     `json:"population"`
	Name          string  `json:"name,omitempty"`
	Bytes         int64   `json:"bytes,omitempty"`
	DurationMs    int     `json:"durationMs,omitempty"`
}

// Heartbeat is HEARTBEAT (contract-a.md §5.2).
type Heartbeat struct {
	SessionID     string   `json:"sessionId"`
	SimTick       *int64   `json:"simTick"`
	SimulatedTime *float64 `json:"simulatedTime"`
	Population    *int     `json:"population"`
	EggCount      *int     `json:"eggCount,omitempty"`
	Paused        *bool    `json:"paused"`
	TimeScale     *float64 `json:"timeScale"`
	// TargetTimeScale is what the game's speed control is asking for. It is
	// optional for compatibility with older mods and distinct from TimeScale,
	// which is the applied value after the game's minimum-FPS governor.
	TargetTimeScale *float64 `json:"targetTimeScale,omitempty"`
	SimulationSize  *float64 `json:"simulationSize"`
	InFlightOut     *int     `json:"inFlightOut,omitempty"`
	PendingIn       *int     `json:"pendingIn,omitempty"`
	// LastSave is OPTIONAL (§15, A21). A mod that omits it is conformant and the
	// status page shows that world's save state as unknown.
	LastSave *SaveReceipt `json:"lastSave,omitempty"`
	// Species is the world's active species census (§17, A35). OPTIONAL, and
	// ABSENT MEANS UNKNOWN rather than empty: an old mod and a mod that does not
	// implement §17 both produce a frame with no field, and no reader can tell
	// them apart from a world with no species. A present `[]` is the stronger
	// statement a reporting mod makes about a world with nothing alive in it.
	//
	// It is deliberately NOT part of Validate: see the note there.
	Species *Census `json:"species,omitempty"`
	// Truncated qualifies Species and nothing else. A receiver MUST ignore it
	// when Species is absent (§17, A35).
	Truncated TruncatedFlag `json:"truncated,omitempty"`
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
	if h.TargetTimeScale != nil && (!wire.Finite(*h.TargetTimeScale) || *h.TargetTimeScale < 0) {
		return invalid("targetTimeScale is negative or not finite")
	}
	if h.SimulationSize == nil || !wire.Finite(*h.SimulationSize) || *h.SimulationSize <= 0 {
		return invalid("simulationSize is missing or not a positive finite number")
	}
	// THE CENSUS IS NOT CHECKED HERE, AND THAT IS THE POINT. Every other field
	// in this method answers a failure with close 4003, because HEARTBEAT has NO
	// NACK CHANNEL AT ALL — and §17 A35 makes the census the named exception to
	// exactly that default: "a label is not worth a session". The sidecar
	// applies wire.CarryCensus at the receiver obligation of §5.2, which strips
	// what fails and processes the heartbeat as before. Putting the same check
	// here would let a display field kill a live session.
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
	// BlobDroppedForSize says THIS PARENT HAD A BLOB AND THE MOD DROPPED IT TO
	// FIT THE FRAME (added — §21, A49). It is the field that finally makes
	// contract-b-m4.md §6.6's "blob_dropped_for_size" reachable, after two
	// milestones in which it was defined and never emitted: a dropped blob and a
	// dead parent both arrive as an entityId with no payload, and until now they
	// were byte for byte the same thing.
	//
	// It may be true ONLY on an entry with no Payload, and ONLY for a drop under
	// §5.3's frame-size rule — never for a dead parent, never for a serialization
	// failure. "I had it and could not fit it" is a different fact from "I could
	// not produce it", and a mod that conflated them would put a recoverable
	// label on a permanent absence.
	//
	// It is a PLAIN BOOL and not a pointer, which is a departure from this
	// package's absence-is-a-value habit and is deliberate. §21 A49 says absence
	// is "no statement", and no statement and false decide IDENTICALLY: both
	// record "parent_gone". A pointer here would model a distinction that no
	// reader is allowed to act on.
	BlobDroppedForSize bool `json:"blobDroppedForSize,omitempty"`
}

// Species is the OPTIONAL species-identity block of §16, A30 — the same shape,
// the same rules and the same validator on both wires, so it lives in package
// wire beside the envelope they also share.
type Species = wire.Species

// The species CENSUS of §17, A35 — a different lane with a different job and a
// DIFFERENT NAME RULE from the block above (§17, A36). It rides HEARTBEAT and
// then the peer stats block, so it lives in package wire too.
type (
	Census        = wire.Census
	CensusEntry   = wire.CensusEntry
	TruncatedFlag = wire.TruncatedFlag
)

// SpeciesCensusMax is §10's bound on HEARTBEAT.species, restated where the rest
// of Contract A's tunables live.
const SpeciesCensusMax = wire.SpeciesCensusMax

// MigrateOut is MIGRATE_OUT (contract-a.md §5.3).
type MigrateOut struct {
	MigrationID string `json:"migrationId"`
	EntityID    *int32 `json:"entityId"`
	Kind        string `json:"kind"`
	GameVersion string `json:"gameVersion"`
	Payload     string `json:"payload"`
	// Parents are the lineage inputs, in genes.parent1 then genes.parent2
	// order (§14, A12). Nothing here may ever fail a migration.
	Parents []ParentBlob `json:"parents,omitempty"`
	// Species is the migrant's species identity, read from the live Species
	// record and never from the payload (§16, A30). OPTIONAL, and absent is
	// ordinary. It is deliberately NOT part of Validate: see the note there.
	Species        *Species `json:"species,omitempty"`
	ExitEdge       string   `json:"exitEdge"`
	ExitPosition   *float64 `json:"exitPosition"`
	Velocity       *Vec     `json:"velocity"`
	Heading        *float64 `json:"heading"`
	SimulationSize *float64 `json:"simulationSize"`
	SimTick        *int64   `json:"simTick"`
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
	// SPECIES IS NOT CHECKED HERE, AND THAT IS THE POINT. Every other field in
	// this method answers a failure with MIGRATE_OUT_NACK / MALFORMED_MESSAGE.
	// §16 A30 makes the species block the ONE NAMED EXCEPTION to that rule: a
	// malformed one is stripped, logged once, and the migration proceeds without
	// it. The sidecar applies wire.CarrySpecies at the receiver obligation of
	// §5.3 step 1; putting the same check here would turn a label into a refused
	// organism.
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
	MigrationID string `json:"migrationId"`
	EntityID    int32  `json:"entityId"`
	Kind        string `json:"kind"`
	GameVersion string `json:"gameVersion"`
	Payload     string `json:"payload"`
	// Species is the block the envelope carried, handed through VERBATIM (§16,
	// A30; contract-b-m4.md §6.6 step 7). The sidecar never authors one, never
	// resolves one, and sends none when the envelope carried none — the mod then
	// applies the absent-block rule of A32 and removes $.genes.speciesID.
	Species       *Species `json:"species,omitempty"`
	EntryEdge     string   `json:"entryEdge"`
	EntryPosition float64  `json:"entryPosition"`
	Velocity      Vec      `json:"velocity"`
	Heading       float64  `json:"heading"`
	BounceBack    bool     `json:"bounceBack"`
	Attempt       int      `json:"attempt"`
	AckDeadlineMs int      `json:"ackDeadlineMs,omitempty"`
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
