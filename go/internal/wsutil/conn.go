// Package wsutil wraps a WebSocket connection in the one-writer-goroutine
// discipline both contracts require: every mainstream Go WebSocket library
// forbids concurrent writes, so nothing writes from a handler
// (contract-a.md §11.1).
//
// The ordinary and paced outbound queues are bounded. A peer that stops reading
// gets its connection closed rather than an ever-growing buffer.
package wsutil

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/wire"
)

// ErrQueueFull is returned by Send when the ordinary outbound queue is
// saturated. The caller should treat it as fatal for the connection.
var ErrQueueFull = errors.New("wsutil: outbound queue full")

// ErrPacedQueueFull is returned by TrySendPaced when either paced queue limit
// is reached. Unlike ErrQueueFull, it does not close the connection.
var ErrPacedQueueFull = errors.New("wsutil: paced outbound queue full")

// ErrPacedQueueDisabled is returned when TrySendPaced is used on a connection
// that was made without a paced queue.
var ErrPacedQueueDisabled = errors.New("wsutil: paced outbound queue disabled")

// ErrDrainDeadlineRequired is returned when ClosePaced is called without a
// caller-owned deadline. A paced drain must never wait forever.
var ErrDrainDeadlineRequired = errors.New("wsutil: paced drain requires a deadline")

// ErrClosed is returned by a send after the connection has begun closing.
var ErrClosed = errors.New("wsutil: connection closed")

const (
	writeTimeout = 10 * time.Second
	drainTimeout = 2 * time.Second
)

// Pacer assigns physical-send slots. Reserve must make its test and update
// atomic because one pacer can be shared by several connections. It returns
// zero after it reserves a slot at now. Otherwise, it returns the finite delay
// after which the caller can try again.
type Pacer interface {
	Reserve(now time.Time) time.Duration
}

// PacedConfig defines the separate paced queue. MaxFrames and MaxBytes are
// independent hard limits. ControlBurst is the largest number of ordinary
// frames that can pass a pending paced frame once that frame is due.
type PacedConfig struct {
	Pacer        Pacer
	MaxFrames    int
	MaxBytes     int64
	ControlBurst int
}

// Conn is a WebSocket connection with one writer goroutine.
type Conn struct {
	ws    *websocket.Conn
	out   chan []byte
	stop  chan struct{}
	done  chan struct{}
	force chan struct{}

	stopOnce  sync.Once
	forceOnce sync.Once

	mu                 sync.Mutex
	stopping           bool
	drainPaced         bool
	code               websocket.StatusCode
	reason             string
	pacedConfig        PacedConfig
	paced              [][]byte
	pacedBytes         int64
	pacedInFlight      int
	pacedInFlightBytes int64
	pacedWake          chan struct{}
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
	c := newConn(ws, queue, maxFrameBytes, PacedConfig{})
	go c.writeLoop()
	return c
}

// NewPaced wraps ws with an ordinary queue and a separate paced queue.
func NewPaced(ws *websocket.Conn, queue int, paced PacedConfig) *Conn {
	return NewPacedLimited(ws, queue, wire.MaxFrameBytes, paced)
}

// NewPacedLimited is NewPaced with a configurable inbound frame ceiling.
// Invalid paced limits leave the paced queue disabled; TrySendPaced reports
// ErrPacedQueueDisabled instead of creating an unbounded queue.
func NewPacedLimited(
	ws *websocket.Conn,
	queue int,
	maxFrameBytes int64,
	paced PacedConfig,
) *Conn {
	c := newConn(ws, queue, maxFrameBytes, paced)
	go c.writeLoop()
	return c
}

func newConn(
	ws *websocket.Conn,
	queue int,
	maxFrameBytes int64,
	paced PacedConfig,
) *Conn {
	if maxFrameBytes <= 0 {
		maxFrameBytes = wire.MaxFrameBytes
	}
	if paced.ControlBurst < 0 {
		paced.ControlBurst = 0
	}
	if paced.Pacer == nil || paced.MaxFrames <= 0 || paced.MaxBytes <= 0 {
		paced = PacedConfig{}
	}
	ws.SetReadLimit(maxFrameBytes)
	return &Conn{
		ws:          ws,
		out:         make(chan []byte, queue),
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
		force:       make(chan struct{}),
		pacedConfig: paced,
		pacedWake:   make(chan struct{}, 1),
	}
}

// Send enqueues one ordinary text frame. It never blocks.
func (c *Conn) Send(frame []byte) error {
	c.mu.Lock()
	if c.stopping {
		c.mu.Unlock()
		return ErrClosed
	}
	select {
	case c.out <- frame:
		c.mu.Unlock()
		return nil
	default:
		// contract-a.md §11.1: close rather than grow without limit.
		c.stopping = true
		c.code = websocket.StatusInternalError
		c.reason = "outbound queue full"
		c.mu.Unlock()
		c.signalStop()
		return ErrQueueFull
	}
}

// TrySend enqueues one ordinary text frame and DROPS IT rather than closing the
// connection when the queue is full. It is Send's twin for a frame that is
// explicitly best effort, and there is exactly one on either wire:
// contract-b-m4.md §6.12's FORWARD_RECEIPT (added — §22, B26).
//
// The distinction is the whole reason this method exists. Send closes because a
// peer that stops reading has failed and every frame Send carries is one the
// peer needs. A receipt is not: §6.12 says the relay MUST NOT delay, block or
// fail a forward on account of a receipt it could not send, and that a receipt
// the sender never sees costs nothing but the certainty it would have added.
// Closing a healthy sender's connection to report a dropped receipt would turn
// the cheapest frame on the wire into the most expensive one.
//
// It returns ErrQueueFull on a drop and ErrClosed after the connection has begun
// closing. Both are counted by the caller, never acted on.
func (c *Conn) TrySend(frame []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopping {
		return ErrClosed
	}
	select {
	case c.out <- frame:
		return nil
	default:
		return ErrQueueFull
	}
}

// TrySendPaced enqueues one opaque text frame on the paced transport. The
// frame's bytes are not decoded or classified here. Queue saturation is
// nonfatal and is reported separately from ordinary queue saturation.
func (c *Conn) TrySendPaced(frame []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopping {
		return ErrClosed
	}
	cfg := c.pacedConfig
	if cfg.Pacer == nil {
		return ErrPacedQueueDisabled
	}
	n := int64(len(frame))
	if len(c.paced)+c.pacedInFlight >= cfg.MaxFrames ||
		n > cfg.MaxBytes-c.pacedBytes-c.pacedInFlightBytes {
		return ErrPacedQueueFull
	}
	c.paced = append(c.paced, frame)
	c.pacedBytes += n
	select {
	case c.pacedWake <- struct{}{}:
	default:
	}
	return nil
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

// Close begins a clean close with the given code. It drains ordinary frames but
// drops the paced backlog. It is idempotent; the first call wins, which keeps
// the real reason for a close instead of a later bookkeeping one.
func (c *Conn) Close(code websocket.StatusCode, reason string) {
	c.mu.Lock()
	if c.stopping {
		c.mu.Unlock()
		return
	}
	c.stopping = true
	c.code = code
	c.reason = closeReason(reason)
	c.mu.Unlock()
	c.signalStop()
}

// ClosePaced begins a clean close that drains both queues. Paced frames keep
// their physical-send spacing. The caller must supply a deadline. At that
// deadline the socket is forcibly closed and any remaining backlog is dropped.
// If another close already won, its queue policy stays in force, but this call
// still waits for it under the same deadline.
func (c *Conn) ClosePaced(
	ctx context.Context,
	code websocket.StatusCode,
	reason string,
) error {
	if ctx == nil {
		return ErrDrainDeadlineRequired
	}
	if _, ok := ctx.Deadline(); !ok {
		return ErrDrainDeadlineRequired
	}

	c.mu.Lock()
	if c.stopping {
		c.mu.Unlock()
		// Preserve the close mode that won. In particular, an ordinary error or
		// replacement close must keep dropping its paced backlog. Relay-wide drain
		// still owns one hard deadline for every socket, so wait for this writer and
		// force it closed if its ordinary drain or close handshake overruns that
		// boundary.
		select {
		case <-c.done:
			return ErrClosed
		case <-ctx.Done():
			c.forceClose()
			return ctx.Err()
		}
	}
	c.stopping = true
	c.drainPaced = true
	c.code = code
	c.reason = closeReason(reason)
	c.mu.Unlock()
	c.signalStop()

	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		c.forceClose()
		return ctx.Err()
	}
}

// CloseNow drops both queues and the connection without a close handshake.
func (c *Conn) CloseNow() {
	c.mu.Lock()
	if !c.stopping {
		c.stopping = true
		c.code = 0
	}
	c.drainPaced = false
	c.dropPacedLocked()
	c.mu.Unlock()
	c.signalStop()
	c.signalForce()
	_ = c.ws.CloseNow()
}

// Done is closed once the writer has finished and the socket is closed.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Ping sends a WebSocket ping and waits for the pong.
func (c *Conn) Ping(ctx context.Context) error { return c.ws.Ping(ctx) }

func (c *Conn) writeLoop() {
	defer close(c.done)
	controlBurst := 0
	for {
		stopping, drainPaced := c.stopState()
		if stopping && !drainPaced {
			c.dropPaced()
			c.drain()
			c.finish()
			return
		}
		if stopping && c.queuesEmpty() {
			c.finish()
			return
		}

		pacedPending := c.pacedPending()
		if pacedPending && controlBurst >= c.pacedConfig.ControlBurst {
			if wrotePaced, ordinaryAfter := c.writePacedWhenReserved(); wrotePaced {
				controlBurst = c.controlBurstAfterPaced(ordinaryAfter)
				continue
			}
			if stopping, _ := c.stopState(); stopping {
				continue
			}
			if c.forced() {
				c.hardClose()
				return
			}
		}

		select {
		case frame := <-c.out:
			if !c.write(frame, writeTimeout) {
				c.hardClose()
				return
			}
			if c.pacedPending() {
				controlBurst++
			} else {
				controlBurst = 0
			}
			continue
		default:
		}

		if pacedPending {
			if wrotePaced, ordinaryAfter := c.writePacedWhenReserved(); wrotePaced {
				controlBurst = c.controlBurstAfterPaced(ordinaryAfter)
				continue
			}
			if stopping, _ := c.stopState(); stopping {
				continue
			}
			if c.forced() {
				c.hardClose()
				return
			}
		}

		frame, haveFrame, keepGoing := c.waitForWork(stopping)
		if keepGoing {
			if haveFrame {
				if !c.write(frame, writeTimeout) {
					c.hardClose()
					return
				}
				if c.pacedPending() {
					controlBurst++
				} else {
					controlBurst = 0
				}
			}
			continue
		}
		c.hardClose()
		return
	}
}

// writePacedWhenReserved waits only until the next reservation attempt. It
// keeps consuming ordinary frames while the timer runs. Its second result says
// that one selected ordinary frame followed the paced write; the caller counts
// that frame against the next pending paced frame's control burst.
func (c *Conn) writePacedWhenReserved() (bool, bool) {
	for c.pacedPending() {
		if stopping, drainPaced := c.stopState(); stopping && !drainPaced {
			return false, false
		}
		delay := c.pacedConfig.Pacer.Reserve(time.Now())
		if delay <= 0 {
			frame, ok := c.takePaced()
			if !ok {
				return false, false
			}
			written := c.write(frame, writeTimeout)
			c.releasePaced(frame)
			if !written {
				c.hardClose()
			}
			return true, false
		}

		timer := time.NewTimer(delay)
		stop := c.stop
		if stopping, _ := c.stopState(); stopping {
			// Once a graceful close has been observed, stop is permanently
			// readable. Disable that case so the pace timer still governs the
			// drain instead of creating a busy retry loop.
			stop = nil
		}
		select {
		case frame := <-c.out:
			stopAndDrainTimer(timer)
			// The timer and c.out can become ready together, and Go selects
			// randomly among ready cases. Recheck the shared pacer after
			// selecting the ordinary frame. If the turn is now due, reserve it
			// and write the paced frame first. This makes the control-burst
			// bound deterministic at the due instant without withholding
			// ordinary traffic before that instant.
			if pacedFirst, ordinaryAfter := c.writeOrdinaryAtPaceBoundary(frame); pacedFirst {
				return true, ordinaryAfter
			}
			continue
		case <-timer.C:
			continue
		case <-c.pacedWake:
			stopAndDrainTimer(timer)
			continue
		case <-stop:
			stopAndDrainTimer(timer)
			stopping, drainPaced := c.stopState()
			if stopping && drainPaced {
				continue
			}
			return false, false
		case <-c.force:
			stopAndDrainTimer(timer)
			return false, false
		}
	}
	return false, false
}

// writeOrdinaryAtPaceBoundary resolves the race between an ordinary frame and
// an expiring pace timer. Reserve changes no state while the paced frame is not
// due, so the ordinary frame remains prompt. If Reserve takes the due slot, the
// single writer emits the paced frame before the ordinary frame it selected.
func (c *Conn) writeOrdinaryAtPaceBoundary(frame []byte) (bool, bool) {
	if delay := c.pacedConfig.Pacer.Reserve(time.Now()); delay <= 0 {
		paced, ok := c.takePaced()
		if ok {
			written := c.write(paced, writeTimeout)
			c.releasePaced(paced)
			if !written {
				c.hardClose()
				return true, false
			}
			if !c.write(frame, writeTimeout) {
				c.hardClose()
				return true, false
			}
			return true, true
		}
	}
	if !c.write(frame, writeTimeout) {
		c.hardClose()
	}
	return false, false
}

func (c *Conn) controlBurstAfterPaced(ordinaryAfter bool) int {
	if ordinaryAfter && c.pacedPending() {
		return 1
	}
	return 0
}

func (c *Conn) waitForWork(alreadyStopping bool) ([]byte, bool, bool) {
	if alreadyStopping {
		select {
		case frame := <-c.out:
			return frame, true, true
		case <-c.pacedWake:
			return nil, false, true
		case <-c.force:
			return nil, false, false
		}
	}
	select {
	case frame := <-c.out:
		return frame, true, true
	case <-c.pacedWake:
		return nil, false, true
	case <-c.stop:
		return nil, false, true
	case <-c.force:
		return nil, false, false
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
	}
	c.drainPaced = false
	c.dropPacedLocked()
	c.mu.Unlock()
	c.signalStop()
	c.signalForce()
	_ = c.ws.CloseNow()
}

func (c *Conn) forceClose() {
	c.mu.Lock()
	c.drainPaced = false
	c.code = 0
	c.dropPacedLocked()
	c.mu.Unlock()
	c.signalForce()
	_ = c.ws.CloseNow()
}

func (c *Conn) signalStop() {
	c.stopOnce.Do(func() { close(c.stop) })
}

func (c *Conn) signalForce() {
	c.forceOnce.Do(func() { close(c.force) })
}

func (c *Conn) stopState() (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopping, c.drainPaced
}

func (c *Conn) pacedPending() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.paced) != 0
}

func (c *Conn) takePaced() ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.paced) == 0 || (c.stopping && !c.drainPaced) {
		return nil, false
	}
	frame := c.paced[0]
	c.paced[0] = nil
	c.paced = c.paced[1:]
	c.pacedBytes -= int64(len(frame))
	c.pacedInFlight++
	c.pacedInFlightBytes += int64(len(frame))
	return frame, true
}

func (c *Conn) releasePaced(frame []byte) {
	c.mu.Lock()
	c.pacedInFlight--
	c.pacedInFlightBytes -= int64(len(frame))
	c.mu.Unlock()
}

func (c *Conn) dropPaced() {
	c.mu.Lock()
	c.dropPacedLocked()
	c.mu.Unlock()
}

func (c *Conn) dropPacedLocked() {
	for i := range c.paced {
		c.paced[i] = nil
	}
	c.paced = nil
	c.pacedBytes = 0
}

func (c *Conn) queuesEmpty() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.out) == 0 && len(c.paced) == 0 && c.pacedInFlight == 0
}

func (c *Conn) forced() bool {
	select {
	case <-c.force:
		return true
	default:
		return false
	}
}

func closeReason(reason string) string {
	if len(reason) > 120 {
		return reason[:120]
	}
	return reason
}

func stopAndDrainTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
