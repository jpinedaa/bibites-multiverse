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
		t.Fatalf("maxPopulation = %d, want 30 — the shared sparkline scale", h.MaxPopulation)
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
	a.observeLaneLocked(1, 2, now-2*flowWindow.Milliseconds())
	a.observeLaneLocked(1, 2, now-int64(1.5*float64(flowWindow.Milliseconds())))
	for i := 0; i < 3; i++ {
		a.observeLaneLocked(1, 2, now-int64(i)*1000)
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
	// A lane nothing has crossed reports zero recent hops, and the map draws no
	// pulses on it. Zero here is a measurement, not an unknown.
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
	for _, forbidden := range []string{"http://", "https://", "//cdn", "<link", "<script src",
		"@import", "url(http", "url(//", "url('http", "url(\"http"} {
		if strings.Contains(fetchable, forbidden) {
			t.Fatalf("the status page reaches outside itself (%q); it must work with no internet",
				forbidden)
		}
	}
	for _, want := range []string{
		"<svg id=", "laneGeom", "arcSeg", "marker-end", "class=\"lane ",
		"bypass", "wrap", "pulse", "api/history", "api/status", "glosslist",
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
	if !strings.Contains(page, `'<g id="bib">'`) {
		t.Fatal("the page has no bibite glyph definition")
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
