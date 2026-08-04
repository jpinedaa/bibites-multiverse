// Command worldstat reads a Bibites world save .zip and emits a JSON statistics
// file describing the organisms in it, then compares two such files.
//
// It is the measurement instrument behind the overnight T0/T1 baseline: what a
// world's population, gene pool, brains and geography looked like at one instant,
// in a form two instants apart can be subtracted from each other.
//
//	worldstat stat   [flags] <save.zip>          # write one snapshot
//	worldstat compare        <t0.json> <t1.json> # print the delta
//
// It reads only. It never talks to a running game, a sidecar or the relay, and
// it never writes into the game's save directory: the caller copies the zip out
// first, and the copy is what gets read.
//
// The genome hash it reports comes from internal/bb8 — the same canonical
// projection (contracts/genome-hash.md, `bb8-genome/1`) the sidecar and the
// archive hash with — so a hash printed here and a hash in migrations.jsonl are
// the same 64 hex characters for the same organism.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"multiverse/internal/bb8"
)

// bb8HashHex is the one indirection into the bb8 package the printing side
// needs; it is kept here so compare.go stays free of imports it does not own.
func bb8HashHex(h string) string { return bb8.HashHex(h) }

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "stat":
		err = statCmd(os.Args[2:])
	case "compare":
		err = compareCmd(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		// `worldstat some.zip` is the same as `worldstat stat some.zip`.
		if strings.HasSuffix(strings.ToLower(os.Args[1]), ".zip") {
			err = statCmd(os.Args[1:])
		} else {
			usage()
			os.Exit(2)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "worldstat:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `worldstat — Bibites world save statistics

  worldstat stat [flags] <save.zip>
      --out <file>          write the JSON there (default: stdout)
      --world <name>        label the snapshot with a world name
      --slot <n>            label the snapshot with a ring slot
      --export-edge <E|W|N|S>  the edge this world exports through (default E)
      --game-version <v>    authoritative game version when the save carries none
      --top <n>             how many genome hashes to list (default 10)
      --summary             also print a human summary to stderr

  worldstat compare <t0.json> <t1.json>
      print the T0 -> T1 delta for one world

`)
}

func statCmd(args []string) error {
	fs := flag.NewFlagSet("stat", flag.ContinueOnError)
	out := fs.String("out", "", "write the JSON stats here (default stdout)")
	world := fs.String("world", "", "world name label")
	slot := fs.Int("slot", 0, "ring slot label")
	edge := fs.String("export-edge", "E", "the export edge of this world")
	version := fs.String("game-version", DefaultGameVersion, "authoritative game version when the save carries none")
	top := fs.Int("top", 10, "how many genome hashes to list")
	summary := fs.Bool("summary", false, "also print a human summary to stderr")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("expected exactly one save .zip, got %d", fs.NArg())
	}
	path := fs.Arg(0)

	name := *world
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	s, err := Read(path, Options{
		World:       name,
		Slot:        *slot,
		ExportEdge:  *edge,
		GameVersion: *version,
		TopN:        *top,
		CapturedAt:  time.Now().UTC(),
	})
	if err != nil {
		return err
	}

	blob, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	blob = append(blob, '\n')
	if *out == "" {
		os.Stdout.Write(blob)
	} else {
		if err := os.WriteFile(*out, blob, 0o644); err != nil {
			return err
		}
	}
	if *summary || *out != "" {
		Summarize(os.Stderr, s)
	}
	return nil
}

// Summarize is the one-screen headline of a single snapshot.
func Summarize(w *os.File, s *Stats) {
	p := func(format string, args ...any) { fmt.Fprintf(w, format+"\n", args...) }
	p("%s (slot %d)  %s", nonEmpty(s.World, "(world)"), s.Slot, s.Source)
	p("  population %d  (dead in save %d, eggs %d)  simTime %.1f s  S=%.0f W=%.0f",
		s.Population, s.Dead, s.Eggs, s.SimulatedTime, s.SimulationSize, s.BorderWidth)
	p("  unique genomes %d  species %d  generation mean %.2f max %.0f",
		s.UniqueGenomes, len(s.Species), s.Generation.Mean, s.Generation.Max)
	p("  brains: synapses mean %.2f (min %.0f max %.0f)  hidden nodes mean %.2f (max %.0f)",
		s.Brain.Synapses.Mean, s.Brain.Synapses.Min, s.Brain.Synapses.Max,
		s.Brain.HiddenNodes.Mean, s.Brain.HiddenNodes.Max)
	p("  age mean %.0f s  maturity mean %.2f  health mean %.1f  energy mean %.1f",
		s.Age.Mean, s.Maturity.Mean, s.Health.Mean, s.Energy.Mean)
	p("  spatial: mean (x,y) = (%+.1f, %+.1f)  stddev x %.1f  mean dist to %s edge %.1f",
		s.Spatial.MeanX, s.Spatial.MeanY, s.Spatial.StdDvX, s.ExportEdge, s.Spatial.MeanDistanceToExportEdge)
	p("  west quarter %d | middle half %d | east quarter %d | capture band %d | outside square E %d",
		s.Spatial.WestEntryQuarter, s.Spatial.MiddleHalf, s.Spatial.EastExportQuarter,
		s.Spatial.CaptureBand, s.Spatial.BeyondSquare)
	p("  x histogram %s (below %d, above %d)", histLine(s.Spatial.HistogramX), s.Spatial.BelowRange, s.Spatial.AboveRange)
	if len(s.TopGenomes) > 0 {
		top := s.TopGenomes[0]
		p("  top genome %s x%d", shortHash(top.Hash), top.Count)
	}
	for _, e := range s.GenomeErrors {
		p("  !! unhashable: %s", e)
	}
}

func compareCmd(args []string) error {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("expected <t0.json> <t1.json>, got %d arguments", fs.NArg())
	}
	t0, err := LoadStats(fs.Arg(0))
	if err != nil {
		return err
	}
	t1, err := LoadStats(fs.Arg(1))
	if err != nil {
		return err
	}
	return Compare(os.Stdout, t0, t1)
}

// LoadStats reads a stats file written by `worldstat stat`.
func LoadStats(path string) (*Stats, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Stats
	if err := json.Unmarshal(blob, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if s.Schema != Schema {
		return nil, fmt.Errorf("%s: schema %q, expected %q", path, s.Schema, Schema)
	}
	return &s, nil
}
