package archive

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/wire"
)

// census builds a stats block carrying a census, so a fixture can put species
// in a world without a rig.
func census(pop, eggs int, entries ...contractb.CensusEntry) *contractb.PeerStats {
	return &contractb.PeerStats{
		Population: contractb.IntPtr(pop),
		EggCount:   contractb.IntPtr(eggs),
		Species:    &contractb.Census{Entries: entries},
	}
}

func entry(generic, specific string, bibites, eggs int) contractb.CensusEntry {
	return contractb.CensusEntry{GenericName: generic, SpecificName: specific,
		Bibites: bibites, Eggs: eggs}
}

// migration builds one ledger record, for driving the aggregate directly.
func migration(atMs int64, from, to int, edge, generic, specific, hash string) Record {
	rec := Record{
		Type: RecordMigration, RecordedAt: atMs, MigrationID: "m" + hash,
		SourceSlot: from, DestSlot: to, ExitEdge: edge,
		Species: &wire.Species{GenericName: generic, SpecificName: specific},
	}
	if hash != "" {
		rec.Lineage = &contractb.Lineage{GenomeHash: hash}
	}
	return rec
}

// TestSpeciesAggregateIsIncrementalAndKeyedOnTheComparedName covers species.go's
// first two rules at once.
//
// RULE 1, INCREMENTAL: the same records folded one at a time produce exactly the
// aggregate a startup replay produces, because there is one function and two
// call sites. A test that only exercised the replay would let the live path rot
// silently — and the live path is the one that runs for a month between
// restarts.
//
// RULE 2, NORMALIZED KEY AND RAW LABEL: two spellings that differ only in
// whitespace are ONE species for counting and TWO labels for display. A34
// normalizes the key; A36 forbids repairing the label.
func TestSpeciesAggregateIsIncrementalAndKeyedOnTheComparedName(t *testing.T) {
	if speciesAggMax != 1<<16 {
		t.Fatalf("species aggregate capacity = %d, want 65,536: the former 4,096 bound "+
			"was exhausted during the first public week", speciesAggMax)
	}
	a, err := New(Config{DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	if a.species.max != speciesAggMax {
		t.Fatalf("a new archive applies species capacity %d, want %d", a.species.max, speciesAggMax)
	}

	base := time.Now().Add(-time.Hour).UnixMilli()
	recs := []Record{
		// "Izus " with a trailing space and "Izus" without it: two Species
		// records in two worlds, ONE species on this index.
		migration(base, 1, 2, "E", "Izus ", "copedylanus", "hash-a"),
		migration(base+1000, 2, 3, "N", "Izus", "copedylanus", "hash-b"),
		// The same genome crossing twice is two crossings and ONE genome.
		migration(base+2000, 3, 1, "W", "Izus", "copedylanus", "hash-b"),
		// A different species entirely.
		migration(base+3000, 1, 2, "E", "Cyanea", "velox", "hash-c"),
		// An envelope with NO species block: a crossing by a creature of no
		// reported species, which must not invent a key.
		{Type: RecordMigration, RecordedAt: base + 4000, MigrationID: "m-none",
			SourceSlot: 1, DestSlot: 2},
	}
	for _, rec := range recs {
		a.mu.Lock()
		a.observeSpeciesLocked(rec)
		a.mu.Unlock()
	}

	a.mu.Lock()
	got := a.species
	a.mu.Unlock()
	if len(got.byKey) != 2 {
		keys := make([]string, 0, len(got.byKey))
		for k := range got.byKey {
			keys = append(keys, k)
		}
		t.Fatalf("the aggregate holds %d species %v, want 2: the whitespace twins are one "+
			"species and the block-less crossing is none", len(got.byKey), keys)
	}
	izus := got.byKey["Izus copedylanus"]
	if izus == nil {
		t.Fatalf("the aggregate is not keyed on the A34-normalized name: %+v", got.byKey)
	}
	if izus.crossings != 3 {
		t.Fatalf("crossings = %d, want 3", izus.crossings)
	}
	if len(izus.genomes) != 2 {
		t.Fatalf("distinct genomes = %d, want 2 — the same hash twice is one genome",
			len(izus.genomes))
	}
	if izus.firstMs != base || izus.lastMs != base+2000 {
		t.Fatalf("first/last = %d/%d, want %d/%d", izus.firstMs, izus.lastMs, base, base+2000)
	}
	// Recent crossings, newest kept, with the lane the organism left by.
	if len(izus.recent) != 3 || izus.recent[2].ExitEdge != "W" {
		t.Fatalf("recent = %+v", izus.recent)
	}

	// The bound, and that it is COUNTED rather than silently dropped.
	a.mu.Lock()
	a.species.max = 4
	for i := 0; len(a.species.byKey) < a.species.max; i++ {
		a.species.byKey["Filler "+strconv.Itoa(i)] = &speciesAgg{
			genomes: map[uint64]struct{}{}}
	}
	a.observeSpeciesLocked(migration(base, 1, 2, "E", "Overflow", "species", ""))
	overflow := a.species.overflow
	tracked := len(a.species.byKey)
	a.mu.Unlock()
	if tracked > a.species.max {
		t.Fatalf("the aggregate tracks %d species, over its test bound of %d", tracked, a.species.max)
	}
	if overflow == 0 {
		t.Fatal("the aggregate hit its bound and did not say so; a truncated answer a " +
			"reader cannot see is a wrong answer")
	}
}

// TestSpeciesAggregateSurvivesARestart is rule 1's other half: the aggregate is
// rebuilt from the ledger by the replay New() already performs, so an archive
// that restarts does not tell an operator a species has never crossed.
func TestSpeciesAggregateSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatalf("OpenLedger: %v", err)
	}
	base := time.Now().Add(-time.Hour).UnixMilli()
	for i, rec := range []Record{
		migration(base, 1, 2, "E", "Izus", "copedylanus", "hash-a"),
		migration(base+1000, 2, 3, "N", "Izus", "copedylanus", "hash-b"),
		migration(base+2000, 3, 1, "W", "Cyanea", "velox", "hash-c"),
	} {
		rec.MigrationID = rec.MigrationID + string(rune('0'+i))
		if err := ledger.Append(rec); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := ledger.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	a, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	a.mu.Lock()
	izus := a.species.byKey["Izus copedylanus"]
	n := len(a.species.byKey)
	a.mu.Unlock()
	if n != 2 || izus == nil || izus.crossings != 2 || len(izus.genomes) != 2 {
		t.Fatalf("the replay did not rebuild the aggregate: %d species, izus=%+v", n, izus)
	}
}

// TestSpeciesIndexIsAliveOnlyAndCarriesTheBadges is the whole species tab's data
// path, and the rule it exists to keep: THE INDEX IS THE CENSUS UNION AND
// NOTHING ELSE.
//
// The fixture is built so the two sources DISAGREE on purpose: one species has
// crossed many times and is extinct everywhere, and one is alive in every world
// and has never crossed. A view that took its rows from the ledger would list
// the first; a view that dropped a row for having no crossings would lose the
// second. Both are wrong, and both are the mistake D11 and §10.1 name.
func TestSpeciesIndexIsAliveOnlyAndCarriesTheBadges(t *testing.T) {
	excl := &wire.ExcludeList{Names: []string{"Basic bibite"}}
	s1 := census(30, 2,
		entry("Basic", "bibite", 20, 1),
		entry("Izus ", "copedylanus", 10, 1))
	s1.MigrationExclude = excl
	s2 := census(12, 0,
		entry("Basic", "bibite", 9, 0),
		entry("Izus", "copedylanus", 3, 0))
	// A third world reporting NO census at all: it is censusless, and it must
	// not count toward "everywhere".
	s3 := &contractb.PeerStats{Population: contractb.IntPtr(50)}

	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 3, Height: 1}, SlotCount: 3,
		Slots: []contractb.SlotInfo{
			slot(1, 0, 0, true, s1), slot(2, 1, 0, true, s2), slot(3, 2, 0, true, s3),
		},
	}
	a := newViewFixture(t, status, time.Second)

	base := time.Now().Add(-2 * time.Hour).UnixMilli()
	a.mu.Lock()
	// A species that crossed a great deal and is alive nowhere.
	a.observeSpeciesLocked(migration(base, 1, 2, "E", "Ghost", "extinctus", "hash-g1"))
	a.observeSpeciesLocked(migration(base+1, 2, 3, "N", "Ghost", "extinctus", "hash-g2"))
	// And one crossing by a species that IS alive, so the annotation half has
	// something to say.
	a.observeSpeciesLocked(migration(base+2, 1, 2, "E", "Izus", "copedylanus", "hash-i1"))
	a.mu.Unlock()

	idx := a.SpeciesIndexView()

	if idx.ReportingSlots != 2 {
		t.Fatalf("reportingSlots = %d, want 2 — a world with no census is not reporting",
			idx.ReportingSlots)
	}
	if idx.CensuslessSlots != 1 {
		t.Fatalf("censuslessSlots = %d, want 1", idx.CensuslessSlots)
	}
	if len(idx.Species) != 2 {
		t.Fatalf("the index holds %d species, want 2 — a species that is extinct everywhere "+
			"is a ledger fact and never a row here: %+v", len(idx.Species), idx.Species)
	}
	for _, r := range idx.Species {
		if r.Key == "Ghost extinctus" {
			t.Fatal("a species with no living members was promoted into the alive index by " +
				"its crossing record; a database built from migrations holds migrants and " +
				"their ancestors, never a resident population")
		}
	}
	// The ledger still knows about it, and says so as a COUNT rather than a row.
	if idx.LedgerSpecies != 2 {
		t.Fatalf("ledgerSpecies = %d, want 2", idx.LedgerSpecies)
	}

	byKey := map[string]SpeciesRow{}
	for _, r := range idx.Species {
		byKey[r.Key] = r
	}

	basic := byKey["Basic bibite"]
	// Default order is population descending: 29 alive beats 13.
	if idx.Species[0].Key != "Basic bibite" {
		t.Fatalf("the default order is not population descending: %+v", idx.Species)
	}
	if basic.Population != 29 || basic.Eggs != 1 {
		t.Fatalf("Basic bibite: %d alive %d eggs, want 29/1", basic.Population, basic.Eggs)
	}
	if !basic.Excluded || len(basic.ExcludedBy) != 1 || basic.ExcludedBy[0] != 1 {
		t.Fatalf("the exclusion badge did not land: %+v", basic)
	}
	if !basic.Everywhere {
		t.Fatal("a species alive in both reporting worlds is not marked everywhere")
	}
	if basic.Endemic {
		t.Fatal("a species alive in two worlds is marked endemic")
	}
	// It has never crossed, which is exactly the shape the exclusion explains,
	// and 0 is a count rather than a gap.
	if basic.Crossings != 0 || basic.LastAgeMs != nil {
		t.Fatalf("Basic bibite has crossings %d / last %v; it is on an exclusion list",
			basic.Crossings, basic.LastAgeMs)
	}

	izus := byKey["Izus copedylanus"]
	if izus.Population != 13 {
		t.Fatalf("the whitespace twins were not summed as one species: %d", izus.Population)
	}
	// RAW LABELS, BOTH OF THEM. The key merged them; the display must not.
	if izus.Name != "Izus  copedylanus" || len(izus.Spellings) != 1 ||
		izus.Spellings[0] != "Izus copedylanus" {
		t.Fatalf("the raw spellings were repaired or dropped: name=%q spellings=%+v",
			izus.Name, izus.Spellings)
	}
	if izus.Crossings != 1 || izus.LastAgeMs == nil || izus.Genomes != 1 {
		t.Fatalf("the ledger annotation did not reach the row: %+v", izus)
	}
	if izus.Excluded {
		t.Fatal("a species nobody excludes is marked excluded")
	}

	// ENDEMIC: alive in exactly one reporting world. Adding a species to slot 2
	// alone is the whole test.
	s2.Species.Entries = append(s2.Species.Entries, entry("Solus", "unicus", 4, 0))
	idx = a.SpeciesIndexView()
	for _, r := range idx.Species {
		if r.Key != "Solus unicus" {
			continue
		}
		if !r.Endemic || r.Everywhere {
			t.Fatalf("a species alive in one of two reporting worlds: endemic=%v everywhere=%v",
				r.Endemic, r.Everywhere)
		}
	}
}

// TestEverywhereIsNotClaimedFromOneWorld pins the honesty guard on the badge
// that would otherwise be the most misleading: with a single world answering,
// "everywhere" and "endemic" are the same sentence, and printing both would
// dress one census up as a finding about the map.
func TestEverywhereIsNotClaimedFromOneWorld(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{
			slot(1, 0, 0, true, census(5, 0, entry("Only", "one", 5, 0))),
			// Live, and reporting no census: half the map is silent.
			slot(2, 1, 0, true, &contractb.PeerStats{Population: contractb.IntPtr(9)}),
		},
	}
	idx := newViewFixture(t, status, time.Second).SpeciesIndexView()
	if len(idx.Species) != 1 {
		t.Fatalf("index = %+v", idx.Species)
	}
	if idx.Species[0].Everywhere {
		t.Fatal("a species alive in the only reporting world is claimed to be everywhere")
	}
	if !idx.Species[0].Endemic {
		t.Fatal("a species alive in exactly one world is not endemic")
	}
}

// TestStaleStatsHideTheSpeciesIndexToo is §10.1 rule 3 on the new view: a stats
// block older than the freshness rule is history, and a species list built from
// it is a list of who WAS alive.
func TestStaleStatsHideTheSpeciesIndexToo(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(5, 0, entry("Old", "news", 5, 0)))},
	}
	idx := newViewFixture(t, status, 5*time.Minute).SpeciesIndexView()
	if len(idx.Species) != 0 {
		t.Fatalf("a five-minute-old census produced a live species list: %+v", idx.Species)
	}
	if idx.ReportingSlots != 0 {
		t.Fatalf("reportingSlots = %d from a stale block", idx.ReportingSlots)
	}
}

// TestSpeciesEndpointShape is the wire shape the page depends on, asserted on
// the JSON rather than on the Go value: a distinction a client cannot see is
// not a distinction.
func TestSpeciesEndpointShape(t *testing.T) {
	s1 := census(10, 1, entry("Izus ", "copedylanus", 10, 1))
	s1.MigrationExclude = &wire.ExcludeList{Names: []string{`Cyanëa<&> velox"issima`}}
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, s1)},
	}
	a := newViewFixture(t, status, time.Second)

	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/species")
	if err != nil {
		t.Fatalf("GET /api/species: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP %d", resp.StatusCode)
	}
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"generatedAtMs", "haveStatus", "reportingSlots",
		"censuslessSlots", "truncatedSlots", "ledgerSpecies", "species"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("/api/species is missing %q: %v", key, raw)
		}
	}
	list := raw["species"].([]any)
	if len(list) != 1 {
		t.Fatalf("species = %v", list)
	}
	row := list[0].(map[string]any)
	for _, key := range []string{"key", "name", "population", "eggs", "worlds",
		"excluded", "everywhere", "endemic", "crossings", "genomes"} {
		if _, ok := row[key]; !ok {
			t.Fatalf("a species row is missing %q: %v", key, row)
		}
	}
	// THE KEY IS NORMALIZED AND THE NAME IS RAW, over the wire, where the page
	// reads them.
	if row["key"] != "Izus copedylanus" || row["name"] != "Izus  copedylanus" {
		t.Fatalf("key/name = %v / %v", row["key"], row["name"])
	}
	// The endpoint's own JSON must be usable by an unbounded list: it is
	// bounded by the census that produced it, and that is stated on the view.
	if raw["ledgerRecords"] == nil {
		t.Fatal("/api/species does not say how much record it is annotating from")
	}
}

// TestSpeciesHistoryDownsamplesAndKeepsUnknownApartFromZero is the sparkline
// endpoint's whole honesty rule, which is the one thing this series can get
// wrong in a way that reads as a finding:
//
//	A WORLD THAT REPORTED A CENSUS WITHOUT THIS SPECIES HELD ZERO OF IT.
//	A WORLD THAT REPORTED NO CENSUS IS UNKNOWN.
//
// Flattening the second into the first draws a line at the bottom of the chart
// for a world that was simply not answering, which is exactly the "confident
// zero" §10.1 forbids.
func TestSpeciesHistoryDownsamplesAndKeepsUnknownApartFromZero(t *testing.T) {
	now := time.Now().UnixMilli()
	window := time.Hour
	samples := []Status{
		{
			GeneratedAtMs: now - 50*60*1000,
			Slots: []SlotView{
				{Slot: 1, PeerID: "p1", Live: true, StatsKnown: true, SpeciesKnown: true,
					Species: []contractb.CensusEntry{entry("Izus ", "copedylanus", 12, 0)}},
				// Reporting, and holding none of it: a genuine zero.
				{Slot: 2, PeerID: "p2", Live: true, StatsKnown: true, SpeciesKnown: true,
					Species: []contractb.CensusEntry{entry("Other", "thing", 4, 0)}},
				// Reporting NOTHING: unknown.
				{Slot: 3, PeerID: "p3", Live: true, StatsKnown: true, SpeciesKnown: false},
			},
		},
		{
			GeneratedAtMs: now - 10*60*1000,
			Slots: []SlotView{
				{Slot: 1, PeerID: "p1", Live: true, StatsKnown: true, SpeciesKnown: true,
					Species: []contractb.CensusEntry{entry("Izus", "copedylanus", 18, 0)}},
				{Slot: 2, PeerID: "p2", Live: false, StatsKnown: true, SpeciesKnown: true,
					Species: []contractb.CensusEntry{}},
				{Slot: 3, PeerID: "p3", Live: true, StatsKnown: false},
			},
		},
	}
	h := BuildSpeciesHistory(samples, "Izus copedylanus", now, window, 12)
	if h.Key != "Izus copedylanus" {
		t.Fatalf("key = %q", h.Key)
	}
	if h.Samples != 2 {
		t.Fatalf("samples = %d, want 2", h.Samples)
	}
	if h.Max != 18 {
		t.Fatalf("max = %d, want 18 — the shared y-scale is the largest per-world reading",
			h.Max)
	}
	byslot := map[int]HistorySeries{}
	for _, s := range h.Slots {
		byslot[s.Slot] = s
	}
	// Slot 1 counted BOTH whitespace spellings under the one compared key.
	if byslot[1].Last == nil || *byslot[1].Last != 18 {
		t.Fatalf("slot 1 last = %v, want 18", byslot[1].Last)
	}
	// Slot 2 reported and held none: zero, a reading.
	var s2zero bool
	for _, p := range byslot[2].Points {
		if p.Value != nil && *p.Value == 0 {
			s2zero = true
		}
	}
	if !s2zero {
		t.Fatalf("a world that reported a census without this species has no zero: %+v",
			byslot[2].Points)
	}
	// Slot 3 never reported: every bucket is null, never zero.
	for _, p := range byslot[3].Points {
		if p.Value != nil {
			t.Fatalf("a world that reported no census produced a value: %+v", p)
		}
	}
	// And the dark bucket is marked, because a healed map hiding a dead world is
	// Risk 5 and the strip is where an operator sees the hour it happened.
	var dark bool
	for _, p := range byslot[2].Points {
		if p.Dark {
			dark = true
		}
	}
	if !dark {
		t.Fatal("a bucket in which the world was seen not live is not marked dark")
	}
}

// TestSpeciesHistoryClampsItsBounds is the other half of the endpoint: a reader
// may ask for a different window, and may not ask the archive to read an
// unbounded file or build an unbounded answer.
func TestSpeciesHistoryClampsItsBounds(t *testing.T) {
	now := time.Now().UnixMilli()
	for _, tc := range []struct {
		window          time.Duration
		buckets         int
		wantWindow      time.Duration
		wantBucketCount int
	}{
		{time.Second, 1, HistoryMinWindow, HistoryMinBuckets},
		{100 * 24 * time.Hour, 10000, HistoryMaxWindow, HistoryMaxBuckets},
		{time.Hour, 60, time.Hour, 60},
	} {
		h := BuildSpeciesHistory(nil, "Izus copedylanus", now, tc.window, tc.buckets)
		if h.Buckets != tc.wantBucketCount {
			t.Fatalf("buckets(%v) = %d, want %d", tc.buckets, h.Buckets, tc.wantBucketCount)
		}
		if got := time.Duration(h.ToMs-h.FromMs) * time.Millisecond; got != tc.wantWindow {
			t.Fatalf("window(%v) = %v, want %v", tc.window, got, tc.wantWindow)
		}
		if len(h.Total) != tc.wantBucketCount {
			t.Fatalf("total series is %d long, want %d", len(h.Total), tc.wantBucketCount)
		}
	}

	// And the key parameter is bounded and normalized at the door: a caller may
	// send a raw spelling, and may not send an unbounded string.
	if _, ok := speciesKeyParam(httptest.NewRequest("GET", "/api/species/history", nil)); ok {
		t.Fatal("a request with no key was accepted")
	}
	long := "/api/species/history?key=" + strings.Repeat("x", maxSpeciesKeyBytes+1)
	if _, ok := speciesKeyParam(httptest.NewRequest("GET", long, nil)); ok {
		t.Fatal("an over-long key was accepted")
	}
	got, ok := speciesKeyParam(httptest.NewRequest("GET",
		"/api/species/history?key=%20Izus%20%20copedylanus%20", nil))
	if !ok || got != "Izus copedylanus" {
		t.Fatalf("a raw spelling was not normalized at the door: %q, %v", got, ok)
	}
}

// TestSpeciesHistoryEndpointRefusesAKeylessRequest covers the HTTP half, so a
// bad request is a 400 rather than an expensive file read for nothing.
func TestSpeciesHistoryEndpointRefusesAKeylessRequest(t *testing.T) {
	a := newViewFixture(t, contractb.PeerStatus{}, time.Second)
	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/species/history")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a keyless request answered HTTP %d, want 400", resp.StatusCode)
	}

	ok, err := http.Get(srv.URL + "/api/species/history?key=Izus+copedylanus&hours=2&buckets=20")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer ok.Body.Close()
	var h SpeciesHistory
	if err := json.NewDecoder(ok.Body).Decode(&h); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if h.Buckets != 20 || h.Key != "Izus copedylanus" {
		t.Fatalf("history = %+v", h)
	}
}

// TestSettingsReachTheSlotViewUntouched is the archive's whole obligation under
// §19 B18: carry the seven fields through StatusView, render an absent one as
// unknown, and never send one back. The wire half is covered end to end in the
// sidecar package; this is the honesty half, on the view the page reads.
func TestSettingsReachTheSlotViewUntouched(t *testing.T) {
	full := census(10, 0, entry("Izus", "copedylanus", 10, 0))
	full.ModVersion = "0.2.0"
	full.ContractAVersion = "contract-a/2.3"
	full.MigrationExclude = &wire.ExcludeList{Names: []string{"Basic bibite"}}
	full.SaveMinutes = f64(0)
	full.SaveKeep = contractb.IntPtr(6)
	full.SaveOnQuit = boolPtr(true)
	full.WorldWrapping = boolPtr(false)

	// A peer whose mod predates §19: two version strings, no settings. This is
	// the state the running deployment is in until the far end's bundle moves,
	// and it must read as unknown throughout without losing anything else.
	old := census(4, 0, entry("Izus", "copedylanus", 4, 0))
	old.ModVersion = "0.1.9"
	old.ContractAVersion = "contract-a/2.2"

	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, full), slot(2, 1, 0, true, old)},
	}
	view := newViewFixture(t, status, time.Second).StatusView()

	v := view.Slots[0]
	if v.ModVersion != "0.2.0" || v.ContractAVersion != "contract-a/2.3" {
		t.Fatalf("versions = %q / %q", v.ModVersion, v.ContractAVersion)
	}
	if !v.MigrationExcludeKnown || len(v.MigrationExclude) != 1 ||
		v.MigrationExclude[0] != "Basic bibite" {
		t.Fatalf("exclusion list = %v / %+v", v.MigrationExcludeKnown, v.MigrationExclude)
	}
	if v.SaveMinutes == nil || *v.SaveMinutes != 0 {
		t.Fatal("saveMinutes 0 was folded into absence; the timer being off is a reading")
	}
	if v.SaveKeep == nil || *v.SaveKeep != 6 || v.SaveOnQuit == nil || !*v.SaveOnQuit {
		t.Fatalf("save policy = %v / %v", v.SaveKeep, v.SaveOnQuit)
	}
	if v.WorldWrapping == nil || *v.WorldWrapping {
		t.Fatal("worldWrapping false did not survive")
	}

	// The older peer: unknown settings, exact everything else, and the two
	// version strings that make the unknown SELF-EXPLAINING.
	o := view.Slots[1]
	if o.MigrationExcludeKnown || o.SaveMinutes != nil || o.SaveKeep != nil ||
		o.SaveOnQuit != nil || o.WorldWrapping != nil {
		t.Fatalf("an older mod produced settings from nowhere: %+v", o)
	}
	if o.ContractAVersion != "contract-a/2.2" {
		t.Fatalf("contractAVersion = %q; it is what turns this slot's unknowns into a fact",
			o.ContractAVersion)
	}
	if o.Population == nil || !o.SpeciesKnown {
		t.Fatal("an older mod lost the fields it does publish")
	}

	// A STALE block hides the settings with everything else: one block, one
	// timestamp, one truth.
	stale := newViewFixture(t, status, 5*time.Minute).StatusView()
	if stale.Slots[0].MigrationExcludeKnown || stale.Slots[0].SaveMinutes != nil ||
		stale.Slots[0].ModVersion != "" {
		t.Fatalf("a stale block published settings as state: %+v", stale.Slots[0])
	}
}

func f64(v float64) *float64 { return &v }
func boolPtr(v bool) *bool   { return &v }

// TestRingstatRendersTheSameTwoViews is WP5's rule applied to the new tabs: the
// terminal tool renders EXACTLY the data the page renders, off the same Status,
// so the two operator surfaces cannot disagree.
func TestRingstatRendersTheSameTwoViews(t *testing.T) {
	full := census(10, 1, entry("Izus ", "copedylanus", 10, 1))
	full.ModVersion = "0.2.0"
	full.ContractAVersion = "contract-a/2.3"
	full.MigrationExclude = &wire.ExcludeList{Names: []string{"Basic bibite"}}
	full.SaveMinutes = f64(0)
	full.WorldWrapping = boolPtr(false)
	// A second world with an EMPTY exclusion list — the policy is off — and no
	// settings otherwise.
	off := census(4, 0, entry("Izus", "copedylanus", 4, 0))
	off.MigrationExclude = &wire.ExcludeList{Names: []string{}}

	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, full), slot(2, 1, 0, true, off)},
	}
	view := newViewFixture(t, status, time.Second).StatusView()

	var settings strings.Builder
	RenderSettings(&settings, view)
	out := settings.String()
	for _, want := range []string{"READ-ONLY", "contract-a/2.3", "OFF", "Basic bibite",
		"the exclusion policy is off", "?"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the settings view never says %q:\n%s", want, out)
		}
	}
	// The one substitution called out by name: saveMinutes must never render as
	// the value the mod ships with.
	if strings.Contains(out, " 10m ") {
		t.Fatalf("the settings view printed a save interval nobody published:\n%s", out)
	}

	var species strings.Builder
	RenderSpecies(&species, view, nil)
	sp := species.String()
	// The RAW spelling, and the alive-only rule, said out loud.
	for _, want := range []string{"Izus  copedylanus", "alive right now", "n/a", "everywhere"} {
		if !strings.Contains(sp, want) {
			t.Fatalf("the species view never says %q:\n%s", want, sp)
		}
	}
	if strings.Contains(sp, "Ghost") {
		t.Fatalf("the species view listed something no world reports:\n%s", sp)
	}
}

// TestTheTrendAnswerFoldsEveryLivingSpeciesInOnePass is the sparkline column's
// whole reason to exist as its own endpoint.
//
// The obvious way to draw a trend beside forty living species is forty requests
// to /api/species/history, which is forty bounded tail reads of the sample file
// on a tab a reader leaves open. This folds the whole living set in ONE pass
// over ONE read — and it keeps the two readings the rest of this file keeps
// apart: a bucket where a world reported a census WITHOUT this species is a
// zero, and a bucket where no world reported one at all is UNKNOWN.
func TestTheTrendAnswerFoldsEveryLivingSpeciesInOnePass(t *testing.T) {
	now := int64(10_000_000)
	window := time.Hour
	// HistoryMinBuckets is the floor every reader of this file is clamped to, so
	// the fixture asks for exactly it and fills the first three.
	buckets := HistoryMinBuckets
	bucketMs := window.Milliseconds() / int64(buckets)
	from := now - bucketMs*int64(buckets)

	samples := []Status{
		{
			GeneratedAtMs: from + bucketMs/2, // bucket 0
			Slots: []SlotView{
				{Slot: 1, StatsKnown: true, SpeciesKnown: true, Live: true,
					Species: []contractb.CensusEntry{entry("Izus", "copedylanus", 12, 0),
						entry("Other", "thing", 3, 0)}},
				{Slot: 2, StatsKnown: true, SpeciesKnown: true, Live: true,
					Species: []contractb.CensusEntry{entry("Izus ", "copedylanus", 8, 0)}},
			},
		},
		{
			GeneratedAtMs: from + bucketMs + bucketMs/2, // bucket 1: reported, none held
			Slots: []SlotView{
				{Slot: 1, StatsKnown: true, SpeciesKnown: true, Live: true,
					Species: []contractb.CensusEntry{entry("Other", "thing", 5, 0)}},
			},
		},
		{
			GeneratedAtMs: from + 2*bucketMs + bucketMs/2, // bucket 2: nobody reported a census
			Slots: []SlotView{
				{Slot: 1, StatsKnown: true, SpeciesKnown: false, Live: true},
				{Slot: 2, StatsKnown: false, Live: false},
			},
		},
	}

	tr := BuildSpeciesTrends(samples, []string{"Izus copedylanus", "Other thing", "Gone away"},
		now, window, buckets)
	if tr.Buckets != buckets || tr.Samples != 3 {
		t.Fatalf("the answer covers %d buckets over %d samples, want %d and 3",
			tr.Buckets, tr.Samples, buckets)
	}
	if len(tr.Species) != 3 {
		t.Fatalf("the answer holds %d species, want the three asked for", len(tr.Species))
	}
	byKey := map[string]SpeciesTrend{}
	for _, s := range tr.Species {
		byKey[s.Key] = s
	}
	izus := byKey["Izus copedylanus"]
	if izus.Points[0] == nil || *izus.Points[0] != 20 {
		t.Fatalf("bucket 0 = %v, want the two worlds summed under ONE compared spelling",
			izus.Points[0])
	}
	// A world that reported and held none of it is a ZERO, and it is a reading.
	if izus.Points[1] == nil || *izus.Points[1] != 0 {
		t.Fatalf("bucket 1 = %v, want a measured zero", izus.Points[1])
	}
	// A bucket in which NO world was in a position to count is UNKNOWN, and a
	// sparkline has to break there rather than draw a line through it.
	if izus.Points[2] != nil {
		t.Fatalf("bucket 2 = %v, want unknown — no world reported a census in it",
			*izus.Points[2])
	}
	for i := 3; i < buckets; i++ {
		if izus.Points[i] != nil {
			t.Fatalf("bucket %d = %v, want unknown — no sample fell in it", i, *izus.Points[i])
		}
	}
	// The scale is PER SPECIES: these are shapes drawn 84 pixels wide beside rows
	// of wildly different abundance, and one shared scale would flatten every
	// rare species into a straight line.
	if izus.Max != 20 || byKey["Other thing"].Max != 5 {
		t.Fatalf("per-species maxima are %d and %d, want 20 and 5",
			izus.Max, byKey["Other thing"].Max)
	}
	if izus.Last == nil || *izus.Last != 0 {
		t.Fatalf("last = %v, want the newest KNOWN value", izus.Last)
	}
	// A species nothing in the window ever knew about carries a whole row of
	// unknowns rather than a row of zeroes.
	for i, p := range byKey["Gone away"].Points {
		if p != nil && *p != 0 {
			t.Fatalf("an unseen species has %d in bucket %d", *p, i)
		}
	}
	if byKey["Gone away"].Max != 0 {
		t.Fatalf("an unseen species has a maximum of %d", byKey["Gone away"].Max)
	}
}

// TestTheTrendEndpointIsAliveOnlyAndBounded pins the endpoint around that
// function: the key set is the LIVING CENSUS UNION and the request's bounds are
// clamped exactly as /api/history's are.
func TestTheTrendEndpointIsAliveOnlyAndBounded(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{
			slot(1, 0, 0, true, census(20, 0, entry("Izus", "copedylanus", 20, 0))),
			slot(2, 1, 0, true, census(4, 0, entry("Other", "thing", 4, 0))),
		},
	}
	a := newViewFixture(t, status, time.Second)
	a.mu.Lock()
	// A species that has crossed a great deal and is alive nowhere. It is in the
	// ledger aggregate and it must NOT be in this answer: the same refusal the
	// flat index makes.
	a.observeSpeciesLocked(migration(time.Now().UnixMilli(), 1, 2, "E", "Extinct", "one", "h1"))
	a.mu.Unlock()

	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)
	body := get(t, srv.URL+"/api/species/trends?hours=24&buckets=32")
	var tr SpeciesTrends
	if err := json.Unmarshal([]byte(body), &tr); err != nil {
		t.Fatalf("decode: %v\n%s", err, body)
	}
	if len(tr.Species) != 2 {
		t.Fatalf("the answer holds %d species, want the two that are ALIVE: %s", len(tr.Species), body)
	}
	if strings.Contains(body, "Extinct") {
		t.Fatalf("a species alive nowhere was given a trend line:\n%s", body)
	}
	if tr.Buckets != 32 {
		t.Fatalf("buckets = %d, want the 32 asked for", tr.Buckets)
	}
	// Clamped, like every other reader of that file: a caller may ask for a
	// different window and may not ask for an unbounded answer.
	var wide SpeciesTrends
	if err := json.Unmarshal([]byte(get(t, srv.URL+"/api/species/trends?hours=9000&buckets=99999")),
		&wide); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if wide.Buckets != HistoryMaxBuckets {
		t.Fatalf("buckets = %d, want the clamp at %d", wide.Buckets, HistoryMaxBuckets)
	}
	if wide.ToMs-wide.FromMs > HistoryMaxWindow.Milliseconds() {
		t.Fatalf("the window spans %d ms, past the clamp", wide.ToMs-wide.FromMs)
	}
}

// TestTheLatestGenomeHashIsTrackedPerSpecies is the one string the brain shape
// is read through, and it belongs to the same single writer as every other
// counter in this file — one call site for the replay, one for the live path.
func TestTheLatestGenomeHashIsTrackedPerSpecies(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatalf("OpenLedger: %v", err)
	}
	base := time.Now().Add(-time.Hour).UnixMilli()
	recs := []Record{
		migration(base, 1, 2, "E", "Izus", "copedylanus", "h-old"),
		migration(base+2000, 1, 2, "E", "Izus", "copedylanus", "h-new"),
		// An OLDER record arriving later does not overwrite a newer answer.
		migration(base+1000, 1, 2, "E", "Izus", "copedylanus", "h-middle"),
		// A crossing carrying no genome at all leaves the answer alone.
		{Type: RecordMigration, RecordedAt: base + 3000, MigrationID: "m-nogenome",
			SourceSlot: 1, DestSlot: 2,
			Species: &wire.Species{GenericName: "Izus", SpecificName: "copedylanus"}},
	}
	for _, rec := range recs {
		if err := ledger.Append(rec); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := ledger.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// THE LIVE PATH and THE REPLAY must agree, because they are the same
	// function called from two places.
	live := newViewFixture(t, contractb.PeerStatus{}, time.Second)
	live.mu.Lock()
	for _, rec := range recs {
		live.observeSpeciesLocked(rec)
	}
	got := live.species.byKey["Izus copedylanus"].genomeHash
	live.mu.Unlock()
	if got != "h-new" {
		t.Fatalf("the live path holds %q, want the LATEST genome the record named", got)
	}

	replayed, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = replayed.Close() })
	replayed.mu.Lock()
	rebuilt := replayed.species.byKey["Izus copedylanus"].genomeHash
	replayed.mu.Unlock()
	if rebuilt != got {
		t.Fatalf("the replay rebuilt %q against the live path's %q", rebuilt, got)
	}
}
