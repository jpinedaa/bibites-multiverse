//go:build linux

package launcher

import (
	"strings"
	"testing"
)

// The per-session headless override is the one thing the window can do that the
// console menu could not: run THIS session with a window, or without one,
// without editing the world. It has to leave the world's own setting alone,
// because a participant who ran one session headless has not asked for every
// session after it to be headless too.
func TestSessionHeadlessOverrideIsForThisSessionOnly(t *testing.T) {
	fastPolling(t)
	h := newHarness(t)
	h.useRealFakes()
	p := h.profile("default", "Multiverse", freeTestPort(t))
	t.Cleanup(func() { killRecorded(p) })

	headless := true
	s := h.session()
	if code := s.Start("default", StartOptions{Headless: &headless}); code != exitOK {
		t.Fatalf("the start exited %d\n%s\n%s", code, h.out(), h.err())
	}
	mustContain(t, "the start output", h.out(), "YOU ARE ON THE MAP:")
	mustContain(t, "the start output", h.out(), "It runs with nothing drawn.")
	// The mod-connected line is what tells a world that is really on the map
	// from a sidecar that merely holds its place, and the window's list reads the
	// same fact from the same endpoint.
	mustContain(t, "the start output", h.out(), "the game joined the map")

	// THE PROFILE DID NOT CHANGE. This is the whole point of the override.
	stored, err := s.World("default")
	if err != nil {
		t.Fatalf("World: %v", err)
	}
	if stored.Headless {
		t.Fatal("a this-session-only override was written into the world")
	}

	snap := s.Snapshot()
	if len(snap.Worlds) != 1 {
		t.Fatalf("the snapshot holds %d worlds", len(snap.Worlds))
	}
	world := snap.Worlds[0]
	if !world.Sidecar.Running || !world.Game.Running {
		t.Fatalf("the snapshot missed a running process: %+v", world)
	}
	if world.Headless {
		t.Fatal("the world list reported the override as the world's setting")
	}
	if !world.Mod.Answered || !world.Mod.Connected || world.Mod.Version == "" {
		t.Fatalf("the world list did not read the mod: %+v", world.Mod)
	}

	// A stop through a session is the lossless one: the mod is asked, and it
	// saves and quits. A headless world has no window, so this is the only ask
	// that can reach it (LOCAL-HEADLESSSTOP).
	waitForFakeGame(t, p)
	h.stdout.Reset()
	h.stderr.Reset()
	if code := s.Stop("default", false, 0); code != exitOK {
		t.Fatalf("the stop exited %d\n%s", code, h.err())
	}
	mustContain(t, "the stop output", h.out(),
		"it was asked through the mod, and it saved and quit")
	if strings.Contains(h.err(), "no close request to post to it") {
		t.Fatalf("a headless world was forced rather than asked:\n%s", h.err())
	}
	if readPidFile(p.GamePidFile()) != 0 || readPidFile(p.SidecarPidFile()) != 0 {
		t.Fatal("the pid ledger survived a clean stop")
	}
}

// Stopping every world is one call, because a window has one button for it and
// a participant who closes down for the night must not have to remember the
// world they are not looking at.
func TestSessionStopAllStopsEveryWorld(t *testing.T) {
	fastPolling(t)
	h := newHarness(t)
	h.useRealFakes()
	first := h.profile("default", "Multiverse", freeTestPort(t))
	second := h.profile("second", "Second", freeTestPort(t))
	t.Cleanup(func() { killRecorded(first); killRecorded(second) })

	s := h.session()
	for _, name := range []string{"default", "second"} {
		if code := s.Start(name, StartOptions{}); code != exitOK {
			t.Fatalf("start %s exited %d\n%s", name, code, h.err())
		}
	}
	waitForFakeGame(t, first)
	waitForFakeGame(t, second)

	h.stdout.Reset()
	if code := s.StopAll(0); code != exitOK {
		t.Fatalf("stop all exited %d\n%s", code, h.err())
	}
	for _, p := range []Profile{first, second} {
		if readPidFile(p.GamePidFile()) != 0 || readPidFile(p.SidecarPidFile()) != 0 {
			t.Fatalf("%s is still recorded as running", p.Name)
		}
	}
	mustContain(t, "the stop-all output", h.out(), "--- default")
	mustContain(t, "the stop-all output", h.out(), "--- second")
}
