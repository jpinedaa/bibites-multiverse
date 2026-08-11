package sidecar

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"multiverse/internal/archive"
	"multiverse/internal/contracta"
	"multiverse/internal/wire"
)

// rawSettings is the settings row the rig sends: contract-a.md §19 A42's five
// OPTIONAL fields, with everything about them that has to survive intact.
//
//   - migrationExclude carries TWO entries, in the mod's own configured order.
//     Nothing anywhere may sort, deduplicate or repair one.
//   - the second entry is the untrusted-text case: a species name is chosen by
//     whoever configured that world, and an exclusion entry now reaches the
//     page on the same unauthenticated block a census name does
//     (contract-b-m4.md §13 item 7, §19 B18).
//   - saveMinutes is 0, which is a READING and not an absence: that world's
//     save timer is off, and it is the explanation for a world that never
//     reports a lastSave.
//   - worldWrapping is false, which is the loud reading: a world that is not
//     containing its own organisms (D10).
const rawSettings = `"migrationExclude":["Basic bibite","Cyanëa<&> velox\"issima"],` +
	`"saveMinutes":0,"saveKeep":6,"saveOnQuit":true,"worldWrapping":false`

func waitSettings(t *testing.T, arc *archive.Archive, slot int, what string,
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

// TestWorldSettingsCrossTheRig is the whole of contract-a.md §19 A42 and
// contract-b-m4.md §19 B18 in one test: mod -> sidecar -> relay -> archive ->
// the operator view the settings tab renders, with the seven fields carried
// verbatim and every absence kept as an absence.
//
// The three states §19 holds apart at every hop:
//
//	PRESENT — the five settings arrive byte for byte, in the mod's own order,
//	          and the two readings that are not gaps (saveMinutes 0,
//	          worldWrapping false) survive as readings.
//	ABSENT  — a mod that sends no settings leaves them UNKNOWN. Not zero, not
//	          empty, and above all NOT the value the game ships with.
//	EMPTY   — the same mod sending `[]` for migrationExclude is a different and
//	          stronger fact: the exclusion policy is OFF.
//
// The two version strings ride with them, and contractAVersion is asserted to
// be THE MOD'S, not this sidecar's build — those differ on exactly the rig the
// field exists to describe.
func TestWorldSettingsCrossTheRig(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	arc := startArchive(t, g.relay)
	a, b := g.node(0), g.node(1)

	// A reports settings from the start; B has never sent any.
	a.mod.setSettings(rawSettings)

	view := waitSettings(t, arc, a.slot, "slot A's settings to reach the archive",
		func(v archive.SlotView) bool { return v.MigrationExcludeKnown })

	want := []string{"Basic bibite", `Cyanëa<&> velox"issima`}
	if len(view.MigrationExclude) != len(want) {
		t.Fatalf("slot A publishes %d exclusions, want %d: %+v",
			len(view.MigrationExclude), len(want), view.MigrationExclude)
	}
	for i := range want {
		if view.MigrationExclude[i] != want[i] {
			t.Fatalf("exclusion %d crossed the rig as %q, want %q — every hop copies bytes "+
				"and nothing sorts, deduplicates or repairs one", i, view.MigrationExclude[i], want[i])
		}
	}
	// saveMinutes 0 IS A READING. A reader that folds it into absence loses the
	// one fact that explains a world with no save receipt.
	if view.SaveMinutes == nil {
		t.Fatal("saveMinutes 0 arrived as ABSENT; the timer being off is a reading, not a gap")
	}
	if *view.SaveMinutes != 0 {
		t.Fatalf("saveMinutes = %v, want 0", *view.SaveMinutes)
	}
	if view.SaveKeep == nil || *view.SaveKeep != 6 {
		t.Fatalf("saveKeep = %v, want 6", view.SaveKeep)
	}
	if view.SaveOnQuit == nil || !*view.SaveOnQuit {
		t.Fatalf("saveOnQuit = %v, want true", view.SaveOnQuit)
	}
	// worldWrapping false is the loud reading, and false must never be dropped
	// by an omitempty that treats it as a zero value.
	if view.WorldWrapping == nil {
		t.Fatal("worldWrapping false arrived as ABSENT; it names a world that is not " +
			"containing its own organisms and it must survive the wire")
	}
	if *view.WorldWrapping {
		t.Fatalf("worldWrapping = %v, want false", *view.WorldWrapping)
	}
	// The two version strings the sidecar has always held and never published.
	if view.ModVersion != "0.2.0" {
		t.Fatalf("modVersion = %q, want the mod's own %q", view.ModVersion, "0.2.0")
	}
	// THE SESSION'S, NOT THIS BUILD'S. They are the same string on this rig
	// because the fake mod speaks the current wire, so the test asserts the
	// mechanism instead: the sidecar publishes what arrived on the frame.
	if view.ContractAVersion != wire.ProtocolA {
		t.Fatalf("contractAVersion = %q, want the protocol the mod's frames carried (%q)",
			view.ContractAVersion, wire.ProtocolA)
	}

	// ABSENT. B has sent nothing, so its settings are unknown while everything
	// else about it is exact.
	bv := waitSettings(t, arc, b.slot, "slot B's stats to reach the archive",
		func(v archive.SlotView) bool { return v.Population != nil })
	if bv.MigrationExcludeKnown || bv.MigrationExclude != nil {
		t.Fatalf("a mod that sent no exclusion list produced %v / %+v; absent is UNKNOWN",
			bv.MigrationExcludeKnown, bv.MigrationExclude)
	}
	if bv.SaveMinutes != nil || bv.SaveKeep != nil || bv.SaveOnQuit != nil || bv.WorldWrapping != nil {
		t.Fatalf("a mod that sent no settings produced saveMinutes=%v keep=%v quit=%v wrap=%v; "+
			"THERE IS NO DEFAULT ANYWHERE IN THIS CHAIN TO PRODUCE ONE FROM",
			bv.SaveMinutes, bv.SaveKeep, bv.SaveOnQuit, bv.WorldWrapping)
	}
	// It still publishes what it has always had: the version strings do not
	// depend on §19 at all, and they are what makes the unknown above
	// self-explaining.
	if bv.ModVersion == "" || bv.ContractAVersion == "" {
		t.Fatalf("a slot with no settings lost its version strings too: mod=%q contract-a=%q",
			bv.ModVersion, bv.ContractAVersion)
	}

	// EMPTY. The same mod now reports a world whose exclusion policy is off,
	// which is a stronger statement than the silence before it.
	b.mod.setSettings(`"migrationExclude":[]`)
	bv = waitSettings(t, arc, b.slot, "slot B's empty exclusion list to reach the archive",
		func(v archive.SlotView) bool { return v.MigrationExcludeKnown })
	if len(bv.MigrationExclude) != 0 {
		t.Fatalf("an empty exclusion list arrived with %d entries", len(bv.MigrationExclude))
	}
	// And its four save/wrap settings are still absent: sending one settings
	// field does not conjure the others.
	if bv.SaveMinutes != nil {
		t.Fatalf("saveMinutes appeared from nowhere: %v", *bv.SaveMinutes)
	}

	// The absent/empty distinction has to survive the JSON, or the page cannot
	// render it: a JS client cannot tell an omitted array from an empty one.
	body := statusJSON(t, arc)
	aj, bj := slotJSON(t, body, a.slot), slotJSON(t, body, b.slot)
	if known, _ := bj["migrationExcludeKnown"].(bool); !known {
		t.Fatalf("the empty exclusion list reads as unknown over HTTP: %v",
			bj["migrationExcludeKnown"])
	}
	if _, ok := bj["migrationExclude"]; ok {
		t.Fatal("an empty exclusion list emitted an array; the KNOWN flag is what carries " +
			"the distinction, and an empty array beside it is redundant state to disagree with")
	}
	if _, ok := aj["migrationExclude"]; !ok {
		t.Fatal("the populated exclusion list did not reach /api/status")
	}
	// saveMinutes 0 must survive omitempty. A pointer is what makes that
	// possible and this is the assertion that keeps it a pointer.
	if v, ok := aj["saveMinutes"]; !ok || v.(float64) != 0 {
		t.Fatalf("saveMinutes 0 did not survive to the JSON: %v (present %v)", aj["saveMinutes"], ok)
	}
	if v, ok := aj["worldWrapping"]; !ok || v.(bool) {
		t.Fatalf("worldWrapping false did not survive to the JSON: %v (present %v)",
			aj["worldWrapping"], ok)
	}
	// The exclusion name is the same untrusted text a census name is, and it
	// leaves the server with < > & escaped by encoding/json exactly as a census
	// name does. That is transport, not repair: it parses back identical.
	if strings.ContainsAny(body, "<>") {
		t.Fatalf("/api/status emitted a raw angle bracket; an exclusion entry reaches "+
			"a renderer through this response:\n%s", body)
	}
	list := aj["migrationExclude"].([]any)
	if list[1] != `Cyanëa<&> velox"issima` {
		t.Fatalf("the parsed exclusion entry is %q", list[1])
	}
}

// TestMalformedSettingsNeverCostTheSession is §19 A42's strip rule where it
// actually matters: on a live socket, on the HANDSHAKE, with no NACK channel to
// answer on. §9.3's default for a bad `data` field is close 4003, and applying
// it here would let an observability row kill a session that is carrying
// organisms — at reconnect, which is the worst possible moment.
//
// Each shape below rides a CONFIG_UPDATE the mod re-sends, and after every one
// of them the session is still up, the good settings are still exact, and a
// migration still crosses.
func TestMalformedSettingsNeverCostTheSession(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	arc := startArchive(t, g.relay)
	a, b := g.node(0), g.node(1)

	// The FIELD strip: migrationExclude is not an array. The whole field goes
	// and the slot's exclusions read as unknown — the same state a mod that
	// never implemented §19 leaves it in, and the page cannot tell the two
	// apart, by design. Everything else on the row survives.
	a.mod.setSettings(`"migrationExclude":41,"saveMinutes":10,"worldWrapping":true`)
	v := waitSettings(t, arc, a.slot, "the field-stripped settings to reach the archive",
		func(v archive.SlotView) bool { return v.SaveMinutes != nil })
	if v.MigrationExcludeKnown {
		t.Fatalf("a migrationExclude that is not an array left a list behind: %+v",
			v.MigrationExclude)
	}
	if *v.SaveMinutes != 10 {
		t.Fatalf("a bad exclusion list took saveMinutes with it: %v", *v.SaveMinutes)
	}
	if v.WorldWrapping == nil || !*v.WorldWrapping {
		t.Fatalf("a bad exclusion list took worldWrapping with it: %v", v.WorldWrapping)
	}

	// The ENTRY strip: the bad entries go, the good one stays. An entry that is
	// not a string, one that is empty, one that is only whitespace — which the
	// policy could never match — and one over the 129-byte bound.
	a.mod.setSettings(`"migrationExclude":["Keeper one",41,"","   ","` +
		strings.Repeat("x", 130) + `"],"saveMinutes":"soon","saveKeep":2.5,` +
		`"saveOnQuit":"yes","worldWrapping":[]`)
	v = waitSettings(t, arc, a.slot, "the entry-stripped settings to reach the archive",
		func(v archive.SlotView) bool { return v.MigrationExcludeKnown })
	if len(v.MigrationExclude) != 1 || v.MigrationExclude[0] != "Keeper one" {
		t.Fatalf("kept %+v, want only the one good entry", v.MigrationExclude)
	}
	// A scalar of the wrong shape costs THAT SCALAR and nothing else, and the
	// result is unknown rather than a substituted default.
	if v.SaveMinutes != nil {
		t.Fatalf("a saveMinutes of \"soon\" produced %v; a bad value is unknown, never a default",
			*v.SaveMinutes)
	}
	if v.SaveKeep != nil {
		t.Fatalf("a fractional saveKeep produced %v", *v.SaveKeep)
	}
	if v.SaveOnQuit != nil || v.WorldWrapping != nil {
		t.Fatalf("a non-boolean flag produced quit=%v wrap=%v", v.SaveOnQuit, v.WorldWrapping)
	}

	// The session survived all of it, and so did everything the handshake is
	// actually for.
	if a.mod.isClosed() {
		t.Fatal("a malformed settings row closed the Contract A session — this is the " +
			"handshake, and closing on an observability field costs the whole session")
	}
	// The heartbeat is a separate frame on the same session, so this is a wait
	// rather than an assertion on the view the handshake produced: what is being
	// checked is that the session KEEPS reporting, not that it had already.
	waitSettings(t, arc, a.slot, "the world to keep reporting its population",
		func(v archive.SlotView) bool { return v.Population != nil })
	id := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 15*time.Second, "a migration to cross after a malformed settings row", func() bool {
		return b.world.spawnCount(id) == 1
	})
}

// TestSettingsAreNeverAnInputToAnything is A42's "not an input to anything" and
// A43's read-only boundary, asserted as behaviour rather than as a comment.
//
// The exclusion list names the species this mod is exporting. If any party
// downstream of the mod ever treated the published list as enforceable — a
// sidecar refusing a MIGRATE_OUT, a relay refusing to forward one — the
// migration below would not arrive. It does, because the capture test lives in
// the origin mod at the origin's capture band, and publishing a policy is not
// the same as delegating it (contract-a.md §18 A39, §19 A42).
func TestSettingsAreNeverAnInputToAnything(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	arc := startArchive(t, g.relay)
	a, b := g.node(0), g.node(1)

	a.mod.setSettings(`"migrationExclude":["Izus copedylanus"]`)
	waitSettings(t, arc, a.slot, "the exclusion list to reach the archive",
		func(v archive.SlotView) bool { return v.MigrationExcludeKnown })

	// An organism OF THE EXCLUDED SPECIES, exported by the very world that
	// published the exclusion. Nothing between here and the destination may
	// refuse, filter or pre-empt it.
	id := a.mod.migrateOutSpecies(testEntityID, contracta.EdgeE, 0.5,
		&wire.Species{GenericName: "Izus", SpecificName: "copedylanus"})
	waitFor(t, 15*time.Second, "an excluded species to cross anyway", func() bool {
		return b.world.spawnCount(id) == 1
	})
}

// TestSettingsSurviveTheRelayAsBytes is B18's third round of §16 B11's SHOULD:
// a relay that re-encodes a stats block from a typed model drops a stat a newer
// sidecar sends. §18's three pacing settings were the field after the census;
// §19's seven are the next.
//
// It is asserted at the ARCHIVE, which sits on the far side of the relay, so
// the seven fields have made the whole trip through a component that was never
// changed for them.
func TestSettingsSurviveTheRelayAsBytes(t *testing.T) {
	g := newGrid(t, 1, gridOptions{layout: layoutRow(1), skipEdgeCheck: true})
	arc := startArchive(t, g.relay)
	a := g.node(0)

	a.mod.setSettings(rawSettings)
	v := waitSettings(t, arc, a.slot, "the settings to cross the relay",
		func(v archive.SlotView) bool { return v.MigrationExcludeKnown })
	for name, ok := range map[string]bool{
		"modVersion":       v.ModVersion != "",
		"contractAVersion": v.ContractAVersion != "",
		"migrationExclude": len(v.MigrationExclude) == 2,
		"saveMinutes":      v.SaveMinutes != nil,
		"saveKeep":         v.SaveKeep != nil,
		"saveOnQuit":       v.SaveOnQuit != nil,
		"worldWrapping":    v.WorldWrapping != nil,
	} {
		if !ok {
			t.Fatalf("%s did not survive the relay; a relay that re-encodes a stats block "+
				"from a typed model drops what it does not understand (§16, B11)", name)
		}
	}
}

// TestPeerStatsCarryUnknownSettingsThrough is the same SHOULD one level down,
// on the type itself: a stats block from a NEWER peer, carrying a field this
// build has never heard of, must arrive on the far side with that field intact.
func TestPeerStatsCarryUnknownSettingsThrough(t *testing.T) {
	raw := []byte(`{"population":7,"saveMinutes":0,"migrationExclude":["Basic bibite"],` +
		`"someFutureSetting":{"x":1}}`)
	var stats struct {
		Population       *int              `json:"population"`
		SaveMinutes      *float64          `json:"saveMinutes"`
		MigrationExclude *wire.ExcludeList `json:"migrationExclude"`
	}
	if err := json.Unmarshal(raw, &stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.SaveMinutes == nil || *stats.SaveMinutes != 0 {
		t.Fatalf("saveMinutes = %v", stats.SaveMinutes)
	}
	if stats.MigrationExclude.Len() != 1 {
		t.Fatalf("migrationExclude = %+v", stats.MigrationExclude)
	}
	// And the wrapper's own absent/empty rule, which is the reason it is a type.
	var absent *wire.ExcludeList
	if absent.Len() != 0 || absent.Has("anything") {
		t.Fatal("a nil exclusion list is not behaving as absent")
	}
	present := &wire.ExcludeList{Names: []string{}}
	if present.Has("Basic bibite") {
		t.Fatal("an empty exclusion list matched a name")
	}
	out, err := json.Marshal(present)
	if err != nil || string(out) != "[]" {
		t.Fatalf("a present empty list encoded as %q, %v; it must never encode as null, "+
			"or the next hop reads 'policy off' as 'we do not know'", out, err)
	}
}
