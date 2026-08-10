package sidecar

import (
	"testing"
	"time"

	"multiverse/internal/contracta"
)

// The §20 A45 tests: heartbeatTimeoutMs is 13 s, not 3.5 s, because a periodic
// world save blocks the Unity main thread that composes the heartbeat, so a long
// save IS heartbeat silence and 3.5 s made every long save a 4004.
//
// Two things are worth pinning, and they are different in kind. The first is
// ARITHMETIC — the number has to clear the measured save tail and it has to sit
// in the right place among the other four liveness deadlines — and arithmetic is
// what a future retune will get wrong silently. The second is BEHAVIOUR: a
// session that goes save-length silent against a sidecar running the SHIPPED
// default must survive, and its arrivals must be held rather than force-fed.

// worstMeasuredSaveMs is the longest periodic save the living deployment has
// logged: 9 427 ms on slot 4, across the 1 801 saves of the thirteen-hour x100
// generation preserved as e2e/logs-m4-lan/bepinex/*.minfps.* (dev_environment.md,
// *Watch items*). It is a MEASUREMENT and it is allowed to move — when it does,
// this test is the one that says whether the timeout still clears it.
const worstMeasuredSaveMs = 9427

// TestHeartbeatDeadlineOrdering is the arithmetic half of §20 A45. Five
// deadlines govern a quiet mod and the order they fire in is the design; each
// inequality below is a rule, not a coincidence.
func TestHeartbeatDeadlineOrdering(t *testing.T) {
	const (
		grace   = int(defaultHeartbeatDeliveryGrace / time.Millisecond)
		idle    = contracta.PacingIdleGraceMs
		timeout = contracta.HeartbeatTimeoutMs
		ping    = contracta.WSPingIntervalMs
		pong    = contracta.WSPongTimeoutMs
		beat    = contracta.HeartbeatIntervalMs
	)

	if timeout != 13000 {
		t.Fatalf("heartbeatTimeoutMs = %d, want 13000 (contract-a.md §10, §20 A45)", timeout)
	}

	// 1. The quiet-mod gate trips FIRST, always. It is what makes a stalling mod
	//    paused into rather than flooded into (§15 A29 defence 3), and a gate that
	//    trips after the close cannot do that.
	if !(grace < idle && grace < timeout) {
		t.Fatalf("heartbeatDeliveryGraceMs=%d must be the first deadline (pacingIdleGraceMs=%d, heartbeatTimeoutMs=%d)",
			grace, idle, timeout)
	}

	// 2. The close is LAST among the application-layer deadlines. A45 inverted
	//    this pair — before it, the pacer's idle grace sat beyond the close and
	//    could never trip on a stall — and the inversion is safe only because both
	//    branches release nothing. Assert the new order so a future retune that
	//    puts the close back under the idle grace has to say why.
	if !(idle < timeout) {
		t.Fatalf("pacingIdleGraceMs=%d must trip before heartbeatTimeoutMs=%d (§20 A45)", idle, timeout)
	}

	// 3. The application-layer detector fires before the TRANSPORT one, so the
	//    4004 an operator reads is the informative one. Both paths close with
	//    4004 (contracta_server.go modPinger), and only the heartbeat path logs a
	//    silentFor — which is what the dose response was matched on.
	if !(timeout < ping) {
		t.Fatalf("heartbeatTimeoutMs=%d must stay under wsPingIntervalMs=%d, or a stall can trip the transport path first",
			timeout, ping)
	}
	if worst := ping + pong; timeout >= worst {
		t.Fatalf("heartbeatTimeoutMs=%d must stay under the transport's worst case %d ms (ping %d + pong %d)",
			timeout, worst, ping, pong)
	}

	// 4. It clears the worst save the deployment has measured, with margin. 13 000
	//    against 9 427 is 3 573 ms — more than the whole retired timeout as slack,
	//    and 38% of the worst case, against the 27% a 12 000 would have bought on
	//    a tail that has grown with every regime change.
	if timeout <= worstMeasuredSaveMs {
		t.Fatalf("heartbeatTimeoutMs=%d does not clear the worst measured save (%d ms)", timeout, worstMeasuredSaveMs)
	}
	if margin := timeout - worstMeasuredSaveMs; margin < worstMeasuredSaveMs/3 {
		t.Fatalf("heartbeatTimeoutMs=%d clears the worst measured save (%d ms) by only %d ms; §20 A45 sized the raise for a third of it in margin",
			timeout, worstMeasuredSaveMs, margin)
	}

	// 5. §8's own framing survives the raise: the timeout is still a whole number
	//    of missed heartbeats plus a beat of slack.
	if timeout%beat != 0 {
		t.Fatalf("heartbeatTimeoutMs=%d is not a whole number of heartbeatIntervalMs=%d", timeout, beat)
	}
	if missed := timeout/beat - 1; missed != 12 {
		t.Fatalf("heartbeatTimeoutMs=%d is %d missed heartbeats plus slack; §10 says twelve", timeout, missed)
	}
}

// TestSaveLengthSilenceKeepsTheSession is the behavioural half of §20 A45. A mod
// that goes app-silent for longer than the RETIRED 3.5 s timeout — which is what
// a periodic save on this rig costs — keeps its session against a sidecar running
// the SHIPPED default. Its arrivals are held by the quiet-mod gate for the whole
// silence and delivered when heartbeats return, so the whole episode costs a
// delivery pause instead of a session churn.
//
// This is deliberately NOT on the suite's short clock: the guarantee under test
// is about the shipped number, and a scaled-down copy of it would pass against
// 3 500 too.
func TestSaveLengthSilenceKeepsTheSession(t *testing.T) {
	const silence = 4500 * time.Millisecond // > the retired 3 500 ms, << the shipped 13 000

	def := DefaultConfig()
	g := newGrid(t, 2, gridOptions{
		layout:    layoutRow(2),
		heartbeat: 100 * time.Millisecond,
		tune: func(i int, c *Config) {
			if i == 1 {
				// The three liveness deadlines back to the contract's own values.
				// fastConfig shortens all three, and the transport pair has to come
				// with the heartbeat one or the pinger closes — also with 4004 —
				// before the point of the test is reached.
				c.HeartbeatTimeout = def.HeartbeatTimeout
				c.WSPingInterval = def.WSPingInterval
				c.WSPongTimeout = def.WSPongTimeout
			}
		},
	})
	a, b := g.node(0), g.node(1)

	if b.cfg.HeartbeatTimeout != 13*time.Second {
		t.Fatalf("the node under test runs heartbeatTimeout=%v, want the shipped 13s", b.cfg.HeartbeatTimeout)
	}

	warm := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "the warm-up delivery to land", func() bool {
		return b.world.spawnCount(warm) == 1
	})

	// The save starts: the main thread is gone, so no HEARTBEAT is composed. The
	// socket stays up because the transport half lives on another thread — which
	// is why the mod's pongs keep answering through a stall, and why raising the
	// application-layer deadline is the change that matters.
	stalled := time.Now()
	b.mod.suppressHeartbeats(true)

	// Wait out heartbeatDeliveryGraceMs before exporting, so the arrivals meet a
	// gate that is already shut. Inside the grace the newest heartbeat is still
	// fresh and delivery is correct — that is the window ordinary traffic lives in.
	time.Sleep(b.cfg.HeartbeatDeliveryGrace + 300*time.Millisecond)

	ids := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		ids = append(ids, a.mod.migrateOut(int32(-97000-i), contracta.EdgeE, 0.5))
	}
	waitFor(t, 10*time.Second, "b to take custody of the arrivals during the stall", func() bool {
		for _, id := range ids {
			if custodyOf(b.side, id) == "absent" {
				return false
			}
		}
		return true
	})

	if rest := silence - time.Since(stalled); rest > 0 {
		time.Sleep(rest)
	}
	if got := time.Since(stalled); got < silence {
		t.Fatalf("the stall lasted %v, want at least %v", got, silence)
	}

	// The whole point: no 4004. Before A45 this window ended in one.
	if b.mod.isClosed() {
		t.Fatalf("the session was closed inside a %v save-length silence; §20 A45 raised heartbeatTimeoutMs so it would not be",
			silence)
	}
	if got := deliveredCount(b.mod, ids); got != 0 {
		t.Fatalf("%d MIGRATE_IN were released into a stalled mod; the quiet-mod gate must hold them for the whole silence", got)
	}

	// The save finishes. Delivery resumes with the same entries, nothing dropped,
	// nothing replayed — there was no reconnect to replay across.
	b.mod.suppressHeartbeats(false)
	waitFor(t, 15*time.Second, "delivery to resume when the save finishes", func() bool {
		for _, id := range ids {
			if b.world.spawnCount(id) != 1 {
				return false
			}
		}
		return true
	})
	if b.mod.isClosed() {
		t.Fatal("the session did not survive the stall it was raised to survive")
	}
}
