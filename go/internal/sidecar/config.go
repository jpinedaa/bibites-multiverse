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
const Version = "m3.0"

// Config is the sidecar's runtime configuration. The four required knobs are
// --listen, --relay, --peer-id and --data-dir; everything else has a contract
// default and exists so tests can run the real code paths on a short clock.
type Config struct {
	// Listen is the Contract A bind address. contract-a.md §2 forbids
	// binding anything but loopback, and §14 A17 keeps it unauthenticated:
	// the mod and its sidecar always share a machine (D9).
	Listen string
	// RelayURL is the Contract B endpoint, ws://host:port/contract-b/v2.
	RelayURL string
	// Token is the shared LAN bearer token of contract-b-m3.md §3.1. It goes
	// on the HTTP upgrade and never in a frame.
	Token string
	// PeerID is this sidecar's stable identity. Slot reclaim keys on it, so it
	// is persisted in <DataDir>/peer-id and generated once if absent (§7.4).
	PeerID string
	// DataDir holds the journal, the peer id, the remembered slot and the
	// genome cache.
	DataDir string
	// PreferredSlot is an optional hint for SECTOR_CLAIM. It is advisory: rule
	// 1 of §7.2 recovers the slot from the peerId anyway.
	PreferredSlot int

	Logger *slog.Logger

	// Contract A tunables (contract-a.md §10).
	HeartbeatTimeout    time.Duration
	WSPingInterval      time.Duration
	WSPongTimeout       time.Duration
	MigrateInAckTimeout time.Duration
	ExportRetention     time.Duration
	InboundQueueMax     int

	// Contract B tunables (contract-b-m3.md §12).
	RelayBackoffMin         time.Duration
	RelayBackoffMax         time.Duration
	ForwardRetry            time.Duration
	BounceTimeout           time.Duration
	GenomeRequestsPerMinute int
	GenomeCacheRetention    time.Duration
	GenomeCacheMaxBytes     int64

	// TickInterval drives the custody scheduler.
	TickInterval time.Duration

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
		HeartbeatTimeout:        contracta.HeartbeatTimeout,
		WSPingInterval:          contracta.WSPingInterval,
		WSPongTimeout:           contracta.WSPongTimeout,
		MigrateInAckTimeout:     contracta.MigrateInAckTimeout,
		ExportRetention:         contracta.ExportRetention,
		InboundQueueMax:         contracta.InboundQueueMax,
		RelayBackoffMin:         contractb.RelayBackoffMin,
		RelayBackoffMax:         contractb.RelayBackoffMax,
		ForwardRetry:            contractb.ForwardRetry,
		BounceTimeout:           contractb.BounceTimeout,
		GenomeRequestsPerMinute: contractb.GenomeRequestsPerMinute,
		GenomeCacheRetention:    contractb.GenomeCacheRetention,
		GenomeCacheMaxBytes:     contractb.GenomeCacheMaxBytes,
		TickInterval:            250 * time.Millisecond,
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
}
