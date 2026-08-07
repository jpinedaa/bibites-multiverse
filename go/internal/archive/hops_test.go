package archive

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
)

// slot4 is `slot` with all four export edges declared, which is what a
// conformant mod reports under D17 (contract-a.md §18, A38).
func slot4(n, col, row int, live bool, stats *contractb.PeerStats) contractb.SlotInfo {
	si := slot(n, col, row, live, stats)
	si.ExportEdges = contracta.CanonicalEdges()
	return si
}

// TestHopFeedIsBoundedInTimeAndCount is contract-b-m4.md §17, B14's "bounded
// twice" row: BY TIME AND BY COUNT, both, not either.
//
// The two bounds have different jobs and the test asserts both separately.
// Time is what stops a quiet archive holding yesterday's crossings. Count is
// what stops a genuine dam — a far-end dropout releasing a two-hour backlog —
// putting thousands of entries into one HTTP response.
func TestHopFeedIsBoundedInTimeAndCount(t *testing.T) {
	a := newViewFixture(t, contractb.PeerStatus{}, time.Second)
	now := time.Now().UnixMilli()

	a.mu.Lock()
	// Inside the window but far more than the count bound allows, in arrival
	// order — which is what a dam releasing a backlog actually looks like.
	const burst = hopMax + 50
	for i := 0; i < burst; i++ {
		a.observeHopLocked(Hop{MigrationID: "burst-" + itoa(i), AtMs: now - int64(burst-i),
			FromSlot: 1, ToSlot: 2, ExitEdge: contracta.EdgeE})
	}
	a.mu.Unlock()

	feed := a.HopFeedView()
	if len(feed.Hops) != hopMax {
		t.Fatalf("the feed holds %d entries, want the count bound %d", len(feed.Hops), hopMax)
	}
	if !feed.Truncated {
		t.Fatal("the count bound fired and the feed did not say so; a feed whose limits a " +
			"reader cannot see is a feed a reader will mistake for a total")
	}
	if feed.WindowMs != hopWindow.Milliseconds() || feed.MaxEntries != hopMax {
		t.Fatalf("the feed publishes window %d / max %d, want %d / %d",
			feed.WindowMs, feed.MaxEntries, hopWindow.Milliseconds(), hopMax)
	}
	// It kept the NEWEST. An animation of what just happened is worthless if the
	// entries it drops are the recent ones.
	if feed.Hops[0].MigrationID != "burst-50" ||
		feed.Hops[len(feed.Hops)-1].MigrationID != "burst-"+itoa(burst-1) {
		t.Fatalf("the feed spans %q..%q; the count bound must drop the OLDEST",
			feed.Hops[0].MigrationID, feed.Hops[len(feed.Hops)-1].MigrationID)
	}

	// The time bound, on its own: a handful of entries, all older than the
	// window, is an EMPTY feed and not a stale one.
	b := newViewFixture(t, contractb.PeerStatus{}, time.Second)
	b.mu.Lock()
	for i := 0; i < 5; i++ {
		b.observeHopLocked(Hop{MigrationID: "old-" + itoa(i),
			AtMs: now - 2*hopWindow.Milliseconds() - int64(i), FromSlot: 1, ToSlot: 2,
			ExitEdge: contracta.EdgeE})
	}
	b.observeHopLocked(Hop{MigrationID: "fresh", AtMs: now, FromSlot: 1, ToSlot: 2,
		ExitEdge: contracta.EdgeW})
	b.mu.Unlock()

	fresh := b.HopFeedView()
	if len(fresh.Hops) != 1 || fresh.Hops[0].MigrationID != "fresh" {
		t.Fatalf("the feed holds %+v; everything older than %v must be gone",
			fresh.Hops, hopWindow)
	}
	// And it is never nil: an empty feed is an empty array, not a null, because
	// a JS client cannot tell an omitted array from an empty one once parsed.
	c := newViewFixture(t, contractb.PeerStatus{}, time.Second)
	if body, err := json.Marshal(c.HopFeedView()); err != nil ||
		!strings.Contains(string(body), `"hops":[]`) {
		t.Fatalf("an empty feed serialized as %s (err %v)", body, err)
	}
}

// TestHopFeedIsNotInTheMetricsMarshal is the design decision B14 forces and the
// reason the feed rides its own endpoint.
//
// §17 B14: "it must be bounded in BOTH time and count, because the status view
// is serialized verbatim into the durable metrics file once a minute". This
// archive answers that with the stronger form — the feed is not on the status
// view at all — so no bound of any size can put a per-organism record into a
// file that is appended forever and never rewritten. The ledger already holds
// every one of those hops, durably, and holds it once.
func TestHopFeedIsNotInTheMetricsMarshal(t *testing.T) {
	stats := &contractb.PeerStats{Population: contractb.IntPtr(4)}
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{slot4(1, 0, 0, true, stats), slot4(2, 1, 0, true, stats)},
	}
	a := newViewFixture(t, status, time.Second)
	now := time.Now().UnixMilli()
	a.mu.Lock()
	a.observeHopLocked(Hop{MigrationID: "m-visible-in-hops-only", AtMs: now,
		FromSlot: 1, ToSlot: 2, ExitEdge: contracta.EdgeE,
		Species: &contractb.Species{GenericName: "Izus", SpecificName: "copedylanus"}})
	a.mu.Unlock()

	view := a.StatusView()
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"m-visible-in-hops-only", "copedylanus", `"hops"`} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("the status view carries %q; it is written verbatim into metrics.jsonl "+
				"every minute and MUST stay lean", forbidden)
		}
	}
	// A round trip through the durable file is the same statement from the other
	// side: what is replayed is what was written, and no hop was.
	if err := a.metrics.Append(view); err != nil {
		t.Fatal(err)
	}
	raw, err := ReadMetrics(a.MetricsPath())
	if err != nil || len(raw) != 1 {
		t.Fatalf("metrics replay gave %d samples (err %v)", len(raw), err)
	}
	// And the feed itself still has it: excluding it from the sample is not
	// dropping it.
	if got := a.HopFeedView(); len(got.Hops) != 1 ||
		got.Hops[0].MigrationID != "m-visible-in-hops-only" {
		t.Fatalf("the hop feed holds %+v, want the one hop", got.Hops)
	}
}

// TestHopEndpointServesTheFeed covers the surface the page actually polls, and
// the two facts a hop must carry that a lane counter cannot: the EDGE it left
// by — because under two-way lanes (fromSlot, toSlot) no longer names one lane
// — and the species block, recorded and never resolved.
func TestHopEndpointServesTheFeed(t *testing.T) {
	a := newViewFixture(t, contractb.PeerStatus{}, time.Second)
	now := time.Now().UnixMilli()
	a.mu.Lock()
	// The shuttle: one pair of worlds, two lanes, both crossed.
	a.observeHopLocked(Hop{MigrationID: "north", AtMs: now - 100, FromSlot: 1, ToSlot: 4,
		ExitEdge: contracta.EdgeN,
		Species:  &contractb.Species{GenericName: "Izus", SpecificName: "copedylanus"}})
	a.observeHopLocked(Hop{MigrationID: "south", AtMs: now, FromSlot: 1, ToSlot: 4,
		ExitEdge: contracta.EdgeS})
	a.mu.Unlock()

	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/api/hops")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/api/hops answered HTTP %d", resp.StatusCode)
	}
	var feed HopFeed
	if err := json.NewDecoder(resp.Body).Decode(&feed); err != nil {
		t.Fatalf("/api/hops is not valid JSON: %v", err)
	}
	if len(feed.Hops) != 2 {
		t.Fatalf("/api/hops served %d hops, want 2", len(feed.Hops))
	}
	// Oldest first, so a page can animate them in the order they happened.
	if feed.Hops[0].MigrationID != "north" || feed.Hops[1].MigrationID != "south" {
		t.Fatalf("the feed is out of order: %+v", feed.Hops)
	}
	// Two lanes between the SAME two worlds, told apart only by the edge.
	if feed.Hops[0].ExitEdge != contracta.EdgeN || feed.Hops[1].ExitEdge != contracta.EdgeS {
		t.Fatalf("the feed lost the edge: %+v", feed.Hops)
	}
	// Absent is ABSENT: a hop with no species block carries no species, never
	// "unknown" as a value. The page draws the neutral glyph from exactly this.
	if feed.Hops[1].Species != nil {
		t.Fatalf("a hop with no species block reported %+v", feed.Hops[1].Species)
	}
	if feed.Hops[0].Species == nil || feed.Hops[0].Species.SpecificName != "copedylanus" {
		t.Fatalf("the species block was not carried verbatim: %+v", feed.Hops[0].Species)
	}
}

// TestStatusViewDrawsFourLanesPerSlot is contract-b-m4.md §17, B13 on the
// archive's side: §10.1 requires a client to reproduce §8's walk for display,
// and D17 makes that FOUR walks instead of two.
//
// It also pins the thing that made the ledger's lane key ambiguous. On a column
// of two the north and south lanes join the same pair of worlds, so a counter
// keyed on (fromSlot, toSlot) would report one flow for two lanes. Keyed on the
// edge as well, the two directions are two numbers.
func TestStatusViewDrawsFourLanesPerSlot(t *testing.T) {
	stats := &contractb.PeerStats{Population: contractb.IntPtr(9)}
	status := contractb.PeerStatus{
		Epoch: 4, Map: contractb.MapShape{Width: 3, Height: 2}, SlotCount: 6,
		Slots: []contractb.SlotInfo{
			slot4(1, 0, 0, true, stats), slot4(2, 1, 0, true, stats), slot4(3, 2, 0, true, stats),
			slot4(4, 0, 1, true, stats), slot4(5, 1, 1, true, stats), slot4(6, 2, 1, true, stats),
		},
	}
	a := newViewFixture(t, status, time.Second)
	now := time.Now().UnixMilli()
	a.mu.Lock()
	// Three hops north out of slot 1 and one south, on the same shuttle.
	for i := 0; i < 3; i++ {
		a.observeLaneLocked(1, 4, contracta.EdgeN, now-int64(i)*1000)
	}
	a.observeLaneLocked(1, 4, contracta.EdgeS, now)
	a.mu.Unlock()

	view := a.StatusView()
	if len(view.Lanes) != 24 {
		t.Fatalf("the view has %d lanes, want 24 — six slots, four declared edges each",
			len(view.Lanes))
	}
	byEdge := map[string]LaneView{}
	for _, l := range view.Lanes {
		if l.FromSlot == 1 {
			byEdge[l.Edge] = l
		}
	}
	// Row 0 is 1 2 3 and column 0 is 1 under 4.
	for edge, want := range map[string]int{
		contracta.EdgeE: 2, contracta.EdgeW: 3, contracta.EdgeN: 4, contracta.EdgeS: 4,
	} {
		l, ok := byEdge[edge]
		if !ok {
			t.Fatalf("slot 1 has no %s lane", edge)
		}
		if !l.Open || l.ToSlot != want {
			t.Fatalf("slot 1's %s lane is %+v, want an open lane to slot %d", edge, l, want)
		}
	}
	// The two lanes of the shuttle carry their own flows.
	if byEdge[contracta.EdgeN].Migrations != 3 {
		t.Fatalf("the north lane counted %d migrations, want 3",
			byEdge[contracta.EdgeN].Migrations)
	}
	if byEdge[contracta.EdgeS].Migrations != 1 {
		t.Fatalf("the south lane counted %d migrations, want 1 — a counter keyed on the "+
			"slot pair alone would report 4 on both", byEdge[contracta.EdgeS].Migrations)
	}
	// The map-wide total is unaffected: it sums the buckets, not the views.
	if view.Totals.Migrations != 4 {
		t.Fatalf("totals.migrations = %d, want 4", view.Totals.Migrations)
	}
}

// TestLegacyLedgerRecordsKeepTheirLane covers the one thing the new lane key
// has to answer for: a migration recorded BEFORE the exit edge was written to
// the ledger carries no edge, and it must not vanish from the per-lane counts
// when the archive replays it after an upgrade.
//
// It can be re-attributed exactly, and the reason is arithmetic: the only
// export edges that existed then were E and N, and E and N can never resolve to
// the same target, because (col+1,row) and (col,row+1) are different positions
// and no slot occupies two of them. So a legacy bucket belongs to at most one
// of the four lanes drawn today.
func TestLegacyLedgerRecordsKeepTheirLane(t *testing.T) {
	stats := &contractb.PeerStats{Population: contractb.IntPtr(2)}
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 3, Height: 2}, SlotCount: 6,
		Slots: []contractb.SlotInfo{
			slot4(1, 0, 0, true, stats), slot4(2, 1, 0, true, stats), slot4(3, 2, 0, true, stats),
			slot4(4, 0, 1, true, stats), slot4(5, 1, 1, true, stats), slot4(6, 2, 1, true, stats),
		},
	}
	a := newViewFixture(t, status, time.Second)
	now := time.Now().UnixMilli()
	a.mu.Lock()
	a.observeLaneLocked(1, 2, "", now) // a pre-D17 record: slot 1 -> slot 2
	a.observeLaneLocked(1, 2, contracta.EdgeE, now)
	a.mu.Unlock()

	view := a.StatusView()
	for _, l := range view.Lanes {
		if l.FromSlot != 1 {
			continue
		}
		switch l.Edge {
		case contracta.EdgeE:
			if l.Migrations != 2 {
				t.Fatalf("the east lane counted %d, want the new record AND the legacy one",
					l.Migrations)
			}
		default:
			if l.Migrations != 0 {
				t.Fatalf("slot 1's %s lane picked up %d legacy migrations; a legacy bucket "+
					"belongs to at most ONE lane", l.Edge, l.Migrations)
			}
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
