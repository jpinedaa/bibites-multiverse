package launchergui

import (
	"strings"
	"sync"
	"testing"
	"time"

	"multiverse/internal/launcher"
)

// THE PROGRESS PHRASES ARE READ OFF THE CORE'S OWN OUTPUT, so the lines below
// are the real ones, copied from internal/launcher with their real shapes: a
// pid, a path, a timestamp on the front where the pane put one.
func TestProgressFollowsAWholeStart(t *testing.T) {
	sequence := []struct {
		line string
		want string
	}{
		{"09:41:07  sidecar started (pid 4242) -> wss://bibitesmultiverse.com/contract-b/v4",
			"Connecting this world to the map..."},
		{"09:41:07  waiting for the map to grant this world a place ...",
			"Waiting for the map to give this world a place..."},
		{"09:41:09  YOU ARE ON THE MAP:", "This world has its place on the map. Starting the game..."},
		{"09:41:09  game started (pid 11004); it loads the world 'Multiverse' by itself, and seeds it on the first start.",
			"The game is starting..."},
		{"09:41:09  waiting up to 120 s for the game's mod to reach the sidecar...",
			"Waiting for the game to join the map (up to two minutes)..."},
		{"09:41:38  the game joined the map: mod connected, speed x10.", "The game joined the map."},
	}
	for _, step := range sequence {
		got, ok := ProgressFor(step.line)
		if !ok {
			t.Fatalf("no phrase for %q", step.line)
		}
		if got != step.want {
			t.Fatalf("%q became %q, want %q", step.line, got, step.want)
		}
	}

	// A whole stop, including the one line that says a save happened.
	stop := []struct {
		line     string
		contains string
	}{
		{"09:44:01  this world's mod took the quit request (queued); it is saving and shutting down.",
			"saving"},
		{"09:44:12  stopped the game (pid 11004) - it was asked through the mod, and it saved and quit",
			"The game has stopped"},
		{"09:44:13  stopped the sidecar (pid 4242) - ended directly; a sidecar keeps nothing unsaved",
			"link to the map"},
	}
	for _, step := range stop {
		got, ok := ProgressFor(step.line)
		if !ok || !strings.Contains(got, step.contains) {
			t.Fatalf("%q became %q (found %v)", step.line, got, ok)
		}
	}

	// 'stop every world' prints a header per world, and it is the only place the
	// window learns which of several it has reached.
	got, ok := ProgressFor("09:44:00  --- second")
	if !ok || !strings.Contains(got, "second") {
		t.Fatalf("the per-world header became %q (found %v)", got, ok)
	}

	// A PHRASE IS A CAPTION AND NEVER A FACT. A line this file does not know
	// costs a phrase and nothing else, so an unrecognised line is silent rather
	// than a guess.
	for _, line := range []string{
		// The window's own opening line, which names the release. It is written
		// with launcher.Release rather than a literal, because this repository
		// allows the release version to be spelled out only where
		// release/bump-version.sh says it may be.
		"09:41:07  Bibites Multiverse launcher " + launcher.Release + ", installed in C:\\Program Files",
		"",
		"---",
		"09:41:07  ",
	} {
		if phrase, ok := ProgressFor(line); ok {
			t.Fatalf("%q was read as progress: %q", line, phrase)
		}
	}
}

// A phrase is the same whether the line has been stamped for the pane or not,
// because one of the two readers sees it before the stamp and one after.
func TestTheTimestampIsNotPartOfTheLine(t *testing.T) {
	stamped, ok := ProgressFor("23:59:59  game started (pid 7)")
	bare, okBare := ProgressFor("game started (pid 7)")
	if !ok || !okBare || stamped != bare {
		t.Fatalf("stamped %q, bare %q", stamped, bare)
	}
	// Anything that is not a stamp is left alone, so a line that happens to
	// start with punctuation is not truncated.
	for _, line := range []string{"ab:cd:ef  x", "1:2:3  x", "short", "12:34:56 x"} {
		if TrimStamp(line) != line {
			t.Fatalf("%q was mistaken for a stamped line: %q", line, TrimStamp(line))
		}
	}
	if got := TrimStamp("09:41:07  hello"); got != "hello" {
		t.Fatalf("a stamped line trimmed to %q", got)
	}
}

// NEVER SILENT. These are the lines that open the details pane without being
// asked, and they are the whole list: a pane that opened on everything would be
// the half-window log this design replaced.
func TestTheAlarmsOpenThePane(t *testing.T) {
	alarming := []string{
		"09:41:07  !! THE GAME STARTED BUT ITS MOD HAS NOT REACHED THE SIDECAR after 120 s.",
		"09:41:07  The map did not grant a place.",
		"09:41:07    ('start --no-headless') and stop it normally. This is LOCAL-HEADLESSSTOP in",
		"09:41:07  another launcher is starting or stopping this world (…launcher.lock was taken 0s ago)",
	}
	for _, line := range alarming {
		if !IsAlarm(line) {
			t.Fatalf("this line did not open the pane: %q", line)
		}
	}
	calm := []string{
		"09:41:07  sidecar started (pid 4242) -> wss://bibitesmultiverse.com/contract-b/v4",
		"09:41:07  the game joined the map: mod connected, speed x10.",
		"09:41:07  wrote C:\\...\\profiles\\world2.json",
		"",
	}
	for _, line := range calm {
		if IsAlarm(line) {
			t.Fatalf("an ordinary line opened the pane: %q", line)
		}
	}
}

// A job says the same thing four ways, and none of them may be blank: the list's
// word, the panel's phrase, the pane's heading, and the line left on screen when
// it is over.
func TestEveryJobSpeaksFourTimes(t *testing.T) {
	jobs := []Job{
		{Kind: JobStart, World: "default"},
		{Kind: JobStop, World: "default"},
		{Kind: JobStopAll},
		{Kind: JobSetDefault, World: "second"},
		{Kind: JobEdit, World: "default", Flags: []string{"--save-minutes", "5"}},
		{Kind: JobHeadless, World: "default", Headless: true},
		{Kind: JobHeadless, World: "default", Headless: false},
		{Kind: JobCreate, World: "world2"},
		{Kind: JobClone, World: "default", Other: "world2"},
		{Kind: JobDelete, World: "world2"},
		{Kind: JobCheck, World: "default"},
	}
	for _, job := range jobs {
		busy := job.Busy()
		if busy.Short == "" || !strings.HasSuffix(busy.Short, "...") {
			t.Fatalf("%+v: the list says %q", job, busy.Short)
		}
		if job.Progress() == "" || !strings.HasSuffix(job.Progress(), "...") {
			t.Fatalf("%+v: the panel says %q", job, job.Progress())
		}
		if job.Heading() == "" {
			t.Fatalf("%+v: the pane's heading is empty", job)
		}
		good, bad := job.Result(0, ""), job.Result(1, "")
		if !good.Good || bad.Good {
			t.Fatalf("%+v: 0 is %+v and 1 is %+v", job, good, bad)
		}
		if good.Text == "" || bad.Text == "" || good.Text == bad.Text {
			t.Fatalf("%+v: success reads %q and failure %q", job, good.Text, bad.Text)
		}
		// A failure always points at where the reason is, because the window
		// invents no diagnosis of its own.
		if !strings.Contains(strings.ToLower(bad.Text), "details") {
			t.Fatalf("%+v: the failure does not point at the details: %q", job, bad.Text)
		}
	}

	// The two ways round the window setting can go read differently, or nobody
	// can tell which way a click went.
	on := Job{Kind: JobHeadless, World: "default", Headless: true}
	off := Job{Kind: JobHeadless, World: "default", Headless: false}
	if on.Result(0, "").Text == off.Result(0, "").Text || on.Progress() == off.Progress() {
		t.Fatal("both ways round the window setting say the same thing")
	}
	// The heading is the command line it amounts to, so a pasted pane is a
	// reproducible report.
	if got := on.Heading(); !strings.Contains(got, "--headless") {
		t.Fatalf("the pane's heading is %q", got)
	}
	if got := (Job{Kind: JobEdit, World: "d", Flags: []string{"--save-minutes", "5"}}).Heading(); !strings.Contains(got, "--save-minutes 5") {
		t.Fatalf("an edit's heading is %q", got)
	}

	// 'stop every world' is about every world, and every other job about one.
	if !(Job{Kind: JobStopAll}).Busy().All {
		t.Fatal("'stop every world' did not cover every world")
	}
	if (Job{Kind: JobStop, World: "default"}).Busy().All {
		t.Fatal("stopping one world covered every world")
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
	// And the assembled line is one the progress reader recognises, which is the
	// join between the two halves of this file.
	if _, ok := ProgressFor(lines[0]); !ok {
		t.Fatalf("the pane's own line is not readable as progress: %q", lines[0])
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

// TestLogFollowsTail. The pane held 191 lines with its first visible line still
// 0 on a real machine, so the follow rule is pinned here: at the bottom it
// follows, one line up it still does, further up it leaves the reader alone.
func TestLogFollowsTail(t *testing.T) {
	// 40 lines of text in a 10-line pane: the range is 0..39 and the last
	// position the bar can take is 30.
	const page, max = 10, 39
	cases := []struct {
		name string
		pos  int
		want bool
	}{
		{"at the bottom", 30, true},
		{"one line off the bottom, which is a partial row", 29, true},
		{"two lines up, which is a reader", 28, false},
		{"at the top of a long log", 0, false},
	}
	for _, test := range cases {
		if got := LogFollowsTail(test.pos, page, max); got != test.want {
			t.Fatalf("%s: LogFollowsTail(%d, %d, %d) = %v, want %v",
				test.name, test.pos, page, max, got, test.want)
		}
	}

	// Nothing to scroll — everything fits, or the bar could not be read — is
	// always a follow. A pane that stopped following is the defect this fixes.
	if !LogFollowsTail(0, 0, 0) {
		t.Fatal("a pane with no scroll bar does not follow its own newest line")
	}
	if !LogFollowsTail(0, 40, 39) {
		t.Fatal("a pane taller than its text does not follow its own newest line")
	}
}

// A REFUSAL IS ALREADY A GOOD SENTENCE. The core says what was wrong and what to
// do about it; a window that answered "the operation failed" instead would be
// throwing away the best line in the program.
func TestAFailureQuotesTheCoresOwnWords(t *testing.T) {
	job := Job{Kind: JobCreate, World: "world2"}
	got := job.Result(1, "the sidecar port 70000 is outside 1024-65535")
	if got.Good {
		t.Fatal("a non-zero exit was called a success")
	}
	if !strings.Contains(got.Text, "world2") || !strings.Contains(got.Text, "70000") {
		t.Fatalf("the failure reads %q", got.Text)
	}
	// With nothing to quote it still points at where the reason is.
	bare := job.Result(1, "")
	if !strings.Contains(strings.ToLower(bare.Text), "details") {
		t.Fatalf("a failure with nothing to quote reads %q", bare.Text)
	}

	// A health check's report is the whole output, so its last line — the exit
	// status — is not quoted at it.
	check := Job{Kind: JobCheck, World: "default"}.Result(1, "the diagnostic finished with: exit status 3")
	if strings.Contains(check.Text, "exit status") {
		t.Fatalf("the health check quoted its own exit status: %q", check.Text)
	}
	if !strings.Contains(check.Text, "fault") {
		t.Fatalf("a failed health check reads %q", check.Text)
	}
}

// WHICH LINE IS THE REFUSAL. The core prints its refusals flush left and every
// continuation — the tail of a log, the five usual causes, the remedy — indented
// under them, so the last flush-left line before a non-zero exit is the refusal
// itself rather than the fourth line of a list explaining it.
func TestSaidLineTakesTheRefusalAndNotItsExplanation(t *testing.T) {
	usable := []struct{ line, want string }{
		{"09:41:07  The map did not grant a place.", "The map did not grant a place."},
		{"the sidecar port 70000 is outside 1024-65535", "the sidecar port 70000 is outside 1024-65535"},
		{"09:41:07  that is not 'world2'. Nothing was deleted", "that is not 'world2'. Nothing was deleted"},
	}
	for _, test := range usable {
		got, ok := SaidLine(test.line)
		if !ok || got != test.want {
			t.Fatalf("%q became %q (usable %v)", test.line, got, ok)
		}
	}
	unusable := []string{
		"",
		"09:41:07  ",
		"09:41:07    2. 'the relay's TLS certificate did not verify'. On a public map that is a",
		"09:41:07  \tsomething indented with a tab",
		"09:41:07  > start default",
		"09:41:07  " + strings.Repeat("x", FailureDetailLimit+1),
	}
	for _, line := range unusable {
		if got, ok := SaidLine(line); ok {
			t.Fatalf("%q was quoted as a refusal: %q", line, got)
		}
	}
}

// DECIDING NOT TO FOLLOW WAS NEVER ENOUGH, which a machine measured on the real
// window: after a scroll to the top LogFollowsTail correctly answered false
// (pos 0, page 15, max 299) and the next appended line still dragged the view
// from line 0 to line 286. walk's AppendText selects the end and replaces the
// selection, and an EDIT control scrolls the caret into view as part of that,
// before this program is consulted. So the reader is put BACK, by the number of
// lines the control moved them.
func TestAReaderWhoScrolledUpIsPutBack(t *testing.T) {
	// The measured case, exactly: reading the top of the log while a start
	// prints.
	if got := LinesToRestore(0, 286); got != -286 {
		t.Fatalf("a reader dragged from line 0 to line 286 is scrolled by %d, want -286", got)
	}
	// A pane that did not move needs no message at all.
	if got := LinesToRestore(120, 120); got != 0 {
		t.Fatalf("an unmoved pane is scrolled by %d", got)
	}
	// And a reading the control could not give is not a licence to guess.
	for _, unreadable := range [][2]int{{-1, 286}, {0, -1}, {-1, -1}} {
		if got := LinesToRestore(unreadable[0], unreadable[1]); got != 0 {
			t.Fatalf("LinesToRestore%v = %d, want 0", unreadable, got)
		}
	}
	// The two halves agree about the same reading: a pane at the bottom follows,
	// and following is the branch that does not restore anything.
	if !LogFollowsTail(285, 15, 299) {
		t.Fatal("a pane at the bottom stopped following")
	}
	if LogFollowsTail(0, 15, 299) {
		t.Fatal("a pane scrolled to the top followed anyway")
	}
}

// THE REFUSALS, AS THE CORE ACTUALLY PRINTS THEM.
//
// A machine watched all three of these happen and reported the panel saying
// "'world2' was not created. The details below say why." while the core's own
// sentence sat one line lower in a pane nobody had opened. The strings below are
// copied from internal/launcher — validate.go, profilecmd.go and lock.go — with
// the heading the window writes above them and the blank line the log emits, in
// the order they arrive. Feeding a whole transcript is the point: the rule is
// "the last flush-left line wins", and only a transcript can prove it picked the
// right one.
func TestTheThreeRefusalsAMachineSawReachThePanel(t *testing.T) {
	cases := []struct {
		name       string
		job        Job
		transcript []string
		want       string
	}{
		{
			// go/internal/launcher/validate.go: the port a new world was given is
			// already another world's.
			name: "a create refused for a port that is taken",
			job:  Job{Kind: JobCreate, World: "world2"},
			transcript: []string{
				"> create world2 and enroll a new identity on the map",
				"",
				"the profile 'default' already uses port 8787. Every world needs its own",
			},
			want: "the profile 'default' already uses port 8787. Every world needs its own",
		},
		{
			// go/internal/launcher/profilecmd.go: the typed name did not match. The
			// custody warning is printed first and is INDENTED, which is what the
			// flush-left rule is for.
			name: "a delete refused because the name did not match",
			job:  Job{Kind: JobDelete, World: "multi-2"},
			transcript: []string{
				"> delete multi-2",
				"",
				"  Deleting this world here is NOT leaving the map. Your credential still authenticates",
				"  until the operator drops it, and your sidecar may still be holding organisms it took",
				"  custody of for somebody else. Custody is local: nobody else can drain it for you.",
				"that is not 'multi-2'. Nothing was deleted",
			},
			want: "that is not 'multi-2'. Nothing was deleted",
		},
		{
			// go/internal/launcher/lock.go: a second launcher holds this world.
			name: "a start refused by the other launcher's lock",
			job:  Job{Kind: JobStart, World: "default"},
			transcript: []string{
				"> start default",
				"",
				"another launcher is starting or stopping this world " +
					`(C:\Users\p\AppData\Local\BibitesMultiverse\launcher.lock was taken 0s ago by pid 12000). ` +
					"Wait for it to finish",
			},
			want: "another launcher is starting or stopping this world " +
				`(C:\Users\p\AppData\Local\BibitesMultiverse\launcher.lock was taken 0s ago by pid 12000). ` +
				"Wait for it to finish",
		},
	}
	for _, test := range cases {
		// This is what the window does: clear at the start of the job, then take
		// every line as it is written.
		said := ""
		for _, line := range test.transcript {
			if quotable, ok := SaidLine(line); ok {
				said = quotable
			}
		}
		if said != test.want {
			t.Fatalf("%s: kept %q, want %q", test.name, said, test.want)
		}
		result := test.job.Result(1, said)
		if result.Good {
			t.Fatalf("%s: a refusal was called a success", test.name)
		}
		if !strings.Contains(result.Text, test.want) {
			t.Fatalf("%s: the panel would say %q, which does not carry the core's own sentence",
				test.name, result.Text)
		}
		// And it still names what was being attempted, because the core's
		// sentence on its own does not always say.
		if !strings.Contains(result.Text, test.job.failedHead()) {
			t.Fatalf("%s: the panel would say %q, which does not say what failed", test.name, result.Text)
		}
	}
}

// The heading the window writes is not the core talking, and neither is a blank
// line — so neither may ever be what a failure quotes.
func TestTheWindowsOwnLinesAreNeverQuotedBackAtIt(t *testing.T) {
	for _, job := range []Job{
		{Kind: JobStart, World: "default"},
		{Kind: JobCreate, World: "world2"},
		{Kind: JobDelete, World: "multi-2"},
		{Kind: JobStopAll},
	} {
		if _, ok := SaidLine("> " + job.Heading()); ok {
			t.Fatalf("the pane's own heading was quotable: %q", job.Heading())
		}
	}
	if _, ok := SaidLine(""); ok {
		t.Fatal("a blank line was quotable")
	}
}
