package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The PID ledger is a cross-tool contract, not an implementation detail: the
// generated Start-Multiverse.ps1 and Stop-Multiverse.ps1 write and read the
// same two files, and go/internal/sidecar/ownslot.go documents the convention.
// One decimal pid, ASCII, on the first line.
//
// A NUMBER IS NOT AN IDENTITY. A pid file lives under a world's data root, so
// it outlives crashes, force-kills and reboots, and the operating system hands
// the same number out again — Windows within minutes of a boot. Every read of
// the ledger therefore asks two questions: is a process with this number alive,
// and is it the program this file claims. A mismatch is a STALE RECORD: it is
// removed and nothing is signalled. Without that second question, `stop` on a
// stale record kills a stranger, and on Windows `taskkill /T` takes its whole
// process tree with it.

// processState is what the operating system will say about a pid.
type processState int

const (
	// processDead: no process carries this number.
	processDead processState = iota
	// processRunning: it is there and this account can inspect it.
	processRunning
	// processOpaque: it is there, but this account cannot inspect it — the
	// world was started elevated, or by another user. A pid that cannot be
	// opened is a pid that exists, and treating it as gone would orphan the
	// process and lose the ledger entry that names it.
	processOpaque
)

// processIdentity is the answer to "is this the program the ledger claims".
type processIdentity int

const (
	// identityUnknown: the image could not be read. Nothing is concluded.
	identityUnknown processIdentity = iota
	identityMatches
	identityMismatch
)

// readPidFile returns 0 when there is no usable pid on record, which every
// caller treats as "nothing is running here".
func readPidFile(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	line := raw
	if cut := indexAny(raw, "\r\n"); cut >= 0 {
		line = raw[:cut]
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(line)))
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

// writePidFile records a pid in the shared format.
func writePidFile(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

// processAlive is the plain liveness question, with no identity check. Only the
// wait loops use it, where the pid was already identified.
func processAlive(pid int) bool {
	return probeProcess(pid) != processDead
}

// identifyProcess compares the running image against the executable the ledger
// claims. It never guesses: an image it cannot read is identityUnknown.
func identifyProcess(pid int, wantExe string) processIdentity {
	if pid <= 0 || wantExe == "" {
		return identityUnknown
	}
	image, err := processImagePath(pid)
	if err != nil || image == "" {
		return identityUnknown
	}
	if imageMatches(image, wantExe) {
		return identityMatches
	}
	return identityMismatch
}

// imageMatches compares a running process's image with an expected executable.
// The full path is the strong form; the base name is the fallback, because a
// game or a sidecar moved to another folder is still that program. Linux's
// /proc comm is truncated to 15 characters, so a truncated prefix counts.
func imageMatches(image, wantExe string) bool {
	if image == "" || wantExe == "" {
		return false
	}
	if pathsEqual(image, wantExe) {
		return true
	}
	got := strings.ToLower(filepath.Base(image))
	want := strings.ToLower(filepath.Base(wantExe))
	if got == want {
		return true
	}
	if len(got) >= 15 && strings.HasPrefix(want, got) {
		return true
	}
	return false
}

// livePid returns the recorded pid when that process is still running AND is
// still the program the file claims. Everything else answers 0.
func livePid(pidFile, wantExe string) int {
	pid, running := describePid(pidFile, wantExe)
	if !running {
		return 0
	}
	return pid
}

// describePid is the read-only form the status output uses. It never removes a
// file: reporting must not mutate the ledger.
func describePid(pidFile, wantExe string) (int, bool) {
	pid := readPidFile(pidFile)
	if pid == 0 {
		return 0, false
	}
	if probeProcess(pid) == processDead {
		return pid, false
	}
	if identifyProcess(pid, wantExe) == identityMismatch {
		return pid, false
	}
	return pid, true
}

// pidState is the words the menu and the text status print.
func pidState(pidFile, wantExe string) string {
	pid, running := describePid(pidFile, wantExe)
	if !running {
		return "stopped"
	}
	return fmt.Sprintf("running (pid %d)", pid)
}

// askResult is what the GRACEFUL phase of a stop learned. It is the difference
// between "it was asked, and is closing", "there is nothing here that can be
// asked" and "the ask itself broke" — three different things to say to somebody
// watching a world shut down, and three different reasons to reach for the
// force.
type askResult int

const (
	// askAccepted: the close request was delivered. The process may still take
	// as long as its own shutdown takes.
	askAccepted askResult = iota
	// askImpossible: there is no window to post a close request to, so waiting
	// for a graceful exit is waiting for an answer nobody can give.
	askImpossible
	// askFailed: the ask could not be made, for a reason that is not "no window".
	askFailed
)

// The two taskkill command lines the Windows stop uses. They are HERE rather
// than in proc_windows.go so a test on any machine can hold them to the rule
// that used to cost a save on every stop.
//
// THE GRACEFUL PHASE MUST NOT PASS /T. taskkill /T walks the process TREE, and
// it refuses the WHOLE call when any member of that tree needs /F. The game
// always spawns UnityCrashHandler64.exe --attach <game pid>, which is
// windowless, so /T failed on the crash handler ("can only be terminated
// forcefully") and then declined to touch the game itself. The launcher read
// that as "there was nothing to ask" and force-killed a game that had a window
// and a world to save. Without /T, taskkill posts WM_CLOSE to the game, and the
// game's own save-on-quit runs.
func taskkillGracefulArgs(pid int) []string {
	return []string{"/PID", strconv.Itoa(pid)}
}

// THE FORCE PHASE MUST PASS /T. It is the last resort, and the crash handler is
// exactly the child that has to go with its parent rather than linger holding a
// dead --attach target.
func taskkillForceArgs(pid int) []string {
	return []string{"/PID", strconv.Itoa(pid), "/T", "/F"}
}

// taskkillNeedsForce is taskkill's own words for "this process has no window, so
// there is nothing to post a close request to". A headless game (-batchmode
// -nographics) answers it, and so does the sidecar, which is started detached
// and owns no console.
const taskkillNeedsForce = "can only be terminated forcefully"

// classifyTaskkill turns taskkill's exit into the three answers a stop needs.
func classifyTaskkill(err error, output string) askResult {
	if err == nil {
		return askAccepted
	}
	if strings.Contains(output, taskkillNeedsForce) {
		return askImpossible
	}
	return askFailed
}

// waitForExit polls until the process is gone or the deadline passes. Polling
// is the only portable answer: the process being stopped is usually not a
// child of this one.
func waitForExit(pid int, timeout time.Duration, now func() time.Time) bool {
	deadline := now().Add(timeout)
	for {
		if !processAlive(pid) {
			return true
		}
		if !now().Before(deadline) {
			return false
		}
		time.Sleep(exitPollInterval)
	}
}

// exitPollInterval is a variable so a test does not spend real seconds waiting
// for a fake process to die.
var exitPollInterval = 100 * time.Millisecond

// forceSettleTimeout bounds the wait after a forced stop. A process that
// survives TerminateProcess or SIGKILL keeps its ledger entry, so a person can
// still find it.
const forceSettleTimeout = 5 * time.Second

// indexAny reports the first position in b holding any byte of set.
func indexAny(b []byte, set string) int {
	for i, c := range b {
		if strings.IndexByte(set, c) >= 0 {
			return i
		}
	}
	return -1
}

// stopOutcome is what a stop actually did. It is returned because the caller has
// something to add to some of them and nothing to add to others: only a game
// that had no window to close has lost anything.
type stopOutcome int

const (
	// stopNothingRecorded: the ledger named nothing.
	stopNothingRecorded stopOutcome = iota
	// stopWasNotRunning: the recorded pid is gone.
	stopWasNotRunning
	// stopStaleRecord: the pid belongs to another program now, so NOTHING was
	// signalled.
	stopStaleRecord
	// stopAsked: it was asked to close and it closed. On a game with a window
	// this is the outcome save-on-quit runs in.
	stopAsked
	// stopAskedMod: it was asked through its own mod's quit verb, it answered,
	// and it saved and quit. THE ONLY OUTCOME A HEADLESS WORLD LOSES NOTHING IN,
	// because it is the only ask that does not need a window (modquit.go).
	stopAskedMod
	// stopForcedNoWindow: there was nothing to ask, so the force was immediate.
	stopForcedNoWindow
	// stopForcedTimeout: it was asked, it did not close in time, it was forced.
	stopForcedTimeout
	// stopForcedAskFailed: the ask broke for some other reason, then the force.
	stopForcedAskFailed
	// stopFailed: it is still there and its record was kept.
	stopFailed
)

// stopRequest is one process to stop. It is a struct rather than a parameter
// list because of askFirst: a stop now has TWO ways to ask, and which one is
// available is a property of what is being stopped rather than of how.
type stopRequest struct {
	pidFile string
	wantExe string
	// what names the process in every line a person reads: "the game".
	what    string
	timeout time.Duration
	now     func() time.Time
	// askFirst is an ask better than the operating system's, when there is one:
	// the game's own mod, which saves and quits and needs no window (modquit.go).
	// It answers whether the request was accepted. Nil — the sidecar has no such
	// channel — or false, and the ordinary close request is used instead.
	askFirst func() bool
	report   func(string, ...any)
	warn     func(string, ...any)
}

// stopProcess is the shared game/sidecar stop: identify, ask, wait, force, and
// remove the record only when the process is actually gone.
func stopProcess(req stopRequest) stopOutcome {
	pid := readPidFile(req.pidFile)
	if pid == 0 {
		os.Remove(req.pidFile)
		return stopNothingRecorded
	}
	switch probeProcess(pid) {
	case processDead:
		req.report("%s was not running (pid %d is gone)", req.what, pid)
		os.Remove(req.pidFile)
		return stopWasNotRunning
	case processOpaque:
		req.warn("pid %d cannot be inspected from here: %s may be running as another user or "+
			"elevated. Stop it from the same account that started it", pid, req.what)
	}
	if identifyProcess(pid, req.wantExe) == identityMismatch {
		req.warn("pid %d is no longer %s - the operating system has given that number to another "+
			"program. The stale record was removed and NOTHING was stopped", pid, req.what)
		os.Remove(req.pidFile)
		return stopStaleRecord
	}

	// THE MOD IS ASKED FIRST WHEN THERE IS ONE, window or no window. It is the
	// only ask a headless world can hear, and for a world with a window it is the
	// same shutdown by a more certain route.
	outcome := stopAsked
	if req.askFirst != nil && req.askFirst() {
		outcome = stopAskedMod
	} else {
		result, askErr := gracefulStop(pid)
		switch result {
		case askImpossible:
			// Expected for anything without a window. Force at once rather than
			// spend the whole timeout waiting for an answer nobody can give.
			outcome = stopForcedNoWindow
		case askFailed:
			req.warn("%s (pid %d) could not be asked to close: %v", req.what, pid, askErr)
			outcome = stopForcedAskFailed
		}
	}
	if outcome == stopAsked || outcome == stopAskedMod {
		if waitForExit(pid, req.timeout, req.now) {
			req.report("stopped %s (pid %d) - %s", req.what, pid, askedWords(outcome))
			os.Remove(req.pidFile)
			return outcome
		}
		outcome = stopForcedTimeout
	}
	if err := forceStop(pid); err != nil {
		req.warn("could not stop %s (pid %d): %v. Its record was kept, so it can still be found",
			req.what, pid, err)
		return stopFailed
	}
	if !waitForExit(pid, forceSettleTimeout, req.now) {
		req.warn("%s (pid %d) did not exit after being forced. Its record was kept", req.what, pid)
		return stopFailed
	}
	req.report("stopped %s (pid %d), forced %s", req.what, pid, forcedReason(outcome, req.timeout))
	os.Remove(req.pidFile)
	return outcome
}

// askedWords says WHICH ask worked, because the two are worth different things:
// one is a window message the game may or may not have had time to act on, and
// the other is the mod reporting that it saved.
func askedWords(outcome stopOutcome) string {
	if outcome == stopAskedMod {
		return "it was asked through the mod, and it saved and quit"
	}
	return "it was asked to close, and it closed"
}

// forcedReason says WHY the force happened, which is the whole of what a person
// needs to know about whether anything was lost. "immediately (there was nothing
// to ask)" was printed for a game with a window too, because /T made every
// graceful ask fail; the three cases are told apart here instead.
func forcedReason(outcome stopOutcome, timeout time.Duration) string {
	switch outcome {
	case stopForcedNoWindow:
		return "immediately: it has no window, so there was no close request to post to it"
	case stopForcedAskFailed:
		return "immediately: the close request could not be delivered"
	default:
		return fmt.Sprintf("after %s: it was asked to close and had not closed by then", timeout)
	}
}
