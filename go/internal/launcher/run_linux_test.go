//go:build linux

package launcher

import (
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// These tests spawn real processes, so they are Linux-only: the Windows
// primitives in proc_windows.go are compile-only on this machine. The fakes are
// copies of this test binary (see TestMain), so the launcher's image-identity
// check sees exactly what it would see with the real sidecar and game.

func TestStartStopWithFakeBinaries(t *testing.T) {
	fastPolling(t)
	h := newHarness(t)
	h.useRealFakes()
	p := h.profile("default", "Multiverse", freeTestPort(t))
	t.Cleanup(func() { killRecorded(p) })

	if code := h.run("start"); code != exitOK {
		t.Fatalf("start: %d\n%s\n%s", code, h.out(), h.err())
	}
	mustContain(t, "the start output", h.out(), "YOU ARE ON THE MAP:")
	mustContain(t, "the start output", h.out(), "contract B: slot granted")

	sidecarPid := readPidFile(p.SidecarPidFile())
	gamePid := readPidFile(p.GamePidFile())
	if sidecarPid == 0 || gamePid == 0 {
		t.Fatalf("the pid ledger is incomplete: sidecar %d game %d", sidecarPid, gamePid)
	}
	if !processAlive(sidecarPid) || !processAlive(gamePid) {
		t.Fatal("a recorded process is not running")
	}
	// The ledger is the shared format the PowerShell scripts read: one decimal
	// pid on the first line.
	if got := readFile(t, p.SidecarPidFile()); got != fmt.Sprintf("%d\n", sidecarPid) {
		t.Fatalf("the pid file holds %q", got)
	}

	// status sees both.
	if code := h.run("status"); code != exitOK {
		t.Fatalf("status: %d\n%s", code, h.err())
	}
	mustContain(t, "the status", h.out(), fmt.Sprintf("running (pid %d)", sidecarPid))
	mustContain(t, "the status", h.out(), fmt.Sprintf("running (pid %d)", gamePid))

	// A second start is refused while the sidecar is up.
	if code := h.run("start"); code != exitRefused {
		t.Fatalf("a second start exited %d, want %d", code, exitRefused)
	}
	mustContain(t, "the refusal", h.err(), "is already running")

	// A world whose port something else holds is refused, against a real bind.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	busy := listener.Addr().(*net.TCPAddr).Port
	third := h.profile("third", "Third", busy)
	t.Cleanup(func() { killRecorded(third) })
	if code := h.run("start", "third"); code != exitRefused {
		t.Fatalf("a start on a busy port exited %d, want %d", code, exitRefused)
	}
	mustContain(t, "the refusal", h.err(), fmt.Sprintf("nothing can listen on 127.0.0.1:%d", busy))

	// Stopping asks before it forces, and clears the ledger.
	if code := h.run("stop", "default"); code != exitOK {
		t.Fatalf("stop: %d\n%s", code, h.err())
	}
	mustContain(t, "the stop output", h.out(), fmt.Sprintf("stopped the game (pid %d)", gamePid))
	mustContain(t, "the stop output", h.out(), fmt.Sprintf("stopped the sidecar (pid %d)", sidecarPid))
	if fileExists(p.SidecarPidFile()) || fileExists(p.GamePidFile()) {
		t.Fatal("stop left a pid file behind")
	}
	if processAlive(sidecarPid) || processAlive(gamePid) {
		t.Fatal("stop left a process running")
	}

	// The launcher's own event log recorded the whole thing, and holds no secret.
	events := readFile(t, p.LauncherLog())
	for _, want := range []string{"sidecar.started", "slot.granted", "game.started",
		"game.stopped", "sidecar.stopped"} {
		mustContain(t, "the launcher log", events, want)
	}
	mustNotContain(t, "the launcher log", events, strings.Repeat("a", 64))
}

// TestStartRefusesWhenTheMapDoesNot proves the wait ends on a refusal rather
// than on the deadline, and that the game is not started.
func TestStartRefusesWhenTheMapDoesNot(t *testing.T) {
	fastPolling(t)
	t.Setenv("LAUNCHER_FAKE_SIDECAR", "refuse")
	h := newHarness(t)
	h.useRealFakes()
	p := h.profile("default", "Multiverse", freeTestPort(t))
	t.Cleanup(func() { killRecorded(p) })

	if code := h.run("start", "--wait", "5"); code != exitRefused {
		t.Fatalf("start exited %d, want %d", code, exitRefused)
	}
	mustContain(t, "the refusal", h.err(), "The map did not grant a place.")
	mustContain(t, "the refusal", h.err(), "placement claim refused")
	mustContain(t, "the refusal", h.err(), "The five usual causes, in order:")
	if fileExists(p.GamePidFile()) {
		t.Fatal("the game was started without a place on the map")
	}
	// The sidecar is deliberately left running, so the next attempt is about the
	// map rather than about the process.
	if readPidFile(p.SidecarPidFile()) == 0 {
		t.Fatal("the sidecar's pid was not recorded")
	}
}

// TestConcurrencyCap: five worlds already run out of one game folder, so the
// sixth is refused and the message names why.
func TestConcurrencyCap(t *testing.T) {
	h := newHarness(t)
	h.useRealFakes()
	for i := 1; i <= MaxLocalWorlds; i++ {
		p := h.profile(fmt.Sprintf("world%d", i), fmt.Sprintf("World %d", i), 9000+i)
		pid := spawnPlaceholder(t, filepath.Join(h.gameDir, gameExeName))
		if err := writePidFile(p.GamePidFile(), pid); err != nil {
			t.Fatalf("write pid: %v", err)
		}
	}
	sixth := h.profile("world6", "World 6", freeTestPort(t))
	t.Cleanup(func() { killRecorded(sixth) })

	if code := h.run("start", "world6"); code != exitRefused {
		t.Fatalf("the sixth world exited %d, want %d", code, exitRefused)
	}
	mustContain(t, "the refusal", h.err(), "LOCAL-STARVATION")
	mustContain(t, "the refusal", h.err(), "docs/error-taxonomy.md")
	mustContain(t, "the refusal", h.err(), h.gameDir)
	if fileExists(sixth.SidecarPidFile()) {
		t.Fatal("the sixth world's sidecar was started anyway")
	}

	// A world in ANOTHER game folder is not capped by those five.
	other := filepath.Join(t.TempDir(), "game2")
	h.makeGameDir(other)
	if code := h.run("profile", "set", "world6", "--game-dir", other); code != exitOK {
		t.Fatalf("profile set: %d\n%s", code, h.err())
	}
	if code := h.run("start", "world6", "--dry-run"); code != exitOK {
		t.Fatalf("the dry run exited %d\n%s", code, h.err())
	}
	mustNotContain(t, "the dry run", h.err(), "LOCAL-STARVATION")
}

// TestNoWriteOutsideOwnedRoots: the launcher writes under <install>/profiles and
// under a world's own data root, and NEVER inside a game folder.
func TestNoWriteOutsideOwnedRoots(t *testing.T) {
	fastPolling(t)
	h := newHarness(t)
	h.useRealFakes()
	h.profile("default", "Multiverse", freeTestPort(t))

	before := snapshot(t, h.gameDir)
	installBefore := snapshot(t, h.root)

	joinFile := filepath.Join(t.TempDir(), "join.txt")
	writeFile(t, joinFile, joinStringPrefix+" wss://relay.example/contract-b/v4 second."+
		strings.Repeat("b", 40)+"\n", 0o600)

	port := freeTestPort(t)
	if code := h.run("profile", "create", "second", "--world", "Second",
		"--sidecar-port", fmt.Sprint(port), "--join-string-file", joinFile); code != exitOK {
		t.Fatalf("profile create: %d\n%s", code, h.err())
	}
	second, err := h.install().LoadProfile("second")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	t.Cleanup(func() { killRecorded(second) })

	if code := h.run("start", "second"); code != exitOK {
		t.Fatalf("start: %d\n%s\n%s", code, h.out(), h.err())
	}
	if code := h.run("stop", "second"); code != exitOK {
		t.Fatalf("stop: %d\n%s", code, h.err())
	}

	if diff := compareSnapshots(before, snapshot(t, h.gameDir)); diff != "" {
		t.Fatalf("the game folder was changed:\n%s", diff)
	}
	installAfter := snapshot(t, h.root)
	for path := range installAfter {
		if installBefore[path] == installAfter[path] {
			continue
		}
		if strings.HasPrefix(path, profilesDirName+string(filepath.Separator)) {
			continue
		}
		t.Fatalf("the launcher wrote %s, which is outside the profiles folder", path)
	}
	// Everything the world itself needs landed under its own data root.
	for _, path := range []string{second.CredentialFile(), second.SidecarLog(), second.LauncherLog()} {
		if !fileExists(path) {
			t.Fatalf("%s was not written", path)
		}
	}
}

// fastPolling shortens the two poll loops so a test does not spend real seconds
// waiting on a fake process.
func fastPolling(t *testing.T) {
	t.Helper()
	slotWas, exitWas := slotPollInterval, exitPollInterval
	modPollWas, modWaitWas := modPollInterval, modWaitTimeout
	slotPollInterval = 20 * time.Millisecond
	exitPollInterval = 20 * time.Millisecond
	modPollInterval = 20 * time.Millisecond
	modWaitTimeout = 2 * time.Second
	t.Cleanup(func() {
		slotPollInterval, exitPollInterval = slotWas, exitWas
		modPollInterval, modWaitTimeout = modPollWas, modWaitWas
	})
}

// A world whose game reaches the sidecar says so on one line, and a world whose
// game does not says so much more loudly. Neither is a failure exit: the start
// itself worked, and the mod-side fix for the cause is what makes it rare.
//
// This is the gate on LOCAL-CONFIGRACE going silent again. Before it, a mod that
// configured itself off produced a completely successful start: sidecar up, slot
// taken, game running, nothing said.
func TestStartReportsWhetherTheModConnected(t *testing.T) {
	fastPolling(t)
	h := newHarness(t)
	h.useRealFakes()
	p := h.profile("default", "Multiverse", freeTestPort(t))
	t.Cleanup(func() { killRecorded(p) })

	if code := h.run("start"); code != exitOK {
		t.Fatalf("start: %d\n%s\n%s", code, h.out(), h.err())
	}
	mustContain(t, "the start output", h.out(), "the game joined the map: mod connected, speed x10.")
	if strings.Contains(h.err(), "HAS NOT REACHED THE SIDECAR") {
		t.Fatalf("a connected world warned anyway:\n%s", h.err())
	}
	// The event log carries the same fact for a supporter reading it later.
	mustContain(t, "the event log", readFile(t, p.LauncherLog()), "mod.connected")

	if code := h.run("stop", "default"); code != exitOK {
		t.Fatalf("stop: %d\n%s", code, h.err())
	}

	// The same start, against a sidecar whose game never arrives.
	t.Setenv("LAUNCHER_FAKE_SIDECAR", "nomod")
	quiet := h.profile("quiet", "Quiet", freeTestPort(t))
	t.Cleanup(func() { killRecorded(quiet) })

	if code := h.run("start", "quiet"); code != exitOK {
		t.Fatalf("a world whose mod never connects must still exit %d, got %d\n%s",
			exitOK, code, h.err())
	}
	for _, want := range []string{
		"HAS NOT REACHED THE SIDECAR",
		"LOCAL-CONFIGRACE",
		"LOCAL-STARVATION",
		"this is a warning, not a failure",
		quiet.BepInExLog(),
	} {
		mustContain(t, "the mod-wait warning", h.err(), want)
	}
	if strings.Contains(h.out(), "the game joined the map") {
		t.Fatalf("a world with no mod claimed it joined:\n%s", h.out())
	}
	mustContain(t, "the event log", readFile(t, quiet.LauncherLog()), "mod.not-connected")

	if code := h.run("stop", "quiet"); code != exitOK {
		t.Fatalf("stop quiet: %d\n%s", code, h.err())
	}
}

// spawnPlaceholder starts a process that stands in for a running game. It is
// the game fake, so the launcher identifies it as the game rather than as a
// stale record.
func spawnPlaceholder(t *testing.T, exe string) int {
	t.Helper()
	cmd := exec.Command(exe)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start a placeholder: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	return cmd.Process.Pid
}

// killRecorded cleans up anything a failed test left running.
func killRecorded(p Profile) {
	for _, file := range []string{p.GamePidFile(), p.SidecarPidFile()} {
		if pid := readPidFile(file); pid != 0 {
			forceStop(pid)
		}
	}
}

// snapshot records every file under root, relative to it, with its size and
// its content hash, so a later comparison names what moved.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[relative] = fmt.Sprintf("%d:%x", len(raw), simpleHash(raw))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func compareSnapshots(before, after map[string]string) string {
	var lines []string
	for path, digest := range after {
		if old, found := before[path]; !found {
			lines = append(lines, "added   "+path)
		} else if old != digest {
			lines = append(lines, "changed "+path)
		}
	}
	for path := range before {
		if _, found := after[path]; !found {
			lines = append(lines, "removed "+path)
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// simpleHash is enough to notice a change; it is not a security primitive.
func simpleHash(data []byte) uint64 {
	var h uint64 = 14695981039346656037
	for _, b := range data {
		h ^= uint64(b)
		h *= 1099511628211
	}
	return h
}

// TestStopLeavesAnUnrelatedProcessAlone is the reviewer's `sleep 300` scenario:
// a stale pid file whose number the operating system has given to somebody
// else. The launcher must remove the record and signal NOTHING.
func TestStopLeavesAnUnrelatedProcessAlone(t *testing.T) {
	h := newHarness(t)
	h.useRealFakes()
	p := h.profile("default", "Multiverse", freeTestPort(t))

	// This process is not the game and not the sidecar; its pid is planted in
	// both records.
	victim := spawnUnrelated(t)
	if err := writePidFile(p.GamePidFile(), victim); err != nil {
		t.Fatalf("write pid: %v", err)
	}
	if err := writePidFile(p.SidecarPidFile(), victim); err != nil {
		t.Fatalf("write pid: %v", err)
	}

	// It is not reported as running ...
	if code := h.run("status", "--json"); code != exitOK {
		t.Fatalf("status: %d %s", code, h.err())
	}
	mustContain(t, "the status", h.out(), `"running": false`)

	// ... it does not block a start ...
	if code := h.run("start", "--dry-run"); code != exitOK {
		t.Fatalf("dry run: %d %s", code, h.err())
	}
	mustNotContain(t, "the dry run", h.err(), "already running")

	// ... and stop removes the stale record without touching it.
	if code := h.run("stop"); code != exitOK {
		t.Fatalf("stop: %d\n%s", code, h.err())
	}
	if !processAlive(victim) {
		t.Fatal("stop killed an unrelated process")
	}
	mustContain(t, "the warning", h.err(), "no longer the game")
	mustContain(t, "the warning", h.err(), "NOTHING was stopped")
	if fileExists(p.GamePidFile()) || fileExists(p.SidecarPidFile()) {
		t.Fatal("the stale records were kept")
	}
}

// TestConcurrencyCapIgnoresStaleRecords: a stale game.pid must not count
// toward the five-world ceiling either.
func TestConcurrencyCapIgnoresStaleRecords(t *testing.T) {
	h := newHarness(t)
	h.useRealFakes()
	victim := spawnUnrelated(t)
	for i := 1; i <= MaxLocalWorlds; i++ {
		p := h.profile(fmt.Sprintf("world%d", i), fmt.Sprintf("World %d", i), 9100+i)
		if err := writePidFile(p.GamePidFile(), victim); err != nil {
			t.Fatalf("write pid: %v", err)
		}
	}
	h.profile("world6", "World 6", freeTestPort(t))
	if code := h.run("start", "world6", "--dry-run"); code != exitOK {
		t.Fatalf("the dry run exited %d\n%s", code, h.err())
	}
	mustNotContain(t, "the dry run", h.err(), "LOCAL-STARVATION")
}

// spawnUnrelated starts a process that is neither the game nor the sidecar, so
// its pid stands in for one the operating system handed out again.
func spawnUnrelated(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "300")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start an unrelated process: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	return cmd.Process.Pid
}
