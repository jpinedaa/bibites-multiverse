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
	stopProcess(p.GamePidFile(), p.GameExe(), "the game", gameTimeout, a.now, report, warn)
	events.event("info", "game.stopped", "world", p.World)

	if opts.gameOnly {
		a.print("the world is down; the sidecar keeps its place and its journal.")
		a.print("Arrivals accumulate there and are delivered, paced, when the world comes back.")
		return exitOK
	}

	stopProcess(p.SidecarPidFile(), a.install.SidecarExe(), "the sidecar", sidecarTimeout,
		a.now, report, warn)
	events.event("info", "sidecar.stopped", "world", p.World)
	a.say("The journal in %s is kept. Do not delete it: it is this machine's record of every "+
		"organism it is holding for somebody.", p.DataDir())
	return exitOK
}

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
