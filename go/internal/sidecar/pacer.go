package sidecar

import "time"

// pacer is the delivery rate limit of contract-a.md §7.5 and §15 A20.
//
// THE PROBLEM IS NOT A RATE, IT IS A DAM. T1 left a mass of organisms on one
// world's west border: a slot slept for two hours, its inbound deliveries queued
// in its own journal while its west neighbour's export pile queued in that
// neighbour's, and both released TOGETHER at wake. A steady rate spreads a crowd
// across hours; a dam delivers it in one breath.
//
// THE CLOCK IS SIMULATED, NOT WALL-CLOCK. It advances from the mod's own
// HEARTBEAT.simulatedTime, so a world at 20x drains a dam 20x faster in
// wall-clock terms and at the same rate AS THE WORLD EXPERIENCES IT, which is
// the only frame of reference the crowd lives in.
//
// A stopped clock releases nothing and banks nothing. While paused is true,
// timeScale is 0, or no HEARTBEAT has arrived for pacingIdleGraceMs,
// simulatedTime does not advance and no token accrues — so a paused world does
// not bank a burst and wake into the very flood pacing exists to prevent.
type pacer struct {
	// ratePerSimMinute is inboundRatePerSimMinute: MIGRATE_IN frames released
	// per simulated minute of the RECEIVING world.
	ratePerSimMinute float64
	// burst is inboundRateBurst, the bucket capacity, so ordinary traffic is
	// never delayed. The bucket starts full for the same reason.
	burst  float64
	tokens float64

	haveSim   bool
	lastSim   float64
	lastBeat  time.Time
	running   bool
	simulated float64
}

func newPacer(ratePerSimMinute, burst float64) pacer {
	if burst <= 0 {
		burst = 1
	}
	return pacer{ratePerSimMinute: ratePerSimMinute, burst: burst, tokens: burst}
}

// beat advances the bucket from one HEARTBEAT. running is false while the world
// is paused or its timeScale is 0.
func (p *pacer) beat(simulatedTime float64, running bool, at time.Time) {
	p.lastBeat = at
	p.running = running
	p.simulated = simulatedTime
	if !p.haveSim {
		p.haveSim = true
		p.lastSim = simulatedTime
		return
	}
	delta := simulatedTime - p.lastSim
	p.lastSim = simulatedTime
	if !running || delta <= 0 {
		// No tokens accrue in the dark. A world that jumped backwards — a reload
		// from an older save — accrues nothing either, which is the safe
		// direction.
		return
	}
	p.tokens += (delta / 60.0) * p.ratePerSimMinute
	if p.tokens > p.burst {
		p.tokens = p.burst
	}
}

// allow reports whether one MIGRATE_IN may be released right now. It does not
// consume the token: the caller journals the attempt first and then calls take,
// so a failed journal write never spends one.
func (p *pacer) allow(now time.Time, idleGrace time.Duration) bool {
	if !p.haveSim || !p.running {
		return false
	}
	if now.Sub(p.lastBeat) > idleGrace {
		// The world has gone quiet. §7.5: the pacing clock stops and nothing is
		// released.
		return false
	}
	return p.tokens >= 1
}

func (p *pacer) take() {
	p.tokens--
	if p.tokens < 0 {
		p.tokens = 0
	}
}

// reset drops the bucket back to a cold start. It is used when the mod session
// changes, because a new session is a new world clock.
func (p *pacer) reset() {
	p.tokens = p.burst
	p.haveSim = false
	p.lastSim = 0
	p.running = false
	p.lastBeat = time.Time{}
}
