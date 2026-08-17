package launcher

import (
	"fmt"
	"net"
	"net/http"
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

// THE GAME STARTING IS NOT THE GAME JOINING, and until this check existed
// nothing on the start path knew the difference.
//
// A mod that fails to configure itself loads, logs, and then does nothing: no
// world is auto-loaded, no heartbeat is ever sent, and the game sits at the main
// menu. The sidecar still takes a slot, so the map shows this world live with no
// game behind it — and the launcher, the installer and the sidecar all reported
// success. That is LOCAL-CONFIGRACE, and it was observed on a fresh single
// install on 2026-08-17. The mod no longer switches itself off for that reason,
// but "the mod is running and talking" is a fact worth reading rather than
// assuming, and it is cheap: the sidecar already knows.
//
// IT IS NEVER FATAL. A world that has not connected yet is the ordinary case for
// the first minute of a first start — the game seeds a world before it loads
// one — so a timeout is a loud warning and an exit code of zero. The remedy is a
// restart, and the fix for the cause is in the mod.
var modWaitTimeout = 120 * time.Second

// modPollInterval is how often /my-slot is asked. A variable so a test does not
// spend real seconds on it.
var modPollInterval = time.Second

// modProbeTimeout bounds one loopback request. The sidecar is on this machine;
// a request that takes longer than this is a request that is not going to answer.
const modProbeTimeout = 2 * time.Second

// ownSlotPath is the sidecar's read-only own-slot endpoint, sidecar.OwnSlotPath.
// It is repeated here rather than imported: the launcher and the sidecar are two
// separate programs, and the launcher has no other reason to carry the sidecar's
// package graph. A test pins the two spellings together.
const ownSlotPath = "/my-slot"

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
	// A COMMAND LEFT BEHIND MUST NOT QUIT THE WORLD THIS START IS BRINGING UP.
	// The mod polls its command file every 200 ms from the moment the plugin
	// loads, so a 'quit' an interrupted stop left there would be taken and obeyed
	// within a second of the game appearing — a world that shuts itself down just
	// after it started, for a reason nothing on screen explains.
	os.Remove(p.CommandFile())
	os.Remove(p.CommandLogFile())
	os.Remove(p.CommandFile() + cmdTempSuffix)
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
	a.say("It saves itself every %s minutes, keeping %d, and runs at x%s.",
		formatMinutes(p.SaveMinutes), p.SaveKeep, startupTimeScale)
	a.say("logs: %s  and  %s", p.SidecarLog(), p.BepInExLog())
	a.waitForMod(p, events)
	a.say("Leave both running. Stop this world when you are done.")
	return exitOK
}

// waitForMod reads the one fact the rest of the start sequence cannot see: has
// the game's mod actually connected to this world's sidecar?
//
// It asks the sidecar, over the read-only loopback endpoint the sidecar already
// serves for `--my-slot` (go/internal/sidecar/ownslot.go). No second process is
// spawned, no token is needed, and nothing is written.
func (a *app) waitForMod(p Profile, events *eventLog) {
	a.say("")
	a.say("waiting up to %.0f s for the game's mod to reach the sidecar...", modWaitTimeout.Seconds())

	client := &http.Client{Timeout: modProbeTimeout}
	// BOUNDED BY ATTEMPTS RATHER THAN BY A CLOCK. Every other wait here reads
	// a.now(), which the tests freeze; a wait that must actually end cannot be
	// one of those. Two knobs decide it, and a test shrinks both.
	attempts := int(modWaitTimeout / modPollInterval)
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 1; ; attempt++ {
		if version, ok := probeModConnected(client, p.SidecarPort); ok {
			events.event("info", "mod.connected", "world", p.World, "modVersion", version)
			a.print("the game joined the map: mod connected, speed x%s.", startupTimeScale)
			return
		}
		if attempt >= attempts {
			break
		}
		time.Sleep(modPollInterval)
	}

	events.event("warn", "mod.not-connected", "world", p.World,
		"waitedSeconds", strconv.Itoa(int(modWaitTimeout.Seconds())))
	a.warn("")
	a.warn("!! THE GAME STARTED BUT ITS MOD HAS NOT REACHED THE SIDECAR after %.0f s.",
		modWaitTimeout.Seconds())
	a.warn("   Your world is NOT on the map yet: the sidecar holds your slot, and the map shows")
	a.warn("   it live with no game behind it. Nothing is lost and nothing is broken here.")
	a.warn("")
	a.warn("   Look in the game's own log:")
	a.warn("     %s", p.BepInExLog())
	a.warn("   An error line beginning '[M2]' is the settings-file trap, LOCAL-CONFIGRACE. No")
	a.warn("   'Bibites Multiverse <version> loaded' line at all is LOCAL-STARVATION — the plugin")
	a.warn("   is not being loaded. Both are in docs/error-taxonomy.md.")
	a.warn("")
	a.warn("   The remedy for either is to restart this world:")
	a.warn("     %s stop %s", ConsoleExeName, p.Name)
	a.warn("     %s start %s", ConsoleExeName, p.Name)
	a.warn("   If it happens twice in a row, report it with that log and the code above.")
	a.warn("")
	a.warn("   The world and the sidecar are still running; this is a warning, not a failure.")
}

// probeModConnected asks the sidecar's own-slot endpoint whether a game is
// connected. Every failure — no listener yet, a body it cannot read, a sidecar
// too old to serve the path — reads as "not yet", because this is a wait.
//
// It reads the same document a session's world list reads (fetchOwnSlot in
// session.go) and takes two fields out of it, so there is one reader of that
// endpoint in this package rather than one per caller.
func probeModConnected(client *http.Client, port int) (string, bool) {
	view, ok := fetchOwnSlot(client, port)
	if !ok {
		return "", false
	}
	return view.Mod.ModVersion, view.Mod.Connected
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

// diagnoseArgs is the command line the sidecar's read-only diagnostic is run
// with, and it names THE SAME PROFILE FIELDS sidecarArgs names, on purpose.
//
// THE MAP FLAGS ARE NOT DECORATION. The diagnostic reports on the configuration
// it is given (docs/sidecar-diagnose-spec.md §1), and the sidecar's own default
// relay is a local one — ws://127.0.0.1:8795. Run with the two local folders
// alone, a perfectly healthy world on the public map is asked whether it can
// reach a relay it has nothing to do with, and answers FAIL relay-reachable and
// FAIL credential: a front door telling somebody their world is broken when what
// is broken is the question. --relay and --credential-file are what make the
// diagnostic about THIS world.
//
// THERE IS DELIBERATELY NO --support-matrix. The sidecar looks for
// support-matrix.json beside its own executable, which is this install root, and
// the installer puts a copy there (internal/sidecar/diagnose.go, supportMatrix).
// Naming it here would be a second source of truth for a path the sidecar
// already knows.
//
// No --json either: this output is read by a person, in the launcher's log pane.
func diagnoseArgs(p Profile) []string {
	return []string{
		"--diagnose",
		"--relay", p.RelayURL,
		"--data-dir", p.DataDir(),
		"--credential-file", p.CredentialFile(),
		"--game-dir", p.GameDir,
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
		{"MULTIVERSE_STARTUP_TIME_SCALE", startupTimeScale},
		{"MULTIVERSE_CONTRACT_A_TOKEN_FILE", p.ContractATokenFile()},
		// The channel `stop` asks this world to save and quit through. Setting it
		// is what makes a HEADLESS stop lossless: a headless game has no window
		// for a close request, and this file is the only way in that does not need
		// one. See modquit.go. It is per world, so two worlds out of one game
		// folder never read each other's commands.
		{cmdFileEnvName, p.CommandFile()},
	}
}

// startupTimeScale is the speed every world starts at, and it is a literal here
// for the same reason MULTIVERSE_PORTAL is: it is a mod setting the package
// chooses, not a per-world identity, and an old profile has no field for it to
// come from. The game itself resets every world it loads to x1 in code, so
// without this line a participant's world starts at x1 however fast the map
// around it runs. It is a TARGET — the game's minimum-FPS servo holds the
// applied value below it on a machine that cannot draw fast enough — and a
// participant moves it for a session with the in-game speed slider.
const startupTimeScale = "10"

// gameEnvironment merges this world's twelve variables into the parent
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
