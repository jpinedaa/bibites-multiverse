package launcher

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Starting a world: the sidecar first, then the game. THE ORDER MATTERS — the
// sidecar mints the local token the game's mod presents, into its own data
// directory, the first time it starts.

// slotGranted is the literal the sidecar writes on stderr when the map has
// given this world a place (go/internal/sidecar/contractb_client.go).
const slotGranted = "contract B: slot granted"

// slotRefusals end the wait early: each is a definite answer, not a delay.
var slotRefusals = []string{
	"placement claim refused",
	"HTTP 401",
	"certificate did not verify",
	"below this relay",
}

// slotPollInterval is how often the sidecar's log is re-read while waiting for
// a place. It is a variable so a test does not spend real seconds on it.
var slotPollInterval = 500 * time.Millisecond

// defaultSlotWait matches the generated Start-Multiverse.ps1's 60 seconds.
const defaultSlotWait = 60 * time.Second

type startOptions struct {
	headless bool
	gameOnly bool
	wait     time.Duration
	dryRun   bool
}

// runStart is the whole start sequence, including every refusal. Each refusal
// carries its own message, because "it did not start" is not a diagnosis.
func (a *app) runStart(p Profile, opts startOptions) int {
	profiles, err := a.install.ListProfiles()
	if err != nil {
		return a.fail("%v", err)
	}
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

	if opts.dryRun {
		a.reportStartChecks(p, profiles, opts)
		a.printStartPlan(p, opts)
		return exitOK
	}

	if code := a.refuseStart(p, profiles, opts); code != exitOK {
		events.event("warn", "start.refused", "world", p.World)
		return code
	}

	if err := os.MkdirAll(p.DataDir(), 0o755); err != nil {
		return a.fail("could not create %s: %v", p.DataDir(), err)
	}

	if !opts.gameOnly {
		if code := a.startSidecar(p, opts, events); code != exitOK {
			return code
		}
	}
	return a.startGame(p, opts, events)
}

// refuseStart runs the refusals in the frozen order.
func (a *app) refuseStart(p Profile, profiles []Profile, opts startOptions) int {
	if !opts.gameOnly {
		if pid := livePid(p.SidecarPidFile(), a.install.SidecarExe()); pid != 0 {
			return a.fail("a sidecar for '%s' is already running (pid %d). Stop this world first, or "+
				"start the game only with --game-only", p.Name, pid)
		}
	}
	if pid := livePid(p.GamePidFile(), p.GameExe()); pid != 0 {
		return a.fail("the game for '%s' is already running (pid %d)", p.Name, pid)
	}
	if opts.gameOnly && livePid(p.SidecarPidFile(), a.install.SidecarExe()) == 0 {
		return a.fail("--game-only needs the sidecar for '%s' already running. It is not. Start this "+
			"world with no switch", p.Name)
	}
	if !opts.gameOnly {
		if owner := a.portInUse(p.SidecarPort, profiles, p.Name); owner != "" {
			return a.fail("%s", owner)
		}
	}
	if running := liveWorldsOnGameDir(profiles, p.GameDir, p.Name); len(running) >= MaxLocalWorlds {
		return a.fail("%d worlds are already running out of %s (%s), which is the ceiling. The mod "+
			"framework hands out %d log files per game folder, and an instance that gets none does "+
			"not merely lose its log - the mod never loads in it. That is LOCAL-STARVATION in "+
			"docs/error-taxonomy.md. Stop one of them, or install a second copy of the game in its "+
			"own folder", len(running), p.GameDir, strings.Join(running, ", "), MaxLocalWorlds)
	}
	if !fileExists(p.CredentialFile()) {
		return a.fail("there is no credential at %s, so this world has no identity on any map. Create "+
			"the world again, or re-run the installer", p.CredentialFile())
	}
	if !opts.gameOnly && !fileExists(a.install.SidecarExe()) {
		return a.fail("this installation has no %s. Re-run the installer", a.install.SidecarExe())
	}
	if err := validateGameDir(p.GameDir, true); err != nil {
		return a.fail("%v", err)
	}
	return exitOK
}

// reportStartChecks tells a dry run what a real start would refuse, on stderr,
// without changing the exit code: the plan on stdout stays exactly the plan.
func (a *app) reportStartChecks(p Profile, profiles []Profile, opts startOptions) {
	if pid := livePid(p.SidecarPidFile(), a.install.SidecarExe()); pid != 0 && !opts.gameOnly {
		a.warn("would refuse: a sidecar for '%s' is already running (pid %d)", p.Name, pid)
	}
	if pid := livePid(p.GamePidFile(), p.GameExe()); pid != 0 {
		a.warn("would refuse: the game for '%s' is already running (pid %d)", p.Name, pid)
	}
	if opts.gameOnly && livePid(p.SidecarPidFile(), a.install.SidecarExe()) == 0 {
		a.warn("would refuse: --game-only needs the sidecar for '%s' already running. It is not", p.Name)
	}
	if owner := a.portInUse(p.SidecarPort, profiles, p.Name); owner != "" {
		a.warn("would refuse: %s", owner)
	}
	if running := liveWorldsOnGameDir(profiles, p.GameDir, p.Name); len(running) >= MaxLocalWorlds {
		a.warn("would refuse: %d worlds already run out of %s (LOCAL-STARVATION)", len(running), p.GameDir)
	}
	if !fileExists(p.CredentialFile()) {
		a.warn("would refuse: there is no credential at %s", p.CredentialFile())
	}
	if !fileExists(a.install.SidecarExe()) {
		a.warn("would refuse: this installation has no %s", a.install.SidecarExe())
	}
	if err := validateGameDir(p.GameDir, true); err != nil {
		a.warn("would refuse: %v", err)
	}
}

// startSidecar spawns the sidecar detached and waits for the map to grant this
// world a place. A failure leaves the sidecar running, because the next attempt
// is usually about the map rather than about the process.
func (a *app) startSidecar(p Profile, opts startOptions, events *eventLog) int {
	for _, path := range []string{p.SidecarLog(), p.SidecarLogOut()} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			return a.fail("could not clear %s: %v", path, err)
		}
	}
	// Prove the pid file can be written BEFORE anything is spawned, so a
	// read-only data root or a scanner holding the file is discovered while
	// there is still nothing to orphan. 0 is not a usable pid, so a start that
	// fails after this leaves nothing claimed.
	if err := writePidFile(p.SidecarPidFile(), 0); err != nil {
		return a.fail("could not write %s, so the sidecar was not started: %v", p.SidecarPidFile(), err)
	}
	stdout, err := os.OpenFile(p.SidecarLogOut(), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return a.fail("could not open %s: %v", p.SidecarLogOut(), err)
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(p.SidecarLog(), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return a.fail("could not open %s: %v", p.SidecarLog(), err)
	}
	defer stderr.Close()

	cmd := exec.Command(a.install.SidecarExe(), sidecarArgs(p)...)
	cmd.Dir = p.DataRoot
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = detachedAttrs()
	if err := cmd.Start(); err != nil {
		return a.fail("the sidecar did not start: %v", err)
	}
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
	}()
	if err := writePidFile(p.SidecarPidFile(), cmd.Process.Pid); err != nil {
		// A process that is running and unrecorded can never be found again by
		// the launcher, by Stop-Multiverse.ps1 or by 'stop --all' - and for the
		// sidecar that means a world holding a map slot with no way to stop it.
		// Take it back down rather than leave it orphaned.
		forceStop(cmd.Process.Pid)
		return a.fail("could not record the sidecar's pid in %s (%v), so the sidecar that had just "+
			"started was stopped again. Nothing is running", p.SidecarPidFile(), err)
	}
	events.event("info", "sidecar.started", "pid", strconv.Itoa(cmd.Process.Pid),
		"port", strconv.Itoa(p.SidecarPort), "relay", p.RelayURL)
	a.print("sidecar started (pid %d) -> %s", cmd.Process.Pid, p.RelayURL)
	a.say("waiting for the map to grant this world a place ...")

	granted, refused := a.waitForSlot(p, opts.wait, exited)
	if granted != "" {
		events.event("info", "slot.granted", "world", p.World)
		a.print("")
		a.print("YOU ARE ON THE MAP:")
		a.print("  %s", granted)
		return exitOK
	}
	events.event("error", "slot.denied", "world", p.World)
	a.warn("")
	a.warn("The map did not grant a place.")
	if refused != "" {
		a.warn("  %s", refused)
	}
	for _, line := range tailLines(p.SidecarLog(), 20) {
		a.warn("  %s", line)
	}
	a.warn("%s", slotFailureCauses(p.PeerID))
	return exitRefused
}

// waitForSlot polls the sidecar's stderr, which is where the answer lands.
func (a *app) waitForSlot(p Profile, wait time.Duration, exited <-chan struct{}) (granted, refused string) {
	// --wait 0 means look once and do not wait, which is what it reads like. A
	// negative value is refused where the flag is parsed.
	deadline := a.now().Add(wait)
	for {
		granted, refused = scanSlotLog(p.SidecarLog())
		if granted != "" || refused != "" {
			return granted, refused
		}
		select {
		case <-exited:
			// One last read: the answer may have been written just before exit.
			return scanSlotLog(p.SidecarLog())
		default:
		}
		if !a.now().Before(deadline) {
			return "", ""
		}
		time.Sleep(slotPollInterval)
	}
}

// scanSlotLog returns the last granted line, or the last refusal line.
func scanSlotLog(path string) (granted, refused string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.Contains(line, slotGranted) {
			granted = line
			continue
		}
		for _, pattern := range slotRefusals {
			if strings.Contains(line, pattern) {
				refused = line
			}
		}
	}
	if granted != "" {
		return granted, ""
	}
	return "", refused
}

// startGame spawns the game with this world's environment and records its pid.
func (a *app) startGame(p Profile, opts startOptions, events *eventLog) int {
	if err := writePidFile(p.GamePidFile(), 0); err != nil {
		return a.fail("could not write %s, so the game was not started: %v", p.GamePidFile(), err)
	}
	cmd := exec.Command(p.GameExe(), gameArgs(opts.headless)...)
	cmd.Dir = p.GameDir
	cmd.Env = gameEnvironment(os.Environ(), p)
	cmd.SysProcAttr = detachedAttrs()
	if err := cmd.Start(); err != nil {
		return a.fail("the game did not start: %v", err)
	}
	go cmd.Wait()
	if err := writePidFile(p.GamePidFile(), cmd.Process.Pid); err != nil {
		// Same rule as the sidecar: ask it to close first, because a game that
		// got as far as loading a world has something to save.
		gracefulStop(cmd.Process.Pid)
		if !waitForExit(cmd.Process.Pid, defaultGameStopTimeout, a.now) {
			forceStop(cmd.Process.Pid)
		}
		return a.fail("could not record the game's pid in %s (%v), so the game that had just "+
			"started was stopped again. The sidecar is still running; stop this world", p.GamePidFile(), err)
	}
	events.event("info", "game.started", "pid", strconv.Itoa(cmd.Process.Pid),
		"world", p.World, "headless", strconv.FormatBool(opts.headless))
	a.print("")
	a.print("game started (pid %d); it loads the world '%s' by itself, and seeds it on the first start.",
		cmd.Process.Pid, p.World)
	if opts.headless {
		a.print("It runs with nothing drawn. The simulation is unchanged; only the picture is gone.")
	}
	a.say("It saves itself every %s minutes, keeping %d.", formatMinutes(p.SaveMinutes), p.SaveKeep)
	a.say("logs: %s  and  %s", p.SidecarLog(), p.BepInExLog())
	a.say("Leave both running. Stop this world when you are done.")
	return exitOK
}

// sidecarArgs is the exact command line the generated Start-Multiverse.ps1
// uses. --credential-file carries the SECRET HALF only, and there is
// deliberately no flag anywhere that takes a secret literally.
func sidecarArgs(p Profile) []string {
	return []string{
		"--listen", fmt.Sprintf("127.0.0.1:%d", p.SidecarPort),
		"--relay", p.RelayURL,
		"--peer-id", p.PeerID,
		"--data-dir", p.DataDir(),
		"--credential-file", p.CredentialFile(),
	}
}

func gameArgs(headless bool) []string {
	if headless {
		return []string{"-batchmode", "-nographics"}
	}
	return nil
}

// multiverseEnv is the mod's whole configuration, in the frozen order.
//
// EVERY ONE OF THESE IS SET EXPLICITLY, INCLUDING THE ONES THAT MATCH THE MOD'S
// OWN DEFAULT. It is what keeps a future change to a default from silently
// moving what an existing world does — and, here, it is also what makes two
// worlds out of one game folder independent: the mod takes the environment of
// its own game process over the shared BepInEx config file.
func multiverseEnv(p Profile) [][2]string {
	return [][2]string{
		{"MULTIVERSE_EXPORT_EDGES", p.ExportEdges},
		{"MULTIVERSE_MIGRATION_EXCLUDE", p.ExcludeSpecies},
		{"MULTIVERSE_SAVE_MINUTES", formatMinutes(p.SaveMinutes)},
		{"MULTIVERSE_SAVE_KEEP", strconv.Itoa(p.SaveKeep)},
		{"MULTIVERSE_SAVE_ON_QUIT", strconv.FormatBool(p.SaveOnQuit)},
		{"MULTIVERSE_SIDECAR_PORT", strconv.Itoa(p.SidecarPort)},
		{"MULTIVERSE_WORLD", p.World},
		{"MULTIVERSE_PORTAL", "true"},
		{"MULTIVERSE_PORTAL_FLOURISHES", "true"},
		{"MULTIVERSE_CONTRACT_A_TOKEN_FILE", p.ContractATokenFile()},
	}
}

// gameEnvironment merges this world's ten variables into the parent
// environment. A variable this world sets REPLACES a pre-existing one rather
// than being appended beside it, so a leftover MULTIVERSE_WORLD in the parent
// shell cannot decide which world is loaded.
func gameEnvironment(parent []string, p Profile) []string {
	own := multiverseEnv(p)
	replaced := make(map[string]bool, len(own))
	for _, pair := range own {
		replaced[pair[0]] = true
	}
	env := make([]string, 0, len(parent)+len(own))
	for _, entry := range parent {
		name, _, found := strings.Cut(entry, "=")
		if found && replaced[name] {
			continue
		}
		env = append(env, entry)
	}
	for _, pair := range own {
		env = append(env, pair[0]+"="+pair[1])
	}
	return env
}

// formatMinutes renders the save interval the way the start script does: 10,
// not 10.000000.
func formatMinutes(minutes float64) string {
	return strconv.FormatFloat(minutes, 'g', -1, 64)
}

// portInUse answers whether this world's port can be taken, and says who has it
// when another profile does. An empty answer means it is free.
func (a *app) portInUse(port int, profiles []Profile, self string) string {
	for _, other := range profiles {
		if strings.EqualFold(other.Name, self) || other.SidecarPort != port {
			continue
		}
		if livePid(other.SidecarPidFile(), a.install.SidecarExe()) != 0 {
			return fmt.Sprintf("port %d is held by the world '%s', which is running. Every world "+
				"needs its own port", port, other.Name)
		}
		return fmt.Sprintf("port %d also belongs to the world '%s'. Give one of them another port",
			port, other.Name)
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Sprintf("nothing can listen on 127.0.0.1:%d (%v). Something else on this machine "+
			"has it; give this world another port", port, err)
	}
	listener.Close()
	return ""
}

// liveWorldsOnGameDir names the worlds already running out of one game folder.
func liveWorldsOnGameDir(profiles []Profile, gameDir, self string) []string {
	var running []string
	for _, other := range profiles {
		if strings.EqualFold(other.Name, self) {
			continue
		}
		if !pathsEqual(other.GameDir, gameDir) {
			continue
		}
		if livePid(other.GamePidFile(), other.GameExe()) != 0 {
			running = append(running, other.Name)
		}
	}
	return running
}

func pathsEqual(a, b string) bool {
	return strings.EqualFold(cleanFold(a), cleanFold(b))
}

func cleanFold(path string) string {
	if path == "" {
		return ""
	}
	return strings.ToLower(strings.TrimRight(filepath.Clean(path), string(filepath.Separator)))
}

// tailLines returns the last n lines of a file, for the failure report.
func tailLines(path string, n int) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\r\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines
}

// slotFailureCauses is the same five-cause list the generated start script
// prints, so a participant sees one explanation whichever front door they used.
func slotFailureCauses(peerID string) string {
	return fmt.Sprintf(`
  The five usual causes, in order:
   1. The relay is not reachable from here - a name that does not resolve, or a
      network that does not carry the connection. Neither is on the map's side.
   2. 'the relay's TLS certificate did not verify'. On a public map that is a
      certificate problem the operator has to fix; on a private map it means the
      authority is not trusted here yet - re-run the installer with -CaFile.
   3. HTTP 401: this credential is not the one the relay holds for %s.
      Ask the operator for a slot handover. The relay stores only a verifier,
      so neither public enrollment nor a join string can recover the secret.
   4. Your wire version is below the minimum this map admits. Install the newest
      release; nobody on the relay's side can push it to you.
   5. Your game build is not the one this map is on. Only the operator can see
      which build that is.

  Each of those is an entry in docs/error-taxonomy.md, with who has to act.
  The game was NOT started. Stop this world, then try again.`, peerID)
}

// printStartPlan is what --dry-run prints: the exact command line and the exact
// environment a real start would use, and nothing else.
func (a *app) printStartPlan(p Profile, opts startOptions) {
	a.print("dry run: nothing was started")
	a.print("world '%s' from profile '%s'", p.World, p.Name)
	a.print("")
	a.print("sidecar: %s", a.install.SidecarExe())
	args := sidecarArgs(p)
	for i := 0; i+1 < len(args); i += 2 {
		a.print("  %s %s", args[i], args[i+1])
	}
	a.print("  working directory: %s", p.DataRoot)
	a.print("  stdout: %s", p.SidecarLogOut())
	a.print("  stderr: %s", p.SidecarLog())
	a.print("")
	a.print("game: %s", p.GameExe())
	if gameArgs(opts.headless) == nil {
		a.print("  arguments: (none)")
	} else {
		a.print("  arguments: %s", strings.Join(gameArgs(opts.headless), " "))
	}
	a.print("  working directory: %s", p.GameDir)
	a.print("  environment:")
	for _, pair := range multiverseEnv(p) {
		a.print("    %s=%s", pair[0], pair[1])
	}
}
