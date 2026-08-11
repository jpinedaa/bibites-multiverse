package sidecar

// A50's admission check: the sidecar refuses a declared export set the map
// cannot use (contract-a.md §21, A50; §12 item 9 closes there).
//
// WHOSE CHECK IT IS. The sidecar's, alone. The mod declares GEOMETRY — "I run a
// capture band here" — and learns no topology: not its coordinate, not its
// neighbour, not the map's shape (D8, D13, §15 A18, §15 A25). Nothing about
// this check reaches CONFIG_UPDATE and there is no field for a mod to be told
// the answer with. The sidecar is the only party that holds both inputs, which
// is exactly why the defect survived two milestones.
//
// WHAT THE DEFECT ACTUALLY IS, because the item's own example expired. §12 item
// 9 wrote it as ["E","S"] on a grid that exports east and north — and §18's A38
// made every edge an export edge, so that pairing became an ordinary
// under-declaration. THE SHAPE SURVIVED THE EXAMPLE AND GOT WORSE: a declared
// edge the map cannot route is reported CLOSED rather than refused, so a
// misconfigured world comes up healthy, handshakes, reports no_peer, and looks
// EXACTLY like a world whose neighbours are asleep. On the rig the owner knows
// which it is; after M5 the person looking at it is the one person who cannot
// ask anybody, and the cause is on their own machine with nothing telling them.
//
// TWO ANSWERS, AND THE SECOND ONE IS NOT A FAULT.
//
//	TOTAL: a map with at least one axis and a declared set with no usable
//	member. Close 4007 EXPORT_EDGES_UNUSABLE, and the log names the remedy and
//	who must act.
//
//	PARTIAL: a set with a usable member and an unusable one is LEGAL and
//	unchanged — one warning line per unusable edge, once per session, naming the
//	edge and the map's shape and saying it will stay closed. It states NO
//	REMEDY, because the remedy is a map that grows an axis and that is nobody at
//	this machine's to apply. Refusing here would eject every peer of every
//	single-row map ever run — contract-b-m4.md §2.1 calls the w×1 north edge a
//	map shape, not a misconfiguration.
//
// AND A MAP WITH NO AXIS REFUSES NOBODY. On a 1×1 map no axis exists, so no
// declaration is usable and nothing is refused: a lone first peer on a map that
// has not grown yet is the normal opening state of every map this project will
// ever run, and a check that ejected it would make an empty map unjoinable. The
// refusal fires only when the map can route SOMETHING and this peer declared
// NONE of it.

import (
	"fmt"
	"strings"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
)

// The two axes a rectangle can have (contract-b-m4.md §2, §6.4, §6.5).
const (
	axisRow    = "row"
	axisColumn = "column"
)

// axisOfEdge names the axis an edge lies on. E and W run along a row; N and S
// run along a column.
func axisOfEdge(edge string) string {
	switch edge {
	case contracta.EdgeE, contracta.EdgeW:
		return axisRow
	case contracta.EdgeN, contracta.EdgeS:
		return axisColumn
	}
	return ""
}

// axisExists is A50's definition of "usable", and it is a fact about the MAP
// and not about any peer: the row axis exists when width ≥ 2 and the column
// axis when height ≥ 2.
func axisExists(shape contractb.MapShape, axis string) bool {
	switch axis {
	case axisRow:
		return shape.Width >= 2
	case axisColumn:
		return shape.Height >= 2
	}
	return false
}

func mapHasAnAxis(shape contractb.MapShape) bool {
	return axisExists(shape, axisRow) || axisExists(shape, axisColumn)
}

// splitDeclaredEdges partitions a declared set into the edges this map can
// route and the edges it cannot.
func splitDeclaredEdges(declared []string, shape contractb.MapShape) (usable, unusable []string) {
	for _, edge := range declared {
		axis := axisOfEdge(edge)
		if axis == "" {
			continue
		}
		if axisExists(shape, axis) {
			usable = append(usable, edge)
			continue
		}
		unusable = append(unusable, edge)
	}
	return usable, unusable
}

func shapeString(shape contractb.MapShape) string {
	return fmt.Sprintf("%dx%d", shape.Width, shape.Height)
}

func edgeSetString(edges []string) string { return "[" + strings.Join(edges, ",") + "]" }

// exportEdgesRefusal is the TOTAL case and returns the close reason, or "" when
// there is nothing to refuse. It reads the two inputs and decides; it logs
// nothing and changes nothing, so the caller can hold a lock across it.
func exportEdgesRefusal(declared []string, shape contractb.MapShape) string {
	if len(declared) == 0 || !mapHasAnAxis(shape) {
		return ""
	}
	usable, unusable := splitDeclaredEdges(declared, shape)
	if len(usable) > 0 || len(unusable) == 0 {
		return ""
	}
	// Every declared edge is unusable and the map has an axis, so every declared
	// edge lies on the ONE axis this map does not have.
	missing := axisOfEdge(unusable[0])
	return fmt.Sprintf("exportEdges %s has no usable edge on a %s map (no %s axis)",
		edgeSetString(declared), shapeString(shape), missing)
}

// checkExportEdgesLocked evaluates A50 at the join of its two inputs and
// returns the close reason for the total case.
//
// IT RUNS WHENEVER EITHER INPUT MOVES — the mod's declaration or the map — and
// WHICHEVER ARRIVES SECOND TRIGGERS IT. Ordinarily that is the SECTOR_GRANT,
// which may land in the same second as the handshake or an hour later.
//
// refusable is what tells the two callers apart, and it is the rule that keeps
// this check from ever losing an organism: ONLY A CONFIG_UPDATE MAY BE REFUSED.
// A change on the map's side never ends a running session — if another peer's
// departure is what turned a usable declaration unusable, the affected edges
// close by the ordinary rules, the partial-case lines are logged, and the world
// goes on RECEIVING organisms on every declared edge (§5.4's open:false closes
// exports and never arrivals). Ejecting a peer for something another peer did
// would be the one way this check could cost an organism.
func (s *Sidecar) checkExportEdgesLocked(sess *modSession, refusable bool) string {
	if sess == nil || !sess.handshaked || len(sess.exportEdges) == 0 {
		return ""
	}
	shape := s.mapShape
	if refusable {
		if reason := exportEdgesRefusal(sess.exportEdges, shape); reason != "" {
			return reason
		}
	}
	if !mapHasAnAxis(shape) {
		// A MAP WITH NO AXIS IS NOT A MISCONFIGURATION AND IT IS NOT WORTH A LINE
		// EITHER. On a 1×1 every declared edge is trivially unusable, and that is
		// the normal opening state of every map this project will ever run — a
		// warning here would say "this edge will stay closed" about a map that is
		// about to grow, and it would say it four times to every peer that ever
		// started first.
		return ""
	}
	_, unusable := splitDeclaredEdges(sess.exportEdges, shape)
	if len(unusable) == 0 {
		return ""
	}
	if sess.warnedEdges == nil {
		sess.warnedEdges = map[string]bool{}
	}
	for _, edge := range unusable {
		// ONE LINE PER UNUSABLE EDGE, ONCE PER SESSION, exactly as A50 words it.
		// The map is re-evaluated on every grant and every broadcast, so without
		// this the commonest legal case — a w×1 rig whose mods declare all four
		// edges — would print two lines every few seconds for the life of the
		// session.
		if sess.warnedEdges[edge] {
			continue
		}
		sess.warnedEdges[edge] = true
		// IT STATES NO REMEDY, deliberately. The remedy is a map that grows an
		// axis, and that is nobody at this machine's to apply — a line that told
		// this operator to fix it would send them looking for a setting that does
		// not exist on their computer.
		s.log.Warn("contract A: a declared export edge lies on an axis this map does not have; "+
			"it will stay closed for the life of this map shape, and no organism is affected",
			"edge", edge, "axis", axisOfEdge(edge), "map", shapeString(shape),
			"declared", edgeSetString(sess.exportEdges),
			"note", "this is a map shape and not a misconfiguration (contract-b-m4.md §2.1, "+
				"contract-a.md §21 A50)")
	}
	return ""
}

// refuseExportEdges closes the mod session with 4007 and says the one thing the
// close code cannot: WHO MUST ACT.
//
// It is an admission policy of ONE MACHINE — not a compatibility rule, and not
// a statement about any other peer. It is the shape contract-b-m4.md §22 B25
// gives minContractVersion on the other wire, one layer down: §3.1's
// compatibility rules are about frames, and this is a decision made about a
// CONFIGURATION before any frame is exchanged on the strength of it.
func (s *Sidecar) refuseExportEdges(sess *modSession, reason string, declared []string,
	shape contractb.MapShape) {
	missingAxis := axisColumn
	if !axisExists(shape, axisRow) {
		missingAxis = axisRow
	}
	s.log.Error(fmt.Sprintf(
		"contract A: refusing session — exportEdges %s declares only the %s axis and this map is "+
			"%s, so no declared edge can ever carry an organism",
		edgeSetString(declared), missingAxis, shapeString(shape)),
		"declared", edgeSetString(declared), "map", shapeString(shape),
		"remedy", "this machine's operator sets MULTIVERSE_EXPORT_EDGES to include "+
			otherAxisEdges(missingAxis)+", or leaves it unset for D17's default of all four edges",
		"whoMustAct", "the operator of THIS machine. Nobody else can fix this and no other peer "+
			"is affected",
		"reconnect", "the mod MUST NOT reconnect automatically: it would re-read the same "+
			"environment variable and reach the same answer (contract-a.md §13, A8; §21, A50)")
	sess.conn.Close(contracta.CloseExportEdgesUnusable, reason)
}

// otherAxisEdges names the edges that WOULD be usable, for the remedy line.
func otherAxisEdges(missingAxis string) string {
	if missingAxis == axisColumn {
		return contracta.EdgeE + " or " + contracta.EdgeW
	}
	return contracta.EdgeN + " or " + contracta.EdgeS
}
