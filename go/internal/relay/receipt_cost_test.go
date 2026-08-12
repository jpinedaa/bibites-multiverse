package relay

// WP3's own done-when clause, in one file: "the forward receipt ships WITH ITS
// MEASURED PER-MIGRATION COST AT RATE" (m5_tracking.md, WP3).
//
// WHY A HARNESS AND NOT AN ESTIMATE. B26 priced itself at "one extra frame per
// migration on a relay whose whole design virtue is that it forwards frames and
// does nothing else", and then said the quiet part: at the rig's 300-500
// crossings a minute it is a rounding error, at a public map's rates it is a
// real cost, and WP3 MEASURES IT AT RATE RATHER THAN ASSUMING IT AWAY (§5.2,
// §22 B26; m5_considerations.md, DQ2). An assumed cost is how a hosted service
// discovers its egress bill in month two.
//
// WHY THERE IS NO "RECEIPTS OFF" ARM. The contract does not admit one: §5.2 says
// the relay MUST send one receipt per forward, so a switch that turned them off
// would be a configuration this wire does not have, and a measurement of a relay
// nobody may run. The marginal cost is therefore measured by ACCOUNTING — the
// receipt's own bytes, its own frame, and its own share of the forward path's
// CPU — which is what B26's cost paragraph is actually asking for.
//
// WHAT IT MEASURES, in the three units a hoster pays in:
//
//	BYTES   the receipt frame's size, exact, against the migration it acknowledges
//	FRAMES  relay-written frames per migration, before and after
//	CPU     the receipt path's share of the whole forward path, single-goroutine
//
// The assertions are structural — one receipt per forward, and the frame stays
// small — because a test that failed on a timing number would fail on a busy
// host and teach nobody anything. THE NUMBERS ARE IN THE LOG, and the log is
// what wp3_hosting_options.md's egress table quotes.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
	"multiverse/internal/wire"
)

// The rig's own measured numbers, from wp3_hosting_options.md's measurement base
// (2026-08-11 22:10Z). They are here so the harness can convert a per-migration
// cost into the per-day term the egress table is written in, and so that a
// reader can see exactly which recorded figures the conversion rests on.
const (
	// rigWireBytesPerCrossing is the mean MIGRATION_PAYLOAD frame on the living
	// deployment: ~15.8 KB, the bb8 body plus its envelope.
	rigWireBytesPerCrossing = 15800
	// rigCrossingsPerDayPerUnitS is 1,065.2 crossings/min over S = 67.9, per day.
	rigCrossingsPerDayPerUnitS = 22600
	// exitTestS is the exit-test bar's summed map speed: five peers at x1 plus the
	// owner (m5_considerations.md, Decision 8).
	exitTestS = 5
)

// receiptMigrations is the harness's scale, on the churn harness's model:
// -short is the CI mode, the default is a run a developer will sit through, and
// MULTIVERSE_RECEIPT_MIGRATIONS is the dial for a long measurement.
func receiptMigrations(t *testing.T) int {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv("MULTIVERSE_RECEIPT_MIGRATIONS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			t.Fatalf("MULTIVERSE_RECEIPT_MIGRATIONS=%q is not a positive integer", v)
		}
		return n
	}
	if testing.Short() {
		return 200
	}
	return 2000
}

// ---------------------------------------------------------------- a counting client

// costPeer is a Contract B client that COUNTS AND DISCARDS. credPeer keeps every
// frame it ever saw, which is right for a test that asks "did the relay ever say
// X" and wrong for one that pushes thousands of 15.8 KB payloads through the
// same process: the harness would be measuring its own retention.
type costPeer struct {
	t      *testing.T
	id     string
	ws     *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
	// ackPayloads makes this client a DESTINATION: it answers every forwarded
	// MIGRATION_PAYLOAD with a MIGRATION_ACK, which is what the real shape of a
	// crossing costs the relay.
	ackPayloads bool

	mu     sync.Mutex
	frames map[string]int
	bytes  map[string]int64
	closed bool
}

func dialCostPeer(t *testing.T, r *credRelay, id string, ack bool) *costPeer {
	t.Helper()
	r.mint(id, peercred.GrantPeer)
	ctx, cancel := context.WithCancel(context.Background())
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	ws, _, err := websocket.Dial(dialCtx, r.url, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
		HTTPHeader:      http.Header{"Authorization": []string{peercred.Header(id, r.secret(id))}},
	})
	dialCancel()
	if err != nil {
		cancel()
		t.Fatalf("cost peer %s: dial: %v", id, err)
	}
	ws.SetReadLimit(wire.MaxFrameBytes)
	p := &costPeer{t: t, id: id, ws: ws, ctx: ctx, cancel: cancel, ackPayloads: ack,
		frames: map[string]int{}, bytes: map[string]int64{}}
	t.Cleanup(func() { cancel(); _ = ws.CloseNow() })
	go p.readLoop()
	p.send(contractb.TypeHandshake, contractb.Handshake{
		PeerID: id, Role: contractb.RolePeer, ProtocolVersion: wire.ProtocolB,
		GameVersion: "0.6.3.1", SidecarVersion: "cost-harness", SimulationSize: 2000,
	})
	return p
}

func (p *costPeer) readLoop() {
	for {
		_, frame, err := p.ws.Read(p.ctx)
		if err != nil {
			p.mu.Lock()
			p.closed = true
			p.mu.Unlock()
			return
		}
		env, decodeErr := wire.Decode(frame)
		if decodeErr != nil {
			continue
		}
		p.mu.Lock()
		p.frames[env.Type]++
		p.bytes[env.Type] += int64(len(frame))
		p.mu.Unlock()
		switch env.Type {
		case contractb.TypePing:
			var ping contractb.Ping
			if json.Unmarshal(env.Data, &ping) == nil {
				p.send(contractb.TypePong, contractb.Pong{Nonce: ping.Nonce})
			}
		case contractb.TypeMigrationPayload:
			if !p.ackPayloads {
				continue
			}
			var head struct {
				MigrationID string `json:"migrationId"`
				SourcePeer  string `json:"sourcePeer"`
				EntityID    int32  `json:"entityId"`
			}
			if json.Unmarshal(env.Data, &head) != nil {
				continue
			}
			p.send(contractb.TypeMigrationAck, contractb.MigrationAck{
				MigrationID: head.MigrationID, SourcePeer: p.id, DestPeer: head.SourcePeer,
				EntityID: head.EntityID, DeliveredAt: time.Now().UnixMilli(),
			})
		}
	}
}

func (p *costPeer) send(typ string, data any) {
	frame, err := wire.Encode(wire.ProtocolB, typ, time.Now().UnixMilli(), data)
	if err != nil {
		return
	}
	p.sendFrame(frame)
}

func (p *costPeer) sendFrame(frame []byte) {
	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
	defer cancel()
	_ = p.ws.Write(ctx, websocket.MessageText, frame)
}

func (p *costPeer) count(typ string) (frames int, bytes int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.frames[typ], p.bytes[typ]
}

func (p *costPeer) waitCount(typ string, want int, timeout time.Duration) int {
	p.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		n, _ := p.count(typ)
		if n >= want {
			return n
		}
		if time.Now().After(deadline) {
			return n
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (p *costPeer) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// inFlightWindow is how many frames of one type the harness lets outrun the
// reader. It is comfortably under wsutil's 128-frame outbound queue, so no arm
// of the measurement can reach the overflow behaviour — which has its own tests
// and is not a per-migration cost.
const inFlightWindow = 48

// awaitDrain blocks until at most window frames of this type are unread. It is
// called OUTSIDE every timed region, so the wait never lands in a measurement.
func awaitDrain(t *testing.T, p *costPeer, typ string, baseline, sent, window int) {
	t.Helper()
	if sent-window <= 0 {
		return
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		got, _ := p.count(typ)
		if (got-baseline) >= sent-window || p.isClosed() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s stopped draining %s: %d sent, %d read", p.id, typ, sent, got-baseline)
		}
		time.Sleep(200 * time.Microsecond)
	}
}

func (p *costPeer) claim(slot int) {
	p.send(contractb.TypeSectorClaim, contractb.SectorClaim{
		PreferredSlot:     slot,
		PreferredPosition: &contractb.Position{Col: slot - 1, Row: 0},
		SimulationSize:    2000,
		ExportEdges:       []string{"E", "N"},
		BorderEdges:       []string{"E", "N", "W", "S"},
		GameVersion:       "0.6.3.1",
		ModConnected:      true,
	})
}

// ---------------------------------------------------------------- the measurement

func TestB26TheReceiptsMeasuredPerMigrationCostAtRate(t *testing.T) {
	migrations := receiptMigrations(t)

	// THE CEILINGS ARE RAISED, AND THAT IS THE HARNESS SAYING WHAT IT MEASURES.
	// §3.3's shipped table is sized for a six-slot map's migration burst, and a
	// sender pushing thousands of crossings flat out would be shed with 4007
	// before it measured anything — correctly, by a limit whose own tests are in
	// capacity_test.go. This harness measures the RELAY'S COST PER MIGRATION,
	// which is what a hoster pays and what does not depend on the ceiling.
	r := startCredRelay(t, credRelayOptions{
		limits: contractb.Limits{
			MaxFramesPerSecond: 1 << 20,
			MaxBytesPerSecond:  1 << 34,
			MaxClaimsPerMinute: 1000,
		},
		// Out of reach: a PEER_STATUS heartbeat inside the measurement window
		// would be counted against a migration it has nothing to do with.
		statsBroadcast: time.Hour,
		inMemoryGrid:   true,
	})
	// THE SLOTS ARE RESERVED BEFORE ANYBODY DIALS, and that is not tidiness. Two
	// peers claiming at once race for slot 1 under §7.2 rule 6, and a harness that
	// let the race decide would sometimes measure a relay forwarding a frame back
	// to its own sender. Pre-seeding is the same operator act the rig uses to make
	// placement independent of start order (§7.2's ReserveSlot).
	for _, seed := range []struct {
		id  string
		at  contractb.Position
		was int
	}{
		{"peer-sender", contractb.Position{Col: 0, Row: 0}, 1},
		{"peer-dest", contractb.Position{Col: 1, Row: 0}, 2},
	} {
		at := seed.at
		res, _, err := r.srv.ReserveSlot(seed.id, &at)
		if err != nil {
			t.Fatalf("reserve %s: %v", seed.id, err)
		}
		if res.Slot != seed.was {
			t.Fatalf("%s was reserved slot %d, want %d", seed.id, res.Slot, seed.was)
		}
	}
	sender := dialCostPeer(t, r, "peer-sender", false)
	dest := dialCostPeer(t, r, "peer-dest", true)
	sender.claim(1)
	dest.claim(2)
	if n := sender.waitCount(contractb.TypeSectorGrant, 1, 5*time.Second); n < 1 {
		t.Fatal("the sender never got its grant")
	}
	if n := dest.waitCount(contractb.TypeSectorGrant, 1, 5*time.Second); n < 1 {
		t.Fatal("the destination never got its grant")
	}
	for _, res := range r.srv.Snapshot() {
		if res.Slot == 2 && res.PeerID != "peer-dest" {
			t.Fatalf("slot 2 is held by %s; the harness would be measuring a forward back to "+
				"its own sender", res.PeerID)
		}
	}

	// ------------------------------------------------------------ 1. bytes
	//
	// The exact frame, encoded the way the relay encodes it, with real uuids and
	// a real millisecond clock. There is no variance to average out: every field
	// is fixed-width except destSlot, so the size is the size.
	oneReceipt := receiptFrameBytes(t, 6)
	receiptBytes := len(oneReceipt)

	// The payload the harness pushes is sized from the living deployment's own
	// measurement — ~15.8 KB per crossing, the bb8 body plus its envelope — so
	// the ratio below is the ratio a hoster will actually see.
	payloads := buildPayloadFrames(t, migrations, rigWireBytesPerCrossing)
	payloadBytes := len(payloads[0])

	// ------------------------------------------------------------ 2. frames, at rate
	//
	// THE LOAD IS FLOW-CONTROLLED, and that is a measurement decision rather than
	// politeness. Every outbound queue on this wire is bounded at 128 frames
	// (wsutil), so a sender that blasts 2 000 payloads faster than the writer
	// goroutine drains them measures the queue's overflow behaviour instead of the
	// relay's per-migration cost. Capping the in-flight window keeps every frame
	// on the path the shipped rig uses.
	start := time.Now()
	for i, frame := range payloads {
		awaitDrain(t, dest, contractb.TypeMigrationPayload, 0, i, inFlightWindow)
		sender.sendFrame(frame)
	}
	// Everything the relay owes for these migrations: the forward, the ack back,
	// and the receipt. The wait is generous because the harness is measuring the
	// relay and not this host's scheduler.
	deadline := 60 * time.Second
	if testing.Short() {
		deadline = 20 * time.Second
	}
	gotForwards := dest.waitCount(contractb.TypeMigrationPayload, migrations, deadline)
	gotReceipts := sender.waitCount(contractb.TypeForwardReceipt, migrations, deadline)
	gotAcks := sender.waitCount(contractb.TypeMigrationAck, migrations, deadline)
	elapsed := time.Since(start)

	if gotForwards != migrations {
		t.Fatalf("the relay forwarded %d of %d migrations; the measurement has no base",
			gotForwards, migrations)
	}
	// THE CENTRAL ASSERTION, and it is B26's own rule rather than a performance
	// one: ONE FORWARD, ONE RECEIPT.
	if gotReceipts != migrations {
		t.Fatalf("%d forwards produced %d receipts; §6.12 is one per forward",
			migrations, gotReceipts)
	}
	sent, dropped := r.srv.ReceiptCounts()
	if sent != int64(migrations) || dropped != 0 {
		t.Fatalf("the relay counted %d receipts sent and %d dropped over %d forwards",
			sent, dropped, migrations)
	}

	_, receiptBytesTotal := sender.count(contractb.TypeForwardReceipt)
	_, ackBytesTotal := sender.count(contractb.TypeMigrationAck)
	_, forwardBytesTotal := dest.count(contractb.TypeMigrationPayload)
	meanReceipt := float64(receiptBytesTotal) / float64(gotReceipts)

	// ------------------------------------------------------------ 3. CPU accounting
	//
	// Two single-goroutine loops over the same relay, so wall time IS cpu time for
	// the code under measurement, and the ratio is what survives a slow host.
	r.srv.mu.Lock()
	senderPeer := r.srv.peers["peer-sender"]
	r.srv.mu.Unlock()
	if senderPeer == nil {
		t.Fatal("the relay has no peer object for the sender")
	}

	cpuFrames := buildPayloadFrames(t, migrations, rigWireBytesPerCrossing)
	envs := make([]wire.Envelope, len(cpuFrames))
	for i, frame := range cpuFrames {
		env, err := wire.Decode(frame)
		if err != nil {
			t.Fatalf("decode payload %d: %v", i, err)
		}
		envs[i] = env
	}
	// ONLY THE CALLS ARE TIMED, and the drain waits sit outside the clock. Both
	// arms are flow-controlled for the reason above: a dropped receipt and a
	// closed destination are both real behaviours with their own tests, and
	// neither is the cost of a forward.
	forwardBaseline, _ := dest.count(contractb.TypeMigrationPayload)

	// The whole forward path, as dispatch runs it: decode the envelope, read the
	// routing fields, record the forward, enqueue the frame, emit the receipt.
	forwardedBefore := r.srv.ForwardedCount()
	droppedBeforeForward := func() int64 { _, d := r.srv.ReceiptCounts(); return d }()
	var forwardPath time.Duration
	for i, frame := range cpuFrames {
		awaitDrain(t, dest, contractb.TypeMigrationPayload, forwardBaseline, i, inFlightWindow)
		at := time.Now()
		_, _ = wire.Decode(frame)
		r.srv.onMigrationPayload(senderPeer, envs[i], frame)
		forwardPath += time.Since(at)
	}
	if grew := r.srv.ForwardedCount() - forwardedBefore; grew != migrations {
		t.Fatalf("the timed forward loop recorded %d of %d forwards; the CPU arm measured "+
			"something other than a forward", grew, migrations)
	}
	if dest.isClosed() {
		t.Fatal("the destination's connection was closed during the timed forward loop")
	}
	if _, d := r.srv.ReceiptCounts(); d != droppedBeforeForward {
		t.Fatalf("%d receipts were dropped during the timed FORWARD loop; that arm would then be "+
			"measuring a drop rather than a forward", d-droppedBeforeForward)
	}
	// The receipt path alone: build the four-field frame and enqueue it.
	//
	// The baseline is taken HERE and not earlier: the forward arm above emits a
	// receipt of its own per iteration, so a baseline captured before it would make
	// every drain check trivially satisfied and this loop would measure the
	// overflow it is supposed to avoid.
	receiptBaseline, _ := sender.count(contractb.TypeForwardReceipt)
	droppedBefore := func() int64 { _, d := r.srv.ReceiptCounts(); return d }()
	var receiptPath time.Duration
	for i := 0; i < migrations; i++ {
		awaitDrain(t, sender, contractb.TypeForwardReceipt, receiptBaseline, i, inFlightWindow)
		at := time.Now()
		r.srv.sendForwardReceipt(senderPeer, wire.NewUUID(), 2)
		receiptPath += time.Since(at)
	}
	if _, nowDropped := r.srv.ReceiptCounts(); nowDropped != droppedBefore {
		t.Fatalf("%d receipts were dropped during the CPU arm; the number below would be the "+
			"cost of a drop rather than of a send", nowDropped-droppedBefore)
	}

	perForward := forwardPath / time.Duration(migrations)
	perReceipt := receiptPath / time.Duration(migrations)
	share := 100 * float64(receiptPath) / float64(forwardPath)

	// ------------------------------------------------------------ the report
	//
	// This block is the deliverable. wp3_hosting_options.md's egress table quotes
	// it, and m5_tracking.md's WP3 row is answered by it.
	// DECIMAL MB AND GB, because wp3_hosting_options.md's egress table is written
	// in them (357 MB/day per unit S, 54 GB/month of forwards) and a term quoted
	// into that table in binary units would not add up against its neighbours.
	perDayPerUnitS := float64(receiptBytes) * rigCrossingsPerDayPerUnitS
	perMonthAtBar := perDayPerUnitS * exitTestS * 30 / 1e9
	forwardMonthAtBar := float64(rigWireBytesPerCrossing) * rigCrossingsPerDayPerUnitS *
		exitTestS * 30 / 1e9

	t.Logf("B26 FORWARD_RECEIPT — measured per-migration cost at rate")
	t.Logf("  scale:      %d migrations forwarded in %s (%.0f crossings/s achieved; the rig's own "+
		"steady rate is 17.8/s)", migrations, elapsed.Truncate(time.Millisecond),
		float64(migrations)/elapsed.Seconds())
	t.Logf("  BYTES:      %d B per receipt on the wire (mean over the run %.1f B), against a "+
		"%d B migration frame = %.2f%% of the frame it acknowledges",
		receiptBytes, meanReceipt, payloadBytes, 100*float64(receiptBytes)/float64(payloadBytes))
	t.Logf("  FRAMES:     relay-written frames per migration 2 -> 3 (forward + ack -> forward + " +
		"ack + receipt); relay-read frames per migration unchanged at 2")
	t.Logf("  BYTES/MIG:  relay egress per migration %d -> %d B (+%.2f%%)",
		payloadBytes+int(float64(ackBytesTotal)/float64(max(gotAcks, 1))),
		payloadBytes+int(float64(ackBytesTotal)/float64(max(gotAcks, 1)))+receiptBytes,
		100*float64(receiptBytes)/float64(payloadBytes+int(float64(ackBytesTotal)/float64(max(gotAcks, 1)))))
	t.Logf("  CPU:        %s per forward path, %s per receipt path = %.1f%% marginal, "+
		"single-goroutine on this host", perForward, perReceipt, share)
	t.Logf("              (the forward path is dominated by JSON: one wire.Decode plus two "+
		"json.Unmarshal passes over the %d B frame. A 287 B encode is noise beside it, which "+
		"is WHY the marginal number is what it is)", payloadBytes)
	t.Logf("  DERIVED:    %.2f MB/day per unit S (%d B x %d crossings/day/unit S); "+
		"%.2f GB/month at the exit-test bar (S=%d), against %.1f GB/month of forwards on the "+
		"same map = %.1f%% of the migration egress term",
		perDayPerUnitS/1e6, receiptBytes, rigCrossingsPerDayPerUnitS,
		perMonthAtBar, exitTestS, forwardMonthAtBar, 100*perMonthAtBar/forwardMonthAtBar)
	t.Logf("  totals:     forwards %d B, acks %d B, receipts %d B over the run",
		forwardBytesTotal, ackBytesTotal, receiptBytesTotal)
	t.Logf("  NOT MEASURED HERE: the SENDER's durable write. One journal Apply per receipt — one " +
		"appended record and one fsync — is measured in the sidecar suite " +
		"(TestB26TheSendersDurableCostOfARecordedReceipt)")

	// The one number this test will fail on, because it is a contract property
	// rather than a performance one: the cheapest frame on this wire must stay
	// cheap. Four fields and no body cannot reach a kilobyte, and a receipt that
	// did would mean something had been added to it.
	if receiptBytes > 1024 {
		t.Fatalf("a FORWARD_RECEIPT frame is %d B; §6.12 is four fields, no body, no answer, and "+
			"the whole cost argument rests on it staying the cheapest frame on this wire", receiptBytes)
	}
	if float64(receiptBytes) > 0.05*float64(payloadBytes) {
		t.Fatalf("a receipt is %.1f%% of the migration frame it acknowledges; B26 priced it as a "+
			"rounding error against the payload", 100*float64(receiptBytes)/float64(payloadBytes))
	}
}

// buildPayloadFrames pre-encodes n MIGRATION_PAYLOAD frames of about `size`
// bytes each, OUTSIDE any timed region: the relay forwards the bytes it received
// and never encodes one, so an encode inside the measurement would be charging
// the relay for work it does not do.
func buildPayloadFrames(t *testing.T, n, size int) [][]byte {
	t.Helper()
	// The blob is padded to bring the whole frame to the rig's measured mean.
	probe, err := wire.Encode(wire.ProtocolB, contractb.TypeMigrationPayload, time.Now().UnixMilli(),
		contractb.MigrationPayload{
			MigrationID: wire.NewUUID(), Kind: "bibite",
			Body:       contractb.Body{Version: "0.6.3.1", BB8: `{"version":"0.6.3.1"}`},
			Lineage:    contractb.Lineage{Parents: []contractb.Parent{}},
			SourcePeer: "peer-sender", SourceSlot: 1, DestSlot: 2, ExitEdge: "E",
			Timestamp: time.Now().UnixMilli(),
		})
	if err != nil {
		t.Fatalf("probe encode: %v", err)
	}
	pad := size - len(probe)
	if pad < 0 {
		pad = 0
	}
	blob := `{"version":"0.6.3.1","genes":"` + strings.Repeat("g", pad) + `"}`
	out := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		frame, err := wire.Encode(wire.ProtocolB, contractb.TypeMigrationPayload,
			time.Now().UnixMilli(), contractb.MigrationPayload{
				MigrationID: wire.NewUUID(), Kind: "bibite",
				Body:       contractb.Body{Version: "0.6.3.1", BB8: blob},
				Lineage:    contractb.Lineage{Parents: []contractb.Parent{}},
				SourcePeer: "peer-sender", SourceSlot: 1, DestSlot: 2, ExitEdge: "E",
				Timestamp: time.Now().UnixMilli(),
			})
		if err != nil {
			t.Fatalf("encode payload %d: %v", i, err)
		}
		out = append(out, frame)
	}
	return out
}
