package archive

import (
	"strings"
	"testing"

	"multiverse/internal/contractb"
)

// TestPageHasFourTabsOverOnePoll is the structural half of the Species, Tree and
// Settings tabs. A Go test cannot run the page's JavaScript, so it asserts the
// MACHINERY four views over one poll need, and the two properties that make
// the tabs usable rather than merely present:
//
//	THE TAB IS IN THE URL HASH, so "#species" is a link somebody can send and a
//	reload lands where the reader was.
//
//	THE STATUS POLL IS SHARED. One timer fetches /api/status for all four; a
//	tab that wanted its own would ask the archive for the same frame four
//	times, and the header's numbers would disagree with themselves.
func TestPageHasFourTabsOverOnePoll(t *testing.T) {
	page := statusPageHTML

	for _, want := range []string{
		`<nav class="tabs"`, `data-tab="map"`, `data-tab="species"`, `data-tab="tree"`,
		`data-tab="settings"`,
		`id="p-map"`, `id="p-species"`, `id="p-tree"`, `id="p-settings"`,
		"function showTab", "function tabFromHash", "function wireTabs",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("the tab machinery is missing %q", want)
		}
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
		t.Fatal("more than one place fetches the status frame; four tabs share ONE poll")
	}
	// The species index rides that same cycle rather than a timer of its own.
	if !strings.Contains(page, `if (TAB === "species") await tickSpecies();`) {
		t.Fatal("the species index does not ride the shared poll")
	}
	// And so does the genealogy, gated on its own tab: it is derived from a
	// ledger the browser is never handed, so it costs the archive work and is
	// worth nothing to a tab nobody has open.
	if !strings.Contains(page, `if (TAB === "tree") await tickTree();`) {
		t.Fatal("the genealogy does not ride the shared poll, or is not gated on its tab")
	}
	if strings.Count(page, `fetch("api/species/tree"`) != 1 {
		t.Fatal("the genealogy is fetched from more than one place")
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

// TestSpeciesTabIsAliveOnlyAndSaysSo covers the species tab's rendering and the
// one claim it must not make. The index is the CENSUS UNION: a species that
// crossed a lane a hundred times and is extinct everywhere is not on it, and
// the page has to say that in its own words rather than leave a reader to
// discover it.
func TestSpeciesTabIsAliveOnlyAndSaysSo(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	for _, want := range []string{
		"function renderSpecies", "function speciesRow", "function speciesDetail",
		"function speciesGlyph", "function spSorted", "function spMatches",
		`id="spq"`, `id="spsort"`, `id="spbody"`, `fetch("api/species"`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("the species tab is missing %q", want)
		}
	}
	// Sortable on three keys, searchable, and population descending by default.
	for _, want := range []string{`value="pop"`, `value="crossings"`, `value="name"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("the species index cannot be sorted by %q", want)
		}
	}
	if !strings.Contains(page, `spSort = "pop"`) {
		t.Fatal("the species index does not default to population descending")
	}
	// The three badges, each with a glossary entry behind it.
	for _, want := range []string{`"exc",`, `"every",`, `"endem",`,
		" endemic:[", " everywhere:[", " excluded:["} {
		if !strings.Contains(page, want) {
			t.Fatalf("a species badge or its glossary entry is missing: %q", want)
		}
	}
	// The alive-only rule is stated on the page, in the glossary and in the
	// section's own note.
	if !strings.Contains(page, " alive:[") {
		t.Fatal("the glossary never explains that the index is alive-only")
	}
	if !strings.Contains(page, "species alive right now") {
		t.Fatal("the species section does not say the list is what is alive")
	}
	// The detail: the ledger annotations, and the honesty note about how far
	// back the record actually reaches.
	for _, want := range []string{"distinct genomes", "parent species", "recent crossings",
		"function speciesHistoryReach", "function fillSpeciesSparks",
		`fetch("api/species/history?key="`} {
		if !strings.Contains(page, want) {
			t.Fatalf("the species detail is missing %q", want)
		}
	}
	// The glyph is the SAME creature the map draws, drawn by reference.
	if !strings.Contains(region, `u.setAttribute("href", "#bib")`) {
		t.Fatal("the species tab draws its own creature instead of the shared glyph")
	}
	// One species is one colour everywhere, from the same hash of the same
	// compared spelling the map uses.
	if !strings.Contains(region, "speciesColor(row.key)") {
		t.Fatal("the species tab colours a row from something other than the compared name; " +
			"a species must be the same colour on every tab")
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
	// And so is the whole species index, which draws census names by another
	// route than the map does.
	for _, want := range []string{"function renderSpecies", "function speciesRow",
		"function speciesDetail"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the species index is rendered outside the fence: %q", want)
		}
	}
	// The two functions that DO build markup for the species detail live outside
	// the fence, and the reason they are allowed to is that neither is ever
	// handed a name. Pin that: nothing in them touches a name-bearing field.
	page := statusPageHTML
	outside := page[strings.Index(page, "SPECIES CENSUS — END"):]
	for _, forbidden := range []string{"row.name", "row.spellings", ".genericName",
		".specificName", "migrationExclude"} {
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
	for _, want := range []string{"function renderTree", "function trTip", "function treeStats",
		"function trSpan", "function svgEl"} {
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
	for _, forbidden := range []string{"n.name", "n.nameFrom", "n.key", "trTip("} {
		if strings.Contains(outside, forbidden) {
			t.Fatalf("code after the fence touches %q; the genealogy's name-bearing fields "+
				"belong inside the region where markup may not be assigned", forbidden)
		}
	}
	// And the raw spelling is PRESERVED rather than tidied: SVG text collapses a
	// run of spaces exactly as HTML does, and 13% of the rig's names carry stray
	// whitespace (contract-a.md §17, A36).
	if !strings.Contains(page, "svg.tree text.nm{fill:var(--text);white-space:pre}") {
		t.Fatal("the tree's names do not preserve their own whitespace")
	}
}

// TestTheTreeTabStatesItsOwnLimit is the page half of tree.go rule 3.
//
// The derivation is honest and PARTIAL: ancestry here is a by-product of travel,
// so a lineage that has never crossed a lane is not connected. That is a
// legitimate property of the record and a misleading one to leave implicit — a
// reader looking at a species standing on its own must be told which of the two
// reasons put it there, and the tab must publish the count rather than wait to
// be asked.
func TestTheTreeTabStatesItsOwnLimit(t *testing.T) {
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
