package launcher

import (
	"encoding/json"
	"os"
	"strings"
)

// The installer's record, read only. Uninstall-BibitesMultiverse.ps1 owns it;
// the launcher reads it for two things: to know which world the installer
// created, and to cross-check that world's game directory.
//
// The Windows record is bibites-multiverse/install-record/3, which added
// world, sidecarPort, settings and the profiles block. The Linux record stays
// at /2 and carries none of them, so every field below is optional and the
// version string is not enforced: a record the launcher cannot fully read is
// still a record it can take gameDir from.

type installRecord struct {
	Record      string          `json:"record"`
	Release     string          `json:"release"`
	KitDir      string          `json:"kitDir"`
	GameDir     string          `json:"gameDir"`
	DataRoot    string          `json:"dataRoot"`
	PeerID      string          `json:"peerId"`
	RelayURL    string          `json:"relayUrl"`
	World       string          `json:"world"`
	SidecarPort int             `json:"sidecarPort"`
	Settings    *recordSettings `json:"settings"`
	Profiles    *recordProfiles `json:"profiles"`
}

type recordSettings struct {
	ExportEdges    string  `json:"exportEdges"`
	ExcludeSpecies string  `json:"excludeSpecies"`
	SaveMinutes    float64 `json:"saveMinutes"`
	SaveKeep       int     `json:"saveKeep"`
	SaveOnQuit     string  `json:"saveOnQuit"`
}

type recordProfiles struct {
	Root    string `json:"root"`
	Default string `json:"default"`
	File    string `json:"file"`
	Active  string `json:"active"`
}

// readInstallRecord returns the record under a profile's data root, or false
// when there is none. A profile the launcher created has no record, which is
// normal rather than an error.
func readInstallRecord(dataRoot string) (installRecord, bool) {
	var record installRecord
	raw, err := os.ReadFile(Profile{DataRoot: dataRoot}.RecordFile())
	if err != nil {
		return record, false
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		return record, false
	}
	return record, true
}

// installerDefaultProfile names the world the installer created, when this
// installation has one. 'profile set' warns on that world, because its
// generated Start-Multiverse.ps1 still holds the values it was installed with.
func (i Install) installerDefaultProfile() string {
	profiles, err := i.ListProfiles()
	if err != nil {
		return ""
	}
	for _, p := range profiles {
		record, found := readInstallRecord(p.DataRoot)
		if !found || record.Profiles == nil {
			continue
		}
		if strings.TrimSpace(record.Profiles.Default) != "" {
			return record.Profiles.Default
		}
	}
	return ""
}

// installedWorldNote is what 'profile set' prints on that world. Two sources of
// truth is a deliberate choice (the generated script keeps working for advanced
// use), so the one that is no longer authoritative has to say so.
const installedWorldNote = `  The generated Start-Multiverse.ps1 still holds the values this world was installed with.
  The launcher is what this world runs from now.`
