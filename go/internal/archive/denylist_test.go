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
	loud := census(6, 0, entry("Rude", "namus", 6, 0))
	loud.MigrationExclude = &wire.ExcludeList{Names: []string{"Basic bibite"}}
	loud.ModVersion = "0.6.4"

	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{
			markupSlot(1, 0, 0, "peer-quiet", stats),
			markupSlot(2, 1, 0, "peer-loud", loud),
		},
	}
	a := newViewFixture(t, status, time.Second)
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
	for _, path := range []string{"/api/status", "/api/species", "/api/hops",
		"/api/species/history?key=Cyanea+velox", "/api/species/tree"} {
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
