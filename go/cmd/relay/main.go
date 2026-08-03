// Command multiverse-relay is the M3 transport: a deliberately dumb WebSocket
// server that forwards Contract B frames around a ring of slots, arbitrates
// ring insertion, and copies every routed migration to the archive. It never
// parses a bb8 body or a lineage annex. See contracts/contract-b-m3.md.
package main

import (
	"os"

	"multiverse/internal/relay"
)

func main() {
	os.Exit(relay.Main(os.Args[1:], os.Stderr))
}
