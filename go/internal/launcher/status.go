package launcher

import (
	"encoding/json"
	"fmt"
	"strings"
)

// StatusFormat is the schema of the --json output. It is stable across
// releases, so a report from an old build is still readable.
const StatusFormat = "bibites-multiverse/launcher-status/1"

// Status is what the launcher knows about this installation.
//
// NOTHING IN HERE IS A SECRET. peerId is the identity half, which is public by
// design; the secret half never leaves <dataRoot>/peer-secret.txt.
type Status struct {
	Format      string          `json:"format"`
	Release     string          `json:"release"`
	InstallRoot string          `json:"installRoot"`
	Active      string          `json:"active"`
	Profiles    []ProfileStatus `json:"profiles"`
	// Problems is every reason this report is not the whole picture: a profile
	// file that could not be read, and — when nothing could be read at all — the
	// refusal the human form prints. It is ADDITIVE and omitted when empty, so a
	// healthy installation's document is byte for byte the one this schema has
	// always emitted.
	//
	// IT EXISTS BECAUSE SILENCE WAS WORSE THAN AN ERROR. `status --all --json`
	// answered {"active": "", "profiles": []} and exit 0 while a world was
	// running, whenever the profile behind it failed to load — the same call the
	// human form exits 1 on. Anything watching this output read "nothing is
	// installed here" from a launcher that simply could not read its own files.
	Problems []string `json:"problems,omitempty"`
}

// ProfileStatus is one world's settings and what is running for it.
type ProfileStatus struct {
	Name           string  `json:"name"`
	Active         bool    `json:"active"`
	World          string  `json:"world"`
	GameDir        string  `json:"gameDir"`
	DataRoot       string  `json:"dataRoot"`
	SidecarPort    int     `json:"sidecarPort"`
	Headless       bool    `json:"headless"`
	ExportEdges    string  `json:"exportEdges"`
	ExcludeSpecies string  `json:"excludeSpecies"`
	SaveMinutes    float64 `json:"saveMinutes"`
	SaveKeep       int     `json:"saveKeep"`
	SaveOnQuit     bool    `json:"saveOnQuit"`
	PeerID         string  `json:"peerId"`
	RelayURL       string  `json:"relayUrl"`
	CreatedUTC     string  `json:"createdUtc"`
	// Keeper and WorldName are what this world publishes about the person
	// running it. They are omitted when unset, because absent here means what
	// absent means on the wire: nobody chose one, and nothing invented one.
	Keeper    string        `json:"keeper,omitempty"`
	WorldName string        `json:"worldName,omitempty"`
	Sidecar   ProcessStatus `json:"sidecar"`
	Game      ProcessStatus `json:"game"`
}

// ProcessStatus is the PID ledger's answer for one process.
type ProcessStatus struct {
	PID     int  `json:"pid"`
	Running bool `json:"running"`
}

// collectStatus reads the ledger for the profiles it is given, and carries the
// reasons any others are missing from it.
func (a *app) collectStatus(profiles []Profile, active string, problems []error) Status {
	status := Status{
		Format:      StatusFormat,
		Release:     Release,
		InstallRoot: a.install.Root,
		Active:      active,
		Profiles:    make([]ProfileStatus, 0, len(profiles)),
	}
	for _, problem := range problems {
		status.Problems = append(status.Problems, problem.Error())
	}
	for _, p := range profiles {
		sidecarPid, sidecarRunning := describePid(p.SidecarPidFile(), a.install.SidecarExe())
		gamePid, gameRunning := describePid(p.GamePidFile(), p.GameExe())
		status.Profiles = append(status.Profiles, ProfileStatus{
			Name:           p.Name,
			Active:         strings.EqualFold(p.Name, active),
			World:          p.World,
			GameDir:        p.GameDir,
			DataRoot:       p.DataRoot,
			SidecarPort:    p.SidecarPort,
			Headless:       p.Headless,
			ExportEdges:    p.ExportEdges,
			ExcludeSpecies: p.ExcludeSpecies,
			SaveMinutes:    p.SaveMinutes,
			SaveKeep:       p.SaveKeep,
			SaveOnQuit:     p.SaveOnQuit,
			PeerID:         p.PeerID,
			RelayURL:       p.RelayURL,
			CreatedUTC:     p.CreatedUTC,
			Keeper:         p.Keeper,
			WorldName:      p.WorldName,
			Sidecar:        ProcessStatus{PID: sidecarPid, Running: sidecarRunning},
			Game:           ProcessStatus{PID: gamePid, Running: gameRunning},
		})
	}
	return status
}

// runStatus prints the status of one world or of every world. The two output
// forms report the SAME facts and exit the SAME way: a report that could read
// nothing is a refusal in both, whatever the format.
func (a *app) runStatus(name string, all bool) int {
	var profiles []Profile
	var problems []error
	active, activeErr := a.install.ActiveName()
	if all {
		found, readProblems := a.install.loadProfiles()
		problems = readProblems
		for _, problem := range problems {
			a.warn("skipping a profile that could not be read: %v", problem)
		}
		if len(found) == 0 {
			if a.asJSON {
				// The document still goes out — a reader wants to know WHICH
				// installation answered nothing — and the exit code still says
				// this call failed.
				a.emitJSON(a.collectStatus(nil, "", append(problems, errNoProfiles)))
				return exitRefused
			}
			return a.fail("%v", errNoProfiles)
		}
		profiles = found
	} else {
		p, err := a.install.ResolveProfile(name)
		if err != nil {
			return a.fail("%v", err)
		}
		profiles = []Profile{p}
		if activeErr != nil {
			active = p.Name
		}
	}
	if activeErr != nil {
		active = ""
	}
	status := a.collectStatus(profiles, active, problems)
	if a.asJSON {
		return a.emitJSON(status)
	}
	a.printStatus(status)
	return exitOK
}

func (a *app) emitJSON(value any) int {
	encoder := json.NewEncoder(a.stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return a.fail("%v", err)
	}
	return exitOK
}

// printStatus is the human form: one block per world.
func (a *app) printStatus(status Status) {
	a.print("Bibites Multiverse launcher %s", status.Release)
	a.print("installed in %s", status.InstallRoot)
	for _, problem := range status.Problems {
		a.print("  a world could not be read: %s", problem)
	}
	for _, p := range status.Profiles {
		marker := " "
		if p.Active {
			marker = "*"
		}
		a.print("")
		a.print("%s %s   world '%s'   port %d   headless %s",
			marker, p.Name, p.World, p.SidecarPort, onOff(p.Headless))
		a.print("    sidecar %-24s game %s", processWords(p.Sidecar), processWords(p.Game))
		a.print("    identity %s", p.PeerID)
		a.print("    map      %s", p.RelayURL)
		a.print("    game     %s", p.GameDir)
		a.print("    data     %s", p.DataRoot)
		a.print("    edges %s   never leaves %s   saves every %s min keeping %d   on quit %s",
			p.ExportEdges, excludeWords(p.ExcludeSpecies), formatMinutes(p.SaveMinutes),
			p.SaveKeep, onOff(p.SaveOnQuit))
		a.print("    published keeper %s   world name %s", orNone(p.Keeper), orNone(p.WorldName))
	}
}

func processWords(p ProcessStatus) string {
	if !p.Running {
		return "stopped"
	}
	return fmt.Sprintf("running (pid %d)", p.PID)
}

func excludeWords(value string) string {
	if value == "" {
		return "nothing - the exclusion policy is OFF"
	}
	return "'" + value + "'"
}

// orNone reads an OPTIONAL public string back. Unset is said out loud rather
// than left as an empty column, because "none" is a choice this program is not
// allowed to fill in and a blank space reads like a missing value.
func orNone(value string) string {
	if value == "" {
		return "none"
	}
	return "'" + value + "'"
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
