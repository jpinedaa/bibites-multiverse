package launcher

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"multiverse/internal/sidecar"
)

// TestNoSecretFlagExists enforces the rule the sidecar and the installer both
// keep: NO FLAG ANYWHERE TAKES A SECRET AS A VALUE, because a value on a
// command line is in every process listing on the machine. --join-string-file
// names a file, which is the only permitted match.
func TestNoSecretFlagExists(t *testing.T) {
	forbidden := []string{"secret", "credential", "join-string"}
	for _, name := range allFlagNames() {
		for _, word := range forbidden {
			if !strings.Contains(name, word) {
				continue
			}
			if name == "join-string-file" {
				continue
			}
			t.Fatalf("--%s is a flag whose name contains %q. A secret is passed by file, never "+
				"as a value", name, word)
		}
	}
	found := false
	for _, name := range allFlagNames() {
		if name == "join-string-file" {
			found = true
		}
	}
	if !found {
		t.Fatal("--join-string-file is not registered, so a private map cannot be joined")
	}
}

// TestStartDryRunPlan is the golden form of the launch: the five sidecar flags
// and the twelve MULTIVERSE_* variables, in their frozen order.
func TestStartDryRunPlan(t *testing.T) {
	h := newHarness(t)
	p := h.profile("default", "Multiverse", 8787)

	if code := h.run("start", "--dry-run"); code != exitOK {
		t.Fatalf("start --dry-run: %d\n%s", code, h.err())
	}
	want := fmt.Sprintf(`dry run: nothing was started
world 'Multiverse' from profile 'default'

sidecar: %s
  --listen 127.0.0.1:8787
  --relay %s
  --peer-id public-9af42a17616742e7a6e8c62cb8b95f4f
  --data-dir %s
  --credential-file %s
  working directory: %s
  stdout: %s
  stderr: %s

game: %s
  arguments: (none)
  working directory: %s
  environment:
    MULTIVERSE_EXPORT_EDGES=E,N,W,S
    MULTIVERSE_MIGRATION_EXCLUDE=Basic bibite
    MULTIVERSE_SAVE_MINUTES=10
    MULTIVERSE_SAVE_KEEP=6
    MULTIVERSE_SAVE_ON_QUIT=true
    MULTIVERSE_SIDECAR_PORT=8787
    MULTIVERSE_WORLD=Multiverse
    MULTIVERSE_PORTAL=true
    MULTIVERSE_PORTAL_FLOURISHES=true
    MULTIVERSE_STARTUP_TIME_SCALE=10
    MULTIVERSE_CONTRACT_A_TOKEN_FILE=%s
    MULTIVERSE_CMD_FILE=%s
`,
		h.install().SidecarExe(), testRelayURL, p.DataDir(), p.CredentialFile(),
		p.DataRoot, p.SidecarLogOut(), p.SidecarLog(),
		p.GameExe(), p.GameDir, p.ContractATokenFile(), p.CommandFile())
	if h.out() != want {
		t.Fatalf("the dry-run plan drifted.\n got:\n%s\nwant:\n%s", h.out(), want)
	}

	// Nothing was spawned and no pid was recorded.
	if fileExists(p.SidecarPidFile()) || fileExists(p.GamePidFile()) {
		t.Fatal("a dry run recorded a pid")
	}

	// --headless changes exactly one line.
	if code := h.run("start", "--dry-run", "--headless"); code != exitOK {
		t.Fatalf("start --dry-run --headless: %d\n%s", code, h.err())
	}
	mustContain(t, "the headless plan", h.out(), "  arguments: -batchmode -nographics")

	// The twelve variables are exactly twelve, and they are the mod's own names.
	if got := len(multiverseEnv(p)); got != 12 {
		t.Fatalf("the game is given %d MULTIVERSE_* variables, want 12", got)
	}
	// MULTIVERSE_CMD_FILE is what makes a HEADLESS stop lossless, and it has to
	// be this world's own file: two worlds out of one game folder sharing one
	// command file would quit each other.
	second := p
	second.Name, second.World = "second", "Second"
	second.DataRoot = filepath.Join(filepath.Dir(p.DataRoot), "BibitesMultiverse-second")
	if p.CommandFile() == second.CommandFile() {
		t.Fatalf("two worlds share the command file %s", p.CommandFile())
	}
	// A leftover variable in the parent environment is replaced, not appended.
	merged := gameEnvironment([]string{"MULTIVERSE_WORLD=stale", "PATH=/bin"}, p)
	worlds := 0
	for _, entry := range merged {
		if strings.HasPrefix(entry, "MULTIVERSE_WORLD=") {
			worlds++
			if entry != "MULTIVERSE_WORLD=Multiverse" {
				t.Fatalf("the parent's MULTIVERSE_WORLD survived: %s", entry)
			}
		}
	}
	if worlds != 1 {
		t.Fatalf("MULTIVERSE_WORLD appears %d times in the game's environment", worlds)
	}
}

func TestStatusJSONSchema(t *testing.T) {
	h := newHarness(t)
	p := h.profile("default", "Multiverse", 8787)

	if code := h.run("status", "--all", "--json"); code != exitOK {
		t.Fatalf("status --all --json: %d\n%s", code, h.err())
	}
	want := fmt.Sprintf(`{
  "format": "bibites-multiverse/launcher-status/1",
  "release": "%s",
  "installRoot": "%s",
  "active": "default",
  "profiles": [
    {
      "name": "default",
      "active": true,
      "world": "Multiverse",
      "gameDir": "%s",
      "dataRoot": "%s",
      "sidecarPort": 8787,
      "headless": false,
      "exportEdges": "E,N,W,S",
      "excludeSpecies": "Basic bibite",
      "saveMinutes": 10,
      "saveKeep": 6,
      "saveOnQuit": true,
      "peerId": "public-9af42a17616742e7a6e8c62cb8b95f4f",
      "relayUrl": "%s",
      "createdUtc": "2026-08-16T09:41:07Z",
      "sidecar": {
        "pid": 0,
        "running": false
      },
      "game": {
        "pid": 0,
        "running": false
      }
    }
  ]
}
`, Release, h.root, p.GameDir, p.DataRoot, testRelayURL)
	if h.out() != want {
		t.Fatalf("the status JSON drifted.\n got:\n%s\nwant:\n%s", h.out(), want)
	}

	var parsed Status
	if err := json.Unmarshal([]byte(h.out()), &parsed); err != nil {
		t.Fatalf("the status is not valid JSON: %v", err)
	}
	if parsed.Format != StatusFormat || len(parsed.Profiles) != 1 {
		t.Fatalf("the status did not round-trip: %+v", parsed)
	}
	// The credential's own bytes must be nowhere in it.
	mustNotContain(t, "the status JSON", h.out(), strings.Repeat("a", 64))
	mustNotContain(t, "the status JSON", h.out(), "secret")
}

// TestStatusJSONReportsWhatItCouldNotRead: the machine-readable form has to fail
// the way the human one does. `status --all --json` answered
// {"active": "", "profiles": []} and exit 0 while a world was running - because
// the profile behind it would not load - so anything watching that output read
// "nothing is installed here" from a launcher that simply could not read its own
// files. The human form exited 1 and said why.
func TestStatusJSONReportsWhatItCouldNotRead(t *testing.T) {
	h := newHarness(t)
	h.writeRawProfile("default", "{ this is not json")

	if code := h.run("status", "--all", "--json"); code != exitRefused {
		t.Fatalf("status --all --json exited %d, want %d\n%s", code, exitRefused, h.out())
	}
	mustContain(t, "the JSON", h.out(), `"problems"`)
	mustContain(t, "the JSON", h.out(), "default.json")
	mustContain(t, "the JSON", h.out(), "no world profiles yet")
	// The two forms agree on the exit code, which is the whole point.
	if code := h.run("status", "--all"); code != exitRefused {
		t.Fatalf("the human form exited %d, want %d", code, exitRefused)
	}
	// 'profile list' answers the same way.
	if code := h.run("profile", "list", "--json"); code != exitRefused {
		t.Fatalf("profile list --json exited %d, want %d", code, exitRefused)
	}

	// A readable world BESIDE the broken file reports normally and exits 0 - one
	// stray file must not hide the worlds that do parse - and still names it.
	h.profile("second", "Second", 8788)
	if code := h.run("status", "--all", "--json"); code != exitOK {
		t.Fatalf("status --all --json exited %d, want %d\n%s", code, exitOK, h.err())
	}
	var parsed Status
	if err := json.Unmarshal([]byte(h.out()), &parsed); err != nil {
		t.Fatalf("the status is not valid JSON: %v", err)
	}
	if len(parsed.Profiles) != 1 {
		t.Fatalf("the report holds %d worlds, want 1", len(parsed.Profiles))
	}
	if len(parsed.Problems) != 1 {
		t.Fatalf("the report holds %d problems, want 1: %v", len(parsed.Problems), parsed.Problems)
	}
	mustContain(t, "the problem", parsed.Problems[0], "default.json")
	// The human form prints the same fact.
	if code := h.run("status", "--all"); code != exitOK {
		t.Fatalf("the human form exited %d\n%s", code, h.err())
	}
	mustContain(t, "the human status", h.out(), "a world could not be read")
}

// TestMenuScript drives the console menu the way a person does: an empty line
// selects Start, and 0 quits.
func TestMenuScript(t *testing.T) {
	h := newHarness(t)
	// A port nothing holds, so the one refusal this world meets is the one the
	// test is about rather than whatever else this machine is running.
	port := freeTestPort(t)
	p := h.profile("default", "Multiverse", port)
	// The world has no credential, so the Start selection refuses immediately
	// and the menu is drawn again. That refusal is the proof the empty line
	// selected Start.
	removeFile(t, p.CredentialFile())

	h.stdin = strings.NewReader("\n3\n0\n")
	if code := h.run(); code != exitOK {
		t.Fatalf("the menu exited %d\n%s", code, h.err())
	}
	out := h.out()

	frame := fmt.Sprintf(`Bibites Multiverse launcher %s
   profile 'default'   world 'Multiverse'   port %d   headless off
   sidecar stopped                  game stopped

   1) Start this world            [Enter]
   2) Stop this world
   3) Status of every world
   4) Choose another world
   5) Edit this world's settings
   6) Create another world
   7) Delete a world
   8) Open this world's log folder
   0) Quit
select: `, Release, port)
	if !strings.HasPrefix(out, frame) {
		t.Fatalf("the first menu frame drifted.\n got:\n%s\nwant it to start with:\n%s", out, frame)
	}
	if got := strings.Count(out, "   1) Start this world            [Enter]"); got != 3 {
		t.Fatalf("the menu was drawn %d times, want 3", got)
	}
	// The empty line selected Start, which refused for the one reason it had.
	mustContain(t, "stderr", h.err(), "there is no credential at "+p.CredentialFile())
	// '3' printed the status of every world.
	mustContain(t, "the menu output", out, "installed in "+h.root)
	mustContain(t, "the menu output", out,
		fmt.Sprintf("* default   world 'Multiverse'   port %d", port))
}

// TestMenuIsNotOpenedWithoutATerminal is the other half of the same rule: a
// program that blocks on a menu nobody can answer is a program that hangs.
func TestMenuIsNotOpenedWithoutATerminal(t *testing.T) {
	h := newHarness(t)
	h.profile("default", "Multiverse", 8787)
	h.stdin = notATerminal(t)
	if code := h.run(); code != exitOK {
		t.Fatalf("the bare command exited %d\n%s", code, h.err())
	}
	mustContain(t, "the output", h.out(), "installed in "+h.root)
	mustContain(t, "the output", h.out(), "Bibites Multiverse launcher "+Release)
	mustContain(t, "the output", h.out(), "Commands:")
	mustNotContain(t, "the output", h.out(), "select:")
}

func TestUsageAndExitCodes(t *testing.T) {
	h := newHarness(t)
	h.profile("default", "Multiverse", 8787)

	if code := h.run("--help"); code != exitOK {
		t.Fatalf("--help exited %d", code)
	}
	mustContain(t, "the usage", h.out(), LauncherExeName+" [global flags] [command] [args]")

	if code := h.run("version"); code != exitOK {
		t.Fatalf("version exited %d", code)
	}
	mustContain(t, "version", h.out(), Release)

	if code := h.run("nonsense"); code != exitUsage {
		t.Fatalf("an unknown command exited %d, want %d", code, exitUsage)
	}
	if code := h.run("start", "one", "two"); code != exitUsage {
		t.Fatalf("two world names exited %d, want %d", code, exitUsage)
	}
	if code := h.run("status", "--profile", "missing"); code != exitRefused {
		t.Fatalf("an unknown world exited %d, want %d", code, exitRefused)
	}
	// A global flag is accepted after the command name too.
	if code := h.run("status", "--all", "--json"); code != exitOK {
		t.Fatalf("status --all --json exited %d\n%s", code, h.err())
	}
}

// The launcher asks the sidecar's own-slot endpoint whether the game's mod has
// arrived, and it spells that path itself rather than importing the sidecar into
// the launcher binary. This is the seam that keeps the two spellings equal.
func TestOwnSlotPathMatchesTheSidecar(t *testing.T) {
	if ownSlotPath != sidecar.OwnSlotPath {
		t.Fatalf("the launcher asks for %q and the sidecar serves %q",
			ownSlotPath, sidecar.OwnSlotPath)
	}
}
