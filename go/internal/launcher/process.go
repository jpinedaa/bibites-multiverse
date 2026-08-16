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

// forceStop is the last resort on both platforms. It is TerminateProcess on
// Windows and SIGKILL on Unix, so whatever the process was writing is lost —
// which is exactly why gracefulStop is tried first.
func forceStop(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	defer process.Release()
	return process.Kill()
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

// stopProcess is the shared game/sidecar stop: identify, ask, wait, force, and
// remove the record only when the process is actually gone.
func stopProcess(pidFile, wantExe, what string, timeout time.Duration, now func() time.Time,
	report func(string, ...any), warn func(string, ...any)) {

	pid := readPidFile(pidFile)
	if pid == 0 {
		os.Remove(pidFile)
		return
	}
	switch probeProcess(pid) {
	case processDead:
		report("%s was not running (pid %d is gone)", what, pid)
		os.Remove(pidFile)
		return
	case processOpaque:
		warn("pid %d cannot be inspected from here: %s may be running as another user or "+
			"elevated. Stop it from the same account that started it", pid, what)
	}
	if identifyProcess(pid, wantExe) == identityMismatch {
		warn("pid %d is no longer %s - the operating system has given that number to another "+
			"program. The stale record was removed and NOTHING was stopped", pid, what)
		os.Remove(pidFile)
		return
	}

	forced := false
	reason := ""
	if err := gracefulStop(pid); err != nil {
		// A headless game has no window to close, so this is expected there:
		// taskkill answers "can only be terminated forcefully". Force at once
		// rather than spend the whole timeout waiting for an answer nobody can
		// give.
		forced = true
		reason = "immediately (there was nothing to ask)"
	} else if !waitForExit(pid, timeout, now) {
		forced = true
		reason = fmt.Sprintf("after %s", timeout)
	}
	if forced {
		if err := forceStop(pid); err != nil {
			warn("could not stop %s (pid %d): %v. Its record was kept, so it can still be found",
				what, pid, err)
			return
		}
		if !waitForExit(pid, forceSettleTimeout, now) {
			warn("%s (pid %d) did not exit after being forced. Its record was kept", what, pid)
			return
		}
		report("stopped %s (pid %d), forced %s", what, pid, reason)
	} else {
		report("stopped %s (pid %d)", what, pid)
	}
	os.Remove(pidFile)
}
