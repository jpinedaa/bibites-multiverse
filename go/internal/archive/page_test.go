package archive

import (
	"encoding/json"
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
		"function lfRows", "function lfMatches", "function lfBar", "function lfJoin",
		"function lfMini", "function lfSpark", "function lfAxis", "function lfTip",
		"function lfStats", "function lfCount", "function lfScale", "function lfCols",
		"function lfIndent", "function lfKin", "function lfLight", "function lfDark",
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
	// The join LANDS at the CHILD's first-seen point, which is the only x-coordinate
	// on the drawing that means anything.
	if !strings.Contains(region, "var jx = sc.x(n.spanFromMs);") {
		t.Fatal("the parent edge does not land at the child's first-seen point")
	}
	// AND EVERY HORIZONTAL DISTANCE ON THE DRAWING IS TIME. The collapsed run used
	// to be the one exception — a lead-in whose LENGTH counted generations at five
	// pixels each — which put two scales on one picture and left the interval those
	// generations really occupied as blank plot. TestALinkIsDrawnOnTheClock below is
	// the whole of the new rule; here it is enough that the old one is gone.
	for _, gone := range []string{"LF_GENPX", "LF_CHAINMIN", "LF_CHAINMAX", "lfChainLen",
		"generations and not time", "generations, not time", "counts generations"} {
		if strings.Contains(page, gone) {
			t.Fatalf("the page still carries the generations-not-time lead-in (%q); a length that "+
				"is not time on an axis that is has no place left on this drawing", gone)
		}
	}
	// THE AXIS IS THE SERVER'S, UNCLAMPED, and the drawing reads the left edge it
	// publishes rather than reaching for the floor.
	if !strings.Contains(region, "var t0 = (seed ? (x.spanStartSeedMs || x.spanStartMs) : x.spanStartMs) || 0;") {
		t.Fatal("the left edge is not the published span start; the axis must fit the bars this " +
			"drawing holds and nothing else")
	}
	// THE RECORD'S FLOOR IS A CAPTION AT THE MARGIN, because the axis starts at the
	// oldest DRAWN bar now and the floor is older than that on any ordinary map.
	// Reserving the space back to it left two thirds of the plot empty on the
	// running rig, and emptier every hour: the floor is a fixed date and the bars
	// are not. The fact stays in words; only the empty pixels went.
	for _, want := range []string{`svgEl("text", "floorcap")`,
		`"the record reaches back to " + trDay(x.ancestrySinceMs)`,
		`fcap.setAttribute("data-t", "recordfloor")`,
		`fcap.setAttribute("text-anchor", "end")`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the record's floor is not captioned at the left margin: %q missing", want)
		}
	}
	// AND THE CAPTION COSTS NO DATE. It sits in the ticks' own row, so the leftmost
	// tick label would straddle its line back over it; that label stands to the
	// RIGHT of its line instead of being dropped, because the leftmost tick is the
	// one a reader looks at to find where the axis begins.
	if !strings.Contains(region, `px - tlbl.length * 3 < capRight + 4 ? "start" : "middle"`) {
		t.Fatal("the leftmost tick label and the floor's caption share the same row with no rule " +
			"between them; one of the two facts is printed over the other")
	}
	// AND THE SHADED RUN BEFORE IT IS GONE, along with the clamp that made it. It
	// shaded the stretch the clamp reserved, which is the stretch that no longer
	// exists.
	if strings.Contains(page, "prefloor") {
		t.Fatal("the page still draws the pre-floor shade; with the axis fitted to the drawn " +
			"bars there is no reserved run for it to shade")
	}
	// THE LINE SURVIVES FOR THE CASE THAT EARNS IT: a floor that genuinely falls
	// INSIDE the drawn span, which happens when a species whose first crossing
	// named no parent predates the oldest crossing that named one. There it marks a
	// stretch where no family line in this drawing can begin. The test is a STRICT
	// > for exactly that reason — at equality the line would sit on the axis's own
	// left edge and mark nothing.
	for _, want := range []string{`svgEl("line", "floor")`,
		"x.ancestrySinceMs && x.ancestrySinceMs > sc.t0"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the floor is no longer drawn where it really is inside the picture: %q "+
				"missing", want)
		}
	}
	if strings.Contains(region, "x.ancestrySinceMs >= sc.t0") {
		t.Fatal("the floor line is gated on >=; the axis is no longer clamped down to the " +
			"floor, so equality means the line lands on the left edge and marks nothing — the " +
			"caption is what says it there")
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

// TestALinkIsDrawnOnTheClock is the rule the collapsed lead-in used to be the one
// exception to, and the two defects that exception hid.
//
// THE OLD MARK. A run of extinct generations with no living branch was drawn as a
// dotted lead-in immediately left of the child's bar, whose LENGTH counted
// generations at five pixels each — deliberately not on the clock, said so in the
// glossary, in the legend, in the axis entry and in every affected row's tooltip.
//
// WHAT IT HID. Those generations lived somewhere, and where they lived was the
// blank stretch between the end of the ancestor's bar and the start of the child's.
// Measured on the running rig: `Sheeplasius dukworthgregorius` starts 40.1 h after
// `Zhiluus tardisitguyus` ends and `Todae raffius` 38.4 h after the same bar, both
// with a 50-odd-pixel mark beside them claiming that same run on another scale. A
// drawing that puts one distance on two scales cannot be read at all.
//
// SO THERE IS ONE RULE NOW, and it is a rule about BOTH ENDS of a link:
//
//	IT LANDS at the child's first-seen point, as it always did — the one
//	horizontal position a join can honestly have.
//
//	IT LEAVES the parent at the latest moment the PARENT'S OWN record supports
//	that is not after that point. For a living parent, and for a child whose
//	first crossing falls inside its parent's span, that is the same instant and
//	the link is the same plain drop it always was.
//
// FOUR SHAPES COME OUT OF THAT, and each is a different thing the record is
// saying. They are lfLink's answer, computed from the two nodes' published spans
// and nothing else.
//
// THE NUMBER SURVIVES ALL OF THEM, and its job is now singular: a count of
// generations cannot be recovered from a duration. Forty generations and one can
// cross the same forty hours.
func TestALinkIsDrawnOnTheClock(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// THE GEOMETRY IS ITS OWN FUNCTION, of the two nodes and the scale. A link that
	// asked only the child could never have found either defect.
	if !strings.Contains(region, "function lfLink(n, p, sc){") {
		t.Fatal("the link's geometry is not derived from the parent as well as the child, so " +
			"nothing on the drawing can know where the parent's record stops")
	}
	link := region[strings.Index(region, "function lfLink(n, p, sc){"):]
	link = link[:strings.Index(link, "/* THE BRAIN")]

	// CASE 1 — no parent bar at all. A parent no crossing of its own was recorded
	// for has no drawn span to leave, so the link drops where it lands.
	if !strings.Contains(link, "if (!(p && p.spanFromMs)) return out;") {
		t.Fatal("a link whose parent the record never dated does not fall back to the plain drop")
	}
	// CASE 2 — the inversion: the child's own record starts first.
	for _, want := range []string{"if (n.spanFromMs < p.spanFromMs){", "out.rev = true;",
		"out.lx = sc.x(p.spanFromMs);"} {
		if !strings.Contains(link, want) {
			t.Fatalf("a child recorded before its parent is not given its own shape: %q missing", want)
		}
	}
	// CASE 3 — a gap: the parent's record stops before the child's begins. A LIVING
	// parent has no last moment, because its bar runs to the right-hand edge.
	for _, want := range []string{"var lastMs = p.alive ? 0 : (p.spanToMs || p.spanFromMs);",
		"if (lastMs && n.spanFromMs > lastMs){", "out.lx = sc.x(lastMs);",
		"out.run = jx - out.lx;"} {
		if !strings.Contains(link, want) {
			t.Fatalf("the run across a gap is not the gap's own interval: %q missing", want)
		}
	}
	// CASE 4 — a gap too short to see. It is widened to the SAME 2.5 px the shortest
	// bar takes, which is the only overstatement either mark is allowed; the number
	// carries the count regardless.
	if !strings.Contains(region, "var LF_RUNMIN = 2.5;") ||
		strings.Count(link, "if (out.run < LF_RUNMIN){ out.run = LF_RUNMIN;") != 2 {
		t.Fatal("a sub-pixel run is not floored to a visible mark in both directions, so a run " +
			"of ten generations across four minutes is drawn as nothing at all")
	}
	// AND NOTHING IS INVENTED WHERE THE RECORD SAYS NOTHING. A living parent, or a
	// child starting inside its parent's span, leaves no interval this drawing can
	// honestly claim — the generations sat somewhere in a stretch the parent itself
	// occupies and the record does not say where — so no run is drawn and the number
	// stands alone. That is the branch's fall-through, and the words say it too.
	if !strings.Contains(page, "Where there is no such stretch — the ancestor is still alive, "+
		"or the descendant's first crossing falls inside the ancestor's own span — nothing is "+
		"drawn across") {
		t.Fatal("the glossary never says that a collapsed run with no interval to occupy is not " +
			"drawn as one")
	}

	// THE DRAWING OF EACH SHAPE. A collapsed run is dotted because the species that
	// carried it are not on the picture; a direct link across the same gap is the
	// plain line, because there is nothing undrawn about it.
	join := region[strings.Index(region, "function lfJoin"):]
	join = join[:strings.Index(join, "function lfBadges")]
	for _, want := range []string{`svgEl("path", n.collapsed > 0 ? "chain" : "link")`,
		`" Q" + lx.toFixed(1) + " " + cy + " " + (lx + er).toFixed(1) + " " + cy +`,
		`" H" + jx.toFixed(1)`, "var er = Math.min(5, lay.run);"} {
		if !strings.Contains(join, want) {
			t.Fatalf("the run across a gap is not drawn from the parent's last moment into the "+
				"child's bar: %q missing", want)
		}
	}
	// THE NUMBER GOES WITH THE SHAPE: centred over a run, tucked against a drop.
	for _, want := range []string{`lbx = (lx + jx) / 2; lba = "middle";`,
		`var jx = lay.jx, lx = lay.lx, lbx = jx - 4, lba = "end", lby = cy - 4;`,
		`gt.setAttribute("text-anchor", lba);`, `gt.textContent = "+" + n.collapsed;`} {
		if !strings.Contains(join, want) {
			t.Fatalf("the collapsed count does not follow the mark it counts: %q missing", want)
		}
	}
	// THE BACKWARDS LINK, drawn ABOVE the child's bar rather than along it — the
	// stretch it crosses is a stretch that bar already occupies — with a ring where
	// it left the parent and the warning colour on both.
	for _, want := range []string{`svgEl("path", "rev")`, `svgEl("circle", "revmark")`,
		"var ry = cy - LF_REVY;", `"M" + lx.toFixed(1) + " " + py + " V" + ry +`,
		`" H" + jx.toFixed(1) + " V" + (cy - 2)`,
		`mk.setAttribute("cx", lx.toFixed(1));`} {
		if !strings.Contains(join, want) {
			t.Fatalf("the inverted link is not drawn as one: %q missing", want)
		}
	}
	for _, want := range []string{"svg.life .rev{fill:none;stroke:var(--warn)",
		"svg.life .revmark{fill:none;stroke:var(--warn)", "var LF_REVY = 8;"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the inverted link is not marked out from ordinary descent: %q missing", want)
		}
	}
	// IT NEVER FABRICATES A PARENT BAR. The only x-coordinates the shape uses are
	// the two published span starts and the parent's published end.
	if strings.Contains(link, "spanDerived") {
		t.Fatal("the page re-derives a span for the inversion; the server's numbers are already " +
			"right — a child recorded before its parent is a true fact about the record, and " +
			"spanDerived is the complementary case where the parent has no record of its own")
	}

	// THE PAIR INDEX. Both the link and the row's tooltip are about two species now,
	// and it is rebuilt with every paint so a link cannot be dated against a species
	// that has left the answer.
	for _, want := range []string{"var LFBY = {};", "LFBY = {};",
		"for (var bi=0;bi<all.length;bi++) LFBY[all[bi].key] = all[bi];"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the drawing has no index of the answer's own nodes: %q missing", want)
		}
	}

	// SAID IN WORDS, in all four places the old exception was written into.
	for _, want := range []string{" collapsed:[", " beforeparent:["} {
		if !strings.Contains(page, want) {
			t.Fatalf("the glossary never explains %q", want)
		}
	}
	if !strings.Contains(page, `"branchpoint","collapsed","beforeparent","noancestry","recordfloor",`) {
		t.Fatal("the new glossary entry is never listed, so nobody can read it")
	}
	for _, want := range []string{
		"THE DOTTED RUN IS ON THE CLOCK, exactly like every other horizontal distance here",
		"a count of generations cannot be recovered from a duration",
		"A species whose first recorded crossing is EARLIER than its parent's own",
		"This page will not invent a bar to tidy it up"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the glossary does not carry the rewritten rule: %q missing", want)
		}
	}
	// The legend names both marks where they are named, and the backwards one has a
	// swatch of its own so it cannot be taken for the dotted run.
	for _, want := range []string{
		`<span class="term" data-t="collapsed">across the stretch they held the line</span>`,
		`<i class="revi"></i><span class="term" data-t="beforeparent">recorded before its`,
		".treelegend i.revi{"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the legend does not name the marks it draws: %q missing", want)
		}
	}
	// AND THE ROW SAYS WHICH STRETCH, from the same two dates the drawing used — or
	// says that there is none, which is the case a mark cannot make.
	for _, want := range []string{
		`"The record of the row above stops " + ms(n.spanFromMs - pend) +`,
		`" before this one's begins, and the link is drawn across exactly that stretch" +`,
		`"Nothing is drawn across for those generations: the record of the row above " +`,
		`"ITS OWN RECORD STARTS BEFORE ITS PARENT'S, by " +`,
		"ms(pn.spanFromMs - n.spanFromMs)"} {
		if !strings.Contains(region, want) {
			t.Fatalf("a row's tooltip does not describe its own link: %q missing", want)
		}
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
	// they are absent. Its SCALE is TestTheBrainRingIsFittedToWhatIsDrawn's subject.
	for _, want := range []string{"function lfBrainR", "n.neurons", "n.synapses",
		"var LF_BRAIN_R0 = 2.4, LF_BRAIN_R1 = 6.4;",
		"if (w <= 0) return 0;", `svgEl("circle", "brain")`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the brain ring is missing %q", want)
		}
	}
	if !strings.Contains(region, "var r = lfBrainR(n);\n  if (r > 0){") {
		t.Fatal("a species with no stored genome is still given a ring; absent must draw nothing")
	}
	// THE ABSENCE SAYS WHY, and it says the RIGHT why now that a measurement
	// outlives the copy it was read from: not "no copy is held" — a copy being
	// gone no longer removes the answer — but "no genome of it has ever been
	// read here", which is the only thing that does.
	if !strings.Contains(region, "no genome of it has ever been read here") {
		t.Fatal("the expanded row never says WHY a brain is missing; a genome that was never " +
			"readable is not a brain of no neurons")
	}
	if strings.Contains(page, "no copy of its latest genome is held here") {
		t.Fatal("the expanded row still blames a missing COPY for a missing brain; the " +
			"measurement is kept after the copy is pruned, so that is no longer the reason")
	}
	// AND A KEPT MEASUREMENT CARRIES ITS AGE. An ancestor extinct for days shows a
	// days-old reading beside a living species' current one, and a row that did not
	// say so would present two instants as one.
	for _, want := range []string{"function lfBrainAge(n){", "n.brainAtMs",
		`", " + dage + " old"`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the kept brain measurement never says how old it is: %q missing", want)
		}
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

// TestTheBrainRingIsFittedToWhatIsDrawn is a mark that encoded nothing, and the
// cost of making it encode something.
//
// THE MEASUREMENT. The ring log-scaled (neurons + synapses) against a fixed 600
// into radii of 2.4 to 6.4 px. On the live tree the seven species that have stats
// at all run 88 to 113 — 5.21 px to 5.36 px. `Sheeplasius dasryanus` carries 29%
// more brain than `Sheeplasius cyberdudei` and drew a ring 0.15 px bigger. A mark
// whose whole range is a seventh of a pixel is not a small signal; it is no signal,
// drawn as though it were one.
//
// THE FIX AND ITS PRICE. The scale is fitted to the species DRAWN RIGHT NOW, so
// the smallest brain on the picture is the smallest ring and the largest is the
// largest. That buys variation and it sells absoluteness: a ring is now a PLACE
// AMONG THE ROWS IN FRONT OF YOU and it re-fits when those rows change, exactly as
// the time axis already does. A page that took that quietly would be a page whose
// marks mean whatever the last poll decided, so it is said in the glossary, on the
// legend, on the row's tooltip and on the ring's own.
//
// THREE DEGENERATE SETS, EACH DECIDED RATHER THAN FALLEN INTO. One species with
// stats is not a comparison; several species all carrying the same total is not a
// comparison either, and is also a zero divisor. Both draw MID-SCALE — the largest
// ring would claim a biggest nothing was measured against and the smallest would
// claim the reverse. And a species with no stored genome still draws NO RING: the
// one thing this change must not do is turn an absence into the smallest brain on
// the map.
func TestTheBrainRingIsFittedToWhatIsDrawn(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// THE RANGE IS COMPUTED OVER THE DRAWN ROWS, once a paint, and the rows are the
	// ones the search, the seed filter and the census actually produced.
	for _, want := range []string{"function lfBrainRange(list){", "var LFBRAIN = {lo: 0, hi: 0, n: 0};",
		"LFBRAIN = lfBrainRange(list);", "function lfBrainW(n){ return (n.neurons || 0) + (n.synapses || 0); }"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the ring's scale is not fitted to the drawn set: %q missing", want)
		}
	}
	if !strings.Contains(region, "var pick = lfRows(x), list = pick.list, i;\n") ||
		strings.Index(region, "LFBRAIN = lfBrainRange(list);") <
			strings.Index(region, "var pick = lfRows(x), list = pick.list, i;") {
		t.Fatal("the range is fitted before the rows are chosen, so it describes a set that is " +
			"not the one being drawn")
	}
	// THE FIXED CEILING IS GONE. It is the whole defect: a range the data does not
	// occupy renders every difference in it as nothing.
	if strings.Contains(page, "LF_BRAIN_FULL") {
		t.Fatal("the ring is still scaled against a fixed ceiling; on this map that put every " +
			"species within 0.15 px of every other")
	}
	// THE MAPPING: the drawn minimum onto the smallest radius, the drawn maximum
	// onto the largest, logarithmically between so one outlier cannot flatten the
	// rest — which is this same defect one range in.
	for _, want := range []string{
		"var f = (Math.log(1 + w) - Math.log(1 + b.lo)) /",
		"(Math.log(1 + b.hi) - Math.log(1 + b.lo));",
		"return LF_BRAIN_R0 + f * (LF_BRAIN_R1 - LF_BRAIN_R0);"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the ring does not map the drawn range onto the whole radius span: %q missing",
				want)
		}
	}
	// BOTH DEGENERATE SETS, ONE ANSWER, NO DIVISION BY ZERO. n < 2 is the single
	// species; !(hi > lo) is the set that is all one number and is also the zero
	// divisor. Neither may be drawn as an extreme.
	if !strings.Contains(region, "if (b.n < 2 || !(b.hi > b.lo)) return mid;") ||
		!strings.Contains(region, "var b = LFBRAIN, mid = (LF_BRAIN_R0 + LF_BRAIN_R1) / 2;") {
		t.Fatal("a drawing with one measured brain, or with several identical ones, either " +
			"divides by zero or draws them all at an extreme")
	}
	// ABSENCE IS STILL ABSENCE. Nine of sixteen species on the running rig have no
	// stored genome, and the gate that gives them no ring at all must come BEFORE
	// anything that could hand them the minimum.
	zero := strings.Index(region, "if (w <= 0) return 0;")
	if zero < 0 || zero > strings.Index(region, "var b = LFBRAIN, mid =") {
		t.Fatal("a species with no stored genome is not returned as nothing before the scale " +
			"runs; an absent brain drawn as the smallest ring is a reading invented out of a gap")
	}
	if !strings.Contains(region, "var r = lfBrainR(n);\n  if (r > 0){") {
		t.Fatal("a radius of zero is still drawn as a circle")
	}

	// THE NUMBERS THE SIZE STOPPED CARRYING, on the ring itself. Its key is
	// generated from the row index — a species name never becomes part of one — and
	// the glossary term stays behind it so the mark explains itself either way.
	for _, want := range []string{"function lfBrainTip(n){", `var bk = "lfb" + i;`,
		"SP[bk] = lfBrainTip(n);", `ring.setAttribute("data-s", bk);`,
		`ring.setAttribute("data-t", "brainsize");`,
		`n.neurons + " neurons and " + n.synapses + " synapses, from the newest genome of this "`,
		`"species this archive EVER READ"`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the ring carries no tooltip of its own: %q missing", want)
		}
	}
	// A 2.4 px unfilled circle is hittable on its stroke alone, which is not a hit
	// target. The whole disc has to answer or the tip is unreachable.
	if !strings.Contains(page, "svg.life .brain{fill:none;stroke:var(--lane);stroke-width:1.3;"+
		"cursor:help;pointer-events:all}") {
		t.Fatal("the ring is not a pointer target, so the tooltip carrying its real numbers " +
			"cannot be reached on the mark it belongs to")
	}
	// The tip says what the size means now, and names the range it was fitted to —
	// including the degenerate one, where it says why it is mid-scale.
	for _, want := range []string{
		`" The ring is sized against the species drawn RIGHT NOW — " + b.lo + " to " +`,
		`" It is the largest on this drawing."`,
		`" It is the smallest on this drawing."`,
		`", so the ring is drawn mid-scale rather than claiming a biggest or a smallest.";`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the ring's tooltip does not say what its size means: %q missing", want)
		}
	}

	// AND THE COST IS WRITTEN DOWN where a reader meets the mark: the glossary, the
	// legend, and the row's own tooltip.
	for _, want := range []string{
		"COMPARED WITH THE OTHER SPECIES ON THIS DRAWING",
		"It is not an absolute size, and two rings of the same size on two different days are " +
			"not the same brain.",
		"the species on this map differed by a seventh of a pixel",
		"HOVER A RING FOR THE REAL NUMBERS"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the glossary does not state what the ring now means or what that cost: "+
				"%q missing", want)
		}
	}
	if !strings.Contains(page, "&mdash; bigger ring, more neurons and synapses <em>than the "+
		"other rows drawn</em>;") {
		t.Fatal("the legend still calls the ring an absolute size")
	}
	if !strings.Contains(region, `"The ring at the end of the bar is where that sits AMONG THE SPECIES DRAWN RIGHT NOW " +`) {
		t.Fatal("a row's own tooltip still says the ring is that size")
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

// TestTheFamilyIsReadableDownTheLabelColumn is the answer to the owner's reading
// of the live view: "the ancestry seems a bit hard to tell how the tree is".
//
// THE DRAWING SAID DESCENT IN ONE PLACE — a line dropping onto a bar somewhere out
// in the plot, at the child's first-seen point. That position is the only honest
// one it has, and it is also the reason the structure was unreadable: with fourteen
// rows and bars scattered across days of axis, two related rows sit a hand's width
// apart and the line between them crosses everything in between.
//
// SO DESCENT IS SAID TWICE, and the second saying is the one a reader scans: the
// label column is a printed tree. Every row is indented by its depth — glyph
// included, because a column of glyphs beside ragged text reads as a list — and a
// bracket runs from under a parent's glyph down to each child, one rail per parent
// and one stub per child. It uses no horizontal position that means anything, so it
// works wherever the bars are.
func TestTheFamilyIsReadableDownTheLabelColumn(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// The bracket itself, and the shape of it: down the rail, then a stub into the
	// child's own glyph.
	for _, want := range []string{`svgEl("path", "tw")`,
		`"M" + gxp + " " + (py + 7) + " V" + cy + " H" + (gx - 5)`,
		"svg.life .tw{fill:none;stroke:var(--line)"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the label column draws no parent-child bracket: %q missing", want)
		}
	}
	// THE WHOLE ROW STEPS RIGHT, glyph and name and badges, from ONE function — so
	// the bracket's stub and the thing it points at cannot drift apart.
	for _, want := range []string{"function lfIndent",
		"return joined ? Math.min(n.depth || 0, LF_MAXD) * LF_INDENT : 0;",
		"var indent = lfIndent(n, joined);",
		`"translate(" + (LF_GLYPHX + indent) + " " + (top + 12) + ") scale(0.85)"`,
		`ring.setAttribute("cx", String(LF_GLYPHX + indent))`,
		`text.setAttribute("x", String(LF_NAMEX + indent))`,
		`bt.setAttribute("x", String(LF_NAMEX + indent + LF_BADGEX))`,
		"rowInd[list[i].key] = lfIndent(list[i], pick.joined);"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the row does not step right with its depth: %q missing", want)
		}
	}
	// THE BRACKET IS DRAWN FOR A CHILD THE RECORD HAS NEVER DATED. The drop cannot
	// be — it has no x — but the RELATIONSHIP is known, and a row whose bar is
	// missing is exactly the row a reader wonders about most. So the bracket comes
	// first in lfJoin and the drop sits behind the date's own gate.
	join := region[strings.Index(region, "function lfJoin"):]
	join = join[:strings.Index(join, "function lfBadges")]
	tw, drop := strings.Index(join, `svgEl("path", "tw")`), strings.Index(join, "if (n.spanFromMs){")
	if tw < 0 || drop < 0 || tw > drop {
		t.Fatal("the bracket is drawn inside the date's gate; a child whose record holds no " +
			"crossing still has a parent, and that is the row whose family a reader most needs " +
			"drawn")
	}
	// THE LINK TURNS INTO THE BAR rather than crossing it: an elbow, and it lands at
	// the child's first-seen point, which is still the only x-coordinate this join
	// can have. Where it LEAVES the parent is lfLink's answer and is tested there.
	for _, want := range []string{"var lay = lfLink(n, LFBY[n.parent], sc);",
		"var jx = lay.jx, lx = lay.lx,", `" Q" + jx.toFixed(1)`} {
		if !strings.Contains(join, want) {
			t.Fatalf("the parent link is not an elbow into the child's bar: %q missing", want)
		}
	}
	// A LINK IS ONLY A LINK IF BOTH ENDS ARE DRAWN. A hidden seed parent, or a
	// parent a search left out, leaves the child with no bracket — never a bracket
	// to a row that is not there, and never a reparenting onto a grandparent.
	if !strings.Contains(region, "if (!n.parent || rowY[n.parent] == null) continue;") {
		t.Fatal("the drawing joins a child to a parent that may not be on it")
	}
	// And it is jargon, so it carries a glossary entry and a legend line like the
	// rest of the drawing's marks.
	if !strings.Contains(page, " descends:[") {
		t.Fatal("the glossary never explains the bracket or the drop")
	}
	if !strings.Contains(page, `"lifespan","timeaxis","descends","lineage",`) {
		t.Fatal("the new glossary entries are never listed, so nobody can read them")
	}
	for _, want := range []string{`<i class="twi"></i><span class="term" data-t="descends">`,
		".treelegend i.twi{"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the legend never names the bracket: %q missing", want)
		}
	}
}

// TestOneLineageLightsAndTheRestDims is the other half of the same answer, and the
// half that answers a question the drawing could not answer at all: HOW IS THIS ONE
// RELATED.
//
// Point at a row — or reach it with the Tab key — and everything that is not its
// ancestor or its descendant fades. It is a class change on elements already on the
// screen: no request, no rebuild, and nothing moves, because the rest of the
// drawing keeps its place. Hiding the other rows would reflow the picture under the
// reader's eye and answer a different question.
func TestOneLineageLightsAndTheRestDims(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// THE LINE IS BOTH DIRECTIONS: up to the root, and the whole subtree below.
	for _, want := range []string{"function lfKin", "cur = LFPAR[cur]",
		"var kids = LFKID[stack.pop()] || [];"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the lit line is not the whole line: %q missing", want)
		}
	}
	// BOTH WALKS ARE GUARDED. The server's tree is acyclic and guarded (tree.go
	// rule 4); this walks what the PAGE built, which is one more place a loop must
	// not become a hang.
	for _, want := range []string{"guard++ < 600", "guard++ < 4000"} {
		if !strings.Contains(region, want) {
			t.Fatalf("a walk over the drawn tree is unguarded: %q missing", want)
		}
	}
	// It is DIMMING and not hiding, and it is a class on things already drawn.
	for _, want := range []string{"svg.life.lit .lfrow{opacity:.2}",
		"svg.life.lit .lfrow.kin{opacity:1}", "svg.life.lit .ln{opacity:.12}",
		"svg.life.lit .ln.kin{opacity:1}"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the lighting is not a dim over the drawing as it stands: %q missing", want)
		}
	}
	for _, want := range []string{"function lfLight", "function lfDark",
		`LFSVG.setAttribute("class", "life lit")`,
		`(k === key ? " kin self" : (on[k] ? " kin" : ""))`,
		`LFLN[k].base + (on[k] ? " kin" : "")`} {
		if !strings.Contains(region, want) {
			t.Fatalf("lighting a lineage is not a class change over the drawn rows: %q missing",
				want)
		}
	}
	// THE LINK IS LIT AS ONE THING. Both marks of a parent-child join — the bracket
	// and the drop — live in one group keyed by the CHILD, so a lit lineage cannot
	// light half a join.
	for _, want := range []string{`var g = svgEl("g", "ln");`, `g.setAttribute("data-k", n.key);`,
		`LFLN[n.key] = {g: lg, base: "ln"};`} {
		if !strings.Contains(region, want) {
			t.Fatalf("a join is not one lightable unit: %q missing", want)
		}
	}
	// NOTHING SURVIVES A REPAINT. The elements the next paint replaces are gone,
	// and a stale entry would light a row that is not on the screen.
	if !strings.Contains(region,
		"LFSVG = null; LFEL = {}; LFLN = {}; LFPAR = {}; LFKID = {}; LFJOINED = false;") {
		t.Fatal("the lighting's registries outlive the drawing they describe")
	}
	// AND NOTHING LIGHTS IN THE POPULATION ORDER, which draws no edges at all.
	// Lighting one row and dimming every other there would be a claim about the
	// record made out of a sort order: "this species is related to nothing".
	for _, want := range []string{"if (!LFSVG || !LFJOINED || !LFEL[key]) return;",
		"LFJOINED = pick.joined;"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the lighting is not gated on the drawing having drawn a family: %q missing",
				want)
		}
	}
	// THE KEYBOARD GETS THE SAME ANSWER, which is the whole reason this is not a
	// :hover rule in the stylesheet: a row is focusable, announced by its <title>,
	// lit on focus and opened with Enter or Space.
	for _, want := range []string{`g.setAttribute("tabindex", "0");`,
		`g.setAttribute("role", "button");`, "svg.life .lfrow:focus .hit{"} {
		if !strings.Contains(page, want) {
			t.Fatalf("a row cannot be reached or seen with the keyboard: %q missing", want)
		}
	}
	for _, want := range []string{`box.addEventListener("mouseover"`,
		`box.addEventListener("mouseleave", lfRest)`, `box.addEventListener("focusin"`,
		`box.addEventListener("focusout"`, `box.addEventListener("keydown"`,
		`if (ev.key !== "Enter" && ev.key !== " " && ev.key !== "Spacebar") return;`} {
		if !strings.Contains(page, want) {
			t.Fatalf("the lineage affordance is not wired for pointer and keyboard both: %q "+
				"missing", want)
		}
	}
	// THE POINTER LEAVING IS NOT THE FOCUS LEAVING. A reader who tabbed to a row and
	// then moved the mouse across the page keeps the focus ring, so the family that
	// ring belongs to must stay lit — the pointer's exit falls back to the
	// keyboard's row rather than to darkness.
	for _, want := range []string{"function lfRest",
		"if (lfFocusKey && LFEL[lfFocusKey]) lfLight(lfFocusKey); else lfDark();"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the pointer leaving puts out a lineage the keyboard is still on: %q "+
				"missing", want)
		}
	}
	if !strings.Contains(page, "if (g) lfLight(g.getAttribute(\"data-k\")); else lfRest();") {
		t.Fatal("the pointer moving off a row darkens a lineage the keyboard may still hold")
	}
	// And the focus survives the repaint — see TestTheKeyboardKeepsItsRowAcrossEvery
	// Paint for the whole of it, which is every paint and not only this one.
	if !strings.Contains(page, "lfFocusKey = g.getAttribute(\"data-k\");") {
		t.Fatal("the key that opens a row does not record the row it opened")
	}
	// The affordance is explained where a reader will meet it, like every other
	// mark on this drawing.
	if !strings.Contains(page, " lineage:[") {
		t.Fatal("the glossary never explains what the dimming means")
	}
	if !strings.Contains(page, `<span class="term" data-t="lineage">hover a row, or tab to it</span>`) {
		t.Fatal("the legend never tells a reader the affordance is there")
	}
}

// TestTheKeyboardKeepsItsRowAcrossEveryPaint is the affordance's other half made
// usable for longer than two seconds.
//
// The species view repaints about every two seconds and each paint replaces the
// whole drawing, so the element holding the keyboard's focus is destroyed thirty
// times a minute. The first version of this recorded the focused row in one place
// only — the key that opens one — and nulled it at the end of the paint, so focus
// survived the paint that OPENING a row causes and no other: a row reached with
// Tab was on <body> within 2.5 s, its lit family went out with it, and a reader
// walking down the tree was returned to the top of the document about twice a row.
//
// The rule this pins is the whole fix. The focus is recorded wherever it ARRIVES,
// it is cleared only when the reader genuinely leaves, and the paint puts it back
// on the row with the same species key — with that row's family lit again, because
// a focus ring on a dark drawing is a different answer from the one the reader
// left. What it must NOT do is take the focus that was never its own.
func TestTheKeyboardKeepsItsRowAcrossEveryPaint(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// ASSIGNED WHERE THE FOCUS LANDS, not only where a row is opened. Tab, a click
	// and the paint's own restore all arrive as focusin, and that is the one place
	// that needs to know.
	for _, want := range []string{`box.addEventListener("focusin", function(ev){`,
		"lfFocusKey = g.getAttribute(\"data-k\");\n      lfLight(lfFocusKey);"} {
		if !strings.Contains(page, want) {
			t.Fatalf("a row taking the focus does not record it, so the next paint cannot "+
				"give it back: %q missing", want)
		}
	}
	// CLEARED ONLY ON A GENUINE BLUR. A row losing the focus because the paint took
	// its element away is not the reader leaving — the paint says so with lfPainting
	// while it is tearing the drawing down, and a row already out of the document is
	// the same event arriving after the removal rather than during it. Row-to-row is
	// not leaving either.
	for _, want := range []string{"if (!g || lfPainting || g.isConnected === false) return;",
		"if (ev.relatedTarget && rowOf(ev.relatedTarget)) return;",
		"lfFocusKey = null;\n      lfDark();"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the paint's own tear-down reads as the reader walking away: %q missing",
				want)
		}
	}
	// The paint is what declares itself, on both sides of the tear-down.
	for _, want := range []string{"lfPainting = true;", "lfPainting = false;",
		"lfFocusKey = null, lfPainting = false;"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the paint never says it is painting, so the blur it causes is "+
				"indistinguishable from the reader's: %q missing", want)
		}
	}
	// AND NOT NULLED AT THE END OF A PAINT, which is the defect itself: it made the
	// focus survive exactly one repaint and then let the next one drop it.
	if strings.Contains(region, "  lfFocusKey = null;\n}") {
		t.Fatal("the paint throws the keyboard's place away as it finishes, so the focus " +
			"survives one repaint and no more")
	}
	// PUT BACK BY KEY, WITH THE FAMILY LIT. The element is new; the species key is
	// what carries across.
	for _, want := range []string{"function lfRefocus(host, mine){", "var ent = LFEL[lfFocusKey];",
		"if (ent && ent.g.focus){", "ent.g.focus();", "lfLight(lfFocusKey);"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the paint does not put the focus back on the row it just rebuilt: %q "+
				"missing", want)
		}
	}
	// ONLY IF THE PAINT IS WHAT TOOK IT. A reader who has tabbed on to the search
	// box, or clicked away entirely, must never be dragged back into the drawing by
	// a poll — so the decision is taken from activeElement BEFORE the tear-down, and
	// the restore is gated on it.
	for _, want := range []string{"var ae = document.activeElement,",
		"mine = !!lfFocusKey && (!ae || ae === document.body || host.contains(ae));",
		"if (!mine) return;"} {
		if !strings.Contains(region, want) {
			t.Fatalf("a poll can steal the focus from wherever the reader actually is: %q "+
				"missing", want)
		}
	}
	// THE ROW THAT IS GONE. The drawn set genuinely turns over, and a species that
	// stops being reported takes its row out of the next paint. Restoring nothing
	// drops the focus on <body> — the defect again — and restoring some OTHER row
	// would light another family and read as if the reader had walked there. The
	// focus lands on the drawing's own box: nothing lit, no row claimed, and the
	// next Tab walks in at the first row instead of at the top of the page.
	for _, want := range []string{"lfFocusKey = null;\n  lfDark();\n  if (host.focus) host.focus();"} {
		if !strings.Contains(region, want) {
			t.Fatalf("a row leaving the census sends the reader back to the top of the "+
				"document: %q missing", want)
		}
	}
	for _, want := range []string{`<div class="lifewrap" id="lfbox" tabindex="-1">`,
		".lifewrap:focus{outline:none}"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the box cannot hold the focus it is given as a landing place: %q missing",
				want)
		}
	}
	// EVERY EXIT FROM THE PAINT RESTORES, including the two that draw a sentence
	// instead of a drawing — otherwise a poll that finds no rows leaves lfPainting
	// set and the next genuine blur is ignored for the life of the tab.
	if got := strings.Count(region, "lfRefocus(host, mine);"); got != 3 {
		t.Fatalf("the paint restores the focus on %d of its exits, not all 3", got)
	}
	// THE SAME REPAINT DROPS THE SEED-REVEAL BUTTON, and the same rule answers it:
	// the stat line is rebuilt every poll too, so the button the reader just pressed
	// is a different element a moment later.
	for _, want := range []string{
		`keepBtn = !!(ae && ae.closest && ae.closest(".seedbtn") && host.contains(ae)),`,
		"newBtn = btn;", "if (keepBtn && newBtn && newBtn.focus) newBtn.focus();"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the reveal control loses the keyboard every poll: %q missing", want)
		}
	}
}

// TestTheShortestBarStaysInsideTheAxis pins the minimum mark's right edge to now.
//
// A species the record dates to effectively one instant would draw a bar of no
// width, so it gets 2.5 px — a mark that says "here", not a duration. Grown to the
// right from a first-seen point that is itself within 2.5 px of the axis end, that
// mark crosses the now line and claims the record reaches into a future it cannot
// have. It is slid left instead: the right edge lands on now, the mark keeps its
// full width, and the newest rows — which are exactly the rows this case is about
// — stay visible instead of being shrunk to nothing against the edge.
func TestTheShortestBarStaysInsideTheAxis(t *testing.T) {
	region := speciesRegion(t)
	for _, want := range []string{"if (x1 - x0 < 2.5){", "x1 = x0 + 2.5;",
		"var xend = sc.x(sc.t1);", "if (x1 > xend){ x1 = xend; x0 = xend - 2.5; }"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the shortest bar overhangs the now line, or is clamped to nothing: %q "+
				"missing", want)
		}
	}
}

// TestTheDrawingExplainsWhatItIs is the tooltip inventory, aimed at a reader who
// has never seen this page: the axis is TIME, a bar is a species' recorded life, a
// drop is DESCENDED FROM, a dotted lead-in is generations nobody is left of, the
// caption at the left margin is how far back the record itself goes, and a species
// with no ancestry has a reason.
//
// Every one of those is a term with a glossary entry behind it, because this page's
// rule is that jargon it invents is jargon it explains — and the drawing itself is
// the biggest piece of jargon on the tab.
func TestTheDrawingExplainsWhatItIs(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// THE AXIS IS TIME, and until now nothing said so: the ticks named WHICH
	// moments and left "left to right is time at all" to be inferred.
	if !strings.Contains(region, `head(cols.plot, "time, left to right", null, "timeaxis");`) {
		t.Fatal("the plot has no heading, so nothing on the drawing says the direction is time")
	}
	for _, want := range []string{" timeaxis:[", " descends:[", " lineage:["} {
		if !strings.Contains(page, want) {
			t.Fatalf("the glossary never explains %q", want)
		}
	}
	// The axis entry has to hold the two things a reader gets wrong about it: the
	// left edge is not the beginning of anything, and — since the collapsed run came
	// on to the clock — that there is no longer any mark on it that is not.
	for _, want := range []string{"The left-hand edge is the OLDEST BAR ON THE PICTURE",
		"EVERY horizontal distance on this drawing is time and there is no exception to that"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the time-axis entry never says %q", want)
		}
	}
	// THE MARKS THAT CARRY A TERM ON THE DRAWING ITSELF, each hoverable where it is
	// drawn: the caption at the margin, the collapsed run's number, the backwards
	// link and the ring where it left its parent, the brain ring.
	for _, want := range []string{`fcap.setAttribute("data-t", "recordfloor")`,
		`gt.setAttribute("data-t", "collapsed")`, `rv.setAttribute("data-t", "beforeparent")`,
		`mk.setAttribute("data-t", "beforeparent")`, `ring.setAttribute("data-t", "brainsize")`} {
		if !strings.Contains(region, want) {
			t.Fatalf("a mark on the drawing carries no explanation: %q missing", want)
		}
	}
	// THE ROW'S OWN TOOLTIP NAMES ITS PARENT, which used to need a click. The three
	// arrangements are three sentences, because a bracket that skips a run of
	// collapsed generations does not say what a direct one says.
	for _, want := range []string{`lines.push("Descends from " + n.parentName`,
		"which is the row the bracket beside the ",
		"the bracket beside the name reaches past it to the nearest ancestor that IS drawn",
		"line is alive on this map, so there is no row above this one to join it to."} {
		if !strings.Contains(region, want) {
			t.Fatalf("the row's tooltip never says who it descends from: %q missing", want)
		}
	}
	// AND THE ABSENCES ARE EXPLAINED TOO, which is the case a first-time reader
	// meets on this map: a species with no ancestry at all, and a bar the record
	// cannot date.
	for _, want := range []string{
		"No crossing of it has ever named a parent species, so the record cannot say ",
		"No crossing of it has ever been recorded, so the drawing can date neither ",
		`"The +" + n.collapsed + " on its link counts extinct generation(s) with no "`} {
		if !strings.Contains(region, want) {
			t.Fatalf("an absence on the drawing goes unexplained: %q missing", want)
		}
	}
	// The legend names the bar itself, which is the mark most likely to be read as
	// a lifespan by someone who has read nothing else.
	if !strings.Contains(page, `<i class="bari"></i><span class="term" data-t="lifespan">a bar</span>`) {
		t.Fatal("the legend's bar carries no term, so the one mark this drawing can be misread " +
			"as has no explanation where it is named")
	}
	// And the keyboard route into the detail is on the legend, not only in the
	// glossary: a reader who cannot use a mouse should not have to hover to find out
	// that hovering is optional.
	if !strings.Contains(page, "or tab to it and press Enter") {
		t.Fatal("the legend never says a row can be opened from the keyboard")
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

// TestTheBrainPanelSharesTheGenealogysAxis is the geometry property the whole
// panel rests on, asserted structurally because a Go test cannot run the page's
// JavaScript (the render-verify shim runs it; this is what stops the shim's
// subject being quietly removed).
//
// THE AXIS RE-FITS. tree.go publishes two left edges and the drawing picks one,
// so "share the axis" cannot mean "draw the same window": it has to mean the
// panel is drawn through THE SAME SCALE OBJECT the tree above it was drawn
// through, in the same paint. Anything else is two clocks that agree until they
// do not.
func TestTheBrainPanelSharesTheGenealogysAxis(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// It is its own box, under the drawing, and it is not a row IN the drawing:
	// an aggregate over every genome the archive could read is not a species, and
	// a row would give it a species' affordances.
	for _, want := range []string{`<div class="brainwrap" id="lfbrain"></div>`,
		"function lfBrainPanel(x, cols, sc){", `svgEl("svg", "brainp")`} {
		if !strings.Contains(page, want) {
			t.Fatalf("the brain panel is missing %q", want)
		}
	}
	if strings.Index(page, `id="lfbox"`) > strings.Index(page, `id="lfbrain"`) {
		t.Fatal("the panel is not below the drawing it shares a clock with")
	}
	// ONE PAINT, ONE cols, ONE sc. The panel is handed the same two objects the
	// rows were drawn with rather than computing its own.
	if !strings.Contains(region, "LFCOLS = cols; LFSCALE = sc;\n  lfBrainPanel(x, cols, sc);") {
		t.Fatal("the panel is not painted from the drawing's own columns and scale in the " +
			"same pass; two derivations of one axis is two places for it to be wrong")
	}
	// EVERY HORIZONTAL POSITION IN THE PANEL IS sc.x OF A REAL MILLISECOND. A
	// mark placed by any other arithmetic is a mark on a second scale, which is
	// the defect the collapsed-run mark on the tree already cost this page once.
	panel := region[strings.Index(region, "function lfBrainPanel(x, cols, sc){"):]
	panel = panel[:strings.Index(panel, "\n/* THE KEYBOARD KEEPS ITS PLACE")]
	if strings.Contains(panel, "cols.plot + ") && !strings.Contains(panel,
		"cols.plot + cols.plotw") {
		t.Fatal("the panel positions something by arithmetic on the plot origin rather than " +
			"through the shared scale")
	}
	for _, want := range []string{"sc.x(tms)", "sc.x(p.tMs)", "sc.x(p.tMs + B.bucketMs)",
		"var span = sc.t1 - sc.t0, step = lfStep(span);"} {
		if !strings.Contains(panel, want) {
			t.Fatalf("the panel does not draw through the shared scale: %q missing", want)
		}
	}
	// The ticks come from the SAME step function over the SAME span, so a vertical
	// line through both pictures is one moment.
	if !strings.Contains(panel, "var first = Math.ceil(sc.t0 / step) * step") {
		t.Fatal("the panel's ticks are not the drawing's ticks")
	}
	// TWO BOXES, ONE CLOCK: on a window too narrow for the drawing both can scroll
	// sideways, and two offsets would not be one clock.
	if !strings.Contains(page, "if (bp.scrollLeft !== box.scrollLeft) bp.scrollLeft = box.scrollLeft;") {
		t.Fatal("the panel's horizontal scroll does not follow the drawing's")
	}
	// It asks for the window the drawing is using, and for that window ONLY —
	// through the same object the paint took its own question from, so the
	// request and the picture cannot be about two different windows.
	if strings.Count(page, `fetch("api/species/brains`) != 1 {
		t.Fatal("the brain panel is not fetched exactly once for the whole view")
	}
	for _, want := range []string{
		"function lfBrainWant(sc, cols){",
		"return {t0: Math.round(sc.t0), buckets: lfBrainBuckets(cols)};",
		`"api/species/brains?from=" + want.t0 +`,
		`"&to=" + Math.round(sc.t1) + "&buckets=" + want.buckets`} {
		if !strings.Contains(page, want) {
			t.Fatalf("the panel does not ask for the window the drawing is actually drawing: "+
				"%q missing", want)
		}
	}
	if !strings.Contains(page, "setInterval(tickBrains, 60000);") {
		t.Fatal("the panel is polled on the two-second cycle, or not at all")
	}
}

// TestTheShortestBrainMarkStaysInsideTheAxis is
// TestTheShortestBarStaysInsideTheAxis's property, applied to the two marks the
// panel below the drawing floors: the stub a lone reading draws, and the newest
// coverage column's minimum width.
//
// A MINIMUM SIZE IS A FLOOR ON VISIBILITY AND NOT A MEASUREMENT. The scale
// already clamps every x to the axis, so a floor applied AFTERWARDS pushes the
// mark straight back out past the clamp — a mark right of the now line claiming
// the record reaches into a future it cannot have. Measured on the live map
// before this: the stub ran to exactly 2.000 px past the now line and the newest
// coverage column to 0.47 px past it, while the tree's own bars — which slide the
// floor LEFT instead — were 0.000 px over at every width.
func TestTheShortestBrainMarkStaysInsideTheAxis(t *testing.T) {
	region := speciesRegion(t)

	// ONE FLOOR FOR THE WHOLE PANEL, so the two marks cannot drift apart.
	for _, want := range []string{"function lfbFloor(x0, x1, min, xend){",
		"if (x1 - x0 < min) x1 = x0 + min;",
		"if (x1 > xend){ x1 = xend; x0 = xend - min; }"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the panel's minimum sizes are not clamped to the axis end: %q missing", want)
		}
	}
	// And both marks go through it, against the scale's own right-hand edge.
	for _, want := range []string{
		"var sx = lfbFloor(sc.x(p.tMs + half), sc.x(p.tMs + half), 2, sc.x(sc.t1));",
		"var xend = sc.x(sc.t1)", "cx = lfbFloor(x0, x1 - 0.5, 1, xend);",
		"hx = lfbFloor(x0, x1, 1, xend);"} {
		if !strings.Contains(region, want) {
			t.Fatalf("a floored mark on the panel still bypasses the clamp: %q missing", want)
		}
	}
	// THE OLD ARITHMETIC IS GONE, both of it: the stub grown rightwards off the
	// clamped x, and the width floored after the subtraction.
	for _, gone := range []string{"(sc.x(p.tMs + half) + 2)", "Math.max(1, x1 - x0 - 0.5)",
		"Math.max(1, x1 - x0)"} {
		if strings.Contains(region, gone) {
			t.Fatalf("the panel still floors a mark after clamping it: %q", gone)
		}
	}
}

// TestTheBrainPanelDrawsCoverageAndGapsAsThemselves is the honesty half.
//
// Three facts have to stay three facts. A slice most of whose genomes were read;
// a slice with crossings and NOT ONE genome ever read; and a slice with no
// crossing at all. The second and third are both "no line" and they are not the
// same thing, and neither is a zero.
func TestTheBrainPanelDrawsCoverageAndGapsAsThemselves(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// A run is a stretch with a reading in EVERY bucket, and only runs are drawn:
	// nothing joins the readings either side of a hole.
	if !strings.Contains(region, "function lfbRuns(pts, medk){") ||
		!strings.Contains(region, "if (pts[i][medk] == null){ cur = null; continue; }") {
		t.Fatal("the panel does not break its lines on a missing reading; a line drawn across " +
			"the record's 24-hour outage is a measurement invented out of a gap")
	}
	// The three coverage marks.
	for _, want := range []string{`svgEl("rect", "cov")`, `svgEl("rect", "covnone")`,
		"var h = lfbCovH(p.n / p.seen);"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the coverage strip is missing %q", want)
		}
	}
	if !strings.Contains(region, "} else if (p.seen > 0){") {
		t.Fatal("a slice with crossings and no measurement is drawn the same as a slice with " +
			"no crossing at all; they are different absences")
	}
	// NO OPACITY RAMP ON THE SERIES. The worst-covered stretch is the newest one,
	// which is the part a reader looks at hardest — fading it would hide the
	// problem in the shape of admitting it.
	if strings.Contains(region, `line.setAttribute("opacity"`) ||
		strings.Contains(region, `band.setAttribute("opacity"`) {
		t.Fatal("the series is faded by coverage; that makes the least certain stretch the " +
			"faintest thing on the panel and confuses 'less certain' with 'smaller'")
	}
	// THE 48-NEURON FLOOR IS ON THE PICTURE, not only in the glossary. A reader
	// who sees "neurons" and a low line has to be told, in the panel, that 48 of
	// every count is fixed and is not drawn.
	if !strings.Contains(region, `"median hidden neurons (every brain also has the same fixed " +`) {
		t.Fatal("the panel never states the fixed neuron floor, so its second series can be " +
			"read as a species with almost no brain")
	}
	// The page reads the floor off the answer rather than hard-coding 48, and the
	// answer really does carry it.
	if !strings.Contains(region, "(B.neuronFloor || 48)") {
		t.Fatal("the panel hard-codes the neuron floor instead of reading the published one")
	}
	raw, err := json.Marshal(BrainHistory{NeuronFloor: BrainNeuronFloor})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"neuronFloor":48`) {
		t.Fatalf("the endpoint does not publish the floor: %s", raw)
	}
	// Every slice answers with its own numbers, under a key that cannot collide
	// with the brain RING's tooltips — which are keyed "lfb"+row on the same page.
	for _, want := range []string{"function lfbTip(p, B){", `SP["lfbp" + i] = lfbTip(p, B);`,
		`hit.setAttribute("data-s", "lfbp" + i);`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the panel's slices carry no tooltip: %q missing", want)
		}
	}
	if !strings.Contains(region, `for (var old in SP){ if (old.indexOf("lfbp") === 0) delete SP[old]; }`) {
		t.Fatal("the panel clears tooltip keys by a prefix that would swallow the brain " +
			"rings' own entries")
	}
	// The tooltip says what the series is, what the sampling rule is, and that the
	// missingness is species-blind — which is the whole argument for drawing a
	// graph over a held sample at all.
	for _, want := range []string{
		`"Measured over " + p.n + " distinct genome"`,
		`", each counted once however often that creature travelled"`,
		`"genome's own fingerprint, so what is missing is missing for reasons that have " +`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the slice tooltip does not explain what it is showing: %q missing", want)
		}
	}
	// And the jargon carries glossary entries, listed so they can be read.
	for _, want := range []string{" braintrend:[", " hiddenneurons:[", " braincoverage:[",
		" braingap:[", " brainwaiting:["} {
		if !strings.Contains(page, want) {
			t.Fatalf("the glossary never explains %q", want)
		}
	}
	if !strings.Contains(page,
		`"minimap","trend","brainsize","braintrend","hiddenneurons","braincoverage","braingap",
    "brainwaiting",`) {
		t.Fatal("the panel's glossary entries exist but are never listed, so nobody can read them")
	}
}

// TestTheBrainPanelReAsksWhenTheQUESTIONChanges is the second half of "it is
// drawn on the tree's clock": a panel that shares an axis it cannot re-fetch for
// is a panel that draws the WRONG window and says nothing about it.
//
// TWO FACES OF ONE DEFECT, both measured live. A cold load's only ask ran before
// the first paint, so it always returned on "no scale yet" and the panel sat on
// "waiting for the measurement" for 60,055 ms and 60,083 ms against a tree drawn
// at 73 ms and 112 ms. And revealing the seed stock moves the axis back 92 hours
// instantly, so for up to a minute 480.6 px of the 857 px plot — 56% of it —
// drew nothing at all, which is the strip's own mark for "no crossing was
// recorded in it", over a stretch really holding 74 measured points and 433,627
// crossings.
//
// THE RULE THIS PINS. The paint is the trigger, because the paint is what knows
// the window. The re-ask key is the WINDOW'S START and the RESOLUTION and NEVER
// the right-hand edge, which is now and moves on every one of the ~30 paints a
// minute. And while the answer is in flight the stretch it does not cover is
// drawn as PENDING, which is a fact about a request and must not be able to be
// read as the record's own emptiness.
func TestTheBrainPanelReAsksWhenTheQUESTIONChanges(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// THE PAINT ASKS. It is the last thing renderLife does with the scale it has
	// just recorded, so there is no state between the two to get out of step.
	if !strings.Contains(region, "LFCOLS = cols; LFSCALE = sc;\n  lfBrainPanel(x, cols, sc);\n"+
		"  // AND THE PAINT IS WHAT TRIGGERS THE PANEL'S FETCH.") {
		t.Fatal("the paint that lays out the panel's window does not ask for it; only a timer " +
			"does, which is how a cold load spent a minute saying it was waiting")
	}
	if !strings.Contains(region, "  lfBrainAsk();\n  lfRefocus(host, mine);") {
		t.Fatal("the ask is not made by the paint")
	}
	// AND THE LOAD-TIME CALL IS GONE, because it could only ever find no scale.
	if strings.Contains(page, "tickBrains(); setInterval") {
		t.Fatal("the page still asks for the panel before there is a window to ask about; that " +
			"call cannot succeed and its failure is what the 60-second wait was")
	}
	if !strings.Contains(page, "setInterval(tickBrains, 60000);") {
		t.Fatal("the panel has no refresh for the one edge that moves by itself")
	}
	// THE KEY IS THE START AND THE RESOLUTION, AND NOT NOW.
	for _, want := range []string{
		"function lfBrainSame(req, want){",
		"return !!req && req.buckets === want.buckets &&",
		"Math.abs(req.t0 - want.t0) <= lfbTol(LFB);",
		"if (lfBrainSame(LFBFOR, want) || lfBrainSame(LFBASK, want)){"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the re-fetch is not keyed on the question the panel is asking: %q missing",
				want)
		}
	}
	key := region[strings.Index(region, "function lfBrainWant(sc, cols){"):]
	key = key[:strings.Index(key, "\nfunction lfBrainSame")]
	if strings.Contains(key, "t1") || strings.Contains(key, "Date.now") {
		t.Fatalf("the re-fetch key carries the right-hand edge, which is now: that is a fetch on "+
			"every paint, about thirty a minute, to move one edge by a pixel:\n%s", key)
	}
	// AND IT IS DEBOUNCED, because a resize drag repaints by the hundred.
	for _, want := range []string{"LFB_REASK = 200", "if (LFBT) clearTimeout(LFBT);",
		"LFBT = setTimeout(function(){ LFBT = 0; tickBrains(); }, LFB_REASK);"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the re-ask is not coalesced: %q missing", want)
		}
	}
	// AN OVERTAKEN ANSWER IS DROPPED rather than painted over a window the reader
	// has already left.
	for _, want := range []string{"var mine = ++LFBSEQ;", "if (mine !== LFBSEQ) return;",
		"LFB = B; LFBFOR = want;"} {
		if !strings.Contains(page, want) {
			t.Fatalf("two answers in flight can land in the wrong order: %q missing", want)
		}
	}
	// WAITING IS DRAWN, AND IT IS NOT THE RECORD'S OWN EMPTINESS. One tolerance
	// serves both the mark and the re-ask, so the panel cannot be waiting for
	// something it has decided not to ask for.
	for _, want := range []string{"function lfbTol(B){", "function lfbPending(B, sc, plotL, plotR){",
		"if (B.fromMs && B.fromMs > sc.t0 + tol) out.push([plotL, sc.x(B.fromMs)]);",
		"if (B.toMs && B.toMs < sc.t1 - tol) out.push([sc.x(B.toMs), plotR]);",
		`svgEl("rect", "pending")`, `svgEl("line", "pendedge")`, `svgEl("text", "pendlbl")`,
		`SP["lfbpwait" + pi] = {title: "brains over time", body: held`,
		"THIS IS NOT AN "} {
		if !strings.Contains(region, want) {
			t.Fatalf("a window with no answer yet is drawn as an empty record: %q missing", want)
		}
	}
	if !strings.Contains(region, "if (!B || !B.points || !B.points.length) return [[plotL, plotR]];") {
		t.Fatal("a panel with no answer at all does not draw the whole plot as pending")
	}
	// The wash is its own mark, in the dim the panel uses for structure — NEVER
	// the amber the two real absences wear.
	if !strings.Contains(page, "svg.brainp .pending{fill:var(--dim)") ||
		strings.Contains(page, "svg.brainp .pending{fill:var(--warn)") {
		t.Fatal("the pending wash wears the colour one of the two real absences wears")
	}
	// AND A STALE ANSWER'S SLICES ARE NOT CLAMPED ONTO THE EDGE OF A CLOCK THEY
	// ARE NOT ABOUT, which would stack dozens of columns in a pixel, each of them
	// a coverage claim about a moment that is not drawn.
	for _, want := range []string{"function lfbVisible(B, sc){",
		"if (pts[i].tMs + bw <= sc.t0 || pts[i].tMs >= sc.t1) continue;",
		"var pts = lfbVisible(B, sc);"} {
		if !strings.Contains(region, want) {
			t.Fatalf("slices off the drawn clock are still drawn on its edge: %q missing", want)
		}
	}
}

// TestTheCoverageStripReadsWhereThisRigActuallyLives is the third defect: the
// strip's mark and the strip's number both broke down below 20% coverage, which
// is where 65 of this record's 68 measured slices sit.
//
// THE HEIGHT ORDERING WAS INVERTED. max(1, f × 11) against a 2 px amber tick made
// every slice under 18.2% draw a SHORTER mark for "we read some of it" than the
// mark for "we read none of it" — 21 of 23 filled bars on the live map — so the
// only thing separating the two facts was colour, and the glossary taught the
// opposite of what the picture showed.
//
// AND THE NUMBER SAID THE ONE THING IT MUST NOT. Math.round made the tooltip
// report "out of 6282 the record says crossed — 0% of them" on 3 of 23 slices
// that plainly had readings, which is exactly the claim the amber tick is
// reserved for, said in words about a slice drawn as measured.
func TestTheCoverageStripReadsWhereThisRigActuallyLives(t *testing.T) {
	page := statusPageHTML
	region := speciesRegion(t)

	// THE TWO HEIGHTS ARE WRITTEN DOWN TOGETHER, which is the only way the
	// relation between them can be kept true.
	if !strings.Contains(region, "var LFB_COVNONE = 2, LFB_COVMIN = 3;") {
		t.Fatal("the two coverage marks' heights are not stated as one relation, so nothing " +
			"stops the mark for 'some' shrinking under the mark for 'none' again")
	}
	for _, want := range []string{"function lfbCovH(f){", "if (!(f > 0)) return LFB_COVMIN;",
		"return LFB_COVMIN + Math.sqrt(f) * (LFB_COVH - LFB_COVMIN);"} {
		if !strings.Contains(region, want) {
			t.Fatalf("the filled column's height is not floored above the tick: %q missing", want)
		}
	}
	if strings.Contains(region, "Math.max(1, f * LFB_COVH)") {
		t.Fatal("the filled column is still the fraction times the strip, which draws 21 of 23 " +
			"of this map's measured slices shorter than the mark for nothing measured")
	}
	// The tick is drawn from the same constant rather than a literal that could
	// drift away from it.
	for _, want := range []string{`tick.setAttribute("y", String(covTop + LFB_COVH - LFB_COVNONE));`,
		`tick.setAttribute("height", String(LFB_COVNONE));`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the amber tick's height is not the one the filled column is floored "+
				"against: %q missing", want)
		}
	}
	// THE SCALE IS ON THE PICTURE, not only in the glossary — the same rule the
	// 48-neuron floor is on the panel for.
	if !strings.Contains(region,
		`clbl.textContent = "how much of it was measured (heights on a square-root scale)";`) {
		t.Fatal("the strip never says its heights are not linear in the fraction")
	}
	if !strings.Contains(page, "THE HEIGHTS ARE ON A SQUARE-ROOT SCALE") {
		t.Fatal("the glossary does not explain the scale or the floor it draws above")
	}
	// AND THE NUMBER. A non-zero count never prints as 0%, and a count that is not
	// all of them never prints as 100%.
	for _, want := range []string{"function lfbShare(n, seen){",
		`if (n >= seen) return "100%";`, `if (f < 0.5) return "less than 1%";`,
		`if (f > 99.5) return "almost all";`,
		`lfbShare(p.n, p.seen) + " of them";`} {
		if !strings.Contains(region, want) {
			t.Fatalf("the tooltip's share can still round a reading away: %q missing", want)
		}
	}
	if strings.Contains(region, "Math.min(100, Math.round(100 * p.n / p.seen))") {
		t.Fatal("the tooltip still rounds its share, which said '0% of them' about 3 of 23 " +
			"slices with readings in hand")
	}
}
