// Package launcher is the installed entry point of a Bibites Multiverse world:
// the program the Windows shortcuts open, and the program that owns the
// per-world profiles under <install root>\profiles.
//
// WHY IT EXISTS. Until this release one install meant one world: one map
// identity, one data root, one sidecar on one port, started by a generated
// PowerShell script. A participant who wanted a second world on the same
// machine had no supported way to get one, and a participant who wanted a
// headless world had to know a switch on a script. The launcher makes both
// ordinary, and it is a real installed program rather than a shortcut to a
// script, so Windows shows it in Add/Remove Programs like anything else.
//
// WHY IT IS A CONSOLE PROGRAM. The menu is rendered to an injected writer and
// read from an injected reader, so every frame of the interface is exercised by
// a test on a machine that has no Windows at all. A native window, a local web
// server or a PowerShell form would all have moved the front door into code
// that cannot be tested here.
//
// The generated Start-Multiverse.ps1 and Stop-Multiverse.ps1 keep working. The
// two paths interoperate through the on-disk PID protocol rather than through
// shared code.
package launcher

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Release is the release this launcher was built for. It is pinned to
// docs/support-matrix.md by a test.
const Release = "0.2.8"

// Exit codes, matching the sidecar: 0 success, 1 a refusal or a runtime
// failure, 2 a usage error (flag.ContinueOnError's own answer).
const (
	exitOK      = 0
	exitRefused = 1
	exitUsage   = 2
)

// enrollmentTimeout bounds the one network call this program makes.
const enrollmentTimeout = 30 * time.Second

// app is one run of the launcher. Everything the program touches that is not a
// file — the clock, the environment, the terminal and the network — arrives
// here, so a test can drive the whole surface.
type app struct {
	install Install
	stdin   *bufio.Reader
	stdout  io.Writer
	stderr  io.Writer

	interactive bool
	asJSON      bool
	assumeYes   bool
	quiet       bool

	now    func() time.Time
	getenv func(string) string
	client *http.Client
}

// Main is the launcher entry point, factored out of package main so the tests
// can run the same code path with injected streams.
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return run(args, stdin, stdout, stderr, os.Getenv, os.Executable, time.Now)
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer,
	getenv func(string) string, executable func() (string, error), now func() time.Time) int {

	// The reader and the terminal answer are settled FIRST, so a failure before
	// the menu exists still holds a double-clicked console open long enough to
	// be read.
	reader := bufio.NewReader(stdin)
	interactive := isTerminal(stdin)
	fatal := func(code int, format string, args ...any) int {
		fmt.Fprintf(stderr, format+"\n", args...)
		if interactive {
			fmt.Fprint(stdout, "Press Enter to close. ")
			reader.ReadString('\n')
		}
		return code
	}

	globals, rest, err := splitGlobals(args)
	if err != nil {
		return fatal(exitUsage, "%v", err)
	}
	if globals.help {
		fmt.Fprint(stdout, usageText)
		return exitOK
	}
	root, err := discoverInstallRoot(globals.installRoot, getenv, executable)
	if err != nil {
		return fatal(exitRefused, "%v", err)
	}
	a := &app{
		install:     Install{Root: root},
		stdin:       reader,
		stdout:      stdout,
		stderr:      stderr,
		interactive: interactive,
		asJSON:      globals.asJSON,
		assumeYes:   globals.assumeYes,
		quiet:       globals.quiet,
		now:         now,
		getenv:      getenv,
		client:      enrollmentClient(),
	}
	return a.dispatch(rest)
}

// enrollmentClient is the one HTTP client this program has.
//
// IT REFUSES EVERY REDIRECT. The request body carries the only recoverable
// secret in the system, and Go replays a POST body verbatim on 307 and 308 —
// so a hijacked DNS answer, a captive portal or a compromised edge could move
// the enrollment POST to plain http and read the credential in clear. The
// documented endpoint is a single POST that never redirects, and every
// enrollment failure keeps the pending record, so refusing is a safe retry.
func enrollmentClient() *http.Client {
	return &http.Client{
		Timeout: enrollmentTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return fmt.Errorf("the enrollment address redirected to %s; this launcher does not "+
				"follow a redirect with a credential in the request", req.URL.Redacted())
		},
	}
}

// isTerminal decides whether the bare command opens the menu. A file that is
// not a character device is a pipe or a redirect, and a program that blocks on
// a menu nobody can answer is a program that hangs. A reader that is not a file
// at all is a test driving the menu deliberately.
func isTerminal(r io.Reader) bool {
	file, ok := r.(*os.File)
	if !ok {
		return true
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func (a *app) dispatch(args []string) int {
	if len(args) == 0 {
		return a.runDefault()
	}
	switch args[0] {
	case "start":
		return a.commandStart(args[1:])
	case "stop":
		return a.commandStop(args[1:])
	case "status":
		return a.commandStatus(args[1:])
	case "profile":
		return a.commandProfile(args[1:])
	case "version":
		a.print("Bibites Multiverse launcher %s", Release)
		return exitOK
	case "help":
		return a.commandHelp(args[1:])
	default:
		a.warn("'%s' is not a command this launcher has.", args[0])
		fmt.Fprint(a.stderr, usageText)
		return exitUsage
	}
}

// runDefault is what a double-clicked shortcut does: the menu on a terminal,
// and the status plus the usage anywhere else. It never blocks on a reader
// nobody is watching.
func (a *app) runDefault() int {
	if a.interactive {
		return a.runMenu()
	}
	if profiles, err := a.install.ListProfiles(); err == nil && len(profiles) > 0 {
		a.runStatus("", true)
		a.print("")
	}
	fmt.Fprint(a.stdout, usageText)
	return exitOK
}

// ---------------------------------------------------------------- global flags

type globalFlags struct {
	installRoot string
	asJSON      bool
	assumeYes   bool
	quiet       bool
	help        bool
}

// splitGlobals pulls the global flags out of the whole command line, wherever
// they appear, and leaves everything else for the command's own flag set. Go's
// flag package stops at the first non-flag argument, so '--json' after a
// command name would otherwise be an error rather than a global.
func splitGlobals(args []string) (globalFlags, []string, error) {
	var g globalFlags
	takesValue := commandValueFlags()
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		var name string
		switch {
		case strings.HasPrefix(arg, "--") && len(arg) > 2:
			name = arg[2:]
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			name = arg[1:]
		default:
			rest = append(rest, arg)
			continue
		}
		value := ""
		hasValue := false
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name, value, hasValue = name[:eq], name[eq+1:], true
		}
		switch name {
		case "install-root":
			if !hasValue {
				i++
				if i >= len(args) {
					return g, nil, fmt.Errorf("--install-root needs the path of the installed folder")
				}
				value = args[i]
			}
			// A missing value silently swallows the next token, and the failure
			// then arrives as "'--all' is not a command" - a diagnosis of the
			// wrong thing. Neither a flag nor a command name is a folder.
			if strings.HasPrefix(value, "-") || commandNames[value] {
				return g, nil, fmt.Errorf("--install-root needs the path of the installed folder, "+
					"and '%s' is not one", value)
			}
			g.installRoot = value
		case "json":
			parsed, err := globalBool(name, value, hasValue)
			if err != nil {
				return g, nil, err
			}
			g.asJSON = parsed
		case "yes":
			parsed, err := globalBool(name, value, hasValue)
			if err != nil {
				return g, nil, err
			}
			g.assumeYes = parsed
		case "quiet":
			parsed, err := globalBool(name, value, hasValue)
			if err != nil {
				return g, nil, err
			}
			g.quiet = parsed
		case "help", "h":
			g.help = true
		default:
			rest = append(rest, arg)
			// A flag that takes a value carries its value with it, untouched.
			// Without this, `profile set x --world -h` would be read as
			// `--help` and the world would silently not be renamed.
			if !hasValue && takesValue[name] && i+1 < len(args) {
				i++
				rest = append(rest, args[i])
			}
		}
	}
	return g, rest, nil
}

// commandNames is what dispatch answers to. splitGlobals uses it to tell a
// missing --install-root value from a real path.
var commandNames = map[string]bool{
	"start": true, "stop": true, "status": true, "profile": true,
	"version": true, "help": true,
}

func globalBool(name, value string, hasValue bool) (bool, error) {
	if !hasValue {
		return true, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("--%s takes true or false, not '%s'", name, value)
	}
	return parsed, nil
}

// ---------------------------------------------------------------- commands

func newFlagSet(name string, out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	return fs
}

// parseInterleaved lets flags and positional arguments mix, which is what a
// participant types. flag.Parse stops at the first positional, so it is called
// again after each one.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

type startFlagSet struct {
	profile    *string
	headless   *bool
	noHeadless *bool
	gameOnly   *bool
	wait       *int
	dryRun     *bool
}

func registerStartFlags(fs *flag.FlagSet) *startFlagSet {
	return &startFlagSet{
		profile:    fs.String("profile", "", "the world to start; the active world by default"),
		headless:   fs.Bool("headless", false, "run with nothing drawn. The simulation is unchanged; only the picture is gone"),
		noHeadless: fs.Bool("no-headless", false, "draw the world even when this profile is set headless"),
		gameOnly:   fs.Bool("game-only", false, "start the game only, against a sidecar already running"),
		wait:       fs.Int("wait", int(defaultSlotWait/time.Second), "seconds to wait for the map to grant this world a place"),
		dryRun:     fs.Bool("dry-run", false, "print what would be started, and start nothing"),
	}
}

func (a *app) commandStart(args []string) int {
	fs := newFlagSet("start", a.stderr)
	flags := registerStartFlags(fs)
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return exitUsage
	}
	name, code := a.oneProfileName("start", *flags.profile, positional)
	if code != exitOK {
		return code
	}
	if *flags.headless && *flags.noHeadless {
		return a.usageError("--headless and --no-headless disagree")
	}
	if *flags.wait < 0 {
		return a.usageError("--wait is %d; it must be 0 or more. 0 looks once and does not wait",
			*flags.wait)
	}
	p, err := a.install.ResolveProfile(name)
	if err != nil {
		return a.fail("%v", err)
	}
	headless := p.Headless
	if *flags.headless {
		headless = true
	}
	if *flags.noHeadless {
		headless = false
	}
	return a.runStart(p, startOptions{
		headless: headless,
		gameOnly: *flags.gameOnly,
		wait:     time.Duration(*flags.wait) * time.Second,
		dryRun:   *flags.dryRun,
	})
}

type stopFlagSet struct {
	profile  *string
	gameOnly *bool
	all      *bool
	timeout  *int
}

func registerStopFlags(fs *flag.FlagSet) *stopFlagSet {
	return &stopFlagSet{
		profile:  fs.String("profile", "", "the world to stop; the active world by default"),
		gameOnly: fs.Bool("game-only", false, "stop the game and leave the sidecar holding this world's place"),
		all:      fs.Bool("all", false, "stop every world this installation knows about"),
		timeout:  fs.Int("timeout", 0, "seconds to wait for a process to close before forcing it"),
	}
}

func (a *app) commandStop(args []string) int {
	fs := newFlagSet("stop", a.stderr)
	flags := registerStopFlags(fs)
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return exitUsage
	}
	opts := stopOptions{gameOnly: *flags.gameOnly, timeout: time.Duration(*flags.timeout) * time.Second}
	if *flags.all {
		if len(positional) > 0 || *flags.profile != "" {
			return a.usageError("stop --all stops every world, so it takes no world name")
		}
		return a.runStopAll(opts)
	}
	name, code := a.oneProfileName("stop", *flags.profile, positional)
	if code != exitOK {
		return code
	}
	p, err := a.install.ResolveProfile(name)
	if err != nil {
		return a.fail("%v", err)
	}
	return a.runStop(p, opts)
}

func (a *app) commandStatus(args []string) int {
	fs := newFlagSet("status", a.stderr)
	profile := fs.String("profile", "", "the world to report on; the active world by default")
	all := fs.Bool("all", false, "report on every world")
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return exitUsage
	}
	if *all {
		if len(positional) > 0 || *profile != "" {
			return a.usageError("status --all reports every world, so it takes no world name")
		}
		return a.runStatus("", true)
	}
	name, code := a.oneProfileName("status", *profile, positional)
	if code != exitOK {
		return code
	}
	return a.runStatus(name, false)
}

// oneProfileName reconciles the positional world name with --profile.
func (a *app) oneProfileName(command, flagValue string, positional []string) (string, int) {
	switch len(positional) {
	case 0:
		return flagValue, exitOK
	case 1:
		if flagValue != "" && !strings.EqualFold(flagValue, positional[0]) {
			return "", a.usageError("%s was given two different worlds: '%s' and '%s'",
				command, positional[0], flagValue)
		}
		return positional[0], exitOK
	default:
		return "", a.usageError("%s takes at most one world name, and got %d",
			command, len(positional))
	}
}

func (a *app) commandHelp(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(a.stdout, usageText)
		return exitOK
	}
	topic := strings.Join(args, " ")
	if text, found := commandHelp[topic]; found {
		fmt.Fprint(a.stdout, text)
		return exitOK
	}
	a.warn("there is no help for '%s'.", topic)
	fmt.Fprint(a.stderr, usageText)
	return exitUsage
}

// ---------------------------------------------------------------- output

func (a *app) print(format string, args ...any) {
	fmt.Fprintf(a.stdout, format+"\n", args...)
}

// say is progress chatter, which --quiet suppresses.
func (a *app) say(format string, args ...any) {
	if !a.quiet {
		a.print(format, args...)
	}
}

func (a *app) warn(format string, args ...any) {
	fmt.Fprintf(a.stderr, format+"\n", args...)
}

func (a *app) fail(format string, args ...any) int {
	a.warn(format, args...)
	return exitRefused
}

func (a *app) usageError(format string, args ...any) int {
	a.warn(format, args...)
	return exitUsage
}

// ask reads one line. A reader at its end answers empty, which every caller
// treats as "no".
func (a *app) ask(prompt string) string {
	line, _ := a.askLine(prompt)
	return line
}

// confirm asks a yes-or-no question. --yes answers every one of them.
func (a *app) confirm(prompt string) bool {
	if a.assumeYes {
		return true
	}
	answer := strings.ToLower(a.ask(prompt))
	return answer == "y" || answer == "yes"
}

// ---------------------------------------------------------------- usage

// usageText is the frozen command surface. The documentation is written against
// it character for character.
const usageText = `Bibites Multiverse launcher ` + Release + `

  ` + LauncherExeName + ` [global flags] [command] [args]

Global flags (accepted anywhere on the line):
  --install-root PATH    default: the directory of this executable; env MULTIVERSE_LAUNCHER_HOME
  --json                 machine-readable output for commands that support it
  --yes                  answer yes to every confirmation
  --quiet                suppress progress chatter
  --help, -h             usage

Commands:
  (none)                 interactive menu on a terminal; ` + "`status --all`" + ` + usage otherwise
  start  [NAME]  [--profile NAME] [--headless|--no-headless] [--game-only]
                 [--wait SECONDS] [--dry-run]
  stop   [NAME]  [--profile NAME] [--game-only] [--all] [--timeout SECONDS]
  status [NAME]  [--profile NAME] [--all]
  profile list
  profile show   NAME
  profile use    NAME
  profile create NAME  <creation flags>
  profile clone  SRC NAME  <creation flags>
  profile set    NAME  <mutable flags>
  profile delete NAME  [--remove-world-data]
  version
  help [COMMAND]

Creation flags:
  --data-root PATH  --sidecar-port N  --world NAME  --game-dir PATH  --headless
  --export-edges LIST  --exclude-species LIST  --no-migration-exclusion
  --save-minutes F  --save-keep N  --save-on-quit on|off
  --join-string-file PATH        private map: the operator's one-line join string
  --relay-url wss://HOST/PATH    only with a join file that holds the identity half alone

Mutable flags (profile set): every creation flag EXCEPT --data-root, --join-string-file
  and --relay-url. dataRoot, peerId and relayUrl are immutable after create.
`

// commandHelp is what 'help COMMAND' prints. Each entry says what the command
// spends, not only what it takes.
var commandHelp = map[string]string{
	"start": `start [NAME] [--profile NAME] [--headless|--no-headless] [--game-only] [--wait SECONDS] [--dry-run]

Starts one world: the sidecar first, then the game. THE ORDER MATTERS - the sidecar mints the
local token the game's mod presents, the first time it starts.

The launcher waits for the map to grant this world a place before it starts the game, and prints
what to do when it does not. --dry-run prints the exact command line and the exact environment a
real start would use, and starts nothing.

At most 5 worlds can run out of one game folder. The mod framework hands out that many log
files, and an instance that gets none does not merely lose its log - the mod never loads in it
(LOCAL-STARVATION in docs/error-taxonomy.md).
`,
	"stop": `stop [NAME] [--profile NAME] [--game-only] [--all] [--timeout SECONDS]

Stops one world: the game first, then the sidecar. Each is asked to close before it is forced,
so the world's save-on-quit runs. A headless world has no window to close and is forced after
the timeout, which can lose the last interval.

--game-only leaves the sidecar up. It keeps this world's place on the map and keeps taking
custody of everything that arrives while the world is away.
`,
	"status": `status [NAME] [--profile NAME] [--all]

Prints what the launcher knows about a world, or about every world: its settings, its identity
on the map, and whether its sidecar and its game are running. --json prints the same thing in a
stable machine-readable shape. Neither form ever contains a secret.
`,
	"profile": `profile list | show NAME | use NAME | create NAME | clone SRC NAME | set NAME | delete NAME

A profile is one world on this machine: its own identity on the map, its own data folder, its own
sidecar port and its own save name.

create enrolls a NEW identity on the map this installation is already on. Every world you add
takes another identity, and the map applies a per-address enrollment limit.

delete removes the profile and, with --remove-world-data, its data folder. Deleting a world here
is not leaving the map - see docs/participant/leave.md.

dataRoot, peerId and relayUrl are immutable after create, because changing any of them would
point one world's identity at another world's journal.
`,
}
