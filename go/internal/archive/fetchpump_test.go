package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contractb"
	"multiverse/internal/wsutil"
)

// The pump's bounds, contract-b-m4.md §21, B21.
//
// The archive's genome fetcher used to walk its WHOLE pending map on every
// tick, under the one lock the frame handler also takes, calling the genome
// store's Has — an os.Stat — on every entry. At the 64,736-entry backlog the
// living deployment reached on 2026-08-10 that was ~0.3-1.0 s of held lock per
// one-second tick, so the read loop was admitted about once per pass; the
// relay's 128-frame outbound queue to the archive filled in about four seconds
// and the relay closed the session with 1011 "outbound queue full". The
// resubscribe changed nothing about the backlog, so it happened again: 26 drops
// in 30 minutes, and 7,789 crossings the ledger will never hold.
//
// genomeRequestsPerMinute bounded the RATE and neither of the two things that
// actually broke: how much work one pass does, and how many requests leave in
// one burst. These tests hold both bounds, and hold the ladder still while they
// do it.

// quietArchive builds an archive whose WARN lines are dropped before they are
// formatted. The no-peer path logs one per examined gap and these tests examine
// tens of thousands.
func quietArchive(t *testing.T) *Archive {
	t.Helper()
	a, err := New(Config{
		DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test",
		Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

// drainingConn dials a WebSocket server that reads and throws away everything,
// so sendLocked exercises the real wsutil path rather than a stub.
func drainingConn(t *testing.T) *wsutil.Conn {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		for {
			if _, _, err := ws.Read(r.Context()); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn := wsutil.New(ws, 256)
	t.Cleanup(func() { conn.Close(websocket.StatusNormalClosure, "test over") })
	return conn
}

func testHash(i int) string {
	s := sha256.Sum256([]byte(fmt.Sprintf("gap-%d", i)))
	return "bb8-genome/1:sha256:" + hex.EncodeToString(s[:])
}

// seedGaps tracks n hashes as pending. A sourcePeer of "" means nextPeerLocked
// finds nobody to ask, which is the cheap way to make an entry eligible without
// putting anything on the wire. The crossing is dated now, so these gaps are
// inside any retention horizon a test sets (§23, B34).
func seedGaps(a *Archive, n int, sourcePeer string, now time.Time) []string {
	hashes := make([]string, n)
	a.mu.Lock()
	for i := 0; i < n; i++ {
		hashes[i] = testHash(i)
		a.trackLocked(hashes[i], sourcePeer, fmt.Sprintf("m-%d", i), int32(i), now, now)
	}
	a.mu.Unlock()
	return hashes
}

func (a *Archive) inFlightCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, f := range a.pending {
		if f.inFlight != "" {
			n++
		}
	}
	return n
}

// TestPumpWorkPerTickIsBoundedByTheScanBudgetNotTheBacklog is the anti-starvation
// property in its structural form. One tick may examine at most ScanPerTick
// entries however large the backlog is, and every entry it examines is pushed
// back so nothing is lost. Before B21 this number was len(pending) and the read
// loop waited for all of it.
func TestPumpWorkPerTickIsBoundedByTheScanBudgetNotTheBacklog(t *testing.T) {
	a := quietArchive(t)
	const backlog = 40000
	now := time.Now()
	seedGaps(a, backlog, "", now)

	a.mu.Lock()
	a.ready = true
	before := len(a.pendingOrder)
	a.mu.Unlock()
	if before != backlog {
		t.Fatalf("seed: pendingOrder is %d, want %d", before, backlog)
	}

	a.pumpFetches(now)

	a.mu.Lock()
	examined := a.pendingHead
	order := len(a.pendingOrder)
	gaps := len(a.pending)
	a.mu.Unlock()

	if examined != a.cfg.ScanPerTick {
		t.Fatalf("one tick examined %d entries, want exactly the scan budget %d "+
			"(an unbounded walk over %d is what starved the read loop)",
			examined, a.cfg.ScanPerTick, backlog)
	}
	if order != backlog+a.cfg.ScanPerTick {
		t.Fatalf("pendingOrder is %d, want %d: every examined entry must be pushed back",
			order, backlog+a.cfg.ScanPerTick)
	}
	if gaps != backlog {
		t.Fatalf("the backlog is %d, want %d: a bounded walk must not drop a gap", gaps, backlog)
	}
}

// TestOneLockAcquisitionExaminesAtMostAChunk is the anti-starvation property
// stated as the frame handler experiences it: the longest a handler can wait
// for the archive's lock is one CHUNK, not one backlog. pumpChunk takes and
// releases the lock exactly once, so bounding what it examines bounds the wait
// — and a whole tick over a large backlog is that bounded wait repeated, with
// the lock free between every pair.
//
// This is asserted structurally rather than by timing a competing goroutine,
// because a wall-clock bar on a host running five Unity worlds at ×100 is not a
// bar this repository can trust.
func TestOneLockAcquisitionExaminesAtMostAChunk(t *testing.T) {
	a := quietArchive(t)
	const backlog = 40000
	now := time.Now()
	seedGaps(a, backlog, "", now)
	a.mu.Lock()
	a.ready = true
	a.mu.Unlock()

	// A chunk asked for more work than the chunk bound allows still stops at it.
	scanned, _, more := a.pumpChunk(now, contractb.GenomeScanChunk, a.cfg.MaxRequestsPerTick)
	if scanned != contractb.GenomeScanChunk {
		t.Fatalf("one lock acquisition examined %d entries, want the chunk bound %d",
			scanned, contractb.GenomeScanChunk)
	}
	if !more {
		t.Fatal("the chunk reported no more work with 40,000 gaps still pending: " +
			"the pump would stop short of its scan budget")
	}

	// And a whole tick is that chunk repeated, so the lock is released and
	// retaken ScanPerTick/GenomeScanChunk times instead of held once.
	a.mu.Lock()
	a.pendingHead = 0
	a.mu.Unlock()
	chunks := 0
	for total := 0; total < a.cfg.ScanPerTick; chunks++ {
		s, _, ok := a.pumpChunk(now, contractb.GenomeScanChunk, a.cfg.MaxRequestsPerTick)
		total += s
		if !ok {
			break
		}
	}
	if want := a.cfg.ScanPerTick / contractb.GenomeScanChunk; chunks != want {
		t.Fatalf("a tick's scan budget took %d lock acquisition(s), want %d", chunks, want)
	}
}

// TestPumpBoundsTheBurstOfRequests is the second half of B21. The rate cap
// allows genomeRequestsPerMinute per peer and says nothing about how many of
// them may leave at once; before the bound, one pass of a 63,000-entry backlog
// spent a whole minute's allowance for every peer inside a few milliseconds.
func TestPumpBoundsTheBurstOfRequests(t *testing.T) {
	a := quietArchive(t)
	now := time.Now()
	seedGaps(a, 5000, "slot-1", now)

	a.mu.Lock()
	a.ready = true
	a.conn = drainingConn(t)
	a.status = contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{{Slot: 1, PeerID: "slot-1", Live: true}},
	}
	a.mu.Unlock()

	a.pumpFetches(now)

	if got, want := a.inFlightCount(), a.cfg.MaxRequestsPerTick; got != want {
		t.Fatalf("one tick put %d GENOME_REQUESTs on the wire, want at most the "+
			"per-tick burst bound %d (the rate cap alone would have allowed %d)",
			got, want, a.cfg.RequestsPerMinute)
	}
}

// TestPumpCapsRequestsInFlightPerPeer holds the bound the rate cap cannot: how
// many unanswered requests one peer may be carrying at any instant. It is set
// above what genomeRequestsPerMinute can sustain over a request timeout, so it
// never throttles the fetch — it only stops a pile-up.
func TestPumpCapsRequestsInFlightPerPeer(t *testing.T) {
	a := quietArchive(t)
	a.cfg.MaxRequestsPerTick = 1000
	a.cfg.RequestsPerMinute = 1000
	now := time.Now()
	seedGaps(a, 5000, "slot-1", now)

	a.mu.Lock()
	a.ready = true
	a.conn = drainingConn(t)
	a.status = contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{{Slot: 1, PeerID: "slot-1", Live: true}},
	}
	a.mu.Unlock()

	a.pumpFetches(now)
	a.pumpFetches(now)

	if got, want := a.inFlightCount(), a.cfg.MaxInFlightPerPeer; got != want {
		t.Fatalf("%d requests are outstanding to slot-1, want the in-flight cap %d", got, want)
	}
	a.mu.Lock()
	counted := a.inFlight["slot-1"]
	a.mu.Unlock()
	if counted != a.cfg.MaxInFlightPerPeer {
		t.Fatalf("the per-peer in-flight counter reads %d, want %d", counted, a.cfg.MaxInFlightPerPeer)
	}
}

// TestANewSessionDoesNotInheritInFlightSlots covers the resubscribe. Requests
// sent on a session that ended can never be answered, so a new session must not
// start with its budget already spent — that is how a flap turns into a stall.
// What resets is the ACCOUNTING only: the ladder is untouched.
func TestANewSessionDoesNotInheritInFlightSlots(t *testing.T) {
	a := quietArchive(t)
	now := time.Now()
	hashes := seedGaps(a, 100, "slot-1", now)

	a.mu.Lock()
	a.ready = true
	a.conn = drainingConn(t)
	a.status = contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{{Slot: 1, PeerID: "slot-1", Live: true}},
	}
	a.mu.Unlock()

	a.pumpFetches(now)
	if a.inFlightCount() == 0 {
		t.Fatal("the first pump asked for nothing")
	}

	// Record the ladder state of every asked fetch, then start a new session the
	// way session() does.
	type ladder struct {
		attempts int
		nextAt   time.Time
		deadline time.Time
		inFlight string
	}
	before := map[string]ladder{}
	a.mu.Lock()
	for _, h := range hashes {
		f := a.pending[h]
		before[h] = ladder{f.attempts, f.nextAt, f.deadline, f.inFlight}
	}
	a.sessionGen++
	a.inFlight = map[string]int{}
	a.mu.Unlock()

	a.mu.Lock()
	if len(a.inFlight) != 0 {
		t.Fatalf("a new session inherited %d in-flight slot(s)", len(a.inFlight))
	}
	for _, h := range hashes {
		f, b := a.pending[h], before[h]
		if f.attempts != b.attempts || !f.nextAt.Equal(b.nextAt) ||
			!f.deadline.Equal(b.deadline) || f.inFlight != b.inFlight {
			a.mu.Unlock()
			t.Fatalf("the session reset moved the ladder on %s: attempts %d->%d, nextAt %v->%v",
				h, b.attempts, f.attempts, b.nextAt, f.nextAt)
		}
	}
	a.mu.Unlock()

	// And the fresh session can ask again at once, rather than waiting out a
	// request timeout for slots nobody will ever free.
	a.pumpFetches(now)
	a.mu.Lock()
	regained := a.inFlight["slot-1"]
	a.mu.Unlock()
	if regained == 0 {
		t.Fatal("the new session could not issue a request: its in-flight budget was still spent")
	}
}

// TestEveryGapGetsItsTurn is what the bounded walk owes the ladder. Go's map
// iteration is random, so a budgeted walk over the map alone could leave a hash
// unexamined for an unbounded number of ticks. The walk is round-robin over a
// stable order instead, so a full backlog is covered in exactly
// ceil(len(pending)/ScanPerTick) ticks and no gap is starved.
func TestEveryGapGetsItsTurn(t *testing.T) {
	a := quietArchive(t)
	const backlog = 20000
	now := time.Now()
	hashes := seedGaps(a, backlog, "", now)
	a.mu.Lock()
	a.ready = true
	a.mu.Unlock()

	ticks := (backlog + a.cfg.ScanPerTick - 1) / a.cfg.ScanPerTick
	for i := 0; i < ticks; i++ {
		a.pumpFetches(now)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	for _, h := range hashes {
		f := a.pending[h]
		if f == nil {
			t.Fatalf("%s left the backlog", h)
		}
		// Nobody to ask, so an examined entry is exactly one that took an attempt.
		if f.attempts != 1 {
			t.Fatalf("%s was examined %d time(s) in one full cycle of %d tick(s), want 1",
				h, f.attempts, ticks)
		}
	}
}

// TestTheBoundedWalkKeepsTheRetryLadder is the promise the fix makes to §10:
// it changes HOW MANY gaps are asked per pump and never WHEN one is retried.
// The rungs are 1 minute, 5 minutes, 30 minutes, 6 hours, then daily, and a
// gap nobody can serve walks them one tick at a time exactly as before.
func TestTheBoundedWalkKeepsTheRetryLadder(t *testing.T) {
	a := quietArchive(t)
	now := time.Now()
	h := testHash(1)
	a.mu.Lock()
	a.trackLocked(h, "", "m-1", 1, now, now)
	a.ready = true
	a.mu.Unlock()

	for i, want := range DefaultRetrySchedule {
		a.pumpFetches(now)
		a.mu.Lock()
		f := a.pending[h]
		got := f.nextAt.Sub(now)
		attempts := f.attempts
		// Wind the clock to the moment the ladder says this gap is due again.
		now = f.nextAt
		a.mu.Unlock()
		if attempts != i+1 {
			t.Fatalf("rung %d: attempts is %d, want %d", i+1, attempts, i+1)
		}
		if got != want {
			t.Fatalf("rung %d: the ladder waits %v, want %v", i+1, got, want)
		}
	}
	// Past the last rung it stays daily.
	a.pumpFetches(now)
	a.mu.Lock()
	got := a.pending[h].nextAt.Sub(now)
	a.mu.Unlock()
	if got != DefaultRetrySchedule[len(DefaultRetrySchedule)-1] {
		t.Fatalf("past the last rung the ladder waits %v, want daily", got)
	}
}

// TestASmallBacklogIsStillWalkedWholeEachTick guards the ordinary case. The
// bound must not slow down an archive that is keeping up: below the scan budget
// every gap is examined every tick, which is what it always was.
func TestASmallBacklogIsStillWalkedWholeEachTick(t *testing.T) {
	a := quietArchive(t)
	now := time.Now()
	hashes := seedGaps(a, 12, "", now)
	a.mu.Lock()
	a.ready = true
	a.mu.Unlock()

	a.pumpFetches(now)

	a.mu.Lock()
	defer a.mu.Unlock()
	for _, h := range hashes {
		if got := a.pending[h].attempts; got != 1 {
			t.Fatalf("%s took %d attempt(s) in one tick over a backlog of 12, want 1", h, got)
		}
	}
	// And the ring did not grow without bound behind the cursor.
	if len(a.pendingOrder) > 2*len(hashes)+1024 {
		t.Fatalf("pendingOrder grew to %d for %d gaps", len(a.pendingOrder), len(hashes))
	}
}

// TestResolvedGapsLeaveTheRing keeps the round-robin order from becoming a leak.
// A hash whose genome arrives is deleted from the map, and its slot in the ring
// must be reclaimed rather than walked over forever.
func TestResolvedGapsLeaveTheRing(t *testing.T) {
	a := quietArchive(t)
	now := time.Now()
	hashes := seedGaps(a, 3000, "", now)
	a.mu.Lock()
	a.ready = true
	for _, h := range hashes[:2500] {
		delete(a.pending, h)
	}
	a.mu.Unlock()

	for i := 0; i < 4; i++ {
		a.pumpFetches(now)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.pending) != 500 {
		t.Fatalf("the backlog is %d, want 500", len(a.pending))
	}
	if len(a.pendingOrder)-a.pendingHead > 1000 {
		t.Fatalf("the ring holds %d live slots for 500 gaps: stale hashes are not being reclaimed",
			len(a.pendingOrder)-a.pendingHead)
	}
}
