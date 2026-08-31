package launchergui

import (
	"path/filepath"
	"strings"
	"testing"

	"multiverse/internal/launcher"
)

// These tests run on the machine this project is developed on, which has no
// Windows. What they cover is everything about the window that is not a widget:
// the words a state is rendered as, the colour that goes with them, which
// control a state enables, and the flags a dialog turns into a 'profile set'.

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

func stoppedWorld() launcher.WorldView {
	world := aWorld()
	world.Sidecar.Running, world.Game.Running = false, false
	world.Mod = launcher.ModView{}
	return world
}

// THE STATE TABLE. Every state is a different sentence AND a different colour,
// and the two that must never collapse into one are "on the map" and "running
// but not on the map": when those two look alike, the map shows a world live
// with nothing behind it and every other sign on the machine looks healthy.
// That is LOCAL-CONFIGRACE and LOCAL-STARVATION in docs/error-taxonomy.md, and
// it is the reason this window exists at all.
func TestEveryStateHasItsOwnWordsAndColour(t *testing.T) {
	waiting := aWorld()
	waiting.Game.Running = false
	waiting.Mod = launcher.ModView{Answered: true, Slot: 3, SlotKnown: true, Live: true}

	orphan := aWorld()
	orphan.Mod = launcher.ModView{Answered: true}

	starting := aWorld()
	starting.Game.Running = false
	starting.Mod = launcher.ModView{}

	gameAlone := aWorld()
	gameAlone.Sidecar.Running = false
	gameAlone.Mod = launcher.ModView{}

	cases := []struct {
		name   string
		world  launcher.WorldView
		busy   Busy
		state  State
		colour Colour
		// short and headline are checked as substrings, so the wording can be
		// improved without rewriting the table — but the DISTINCTION each one
		// carries is pinned.
		short    string
		headline string
	}{
		{"stopped", stoppedWorld(), Busy{}, StateStopped, ColourGrey, "Stopped", "Stopped"},
		{"an action of ours is running", stoppedWorld(),
			Busy{World: "default", Short: "Starting...", Phrase: "Waiting for the map to give this world a place..."},
			StateWorking, ColourAmber, "Starting...", "Waiting for the map"},
		{"up, nothing answering yet", starting, Busy{}, StateStarting, ColourAmber,
			"Starting...", "waiting for the map to answer"},
		{"on the map, no game yet", waiting, Busy{}, StateWaiting, ColourAmber,
			"Waiting for the game", "waiting for the game to join"},
		{"on the map with a game behind it", aWorld(), Busy{}, StateOnTheMap, ColourGreen,
			"On the map", "Running - on the map (place 3) - speed x10"},
		{"running, and NOT on the map", orphan, Busy{}, StateNotOnTheMap, ColourRed,
			"NOT on the map", "Running, but NOT on the map"},
		{"a game with nothing holding its place", gameAlone, Busy{}, StateNoMapLink, ColourRed,
			"NOT on the map", "no link to the map"},
	}
	for _, test := range cases {
		got := StatusFor(test.world, test.busy)
		if got.State != test.state {
			t.Fatalf("%s: state %d, want %d", test.name, got.State, test.state)
		}
		if got.Colour != test.colour {
			t.Fatalf("%s: colour %d, want %d", test.name, got.Colour, test.colour)
		}
		if !strings.Contains(got.Short, test.short) {
			t.Fatalf("%s: the list says %q, want %q in it", test.name, got.Short, test.short)
		}
		if !strings.Contains(got.Headline, test.headline) {
			t.Fatalf("%s: the panel says %q, want %q in it", test.name, got.Headline, test.headline)
		}
	}

	// The distinction that costs a participant their world when it is lost: the
	// two red states and the green one are three different sentences.
	onMap := StatusFor(aWorld(), Busy{})
	notOnMap := StatusFor(orphan, Busy{})
	if onMap.Headline == notOnMap.Headline || onMap.Short == notOnMap.Short {
		t.Fatal("a world on the map and a world with no mod behind it read the same")
	}
	if !strings.Contains(notOnMap.Headline, "NOT") {
		t.Fatalf("the state that matters is not shouted: %q", notOnMap.Headline)
	}
}

// A slot the map has not granted yet is SAID NOTHING ABOUT rather than rendered
// as a dash: "(place -)" in a sentence reads as a fault instead of as a fact not
// in yet.
func TestTheHeadlineOnlyNamesWhatIsKnown(t *testing.T) {
	slotless := aWorld()
	slotless.Mod.SlotKnown = false
	if got := StatusFor(slotless, Busy{}).Headline; strings.Contains(got, "place") {
		t.Fatalf("an unknown place was named: %q", got)
	}
	notLive := aWorld()
	notLive.Mod.Live = false
	if got := StatusFor(notLive, Busy{}).Headline; !strings.Contains(got, "not live") {
		t.Fatalf("a place that is not live yet reads %q", got)
	}
	unmeasured := aWorld()
	unmeasured.Mod.TimeScale = 0
	if got := StatusFor(unmeasured, Busy{}).Headline; strings.Contains(got, "speed") {
		t.Fatalf("a world that has not said its speed claimed one: %q", got)
	}
}

// A busy world is amber in the list wherever the action came from, and 'stop
// every world' covers all of them at once.
func TestBusyCoversTheWorldsAnActionIsAbout(t *testing.T) {
	one := aWorld()
	two := aWorld()
	two.Name, two.Active = "second", false
	snap := launcher.Snapshot{Worlds: []launcher.WorldView{one, two}}

	rows := RowsFrom(snap, Job{Kind: JobStop, World: "second"}.Busy())
	if rows[0].Status.State == StateWorking {
		t.Fatal("a world no action is about was marked busy")
	}
	if rows[1].Status.State != StateWorking || rows[1].Status.Colour != ColourAmber {
		t.Fatalf("the world being stopped reads %+v", rows[1].Status)
	}
	// Names are compared without case, because the file system a profile lands
	// on is.
	if rows := RowsFrom(snap, Job{Kind: JobStop, World: "SECOND"}.Busy()); rows[1].Status.State != StateWorking {
		t.Fatal("a world named in another case was not matched")
	}
	all := RowsFrom(snap, Job{Kind: JobStopAll}.Busy())
	for i, row := range all {
		if row.Status.State != StateWorking {
			t.Fatalf("row %d was not covered by 'stop every world': %+v", i, row.Status)
		}
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
	row := RowFrom(aWorld(), Busy{})
	for i, column := range columns {
		if column.Title == "" {
			t.Fatalf("column %d has no title", i)
		}
		if column.Width <= 0 {
			t.Fatalf("column %q has width %d", column.Title, column.Width)
		}
		if strings.TrimSpace(row.Cell(i)) == "" {
			t.Fatalf("column %q renders nothing for a whole world", column.Title)
		}
	}
	// Out of range is empty rather than a panic: a table asks for the columns it
	// was given, and a mismatch must not take the window down.
	if row.Cell(columnCount) != "" || row.Cell(-1) != "" {
		t.Fatal("a column that does not exist rendered something")
	}
	// The default world is marked, the way the console status marks it.
	if !strings.HasPrefix(row.Cell(ColWorld), "* ") {
		t.Fatalf("the default world is not marked: %q", row.Cell(ColWorld))
	}
	if strings.HasPrefix(RowFrom(stoppedNamed("second"), Busy{}).Cell(ColWorld), "*") {
		t.Fatal("a world that is not the default was marked")
	}
}

func stoppedNamed(name string) launcher.WorldView {
	world := stoppedWorld()
	world.Name, world.Active = name, false
	return world
}

// THE PANEL IS ONE DECISION, taken here and moved into widgets by the window. A
// test that can read a whole screen is what keeps the window from growing rules
// of its own.
func TestThePanelSaysOneThingAboutOneWorld(t *testing.T) {
	world := aWorld()
	snap := launcher.Snapshot{Worlds: []launcher.WorldView{world}}
	panel := PanelFor(&world, snap, Busy{}, NewResultLog())
	if panel.World != "default" {
		t.Fatalf("the panel names %q", panel.World)
	}
	if panel.Primary.Caption != ButtonStop || !panel.Primary.Enabled {
		t.Fatalf("a running world's button is %+v", panel.Primary)
	}
	if panel.Working {
		t.Fatal("an idle window is showing a progress bar")
	}
	if panel.Primary.Tip == "" || panel.Primary.Tip == panel.Primary.Caption {
		t.Fatalf("the primary button's tooltip is %q", panel.Primary.Tip)
	}
	// The facts are FIXED IN ORDER AND IN LENGTH, because the grid is built once
	// and only its values change. A short list would leave a value under the
	// wrong label.
	labels := FactLabels()
	if len(panel.Facts) != len(labels) {
		t.Fatalf("%d facts for %d labels", len(panel.Facts), len(labels))
	}
	for i, label := range labels {
		if panel.Facts[i].Label != label {
			t.Fatalf("fact %d is %q, want %q", i, panel.Facts[i].Label, label)
		}
	}
	byLabel := map[string]string{}
	for _, fact := range panel.Facts {
		byLabel[fact.Label] = fact.Value
	}
	if byLabel[FactSave] != "Multiverse" || byLabel[FactPort] != "8787" {
		t.Fatalf("the facts are %v", byLabel)
	}
	if byLabel[FactSpeed] != "x10 (target), x6.5 (achieved)" {
		t.Fatalf("the speed reads %q", byLabel[FactSpeed])
	}
	if byLabel[FactIdentity] != world.PeerID || byLabel[FactData] != world.DataRoot {
		t.Fatalf("the facts are %v", byLabel)
	}

	// A world that has not measured a span yet reports the target alone rather
	// than claiming zero.
	world.Mod.Achieved = 0
	if got := PanelFor(&world, snap, Busy{}, NewResultLog()).Facts[2].Value; got != "x10 (target)" {
		t.Fatalf("an unmeasured speed reads %q", got)
	}

	// A DASH IS NOT AN ANSWER. The two reasons there is no number are different
	// and each of them is a short sentence.
	if got := speedFact(stoppedWorld()); got != "not running" {
		t.Fatalf("a stopped world's speed reads %q", got)
	}
	starting := aWorld()
	starting.Game.Running = false
	starting.Mod = launcher.ModView{}
	if got := speedFact(starting); got != "not measured yet" {
		t.Fatalf("a starting world's speed reads %q", got)
	}
	for _, fact := range PanelFor(&world, snap, Busy{}, NewResultLog()).Facts {
		if fact.Value == "-" {
			t.Fatalf("the %s fact is a dash", fact.Label)
		}
	}

	// A stopped world offers Start.
	stopped := stoppedWorld()
	if got := PanelFor(&stopped, snap, Busy{}, NewResultLog()).Primary.Caption; got != ButtonStart {
		t.Fatalf("a stopped world's button is %q", got)
	}

	// While anything at all is running, the panel says what is happening and the
	// button cannot be pressed — including for a job about a DIFFERENT world,
	// because there is one action goroutine and it is occupied.
	busy := Job{Kind: JobCreate, World: "world2"}.Busy()
	during := PanelFor(&stopped, snap, busy, NewResultLog())
	if !during.Working || during.Primary.Enabled {
		t.Fatalf("a busy window offered its primary action: %+v", during)
	}
	if !strings.Contains(during.Headline, "world2") {
		t.Fatalf("the panel does not say what is happening: %q", during.Headline)
	}
}

// The first run: one world, stopped, and a person who has never seen this
// window. The headline alone does not tell them what to do.
func TestTheFirstRunSaysWhatToDo(t *testing.T) {
	world := stoppedWorld()
	one := launcher.Snapshot{Worlds: []launcher.WorldView{world}}
	hint := PanelFor(&world, one, Busy{}, NewResultLog()).Hint
	if !strings.Contains(hint, ButtonStart) {
		t.Fatalf("a first run is told %q", hint)
	}

	// With two worlds this person has used the window before, and a standing
	// instruction under every stopped world is noise.
	second := stoppedNamed("second")
	two := launcher.Snapshot{Worlds: []launcher.WorldView{world, second}}
	if got := PanelFor(&world, two, Busy{}, NewResultLog()).Hint; got != "" {
		t.Fatalf("an experienced installation is told %q", got)
	}

	// A world that is running and not on the map is told where to look, whatever
	// else is installed.
	orphan := aWorld()
	orphan.Mod = launcher.ModView{Answered: true}
	if got := PanelFor(&orphan, two, Busy{}, NewResultLog()).Hint; !strings.Contains(got, "details") {
		t.Fatalf("a world that is not on the map is told %q", got)
	}

	// Nothing installed at all: the panel is not blank, and it names the button.
	empty := PanelFor(nil, launcher.Snapshot{}, Busy{}, NewResultLog())
	if empty.Headline == "" || !strings.Contains(empty.Hint, ButtonCreate) {
		t.Fatalf("an empty installation's panel is %+v", empty)
	}
	if len(empty.Facts) != len(FactLabels()) {
		t.Fatal("an empty panel dropped the facts grid, so its labels would move")
	}
}

// The banner REPORTS AND THEN SAYS WHAT TO DO. An installation whose files
// cannot be read must never look like an installation with no worlds.
func TestTheBannerNamesTheFileAndTheRemedy(t *testing.T) {
	if got := BannerFor(launcher.Snapshot{Worlds: []launcher.WorldView{aWorld()}}); got != "" {
		t.Fatalf("a healthy installation showed %q", got)
	}
	broken := launcher.Snapshot{
		Worlds:   []launcher.WorldView{aWorld()},
		Problems: []string{`C:\...\profiles\broken.json is not readable as a world profile`},
	}
	got := BannerFor(broken)
	if !strings.Contains(got, "broken.json") || !strings.Contains(got, "installer") {
		t.Fatalf("the banner reads %q", got)
	}
	empty := BannerFor(launcher.Snapshot{})
	if !strings.Contains(empty, ButtonCreate) {
		t.Fatalf("an empty installation's banner reads %q", empty)
	}
}

// The title bar carries the one fact somebody wants from the taskbar.
func TestTheTitleSaysHowManyWorldsAreRunning(t *testing.T) {
	// A window with nothing running keeps the frozen caption exactly, because
	// that is the string the machine harness looks for.
	if got := WindowTitleFor(launcher.Snapshot{Worlds: []launcher.WorldView{stoppedWorld()}}); got != WindowTitle {
		t.Fatalf("an idle window is titled %q", got)
	}
	snap := launcher.Snapshot{Worlds: []launcher.WorldView{aWorld(), stoppedNamed("second")}}
	got := WindowTitleFor(snap)
	if !strings.HasPrefix(got, WindowTitle) {
		t.Fatalf("the title no longer starts with the frozen caption: %q", got)
	}
	if !strings.Contains(got, "1 of 2") {
		t.Fatalf("the title reads %q", got)
	}
}

// A control that is enabled when the core would refuse it is a refusal a
// participant has to read for no reason, and a control that is disabled when the
// core would accept it is a feature they cannot reach.
func TestControlsFollowWhatTheCoreWouldAccept(t *testing.T) {
	running := aWorld()
	stopped := stoppedNamed("second")
	snap := launcher.Snapshot{Worlds: []launcher.WorldView{running, stopped}}

	onRunning := ActionsFor(&running, snap, false)
	if onRunning.Start {
		t.Fatal("a running world offered Start")
	}
	if !onRunning.Stop || !onRunning.StopAll {
		t.Fatal("a running world did not offer Stop")
	}
	if onRunning.Delete {
		t.Fatal("a running world offered Delete; the core refuses it")
	}
	if onRunning.SetDefault {
		t.Fatal("the default world offered Set as the default world")
	}
	if !onRunning.CopyPeerID || !onRunning.Diagnose || !onRunning.OpenLogs || !onRunning.OpenData {
		t.Fatalf("a running world lost a reading action: %+v", onRunning)
	}
	// The window setting is a SETTING, so it can be changed whichever way the
	// world is: the core writes it and it takes effect at the next start.
	if !onRunning.Headless {
		t.Fatal("a running world cannot be told how to run next time")
	}

	onStopped := ActionsFor(&stopped, snap, false)
	if !onStopped.Start || !onStopped.Delete || !onStopped.SetDefault {
		t.Fatalf("a stopped world is missing an action: %+v", onStopped)
	}
	if onStopped.Stop {
		t.Fatal("a stopped world offered Stop")
	}

	// A world with no identity yet cannot have one copied.
	noID := stopped
	noID.PeerID = ""
	if ActionsFor(&noID, snap, false).CopyPeerID {
		t.Fatal("a world with no identity offered to copy it")
	}

	// Nothing selected: an installation with no worlds at all can still make one.
	empty := ActionsFor(nil, launcher.Snapshot{}, false)
	if !empty.Create {
		t.Fatal("an empty installation cannot create a world")
	}
	if empty.Start || empty.Stop || empty.Delete || empty.StopAll || empty.Headless {
		t.Fatalf("an empty installation offered an action: %+v", empty)
	}

	// While an action runs, nothing else can be pressed: the core's per-world
	// lock would refuse it and a grey button says so better than a refusal.
	busy := ActionsFor(&stopped, snap, true)
	if busy != (Actions{}) {
		t.Fatalf("a busy window still offered actions: %+v", busy)
	}
}

// Every caption the harness clicks by is unique: two DIFFERENT actions with one
// caption is an ambiguous click. (The same action may wear its caption on a
// button, a menu item and a context menu item — that is the point of keying them
// by caption.)
func TestCaptionsAreUniqueAndPlain(t *testing.T) {
	captions := []string{
		ButtonStart, ButtonStop, ButtonCreate, ButtonStopAll, ButtonSetDefault,
		ButtonEdit, ButtonClone, ButtonDelete, ButtonDiagnose,
		ButtonOpenData, ButtonOpenLogs, ButtonOpenGameLog, ButtonOpenConsole,
		ButtonCopyPeerID, ButtonCopyLog, ButtonRefresh, ButtonQuit, ButtonDocs, ButtonAbout,
		ButtonGetUpdate,
		CheckHeadless, ButtonShowDetails, ButtonHideDetails,
		ButtonDialogSave, ButtonDialogCreate, ButtonDialogClone, ButtonDialogDelete,
		ButtonDialogCancel, ButtonShowAdvanced, ButtonHideAdvanced, CheckRemoveWorldData,
	}
	seen := make(map[string]bool, len(captions))
	for _, caption := range captions {
		if caption == "" {
			t.Fatal("a control has no caption")
		}
		if seen[caption] {
			t.Fatalf("two controls are captioned %q", caption)
		}
		seen[caption] = true
	}

	// PLAIN WORDS. Nothing a person reads on a control names a thing only this
	// project has. Those words are all still in the window — in the details pane,
	// which is where the program's own output goes.
	for _, caption := range captions {
		mustBePlain(t, "the caption", caption)
	}

	// Every tooltip says something the caption does not, because a tooltip that
	// repeats its caption has cost a person a hover for nothing.
	tips := map[string]string{
		ButtonStart: StartTip, ButtonStop: StopTip, ButtonCreate: CreateTip,
		ButtonStopAll: StopAllTip, ButtonSetDefault: SetDefaultTip, ButtonEdit: EditTip,
		ButtonClone: CloneTip, ButtonDelete: DeleteTip, ButtonDiagnose: DiagnoseTip,
		ButtonOpenData: OpenDataTip, ButtonOpenLogs: OpenLogsTip,
		ButtonOpenGameLog: OpenGameLogTip, ButtonOpenConsole: OpenConsoleTip,
		ButtonCopyPeerID: CopyPeerIDTip, ButtonCopyLog: CopyLogTip,
		CheckHeadless: HeadlessTip, ButtonShowDetails: DetailsTip, ButtonRefresh: RefreshTip,
		ButtonGetUpdate: GetUpdateTip,
	}
	for caption, tip := range tips {
		if len(tip) <= len(caption) {
			t.Fatalf("the tooltip for %q is %q, which says no more than the caption does", caption, tip)
		}
		// A tooltip is read by the same person as the caption above it.
		mustBePlain(t, "the tooltip for "+caption, tip)
	}
}

// THE UPDATE LINE IS ITS OWN LINE, AND IT IS NOT THE BANNER. The banner is red
// and means something is wrong with this installation; an available version is
// neither wrong nor urgent, and putting the two in one place would teach a
// participant to ignore the one that is about a fault. The words themselves come
// from the core, so the console menu and this window cannot come to say it
// differently.
func TestTheUpdateLineIsSeparateFromTheBanner(t *testing.T) {
	healthy := launcher.Snapshot{Worlds: []launcher.WorldView{aWorld()}}
	if got := UpdateNoticeFor(healthy); got != "" {
		t.Fatalf("an installation with nothing to say carries %q", got)
	}

	withUpdate := healthy
	withUpdate.NewerRelease = "99.0.0"
	notice := UpdateNoticeFor(withUpdate)
	if notice != launcher.UpdateNotice("99.0.0") {
		t.Fatalf("the window words it as %q and the core as %q",
			notice, launcher.UpdateNotice("99.0.0"))
	}
	for _, want := range []string{"99.0.0", launcher.Release, DocsURL} {
		if !strings.Contains(notice, want) {
			t.Errorf("the notice %q does not name %q", notice, want)
		}
	}
	// The banner has no opinion about it either way: a healthy installation with
	// an update available still has nothing wrong with it.
	if got := BannerFor(withUpdate); got != "" {
		t.Fatalf("an available version raised the fault banner: %q", got)
	}
	// And a broken installation still says what is broken, update or no update.
	broken := withUpdate
	broken.Problems = []string{"profiles\\second.json: unexpected end of JSON input"}
	if got := BannerFor(broken); got == "" {
		t.Fatal("an update silenced the fault banner")
	}
	if got := UpdateNoticeFor(broken); got == "" {
		t.Fatal("a fault silenced the update line")
	}
}

// The button opens THIS PROGRAM'S OWN address, and it is the same one the
// documentation item opens, because the homepage is the download page.
func TestTheUpdateButtonOpensThisProjectsOwnAddress(t *testing.T) {
	if DocsURL != launcher.HomeURL {
		t.Fatalf("the window opens %q and the core names %q", DocsURL, launcher.HomeURL)
	}
	if !strings.HasPrefix(DocsURL, "https://") {
		t.Fatalf("the window opens %q, which is not https", DocsURL)
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
	// And the panel's own checkbox sends exactly the flag an edit would.
	if HeadlessFlag(true) != "--headless" || HeadlessFlag(false) != "--no-headless" {
		t.Fatalf("the checkbox sends %q and %q", HeadlessFlag(true), HeadlessFlag(false))
	}

	form = FormFor(before)
	form.SaveOnQuit = false
	if strings.Join(EditFlags(before, form), " ") != "--save-on-quit off" {
		t.Fatalf("clearing save-on-quit produced %v", EditFlags(before, form))
	}

	// EMPTYING A PUBLISHED NAME IS TAKING IT OFF THE MAP, and the core spells
	// that as '-' because a blank answer at its own prompts means "keep the one
	// on offer". A window that sent an empty value would be asking the core to
	// guess which of the two a cleared box meant.
	named := before
	named.Keeper = "Alice"
	named.WorldName = "Alice's world"
	form = FormFor(named)
	form.Keeper = "  "
	if got := strings.Join(EditFlags(named, form), " "); got != "--keeper "+PublishNone {
		t.Fatalf("clearing the keeper produced %q", got)
	}
	form = FormFor(named)
	form.WorldName = "The Deep"
	if got := strings.Join(EditFlags(named, form), " "); got != "--world-name The Deep" {
		t.Fatalf("renaming the world produced %q", got)
	}
	if flags := EditFlags(named, FormFor(named)); len(flags) != 0 {
		t.Fatalf("an unchanged named world produced %v", flags)
	}

	// A SETTING IS RENDERED EXACTLY. Three significant figures would show the
	// longest save interval the core accepts as 1.44e+03 in a field somebody is
	// about to edit, and a measurement's rounding has no business there.
	long := before
	long.SaveMinutes = 1440
	if got := FormFor(long).SaveMinutes; got != "1440" {
		t.Fatalf("a 1440-minute interval renders as %q", got)
	}
	precise := before
	precise.SaveMinutes = 12.345
	if got := FormFor(precise).SaveMinutes; got != "12.345" {
		t.Fatalf("a 12.345-minute interval renders as %q", got)
	}
	// And an untouched form of either still asks for no change.
	for _, p := range []launcher.Profile{long, precise} {
		if flags := EditFlags(p, FormFor(p)); len(flags) != 0 {
			t.Fatalf("an unchanged form of a %g-minute world produced %v", p.SaveMinutes, flags)
		}
	}
}

// A person editing settings they did not choose cannot otherwise tell which of
// them are decisions and which are simply what the packaged default happens to
// be.
func TestDefaultNoteSaysWhichValuesAreThePackagedOnes(t *testing.T) {
	if got := DefaultNote("E,N,W,S", launcher.DefaultExportEdges); !strings.Contains(got, "the default") {
		t.Fatalf("an untouched default reads %q", got)
	}
	got := DefaultNote("E", launcher.DefaultExportEdges)
	if !strings.Contains(got, launcher.DefaultExportEdges) {
		t.Fatalf("a changed value does not name the default: %q", got)
	}
}

// The name a create dialog opens on has to be one the core would accept, and it
// has to be chosen BEFORE the defaults that are derived from it.
func TestNextFreeWorldName(t *testing.T) {
	if got := NextFreeWorldName(launcher.Snapshot{}); got != "world2" {
		t.Fatalf("an empty installation is offered %q", got)
	}
	second := aWorld()
	second.Name = "world2"
	third := aWorld()
	third.Name = "WORLD3"
	snap := launcher.Snapshot{Worlds: []launcher.WorldView{aWorld(), second, third}}
	// Names are compared without case, because the file system the profile lands
	// on is.
	if got := NextFreeWorldName(snap); got != "world4" {
		t.Fatalf("an installation holding world2 and WORLD3 is offered %q", got)
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

// mustBePlain is the wording rule, applied to one string a participant reads.
//
// The words it bans are this program's own names for its parts, plus a path into
// the repository it was built from. Every one of them is still in the window —
// in the details pane, which is the core's own output and is deliberately left
// in the core's own vocabulary.
func mustBePlain(t *testing.T, what, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, word := range InternalWords() {
		if strings.Contains(lower, word) {
			t.Fatalf("%s %q uses the internal word %q", what, text, word)
		}
	}
}

// THE DIALOGS ARE PRIMARY UI TOO, which the first round forgot: the create
// dialog told a participant that "the map applies a per-address enrollment
// limit", the delete dialog that "your sidecar may still be holding organisms it
// took custody of", and both pointed at docs/participant/leave.md — a file on a
// stranger's disk that nobody who installed a game has ever seen.
func TestDialogProseIsPlainAndPointsAtTheWebsite(t *testing.T) {
	prose := DialogProse()
	if len(prose) < 20 {
		t.Fatalf("only %d dialog sentences are covered; a new one that is not on the list is one nothing reads", len(prose))
	}
	for _, text := range prose {
		if strings.TrimSpace(text) == "" {
			t.Fatal("a dialog sentence is empty")
		}
		mustBePlain(t, "the dialog text", text)
		// No repository path reaches a participant. InternalWords covers "docs/"
		// and ".md"; this covers the separator the other way round.
		if strings.Contains(text, `docs\`) {
			t.Fatalf("a dialog names a file in this repository: %q", text)
		}
	}

	// The two warnings that MUST survive being reworded, because they are the
	// two things a person cannot undo by clicking again.
	if !strings.Contains(ProseAnotherIdentity, "limits how many worlds") {
		t.Fatalf("the create dialog no longer explains the map's limit: %q", ProseAnotherIdentity)
	}
	for _, note := range []string{ProseDeletingIsNotLeaving, ProseCustody} {
		if !strings.Contains(note, DocsURL) {
			t.Fatalf("a leaving note points nowhere a participant can go: %q", note)
		}
		if !strings.Contains(strings.ToLower(note), "not take it off the map") {
			t.Fatalf("a leaving note no longer says deleting is not leaving: %q", note)
		}
	}
	// The custody warning's whole point is that only this computer can pass on
	// what it is holding, and that starting the world once is the fix.
	for _, phrase := range []string{"only this computer", "Start it once"} {
		if !strings.Contains(ProseCustody, phrase) {
			t.Fatalf("the delete dialog lost %q: %s", phrase, ProseCustody)
		}
	}
}

// THE RESULT LINE IS ONE WORLD'S, which a machine caught it not being: a health
// check on multi-2 left "The health check found no faults" in the panel, and
// selecting default then showed default's headline, "Stopped", stacked on top of
// multi-2's result as though the two belonged together.
func TestAResultBelongsToTheWorldItIsAbout(t *testing.T) {
	first := aWorld()
	second := stoppedNamed("multi-2")
	snap := launcher.Snapshot{Worlds: []launcher.WorldView{first, second}}
	names := []string{"default", "multi-2"}

	results := NewResultLog()
	check := Job{Kind: JobCheck, World: "multi-2"}
	results.Record(check.ResultAbout(), check.Result(0, ""), names)

	if got := PanelFor(&second, snap, Busy{}, results).Result.Text; !strings.Contains(got, "health check") {
		t.Fatalf("the world that was checked shows %q", got)
	}
	if got := PanelFor(&first, snap, Busy{}, results).Result.Text; got != "" {
		t.Fatalf("a world nothing happened to shows %q", got)
	}

	// A second world's own action does not disturb the first's.
	start := Job{Kind: JobStart, World: "default"}
	results.Record(start.ResultAbout(), start.Result(0, ""), names)
	if got := PanelFor(&second, snap, Busy{}, results).Result.Text; !strings.Contains(got, "health check") {
		t.Fatalf("multi-2's result was lost when default was started: %q", got)
	}
	if got := PanelFor(&first, snap, Busy{}, results).Result.Text; got != "Started 'default'." {
		t.Fatalf("default shows %q", got)
	}

	// The NEXT action about a world clears its line while it runs, so nothing
	// claims the last outcome while the next one is happening.
	results.Record(start.ResultAbout(), Result{}, names)
	if got := PanelFor(&first, snap, Busy{}, results).Result.Text; got != "" {
		t.Fatalf("a running action left the previous result up: %q", got)
	}

	// And nothing is shown at all while an action is running, whichever world it
	// is about.
	busy := Job{Kind: JobStart, World: "default"}.Busy()
	if got := PanelFor(&second, snap, busy, results).Result; got.Text != "" {
		t.Fatalf("a result was shown during an action: %q", got.Text)
	}
}

// 'Stop every world' is about every world there is, so its result belongs beside
// all of them.
func TestAGlobalActionsResultIsShownOnEveryWorld(t *testing.T) {
	names := []string{"default", "multi-2"}
	results := NewResultLog()
	job := Job{Kind: JobStopAll}
	results.Record(job.ResultAbout(), job.Result(0, ""), names)
	for _, name := range names {
		if got := results.For(name).Text; got != "Stopped every world." {
			t.Fatalf("%s shows %q", name, got)
		}
	}
}

// A result about a world that is NOT in the list — a create that was refused, a
// delete that succeeded — has no row to live beside, and it is the one a person
// most needs to read. It is shown whatever is selected, until the next action.
func TestAResultWithNoRowIsShownAnyway(t *testing.T) {
	names := []string{"default"}
	results := NewResultLog()
	create := Job{Kind: JobCreate, World: "world2"}
	results.Record(create.ResultAbout(), create.Result(1, "the sidecar port 70000 is outside 1024-65535"), names)

	got := results.For("default")
	if !strings.Contains(got.Text, "70000") || got.Good {
		t.Fatalf("a refused create shows %+v", got)
	}
	// It is the LOOSE one, and the NEWER one wins: a world's own result taken
	// before it does not hide it, and one taken after it does.
	start := Job{Kind: JobStart, World: "default"}
	results.Record(start.ResultAbout(), start.Result(0, ""), names)
	if got := results.For("default").Text; got != "Started 'default'." {
		t.Fatalf("a newer result about the world itself was not preferred: %q", got)
	}

	// A delete names a world that is on its way out of the list, so it goes to
	// the same place.
	del := Job{Kind: JobDelete, World: "default"}
	if about := del.ResultAbout(); len(about.Worlds) != 0 || about.Every {
		t.Fatalf("a delete's result was pinned to a row that is about to go: %+v", about)
	}

	// A world that leaves the list takes its result with it, so a world made
	// again under a name somebody has used before opens clean.
	results.Forget("default")
	if got := results.For("default").Text; got != "" {
		t.Fatalf("a forgotten world still shows %q", got)
	}
}

// The status bar counted "2 world(s)", which is a plural rule left for the
// reader to apply.
func TestTheStatusBarCountsInEnglish(t *testing.T) {
	if got := WorldCountWords(1); got != "1 world" {
		t.Fatalf("one world reads %q", got)
	}
	if got := WorldCountWords(2); got != "2 worlds" {
		t.Fatalf("two worlds read %q", got)
	}
	if got := WorldCountWords(0); got != "0 worlds" {
		t.Fatalf("no worlds read %q", got)
	}
	line := StatusBarText(launcher.Snapshot{
		Worlds:      []launcher.WorldView{aWorld()},
		InstallRoot: `C:\Program Files\Bibites Multiverse`,
	})
	if strings.Contains(line, "(s)") {
		t.Fatalf("the status bar still hedges its plural: %q", line)
	}
	if !strings.Contains(line, CloseHint) || !strings.Contains(line, "1 world") {
		t.Fatalf("the status bar reads %q", line)
	}
}

// Every colour is drawn by name in one place in the window, and that place is
// built from this list. A colour missing from it would be a state drawn in
// whatever the last case happened to set.
func TestEveryColourIsAccountedFor(t *testing.T) {
	all := Colours()
	seen := make(map[Colour]bool, len(all))
	for _, colour := range all {
		if seen[colour] {
			t.Fatalf("colour %d is listed twice", colour)
		}
		seen[colour] = true
	}
	// Every state StatusFor can produce uses one of them.
	worlds := []launcher.WorldView{aWorld(), stoppedWorld()}
	orphan := aWorld()
	orphan.Mod = launcher.ModView{Answered: true}
	worlds = append(worlds, orphan)
	for _, world := range worlds {
		if !seen[StatusFor(world, Busy{}).Colour] {
			t.Fatalf("a state is drawn in a colour that is not in Colours(): %+v", StatusFor(world, Busy{}))
		}
	}
	if !seen[StatusFor(stoppedWorld(), Busy{World: "default", Short: "Starting..."}).Colour] {
		t.Fatal("the working state is drawn in a colour that is not in Colours()")
	}
}

// THE BUG A MACHINE FOUND, EXACTLY: 'Stop every world' writes a line against
// EVERY world, so the next refusal — which has no row of its own and lands in
// the loose slot — was newer, was correct, was the only thing the person needed,
// and was invisible behind "Stopped every world." on every row in the list.
//
// The panel showed nothing at all about a create that had just been refused, and
// went on saying the last thing that had gone right.
func TestAFreshRefusalIsNeverHiddenByAnOlderSuccess(t *testing.T) {
	first := aWorld()
	second := stoppedNamed("multi-2")
	snap := launcher.Snapshot{Worlds: []launcher.WorldView{first, second}}
	names := []string{"default", "multi-2"}
	results := NewResultLog()

	// Everything is fine, and every world says so.
	all := Job{Kind: JobStopAll}
	results.Record(all.ResultAbout(), all.Result(0, ""), names)
	for _, name := range names {
		if results.For(name).Text != "Stopped every world." {
			t.Fatalf("%s did not take the global result", name)
		}
	}

	// Now a create is refused. The world it names will never exist, so its result
	// has no row — and it must still be the line every world shows, because it is
	// what just happened.
	create := Job{Kind: JobCreate, World: "world2"}
	results.Record(create.ResultAbout(), Result{}, names) // the job starts
	results.Record(create.ResultAbout(),
		create.Result(1, "the world 'default' already uses port 8787. Every world needs its own"), names)

	want := "'world2' was not created: the world 'default' already uses port 8787. Every world needs its own"
	for _, name := range names {
		got := results.For(name)
		if got.Text != want {
			t.Fatalf("with %s selected the panel would say %q, want %q", name, got.Text, want)
		}
		if got.Good {
			t.Fatalf("with %s selected the refusal would be drawn green", name)
		}
	}
	// And through the panel, which is what is actually on screen.
	for _, world := range []launcher.WorldView{first, second} {
		panel := PanelFor(&world, snap, Busy{}, results)
		if panel.Result.Text != want || panel.Result.Good {
			t.Fatalf("the panel for %s shows %+v", world.Name, panel.Result)
		}
	}

	// The same for a delete that was refused: the world is still in the list, but
	// the result is about an act on the installation and goes to the same place.
	del := Job{Kind: JobDelete, World: "multi-2"}
	results.Record(del.ResultAbout(), Result{}, names)
	results.Record(del.ResultAbout(),
		del.Result(1, "that is not 'multi-2'. Nothing was deleted"), names)
	want = "'multi-2' was not deleted: that is not 'multi-2'. Nothing was deleted"
	for _, name := range names {
		if got := results.For(name).Text; got != want {
			t.Fatalf("with %s selected the panel would say %q, want %q", name, got, want)
		}
	}

	// A world's OWN next action takes its line back, because that is newer again.
	start := Job{Kind: JobStart, World: "default"}
	results.Record(start.ResultAbout(), start.Result(0, ""), names)
	if got := results.For("default").Text; got != "Started 'default'." {
		t.Fatalf("default shows %q", got)
	}
	// And multi-2 goes QUIET rather than back to "Stopped every world.". That
	// line was overtaken two actions ago; resurrecting it would be a lie about
	// the order things happened in, which is the one thing this log is for.
	if got := results.For("multi-2").Text; got != "" {
		t.Fatalf("multi-2 resurrected an overtaken line: %q", got)
	}
}

// Starting anything clears the loose slot, so a result belonging to the last
// action cannot outlive it.
func TestStartingAnActionDropsTheLooseResult(t *testing.T) {
	names := []string{"default"}
	results := NewResultLog()
	create := Job{Kind: JobCreate, World: "world2"}
	results.Record(create.ResultAbout(), create.Result(1, "no"), names)
	if results.For("default").Text == "" {
		t.Fatal("a refusal was not recorded at all")
	}
	// Any other job starting wipes it, whatever that job is about.
	start := Job{Kind: JobStart, World: "default"}
	results.Record(start.ResultAbout(), Result{}, names)
	if got := results.For("default").Text; got != "" {
		t.Fatalf("the last action's line outlived it: %q", got)
	}
}

// THE QUOTE WINS, and it is the one deliberate exception to the plain-words
// rule. A failure's second half is the core speaking, passed through exactly as
// it was printed; a launcher that paraphrased its own refusals would be one
// whose window and whose log disagreed about what went wrong, and the log is
// what gets pasted into a bug report.
//
// So this asserts the OPPOSITE of mustBePlain for that half: whatever the core
// said arrives unedited, internal words and all.
func TestAQuotedRefusalIsNeverEditedToObeyTheWordingRule(t *testing.T) {
	job := Job{Kind: JobDelete, World: "multi-2"}
	// A sentence with an internal word in it that the core could not avoid.
	said := "the sidecar for 'multi-2' is running (pid 4242). Stop this world first"
	got := job.Result(1, said).Text
	if !strings.HasSuffix(got, said) {
		t.Fatalf("the core's sentence was edited on its way to the panel: %q", got)
	}
	// The half the WINDOW wrote still obeys the rule.
	mustBePlain(t, "the window's half of a failure", job.failedHead())
	for _, kind := range []JobKind{JobStart, JobStop, JobStopAll, JobSetDefault, JobEdit,
		JobHeadless, JobCreate, JobClone, JobDelete, JobCheck} {
		one := Job{Kind: kind, World: "default", Other: "world2"}
		mustBePlain(t, "a job's own words", one.failedHead())
		mustBePlain(t, "a job's own words", one.succeeded())
		mustBePlain(t, "a job's own words", one.Progress())
	}
}
