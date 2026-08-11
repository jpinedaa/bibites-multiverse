// Package contractb holds Contract B exactly as contracts/contract-b-m4.md
// specifies it (`contract-b/4.0`): sidecar -> relay -> sidecar, a rectangular
// map of slots, a read-only AUTHORISED archive subscriber, a JSON envelope, and
// an opaque bb8 body.
//
// contract-b/3 was a major bump from M3's contract-b/2: the ring became a grid,
// so a reservation gained a coordinate; SECTOR_GRANT returns one effective
// neighbour per export edge instead of one east neighbour; SECTOR_CLAIM carries
// an array of export edges; ringSize became slotCount; MIGRATION_NACK gained the
// non-delivery proof and two new codes, and SLOT_VACANT changed class from
// transient to permanent.
//
// contract-b/4 is the second major, and ONE ROW OF ONE TABLE is why (§22, B32).
// §3.1's shared token is REPLACED by a per-peer credential bound to the peerId
// (internal/peercred), §3.2's 4006 narrows to require it, and the transport
// becomes TLS. A changed rule with an installed base is the expensive kind.
// Everything else in §22 passes the additive test, and the message catalogue,
// the envelope, custody, dedup, the hold, the fan-out, hashing and the routing
// inputs are untouched.
//
// A contract-b/3 sidecar and a contract-b/4 relay are incompatible BY DESIGN and
// say so with close 4000 on a retired path rather than misrouting an organism
// (§4). There is no field-level fallback anywhere: the RULE changed and not the
// shape, so a compatibility path only an already-rejected peer could take would
// be dead code that reads like a supported configuration.
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
	// CloseReplaced now fires ONLY for a connection that AUTHENTICATED AS the
	// same peerId (amended — §22, B22). The credential is what makes the rule
	// safe: under the shared token any holder could take any peer's slot with one
	// frame, and under §3.1 only the peer itself can replace itself. The
	// self-healing rule contract-a.md §2 gives the mod socket survives; the
	// impersonation it permitted does not.
	CloseReplaced = 4006
)

// The three refusal axes of §6.5's `lastRefusal` (amended — §22, B24, B25). The
// string names WHICH axis fired, because "this peer was refused" without the
// axis is a support conversation rather than a remedy.
//
// A CREDENTIAL FAILURE NEVER APPEARS HERE (B22). A refused credential reaches no
// slot, and writing it on the slot whose id was borrowed would tell an innocent
// peer it had been attacked and hand an attacker a confirmation surface.
const (
	// RefusalGameVersion is §6.1's kept game-version gate — the fourth of the
	// four exceptions B31 names to the game version's diagnostic-only rule.
	RefusalGameVersion = "game_version_incompatible"
	// RefusalContractVersion is B25's floor. The string carries BOTH versions so
	// the remedy is legible without a second lookup.
	RefusalContractVersion = "contract_version_below_minimum"
	// RefusalCapacity is B24's, and B24 is WP4: nothing writes it yet.
	RefusalCapacity = "capacity"
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
	// DefaultRelayPort moved from 8790 to 8795 for the M4 port plan
	// (contract-b-m4.md §3, §12). contract-a.md §10 gives the six-slot rig the
	// Contract A range 8787-8792, and 8790 is slot 4's port: the relay's own
	// default sat inside the range it has to avoid. 8795 and the archive's 8796
	// are the two ports outside it.
	DefaultRelayPort = 8795
	// ContractBPath moved with the major again, to /contract-b/v4
	// (amended — §22, B32). The relay MUST keep serving EVERY retired path —
	// RetiredContractBPaths below — and MUST close every connection on one
	// immediately with 4000, so an older sidecar gets the defined loud error
	// instead of a bare HTTP 404. A retired path is served over TLS like the live
	// one (B23): a peer that cannot complete a handshake learns nothing, and the
	// point of a retired path is that it teaches.
	ContractBPath = "/contract-b/v4"

	// RelayTLSMinVersion is `relayTLSMinVersion` (§12, added — §22, B23): the
	// lowest TLS version the relay's OWN listener accepts. A relay behind a
	// fronting proxy does not own this; the proxy does, and B23 says the
	// wire-visible behaviour is what the contract specifies either way.
	RelayTLSMinVersion = "1.2"

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

	// The four bounds of §21, B21. genomeRequestsPerMinute above is a RATE and
	// says nothing about how much work one pass of the backlog may do, nor how
	// many requests it may put on the wire at once. On a large backlog that is
	// the difference between a paced fetcher and a requester that starves its
	// own read loop.
	//
	// GenomeScanPerTick bounds how many pending entries one pump examines. The
	// walk is round-robin and resumes where it stopped, so the whole backlog is
	// still visited; it is visited over several ticks instead of one.
	GenomeScanPerTick = 2048
	// GenomeScanChunk bounds how many entries are examined under one acquisition
	// of the requester's lock. It is the yield: the read loop can never wait
	// longer than one chunk, whatever the backlog.
	GenomeScanChunk = 256
	// GenomeRequestsPerTick bounds the BURST — how many GENOME_REQUESTs one pump
	// may put on the wire. It sits far above what genomeRequestsPerMinute allows
	// to be sustained, so it never lowers the fetch rate; it only stops a minute's
	// whole allowance leaving in one pass.
	GenomeRequestsPerTick = 8
	// GenomeInFlightPerPeer bounds concurrent unanswered requests to one peer,
	// independent of the rate. At GenomeRequestTimeout it still admits more than
	// genomeRequestsPerMinute permits, so it too never binds before the rate does.
	GenomeInFlightPerPeer = 8
)

// RetiredContractBPaths is every path a relay MUST keep answering with 4000
// (§3, and again at §22 B32). It grows with each major and never shrinks: the
// whole value of a retired path is that a peer left behind gets a defined loud
// close instead of a bare HTTP 404, and that value is exactly as large for the
// path retired two majors ago.
var RetiredContractBPaths = []string{"/contract-b/v2", "/contract-b/v3"}

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
	// MinContractVersion is the floor this relay admits (added — §22, B25).
	// ABSENT MEANS NO MINIMUM, which is the default and the only honest one: a
	// floor is a deployment decision, and a relay that has not made one must not
	// enforce a guess. A peer that reads it can say what it will need before it
	// needs it, and an operator surface can say which peers are one release from
	// being refused.
	MinContractVersion string `json:"minContractVersion,omitempty"`
	// Limits is B24's published capacity table, and B24 is WP4. The field is
	// declared so the SHAPE is settled — §6.2 makes it REQUIRED and it is
	// deliberately omitted while the table does not exist, because publishing an
	// empty object would claim a relay running with no ceilings rather than a
	// relay that has not shipped them yet. WP4 fills it, and every reader here
	// already treats absence as unknown.
	Limits map[string]int64 `json:"limits,omitempty"`
}

// SaveReceipt is the mod's save receipt, copied verbatim from
// HEARTBEAT.lastSave (contract-a.md §5.2, §15 A21).
type SaveReceipt = contracta.SaveReceipt

// The species census, copied VERBATIM from HEARTBEAT.species (contract-a.md
// §5.2, §17 A35; §6.3.1, §16 B11). Same shape, same types, one validator — and
// that validator lives on the OTHER wire, where the census enters the system.
// This wire adds no second validator; it adds a BOUND (SpeciesCensusMax).
type (
	Census        = wire.Census
	CensusEntry   = wire.CensusEntry
	TruncatedFlag = wire.TruncatedFlag
)

// ExcludeList is `migrationExclude`, copied verbatim from the mod's handshake
// (contract-a.md §5.1, §19 A42; §6.3.1, §19 B18). Same shape, same rules, one
// validator — and that validator lives on the OTHER wire, where the list enters
// the system. THE ABSENT/EMPTY DISTINCTION IS THE WHOLE POINT OF THE TYPE: nil
// is a world that has not said, and a present empty list is a world that has
// said its exclusion policy is off.
type ExcludeList = wire.ExcludeList

// SpeciesCensusMax is §12's shared bound, the same constant contract-a.md §10
// names. It is what keeps a stats block — and the six-slot PEER_STATUS that
// republishes six of them — bounded, and no party on this wire may raise it
// unilaterally.
const SpeciesCensusMax = wire.SpeciesCensusMax

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
	// TimeScale is how fast the world is running, copied out of the last
	// HEARTBEAT (contract-a.md §5.2) and never computed here (§18, B16). 5 means
	// five simulated seconds per real second; 0 means the world is standing
	// still, which is a reading and not a gap.
	//
	// nil is UNKNOWN — no mod, or no heartbeat yet. It is the interpretive key to
	// every other pacing number on this block: simulatedTime advances at this
	// rate, and inboundRatePerSimMinute is spent at it.
	TimeScale *float64 `json:"timeScale,omitempty"`
	// InboundRatePerSimMinute and InboundRateBurst are the delivery rate limit
	// this sidecar is CONFIGURED with (contract-a.md §7.5, §18 A40; §18, B16).
	// They are settings, not measurements, and they are what makes pacedDepth
	// readable: a depth is only high or low against the cap it is queued behind.
	//
	// nil is UNKNOWN — a sidecar that predates B16 publishes neither, and a
	// reader MUST render the cap as unknown rather than assume the shipped
	// default, which has moved three times.
	InboundRatePerSimMinute *float64 `json:"inboundRatePerSimMinute,omitempty"`
	InboundRateBurst        *float64 `json:"inboundRateBurst,omitempty"`
	// Species is the world's active species census, copied out of the last
	// HEARTBEAT the sidecar received and NEVER AUTHORED HERE (§16, B11). A
	// sidecar MUST NOT synthesize one, re-sort one, merge two entries, fill a
	// missing count, or derive one from its journal, its genome cache or the
	// MIGRATION_PAYLOAD.species blocks that pass through it: those describe
	// migrants, this describes a population.
	//
	// nil is UNKNOWN — no mod, a mod older than contract-a/2.2, or no heartbeat
	// carrying one yet. A present census with no entries is a reporting mod with
	// nothing alive in its world, which is a different fact (§10.1).
	Species *Census `json:"species,omitempty"`
	// Truncated qualifies Species and nothing else (§16, B11). Monotonic, and
	// ignored when Species is absent.
	Truncated TruncatedFlag `json:"truncated,omitempty"`

	// ---------------------------------------------- what this world was TOLD to do
	//
	// The seven fields of §19, B18. Everything above answers "what is this world
	// doing"; these answer "what was it configured to do", and each is the CAUSE
	// behind a number the operator surface already shows: the exclusion list
	// beside the census that holds the excluded species and the lane that never
	// carries it, saveMinutes beside lastSave, contractAVersion beside whichever
	// field group that mod is too old to send.
	//
	// ABSENCE IS UNKNOWN IN EVERY CASE — no mod connected, a peer whose build
	// predates this amendment, or a mod older than contract-a/2.3 — and A READER
	// MUST NOT SUBSTITUTE A DEFAULT. One substitution is called out by name
	// because it is the tempting one: saveMinutes must never render as 10, which
	// is what the mod ships with, because a page that does it claims a world is
	// being saved when its timer may be off (§10.1, §19 B19).
	//
	// THEY ARE READ-ONLY AND ONE-WAY. There is no path by which any of them
	// travels back toward a mod; the sidecar never sends a CONFIG_UPDATE, so
	// there is no code path to extend (contract-a.md §19, A43).

	// ModVersion is CONFIG_UPDATE.modVersion, which this wire's sidecar has held
	// since M2 and never forwarded. IT IS NOT A CAPABILITY STATEMENT: what a mod
	// can do is ContractAVersion plus, field by field, presence. A reader that
	// gates a rendering on a version STRING rather than on a field's presence is
	// doing the arithmetic §3.1 forbids, with a looser number.
	ModVersion string `json:"modVersion,omitempty"`
	// ContractAVersion is the `protocol` identifier on that mod's frames — the
	// SESSION'S OWN, never the sidecar's build. Those differ on exactly the rig
	// this field exists to describe, and publishing the wrong one would make
	// every "unknown" on the page explain itself incorrectly.
	ContractAVersion string `json:"contractAVersion,omitempty"`
	// MigrationExclude is CONFIG_UPDATE.migrationExclude, copied VERBATIM and in
	// the mod's own order. Entries are A34-NORMALIZED because that lane matches,
	// while Species above is RAW because that lane labels — both rules live on
	// this one block and no party may apply either to the other's field. A
	// consumer asking whether an excluded species is in the census normalizes
	// THE CENSUS COPY FOR THE COMPARISON ONLY, and rewrites neither.
	//
	// nil is unknown; a present list with no entries is the policy switched OFF.
	MigrationExclude *ExcludeList `json:"migrationExclude,omitempty"`
	// SaveMinutes is wall-clock minutes between that world's periodic saves. 0 IS
	// A READING, NOT A GAP — the save timer is off — exactly as TimeScale 0 is a
	// stopped world, and a reader that folds the two together loses the one fact
	// that explains an absent LastSave.
	SaveMinutes *float64 `json:"saveMinutes,omitempty"`
	SaveKeep    *int     `json:"saveKeep,omitempty"`
	SaveOnQuit  *bool    `json:"saveOnQuit,omitempty"`
	// WorldWrapping is D10's containment fact for that world, reported by the mod
	// and never written by it. false is a reading and a loud one: it names a
	// world that is not containing its own organisms.
	WorldWrapping *bool `json:"worldWrapping,omitempty"`

	// unknown is every key of the block this build does not have a field for,
	// kept as it arrived and re-emitted on the way out. It is §16 B11's one
	// SHOULD, "for the next field after this one": a relay that re-encodes a
	// stats block from a typed model DROPS a stat a newer sidecar sends, and a
	// contract-b/3.1 relay is exactly why the census needed this note. Storing
	// what it does not understand costs a map and a merge, and it means the
	// field after the census survives a relay that predates it.
	//
	// Unexported: it is a transport detail, never a field any component reads.
	unknown map[string]json.RawMessage
}

// knownStatKeys is every key PeerStats has a field for. Anything else a peer
// sends is carried through untouched.
var knownStatKeys = []string{
	"population", "eggCount", "custodyDepth", "pacedDepth", "heldDepth",
	"bouncedTimeoutTotal", "simulatedTime", "timeScale",
	"inboundRatePerSimMinute", "inboundRateBurst",
	"lastSave", "species", "truncated",
	// §19, B18.
	"modVersion", "contractAVersion", "migrationExclude",
	"saveMinutes", "saveKeep", "saveOnQuit", "worldWrapping",
}

// UnmarshalJSON decodes the block and REMEMBERS WHAT IT DID NOT UNDERSTAND
// (§6.3.1, §16 B11's SHOULD). Unknown keys are still ignored for every purpose
// — nothing reads them, nothing routes on them — they are simply not thrown
// away, so a hop that predates a field is not the place that field dies.
func (p *PeerStats) UnmarshalJSON(b []byte) error {
	type plain PeerStats
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return err
	}
	for _, k := range knownStatKeys {
		delete(all, k)
	}
	if len(all) == 0 {
		all = nil
	}
	*p = PeerStats(v)
	p.unknown = all
	return nil
}

// MarshalJSON re-emits the known fields and then everything else the block
// arrived with. A known field always wins: this carries a stranger's key, it
// never lets one shadow a value this build computed.
func (p PeerStats) MarshalJSON() ([]byte, error) {
	type plain PeerStats
	b, err := json.Marshal(plain(p))
	if err != nil {
		return nil, err
	}
	if len(p.unknown) == 0 {
		return b, nil
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(b, &merged); err != nil {
		return nil, err
	}
	if merged == nil {
		merged = map[string]json.RawMessage{}
	}
	for k, v := range p.unknown {
		if _, taken := merged[k]; !taken {
			merged[k] = v
		}
	}
	return json.Marshal(merged)
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
	// Observers counts connected read-only subscribers. Each one is now
	// INDIVIDUALLY AUTHORISED (amended — §22, B27), so this is a count of
	// decisions an operator made rather than of sockets that knew a token.
	Observers int `json:"observers"`
	// MinContractVersion is republished here so an operator surface can say which
	// peers are one release away from being refused, and so that D25's
	// *publish, then raise the floor* is a sequence a reader can watch rather
	// than a policy they must be told (added — §22, B25). Absent means no
	// minimum.
	MinContractVersion string `json:"minContractVersion,omitempty"`
	// Limits is B24's table, republished on every broadcast so a page can render
	// each peer's behaviour against the ceilings it is measured on. B24 is WP4;
	// see HandshakeAck.Limits for why the shape is here and the value is not.
	Limits map[string]int64 `json:"limits,omitempty"`
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

// Species is the OPTIONAL species-identity block of §15, B9. It is the same
// shape and the same validator as Contract A's (contract-a.md §16, A30),
// because this wire's ENTIRE JOB is to carry that block without touching it.
type Species = wire.Species

// MigrationPayload is MIGRATION_PAYLOAD (contract-b-m4.md §6.6): the Contract C
// MigrationEnvelope with the lineage annex. The wire never carries a parent
// blob — the source sidecar hashes them, caches them and strips them.
type MigrationPayload struct {
	MigrationID string  `json:"migrationId"`
	Kind        string  `json:"kind"`
	Body        Body    `json:"body"`
	Lineage     Lineage `json:"lineage"`
	// Species is envelope metadata beside the blob, exactly like ExitEdge (§15,
	// B9). It is copied verbatim out of the origin mod's MIGRATE_OUT and handed
	// verbatim to the destination mod on MIGRATE_IN. It is NOT part of the
	// lineage annex, and deliberately: the annex holds values this wire COMPUTES
	// under genome-hash.md, and a species name is asserted by the origin world,
	// is not hashed, and is excluded from the canonical projection (§4.3 there).
	// It takes no part in dedup, custody, admission control, the S check, the
	// hold clock or a re-route, and a RE-ROUTED frame carries the same block
	// byte for byte.
	Species    *Species `json:"species,omitempty"`
	SourcePeer string   `json:"sourcePeer"`
	SourceSlot int      `json:"sourceSlot"`
	DestSlot   int      `json:"destSlot"`
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
	if !contracta.ValidEdge(p.ExitEdge) {
		// All four values, since D17 (§17, B13). The relay has never ROUTED on this
		// field — routing is on destSlot — so it is carried for the record and
		// validated only as an enum member.
		return invalid("exitEdge %q is not one of E, N, W, S", p.ExitEdge)
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
	// about Lineage is validated here. NEITHER IS SPECIES, for the stronger
	// reason B9 gives: a malformed block is STRIPPED AND LOGGED, never NACKed,
	// so its check cannot live in a function whose failure mode is
	// MIGRATION_NACK / MALFORMED_MESSAGE. The receiving sidecar applies
	// wire.CarrySpecies instead.
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
