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
	// StatusAgeMs is how old the PEER_STATUS behind this view is. A view with no
	// status at all reports HaveStatus false and nothing else is trustworthy.
	HaveStatus  bool                 `json:"haveStatus"`
	StatusAgeMs int64                `json:"statusAgeMs"`
	Epoch       int64                `json:"epoch"`
	Map         contractb.MapShape   `json:"map"`
	SlotCount   int                  `json:"slotCount"`
	Observers   int                  `json:"observers"`
	Slots       []SlotView           `json:"slots"`
	Holes       []contractb.Position `json:"holes"`
	Lanes       []LaneView           `json:"lanes"`
	Totals      Totals               `json:"totals"`
	Gaps        int                  `json:"genomeGaps"`
	Records     int                  `json:"ledgerRecords"`
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

	// StatsKnown is false when no stats block has arrived or the one that did is
	// older than statsStaleMs. Every stat below is then UNKNOWN, and the page
	// says so rather than showing a stale number as state.
	StatsKnown          bool                   `json:"statsKnown"`
	StatsAgeMs          int64                  `json:"statsAgeMs,omitempty"`
	Population          *int                   `json:"population,omitempty"`
	EggCount            *int                   `json:"eggCount,omitempty"`
	CustodyDepth        *int                   `json:"custodyDepth,omitempty"`
	PacedDepth          *int                   `json:"pacedDepth,omitempty"`
	HeldDepth           *int                   `json:"heldDepth,omitempty"`
	BouncedTimeoutTotal *int                   `json:"bouncedTimeoutTotal,omitempty"`
	SimulatedTime       *float64               `json:"simulatedTime,omitempty"`
	LastSave            *contracta.SaveReceipt `json:"lastSave,omitempty"`
	LastSaveAgeMs       *int64                 `json:"lastSaveAgeMs,omitempty"`
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
	LastAtMs   int64   `json:"lastAtMs,omitempty"`
}

// Totals are the map-wide numbers, each of which is a SUM OF KNOWN VALUES only.
type Totals struct {
	LiveSlots     int  `json:"liveSlots"`
	DarkSlots     int  `json:"darkSlots"`
	Holes         int  `json:"holes"`
	Population    *int `json:"population,omitempty"`
	CustodyDepth  *int `json:"custodyDepth,omitempty"`
	PacedDepth    *int `json:"pacedDepth,omitempty"`
	HeldDepth     *int `json:"heldDepth,omitempty"`
	TimeoutBounce *int `json:"timeoutBounces,omitempty"`
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
}

func (l *lane) observe(atMs int64) {
	l.total++
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
		Slots:          []SlotView{},
		Holes:          a.status.Holes(),
		Lanes:          []LaneView{},
		Gaps:           len(a.pending),
		Records:        a.recordCount,
	}
	if out.HaveStatus {
		out.StatusAgeMs = now.Sub(a.statusAt).Milliseconds()
	}
	out.Totals.Holes = len(out.Holes)

	popTotal, custody, paced, held, bounces := 0, 0, 0, 0, 0
	havePop, haveCustody, havePaced, haveHeld, haveBounce := false, false, false, false, false

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
				v.HeldDepth = si.Stats.HeldDepth
				v.BouncedTimeoutTotal = si.Stats.BouncedTimeoutTotal
				v.SimulatedTime = si.Stats.SimulatedTime
				if si.Stats.LastSave != nil {
					save := *si.Stats.LastSave
					v.LastSave = &save
					age := nowMs - save.AtMs
					v.LastSaveAgeMs = &age
				}
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
			if v.HeldDepth != nil {
				held += *v.HeldDepth
				haveHeld = true
			}
			if v.BouncedTimeoutTotal != nil {
				bounces += *v.BouncedTimeoutTotal
				haveBounce = true
			}
		}

		// Rule 2: the lanes are DERIVED, by §8's walk, from this very frame.
		for _, edge := range []string{contracta.EdgeE, contracta.EdgeN} {
			if !declares(si.ExportEdges, edge) {
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
			if l, ok := a.lanes[lanePair{si.Slot, lv.ToSlot}]; ok {
				lv.Migrations = l.total
				lv.PerMinute = l.perMinute(nowMs)
				lv.LastAtMs = l.lastAt
			}
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
	if haveHeld {
		out.Totals.HeldDepth = &held
	}
	if haveBounce {
		out.Totals.TimeoutBounce = &bounces
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

type lanePair struct{ from, to int }

func declares(edges []string, edge string) bool {
	for _, e := range edges {
		if e == edge {
			return true
		}
	}
	return false
}
