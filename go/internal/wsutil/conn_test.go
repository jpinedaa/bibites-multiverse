package wsutil

// The one behavioural difference between Send and TrySend, tested at the channel
// rather than over a socket.
//
// WHY IT IS TESTED HERE AND NOT THROUGH A RELAY. The property is "what happens
// when the outbound queue is full", and filling a real queue behind a real
// writer goroutine over a real loopback socket is a race with the kernel's own
// buffers — a test that tried it would pass or fail on how much the host felt
// like buffering. The Conn is constructed directly instead, with a queue of one
// and no writer draining it, so "full" is a fact rather than a hope.
//
// WHAT IT PROTECTS. contract-b-m4.md §6.12 (added — §22, B26) says the relay MUST
// NOT delay, block or fail a forward on account of a receipt it could not send,
// and that a receipt is dropped rather than queued indefinitely. Send's answer to
// a full queue is to CLOSE the connection — right for every frame a peer needs
// and wrong for the one frame that is explicitly allowed to go missing. If
// TrySend ever grew Send's behaviour, a full queue would turn the cheapest frame
// on the wire into a disconnected sender.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// newTestConn is a Conn with a bounded queue and NO writer goroutine, so nothing
// drains what a test enqueues. It never touches a socket, which is why every
// method under test here is one that only touches the queue.
func newTestConn(queue int) *Conn {
	return &Conn{
		out:       make(chan []byte, queue),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		force:     make(chan struct{}),
		pacedWake: make(chan struct{}, 1),
	}
}

func newPacedTestConn(queue, maxFrames int, maxBytes int64, controlBurst int) *Conn {
	c := newTestConn(queue)
	c.pacedConfig = PacedConfig{
		Pacer:        &testIntervalPacer{},
		MaxFrames:    maxFrames,
		MaxBytes:     maxBytes,
		ControlBurst: controlBurst,
	}
	return c
}

func (c *Conn) isStopping() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopping
}

func TestTrySendDropsAFullQueueAndKeepsTheConnection(t *testing.T) {
	c := newTestConn(1)
	if err := c.TrySend([]byte("first")); err != nil {
		t.Fatalf("the first frame into an empty queue failed: %v", err)
	}
	err := c.TrySend([]byte("second"))
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("a frame into a full queue returned %v, want ErrQueueFull", err)
	}
	if c.isStopping() {
		t.Fatal("TrySend closed the connection over a dropped frame; contract-b-m4.md §6.12 " +
			"says a receipt is DROPPED, not queued and not fatal, and that nothing about a " +
			"receipt may fail a forward")
	}
	// And the connection still works: the drop cost one frame and nothing else.
	<-c.out
	if err := c.TrySend([]byte("third")); err != nil {
		t.Fatalf("the queue did not recover after a drop: %v", err)
	}
}

func TestSendStillClosesAFullQueue(t *testing.T) {
	// The contrast is the point. Every frame Send carries is one the peer needs,
	// so a peer that has stopped reading has failed and an unbounded buffer is
	// the wrong answer (contract-a.md §11.1).
	c := newTestConn(1)
	if err := c.Send([]byte("first")); err != nil {
		t.Fatalf("the first frame failed: %v", err)
	}
	if err := c.Send([]byte("second")); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Send returned %v on a full queue, want ErrQueueFull", err)
	}
	if !c.isStopping() {
		t.Fatal("Send did NOT begin closing a connection whose outbound queue is full; that is " +
			"the behaviour TrySend exists to opt out of, and if it has changed then TrySend is " +
			"no longer a distinction")
	}
}

func TestBothRefuseAfterTheCloseHasBegun(t *testing.T) {
	c := newTestConn(4)
	c.Close(websocket.StatusNormalClosure, "test")
	if err := c.Send([]byte("x")); !errors.Is(err, ErrClosed) {
		t.Fatalf("Send after Close returned %v, want ErrClosed", err)
	}
	if err := c.TrySend([]byte("x")); !errors.Is(err, ErrClosed) {
		t.Fatalf("TrySend after Close returned %v, want ErrClosed", err)
	}
}

func TestPacedQueueEnforcesFrameAndRetainedByteBoundsWithoutClosing(t *testing.T) {
	c := newPacedTestConn(4, 2, 5, 0)
	if err := c.TrySendPaced([]byte("abc")); err != nil {
		t.Fatalf("first paced enqueue failed: %v", err)
	}
	if err := c.TrySendPaced([]byte("de")); err != nil {
		t.Fatalf("enqueue at both exact limits failed: %v", err)
	}
	if err := c.TrySendPaced([]byte("x")); !errors.Is(err, ErrPacedQueueFull) {
		t.Fatalf("enqueue beyond frame bound returned %v, want ErrPacedQueueFull", err)
	}
	if c.isStopping() {
		t.Fatal("paced queue saturation closed the connection")
	}

	if _, ok := c.takePaced(); !ok {
		t.Fatal("could not remove the first queued paced frame")
	}
	if err := c.TrySendPaced([]byte("wxyz")); !errors.Is(err, ErrPacedQueueFull) {
		t.Fatalf("enqueue beyond retained-byte bound returned %v, want ErrPacedQueueFull", err)
	}
	if c.isStopping() {
		t.Fatal("retained-byte saturation closed the connection")
	}
	if err := c.Send([]byte("ordinary still works")); err != nil {
		t.Fatalf("ordinary transport did not remain usable after a paced refusal: %v", err)
	}

	oneFrame := newPacedTestConn(1, 1, 100, 0)
	if err := oneFrame.TrySendPaced([]byte{0xff, 0x00}); err != nil {
		t.Fatalf("opaque bytes were rejected: %v", err)
	}
	if err := oneFrame.TrySendPaced([]byte("second")); !errors.Is(err, ErrPacedQueueFull) {
		t.Fatalf("independent frame bound returned %v, want ErrPacedQueueFull", err)
	}

	tooLarge := newPacedTestConn(1, 10, 4, 0)
	if err := tooLarge.TrySendPaced([]byte("12345")); !errors.Is(err, ErrPacedQueueFull) {
		t.Fatalf("one frame larger than the byte bound returned %v, want ErrPacedQueueFull", err)
	}
	if err := tooLarge.TrySendPaced([]byte("1234")); err != nil {
		t.Fatalf("a refusal consumed retained-byte capacity: %v", err)
	}
}

func TestPacedQueueDisabledIsDistinctFromSaturation(t *testing.T) {
	c := newTestConn(1)
	if err := c.TrySendPaced([]byte("x")); !errors.Is(err, ErrPacedQueueDisabled) {
		t.Fatalf("TrySendPaced without configuration returned %v, want ErrPacedQueueDisabled", err)
	}
}

func TestPacedAdmissionCountsTheWritersInFlightFrame(t *testing.T) {
	// takePaced models the single writer after it has selected the frame and
	// while its physical WebSocket write is blocked. The frame has left the
	// FIFO, but its []byte is still retained by the writer and must continue to
	// consume both admission limits.
	frameBound := newPacedTestConn(1, 1, 100, 0)
	if err := frameBound.TrySendPaced([]byte("in flight")); err != nil {
		t.Fatal(err)
	}
	inFlight, ok := frameBound.takePaced()
	if !ok {
		t.Fatal("writer could not select its paced frame")
	}
	if err := frameBound.TrySendPaced([]byte("replacement")); !errors.Is(err, ErrPacedQueueFull) {
		t.Fatalf("frame-bound admission beside an in-flight frame returned %v, want ErrPacedQueueFull", err)
	}
	frameBound.releasePaced(inFlight)
	if err := frameBound.TrySendPaced([]byte("after write")); err != nil {
		t.Fatalf("completed physical write did not release the frame allowance: %v", err)
	}

	byteBound := newPacedTestConn(1, 2, 4, 0)
	if err := byteBound.TrySendPaced([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	inFlight, ok = byteBound.takePaced()
	if !ok {
		t.Fatal("writer could not select its byte-shaped paced frame")
	}
	if err := byteBound.TrySendPaced([]byte("x")); !errors.Is(err, ErrPacedQueueFull) {
		t.Fatalf("byte-bound admission beside an in-flight frame returned %v, want ErrPacedQueueFull", err)
	}
	byteBound.releasePaced(inFlight)
	if err := byteBound.TrySendPaced([]byte("x")); err != nil {
		t.Fatalf("completed physical write did not release the byte allowance: %v", err)
	}
}

func TestOrdinarySendWakesIdleWriter(t *testing.T) {
	serverWS, clientWS, cleanup := websocketPair(t)
	defer cleanup()

	c := NewLimited(serverWS, 2, 1024)
	// Let the writer reach its no-work select. Send must wake that select.
	time.Sleep(10 * time.Millisecond)
	if err := c.Send([]byte("ordinary")); err != nil {
		t.Fatal(err)
	}
	got, _ := readTextFrames(t, clientWS, 1)
	if got[0] != "ordinary" {
		t.Fatalf("received %q, want ordinary", got[0])
	}
	c.CloseNow()
	waitDone(t, c)
}

func TestWriterBoundsControlPriorityWithoutStarvingDuePacedFrame(t *testing.T) {
	serverWS, clientWS, cleanup := websocketPair(t)
	defer cleanup()

	c := newConn(serverWS, 8, 1024, PacedConfig{
		Pacer:        &testIntervalPacer{},
		MaxFrames:    4,
		MaxBytes:     1024,
		ControlBurst: 2,
	})
	if err := c.TrySendPaced([]byte("paced")); err != nil {
		t.Fatal(err)
	}
	for _, frame := range []string{"control-1", "control-2", "control-3"} {
		if err := c.Send([]byte(frame)); err != nil {
			t.Fatalf("enqueue %q: %v", frame, err)
		}
	}
	go c.writeLoop()

	got, _ := readTextFrames(t, clientWS, 4)
	want := []string{"control-1", "control-2", "paced", "control-3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("frame %d = %q, want %q; due paced frame must advance after the control burst", i, got[i], want[i])
		}
	}
	c.CloseNow()
	waitDone(t, c)
}

func TestPaceBoundaryDeterministicallyOrdersSelectedOrdinaryFrame(t *testing.T) {
	t.Run("due", func(t *testing.T) {
		serverWS, clientWS, cleanup := websocketPair(t)
		defer cleanup()

		c := newConn(serverWS, 2, 1024, PacedConfig{
			Pacer:        &testIntervalPacer{},
			MaxFrames:    2,
			MaxBytes:     1024,
			ControlBurst: 2,
		})
		for _, frame := range []string{"paced-due", "paced-next"} {
			if err := c.TrySendPaced([]byte(frame)); err != nil {
				t.Fatal(err)
			}
		}

		pacedFirst, ordinaryAfter := c.writeOrdinaryAtPaceBoundary([]byte("ordinary"))
		if !pacedFirst || !ordinaryAfter {
			t.Fatalf("boundary result = pacedFirst:%v ordinaryAfter:%v, want both true", pacedFirst, ordinaryAfter)
		}
		got, _ := readTextFrames(t, clientWS, 2)
		if got[0] != "paced-due" || got[1] != "ordinary" {
			t.Fatalf("boundary order = %q, want paced-due before selected ordinary", got)
		}
		if burst := c.controlBurstAfterPaced(ordinaryAfter); burst != 1 {
			t.Fatalf("next paced frame sees control burst %d, want selected ordinary counted as 1", burst)
		}
		c.mu.Lock()
		inFlight, inFlightBytes := c.pacedInFlight, c.pacedInFlightBytes
		queued, queuedBytes := len(c.paced), c.pacedBytes
		c.mu.Unlock()
		if inFlight != 0 || inFlightBytes != 0 || queued != 1 || queuedBytes != int64(len("paced-next")) {
			t.Fatalf("paced accounting after boundary = inFlight:%d/%d queued:%d/%d", inFlight, inFlightBytes, queued, queuedBytes)
		}
	})

	t.Run("not due", func(t *testing.T) {
		serverWS, clientWS, cleanup := websocketPair(t)
		defer cleanup()

		c := newConn(serverWS, 1, 1024, PacedConfig{
			Pacer:        newTestDelayedPacer(time.Hour, 0),
			MaxFrames:    1,
			MaxBytes:     1024,
			ControlBurst: 0,
		})
		if err := c.TrySendPaced([]byte("paced-later")); err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		pacedFirst, ordinaryAfter := c.writeOrdinaryAtPaceBoundary([]byte("ordinary"))
		if pacedFirst || ordinaryAfter {
			t.Fatalf("not-due boundary result = pacedFirst:%v ordinaryAfter:%v, want both false", pacedFirst, ordinaryAfter)
		}
		if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
			t.Fatalf("not-due ordinary frame waited %v for the pace turn", elapsed)
		}
		got, _ := readTextFrames(t, clientWS, 1)
		if got[0] != "ordinary" {
			t.Fatalf("not-due boundary wrote %q, want ordinary", got[0])
		}
		if !c.pacedPending() {
			t.Fatal("not-due boundary consumed the paced frame")
		}
	})
}

func TestOrdinaryFrameRemainsSendableWhilePaceTimerRuns(t *testing.T) {
	serverWS, clientWS, cleanup := websocketPair(t)
	defer cleanup()

	pacer := newTestDelayedPacer(180*time.Millisecond, 0)
	c := NewPacedLimited(serverWS, 8, 1024, PacedConfig{
		Pacer:        pacer,
		MaxFrames:    2,
		MaxBytes:     1024,
		ControlBurst: 0,
	})
	if err := c.TrySendPaced([]byte("paced")); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, pacer.waiting)
	if err := c.Send([]byte("control")); err != nil {
		t.Fatal(err)
	}

	got, at := readTextFrames(t, clientWS, 2)
	if got[0] != "control" || got[1] != "paced" {
		t.Fatalf("received %q, want control before paced", got)
	}
	if at[1].Sub(at[0]) < 90*time.Millisecond {
		t.Fatalf("paced frame arrived only %v after control; pace timer did not remain in force", at[1].Sub(at[0]))
	}
	c.CloseNow()
	waitDone(t, c)
}

func TestPacedFramesArePhysicallySpaced(t *testing.T) {
	serverWS, clientWS, cleanup := websocketPair(t)
	defer cleanup()

	const interval = 60 * time.Millisecond
	c := newConn(serverWS, 4, 1024, PacedConfig{
		Pacer:        &testIntervalPacer{interval: interval},
		MaxFrames:    3,
		MaxBytes:     1024,
		ControlBurst: 0,
	})
	for _, frame := range []string{"one", "two", "three"} {
		if err := c.TrySendPaced([]byte(frame)); err != nil {
			t.Fatal(err)
		}
	}
	go c.writeLoop()

	got, at := readTextFrames(t, clientWS, 3)
	for i, want := range []string{"one", "two", "three"} {
		if got[i] != want {
			t.Fatalf("frame %d = %q, want %q", i, got[i], want)
		}
	}
	for i := 1; i < len(at); i++ {
		if gap := at[i].Sub(at[i-1]); gap < interval-10*time.Millisecond {
			t.Fatalf("physical write gap %d = %v, want at least %v", i, gap, interval-10*time.Millisecond)
		}
	}
	c.CloseNow()
	waitDone(t, c)
}

func TestOrdinaryCloseDropsPacedBacklogWithoutUnpacedDrain(t *testing.T) {
	serverWS, clientWS, cleanup := websocketPair(t)
	defer cleanup()

	pacer := newTestDelayedPacer(500*time.Millisecond, 0)
	c := NewPacedLimited(serverWS, 4, 1024, PacedConfig{
		Pacer:        pacer,
		MaxFrames:    2,
		MaxBytes:     1024,
		ControlBurst: 0,
	})
	if err := c.TrySendPaced([]byte("must be dropped")); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, pacer.waiting)
	c.Close(websocket.StatusNormalClosure, "ordinary close")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err := clientWS.Read(ctx)
	if err == nil {
		t.Fatal("ordinary Close unpaced-flushed a paced frame")
	}
	waitDone(t, c)
	if c.pacedPending() {
		t.Fatal("ordinary Close retained its paced backlog")
	}
}

func TestClosePacedDrainsWithPhysicalSpacing(t *testing.T) {
	serverWS, clientWS, cleanup := websocketPair(t)
	defer cleanup()

	const interval = 45 * time.Millisecond
	c := newConn(serverWS, 4, 1024, PacedConfig{
		Pacer:        &testIntervalPacer{interval: interval},
		MaxFrames:    3,
		MaxBytes:     1024,
		ControlBurst: 0,
	})
	for _, frame := range []string{"one", "two", "three"} {
		if err := c.TrySendPaced([]byte(frame)); err != nil {
			t.Fatal(err)
		}
	}
	go c.writeLoop()

	closeDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		closeDone <- c.ClosePaced(ctx, websocket.StatusNormalClosure, "paced drain")
	}()
	frames, at := readTextFrames(t, clientWS, 3)
	_ = clientWS.Close(websocket.StatusNormalClosure, "drained")
	if err := <-closeDone; err != nil {
		t.Fatalf("ClosePaced: %v", err)
	}
	for i, want := range []string{"one", "two", "three"} {
		if frames[i] != want {
			t.Fatalf("frame %d = %q, want %q", i, frames[i], want)
		}
	}
	for i := 1; i < len(at); i++ {
		if gap := at[i].Sub(at[i-1]); gap < interval-10*time.Millisecond {
			t.Fatalf("drain gap %d = %v, want at least %v", i, gap, interval-10*time.Millisecond)
		}
	}
}

func TestClosePacedForcesAtCallerDeadline(t *testing.T) {
	serverWS, clientWS, cleanup := websocketPair(t)
	defer cleanup()

	pacer := newTestDelayedPacer(time.Second, 0)
	c := NewPacedLimited(serverWS, 4, 1024, PacedConfig{
		Pacer:        pacer,
		MaxFrames:    1,
		MaxBytes:     1024,
		ControlBurst: 0,
	})
	if err := c.TrySendPaced([]byte("too late")); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, pacer.waiting)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := c.ClosePaced(ctx, websocket.StatusGoingAway, "deadline")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ClosePaced returned %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("forced close returned %v after its 80ms deadline", elapsed)
	}
	waitDone(t, c)

	readCtx, readCancel := context.WithTimeout(context.Background(), time.Second)
	defer readCancel()
	_, _, readErr := clientWS.Read(readCtx)
	if readErr == nil {
		t.Fatal("deadline force-close sent a paced frame before dropping it")
	}
}

func TestClosePacedBoundsAnAlreadyStoppingConnection(t *testing.T) {
	serverWS, _, cleanup := websocketPair(t)
	defer cleanup()

	// Build without starting the writer. This gives the ordinary Close path a
	// queue that cannot finish before the caller's hard deadline.
	c := newConn(serverWS, 1, 1024, PacedConfig{})
	if err := c.Send([]byte("ordinary backlog")); err != nil {
		t.Fatal(err)
	}
	c.Close(websocket.StatusInternalError, "ordinary close already won")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := c.ClosePaced(ctx, websocket.StatusGoingAway, "relay drain")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ClosePaced returned %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("already-stopping connection escaped its 60ms drain boundary: %v", elapsed)
	}

	// Let the writer observe the forced socket close and finish, so the test also
	// proves that the old ordinary-close state remains terminable.
	go c.writeLoop()
	waitDone(t, c)
}

func TestClosePacedPreservesAnAlreadyStoppingPolicyAndReason(t *testing.T) {
	serverWS, clientWS, cleanup := websocketPair(t)
	defer cleanup()

	c := newConn(serverWS, 1, 1024, PacedConfig{
		Pacer:        &testIntervalPacer{interval: time.Second},
		MaxFrames:    1,
		MaxBytes:     1024,
		ControlBurst: 0,
	})
	if err := c.TrySendPaced([]byte("must be dropped")); err != nil {
		t.Fatal(err)
	}
	if err := c.Send([]byte("ordinary backlog")); err != nil {
		t.Fatal(err)
	}
	c.Close(websocket.StatusInternalError, "original close reason")

	closeDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		closeDone <- c.ClosePaced(ctx, websocket.StatusGoingAway, "later relay drain")
	}()
	go c.writeLoop()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	typ, frame, err := clientWS.Read(ctx)
	if err != nil || typ != websocket.MessageText || string(frame) != "ordinary backlog" {
		t.Fatalf("ordinary drain = type %v frame %q err %v", typ, frame, err)
	}
	_, _, err = clientWS.Read(ctx)
	var closeErr websocket.CloseError
	if !errors.As(err, &closeErr) {
		t.Fatalf("final read error = %v, want WebSocket close", err)
	}
	if closeErr.Code != websocket.StatusInternalError || closeErr.Reason != "original close reason" {
		t.Fatalf("close = %d %q, want original %d %q", closeErr.Code, closeErr.Reason,
			websocket.StatusInternalError, "original close reason")
	}
	if err := <-closeDone; err != nil && !errors.Is(err, ErrClosed) {
		t.Fatalf("ClosePaced after ordinary close = %v, want nil or ErrClosed", err)
	}
	if c.pacedPending() {
		t.Fatal("ClosePaced changed the winning ordinary close into a paced drain")
	}
}

func TestClosePacedWaitsForPeerCloseHandshake(t *testing.T) {
	serverWS, clientWS, cleanup := websocketPair(t)
	defer cleanup()

	c := NewLimited(serverWS, 1, 1024)
	closed := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		closed <- c.ClosePaced(ctx, websocket.StatusNormalClosure, "handshake")
	}()

	select {
	case err := <-closed:
		t.Fatalf("ClosePaced returned %v before the peer answered the close handshake", err)
	case <-time.After(80 * time.Millisecond):
	}
	if err := clientWS.Close(websocket.StatusNormalClosure, "peer answer"); err != nil {
		t.Fatalf("peer close response: %v", err)
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("ClosePaced after peer response: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ClosePaced did not finish after the peer answered")
	}
}

func TestClosePacedRequiresCallerDeadline(t *testing.T) {
	c := newPacedTestConn(1, 1, 10, 0)
	if err := c.ClosePaced(context.Background(), websocket.StatusNormalClosure, "no deadline"); !errors.Is(err, ErrDrainDeadlineRequired) {
		t.Fatalf("ClosePaced without deadline returned %v, want ErrDrainDeadlineRequired", err)
	}
	if c.isStopping() {
		t.Fatal("a rejected drain request changed connection state")
	}
}

func TestPacedEnqueueAndOrdinarySendAreLinearizedWithClose(t *testing.T) {
	c := newPacedTestConn(512, 512, 1<<20, 0)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			err := c.TrySendPaced([]byte("paced"))
			if err != nil && !errors.Is(err, ErrClosed) && !errors.Is(err, ErrPacedQueueFull) {
				t.Errorf("TrySendPaced racing Close returned %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			err := c.Send([]byte("ordinary"))
			if err != nil && !errors.Is(err, ErrClosed) && !errors.Is(err, ErrQueueFull) {
				t.Errorf("Send racing Close returned %v", err)
			}
		}()
	}
	close(start)
	c.Close(websocket.StatusNormalClosure, "race")
	wg.Wait()
	if err := c.TrySendPaced([]byte("after")); !errors.Is(err, ErrClosed) {
		t.Fatalf("paced enqueue after Close returned %v, want ErrClosed", err)
	}
	if err := c.Send([]byte("after")); !errors.Is(err, ErrClosed) {
		t.Fatalf("ordinary send after Close returned %v, want ErrClosed", err)
	}
}

type testIntervalPacer struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
}

func (p *testIntervalPacer) Reserve(now time.Time) time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.next.IsZero() && now.Before(p.next) {
		return p.next.Sub(now)
	}
	p.next = now.Add(p.interval)
	return 0
}

type testDelayedPacer struct {
	mu       sync.Mutex
	next     time.Time
	initial  time.Duration
	interval time.Duration
	waiting  chan struct{}
	waitOnce sync.Once
}

func newTestDelayedPacer(initial, interval time.Duration) *testDelayedPacer {
	return &testDelayedPacer{
		initial:  initial,
		interval: interval,
		waiting:  make(chan struct{}),
	}
}

func (p *testDelayedPacer) Reserve(now time.Time) time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.next.IsZero() && p.initial > 0 {
		p.next = now.Add(p.initial)
		p.waitOnce.Do(func() { close(p.waiting) })
		return p.initial
	}
	if now.Before(p.next) {
		p.waitOnce.Do(func() { close(p.waiting) })
		return p.next.Sub(now)
	}
	p.next = now.Add(p.interval)
	return 0
}

func websocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()
	type acceptResult struct {
		ws  *websocket.Conn
		err error
	}
	accepted := make(chan acceptResult, 1)
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		// This workstation can have local service probes. A request outside /ws
		// never reaches this handler, and an extra successful WebSocket must not
		// leave the one-shot result channel or httptest.Server.Close blocked.
		select {
		case accepted <- acceptResult{ws: ws}:
			<-release
		default:
			_ = ws.CloseNow()
		}
	})
	server := httptest.NewServer(mux)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	clientWS, _, err := websocket.Dial(ctx,
		"ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial test WebSocket: %v", err)
	}
	result := <-accepted
	if result.err != nil {
		_ = clientWS.CloseNow()
		server.Close()
		t.Fatalf("accept test WebSocket: %v", result.err)
	}
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			_ = clientWS.CloseNow()
			_ = result.ws.CloseNow()
			close(release)
			server.Close()
		})
	}
	t.Cleanup(cleanup)
	return result.ws, clientWS, cleanup
}

func readTextFrames(t *testing.T, ws *websocket.Conn, count int) ([]string, []time.Time) {
	t.Helper()
	frames := make([]string, 0, count)
	at := make([]time.Time, 0, count)
	for len(frames) < count {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		typ, frame, err := ws.Read(ctx)
		cancel()
		if err != nil {
			t.Fatalf("read frame %d of %d: %v", len(frames)+1, count, err)
		}
		if typ != websocket.MessageText {
			t.Fatalf("frame %d has type %v, want text", len(frames)+1, typ)
		}
		frames = append(frames, string(frame))
		at = append(at, time.Now())
	}
	return frames, at
}

func waitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for pacer")
	}
}

func waitDone(t *testing.T, c *Conn) {
	t.Helper()
	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for writer to finish")
	}
}
