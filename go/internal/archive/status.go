package archive

// The operator surface of D15, and contract-b-m4.md §10.1's three rules.
//
//  1. ONE SOURCE, NO POLLING. Everything the page shows about the map comes
//     from the PEER_STATUS broadcasts the archive already receives as a
//     subscriber, plus the envelope copies it already records. The page never
//     connects to a sidecar, never reads another component's files, and never
//     asks the relay for anything. NOTHING ON THE MIGRATION PATH MAY EVER WAIT
//     FOR A READER (Risk 4).
//  2. DERIVED, AND MARKED AS DERIVED. The effective lanes and the bypasses are
//     recomputed by the walk of §8 from PEER_STATUS. They are the same
//     computation the relay performs, on the same inputs, and they are for
//     display: the relay's SECTOR_GRANT remains the authority for a peer's
//     actual routing, and where the two disagree the display is stale.
//  3. UNKNOWN IS A VALUE. A field absent from stats, a slot with no stats at
//     all, a statsAsOfMs older than statsStaleMs — every one of them renders as
//     unknown, never as zero and never as the last value seen without its age.
//     AN HONEST GAP BEATS A CONFIDENT ZERO.

import (
	"sort"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/mapwalk"
)

// Status is the whole operator view, and the JSON the page and ringstat both
// render. Every pointer field is nil when the value is UNKNOWN.
type Status struct {
	GeneratedAtMs  int64  `json:"generatedAtMs"`
	RelayConnected bool   `json:"relayConnected"`
	RelayURL       string `json:"relayUrl"`
	ArchivePeerID  string `json:"archivePeerId"`

	// BroadcastPeerID names the world the shared camera at /watch is pointed at,
	// as the DEPLOYMENT declares it. It is not a wire fact: no world announces
	// that somebody is filming it, and the archive is told rather than informed.
	// Empty is the honest answer for a map with no broadcast, and it is the
	// default: a page that guessed would be naming the wrong world.
	BroadcastPeerID string `json:"broadcastPeerId,omitempty"`
	// StatusAgeMs is how old the PEER_STATUS behind this view is. A view with no
	// status at all reports HaveStatus false and nothing else is trustworthy.
	HaveStatus  bool               `json:"haveStatus"`
	StatusAgeMs int64              `json:"statusAgeMs"`
	Epoch       int64              `json:"epoch"`
	Map         contractb.MapShape `json:"map"`
	SlotCount   int                `json:"slotCount"`
	Observers   int                `json:"observers"`
	// WHAT THE RELAY ITSELF IS RUNNING WITH, carried off the same PEER_STATUS
	// every slot below is (contract-b-m4.md §6.5; §22, B24 and B25). They are the
	// only values on this view that are AUTHORITATIVE RATHER THAN REPORTED: a
	// stats field is a peer's claim about itself, and these two are the relay's
	// own configuration, which changes only when the relay restarts. §10.1's B24
	// rule is why they are here at all — a peer's frame or claim rate is only
	// readable against the ceiling it is measured on, and a lastRefusal of
	// contract_version_below_minimum is only actionable beside the floor that
	// refused it.
	//
	// They age with the frame that carried them and have no clock of their own,
	// exactly as Epoch and Map do: StatusAgeMs dates all of it, and a view with
	// HaveStatus false carries neither.
	//
	// BOTH ARE omitempty AND THE TWO ABSENCES ARE DIFFERENT FACTS. An absent
	// Limits reads as UNKNOWN and never as "no ceilings" — the only relay it can
	// come from is one that predates B24. An absent MinContractVersion is the
	// relay's real answer rather than a gap: NO MINIMUM, which is B25's default
	// and the only honest one for a deployment that has made no floor decision.
	MinContractVersion string `json:"minContractVersion,omitempty"`
	// Limits is the published table exactly as it arrived, key for key: nothing
	// here renames a key, drops one this build has never heard of, or fills one
	// in from contractb's shipped defaults. The relay publishes what IT IS
	// RUNNING WITH, and a default substituted into that table would be the one
	// number on this page nobody could check.
	Limits  map[string]int64     `json:"limits,omitempty"`
	Slots   []SlotView           `json:"slots"`
	Holes   []contractb.Position `json:"holes"`
	Lanes   []LaneView           `json:"lanes"`
	Totals  Totals               `json:"totals"`
	Gaps    int                  `json:"genomeGaps"`
	Records int                  `json:"ledgerRecords"`
	// LedgerSkipped is how many ledger lines the startup replay could not parse
	// and read past — a record of what happened that the archive can no longer
	// account for. Omitted when 0, which is every healthy archive; it is here
	// rather than only in the startup log because the ledger is append-only, so
	// damage is PERMANENT and the operator who eventually asks why
	// `ledgerRecords` and `wc -l` disagree is not the operator who read the log.
	LedgerSkipped int `json:"ledgerSkippedLines,omitempty"`
	// The retention horizon of §23, B33/B34, and what it has done. All four are
	// omitted on an archive with no horizon, which is the default and is every
	// archive that has not been told one — an ABSENT genomeHorizonMs is how a
	// reader knows nothing is being pruned, and §10.1's rule that unknown is a
	// value is why it is absent rather than 0.
	//
	// GenomesEvicted and GenomesEvictedBytes are process totals like every other
	// counter here. GapsExpired counts gap entries retired from retry because
	// their crossing aged past the horizon: it is Risk 7's "record the
	// abandonment as a fact the operator can count", and it is the number that
	// says the fetch queue finally has a drain.
	GenomeHorizonMs     int64 `json:"genomeHorizonMs,omitempty"`
	GenomesEvicted      int   `json:"genomesEvicted,omitempty"`
	GenomesEvictedBytes int64 `json:"genomesEvictedBytes,omitempty"`
	GapsExpired         int   `json:"genomeGapsExpired,omitempty"`
	// THE ROLL-UP HONESTY FIELDS (rollup.go). They exist so a page can say WHICH
	// PART OF ITS ANSWER IS AGGREGATE AND WHICH IS RAW, which stops mattering
	// only if the raw record is kept forever — and the record roll-up is the
	// decision that it is not. THE NAMES ARE STABLE: pages, `monitor.sh` and the
	// M5 evidence record read them.
	//
	// RollupCoveredRecords is how many ledger records the ON-DISK roll-up state
	// covers as of its last successful save. A restart folds exactly these from
	// the sidecar and replays what is behind them. 0 means nothing has been
	// written yet, which is a brand-new archive and never a loss.
	//
	// RollupSavedAtMs is when that state was last written. A value that stops
	// advancing is a sidecar that is failing to save, and the counter beside it
	// will go on rising — which is exactly the shape an operator needs to see.
	//
	// The earliest RAW record still on this host is LedgerRawWindowFromMs, in
	// the segment block below — one fact, one name. The roll-up state keeps its
	// own rawFromMs in the sidecar's floor line, because that one is durable
	// state rather than a view.
	// ReplayRawRecords and ReplayRawSeconds are WHAT THE LAST START ACTUALLY
	// COST: how many raw records the restart parsed, and how long it took. They
	// are here because the whole roll-up rests on a claim about restart time —
	// the raw scan is min(age of the raw window, the duplicate window) of records
	// at the host's measured 62 000-72 000 a second — and a claim about restart
	// time that nobody measures is a claim nobody can check. An operator, and
	// monitor.sh, gate on the measurement rather than on the model.
	//
	// ReplayFromRetired is the one bad case, and it is loud: the saved cursor
	// named a raw segment that has left this host, so the aggregates have a hole
	// between the cursor and the oldest segment still here. It is absent on every
	// healthy archive.
	//
	// DuplicatesRefused is ALL-TIME and persisted (§25, B38): records the guard
	// refused as duplicates since this archive began. It is NOT omitempty,
	// deliberately — 0 is the answer that matters, because it is what says the
	// fleet has crossed and the duplicate window may safely come down. A refusal
	// leaves no other trace anywhere: nothing is appended, and the record is by
	// construction the record without it.
	// THE TWO FLOORS, and they are two on purpose. LedgerFirstRecordMs is where
	// the AGGREGATES reach back to — the first record this archive ever folded,
	// which is kept forever — and LedgerRawWindowFromMs below is where the RAW
	// LINES reach back to, which is the window. AncestrySinceMs is the third,
	// narrower one: the oldest crossing that names a parent, which is what the
	// genealogy can draw from.
	//
	// A page that has only one of them has to guess which it is, and the guess a
	// reader makes is "the record starts here" — which after the roll-up is true
	// of the first and false of the second. `/api/species/tree` has published
	// ancestrySinceMs all along; it is repeated here so one request answers
	// "what can this archive still say, and about what period".
	LedgerFirstRecordMs  int64   `json:"ledgerFirstRecordMs,omitempty"`
	AncestrySinceMs      int64   `json:"ancestrySinceMs,omitempty"`
	RollupCoveredRecords int     `json:"rollupCoveredRecords"`
	RollupSavedAtMs      int64   `json:"rollupSavedAtMs"`
	ReplayRawRecords     int     `json:"replayRawRecords"`
	ReplayRawSeconds     float64 `json:"replayRawSeconds"`
	ReplayFromRetired    bool    `json:"replayFromRetired,omitempty"`
	DuplicatesRefused    int     `json:"duplicatesRefused"`
	// THE SEGMENTED LEDGER (segments.go). These five say what the archive still
	// holds the RAW LINES for, which is a different and smaller thing than what
	// it can still answer: every aggregate is kept forever, and these describe
	// the file layer under it.
	//
	// LedgerSegments is closed segments PRESENT on this host, live file
	// excluded. LedgerRawBytes is what they occupy plus the live file, as stored
	// — so a compressed segment counts its compressed size, because that is the
	// number the volume cares about.
	//
	// LedgerRawWindowFromMs IS THE HONESTY FIELD. It is the start of the oldest
	// raw record still on this host, and it is what a reader needs in order to
	// know that "no crossings before this" means "none recorded" rather than
	// "none kept". It is present even when the window is off, because it is a
	// fact about the disk rather than about the policy.
	//
	// LedgerSegmentsAwaitingColdCopy is segments past the window with no usable
	// off-host receipt. IT IS THE NUMBER THAT SHOULD BE ZERO: a segment is never
	// removed without one, so a cold archive that has stopped working shows up
	// here and in disk usage, and never as a record that is gone. LedgerRetired
	// is what this process removed after confirming a copy.
	LedgerSegments                 int   `json:"ledgerSegments"`
	LedgerRawBytes                 int64 `json:"ledgerRawBytes"`
	LedgerRawWindowFromMs          int64 `json:"ledgerRawWindowFromMs,omitempty"`
	LedgerWindowMs                 int64 `json:"ledgerWindowMs,omitempty"`
	LedgerSegmentsAwaitingColdCopy int   `json:"ledgerSegmentsAwaitingColdCopy"`
	LedgerRetired                  int   `json:"ledgerRetiredTotal"`
	LedgerRetiredBytes             int64 `json:"ledgerRetiredBytes,omitempty"`
	// FlowWindowMs is the span LaneView.RecentHops and PerMinute are measured
	// over. A rate with no window on it is not a measurement.
	FlowWindowMs int64 `json:"flowWindowMs"`
	// AchievedWindowMs is the longest span SlotView.AchievedTimeScale looks
	// back over. Each slot also reports the span its own reading actually used,
	// which is shorter while a window is still filling.
	AchievedWindowMs int64 `json:"achievedWindowMs"`
}

// SlotView is one reserved slot as the page renders it.
type SlotView struct {
	Slot           int                `json:"slot"`
	Position       contractb.Position `json:"position"`
	PeerID         string             `json:"peerId"`
	Live           bool               `json:"live"`
	ModConnected   bool               `json:"modConnected"`
	GameVersion    string             `json:"gameVersion"`
	SimulationSize float64            `json:"simulationSize"`
	ExportEdges    []string           `json:"exportEdges"`
	// DarkForMs is Risk 5's field: a healed map hides a dead world, and
	// "bypassed since 04:12" is what stops an operator missing it for a day.
	DarkSinceMs int64  `json:"darkSinceMs,omitempty"`
	DarkForMs   int64  `json:"darkForMs,omitempty"`
	LastRefusal string `json:"lastRefusal,omitempty"`

	// Broadcast is true for the one world the deployment says /watch is showing.
	// It is a DEPLOYMENT claim, not this world's own (see BroadcastPeerID), and
	// it changes nothing about how the world is treated on the map.
	Broadcast bool `json:"broadcast,omitempty"`

	// StatsKnown is false when no stats block has arrived or the one that did is
	// older than statsStaleMs. Every stat below is then UNKNOWN, and the page
	// says so rather than showing a stale number as state.
	StatsKnown   bool  `json:"statsKnown"`
	StatsAgeMs   int64 `json:"statsAgeMs,omitempty"`
	Population   *int  `json:"population,omitempty"`
	EggCount     *int  `json:"eggCount,omitempty"`
	CustodyDepth *int  `json:"custodyDepth,omitempty"`
	PacedDepth   *int  `json:"pacedDepth,omitempty"`
	// LostForwardTotal is what migration costs on this map: organisms this peer
	// forwarded once and never heard an answer for (§6.3.1, §25 B37). `heldDepth`
	// and `bouncedTimeoutTotal` stood here; they went with the bounded hold, and
	// a peer old enough to still send them has its values carried as unknown keys
	// and read by nobody.
	LostForwardTotal *int                   `json:"lostForwardTotal,omitempty"`
	SimulatedTime    *float64               `json:"simulatedTime,omitempty"`
	LastSave         *contracta.SaveReceipt `json:"lastSave,omitempty"`
	LastSaveAgeMs    *int64                 `json:"lastSaveAgeMs,omitempty"`

	// How fast the world runs, and the cap on how fast organisms are let into it
	// (contract-b-m4.md §18, B16). TimeScale comes from the mod's HEARTBEAT;
	// the two pacing numbers are the sidecar's own configuration. All three are
	// nil when the peer publishes them and this view has not heard, and nil when
	// the peer is an older build that does not publish them at all — which is
	// the case this rig actually runs, and the page renders it as unknown rather
	// than as the shipped default (which has moved three times).
	TimeScale                *float64 `json:"timeScale,omitempty"`
	InboundRatePerSimMinute  *float64 `json:"inboundRatePerSimMinute,omitempty"`
	InboundRateBurst         *float64 `json:"inboundRateBurst,omitempty"`
	TargetTimeScale          *float64 `json:"targetTimeScale,omitempty"`
	AdmissionMode            string   `json:"admissionMode,omitempty"`
	AdmissionTargetTimeScale *float64 `json:"admissionTargetTimeScale,omitempty"`
	AdmissionPopulationLimit *int     `json:"admissionPopulationLimit,omitempty"`
	AdmissionEstimatedLimit  *int     `json:"admissionEstimatedLimit,omitempty"`
	AdmissionCommitted       *int     `json:"admissionCommitted,omitempty"`
	AdmissionClosed          *bool    `json:"admissionClosed,omitempty"`
	AdmissionEnforcing       *bool    `json:"admissionEnforcing,omitempty"`
	AdmissionSampleCount     *int     `json:"admissionSampleCount,omitempty"`
	AdmissionRejectedTotal   *int     `json:"admissionRejectedTotal,omitempty"`

	// AchievedTimeScale is what this world's clock ACTUALLY did — simulated
	// seconds per wall second, measured by the archive over AchievedSpanMs from
	// the (statsAsOfMs, simulatedTime) pairs it already receives. TimeScale
	// beside it is what the game reports APPLYING, and the two are different
	// questions: at a target the host cannot meet a world reports a clean 100
	// and advances 5. Neither is derived from the other and neither replaces
	// the other, which is why both are here.
	//
	// It is DERIVED (§10.1 rule 2) and it obeys rule 3 without exception: nil
	// whenever the archive has not watched two comparable samples ten seconds
	// apart inside the last minute — at startup, across a mod-quiet gap, or
	// after a seat changed worlds. The page then shows the applied value ALONE.
	// See achieved.go for every guard and its reason.
	AchievedTimeScale *float64 `json:"achievedTimeScale,omitempty"`
	// AchievedSpanMs is the span AchievedTimeScale was measured over. A rate
	// with no window on it is not a measurement — the same rule FlowWindowMs
	// exists for. It is present exactly when AchievedTimeScale is.
	AchievedSpanMs int64 `json:"achievedSpanMs,omitempty"`

	// The species census (contract-b-m4.md §16, B11 and B12). SpeciesKnown is
	// the ABSENT/EMPTY distinction made explicit for a JSON reader, because a
	// JS client cannot tell an omitted array from an empty one once it has
	// parsed: false means UNKNOWN — no census on the block, or a block too old
	// to be state — and the page renders "species unknown", never "no species",
	// never an empty list and never a zero. True with an empty Species is the
	// stronger, different fact: a reporting world with nothing alive in it.
	//
	// The entries are carried in the census's own order, which is the mod's:
	// sorted by bibites + eggs descending. NOTHING HERE RE-SORTS, MERGES,
	// DE-DUPLICATES OR REPAIRS A NAME (contract-a.md §17, A36) — two entries
	// whose names differ only in whitespace are two Species records in that
	// world, and this view reports two.
	SpeciesKnown bool                    `json:"speciesKnown"`
	Species      []contractb.CensusEntry `json:"species,omitempty"`
	// SpeciesTruncated says the 32 most abundant species are named and the rest
	// is UNREPORTED. The page must say so rather than present the list as whole.
	SpeciesTruncated bool `json:"speciesTruncated,omitempty"`

	// WHAT THIS WORLD WAS TOLD TO DO (contract-b-m4.md §19, B18 and B19).
	// Carried through UNTOUCHED — the archive's whole obligation here is to
	// render an absent value as unknown and to never send one back — and each
	// of them sits beside the number it explains: the exclusion list beside the
	// census that holds the excluded species, SaveMinutes beside LastSave,
	// ContractAVersion beside whichever field group that mod is too old to send.
	ModVersion       string `json:"modVersion,omitempty"`
	ContractAVersion string `json:"contractAVersion,omitempty"`
	// MigrationExcludeKnown is the ABSENT/EMPTY distinction made explicit for a
	// JSON reader, for the same reason SpeciesKnown is: a JS client cannot tell
	// an omitted array from an empty one once it has parsed. false means UNKNOWN
	// — no list on the block, or a block too old to be state — and true with an
	// empty MigrationExclude is the stronger, different fact that the origin
	// mod has the exclusion policy switched OFF.
	MigrationExcludeKnown bool     `json:"migrationExcludeKnown"`
	MigrationExclude      []string `json:"migrationExclude,omitempty"`
	// SaveMinutes of 0 is a save timer that is OFF, and it is the explanation
	// for a world with no LastSave. IT MUST NEVER RENDER AS 10, which is what
	// the mod ships with: that would claim a world is being saved when its timer
	// may be off, the most expensive wrong number this page could print.
	SaveMinutes   *float64 `json:"saveMinutes,omitempty"`
	SaveKeep      *int     `json:"saveKeep,omitempty"`
	SaveOnQuit    *bool    `json:"saveOnQuit,omitempty"`
	WorldWrapping *bool    `json:"worldWrapping,omitempty"`
}

// LaneView is one derived effective lane, with the flow the ledger measured on
// it.
type LaneView struct {
	Edge     string `json:"edge"`
	FromSlot int    `json:"fromSlot"`
	// ToSlot is 0 when the axis has no deliverable target, and Reason then says
	// which EDGE_STATUS reason §8 maps the skips into.
	ToSlot  int              `json:"toSlot"`
	Open    bool             `json:"open"`
	Reason  string           `json:"reason"`
	Skipped []contractb.Skip `json:"skipped"`
	// Migrations is every envelope the archive was copied on this lane, and
	// PerMinute is the rate over the last flowWindow.
	Migrations int     `json:"migrations"`
	PerMinute  float64 `json:"perMinute"`
	// RecentHops is how many of those envelopes landed inside flowWindow: the
	// raw count behind PerMinute, over a window a reader can see, rather than a
	// rate they have to take on trust. The alternative — shipping the ledger to
	// the browser and letting it count — would put the migration record on the
	// wire for a picture. Migrations is cumulative and monotonic, so a reader
	// that polls can also difference it to see a hop ARRIVE, which is what the
	// page falls back to when the hop feed is unreachable.
	RecentHops int   `json:"recentHops"`
	LastAtMs   int64 `json:"lastAtMs,omitempty"`
}

// Totals are the map-wide numbers, each of which is a SUM OF KNOWN VALUES only.
type Totals struct {
	LiveSlots    int  `json:"liveSlots"`
	DarkSlots    int  `json:"darkSlots"`
	Holes        int  `json:"holes"`
	Population   *int `json:"population,omitempty"`
	CustodyDepth *int `json:"custodyDepth,omitempty"`
	PacedDepth   *int `json:"pacedDepth,omitempty"`
	LostForward  *int `json:"lostForwards,omitempty"`
	// UnknownSlots is how many slots contributed nothing to the sums above.
	UnknownSlots int     `json:"unknownSlots"`
	Migrations   int     `json:"migrations"`
	PerMinute    float64 `json:"perMinute"`
}

// flowWindow is how far back the per-minute lane rate looks.
const flowWindow = 5 * time.Minute

// lane is the archive's in-memory flow counter for one ordered slot pair.
type lane struct {
	total  int
	recent []int64 // recordedAt milliseconds, bounded to flowWindow
	lastAt int64
	// firstAt is the recordedAt of the FIRST crossing this archive was copied on
	// this lane. Nothing renders it, and it is here because a rate without a span
	// is not a measurement: M5's per-lane flow evidence is total/(last-first), and
	// once the raw record has a window that span cannot be recovered from the
	// records still on the host. It is persisted with the rest of the fold
	// (rollup.go) for the same reason the ancestry floor is.
	firstAt int64
}

func (l *lane) observe(atMs int64) {
	l.total++
	if atMs > 0 && (l.firstAt == 0 || atMs < l.firstAt) {
		l.firstAt = atMs
	}
	l.lastAt = atMs
	l.recent = append(l.recent, atMs)
	l.trim(atMs)
}

func (l *lane) trim(nowMs int64) {
	cut := nowMs - flowWindow.Milliseconds()
	i := 0
	for i < len(l.recent) && l.recent[i] < cut {
		i++
	}
	if i > 0 {
		l.recent = append(l.recent[:0], l.recent[i:]...)
	}
}

func (l *lane) perMinute(nowMs int64) float64 {
	l.trim(nowMs)
	if len(l.recent) == 0 {
		return 0
	}
	return float64(len(l.recent)) / flowWindow.Minutes()
}

// recentHops is how many envelopes are still inside flowWindow.
func (l *lane) recentHops(nowMs int64) int {
	l.trim(nowMs)
	return len(l.recent)
}

// publishedLimits copies §3.3's table out of the frame it arrived on.
//
// It COPIES because this view outlives the lock and a later PEER_STATUS replaces
// the frame under it, and it copies rather than interprets: every key the relay
// published survives, including one this build has never heard of, because the
// archive renders the published table and does not restate a table of its own.
//
// An empty object is treated as no object. §3.3 is explicit that a published
// table with a hole in it is not a table, so `{}` is UNKNOWN — the same reading
// as absence, and still never "no ceilings".
func publishedLimits(src map[string]int64) map[string]int64 {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]int64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// StatusView builds the whole operator view. It holds the archive's lock for the
// duration and touches nothing on the migration path.
func (a *Archive) StatusView() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	nowMs := now.UnixMilli()
	out := Status{
		GeneratedAtMs:  nowMs,
		RelayConnected: a.ready,
		RelayURL:       a.cfg.RelayURL,
		ArchivePeerID:  a.cfg.PeerID,
		HaveStatus:     a.statusAt.After(time.Time{}),
		Epoch:          a.status.Epoch,
		Map:            a.status.Map,
		SlotCount:      a.status.SlotCount,
		Observers:      a.status.Observers,
		// The relay's own two published values, carried straight off the frame.
		// NOTHING IS REMEMBERED ACROSS FRAMES here, which is where this differs
		// from the sidecar's adoption of the same table: a sidecar PACES itself
		// from the ceiling and must not lose one mid-session, while this view only
		// renders the last broadcast, and a view that stitched two frames together
		// would be showing a table beside a map that no single frame ever carried.
		MinContractVersion: a.status.MinContractVersion,
		Limits:             publishedLimits(a.status.Limits),
		Slots:              []SlotView{},
		Holes:              a.status.Holes(),
		Lanes:              []LaneView{},
		Gaps:               len(a.pending),
		Records:            a.recordCount,
		LedgerSkipped:      a.ledgerSkipped,
		FlowWindowMs:       flowWindow.Milliseconds(),

		LedgerFirstRecordMs:  a.tally.firstMs,
		AncestrySinceMs:      a.species.edgeFirstMs,
		RollupCoveredRecords: int(a.rollupCovered.Record),
		RollupSavedAtMs:      a.rollupSavedAtMs,
		ReplayRawRecords:     a.replayRawRecords,
		ReplayRawSeconds:     a.replayRawSeconds,
		ReplayFromRetired:    a.replayFromRetired,
		DuplicatesRefused:    a.duplicatesRefused,

		GenomeHorizonMs:     a.cfg.GenomeHorizon.Milliseconds(),
		GenomesEvicted:      a.evict.evicted,
		GenomesEvictedBytes: a.evict.bytes,
		GapsExpired:         a.evict.gapsExpired,

		LedgerSegments:                 a.seg.segments,
		LedgerRawBytes:                 a.seg.rawBytes,
		LedgerRawWindowFromMs:          a.seg.windowFromMs,
		LedgerWindowMs:                 a.ledgerWindow().Milliseconds(),
		LedgerSegmentsAwaitingColdCopy: a.seg.awaitingColdCopy,
		LedgerRetired:                  a.seg.retired,
		LedgerRetiredBytes:             a.seg.retiredBytes,

		AchievedWindowMs: achievedWindow.Milliseconds(),

		BroadcastPeerID: a.cfg.BroadcastPeerID,
	}
	if out.HaveStatus {
		out.StatusAgeMs = now.Sub(a.statusAt).Milliseconds()
	}
	out.Totals.Holes = len(out.Holes)

	popTotal, custody, paced, lost := 0, 0, 0, 0
	havePop, haveCustody, havePaced, haveLost := false, false, false, false

	for _, si := range a.status.Slots {
		v := SlotView{
			Slot:           si.Slot,
			Position:       si.Position,
			PeerID:         si.PeerID,
			Live:           si.Live,
			ModConnected:   si.ModConnected,
			GameVersion:    si.GameVersion,
			SimulationSize: si.SimulationSize,
			ExportEdges:    si.ExportEdges,
			LastRefusal:    si.LastRefusal,
			Broadcast:      a.cfg.BroadcastPeerID != "" && si.PeerID == a.cfg.BroadcastPeerID,
		}
		if v.ExportEdges == nil {
			v.ExportEdges = []string{}
		}
		if si.Live {
			out.Totals.LiveSlots++
		} else {
			out.Totals.DarkSlots++
			if si.DarkSinceMs > 0 {
				v.DarkSinceMs = si.DarkSinceMs
				v.DarkForMs = nowMs - si.DarkSinceMs
			}
		}
		// Rule 3. A stats block older than statsStaleMs is history, not state.
		if si.Stats != nil && si.StatsAsOfMs > 0 {
			age := nowMs - si.StatsAsOfMs
			v.StatsAgeMs = age
			if age <= a.cfg.StatsStale.Milliseconds() {
				v.StatsKnown = true
				v.Population = si.Stats.Population
				v.EggCount = si.Stats.EggCount
				v.CustodyDepth = si.Stats.CustodyDepth
				v.PacedDepth = si.Stats.PacedDepth
				v.LostForwardTotal = si.Stats.LostForwardTotal
				v.SimulatedTime = si.Stats.SimulatedTime
				v.TimeScale = si.Stats.TimeScale
				// The measured rate ages with the block that carries the
				// applied one: a fresh measurement beside a stale timeScale
				// would be two ages under one timestamp, which is the same
				// mistake the census and the settings are kept out of.
				if r := a.simRates[si.Slot]; r != nil {
					if achieved, span, ok := r.rate(nowMs); ok {
						v.AchievedTimeScale = &achieved
						v.AchievedSpanMs = span
					}
				}
				v.InboundRatePerSimMinute = si.Stats.InboundRatePerSimMinute
				v.InboundRateBurst = si.Stats.InboundRateBurst
				v.TargetTimeScale = si.Stats.TargetTimeScale
				v.AdmissionMode = si.Stats.AdmissionMode
				v.AdmissionTargetTimeScale = si.Stats.AdmissionTargetTimeScale
				v.AdmissionPopulationLimit = si.Stats.AdmissionPopulationLimit
				v.AdmissionEstimatedLimit = si.Stats.AdmissionEstimatedLimit
				v.AdmissionCommitted = si.Stats.AdmissionCommitted
				v.AdmissionClosed = si.Stats.AdmissionClosed
				v.AdmissionEnforcing = si.Stats.AdmissionEnforcing
				v.AdmissionSampleCount = si.Stats.AdmissionSampleCount
				v.AdmissionRejectedTotal = si.Stats.AdmissionRejectedTotal
				if si.Stats.LastSave != nil {
					save := *si.Stats.LastSave
					v.LastSave = &save
					age := nowMs - save.AtMs
					v.LastSaveAgeMs = &age
				}
				// The census ages with the rest of the block and needs no clock
				// of its own: giving it one would let the page show a fresh
				// species list beside a stale population from the same frame
				// (§16, B11). Absent stays absent — SpeciesKnown is only true
				// when a census actually arrived.
				if si.Stats.Species != nil {
					v.SpeciesKnown = true
					v.Species = append([]contractb.CensusEntry{},
						si.Stats.Species.Entries...)
					v.SpeciesTruncated = bool(si.Stats.Truncated)
				}
				// The settings age with the rest of the block for the same
				// reason the census does: one stats block carries one
				// statsAsOfMs, and a fresh exclusion list beside a stale
				// population from the same frame would be two ages under one
				// timestamp. Absent stays absent all the way here — nothing
				// below fills one in, and there is no default in this chain to
				// fill one in FROM, which is why no configuration can produce a
				// wrong setting on the page (§19, B19).
				v.ModVersion = si.Stats.ModVersion
				v.ContractAVersion = si.Stats.ContractAVersion
				if si.Stats.MigrationExclude != nil {
					v.MigrationExcludeKnown = true
					v.MigrationExclude = append([]string{},
						si.Stats.MigrationExclude.Names...)
				}
				v.SaveMinutes = si.Stats.SaveMinutes
				v.SaveKeep = si.Stats.SaveKeep
				v.SaveOnQuit = si.Stats.SaveOnQuit
				v.WorldWrapping = si.Stats.WorldWrapping
			}
		}
		if !v.StatsKnown {
			out.Totals.UnknownSlots++
		} else {
			if v.Population != nil {
				popTotal += *v.Population
				havePop = true
			}
			if v.CustodyDepth != nil {
				custody += *v.CustodyDepth
				haveCustody = true
			}
			if v.PacedDepth != nil {
				paced += *v.PacedDepth
				havePaced = true
			}
			if v.LostForwardTotal != nil {
				lost += *v.LostForwardTotal
				haveLost = true
			}
		}

		// Rule 2: the lanes are DERIVED, by §8's walk, from this very frame — and
		// under two-way lanes there are FOUR walks per slot, not two (§17, B13).
		// The archive reproduces them itself rather than reading them off a grant,
		// because a subscriber gets no grant; §10.1 requires exactly this and B13
		// names the archive as one of the parties that must now run four.
		for _, edge := range contracta.CanonicalEdges() {
			if !declares(si.ExportEdges, edge) {
				// A slot that declared two edges is a two-edge world on a two-way
				// map: it receives from all four sides and exports to two, and the
				// page draws it with two dead lanes rather than inventing them.
				continue
			}
			lv := LaneView{Edge: edge, FromSlot: si.Slot, Skipped: []contractb.Skip{}}
			target, skipped, found := mapwalk.Walk(a.status, si, edge)
			lv.Skipped = skipped
			if found {
				lv.Open = true
				lv.ToSlot = target.Slot
				lv.Reason = contracta.ReasonPeerLive
			} else {
				lv.Reason = mapwalk.EdgeReason(skipped)
			}
			a.fillFlowLocked(&lv, si.Slot, edge, nowMs)
			out.Lanes = append(out.Lanes, lv)
		}
		out.Slots = append(out.Slots, v)
	}

	if havePop {
		out.Totals.Population = &popTotal
	}
	if haveCustody {
		out.Totals.CustodyDepth = &custody
	}
	if havePaced {
		out.Totals.PacedDepth = &paced
	}
	if haveLost {
		out.Totals.LostForward = &lost
	}
	for _, l := range a.lanes {
		out.Totals.Migrations += l.total
		out.Totals.PerMinute += l.perMinute(nowMs)
	}
	sort.SliceStable(out.Lanes, func(i, j int) bool {
		if out.Lanes[i].FromSlot != out.Lanes[j].FromSlot {
			return out.Lanes[i].FromSlot < out.Lanes[j].FromSlot
		}
		return out.Lanes[i].Edge < out.Lanes[j].Edge
	})
	return out
}

// lanePair keys the flow counters. The EDGE is part of the key because a
// directed lane is what carries a flow, and two-way lanes made (from, to)
// ambiguous: on an axis of length 2 the forward and reverse lanes join the same
// two worlds (§17, B13). Records written before D17 carry edge "".
type lanePair struct {
	from, to int
	edge     string
}

// fillFlowLocked puts one directed lane's measured flow onto its view.
//
// It also carries the pre-D17 ledger forward, which is the one place the new
// key has to answer for the old one. A record written before the edge was
// recorded sits under edge "", and it can be re-attributed EXACTLY because the
// only export edges that existed then were E and N — and E and N can never
// resolve to the same target, since (col+1,row) and (col,row+1) are different
// positions and no slot occupies two of them. So a legacy bucket belongs to at
// most one of the four lanes drawn today, and adding it double-counts nothing.
// W and S never have one.
func (a *Archive) fillFlowLocked(lv *LaneView, from int, edge string, nowMs int64) {
	keys := []lanePair{{from: from, to: lv.ToSlot, edge: edge}}
	if edge == contracta.EdgeE || edge == contracta.EdgeN {
		keys = append(keys, lanePair{from: from, to: lv.ToSlot, edge: ""})
	}
	for _, k := range keys {
		l, ok := a.lanes[k]
		if !ok {
			continue
		}
		lv.Migrations += l.total
		lv.PerMinute += l.perMinute(nowMs)
		lv.RecentHops += l.recentHops(nowMs)
		if l.lastAt > lv.LastAtMs {
			lv.LastAtMs = l.lastAt
		}
	}
}

func declares(edges []string, edge string) bool {
	for _, e := range edges {
		if e == edge {
			return true
		}
	}
	return false
}
