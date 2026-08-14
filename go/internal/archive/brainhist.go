package archive

// BRAIN COMPLEXITY OVER TIME: the maintained aggregate behind the panel under
// the genealogy, and the PERSISTED per-species brain the tree's rings are drawn
// from. Both are folded at the same two write points, out of the same bytes, and
// both outlive the blob they were read from.
//
// WHAT THE PANEL DRAWS, AND WHY IT IS DEFENSIBLE. Every migration record names
// the migrant's genome hash; the archive fetches the blob behind it into the
// content-addressed store and can then count that brain's neurons and synapses
// (bb8.BrainStats). Over the running rig's 181.5 h the ledger holds 4.98 M
// migrations naming ~5.2 k distinct genomes an hour, of which the store holds
// 81.4 % of the distinct (bucket, hash) pairs — a median of 4 217 measurable
// genomes an hour. The graph is over THAT HELD SAMPLE and the page says so,
// because two properties make the sample honest and neither is an assumption:
//
//	MISSINGNESS IS SPECIES-BLIND. Measured across the whole record, the most
//	prolific species is over-represented among the held blobs by 1.2 points.
//	Nothing about which species a genome belongs to changes its chance of being
//	held.
//
//	MISSINGNESS IS CONTENT-BLIND BY CONSTRUCTION. pumpFetches walks the pending
//	set round-robin on a stable order keyed by HASH (§21, B21), so which gaps
//	get filled first is decided by a content address and by nothing about the
//	brain inside it. This is not a measurement; it is the shape of the queue.
//
// THE SAMPLING RULE IS EVERY HELD DISTINCT GENOME IN THE BUCKET, deduplicated by
// hash and NEVER weighted by crossings. A species that crosses two hundred times
// on one genome is one genome. The alternative — one genome per species per
// bucket — was measured and produces the same medians (within 2 synapses in 96 of
// 118 hourly buckets), is EMPTY for the first 19.3 h because no record carried a
// species block yet, and collapses to 4–8 samples in a dozen buckets. The rule
// that keeps the most evidence wins, and the count is published per bucket so a
// reader can see how much evidence each point rests on.
//
// FOUR RULES, and each is a decision this file records:
//
//  1. IT FOLDS AT GENOME ARRIVAL, NOT AT THE CROSSING. A bucket is only ~42 %
//     covered while it is new and reaches ~97 % about five days later: the fetch
//     backlog drains backwards into it. Folding at the crossing would therefore
//     measure the WORST-covered version of every bucket and freeze it. So the
//     fold happens where the blob is already in memory and the pending entry is
//     already in hand — onGenomeResponse — and the bucket key is that entry's own
//     `crossedAt`, the recordedAt of the migration that wanted the genome, which
//     is durable and was already there for the retention horizon (§23, B34). One
//     BrainStats on bytes the process is holding anyway: 470 µs, ~2.3/s at the
//     measured arrival rate, under 0.15 % of a core, and NO STORE READ AT ALL.
//     For a crossing whose hash the store already holds there is no arrival to
//     wait for, so onMigration folds it through the per-hash parse cache.
//
//  2. THERE IS NO FULL-HISTORY SWEEP, AND THERE MUST NOT BE ONE. bb8.Store.Get
//     refreshes a blob's mtime and eviction is ordered by mtime (bb8/evict.go),
//     so reading all 702 159 blobs to backfill would POSTPONE THE 720 h RETENTION
//     HORIZON FOR THE WHOLE STORE — a sweep that quietly turns a bounded store
//     into an unbounded one. It also holds the store's mutex for about eleven
//     minutes, which is the fetch pump's and the genome-serving path's lock. The
//     horizon is also why a sweep would not even be a one-off fix: an old bucket
//     becomes permanently unrecomputable once its blobs age out, while a new one
//     reaches ~97 % coverage in about five days — far inside the horizon. Capture
//     at arrival loses essentially nothing and costs nothing.
//
//  3. IT IS PERSISTED AND REPLAYED, NEVER RECOMPUTED AT REPLAY. The startup
//     replay is 28 s for 10.62 M records; re-deriving brains there would add
//     about eleven minutes and would grow with the store forever. The sidecar
//     (brainsave.go) is replayed instead, and ITS LOSS IS "HISTORY STARTS NOW"
//     AND NEVER ZEROES: a bucket this archive holds no measurement for publishes
//     null, is drawn as a gap, and says how many genomes the record says crossed
//     in it that it measured none of.
//
//  4. THE SERVED SERIES IS RE-AGGREGATED PER REQUEST. The panel shares the
//     genealogy's axis, and that axis RE-FITS to the drawn set (tree.go,
//     SpanStartMs/SpanStartSeedMs) — so a fixed-resolution series cannot share
//     it. The aggregate is kept at a fixed fine resolution and downsampled onto
//     whatever window is asked for, and the downsampling MERGES HISTOGRAMS rather
//     than averaging medians, which is the whole reason a bucket keeps a
//     histogram: the median over a re-aggregated window is the true median of
//     every held genome in it, not a mean of five-minute medians.

import (
	"sort"
	"strconv"
	"sync"
	"time"

	"multiverse/internal/bb8"
)

const (
	// BrainBucketMs is the aggregate's own resolution. Five minutes is ~54 MB a
	// year of sidecar at the measured shape below; an hour is 4.5 MB but is far
	// too coarse for an axis that re-fits — a 12-hour window would be twelve
	// points.
	BrainBucketMs int64 = 5 * 60 * 1000

	// BrainNeuronFloor is the FIXED SENSOR AND ACTUATOR SET every bibite brain
	// carries. It is not a modelling assumption: 48 is the global minimum neuron
	// count across all 41 165 genomes parsed out of the running rig's store, and
	// no genome in the corpus has fewer. It matters because the raw neuron count
	// therefore UNDERSTATES the change by about sevenfold — median neurons went
	// 49.0 → 56.6 over the record (×1.15) while median HIDDEN neurons went
	// 1.0 → 8.6 (×8.6) — so the panel draws the hidden count and the page states
	// the floor rather than letting a reader read growth off a number that is
	// mostly constant.
	BrainNeuronFloor = 48

	// brainHistKeysMax bounds the DISTINCT VALUES one bucket's histogram holds.
	// The number of distinct neuron or synapse counts inside five minutes is
	// bounded by the genomes in it — ~350 at the measured arrival rate, and 20 to
	// 60 in practice — so this is several times the observed shape. Past it a new
	// value is folded into the nearest key the histogram already has and the
	// bucket is marked binned, because a bounded answer that says it is bounded
	// beats an unbounded one.
	brainHistKeysMax = 192
	// brainValueMax clamps a single reading. A genome claiming more than this is
	// counted at this, and the bucket is marked binned.
	brainValueMax = 4095

	// brainBucketMax bounds the retained aggregate: 365 days of five-minute
	// buckets. Past it the OLDEST bucket is dropped, which is the only direction
	// that can be dropped without lying about the newest.
	brainBucketMax = 365 * 24 * 12

	// brainRecordMax bounds the per-species brain records. It matches
	// speciesAggMax, because the record set is keyed the same way the species
	// aggregate is and cannot usefully outgrow it. Past it the record with the
	// OLDEST measurement is dropped and the count is published.
	brainRecordMax = speciesAggMax

	// brainDedupBuckets is how many buckets keep a (bucket, hash) set so one
	// genome is counted once per bucket however often it crosses. It only needs
	// to cover the WRITE FRONTIER: `seen` is folded at the crossing, which is
	// always now or the replay cursor, and the only held fold that can repeat is
	// the already-held one at onMigration, which is also always at the frontier.
	// An arrival fold reaches back days into the backlog and needs no dedup at
	// all — it happens exactly once per hash, because the pending entry is
	// deleted with it.
	brainDedupBuckets = 24
)

// The window and resolution a /api/species/brains request may ask for.
const (
	BrainMinBuckets = 8
	BrainMaxBuckets = 720
	// BrainMaxWindow bounds one request's span. It is the retained aggregate, so
	// asking for more can never be answered anyway.
	BrainMaxWindow = time.Duration(brainBucketMax) * time.Duration(BrainBucketMs) * time.Millisecond
)

// brainHist is a sparse histogram of small non-negative counts: value → how many
// genomes carried it. Sparse because the values cluster — the observed corpus
// puts every neuron count between 48 and about 70 — and a dense array over the
// clamp range would be 16 KB a bucket for 60 live entries.
type brainHist map[uint16]uint32

// add folds one reading in, honouring both bounds. It returns whether the
// reading had to be moved to fit, which the bucket publishes.
func (h brainHist) add(v int) bool {
	binned := false
	if v < 0 {
		v, binned = 0, true
	}
	if v > brainValueMax {
		v, binned = brainValueMax, true
	}
	k := uint16(v)
	if _, ok := h[k]; !ok && len(h) >= brainHistKeysMax {
		// FULL. Fold into the nearest key this histogram already holds rather
		// than dropping the reading: a dropped reading changes n, which is the
		// coverage number, and a slightly misplaced one changes a quantile by at
		// most the gap it was moved across.
		best, bestD := k, 1<<30
		for e := range h {
			d := int(e) - int(k)
			if d < 0 {
				d = -d
			}
			if d < bestD || (d == bestD && e < best) {
				best, bestD = e, d
			}
		}
		k, binned = best, true
	}
	h[k]++
	return binned
}

// brainBucket is one five-minute slice of the record.
type brainBucket struct {
	// held is how many DISTINCT held genomes were measured into this bucket, and
	// it is the n the page publishes. seen is how many distinct genomes the
	// RECORD says crossed in it, held or not — the coverage denominator. held is
	// a subset of seen by construction: every held fold's (bucket, hash) came
	// from a crossing that folded seen first.
	held int
	seen int
	// The two histograms. Synapses is drawn directly; neurons is drawn as
	// max(0, v - BrainNeuronFloor), which is a monotone transform, so a quantile
	// of the transform is the transform of the quantile and nothing is lost by
	// storing the raw count.
	neurons  brainHist
	synapses brainHist
	// binned says a reading in this bucket was clamped or moved to fit a bound.
	binned bool
}

// newBrainBucket allocates NO HISTOGRAMS. Most buckets on a running archive are
// seen-only for a while — the crossing is recorded the moment it happens and the
// genome arrives over the following days — and over a year of five-minute
// buckets two empty maps apiece is megabytes spent on nothing. They are created
// by the first measurement that needs them.
//
// THE RESIDENT COST, named rather than left to be discovered. A bucket with no
// measurement is a few tens of bytes; a measured one is bounded by DISTINCT
// VALUES rather than by how many genomes fell in it — measured on the running
// rig's corpus, about 111 bytes serialized and well under brainHistKeysMax keys
// — so the whole retained year is tens of megabytes and does not grow with the
// arrival rate.
func newBrainBucket() *brainBucket { return &brainBucket{} }

// brainRecord is the LATEST MEASURED BRAIN OF ONE SPECIES, persisted.
//
// It is what the genealogy's ring is drawn from now, and it is a different claim
// from the one the ring used to make. The old rule was a per-process cache over
// blobs the store still holds — so a restart emptied the picture (2 of 14 drawn
// rows carried a ring twelve minutes after a restart, 5 of 13 after hours of
// uptime), and an EXTINCT ancestor could never gain a ring at all: nothing is
// fetching its genomes any more, and once its blobs pass the retention horizon
// the measurement is permanently unobtainable. The interior nodes of the tree —
// the branch points, which are the reason the tree has interior nodes — were
// structurally ringless.
//
// A MEASUREMENT SURVIVES ITS BLOB. That is the point, and it is the same
// argument as capturing at arrival: the horizon prunes the genome and does not
// prune the number. It also changes what the ring MEANS, and the page's words
// are changed with it — from "the newest genome of it this archive HOLDS A COPY
// OF" to "the newest genome of it this archive EVER READ", which for a
// long-extinct species may be days old and may describe a genome that no longer
// exists anywhere. AtMs is published so the row can say how old.
//
// ABSENCE IS STILL ABSENCE. A species this archive has never once read a genome
// for has no record and draws no ring.
type brainRecord struct {
	Neurons  int
	Synapses int
	// AtMs is the CROSSING the measured genome was recorded on, not the moment
	// the archive got round to reading it. Latest writer wins on this clock, so a
	// blob that arrives out of order can never overwrite a newer measurement.
	AtMs int64
	// Hash is the genome the numbers were read out of, kept so the record can be
	// audited against the store while the blob still exists.
	Hash string
}

// brainAgg is the whole maintained aggregate. IT HAS ITS OWN LOCK and is
// deliberately not under Archive.mu: a view copies a window of histograms out of
// it, and nothing that walks thousands of map entries may hold the lock the
// migration path takes (Risk 4). The fold sites take this lock briefly and
// never while holding Archive.mu.
type brainAgg struct {
	mu      sync.Mutex
	buckets map[int64]*brainBucket
	species map[string]brainRecord
	// dedup is the (bucket, hash) set for the last brainDedupBuckets buckets:
	// bit 1 seen, bit 2 held. frontier is the newest bucket any fold has touched.
	dedup    map[int64]map[uint64]uint8
	frontier int64
	// The published bounds and losses.
	bucketsDropped  int
	speciesOverflow int
	// dirty is what has changed since the last save, so a save appends what moved
	// rather than rewriting the whole file every time (brainsave.go).
	dirtyBuckets map[int64]bool
	dirtySpecies map[string]bool
	// loaded says a sidecar was replayed at startup, and lost says one existed
	// and could not be used. Both are published: an aggregate that starts empty
	// because there was no file is a new archive, and one that starts empty
	// because a file was unreadable is a loss, and a reader deserves to know
	// which.
	loaded bool
	lost   bool
}

func newBrainAgg() *brainAgg {
	return &brainAgg{
		buckets:      map[int64]*brainBucket{},
		species:      map[string]brainRecord{},
		dedup:        map[int64]map[uint64]uint8{},
		dirtyBuckets: map[int64]bool{},
		dirtySpecies: map[string]bool{},
	}
}

// brainBucketStart is the bucket key one millisecond falls in.
func brainBucketStart(ms int64) int64 {
	if ms <= 0 {
		return 0
	}
	return ms - ms%BrainBucketMs
}

// mark records a (bucket, hash) in the frontier dedup window and says whether
// this fold is the first of its kind there. Buckets older than the window are
// NOT deduplicated and always report first: the only fold that reaches them is
// the arrival fold, which is once-per-hash by construction.
func (g *brainAgg) mark(bucket int64, hash string, bit uint8) bool {
	if bucket > g.frontier {
		g.frontier = bucket
		// Drop the sets that have fallen out of the window. This is the only
		// place the dedup map shrinks, and it is what keeps it O(window) rather
		// than O(record).
		cut := g.frontier - int64(brainDedupBuckets)*BrainBucketMs
		for k := range g.dedup {
			if k < cut {
				delete(g.dedup, k)
			}
		}
	}
	if bucket < g.frontier-int64(brainDedupBuckets)*BrainBucketMs {
		return true
	}
	set := g.dedup[bucket]
	if set == nil {
		set = map[uint64]uint8{}
		g.dedup[bucket] = set
	}
	fp := fingerprint(hash)
	if set[fp]&bit != 0 {
		return false
	}
	set[fp] |= bit
	return true
}

// bucketFor returns the bucket for a key, creating it and evicting the oldest
// when the retained set is full.
func (g *brainAgg) bucketFor(key int64) *brainBucket {
	if b := g.buckets[key]; b != nil {
		return b
	}
	if len(g.buckets) >= brainBucketMax {
		oldest := int64(0)
		for k := range g.buckets {
			if oldest == 0 || k < oldest {
				oldest = k
			}
		}
		if oldest >= key {
			// The set is full and this key is older than everything in it. Refuse
			// rather than evict something newer for it.
			g.bucketsDropped++
			return nil
		}
		delete(g.buckets, oldest)
		delete(g.dirtyBuckets, oldest)
		g.bucketsDropped++
	}
	b := newBrainBucket()
	g.buckets[key] = b
	return b
}

// observeBrainSeen folds ONE crossing's own genome hash into the coverage
// denominator of the bucket its crossing falls in — held or not, fetched or not.
// It is the only thing here the ledger replay rebuilds, and it is what lets a
// bucket say "312 genomes crossed in this five minutes and this archive measured
// none of them" rather than drawing an unexplained gap.
func (a *Archive) observeBrainSeen(atMs int64, hash string) {
	if hash == "" || atMs <= 0 {
		return
	}
	key := brainBucketStart(atMs)
	g := a.brainAgg
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.mark(key, hash, 1) {
		return
	}
	b := g.bucketFor(key)
	if b == nil {
		return
	}
	b.seen++
	// IT IS NOT MARKED DIRTY, BECAUSE seen IS NEVER PERSISTED. It is derived from
	// the ledger, the ledger is replayed at every start anyway, and re-deriving it
	// is a map lookup per record with no disk read at all. Writing it to the
	// sidecar as well would put one fact in two places and give a restart a way to
	// disagree with the record about how many genomes crossed.
}

// observeBrainHeld folds ONE MEASURED GENOME into the bucket of the crossing
// that wanted it, and into that species' persisted record. It is called from
// exactly two places and that is the point: the arrival path
// (onGenomeResponse) and the already-held path (onMigration). One function, two
// call sites, no third view of the same fact — species.go rule 1 applied to a
// second aggregate.
//
// The species key is the MIGRANT'S OWN, and only ever the migrant's. A parent
// hash in the lineage annex is the genome of the migrant's mother or father —
// an individual, whose species this archive is not told — and the species
// block's parentGenericName names a TAXONOMIC ancestor, which is a different
// thing entirely. Attributing one to the other would put a measurement on a
// species no record supports, so a parent genome is measured for nothing and
// counted in nothing.
func (a *Archive) observeBrainHeld(crossedAtMs int64, speciesKey, hash string, br bb8.Brain) {
	if hash == "" || crossedAtMs <= 0 {
		return
	}
	key := brainBucketStart(crossedAtMs)
	g := a.brainAgg
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.mark(key, hash, 2) {
		if b := g.bucketFor(key); b != nil {
			b.held++
			if b.neurons == nil {
				b.neurons, b.synapses = brainHist{}, brainHist{}
			}
			if b.neurons.add(br.Neurons) {
				b.binned = true
			}
			if b.synapses.add(br.Synapses) {
				b.binned = true
			}
			g.dirtyBuckets[key] = true
		}
	}
	g.recordSpeciesLocked(speciesKey, crossedAtMs, hash, br)
}

// observeSpeciesBrain records a measurement against a species without touching
// the time series. It is the render path's contribution: the genealogy parses a
// species' latest named hash when the store holds it, and that reading belongs in
// the persisted record even though the arrival that would have folded it happened
// before this feature existed.
func (a *Archive) observeSpeciesBrain(speciesKey string, atMs int64, hash string, br bb8.Brain) {
	if speciesKey == "" || hash == "" {
		return
	}
	g := a.brainAgg
	g.mu.Lock()
	defer g.mu.Unlock()
	g.recordSpeciesLocked(speciesKey, atMs, hash, br)
}

// recordSpeciesLocked is LATEST WRITER WINS ON THE CROSSING'S CLOCK, which is
// what makes an out-of-order arrival safe: a blob fetched today for a crossing
// three days old can never displace a measurement of a newer genome.
func (g *brainAgg) recordSpeciesLocked(key string, atMs int64, hash string, br bb8.Brain) {
	if key == "" || hash == "" || atMs <= 0 {
		return
	}
	cur, ok := g.species[key]
	if ok && cur.AtMs >= atMs {
		return
	}
	if !ok && len(g.species) >= brainRecordMax {
		// Full. Drop the record with the OLDEST measurement — the least likely to
		// be drawn, since the tree keeps living species and the ancestors living
		// lines part at — and publish that it happened.
		oldestKey, oldestAt := "", int64(0)
		for k, v := range g.species {
			if oldestAt == 0 || v.AtMs < oldestAt {
				oldestKey, oldestAt = k, v.AtMs
			}
		}
		if oldestAt >= atMs {
			g.speciesOverflow++
			return
		}
		delete(g.species, oldestKey)
		delete(g.dirtySpecies, oldestKey)
		g.speciesOverflow++
	}
	g.species[key] = brainRecord{Neurons: br.Neurons, Synapses: br.Synapses,
		AtMs: atMs, Hash: hash}
	g.dirtySpecies[key] = true
}

// record reads one species' persisted measurement.
func (g *brainAgg) record(key string) (brainRecord, bool) {
	if key == "" {
		return brainRecord{}, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	r, ok := g.species[key]
	return r, ok
}

// ------------------------------------------------------------- the served view

// BrainBucket is one source bucket copied out of the aggregate, which is what
// makes the re-aggregation below PURE: every input is an argument, so the
// downsampling is testable without a rig, a store or a file — exactly as
// BuildHistory is.
type BrainBucket struct {
	AtMs int64 `json:"tMs"`
	Held int   `json:"n"`
	Seen int   `json:"seen"`
	// The histograms, value → count.
	Neurons  map[int]int `json:"neurons,omitempty"`
	Synapses map[int]int `json:"synapses,omitempty"`
	Binned   bool        `json:"binned,omitempty"`
}

// BrainPoint is one drawn bucket of the panel.
//
// EVERY READING IS A POINTER AND NULL MEANS NO READING, which is history.go rule
// 2 applied again: a five minutes in which this archive measured no genome is a
// GAP, and a gap drawn as zero would say every creature on the map lost its brain.
// 45 of the record's 183 hours have no crossing at all — outages of 6 h, 1 h,
// 24 h and 14 h — and they must read as the outages they were.
type BrainPoint struct {
	AtMs int64 `json:"tMs"`
	// N is how many distinct HELD genomes this bucket's numbers rest on, and Seen
	// how many distinct genomes the record says crossed in it. N/Seen is the
	// bucket's coverage, and it is published rather than folded into an opacity
	// because the reader looks hardest at the right-hand edge, which is the
	// WORST-covered part of the picture: 41.8 % at 0–6 h of age against 96.8 % at
	// 120–200 h. That is a fetch backlog draining backwards, not decay, and a
	// panel that hid it would be at its least honest exactly where it is read most.
	N    int `json:"n"`
	Seen int `json:"seen"`
	// MedSyn is the median synapse count per genome and MedHid the median HIDDEN
	// neuron count — the total less BrainNeuronFloor, floored at zero. These are
	// the two series that move: Spearman against time is 0.971 for synapses and
	// 0.840 for raw neurons, and the raw neuron count understates the change
	// about sevenfold because 48 of it never varies.
	MedSyn *int `json:"medSyn"`
	MedHid *int `json:"medHid"`
	// The interquartile band, p25 to p75, which is what the panel shades.
	LoSyn *int `json:"loSyn"`
	HiSyn *int `json:"hiSyn"`
	LoHid *int `json:"loHid"`
	HiHid *int `json:"hiHid"`
	// The extremes, for the tooltip only. They are NOT the band: on a bucket of
	// three hundred genomes the minimum hidden count is 0 in almost every bucket
	// (a genome at the floor) and the maximum is one mutant, so a min-max band
	// would draw a flat floor and a spiky ceiling and would encode the presence of
	// outliers rather than the spread of the population.
	MinSyn *int `json:"minSyn,omitempty"`
	MaxSyn *int `json:"maxSyn,omitempty"`
	MinHid *int `json:"minHid,omitempty"`
	MaxHid *int `json:"maxHid,omitempty"`
	// Binned says a reading in this bucket hit a histogram bound and was moved to
	// fit. A truncated answer a reader cannot see is a wrong answer.
	Binned bool `json:"binned,omitempty"`
}

// BrainHistory is /api/species/brains: the brain-complexity series over one
// window, at whatever resolution the caller asked for.
type BrainHistory struct {
	GeneratedAtMs int64 `json:"generatedAtMs"`
	FromMs        int64 `json:"fromMs"`
	ToMs          int64 `json:"toMs"`
	BucketMs      int64 `json:"bucketMs"`
	Buckets       int   `json:"buckets"`
	// SourceBucketMs is the aggregate's own resolution. A request for finer than
	// this gets this — the panel cannot invent detail the fold never kept — and
	// the page says so rather than drawing a smoother line than it has.
	SourceBucketMs int64 `json:"sourceBucketMs"`
	// NeuronFloor is BrainNeuronFloor, published rather than assumed by the page:
	// it is the one number that makes "hidden neurons" mean anything.
	NeuronFloor int `json:"neuronFloor"`
	// HaveFromMs is the oldest bucket this archive holds a MEASUREMENT in. Left of
	// it the panel has nothing, and after a lost sidecar that edge is recent — so
	// the page captions it the way the genealogy captions its ancestry floor
	// instead of drawing an unexplained emptiness.
	HaveFromMs int64 `json:"haveFromMs,omitempty"`
	// Genomes is how many measured genomes the drawn window rests on and Seen how
	// many the record says crossed in it.
	Genomes int `json:"genomes"`
	Seen    int `json:"seen"`
	// Records is how many species carry a persisted brain measurement, and
	// RecordsOverflow how many were dropped at the bound.
	Records         int `json:"records"`
	RecordsOverflow int `json:"recordsOverflow,omitempty"`
	// Persisted says a sidecar was replayed at startup; Lost says one was there
	// and could not be used, so this history starts now through no fault of the
	// record. Neither is ever expressed as a zero reading.
	Persisted bool `json:"persisted"`
	Lost      bool `json:"lost,omitempty"`
	// MaxSyn and MaxHid are the drawn window's own upper edges — the two y-scales
	// the panel fits to what it is showing, published here so the page and any
	// other renderer scale the same series the same way.
	MaxSyn int          `json:"maxSyn"`
	MaxHid int          `json:"maxHid"`
	Points []BrainPoint `json:"points"`
}

// BuildBrainHistory downsamples source buckets onto an arbitrary window. It is
// PURE, and it MERGES HISTOGRAMS: a drawn bucket's median is the median of every
// held genome inside it, never a mean of the finer medians it swallowed. That is
// the property rule 4 exists for, and the reason a bucket keeps a distribution
// rather than the three numbers a fixed-resolution series would need.
func BuildBrainHistory(src []BrainBucket, fromMs, toMs int64, buckets int) BrainHistory {
	if buckets < BrainMinBuckets {
		buckets = BrainMinBuckets
	}
	if buckets > BrainMaxBuckets {
		buckets = BrainMaxBuckets
	}
	if toMs <= fromMs {
		toMs = fromMs + BrainBucketMs
	}
	if max := BrainMaxWindow.Milliseconds(); toMs-fromMs > max {
		fromMs = toMs - max
	}
	// THE DRAWN RESOLUTION IS A WHOLE NUMBER OF SOURCE BUCKETS AND THE LEFT EDGE
	// SITS ON A SOURCE BOUNDARY. Anything else would split a five-minute
	// distribution across two drawn buckets, and splitting it needs a
	// within-bucket time the fold never kept — inventing one is the kind of
	// smoothing that turns an outage into a slope. So a request for finer than
	// the aggregate holds gets what the aggregate holds, and the answer publishes
	// the resolution it actually used rather than the one it was asked for.
	fromMs = brainBucketStart(fromMs)
	if toMs < fromMs+BrainBucketMs {
		toMs = fromMs + BrainBucketMs
	}
	span := toMs - fromMs
	srcBuckets := ceilDiv(span, BrainBucketMs)
	mult := ceilDiv(srcBuckets, int64(buckets))
	if mult < 1 {
		mult = 1
	}
	bucketMs := mult * BrainBucketMs
	buckets = int(ceilDiv(span, bucketMs))
	if buckets < 1 {
		buckets = 1
	}

	out := BrainHistory{
		FromMs:         fromMs,
		ToMs:           fromMs + bucketMs*int64(buckets),
		BucketMs:       bucketMs,
		Buckets:        buckets,
		SourceBucketMs: BrainBucketMs,
		NeuronFloor:    BrainNeuronFloor,
		Points:         make([]BrainPoint, buckets),
	}

	neu := make([]map[int]int, buckets)
	syn := make([]map[int]int, buckets)
	held := make([]int, buckets)
	seen := make([]int, buckets)
	binned := make([]bool, buckets)

	for _, b := range src {
		// A SOURCE BUCKET BELONGS TO THE DRAWN BUCKET ITS START FALLS IN, and to
		// exactly one. Splitting a five-minute distribution across two drawn
		// buckets would need a within-bucket time the fold never kept, and
		// inventing one is the kind of smoothing that turns a gap into a slope.
		if b.AtMs < out.FromMs || b.AtMs >= out.ToMs {
			continue
		}
		i := int((b.AtMs - out.FromMs) / bucketMs)
		if i < 0 || i >= buckets {
			continue
		}
		held[i] += b.Held
		seen[i] += b.Seen
		if b.Binned {
			binned[i] = true
		}
		if len(b.Neurons) > 0 {
			if neu[i] == nil {
				neu[i] = map[int]int{}
			}
			for v, c := range b.Neurons {
				neu[i][v] += c
			}
		}
		if len(b.Synapses) > 0 {
			if syn[i] == nil {
				syn[i] = map[int]int{}
			}
			for v, c := range b.Synapses {
				syn[i][v] += c
			}
		}
	}

	for i := 0; i < buckets; i++ {
		p := BrainPoint{AtMs: out.FromMs + int64(i)*bucketMs, N: held[i], Seen: seen[i],
			Binned: binned[i]}
		out.Genomes += held[i]
		out.Seen += seen[i]
		// A BUCKET WITH NO MEASUREMENT IS A GAP AND STAYS ONE. Not a zero, not an
		// interpolation across it, and not a value carried forward from the left.
		if n := histTotal(syn[i]); n > 0 {
			p.MedSyn = histQuantile(syn[i], n, 0.5, nil)
			p.LoSyn = histQuantile(syn[i], n, 0.25, nil)
			p.HiSyn = histQuantile(syn[i], n, 0.75, nil)
			p.MinSyn = histQuantile(syn[i], n, 0, nil)
			p.MaxSyn = histQuantile(syn[i], n, 1, nil)
			if p.HiSyn != nil && *p.HiSyn > out.MaxSyn {
				out.MaxSyn = *p.HiSyn
			}
		}
		if n := histTotal(neu[i]); n > 0 {
			p.MedHid = histQuantile(neu[i], n, 0.5, hidden)
			p.LoHid = histQuantile(neu[i], n, 0.25, hidden)
			p.HiHid = histQuantile(neu[i], n, 0.75, hidden)
			p.MinHid = histQuantile(neu[i], n, 0, hidden)
			p.MaxHid = histQuantile(neu[i], n, 1, hidden)
			if p.HiHid != nil && *p.HiHid > out.MaxHid {
				out.MaxHid = *p.HiHid
			}
		}
		out.Points[i] = p
	}
	return out
}

// hidden turns a raw neuron count into the count above the fixed floor. It is
// MONOTONE NON-DECREASING, which is the whole reason the histogram stores raw
// counts: a quantile of the transform is the transform of the quantile, so
// nothing has to be stored twice.
func hidden(v int) int {
	if v <= BrainNeuronFloor {
		return 0
	}
	return v - BrainNeuronFloor
}

func ceilDiv(a, b int64) int64 {
	if b <= 0 {
		return 0
	}
	return (a + b - 1) / b
}

func histTotal(h map[int]int) int {
	n := 0
	for _, c := range h {
		n += c
	}
	return n
}

// histQuantile is the NEAREST-RANK quantile of a histogram, optionally through a
// monotone transform. q=0 is the minimum and q=1 the maximum, so the extremes and
// the band come out of one function and cannot disagree about ordering.
func histQuantile(h map[int]int, total int, q float64, xf func(int) int) *int {
	if total <= 0 {
		return nil
	}
	keys := make([]int, 0, len(h))
	for v := range h {
		keys = append(keys, v)
	}
	sort.Ints(keys)
	rank := int(q*float64(total) + 0.5)
	if rank < 1 {
		rank = 1
	}
	if rank > total {
		rank = total
	}
	cum := 0
	for _, v := range keys {
		cum += h[v]
		if cum >= rank {
			out := v
			if xf != nil {
				out = xf(v)
			}
			return &out
		}
	}
	out := keys[len(keys)-1]
	if xf != nil {
		out = xf(out)
	}
	return &out
}

// brainSnapshot copies the source buckets covering one window out from under the
// aggregate's lock. It is bounded by the WINDOW and never by the record: a
// request for a day copies 288 buckets whatever the aggregate holds.
func (g *brainAgg) snapshot(fromMs, toMs int64) (out []BrainBucket, haveFrom int64,
	records, overflow int, persisted, lost bool) {

	g.mu.Lock()
	defer g.mu.Unlock()
	for k, b := range g.buckets {
		if b.held > 0 && (haveFrom == 0 || k < haveFrom) {
			haveFrom = k
		}
		if k < fromMs || k >= toMs {
			continue
		}
		c := BrainBucket{AtMs: k, Held: b.held, Seen: b.seen, Binned: b.binned}
		if len(b.neurons) > 0 {
			c.Neurons = make(map[int]int, len(b.neurons))
			for v, n := range b.neurons {
				c.Neurons[int(v)] = int(n)
			}
		}
		if len(b.synapses) > 0 {
			c.Synapses = make(map[int]int, len(b.synapses))
			for v, n := range b.synapses {
				c.Synapses[int(v)] = int(n)
			}
		}
		out = append(out, c)
	}
	return out, haveFrom, len(g.species), g.speciesOverflow, g.loaded, g.lost
}

// brainHistoryCacheMax is how many (from, to, buckets) answers are kept, and
// brainHistoryCacheFor how long. The panel rides a slow timer of its own, but the
// axis it shares re-fits on every poll, so two readers on slightly different
// windows must not each walk the aggregate.
const (
	brainHistoryCacheMax = 8
	brainHistoryCacheFor = 10 * time.Second
)

type brainHistoryEntry struct {
	at  time.Time
	val BrainHistory
}

// BrainHistoryView answers one /api/species/brains request, from a short-lived
// bounded cache over a snapshot of the maintained aggregate. It reads NO FILE and
// touches neither the ledger nor the genome store: the whole cost of the answer
// was paid when each genome arrived.
func (a *Archive) BrainHistoryView(fromMs, toMs int64, buckets int) BrainHistory {
	key := strconv.FormatInt(fromMs, 10) + "|" + strconv.FormatInt(toMs, 10) + "|" +
		strconv.Itoa(buckets)
	a.brainHistMu.Lock()
	defer a.brainHistMu.Unlock()
	if a.brainHist == nil {
		a.brainHist = map[string]brainHistoryEntry{}
	}
	if e, ok := a.brainHist[key]; ok && time.Since(e.at) < brainHistoryCacheFor {
		return e.val
	}
	src, haveFrom, records, overflow, persisted, lost := a.brainAgg.snapshot(fromMs, toMs)
	h := BuildBrainHistory(src, fromMs, toMs, buckets)
	h.GeneratedAtMs = time.Now().UnixMilli()
	h.HaveFromMs = haveFrom
	h.Records, h.RecordsOverflow = records, overflow
	h.Persisted, h.Lost = persisted, lost
	if len(a.brainHist) >= brainHistoryCacheMax {
		oldest, oldestAt := "", time.Time{}
		for k, e := range a.brainHist {
			if oldestAt.IsZero() || e.at.Before(oldestAt) {
				oldest, oldestAt = k, e.at
			}
		}
		delete(a.brainHist, oldest)
	}
	a.brainHist[key] = brainHistoryEntry{at: time.Now(), val: h}
	return h
}
