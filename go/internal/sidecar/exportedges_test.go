package sidecar

// WP4's sidecar half of Contract Change 12 (contract-a.md §21, A50): the
// sidecar refuses a declared export set the map cannot use.
//
// The failure this closes is DQ8's worst class — the peer cannot see the cause,
// and the cause is not on their machine — except that here it IS on their
// machine and nothing told them. A world that declares only the column axis on
// a single-row map comes up healthy, handshakes, reports no_peer on every edge,
// and looks exactly like a world whose neighbours are asleep. On the rig the
// owner knows which it is. After M5 the person looking at it is the one person
// who cannot ask anybody.

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
)

// TestExportEdgesUsabilityIsAFactAboutTheMap covers the decision itself, over
// every shape that matters, before any socket is involved. "Usable" is: the
// axis this edge lies on EXISTS on this map — row when width ≥ 2, column when
// height ≥ 2.
func TestExportEdgesUsabilityIsAFactAboutTheMap(t *testing.T) {
	cases := []struct {
		name     string
		declared []string
		shape    contractb.MapShape
		refuse   bool
	}{
		{"the total case A50 is written about: N,S on a single row",
			[]string{"N", "S"}, contractb.MapShape{Width: 3, Height: 1}, true},
		{"the mirror: E,W on a single column",
			[]string{"E", "W"}, contractb.MapShape{Width: 1, Height: 3}, true},
		{"the partial case is legal: one usable edge is enough",
			[]string{"E", "N"}, contractb.MapShape{Width: 3, Height: 1}, false},
		{"a 1x1 map has no axis, so it refuses nobody",
			[]string{"N", "S"}, contractb.MapShape{Width: 1, Height: 1}, false},
		{"a map that has not been granted yet refuses nobody either",
			[]string{"N", "S"}, contractb.MapShape{}, false},
		{"the same declaration on a 3x2 map is granted without comment",
			[]string{"N", "S"}, contractb.MapShape{Width: 3, Height: 2}, false},
		{"all four edges are always usable once either axis exists",
			[]string{"E", "N", "W", "S"}, contractb.MapShape{Width: 2, Height: 1}, false},
	}
	for _, tc := range cases {
		got := exportEdgesRefusal(tc.declared, tc.shape)
		if tc.refuse && got == "" {
			t.Errorf("%s: nothing was refused", tc.name)
			continue
		}
		if !tc.refuse && got != "" {
			t.Errorf("%s: refused with %q", tc.name, got)
			continue
		}
		if !tc.refuse {
			continue
		}
		// The reason names the declared set, the map's shape AND the missing
		// axis, because a close code cannot and the operator has to act on it.
		for _, want := range []string{"exportEdges", "map", "axis"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: the reason %q does not mention %q", tc.name, got, want)
			}
		}
	}
}

// TestASetTheMapCannotUseIsRefusedAtTheHandshake is the total case end to end:
// a real 3×1 map, a real mod declaring only the column axis, and close 4007.
func TestASetTheMapCannotUseIsRefusedAtTheHandshake(t *testing.T) {
	g := newGrid(t, 3, gridOptions{layout: layoutRow(3)})
	n := g.node(2)
	// Take the conformant mod off first, so the refusal under test is the one
	// this test dialled and not a replacement.
	n.mod.close()

	bad := dialFakeMod(t, fakeModOptions{
		url:         n.side.URL(),
		world:       newWorld(),
		exportEdges: []string{contracta.EdgeN, contracta.EdgeS},
	})
	code := bad.waitClosed(5 * time.Second)
	if code != websocket.StatusCode(contracta.CloseExportEdgesUnusable) {
		t.Fatalf("the mod was closed %d, want 4007 EXPORT_EDGES_UNUSABLE", code)
	}
}

// TestA1x1MapRefusesNobody. A lone first peer on a map that has not grown yet
// is the normal opening state of every map this project will ever run, and a
// check that ejected it would make an empty map unjoinable.
func TestA1x1MapRefusesNobody(t *testing.T) {
	g := newGrid(t, 1, gridOptions{
		layout:        []contractb.Position{{Col: 0, Row: 0}},
		exportEdges:   []string{contracta.EdgeN, contracta.EdgeS},
		skipEdgeCheck: true,
	})
	n := g.node(0)
	waitSlot(t, n.side, 1)
	if shape := n.side.MapShape(); shape.Width > 1 || shape.Height > 1 {
		t.Fatalf("the test map is %+v, and this case needs one with no axis", shape)
	}
	// Give the refusal every chance to fire, then insist it did not.
	time.Sleep(300 * time.Millisecond)
	if n.mod.isClosed() {
		t.Fatal("a peer alone on a 1x1 map was refused; the map can route NOTHING, so this " +
			"declaration is not a misconfiguration")
	}
}

// TestThePartialCaseWarnsWithoutRefusing is A50's second answer, and it is the
// one the whole living deployment depends on: every mod on a w×1 rig declares
// all four edges, and refusing that would eject every peer of every single-row
// map ever run.
func TestThePartialCaseWarnsWithoutRefusing(t *testing.T) {
	logs := &syncBuffer{}
	g := newGrid(t, 3, gridOptions{
		layout: layoutRow(3),
		tune: func(i int, c *Config) {
			if i == 2 {
				c.Logger = slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
			}
		},
	})
	n := g.node(2)

	waitFor(t, 5*time.Second, "the partial-case warning", func() bool {
		return strings.Contains(logs.String(), "does not have")
	})
	if n.mod.isClosed() {
		t.Fatal("a partially usable declaration ended the session; A50 says it is legal and " +
			"unchanged")
	}
	line := logs.String()
	// It names the edge and the map's shape, and says the edge will stay closed.
	for _, want := range []string{"edge=N", "map=3x1", "stay closed"} {
		if !strings.Contains(line, want) {
			t.Fatalf("the partial-case line does not carry %q:\n%s", want, line)
		}
	}
	// AND IT STATES NO REMEDY, because the remedy is a map that grows an axis
	// and that is nobody at this machine's to apply. A line that told this
	// operator to fix it would send them looking for a setting that does not
	// exist on their computer.
	if strings.Contains(line, "remedy") || strings.Contains(line, "MULTIVERSE_EXPORT_EDGES") {
		t.Fatalf("the partial-case line states a remedy this machine cannot apply:\n%s", line)
	}
	// The usable edge goes on working: the world exports east and receives on
	// every declared edge, exactly as before.
	n.mod.waitEdge(contracta.EdgeN, false, 5*time.Second)
	if n.mod.isClosed() {
		t.Fatal("the session ended while its usable edges were still running")
	}
}

// TestTheRefusalNamesTheRemedyAndWhoMustAct is what the close code cannot do,
// and it is WP7's taxonomy rule arriving one package early: this refusal is the
// first one M5 invents on this wire, and it names the remedy AND the actor.
func TestTheRefusalNamesTheRemedyAndWhoMustAct(t *testing.T) {
	logs := &syncBuffer{}
	g := newGrid(t, 3, gridOptions{
		layout: layoutRow(3),
		tune: func(i int, c *Config) {
			if i == 2 {
				c.Logger = slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
			}
		},
	})
	n := g.node(2)
	n.mod.close()

	bad := dialFakeMod(t, fakeModOptions{
		url: n.side.URL(), world: newWorld(),
		exportEdges: []string{contracta.EdgeN, contracta.EdgeS},
	})
	if code := bad.waitClosed(5 * time.Second); code != websocket.StatusCode(contracta.CloseExportEdgesUnusable) {
		t.Fatalf("closed %d, want 4007", code)
	}
	line := logs.String()
	for _, want := range []string{
		"refusing session",
		"MULTIVERSE_EXPORT_EDGES",  // the remedy, by name
		"operator of THIS machine", // who must act
		"no other peer is affected",
		"MUST NOT reconnect", // §13 A8: a redial loop with a config file at the bottom of it
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("the refusal log does not carry %q:\n%s", want, line)
		}
	}
}

// TestAMapChangeNeverEndsARunningSession is the rule that keeps this check from
// ever costing an organism. Only a CONFIG_UPDATE may be refused: if another
// peer's departure is what turned a usable declaration unusable, the affected
// edges close by the ordinary rules and the world goes on RECEIVING on every
// declared edge.
func TestAMapChangeNeverEndsARunningSession(t *testing.T) {
	// A 2×2 map: both axes exist, and this peer declares only the column one.
	g := newGrid(t, 4, gridOptions{
		layout: []contractb.Position{
			{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 0, Row: 1}, {Col: 1, Row: 1},
		},
		exportEdges:   []string{contracta.EdgeN, contracta.EdgeS},
		skipEdgeCheck: true,
	})
	n := g.node(0)
	waitFor(t, 5*time.Second, "a map with a column axis", func() bool {
		return n.side.MapShape().Height >= 2
	})
	if n.mod.isClosed() {
		t.Fatal("a legal column-only declaration was refused on a map that has a column axis")
	}

	// Now simulate the map's side moving underneath it: the sidecar is handed a
	// shape with no column axis at all, which is what a released row would do.
	// A50 is explicit that this MUST NOT end the session.
	n.side.mu.Lock()
	n.side.mapShape = contractb.MapShape{Width: 4, Height: 1}
	reason := n.side.checkExportEdgesLocked(n.side.mod, false)
	n.side.mu.Unlock()
	if reason != "" {
		t.Fatalf("a map change produced a refusal (%q); only a CONFIG_UPDATE may be refused", reason)
	}
	time.Sleep(200 * time.Millisecond)
	if n.mod.isClosed() {
		t.Fatal("the session ended because the MAP changed; ejecting a peer for what another " +
			"peer's departure did is the one way this check could lose an organism")
	}
}

// syncBuffer is a log sink a test can read while a component is still writing
// to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
