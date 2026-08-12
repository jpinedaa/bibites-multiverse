package sidecar

// WHAT THIS SIDECAR REMEMBERS ABOUT ITSELF, so that something else does not have
// to go and find it out.
//
// WP7's contract for `--diagnose` has one rule that decides this file's whole
// shape: "Where a check needs live map state, it reads what the running sidecar
// already holds rather than dialling again" (docs/sidecar-diagnose-spec.md §1).
// A diagnostic that opened its own Contract B session to answer "is my
// credential accepted" would be shed by maxConnectionsPerPeer, or would take
// over the running session under the newer-connection-replaces-older rule
// (§3.2, B-4006) — it would break the thing it was called to measure.
//
// So the sidecar records the facts a diagnosis needs AT THE MOMENT IT LEARNS
// THEM, and the diagnostic reads the record:
//
//   - the classified reason its last relay session ended — 401, a certificate,
//     a capacity shed, a close code — which is the only place the difference
//     between B-401 and B-4003b exists on this machine;
//   - the last placement answer, granted or refused, with the relay's own
//     reason string, which is what `slot` reports;
//   - what this connection has actually put on the wire, against the ceilings
//     the relay published, which is what makes an approaching-a-limit warning a
//     measurement rather than a guess;
//   - the ACHIEVED time scale, Δ simulated time over Δ wall, which is the half
//     of "is this world running at the speed it was told to" that no wire field
//     carries.
//
// Every one of them is a fact this process already had and threw away.

import (
	"strings"
	"time"

	"multiverse/internal/contractb"
)

// The classified kinds of relay-session failure. They are the axes of the
// taxonomy's §3.1 and §3.2 as this sidecar can tell them apart, and they exist
// so a diagnostic reports ONE root cause with a trail of unknowns behind it
// rather than five failures.
const (
	// FaultUnauthorized is HTTP 401 on the upgrade: B-401. No WebSocket was
	// created and nothing reached any slot's lastRefusal.
	FaultUnauthorized = "unauthorized"
	// FaultTLS is a certificate that did not verify: B-TLS. It is decided before
	// the HTTP request, so it says nothing at all about the credential.
	FaultTLS = "tls"
	// FaultCapacity is close 4007: B-4007. The reason string names the limit,
	// its value and the measurement that crossed it.
	FaultCapacity = "capacity"
	// FaultClosed is any other close or transport error, carrying the code and
	// the reason verbatim. B-4003b (an identity mismatch), B-4003d (a wire
	// version below the floor) and B-4003c (a game version) all arrive here and
	// are told apart by the reason string, which is where the relay explains
	// itself.
	FaultClosed = "closed"
)

// relayFault is the last reason a relay session ended, kept so a diagnostic can
// name it without reproducing it.
type relayFault struct {
	kind        string
	reason      string
	at          time.Time
	consecutive int
}

// recordRelayFault stores one classified session failure.
//
// IT SCRUBS THE SECRET, unconditionally. Nothing on this path is supposed to
// carry one — a dial error names a host, a close carries the relay's own words —
// but this text is written to a file, served over loopback and pasted into
// support conversations, and "no code path currently puts it there" is a weaker
// guarantee than "the value is removed if it is". The taxonomy's rule is that
// no log in this system prints a credential at any level, and this is the same
// obligation one surface further out.
func (s *Sidecar) recordRelayFault(kind, reason string, consecutive int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.relayFault = &relayFault{
		kind:        kind,
		reason:      s.scrubSecret(reason),
		at:          s.now(),
		consecutive: consecutive,
	}
}

// scrubSecret removes this peer's credential from any text about to leave the
// process. Caller holds the lock, or is New.
func (s *Sidecar) scrubSecret(text string) string {
	if s.cfg.Secret == "" || !strings.Contains(text, s.cfg.Secret) {
		return text
	}
	return strings.ReplaceAll(text, s.cfg.Secret, "[REDACTED SECRET]")
}

// adoptPublishedLocked keeps the relay's own published configuration — its
// capacity table and its wire-version floor — as it arrived. It is kept for a
// READER and never for a decision this file makes: sendPace decides the pace,
// and this is what lets a participant see the ceiling their reading is measured
// against.
//
// ABSENCE NEVER CLEARS A KNOWN VALUE. §6.2's omitempty survives for a relay that
// predates B24, so a table missing from one frame reads as UNKNOWN rather than
// as "the ceilings were removed" — and a value already learned on this session
// stays learned.
func (s *Sidecar) adoptPublishedLocked(limits map[string]int64, minVersion string) {
	if len(limits) > 0 {
		adopted := make(map[string]int64, len(limits))
		for k, v := range limits {
			adopted[k] = v
		}
		s.publishedLimits = adopted
	}
	if minVersion != "" {
		s.minContractVersion = minVersion
	}
}

// grantRecord is the last answer the relay gave this peer's placement claim
// (contract-b-m4.md §7.2). The reason is the relay's word — granted, reclaimed,
// updated, or a refusal reason the taxonomy's BCLAIM-* entries name.
type grantRecord struct {
	granted bool
	reason  string
	slot    int
	at      time.Time
}

func (s *Sidecar) recordGrantLocked(granted bool, reason string, slot int) {
	s.lastGrant = &grantRecord{granted: granted, reason: reason, slot: slot, at: s.now()}
}

// ---------------------------------------------------------------- send meter

// sentMeter measures what this connection ACTUALLY PUT ON THE WIRE, in the same
// terms the relay's own meter counts it: frames in a one-second window and bytes
// in a one-second window, with the peak of each kept for the session
// (internal/relay/capacity.go's rateMeter is the far end of this).
//
// It is not the pacer. The pacer decides what MAY go; this records what DID,
// including every frame the pacer never sees — a PONG, a stats PING, an ACK.
// The distinction is the whole reason the reading is worth having: (4) in
// sendpace.go says the relay counts every frame type, so a peer's own bulk
// budget is not its own traffic.
type sentMeter struct {
	window      time.Duration
	start       time.Time
	frames      int64
	bytes       int64
	peakFrames  int64
	peakBytes   int64
	largest     int64
	totalFrames int64
	totalBytes  int64
}

func newSentMeter() sentMeter { return sentMeter{window: time.Second} }

// observe counts one frame of size bytes at now.
func (m *sentMeter) observe(now time.Time, size int) {
	if m.window <= 0 {
		m.window = time.Second
	}
	if m.start.IsZero() || now.Sub(m.start) >= m.window {
		m.roll()
		m.start = now
	}
	m.frames++
	m.bytes += int64(size)
	m.totalFrames++
	m.totalBytes += int64(size)
	if int64(size) > m.largest {
		m.largest = int64(size)
	}
	// The peak has to include the window in progress: a burst that is happening
	// right now is exactly the reading a capacity warning is about, and waiting
	// for the window to close would make the warning arrive after the shed.
	if m.frames > m.peakFrames {
		m.peakFrames = m.frames
	}
	if m.bytes > m.peakBytes {
		m.peakBytes = m.bytes
	}
}

func (m *sentMeter) roll() { m.frames, m.bytes = 0, 0 }

// reset clears the session's measurements. It runs where sendPace.reset runs,
// because a peak belongs to one connection: the relay's meter is per connection
// too, and a peak carried across a reconnect would be measured against a
// ceiling nobody was holding at the time.
func (m *sentMeter) reset() { *m = newSentMeter() }

// claimMeter counts this peer's own SECTOR_CLAIMs in a rolling minute, which is
// exactly how the relay counts them for maxClaimsPerMinute — and per peer
// rather than per connection, because that is how the relay keys it.
type claimMeter struct {
	at []time.Time
}

func (c *claimMeter) observe(now time.Time) {
	c.trim(now)
	c.at = append(c.at, now)
}

func (c *claimMeter) count(now time.Time) int {
	c.trim(now)
	return len(c.at)
}

func (c *claimMeter) trim(now time.Time) {
	cut := now.Add(-time.Minute)
	i := 0
	for i < len(c.at) && c.at[i].Before(cut) {
		i++
	}
	if i > 0 {
		c.at = append(c.at[:0], c.at[i:]...)
	}
}

// ---------------------------------------------------------------- achieved

// THE ACHIEVED TIME SCALE, measured where the heartbeats are.
//
// Three different numbers get called the speed of a world and only the third is
// a measurement (internal/archive/achieved.go): the TARGET the operator asked
// for, the APPLIED scale the game's governor allowed — which the mod copies from
// Time.timeScale and this sidecar copies again, never computing it — and the
// ACHIEVED rate, how much simulated time the world actually produced per wall
// second. The two come apart precisely when the host cannot keep up, and the gap
// is the news (dev_environment.md, *A world can be at the wrong time scale*).
//
// The archive measures it for the operator, from PEER_STATUS. A participant has
// no archive, so the sidecar measures its own from the frames it already
// receives: `simulatedTime` is on every HEARTBEAT (contract-a.md §5.2). It adds
// no wire field, and the guards are the archive's, for the same reasons — a
// world that reloaded, a mod that went quiet and a span too short to smooth the
// jitter are all UNKNOWN rather than a number nothing can stand behind.
const (
	achievedWindow   = time.Minute
	achievedMinSpan  = 10 * time.Second
	achievedMaxGap   = 20 * time.Second
	achievedMaxPairs = 512
)

type simSample struct {
	at  time.Time
	sim float64
}

type achievedRate struct {
	session string
	samples []simSample
}

// observe records one (wall, simulated) pair, or throws the history away when
// the new pair cannot honestly be compared with what came before.
func (r *achievedRate) observe(session string, at time.Time, sim float64) {
	if session != r.session {
		// A different Contract A session is a world that was reloaded, or
		// another world entirely. Nothing before this belongs to it.
		r.session, r.samples = session, nil
	}
	if n := len(r.samples); n > 0 {
		last := r.samples[n-1]
		switch {
		case !at.After(last.at):
			// A clock that did not move is not a second measurement, and a zero
			// Δ wall is not a rate.
			return
		case sim < last.sim:
			// simulatedTime runs forward within one session; a fall means the
			// world behind it is not the world that produced the earlier pair.
			r.samples = nil
		case at.Sub(last.at) > achievedMaxGap:
			// The two pairs do not describe one continuous run.
			r.samples = nil
		}
	}
	r.samples = append(r.samples, simSample{at: at, sim: sim})
	if len(r.samples) > achievedMaxPairs {
		r.samples = append(r.samples[:0], r.samples[len(r.samples)-achievedMaxPairs:]...)
	}
}

// rate is Δ simulated / Δ wall over the window, with the span it was measured
// over. It reports unknown rather than a number it cannot stand behind.
func (r *achievedRate) rate(now time.Time) (float64, time.Duration, bool) {
	r.trim(now)
	if len(r.samples) < 2 {
		return 0, 0, false
	}
	first, last := r.samples[0], r.samples[len(r.samples)-1]
	span := last.at.Sub(first.at)
	if span < achievedMinSpan {
		return 0, 0, false
	}
	advanced := last.sim - first.sim
	if advanced < 0 {
		return 0, 0, false
	}
	return advanced / span.Seconds(), span, true
}

func (r *achievedRate) trim(now time.Time) {
	cut := now.Add(-achievedWindow)
	i := 0
	for i < len(r.samples) && r.samples[i].at.Before(cut) {
		i++
	}
	if i > 0 {
		r.samples = append(r.samples[:0], r.samples[i:]...)
	}
}

// ---------------------------------------------------------------- the band

// LimitWarnFraction is the share of a published limit at which a reading counts
// as APPROACHING it, and it closes the specification's own slot
// (docs/sidecar-diagnose-spec.md §7, "the band that makes a reading approaching
// a limit a WARN rather than a PASS").
//
// IT IS DERIVED, NOT CHOSEN. This sidecar's own paced sender admits at most
// sendPaceRateFraction + sendPaceBurstFraction of a published ceiling in any one
// second — half sustained plus a quarter of burst — so 0.75 of the ceiling is
// the most a peer doing exactly what B24 asks of it can produce. A band below
// that would warn about correct behaviour; a band above it would be a number
// this project invented. A reading over it is therefore one of two things worth
// a line: traffic the pacer did not schedule, or a ceiling this world no longer
// fits under.
//
// It is a FRACTION and never an absolute, for the reason the pacer is: every
// limit is a knob, the relay publishes what it is running with, and an operator
// who raises one raises this with it at the next connect with nothing to
// redeploy at this end.
const LimitWarnFraction = sendPaceRateFraction + sendPaceBurstFraction

// approachingLimit reports whether a measured reading is inside the warn band of
// a published ceiling, and the fraction it is at. A non-positive ceiling is a
// limit the relay did not publish, which reads as UNKNOWN and never as "no
// ceiling" (contract-b-m4.md §6.2).
func approachingLimit(measured, published int64) (fraction float64, known, approaching bool) {
	if published <= 0 {
		return 0, false, false
	}
	f := float64(measured) / float64(published)
	return f, true, f > LimitWarnFraction
}

// publishedLimit reads one entry out of the table the relay published. Absence
// is UNKNOWN.
func publishedLimit(limits map[string]int64, key string) (int64, bool) {
	v, ok := limits[key]
	if !ok || v <= 0 {
		return 0, false
	}
	return v, true
}

// limitKeys is the published table in §3.3's own order, for a renderer that must
// not depend on map iteration order.
var limitKeys = contractb.PublishedLimitKeys
