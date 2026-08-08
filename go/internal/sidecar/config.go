package sidecar

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
)

// Version is reported to the relay in HANDSHAKE.
const Version = "m4.0"

// Sidecar defence-in-depth defaults. Neither is a wire value: they are the
// sidecar's own answer to the slot-6 livelock, so they live here rather than in
// the contract packages.
const (
	// defaultHeartbeatDeliveryGrace is ~1.5x the 1000 ms heartbeat interval
	// (contract-a.md §8). A mod silent for longer than this at the application
	// layer is not keeping up, so inbound delivery is HELD until heartbeats
	// resume — the pacer's own idle grace (10 s) is longer than the 3.5 s
	// heartbeat timeout and so never trips during a stall (contract-a.md §7.5).
	defaultHeartbeatDeliveryGrace = 1500 * time.Millisecond
	// defaultForwardRetryMax caps the exponential backoff a re-forward to a LIVE
	// destination climbs to: 5 s, 10 s, 20 s, 40 s, then this. It bounds the
	// re-forward hammer at a live-but-slow peer without touching the dark-side
	// cadence or the hold clock (contract-b-m4.md §9.3, B5).
	defaultForwardRetryMax = 60 * time.Second
)

// Config is the sidecar's runtime configuration. The four required knobs are
// --listen, --relay, --peer-id and --data-dir; everything else has a contract
// default and exists so tests can run the real code paths on a short clock.
type Config struct {
	// Listen is the Contract A bind address. contract-a.md §2 forbids
	// binding anything but loopback, and §14 A17 keeps it unauthenticated:
	// the mod and its sidecar always share a machine (D9).
	Listen string
	// RelayURL is the Contract B endpoint, ws://host:port/contract-b/v3.
	RelayURL string
	// Token is the shared LAN bearer token of contract-b-m4.md §3.1. It goes
	// on the HTTP upgrade and never in a frame.
	Token string
	// PeerID is this sidecar's stable identity. Slot reclaim keys on it, so it
	// is persisted in <DataDir>/peer-id and generated once if absent (§7.4).
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
	ForwardRetry    time.Duration
	// ForwardRetryMax caps the exponential backoff a re-forward to a LIVE
	// destination climbs to (§9.3, B5). Re-forwards toward a DARK destination
	// keep the flat ForwardRetry cadence, because that retry is the only conduit
	// the non-delivery proof has and the hold clock accrues against it.
	ForwardRetryMax time.Duration
	BounceTimeout   time.Duration
	// HoldTimeout is the ACCRUED dark time before a held entry bounces home by
	// itself (§9.3). The clock runs only while the destination is dark and this
	// sidecar can see it.
	HoldTimeout time.Duration
	// HoldAccrualFlush is how often that accrual is written to the journal. A
	// crash loses at most this much, in the safe direction.
	HoldAccrualFlush time.Duration
	// MaxReroutes bounds the re-routes one entry may take (§9.2).
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
		ForwardRetryMax:         defaultForwardRetryMax,
		BounceTimeout:           contractb.BounceTimeout,
		HoldTimeout:             contractb.HoldTimeout,
		HoldAccrualFlush:        contractb.HoldAccrualFlush,
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
	if c.ForwardRetryMax <= 0 {
		c.ForwardRetryMax = d.ForwardRetryMax
	}
	if c.BounceTimeout <= 0 {
		c.BounceTimeout = d.BounceTimeout
	}
	if c.HoldTimeout <= 0 {
		c.HoldTimeout = d.HoldTimeout
	}
	if c.HoldAccrualFlush <= 0 {
		c.HoldAccrualFlush = d.HoldAccrualFlush
	}
	if c.MaxReroutes <= 0 {
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
