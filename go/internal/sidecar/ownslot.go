package sidecar

// THE PARTICIPANT'S OWN-SLOT VIEW — `multiverse-sidecar --my-slot`.
//
// DQ8's bar, and WP7's: "A participant should be able to read their own slot's
// liveness, lane states, paced depth and last save without an operator reading
// it to them" (m5_considerations.md, Design Question 8).
//
// WHY IT IS THE SIDECAR AND NOT THE PAGE. The archive's status page renders
// every one of those fields already, and DQ8 called the work "mostly a
// presentation change". It is not, for one reason: the page is the OPERATOR'S
// service. It lives at the map's address, it is one more thing that can be down
// or unreachable or behind a hosting decision WP3 has not made yet, and a
// participant whose relay link is refused — the case they most need to read —
// is exactly the participant whose network path to that page is least likely to
// be the one working. The sidecar, by contrast, is on the participant's own
// machine, is the process the question is about, and already holds every field:
// its own slot and position from the SECTOR_GRANT, every peer's liveness and
// game version from PEER_STATUS, its lanes from its own edge computation, its
// depths from its own journal, and its last save from its mod's heartbeat.
//
// SO IT ADDS NO WIRE MESSAGE, which is D1's rule and the constraint WP7 was
// given. Nothing here asks the relay for anything. Every value is one this
// process already had.
//
// THE SURFACE IS A READ-ONLY LOOPBACK ENDPOINT, plus a CLI that renders it.
// `GET /my-slot` on the Contract A listener the sidecar already binds — the same
// address, the same loopback-only rule (contract-a.md §2), served beside
// /healthz. The CLI reads <data-dir>/listen.addr to find it, which is the
// mechanism that already exists for a sidecar started on port 0.
//
// IT IS NOT BEHIND THE CONTRACT A TOKEN, deliberately, and the reason is worth
// stating because the default answer would be the other one. A47's bearer token
// exists because ANY local process could otherwise drive this world's migrations
// and impersonate the sidecar to the mod; it is an authority control on a wire
// that MOVES ORGANISMS. This endpoint moves nothing, answers only GET, and
// carries no field that is not ALREADY BROADCAST TO EVERY PEER ON THE MAP in
// PEER_STATUS — liveness, slot, position, versions, depths, save receipt,
// lastRefusal. Putting the participant's own view behind the token would also
// make the one diagnosis they most need — "my mod cannot authenticate" — the one
// diagnosis they cannot run.
//
// AND IT NEVER CARRIES A SECRET. Not the relay credential, not the Contract A
// token, in whole, in prefix, hashed or elided-with-length. It carries the PATHS
// they are read from and whether each file exists, because that is the fact a
// participant needs and it is not the secret (docs/sidecar-diagnose-spec.md §1).

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/mapwalk"
	"multiverse/internal/termsafe"
	"multiverse/internal/wire"
)

// OwnSlotPath is where the view is served on the Contract A listener.
const OwnSlotPath = "/my-slot"

// OwnSlotSchema names the JSON shape. It is versioned so that a report from an
// old build is still readable, which is the same promise --diagnose --json
// makes.
const OwnSlotSchema = "multiverse-own-slot/1"

// processRecordName is the file the stale-process check reads.
//
// IT IS NOT CALLED sidecar.pid, and the reason is next door: the packaged
// install's own `Start-Multiverse.ps1` already writes `<data-root>\sidecar.pid`,
// a bare process id that `Stop-Multiverse.ps1` reads. That file is one directory
// up from this one — the data root, not the data directory — so the two would
// not collide, but two files with one name and two formats a directory apart is
// a trap somebody eventually falls into. This one says what it is and what it
// holds.
const processRecordName = "sidecar-process.json"

// OwnSlot is one sidecar's answer to "what does the map say about MY world".
//
// EVERY ABSENCE IS A VALUE. A pointer that is nil is a fact this sidecar has not
// been told, never a zero: a slot that reports nothing is unknown, not empty,
// and an honest gap beats a confident zero (Risk 4, contract-b-m4.md §6.3.1).
type OwnSlot struct {
	Schema     string `json:"schema"`
	ObservedAt int64  `json:"observedAtMs"`

	// This process.
	SidecarVersion   string `json:"sidecarVersion"`
	ContractAVersion string `json:"contractAVersion"`
	ContractBVersion string `json:"contractBVersion"`
	PID              int    `json:"pid"`
	StartedAt        int64  `json:"startedAtMs"`
	DataDir          string `json:"dataDir"`
	PeerID           string `json:"peerId"`
	RelayURL         string `json:"relayUrl"`
	ContractAListen  string `json:"contractAListen"`

	// Paths only. See the file comment: never a secret, ever.
	ContractATokenFile    string `json:"contractATokenFile"`
	ContractATokenPresent bool   `json:"contractATokenPresent"`
	ContractAInsecure     bool   `json:"contractAInsecure"`
	ContractAAuthFailures int    `json:"contractAAuthFailures"`
	CredentialConfigured  bool   `json:"credentialConfigured"`

	Relay   RelayState   `json:"relay"`
	Slot    SlotState    `json:"slot"`
	Mod     ModState     `json:"mod"`
	Custody CustodyState `json:"custody"`
	Edges   []EdgeState  `json:"edges"`
	Peers   []PeerState  `json:"peers"`
	Wire    WireState    `json:"wire"`
}

// RelayState is this sidecar's Contract B link as it stands.
type RelayState struct {
	Connected        bool   `json:"connected"`
	ConnectedSince   int64  `json:"connectedSinceMs,omitempty"`
	RelaySessionID   string `json:"relaySessionId,omitempty"`
	CapacitySheds    int    `json:"capacityShedTotal"`
	LastFaultKind    string `json:"lastFaultKind,omitempty"`
	LastFaultReason  string `json:"lastFaultReason,omitempty"`
	LastFaultAt      int64  `json:"lastFaultAtMs,omitempty"`
	LastFaultRepeats int    `json:"lastFaultConsecutive,omitempty"`
}

// SlotState is this world's place on the map, and what the map last said about
// it.
type SlotState struct {
	Slot           int                `json:"slot"`
	Position       contractb.Position `json:"position"`
	RememberedSlot int                `json:"rememberedSlot"`
	MapWidth       int                `json:"mapWidth"`
	MapHeight      int                `json:"mapHeight"`
	SlotCount      int                `json:"slotCount"`
	Epoch          int64              `json:"epoch,omitempty"`
	Live           bool               `json:"live"`
	// LastRefusal is §6.5's field for THIS slot, verbatim. Absent means nothing
	// was refused that reaches a slot — which is not the same as nothing being
	// refused: a credential refusal and an eviction never appear here.
	LastRefusal string `json:"lastRefusal,omitempty"`
	// The last placement answer, granted or refused, with the relay's own word.
	GrantSeen   bool   `json:"grantSeen"`
	Granted     bool   `json:"granted,omitempty"`
	GrantReason string `json:"grantReason,omitempty"`
	GrantAt     int64  `json:"grantAtMs,omitempty"`
}

// ModState is the game behind this sidecar.
type ModState struct {
	Connected        bool     `json:"connected"`
	GameVersion      string   `json:"gameVersion,omitempty"`
	ModVersion       string   `json:"modVersion,omitempty"`
	ContractAVersion string   `json:"contractAVersion,omitempty"`
	SimulationSize   float64  `json:"simulationSize,omitempty"`
	ExportEdges      []string `json:"exportEdges,omitempty"`
	BorderEdges      []string `json:"borderEdges,omitempty"`
	LastHeartbeatAge int64    `json:"lastHeartbeatAgeMs,omitempty"`
	HeartbeatTimeout int64    `json:"heartbeatTimeoutMs"`
	Paused           *bool    `json:"paused,omitempty"`
	Population       *int     `json:"population,omitempty"`
	// TimeScale is the APPLIED scale, copied from the heartbeat and never
	// computed. AchievedTimeScale is what the world actually produced per wall
	// second, measured here. The gap between them is the reading.
	TimeScale         *float64               `json:"timeScale,omitempty"`
	AchievedTimeScale *float64               `json:"achievedTimeScale,omitempty"`
	AchievedSpanMs    int64                  `json:"achievedSpanMs,omitempty"`
	SimulatedTime     *float64               `json:"simulatedTime,omitempty"`
	LastSave          *contracta.SaveReceipt `json:"lastSave,omitempty"`
	SaveMinutes       *float64               `json:"saveMinutes,omitempty"`
	SaveKeep          *int                   `json:"saveKeep,omitempty"`
	SaveOnQuit        *bool                  `json:"saveOnQuit,omitempty"`
	WorldWrapping     *bool                  `json:"worldWrapping,omitempty"`
}

// CustodyState is this machine's journal, which is the half of the system no
// operator can see (contract-b-m4.md §7.5).
type CustodyState struct {
	CustodyDepth    int `json:"custodyDepth"`
	PacedDepth      int `json:"pacedDepth"`
	PendingAckDepth int `json:"pendingAckDepth"`
	// UnresolvedDepth is outbound entries forwarded once with no answer yet, and
	// LostForwardTotal is how many of them this sidecar has written off since it
	// last lost its journal (§9.3, §25 B37). `heldDepth` and
	// `bouncedTimeoutTotal` stood here and went with the bounded hold.
	UnresolvedDepth         int     `json:"unresolvedDepth"`
	LostForwardTotal        int     `json:"lostForwardTotal"`
	LateAckTotal            int     `json:"lateAckTotal"`
	InboundRatePerSimMinute float64 `json:"inboundRatePerSimMinute"`
	InboundRateBurst        float64 `json:"inboundRateBurst"`
	// The oldest waiting entry of each kind. A depth is a number; an AGE is what
	// says whether it is draining.
	OldestPacedAgeMs      int64 `json:"oldestPacedAgeMs,omitempty"`
	OldestPendingAckAgeMs int64 `json:"oldestPendingAckAgeMs,omitempty"`
	OldestUnresolvedAgeMs int64 `json:"oldestUnresolvedAgeMs,omitempty"`
	JournalBytes          int64 `json:"journalBytes"`
	JournalLive           int   `json:"journalLive"`
	// JournalDiscardedBytes is what replay threw away behind a torn record AT
	// THIS PROCESS'S START. Zero on every healthy start.
	JournalDiscardedBytes int64 `json:"journalDiscardedBytes"`
}

// EdgeState is one declared export edge as this sidecar last computed it
// (contract-b-m4.md §8).
type EdgeState struct {
	Edge   string `json:"edge"`
	Open   bool   `json:"open"`
	Reason string `json:"reason"`
	// PeerSlot is the effective neighbour on this edge, when there is one.
	PeerSlot int `json:"peerSlot,omitempty"`
	// Usable is A50: whether this edge lies on an axis the map HAS. An edge that
	// is not usable stays closed for the life of this map shape and no organism
	// is affected.
	Usable bool `json:"usable"`
}

// PeerState is one other world, as the map broadcasts it. It is the evidence
// DQ8's worst failure needs: the peer that suffers is not the peer that is
// stale, and this is where the sufferer can see which one is.
type PeerState struct {
	Slot int `json:"slot"`
	// PeerID is public — the taxonomy's own "what to send when you ask for help"
	// says so — and it is carried in the machine-readable form only. The
	// terminal table renders slots, because the slot is what an operator needs
	// to find a world and a column of identities is a column of somebody else's
	// chosen text.
	PeerID       string `json:"peerId,omitempty"`
	Col          int    `json:"col"`
	Row          int    `json:"row"`
	Me           bool   `json:"me"`
	Live         bool   `json:"live"`
	ModConnected bool   `json:"modConnected"`
	GameVersion  string `json:"gameVersion,omitempty"`
	ModVersion   string `json:"modVersion,omitempty"`
	LastRefusal  string `json:"lastRefusal,omitempty"`
	DarkSinceMs  int64  `json:"darkSinceMs,omitempty"`
}

// WireState is what the map published about itself, and what this connection
// has actually done under it.
type WireState struct {
	// Limits is the table the relay published — the values IT IS RUNNING WITH,
	// never a default this build knows. Absent means the relay published none,
	// which reads as UNKNOWN and never as "no ceilings".
	Limits map[string]int64 `json:"limits,omitempty"`
	// MinContractVersion is the floor this map admits. Absent means no minimum.
	MinContractVersion string `json:"minContractVersion,omitempty"`
	// What this connection put on the wire, in the terms the relay counts it.
	PeakFramesPerSecond int64 `json:"peakFramesPerSecond"`
	PeakBytesPerSecond  int64 `json:"peakBytesPerSecond"`
	LargestFrameBytes   int64 `json:"largestFrameBytes"`
	ClaimsLastMinute    int   `json:"claimsLastMinute"`
	// The pace this sidecar adopted under the published ceiling, and how many
	// journal-backed frames it held back to keep it. Rising deferrals with no shed
	// is the pacing working.
	PacedFramesPerSecond float64 `json:"pacedFramesPerSecond"`
	PacedDeferrals       int64   `json:"pacedDeferrals"`
	// WarnFraction is the share of a published limit at which a reading counts
	// as approaching it. Published here so a reader can reproduce the judgement.
	WarnFraction float64 `json:"warnFraction"`
}

// ---------------------------------------------------------------- the reader

// OwnSlot builds the view from this sidecar's live state.
func (s *Sidecar) OwnSlot() OwnSlot {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()

	v := OwnSlot{
		Schema:                OwnSlotSchema,
		ObservedAt:            now.UnixMilli(),
		SidecarVersion:        Version,
		ContractAVersion:      wire.ProtocolA,
		ContractBVersion:      wire.ProtocolB,
		PID:                   os.Getpid(),
		StartedAt:             s.startedAt.UnixMilli(),
		DataDir:               s.cfg.DataDir,
		PeerID:                s.cfg.PeerID,
		RelayURL:              s.cfg.RelayURL,
		ContractAListen:       s.listenAddrLocked(),
		ContractATokenFile:    s.cfg.ContractATokenFile,
		ContractAInsecure:     s.cfg.InsecureNoContractAToken,
		ContractAAuthFailures: s.contractAAuthFailures,
		CredentialConfigured:  s.cfg.Secret != "",
	}
	if _, err := os.Stat(s.cfg.ContractATokenFile); err == nil {
		v.ContractATokenPresent = true
	}

	v.Relay = RelayState{
		Connected:      s.relayReady,
		RelaySessionID: s.relaySessionID,
		CapacitySheds:  s.capacityShedTotal,
	}
	if !s.relayConnectedAt.IsZero() {
		v.Relay.ConnectedSince = s.relayConnectedAt.UnixMilli()
	}
	if f := s.relayFault; f != nil {
		v.Relay.LastFaultKind = f.kind
		v.Relay.LastFaultReason = f.reason
		v.Relay.LastFaultAt = f.at.UnixMilli()
		v.Relay.LastFaultRepeats = f.consecutive
	}

	v.Slot = SlotState{
		Slot:           s.slot,
		Position:       s.position,
		RememberedSlot: s.readSlot(),
		MapWidth:       s.mapShape.Width,
		MapHeight:      s.mapShape.Height,
		SlotCount:      s.slotCount,
		Epoch:          s.peerEpoch,
	}
	if me, ok := mapwalk.Find(s.status, s.slot); ok && s.slot != 0 {
		v.Slot.Live = me.Live
		v.Slot.LastRefusal = me.LastRefusal
	}
	if g := s.lastGrant; g != nil {
		v.Slot.GrantSeen = true
		v.Slot.Granted = g.granted
		v.Slot.GrantReason = g.reason
		v.Slot.GrantAt = g.at.UnixMilli()
	}

	v.Mod = s.modStateLocked(now)
	v.Custody = s.custodyStateLocked(now)
	v.Edges = s.edgeStatesLocked()
	v.Peers = s.peerStatesLocked()
	v.Wire = s.wireStateLocked(now)
	return v
}

func (s *Sidecar) modStateLocked(now time.Time) ModState {
	m := ModState{HeartbeatTimeout: s.cfg.HeartbeatTimeout.Milliseconds()}
	sess := s.mod
	if sess == nil || !sess.handshaked {
		return m
	}
	m.Connected = true
	m.GameVersion = sess.gameVersion
	m.ModVersion = sess.modVersion
	m.ContractAVersion = sess.contractAVersion
	m.SimulationSize = sess.simSize
	m.ExportEdges = append([]string(nil), sess.exportEdges...)
	m.BorderEdges = append([]string(nil), sess.borderEdges...)
	if !sess.lastHeartbeat.IsZero() {
		// The heartbeat clock is the sidecar's injectable one, because the
		// deadline it is compared against is (contract-a.md §8).
		m.LastHeartbeatAge = s.now().Sub(sess.lastHeartbeat).Milliseconds()
	}
	paused := sess.paused
	m.Paused = &paused
	if sess.haveTimeScale {
		ts := sess.timeScale
		m.TimeScale = &ts
	}
	if sess.havePopulation {
		p := sess.population
		m.Population = &p
	}
	if sess.haveSimTime {
		st := sess.simulatedTime
		m.SimulatedTime = &st
	}
	if rate, span, ok := s.achieved.rate(now); ok {
		m.AchievedTimeScale = &rate
		m.AchievedSpanMs = span.Milliseconds()
	}
	if sess.lastSave != nil {
		save := *sess.lastSave
		m.LastSave = &save
	}
	m.SaveMinutes = copyFloat(sess.settings.SaveMinutes)
	m.SaveKeep = copyInt(sess.settings.SaveKeep)
	m.SaveOnQuit = copyBool(sess.settings.SaveOnQuit)
	m.WorldWrapping = copyBool(sess.settings.WorldWrapping)
	return m
}

func (s *Sidecar) custodyStateLocked(now time.Time) CustodyState {
	states := s.jr.List()
	c := CustodyState{
		CustodyDepth:            s.jr.CountPending(journal.Out) + s.jr.CountPending(journal.In),
		PacedDepth:              s.pacedDepthLocked(),
		UnresolvedDepth:         s.unresolvedDepthLocked(),
		LostForwardTotal:        s.lostForwardTotal,
		LateAckTotal:            s.lateAckTotal,
		InboundRatePerSimMinute: s.cfg.InboundRatePerSimMinute,
		InboundRateBurst:        s.cfg.InboundRateBurst,
		JournalBytes:            s.jr.Size(),
		JournalLive:             s.jr.Live(),
		JournalDiscardedBytes:   s.jr.Discarded(),
	}
	c.PendingAckDepth, c.OldestPendingAckAgeMs = pendingAckWaiting(states, now)
	c.OldestPacedAgeMs, c.OldestUnresolvedAgeMs = oldestWaiting(states, now)
	return c
}

// pendingAckWaiting counts completed inbound entries whose durable
// MIGRATION_ACK has not left yet. Its age starts when local delivery completed.
// An old record without CompletedAt keeps the depth but reports no invented age.
func pendingAckWaiting(states []*journal.State, now time.Time) (depth int, oldestAge int64) {
	var oldest int64
	for _, st := range states {
		if !st.AwaitsUpstreamAck() {
			continue
		}
		depth++
		at := st.CompletedAt
		if at > 0 && (oldest == 0 || at < oldest) {
			oldest = at
		}
	}
	if nowMs := now.UnixMilli(); oldest > 0 && nowMs > oldest {
		oldestAge = nowMs - oldest
	}
	return depth, oldestAge
}

// oldestWaiting is the age of the oldest entry waiting on the delivery rate and
// the age of the oldest entry forwarded with no answer. A depth with no age
// beside it cannot say whether it is draining — and on the second one an age
// approaching forwardTimeoutMs says the entry is about to be written off.
func oldestWaiting(states []*journal.State, now time.Time) (paced, unresolved int64) {
	var oldestPaced, oldestUnresolved int64
	for _, st := range states {
		switch {
		case st.Direction == journal.In && st.Status == journal.StatusOpen:
			if at := st.Entry.JournaledAt; at > 0 && (oldestPaced == 0 || at < oldestPaced) {
				oldestPaced = at
			}
		case st.Direction == journal.Out && st.Handoff == journal.HandoffSent &&
			(st.Status == journal.StatusOpen || st.Status == journal.StatusInFlight):
			if at := st.Entry.JournaledAt; at > 0 && (oldestUnresolved == 0 || at < oldestUnresolved) {
				oldestUnresolved = at
			}
		}
	}
	nowMs := now.UnixMilli()
	if oldestPaced > 0 && nowMs > oldestPaced {
		paced = nowMs - oldestPaced
	}
	if oldestUnresolved > 0 && nowMs > oldestUnresolved {
		unresolved = nowMs - oldestUnresolved
	}
	return paced, unresolved
}

func (s *Sidecar) edgeStatesLocked() []EdgeState {
	if s.mod == nil || !s.mod.handshaked {
		return nil
	}
	out := make([]EdgeState, 0, len(s.mod.exportEdges))
	for _, edge := range s.mod.exportEdges {
		open, reason, _ := s.edgeStateLocked(edge)
		st := EdgeState{
			Edge:   edge,
			Open:   open,
			Reason: reason,
			Usable: axisExists(s.mapShape, axisOfEdge(edge)),
		}
		if n := s.neighbours[edge]; n != nil {
			st.PeerSlot = n.Slot
		}
		out = append(out, st)
	}
	return out
}

func (s *Sidecar) peerStatesLocked() []PeerState {
	out := make([]PeerState, 0, len(s.status.Slots))
	for _, si := range s.status.Slots {
		p := PeerState{
			Slot:         si.Slot,
			PeerID:       si.PeerID,
			Col:          si.Position.Col,
			Row:          si.Position.Row,
			Me:           si.PeerID == s.cfg.PeerID,
			Live:         si.Live,
			ModConnected: si.ModConnected,
			GameVersion:  si.GameVersion,
			LastRefusal:  si.LastRefusal,
			DarkSinceMs:  si.DarkSinceMs,
		}
		if si.Stats != nil {
			p.ModVersion = si.Stats.ModVersion
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

func (s *Sidecar) wireStateLocked(now time.Time) WireState {
	w := WireState{
		MinContractVersion:   s.minContractVersion,
		PeakFramesPerSecond:  s.sent.peakFrames,
		PeakBytesPerSecond:   s.sent.peakBytes,
		LargestFrameBytes:    s.sent.largest,
		ClaimsLastMinute:     s.claims.count(now),
		PacedFramesPerSecond: s.sendPace.framesPerSecond(),
		PacedDeferrals:       s.sendPace.deferred,
		WarnFraction:         LimitWarnFraction,
	}
	if len(s.publishedLimits) > 0 {
		w.Limits = make(map[string]int64, len(s.publishedLimits))
		for k, v := range s.publishedLimits {
			w.Limits[k] = v
		}
	}
	return w
}

func (s *Sidecar) listenAddrLocked() string {
	if s.ln != nil {
		return s.ln.Addr().String()
	}
	return s.cfg.Listen
}

// ---------------------------------------------------------------- the server

// serveOwnSlot answers GET /my-slot with the view as JSON. It is read-only in
// the strongest available sense: it takes no parameters, it answers nothing but
// GET, and it touches no state at all.
func (s *Sidecar) serveOwnSlot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "the own-slot view is read-only", http.StatusMethodNotAllowed)
		return
	}
	v := s.OwnSlot()
	w.Header().Set("Content-Type", "application/json")
	// Nothing here is cacheable: every field is a reading of this instant.
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// ---------------------------------------------------------- the process record

// writeProcessRecord publishes this process's identity beside its address, so
// something outside it can tell a running sidecar from a stale record of one.
//
// It is written and never read by the sidecar itself. `--diagnose`'s
// stale-process check is the reader, and the two facts it needs are here for a
// reason: a pid ALONE is not evidence, because pid numbers are reused after a
// reboot, so the record carries the start time and the address as well and the
// check corroborates them against a process that actually answers.
func (s *Sidecar) writeProcessRecord(addr string) {
	rec := processRecord{
		PID:       os.Getpid(),
		StartedAt: s.startedAt.UnixMilli(),
		PeerID:    s.cfg.PeerID,
		Listen:    addr,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	// Best effort, and a failure is not fatal: a sidecar that cannot write this
	// file still runs, and the diagnostic then reports the absence honestly
	// rather than the sidecar refusing to start over a diagnostic aid.
	if err := os.WriteFile(filepath.Join(s.cfg.DataDir, processRecordName), append(b, '\n'), 0o644); err != nil {
		s.log.Debug("sidecar: could not write the process record", "err", err)
	}
}

func (s *Sidecar) removeProcessRecord() {
	if err := os.Remove(filepath.Join(s.cfg.DataDir, processRecordName)); err != nil &&
		!errors.Is(err, os.ErrNotExist) {
		s.log.Debug("sidecar: could not remove the process record", "err", err)
	}
}

type processRecord struct {
	PID       int    `json:"pid"`
	StartedAt int64  `json:"startedAtMs"`
	PeerID    string `json:"peerId"`
	Listen    string `json:"listen"`
}

func readProcessRecord(dataDir string) (processRecord, bool) {
	b, err := os.ReadFile(filepath.Join(dataDir, processRecordName))
	if err != nil {
		return processRecord{}, false
	}
	var rec processRecord
	if err := json.Unmarshal(b, &rec); err != nil || rec.PID <= 0 {
		return processRecord{}, false
	}
	return rec, true
}

// ---------------------------------------------------------------- the client

// ownSlotResult is what a fetch produced: the view, or the reason there is none.
// The REASON is load-bearing — "nothing is listening" and "something is
// listening that does not serve this view" are different facts with different
// remedies, and a diagnostic that collapsed them would tell a participant to
// start a sidecar that is already running.
type ownSlotResult struct {
	View OwnSlot
	OK   bool
	// Addr is where the fetch was attempted, and Why is the one-sentence reason
	// it did not produce a view.
	Addr string
	Why  string
	// Predates is set when something answered and did not know this path, which
	// is a sidecar older than this build.
	Predates bool
}

// fetchOwnSlot reads the view from a running sidecar's loopback endpoint.
//
// EVERY PROBE IS BOUNDED. A diagnostic that hangs is a diagnostic nobody runs
// twice, and the whole timeout budget belongs to the caller.
func fetchOwnSlot(dataDir string, timeout time.Duration) ownSlotResult {
	addr, err := readListenAddr(dataDir)
	if err != nil {
		return ownSlotResult{Why: "this data directory holds no listen.addr, so no sidecar has " +
			"served from it since it was created"}
	}
	res := ownSlotResult{Addr: addr}
	url := "http://" + addr + OwnSlotPath
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		res.Why = "nothing answered at " + addr + ": " + trimDialError(err)
		return res
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		res.Predates = true
		res.Why = "a sidecar is listening at " + addr + " and does not serve " + OwnSlotPath +
			", so it is a build older than this one"
		return res
	}
	if resp.StatusCode != http.StatusOK {
		res.Why = fmt.Sprintf("%s answered HTTP %d", addr, resp.StatusCode)
		return res
	}
	var v OwnSlot
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<22)).Decode(&v); err != nil {
		res.Why = "the answer from " + addr + " was not a readable own-slot view: " + err.Error()
		return res
	}
	res.View, res.OK = v, true
	return res
}

func readListenAddr(dataDir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dataDir, "listen.addr"))
	if err != nil {
		return "", err
	}
	addr := strings.TrimSpace(string(b))
	if addr == "" {
		return "", errors.New("empty listen.addr")
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return "", err
	}
	return addr, nil
}

// trimDialError keeps the part of a transport error a person can act on and
// drops the URL repetition in front of it.
func trimDialError(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ": "); i > 0 && strings.HasPrefix(msg, "Get ") {
		msg = msg[i+2:]
	}
	return termsafe.Text(msg)
}

// ---------------------------------------------------------------- the render

// RenderOwnSlot writes the participant's own-slot view as a terminal block.
//
// Every string it prints that this project did not author — a peer id, a game
// version, a close reason, a lastRefusal — goes through termsafe. They arrive
// from the relay and from other people's worlds, and a terminal is a rendering
// surface with its own injection story (contract-b-m4.md §22, B30).
func RenderOwnSlot(w io.Writer, v OwnSlot) {
	now := time.UnixMilli(v.ObservedAt)
	fmt.Fprintf(w, "YOUR WORLD, as your sidecar and the map see it — %s\n\n",
		now.Format("2006-01-02 15:04:05"))

	// 1. Am I on the map, and is the map talking to me.
	slotText := "none yet"
	if v.Slot.Slot > 0 {
		slotText = fmt.Sprintf("slot %d at column %d, row %d of a %dx%d map",
			v.Slot.Slot, v.Slot.Position.Col, v.Slot.Position.Row, v.Slot.MapWidth, v.Slot.MapHeight)
	}
	fmt.Fprintf(w, "  place        %s\n", slotText)
	fmt.Fprintf(w, "  peer         %s\n", termsafe.Text(v.PeerID))
	if v.Relay.Connected {
		fmt.Fprintf(w, "  map link     connected to %s\n", termsafe.Text(v.RelayURL))
	} else {
		fmt.Fprintf(w, "  map link     NOT CONNECTED to %s\n", termsafe.Text(v.RelayURL))
	}
	if v.Relay.LastFaultKind != "" {
		fmt.Fprintf(w, "               last refusal of this link: %s — %s\n",
			v.Relay.LastFaultKind, termsafe.Text(v.Relay.LastFaultReason))
	}
	if v.Slot.LastRefusal != "" {
		fmt.Fprintf(w, "  lastRefusal  %s\n", termsafe.Text(v.Slot.LastRefusal))
	} else {
		fmt.Fprintf(w, "  lastRefusal  none on your slot (a credential refusal and an eviction "+
			"never appear here)\n")
	}
	if v.Slot.GrantSeen {
		verdict := "granted"
		if !v.Slot.Granted {
			verdict = "REFUSED"
		}
		fmt.Fprintf(w, "  last claim   %s, reason %s\n", verdict, termsafe.Text(v.Slot.GrantReason))
	}

	// 2. Is a game behind it.
	fmt.Fprintln(w)
	if v.Mod.Connected {
		fmt.Fprintf(w, "  game         connected — %s, mod %s, %s\n",
			versionOrUnknown(v.Mod.GameVersion), versionOrUnknown(v.Mod.ModVersion),
			versionOrUnknown(v.Mod.ContractAVersion))
		if v.Mod.Population != nil {
			fmt.Fprintf(w, "  population   %d\n", *v.Mod.Population)
		}
		fmt.Fprintf(w, "  speed        %s\n", speedText(v.Mod))
	} else {
		fmt.Fprintf(w, "  game         NOT CONNECTED to this sidecar\n")
	}
	fmt.Fprintf(w, "  last save    %s\n", saveText(v.Mod, now))

	// 3. The lanes.
	fmt.Fprintln(w)
	if len(v.Edges) == 0 {
		fmt.Fprintf(w, "  lanes        none declared (no game is connected)\n")
	}
	for i, e := range v.Edges {
		label := "  lanes      "
		if i > 0 {
			label = "             "
		}
		state := "closed, " + termsafe.Text(e.Reason)
		if e.Open {
			state = fmt.Sprintf("open to slot %d", e.PeerSlot)
		} else if !e.Usable {
			state += " — this edge is on an axis this map does not have, and stays closed " +
				"for the life of this map shape"
		}
		fmt.Fprintf(w, "%s  %s  %s\n", label, e.Edge, state)
	}

	// 4. The queues, which nobody else can see.
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  in custody   %d\n", v.Custody.CustodyDepth)
	fmt.Fprintf(w, "  paced        %d waiting on your own delivery rate of %.1f per simulated minute%s\n",
		v.Custody.PacedDepth, v.Custody.InboundRatePerSimMinute, ageSuffix(v.Custody.OldestPacedAgeMs))
	fmt.Fprintf(w, "  pending ACK  %d delivered to your game, waiting to release sender custody%s\n",
		v.Custody.PendingAckDepth, ageSuffix(v.Custody.OldestPendingAckAgeMs))
	fmt.Fprintf(w, "  unresolved   %d forwarded once, waiting for an answer%s\n",
		v.Custody.UnresolvedDepth, ageSuffix(v.Custody.OldestUnresolvedAgeMs))
	if v.Custody.LostForwardTotal > 0 {
		fmt.Fprintf(w, "  lost         %d forwarded and never answered. migration is at-most-once:\n"+
			"               an organism that does not arrive is gone, not returned\n",
			v.Custody.LostForwardTotal)
	}
	if v.Custody.LateAckTotal > 0 {
		fmt.Fprintf(w, "  late answers %d arrived after their entry was written off; "+
			"raise --forward-timeout\n", v.Custody.LateAckTotal)
	}

	// 5. The neighbours — DQ8's row, from the machine that suffers for it.
	if len(v.Peers) > 1 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  the map      %-4s %-7s %-6s %-9s %s\n", "slot", "at", "live", "game?", "build")
		for _, p := range v.Peers {
			mark := "  "
			if p.Me {
				mark = "->"
			}
			live, game := "dark", "no"
			if p.Live {
				live = "live"
			}
			if p.ModConnected {
				game = "yes"
			}
			fmt.Fprintf(w, "            %s %-4d %-7s %-6s %-9s %s\n", mark, p.Slot,
				fmt.Sprintf("%d,%d", p.Col, p.Row), live, game,
				termsafe.Clip(versionOrUnknown(p.GameVersion), 24))
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Nothing above is a secret. Map fields are already public to every peer.\n"+
		"Queue details come only from this machine's journal and stay on this machine.\n"+
		"Run `multiverse-sidecar --diagnose` to have it judged.\n")
}

func versionOrUnknown(v string) string {
	if v == "" {
		return "version unknown"
	}
	return termsafe.Text(v)
}

func speedText(m ModState) string {
	applied := "unknown"
	if m.TimeScale != nil {
		applied = strconv.FormatFloat(*m.TimeScale, 'f', -1, 64)
	}
	if m.AchievedTimeScale == nil {
		return fmt.Sprintf("applied x%s, achieved not measurable yet", applied)
	}
	return fmt.Sprintf("applied x%s, achieved x%.1f over the last %s",
		applied, *m.AchievedTimeScale, (time.Duration(m.AchievedSpanMs) * time.Millisecond).Round(time.Second))
}

func saveText(m ModState, now time.Time) string {
	if m.SaveMinutes != nil && *m.SaveMinutes == 0 {
		return "this world's save timer is off (saveMinutes 0) — a reading, not a gap"
	}
	if m.LastSave == nil {
		return "unknown — no save receipt has arrived on this session"
	}
	age := now.Sub(time.UnixMilli(m.LastSave.AtMs)).Round(time.Second)
	text := fmt.Sprintf("%s ago", age)
	if m.LastSave.DurationMs > 0 {
		text += fmt.Sprintf(", took %d ms", m.LastSave.DurationMs)
	}
	if m.SaveMinutes != nil {
		text += fmt.Sprintf(", against a %g-minute interval", *m.SaveMinutes)
	}
	return text
}

func ageSuffix(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return fmt.Sprintf("; the oldest has waited %s",
		(time.Duration(ms) * time.Millisecond).Round(time.Second))
}
