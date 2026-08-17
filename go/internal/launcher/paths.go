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

	// cmdFileName is the mod's command channel for one world, named by
	// MULTIVERSE_CMD_FILE. cmdLogSuffix is not ours to choose: the mod appends
	// its answers to <command file>.log (bibites-mod/src/DevCommands.cs).
	cmdFileName  = "cmd.txt"
	cmdLogSuffix = ".log"
	// cmdTempSuffix is what a command is written to before it is renamed onto
	// cmdFileName, so the mod never sees half of one.
	cmdTempSuffix = ".tmp"

	dataDirName = "data"
	// runtimesDirName is where the COMPLETE EDITION puts the game it manages:
	// <data root>\runtimes\<assembly sha256>, written by
	// release/kit/Install-BibitesMultiverse.ps1 and by its Linux twin. It is the
	// one folder a world's data root is allowed to hold that is not the world's
	// own, and the only reason a game folder may sit inside a data root at all.
	runtimesDirName = "runtimes"
	// peerIDFileName is what the sidecar persists a world's identity in
	// (contract-b-m4.md 7.4) and what the installers write beside the journal. It
	// outlives an uninstall, so it is also how a data root says "a world lives
	// here" once its credential and its install record are gone.
	peerIDFileName = "peer-id"
	logDirName     = "logs"

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

// CommandFile is where this world's mod is asked to do something — today, only
// to save and quit. IT IS PER WORLD, under the world's own data root, because
// two worlds out of one game folder must not read each other's commands.
func (p Profile) CommandFile() string { return filepath.Join(p.DataRoot, cmdFileName) }

// CommandLogFile is where the mod answers. The launcher empties it before each
// request, so it is one exchange rather than a growing file.
func (p Profile) CommandLogFile() string { return p.CommandFile() + cmdLogSuffix }

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

// ManagedRuntimeRoot is <data root>\runtimes: where the complete edition's
// installer puts the game builds it manages. The launcher NEVER writes in it and
// never deletes it; it is named here so the validation and the deletion rules can
// point at the one folder that makes a game directory inside a data root
// legitimate.
func (p Profile) ManagedRuntimeRoot() string { return filepath.Join(p.DataRoot, runtimesDirName) }

// worldOwnedEntries is EXACTLY what the launcher and the installers put in a
// world's data root, and it is the whole of what --remove-world-data deletes.
//
// A DATA ROOT IS NOT ALWAYS ONLY THIS WORLD'S. The complete edition installs the
// GAME ITSELF under runtimes\; the installer leaves install-record.json there for
// Uninstall-BibitesMultiverse.ps1 to read; an interrupted install leaves
// peer-secret.txt.<stamp>.orphan, which is the only recoverable half of an
// identity no map can print again; a private map leaves relay-ca.crt; and a
// machine that has hosted more than one deployment can have whole sibling folders
// in it belonging to worlds this launcher has never heard of. A RemoveAll on the
// root took every one of them.
func worldOwnedEntries() []string {
	return []string{
		dataDirName,
		logDirName,
		credentialFileName,
		pendingFileName,
		sidecarPidFileName,
		gamePidFileName,
		lockFileName,
		cmdFileName,
		cmdFileName + cmdLogSuffix,
		cmdFileName + cmdTempSuffix,
	}
}

// removeWorldData deletes the entries above and NOTHING else, and removes the
// data root itself only when nothing is left in it. It names what it deleted and
// what it left, so "deleted" is never a claim about somebody else's files.
//
// Entry names are compared WITHOUT CASE, because the file system this runs on is:
// RUNTIMES\ is the same folder as runtimes\ and has to be as safe.
func removeWorldData(dataRoot string) (removed, kept []string, rootRemoved bool, err error) {
	entries, err := os.ReadDir(dataRoot)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing to delete is not a failure: the folder is already gone,
			// which is the state the caller asked for.
			return nil, nil, true, nil
		}
		return nil, nil, false, err
	}
	owned := make(map[string]bool)
	for _, name := range worldOwnedEntries() {
		owned[strings.ToLower(name)] = true
	}
	for _, entry := range entries {
		name := entry.Name()
		if !owned[strings.ToLower(name)] {
			kept = append(kept, name)
			continue
		}
		if err := os.RemoveAll(filepath.Join(dataRoot, name)); err != nil {
			return removed, append(kept, name), false, err
		}
		removed = append(removed, name)
	}
	if len(kept) > 0 {
		return removed, kept, false, nil
	}
	if err := os.Remove(dataRoot); err != nil {
		return removed, kept, false, err
	}
	return removed, kept, true, nil
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
