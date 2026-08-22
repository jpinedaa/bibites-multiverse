package contractb

import "time"

// §3.3's capacity and abuse table, added by §22, B24.
//
// THE WHOLE DESIGN CONSTRAINT IS D1. Every limit here is countable at the FRAME
// level — connections, frames, bytes, claims, genome requests, subscribers — so
// none of them requires the relay to decode a body it is forbidden to read
// (§5). A limit that needed a payload read is a limit this relay may not have:
// D1 is why the archive is a separate service and why M6 can replace the relay
// with libp2p, and an abuse limit is not worth spending it.
//
// EVERY ONE IS A KNOB (D20). Each has a flag and an environment variable on
// multiverse-relay and none is a compiled constant — "a tunable an operator
// cannot retune from the metric that measures it is not a tunable." The
// constants below are DEFAULTS, which is a different thing from a compiled
// limit: they are what a relay runs with until its operator says otherwise, and
// what it PUBLISHES is what it is running with, never this table.
//
// EVERY ONE IS PUBLISHED. Published() is what rides HANDSHAKE_ACK.limits at
// connect and PEER_STATUS.limits thereafter (§6.2, §6.5), BESIDE the peer stats
// blocks and never inside one: §6.3.1's block is peer-authored end to end, and
// a relay-authored key inside it would be the first value on it the peer did
// not write.

// The shipped defaults of §3.3, restated in §12. A relay configured differently
// publishes what it is running with; these are only where it starts.
const (
	// DefaultMaxConnectionsPerPeer admits the §3.2 4006 overlap during a
	// reconnect and nothing more. A third simultaneous authenticated connection
	// for one peerId is close 4007.
	DefaultMaxConnectionsPerPeer = 2
	// DefaultMaxConnectionsPerAddress is DELIBERATELY LOOSE: one machine
	// legitimately runs several peers, and the living deployment runs five.
	DefaultMaxConnectionsPerAddress = 8
	// DefaultMaxFramesPerSecond is sized for a migration burst, not for a steady
	// rate: a peer at statsIntervalMs sends well under one frame a second.
	DefaultMaxFramesPerSecond = 50
	// DefaultMaxFrameBytes is §12's shared 8 MiB, unchanged. Over it is close
	// 1009 TOO_BIG and never 4007 — a frame too big is a shape fault, not a
	// capacity one, and the two have different remedies.
	DefaultMaxFrameBytes = 8388608
	// DefaultMaxBytesPerSecond is what stops maxFramesPerSecond being evaded
	// with maximum frames.
	DefaultMaxBytesPerSecond = 4194304
	// DefaultMaxClaimsPerMinute answers a claim storm with a REFUSAL THE PEER
	// CAN READ rather than a close it must recover from: a storm is usually a
	// peer whose measured time scale is wandering (DQ3's 64 claims in a day).
	DefaultMaxClaimsPerMinute = 12
	// DefaultMaxGenomeRequestsPerMinute is the former compiled constant
	// contractb.GenomeRequestsPerMinute, which is the worked example D20's knob
	// rule was written about. Per requester, PER ANSWERING PEER, on both sides.
	DefaultMaxGenomeRequestsPerMinute = GenomeRequestsPerMinute
	// DefaultMaxSubscribers bounds the FAN-OUT COST of role:"archive" clients.
	// B27's grant is what bounds the trust; this bounds the copying.
	DefaultMaxSubscribers = 4

	// MigrationFanInDivisor derives the relay's per-destination migration
	// forwarding rate from maxFramesPerSecond. The sidecar's shared send bucket
	// keeps one eighth of that published ceiling outside its journal drain. The
	// relay spaces migration arrivals at the same rate, so the immediate ACK or
	// NACK for one arrival cannot turn aggregate fan-in into an unbounded control
	// burst. This is a derived invariant, not a second operator knob: changing
	// maxFramesPerSecond changes both rates together.
	MigrationFanInDivisor int64 = 8
	// MinimumMaxFramesPerSecond is the smallest supported relay frame ceiling.
	// One migration response can consume one eighth of the ceiling. The other
	// seven eighths remain available for PONG, stats, claims, and other control
	// traffic. A smaller ceiling cannot preserve that control reserve.
	MinimumMaxFramesPerSecond int64 = MigrationFanInDivisor
	// MigrationControlBurst is the maximum number of ready ordinary frames that
	// can leapfrog one queued migration. It gives control traffic priority
	// without letting a continuous control stream starve an accepted organism.
	MigrationControlBurst = MigrationFanInDivisor - 1
	// MigrationDrainTimeout bounds the relay-wide graceful close. A destination
	// queue retains at most F frames and drains at floor(F/8) per second. Across
	// all supported F values, that takes at most 15 seconds. The shared deadline
	// leaves scheduling and close-handshake margin, then force-closes any socket
	// write that is still blocked and drops its remainder conservatively.
	MigrationDrainTimeout = 18 * time.Second
)

// MigrationFanInRate is the number of MIGRATION_PAYLOAD frames that a relay
// forwards to one destination peer per second. A positive floor keeps a relay
// usable when it reads a limit from an older relay. Current relays reject a
// maxFramesPerSecond value below MinimumMaxFramesPerSecond at startup.
func MigrationFanInRate(maxFramesPerSecond int64) int64 {
	rate := maxFramesPerSecond / MigrationFanInDivisor
	if rate < 1 {
		return 1
	}
	return rate
}

// MigrationFanInInterval is the minimum wall-clock space between migration
// forwards to one destination. The division rounds up to a nanosecond. Without
// that ceiling, six truncated intervals fit just inside one second and a relay
// configured for six frames per second can emit a seventh at the boundary.
func MigrationFanInInterval(maxFramesPerSecond int64) time.Duration {
	rate := time.Duration(MigrationFanInRate(maxFramesPerSecond))
	interval := time.Second / rate
	if time.Second%rate != 0 {
		interval++
	}
	if interval < time.Nanosecond {
		return time.Nanosecond
	}
	return interval
}

// The published key names. They are the wire's, spelled once, so a relay, a
// page and a test cannot disagree about them (§3.3, §6.2, §6.5).
const (
	LimitMaxConnectionsPerPeer      = "maxConnectionsPerPeer"
	LimitMaxConnectionsPerAddress   = "maxConnectionsPerAddress"
	LimitMaxFramesPerSecond         = "maxFramesPerSecond"
	LimitMaxFrameBytes              = "maxFrameBytes"
	LimitMaxBytesPerSecond          = "maxBytesPerSecond"
	LimitMaxClaimsPerMinute         = "maxClaimsPerMinute"
	LimitMaxGenomeRequestsPerMinute = "maxGenomeRequestsPerMinute"
	LimitMaxSubscribers             = "maxSubscribers"
)

// PublishedLimitKeys is every key §3.3 requires on the published object, in the
// order §3.3's own table lists them. A published table with a hole in it is not
// a table, so this list is what a test asserts completeness against.
var PublishedLimitKeys = []string{
	LimitMaxConnectionsPerPeer,
	LimitMaxConnectionsPerAddress,
	LimitMaxFramesPerSecond,
	LimitMaxFrameBytes,
	LimitMaxBytesPerSecond,
	LimitMaxClaimsPerMinute,
	LimitMaxGenomeRequestsPerMinute,
	LimitMaxSubscribers,
}

// Limits is §3.3's table as one relay runs it.
type Limits struct {
	MaxConnectionsPerPeer      int64
	MaxConnectionsPerAddress   int64
	MaxFramesPerSecond         int64
	MaxFrameBytes              int64
	MaxBytesPerSecond          int64
	MaxClaimsPerMinute         int64
	MaxGenomeRequestsPerMinute int64
	MaxSubscribers             int64
}

// DefaultLimits is the shipped table.
func DefaultLimits() Limits {
	return Limits{
		MaxConnectionsPerPeer:      DefaultMaxConnectionsPerPeer,
		MaxConnectionsPerAddress:   DefaultMaxConnectionsPerAddress,
		MaxFramesPerSecond:         DefaultMaxFramesPerSecond,
		MaxFrameBytes:              DefaultMaxFrameBytes,
		MaxBytesPerSecond:          DefaultMaxBytesPerSecond,
		MaxClaimsPerMinute:         DefaultMaxClaimsPerMinute,
		MaxGenomeRequestsPerMinute: DefaultMaxGenomeRequestsPerMinute,
		MaxSubscribers:             DefaultMaxSubscribers,
	}
}

// ApplyDefaults fills every unset entry from the shipped table.
//
// A NON-POSITIVE VALUE IS NOT "UNLIMITED", IT IS "UNSET". §3.3 defines no
// off switch and this package must not invent one: a relay running with a limit
// switched off would publish a number that is not the ceiling it enforces, and
// the published table is the whole point of B24.
func (l *Limits) ApplyDefaults() {
	d := DefaultLimits()
	if l.MaxConnectionsPerPeer <= 0 {
		l.MaxConnectionsPerPeer = d.MaxConnectionsPerPeer
	}
	if l.MaxConnectionsPerAddress <= 0 {
		l.MaxConnectionsPerAddress = d.MaxConnectionsPerAddress
	}
	if l.MaxFramesPerSecond <= 0 {
		l.MaxFramesPerSecond = d.MaxFramesPerSecond
	}
	if l.MaxFrameBytes <= 0 {
		l.MaxFrameBytes = d.MaxFrameBytes
	}
	if l.MaxBytesPerSecond <= 0 {
		l.MaxBytesPerSecond = d.MaxBytesPerSecond
	}
	if l.MaxClaimsPerMinute <= 0 {
		l.MaxClaimsPerMinute = d.MaxClaimsPerMinute
	}
	if l.MaxGenomeRequestsPerMinute <= 0 {
		l.MaxGenomeRequestsPerMinute = d.MaxGenomeRequestsPerMinute
	}
	if l.MaxSubscribers <= 0 {
		l.MaxSubscribers = d.MaxSubscribers
	}
}

// Published is the object §6.2 and §6.5 carry: THE VALUES THIS RELAY IS RUNNING
// WITH, never the shipped defaults (D20). A peer that does not know the ceiling
// it is measured against cannot be built to respect it, and a support
// conversation about a limit nobody can read is unwinnable.
func (l Limits) Published() map[string]int64 {
	return map[string]int64{
		LimitMaxConnectionsPerPeer:      l.MaxConnectionsPerPeer,
		LimitMaxConnectionsPerAddress:   l.MaxConnectionsPerAddress,
		LimitMaxFramesPerSecond:         l.MaxFramesPerSecond,
		LimitMaxFrameBytes:              l.MaxFrameBytes,
		LimitMaxBytesPerSecond:          l.MaxBytesPerSecond,
		LimitMaxClaimsPerMinute:         l.MaxClaimsPerMinute,
		LimitMaxGenomeRequestsPerMinute: l.MaxGenomeRequestsPerMinute,
		LimitMaxSubscribers:             l.MaxSubscribers,
	}
}
