package archive

// The SPECIES VIEW: what is alive, when each kind was first and last seen
// crossing, and where their lines meet. It is ONE view now — it was two, a flat
// census list and a separate family tree, and they are merged here.
//
// WHY THEY MERGED. The two views listed the same species from the same census in
// two different orders, and a reader who wanted "how many of this one, and where
// did it come from" had to hold one tab in their head while reading the other.
// Worse, they made the same claim twice and could drift: two renderings of one
// alive union is two places for it to be wrong. The merged view puts the census
// facts ON the family — population, per-world counts, eggs and the three badges
// sit on the row whose bar says when that species was first recorded and which
// bar it descends from — and the flat ranking survives as a SORT of the same
// rows rather than as a second surface. /api/species is untouched and still
// serves the flat index: ringstat reads it, and a terminal table is a different
// medium with a different answer.
//
// THE DRAWING IS A TIME AXIS, which is the other half of the merge. Horizontal
// is wall-clock time, from the oldest thing the record dates to now; a species
// is a BAR from its first recorded crossing to its last, or to the right-hand
// edge while it is alive; a parent edge drops from the parent's bar to the
// child's at the child's first-seen point. That turns "who came from whom" and
// "when" into one picture, and it makes the record's own floor a line on it
// rather than a footnote.
//
// WHERE THE ANCESTRY COMES FROM, AND WHY NOTHING HERE IS A GUESS. A migration
// envelope carries an OPTIONAL species block, and that block has carried the
// migrant's PARENT SPECIES NAME since contract-a.md §16 A30 and
// contract-b-m4.md §15 B9 — `parentGenericName` and `parentSpecificName`, all
// or nothing, exactly one generation. The origin mod reads it off the live
// `Species.parentSpecies` record on the main thread in the same FixedUpdate as
// the migrant's serialization, so it is the game's own answer about its own
// registry, not an inference from genome distance or from anything this archive
// computed. B10 records it. species.go has kept it per species since the tab
// existed and SHOWED it, one generation deep, as a label.
//
// THIS FILE ADDS EXACTLY ONE IDEA: one generation per record, chained. If B's
// records name A as its parent and C's name B as its, then A → B → C is what
// the record says, and no step of it was resolved, merged or inferred here. The
// depth is not theoretical — the running rig's living species sit 39 to 40
// generations below a common ancestor, and every link of that is a crossing the
// ledger holds.
//
// WHAT IS STILL NOT A RESOLUTION, said once because §16 A31 is emphatic and
// right. The archive does not resolve a name against any registry; it cannot,
// because the registry is inside a game process. An edge here is a claim about
// WHAT THE RECORD SAYS, which is the only kind of claim this archive ever
// makes. Two worlds that spell one species differently are one node, under the
// A34 comparison key, for the reason rule 2 of species.go gives.
//
// THE PRUNING, AND WHY IT IS THE WHOLE POINT. The full historical tree is 2 400
// species on the running rig, almost all of them extinct, and drawing it would
// answer a question nobody asked. What a reader wants is the shape of what is
// ALIVE: the living species as leaves, joined at the ancestors where their
// lines actually part. So the tree is REDUCED — leaves are the census's living
// species, an ancestor is kept only when it is a real branching point (two or
// more living lineages descend from it by different children) or is itself
// alive, and every chain between two kept nodes COLLAPSES to one edge that
// says how many generations it swallowed. On the running rig that turns a
// 2 400-node history into a 15-node tree, and the 15 are the answer.
//
// FOUR RULES, each a decision this file records:
//
//  1. NO SCAN, EVER, AND NO NEW STATE THAT GROWS WITH THE RECORD. The edge is
//     one string per species, set by the same observeSpeciesLocked the tab has
//     always used — one call site for the replay, one for the live path, no
//     third view of the same fact (species.go rule 1). The state stays
//     O(species); the streamed replay holds no record after it has folded it.
//     This file reads the aggregate and never reads the ledger.
//
//     IT DOES TOUCH ONE FILE, ONCE PER GENOME HASH, and the exception is worth
//     naming because rule 1 used to say "no disk at all". The brain shape drawn
//     on each species is parsed from ONE stored blob per species — the latest
//     hash the record named, kept as one more string by the same single writer —
//     and the parse is cached on that hash for the life of the process, outside
//     every lock. So it is bounded by the drawn node set, not by the record, and
//     a poll after the first costs nothing. See brain.go.
//
//  2. THE REDUCTION IS SERVER-SIDE. Three reasons and they compound: the page
//     and ringstat render the SAME JSON, so one reduction in one place is what
//     stops a terminal tool and a browser disagreeing about who is related to
//     whom (§10.1); reducing on the client would mean shipping the whole 2 400-
//     node ledger graph to a browser so it could throw 99% of it away; and the
//     deny list is applied at the HTTP boundary (denylist.go), so a name must
//     be suppressible in the node set that is actually served — a client-side
//     reduction would have to be handed the suppressed ancestors to walk
//     through them.
//
//  3. EVERY HONEST SHAPE IS A SHAPE, NOT A FAILURE. A forest of several roots,
//     an alive species that is itself an ancestor, an alive species with no
//     recorded ancestry at all: all three occur on the running rig, all three
//     are drawn, and the last one is LABELLED rather than hidden. So is the
//     derivation's own limit — a lineage that never crossed a lane has no
//     record to carry a parent name, so it is not connected here, and the view
//     publishes how many of the living species that is.
//
//     A DRAWN ROOT IS NOT A CLAIM OF ORIGIN, and this is the shape that reads
//     as one. The reduction stops at the highest node with a living branch, so
//     a root very often has AncestryKnown true and AncestryDepth in the
//     dozens — `Zhiluus tardisitguyus` sits under 31 recorded generations, all
//     collapsed because not one of them has another living descendant. Above
//     THAT is the floor of the record itself (AncestrySinceMs): the parent
//     field arrived with §16 A30 and the ledger is older than it, so the top of
//     every chain is where the RECORD begins, never where the family did. The
//     view publishes both numbers and the page badges the root with them,
//     because the alternative a reader reaches for otherwise is an invented
//     edge down to the game's first species — and this file fabricates nothing.
//
//  4. A DERIVED EDGE IS GUARDED. The measured ledger holds no cycle and no
//     self-parent across 2 407 species, but the two names an edge is built from
//     are attacker-chosen strings normalized into a key, and "measured clean
//     today" is not an invariant. Every walk carries a visited set and a hard
//     depth cap, every node set a hard bound, and what the guards caught is
//     PUBLISHED rather than swallowed.

import (
	"sort"
	"time"
)

const (
	// treeWalkMax bounds ONE ancestor walk. The running rig's deepest living
	// lineage sits 40 generations down, so this is six times the observed shape;
	// past it the walk stops and the view says it was capped. It is a guard
	// against a cycle the visited set somehow misses and against a chain long
	// enough to be a denial of service by itself, and it is not a display
	// preference.
	treeWalkMax = 256
	// treeNodeMax bounds the REDUCED tree. Leaves are bounded already — the
	// census caps each world at 32 species and there are six worlds — and the
	// branch points between them cannot outnumber the leaves in a tree, so this
	// is unreachable by construction on any map this system describes. It exists
	// because "unreachable by construction" has a way of becoming reachable, and
	// a bounded answer that says it is bounded beats an unbounded one.
	treeNodeMax = 512
)

// TreeNode is one node of the reduced genealogy: a living species, or an
// ancestor kept because living lines part at it.
type TreeNode struct {
	// Key is the A34-normalized comparison key, and it is the node's id: Parent
	// and SpeciesTree.Roots hold keys.
	Key string `json:"key"`
	// Name is the raw spelling to display, and NameFrom says which side it came
	// from — which §16 B12 requires of anything that joins these two sources,
	// because the census's copy is raw (A36) and the ledger's is normalized at
	// the source (A34), so one species can legitimately read `Izus` here and
	// `Izus ` there.
	//
	//	"census" — this species is alive and the world holding it spells it
	//	this way, right now.
	//
	//	"record" — this species is not alive anywhere reporting, so no census
	//	names it. The spelling is the one its DESCENDANTS' migration records
	//	carried for their parent, which is the only spelling the archive has.
	Name     string `json:"name"`
	NameFrom string `json:"nameFrom"`

	Alive bool `json:"alive"`
	// Population, Eggs and Worlds are the census's, and are present only for a
	// living node. An ancestor has no population because it has none — that is
	// why it is an ancestor and not a leaf.
	//
	// WORLDS CARRIES THE COUNTS, not just the slots, because THIS VIEW IS THE
	// SPECIES LIST NOW. The flat census tab it replaced showed a chip per world
	// with that world's own bibites and eggs, and a merged view that dropped them
	// would be a merge that lost half of what it merged. It is also what the
	// per-species mini-map is filled from, against the grid published below.
	Population int            `json:"population,omitempty"`
	Eggs       int            `json:"eggs,omitempty"`
	Worlds     []SpeciesWorld `json:"worlds,omitempty"`
	// The rest of what the census tab showed about a living species, carried here
	// for the same reason: Spellings is every OTHER raw spelling alive under this
	// key right now (two worlds that write one species differently are one
	// species and two labels), and the three badges are §19 B19's.
	Spellings  []string `json:"spellings,omitempty"`
	Excluded   bool     `json:"excluded,omitempty"`
	ExcludedBy []int    `json:"excludedBy,omitempty"`
	Everywhere bool     `json:"everywhere,omitempty"`
	Endemic    bool     `json:"endemic,omitempty"`
	// Recent is the newest recorded crossings of this species: lane and age. A
	// SAMPLE, bounded by speciesRecentMax, never a count — the count is
	// Crossings.
	Recent []SpeciesCrossing `json:"recent,omitempty"`
	// ParentName is the RAW parent species name the record last carried for this
	// node, shown and never resolved (contract-a.md §16, A31). It is not the same
	// thing as Parent below: Parent is an edge in the REDUCED tree and is empty
	// on a root, while this is what the record says about the generation
	// immediately above — which is exactly the fact a collapsed edge or a root
	// hides.
	ParentName string `json:"parentName,omitempty"`

	// Parent is the key of this node's parent IN THE REDUCED TREE, "" for a
	// root, and Collapsed is how many generations the edge swallowed — 0 for a
	// direct parent-child link, 7 for a chain of seven extinct intermediates
	// with no living branch among them. It is published because a reader
	// deserves to know the difference between a sibling and a distant cousin,
	// and because the collapse is the one place this view discards something the
	// record holds.
	Parent    string `json:"parent,omitempty"`
	Collapsed int    `json:"collapsed,omitempty"`
	Depth     int    `json:"depth"`
	// Leaves is how many LIVING species sit in this node's reduced subtree,
	// itself included when it is alive. For a leaf it is 1; for an ancestor it
	// is what makes "a branching point" a number a reader can check.
	Leaves int `json:"leaves"`
	// Root marks a node with no kept ancestor, and Isolated the honest shape
	// rule 3 names: a living species that is a root AND has no living relative
	// in this tree, so the reduction connects it to nothing. It is drawn and
	// labelled, never dropped.
	Root     bool `json:"root,omitempty"`
	Isolated bool `json:"isolated,omitempty"`
	// AncestryKnown and AncestryDepth are how far the RECORD reaches above this
	// node, before any reduction — and they are what keeps the page's label
	// honest, because TWO DIFFERENT FACTS BOTH PRODUCE AN ISOLATED LEAF and
	// calling them the same thing would be the derivation lying about itself:
	//
	//	AncestryKnown false — no crossing of this species ever carried a parent
	//	name. The record genuinely does not know where it came from. This is the
	//	"no recorded ancestry" case, and on the running rig it is `Basic bibite`.
	//
	//	AncestryKnown true, and still isolated — the record traces it back
	//	AncestryDepth generations, and not one of those ancestors has another
	//	living descendant on this map. Its family is real and its family is
	//	extinct. Printing "no recorded ancestry" over that would discard the one
	//	thing the record does know.
	AncestryKnown bool `json:"ancestryKnown"`
	AncestryDepth int  `json:"ancestryDepth"`

	// The ledger's annotation, the same numbers the index carries, for whichever
	// nodes have them. An ancestor that never crossed a lane has zeroes here and
	// they are counts, not gaps: this archive recorded no crossing by it.
	Crossings int   `json:"crossings"`
	FirstMs   int64 `json:"firstMs,omitempty"`
	LastMs    int64 `json:"lastMs,omitempty"`
	Genomes   int   `json:"genomes,omitempty"`

	// ---- THE LIFESPAN BAR, which is what this view draws on a time axis.
	//
	// SpanFromMs and SpanToMs are the two ends of one node's bar in wall-clock
	// milliseconds, and they are DERIVED HERE rather than on the page for tree.go
	// rule 2's reason: the page and any terminal renderer must not disagree about
	// where a bar begins.
	//
	//	SpanFromMs is this node's FIRST RECORDED CROSSING. It is not a birth date
	//	and this view never calls it one: it is the first time this archive was
	//	copied on a creature of this species leaving a world.
	//
	//	SpanToMs is 0 for a LIVING species, which the renderer draws as a bar
	//	running to the right-hand edge — to now. For a species alive nowhere that
	//	is reporting, it is the LAST recorded crossing, and the bar stops there.
	//
	//	SpanDerived marks a node whose own record holds no crossing at all — an
	//	ancestor named only as some child's parent. Its bar begins at its EARLIEST
	//	DATED DESCENDANT, which is a bound the record genuinely supports (that
	//	child's crossing named it, so it existed by then) and is drawn differently
	//	because it is an inference and not a reading.
	SpanFromMs  int64 `json:"spanFromMs,omitempty"`
	SpanToMs    int64 `json:"spanToMs,omitempty"`
	SpanDerived bool  `json:"spanDerived,omitempty"`

	// ---- THE BRAIN, from ONE stored genome per species (brain.go).
	//
	// Neurons and Synapses are the shape of the LATEST genome of this species the
	// crossing record named, as the content-addressed store holds it. Both are
	// ABSENT when the store does not hold that blob — pruned past the retention
	// horizon, never fetched, or a shape this parser cannot read — and absent
	// renders as nothing at all. It is never an error and never a zero: a brain
	// this archive cannot see is not a brain of no neurons.
	Neurons  int `json:"neurons,omitempty"`
	Synapses int `json:"synapses,omitempty"`
}

// TreeCell is one seat on the map, for the per-species mini-map beside each
// living row. The grid is published because the SHAPE OF THE MAP IS NOT A
// CONSTANT: this rig is 3×2 and the next one is not, and a renderer that drew
// six dots in two rows because this one has six worlds would be wrong the first
// time a seat is added.
type TreeCell struct {
	Slot int `json:"slot"`
	Col  int `json:"col"`
	Row  int `json:"row"`
	// Reporting is whether this world contributed a census to the union above. A
	// world that is live but silent is UNKNOWN in every dot of its column, never
	// an absence of the species (§10.1), and the renderer marks it as such.
	Reporting bool `json:"reporting"`
	Live      bool `json:"live"`
}

// TreeMap is the seats the mini-map is drawn on. A (col,row) with no cell is a
// HOLE — a square inside the map no world has claimed — and is drawn as nothing.
type TreeMap struct {
	Width  int        `json:"width"`
	Height int        `json:"height"`
	Cells  []TreeCell `json:"cells"`
}

// SpeciesTree is /api/species/tree: the reduced genealogy of everything alive.
type SpeciesTree struct {
	GeneratedAtMs int64 `json:"generatedAtMs"`
	// HaveStatus and StatusAgeMs are §10.1's freshness rule, inherited from the
	// index this view is built on: a tree whose leaves came from a frame the
	// archive cannot date is not state, and the page says so instead of drawing.
	HaveStatus      bool  `json:"haveStatus"`
	StatusAgeMs     int64 `json:"statusAgeMs"`
	ReportingSlots  int   `json:"reportingSlots"`
	CensuslessSlots int   `json:"censuslessSlots"`
	TruncatedSlots  int   `json:"truncatedSlots"`

	// Alive is how many living species the census offered as leaves. Connected
	// is how many of them the record joins to at least one other; Isolated is
	// the rest, and Unrecorded is the subset of those for which the record holds
	// no ancestry at all. THESE ARE PUBLISHED SEPARATELY AND THAT IS THE POINT:
	// the derivation's limit is a number, not a disclaimer, and a reader can see
	// exactly how much of the map it covers and why the rest is out.
	Alive      int `json:"alive"`
	Connected  int `json:"connected"`
	Isolated   int `json:"isolated"`
	Unrecorded int `json:"unrecorded"`
	// Ancestors is how many non-living nodes were kept as branch points, and
	// Collapsed the total generations every edge swallowed between them.
	Ancestors int `json:"ancestors"`
	Collapsed int `json:"collapsed"`

	// LedgerSpecies is every species the crossing record holds and LedgerEdges
	// how many of those carry a parent name at all. The ratio is the honest
	// answer to "how much ancestry does this archive have", and it is a much
	// bigger tree than the one below — because below is only what is alive.
	LedgerSpecies int `json:"ledgerSpecies"`
	LedgerEdges   int `json:"ledgerEdges"`
	// AncestrySinceMs is the derivation's FLOOR: the recordedAt of the earliest
	// crossing this archive holds that named a parent species at all
	// (species.go's edgeFirstMs). It is the other half of a root's badge — the
	// badge says the record reaches N generations above this node and stops, and
	// this says WHEN the record it stops at begins. 0 when no record has ever
	// named a parent, which is an absent caption and never a date.
	AncestrySinceMs int64 `json:"ancestrySinceMs,omitempty"`

	// ---- THE TIME AXIS every bar is drawn against.
	//
	// SpanStartMs is the left-hand edge: the earliest thing this drawing has to
	// hold, which is the oldest bar start OR the record's own ancestry floor,
	// whichever is older — so the floor is always INSIDE the picture and can be
	// drawn as the boundary it is. SpanEndMs is now.
	//
	// They are published rather than left to the renderer to infer, because a
	// renderer inferring them from the nodes alone would put the floor off the
	// left edge on every map whose oldest bar is younger than the floor.
	SpanStartMs int64 `json:"spanStartMs,omitempty"`
	SpanEndMs   int64 `json:"spanEndMs,omitempty"`

	// Map is the grid the per-species mini-maps are drawn on. It is the census's
	// own seats, from the same status frame the leaves came from.
	Map TreeMap `json:"map"`

	// LedgerOverflow is how many species the aggregate refused to track after
	// speciesAggMax, carried here because THIS VIEW IS THE SPECIES LIST NOW and a
	// truncated aggregate a reader cannot see is a wrong answer.
	LedgerOverflow int `json:"ledgerOverflow,omitempty"`
	// LedgerRecords is how many records the whole ledger holds, the same number
	// the flat index published.
	LedgerRecords int `json:"ledgerRecords"`

	// The guards of rule 4, published rather than swallowed. MaxDepth is the
	// deepest walk performed, WalkCapped how many hit treeWalkMax, CycleGuard
	// how many met a node twice, and NodesCapped whether the reduced tree hit
	// treeNodeMax.
	MaxDepth    int  `json:"maxDepth"`
	WalkCapped  int  `json:"walkCapped,omitempty"`
	CycleGuard  int  `json:"cycleGuard,omitempty"`
	NodesCapped bool `json:"nodesCapped,omitempty"`

	// Roots are the forest's roots in draw order, and Nodes is every node in
	// DFS pre-order from those roots — so a renderer can lay the tree out in one
	// pass and two polls produce the same order.
	Roots []string   `json:"roots"`
	Nodes []TreeNode `json:"nodes"`
}

// treeFacts is what one walk copies out from under the lock about one node.
type treeFacts struct {
	crossings int
	firstMs   int64
	lastMs    int64
	genomes   int
	// raw is a spelling for a node the census does not name: the one its
	// descendants' records carried. "" when only the census names it.
	raw string
	// parent is this node's own recorded parent name, raw, and genomeHash the
	// latest genome any crossing of it carried. Both are copies of one string
	// each, taken under the lock and read after it.
	parent     string
	genomeHash string
}

// SpeciesTreeView builds the reduced genealogy. It takes the archive's lock
// once, briefly, and touches no disk.
//
// COST. Maintenance is O(1) per recorded migration, in species.go, inside a
// lock already held. This view is O(A·D) map lookups for the walks plus O(N log N)
// to order the result, where A is the living species (bounded by the census: 32
// a world), D the ancestor depth (bounded by treeWalkMax) and N the reduced
// nodes (bounded by treeNodeMax, and by A in any real tree). On the running rig
// that is twelve walks of forty steps. Plus, on the first poll after a species'
// genome hash moves, one small file read and one JSON decode per such species —
// never on a later poll, never per request, and never for a hash the store does
// not hold. Nothing here is proportional to the ledger, which is the property
// rule 1 exists to keep.
func (a *Archive) SpeciesTreeView() SpeciesTree {
	// ONE status frame, and the index derived from it. Both take the lock
	// themselves and are finished with before the walk takes it again — two short
	// holds, never a nested one — and reusing the index rather than re-deriving
	// the alive union is rule 2's discipline applied to this file: there is ONE
	// statement of which species are alive, and now also one statement of which
	// map they are alive on.
	status := a.StatusView()
	idx := a.speciesIndexFrom(status)
	nowMs := time.Now().UnixMilli()

	out := SpeciesTree{
		GeneratedAtMs:   nowMs,
		HaveStatus:      idx.HaveStatus,
		StatusAgeMs:     idx.StatusAgeMs,
		ReportingSlots:  idx.ReportingSlots,
		CensuslessSlots: idx.CensuslessSlots,
		TruncatedSlots:  idx.TruncatedSlots,
		LedgerSpecies:   idx.LedgerSpecies,
		LedgerOverflow:  idx.LedgerOverflow,
		LedgerRecords:   idx.LedgerRecords,
		Alive:           len(idx.Species),
		SpanEndMs:       nowMs,
		Roots:           []string{},
		Nodes:           []TreeNode{},
		Map:             TreeMap{Cells: []TreeCell{}},
	}
	// The grid the mini-maps are drawn on, from the SAME status frame the leaves
	// came from — never from a second read, which could describe a map the rows
	// above it do not belong to.
	if status.HaveStatus {
		out.Map.Width, out.Map.Height = status.Map.Width, status.Map.Height
		for _, sv := range status.Slots {
			out.Map.Cells = append(out.Map.Cells, TreeCell{
				Slot: sv.Slot, Col: sv.Position.Col, Row: sv.Position.Row,
				Reporting: sv.StatsKnown && sv.SpeciesKnown, Live: sv.Live,
			})
		}
	}

	// aliveOrder is the index's own order — population descending, key ascending
	// — which becomes the tie-break for everything below, so the drawn tree is
	// stable between polls.
	aliveOrder := make([]string, 0, len(idx.Species))
	aliveRow := make(map[string]*SpeciesRow, len(idx.Species))
	for i := range idx.Species {
		row := &idx.Species[i]
		aliveOrder = append(aliveOrder, row.Key)
		aliveRow[row.Key] = row
	}

	// ---- the walk, under the lock, as a copy.
	//
	// parentOf is the FULL graph the living set reaches, before any reduction:
	// every ancestor of every living species, however extinct. It is bounded by
	// A·treeWalkMax and is thrown away at the end of this function.
	parentOf := map[string]string{}
	facts := map[string]*treeFacts{}
	a.mu.Lock()
	out.LedgerSpecies = len(a.species.byKey)
	// All three are maintained counters, not walks: nothing in this function is
	// proportional to the size of the aggregate, let alone to the ledger.
	out.LedgerEdges = a.species.edges
	out.AncestrySinceMs = a.species.edgeFirstMs
	for _, key := range aliveOrder {
		// visited is PER WALK and is rule 4's real guard: a cycle among extinct
		// ancestors is met on the second visit and the walk stops there, keeping
		// every edge it had already established. Cutting the whole lineage
		// because its far end is malformed would lose good edges to a bad one.
		visited := map[string]bool{key: true}
		cur, depth := key, 0
		for {
			e := a.species.byKey[cur]
			if e == nil {
				break
			}
			// The ledger facts are assigned on EVERY visit rather than only when
			// the entry is new, and the difference is not cosmetic: a node is
			// very often created as some child's PARENT first — a stub carrying
			// only that child's spelling of it — and a `if facts[cur] == nil`
			// here would then leave a well-travelled ancestor reporting zero
			// crossings forever. The assignment is idempotent, so the cheap
			// correct thing and the cheap wrong thing cost the same.
			f := facts[cur]
			if f == nil {
				f = &treeFacts{}
				facts[cur] = f
			}
			f.crossings, f.firstMs, f.lastMs, f.genomes =
				e.crossings, e.firstMs, e.lastMs, len(e.genomes)
			f.parent, f.genomeHash = e.parent, e.genomeHash
			p := e.parentKey
			if p == "" {
				break
			}
			// The parent's own raw spelling, from THIS record — the only place a
			// node the census does not name can get a label (NameFrom "record").
			// First writer wins, and the walks run in the index's order, so the
			// spelling shown is the one the most populous living descendant's
			// record carried.
			if facts[p] == nil {
				facts[p] = &treeFacts{raw: e.parent}
			} else if facts[p].raw == "" {
				facts[p].raw = e.parent
			}
			if visited[p] {
				out.CycleGuard++
				break
			}
			if depth >= treeWalkMax {
				out.WalkCapped++
				break
			}
			visited[p] = true
			parentOf[cur] = p
			cur = p
			depth++
			if depth > out.MaxDepth {
				out.MaxDepth = depth
			}
		}
	}
	a.mu.Unlock()

	// ---- the reduction.
	//
	// childOn[p] is the set of DISTINCT children through which living lineages
	// reach p. Two is what makes p a branching point: one means p is a link in a
	// chain and collapses away, however many living species hang below it.
	childOn := map[string]map[string]bool{}
	for child, parent := range parentOf {
		if childOn[parent] == nil {
			childOn[parent] = map[string]bool{}
		}
		childOn[parent][child] = true
	}
	keep := map[string]bool{}
	for _, key := range aliveOrder {
		// A LIVING SPECIES IS ALWAYS KEPT, leaf or not. Todae willlaytonius on
		// the running rig is one bibite in one world AND the parent of three
		// living species: dropping it for being a chain link would delete the
		// most interesting node on the map.
		keep[key] = true
	}
	for p, kids := range childOn {
		if len(kids) >= 2 {
			keep[p] = true
		}
	}

	// reducedParent walks up to the nearest KEPT strict ancestor, counting what
	// it skipped. The visited set is the same guard again: this walk runs over
	// the graph the first one built, and a guard that only ran once would be a
	// guard that protected the cheaper loop.
	reducedParent := func(k string) (string, int) {
		visited := map[string]bool{k: true}
		skipped := 0
		cur := parentOf[k]
		for cur != "" && !visited[cur] {
			if keep[cur] {
				return cur, skipped
			}
			visited[cur] = true
			skipped++
			if skipped > treeWalkMax {
				return "", skipped
			}
			cur = parentOf[cur]
		}
		return "", skipped
	}

	type built struct {
		node TreeNode
		kids []string
	}
	nodes := map[string]*built{}
	// Ordered keys: the living ones in the index's order first, then the kept
	// ancestors by key, so the whole build is deterministic.
	ordered := make([]string, 0, len(keep))
	ordered = append(ordered, aliveOrder...)
	extra := make([]string, 0, len(keep))
	for k := range keep {
		if aliveRow[k] == nil {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	ordered = append(ordered, extra...)
	if len(ordered) > treeNodeMax {
		out.NodesCapped = true
		ordered = ordered[:treeNodeMax]
		newKeep := map[string]bool{}
		for _, k := range ordered {
			newKeep[k] = true
		}
		keep = newKeep
	}

	for _, k := range ordered {
		f := facts[k]
		n := TreeNode{Key: k}
		if row := aliveRow[k]; row != nil {
			n.Alive = true
			// A LIVING NODE'S LABEL IS THE CENSUS'S, raw, exactly as the index
			// shows it — so a species reads the same on every view.
			n.Name, n.NameFrom = row.Name, "census"
			n.Population, n.Eggs = row.Population, row.Eggs
			// THE WHOLE CENSUS ROW, because this view is the species list now.
			n.Worlds = append([]SpeciesWorld(nil), row.Worlds...)
			n.Spellings = row.Spellings
			n.Excluded, n.ExcludedBy = row.Excluded, row.ExcludedBy
			n.Everywhere, n.Endemic = row.Everywhere, row.Endemic
			n.Recent = row.Recent
			n.ParentName = row.Parent
			n.Crossings, n.FirstMs, n.LastMs = row.Crossings, row.FirstMs, row.LastMs
			n.Genomes = row.Genomes
		} else {
			// A NODE NO CENSUS NAMES. Its label is what its descendants' records
			// carried, and NameFrom says so; when even that is missing the
			// comparison key is all there is, and it is shown as itself rather
			// than dressed up as a spelling some world holds.
			n.NameFrom = "record"
			if f != nil && f.raw != "" {
				n.Name = f.raw
			} else {
				n.Name = k
			}
			if f != nil {
				n.Crossings, n.FirstMs, n.LastMs = f.crossings, f.firstMs, f.lastMs
				n.Genomes = f.genomes
				n.ParentName = f.parent
			}
		}
		nodes[k] = &built{node: n}
	}
	// chainLen is how many generations the RECORD reaches above a node, over the
	// unreduced graph, memoized so the deep shared trunk is walked once. It
	// carries the same visited guard for the same reason as every other walk
	// here.
	chainMemo := map[string]int{}
	var chainLen func(k string, seen map[string]bool) int
	chainLen = func(k string, seen map[string]bool) int {
		if n, ok := chainMemo[k]; ok {
			return n
		}
		p := parentOf[k]
		if p == "" || seen[p] || len(seen) > treeWalkMax {
			chainMemo[k] = 0
			return 0
		}
		seen[p] = true
		n := 1 + chainLen(p, seen)
		chainMemo[k] = n
		return n
	}
	for _, k := range ordered {
		nodes[k].node.AncestryKnown = parentOf[k] != ""
		nodes[k].node.AncestryDepth = chainLen(k, map[string]bool{k: true})
	}

	for _, k := range ordered {
		p, skipped := reducedParent(k)
		if p == "" || nodes[p] == nil {
			continue
		}
		nodes[k].node.Parent = p
		nodes[k].node.Collapsed = skipped
		out.Collapsed += skipped
		nodes[p].kids = append(nodes[p].kids, k)
	}

	// ---- roots, depth, subtree sizes, draw order.
	roots := make([]string, 0, 4)
	for _, k := range ordered {
		if nodes[k].node.Parent == "" {
			nodes[k].node.Root = true
			roots = append(roots, k)
		}
	}

	// Leaves is computed bottom-up. The recursion is bounded by the node set and
	// carries its own visited set, because a reduced tree built from guarded
	// walks is a tree in every measured case and this is the one place an
	// unguarded assumption would be an infinite loop rather than a wrong number.
	var leavesOf func(k string, seen map[string]bool) int
	leavesOf = func(k string, seen map[string]bool) int {
		if seen[k] {
			return 0
		}
		seen[k] = true
		n := nodes[k]
		total := 0
		if n.node.Alive {
			total = 1
		}
		for _, c := range n.kids {
			total += leavesOf(c, seen)
		}
		n.node.Leaves = total
		return total
	}
	for _, r := range roots {
		leavesOf(r, map[string]bool{})
	}

	// Children are drawn most-living-lineages first, then most populous, then by
	// key: the busiest branch reads down the left of the tree and the order does
	// not move between polls.
	for _, k := range ordered {
		kids := nodes[k].kids
		sort.SliceStable(kids, func(i, j int) bool {
			ni, nj := nodes[kids[i]].node, nodes[kids[j]].node
			if ni.Leaves != nj.Leaves {
				return ni.Leaves > nj.Leaves
			}
			if ni.Population != nj.Population {
				return ni.Population > nj.Population
			}
			return ni.Key < nj.Key
		})
	}
	sort.SliceStable(roots, func(i, j int) bool {
		ni, nj := nodes[roots[i]].node, nodes[roots[j]].node
		if ni.Leaves != nj.Leaves {
			return ni.Leaves > nj.Leaves
		}
		if ni.Population != nj.Population {
			return ni.Population > nj.Population
		}
		return ni.Key < nj.Key
	})

	// An ISOLATED node is a living root with no children: the record connects it
	// to nothing at all. It is a shape, not a failure — a species whose lineage
	// has never crossed a lane has no record that could carry a parent name.
	for _, r := range roots {
		n := nodes[r]
		if n.node.Alive && len(n.kids) == 0 {
			n.node.Isolated = true
		}
	}

	// DFS pre-order from the sorted roots, assigning depth on the way down.
	var emit func(k string, depth int, seen map[string]bool)
	emit = func(k string, depth int, seen map[string]bool) {
		if seen[k] {
			return
		}
		seen[k] = true
		n := nodes[k]
		n.node.Depth = depth
		out.Nodes = append(out.Nodes, n.node)
		for _, c := range n.kids {
			emit(c, depth+1, seen)
		}
	}
	seen := map[string]bool{}
	for _, r := range roots {
		out.Roots = append(out.Roots, r)
		emit(r, 0, seen)
	}
	// Any node the DFS did not reach would be one a guard cut loose from every
	// root. It is emitted as its own root rather than dropped: a node this view
	// built and then failed to place is exactly the thing a reader must be able
	// to see.
	for _, k := range ordered {
		if seen[k] {
			continue
		}
		n := nodes[k]
		n.node.Parent, n.node.Collapsed, n.node.Root = "", 0, true
		if n.node.Leaves == 0 && n.node.Alive {
			n.node.Leaves = 1
		}
		out.Roots = append(out.Roots, k)
		emit(k, 0, seen)
	}

	// ---- THE LIFESPAN BARS, in one backwards pass.
	//
	// out.Nodes is DFS pre-order, so EVERY CHILD SITS AFTER ITS PARENT and a walk
	// from the end has already seen a node's whole subtree by the time it reaches
	// the node. That is what lets an undated ancestor inherit its earliest dated
	// descendant's start without a recursion, without a memo and without a
	// visited set — the ordering the emit above produced is the guard.
	childFrom := make(map[string]int64, len(out.Nodes))
	for i := len(out.Nodes) - 1; i >= 0; i-- {
		n := &out.Nodes[i]
		switch {
		case n.FirstMs > 0:
			// THE RECORD'S OWN ANSWER, always preferred: this node crossed a lane
			// and the archive was copied on it.
			n.SpanFromMs = n.FirstMs
		case childFrom[n.Key] > 0:
			// A node no crossing of its own ever dated. Its bar begins where its
			// earliest dated descendant does, because that descendant's crossing
			// NAMED IT as a parent — so the record does support "it existed by
			// then", and supports nothing earlier.
			n.SpanFromMs, n.SpanDerived = childFrom[n.Key], true
		}
		// A LIVING SPECIES HAS NO RIGHT-HAND END. 0 says so, and the renderer
		// draws to the edge of the picture rather than to its last crossing —
		// which would say it stopped when it merely stopped travelling.
		if !n.Alive {
			if n.LastMs > n.SpanFromMs {
				n.SpanToMs = n.LastMs
			} else {
				n.SpanToMs = n.SpanFromMs
			}
		}
		if n.SpanFromMs > 0 && n.Parent != "" {
			if cur, ok := childFrom[n.Parent]; !ok || n.SpanFromMs < cur {
				childFrom[n.Parent] = n.SpanFromMs
			}
		}
		if n.SpanFromMs > 0 && (out.SpanStartMs == 0 || n.SpanFromMs < out.SpanStartMs) {
			out.SpanStartMs = n.SpanFromMs
		}
	}
	// THE FLOOR IS ALWAYS INSIDE THE PICTURE. It is where the record's ancestry
	// begins, and a drawing that put it off the left edge would leave every root
	// badge pointing at something the reader cannot see.
	if out.AncestrySinceMs > 0 &&
		(out.SpanStartMs == 0 || out.AncestrySinceMs < out.SpanStartMs) {
		out.SpanStartMs = out.AncestrySinceMs
	}
	if out.SpanStartMs == 0 || out.SpanStartMs >= out.SpanEndMs {
		// Nothing here is dated, or every date is in the future — which is a
		// clock, not a genealogy. An hour of axis is drawn rather than a degenerate
		// one, and every bar in it is absent anyway.
		out.SpanStartMs = out.SpanEndMs - int64(time.Hour/time.Millisecond)
	}

	// ---- THE BRAIN, one stored blob per species, parsed once per hash ever.
	// This is the only part of this view that can touch the disk, it does so
	// outside every lock, and an absence is an absence (brain.go).
	for i := range out.Nodes {
		f := facts[out.Nodes[i].Key]
		if f == nil || f.genomeHash == "" {
			continue
		}
		if b, ok := a.brainFor(f.genomeHash); ok {
			out.Nodes[i].Neurons, out.Nodes[i].Synapses = b.Neurons, b.Synapses
		}
	}

	for i := range out.Nodes {
		n := out.Nodes[i]
		if !n.Alive {
			out.Ancestors++
			continue
		}
		if n.Isolated {
			out.Isolated++
			if !n.AncestryKnown {
				out.Unrecorded++
			}
		} else {
			out.Connected++
		}
	}
	return out
}
