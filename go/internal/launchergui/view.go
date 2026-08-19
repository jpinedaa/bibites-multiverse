// Package launchergui is the window the Bibites Multiverse shortcuts open.
//
// WHY A WINDOW. Everything this program does was already reachable from a
// console menu, and the menu is still shipped (multiverse-launcher.exe). But the
// front door of a game is not a terminal: a participant who installed a
// simulation and double-clicked an icon should be handed their worlds and one
// obvious button, not a numbered prompt.
//
// WHAT THIS WINDOW IS FOR, IN FOUR RULES. The first shape of it was a
// spreadsheet: ten columns, fourteen buttons in two rows, and a raw log filling
// half the window. Everything was reachable and nothing was obvious. The rules
// it is built to now are:
//
//	ONE OBVIOUS PRIMARY ACTION PER WORLD. A world is either started or stopped,
//	so there is one big button and its caption says which.
//	PLAIN WORDS. Nothing a person reads in the window names a thing only this
//	project has: no profile, no sidecar, no contract, no mod. Those words are
//	true and they are all still there — in the details pane, which is where the
//	program's own output goes.
//	STATE AT A GLANCE. The list says, per world, in one coloured line, which of
//	the states below it is in. The one that matters is the difference between
//	running and ON THE MAP.
//	NEVER SILENT. Anything that fails opens the details pane by itself and puts
//	the core's own words in it. A greyed button, a spinning bar and a result line
//	are additions to that, never replacements for it.
//
// IT IS A FRONT DOOR AND NOT A SECOND LAUNCHER. Every action here is a call into
// internal/launcher through launcher.Session: the same refusals, the same slot
// wait, the same mod-quit stop, the same enrollment, the same gate before a
// folder is deleted. Nothing in this package decides anything about a world.
// What it decides is what a person can press and what they are shown.
//
// WHAT IS IN THIS FILE, AND WHY IT HAS NO BUILD TAG. Windows is the only place
// walk runs and the only place this window exists, so the widgets live in
// window_windows.go. But the parts most likely to be wrong are not the widgets:
// they are the words a state is rendered as, the colour that goes with them,
// which button a state enables, the flags an edit dialog turns into a
// 'profile set', and the phrase a line of the core's output is turned into.
// Those are here, in files with no build tag on them, so they are exercised by
// ordinary tests on the machine this project is developed on — which has no
// Windows at all.
package launchergui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"multiverse/internal/launcher"
)

// WindowTitle is the FIRST PART of the main window's caption, and it is FROZEN:
// the machine harness finds this window by it.
//
// The rest of the caption is state — see WindowTitleFor — so the harness matches
// this as a PREFIX rather than as the whole string. A window whose caption never
// changes is one more thing a person has to click into to learn something the
// title bar could have told them from the taskbar.
const WindowTitle = "Bibites Multiverse"

// WindowTitleFor is the caption for one reading: the name, and how many worlds
// are up, when any are.
func WindowTitleFor(snap launcher.Snapshot) string {
	running := 0
	for _, world := range snap.Worlds {
		if world.Sidecar.Running || world.Game.Running {
			running++
		}
	}
	if running == 0 {
		return WindowTitle
	}
	return fmt.Sprintf("%s - %d of %d worlds running", WindowTitle, running, len(snap.Worlds))
}

// CloseHint is the standing line in the status bar. Closing the window is not
// stopping the worlds, and a participant who believed otherwise would think
// their world had shut down when it had not.
const CloseHint = "Your worlds keep running when you close this window"

// DocsURL is where the menu's documentation item goes, and where the update
// button goes. ONE ADDRESS, from the core, so the window and the console menu
// cannot come to name two different places to download the same release, and so
// that nothing a remote answer carries can decide what this window opens
// (internal/launcher/update.go).
const DocsURL = launcher.HomeURL

// The window's smallest useful size. Below this the world list and the panel
// beside it start to hide their own text, and a saved size smaller than this is
// not restored.
const (
	MinWindowWidth  = 900
	MinWindowHeight = 560
)

// ---------------------------------------------------------------- the colours

// Colour is what a status line is drawn in. It is an enum rather than an RGB
// value so that the mapping from a world's state to a colour is decided here,
// where a test can read it, and turned into a Windows colour in one place in
// window_windows.go.
type Colour int

// The four colours, and the ONLY four. A fifth would be a distinction nobody
// can name.
const (
	// ColourGrey is nothing is happening and nothing is wrong.
	ColourGrey Colour = iota
	// ColourGreen is the state a participant wants: on the map, with a game
	// behind it.
	ColourGreen
	// ColourAmber is in motion: starting, stopping, or waiting for something
	// that is expected to arrive.
	ColourAmber
	// ColourRed is running and NOT on the map, or an outright fault. It is the
	// colour of the state this window was written to make visible.
	ColourRed
)

// Colours is every colour there is, so that the one place a Colour becomes a
// Windows colour can be built from this list rather than from a switch that
// quietly loses a case.
func Colours() []Colour {
	return []Colour{ColourGrey, ColourGreen, ColourAmber, ColourRed}
}

// ---------------------------------------------------------------- the states

// State is what one world is doing, as a person would describe it.
type State int

// The states. THE TWO THAT MUST NEVER COLLAPSE INTO ONE are StateOnTheMap and
// StateNotOnTheMap: a world whose sidecar is up has a place on the map, and a
// world whose game's mod has connected is one the map can actually move
// organisms through. When those two disagree the map shows this world live with
// nothing behind it — LOCAL-CONFIGRACE and LOCAL-STARVATION in
// docs/error-taxonomy.md — and every other sign on the machine looks healthy.
const (
	// StateStopped is nothing running for this world.
	StateStopped State = iota
	// StateWorking is an action of this window's own running against it.
	StateWorking
	// StateStarting is up, with nothing answering for it yet.
	StateStarting
	// StateWaiting is on the map, waiting for the game to join.
	StateWaiting
	// StateOnTheMap is the whole point.
	StateOnTheMap
	// StateNotOnTheMap is the game running with its mod never having arrived.
	StateNotOnTheMap
	// StateNoMapLink is a game running with nothing holding its place.
	StateNoMapLink
)

// Status is one world's state in words and in colour.
type Status struct {
	State State
	// Short is the world list's own line: a few words, no numbers that move.
	Short string
	// Headline is the panel's line: the same fact with the detail that makes it
	// actionable.
	Headline string
	Colour   Colour
}

// Busy is the action running right now, or the zero value when none is.
//
// It is not read from the world — a start's first two seconds look exactly like
// a stopped world — so it is carried alongside the snapshot rather than derived
// from it.
type Busy struct {
	// World is the world the action is about. All is set instead when the action
	// is about every world at once.
	World string
	All   bool
	// Short is the world list's word for it ("Starting..."), and Phrase is the
	// panel's live one, which follows the core's output as it prints.
	Short  string
	Phrase string
}

// covers answers whether this action is the reason a given world is unavailable.
func (b Busy) covers(name string) bool {
	if b.Short == "" {
		return false
	}
	if b.All {
		return true
	}
	return strings.EqualFold(b.World, name)
}

// StatusFor renders one world.
//
// EVERY BRANCH HERE IS A SENTENCE SOMEBODY READS, so the order is the order the
// facts arrive in: an action of ours first, because it is the only one this
// window knows and the world's own signs lag it by a refresh; then the two
// processes; then what the sidecar says about the game behind it.
func StatusFor(world launcher.WorldView, busy Busy) Status {
	if busy.covers(world.Name) {
		phrase := busy.Phrase
		if phrase == "" {
			phrase = busy.Short
		}
		return Status{State: StateWorking, Short: busy.Short, Headline: phrase, Colour: ColourAmber}
	}
	switch {
	case !world.Sidecar.Running && !world.Game.Running:
		return Status{State: StateStopped, Short: "Stopped", Headline: "Stopped", Colour: ColourGrey}

	case !world.Sidecar.Running:
		// A game with nothing holding its place. Whatever it is simulating is
		// going nowhere: nothing is carrying it to the map or bringing anything
		// back.
		return Status{State: StateNoMapLink, Short: "NOT on the map",
			Headline: "The game is running, but this world has no link to the map - see the details",
			Colour:   ColourRed}

	case !world.Mod.Answered:
		return Status{State: StateStarting, Short: "Starting...",
			Headline: "Starting - waiting for the map to answer...", Colour: ColourAmber}

	case !world.Mod.Connected && !world.Game.Running:
		return Status{State: StateWaiting, Short: "Waiting for the game",
			Headline: "On the map" + slotSuffix(world) + ", waiting for the game to join",
			Colour:   ColourAmber}

	case !world.Mod.Connected:
		// THE ONE THIS WINDOW EXISTS FOR. The game is up, the place on the map is
		// held, and nothing is moving through it.
		return Status{State: StateNotOnTheMap, Short: "NOT on the map",
			Headline: "Running, but NOT on the map - see the details", Colour: ColourRed}
	}
	return Status{State: StateOnTheMap, Short: "On the map" + speedSuffix(world),
		Headline: "Running - on the map" + slotSuffix(world) + speedSuffix(world), Colour: ColourGreen}
}

// slotSuffix names this world's place on the map when the map has said what it
// is, and says nothing at all when it has not — a slot rendered as "-" in a
// sentence reads as a fault rather than as a fact not in yet.
func slotSuffix(world launcher.WorldView) string {
	if !world.Mod.SlotKnown {
		return ""
	}
	if !world.Mod.Live {
		return fmt.Sprintf(" (place %d, not live yet)", world.Mod.Slot)
	}
	return fmt.Sprintf(" (place %d)", world.Mod.Slot)
}

// speedSuffix is the target speed, which is a setting and therefore steady. What
// the world ACHIEVED moves every couple of seconds and belongs in the facts
// grid, not in a headline somebody is trying to read.
func speedSuffix(world launcher.WorldView) string {
	if world.Mod.TimeScale == 0 {
		return ""
	}
	return " - speed x" + roundedFloat(world.Mod.TimeScale)
}

// ---------------------------------------------------------------- the list

// Column is one column of the world list. There are two, and that is the point:
// the ten columns this list used to have were a report, and what a person wants
// from a list of two or three worlds is which one to click.
type Column struct {
	Title string
	Width int
}

// The column order is the FROZEN order of Row.Cell, and one test walks both.
const (
	ColWorld = iota
	ColStatus
	columnCount
)

// Columns is the world list's header.
func Columns() []Column {
	return []Column{
		ColWorld:  {Title: "World", Width: 150},
		ColStatus: {Title: "Status", Width: 240},
	}
}

// Row is one line of the world list, rendered.
type Row struct {
	// Name is the world's profile name, and it is what every action is asked
	// for. The selection is remembered by name rather than by index, because the
	// list is rebuilt under it every couple of seconds.
	Name string
	// Default marks the world the commands act on when no world is named, and
	// the one this window selects when it opens.
	Default bool
	Status  Status
}

// Cell is the value of one column, in the frozen order above.
func (r Row) Cell(column int) string {
	switch column {
	case ColWorld:
		if r.Default {
			// The marker the console status uses, for the same reason.
			return "* " + r.Name
		}
		return "   " + r.Name
	case ColStatus:
		return r.Status.Short
	}
	return ""
}

// RowsFrom renders a whole snapshot.
func RowsFrom(snap launcher.Snapshot, busy Busy) []Row {
	rows := make([]Row, 0, len(snap.Worlds))
	for _, world := range snap.Worlds {
		rows = append(rows, RowFrom(world, busy))
	}
	return rows
}

// RowFrom renders one world.
func RowFrom(world launcher.WorldView, busy Busy) Row {
	return Row{Name: world.Name, Default: world.Active, Status: StatusFor(world, busy)}
}

// ---------------------------------------------------------------- the panel

// Fact is one label-and-value line of the panel's small grid. The grid is the
// old table's remaining columns, moved to where they are read: beside ONE world,
// once it has been chosen.
type Fact struct {
	Label string
	Value string
}

// The panel's fact labels, in the order they are drawn. They are constants
// because the grid is built once and only its values change, so a label and the
// value under it can never drift apart.
const (
	FactSave     = "Save name"
	FactPort     = "Port"
	FactSpeed    = "Speed"
	FactData     = "Data folder"
	FactIdentity = "Map identity"
)

// FactLabels is the fixed order of the panel's grid.
func FactLabels() []string {
	return []string{FactSave, FactPort, FactSpeed, FactData, FactIdentity}
}

// Panel is the whole right-hand side for one selection, decided in one place so
// that a test can read a whole screen rather than a widget at a time.
type Panel struct {
	// World is the selected world's name, or "" when there is none.
	World string
	// Headline and Colour are the status, in the words and the colour of it.
	Headline string
	Colour   Colour
	// Hint is the next thing to do, in plain words, or "" when the headline says
	// it all. It is what a first run needs and an experienced one never sees.
	Hint string
	// Working is whether the spinning bar is shown.
	Working bool
	// Facts is FactLabels' values, in that order and always that length.
	Facts []Fact
	// Result is the last thing that happened TO THIS WORLD, or the zero value
	// when nothing has. It is not shown while something is happening.
	Result Result
	// Headless is the checkbox's state: what the world's own setting says.
	Headless bool
	// Primary is the big button.
	Primary Primary
}

// Primary is the one obvious action: its caption, and whether it can be pressed.
type Primary struct {
	Caption string
	Enabled bool
	// Tip is the tooltip, which says what pressing it will do rather than
	// repeating the caption.
	Tip string
}

// PanelFor decides the whole right-hand side.
//
// selected is nil when nothing is selected, which is the state an installation
// with no world at all is permanently in.
func PanelFor(selected *launcher.WorldView, snap launcher.Snapshot, busy Busy, results *ResultLog) Panel {
	panel := Panel{Primary: Primary{Caption: ButtonStart, Tip: StartTip}}
	if selected == nil {
		panel.Headline = "No world is selected."
		if len(snap.Worlds) == 0 {
			panel.Headline = "There are no worlds on this computer yet."
			panel.Hint = "Click '" + ButtonCreate + "' to make one."
		}
		panel.Facts = emptyFacts()
		panel.Result = results.For("")
		return panel
	}
	status := StatusFor(*selected, busy)
	panel.World = selected.Name
	panel.Headline = status.Headline
	panel.Colour = status.Colour
	panel.Headless = selected.Headless
	panel.Facts = factsFor(*selected)
	panel.Hint = hintFor(*selected, snap, status)
	panel.Result = results.For(selected.Name)

	// WHILE ANYTHING AT ALL IS RUNNING, THE PANEL SAYS WHAT IT IS. There is one
	// action goroutine, so an action about another world — a create, which has no
	// row of its own yet — still occupies this window, and a panel that went on
	// describing the selected world would have said nothing about the only thing
	// happening.
	if busy.Short != "" {
		panel.Working = true
		panel.Colour = ColourAmber
		panel.Hint = ""
		panel.Result = Result{}
		if busy.Phrase != "" {
			panel.Headline = busy.Phrase
		} else {
			panel.Headline = busy.Short
		}
	}

	running := selected.Sidecar.Running || selected.Game.Running
	if running {
		panel.Primary = Primary{Caption: ButtonStop, Tip: StopTip}
	}
	panel.Primary.Enabled = busy.Short == ""
	return panel
}

// hintFor is the sentence that tells somebody what to do next, and it is
// deliberately rare: a hint under every state is noise, and noise is not read.
func hintFor(selected launcher.WorldView, snap launcher.Snapshot, status Status) string {
	switch status.State {
	case StateStopped:
		if len(snap.Worlds) == 1 {
			// A FIRST RUN. One world, nothing running: this person has just
			// installed a simulation and is looking at a window for the first
			// time.
			return "This is your world. Click " + ButtonStart + " to join the map."
		}
		return ""
	case StateNotOnTheMap, StateNoMapLink:
		return "Nothing is reaching the map from this world. Open the details below for what to do."
	}
	return ""
}

func factsFor(world launcher.WorldView) []Fact {
	return []Fact{
		{Label: FactSave, Value: world.World},
		{Label: FactPort, Value: strconv.Itoa(world.SidecarPort)},
		{Label: FactSpeed, Value: speedFact(world)},
		{Label: FactData, Value: world.DataRoot},
		{Label: FactIdentity, Value: identityFact(world)},
	}
}

func emptyFacts() []Fact {
	facts := make([]Fact, 0, len(FactLabels()))
	for _, label := range FactLabels() {
		facts = append(facts, Fact{Label: label, Value: ""})
	}
	return facts
}

// speedFact is the target AND what the world actually produced. The gap between
// them is the reading: a machine that cannot draw fast enough holds the applied
// value below the target, and a participant who sees only one number cannot tell
// that from a world that is not running.
//
// A DASH IS NOT AN ANSWER. This read "-" for a stopped world, which is a symbol
// a person has to interpret and can read as a fault. There are two reasons there
// is no number, they are different, and each of them is a short sentence.
func speedFact(world launcher.WorldView) string {
	if !world.Sidecar.Running && !world.Game.Running {
		return "not running"
	}
	if !world.Mod.Connected || world.Mod.TimeScale == 0 {
		return "not measured yet"
	}
	target := "x" + roundedFloat(world.Mod.TimeScale) + " (target)"
	if world.Mod.Achieved == 0 {
		return target
	}
	return target + ", x" + roundedFloat(world.Mod.Achieved) + " (achieved)"
}

func identityFact(world launcher.WorldView) string {
	if world.PeerID == "" {
		return "-"
	}
	return world.PeerID
}

// roundedFloat renders a MEASUREMENT: three significant figures, because the
// achieved time scale is a live reading and its last decimals are noise in a
// panel that is redrawn every two seconds.
func roundedFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', 3, 64)
}

// exactFloat renders a SETTING, the way the launcher itself renders a save
// interval: 10, not 10.000000 — and never 1.44e+03, which is what three
// significant figures would make of the longest interval the core accepts, in a
// field somebody is about to edit and save.
func exactFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

// ---------------------------------------------------------------- the banner

// BannerFor is the red line above everything, or "" when there is nothing wrong.
//
// THE PROBLEMS ARE A BANNER, NOT AN EMPTY LIST. An installation whose profiles
// cannot be read used to answer "no worlds", which reads as "nothing is
// installed" from a program that simply could not read its own files. And a
// banner that only reports is half a banner: each one below says what to do.
func BannerFor(snap launcher.Snapshot) string {
	if len(snap.Problems) > 0 {
		return "The launcher could not read " + countWords(len(snap.Problems)) +
			" of its own: " + strings.Join(snap.Problems, " | ") +
			"   Run the installer again, or move the named file out of that folder."
	}
	if len(snap.Worlds) == 0 {
		return "There are no worlds on this computer yet. Click '" + ButtonCreate +
			"' to make one, or run the installer again."
	}
	return ""
}

// ---------------------------------------------------------------- the update

// UpdateNoticeFor is the line above the world list when a newer release has been
// published, and "" — the state this window is in almost always — when there is
// nothing to say.
//
// IT IS NOT THE BANNER. The banner is red and it means something is WRONG with
// this installation; an update is neither wrong nor urgent, and drawing it in
// the same place in the same colour would teach a participant to ignore the one
// line that is about a fault. This is its own line, in its own words, with the
// button that acts on it beside it.
//
// THE WORDS COME FROM THE CORE. launcher.UpdateNotice is what the console menu
// prints, and one sentence for both front doors is the same rule every refusal
// in this window follows.
func UpdateNoticeFor(snap launcher.Snapshot) string {
	return launcher.UpdateNotice(snap.NewerRelease)
}

func countWords(n int) string {
	if n == 1 {
		return "one file"
	}
	return fmt.Sprintf("%d files", n)
}

// WorldCountWords is the status bar's count. "2 world(s)" is a programmer
// writing a plural rule into the thing a participant reads.
func WorldCountWords(n int) string {
	if n == 1 {
		return "1 world"
	}
	return fmt.Sprintf("%d worlds", n)
}

// StatusBarText is the whole of the line along the bottom.
func StatusBarText(snap launcher.Snapshot) string {
	return fmt.Sprintf("%s   -   %s in %s", CloseHint, WorldCountWords(len(snap.Worlds)), snap.InstallRoot)
}

// ---------------------------------------------------------------- the captions

// THE CAPTIONS. They are STABLE AND UNIQUE, because the machine harness finds a
// control by its caption and a duplicate caption is an ambiguous click. One
// caption may appear on several controls — a button and the menu item that does
// the same thing — and that is the point: one rule enables both.
//
// EVERY ONE OF THEM IS PLAIN ENGLISH. A caption is not the place for a word this
// project invented; the details pane carries the program's own output, and that
// is where the internal names belong.
const (
	ButtonStart  = "Start"
	ButtonStop   = "Stop"
	ButtonCreate = "Create a world..."
	// ButtonStopAll is not "Stop all": all of WHAT is exactly the question a
	// participant with one world running and one stopped would have to stop and
	// ask.
	ButtonStopAll     = "Stop every world"
	ButtonSetDefault  = "Set as the default world"
	ButtonEdit        = "Edit settings..."
	ButtonClone       = "Clone this world..."
	ButtonDelete      = "Delete this world..."
	ButtonDiagnose    = "Run a health check"
	ButtonOpenData    = "Open the data folder"
	ButtonOpenLogs    = "Open the logs folder"
	ButtonOpenGameLog = "Open the game's own log"
	ButtonOpenConsole = "Open the commands window"
	ButtonCopyPeerID  = "Copy this world's identity"
	ButtonCopyLog     = "Copy the details"
	ButtonRefresh     = "Refresh now"
	ButtonQuit        = "Quit"
	ButtonDocs        = "Documentation (bibitesmultiverse.com)"
	ButtonAbout       = "About"
	// ButtonGetUpdate opens the website's download, and it is the ONLY thing
	// this window does about an update: nothing is fetched, nothing is
	// replaced, and no world is interrupted. A launcher that updated itself
	// would be a launcher that could replace the sidecar under a running world.
	ButtonGetUpdate = "Get the new version"

	// CheckHeadless is the world's own setting, and changing it WRITES the world
	// — which is the whole difference from the per-session override this window
	// used to carry. That override is gone from the window; the commands keep it
	// (multiverse-launcher.exe start --headless / --no-headless), because a
	// script is where a one-off belongs.
	CheckHeadless = "Run without a game window (headless)"

	// ButtonShowDetails and ButtonHideDetails are one button whose caption says
	// what pressing it does. An arrow glyph would have said the same thing to
	// people who already knew.
	ButtonShowDetails = "Show details"
	ButtonHideDetails = "Hide details"
)

// The menus' own captions. They group the secondary actions by what a person is
// trying to do rather than by which part of the program does it.
const (
	MenuWorld = "&World"
	MenuOpen  = "&Open"
	MenuHelp  = "&Help"
)

// The dialogs' own buttons. The accepting button says WHAT IT DOES rather than
// "OK": the create dialog's button takes a new identity on a shared map, and the
// delete dialog's removes a world, and neither is a thing to agree to by
// pressing the word "OK".
const (
	ButtonDialogSave     = "Save changes"
	ButtonDialogCreate   = "Create this world"
	ButtonDialogClone    = "Create the copy"
	ButtonDialogDelete   = "Delete it permanently"
	ButtonDialogCancel   = "Cancel"
	ButtonShowAdvanced   = "Show advanced settings"
	ButtonHideAdvanced   = "Hide advanced settings"
	CheckRemoveWorldData = "Also delete this world's own folder"
)

// The tooltips. EVERY CONTROL HAS ONE, and none of them repeats its caption: a
// tooltip that says "Start: starts" has cost a person a hover for nothing. What
// they say is the consequence — what will have changed after the click.
const (
	StartTip = "Starts this world: it takes its place on the map, then the game opens and joins it. " +
		"This takes up to a couple of minutes and the details below say where it has got to."
	StopTip = "Asks this world to save and quit, then closes its link to the map. Nothing is lost, " +
		"and its place on the map is kept for when it comes back."
	CreateTip = "Makes another world on this computer, with its own save file, its own folder and " +
		"its own identity on the public map."
	StopAllTip     = "Stops every world on this computer, one after another. Each one saves first."
	SetDefaultTip  = "Makes this the world the commands act on when no world is named, and the one this window opens on."
	EditTip        = "Changes this world's own settings: its save name, its port, how often it saves, and more."
	CloneTip       = "Makes a new world with this one's settings. Its identity, folder, port and save name are new, because two worlds cannot share any of them."
	DeleteTip      = "Removes this world from this computer. You will be asked to type its name first."
	DiagnoseTip    = "Checks this world from end to end - its files, the game build, the key that proves it is itself, and the map - and reports every result below."
	OpenDataTip    = "Opens this world's own folder: what it is holding for other worlds, its logs and the key that proves it is itself all live there."
	OpenLogsTip    = "Opens the folder holding this world's launcher and map logs."
	OpenGameLogTip = "Opens the game's own log file, which is where to look when the game starts but never joins the map."
	OpenConsoleTip = "Opens the same launcher as a text window with a numbered menu. It is the one a script should call."
	CopyPeerIDTip  = "Copies this world's identity on the public map to the clipboard."
	CopyLogTip     = "Copies everything in the details pane to the clipboard, for a bug report."
	HeadlessTip    = "Runs the game with nothing drawn. The simulation is exactly the same; only the picture is gone. " +
		"This is this world's own setting, and it takes effect the next time the world starts."
	DetailsTip = "Shows everything the launcher has done this session, newest at the bottom. It opens by itself when something goes wrong."
	WorldsTip  = "Every world on this computer. Double-click one, or press Enter, to start or stop it."
	RefreshTip = "Reads every world again now, rather than waiting for the next couple of seconds to pass."
	GetUpdateTip = "Opens bibitesmultiverse.com in your browser, where the newest version is downloaded from. " +
		"Nothing here is changed by pressing it, and your worlds keep running."
)

// ---------------------------------------------------------------- dialog prose

// WHAT THE DIALOGS SAY, and why it is written HERE rather than taken from the
// core.
//
// The core has its own sentences for all of this (launcher.PublicMapNote,
// LeavingNote, CustodyWarning) and it still prints them — into the details pane,
// after the action, where the program's own vocabulary belongs and where an
// advanced reader wants a repository path. But a dialog is the primary UI, and a
// machine reading the previous round found the core's words leaking straight
// into it: "the map applies a per-address enrollment limit", "your sidecar may
// still be holding organisms it took custody of", and a link to
// docs/participant/leave.md — a file on a stranger's disk that a participant has
// never seen.
//
// So these say the SAME FACTS in words somebody who has only ever played the
// game can act on, and they point at the website rather than at this repository.
// A test walks every one of them for the words that must not appear.
const (
	ProseWhatIsAWorld = "A world is a simulation on this computer with its own place on the public " +
		"map. It gets its own save file, its own folder and its own identity, and it stays on the " +
		"map until you take it off."

	// ProseAnotherIdentity is launcher.PublicMapNote, said plainly. The fact that
	// matters is that making worlds quickly can be refused for a while, and that
	// it is the map refusing rather than anything on this computer.
	ProseAnotherIdentity = "Every world you make takes another identity on the public map, and the " +
		"map limits how many worlds one address may create in a short time - so several worlds made " +
		"quickly can be refused for a while. Waiting a few minutes is the whole of the fix."

	// ProseDeletingIsNotLeaving is launcher.LeavingNote, said plainly and
	// pointing at the website instead of at a file in this repository.
	ProseDeletingIsNotLeaving = "Deleting a world here does not take it off the map. See " +
		DocsURL + " for how to leave it properly."

	// ProseCustody is launcher.CustodyWarning, said plainly. The core's version
	// says "your sidecar may still be holding organisms it took custody of for
	// somebody else", which is three ideas and two of this program's own words.
	ProseCustody = "Deleting this world here does NOT take it off the map: it stays known there " +
		"until the map's operator drops it. This world may also still be holding creatures that " +
		"were on their way somewhere else, and only this computer can pass them on. Start it once " +
		"and let it finish doing that before you delete it. " + DocsURL + " explains how to leave " +
		"for good."

	ProseCloneCopies = "The copy takes this world's settings. Its identity on the map, its own " +
		"folder, its port and its save name are all new, because two worlds cannot share any of " +
		"them. Nothing living in this world is copied."

	ProseOnlyWhatChanged = "Only what you change is written. Everything else about this world is " +
		"left exactly as it is."

	ProseSaveFileIsSafe = "The game's own save file is NOT in that folder and is never touched: it " +
		"stays with the game, whichever box you tick."

	ProseNoWorldToCopyFrom = "This installation has no world to copy from, so the folder the game " +
		"is installed in has to be filled in below."
)

// The dialogs' field tooltips. They are here for the same reason as the prose
// above: one test reads them all.
const (
	TipDialogName     = "What you will call it here. It is not the name of the save file."
	TipDialogCopyName = "What you will call the new world here."
	TipDialogSave     = "The name of the save file the game loads for this world."
	TipDialogPort     = "The port on this computer this world talks to the map through. Every world needs its own."
	TipDialogNewPort  = "The port it talks to the map through. This is the lowest one no other world on this computer holds."
	TipDialogDataRoot = "Where this world's own files live: what it is holding for other worlds, its logs, and the key that proves it is itself."
	TipDialogGameDir  = "Where The Bibites is installed. The launcher starts that copy, and it is the same for every world."
	TipDialogEdges    = "Which sides of your world are doors: E, N, W and S, separated by commas. Every door works both ways."
	TipDialogSpecies  = "Species named here stay in your world. Emptying this field turns the whole rule off, which the launcher reports as the real change it is."
	TipDialogMinutes  = "How often this world writes itself out. 0 turns the timer off. A world with no game window loses everything since its last save if it has to be forced, so keep this short for one of those."
	TipDialogKeep     = "How many of those saves are kept before the oldest is removed."
	TipDialogOnQuit   = "Saves once more on the way out, so a normal stop loses nothing."
	TipDialogAdvanced = "Everything else about the new world. All of it is already filled in with values that work."
	TipDialogRemove   = "Deletes this world's own folder: what it is holding for other worlds, its logs and its key. Anything else in that folder is left where it is."
	TipDialogTypeName = "The launcher deletes nothing unless this matches the world's name."
	CheckSaveOnQuit   = "Write the world out before it quits"
)

// DialogProse is every sentence and tooltip the dialogs show, for the test that
// walks them. A new one that is not on this list is one nothing reads.
func DialogProse() []string {
	return []string{
		ProseWhatIsAWorld, ProseAnotherIdentity, ProseDeletingIsNotLeaving, ProseCustody,
		ProseCloneCopies, ProseOnlyWhatChanged, ProseSaveFileIsSafe, ProseNoWorldToCopyFrom,
		TipDialogName, TipDialogCopyName, TipDialogSave, TipDialogPort, TipDialogNewPort,
		TipDialogDataRoot, TipDialogGameDir, TipDialogEdges, TipDialogSpecies, TipDialogMinutes,
		TipDialogKeep, TipDialogOnQuit, TipDialogAdvanced, TipDialogRemove, TipDialogTypeName,
	}
}

// InternalWords are the words that must not appear anywhere a participant reads
// in this window: this program's own names for its parts, and a path into the
// repository it was built from. Every one of them is still in the details pane,
// which is the core's own output and is left in the core's own vocabulary.
func InternalWords() []string {
	return []string{
		"profile", "sidecar", "contract", "peer", "enroll", "journal", "credential",
		"docs/", ".md", "bepinex", "mod ",
	}
}

// ---------------------------------------------------------------- the actions

// Actions is which controls a person can press. Every one of them is disabled
// when the core would refuse it, so a refusal is a message the participant does
// not have to read.
type Actions struct {
	Start       bool
	Stop        bool
	StopAll     bool
	SetDefault  bool
	Edit        bool
	Headless    bool
	Create      bool
	Clone       bool
	Delete      bool
	Diagnose    bool
	OpenData    bool
	OpenLogs    bool
	OpenGameLog bool
	CopyPeerID  bool
}

// ActionsFor decides the controls for one selection.
//
// busy is an action already running. The core's per-world lock would refuse a
// second one anyway, and it is a better experience to grey the button than to
// print "another launcher is starting or stopping this world".
func ActionsFor(selected *launcher.WorldView, snap launcher.Snapshot, busy bool) Actions {
	if busy {
		// Nothing but reading, and the details pane keeps filling.
		return Actions{}
	}
	anyRunning := false
	for _, world := range snap.Worlds {
		if world.Sidecar.Running || world.Game.Running {
			anyRunning = true
			break
		}
	}
	actions := Actions{
		StopAll: anyRunning,
		// A world can always be created, EXCEPT that an installation with
		// nothing in it has no game folder to copy — and the create dialog says
		// so itself rather than being greyed with no explanation.
		Create: true,
	}
	if selected == nil {
		return actions
	}
	running := selected.Sidecar.Running || selected.Game.Running
	actions.Start = !running
	actions.Stop = running
	actions.SetDefault = !selected.Active
	actions.Edit = true
	actions.Headless = true
	actions.Clone = true
	// The core refuses to delete a world that is running, and it is right to:
	// the profile is how its processes are found again.
	actions.Delete = !running
	actions.Diagnose = true
	actions.OpenData = true
	actions.OpenLogs = true
	actions.OpenGameLog = true
	actions.CopyPeerID = selected.PeerID != ""
	return actions
}

// NextFreeWorldName is the name a create or a clone dialog opens on: the first
// world<N> this installation has not got. A dialog that opened on a name the
// core would refuse the moment it was accepted is a dialog that wastes a
// participant's attempt.
//
// It is also the name the OTHER defaults are derived from — a new world's data
// folder is named after it — so it is chosen before them and never after.
func NextFreeWorldName(snap launcher.Snapshot) string {
	taken := make(map[string]bool, len(snap.Worlds))
	for _, world := range snap.Worlds {
		taken[strings.ToLower(world.Name)] = true
	}
	for n := 2; n <= 99; n++ {
		candidate := fmt.Sprintf("world%d", n)
		if !taken[candidate] {
			return candidate
		}
	}
	// Ninety-eight worlds on one machine is not a case worth a second naming
	// scheme: the core refuses a duplicate by name, and says so.
	return "world"
}

// ---------------------------------------------------------------- edit dialog

// WorldForm is the editable half of a world, as text — which is what a dialog
// holds. Nothing here is validated: every value goes to 'profile set' through
// the core, so the answer a participant reads is the core's own words rather
// than a second opinion this package invented.
type WorldForm struct {
	Save           string
	Port           string
	Headless       bool
	ExportEdges    string
	ExcludeSpecies string
	SaveMinutes    string
	SaveKeep       string
	SaveOnQuit     bool
	GameDir        string
}

// FormFor fills a form from a world.
func FormFor(p launcher.Profile) WorldForm {
	return WorldForm{
		Save:           p.World,
		Port:           strconv.Itoa(p.SidecarPort),
		Headless:       p.Headless,
		ExportEdges:    p.ExportEdges,
		ExcludeSpecies: p.ExcludeSpecies,
		SaveMinutes:    exactFloat(p.SaveMinutes),
		SaveKeep:       strconv.Itoa(p.SaveKeep),
		SaveOnQuit:     p.SaveOnQuit,
		GameDir:        p.GameDir,
	}
}

// DefaultNote says, beside a field, whether the value in it is the one a new
// world is created with. A person editing settings they did not choose cannot
// otherwise tell which of them are decisions and which are simply what the
// packaged default happens to be.
func DefaultNote(value, packaged string) string {
	if value == packaged {
		return "(the default)"
	}
	return "(default: " + packaged + ")"
}

// EditFlags turns what a dialog changed into the flags 'profile set' takes.
//
// ONLY WHAT CHANGED. 'profile set' changes nothing it is not given a flag for,
// and that is what makes an edit an edit: a dialog that sent every field would
// rewrite a world's settings from whatever the form happened to hold, including
// the fields a future release adds to the profile and this form does not know
// about.
func EditFlags(before launcher.Profile, after WorldForm) []string {
	was := FormFor(before)
	var flags []string
	add := func(name, value string) { flags = append(flags, name, value) }
	if trim(after.Save) != was.Save {
		add("--world", trim(after.Save))
	}
	if trim(after.Port) != was.Port {
		add("--sidecar-port", trim(after.Port))
	}
	if after.Headless != was.Headless {
		flags = append(flags, HeadlessFlag(after.Headless))
	}
	if trim(after.ExportEdges) != was.ExportEdges {
		add("--export-edges", trim(after.ExportEdges))
	}
	if trim(after.ExcludeSpecies) != was.ExcludeSpecies {
		// AN EMPTY LIST IS NOT AN EMPTY VALUE. Emptying this field turns the
		// migration exclusion policy OFF, which is a real choice with its own
		// switch — the core refuses --exclude-species "" without it, and says
		// why. The dialog carries the same shape as the command line rather than
		// routing around the refusal.
		if trim(after.ExcludeSpecies) == "" {
			flags = append(flags, "--no-migration-exclusion")
		} else {
			add("--exclude-species", trim(after.ExcludeSpecies))
		}
	}
	if trim(after.SaveMinutes) != was.SaveMinutes {
		add("--save-minutes", trim(after.SaveMinutes))
	}
	if trim(after.SaveKeep) != was.SaveKeep {
		add("--save-keep", trim(after.SaveKeep))
	}
	if after.SaveOnQuit != was.SaveOnQuit {
		add("--save-on-quit", onOff(after.SaveOnQuit))
	}
	if trim(after.GameDir) != was.GameDir {
		add("--game-dir", trim(after.GameDir))
	}
	return flags
}

// HeadlessFlag is the one flag the window's own checkbox sends. It goes through
// the same 'profile set' path as every other edit, which is what makes the
// checkbox a SETTING rather than a mode this window remembers on its own.
func HeadlessFlag(headless bool) string {
	if headless {
		return "--headless"
	}
	return "--no-headless"
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func trim(value string) string { return strings.TrimSpace(value) }

// ---------------------------------------------------------------- forwarding

// ConsoleExePath is the console launcher beside this executable.
//
// WHY THE WINDOW FORWARDS AT ALL. BibitesMultiverseLauncher.exe is the name the
// console program shipped under until this release, so a script somebody has
// already written may call it with a command line. Those keep working: the
// window hands the whole command line to multiverse-launcher.exe, waits for it,
// and exits with its code. It is best-effort rather than a promise — a shell
// does not wait for a process in the Windows GUI subsystem, so a script that
// chains two calls must name the console program itself, which is what the
// documentation now says.
func ConsoleExePath(ownExe string) string {
	return filepath.Join(filepath.Dir(ownExe), launcher.ConsoleExeName)
}
