// Package termsafe is B30's escaping obligation for THE TERMINAL, which
// contract-b-m4.md §22 binds identically to the page: "escape for the surface
// rendered into — HTML, an HTML attribute, a URL, JSON in a script, TERMINAL
// ESCAPE SEQUENCES — and never render one as markup."
//
// It lives in its own package because there is now more than one terminal
// surface printing text this project did not author. `ringstat` prints species
// names and peer ids from the map; `multiverse-sidecar --my-slot` and
// `--diagnose` print peer ids, game versions, close reasons and `lastRefusal`
// strings, which arrive from the relay and from other people's worlds. A rule
// that is re-implemented per surface is a rule that is eventually implemented
// once too few, and the second surface is the one a participant runs on their
// own machine.
//
// A terminal's markup is the control character. Attacker-chosen text carrying
// ESC can move the cursor, repaint the screen, retitle the window or — on some
// terminals — put text into the user's input buffer. So every C0 control, DEL,
// every C1 control and every byte that is not valid UTF-8 becomes U+FFFD.
package termsafe

import (
	"strings"
	"unicode/utf8"
)

// Text replaces every control character and every invalid byte with U+FFFD.
//
// IT REPLACES RATHER THAN DROPS, on purpose: a dropped byte makes two different
// names print identically, and an operator comparing a page against a terminal
// has to be able to see that something was there.
func Text(s string) string {
	if s == "" {
		return s
	}
	if safe(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == utf8.RuneError || control(r) {
			// Either a real U+FFFD or an invalid byte; both render as the same
			// mark, which is the honest answer for a byte nothing can display.
			b.WriteRune('�')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Clip fits sanitized text to n bytes, ending it with an ellipsis when it had
// to be cut. Every caller is printing somebody else's chosen text, so the
// sanitizer belongs here rather than at each call site: a new column added next
// year gets it for free, and forgetting it is not one of the available
// mistakes.
func Clip(s string, n int) string {
	s = Text(s)
	if len(s) <= n {
		return s
	}
	cut := n - 1
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func safe(s string) bool {
	for _, r := range s {
		if control(r) || r == utf8.RuneError {
			return false
		}
	}
	return utf8.ValidString(s)
}

func control(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}
