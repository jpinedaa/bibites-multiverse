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
	// a person's eye lands on. It is the only oversized control in the window,
	// and it also carries the default-button border and a bold face — three
	// signals, because on a themed Windows any one of them alone is subtle.
	primaryButtonWidth  = 200
	primaryButtonHeight = 44
	// detailsMinHeight is about ten lines of the pane plus its header. The pane
	// opened three lines tall on an 840-pixel window before this existed, which
	// is a log nobody can read and a control nobody would think to drag.
	detailsMinHeight = 190
	// splitterHandle is wide enough to find with a mouse without being a bar.
	splitterHandle = 6
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
		results: NewResultLog(),
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
	// THE FIRST READING IS TAKEN HERE, ON THIS THREAD, BEFORE THE LOOP STARTS.
	// Asking the refresher for it instead left the first painted frame showing an
	// empty list and the bare caption "Bibites Multiverse" while two worlds were
	// running, for as long as it took the message loop to reach the refresher's
	// Synchronize — which a machine caught in a screenshot. A snapshot reads
	// local files and loopback, so this costs milliseconds on a healthy machine
	// and at worst one probe timeout on a wedged one, which is a wait with the
	// right thing on screen at the end of it rather than the wrong thing at the
	// start.
	u.applySnapshot(u.session.Snapshot())
	go u.refresher()
	go u.actor()
	go u.logPump()
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
	factValue   map[string]*walk.Label
	factHolders []factHolder

	// The details pane, which is collapsed until it is wanted or until something
	// goes wrong. detailsSplit is the divider above it, and where a person leaves
	// it is remembered; settings is the four-line walk.Settings that lets walk do
	// the remembering into this window's own file rather than an INI of its own.
	detailsToggle *walk.PushButton
	detailsPane   *walk.Composite
	detailsSplit  *walk.Splitter
	logView       *walk.TextEdit
	settings      *splitSettings

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
	// results is the last thing that happened to each world, so the coloured
	// line under a headline is about the world that headline is about.
	results     *ResultLog
	detailsOpen bool
	// settingHeadless is up while the refresher is writing the checkbox, so that
	// the write does not read as a person having clicked it. Without it every
	// refresh of a headless world would queue an edit, forever.
	settingHeadless bool

	// mu guards what crosses a thread boundary and nothing else.
	mu      sync.Mutex
	pending []string
	// said is the last flush-left line the core printed since the running action
	// started, which is what a failure quotes.
	//
	// IT IS CAPTURED ON THE WRITING GOROUTINE, in appendLine, and that is the fix
	// for a bug a machine found: reading it off the pane's own hundred-
	// millisecond batch lost every refusal, because a refusal is printed and
	// returned from in microseconds and the action had already finished by the
	// time the batch was drained. See SaidLine.
	said string

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
	u.factValue = make(map[string]*walk.Label, len(FactLabels()))

	columns := make([]d.TableViewColumn, 0, len(Columns()))
	for _, column := range Columns() {
		columns = append(columns, d.TableViewColumn{Title: column.Title, Width: column.Width})
	}

	window := d.MainWindow{
		AssignTo: &u.mw,
		// Name is half of the key the splitter's remembered position is stored
		// under (WindowBase.path is parent/child), so it must not be empty.
		Name:    "main",
		Title:   WindowTitle,
		MinSize: d.Size{Width: MinWindowWidth, Height: MinWindowHeight},
		Size:    d.Size{Width: defaultWindowWidth, Height: defaultWindowHeight},
		Layout:  d.VBox{},
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
				Background: labelBackground,
				MinSize:    d.Size{Width: MinWindowWidth - 60}},
			d.VSplitter{
				AssignTo:      &u.detailsSplit,
				Name:          SplitNameDetails,
				Persistent:    true,
				HandleWidth:   splitterHandle,
				StretchFactor: 1,
				Children: []d.Widget{
					d.HSplitter{
						// Named, so it gets a key of its own and is remembered
						// too. It is the names that keep the two splitters apart
						// in the settings; an unnamed one would be handed the
						// other's state — see splitSettings.
						Name:        SplitNameWorlds,
						Persistent:  true,
						HandleWidth: splitterHandle,
						// The list needs two columns and the panel needs room for
						// a path, so the panel gets two thirds.
						StretchFactor: 1,
						Children: []d.Widget{
							u.worldList(columns),
							u.panel(),
						},
					},
					u.details(),
				},
			},
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
	// The settings have to exist before anything asks walk to read state.
	u.settings = newSplitSettings()
	walk.App().SetSettings(u.settings)

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
	u.showDetails(false)
	if icon := windowIcon(); icon != nil {
		u.mw.SetIcon(icon)
	}
	// THE THIRD SIGNAL THAT THIS IS THE BUTTON. It is already the biggest and the
	// only bold one; BS_DEFPUSHBUTTON adds the accent border Windows draws round
	// the button a dialog would press on Enter. There is no default button on a
	// main window, so this is drawing rather than behaviour — the Enter key is
	// handled on the world list, where the selection is.
	u.primary.SendMessage(win.BM_SETSTYLE, uintptr(win.BS_DEFPUSHBUTTON), 1)
	u.restorePlacement()
	u.restoreDividers()
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
		StretchFactor:         1,
		MinSize:               d.Size{Width: 230},
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
		Layout: d.VBox{},
		// Two thirds of the width. The list needs two columns; the panel needs
		// room for a Windows path and a button beside it.
		StretchFactor: 2,
		MinSize:       d.Size{Width: 470},
		Children: []d.Widget{
			d.TextLabel{AssignTo: &u.worldName, Text: "",
				Font: d.Font{Family: uiFontFamily, PointSize: 13, Bold: true}},
			d.TextLabel{AssignTo: &u.headline, Text: "",
				Font:       d.Font{Family: uiFontFamily, PointSize: 10},
				Background: labelBackground,
				MinSize:    d.Size{Width: 380}},
			d.TextLabel{AssignTo: &u.hint, Text: "",
				Background: labelBackground, MinSize: d.Size{Width: 380}},
			d.ProgressBar{AssignTo: &u.spinner, MarqueeMode: true, MaxSize: d.Size{Height: 12}},
			d.TextLabel{AssignTo: &u.resultLine, Text: "",
				Background: labelBackground, MinSize: d.Size{Width: 380}},
			d.Composite{
				Layout: d.HBox{MarginsZero: true},
				Children: []d.Widget{
					d.PushButton{AssignTo: &u.primary, Text: ButtonStart, ToolTipText: StartTip,
						MinSize:   d.Size{Width: primaryButtonWidth, Height: primaryButtonHeight},
						Font:      d.Font{Family: uiFontFamily, PointSize: 11, Bold: true},
						OnClicked: u.onPrimary},
					d.HSpacer{},
				},
			},
			// THE CHECKBOX BELONGS TO THE BUTTON ABOVE IT, so it is put in a row
			// of its own with the spacer on the right. Left to a VBox it was laid
			// out at the row's centre — 220 pixels to the right of the button it
			// qualifies, lined up with nothing.
			d.Composite{
				Layout: d.HBox{MarginsZero: true},
				Children: []d.Widget{
					d.CheckBox{AssignTo: &u.headless, Text: CheckHeadless, ToolTipText: HeadlessTip,
						OnCheckedChanged: u.onHeadlessChanged},
					d.HSpacer{},
				},
			},
			// ONE ROW OF SECONDARY BUTTONS, and the details toggle is in it. It
			// used to be pinned to the bottom of the panel by a spacer, which put
			// fifty-eight pixels of nothing above it and read as a gap where
			// something had failed to load.
			d.Composite{
				Layout: d.HBox{MarginsZero: true},
				Children: []d.Widget{
					u.button(ButtonEdit, EditTip, u.onEdit),
					u.button(ButtonDiagnose, DiagnoseTip, u.onDiagnose),
					d.PushButton{AssignTo: &u.detailsToggle, Text: ButtonShowDetails,
						ToolTipText: DetailsTip, OnClicked: u.onToggleDetails},
					d.HSpacer{},
				},
			},
			u.factGrid(),
			d.VSpacer{},
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
		holder := new(*walk.Label)
		u.factHolders = append(u.factHolders, factHolder{label: label, into: holder})
		children = append(children,
			d.Label{Text: label + ":", MinSize: d.Size{Width: factLabelWidth}},
			// A LABEL AND NOT A TEXT LABEL, for one reason: only walk's Label
			// carries an ellipsis mode, and SS_PATHELLIPSIS is Windows' own answer
			// to a folder wider than the space it has. The value used to be cut
			// dead at the pixel the button beside it began, mid-word, with nothing
			// to say it had been cut. The whole value is in the tooltip either way.
			d.Label{AssignTo: holder, Text: "", MinSize: d.Size{Width: 180},
				EllipsisMode: ellipsisFor(label)},
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

// factLabelWidth keeps the values in one column, so the grid reads down rather
// than zig-zagging with the length of each label.
const factLabelWidth = 90

// ellipsisFor picks Windows' own shortening. A folder is shortened in the
// middle, keeping the drive and the last folder — which is the half that says
// WHICH world this is — and everything else is shortened at the end.
func ellipsisFor(label string) d.EllipsisMode {
	if label == FactData {
		return d.EllipsisPath
	}
	return d.EllipsisEnd
}

// details is the whole truth, collapsed. It holds the core's own output, which
// is the only place in this window the program's own vocabulary appears.
func (u *ui) details() d.Widget {
	return d.Composite{
		AssignTo: &u.detailsPane,
		Layout:   d.VBox{MarginsZero: true},
		// A pane worth opening. The stretch factor decides its share of a window
		// with room to spare; the minimum decides what it gets on a small one,
		// and it is what stopped this opening three lines tall.
		StretchFactor: 2,
		MinSize:       d.Size{Height: detailsMinHeight},
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
	hintColour   = walk.RGB(72, 72, 72)
)

// labelBackground is what makes a coloured label actually come out in its
// colour, and it is not decoration: it is the whole of the fix.
//
// WHAT A MACHINE MEASURED. Every panel label — the headline, the hint, the
// result line and the banner — was drawn in the system's near-black whatever
// colour SetTextColor had been given, while the table's cells (which walk draws
// itself, through StyleCell) carried theirs perfectly.
//
// WHY. A walk static is two windows: an outer one whose WndProc answers
// WM_CTLCOLORSTATIC, and a real "static" child that draws the glyphs. The child
// asks its parent for the colour, and the parent's answer is
// static.WndProc (static.go:262):
//
//	case win.WM_CTLCOLORSTATIC:
//	    if hBrush := s.handleWMCTLCOLOR(wp, uintptr(s.hWnd)); hBrush != 0 {
//	        return hBrush
//	    }
//	    ...
//	return s.WidgetBase.WndProc(hwnd, msg, wp, lp)
//
// handleWMCTLCOLOR (window.go:2263) DOES call SetTextColor with our colour — and
// then looks for a background brush to answer with. It finds none: a static's
// own background is walk's null brush, backgroundEffective walks up the parents
// looking for one that is not, and walk sets the form's client composite's
// background to nil outright (form.go:111). With no brush it returns 0, so
// static.WndProc falls through to WidgetBase.WndProc and on to DefWindowProc —
// and DefWindowProc's WM_CTLCOLORSTATIC selects the DEFAULT system colours into
// the DC before returning the default brush. Our SetTextColor is overwritten
// microseconds after it is made, on every single paint.
//
// So the label needs a background of its own, and it must be the one every walk
// window class already paints with (MustRegisterWindowClass sets
// wc.HbrBackground = COLOR_BTNFACE+1), so that nothing about the window looks
// any different. With a brush to answer with, handleWMCTLCOLOR returns it,
// static.WndProc returns it too, and DefWindowProc is never reached.
var labelBackground = d.SystemColorBrush{Color: walk.SysColorBtnFace}

// Every colour has a Windows colour. A missing one would draw a state in black,
// which is a state with no signal at all.
func init() {
	for _, colour := range Colours() {
		if _, ok := colourFor[colour]; !ok {
			panic("launchergui: no text colour for colour")
		}
	}
}

// factHolder is one row of the facts grid, waiting for declarative to create it.
type factHolder struct {
	label string
	into  **walk.Label
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
// THE SELECTED ROW KEEPS ITS COLOUR TOO, and the first attempt at this was
// wrong: it skipped the selected row so that Windows' own highlight stayed
// legible, which meant the ONE row a person's eye is on was the one row with no
// signal on it — select the world you are worried about and its red goes away.
// So the state's colour is put on the text of BOTH cells of the selected row,
// and the row keeps its signal.
//
// THE BACKGROUND OF THE SELECTED ROW IS WINDOWS', AND WALK WILL NOT LET IT BE
// ANYTHING ELSE. A round of this window promised a pale wash of the state's own
// hue behind the selected row, and a machine measured the system's selection
// colour there instead — 204,232,255 across the whole row, with our green text
// on top of it and not one pixel of the wash. It is not a bug in this file:
// walk's TableView throws the value away, in tableview.go's NM_CUSTOMDRAW
// handler, at CDDS_ITEMPREPAINT:
//
//	if selected {
//	    tv.style.BackgroundColor = tv.itemBGColor      // the theme's colour
//	    tv.style.TextColor = tv.itemTextColor
//	} else {
//	    tv.itemBGColor = tv.style.BackgroundColor      // ours is kept
//	    tv.itemTextColor = tv.style.TextColor
//	}
//
// — and the fill three lines below it uses that overwritten value. An
// unselected row's background is ours; a selected row's never is. The subitem
// pass does hand ClrTextBk our colour, but a themed ListView paints the
// selection itself and ignores it, which is why setting it changed nothing on
// screen. walk's only remaining door is CellStyle.Canvas(), which means drawing
// every cell's text by hand — the font, the alignment, the ellipsis and the
// focus rectangle — to change a background colour, and that is a great deal of
// drawing code to own for a tint.
//
// So the promise is the text and not the wash: Windows' selection colour is
// pale (it is the Explorer theme's, not COLOR_HIGHLIGHT), the state's colour is
// dark, and the two are readable together. The manual test says so too.
func (u *ui) styleCell(style *walk.CellStyle) {
	row := style.Row()
	if row < 0 || row >= len(u.model.rows) {
		return
	}
	colour := u.model.rows[row].Status.Colour
	selected := u.table != nil && row == u.table.CurrentIndex()
	if !selected && style.Col() != ColStatus {
		return
	}
	style.TextColor = colourFor[colour]
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
//
// IT IS ALSO WHERE THE CORE'S LAST WORD IS TAKEN, and that placement is the
// whole of a fix. Reading it later, off the pane's own batch, lost every
// refusal: a refusal is printed and returned from in microseconds, so the action
// had already finished and the panel said "'world2' was not created. The details
// below say why." while the core's own sentence sat one line further down. Here
// the line is seen by the goroutine that wrote it, before the action returns.
func (u *ui) appendLine(line string) {
	u.mu.Lock()
	u.pending = append(u.pending, line)
	if said, ok := SaidLine(line); ok {
		u.said = said
	}
	u.mu.Unlock()
	select {
	case u.wake <- struct{}{}:
	default:
	}
}

// ---------------------------------------------------------------- the UI thread

// writeLines appends to the pane, keeps it bounded, and READS WHAT IT IS
// APPENDING: the same lines are the progress phrase in the panel and the reason
// the pane opens itself. (The core's last word for a failure is taken in
// appendLine instead, and the comment there says why it cannot be taken here.)
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
	// A world that has left the list takes its result with it, so a world made
	// again under a name somebody has used before does not open on the last thing
	// that happened to a different world. It is read BEFORE the new reading
	// replaces the old one.
	for _, gone := range goneFrom(u.worldNames(), snap.Worlds) {
		u.results.Forget(gone)
	}
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

	setLabel(u.banner, BannerFor(snap), bannerColour)
	u.mw.SetTitle(WindowTitleFor(snap))
	u.statusBar.SetText(StatusBarText(snap))
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
	u.applyPanel(PanelFor(selected, u.snapshot, u.busyView(), u.results), actions)
}

// applyPanel moves one decided Panel into the widgets. NOTHING IS DECIDED HERE.
func (u *ui) applyPanel(panel Panel, actions Actions) {
	u.worldName.SetText(panel.World)
	// Through setLabel like the others, so the one rule about repainting a
	// coloured static applies to every coloured static in the window. The
	// headline is never empty for a selected world, and an empty one is a panel
	// with no world in it, which has nothing to say anyway.
	setLabel(u.headline, panel.Headline, colourFor[panel.Colour])
	setLabel(u.hint, panel.Hint, hintColour)
	u.spinner.SetVisible(panel.Working)
	if panel.Working {
		// PBM_SETMARQUEE is re-asserted on every show: a bar that was hidden
		// mid-animation does not always take it up again by itself.
		u.spinner.SetMarqueeMode(true)
	}
	// THE RESULT IS THIS WORLD'S, and it stays until the next action about this
	// world — so somebody who looked away for the ninety seconds a start takes
	// still learns how it went, and somebody who selects another world is not
	// shown that other world's headline over this one's outcome.
	colour := colourFor[ColourGreen]
	if !panel.Result.Good {
		colour = colourFor[ColourRed]
	}
	setLabel(u.resultLine, panel.Result.Text, colour)
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

// setLabel writes a label in a colour, and hides it when there is nothing to
// say, so an empty line does not sit in the layout pushing everything below it
// down.
//
// WHY IT REDRAWS BY HAND, which is HALF of what a coloured walk label needs.
// A walk static is TWO windows: an outer one whose WndProc answers
// WM_CTLCOLORSTATIC, and a real child "static" control that draws the text.
// Changing the text calls setWindowText on the CHILD, which repaints it and so
// asks for the colour again. Changing only the COLOUR calls Invalidate on the
// OUTER window — and invalidating a parent does not reach a child window, so the
// child keeps the pixels it already had and nothing ever asks for the new
// colour. RDW_ALLCHILDREN reaches the child; RDW_UPDATENOW makes it happen
// before the next thing is drawn rather than whenever the queue gets round to
// it.
//
// THE OTHER HALF IS labelBackground, and without it this one buys nothing: a
// repaint that does reach the child still ends at DefWindowProc, which selects
// the system's near-black into the DC. A round that had only this half measured
// zero coloured pixels in every label in the window. The two go together — one
// makes the child repaint, the other makes the repaint keep our colour — and
// every coloured label in this window goes through both.
//
// The colour is set BEFORE the text as well as after it, so that a repaint the
// text change schedules already has the right one.
func setLabel(label *walk.TextLabel, text string, colour walk.Color) {
	label.SetTextColor(colour)
	label.SetText(text)
	label.SetVisible(text != "")
	if text == "" {
		return
	}
	label.SetTextColor(colour)
	redrawWithChildren(label)
}

// redrawWithChildren repaints a widget AND the child windows inside it, which is
// what walk's own Invalidate does not do.
func redrawWithChildren(widget walk.Widget) {
	win.RedrawWindow(widget.Handle(), nil, 0,
		win.RDW_INVALIDATE|win.RDW_ERASE|win.RDW_ALLCHILDREN|win.RDW_UPDATENOW)
}

// setDetails opens or closes the details pane, and remembers which.
func (u *ui) setDetails(open bool) {
	if u.detailsOpen == open {
		return
	}
	u.detailsOpen = open
	u.showDetails(open)
	if open {
		u.scrollLogToEnd()
	}
}

// showDetails puts the pane AND ITS DIVIDER on screen, or takes both off.
//
// THE DIVIDER HAS TO BE HIDDEN BY HAND, which is a walk limitation a machine
// found: hiding a splitter's child makes walk mark the matching handle's LAYOUT
// ITEM invisible (splitterlayout.go, reset) so that it is excluded from the
// arithmetic — but nothing hides the handle's own window. It therefore stayed on
// screen at whatever bounds it last had: a nine-pixel bar across the bottom of
// the window, with nothing under it, which could still be dragged and which
// resized a pane nobody could see.
func (u *ui) showDetails(open bool) {
	u.detailsPane.SetVisible(open)
	if handle := u.detailsHandle(); handle != nil {
		handle.SetVisible(open)
	}
	if open {
		u.detailsToggle.SetText(ButtonHideDetails)
	} else {
		u.detailsToggle.SetText(ButtonShowDetails)
	}
}

// detailsHandle is the divider walk inserted between the two halves of the
// vertical splitter. walk keeps a splitter's handles at the ODD indices of its
// children — [child, handle, child] — which is the same arithmetic its own
// layout uses (i%2 == 1 is a handle), so this reads the structure rather than
// guessing at it. A splitter that has not been built yet has neither.
func (u *ui) detailsHandle() walk.Widget {
	if u.detailsSplit == nil {
		return nil
	}
	children := u.detailsSplit.Children()
	if children == nil || children.Len() < 2 {
		return nil
	}
	return children.At(1)
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
	u.results.Record(job.ResultAbout(), Result{}, u.worldNames())
	u.mu.Lock()
	u.said = ""
	u.mu.Unlock()
	u.applyActions()
	fmt.Fprintf(u.log, "\n> %s\n", job.Heading())
	select {
	case u.actions <- func() {
		code := run()
		// ANYTHING HALF-WRITTEN GOES TO THE PANE BEFORE THE RESULT IS DECIDED.
		// The core writes a prompt without a newline, and a session answers
		// rather than asks — so the last thing it said can be sitting in the
		// log's own buffer at the moment the exit code arrives.
		u.log.Flush()
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
	u.mu.Lock()
	said := u.said
	u.mu.Unlock()
	result := job.Result(code, said)
	u.results.Record(job.ResultAbout(), result, u.worldNames())
	if !result.Good {
		u.setDetails(true)
	}
	u.applyActions()
	u.askForRefresh()
}

// worldNames is the worlds that exist right now, which is what decides whether
// a result has a row to live beside.
func (u *ui) worldNames() []string {
	names := make([]string, 0, len(u.snapshot.Worlds))
	for _, world := range u.snapshot.Worlds {
		names = append(names, world.Name)
	}
	return names
}

// say records something the WINDOW decided — a copy, an edit that changed
// nothing — beside the world it is about, through the same log every action's
// result goes through, so one rule decides what the coloured line says.
func (u *ui) say(result Result, worlds ...string) {
	u.results.Record(ResultAbout{Worlds: worlds}, result, u.worldNames())
	u.applyActions()
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
		fmt.Fprintf(u.log, "nothing about '%s' was changed.\n", name)
		u.say(Result{Text: "Nothing about '" + name + "' was changed.", Good: true}, name)
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
	fmt.Fprintf(u.log, "copied this world's identity on the map: %s\n", world.PeerID)
	u.say(Result{Text: "Copied this world's identity on the map.", Good: true}, world.Name)
}

func (u *ui) onCopyLog() {
	if err := walk.Clipboard().SetText(u.logView.Text()); err != nil {
		u.warn(err.Error())
		return
	}
	// Not about any one world, so it is shown whatever is selected.
	u.say(Result{Text: "Copied the details to the clipboard.", Good: true})
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

// splitSettings is a walk.Settings of a dozen useful lines.
//
// WHY THIS EXISTS. walk already knows how to remember a splitter's position:
// mark it Persistent, call the window's SaveState and RestoreState, and it reads
// and writes one string per splitter through walk.App().Settings(). What walk
// SHIPS as a Settings is an INI file under %APPDATA%\<organisation>\<product> —
// a second preferences file beside the one this window already keeps, in a
// folder named after two fields nothing else in this program sets.
//
// So the mechanism is walk's and the storage is ours: this holds the strings
// walk writes, and windowstate.go carries the whole map in the same JSON as the
// size and the position.
//
// IT IS KEYED, AND THAT IS NOT INCIDENTAL. A first attempt held one value and
// answered every Get with it, on the grounds that there is one splitter worth
// remembering. There are TWO splitters — the details divider and the world
// list's, nested inside it — and walk's Splitter.RestoreState descends into its
// own children whether or not they are Persistent. A single shared value was
// therefore handed to the inner splitter as well, whose two children happen to
// match the two numbers, so restoring the height of the details pane also
// silently set the width of the world list to it.
//
// IT IS KEYED BY THE SHORT NAME, not by walk's whole path (SplitAlias). The path
// is "main/clientComposite/details" — correct and stable, and a piece of walk's
// furniture in a file a participant can open. The last segment is the name this
// program chose for the widget, and the two names differ, so it is exact.
type splitSettings struct {
	values map[string]string
}

func newSplitSettings() *splitSettings {
	return &splitSettings{values: make(map[string]string)}
}

func (s *splitSettings) Get(key string) (string, bool) {
	value, ok := s.values[SplitAlias(key)]
	return value, ok
}
func (s *splitSettings) Timestamp(string) (time.Time, bool) { return time.Time{}, false }
func (s *splitSettings) Put(key, value string) error {
	s.values[SplitAlias(key)] = value
	return nil
}
func (s *splitSettings) PutExpiring(key, value string) error { return s.Put(key, value) }
func (s *splitSettings) Remove(key string) error             { delete(s.values, SplitAlias(key)); return nil }
func (s *splitSettings) ExpireDuration() time.Duration       { return 0 }
func (s *splitSettings) SetExpireDuration(time.Duration)     {}
func (s *splitSettings) Load() error                         { return nil }
func (s *splitSettings) Save() error                         { return nil }

// seed fills it from the file.
func (s *splitSettings) seed(saved map[string]string) {
	for key, value := range saved {
		s.values[key] = value
	}
}

// any answers whether there is anything worth restoring.
func (s *splitSettings) any() bool { return len(s.values) > 0 }

// usable is what goes back into the file. The rule it applies is UsableSplit's,
// in windowstate.go, where it can be tested.
func (s *splitSettings) usable() map[string]string { return UsableSplit(s.values) }

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
	u.showDetails(state.Details)
	// The divider between the worlds and the details pane. walk reads this back
	// itself, from the settings object above, when the window's state is
	// restored — which happens after the size is set, because the sizes it holds
	// are pixels and a splitter about to be resized would only redistribute them.
	u.settings.seed(state.Split)
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

// restoreDividers puts the two dividers back where they were left.
//
// IT ASKS THE SPLITTER AND NOT THE WINDOW, and that is the whole of the second
// fix. The obvious call is MainWindow.RestoreState, which is what walk's own
// examples use — and it silently did nothing at all. FormBase.RestoreState
// (form.go:622) reads the FORM'S state first:
//
//	state, err := fb.ReadState()
//	...
//	if state == "" {
//	    return nil
//	}
//	...
//	return fb.clientComposite.RestoreState()
//
// The descent into the children — which is where the splitters are — is the LAST
// line of that function, and the early return above it is taken whenever the
// form itself has nothing stored. This window's placement is its own (a
// rectangle sanity-checked against the screen it is opening on, in
// restorePlacement), so nothing is ever stored under the form's key and the
// early return was taken every single time. The positions were written on close,
// were read back into the settings on open, and were never handed to a splitter.
//
// Splitter.RestoreState descends into its own children whether or not they are
// Persistent, so asking the outer one restores both: "details" from this call
// and "worlds" from the recursion inside it.
//
// AND IT IS BEFORE Run, WITH THE SIZE ALREADY SET, because what walk stores for
// a splitter is pixel sizes: restoring them into a window that is about to be
// resized only gives them straight back to be redistributed.
func (u *ui) restoreDividers() {
	if !u.settings.any() || u.detailsSplit == nil {
		return
	}
	if err := u.detailsSplit.RestoreState(); err != nil {
		fmt.Fprintf(u.log, "The dividers could not be put back where they were: %v\n", err)
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
		Split:     u.splitState(),
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

// splitState is where the dividers were left. walk writes them into the settings
// above; usable is what decides which of them are worth keeping.
//
// IT ASKS THE SAME SPLITTER restoreDividers asks, for symmetry and for one
// concrete gain: MainWindow.SaveState writes the form's own WINDOWPLACEMENT into
// the settings too, under walk's key for the form, and that string
// ("0 1 -1 -1 -1 -1 114 114 1734 1164") is not this program's business — the
// placement is kept as named fields in the same file. UsableSplit dropped it on
// the way out, so it never reached anybody; not writing it is simply one less
// thing to drop.
func (u *ui) splitState() map[string]string {
	if u.detailsSplit != nil {
		u.detailsSplit.SaveState()
	}
	return u.settings.usable()
}

// goneFrom is the names in was that are not in now.
func goneFrom(was []string, now []launcher.WorldView) []string {
	var gone []string
	for _, name := range was {
		found := false
		for i := range now {
			if strings.EqualFold(now[i].Name, name) {
				found = true
				break
			}
		}
		if !found {
			gone = append(gone, name)
		}
	}
	return gone
}
