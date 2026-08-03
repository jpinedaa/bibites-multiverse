package relay

// The ring model of contract-b-m3.md §7.
//
// The relay holds an ordered list of reservations. Slot numbers are
// identifiers, not positions: the list order is the ring order, the east
// neighbour of entry i is entry i+1, and the east neighbour of the last entry
// is the first. A slot number is never reused while its reservation exists and
// is never renumbered, which is what makes "the map never reshuffles" true.
//
// The list is durable. A reservation that never expires (D8) is worthless if it
// lives only in RAM — across a relay restart every peer would be inserted again
// as a new slot in connect order, silently rewiring the ring. M2 kept the sector
// map in memory and listed durability as an open item; §11 item 5 closes it.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"multiverse/internal/fsutil"
)

// Reservation binds one ring slot to one peer identity.
type Reservation struct {
	Slot   int    `json:"slot"`
	PeerID string `json:"peerId"`
}

// Ring is the ordered reservation list plus the slot counter. It is not safe
// for concurrent use; the Server holds it under its own lock.
type Ring struct {
	Order             []Reservation `json:"ring"`
	MaxSlotEverIssued int           `json:"maxSlotEverIssued"`

	path string
}

// LoadRing reads <dir>/ring.json, or returns an empty ring when there is none.
// An empty dir keeps the ring in memory, which is what a test rig wants.
func LoadRing(dir string) (*Ring, error) {
	r := &Ring{}
	if dir == "" {
		return r, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	r.path = filepath.Join(dir, "ring.json")
	b, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, r); err != nil {
		return nil, fmt.Errorf("relay: %s is unreadable: %w", r.path, err)
	}
	for _, res := range r.Order {
		if res.Slot > r.MaxSlotEverIssued {
			r.MaxSlotEverIssued = res.Slot
		}
	}
	return r, nil
}

// Save flushes the ring to disk. §7.4 requires this to happen *before* the
// relay answers a SECTOR_CLAIM that created or changed a reservation: an
// answered grant that is not on disk can hand the same slot to two peers across
// a restart.
func (r *Ring) Save() error {
	if r.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, r.path); err != nil {
		return err
	}
	return fsutil.SyncDir(filepath.Dir(r.path))
}

// Size is the number of slots in the ring.
func (r *Ring) Size() int { return len(r.Order) }

// IndexOfPeer returns the ring position of peerID, or -1.
func (r *Ring) IndexOfPeer(peerID string) int {
	for i, res := range r.Order {
		if res.PeerID == peerID {
			return i
		}
	}
	return -1
}

// IndexOfSlot returns the ring position of slot, or -1.
func (r *Ring) IndexOfSlot(slot int) int {
	for i, res := range r.Order {
		if res.Slot == slot {
			return i
		}
	}
	return -1
}

// SlotOfPeer returns the slot reserved to peerID, or 0.
func (r *Ring) SlotOfPeer(peerID string) int {
	if i := r.IndexOfPeer(peerID); i >= 0 {
		return r.Order[i].Slot
	}
	return 0
}

// PeerOfSlot returns the identity reserved to slot, live or not.
func (r *Ring) PeerOfSlot(slot int) string {
	if i := r.IndexOfSlot(slot); i >= 0 {
		return r.Order[i].PeerID
	}
	return ""
}

// East returns the reservation one step east of peerID. It is absent at ring
// size 1, where a peer's east neighbour would be itself — §2 forbids granting
// that, and the peer's export edge stays closed with no_peer.
func (r *Ring) East(peerID string) (Reservation, bool) {
	i := r.IndexOfPeer(peerID)
	if i < 0 || len(r.Order) < 2 {
		return Reservation{}, false
	}
	return r.Order[(i+1)%len(r.Order)], true
}

// Append inserts a new slot at the tail and returns it.
//
// The tail is the only insertion point, and that is a decision rather than a
// convenience (§7.2): appending changes exactly one existing lane — the old
// tail's east neighbour becomes the new peer instead of the head. Inserting
// anywhere else changes two, and inserting between two live peers is what
// m3_considerations.md Risk 4 warns about. A ring is symmetric under rotation,
// so the tail is as good a position as any and costs the least churn.
func (r *Ring) Append(peerID string) Reservation {
	r.MaxSlotEverIssued++
	res := Reservation{Slot: r.MaxSlotEverIssued, PeerID: peerID}
	r.Order = append(r.Order, res)
	return res
}

// Release splices a slot out of the ring order. Surviving slots keep their
// numbers and their relative order, and MaxSlotEverIssued never decreases, so
// the released number is never reused (§7.5).
func (r *Ring) Release(slot int) (Reservation, bool) {
	i := r.IndexOfSlot(slot)
	if i < 0 {
		return Reservation{}, false
	}
	res := r.Order[i]
	r.Order = append(r.Order[:i:i], r.Order[i+1:]...)
	return res, true
}
