//go:build windows

package launchergui

// THE DIALOGS. Each one collects values and NOTHING ELSE: no validation, no
// decision, no refusal. The core is what judges a port, a save name, an edge
// list or a data folder, and its answer is what the participant reads in the
// details pane — so a dialog that checked a value here would either agree with
// the core (and be dead code) or disagree with it (and be a lie in a window).
//
// The one thing a dialog does decide is whether the action happens at all, and
// for the destructive one that is the whole of its job: the world's name, typed,
// which is then handed to the core's own comparison (launcher.Session.Delete).
//
// WHAT CHANGED WHEN THE WINDOW WAS REDESIGNED. Each of these used to open on a
// grid of every field it had, which asked a person to have an opinion about a
// port before they had a world. Now the create dialog asks for A NAME, and hides
// the rest behind one button; the edit dialog says which of its values are the
// packaged defaults, so a person can tell a decision from a leftover; and both
// destructive warnings sit immediately above the button that acts on them,
// rather than at the top where they are read once and then scrolled past.

import (
	"strconv"
	"strings"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"

	"multiverse/internal/launcher"
)

// dialogWidth is wide enough for a Windows path in a single line edit, which is
// what most of these fields hold.
const dialogWidth = 640

// noteColour is the grey the "(the default)" notes and the explanations are
// drawn in: present, and not competing with the field beside it.
var noteColour = walk.RGB(96, 96, 96)

// runEditDialog is every mutable setting of one world, at once, grouped.
func runEditDialog(owner walk.Form, p launcher.Profile) (WorldForm, bool, error) {
	form := FormFor(p)
	var (
		dlg      *walk.Dialog
		accept   *walk.PushButton
		cancel   *walk.PushButton
		save     *walk.LineEdit
		port     *walk.LineEdit
		headless *walk.CheckBox
		edges    *walk.LineEdit
		species  *walk.LineEdit
		minutes  *walk.LineEdit
		keep     *walk.LineEdit
		onQuit   *walk.CheckBox
		gameDir  *walk.LineEdit
	)
	var out WorldForm
	accepted := false

	layout := d.Dialog{
		AssignTo:      &dlg,
		Title:         "Settings for the world '" + p.Name + "'",
		DefaultButton: &accept,
		CancelButton:  &cancel,
		MinSize:       d.Size{Width: dialogWidth, Height: 460},
		Layout:        d.VBox{},
		Children: []d.Widget{
			d.GroupBox{
				Title:  "This world",
				Layout: d.Grid{Columns: 3},
				Children: []d.Widget{
					d.Label{Text: "Save name"},
					d.LineEdit{AssignTo: &save, Text: form.Save,
						ToolTipText: "The name of the save file the game loads for this world."},
					note(""),

					d.Label{Text: "Port"},
					d.LineEdit{AssignTo: &port, Text: form.Port,
						ToolTipText: "The port on this computer this world talks to the map through. " +
							"Every world needs its own."},
					note(DefaultNote(form.Port, strconv.Itoa(launcher.DefaultSidecarPort))),

					d.Label{Text: "Game window"},
					d.CheckBox{AssignTo: &headless, Text: CheckHeadless, Checked: form.Headless,
						ToolTipText: HeadlessTip},
					note(""),

					d.Label{Text: "The folder the game is installed in"},
					d.LineEdit{AssignTo: &gameDir, Text: form.GameDir,
						ToolTipText: "Where The Bibites is installed. The launcher starts that copy."},
					note(""),
				},
			},
			d.GroupBox{
				Title:  "Who may leave, and where",
				Layout: d.Grid{Columns: 3},
				Children: []d.Widget{
					d.Label{Text: "Edges organisms may cross"},
					d.LineEdit{AssignTo: &edges, Text: form.ExportEdges,
						ToolTipText: "Which sides of your world are doors: E, N, W and S, " +
							"separated by commas. Every door works both ways."},
					note(DefaultNote(form.ExportEdges, launcher.DefaultExportEdges)),

					d.Label{Text: "Species that never leave"},
					d.LineEdit{AssignTo: &species, Text: form.ExcludeSpecies,
						CueBanner: "empty means every species may leave",
						ToolTipText: "Species named here stay in your world. Emptying this field " +
							"turns the whole rule off, which the launcher reports as the real change it is."},
					note(DefaultNote(form.ExcludeSpecies, launcher.DefaultExcludeSpecies)),
				},
			},
			d.GroupBox{
				Title:  "Saving",
				Layout: d.Grid{Columns: 3},
				Children: []d.Widget{
					d.Label{Text: "Save every N minutes"},
					d.LineEdit{AssignTo: &minutes, Text: form.SaveMinutes,
						ToolTipText: "How often this world writes itself out. 0 turns the timer off. " +
							"A world with no game window loses everything since its last save if it " +
							"has to be forced, so keep this short for one of those."},
					note(DefaultNote(form.SaveMinutes, exactFloat(launcher.DefaultSaveMinutes))),

					d.Label{Text: "Saves kept"},
					d.LineEdit{AssignTo: &keep, Text: form.SaveKeep,
						ToolTipText: "How many of those saves are kept before the oldest is removed."},
					note(DefaultNote(form.SaveKeep, strconv.Itoa(launcher.DefaultSaveKeep))),

					d.Label{Text: "When the game closes"},
					d.CheckBox{AssignTo: &onQuit, Text: "Write the world out before it quits",
						Checked:     form.SaveOnQuit,
						ToolTipText: "Saves once more on the way out, so a normal stop loses nothing."},
					note(""),
				},
			},
			d.TextLabel{
				MinSize:   d.Size{Width: dialogWidth - 40},
				TextColor: noteColour,
				Text: "Only what you change is written. Everything else about this world is left " +
					"exactly as it is.",
			},
			d.Composite{
				Layout: d.HBox{},
				Children: []d.Widget{
					d.HSpacer{},
					d.PushButton{
						AssignTo: &accept,
						Text:     ButtonDialogSave,
						OnClicked: func() {
							out = WorldForm{
								Save:           save.Text(),
								Port:           port.Text(),
								Headless:       headless.Checked(),
								ExportEdges:    edges.Text(),
								ExcludeSpecies: species.Text(),
								SaveMinutes:    minutes.Text(),
								SaveKeep:       keep.Text(),
								SaveOnQuit:     onQuit.Checked(),
								GameDir:        gameDir.Text(),
							}
							accepted = true
							dlg.Accept()
						},
					},
					d.PushButton{AssignTo: &cancel, Text: ButtonDialogCancel,
						OnClicked: func() { dlg.Cancel() }},
				},
			},
		},
	}
	if _, err := layout.Run(owner); err != nil {
		return form, false, err
	}
	return out, accepted, nil
}

// note is the grey line beside a field that says what the packaged default is.
// An empty one still occupies its grid cell, so the column stays a column.
func note(text string) d.Widget {
	return d.TextLabel{Text: text, TextColor: noteColour, MinSize: d.Size{Width: 130}}
}

// runCreateDialog asks for A NAME, and nothing else unless it is asked for.
//
// IT IS STEP ONE OF TWO, and step two is the window itself: this dialog closes
// the moment it is accepted, and the panel behind it shows the enrollment
// happening — the spinning bar, the phrase, and then a green or a red line. A
// modal progress box would have hidden the one thing worth watching, which is
// the world list filling in behind it.
//
// WHAT IT SAYS BEFORE IT IS AGREED TO: that a world is permanent on a shared
// map, that the map applies a per-address limit a run of quick creates will
// meet, and that deleting a world here is not leaving the map. Those are the
// core's own sentences, immediately above the button that acts on them.
func runCreateDialog(owner walk.Form, spec launcher.CreateSpec, defaultsErr error) (launcher.CreateSpec, bool, error) {
	var (
		dlg      *walk.Dialog
		accept   *walk.PushButton
		cancel   *walk.PushButton
		advanced *walk.PushButton
		more     *walk.Composite
		name     *walk.LineEdit
		world    *walk.LineEdit
		port     *walk.LineEdit
		dataRoot *walk.LineEdit
		gameDir  *walk.LineEdit
		headless *walk.CheckBox
	)
	out := spec
	accepted := false

	// AN INSTALLATION WITH NOTHING IN IT still creates a world, but nobody can
	// guess where its game is — so the advanced half opens by itself, with the
	// core's own reason above it, rather than hiding the only field that has to
	// be filled in.
	trouble := ""
	open := false
	if defaultsErr != nil {
		trouble = "This installation has no world to copy from, so the folder the game is " +
			"installed in has to be filled in below: " + defaultsErr.Error()
		open = true
	}
	advancedCaption := ButtonShowAdvanced
	if open {
		advancedCaption = ButtonHideAdvanced
	}

	layout := d.Dialog{
		AssignTo:      &dlg,
		Title:         "Create a world on this computer",
		DefaultButton: &accept,
		CancelButton:  &cancel,
		MinSize:       d.Size{Width: dialogWidth, Height: 400},
		Layout:        d.VBox{},
		Children: []d.Widget{
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40},
				Text: "A world is a simulation on this computer with its own place on the public " +
					"map. It gets its own save file, its own folder and its own identity, and it " +
					"stays on the map until you take it off.",
			},
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40}, TextColor: bannerColour, Text: trouble,
				Visible: trouble != "",
			},
			d.Composite{
				Layout: d.Grid{Columns: 2},
				Children: []d.Widget{
					d.Label{Text: "A name for the new world"},
					d.LineEdit{AssignTo: &name, Text: spec.Name,
						ToolTipText: "What you will call it here. It is not the name of the save file."},
				},
			},
			d.Composite{
				Layout: d.HBox{MarginsZero: true},
				Children: []d.Widget{
					d.PushButton{AssignTo: &advanced, Text: advancedCaption,
						ToolTipText: "Everything else about the new world. All of it is already " +
							"filled in with values that work.",
						OnClicked: func() {
							open = !open
							more.SetVisible(open)
							if open {
								advanced.SetText(ButtonHideAdvanced)
							} else {
								advanced.SetText(ButtonShowAdvanced)
							}
						}},
					d.HSpacer{},
				},
			},
			d.Composite{
				AssignTo: &more,
				Visible:  open,
				Layout:   d.Grid{Columns: 2, MarginsZero: true},
				Children: []d.Widget{
					d.Label{Text: "Its save name"},
					d.LineEdit{AssignTo: &world, Text: spec.World,
						ToolTipText: "The save file the game will load for it."},

					d.Label{Text: "Its port"},
					d.LineEdit{AssignTo: &port, Text: strconv.Itoa(spec.Port),
						ToolTipText: "The port it talks to the map through. This is the lowest one " +
							"no other world on this computer holds."},

					d.Label{Text: "Its own folder"},
					d.LineEdit{AssignTo: &dataRoot, Text: spec.DataRoot,
						ToolTipText: "Where its journal, its logs and its credential will live. " +
							"No two worlds may share one."},

					d.Label{Text: "The folder the game is installed in"},
					d.LineEdit{AssignTo: &gameDir, Text: spec.GameDir,
						ToolTipText: "Where The Bibites is installed. It is the same for every world."},

					d.Label{Text: "Game window"},
					d.CheckBox{AssignTo: &headless, Text: CheckHeadless, Checked: spec.Headless,
						ToolTipText: HeadlessTip},
				},
			},
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40}, TextColor: noteColour,
				Text: Dedent(launcher.PublicMapNote()) + " " + Dedent(launcher.LeavingNote()),
			},
			d.Composite{
				Layout: d.HBox{},
				Children: []d.Widget{
					d.HSpacer{},
					d.PushButton{
						AssignTo: &accept,
						Text:     ButtonDialogCreate,
						OnClicked: func() {
							out.Name = strings.TrimSpace(name.Text())
							out.World = strings.TrimSpace(world.Text())
							out.DataRoot = strings.TrimSpace(dataRoot.Text())
							out.GameDir = strings.TrimSpace(gameDir.Text())
							out.Headless = headless.Checked()
							// A PORT THAT IS NOT A NUMBER IS NOT REJECTED HERE.
							// It is passed on as 0, which the core refuses with
							// its own message about the range a port lives in —
							// one answer to one question, from one place.
							out.Port, _ = strconv.Atoi(strings.TrimSpace(port.Text()))
							accepted = true
							dlg.Accept()
						},
					},
					d.PushButton{AssignTo: &cancel, Text: ButtonDialogCancel,
						OnClicked: func() { dlg.Cancel() }},
				},
			},
		},
	}
	if _, err := layout.Run(owner); err != nil {
		return spec, false, err
	}
	return out, accepted, nil
}

// runCloneDialog copies a world's settings onto a new identity. Everything that
// has to be unique — the identity, the port, the data folder and the save name —
// is chosen by the core, so the only question here is the new world's name.
func runCloneDialog(owner walk.Form, source, suggested string) (string, bool, error) {
	var (
		dlg    *walk.Dialog
		accept *walk.PushButton
		cancel *walk.PushButton
		name   *walk.LineEdit
	)
	out := ""
	accepted := false

	layout := d.Dialog{
		AssignTo:      &dlg,
		Title:         "Make a copy of the world '" + source + "'",
		DefaultButton: &accept,
		CancelButton:  &cancel,
		MinSize:       d.Size{Width: dialogWidth, Height: 260},
		Layout:        d.VBox{},
		Children: []d.Widget{
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40},
				Text: "The copy takes this world's settings. Its identity on the map, its own " +
					"folder, its port and its save name are all new, because two worlds cannot " +
					"share any of them. Nothing living in this world is copied.",
			},
			d.Composite{
				Layout: d.Grid{Columns: 2},
				Children: []d.Widget{
					d.Label{Text: "A name for the copy"},
					d.LineEdit{AssignTo: &name, Text: suggested,
						ToolTipText: "What you will call the new world here."},
				},
			},
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40}, TextColor: noteColour,
				Text: Dedent(launcher.PublicMapNote()),
			},
			d.Composite{
				Layout: d.HBox{},
				Children: []d.Widget{
					d.HSpacer{},
					d.PushButton{
						AssignTo: &accept,
						Text:     ButtonDialogClone,
						OnClicked: func() {
							out = strings.TrimSpace(name.Text())
							accepted = true
							dlg.Accept()
						},
					},
					d.PushButton{AssignTo: &cancel, Text: ButtonDialogCancel,
						OnClicked: func() { dlg.Cancel() }},
				},
			},
		},
	}
	if _, err := layout.Run(owner); err != nil {
		return "", false, err
	}
	return out, accepted, nil
}

// runDeleteDialog is the one dialog whose job is to make something HARDER.
//
// The typing of the name is the confirmation, exactly as it is on the command
// line, and for the same reason: the journal under a world's data root is this
// machine's record of every organism it is holding for somebody else. The typed
// value is not compared here — it is handed to the core, which compares it
// before it removes anything.
func runDeleteDialog(owner walk.Form, name, dataRoot string) (string, bool, bool, error) {
	var (
		dlg        *walk.Dialog
		accept     *walk.PushButton
		cancel     *walk.PushButton
		typed      *walk.LineEdit
		removeData *walk.CheckBox
	)
	typedName := ""
	remove := false
	accepted := false

	layout := d.Dialog{
		AssignTo:      &dlg,
		Title:         "Delete the world '" + name + "'",
		DefaultButton: &cancel, // The safe button is the one Enter presses.
		CancelButton:  &cancel,
		MinSize:       d.Size{Width: dialogWidth, Height: 380},
		Layout:        d.VBox{},
		Children: []d.Widget{
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40},
				Text:    Dedent(launcher.CustodyWarning()),
			},
			d.CheckBox{
				AssignTo: &removeData,
				Text:     CheckRemoveWorldData,
				ToolTipText: "Deletes this world's journal, its logs and its credential from " +
					dataRoot + ". Anything else in that folder is left where it is.",
			},
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40}, TextColor: noteColour,
				Text: "That folder is " + dataRoot + ". The game's own save file is NOT in it and " +
					"is never touched: it stays with the game, whichever box you tick.",
			},
			d.Composite{
				Layout: d.Grid{Columns: 2},
				Children: []d.Widget{
					d.Label{Text: "Type " + name + " to confirm"},
					d.LineEdit{AssignTo: &typed, CueBanner: name,
						ToolTipText: "The launcher deletes nothing unless this matches the world's name."},
				},
			},
			d.Composite{
				Layout: d.HBox{},
				Children: []d.Widget{
					d.HSpacer{},
					d.PushButton{
						AssignTo: &accept,
						Text:     ButtonDialogDelete,
						OnClicked: func() {
							typedName = strings.TrimSpace(typed.Text())
							remove = removeData.Checked()
							accepted = true
							dlg.Accept()
						},
					},
					d.PushButton{AssignTo: &cancel, Text: ButtonDialogCancel,
						OnClicked: func() { dlg.Cancel() }},
				},
			},
		},
	}
	if _, err := layout.Run(owner); err != nil {
		return "", false, false, err
	}
	return typedName, remove, accepted, nil
}
