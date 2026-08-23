package archive

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"multiverse/internal/bb8"
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

func livingTreeNode(t *testing.T, view SpeciesTree, nameKey string) TreeNode {
	t.Helper()
	var found *TreeNode
	for i := range view.Nodes {
		n := &view.Nodes[i]
		if n.Alive && n.NameKey == nameKey {
			if found != nil {
				t.Fatalf("the view has more than one living lineage instance named %q", nameKey)
			}
			found = n
		}
	}
	if found == nil {
		t.Fatalf("the view has no living lineage instance named %q: %+v", nameKey, view.Nodes)
	}
	return *found
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
	// A second parent behind one name is a second lineage instance. The old edge
	// stays intact and the new instance contributes its own edge.
	a.mu.Lock()
	a.observeSpeciesLocked(child(base+9, "Beta", "one", "Other", "ancestor"))
	a.mu.Unlock()
	if again := a.SpeciesTreeView(); again.LedgerEdges != 3 || again.SplitNames == 0 {
		t.Fatalf("ledgerEdges=%d splitNames=%d after a reused name, want 3 and a published split",
			again.LedgerEdges, again.SplitNames)
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
	gamma := livingTreeNode(t, lowered, "Gamma two")
	if parent := treeNodes(t, lowered)[gamma.Parent]; parent.NameKey != "Beta one" {
		t.Fatalf("the older record displaced the current lineage instance: %+v", gamma)
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
	gamma := livingTreeNode(t, after, "Gamma two")
	if byKey[gamma.Parent].NameKey != "Alpha nullus" {
		t.Fatalf("the new edge did not land: %+v", gamma)
	}
	// And a CORRECTION is taken, latest writer wins on the archive's own clock:
	// a species has one parent species in the game's model, so a second answer
	// is a correction and not a second edge.
	a.mu.Lock()
	a.observeSpeciesLocked(child(base+2000, "Gamma", "two", "Beta", "one"))
	a.mu.Unlock()
	correctedView := a.SpeciesTreeView()
	corrected := treeNodes(t, correctedView)
	gamma = livingTreeNode(t, correctedView, "Gamma two")
	if corrected[gamma.Parent].NameKey != "Beta one" {
		t.Fatalf("a later parent claim did not create the current instance: %+v", gamma)
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

// TestAConflictingNameCannotLoopTheView covers the identity fold and the final
// walk together. A second parent behind the same name creates a second lineage
// instance. It does not close an edge back into the first instance.
func TestAConflictingNameCannotLoopTheView(t *testing.T) {
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
	if view.CycleGuard != 0 {
		t.Fatalf("the lineage-instance fold left %d cycle guard hit(s), want none", view.CycleGuard)
	}
	if view.SplitNames == 0 {
		t.Fatal("the conflicting name was not published as separate lineage instances")
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

// ---------------------------------------------------------- the merged view

// TestTheLifespanGeometryIsTheRecordsOwnSpan is the layout the drawing is built
// from, tested as DATA rather than as pixels: the page maps these two numbers
// onto one linear scale and nothing else about a bar is decided in the browser.
//
// THE FIXTURE IS A FAMILY WITH KNOWN DATES, and it is arranged so that every
// rule about a bar's ends is exercised by a node that needs it:
//
//	Beta crossed BEFORE anything named a parent, so its bar starts before the
//	record's own ancestry floor and before its parent's bar. That is not
//	repaired: the record says what it says, and clamping a child up to its
//	parent would draw a picture the ledger does not support.
//
//	Ghost never crossed at all. It exists only because two of its children's
//	crossings named it, so its bar begins at the EARLIEST OF THEM — a bound the
//	record genuinely carries — and is marked derived so the page can draw it as
//	the inference it is.
//
//	Alpha is extinct, so its bar STOPS at its last recorded crossing. The living
//	species have no right-hand end at all, because a bar that stopped at their
//	last crossing would say they died when they merely stopped travelling.
func TestTheLifespanGeometryIsTheRecordsOwnSpan(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(60, 0,
			entry("Beta", "one", 30, 0), entry("Gamma", "two", 20, 0),
			entry("Delta", "four", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-3 * time.Hour).UnixMilli()

	a.mu.Lock()
	// A parented crossing by a species with no living descendant. It is the
	// OLDEST ancestry the record holds and it is drawn nowhere — which is exactly
	// why the axis has to be told about it, or the floor would sit off the left
	// edge of every picture that has one.
	a.observeSpeciesLocked(child(base+500, "Dead", "end", "Long", "gone"))
	// Beta's first crossing, carrying no parent at all.
	a.observeSpeciesLocked(migration(base+1000, 1, 2, "E", "Beta", "one", "h-beta-early"))
	a.observeSpeciesLocked(child(base+2000, "Alpha", "nullus", "Ghost", "none"))
	a.observeSpeciesLocked(migration(base+4000, 1, 2, "E", "Alpha", "nullus", "h-alpha-late"))
	a.observeSpeciesLocked(child(base+5000, "Beta", "one", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+6000, "Gamma", "two", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+7000, "Delta", "four", "Ghost", "none"))
	a.mu.Unlock()

	view := a.SpeciesTreeView()
	treeNodes(t, view)

	// The rows, in the order the renderer lays them out: DFS pre-order, busiest
	// branch first. The array index IS the row.
	want := []string{"Alpha nullus", "Beta one", "Gamma two", "Delta four"}
	if len(view.Nodes) != len(want) {
		t.Fatalf("the view holds %d rows, want %d: %+v", len(view.Nodes), len(want), view.Nodes)
	}
	for i, key := range want {
		if view.Nodes[i].NameKey != key {
			t.Fatalf("row %d is %q, want %q — the row order is the layout", i, view.Nodes[i].NameKey, key)
		}
	}

	for _, tc := range []struct {
		key      string
		from, to int64
		derived  bool
	}{
		// A living species: from its first recorded crossing, and NO right-hand
		// end.
		{"Beta one", base + 5000, 0, false},
		{"Gamma two", base + 6000, 0, false},
		{"Delta four", base + 7000, 0, false},
		// Extinct: first crossing to last, and it stops there.
		{"Alpha nullus", base + 4000, base + 4000, false},
	} {
		n := TreeNode{}
		for _, candidate := range view.Nodes {
			if candidate.NameKey == tc.key {
				n = candidate
				break
			}
		}
		if n.SpanFromMs != tc.from || n.SpanToMs != tc.to || n.SpanDerived != tc.derived {
			t.Fatalf("%s spans %d..%d (derived %v), want %d..%d (derived %v)",
				tc.key, n.SpanFromMs, n.SpanToMs, n.SpanDerived, tc.from, tc.to, tc.derived)
		}
	}

	// THE AXIS. It starts at the OLDEST DRAWN BAR — Alpha's current instance — and ends
	// now. The record's ancestry floor is half a second older, carried by a species
	// this picture does not draw, and it does NOT pull the edge back with it: that
	// clamp existed so the floor could be drawn as a boundary, and it paid for the
	// boundary with a stretch of plot that holds nothing and grows.
	if view.AncestrySinceMs != base+500 {
		t.Fatalf("ancestrySinceMs = %d, want %d", view.AncestrySinceMs, base+500)
	}
	if view.SpanStartMs != base+4000 {
		t.Fatalf("spanStartMs = %d, want the oldest drawn bar at %d — the axis fits the record "+
			"it draws, and the floor is a caption at the margin rather than empty pixels inside "+
			"it", view.SpanStartMs, base+4000)
	}
	if !(view.AncestrySinceMs < view.SpanStartMs) {
		t.Fatal("the fixture no longer has a floor older than every drawn bar, which is the " +
			"shape the caption exists for")
	}
	if view.SpanEndMs != view.GeneratedAtMs {
		t.Fatalf("spanEndMs = %d, want now (%d)", view.SpanEndMs, view.GeneratedAtMs)
	}
	if view.SpanStartMs >= view.SpanEndMs {
		t.Fatal("the axis has no width")
	}

	// AND ON THE WIRE, because a distinction a client cannot see is not one.
	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)
	var served SpeciesTree
	if err := json.Unmarshal([]byte(get(t, srv.URL+"/api/species/tree")), &served); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if served.SpanStartMs != view.SpanStartMs || served.SpanEndMs == 0 {
		t.Fatalf("the axis did not survive the wire: %d..%d", served.SpanStartMs, served.SpanEndMs)
	}
	if delta := livingTreeNode(t, served, "Delta four"); delta.AncestryDepth != 1 {
		t.Fatal("the collapsed parent evidence did not survive the wire")
	}
}

// TestTheRecordProducesEveryShapeALinkMustDraw is the server half of the page's
// link geometry. The drawing puts EVERY horizontal distance on the clock now,
// including the dotted run that stands for collapsed generations — it is drawn
// from where the parent's own record stops to where the child's begins — and the
// shape it takes is decided entirely by the three numbers published here. So the
// four arrangements are pinned as LAYOUT DATA, on one fixture, in the shapes the
// running rig really holds:
//
//	A GAP UNDER A COLLAPSED RUN. An extinct branch point whose record stopped
//	38 hours before its next drawn descendant's began. That interval is exactly
//	when the collapsed generations held the line, and until it was drawn it was
//	blank plot with a fixed-length mark beside it on another scale.
//
//	A GAP UNDER A DIRECT LINK. The same silence with nothing collapsed in it —
//	which the page draws as the same run, undotted, because nothing about it is
//	undrawn.
//
//	A GAP TOO SHORT TO SEE. Four minutes across days of axis. The interval is
//	real and sub-pixel, which is a floor for the drawing and not a licence to
//	round it away here.
//
//	NO GAP AT ALL. A living parent has no last recorded moment — its bar runs
//	to the right-hand edge — so every child of one starts inside its span and
//	there is no interval any mark could honestly claim.
//
//	AND THE INVERSION. A child whose own first crossing PRECEDES its parent's.
//	Ancestry here is a by-product of travel, so a species is first seen when it
//	happens to cross a lane, and the younger kind can cross first. Both dates
//	are the record's and the descent is real. Note where this does NOT go: the
//	derived-span machinery fires only when a node's own record holds no crossing
//	at all, and it already takes the earliest dated descendant. This is the
//	complementary case — the parent HAS a crossing, later than its child's — and
//	the honest answer is to publish both and draw the fact.
func TestTheRecordProducesEveryShapeALinkMustDraw(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(60, 0,
			entry("Gap", "far", 10, 0), entry("Near", "leaf", 10, 0),
			entry("Tiny", "leaf", 10, 0), entry("Live", "par", 10, 0),
			entry("In", "kid", 10, 0), entry("Late", "par", 10, 0),
			entry("Early", "kid", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	const hour = int64(3600000)
	base := time.Now().Add(-60 * time.Hour).UnixMilli()

	a.mu.Lock()
	// The extinct branch point: two crossings of its own, two hours apart, and
	// nothing of it in the census.
	a.observeSpeciesLocked(migration(base, 1, 2, "E", "Old", "root", "h-old-a"))
	a.observeSpeciesLocked(migration(base+2*hour, 1, 2, "E", "Old", "root", "h-old-b"))
	// A chain of extinct generations under it, ending in a living species whose own
	// record begins long after the branch point's ended.
	a.observeSpeciesLocked(child(base+3*hour, "Mid", "one", "Old", "root"))
	a.observeSpeciesLocked(child(base+4*hour, "Mid", "two", "Mid", "one"))
	a.observeSpeciesLocked(child(base+5*hour, "Mid", "three", "Mid", "two"))
	a.observeSpeciesLocked(child(base+40*hour, "Gap", "far", "Mid", "three"))
	// A direct child across the same kind of silence, and one across four minutes.
	a.observeSpeciesLocked(child(base+30*hour, "Near", "leaf", "Old", "root"))
	a.observeSpeciesLocked(child(base+2*hour+240000, "Tiny", "leaf", "Old", "root"))
	// A living parent, with a child that first crossed deep inside its span.
	a.observeSpeciesLocked(migration(base+hour, 1, 2, "E", "Live", "par", "h-live"))
	a.observeSpeciesLocked(child(base+50*hour, "In", "kid", "Live", "par"))
	// And the inversion: the child crossed four hours before its parent ever did.
	a.observeSpeciesLocked(child(base+45*hour, "Late", "par", "Live", "par"))
	a.observeSpeciesLocked(child(base+41*hour, "Early", "kid", "Late", "par"))
	a.mu.Unlock()

	byKey := treeNodes(t, a.SpeciesTreeView())
	need := func(k string) TreeNode {
		t.Helper()
		n, ok := byKey[k]
		if !ok {
			t.Fatalf("%q is not drawn; the fixture no longer holds the shape it exists for", k)
		}
		return n
	}

	// The parent every gap hangs off: extinct, and its record really does stop.
	root := need("Old root")
	if root.Alive || root.SpanFromMs != base || root.SpanToMs != base+2*hour {
		t.Fatalf("Old root spans %d..%d (alive %v), want %d..%d extinct — the parent's LAST "+
			"recorded moment is where a link across a gap leaves it",
			root.SpanFromMs, root.SpanToMs, root.Alive, base, base+2*hour)
	}

	for _, tc := range []struct {
		key       string
		parent    string
		collapsed int
		gapMs     int64 // child's first-seen MINUS the parent's last recorded moment
	}{
		// A run of 38 h under three collapsed generations. Drawn dotted, because the
		// species that carried the line through it are not on the picture.
		{"Gap far", "Old root", 3, 38 * hour},
		// The same silence with a direct link across it: 28 h, nothing collapsed.
		{"Near leaf", "Old root", 0, 28 * hour},
		// Four minutes. Real, positive, and a fraction of a pixel.
		{"Tiny leaf", "Old root", 0, 240000},
	} {
		n := need(tc.key)
		if n.Parent != tc.parent || n.Collapsed != tc.collapsed {
			t.Fatalf("%s hangs off %q with %d collapsed, want %q with %d",
				tc.key, n.Parent, n.Collapsed, tc.parent, tc.collapsed)
		}
		p := need(tc.parent)
		got := n.SpanFromMs - (p.SpanToMs)
		if got != tc.gapMs {
			t.Fatalf("%s begins %d ms after %s's record stops, want %d — the run the page "+
				"draws IS this interval", tc.key, got, tc.parent, tc.gapMs)
		}
		if got <= 0 {
			t.Fatalf("%s has no gap to draw", tc.key)
		}
	}

	// NO GAP AT ALL, and it is the LIVING parent that makes it so: SpanToMs is 0 for
	// a species that is alive, which is the page's whole test for "this bar runs to
	// the edge and every child of it starts inside it".
	live, in := need("Live par"), need("In kid")
	if !live.Alive || live.SpanToMs != 0 {
		t.Fatalf("Live par is %v with an end at %d; a living species has no right-hand end",
			live.Alive, live.SpanToMs)
	}
	if !(in.SpanFromMs > live.SpanFromMs) || in.Parent != "Live par" {
		t.Fatalf("In kid at %d under %q does not start inside its living parent's span (%d)",
			in.SpanFromMs, in.Parent, live.SpanFromMs)
	}

	// THE INVERSION, on the record's own numbers, with neither end inferred.
	late, early := need("Late par"), need("Early kid")
	if early.Parent != "Late par" {
		t.Fatalf("Early kid hangs off %q, want Late par", early.Parent)
	}
	if !(early.SpanFromMs < late.SpanFromMs) {
		t.Fatalf("Early kid begins at %d and its parent at %d; the fixture no longer holds a "+
			"child the record saw first", early.SpanFromMs, late.SpanFromMs)
	}
	if late.SpanFromMs-early.SpanFromMs != 4*hour {
		t.Fatalf("the inversion is %d ms, want %d", late.SpanFromMs-early.SpanFromMs, 4*hour)
	}
	if early.SpanDerived || late.SpanDerived {
		t.Fatal("one end of the inversion is a DERIVED span; that machinery fires only where a " +
			"node's own record holds no crossing, and it already takes the earliest dated " +
			"descendant. An inversion is the opposite case and must reach the page as itself")
	}

	// AND ALL OF IT SURVIVES THE WIRE, because a distinction a client cannot see is
	// not one it can draw.
	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)
	var served SpeciesTree
	if err := json.Unmarshal([]byte(get(t, srv.URL+"/api/species/tree")), &served); err != nil {
		t.Fatalf("decode: %v", err)
	}
	w := treeNodes(t, served)
	if w["Gap far"].SpanFromMs-w["Old root"].SpanToMs != 38*hour ||
		w["Late par"].SpanFromMs-w["Early kid"].SpanFromMs != 4*hour ||
		w["Live par"].SpanToMs != 0 {
		t.Fatalf("the shapes did not survive the wire: %+v", w)
	}
}

// TestAnUndatedLivingSpeciesHasNoBarAndSaysSo is the fourth shape of the same
// rule, and the one that must not be filled in. A species alive right now that
// has never crossed a lane has no recorded crossing at all — so the record dates
// neither end of it, and the honest layout datum is ZERO rather than "now" or
// "the beginning of the axis".
func TestAnUndatedLivingSpeciesHasNoBarAndSaysSo(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(20, 0,
			entry("Homebody", "one", 10, 0), entry("Beta", "two", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	a.observeSpeciesLocked(child(base, "Beta", "two", "Alpha", "nullus"))
	a.mu.Unlock()

	byKey := treeNodes(t, a.SpeciesTreeView())
	home := byKey["Homebody one"]
	if home.SpanFromMs != 0 || home.SpanToMs != 0 || home.SpanDerived {
		t.Fatalf("a species the record has never dated was given a bar: %+v", home)
	}
	if !home.Alive || !home.Isolated || home.AncestryKnown {
		t.Fatalf("the undated species lost the rest of its answer: %+v", home)
	}
}

// TestTheMergedViewCarriesTheCensusFactsOnItsLeaves is the merge itself.
//
// This view IS the species list now, so a living leaf has to carry what the flat
// census tab carried — the per-world counts, the eggs, the badges, the other
// spellings, the recent lanes and the parent the record named. A merge that
// dropped them would be a merge that lost half of what it merged, and the reader
// would be back to holding one tab in their head while reading another.
//
// AND AN ANCESTOR CARRIES NONE OF IT, which is §10.1's rule on the node type
// most easily misread as a resident: it has no population because it has none.
func TestTheMergedViewCarriesTheCensusFactsOnItsLeaves(t *testing.T) {
	one := slot(1, 0, 0, true, census(30, 4,
		entry("Beta ", "one", 20, 3), entry("Gamma", "two", 10, 1)))
	one.Stats.MigrationExclude = &wire.ExcludeList{Names: []string{"Beta one"}}
	two := slot(2, 1, 0, true, census(5, 1, entry("Beta", "one", 5, 1)))
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{one, two},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	a.observeSpeciesLocked(child(base, "Beta", "one", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+1000, "Gamma", "two", "Alpha", "nullus"))
	a.mu.Unlock()

	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)
	var view SpeciesTree
	if err := json.Unmarshal([]byte(get(t, srv.URL+"/api/species/tree")), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byKey := treeNodes(t, view)

	beta := byKey["Beta one"]
	if beta.Population != 25 || beta.Eggs != 4 {
		t.Fatalf("the leaf lost the census union's totals: %+v", beta)
	}
	// PER WORLD, with the counts — this is what the row's expanded detail draws
	// and what its mini-map is filled from.
	if len(beta.Worlds) != 2 {
		t.Fatalf("the leaf carries %d worlds, want 2: %+v", len(beta.Worlds), beta.Worlds)
	}
	if beta.Worlds[0].Slot != 1 || beta.Worlds[0].Bibites != 20 || beta.Worlds[0].Eggs != 3 {
		t.Fatalf("world 1's own counts are missing: %+v", beta.Worlds[0])
	}
	if beta.Worlds[1].Slot != 2 || beta.Worlds[1].Bibites != 5 {
		t.Fatalf("world 2's own counts are missing: %+v", beta.Worlds[1])
	}
	// The three badges of §19 B19, and the second raw spelling, which is a real
	// difference between two worlds rather than noise to tidy away.
	if !beta.Everywhere || beta.Endemic {
		t.Fatalf("the leaf lost its everywhere/endemic badges: %+v", beta)
	}
	if !beta.Excluded || len(beta.ExcludedBy) != 1 || beta.ExcludedBy[0] != 1 {
		t.Fatalf("the leaf lost the exclusion that explains why it never travels: %+v", beta)
	}
	if len(beta.Spellings) != 1 || beta.Spellings[0] != "Beta one" {
		t.Fatalf("the leaf lost the other world's spelling of it: %+v", beta.Spellings)
	}
	if beta.Name != "Beta  one" {
		t.Fatalf("the raw spelling was repaired: %q", beta.Name)
	}
	if len(beta.Recent) == 0 || beta.Recent[0].FromSlot != 1 {
		t.Fatalf("the leaf lost the sample of lanes it uses: %+v", beta.Recent)
	}
	if beta.ParentName != "Alpha nullus" {
		t.Fatalf("the leaf lost the parent the record named: %q", beta.ParentName)
	}
	gamma := byKey["Gamma two"]
	if !gamma.Endemic || gamma.Everywhere || gamma.Excluded {
		t.Fatalf("a one-world species is not endemic here: %+v", gamma)
	}

	alpha := byKey["Alpha nullus"]
	if alpha.Alive || alpha.Population != 0 || len(alpha.Worlds) != 0 ||
		len(alpha.Recent) != 0 || alpha.Everywhere || alpha.Endemic {
		t.Fatalf("an extinct ancestor is dressed as a resident: %+v", alpha)
	}
	// The whole-view counts the merged tab prints where the flat one used to.
	if view.LedgerRecords != 0 && view.LedgerSpecies == 0 {
		t.Fatalf("the merged view lost the record's own size: %+v", view)
	}
}

// TestTheMiniMapGridIsTheMapsOwnShape covers the six dots beside each living
// species — and the reason they are published rather than assumed.
//
// SIX DOTS IN TWO ROWS BECAUSE THIS MAP IS THREE BY TWO, never because there
// happen to be six worlds. The grid is the map's, from the same status frame the
// leaves came from, and the three states a dot can have are three different
// facts: alive there, reported-and-absent, and a world reporting no census at
// all — which is unknown and never an absence (§10.1).
func TestTheMiniMapGridIsTheMapsOwnShape(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 3, Height: 2}, SlotCount: 5,
		Slots: []contractb.SlotInfo{
			slot(1, 0, 0, true, census(10, 0, entry("Beta", "one", 10, 0))),
			slot(2, 1, 0, true, census(5, 0, entry("Gamma", "two", 5, 0))),
			// Reporting stats and NO census: unknown in every dot of its column.
			slot(3, 2, 0, true, &contractb.PeerStats{Population: contractb.IntPtr(7)}),
			slot(4, 0, 1, true, census(3, 0, entry("Beta", "one", 3, 0))),
			// Dark, and still a seat on the map.
			slot(5, 1, 1, false, nil),
			// (2,1) is a HOLE: no world has claimed it, and it is drawn as nothing.
		},
	}
	a := newViewFixture(t, status, time.Second)
	view := a.SpeciesTreeView()

	if view.Map.Width != 3 || view.Map.Height != 2 {
		t.Fatalf("the grid is %dx%d, want the map's own 3x2", view.Map.Width, view.Map.Height)
	}
	if len(view.Map.Cells) != 5 {
		t.Fatalf("the grid holds %d cells, want the five seats — a hole is drawn as nothing, "+
			"not as an empty dot: %+v", len(view.Map.Cells), view.Map.Cells)
	}
	byslot := map[int]TreeCell{}
	for _, c := range view.Map.Cells {
		byslot[c.Slot] = c
	}
	if c := byslot[4]; c.Col != 0 || c.Row != 1 {
		t.Fatalf("slot 4 sits at (%d,%d), want the map's (0,1)", c.Col, c.Row)
	}
	if !byslot[1].Reporting || !byslot[1].Live {
		t.Fatalf("a reporting world is not marked as one: %+v", byslot[1])
	}
	// A world with stats and no census is NOT reporting a census, and the page
	// draws that dot as unknown rather than as "this species is not there".
	if byslot[3].Reporting {
		t.Fatalf("a world reporting no census is marked as reporting: %+v", byslot[3])
	}
	if byslot[5].Reporting || byslot[5].Live {
		t.Fatalf("a dark world is not marked dark: %+v", byslot[5])
	}

	// And the dots a species fills are its own census worlds, by slot.
	beta := treeNodes(t, view)["Beta one"]
	if len(beta.Worlds) != 2 || beta.Worlds[0].Slot != 1 || beta.Worlds[1].Slot != 4 {
		t.Fatalf("the leaf's worlds are not the ones holding it: %+v", beta.Worlds)
	}
}

// TestTheBrainShapeComesFromOneStoredGenomePerSpecies is the genealogy's newest
// figure and the one with the most ways to be wrong.
//
// THE HASH IS TRACKED, NOT SEARCHED FOR. species.go keeps the LATEST genome hash
// each species' crossings carried — one string, maintained by the same single
// writer as every other counter there — so this view names a genome without
// reading the ledger. The blob behind it is parsed ONCE PER HASH and never per
// request.
//
// AND AN ABSENCE IS AN ABSENCE. A species whose blob the store does not hold —
// pruned past the retention horizon, or never fetched — carries no figures at
// all. Never a zero, never an error, and never a view that fails because a
// genome aged out.
func TestTheBrainShapeComesFromOneStoredGenomePerSpecies(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(30, 0,
			entry("Beta", "one", 10, 0), entry("Gamma", "two", 10, 0),
			entry("Ghostly", "three", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()

	small := bb8.HashPrefix + strings.Repeat("a", 64)
	big := bb8.HashPrefix + strings.Repeat("b", 64)
	gone := bb8.HashPrefix + strings.Repeat("c", 64)
	store := func(hash string, neurons, synapses int) {
		t.Helper()
		nodes, syn := "", ""
		for i := 0; i < neurons; i++ {
			if i > 0 {
				nodes += ","
			}
			nodes += `{"Index":` + strconv.Itoa(i) + `,"Type":0}`
		}
		for i := 0; i < synapses; i++ {
			if i > 0 {
				syn += ","
			}
			syn += `{"NodeIn":0,"NodeOut":1,"Inov":` + strconv.Itoa(i) + `,"Weight":1.0}`
		}
		blob := `{"genes":{"SizeRatio":1.0},"nodes":[` + nodes + `],"synapses":[` + syn +
			`],"version":"0.6.3.1"}`
		if err := a.genomes.Put(hash, "0.6.3.1", blob); err != nil {
			t.Fatalf("store %s: %v", hash, err)
		}
	}
	store(small, 3, 2)
	store(big, 40, 90)

	a.mu.Lock()
	// Beta's LATEST genome is the big one; the older, smaller one must not win.
	a.observeSpeciesLocked(migration(base, 1, 2, "E", "Beta", "one", small))
	a.observeSpeciesLocked(migration(base+1000, 1, 2, "E", "Beta", "one", big))
	a.observeSpeciesLocked(migration(base+2000, 1, 2, "E", "Gamma", "two", small))
	// Ghostly's genome is one the store does not hold.
	a.observeSpeciesLocked(migration(base+3000, 1, 2, "E", "Ghostly", "three", gone))
	a.mu.Unlock()

	byKey := treeNodes(t, a.SpeciesTreeView())
	if n := byKey["Beta one"]; n.Neurons != 40 || n.Synapses != 90 {
		t.Fatalf("Beta's brain is %d/%d, want the LATEST genome's 40/90", n.Neurons, n.Synapses)
	}
	if n := byKey["Gamma two"]; n.Neurons != 3 || n.Synapses != 2 {
		t.Fatalf("Gamma's brain is %d/%d, want 3/2", n.Neurons, n.Synapses)
	}
	ghost := byKey["Ghostly three"]
	if ghost.Neurons != 0 || ghost.Synapses != 0 {
		t.Fatalf("a species whose genome the store does not hold reported a brain: %+v", ghost)
	}
	if ghost.Population != 10 || ghost.Crossings != 1 {
		t.Fatalf("the missing blob cost the row the rest of its answer: %+v", ghost)
	}

	// PARSED ONCE PER HASH, EVER. The blob is removed from the store and the same
	// answer comes back — which is the property that keeps a two-second poll off
	// the disk, and the reason the cache is keyed on content.
	hex := bb8.HashHex(big)
	if err := os.Remove(filepath.Join(a.genomes.Dir(), hex[:2], hex+".json")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if n := treeNodes(t, a.SpeciesTreeView())["Beta one"]; n.Neurons != 40 {
		t.Fatalf("the brain shape was re-read from the store on a second poll: %+v", n)
	}
	// And a hash the store never held is cached as an absence rather than
	// stat()ed on every poll.
	a.brains.mu.Lock()
	e, cached := a.brains.byHash[gone]
	a.brains.mu.Unlock()
	if !cached || e.known {
		t.Fatalf("a missing blob is not remembered as missing (%v/%+v); it would be looked up "+
			"again every two seconds forever", cached, e)
	}
}

// TestTheFlatIndexOutlivesTheTabItDrew is the removal's other half.
//
// The page's census tab is gone and /api/species is NOT: ringstat reads it
// (cmd/ringstat, FetchSpecies), a terminal table is a different medium with a
// different answer, and an endpoint is a contract with whoever is calling it
// rather than a private detail of one renderer. So the flat index keeps its
// shape, keeps its ledger annotations, and the terminal view still renders from
// it.
func TestTheFlatIndexOutlivesTheTabItDrew(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{
			slot(1, 0, 0, true, census(20, 2, entry("Beta", "one", 20, 2))),
			slot(2, 1, 0, true, census(5, 0, entry("Beta", "one", 5, 0))),
		},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	a.observeSpeciesLocked(child(base, "Beta", "one", "Alpha", "nullus"))
	a.mu.Unlock()

	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)
	idx, err := FetchSpecies(srv.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("FetchSpecies: %v", err)
	}
	if len(idx.Species) != 1 || idx.Species[0].Population != 25 || idx.Species[0].Crossings != 1 {
		t.Fatalf("the flat index lost its shape when the tab that drew it went: %+v", idx.Species)
	}
	if idx.Species[0].Parent != "Alpha nullus" || len(idx.Species[0].Worlds) != 2 {
		t.Fatalf("the flat index lost its annotations: %+v", idx.Species[0])
	}
	// And the terminal renderer still draws from it.
	var out strings.Builder
	RenderSpecies(&out, a.StatusView(), &idx)
	if !strings.Contains(out.String(), "Beta one") {
		t.Fatalf("ringstat's species table no longer renders:\n%s", out.String())
	}
}

// TestSeedStockIsTheSpeciesEveryWorldHoldingItRefusesToExport is the seed-template
// rule, and it is a RULE rather than a name on purpose.
//
// The game seeds a world with a starting species, and the operator's policy
// (contract-a.md §19 A39) puts that species on every world's migrationExclude —
// so it is alive everywhere, travels nowhere, and has no living descendant here.
// A merged view that draws it draws a full-width bar that participates in nothing
// else on the picture. The view needs to be able to leave it out, and the only
// honest way to name it is by what makes it what it is: EVERY WORLD HOLDING IT
// REFUSES TO EXPORT IT.
//
// THE THREE SHAPES THAT MUST NOT BE CONFUSED, all of them in this fixture:
//
//	Excluded EVERYWHERE IT LIVES — seed stock.
//
//	Excluded by SOME world that holds it — not seed stock, and this is the common
//	case: a species one world holds back travels freely out of another (§19 A42).
//
//	ALIVE NOWHERE — an ancestor. Never seed stock, whatever any list says: an
//	empty set of holders is not a unanimous one, and the branch points this view
//	exists to draw would be the first thing such a reading deleted.
func TestSeedStockIsTheSpeciesEveryWorldHoldingItRefusesToExport(t *testing.T) {
	one := slot(1, 0, 0, true, census(60, 0,
		entry("Basic", "bibite", 40, 0), entry("Beta", "one", 20, 0)))
	one.Stats.MigrationExclude = &wire.ExcludeList{Names: []string{"Basic bibite", "Beta one"}}
	two := slot(2, 1, 0, true, census(40, 0,
		entry("Basic", "bibite", 30, 0), entry("Beta", "one", 10, 0)))
	two.Stats.MigrationExclude = &wire.ExcludeList{Names: []string{"Basic bibite"}}
	// A world that publishes NO exclusion list at all. It holds neither of the two
	// above, so it says nothing about either — but it is here because the next
	// phase moves a species into it.
	three := slot(3, 2, 0, true, census(5, 0, entry("Gamma", "two", 5, 0)))

	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 3, Height: 1}, SlotCount: 3,
		Slots: []contractb.SlotInfo{one, two, three},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()
	a.mu.Lock()
	// Beta and Gamma part at Alpha, which is alive nowhere: the ancestor case.
	a.observeSpeciesLocked(child(base+1000, "Beta", "one", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+2000, "Gamma", "two", "Alpha", "nullus"))
	a.observeSpeciesLocked(migration(base+3000, 1, 2, "E", "Basic", "bibite", "h-basic"))
	a.mu.Unlock()

	byKey := treeNodes(t, a.SpeciesTreeView())
	view := a.SpeciesTreeView()
	for _, tc := range []struct {
		key  string
		seed bool
		why  string
	}{
		{"Basic bibite", true, "it is alive in S1 and S2 and both of them refuse to export it"},
		{"Beta one", false, "S1 excludes it and S2 does not, so it can still leave S2"},
		{"Gamma two", false, "no world it lives in has published an exclusion list at all"},
		{"Alpha nullus", false, "it is alive nowhere; an ancestor is never seed stock"},
	} {
		n, ok := byKey[tc.key]
		if !ok {
			t.Fatalf("%s is not in the tree at all", tc.key)
		}
		if n.SeedStock != tc.seed {
			t.Fatalf("%s: seedStock = %v, want %v — %s", tc.key, n.SeedStock, tc.seed, tc.why)
		}
	}
	// EXCLUDED AND SEED STOCK ARE TWO DIFFERENT FACTS, and the weaker one survives
	// on the row that carries the stronger.
	if basic := byKey["Basic bibite"]; !basic.Excluded || len(basic.ExcludedBy) != 2 {
		t.Fatalf("the seed species lost the exclusion badge it is built from: %+v", basic)
	}
	// COUNTED, NOT DISAPPEARED. It is one of the living species, it is in Alive,
	// and the count of seed stock is published beside it so a renderer that leaves
	// the row out can say how many it left out.
	if view.SeedStock != 1 {
		t.Fatalf("seedStock count = %d, want 1", view.SeedStock)
	}
	if view.Alive != 3 {
		t.Fatalf("alive = %d, want 3 — hiding a row is a renderer's business and never a "+
			"count's", view.Alive)
	}

	// ONE RULE, EVALUATED ONCE. The flat index /api/species is where the exclusion
	// lists and the census union meet, so the answer is computed there and the tree
	// copies it — and the two views cannot disagree about which species can never
	// leave.
	idx := a.SpeciesIndexView()
	for _, row := range idx.Species {
		if row.SeedStock != byKey[row.Key].SeedStock {
			t.Fatalf("%s reads seedStock=%v on the flat index and %v on the tree; the rule is "+
				"evaluated in one place", row.Key, row.SeedStock, byKey[row.Key].SeedStock)
		}
	}

	// AND ON THE WIRE, because a mark a client cannot see is not one — and because
	// the hiding is a VIEW's decision: the node travels complete, so revealing it
	// costs a repaint and never a second request.
	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)
	var served SpeciesTree
	if err := json.Unmarshal([]byte(get(t, srv.URL+"/api/species/tree")), &served); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sn := treeNodes(t, served)["Basic bibite"]
	if !sn.SeedStock || served.SeedStock != 1 {
		t.Fatalf("the seed mark did not survive the wire: node %+v, count %d", sn, served.SeedStock)
	}
	if sn.Population != 70 || sn.Crossings != 1 {
		t.Fatalf("the seed node travels short of the rest of its answer: %+v", sn)
	}
	if !strings.Contains(get(t, srv.URL+"/api/species"), `"seedStock":true`) {
		t.Fatal("the flat index does not publish the seed mark; ringstat and any other reader " +
			"of it would have to re-derive a policy from an exclusion list")
	}

	// THE SAME SPECIES IN ONE MORE WORLD, and that world has published no
	// exclusion list. Unknown is not a refusal (§10.1), so the unanimity is gone
	// and so is the mark.
	three.Stats = census(35, 0, entry("Gamma", "two", 5, 0), entry("Basic", "bibite", 30, 0))
	wider := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 3, Height: 1}, SlotCount: 3,
		Slots: []contractb.SlotInfo{one, two, three},
	}
	b := newViewFixture(t, wider, time.Second)
	for _, row := range b.SpeciesIndexView().Species {
		if row.Key == "Basic bibite" && row.SeedStock {
			t.Fatalf("a world that has told us nothing about its exclusions was read as a world "+
				"that excludes: %+v", row)
		}
	}
}

// TestTheAxisIsFittedToTheSpeciesItDraws is the geometry half of the same
// decision, and it is the reason hiding the row is worth anything at all.
//
// A seed species has been in the record since the record began. If its bar sets
// the left edge, every bar that IS drawn is squeezed into the right-hand sliver of
// a picture scaled to a species nobody is looking at. So the published axis is the
// axis of the DRAWN rows and of nothing else — the record's ancestry floor does
// not pin itself inside it either, for the same reason and with the same measured
// cost (TestTheAxisStartsAtTheOldestDrawnBar).
//
// AND THE WIDER AXIS IS PUBLISHED TOO. Revealing the seed stock has to stretch the
// drawing rather than clamp a bar against its left edge, and it has to do that
// from the answer the page is already holding.
func TestTheAxisIsFittedToTheSpeciesItDraws(t *testing.T) {
	excl := &wire.ExcludeList{Names: []string{"Basic bibite"}}
	one := slot(1, 0, 0, true, census(50, 0,
		entry("Basic", "bibite", 40, 0), entry("Beta", "one", 10, 0)))
	one.Stats.MigrationExclude = excl
	two := slot(2, 1, 0, true, census(20, 0,
		entry("Basic", "bibite", 15, 0), entry("Beta", "one", 5, 0)))
	two.Stats.MigrationExclude = excl
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 2, Height: 1}, SlotCount: 2,
		Slots: []contractb.SlotInfo{one, two},
	}
	a := newViewFixture(t, status, time.Second)

	hour := time.Hour.Milliseconds()
	base := time.Now().UnixMilli() - 6*hour
	a.mu.Lock()
	// The seed species crossed first, six hours ago, and nothing else here is
	// anywhere near that old.
	a.observeSpeciesLocked(migration(base, 1, 2, "E", "Basic", "bibite", "h-basic"))
	// The record's ancestry floor, two hours later, carried by a species that is
	// drawn nowhere.
	a.observeSpeciesLocked(child(base+2*hour, "Dead", "end", "Long", "gone"))
	// And the one species this drawing is actually about.
	a.observeSpeciesLocked(migration(base+4*hour, 1, 2, "E", "Beta", "one", "h-beta"))
	a.mu.Unlock()

	view := a.SpeciesTreeView()
	if view.AncestrySinceMs != base+2*hour {
		t.Fatalf("ancestrySinceMs = %d, want %d", view.AncestrySinceMs, base+2*hour)
	}
	if view.SpanStartMs != base+4*hour {
		t.Fatalf("spanStartMs = %d, want the one drawn bar's own start at %d: the seed species' "+
			"six-hour-old bar must not set the left edge of a picture it is not drawn on, and "+
			"neither must the floor, which is drawn nowhere at all", view.SpanStartMs, base+4*hour)
	}
	if view.SpanStartSeedMs != base {
		t.Fatalf("spanStartSeedMs = %d, want the seed bar's own start at %d — without it a "+
			"reader who reveals the row gets a bar clamped against the left edge",
			view.SpanStartSeedMs, base)
	}
	if !(view.SpanStartSeedMs < view.SpanStartMs) {
		t.Fatal("the two axes are the same; the fixture no longer has a seed species older " +
			"than everything drawn, which is the whole shape under test")
	}
	// THE NODE IS STILL THERE AND STILL DATED. Nothing is dropped server-side: the
	// bar the page may reveal is in the answer, complete.
	if n := treeNodes(t, view)["Basic bibite"]; !n.SeedStock || n.SpanFromMs != base {
		t.Fatalf("the seed node lost its bar to the axis it was kept out of: %+v", n)
	}

	// AND THE FLOOR IS OUTSIDE THE AXIS, which is the ordinary case and the reason
	// the page captions it at the margin instead of drawing it: two hours of empty
	// plot here, days of it on the running rig, and more of it every hour the map
	// runs.
	if !(view.AncestrySinceMs < view.SpanStartMs) {
		t.Fatalf("the floor (%d) is not older than the left edge (%d); the fixture no longer "+
			"produces the shape the caption exists for", view.AncestrySinceMs, view.SpanStartMs)
	}

	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)
	var served SpeciesTree
	if err := json.Unmarshal([]byte(get(t, srv.URL+"/api/species/tree")), &served); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if served.SpanStartMs != view.SpanStartMs || served.SpanStartSeedMs != view.SpanStartSeedMs {
		t.Fatalf("the two axes did not survive the wire: %d / %d",
			served.SpanStartMs, served.SpanStartSeedMs)
	}
}

// TestTheAxisStartsAtTheOldestDrawnBar is the axis rule itself, and it exists
// because the rule it replaced was measured on the running rig and found to be
// paying for a mark nobody could read.
//
// THE OLD RULE clamped both published edges down to AncestrySinceMs, the record's
// ancestry floor, so the floor was always inside the picture and could be drawn as
// a dashed boundary with a shaded run behind it. The floor is 2026-08-07 there and
// the oldest DRAWN bar was days younger, so about two thirds of the plot was a
// stretch with nothing in it — and the share grew every hour the map ran, because
// one edge is a fixed date and the other travels with the record.
//
// THE NEW RULE is that the drawing fits the living record: the left edge is the
// oldest drawn bar and nothing else. The floor is still published, still counted
// on the stat line, and the page captions it at the left margin — a fact in words
// where it was a boundary made of emptiness.
//
// ALL THREE POSITIONS OF THE FLOOR ARE HERE, because the rule has to be the same
// rule in each: older than every drawn bar (the ordinary case, and the caption's),
// exactly equal to the oldest (where a boundary line would land on the axis's own
// edge and mark nothing), and inside the drawn span (where it is a real boundary
// and the page still draws it — a species whose first crossing named no parent can
// predate the oldest crossing that named one).
func TestTheAxisStartsAtTheOldestDrawnBar(t *testing.T) {
	hour := time.Hour.Milliseconds()
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(30, 0,
			entry("Beta", "one", 20, 0), entry("Gamma", "two", 10, 0)))},
	}
	for _, tc := range []struct {
		name string
		// records is what the ledger holds, and floor/start what the view must
		// publish, both as offsets in hours from the fixture's base.
		records     func(base int64) []Record
		floorH      int64
		startH      int64
		floorInside bool
	}{
		{
			// THE ORDINARY CASE. The oldest ancestry the record holds belongs to a
			// species with no living descendant — drawn nowhere — and it is an hour
			// older than anything on the picture. The axis does not reach back for it.
			name: "the floor is older than every drawn bar",
			records: func(base int64) []Record {
				return []Record{
					child(base+1*hour, "Dead", "end", "Long", "gone"),
					migration(base+2*hour, 1, 2, "E", "Beta", "one", "h-beta"),
					migration(base+3*hour, 1, 2, "E", "Gamma", "two", "h-gamma"),
				}
			},
			floorH: 1, startH: 2,
		},
		{
			// EXACTLY EQUAL. Beta's own first crossing is the first crossing that
			// ever named a parent, so the floor and the left edge are one instant.
			// A boundary drawn here sits on the axis's own left edge and separates
			// nothing from nothing, which is why the page's line is gated on a
			// strict > and the caption carries this case instead.
			name: "the floor is the oldest drawn bar",
			records: func(base int64) []Record {
				return []Record{
					child(base+2*hour, "Beta", "one", "Alpha", "nullus"),
					migration(base+3*hour, 1, 2, "E", "Gamma", "two", "h-gamma"),
				}
			},
			floorH: 2, startH: 2,
		},
		{
			// INSIDE THE SPAN. Beta crossed an hour before anything named a parent
			// at all, so its bar starts left of the floor. This is the one shape
			// where the floor is a boundary a reader can see — left of it, no line
			// in this drawing can begin — and the page still draws it there.
			name: "the floor is inside the drawn span",
			records: func(base int64) []Record {
				return []Record{
					migration(base+1*hour, 1, 2, "E", "Beta", "one", "h-beta"),
					child(base+2*hour, "Dead", "end", "Long", "gone"),
					migration(base+3*hour, 1, 2, "E", "Gamma", "two", "h-gamma"),
				}
			},
			floorH: 2, startH: 1, floorInside: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newViewFixture(t, status, time.Second)
			base := time.Now().UnixMilli() - 6*hour
			a.mu.Lock()
			for _, rec := range tc.records(base) {
				a.observeSpeciesLocked(rec)
			}
			a.mu.Unlock()

			view := a.SpeciesTreeView()
			if view.AncestrySinceMs != base+tc.floorH*hour {
				t.Fatalf("ancestrySinceMs = %d, want %d — the fixture no longer places the "+
					"floor where this case is about", view.AncestrySinceMs, base+tc.floorH*hour)
			}
			if view.SpanStartMs != base+tc.startH*hour {
				t.Fatalf("spanStartMs = %d, want the oldest drawn bar at %d",
					view.SpanStartMs, base+tc.startH*hour)
			}
			// THE FLOOR NEVER MOVES THE EDGE, in any of the three positions. That is
			// the whole rule, and each case would have failed differently under the
			// clamp: the first by an hour, the third not at all.
			if (view.SpanStartMs < view.AncestrySinceMs) != tc.floorInside {
				t.Fatalf("the floor at %d and the left edge at %d are not the arrangement "+
					"this case is about (inside = %v)",
					view.AncestrySinceMs, view.SpanStartMs, tc.floorInside)
			}
			// AND WITH NO SEED STOCK ON THE MAP THE TWO EDGES ARE ONE. The revealed
			// axis holds a superset of the same bars and there is nothing extra to
			// hold, so a difference here would be a floor pulling one edge and not
			// the other — the exact defect the clamp used to hide.
			if view.SpanStartSeedMs != view.SpanStartMs {
				t.Fatalf("spanStartSeedMs = %d and spanStartMs = %d with no seed species on "+
					"the map", view.SpanStartSeedMs, view.SpanStartMs)
			}
			if view.SpanEndMs != view.GeneratedAtMs || view.SpanStartMs >= view.SpanEndMs {
				t.Fatalf("the axis is not %d..now: %d..%d",
					view.SpanStartMs, view.SpanStartMs, view.SpanEndMs)
			}
		})
	}
}

// TestTheRevealedAxisIsTheSeedBarAndNotTheFloor is the other half of the clamp's
// removal, and the half that is easy to leave behind.
//
// SpanStartSeedMs exists so that revealing the seed stock STRETCHES the axis from
// the answer already in hand. It was clamped down to the floor as well — so on a
// map whose floor is older than the seed species' own first crossing, revealing the
// row produced an axis reaching back to a date no bar on it reaches, which is the
// same empty stretch the drawn axis had, in the picture that was supposed to be
// fitted to the seed bar.
func TestTheRevealedAxisIsTheSeedBarAndNotTheFloor(t *testing.T) {
	excl := &wire.ExcludeList{Names: []string{"Basic bibite"}}
	one := slot(1, 0, 0, true, census(50, 0,
		entry("Basic", "bibite", 40, 0), entry("Beta", "one", 10, 0)))
	one.Stats.MigrationExclude = excl
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{one},
	}
	a := newViewFixture(t, status, time.Second)

	hour := time.Hour.Milliseconds()
	base := time.Now().UnixMilli() - 8*hour
	a.mu.Lock()
	// THE FLOOR IS THE OLDEST THING HERE, and it belongs to a species drawn on
	// neither picture.
	a.observeSpeciesLocked(child(base, "Dead", "end", "Long", "gone"))
	// The seed species, older than everything drawn and younger than the floor.
	a.observeSpeciesLocked(migration(base+2*hour, 1, 2, "E", "Basic", "bibite", "h-basic"))
	// And the one row a default reader sees.
	a.observeSpeciesLocked(migration(base+5*hour, 1, 2, "E", "Beta", "one", "h-beta"))
	a.mu.Unlock()

	view := a.SpeciesTreeView()
	if view.AncestrySinceMs != base {
		t.Fatalf("ancestrySinceMs = %d, want %d", view.AncestrySinceMs, base)
	}
	if view.SpanStartMs != base+5*hour {
		t.Fatalf("spanStartMs = %d, want the drawn bar at %d", view.SpanStartMs, base+5*hour)
	}
	if view.SpanStartSeedMs != base+2*hour {
		t.Fatalf("spanStartSeedMs = %d, want the seed bar's own start at %d — the revealed "+
			"axis is fitted to the rows it reveals, and the floor is two hours older than the "+
			"oldest of them", view.SpanStartSeedMs, base+2*hour)
	}
	if !(view.AncestrySinceMs < view.SpanStartSeedMs) {
		t.Fatal("the fixture no longer has a floor older than the seed bar, which is the one " +
			"shape that tells the clamp apart from the rule")
	}
	// AND THE REVEALED AXIS IS STILL THE WIDER OF THE TWO, which is the invariant
	// the page's reveal depends on: it stretches, never clamps a bar against an edge.
	if !(view.SpanStartSeedMs < view.SpanStartMs) {
		t.Fatalf("the revealed axis (%d) is not wider than the drawn one (%d)",
			view.SpanStartSeedMs, view.SpanStartMs)
	}
}

// TestTheBrainRingKeepsTheNewestShapeItCouldRead is the ring's own honesty rule,
// corrected by what the running rig does to it.
//
// The record names a LATEST genome per species and the store holds a copy of some
// of them. With a fetch backlog — 154 000 genome gaps when this was measured — an
// actively-travelling species rotates its latest hash faster than the archive
// fetches the blob behind it, so a ring drawn strictly from the latest hash
// BLINKED: present on one poll, gone on the next, back on the third. That reads as
// a brain that comes and goes rather than as a fetch that has not landed.
//
// So the figure is the newest genome of that species this archive HAS BEEN ABLE TO
// READ, which is what the page's tooltip and the glossary have always called it.
// An absence is still an absence: a species no readable genome has ever been held
// for draws nothing at all.
func TestTheBrainRingKeepsTheNewestShapeItCouldRead(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(20, 0,
			entry("Beta", "one", 10, 0), entry("Ghostly", "three", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()

	held := bb8.HashPrefix + strings.Repeat("a", 64)
	pending := bb8.HashPrefix + strings.Repeat("b", 64)
	never := bb8.HashPrefix + strings.Repeat("c", 64)
	blob := `{"genes":{"SizeRatio":1.0},"nodes":[{"Index":0,"Type":0},{"Index":1,"Type":0},` +
		`{"Index":2,"Type":0}],"synapses":[{"NodeIn":0,"NodeOut":1,"Inov":0,"Weight":1.0},` +
		`{"NodeIn":1,"NodeOut":2,"Inov":1,"Weight":1.0}],"version":"0.6.3.1"}`
	if err := a.genomes.Put(held, "0.6.3.1", blob); err != nil {
		t.Fatalf("store: %v", err)
	}

	a.mu.Lock()
	a.observeSpeciesLocked(migration(base, 1, 2, "E", "Beta", "one", held))
	a.observeSpeciesLocked(migration(base+1000, 1, 2, "E", "Ghostly", "three", never))
	a.mu.Unlock()
	if n := treeNodes(t, a.SpeciesTreeView())["Beta one"]; n.Neurons != 3 || n.Synapses != 2 {
		t.Fatalf("Beta's brain is %d/%d, want the stored genome's 3/2", n.Neurons, n.Synapses)
	}

	// A NEWER CROSSING WHOSE BLOB HAS NOT ARRIVED. The record's latest hash moves;
	// the readable answer does not, and the ring stays put.
	a.mu.Lock()
	a.observeSpeciesLocked(migration(base+2000, 1, 2, "E", "Beta", "one", pending))
	a.mu.Unlock()
	n := treeNodes(t, a.SpeciesTreeView())["Beta one"]
	if n.Neurons != 3 || n.Synapses != 2 {
		t.Fatalf("the ring blinked out when the record named a genome the store has not "+
			"fetched yet: %d/%d, want 3/2", n.Neurons, n.Synapses)
	}
	// AND AN ABSENCE IS STILL AN ABSENCE. Nothing is invented for a species no
	// readable genome was ever held for.
	if g := treeNodes(t, a.SpeciesTreeView())["Ghostly three"]; g.Neurons != 0 || g.Synapses != 0 {
		t.Fatalf("a species with no readable genome was given a brain: %+v", g)
	}

	// AND THE LAST-KNOWN SHAPE NEVER FREEZES THE ANSWER. The next genome the store
	// can actually give up wins immediately: this is a fallback for a hash whose
	// blob is not here, not a preference for old readings.
	newer := bb8.HashPrefix + strings.Repeat("d", 64)
	if err := a.genomes.Put(newer, "0.6.3.1",
		`{"genes":{"SizeRatio":1.0},"nodes":[{"Index":0,"Type":0}],"synapses":[],`+
			`"version":"0.6.3.1"}`); err != nil {
		t.Fatalf("store: %v", err)
	}
	a.mu.Lock()
	a.observeSpeciesLocked(migration(base+3000, 1, 2, "E", "Beta", "one", newer))
	a.mu.Unlock()
	if n := treeNodes(t, a.SpeciesTreeView())["Beta one"]; n.Neurons != 1 || n.Synapses != 0 {
		t.Fatalf("a readable newer genome did not win: %+v", n)
	}
}
