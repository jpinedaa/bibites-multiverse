package sidecar

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contractb"
)

// The two strings the rig configures. The keeper carries edge whitespace and a
// control rune, because a config file is typed by a person and this is what
// the author-side bound is for; the world name carries an accented rune and a
// pair of angle brackets, because it is untrusted display text on the same
// footing as a census name and nothing on the way may repair it.
const (
	rawKeeper   = "  Jorge\a  "
	cleanKeeper = "Jorge"
	rawWorld    = "Cyanëa <Reach>"
)

// waitPeerStats polls the subscriber's broadcasts for one slot's stats block.
// §6.5 makes PEER_STATUS full state, so any frame that satisfies ok answers.
func waitPeerStats(t *testing.T, sub *subscriber, slot int, what string,
	ok func(contractb.SlotInfo) bool) contractb.SlotInfo {
	t.Helper()
	var got contractb.SlotInfo
	waitFor(t, 20*time.Second, what, func() bool {
		for _, st := range sub.peerStatuses(t) {
			for _, si := range st.Slots {
				if si.Slot == slot && si.Stats != nil && ok(si) {
					got = si
					return true
				}
			}
		}
		return false
	})
	return got
}

// TestParticipantNamesCrossTheRig is the whole of contract-b-m4.md §33 B49's
// wire half: the two strings the participant configured leave the sidecar on
// the stats-bearing PING, the relay carries them without knowing what they are,
// and a subscriber reads them off PEER_STATUS.
//
// It also asserts the author-side bound where it is actually enforced. The
// keeper was configured with edge whitespace and a control rune and reaches the
// map as neither: nothing downstream trims or strips one, so the value that
// leaves the sidecar is the value the map shows.
func TestParticipantNamesCrossTheRig(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2), tune: func(i int, c *Config) {
		if i == 0 {
			c.Keeper = rawKeeper
			c.WorldName = rawWorld
		}
	}})
	sub := dialSubscriber(t, g.relay.url(), g.relay)
	sub.wait(contractb.TypeHandshakeAck, 10*time.Second)
	a, b := g.node(0), g.node(1)

	first := waitPeerStats(t, sub, a.slot, "slot A's keeper to reach a subscriber",
		func(si contractb.SlotInfo) bool { return si.Stats.Keeper != "" })
	if first.Stats.Keeper != cleanKeeper {
		t.Fatalf("keeper crossed the rig as %q, want %q — it is trimmed and control-stripped "+
			"by the sidecar that authors it, and by nobody else",
			first.Stats.Keeper, cleanKeeper)
	}
	// The world name was already within bounds, so it crosses byte for byte,
	// angle brackets and all. Escaping is a renderer's job and repair is
	// nobody's.
	if first.Stats.WorldName != rawWorld {
		t.Fatalf("worldName crossed the rig as %q, want %q", first.Stats.WorldName, rawWorld)
	}

	// They ride the PERIODIC PING rather than the one-off SECTOR_CLAIM: a later
	// broadcast carries a newer statsAsOfMs and still carries both names.
	later := waitPeerStats(t, sub, a.slot, "the names to survive the periodic stats republish",
		func(si contractb.SlotInfo) bool { return si.StatsAsOfMs > first.StatsAsOfMs })
	if later.Stats.Keeper != cleanKeeper || later.Stats.WorldName != rawWorld {
		t.Fatalf("the republished block lost a name: keeper=%q worldName=%q",
			later.Stats.Keeper, later.Stats.WorldName)
	}

	// ABSENT. B configured neither, and NOTHING ANYWHERE IN THIS CHAIN INVENTS
	// ONE — not from the OS user, not from the peer id, not from the data
	// directory. Its other stats are exact, which is what makes the two gaps a
	// reading rather than a broken slot.
	bv := waitPeerStats(t, sub, b.slot, "slot B's stats to reach a subscriber",
		func(si contractb.SlotInfo) bool { return si.Stats.CustodyDepth != nil })
	if bv.Stats.Keeper != "" || bv.Stats.WorldName != "" {
		t.Fatalf("a world that configured neither name published keeper=%q worldName=%q; "+
			"an unchosen name is not a name anybody consented to publish",
			bv.Stats.Keeper, bv.Stats.WorldName)
	}
}

// TestParticipantNamesArePublishedWithNoMod is the deliberate precedent
// inboundRatePerSimMinute set (§18, B16) applied to §33's two strings: they are
// this sidecar's own configuration rather than a reading of a mod, so a slot
// with no game running still says whose world it is. That is the state an
// operator most needs a name for — a dark slot is otherwise a number.
func TestParticipantNamesArePublishedWithNoMod(t *testing.T) {
	g := newGrid(t, 1, gridOptions{
		layout: layoutRow(1), noMods: true, skipEdgeCheck: true,
		tune: func(_ int, c *Config) {
			c.Keeper = cleanKeeper
			c.WorldName = rawWorld
		}})
	sub := dialSubscriber(t, g.relay.url(), g.relay)
	sub.wait(contractb.TypeHandshakeAck, 10*time.Second)
	a := g.node(0)

	si := waitPeerStats(t, sub, a.slot, "a mod-less world to publish its keeper",
		func(si contractb.SlotInfo) bool { return si.Stats.Keeper != "" })
	if si.ModConnected {
		t.Fatal("the rig started a mod; this test is about the slot that has none")
	}
	if si.Stats.Keeper != cleanKeeper || si.Stats.WorldName != rawWorld {
		t.Fatalf("a mod-less world published keeper=%q worldName=%q",
			si.Stats.Keeper, si.Stats.WorldName)
	}
	// And the fields that DO need a mod are still absent, which is the contrast
	// the precedent rests on: config-sourced is known, mod-sourced is unknown.
	if si.Stats.Population != nil {
		t.Fatalf("a mod-less world reported a population of %d", *si.Stats.Population)
	}
}

// TestSanitizePublicName is §33 B49's author-side bound, rule by rule. Nothing
// downstream applies any of it, so every case here is the last chance the value
// has to be made publishable — and the one thing no case does is substitute.
func TestSanitizePublicName(t *testing.T) {
	// Sixty-four bytes of "é" is thirty-two runes, so a clip at the bound falls
	// exactly on a boundary. Shifting the same string by one ASCII byte puts a
	// CONTINUATION byte at the bound instead, which is the case a byte-wise clip
	// breaks: it would publish half a rune, and half a rune is not a name.
	const twoByteRune = "é"
	fits := strings.Repeat(twoByteRune, MaxPublicNameBytes/2)
	overByOne := fits + "x"
	splitAtTheBound := "a" + fits
	shortOfTheBound := "a" + strings.Repeat(twoByteRune, MaxPublicNameBytes/2-1)

	for _, tc := range []struct {
		name     string
		in       string
		want     string
		adjusted bool
	}{
		{"empty stays empty and says nothing", "", "", false},
		{"an ordinary handle is untouched", "Jorge", "Jorge", false},
		{"surrounding whitespace is trimmed", "  Jorge\t\n", "Jorge", true},
		{"internal spacing is the participant's own", "Cyanea  Reach", "Cyanea  Reach", false},
		{"control runes are stripped", "Jor\age\x1b", "Jorge", true},
		{"whitespace alone is dropped", "   \t ", "", true},
		{"control runes alone are dropped", "\x01\x02", "", true},
		{"invalid UTF-8 is dropped whole", "Jorge\xff", "", true},
		{"a value at the bound is kept", fits, fits, false},
		{"a value over the bound is clipped at a rune boundary", overByOne, fits, true},
		{"a clip never splits a rune", splitAtTheBound, shortOfTheBound, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, why := sanitizePublicName(tc.in)
			if got != tc.want {
				t.Fatalf("sanitizePublicName(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if (why != "") != tc.adjusted {
				t.Fatalf("sanitizePublicName(%q) reported %q; adjusted should be %v",
					tc.in, why, tc.adjusted)
			}
			if len(got) > MaxPublicNameBytes {
				t.Fatalf("sanitizePublicName(%q) returned %d bytes, over the %d bound",
					tc.in, len(got), MaxPublicNameBytes)
			}
			// A second pass changes nothing: what this function publishes is
			// already what it would publish again.
			again, whyAgain := sanitizePublicName(got)
			if again != got || whyAgain != "" {
				t.Fatalf("sanitizing %q again produced %q (%s); the bound is not idempotent",
					got, again, whyAgain)
			}
		})
	}
}

// TestSanitizePublicNamesDropsRatherThanDefaults is the consent rule as
// behaviour: a value that cannot be published is REMOVED, and what replaces it
// is nothing at all. There is no fallback in this function to reach for.
func TestSanitizePublicNamesDropsRatherThanDefaults(t *testing.T) {
	cfg := Config{Keeper: "\x01\x02", WorldName: "  ", Logger: testLogger(t)}
	sanitizePublicNames(&cfg)
	if cfg.Keeper != "" || cfg.WorldName != "" {
		t.Fatalf("an unpublishable name became keeper=%q worldName=%q, and neither is empty",
			cfg.Keeper, cfg.WorldName)
	}
}

// TestAStrayArgumentStopsTheSidecarStarting is the DEFENCE against the shape of
// failure that made this whole area worth a review: a start script wrote
// --world-name Alice's world without quotes, the shell handed this program three
// words, Go's flag package took the first as the value and handed the rest back
// — and nothing collected them. The world started, looked healthy, and published
// half of somebody's name to every peer on the map.
//
// The quoting is fixed where it was written. This is the second half: a value
// that loses its quotes again, anywhere, stops the process instead of
// configuring it as nobody asked.
func TestAStrayArgumentStopsTheSidecarStarting(t *testing.T) {
	dir := t.TempDir()
	// --list-inflight makes a well-formed command line exit 0 without dialling
	// anything, so what is under test is the parse and nothing else.
	base := []string{"--data-dir", dir, "--list-inflight"}
	run := func(args ...string) (int, string) {
		t.Helper()
		var out, errOut bytes.Buffer
		code := Main(append(append([]string{}, base...), args...), &out, &errOut)
		return code, errOut.String()
	}

	code, stderr := run("--world-name", "Alice's", "world")
	if code == 0 {
		t.Fatal("a mis-quoted --world-name started the sidecar with the first word of it")
	}
	if code != 2 {
		t.Fatalf("a stray argument exited %d, want 2 — the same code an unusable flag gives", code)
	}
	// It NAMES what it did not understand: the reader is looking at a start
	// script, and this is the half of the value that lost its quotes.
	for _, want := range []string{`"world"`, "not a flag", "quoted as one argument"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("the refusal does not say %q:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "Alice's world") {
		t.Errorf("the refusal claims to know the name that was meant:\n%s", stderr)
	}

	// More than one stray word is listed, not just the first.
	if _, stderr := run("--keeper", "Ada", "Lovelace", "of", "Tidepool"); !strings.Contains(
		stderr, `"Lovelace", "of", "Tidepool"`) {
		t.Errorf("only some of the stray arguments were named:\n%s", stderr)
	}

	// AND THE CORRECTLY QUOTED COMMAND LINE IS UNTOUCHED. A name with a space
	// and an apostrophe in it is exactly what these two flags are for.
	if code, stderr := run("--world-name", "Alice's world", "--keeper", "Ada Lovelace"); code != 0 {
		t.Fatalf("a properly quoted name exited %d:\n%s", code, stderr)
	}

	// The one command here that takes a positional argument of its own keeps it:
	// --release-inflight needs bounce|drop, and its own handler is what judges
	// that word. It fails here for its own reason — there is no such entry —
	// and that is the point: it is not turned away at the door.
	var out, errOut bytes.Buffer
	Main([]string{"--data-dir", dir, "--yes", "--release-inflight", "mig-1", "bounce"}, &out, &errOut)
	if strings.Contains(errOut.String(), "refusing to start") {
		t.Errorf("--release-inflight lost the bounce|drop argument it takes:\n%s", errOut.String())
	}
	// And a SECOND word after it is stray on the same terms as everywhere else.
	errOut.Reset()
	Main([]string{"--data-dir", dir, "--yes", "--release-inflight", "mig-1", "bounce", "twice"},
		&out, &errOut)
	if !strings.Contains(errOut.String(), `"twice"`) {
		t.Errorf("a stray word after bounce|drop was ignored:\n%s", errOut.String())
	}
}
