package launcher

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The regressions from the adversarial review. Every test here reproduces a
// scenario that destroyed data, killed a stranger's process, or leaked the map
// credential before it was fixed.

// TestProfileLoadRefusesUnsafeShapes: a profile is validated when it is READ,
// not only when it is written. An empty or relative dataRoot resolves against
// whatever folder the program was started from, and everything downstream —
// logs, the lock, the pid files, --remove-world-data — then acts there.
func TestProfileLoadRefusesUnsafeShapes(t *testing.T) {
	h := newHarness(t)
	good := h.profile("default", "Multiverse", 8787)

	field := func(name, value string) string {
		p := good
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var generic map[string]any
		if err := json.Unmarshal(raw, &generic); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		generic["name"] = "bad"
		var decoded any
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			decoded = value
		}
		generic[name] = decoded
		out, err := json.Marshal(generic)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return string(out)
	}

	cases := []struct {
		name string
		body string
		want string
	}{
		{"an empty dataRoot", field("dataRoot", `""`), "'dataRoot' is empty"},
		{"a relative dataRoot", field("dataRoot", `"precious"`), "relative path"},
		{"a dataRoot at the top of a drive", field("dataRoot", `"/"`), "top of a drive"},
		{"an empty gameDir", field("gameDir", `""`), "'gameDir' is empty"},
		{"a relative gameDir", field("gameDir", `"game"`), "relative path"},
		{"a port of 0", field("sidecarPort", `0`), "'sidecarPort' (0)"},
		{"a port above the range", field("sidecarPort", `70000`), "outside"},
		{"an empty world", field("world", `""`), "'world' is empty"},
		{"the wrong format", field("format", `"something/else/9"`), "this launcher reads"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			h.writeRawProfile("bad", test.body)
			defer os.Remove(h.install().ProfilePath("bad"))
			_, err := h.install().LoadProfile("bad")
			if err == nil {
				t.Fatalf("%s was loaded and must not be", test.name)
			}
			mustContain(t, "the refusal", err.Error(), test.want)
		})
	}

	// A dataRoot that overlaps the installed application, or its own game
	// folder, is refused at load too - that is the load-side half of the
	// deletion hazard.
	for _, dataRoot := range []string{h.root, h.install().ProfilesDir(), h.gameDir,
		filepath.Join(h.gameDir, "BepInEx")} {
		h.writeRawProfile("bad", field("dataRoot", `"`+dataRoot+`"`))
		if _, err := h.install().LoadProfile("bad"); err == nil {
			t.Fatalf("a profile with dataRoot %q was loaded", dataRoot)
		}
		os.Remove(h.install().ProfilePath("bad"))
	}
}

// TestProfileCreateRefusesUnsafeDataRoot: the writer's side of the same rule.
func TestProfileCreateRefusesUnsafeDataRoot(t *testing.T) {
	h := newHarness(t)
	h.profile("default", "Multiverse", 8787)
	joinFile := writeJoinFile(t, "second")

	for _, dataRoot := range []string{h.gameDir, h.root, h.install().ProfilesDir()} {
		code := h.run("profile", "create", "second", "--world", "Second",
			"--sidecar-port", "8790", "--data-root", dataRoot,
			"--game-dir", h.gameDir, "--join-string-file", joinFile)
		if code != exitRefused {
			t.Fatalf("--data-root %s exited %d, want %d\n%s", dataRoot, code, exitRefused, h.err())
		}
		mustContain(t, "the refusal", h.err(), "overlap")
		if fileExists(h.install().ProfilePath("second")) {
			t.Fatalf("a profile was written for --data-root %s", dataRoot)
		}
	}
}

// TestProfileDeleteRefusesUnsafeDataRoot is the reviewer's scenario: a profile
// whose dataRoot is a game folder, the install root, a relative path or empty
// must refuse, and must delete NOTHING - not the tree, not even the profile.
func TestProfileDeleteRefusesUnsafeDataRoot(t *testing.T) {
	cases := []struct {
		name     string
		dataRoot func(h *harness) string
		guard    func(h *harness) string
	}{
		{"a game folder", func(h *harness) string { return h.gameDir },
			func(h *harness) string { return h.gameDir }},
		{"the install root", func(h *harness) string { return h.root },
			func(h *harness) string { return h.root }},
		{"a relative path", func(h *harness) string { return "precious" },
			func(h *harness) string { return h.gameDir }},
		{"empty", func(h *harness) string { return "" },
			func(h *harness) string { return h.gameDir }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t)
			h.profile("default", "Multiverse", 8787)
			victim := h.profile("victim", "Victim", 8788)

			// Rewrite the profile by hand, the way a lost or edited key would.
			raw, err := json.Marshal(victim)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var generic map[string]any
			json.Unmarshal(raw, &generic)
			generic["dataRoot"] = test.dataRoot(h)
			body, _ := json.Marshal(generic)
			h.writeRawProfile("victim", string(body))

			guarded := test.guard(h)
			before := len(mustReadDir(t, guarded))

			code := h.runWith("victim\nvictim\n", "--yes", "profile", "delete", "victim",
				"--remove-world-data")
			if code == exitOK {
				t.Fatalf("the delete succeeded and must not have\n%s", h.out())
			}
			if _, err := os.Stat(guarded); err != nil {
				t.Fatalf("%s was deleted: %v", guarded, err)
			}
			if after := len(mustReadDir(t, guarded)); after != before {
				t.Fatalf("%s went from %d entries to %d", guarded, before, after)
			}
			if !fileExists(h.install().ProfilePath("victim")) {
				t.Fatal("the profile file was deleted even though the removal was refused")
			}
			if !fileExists(h.install().ProfilePath("default")) {
				t.Fatal("another world's profile was deleted")
			}
		})
	}
}

// TestProfileDeleteRemovesOnlyItsOwnData is the happy path, plus the rule that
// --yes alone can never destroy a directory tree.
func TestProfileDeleteRemovesOnlyItsOwnData(t *testing.T) {
	h := newHarness(t)
	h.profile("default", "Multiverse", 8787)
	second := h.profile("second", "Second", 8788)
	writeFile(t, filepath.Join(second.DataDir(), "journal.jsonl"), "custody\n", 0o644)

	// --yes does NOT answer the typed-name question when a folder is going away.
	if code := h.runWith("\n", "--yes", "profile", "delete", "second", "--remove-world-data"); code == exitOK {
		t.Fatalf("--yes deleted a data folder with no name typed\n%s", h.out())
	}
	if !dirExists(second.DataRoot) {
		t.Fatal("the data folder was deleted anyway")
	}
	if !fileExists(h.install().ProfilePath("second")) {
		t.Fatal("the profile was deleted anyway")
	}

	// Typing the name does it.
	if code := h.runWith("second\n", "profile", "delete", "second", "--remove-world-data"); code != exitOK {
		t.Fatalf("delete exited %d\n%s\n%s", code, h.out(), h.err())
	}
	if dirExists(second.DataRoot) {
		t.Fatalf("%s survived", second.DataRoot)
	}
	if fileExists(h.install().ProfilePath("second")) {
		t.Fatal("the profile survived")
	}
	// Nothing else moved.
	if !dirExists(h.gameDir) || !fileExists(h.install().ProfilePath("default")) {
		t.Fatal("the delete reached beyond its own world")
	}

	// Without --remove-world-data, --yes still answers, and the journal stays.
	third := h.profile("third", "Third", 8789)
	if code := h.run("--yes", "profile", "delete", "third"); code != exitOK {
		t.Fatalf("delete exited %d\n%s", code, h.err())
	}
	if !dirExists(third.DataRoot) {
		t.Fatal("the data folder was deleted without --remove-world-data")
	}
	mustContain(t, "the output", h.out(), "kept "+third.DataRoot)
}

// TestDeleteRemovesOnlyWhatTheWorldOwns is the complete edition's data root as
// it is on a real machine: the world's journal and credential, THE GAME under
// runtimes\, the installer's record, an orphaned credential from an interrupted
// install, a private map's certificate, and the leftover folders of two earlier
// deployments. os.RemoveAll on that root took every one of them - a multi-gigabyte
// game, the file the uninstaller reads, an identity no map can print again, and
// two other worlds' journals - for "delete this world".
func TestDeleteRemovesOnlyWhatTheWorldOwns(t *testing.T) {
	h := newHarness(t)
	h.profile("default", "Multiverse", 8787)
	second := h.managedProfile("second", "Second", 8788)

	// The world's own entries.
	writeFile(t, filepath.Join(second.DataDir(), "journal.jsonl"), "custody\n", 0o644)
	writeFile(t, filepath.Join(second.DataDir(), peerIDFileName), "public-abc\n", 0o644)
	writeFile(t, second.SidecarLog(), "sidecar: contract B: slot granted\n", 0o644)
	writeFile(t, second.PendingFile(), "{}", 0o600)

	// Everything in that folder that is NOT the world's.
	strangers := []string{
		filepath.Join(second.GameDir, gameExeName),
		filepath.Join(second.DataRoot, recordName),
		filepath.Join(second.DataRoot, credentialFileName+".20260814T101112Z.orphan"),
		filepath.Join(second.DataRoot, "relay-ca.crt"),
		filepath.Join(second.DataRoot, "data-slot-2", "data", "journal.jsonl"),
		filepath.Join(second.DataRoot, "data-slot-6", "data", "journal.jsonl"),
	}
	writeFile(t, filepath.Join(second.DataRoot, recordName),
		`{"record":"bibites-multiverse/install-record/3","gameDir":"`+second.GameDir+`"}`, 0o644)
	writeFile(t, filepath.Join(second.DataRoot, credentialFileName+".20260814T101112Z.orphan"),
		strings.Repeat("c", 64)+"\n", 0o600)
	writeFile(t, filepath.Join(second.DataRoot, "relay-ca.crt"), "-----BEGIN CERTIFICATE-----\n", 0o644)
	writeFile(t, filepath.Join(second.DataRoot, "data-slot-2", "data", "journal.jsonl"), "somebody else\n", 0o644)
	writeFile(t, filepath.Join(second.DataRoot, "data-slot-6", "data", "journal.jsonl"), "somebody else\n", 0o644)

	if code := h.runWith("second\n", "profile", "delete", "second", "--remove-world-data"); code != exitOK {
		t.Fatalf("delete exited %d\n%s\n%s", code, h.out(), h.err())
	}

	// The world's own entries are gone ...
	for _, path := range []string{second.DataDir(), second.LogDir()} {
		if dirExists(path) {
			t.Fatalf("%s survived the removal", path)
		}
	}
	for _, path := range []string{second.CredentialFile(), second.PendingFile(),
		h.install().ProfilePath("second")} {
		if fileExists(path) {
			t.Fatalf("%s survived the removal", path)
		}
	}
	// ... and NOTHING else was touched, starting with the game.
	for _, path := range strangers {
		if !fileExists(path) {
			t.Fatalf("%s was deleted, and it is not this world's", path)
		}
	}
	if !dirExists(second.DataRoot) {
		t.Fatalf("%s was removed although it still holds things this world does not own",
			second.DataRoot)
	}

	// And the output says so, by name, rather than claiming the folder is gone.
	out := h.out()
	mustContain(t, "the output", out, "kept "+second.DataRoot)
	for _, name := range []string{runtimesDirName, recordName, "data-slot-6", "relay-ca.crt"} {
		mustContain(t, "the list of what was left", out, name)
	}
	mustContain(t, "the output", out, "The game's own save of 'Second' is NOT here")
	mustNotContain(t, "the output", out, "deleted "+second.DataRoot+",")

	// The other world is untouched, and the installation still works.
	if code := h.run("status", "--all"); code != exitOK {
		t.Fatalf("status after the delete exited %d\n%s", code, h.err())
	}
}

// TestDeleteWholeDataRootWhenItIsAllOurs: the other half of the same rule. A
// world whose data root holds only its own entries still loses the root itself,
// so an ordinary delete leaves nothing behind.
func TestDeleteWholeDataRootWhenItIsAllOurs(t *testing.T) {
	h := newHarness(t)
	h.profile("default", "Multiverse", 8787)
	second := h.profile("second", "Second", 8788)
	writeFile(t, filepath.Join(second.DataDir(), "journal.jsonl"), "custody\n", 0o644)
	writeFile(t, second.SidecarLog(), "log\n", 0o644)

	if code := h.runWith("second\n", "profile", "delete", "second", "--remove-world-data"); code != exitOK {
		t.Fatalf("delete exited %d\n%s", code, h.err())
	}
	if dirExists(second.DataRoot) {
		t.Fatalf("%s survived", second.DataRoot)
	}
	mustContain(t, "the output", h.out(), "deleted "+second.DataRoot+", including its journal")
}

// TestDeleteWarnsAboutTheInstallRecord: the installer-created world's data root
// holds install-record.json, which the uninstaller needs.
func TestDeleteWarnsAboutTheInstallRecord(t *testing.T) {
	h := newHarness(t)
	p := h.profile("default", "Multiverse", 8787)
	writeFile(t, p.RecordFile(),
		`{"record":"bibites-multiverse/install-record/3","gameDir":"`+h.gameDir+`"}`, 0o644)

	if code := h.runWith("default\n", "profile", "delete", "default", "--remove-world-data"); code != exitOK {
		t.Fatalf("delete exited %d\n%s", code, h.err())
	}
	mustContain(t, "the warning", h.out(), "install-record.json")
	mustContain(t, "the warning", h.out(), "uninstaller")
	// And the record it warns about is still there afterwards, which is what makes
	// the uninstaller able to undo the mod installation at all.
	if !fileExists(p.RecordFile()) {
		t.Fatalf("%s was deleted; the uninstaller reads it", p.RecordFile())
	}
}

// TestProfileSetValidatesGameDir: 'profile set --game-dir' reached the file
// without any check, so a mistyped folder was accepted silently.
func TestProfileSetValidatesGameDir(t *testing.T) {
	h := newHarness(t)
	p := h.profile("default", "Multiverse", 8787)

	missing := filepath.Join(t.TempDir(), "nowhere")
	if code := h.run("profile", "set", "default", "--game-dir", missing); code != exitRefused {
		t.Fatalf("a missing game folder exited %d, want %d\n%s", code, exitRefused, h.err())
	}
	mustContain(t, "the refusal", h.err(), "is not a game folder")

	// And it cannot be pointed at this world's own data root, which would feed
	// the deletion hazard.
	if code := h.run("profile", "set", "default", "--game-dir", p.DataRoot); code != exitRefused {
		t.Fatalf("the data root as a game folder exited %d, want %d", code, exitRefused)
	}
	after, err := h.install().LoadProfile("default")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if after.GameDir != h.gameDir {
		t.Fatalf("the game folder was changed to %s", after.GameDir)
	}
}

// TestBadProfileDoesNotBlockReportingOrStopping: one stray file in profiles\
// must not leave a participant unable to see or shut down the worlds that do
// parse. The collision-sensitive commands still refuse.
func TestBadProfileDoesNotBlockReportingOrStopping(t *testing.T) {
	h := newHarness(t)
	h.profile("default", "Multiverse", 8787)
	h.writeRawProfile("broken", "{ this is not json")

	for _, command := range [][]string{
		{"status", "--all"},
		{"profile", "list"},
		{"stop", "--all"},
	} {
		if code := h.run(command...); code != exitOK {
			t.Fatalf("%v exited %d\n%s", command, code, h.err())
		}
		mustContain(t, "stderr for "+strings.Join(command, " "), h.err(), "broken.json")
		mustContain(t, "stdout for "+strings.Join(command, " "), h.out()+h.err(), "default")
	}

	// start is collision-sensitive, so it still refuses and names the escape.
	if code := h.run("start", "--dry-run"); code != exitRefused {
		t.Fatalf("start exited %d, want %d", code, exitRefused)
	}
	mustContain(t, "the refusal", h.err(), "broken.json")
	mustContain(t, "the refusal", h.err(), "still work")
}

// TestGlobalFlagDoesNotEatAFlagValue: global flags are lifted from anywhere on
// the line, so a VALUE that looks like a global has to be stepped over.
func TestGlobalFlagDoesNotEatAFlagValue(t *testing.T) {
	h := newHarness(t)
	h.profile("default", "Multiverse", 8787)

	// The value reaches the flag rather than being read as --help: the command
	// runs, the usage is not printed, and the world is what was typed.
	if code := h.run("profile", "set", "default", "--world", "-h"); code != exitOK {
		t.Fatalf("'--world -h' exited %d\n%s", code, h.err())
	}
	mustNotContain(t, "the output", h.out(), "Global flags (accepted anywhere on the line)")
	after, err := h.install().LoadProfile("default")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if after.World != "-h" {
		t.Fatalf("the world became %q, want the value that was typed", after.World)
	}
	// -h on its own is still the help flag.
	if code := h.run("profile", "set", "default", "-h"); code != exitOK {
		t.Fatalf("-h exited %d", code)
	}
	mustContain(t, "the output", h.out(), "Global flags (accepted anywhere on the line)")
	// And an ordinary set still works.
	if code := h.run("profile", "set", "default", "--world", "Second"); code != exitOK {
		t.Fatalf("a normal set exited %d\n%s", code, h.err())
	}
	// A global after the command still works.
	if code := h.run("status", "--all", "--json"); code != exitOK {
		t.Fatalf("status --all --json exited %d\n%s", code, h.err())
	}
}

// TestInstallRootNeedsAPath: --install-root must not swallow the command name.
func TestInstallRootNeedsAPath(t *testing.T) {
	h := newHarness(t)
	h.profile("default", "Multiverse", 8787)
	var out, errOut bytes.Buffer
	code := run([]string{"--install-root", "status", "--all"}, strings.NewReader(""),
		&out, &errOut,
		func(string) string { return "" },
		func() (string, error) { return filepath.Join(h.root, LauncherExeName), nil },
		func() time.Time { return h.now })
	if code != exitUsage {
		t.Fatalf("--install-root with no path exited %d, want %d\n%s", code, exitUsage, errOut.String())
	}
	mustContain(t, "the refusal", errOut.String(), "--install-root needs the path")
}

// TestWaitRejectsANegativeValue: --wait 0 means look once; a negative value is
// a usage error rather than a silent minute.
func TestWaitRejectsANegativeValue(t *testing.T) {
	h := newHarness(t)
	h.profile("default", "Multiverse", 8787)
	if code := h.run("start", "--wait", "-1"); code != exitUsage {
		t.Fatalf("--wait -1 exited %d, want %d", code, exitUsage)
	}
	mustContain(t, "the refusal", h.err(), "0 or more")
}

// TestDryRunReportsTheGameOnlyRefusal: the plan must not promise a start the
// real command would refuse.
func TestDryRunReportsTheGameOnlyRefusal(t *testing.T) {
	h := newHarness(t)
	h.profile("default", "Multiverse", 8787)
	if code := h.run("start", "--game-only", "--dry-run"); code != exitOK {
		t.Fatalf("the dry run exited %d\n%s", code, h.err())
	}
	mustContain(t, "the dry run", h.err(), "--game-only needs the sidecar")
	if code := h.run("start", "--game-only"); code != exitRefused {
		t.Fatalf("the real command exited %d, want %d", code, exitRefused)
	}
}

// writeJoinFile puts a private-map join string on disk, so a test can create a
// world without touching the network.
func writeJoinFile(t *testing.T, peerID string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "join.txt")
	writeFile(t, path, joinStringPrefix+" wss://relay.example/contract-b/v4 "+peerID+"."+
		strings.Repeat("b", 40)+"\n", 0o600)
	return path
}

func mustReadDir(t *testing.T, path string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return entries
}
