package sidecar

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/modtoken"
)

// Version is reported to the relay in HANDSHAKE.
const Version = "m5.0"

// Sidecar defence-in-depth defaults. Neither is a wire value: they are the
// sidecar's own answer to the slot-6 livelock, so they live here rather than in
// the contract packages.
const (
	// defaultHeartbeatDeliveryGrace is ~1.5x the 1000 ms heartbeat interval
	// (contract-a.md §8). A mod silent for longer than this at the application
	// layer is not keeping up, so inbound delivery is HELD until heartbeats
	// resume. It is the FIRST of the three quiet-mod deadlines and that ordering
	// is the point: 1.5 s holds delivery, the pacer's idle grace (10 s) stops the
	// pacing clock, and only then does heartbeatTimeoutMs (13 s since §20 A45,
	// 3.5 s before it) close with 4004. The raise moved the pacer's grace from
	// after the close to before it, which changes nothing this gate does — both
	// branches release nothing — and it is why this value did not move with it
	// (contract-a.md §7.5, §8, §20 A45).
	defaultHeartbeatDeliveryGrace = 1500 * time.Millisecond
)

// Config is the sidecar's runtime configuration. The four required knobs are
// --listen, --relay, --peer-id and --data-dir; everything else has a contract
// default and exists so tests can run the real code paths on a short clock.
type Config struct {
	// Listen is the Contract A bind address. contract-a.md §2 forbids binding
	// anything but loopback, and §21 A47 now puts a BEARER TOKEN on the upgrade:
	// §12 item 1 stayed open for three milestones because the wire never leaves
	// loopback and one person owns the machine, and after M5 that machine is a
	// stranger's.
	Listen string
	// RelayURL is the Contract B endpoint, wss://host:port/contract-b/v4 for
	// anything a stranger dials, ws:// only for a loopback rehearsal (§22, B23).
	RelayURL string
	// Secret is the SECRET HALF of this peer's credential (contract-b-m4.md §3.1,
	// §22 B22). It rides the HTTP upgrade as `Bearer <peerId>.<secret>` and never
	// appears in a frame. The peerId half is not a secret and comes from
	// <DataDir>/peer-id.
	Secret string
	// ContractAToken is the token the mod must present on the Contract A upgrade
	// (contract-a.md §21, A47). Empty and not insecure means the sidecar mints one
	// into ContractATokenFile at first start, mode 0600.
	ContractAToken string
	// ContractATokenFile is `contractATokenFile` (§10). Empty defaults to
	// <DataDir>/contract-a.token.
	ContractATokenFile string
	// InsecureNoContractAToken disables the Contract A check and logs one loud
	// warning PER ACCEPTED CONNECTION. It exists for a single-machine rehearsal
	// and for nothing else, and it is a DIFFERENT FLAG ON A DIFFERENT BINARY FOR
	// A DIFFERENT WIRE from the relay's --insecure-no-token, named differently on
	// purpose so a runbook cannot confuse them (A47).
	InsecureNoContractAToken bool
	// PeerID is this sidecar's stable identity. Slot reclaim keys on it, so it
	// is persisted in <DataDir>/peer-id and generated once if absent (§7.4).
	//
	// A SIDECAR WHOSE CREDENTIAL IS REFUSED MUST NOT GENERATE A FRESH ONE (§3.1).
	// That would strand its slot, its journal's destSlot and every organism
	// addressed to it. It keeps its journal, keeps delivering inbound entries to
	// its own mod, and waits for a person.
	PeerID string
	// DataDir holds the journal, the peer id, the remembered slot and position,
	// and the genome cache.
	DataDir string
	// PreferredSlot is an optional hint for SECTOR_CLAIM. It is advisory: rule
	// 1 of §7.2 recovers the slot from the peerId anyway.
	PreferredSlot int
	// PreferredPosition is the advisory coordinate of §7.2 rule 4. It is how a
	// rig that wants a SPECIFIC layout names it, because auto-placement
	// reproduces the SHAPE and not the assignment.
	PreferredPosition *contractb.Position
	// InsertAfterSlot and InsertAxis are the advisory splice of §7.2 rule 5:
	// "place me immediately after this slot on this axis".
	InsertAfterSlot int
	InsertAxis      string

	Logger *slog.Logger
	// Clock is the time source. It exists so the bounded hold of §9.3 — a
	// twenty-four hour accrual — can be tested in milliseconds without changing
	// a single rule it tests.
	Clock func() time.Time

	// Contract A tunables (contract-a.md §10).
	HeartbeatTimeout    time.Duration
	WSPingInterval      time.Duration
	WSPongTimeout       time.Duration
	MigrateInAckTimeout time.Duration
	ExportRetention     time.Duration
	InboundQueueMax     int
	// The delivery rate limit (§7.5, §15 A20).
	InboundRatePerSimMinute float64
	InboundRateBurst        float64
	PacingIdleGrace         time.Duration
	// HeartbeatDeliveryGrace holds inbound MIGRATE_IN delivery into a mod whose
	// last app-level HEARTBEAT is older than this. It is the defence-in-depth
	// gate of the slot-6 livelock: keep releasing frames into a mod whose main
	// thread is already drowning and the stall never ends. It changes WHEN a
	// delivery happens, never WHETHER (§7.5).
	HeartbeatDeliveryGrace time.Duration

	// Contract B tunables (contract-b-m4.md §12).
	RelayBackoffMin time.Duration
	RelayBackoffMax time.Duration
	// ForwardRetry is the cadence at which an entry that has NOT yet reached a
	// live relay connection is offered to one again. It never re-sends a frame
	// that was written: a written frame is written once (§25, B37).
	ForwardRetry  time.Duration
	BounceTimeout time.Duration
	// ForwardTimeout is how long a `sent` entry waits for its answer before this
	// sidecar records it LOST (§9.3, §25 B37). Nothing is re-sent at the deadline
	// and nothing comes home: it closes a record the sender can no longer resolve.
	ForwardTimeout time.Duration
	// MaxReroutes bounds the re-routes one entry may take (§9.2). A NEGATIVE
	// value turns re-routing off altogether, so an organism refused at its first
	// destination bounces home instead of trying a second one. That is the one
	// knob that makes migration a single hop, and it is here rather than in a
	// code path so the owner can take it without a release.
	MaxReroutes             int
	StatsInterval           time.Duration
	GenomeRequestsPerMinute int
	GenomeCacheRetention    time.Duration
	GenomeCacheMaxBytes     int64

	// TickInterval drives the custody scheduler.
	TickInterval time.Duration

	// JournalCompactInterval is how often the journal is rewritten to its live
	// entries (contract-b-m4.md §12, journalCompactMinutes). Before it existed
	// the journal only ever shrank at startup, so a sidecar that stayed up
	// accumulated every create record it had ever written — payload included —
	// until the disk ran out. A compaction reads nothing and writes only the
	// live set, so the interval trades a few milliseconds against gigabytes.
	JournalCompactInterval time.Duration

	// Fault is a test-only crash point. It is set from MULTIVERSE_FAULT and is
	// never a flag. On reaching the named point the sidecar touches
	// <DataDir>/fault.hit and blocks that goroutine forever, so a test can
	// SIGKILL it at an exactly known moment. See faultPost* below.
	Fault string
}

// Test-only fault points.
const (
	// FaultPostJournal blocks after the outbound journal entry is fsynced and
	// before MIGRATE_OUT_ACK is sent — the single most dangerous instant in
	// the custody chain (D2).
	FaultPostJournal = "post-journal"
	// FaultPostForward blocks after MIGRATION_PAYLOAD has been handed to the
	// relay.
	FaultPostForward = "post-forward"
)

// DefaultConfig returns the contract defaults.
func DefaultConfig() Config {
	return Config{
		Listen:                  fmt.Sprintf("127.0.0.1:%d", contracta.DefaultPort),
		RelayURL:                fmt.Sprintf("ws://127.0.0.1:%d%s", contractb.DefaultRelayPort, contractb.ContractBPath),
		DataDir:                 "multiverse-data",
		Clock:                   time.Now,
		HeartbeatTimeout:        contracta.HeartbeatTimeout,
		WSPingInterval:          contracta.WSPingInterval,
		WSPongTimeout:           contracta.WSPongTimeout,
		MigrateInAckTimeout:     contracta.MigrateInAckTimeout,
		ExportRetention:         contracta.ExportRetention,
		InboundQueueMax:         contracta.InboundQueueMax,
		InboundRatePerSimMinute: contracta.InboundRatePerSimMinute,
		InboundRateBurst:        contracta.InboundRateBurst,
		PacingIdleGrace:         contracta.PacingIdleGrace,
		HeartbeatDeliveryGrace:  defaultHeartbeatDeliveryGrace,
		RelayBackoffMin:         contractb.RelayBackoffMin,
		RelayBackoffMax:         contractb.RelayBackoffMax,
		ForwardRetry:            contractb.ForwardRetry,
		BounceTimeout:           contractb.BounceTimeout,
		ForwardTimeout:          contractb.ForwardTimeout,
		MaxReroutes:             contractb.MaxReroutes,
		StatsInterval:           contractb.StatsInterval,
		GenomeRequestsPerMinute: contractb.GenomeRequestsPerMinute,
		GenomeCacheRetention:    contractb.GenomeCacheRetention,
		GenomeCacheMaxBytes:     contractb.GenomeCacheMaxBytes,
		TickInterval:            250 * time.Millisecond,
		JournalCompactInterval:  15 * time.Minute,
	}
}

func (c *Config) applyDefaults() {
	d := DefaultConfig()
	if c.Listen == "" {
		c.Listen = d.Listen
	}
	if c.RelayURL == "" {
		c.RelayURL = d.RelayURL
	}
	if c.DataDir == "" {
		c.DataDir = d.DataDir
	}
	if c.ContractATokenFile == "" {
		// §10's `contractATokenFile` is <state-dir>/contract-a.token, and the
		// sidecar's state directory is the one it already owns.
		c.ContractATokenFile = filepath.Join(c.DataDir, modtoken.DefaultFileName)
	}
	if c.PeerID == "" {
		host, err := os.Hostname()
		if err != nil || host == "" {
			host = "sidecar"
		}
		c.PeerID = host + "-" + strconv.Itoa(os.Getpid())
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.Clock == nil {
		c.Clock = time.Now
	}
	if c.HeartbeatTimeout <= 0 {
		c.HeartbeatTimeout = d.HeartbeatTimeout
	}
	if c.WSPingInterval <= 0 {
		c.WSPingInterval = d.WSPingInterval
	}
	if c.WSPongTimeout <= 0 {
		c.WSPongTimeout = d.WSPongTimeout
	}
	if c.MigrateInAckTimeout <= 0 {
		c.MigrateInAckTimeout = d.MigrateInAckTimeout
	}
	if c.ExportRetention <= 0 {
		c.ExportRetention = d.ExportRetention
	}
	if c.InboundQueueMax <= 0 {
		c.InboundQueueMax = d.InboundQueueMax
	}
	if c.InboundRatePerSimMinute <= 0 {
		c.InboundRatePerSimMinute = d.InboundRatePerSimMinute
	}
	if c.InboundRateBurst <= 0 {
		c.InboundRateBurst = d.InboundRateBurst
	}
	if c.PacingIdleGrace <= 0 {
		c.PacingIdleGrace = d.PacingIdleGrace
	}
	if c.HeartbeatDeliveryGrace <= 0 {
		c.HeartbeatDeliveryGrace = d.HeartbeatDeliveryGrace
	}
	if c.RelayBackoffMin <= 0 {
		c.RelayBackoffMin = d.RelayBackoffMin
	}
	if c.RelayBackoffMax <= 0 {
		c.RelayBackoffMax = d.RelayBackoffMax
	}
	if c.ForwardRetry <= 0 {
		c.ForwardRetry = d.ForwardRetry
	}
	if c.BounceTimeout <= 0 {
		c.BounceTimeout = d.BounceTimeout
	}
	if c.ForwardTimeout <= 0 {
		c.ForwardTimeout = d.ForwardTimeout
	}
	// 0 is "unset" and takes the default; a NEGATIVE value is a choice — no
	// re-routes at all — and applyDefaults must not overwrite a choice.
	if c.MaxReroutes == 0 {
		c.MaxReroutes = d.MaxReroutes
	}
	if c.StatsInterval <= 0 {
		c.StatsInterval = d.StatsInterval
	}
	if c.GenomeRequestsPerMinute <= 0 {
		c.GenomeRequestsPerMinute = d.GenomeRequestsPerMinute
	}
	if c.GenomeCacheRetention <= 0 {
		c.GenomeCacheRetention = d.GenomeCacheRetention
	}
	if c.GenomeCacheMaxBytes <= 0 {
		c.GenomeCacheMaxBytes = d.GenomeCacheMaxBytes
	}
	if c.TickInterval <= 0 {
		c.TickInterval = d.TickInterval
	}
	if c.JournalCompactInterval <= 0 {
		c.JournalCompactInterval = d.JournalCompactInterval
	}
}
