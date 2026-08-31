package launcher

// A SESSION IS THIS PROGRAM WITH ITS TERMINAL TAKEN AWAY.
//
// WHY IT EXISTS. The graphical launcher (internal/launchergui) is a second front
// door onto the same worlds, and the one thing it must never be is a second
// implementation. Every refusal, every wait, every warning a participant reads
// in the console menu is written once, here, in the commands this package
// already had — the slot wait, waitForMod's loud warning, the mod-quit stop and
// its lossless-or-not verdict, the enrollment refusals with their 429 and 409
// wording, the delete gate that re-validates before it removes a folder. A
// window that reimplemented any of them would drift from the console in exactly
// the places that cost a participant a save or an identity.
//
// SO WHAT A SESSION ADDS IS ONLY WHAT A WINDOW NEEDS AND A TERMINAL DOES NOT:
//
//   - The words go to a WRITER the caller owns, which for the GUI is the log
//     pane. Both streams: the core says things on stderr that a person must see
//     (the mod-not-connected warning is one), and a window with no stderr would
//     have swallowed them. That is the failure this file exists to prevent, so
//     nothing here writes to os.Stdout or os.Stderr at all.
//   - Questions are ANSWERED RATHER THAN ASKED. The core's one destructive gate
//     — type the world's name before its folder is deleted — still runs IN THE
//     CORE, comparing what the dialog collected. The gate did not move; only the
//     keyboard did.
//   - A READING of every world, in one struct, cheap enough to take every couple
//     of seconds: what the profile says, what the PID ledger says, and what the
//     sidecar's own read-only endpoint says about the game behind it. The menu
//     redraws that from a fresh process each time; a window holds it.
//
// WHAT IS SAFE TO CALL TOGETHER, EXACTLY. Snapshot reads and never writes — not
// a file, not a field of this type — so it is safe beside anything, which is what
// lets a window refresh its list while an action runs. THE ACTIONS ARE NOT: they
// share one writer pair and one answer function, and two of them at once would
// interleave a participant's output and could hand one action's typed name to
// another's question. A caller runs them one at a time; the GUI has a single
// goroutine for exactly that.
//
// And a SECOND launcher — another window, or a console beside it — is made safe
// by the per-world launcher.lock rather than by this type.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// SessionOptions is everything a session needs from its embedder. Every field
// has a working default except the writers, which are the whole point.
type SessionOptions struct {
	// InstallRoot follows the frozen discovery order when it is empty: the
	// value given here, then MULTIVERSE_LAUNCHER_HOME, then the directory this
	// executable was started from.
	InstallRoot string
	// Out and Err are where the core's own words go. Err defaults to Out,
	// because a front door with one log pane still has to show both.
	Out io.Writer
	Err io.Writer
	// Now and Getenv are injected for the same reason they are on the command
	// line: so a test drives the whole surface.
	Now    func() time.Time
	Getenv func(string) string
	// Executable is how the install root is discovered when InstallRoot is
	// empty and the environment says nothing.
	Executable func() (string, error)
}

// Session is one graphical or embedded run of the launcher.
type Session struct {
	a *app
	// probe is the client the own-slot reading uses. It is separate from the
	// enrollment client: this one is loopback-only and short, and a reading
	// that hangs would stall a refresh the participant is watching.
	probe *http.Client
}

// NewSession discovers the installation and prepares the core to be driven
// without a terminal.
func NewSession(opts SessionOptions) (*Session, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	if opts.Executable == nil {
		opts.Executable = os.Executable
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.Err == nil {
		opts.Err = opts.Out
	}
	root, err := discoverInstallRoot(opts.InstallRoot, opts.Getenv, opts.Executable)
	if err != nil {
		return nil, err
	}
	a := &app{
		install: Install{Root: root},
		// NOTHING HERE READS A TERMINAL. The reader is empty rather than nil so
		// that a path which somehow reaches it answers "end of input", which
		// every loop in this package treats as quit, rather than panicking.
		stdin:  bufio.NewReader(strings.NewReader("")),
		stdout: opts.Out,
		stderr: opts.Err,
		// interactive is false: it is what keeps runDefault from ever opening a
		// console menu, and what keeps a failure from waiting on Enter.
		interactive: false,
		now:         opts.Now,
		getenv:      opts.Getenv,
		client:      enrollmentClient(),
	}
	// A question nobody answered is a NO. Every caller of askLine treats
	// (\"\", false) as "quit" or "no", never as consent.
	a.prompt = func(string) (string, bool) { return "", false }
	// The one background lookup a session starts, and it is started HERE rather
	// than on the first Snapshot so that a window opened and left alone still
	// learns about a release. It answers into memory; nothing waits on it and
	// nothing fails if it never answers (update.go).
	a.updates = startUpdateWatch(a.getenv)
	return &Session{a: a, probe: &http.Client{Timeout: modProbeTimeout}}, nil
}

// InstallRoot is the folder this session acts on.
func (s *Session) InstallRoot() string { return s.a.install.Root }

// SidecarExe is the sidecar this installation ships. The GUI names it in the
// one place it runs it itself: the diagnostic.
func (s *Session) SidecarExe() string { return s.a.install.SidecarExe() }

// answerWith makes the next question the core asks be answered with value, and
// every question after it be answered with nothing. It returns the restore
// function, so an action cannot leave a canned answer lying about for the next
// one.
//
// ONE ANSWER, NOT A SCRIPT. A queue of answers keyed by nothing at all is how a
// prompt added later silently consumes the answer meant for another question —
// and the question this exists for is the one before a folder is deleted.
func (s *Session) answerWith(value string) func() {
	previous := s.a.prompt
	var once sync.Once
	s.a.prompt = func(string) (string, bool) {
		answered := false
		once.Do(func() { answered = true })
		if answered {
			return value, true
		}
		return "", false
	}
	return func() { s.a.prompt = previous }
}

// ---------------------------------------------------------------- the reading

// Snapshot is everything a front door needs to draw one installation, taken at
// one moment.
type Snapshot struct {
	Release     string
	InstallRoot string
	// Active is the world the bare commands act on, or "" when nothing is
	// selected — which is a state a window has to be able to draw.
	Active string
	Worlds []WorldView
	// Problems is every reason this reading is not the whole picture: a profile
	// file that could not be read, and the refusal itself when nothing could.
	// A window shows it as a banner rather than an empty list, because an empty
	// list from a launcher that cannot read its own files is a lie.
	Problems []string
	// NewerRelease is the release the homepage says is newest, when that is
	// newer than Release, and "" in every other case — including "the lookup has
	// not answered", "the lookup failed" and "there is no route out of here".
	// A front door draws it or does not; there is nothing else to decide
	// (update.go).
	NewerRelease string
}

// WorldView is one world: its profile, its processes, and what the sidecar says
// about the game behind it.
type WorldView struct {
	ProfileStatus
	Mod ModView
}

// ModView is the sidecar's own-slot reading for one world. EVERY ABSENCE IS A
// VALUE here, the same rule the endpoint itself follows: Answered false means
// nobody was asked or nobody replied, which is not the same as a game that is
// not connected.
type ModView struct {
	// Answered is whether the sidecar's read-only endpoint replied at all.
	Answered bool
	// Connected is whether the game's mod has reached that sidecar. It is the
	// fact that tells a world that is ON THE MAP from a world whose sidecar
	// merely holds its place (LOCAL-CONFIGRACE, LOCAL-STARVATION).
	Connected bool
	Version   string
	// TimeScale is the speed the mod reports it is applying, and Achieved is
	// what the world actually produced per wall second. The gap between them is
	// the reading; both are 0 when the mod has not said.
	TimeScale float64
	Achieved  float64
	// Slot and Live are this world's place on the map, as the map last said.
	Slot      int
	SlotKnown bool
	Live      bool
	// RelayConnected is the sidecar's own link to the map.
	RelayConnected bool
	Population     int
	PopulationSaid bool
}

// Snapshot reads every world. It touches only files and loopback, never the
// map, and it never writes: a reading that mutates is a reading nobody can take
// twice.
func (s *Session) Snapshot() Snapshot {
	profiles, problems := s.a.install.loadProfiles()
	active, activeErr := s.a.install.ActiveName()
	if activeErr != nil {
		active = ""
	}
	status := s.a.collectStatus(profiles, active, problems)
	if len(profiles) == 0 && activeErr != nil {
		status.Problems = append(status.Problems, activeErr.Error())
	}
	snap := Snapshot{
		Release:     status.Release,
		InstallRoot: status.InstallRoot,
		Active:      status.Active,
		Problems:    status.Problems,
		// A LOCK AND A STRING. The reading does not become a network call by
		// carrying this: the lookup ran once, in its own goroutine, when the
		// session was made.
		NewerRelease: s.a.updates.Available(),
		Worlds:       make([]WorldView, len(status.Profiles)),
	}
	// THE PROBES RUN TOGETHER. One unanswered loopback request costs
	// modProbeTimeout, and five worlds in a row would cost five of them — long
	// enough for a refresh to fall behind the interval it is drawn on.
	var wait sync.WaitGroup
	for i, p := range status.Profiles {
		snap.Worlds[i] = WorldView{ProfileStatus: p}
		if !p.Sidecar.Running {
			continue
		}
		wait.Add(1)
		go func(index, port int) {
			defer wait.Done()
			if view, ok := fetchOwnSlot(s.probe, port); ok {
				snap.Worlds[index].Mod = view.modView()
			}
		}(i, p.SidecarPort)
	}
	wait.Wait()
	return snap
}

// World resolves one world by name, or the active world for an empty name. A
// window needs the profile itself for the paths it opens and the values it
// fills a dialog with.
func (s *Session) World(name string) (Profile, error) {
	return s.a.install.ResolveProfile(name)
}

// ---------------------------------------------------------------- the actions
//
// Every action answers with the SAME exit code the command line would have, and
// says everything it has to say through the session's writers. A front door
// therefore reports what happened by showing what the core said, rather than by
// inventing its own words for it.

// StartOptions is one start. Headless nil takes the profile's own setting,
// which is what makes a non-nil value a THIS SESSION ONLY override — the switch
// the console menu never had.
type StartOptions struct {
	Headless *bool
	GameOnly bool
	Wait     time.Duration
}

// Start starts one world: the sidecar, the wait for a place on the map, the
// game, and the wait for the game's mod to reach the sidecar.
func (s *Session) Start(name string, opts StartOptions) int {
	p, err := s.a.install.ResolveProfile(name)
	if err != nil {
		return s.a.fail("%v", err)
	}
	headless := p.Headless
	if opts.Headless != nil {
		headless = *opts.Headless
	}
	wait := opts.Wait
	if wait <= 0 {
		wait = defaultSlotWait
	}
	return s.a.runStart(p, startOptions{headless: headless, gameOnly: opts.GameOnly, wait: wait})
}

// Stop stops one world: the game through its own mod first, then the sidecar.
func (s *Session) Stop(name string, gameOnly bool, timeout time.Duration) int {
	p, err := s.a.install.ResolveProfile(name)
	if err != nil {
		return s.a.fail("%v", err)
	}
	return s.a.runStop(p, stopOptions{gameOnly: gameOnly, timeout: timeout})
}

// StopAll stops every world this installation knows about.
func (s *Session) StopAll(timeout time.Duration) int {
	return s.a.runStopAll(stopOptions{timeout: timeout})
}

// SetDefault records which world the bare commands — and the window's own
// default selection — act on.
func (s *Session) SetDefault(name string) int {
	return s.a.profileUse([]string{name})
}

// Edit applies the changes 'profile set' applies, given the flags a dialog
// filled in. THE FLAGS ARE THE CONTRACT, not a second parser: a field the
// dialog did not change contributes no flag, and 'profile set' therefore
// changes nothing about it — which is the rule that keeps an edit from
// rewriting a world.
func (s *Session) Edit(name string, flags []string) int {
	if len(flags) == 0 {
		return s.a.usageError("nothing was changed.")
	}
	return s.a.profileSet(append([]string{name}, flags...))
}

// Clone copies a world's settings onto a new identity, port, data folder and
// save name.
func (s *Session) Clone(source, name string) int {
	return s.a.profileClone([]string{source, name})
}

// Delete removes a profile, and with removeWorldData its own entries under its
// data folder.
//
// typedName IS THE GATE, and it is checked in the core: the caller collected it
// from a person who had to type this world's name, and it is handed to the same
// comparison the console asks its question for. A dialog that got it wrong
// deletes nothing.
func (s *Session) Delete(name string, removeWorldData bool, typedName string) int {
	args := []string{name}
	if removeWorldData {
		args = append(args, "--remove-world-data")
	}
	restore := s.answerWith(typedName)
	defer restore()
	return s.a.profileDelete(args)
}

// CreateSpec is a new world as a dialog collects it. It is deliberately the
// four things a person is asked for plus the two a new world inherits: every
// other setting arrives at its packaged default and is edited afterwards, which
// is what the console menu does.
type CreateSpec struct {
	Name     string
	World    string
	Port     int
	DataRoot string
	Headless bool
	// GameDir is inherited from the world this installation already has. It is
	// here so an installation with no world at all can still be given one.
	GameDir string
	// Keeper and WorldName are the two PUBLIC names this world is published
	// under. NewWorldDefaults fills them with a SUGGESTION the dialog shows in
	// an editable field, and an empty field is the decline: the window has no
	// blank-versus-'-' distinction to make, because a person can see the box and
	// clear it. Whatever comes back is what is written down, and an empty one is
	// never filled in on the way through.
	//
	// Create still reads them with resolvePublicName, so a '-' typed into a box
	// by somebody who learned the console's decline means what it means
	// everywhere else, and a value the map could not carry is refused with the
	// same sentence the console gives.
	Keeper    string
	WorldName string
}

// NewWorldDefaults is what a create dialog opens with: the game folder of the
// world this installation already has, a data folder beside its own, and the
// lowest port no other world holds.
func (s *Session) NewWorldDefaults(name string) (CreateSpec, error) {
	profiles, err := s.a.install.ListProfiles()
	if err != nil {
		return CreateSpec{}, err
	}
	spec := CreateSpec{Name: name, World: name, Port: nextFreePort(profiles)}
	base, hasBase := s.a.baseProfile(profiles)
	// The two public names are SUGGESTED before the game folder is, because they
	// are offered even to an installation with nothing in it: the dialog below
	// still opens, and a person filling in where their game lives should be
	// offered the same handle as everybody else.
	//
	// inheritedKeeper is the rule, and it is the same one the console's create
	// prompt uses: what this installation already answered, and the account name
	// only when there is no world to have answered it.
	spec.Keeper = inheritedKeeper(base, hasBase, s.a.getenv)
	spec.WorldName = suggestedWorldName(spec.Keeper)
	if !hasBase {
		return spec, fmt.Errorf("this installation has no world to copy the game folder from. Use " +
			"'profile create NAME --game-dir PATH --data-root PATH'")
	}
	spec.GameDir = base.GameDir
	spec.DataRoot = defaultDataRoot(base, name)
	return spec, nil
}

// Create enrolls a NEW identity on the map this installation is already on and
// writes the world down — in that order, and only after every rule about the
// world itself has passed, so an invalid world never costs an identity.
func (s *Session) Create(spec CreateSpec) int {
	profiles, err := s.a.install.ListProfiles()
	if err != nil {
		return s.a.fail("%v", err)
	}
	if err := validateProfileNameUnique(spec.Name, profiles); err != nil {
		return s.a.fail("%v", err)
	}
	base, hasBase := s.a.baseProfile(profiles)
	// THE TWO PUBLIC NAMES GO THROUGH THE ONE READER OF THEM, exactly as the
	// edit path does (applyProfileFlags). A dialog is a prompt with a box instead
	// of a line, and the decline is the same character at every prompt this
	// project has: a person who typed '-' into the box meant "publish none", and
	// a create that trimmed it and wrote it down would then be refused by the
	// writer's own rule — with a message about spaces, over a value that has
	// none. resolvePublicName also refuses what it refuses everywhere else, so a
	// name too long or holding a control character is one answer from one place.
	keeper, err := resolvePublicName(spec.Keeper)
	if err != nil {
		return s.a.fail("the keeper handle cannot be published: %v", err)
	}
	worldName, err := resolvePublicName(spec.WorldName)
	if err != nil {
		return s.a.fail("this world's published name cannot be published: %v", err)
	}
	p := Profile{
		Format:         ProfileFormat,
		Name:           spec.Name,
		GameDir:        spec.GameDir,
		DataRoot:       spec.DataRoot,
		SidecarPort:    spec.Port,
		World:          spec.World,
		Headless:       spec.Headless,
		ExportEdges:    DefaultExportEdges,
		ExcludeSpecies: DefaultExcludeSpecies,
		SaveMinutes:    DefaultSaveMinutes,
		SaveKeep:       DefaultSaveKeep,
		SaveOnQuit:     true,
		CreatedUTC:     createdNow(s.a.now),
		Keeper:         keeper,
		WorldName:      worldName,
	}
	if p.GameDir == "" && hasBase {
		p.GameDir = base.GameDir
	}
	if p.World == "" {
		p.World = p.Name
	}
	return s.a.createProfile(p, base, hasBase, profiles, "", "")
}

// Diagnose runs the sidecar's own read-only diagnostic against one world, with
// the flags that make it about THAT world (diagnoseArgs), and puts its whole
// output where everything else this session says goes.
//
// IT IS THE SIDECAR'S ANSWER, NOT THIS PROGRAM'S. The launcher adds no check of
// its own here: twenty-one of them already exist in one place, with a taxonomy
// id and an actor on every failure, and a second opinion from a second program
// is how two front doors start disagreeing about a machine.
//
// WHAT THE LAUNCHER OWES IT IS THE QUESTION. A diagnostic is only as honest as
// the configuration it is handed, and this program is the one thing on the
// machine that knows which relay and which credential a given world runs with.
// Handing it less than that is how a healthy world gets reported as a broken
// one; see diagnoseArgs.
func (s *Session) Diagnose(name string) int {
	p, err := s.a.install.ResolveProfile(name)
	if err != nil {
		return s.a.fail("%v", err)
	}
	exe := s.a.install.SidecarExe()
	if !fileExists(exe) {
		return s.a.fail("this installation has no %s. Re-run the installer", exe)
	}
	args := diagnoseArgs(p)
	s.a.print("%s %s", exe, strings.Join(args, " "))
	cmd := exec.Command(exe, args...)
	cmd.Dir = p.DataRoot
	cmd.Stdout = s.a.stdout
	cmd.Stderr = s.a.stderr
	// NO CONSOLE WINDOW. The sidecar is a console program and a graphical front
	// door has no console to lend it, so Windows would give it one of its own —
	// a black window that appears for the length of the diagnostic and reads as
	// something having gone wrong. Its output is being captured above, which is
	// where it belongs.
	cmd.SysProcAttr = noWindowAttrs()
	if err := cmd.Run(); err != nil {
		// A diagnostic that finds a fault EXITS NON-ZERO, and that is a report
		// rather than a failure of the launcher. Say what it was and let the
		// caller show the output that came with it.
		s.a.warn("the diagnostic finished with: %v", err)
		return exitRefused
	}
	return exitOK
}

// OpenLogFolder shows this world's log folder.
func (s *Session) OpenLogFolder(name string) error {
	p, err := s.a.install.ResolveProfile(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.LogDir(), 0o755); err != nil {
		return err
	}
	return openFolder(p.LogDir())
}

// OpenGameLog shows the mod framework's own log, which is where the two
// failures waitForMod warns about are read.
func (s *Session) OpenGameLog(name string) error {
	p, err := s.a.install.ResolveProfile(name)
	if err != nil {
		return err
	}
	if !fileExists(p.BepInExLog()) {
		return fmt.Errorf("there is no %s yet. It appears the first time the game runs with the mod "+
			"in it", p.BepInExLog())
	}
	return revealFile(p.BepInExLog())
}

// PublicMapNote and LeavingNote are what the console prints after a world is
// created. A dialog says them BEFORE the enrollment rather than after it, so
// they are here rather than duplicated into the window.
func PublicMapNote() string { return publicMapNote }

// LeavingNote is the other half: deleting a world here is not leaving the map.
func LeavingNote() string { return leavingNote }

// CustodyWarning is what a delete dialog has to say before it asks anything,
// for the same reason the console says it: deleting a world here is the moment
// somebody is most likely to believe it is the same act as leaving the map.
func CustodyWarning() string { return custodyWarning }

// HeadlessStopNote is what a stop that had to force a windowless game says. A
// front door that hides it has hidden the loss of everything since the last
// save.
func HeadlessStopNote() string { return headlessStopNote }

// ---------------------------------------------------------------- own-slot

// ownSlotView is the handful of fields the launcher reads from the sidecar's
// read-only own-slot endpoint. The endpoint's whole document is much larger;
// this is deliberately the part a front door draws, and it is spelled here
// rather than imported for the reason ownSlotPath is: the launcher and the
// sidecar are two separate programs, and a test pins the spellings together.
type ownSlotView struct {
	Mod struct {
		Connected         bool     `json:"connected"`
		ModVersion        string   `json:"modVersion"`
		TimeScale         *float64 `json:"timeScale"`
		AchievedTimeScale *float64 `json:"achievedTimeScale"`
		Population        *int     `json:"population"`
	} `json:"mod"`
	Slot struct {
		Slot int  `json:"slot"`
		Live bool `json:"live"`
	} `json:"slot"`
	Relay struct {
		Connected bool `json:"connected"`
	} `json:"relay"`
}

func (v ownSlotView) modView() ModView {
	out := ModView{
		Answered:       true,
		Connected:      v.Mod.Connected,
		Version:        v.Mod.ModVersion,
		Slot:           v.Slot.Slot,
		SlotKnown:      v.Slot.Slot > 0,
		Live:           v.Slot.Live,
		RelayConnected: v.Relay.Connected,
	}
	if v.Mod.TimeScale != nil {
		out.TimeScale = *v.Mod.TimeScale
	}
	if v.Mod.AchievedTimeScale != nil {
		out.Achieved = *v.Mod.AchievedTimeScale
	}
	if v.Mod.Population != nil {
		out.Population = *v.Mod.Population
		out.PopulationSaid = true
	}
	return out
}

// fetchOwnSlot asks one sidecar about itself. EVERY FAILURE IS "NOT YET":
// no listener, a body it cannot read, a sidecar too old to serve the path.
// Nothing here is fatal, because this is a reading rather than a check.
func fetchOwnSlot(client *http.Client, port int) (ownSlotView, bool) {
	var view ownSlotView
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, ownSlotPath)
	resp, err := client.Get(url)
	if err != nil {
		return view, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return view, false
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, ownSlotBodyLimit)).Decode(&view); err != nil {
		return view, false
	}
	return view, true
}

// ownSlotBodyLimit bounds what is read from a local endpoint. The document
// carries one entry per peer on the map, so it grows with the map; a megabyte is
// far above anything the relay's own limits allow and far below a memory
// problem.
const ownSlotBodyLimit = 1 << 20
