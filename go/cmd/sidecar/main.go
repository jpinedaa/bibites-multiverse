// Command multiverse-sidecar serves Contract A to one game instance's mod,
// speaks Contract B to the relay, and owns the durable migration journal that
// makes organism handoff at-most-once (D2). See contracts/contract-a.md and
// contracts/contract-b-m2.md.
package main

import (
	"os"

	"multiverse/internal/sidecar"
)

func main() {
	os.Exit(sidecar.Main(os.Args[1:], os.Stderr))
}
