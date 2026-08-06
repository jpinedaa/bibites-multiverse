// Command multiverse-relay is the M4 transport: a deliberately dumb WebSocket
// server that forwards Contract B frames across a rectangular map of slots,
// arbitrates placement, computes the effective neighbour on each export edge,
// proves what it did and did not forward, and copies every routed migration to
// the archive. It never parses a bb8 body or a lineage annex. See
// contracts/contract-b-m4.md.
package main

import (
	"os"

	"multiverse/internal/relay"
)

func main() {
	os.Exit(relay.Main(os.Args[1:], os.Stdout, os.Stderr))
}
