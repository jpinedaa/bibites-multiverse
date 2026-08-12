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
	"errors"
	"testing"

	"github.com/coder/websocket"
)

// newTestConn is a Conn with a bounded queue and NO writer goroutine, so nothing
// drains what a test enqueues. It never touches a socket, which is why every
// method under test here is one that only touches the queue.
func newTestConn(queue int) *Conn {
	return &Conn{
		out:  make(chan []byte, queue),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
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
