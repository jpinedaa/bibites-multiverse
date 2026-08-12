package archive

import (
	"strings"
	"testing"

	"multiverse/internal/contractb"
)

// TestPageHasThreeTabsOverOnePoll is the structural half of the Species and
// Settings tabs. A Go test cannot run the page's JavaScript, so it asserts the
// MACHINERY three views over one poll need, and the properties that make the
// tabs usable rather than merely present:
//
//	THE TAB IS IN THE URL HASH, so "#species" is a link somebody can send and a
//	reload lands where the reader was — including "#tree", which was the
//	genealogy's own tab before it and the census list became one drawing and is
//	still a link somebody sent.
//
//	THE STATUS POLL IS SHARED. One timer fetches /api/status for all three; a
//	tab that wanted its own would ask the archive for the same frame three
//	times, and the header's numbers would disagree with themselves.
func TestPageHasThreeTabsOverOnePoll(t *testing.T) {
	page := statusPageHTML

	for _, want := range []string{
		`<nav class="tabs"`, `data-tab="map"`, `data-tab="species"`, `data-tab="settings"`,
		`id="p-map"`, `id="p-species"`, `id="p-settings"`,
		"function showTab", "function tabFromHash", "function wireTabs",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("the tab machinery is missing %q", want)
		}
	}
	// THE TWO OLD TABS ARE GONE, and gone means gone: no panel, no button, no
	// second renderer for the same alive union.
	for _, forbidden := range []string{`data-tab="tree"`, `id="p-tree"`, `id="spbody"`,
		`id="sptab"`, "function renderSpecies", "function renderTree", "function speciesRow",
		"function speciesDetail"} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("the merged view left %q behind; two renderings of one alive union is two "+
				"places for it to be wrong", forbidden)
		}
	}
	// The old genealogy link still lands on the view the genealogy went into.
	if !strings.Contains(page, `if (h === "tree") return "species";`) {
		t.Fatal("#tree is not redirected to the merged view; it is a link somebody sent")
	}
	// The hash is both read and written, and a hashchange under an open page
	// moves the view — that is what makes the back button work.
	for _, want := range []string{"location.hash", `window.addEventListener("hashchange"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("the tab is not in the URL: missing %q", want)
		}
	}
	// ONE status poll, on one interval, for all three views.
	if !strings.Contains(page, "tick(); setInterval(tick, 2000);") {
		t.Fatal("the shared status poll is gone")
	}
	if strings.Count(page, `fetch("api/status"`) != 1 {
		t.Fatal("more than one place fetches the status frame; three tabs share ONE poll")
	}
	// The species view rides that same cycle rather than a timer of its own, and
	// is gated on its own tab: it is derived from a ledger the browser is never
	// handed, so it costs the archive work and is worth nothing to a tab nobody
	// has open.
	if !strings.Contains(page, `if (TAB === "species") await tickLife();`) {
		t.Fatal("the species view does not ride the shared poll, or is not gated on its tab")
	}
	if strings.Count(page, `fetch("api/species/tree"`) != 1 {
		t.Fatal("the species view is fetched from more than one place")
	}
	// ONE TREND REQUEST FOR EVERY ROW, on a slow timer of its own. Per-species
	// history requests would be one bounded read of the sample file per species
	// on a tab a reader leaves open.
	if strings.Count(page, `fetch("api/species/trends`) != 1 {
		t.Fatal("the trend column is not fetched exactly once for the whole view")
	}
	if strings.Contains(page, `fetch("api/species/history`) {
		t.Fatal("the page fetches a per-species history again; the trend column exists so that " +
			"forty sparklines cost one request")
	}
	if !strings.Contains(page, "tickTrends(); setInterval(tickTrends, 60000);") {
		t.Fatal("the trend column is polled on the two-second cycle, or not at all")
	}
	// And the two map-only feeds are gated on the map being visible: a hidden
	// panel has no laid-out geometry to animate along.
	if !strings.Contains(page, `if (TAB !== "map") return;`) {
		t.Fatal("the hop feed and history strip are not gated on the map being visible")
	}
	// Coming back to the map REBUILDS it. An SVG in a hidden panel cannot be
	// measured, and every lane label and hop path is built from getTotalLength.
	if !strings.Contains(page, `mapSig = "";`) || !strings.Contains(page, "hopsPrimed = false;") {
		t.Fatal("returning to the map neither rebuilds it nor re-primes the hop seen-set; " +
			"the second is what stops a tab switch replaying the last minute")
	}
	// The page is still SELF-CONTAINED after all of it.
	fetchable := strings.ReplaceAll(page, "http://www.w3.org/2000/svg", "")
	for _, forbidden := range []string{"http://", "https://", "//cdn", "<link", "<script src",
		"@import", "url(http"} {
		if strings.Contains(fetchable, forbidden) {
			t.Fatalf("the tabs reached outside the page (%q)", forbidden)
		}
	}
}

// TestTheSpeciesViewIsAliveOnlyAndSaysSo covers the merged view's rendering and
// the one claim it must not make. The rows are the CENSUS UNION: a species that
// crossed a lane a hundred times and is extinct everywhere is not a row of its
// own, and the page has to say that in its own words rather than leave a reader
// to discover it.
func TestTheSpeciesViewIsAliveOnlyAndSaysSo(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	for _, want := range []string{
		"function renderLife", "function lfRow", "function lfDetail", "function lfDetailLines",
		"function lfRows", "function lfMatches", "function lfBar", "function lfEdge",
		"function lfMini", "function lfSpark", "function lfAxis", "function lfTip",
		"function lfStats", "function lfCount", "function lfScale", "function lfCols",
	} {
		if !strings.Contains(region, want) {
			t.Fatalf("the species view is missing %q", want)
		}
	}
	for _, want := range []string{`id="lfq"`, `id="lfsort"`, `id="lfbox"`, `id="lfstat"`,
		`id="lfcount"`, `fetch("api/species/tree"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("the species view is missing %q", want)
		}
	}
	// THE FLAT RANKING SURVIVES AS A SORT of the same rows. The family order is
	// the default, because it is the thing the flat table could not say.
	for _, want := range []string{`value="family"`, `value="pop"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("the species view cannot be ordered by %q", want)
		}
	}
	if !strings.Contains(page, `lfSort = "family"`) {
		t.Fatal("the species view does not default to the family order")
	}
	// A flat ranking draws no parent edges: rows in abundance order are not in
	// family order, and a line between two of them would say nothing.
	if !strings.Contains(region, "return {list: flat, joined: false,") {
		t.Fatal("the population ranking still claims its rows are joined")
	}
	// The three census badges, each with a glossary entry behind it.
	for _, want := range []string{`"NEVER EXPORTED"`, `"EVERYWHERE"`, `"ENDEMIC"`} {
		if !strings.Contains(region, want) {
			t.Fatalf("a census badge is missing from the merged row: %q", want)
		}
	}
	for _, want := range []string{" endemic:[", " everywhere:[", " excluded:["} {
		if !strings.Contains(page, want) {
			t.Fatalf("a badge's glossary entry is missing: %q", want)
		}
	}
	// The alive-only rule is stated on the page, in the glossary and in the
	// section's own note.
	if !strings.Contains(page, " alive:[") {
		t.Fatal("the glossary never explains that the view is alive-only")
	}
	if !strings.Contains(page, "species alive right now") {
		t.Fatal("the species section does not say the list is what is alive")
	}
	// THE CENSUS FACTS ARE ON THE ROW, which is the whole of the merge: the
	// per-world counts and eggs the flat table showed are read here.
	for _, want := range []string{"worlds[i].bibites", "worlds[i].eggs", "n.population",
		"n.eggs", "n.spellings", "n.recent", "n.parentName", "n.excludedBy"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the merged row never reads %q; the flat census tab showed it", want)
		}
	}
	// The glyph is the SAME creature the map draws, drawn by reference.
	if !strings.Contains(region, `u.setAttribute("href", "#bib")`) {
		t.Fatal("the species view draws its own creature instead of the shared glyph")
	}
	// One species is one colour everywhere, from the same hash of the same
	// compared spelling the map uses.
	if !strings.Contains(region, "speciesColor(n.key)") {
		t.Fatal("the species view colours a row from something other than the compared name; " +
			"a species must be the same colour on every view")
	}
	// AND THE GLYPH IS THE ONLY THING WEARING IT. A swatch beside it, or a bar
	// tinted to match, repeats one fact twice and invites a reader to look for a
	// second meaning in the second mark. ONE call site inside the view, and the
	// bar's own fill is a page colour rather than a species one.
	view := region[strings.Index(region, "function renderLife"):]
	if n := strings.Count(region, "speciesColor(n.key)"); n != 1 {
		t.Fatalf("the merged row colours %d things by species; the glyph is the sole colour "+
			"carrier", n)
	}
	if strings.Contains(view, "speciesColor") {
		t.Fatal("something in the drawing other than the row's glyph is coloured by species")
	}
	if strings.Contains(page, ".spglyph{") || strings.Contains(page, "function speciesGlyph") ||
		strings.Contains(region, `el("i", "sw")`) {
		t.Fatal("the species row still carries the swatch the glyph replaced")
	}
	for _, want := range []string{"svg.life .lfbar.live{fill:var(--text)",
		"svg.life .lfbar.ext{fill:var(--dim)"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the lifespan bar is not drawn in a page colour (%q); a bar tinted to the "+
				"species repeats what the glyph already said", want)
		}
	}
}

// TestTheSpeciesViewDrawsTheRecordAgainstTime is the merged view's own idea: the
// horizontal axis is wall-clock time and a species is a bar on it.
//
// A BAR IS THE SPAN OF THE RECORD AND NOT A LIFESPAN, and that is the one thing
// this drawing can be misread as. The page must take both ends from the server's
// own layout data (so a terminal renderer and a browser cannot disagree), must
// draw a living species to the right-hand edge rather than to its last crossing,
// and must say all of it in words as well as in marks.
func TestTheSpeciesViewDrawsTheRecordAgainstTime(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// The two ends and the axis are the SERVER'S numbers, not the page's.
	for _, want := range []string{"n.spanFromMs", "n.spanToMs", "n.spanDerived",
		"x.spanStartMs", "x.spanEndMs"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the drawing does not read the published layout datum %q", want)
		}
	}
	// A living species runs to the edge of the picture; an extinct one stops.
	if !strings.Contains(region, "var x1 = n.alive ? sc.x(sc.t1) : sc.x(n.spanToMs || n.spanFromMs);") {
		t.Fatal("a living species' bar does not run to now, or an extinct one does")
	}
	// The join is the CHILD's first-seen point, which is the only x-coordinate on
	// the drawing that means anything.
	if !strings.Contains(region, "var jx = sc.x(n.spanFromMs);") {
		t.Fatal("the parent edge does not drop at the child's first-seen point")
	}
	// THE COLLAPSED RUN IS A DOTTED LEAD-IN WHOSE LENGTH COUNTS GENERATIONS, and
	// the number is printed beside it anyway — a length a reader has to measure
	// is not a number, and a length on a time axis that is not time has to say so.
	if !strings.Contains(region,
		"return Math.max(LF_CHAINMIN, Math.min(LF_CHAINMAX, gens * LF_GENPX));") {
		t.Fatal("the collapsed run's length is not a bounded multiple of the generations it " +
			"stands for")
	}
	for _, want := range []string{"var LF_GENPX = 5, LF_CHAINMIN = 12, LF_CHAINMAX = 130;",
		"lfChainLen(n.collapsed)", `svgEl("path", "chain")`, `gt.textContent = "+" + n.collapsed;`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the collapsed edge is missing %q", want)
		}
	}
	if !strings.Contains(page, "generations and not time") {
		t.Fatal("the glossary never says the dotted length is generations rather than time")
	}
	// The record's floor is a BOUNDARY on the picture, not only a caption.
	//
	// AND THE TEST IS >=, WHICH IS THE WHOLE MARK. The server clamps the axis down
	// to the floor so the floor is always inside the picture, so on every map whose
	// oldest drawn bar is younger than the floor the two are EXACTLY EQUAL — the
	// running rig's own case, where a strict > drew the boundary on no map at all.
	for _, want := range []string{`svgEl("line", "floor")`, `svgEl("rect", "prefloor")`,
		"x.ancestrySinceMs && x.ancestrySinceMs >= sc.t0"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the record's floor is not drawn as a boundary: %q missing", want)
		}
	}
	if strings.Contains(region, "x.ancestrySinceMs > sc.t0") {
		t.Fatal("the floor is gated on a STRICT >; the axis is clamped down to the floor, so " +
			"equality is the common case and a strict test hides the boundary exactly when it " +
			"is the boundary")
	}
	// And the axis is UTC and dated, because it is a fixed point weeks back and a
	// local rendering would put two readers a day apart on the same fact.
	for _, want := range []string{"function trDay", "function trClock", "function lfStep"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the time axis is missing %q", want)
		}
	}
	// SAID IN WORDS TOO. The bar is the record's span, and the page says so on
	// the tab, in every row's tooltip and in the glossary.
	if !strings.Contains(page, " lifespan:[") {
		t.Fatal("the glossary never explains what the bar is and is not")
	}
	if !strings.Contains(region, "The bar runs from the first crossing this archive recorded of it") {
		t.Fatal("a row's tooltip never says what its bar measures")
	}
	if !strings.Contains(region, "when the RECORD of it stops, not when it died.") {
		t.Fatal("the tooltip never holds the record's end apart from the species' end")
	}
	// A derived start is drawn as the inference it is, and never as a reading.
	if !strings.Contains(region, `n.spanDerived ? " derived" : ""`) {
		t.Fatal("a bar whose start was inferred from a descendant is drawn like a recorded one")
	}
}

// TestTheSpeciesViewDrawsTheMiniMapAndTheBrain covers the two marks that are not
// time: where a species lives, and how big its brain is.
//
// THE GRID IS THE MAP'S OWN SHAPE. Six dots in two rows because this map is
// three by two — never because six is how many worlds there are — so the page
// reads the published grid rather than a constant.
//
// THE BRAIN IS ABSENT WHEN IT IS ABSENT. A genome pruned past the retention
// horizon leaves no ring at all, which cannot be misread as a small brain the
// way a thin bar could.
func TestTheSpeciesViewDrawsTheMiniMapAndTheBrain(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// The grid comes from the answer, and both of its dimensions are used.
	for _, want := range []string{"var m = (x && x.map) || {}", "m.width", "m.height",
		"c.col", "c.row", "c.reporting", "cells.length"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the mini-map does not read the map's own shape: %q missing", want)
		}
	}
	// Row 0 is the BOTTOM row of the map and it is the bottom row here too.
	if !strings.Contains(region, "(rows - 1 - c.row) * pitch") {
		t.Fatal("the mini-map does not put row 0 at the bottom, as the map does")
	}
	// THREE STATES, THREE FACTS: alive there, reported-and-absent, and a world
	// reporting no census at all — which is unknown and never an absence.
	if !strings.Contains(region,
		`have[c.slot] ? "wdot on"`) || !strings.Contains(region, `(c.reporting ? "wdot off" : "wdot unk")`) {
		t.Fatal("the mini-map collapses 'not there' and 'we were not told' into one dot")
	}
	if !strings.Contains(page, "svg.life .wdot.unk{fill:none;stroke:var(--warn)") {
		t.Fatal("the unknown dot is not drawn as an unknown")
	}
	// The brain: a ring sized from the two published figures, and NOTHING when
	// they are absent.
	for _, want := range []string{"function lfBrainR", "n.neurons", "n.synapses",
		"var LF_BRAIN_R0 = 2.4, LF_BRAIN_R1 = 6.4, LF_BRAIN_FULL = 600;",
		"if (w <= 0) return 0;", `svgEl("circle", "brain")`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the brain ring is missing %q", want)
		}
	}
	if !strings.Contains(region, "var r = lfBrainR(n);\n  if (r > 0){") {
		t.Fatal("a species with no stored genome is still given a ring; absent must draw nothing")
	}
	if !strings.Contains(region, "no copy of its latest genome is held here") {
		t.Fatal("the expanded row never says WHY a brain is missing; a pruned genome is not a " +
			"brain of no neurons")
	}
	// Both carry a glossary entry, like every other piece of jargon here.
	for _, want := range []string{" brainsize:[", " minimap:[", " trend:["} {
		if !strings.Contains(page, want) {
			t.Fatalf("the glossary never explains %q", want)
		}
	}
	if !strings.Contains(page, `"minimap","trend","brainsize",`) {
		t.Fatal("the new glossary entries exist but are never listed, so nobody can read them")
	}
}

// TestSettingsTabIsReadOnlyAndSaysUnknown covers contract-b-m4.md §19 B19's two
// page rules, which are the whole of what the settings view owes a reader:
//
//	UNKNOWN, OUT LOUD, WITH ONE DEFAULT NAMED. A field no world has published
//	renders "?", and saveMinutes must NEVER render as 10 — the value the mod
//	ships with — because that claims a world is being saved when its timer may
//	be off.
//
//	READ-ONLY, AND STATED ON THE PAGE. The view offers no control, and says so,
//	because a control surface is a separate design and not an extension of an
//	OPTIONAL field on a stats block (contract-a.md §19, A43).
func TestSettingsTabIsReadOnlyAndSaysUnknown(t *testing.T) {
	page := statusPageHTML

	for _, want := range []string{"function renderSettings", "function settingsCard",
		"function setKV", `id="setcards"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("the settings tab is missing %q", want)
		}
	}
	// Every one of the seven fields is on the card.
	for _, want := range []string{"v.modVersion", "v.contractAVersion", "v.migrationExclude",
		"v.migrationExcludeKnown", "v.saveMinutes", "v.saveKeep", "v.saveOnQuit",
		"v.worldWrapping"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the settings card never reads %q", want)
		}
	}
	// And so are the numbers they explain — the value of this view is that the
	// cause sits beside the effect.
	for _, want := range []string{"v.gameVersion", "v.simulationSize", "v.exportEdges",
		"v.lastSave", "v.population", "v.eggCount"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the settings card is missing the reading %q explains", want)
		}
	}
	// READ-ONLY, said on the page and explained in the glossary.
	if !strings.Contains(page, "<b>Read-only.</b>") {
		t.Fatal("the settings tab does not state that it is read-only")
	}
	if !strings.Contains(page, " readonly:[") {
		t.Fatal("the glossary never explains why the settings are read-only")
	}
	// There is no control of any kind on the settings panel: nothing posts, and
	// nothing writes.
	for _, forbidden := range []string{"method=\"post\"", "method=post", "<form",
		`fetch("api/settings`, `type="submit"`} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("the settings tab grew a control (%q); a control surface is a separate "+
				"design with its own authentication, authorization, ordering and audit trail",
				forbidden)
		}
	}
	// saveMinutes: 0 is a READING and renders as OFF, and the shipped default is
	// never substituted.
	if !strings.Contains(page, "v.saveMinutes === 0") {
		t.Fatal("saveMinutes 0 is not held apart from absence; the timer being off is the " +
			"explanation for a world that never reports a save")
	}
	if strings.Contains(page, "saveMinutes || 10") || strings.Contains(page, "saveMinutes || ") {
		t.Fatal("the settings tab defaults a save interval nobody published")
	}
	// worldWrapping false is the loud reading, not a gap.
	if !strings.Contains(page, "v.worldWrapping == null") {
		t.Fatal("worldWrapping is tested for truthiness; false names a world that is not " +
			"containing its own organisms and is a reading, not an absence")
	}
	// A present empty exclusion list is a stronger statement than an absent one,
	// and the page makes both.
	for _, want := range []string{"this world has not told us", "the exclusion policy is off"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the settings tab never says %q; absent and empty are different facts", want)
		}
	}
}

// TestSettingsTabDrawsThePublishedCeilings covers contract-b-m4.md §22, B24 and
// B25 on the page: the relay publishes the capacity table it is RUNNING WITH and
// its admission floor on every PEER_STATUS, and §10.1's B24 rule is that they
// render BESIDE the behaviour they bound — a world's message rate is only
// readable against the ceiling it is measured on.
//
// They are drawn on the settings tab, which is where this page keeps what
// something was TOLD to do, and they are the one thing on that tab that is
// AUTHORITATIVE rather than reported: every world card is that world's claim
// about itself, and this card is the relay's own configuration.
func TestSettingsTabDrawsThePublishedCeilings(t *testing.T) {
	page := statusPageHTML

	for _, want := range []string{`id="mapcard"`, "function relayCard", "function limitKV",
		"function limitValue", "d.limits", "d.minContractVersion", "var LIMITS = ["} {
		if !strings.Contains(page, want) {
			t.Fatalf("the map's own card is missing %q", want)
		}
	}
	// Every one of §3.3's eight keys is named, by the spelling the wire uses —
	// which is the spelling a close 4007 quotes back.
	for _, key := range contractb.PublishedLimitKeys {
		if !strings.Contains(page, `"`+key+`"`) {
			t.Fatalf("the card never names the published ceiling %q", key)
		}
	}
	// NO DEFAULT IS SUBSTITUTED ANYWHERE. Every limit is a knob, the relay
	// publishes what it runs with, and a shipped value drawn in place of an
	// unpublished one would be the only number on this page nobody could check.
	// The two byte ceilings are the tell: they appear nowhere in this document.
	for _, forbidden := range []string{"8388608", "4194304"} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("the page carries the shipped default %s; the published table is the "+
				"values the RELAY is running with", forbidden)
		}
	}
	// The two absences are different facts and the page makes both.
	for _, want := range []string{"no minimum — any compatible version",
		"this map publishes no ceilings"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the card never says %q; an unpublished table is UNKNOWN and an "+
				"unpublished floor is the relay's own answer", want)
		}
	}
	// Both have a glossary entry, like every other piece of jargon on the page.
	for _, want := range []string{" ceilings:[", " floor:["} {
		if !strings.Contains(page, want) {
			t.Fatalf("the glossary never explains %q", want)
		}
	}
	// A published key is the relay's own string arriving in a broadcast, so the
	// card is built where markup may not be assigned — the same fence the census
	// names are drawn inside.
	region := speciesRegion(t)
	for _, want := range []string{"function relayCard", "function limitKV", "function limitValue"} {
		if !strings.Contains(region, want) {
			t.Fatalf("%q is OUTSIDE the fenced region; a published key reaches the DOM as a "+
				"text node or not at all", want)
		}
	}
	// And the settings tab actually draws it, on the shared poll like everything
	// else on this page.
	if !strings.Contains(page, "mhost.appendChild(relayCard(d || {}))") {
		t.Fatal("renderSettings never draws the map's own card")
	}
}

// TestExclusionNamesNeverBecomeMarkup extends contract-b-m4.md §13 item 7 to the
// field §19 added.
//
// An exclusion entry is a species name that whoever configured another world
// typed. It arrives on the same unauthenticated stats block a census name does,
// the relay copies it verbatim into a broadcast every client reads, and nothing
// upstream sanitizes it — §19 A42 guarantees the opposite, because the
// published strings must be the exact strings the origin mod compares against.
//
// So it is handled inside the SAME fenced region, and the same structural
// property covers it: that region contains no assignment of markup at all, and
// names reach the DOM only through text nodes and created elements.
func TestExclusionNamesNeverBecomeMarkup(t *testing.T) {
	region := speciesRegion(t)

	// The fence-wide rule (asserted in TestCensusNamesNeverBecomeMarkup) only
	// protects this field if the code that draws it is actually inside the
	// fence. That is what this checks.
	for _, want := range []string{"v.migrationExclude[i]", "function settingsCard",
		"function renderSettings"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the exclusion list is rendered OUTSIDE the fenced region (%q missing "+
				"from it); the escaping rule is enforced by a property over that fence", want)
		}
	}
	// And so is the whole species view, which draws census names by another
	// route than the map does.
	for _, want := range []string{"function renderLife", "function lfRow", "function lfDetail"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the species view is rendered outside the fence: %q", want)
		}
	}
	// NOTHING AFTER THE FENCE TOUCHES A NAME-BEARING FIELD. The rest of the page
	// still builds HTML strings, which is fine — what it must never do is put a
	// name in one.
	page := statusPageHTML
	outside := page[strings.Index(page, "SPECIES CENSUS — END"):]
	for _, forbidden := range []string{"row.name", "row.spellings", ".genericName",
		".specificName", "migrationExclude", "n.spellings", "n.parentName"} {
		if strings.Contains(outside, forbidden) {
			t.Fatalf("code after the fence touches %q; every name-bearing field belongs "+
				"inside the region where markup may not be assigned", forbidden)
		}
	}
	// The peer id is another peer's chosen string and goes the same way.
	if !strings.Contains(region, `el("span", "peer", v.peerId)`) {
		t.Fatal("the settings card interpolates a peer id instead of setting it as text")
	}
}

// TestAncestorNamesNeverBecomeMarkup extends the same structural property to the
// field the family tree added, and it is the one most easily forgotten.
//
// An ANCESTOR'S label is not a census name. No world is reporting the species —
// that is what makes it an ancestor — so the only spelling the archive has is a
// `parentGenericName` off a migration envelope (contract-a.md §16 A30). It is
// the same 64 attacker-chosen bytes as a census name, arriving by a different
// route, with the same guarantee that nothing upstream will repair it (A34
// repairs whitespace at the SOURCE; it does not sanitize). So the whole
// genealogy renderer belongs inside the fence, and this asserts that it is.
func TestAncestorNamesNeverBecomeMarkup(t *testing.T) {
	region := speciesRegion(t)
	for _, want := range []string{"function renderLife", "function lfTip", "function lfStats",
		"function lfDetailLines", "function trSpan", "function svgEl"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the genealogy is drawn OUTSIDE the fenced region (%q missing from it); "+
				"an ancestor's name is attacker-chosen text and reaches the DOM as a text "+
				"node or not at all", want)
		}
	}
	// Names land on tspans through textContent, and the two name-bearing fields
	// of this view never appear after the fence.
	if !strings.Contains(region, "s.textContent = String(text);") {
		t.Fatal("trSpan does not set its text as a text node")
	}
	page := statusPageHTML
	outside := page[strings.Index(page, "SPECIES CENSUS — END"):]
	for _, forbidden := range []string{"n.name", "n.nameFrom", "n.key", "lfTip(",
		"lfDetailLines("} {
		if strings.Contains(outside, forbidden) {
			t.Fatalf("code after the fence touches %q; the genealogy's name-bearing fields "+
				"belong inside the region where markup may not be assigned", forbidden)
		}
	}
	// EVERY NAME THIS VIEW DRAWS goes through one of two calls, and both build a
	// text node: the label and its badges, and the expanded row's lines — where
	// the parent species and the other worlds' spellings are printed.
	if !strings.Contains(region, "var s = trSpan(text, seg.c || null, seg.t);") {
		t.Fatal("the expanded row does not build its segments through trSpan")
	}
	// And the raw spelling is PRESERVED rather than tidied: SVG text collapses a
	// run of spaces exactly as HTML does, and 13% of the rig's names carry stray
	// whitespace (contract-a.md §17, A36).
	for _, want := range []string{"svg.life text.nm{fill:var(--text);white-space:pre}",
		"svg.life .det{fill:var(--text);font-size:11px;white-space:pre}"} {
		if !strings.Contains(page, want) {
			t.Fatalf("a name this view draws does not preserve its own whitespace: %q", want)
		}
	}
}

// TestTheSpeciesViewStatesItsOwnLimit is the page half of tree.go rule 3.
//
// The derivation is honest and PARTIAL: ancestry here is a by-product of travel,
// so a lineage that has never crossed a lane is not connected. That is a
// legitimate property of the record and a misleading one to leave implicit — a
// reader looking at a species standing on its own must be told which of the two
// reasons put it there, and the view must publish the count rather than wait to
// be asked.
func TestTheSpeciesViewStatesItsOwnLimit(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// The limit, in the page's own voice, on the tab and in the footer.
	for _, want := range []string{
		"a lineage\n        that has never crossed a lane has nothing here to connect it",
		"lineages that have never crossed a lane are not connected here",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("the page never states the derivation's limit: %q missing", want)
		}
	}
	// THE TWO REASONS A LEAF STANDS ALONE ARE NEVER GIVEN THE SAME LABEL.
	for _, want := range []string{`"NO LIVING RELATIVE"`, `"NO RECORDED ANCESTRY"`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the tree conflates the two reasons a species stands alone: %q missing",
				want)
		}
	}
	if !strings.Contains(region, "with no ancestry recorded, ") {
		t.Fatal("the counts line does not split the two reasons apart")
	}
	// An ancestor is never drawn as a resident, which is §10.1's rule on the node
	// type most likely to be misread as one.
	if !strings.Contains(region, "Not alive in any world that is reporting a census.") {
		t.Fatal("the tooltip for an extinct ancestor does not say it is not alive")
	}
	// And every new piece of jargon carries a glossary entry, like the rest.
	for _, want := range []string{" genealogy:[", " branchpoint:[", " collapsed:[",
		" noancestry:["} {
		if !strings.Contains(page, want) {
			t.Fatalf("the glossary never explains %q", want)
		}
	}
	// The stale claim the tree replaced is gone: the parent-species entry used to
	// say there was no family tree here.
	if strings.Contains(page, "there is no family tree here") {
		t.Fatal("the parent-species glossary entry still denies the tab that now exists")
	}
}

// TestTheTreeBadgesARootThatIsNotTheBeginning is the page half of tree.go rule
// 3's second paragraph, and it exists because of a question the drawing provoked:
// why is the game's own starting species not at the top of this tree?
//
// A ROOT DRAWN WITH NO MARK READS AS AN ORIGIN. The reduction stops at the
// highest node with a living branch, so a root usually has ancestors — 31 of them
// for `Zhiluus tardisitguyus` on the running rig — that the record holds and the
// drawing collapses. The honest answer is a LABEL SAYING SO, never an invented
// edge down to a species no crossing ever linked, and the label must not be the
// one worn by a species the record can say nothing about at all.
func TestTheTreeBadgesARootThatIsNotTheBeginning(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// The badge, on a ROOT the record reaches above, with the depth in it.
	for _, want := range []string{
		`if (joined && !n.parent && n.ancestryKnown && n.ancestryDepth > 0)`,
		`"THE RECORD BEGINS HERE · "`, `" GENERATIONS ABOVE"`, `" GENERATION ABOVE"`,
		`c: "tbadge rec", term: "recordfloor"`,
	} {
		if !strings.Contains(region, want) {
			t.Fatalf("the tree never badges a root that has recorded ancestry above it: %q "+
				"missing", want)
		}
	}
	// THREE STATES, THREE LABELS, and the new one must not be either warning: a
	// root whose ancestry IS recorded is not a gap in the record, and a reader
	// who sees the same colour and the same voice will read them as one thing.
	if !strings.Contains(page, "svg.life .tbadge.rec{fill:var(--dim)}") {
		t.Fatal("the root badge has no style of its own, or wears the warning colour the two " +
			"standing-alone badges wear")
	}
	// The badge's text is STATIC WORDS AND ONE INTEGER. Every other label on this
	// view is 64 attacker-chosen bytes; this one cannot be, and the check is
	// structural rather than a matter of escaping it correctly.
	start := strings.Index(region, "if (joined && !n.parent && n.ancestryKnown")
	stmt := region[start:]
	if end := strings.Index(stmt, "return out;"); end > 0 {
		stmt = stmt[:end]
	}
	for _, forbidden := range []string{"n.name", "n.key", "n.nameFrom", "row.name"} {
		if strings.Contains(stmt, forbidden) {
			t.Fatalf("the root badge interpolates %q; its text is static plus a number, and "+
				"no name may enter it", forbidden)
		}
	}
	if !strings.Contains(stmt, "n.ancestryDepth") {
		t.Fatal("the root badge prints no generation count; the number is the whole of what " +
			"it adds over silence")
	}

	// The tooltip says what the badge MEANS: those ancestors are recorded and
	// collapsed, and the run ends at the record's edge rather than at an origin.
	for _, want := range []string{
		"Nothing is drawn above it because not one of those ",
		"the run ends where the RECORD ends rather than where the family did",
	} {
		if !strings.Contains(region, want) {
			t.Fatalf("the root's tooltip never explains its badge: %q missing", want)
		}
	}

	// And the tab prints the record's own floor, which is the other half of the
	// answer: the chain stops where the archive's ancestry starts.
	for _, want := range []string{"function trDay", "x.ancestrySinceMs",
		`termEl("span", "recordfloor", "ancestry recorded since")`,
		"the oldest crossing kept here that names a parent"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the tree tab never states the record's floor: %q missing", want)
		}
	}
	// It is a DATE and not a zero: an archive that has never been told a parent
	// prints no clause at all rather than the epoch.
	if !strings.Contains(region, "if (x.ancestrySinceMs){") {
		t.Fatal("the floor clause is not gated on there being a floor")
	}

	// The jargon carries a glossary entry, and the entry is the one that answers
	// the owner's question outright.
	if !strings.Contains(page, " recordfloor:[") {
		t.Fatal("the glossary never explains the root badge")
	}
	if !strings.Contains(page, `"noancestry","recordfloor",`) {
		t.Fatal("the glossary entry exists but is never listed, so nobody can read it")
	}
	if !strings.Contains(page, "is not the root of this tree") {
		t.Fatal("the glossary never answers why the game's starting species is not the root")
	}
}

// TestTheSeedStockIsHiddenAndSaidSo is the page half of the seed-template rule.
//
// The one set of rows this view leaves out on purpose is the seed stock — a
// species every world holding it refuses to export, which on this map is the
// game's own starting template: a full-width bar since the record began, no living
// descendant, no part in anything else on the drawing. It took over the timeline
// and answered nothing.
//
// FOUR PROPERTIES MAKE THAT HONEST RATHER THAN A LIE BY OMISSION:
//
//	THE POLICY IS THE SERVER'S. The page reads a mark on the node; it never
//	re-derives a world's export policy from an exclusion list.
//
//	THE FILTER SAYS SO. The count is on the stat line with the reason beside it
//	and a control that undoes it. A filter a reader cannot see makes the view
//	wrong.
//
//	THE REVEAL IS A REDRAW. The rows are already in the answer, so showing them
//	costs a repaint and a wider axis — never a request.
//
//	A SEARCH BEATS THE FILTER. "No species matches that search" about a species
//	this view is holding back would be the view lying about its own contents.
func TestTheSeedStockIsHiddenAndSaidSo(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// The mark is READ, never derived: the page tests a published field and does
	// not look at anybody's exclusion list to decide.
	for _, want := range []string{"function lfSeedHidden", "var lfSeedShown = false;",
		"n.seedStock", "x.seedStock", "lfSeedHidden(n)"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the seed filter is missing %q", want)
		}
	}
	// THE FILTER RUNS IN BOTH ORDERS, and the counts it reports are this drawing's.
	if !strings.Contains(region, "function drop(n)") ||
		!strings.Contains(region, "return {list: flat, joined: false, hid: hid, seed: seed};") ||
		!strings.Contains(region, "return {list: out, joined: true, hid: hid, seed: seed};") {
		t.Fatal("the seed filter does not run over both the family order and the population " +
			"ranking, or does not report what it took out")
	}
	// A SEARCH REVEALS THE ROW rather than hiding it and reporting nothing found.
	if !strings.Contains(region, "return !(lfQuery && lfMatches(n));") {
		t.Fatal("a search that matches a seed species does not reveal it")
	}
	// THE NOTICE: the count, the reason, and the control — in the page's own voice.
	for _, want := range []string{`"seed species hidden"`, `"seed species shown"`,
		`"seed species shown by your search"`,
		`" — excluded from migration on every world where it lives"`,
		`el("button", "seedbtn", hid > 0 ? "show" : "hide")`,
		`termEl("span", "seedstock",`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the stat line never says a row was left out: %q missing", want)
		}
	}
	// NOTHING IN THE NOTICE IS A NAME. It is static words, two integers and a
	// glossary term — the same structural property the root badge has, checked the
	// same way, because this line is built in the fenced region and drawn as text
	// nodes either way.
	start := strings.Index(region, "if (x.seedStock > 0 && (hid > 0 || seed > 0)){")
	if start < 0 {
		t.Fatal("the notice is not gated on the server's own count of seed species")
	}
	notice := region[start:]
	if end := strings.Index(notice, "host.appendChild(sd);"); end > 0 {
		notice = notice[:end]
	}
	for _, forbidden := range []string{"n.name", "n.key", "row.name", "nameFrom", "spellings"} {
		if strings.Contains(notice, forbidden) {
			t.Fatalf("the seed notice interpolates %q; its text is static plus integers", forbidden)
		}
	}
	// THE REVEAL IS A REDRAW AND NOT A REQUEST, and the state outlives nothing.
	toggle := page[strings.Index(page, "function toggleSeed()"):]
	toggle = toggle[:strings.Index(toggle, "var lfResizeT")]
	if !strings.Contains(toggle, "lfSeedShown = !lfSeedShown;") ||
		!strings.Contains(toggle, "renderLife(LFX)") {
		t.Fatal("the reveal does not repaint from the answer already held")
	}
	if strings.Contains(toggle, "fetch") {
		t.Fatal("revealing a hidden row fetches; the rows are already in the answer the page " +
			"is holding, which is why the server marks them instead of dropping them")
	}
	if strings.Contains(region, "localStorage") || strings.Contains(region, "sessionStorage") {
		t.Fatal("the species view persists something; the seed state is a variable and a " +
			"reload is meant to be back at the default")
	}
	// The control is rebuilt with the line every poll, so the listener is on the
	// line rather than on the button.
	if !strings.Contains(page, `ev.target.closest(".seedbtn")`) {
		t.Fatal("the reveal control is not wired, or is wired to an element the next poll " +
			"replaces")
	}
	// THE AXIS STRETCHES WITH IT, from the second published edge and not from a
	// number the page invented.
	for _, want := range []string{"x.spanStartSeedMs", "function lfScale(x, cols, seed)",
		"lfScale(x, cols, pick.seed > 0)"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the axis does not follow the rows that are drawn: %q missing", want)
		}
	}
	// The row that is revealed says why it is normally not there, on the row and in
	// the glossary.
	if !strings.Contains(region, `"SEED STOCK · NEVER EXPORTED"`) {
		t.Fatal("a revealed seed row carries no badge saying what it is")
	}
	if !strings.Contains(region, "Seed stock: every world it lives in refuses to export it") {
		t.Fatal("a revealed seed row's tooltip never explains why it is normally absent")
	}
	if !strings.Contains(page, " seedstock:[") {
		t.Fatal("the glossary never explains what seed stock is")
	}
	if !strings.Contains(page, `"excluded","seedstock",`) {
		t.Fatal("the glossary entry exists but is never listed, so nobody can read it")
	}
	// And it is held APART from the weaker fact it is built on: excluded somewhere
	// is not excluded everywhere it lives.
	if !strings.Contains(page, "a species one world holds back can still travel freely out of "+
		"another") {
		t.Fatal("the glossary does not hold 'never exported' apart from 'seed stock'")
	}
}

// TestTheRowsLabelsAreReadable is a regression, and it is the kind that a
// structural test can actually hold: the badges were drawn INSIDE the clip that
// bounds a species name.
//
// A name is 64 bytes somebody else chose, so the label column is clipped or a name
// draws itself over the timeline. The badges rode in that same text run with no
// width budget of their own, and the clip cut them off: measured on the running
// rig, "NO RECORDED ANCESTRY" painted as "NO R" and "THE RECORD BEGINS HERE · 31
// GENERATIONS ABOVE" was invisible in its entirety. A row with something to say
// looked like a row with nothing.
//
// So the badges are a LINE OF THEIR OWN, with their own clip and their own height,
// and the name run holds the name and nothing else.
func TestTheRowsLabelsAreReadable(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	for _, want := range []string{"function lfBadges", "function lfBadgeH", "function lfRowH",
		`svgEl("text", "bdg")`, `bt.setAttribute("clip-path", "url(#lfbclip)")`,
		`bclip.setAttribute("id", "lfbclip")`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the badges have no line of their own: %q missing", want)
		}
	}
	// THE NAME RUN HOLDS THE NAME. Nothing else goes into the clipped element.
	if !strings.Contains(region, "trSpan(text, null, n.name);\n  g.appendChild(text);") {
		t.Fatal("something other than the name is still appended to the clipped label run")
	}
	if strings.Contains(region, `trSpan(text, "tbadge`) ||
		strings.Contains(region, `trSpan(text, "meta"`) {
		t.Fatal("a badge is still drawn inside the name's clip, which is what cut it off")
	}
	// THE BADGE CLIP IS THE LABEL REGION, bounded by the plot: a badge may be as
	// long as this page's own words and still cannot reach the timeline.
	if !strings.Contains(region, "Math.max(60, cols.plot - LF_NAMEX - 10)") {
		t.Fatal("the badge line is unbounded, or is not bounded by where the plot starts")
	}
	// THE ROW IS TALLER FOR IT, and only when it has one. One function answers
	// both the layout and the drawing, so the height reserved and the marks emitted
	// cannot disagree.
	for _, want := range []string{"return LF_ROWH + lfBadgeH(n, joined) + (open ? lfDetailHeight(n) : 0);",
		"y += lfRowH(list[i], pick.joined, lfOpenKey === list[i].key);",
		"lfDetail(g, n, top + LF_ROWH + badgeH, cols)",
		`String(LF_ROWH - 1 + badgeH + (open ? lfDetailHeight(n) : 0))`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the row reserves no room for its badge line: %q missing", want)
		}
	}
	// Every badge that was measured clipped is still drawn, and still says the
	// thing it said.
	for _, want := range []string{`"NO RECORDED ANCESTRY"`, `"NO LIVING RELATIVE"`,
		`"ALSO AN ANCESTOR"`, `"THE RECORD BEGINS HERE · "`, `"NEVER EXPORTED"`,
		`"· extinct here · "`} {
		if !strings.Contains(region, want) {
			t.Fatalf("a badge went missing with the clip: %q", want)
		}
	}
	// AND THE PROMISED TOOLTIP IS REAL. The comment said the full name is always in
	// the row's tooltip; that used to mean this page's own hover tip and nothing a
	// browser, a keyboard or a screen reader could reach.
	if !strings.Contains(region, `var ttl = svgEl("title");`) ||
		!strings.Contains(region, "ttl.textContent = String(n.name);") {
		t.Fatal("the row carries no title element, so the clipped name is nowhere a browser " +
			"or a screen reader can read it")
	}
	if !strings.Contains(page, "svg.life text.bdg{") {
		t.Fatal("the badge line has no style of its own")
	}
}

// TestTheTimelineFitsItsBoxAndKeepsNowInIt is the third rendering defect: the
// drawing was a fixed 1344 pixels wide and only fitted a window of 1427 or more.
// At 1280 the right-hand end of every living bar — NOW, which is where every
// living bar ends and the one mark a reader is looking for — sat 147 pixels past
// the right edge of a box scrolled to zero.
//
// The columns left of the plot are text and dots and have the widths they have.
// The timeline is the elastic one.
func TestTheTimelineFitsItsBoxAndKeepsNowInIt(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	for _, want := range []string{"var host = document.getElementById(\"lfbox\");",
		"var avail = host ? host.clientWidth : 0;",
		"Math.max(LF_PLOTMIN, avail - plot - LF_PLOTPAD - LF_SCROLLW)",
		"w: plot + plotw + LF_PLOTPAD"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the plot is not measured against its own box: %q missing", want)
		}
	}
	// The scale and the now line are drawn against the measured width, not the
	// constant. The constant survives as the answer for a box nothing has laid out
	// yet — a panel still hidden — and the resize that follows repaints it.
	if !strings.Contains(region, "return cols.plot + f * cols.plotw;") {
		t.Fatal("the time scale still maps onto a fixed plot width")
	}
	if strings.Contains(region, "f * LF_PLOTW") || strings.Contains(region, "cols.plot + LF_PLOTW") {
		t.Fatal("something is still drawn against the fixed plot width")
	}
	if !strings.Contains(region, "String(cols.plot + cols.plotw)") {
		t.Fatal("the now line is not at the right-hand end of the measured plot")
	}
	// A RESIZE REPAINTS FROM THE ANSWER ALREADY HELD. It changes geometry and not
	// one fact, so it pulls no poll forward — and it is coalesced, because a drag
	// across a screen fires it by the hundred.
	resize := page[strings.Index(page, `window.addEventListener("resize"`):]
	resize = resize[:strings.Index(resize, "})();")]
	for _, want := range []string{"clearTimeout(lfResizeT)", "renderLife(LFX)",
		`TAB === "species"`} {
		if !strings.Contains(resize, want) {
			t.Fatalf("the resize repaint is missing %q", want)
		}
	}
	if strings.Contains(resize, "fetch") {
		t.Fatal("a resize fetches; nothing about the data changed")
	}
	// The box still scrolls in both directions when even the minimum does not fit,
	// which is this page's rule for wide content everywhere.
	if !strings.Contains(page, ".lifewrap{overflow:auto") {
		t.Fatal("the drawing no longer scrolls inside its own box")
	}
}
