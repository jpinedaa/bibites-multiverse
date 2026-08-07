package wire

import (
	"encoding/json"
	"strings"
	"testing"
)

// carry decodes one `{"species":…,"truncated":…}` object the way a HEARTBEAT
// decode does, then applies §5.2's strip table to it. It is the whole path a
// census takes into the system, in one helper, so every row below is exercised
// through the real decoder rather than through a hand-built Go value.
func carry(t *testing.T, body string) (*Census, bool, []string) {
	t.Helper()
	var frame struct {
		Species   *Census       `json:"species"`
		Truncated TruncatedFlag `json:"truncated"`
	}
	// THE DECODE ITSELF MUST NOT FAIL. HEARTBEAT has no NACK channel, so a
	// decode error here is close 4003 — a display field killing a live session,
	// which is exactly what §17 A35 exists to forbid.
	if err := json.Unmarshal([]byte(body), &frame); err != nil {
		t.Fatalf("decoding %s failed with %v; a malformed census MUST NOT fail the frame", body, err)
	}
	return CarryCensus(frame.Species, bool(frame.Truncated))
}

func names(c *Census) []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.Entries))
	for _, e := range c.Entries {
		out = append(out, e.GenericName+"|"+e.SpecificName)
	}
	return out
}

// TestCensusStripTableRow1Entry is the FIRST row of contract-a.md §5.2's strip
// table, entry by entry: one broken entry costs THAT ENTRY, the rest of the
// census survives, `truncated` is set, and the heartbeat is otherwise processed
// normally. Every case here would be a close 4003 under §9.3's default, which
// is precisely why the row had to be written down.
func TestCensusStripTableRow1Entry(t *testing.T) {
	good := `{"genericName":"Keep","specificName":"me","bibites":9,"eggs":1}`
	long := strings.Repeat("x", MaxCensusNameBytes+1)
	cases := []struct{ name, entry, why string }{
		{"not an object", `41`, "shape"},
		{"an array", `["Cyanea","velox",1,0]`, "shape"},
		{"genericName absent", `{"specificName":"velox","bibites":1,"eggs":0}`, "genericName"},
		{"specificName absent", `{"genericName":"Cyanea","bibites":1,"eggs":0}`, "specificName"},
		{"genericName not a string", `{"genericName":7,"specificName":"velox","bibites":1,"eggs":0}`, "shape"},
		{"genericName empty", `{"genericName":"","specificName":"velox","bibites":1,"eggs":0}`, "empty"},
		{"specificName empty", `{"genericName":"Cyanea","specificName":"","bibites":1,"eggs":0}`, "empty"},
		{"genericName over 64 bytes", `{"genericName":"` + long + `","specificName":"velox","bibites":1,"eggs":0}`, "over the 64"},
		{"bibites absent", `{"genericName":"Cyanea","specificName":"velox","eggs":0}`, "bibites is absent"},
		{"eggs absent", `{"genericName":"Cyanea","specificName":"velox","bibites":1}`, "eggs is absent"},
		{"bibites not a number", `{"genericName":"Cyanea","specificName":"velox","bibites":"1","eggs":0}`, "shape"},
		{"bibites not an integer", `{"genericName":"Cyanea","specificName":"velox","bibites":1.5,"eggs":0}`, "shape"},
		{"bibites negative", `{"genericName":"Cyanea","specificName":"velox","bibites":-1,"eggs":0}`, "negative"},
		{"eggs negative", `{"genericName":"Cyanea","specificName":"velox","bibites":1,"eggs":-2}`, "negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, truncated, why := carry(t, `{"species":[`+good+`,`+tc.entry+`,`+good+`]}`)
			if got == nil {
				t.Fatal("a bad ENTRY cost the whole FIELD; row 1 strips the entry and keeps the census")
			}
			if n := names(got); len(n) != 2 || n[0] != "Keep|me" || n[1] != "Keep|me" {
				t.Fatalf("the surviving entries are %v, want both copies of the good one", n)
			}
			if !truncated {
				t.Fatal("the sidecar stripped an entry and did not set truncated")
			}
			if len(why) != 1 {
				t.Fatalf("stripping one entry logged %d lines, want exactly one: %v", len(why), why)
			}
			if !strings.Contains(why[0], tc.why) && tc.why != "shape" {
				t.Fatalf("the log line %q does not name the defect %q", why[0], tc.why)
			}
		})
	}
}

// TestCensusStripTableRow2Field is the SECOND row: `species` present and not an
// array costs the WHOLE FIELD, the heartbeat is processed without a census, and
// the flag that qualified it goes with it — because `truncated` is meaningless
// without the array and a receiver MUST ignore it when the array is absent.
//
// The last case is the row's other half: a `truncated` that is not a boolean is
// dropped ON ITS OWN and read as absent, leaving a perfectly good census alone.
func TestCensusStripTableRow2Field(t *testing.T) {
	for _, body := range []string{
		`{"species":41}`,
		`{"species":"Cyanea velox"}`,
		`{"species":{"Cyanea":"velox"}}`,
		`{"species":true}`,
		// truncated cannot rescue a field that is not an array.
		`{"species":41,"truncated":true}`,
	} {
		got, truncated, why := carry(t, body)
		if got != nil {
			t.Fatalf("%s left a census behind: %v", body, names(got))
		}
		if truncated {
			t.Fatalf("%s carried truncated through with no array to qualify", body)
		}
		if len(why) != 1 {
			t.Fatalf("%s logged %d lines, want exactly one: %v", body, len(why), why)
		}
	}

	// A truncated that is not a boolean is dropped on its own.
	one := `{"genericName":"Cyanea","specificName":"velox","bibites":3,"eggs":1}`
	got, truncated, why := carry(t, `{"species":[`+one+`],"truncated":"yes"}`)
	if got == nil || len(got.Entries) != 1 {
		t.Fatalf("a bad truncated cost the census: %v", names(got))
	}
	if truncated {
		t.Fatal("a non-boolean truncated was read as true; it is read as ABSENT")
	}
	if len(why) != 0 {
		t.Fatalf("dropping a non-boolean truncated logged %v; it costs nothing else", why)
	}
}

// TestCensusStripTableRow3Trim is the THIRD row: an over-long array is trimmed
// to speciesCensusMax and truncated is set. The kept entries are the FIRST 32,
// which is only correct because the sender sorted descending — the sort is a
// sender obligation and this is the code that depends on it.
func TestCensusStripTableRow3Trim(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"species":[`)
	for i := 0; i < SpeciesCensusMax+8; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		// Descending, as the sender is obliged to send them.
		b.WriteString(`{"genericName":"Sp","specificName":"` + string(rune('a'+i%26)) +
			`","bibites":` + itoa(1000-i) + `,"eggs":0}`)
	}
	b.WriteString(`]}`)

	got, truncated, why := carry(t, b.String())
	if got == nil {
		t.Fatal("an over-long array cost the whole field; row 3 trims it")
	}
	if len(got.Entries) != SpeciesCensusMax {
		t.Fatalf("kept %d entries, want the cap of %d", len(got.Entries), SpeciesCensusMax)
	}
	if got.Entries[0].Bibites != 1000 || got.Entries[SpeciesCensusMax-1].Bibites != 1000-(SpeciesCensusMax-1) {
		t.Fatalf("the wrong end was kept: first %d, last %d — the FIRST 32 are the largest",
			got.Entries[0].Bibites, got.Entries[SpeciesCensusMax-1].Bibites)
	}
	if !truncated {
		t.Fatal("trimming an over-long array did not set truncated")
	}
	if len(why) != 1 || !strings.Contains(why[0], "speciesCensusMax") {
		t.Fatalf("the trim logged %v; it is a mod defect and is logged as one", why)
	}
}

// TestCensusTruncatedIsMonotonic pins the one thing the flag means and the one
// way it moves: SET ON THE WAY, NEVER CLEARED. A sender that already truncated
// keeps its claim across a hop that had nothing to strip, and a hop that strips
// sets it over a sender that did not.
func TestCensusTruncatedIsMonotonic(t *testing.T) {
	clean := `{"genericName":"Cyanea","specificName":"velox","bibites":3,"eggs":1}`

	got, truncated, why := carry(t, `{"species":[`+clean+`],"truncated":true}`)
	if got == nil || len(got.Entries) != 1 {
		t.Fatal("a clean census did not survive")
	}
	if !truncated {
		t.Fatal("the sender's truncated was CLEARED by a hop with nothing to strip")
	}
	if len(why) != 0 {
		t.Fatalf("a clean census logged %v", why)
	}

	_, truncated, _ = carry(t, `{"species":[`+clean+`,41],"truncated":false}`)
	if !truncated {
		t.Fatal("a hop that stripped an entry left the sender's false in place")
	}
}

// TestCensusAbsentIsNotEmpty is the distinction the whole field is OPTIONAL
// for, held end to end. ABSENT IS UNKNOWN — an old mod, or one that does not
// implement §17. A PRESENT [] IS A STRONGER STATEMENT: a reporting mod, from a
// world with nothing alive in it. A Go type that flattened the two would make
// the page say "no species" about a world that never said anything.
func TestCensusAbsentIsNotEmpty(t *testing.T) {
	type frame struct {
		Species   *Census       `json:"species,omitempty"`
		Truncated TruncatedFlag `json:"truncated,omitempty"`
	}

	// Absent in, absent out, and no key on the way out either.
	var in frame
	if err := json.Unmarshal([]byte(`{}`), &in); err != nil {
		t.Fatal(err)
	}
	if in.Species != nil {
		t.Fatal("a frame with no species key decoded to a present census")
	}
	got, truncated, why := CarryCensus(in.Species, bool(in.Truncated))
	if got != nil || truncated || why != nil {
		t.Fatalf("an absent census produced %v / %v / %v", names(got), truncated, why)
	}
	out, err := json.Marshal(frame{Species: got})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{}` {
		t.Fatalf("an absent census re-encoded as %s, want {}", out)
	}

	// Present and empty in, present and empty out, and a `[]` on the way out —
	// never `null`, which the next hop would read back as absent.
	var empty frame
	if err := json.Unmarshal([]byte(`{"species":[]}`), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Species == nil {
		t.Fatal("a present [] decoded as absent; that is the fact this field exists to keep")
	}
	got, _, _ = CarryCensus(empty.Species, false)
	if got == nil {
		t.Fatal("a present [] was stripped into absence")
	}
	if len(got.Entries) != 0 {
		t.Fatalf("a present [] gained %d entries", len(got.Entries))
	}
	out, err = json.Marshal(frame{Species: got})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"species":[]}` {
		t.Fatalf("a present empty census re-encoded as %s, want {\"species\":[]}", out)
	}
}

// TestCensusNamesAreCarriedRaw is contract-a.md §17 A36, and it is the rule
// that separates this validator from species.go's. Edge whitespace, doubled
// internal whitespace and a half that is NOTHING BUT whitespace are all LEGAL
// here and travel untouched — they are what a faithful census carries, and the
// species they belong to were 27% of one measured world's living population.
//
// The second half of the test is the contrast: the SAME names are rejected by
// the migration lane's validator, on purpose, because a name there is a
// matching key and a name here is a label.
func TestCensusNamesAreCarriedRaw(t *testing.T) {
	body := `{"species":[` +
		`{"genericName":"Izus ","specificName":"copedylanus","bibites":96,"eggs":14},` +
		`{"genericName":"Izus","specificName":"copedylanus","bibites":8,"eggs":1},` +
		`{"genericName":"Banagellus","specificName":"polatus ","bibites":17,"eggs":3},` +
		`{"genericName":" ","specificName":"anonymus","bibites":2,"eggs":0},` +
		`{"genericName":"Cyanëa<&>","specificName":"velox  prima","bibites":1,"eggs":0}` +
		`]}`
	got, truncated, why := carry(t, body)
	if got == nil {
		t.Fatalf("a raw-name census was stripped: %v", why)
	}
	if truncated || len(why) != 0 {
		t.Fatalf("a legal raw-name census set truncated=%v and logged %v", truncated, why)
	}
	want := []string{
		"Izus |copedylanus",
		"Izus|copedylanus",
		"Banagellus|polatus ",
		" |anonymus",
		"Cyanëa<&>|velox  prima",
	}
	gotNames := names(got)
	if len(gotNames) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(gotNames), len(want), gotNames)
	}
	for i := range want {
		if gotNames[i] != want[i] {
			t.Fatalf("entry %d is %q, want %q — nothing here may repair a name", i, gotNames[i], want[i])
		}
	}
	// NOT DE-DUPLICATED: "Izus " and "Izus" are two Species records in that
	// world and the census reports two, which is also the cheapest way an
	// operator ever sees a pre-A34 whitespace twin.
	if gotNames[0] == gotNames[1] {
		t.Fatal("two whitespace twins were merged into one entry")
	}

	// The other lane, on the same strings: rejected, and that is correct.
	for _, half := range []string{"Izus ", "polatus ", " "} {
		s := &Species{GenericName: half, SpecificName: "x"}
		if s.MalformedReason() == "" {
			t.Fatalf("the MIGRATION validator accepted %q; §16 A34 requires it to reject one",
				half)
		}
	}
}

// TestCensusNameByteLimitIsBytes pins the limit as a UTF-8 BYTE count, which is
// what the C# side can agree on without agreeing on anything about language.
func TestCensusNameByteLimitIsBytes(t *testing.T) {
	// 16 four-byte runes: 16 characters, 64 bytes, and legal.
	at64 := strings.Repeat("𝔘", 16)
	if len(at64) != MaxCensusNameBytes {
		t.Fatalf("the fixture is %d bytes, not %d", len(at64), MaxCensusNameBytes)
	}
	got, truncated, _ := CarryCensus(&Census{Entries: []CensusEntry{
		{GenericName: at64, SpecificName: "ok", Bibites: 1, Eggs: 0},
	}}, false)
	if got == nil || len(got.Entries) != 1 || truncated {
		t.Fatal("a 64-byte name was stripped; the limit is inclusive")
	}

	got, truncated, why := CarryCensus(&Census{Entries: []CensusEntry{
		{GenericName: at64 + "x", SpecificName: "ok", Bibites: 1, Eggs: 0},
	}}, false)
	if got == nil || len(got.Entries) != 0 {
		t.Fatalf("a 65-byte name survived: %v", names(got))
	}
	if !truncated || len(why) != 1 {
		t.Fatalf("the over-long name set truncated=%v and logged %v", truncated, why)
	}
}

// TestCensusIsNeverAnError is the property the whole file is arranged around,
// stated once: NOTHING a peer can put in this field produces a Go error, at
// decode or at carry. Every other outcome is a strip and a log line.
func TestCensusIsNeverAnError(t *testing.T) {
	for _, body := range []string{
		`{"species":null}`,
		`{"species":[]}`,
		`{"species":[null]}`,
		`{"species":[[]]}`,
		`{"species":[{}]}`,
		`{"species":"nope","truncated":[1,2]}`,
		`{"species":[{"genericName":"a","specificName":"b","bibites":1e400,"eggs":0}]}`,
		`{"truncated":true}`,
	} {
		var frame struct {
			Species   *Census       `json:"species"`
			Truncated TruncatedFlag `json:"truncated"`
		}
		if err := json.Unmarshal([]byte(body), &frame); err != nil {
			t.Fatalf("%s failed to decode with %v; a census may never fail a heartbeat", body, err)
		}
		// And it must not panic or misbehave on the way through either.
		CarryCensus(frame.Species, bool(frame.Truncated))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
