// Command multiverse-relay is the M2 transport: a deliberately dumb WebSocket
// server that forwards Contract B frames between two sidecars and arbitrates
// the two-sector map. See contracts/contract-b-m2.md.
package main

import (
	"os"

	"multiverse/internal/relay"
)

func main() {
	os.Exit(relay.Main(os.Args[1:], os.Stderr))
}
