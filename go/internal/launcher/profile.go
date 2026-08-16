package launcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ProfileFormat is the schema string every profile file carries. A file that
// does not carry it is refused rather than guessed at.
const ProfileFormat = "bibites-multiverse/launcher-profile/1"

// DefaultSidecarPort is the first port a new world is offered. It is the
// Contract A default, and a test pins the two together.
const DefaultSidecarPort = 8787

// The defaults a new world is created with. They are the same values
// Install-BibitesMultiverse.ps1 installs the first world with, written out
// explicitly so a future change to a default cannot silently move what an
// existing world does.
const (
	DefaultExportEdges    = "E,N,W,S"
	DefaultExcludeSpecies = "Basic bibite"
	DefaultSaveMinutes    = 10.0
	DefaultSaveKeep       = 6
)

// MaxLocalWorlds is the number of worlds that can run at once out of ONE game
// directory. The mod framework hands out five log files and an instance that
// gets none does not merely lose its log — the mod never loads in it
// (LOCAL-STARVATION in docs/error-taxonomy.md).
const MaxLocalWorlds = 5

// Profile is one world on this machine: its identity on the map, its own data
// root, its own sidecar port and the settings the game is started with.
//
// The field order below is the JSON key order, and it is frozen: the Windows
// installer writes the same object with ConvertTo-Json for the first world, and
// a test parses that file to keep the two writers from drifting.
//
// THERE IS NO SECRET IN THIS STRUCT. The secret half of the map credential
// lives in <dataRoot>/peer-secret.txt and nowhere else.
type Profile struct {
	Format         string  `json:"format"`
	Name           string  `json:"name"`
	GameDir        string  `json:"gameDir"`
	DataRoot       string  `json:"dataRoot"`
	SidecarPort    int     `json:"sidecarPort"`
	World          string  `json:"world"`
	Headless       bool    `json:"headless"`
	ExportEdges    string  `json:"exportEdges"`
	ExcludeSpecies string  `json:"excludeSpecies"`
	SaveMinutes    float64 `json:"saveMinutes"`
	SaveKeep       int     `json:"saveKeep"`
	SaveOnQuit     bool    `json:"saveOnQuit"`
	PeerID         string  `json:"peerId"`
	RelayURL       string  `json:"relayUrl"`
	CreatedUTC     string  `json:"createdUtc"`
}

// errNoProfiles is returned when the install holds no profile at all, which is
// a different problem from "the named one is missing".
var errNoProfiles = errors.New("this installation has no world profiles yet")

// LoadProfile reads one profile and refuses anything the file itself can be
// wrong about — format, the name against the file name, and every structural
// rule of validateProfileStructure. NOTHING JOINS A PATH ONTO A PROFILE THAT
// HAS NOT BEEN THROUGH HERE.
func (i Install) LoadProfile(name string) (Profile, error) {
	return i.readProfileFile(i.ProfilePath(name))
}

func (i Install) readProfileFile(path string) (Profile, error) {
	var p Profile
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return p, fmt.Errorf("there is no world called '%s' here (%s)",
				strings.TrimSuffix(filepath.Base(path), ".json"), path)
		}
		return p, err
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("%s is not readable as a world profile: %w", path, err)
	}
	base := strings.TrimSuffix(filepath.Base(path), ".json")
	if !strings.EqualFold(p.Name, base) {
		return p, fmt.Errorf("%s names the world '%s'; the file name and the name must agree", path, p.Name)
	}
	if err := validateProfileStructure(p); err != nil {
		return Profile{}, fmt.Errorf("%s is not a world profile the launcher will act on: %w", path, err)
	}
	if err := validateProfilePaths(p, i); err != nil {
		return Profile{}, fmt.Errorf("%s is not a world profile the launcher will act on: %w", path, err)
	}
	return p, nil
}

// loadProfiles reads every profile and reports the files it could not use,
// instead of failing the whole call. It is what the commands that only REPORT
// or STOP use, so one stray file in profiles\ cannot leave a participant unable
// to shut a running world down.
func (i Install) loadProfiles() ([]Profile, []error) {
	entries, err := os.ReadDir(i.ProfilesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []error{err}
	}
	var profiles []Profile
	var problems []error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		p, err := i.readProfileFile(filepath.Join(i.ProfilesDir(), entry.Name()))
		if err != nil {
			problems = append(problems, err)
			continue
		}
		profiles = append(profiles, p)
	}
	sort.Slice(profiles, func(a, b int) bool { return profiles[a].Name < profiles[b].Name })
	return profiles, problems
}

// ListProfiles is the strict form: any unusable file is an error. The
// collision-sensitive commands — start, and everything that writes a profile —
// use it, because a world the launcher cannot see is a world it would happily
// collide with.
func (i Install) ListProfiles() ([]Profile, error) {
	profiles, problems := i.loadProfiles()
	if len(problems) > 0 {
		return nil, fmt.Errorf("%w. Fix or remove that file; 'stop NAME' and 'status' still work "+
			"on the worlds that do parse", problems[0])
	}
	return profiles, nil
}

// SaveProfile writes the profile atomically: a temporary file beside it, then a
// rename, which replaces on Windows too. A half-written profile would be a
// world the launcher refuses to load.
func (i Install) SaveProfile(p Profile) error {
	if err := os.MkdirAll(i.ProfilesDir(), 0o755); err != nil {
		return err
	}
	final := i.ProfilePath(p.Name)
	tmp := final + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(p); err != nil {
		file.Close()
		os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// DeleteProfile removes the profile file only. What lives under its data root
// is the caller's decision, because that is where the world's journal is.
func (i Install) DeleteProfile(name string) error {
	return os.Remove(i.ProfilePath(name))
}

// ActiveName resolves which world the bare commands act on: active.txt when it
// names a profile that exists, then 'default', then the only profile when there
// is exactly one. Anything else is a refusal that lists the choices.
func (i Install) ActiveName() (string, error) {
	profiles, problems := i.loadProfiles()
	if len(profiles) == 0 && len(problems) > 0 {
		return "", problems[0]
	}
	if len(profiles) == 0 {
		return "", errNoProfiles
	}
	known := make(map[string]string, len(profiles))
	for _, p := range profiles {
		known[strings.ToLower(p.Name)] = p.Name
	}
	if raw, err := os.ReadFile(i.ActiveFile()); err == nil {
		wanted := strings.TrimSpace(firstLine(string(raw)))
		if name, ok := known[strings.ToLower(wanted)]; ok {
			return name, nil
		}
	}
	if name, ok := known["default"]; ok {
		return name, nil
	}
	if len(profiles) == 1 {
		return profiles[0].Name, nil
	}
	return "", fmt.Errorf("no world is selected. Choose one with 'profile use NAME', or name it on "+
		"the command line. This installation has: %s", strings.Join(profileNames(profiles), ", "))
}

// SetActive records which world the bare commands act on.
func (i Install) SetActive(name string) error {
	if err := os.MkdirAll(i.ProfilesDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(i.ActiveFile(), []byte(name+"\n"), 0o644)
}

// ResolveProfile turns an optional name into a profile: the name when it was
// given, the active world otherwise. Names are matched WITHOUT CASE here and
// everywhere else, so 'show DEFAULT' and 'set DEFAULT' agree on Linux the way
// they already did on Windows.
func (i Install) ResolveProfile(name string) (Profile, error) {
	if strings.TrimSpace(name) == "" {
		active, err := i.ActiveName()
		if err != nil {
			return Profile{}, err
		}
		name = active
	}
	profiles, _ := i.loadProfiles()
	if p, found := findProfile(profiles, name); found {
		return p, nil
	}
	return i.LoadProfile(name)
}

// findProfile looks a name up case-insensitively, the way the Windows file
// system does.
func findProfile(profiles []Profile, name string) (Profile, bool) {
	for _, p := range profiles {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Profile{}, false
}

// otherProfiles is every profile except the named one — the set every
// uniqueness rule is checked against.
func otherProfiles(profiles []Profile, name string) []Profile {
	out := make([]Profile, 0, len(profiles))
	for _, p := range profiles {
		if !strings.EqualFold(p.Name, name) {
			out = append(out, p)
		}
	}
	return out
}

func profileNames(profiles []Profile) []string {
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}
	return names
}

// nextFreePort is the default a new world is offered: the lowest port from the
// Contract A default upwards that no other profile already holds.
func nextFreePort(profiles []Profile) int {
	taken := make(map[int]bool, len(profiles))
	for _, p := range profiles {
		taken[p.SidecarPort] = true
	}
	for port := DefaultSidecarPort; port <= maxPort; port++ {
		if !taken[port] {
			return port
		}
	}
	return 0
}

// defaultDataRoot places a new world's data beside the active world's, which is
// where a participant already knows to look for the journal and the logs.
func defaultDataRoot(active Profile, name string) string {
	parent := filepath.Dir(active.DataRoot)
	if parent == "" || parent == "." {
		parent = active.DataRoot
	}
	return filepath.Join(parent, "BibitesMultiverse-"+name)
}

// createdNow is the createdUtc stamp, in the same shape the installer writes.
func createdNow(now func() time.Time) string {
	return now().UTC().Format("2006-01-02T15:04:05Z")
}

func firstLine(s string) string {
	if cut := strings.IndexAny(s, "\r\n"); cut >= 0 {
		return s[:cut]
	}
	return s
}
