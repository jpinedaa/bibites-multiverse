package wire

// The OPTIONAL species-identity block, shared by both wires: contract-a.md §16
// (A30, `MIGRATE_OUT.species` and `MIGRATE_IN.species`) and contract-b-m4.md
// §15 (B9, `MIGRATION_PAYLOAD.species`). One shape, one validator, one rule.
//
// THE RULE, and it is the whole of this file: SCHEMA VALIDATION ONLY, AND A
// MALFORMED BLOCK IS STRIPPED, NEVER REFUSED. A species name is a label the
// origin world asserts; refusing an organism over a label is the trade
// contract-a.md §9.1 declines to make, so nothing here returns an error up the
// stack. A block that breaks the shape is dropped with one log line and the
// migration proceeds without it, and the destination mod then applies its
// absent-block rule (contract-a.md §16, A32).
//
// Nothing in Go ever reads a MEANING out of these two names. The registry they
// resolve against lives inside a game process, and only the mod can see it
// (A31). This layer copies bytes.

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxSpeciesNameBytes bounds each half of a species name (contract-a.md §5.3,
// contract-b-m4.md §6.6). It is a UTF-8 BYTE limit, not a rune count.
const MaxSpeciesNameBytes = 64

// Species is the migrant's species identity as its ORIGIN world named it.
//
// Four fields, no nesting, and no id of any kind: an id is world-local by
// construction and that is the defect §16 closes. The two halves are REQUIRED
// together whenever the block is present; the two parent halves are
// ALL-OR-NOTHING and carry exactly one generation, because the importer can
// only link under a species that exists locally (A30).
//
// The names are carried BYTE FOR BYTE. No trimming, no case folding, no
// Unicode normalization, no re-generation: the importer's match is a byte
// comparison, so any tidying on this side is a silent mismatch on the other.
type Species struct {
	GenericName        string `json:"genericName"`
	SpecificName       string `json:"specificName"`
	ParentGenericName  string `json:"parentGenericName,omitempty"`
	ParentSpecificName string `json:"parentSpecificName,omitempty"`

	// decodeErr is why the raw JSON was not the shape above, "" when it was. It
	// is unexported so it never reaches a wire, a journal or a ledger: it exists
	// only to carry a decode-time complaint as far as the strip-and-log site.
	decodeErr string
}

// UnmarshalJSON decodes the block PERMISSIVELY, and that is deliberate.
//
// `species` of the wrong JSON shape — a string, a number, a nested object, a
// non-string half — MUST NOT fail the enclosing message's decode, because a
// failed decode is a MALFORMED_MESSAGE NACK and A30 names this block as the one
// exception to that rule. So every shape error is recorded here and answered by
// MalformedReason, and this method never returns one.
//
// A literal `null` never reaches this method: encoding/json sets the pointer
// field to nil instead, which is exactly "absent", and absent is valid.
func (s *Species) UnmarshalJSON(b []byte) error {
	// Pointers, because PRESENCE is the question the parent pair turns on: a
	// block carrying exactly one parent field is malformed, and `""` is not the
	// same statement as "no field at all".
	var raw struct {
		GenericName        *string `json:"genericName"`
		SpecificName       *string `json:"specificName"`
		ParentGenericName  *string `json:"parentGenericName"`
		ParentSpecificName *string `json:"parentSpecificName"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		*s = Species{decodeErr: "species is not an object of four string fields: " + err.Error()}
		return nil
	}
	*s = Species{}
	if raw.GenericName != nil {
		s.GenericName = *raw.GenericName
	}
	if raw.SpecificName != nil {
		s.SpecificName = *raw.SpecificName
	}
	if raw.ParentGenericName != nil {
		s.ParentGenericName = *raw.ParentGenericName
	}
	if raw.ParentSpecificName != nil {
		s.ParentSpecificName = *raw.ParentSpecificName
	}
	switch {
	case raw.GenericName == nil:
		s.decodeErr = "species.genericName is absent; both halves are REQUIRED when the block is present"
	case raw.SpecificName == nil:
		s.decodeErr = "species.specificName is absent; both halves are REQUIRED when the block is present"
	case (raw.ParentGenericName == nil) != (raw.ParentSpecificName == nil):
		s.decodeErr = "species carries exactly one parent name; the parent pair is all-or-nothing"
	}
	return nil
}

// MalformedReason returns why s breaks the shape rules of contract-a.md §5.3
// and contract-b-m4.md §6.6, or "" when it is well formed.
//
// A NIL BLOCK IS WELL FORMED. Absent is valid and ordinary: an organism with no
// species record, a mod that does not implement contract-a/2.1, or a block a
// schema check already stripped — an importer cannot tell the three apart and
// is not meant to (A30).
func (s *Species) MalformedReason() string {
	if s == nil {
		return ""
	}
	if s.decodeErr != "" {
		return s.decodeErr
	}
	if why := speciesNameProblem("genericName", s.GenericName); why != "" {
		return why
	}
	if why := speciesNameProblem("specificName", s.SpecificName); why != "" {
		return why
	}
	if (s.ParentGenericName == "") != (s.ParentSpecificName == "") {
		return "species carries one parent name and not the other; the parent pair is " +
			"all-or-nothing and neither half may be empty"
	}
	if s.ParentGenericName == "" {
		// No parent. A root species travels without one, and that is normal.
		return ""
	}
	if why := speciesNameProblem("parentGenericName", s.ParentGenericName); why != "" {
		return why
	}
	return speciesNameProblem("parentSpecificName", s.ParentSpecificName)
}

// speciesNameProblem applies the per-half rules of contract-a.md §5.3: present,
// non-empty, at most 64 UTF-8 bytes, and no leading or trailing whitespace.
//
// The whitespace rule is not fussiness. The destination mod matches on
// `genericName + " " + specificName` by exact ordinal comparison (§5.7 step 3),
// so a stray space founds a second local species that reads identically to the
// first one in every UI that shows it.
func speciesNameProblem(field, v string) string {
	switch {
	case v == "":
		return "species." + field + " is empty; it is REQUIRED and non-empty when the block is present"
	case len(v) > MaxSpeciesNameBytes:
		return fmt.Sprintf("species.%s is %d UTF-8 bytes, over the %d limit",
			field, len(v), MaxSpeciesNameBytes)
	case !utf8.ValidString(v):
		return "species." + field + " is not valid UTF-8"
	case strings.TrimSpace(v) != v:
		return "species." + field + " carries leading or trailing whitespace"
	}
	return ""
}

// CarrySpecies is the strip-or-carry decision, in one place, used at every hop
// that takes a block off a wire.
//
// It returns the block to pass on and the reason a malformed one was dropped.
// The caller logs that reason ONCE and carries on: the return is never an
// error, because there is no code path on which a species name may refuse an
// organism (contract-b-m4.md §6.6, contract-a.md §9.1).
//
// The block it returns is a COPY. The decoded frame it came from is transient,
// and the journal, the ledger and the next hop all outlive it.
func CarrySpecies(s *Species) (*Species, string) {
	if why := s.MalformedReason(); why != "" {
		return nil, why
	}
	if s == nil {
		return nil, ""
	}
	c := *s
	c.decodeErr = ""
	return &c, ""
}

// SpeciesName is the name the destination mod matches on: the two halves with
// exactly one U+0020 between them, which is the game's own Species.name
// (contract-a.md §5.7 step 3). Nothing in Go resolves it — it is here so a log
// line can say which organism carried which label, and for nothing else.
func SpeciesName(s *Species) string {
	if s == nil {
		return ""
	}
	return s.GenericName + " " + s.SpecificName
}
