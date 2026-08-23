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
