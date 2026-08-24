package archive

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
)

// sample builds one metrics line: a map of len(pops) slots, where a nil entry is
// a slot that reported NOTHING.
func sample(atMs int64, migrations int, pops []*int, live []bool) Status {
	s := Status{
		GeneratedAtMs: atMs, HaveStatus: true,
		Map: contractb.MapShape{Width: len(pops), Height: 1}, SlotCount: len(pops),
	}
	total, haveTotal := 0, false
	for i, p := range pops {
		v := SlotView{
			Slot:     i + 1,
			Position: contractb.Position{Col: i, Row: 0},
			PeerID:   "slot-" + string(rune('1'+i)),
			Live:     live[i],
		}
		if p != nil {
			v.StatsKnown = true
			v.Population = p
			total += *p
			haveTotal = true
		}
		s.Slots = append(s.Slots, v)
	}
	if haveTotal {
		s.Totals.Population = &total
	}
	s.Totals.Migrations = migrations
	return s
}

func ip(v int) *int { return &v }

// TestBuildHistoryDownsamples covers the whole point of /api/history: a page
// gets a FIXED number of buckets no matter how many samples the file holds, the
// bucket carries the mean of what was in it, and — the rule that matters —
// UNKNOWN SURVIVES THE DOWNSAMPLING. A bucket nobody reported in is null, never
// zero (§10.1 rule 3).
func TestBuildHistoryDownsamples(t *testing.T) {
	now := int64(10_000_000)
	window := time.Hour
	buckets := 12
	bucketMs := window.Milliseconds() / int64(buckets) // 5 minutes
	from := now - bucketMs*int64(buckets)

	var samples []Status
	// Bucket 0: two samples, populations 10 and 20 -> mean 15.
	samples = append(samples,
		sample(from+1000, 100, []*int{ip(10), ip(4)}, []bool{true, true}),
		sample(from+2000, 110, []*int{ip(20), ip(6)}, []bool{true, true}),
	)
	// Bucket 2: one sample, slot 1 reported nothing and slot 2 went DARK.
	samples = append(samples,
		sample(from+2*bucketMs+5000, 140, []*int{nil, ip(8)}, []bool{true, false}),
	)
	// Bucket 5: back to normal.
	samples = append(samples,
		sample(from+5*bucketMs+1000, 200, []*int{ip(30), ip(9)}, []bool{true, true}),
	)

	h := BuildHistory(samples, now, window, buckets)
	if h.Buckets != buckets || h.BucketMs != bucketMs {
		t.Fatalf("shape = %d buckets of %dms, want %d of %d", h.Buckets, h.BucketMs, buckets, bucketMs)
	}
	if h.Samples != 4 {
		t.Fatalf("samples inside the window = %d, want 4", h.Samples)
	}
	if len(h.Slots) != 2 {
		t.Fatalf("series = %d, want one per slot", len(h.Slots))
	}
	one := h.Slots[0]
	if len(one.Points) != buckets {
		t.Fatalf("slot 1 has %d points, want %d — the page indexes them by bucket",
			len(one.Points), buckets)
	}
	if one.Points[0].Value == nil || *one.Points[0].Value != 15 {
		t.Fatalf("bucket 0 = %v, want the mean 15", one.Points[0].Value)
	}
	if one.Points[0].N != 2 {
		t.Fatalf("bucket 0 counted %d samples, want 2", one.Points[0].N)
	}
	// THE RULE. An empty bucket and a bucket whose only sample knew nothing are
	// both null. A confident zero here would draw a population crash that never
	// happened.
	for i := 1; i < buckets; i++ {
		if i == 5 {
			continue
		}
		if one.Points[i].Value != nil {
			t.Fatalf("bucket %d invented %d out of no reading", i, *one.Points[i].Value)
		}
	}
	if one.Points[1].N != 0 || one.Points[2].N != 1 {
		t.Fatalf("bucket sample counts are wrong: %+v", one.Points)
	}
	if one.Last == nil || *one.Last != 30 || one.Max != 30 {
		t.Fatalf("slot 1 last/max = %v/%d, want 30/30", one.Last, one.Max)
	}
	// Darkness survives too: Risk 5's whole point is that a healed map hides it.
	if !h.Slots[1].Points[2].Dark {
		t.Fatal("the bucket a slot was dark in did not carry it; a bypassed world would vanish")
	}
	if h.Slots[1].Points[0].Dark {
		t.Fatal("a live bucket was marked dark")
	}
	if h.MaxPopulation != 30 {
		t.Fatalf("maxPopulation = %d, want the API summary 30", h.MaxPopulation)
	}
	if h.Total[0].Value == nil || *h.Total[0].Value != 20 {
		t.Fatalf("total bucket 0 = %v, want the mean of 14 and 26", h.Total[0].Value)
	}
	// Flow is a DIFFERENCE per bucket, never the cumulative counter.
	if h.Flow[0].Value == nil || *h.Flow[0].Value != 10 {
		t.Fatalf("flow bucket 0 = %v, want 10 (110-100), not the cumulative 110", h.Flow[0].Value)
	}
	if h.Flow[2].Value == nil || *h.Flow[2].Value != 30 {
		t.Fatalf("flow bucket 2 = %v, want 30 (140-110)", h.Flow[2].Value)
	}
	if h.Flow[1].Value != nil {
		t.Fatalf("a bucket with no sample reported flow %d", *h.Flow[1].Value)
	}
}

// TestBuildHistoryClampsAndBaselines covers the two edges: a request outside the
// allowed window or bucket count is clamped rather than honoured, and the newest
// sample BEFORE the window is used as the flow baseline so the first bucket is a
// real difference instead of a cumulative spike.
func TestBuildHistoryClampsAndBaselines(t *testing.T) {
	now := int64(50_000_000)
	h := BuildHistory(nil, now, 400*time.Hour, 100000)
	if h.Buckets != HistoryMaxBuckets {
		t.Fatalf("buckets = %d, want the clamp %d", h.Buckets, HistoryMaxBuckets)
	}
	if h.ToMs-h.FromMs > HistoryMaxWindow.Milliseconds() {
		t.Fatalf("window = %dms, want at most %dms", h.ToMs-h.FromMs,
			HistoryMaxWindow.Milliseconds())
	}
	if len(h.Slots) != 0 || len(h.Total) != h.Buckets {
		t.Fatalf("an empty history is not empty-shaped: %d series, %d totals",
			len(h.Slots), len(h.Total))
	}

	window, buckets := time.Hour, 8
	bucketMs := window.Milliseconds() / int64(buckets)
	from := now - bucketMs*int64(buckets)
	samples := []Status{
		sample(from-60_000, 500, []*int{ip(5)}, []bool{true}), // OUTSIDE: the baseline
		sample(from+1000, 507, []*int{ip(5)}, []bool{true}),
	}
	got := BuildHistory(samples, now, window, buckets)
	if got.Samples != 1 {
		t.Fatalf("samples = %d; the out-of-window sample must not be counted", got.Samples)
	}
	if got.Flow[0].Value == nil || *got.Flow[0].Value != 7 {
		t.Fatalf("flow bucket 0 = %v, want 7 — measured from the sample before the window",
			got.Flow[0].Value)
	}
	// A cumulative counter that went backwards is a new ledger, not negative flow.
	back := []Status{
		sample(from-60_000, 900, []*int{ip(5)}, []bool{true}),
		sample(from+1000, 3, []*int{ip(5)}, []bool{true}),
	}
	if v := BuildHistory(back, now, window, buckets).Flow[0].Value; v == nil || *v != 0 {
		t.Fatalf("a reset ledger produced flow %v, want 0", v)
	}
}

func TestBuildAllHistoryIncludesSamplesOlderThanRollingLimit(t *testing.T) {
	now := time.Now().UnixMilli()
	oldest := now - (30 * 24 * time.Hour).Milliseconds()
	samples := []historyStatus{
		compactHistoryStatus(sample(oldest, 1, []*int{ip(10)}, []bool{true})),
		compactHistoryStatus(sample(now-1000, 2, []*int{ip(12)}, []bool{true})),
	}
	h := buildAllHistory(samples, now, 20)
	if h.ToMs-h.FromMs <= HistoryMaxWindow.Milliseconds() {
		t.Fatalf("all-record window = %dms, want more than the rolling limit", h.ToMs-h.FromMs)
	}
	if h.FromMs > oldest || h.Samples != 2 {
		t.Fatalf("all-record history starts at %d with %d samples, want to include %d and both samples",
			h.FromMs, h.Samples, oldest)
	}
}

// TestReadMetricsTailIsBounded covers the read bound: a history request reads the
// END of the sample file, drops the partial record it landed in the middle of,
// and says it truncated. Nothing that serves a picture may read an unbounded
// file (Risk 4).
func TestReadMetricsTailIsBounded(t *testing.T) {
	dir := t.TempDir()
	m, err := OpenMetrics(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 40; i++ {
		if err := m.Append(sample(int64(i)*1000, i, []*int{ip(i)}, []bool{true})); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	all, truncated, err := ReadMetricsTail(m.Path(), 0)
	if err != nil || truncated || len(all) != 40 {
		t.Fatalf("unbounded read = %d samples, truncated %v, %v", len(all), truncated, err)
	}
	info, err := os.Stat(m.Path())
	if err != nil {
		t.Fatal(err)
	}
	// Half the file, which lands in the middle of a record.
	tail, truncated, err := ReadMetricsTail(m.Path(), info.Size()/2)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("a bounded read of half the file did not report truncation")
	}
	if len(tail) == 0 || len(tail) >= 40 {
		t.Fatalf("tail read %d samples, want some but not all of 40", len(tail))
	}
	if tail[len(tail)-1].GeneratedAtMs != 40000 {
		t.Fatalf("the tail did not end at the newest sample: %d", tail[len(tail)-1].GeneratedAtMs)
	}
	// Every returned record parsed: the partial first line was dropped, not
	// half-decoded into a sample with a zero population.
	for _, s := range tail {
		if len(s.Slots) != 1 || s.Slots[0].Population == nil {
			t.Fatalf("a torn record survived the tail read: %+v", s)
		}
	}
	if _, _, err := ReadMetricsTail(filepath.Join(dir, "nothing.jsonl"), 1<<20); err != nil {
		t.Fatalf("a missing metrics file is not an error, it is an empty history: %v", err)
	}
}

func TestReadHistoryStatusesIsCompactBoundedAndTornSafe(t *testing.T) {
	dir := t.TempDir()
	m, err := OpenMetrics(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		if err := m.Append(sample(int64(i)*1000, i, []*int{ip(i)}, []bool{true})); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	all, truncated, err := ReadHistoryStatuses(m.Path(), 0)
	if err != nil || truncated || len(all) != 4 {
		t.Fatalf("complete compact read = %d samples, truncated %v, %v", len(all), truncated, err)
	}
	if all[3].GeneratedAtMs != 4000 || len(all[3].Slots) != 1 || all[3].Slots[0].Population == nil {
		t.Fatalf("newest compact sample lost required fields: %+v", all[3])
	}

	info, err := os.Stat(m.Path())
	if err != nil {
		t.Fatal(err)
	}
	tail, truncated, err := ReadHistoryStatuses(m.Path(), info.Size()/2)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(tail) == 0 || len(tail) >= 4 {
		t.Fatalf("bounded compact read = %d samples, truncated %v", len(tail), truncated)
	}
	if tail[len(tail)-1].GeneratedAtMs != 4000 {
		t.Fatalf("bounded compact read ended at %d, want 4000", tail[len(tail)-1].GeneratedAtMs)
	}

	before, err := os.ReadFile(m.Path())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.Path(), append(before, []byte(`{"generatedAtMs":`)...), 0o644); err != nil {
		t.Fatal(err)
	}
	all, _, err = ReadHistoryStatuses(m.Path(), 0)
	if err != nil || len(all) != 4 {
		t.Fatalf("torn final record changed the compact read: %d samples, %v", len(all), err)
	}
}

// TestHistoryEndpoint covers the HTTP surface: the parameters clamp, the answer
// is valid JSON with the shape the page indexes, and the whole thing is served
// off the archive's own sample file.
func TestHistoryEndpoint(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	now := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		if err := a.metrics.Append(sample(now-int64(4-i)*60_000, 10*i,
			[]*int{ip(20 + i), ip(30)}, []bool{true, true})); err != nil {
			t.Fatal(err)
		}
	}

	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/history?hours=1&buckets=20")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/api/history answered HTTP %d", resp.StatusCode)
	}
	var h History
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		t.Fatalf("/api/history is not valid JSON: %v", err)
	}
	if h.Buckets != 20 || len(h.Total) != 20 || len(h.Flow) != 20 {
		t.Fatalf("buckets = %d, total %d, flow %d; all three must match",
			h.Buckets, len(h.Total), len(h.Flow))
	}
	if len(h.Slots) != 2 {
		t.Fatalf("series = %d, want one per slot in the samples", len(h.Slots))
	}
	if h.MaxPopulation != 30 {
		t.Fatalf("maxPopulation = %d, want 30", h.MaxPopulation)
	}

	allResp, err := http.Get(srv.URL + "/api/history?range=all&buckets=20")
	if err != nil {
		t.Fatal(err)
	}
	defer allResp.Body.Close()
	var all History
	if err := json.NewDecoder(allResp.Body).Decode(&all); err != nil {
		t.Fatalf("all-record /api/history is not valid JSON: %v", err)
	}
	if all.Buckets != 20 || all.Samples != 5 {
		t.Fatalf("all-record history = %d buckets and %d samples, want 20 and 5",
			all.Buckets, all.Samples)
	}
	if all.Truncated {
		t.Fatal("a small complete metrics file was marked truncated")
	}

	// An absurd request is clamped, not honoured.
	r2, err := http.Get(srv.URL + "/api/history?hours=9999&buckets=99999")
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	var big History
	if err := json.NewDecoder(r2.Body).Decode(&big); err != nil {
		t.Fatal(err)
	}
	if big.Buckets != HistoryMaxBuckets {
		t.Fatalf("buckets = %d, want the clamp %d", big.Buckets, HistoryMaxBuckets)
	}
	if big.ToMs-big.FromMs > HistoryMaxWindow.Milliseconds() {
		t.Fatalf("window = %dms, want at most %dms", big.ToMs-big.FromMs,
			HistoryMaxWindow.Milliseconds())
	}
}

// TestPageAndFeedsAreNeverCached pins the other way a reader can be looking at
// a map that is quietly wrong: a browser holding a copy of the page from before
// a feature existed. Every surface here is a live reading of a running system,
// and none of the four may be served from a cache.
func TestPageAndFeedsAreNeverCached(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)

	for _, path := range []string{"/", "/live", "/api/status", "/api/hops", "/api/history"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		cc := resp.Header.Get("Cache-Control")
		_ = resp.Body.Close()
		// no-store, not merely no-cache: an operator surface that a browser may
		// keep on disk is one an operator may be shown after the fact.
		if !strings.Contains(cc, "no-store") {
			t.Fatalf("GET %s served Cache-Control %q; a stale copy of this page shows a "+
				"map that is not the one running", path, cc)
		}
	}
}

// TestLaneRecentHops covers the number the map animates. RecentHops counts only
// what is inside flowWindow, so a lane that carried a thousand organisms
// yesterday and none today animates at a standstill — and Migrations stays
// cumulative, which is what lets a poller see a hop ARRIVE by differencing it.
func TestLaneRecentHops(t *testing.T) {
	stats := &contractb.PeerStats{Population: contractb.IntPtr(4)}
	status := contractb.PeerStatus{
		Epoch: 3, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, stats), slot(2, 1, 0, true, stats)},
	}
	a := newViewFixture(t, status, time.Second)
	now := time.Now().UnixMilli()
	a.mu.Lock()
	// Two hops long past the window, three inside it.
	a.observeLaneLocked(1, 2, contracta.EdgeE, now-2*flowWindow.Milliseconds())
	a.observeLaneLocked(1, 2, contracta.EdgeE, now-int64(1.5*float64(flowWindow.Milliseconds())))
	for i := 0; i < 3; i++ {
		a.observeLaneLocked(1, 2, contracta.EdgeE, now-int64(i)*1000)
	}
	a.mu.Unlock()

	view := a.StatusView()
	if view.FlowWindowMs != flowWindow.Milliseconds() {
		t.Fatalf("flowWindowMs = %d, want %d — a rate with no window is not a measurement",
			view.FlowWindowMs, flowWindow.Milliseconds())
	}
	var east LaneView
	for _, l := range view.Lanes {
		if l.FromSlot == 1 && l.Edge == contracta.EdgeE {
			east = l
		}
	}
	if east.Migrations != 5 {
		t.Fatalf("migrations = %d, want the cumulative 5", east.Migrations)
	}
	if east.RecentHops != 3 {
		t.Fatalf("recentHops = %d, want the 3 inside the %v window", east.RecentHops, flowWindow)
	}
	want := 3 / flowWindow.Minutes()
	if east.PerMinute < want-0.001 || east.PerMinute > want+0.001 {
		t.Fatalf("perMinute = %v, want %v — recentHops and perMinute must agree",
			east.PerMinute, want)
	}
	// A lane nothing has crossed reports zero recent hops, and its label says
	// 0.0/min. Zero here is a measurement, not an unknown.
	for _, l := range view.Lanes {
		if l.FromSlot == 2 && l.RecentHops != 0 {
			t.Fatalf("an untravelled lane reported %d recent hops", l.RecentHops)
		}
	}
}

// TestStatusPageIsSelfContainedAndDrawsTheMap covers what the page has to be: no
// external asset of any kind (it is opened on a LAN with no internet), an actual
// SVG map with lanes rather than a table of numbers, and a glossary — because
// the people who open it did not build the system.
func TestStatusPageIsSelfContainedAndDrawsTheMap(t *testing.T) {
	page := statusPageHTML
	// The SVG namespace is an identifier, not an address: nothing fetches it.
	fetchable := strings.ReplaceAll(page, "http://www.w3.org/2000/svg", "")
	// url(#mOpen) is a reference to a marker defined ten lines up; only an
	// absolute one leaves the page.
	for _, forbidden := range []string{"//cdn", `<link rel="stylesheet"`, "<script src",
		"@import", "url(http", "url(//", "url('http", "url(\"http"} {
		if strings.Contains(fetchable, forbidden) {
			t.Fatalf("the status page reaches outside itself (%q); it must work with no internet",
				forbidden)
		}
	}
	for _, want := range []string{
		"<svg id=", "laneGeom", "arcSeg", "marker-end", "class=\"lane ",
		"bypass", "wrap", "api/history", "api/status", "glosslist",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("the page is missing %q — it is meant to DRAW the map, not tabulate it", want)
		}
	}
	// Every tooltip term the page marks up must exist in the glossary table, or
	// a reader gets a dotted underline that explains nothing.
	gloss := page[strings.Index(page, "var G = {"):strings.Index(page, "(function buildGlossary")]
	checked := 0
	for _, rest := range strings.Split(page, `data-t="`)[1:] {
		key := rest[:strings.Index(rest, `"`)]
		if strings.ContainsAny(key, "'+ ") {
			continue // the one JS helper that fills the attribute in at render time
		}
		checked++
		if !strings.Contains(gloss, "\n "+key+":[") {
			t.Fatalf("the page marks up the term %q with no glossary entry behind it", key)
		}
	}
	if checked < 10 {
		t.Fatalf("only %d marked-up terms found; the page is meant to explain itself", checked)
	}
	// And the reverse: every glossary key the t() helper is handed must exist.
	calls := regexp.MustCompile(`[^A-Za-z0-9_$.]t\("([A-Za-z]+)"`).FindAllStringSubmatch(page, -1)
	if len(calls) < 5 {
		t.Fatalf("only %d t() calls found; the tables and header should carry terms too", len(calls))
	}
	for _, c := range calls {
		if !strings.Contains(gloss, "\n "+c[1]+":[") {
			t.Fatalf("t(%q, ...) names a glossary entry that does not exist", c[1])
		}
	}
}

func TestPopulationHistoryDefaultsToAllTimeAndFitsEachVisibleRange(t *testing.T) {
	page := statusPageHTML
	for _, want := range []string{
		`data-history-range="all" aria-pressed="true"`,
		`data-history-range="24h" aria-pressed="false"`,
		`"range=all&buckets=120"`,
		`function valueRange(points){`,
		`function sparkPath(points, min, max, w, h){`,
		`min+'&ndash;'+max`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("population history is missing %q", want)
		}
	}
	if strings.Contains(page, "H.maxPopulation") {
		t.Fatal("population charts still share a zero-based maximum")
	}
}

// speciesRegion returns the fenced part of the page's script that handles
// census names, and fails if the fence is gone. The fence is not decoration:
// the test below asserts a property OVER it, so deleting the markers would
// silently delete the assertion.
func speciesRegion(t *testing.T) string {
	t.Helper()
	const open = "SPECIES CENSUS — BEGIN"
	const close = "SPECIES CENSUS — END"
	i, j := strings.Index(statusPageHTML, open), strings.Index(statusPageHTML, close)
	if i < 0 || j < 0 || j <= i {
		t.Fatal("the page's species-rendering region is not fenced; the escaping rule of " +
			"contract-b-m4.md §13 item 7 is enforced by a property over that fence")
	}
	return statusPageHTML[i:j]
}

// TestCensusNamesNeverBecomeMarkup is contract-b-m4.md §13 item 7, which is the
// one rule this contract explicitly says it cannot fix for the renderer:
//
//	"a species name is attacker-chosen text of up to 64 bytes a slot, 32 entries
//	 a peer, that lands in a broadcast and then in a renderer. The wire's own
//	 answer is the shape check and the cap; the RENDERER'S answer is its own, and
//	 a page that interpolates a name into HTML without escaping it has a defect
//	 this contract cannot fix for it."
//
// A Go test cannot run the page's JavaScript, so it asserts the STRUCTURAL
// property that makes the defect impossible instead of the behaviour: every
// line of code that touches a census name lives inside one fenced region, and
// THAT REGION CONTAINS NO innerHTML. Names get to the DOM through textContent
// and createElementNS, which cannot parse markup at all, so a name like
// `<img src=x onerror=…>` is inert by construction rather than by escaping.
//
// The rest of the page still builds HTML strings, and that is fine — what it
// must never do is put a name in one. The second half of the test pins that:
// the two places a name would naturally be interpolated (the cell's <title> and
// the worlds table's species column) are emitted EMPTY and filled afterwards.
func TestCensusNamesNeverBecomeMarkup(t *testing.T) {
	region := speciesRegion(t)

	if strings.Contains(region, "innerHTML") {
		t.Fatal("the species-rendering region assigns innerHTML; a census name is " +
			"attacker-chosen text and may only reach the DOM as a text node")
	}
	// It has to actually build nodes, or the rule above is satisfied vacuously.
	for _, want := range []string{"textContent", "createElementNS", "createTextNode",
		"setAttribute", "appendChild"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the species region never calls %s; names are meant to be BUILT, not parsed",
				want)
		}
	}
	// The two markup strings that would otherwise carry a name are emitted
	// empty. If either ever gains one, the innerHTML that assigns it parses it.
	for _, want := range []string{
		`'<title id="ctitle-'+v.slot+'"></title></rect>'`,
		`'<td class="spx" id="wsp-'+v.slot+'"></td>'`,
	} {
		if !strings.Contains(statusPageHTML, want) {
			t.Fatalf("the page no longer emits %s as an EMPTY element; a census name is one "+
				"careless concatenation away from being parsed", want)
		}
	}
	// And the tooltip, which is the other place a name is displayed, sets text
	// rather than markup — for BOTH registries, so the safe path is the only one.
	tipFn := statusPageHTML[strings.Index(statusPageHTML, "function showTip("):]
	tipFn = tipFn[:strings.Index(tipFn, "function hideTip(")]
	if strings.Contains(tipFn, "innerHTML") {
		t.Fatal("showTip assigns innerHTML; the species registry feeds it attacker-chosen text")
	}
	if !strings.Contains(tipFn, "tipHead.textContent") || !strings.Contains(tipFn, "tipBody.textContent") {
		t.Fatalf("showTip does not fill the tip with textContent:\n%s", tipFn)
	}
}

// TestPageDrawsSpeciesGroupedCreatures covers the rendering half of §16, B12
// and §10.1's census rules: the machinery has to be there, and the three
// answers a cell can give have to be three different answers.
func TestPageDrawsSpeciesGroupedCreatures(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// The creature glyph: ONE definition, drawn by reference. A per-organism
	// path would be a few hundred kilobytes of DOM for six worlds.
	//
	// It is defined in the DOCUMENT rather than inside the map's SVG, and that
	// moved when the species tab needed the same glyph: the map's SVG is thrown
	// away and rebuilt whenever the map's shape changes, so a definition living
	// inside it is one the other tabs cannot reference.
	if !strings.Contains(page, `<g id="bib">`) {
		t.Fatal("the page has no bibite glyph definition")
	}
	if !strings.Contains(page, `<svg class="glyphdefs"`) {
		t.Fatal("the creature glyph is not defined for the whole document; a tab other than " +
			"the map cannot draw a species that only exists inside the map's own SVG")
	}
	if strings.Count(page, `<path d="M 4.5 -1.26`) != 1 {
		t.Fatal("the creature body is not defined exactly once")
	}
	if !strings.Contains(region, `"use"`) || !strings.Contains(region, `"#bib"`) {
		t.Fatal("the cells do not draw the glyph by reference")
	}
	// Colour by a stable hash of the compared name, so one species is one
	// colour in every world and across every reload, with no table to keep.
	for _, want := range []string{"function hashStr", "function speciesColor", "hsl("} {
		if !strings.Contains(region, want) {
			t.Fatalf("the species colouring is missing %q", want)
		}
	}
	// Normalization exists exactly once, is named for what it is, and is used
	// for comparison only (contract-a.md §17, A36).
	if strings.Count(region, "function normName") != 1 {
		t.Fatal("normName is not defined exactly once; it is the ONLY place a census name " +
			"may be whitespace-normalized, and only for a comparison")
	}
	if strings.Contains(region, "e.genericName = ") || strings.Contains(region, ".genericName=") {
		t.Fatal("the renderer writes back to a census name; nothing may repair one")
	}
	// The cross-world join, and the second tooltip registry that shows it.
	for _, want := range []string{"function buildCrossWorld", "XW[", "data-s", "SP[key]"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the cross-world species join is missing %q", want)
		}
	}
	if !strings.Contains(page, `"[data-t],[data-s]"`) {
		t.Fatal("the tooltip listeners do not cover the species registry, so the glyphs " +
			"would have no tooltip and no dismissal rules")
	}
	// Eggs are their own mark, because a world's population count excludes them.
	if !strings.Contains(region, `"egg"`) || !strings.Contains(page, ".egg{fill:none") {
		t.Fatal("eggs are not drawn distinctly from creatures")
	}
	// §10.1's three answers, each in the page's own words.
	for _, want := range []string{"species unknown", "rest unreported", "nothing alive",
		"no species record"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the page never says %q; absent, truncated and empty are three different "+
				"facts and an honest gap beats a confident zero", want)
		}
	}
	// The worlds table gained a species column, and the totals say how many
	// worlds are reporting none.
	if !strings.Contains(page, "paintSpeciesCell") || !strings.Contains(page, "function censusless") {
		t.Fatal("the worlds table has no species column, or the totals hide the gap")
	}
}

// TestPageDrawsTwoWayLanes is the rendering half of contract-b-m4.md §17, B13.
// A Go test cannot run the page's JavaScript, so it asserts that the MACHINERY
// a two-way map needs exists and that the machinery a one-way map used is gone.
//
// The rendering choice being pinned here: each lane pair is drawn as TWO
// SEPARATE DIRECTED ARROWS ON PARALLEL RAILS, not as one line with a head at
// each end. On the 3x2 rig that is 12 pairs = 24 directions, and a double-headed
// line cannot carry any of the three things a direction owns — its own rate
// label, its own state (skip lists are per WALK, so one direction can be a
// bypass while the other is direct), and its own flow.
func TestPageDrawsTwoWayLanes(t *testing.T) {
	page := statusPageHTML

	// The rail offset and the bow allowance are what keep the two arrows of a
	// pair apart and stop their bypass arcs crossing.
	for _, want := range []string{"function laneOff", "function laneBow", "var LSEP"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the page has no %s; two arrows in one gutter would be drawn on top "+
				"of each other", want)
		}
	}
	// The geometry knows all four edges and both wrap directions.
	for _, want := range []string{`e === "N" || e === "S"`, `e === "W" || e === "S"`,
		"(c - steps) >= 0", "(r - steps) >= 0"} {
		if !strings.Contains(page, want) {
			t.Fatalf("laneGeom is missing %q; a west or south lane wraps through the "+
				"OTHER edge of the map", want)
		}
	}
	// Every direction names itself in a tooltip.
	if !strings.Contains(page, "var EDGEWORD = {E:\"east\", N:\"north\", W:\"west\", S:\"south\"}") {
		t.Fatal("the lane tooltip does not name all four directions")
	}
	// The retired sentence must be GONE from the glossary, not merely
	// contradicted elsewhere: D17 supersedes "out and in are different doors",
	// and a page that still says otherwise is teaching a reader something false.
	for _, stale := range []string{"A one-way road", "Creatures only ever travel east and north",
		"Those two are the only exits"} {
		if strings.Contains(page, stale) {
			t.Fatalf("the glossary still says %q; every edge is both an exit and a door now",
				stale)
		}
	}
	// And the two facts a reader of a two-way map has to be told: the pair, and
	// the degenerate axis where both lanes of it name one peer.
	if !strings.Contains(page, " shuttle:[") {
		t.Fatal("the glossary never explains the two-lane pair of an axis of length 2, " +
			"which is every COLUMN of the rig")
	}
}

// TestPageAnimatesTheSpeciesGlyphOnAHop is D19 / §17 B14's rendering half. The
// page polls the bounded hop feed and sends THAT SPECIES' OWN GLYPH down the
// lane it crossed, and the test pins the machinery, the bounds and the
// escaping structure that makes an attacker-chosen name inert.
func TestPageAnimatesTheSpeciesGlyphOnAHop(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// It polls its own endpoint, not the status view. /api/status is written
	// verbatim into the durable metrics file every minute and must stay lean.
	if !strings.Contains(page, `fetch("api/hops"`) {
		t.Fatal("the page never polls /api/hops")
	}
	// The machinery: a queue, a launcher, a per-frame step, and a teardown for
	// when the SVG is rebuilt under a travelling glyph.
	for _, want := range []string{"function onHops", "function launchHop", "function stepHops",
		"function killHops", "function hopMarkSeen", "requestAnimationFrame"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the hop animation is missing %q", want)
		}
	}
	// Bounded on every axis a burst can grow along: concurrent glyphs, the
	// pending queue, the stagger between launches, and the seen-set that stops
	// one migration animating twice.
	for _, want := range []string{"HOPMAX", "HOPQMAX", "HOPSTAGGER", "HOPSEENMAX", "HOPMS"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the hop animation has no %s bound; a dam releasing hundreds of "+
				"arrivals must not put hundreds of creatures on the map", want)
		}
	}
	// ~1.5-2s per hop, whatever the lane's length.
	if !strings.Contains(page, "var HOPMS = 1700") {
		t.Fatal("the hop duration is not the ~1.7s the design calls for")
	}
	// It travels the LANE, and it is the same glyph the cells draw — one
	// definition, drawn by reference — rather than a second creature drawing.
	if !strings.Contains(page, "getPointAtLength") || !strings.Contains(page, "q.a.tracks") {
		t.Fatal("the hop does not follow the lane path")
	}
	if !strings.Contains(region, "function buildHopGlyph") {
		t.Fatal("the hop glyph is built outside the fenced species region, where the " +
			"escaping rule is enforced by a property over the fence")
	}
	if !strings.Contains(region, `u.setAttribute("href", "#bib")`) {
		t.Fatal("the hop draws its own creature instead of the shared glyph")
	}
	// ESCAPING: the name reaches the DOM as a text node and nothing else. The
	// fence-wide innerHTML assertion in TestCensusNamesNeverBecomeMarkup covers
	// buildHopGlyph too, now that it lives inside the fence; this pins the
	// positive half.
	if !strings.Contains(region, "ttl.textContent") {
		t.Fatal("the hop's species name does not reach the DOM through textContent")
	}
	// NEUTRAL WHEN UNKNOWN: absent is absent, never a guessed name and never
	// "unknown" rendered as if it were a species.
	if !strings.Contains(region, "function hopName") ||
		!strings.Contains(region, `"hopbib" : "hopbib neutral"`) {
		t.Fatal("a hop with no species block is not drawn as the neutral glyph")
	}
	if !strings.Contains(page, "a creature of no reported species") {
		t.Fatal("the page never says a hop's species is unreported")
	}
	// It is LEDGER, not census, and the page has to say so: the glyphs in a cell
	// and the glyph on a lane are two facts from two sources.
	if !strings.Contains(page, " hopfeed:[") {
		t.Fatal("the glossary never distinguishes who CROSSED from who LIVES here")
	}
	// MIGRATION_PAYLOAD counters measure offers, not completed deliveries. If
	// the delivery feed is unreachable the page must pause animation instead of
	// turning a rejected population-admission attempt into a false arrival.
	for _, forbidden := range []string{"prevMig", "hopFeedOK", "l.migrations > was"} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("the page still infers a delivery from offer counters: %q", forbidden)
		}
	}
	for _, want := range []string{"MIGRATION_ACK", "Rejected offers never move",
		"eventual acknowledged destination"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the page does not explain delivery-confirmed motion: missing %q", want)
		}
	}
	// The feed is polled UNCONDITIONALLY. It used to be gated on
	// prefers-reduced-motion, which meant a reader whose system asks for less
	// motion was never told a crossing happened at all — see
	// TestReducedMotionDegradesRatherThanSuppresses.
	if !strings.Contains(page, "tickHops(); setInterval(tickHops, 1500);") {
		t.Fatal("the hop feed is not polled on a timer")
	}
	if strings.Contains(page, "if (!reduced){ tickHops()") {
		t.Fatal("the hop feed is gated on reduced motion again; motion changes how a " +
			"crossing is drawn, never whether the page knows one happened")
	}
}

// TestReducedMotionDegradesRatherThanSuppresses pins the rule the first version
// of the hop animation got wrong, and got wrong INVISIBLY.
//
// prefers-reduced-motion used to skip the hop poll and the frame loop outright,
// so on a machine reporting reduce — which on Windows is the ordinary
// "Animation effects: off", nothing to do with this page — the map drew, the
// numbers moved, and no crossing was ever shown or explained. A reader had no
// way to tell a quiet multiverse from a suppressed one.
//
// The rule is now DEGRADE, NEVER SUPPRESS, and it has two halves a Go test can
// assert structurally: a still form of the same event, and a switch that beats
// the media query in BOTH directions.
func TestReducedMotionDegradesRatherThanSuppresses(t *testing.T) {
	page := statusPageHTML

	// Half one: the still arrival exists, is bounded like the travelling form,
	// and is chosen from onHops rather than the hop being dropped.
	for _, want := range []string{"function stillHop", "HOPSTILLMS", "HOPSTILLMAX",
		"if (reduced){ stillHop(", ".hopstill{animation:hopstill"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the reduced-motion fallback is missing %q; a crossing that is not "+
				"animated must still be SHOWN, not dropped", want)
		}
	}
	// It lands where the travelling glyph would have ended — the far end of the
	// lane's own path — so both forms agree about where a crossing arrives, and
	// a bypass or a wrap is not guessed at from cell coordinates.
	if !strings.Contains(page, "a.tracks[a.tracks.length-1]") {
		t.Fatal("the still arrival does not read the end of the lane path")
	}
	// And it is the SAME glyph, in the same species colour, with the same
	// escaping: it goes through buildHopGlyph inside the fence.
	if !strings.Contains(page, "var g = buildHopGlyph(name, to);") {
		t.Fatal("the still arrival builds its own glyph instead of the fenced one")
	}
	// The arrival flash is a pure opacity fade and is deliberately NOT suppressed
	// under reduced motion any more: with travel off it is half the signal.
	if strings.Contains(page, "@media (prefers-reduced-motion:reduce){.cell.hit") {
		t.Fatal("the arrival flash is suppressed under reduced motion again; it is an " +
			"opacity fade with no movement in it, and it is what says a creature landed")
	}

	// Half two: the switch. Three states, in the page, persisted, and read back
	// on load — a media query is a guess about a person and has to be beatable.
	for _, want := range []string{`id="motion"`, `data-m="auto"`, `data-m="on"`, `data-m="off"`,
		"function applyMotion", "function setMotionPref", "function readMotionPref",
		"localStorage.setItem(MOTIONKEY", "localStorage.getItem(MOTIONKEY"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the motion switch is missing %q", want)
		}
	}
	// It overrides the query BOTH ways: off forces reduced on a system that never
	// asked, on animates on a system that did.
	if !strings.Contains(page,
		`reduced = (motionPref === "off") || (motionPref === "auto" && mqReduced);`) {
		t.Fatal("the motion switch does not override prefers-reduced-motion in both directions")
	}
	// Switching it at runtime must actually start and stop the loop, and sweep
	// the glyphs that would otherwise stall mid-lane with no loop to move them.
	for _, want := range []string{"cancelAnimationFrame(rafId)", "function killHops",
		"rafId = requestAnimationFrame(frame)"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the motion switch does not take effect live: missing %q", want)
		}
	}
	// The system setting can change under an open page.
	if !strings.Contains(page, `mq.addEventListener("change"`) {
		t.Fatal("a change to the system reduced-motion setting does not reach the open page")
	}
	// And the page SAYS which way it went. The whole defect was that it did not.
	if !strings.Contains(page, `id="motionwhy"`) ||
		!strings.Contains(page, "your system asks for reduced motion") {
		t.Fatal("the page never tells a reader why crossings are not travelling")
	}
}

// TestTheMapHasNoAmbientPulses pins a removal, which is the kind of change that
// creeps back.
//
// The map used to walk a small dot down every open lane at that lane's measured
// rate. It was wallpaper: it never named an organism, it said nothing the
// lane's own "/min" label did not say in a number a reader can compare, and
// once real hop glyphs travelled the same arrows (D19) it actively misled —
// two things moving along one lane, one of them evidence and one of them
// decoration, drawn at the same size on the same path.
//
// WHAT MOVES ON THIS MAP IS NOW ALWAYS A RECEIVER-ACKNOWLEDGED DELIVERY.
func TestTheMapHasNoAmbientPulses(t *testing.T) {
	page := statusPageHTML
	for _, gone := range []string{
		"pulse", "pulseLayer", `id="pulses"`, "function spawn(", "fireHot",
		"killPulses", "a.pulses", "a.rate", "a.gap",
	} {
		if strings.Contains(page, gone) {
			t.Fatalf("the ambient lane pulse is back: %q is still in the page", gone)
		}
	}
	// The rate itself is NOT gone. It was never the pulse's to carry: it is a
	// measurement, and it stays on the lane as a number.
	if !strings.Contains(page, `lab.textContent = rate(l.perMinute)+"/min";`) {
		t.Fatal("the lane lost its measured rate along with the dots that paced it")
	}
	// And the two things that DID depend on the frame loop are untouched: the
	// travelling hop glyph and, under reduced motion, the still one.
	for _, want := range []string{"function stepHops", "function stillHop", "function flashCell",
		"rafId = requestAnimationFrame(frame)"} {
		if !strings.Contains(page, want) {
			t.Fatalf("removing the pulses took %q with them", want)
		}
	}
}

// TestEachWorldShowsItsSpeedAndItsPace covers contract-b-m4.md §18, B17 on the
// page: a cell says how fast its world runs and how close its arrivals are to
// the cap they are paced behind — and says UNKNOWN for either when the peer has
// not published it, which is the case for any sidecar older than B16.
func TestEachWorldShowsItsSpeedAndItsPace(t *testing.T) {
	page := statusPageHTML
	for _, want := range []string{
		"function speedText", "function paceRateText", "function paceDepthText",
		"function paintChips", "paintChips(v);", `id="cchip-`,
		`data-t="speed"`, `data-t="pace"`, " speed:[", " pace:[",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("the per-world speed and pace display is missing %q", want)
		}
	}
	// UNKNOWN, NEVER THE DEFAULT. The shipped inboundRatePerSimMinute has moved
	// three times; a page that fills one in for a peer that did not state it is
	// lying about the only slot an operator cannot check by hand.
	if !strings.Contains(page,
		`return (v.statsKnown && v.inboundRatePerSimMinute != null)`) {
		t.Fatal("the pace cap is not read straight off the stats block, or absence is not a value")
	}
	for _, want := range []string{`? fmtScale(v.inboundRatePerSimMinute) : "?"`, `: "×?"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("an unpublished speed or cap does not render as unknown: missing %q", want)
		}
	}
	if strings.Contains(page, "inboundRatePerSimMinute || 100") ||
		strings.Contains(page, "|| 2.0") {
		t.Fatal("the page defaults a pacing number it was not told")
	}
	// A world standing still reports ×0, and 0 is a reading. It must not be
	// swallowed by a truthiness test on the way to the screen.
	if !strings.Contains(page, "v.timeScale != null") {
		t.Fatal("timeScale is tested for truthiness somewhere; a paused world reports 0 " +
			"and ×0 is a fact, not a gap")
	}
	// The worlds table carries both, beside the numbers they explain.
	if !strings.Contains(page, `<td class="num">'+speed+'</td>`) ||
		!strings.Contains(page, `<td class="num">'+pace+'</td>`) {
		t.Fatal("the worlds table does not carry the speed and pace columns")
	}
}
