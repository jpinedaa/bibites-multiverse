package launchergui

import (
	"path/filepath"
	"strings"
	"testing"
)

// The file goes where a preference goes, and NOWHERE NEAR a world.
//
// The second of these is the one that would have shown: every .json in the
// install root's profiles\ folder is read as a world, and one that will not
// parse as a world is reported as a file the launcher could not read — so a
// window-position file dropped in there raises the red banner on every start.
func TestTheWindowStateIsKeptAwayFromTheWorlds(t *testing.T) {
	path := WindowStatePath(filepath.Join("C:", "Users", "p", "AppData", "Roaming"))
	if filepath.Base(path) != WindowStateFile {
		t.Fatalf("the file is %q", path)
	}
	if filepath.Base(filepath.Dir(path)) != WindowStateDir {
		t.Fatalf("the folder is %q", filepath.Dir(path))
	}
	lower := strings.ToLower(path)
	for _, forbidden := range []string{"profiles", "bibitesmultiverse\\data", "journal"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("the window's own file is under %q: %s", forbidden, path)
		}
	}
}

func TestWindowStateRoundTrips(t *testing.T) {
	state := WindowState{X: 120, Y: 80, Width: 1200, Height: 800, Maximized: true, Details: true,
		Split: map[string]string{SplitNameDetails: "612 244"}}
	raw, err := state.Encode()
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if !strings.Contains(string(raw), WindowStateFormat) {
		t.Fatalf("the file does not say what it is: %s", raw)
	}
	back, ok := DecodeWindowState(raw)
	if !ok {
		t.Fatalf("a file this program wrote was not readable: %s", raw)
	}
	back.Format = ""
	state.Format = ""
	if back.X != state.X || back.Y != state.Y || back.Width != state.Width ||
		back.Height != state.Height || back.Maximized != state.Maximized ||
		back.Details != state.Details {
		t.Fatalf("read back %+v, wrote %+v", back, state)
	}
	if len(back.Split) != 1 || back.Split[SplitNameDetails] != "612 244" {
		t.Fatalf("the dividers read back as %v", back.Split)
	}
}

// EVERY FAILURE IS "NO STATE". Nothing about a window position is worth refusing
// to open a window over.
func TestAnUnreadableWindowStateIsSimplyIgnored(t *testing.T) {
	for _, raw := range []string{
		"",
		"not json at all",
		`{"format":"something/else/1","width":900,"height":600}`,
		`{"width":900}`,
		`[]`,
	} {
		if _, ok := DecodeWindowState([]byte(raw)); ok {
			t.Fatalf("%q was accepted as a window state", raw)
		}
	}
}

// THE CASE THIS EXISTS FOR is a laptop: the window is closed while docked to a
// monitor on the right, the laptop is undocked, and the saved position is now
// off the edge of the world — a window nobody can reach, drag or close.
func TestAWindowIsNeverRestoredOffTheScreen(t *testing.T) {
	const screenW, screenH = 1920, 1080
	cases := []struct {
		name  string
		state WindowState
		want  bool
	}{
		{"where it was", WindowState{X: 100, Y: 100, Width: 1200, Height: 800}, true},
		{"on a monitor that is no longer there", WindowState{X: 3000, Y: 100, Width: 1200, Height: 800}, false},
		{"above the desktop", WindowState{X: 100, Y: -900, Width: 1200, Height: 800}, false},
		{"below the desktop", WindowState{X: 100, Y: 1200, Width: 1200, Height: 800}, false},
		{"off to the left", WindowState{X: -1300, Y: 100, Width: 1200, Height: 800}, false},
		{"a little off the left, still grabbable", WindowState{X: -200, Y: 40, Width: 1200, Height: 800}, true},
		{"no size at all", WindowState{X: 100, Y: 100}, false},
	}
	for _, test := range cases {
		if _, ok := test.state.Fit(0, 0, screenW, screenH); ok != test.want {
			t.Fatalf("%s: usable = %v, want %v", test.name, ok, test.want)
		}
	}

	// A SECOND MONITOR TO THE LEFT puts real, reachable positions at negative
	// coordinates. A rule that assumed the desktop started at 0,0 would throw
	// away exactly the position a two-monitor participant had chosen.
	twoMonitors := WindowState{X: -1800, Y: 60, Width: 1200, Height: 800}
	if _, ok := twoMonitors.Fit(-1920, 0, 3840, 1080); !ok {
		t.Fatal("a window on the left-hand monitor was treated as off the screen")
	}
	if _, ok := twoMonitors.Fit(0, 0, 1920, 1080); ok {
		t.Fatal("that same window was restored after the left-hand monitor went away")
	}

	// A size below the window's own minimum is raised to it, and one bigger than
	// the screen is cut down to it — neither is a reason to throw the position
	// away.
	small, ok := WindowState{X: 10, Y: 10, Width: 200, Height: 100}.Fit(0, 0, screenW, screenH)
	if !ok || small.Width != MinWindowWidth || small.Height != MinWindowHeight {
		t.Fatalf("a tiny saved size became %+v (usable %v)", small, ok)
	}
	huge, ok := WindowState{X: 0, Y: 0, Width: 9000, Height: 9000}.Fit(0, 0, screenW, screenH)
	if !ok || huge.Width != screenW || huge.Height != screenH {
		t.Fatalf("an oversized saved size became %+v (usable %v)", huge, ok)
	}

	// A screen this program could not measure is not one to place a window on.
	if _, ok := (WindowState{X: 0, Y: 0, Width: 1000, Height: 700}).Fit(0, 0, 0, 0); ok {
		t.Fatal("a window was placed on a screen of no size")
	}
}

// THE DIVIDERS ARE REMEMBERED IN THIS FILE and not in a second one. walk knows
// how to save a splitter's position but ships an INI of its own to save it into;
// this keeps the strings walk writes beside the size and the position, so there
// is one file to find, to document and to delete.
//
// THEY ARE KEYED SEPARATELY, which is what keeps the two splitters in this
// window apart: the details divider and the world list's, nested inside it. A
// single shared value was handed to both by the first attempt at this, so
// restoring the height of the details pane also set the width of the world list
// to it.
func TestTheDividersTravelInTheSameFile(t *testing.T) {
	raw, err := WindowState{X: 0, Y: 0, Width: 1200, Height: 800, Split: map[string]string{
		SplitNameDetails: "612 244",
		SplitNameWorlds:  "300 900",
	}}.Encode()
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if !strings.Contains(string(raw), `"details": "612 244"`) {
		t.Fatalf("the divider is not in the file: %s", raw)
	}
	back, ok := DecodeWindowState(raw)
	if !ok || back.Split[SplitNameDetails] != "612 244" || back.Split[SplitNameWorlds] != "300 900" {
		t.Fatalf("read back %v", back.Split)
	}

	// It is ADDITIVE: a file written by the build before this one still opens,
	// and simply has no dividers in it.
	older := `{"format":"` + WindowStateFormat + `","x":10,"y":10,"width":1000,"height":700,"details":true}`
	back, ok = DecodeWindowState([]byte(older))
	if !ok {
		t.Fatal("a file from the previous build was refused")
	}
	if len(back.Split) != 0 || !back.Details || back.Width != 1000 {
		t.Fatalf("an older file read back as %+v", back)
	}

	// A window that never opened the pane writes no divider rather than a zero
	// one: walk stores the height of every child including a hidden one, and a
	// saved zero would open the pane at no height forever.
	empty, err := WindowState{X: 0, Y: 0, Width: 1200, Height: 800}.Encode()
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if strings.Contains(string(empty), "split") {
		t.Fatalf("an unopened pane wrote a divider: %s", empty)
	}
}

// THE FILE IS SOMETHING A PARTICIPANT CAN OPEN, so the names in it are this
// program's and not walk's. walk keys a splitter's state by a path built from
// the name of every window between the form and the widget, which came out as
// "main/clientComposite/details" — correct, stable, and a piece of another
// package's furniture in somebody's preferences.
func TestTheDividerNamesAreOursAndNotWalksPath(t *testing.T) {
	cases := map[string]string{
		"main/clientComposite/details":                         SplitNameDetails,
		"main/clientComposite/details/worlds":                  SplitNameWorlds,
		"whatever/walk/decides/to/call/it/" + SplitNameDetails: SplitNameDetails,
		SplitNameWorlds: SplitNameWorlds,
		"":              "",
	}
	for key, want := range cases {
		if got := SplitAlias(key); got != want {
			t.Fatalf("SplitAlias(%q) = %q, want %q", key, got, want)
		}
	}
	// The two names must differ, because it is the last segment alone that tells
	// the two splitters apart.
	if SplitNameDetails == SplitNameWorlds || SplitNameDetails == "" || SplitNameWorlds == "" {
		t.Fatalf("the splitter names are %q and %q", SplitNameDetails, SplitNameWorlds)
	}
}

// A CLOSED PANE IS NOT A MEASUREMENT. walk writes the pixel height of every
// child of a splitter including a hidden one, and a hidden one measures zero —
// so a position saved while the details pane was closed would re-open it at no
// height forever, with its divider sitting on the bottom edge of the window and
// nothing above it to grab.
func TestOnlyRealDividerPositionsAreKept(t *testing.T) {
	got := UsableSplit(map[string]string{
		SplitNameDetails: "612 244",
		SplitNameWorlds:  "300 900",
	})
	if len(got) != 2 {
		t.Fatalf("two good positions became %v", got)
	}

	// The case this exists for: the pane was never opened, so walk measured it
	// as nothing.
	got = UsableSplit(map[string]string{
		SplitNameDetails: "856 0",
		SplitNameWorlds:  "300 900",
	})
	if _, ok := got[SplitNameDetails]; ok {
		t.Fatalf("a pane of no height was kept: %v", got)
	}
	if got[SplitNameWorlds] != "300 900" {
		t.Fatalf("the other divider was dropped with it: %v", got)
	}

	// Anything that is not two or more positive numbers is not a position.
	for _, junk := range []string{"", "   ", "600", "600 abc", "600 -4", "0 0"} {
		if kept := UsableSplit(map[string]string{SplitNameDetails: junk}); kept != nil {
			t.Fatalf("%q was kept as a divider position: %v", junk, kept)
		}
	}
	// Nothing at all writes nothing at all, so the field stays out of the file.
	if UsableSplit(nil) != nil {
		t.Fatal("an empty set of dividers became a value")
	}
}

// AN UPGRADE MUST NOT LEAVE TWO SPELLINGS IN THE FILE. Until this release the
// dividers were stored under walk's own widget path; a machine upgrading found
// "main/clientComposite/details" sitting in launcher-window.json beside the new
// "details", and the stale pair would have stayed there forever.
func TestAnOlderFilesDividerNamesAreMigratedAndTheStaleOnesDropped(t *testing.T) {
	// Exactly what the machine found after an upgrade.
	upgraded := UsableSplit(map[string]string{
		"main/clientComposite/details":        "700 200",
		"main/clientComposite/details/worlds": "280 900",
		SplitNameDetails:                      "612 244",
		SplitNameWorlds:                       "300 900",
	})
	if len(upgraded) != 2 {
		t.Fatalf("an upgraded file kept %d entries: %v", len(upgraded), upgraded)
	}
	// The CURRENT spelling wins, and it wins every time rather than depending on
	// which order Go happened to walk the map in.
	for i := 0; i < 50; i++ {
		again := UsableSplit(map[string]string{
			"main/clientComposite/details": "700 200",
			SplitNameDetails:               "612 244",
		})
		if again[SplitNameDetails] != "612 244" {
			t.Fatalf("the current spelling lost to the old one: %v", again)
		}
	}

	// A file holding ONLY the old spelling keeps its positions, under the new
	// names: an upgrade must not throw away where somebody put their dividers.
	migrated := UsableSplit(map[string]string{
		"main/clientComposite/details":        "700 200",
		"main/clientComposite/details/worlds": "280 900",
	})
	if migrated[SplitNameDetails] != "700 200" || migrated[SplitNameWorlds] != "280 900" {
		t.Fatalf("an old file's positions were lost: %v", migrated)
	}
	for key := range migrated {
		if strings.Contains(key, "/") {
			t.Fatalf("a walk widget path survived into the file: %q", key)
		}
	}

	// A divider this build does not have is dropped rather than carried around.
	pruned := UsableSplit(map[string]string{
		SplitNameDetails: "612 244",
		"somethingElse":  "100 200",
		"":               "100 200",
	})
	if len(pruned) != 1 || pruned[SplitNameDetails] != "612 244" {
		t.Fatalf("an unknown divider was kept: %v", pruned)
	}

	// And it happens on the way IN, so a file written by an older build stops
	// carrying that build's spelling the first time this one reads it.
	older := `{"format":"` + WindowStateFormat + `","x":10,"y":10,"width":1000,"height":700,` +
		`"split":{"main/clientComposite/details":"700 200","details":"612 244"}}`
	back, ok := DecodeWindowState([]byte(older))
	if !ok {
		t.Fatal("a file from the previous build was refused")
	}
	if len(back.Split) != 1 || back.Split[SplitNameDetails] != "612 244" {
		t.Fatalf("reading an upgraded file gave %v", back.Split)
	}
}
