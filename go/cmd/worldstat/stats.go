package main

// The reader half of worldstat: one Bibites world save .zip in, one Stats value
// out.
//
// Three things about the save format are load-bearing and were established by
// reading a real M3 slot save rather than the decompile:
//
//   - entry names use BACKSLASH separators (`bibites\bibite_0.bb8`), because the
//     game writes them on Windows. archive/zip hands them over verbatim, so
//     every name is normalised before it is matched.
//   - `scene.bb8scene` carries a UTF-8 BOM; the per-organism `.bb8` files do
//     not. The BOM is stripped from every entry before it is parsed.
//   - a save holds DEAD organisms as well as living ones. `scene.bb8scene`'s
//     `nBibites` counts only the living, and that is the number the mod's
//     `census` verb reports, so population here means the same thing:
//     `!dead && !dying`, matching FamilyScan.IsAlive.

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"multiverse/internal/bb8"
)

// Schema is the version tag every stats file carries. A comparison refuses two
// files that do not agree on it.
const Schema = "worldstat/1"

// DefaultGameVersion is the authoritative game version used when a save carries
// none. The saved organism dialect has no `$.version` key of its own, so the
// canonical genome projection (contracts/genome-hash.md §3) needs one supplied;
// `scene.bb8scene` normally provides it.
const DefaultGameVersion = "0.6.3.1"

// GeneOrder is BibiteGenes.Genes, in declaration order
// (decompiled/BibitesAssembly/SimulationScripts.BibiteScripts/BibiteGenes.cs:15).
// The float[] the game indexes with that enum is exactly this order, so the
// position in this slice IS the gene index the task asks statistics for. The
// gene object in a .bb8 is keyed by these names.
var GeneOrder = []string{
	"LayTime", "BroodTime", "HatchTime", "SizeRatio", "SpeedRatio",
	"ColorR", "ColorG", "ColorB",
	"MutationAmountSigma", "AverageMutationNumber", "BrainMutationSigma", "BrainAverageMutation",
	"ViewAngle", "ViewRadius", "ClockSpeed", "PheroSense", "Diet",
	"HerdSeparationWeight", "HerdAlignmentWeight", "HerdCohesionWeight", "HerdVelocityWeight",
	"HerdSeparationDistance",
	"GrowthScale", "GrowthMaturityFactor", "GrowthMaturityExponent", "EyeOffset",
	"StomachWAG", "WombWAG", "FatWAG", "ArmorWAG", "ThroatWAG", "MouthMusclesWAG", "MoveMusclesWAG",
	"FatStorageThreshold", "FatStorageDeadband",
}

// wagGenes are indices 26..32, the seven organ weight-at-growth genes that
// TotalOrganWAG sums (BibiteGenes.cs:222).
var wagGenes = []string{"StomachWAG", "WombWAG", "FatWAG", "ArmorWAG", "ThroatWAG", "MouthMusclesWAG", "MoveMusclesWAG"}

// SpatialBins is the number of histogram bins spanning [-S, +S].
const SpatialBins = 10

// ArchetypeHidden is NEATBrain.NodeArchetype.Hidden (NEATBrain.cs:157: Input,
// Output, Hidden). The saved node carries the game's own computed archetype, so
// a hidden node is read from the save rather than inferred.
const ArchetypeHidden = 2

// BaseNodeCount is NInputs + NOutputs for game 0.6.3.1 — 33 sensory inputs and
// 15 named outputs (NEATBrain.cs:244-246). It is only the FALLBACK: NEATBrain
// computes nHidden as Nodes.Length - NInputs - NOutputs (NEATBrain.cs:365), and
// that is what this reproduces when a blob carries no per-node archetype. A
// mutated hidden node is named "Hidden0", "Hidden1", … and appended past this
// index, so it is never confused with a stock node.
const BaseNodeCount = 48

// ---------------------------------------------------------------- the model

// Stats is one world's T0 (or T1) record. Every field is comparable against the
// same field of another Stats by `worldstat compare`.
type Stats struct {
	Schema      string `json:"schema"`
	World       string `json:"world"`
	Slot        int    `json:"slot,omitempty"`
	Source      string `json:"source"`
	SourceBytes int64  `json:"sourceBytes"`
	SourceSHA   string `json:"sourceSha256"`
	// SourceModified is the save file's mtime: the instant the game wrote it.
	SourceModified string `json:"sourceModified"`
	CapturedAt     string `json:"capturedAt"`

	GameVersion   string  `json:"gameVersion"`
	SimulatedTime float64 `json:"simulatedTime"`
	ScenePellets  int     `json:"scenePellets"`
	SceneBibites  int     `json:"sceneBibites"`

	SimulationSize float64 `json:"simulationSize"` // S, the playable half-extent
	BorderWidth    float64 `json:"borderWidth"`    // W = max(20, 0.02*S)
	ExportEdge     string  `json:"exportEdge"`     // E under the ring (D8)

	Population int `json:"population"` // living: !dead && !dying
	Dead       int `json:"dead"`
	Eggs       int `json:"eggs"`
	Records    int `json:"records"` // every .bb8 entry in the zip

	UniqueGenomes int           `json:"uniqueGenomes"`
	TopGenomes    []GenomeCount `json:"topGenomes"`
	GenomeErrors  []string      `json:"genomeErrors,omitempty"`

	Species    []SpeciesCount `json:"species"`
	Generation Dist           `json:"generation"`

	Genes []GeneStat `json:"genes"`

	Brain BrainStats `json:"brain"`

	Age      Dist `json:"age"`
	Maturity Dist `json:"maturity"`
	Health   Dist `json:"health"`
	Energy   Dist `json:"energy"`
	Speed    Dist `json:"speed"`

	Spatial SpatialStats `json:"spatial"`

	Organisms []Organism `json:"organisms"`
}

// GenomeCount is one canonical genome projection and how many living organisms
// carry it.
type GenomeCount struct {
	Hash  string `json:"hash"`
	Count int    `json:"count"`
}

// SpeciesCount is the game's own species bookkeeping (genes.speciesID).
type SpeciesCount struct {
	SpeciesID int `json:"speciesId"`
	Count     int `json:"count"`
}

// Dist is the four-number summary the task asks for, plus the count it was
// taken over. StdDev is the population standard deviation (divided by N): a
// save is a full census, not a sample.
type Dist struct {
	N      int     `json:"n"`
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"stddev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

// GeneStat is one gene's distribution across the living population. Index is
// the BibiteGenes.Genes ordinal; a gene the save carries that this table does
// not know gets an index past the end of GeneOrder and is still reported.
type GeneStat struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Dist
}

// BrainStats is the brain-complexity half: how big the networks are and how
// they are spread.
type BrainStats struct {
	Synapses    Dist `json:"synapses"`
	Nodes       Dist `json:"nodes"`
	HiddenNodes Dist `json:"hiddenNodes"`
	// SynapseHistogram and HiddenNodeHistogram are exact count -> organisms
	// tallies, sorted by count. Brains are small enough that an exact
	// distribution beats a binned one.
	SynapseHistogram    []Bucket `json:"synapseHistogram"`
	HiddenNodeHistogram []Bucket `json:"hiddenNodeHistogram"`
}

// Bucket is one exact-value tally.
type Bucket struct {
	Value int `json:"value"`
	Count int `json:"count"`
}

// SpatialStats answers the geographic-behaviour half: where the population sits
// along the export axis, and how close it is to the edge it leaves through.
type SpatialStats struct {
	MeanX  float64 `json:"meanX"`
	MeanY  float64 `json:"meanY"`
	StdDvX float64 `json:"stddevX"`
	MinX   float64 `json:"minX"`
	MaxX   float64 `json:"maxX"`

	// HistogramX has SpatialBins bins of width 2S/SpatialBins across [-S,+S].
	// BinEdges has SpatialBins+1 entries. Organisms outside the playable square
	// are counted in BelowRange / AboveRange instead — the wrap radius is
	// 1.5*S+1000, so a live organism legitimately sits outside [-S,+S].
	HistogramX []int     `json:"histogramX"`
	BinEdges   []float64 `json:"binEdges"`
	BelowRange int       `json:"belowRange"`
	AboveRange int       `json:"aboveRange"`

	// WestEntryQuarter is x in [-S, -S/2); EastExportQuarter is x in (S/2, S];
	// both count anything further out on that side too, so the two quarters plus
	// the middle half always sum to the population.
	WestEntryQuarter  int `json:"westEntryQuarter"`
	EastExportQuarter int `json:"eastExportQuarter"`
	MiddleHalf        int `json:"middleHalf"`

	// CaptureBand is x >= S-W: the half-open, outward-unbounded export trigger
	// zone of contract-a.md §4.3.1.
	CaptureBand int `json:"captureBand"`
	// BeyondSquare is x > S: already outside the playable square to the east.
	BeyondSquare int `json:"beyondSquare"`

	// MeanDistanceToExportEdge is mean(S - x). Positive means inside the square;
	// it falls as the population drifts east.
	MeanDistanceToExportEdge float64 `json:"meanDistanceToExportEdge"`
	// MeanEastwardVelocity is mean(vx). Positive means the population is, on
	// average, moving towards the export edge right now.
	MeanEastwardVelocity float64 `json:"meanEastwardVelocity"`
}

// Organism is one row of the raw per-organism table, for exact diffing.
type Organism struct {
	Entry      string  `json:"entry"`
	ID         int32   `json:"id"`
	Alive      bool    `json:"alive"`
	GenomeHash string  `json:"genomeHash,omitempty"`
	HashError  string  `json:"hashError,omitempty"`
	SpeciesID  int     `json:"speciesId"`
	Generation int     `json:"generation"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	VX         float64 `json:"vx"`
	VY         float64 `json:"vy"`
	Age        float64 `json:"age"`
	Maturity   float64 `json:"maturity"`
	Health     float64 `json:"health"`
	Energy     float64 `json:"energy"`
	Nodes      int     `json:"nodes"`
	Hidden     int     `json:"hiddenNodes"`
	Synapses   int     `json:"synapses"`
}

// ---------------------------------------------------------------- reading

type sceneFile struct {
	Version       string  `json:"version"`
	NPellets      int     `json:"nPellets"`
	NBibites      int     `json:"nBibites"`
	SimulatedTime float64 `json:"simulatedTime"`
}

// Options carries the labelling a save file cannot supply for itself.
type Options struct {
	World       string
	Slot        int
	ExportEdge  string
	GameVersion string
	TopN        int
	CapturedAt  time.Time
}

// Read parses one save zip into a Stats.
func Read(path string, opt Options) (*Stats, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	sum, err := fileSHA256(path)
	if err != nil {
		return nil, err
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer zr.Close()

	if opt.TopN <= 0 {
		opt.TopN = 10
	}
	if opt.CapturedAt.IsZero() {
		opt.CapturedAt = time.Now().UTC()
	}
	if opt.ExportEdge == "" {
		opt.ExportEdge = "E"
	}

	s := &Stats{
		Schema:         Schema,
		World:          opt.World,
		Slot:           opt.Slot,
		Source:         path,
		SourceBytes:    info.Size(),
		SourceSHA:      sum,
		SourceModified: info.ModTime().UTC().Format(time.RFC3339),
		CapturedAt:     opt.CapturedAt.UTC().Format(time.RFC3339),
		ExportEdge:     strings.ToUpper(opt.ExportEdge),
		GameVersion:    opt.GameVersion,
		SimulationSize: 2000,
	}

	byName := map[string]*zip.File{}
	var organismEntries []*zip.File
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, `\`, "/")
		byName[name] = f
		if strings.HasSuffix(strings.ToLower(name), ".bb8") {
			organismEntries = append(organismEntries, f)
		}
	}
	sort.Slice(organismEntries, func(i, j int) bool {
		return organismEntries[i].Name < organismEntries[j].Name
	})

	if f, ok := byName["scene.bb8scene"]; ok {
		raw, err := readEntry(f)
		if err != nil {
			return nil, err
		}
		var scene sceneFile
		if err := json.Unmarshal(raw, &scene); err != nil {
			return nil, fmt.Errorf("scene.bb8scene: %w", err)
		}
		s.SimulatedTime = scene.SimulatedTime
		s.ScenePellets = scene.NPellets
		s.SceneBibites = scene.NBibites
		if scene.Version != "" {
			s.GameVersion = scene.Version
		}
	}
	if s.GameVersion == "" {
		s.GameVersion = DefaultGameVersion
	}

	if f, ok := byName["settings.bb8settings"]; ok {
		raw, err := readEntry(f)
		if err != nil {
			return nil, err
		}
		if v, ok := simulationSize(raw); ok {
			s.SimulationSize = v
		}
	}
	// contract-a.md §4.3.1 / MultiverseConfig: W = max(20, 0.02*S) unless
	// MULTIVERSE_BORDER_WIDTH overrides it, and the rig leaves it at 0.
	s.BorderWidth = math.Max(20, 0.02*s.SimulationSize)

	acc := newAccumulator(s.SimulationSize, s.BorderWidth)
	for _, f := range organismEntries {
		raw, err := readEntry(f)
		if err != nil {
			return nil, err
		}
		s.Records++
		name := strings.ReplaceAll(f.Name, `\`, "/")
		if err := acc.add(name, raw, s.GameVersion); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	acc.finish(s, opt.TopN)
	return s, nil
}

func readEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.Name, err)
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.Name, err)
	}
	return stripBOM(raw), nil
}

func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// simulationSize pulls independents.SimulationSize.Value out of a
// settings.bb8settings blob. It is the live S the mod's BorderGeometry reads
// from ScenarioIndependentSettings.
func simulationSize(raw []byte) (float64, bool) {
	var root struct {
		Independents struct {
			SimulationSize struct {
				Value *float64 `json:"Value"`
			} `json:"SimulationSize"`
		} `json:"independents"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return 0, false
	}
	if v := root.Independents.SimulationSize.Value; v != nil && *v > 0 {
		return *v, true
	}
	return 0, false
}

// ---------------------------------------------------------------- parsing one organism

type savedBibite struct {
	Transform struct {
		Position []float64 `json:"position"`
	} `json:"transform"`
	RB2D struct {
		PX float64 `json:"px"`
		PY float64 `json:"py"`
		VX float64 `json:"vx"`
		VY float64 `json:"vy"`
	} `json:"rb2d"`
	Genes struct {
		SpeciesID int                `json:"speciesID"`
		Gen       int                `json:"gen"`
		Genes     map[string]float64 `json:"genes"`
	} `json:"genes"`
	Body *struct {
		ID     int32   `json:"id"`
		Health float64 `json:"health"`
		Energy float64 `json:"energy"`
		D2Size float64 `json:"d2Size"`
		Dying  bool    `json:"dying"`
		Dead   bool    `json:"dead"`
	} `json:"body"`
	Egg   json.RawMessage `json:"egg"`
	Clock struct {
		TimeAlive float64 `json:"timeAlive"`
	} `json:"clock"`
	Brain struct {
		Nodes []struct {
			Type      int    `json:"Type"`
			Index     int    `json:"Index"`
			Desc      string `json:"Desc"`
			Archetype *int   `json:"archetype"`
		} `json:"Nodes"`
		Synapses []json.RawMessage `json:"Synapses"`
	} `json:"brain"`
}

// maturity reproduces BibiteGrowth.maturity = growth / growthAtMature with
// growth = d2Size / SizeRatio^2 (BibiteBody.cs:996) and
// growthAtMature = max(1, (HatchTime/BroodTime)^2 / max(wombPortion, 0.001))
// (BibiteGenes.cs:167-169, 217-222). It returns 0 when the genes it needs are
// missing rather than guessing.
func maturityOf(genes map[string]float64, d2Size float64) float64 {
	sizeRatio := genes["SizeRatio"]
	brood := genes["BroodTime"]
	if sizeRatio <= 0 || brood <= 0 {
		return 0
	}
	growth := d2Size / (sizeRatio * sizeRatio)
	growthAtBirth := math.Pow(genes["HatchTime"]/brood, 2)
	total := 0.0
	for _, g := range wagGenes {
		total += math.Max(0, genes[g])
	}
	womb := math.Max(0, genes["WombWAG"]) / math.Max(total, 0.0001)
	growthAtMature := math.Max(1, growthAtBirth/math.Max(womb, 0.001))
	return growth / growthAtMature
}

// ---------------------------------------------------------------- accumulation

type accumulator struct {
	s, w float64

	genomeCounts map[string]int
	genomeErrs   map[string]int
	speciesCount map[int]int
	geneSeries   map[string][]float64
	geneSeen     []string

	generation, age, maturity, health, energy, speed []float64
	synapses, nodes, hidden                          []float64

	synHist, hidHist map[int]int

	xs, ys, vxs []float64

	organisms []Organism

	population, dead, eggs int
}

func newAccumulator(s, w float64) *accumulator {
	return &accumulator{
		s: s, w: w,
		genomeCounts: map[string]int{},
		genomeErrs:   map[string]int{},
		speciesCount: map[int]int{},
		geneSeries:   map[string][]float64{},
		synHist:      map[int]int{},
		hidHist:      map[int]int{},
	}
}

func (a *accumulator) add(entry string, raw []byte, gameVersion string) error {
	var b savedBibite
	if err := json.Unmarshal(raw, &b); err != nil {
		return fmt.Errorf("not a bb8 organism: %w", err)
	}
	if b.Body == nil {
		// A saved egg keys off `egg` rather than `body`, exactly as
		// SaveSystem.LoadBibiteOrEggFromData does. It has no live state to
		// measure, so it is counted and skipped.
		if len(b.Egg) > 0 {
			a.eggs++
			return nil
		}
		return fmt.Errorf("entry has neither a body nor an egg")
	}

	x, y := b.RB2D.PX, b.RB2D.PY
	if len(b.Transform.Position) == 2 && x == 0 && y == 0 {
		// rb2d holds the world position and transform holds localPosition; the
		// two agree while bibiteHolder is the identity (m2_findings.md §4.3
		// caveat 1). transform is the fallback, never the primary.
		x, y = b.Transform.Position[0], b.Transform.Position[1]
	}

	// Hidden nodes are the brain-growth half of brain complexity. The save
	// carries the game's own NodeArchetype on each node, so that is what is
	// counted; only a blob that omits it falls back to NEATBrain's arithmetic.
	hidden, archetyped := 0, false
	for _, n := range b.Brain.Nodes {
		if n.Archetype == nil {
			continue
		}
		archetyped = true
		if *n.Archetype == ArchetypeHidden {
			hidden++
		}
	}
	if !archetyped {
		hidden = len(b.Brain.Nodes) - BaseNodeCount
		if hidden < 0 {
			hidden = 0
		}
	}

	o := Organism{
		Entry:      entry,
		ID:         b.Body.ID,
		Alive:      !b.Body.Dead && !b.Body.Dying,
		SpeciesID:  b.Genes.SpeciesID,
		Generation: b.Genes.Gen,
		X:          x,
		Y:          y,
		VX:         b.RB2D.VX,
		VY:         b.RB2D.VY,
		Age:        b.Clock.TimeAlive,
		Maturity:   maturityOf(b.Genes.Genes, b.Body.D2Size),
		Health:     b.Body.Health,
		Energy:     b.Body.Energy,
		Nodes:      len(b.Brain.Nodes),
		Hidden:     hidden,
		Synapses:   len(b.Brain.Synapses),
	}

	// The canonical projection of contracts/genome-hash.md, reused verbatim from
	// the package the sidecar and the archive hash with — a worldstat hash and an
	// archive hash of the same organism are the same 64 hex characters.
	if h, err := bb8.GenomeHash(string(raw), gameVersion); err == nil {
		o.GenomeHash = h
	} else {
		o.HashError = err.Error()
	}
	a.organisms = append(a.organisms, o)

	if !o.Alive {
		a.dead++
		return nil
	}
	a.population++

	if o.GenomeHash != "" {
		a.genomeCounts[o.GenomeHash]++
	} else {
		a.genomeErrs[o.HashError]++
	}
	a.speciesCount[o.SpeciesID]++

	for name, v := range b.Genes.Genes {
		if _, seen := a.geneSeries[name]; !seen {
			a.geneSeen = append(a.geneSeen, name)
		}
		a.geneSeries[name] = append(a.geneSeries[name], v)
	}

	a.generation = append(a.generation, float64(o.Generation))
	a.age = append(a.age, o.Age)
	a.maturity = append(a.maturity, o.Maturity)
	a.health = append(a.health, o.Health)
	a.energy = append(a.energy, o.Energy)
	a.speed = append(a.speed, math.Hypot(o.VX, o.VY))
	a.synapses = append(a.synapses, float64(o.Synapses))
	a.nodes = append(a.nodes, float64(o.Nodes))
	a.hidden = append(a.hidden, float64(o.Hidden))
	a.synHist[o.Synapses]++
	a.hidHist[o.Hidden]++
	a.xs = append(a.xs, o.X)
	a.ys = append(a.ys, o.Y)
	a.vxs = append(a.vxs, o.VX)
	return nil
}

func (a *accumulator) finish(s *Stats, topN int) {
	s.Population = a.population
	s.Dead = a.dead
	s.Eggs = a.eggs

	s.UniqueGenomes = len(a.genomeCounts)
	s.TopGenomes = topGenomes(a.genomeCounts, topN)
	for msg, n := range a.genomeErrs {
		s.GenomeErrors = append(s.GenomeErrors, fmt.Sprintf("%d x %s", n, msg))
	}
	sort.Strings(s.GenomeErrors)

	for id, n := range a.speciesCount {
		s.Species = append(s.Species, SpeciesCount{SpeciesID: id, Count: n})
	}
	sort.Slice(s.Species, func(i, j int) bool {
		if s.Species[i].Count != s.Species[j].Count {
			return s.Species[i].Count > s.Species[j].Count
		}
		return s.Species[i].SpeciesID < s.Species[j].SpeciesID
	})

	s.Genes = geneStats(a.geneSeries, a.geneSeen)

	s.Generation = distOf(a.generation)
	s.Age = distOf(a.age)
	s.Maturity = distOf(a.maturity)
	s.Health = distOf(a.health)
	s.Energy = distOf(a.energy)
	s.Speed = distOf(a.speed)

	s.Brain = BrainStats{
		Synapses:            distOf(a.synapses),
		Nodes:               distOf(a.nodes),
		HiddenNodes:         distOf(a.hidden),
		SynapseHistogram:    buckets(a.synHist),
		HiddenNodeHistogram: buckets(a.hidHist),
	}

	s.Spatial = spatialOf(a.xs, a.ys, a.vxs, a.s, a.w)

	sort.Slice(a.organisms, func(i, j int) bool { return a.organisms[i].ID < a.organisms[j].ID })
	s.Organisms = a.organisms
	if s.Organisms == nil {
		s.Organisms = []Organism{}
	}
}

func topGenomes(counts map[string]int, n int) []GenomeCount {
	out := make([]GenomeCount, 0, len(counts))
	for h, c := range counts {
		out = append(out, GenomeCount{Hash: h, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Hash < out[j].Hash
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func geneStats(series map[string][]float64, seen []string) []GeneStat {
	index := map[string]int{}
	for i, name := range GeneOrder {
		index[name] = i
	}
	// A gene the save carries that BibiteGenes.Genes does not name is still
	// reported: it lands past the end of the table, in name order, so a game
	// update that adds a gene does not silently drop it.
	var extra []string
	for _, name := range seen {
		if _, ok := index[name]; !ok {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for i, name := range extra {
		index[name] = len(GeneOrder) + i
	}

	out := make([]GeneStat, 0, len(series))
	for name, values := range series {
		out = append(out, GeneStat{Index: index[name], Name: name, Dist: distOf(values)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

func distOf(values []float64) Dist {
	d := Dist{N: len(values)}
	if len(values) == 0 {
		return d
	}
	d.Min, d.Max = values[0], values[0]
	sum := 0.0
	for _, v := range values {
		sum += v
		if v < d.Min {
			d.Min = v
		}
		if v > d.Max {
			d.Max = v
		}
	}
	d.Mean = sum / float64(len(values))
	varsum := 0.0
	for _, v := range values {
		varsum += (v - d.Mean) * (v - d.Mean)
	}
	d.StdDev = math.Sqrt(varsum / float64(len(values)))
	return d
}

func buckets(hist map[int]int) []Bucket {
	out := make([]Bucket, 0, len(hist))
	for v, c := range hist {
		out = append(out, Bucket{Value: v, Count: c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

func spatialOf(xs, ys, vxs []float64, s, w float64) SpatialStats {
	sp := SpatialStats{
		HistogramX: make([]int, SpatialBins),
		BinEdges:   make([]float64, SpatialBins+1),
	}
	width := 2 * s / float64(SpatialBins)
	for i := 0; i <= SpatialBins; i++ {
		sp.BinEdges[i] = -s + float64(i)*width
	}
	if len(xs) == 0 {
		return sp
	}

	dx := distOf(xs)
	sp.MeanX, sp.StdDvX, sp.MinX, sp.MaxX = dx.Mean, dx.StdDev, dx.Min, dx.Max
	sp.MeanY = distOf(ys).Mean
	sp.MeanEastwardVelocity = distOf(vxs).Mean

	distSum := 0.0
	for _, x := range xs {
		distSum += s - x
		switch {
		case x < -s:
			sp.BelowRange++
		case x > s:
			sp.AboveRange++
		default:
			bin := int((x + s) / width)
			if bin >= SpatialBins {
				bin = SpatialBins - 1
			}
			sp.HistogramX[bin]++
		}
		switch {
		case x < -s/2:
			sp.WestEntryQuarter++
		case x > s/2:
			sp.EastExportQuarter++
		default:
			sp.MiddleHalf++
		}
		if x >= s-w {
			sp.CaptureBand++
		}
		if x > s {
			sp.BeyondSquare++
		}
	}
	sp.MeanDistanceToExportEdge = distSum / float64(len(xs))
	return sp
}
