package relay

import (
	"testing"
)

// TestAppendMovesExactlyOneLane covers contract-b-m3.md §7.2: appending at the
// tail changes the old tail's east neighbour and nothing else.
func TestAppendMovesExactlyOneLane(t *testing.T) {
	r := &Ring{}
	r.Append("a")
	r.Append("b")
	if east, ok := r.East("a"); !ok || east.PeerID != "b" {
		t.Fatalf("east(a) = %+v, want b", east)
	}
	if east, ok := r.East("b"); !ok || east.PeerID != "a" {
		t.Fatalf("east(b) = %+v, want a (the wrap-around is the ring)", east)
	}

	r.Append("c")
	if east, _ := r.East("a"); east.PeerID != "b" {
		t.Fatalf("east(a) moved to %s; an append must change exactly one lane", east.PeerID)
	}
	if east, _ := r.East("b"); east.PeerID != "c" {
		t.Fatalf("east(b) = %s, want the new tail c", east.PeerID)
	}
	if east, _ := r.East("c"); east.PeerID != "a" {
		t.Fatalf("east(c) = %s, want the head a", east.PeerID)
	}
	if r.MaxSlotEverIssued != 3 {
		t.Fatalf("maxSlotEverIssued = %d, want 3", r.MaxSlotEverIssued)
	}
}

// TestRingSizeOneHasNoEastNeighbour covers §2's degenerate case: at ringSize 1 a
// peer's east neighbour would be itself, and the relay MUST NOT grant that.
func TestRingSizeOneHasNoEastNeighbour(t *testing.T) {
	r := &Ring{}
	r.Append("a")
	if _, ok := r.East("a"); ok {
		t.Fatal("a lone peer was granted itself as an east neighbour")
	}
}

// TestReleaseSplicesOutAndNeverReusesTheNumber covers §7.5: surviving slots keep
// their numbers and their relative order, and maxSlotEverIssued never decreases.
func TestReleaseSplicesOutAndNeverReusesTheNumber(t *testing.T) {
	r := &Ring{}
	r.Append("a")
	r.Append("b")
	r.Append("c")

	res, ok := r.Release(2)
	if !ok || res.PeerID != "b" {
		t.Fatalf("Release(2) = %+v, %v", res, ok)
	}
	if r.Size() != 2 {
		t.Fatalf("ring size = %d, want 2", r.Size())
	}
	if r.Order[0].Slot != 1 || r.Order[1].Slot != 3 {
		t.Fatalf("ring order is %v; a release must not renumber or reorder", r.Order)
	}
	if east, _ := r.East("a"); east.PeerID != "c" {
		t.Fatalf("east(a) = %s, want c after the splice", east.PeerID)
	}

	// A returning peer with the released identity is a new tail slot, not slot 2.
	back := r.Append("b")
	if back.Slot != 4 {
		t.Fatalf("the returning peer took slot %d, want 4; a released number is retired", back.Slot)
	}
	if _, ok := r.Release(99); ok {
		t.Fatal("Release of an unknown slot reported success")
	}
}

// TestRingIsDurable covers §7.4 and §11 item 5: a reservation that never expires
// is worthless if it lives only in RAM.
func TestRingIsDurable(t *testing.T) {
	dir := t.TempDir()
	r, err := LoadRing(dir)
	if err != nil {
		t.Fatalf("LoadRing: %v", err)
	}
	r.Append("peer-main-slot1")
	r.Append("peer-lan-slot2")
	r.Release(1)
	if err := r.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	again, err := LoadRing(dir)
	if err != nil {
		t.Fatalf("LoadRing: %v", err)
	}
	if again.Size() != 1 || again.Order[0].Slot != 2 || again.Order[0].PeerID != "peer-lan-slot2" {
		t.Fatalf("reloaded ring is %v", again.Order)
	}
	if again.MaxSlotEverIssued != 2 {
		t.Fatalf("reloaded maxSlotEverIssued = %d, want 2", again.MaxSlotEverIssued)
	}
	if got := again.Append("peer-main-slot3"); got.Slot != 3 {
		t.Fatalf("the next slot is %d, want 3", got.Slot)
	}
}

// TestRelayRefusesToStartWithoutAToken covers §3.1: no token configured means
// the relay MUST refuse to start, unless --insecure-no-token is passed.
func TestRelayRefusesToStartWithoutAToken(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("the relay started with no token and no --insecure-no-token")
	}
	if _, err := New(Options{InsecureNoToken: true}); err != nil {
		t.Fatalf("--insecure-no-token should start a test rig: %v", err)
	}
	if _, err := New(Options{Token: "0123456789abcdef"}); err != nil {
		t.Fatalf("a configured token should start: %v", err)
	}
}

// TestReleaseRefusesALivePeersSlot covers §7.5: releasing a slot whose peer is
// live is a mis-operation, and the relay says so.
func TestReleaseRefusesALivePeersSlot(t *testing.T) {
	s, err := New(Options{InsecureNoToken: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.ring.Append("peer-a")
	s.peers["peer-a"] = &peer{id: "peer-a"}
	if err := s.ReleaseSlot(1); err == nil {
		t.Fatal("the relay released a live peer's slot")
	}
	delete(s.peers, "peer-a")
	if err := s.ReleaseSlot(1); err != nil {
		t.Fatalf("releasing an offline peer's slot: %v", err)
	}
	if s.ring.Size() != 0 {
		t.Fatalf("ring size = %d after the release", s.ring.Size())
	}
}
