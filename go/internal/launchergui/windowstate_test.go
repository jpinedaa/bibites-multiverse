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
	state := WindowState{X: 120, Y: 80, Width: 1200, Height: 800, Maximized: true, Details: true}
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
	if back != state {
		t.Fatalf("read back %+v, wrote %+v", back, state)
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
