package launcher

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The profile commands. A profile is one world: its identity on the map, its
// own data folder, its own sidecar port and its own save name. Creating one
// enrolls a NEW identity, which is why create is the only command here that
// touches the network.

type profileFlagSet struct {
	dataRoot             *string
	sidecarPort          *int
	world                *string
	gameDir              *string
	headless             *bool
	noHeadless           *bool
	exportEdges          *string
	excludeSpecies       *string
	noMigrationExclusion *bool
	saveMinutes          *float64
	saveKeep             *int
	saveOnQuit           *string
	joinStringFile       *string
	relayURL             *string

	provided map[string]bool
}

// registerProfileFlags registers the creation flags. With creation false it
// registers the mutable subset only: dataRoot, peerId and relayUrl are
// immutable after create, because changing any of them would point one world's
// identity at another world's journal.
//
// THERE IS NO FLAG HERE THAT TAKES A SECRET AS A VALUE. --join-string-file
// names a file, the way the sidecar's --credential-file does, so no secret ever
// reaches a process listing. A test enforces it.
func registerProfileFlags(fs *flag.FlagSet, creation bool) *profileFlagSet {
	f := &profileFlagSet{
		sidecarPort:          fs.Int("sidecar-port", 0, "the loopback port this world's sidecar listens on"),
		world:                fs.String("world", "", "the save name this world loads"),
		gameDir:              fs.String("game-dir", "", "the folder the game is installed in"),
		headless:             fs.Bool("headless", false, "run this world with nothing drawn"),
		noHeadless:           fs.Bool("no-headless", false, "draw this world"),
		exportEdges:          fs.String("export-edges", "", "which edges this world exports on: E, N, W, S"),
		excludeSpecies:       fs.String("exclude-species", "", "the species that never leave this world"),
		noMigrationExclusion: fs.Bool("no-migration-exclusion", false, "turn the exclusion policy off, deliberately"),
		saveMinutes:          fs.Float64("save-minutes", 0, "how often this world saves itself, in minutes. 0 turns the timer off"),
		saveKeep:             fs.Int("save-keep", 0, "how many saves of this world to keep"),
		saveOnQuit:           fs.String("save-on-quit", "", "on or off: write this world out when the game closes"),
	}
	if creation {
		f.dataRoot = fs.String("data-root", "", "the folder this world's journal, credential and logs live in")
		f.joinStringFile = fs.String("join-string-file", "", "private map: the file holding the operator's one-line join string")
		f.relayURL = fs.String("relay-url", "", "private map: the wss:// relay address, when the join file holds the identity half alone")
	}
	return f
}

func (f *profileFlagSet) record(fs *flag.FlagSet) {
	f.provided = make(map[string]bool)
	fs.Visit(func(fl *flag.Flag) { f.provided[fl.Name] = true })
}

func (f *profileFlagSet) was(name string) bool { return f.provided[name] }

func (a *app) commandProfile(args []string) int {
	if len(args) == 0 {
		return a.usageError("profile takes one of: list, show, use, create, clone, set, delete")
	}
	switch args[0] {
	case "list":
		return a.profileList(args[1:])
	case "show":
		return a.profileShow(args[1:])
	case "use":
		return a.profileUse(args[1:])
	case "create":
		return a.profileCreate(args[1:])
	case "clone":
		return a.profileClone(args[1:])
	case "set":
		return a.profileSet(args[1:])
	case "delete":
		return a.profileDelete(args[1:])
	default:
		return a.usageError("'profile %s' is not a command. Use list, show, use, create, clone, "+
			"set or delete", args[0])
	}
}

func (a *app) profileList(args []string) int {
	if len(args) > 0 {
		return a.usageError("profile list takes no arguments")
	}
	profiles, problems := a.install.loadProfiles()
	for _, problem := range problems {
		a.warn("skipping a profile that could not be read: %v", problem)
	}
	if a.asJSON {
		// The same rule status --json follows: the document goes out, and an
		// installation nothing could be read from is a refusal in both forms.
		a.emitJSON(profiles)
		if len(profiles) == 0 {
			return a.fail("%v", errNoProfiles)
		}
		return exitOK
	}
	if len(profiles) == 0 {
		return a.fail("%v", errNoProfiles)
	}
	active, _ := a.install.ActiveName()
	for _, p := range profiles {
		marker := " "
		if strings.EqualFold(p.Name, active) {
			marker = "*"
		}
		a.print("%s %-16s world '%s'   port %d   headless %s   %s",
			marker, p.Name, p.World, p.SidecarPort, onOff(p.Headless), pidState(p.GamePidFile(), p.GameExe()))
	}
	return exitOK
}

func (a *app) profileShow(args []string) int {
	if len(args) != 1 {
		return a.usageError("profile show takes one world name")
	}
	p, err := a.install.ResolveProfile(args[0])
	if err != nil {
		return a.fail("%v", err)
	}
	if a.asJSON {
		return a.emitJSON(p)
	}
	return a.runStatus(p.Name, false)
}

func (a *app) profileUse(args []string) int {
	if len(args) != 1 {
		return a.usageError("profile use takes one world name")
	}
	p, err := a.install.ResolveProfile(args[0])
	if err != nil {
		return a.fail("%v", err)
	}
	if err := a.install.SetActive(p.Name); err != nil {
		return a.fail("%v", err)
	}
	a.print("'%s' is the world the bare commands act on now.", p.Name)
	return exitOK
}

func (a *app) profileCreate(args []string) int {
	fs := newFlagSet("profile create", a.stderr)
	flags := registerProfileFlags(fs, true)
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return exitUsage
	}
	flags.record(fs)
	if len(positional) != 1 {
		return a.usageError("profile create takes one world name")
	}
	name := positional[0]

	profiles, err := a.install.ListProfiles()
	if err != nil {
		return a.fail("%v", err)
	}
	if err := validateProfileNameUnique(name, profiles); err != nil {
		return a.fail("%v", err)
	}

	base, hasBase := a.baseProfile(profiles)
	p := Profile{
		Format:         ProfileFormat,
		Name:           name,
		World:          name,
		SidecarPort:    nextFreePort(profiles),
		ExportEdges:    DefaultExportEdges,
		ExcludeSpecies: DefaultExcludeSpecies,
		SaveMinutes:    DefaultSaveMinutes,
		SaveKeep:       DefaultSaveKeep,
		SaveOnQuit:     true,
		CreatedUTC:     createdNow(a.now),
	}
	if hasBase {
		p.GameDir = base.GameDir
		p.DataRoot = defaultDataRoot(base, name)
	}
	return a.finishCreate(p, base, hasBase, profiles, flags)
}

func (a *app) profileClone(args []string) int {
	fs := newFlagSet("profile clone", a.stderr)
	flags := registerProfileFlags(fs, true)
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return exitUsage
	}
	flags.record(fs)
	if len(positional) != 2 {
		return a.usageError("profile clone takes the world to copy and the new world's name")
	}
	source, name := positional[0], positional[1]

	profiles, err := a.install.ListProfiles()
	if err != nil {
		return a.fail("%v", err)
	}
	src, found := findProfile(profiles, source)
	if !found {
		return a.fail("there is no world called '%s' here", source)
	}
	if err := validateProfileNameUnique(name, profiles); err != nil {
		return a.fail("%v", err)
	}

	// A clone copies the settings and nothing that has to be unique: the map
	// identity, the data folder, the port and the save name are all new.
	p := src
	p.Name = name
	p.World = name
	p.SidecarPort = nextFreePort(profiles)
	p.DataRoot = defaultDataRoot(src, name)
	p.PeerID = ""
	p.RelayURL = ""
	p.CreatedUTC = createdNow(a.now)
	return a.finishCreate(p, src, true, profiles, flags)
}

// finishCreate applies the flags, checks every rule, enrolls the new identity
// and writes the profile — in that order, so an invalid world never costs an
// identity on the map.
func (a *app) finishCreate(p Profile, base Profile, hasBase bool, profiles []Profile, flags *profileFlagSet) int {
	if err := a.applyProfileFlags(&p, flags); err != nil {
		return a.fail("%v", err)
	}
	joinFile, relayFlag := "", ""
	if flags.joinStringFile != nil {
		joinFile = *flags.joinStringFile
	}
	if flags.relayURL != nil {
		relayFlag = *flags.relayURL
	}
	return a.createProfile(p, base, hasBase, profiles, joinFile, relayFlag)
}

// createProfile is the shared tail of every way a world is created: the command
// line, and the console menu.
func (a *app) createProfile(p Profile, base Profile, hasBase bool, profiles []Profile, joinFile, relayFlag string) int {
	if p.GameDir == "" {
		return a.usageError("this installation has no world to copy the game folder from. Pass --game-dir")
	}
	if p.DataRoot == "" {
		return a.usageError("this installation has no world to place the data folder beside. Pass --data-root")
	}
	if err := validateNewProfile(p, profiles, a.install); err != nil {
		return a.fail("%v", err)
	}
	if !fileExists(p.PluginPath()) {
		a.warn("warning: %s has no %s yet. The game would start, join nothing, and look healthy. "+
			"Re-run the installer against this game folder before starting this world",
			p.GameDir, pluginRelPath)
	}

	id, code := a.identityForNewProfile(p, base, hasBase, joinFile, relayFlag)
	if code != exitOK {
		return code
	}
	onPublicMap := joinFile == ""
	p.PeerID = id.PeerID
	p.RelayURL = id.RelayURL

	if err := validateProfile(p, profiles, a.install); err != nil {
		return a.fail("%v", err)
	}
	if err := a.install.SaveProfile(p); err != nil {
		return a.fail("could not write the profile: %v", err)
	}
	finishEnrollment(id)

	a.print("wrote %s", a.install.ProfilePath(p.Name))
	a.print("your world's identity on this map: %s", p.PeerID)
	a.print("the relay it dials: %s", p.RelayURL)
	a.print("world '%s'   port %d   data %s", p.World, p.SidecarPort, p.DataRoot)
	if p.ExcludeSpecies == "" {
		a.print("%s", noExclusionWarning)
	}
	if onPublicMap {
		a.print("%s", publicMapNote)
	}
	a.print("%s", leavingNote)
	return exitOK
}

// identityForNewProfile is the fork between the public map and a private one.
func (a *app) identityForNewProfile(p Profile, base Profile, hasBase bool, joinFile, relayFlag string) (identity, int) {
	if joinFile == "" {
		if relayFlag != "" {
			return identity{}, a.usageError("--relay-url goes with --join-string-file, and this " +
				"world is being enrolled on the packaged public map")
		}
		activeRelay := ""
		if hasBase {
			activeRelay = base.RelayURL
		}
		id, err := a.enrollPublicMap(p.DataRoot, activeRelay)
		if err != nil {
			return identity{}, a.fail("%v", err)
		}
		return id, exitOK
	}
	id, err := a.identityFromJoinFile(joinFile, relayFlag, p.DataRoot)
	if err != nil {
		return identity{}, a.fail("%v", err)
	}
	return id, exitOK
}

func (a *app) profileSet(args []string) int {
	fs := newFlagSet("profile set", a.stderr)
	flags := registerProfileFlags(fs, false)
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return exitUsage
	}
	flags.record(fs)
	if len(positional) != 1 {
		return a.usageError("profile set takes one world name")
	}
	profiles, err := a.install.ListProfiles()
	if err != nil {
		return a.fail("%v", err)
	}
	p, found := findProfile(profiles, positional[0])
	if !found {
		return a.fail("there is no world called '%s' here", positional[0])
	}
	if len(flags.provided) == 0 {
		return a.usageError("profile set changes nothing unless it is given a flag. See 'help profile'")
	}
	if err := a.applyProfileFlags(&p, flags); err != nil {
		return a.fail("%v", err)
	}
	if err := validateProfile(p, otherProfiles(profiles, p.Name), a.install); err != nil {
		return a.fail("%v", err)
	}
	if err := a.install.SaveProfile(p); err != nil {
		return a.fail("could not write the profile: %v", err)
	}
	a.print("wrote %s", a.install.ProfilePath(p.Name))
	if strings.EqualFold(p.Name, a.install.installerDefaultProfile()) {
		a.print("%s", installedWorldNote)
	}
	return exitOK
}

func (a *app) profileDelete(args []string) int {
	fs := newFlagSet("profile delete", a.stderr)
	removeData := fs.Bool("remove-world-data", false,
		"also delete this world's journal, logs and credential under its data folder")
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(positional) != 1 {
		return a.usageError("profile delete takes one world name")
	}
	profiles, problems := a.install.loadProfiles()
	for _, problem := range problems {
		a.warn("skipping a profile that could not be read: %v", problem)
	}
	p, found := findProfile(profiles, positional[0])
	if !found {
		return a.fail("there is no world called '%s' here", positional[0])
	}
	if pid := livePid(p.SidecarPidFile(), a.install.SidecarExe()); pid != 0 {
		return a.fail("the sidecar for '%s' is running (pid %d). Stop this world first", p.Name, pid)
	}
	if pid := livePid(p.GamePidFile(), p.GameExe()); pid != 0 {
		return a.fail("the game for '%s' is running (pid %d). Stop this world first", p.Name, pid)
	}

	// THE LAST GATE BEFORE RemoveAll. The writer's rules are re-run on the
	// LOADED profile, because a profile file can be hand-edited between the
	// write and the delete, and a dataRoot that had landed on a game folder or
	// on the installed application would take that whole tree with it. A
	// refusal here deletes NOTHING - not even the profile file - so the state
	// on disk still describes what exists.
	if *removeData {
		if err := validateRemovable(p, profiles, a.install); err != nil {
			return a.fail("--remove-world-data will not delete '%s': %v.\nNothing was deleted, "+
				"including the profile itself. Fix the world's data folder first, or delete the "+
				"profile without --remove-world-data", p.DataRoot, err)
		}
	}

	a.print("%s", custodyWarning)
	if *removeData {
		if _, found := readInstallRecord(p.DataRoot); found {
			a.print("%s", installRecordWarning)
		}
		a.print("  This will delete this world's own entries under %s: %s.",
			p.DataRoot, strings.Join(worldOwnedEntries(), ", "))
		a.print("  Anything else in that folder is left where it is, and named when it is.")
	}
	// The typed name is required for --remove-world-data EVEN under --yes: a
	// blanket "answer yes to everything" must not be able to destroy a
	// directory tree without naming it. Without --remove-world-data, --yes
	// still answers the question.
	if *removeData || !a.assumeYes {
		typed := a.ask(fmt.Sprintf("type the world's name to delete it (%s): ", p.Name))
		if !strings.EqualFold(typed, p.Name) {
			return a.fail("that is not '%s'. Nothing was deleted", p.Name)
		}
	}
	if err := a.install.DeleteProfile(p.Name); err != nil {
		return a.fail("could not delete the profile: %v", err)
	}
	a.print("deleted %s", a.install.ProfilePath(p.Name))

	if *removeData {
		// removeWorldData deletes THIS WORLD'S entries and leaves the rest. What
		// it left is printed by name, because "deleted the folder" was a claim
		// about other people's files that was true only by accident.
		removed, kept, rootRemoved, err := removeWorldData(p.DataRoot)
		if rootRemoved && err == nil {
			a.print("deleted %s, including its journal", p.DataRoot)
		} else {
			for _, name := range removed {
				a.print("deleted %s", filepath.Join(p.DataRoot, name))
			}
		}
		if err != nil {
			return a.fail("the profile is gone, but %s could not be emptied: %v", p.DataRoot, err)
		}
		if !rootRemoved {
			a.print("kept %s: %d thing(s) in it are not this world's, and every one was left "+
				"exactly where it is - %s", p.DataRoot, len(kept), strings.Join(kept, ", "))
			a.print("%s", keptEntriesNote)
		}
		a.print("%s", gameSaveNote(p.World))
	} else {
		a.print("kept %s - the journal, the logs and the credential are still there", p.DataRoot)
	}

	if active, err := a.install.ActiveName(); err == nil {
		a.install.SetActive(active)
	} else {
		os.Remove(a.install.ActiveFile())
	}
	return exitOK
}

// installRecordWarning fires when the world being removed is the one the
// installer created: its data root holds install-record.json, which
// Uninstall-BibitesMultiverse.ps1 reads to undo the mod installation.
//
// THE RECORD IS KEPT, and so is everything else this world does not own. It used
// to go with the folder, which left the uninstaller unable to find what it had
// installed - and on a complete-edition install the same RemoveAll took
// runtimes\, the game itself.
const installRecordWarning = `  This world's data folder holds install-record.json, which the uninstaller reads to
  undo the mod installation, and it is LEFT IN PLACE - deleting a world is not
  uninstalling this software. Run Uninstall-BibitesMultiverse.ps1 if you mean to
  remove the whole installation.`

// keptEntriesNote explains what "left in place" is protecting. Every name in it
// has been in a real data root on a real machine.
const keptEntriesNote = `  A data folder is not always only one world's: the complete edition keeps THE GAME
  in runtimes\, the installer keeps its record beside it, an interrupted install
  leaves an orphaned credential no map can print again, and a computer that has
  hosted more than one deployment keeps those worlds' folders here too. Delete what
  you recognise, by hand.`

// gameSaveNote says where the world's actual save file is, because "including its
// journal" reads like "including everything" and the game's own saves are
// somewhere else entirely - which is correct, and worth saying out loud.
func gameSaveNote(world string) string {
	return fmt.Sprintf("  The game's own save of '%s' is NOT here and was not touched: the game keeps "+
		"its\n  save files in its own Savefiles folder, and nothing in this launcher writes or "+
		"deletes\n  them. Remove it from inside the game if you want it gone.", world)
}

// custodyWarning is what docs/participant/leave.md says, said here because
// deleting a profile is the moment a participant is most likely to believe the
// two are the same act.
const custodyWarning = `  Deleting this world here is NOT leaving the map. Your credential still authenticates
  until the operator drops it, and your sidecar may still be holding organisms it took
  custody of for somebody else. Custody is local: nobody else can drain it for you.
  Start this world, drain its journal, and then delete it - see docs/participant/leave.md.`

// baseProfile is the world a new world copies its game folder and its data
// folder's parent from.
func (a *app) baseProfile(profiles []Profile) (Profile, bool) {
	if len(profiles) == 0 {
		return Profile{}, false
	}
	if active, err := a.install.ActiveName(); err == nil {
		if p, found := findProfile(profiles, active); found {
			return p, true
		}
	}
	return profiles[0], true
}

// applyProfileFlags folds the flags that were actually given into the profile.
// A flag that was not given never overwrites a value, which is what makes
// 'profile set' a change rather than a rewrite.
func (a *app) applyProfileFlags(p *Profile, flags *profileFlagSet) error {
	if flags.was("data-root") && flags.dataRoot != nil {
		root, err := absClean(*flags.dataRoot)
		if err != nil {
			return err
		}
		p.DataRoot = root
	}
	if flags.was("game-dir") {
		dir, err := absClean(*flags.gameDir)
		if err != nil {
			return err
		}
		p.GameDir = dir
	}
	if flags.was("sidecar-port") {
		p.SidecarPort = *flags.sidecarPort
	}
	if flags.was("world") {
		p.World = *flags.world
	}
	if flags.was("headless") && flags.was("no-headless") {
		return fmt.Errorf("--headless and --no-headless disagree")
	}
	if flags.was("headless") {
		p.Headless = *flags.headless
	}
	if flags.was("no-headless") {
		p.Headless = false
	}
	if flags.was("export-edges") {
		edges, err := normalizeExportEdges(*flags.exportEdges)
		if err != nil {
			return err
		}
		p.ExportEdges = edges
	}
	if flags.was("exclude-species") || flags.was("no-migration-exclusion") {
		value := p.ExcludeSpecies
		if flags.was("exclude-species") {
			value = *flags.excludeSpecies
		}
		resolved, err := resolveExcludeSpecies(value, *flags.noMigrationExclusion)
		if err != nil {
			return err
		}
		p.ExcludeSpecies = resolved
	}
	if flags.was("save-minutes") {
		if err := validateSaveMinutes(*flags.saveMinutes); err != nil {
			return err
		}
		p.SaveMinutes = *flags.saveMinutes
	}
	if flags.was("save-keep") {
		if err := validateSaveKeep(*flags.saveKeep); err != nil {
			return err
		}
		p.SaveKeep = *flags.saveKeep
	}
	if flags.was("save-on-quit") {
		value, err := parseOnOff("--save-on-quit", *flags.saveOnQuit)
		if err != nil {
			return err
		}
		p.SaveOnQuit = value
	}
	// The edge list is normalised even when it came from a default, so what is
	// written down is what the mod will be told.
	edges, err := normalizeExportEdges(p.ExportEdges)
	if err != nil {
		return err
	}
	p.ExportEdges = edges
	return nil
}

// allCommandFlagSets builds one of every flag set this program registers, so
// the no-secret rule and the value-flag table are derived from the real
// registrations rather than from a list somebody has to remember to update.
func allCommandFlagSets() []*flag.FlagSet {
	sink := io.Discard
	start := newFlagSet("start", sink)
	registerStartFlags(start)
	stop := newFlagSet("stop", sink)
	registerStopFlags(stop)
	status := newFlagSet("status", sink)
	status.String("profile", "", "")
	status.Bool("all", false, "")
	create := newFlagSet("profile create", sink)
	registerProfileFlags(create, true)
	set := newFlagSet("profile set", sink)
	registerProfileFlags(set, false)
	del := newFlagSet("profile delete", sink)
	del.Bool("remove-world-data", false, "")
	return []*flag.FlagSet{start, stop, status, create, set, del}
}

// allFlagNames is every flag this program registers. The no-secret-flag test
// walks it.
func allFlagNames() []string {
	names := []string{"install-root", "json", "yes", "quiet", "help", "h"}
	for _, fs := range allCommandFlagSets() {
		fs.VisitAll(func(fl *flag.Flag) { names = append(names, fl.Name) })
	}
	return names
}

// commandValueFlags is every non-global flag that takes a VALUE. splitGlobals
// uses it to step over a value rather than read it as a flag of its own.
func commandValueFlags() map[string]bool {
	type boolFlag interface{ IsBoolFlag() bool }
	names := make(map[string]bool)
	for _, fs := range allCommandFlagSets() {
		fs.VisitAll(func(fl *flag.Flag) {
			if asBool, ok := fl.Value.(boolFlag); ok && asBool.IsBoolFlag() {
				return
			}
			names[fl.Name] = true
		})
	}
	return names
}
