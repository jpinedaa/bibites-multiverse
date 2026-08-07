package relay

// The grid model of contract-b-m4.md §7.
//
// The relay holds a RECTANGLE and a set of RESERVATIONS, each binding one slot
// number and one position to one peerId. The two are deliberately different
// things (§2, D13):
//
//	slot number  the routing address. An integer >= 1, never reused, never
//	             renumbered. destSlot names it, and every journal entry in the
//	             map names it.
//	position     the coordinate {col,row}. It decides who a peer's neighbours
//	             are, and nothing else. It MOVES when the map grows.
//
// That split is what makes insertion, re-positioning and handover safe for work
// in flight: a splice shifts coordinates, which changes who is next to whom,
// which changes future lanes — and it changes no address, so it changes nothing
// already in a journal (§7.3).
//
// DO NOT DERIVE A COORDINATE FROM AN INDEX. A row-major index over an ordered
// list is cheaper to store and renumbers every peer on each insertion, which is
// exactly the reshuffle D8 forbids and D12 preserves.
//
// The file is durable. A reservation that never expires (D8) is worthless if it
// lives only in RAM — across a relay restart every peer would be placed again as
// a new slot in connect order, silently rewiring the map, and every journaled
// destSlot would become SLOT_VACANT.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/fsutil"
)

// Reservation binds one slot number and one position to one peer identity.
type Reservation struct {
	Slot   int    `json:"slot"`
	Col    int    `json:"col"`
	Row    int    `json:"row"`
	PeerID string `json:"peerId"`
}

// Position is the reservation's coordinate on the wire.
func (r Reservation) Position() contractb.Position {
	return contractb.Position{Col: r.Col, Row: r.Row}
}

// Grid is the rectangle plus the reservation set plus the slot counter. It is
// not safe for concurrent use; the Server holds it under its own lock.
type Grid struct {
	Width             int           `json:"width"`
	Height            int           `json:"height"`
	Slots             []Reservation `json:"slots"`
	MaxSlotEverIssued int           `json:"maxSlotEverIssued"`

	// LegacyRing carries an M3 ring.json forward. §2 states that the M3 ring is
	// the one-row case of the map, so a ring of w slots loads as a w x 1 grid
	// with the reservations in their old order. It is written back under the new
	// key on the first Save and never again.
	LegacyRing []Reservation `json:"ring,omitempty"`

	path string
}

// LoadGrid reads <dir>/ring.json, or returns an empty map when there is none.
// An empty dir keeps the map in memory, which is what a test rig wants and what
// production must never do (§7.4).
func LoadGrid(dir string) (*Grid, error) {
	g := &Grid{}
	if dir == "" {
		return g, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	g.path = filepath.Join(dir, "ring.json")
	b, err := os.ReadFile(g.path)
	if errors.Is(err, os.ErrNotExist) {
		return g, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, g); err != nil {
		return nil, fmt.Errorf("relay: %s is unreadable: %w", g.path, err)
	}
	if len(g.Slots) == 0 && len(g.LegacyRing) > 0 {
		// An M3 ring: the height-1 specialization of the map (§2).
		for i := range g.LegacyRing {
			res := g.LegacyRing[i]
			res.Col, res.Row = i, 0
			g.Slots = append(g.Slots, res)
		}
		g.Width, g.Height = len(g.Slots), 1
	}
	g.LegacyRing = nil
	for _, res := range g.Slots {
		if res.Slot > g.MaxSlotEverIssued {
			g.MaxSlotEverIssued = res.Slot
		}
		if res.Col >= g.Width {
			g.Width = res.Col + 1
		}
		if res.Row >= g.Height {
			g.Height = res.Row + 1
		}
	}
	g.sort()
	return g, nil
}

// Save flushes the map to disk. §7.4 requires this to happen BEFORE the relay
// answers a SECTOR_CLAIM that created or changed a reservation: an answered
// grant that is not on disk can hand the same slot to two peers across a
// restart.
func (g *Grid) Save() error {
	if g.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	tmp := g.path + ".tmp"
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
	if err := os.Rename(tmp, g.path); err != nil {
		return err
	}
	return fsutil.SyncDir(filepath.Dir(g.path))
}

// Shape is the rectangle right now.
func (g *Grid) Shape() contractb.MapShape {
	return contractb.MapShape{Width: g.Width, Height: g.Height}
}

// Size is the number of reserved slots.
func (g *Grid) Size() int { return len(g.Slots) }

// sort puts the reservation list in structural order: row ascending, then col
// ascending (§6.5). Everything published reads from this order, so the shape a
// client sees is always the shape the relay holds.
func (g *Grid) sort() {
	sort.SliceStable(g.Slots, func(a, b int) bool {
		if g.Slots[a].Row != g.Slots[b].Row {
			return g.Slots[a].Row < g.Slots[b].Row
		}
		return g.Slots[a].Col < g.Slots[b].Col
	})
}

// Order returns the reservations in structural order.
func (g *Grid) Order() []Reservation { return append([]Reservation(nil), g.Slots...) }

func (g *Grid) indexOfPeer(peerID string) int {
	for i, res := range g.Slots {
		if res.PeerID == peerID {
			return i
		}
	}
	return -1
}

func (g *Grid) indexOfSlot(slot int) int {
	for i, res := range g.Slots {
		if res.Slot == slot {
			return i
		}
	}
	return -1
}

// ResOfPeer returns the reservation held by peerID.
func (g *Grid) ResOfPeer(peerID string) (Reservation, bool) {
	if i := g.indexOfPeer(peerID); i >= 0 {
		return g.Slots[i], true
	}
	return Reservation{}, false
}

// ResOfSlot returns the reservation for slot, live or not.
func (g *Grid) ResOfSlot(slot int) (Reservation, bool) {
	if i := g.indexOfSlot(slot); i >= 0 {
		return g.Slots[i], true
	}
	return Reservation{}, false
}

// SlotOfPeer returns the slot reserved to peerID, or 0.
func (g *Grid) SlotOfPeer(peerID string) int {
	if res, ok := g.ResOfPeer(peerID); ok {
		return res.Slot
	}
	return 0
}

// PeerOfSlot returns the identity reserved to slot, live or not.
func (g *Grid) PeerOfSlot(slot int) string {
	if res, ok := g.ResOfSlot(slot); ok {
		return res.PeerID
	}
	return ""
}

// At returns the reservation at a coordinate. A position inside the rectangle
// with no reservation is a HOLE, and a hole and a dark slot look identical to a
// router — which is why route-around is what makes a map with holes viable.
func (g *Grid) At(col, row int) (Reservation, bool) {
	for _, res := range g.Slots {
		if res.Col == col && res.Row == row {
			return res, true
		}
	}
	return Reservation{}, false
}

// Holes lists the positions inside the rectangle that no slot names.
func (g *Grid) Holes() []contractb.Position {
	out := []contractb.Position{}
	for row := 0; row < g.Height; row++ {
		for col := 0; col < g.Width; col++ {
			if _, ok := g.At(col, row); !ok {
				out = append(out, contractb.Position{Col: col, Row: row})
			}
		}
	}
	return out
}

// Preference is the advisory part of a SECTOR_CLAIM (§6.3). Every field of it
// may be ignored, and a claim never fails for a lost race.
type Preference struct {
	Slot            int
	Position        *contractb.Position
	InsertAfterSlot int
	InsertAxis      string
}

// Place implements contract-b-m4.md §7.2's arbitration rules 3 to 6, in order.
// Rules 1 and 2 — this peerId already holds a reservation — are the caller's,
// because only the caller knows whether this is the first claim of a connection
// (reclaimed) or a repeat (updated).
//
// The first rule that applies wins, and the result is always SOME placement:
// there is no position_taken refusal, because a refusal would leave a peer with
// no world in the map and nothing useful to do about it.
func (g *Grid) Place(peerID string, pref Preference) Reservation {
	// Rule 3 is a no-op by construction: a preferredSlot held by somebody else
	// is simply not consulted here. A preference never evicts anybody.

	// Rule 4: preferredPosition, when usable.
	if pref.Position != nil {
		if res, ok := g.placeAt(peerID, pref.Position.Col, pref.Position.Row); ok {
			return res
		}
	}
	// Rule 5: insertAfterSlot splices a whole column or a whole row.
	if pref.InsertAfterSlot > 0 {
		if anchor, ok := g.ResOfSlot(pref.InsertAfterSlot); ok {
			axis := pref.InsertAxis
			if axis == "" {
				axis = contracta.EdgeE
			}
			return g.splice(peerID, anchor, axis)
		}
	}
	// Rule 6: auto-place. The first hole in structural order, else grow the
	// shorter axis. Six peers with no opinion land in a full 3x2 map, which is
	// the shape the exit test needs and the smallest honest two-axis map.
	for _, hole := range g.Holes() {
		return g.reserve(peerID, hole.Col, hole.Row)
	}
	switch {
	case g.Width == 0 || g.Height == 0:
		g.Width, g.Height = 1, 1
		return g.reserve(peerID, 0, 0)
	case g.Width <= g.Height:
		g.Width++
		return g.reserve(peerID, g.Width-1, 0)
	default:
		g.Height++
		return g.reserve(peerID, 0, g.Height-1)
	}
}

// placeAt implements rule 4's "usable" test. A RAGGED MAP IS NEVER LEGAL:
// growth is always by a whole column or a whole row, and the rest of it is
// holes.
func (g *Grid) placeAt(peerID string, col, row int) (Reservation, bool) {
	if col < 0 || row < 0 {
		return Reservation{}, false
	}
	switch {
	case g.Width == 0 && g.Height == 0:
		if col != 0 || row != 0 {
			return Reservation{}, false
		}
		g.Width, g.Height = 1, 1
	case col < g.Width && row < g.Height:
		if _, taken := g.At(col, row); taken {
			return Reservation{}, false
		}
	case col == g.Width && row < max(g.Height, 1):
		g.Width++
		if g.Height == 0 {
			g.Height = 1
		}
	case row == g.Height && col < max(g.Width, 1):
		g.Height++
		if g.Width == 0 {
			g.Width = 1
		}
	default:
		// A gap of two columns, or a diagonal beyond both axes.
		return Reservation{}, false
	}
	return g.reserve(peerID, col, row), true
}

// splice implements rule 5. It inserts a whole column at index anchor.col+1
// ("E") or a whole row at anchor.row+1 ("N"), shifts everything after it by
// one, and places the newcomer at the crossing. Two lanes change and NO SLOT
// NUMBER CHANGES.
func (g *Grid) splice(peerID string, anchor Reservation, axis string) Reservation {
	if axis == contracta.EdgeN {
		for i := range g.Slots {
			if g.Slots[i].Row > anchor.Row {
				g.Slots[i].Row++
			}
		}
		g.Height++
		return g.reserve(peerID, anchor.Col, anchor.Row+1)
	}
	for i := range g.Slots {
		if g.Slots[i].Col > anchor.Col {
			g.Slots[i].Col++
		}
	}
	g.Width++
	return g.reserve(peerID, anchor.Col+1, anchor.Row)
}

// reserve mints the next slot number and binds it. A newcomer's slot number is
// greater than every number ever issued, so NO JOURNAL ENTRY ANYWHERE CAN NAME
// IT — which is what makes insertion provably safe for work in flight rather
// than argued to be (§7.3 rule 4).
func (g *Grid) reserve(peerID string, col, row int) Reservation {
	g.MaxSlotEverIssued++
	res := Reservation{Slot: g.MaxSlotEverIssued, Col: col, Row: row, PeerID: peerID}
	g.Slots = append(g.Slots, res)
	if col >= g.Width {
		g.Width = col + 1
	}
	if row >= g.Height {
		g.Height = row + 1
	}
	g.sort()
	return res
}

// Release splices a slot out and leaves its position a HOLE. Surviving slots
// keep their numbers, their positions and their relative order, and
// MaxSlotEverIssued never decreases, so the released number is never reused
// (§7.5).
func (g *Grid) Release(slot int) (Reservation, bool) {
	i := g.indexOfSlot(slot)
	if i < 0 {
		return Reservation{}, false
	}
	res := g.Slots[i]
	g.Slots = append(g.Slots[:i:i], g.Slots[i+1:]...)
	return res, true
}

// Handover rebinds a reservation — slot number AND position — to a different
// peerId (§7.5, new in M4). The map does not change shape and no lane moves.
//
// The new occupant inherits NOTHING: it starts with an empty journal and a
// different world, and it is not told about the old peer's entries because it
// cannot do anything correct with them. In-flight work addressed to that slot
// arrives at the new occupant, because routing is on the slot.
func (g *Grid) Handover(slot int, newPeerID string) (old Reservation, res Reservation, ok bool) {
	i := g.indexOfSlot(slot)
	if i < 0 {
		return Reservation{}, Reservation{}, false
	}
	old = g.Slots[i]
	g.Slots[i].PeerID = newPeerID
	return old, g.Slots[i], true
}

// Deliverable is §8's per-candidate filter, from the point of view of the peer
// doing the asking. DELIVERABILITY, NOT LIVENESS, SELECTS THE TARGET (D12): M3
// evaluated these conditions and closed the edge on the first failure; M4
// evaluates the same conditions per candidate slot and uses them to FILTER.
//
// It returns the empty string when the slot is deliverable, and the §6.4 skip
// reason otherwise.
type Deliverable func(res Reservation) string

// Effective walks one export edge and returns the first deliverable slot on it,
// with the bypass list in walk order (§8, and §17 B13 for the reverse pair):
//
//	effective(E) = first deliverable slot at (col+1,row), (col+2,row), ... mod width
//	effective(N) = first deliverable slot at (col,row+1), (col,row+2), ... mod height
//	effective(W) = first deliverable slot at (col-1,row), (col-2,row), ... mod width
//	effective(S) = first deliverable slot at (col,row-1), (col,row-2), ... mod height
//
// THE TWO REVERSE WALKS ARE THE TWO FORWARD WALKS WITH THE STEP NEGATED, and
// that is the whole of D17's routing change. deliverable() is untouched — one
// filter, four walks — and each walk still visits every OTHER position on its
// axis exactly once and then stops, so a peer never exports to itself. An axis
// of length 1 has no candidates in either direction, and an empty skip list
// with no target is what closes that export edge with no_peer.
//
// Two consequences fall out of the arithmetic and are NOT defects:
//
//   - SKIP LISTS ARE PER WALK, NOT PER AXIS. E and W traverse the same row from
//     opposite ends, meet its dark slots in a different order and stop at the
//     first live one, so their skipped lists routinely differ even when they
//     name the same target.
//   - ON AN AXIS OF LENGTH 2 THE FORWARD AND REVERSE WALKS NAME THE SAME SLOT,
//     because +1 and -1 are the same position mod 2. Both lanes of that shuttle
//     open together and close together. On the 3x2 rig every COLUMN is such an
//     axis (§2.1).
func (g *Grid) Effective(from Reservation, edge string, ok Deliverable) (Reservation, []contractb.Skip, bool) {
	skipped := []contractb.Skip{}
	vertical := contracta.Vertical(edge)
	length := g.Width
	if vertical {
		length = g.Height
	}
	delta := 1
	if contracta.Reverse(edge) {
		delta = -1
	}
	for step := 1; step < length; step++ {
		col, row := from.Col, from.Row
		if vertical {
			row = wrapIndex(from.Row+delta*step, g.Height)
		} else {
			col = wrapIndex(from.Col+delta*step, g.Width)
		}
		pos := contractb.Position{Col: col, Row: row}
		res, reserved := g.At(col, row)
		if !reserved {
			skipped = append(skipped, contractb.Skip{Position: pos, Reason: contractb.SkipHole})
			continue
		}
		if reason := ok(res); reason != "" {
			slot := res.Slot
			skipped = append(skipped, contractb.Skip{Slot: &slot, Position: pos, Reason: reason})
			continue
		}
		return res, skipped, true
	}
	return Reservation{}, skipped, false
}

// wrapIndex is the torus, written once so a reverse walk cannot pick up Go's
// negative remainder. Go's % keeps the sign of the dividend, so (0-1)%3 is -1
// and not 2, and a west lane off column 0 would address a position that does
// not exist.
func wrapIndex(i, n int) int {
	if n <= 0 {
		return 0
	}
	return ((i % n) + n) % n
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
