package launchergui

// WHERE THE WINDOW WAS LAST TIME.
//
// A window that opens in the middle of the screen at the size its author chose,
// every time, is a window somebody drags and resizes every time. So its size,
// its position and whether the details pane was open are written down when it
// closes and read back when it opens.
//
// WHERE IT IS NOT WRITTEN, AND WHY THAT MATTERS MORE THAN WHERE IT IS:
//
//	NOT IN A WORLD'S DATA ROOT. That folder is this machine's record of every
//	organism it is holding for somebody else. Nothing about a window belongs in
//	the same place as a journal, and a participant told to copy or to keep a data
//	folder must not be copying a window position with it.
//	NOT IN THE INSTALL ROOT'S profiles\ FOLDER. Every .json in there is read as a
//	world, and one that will not parse as a world is reported as a problem the
//	launcher could not read — so a window-position file dropped in there would
//	raise the red banner on every start. (Install.loadProfiles, and BannerFor.)
//
// So it is in the user's own roaming application data, which is what that folder
// is for: a preference, belonging to one person, that no other program needs and
// that losing costs nothing.

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
)

// WindowStateFormat is the schema string the file carries, for the same reason
// a profile carries one: a file that does not say what it is gets ignored rather
// than guessed at.
const WindowStateFormat = "bibites-multiverse/launcher-window/1"

// WindowStateDir and WindowStateFile are the folder and the name under the
// user's roaming application data. The folder is the product's own, so an
// uninstall or a person cleaning up has one obvious thing to remove.
const (
	WindowStateDir  = "Bibites Multiverse"
	WindowStateFile = "launcher-window.json"
)

// WindowState is everything about the window that is worth remembering, and
// nothing about the worlds. Losing this file loses a window position.
type WindowState struct {
	Format string `json:"format"`
	// X and Y are the top-left corner in screen pixels, and Width and Height the
	// size when the window is NOT maximised — which is what Windows itself keeps
	// in a WINDOWPLACEMENT, so a window restored from maximised lands where it
	// was rather than filling the screen forever.
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximized bool `json:"maximized"`
	// Details is whether the details pane was open. It is remembered because
	// somebody who works with it open is working with it open.
	Details bool `json:"details"`
	// Split is where the window's dividers were left: the one above the details
	// pane, and the one between the world list and the panel.
	//
	// IT IS WALK'S OWN STATE, KEY AND ALL. Marking a Splitter Persistent makes
	// walk read and write one string per splitter through walk.App().Settings(),
	// keyed by a path it builds from the window's and the widget's names. This
	// field is that whole map, so nothing in this program has to guess at either
	// the key or the encoding — see splitSettings in window_windows.go, which is
	// the walk.Settings that puts it here rather than in an INI file of walk's
	// own under a second folder.
	//
	// HEIGHTS AND NOT FRACTIONS, because that is what walk stores; a window
	// re-opened on a different screen therefore gets the same-sized pane rather
	// than the same-shaped one, and dragging it once fixes that forever.
	Split map[string]string `json:"split,omitempty"`
}

// WindowStatePath is the file, given the user's configuration folder — which on
// Windows is %APPDATA%, from os.UserConfigDir.
func WindowStatePath(configDir string) string {
	return filepath.Join(configDir, WindowStateDir, WindowStateFile)
}

// Encode writes the state, always stamped with the format.
func (s WindowState) Encode() ([]byte, error) {
	s.Format = WindowStateFormat
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// DecodeWindowState reads one back. EVERY FAILURE IS "NO STATE": a file that is
// missing, truncated, from another program or from a format this build does not
// know answers false, and the window opens at its default size. Nothing here is
// worth a message to a participant, and nothing here is worth refusing to open
// a window over.
func DecodeWindowState(raw []byte) (WindowState, bool) {
	var state WindowState
	if err := json.Unmarshal(raw, &state); err != nil {
		return WindowState{}, false
	}
	if state.Format != WindowStateFormat {
		return WindowState{}, false
	}
	return state, true
}

// Fit answers where the window should actually open, given the screen it is
// opening on.
//
// THE CASE THIS EXISTS FOR is a laptop: the window is closed while docked to a
// second monitor to the right, the laptop is undocked, and the saved position is
// now off the edge of the world — a window nobody can reach, drag or close. So a
// saved rectangle is used only when a usable piece of it is on the screen, and
// its size is capped to the screen even when its position is fine.
//
// THE SCREEN IS THE VIRTUAL ONE — every monitor, as one rectangle, which is why
// it is given an origin as well as a size: a second monitor to the LEFT of the
// primary one puts real, reachable window positions at negative coordinates, and
// a rule that assumed the desktop started at 0,0 would throw away exactly the
// position a two-monitor participant had chosen.
//
// A size smaller than the window's own minimum is raised to it rather than
// rejected: a person who dragged it that small on a bigger screen still meant to
// have it small.
func (s WindowState) Fit(screenX, screenY, screenWidth, screenHeight int) (WindowState, bool) {
	if screenWidth <= 0 || screenHeight <= 0 {
		return WindowState{}, false
	}
	if s.Width <= 0 || s.Height <= 0 {
		return WindowState{}, false
	}
	fitted := s
	fitted.Width = clamp(s.Width, MinWindowWidth, screenWidth)
	fitted.Height = clamp(s.Height, MinWindowHeight, screenHeight)

	// ENOUGH OF THE TITLE BAR TO GRAB. A window whose only visible pixels are its
	// bottom-right corner is a window that cannot be moved back.
	const reachable = 120
	if fitted.X+fitted.Width < screenX+reachable || fitted.X > screenX+screenWidth-reachable {
		return WindowState{}, false
	}
	// The title bar must be BELOW the top of the desktop — a window dragged above
	// it has no bar left to grab — and above the bottom of it.
	if fitted.Y < screenY || fitted.Y > screenY+screenHeight-40 {
		return WindowState{}, false
	}
	return fitted, true
}

func clamp(value, low, high int) int {
	if high < low {
		return low
	}
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// UsableSplit is which of walk's remembered divider positions are worth keeping.
//
// A CLOSED PANE IS NOT A MEASUREMENT. walk writes the pixel height of every
// child of a splitter, INCLUDING A HIDDEN ONE, and a hidden one measures zero —
// so saving while the details pane was closed would record a pane of no height
// and re-open it that way forever, with a divider sitting on the bottom edge of
// the window and nothing above it to grab.
//
// The whole entry is dropped rather than repaired, because the file is seeded
// back into walk's settings before walk ever writes to it: dropping keeps the
// last position the pane had while it was open, which is the one somebody chose.
func UsableSplit(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		if key == "" || !usableSizes(value) {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// usableSizes answers whether every one of walk's space-separated child sizes is
// a real measurement.
func usableSizes(state string) bool {
	fields := strings.Fields(state)
	if len(fields) < 2 {
		return false
	}
	for _, field := range fields {
		size, err := strconv.Atoi(field)
		if err != nil || size <= 0 {
			return false
		}
	}
	return true
}
