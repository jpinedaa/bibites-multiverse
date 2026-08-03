package relay

import (
	"testing"

	"multiverse/internal/contractb"
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

// TestReserveSlotPreSeedsTheRingInAnyStartOrder covers the LAN case of §7.2:
// the reservations are created before any peer connects, so rule 1 hands each
// peer its slot whenever it arrives and start order stops mattering.
func TestReserveSlotPreSeedsTheRingInAnyStartOrder(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Options{InsecureNoToken: true, DataDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i, id := range []string{"slot-1", "slot-2", "slot-3"} {
		res, created, err := s.ReserveSlot(id)
		if err != nil {
			t.Fatalf("ReserveSlot(%s): %v", id, err)
		}
		if !created || res.Slot != i+1 || res.PeerID != id {
			t.Fatalf("ReserveSlot(%s) = %+v created=%v, want slot %d", id, res, created, i+1)
		}
	}

	// Idempotent: a re-run of the pre-seed must not insert a second entry.
	res, created, err := s.ReserveSlot("slot-2")
	if err != nil || created || res.Slot != 2 {
		t.Fatalf("re-reserving slot-2 gave %+v created=%v err=%v", res, created, err)
	}
	if s.ring.Size() != 3 {
		t.Fatalf("ring size = %d after a repeated pre-seed, want 3", s.ring.Size())
	}
	if _, _, err := s.ReserveSlot(""); err == nil {
		t.Fatal("an empty peer id was reserved a slot")
	}

	// Durable, and in ring order: a restarted relay must hand slot 2 to the
	// far-end peer even though it connects last.
	again, err := LoadRing(dir)
	if err != nil {
		t.Fatalf("LoadRing: %v", err)
	}
	if again.Size() != 3 || again.Order[1].PeerID != "slot-2" || again.MaxSlotEverIssued != 3 {
		t.Fatalf("reloaded ring is %v (max %d)", again.Order, again.MaxSlotEverIssued)
	}
	if east, ok := again.East("slot-1"); !ok || east.PeerID != "slot-2" {
		t.Fatalf("east(slot-1) = %+v, want slot-2", east)
	}
}

// TestReserveThenClaimGivesTheReservedSlot covers rule 1 over a pre-seeded
// ring: the peer that connects FIRST must still get the slot its peerId names,
// not slot 1.
func TestReserveThenClaimGivesTheReservedSlot(t *testing.T) {
	s, err := New(Options{InsecureNoToken: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, id := range []string{"slot-1", "slot-2", "slot-3"} {
		if _, _, err := s.ReserveSlot(id); err != nil {
			t.Fatalf("ReserveSlot(%s): %v", id, err)
		}
	}
	p := &peer{id: "slot-3"}
	slot, reason, inserted := s.assignLocked(p, 0)
	if slot != 3 || inserted || reason != contractb.GrantReclaimed {
		t.Fatalf("assign(slot-3) = slot %d reason %s inserted %v, want slot 3 reclaimed", slot, reason, inserted)
	}
}
