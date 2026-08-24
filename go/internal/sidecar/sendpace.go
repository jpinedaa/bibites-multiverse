package sidecar

// THE CLIENT HALF OF §3.3, which the relay has always had and this sidecar
// never did (contract-b-m4.md §3.3, §6.2, §22 B24).
//
// B24 publishes the capacity table so "a peer can therefore be BUILT to respect
// a ceiling instead of discovering it as a 4007", and §6.2's limits row states
// the obligation as a MUST: "A peer reads it at connect and MUST respect it; a
// peer that cannot is a peer that will be shed with 4007." Until this file the
// sidecar read the object, logged it, and then sent as fast as its journal could
// hand frames over. WP4 added only §3.2's DEFENSIVE pin — two 4007s in a row
// hold at the backoff ceiling — which is a backstop for a limit already
// breached, not a way of staying under one.
//
// WHAT IT COST, MEASURED. Slot 6 rejoined on 2026-08-11 after four and a half
// hours dark and drained its journal backlog at the relay: eighteen sheds with
// close 4007, EVERY MEASURED PEAK EXACTLY 51/s against maxFramesPerSecond 50,
// in a cycle of reclaim -> burst -> shed -> ~30 s backoff -> reclaim. It
// converged only because each connection window drained some frames before the
// shed fired. A stranger returning after days away cycles exactly the same way,
// and it is the first thing a public relay will meet.
//
// THE PEAK OF 51 IS NOT "BARELY OVER", AND THAT IS WHY THIS PACES WELL UNDER
// THE NUMBER. relay/capacity.go's rateMeter sheds on the FIRST frame that makes
// the count exceed the ceiling (`n > l.MaxFramesPerSecond`), so a peak of
// limit+1 is what an UNBOUNDED burst always reads as: the measurement says
// nothing about how fast the peer was really going. Four properties of that
// meter set the headroom:
//
//  1. THE WINDOW IS THE RELAY'S AND IS INVISIBLE HERE. It is a fixed window
//     whose start is set lazily by whichever frame arrives after a one-second
//     gap, so a client cannot align to it and cannot see where it begins.
//  2. IT HAS NO TOLERANCE. There is no averaging and no second chance; the
//     first offending frame closes the connection. A ceiling like that is
//     approached with margin, never touched.
//  3. IT COUNTS ARRIVALS, NOT SENDS. Between this bucket and that counter sit
//     the wsutil writer goroutine's 128-deep queue, TLS record batching and the
//     relay's own read loop, all of which COMPRESS the spacing this end chose.
//     Frames sent evenly do not arrive evenly.
//  4. IT COUNTS EVERY FRAME TYPE, including the PONGs the relay itself asks
//     for and the stats PING of §6.11. A client's journal-backed traffic must
//     therefore leave room for traffic the client does not schedule.
//
// So the sustained rate is HALF the published ceiling and the burst is a
// QUARTER of it. A token bucket admits at most capacity+rate in any one second,
// which is 0.75 of the ceiling before any of (3)'s bunching is counted, and
// still under it after a quarter-second of arrival compression. The reserve
// below then keeps the last part of that budget for frames nobody queued.
//
// THE NUMBER IS THE PUBLISHED ONE, NEVER A CONSTANT. Everything here is a
// FRACTION of what HANDSHAKE_ACK.limits and PEER_STATUS.limits carry; an
// operator who raises the knob and restarts the relay raises this with it, at
// the next connect, with nothing to redeploy at this end. A relay that publishes
// no limits object at all — one that predates B24 — leaves this switched off,
// because absence reads as UNKNOWN and a client that invented a ceiling for an
// unknown relay would be throttling against a number nobody stated.
//
// FRAMES ARE DELAYED, NEVER DROPPED. A deferred MIGRATION_PAYLOAD leaves its
// outbound journal entry exactly as it found it. A deferred journal-backed
// MIGRATION_ACK leaves AckedUpstream false. The custody scheduler re-offers
// either frame on the next tick, so a backlog drains slower instead of shorter.
// Custody is untouched: this is a rate on the wire, not a decision about an
// organism.

import (
	"time"

	"multiverse/internal/contractb"
)

const (
	// sendPaceRateFraction is the sustained share of the published ceiling this
	// sidecar will use. Half, for reason (3) above: what this end spaces out,
	// the wire bunches back together.
	sendPaceRateFraction = 0.5
	// sendPaceBurstFraction is the bucket's capacity as a share of the same
	// ceiling. rate+capacity is the most a token bucket can put into any one
	// second, so a half and a quarter is three quarters of the ceiling at the
	// absolute worst — and the relay's window has no tolerance at all (2).
	sendPaceBurstFraction = 0.25
	// sendPaceReserveFraction is the share of the BUCKET that deferred traffic
	// may not draw below. It leaves room for a PONG, a stats PING, and the
	// immediate ACK/NACK response to a relay-paced migration arrival. It is
	// deliberately half: control traffic is a handful of frames a second and
	// the reserve only has to be there when one arrives.
	sendPaceReserveFraction = 0.5
)

// bucket is one token bucket with a floor under it. The floor is what makes
// control traffic independent of a drain: deferred traffic spends down to the
// reserve and stops, control spends the reserve and may go into debt, and the
// refill pays the debt back before deferred traffic moves again.
type bucket struct {
	rate     float64
	capacity float64
	reserve  float64
	tokens   float64
}

// newBucket derives one bucket from one published ceiling. EVERY NUMBER IN IT
// IS A FRACTION OF THAT CEILING — there is deliberately no absolute rate here to
// go stale when an operator turns the knob.
func newBucket(published int64) bucket {
	rate := float64(int64(float64(published) * sendPaceRateFraction))
	capacity := float64(int64(float64(published) * sendPaceBurstFraction))
	if rate < 1 {
		rate = 1
	}
	if capacity < 1 {
		capacity = 1
	}
	return bucket{
		rate:     rate,
		capacity: capacity,
		reserve:  capacity * sendPaceReserveFraction,
		// A fresh bucket starts FULL. A reconnect after a long dark period is
		// exactly the case this file exists for, and starting empty would only
		// add a second of nothing to a peer that is already behind.
		tokens: capacity,
	}
}

func (b *bucket) refill(elapsed time.Duration) {
	if elapsed <= 0 {
		return
	}
	b.tokens += b.rate * elapsed.Seconds()
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
}

// allowDeferred answers for one frame that has a durable retry path. A cost
// larger than the whole deferred allowance — an 8 MiB payload against a byte
// budget, which is legal on this wire — is admitted on a FULL bucket rather
// than never. Refusing it forever would turn a rate limit into a dropped
// organism, and the debt that it takes leaves behind keeps the next frame from
// following it out.
func (b *bucket) allowDeferred(cost float64) bool {
	usable := b.capacity - b.reserve
	if usable <= 0 {
		usable = b.capacity
	}
	if cost > usable {
		cost = usable
	}
	return b.tokens-b.reserve >= cost
}

// take spends a cost. It may drive the bucket NEGATIVE, which is deliberate:
// control frames are never refused, so the only honest way to account for one
// is a debt the refill has to clear before deferred traffic moves again.
func (b *bucket) take(cost float64) { b.tokens -= cost }

// sendPace is one connection's outbound rate, derived from that connection's
// published limits object. It is owned by Sidecar.mu, like every other field the
// send path touches.
type sendPace struct {
	on bool
	// acked is whether HANDSHAKE_ACK has arrived on this session, and it gates
	// DEFERRED TRAFFIC ON ITS OWN — before it, no ceiling is known and none can be.
	// The custody scheduler runs on its own tick and does not wait for a
	// handshake, so without this gate a rejoin whose first tick lands in the
	// millisecond before the ack would put the WHOLE BACKLOG on the wire
	// unpaced, which is the exact failure this file exists to stop.
	//
	// It also closes a second hole in the same window: forwardLocked records
	// s.relaySessionID against the entry at the durable send commit (§5.2, §9.2), and
	// that id arrives ON the HANDSHAKE_ACK. A frame sent before it stamps the
	// entry with an EMPTY session, and an empty session is one no later proof of
	// non-delivery can ever match.
	acked  bool
	frames bucket
	bytes  bucket
	last   time.Time

	// publishedFrames and publishedBytes are what the relay said, kept verbatim
	// so a log line can name the ceiling beside the rate chosen under it, and so
	// a republished table that did not change is not treated as a change.
	publishedFrames int64
	publishedBytes  int64
	// deferred counts retryable frames this pacer has held back on the current
	// session. It is the operator-visible measure of the fix working: a rising
	// count with no 4007 is a backlog draining under the ceiling.
	deferred int64
}

// reset returns the pacer to its unconfigured state. It runs at the START of
// every session, because the limits object is the relay's configuration and a
// new connection may be to a relay that was restarted with a different one.
func (p *sendPace) reset() { *p = sendPace{} }

// configure adopts the published table. It reports whether anything changed, so
// the caller logs a new ceiling and stays quiet about the same one republished
// on every PEER_STATUS.
//
// AN ABSENT OR ZERO ENTRY IS NEVER AN INSTRUCTION. §6.2 makes limits REQUIRED
// and its omitempty survives for one reader only — a peer talking to a relay
// that predates B24 — so absence reads as UNKNOWN: it leaves an unconfigured
// pacer off, and it leaves a configured one exactly as the handshake set it
// rather than silently unthrottling mid-session.
func (p *sendPace) configure(limits map[string]int64, now time.Time) bool {
	changed := false
	// The ack itself is the gate on deferred traffic, and it opens whether or not the
	// table came with it: a relay that predates B24 still completes a handshake,
	// and holding its peer's backlog forever would be a worse answer than M4's.
	p.acked = true
	if v := limits[contractb.LimitMaxFramesPerSecond]; v > 0 && v != p.publishedFrames {
		p.publishedFrames = v
		p.frames = newBucket(v)
		changed = true
	}
	if v := limits[contractb.LimitMaxBytesPerSecond]; v > 0 && v != p.publishedBytes {
		p.publishedBytes = v
		p.bytes = newBucket(v)
		changed = true
	}
	if changed {
		p.on = true
		p.last = now
	}
	return changed
}

// admit is the whole of the send path's obligation: refill, then decide.
//
// A deferred frame is one that has a durable retry path. MIGRATION_PAYLOAD
// remains pending when it is refused. A journal-backed MIGRATION_ACK keeps
// AckedUpstream false. The custody scheduler offers either frame again on the
// next tick.
//
// A control frame is ALWAYS admitted and ALWAYS charged. Refusing one would trade
// a capacity shed for a liveness timeout (close 4004 inside peerTimeoutMs), which
// is the same outage with a different close code; charging one is what makes the
// pacer's arithmetic true — every frame this connection puts on the wire is
// counted by the relay, so every frame has to be counted here.
func (p *sendPace) admit(now time.Time, size int, deferred bool) bool {
	if deferred && !p.acked {
		p.deferred++
		return false
	}
	if !p.on {
		return true
	}
	p.advance(now)
	if deferred {
		if !p.frames.allowDeferred(1) ||
			(p.publishedBytes > 0 && !p.bytes.allowDeferred(float64(size))) {
			p.deferred++
			return false
		}
	}
	p.frames.take(1)
	if p.publishedBytes > 0 {
		p.bytes.take(float64(size))
	}
	return true
}

func (p *sendPace) advance(now time.Time) {
	if !p.last.IsZero() {
		if elapsed := now.Sub(p.last); elapsed > 0 {
			p.frames.refill(elapsed)
			p.bytes.refill(elapsed)
		}
	}
	p.last = now
}

// readyForBulk is the same decision as admit's bulk half, asked BEFORE the
// caller builds a frame. A drain offers every entry in the journal on every
// tick, and encoding a multi-megabyte MIGRATION_PAYLOAD only to defer it would
// spend the whole backlog's serialisation cost on every tick it does not fit in.
// It is an optimisation and never an authority: admit still decides, because
// admit is the only place that knows the frame's real size.
func (p *sendPace) readyForBulk(now time.Time) bool {
	if !p.acked {
		p.deferred++
		return false
	}
	if !p.on {
		return true
	}
	p.advance(now)
	if !p.frames.allowDeferred(1) {
		p.deferred++
		return false
	}
	return true
}

// paceDeferred names the outbound types whose durable state can offer them
// again. MIGRATION_PAYLOAD remains pending until pace admission. It then commits
// sent before the one socket enqueue. A journal-backed
// MIGRATION_ACK remains unacknowledged upstream until it is sent. Immediate
// duplicate ACKs and NACKs use the bounded reply path instead, because the
// relay paces the migration arrivals that produce them.
func paceDeferred(typ string) bool {
	return typ == contractb.TypeMigrationPayload || typ == contractb.TypeMigrationAck
}

// framesPerSecond is the rate this pacer is running at, or 0 when it is off.
func (p *sendPace) framesPerSecond() float64 {
	if !p.on {
		return 0
	}
	return p.frames.rate
}
