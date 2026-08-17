package launcher

import (
	"fmt"
	"os"
	slashpath "path"
	"path/filepath"
	"regexp"
	"strings"
)

// Every rule a world has to satisfy before the launcher will write it down.
// They are here rather than spread through the commands because the console
// menu, the command line and the installer's own first world all have to be
// judged by the same rules.

const (
	minPort = 1024
	maxPort = 65535

	maxWorldNameLength = 64
	maxSaveMinutes     = 1440.0
	maxSaveKeep        = 100
)

var (
	profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`)
	relayURLPattern    = regexp.MustCompile(`^wss://\S+$`)
	enrollURLPattern   = regexp.MustCompile(`^https://\S+$`)
	printableASCII     = regexp.MustCompile(`^[\x21-\x7e]+$`)

	// deviceNames are reserved by Windows in every directory, in any case, with
	// or without an extension. A file called CON cannot be created at all.
	deviceNames = map[string]bool{
		"CON": true, "PRN": true, "AUX": true, "NUL": true,
		"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
		"COM6": true, "COM7": true, "COM8": true, "COM9": true,
		"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
		"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
	}
)

// isDeviceName reports whether value is a Windows device name, judged the way
// Windows judges it: on the part before the first dot, ignoring case.
func isDeviceName(value string) bool {
	stem := value
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	return deviceNames[strings.ToUpper(strings.TrimSpace(stem))]
}

// validateProfileName checks the name of the profile FILE. It is deliberately
// narrower than the world name: it becomes a file name on a Windows disk.
func validateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("a world needs a name")
	}
	if !profileNamePattern.MatchString(name) {
		return fmt.Errorf("'%s' is not a usable world name. Use 1 to 32 letters, digits, dots, "+
			"dashes or underscores, starting with a letter or a digit", name)
	}
	if isDeviceName(name) {
		return fmt.Errorf("'%s' is a name Windows reserves for a device, so no file can carry it. "+
			"Choose another", name)
	}
	return nil
}

// validateProfileNameUnique adds the uniqueness half, compared the way the
// Windows file system compares: without case.
func validateProfileNameUnique(name string, others []Profile) error {
	if err := validateProfileName(name); err != nil {
		return err
	}
	if existing, found := findProfile(others, name); found {
		return fmt.Errorf("there is already a world called '%s'. Names are compared without case, "+
			"because the file system is", existing.Name)
	}
	return nil
}

// validateWorld checks the SAVE FILE name. Every copy of the game on this
// machine writes into one Savefiles directory for this user, so a world name
// has to be unique across every profile, not just per game folder.
func validateWorld(world string, others []Profile) error {
	if world == "" {
		return fmt.Errorf("a world needs a save name")
	}
	if len([]rune(world)) > maxWorldNameLength {
		return fmt.Errorf("the world name '%s' is longer than %d characters", world, maxWorldNameLength)
	}
	if strings.ContainsAny(world, `\/:*?"<>|`) {
		return fmt.Errorf(`the world name '%s' holds one of \ / : * ? " < > |, which a file name cannot`, world)
	}
	for _, r := range world {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("the world name '%s' holds a control character", world)
		}
	}
	if strings.TrimSpace(world) != world || strings.HasPrefix(world, ".") || strings.HasSuffix(world, ".") {
		return fmt.Errorf("the world name '%s' starts or ends with a space or a dot, which Windows "+
			"silently drops", world)
	}
	if isDeviceName(world) {
		return fmt.Errorf("'%s' is a name Windows reserves for a device. Choose another", world)
	}
	for _, other := range others {
		if strings.EqualFold(other.World, world) {
			return fmt.Errorf("the world '%s' is already used by the profile '%s'. Every world on this "+
				"machine saves into one folder, so two worlds cannot share a save name",
				world, other.Name)
		}
	}
	return nil
}

// validatePort checks the range and the uniqueness. Whether the port can
// actually be bound is a separate question, asked at start time.
func validatePort(port int, others []Profile) error {
	if port < minPort || port > maxPort {
		return fmt.Errorf("the sidecar port %d is outside %d-%d", port, minPort, maxPort)
	}
	for _, other := range others {
		if other.SidecarPort == port {
			// THE WORD IS "WORLD" AND NOT "PROFILE". A profile is the file a
			// world is written down in, and this sentence is read by somebody who
			// has one world and is making another — including in the graphical
			// launcher, which quotes the core's refusals into its panel verbatim
			// rather than paraphrasing them.
			return fmt.Errorf("the world '%s' already uses port %d. Every world needs its own",
				other.Name, port)
		}
	}
	return nil
}

// minDataRootElements is the shallowest data root the launcher will accept.
// `C:\` and `D:\Games` are not world data roots, and --remove-world-data would
// take the whole of either. Two elements below the volume root is the floor.
const minDataRootElements = 2

// validateDataRoot keeps two worlds from writing into one journal, and keeps
// any world from writing inside a game folder, inside the installed
// application, or into a place a later --remove-world-data would be catastrophic
// to delete.
//
// ownGameDir is the game folder of the profile BEING validated. It is a
// separate argument because `others` deliberately excludes that profile, and
// leaving its own game folder unchecked was how a data root could be pointed at
// a Steam installation that --remove-world-data then removed.
func validateDataRoot(dataRoot, ownGameDir string, others []Profile, install Install) error {
	if strings.TrimSpace(dataRoot) == "" {
		return fmt.Errorf("a world needs its own data folder")
	}
	if !isAbsolutePath(dataRoot) {
		return fmt.Errorf("the data folder '%s' is a relative path, so it would mean whatever "+
			"folder this program happens to be run from. Give the whole path", dataRoot)
	}
	clean := filepath.Clean(dataRoot)
	if depth := pathDepth(clean); depth < minDataRootElements {
		return fmt.Errorf("the data folder '%s' is at the top of a drive. A world's data folder "+
			"needs at least %d folders below the drive, because deleting a world deletes it",
			clean, minDataRootElements)
	}
	if ownGameDir != "" && pathsOverlap(clean, ownGameDir) && !isManagedRuntime(clean, ownGameDir) {
		return fmt.Errorf("the data folder '%s' overlaps this world's own game folder '%s'. The "+
			"launcher never writes inside a game folder, and deleting the world would take the "+
			"game with it", clean, ownGameDir)
	}
	for _, other := range others {
		if pathsOverlap(clean, other.DataRoot) {
			return fmt.Errorf("the data folder '%s' overlaps the profile '%s' at '%s'. Two worlds "+
				"sharing a journal is how custody history is lost", clean, other.Name, other.DataRoot)
		}
		// A second world in the complete edition runs the SAME managed runtime,
		// which lives under the FIRST world's data root. That is the installer's
		// own layout, not an accident, so it is the one overlap allowed here too.
		if other.GameDir != "" && pathsOverlap(clean, other.GameDir) && !isManagedRuntime(clean, other.GameDir) {
			return fmt.Errorf("the data folder '%s' overlaps the game folder of the profile '%s'. "+
				"The launcher never writes inside a game folder", clean, other.Name)
		}
	}
	if pathsOverlap(clean, install.Root) {
		return fmt.Errorf("the data folder '%s' overlaps '%s', where this program is installed. "+
			"Deleting the world would take the launcher, the sidecar and every other world's "+
			"profile with it", clean, install.Root)
	}
	if err := reachableDir(clean); err != nil {
		return err
	}
	return nil
}

// isAbsolutePath is filepath.IsAbs plus the Windows forms, so a profile written
// by the Windows installer can be read and checked anywhere. A relative path is
// the thing being refused, and `C:\...` or `\\server\share` is never one.
func isAbsolutePath(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	if strings.HasPrefix(path, `\\`) {
		return true
	}
	if len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		letter := path[0]
		return (letter >= 'A' && letter <= 'Z') || (letter >= 'a' && letter <= 'z')
	}
	return false
}

// pathDepth counts the elements below a volume root, so `C:\` and `/` are 0.
func pathDepth(path string) int {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	rest := strings.Trim(strings.TrimPrefix(clean, volume), `\/`)
	if rest == "" {
		return 0
	}
	return len(strings.FieldsFunc(rest, func(r rune) bool { return r == '\\' || r == '/' }))
}

// reachableDir checks that a directory exists or could be created, WITHOUT
// creating it: walk up to the first path that exists, and require it to be a
// directory. Validation that writes is validation nobody can run twice.
func reachableDir(path string) error {
	for current := path; ; {
		info, err := os.Stat(current)
		if err == nil {
			if info.IsDir() {
				return nil
			}
			return fmt.Errorf("'%s' is a file, so '%s' cannot be created under it", current, path)
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("'%s' cannot be read: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("'%s' is on a drive or a root that does not exist here", path)
		}
		current = parent
	}
}

// pathsOverlap reports whether two paths are the same, or one contains the
// other. Comparison folds case, because the Windows file system does.
func pathsOverlap(a, b string) bool {
	return pathWithin(a, b) || pathWithin(b, a)
}

// foldPath is the comparable form of a path: BOTH separators folded to '/', the
// path cleaned, and the case folded.
//
// FOLDING BOTH SEPARATORS IS THE POINT. The Windows installer writes a profile
// full of backslashes, and that profile is judged here — including on Linux,
// where every anti-drift test that reads the installer's own fixture runs.
// With filepath.Clean alone, a backslash is an ordinary character off
// Windows, so `C:\Data` "did not contain" `C:\Data\runtimes\<build>` and every
// overlap rule silently passed a file the real machine refuses. The rest of this
// file already reads both forms (isAbsolutePath, pathDepth); this is the half
// that did not.
func foldPath(path string) string {
	if path == "" {
		return ""
	}
	return strings.ToLower(slashpath.Clean(strings.ReplaceAll(path, `\`, "/")))
}

// pathWithin reports whether inner is root or lives under it. It is
// separator-aware, so /data/world does not "contain" /data/world2.
func pathWithin(inner, root string) bool {
	i, r := foldPath(inner), foldPath(root)
	if i == "" || r == "" {
		return false
	}
	if i == r {
		return true
	}
	if !strings.HasSuffix(r, "/") {
		r += "/"
	}
	return strings.HasPrefix(i, r)
}

// isManagedRuntime reports whether gameDir is a game the COMPLETE EDITION
// installed under this world's own data root: <data root>\runtimes\<build>.
//
// That layout is the only overlap of a data root and a game folder the launcher
// accepts, and it is accepted because the launcher owns both halves of it. The
// rule the overlap check exists for is unchanged: the launcher never writes
// inside a game folder (it writes data\, logs\ and the pid ledger, all beside
// runtimes\ and never in it), and --remove-world-data deletes only the entries
// worldOwnedEntries names, which runtimes\ is deliberately not one of. Every
// other overlap — a data root on a Steam folder, on the installed application,
// on a neighbour's journal — is still refused with the message it always had.
func isManagedRuntime(dataRoot, gameDir string) bool {
	root, inner := foldPath(dataRoot), foldPath(gameDir)
	if root == "" || inner == "" {
		return false
	}
	// <data root>\runtimes itself is not a game folder: a build directory below
	// it is. Requiring the extra element keeps "the data root is the game folder"
	// and "the game folder is runtimes\" out of the exception.
	return strings.HasPrefix(inner, root+"/"+runtimesDirName+"/")
}

// validateGameDir checks the folder actually holds the game, and — when strict,
// which is at start time — that the mod is in it. At create time a missing mod
// is a warning, because a participant may be pointing at a folder they are
// about to install into.
func validateGameDir(gameDir string, strict bool) error {
	if gameDir == "" {
		return fmt.Errorf("a world needs the folder the game is installed in")
	}
	if !isAbsolutePath(gameDir) {
		return fmt.Errorf("the game folder '%s' is a relative path. Give the whole path", gameDir)
	}
	if !fileExists(filepath.Join(gameDir, gameExeName)) {
		return fmt.Errorf("'%s' does not hold %s, so it is not a game folder", gameDir, gameExeName)
	}
	if strict && !fileExists(filepath.Join(gameDir, filepath.FromSlash(pluginRelPath))) {
		return fmt.Errorf("'%s' has no %s. The game would start, join nothing, and look healthy. "+
			"Re-run the installer against this game folder", gameDir, pluginRelPath)
	}
	return nil
}

// normalizeExportEdges parses the edge list exactly as
// Install-BibitesMultiverse.ps1 does: split on commas, semicolons and
// whitespace, upper-case, each token one of E N W S, no repeats, at least one.
func normalizeExportEdges(value string) (string, error) {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t'
	})
	if len(fields) == 0 {
		return "", fmt.Errorf("the export edges name no edge. Use E, N, W or S, comma separated - " +
			"normally 'E,N,W,S'. If you want this world off the map, do not start it")
	}
	seen := make(map[string]bool, len(fields))
	edges := make([]string, 0, len(fields))
	for _, field := range fields {
		edge := strings.ToUpper(field)
		switch edge {
		case "E", "N", "W", "S":
		default:
			return "", fmt.Errorf("the export edges hold '%s'. Use E, N, W or S", edge)
		}
		if seen[edge] {
			return "", fmt.Errorf("the export edges repeat an edge: '%s'", value)
		}
		seen[edge] = true
		edges = append(edges, edge)
	}
	return strings.Join(edges, ","), nil
}

// resolveExcludeSpecies enforces that turning the exclusion policy off is an
// explicit act with its own switch, the way the installer does.
func resolveExcludeSpecies(value string, noExclusion bool) (string, error) {
	if noExclusion {
		return "", nil
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("--exclude-species is empty, which would turn the exclusion policy off. " +
			"That is a real choice and it takes its own switch: pass --no-migration-exclusion if you mean it")
	}
	return value, nil
}

// noExclusionWarning is the same warning the installer prints when the policy
// is off, so the two front doors say the same thing.
const noExclusionWarning = `  species that       NONE - the exclusion policy is OFF
  never leave        you asked for --no-migration-exclusion. Your world will export
                     starter stock onto a shared map, and the map's census shows
                     that as entirely normal while it happens.`

func validateSaveMinutes(minutes float64) error {
	if minutes < 0 || minutes > maxSaveMinutes {
		return fmt.Errorf("--save-minutes is %g; it must be between 0 and %g. 0 turns the timer off",
			minutes, maxSaveMinutes)
	}
	return nil
}

func validateSaveKeep(keep int) error {
	if keep < 0 || keep > maxSaveKeep {
		return fmt.Errorf("--save-keep is %d; it must be between 0 and %d", keep, maxSaveKeep)
	}
	return nil
}

// parseOnOff reads the on|off spelling the installer uses for save-on-quit.
func parseOnOff(flagName, value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "yes", "1":
		return true, nil
	case "off", "false", "no", "0":
		return false, nil
	}
	return false, fmt.Errorf("%s takes 'on' or 'off', not '%s'", flagName, value)
}

func validateRelayURL(relayURL string) error {
	if relayURLPattern.MatchString(relayURL) {
		return nil
	}
	if strings.HasPrefix(relayURL, "ws://") {
		return fmt.Errorf("the relay address is ws://, which is not encrypted. This wire is always " +
			"wss:// and there is no plain fallback anywhere in this software: a relay that answers " +
			"ws:// off loopback refuses the connection rather than serving it. Ask your operator for " +
			"the wss:// address")
	}
	return fmt.Errorf("the relay address '%s' is not a wss:// URL", relayURL)
}

// validateNewProfile checks everything about a world EXCEPT its map identity,
// which does not exist yet when a world is being created. Enrollment happens
// after this passes, so an invalid world never costs an identity on the map.
func validateNewProfile(p Profile, others []Profile, install Install) error {
	if p.Format != ProfileFormat {
		return fmt.Errorf("a world profile carries format '%s'", ProfileFormat)
	}
	if err := validateProfileNameUnique(p.Name, others); err != nil {
		return err
	}
	if err := validateWorld(p.World, others); err != nil {
		return err
	}
	if err := validatePort(p.SidecarPort, others); err != nil {
		return err
	}
	if err := validateDataRoot(p.DataRoot, p.GameDir, others, install); err != nil {
		return err
	}
	// The game folder is checked on EVERY write, not only on create: 'profile
	// set --game-dir' reached the file without it, so a mistyped folder was
	// accepted silently and only surfaced as a refusal at the next start.
	if err := validateGameDir(p.GameDir, false); err != nil {
		return err
	}
	if _, err := normalizeExportEdges(p.ExportEdges); err != nil {
		return err
	}
	if err := validateSaveMinutes(p.SaveMinutes); err != nil {
		return err
	}
	if err := validateSaveKeep(p.SaveKeep); err != nil {
		return err
	}
	return nil
}

// validateProfile is the whole-object check every write goes through, so a
// profile on disk is never one the launcher would refuse to start.
func validateProfile(p Profile, others []Profile, install Install) error {
	if err := validateNewProfile(p, others, install); err != nil {
		return err
	}
	if err := validateRelayURL(p.RelayURL); err != nil {
		return err
	}
	if p.PeerID == "" {
		return fmt.Errorf("a world profile carries the identity half of its map credential")
	}
	return nil
}

// validateProfileStructure is what EVERY profile read from disk must satisfy,
// with no knowledge of the installation around it. A hand-edited or truncated
// file reaches this before any code joins a path onto it: an empty or relative
// dataRoot would otherwise resolve against whatever folder the program was
// started from, which is how `logs\` appeared in a shortcut's "Start in"
// directory and how --remove-world-data could delete a folder there.
func validateProfileStructure(p Profile) error {
	if p.Format != ProfileFormat {
		return fmt.Errorf("its format is '%s'; this launcher reads '%s' only", p.Format, ProfileFormat)
	}
	if err := validateProfileName(p.Name); err != nil {
		return err
	}
	if strings.TrimSpace(p.World) == "" {
		return fmt.Errorf("its 'world' is empty, so no save file is named")
	}
	if strings.TrimSpace(p.DataRoot) == "" {
		return fmt.Errorf("its 'dataRoot' is empty")
	}
	if !isAbsolutePath(p.DataRoot) {
		return fmt.Errorf("its 'dataRoot' ('%s') is a relative path, so it would mean whatever "+
			"folder this program happens to be run from", p.DataRoot)
	}
	if pathDepth(p.DataRoot) < minDataRootElements {
		return fmt.Errorf("its 'dataRoot' ('%s') is at the top of a drive", p.DataRoot)
	}
	if strings.TrimSpace(p.GameDir) == "" {
		return fmt.Errorf("its 'gameDir' is empty")
	}
	if !isAbsolutePath(p.GameDir) {
		return fmt.Errorf("its 'gameDir' ('%s') is a relative path", p.GameDir)
	}
	if p.SidecarPort < minPort || p.SidecarPort > maxPort {
		return fmt.Errorf("its 'sidecarPort' (%d) is outside %d-%d", p.SidecarPort, minPort, maxPort)
	}
	return nil
}

// validateProfilePaths is the half of the load-time check that needs to know
// where this program is installed. It runs on every profile the launcher loads,
// so a file that was hand-edited into a dangerous shape is refused at the point
// it is read rather than at the point it is acted on.
func validateProfilePaths(p Profile, install Install) error {
	if pathsOverlap(p.DataRoot, install.Root) {
		return fmt.Errorf("its 'dataRoot' ('%s') overlaps '%s', where this program is installed",
			p.DataRoot, install.Root)
	}
	// The complete edition installs the game INSIDE the data root, at
	// <data root>\runtimes\<build>, and writes exactly that pair into
	// profiles\default.json. Refusing it made every complete-edition install
	// produce a profile the launcher would not read, so the icons opened a
	// launcher with no worlds in it. Every other overlap is still refused.
	if pathsOverlap(p.DataRoot, p.GameDir) && !isManagedRuntime(p.DataRoot, p.GameDir) {
		return fmt.Errorf("its 'dataRoot' ('%s') overlaps its own game folder ('%s')",
			p.DataRoot, p.GameDir)
	}
	return nil
}

// validateRemovable is the last gate before os.RemoveAll. The writer's rules
// are re-run here on the LOADED profile, because a profile file can be edited
// by hand between the write and the delete, and this is the one command in the
// launcher that destroys a directory tree.
func validateRemovable(p Profile, profiles []Profile, install Install) error {
	if err := validateProfileStructure(p); err != nil {
		return fmt.Errorf("this world's profile is not one the launcher will delete a folder for: %w", err)
	}
	if err := validateDataRoot(p.DataRoot, p.GameDir, otherProfiles(profiles, p.Name), install); err != nil {
		return err
	}
	// Every game folder on this installation, not only this world's, because a
	// data root that landed on a neighbour's game folder is the same accident.
	for _, other := range profiles {
		if other.GameDir == "" || !pathsOverlap(p.DataRoot, other.GameDir) {
			continue
		}
		// The complete edition's managed runtime under this very data root is the
		// exception, and it is safe here for the reason it is safe anywhere:
		// removeWorldData deletes the world's own entries and leaves runtimes\
		// alone, so the game every world on this machine runs survives.
		if isManagedRuntime(p.DataRoot, other.GameDir) {
			continue
		}
		return fmt.Errorf("the data folder '%s' overlaps the game folder of the profile '%s'",
			p.DataRoot, other.Name)
	}
	return nil
}
