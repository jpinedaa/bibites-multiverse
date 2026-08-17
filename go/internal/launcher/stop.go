package launcher

import (
	"os"
	"time"
)

// Stopping a world: the game first, then the sidecar, which is the order the
// generated Stop-Multiverse.ps1 uses.
//
// GRACEFUL FIRST IS A DELIBERATE CHANGE. The generated script used
// Stop-Process -Force, which is TerminateProcess and therefore skips Unity's
// OnApplicationQuit — so save-on-quit did not run on Windows, while the Linux
// script waited for it. The launcher asks the process to close, waits, and
// forces only when the ask did not work. When the ask CANNOT work — a headless
// game has no window for taskkill to close — the force is immediate rather than
// after a timeout nobody is waiting on.
//
// THAT INTENT DID NOT REACH THE MACHINE UNTIL taskkillGracefulArgs. The ask
// carried /T, which walks the game's process tree, meets the windowless
// UnityCrashHandler64.exe the game always spawns, and refuses the whole call —
// so EVERY Windows stop, windowed or headless, was reported as "forced
// immediately (there was nothing to ask)" and every save-on-quit was skipped.
//
// AND A HEADLESS WORLD IS NOT ASKED THROUGH A WINDOW AT ALL. It is asked through
// its own mod, in modquit.go, which is what makes a headless stop lossless. The
// window remains the fallback, for a world whose mod is not there to ask.
const (
	defaultGameStopTimeout    = 30 * time.Second
	defaultSidecarStopTimeout = 10 * time.Second
)

type stopOptions struct {
	gameOnly bool
	timeout  time.Duration
}

// runStop stops one world.
func (a *app) runStop(p Profile, opts stopOptions) int {
	if err := os.MkdirAll(p.LogDir(), 0o755); err != nil {
		return a.fail("could not create %s: %v", p.LogDir(), err)
	}
	events := newEventLog(p.LauncherLog(), a.now)
	held, err := acquireLock(p.LockFile(), a.now, func(format string, args ...any) {
		a.warn(format, args...)
		events.event("warn", "lock.broken")
	})
	if err != nil {
		return a.fail("%v", err)
	}
	defer held.release()

	gameTimeout := defaultGameStopTimeout
	sidecarTimeout := defaultSidecarStopTimeout
	if opts.timeout > 0 {
		gameTimeout = opts.timeout
		sidecarTimeout = opts.timeout
	}

	report := func(format string, args ...any) { a.print(format, args...) }
	warn := func(format string, args ...any) { a.warn(format, args...) }
	outcome := stopProcess(stopRequest{
		pidFile:  p.GamePidFile(),
		wantExe:  p.GameExe(),
		what:     "the game",
		timeout:  gameTimeout,
		now:      a.now,
		askFirst: func() bool { return a.askWorldToQuit(p, events) },
		report:   report,
		warn:     warn,
	})
	events.event("info", "game.stopped", "world", p.World)
	// Only the game has a save to lose, and only the no-window case loses it.
	if outcome == stopForcedNoWindow {
		a.warn("%s", headlessStopNote)
		events.event("warn", "game.forced", "world", p.World, "why", "no-window")
	}

	if opts.gameOnly {
		a.print("the world is down; the sidecar keeps its place and its journal.")
		a.print("Arrivals accumulate there and are delivered, paced, when the world comes back.")
		return exitOK
	}

	// The sidecar has no mod and no window; it is asked the only way there is.
	stopProcess(stopRequest{
		pidFile: p.SidecarPidFile(),
		wantExe: a.install.SidecarExe(),
		what:    "the sidecar",
		timeout: sidecarTimeout,
		now:     a.now,
		report:  report,
		warn:    warn,
	})
	events.event("info", "sidecar.stopped", "world", p.World)
	a.say("The journal in %s is kept. Do not delete it: it is this machine's record of every "+
		"organism it is holding for somebody.", p.DataDir())
	return exitOK
}

// headlessStopNote is what a person needs after a game was killed rather than
// closed: the reason, and the two ways out. It is LOCAL-HEADLESSSTOP in
// docs/error-taxonomy.md, said here at the moment it happens.
const headlessStopNote = `  That game had no window, which is what -batchmode -nographics removes, so there was
  no close request to post to it and it was stopped the only way left. Everything since
  its last save is gone. Give a headless world a short save interval so that is a small
  amount ('profile set NAME --save-minutes 5'), or run the session with a window
  ('start --no-headless') and stop it normally. This is LOCAL-HEADLESSSTOP in
  docs/error-taxonomy.md.`

// runStopAll stops every world this installation knows about. A file in
// profiles\ that cannot be read is REPORTED AND SKIPPED rather than fatal: one
// stray file must not leave a participant unable to shut running worlds down.
func (a *app) runStopAll(opts stopOptions) int {
	profiles, problems := a.install.loadProfiles()
	for _, problem := range problems {
		a.warn("skipping a profile that could not be read: %v", problem)
	}
	if len(profiles) == 0 {
		if len(problems) > 0 {
			return a.fail("no world profile in %s could be read", a.install.ProfilesDir())
		}
		return a.fail("%v", errNoProfiles)
	}
	code := exitOK
	for _, p := range profiles {
		a.print("--- %s", p.Name)
		if result := a.runStop(p, opts); result != exitOK {
			code = result
		}
	}
	return code
}
