//go:build windows

package launchergui

// THE WINDOW, AND ITS ONE RULE ABOUT THREADS.
//
// Win32 is single-threaded per window: everything that touches a control has to
// happen on the thread that runs the message loop. Everything this program does
// that is worth doing takes SECONDS — a start waits up to a minute for the map
// to grant a place and then up to two minutes for the game's mod to arrive, a
// stop waits for a world to save, an enrollment is a network round trip. Doing
// any of that on the message loop would freeze the window for the whole of it,
// which is exactly the shape a person reads as "it has crashed".
//
// So there are four goroutines and one rule:
//
//   1. THE UI THREAD runs walk's loop. It draws, it opens dialogs, it decides
//      which buttons are enabled. It NEVER reads a file, asks a port or waits.
//   2. THE REFRESHER takes a snapshot every refreshInterval — profiles, the pid
//      ledger, and each running sidecar's own-slot endpoint — and hands it to
//      the UI thread with Synchronize.
//   3. THE ACTION goroutine runs one action at a time, in order. One at a time
//      is deliberate: the core's per-world lock would refuse a second one, and a
//      queue of clicked buttons is easier to reason about than a race between
//      them.
//   4. THE LOG PUMP moves finished lines into the pane in batches, because the
//      core can print a hundred lines in a second and one Synchronize per line
//      is a hundred messages.
//
// Synchronize is walk's own mechanism for "run this on the UI thread", and it is
// the only way any of the other three touch a control.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"github.com/lxn/win"

	"multiverse/internal/launcher"
)

// refreshInterval is how often the world list is re-read. It is the same order
// as the sidecar's own heartbeat, and every reading it takes is local.
const refreshInterval = 2 * time.Second

// logPaneLimit bounds the pane. A long session prints a great deal, and an edit
// control that grows without limit is a window that slows down and then a
// process that runs out of memory. When it is reached the oldest half goes; the
// event log under each world's data root is the permanent record, and the pane
// says so when it trims.
const logPaneLimit = 200000

// Options is what the executable hands the window.
type Options struct {
	// InstallRoot overrides the discovery order. It is empty in the shipped
	// program: the window is installed beside the sidecar and the profiles.
	InstallRoot string
	Now         func() time.Time
}

// Run opens the window and returns when it is closed. A world that is running
// keeps running: closing this window stops nothing, which the status bar says
// out loud.
func Run(opts Options) error {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	u := &ui{
		actions: make(chan func(), 1),
		wake:    make(chan struct{}, 1),
		refresh: make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	u.log = NewLog(opts.Now, u.appendLine)

	session, err := launcher.NewSession(launcher.SessionOptions{
		InstallRoot: opts.InstallRoot,
		Out:         u.log,
		Err:         u.log,
		Now:         opts.Now,
	})
	if err != nil {
		// There is no window yet and there never will be: this installation
		// cannot say where it is. A message box is the only place left to say so.
		walk.MsgBox(nil, WindowTitle, err.Error(), walk.MsgBoxIconError|walk.MsgBoxOK)
		return err
	}
	u.session = session

	if err := u.build(); err != nil {
		walk.MsgBox(nil, WindowTitle, err.Error(), walk.MsgBoxIconError|walk.MsgBoxOK)
		return err
	}
	go u.refresher()
	go u.actor()
	go u.logPump()
	// The first reading is asked for before the loop starts, so the list is
	// filled by the time the window is on screen rather than two seconds after.
	u.askForRefresh()
	u.mw.Run()
	close(u.done)
	return nil
}

// ui is the whole window. Every field is either read-only after build, or
// documented with the thread that owns it.
type ui struct {
	session *launcher.Session
	log     *Log

	mw        *walk.MainWindow
	table     *walk.TableView
	model     *worldModel
	logView   *walk.TextEdit
	banner    *walk.TextLabel
	statusBar *walk.StatusBarItem
	override  *walk.PushButton
	// buttons and menuActions are keyed by the caption, which is the same
	// caption for a button and for the menu item that does the same thing, so
	// one rule enables both. They hold the ADDRESS declarative writes the widget
	// into, because a widget does not exist until Create runs.
	buttons     map[string]**walk.PushButton
	menuActions map[string]**walk.Action

	// UI THREAD ONLY.
	snapshot launcher.Snapshot
	selected string
	busy     bool

	// mu guards what crosses a thread boundary and nothing else.
	mu      sync.Mutex
	pending []string

	actions chan func()
	wake    chan struct{}
	refresh chan struct{}
	done    chan struct{}
}

// ---------------------------------------------------------------- the widgets

func (u *ui) build() error {
	u.model = &worldModel{}
	u.buttons = make(map[string]**walk.PushButton)
	u.menuActions = make(map[string]**walk.Action)

	columns := make([]d.TableViewColumn, 0, len(Columns()))
	for _, column := range Columns() {
		columns = append(columns, d.TableViewColumn{Title: column.Title, Width: column.Width})
	}

	window := d.MainWindow{
		AssignTo: &u.mw,
		Title:    WindowTitle,
		MinSize:  d.Size{Width: 900, Height: 600},
		Size:     d.Size{Width: 1180, Height: 760},
		Layout:   d.VBox{},
		MenuItems: []d.MenuItem{
			d.Menu{
				Text: "&Worlds",
				Items: []d.MenuItem{
					d.Action{Text: "&Refresh now", OnTriggered: func() { u.askForRefresh() }},
					d.Separator{},
					d.Action{Text: ButtonStopAll, AssignTo: u.action(ButtonStopAll),
						OnTriggered: u.onStopAll},
					d.Separator{},
					d.Action{Text: "Open the &console launcher", OnTriggered: u.onOpenConsole},
					d.Separator{},
					d.Action{Text: "&Quit", OnTriggered: func() { u.mw.Close() }},
				},
			},
			d.Menu{
				Text: "&Help",
				Items: []d.MenuItem{
					d.Action{Text: "Documentation (bibitesmultiverse.com)", OnTriggered: u.onDocs},
					d.Separator{},
					d.Action{Text: "&About", OnTriggered: u.onAbout},
				},
			},
		},
		StatusBarItems: []d.StatusBarItem{
			{AssignTo: &u.statusBar, Text: CloseHint, Width: 900},
		},
		Children: []d.Widget{
			// MinSize.Width IS WHAT MAKES IT WRAP. Without it a label is measured
			// as one line, and this one carries every unreadable profile's whole
			// refusal — so the window's minimum width would grow to fit the lot
			// on one line and the text would run off the edge of the screen.
			d.TextLabel{AssignTo: &u.banner, Text: "", TextColor: walk.RGB(160, 0, 0),
				MinSize: d.Size{Width: 860}},
			d.VSplitter{
				Children: []d.Widget{
					d.TableView{
						AssignTo:              &u.table,
						Model:                 u.model,
						Columns:               columns,
						LastColumnStretched:   true,
						MultiSelection:        false,
						StretchFactor:         3,
						OnCurrentIndexChanged: u.onSelectionChanged,
						OnItemActivated:       u.onStart,
						ContextMenuItems: []d.MenuItem{
							d.Action{Text: ButtonStart, AssignTo: u.action(ButtonStart), OnTriggered: u.onStart},
							d.Action{Text: ButtonStop, AssignTo: u.action(ButtonStop), OnTriggered: u.onStop},
							d.Separator{},
							d.Action{Text: ButtonSetDefault, AssignTo: u.action(ButtonSetDefault),
								OnTriggered: u.onSetDefault},
							d.Action{Text: ButtonEdit, AssignTo: u.action(ButtonEdit), OnTriggered: u.onEdit},
							d.Action{Text: ButtonClone, AssignTo: u.action(ButtonClone), OnTriggered: u.onClone},
							d.Action{Text: ButtonDelete, AssignTo: u.action(ButtonDelete), OnTriggered: u.onDelete},
							d.Separator{},
							d.Action{Text: ButtonDiagnose, AssignTo: u.action(ButtonDiagnose),
								OnTriggered: u.onDiagnose},
							d.Action{Text: ButtonOpenLogs, AssignTo: u.action(ButtonOpenLogs),
								OnTriggered: u.onOpenLogs},
							d.Action{Text: ButtonOpenGameLog, AssignTo: u.action(ButtonOpenGameLog),
								OnTriggered: u.onOpenGameLog},
							d.Action{Text: ButtonCopyPeerID, AssignTo: u.action(ButtonCopyPeerID),
								OnTriggered: u.onCopyPeerID},
						},
					},
					d.Composite{
						Layout:        d.VBox{MarginsZero: true},
						StretchFactor: 2,
						Children: []d.Widget{
							d.TextEdit{
								AssignTo: &u.logView,
								ReadOnly: true,
								VScroll:  true,
								HScroll:  true,
								// AN EDIT CONTROL HAS ITS OWN CEILING, about
								// 32,767 characters, and past it every append is
								// silently dropped — the pane would freeze on old
								// text with nothing to say it had, and Copy the log
								// would copy that. It is raised above the trimming
								// limit below, which is what actually bounds this.
								MaxLength: logPaneLimit * 2,
								Font:      d.Font{Family: "Consolas", PointSize: 9},
							},
						},
					},
				},
			},
			d.Composite{
				Layout: d.HBox{MarginsZero: true},
				Children: []d.Widget{
					u.button(ButtonStart, u.onStart),
					u.overrideButton(),
					u.button(ButtonStop, u.onStop),
					u.button(ButtonStopAll, u.onStopAll),
					u.button(ButtonSetDefault, u.onSetDefault),
					u.button(ButtonEdit, u.onEdit),
					d.HSpacer{},
				},
			},
			d.Composite{
				Layout: d.HBox{MarginsZero: true},
				Children: []d.Widget{
					u.button(ButtonCreate, u.onCreate),
					u.button(ButtonClone, u.onClone),
					u.button(ButtonDelete, u.onDelete),
					u.button(ButtonDiagnose, u.onDiagnose),
					u.button(ButtonOpenLogs, u.onOpenLogs),
					u.button(ButtonOpenGameLog, u.onOpenGameLog),
					u.button(ButtonCopyPeerID, u.onCopyPeerID),
					u.button(ButtonCopyLog, u.onCopyLog),
					d.HSpacer{},
				},
			},
		},
	}
	if err := window.Create(); err != nil {
		return err
	}
	u.banner.SetVisible(false)
	if icon := windowIcon(); icon != nil {
		u.mw.SetIcon(icon)
	}
	u.applyActions()
	fmt.Fprintf(u.log, "Bibites Multiverse launcher %s, installed in %s\n",
		launcher.Release, u.session.InstallRoot())
	fmt.Fprintf(u.log, "%s. The commands are in %s.\n", CloseHint, launcher.ConsoleExeName)
	return nil
}

// button registers one push button under its caption, which is how the harness
// finds it and how applyActions enables it.
func (u *ui) button(caption string, onClicked func()) d.PushButton {
	holder := new(*walk.PushButton)
	u.buttons[caption] = holder
	return d.PushButton{AssignTo: holder, Text: caption, OnClicked: onClicked}
}

// overrideButton is the per-session headless switch. Its caption changes with
// the selected world, so it is held on its own rather than by caption.
func (u *ui) overrideButton() d.PushButton {
	return d.PushButton{AssignTo: &u.override, Text: ButtonStartHeadless, OnClicked: u.onStartOverride}
}

// action registers a menu item under the same caption as the button that does
// the same thing, so the two are enabled and disabled together.
func (u *ui) action(caption string) **walk.Action {
	holder := new(*walk.Action)
	u.menuActions[caption] = holder
	return holder
}

// windowIcon is the icon compiled into this executable by the resource object
// (see cmd/multiverse-launcher-gui). rsrc numbers what it is given from 1 — the
// manifest, then the icon group, then the group's images — so with the manifest
// first the group is 2 and the first probe below finds it. The loop is there
// because that ordering is rsrc's and not ours: a resource id that holds no icon
// group answers with an error rather than an icon, never a panic, and a window
// with no icon is a cosmetic loss rather than a reason to refuse to open.
func windowIcon() *walk.Icon {
	for id := 2; id <= 32; id++ {
		if icon, err := walk.NewIconFromResourceId(id); err == nil {
			return icon
		}
	}
	return nil
}

// worldModel is the table's model. It is REPLACED WHOLE on every refresh rather
// than mutated: a snapshot is one consistent reading, and a table that took
// half of one and half of the next would show a world that never existed.
type worldModel struct {
	walk.TableModelBase
	rows []Row
}

func (m *worldModel) RowCount() int { return len(m.rows) }

func (m *worldModel) Value(row, col int) interface{} {
	if row < 0 || row >= len(m.rows) {
		return ""
	}
	return m.rows[row].Cell(col)
}

// ---------------------------------------------------------------- the threads

// refresher is goroutine 2. It never touches a control.
func (u *ui) refresher() {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		snap := u.session.Snapshot()
		u.mw.Synchronize(func() { u.applySnapshot(snap) })
		select {
		case <-u.done:
			return
		case <-u.refresh:
		case <-ticker.C:
		}
	}
}

// askForRefresh asks for a reading now rather than at the next tick. It is
// non-blocking: one pending request is as good as ten.
func (u *ui) askForRefresh() {
	select {
	case u.refresh <- struct{}{}:
	default:
	}
}

// actor is goroutine 3: one action at a time, in the order they were clicked.
func (u *ui) actor() {
	for {
		select {
		case <-u.done:
			return
		case fn := <-u.actions:
			fn()
			// Anything the core left half-written — a prompt, which a session
			// answers rather than asks — goes to the pane now.
			u.log.Flush()
			u.mw.Synchronize(func() {
				u.busy = false
				u.applyActions()
			})
			u.askForRefresh()
		}
	}
}

// logPump is goroutine 4. It batches, because the start sequence prints a great
// deal in a short time.
func (u *ui) logPump() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-u.done:
			return
		case <-u.wake:
		case <-ticker.C:
		}
		u.mu.Lock()
		lines := u.pending
		u.pending = nil
		u.mu.Unlock()
		if len(lines) == 0 {
			continue
		}
		u.mw.Synchronize(func() { u.writeLines(lines) })
	}
}

// appendLine is called by the Log, from whatever goroutine printed.
func (u *ui) appendLine(line string) {
	u.mu.Lock()
	u.pending = append(u.pending, line)
	u.mu.Unlock()
	select {
	case u.wake <- struct{}{}:
	default:
	}
}

// ---------------------------------------------------------------- the UI thread

// writeLines appends to the pane and keeps it bounded.
func (u *ui) writeLines(lines []string) {
	// Whether to follow is decided BEFORE the append, because appending is what
	// moves the bottom away from wherever the reader is.
	follow := u.logFollowsTail()
	// A Windows edit control's line ending is CRLF; a bare newline draws as one
	// long line.
	u.logView.AppendText(strings.Join(lines, "\r\n") + "\r\n")
	if u.logView.TextLength() > logPaneLimit {
		text := u.logView.Text()
		cut := len(text) / 2
		if index := strings.Index(text[cut:], "\r\n"); index >= 0 {
			cut += index + 2
		} else {
			// No line ending in the second half: cut on a rune boundary instead,
			// so a path with an accent in it does not become a replacement
			// character at the top of the pane.
			for cut < len(text) && !utf8.RuneStart(text[cut]) {
				cut++
			}
		}
		u.logView.SetText("[the older half of this session's log was trimmed here; " +
			"each world's own log folder keeps the whole of it]\r\n" + text[cut:])
		// A trim replaces the whole document, so nobody is where they were. The
		// newest line is the one place worth landing.
		follow = true
	}
	if follow {
		u.scrollLogToEnd()
	}
}

// scrollLogToEnd puts the newest line on screen.
//
// WHY ScrollToCaret ALONE DID NOTHING, which is what a machine showed: 191 lines
// in the pane and EM_GETFIRSTVISIBLELINE still 0. walk's AppendText saves the
// current selection, appends at the end, AND PUTS THE SELECTION BACK
// (walk/textedit.go). In a pane nobody has ever clicked in, that selection is
// (0,0) — so by the time ScrollToCaret sends EM_SCROLLCARET the caret is back at
// the very top, and the control does exactly what it was asked: it scrolls the
// caret into view, at line 0. It was not a scroll that failed; it was a scroll to
// the wrong place.
//
// So the caret is moved to the end HERE, after the append, and then the pane is
// asked twice: EM_SCROLLCARET, which is the caret's way, and WM_VSCROLL
// SB_BOTTOM, which needs no caret at all. The second is the belt to the first's
// braces — this is a READ-ONLY edit, which is exactly the kind that is entitled
// to ignore a caret operation, and it costs one message.
//
// A SELECTION SOMEBODY MADE IS NOT MOVED. If there is one, only the scroll
// message is sent: dragging a person's own selection out from under them to copy
// a line is worse than not following.
func (u *ui) scrollLogToEnd() {
	if start, end := u.logView.TextSelection(); start == end {
		length := u.logView.TextLength()
		u.logView.SetTextSelection(length, length)
		u.logView.ScrollToCaret()
	}
	u.logView.SendMessage(win.WM_VSCROLL, uintptr(win.SB_BOTTOM), 0)
}

// logFollowsTail reads the pane's scroll bar and answers whether the newest line
// should be scrolled to. When the bar cannot be read the answer is yes: a log
// that follows is the behaviour somebody asked for, and a log that silently
// stopped following is the defect.
func (u *ui) logFollowsTail() bool {
	info := win.SCROLLINFO{FMask: win.SIF_RANGE | win.SIF_PAGE | win.SIF_POS}
	info.CbSize = uint32(unsafe.Sizeof(info))
	if !win.GetScrollInfo(u.logView.Handle(), win.SB_VERT, &info) {
		return true
	}
	return LogFollowsTail(int(info.NPos-info.NMin), int(info.NPage), int(info.NMax-info.NMin))
}

// applySnapshot redraws the list, keeping the selection on the world it was on.
func (u *ui) applySnapshot(snap launcher.Snapshot) {
	u.snapshot = snap
	u.model.rows = RowsFrom(snap)
	u.model.PublishRowsReset()

	if u.selected == "" {
		u.selected = snap.Active
	}
	index := -1
	for i, row := range u.model.rows {
		if strings.EqualFold(row.Name, u.selected) {
			index = i
			break
		}
	}
	if index < 0 && len(u.model.rows) > 0 {
		index = 0
		u.selected = u.model.rows[0].Name
	}
	if index >= 0 {
		u.table.SetCurrentIndex(index)
	} else {
		u.selected = ""
	}

	// THE PROBLEMS ARE A BANNER, NOT AN EMPTY LIST. An installation whose
	// profiles cannot be read used to answer "no worlds", which reads as
	// "nothing is installed" from a program that simply could not read its own
	// files.
	if len(snap.Problems) > 0 {
		u.banner.SetText("This installation has " + countWords(len(snap.Problems)) +
			" the launcher could not read: " + strings.Join(snap.Problems, " | "))
		u.banner.SetVisible(true)
	} else if len(snap.Worlds) == 0 {
		u.banner.SetText("This installation has no worlds yet. Use " + ButtonCreate +
			", or re-run the installer.")
		u.banner.SetVisible(true)
	} else {
		u.banner.SetVisible(false)
	}
	u.statusBar.SetText(fmt.Sprintf("%s   -   %d world(s) in %s",
		CloseHint, len(snap.Worlds), snap.InstallRoot))
	u.applyActions()
}

func countWords(n int) string {
	if n == 1 {
		return "one file"
	}
	return fmt.Sprintf("%d files", n)
}

func (u *ui) onSelectionChanged() {
	index := u.table.CurrentIndex()
	if index >= 0 && index < len(u.model.rows) {
		u.selected = u.model.rows[index].Name
	}
	u.applyActions()
}

// selectedWorld is the world the buttons act on, or nil when there is none.
func (u *ui) selectedWorld() *launcher.WorldView {
	for i := range u.snapshot.Worlds {
		if strings.EqualFold(u.snapshot.Worlds[i].Name, u.selected) {
			return &u.snapshot.Worlds[i]
		}
	}
	return nil
}

// applyActions greys every button the core would refuse.
func (u *ui) applyActions() {
	actions := ActionsFor(u.selectedWorld(), u.snapshot, u.busy)
	enabled := map[string]bool{
		ButtonStart:       actions.Start,
		ButtonStop:        actions.Stop,
		ButtonStopAll:     actions.StopAll,
		ButtonSetDefault:  actions.SetDefault,
		ButtonEdit:        actions.Edit,
		ButtonCreate:      actions.Create,
		ButtonClone:       actions.Clone,
		ButtonDelete:      actions.Delete,
		ButtonDiagnose:    actions.Diagnose,
		ButtonOpenLogs:    actions.OpenLogs,
		ButtonOpenGameLog: actions.OpenGameLog,
		ButtonCopyPeerID:  actions.CopyPeerID,
		ButtonCopyLog:     true,
	}
	for caption, holder := range u.buttons {
		if button := *holder; button != nil {
			button.SetEnabled(enabled[caption])
		}
	}
	for caption, holder := range u.menuActions {
		if action := *holder; action != nil {
			action.SetEnabled(enabled[caption])
		}
	}
	if u.override != nil {
		u.override.SetText(actions.OverrideOffers)
		u.override.SetEnabled(actions.StartOverride)
	}
}

// start queues one action. The window says what it started, so the log pane
// reads as a session rather than as a stream of unattributed output.
func (u *ui) start(what string, fn func()) {
	if u.busy {
		return
	}
	u.busy = true
	u.applyActions()
	fmt.Fprintf(u.log, "\n> %s\n", what)
	select {
	case u.actions <- fn:
	default:
		// The queue is one deep and the busy flag is what keeps it that way; if
		// it is ever full, say so rather than block the message loop.
		u.busy = false
		u.applyActions()
		fmt.Fprintf(u.log, "another action is still running; nothing was started\n")
	}
}

// ---------------------------------------------------------------- the actions

func (u *ui) onStart() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	// The button for this is greyed when a world is already running, but a
	// double-click on the row reaches here too, and the same rule has to hold on
	// both paths.
	if !ActionsFor(world, u.snapshot, u.busy).Start {
		return
	}
	name := world.Name
	u.start("start "+name, func() { u.session.Start(name, launcher.StartOptions{}) })
}

// onStartOverride is the switch the console menu never had: run THIS session the
// other way round, with a window or without one, and leave the world alone.
func (u *ui) onStartOverride() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	name := world.Name
	headless := !world.Headless
	what := fmt.Sprintf("start %s with no window (this time only)", name)
	if !headless {
		what = fmt.Sprintf("start %s with a window (this time only)", name)
	}
	u.start(what, func() {
		u.session.Start(name, launcher.StartOptions{Headless: &headless})
	})
}

func (u *ui) onStop() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	name := world.Name
	u.start("stop "+name, func() { u.session.Stop(name, false, 0) })
}

func (u *ui) onStopAll() {
	u.start("stop every world", func() { u.session.StopAll(0) })
}

func (u *ui) onSetDefault() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	name := world.Name
	u.start("make "+name+" the default world", func() { u.session.SetDefault(name) })
}

func (u *ui) onEdit() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	name := world.Name
	current, err := u.session.World(name)
	if err != nil {
		u.warn(err.Error())
		return
	}
	form, ok, err := runEditDialog(u.mw, current)
	if err != nil {
		u.warn(err.Error())
		return
	}
	if !ok {
		return
	}
	flags := EditFlags(current, form)
	if len(flags) == 0 {
		fmt.Fprintf(u.log, "nothing about '%s' was changed.\n", name)
		return
	}
	u.start("edit "+name+" ("+strings.Join(flags, " ")+")", func() {
		u.session.Edit(name, flags)
	})
}

func (u *ui) onCreate() {
	// THE NAME IS CHOSEN FIRST, because the defaults are derived FROM it: the
	// data folder a new world is offered is named after it. Asking for defaults
	// under one name and then renaming the world left the second create dialog
	// offering the FIRST new world's data folder, which the core refuses — two
	// worlds over one journal strand both.
	spec, defaultsErr := u.session.NewWorldDefaults(NextFreeWorldName(u.snapshot))
	if defaultsErr != nil {
		// An installation with nothing in it can still be given a world, but the
		// person has to say where the game is. The dialog opens with the fields
		// empty and the reason shown.
		u.warn(defaultsErr.Error())
	}
	created, ok, err := runCreateDialog(u.mw, spec)
	if err != nil {
		u.warn(err.Error())
		return
	}
	if !ok {
		return
	}
	u.start("create "+created.Name+" and enroll a new identity on the map", func() {
		u.session.Create(created)
	})
}

func (u *ui) onClone() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	source := world.Name
	name, ok, err := runCloneDialog(u.mw, source, NextFreeWorldName(u.snapshot))
	if err != nil {
		u.warn(err.Error())
		return
	}
	if !ok {
		return
	}
	u.start("clone "+source+" as "+name, func() { u.session.Clone(source, name) })
}

func (u *ui) onDelete() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	name := world.Name
	typed, removeData, ok, err := runDeleteDialog(u.mw, name, world.DataRoot)
	if err != nil {
		u.warn(err.Error())
		return
	}
	if !ok {
		return
	}
	u.start("delete "+name, func() { u.session.Delete(name, removeData, typed) })
}

func (u *ui) onDiagnose() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	name := world.Name
	u.start("check "+name, func() { u.session.Diagnose(name) })
}

func (u *ui) onOpenLogs() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	if err := u.session.OpenLogFolder(world.Name); err != nil {
		u.warn(err.Error())
	}
}

func (u *ui) onOpenGameLog() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	if err := u.session.OpenGameLog(world.Name); err != nil {
		u.warn(err.Error())
	}
}

func (u *ui) onCopyPeerID() {
	world := u.selectedWorld()
	if world == nil || world.PeerID == "" {
		return
	}
	if err := walk.Clipboard().SetText(world.PeerID); err != nil {
		u.warn(err.Error())
		return
	}
	fmt.Fprintf(u.log, "copied this world's identity on the map: %s\n", world.PeerID)
}

func (u *ui) onCopyLog() {
	if err := walk.Clipboard().SetText(u.logView.Text()); err != nil {
		u.warn(err.Error())
	}
}

// onOpenConsole opens the console launcher, which is the same program with the
// commands and the menu on it.
//
// IT GOES THROUGH THE SHELL, AND THAT IS NOT LAZINESS. A window has no console
// to lend, and os/exec ALWAYS passes standard handles to a child
// (STARTF_USESTDHANDLES is set unconditionally in syscall's exec), opening the
// null device for any stream left nil. A console launcher started that way gets
// a console window of its own AND the null device for all three streams: NUL is
// a character device, so the menu opens, draws into nothing, reads end-of-input
// on its first prompt and quits. A blank console flashes and vanishes.
//
// `cmd /c start "" <exe>` hands the job to the shell, which creates the console
// and its handles the way it does for anything a person types. The empty string
// is start's title argument, and it is required: without it start would read a
// quoted path AS the title and open a console with nothing in it.
func (u *ui) onOpenConsole() {
	exe, err := os.Executable()
	if err != nil {
		u.warn(err.Error())
		return
	}
	console := ConsoleExePath(exe)
	cmd := exec.Command(comSpec(), "/c", "start", "", console)
	cmd.Dir = u.session.InstallRoot()
	if err := cmd.Start(); err != nil {
		u.warn(fmt.Sprintf("could not open %s: %v", console, err))
		return
	}
	fmt.Fprintf(u.log, "opened %s in a console window of its own.\n", console)
}

// comSpec prefers the shell this Windows says it has, and falls back to the
// name rather than to a path this machine may not have.
func comSpec() string {
	if shell := os.Getenv("ComSpec"); shell != "" {
		return shell
	}
	return "cmd.exe"
}

func (u *ui) onDocs() {
	// url.dll's protocol handler is Windows' own answer to "open this in
	// whatever the user browses with", and it needs no shell parsing of a string
	// somebody could have put a quote in.
	if err := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", DocsURL).Start(); err != nil {
		u.warn(err.Error())
	}
}

func (u *ui) onAbout() {
	walk.MsgBox(u.mw, "About "+WindowTitle, strings.Join([]string{
		fmt.Sprintf("Bibites Multiverse launcher %s", launcher.Release),
		"",
		"installed in " + u.session.InstallRoot(),
		"the sidecar: " + u.session.SidecarExe(),
		"the commands: " + launcher.ConsoleExeName,
		"",
		DocsURL,
		"",
		CloseHint + ".",
	}, "\n"), walk.MsgBoxIconInformation|walk.MsgBoxOK)
}

// warn says something the window itself decided, in the pane and nowhere else:
// a message box for every refusal would be a window a person has to dismiss
// before they can read what it was about.
func (u *ui) warn(message string) {
	fmt.Fprintf(u.log, "%s\n", message)
}
