package launcher

import (
	"os"
	"strings"
	"testing"
	"time"
)

// The mod's command protocol, held to bibites-mod/src/DevCommands.cs rather than
// to what the launcher happens to send. Every rule here is one the mod enforces,
// and getting any of them wrong is a stop that silently falls back to killing the
// game — which is what LOCAL-HEADLESSSTOP costs a headless world.

func TestReadModAnswer(t *testing.T) {
	const token = "stop-4242-1"
	cases := []struct {
		name    string
		log     string
		found   bool
		ok      bool
		details string
	}{
		{"the mod's own answer to quit", token + " OK quitting\n", true, true, "quitting"},
		{"a refusal", token + " ERROR no world is loaded\n", true, false, "no world is loaded"},
		{"an answer with no details", token + " OK\n", true, true, ""},
		{"another request's answer", "stop-4242-0 OK quitting\n", false, false, ""},
		{"an answer after another one", "other OK census\n" + token + " OK quitting\n", true, true, "quitting"},
		{"a line still being written", token + " OK quit", false, false, ""},
		{"an empty log", "", false, false, ""},
		{"a token that is a prefix of ours", token + "x OK quitting\n", false, false, ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			path := t.TempDir() + "/cmd.txt.log"
			if err := os.WriteFile(path, []byte(test.log), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			ok, details, found := readModAnswer(path, token)
			if found != test.found {
				t.Fatalf("found = %v, want %v", found, test.found)
			}
			if !found {
				return
			}
			if ok != test.ok || details != test.details {
				t.Fatalf("ok = %v details = %q, want %v %q", ok, details, test.ok, test.details)
			}
		})
	}

	// A log that is not there at all is "no answer yet", not a failure: the mod
	// creates it when it first answers.
	if _, _, found := readModAnswer(t.TempDir()+"/nothing", token); found {
		t.Fatal("a missing log answered something")
	}
}

// TestModQuitTokenHasNoWhitespace: the mod splits the command on spaces and tabs
// and takes the FIRST field as the token. A token with a space in it would be
// read as a token plus a verb, and the answer would carry a name the launcher
// never waits for.
func TestModQuitTokenHasNoWhitespace(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token := modQuitToken()
		if len(strings.Fields(token)) != 1 {
			t.Fatalf("the token %q is not one field", token)
		}
		if seen[token] {
			t.Fatalf("the token %q was handed out twice", token)
		}
		seen[token] = true
	}
}

// TestAskModToQuitWritesWhatTheModReads is the writer's half: one line, ending
// in a newline (the mod discards content that does not, as a partial write), the
// verb the mod answers with a save-and-quit, and a file that appears whole
// because it is renamed into place.
func TestAskModToQuitWritesWhatTheModReads(t *testing.T) {
	fastModQuit(t)
	h := newHarness(t)
	p := h.profile("default", "Multiverse", 8787)

	// Stand in for the mod: take the command, check its shape, answer it.
	seen := make(chan string, 1)
	go func() {
		for {
			raw, err := os.ReadFile(p.CommandFile())
			if err != nil || len(raw) == 0 {
				time.Sleep(2 * time.Millisecond)
				continue
			}
			os.Remove(p.CommandFile())
			seen <- string(raw)
			fields := strings.Fields(string(raw))
			os.WriteFile(p.CommandLogFile(), []byte(fields[0]+" OK quitting\n"), 0o644)
			return
		}
	}()

	result, details, err := askModToQuit(p)
	if err != nil || result != modQuitAccepted {
		t.Fatalf("askModToQuit = %v %q %v, want modQuitAccepted", result, details, err)
	}
	if details != "quitting" {
		t.Fatalf("the answer's details are %q", details)
	}
	command := <-seen
	if !strings.HasSuffix(command, "\n") {
		t.Fatalf("the command %q does not end in a newline, so the mod would discard it", command)
	}
	if strings.Count(command, "\n") != 1 {
		t.Fatalf("the command %q is more than one line", command)
	}
	fields := strings.Fields(command)
	if len(fields) != 2 || fields[1] != modQuitVerb {
		t.Fatalf("the command is %q, want '<token> %s'", command, modQuitVerb)
	}
	if fileExists(p.CommandFile() + cmdTempSuffix) {
		t.Fatal("the temporary the command was renamed from was left behind")
	}
}

// TestAskModToQuitTakesItsRequestBack: nothing read the file, which is what a
// world started before this launcher set MULTIVERSE_CMD_FILE looks like. The
// answer is "no consumer", it arrives in about a second rather than five, and
// THE REQUEST IS REMOVED — a 'quit' left there is taken by the next start of
// this world, seconds after it comes up.
func TestAskModToQuitTakesItsRequestBack(t *testing.T) {
	fastModQuit(t)
	h := newHarness(t)
	p := h.profile("default", "Multiverse", 8787)

	// A stale answer from an earlier stop must not be read as this one's.
	writeFile(t, p.CommandLogFile(), "stop-1-1 OK quitting\n", 0o644)

	result, _, err := askModToQuit(p)
	if err != nil {
		t.Fatalf("askModToQuit: %v", err)
	}
	if result != modQuitNoConsumer {
		t.Fatalf("askModToQuit = %v, want modQuitNoConsumer", result)
	}
	if fileExists(p.CommandFile()) {
		t.Fatalf("%s was left behind", p.CommandFile())
	}
}

// TestAskModToQuitReadsARefusal: the mod answered, and said no.
func TestAskModToQuitReadsARefusal(t *testing.T) {
	fastModQuit(t)
	h := newHarness(t)
	p := h.profile("default", "Multiverse", 8787)

	go func() {
		for {
			raw, err := os.ReadFile(p.CommandFile())
			if err != nil || len(raw) == 0 {
				time.Sleep(2 * time.Millisecond)
				continue
			}
			os.Remove(p.CommandFile())
			token := strings.Fields(string(raw))[0]
			os.WriteFile(p.CommandLogFile(), []byte(token+" ERROR no world is loaded\n"), 0o644)
			return
		}
	}()

	result, details, err := askModToQuit(p)
	if err != nil || result != modQuitRefused {
		t.Fatalf("askModToQuit = %v %v, want modQuitRefused", result, err)
	}
	if details != "no world is loaded" {
		t.Fatalf("the refusal's details are %q", details)
	}
}

// TestCommandFileIsThisWorldsOwn: two worlds out of one game folder that shared
// a command file would quit each other, and the file is inside the data root so
// --remove-world-data takes it with the world.
func TestCommandFileIsThisWorldsOwn(t *testing.T) {
	h := newHarness(t)
	first := h.profile("default", "Multiverse", 8787)
	second := h.profile("second", "Second", 8788)

	if first.CommandFile() == second.CommandFile() {
		t.Fatalf("two worlds share %s", first.CommandFile())
	}
	if !pathWithin(first.CommandFile(), first.DataRoot) {
		t.Fatalf("%s is not under %s", first.CommandFile(), first.DataRoot)
	}
	if first.CommandLogFile() != first.CommandFile()+".log" {
		t.Fatalf("the answer log is %s; the mod appends to <command file>.log",
			first.CommandLogFile())
	}
	owned := make(map[string]bool)
	for _, name := range worldOwnedEntries() {
		owned[name] = true
	}
	for _, name := range []string{cmdFileName, cmdFileName + cmdLogSuffix, cmdFileName + cmdTempSuffix} {
		if !owned[name] {
			t.Fatalf("%s is not one of the entries --remove-world-data deletes", name)
		}
	}
}

// fastModQuit shrinks the command channel's poll so a test does not spend real
// seconds on it.
func fastModQuit(t *testing.T) {
	t.Helper()
	was := modQuitPollInterval
	modQuitPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { modQuitPollInterval = was })
}
