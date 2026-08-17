package launcher

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Asking a world to QUIT THROUGH ITS OWN MOD, which is the only stop a headless
// world survives without losing anything.
//
// The operating system's close request is a window message. A game started with
// -batchmode -nographics has no window, so on Windows there is nothing to post
// it to, the stop forces the process, and the game never reaches its own
// shutdown — that is LOCAL-HEADLESSSTOP, and until this file existed the only
// remedies were a short save interval or a session run with a window.
//
// The mod already has a channel that does not go through a window:
// MULTIVERSE_CMD_FILE (bibites-mod/src/DevCommands.cs). The launcher writes one
// command into it, the mod polls that path every 200 ms, executes the verb and
// appends its answer to <command file>.log. The 'quit' verb answers OK, waits
// half a second and calls Application.Quit(), which is the ordinary Unity
// shutdown — the same one a window close would have started, and the one
// WorldSaver.OnApplicationQuit writes '[M4-SAVE] event=SAVED why=quit' from.
//
// THE PROTOCOL IS THE MOD'S, NOT OURS. Everything below is written to
// DevCommands.cs as it stands:
//   - a command is '<token> <verb> [args]' and MUST end in a newline; the mod
//     treats a file that does not as a partial write and leaves it for the next
//     poll, which is why the file is renamed into place rather than written in
//     place;
//   - the token is the first whitespace-separated field, so it may be any string
//     that holds no whitespace;
//   - the mod DELETES the command file once it has taken it, which is how the
//     launcher tells "nobody is reading this" from "it has not answered yet";
//   - the answer is one appended line, '<token> OK|ERROR <details>'.
//
// AN OLD WORLD CANNOT BE ASKED. The mod reads MULTIVERSE_CMD_FILE once, at
// plugin start, so a world that was already running before this launcher set the
// variable has no consumer for that file and never will. That is neither an
// error nor a hang: nothing takes the file, the launcher takes its own request
// back within a second, says why, and stops the world the way it always did.

// modQuitVerb is the verb DevCommands answers with a save-and-quit.
const modQuitVerb = "quit"

// cmdFileEnvName is the variable the mod reads the command file's path from. It
// is spelled here as well as in multiverseEnv because the stop path has to name
// it to somebody whose world was started without it.
const cmdFileEnvName = "MULTIVERSE_CMD_FILE"

// modQuitPollInterval matches DevCommands.PollSeconds. Polling faster than the
// mod does buys nothing. It is a variable so a test does not spend real seconds.
var modQuitPollInterval = 200 * time.Millisecond

// The wait is in two parts, and BOUNDED BY ATTEMPTS RATHER THAN BY A CLOCK,
// because the clock every command here reads is injected and the tests freeze it.
//
// The first part waits for the file to be TAKEN, and it is short: a loaded mod
// polls every 200 ms whether or not a world is loaded, so a file still sitting
// there after a second is a file nobody is reading — the ordinary case for a
// world started before this launcher wrote the variable, and the case that must
// not cost a person five seconds on every stop.
//
// The second part waits for the ANSWER, and it is longer, because it spans one
// pass of the mod's own command loop.
var (
	modQuitTakeAttempts   = 5
	modQuitAnswerAttempts = 20
)

// modQuitResult is what asking got. Every value except modQuitAccepted means the
// caller falls back to the operating system's own stop.
type modQuitResult int

const (
	// modQuitAccepted: the mod took the request and answered OK. The game is
	// saving and shutting down; the caller waits for it the ordinary way.
	modQuitAccepted modQuitResult = iota
	// modQuitNoConsumer: nothing took the file, so no mod is reading it.
	modQuitNoConsumer
	// modQuitNoAnswer: the file was taken and no answer came.
	modQuitNoAnswer
	// modQuitRefused: the mod answered ERROR.
	modQuitRefused
	// modQuitUnavailable: the request could not be written at all.
	modQuitUnavailable
)

// askModToQuit puts one quit request in this world's command file and waits for
// the mod's answer. It leaves NOTHING behind on any path it does not win on: a
// request still sitting in that file would be taken by the next start of this
// world, seconds after it came up, and shut it down again.
func askModToQuit(p Profile) (modQuitResult, string, error) {
	cmdFile, logFile := p.CommandFile(), p.CommandLogFile()
	token := modQuitToken()

	// Start from a clean slate on both halves. The answers of every past stop
	// are of no use to this one, and the file that carries them is the one file
	// here that would otherwise grow without bound.
	os.Remove(cmdFile)
	os.Remove(logFile)
	os.Remove(cmdFile + cmdTempSuffix)

	// RENAMED INTO PLACE, NEVER WRITTEN IN PLACE. The mod refuses content that
	// does not end in a newline, so a half-written file is skipped rather than
	// misread — but it is skipped on every poll until it is complete, and a
	// rename makes the file appear whole or not at all.
	temporary := cmdFile + cmdTempSuffix
	if err := os.WriteFile(temporary, []byte(token+" "+modQuitVerb+"\n"), 0o644); err != nil {
		return modQuitUnavailable, "", err
	}
	if err := os.Rename(temporary, cmdFile); err != nil {
		os.Remove(temporary)
		return modQuitUnavailable, "", err
	}

	taken := false
	for attempt := 1; attempt <= modQuitTakeAttempts; attempt++ {
		if !fileExists(cmdFile) {
			taken = true
			break
		}
		time.Sleep(modQuitPollInterval)
	}
	if !taken {
		// Take the request back before the next start of this world finds it.
		os.Remove(cmdFile)
		return modQuitNoConsumer, "", nil
	}
	for attempt := 1; ; attempt++ {
		if ok, details, found := readModAnswer(logFile, token); found {
			if ok {
				return modQuitAccepted, details, nil
			}
			return modQuitRefused, details, nil
		}
		if attempt >= modQuitAnswerAttempts {
			return modQuitNoAnswer, "", nil
		}
		time.Sleep(modQuitPollInterval)
	}
}

// modQuitToken names one request, so an answer is this stop's answer and not the
// leftover of another. It carries no whitespace, because the mod splits on it.
func modQuitToken() string {
	return fmt.Sprintf("stop-%d-%d", os.Getpid(), time.Now().UnixNano())
}

// readModAnswer looks for one request's answer in the mod's result log. The
// format is the mod's: '<token> OK|ERROR <details>', one appended line per
// finished command.
func readModAnswer(path, token string) (ok bool, details string, found bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, "", false
	}
	lines := strings.Split(string(raw), "\n")
	// ONLY COMPLETE LINES. The last element of a split is whatever follows the
	// final newline, which is either nothing or an answer still being appended;
	// reading it would hand back half a detail, or half a status.
	for _, line := range lines[:len(lines)-1] {
		fields := strings.Fields(strings.TrimRight(line, "\r"))
		if len(fields) < 2 || fields[0] != token {
			continue
		}
		return fields[1] == "OK", strings.Join(fields[2:], " "), true
	}
	return false, "", false
}

// askWorldToQuit is the app's half: it asks, and when the ask does not land it
// says which of the reasons it was, because they have different remedies.
func (a *app) askWorldToQuit(p Profile, events *eventLog) bool {
	result, details, err := askModToQuit(p)
	switch result {
	case modQuitAccepted:
		events.event("info", "game.quit-asked", "world", p.World, "answer", details)
		a.say("this world's mod took the quit request (%s); it is saving and shutting down.", details)
		return true
	case modQuitNoConsumer:
		a.say("nothing is reading %s, so this world's mod cannot be asked to quit: it was started "+
			"before this launcher set %s, or its mod is not loaded. Start this world again once "+
			"and the next stop is lossless. Asking the window instead.",
			p.CommandFile(), cmdFileEnvName)
	case modQuitNoAnswer:
		a.warn("this world's mod took the quit request and did not answer it. Asking the window "+
			"instead; look in %s.", p.BepInExLog())
	case modQuitRefused:
		a.warn("this world's mod refused the quit request (%s). Asking the window instead.", details)
	case modQuitUnavailable:
		a.warn("the quit request could not be written to %s (%v). Asking the window instead.",
			p.CommandFile(), err)
	}
	events.event("warn", "game.quit-ask-failed", "world", p.World, "why", modQuitWhy(result))
	return false
}

// modQuitWhy is the event log's one-word form of the same thing.
func modQuitWhy(result modQuitResult) string {
	switch result {
	case modQuitNoConsumer:
		return "no-consumer"
	case modQuitNoAnswer:
		return "no-answer"
	case modQuitRefused:
		return "refused"
	case modQuitUnavailable:
		return "unwritable"
	}
	return "accepted"
}
