package archive

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/wire"
)

// child builds one ledger record whose species block names a PARENT species —
// the field contract-a.md §16 A30 added and contract-b-m4.md §15 B9 carries, and
// the only source of every edge in the genealogy.
func child(atMs int64, generic, specific, pGeneric, pSpecific string) Record {
	rec := migration(atMs, 1, 2, "E", generic, specific, "h-"+generic+specific)
	rec.MigrationID = "m-" + generic + specific + "-" + wire.NormalizeSpeciesName(pGeneric+pSpecific)
	rec.Species.ParentGenericName = pGeneric
	rec.Species.ParentSpecificName = pSpecific
	return rec
}

// treeNodes indexes a view by key, for assertions that read like sentences.
func treeNodes(t *testing.T, view SpeciesTree) map[string]TreeNode {
	t.Helper()
	byKey := map[string]TreeNode{}
	for _, n := range view.Nodes {
		if _, dup := byKey[n.Key]; dup {
			t.Fatalf("the tree emits %q twice; a node placed under two parents is not a tree", n.Key)
		}
		byKey[n.Key] = n
	}
	return byKey
}

// TestTheGenealogyJoinsLivingLineagesAtTheirBranchPoint is the whole derivation
// on the shape it exists for, and it is the shape the running rig actually has.
//
// THE FIXTURE IS A FAMILY. `Alpha nullus` founded `Beta` and `Gamma`; both are
// alive and Alpha is extinct everywhere. A view that listed only what is alive
// could never say those two are siblings, and a view that drew the whole
// historical tree would bury the fact in two thousand extinct nodes. The answer
// is Alpha KEPT — not as a resident, which it is not, but as the point where two
// living lines part.
func TestTheGenealogyJoinsLivingLineagesAtTheirBranchPoint(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{
			slot(1, 0, 0, true, census(30, 0, entry("Beta", "one", 20, 0), entry("Gamma", "two", 10, 0))),
			slot(2, 1, 0, true, census(5, 0, entry("Beta", "one", 5, 0))),
		},
	}
	a := newViewFixture(t, status, time.Second)

	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	a.observeSpeciesLocked(child(base, "Beta", "one", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+1000, "Gamma", "two", "Alpha", "nullus"))
	a.mu.Unlock()

	view := a.SpeciesTreeView()
	byKey := treeNodes(t, view)

	if len(view.Nodes) != 3 {
		t.Fatalf("the tree holds %d nodes, want 3 (two living leaves and the ancestor they "+
			"part at): %+v", len(view.Nodes), view.Nodes)
	}
	if len(view.Roots) != 1 || view.Roots[0] != "Alpha nullus" {
		t.Fatalf("roots = %v, want the one branch point", view.Roots)
	}
	alpha, ok := byKey["Alpha nullus"]
	if !ok {
		t.Fatal("the ancestor two living lineages part at is not in the tree; without it the " +
			"view cannot say the two are related at all")
	}
	// THE ANCESTOR IS NOT CLAIMED TO BE ALIVE, which is the one mistake this
	// whole family of views is built to avoid (D11, §10.1).
	if alpha.Alive || alpha.Population != 0 {
		t.Fatalf("an extinct ancestor is drawn as a resident: %+v", alpha)
	}
	if alpha.Leaves != 2 {
		t.Fatalf("Alpha.leaves = %d, want 2 — the number is what makes 'a branching point' "+
			"checkable", alpha.Leaves)
	}
	// Its label came from the RECORD, not from a census, and the view says so —
	// §16 B12's rule for anything that joins these two sources.
	if alpha.NameFrom != "record" || alpha.Name != "Alpha nullus" {
		t.Fatalf("the ancestor's label or its provenance is wrong: %+v", alpha)
	}
	// AND IT KEEPS ITS OWN LEDGER ANNOTATION. An ancestor is very often met first
	// as some child's parent, and a node built from that stub alone would report
	// zero crossings for a species that has crossed plenty.
	a.mu.Lock()
	a.observeSpeciesLocked(child(base+2000, "Alpha", "nullus", "Older", "still"))
	a.mu.Unlock()
	alpha = treeNodes(t, a.SpeciesTreeView())["Alpha nullus"]
	if alpha.Crossings != 1 || alpha.Genomes != 1 || alpha.FirstMs == 0 {
		t.Fatalf("the ancestor lost its own crossing record to the stub its children "+
			"created: %+v", alpha)
	}
	if !alpha.AncestryKnown || alpha.AncestryDepth != 1 {
		t.Fatalf("the ancestor's own recorded ancestry did not reach it: %+v", alpha)
	}
	for _, key := range []string{"Beta one", "Gamma two"} {
		n := byKey[key]
		if !n.Alive || n.Parent != "Alpha nullus" || n.Collapsed != 0 || n.Leaves != 1 {
			t.Fatalf("%s did not land under the branch point as a direct child: %+v", key, n)
		}
		if n.NameFrom != "census" {
			t.Fatalf("%s's label should be the census's raw spelling: %+v", key, n)
		}
	}
	if byKey["Beta one"].Population != 25 {
		t.Fatalf("the leaf carries %d alive, want the census union's 25",
			byKey["Beta one"].Population)
	}
	// The published counts are the derivation's own account of itself.
	if view.Alive != 2 || view.Connected != 2 || view.Isolated != 0 || view.Ancestors != 1 {
		t.Fatalf("counts: alive=%d connected=%d isolated=%d ancestors=%d",
			view.Alive, view.Connected, view.Isolated, view.Ancestors)
	}
}

// TestALivingSpeciesIsKeptEvenWhenItIsAlsoAnAncestor pins the shape that would be
// lost by any rule phrased as "leaves are alive, internals are dead".
//
// It is not hypothetical: on the running rig `Todae willlaytonius` is ONE bibite
// in ONE world and the parent of three living species. It is simultaneously the
// least abundant thing on the map and the most informative node on it.
func TestALivingSpeciesIsKeptEvenWhenItIsAlsoAnAncestor(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(60, 0,
			entry("Todae", "willlaytonius", 1, 0),
			entry("Todae", "lisebus", 30, 0),
			entry("Todae", "qignus", 29, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	a.observeSpeciesLocked(child(base, "Todae", "lisebus", "Todae", "willlaytonius"))
	a.observeSpeciesLocked(child(base+1, "Todae", "qignus", "Todae", "willlaytonius"))
	a.mu.Unlock()

	view := a.SpeciesTreeView()
	byKey := treeNodes(t, view)
	root := byKey["Todae willlaytonius"]
	if !root.Alive || !root.Root {
		t.Fatalf("the living ancestor is not the living root it is: %+v", root)
	}
	if root.Leaves != 3 {
		t.Fatalf("a living ancestor's leaves = %d, want 3 — itself and its two living "+
			"children", root.Leaves)
	}
	if root.Isolated {
		t.Fatal("a living root with living children is marked isolated")
	}
	if view.Connected != 3 || view.Isolated != 0 || view.Ancestors != 0 {
		t.Fatalf("counts: connected=%d isolated=%d ancestors=%d — every node here is alive",
			view.Connected, view.Isolated, view.Ancestors)
	}
}

// TestAChainOfExtinctAncestorsCollapsesToOneEdge is the pruning's other half, and
// the reason the running rig's 40-generation lineages are readable at all.
//
// Between two kept nodes the record may hold any number of extinct intermediates
// with no living branch among them. Drawing them would be forty nodes of nothing;
// dropping them silently would make a distant cousin look like a sibling. So the
// chain becomes ONE EDGE THAT SAYS HOW MANY GENERATIONS IT SWALLOWED.
func TestAChainOfExtinctAncestorsCollapsesToOneEdge(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(20, 0,
			entry("Leaf", "near", 10, 0), entry("Leaf", "far", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	// Root -> mid1 -> mid2 -> mid3 -> Leaf far, and Root -> Leaf near. Only Root
	// is a branch point; the three mids are a chain.
	a.observeSpeciesLocked(child(base, "Leaf", "near", "Root", "ancestor"))
	a.observeSpeciesLocked(child(base+1, "Mid", "one", "Root", "ancestor"))
	a.observeSpeciesLocked(child(base+2, "Mid", "two", "Mid", "one"))
	a.observeSpeciesLocked(child(base+3, "Mid", "three", "Mid", "two"))
	a.observeSpeciesLocked(child(base+4, "Leaf", "far", "Mid", "three"))
	a.mu.Unlock()

	view := a.SpeciesTreeView()
	byKey := treeNodes(t, view)
	if len(view.Nodes) != 3 {
		keys := make([]string, 0, len(view.Nodes))
		for _, n := range view.Nodes {
			keys = append(keys, n.Key)
		}
		t.Fatalf("the tree holds %d nodes %v, want 3: a chain with no living branch on it "+
			"is one edge", len(view.Nodes), keys)
	}
	far := byKey["Leaf far"]
	if far.Parent != "Root ancestor" {
		t.Fatalf("the collapsed edge does not reach the branch point: %+v", far)
	}
	if far.Collapsed != 3 {
		t.Fatalf("collapsed = %d, want 3 — the count is the difference between a sibling "+
			"and a distant cousin", far.Collapsed)
	}
	if byKey["Leaf near"].Collapsed != 0 {
		t.Fatalf("a direct child reports a collapse: %+v", byKey["Leaf near"])
	}
	if view.Collapsed != 3 {
		t.Fatalf("the view's total collapsed = %d, want 3", view.Collapsed)
	}
	if view.MaxDepth < 4 {
		t.Fatalf("maxDepth = %d; the walk went four generations up and should say so",
			view.MaxDepth)
	}
}

// TestAnIsolatedLivingSpeciesIsDrawnAndLabelled is rule 3: the derivation's limit
// is a shape, not a failure and not a disclaimer.
//
// A species whose lineage has never crossed a lane has no migration record, so
// no record can carry its parent's name. It is alive, it is real, and the honest
// thing to draw is a leaf with nothing above it — counted separately so a reader
// can see exactly how much of the map the ancestry covers.
func TestAnIsolatedLivingSpeciesIsDrawnAndLabelled(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(30, 0,
			entry("Beta", "one", 10, 0), entry("Gamma", "two", 10, 0),
			// Alive, and no crossing of it has ever carried a parent name.
			entry("Basic", "bibite", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	a.observeSpeciesLocked(child(base, "Beta", "one", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+1, "Gamma", "two", "Alpha", "nullus"))
	// It has crossed — a great deal — and never with a parent block.
	a.observeSpeciesLocked(migration(base+2, 1, 2, "E", "Basic", "bibite", "h-basic"))
	a.mu.Unlock()

	view := a.SpeciesTreeView()
	byKey := treeNodes(t, view)
	basic, ok := byKey["Basic bibite"]
	if !ok {
		t.Fatal("a living species with no recorded ancestry was DROPPED from the tree; it is " +
			"alive, and a genealogy that hides what it cannot explain is worse than one that " +
			"labels it")
	}
	if !basic.Isolated || !basic.Root || basic.Parent != "" || basic.Leaves != 1 {
		t.Fatalf("the isolated leaf is not drawn as one: %+v", basic)
	}
	// AND IT IS THE "no recorded ancestry" KIND, which is the honest label. The
	// other kind — ancestry recorded, no living relative — must not borrow it.
	if basic.AncestryKnown || basic.AncestryDepth != 0 {
		t.Fatalf("a species no record names a parent for is claimed to have ancestry: %+v",
			basic)
	}
	if view.Unrecorded != 1 {
		t.Fatalf("unrecorded = %d, want 1", view.Unrecorded)
	}
	if basic.Crossings == 0 {
		t.Fatal("the isolated leaf lost its ledger annotation; it has crossed, it simply " +
			"never carried a parent name")
	}
	if view.Isolated != 1 || view.Connected != 2 {
		t.Fatalf("isolated=%d connected=%d, want 1 and 2 — the limit is published as a "+
			"number", view.Isolated, view.Connected)
	}
	// A FOREST, and it is drawn as one. The bigger tree comes first so the page
	// reads top-down from the map's main family.
	if len(view.Roots) != 2 || view.Roots[0] != "Alpha nullus" ||
		view.Roots[1] != "Basic bibite" {
		t.Fatalf("roots = %v, want the two-leaf family before the lone leaf", view.Roots)
	}
	// And the honest ratio behind the label on the page.
	if view.LedgerEdges != 2 || view.LedgerSpecies != 3 {
		t.Fatalf("ledgerEdges=%d ledgerSpecies=%d; the page states this ratio as the "+
			"derivation's reach", view.LedgerEdges, view.LedgerSpecies)
	}
	// The edge count is a MAINTAINED COUNTER and not a walk, so a correction to a
	// species' parent must not inflate it: one species, one edge, however many
	// records name it.
	a.mu.Lock()
	a.observeSpeciesLocked(child(base+9, "Beta", "one", "Other", "ancestor"))
	a.mu.Unlock()
	if again := a.SpeciesTreeView(); again.LedgerEdges != 2 {
		t.Fatalf("ledgerEdges=%d after a correction, want 2 — a replaced parent is not a "+
			"second edge", again.LedgerEdges)
	}
}

// TestADrawnRootIsNotAClaimOfOrigin is the second half of rule 3, and it is the
// shape a reader misreads by default.
//
// The reduction stops at the highest node with a living branch, so the row at the
// top of the drawing very often has ancestors — dozens of them — that the record
// holds and the tree does not draw, because not one of them has another living
// descendant. Drawn with no mark at all that row says "the family begins here",
// which on the running rig is wrong by 31 generations for `Zhiluus tardisitguyus`.
//
// THE FIX IS A LABEL AND NEVER AN EDGE. The view already carries the two facts a
// label needs, and this pins that they stay TOLD APART on every root: a root the
// record reaches above (AncestryKnown, with a depth) and a root it does not
// (`Basic bibite`) are different answers and the page badges them differently.
func TestADrawnRootIsNotAClaimOfOrigin(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(46, 0,
			entry("Beta", "one", 20, 0), entry("Gamma", "two", 10, 0),
			// Alive, and no crossing of it has ever named a parent.
			entry("Basic", "bibite", 11, 0),
			// Alive, with a recorded family that is entirely extinct.
			entry("Lone", "one", 5, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	// Two living lines part at Alpha — and Alpha has a recorded line of its own
	// above it, two generations of it, with nothing alive on either.
	a.observeSpeciesLocked(child(base, "Beta", "one", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+1, "Gamma", "two", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+2, "Alpha", "nullus", "Older", "still"))
	a.observeSpeciesLocked(child(base+3, "Older", "still", "Elder", "one"))
	// A living species under three extinct generations, none of them shared.
	a.observeSpeciesLocked(child(base+4, "Lone", "one", "Gone", "first"))
	a.observeSpeciesLocked(child(base+5, "Gone", "first", "Gone", "second"))
	a.observeSpeciesLocked(child(base+6, "Gone", "second", "Gone", "third"))
	// And one that has crossed plenty and never carried a parent block.
	a.observeSpeciesLocked(migration(base+7, 1, 2, "E", "Basic", "bibite", "h-basic"))
	a.mu.Unlock()

	view := a.SpeciesTreeView()
	byKey := treeNodes(t, view)
	if len(view.Roots) != 3 || view.Roots[0] != "Alpha nullus" {
		t.Fatalf("roots = %v, want three with the two-leaf family first", view.Roots)
	}

	// THE BADGED ROOT. It is drawn at the top and the record reaches two
	// generations past it; both are on the node, so the page can say so.
	alpha := byKey["Alpha nullus"]
	if alpha.Parent != "" || !alpha.Root || alpha.Isolated {
		t.Fatalf("the branch point is not the drawn root it is: %+v", alpha)
	}
	if !alpha.AncestryKnown || alpha.AncestryDepth != 2 {
		t.Fatalf("the root's own recorded ancestry is %v/%d, want known and 2 generations — "+
			"the number the badge prints is the difference between 'the family begins here' "+
			"and 'the drawing does'", alpha.AncestryKnown, alpha.AncestryDepth)
	}
	// The collapsed run ABOVE a root is not an edge and is not counted as one:
	// there is no kept node up there for an edge to reach.
	if alpha.Collapsed != 0 || view.Collapsed != 0 {
		t.Fatalf("the run above the root was counted as a collapsed edge: node=%d view=%d",
			alpha.Collapsed, view.Collapsed)
	}
	// Nothing above it was invented into the node set. The honest answer is a
	// label on a root, never an edge down to a species nobody recorded.
	for _, gone := range []string{"Older still", "Elder one", "Gone first", "Gone second",
		"Gone third"} {
		if _, ok := byKey[gone]; ok {
			t.Fatalf("%q was drawn; an ancestor with one living line below it is a step in a "+
				"corridor with no doors", gone)
		}
	}

	// THE OTHER TWO ROOTS, which must not borrow each other's label. Both stand
	// alone; only one of them is a species the record cannot place.
	lone := byKey["Lone one"]
	if !lone.Isolated || !lone.AncestryKnown || lone.AncestryDepth != 3 {
		t.Fatalf("a species with a recorded and extinct family is not carrying it: %+v", lone)
	}
	basic := byKey["Basic bibite"]
	if !basic.Isolated || basic.AncestryKnown || basic.AncestryDepth != 0 {
		t.Fatalf("a species no record names a parent for is claimed to have ancestry: %+v",
			basic)
	}
	if view.Isolated != 2 || view.Unrecorded != 1 {
		t.Fatalf("isolated=%d unrecorded=%d, want 2 and 1 — one stands alone because the "+
			"record holds nothing, the other because its whole family is gone",
			view.Isolated, view.Unrecorded)
	}
}

// TestTheRecordsAncestryFloorIsPublishedAndMaintained is the caption under the
// badge above, and the reason a root's depth is not a mystery.
//
// The chain above a root ends where the RECORD ends. This archive's ledger is
// older than the parent-species field itself (contract-a.md §16 A30): on the
// running rig the first 19.5 hours of crossings carry no species block at all, so
// no edge can exist before the first one that does. That instant is published, it
// is ONE MAINTAINED TIMESTAMP rather than a walk of the aggregate, and it survives
// a restart the same way every other counter here does.
func TestTheRecordsAncestryFloorIsPublishedAndMaintained(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(20, 0,
			entry("Beta", "one", 10, 0), entry("Gamma", "two", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()

	a.mu.Lock()
	// A crossing an hour before any ancestry was ever recorded. It is a record,
	// it is counted, and it is NOT a floor: nothing in it could ever be an edge.
	a.observeSpeciesLocked(migration(base-3600000, 1, 2, "E", "Beta", "one", "h-early"))
	a.observeSpeciesLocked(child(base+1000, "Beta", "one", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+2000, "Gamma", "two", "Alpha", "nullus"))
	a.mu.Unlock()

	view := a.SpeciesTreeView()
	if view.AncestrySinceMs != base+1000 {
		t.Fatalf("ancestrySinceMs = %d, want %d — the floor is the first crossing that NAMED "+
			"a parent, not the first crossing", view.AncestrySinceMs, base+1000)
	}

	// A later record does not move a floor.
	a.mu.Lock()
	a.observeSpeciesLocked(child(base+9000, "Gamma", "two", "Beta", "one"))
	a.mu.Unlock()
	if again := a.SpeciesTreeView(); again.AncestrySinceMs != base+1000 {
		t.Fatalf("a later crossing moved the floor to %d", again.AncestrySinceMs)
	}
	// An OLDER one does, even though it loses the latest-writer test for the edge
	// itself: an older answer about a parent is exactly what lowers a floor.
	a.mu.Lock()
	a.observeSpeciesLocked(child(base+500, "Gamma", "two", "Alpha", "nullus"))
	a.mu.Unlock()
	lowered := a.SpeciesTreeView()
	if lowered.AncestrySinceMs != base+500 {
		t.Fatalf("ancestrySinceMs = %d, want %d — a record older than the floor and carrying "+
			"a parent is the floor", lowered.AncestrySinceMs, base+500)
	}
	if byKey := treeNodes(t, lowered); byKey["Gamma two"].Parent != "Beta one" {
		t.Fatalf("the older record overwrote the newer edge: %+v", byKey["Gamma two"])
	}

	// ON THE WIRE, because a distinction a client cannot see is not a distinction:
	// the page prints this date under the tree.
	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)
	body := get(t, srv.URL+"/api/species/tree")
	if !strings.Contains(body, `"ancestrySinceMs":`) {
		t.Fatalf("the endpoint does not publish the record's floor:\n%s", body)
	}
	var served SpeciesTree
	if err := json.Unmarshal([]byte(body), &served); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if served.AncestrySinceMs != base+500 {
		t.Fatalf("served floor = %d, want %d", served.AncestrySinceMs, base+500)
	}

	// AND IT IS REBUILT BY THE REPLAY, like every other maintained counter — a
	// restart must not tell an operator the record's ancestry started today.
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatalf("OpenLedger: %v", err)
	}
	for _, rec := range []Record{
		migration(base-3600000, 1, 2, "E", "Beta", "one", "h-early"),
		child(base+1000, "Beta", "one", "Alpha", "nullus"),
		child(base+2000, "Gamma", "two", "Alpha", "nullus"),
	} {
		if err := ledger.Append(rec); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := ledger.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	restarted, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	restarted.mu.Lock()
	floor := restarted.species.edgeFirstMs
	restarted.mu.Unlock()
	if floor != base+1000 {
		t.Fatalf("the replay rebuilt the floor as %d, want %d", floor, base+1000)
	}
}

// TestTheGenealogyIsMaintainedWithoutARescan is rule 1, and it is the rule that
// keeps this view affordable on a ledger of 8.8 million records.
//
// There is ONE writer of the edge and it is observeSpeciesLocked — the same
// function the startup replay calls and the same one the live path calls. So a
// migration arriving now must move the tree without anything re-reading the
// ledger, and the test proves it by taking the view, folding one more record,
// and taking it again.
func TestTheGenealogyIsMaintainedWithoutARescan(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(20, 0,
			entry("Beta", "one", 10, 0), entry("Gamma", "two", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()

	a.mu.Lock()
	a.observeSpeciesLocked(child(base, "Beta", "one", "Alpha", "nullus"))
	a.mu.Unlock()

	// Before: the record knows where Beta came from and nothing about Gamma, and
	// neither has a living relative — so both stand alone, for TWO DIFFERENT
	// REASONS the view keeps apart.
	before := a.SpeciesTreeView()
	if before.Isolated != 2 || before.Unrecorded != 1 || len(before.Roots) != 2 {
		t.Fatalf("before: isolated=%d unrecorded=%d roots=%v",
			before.Isolated, before.Unrecorded, before.Roots)
	}
	beforeKeys := treeNodes(t, before)
	if !beforeKeys["Beta one"].AncestryKnown || beforeKeys["Beta one"].AncestryDepth != 1 {
		t.Fatalf("Beta's recorded ancestry was thrown away with its lone branch: %+v",
			beforeKeys["Beta one"])
	}
	if beforeKeys["Gamma two"].AncestryKnown {
		t.Fatalf("Gamma has no recorded parent and is claimed to have one: %+v",
			beforeKeys["Gamma two"])
	}

	// One live record — the ledger is not touched and nothing is re-read.
	a.mu.Lock()
	a.observeSpeciesLocked(child(base+1000, "Gamma", "two", "Alpha", "nullus"))
	a.mu.Unlock()

	after := a.SpeciesTreeView()
	if after.Isolated != 0 || len(after.Roots) != 1 {
		t.Fatalf("one live crossing did not move the graph: isolated=%d roots=%v",
			after.Isolated, after.Roots)
	}
	byKey := treeNodes(t, after)
	if byKey["Gamma two"].Parent != "Alpha nullus" {
		t.Fatalf("the new edge did not land: %+v", byKey["Gamma two"])
	}
	// And a CORRECTION is taken, latest writer wins on the archive's own clock:
	// a species has one parent species in the game's model, so a second answer
	// is a correction and not a second edge.
	a.mu.Lock()
	a.observeSpeciesLocked(child(base+2000, "Gamma", "two", "Beta", "one"))
	a.mu.Unlock()
	corrected := treeNodes(t, a.SpeciesTreeView())
	if corrected["Gamma two"].Parent != "Beta one" {
		t.Fatalf("a later record did not correct the edge: %+v", corrected["Gamma two"])
	}
	if n := len(a.SpeciesTreeView().Nodes); n != 2 {
		t.Fatalf("the corrected tree holds %d nodes, want 2: Beta is now Gamma's parent and "+
			"Alpha is a chain link above a single line, which collapses", n)
	}
}

// TestTheGenealogySurvivesARestart is rule 1's other half, exactly as
// TestSpeciesAggregateSurvivesARestart is for the flat index: the edge is rebuilt
// by the streaming replay New() already performs, so a restart does not tell an
// operator that nothing on the map is related to anything.
func TestTheGenealogySurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatalf("OpenLedger: %v", err)
	}
	base := time.Now().Add(-time.Hour).UnixMilli()
	for _, rec := range []Record{
		child(base, "Beta", "one", "Alpha", "nullus"),
		child(base+1000, "Gamma", "two", "Alpha", "nullus"),
	} {
		if err := ledger.Append(rec); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := ledger.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	a, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	a.mu.Lock()
	beta := a.species.byKey["Beta one"]
	a.mu.Unlock()
	if beta == nil || beta.parentKey != "Alpha nullus" {
		t.Fatalf("the replay did not rebuild the ancestry edge: %+v", beta)
	}
}

// TestADerivedEdgeCannotLoopTheView is rule 4. The measured ledger holds no cycle
// across 2 407 species — but the two names an edge is built from are
// attacker-chosen strings normalized into a key, and "measured clean today" is
// not an invariant. A view that hung forever on a malformed pair would be a
// denial of service delivered by a species name.
func TestADerivedEdgeCannotLoopTheView(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(10, 0,
			entry("Loop", "aye", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	// A two-cycle among the ancestors: aye <- bee <- aye.
	a.observeSpeciesLocked(child(base, "Loop", "aye", "Loop", "bee"))
	a.observeSpeciesLocked(child(base+1, "Loop", "bee", "Loop", "aye"))
	// And a species that is its own parent, which the ingest guard drops before
	// any walk can see it.
	a.observeSpeciesLocked(child(base+2, "Self", "same", "Self", "same"))
	a.mu.Unlock()

	done := make(chan SpeciesTree, 1)
	go func() { done <- a.SpeciesTreeView() }()
	var view SpeciesTree
	select {
	case view = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("SpeciesTreeView did not return on a cyclic derived edge; the guard is the " +
			"whole reason a derived graph may be walked at all")
	}
	if view.CycleGuard == 0 {
		t.Fatal("the walk met a cycle and did not say so; a guard that fires silently is a " +
			"guard nobody can trust")
	}
	// The self-parent never became an edge at all.
	a.mu.Lock()
	self := a.species.byKey["Self same"]
	a.mu.Unlock()
	if self == nil || self.parentKey != "" {
		t.Fatalf("a species is recorded as its own parent: %+v", self)
	}
	// And the tree it produced is still a tree: every node placed once, and the
	// living species is in it.
	byKey := treeNodes(t, view)
	if _, ok := byKey["Loop aye"]; !ok {
		t.Fatalf("the living species was lost to its own malformed ancestry: %+v", view.Nodes)
	}
}

// TestAStaleCensusHidesTheGenealogyToo is §10.1's freshness rule on the third
// view. A tree whose LEAVES came from a census too old to be state is a tree of
// who WAS alive, and drawing it as a live genealogy is the same error the flat
// index refuses to make.
func TestAStaleCensusHidesTheGenealogyToo(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(20, 0,
			entry("Beta", "one", 10, 0), entry("Gamma", "two", 10, 0)))},
	}
	a := newViewFixture(t, status, 5*time.Minute)
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	a.observeSpeciesLocked(child(base, "Beta", "one", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+1, "Gamma", "two", "Alpha", "nullus"))
	a.mu.Unlock()

	view := a.SpeciesTreeView()
	if len(view.Nodes) != 0 || len(view.Roots) != 0 || view.Alive != 0 {
		t.Fatalf("a five-minute-old census produced a live genealogy: %+v", view)
	}
}

// TestTheGenealogyEndpointShape asserts the wire shape the page depends on, on
// the JSON rather than on the Go value — a distinction a client cannot see is not
// a distinction. It also pins the endpoint's separateness: this view is DERIVED
// and must not widen the durable sample file (§20, B20).
func TestTheGenealogyEndpointShape(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(20, 0,
			// A raw spelling with a doubled space, which must survive verbatim.
			entry("Beta ", "one", 10, 0), entry("Gamma", "two", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	a.observeSpeciesLocked(child(base, "Beta", "one", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+1, "Gamma", "two", "Alpha", "nullus"))
	a.mu.Unlock()

	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/api/species/tree")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var view SpeciesTree
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !view.HaveStatus || view.Alive != 2 || len(view.Nodes) != 3 {
		t.Fatalf("the endpoint's answer is not the view: %+v", view)
	}
	if len(view.Roots) != 1 {
		t.Fatalf("roots = %v", view.Roots)
	}
	byKey := treeNodes(t, view)
	// THE RAW SPELLING SURVIVES THE ROUND TRIP. A34 is a comparison, never a
	// repair (contract-a.md §17, A36).
	if byKey["Beta one"].Name != "Beta  one" {
		t.Fatalf("the raw spelling was repaired on the wire: %q", byKey["Beta one"].Name)
	}
	// Nodes arrive in DFS pre-order, so a renderer lays the tree out in one pass.
	if view.Nodes[0].Key != "Alpha nullus" || view.Nodes[0].Depth != 0 {
		t.Fatalf("the first node is not the root at depth 0: %+v", view.Nodes[0])
	}
	for _, n := range view.Nodes[1:] {
		if n.Depth != 1 {
			t.Fatalf("%s is at depth %d, want 1", n.Key, n.Depth)
		}
	}

	// THE DERIVED TREE STAYS OUT OF /api/status, which is what MetricsLog
	// serializes verbatim once a minute. Every edge in it is already in the
	// ledger and every leaf already in the census, so writing it to disk forever
	// would be the same mistake §17 B14 named.
	sresp, err := http.Get(srv.URL + "/api/status")
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	defer sresp.Body.Close()
	var raw map[string]any
	if err := json.NewDecoder(sresp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	for _, forbidden := range []string{"tree", "nodes", "roots", "ancestors"} {
		if _, ok := raw[forbidden]; ok {
			t.Fatalf("/api/status carries %q; the genealogy rides its own endpoint so the "+
				"durable sample file does not grow a recomputable tree", forbidden)
		}
	}
}

// TestTheDenyListSuppressesAGenealogyNodeAndEveryEdgeToIt is DQ7 (§22, B30) on
// the third surface, and it carries one requirement the flat index does not: a
// key here is ALSO AN EDGE. A suppression that renamed a node and left its
// children pointing at the old key would publish the suppressed name in the
// parent field of every child.
func TestTheDenyListSuppressesAGenealogyNodeAndEveryEdgeToIt(t *testing.T) {
	dir := t.TempDir()
	denyPath := dir + "/deny.txt"
	write(t, denyPath, "Alpha nullus\n")
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(20, 0,
			entry("Beta", "one", 10, 0), entry("Gamma", "two", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	deny, err := NewDenyList(denyPath)
	if err != nil {
		t.Fatalf("NewDenyList: %v", err)
	}
	a.deny = deny
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	a.observeSpeciesLocked(child(base, "Beta", "one", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+1, "Gamma", "two", "Alpha", "nullus"))
	a.mu.Unlock()

	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)
	body := get(t, srv.URL+"/api/species/tree")
	if strings.Contains(body, "Alpha") {
		t.Fatalf("the denied ancestor's name is served anyway:\n%s", body)
	}
	if !strings.Contains(body, Suppressed) {
		t.Fatalf("a denied name is ERASED rather than marked; an operator reading their own "+
			"deny list has to see it working:\n%s", body)
	}
	var view SpeciesTree
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// THE SHAPE SURVIVES. Deleting the node would reparent two living species
	// under a grandparent they never descended from — a lie about the record,
	// told to hide a name.
	if len(view.Nodes) != 3 || len(view.Roots) != 1 {
		t.Fatalf("suppression changed the tree's shape: %+v", view)
	}
	suppressedKey := view.Roots[0]
	if !strings.HasPrefix(suppressedKey, Suppressed) {
		t.Fatalf("the root's KEY is the name spelled normalized and was served: %q",
			suppressedKey)
	}
	// Every edge follows the rename, and no child still points at the plaintext.
	for _, n := range view.Nodes {
		if n.Key == suppressedKey {
			continue
		}
		if n.Parent != suppressedKey {
			t.Fatalf("%s's parent is %q, not the renamed key — a page would resolve the "+
				"plaintext key the suppression was meant to remove", n.Key, n.Parent)
		}
	}
}

// TestAMarkupSpeciesNameNeverBecomesMarkupInTheGenealogy extends
// contract-b-m4.md §13 item 7 to this view's two name-bearing fields.
//
// A node's label comes from one of two attacker-controlled places — a census
// entry or a `parentGenericName` on a migration envelope — and NEITHER is
// sanitized upstream, by guarantee (§16 A34 and §17 A36 both promise the
// opposite). So the JSON must carry the bytes verbatim, escaped as JSON, and the
// page must build every one of them as a text node. The second half is a
// property over the fenced region and is asserted in page_test.go.
func TestAMarkupSpeciesNameNeverBecomesMarkupInTheGenealogy(t *testing.T) {
	const nasty = `<script>alert("x")</script>`
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(20, 0,
			entry(nasty, "leaf", 10, 0), entry("Gamma", "two", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	// The markup rides in as an ANCESTOR name too, which is the field this view
	// added and the one nothing else on the page reads.
	a.observeSpeciesLocked(child(base, nasty, "leaf", nasty, "root"))
	a.observeSpeciesLocked(child(base+1, "Gamma", "two", nasty, "root"))
	a.mu.Unlock()

	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)
	body := get(t, srv.URL+"/api/species/tree")
	// JSON, not HTML: the raw angle brackets must not appear unescaped in the
	// served bytes, and Go's encoder is what guarantees it.
	if strings.Contains(body, "<script>") {
		t.Fatalf("the served JSON carries unescaped markup:\n%s", body)
	}
	var view SpeciesTree
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byKey := treeNodes(t, view)
	// And it survives VERBATIM as data, in both the leaf's label and the
	// ancestor's: repairing it would be the other failure.
	leaf, ok := byKey[wire.SpeciesKey(nasty, "leaf")]
	if !ok || leaf.Name != nasty+" leaf" {
		t.Fatalf("the leaf's raw name did not survive as data: %+v", byKey)
	}
	root, ok := byKey[wire.SpeciesKey(nasty, "root")]
	if !ok || !strings.Contains(root.Name, nasty) || root.NameFrom != "record" {
		t.Fatalf("the ancestor's raw name did not survive as data: %+v", root)
	}
}
