package launcher

import (
	"fmt"
	"strconv"
	"strings"
)

// The console menu is the front door: it is what a double-clicked shortcut
// opens. It is rendered to the app's writer and read from the app's reader, so
// every frame of it is exercised by a test rather than by a person on a machine
// this rig does not have.
//
// An empty line selects Start, so one click plus Enter starts the world — the
// behaviour today's shortcut gives.

func (a *app) runMenu() int {
	// THE LOOKUP STARTS HERE AND THE FIRST FRAME DOES NOT WAIT FOR IT. A menu
	// frame is drawn and then this program blocks on a person reading it, so the
	// answer lands between two frames rather than in front of the first one —
	// which is the whole of the cost of never making anybody wait (update.go).
	a.updates = startUpdateWatch(a.getenv)
	for {
		p, err := a.install.ResolveProfile("")
		if err != nil {
			return a.fatalInteractive("%v", err)
		}
		a.renderMenu(p)
		choice, ok := a.askLine("select: ")
		if !ok {
			return exitOK
		}
		switch strings.ToLower(choice) {
		case "", "1":
			a.runStart(p, startOptions{headless: p.Headless, wait: defaultSlotWait})
		case "2":
			a.runStop(p, stopOptions{})
		case "3":
			a.runStatus("", true)
		case "4":
			a.menuChooseWorld()
		case "5":
			a.menuEditWorld(p)
		case "6":
			a.menuCreateWorld()
		case "7":
			a.menuDeleteWorld()
		case "8":
			a.menuOpenLogs(p)
		case "0", "q", "quit", "exit":
			return exitOK
		default:
			a.warn("'%s' is not one of the choices.", choice)
		}
		a.print("")
	}
}

// renderMenu draws one frame. The spacing is deliberate: the two status words
// line up under each other from one frame to the next.
//
// THE FIELD IS 24 WIDE, the same as the status output's, because "running (pid
// 11004)" is 19 characters and the 12 this used to reserve pushed 'game' out of
// its column the moment a world was actually running — which is every frame the
// alignment was there for. 24 holds the longest form the operating system can
// produce, "running (pid 4294967295)".
func (a *app) renderMenu(p Profile) {
	a.print("Bibites Multiverse launcher %s", Release)
	a.print("   profile '%s'   world '%s'   port %d   headless %s",
		p.Name, p.World, p.SidecarPort, onOff(p.Headless))
	a.print("   sidecar %-24s game %s",
		pidState(p.SidecarPidFile(), a.install.SidecarExe()), pidState(p.GamePidFile(), p.GameExe()))
	// The header names ONE world, and this menu can start, stop and delete only
	// that one. A second world running out of this installation is invisible here
	// otherwise, which is how a participant stops "the" world and leaves another
	// holding a slot on the map.
	if others := a.otherRunningWorlds(p.Name); len(others) > 0 {
		a.print("   also running: %s", strings.Join(others, ", "))
	}
	// A line, in the frame, and never a prompt. There is no choice to make here
	// and nothing to dismiss: the line appears once a newer release is known and
	// is simply absent otherwise (update.go).
	if notice := UpdateNotice(a.updates.Available()); notice != "" {
		a.print("   %s", notice)
	}
	a.print("")
	a.print("   1) Start this world            [Enter]")
	a.print("   2) Stop this world")
	a.print("   3) Status of every world")
	a.print("   4) Choose another world")
	a.print("   5) Edit this world's settings")
	a.print("   6) Create another world")
	a.print("   7) Delete a world")
	a.print("   8) Open this world's log folder")
	a.print("   0) Quit")
}

// otherRunningWorlds names every world of this installation, except the one
// given, whose game or sidecar is running. A profile that cannot be read is
// silently not running: the menu is not the place that reports broken files.
func (a *app) otherRunningWorlds(self string) []string {
	profiles, _ := a.install.loadProfiles()
	var running []string
	for _, other := range profiles {
		if strings.EqualFold(other.Name, self) {
			continue
		}
		if livePid(other.GamePidFile(), other.GameExe()) != 0 ||
			livePid(other.SidecarPidFile(), a.install.SidecarExe()) != 0 {
			running = append(running, other.Name)
		}
	}
	return running
}

// fatalInteractive keeps a double-clicked console window on screen long enough
// to be read.
func (a *app) fatalInteractive(format string, args ...any) int {
	a.warn(format, args...)
	a.askLine("Press Enter to close. ")
	return exitRefused
}

// askLine reads one line and says whether there was one. A reader at its end
// answers false, which every loop treats as "quit" rather than as an empty
// selection.
//
// THIS IS THE ONLY PLACE THIS PROGRAM ASKS A PERSON ANYTHING, which is what
// lets a front door with no terminal answer instead of hanging: a session sets
// app.prompt and the question never reaches a stream nobody is watching.
func (a *app) askLine(question string) (string, bool) {
	if a.prompt != nil {
		return a.prompt(question)
	}
	fmt.Fprint(a.stdout, question)
	line, err := a.stdin.ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	return strings.TrimSpace(line), true
}

func (a *app) menuChooseWorld() {
	profiles, problems := a.install.loadProfiles()
	for _, problem := range problems {
		a.warn("%v", problem)
	}
	a.print("")
	for i, p := range profiles {
		a.print("   %d) %s   world '%s'   port %d", i+1, p.Name, p.World, p.SidecarPort)
	}
	answer, ok := a.askLine("which world: ")
	if !ok || answer == "" {
		return
	}
	if index, err := strconv.Atoi(answer); err == nil && index >= 1 && index <= len(profiles) {
		answer = profiles[index-1].Name
	}
	a.profileUse([]string{answer})
}

// menuEditWorld is a sub-menu over the mutable fields. Every edit is validated
// and the menu is drawn again, so a refusal costs a retry rather than the whole
// session.
func (a *app) menuEditWorld(p Profile) {
	for {
		current, err := a.install.LoadProfile(p.Name)
		if err != nil {
			a.warn("%v", err)
			return
		}
		a.print("")
		a.print("   editing '%s'", current.Name)
		a.print("   1) world (save name)         '%s'", current.World)
		a.print("   2) sidecar port              %d", current.SidecarPort)
		a.print("   3) headless                  %s", onOff(current.Headless))
		a.print("   4) export edges              %s", current.ExportEdges)
		a.print("   5) species that never leave  %s", excludeWords(current.ExcludeSpecies))
		a.print("   6) save every N minutes      %s", formatMinutes(current.SaveMinutes))
		a.print("   7) saves kept                %d", current.SaveKeep)
		a.print("   8) save on quit              %s", onOff(current.SaveOnQuit))
		a.print("   9) game folder               %s", current.GameDir)
		a.print("   0) back")
		choice, ok := a.askLine("select: ")
		if !ok || choice == "0" || choice == "" {
			return
		}
		var flagName, prompt string
		switch choice {
		case "1":
			flagName, prompt = "--world", "new save name: "
		case "2":
			flagName, prompt = "--sidecar-port", "new port: "
		case "3":
			// headless is a switch rather than a value, so it is toggled.
			if current.Headless {
				a.profileSet([]string{current.Name, "--no-headless"})
			} else {
				a.profileSet([]string{current.Name, "--headless"})
			}
			continue
		case "4":
			flagName, prompt = "--export-edges", "new edges (E,N,W,S): "
		case "5":
			flagName, prompt = "--exclude-species", "species that never leave: "
		case "6":
			flagName, prompt = "--save-minutes", "minutes between saves: "
		case "7":
			flagName, prompt = "--save-keep", "saves to keep: "
		case "8":
			flagName, prompt = "--save-on-quit", "on or off: "
		case "9":
			flagName, prompt = "--game-dir", "the folder the game is installed in: "
		default:
			a.warn("'%s' is not one of the choices.", choice)
			continue
		}
		value, ok := a.askLine(prompt)
		if !ok || value == "" {
			continue
		}
		a.profileSet([]string{current.Name, flagName, value})
	}
}

// menuCreateWorld walks the values a new world needs, in the order a
// participant can answer them, and enrolls it.
func (a *app) menuCreateWorld() {
	profiles, err := a.install.ListProfiles()
	if err != nil {
		a.warn("%v", err)
		return
	}
	base, hasBase := a.baseProfile(profiles)
	if !hasBase {
		a.warn("this installation has no world to copy the game folder from. Use " +
			"'profile create NAME --game-dir PATH --data-root PATH'")
		return
	}

	a.print("")
	name, ok := a.askLine("a name for the new world: ")
	if !ok || name == "" {
		return
	}
	if err := validateProfileNameUnique(name, profiles); err != nil {
		a.warn("%v", err)
		return
	}
	p := Profile{
		Format:         ProfileFormat,
		Name:           name,
		GameDir:        base.GameDir,
		DataRoot:       defaultDataRoot(base, name),
		SidecarPort:    nextFreePort(profiles),
		World:          name,
		ExportEdges:    DefaultExportEdges,
		ExcludeSpecies: DefaultExcludeSpecies,
		SaveMinutes:    DefaultSaveMinutes,
		SaveKeep:       DefaultSaveKeep,
		SaveOnQuit:     true,
		CreatedUTC:     createdNow(a.now),
	}

	if world, ok := a.askLine(fmt.Sprintf("its save name [%s]: ", p.World)); ok && world != "" {
		p.World = world
	}
	if port, ok := a.askLine(fmt.Sprintf("its sidecar port [%d]: ", p.SidecarPort)); ok && port != "" {
		parsed, err := strconv.Atoi(port)
		if err != nil {
			a.warn("'%s' is not a port number.", port)
			return
		}
		p.SidecarPort = parsed
	}
	if root, ok := a.askLine(fmt.Sprintf("its data folder [%s]: ", p.DataRoot)); ok && root != "" {
		cleaned, err := absClean(root)
		if err != nil {
			a.warn("%v", err)
			return
		}
		p.DataRoot = cleaned
	}
	if headless, ok := a.askLine("run it headless? [n]: "); ok {
		p.Headless = strings.HasPrefix(strings.ToLower(headless), "y")
	}

	a.print("")
	a.print("   world '%s'   port %d   headless %s", p.World, p.SidecarPort, onOff(p.Headless))
	a.print("   data  %s", p.DataRoot)
	a.print("   game  %s", p.GameDir)
	a.print("%s", publicMapNote)
	a.print("%s", leavingNote)
	if !a.confirm("create it and enroll a new identity on the map? [y/N]: ") {
		a.print("nothing was created.")
		return
	}
	a.createProfile(p, base, true, profiles, "", "")
}

// menuDeleteWorld makes the typing of the name the confirmation, because the
// journal under a world's data root is the machine's record of every organism
// it is holding for somebody.
func (a *app) menuDeleteWorld() {
	profiles, problems := a.install.loadProfiles()
	for _, problem := range problems {
		a.warn("%v", problem)
	}
	a.print("")
	for _, p := range profiles {
		a.print("   %s   world '%s'   data %s", p.Name, p.World, p.DataRoot)
	}
	name, ok := a.askLine("which world: ")
	if !ok || name == "" {
		return
	}
	args := []string{name}
	if remove, ok := a.askLine("also delete its data folder and journal? [n]: "); ok &&
		strings.HasPrefix(strings.ToLower(remove), "y") {
		args = append(args, "--remove-world-data")
	}
	a.profileDelete(args)
}

func (a *app) menuOpenLogs(p Profile) {
	if err := openFolder(p.LogDir()); err != nil {
		a.warn("could not open %s: %v", p.LogDir(), err)
		return
	}
	a.print("opened %s", p.LogDir())
}
