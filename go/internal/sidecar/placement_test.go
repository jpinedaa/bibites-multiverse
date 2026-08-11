package sidecar

// WP5's sidecar half of contract-b-m4.md §22, B29.
//
// B29's *Enforced by* line gives the sidecar exactly one obligation and it is
// worth quoting, because it is easy to read as nothing at all: "the SIDECAR,
// for sending a preferredPosition IT IS CONTENT TO LOSE (§7.2 — a claim is
// advisory in every part and never fails for a lost race)."
//
// Under contract-b/3.x that obligation was nearly free: a preference that named
// a growable position was granted, so a peer usually got what it asked for.
// B29 narrows rule 4, and now A CORRECTLY CONFIGURED PEER ROUTINELY DOES NOT
// GET THE POSITION IT ASKED FOR — a public map has holes, and holes come before
// growth. That turns "content to lose" from a property nobody exercised into
// the ordinary path, so it gets a test on the ordinary path.
//
// The relay-side rules live in internal/relay/churn_test.go with the churn
// harness. This file is only about what the peer does with the answer.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
)

// TestB29ASidecarIsContentToLoseItsPreferredPosition is the ordinary public-map
// join: a stranger's configuration file names a position, the map has a hole,
// and the relay puts the stranger in the hole instead.
//
// The peer must treat that as a NORMAL GRANT and not as a failure: it takes the
// slot, it opens its lanes, it persists the position it was GIVEN rather than
// the one it asked for, and its next claim therefore prefers where it actually
// is. A peer that kept re-asking for its original preference would produce
// exactly the re-claim storm B29's second rule was written to stop.
func TestB29ASidecarIsContentToLoseItsPreferredPosition(t *testing.T) {
	// Three peers named into a 2x2, which leaves (1,1) a hole. skipEdgeCheck is
	// required rather than convenient: slot 3 at (0,1) shares its row with the
	// hole and nothing else, so its east lane is legitimately CLOSED until the
	// hole is filled (§2.1, §8).
	g := newGrid(t, 3, gridOptions{
		layout:        []contractb.Position{{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 0, Row: 1}},
		skipEdgeCheck: true,
	})
	if shape := g.relay.relay.MapShape(); shape.Width != 2 || shape.Height != 2 {
		t.Fatalf("the rig came up %dx%d, want 2x2 with a hole at (1,1)", shape.Width, shape.Height)
	}

	// A newcomer whose configuration asks to extend the map by a column. Under
	// B29 it is ignored while the hole exists.
	asked := contractb.Position{Col: 2, Row: 0}
	fourth := g.addPeer("peer-slot4", &asked, gridOptions{})

	if fourth.slot != 4 {
		t.Fatalf("the newcomer took slot %d, want 4 (maxSlotEverIssued + 1)", fourth.slot)
	}
	got := fourth.side.Position()
	if got != (contractb.Position{Col: 1, Row: 1}) {
		t.Fatalf("the newcomer is at %+v, want the hole at (1,1): it asked for %+v and B29 says a "+
			"preference that would extend an axis while a hole exists is ignored", got, asked)
	}
	if shape := g.relay.relay.MapShape(); shape.Width != 2 || shape.Height != 2 {
		t.Fatalf("the map grew to %dx%d for a newcomer that should have filled a hole",
			shape.Width, shape.Height)
	}

	// IT IS A NORMAL GRANT, not a degraded one. The peer has a slot, its lanes
	// open in both directions, and the hole it filled reopens the lane its new
	// row-mate had closed.
	waitLane(t, fourth.side, contracta.EdgeE, 3)
	waitLane(t, fourth.side, contracta.EdgeN, 2)
	waitLane(t, g.bySlot(3).side, contracta.EdgeE, 4)
	fourth.mod.waitEdge(contracta.EdgeE, true, 10*time.Second)

	// §7.4: THE POSITION IT WAS GIVEN IS THE POSITION IT PERSISTS. This is what
	// makes "content to lose" durable rather than momentary — after a restart
	// the peer prefers where it is, so a lost race is lost once and not on every
	// reconnect for the life of the install.
	raw, err := os.ReadFile(filepath.Join(fourth.cfg.DataDir, "position"))
	if err != nil {
		t.Fatalf("the sidecar persisted no position: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "1,1" {
		t.Fatalf("the sidecar persisted position %q, want the GRANTED 1,1 rather than the "+
			"preference it lost", strings.TrimSpace(string(raw)))
	}

	// And organisms actually cross into and out of the position it did not ask
	// for, which is the only proof that the placement is a working one.
	in := g.bySlot(3).mod.migrateOut(-79001, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "an organism to cross into the hole-filling peer", func() bool {
		return fourth.world.spawnCount(in) == 1
	})
	out := fourth.mod.migrateOut(-79002, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "an organism to cross out of it", func() bool {
		return g.bySlot(3).world.spawnCount(out) == 1
	})
}
