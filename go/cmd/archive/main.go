// Command multiverse-archive is the map's read-only subscriber: it records
// every migration envelope and its lineage annex, fetches by hash any genome it
// has never seen, serves the live status page of D15, and appends periodic
// PEER_STATUS samples to a durable metrics file. It owns no world, holds no
// slot, and never appears in the structural order. See
// contracts/contract-b-m4.md §5.1, §10 and §10.1.
package main

import (
	"os"

	"multiverse/internal/archive"
)

func main() {
	os.Exit(archive.Main(os.Args[1:], os.Stdout, os.Stderr))
}
