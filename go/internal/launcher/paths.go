package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The names the installer writes and the launcher reads. They are a contract
// with release/kit/Install-BibitesMultiverse.ps1 and with the generated
// Start-Multiverse.ps1 / Stop-Multiverse.ps1, which keep working beside the
// launcher and share the same on-disk PID protocol.
const (
	// LauncherExeName is the installed name of this program on Windows. It is
	// the name the shortcuts, Add/Remove Programs and Task Manager show, so it
	// is branded rather than internal.
	LauncherExeName = "BibitesMultiverseLauncher.exe"

	profilesDirName = "profiles"
	activeFileName  = "active.txt"
	publicMapName   = "public-map.json"
	recordName      = "install-record.json"

	credentialFileName = "peer-secret.txt"
	pendingFileName    = "enrollment-pending.json"
	sidecarPidFileName = "sidecar.pid"
	gamePidFileName    = "game.pid"
	lockFileName       = "launcher.lock"

	dataDirName = "data"
	logDirName  = "logs"

	sidecarLogName    = "sidecar.log"
	sidecarLogOutName = "sidecar.log.out"
	launcherLogName   = "launcher.log"

	// contractATokenName must match what the sidecar mints and what the mod
	// reads; the game is told the path through MULTIVERSE_CONTRACT_A_TOKEN_FILE.
	contractATokenName = "contract-a.token"

	// pluginRelPath is the mod inside a game directory. Its absence at start is
	// a refusal: the game would run, join nothing, and look healthy.
	pluginRelPath = "BepInEx/plugins/BibitesMultiverse.dll"
)

// Install is one installed application directory: the folder that holds the
// launcher, multiverse-sidecar, public-map.json and the profiles.
type Install struct {
	Root string
}

// discoverInstallRoot implements the frozen discovery order: the flag, then
// MULTIVERSE_LAUNCHER_HOME, then the directory this executable was started
// from with symlinks resolved. executable is injected so a test can pin the
// third step without building an executable.
func discoverInstallRoot(flagValue string, getenv func(string) string, executable func() (string, error)) (string, error) {
	if v := strings.TrimSpace(flagValue); v != "" {
		return absClean(v)
	}
	if v := strings.TrimSpace(getenv("MULTIVERSE_LAUNCHER_HOME")); v != "" {
		return absClean(v)
	}
	exe, err := executable()
	if err != nil {
		return "", fmt.Errorf("this program could not find its own location, so it does not know "+
			"where it was installed. Pass --install-root: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return absClean(filepath.Dir(exe))
}

// absClean turns a user-supplied path into the cleaned absolute form every
// comparison in validate.go assumes.
func absClean(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%s is not a usable path: %w", path, err)
	}
	return filepath.Clean(abs), nil
}

// ProfilesDir is the only directory under the install root the launcher writes.
func (i Install) ProfilesDir() string { return filepath.Join(i.Root, profilesDirName) }

// ProfilePath is where the profile called name lives. The file base name and
// the profile's own name field must agree; LoadProfile enforces it.
func (i Install) ProfilePath(name string) string {
	return filepath.Join(i.ProfilesDir(), name+".json")
}

// ActiveFile holds one line: the name of the profile the bare commands act on.
func (i Install) ActiveFile() string { return filepath.Join(i.ProfilesDir(), activeFileName) }

// SidecarExe is the sidecar this install ships beside the launcher.
func (i Install) SidecarExe() string { return filepath.Join(i.Root, sidecarExeName) }

// PublicMapPath carries the public map's enrollment and relay addresses. It
// holds no secret, which is why it can be shipped and read here.
func (i Install) PublicMapPath() string { return filepath.Join(i.Root, publicMapName) }

// The per-profile paths. Everything below lives under the profile's own data
// root, which is what makes two worlds on one machine independent.

// DataDir is the sidecar's --data-dir: the journal, the peer id, the slot and
// the Contract A token.
func (p Profile) DataDir() string { return filepath.Join(p.DataRoot, dataDirName) }

// LogDir holds the sidecar's two streams and the launcher's own event log.
func (p Profile) LogDir() string { return filepath.Join(p.DataRoot, logDirName) }

// CredentialFile holds the SECRET HALF of this world's map credential and
// nothing else. It is the only file in the system that holds it.
func (p Profile) CredentialFile() string { return filepath.Join(p.DataRoot, credentialFileName) }

// PendingFile exists only between the first enrollment attempt and its success,
// so a lost response is a retry rather than a second identity.
func (p Profile) PendingFile() string { return filepath.Join(p.DataRoot, pendingFileName) }

// RecordFile is the installer's record. Only the profile the installer created
// has one.
func (p Profile) RecordFile() string { return filepath.Join(p.DataRoot, recordName) }

// SidecarPidFile and GamePidFile are the ledger the PowerShell scripts share:
// one decimal pid, ASCII, on the first line.
func (p Profile) SidecarPidFile() string { return filepath.Join(p.DataRoot, sidecarPidFileName) }

// GamePidFile is the game half of that ledger.
func (p Profile) GamePidFile() string { return filepath.Join(p.DataRoot, gamePidFileName) }

// LockFile is held only while the launcher is starting or stopping this world.
func (p Profile) LockFile() string { return filepath.Join(p.DataRoot, lockFileName) }

// SidecarLog is the sidecar's stderr, which is where the slot-granted line
// lands and where the slot wait reads.
func (p Profile) SidecarLog() string { return filepath.Join(p.LogDir(), sidecarLogName) }

// SidecarLogOut is the sidecar's stdout.
func (p Profile) SidecarLogOut() string { return filepath.Join(p.LogDir(), sidecarLogOutName) }

// LauncherLog is this program's own append-only event log for this world.
func (p Profile) LauncherLog() string { return filepath.Join(p.LogDir(), launcherLogName) }

// ContractATokenFile is the local token the sidecar mints and the mod presents.
// It is not the map credential.
func (p Profile) ContractATokenFile() string {
	return filepath.Join(p.DataDir(), contractATokenName)
}

// GameExe is the game this profile launches.
func (p Profile) GameExe() string { return filepath.Join(p.GameDir, gameExeName) }

// PluginPath is the mod inside this profile's game directory.
func (p Profile) PluginPath() string {
	return filepath.Join(p.GameDir, filepath.FromSlash(pluginRelPath))
}

// BepInExLog is the mod framework's own log. It is SHARED by every world run
// out of one game folder, and there is no per-instance path to configure — see
// LOCAL-STARVATION in docs/error-taxonomy.md, which is why MaxLocalWorlds
// exists.
func (p Profile) BepInExLog() string {
	return filepath.Join(p.GameDir, "BepInEx", "LogOutput.log")
}

// fileExists answers only the question its name asks: a directory is not a file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists is the directory half of fileExists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
