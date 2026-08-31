package archive

// WP4's tests for DQ7 and contract-b-m4.md §22, B30: attacker-chosen text, the
// escaping obligation, and the operator-side render deny list.
//
// TestAMarkupSpeciesNameIsNeverRenderedAsMarkup is the one B30 exists for. §13
// item 7 named the surface and was blunt about the split — the wire's answer is
// the shape check and the cap, and it is done; the renderer's answer is its own
// — and then B30 said the quiet part: AN ESCAPING RULE WRITTEN IN A CONTRACT IS
// NOT CODE. Nothing in this project asserted it until here, and CI is the only
// form in which the rule is true of a running system.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/wire"
)

// The four names this file attacks with. Each is 64 UTF-8 bytes or fewer, so
// each is a name a peer may legally put on the wire (§10.1's inventory).
const (
	markupName   = `<script>alert(1)</script>`
	attrName     = `" onload="alert(1)`
	terminalName = "\x1b]0;pwned\x07\x1b[31mred"
	scriptClose  = `</script><img src=x onerror=alert(1)>`
)

// TestAMarkupSpeciesNameIsNeverRenderedAsMarkup is B30's CI test, over both
// rendering surfaces at once: the page and ringstat's terminal.
//
// A Go test cannot run the page's JavaScript, so the page half is asserted the
// only two ways that are actually decisive:
//
//	THE SERVED DOCUMENT NEVER CONTAINS THE NAME. The page is static HTML that
//	fetches JSON; no peer's text is ever interpolated into the document the
//	browser parses, so there is no server-side injection surface at all.
//
//	AND THE FENCED REGION NEVER ASSIGNS MARKUP. Every function that draws a name
//	lives inside SPECIES CENSUS — BEGIN/END and that region assigns no innerHTML,
//	which is the property TestCensusNamesNeverBecomeMarkup pins and this test
//	re-states against a name that would actually execute.
func TestAMarkupSpeciesNameIsNeverRenderedAsMarkup(t *testing.T) {
	stats := census(9, 0,
		entry(markupName, "copedylanus", 5, 0),
		entry("Cyanea", scriptClose, 4, 0))
	stats.MigrationExclude = &wire.ExcludeList{Names: []string{attrName}}
	stats.ModVersion = markupName
	stats.ContractAVersion = "contract-a/2.4"

	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{
			markupSlot(1, 0, 0, markupName, stats),
			slot(2, 1, 0, true, census(3, 0, entry("Izus", "velox", 3, 0))),
		},
	}
	a := newViewFixture(t, status, time.Second)
	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)

	// ---- the page. The document a browser parses carries no peer's text.
	page := get(t, ts.URL+"/")
	for _, name := range []string{markupName, scriptClose, attrName} {
		if strings.Contains(page, name) {
			t.Fatalf("the served HTML document interpolates a peer's chosen text (%q); the page "+
				"is static and every name must arrive as JSON a script sets as TEXT", name)
		}
	}
	// The fence is what makes that true of the JSON path too.
	region := speciesRegion(t)
	if strings.Contains(region, "innerHTML") {
		t.Fatal("the species-rendering region assigns innerHTML; a census name is " +
			"attacker-chosen text and may only reach the DOM as a text node")
	}

	// ---- the JSON. The name survives VERBATIM and it survives ESCAPED: a
	// reader that parses it gets the world's own spelling, and a reader that
	// splices the body into a document gets no tag.
	body := get(t, ts.URL+"/api/status")
	if strings.Contains(body, "<script>") {
		t.Fatalf("the status JSON carries a raw <script> tag:\n%s", body)
	}
	var view Status
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode /api/status: %v", err)
	}
	if got := view.Slots[0].Species[0].GenericName; got != markupName {
		t.Fatalf("the census name was REPAIRED to %q; §17 A36 forbids repairing a label, and "+
			"escaping is the renderer's job and not the record's", got)
	}

	// ---- the terminal. ringstat renders the same Status, and a terminal's
	// markup is the control character.
	var out strings.Builder
	RenderRingstat(&out, view)
	RenderSettings(&out, view)
	RenderSpecies(&out, view, nil)
	assertNoTerminalControls(t, out.String())
	if !strings.Contains(out.String(), "script") {
		t.Fatal("ringstat printed no part of the attacking name at all; suppression is the deny " +
			"list's job and escaping must not silently drop text")
	}
}

// TestATerminalEscapeInANameIsNeverExecuted is the ringstat half on its own,
// with a name that carries a real OSC and a real CSI. B30 binds the terminal
// IDENTICALLY to the page, and a terminal's injection story is its own: a name
// carrying ESC can retitle a window, repaint the screen or — on some terminals
// — put text into the reader's input buffer.
func TestATerminalEscapeInANameIsNeverExecuted(t *testing.T) {
	stats := census(4, 0, entry(terminalName, "velox", 4, 0))
	stats.ModVersion = "0.6.4\x1b[2J"
	stats.MigrationExclude = &wire.ExcludeList{Names: []string{"Basic\x07bibite"}}
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{markupSlot(1, 0, 0, "peer\x1b[31m-one", stats)},
	}
	a := newViewFixture(t, status, time.Second)
	view := a.StatusView()

	var out strings.Builder
	RenderRingstat(&out, view)
	RenderSettings(&out, view)
	RenderSpecies(&out, view, nil)
	assertNoTerminalControls(t, out.String())
}

func assertNoTerminalControls(t *testing.T, out string) {
	t.Helper()
	for i, r := range out {
		if r == '\n' || r == '\t' {
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			t.Fatalf("ringstat emitted control character %#U at byte %d; a name carrying ESC is "+
				"a name that can repaint the reader's screen (contract-b-m4.md §22, B30)", r, i)
		}
	}
}

// TestTheDenyListSuppressesTheViewAndNeverTheRecord is DQ7's minimum: an
// operator action between IGNORE IT and EVICT THE PEER that costs the
// suppressed world nothing, needs no wire field and no peer's cooperation.
//
// It also pins the half that must never quietly grow: THE RECORD IS UNTOUCHED.
// D11 and §10 make the ledger a thing nothing evicts from, so M5 promises
// removal from the view and explicitly does not promise removal from the
// record.
func TestTheDenyListSuppressesTheViewAndNeverTheRecord(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "deny.txt")
	write(t, file, "# an operator's list\nCyanea velox\npeer:peer-loud\n")

	stats := census(9, 1,
		entry("Cyanea", "velox", 5, 1),
		entry("Izus", "copedylanus", 4, 0))
	stats.Keeper, stats.WorldName = "ada", "Tidepool"
	loud := census(6, 0, entry("Rude", "namus", 6, 0))
	loud.MigrationExclude = &wire.ExcludeList{Names: []string{"Basic bibite"}}
	loud.ModVersion = "0.6.4"
	// The two strings §33 B49 added are the peer's own, so a denied peer loses
	// them with the rest of what it authored.
	loud.Keeper, loud.WorldName = "Rude Keeper", "Rude World"

	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{
			markupSlot(1, 0, 0, "peer-quiet", stats),
			markupSlot(2, 1, 0, "peer-loud", loud),
		},
	}
	a := newRecognizedFixture(t, status, time.Second, time.Now().UnixMilli())
	deny, err := NewDenyList(file)
	if err != nil {
		t.Fatalf("NewDenyList: %v", err)
	}
	a.deny = deny

	raw := a.StatusView()
	shown := a.deny.ApplyStatus(raw)

	// The named species is suppressed on the quiet world, and its NEIGHBOUR in
	// the same census is not: a deny list is a list, not a mood.
	if shown.Slots[0].Species[0].GenericName != Suppressed {
		t.Fatalf("the denied species still renders as %q", shown.Slots[0].Species[0].GenericName)
	}
	if shown.Slots[0].Species[0].Bibites != 5 {
		t.Fatal("suppression changed a count; the map still says how many organisms are alive")
	}
	if shown.Slots[0].Species[1].GenericName != "Izus" {
		t.Fatalf("an undenied species was suppressed: %q", shown.Slots[0].Species[1].GenericName)
	}
	// The denied PEER loses every string it authored — its id, its census, its
	// exclusion list, its version — and keeps every number the archive and the
	// relay computed about it.
	if shown.Slots[1].PeerID != Suppressed {
		t.Fatalf("the denied peer's id still renders as %q", shown.Slots[1].PeerID)
	}
	if shown.Slots[1].Species[0].GenericName != Suppressed {
		t.Fatal("a denied peer's census still renders")
	}
	if shown.Slots[1].MigrationExclude[0] != Suppressed {
		t.Fatal("a denied peer's exclusion list still renders")
	}
	if shown.Slots[1].ModVersion != Suppressed {
		t.Fatal("a denied peer's version string still renders")
	}
	// ...and the two names it chose about ITSELF go BLANK rather than marked:
	// they are the only fields a reader groups by, so a marker would be a shared
	// identity. See suppressedName and the test below.
	if shown.Slots[1].Keeper != "" || shown.Slots[1].WorldName != "" {
		t.Fatalf("a denied peer keeps the two strings it chose about itself: keeper %q, "+
			"world %q", shown.Slots[1].Keeper, shown.Slots[1].WorldName)
	}
	if shown.Slots[0].Keeper != "ada" || shown.Slots[0].WorldName != "Tidepool" {
		t.Fatalf("an undenied world lost its name: keeper %q, world %q",
			shown.Slots[0].Keeper, shown.Slots[0].WorldName)
	}
	if !shown.Slots[1].Live || shown.Slots[1].Slot != 2 {
		t.Fatal("suppression hid a world's liveness; that is Risk 5, not moderation")
	}

	// THE RECORD IS UNTOUCHED. The view the archive built, which is what
	// metrics.jsonl serializes, still holds every name.
	if raw.Slots[0].Species[0].GenericName != "Cyanea" {
		t.Fatal("suppression reached back into the archive's own view; the record is not where " +
			"suppression happens")
	}
	if raw.Slots[1].PeerID != "peer-loud" {
		t.Fatal("suppression rewrote the archive's own status")
	}
}

// TestTheDenyListIsRereadWithoutARestart. An archive restart costs minutes of
// dark status page and a permanent hole in the record of what crossed while it
// replayed, so "edit the file" has to be the whole procedure.
func TestTheDenyListIsRereadWithoutARestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "deny.txt")
	write(t, file, "# nothing yet\n")
	deny, err := NewDenyList(file)
	if err != nil {
		t.Fatalf("NewDenyList: %v", err)
	}
	if !deny.Empty() {
		t.Fatal("an empty file denied something")
	}
	// Backdate the load so the reload interval has passed, which is what a real
	// archive gets for free by running for longer than two seconds.
	deny.mu.Lock()
	deny.loadedAt = time.Now().Add(-time.Minute)
	deny.mu.Unlock()
	write(t, file, "Cyanea velox\n")
	if deny.Empty() {
		t.Fatal("the deny list was not re-read after the file changed")
	}
	if !deny.DeniesSpecies("Cyanea", "velox") {
		t.Fatal("the reloaded entry does not match")
	}
}

// TestADenyEntryMatchesTheNormalizedNameAndAGenus covers the two matching rules
// an operator will actually rely on: the name as the page shows it, whatever
// whitespace the owning world holds, and a whole genus in one line.
func TestADenyEntryMatchesTheNormalizedNameAndAGenus(t *testing.T) {
	file := filepath.Join(t.TempDir(), "deny.txt")
	write(t, file, "Cyanea  velox\nRudus\n")
	deny, err := NewDenyList(file)
	if err != nil {
		t.Fatalf("NewDenyList: %v", err)
	}
	// A34's normalization on both sides: two spellings that differ only in
	// whitespace are one species for comparison.
	if !deny.DeniesSpecies("Cyanea", "velox") {
		t.Fatal("a doubled space in the operator's entry stopped it matching")
	}
	if !deny.DeniesSpecies("Cyanea ", " velox") {
		t.Fatal("a doubled space in the WORLD's spelling stopped it matching")
	}
	// A one-word entry is a genus, so a mutating population cannot outrun the
	// list one specific name at a time.
	if !deny.DeniesSpecies("Rudus", "anything") {
		t.Fatal("a one-word entry did not suppress the genus")
	}
	if deny.DeniesSpecies("Izus", "copedylanus") {
		t.Fatal("an unrelated species was suppressed")
	}
}

// TestTheDenyListReachesEverySurfaceTheArchiveServes: the page's JSON, the
// species index, the history answer and the hop feed. One join in one place is
// what stops the terminal tool and the page disagreeing.
func TestTheDenyListReachesEverySurfaceTheArchiveServes(t *testing.T) {
	file := filepath.Join(t.TempDir(), "deny.txt")
	write(t, file, "Cyanea velox\n")

	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{
			markupSlot(1, 0, 0, "peer-one", census(9, 0,
				entry("Cyanea", "velox", 5, 0), entry("Izus", "copedylanus", 4, 0))),
			markupSlot(2, 1, 0, "peer-two", census(3, 0, entry("Cyanea", "velox", 3, 0))),
		},
	}
	a := newViewFixture(t, status, time.Second)
	deny, err := NewDenyList(file)
	if err != nil {
		t.Fatalf("NewDenyList: %v", err)
	}
	a.deny = deny
	a.mu.Lock()
	a.observeHopLocked(Hop{MigrationID: "m1", AtMs: time.Now().UnixMilli(),
		FromSlot: 1, ToSlot: 2, ExitEdge: contracta.EdgeE,
		Species: &wire.Species{GenericName: "Cyanea", SpecificName: "velox"}})
	a.mu.Unlock()

	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)

	// The genealogy too: an ancestor's name arrives by a different route (a
	// migration envelope's parentGenericName) and a suppression that reached
	// every OTHER surface would leave the one nothing else on the page reads.
	// And the shared trend answer, which is KEYED ON NAMES and is the newest
	// surface — a species' population shape is not a name, but the key it is
	// filed under is one.
	for _, path := range []string{"/api/status", "/api/species", "/api/hops",
		"/api/species/history?key=Cyanea+velox", "/api/species/tree",
		"/api/species/trends"} {
		body := get(t, ts.URL+path)
		if strings.Contains(body, "velox") {
			t.Fatalf("%s still renders the denied name:\n%s", path, body)
		}
		if !strings.Contains(body, Suppressed) {
			t.Fatalf("%s suppressed the name without SAYING it suppressed anything; an operator "+
				"reading their own deny list has to be able to see it working:\n%s", path, body)
		}
	}
	// The undenied species is untouched everywhere.
	if !strings.Contains(get(t, ts.URL+"/api/species"), "Izus") {
		t.Fatal("the species index lost a name nobody denied")
	}
}

// TestTheDenyListSuppressesAQuotedParentName covers the one name-bearing field
// that is not the row's own: BOTH the flat index and the merged view print the
// raw parent species a record named, and a species is denied or not on its own
// account. So a row nobody denied, quoting a species somebody did, must lose the
// quote and keep everything else — which is the difference between suppressing a
// name and suppressing a row.
func TestTheDenyListSuppressesAQuotedParentName(t *testing.T) {
	file := filepath.Join(t.TempDir(), "deny.txt")
	write(t, file, "Alpha nullus\n")
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(20, 0,
			entry("Beta", "one", 10, 0), entry("Gamma", "two", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	deny, err := NewDenyList(file)
	if err != nil {
		t.Fatalf("NewDenyList: %v", err)
	}
	a.deny = deny
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	// Only ONE of the two descends from the denied species, so the other is the
	// control: nothing about it may change.
	a.observeSpeciesLocked(child(base, "Beta", "one", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+1, "Gamma", "two", "Delta", "two"))
	a.mu.Unlock()

	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)
	for _, path := range []string{"/api/species", "/api/species/tree"} {
		body := get(t, ts.URL+path)
		if strings.Contains(body, "Alpha") {
			t.Fatalf("%s prints the denied species as another row's parent:\n%s", path, body)
		}
		if !strings.Contains(body, "Beta") || !strings.Contains(body, "Delta two") {
			t.Fatalf("%s suppressed a row instead of the name it quoted:\n%s", path, body)
		}
	}
}

// TestAKeeperOrWorldEntrySuppressesOneNameAndNotTheWorld covers the two rule
// prefixes §33 B49's strings needed, and the reason they are not just
// `peer:` entries: a keeper handle is one name, and a peer entry takes down the
// world's census, its versions and its refusal text with it. An operator asked
// to choose between ignoring a handle and hiding a working world has no usable
// tool at all, so `keeper:` and `world:` are the narrow one.
func TestAKeeperOrWorldEntrySuppressesOneNameAndNotTheWorld(t *testing.T) {
	file := filepath.Join(t.TempDir(), "deny.txt")
	// The doubled space is deliberate: entries match on the NORMALIZED string on
	// both sides, exactly as a species entry does.
	write(t, file, "# names, not peers\nkeeper:Rude  Handle\nworld:Rude World\n")

	loud := census(6, 0, entry("Izus", "velox", 6, 0))
	loud.Keeper, loud.WorldName = "Rude Handle", "Rude World"
	quiet := census(9, 0, entry("Cyanea", "borealis", 9, 0))
	quiet.Keeper, quiet.WorldName = "ada", "Tidepool"
	// A SECOND world under the same handle. A keeper entry is about the name and
	// not about one slot, so both lose it.
	second := census(4, 0, entry("Izus", "gracilis", 4, 0))
	second.Keeper, second.WorldName = "Rude Handle", "Saltmarsh"

	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 3, Height: 1}, SlotCount: 3,
		Slots: []contractb.SlotInfo{
			markupSlot(1, 0, 0, "peer-quiet", quiet),
			markupSlot(2, 1, 0, "peer-loud", loud),
			markupSlot(3, 2, 0, "peer-second", second),
		},
	}
	a := newRecognizedFixture(t, status, time.Second, time.Now().UnixMilli())
	deny, err := NewDenyList(file)
	if err != nil {
		t.Fatalf("NewDenyList: %v", err)
	}
	a.deny = deny

	// The matchers on their own, including the whitespace rule on both sides.
	if !deny.DeniesKeeper("Rude Handle") || !deny.DeniesKeeper("Rude   Handle") {
		t.Fatal("a keeper entry does not match the handle the page shows")
	}
	if !deny.DeniesWorld("Rude World") {
		t.Fatal("a world entry does not match the name the page shows")
	}
	// A keeper entry is not a world entry and neither is a species entry: three
	// namespaces, and a line in one must not reach the others.
	if deny.DeniesWorld("Rude Handle") || deny.DeniesKeeper("Rude World") {
		t.Fatal("a keeper entry and a world entry match each other's namespace")
	}
	if deny.DeniesSpecies("Rude", "Handle") || deny.DeniesKeeper("ada") {
		t.Fatal("a keeper entry reached the species list, or an unrelated handle")
	}
	// THERE IS NO ONE-WORD RULE HERE. A handle is one string, and a partial match
	// would be a filter rather than a moderation decision.
	if deny.DeniesKeeper("Rude") || deny.DeniesWorld("Rude") {
		t.Fatal("a keeper or world entry matched on a prefix; that suppresses names nobody " +
			"looked at")
	}

	shown := deny.ApplyStatus(a.StatusView())
	if shown.Slots[1].Keeper != "" || shown.Slots[1].WorldName != "" {
		t.Fatalf("the denied handle and world name still render: %q / %q",
			shown.Slots[1].Keeper, shown.Slots[1].WorldName)
	}
	// AND THE WORLD IS WHOLE. Its id, its liveness, its population and its
	// census are the archive's and the relay's own numbers, and a name taken
	// down is not a world hidden (Risk 5).
	if shown.Slots[1].PeerID != "peer-loud" || !shown.Slots[1].Live {
		t.Fatalf("suppressing a name hid the world: %+v", shown.Slots[1])
	}
	if shown.Slots[1].Species[0].GenericName != "Izus" {
		t.Fatalf("suppressing a handle took the census with it: %q",
			shown.Slots[1].Species[0].GenericName)
	}
	// The second world under the same handle loses the handle and KEEPS its own
	// name, which nobody denied.
	if shown.Slots[2].Keeper != "" {
		t.Fatalf("the same handle on another world still renders: %q", shown.Slots[2].Keeper)
	}
	if shown.Slots[2].WorldName != "Saltmarsh" {
		t.Fatalf("a world nobody denied lost its name: %q", shown.Slots[2].WorldName)
	}
	// And the world nobody named in the file is untouched on both fields.
	if shown.Slots[0].Keeper != "ada" || shown.Slots[0].WorldName != "Tidepool" {
		t.Fatalf("an unrelated world was suppressed: %+v", shown.Slots[0])
	}

	// One join in one place: the served JSON says the same thing the view does.
	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)
	body := get(t, ts.URL+"/api/status")
	if strings.Contains(body, "Rude Handle") || strings.Contains(body, "Rude World") {
		t.Fatalf("/api/status still renders a denied name:\n%s", body)
	}
	if !strings.Contains(body, "Saltmarsh") || !strings.Contains(body, "Tidepool") {
		t.Fatalf("/api/status suppressed a name nobody denied:\n%s", body)
	}
}

// TestASuppressedKeeperIsBlankedAndNeverASharedName is the rule the marker got
// wrong for these two fields alone.
//
// Every other denied string on this surface renders as "[suppressed]", and it
// should: a page that dropped a census row would be lying about how many species
// a world holds. But keeper and worldName are the only fields anything GROUPS
// BY — the landing page's leaders module gathers worlds on the exact handle —
// so a marker is not a marker there, it is a NAME, shared by every moderated
// participant on the map: one row summing strangers' simulated time together,
// capable of ranking above the people who chose their own handles, with every
// world under it reading "kept by [suppressed]".
//
// Blank is the state the whole system already agrees about: absence is UNKNOWN
// (§33, §10.1), the page falls back to the world's slot, and the leaders module
// skips a world with no keeper. So a suppressed name is NO NAME.
func TestASuppressedKeeperIsBlankedAndNeverASharedName(t *testing.T) {
	file := filepath.Join(t.TempDir(), "deny.txt")
	// TWO different denied handles, on three worlds, plus a denied PEER. Every
	// one of them has to end up unnamed on its own, and none of them may end up
	// sharing a name with any other.
	write(t, file, "keeper:Rude Handle\nkeeper:Worse Handle\nworld:Rude World\npeer:peer-loud\n")

	first := census(6, 0, entry("Izus", "velox", 6, 0))
	first.Keeper, first.WorldName = "Rude Handle", "Rude World"
	second := census(4, 0, entry("Izus", "gracilis", 4, 0))
	second.Keeper, second.WorldName = "Worse Handle", "Saltmarsh"
	loud := census(3, 0, entry("Izus", "minor", 3, 0))
	loud.Keeper, loud.WorldName = "Third Handle", "Third World"
	quiet := census(9, 0, entry("Cyanea", "borealis", 9, 0))
	quiet.Keeper, quiet.WorldName = "ada", "Tidepool"

	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 4, Height: 1}, SlotCount: 4,
		Slots: []contractb.SlotInfo{
			markupSlot(1, 0, 0, "peer-quiet", quiet),
			markupSlot(2, 1, 0, "peer-first", first),
			markupSlot(3, 2, 0, "peer-second", second),
			markupSlot(4, 3, 0, "peer-loud", loud),
		},
	}
	a := newRecognizedFixture(t, status, time.Second, time.Now().UnixMilli())
	deny, err := NewDenyList(file)
	if err != nil {
		t.Fatalf("NewDenyList: %v", err)
	}
	a.deny = deny

	shown := deny.ApplyStatus(a.StatusView())
	for _, i := range []int{1, 2, 3} {
		v := shown.Slots[i]
		if v.Keeper == Suppressed || v.WorldName == Suppressed {
			t.Fatalf("slot %d renders the marker as a name: keeper %q, world %q. Two "+
				"moderated participants would then share one identity", v.Slot, v.Keeper, v.WorldName)
		}
		if v.Keeper != "" {
			t.Fatalf("slot %d still publishes a denied keeper: %q", v.Slot, v.Keeper)
		}
	}
	// Suppression took the NAMES and nothing else: three whole worlds, still
	// live, still counted, still reporting a census (Risk 5).
	for _, i := range []int{1, 2, 3} {
		if v := shown.Slots[i]; !v.Live || len(v.Species) == 0 {
			t.Fatalf("suppressing a handle hid the world in slot %d: %+v", v.Slot, v)
		}
	}
	// The world nobody denied keeps both of its names.
	if shown.Slots[0].Keeper != "ada" || shown.Slots[0].WorldName != "Tidepool" {
		t.Fatalf("an unrelated world was suppressed: %+v", shown.Slots[0])
	}

	// THE GROUPING ITSELF, at the seam the page reads. The leaders module keys on
	// the served `keeper` string, so what decides this is whether the JSON
	// carries one: a blanked field is omitted (omitempty), an omitted field is
	// falsy in the browser, and the module's first act is to skip a world that
	// has none.
	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)
	body := get(t, ts.URL+"/api/status")
	// The marker is still right for the peer id beside them — that field is an
	// address nothing groups people by, and dropping it would hide a world.
	for _, forbidden := range []string{
		`"keeper":"` + Suppressed + `"`, `"worldName":"` + Suppressed + `"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("/api/status serves %s, a name every moderated world would share:\n%s",
				forbidden, body)
		}
	}
	var view Status
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode /api/status: %v", err)
	}
	named := map[string]int{}
	for _, v := range view.Slots {
		if v.Keeper != "" {
			named[v.Keeper]++
		}
	}
	if len(named) != 1 || named["ada"] != 1 {
		t.Fatalf("the served keepers are %v, want only the one nobody denied", named)
	}
	if !strings.Contains(recognitionRegion(t), "if (!k) continue;") {
		t.Fatal("the leaders module no longer skips a world with no keeper, so a blanked " +
			"handle would be gathered into a group of its own")
	}
}

// markupSlot is `slot` with a chosen peerId, because the peer id is another
// peer's chosen string and §10.1's inventory counts it.
func markupSlot(n, col, row int, peerID string, stats *contractb.PeerStats) contractb.SlotInfo {
	s := slot(n, col, row, true, stats)
	s.PeerID = peerID
	return s
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func get(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return string(body)
}
