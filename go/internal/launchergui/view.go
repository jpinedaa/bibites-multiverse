// Package launchergui is the window the Bibites Multiverse shortcuts open.
//
// WHY A WINDOW. Everything this program does was already reachable from a
// console menu, and the menu is still shipped (multiverse-launcher.exe). But the
// front door of a game is not a terminal: a participant who installed a
// simulation and double-clicked an icon should be handed a list of their worlds
// with buttons beside it, not a numbered prompt. The window also shows what the
// menu structurally could not — every world at once, refreshed while they run,
// with the one fact that says a world is really on the map (its mod has reached
// its sidecar) in a column rather than in a line that scrolled past a minute
// ago.
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
// they are the column of a table that does not match the value put in it, the
// flags an edit dialog turns into a 'profile set', the enabling of a button that
// should have been grey, and the timestamping of a log line written by another
// goroutine mid-sentence. Those are all in this file, with no build tag on it,
// so they are exercised by ordinary tests on the machine this project is
// developed on — which has no Windows at all.
package launchergui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"multiverse/internal/launcher"
)

// WindowTitle is the title of the main window, and it is FROZEN: the machine
// harness finds this window by its caption.
const WindowTitle = "Bibites Multiverse"

// CloseHint is the standing line in the status bar. Closing the window is not
// stopping the worlds, and a participant who believed otherwise would think
// their world had shut down when it had not.
const CloseHint = "worlds keep running when you close this window"

// DocsURL is where the menu's documentation item goes.
const DocsURL = "https://bibitesmultiverse.com"

// ---------------------------------------------------------------- the columns

// Column is one column of the world list.
type Column struct {
	Title string
	Width int
}

// The column order is the FROZEN order of Row.Cell, and one test walks both.
const (
	ColWorld = iota
	ColSave
	ColPort
	ColHeadless
	ColSidecar
	ColGame
	ColMod
	ColSpeed
	ColSlot
	ColData
	columnCount
)

// Columns is the world list's header. The widths are what the strings below
// need at the default font: a pid form is 19 characters, and the data folder is
// last because it is the one column with no upper bound.
func Columns() []Column {
	return []Column{
		ColWorld:    {Title: "World", Width: 130},
		ColSave:     {Title: "Save name", Width: 120},
		ColPort:     {Title: "Port", Width: 55},
		ColHeadless: {Title: "Window", Width: 70},
		ColSidecar:  {Title: "Sidecar", Width: 130},
		ColGame:     {Title: "Game", Width: 130},
		ColMod:      {Title: "On the map", Width: 130},
		ColSpeed:    {Title: "Speed", Width: 110},
		ColSlot:     {Title: "Slot", Width: 60},
		ColData:     {Title: "Data folder", Width: 320},
	}
}

// Row is one line of the world list, rendered.
type Row struct {
	// Name is the world's profile name, and it is what every action is asked
	// for. The selection is remembered by name rather than by index, because the
	// list is rebuilt under it every couple of seconds.
	Name string
	// Active marks the world the bare commands and this window's default
	// selection act on.
	Active bool
	Save   string
	Port   string
	// Headless says which way round this world runs, in the words a person would
	// use rather than the field's name.
	Headless string
	Sidecar  string
	Game     string
	Mod      string
	Speed    string
	Slot     string
	Data     string
}

// Cell is the value of one column, in the frozen order above.
func (r Row) Cell(column int) string {
	switch column {
	case ColWorld:
		if r.Active {
			// The marker the console status uses, for the same reason.
			return "* " + r.Name
		}
		return "  " + r.Name
	case ColSave:
		return r.Save
	case ColPort:
		return r.Port
	case ColHeadless:
		return r.Headless
	case ColSidecar:
		return r.Sidecar
	case ColGame:
		return r.Game
	case ColMod:
		return r.Mod
	case ColSpeed:
		return r.Speed
	case ColSlot:
		return r.Slot
	case ColData:
		return r.Data
	}
	return ""
}

// RowsFrom renders a whole snapshot.
func RowsFrom(snap launcher.Snapshot) []Row {
	rows := make([]Row, 0, len(snap.Worlds))
	for _, world := range snap.Worlds {
		rows = append(rows, RowFrom(world))
	}
	return rows
}

// RowFrom renders one world.
func RowFrom(world launcher.WorldView) Row {
	return Row{
		Name:     world.Name,
		Active:   world.Active,
		Save:     world.World,
		Port:     strconv.Itoa(world.SidecarPort),
		Headless: headlessWords(world.Headless),
		Sidecar:  processWords(world.Sidecar),
		Game:     processWords(world.Game),
		Mod:      modWords(world),
		Speed:    speedWords(world),
		Slot:     slotWords(world),
		Data:     world.DataRoot,
	}
}

// headlessWords says what a person sees rather than what the field is called.
func headlessWords(headless bool) string {
	if headless {
		return "no window"
	}
	return "window"
}

func processWords(p launcher.ProcessStatus) string {
	if !p.Running {
		return "stopped"
	}
	return fmt.Sprintf("running (pid %d)", p.PID)
}

// modWords is the column that matters most on this list, and the reason it
// exists at all.
//
// A WORLD WHOSE SIDECAR IS UP IS NOT A WORLD ON THE MAP. The sidecar takes the
// slot; the game's mod is what puts organisms through it. When the mod never
// arrives, the map shows this world live with nothing behind it, and every other
// column on this row looks healthy — that is LOCAL-CONFIGRACE and
// LOCAL-STARVATION in docs/error-taxonomy.md. So the words here distinguish four
// states and never collapse them: not started, started and not asked yet, asked
// and not connected, connected with the mod's own version.
func modWords(world launcher.WorldView) string {
	if !world.Sidecar.Running {
		return "-"
	}
	if !world.Mod.Answered {
		return "sidecar not answering"
	}
	if !world.Mod.Connected {
		if world.Game.Running {
			return "NOT CONNECTED - see the log"
		}
		return "no game"
	}
	if world.Mod.Version == "" {
		return "connected"
	}
	return "connected (mod " + world.Mod.Version + ")"
}

// speedWords is the target and what the world actually produced. The gap
// between them is the reading: a machine that cannot draw fast enough holds the
// applied value below the target, and a participant who sees only one number
// cannot tell that from a world that is not running.
func speedWords(world launcher.WorldView) string {
	if !world.Mod.Connected || world.Mod.TimeScale == 0 {
		return "-"
	}
	target := "x" + roundedFloat(world.Mod.TimeScale)
	if world.Mod.Achieved == 0 {
		return target
	}
	return target + " (" + roundedFloat(world.Mod.Achieved) + " achieved)"
}

func slotWords(world launcher.WorldView) string {
	if !world.Mod.SlotKnown {
		return "-"
	}
	slot := strconv.Itoa(world.Mod.Slot)
	if !world.Mod.Live {
		return slot + " (not live)"
	}
	return slot
}

// roundedFloat renders a MEASUREMENT: three significant figures, because the
// achieved time scale is a live reading and its last decimals are noise in a
// column that is redrawn every two seconds.
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

// ---------------------------------------------------------------- the buttons

// The button captions. THEY ARE STABLE AND UNIQUE, because the machine harness
// finds a control by its caption and a duplicate caption is an ambiguous click.
const (
	ButtonStart       = "Start"
	ButtonStop        = "Stop"
	ButtonStopAll     = "Stop every world"
	ButtonSetDefault  = "Set as default"
	ButtonEdit        = "Edit settings..."
	ButtonCreate      = "Create another world..."
	ButtonClone       = "Clone world..."
	ButtonDelete      = "Delete world..."
	ButtonDiagnose    = "Check this world"
	ButtonOpenLogs    = "Open logs folder"
	ButtonOpenGameLog = "Open the game's BepInEx log"
	ButtonCopyPeerID  = "Copy peer id"
	ButtonCopyLog     = "Copy the log"

	// ButtonStartWindowed and ButtonStartHeadless are the same button, whose
	// caption flips with the world's own setting: it always offers the OTHER way
	// round, for this session only.
	ButtonStartWindowed = "Start with a window (this time only)"
	ButtonStartHeadless = "Start with no window (this time only)"
)

// The dialogs' own buttons. The accepting button says WHAT IT DOES rather than
// "OK": the create dialog's button enrolls an identity on a shared map, and the
// delete dialog's deletes a world, and neither is a thing to agree to by
// pressing the word "OK".
const (
	ButtonDialogSave   = "Save"
	ButtonDialogCreate = "Create and enroll"
	ButtonDialogClone  = "Clone and enroll"
	ButtonDialogDelete = "Delete this world"
	ButtonDialogCancel = "Cancel"
)

// StartOverrideCaption is the caption of the override button for a world whose
// profile says headless or does not.
func StartOverrideCaption(headless bool) string {
	if headless {
		return ButtonStartWindowed
	}
	return ButtonStartHeadless
}

// Actions is which buttons a person can press. Every one of them is disabled
// when the core would refuse it, so a refusal is a message the participant does
// not have to read.
type Actions struct {
	Start          bool
	StartOverride  bool
	Stop           bool
	StopAll        bool
	SetDefault     bool
	Edit           bool
	Create         bool
	Clone          bool
	Delete         bool
	Diagnose       bool
	OpenLogs       bool
	OpenGameLog    bool
	CopyPeerID     bool
	OverrideOffers string
}

// ActionsFor decides the buttons for one selection.
//
// selected is nil when nothing is selected, which is the state an installation
// with no world at all is permanently in — and the state in which the only
// useful button is the one that makes a world.
//
// busy is an action already running. The core's per-world lock would refuse a
// second one anyway, and it is a better experience to grey the button than to
// print "another launcher is starting or stopping this world".
func ActionsFor(selected *launcher.WorldView, snap launcher.Snapshot, busy bool) Actions {
	if busy {
		// Nothing but reading, and the log pane keeps filling. The override
		// button keeps the caption it had: a caption that flipped while an action
		// ran and flipped back afterwards would read as the world's setting
		// having changed.
		held := Actions{OverrideOffers: ButtonStartHeadless}
		if selected != nil {
			held.OverrideOffers = StartOverrideCaption(selected.Headless)
		}
		return held
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
		Create:         true,
		OverrideOffers: ButtonStartHeadless,
	}
	if selected == nil {
		return actions
	}
	running := selected.Sidecar.Running || selected.Game.Running
	actions.Start = !running
	actions.StartOverride = !running
	actions.Stop = running
	actions.SetDefault = !selected.Active
	actions.Edit = true
	actions.Clone = true
	// The core refuses to delete a world that is running, and it is right to:
	// the profile is how its processes are found again.
	actions.Delete = !running
	actions.Diagnose = true
	actions.OpenLogs = true
	actions.OpenGameLog = true
	actions.CopyPeerID = selected.PeerID != ""
	actions.OverrideOffers = StartOverrideCaption(selected.Headless)
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
		if after.Headless {
			flags = append(flags, "--headless")
		} else {
			flags = append(flags, "--no-headless")
		}
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

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func trim(value string) string { return strings.TrimSpace(value) }

// ---------------------------------------------------------------- the log pane

// Log is the writer the core prints into, turned into whole timestamped lines.
//
// WHY IT IS NOT JUST AN APPEND. The core writes with fmt.Fprintf, so one line
// can arrive in several writes and several lines can arrive in one; a stamp per
// WRITE would put a time in the middle of a sentence, and appending raw bytes
// would leave a half line at the bottom of the pane for as long as the next
// write takes — which, during the two-minute wait for a game's mod, is a long
// time to look at an unfinished sentence.
//
// It is safe for several goroutines, because both of the core's streams are
// pointed at it and the action goroutine is not the only thing that writes.
type Log struct {
	mu      sync.Mutex
	now     func() time.Time
	pending strings.Builder
	emit    func(line string)
}

// NewLog makes a log whose finished lines go to emit. emit is called with the
// lock released, from whatever goroutine wrote, and it must be safe there: the
// window's own emit hands the line to the UI thread.
func NewLog(now func() time.Time, emit func(line string)) *Log {
	if now == nil {
		now = time.Now
	}
	return &Log{now: now, emit: emit}
}

// Write takes whatever the core printed and emits the lines that are complete.
func (l *Log) Write(p []byte) (int, error) {
	var ready []string
	l.mu.Lock()
	for _, b := range p {
		switch b {
		case '\n':
			ready = append(ready, l.stampLocked())
		case '\r':
			// A stray carriage return would draw as a box in a Windows edit
			// control. The line ends at the newline that follows it.
		default:
			l.pending.WriteByte(b)
		}
	}
	l.mu.Unlock()
	for _, line := range ready {
		l.emitLine(line)
	}
	return len(p), nil
}

// Flush emits whatever is pending without a newline, which is what a prompt is.
// The window calls it when an action ends, so nothing sits half-written.
func (l *Log) Flush() {
	l.mu.Lock()
	if l.pending.Len() == 0 {
		l.mu.Unlock()
		return
	}
	line := l.stampLocked()
	l.mu.Unlock()
	l.emitLine(line)
}

func (l *Log) emitLine(line string) {
	if l.emit != nil {
		l.emit(line)
	}
}

// stampLocked takes the pending line and puts the time on the front of it.
func (l *Log) stampLocked() string {
	text := l.pending.String()
	l.pending.Reset()
	if text == "" {
		// An empty line is a blank line the core printed on purpose — the start
		// sequence uses them to separate its blocks — and a bare timestamp would
		// undo the spacing. It stays empty.
		return ""
	}
	return l.now().Format("15:04:05") + "  " + text
}

// ---------------------------------------------------------------- the log pane

// LogFollowsTail answers the one question an appending log pane has to ask
// before it scrolls: is the reader at the bottom, watching it happen, or have
// they scrolled up to read something?
//
// A pane that always jumps to the newest line drags a person off the line they
// were reading every hundred milliseconds, and a start prints for two minutes.
// A pane that never jumps shows the two oldest lines of a session forever, which
// is the bug this was written for. So it follows while the bar is at the bottom
// and stops the moment it is not.
//
// The three numbers are the vertical scroll bar's own, as Windows reports them
// (SCROLLINFO with SIF_RANGE|SIF_PAGE|SIF_POS, normalised so the range starts at
// zero): the first visible line, how many lines are visible, and the last line
// the range covers. Windows leaves nMax at the LAST LINE rather than the last
// line one can scroll to, so at the bottom is pos+page == max+1 — and one line
// of slack is allowed, because a pane whose last line is half-hidden by a
// partial row is still a pane somebody is watching.
//
// page == 0 means there is no scroll bar to read: everything fits, and following
// costs nothing.
func LogFollowsTail(pos, page, max int) bool {
	if page <= 0 {
		return true
	}
	return pos+page >= max
}

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
