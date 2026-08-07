package wire

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSpeciesShapeRules is contract-a.md §5.3's field table and §16 A30's rule
// table, one row at a time. Every malformed shape resolves the SAME way — the
// block is stripped and a reason is returned — because no shape of a label may
// ever refuse an organism.
func TestSpeciesShapeRules(t *testing.T) {
	sixtyFour := strings.Repeat("a", MaxSpeciesNameBytes)
	// Sixteen four-byte runes: 64 UTF-8 BYTES and only 16 characters. The limit
	// is bytes, so this one fits exactly and the same string plus one rune does
	// not.
	sixtyFourBytesOfRunes := strings.Repeat("𝔘", 16)

	cases := []struct {
		name string
		raw  string
		// want is the block that may be carried; "" in wantStripped means the
		// block survives.
		want         *Species
		wantStripped string
	}{{
		name: "the contract's own example",
		raw: `{"genericName":"Cyanea","specificName":"velox",` +
			`"parentGenericName":"Cyanea","parentSpecificName":"prima"}`,
		want: &Species{GenericName: "Cyanea", SpecificName: "velox",
			ParentGenericName: "Cyanea", ParentSpecificName: "prima"},
	}, {
		name: "a root species travels with no parent pair",
		raw:  `{"genericName":"Cyanea","specificName":"velox"}`,
		want: &Species{GenericName: "Cyanea", SpecificName: "velox"},
	}, {
		name: "exactly 64 ASCII bytes is inside the limit",
		raw:  `{"genericName":"` + sixtyFour + `","specificName":"velox"}`,
		want: &Species{GenericName: sixtyFour, SpecificName: "velox"},
	}, {
		name: "the limit counts UTF-8 BYTES, not runes",
		raw:  `{"genericName":"` + sixtyFourBytesOfRunes + `","specificName":"velox"}`,
		want: &Species{GenericName: sixtyFourBytesOfRunes, SpecificName: "velox"},
	}, {
		name:         "65 bytes is over it",
		raw:          `{"genericName":"` + sixtyFour + `b","specificName":"velox"}`,
		wantStripped: "over the 64 limit",
	}, {
		name:         "a missing genericName",
		raw:          `{"specificName":"velox"}`,
		wantStripped: "species.genericName is absent",
	}, {
		name:         "a missing specificName",
		raw:          `{"genericName":"Cyanea"}`,
		wantStripped: "species.specificName is absent",
	}, {
		name:         "a null half is an absent half",
		raw:          `{"genericName":"Cyanea","specificName":null}`,
		wantStripped: "species.specificName is absent",
	}, {
		name:         "an empty half",
		raw:          `{"genericName":"","specificName":"velox"}`,
		wantStripped: "species.genericName is empty",
	}, {
		name:         "a non-string half",
		raw:          `{"genericName":41,"specificName":"velox"}`,
		wantStripped: "species is not an object of four string fields",
	}, {
		name:         "a species that is not an object at all",
		raw:          `"Cyanea velox"`,
		wantStripped: "species is not an object of four string fields",
	}, {
		name:         "a lone parent genus",
		raw:          `{"genericName":"Cyanea","specificName":"velox","parentGenericName":"Cyanea"}`,
		wantStripped: "the parent pair is all-or-nothing",
	}, {
		name: "a lone parent specific",
		raw: `{"genericName":"Cyanea","specificName":"velox",` +
			`"parentSpecificName":"prima"}`,
		wantStripped: "the parent pair is all-or-nothing",
	}, {
		name: "a present but empty parent half",
		raw: `{"genericName":"Cyanea","specificName":"velox",` +
			`"parentGenericName":"Cyanea","parentSpecificName":""}`,
		wantStripped: "the parent pair is all-or-nothing",
	}, {
		name: "an over-long parent half",
		raw: `{"genericName":"Cyanea","specificName":"velox",` +
			`"parentGenericName":"` + sixtyFour + `b","parentSpecificName":"prima"}`,
		wantStripped: "species.parentGenericName is 65 UTF-8 bytes",
	}, {
		name:         "leading whitespace founds a second species that reads the same",
		raw:          `{"genericName":" Cyanea","specificName":"velox"}`,
		wantStripped: "carries leading or trailing whitespace",
	}, {
		name:         "trailing whitespace does too",
		raw:          `{"genericName":"Cyanea","specificName":"velox\n"}`,
		wantStripped: "carries leading or trailing whitespace",
	}, {
		name: "an unknown field inside the block is IGNORED, not fatal",
		raw: `{"genericName":"Cyanea","specificName":"velox",` +
			`"grandparentGenericName":"Cyanea"}`,
		want: &Species{GenericName: "Cyanea", SpecificName: "velox"},
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got *Species
			// A pointer field is how both wires carry it, and unmarshalling into
			// one is the exact path a frame takes.
			if err := json.Unmarshal([]byte(`{"species":`+c.raw+`}`), &struct {
				Species **Species `json:"species"`
			}{Species: &got}); err != nil {
				t.Fatalf("the enclosing message failed to decode: %v — a species block "+
					"MUST NOT be able to do that (§16, A30)", err)
			}
			carried, stripped := CarrySpecies(got)
			if c.wantStripped == "" {
				if stripped != "" {
					t.Fatalf("block was stripped for %q, want it carried", stripped)
				}
				if carried == nil || *carried != *c.want {
					t.Fatalf("carried %+v, want %+v", carried, c.want)
				}
				return
			}
			if stripped == "" {
				t.Fatalf("block was carried as %+v, want it stripped for %q", carried, c.wantStripped)
			}
			if carried != nil {
				t.Fatalf("a stripped block still produced %+v; the strip must be WHOLE, "+
					"never partial (§16, A30)", carried)
			}
			if !strings.Contains(stripped, c.wantStripped) {
				t.Fatalf("strip reason %q does not name %q", stripped, c.wantStripped)
			}
		})
	}
}

// TestAbsentSpeciesIsValid pins the rule three different states depend on: an
// absent block is CONFORMANT, not an error. An organism with a null species
// record, a mod that does not implement §16, and a block a schema check already
// stripped all produce it, and an importer cannot tell them apart.
func TestAbsentSpeciesIsValid(t *testing.T) {
	for _, raw := range []string{`{}`, `{"species":null}`} {
		var msg struct {
			Species *Species `json:"species"`
		}
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if msg.Species != nil {
			t.Fatalf("%s decoded to %+v, want an absent block", raw, msg.Species)
		}
		carried, stripped := CarrySpecies(msg.Species)
		if carried != nil || stripped != "" {
			t.Fatalf("%s: absent became (%+v, %q), want (nil, \"\")", raw, carried, stripped)
		}
	}
	if got := SpeciesName(nil); got != "" {
		t.Fatalf("SpeciesName(nil) = %q, want the empty string", got)
	}
}

// TestSpeciesNamesRoundTripByteForByte is the whole point of the block: the two
// names reach the far side as the SAME BYTES the origin world holds. Nothing
// trims, folds, normalizes or re-escapes them, and the JSON escaping a Go
// encoder chooses is not allowed to change the value it decodes back to.
func TestSpeciesNamesRoundTripByteForByte(t *testing.T) {
	want := &Species{
		// Non-ASCII, a quote, and the three characters Go's JSON encoder escapes
		// to < > & on the way out.
		GenericName:        "Cyanëa<&>",
		SpecificName:       `velox"íssima`,
		ParentGenericName:  "Cyanëa",
		ParentSpecificName: "prīma",
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Species
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != *want {
		t.Fatalf("round trip changed the names:\n got %+v\nwant %+v", got, *want)
	}
	if reason := got.MalformedReason(); reason != "" {
		t.Fatalf("a legal name was called malformed: %s", reason)
	}
	if name := SpeciesName(&got); name != want.GenericName+" "+want.SpecificName {
		t.Fatalf("SpeciesName = %q; §5.7 assembles the two halves with exactly one U+0020", name)
	}
}

// TestCarriedSpeciesIsACopy pins a durability property, not a style one. The
// decoded frame is transient and the journal, the ledger and the next hop all
// outlive it, so the block that leaves CarrySpecies must not alias the caller's.
func TestCarriedSpeciesIsACopy(t *testing.T) {
	src := &Species{GenericName: "Cyanea", SpecificName: "velox"}
	carried, stripped := CarrySpecies(src)
	if stripped != "" {
		t.Fatalf("unexpected strip: %s", stripped)
	}
	if carried == src {
		t.Fatal("CarrySpecies returned the caller's own block")
	}
	src.GenericName = "Rewritten"
	if carried.GenericName != "Cyanea" {
		t.Fatal("the carried block followed a later edit of its source")
	}
}

// TestSpeciesMarshalsWithoutItsDecodeComplaint keeps the internal bookkeeping
// internal: nothing about WHY a block was malformed may ever reach a wire, a
// journal or a ledger.
func TestSpeciesMarshalsWithoutItsDecodeComplaint(t *testing.T) {
	var s Species
	if err := s.UnmarshalJSON([]byte(`{"genericName":41}`)); err != nil {
		t.Fatalf("UnmarshalJSON returned an error: %v — it must never do that", err)
	}
	if s.MalformedReason() == "" {
		t.Fatal("a non-string half was accepted")
	}
	b, err := json.Marshal(&s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "not an object") || strings.Contains(string(b), "decodeErr") {
		t.Fatalf("the decode complaint leaked onto the wire: %s", b)
	}
}
