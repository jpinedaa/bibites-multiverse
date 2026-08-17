//go:build !windows

// The graphical launcher is a Windows program: the Linux kit of this release
// ships no launcher at all, and walk is Win32. This file exists so that the
// whole module still builds, vets and tests on the machine this project is
// developed on — a command whose package has no files for the host platform is
// a build error for `go build ./...`, which would take the host's own checks
// down with it.
package main

import (
	"fmt"
	"os"

	"multiverse/internal/launcher"
)

func main() {
	fmt.Fprintf(os.Stderr, "the graphical Bibites Multiverse launcher is a Windows program. "+
		"On this platform the launcher's commands are %s.\n", launcher.ConsoleExeName)
	os.Exit(1)
}
