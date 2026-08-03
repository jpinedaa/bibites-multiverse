package sidecar

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
)

// childSidecar is a real multiverse-sidecar process. The test binary
// re-executes itself through TestMain's helper branch, so the child runs the
// same Main the shipped binary runs and can be SIGKILLed like any other process.
type childSidecar struct {
	t       *testing.T
	dir     string
	relay   string
	peerID  string
	sector  string
	cmd     *exec.Cmd
	addr    string
	logFile *os.File
}

func startChildSidecar(t *testing.T, dir, relayURL, peerID, sector, fault string) *childSidecar {
	t.Helper()
	c := &childSidecar{t: t, dir: dir, relay: relayURL, peerID: peerID, sector: sector}
	c.start(fault)
	t.Cleanup(c.kill)
	return c
}

func (c *childSidecar) start(fault string) {
	c.t.Helper()
	addrFile := filepath.Join(c.dir, "listen.addr")
	_ = os.Remove(addrFile)
	_ = os.Remove(filepath.Join(c.dir, "fault.hit"))

	args := []string{
		"--listen", "127.0.0.1:0",
		"--relay", c.relay,
		"--peer-id", c.peerID,
		"--data-dir", c.dir,
		"--sector", c.sector,
		"--log-level", "info",
	}
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(),
		"MULTIVERSE_TEST_HELPER=sidecar",
		"MULTIVERSE_TEST_ARGS="+strings.Join(args, "\x1f"),
	)
	if fault != "" {
		cmd.Env = append(cmd.Env, "MULTIVERSE_FAULT="+fault)
	}
	logPath := filepath.Join(c.dir, "child.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		c.t.Fatalf("child sidecar: open log: %v", err)
	}
	c.logFile = f
	cmd.Stdout = f
	cmd.Stderr = f
	// A dedicated process group, so SIGKILL reaches only the child.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		c.t.Fatalf("child sidecar: start: %v", err)
	}
	c.cmd = cmd

	deadline := time.Now().Add(20 * time.Second)
	for {
		b, err := os.ReadFile(addrFile)
		if err == nil && len(strings.TrimSpace(string(b))) > 0 {
			c.addr = strings.TrimSpace(string(b))
			return
		}
		if time.Now().After(deadline) {
			c.t.Fatalf("child sidecar: no listen.addr within 20s; log:\n%s", c.tailLog())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (c *childSidecar) url() string { return "ws://" + c.addr + contracta.ContractAPath }

func (c *childSidecar) waitFault(timeout time.Duration) {
	c.t.Helper()
	marker := filepath.Join(c.dir, "fault.hit")
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		if time.Now().After(deadline) {
			c.t.Fatalf("child sidecar: fault point not reached within %s; log:\n%s", timeout, c.tailLog())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// sigkill is the kill -9 of m2_considerations.md's kill test.
func (c *childSidecar) sigkill() {
	c.t.Helper()
	if c.cmd == nil || c.cmd.Process == nil {
		return
	}
	_ = c.cmd.Process.Signal(syscall.SIGKILL)
	_, _ = c.cmd.Process.Wait()
	c.cmd = nil
}

func (c *childSidecar) kill() {
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Signal(syscall.SIGKILL)
		_, _ = c.cmd.Process.Wait()
		c.cmd = nil
	}
	if c.logFile != nil {
		_ = c.logFile.Close()
		c.logFile = nil
	}
}

func (c *childSidecar) tailLog() string {
	b, err := os.ReadFile(filepath.Join(c.dir, "child.log"))
	if err != nil {
		return "(no log)"
	}
	s := string(b)
	if len(s) > 8000 {
		return s[len(s)-8000:]
	}
	return s
}

// TestCrashCustodyExactlyOnce is contract test (d): SIGKILL the exporting
// sidecar mid-migration, restart it against the same --data-dir, and the
// organism ends up delivered-or-bounced exactly once. D2 accepts loss; it never
// accepts duplication.
func TestCrashCustodyExactlyOnce(t *testing.T) {
	for _, fault := range []string{FaultPostJournal, FaultPostForward} {
		t.Run(fault, func(t *testing.T) { runCrashCustody(t, fault) })
	}
}

func runCrashCustody(t *testing.T, fault string) {
	relaySrv := startRelay(t)

	// Sector B and its mod stay in-process; only the exporting side is a real
	// process that can be killed.
	cfgB := fastConfig(t, relaySrv.url(), "peer-b", contractb.SectorB)
	cfgB.MigrateInAckTimeout = 2 * time.Second
	sideB := startSidecar(t, cfgB)
	waitSector(t, sideB, contractb.SectorB)
	worldB := newWorld()
	modB := dialFakeMod(t, fakeModOptions{
		url: sideB.URL(), world: worldB, borderEdges: []string{contracta.EdgeW},
		sector: &contracta.Sector{X: 1, Y: 0}, heartbeat: 200 * time.Millisecond})

	dataDir := t.TempDir()
	childA := startChildSidecar(t, dataDir, relaySrv.url(), "peer-a", contractb.SectorA, fault)

	worldA := newWorld()
	modA := dialFakeMod(t, fakeModOptions{
		url: childA.url(), world: worldA, borderEdges: []string{contracta.EdgeE},
		sector: &contracta.Sector{X: 0, Y: 0}, heartbeat: 300 * time.Millisecond})
	modA.waitEdge(contracta.EdgeE, true, 20*time.Second)
	modB.waitEdge(contracta.EdgeW, true, 20*time.Second)

	migrationID := modA.migrateOut(testEntityID, contracta.EdgeE, 0.6031925)

	// Stop the process at exactly the point the custody chain is most exposed.
	childA.waitFault(20 * time.Second)
	childA.sigkill()
	modA.abort()

	// Restart against the same --data-dir, with no fault this time.
	childA.start("")
	modA2 := dialFakeMod(t, fakeModOptions{
		url: childA.url(), world: worldA, borderEdges: []string{contracta.EdgeE},
		sector: &contracta.Sector{X: 0, Y: 0}, heartbeat: 300 * time.Millisecond})
	_ = modA2

	waitFor(t, 30*time.Second, "the organism to be accounted for after the crash", func() bool {
		return worldA.spawnCount(migrationID)+worldB.spawnCount(migrationID) >= 1
	})
	// Let any second copy show up before asserting.
	time.Sleep(2 * time.Second)

	inA := worldA.spawnCount(migrationID)
	inB := worldB.spawnCount(migrationID)
	if inA+inB != 1 {
		t.Fatalf("the organism is delivered-or-bounced %d times (A=%d, B=%d), want exactly 1; child log:\n%s",
			inA+inB, inA, inB, childA.tailLog())
	}
	if worldA.isAlive(testEntityID) && worldB.isAlive(testEntityID) {
		t.Fatal("the organism is alive in both sims: this is the duplication D2 forbids")
	}
	if got := worldA.destroyCount(migrationID); got > 1 {
		t.Fatalf("sector A destroyed its copy %d times, want at most 1", got)
	}
}
