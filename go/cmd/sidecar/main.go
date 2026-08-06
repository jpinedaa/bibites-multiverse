// Command multiverse-sidecar serves Contract A to one game instance's mod,
// speaks Contract B to the relay, owns the durable migration journal that makes
// organism handoff at-most-once (D2), paces inbound delivery in simulated time,
// runs the bounded hold and the proof-based re-route, and assembles the lineage
// annex from the parent blobs the mod ships (D11). See contracts/contract-a.md
// and contracts/contract-b-m4.md.
package main

import (
	"os"

	"multiverse/internal/sidecar"
)

func main() {
	os.Exit(sidecar.Main(os.Args[1:], os.Stdout, os.Stderr))
}
