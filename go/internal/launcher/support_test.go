package launcher

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestMain lets the test binary stand in for the sidecar and for the game.
//
// This is not a convenience: the launcher now identifies a recorded process by
// its IMAGE before it signals it, so a fake that is a /bin/sh script — whose
// /proc/<pid>/exe is the shell or whatever it exec'd — would be correctly
// judged "not the game". A copy of this binary placed at
// <install>/multiverse-sidecar or <gameDir>/The Bibites.x86_64 is identified
// exactly as the real ones are.
func TestMain(m *testing.M) {
	switch filepath.Base(os.Args[0]) {
	case sidecarExeName:
		os.Exit(fakeSidecarMain())
	case gameExeName:
		os.Exit(fakeGameMain())
	}
	os.Exit(m.Run())
}

// fakeSidecarMain writes what the slot wait reads, serves the own-slot endpoint
// the mod-connected wait reads, then sits there until it is stopped. Its stderr
// is the log file the launcher redirected.
func fakeSidecarMain() int {
	mode := os.Getenv("LAUNCHER_FAKE_SIDECAR")
	switch mode {
	case "refuse":
		fmt.Fprintln(os.Stderr, "sidecar: placement claim refused: another peer holds that slot")
	case "silent":
		// Say nothing, so the wait runs to its deadline.
	default:
		go func() {
			time.Sleep(100 * time.Millisecond)
			fmt.Fprintln(os.Stderr, "sidecar: contract B: slot granted slot=3 position=0,0")
		}()
	}
	// The real sidecar binds the Contract A listener it was given and serves
	// /my-slot on it (go/internal/sidecar/ownslot.go). "nomod" is the world whose
	// game never reaches the mod — the LOCAL-CONFIGRACE shape.
	if addr := fakeListenArg(); addr != "" && mode != "nolisten" {
		go serveFakeOwnSlot(addr, mode != "nomod")
	}
	time.Sleep(2 * time.Minute)
	return 0
}

// fakeListenArg reads --listen off this fake's own command line, exactly as the
// launcher wrote it in sidecarArgs.
func fakeListenArg() string {
	for i, arg := range os.Args {
		if arg == "--listen" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

// serveFakeOwnSlot answers the one endpoint the launcher's mod wait reads, with
// the one field it reads from it.
func serveFakeOwnSlot(addr string, connected bool) {
	mux := http.NewServeMux()
	mux.HandleFunc("/my-slot", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"schema":"multiverse-own-slot/1","mod":{"connected":%t,"modVersion":"9.9.9"}}`,
			connected)
	})
	server := &http.Server{Addr: addr, Handler: mux}
	server.ListenAndServe()
}

// fakeGameMain stands in for a Unity instance: it runs until it is asked to
// stop. In the world called deafWorld it stands in for one that was asked and
// did NOT go — the timeout path, which is a different sentence to the person
// watching and a different amount of lost world.
//
// IT ANNOUNCES ITSELF IN THE WORLD'S DATA DIRECTORY, and waitForFakeGame is how
// a test waits for that. A process that has not finished starting has not
// installed its signal handling either, and Unity is far slower to get there
// than this is: a stop delivered into that window kills a game that would have
// closed, and a test that does it is testing the window rather than the stop.
func fakeGameMain() int {
	world := os.Getenv("MULTIVERSE_WORLD")
	if world == deafWorld {
		signal.Ignore(syscall.SIGTERM)
	}
	if cmdFile := os.Getenv(cmdFileEnvName); cmdFile != "" &&
		world != deafWorld && world != modlessWorld {
		go fakeCommandPump(cmdFile, world)
	}
	if token := os.Getenv("MULTIVERSE_CONTRACT_A_TOKEN_FILE"); token != "" {
		os.WriteFile(filepath.Join(filepath.Dir(token), fakeGameReadyName), nil, 0o644)
	}
	time.Sleep(2 * time.Minute)
	return 0
}

// fakeCommandPump is the MOD'S half of MULTIVERSE_CMD_FILE, written to
// bibites-mod/src/DevCommands.cs rather than to what the launcher happens to
// send: poll the path, ignore content that does not end in a newline, DELETE the
// file once it is taken, append '<token> OK|ERROR <details>' to <file>.log, and
// for 'quit' go a moment later — the mod's own half-second pause before
// Application.Quit(), which is what gives it time to write its answer.
func fakeCommandPump(path, world string) {
	for {
		time.Sleep(10 * time.Millisecond)
		raw, err := os.ReadFile(path)
		if err != nil || len(raw) == 0 || !strings.HasSuffix(string(raw), "\n") {
			continue
		}
		os.Remove(path)
		fields := strings.Fields(string(raw))
		if len(fields) < 2 {
			continue
		}
		token, verb := fields[0], fields[1]
		if world == mumWorld {
			continue
		}
		status, details := "OK", "quitting"
		if world == refusingWorld {
			status, details = "ERROR", "no world is loaded"
		}
		answers, err := os.OpenFile(path+cmdLogSuffix, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			continue
		}
		fmt.Fprintf(answers, "%s %s %s\n", token, status, details)
		answers.Close()
		if verb == modQuitVerb && status == "OK" {
			time.Sleep(20 * time.Millisecond)
			os.Exit(0)
		}
	}
}

// fakeGameReadyName is the fake game's "I am up" marker, in the world's own data
// directory, where the launcher already writes.
const fakeGameReadyName = "fake-game-ready"

// waitForFakeGame blocks until the game this world just started is really there.
func waitForFakeGame(t *testing.T, p Profile) {
	t.Helper()
	marker := filepath.Join(p.DataDir(), fakeGameReadyName)
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		if fileExists(marker) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the fake game never announced itself at %s", marker)
}

// harness is one fake installation: an install root with a sidecar and a
// public map, a game folder that looks like a game folder, and somewhere for
// the worlds' data to live. Every test drives the real entry point through it.
type harness struct {
	t          *testing.T
	root       string
	gameDir    string
	dataParent string
	stdout     bytes.Buffer
	stderr     bytes.Buffer
	stdin      io.Reader
	now        time.Time
	// nowStep advances the injected clock on every read. It is zero by default,
	// because the golden stamps need a frozen clock; a test that exercises a
	// TIMEOUT sets it, so the deadline arrives without anybody waiting for it.
	nowStep time.Duration
}

const testRelayURL = "wss://bibitesmultiverse.com/contract-b/v4"

// The save names that make the fake game behave like a world in some particular
// trouble. They travel in MULTIVERSE_WORLD, which the launcher sets for every
// world it starts, so a test picks a behaviour by naming its world.
//
// Any OTHER name gets a fake game that behaves like a healthy modded one: it
// answers the mod command file and quits when asked to (see fakeCommandPump).
const (
	// deafWorld reads no command file AND ignores SIGTERM: nothing can ask it to
	// go, so it drives the force-after-the-timeout path.
	deafWorld = "Deaf"
	// modlessWorld reads no command file and closes on SIGTERM, which is a world
	// started before the launcher set MULTIVERSE_CMD_FILE, or one whose mod never
	// loaded. It drives the fallback to the window.
	modlessWorld = "Modless"
	// mumWorld takes the command and never answers it.
	mumWorld = "Mum"
	// refusingWorld answers ERROR.
	refusingWorld = "Refusing"
)

func newHarness(t *testing.T) *harness {
	t.Helper()
	base := t.TempDir()
	h := &harness{
		t:          t,
		root:       filepath.Join(base, "install"),
		gameDir:    filepath.Join(base, "game"),
		dataParent: filepath.Join(base, "worlds"),
		now:        time.Date(2026, 8, 16, 9, 41, 7, 0, time.UTC),
	}
	mkdir(t, h.root)
	mkdir(t, h.dataParent)
	h.makeGameDir(h.gameDir)
	writeFile(t, filepath.Join(h.root, sidecarExeName), "a placeholder sidecar", 0o755)
	writeFile(t, h.install().PublicMapPath(),
		`{"format":"`+publicMapFormat+`","enrollmentUrl":"https://example.invalid/api/enroll",`+
			`"relayUrl":"`+testRelayURL+`"}`, 0o644)
	return h
}

func (h *harness) install() Install { return Install{Root: h.root} }

// makeGameDir builds the two files validateGameDir looks for.
func (h *harness) makeGameDir(dir string) {
	h.t.Helper()
	mkdir(h.t, dir)
	writeFile(h.t, filepath.Join(dir, gameExeName), "a placeholder game", 0o755)
	mkdir(h.t, filepath.Join(dir, "BepInEx", "plugins"))
	writeFile(h.t, filepath.Join(dir, filepath.FromSlash(pluginRelPath)), "not really a dll", 0o644)
}

// useRealFakes replaces the placeholders with copies of this test binary, which
// run as the sidecar and the game and can therefore be spawned, identified and
// stopped for real.
func (h *harness) useRealFakes() {
	h.t.Helper()
	self, err := os.Executable()
	if err != nil {
		h.t.Fatalf("os.Executable: %v", err)
	}
	copyExecutable(h.t, self, h.install().SidecarExe())
	copyExecutable(h.t, self, filepath.Join(h.gameDir, gameExeName))
}

func copyExecutable(t *testing.T, from, to string) {
	t.Helper()
	raw, err := os.ReadFile(from)
	if err != nil {
		t.Fatalf("read %s: %v", from, err)
	}
	if err := os.WriteFile(to, raw, 0o755); err != nil {
		t.Fatalf("write %s: %v", to, err)
	}
}

// profile writes a valid profile with the fixture's paths and returns it.
func (h *harness) profile(name, world string, port int) Profile {
	h.t.Helper()
	p := Profile{
		Format:         ProfileFormat,
		Name:           name,
		GameDir:        h.gameDir,
		DataRoot:       filepath.Join(h.dataParent, "BibitesMultiverse-"+name),
		SidecarPort:    port,
		World:          world,
		ExportEdges:    DefaultExportEdges,
		ExcludeSpecies: DefaultExcludeSpecies,
		SaveMinutes:    DefaultSaveMinutes,
		SaveKeep:       DefaultSaveKeep,
		SaveOnQuit:     true,
		PeerID:         "public-9af42a17616742e7a6e8c62cb8b95f4f",
		RelayURL:       testRelayURL,
		CreatedUTC:     "2026-08-16T09:41:07Z",
	}
	mkdir(h.t, p.DataRoot)
	writeFile(h.t, p.CredentialFile(), strings.Repeat("a", 64)+"\n", 0o600)
	if err := h.install().SaveProfile(p); err != nil {
		h.t.Fatalf("SaveProfile: %v", err)
	}
	if err := h.install().SetActive(name); err != nil {
		h.t.Fatalf("SetActive: %v", err)
	}
	return p
}

// managedProfile writes a COMPLETE EDITION world: the game lives INSIDE this
// world's own data root, at <dataRoot>/runtimes/<assembly sha256>, which is what
// release/kit/Install-BibitesMultiverse.ps1 produces and what the launcher used
// to refuse to read.
func (h *harness) managedProfile(name, world string, port int) Profile {
	h.t.Helper()
	p := h.profile(name, world, port)
	p.GameDir = filepath.Join(p.DataRoot, runtimesDirName,
		"12455E485199CDBCAEA5978B8B0095EEDCBDD09D1FB87EFD65CCACB15D96E7EE")
	h.makeGameDir(p.GameDir)
	if err := h.install().SaveProfile(p); err != nil {
		h.t.Fatalf("SaveProfile: %v", err)
	}
	return p
}

// writeRawProfile puts a profile file on disk without going through any
// validation, which is what a hand-edited or truncated file looks like.
func (h *harness) writeRawProfile(name, body string) {
	h.t.Helper()
	writeFile(h.t, h.install().ProfilePath(name), body, 0o644)
}

// run drives the real entry point with the fixture's install root.
func (h *harness) run(args ...string) int {
	h.t.Helper()
	h.stdout.Reset()
	h.stderr.Reset()
	stdin := h.stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	full := append([]string{"--install-root", h.root}, args...)
	return run(full, stdin, &h.stdout, &h.stderr,
		func(string) string { return "" },
		func() (string, error) { return filepath.Join(h.root, LauncherExeName), nil },
		h.clock)
}

// clock is the time the launcher sees: frozen unless a test asked it to move.
func (h *harness) clock() time.Time {
	now := h.now
	h.now = h.now.Add(h.nowStep)
	return now
}

// runWith drives the entry point with a scripted stdin, for the commands that
// ask a question.
func (h *harness) runWith(input string, args ...string) int {
	h.t.Helper()
	was := h.stdin
	h.stdin = strings.NewReader(input)
	defer func() { h.stdin = was }()
	return h.run(args...)
}

func (h *harness) out() string { return h.stdout.String() }
func (h *harness) err() string { return h.stderr.String() }

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", path, err)
	}
}

// freeTestPort asks the operating system for a port nothing is using, so a test
// that exercises the real bind probe does not depend on what else this machine
// happens to be running.
func freeTestPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

// notATerminal is a regular file, which the launcher must treat as "nobody is
// watching a menu".
func notATerminal(t *testing.T) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}

func mustContain(t *testing.T, what, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("%s does not contain %q:\n%s", what, want, text)
	}
}

func mustNotContain(t *testing.T, what, text, unwanted string) {
	t.Helper()
	if strings.Contains(text, unwanted) {
		t.Fatalf("%s contains %q and must not:\n%s", what, unwanted, text)
	}
}
