// Package wsutil wraps a WebSocket connection in the one-writer-goroutine
// discipline both contracts require: every mainstream Go WebSocket library
// forbids concurrent writes, so nothing writes from a handler
// (contract-a.md §11.1).
//
// The outbound queue is bounded. A peer that stops reading gets its connection
// closed rather than an ever-growing buffer.
package wsutil

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/wire"
)

// ErrQueueFull is returned by Send when the outbound queue is saturated. The
// caller should treat it as fatal for the connection.
var ErrQueueFull = errors.New("wsutil: outbound queue full")

// ErrClosed is returned by Send after the connection has begun closing.
var ErrClosed = errors.New("wsutil: connection closed")

const (
	writeTimeout = 10 * time.Second
	drainTimeout = 2 * time.Second
)

// Conn is a WebSocket connection with one writer goroutine.
type Conn struct {
	ws   *websocket.Conn
	out  chan []byte
	stop chan struct{}
	done chan struct{}

	mu       sync.Mutex
	stopping bool
	code     websocket.StatusCode
	reason   string
}

// New wraps ws and starts its writer goroutine, with the shared
// contract-a.md §10 / contract-b-m4.md §12 frame ceiling.
func New(ws *websocket.Conn, queue int) *Conn {
	return NewLimited(ws, queue, wire.MaxFrameBytes)
}

// NewLimited is New with maxFrameBytes as a KNOB rather than a constant
// (contract-b-m4.md §3.3, §22 B24). The relay takes its ceiling from its own
// published limits table; every other caller takes the shared default, because
// the table is the relay's configuration and not a peer's.
//
// Over the limit the library closes the connection with 1009 TOO_BIG, which is
// what §3.2 asks for and is deliberately NOT 4007: a frame too big is a shape
// fault with a shape remedy, and a capacity shed is a rate fault with a rate
// remedy.
func NewLimited(ws *websocket.Conn, queue int, maxFrameBytes int64) *Conn {
	if maxFrameBytes <= 0 {
		maxFrameBytes = wire.MaxFrameBytes
	}
	ws.SetReadLimit(maxFrameBytes)
	c := &Conn{
		ws:   ws,
		out:  make(chan []byte, queue),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go c.writeLoop()
	return c
}

// Send enqueues one text frame. It never blocks.
func (c *Conn) Send(frame []byte) error {
	c.mu.Lock()
	stopping := c.stopping
	c.mu.Unlock()
	if stopping {
		return ErrClosed
	}
	select {
	case c.out <- frame:
		return nil
	default:
		// contract-a.md §11.1: close rather than grow without limit.
		c.Close(websocket.StatusInternalError, "outbound queue full")
		return ErrQueueFull
	}
}

// Read returns the next text frame. Binary frames are rejected as malformed by
// the caller's contract, so they are surfaced as an error here.
func (c *Conn) Read(ctx context.Context) ([]byte, error) {
	typ, data, err := c.ws.Read(ctx)
	if err != nil {
		return nil, err
	}
	if typ != websocket.MessageText {
		return nil, errors.New("wsutil: binary frame on a text-only wire")
	}
	return data, nil
}

// Close begins a clean close with the given code. It is idempotent; the first
// call wins, which keeps the real reason for a close instead of a later
// bookkeeping one.
func (c *Conn) Close(code websocket.StatusCode, reason string) {
	c.mu.Lock()
	if c.stopping {
		c.mu.Unlock()
		return
	}
	c.stopping = true
	c.code = code
	if len(reason) > 120 {
		reason = reason[:120]
	}
	c.reason = reason
	c.mu.Unlock()
	close(c.stop)
}

// CloseNow drops the connection without a close handshake.
func (c *Conn) CloseNow() {
	c.mu.Lock()
	if !c.stopping {
		c.stopping = true
		c.code = 0
		close(c.stop)
	}
	c.mu.Unlock()
	_ = c.ws.CloseNow()
}

// Done is closed once the writer has finished and the socket is closed.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Ping sends a WebSocket ping and waits for the pong.
func (c *Conn) Ping(ctx context.Context) error { return c.ws.Ping(ctx) }

func (c *Conn) writeLoop() {
	defer close(c.done)
	for {
		select {
		case frame := <-c.out:
			if !c.write(frame, writeTimeout) {
				c.hardClose()
				return
			}
		case <-c.stop:
			c.drain()
			c.finish()
			return
		}
	}
}

func (c *Conn) drain() {
	for {
		select {
		case frame := <-c.out:
			if !c.write(frame, drainTimeout) {
				return
			}
		default:
			return
		}
	}
}

func (c *Conn) write(frame []byte, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.ws.Write(ctx, websocket.MessageText, frame) == nil
}

func (c *Conn) finish() {
	c.mu.Lock()
	code, reason := c.code, c.reason
	c.mu.Unlock()
	if code == 0 {
		_ = c.ws.CloseNow()
		return
	}
	_ = c.ws.Close(code, reason)
}

func (c *Conn) hardClose() {
	c.mu.Lock()
	if !c.stopping {
		c.stopping = true
		c.code = 0
		close(c.stop)
	}
	c.mu.Unlock()
	_ = c.ws.CloseNow()
}
