package launchergui

// THE DETAILS PANE, THE SPINNING BAR AND THE RESULT LINE — which are three views
// of ONE stream of text: the core's own output.
//
// The window starts an action and then has nothing to say about it for up to two
// minutes. The core, meanwhile, says a great deal: it names the process it
// started, the wait it is in, the map's answer, and the one thing that decides
// whether a world is really on the map. All of that used to land in a raw log
// pane taking half the window, where it was the only feedback there was — so a
// person either read a terminal or watched a grey button.
//
// So the same lines are read three ways:
//
//	THE DETAILS PANE keeps every line, timestamped, and is collapsed until it is
//	wanted. It is the whole truth and it is never edited.
//	THE PROGRESS PHRASE is one line of plain words, replaced as the core reaches
//	each stage (ProgressFor). It is what the panel shows while the bar spins.
//	THE ALARM opens the pane by itself (IsAlarm). A window that stayed silent and
//	closed while the core printed a warning would be worse than no window.
//
// Nothing here parses a VALUE out of the core's output — no pid, no port, no
// verdict. Values come from launcher.Session.Snapshot, which reads the machine.
// These are captions for a stage that has been reached, and a line this file
// fails to recognise costs a phrase, never a fact.

import (
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------- the jobs

// JobKind is one thing this window can be asked to do.
type JobKind int

// The jobs. Every one of them is a call into launcher.Session and takes long
// enough to need saying out loud.
const (
	JobStart JobKind = iota
	JobStop
	JobStopAll
	JobSetDefault
	JobEdit
	JobHeadless
	JobCreate
	JobClone
	JobDelete
	JobCheck
)

// Job is one queued action, and the source of every word said about it: what the
// list shows while it runs, what the panel says it is doing, what goes in the
// details pane as its heading, and what is left on screen when it is over.
type Job struct {
	Kind JobKind
	// World is the world it is about. JobStopAll has none.
	World string
	// Other is the second name a job needs: a clone's new world.
	Other string
	// Headless is the value JobHeadless is setting.
	Headless bool
	// Flags is what an edit is sending, and it appears in the details heading so
	// that the pane records what was asked for as well as what came back.
	Flags []string
}

// Busy is what the list and the panel show while this job runs.
func (j Job) Busy() Busy {
	busy := Busy{World: j.World, Short: j.short(), Phrase: j.Progress()}
	if j.Kind == JobStopAll {
		busy.All = true
	}
	return busy
}

func (j Job) short() string {
	switch j.Kind {
	case JobStart:
		return "Starting..."
	case JobStop, JobStopAll:
		return "Stopping..."
	case JobCreate:
		return "Creating..."
	case JobClone:
		return "Copying..."
	case JobDelete:
		return "Deleting..."
	case JobCheck:
		return "Checking..."
	}
	return "Saving..."
}

// Progress is the FIRST phrase, said the moment the button is pressed. The
// phrases after it come from the core's own output, through ProgressFor.
func (j Job) Progress() string {
	switch j.Kind {
	case JobStart:
		return "Starting '" + j.World + "'..."
	case JobStop:
		return "Stopping '" + j.World + "' - asking it to save and quit..."
	case JobStopAll:
		return "Stopping every world - each one saves first..."
	case JobSetDefault:
		return "Making '" + j.World + "' the default world..."
	case JobEdit:
		return "Saving the settings for '" + j.World + "'..."
	case JobHeadless:
		if j.Headless {
			return "Setting '" + j.World + "' to run without a game window..."
		}
		return "Setting '" + j.World + "' to run with a game window..."
	case JobCreate:
		return "Creating '" + j.World + "' - asking the map for a new identity..."
	case JobClone:
		return "Copying '" + j.World + "' to '" + j.Other + "'..."
	case JobDelete:
		return "Deleting '" + j.World + "'..."
	case JobCheck:
		return "Checking '" + j.World + "'..."
	}
	return "Working..."
}

// Heading is the line written into the details pane before the core says
// anything, so the pane reads as a session rather than as a stream of
// unattributed output. IT IS THE PROGRAM'S OWN VOCABULARY, deliberately: this is
// the one part of the window where the internal names are the useful ones,
// because it is what somebody pastes into a bug report.
func (j Job) Heading() string {
	switch j.Kind {
	case JobStart:
		return "start " + j.World
	case JobStop:
		return "stop " + j.World
	case JobStopAll:
		return "stop every world"
	case JobSetDefault:
		return "make " + j.World + " the default world"
	case JobEdit:
		return "edit " + j.World + " (" + strings.Join(j.Flags, " ") + ")"
	case JobHeadless:
		return "edit " + j.World + " (" + HeadlessFlag(j.Headless) + ")"
	case JobCreate:
		return "create " + j.World + " and enroll a new identity on the map"
	case JobClone:
		return "clone " + j.World + " as " + j.Other
	case JobDelete:
		return "delete " + j.World
	case JobCheck:
		return "check " + j.World
	}
	return "work"
}

// Result is the one line left on screen when a job is over, until the next one
// starts. It stays because a person who looked away for the ninety seconds a
// start takes has otherwise no way to learn how it went except by reading a log.
type Result struct {
	Text string
	// Good is whether it is drawn green or red.
	Good bool
}

// Result is what a job's exit code means, in plain words.
//
// THE CODE IS THE CORE'S, AND SO ARE THE WORDS. Every Session action answers
// with the same code the command line would have, and every refusal in
// internal/launcher already says what was wrong and what to do about it — "the
// sidecar port 70000 is outside 1024-65535", "another launcher is starting or
// stopping this world". A window that replaced those with "the operation failed"
// would be throwing away the best sentence in the program. So `said` is the last
// thing the core said before it gave up (see SaidLine), and it is what a failure
// shows.
// THE QUOTE IS NEVER EDITED, and that is the one deliberate exception to this
// window's plain-words rule. Everything a person reads here is written in words
// that name nothing only this project has — except the half of a failure that is
// the core speaking, which is passed through exactly as it was printed. A
// launcher that paraphrased its own refusals would be a launcher whose window and
// whose log disagreed about what went wrong, and the log is what gets pasted into
// a bug report. Where a core message can say "world" instead of an internal name
// it now does (validatePort in internal/launcher/validate.go); where it cannot,
// the sentence still wins.
func (j Job) Result(code int, said string) Result {
	if code == 0 {
		return Result{Text: j.succeeded(), Good: true}
	}
	// A health check's report is the whole output, not one line of it, and its
	// last line is only the exit status. It keeps its own sentence.
	if said != "" && j.Kind != JobCheck {
		return Result{Text: j.failedHead() + ": " + said, Good: false}
	}
	return Result{Text: j.failed(), Good: false}
}

// FailureDetailLimit bounds what a panel will quote. A refusal is a sentence; a
// paragraph is what the details pane is for.
const FailureDetailLimit = 220

// SaidLine decides whether one line of the core's output is usable as the last
// word of a failure.
//
// IT TAKES ONLY UNINDENTED LINES, and that is the whole trick: the core prints
// its refusals flush left and every continuation — the tail of a log, the five
// usual causes, the remedy — indented under them (a.warn("  %s", ...)). So the
// last flush-left line before a non-zero exit is the refusal itself rather than
// the fourth line of a list explaining it.
//
// WHERE IT MUST BE CALLED, which is the whole of a bug a machine found: on the
// goroutine that WRITES, as the line is written, and never off the pane. The
// window's first attempt read it while draining the pane's own hundred-
// millisecond batch, and a refusal is printed and returned from in microseconds
// — so the action had already finished, and the panel showed "'world2' was not
// created. The details below say why." while the core's own sentence, "the
// profile 'default' already uses port 8787. Every world needs its own", sat one
// line further down in a pane nobody had opened.
func SaidLine(line string) (string, bool) {
	body := TrimStamp(line)
	if body == "" || body != strings.TrimLeft(body, " \t") {
		return "", false
	}
	if strings.HasPrefix(body, ">") || len(body) > FailureDetailLimit {
		return "", false
	}
	return body, true
}

func (j Job) succeeded() string {
	switch j.Kind {
	case JobStart:
		return "Started '" + j.World + "'."
	case JobStop:
		return "Stopped '" + j.World + "'."
	case JobStopAll:
		return "Stopped every world."
	case JobSetDefault:
		return "'" + j.World + "' is now the default world."
	case JobEdit:
		return "Saved the settings for '" + j.World + "'."
	case JobHeadless:
		if j.Headless {
			return "'" + j.World + "' will run without a game window from now on."
		}
		return "'" + j.World + "' will run with a game window from now on."
	case JobCreate:
		return "Created '" + j.World + "'. It has its own identity on the map."
	case JobClone:
		return "Copied '" + j.World + "' to '" + j.Other + "'."
	case JobDelete:
		return "Deleted '" + j.World + "'."
	case JobCheck:
		return "The health check found no faults. Its whole report is in the details."
	}
	return "Done."
}

func (j Job) failed() string {
	if j.Kind == JobCheck {
		return "The health check found a fault. Its whole report is in the details."
	}
	return j.failedHead() + ". The details below say why."
}

// failedHead is the job half of a failure — what was being attempted — with no
// full stop, because either the core's own sentence or a pointer at the details
// follows it.
func (j Job) failedHead() string {
	switch j.Kind {
	case JobStart:
		return "Could not start '" + j.World + "'"
	case JobStop:
		return "Could not stop '" + j.World + "'"
	case JobStopAll:
		return "At least one world could not be stopped"
	case JobSetDefault:
		return "Could not make '" + j.World + "' the default world"
	case JobEdit:
		return "Could not save the settings for '" + j.World + "'"
	case JobHeadless:
		return "Could not change how '" + j.World + "' runs"
	case JobCreate:
		return "'" + j.World + "' was not created"
	case JobClone:
		return "'" + j.Other + "' was not created"
	case JobDelete:
		return "'" + j.World + "' was not deleted"
	case JobCheck:
		return "The health check found a fault"
	}
	return "That did not work"
}

// ---------------------------------------------------------------- where a result goes

// ResultAbout says WHICH WORLD a result belongs beside.
//
// IT MATTERS BECAUSE ONE LINE WAS SHARED BY EVERY WORLD. A health check on
// multi-2 left "The health check found no faults" in the panel, and selecting
// default then showed default's own headline, "Stopped", with multi-2's result
// underneath it — two worlds' facts stacked as though they were one world's.
type ResultAbout struct {
	// Worlds is the world names the result belongs to.
	Worlds []string
	// Every is a job about every world at once, whichever worlds those turn out
	// to be when it finishes.
	Every bool
}

// ResultAbout is where this job's result belongs.
func (j Job) ResultAbout() ResultAbout {
	switch j.Kind {
	case JobStopAll:
		return ResultAbout{Every: true}
	case JobCreate, JobClone, JobDelete:
		// THE LIST IS ABOUT TO CHANGE UNDER IT. A created world is not in the
		// reading this result is recorded against, and a deleted one is about to
		// leave it — and a create that was REFUSED names a world that will never
		// exist, which is the result a person most needs to read. None of them
		// has a row to live beside, so they are shown whatever is selected until
		// the next action. See ResultLog.
		return ResultAbout{}
	}
	return ResultAbout{Worlds: []string{j.World}}
}

// ResultLog is the last thing that happened to each world, and it is the whole
// of what the panel's coloured line is drawn from.
//
// A result is REPLACED by something NEWER and by nothing else, so a world that
// has been sitting there for ten minutes still says how it got there.
//
// NEWER IS THE WHOLE RULE, and getting that wrong is a bug a machine found. The
// first version preferred a world's own result over the loose one whatever their
// ages, on the reasoning that a world's own line is more specific. Then 'stop
// every world' wrote a line against EVERY world — so when the next create was
// refused, its loose refusal was newer, was correct, was the only thing the
// person needed, and was invisible behind "Stopped every world." on every row in
// the list. Specificity is not recency, and it is recency a person is reading.
type ResultLog struct {
	byWorld map[string]stamped
	// loose is a result with no row to live beside — see ResultAbout.
	loose stamped
	// at counts recordings. It is not a clock: nothing here needs to know when
	// something happened, only what happened last.
	at int
}

// stamped is one result and how recent it is.
type stamped struct {
	result Result
	at     int
}

// NewResultLog makes an empty one.
func NewResultLog() *ResultLog {
	return &ResultLog{byWorld: make(map[string]stamped)}
}

// Record writes one result down. A zero Result CLEARS instead, which is what
// starting an action does: no line may go on claiming the last thing that
// happened while the next thing is happening.
//
// names is the worlds that exist right now, and it decides whether a result has
// a row to live beside at all.
func (r *ResultLog) Record(about ResultAbout, result Result, names []string) {
	if r.byWorld == nil {
		r.byWorld = make(map[string]stamped)
	}
	r.at++
	// THE LOOSE SLOT IS ALWAYS CLEARED, whatever this recording is about: it
	// belongs to one action, and the moment another one starts it is history.
	r.loose = stamped{}
	targets := about.Worlds
	if about.Every {
		targets = names
	}
	matched := false
	for _, target := range targets {
		for _, name := range names {
			if strings.EqualFold(target, name) {
				matched = true
				if result.Text == "" {
					delete(r.byWorld, strings.ToLower(name))
				} else {
					r.byWorld[strings.ToLower(name)] = stamped{result: result, at: r.at}
				}
			}
		}
	}
	if !matched && result.Text != "" {
		// A LOOSE RESULT SUPERSEDES EVERY WORLD'S OWN LINE, and the older lines
		// are dropped rather than merely outranked. They are all older than this
		// by construction, so none of them can be shown while this is here — and
		// when this is cleared by the next action they must not come BACK, which
		// is what leaving them in place would do: a world would go from a refusal
		// straight back to a success it had already been overtaken by.
		r.byWorld = make(map[string]stamped)
		r.loose = stamped{result: result, at: r.at}
	}
}

// For is the line to show beside one world: whichever of that world's own result
// and the loose one is NEWER.
func (r *ResultLog) For(world string) Result {
	if r == nil {
		return Result{}
	}
	own := r.byWorld[strings.ToLower(world)]
	if r.loose.at > own.at {
		return r.loose.result
	}
	return own.result
}

// Forget drops a world's result, for a world that has left the list.
func (r *ResultLog) Forget(world string) {
	delete(r.byWorld, strings.ToLower(world))
}

// ---------------------------------------------------------------- the phrases

// stage is one recognised line of the core's output and the plain-words phrase
// it becomes.
type stage struct {
	// match is a fragment of the core's line. It is a FRAGMENT and not the whole
	// line because every one of these carries a pid, a path or a number in it.
	match  string
	phrase string
}

// stages is read in order, so a longer match goes above a shorter one it
// contains. The right-hand column is the whole of what a participant is shown
// while an action runs.
//
// KEEPING THIS IN STEP WITH THE CORE. A line that changes wording in
// internal/launcher costs a phrase here and nothing else: the pane still has the
// line, the bar still spins, and the result line still arrives. So these are
// matched loosely and on purpose — the alternative, a progress hook the core has
// to call, would put a second contract between two packages for the sake of a
// caption.
var stages = []stage{
	{"sidecar started (pid", "Connecting this world to the map..."},
	{"waiting for the map to grant this world a place", "Waiting for the map to give this world a place..."},
	{"YOU ARE ON THE MAP", "This world has its place on the map. Starting the game..."},
	{"game started (pid", "The game is starting..."},
	{"for the game's mod to reach the sidecar", "Waiting for the game to join the map (up to two minutes)..."},
	{"the game joined the map", "The game joined the map."},
	{"THE GAME STARTED BUT ITS MOD HAS NOT REACHED", "The game did not join the map."},
	{"The map did not grant a place", "The map did not give this world a place."},
	{"requesting a unique identity from", "Asking the map for a new identity..."},
	{"took the quit request", "The world is saving and shutting down..."},
	{"stopped the game (pid", "The game has stopped. Closing the link to the map..."},
	{"stopped the sidecar (pid", "Closing the link to the map..."},
	{"the game was not running", "The game was not running. Closing the link to the map..."},
}

// ProgressFor turns one line of the core's output into the phrase the panel
// shows, or answers false for a line that is not a stage.
func ProgressFor(line string) (string, bool) {
	body := TrimStamp(line)
	// 'stop --all' prints a header per world, and it is the only place the
	// window learns which of several worlds it has reached.
	if rest := strings.TrimPrefix(body, "--- "); rest != body && rest != "" {
		return "Stopping '" + rest + "'...", true
	}
	for _, s := range stages {
		if strings.Contains(body, s.match) {
			return s.phrase, true
		}
	}
	return "", false
}

// alarms are the lines that OPEN THE DETAILS PANE BY THEMSELVES.
//
// The rule is narrow on purpose. A pane that opened on any line containing
// "could not" would be open all the time, and a pane that is always open is the
// half-window log this design replaced. What is here is: the core's own alarm
// prefix, the two states in which a world is running and not on the map, the
// loss a forced headless stop causes, and the refusal a second launcher gets.
var alarms = []string{
	"!!",
	"The map did not grant a place",
	"LOCAL-HEADLESSSTOP",
	"NOT CONNECTED",
	"another launcher is starting or stopping this world",
}

// IsAlarm answers whether one line is reason enough to open the details pane
// without being asked.
func IsAlarm(line string) bool {
	body := TrimStamp(line)
	for _, alarm := range alarms {
		if strings.Contains(body, alarm) {
			return true
		}
	}
	return false
}

// TrimStamp removes the HH:MM:SS the pane's own lines carry, so that the two
// readers above see what the core actually wrote. A line without a stamp — which
// is what a test writes, and what a future caller might — is returned unchanged.
func TrimStamp(line string) string {
	const stamped = len("15:04:05  ")
	if len(line) < stamped {
		return line
	}
	if line[2] != ':' || line[5] != ':' || line[8] != ' ' || line[9] != ' ' {
		return line
	}
	for _, i := range []int{0, 1, 3, 4, 6, 7} {
		if line[i] < '0' || line[i] > '9' {
			return line
		}
	}
	return line[stamped:]
}

// ---------------------------------------------------------------- the log

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

// LinesToRestore is the other half of that answer, and it exists because
// DECIDING NOT TO FOLLOW WAS NEVER ENOUGH.
//
// A machine measured it on the real window: after scrolling to the top,
// LogFollowsTail correctly answered false (pos 0, page 15, max 299) — and the
// very next appended line still dragged the view from line 0 to line 286. The
// scroll was not this program's. walk's TextEdit.AppendText selects the end of
// the document and replaces the selection (EM_REPLACESEL), and an EDIT control
// scrolls the caret into view as part of that, before anything here is
// consulted. There is no flag to turn it off.
//
// So a pane that is not following is put BACK: the first visible line is read
// before the append and again after it, and the difference is handed to
// EM_LINESCROLL, whose vertical argument is a number of lines to scroll by. A
// negative answer scrolls up, which is the direction an append always moves the
// view.
//
// Both readings come from EM_GETFIRSTVISIBLELINE, which answers -1 for nothing
// at all; either being unreadable means the view cannot be restored honestly, so
// it is left where the control put it rather than moved by a guess.
func LinesToRestore(before, after int) int {
	if before < 0 || after < 0 {
		return 0
	}
	return before - after
}
