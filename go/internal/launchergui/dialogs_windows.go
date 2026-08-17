//go:build windows

package launchergui

// THE DIALOGS. Each one collects values and NOTHING ELSE: no validation, no
// decision, no refusal. The core is what judges a port, a save name, an edge
// list or a data folder, and its answer is what the participant reads in the log
// pane — so a dialog that checked a value here would either agree with the core
// (and be dead code) or disagree with it (and be a lie in a window).
//
// The one thing a dialog does decide is whether the action happens at all, and
// for the destructive one that is the whole of its job: the world's name, typed,
// which is then handed to the core's own comparison (launcher.Session.Delete).

import (
	"strconv"
	"strings"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"

	"multiverse/internal/launcher"
)

// dialogWidth is wide enough for a Windows path in a single line edit, which is
// what most of these fields hold.
const dialogWidth = 620

// runEditDialog is menu item 5 of the console menu, as one form: every mutable
// field at once, rather than one at a time.
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
		MinSize:       d.Size{Width: dialogWidth, Height: 380},
		Layout:        d.VBox{},
		Children: []d.Widget{
			d.Composite{
				Layout: d.Grid{Columns: 2},
				Children: []d.Widget{
					d.Label{Text: "save name (the world the game loads)"},
					d.LineEdit{AssignTo: &save, Text: form.Save},

					d.Label{Text: "sidecar port"},
					d.LineEdit{AssignTo: &port, Text: form.Port},

					d.Label{Text: "run it with nothing drawn"},
					d.CheckBox{AssignTo: &headless, Text: "headless", Checked: form.Headless},

					d.Label{Text: "export edges (E,N,W,S)"},
					d.LineEdit{AssignTo: &edges, Text: form.ExportEdges},

					d.Label{Text: "species that never leave"},
					d.LineEdit{AssignTo: &species, Text: form.ExcludeSpecies,
						CueBanner: "empty turns the exclusion policy OFF"},

					d.Label{Text: "save every N minutes (0 turns the timer off)"},
					d.LineEdit{AssignTo: &minutes, Text: form.SaveMinutes},

					d.Label{Text: "saves kept"},
					d.LineEdit{AssignTo: &keep, Text: form.SaveKeep},

					d.Label{Text: "write the world out when the game closes"},
					d.CheckBox{AssignTo: &onQuit, Text: "save on quit", Checked: form.SaveOnQuit},

					d.Label{Text: "the folder the game is installed in"},
					d.LineEdit{AssignTo: &gameDir, Text: form.GameDir},
				},
			},
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40},
				Text: "Only what you change is written. Emptying the species field turns the " +
					"migration exclusion policy off, which is a real choice and is reported as one.",
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

// runCreateDialog collects a new world AND SAYS WHAT CREATING ONE COSTS before
// it is agreed to: a new identity on the map, a per-address limit that a run of
// quick creates will meet, and the fact that deleting a world here is not
// leaving the map. The console prints those after the fact; a dialog can say
// them first, so the confirm button is what enrolls.
func runCreateDialog(owner walk.Form, spec launcher.CreateSpec) (launcher.CreateSpec, bool, error) {
	var (
		dlg      *walk.Dialog
		accept   *walk.PushButton
		cancel   *walk.PushButton
		name     *walk.LineEdit
		world    *walk.LineEdit
		port     *walk.LineEdit
		dataRoot *walk.LineEdit
		gameDir  *walk.LineEdit
		headless *walk.CheckBox
	)
	out := spec
	accepted := false

	layout := d.Dialog{
		AssignTo:      &dlg,
		Title:         "Create another world on this computer",
		DefaultButton: &accept,
		CancelButton:  &cancel,
		MinSize:       d.Size{Width: dialogWidth, Height: 420},
		Layout:        d.VBox{},
		Children: []d.Widget{
			d.Composite{
				Layout: d.Grid{Columns: 2},
				Children: []d.Widget{
					d.Label{Text: "a name for the new world"},
					d.LineEdit{AssignTo: &name, Text: spec.Name},

					d.Label{Text: "its save name"},
					d.LineEdit{AssignTo: &world, Text: spec.World},

					d.Label{Text: "its sidecar port"},
					d.LineEdit{AssignTo: &port, Text: strconv.Itoa(spec.Port)},

					d.Label{Text: "its data folder"},
					d.LineEdit{AssignTo: &dataRoot, Text: spec.DataRoot},

					d.Label{Text: "the folder the game is installed in"},
					d.LineEdit{AssignTo: &gameDir, Text: spec.GameDir},

					d.Label{Text: "run it with nothing drawn"},
					d.CheckBox{AssignTo: &headless, Text: "headless", Checked: spec.Headless},
				},
			},
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40},
				Text:    strings.TrimRight(launcher.PublicMapNote(), "\n"),
			},
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40},
				Text:    strings.TrimRight(launcher.LeavingNote(), "\n"),
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
		Title:         "Clone the world '" + source + "'",
		DefaultButton: &accept,
		CancelButton:  &cancel,
		MinSize:       d.Size{Width: dialogWidth, Height: 240},
		Layout:        d.VBox{},
		Children: []d.Widget{
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40},
				Text: "A clone copies this world's settings. Its identity on the map, its data " +
					"folder, its port and its save name are all new, because two worlds cannot " +
					"share any of them.",
			},
			d.Composite{
				Layout: d.Grid{Columns: 2},
				Children: []d.Widget{
					d.Label{Text: "a name for the new world"},
					d.LineEdit{AssignTo: &name, Text: suggested},
				},
			},
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40},
				Text:    strings.TrimRight(launcher.PublicMapNote(), "\n"),
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
		MinSize:       d.Size{Width: dialogWidth, Height: 340},
		Layout:        d.VBox{},
		Children: []d.Widget{
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40},
				Text:    strings.TrimRight(launcher.CustodyWarning(), "\n"),
			},
			d.TextLabel{
				MinSize: d.Size{Width: dialogWidth - 40},
				Text:    "This world's data folder is " + dataRoot + ".",
			},
			d.CheckBox{
				AssignTo: &removeData,
				Text: "also remove this world's data (journal, logs, credential). " +
					"Anything in that folder that is not this world's is left where it is.",
			},
			d.Composite{
				Layout: d.Grid{Columns: 2},
				Children: []d.Widget{
					d.Label{Text: "type the world's name to delete it (" + name + ")"},
					d.LineEdit{AssignTo: &typed, CueBanner: name},
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
