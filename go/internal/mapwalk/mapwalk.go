// Package mapwalk is contract-b-m4.md §8's per-axis walk, over a PEER_STATUS
// frame.
//
// The relay performs this walk and publishes the result in
// SECTOR_GRANT.neighbours (§6.4). ANY CLIENT CAN REPRODUCE IT from PEER_STATUS,
// and that is what lets the archive draw the whole map — bypasses included —
// without asking six sidecars anything (§6.5, §10.1). The walk is deterministic
// and every input is on that frame: position, live, modConnected, gameVersion,
// simulationSize, exportEdges.
//
// THE RELAY'S SECTOR_GRANT REMAINS THE AUTHORITY for a peer's actual routing. A
// client's recomputation is for display and for the one thing a grant cannot
// carry — the skip list of an axis that has NO deliverable target, which is what
// §8 maps into an EDGE_STATUS reason.
package mapwalk

import (
	"math"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
)

// Deliverable is §8's filter, evaluated per candidate slot:
//
//	deliverable(slot) <=> reserved and live and modConnected
//	                      and gameVersion compatible with mine
//	                      and simulationSize equal to mine
//	                      and slot != me
//
// It returns the empty string when the candidate is deliverable, and the §6.4
// skip reason otherwise. The hole case is handled by the walk itself.
func Deliverable(me, candidate contractb.SlotInfo) string {
	if !candidate.Live {
		return contractb.SkipPeerOffline
	}
	if !candidate.ModConnected {
		return contractb.SkipPeerModAbsent
	}
	if me.GameVersion != "" && candidate.GameVersion != "" && me.GameVersion != candidate.GameVersion {
		return contractb.SkipPeerIncompatible
	}
	if me.SimulationSize > 0 && candidate.SimulationSize > 0 &&
		!SameSize(me.SimulationSize, candidate.SimulationSize) {
		return contractb.SkipSimSizeMismatch
	}
	return ""
}

// SameSize is contract-a.md §13 A10's relative epsilon; §4.1 forbids exact
// float equality.
func SameSize(a, b float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return false
	}
	return math.Abs(a-b) <= 1e-6*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

// Walk returns the first deliverable slot along one export edge of me, with the
// bypass list in walk order:
//
//	effective(E) = first deliverable slot at (col+1,row), (col+2,row), ... mod width
//	effective(N) = first deliverable slot at (col,row+1), (col,row+2), ... mod height
//	effective(W) = first deliverable slot at (col-1,row), (col-2,row), ... mod width
//	effective(S) = first deliverable slot at (col,row-1), (col,row-2), ... mod height
//
// The reverse pair is the forward pair with the step negated (§17, B13) and
// nothing else about it differs: same Deliverable filter, same "visit every
// OTHER position on the axis exactly once and stop", same empty-skip-list-means
// -no_peer. THE SKIP LIST IS PER WALK: E and W cross one row from opposite ends
// and legitimately report different bypasses.
//
// This is the same computation relay.Grid.Effective performs, on the inputs
// PEER_STATUS carries, and the two MUST agree. It is duplicated rather than
// shared because the relay walks its reservation set and a client walks a
// broadcast frame.
func Walk(status contractb.PeerStatus, me contractb.SlotInfo, edge string) (contractb.SlotInfo, []contractb.Skip, bool) {
	skipped := []contractb.Skip{}
	vertical := contracta.Vertical(edge)
	length := status.Map.Width
	if vertical {
		length = status.Map.Height
	}
	delta := 1
	if contracta.Reverse(edge) {
		delta = -1
	}
	for step := 1; step < length; step++ {
		pos := me.Position
		if vertical {
			pos.Row = wrapIndex(me.Position.Row+delta*step, status.Map.Height)
		} else {
			pos.Col = wrapIndex(me.Position.Col+delta*step, status.Map.Width)
		}
		cand, reserved := At(status, pos)
		if !reserved {
			skipped = append(skipped, contractb.Skip{Position: pos, Reason: contractb.SkipHole})
			continue
		}
		if reason := Deliverable(me, cand); reason != "" {
			slot := cand.Slot
			skipped = append(skipped, contractb.Skip{Slot: &slot, Position: pos, Reason: reason})
			continue
		}
		return cand, skipped, true
	}
	return contractb.SlotInfo{}, skipped, false
}

// wrapIndex is the torus. Go's % keeps the sign of the dividend, so a reverse
// walk off column 0 would address column -1 without it.
func wrapIndex(i, n int) int {
	if n <= 0 {
		return 0
	}
	return ((i % n) + n) % n
}

// At finds the slot reserved at a position, if any. A position inside the
// rectangle that no slot names is a HOLE.
func At(status contractb.PeerStatus, pos contractb.Position) (contractb.SlotInfo, bool) {
	for _, s := range status.Slots {
		if s.Position == pos {
			return s, true
		}
	}
	return contractb.SlotInfo{}, false
}

// Find looks one slot number up in a PEER_STATUS frame.
func Find(status contractb.PeerStatus, slot int) (contractb.SlotInfo, bool) {
	for _, s := range status.Slots {
		if s.Slot == slot {
			return s, true
		}
	}
	return contractb.SlotInfo{}, false
}

// EdgeReason maps the skip list of an axis with NO deliverable target into
// Contract A's unchanged EDGE_STATUS reason enum (§8):
//
//	empty list                       no_peer   (no other position on the axis at all)
//	every entry shares peer_mod_absent      peer_mod_absent
//	every entry shares peer_incompatible    peer_incompatible
//	every entry shares sim_size_mismatch    sim_size_mismatch
//	every entry shares hole or peer_offline no_peer
//	entries disagree                        no_peer
//
// A two-world column whose other world was resized therefore tells the mod
// sim_size_mismatch rather than a shrug, and a three-world row that lost two
// peers for two different reasons says no_peer and leaves the detail to the
// status page, which has the skip list itself.
func EdgeReason(skipped []contractb.Skip) string {
	if len(skipped) == 0 {
		return contracta.ReasonNoPeer
	}
	shared := ""
	for i, s := range skipped {
		r := s.Reason
		if r == contractb.SkipHole || r == contractb.SkipPeerOffline {
			r = contracta.ReasonNoPeer
		}
		if i == 0 {
			shared = r
			continue
		}
		if shared != r {
			return contracta.ReasonNoPeer
		}
	}
	switch shared {
	case contractb.SkipPeerModAbsent:
		return contracta.ReasonPeerModAbsent
	case contractb.SkipPeerIncompatible:
		return contracta.ReasonPeerIncompatible
	case contractb.SkipSimSizeMismatch:
		return contracta.ReasonSimSizeMismatch
	default:
		return contracta.ReasonNoPeer
	}
}
