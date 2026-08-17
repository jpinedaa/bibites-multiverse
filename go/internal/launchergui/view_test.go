package launchergui

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"multiverse/internal/launcher"
)

// These tests run on the machine this project is developed on, which has no
// Windows. What they cover is everything about the window that is not a widget:
// the table's columns against the values put in them, the buttons a state
// enables, the flags a dialog turns into a 'profile set', and the log pane's
// line assembly.

func aWorld() launcher.WorldView {
	return launcher.WorldView{
		ProfileStatus: launcher.ProfileStatus{
			Name:           "default",
			Active:         true,
			World:          "Multiverse",
			GameDir:        `C:\Games\The Bibites`,
			DataRoot:       `C:\Users\p\AppData\Local\BibitesMultiverse`,
			SidecarPort:    8787,
			Headless:       false,
			ExportEdges:    "E,N,W,S",
			ExcludeSpecies: "Basic bibite",
			SaveMinutes:    10,
			SaveKeep:       6,
			SaveOnQuit:     true,
			PeerID:         "public-9af42a17616742e7a6e8c62cb8b95f4f",
			RelayURL:       "wss://bibitesmultiverse.com/contract-b/v4",
			Sidecar:        launcher.ProcessStatus{PID: 4242, Running: true},
			Game:           launcher.ProcessStatus{PID: 11004, Running: true},
		},
		Mod: launcher.ModView{
			Answered: true, Connected: true, Version: "0.6.7",
			TimeScale: 10, Achieved: 6.5,
			Slot: 3, SlotKnown: true, Live: true, RelayConnected: true,
		},
	}
}

// Every column has a header and every header has a value. A table whose Value
// switch has drifted from its Columns list shows one column's data under
// another's title, silently.
func TestEveryColumnHasAHeaderAndAValue(t *testing.T) {
	columns := Columns()
	if len(columns) != columnCount {
		t.Fatalf("there are %d columns and %d column indices", len(columns), columnCount)
	}
	row := RowFrom(aWorld())
	for i, column := range columns {
		if column.Title == "" {
			t.Fatalf("column %d has no title", i)
		}
		if column.Width <= 0 {
			t.Fatalf("column %q has width %d", column.Title, column.Width)
		}
		if row.Cell(i) == "" {
			t.Fatalf("column %q renders nothing for a whole world", column.Title)
		}
	}
	// Out of range is empty rather than a panic: a table asks for the columns it
	// was given, and a mismatch must not take the window down.
	if row.Cell(columnCount) != "" || row.Cell(-1) != "" {
		t.Fatal("a column that does not exist rendered something")
	}
	// The active world is marked, the way the console status marks it.
	if !strings.HasPrefix(row.Cell(ColWorld), "* ") {
		t.Fatalf("the active world is not marked: %q", row.Cell(ColWorld))
	}
}

// The four states of "is this world actually on the map" are four different
// words. Collapsing any two of them is how a world with no game behind it looks
// healthy, which is the whole reason this column exists.
func TestTheMapColumnNeverCollapsesTheFourStates(t *testing.T) {
	world := aWorld()
	if got := RowFrom(world).Mod; got != "connected (mod 0.6.7)" {
		t.Fatalf("a connected world reads %q", got)
	}

	stopped := aWorld()
	stopped.Sidecar.Running = false
	stopped.Game.Running = false
	stopped.Mod = launcher.ModView{}
	if got := RowFrom(stopped).Mod; got != "-" {
		t.Fatalf("a stopped world reads %q", got)
	}

	unanswered := aWorld()
	unanswered.Mod = launcher.ModView{}
	if got := RowFrom(unanswered).Mod; got != "sidecar not answering" {
		t.Fatalf("a sidecar that says nothing reads %q", got)
	}

	// THE ONE THAT MATTERS: the sidecar holds the slot, the game is up, and the
	// mod never arrived. The map shows this world live with nothing behind it.
	orphan := aWorld()
	orphan.Mod = launcher.ModView{Answered: true}
	got := RowFrom(orphan).Mod
	if !strings.Contains(got, "NOT CONNECTED") {
		t.Fatalf("a world whose mod never arrived reads %q", got)
	}
	if RowFrom(orphan).Speed != "-" && RowFrom(orphan).Speed != "" {
		t.Fatalf("a world with no mod reported a speed: %q", RowFrom(orphan).Speed)
	}

	// A sidecar up with no game yet is neither of those.
	waiting := aWorld()
	waiting.Game.Running = false
	waiting.Mod = launcher.ModView{Answered: true}
	if got := RowFrom(waiting).Mod; got != "no game" {
		t.Fatalf("a world whose game is not running reads %q", got)
	}
}

// The speed column carries the target AND what the world produced, because the
// gap between them is the reading.
func TestSpeedShowsTargetAndAchieved(t *testing.T) {
	world := aWorld()
	if got := RowFrom(world).Speed; got != "x10 (6.5 achieved)" {
		t.Fatalf("the speed reads %q", got)
	}
	// A mod that has not measured a span yet reports the target alone rather
	// than claiming zero.
	world.Mod.Achieved = 0
	if got := RowFrom(world).Speed; got != "x10" {
		t.Fatalf("an unmeasured speed reads %q", got)
	}
	slotless := aWorld()
	slotless.Mod.SlotKnown = false
	if got := RowFrom(slotless).Slot; got != "-" {
		t.Fatalf("an unknown slot reads %q", got)
	}
	notLive := aWorld()
	notLive.Mod.Live = false
	if got := RowFrom(notLive).Slot; got != "3 (not live)" {
		t.Fatalf("a slot that is not live reads %q", got)
	}
}

func TestRowsFromASnapshot(t *testing.T) {
	second := aWorld()
	second.Name, second.World, second.Active = "second", "Second", false
	second.Sidecar.Running, second.Game.Running = false, false
	second.Headless = true
	second.Mod = launcher.ModView{}
	rows := RowsFrom(launcher.Snapshot{Worlds: []launcher.WorldView{aWorld(), second}})
	if len(rows) != 2 {
		t.Fatalf("%d rows from two worlds", len(rows))
	}
	if rows[1].Headless != "no window" || rows[0].Headless != "window" {
		t.Fatalf("the window column reads %q and %q", rows[0].Headless, rows[1].Headless)
	}
	if rows[1].Sidecar != "stopped" {
		t.Fatalf("a stopped sidecar reads %q", rows[1].Sidecar)
	}
	if !strings.Contains(rows[0].Sidecar, "4242") {
		t.Fatalf("a running sidecar does not name its pid: %q", rows[0].Sidecar)
	}
}

// A button that is enabled when the core would refuse it is a refusal a
// participant has to read for no reason, and a button that is disabled when the
// core would accept it is a feature they cannot reach.
func TestButtonsFollowWhatTheCoreWouldAccept(t *testing.T) {
	running := aWorld()
	stopped := aWorld()
	stopped.Name, stopped.Active = "second", false
	stopped.Sidecar.Running, stopped.Game.Running = false, false
	snap := launcher.Snapshot{Worlds: []launcher.WorldView{running, stopped}}

	onRunning := ActionsFor(&running, snap, false)
	if onRunning.Start || onRunning.StartOverride {
		t.Fatal("a running world offered Start")
	}
	if !onRunning.Stop || !onRunning.StopAll {
		t.Fatal("a running world did not offer Stop")
	}
	if onRunning.Delete {
		t.Fatal("a running world offered Delete; the core refuses it")
	}
	if onRunning.SetDefault {
		t.Fatal("the active world offered Set as default")
	}
	if !onRunning.CopyPeerID || !onRunning.Diagnose || !onRunning.OpenLogs {
		t.Fatalf("a running world lost a reading action: %+v", onRunning)
	}

	onStopped := ActionsFor(&stopped, snap, false)
	if !onStopped.Start || !onStopped.StartOverride || !onStopped.Delete || !onStopped.SetDefault {
		t.Fatalf("a stopped world is missing an action: %+v", onStopped)
	}
	if onStopped.Stop {
		t.Fatal("a stopped world offered Stop")
	}

	// Nothing selected: an installation with no worlds at all can still make one.
	empty := ActionsFor(nil, launcher.Snapshot{}, false)
	if !empty.Create {
		t.Fatal("an empty installation cannot create a world")
	}
	if empty.Start || empty.Stop || empty.Delete || empty.StopAll {
		t.Fatalf("an empty installation offered an action: %+v", empty)
	}

	// While an action runs, nothing else can be pressed: the core's per-world
	// lock would refuse it and a grey button says so better than a refusal.
	busy := ActionsFor(&stopped, snap, true)
	if busy.Start || busy.Stop || busy.Create || busy.Delete || busy.Diagnose {
		t.Fatalf("a busy window still offered actions: %+v", busy)
	}

	// The override button always offers the OTHER way round.
	if onStopped.OverrideOffers != ButtonStartHeadless {
		t.Fatalf("a windowed world offers %q", onStopped.OverrideOffers)
	}
	headless := stopped
	headless.Headless = true
	if got := ActionsFor(&headless, snap, false).OverrideOffers; got != ButtonStartWindowed {
		t.Fatalf("a headless world offers %q", got)
	}
	if StartOverrideCaption(true) == StartOverrideCaption(false) {
		t.Fatal("the two override captions are the same string")
	}
}

// Every caption the harness clicks by is unique: two controls with one caption
// is an ambiguous click.
func TestButtonCaptionsAreUnique(t *testing.T) {
	captions := []string{
		ButtonStart, ButtonStop, ButtonStopAll, ButtonSetDefault, ButtonEdit,
		ButtonCreate, ButtonClone, ButtonDelete, ButtonDiagnose, ButtonOpenLogs,
		ButtonOpenGameLog, ButtonCopyPeerID, ButtonCopyLog,
		ButtonStartWindowed, ButtonStartHeadless,
		ButtonDialogSave, ButtonDialogCreate, ButtonDialogClone, ButtonDialogDelete,
		ButtonDialogCancel,
	}
	seen := make(map[string]bool, len(captions))
	for _, caption := range captions {
		if caption == "" {
			t.Fatal("a button has no caption")
		}
		if seen[caption] {
			t.Fatalf("two buttons are captioned %q", caption)
		}
		seen[caption] = true
	}
}

// An edit sends only what changed, and it sends the exclusion policy's own
// switch rather than an empty value the core would refuse.
func TestEditFlagsSendOnlyWhatChanged(t *testing.T) {
	before := launcher.Profile{
		Name: "default", World: "Multiverse", SidecarPort: 8787,
		ExportEdges: "E,N,W,S", ExcludeSpecies: "Basic bibite",
		SaveMinutes: 10, SaveKeep: 6, SaveOnQuit: true,
		GameDir: `C:\Games\The Bibites`,
	}
	if flags := EditFlags(before, FormFor(before)); len(flags) != 0 {
		t.Fatalf("an unchanged form produced %v", flags)
	}

	form := FormFor(before)
	form.Port = " 8790 "
	form.Headless = true
	form.SaveMinutes = "5"
	got := EditFlags(before, form)
	want := []string{"--sidecar-port", "8790", "--headless", "--save-minutes", "5"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("the flags are %v, want %v", got, want)
	}

	// Emptying the species field is turning the policy OFF, which has its own
	// switch. --exclude-species "" is refused by the core, and rightly.
	form = FormFor(before)
	form.ExcludeSpecies = "   "
	got = EditFlags(before, form)
	if strings.Join(got, " ") != "--no-migration-exclusion" {
		t.Fatalf("emptying the species list produced %v", got)
	}

	// Turning the window back on is --no-headless rather than a missing flag.
	headless := before
	headless.Headless = true
	form = FormFor(headless)
	form.Headless = false
	if strings.Join(EditFlags(headless, form), " ") != "--no-headless" {
		t.Fatalf("clearing headless produced %v", EditFlags(headless, form))
	}

	form = FormFor(before)
	form.SaveOnQuit = false
	if strings.Join(EditFlags(before, form), " ") != "--save-on-quit off" {
		t.Fatalf("clearing save-on-quit produced %v", EditFlags(before, form))
	}
}

// The log pane's lines: one stamp per line, whole lines only, and a blank line
// stays blank.
func TestLogAssemblesWholeLinesWithOneStampEach(t *testing.T) {
	at := time.Date(2026, 8, 17, 9, 41, 7, 0, time.UTC)
	var lines []string
	log := NewLog(func() time.Time { return at }, func(line string) {
		lines = append(lines, line)
	})

	// One sentence in three writes, which is what fmt.Fprintf does.
	log.Write([]byte("sidecar started "))
	log.Write([]byte("(pid 42)"))
	if len(lines) != 0 {
		t.Fatalf("an unfinished line was emitted: %v", lines)
	}
	log.Write([]byte("\n"))
	if len(lines) != 1 || lines[0] != "09:41:07  sidecar started (pid 42)" {
		t.Fatalf("the assembled line is %v", lines)
	}

	// Several lines in one write, a blank line among them, and Windows line
	// endings.
	lines = nil
	log.Write([]byte("YOU ARE ON THE MAP:\r\n\r\n  slot granted\r\n"))
	if len(lines) != 3 {
		t.Fatalf("%d lines from three: %v", len(lines), lines)
	}
	if lines[1] != "" {
		t.Fatalf("a blank line became %q", lines[1])
	}
	for _, line := range []string{lines[0], lines[2]} {
		if !strings.HasPrefix(line, "09:41:07  ") {
			t.Fatalf("a line lost its stamp: %q", line)
		}
		if strings.Contains(line, "\r") {
			t.Fatalf("a carriage return reached the pane: %q", line)
		}
	}

	// A prompt has no newline, and Flush is what puts it on screen.
	lines = nil
	log.Write([]byte("type the world's name: "))
	log.Flush()
	if len(lines) != 1 || !strings.HasSuffix(lines[0], "type the world's name: ") {
		t.Fatalf("Flush emitted %v", lines)
	}
	// Flushing nothing emits nothing, so an idle window does not fill with
	// stamps.
	lines = nil
	log.Flush()
	if len(lines) != 0 {
		t.Fatalf("Flush on an empty log emitted %v", lines)
	}
}

// Both of the core's streams point at one log, and the action goroutine is not
// the only writer. A line must never interleave with another line.
func TestLogIsSafeForSeveralWriters(t *testing.T) {
	var mu sync.Mutex
	var lines []string
	log := NewLog(time.Now, func(line string) {
		mu.Lock()
		lines = append(lines, line)
		mu.Unlock()
	})
	var writers sync.WaitGroup
	for writer := 0; writer < 8; writer++ {
		writers.Add(1)
		go func(id int) {
			defer writers.Done()
			for i := 0; i < 50; i++ {
				log.Write([]byte(strings.Repeat(string(rune('a'+id)), 20) + "\n"))
			}
		}(writer)
	}
	writers.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 400 {
		t.Fatalf("%d lines from 400 writes", len(lines))
	}
	for _, line := range lines {
		body := line[strings.LastIndex(line, "  ")+2:]
		if len(body) != 20 {
			t.Fatalf("a line was interleaved: %q", line)
		}
		if strings.Count(body, body[:1]) != 20 {
			t.Fatalf("a line mixes two writers: %q", line)
		}
	}
}

// The window forwards a command line to the console program beside it.
func TestConsoleExePathIsBesideTheWindow(t *testing.T) {
	// The separator is the host's, so what this pins is the two halves: the
	// window's own folder, and the console program rather than the window.
	got := ConsoleExePath(filepath.Join("somewhere", "Bibites Multiverse", launcher.LauncherExeName))
	if filepath.Base(got) != launcher.ConsoleExeName {
		t.Fatalf("the forwarded program is %q, want %q", got, launcher.ConsoleExeName)
	}
	if filepath.Dir(got) != filepath.Join("somewhere", "Bibites Multiverse") {
		t.Fatalf("the forwarded program is not beside the window: %q", got)
	}
}
