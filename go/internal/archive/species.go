package archive

// The species half of the operator surface: the LEDGER AGGREGATE behind every
// species view, and the FLAT INDEX of /api/species (contract-b-m4.md §10.1,
// §16 B12, §19 B19).
//
// The page draws the merged view of tree.go now, and this index is still served
// and still read: ringstat renders it (cmd/ringstat), an endpoint is a contract
// with whoever is calling it rather than a private detail of one renderer, and a
// terminal table is a different medium with a different answer. Both are built
// from the aggregate below, so the two cannot disagree about who is alive.
//
// It answers one question the map cannot — "what lives in this multiverse, and
// which of it travels" — by joining two sources that must never be confused
// with each other:
//
//	THE CENSUS says who is ALIVE, right now, in each world. It is the only
//	source for the index itself: THE INDEX LISTS CURRENTLY-ALIVE SPECIES AND
//	NOTHING ELSE. A species that crossed a lane four hours ago and is extinct
//	everywhere today is a ledger fact, not a resident, and putting it in a list
//	of what lives here would be exactly the mis-reading D11 and §10.1 forbid:
//	"a database built from migrations holds migrants and their ancestors, never
//	a resident population".
//
//	THE LEDGER says what has CROSSED. It is only ever an ANNOTATION on a row the
//	census put there — how often that species has travelled, when it first did,
//	how many distinct genomes of it the archive has seen. Nothing in this file
//	sums the two, and nothing here promotes a ledger species into the index.
//
// THREE RULES, and each is a decision this file records:
//
//  1. THE LEDGER AGGREGATE IS INCREMENTAL, NEVER A SCAN. migrations.jsonl is
//     over half a million lines on the running rig and grows forever. A poll
//     that re-read it would put a file read on the operator's 2-second cadence
//     and, worse, would make the cost of watching grow with the age of the
//     record. The aggregate is built ONCE during the replay New() already
//     performs, and maintained afterwards by the same call site that records a
//     migration — one map lookup inside a lock that is already held. Reading it
//     is a copy, and it touches no disk at all (Risk 4: nothing on the
//     migration path ever waits for a reader).
//
//  2. NORMALIZED IS THE KEY, RAW IS THE LABEL. Species are grouped under the
//     A34-normalized full name, because two worlds spelling one species
//     differently are one species; but what is DISPLAYED is a raw spelling the
//     owning world holds, doubled spaces and all, because a tidied name is a
//     name the player cannot find in their own game (contract-a.md §16 A34, §17
//     A36). The normalized form is a comparison key and is never shown.
//
//  3. EVERY BOUND IS NAMED AND PUBLISHED. The aggregate is bounded in species,
//     in genome fingerprints per species and in remembered crossings per
//     species; the index is bounded by the census that produced it. A view that
//     hit a bound says so, because a truncated answer a reader cannot see is a
//     wrong answer.

import (
	"hash/fnv"
	"sort"
	"strconv"
	"sync"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/wire"
)

// The aggregate's bounds.
const (
	// speciesAggMax is how many distinct species the ledger aggregate tracks.
	// The running rig's ledger holds ~780 after a week of six worlds speciating
	// freely, so this is roughly five times the observed shape rather than a
	// number picked to be large. Past it, new species are COUNTED and not
	// tracked, and the view says so.
	speciesAggMax = 4096
	// speciesGenomeMax bounds the distinct-genome fingerprint set of ONE
	// species. The whole ledger holds ~59 000 distinct genomes across every
	// species; a single species reaching this many is a world in a genetic
	// explosion, and "8192+" is a truer thing to print than an unbounded map.
	speciesGenomeMax = 8192
	// speciesRecentMax is how many recent crossings are remembered per species,
	// for the detail panel's "who crossed, and when". It is a SAMPLE of the
	// lanes that species uses and never a count: the count is Crossings.
	speciesRecentMax = 6
)

// SpeciesCrossing is one recorded migration as the species detail reports it.
// It is the ledger's own record, not the live hop feed's: the feed forgets
// after a minute, and this survives a restart because it is rebuilt from the
// ledger during replay.
type SpeciesCrossing struct {
	AtMs     int64 `json:"atMs"`
	AgeMs    int64 `json:"ageMs"`
	FromSlot int   `json:"fromSlot"`
	ToSlot   int   `json:"toSlot"`
	// ExitEdge is the edge the organism LEFT by, which is what names the lane
	// under two-way lanes: on an axis of length 2 the forward and reverse lanes
	// join the same two worlds (§17, B13). Records written before D17 carry "".
	ExitEdge string `json:"exitEdge,omitempty"`
}

// speciesAgg is the ledger's whole answer about one species, maintained
// incrementally. It is guarded by Archive.mu, like every other counter the
// migration path touches.
type speciesAgg struct {
	// There is deliberately NO NAME HERE. The aggregate is keyed on the
	// A34-normalized comparison key and holds counts; the LABEL a reader sees
	// comes from the census, which is the only source that can say how the
	// owning world spells the name today (contract-a.md §17, A36). Keeping a
	// second spelling here would be a second opinion about a display name, and
	// the index would eventually print the older one.
	crossings int
	firstMs   int64
	lastMs    int64
	// genomes is a set of 64-bit FINGERPRINTS of the genome hashes, not the
	// hashes themselves. The hash is a 85-byte string and the rig holds ~59 000
	// distinct ones; fingerprinting turns a 5 MB retained-string set into a
	// bounded integer set. The count is what is displayed, and the collision
	// probability over the whole observed corpus is around 1e-10 — small enough
	// that the number is honest, and it is the reason the field is a COUNT and
	// never a list a reader could join against anything.
	genomes          map[uint64]struct{}
	genomesTruncated bool
	// recent is the newest crossings, oldest first, bounded by
	// speciesRecentMax.
	recent []SpeciesCrossing
	// parent is the RAW parent species name the migration envelope last recorded
	// for this species, "" when none ever did, and parentKey is the same name
	// under the A34 comparison key. THE RAW FORM IS THE LABEL AND THE KEY IS THE
	// EDGE, which is rule 2 applied to one more field.
	//
	// Two readers use these and they use them differently. The flat index
	// SHOWS parent and resolves nothing (contract-a.md §16, A31: the registry a
	// name resolves against is inside a game process). The genealogy of tree.go
	// treats parentKey as ONE EDGE OF A GRAPH — still no resolution, because an
	// edge between two names this archive was told about is not a claim about
	// any world's registry; it is a claim about what the record says, which is
	// the only thing this archive ever claims.
	//
	// LATEST WRITER WINS, on the archive's own clock. A species has one parent
	// species in the game's model (Species.parentSpecies is a single reference),
	// so a second answer is a correction and not a second edge — and taking the
	// newest is what lets a world that re-derived its own tree be believed
	// instead of argued with.
	parent     string
	parentKey  string
	parentAtMs int64
	// genomeHash is the LATEST genome hash any crossing of this species carried,
	// and genomeAtMs when. ONE STRING PER SPECIES, maintained by this same
	// single writer, for the same reason parent is: the genealogy publishes a
	// brain shape per species, the material to compute it from is in the
	// content-addressed store, and the alternative — finding a genome for a
	// species at request time — is a walk of the ledger per poll, which is the
	// shape rule 1 refuses.
	//
	// LATEST, not first, and not "the one the store still holds". A species'
	// newest genome is the one that describes what it is now, and whether a copy
	// of it survives is a question for the store and the retention horizon
	// (§23, B33) — this aggregate records what CROSSED, and an answer the store
	// cannot back is absent at the view, never wrong here.
	genomeHash string
	genomeAtMs int64
}

// speciesLedger is the whole aggregate.
type speciesLedger struct {
	byKey map[string]*speciesAgg
	// overflow counts species the aggregate refused to track because it was
	// full. It is published, because a truncated aggregate a reader cannot see
	// is a wrong answer.
	overflow int
	// edges is how many tracked species carry a parent name — maintained here
	// rather than counted per request, because the genealogy publishes it on
	// every poll and a walk of the whole aggregate to produce one integer is the
	// shape rule 1 exists to refuse. It counts a species ONCE, when it first
	// gains an edge: a later correction replaces that species' parent and does
	// not add a second.
	edges int
	// edgeFirstMs is the recordedAt of the EARLIEST record that ever carried a
	// usable parent species — the FLOOR OF THE WHOLE GENEALOGY. Nothing older
	// than it can ever be an edge, however many crossings the ledger holds
	// before it, because the parent-species field arrived with contract-a.md
	// §16 A30 and the record predates it: on the running rig the first 19.5
	// hours of crossings carry no species block at all. It is what lets the tree
	// say WHY a chain of ancestors stops where it stops, instead of leaving a
	// reader to conclude the top of it is the first creature that ever lived.
	//
	// It is ONE INT64, maintained by the same single writer as every other
	// counter here and rebuilt by the same replay, because the alternative — a
	// walk of the aggregate per poll to produce one caption — is exactly the
	// shape rule 1 refuses. 0 means no record has ever named a parent.
	edgeFirstMs int64
}

func newSpeciesLedger() *speciesLedger {
	return &speciesLedger{byKey: map[string]*speciesAgg{}}
}

// observeSpeciesLocked folds one recorded migration into the aggregate. The
// caller holds a.mu, and this is the only writer.
//
// It is called from exactly two places and that is the point: the startup
// replay New() already runs, and the live path in onMigration. One function,
// two call sites, no third view of the same fact.
func (a *Archive) observeSpeciesLocked(rec Record) {
	if rec.Species == nil {
		// ABSENT IS ABSENT. An envelope that carried no species block is a
		// crossing by a creature of no reported species, and inventing a key for
		// it would put a species in the index that no world ever named.
		return
	}
	key := wire.SpeciesKey(rec.Species.GenericName, rec.Species.SpecificName)
	if key == "" {
		return
	}
	e := a.species.byKey[key]
	if e == nil {
		if len(a.species.byKey) >= speciesAggMax {
			a.species.overflow++
			return
		}
		e = &speciesAgg{genomes: map[uint64]struct{}{}}
		a.species.byKey[key] = e
	}
	e.crossings++
	if e.firstMs == 0 || rec.RecordedAt < e.firstMs {
		e.firstMs = rec.RecordedAt
	}
	if rec.RecordedAt > e.lastMs {
		e.lastMs = rec.RecordedAt
	}
	if rec.Lineage != nil && rec.Lineage.GenomeHash != "" {
		if len(e.genomes) >= speciesGenomeMax {
			e.genomesTruncated = true
		} else {
			e.genomes[fingerprint(rec.Lineage.GenomeHash)] = struct{}{}
		}
		// The FULL hash of the newest crossing, kept once. The fingerprint set
		// above is a count and is deliberately unjoinable (see speciesAgg.genomes);
		// this is one hash a reader of the store can actually open, and the
		// genealogy uses it to say how big this species' brain is.
		if rec.RecordedAt >= e.genomeAtMs {
			e.genomeHash = rec.Lineage.GenomeHash
			e.genomeAtMs = rec.RecordedAt
		}
	}
	if rec.Species.ParentGenericName != "" {
		pkey := wire.SpeciesKey(rec.Species.ParentGenericName, rec.Species.ParentSpecificName)
		// A SELF-PARENT IS DROPPED AT INGEST, which is the cheapest place a cycle
		// can be refused and the only one where it costs nothing. It is not a
		// hypothetical shape — the two halves are attacker-chosen strings and the
		// key is normalized, so a name and its own parent CAN normalize together
		// without either world intending it. The record still says what it said;
		// this aggregate simply holds no edge from a species to itself, so no
		// reader has to cope with one. tree.go guards the longer cycles this
		// cannot see.
		if pkey != "" && pkey != key {
			// THE FLOOR IS A MINIMUM OVER EVERY SUCH RECORD, and it is taken
			// outside the latest-writer test below on purpose: a record that
			// loses that test is an OLDER answer about one species' parent, and
			// an older answer is precisely what moves a floor down.
			if rec.RecordedAt > 0 &&
				(a.species.edgeFirstMs == 0 || rec.RecordedAt < a.species.edgeFirstMs) {
				a.species.edgeFirstMs = rec.RecordedAt
			}
			// LATEST WRITER WINS for the edge itself; see speciesAgg.parent.
			if rec.RecordedAt >= e.parentAtMs {
				if e.parentKey == "" {
					a.species.edges++
				}
				e.parent = rec.Species.ParentGenericName + " " + rec.Species.ParentSpecificName
				e.parentKey = pkey
				e.parentAtMs = rec.RecordedAt
			}
		}
	}
	e.recent = append(e.recent, SpeciesCrossing{
		AtMs: rec.RecordedAt, FromSlot: rec.SourceSlot, ToSlot: rec.DestSlot,
		ExitEdge: rec.ExitEdge,
	})
	if n := len(e.recent); n > speciesRecentMax {
		// Keep the NEWEST: a panel showing what a species has been doing lately
		// is worthless if the entries it drops are the recent ones.
		e.recent = append(e.recent[:0], e.recent[n-speciesRecentMax:]...)
	}
}

// fingerprint is FNV-1a over a genome hash string. See speciesAgg.genomes for
// why the set holds these rather than the hashes.
func fingerprint(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// ---------------------------------------------------------------- the view

// SpeciesWorld is one world's holding of one species, from that world's own
// census.
type SpeciesWorld struct {
	Slot    int `json:"slot"`
	Bibites int `json:"bibites"`
	Eggs    int `json:"eggs"`
}

// SpeciesRow is one currently-alive species, with what the ledger knows about
// it hung off the side.
type SpeciesRow struct {
	// Key is the A34-normalized comparison key. It is published because the
	// page needs a stable id for a row and for the detail request, and it is
	// deliberately NOT the label: Name is.
	Key string `json:"key"`
	// Name is a RAW spelling, exactly as the first reporting world in slot
	// order holds it. Spellings names every OTHER raw spelling alive right now
	// under the same key — two worlds that write one species differently are one
	// species and two labels, and flattening that would hide a real difference.
	Name      string   `json:"name"`
	Spellings []string `json:"spellings,omitempty"`

	// Population and Eggs are sums over the worlds below, and therefore over
	// KNOWN CENSUSES ONLY. A world reporting no census contributes nothing here
	// and is counted in SpeciesIndex.CensuslessSlots instead.
	Population int            `json:"population"`
	Eggs       int            `json:"eggs"`
	Worlds     []SpeciesWorld `json:"worlds"`

	// The three badges of §19 B19 and §16 B12.
	//
	// Excluded: this species is named on at least one world's migrationExclude,
	// which is WHY a world can be full of it and no lane ever carry it. It is a
	// statement about ExcludedBy's worlds and never an obligation on any other
	// (contract-a.md §19, A42).
	Excluded   bool  `json:"excluded"`
	ExcludedBy []int `json:"excludedBy,omitempty"`
	// Everywhere: alive in every world that is reporting a census. It is only
	// claimed when at least two worlds report — with one reporting world
	// "everywhere" and "endemic" are the same sentence, and printing both would
	// dress a single census up as a map-wide finding.
	Everywhere bool `json:"everywhere"`
	// Endemic: alive in exactly one reporting world.
	Endemic bool `json:"endemic"`

	// ---- the ledger's annotation. Crossings is ALL TIME, from this archive's
	// own record, and 0 means this archive has recorded no crossing by this
	// species — not that none happened before the archive existed.
	Crossings int   `json:"crossings"`
	FirstMs   int64 `json:"firstMs,omitempty"`
	LastMs    int64 `json:"lastMs,omitempty"`
	// LastAgeMs is how long ago the newest recorded crossing was, and it is
	// omitted rather than zeroed when there has never been one.
	LastAgeMs *int64 `json:"lastAgeMs,omitempty"`
	// Genomes is how many DISTINCT genomes of this species the ledger has seen
	// cross. GenomesAtLeast says the per-species bound was hit and the true
	// number is higher.
	Genomes        int  `json:"genomes"`
	GenomesAtLeast bool `json:"genomesAtLeast,omitempty"`
	// Parent is the raw parent species name the lineage annex recorded, when one
	// ever did. SHOWN AND NOTHING ELSE — no tree, no ancestry.
	Parent string `json:"parent,omitempty"`
	// Recent is the newest recorded crossings of this species: lane and age. A
	// SAMPLE, bounded by speciesRecentMax, never a count.
	Recent []SpeciesCrossing `json:"recent,omitempty"`
}

// SpeciesIndex is /api/species: every species alive anywhere on the map right
// now, with what the ledger knows about each.
type SpeciesIndex struct {
	GeneratedAtMs int64 `json:"generatedAtMs"`
	// HaveStatus and StatusAgeMs are §10.1's freshness rule carried onto this
	// view: an index built from a frame this archive cannot date, or dated too
	// long ago, is not state, and the page says so rather than drawing a list.
	HaveStatus  bool  `json:"haveStatus"`
	StatusAgeMs int64 `json:"statusAgeMs"`
	// ReportingSlots is how many slots contributed a census, and
	// CensuslessSlots how many are live-but-silent. The second is what stops
	// "everywhere" being read as a map-wide claim when half the map said
	// nothing.
	ReportingSlots  int `json:"reportingSlots"`
	CensuslessSlots int `json:"censuslessSlots"`
	// TruncatedSlots is how many contributing censuses were themselves capped
	// at the 32 most numerous species, so the union below is missing whatever
	// those worlds did not name.
	TruncatedSlots int `json:"truncatedSlots"`
	// LedgerSpecies is how many distinct species the crossing record holds — a
	// bigger number than the index, because it counts everything that has ever
	// travelled and this list counts what is alive. LedgerOverflow is how many
	// the aggregate refused to track after speciesAggMax.
	LedgerSpecies  int          `json:"ledgerSpecies"`
	LedgerOverflow int          `json:"ledgerOverflow,omitempty"`
	LedgerRecords  int          `json:"ledgerRecords"`
	Species        []SpeciesRow `json:"species"`
}

// SpeciesIndexView builds the whole flat species index. It takes the archive's
// lock twice, briefly, and touches no disk.
func (a *Archive) SpeciesIndexView() SpeciesIndex {
	// StatusView takes the lock itself, so it is called first and finished
	// with before the aggregate is read. Two short holds, never a nested one.
	return a.speciesIndexFrom(a.StatusView())
}

// speciesIndexFrom is the index over ONE status frame, so a caller that needs
// both this and something else derived from the same frame — the genealogy needs
// the map's own grid — reads the frame once and cannot end up describing two
// different instants in one answer.
func (a *Archive) speciesIndexFrom(view Status) SpeciesIndex {
	nowMs := time.Now().UnixMilli()

	out := SpeciesIndex{
		GeneratedAtMs: nowMs,
		HaveStatus:    view.HaveStatus,
		StatusAgeMs:   view.StatusAgeMs,
		LedgerRecords: view.Records,
		Species:       []SpeciesRow{},
	}

	// ---- the union, from the censuses and from nothing else.
	type acc struct {
		row       *SpeciesRow
		spellings map[string]bool
		order     []string
	}
	byKey := map[string]*acc{}
	order := []string{}
	// excludedBy is built from every world's published list, whatever that
	// world's own census says: a world excludes a species whether or not it
	// currently holds one.
	excludedBy := map[string][]int{}

	for _, sv := range view.Slots {
		if sv.MigrationExcludeKnown {
			for _, name := range sv.MigrationExclude {
				// COMPARED AS PUBLISHED. The exclusion lane is already
				// A34-normalized by the mod that owns the policy, and
				// re-normalizing it here would describe a match nobody
				// performs. It is the CENSUS COPY that is normalized for the
				// comparison, and neither is rewritten (§19, B18).
				excludedBy[name] = append(excludedBy[name], sv.Slot)
			}
		}
		if !sv.StatsKnown {
			continue
		}
		if !sv.SpeciesKnown {
			out.CensuslessSlots++
			continue
		}
		out.ReportingSlots++
		if sv.SpeciesTruncated {
			out.TruncatedSlots++
		}
		for _, e := range sv.Species {
			key := censusEntryKey(e)
			if key == "" {
				continue
			}
			raw := e.GenericName + " " + e.SpecificName
			a2 := byKey[key]
			if a2 == nil {
				a2 = &acc{row: &SpeciesRow{Key: key, Name: raw, Worlds: []SpeciesWorld{}},
					spellings: map[string]bool{raw: true}}
				byKey[key] = a2
				order = append(order, key)
			} else if !a2.spellings[raw] {
				// A SECOND RAW SPELLING of one species. Both are real: two
				// worlds legitimately hold the same species under labels that
				// differ by whitespace, and the page shows every one of them
				// rather than picking a winner and hiding the difference.
				a2.spellings[raw] = true
				a2.order = append(a2.order, raw)
			}
			a2.row.Population += e.Bibites
			a2.row.Eggs += e.Eggs
			a2.row.Worlds = append(a2.row.Worlds,
				SpeciesWorld{Slot: sv.Slot, Bibites: e.Bibites, Eggs: e.Eggs})
		}
	}

	// ---- the ledger's annotation, under the lock, as a copy.
	a.mu.Lock()
	out.LedgerSpecies = len(a.species.byKey)
	out.LedgerOverflow = a.species.overflow
	for _, key := range order {
		row := byKey[key].row
		e := a.species.byKey[key]
		if e == nil {
			continue
		}
		row.Crossings = e.crossings
		row.FirstMs = e.firstMs
		row.LastMs = e.lastMs
		if e.lastMs > 0 {
			age := nowMs - e.lastMs
			row.LastAgeMs = &age
		}
		row.Genomes = len(e.genomes)
		row.GenomesAtLeast = e.genomesTruncated
		row.Parent = e.parent
		row.Recent = make([]SpeciesCrossing, 0, len(e.recent))
		for i := len(e.recent) - 1; i >= 0; i-- {
			c := e.recent[i]
			c.AgeMs = nowMs - c.AtMs
			row.Recent = append(row.Recent, c)
		}
	}
	a.mu.Unlock()

	// ---- the badges, once the whole union is known.
	for _, key := range order {
		a2 := byKey[key]
		row := a2.row
		row.Spellings = a2.order
		row.Endemic = len(row.Worlds) == 1
		row.Everywhere = out.ReportingSlots >= 2 && len(row.Worlds) == out.ReportingSlots
		if slots, ok := excludedBy[key]; ok {
			row.Excluded = true
			row.ExcludedBy = slots
		}
		out.Species = append(out.Species, *row)
	}

	// Population descending is the default the page opens on, and the tie is
	// broken on the key so the order is stable between polls — a list that
	// reshuffles under a reader's cursor is unusable.
	sort.SliceStable(out.Species, func(i, j int) bool {
		if out.Species[i].Population != out.Species[j].Population {
			return out.Species[i].Population > out.Species[j].Population
		}
		return out.Species[i].Key < out.Species[j].Key
	})
	return out
}

// ---------------------------------------------------------- species history

// speciesHistoryCacheMax is how many (species, window, buckets) answers are
// kept. A reader clicking through species must not re-read the sample file per
// click, and a bounded cache is what stops a page with many open panels from
// turning into an unbounded one.
const speciesHistoryCacheMax = 12

// SpeciesHistory is one species' population per world over time, downsampled
// from the archive's own durable sample file exactly as /api/history is.
//
// THE ZERO HERE IS A READING. A bucket in which a world reported a census that
// did not name this species is 0 — that world genuinely held none — and a
// bucket in which the world reported NO census is null, which is unknown. The
// two must not be flattened, and they are the reason this endpoint exists
// rather than the page inferring a series from the live view.
type SpeciesHistory struct {
	GeneratedAtMs int64  `json:"generatedAtMs"`
	Key           string `json:"key"`
	FromMs        int64  `json:"fromMs"`
	ToMs          int64  `json:"toMs"`
	BucketMs      int64  `json:"bucketMs"`
	Buckets       int    `json:"buckets"`
	Samples       int    `json:"samples"`
	Truncated     bool   `json:"truncated"`
	// Max is the shared y-scale across the per-world sparklines. Small
	// multiples that do not share a scale invite a reader to compare shapes
	// that are not comparable.
	Max   int             `json:"max"`
	Slots []HistorySeries `json:"slots"`
	Total []HistoryPoint  `json:"total"`
}

type speciesHistoryCache struct {
	mu      sync.Mutex
	entries map[string]speciesHistoryEntry
	// trends is the same bound over the same file for the whole-map trend
	// answer. It shares this lock because it shares the read: two readers of the
	// same tail must not queue behind two different mutexes.
	trends map[string]speciesTrendsEntry
}

type speciesHistoryEntry struct {
	at  time.Time
	val SpeciesHistory
}

type speciesTrendsEntry struct {
	at  time.Time
	val SpeciesTrends
}

// BuildSpeciesHistory downsamples one species out of a slice of samples. It is
// pure — every input is an argument — which is what makes the downsampling
// testable without a rig or a file, exactly as BuildHistory is.
func BuildSpeciesHistory(samples []Status, key string, nowMs int64,
	window time.Duration, buckets int) SpeciesHistory {

	if window < HistoryMinWindow {
		window = HistoryMinWindow
	}
	if window > HistoryMaxWindow {
		window = HistoryMaxWindow
	}
	if buckets < HistoryMinBuckets {
		buckets = HistoryMinBuckets
	}
	if buckets > HistoryMaxBuckets {
		buckets = HistoryMaxBuckets
	}
	windowMs := window.Milliseconds()
	bucketMs := windowMs / int64(buckets)
	if bucketMs < 1 {
		bucketMs = 1
	}
	fromMs := nowMs - bucketMs*int64(buckets)

	h := SpeciesHistory{
		GeneratedAtMs: nowMs,
		Key:           key,
		FromMs:        fromMs,
		ToMs:          fromMs + bucketMs*int64(buckets),
		BucketMs:      bucketMs,
		Buckets:       buckets,
		Slots:         []HistorySeries{},
		Total:         make([]HistoryPoint, buckets),
	}

	ordered := append([]Status(nil), samples...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].GeneratedAtMs < ordered[j].GeneratedAtMs
	})

	perSlot := map[int][]bucketAcc{}
	peerOf := map[int]string{}
	total := make([]bucketAcc, buckets)

	for _, s := range ordered {
		if s.GeneratedAtMs < fromMs {
			continue
		}
		idx := int((s.GeneratedAtMs - fromMs) / bucketMs)
		if idx < 0 || idx >= buckets {
			continue
		}
		h.Samples++
		sampleTotal, sampleKnown := 0, false
		for _, sv := range s.Slots {
			accs, ok := perSlot[sv.Slot]
			if !ok {
				accs = make([]bucketAcc, buckets)
				perSlot[sv.Slot] = accs
			}
			if sv.PeerID != "" {
				peerOf[sv.Slot] = sv.PeerID
			}
			accs[idx].n++
			if !sv.Live {
				accs[idx].dark = true
			}
			if !sv.StatsKnown || !sv.SpeciesKnown {
				// UNKNOWN, not zero: that world told us nothing about its
				// species in this sample, which is a different fact from
				// telling us it has none.
				continue
			}
			n := 0
			for _, e := range sv.Species {
				if censusEntryKey(e) == key {
					n += e.Bibites
				}
			}
			accs[idx].sum += n
			accs[idx].have++
			sampleTotal += n
			sampleKnown = true
			if n > h.Max {
				h.Max = n
			}
		}
		total[idx].n++
		if sampleKnown {
			total[idx].sum += sampleTotal
			total[idx].have++
		}
	}

	for i := 0; i < buckets; i++ {
		h.Total[i] = total[i].point(fromMs + int64(i)*bucketMs)
	}
	slots := make([]int, 0, len(perSlot))
	for slot := range perSlot {
		slots = append(slots, slot)
	}
	sort.Ints(slots)
	for _, slot := range slots {
		accs := perSlot[slot]
		series := HistorySeries{Slot: slot, PeerID: peerOf[slot],
			Points: make([]HistoryPoint, buckets)}
		for i := 0; i < buckets; i++ {
			p := accs[i].point(fromMs + int64(i)*bucketMs)
			series.Points[i] = p
			if p.Value != nil {
				if *p.Value > series.Max {
					series.Max = *p.Value
				}
				v := *p.Value
				series.Last = &v
			}
		}
		h.Slots = append(h.Slots, series)
	}
	return h
}

// SpeciesHistoryView answers one /api/species/history request, from a
// short-lived bounded cache over a bounded tail read of the sample file — the
// same two bounds /api/history has, for the same reason: the migration path
// must never wait for a reader, and neither must the disk.
func (a *Archive) SpeciesHistoryView(key string, window time.Duration, buckets int) (SpeciesHistory, error) {
	ck := key + "|" + window.String() + "|" + strconv.Itoa(buckets)
	a.speciesHistory.mu.Lock()
	defer a.speciesHistory.mu.Unlock()
	if a.speciesHistory.entries == nil {
		a.speciesHistory.entries = map[string]speciesHistoryEntry{}
	}
	if e, ok := a.speciesHistory.entries[ck]; ok && time.Since(e.at) < historyCacheFor {
		return e.val, nil
	}
	samples, truncated, err := ReadMetricsTail(a.metrics.Path(), historyTailBytes)
	if err != nil {
		return SpeciesHistory{}, err
	}
	h := BuildSpeciesHistory(samples, key, time.Now().UnixMilli(), window, buckets)
	h.Truncated = truncated
	if len(a.speciesHistory.entries) >= speciesHistoryCacheMax {
		// Evict the oldest. A map with no eviction is the unbounded thing this
		// cache exists to prevent.
		oldest, oldestAt := "", time.Time{}
		for k, e := range a.speciesHistory.entries {
			if oldestAt.IsZero() || e.at.Before(oldestAt) {
				oldest, oldestAt = k, e.at
			}
		}
		delete(a.speciesHistory.entries, oldest)
	}
	a.speciesHistory.entries[ck] = speciesHistoryEntry{at: time.Now(), val: h}
	return h, nil
}

// ---------------------------------------------------------- species trends
//
// ONE READ FOR EVERY SPARKLINE. The genealogy draws a trend line beside every
// living species, and the obvious way to feed it — /api/species/history once per
// species — is a bounded tail read of the sample file PER SPECIES, forty of them
// on the running rig, on a tab a reader leaves open. This is the same
// downsampling over the same file, with the whole living set folded in ONE pass,
// and it is the reason the trend column costs one request rather than forty.
//
// It is a SHAPE AND NOT A READING, and the payload says so by being small: one
// value per bucket per species, the map-wide total of that species, and no
// per-world split at all. A reader who wants the per-world answer opens the
// species' own history, which still exists and still says which worlds were
// silent. The two must not be confused: a null here is the same unknown the rest
// of this file means by it, and a 0 is still a census that reported none.

// speciesTrendMax bounds how many species one trends answer carries. The census
// caps each world at 32 species and there are six worlds, so the living union
// cannot exceed 192 by construction; this is that bound written down rather than
// assumed, and a request that hits it says so.
const speciesTrendMax = 192

// SpeciesTrend is one species' map-wide population over the window.
type SpeciesTrend struct {
	Key string `json:"key"`
	// Points is one value per bucket, null where no world knew a value in that
	// bucket. It is a bare array rather than a list of objects because the bucket
	// times are the same for every species in the answer and are published once,
	// at the top — forty species times forty buckets is where a payload gets away
	// from you.
	Points []*int `json:"points"`
	// Max is this species' own largest known value, and Last the newest known
	// one. The scale is PER SPECIES here, unlike History's shared one: these are
	// shapes drawn 84 pixels wide beside rows of wildly different abundance, and
	// a shared scale would flatten every rare species into a straight line.
	Max  int  `json:"max"`
	Last *int `json:"last,omitempty"`
}

// SpeciesTrends is /api/species/trends: every living species' recent shape, in
// one answer.
type SpeciesTrends struct {
	GeneratedAtMs int64 `json:"generatedAtMs"`
	FromMs        int64 `json:"fromMs"`
	ToMs          int64 `json:"toMs"`
	BucketMs      int64 `json:"bucketMs"`
	Buckets       int   `json:"buckets"`
	Samples       int   `json:"samples"`
	Truncated     bool  `json:"truncated"`
	// Capped says the living union was larger than speciesTrendMax and the rest
	// carry no trend. A truncated answer a reader cannot see is a wrong answer.
	Capped  bool           `json:"capped,omitempty"`
	Species []SpeciesTrend `json:"species"`
}

// BuildSpeciesTrends downsamples every key in one pass. It is pure, like
// BuildHistory and BuildSpeciesHistory, and for the same reason.
func BuildSpeciesTrends(samples []Status, keys []string, nowMs int64,
	window time.Duration, buckets int) SpeciesTrends {

	if window < HistoryMinWindow {
		window = HistoryMinWindow
	}
	if window > HistoryMaxWindow {
		window = HistoryMaxWindow
	}
	if buckets < HistoryMinBuckets {
		buckets = HistoryMinBuckets
	}
	if buckets > HistoryMaxBuckets {
		buckets = HistoryMaxBuckets
	}
	bucketMs := window.Milliseconds() / int64(buckets)
	if bucketMs < 1 {
		bucketMs = 1
	}
	fromMs := nowMs - bucketMs*int64(buckets)

	out := SpeciesTrends{
		GeneratedAtMs: nowMs,
		FromMs:        fromMs,
		ToMs:          fromMs + bucketMs*int64(buckets),
		BucketMs:      bucketMs,
		Buckets:       buckets,
		Species:       []SpeciesTrend{},
	}

	wanted := make(map[string]int, len(keys))
	ordered := make([]string, 0, len(keys))
	for _, k := range keys {
		if k == "" {
			continue
		}
		if _, dup := wanted[k]; dup {
			continue
		}
		if len(ordered) >= speciesTrendMax {
			out.Capped = true
			break
		}
		wanted[k] = len(ordered)
		ordered = append(ordered, k)
	}
	if len(ordered) == 0 {
		return out
	}

	// One accumulator grid: species-major, bucket-minor. It is bounded by
	// speciesTrendMax * HistoryMaxBuckets and is thrown away with this call.
	accs := make([][]bucketAcc, len(ordered))
	for i := range accs {
		accs[i] = make([]bucketAcc, buckets)
	}

	sorted := append([]Status(nil), samples...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].GeneratedAtMs < sorted[j].GeneratedAtMs
	})
	// perSample is reused across samples so the pass allocates nothing per
	// sample: it holds this sample's total per species and whether any world was
	// in a position to know it.
	total := make([]int, len(ordered))
	known := make([]bool, len(ordered))
	for _, s := range sorted {
		if s.GeneratedAtMs < fromMs {
			continue
		}
		idx := int((s.GeneratedAtMs - fromMs) / bucketMs)
		if idx < 0 || idx >= buckets {
			continue
		}
		out.Samples++
		for i := range total {
			total[i], known[i] = 0, false
		}
		for _, sv := range s.Slots {
			if !sv.StatsKnown || !sv.SpeciesKnown {
				// UNKNOWN, not zero: that world told us nothing about its species
				// in this sample, which is a different fact from telling us it
				// has none. A species is only KNOWN in this sample because some
				// world was in a position to count it.
				continue
			}
			for i := range known {
				known[i] = true
			}
			for _, e := range sv.Species {
				if at, ok := wanted[censusEntryKey(e)]; ok {
					total[at] += e.Bibites
				}
			}
		}
		for i := range ordered {
			accs[i][idx].n++
			if known[i] {
				accs[i][idx].sum += total[i]
				accs[i][idx].have++
			}
		}
	}

	for i, key := range ordered {
		tr := SpeciesTrend{Key: key, Points: make([]*int, buckets)}
		for b := 0; b < buckets; b++ {
			p := accs[i][b].point(fromMs + int64(b)*bucketMs)
			tr.Points[b] = p.Value
			if p.Value != nil {
				if *p.Value > tr.Max {
					tr.Max = *p.Value
				}
				v := *p.Value
				tr.Last = &v
			}
		}
		out.Species = append(out.Species, tr)
	}
	return out
}

// SpeciesTrendsView answers one /api/species/trends request. The KEY SET IS THE
// LIVING CENSUS UNION and nothing else — the same refusal the flat index makes:
// a species that died out is not drawn a trend line here, however far it
// travelled.
func (a *Archive) SpeciesTrendsView(window time.Duration, buckets int) (SpeciesTrends, error) {
	ck := "trends|" + window.String() + "|" + strconv.Itoa(buckets)
	a.speciesHistory.mu.Lock()
	defer a.speciesHistory.mu.Unlock()
	if a.speciesHistory.trends == nil {
		a.speciesHistory.trends = map[string]speciesTrendsEntry{}
	}
	if e, ok := a.speciesHistory.trends[ck]; ok && time.Since(e.at) < historyCacheFor {
		return e.val, nil
	}
	// StatusView takes the archive's lock itself and is finished with before the
	// file is read: the disk is never touched with that lock held.
	keys := aliveKeys(a.StatusView())
	samples, truncated, err := ReadMetricsTail(a.metrics.Path(), historyTailBytes)
	if err != nil {
		return SpeciesTrends{}, err
	}
	tr := BuildSpeciesTrends(samples, keys, time.Now().UnixMilli(), window, buckets)
	tr.Truncated = truncated
	if len(a.speciesHistory.trends) >= speciesHistoryCacheMax {
		oldest, oldestAt := "", time.Time{}
		for k, e := range a.speciesHistory.trends {
			if oldestAt.IsZero() || e.at.Before(oldestAt) {
				oldest, oldestAt = k, e.at
			}
		}
		delete(a.speciesHistory.trends, oldest)
	}
	a.speciesHistory.trends[ck] = speciesTrendsEntry{at: time.Now(), val: tr}
	return tr, nil
}

// aliveKeys is the living census union as comparison keys, in the census's own
// slot order. It is the same union SpeciesIndexView builds, expressed through
// the same key helper, so the trend column cannot describe a different set of
// species from the rows it sits beside.
func aliveKeys(view Status) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, sv := range view.Slots {
		if !sv.StatsKnown || !sv.SpeciesKnown {
			continue
		}
		for _, e := range sv.Species {
			key := censusEntryKey(e)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, key)
		}
	}
	return out
}

// censusEntryKey is the one place the archive turns a census entry into a
// comparison key, so the index, the history and any test cannot drift apart.
func censusEntryKey(e contractb.CensusEntry) string {
	return wire.SpeciesKey(e.GenericName, e.SpecificName)
}
