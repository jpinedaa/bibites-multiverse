package launcher

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileNameRules(t *testing.T) {
	cases := []struct {
		name  string
		value string
		valid bool
	}{
		{"plain", "default", true},
		{"digits and marks", "world-2.a_b", true},
		{"one character", "a", true},
		{"thirty two characters", strings.Repeat("a", 32), true},
		{"thirty three characters", strings.Repeat("a", 33), false},
		{"empty", "", false},
		{"leading dot", ".hidden", false},
		{"leading dash", "-world", false},
		{"a space", "my world", false},
		{"a path separator", "a/b", false},
		{"device name", "CON", false},
		{"device name in another case", "coM1", false},
		{"device name with an extension", "com1.json", false},
		{"unicode", "wörld", false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := validateProfileName(test.value)
			if test.valid && err != nil {
				t.Fatalf("%q was refused: %v", test.value, err)
			}
			if !test.valid && err == nil {
				t.Fatalf("%q was accepted and must not be", test.value)
			}
		})
	}

	// Uniqueness folds case, because the Windows file system does.
	others := []Profile{{Name: "Default"}}
	if err := validateProfileNameUnique("default", others); err == nil {
		t.Fatal("'default' was accepted beside 'Default'")
	}
}

func TestPortRules(t *testing.T) {
	others := []Profile{{Name: "default", SidecarPort: 8787}, {Name: "second", SidecarPort: 8788}}

	if err := validatePort(1023, nil); err == nil {
		t.Fatal("port 1023 was accepted")
	}
	if err := validatePort(65536, nil); err == nil {
		t.Fatal("port 65536 was accepted")
	}
	if err := validatePort(1024, nil); err != nil {
		t.Fatalf("port 1024 was refused: %v", err)
	}
	if err := validatePort(65535, nil); err != nil {
		t.Fatalf("port 65535 was refused: %v", err)
	}
	err := validatePort(8788, others)
	if err == nil {
		t.Fatal("a port another world already holds was accepted")
	}
	mustContain(t, "the refusal", err.Error(), "second")
	// THE WORD IS "WORLD". A profile is the file a world is written down in, and
	// this sentence is read by somebody who has one world and is making another —
	// including in the graphical launcher, which quotes the core's refusals into
	// its panel verbatim rather than paraphrasing them, so an internal word here
	// is an internal word on a participant's screen.
	mustContain(t, "the refusal", err.Error(), "world 'second'")
	if strings.Contains(err.Error(), "profile") {
		t.Fatalf("the refusal names a profile at a participant: %q", err.Error())
	}

	// The default offered to a new world is the lowest free port from the
	// Contract A default upwards.
	if got := nextFreePort(nil); got != DefaultSidecarPort {
		t.Fatalf("the first world was offered %d, want %d", got, DefaultSidecarPort)
	}
	if got := nextFreePort(others); got != 8789 {
		t.Fatalf("the third world was offered %d, want 8789", got)
	}
	gap := []Profile{{SidecarPort: 8787}, {SidecarPort: 8789}}
	if got := nextFreePort(gap); got != 8788 {
		t.Fatalf("the lowest free port was %d, want 8788", got)
	}
}

func TestDataRootRules(t *testing.T) {
	base := t.TempDir()
	install := Install{Root: filepath.Join(base, "install")}
	gameDir := filepath.Join(base, "game")
	ownGameDir := filepath.Join(base, "game2")
	worldA := filepath.Join(base, "worlds", "a")
	others := []Profile{{Name: "a", DataRoot: worldA, GameDir: gameDir}}

	cases := []struct {
		name  string
		value string
		valid bool
	}{
		{"its own folder", filepath.Join(base, "worlds", "b"), true},
		{"a sibling with a shared prefix", worldA + "2", true},
		{"the same folder", worldA, false},
		{"an ancestor", filepath.Join(base, "worlds"), false},
		{"a descendant", filepath.Join(worldA, "inner"), false},
		{"the same folder in another case", strings.ToUpper(worldA), false},
		{"a relative path", filepath.Join("worlds", "c"), false},
		{"inside another profile's game folder", filepath.Join(gameDir, "data"), false},
		{"another profile's game folder itself", gameDir, false},
		// The two holes that let --remove-world-data delete a Steam install or
		// the installed application.
		{"its OWN game folder", ownGameDir, false},
		{"inside its OWN game folder", filepath.Join(ownGameDir, "data"), false},
		{"the install root itself", install.Root, false},
		{"inside the install root", filepath.Join(install.Root, "worlds"), false},
		{"inside the profiles folder", filepath.Join(install.ProfilesDir(), "data"), false},
		{"an ancestor of the install root", base, false},
		{"the top of a drive", string(filepath.Separator), false},
		{"one folder below a drive", filepath.Join(string(filepath.Separator), "worlds"), false},
		{"empty", "", false},
		{"blank", "   ", false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := validateDataRoot(test.value, ownGameDir, others, install)
			if test.valid && err != nil {
				t.Fatalf("%q was refused: %v", test.value, err)
			}
			if !test.valid && err == nil {
				t.Fatalf("%q was accepted and must not be", test.value)
			}
		})
	}
}

// TestValidateRemovable is the last gate before os.RemoveAll. Everything it
// refuses is a directory tree the launcher would otherwise have deleted.
func TestValidateRemovable(t *testing.T) {
	h := newHarness(t)
	good := h.profile("default", "Multiverse", 8787)
	neighbour := h.profile("second", "Second", 8788)
	profiles := []Profile{good, neighbour}

	if err := validateRemovable(good, profiles, h.install()); err != nil {
		t.Fatalf("an ordinary world was refused: %v", err)
	}

	cases := []struct {
		name     string
		dataRoot string
	}{
		{"its own game folder", h.gameDir},
		{"inside its own game folder", filepath.Join(h.gameDir, "BepInEx")},
		{"the install root", h.root},
		{"inside the install root", h.install().ProfilesDir()},
		{"a relative path", "precious"},
		{"empty", ""},
		{"the top of a drive", string(filepath.Separator)},
		{"a neighbour's data root", neighbour.DataRoot},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			bad := good
			bad.DataRoot = test.dataRoot
			if err := validateRemovable(bad, profiles, h.install()); err == nil {
				t.Fatalf("deleting %q was allowed", test.dataRoot)
			}
		})
	}
}

// TestManagedRuntimeIsTheOnlyAllowedOverlap: the complete edition's game folder
// sits inside its own world's data root. That ONE shape is allowed; everything
// else that overlaps is the deletion hazard the rule exists for.
func TestManagedRuntimeIsTheOnlyAllowedOverlap(t *testing.T) {
	const dataRoot = `C:\Users\alice\AppData\Local\BibitesMultiverse`
	cases := []struct {
		name    string
		gameDir string
		managed bool
	}{
		{"the installer's own layout", dataRoot + `\runtimes\ABC123`, true},
		{"a build folder deeper still", dataRoot + `\runtimes\ABC123\inner`, true},
		{"the same path with forward slashes", "C:/Users/alice/AppData/Local/BibitesMultiverse/runtimes/ABC123", true},
		{"the same path in another case", strings.ToUpper(dataRoot) + `\RUNTIMES\ABC123`, true},
		{"the runtimes folder itself", dataRoot + `\runtimes`, false},
		{"the data root itself", dataRoot, false},
		{"the journal", dataRoot + `\data`, false},
		{"a folder that merely starts the same way", dataRoot + `\runtimes-old\ABC123`, false},
		{"another data root's runtime", `C:\Users\alice\Other\runtimes\ABC123`, false},
		{"nothing", "", false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := isManagedRuntime(dataRoot, test.gameDir); got != test.managed {
				t.Fatalf("isManagedRuntime(%q, %q) = %v, want %v",
					dataRoot, test.gameDir, got, test.managed)
			}
		})
	}

	// The overlap itself has to be SEEN wherever this program runs. A backslash is
	// an ordinary character off Windows, so before foldPath the installer's own
	// profile "did not overlap" anything here and every test of the rule passed on
	// a file the real machine refused.
	if !pathsOverlap(dataRoot, dataRoot+`\runtimes\ABC123`) {
		t.Fatal("a Windows path did not overlap its own parent")
	}
	if pathsOverlap(dataRoot, dataRoot+`-second`) {
		t.Fatal("a sibling with a shared prefix was read as an overlap")
	}
}

// TestManagedRuntimeWorldValidates is the same rule against the real validators,
// with the paths this machine actually has.
func TestManagedRuntimeWorldValidates(t *testing.T) {
	h := newHarness(t)
	p := h.managedProfile("default", "Multiverse", 8787)

	if err := validateProfile(p, nil, h.install()); err != nil {
		t.Fatalf("the complete edition's own world was refused: %v", err)
	}
	if err := validateProfilePaths(p, h.install()); err != nil {
		t.Fatalf("the complete edition's own world was refused at load: %v", err)
	}
	if err := validateRemovable(p, []Profile{p}, h.install()); err != nil {
		t.Fatalf("the complete edition's own world could not be removed: %v", err)
	}

	// A SECOND world runs the same managed runtime, which lives under the FIRST
	// world's data root. Neither profile may be invalidated by the other.
	second := p
	second.Name = "second"
	second.World = "Second"
	second.SidecarPort = 8788
	second.DataRoot = filepath.Join(h.dataParent, "BibitesMultiverse-second")
	if err := validateProfile(p, []Profile{second}, h.install()); err != nil {
		t.Fatalf("the world hosting the runtime was refused once a second world shared it: %v", err)
	}
	if err := validateProfile(second, []Profile{p}, h.install()); err != nil {
		t.Fatalf("the second world on the shared runtime was refused: %v", err)
	}
	if err := validateRemovable(p, []Profile{p, second}, h.install()); err != nil {
		t.Fatalf("the world hosting the runtime could not be removed: %v", err)
	}

	// A game folder ANYWHERE ELSE inside the data root is still the old hazard.
	bad := p
	bad.GameDir = filepath.Join(p.DataRoot, "game")
	h.makeGameDir(bad.GameDir)
	err := validateProfile(bad, nil, h.install())
	if err == nil {
		t.Fatal("a game folder inside the data root, outside runtimes\\, was accepted")
	}
	mustContain(t, "the refusal", err.Error(), "overlaps this world's own game folder")
	if err := validateProfilePaths(bad, h.install()); err == nil {
		t.Fatal("that profile was accepted at load")
	}
}

func TestWorldRules(t *testing.T) {
	others := []Profile{{Name: "default", World: "Multiverse"}}
	cases := []struct {
		name  string
		value string
		valid bool
	}{
		{"plain", "Second", true},
		{"a space inside", "My World", true},
		{"sixty four characters", strings.Repeat("w", 64), true},
		{"sixty five characters", strings.Repeat("w", 65), false},
		{"empty", "", false},
		{"a backslash", `a\b`, false},
		{"a colon", "a:b", false},
		{"a question mark", "a?b", false},
		{"a control character", "a\x01b", false},
		{"a trailing space", "world ", false},
		{"a trailing dot", "world.", false},
		{"a device name", "nul", false},
		{"a duplicate", "Multiverse", false},
		{"a duplicate in another case", "MULTIVERSE", false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := validateWorld(test.value, others)
			if test.valid && err != nil {
				t.Fatalf("%q was refused: %v", test.value, err)
			}
			if !test.valid && err == nil {
				t.Fatalf("%q was accepted and must not be", test.value)
			}
		})
	}
}

// TestExportEdgeParsing matches Install-BibitesMultiverse.ps1's own parser:
// split on commas, semicolons and whitespace, upper-case, refuse a repeat.
func TestExportEdgeParsing(t *testing.T) {
	cases := []struct {
		value string
		want  string
	}{
		{"E,N,W,S", "E,N,W,S"},
		{"e,n,w,s", "E,N,W,S"},
		{"E N W S", "E,N,W,S"},
		{"E;N", "E,N"},
		{" E ,\tN ", "E,N"},
		{"S", "S"},
	}
	for _, test := range cases {
		got, err := normalizeExportEdges(test.value)
		if err != nil {
			t.Fatalf("%q was refused: %v", test.value, err)
		}
		if got != test.want {
			t.Fatalf("%q normalised to %q, want %q", test.value, got, test.want)
		}
	}

	for _, bad := range []string{"", " ", ",", "X", "E,X", "E,E", "e,E", "N,S,N"} {
		if _, err := normalizeExportEdges(bad); err == nil {
			t.Fatalf("%q was accepted and must not be", bad)
		}
	}
	if _, err := normalizeExportEdges("E,E"); err == nil || !strings.Contains(err.Error(), "repeat") {
		t.Fatalf("a repeated edge must be refused as a repeat, got %v", err)
	}
}

func TestExcludeSpeciesRequiresExplicitOptOut(t *testing.T) {
	if _, err := resolveExcludeSpecies("", false); err == nil {
		t.Fatal("an empty exclusion list was accepted without the switch")
	}
	if _, err := resolveExcludeSpecies("   ", false); err == nil {
		t.Fatal("a blank exclusion list was accepted without the switch")
	}
	value, err := resolveExcludeSpecies("", true)
	if err != nil {
		t.Fatalf("--no-migration-exclusion was refused: %v", err)
	}
	if value != "" {
		t.Fatalf("--no-migration-exclusion left %q behind", value)
	}
	// The switch wins over a value, the way the installer's -NoMigrationExclusion does.
	value, err = resolveExcludeSpecies("Basic bibite", true)
	if err != nil || value != "" {
		t.Fatalf("--no-migration-exclusion did not clear the list: %q %v", value, err)
	}
	value, err = resolveExcludeSpecies("Basic bibite", false)
	if err != nil || value != "Basic bibite" {
		t.Fatalf("a normal list did not survive: %q %v", value, err)
	}
}
