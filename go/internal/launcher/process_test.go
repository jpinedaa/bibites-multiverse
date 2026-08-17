package launcher

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestImageMatches is the rule that decides whether a recorded pid is still the
// program the ledger claims. Everything it gets wrong is either a stranger's
// process being killed, or a running world reported as stopped.
func TestImageMatches(t *testing.T) {
	want := filepath.Join("C:", "Program Files", "Bibites Multiverse", "multiverse-sidecar.exe")
	cases := []struct {
		name  string
		image string
		want  string
		match bool
	}{
		{"the same path", want, want, true},
		{"the same path in another case", strings.ToLower(want), want, true},
		{"the same program in another folder",
			filepath.Join("D:", "moved", "multiverse-sidecar.exe"), want, true},
		{"a bare base name", "multiverse-sidecar.exe", want, true},
		{"another program", filepath.Join("C:", "Windows", "System32", "notepad.exe"), want, false},
		{"a shell a fake script would have exec'd", "/usr/bin/sleep",
			"/games/bibites/The Bibites.x86_64", false},
		{"the game", "/games/bibites/The Bibites.x86_64",
			"/games/bibites/The Bibites.x86_64", true},
		// Linux truncates /proc comm to 15 characters, so the fallback answer
		// for "The Bibites.x86_64" is a prefix of it.
		{"a truncated comm", "The Bibites.x8", "/games/bibites/The Bibites.x86_64", false},
		{"a truncated comm at the limit", "The Bibites.x86", "/games/bibites/The Bibites.x86_64", true},
		{"nothing", "", want, false},
		{"nothing expected", want, "", false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := imageMatches(test.image, test.want); got != test.match {
				t.Fatalf("imageMatches(%q, %q) = %v, want %v",
					test.image, test.want, got, test.match)
			}
		})
	}
}

// TestStopCommandLines pins the two Windows command lines. proc_windows.go
// cannot run here, so the thing that broke every save-on-quit — a /T on the
// GRACEFUL ask — is held to its rule where a test can reach it.
func TestStopCommandLines(t *testing.T) {
	graceful := strings.Join(taskkillGracefulArgs(11004), " ")
	if graceful != "/PID 11004" {
		t.Fatalf("the graceful ask is `taskkill %s`, want `taskkill /PID 11004`", graceful)
	}
	if strings.Contains(graceful, "/T") {
		t.Fatal("the graceful ask carries /T, which walks the game's process tree, meets the " +
			"windowless UnityCrashHandler64.exe and refuses the whole call - so the game is " +
			"never asked to close and its save-on-quit never runs")
	}
	if strings.Contains(graceful, "/F") {
		t.Fatal("the graceful ask carries /F, which is TerminateProcess: there is nothing " +
			"graceful about it")
	}
	force := strings.Join(taskkillForceArgs(11004), " ")
	if force != "/PID 11004 /T /F" {
		t.Fatalf("the force is `taskkill %s`, want `taskkill /PID 11004 /T /F`", force)
	}
}

// TestTaskkillClassification uses taskkill's own words, as they came off the
// machine. "can only be terminated forcefully" is not a failure - it is the
// answer "this process has no window", and it means force NOW rather than after
// a timeout nobody is waiting on.
func TestTaskkillClassification(t *testing.T) {
	const noWindow = "ERROR: The process with PID 11004 could not be terminated.\r\n" +
		"Reason: This process can only be terminated forcefully (with /F option)."
	const childRefused = "ERROR: The process with PID 23180 (child process of PID 11004) could " +
		"not be terminated.\r\nReason: This process can only be terminated forcefully (with /F option)."
	const notFound = "ERROR: The process \"11004\" not found."

	cases := []struct {
		name   string
		err    error
		output string
		want   askResult
	}{
		{"it was asked", nil, "SUCCESS: Sent termination signal to the process with PID 11004.", askAccepted},
		{"it has no window", errors.New("exit status 1"), noWindow, askImpossible},
		{"its crash handler has no window", errors.New("exit status 128"), childRefused, askImpossible},
		{"it is not there", errors.New("exit status 128"), notFound, askFailed},
		{"taskkill itself would not run", errors.New("file does not exist"), "", askFailed},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyTaskkill(test.err, test.output); got != test.want {
				t.Fatalf("classifyTaskkill = %v, want %v", got, test.want)
			}
		})
	}
}

// TestForcedReasonTellsTheThreeApart: "forced immediately (there was nothing to
// ask)" was printed for a WINDOWED game too, which told a participant their save
// was lost when it was not - and hid the case where it really was.
func TestForcedReasonTellsTheThreeApart(t *testing.T) {
	noWindow := forcedReason(stopForcedNoWindow, 30*time.Second)
	timedOut := forcedReason(stopForcedTimeout, 30*time.Second)
	askBroke := forcedReason(stopForcedAskFailed, 30*time.Second)

	if noWindow == timedOut || noWindow == askBroke || timedOut == askBroke {
		t.Fatalf("two of the three forced reasons read the same:\n%s\n%s\n%s",
			noWindow, timedOut, askBroke)
	}
	mustContain(t, "the no-window reason", noWindow, "no window")
	mustContain(t, "the timeout reason", timedOut, "30s")
	mustContain(t, "the timeout reason", timedOut, "asked to close")
}

// TestForcedLineTellsTheSidecarFromTheGame. Every sidecar the launcher starts is
// detached and windowless on purpose, so EVERY healthy stop of one used to print
// "forced immediately: it has no window" - the exact sentence
// docs/error-taxonomy.md tells a person to search their log for when a headless
// world has come back missing everything since its last save. The game's wording
// is unchanged, because for a game it is true.
func TestForcedLineTellsTheSidecarFromTheGame(t *testing.T) {
	game := stopRequest{what: "the game", timeout: 30 * time.Second}
	sidecar := stopRequest{what: "the sidecar", timeout: 10 * time.Second,
		noWindowWords: sidecarEndedWords}

	gameLine := forcedLine(game, 4242, stopForcedNoWindow)
	mustContain(t, "the game's no-window line", gameLine,
		"forced immediately: it has no window")

	sidecarLine := forcedLine(sidecar, 4243, stopForcedNoWindow)
	mustContain(t, "the sidecar's no-window line", sidecarLine,
		"stopped the sidecar (pid 4243) - ")
	mustContain(t, "the sidecar's no-window line", sidecarLine, "keeps nothing unsaved")
	if strings.Contains(sidecarLine, "no window") || strings.Contains(sidecarLine, "forced") {
		t.Fatalf("a healthy sidecar stop reads like LOCAL-HEADLESSSTOP: %s", sidecarLine)
	}

	// Only the no-window case is calmer. A sidecar that was asked and would not
	// go is still a force, and still says so.
	timedOut := forcedLine(sidecar, 4243, stopForcedTimeout)
	mustContain(t, "the sidecar's timeout line", timedOut, "forced after 10s")
}

// TestProbeProcessAnswersForThisProcess pins the three states against the two
// pids every machine has: this one, and one nothing is using.
func TestProbeProcessAnswersForThisProcess(t *testing.T) {
	if got := probeProcess(os.Getpid()); got != processRunning {
		t.Fatalf("this process probes as %v, want processRunning", got)
	}
	if got := probeProcess(findDeadPid(t)); got != processDead {
		t.Fatalf("an unused pid probes as %v, want processDead", got)
	}
	if got := probeProcess(0); got != processDead {
		t.Fatalf("pid 0 probes as %v, want processDead", got)
	}
	if got := probeProcess(-1); got != processDead {
		t.Fatalf("pid -1 probes as %v, want processDead", got)
	}
}

// TestIdentifyThisProcess proves the image of a running process can actually be
// read on this platform - the check is worthless if it always answers unknown.
func TestIdentifyThisProcess(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	if got := identifyProcess(os.Getpid(), self); got != identityMatches {
		t.Fatalf("this process identified as %v against its own executable, want identityMatches", got)
	}
	if got := identifyProcess(os.Getpid(), filepath.Join(t.TempDir(), "something-else")); got != identityMismatch {
		t.Fatalf("this process identified as %v against another name, want identityMismatch", got)
	}
	if got := identifyProcess(findDeadPid(t), self); got != identityUnknown {
		t.Fatalf("a dead pid identified as %v, want identityUnknown", got)
	}
}

// TestStalePidFileIsNotLive: a pid file naming a process that is not this
// world's program answers "nothing is running", and reporting never mutates it.
func TestStalePidFileIsNotLive(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, gamePidFileName)
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	if err := writePidFile(pidFile, os.Getpid()); err != nil {
		t.Fatalf("write pid: %v", err)
	}
	if livePid(pidFile, self) != os.Getpid() {
		t.Fatal("a pid file naming this very program was not read as live")
	}
	if livePid(pidFile, filepath.Join(dir, "The Bibites.x86_64")) != 0 {
		t.Fatal("a pid file naming another program was read as live")
	}
	if !fileExists(pidFile) {
		t.Fatal("reading the ledger deleted it")
	}
}
