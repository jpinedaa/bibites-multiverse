// Command ringstat prints the multiverse map as a terminal table: the grid with
// per-slot liveness and population, the effective lanes with their flow, every
// bypass with the time it went dark, the custody, paced and held depths, and
// each bounce a hold timeout caused.
//
// It renders EXACTLY the data the archive's status page renders, from the same
// place (contract-b-m4.md §10.1): the archive's own JSON endpoint, or its
// durable metrics file when the archive is not running. It never connects to a
// sidecar and never asks the relay for anything.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"multiverse/internal/archive"
)

func main() {
	// Tracks the archive's own --http default (8796), which moved out of the
	// six-slot Contract A range 8787-8792 for M4.
	url := flag.String("url", env("MULTIVERSE_ARCHIVE_URL", "http://127.0.0.1:8796"),
		"the archive's status endpoint")
	metrics := flag.String("metrics", "",
		"read the newest sample from this metrics.jsonl instead of asking the archive")
	watch := flag.Duration("watch", 0, "repeat every interval, e.g. 5s")
	timeout := flag.Duration("timeout", 5*time.Second, "HTTP timeout")
	// The page's three tabs, in the terminal, over the same data
	// (contract-b-m4.md §19, B19). The default is the map.
	species := flag.Bool("species", false,
		"print the species view: every species alive anywhere right now, joined to what the "+
			"crossing record knows about it. Against --metrics the crossing columns read n/a, "+
			"because that half is derived from the ledger and only a running archive holds it")
	settings := flag.Bool("settings", false,
		"print the settings view: what each world was TOLD to do — its mod and protocol "+
			"versions, its save policy, its wrap and the species it never exports. Read-only: "+
			"this tool changes nothing, anywhere")
	// DQ7's deny list (contract-b-m4.md §22, B30). An archive that has one
	// applies it to the JSON this tool reads, so the ordinary path needs nothing
	// here. It is a flag because --metrics reads the DURABLE SAMPLE FILE
	// directly, and that file holds what the archive saw: a terminal rendering
	// it is a rendering surface with the same obligation and no archive between
	// it and the text.
	denyFile := flag.String("deny-list", env("MULTIVERSE_ARCHIVE_DENY_LIST", ""),
		"file of species names and peer:<peerId> entries this tool refuses to render, one per "+
			"line. Only needed with --metrics: a running archive applies its own to the JSON "+
			"served here. It suppresses THE VIEW AND NOT THE RECORD")
	flag.Parse()

	deny, err := archive.NewDenyList(*denyFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ringstat: --deny-list: %v\n", err)
		os.Exit(1)
	}

	for {
		var s archive.Status
		if *metrics != "" {
			s, err = archive.LastSample(*metrics)
		} else {
			s, err = archive.FetchStatus(*url, *timeout)
		}
		s = deny.ApplyStatus(s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ringstat: %v\n", err)
			if *watch <= 0 {
				os.Exit(1)
			}
		} else {
			if *watch > 0 {
				fmt.Print("\033[H\033[2J")
			}
			switch {
			case *settings:
				archive.RenderSettings(os.Stdout, s)
			case *species:
				// The ledger half is fetched only when there is an archive to ask.
				// A failure is NOT fatal: the census half is still the whole
				// answer to "what is alive", and saying so beats printing nothing.
				var idx *archive.SpeciesIndex
				if *metrics == "" {
					if x, ferr := archive.FetchSpecies(*url, *timeout); ferr == nil {
						x = deny.ApplySpeciesIndex(x)
						idx = &x
					} else {
						fmt.Fprintf(os.Stderr,
							"ringstat: the crossing record is unavailable (%v); "+
								"showing what is alive without it\n", ferr)
					}
				}
				archive.RenderSpecies(os.Stdout, s, idx)
			default:
				archive.RenderRingstat(os.Stdout, s)
			}
		}
		if *watch <= 0 {
			return
		}
		time.Sleep(*watch)
	}
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
