package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiverse/internal/bb8"
)

// synthetic builds a save zip in the exact shape the game writes one: BACKSLASH
// entry names for the bibites directory, a BOM on scene.bb8scene, and no BOM on
// the organism blobs.
func synthetic(t *testing.T, dir, name string, organisms []string, simSize, simTime float64, living int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)

	write := func(entry string, body []byte) {
		w, err := zw.Create(entry)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}

	scene := fmt.Sprintf(`{"version":"0.6.3.1","nPellets":577,"nBibites":%d,"simulatedTime":%v}`, living, simTime)
	write("scene.bb8scene", append([]byte{0xEF, 0xBB, 0xBF}, scene...))
	settings := fmt.Sprintf(`{"independents":{"SimulationSize":{"Value":%v}}}`, simSize)
	write("settings.bb8settings", []byte(settings))
	for i, o := range organisms {
		write(fmt.Sprintf(`bibites\bibite_%d.bb8`, i), []byte(o))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

type bibiteSpec struct {
	id             int32
	x, y, vx, vy   float64
	age            float64
	dead           bool
	synapses       int
	hidden         int
	viewRadius     float64
	generation     int
	speciesID      int
	health, energy float64
	noArchetype    bool
}

func bibite(s bibiteSpec) string {
	genes := map[string]float64{
		"LayTime": 15, "BroodTime": 20, "HatchTime": 10, "SizeRatio": 1, "SpeedRatio": 1,
		"ColorR": 0.1, "ColorG": 0.1, "ColorB": 0.1,
		"MutationAmountSigma": 0.025, "AverageMutationNumber": 4,
		"BrainMutationSigma": 0.1, "BrainAverageMutation": 2,
		"ViewAngle": 180, "ViewRadius": s.viewRadius, "ClockSpeed": 1, "PheroSense": 160, "Diet": 0.3,
		"HerdSeparationWeight": 1, "HerdAlignmentWeight": 1.5, "HerdCohesionWeight": 1,
		"HerdVelocityWeight": 0.25, "HerdSeparationDistance": 35,
		"GrowthScale": 0.35, "GrowthMaturityFactor": 2, "GrowthMaturityExponent": 1, "EyeOffset": 0.5,
		"StomachWAG": 5, "WombWAG": 2.5, "FatWAG": 2.5, "ArmorWAG": 0.05,
		"ThroatWAG": 1.3, "MouthMusclesWAG": 1.07, "MoveMusclesWAG": 0.65,
		"FatStorageThreshold": 0.5, "FatStorageDeadband": 0.25,
	}

	// One input and one named output, then s.hidden mutated nodes. The game
	// serialises NEATBrain.NodeArchetype on every node — 0 Input, 1 Output,
	// 2 Hidden — and names a mutated node "HiddenN", exactly as below.
	nodes := []map[string]any{
		{"Type": 0, "TypeName": "Input", "Index": 0, "Inov": 1, "Desc": "EnergyRatio", "baseActivation": 0.0, "archetype": 0},
		{"Type": 3, "TypeName": "TanH", "Index": 1, "Inov": 2, "Desc": "Accelerate", "baseActivation": 0.0, "archetype": 1},
	}
	for i := 0; i < s.hidden; i++ {
		nodes = append(nodes, map[string]any{
			"Type": 5, "TypeName": "ReLu", "Index": 100 + i, "Inov": 100 + i,
			"Desc": fmt.Sprintf("Hidden%d", i), "baseActivation": 0.0, "archetype": 2,
		})
	}
	if s.noArchetype {
		// The fallback path: a blob with no per-node archetype at all. NEATBrain
		// derives nHidden as Nodes.Length - NInputs - NOutputs, so the fixture
		// pads to BaseNodeCount + s.hidden and drops the field.
		for len(nodes) < BaseNodeCount+s.hidden {
			nodes = append(nodes, map[string]any{
				"Type": 0, "TypeName": "Input", "Index": len(nodes), "Inov": len(nodes) + 1,
				"Desc": fmt.Sprintf("Pad%d", len(nodes)), "baseActivation": 0.0,
			})
		}
		for _, n := range nodes {
			delete(n, "archetype")
		}
	}
	syn := make([]map[string]any, 0, s.synapses)
	for i := 0; i < s.synapses; i++ {
		syn = append(syn, map[string]any{
			"Inov": i + 1, "NodeIn": 0, "NodeOut": 1 + i, "Weight": 1.5 + float64(i), "En": true,
		})
	}

	blob := map[string]any{
		"transform": map[string]any{"position": []float64{s.x, s.y}, "rotation": 0.0, "scale": 1.0},
		"rb2d":      map[string]any{"px": s.x, "py": s.y, "vx": s.vx, "vy": s.vy, "r": 0.0},
		"genes":     map[string]any{"speciesID": s.speciesID, "isReady": true, "gen": s.generation, "genes": genes},
		"body": map[string]any{
			"health": s.health, "energy": s.energy, "id": s.id,
			"d2Size": 1.5, "dying": false, "dead": s.dead,
		},
		"clock": map[string]any{"tic": 0, "ticProgress": 0.5, "timeAlive": s.age, "chronoTime": s.age},
		"brain": map[string]any{"isReady": true, "Nodes": nodes, "Synapses": syn},
	}
	out, err := json.Marshal(blob)
	if err != nil {
		panic(err)
	}
	return string(out)
}

func TestReadSyntheticSave(t *testing.T) {
	dir := t.TempDir()

	// Three living, one dead. Two of the living are genetically identical, the
	// third carries a different ViewRadius, so the projection must see two
	// distinct genomes across three living organisms.
	a := bibite(bibiteSpec{id: 1001, x: -1500, y: 10, vx: 1, vy: 0, age: 300, synapses: 3, viewRadius: 75, generation: 0, speciesID: 1, health: 100, energy: 50})
	b := bibite(bibiteSpec{id: 1002, x: 0, y: -20, vx: 2, vy: 1, age: 200, synapses: 3, viewRadius: 75, generation: 0, speciesID: 1, health: 120, energy: 60})
	c := bibite(bibiteSpec{id: 1003, x: 1975, y: 5, vx: 9, vy: 0, age: 100, synapses: 5, hidden: 1, viewRadius: 90, generation: 1, speciesID: 2, health: 80, energy: 40})
	d := bibite(bibiteSpec{id: 1004, x: 500, y: 0, age: 50, dead: true, synapses: 3, viewRadius: 75, generation: 1, speciesID: 1, health: 0, energy: 0})

	path := synthetic(t, dir, "T0.zip", []string{a, b, c, d}, 2000, 1234.5, 3)

	s, err := Read(path, Options{World: "Synthetic", Slot: 1})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if s.Schema != Schema {
		t.Errorf("schema = %q, want %q", s.Schema, Schema)
	}
	if s.Records != 4 {
		t.Errorf("records = %d, want 4", s.Records)
	}
	if s.Population != 3 {
		t.Errorf("population = %d, want 3 (the dead one must not count)", s.Population)
	}
	if s.Dead != 1 {
		t.Errorf("dead = %d, want 1", s.Dead)
	}
	if s.SceneBibites != 3 {
		t.Errorf("sceneBibites = %d, want 3", s.SceneBibites)
	}
	if s.SimulationSize != 2000 {
		t.Errorf("S = %v, want 2000 (read from settings.bb8settings)", s.SimulationSize)
	}
	if s.BorderWidth != 40 {
		t.Errorf("W = %v, want 40 = max(20, 0.02*S)", s.BorderWidth)
	}
	if s.SimulatedTime != 1234.5 {
		t.Errorf("simulatedTime = %v, want 1234.5 (BOM on scene.bb8scene must be stripped)", s.SimulatedTime)
	}
	if s.GameVersion != "0.6.3.1" {
		t.Errorf("gameVersion = %q, want 0.6.3.1", s.GameVersion)
	}

	// -- genomes: the two identical living organisms collapse, the third does not.
	if s.UniqueGenomes != 2 {
		t.Errorf("uniqueGenomes = %d, want 2", s.UniqueGenomes)
	}
	if len(s.TopGenomes) != 2 || s.TopGenomes[0].Count != 2 {
		t.Errorf("topGenomes = %+v, want the shared genome first with count 2", s.TopGenomes)
	}
	if len(s.GenomeErrors) != 0 {
		t.Errorf("genomeErrors = %v, want none", s.GenomeErrors)
	}
	// The hash must be the canonical projection, not something worldstat invented.
	want, err := bb8.GenomeHash(a, "0.6.3.1")
	if err != nil {
		t.Fatal(err)
	}
	if s.TopGenomes[0].Hash != want {
		t.Errorf("top genome hash = %q, want the bb8 canonical hash %q", s.TopGenomes[0].Hash, want)
	}
	if !strings.HasPrefix(want, bb8.HashPrefix) {
		t.Errorf("hash %q lost its projection label", want)
	}

	// -- species
	if len(s.Species) != 2 {
		t.Errorf("species = %+v, want two entries", s.Species)
	}
	if s.Species[0].SpeciesID != 1 || s.Species[0].Count != 2 {
		t.Errorf("species[0] = %+v, want speciesID 1 with 2 living", s.Species[0])
	}

	// -- genes: indices come from the BibiteGenes.Genes enum order, and only the
	//    living contribute.
	geneByName := map[string]GeneStat{}
	for _, g := range s.Genes {
		geneByName[g.Name] = g
	}
	if got := geneByName["ViewRadius"]; got.Index != 13 {
		t.Errorf("ViewRadius index = %d, want 13", got.Index)
	}
	vr := geneByName["ViewRadius"]
	if vr.N != 3 || vr.Min != 75 || vr.Max != 90 {
		t.Errorf("ViewRadius dist = %+v, want n=3 min=75 max=90 over the living only", vr.Dist)
	}
	if wantMean := (75.0 + 75 + 90) / 3; math.Abs(vr.Mean-wantMean) > 1e-9 {
		t.Errorf("ViewRadius mean = %v, want %v", vr.Mean, wantMean)
	}
	if lay := geneByName["LayTime"]; lay.StdDev != 0 {
		t.Errorf("LayTime stddev = %v, want 0 for an unmutated gene", lay.StdDev)
	}

	// -- brains
	if s.Brain.Synapses.Min != 3 || s.Brain.Synapses.Max != 5 {
		t.Errorf("synapse dist = %+v, want min 3 max 5", s.Brain.Synapses)
	}
	if wantMean := (3.0 + 3 + 5) / 3; math.Abs(s.Brain.Synapses.Mean-wantMean) > 1e-9 {
		t.Errorf("synapse mean = %v, want %v", s.Brain.Synapses.Mean, wantMean)
	}
	if s.Brain.HiddenNodes.Max != 1 || s.Brain.HiddenNodes.Min != 0 {
		t.Errorf("hidden node dist = %+v, want min 0 max 1", s.Brain.HiddenNodes)
	}
	if got := bucketLine(s.Brain.SynapseHistogram); got != "3:2 5:1" {
		t.Errorf("synapse histogram = %q, want \"3:2 5:1\"", got)
	}

	// -- spatial. S = 2000, so bins are 400 wide: -1500 lands in bin 1, 0 in bin
	//    5, 1975 in bin 9. The capture band starts at S-W = 1960.
	sp := s.Spatial
	if wantMean := (-1500.0 + 0 + 1975) / 3; math.Abs(sp.MeanX-wantMean) > 1e-9 {
		t.Errorf("meanX = %v, want %v", sp.MeanX, wantMean)
	}
	if got := histLine(sp.HistogramX); got != "[0 1 0 0 0 1 0 0 0 1]" {
		t.Errorf("x histogram = %s, want [0 1 0 0 0 1 0 0 0 1]", got)
	}
	if sp.WestEntryQuarter != 1 || sp.MiddleHalf != 1 || sp.EastExportQuarter != 1 {
		t.Errorf("quarters = %d/%d/%d, want 1/1/1", sp.WestEntryQuarter, sp.MiddleHalf, sp.EastExportQuarter)
	}
	if sp.CaptureBand != 1 {
		t.Errorf("captureBand = %d, want 1 (x=1975 >= S-W=1960)", sp.CaptureBand)
	}
	if sp.BeyondSquare != 0 {
		t.Errorf("beyondSquare = %d, want 0", sp.BeyondSquare)
	}
	if wantDist := (3500.0 + 2000 + 25) / 3; math.Abs(sp.MeanDistanceToExportEdge-wantDist) > 1e-9 {
		t.Errorf("meanDistanceToExportEdge = %v, want %v", sp.MeanDistanceToExportEdge, wantDist)
	}

	// -- the raw table keeps the dead row, flagged, so a diff can see it leave.
	if len(s.Organisms) != 4 {
		t.Fatalf("organisms = %d rows, want 4 (living and dead)", len(s.Organisms))
	}
	byID := map[int32]Organism{}
	for _, o := range s.Organisms {
		byID[o.ID] = o
	}
	if byID[1004].Alive {
		t.Errorf("organism 1004 should be marked dead")
	}
	if o := byID[1003]; o.X != 1975 || o.VX != 9 || o.Age != 100 || o.Synapses != 5 || o.Hidden != 1 {
		t.Errorf("organism 1003 = %+v, want x=1975 vx=9 age=100 syn=5 hidden=1", o)
	}
	if byID[1001].GenomeHash != byID[1002].GenomeHash {
		t.Errorf("identical organisms hashed differently")
	}
	if byID[1003].GenomeHash == byID[1001].GenomeHash {
		t.Errorf("a different ViewRadius must produce a different genome hash")
	}
	// maturity = (d2Size/SizeRatio^2) / max(1, (HatchTime/BroodTime)^2 / wombPortion)
	// = 1.5 / max(1, 0.25 / (2.5/13.07)) = 1.5 / 1.3069... in this fixture.
	if m := byID[1001].Maturity; m <= 0 || m > 3 {
		t.Errorf("maturity = %v, want a positive plausible value", m)
	}
}

func TestHiddenNodesFallBackToNEATBrainArithmetic(t *testing.T) {
	dir := t.TempDir()
	// No node in this save carries an archetype, so hidden must come out of
	// Nodes.Length - (NInputs + NOutputs) instead.
	two := bibite(bibiteSpec{id: 1, x: 0, age: 10, synapses: 3, hidden: 2, viewRadius: 75, speciesID: 1, noArchetype: true})
	none := bibite(bibiteSpec{id: 2, x: 0, age: 10, synapses: 3, hidden: 0, viewRadius: 75, speciesID: 1, noArchetype: true})
	path := synthetic(t, dir, "fallback.zip", []string{two, none}, 2000, 10, 2)

	s, err := Read(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := bucketLine(s.Brain.HiddenNodeHistogram); got != "0:1 2:1" {
		t.Errorf("hidden histogram = %q, want \"0:1 2:1\"", got)
	}
	if s.Brain.Nodes.Min != BaseNodeCount || s.Brain.Nodes.Max != BaseNodeCount+2 {
		t.Errorf("node dist = %+v, want min %d max %d", s.Brain.Nodes, BaseNodeCount, BaseNodeCount+2)
	}
}

func TestOutOfSquarePositionsAreCountedOutside(t *testing.T) {
	dir := t.TempDir()
	// The wrap radius is 1.5*S+1000, so a living organism legitimately sits
	// outside [-S,+S]. It must not be forced into an end bin.
	east := bibite(bibiteSpec{id: 1, x: 2400, y: -2500, age: 10, synapses: 3, viewRadius: 75, speciesID: 1})
	west := bibite(bibiteSpec{id: 2, x: -2100, y: 0, age: 10, synapses: 3, viewRadius: 75, speciesID: 1})
	path := synthetic(t, dir, "outside.zip", []string{east, west}, 2000, 10, 2)

	s, err := Read(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Spatial.AboveRange != 1 || s.Spatial.BelowRange != 1 {
		t.Errorf("above/below = %d/%d, want 1/1", s.Spatial.AboveRange, s.Spatial.BelowRange)
	}
	if got := histLine(s.Spatial.HistogramX); got != "[0 0 0 0 0 0 0 0 0 0]" {
		t.Errorf("x histogram = %s, want every bin empty", got)
	}
	if s.Spatial.BeyondSquare != 1 || s.Spatial.CaptureBand != 1 {
		t.Errorf("beyondSquare/captureBand = %d/%d, want 1/1", s.Spatial.BeyondSquare, s.Spatial.CaptureBand)
	}
	if s.Spatial.EastExportQuarter != 1 || s.Spatial.WestEntryQuarter != 1 {
		t.Errorf("quarters = %d/%d, want 1/1", s.Spatial.EastExportQuarter, s.Spatial.WestEntryQuarter)
	}
}

func TestCompareReportsTheDeltas(t *testing.T) {
	dir := t.TempDir()

	t0Path := synthetic(t, dir, "t0.zip", []string{
		bibite(bibiteSpec{id: 1, x: -1000, age: 100, synapses: 3, viewRadius: 75, speciesID: 1}),
		bibite(bibiteSpec{id: 2, x: -800, age: 100, synapses: 3, viewRadius: 75, speciesID: 1}),
	}, 2000, 1000, 2)
	t1Path := synthetic(t, dir, "t1.zip", []string{
		// id 1 survived and moved east; id 2 is gone; id 3 is new with a bigger
		// brain and a drifted ViewRadius.
		bibite(bibiteSpec{id: 1, x: 400, age: 5000, synapses: 3, viewRadius: 75, speciesID: 1}),
		bibite(bibiteSpec{id: 3, x: 1200, age: 200, synapses: 7, hidden: 2, viewRadius: 120, speciesID: 2, generation: 4}),
	}, 2000, 5000, 2)

	t0, err := Read(t0Path, Options{World: "W"})
	if err != nil {
		t.Fatal(err)
	}
	t1, err := Read(t1Path, Options{World: "W"})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := Compare(&buf, t0, t1); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"-- population",
		"-- genomes",
		"-- gene drift",
		"-- brain complexity",
		"-- geography",
		"-- individuals",
		"MOVED EAST",
		"ViewRadius",
		"present at both T0 and T1   1",
		"gone since T0 (died/left)   1",
		"new since T0 (born/arrived) 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("compare output is missing %q\n---\n%s", want, out)
		}
	}
	if !strings.Contains(out, "new genomes      1") {
		t.Errorf("compare should report exactly one new genome\n---\n%s", out)
	}
	if !strings.Contains(out, "extinct genomes  0") {
		t.Errorf("the shared T0 genome survives in id 1, so nothing is extinct\n---\n%s", out)
	}
}

func TestRoundTripThroughJSON(t *testing.T) {
	dir := t.TempDir()
	path := synthetic(t, dir, "rt.zip", []string{
		bibite(bibiteSpec{id: 7, x: 100, age: 42, synapses: 4, viewRadius: 75, speciesID: 1}),
	}, 2000, 99, 1)
	s, err := Read(path, Options{World: "RT", Slot: 3})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "rt.json")
	blob, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, blob, 0o644); err != nil {
		t.Fatal(err)
	}
	back, err := LoadStats(out)
	if err != nil {
		t.Fatal(err)
	}
	if back.Population != s.Population || back.Slot != 3 || back.World != "RT" {
		t.Errorf("round trip lost fields: %+v", back)
	}
	if len(back.Organisms) != 1 || back.Organisms[0].ID != 7 {
		t.Errorf("round trip lost the organism table: %+v", back.Organisms)
	}
	if err := Compare(&bytes.Buffer{}, back, s); err != nil {
		t.Errorf("compare of a round-tripped snapshot failed: %v", err)
	}
}
