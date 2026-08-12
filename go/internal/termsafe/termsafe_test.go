package termsafe

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestEveryControlCharacterBecomesAMark is B30 on a terminal: a name carrying
// ESC can move the cursor, repaint the screen, retitle the window or put text
// into the reader's input buffer, and there is one rule for every surface that
// prints text this project did not author.
func TestEveryControlCharacterBecomesAMark(t *testing.T) {
	cases := []struct {
		what string
		in   string
	}{
		{"an OSC window title", "Cyanëa\x1b]0;pwned\x07velox"},
		{"a screen clear", "Alpha\x1b[2Jbigbuckdeluxus"},
		{"a bare escape", "\x1b"},
		{"a C1 control", "before\u0085after"},
		{"DEL", "before\x7fafter"},
		{"an invalid byte", "before\xffafter"},
		{"a newline, which breaks a table row", "one\ntwo"},
		{"a carriage return, which overwrites the line", "one\rtwo"},
		{"a tab, which breaks a column", "one\ttwo"},
	}
	for _, c := range cases {
		got := Text(c.in)
		for _, r := range got {
			if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
				t.Errorf("%s: %q survived sanitising as %q", c.what, c.in, got)
				break
			}
		}
		if !strings.Contains(got, "�") {
			t.Errorf("%s: %q became %q, with no mark where the bytes were — it must REPLACE "+
				"rather than drop, or two different names print identically", c.what, c.in, got)
		}
	}
}

// TestOrdinaryTextIsUntouched, including the non-ASCII a species name is free to
// carry: the rule is about control characters and not about alphabets.
func TestOrdinaryTextIsUntouched(t *testing.T) {
	for _, s := range []string{
		"", "slot-4", "Cyanëa velox", "0.6.3.1", "capacity: maxFramesPerSecond 50 exceeded",
		"名前", "emoji 🐛 is text",
	} {
		if got := Text(s); got != s {
			t.Errorf("Text(%q) = %q, want it unchanged", s, got)
		}
	}
}

// TestClipSanitisesBeforeItCuts, because a caller that clipped first could cut a
// multi-byte rune in half and hand the terminal a byte nothing can display.
func TestClipSanitisesBeforeItCuts(t *testing.T) {
	if got := Clip("short", 40); got != "short" {
		t.Errorf("Clip did not leave a short string alone: %q", got)
	}
	got := Clip("\x1b[31mmuch longer than the column allows", 12)
	if strings.Contains(got, "\x1b") {
		t.Errorf("Clip let a control character through: %q", got)
	}
	if len(got) > 12+len("…") {
		t.Errorf("Clip returned %d bytes for a 12-byte column: %q", len(got), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("Clip cut without saying it did: %q", got)
	}
	// A rune that straddles the cut is never split: an odd column against
	// two-byte runes is exactly where a naive slice hands the terminal half a
	// character.
	for n := 3; n < 20; n++ {
		if got := Clip(strings.Repeat("é", 19), n); !utf8.ValidString(got) {
			t.Errorf("Clip to %d bytes split a rune: %q", n, got)
		}
	}
}
