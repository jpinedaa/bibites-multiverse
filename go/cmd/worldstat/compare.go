package main

// `worldstat compare <t0.json> <t1.json>` — the human half of the tool.
//
// It answers the six questions the overnight run is being watched for, in this
// order, because that is the order a reader wants them: did the population
// change, did the gene pool change, did the brains change, did the population
// move east, which genomes survived, and which individuals are still there.

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
)

// Compare writes the human-readable delta between two snapshots of one world.
func Compare(w io.Writer, t0, t1 *Stats) error {
	if t0.Schema != t1.Schema {
		return fmt.Errorf("schema mismatch: %q vs %q", t0.Schema, t1.Schema)
	}
	p := func(format string, args ...any) { fmt.Fprintf(w, format+"\n", args...) }

	name := t1.World
	if name == "" {
		name = t0.World
	}
	p("=== %s: %s  ->  %s", nonEmpty(name, "(world)"), t0.CapturedAt, t1.CapturedAt)
	simDelta := t1.SimulatedTime - t0.SimulatedTime
	p("    simulated time %.1f s -> %.1f s  (+%.1f s = %.2f sim hours)",
		t0.SimulatedTime, t1.SimulatedTime, simDelta, simDelta/3600)
	if t0.SimulationSize != t1.SimulationSize {
		p("    !! S changed %.1f -> %.1f — every spatial number below is measured against a different square",
			t0.SimulationSize, t1.SimulationSize)
	}

	// ---- population
	p("")
	p("-- population")
	p("    living           %d -> %d   (%+d)", t0.Population, t1.Population, t1.Population-t0.Population)
	p("    dead in save     %d -> %d", t0.Dead, t1.Dead)
	p("    eggs             %d -> %d", t0.Eggs, t1.Eggs)
	p("    generation mean  %.2f -> %.2f   (%+.2f), max %.0f -> %.0f",
		t0.Generation.Mean, t1.Generation.Mean, t1.Generation.Mean-t0.Generation.Mean,
		t0.Generation.Max, t1.Generation.Max)
	p("    age mean         %.1f s -> %.1f s   (%+.1f)", t0.Age.Mean, t1.Age.Mean, t1.Age.Mean-t0.Age.Mean)
	p("    maturity mean    %.3f -> %.3f   (%+.3f)", t0.Maturity.Mean, t1.Maturity.Mean, t1.Maturity.Mean-t0.Maturity.Mean)
	p("    health mean      %.1f -> %.1f   (%+.1f)", t0.Health.Mean, t1.Health.Mean, t1.Health.Mean-t0.Health.Mean)
	p("    energy mean      %.1f -> %.1f   (%+.1f)", t0.Energy.Mean, t1.Energy.Mean, t1.Energy.Mean-t0.Energy.Mean)

	// ---- genome turnover
	p("")
	p("-- genomes (canonical %s projection)", "bb8-genome/1")
	p("    unique genomes   %d -> %d   (%+d)", t0.UniqueGenomes, t1.UniqueGenomes, t1.UniqueGenomes-t0.UniqueGenomes)
	g0 := genomeSet(t0)
	g1 := genomeSet(t1)
	var newG, goneG, keptG []string
	for h := range g1 {
		if _, ok := g0[h]; ok {
			keptG = append(keptG, h)
		} else {
			newG = append(newG, h)
		}
	}
	for h := range g0 {
		if _, ok := g1[h]; !ok {
			goneG = append(goneG, h)
		}
	}
	sort.Strings(newG)
	sort.Strings(goneG)
	sort.Strings(keptG)
	p("    new genomes      %d", len(newG))
	p("    extinct genomes  %d", len(goneG))
	p("    carried over     %d", len(keptG))
	p("    per-genome survival (T0 genomes, count then -> count now):")
	surv := make([]string, 0, len(g0))
	for h := range g0 {
		surv = append(surv, h)
	}
	sort.Slice(surv, func(i, j int) bool {
		if g0[surv[i]] != g0[surv[j]] {
			return g0[surv[i]] > g0[surv[j]]
		}
		return surv[i] < surv[j]
	})
	for i, h := range surv {
		if i >= 12 {
			p("      ... and %d more T0 genomes", len(surv)-12)
			break
		}
		mark := "EXTINCT"
		if n, ok := g1[h]; ok {
			mark = fmt.Sprintf("%d", n)
		}
		p("      %s  %d -> %s", shortHash(h), g0[h], mark)
	}
	if len(newG) > 0 {
		p("    new genomes by size now:")
		sort.Slice(newG, func(i, j int) bool {
			if g1[newG[i]] != g1[newG[j]] {
				return g1[newG[i]] > g1[newG[j]]
			}
			return newG[i] < newG[j]
		})
		for i, h := range newG {
			if i >= 8 {
				p("      ... and %d more new genomes", len(newG)-8)
				break
			}
			p("      %s  %d", shortHash(h), g1[h])
		}
	}

	// ---- species
	p("")
	p("-- species (the game's own speciesID)")
	s0 := speciesMap(t0)
	s1 := speciesMap(t1)
	ids := unionInts(s0, s1)
	p("    distinct species %d -> %d", len(s0), len(s1))
	for _, id := range ids {
		p("      species %-6d %d -> %d", id, s0[id], s1[id])
	}

	// ---- gene drift
	p("")
	p("-- gene drift (largest mean shifts, by |delta| in units of the T0 stddev)")
	drifts := geneDrift(t0, t1)
	if len(drifts) == 0 {
		p("    no gene is present in both snapshots")
	}
	for i, d := range drifts {
		if i >= 12 {
			break
		}
		p("    %-24s %10.4f -> %10.4f   %+9.4f  (%+.2f sd)  sd %.4f -> %.4f",
			fmt.Sprintf("[%d] %s", d.Index, d.Name), d.Mean0, d.Mean1, d.Delta, d.Sigmas, d.SD0, d.SD1)
	}

	// ---- brains
	p("")
	p("-- brain complexity")
	p("    synapses     mean %.2f -> %.2f  (%+.2f)   max %.0f -> %.0f",
		t0.Brain.Synapses.Mean, t1.Brain.Synapses.Mean, t1.Brain.Synapses.Mean-t0.Brain.Synapses.Mean,
		t0.Brain.Synapses.Max, t1.Brain.Synapses.Max)
	p("    hidden nodes mean %.2f -> %.2f  (%+.2f)   max %.0f -> %.0f",
		t0.Brain.HiddenNodes.Mean, t1.Brain.HiddenNodes.Mean, t1.Brain.HiddenNodes.Mean-t0.Brain.HiddenNodes.Mean,
		t0.Brain.HiddenNodes.Max, t1.Brain.HiddenNodes.Max)
	p("    total nodes  mean %.2f -> %.2f  (%+.2f)   max %.0f -> %.0f",
		t0.Brain.Nodes.Mean, t1.Brain.Nodes.Mean, t1.Brain.Nodes.Mean-t0.Brain.Nodes.Mean,
		t0.Brain.Nodes.Max, t1.Brain.Nodes.Max)
	p("    synapse-count histogram   T0: %s", bucketLine(t0.Brain.SynapseHistogram))
	p("                              T1: %s", bucketLine(t1.Brain.SynapseHistogram))
	p("    hidden-node histogram     T0: %s", bucketLine(t0.Brain.HiddenNodeHistogram))
	p("                              T1: %s", bucketLine(t1.Brain.HiddenNodeHistogram))

	// ---- space
	p("")
	p("-- geography (export edge %s, S=%.0f, capture band x >= %.0f)",
		nonEmpty(t1.ExportEdge, "E"), t1.SimulationSize, t1.SimulationSize-t1.BorderWidth)
	dx := t1.Spatial.MeanX - t0.Spatial.MeanX
	p("    mean x                %+.1f -> %+.1f   (%+.1f)  %s", t0.Spatial.MeanX, t1.Spatial.MeanX, dx, eastWest(dx))
	p("    mean y                %+.1f -> %+.1f   (%+.1f)", t0.Spatial.MeanY, t1.Spatial.MeanY, t1.Spatial.MeanY-t0.Spatial.MeanY)
	p("    stddev x               %.1f -> %.1f   (%+.1f)  %s",
		t0.Spatial.StdDvX, t1.Spatial.StdDvX, t1.Spatial.StdDvX-t0.Spatial.StdDvX, spread(t1.Spatial.StdDvX-t0.Spatial.StdDvX))
	p("    mean distance to edge %.1f -> %.1f   (%+.1f)",
		t0.Spatial.MeanDistanceToExportEdge, t1.Spatial.MeanDistanceToExportEdge,
		t1.Spatial.MeanDistanceToExportEdge-t0.Spatial.MeanDistanceToExportEdge)
	p("    mean vx               %+.2f -> %+.2f", t0.Spatial.MeanEastwardVelocity, t1.Spatial.MeanEastwardVelocity)
	p("    west entry quarter    %d (%s) -> %d (%s)",
		t0.Spatial.WestEntryQuarter, pct(t0.Spatial.WestEntryQuarter, t0.Population),
		t1.Spatial.WestEntryQuarter, pct(t1.Spatial.WestEntryQuarter, t1.Population))
	p("    middle half           %d (%s) -> %d (%s)",
		t0.Spatial.MiddleHalf, pct(t0.Spatial.MiddleHalf, t0.Population),
		t1.Spatial.MiddleHalf, pct(t1.Spatial.MiddleHalf, t1.Population))
	p("    east export quarter   %d (%s) -> %d (%s)",
		t0.Spatial.EastExportQuarter, pct(t0.Spatial.EastExportQuarter, t0.Population),
		t1.Spatial.EastExportQuarter, pct(t1.Spatial.EastExportQuarter, t1.Population))
	p("    in capture band       %d -> %d", t0.Spatial.CaptureBand, t1.Spatial.CaptureBand)
	p("    outside square (E)    %d -> %d", t0.Spatial.BeyondSquare, t1.Spatial.BeyondSquare)
	p("    x histogram [-S..S]   T0: %s  (below %d, above %d)",
		histLine(t0.Spatial.HistogramX), t0.Spatial.BelowRange, t0.Spatial.AboveRange)
	p("                          T1: %s  (below %d, above %d)",
		histLine(t1.Spatial.HistogramX), t1.Spatial.BelowRange, t1.Spatial.AboveRange)

	// ---- individuals
	p("")
	p("-- individuals")
	o0 := organismMap(t0)
	o1 := organismMap(t1)
	var stayed, left, arrived []int32
	for id := range o1 {
		if _, ok := o0[id]; ok {
			stayed = append(stayed, id)
		} else {
			arrived = append(arrived, id)
		}
	}
	for id := range o0 {
		if _, ok := o1[id]; !ok {
			left = append(left, id)
		}
	}
	p("    present at both T0 and T1   %d", len(stayed))
	p("    gone since T0 (died/left)   %d", len(left))
	p("    new since T0 (born/arrived) %d", len(arrived))
	if len(stayed) > 0 {
		sort.Slice(stayed, func(i, j int) bool { return stayed[i] < stayed[j] })
		p("    survivors' own eastward movement:")
		shown := 0
		var sum float64
		for _, id := range stayed {
			sum += o1[id].X - o0[id].X
		}
		p("      mean dx across %d survivors: %+.1f", len(stayed), sum/float64(len(stayed)))
		for _, id := range stayed {
			if shown >= 10 {
				p("      ... and %d more survivors", len(stayed)-10)
				break
			}
			a, b := o0[id], o1[id]
			p("      %-12d x %+8.1f -> %+8.1f (%+7.1f)  age %.0f -> %.0f  syn %d -> %d",
				id, a.X, b.X, b.X-a.X, a.Age, b.Age, a.Synapses, b.Synapses)
			shown++
		}
	}
	return nil
}

type drift struct {
	Index              int
	Name               string
	Mean0, Mean1       float64
	SD0, SD1           float64
	Delta, Sigmas, Abs float64
}

func geneDrift(t0, t1 *Stats) []drift {
	byName := map[string]GeneStat{}
	for _, g := range t0.Genes {
		byName[g.Name] = g
	}
	var out []drift
	for _, g1 := range t1.Genes {
		g0, ok := byName[g1.Name]
		if !ok {
			continue
		}
		d := drift{
			Index: g1.Index, Name: g1.Name,
			Mean0: g0.Mean, Mean1: g1.Mean,
			SD0: g0.StdDev, SD1: g1.StdDev,
			Delta: g1.Mean - g0.Mean,
		}
		// A shift is only interesting relative to the spread it moved through.
		// A gene with no spread at T0 falls back to the absolute shift scaled by
		// its own magnitude, so a locked gene that suddenly moves still ranks.
		switch {
		case g0.StdDev > 1e-12:
			d.Sigmas = d.Delta / g0.StdDev
		case math.Abs(g0.Mean) > 1e-12:
			d.Sigmas = d.Delta / math.Abs(g0.Mean)
		default:
			d.Sigmas = d.Delta
		}
		d.Abs = math.Abs(d.Sigmas)
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Abs != out[j].Abs {
			return out[i].Abs > out[j].Abs
		}
		return out[i].Index < out[j].Index
	})
	return out
}

func genomeSet(s *Stats) map[string]int {
	m := map[string]int{}
	for _, o := range s.Organisms {
		if o.Alive && o.GenomeHash != "" {
			m[o.GenomeHash]++
		}
	}
	return m
}

func speciesMap(s *Stats) map[int]int {
	m := map[int]int{}
	for _, sp := range s.Species {
		m[sp.SpeciesID] = sp.Count
	}
	return m
}

func organismMap(s *Stats) map[int32]Organism {
	m := map[int32]Organism{}
	for _, o := range s.Organisms {
		if o.Alive {
			m[o.ID] = o
		}
	}
	return m
}

func unionInts(a, b map[int]int) []int {
	seen := map[int]bool{}
	var out []int
	for k := range a {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for k := range b {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Ints(out)
	return out
}

// shortHash prints a genome hash for a human. genome-hash.md §6 forbids a
// truncated hash in any FIELD, KEY or COMPARISON; this is a log line, which the
// same section explicitly allows.
func shortHash(h string) string {
	hex := bb8HashHex(h)
	if hex == "" {
		return h
	}
	return hex[:12] + "…"
}

func histLine(h []int) string {
	parts := make([]string, len(h))
	for i, v := range h {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func bucketLine(b []Bucket) string {
	parts := make([]string, 0, len(b))
	for _, x := range b {
		parts = append(parts, fmt.Sprintf("%d:%d", x.Value, x.Count))
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, " ")
}

func pct(n, total int) string {
	if total == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(n)/float64(total))
}

func eastWest(dx float64) string {
	switch {
	case dx > 25:
		return "<= MOVED EAST, towards the export edge"
	case dx < -25:
		return "<= moved west, away from the export edge"
	default:
		return "(no material shift)"
	}
}

func spread(d float64) string {
	switch {
	case d > 25:
		return "(more spread out)"
	case d < -25:
		return "(more clustered)"
	default:
		return ""
	}
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
