package wire

// The OPTIONAL species CENSUS, shared by both wires: contract-a.md §5.2 and
// §17 (A35, A36, `HEARTBEAT.species` and its sibling `truncated`) and
// contract-b-m4.md §6.3.1 and §16 (B11, the same two fields inside the peer
// stats block).
//
// THIS IS NOT species.go's BLOCK, AND THE DIFFERENCE IS THE POINT.
// `MIGRATE_OUT.species` is a MATCHING KEY: one migrant, resolved against a
// foreign registry, normalized at the source by the exporting mod, and a stray
// space there is a silent mis-merge (§16, A34). `HEARTBEAT.species[]` is a
// DISPLAY CENSUS: a whole world, read by a human on a status page and by
// nothing else, carrying the bytes the owning world's `Species` record holds so
// the label reads as that world's own player sees it (§17, A36).
//
// So THE VALIDATOR IS NOT SHARED EITHER. species.go rejects a name with edge
// whitespace. Here edge whitespace, doubled internal whitespace and a half that
// is nothing but whitespace are ALL LEGAL, and are exactly what a faithful
// census carries: a measured 13% of the names standing in the rig's registries
// carry stray whitespace, and in one sampled world those species were 27% of
// the living population. Reusing species.go's rule here would delete a
// quarter of a world from the page with one log line nobody reads.
//
// THE FAILURE PHILOSOPHY IS SHARED: a label is never worth an organism, a
// session, or a heartbeat. Nothing in this file returns an error up the stack.
// `HEARTBEAT` has NO NACK CHANNEL AT ALL, so §9.3's default answer for a bad
// `data` field is close 4003 — and applying that default to a display field
// would let a species name kill a live session. A malformed census is
// STRIPPED, and the heartbeat is processed exactly as before.

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// SpeciesCensusMax bounds the census array (contract-a.md §10, §17 A35;
// contract-b-m4.md §12, §16 B11). It is a WIRE BOUND, NOT A DISPLAY
// PREFERENCE: it is what keeps a stats block — and the PEER_STATUS broadcast
// that republishes six of them — bounded. No party may raise it unilaterally.
const SpeciesCensusMax = 32

// The per-half byte limit is the same NUMBER as species.go's, and deliberately
// not the same RULE (§17, A36). It is spelled out again here so a change to one
// lane cannot silently move the other.
const MaxCensusNameBytes = 64

// CensusEntry is one species in one world at the instant a heartbeat was built.
//
// Four fields, no nesting, and NO SPECIES ID IN PARTICULAR: `Species.speciesID`
// is a world-local counter, and putting one here would invite exactly the
// cross-world reading §16 exists to forbid (§17, A35).
//
// Bibites and Eggs travel SEPARATELY rather than as their sum, because the page
// reconciles them against two different counters: `population` counts organisms
// and excludes eggs, `eggCount` counts eggs. A single total would be
// reconcilable against neither.
type CensusEntry struct {
	GenericName  string `json:"genericName"`
	SpecificName string `json:"specificName"`
	Bibites      int    `json:"bibites"`
	Eggs         int    `json:"eggs"`
}

// Count is `bibites + eggs`, the game's own Species.count, and the key the
// census is sorted by, descending. NOTHING DOWNSTREAM RE-SORTS: the array's
// order is the census's own statement (§17, A35).
func (e CensusEntry) Count() int { return e.Bibites + e.Eggs }

// Census is the array itself, wrapped so that ABSENT AND EMPTY STAY DIFFERENT
// FACTS all the way to the page.
//
// A nil *Census is ABSENT: an old mod, a mod that does not implement §17, or no
// heartbeat carrying one yet — and no reader can tell those apart from a world
// with nothing alive, so the page renders UNKNOWN. A non-nil *Census with no
// entries is the stronger statement a reporting mod makes about a world that
// really has nothing alive in it. The distinction is the whole reason the field
// is OPTIONAL rather than defaulted, so it may never be flattened into a Go
// `[]CensusEntry` whose nil and empty both encode as absent.
type Census struct {
	Entries []CensusEntry

	// fieldErr is why the raw JSON was not an array, "" when it was. It is
	// unexported so it never reaches a wire, a journal or a stats block: it
	// carries a decode-time complaint as far as the strip-and-log site and no
	// further.
	fieldErr string
	// stripped is one line per entry the decoder could not shape. Also
	// unexported, and also read exactly once, by CarryCensus.
	stripped []string
}

// UnmarshalJSON decodes the array PERMISSIVELY, and never returns an error.
//
// A failed decode inside a HEARTBEAT is a close 4003 (contract-a.md §3.2,
// §9.3), and §5.2's strip table names this field as the exception: `species`
// that is not an array costs the FIELD, an entry that breaks its shape costs
// THAT ENTRY, and neither costs the heartbeat, the connection, or the liveness
// the heartbeat carries. So every shape error is recorded here and answered by
// CarryCensus, and this method never reports one.
//
// A literal `null` never reaches this method: encoding/json sets the pointer
// field to nil instead, which is exactly "absent", and absent is valid.
func (c *Census) UnmarshalJSON(b []byte) error {
	*c = Census{}
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		// STRIP TABLE ROW 2, the field strip: `species` present and not an
		// array. The whole field goes, the heartbeat is processed without a
		// census, and the page reads unknown.
		c.fieldErr = "species is present and is not an array: " + err.Error()
		return nil
	}
	c.Entries = make([]CensusEntry, 0, len(raw))
	for i, elem := range raw {
		// Pointers, because PRESENCE is a rule of its own: "a name missing" and
		// "a count missing" are entry-strip reasons in their own right, and `0`
		// is a legal egg count while a missing one is not a legal entry.
		var e struct {
			GenericName  *string `json:"genericName"`
			SpecificName *string `json:"specificName"`
			Bibites      *int    `json:"bibites"`
			Eggs         *int    `json:"eggs"`
		}
		if err := json.Unmarshal(elem, &e); err != nil {
			// Not an object; a name that is not a string; a count that is not a
			// number, or is a number that is not an integer. All one row.
			c.stripped = append(c.stripped,
				fmt.Sprintf("species[%d] does not have the shape of a census entry: %v", i, err))
			continue
		}
		switch {
		case e.GenericName == nil:
			c.stripped = append(c.stripped, fmt.Sprintf("species[%d].genericName is absent", i))
			continue
		case e.SpecificName == nil:
			c.stripped = append(c.stripped, fmt.Sprintf("species[%d].specificName is absent", i))
			continue
		case e.Bibites == nil:
			c.stripped = append(c.stripped, fmt.Sprintf("species[%d].bibites is absent", i))
			continue
		case e.Eggs == nil:
			c.stripped = append(c.stripped, fmt.Sprintf("species[%d].eggs is absent", i))
			continue
		}
		c.Entries = append(c.Entries, CensusEntry{
			GenericName:  *e.GenericName,
			SpecificName: *e.SpecificName,
			Bibites:      *e.Bibites,
			Eggs:         *e.Eggs,
		})
	}
	return nil
}

// MarshalJSON emits the entries and nothing else: a census that is PRESENT
// always encodes as an array, never as `null`, because `null` would decode back
// as absent at the next hop and turn "this world has nothing alive" into "we do
// not know", which is a different and weaker fact (§17, A35).
func (c Census) MarshalJSON() ([]byte, error) {
	if c.Entries == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(c.Entries)
}

// Clone copies the entries. A decoded frame is transient and a stats block
// outlives it, so every hop that holds a census holds its own.
func (c *Census) Clone() *Census {
	if c == nil {
		return nil
	}
	out := &Census{Entries: make([]CensusEntry, len(c.Entries))}
	copy(out.Entries, c.Entries)
	return out
}

// Len is the number of entries a present census carries, and 0 for an absent
// one. A caller that needs to tell those apart tests the pointer, not this.
func (c *Census) Len() int {
	if c == nil {
		return 0
	}
	return len(c.Entries)
}

// TruncatedFlag is `truncated`, the boolean that qualifies `species` and
// nothing else (contract-a.md §5.2, §17 A35).
//
// It means EXACTLY ONE THING: this array is not the whole census. The mod sets
// it when it applies the cap; the sidecar sets it when it strips an entry or
// trims an over-long array. It is MONOTONIC — set on the way, never cleared —
// and MEANINGLESS WITHOUT `species`, so a receiver ignores it when the array is
// absent.
//
// Absent and `false` say the same thing, which is why this is a bare bool with
// `omitempty` rather than a pointer: there is no third state to represent. The
// permissive decode below is the other half of §5.2's row 2 — "a `truncated`
// that is not a boolean is dropped on its own and read as absent" — and it must
// not be a decode error, because a decode error here is a closed session.
type TruncatedFlag bool

func (t *TruncatedFlag) UnmarshalJSON(b []byte) error {
	var v bool
	if err := json.Unmarshal(b, &v); err != nil {
		*t = false
		return nil
	}
	*t = TruncatedFlag(v)
	return nil
}

// CarryCensus is the strip-or-carry decision for the census, in one place, used
// at every hop that takes one off a wire. It is species.go's CarrySpecies for
// the other lane, and it applies contract-a.md §5.2's three-row strip table AND
// ONLY THAT TABLE:
//
//	row 1, the ENTRY strip — an entry that is not an object, a name that is
//	missing, not a string, empty, over 64 UTF-8 bytes or not valid UTF-8, or a
//	count that is missing, not a number, not an integer or negative: strip THAT
//	ENTRY, keep the rest, set truncated.
//
//	row 2, the FIELD strip — `species` present and not an array: strip the WHOLE
//	FIELD and process the heartbeat without a census.
//
//	row 3, the TRIM — more than speciesCensusMax entries: keep the FIRST 32,
//	because the array is sorted descending and those are the largest, and set
//	truncated. It is a mod defect, and the caller logs it as one.
//
// It returns the census to carry (nil when the field was stripped or was absent
// to begin with), the truncated flag after the monotonic OR, and one log line
// per rule that fired. NEVER AN ERROR: there is no code path on which a species
// name may fail a heartbeat.
//
// NOTHING HERE REPAIRS A NAME. No trimming, no whitespace collapsing, no case
// folding, no Unicode normalization, by any party (§17, A36) — a half that is
// only whitespace PASSES, because that is a name the world is genuinely
// displaying. Nothing here re-sorts or de-duplicates either: two entries whose
// names differ only in whitespace are two `Species` records in that world, and
// the census reports two.
func CarryCensus(c *Census, truncated bool) (*Census, bool, []string) {
	if c == nil {
		// Absent. `truncated` is meaningless without the array, so it is
		// ignored rather than carried (§5.2).
		return nil, false, nil
	}
	if c.fieldErr != "" {
		// Row 2. The whole field goes, and with it the flag that qualified it.
		return nil, false, []string{c.fieldErr}
	}

	var why []string
	out := &Census{Entries: make([]CensusEntry, 0, len(c.Entries))}
	// Row 1 reasons the decoder already found (shape), then row 1 reasons only
	// the values can show (byte length, UTF-8, sign).
	why = append(why, c.stripped...)
	for i, e := range c.Entries {
		if bad := censusEntryProblem(i, e); bad != "" {
			why = append(why, bad)
			continue
		}
		out.Entries = append(out.Entries, e)
	}
	if len(out.Entries) > SpeciesCensusMax {
		// Row 3. Keep the FIRST 32 — the sender sorted descending, so those are
		// the largest, which is why the sort is a sender obligation and not a
		// display preference.
		why = append(why, fmt.Sprintf(
			"species carries %d entries, over the speciesCensusMax of %d; keeping the first %d",
			len(out.Entries), SpeciesCensusMax, SpeciesCensusMax))
		out.Entries = out.Entries[:SpeciesCensusMax]
	}
	// MONOTONIC: whatever the sender said, OR whatever this hop had to do.
	return out, truncated || len(why) > 0, why
}

// censusEntryProblem applies the per-entry VALUE rules of contract-a.md §5.2
// and §17 A36. Everything on the list is checkable without an opinion: a byte
// count, a UTF-8 decode, a sign. Every rule that needs an opinion about what a
// name SHOULD look like belongs to the migration lane, and that lane is §16's —
// which is what lets a Go sidecar and a C# mod agree on the validity of a
// census without agreeing on anything about language.
func censusEntryProblem(i int, e CensusEntry) string {
	if why := censusNameProblem(i, "genericName", e.GenericName); why != "" {
		return why
	}
	if why := censusNameProblem(i, "specificName", e.SpecificName); why != "" {
		return why
	}
	if e.Bibites < 0 {
		return fmt.Sprintf("species[%d].bibites is %d, which is negative", i, e.Bibites)
	}
	if e.Eggs < 0 {
		return fmt.Sprintf("species[%d].eggs is %d, which is negative", i, e.Eggs)
	}
	return ""
}

// censusNameProblem is the whole of the name rule: valid UTF-8, 1 to 64 UTF-8
// bytes, and NOTHING ELSE. There is deliberately no whitespace test here — a
// half carrying an edge space shows in-game as a doubled space, and the census
// must not repair it (§17, A36).
func censusNameProblem(i int, field, v string) string {
	switch {
	case v == "":
		return fmt.Sprintf("species[%d].%s is empty; a name with no bytes is not a name", i, field)
	case len(v) > MaxCensusNameBytes:
		return fmt.Sprintf("species[%d].%s is %d UTF-8 bytes, over the %d limit",
			i, field, len(v), MaxCensusNameBytes)
	case !utf8.ValidString(v):
		return fmt.Sprintf("species[%d].%s is not valid UTF-8", i, field)
	}
	return ""
}
