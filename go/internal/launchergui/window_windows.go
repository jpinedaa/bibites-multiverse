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
//  1. THE UI THREAD runs walk's loop. It draws, it opens dialogs, it decides
//     which buttons are enabled. It NEVER reads a file, asks a port or waits.
//  2. THE REFRESHER takes a snapshot every refreshInterval — profiles, the pid
//     ledger, and each running sidecar's own-slot endpoint — and hands it to
//     the UI thread with Synchronize.
//  3. THE ACTION goroutine runs one action at a time, in order. One at a time
//     is deliberate: the core's per-world lock would refuse a second one, and a
//     queue of clicked buttons is easier to reason about than a race between
//     them.
//  4. THE LOG PUMP moves finished lines into the pane in batches, because the
//     core can print a hundred lines in a second and one Synchronize per line
//     is a hundred messages.
//
// Synchronize is walk's own mechanism for "run this on the UI thread", and it is
// the only way any of the other three touch a control.
//
// WHAT THIS FILE DECIDES, AND WHAT IT DOES NOT. It builds widgets and moves
// values into them. Every word it puts on screen, every colour, every enabling
// rule and every phrase it makes of the core's output comes from view.go,
// details.go and windowstate.go — which carry no build tag and are tested on a
// machine with no Windows at all.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// logPaneLimit bounds the details pane. A long session prints a great deal, and
// an edit control that grows without limit is a window that slows down and then
// a process that runs out of memory. When it is reached the oldest half goes;
// the event log under each world's data root is the permanent record, and the
// pane says so when it trims.
const logPaneLimit = 200000

// The window's own metrics, in the 96-dpi units walk lays out in.
const (
	defaultWindowWidth  = 1080
	defaultWindowHeight = 700
	// primaryButtonWidth and primaryButtonHeight are what make Start the button
	// a person's eye lands on. It is the only oversized control in the window.
	primaryButtonWidth  = 190
	primaryButtonHeight = 40
)

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

	mw     *walk.MainWindow
	table  *walk.TableView
	model  *worldModel
	banner *walk.TextLabel

	// The panel: one world, in plain words.
	worldName  *walk.TextLabel
	headline   *walk.TextLabel
	hint       *walk.TextLabel
	spinner    *walk.ProgressBar
	resultLine *walk.TextLabel
	primary    *walk.PushButton
	headless   *walk.CheckBox
	// factValue is the panel's grid, by label. factHolders is where declarative
	// writes the labels before Create has run.
	factValue   map[string]*walk.TextLabel
	factHolders []factHolder

	// The details pane, which is collapsed until it is wanted or until something
	// goes wrong.
	detailsToggle *walk.PushButton
	detailsPane   *walk.Composite
	logView       *walk.TextEdit

	statusBar *walk.StatusBarItem

	// controls are keyed by CAPTION, which is the same caption for a button and
	// for the menu item that does the same thing, so one rule enables both. A
	// caption may hold several controls for exactly that reason.
	buttons map[string][]**walk.PushButton
	menus   map[string][]**walk.Action

	// UI THREAD ONLY.
	snapshot launcher.Snapshot
	selected string
	job      Job
	busy     bool
	phrase   string
	// said is the last flush-left line the core printed during the job that is
	// running, which is what a failure quotes. See SaidLine.
	said        string
	result      Result
	detailsOpen bool
	// settingHeadless is up while the refresher is writing the checkbox, so that
	// the write does not read as a person having clicked it. Without it every
	// refresh of a headless world would queue an edit, forever.
	settingHeadless bool

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
	u.buttons = make(map[string][]**walk.PushButton)
	u.menus = make(map[string][]**walk.Action)
	u.factValue = make(map[string]*walk.TextLabel, len(FactLabels()))

	columns := make([]d.TableViewColumn, 0, len(Columns()))
	for _, column := range Columns() {
		columns = append(columns, d.TableViewColumn{Title: column.Title, Width: column.Width})
	}

	window := d.MainWindow{
		AssignTo: &u.mw,
		Title:    WindowTitle,
		MinSize:  d.Size{Width: MinWindowWidth, Height: MinWindowHeight},
		Size:     d.Size{Width: defaultWindowWidth, Height: defaultWindowHeight},
		Layout:   d.VBox{},
		MenuItems: []d.MenuItem{
			d.Menu{
				Text: MenuWorld,
				Items: []d.MenuItem{
					u.item(ButtonStart, u.onPrimaryStart),
					u.item(ButtonStop, u.onPrimaryStop),
					d.Separator{},
					u.item(ButtonDiagnose, u.onDiagnose),
					d.Separator{},
					u.item(ButtonEdit, u.onEdit),
					u.item(ButtonClone, u.onClone),
					u.item(ButtonDelete, u.onDelete),
					u.item(ButtonSetDefault, u.onSetDefault),
					d.Separator{},
					u.item(ButtonCreate, u.onCreate),
					u.item(ButtonStopAll, u.onStopAll),
					d.Separator{},
					d.Action{Text: ButtonRefresh, OnTriggered: func() { u.askForRefresh() }},
					d.Action{Text: ButtonQuit, OnTriggered: func() { u.mw.Close() }},
				},
			},
			d.Menu{
				Text: MenuOpen,
				Items: []d.MenuItem{
					u.item(ButtonOpenData, u.onOpenData),
					u.item(ButtonOpenLogs, u.onOpenLogs),
					u.item(ButtonOpenGameLog, u.onOpenGameLog),
					d.Separator{},
					d.Action{Text: ButtonOpenConsole, OnTriggered: u.onOpenConsole},
				},
			},
			d.Menu{
				Text: MenuHelp,
				Items: []d.MenuItem{
					d.Action{Text: ButtonDocs, OnTriggered: u.onDocs},
					d.Separator{},
					d.Action{Text: ButtonAbout, OnTriggered: u.onAbout},
				},
			},
		},
		StatusBarItems: []d.StatusBarItem{
			{AssignTo: &u.statusBar, Text: CloseHint, Width: 900},
		},
		Children: []d.Widget{
			// MinSize.Width IS WHAT MAKES IT WRAP. Without it a label is measured
			// as one line, and this one carries every unreadable file's whole
			// refusal — so the window's minimum width would grow to fit the lot
			// on one line and the text would run off the edge of the screen.
			d.TextLabel{AssignTo: &u.banner, Text: "", TextColor: bannerColour,
				MinSize: d.Size{Width: MinWindowWidth - 60}},
			d.HSplitter{
				StretchFactor: 3,
				Children: []d.Widget{
					u.worldList(columns),
					u.panel(),
				},
			},
			u.details(),
			d.Composite{
				Layout: d.HBox{MarginsZero: true},
				Children: []d.Widget{
					u.button(ButtonCreate, CreateTip, u.onCreate),
					u.button(ButtonStopAll, StopAllTip, u.onStopAll),
					d.HSpacer{},
				},
			},
		},
	}
	if err := window.Create(); err != nil {
		return err
	}
	for _, holder := range u.factHolders {
		u.factValue[holder.label] = *holder.into
	}
	u.banner.SetVisible(false)
	u.spinner.SetVisible(false)
	u.resultLine.SetVisible(false)
	u.hint.SetVisible(false)
	u.detailsPane.SetVisible(false)
	u.detailsToggle.SetText(ButtonShowDetails)
	if icon := windowIcon(); icon != nil {
		u.mw.SetIcon(icon)
	}
	u.restorePlacement()
	u.mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) { u.savePlacement() })
	u.applyActions()
	fmt.Fprintf(u.log, "Bibites Multiverse launcher %s, installed in %s\n",
		launcher.Release, u.session.InstallRoot())
	fmt.Fprintf(u.log, "%s. The commands are in %s.\n", CloseHint, launcher.ConsoleExeName)
	return nil
}

// worldList is the left half: every world, one line each, in the colour of what
// it is doing.
func (u *ui) worldList(columns []d.TableViewColumn) d.Widget {
	return d.TableView{
		AssignTo:              &u.table,
		Model:                 u.model,
		Columns:               columns,
		LastColumnStretched:   true,
		MultiSelection:        false,
		StretchFactor:         2,
		MinSize:               d.Size{Width: 240},
		ToolTipText:           WorldsTip,
		OnCurrentIndexChanged: u.onSelectionChanged,
		// Enter and a double-click both do the world's OWN primary action, which
		// is start when it is stopped and stop when it is running. A stop is not
		// guarded by a confirmation: it asks the world to save and quit, so it
		// costs nothing, and a dialog in front of a lossless action teaches
		// people to click through dialogs.
		OnItemActivated: u.onPrimary,
		StyleCell:       u.styleCell,
		ContextMenuItems: []d.MenuItem{
			u.item(ButtonStart, u.onPrimaryStart),
			u.item(ButtonStop, u.onPrimaryStop),
			d.Separator{},
			u.item(ButtonDiagnose, u.onDiagnose),
			u.item(ButtonEdit, u.onEdit),
			u.item(ButtonSetDefault, u.onSetDefault),
			d.Separator{},
			u.item(ButtonClone, u.onClone),
			u.item(ButtonDelete, u.onDelete),
			d.Separator{},
			u.item(ButtonOpenData, u.onOpenData),
			u.item(ButtonOpenLogs, u.onOpenLogs),
			u.item(ButtonOpenGameLog, u.onOpenGameLog),
			u.item(ButtonCopyPeerID, u.onCopyPeerID),
		},
	}
}

// panel is the right half: ONE world, said in plain words, with the one button
// that acts on it.
func (u *ui) panel() d.Widget {
	return d.Composite{
		Layout:        d.VBox{},
		StretchFactor: 3,
		MinSize:       d.Size{Width: 420},
		Children: []d.Widget{
			d.TextLabel{AssignTo: &u.worldName, Text: "",
				Font: d.Font{Family: uiFontFamily, PointSize: 13, Bold: true}},
			d.TextLabel{AssignTo: &u.headline, Text: "",
				Font:    d.Font{Family: uiFontFamily, PointSize: 10},
				MinSize: d.Size{Width: 380}},
			d.TextLabel{AssignTo: &u.hint, Text: "", MinSize: d.Size{Width: 380}},
			d.ProgressBar{AssignTo: &u.spinner, MarqueeMode: true, MaxSize: d.Size{Height: 12}},
			d.TextLabel{AssignTo: &u.resultLine, Text: "", MinSize: d.Size{Width: 380}},
			d.Composite{
				Layout: d.HBox{MarginsZero: true},
				Children: []d.Widget{
					d.PushButton{AssignTo: &u.primary, Text: ButtonStart, ToolTipText: StartTip,
						MinSize:   d.Size{Width: primaryButtonWidth, Height: primaryButtonHeight},
						OnClicked: u.onPrimary},
					d.HSpacer{},
				},
			},
			d.CheckBox{AssignTo: &u.headless, Text: CheckHeadless, ToolTipText: HeadlessTip,
				OnCheckedChanged: u.onHeadlessChanged},
			u.factGrid(),
			d.Composite{
				Layout: d.HBox{MarginsZero: true},
				Children: []d.Widget{
					u.button(ButtonEdit, EditTip, u.onEdit),
					u.button(ButtonDiagnose, DiagnoseTip, u.onDiagnose),
					d.HSpacer{},
				},
			},
			d.VSpacer{},
			d.Composite{
				Layout: d.HBox{MarginsZero: true},
				Children: []d.Widget{
					d.PushButton{AssignTo: &u.detailsToggle, Text: ButtonShowDetails,
						ToolTipText: DetailsTip, OnClicked: u.onToggleDetails},
					d.HSpacer{},
				},
			},
		},
	}
}

// factGrid is the old table's remaining columns, moved to where they are read:
// beside ONE world, once it has been chosen. It is built once, with a label per
// row in a fixed order, and only the values change — so a value can never end up
// under the wrong label.
func (u *ui) factGrid() d.Widget {
	children := make([]d.Widget, 0, len(FactLabels())*3)
	for _, label := range FactLabels() {
		holder := new(*walk.TextLabel)
		u.factHolders = append(u.factHolders, factHolder{label: label, into: holder})
		children = append(children,
			d.Label{Text: label + ":"},
			d.TextLabel{AssignTo: holder, Text: "", MinSize: d.Size{Width: 200}},
		)
		switch label {
		case FactData:
			children = append(children, u.button(ButtonOpenData, OpenDataTip, u.onOpenData))
		case FactIdentity:
			children = append(children, u.button(ButtonCopyPeerID, CopyPeerIDTip, u.onCopyPeerID))
		default:
			// The third column has to exist on every row or the two that hold a
			// button would be laid out against a different grid.
			children = append(children, d.Label{Text: ""})
		}
	}
	return d.Composite{Layout: d.Grid{Columns: 3, MarginsZero: true}, Children: children}
}

// details is the whole truth, collapsed. It holds the core's own output, which
// is the only place in this window the program's own vocabulary appears.
func (u *ui) details() d.Widget {
	return d.Composite{
		AssignTo:      &u.detailsPane,
		Layout:        d.VBox{MarginsZero: true},
		StretchFactor: 2,
		Children: []d.Widget{
			d.Composite{
				Layout: d.HBox{MarginsZero: true},
				Children: []d.Widget{
					d.Label{Text: "Everything the launcher did this session, newest at the bottom:"},
					d.HSpacer{},
					u.button(ButtonCopyLog, CopyLogTip, u.onCopyLog),
				},
			},
			d.TextEdit{
				AssignTo: &u.logView,
				ReadOnly: true,
				VScroll:  true,
				HScroll:  true,
				// AN EDIT CONTROL HAS ITS OWN CEILING, about 32,767 characters,
				// and past it every append is silently dropped — the pane would
				// freeze on old text with nothing to say it had, and Copy the
				// details would copy that. It is raised above the trimming limit
				// below, which is what actually bounds this.
				MaxLength: logPaneLimit * 2,
				Font:      d.Font{Family: "Consolas", PointSize: 9},
			},
		},
	}
}

// uiFontFamily is the face the rest of Windows uses. walk takes the system font
// when none is named, which on a modern Windows is not the one the shell draws
// with, so the two labels that are meant to be read first name it.
const uiFontFamily = "Segoe UI"

// The colours. They are the ONLY place a Colour becomes a Windows colour, and
// they are chosen for a white list background: dark enough to read, different
// enough from each other to tell apart without reading.
var (
	colourFor = map[Colour]walk.Color{
		ColourGrey:  walk.RGB(96, 96, 96),
		ColourGreen: walk.RGB(0, 112, 48),
		ColourAmber: walk.RGB(160, 96, 0),
		ColourRed:   walk.RGB(176, 0, 0),
	}
	bannerColour = walk.RGB(160, 0, 0)
)

// factHolder is one row of the facts grid, waiting for declarative to create it.
type factHolder struct {
	label string
	into  **walk.TextLabel
}

// button registers one push button under its caption, which is how the harness
// finds it and how applyActions enables it. Several controls may share a caption
// — a button in the panel and the menu item that does the same thing — and they
// are enabled together.
func (u *ui) button(caption, tip string, onClicked func()) d.PushButton {
	holder := new(*walk.PushButton)
	u.buttons[caption] = append(u.buttons[caption], holder)
	return d.PushButton{AssignTo: holder, Text: caption, ToolTipText: tip, OnClicked: onClicked}
}

// item registers a menu item under the same caption as the button that does the
// same thing.
func (u *ui) item(caption string, onTriggered func()) d.Action {
	holder := new(*walk.Action)
	u.menus[caption] = append(u.menus[caption], holder)
	return d.Action{Text: caption, AssignTo: holder, OnTriggered: onTriggered}
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

// styleCell paints the status column in the colour of the state it names.
//
// THE SELECTED ROW IS LEFT ALONE. Windows draws it on the highlight colour, and
// a dark green on that blue is harder to read than the system's own white — so
// the one row a person is looking at keeps the colours the rest of Windows would
// give it, and every other row carries the signal.
func (u *ui) styleCell(style *walk.CellStyle) {
	if style.Col() != ColStatus {
		return
	}
	if style.Row() < 0 || style.Row() >= len(u.model.rows) {
		return
	}
	if u.table != nil && style.Row() == u.table.CurrentIndex() {
		return
	}
	if colour, ok := colourFor[u.model.rows[style.Row()].Status.Colour]; ok {
		style.TextColor = colour
	}
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

// writeLines appends to the pane, keeps it bounded, and READS WHAT IT IS
// APPENDING: the same lines are the progress phrase in the panel, the core's own
// last word for a failure, and the reason the pane opens itself.
//
// AND IT PUTS THE READER BACK. See LinesToRestore: deciding not to follow is not
// enough, because walk's AppendText scrolls the caret into view itself, so the
// view has already moved by the time this code gets to have an opinion. The
// first visible line is therefore read on both sides of the append, and a pane
// nobody is following is scrolled back by the difference.
func (u *ui) writeLines(lines []string) {
	for _, line := range lines {
		if u.busy {
			if phrase, ok := ProgressFor(line); ok {
				u.setPhrase(phrase)
			}
			if said, ok := SaidLine(line); ok {
				u.said = said
			}
		}
		if IsAlarm(line) {
			u.setDetails(true)
		}
	}
	// Whether to follow is decided BEFORE the append, because appending is what
	// moves the bottom away from wherever the reader is.
	follow := u.logFollowsTail()
	before := u.firstVisibleLine()

	// The append and the correction are ONE redraw. Without this the view is
	// drawn at the bottom and then drawn again where the reader was, which is a
	// flicker on every batch — ten times a second while a world starts.
	u.logView.SendMessage(win.WM_SETREDRAW, 0, 0)
	// A Windows edit control's line ending is CRLF; a bare newline draws as one
	// long line.
	u.logView.AppendText(strings.Join(lines, "\r\n") + "\r\n")
	if u.logView.TextLength() > logPaneLimit {
		u.trimLogPane()
		// A trim replaces the whole document, so nobody is where they were. The
		// newest line is the one place worth landing.
		follow = true
	}
	if follow {
		u.scrollLogToEnd()
	} else if back := LinesToRestore(before, u.firstVisibleLine()); back != 0 {
		u.logView.SendMessage(win.EM_LINESCROLL, 0, uintptr(int32(back)))
	}
	u.logView.SendMessage(win.WM_SETREDRAW, 1, 0)
	// RDW_FRAME AND NOT JUST InvalidateRect. WM_SETREDRAW false also stops the
	// control repainting its scroll bar, which is non-client area — an
	// invalidation of the client rectangle alone leaves a bar drawn at the
	// position it held before the append, which is worse than the flicker this
	// suppresses.
	win.RedrawWindow(u.logView.Handle(), nil, 0,
		win.RDW_INVALIDATE|win.RDW_ERASE|win.RDW_FRAME)
}

// trimLogPane drops the oldest half. A long session prints a great deal, and an
// edit control that grows without limit is a window that slows down and then a
// process that runs out of memory. Each world's own log folder keeps the whole
// of it, and the line left behind says so.
func (u *ui) trimLogPane() {
	text := u.logView.Text()
	cut := len(text) / 2
	if index := strings.Index(text[cut:], "\r\n"); index >= 0 {
		cut += index + 2
	} else {
		// No line ending in the second half: cut on a rune boundary instead, so a
		// path with an accent in it does not become a replacement character at the
		// top of the pane.
		for cut < len(text) && !utf8.RuneStart(text[cut]) {
			cut++
		}
	}
	u.logView.SetText("[the older half of this session's log was trimmed here; " +
		"each world's own log folder keeps the whole of it]\r\n" + text[cut:])
}

// firstVisibleLine is the line at the top of the pane, or -1 when the control
// cannot say. EM_GETFIRSTVISIBLELINE is the same reading the machine harness
// takes, which is what makes the assertion in the manual test the same fact this
// code acts on.
func (u *ui) firstVisibleLine() int {
	return int(int32(u.logView.SendMessage(win.EM_GETFIRSTVISIBLELINE, 0, 0)))
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

// applySnapshot redraws everything, keeping the selection on the world it was on.
func (u *ui) applySnapshot(snap launcher.Snapshot) {
	u.snapshot = snap
	u.model.rows = RowsFrom(snap, u.busyView())
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

	if banner := BannerFor(snap); banner != "" {
		u.banner.SetText(banner)
		u.banner.SetVisible(true)
	} else {
		u.banner.SetVisible(false)
	}
	u.mw.SetTitle(WindowTitleFor(snap))
	u.statusBar.SetText(fmt.Sprintf("%s   -   %d world(s) in %s",
		CloseHint, len(snap.Worlds), snap.InstallRoot))
	u.applyActions()
}

// busyView is the action running right now, as the list and the panel see it.
func (u *ui) busyView() Busy {
	if !u.busy {
		return Busy{}
	}
	busy := u.job.Busy()
	if u.phrase != "" {
		busy.Phrase = u.phrase
	}
	return busy
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

// applyActions is the whole of what a person can press and what the panel says,
// applied from one decision taken in view.go.
func (u *ui) applyActions() {
	selected := u.selectedWorld()
	actions := ActionsFor(selected, u.snapshot, u.busy)
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
		ButtonOpenData:    actions.OpenData,
		ButtonOpenLogs:    actions.OpenLogs,
		ButtonOpenGameLog: actions.OpenGameLog,
		ButtonCopyPeerID:  actions.CopyPeerID,
		ButtonCopyLog:     true,
	}
	for caption, holders := range u.buttons {
		for _, holder := range holders {
			if button := *holder; button != nil {
				if want, known := enabled[caption]; known {
					button.SetEnabled(want)
				}
			}
		}
	}
	for caption, holders := range u.menus {
		for _, holder := range holders {
			if action := *holder; action != nil {
				action.SetEnabled(enabled[caption])
			}
		}
	}
	u.applyPanel(PanelFor(selected, u.snapshot, u.busyView()), actions)
}

// applyPanel moves one decided Panel into the widgets. NOTHING IS DECIDED HERE.
func (u *ui) applyPanel(panel Panel, actions Actions) {
	u.worldName.SetText(panel.World)
	u.headline.SetText(panel.Headline)
	u.headline.SetTextColor(colourFor[panel.Colour])
	setLabel(u.hint, panel.Hint, walk.RGB(64, 64, 64))
	u.spinner.SetVisible(panel.Working)
	if panel.Working {
		// PBM_SETMARQUEE is re-asserted on every show: a bar that was hidden
		// mid-animation does not always take it up again by itself.
		u.spinner.SetMarqueeMode(true)
	}
	// The result of the LAST action stays until the next one starts, so somebody
	// who looked away for the ninety seconds a start takes still learns how it
	// went.
	if panel.Working {
		u.resultLine.SetVisible(false)
	} else if u.result.Text != "" {
		colour := colourFor[ColourGreen]
		if !u.result.Good {
			colour = colourFor[ColourRed]
		}
		setLabel(u.resultLine, u.result.Text, colour)
	}
	u.primary.SetText(panel.Primary.Caption)
	u.primary.SetToolTipText(panel.Primary.Tip)
	u.primary.SetEnabled(panel.Primary.Enabled)
	for _, fact := range panel.Facts {
		if label := u.factValue[fact.Label]; label != nil {
			label.SetText(fact.Value)
			// A data folder and an identity are both longer than the column they
			// sit in, and a hover is cheaper than widening the window.
			label.SetToolTipText(fact.Value)
		}
	}
	// A CHECKBOX THAT SNAPS BACK WHILE ITS OWN EDIT IS RUNNING is worse than one
	// that lags. The world has not been written yet, so the snapshot still holds
	// the OLD value, and putting it back into the box would undo the click in
	// front of the person who just made it. It is left where they put it until
	// the edit lands and the next reading confirms it.
	if !(u.busy && u.job.Kind == JobHeadless) {
		u.settingHeadless = true
		u.headless.SetChecked(panel.Headless)
		u.settingHeadless = false
	}
	u.headless.SetEnabled(actions.Headless)
}

// setLabel writes a label and hides it when there is nothing to say, so an empty
// line does not sit in the layout pushing everything below it down.
func setLabel(label *walk.TextLabel, text string, colour walk.Color) {
	label.SetText(text)
	label.SetTextColor(colour)
	label.SetVisible(text != "")
}

// setDetails opens or closes the details pane, and remembers which.
func (u *ui) setDetails(open bool) {
	if u.detailsOpen == open {
		return
	}
	u.detailsOpen = open
	u.detailsPane.SetVisible(open)
	if open {
		u.detailsToggle.SetText(ButtonHideDetails)
		u.scrollLogToEnd()
	} else {
		u.detailsToggle.SetText(ButtonShowDetails)
	}
}

func (u *ui) onToggleDetails() { u.setDetails(!u.detailsOpen) }

// start queues one job. The window says what it started, so the details pane
// reads as a session rather than as a stream of unattributed output.
func (u *ui) start(job Job, run func() int) {
	if u.busy {
		return
	}
	u.busy = true
	u.job = job
	u.phrase = job.Progress()
	u.said = ""
	u.result = Result{}
	u.applyActions()
	fmt.Fprintf(u.log, "\n> %s\n", job.Heading())
	select {
	case u.actions <- func() {
		code := run()
		u.mw.Synchronize(func() { u.finish(job, code) })
	}:
	default:
		// The queue is one deep and the busy flag is what keeps it that way; if
		// it is ever full, say so rather than block the message loop.
		u.busy = false
		u.applyActions()
		fmt.Fprintf(u.log, "another action is still running; nothing was started\n")
	}
}

// setPhrase moves the panel on to the next stage of an action that is running.
func (u *ui) setPhrase(phrase string) {
	if u.phrase == phrase {
		return
	}
	u.phrase = phrase
	u.applyActions()
}

// finish is the end of a job, on the UI thread: the bar stops, the buttons come
// back, and one line says how it went until the next action starts.
//
// A FAILURE OPENS THE DETAILS PANE. The result line names the job and nothing
// else; the reason is the core's own words, and this is the window's promise
// that they are never behind a button somebody did not know to press.
func (u *ui) finish(job Job, code int) {
	u.busy = false
	u.phrase = ""
	u.result = job.Result(code, u.said)
	if !u.result.Good {
		u.setDetails(true)
	}
	u.applyActions()
	u.askForRefresh()
}

// ---------------------------------------------------------------- the actions

// onPrimary is the one obvious action: start a stopped world, stop a running
// one. It is the big button, Enter on the list, and a double-click on a row.
func (u *ui) onPrimary() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	if world.Sidecar.Running || world.Game.Running {
		u.onPrimaryStop()
		return
	}
	u.onPrimaryStart()
}

func (u *ui) onPrimaryStart() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	// The rule that greys the button has to hold on the keyboard path too.
	if !ActionsFor(world, u.snapshot, u.busy).Start {
		return
	}
	name := world.Name
	u.start(Job{Kind: JobStart, World: name},
		func() int { return u.session.Start(name, launcher.StartOptions{}) })
}

func (u *ui) onPrimaryStop() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	if !ActionsFor(world, u.snapshot, u.busy).Stop {
		return
	}
	name := world.Name
	u.start(Job{Kind: JobStop, World: name},
		func() int { return u.session.Stop(name, false, 0) })
}

func (u *ui) onStopAll() {
	u.start(Job{Kind: JobStopAll}, func() int { return u.session.StopAll(0) })
}

func (u *ui) onSetDefault() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	name := world.Name
	u.start(Job{Kind: JobSetDefault, World: name}, func() int { return u.session.SetDefault(name) })
}

// onHeadlessChanged writes the world, through the same 'profile set' path an
// edit uses. It is a SETTING and not a mode this window remembers: a checkbox
// that only applied until the window closed would be a promise the next start
// broke.
func (u *ui) onHeadlessChanged() {
	if u.settingHeadless {
		return
	}
	world := u.selectedWorld()
	if world == nil {
		return
	}
	want := u.headless.Checked()
	if want == world.Headless {
		return
	}
	if u.busy {
		// Put it back: the action goroutine is occupied and the click cannot be
		// honoured, so the checkbox must not go on showing a value the world
		// does not have.
		u.settingHeadless = true
		u.headless.SetChecked(world.Headless)
		u.settingHeadless = false
		return
	}
	name := world.Name
	u.start(Job{Kind: JobHeadless, World: name, Headless: want},
		func() int { return u.session.Edit(name, []string{HeadlessFlag(want)}) })
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
		u.result = Result{Text: "Nothing about '" + name + "' was changed.", Good: true}
		fmt.Fprintf(u.log, "nothing about '%s' was changed.\n", name)
		u.applyActions()
		return
	}
	u.start(Job{Kind: JobEdit, World: name, Flags: flags},
		func() int { return u.session.Edit(name, flags) })
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
		u.setDetails(true)
		u.warn(defaultsErr.Error())
	}
	created, ok, err := runCreateDialog(u.mw, spec, defaultsErr)
	if err != nil {
		u.warn(err.Error())
		return
	}
	if !ok {
		return
	}
	u.start(Job{Kind: JobCreate, World: created.Name}, func() int { return u.session.Create(created) })
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
	u.start(Job{Kind: JobClone, World: source, Other: name},
		func() int { return u.session.Clone(source, name) })
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
	u.start(Job{Kind: JobDelete, World: name},
		func() int { return u.session.Delete(name, removeData, typed) })
}

func (u *ui) onDiagnose() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	name := world.Name
	// A health check is a REPORT, and a report nobody can see is not one. This is
	// the one action that opens the details pane whether or not it finds a fault.
	u.setDetails(true)
	u.start(Job{Kind: JobCheck, World: name}, func() int { return u.session.Diagnose(name) })
}

func (u *ui) onOpenData() {
	world := u.selectedWorld()
	if world == nil {
		return
	}
	if err := openFolder(world.DataRoot); err != nil {
		u.warn(err.Error())
	}
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

// openFolder shows a folder in Explorer. It is the window's own because the data
// root is the one path here the core has no opener for — it opens the LOG folder
// and the game's log, both of which it creates.
func openFolder(path string) error {
	if path == "" {
		return fmt.Errorf("this world has no data folder recorded")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%s cannot be opened: %v", path, err)
	}
	return exec.Command("explorer.exe", filepath.Clean(path)).Start()
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
	u.result = Result{Text: "Copied this world's identity on the map.", Good: true}
	fmt.Fprintf(u.log, "copied this world's identity on the map: %s\n", world.PeerID)
	u.applyActions()
}

func (u *ui) onCopyLog() {
	if err := walk.Clipboard().SetText(u.logView.Text()); err != nil {
		u.warn(err.Error())
		return
	}
	u.result = Result{Text: "Copied the details to the clipboard.", Good: true}
	u.applyActions()
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
	walk.MsgBox(u.mw, ButtonAbout+" "+WindowTitle, strings.Join([]string{
		fmt.Sprintf("Bibites Multiverse launcher %s", launcher.Release),
		"",
		"installed in " + u.session.InstallRoot(),
		"the sidecar: " + u.session.SidecarExe(),
		"the commands: " + launcher.ConsoleExeName,
		"this window remembers its size in: " + u.placementPath(),
		"",
		DocsURL,
		"",
		CloseHint + ".",
	}, "\n"), walk.MsgBoxIconInformation|walk.MsgBoxOK)
}

// warn says something the window itself decided, in the details pane and
// nowhere else: a message box for every refusal would be a window a person has
// to dismiss before they can read what it was about. The pane is opened, because
// a refusal written into a collapsed pane is a refusal nobody read.
func (u *ui) warn(message string) {
	u.setDetails(true)
	fmt.Fprintf(u.log, "%s\n", message)
}

// ---------------------------------------------------------------- the placement

// placementPath is where this window's size and position are kept. It is the
// user's own roaming application data, for the two reasons in windowstate.go:
// not in a world's data folder, and not in the profiles folder.
func (u *ui) placementPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}
	return WindowStatePath(dir)
}

// restorePlacement puts the window back where it was, if that is still a place
// on this machine's screen.
//
// THE ORDER MATTERS, and it is the reason this is two calls rather than one
// SetWindowPlacement. walk's own Run() re-applies the window's current bounds as
// it starts (FormBase.Run calls SetBoundsPixels(BoundsPixels())), which on a
// window already put into the maximised state would move a maximised window with
// SetWindowPos — a shape Windows does not promise anything sensible about. So
// the rectangle is set here, before Run, with a plain SetWindowPos that does not
// show the window; and the maximising, if any, is POSTED to the message loop, so
// it happens after walk has finished starting up.
func (u *ui) restorePlacement() {
	path := u.placementPath()
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	state, ok := DecodeWindowState(raw)
	if !ok {
		return
	}
	u.detailsOpen = state.Details
	u.detailsPane.SetVisible(state.Details)
	if state.Details {
		u.detailsToggle.SetText(ButtonHideDetails)
	}
	fitted, usable := state.Fit(
		int(win.GetSystemMetrics(win.SM_XVIRTUALSCREEN)),
		int(win.GetSystemMetrics(win.SM_YVIRTUALSCREEN)),
		int(win.GetSystemMetrics(win.SM_CXVIRTUALSCREEN)),
		int(win.GetSystemMetrics(win.SM_CYVIRTUALSCREEN)))
	if !usable {
		return
	}
	win.SetWindowPos(u.mw.Handle(), 0,
		int32(fitted.X), int32(fitted.Y), int32(fitted.Width), int32(fitted.Height),
		win.SWP_NOZORDER|win.SWP_NOACTIVATE)
	if fitted.Maximized {
		u.mw.Synchronize(func() { win.ShowWindow(u.mw.Handle(), win.SW_MAXIMIZE) })
	}
}

// savePlacement writes it down as the window closes. EVERY FAILURE IS SILENT: a
// window position that could not be written is not worth a message box in front
// of somebody who is closing the program.
func (u *ui) savePlacement() {
	path := u.placementPath()
	if path == "" {
		return
	}
	var placement win.WINDOWPLACEMENT
	placement.Length = uint32(unsafe.Sizeof(placement))
	if !win.GetWindowPlacement(u.mw.Handle(), &placement) {
		return
	}
	rect := placement.RcNormalPosition
	state := WindowState{
		X:         int(rect.Left),
		Y:         int(rect.Top),
		Width:     int(rect.Right - rect.Left),
		Height:    int(rect.Bottom - rect.Top),
		Maximized: placement.ShowCmd == win.SW_SHOWMAXIMIZED,
		Details:   u.detailsOpen,
	}
	raw, err := state.Encode()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	os.WriteFile(path, raw, 0o644)
}
