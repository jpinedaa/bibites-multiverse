package sidecar

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"multiverse/internal/archive"
	"multiverse/internal/contracta"
	"multiverse/internal/wire"
)

// rawCensus is the census the rig sends: FOUR ENTRIES, DESCENDING, and every
// awkward thing contract-a.md §17 A36 makes legal on this field and only this
// field.
//
//   - "Izus " carries a TRAILING SPACE, so the world's own display name is
//     "Izus  copedylanus" with a doubled space — the case A36 was written for,
//     and 27% of one measured world's living population.
//   - "Izus" is the same species without it. It is a SECOND Species record in
//     that world and the census reports two: nothing anywhere de-duplicates.
//   - " " is a half that is NOTHING BUT whitespace, which passes, because that
//     is a name the world is genuinely displaying.
//   - the last name carries the three characters a JSON encoder escapes and a
//     non-ASCII rune, so a hop that re-encodes instead of copying shows up.
const rawCensus = `"species":[` +
	`{"genericName":"Izus ","specificName":"copedylanus","bibites":96,"eggs":14},` +
	`{"genericName":"Izus","specificName":"copedylanus","bibites":61,"eggs":9},` +
	`{"genericName":" ","specificName":"anonymus","bibites":38,"eggs":11},` +
	`{"genericName":"Cyanëa<&>","specificName":"velox\"issima","bibites":17,"eggs":3}` +
	`]`

func wantCensus() []wire.CensusEntry {
	return []wire.CensusEntry{
		{GenericName: "Izus ", SpecificName: "copedylanus", Bibites: 96, Eggs: 14},
		{GenericName: "Izus", SpecificName: "copedylanus", Bibites: 61, Eggs: 9},
		{GenericName: " ", SpecificName: "anonymus", Bibites: 38, Eggs: 11},
		{GenericName: "Cyanëa<&>", SpecificName: `velox"issima`, Bibites: 17, Eggs: 3},
	}
}

// findSlotView looks for one slot in the archive's operator view, and answers
// "not yet" rather than failing: before the first PEER_STATUS arrives the view
// legitimately has no slots at all.
func findSlotView(arc *archive.Archive, slot int) (archive.SlotView, bool) {
	for _, v := range arc.StatusView().Slots {
		if v.Slot == slot {
			return v, true
		}
	}
	return archive.SlotView{}, false
}

func waitCensus(t *testing.T, arc *archive.Archive, slot int, what string,
	ok func(archive.SlotView) bool) archive.SlotView {
	t.Helper()
	var got archive.SlotView
	waitFor(t, 20*time.Second, what, func() bool {
		v, found := findSlotView(arc, slot)
		if !found || !v.StatsKnown || !ok(v) {
			return false
		}
		got = v
		return true
	})
	return got
}

// TestCensusCrossesTheRigAndAbsentIsNotEmpty is the whole path in one test:
// mod -> sidecar -> relay -> archive -> the operator view the page renders,
// with contract-a.md §17 A35's three states held apart at every hop.
//
//	PRESENT — every name arrives byte for byte, in the sender's order, and the
//	          doubled space is still doubled.
//	ABSENT  — a mod that sends no census leaves the slot's species UNKNOWN. Not
//	          zero, not empty, not the last census anybody saw.
//	EMPTY   — the SAME mod sending `[]` is a different and stronger fact, and it
//	          must not read as the absent case.
//
// The absent/empty pair is the reason the field is OPTIONAL rather than
// defaulted, so it is asserted on the JSON bytes as well as on the Go value: a
// `speciesKnown` that a client cannot see is not a distinction that survives.
func TestCensusCrossesTheRigAndAbsentIsNotEmpty(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	arc := startArchive(t, g.relay.url())
	a, b := g.node(0), g.node(1)

	// A reports a census from the start; B has never sent one.
	a.mod.setCensus(rawCensus)

	view := waitCensus(t, arc, a.slot, "slot A's census to reach the archive",
		func(v archive.SlotView) bool { return v.SpeciesKnown })
	want := wantCensus()
	if len(view.Species) != len(want) {
		t.Fatalf("slot A reports %d species, want %d: %+v", len(view.Species), len(want), view.Species)
	}
	for i := range want {
		if view.Species[i] != want[i] {
			t.Fatalf("entry %d crossed the rig as %+v, want %+v — every hop copies bytes",
				i, view.Species[i], want[i])
		}
	}
	if view.SpeciesTruncated {
		t.Fatal("a four-entry census arrived flagged as truncated")
	}
	// The order is the SENDER'S, descending by bibites + eggs. Nothing
	// downstream re-sorts: the array's order is the census's own statement.
	for i := 1; i < len(view.Species); i++ {
		if view.Species[i].Count() > view.Species[i-1].Count() {
			t.Fatalf("the census was re-sorted somewhere: %+v", view.Species)
		}
	}
	// And the two whitespace twins are still two entries.
	if view.Species[0].GenericName == view.Species[1].GenericName {
		t.Fatal("two Species records that differ only in whitespace were merged")
	}

	// ABSENT. B has sent nothing, so its species are unknown while everything
	// else about it is exact.
	bv := waitCensus(t, arc, b.slot, "slot B's stats to reach the archive",
		func(v archive.SlotView) bool { return v.Population != nil })
	if bv.SpeciesKnown || bv.Species != nil {
		t.Fatalf("a mod that sent no census produced %v / %+v; absent is UNKNOWN",
			bv.SpeciesKnown, bv.Species)
	}

	// EMPTY. The same mod now reports a world with nothing alive in it, which
	// is a stronger statement than the silence before it.
	b.mod.setCensus(`"species":[]`)
	bv = waitCensus(t, arc, b.slot, "slot B's empty census to reach the archive",
		func(v archive.SlotView) bool { return v.SpeciesKnown })
	if len(bv.Species) != 0 {
		t.Fatalf("an empty census arrived with %d entries", len(bv.Species))
	}

	// The distinction has to survive the JSON, or the page cannot render it.
	body := statusJSON(t, arc)
	aj, bj := slotJSON(t, body, a.slot), slotJSON(t, body, b.slot)
	if known, _ := bj["speciesKnown"].(bool); !known {
		t.Fatalf("the empty census reads as unknown over HTTP: %v", bj["speciesKnown"])
	}
	if _, ok := aj["species"]; !ok {
		t.Fatal("the populated census did not reach /api/status")
	}
	// Attacker-choosable text (contract-b-m4.md §13 item 7) leaves the server
	// with < > & escaped by encoding/json's own HTML escaping. That is NOT the
	// renderer's defence — the browser turns them back into < and & the moment
	// it parses — but it is one fewer way for a careless reader to fail, and it
	// is worth keeping deliberately rather than by accident.
	if strings.ContainsAny(body, "<>") {
		t.Fatalf("/api/status emitted a raw angle bracket; a census name reaches "+
			"a renderer through this response:\n%s", body)
	}
	// And it is still the same name once a client parses it: the escaping is
	// transport, not repair (contract-a.md §17, A36).
	hostile := aj["species"].([]any)[3].(map[string]any)
	if hostile["genericName"] != "Cyanëa<&>" {
		t.Fatalf("the parsed name is %q", hostile["genericName"])
	}

	// ABSENT AGAIN. A mod that stops reporting goes back to unknown rather than
	// leaving its last census standing beside a fresh population: one block,
	// one timestamp, one truth (contract-b-m4.md §16, B11).
	a.mod.setCensus("")
	waitCensus(t, arc, a.slot, "slot A to fall back to unknown species",
		func(v archive.SlotView) bool { return !v.SpeciesKnown })
}

// TestMalformedCensusNeverCostsTheSession is §5.2's strip table where it
// actually matters: on a live socket, against a running sidecar, with no NACK
// channel to answer on. §9.3's default for a bad `data` field is close 4003, and
// applying it here would let a display field kill a live session.
//
// Each shape below is sent on a HEARTBEAT the mod keeps sending, and after every
// one of them the session is still up, the population is still exact, and a
// migration still crosses.
func TestMalformedCensusNeverCostsTheSession(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	arc := startArchive(t, g.relay.url())
	a, b := g.node(0), g.node(1)

	// Row 2, the field strip: `species` is not an array. The whole field goes
	// and the slot's species read as unknown — which is exactly the state a mod
	// that never implemented §17 leaves it in, and the page cannot tell the two
	// apart, by design.
	a.mod.setCensus(`"species":41,"truncated":true`)
	v := waitCensus(t, arc, a.slot, "the field-stripped census to reach the archive",
		func(v archive.SlotView) bool { return v.Population != nil })
	if v.SpeciesKnown {
		t.Fatalf("a `species` that is not an array left a census behind: %+v", v.Species)
	}
	if v.SpeciesTruncated {
		t.Fatal("truncated survived the field that gave it meaning")
	}

	// Row 1, the entry strip: the bad entry goes, the good ones stay, and
	// truncated is set even though the sender said nothing.
	a.mod.setCensus(`"species":[` +
		`{"genericName":"Good","specificName":"one","bibites":5,"eggs":0},` +
		`41,` +
		`{"genericName":"","specificName":"empty","bibites":1,"eggs":0},` +
		`{"genericName":"Good","specificName":"two","bibites":3,"eggs":1}]`)
	v = waitCensus(t, arc, a.slot, "the entry-stripped census to reach the archive",
		func(v archive.SlotView) bool { return v.SpeciesKnown })
	if len(v.Species) != 2 {
		t.Fatalf("kept %d entries, want the 2 good ones: %+v", len(v.Species), v.Species)
	}
	if !v.SpeciesTruncated {
		t.Fatal("the sidecar stripped two entries and did not set truncated")
	}

	// The session survived all of it, and so did everything the heartbeat is
	// actually for.
	if a.mod.isClosed() {
		t.Fatal("a malformed census closed the Contract A session")
	}
	if v.Population == nil {
		t.Fatal("a world whose census was stripped stopped reporting its population")
	}
	id := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 15*time.Second, "a migration to cross after a malformed census", func() bool {
		return b.world.spawnCount(id) == 1
	})
}

// TestOverlongCensusIsTrimmedNotRefused is row 3, end to end. The cap is a WIRE
// BOUND: it is what keeps a stats block, and the six-slot PEER_STATUS that
// republishes six of them, bounded. A sender that ignores it is a mod defect,
// and the answer is a trim and a log line, never a close.
//
// The kept entries are the FIRST 32 because the sender sorted descending, which
// is the one place that obligation is load-bearing rather than cosmetic.
func TestOverlongCensusIsTrimmedNotRefused(t *testing.T) {
	g := newGrid(t, 1, gridOptions{layout: layoutRow(1), skipEdgeCheck: true})
	arc := startArchive(t, g.relay.url())
	a := g.node(0)

	var b strings.Builder
	b.WriteString(`"species":[`)
	const sent = wire.SpeciesCensusMax + 9
	for i := 0; i < sent; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"genericName":"Sp","specificName":"n` + itoa(i) +
			`","bibites":` + itoa(sent-i) + `,"eggs":0}`)
	}
	b.WriteString(`]`)
	a.mod.setCensus(b.String())

	v := waitCensus(t, arc, a.slot, "the trimmed census to reach the archive",
		func(v archive.SlotView) bool { return v.SpeciesKnown })
	if len(v.Species) != wire.SpeciesCensusMax {
		t.Fatalf("the archive holds %d entries, want the cap of %d",
			len(v.Species), wire.SpeciesCensusMax)
	}
	if !v.SpeciesTruncated {
		t.Fatal("a trimmed census did not arrive flagged truncated")
	}
	if v.Species[0].SpecificName != "n0" ||
		v.Species[wire.SpeciesCensusMax-1].SpecificName != "n"+itoa(wire.SpeciesCensusMax-1) {
		t.Fatalf("the wrong end of the census was kept: %s … %s",
			v.Species[0].SpecificName, v.Species[wire.SpeciesCensusMax-1].SpecificName)
	}
	if a.mod.isClosed() {
		t.Fatal("an over-long census closed the session")
	}
}

// TestSidecarNeverAuthorsACensus pins contract-b-m4.md §16 B11's first rule:
// COPY, NEVER AUTHOR. A sidecar with a journal full of migrations, a genome
// cache and a stack of MIGRATION_PAYLOAD.species blocks still reports NO census
// when its mod has not sent one — because those describe migrants and a census
// describes a population, and a plausible-looking wrong number is the one thing
// this design is arranged to make impossible.
func TestSidecarNeverAuthorsACensus(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)

	// Push a named organism across, so the destination sidecar holds a species
	// name in its journal and its ledger path.
	id := a.mod.migrateOutSpecies(testEntityID, contracta.EdgeE, 0.5, awkwardSpecies())
	waitFor(t, 15*time.Second, "the named migration to arrive", func() bool {
		return b.world.spawnCount(id) == 1
	})
	if got := speciesOf(t, b.side, id); got == nil {
		t.Fatal("the destination journal did not hold the migration's species block")
	}

	if st := b.side.Stats(); st.Species != nil {
		t.Fatalf("the sidecar built a census out of what crossed into it: %+v", st.Species.Entries)
	}
	if st := a.side.Stats(); st.Species != nil {
		t.Fatalf("the sidecar built a census out of what left it: %+v", st.Species.Entries)
	}
}

// statusJSON reads /api/status off the archive's own HTTP listener, which is on
// an ephemeral 127.0.0.1 port the rig owns.
func statusJSON(t *testing.T, arc *archive.Archive) string {
	t.Helper()
	resp, err := http.Get("http://" + arc.HTTPAddr() + "/api/status")
	if err != nil {
		t.Fatalf("GET /api/status: %v", err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}

func slotJSON(t *testing.T, body string, slot int) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("/api/status is not valid JSON: %v", err)
	}
	for _, raw := range doc["slots"].([]any) {
		s := raw.(map[string]any)
		if int(s["slot"].(float64)) == slot {
			return s
		}
	}
	t.Fatalf("/api/status has no slot %d", slot)
	return nil
}
