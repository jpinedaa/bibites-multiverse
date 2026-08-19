package launcher

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// A session is the window's whole access to this package, so what these tests
// hold it to is that it is the SAME code the console runs: the same refusals,
// the same gate before a delete, the same 'profile set' semantics. Anything a
// session did differently would be a second implementation of the launcher,
// which is the one thing it must not be.

// session is a launcher driven the way the graphical front door drives it: no
// terminal, and the fixture's writers.
func (h *harness) session() *Session {
	h.t.Helper()
	h.stdout.Reset()
	h.stderr.Reset()
	s, err := NewSession(SessionOptions{
		InstallRoot: h.root,
		Out:         &h.stdout,
		Err:         &h.stderr,
		Now:         h.clock,
		Getenv:      h.getenv,
		Executable:  func() (string, error) { return filepath.Join(h.root, LauncherExeName), nil },
	})
	if err != nil {
		h.t.Fatalf("NewSession: %v", err)
	}
	return s
}

func TestSessionSnapshotIsTheWholeInstallation(t *testing.T) {
	h := newHarness(t)
	first := h.profile("default", "Multiverse", 8787)
	h.profile("second", "Second", 8788)
	// 'default' stays the world the bare commands act on: h.profile sets the
	// active world to the last one it wrote.
	if err := h.install().SetActive("default"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	// A file in profiles\ that cannot be read must not hide the worlds that can.
	h.writeRawProfile("broken", "{ this is not json")

	snap := h.session().Snapshot()
	if snap.Release != Release || snap.InstallRoot != h.root {
		t.Fatalf("the snapshot names %s in %s", snap.Release, snap.InstallRoot)
	}
	if len(snap.Worlds) != 2 {
		t.Fatalf("the snapshot holds %d worlds, want 2: %+v", len(snap.Worlds), snap.Worlds)
	}
	if snap.Active != "default" || !snap.Worlds[0].Active || snap.Worlds[1].Active {
		t.Fatalf("the active world is %q and the flags are %t/%t",
			snap.Active, snap.Worlds[0].Active, snap.Worlds[1].Active)
	}
	if len(snap.Problems) != 1 || !strings.Contains(snap.Problems[0], "broken.json") {
		t.Fatalf("the snapshot's problems are %v; the unreadable file must be one of them",
			snap.Problems)
	}
	if snap.Worlds[0].World != first.World || snap.Worlds[0].SidecarPort != first.SidecarPort {
		t.Fatalf("the first world reads %+v", snap.Worlds[0])
	}
	// Nothing is running, so nothing was asked and nothing is claimed: an
	// unanswered endpoint is not a game that is not connected.
	if snap.Worlds[0].Mod.Answered || snap.Worlds[0].Mod.Connected {
		t.Fatalf("a world with no sidecar reported a mod: %+v", snap.Worlds[0].Mod)
	}
	if snap.Worlds[0].Sidecar.Running || snap.Worlds[0].Game.Running {
		t.Fatal("a world with no processes reported one running")
	}
	// A READING NEVER PRINTS. The window draws the snapshot; a snapshot that
	// wrote to the log pane would fill it every two seconds.
	if h.out() != "" || h.err() != "" {
		t.Fatalf("Snapshot printed:\nout: %q\nerr: %q", h.out(), h.err())
	}
	// And it never writes: the pid ledger is reported, never repaired.
	if fileExists(first.SidecarPidFile()) {
		t.Fatal("Snapshot created a pid file")
	}
}

// The world list's mod, speed and slot columns come from the sidecar's own
// read-only endpoint, and this is the decoding of it: every absence is a value.
func TestSessionReadsTheOwnSlotDocument(t *testing.T) {
	const body = `{
	  "schema": "multiverse-own-slot/1",
	  "mod": {"connected": true, "modVersion": "0.6.7", "timeScale": 10, "achievedTimeScale": 6.5,
	          "population": 412},
	  "slot": {"slot": 3, "live": true},
	  "relay": {"connected": true}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ownSlotPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	view, ok := fetchOwnSlot(server.Client(), portOf(t, server.URL))
	if !ok {
		t.Fatal("the own-slot document was not read")
	}
	mod := view.modView()
	if !mod.Answered || !mod.Connected || mod.Version != "0.6.7" {
		t.Fatalf("the mod reads %+v", mod)
	}
	if mod.TimeScale != 10 || mod.Achieved != 6.5 {
		t.Fatalf("the speed reads target %g achieved %g", mod.TimeScale, mod.Achieved)
	}
	if mod.Slot != 3 || !mod.SlotKnown || !mod.Live || !mod.RelayConnected {
		t.Fatalf("the slot reads %+v", mod)
	}
	if mod.Population != 412 || !mod.PopulationSaid {
		t.Fatalf("the population reads %d (said %t)", mod.Population, mod.PopulationSaid)
	}

	// A sidecar that says nothing about the mod is a sidecar whose game has not
	// arrived, and the fields it did not send stay unsaid rather than becoming
	// zeroes somebody could read as a measurement.
	quiet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"schema":"multiverse-own-slot/1","mod":{"connected":false}}`)
	}))
	defer quiet.Close()
	view, ok = fetchOwnSlot(quiet.Client(), portOf(t, quiet.URL))
	if !ok {
		t.Fatal("a document with fewer fields was not read")
	}
	mod = view.modView()
	if !mod.Answered || mod.Connected || mod.PopulationSaid || mod.SlotKnown || mod.TimeScale != 0 {
		t.Fatalf("an unsaid field became a value: %+v", mod)
	}

	// Every failure is "not yet": nothing here is fatal, because this is a
	// reading taken every couple of seconds and not a check.
	if _, ok := fetchOwnSlot(server.Client(), freeTestPort(t)); ok {
		t.Fatal("a port with no listener answered")
	}
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json at all")
	}))
	defer broken.Close()
	if _, ok := fetchOwnSlot(broken.Client(), portOf(t, broken.URL)); ok {
		t.Fatal("a body that is not the document answered")
	}
}

// The window's own-slot reading and the start sequence's mod wait read ONE
// document through one function, so a sidecar that changes the shape of it
// cannot leave the two disagreeing.
func TestModWaitAndTheWorldListShareOneReader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"mod":{"connected":true,"modVersion":"9.9.9"}}`)
	}))
	defer server.Close()
	port := portOf(t, server.URL)
	version, connected := probeModConnected(server.Client(), port)
	if !connected || version != "9.9.9" {
		t.Fatalf("the start sequence read %q/%t", version, connected)
	}
	view, ok := fetchOwnSlot(server.Client(), port)
	if !ok || view.Mod.ModVersion != version || view.Mod.Connected != connected {
		t.Fatalf("the two readers disagree: %+v", view)
	}
}

// THE GATE DID NOT MOVE. A dialog collects the typed name; the comparison that
// decides whether a folder is deleted is still the core's, so a front door that
// asked the wrong question, or did not ask at all, deletes nothing.
func TestSessionDeleteIsGatedByTheCoreItself(t *testing.T) {
	h := newHarness(t)
	p := h.profile("default", "Multiverse", 8787)
	writeFile(t, filepath.Join(p.DataDir(), "journal.log"), "custody", 0o644)
	writeFile(t, filepath.Join(p.DataRoot, "somebody-elses.txt"), "not this world's", 0o644)

	s := h.session()
	if code := s.Delete("default", true, "not the name"); code != exitRefused {
		t.Fatalf("a wrong name exited %d, want %d", code, exitRefused)
	}
	mustContain(t, "the refusal", h.err(), "that is not 'default'. Nothing was deleted")
	if !fileExists(h.install().ProfilePath("default")) {
		t.Fatal("a refused delete removed the profile")
	}

	// An unanswered question is a NO, which is what the default answer of a
	// session means: nothing was typed, so nothing is deleted.
	s = h.session()
	if code := s.Delete("default", true, ""); code != exitRefused {
		t.Fatalf("an empty name exited %d, want %d", code, exitRefused)
	}
	if !fileExists(filepath.Join(p.DataDir(), "journal.log")) {
		t.Fatal("a refused delete removed the journal")
	}

	// The right name deletes this world's own entries and NOTHING ELSE, and says
	// what it left where it is.
	s = h.session()
	if code := s.Delete("default", true, "DEFAULT"); code != exitOK {
		t.Fatalf("the delete exited %d\n%s\n%s", code, h.out(), h.err())
	}
	if fileExists(h.install().ProfilePath("default")) {
		t.Fatal("the profile survived its own delete")
	}
	if dirExists(p.DataDir()) {
		t.Fatal("the world's data directory survived --remove-world-data")
	}
	if !fileExists(filepath.Join(p.DataRoot, "somebody-elses.txt")) {
		t.Fatal("the delete took a file that is not this world's")
	}
	mustContain(t, "the delete output", h.out(), "somebody-elses.txt")
	// The custody warning is said, the same as on the command line.
	mustContain(t, "the delete output", h.out(), "Deleting this world here is NOT leaving the map")

	// A canned answer never outlives the action it was collected for.
	if answer, ok := s.a.prompt("anything at all: "); ok || answer != "" {
		t.Fatalf("the session still answers questions with %q/%t", answer, ok)
	}
}

// An edit is 'profile set': the flags a dialog filled in, the core's own
// validation, and the core's own message when a value is refused.
func TestSessionEditIsProfileSet(t *testing.T) {
	h := newHarness(t)
	h.profile("default", "Multiverse", 8787)
	s := h.session()

	if code := s.Edit("default", []string{"--sidecar-port", "8790", "--save-minutes", "5"}); code != exitOK {
		t.Fatalf("the edit exited %d\n%s", code, h.err())
	}
	after, err := s.World("default")
	if err != nil {
		t.Fatalf("World: %v", err)
	}
	if after.SidecarPort != 8790 || after.SaveMinutes != 5 {
		t.Fatalf("the edit wrote %+v", after)
	}
	// A field no flag named is untouched, which is what makes this a change
	// rather than a rewrite.
	if after.World != "Multiverse" || after.ExportEdges != DefaultExportEdges {
		t.Fatalf("the edit rewrote a field it was not given: %+v", after)
	}

	s = h.session()
	if code := s.Edit("default", []string{"--sidecar-port", "70000"}); code != exitRefused {
		t.Fatalf("an impossible port exited %d, want %d", code, exitRefused)
	}
	mustContain(t, "the refusal", h.err(), "is outside 1024-65535")

	s = h.session()
	if code := s.Edit("default", nil); code != exitUsage {
		t.Fatalf("an edit with no change exited %d, want %d", code, exitUsage)
	}
}

// What a create dialog opens with: the game folder this installation already
// uses, a data folder beside the world it already has, and a port nothing holds.
func TestSessionNewWorldDefaults(t *testing.T) {
	h := newHarness(t)
	first := h.profile("default", "Multiverse", DefaultSidecarPort)
	spec, err := h.session().NewWorldDefaults("second")
	if err != nil {
		t.Fatalf("NewWorldDefaults: %v", err)
	}
	if spec.Name != "second" || spec.World != "second" {
		t.Fatalf("the defaults name %+v", spec)
	}
	if spec.Port != DefaultSidecarPort+1 {
		t.Fatalf("the offered port is %d, want the first one nothing holds", spec.Port)
	}
	if spec.GameDir != first.GameDir {
		t.Fatalf("the new world's game folder is %s", spec.GameDir)
	}
	if spec.DataRoot != filepath.Join(filepath.Dir(first.DataRoot), "BibitesMultiverse-second") {
		t.Fatalf("the new world's data folder is %s", spec.DataRoot)
	}

	// An installation with nothing in it says what to pass rather than offering
	// a folder it does not know.
	empty := newHarness(t)
	if _, err := empty.session().NewWorldDefaults("first"); err == nil {
		t.Fatal("an installation with no world offered defaults anyway")
	}
}

// Creating a world through a session enrolls a NEW identity the way the console
// does — the same request, the same refusals — and writes the profile only after
// the map has answered.
func TestSessionCreateEnrollsOnceAndWritesTheProfile(t *testing.T) {
	h := newHarness(t)
	base := h.profile("default", "Multiverse", 8787)

	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body enrollmentRequest
		json.NewDecoder(r.Body).Decode(&body)
		if body.Release != Release {
			t.Errorf("the request names release %q", body.Release)
		}
		peer := "public-" + strings.ReplaceAll(strings.ToLower(body.InstallID), "-", "")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"format":"%s","relayUrl":"%s","peerId":"%s","created":true}`,
			enrollResponseFormat, testRelayURL, peer)
	}))
	defer server.Close()
	writeFile(t, h.install().PublicMapPath(),
		`{"format":"`+publicMapFormat+`","enrollmentUrl":"`+server.URL+
			`","relayUrl":"`+testRelayURL+`"}`, 0o644)

	s := h.session()
	s.a.client = server.Client()
	spec, err := s.NewWorldDefaults("second")
	if err != nil {
		t.Fatalf("NewWorldDefaults: %v", err)
	}
	spec.World = "Second"
	spec.Headless = true
	if code := s.Create(spec); code != exitOK {
		t.Fatalf("the create exited %d\n%s\n%s", code, h.out(), h.err())
	}
	if requests != 1 {
		t.Fatalf("the map was asked %d times", requests)
	}
	created, err := s.World("second")
	if err != nil {
		t.Fatalf("the new world does not load: %v", err)
	}
	if created.World != "Second" || !created.Headless || created.SidecarPort != 8788 {
		t.Fatalf("the new world reads %+v", created)
	}
	if created.GameDir != base.GameDir || created.RelayURL != testRelayURL {
		t.Fatalf("the new world's map or game folder is wrong: %+v", created)
	}
	if !strings.HasPrefix(created.PeerID, "public-") {
		t.Fatalf("the new world's identity is %q", created.PeerID)
	}
	// The secret half is where it belongs, and the pending record is gone
	// because the profile exists.
	if !fileExists(created.CredentialFile()) {
		t.Fatalf("no credential at %s", created.CredentialFile())
	}
	if fileExists(created.PendingFile()) {
		t.Fatalf("the pending record survived a successful enrollment")
	}
	// The notes the console prints after a create are said here too.
	mustContain(t, "the create output", h.out(), "per-address enrollment limit")

	// A second world with the same name is refused before anything is contacted.
	s = h.session()
	s.a.client = server.Client()
	if code := s.Create(spec); code != exitRefused {
		t.Fatalf("a duplicate name exited %d, want %d", code, exitRefused)
	}
	if requests != 1 {
		t.Fatalf("a refused create still contacted the map (%d requests)", requests)
	}
}

// A session refuses what the command line refuses, and says the same thing: it
// is the same code, reached without a terminal.
func TestSessionStartRefusesTheSameWayTheCommandDoes(t *testing.T) {
	h := newHarness(t)
	p := h.profile("default", "Multiverse", freeTestPort(t))
	removeFile(t, p.CredentialFile())

	s := h.session()
	if code := s.Start("default", StartOptions{}); code != exitRefused {
		t.Fatalf("a world with no credential started anyway (%d)", code)
	}
	mustContain(t, "the refusal", h.err(), "there is no credential at "+p.CredentialFile())

	if code := s.Start("nowhere", StartOptions{}); code != exitRefused {
		t.Fatalf("an unknown world exited %d, want %d", code, exitRefused)
	}
	mustContain(t, "the refusal", h.err(), "there is no world called 'nowhere' here")
}

// THE WINDOW'S "Check this world" HANDS THE DIAGNOSTIC THE WORLD'S OWN MAP.
// It ran with the local folders alone once, which left the sidecar on its
// default relay and reported a healthy world as two failures; the command line
// this session prints is the one thing a person can compare against the flags
// they would have typed, so it is asserted here as well as in diagnoseArgs.
func TestSessionDiagnosePassesTheRelayAndTheCredential(t *testing.T) {
	h := newHarness(t)
	p := h.profile("default", "Multiverse", 8787)

	s := h.session()
	// The placeholder sidecar is not a program, so the run itself fails; the
	// command line has already been printed by then, which is what is under test.
	s.Diagnose("default")

	line := h.out()
	mustContain(t, "the diagnostic's command line", line, "--diagnose")
	mustContain(t, "the diagnostic's command line", line, "--relay "+testRelayURL)
	mustContain(t, "the diagnostic's command line", line, "--data-dir "+p.DataDir())
	mustContain(t, "the diagnostic's command line", line, "--credential-file "+p.CredentialFile())
	mustContain(t, "the diagnostic's command line", line, "--game-dir "+p.GameDir)
}

// The diagnostic is the sidecar's, and an installation without one says so
// rather than reporting nothing.
func TestSessionDiagnoseNeedsTheSidecar(t *testing.T) {
	h := newHarness(t)
	h.profile("default", "Multiverse", 8787)
	removeFile(t, h.install().SidecarExe())
	s := h.session()
	if code := s.Diagnose("default"); code != exitRefused {
		t.Fatalf("a missing sidecar exited %d, want %d", code, exitRefused)
	}
	mustContain(t, "the refusal", h.err(), "Re-run the installer")
}

// portOf reads the port out of a test server's URL, because everything the
// launcher asks on loopback it asks by port.
func portOf(t *testing.T, rawURL string) int {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %s: %v", rawURL, err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("%s has no port: %v", rawURL, err)
	}
	return port
}
