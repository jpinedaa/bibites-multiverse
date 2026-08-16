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
}

// ProfileStatus is one world's settings and what is running for it.
type ProfileStatus struct {
	Name           string        `json:"name"`
	Active         bool          `json:"active"`
	World          string        `json:"world"`
	GameDir        string        `json:"gameDir"`
	DataRoot       string        `json:"dataRoot"`
	SidecarPort    int           `json:"sidecarPort"`
	Headless       bool          `json:"headless"`
	ExportEdges    string        `json:"exportEdges"`
	ExcludeSpecies string        `json:"excludeSpecies"`
	SaveMinutes    float64       `json:"saveMinutes"`
	SaveKeep       int           `json:"saveKeep"`
	SaveOnQuit     bool          `json:"saveOnQuit"`
	PeerID         string        `json:"peerId"`
	RelayURL       string        `json:"relayUrl"`
	CreatedUTC     string        `json:"createdUtc"`
	Sidecar        ProcessStatus `json:"sidecar"`
	Game           ProcessStatus `json:"game"`
}

// ProcessStatus is the PID ledger's answer for one process.
type ProcessStatus struct {
	PID     int  `json:"pid"`
	Running bool `json:"running"`
}

// collectStatus reads the ledger for the profiles it is given.
func (a *app) collectStatus(profiles []Profile, active string) Status {
	status := Status{
		Format:      StatusFormat,
		Release:     Release,
		InstallRoot: a.install.Root,
		Active:      active,
		Profiles:    make([]ProfileStatus, 0, len(profiles)),
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
			Sidecar:        ProcessStatus{PID: sidecarPid, Running: sidecarRunning},
			Game:           ProcessStatus{PID: gamePid, Running: gameRunning},
		})
	}
	return status
}

// runStatus prints the status of one world or of every world.
func (a *app) runStatus(name string, all bool) int {
	var profiles []Profile
	active, activeErr := a.install.ActiveName()
	if all {
		found, problems := a.install.loadProfiles()
		for _, problem := range problems {
			a.warn("skipping a profile that could not be read: %v", problem)
		}
		if len(found) == 0 {
			if a.asJSON {
				return a.emitJSON(a.collectStatus(nil, ""))
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
	status := a.collectStatus(profiles, active)
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

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
