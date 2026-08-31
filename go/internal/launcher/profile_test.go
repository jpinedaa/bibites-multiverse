package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiverse/internal/contracta"
)

// profileKeyOrder is the frozen JSON key order. The Windows installer writes
// the same object with ConvertTo-Json, and a drift on either side is a profile
// one of the two writers cannot read.
//
// keeper and worldName are LAST because they were added last: a new key goes on
// the end so that every key already here keeps the position both writers put it
// in. They are written whether or not they hold anything — an unset public name
// is an empty string in both writers, not an absent key, or these two lists stop
// being the same list.
var profileKeyOrder = []string{
	"format", "name", "gameDir", "dataRoot", "sidecarPort", "world", "headless",
	"exportEdges", "excludeSpecies", "saveMinutes", "saveKeep", "saveOnQuit",
	"peerId", "relayUrl", "createdUtc", "keeper", "worldName",
}

func TestProfileRoundTrip(t *testing.T) {
	h := newHarness(t)
	want := h.profile("default", "Multiverse", 8787)

	got, err := h.install().LoadProfile("default")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if got != want {
		t.Fatalf("round trip changed the profile:\n got %+v\nwant %+v", got, want)
	}

	raw := readFile(t, h.install().ProfilePath("default"))
	if !strings.HasSuffix(raw, "\n") {
		t.Fatalf("the profile file does not end with a newline:\n%q", raw)
	}
	if keys := jsonKeys(t, raw); !equalStrings(keys, profileKeyOrder) {
		t.Fatalf("key order is %v, want %v", keys, profileKeyOrder)
	}

	// The keys are what matters, not their order in the file: a profile written
	// with the keys shuffled must still load.
	shuffled := `{"createdUtc":"2026-08-16T09:41:07Z","name":"default","format":"` + ProfileFormat + `"}`
	var partial Profile
	if err := json.Unmarshal([]byte(shuffled), &partial); err != nil {
		t.Fatalf("unmarshal shuffled: %v", err)
	}
	if partial.Name != "default" || partial.CreatedUTC != "2026-08-16T09:41:07Z" {
		t.Fatalf("shuffled keys did not load: %+v", partial)
	}
}

// TestInstallerWrittenProfileParses is the anti-drift pin between the launcher
// and release/kit/Install-BibitesMultiverse.ps1: each file in testdata is what
// that script's ConvertTo-Json emits for a reference install.
//
// BOTH EDITIONS ARE HERE, and the second one is the whole reason this test is
// worth having. The add-on edition binds to a game the participant already owns,
// so its gameDir is somewhere else entirely; the COMPLETE edition copies the game
// into <data root>\runtimes\<assembly sha256>, so its gameDir is INSIDE its own
// dataRoot. Only the add-on layout was pinned here, so a rule that refused a
// dataRoot overlapping its own game folder passed every test and then refused
// every complete-edition install on a real machine: 'profile list' answered
// "this installation has no world profiles yet" and the desktop icon opened a
// launcher that could do nothing.
func TestInstallerWrittenProfileParses(t *testing.T) {
	const completeRuntime = `C:\Users\alice\AppData\Local\BibitesMultiverse\runtimes\` +
		"12455E485199CDBCAEA5978B8B0095EEDCBDD09D1FB87EFD65CCACB15D96E7EE"

	cases := []struct {
		name     string
		file     string
		gameDir  string
		dataRoot string
	}{
		{
			name:     "the add-on edition, bound to a game already installed",
			file:     "installer-default-profile.json",
			gameDir:  `C:\Program Files (x86)\Steam\steamapps\common\The Bibites`,
			dataRoot: `C:\Users\alice\AppData\Local\BibitesMultiverse`,
		},
		{
			name:     "the complete edition, whose game lives under its own data root",
			file:     "installer-complete-profile.json",
			gameDir:  completeRuntime,
			dataRoot: `C:\Users\alice\AppData\Local\BibitesMultiverse`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", test.file))
			if err != nil {
				t.Fatalf("read testdata: %v", err)
			}

			// It has to load through the same path a real profile does, file name
			// agreement included.
			install := Install{Root: t.TempDir()}
			if err := os.MkdirAll(install.ProfilesDir(), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(install.ProfilePath("default"), raw, 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			p, err := install.LoadProfile("default")
			if err != nil {
				t.Fatalf("LoadProfile: %v", err)
			}

			if keys := jsonKeys(t, string(raw)); !equalStrings(keys, profileKeyOrder) {
				t.Fatalf("the installer's key order is %v, want %v", keys, profileKeyOrder)
			}

			want := Profile{
				Format:         "bibites-multiverse/launcher-profile/1",
				Name:           "default",
				GameDir:        test.gameDir,
				DataRoot:       test.dataRoot,
				SidecarPort:    8787,
				World:          "Multiverse",
				Headless:       false,
				ExportEdges:    "E,N,W,S",
				ExcludeSpecies: "Basic bibite",
				SaveMinutes:    10,
				SaveKeep:       6,
				SaveOnQuit:     true,
				PeerID:         "public-9af42a17616742e7a6e8c62cb8b95f4f",
				RelayURL:       "wss://bibitesmultiverse.com/contract-b/v4",
				CreatedUTC:     "2026-08-16T09:41:07Z",
				// THE REFERENCE INSTALL PUBLISHED NEITHER, which is the answer a
				// silent install and a participant who declined both leave behind.
				// It is pinned as the empty string rather than left out, because
				// the whole point of these two fields is that nothing fills one in.
				Keeper:    "",
				WorldName: "",
			}
			if p != want {
				t.Fatalf("the installer's profile did not parse as expected:\n got %+v\nwant %+v", p, want)
			}

			// Every value's JSON TYPE matters as much as its value: a port written
			// as a string would parse into a different program and fail here.
			var generic map[string]any
			if err := json.Unmarshal(raw, &generic); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			wantTypes := map[string]string{
				"format": "string", "name": "string", "gameDir": "string", "dataRoot": "string",
				"sidecarPort": "number", "world": "string", "headless": "bool",
				"exportEdges": "string", "excludeSpecies": "string", "saveMinutes": "number",
				"saveKeep": "number", "saveOnQuit": "bool", "peerId": "string",
				"relayUrl": "string", "createdUtc": "string",
				"keeper": "string", "worldName": "string",
			}
			for key, want := range wantTypes {
				value, found := generic[key]
				if !found {
					t.Fatalf("the installer's profile has no %q", key)
				}
				if got := jsonTypeName(value); got != want {
					t.Fatalf("%q is a %s, want a %s", key, got, want)
				}
			}
			if len(generic) != len(wantTypes) {
				t.Fatalf("the installer's profile has %d keys, want %d", len(generic), len(wantTypes))
			}
			if !relayURLPattern.MatchString(p.RelayURL) {
				t.Fatalf("the installer's relayUrl is not a wss:// URL: %s", p.RelayURL)
			}
			if !strings.HasPrefix(p.PeerID, "public-") {
				t.Fatalf("the installer's peerId is not a public-map identity: %s", p.PeerID)
			}

			// The whole profile has to survive the writer's rules too, not only the
			// reader's: 'profile set' re-validates everything it writes back.
			if err := validateProfilePaths(p, install); err != nil {
				t.Fatalf("validateProfilePaths refused the installer's own profile: %v", err)
			}
		})
	}

	// And the exception is EXACTLY the installer's layout: the same data root with
	// a game folder anywhere else inside it is still the hazard it always was.
	complete, err := os.ReadFile(filepath.Join("testdata", "installer-complete-profile.json"))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	for _, gameDir := range []string{
		`C:\Users\alice\AppData\Local\BibitesMultiverse`,
		`C:\Users\alice\AppData\Local\BibitesMultiverse\runtimes`,
		`C:\Users\alice\AppData\Local\BibitesMultiverse\data`,
		`C:\Users\alice\AppData\Local\BibitesMultiverse\game`,
	} {
		install := Install{Root: t.TempDir()}
		if err := os.MkdirAll(install.ProfilesDir(), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		var generic map[string]any
		if err := json.Unmarshal(complete, &generic); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		generic["gameDir"] = gameDir
		body, err := json.Marshal(generic)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := os.WriteFile(install.ProfilePath("default"), body, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := install.LoadProfile("default"); err == nil {
			t.Fatalf("a gameDir of %q was accepted inside its own data root", gameDir)
		}
	}
}

func TestProfileJSONNeverContainsSecret(t *testing.T) {
	const secret = "5f3a9c1d5f3a9c1d5f3a9c1d5f3a9c1d5f3a9c1d5f3a9c1d5f3a9c1d5f3a9c1d"
	h := newHarness(t)
	p := h.profile("default", "Multiverse", 8787)
	writeFile(t, p.CredentialFile(), secret+"\n", 0o600)

	marshalled, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	mustNotContain(t, "the marshalled profile", string(marshalled), secret)
	mustNotContain(t, "the profile on disk", readFile(t, h.install().ProfilePath("default")), secret)

	// The whole status surface is held to the same rule.
	if code := h.run("status", "--all", "--json"); code != exitOK {
		t.Fatalf("status: %d %s", code, h.err())
	}
	mustNotContain(t, "the status JSON", h.out(), secret)
}

func TestDefaultPortMatchesContractA(t *testing.T) {
	if DefaultSidecarPort != contracta.DefaultPort {
		t.Fatalf("the launcher offers port %d and Contract A defaults to %d",
			DefaultSidecarPort, contracta.DefaultPort)
	}
}

// TestReleaseConstantMatchesSupportMatrix pins the launcher's release to the
// one docs/support-matrix.md publishes, which release/make-release.sh also
// checks against its own.
func TestReleaseConstantMatchesSupportMatrix(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "support-matrix.md"))
	if err != nil {
		t.Fatalf("read the support matrix: %v", err)
	}
	const begin = "<!-- SUPPORT-MATRIX-JSON-BEGIN -->"
	const end = "<!-- SUPPORT-MATRIX-JSON-END -->"
	_, after, found := strings.Cut(string(raw), begin)
	if !found {
		t.Fatalf("the support matrix has no %s marker", begin)
	}
	block, _, found := strings.Cut(after, end)
	if !found {
		t.Fatalf("the support matrix has no %s marker", end)
	}
	block = strings.TrimSpace(block)
	block = strings.TrimPrefix(block, "```json")
	block = strings.TrimSuffix(strings.TrimSpace(block), "```")

	var matrix struct {
		Release string `json:"release"`
	}
	if err := json.Unmarshal([]byte(block), &matrix); err != nil {
		t.Fatalf("the support matrix block is not JSON: %v", err)
	}
	if matrix.Release != Release {
		t.Fatalf("docs/support-matrix.md publishes release %q and the launcher is %q",
			matrix.Release, Release)
	}
}

// jsonKeys returns the keys of a JSON object in the order they appear.
func jsonKeys(t *testing.T, text string) []string {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(text))
	token, err := decoder.Token()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		t.Fatalf("the document is not a JSON object")
	}
	var keys []string
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		key, ok := token.(string)
		if !ok {
			t.Fatalf("expected an object key, got %v", token)
		}
		keys = append(keys, key)
		var discard any
		if err := decoder.Decode(&discard); err != nil {
			t.Fatalf("decode the value of %q: %v", key, err)
		}
	}
	return keys
}

func jsonTypeName(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		return "other"
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------- the two public names

// TestPublicNamesAreOfferedAndNeverInvented is the whole of the consent rule
// this project made about the two strings a world publishes about the person
// running it (contract-b-m4.md §33 B49): a default is SHOWN and answered, and a
// path with nobody to show it to publishes nothing.
func TestPublicNamesAreOfferedAndNeverInvented(t *testing.T) {
	t.Run("a suggestion is only ever a suggestion", func(t *testing.T) {
		// The account name is reduced to the part a person would recognise, and
		// the world name is built from whatever keeper was settled on.
		for _, test := range []struct{ account, want string }{
			{`CORP\alice`, "alice"},
			{"/home/alice", "alice"},
			{"alice", "alice"},
			{"  ", ""},
			{strings.Repeat("x", MaxPublicNameBytes+1), ""},
		} {
			if got := handleFromAccountName(test.account); got != test.want {
				t.Fatalf("the handle offered for %q is %q, want %q", test.account, got, test.want)
			}
		}
		if got := suggestedWorldName("alice"); got != "alice's world" {
			t.Fatalf("the world name offered is %q", got)
		}
		// WITH NO KEEPER THERE IS NOTHING TO BUILD ONE FROM, and "'s world" is
		// not a name anybody chose.
		if got := suggestedWorldName(""); got != "" {
			t.Fatalf("a world name was offered with no keeper: %q", got)
		}
	})

	t.Run("the decline is the same character everywhere", func(t *testing.T) {
		for _, value := range []string{"", "   ", declineToken, " - "} {
			got, err := resolvePublicName(value)
			if err != nil || got != "" {
				t.Fatalf("%q resolved to (%q, %v), want (\"\", nil)", value, got, err)
			}
		}
		if got, err := resolvePublicName("  alice  "); err != nil || got != "alice" {
			t.Fatalf("a typed name resolved to (%q, %v)", got, err)
		}
		// The bound is the sidecar's, in BYTES, and it is refused rather than
		// clipped: a name published under somebody's own hand is not a value to
		// shorten on their behalf.
		if _, err := resolvePublicName(strings.Repeat("x", MaxPublicNameBytes+1)); err == nil {
			t.Fatal("a name over the bound was accepted")
		}
		if _, err := resolvePublicName("ali\x07ce"); err == nil {
			t.Fatal("a control character was accepted")
		}
	})

	t.Run("a world with no keyboard publishes nothing", func(t *testing.T) {
		h := newHarness(t)
		h.profile("default", "Multiverse", 8787)
		// A REDIRECTED STDIN IS THE SCRIPTED CASE, and it is what a silent
		// install, a service and a CI job all look like: a real file rather than
		// a character device, which is the same test the launcher makes to decide
		// whether there is anybody to ask (isTerminal). Nothing is asked, and the
		// account name of whoever ran it must not reach the file or the screen.
		scripted := filepath.Join(t.TempDir(), "no-input")
		writeFile(t, scripted, "", 0o644)
		file, err := os.Open(scripted)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer file.Close()
		h.stdin = file

		if code := h.run("profile", "create", "quiet", "--sidecar-port", "8790",
			"--join-string-file", writeJoinFile(t, "quiet")); code != exitOK {
			t.Fatalf("profile create: %d\n%s", code, h.err())
		}
		if strings.Contains(h.out(), "published publicly") {
			t.Fatalf("a create with no terminal asked a question anyway:\n%s", h.out())
		}
		p, err := h.install().LoadProfile("quiet")
		if err != nil {
			t.Fatalf("LoadProfile: %v", err)
		}
		if p.Keeper != "" || p.WorldName != "" {
			t.Fatalf("a silent create published keeper %q and world name %q", p.Keeper, p.WorldName)
		}
	})

	t.Run("a terminal is shown the value before it is taken", func(t *testing.T) {
		h := newHarness(t)
		// THE INSTALLATION HAS ALREADY ANSWERED, which is what a second world's
		// prompt opens with (inheritedKeeper). The account name is only ever
		// offered to an installation with no world to have answered — see the
		// two subtests below.
		base := h.profile("default", "Multiverse", 8787)
		base.Keeper = "ada"
		if err := h.install().SaveProfile(base); err != nil {
			t.Fatalf("SaveProfile: %v", err)
		}
		offered := "ada"

		// The harness's reader counts as a terminal (isTerminal), so this is the
		// interactive path: a blank line ACCEPTS THE VALUE THAT WAS ON SCREEN,
		// and a typed line replaces it.
		if code := h.runWith("\nA world of my own\n",
			"profile", "create", "asked", "--sidecar-port", "8792",
			"--join-string-file", writeJoinFile(t, "asked")); code != exitOK {
			t.Fatalf("profile create: %d\n%s", code, h.err())
		}
		mustContain(t, "the keeper prompt", h.out(), "published publicly ["+offered+"]")
		p, err := h.install().LoadProfile("asked")
		if err != nil {
			t.Fatalf("LoadProfile: %v", err)
		}
		if p.Keeper != offered {
			t.Fatalf("a blank answer wrote %q, want the value that was shown (%q)", p.Keeper, offered)
		}
		if p.WorldName != "A world of my own" {
			t.Fatalf("the typed world name became %q", p.WorldName)
		}

		// AND THE DECLINE IS AVAILABLE AT THE PROMPT ITSELF, which is what makes
		// the suggestion an offer rather than a default.
		if code := h.runWith("-\n-\n",
			"profile", "create", "declined", "--sidecar-port", "8793",
			"--join-string-file", writeJoinFile(t, "declined")); code != exitOK {
			t.Fatalf("profile create: %d\n%s", code, h.err())
		}
		declined, err := h.install().LoadProfile("declined")
		if err != nil {
			t.Fatalf("LoadProfile: %v", err)
		}
		if declined.Keeper != "" || declined.WorldName != "" {
			t.Fatalf("'-' published keeper %q and world name %q", declined.Keeper, declined.WorldName)
		}
	})

	t.Run("the flags carry what a prompt would have asked", func(t *testing.T) {
		h := newHarness(t)
		h.profile("default", "Multiverse", 8787)
		if code := h.run("profile", "create", "named", "--sidecar-port", "8791",
			"--keeper", "Alice", "--world-name", "Alice's world",
			"--join-string-file", writeJoinFile(t, "named")); code != exitOK {
			t.Fatalf("profile create: %d\n%s", code, h.err())
		}
		p, err := h.install().LoadProfile("named")
		if err != nil {
			t.Fatalf("LoadProfile: %v", err)
		}
		if p.Keeper != "Alice" || p.WorldName != "Alice's world" {
			t.Fatalf("the flags wrote keeper %q and world name %q", p.Keeper, p.WorldName)
		}

		// AND THEY CAN BE TAKEN BACK, with the same '-' every prompt takes.
		if code := h.run("profile", "set", "named", "--keeper", declineToken); code != exitOK {
			t.Fatalf("profile set --keeper -: %d\n%s", code, h.err())
		}
		p, err = h.install().LoadProfile("named")
		if err != nil {
			t.Fatalf("LoadProfile: %v", err)
		}
		if p.Keeper != "" {
			t.Fatalf("'-' left the keeper as %q", p.Keeper)
		}
		if p.WorldName != "Alice's world" {
			t.Fatalf("unsetting the keeper changed the world name to %q", p.WorldName)
		}

		// A name the map could not carry is refused at the write, with the
		// reason, rather than clipped into something else on the way out.
		if code := h.run("profile", "set", "named", "--keeper",
			strings.Repeat("x", MaxPublicNameBytes+1)); code != exitRefused {
			t.Fatalf("an over-long keeper exited %d, want %d", code, exitRefused)
		}
	})

	// A SECOND WORLD IS NOT A SECOND PERSON. The machine's account name is the
	// offer of last resort, and an installation that has already answered the
	// question has an answer to inherit — including the answer "publish none".
	// Session.NewWorldDefaults has always inherited it (session.go) and said in
	// its comment that the machine name must not be re-offered; the console's
	// own create prompt was reaching past that to the account name every time.
	t.Run("a second world is offered the handle this installation already uses", func(t *testing.T) {
		h := newHarness(t)
		base := h.profile("default", "Multiverse", 8787)
		base.Keeper, base.WorldName = "ada", "Tidepool"
		if err := h.install().SaveProfile(base); err != nil {
			t.Fatalf("SaveProfile: %v", err)
		}
		account := suggestedKeeper(h.getenv)

		// Two blank answers: whatever is on screen is what is written down.
		if code := h.runWith("\n\n", "profile", "create", "second", "--sidecar-port", "8794",
			"--join-string-file", writeJoinFile(t, "second")); code != exitOK {
			t.Fatalf("profile create: %d\n%s", code, h.err())
		}
		mustContain(t, "the keeper prompt", h.out(), "your keeper handle, published publicly [ada]")
		// The world name is built from the handle that was settled on, so the two
		// answers still read as one sentence.
		mustContain(t, "the world-name prompt", h.out(),
			"this world's name, published publicly [ada's world]")
		if account != "" {
			mustNotContain(t, "the create prompts", h.out(), "["+account+"]")
		}
		p, err := h.install().LoadProfile("second")
		if err != nil {
			t.Fatalf("LoadProfile: %v", err)
		}
		if p.Keeper != "ada" || p.WorldName != "ada's world" {
			t.Fatalf("the second world was written with keeper %q and world name %q", p.Keeper, p.WorldName)
		}
		// The window's create dialog opens with the same value, from the same
		// rule: one answer, three front doors.
		spec, err := h.session().NewWorldDefaults("third")
		if err != nil {
			t.Fatalf("NewWorldDefaults: %v", err)
		}
		if spec.Keeper != "ada" || spec.WorldName != "ada's world" {
			t.Fatalf("the create dialog opens with keeper %q and world name %q", spec.Keeper, spec.WorldName)
		}
	})

	t.Run("a declined handle is inherited as a decline", func(t *testing.T) {
		h := newHarness(t)
		// h.profile writes a world with no keeper, which is what an installation
		// whose participant declined at the join prompt looks like on disk.
		h.profile("default", "Multiverse", 8787)
		account := suggestedKeeper(h.getenv)
		if account == "" {
			t.Skip("this machine has no account name to be wrongly offered")
		}

		if code := h.runWith("\n\n", "profile", "create", "second", "--sidecar-port", "8795",
			"--join-string-file", writeJoinFile(t, "second")); code != exitOK {
			t.Fatalf("profile create: %d\n%s", code, h.err())
		}
		// NOTHING IS OFFERED, because somebody here has already been asked and
		// said no. Offering the account name again would be asking them to
		// decline the same name a second time per world.
		mustContain(t, "the keeper prompt", h.out(), "your keeper handle, published publicly [none]")
		mustNotContain(t, "the create prompts", h.out(), "["+account+"]")
		p, err := h.install().LoadProfile("second")
		if err != nil {
			t.Fatalf("LoadProfile: %v", err)
		}
		if p.Keeper != "" || p.WorldName != "" {
			t.Fatalf("a declined installation published keeper %q and world name %q",
				p.Keeper, p.WorldName)
		}
		// Same rule, same answer, in the window's dialog.
		spec, err := h.session().NewWorldDefaults("third")
		if err != nil {
			t.Fatalf("NewWorldDefaults: %v", err)
		}
		if spec.Keeper != "" || spec.WorldName != "" {
			t.Fatalf("the create dialog re-offered a declined name: %+v", spec)
		}
		// And the machine's account name is still what an installation with NO
		// world to inherit from is offered: there is nobody to have answered.
		if got := inheritedKeeper(Profile{}, false, h.getenv); got != account {
			t.Fatalf("an installation with no world was offered %q, want the account name %q",
				got, account)
		}
	})

	t.Run("the sidecar is told only what was chosen", func(t *testing.T) {
		h := newHarness(t)
		p := h.profile("default", "Multiverse", 8787)
		for _, arg := range sidecarArgs(p) {
			if arg == "--keeper" || arg == "--world-name" {
				t.Fatalf("a world that publishes nothing was started with %s", arg)
			}
		}
		p.Keeper = "Alice"
		p.WorldName = "Alice's world"
		args := strings.Join(sidecarArgs(p), " ")
		if !strings.Contains(args, "--keeper Alice") ||
			!strings.Contains(args, "--world-name Alice's world") {
			t.Fatalf("the sidecar's command line is missing a chosen name: %s", args)
		}
	})
}
