// Command multiverse-launcher is the installed entry point of a Bibites
// Multiverse world on this machine. It is shipped on Windows as
// BibitesMultiverseLauncher.exe: the Start Menu and desktop shortcuts open it,
// it owns the per-world profiles under <install root>\profiles, and it starts
// and stops the sidecar and the game itself. See docs/participant/install.md.
//
// The whole program lives in internal/launcher so the console menu, the
// commands and the enrollment client can be driven from tests with injected
// readers and writers rather than a terminal.
package main

import (
	"os"

	"multiverse/internal/launcher"
)

func main() {
	os.Exit(launcher.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
