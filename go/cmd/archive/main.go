// Command multiverse-archive is the ring's read-only subscriber: it records
// every migration envelope and its lineage annex, fetches by hash any genome it
// has never seen, and lists what it recorded. It owns no world, holds no ring
// slot, and never appears in the ring order. See contracts/contract-b-m3.md
// §5.1 and §10.
package main

import (
	"os"

	"multiverse/internal/archive"
)

func main() {
	os.Exit(archive.Main(os.Args[1:], os.Stdout, os.Stderr))
}
