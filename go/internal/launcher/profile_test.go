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
var profileKeyOrder = []string{
	"format", "name", "gameDir", "dataRoot", "sidecarPort", "world", "headless",
	"exportEdges", "excludeSpecies", "saveMinutes", "saveKeep", "saveOnQuit",
	"peerId", "relayUrl", "createdUtc",
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
// and release/kit/Install-BibitesMultiverse.ps1: the file in testdata is what
// that script's ConvertTo-Json emits for a reference install.
func TestInstallerWrittenProfileParses(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "installer-default-profile.json"))
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
		GameDir:        `C:\Program Files (x86)\Steam\steamapps\common\The Bibites`,
		DataRoot:       `C:\Users\alice\AppData\Local\BibitesMultiverse`,
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
	}
	if p != want {
		t.Fatalf("the installer's profile did not parse as expected:\n got %+v\nwant %+v", p, want)
	}

	// Every value's JSON TYPE matters as much as its value: a port written as a
	// string would parse into a different program and fail here.
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
