package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
