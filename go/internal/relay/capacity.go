package relay

// B24's enforcement, at the frame level and nowhere else
// (contract-b-m4.md §3.3, §22 B24).
//
// EVERY COUNTER IN THIS FILE COUNTS SOMETHING THE RELAY ALREADY HAS IN ITS
// HAND: a socket, a frame's length, a frame's type, a registry entry. Nothing
// here decodes data.body.bb8, data.lineage or data.species, nothing indexes
// anything, and nothing keeps per-organism state. That is not a coincidence of
// implementation — it is the whole of D1, which is why the archive is a
// separate service and why M6 can replace this relay with libp2p. A limit that
// needed a payload read is a limit this relay may not have, and an abuse limit
// is not worth spending D1 on.
//
// THE RELAY SHEDS THE CONNECTION, NEVER THE MAP. A peer over a limit is closed
// with 4007 or refused a claim; no other peer's traffic changes, no lane
// closes, no migration is dropped in flight, SLOT_VACANT still means what §6.8
// says, and the shed peer is live:false with darkSinceMs set like any other
// dark peer — which its neighbours route around exactly as they always did.

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"multiverse/internal/contractb"
)

// rateMeter is a fixed-window counter. It is the cheapest shape that answers
// the only question B24 asks — "did this connection put more than N through in
// a second?" — and it keeps no history, so a peer cannot make the relay spend
// memory by sending fast.
//
// A fixed window admits at most 2N over a window boundary, and that is
// deliberate rather than overlooked: §3.3 sizes these ceilings for a migration
// burst, so a meter that fired on a burst spanning a tick boundary would shed
// exactly the traffic the ceiling was sized to allow.
type rateMeter struct {
	window time.Duration
	start  time.Time
	count  int64
}

// observe adds n to the current window and returns the window's running total
// and its length. The caller compares the total against its own limit, because
// the same meter counts frames for one limit and bytes for another.
func (m *rateMeter) observe(now time.Time, n int64) (total int64, over time.Duration) {
	if m.start.IsZero() || now.Sub(m.start) >= m.window {
		m.start = now
		m.count = 0
	}
	m.count += n
	return m.count, m.window
}

// connMeter is one connection's frame and byte rate. It is per CONNECTION
// rather than per peerId because the answer to a breach is to shed THIS
// CONNECTION (§3.3): the relay counts what the socket it is about to close
// actually did, and maxConnectionsPerPeer is what bounds how many sockets one
// identity may have at once.
type connMeter struct {
	frames rateMeter
	bytes  rateMeter
}

func newConnMeter() *connMeter {
	return &connMeter{
		frames: rateMeter{window: time.Second},
		bytes:  rateMeter{window: time.Second},
	}
}

// observe counts one frame and returns the §3.3 breach text, or "" when the
// frame is inside both ceilings. The reason names WHICH limit, its configured
// value and the rate measured against it, because "capacity" alone is a support
// conversation rather than a remedy.
func (c *connMeter) observe(now time.Time, size int, l contractb.Limits) string {
	if n, w := c.frames.observe(now, 1); n > l.MaxFramesPerSecond {
		return fmt.Sprintf("maxFramesPerSecond %d exceeded (peak %d/s over %s)",
			l.MaxFramesPerSecond, n, w)
	}
	if n, w := c.bytes.observe(now, int64(size)); n > l.MaxBytesPerSecond {
		return fmt.Sprintf("maxBytesPerSecond %d exceeded (peak %d B/s over %s)",
			l.MaxBytesPerSecond, n, w)
	}
	return ""
}

// slotCounter bounds simultaneous connections on some key — a source address or
// a peerId. It counts SOCKETS, which is the only thing either limit is about.
type slotCounter struct {
	mu sync.Mutex
	n  map[string]int
}

func newSlotCounter() *slotCounter { return &slotCounter{n: map[string]int{}} }

// acquire takes one slot for key and reports the count AFTER it and whether it
// is within max. A refused acquire still holds its slot, so the caller MUST
// release in every path; that is what keeps the count from drifting upward on a
// storm of refusals.
func (c *slotCounter) acquire(key string, max int64) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n[key]++
	now := c.n[key]
	return now, int64(now) <= max
}

func (c *slotCounter) release(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n[key] <= 1 {
		delete(c.n, key)
		return
	}
	c.n[key]--
}

func (c *slotCounter) count(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n[key]
}

// sourceKey is the address half of maxConnectionsPerAddress. The PORT is
// deliberately dropped: a limit keyed on host:port would count every connection
// as its own address and bound nothing at all.
func sourceKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// claimMeter is maxClaimsPerMinute, and it is the one limit kept PER PEER
// rather than per connection: a claim changes the registry, the registry is
// keyed on peerId, and a peer that reconnected to get a fresh allowance would
// have found the loophole rather than the ceiling.
type claimMeter struct {
	rateMeter
}

func newClaimMeter() *claimMeter {
	return &claimMeter{rateMeter{window: time.Minute}}
}

// observe counts one SECTOR_CLAIM and returns the breach text, or "".
func (m *claimMeter) observe(now time.Time, l contractb.Limits) string {
	if n, w := m.rateMeter.observe(now, 1); n > l.MaxClaimsPerMinute {
		return fmt.Sprintf("maxClaimsPerMinute %d exceeded (%d claims in %s)",
			l.MaxClaimsPerMinute, n, w)
	}
	return ""
}

// capacityRefusal is what §6.5's lastRefusal carries for a limit that fired:
// the axis name, then the limit and the measurement. B22's rule still holds
// around it — a CREDENTIAL failure never reaches a slot and never appears here.
func capacityRefusal(breach string) string {
	return contractb.RefusalCapacity + ": " + breach
}
