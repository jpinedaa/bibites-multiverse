//go:build windows

// Command multiverse-launcher-gui is the GRAPHICAL front door of a Bibites
// Multiverse installation. It ships as BibitesMultiverseLauncher.exe, which is
// the name the desktop and Start Menu shortcuts have always opened, so nothing
// about a shortcut, a registry value or an installed record changed when the
// window took that name.
//
// It is built for the Windows GUI subsystem (-ldflags -H=windowsgui), so a
// double-clicked icon opens a window and no console flashes up beside it.
//
// THE COMMANDS ARE A SEPARATE EXECUTABLE, multiverse-launcher.exe, and that is
// deliberate: PowerShell and cmd do not wait for a process in the GUI subsystem,
// so `& $launcher stop world; & $launcher status world` in one script would
// report on a world that was still shutting down. One file cannot be both
// without breaking every script anybody has already written, so there are two.
//
// A COMMAND LINE GIVEN TO THIS PROGRAM IS FORWARDED to that one and waited for,
// with its exit code passed back, because the name a script already knows is
// this one. It is compatibility rather than a promise: the documentation tells a
// script to call multiverse-launcher.exe, for the reason above.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"multiverse/internal/launchergui"
)

func main() {
	if args := os.Args[1:]; len(args) > 0 {
		os.Exit(forward(args))
	}
	if err := launchergui.Run(launchergui.Options{}); err != nil {
		// Run has already said so in the only place a window can say anything
		// before it exists: a message box.
		os.Exit(1)
	}
}

// forward hands the whole command line to the console launcher beside this
// executable and answers with its exit code.
//
// The three streams are passed straight through. A window is started with no
// console of its own, so they are whatever the caller redirected — which is the
// case that matters, because the only caller that passes arguments is a script.
func forward(args []string) int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "this program could not find its own location, so it cannot find "+
			"the console launcher beside it: %v\n", err)
		return 1
	}
	console := launchergui.ConsoleExePath(exe)
	if _, err := os.Stat(console); err != nil {
		fmt.Fprintf(os.Stderr, "%s is not beside this program, so a command line cannot be run. "+
			"Re-run the installer, and call %s directly from a script.\n",
			console, console)
		return 1
	}
	cmd := exec.Command(console, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "%s could not be run: %v\n", console, err)
		return 1
	}
	return 0
}
