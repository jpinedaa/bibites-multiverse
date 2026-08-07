package wire

// The OPTIONAL world-settings vocabulary, shared by both wires: contract-a.md
// §5.1 and §19 (A42, the five settings on `CONFIG_UPDATE`) and
// contract-b-m4.md §6.3.1 and §19 (B18, the same five inside the peer stats
// block beside the two version strings).
//
// ONE TYPE LIVES HERE AND ONLY ONE: the exclusion list. The other four settings
// are a float, an int and two bools, and a plain pointer already says
// everything about them that has to be said — nil is unknown, and a value is a
// value. The list needs a type because AN EMPTY LIST IS NOT AN ABSENT LIST: a
// present `[]` says the origin mod has the exclusion policy OFF, absent says
// nobody told us, and a Go `[]string` whose nil and empty both encode as
// absent would destroy the distinction at the first hop (§19, A42; B18).
//
// THE NAME RULE HERE IS A34'S, AND IT IS NOT census.go's. An exclusion entry is
// a MATCHING key: it is the exact string the origin mod compares an organism's
// normalized full name against, so it travels A34-normalized — trimmed,
// internal whitespace runs collapsed — and a reader sees the comparison the
// policy performs rather than the string an operator typed. A census name is a
// DISPLAY label and travels raw (§17, A36). Both rules now live on one stats
// block and NO PARTY MAY APPLY EITHER RULE TO THE OTHER'S FIELD (§19, B18).
//
// NOTHING HERE NORMALIZES, RE-NORMALIZES, SORTS, DEDUPLICATES OR REPAIRS AN
// ENTRY. The mod normalizes on the way out because it is the party that holds
// the comparison; every hop after that copies bytes. What this file does is
// CHECK and, when a check fails, STRIP — because a settings row must never be
// able to end a session that is carrying organisms (§19, A42's strip rule).

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxExcludeNameBytes is §19 A42's bound on one entry: §16's 64-byte half
// twice, plus the single U+0020 that joins them.
const MaxExcludeNameBytes = 2*MaxSpeciesNameBytes + 1

// ExcludeList is `migrationExclude`: the species a world never exports.
//
// A nil *ExcludeList is ABSENT — an older mod, a mod that does not implement
// §19, or no mod connected at all — and every reader renders it UNKNOWN. A
// non-nil list with no entries is the stronger, different statement that the
// policy is OFF. That distinction is the whole reason this is a wrapper type
// and not a slice, and it may never be flattened.
type ExcludeList struct {
	Names []string

	// fieldErr is why the raw JSON was not an array, "" when it was.
	// Unexported: it carries a decode-time complaint as far as the
	// strip-and-log site and no further.
	fieldErr string
	// stripped is one line per entry the decoder could not shape. Also
	// unexported, and also read exactly once, by CarryExclude.
	stripped []string
}

// UnmarshalJSON decodes the array PERMISSIVELY and never returns an error, for
// the same reason census.go's does: `CONFIG_UPDATE` is the handshake and has no
// NACK channel, so §9.3's default answer for a bad `data` field is close 4003 —
// and applying that default to an observability field would let a settings row
// kill a live rig at reconnect (§19, A42).
//
// A literal `null` never reaches this method: encoding/json sets the pointer
// field to nil instead, which is exactly "absent", and absent is valid.
func (l *ExcludeList) UnmarshalJSON(b []byte) error {
	*l = ExcludeList{}
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		// The FIELD strip: `migrationExclude` present and not an array. The whole
		// field goes and the reader is left with unknown, which is honest.
		l.fieldErr = "migrationExclude is present and is not an array: " + err.Error()
		return nil
	}
	l.Names = make([]string, 0, len(raw))
	for i, elem := range raw {
		var s string
		if err := json.Unmarshal(elem, &s); err != nil {
			// The ENTRY strip: an element that is not a string.
			l.stripped = append(l.stripped,
				fmt.Sprintf("migrationExclude[%d] is not a string: %v", i, err))
			continue
		}
		l.Names = append(l.Names, s)
	}
	return nil
}

// MarshalJSON emits the entries and never `null`: a list that is PRESENT always
// encodes as an array, because `null` would decode back as absent at the next
// hop and turn "this world excludes nothing" into "we do not know", which is a
// different and weaker fact (§19, A42).
func (l ExcludeList) MarshalJSON() ([]byte, error) {
	if l.Names == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(l.Names)
}

// Clone copies the entries. A decoded frame is transient and a stats block
// outlives it, so every hop that holds a list holds its own.
func (l *ExcludeList) Clone() *ExcludeList {
	if l == nil {
		return nil
	}
	out := &ExcludeList{Names: make([]string, len(l.Names))}
	copy(out.Names, l.Names)
	return out
}

// Len is the number of entries a present list carries, and 0 for an absent one.
// A caller that needs to tell those apart tests the pointer, not this.
func (l *ExcludeList) Len() int {
	if l == nil {
		return 0
	}
	return len(l.Names)
}

// Has reports whether name — already A34-normalized by the caller — is on the
// list. It is a COMPARISON and never a rewrite: the list is not re-normalized
// here, because the mod published the exact strings its own hot path compares
// against and re-normalizing them would describe a match nobody attempts.
//
// It is also NOT ENFORCEMENT. No party that reads this list can enforce it
// (contract-a.md §18 A39, §19 A42): the capture band that applies the policy
// lives in the origin mod. This answers a display question — "is that why this
// world's lanes are quiet" — and nothing else.
func (l *ExcludeList) Has(normalized string) bool {
	if l == nil {
		return false
	}
	for _, n := range l.Names {
		if n == normalized {
			return true
		}
	}
	return false
}

// CarryExclude is the strip-or-carry decision for `migrationExclude`, in one
// place, used at every hop that takes one off a wire. It applies contract-a.md
// §19 A42's rules AND ONLY THOSE:
//
//	the ENTRY strip — an element that is not a string, is empty or is nothing
//	but whitespace (it is not a name the policy can ever match), is over
//	MaxExcludeNameBytes UTF-8 bytes, or is not valid UTF-8: strip THAT ENTRY and
//	keep the rest.
//
//	the FIELD strip — `migrationExclude` present and not an array: strip the
//	WHOLE FIELD, and the reader renders unknown.
//
// It returns the list to carry (nil when the field was stripped or was absent
// to begin with) and one log line per rule that fired. NEVER AN ERROR: there is
// no code path on which a settings row may fail a handshake.
//
// NOTHING HERE REPAIRS AN ENTRY. A name that is over-long is dropped, never
// truncated; a name carrying an internal double space is carried as it arrived,
// because the mod is the party that normalizes and a second opinion here would
// publish a comparison nobody performs.
func CarryExclude(l *ExcludeList) (*ExcludeList, []string) {
	if l == nil {
		return nil, nil
	}
	if l.fieldErr != "" {
		return nil, []string{l.fieldErr}
	}
	var why []string
	why = append(why, l.stripped...)
	out := &ExcludeList{Names: make([]string, 0, len(l.Names))}
	for i, n := range l.Names {
		if bad := excludeEntryProblem(i, n); bad != "" {
			why = append(why, bad)
			continue
		}
		out.Names = append(out.Names, n)
	}
	return out, why
}

func excludeEntryProblem(i int, n string) string {
	switch {
	case n == "":
		return fmt.Sprintf("migrationExclude[%d] is empty; a name with no bytes is not a name", i)
	case strings.TrimFunc(n, unicode.IsSpace) == "":
		return fmt.Sprintf("migrationExclude[%d] is only whitespace; the policy can never match it", i)
	case len(n) > MaxExcludeNameBytes:
		return fmt.Sprintf("migrationExclude[%d] is %d UTF-8 bytes, over the %d limit",
			i, len(n), MaxExcludeNameBytes)
	case !utf8.ValidString(n):
		return fmt.Sprintf("migrationExclude[%d] is not valid UTF-8", i)
	}
	return ""
}

// NormalizeSpeciesName is contract-a.md §16 A34's repair: trim the ends,
// collapse every internal whitespace run to one U+0020.
//
// IT IS FOR COMPARISON, AND THE RESULT IS NEVER DISPLAYED AND NEVER WRITTEN
// BACK. Two questions need it and no others: is this census entry the same
// species as that one in another world, and is it the species named on a
// world's exclusion list. A census name is displayed RAW, doubled spaces and
// all, because a tidied name is a name the player cannot find in their own game
// (§17, A36) — and two spellings a world keeps apart really are two Species
// records in it.
func NormalizeSpeciesName(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

// SpeciesKey is the comparison key for one census entry: the two halves joined
// by one space and then A34-normalized, which is the same string an exclusion
// entry carries. It is what lets the operator surface say "this world is full
// of a species it never exports" without either lane re-normalizing the other's
// field (§19, B18).
func SpeciesKey(generic, specific string) string {
	return NormalizeSpeciesName(generic + " " + specific)
}
