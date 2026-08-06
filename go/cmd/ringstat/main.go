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
	url := flag.String("url", env("MULTIVERSE_ARCHIVE_URL", "http://127.0.0.1:8791"),
		"the archive's status endpoint")
	metrics := flag.String("metrics", "",
		"read the newest sample from this metrics.jsonl instead of asking the archive")
	watch := flag.Duration("watch", 0, "repeat every interval, e.g. 5s")
	timeout := flag.Duration("timeout", 5*time.Second, "HTTP timeout")
	flag.Parse()

	for {
		var (
			s   archive.Status
			err error
		)
		if *metrics != "" {
			s, err = archive.LastSample(*metrics)
		} else {
			s, err = archive.FetchStatus(*url, *timeout)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "ringstat: %v\n", err)
			if *watch <= 0 {
				os.Exit(1)
			}
		} else {
			if *watch > 0 {
				fmt.Print("\033[H\033[2J")
			}
			archive.RenderRingstat(os.Stdout, s)
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
