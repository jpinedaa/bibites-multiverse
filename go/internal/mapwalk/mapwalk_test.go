package mapwalk

import (
	"testing"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
)

func TestWalkAfterSkipsSourceAndPreviouslyRefusedWorlds(t *testing.T) {
	status := contractb.PeerStatus{
		Map: contractb.MapShape{Width: 4, Height: 1},
		Slots: []contractb.SlotInfo{
			{Slot: 1, Position: contractb.Position{Col: 0}, Live: true, ModConnected: true},
			{Slot: 2, Position: contractb.Position{Col: 1}, Live: true, ModConnected: true},
			{Slot: 3, Position: contractb.Position{Col: 2}, Live: true, ModConnected: true},
			{Slot: 4, Position: contractb.Position{Col: 3}, Live: true, ModConnected: true},
		},
	}
	me := status.Slots[0]

	got, ok := WalkAfter(status, me, contracta.EdgeE, 2, map[int]bool{2: true})
	if !ok || got.Slot != 3 {
		t.Fatalf("east after slot 2 = slot %d, %v; want slot 3", got.Slot, ok)
	}
	got, ok = WalkAfter(status, me, contracta.EdgeE, 3, map[int]bool{2: true, 3: true})
	if !ok || got.Slot != 4 {
		t.Fatalf("east after slots 2 and 3 = slot %d, %v; want slot 4", got.Slot, ok)
	}
	if _, ok := WalkAfter(status, me, contracta.EdgeE, 4,
		map[int]bool{2: true, 3: true, 4: true}); ok {
		t.Fatal("walk wrapped through the source or revisited a refused world")
	}
}

func TestWalkAfterPreservesAxisAndDirection(t *testing.T) {
	status := contractb.PeerStatus{
		Map: contractb.MapShape{Width: 1, Height: 4},
		Slots: []contractb.SlotInfo{
			{Slot: 1, Position: contractb.Position{Row: 0}, Live: true, ModConnected: true},
			{Slot: 2, Position: contractb.Position{Row: 1}, Live: true, ModConnected: true},
			{Slot: 3, Position: contractb.Position{Row: 2}, Live: true, ModConnected: true},
			{Slot: 4, Position: contractb.Position{Row: 3}, Live: true, ModConnected: true},
		},
	}
	me := status.Slots[0]
	got, ok := WalkAfter(status, me, contracta.EdgeS, 4, map[int]bool{4: true})
	if !ok || got.Slot != 3 {
		t.Fatalf("south after slot 4 = slot %d, %v; want slot 3", got.Slot, ok)
	}
}

// grid3x3 reserves slot 1+col+3*row at every position of a 3x3 torus, all live
// and mod-connected. Slot 5 sits at the centre.
func grid3x3() contractb.PeerStatus {
	status := contractb.PeerStatus{Map: contractb.MapShape{Width: 3, Height: 3}}
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			status.Slots = append(status.Slots, contractb.SlotInfo{
				Slot:         1 + col + 3*row,
				Position:     contractb.Position{Col: col, Row: row},
				Live:         true,
				ModConnected: true,
			})
		}
	}
	return status
}

// drainOrder pins the complete WalkAnywhere enumeration by excluding each pick
// as it is made, the way a refusal chain's durable tried set does.
func drainOrder(t *testing.T, status contractb.PeerStatus, me contractb.SlotInfo,
	edge string, want []int) {
	t.Helper()
	excluded := map[int]bool{}
	for i, wantSlot := range want {
		got, ok := WalkAnywhere(status, me, edge, excluded)
		if !ok || got.Slot != wantSlot {
			t.Fatalf("%s pick %d = slot %d, %v; want slot %d", edge, i, got.Slot, ok, wantSlot)
		}
		excluded[got.Slot] = true
	}
	if got, ok := WalkAnywhere(status, me, edge, excluded); ok {
		t.Fatalf("%s walk returned slot %d after every other slot was excluded", edge, got.Slot)
	}
}

func TestWalkAnywhereOrderIsAxisMajorFromTheSource(t *testing.T) {
	status := grid3x3()
	me, _ := Find(status, 5)

	// E: rows in ascending perpendicular offset from mine (1, 2, 0), each row
	// scanned east from my column with wrap. Offset zero re-covers the exit
	// axis first.
	drainOrder(t, status, me, contracta.EdgeE, []int{6, 4, 8, 9, 7, 2, 3, 1})
	// W is E with the step negated, row order unchanged.
	drainOrder(t, status, me, contracta.EdgeW, []int{4, 6, 8, 7, 9, 2, 1, 3})
	// N: columns in ascending perpendicular offset (1, 2, 0), each scanned
	// north (rows ascending) from my row with wrap.
	drainOrder(t, status, me, contracta.EdgeN, []int{8, 2, 6, 9, 3, 4, 7, 1})
	// S is N with the step negated, column order unchanged.
	drainOrder(t, status, me, contracta.EdgeS, []int{2, 8, 6, 3, 9, 4, 1, 7})
}

func TestWalkAnywhereSkipsHolesSourceAndExcluded(t *testing.T) {
	status := grid3x3()
	// Slot 6 becomes a hole, slot 8 loses its mod: the first two picks of the
	// east order must be passed over for their own reasons.
	slots := status.Slots[:0]
	for _, s := range status.Slots {
		if s.Slot == 6 {
			continue
		}
		if s.Slot == 8 {
			s.ModConnected = false
		}
		slots = append(slots, s)
	}
	status.Slots = slots
	me, _ := Find(status, 5)

	got, ok := WalkAnywhere(status, me, contracta.EdgeE, map[int]bool{4: true})
	if !ok || got.Slot != 9 {
		t.Fatalf("east pick = slot %d, %v; want slot 9 past the hole, the excluded and the mod-absent", got.Slot, ok)
	}
}

func TestWalkAnywhereNotFoundMeansEveryCompatibleSlotTried(t *testing.T) {
	status := grid3x3()
	me, _ := Find(status, 5)
	excluded := map[int]bool{}
	for _, s := range status.Slots {
		if s.Slot != me.Slot && s.Slot != 3 {
			excluded[s.Slot] = true
		}
	}
	// The one untried slot is incompatible: not-found, and nothing is invented.
	for i := range status.Slots {
		if status.Slots[i].Slot == 3 {
			status.Slots[i].Live = false
		}
	}
	if got, ok := WalkAnywhere(status, me, contracta.EdgeE, excluded); ok {
		t.Fatalf("walk returned slot %d; want not-found when the only untried slot is undeliverable", got.Slot)
	}
}
