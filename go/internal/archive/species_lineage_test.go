package archive

import (
	"fmt"
	"testing"
	"time"

	"multiverse/internal/contractb"
)

func TestAReusedNameCreatesANewLineageInstance(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	base := time.Now().Add(-time.Hour).UnixMilli()

	root := migration(base, 1, 2, "E", "Alpha", "one", "hash-root")
	childRec := child(base+1, "Beta", "two", "Alpha", "one")
	reused := child(base+2, "Alpha", "one", "Beta", "two")

	a.mu.Lock()
	a.observeSpeciesLocked(root)
	a.observeSpeciesLocked(childRec)
	a.observeSpeciesLocked(reused)
	lineage := a.species.lineage
	alpha := lineage.byName["Alpha one"]
	current := lineage.bound(1, "Alpha one")
	a.mu.Unlock()

	if len(alpha) != 2 {
		t.Fatalf("Alpha has %d lineage instances, want 2 after the name was reused", len(alpha))
	}
	if current == nil || current.parentKey != "Beta two" {
		t.Fatalf("the current Alpha instance is %+v, want the instance below Beta", current)
	}
	if lineage.wouldCycle(current.id, current.parentID) {
		t.Fatal("the instance fold created a cycle from a reused name")
	}
	betaID := "Beta two"
	if current.id != lineageInstanceID("Alpha one", betaID) {
		t.Fatalf("current Alpha id = %q, want a path-derived instance below Beta", current.id)
	}
}

func TestAReusedNameDoesNotDisconnectLivingCousins(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(20, 0,
			entry("Node", "00", 10, 0), entry("Side", "leaf", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()

	a.mu.Lock()
	a.observeSpeciesLocked(migration(base, 1, 2, "E", "Node", "00", "hash-root"))
	for i := 1; i < 50; i++ {
		a.observeSpeciesLocked(child(base+int64(i), "Node", fmt.Sprintf("%02d", i),
			"Node", fmt.Sprintf("%02d", i-1)))
	}
	// The generator reuses the root's display name after 50 generations.
	a.observeSpeciesLocked(child(base+50, "Node", "00", "Node", "49"))
	a.observeSpeciesLocked(child(base+51, "Side", "leaf", "Node", "25"))
	a.mu.Unlock()

	view := a.SpeciesTreeView()
	if view.CycleGuard != 0 {
		t.Fatalf("the 50-generation name reuse produced %d cycle guard hit(s)", view.CycleGuard)
	}
	if view.SplitNames != 1 {
		t.Fatalf("splitNames = %d, want the one reused root name", view.SplitNames)
	}
	if len(view.Roots) != 1 || view.Connected != 2 {
		t.Fatalf("roots=%v connected=%d, want both living cousins in one family",
			view.Roots, view.Connected)
	}
	root := treeNodes(t, view)[view.Roots[0]]
	if root.NameKey != "Node 25" || root.Leaves != 2 {
		t.Fatalf("the living lines do not meet at their recorded branch point: %+v", root)
	}
}

func TestAnUnseenParentPlaceholderCannotBecomeItsOwnDescendant(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	base := time.Now().Add(-time.Hour).UnixMilli()

	a.mu.Lock()
	a.observeSpeciesLocked(child(base, "Alpha", "one", "Beta", "two"))
	a.observeSpeciesLocked(child(base+1, "Beta", "two", "Alpha", "one"))
	lineage := a.species.lineage
	for _, inst := range lineage.byID {
		if lineage.wouldCycle(inst.id, inst.parentID) {
			a.mu.Unlock()
			t.Fatalf("lineage instance %q points into a cycle", inst.id)
		}
	}
	a.mu.Unlock()

	if len(lineage.byName["Beta two"]) != 2 {
		t.Fatalf("Beta has %d instances, want an unresolved ancestor and a later child instance",
			len(lineage.byName["Beta two"]))
	}
}

func TestAnAmbiguousParentNameStaysUnresolved(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 3, Height: 1}, SlotCount: 3,
		Slots: []contractb.SlotInfo{
			slot(1, 0, 0, true, census(0, 0)),
			slot(2, 1, 0, true, census(0, 0)),
			slot(3, 2, 0, true, census(10, 0, entry("Gamma", "three", 10, 0))),
		},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()

	a.mu.Lock()
	a.observeSpeciesLocked(migration(base, 1, 2, "E", "Alpha", "one", "hash-a"))
	a.observeSpeciesLocked(child(base+1, "Beta", "two", "Alpha", "one"))
	a.observeSpeciesLocked(child(base+2, "Alpha", "one", "Beta", "two"))
	gamma := child(base+3, "Gamma", "three", "Alpha", "one")
	gamma.SourceSlot, gamma.DestSlot = 3, 3
	a.observeSpeciesLocked(gamma)
	gamma.RecordedAt++
	gamma.SourceSlot, gamma.DestSlot = 4, 4
	gamma.Species.ParentSpecificName = " one "
	a.observeSpeciesLocked(gamma)
	alphaInstances := len(a.species.lineage.byName["Alpha one"])
	gammaInstances := len(a.species.lineage.byName["Gamma three"])
	gammaCrossings := a.species.lineage.unique("Gamma three").crossings
	a.mu.Unlock()

	if alphaInstances != 2 {
		t.Fatalf("ambiguous lookup invented a third Alpha instance: got %d", alphaInstances)
	}
	if gammaInstances != 1 || gammaCrossings != 2 {
		t.Fatalf("the same unresolved claim split or reset: instances=%d crossings=%d",
			gammaInstances, gammaCrossings)
	}
	view := a.SpeciesTreeView()
	node := livingTreeNode(t, view, "Gamma three")
	if !node.AncestryUnresolved || node.AncestryKnown || view.Unrecorded != 0 {
		t.Fatalf("ambiguous parent identity was reported as a resolved edge: %+v", node)
	}
}

func TestLineageInstancesHaveABoundedVisibleOverflow(t *testing.T) {
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(10, 0,
			entry("Beta", "two", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()

	a.mu.Lock()
	a.species.lineage.max = 1
	a.observeSpeciesLocked(migration(base, 1, 2, "E", "Alpha", "one", "hash-a"))
	a.observeSpeciesLocked(migration(base+1, 1, 2, "E", "Beta", "two", "hash-b"))
	instances := len(a.species.lineage.byID)
	overflow := a.species.lineage.overflow
	dirty := a.rollupDirty.ledger
	a.mu.Unlock()

	if instances != 1 || overflow != 1 {
		t.Fatalf("instances=%d overflow=%d, want the one admitted instance and one refusal",
			instances, overflow)
	}
	if !dirty {
		t.Fatal("the published lineage overflow was not marked for persistence")
	}
	view := a.SpeciesTreeView()
	if view.LineageOverflow != 1 {
		t.Fatalf("tree lineageOverflow = %d, want 1", view.LineageOverflow)
	}
	beta := livingTreeNode(t, view, "Beta two")
	if !beta.AncestryUnresolved || view.Unrecorded != 0 {
		t.Fatalf("a capacity-refused lineage was reported as no evidence: %+v", beta)
	}
}
