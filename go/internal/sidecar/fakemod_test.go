package sidecar

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contracta"
	"multiverse/internal/wire"
)

// fakeWorld is the part of a fake mod that survives a reconnect and a sidecar
// restart: the organisms that are alive in the sim, the migrations that are
// still unresolved, and the counters the tests assert on.
//
// It models exactly the two mod-side rules the custody chain depends on
// (contract-a.md §6.3, §7.3): an organism with an unresolved MIGRATE_OUT stays
// inert and keeps its migrationId until a sidecar answers, and a spawn is
// suppressed when an organism with that entityId is already in the world.
type fakeWorld struct {
	mu        sync.Mutex
	sessionID string

	alive       map[int32]bool
	spawns      map[string]int // migrationId -> organisms actually spawned
	destroys    map[string]int // migrationId -> local copies destroyed
	bounces     map[string]int // migrationId -> deliveries flagged bounceBack
	inflightOut map[string]contracta.MigrateOut
	sent        map[string]contracta.MigrateOut // every MIGRATE_OUT ever built
	nackedOut   map[string]string
	unsolicited int
}

func newWorld() *fakeWorld {
	return &fakeWorld{
		sessionID:   wire.NewUUID(),
		alive:       map[int32]bool{},
		spawns:      map[string]int{},
		destroys:    map[string]int{},
		bounces:     map[string]int{},
		inflightOut: map[string]contracta.MigrateOut{},
		sent:        map[string]contracta.MigrateOut{},
		nackedOut:   map[string]string{},
	}
}

func (w *fakeWorld) spawnCount(migrationID string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.spawns[migrationID]
}

func (w *fakeWorld) destroyCount(migrationID string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.destroys[migrationID]
}

func (w *fakeWorld) isAlive(entityID int32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.alive[entityID]
}

func (w *fakeWorld) put(entityID int32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.alive[entityID] = true
}

// ackMode drives how the fake mod answers MIGRATE_IN.
type ackMode int

const (
	ackNormal ackMode = iota
	ackSilent         // never answer: the reconnect-replay case
	ackNackTransient
	ackNackPermanent
)

type fakeModOptions struct {
	url         string
	world       *fakeWorld
	gameVersion string
	modVersion  string
	simSize     float64
	borderEdges []string
	borderWidth float64
	sector      *contracta.Sector
	heartbeat   time.Duration // 0 disables heartbeats entirely
	ackMode     ackMode
}

// fakeMod is an in-process WebSocket client that speaks Contract A exactly as
// contracts/contract-a.md specifies it.
type fakeMod struct {
	t    *testing.T
	opts fakeModOptions
	ws   *websocket.Conn

	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	frames    []wire.Envelope
	lastEpoch int64
	edges     map[string]contracta.EdgeState
	closeCode websocket.StatusCode
	closed    bool
	simTick   int64
	ackMode   ackMode
	world     *fakeWorld
	wg        sync.WaitGroup
}

func dialFakeMod(t *testing.T, opts fakeModOptions) *fakeMod {
	t.Helper()
	if opts.gameVersion == "" {
		opts.gameVersion = "0.6.3.1"
	}
	if opts.modVersion == "" {
		opts.modVersion = "0.2.0"
	}
	if opts.simSize == 0 {
		opts.simSize = 2000
	}
	if opts.borderWidth == 0 {
		opts.borderWidth = 60
	}
	if opts.world == nil {
		opts.world = newWorld()
	}

	ctx, cancel := context.WithCancel(context.Background())
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	ws, _, err := websocket.Dial(dialCtx, opts.url, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	dialCancel()
	if err != nil {
		cancel()
		t.Fatalf("fake mod: dial %s: %v", opts.url, err)
	}
	ws.SetReadLimit(wire.MaxFrameBytes)

	m := &fakeMod{
		t: t, opts: opts, ws: ws, ctx: ctx, cancel: cancel,
		edges: map[string]contracta.EdgeState{}, ackMode: opts.ackMode, world: opts.world,
		closeCode: -1,
	}
	m.wg.Add(1)
	go m.readLoop()

	m.sendConfigUpdate("connect")
	if opts.heartbeat > 0 {
		m.wg.Add(1)
		go m.heartbeatLoop(opts.heartbeat)
	}
	// contract-a.md §6.3: an organism whose MIGRATE_OUT never resolved keeps
	// its migrationId and the identical message is re-sent on the next
	// connection. Never mint a second id.
	for _, out := range m.world.pendingOut() {
		m.sendFrame(contracta.TypeMigrateOut, out)
	}
	t.Cleanup(m.close)
	return m
}

func (w *fakeWorld) pendingOut() []contracta.MigrateOut {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]contracta.MigrateOut, 0, len(w.inflightOut))
	for _, m := range w.inflightOut {
		out = append(out, m)
	}
	return out
}

func (m *fakeMod) close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.mu.Unlock()
	m.cancel()
	_ = m.ws.Close(websocket.StatusNormalClosure, "world unload")
	m.wg.Wait()
}

// abort drops the socket without a close handshake, the way a killed game does.
func (m *fakeMod) abort() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.cancel()
	_ = m.ws.CloseNow()
	m.wg.Wait()
}

func (m *fakeMod) readLoop() {
	defer m.wg.Done()
	for {
		_, data, err := m.ws.Read(m.ctx)
		if err != nil {
			m.mu.Lock()
			m.closeCode = websocket.CloseStatus(err)
			m.closed = true
			m.mu.Unlock()
			return
		}
		env, decodeErr := wire.Decode(data)
		if decodeErr != nil {
			m.t.Errorf("fake mod: sidecar sent a malformed frame: %v (%s)", decodeErr, string(data))
			continue
		}
		m.mu.Lock()
		m.frames = append(m.frames, env)
		m.mu.Unlock()
		m.handle(env)
	}
}

func (m *fakeMod) handle(env wire.Envelope) {
	switch env.Type {
	case contracta.TypeEdgeStatus:
		var st contracta.EdgeStatus
		if json.Unmarshal(env.Data, &st) != nil {
			return
		}
		m.mu.Lock()
		if st.Epoch <= m.lastEpoch {
			// contract-a.md §5.4: ignore a stale or repeated epoch.
			m.mu.Unlock()
			return
		}
		m.lastEpoch = st.Epoch
		m.edges = map[string]contracta.EdgeState{}
		for _, e := range st.Edges {
			m.edges[e.Edge] = e
		}
		m.mu.Unlock()
	case contracta.TypeMigrateOutAck:
		var ack contracta.MigrateOutAck
		if json.Unmarshal(env.Data, &ack) != nil {
			return
		}
		m.onMigrateOutAck(ack)
	case contracta.TypeMigrateOutNack:
		var nack contracta.MigrateOutNack
		if json.Unmarshal(env.Data, &nack) != nil {
			return
		}
		m.world.mu.Lock()
		delete(m.world.inflightOut, nack.MigrationID)
		m.world.nackedOut[nack.MigrationID] = nack.Code
		// The organism is revived: it is still in the world.
		m.world.alive[nack.EntityID] = true
		m.world.mu.Unlock()
	case contracta.TypeMigrateIn:
		var in contracta.MigrateIn
		if json.Unmarshal(env.Data, &in) != nil {
			return
		}
		m.onMigrateIn(in)
	}
}

// onMigrateOutAck implements contract-a.md §5.5's receiver obligations,
// including the unsolicited custody-reassertion case of §7.4.
func (m *fakeMod) onMigrateOutAck(ack contracta.MigrateOutAck) {
	m.world.mu.Lock()
	defer m.world.mu.Unlock()
	if ack.Unsolicited {
		m.world.unsolicited++
	}
	if _, ok := m.world.inflightOut[ack.MigrationID]; ok {
		delete(m.world.inflightOut, ack.MigrationID)
		if m.world.alive[ack.EntityID] {
			delete(m.world.alive, ack.EntityID)
			m.world.destroys[ack.MigrationID]++
		}
		return
	}
	// No in-flight record: scan the world by entityId. A hit is a rollback
	// artefact and the sidecar holds custody of it.
	if m.world.alive[ack.EntityID] {
		delete(m.world.alive, ack.EntityID)
		m.world.destroys[ack.MigrationID]++
	}
}

// onMigrateIn implements contract-a.md §5.7's receiver obligations, reduced to
// what a fake mod can do: the durable entityId dedup, the spawn, and the reply.
func (m *fakeMod) onMigrateIn(in contracta.MigrateIn) {
	m.mu.Lock()
	mode := m.ackMode
	tick := m.simTick
	m.mu.Unlock()

	switch mode {
	case ackSilent:
		return
	case ackNackTransient:
		retry := 200
		m.sendFrame(contracta.TypeMigrateInNack, contracta.MigrateInNack{
			MigrationID: in.MigrationID, EntityID: &in.EntityID,
			Code: contracta.InSimNotReady, Class: contracta.ClassTransient,
			Message: "fake mod is not ready", RetryAfterMs: &retry})
		return
	case ackNackPermanent:
		m.sendFrame(contracta.TypeMigrateInNack, contracta.MigrateInNack{
			MigrationID: in.MigrationID, EntityID: &in.EntityID,
			Code: contracta.InDeserializeFailed, Class: contracta.ClassPermanent,
			Message: "LoadBibiteOrEggFromData returned null"})
		return
	}

	m.world.mu.Lock()
	duplicate := m.world.alive[in.EntityID]
	if !duplicate {
		m.world.alive[in.EntityID] = true
		m.world.spawns[in.MigrationID]++
	}
	if in.BounceBack {
		m.world.bounces[in.MigrationID]++
	}
	m.world.mu.Unlock()

	parents, children := 2, 3
	m.sendFrame(contracta.TypeMigrateInAck, contracta.MigrateInAck{
		MigrationID: in.MigrationID, EntityID: &in.EntityID, Duplicate: &duplicate,
		SimTick: &tick, RelinkedParents: &parents, RelinkedChildren: &children})
}

func (m *fakeMod) sendConfigUpdate(reason string) {
	m.t.Helper()
	simSize := m.opts.simSize
	borderWidth := m.opts.borderWidth
	m.sendFrame(contracta.TypeConfigUpdate, contracta.ConfigUpdate{
		SessionID:      m.world.sessionID,
		Reason:         reason,
		GameVersion:    m.opts.gameVersion,
		ModVersion:     m.opts.modVersion,
		SimulationSize: &simSize,
		BorderEdges:    m.opts.borderEdges,
		BorderWidth:    &borderWidth,
		Sector:         m.opts.sector,
		WorldName:      "M2-Sector",
	})
}

func (m *fakeMod) heartbeatLoop(interval time.Duration) {
	defer m.wg.Done()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-t.C:
			m.mu.Lock()
			m.simTick++
			tick := m.simTick
			m.mu.Unlock()
			simTime := float64(tick)
			pop := 100
			paused := false
			scale := 1.0
			simSize := m.opts.simSize
			m.sendFrame(contracta.TypeHeartbeat, contracta.Heartbeat{
				SessionID: m.world.sessionID, SimTick: &tick, SimulatedTime: &simTime,
				Population: &pop, Paused: &paused, TimeScale: &scale, SimulationSize: &simSize})
		}
	}
}

func (m *fakeMod) sendFrame(typ string, data any) {
	frame, err := wire.Encode(wire.ProtocolA, typ, time.Now().UnixMilli(), data)
	if err != nil {
		m.t.Errorf("fake mod: encode %s: %v", typ, err)
		return
	}
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	defer cancel()
	_ = m.ws.Write(ctx, websocket.MessageText, frame)
}

// sendRaw writes bytes that are not necessarily a valid envelope, so a test can
// exercise the malformed-frame rules of contract-a.md §3.2.
func (m *fakeMod) sendRaw(b []byte) {
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	defer cancel()
	_ = m.ws.Write(ctx, websocket.MessageText, b)
}

// migrateOut mints one migration, marks the organism inert, and sends
// MIGRATE_OUT. It returns the migrationId.
func (m *fakeMod) migrateOut(entityID int32, edge string, position float64) string {
	m.t.Helper()
	return m.migrateOutPayload(wire.NewUUID(), entityID, edge, position, makePayload(entityID))
}

func (m *fakeMod) migrateOutPayload(migrationID string, entityID int32, edge string, position float64, payload string) string {
	m.t.Helper()
	m.world.put(entityID)
	m.mu.Lock()
	tick := m.simTick
	m.mu.Unlock()
	simSize := m.opts.simSize
	heading := 274.11
	out := contracta.MigrateOut{
		MigrationID:    migrationID,
		EntityID:       &entityID,
		Kind:           contracta.KindBibite,
		GameVersion:    m.opts.gameVersion,
		Payload:        payload,
		ExitEdge:       edge,
		ExitPosition:   &position,
		Velocity:       &contracta.Vec{X: 6.12, Y: 0.44},
		Heading:        &heading,
		SimulationSize: &simSize,
		SimTick:        &tick,
	}
	m.world.mu.Lock()
	m.world.inflightOut[migrationID] = out
	m.world.sent[migrationID] = out
	m.world.mu.Unlock()
	m.sendFrame(contracta.TypeMigrateOut, out)
	return migrationID
}

// resend repeats an identical MIGRATE_OUT, which contract-a.md §7.2 requires to
// be idempotent.
func (m *fakeMod) resend(migrationID string) {
	m.t.Helper()
	m.world.mu.Lock()
	out, ok := m.world.sent[migrationID]
	m.world.mu.Unlock()
	if !ok {
		m.t.Fatalf("fake mod: no MIGRATE_OUT on record for %s", migrationID)
	}
	m.sendFrame(contracta.TypeMigrateOut, out)
}

func (m *fakeMod) setAckMode(mode ackMode) {
	m.mu.Lock()
	m.ackMode = mode
	m.mu.Unlock()
}

// ---------------------------------------------------------------- assertions

func (m *fakeMod) frameCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.frames)
}

// waitFrom waits for the first frame at or after index from that satisfies
// pred, and returns it with its index.
func (m *fakeMod) waitFrom(from int, timeout time.Duration, pred func(wire.Envelope) bool) (wire.Envelope, int) {
	m.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		m.mu.Lock()
		for i := from; i < len(m.frames); i++ {
			if pred(m.frames[i]) {
				env := m.frames[i]
				m.mu.Unlock()
				return env, i
			}
		}
		m.mu.Unlock()
		if time.Now().After(deadline) {
			m.t.Fatalf("fake mod: timed out after %s waiting for a frame; saw %s",
				timeout, m.seenTypes())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (m *fakeMod) waitType(typ string, timeout time.Duration) wire.Envelope {
	m.t.Helper()
	env, _ := m.waitFrom(0, timeout, func(e wire.Envelope) bool { return e.Type == typ })
	return env
}

func (m *fakeMod) seenTypes() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := ""
	for _, f := range m.frames {
		out += f.Type + " "
	}
	if out == "" {
		return "(nothing)"
	}
	return out
}

// waitEdge waits until the named edge reaches the wanted open state.
func (m *fakeMod) waitEdge(edge string, open bool, timeout time.Duration) contracta.EdgeState {
	m.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		m.mu.Lock()
		st, ok := m.edges[edge]
		m.mu.Unlock()
		if ok && st.Open == open {
			return st
		}
		if time.Now().After(deadline) {
			m.t.Fatalf("fake mod: edge %s did not reach open=%v within %s (state %+v, frames %s)",
				edge, open, timeout, st, m.seenTypes())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (m *fakeMod) waitClosed(timeout time.Duration) websocket.StatusCode {
	m.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		m.mu.Lock()
		closed, code := m.closed, m.closeCode
		m.mu.Unlock()
		if closed {
			return code
		}
		if time.Now().After(deadline) {
			m.t.Fatalf("fake mod: connection was not closed within %s", timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// rawMod is a bare Contract A socket with no handshake and no bookkeeping, for
// the frame-validity and close-code rules of contract-a.md §2.1 and §3.2.
type rawMod struct {
	t      *testing.T
	ws     *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	closed bool
	code   int
}

func dialRawMod(t *testing.T, url string) *rawMod {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	ws, _, err := websocket.Dial(dialCtx, url, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	dialCancel()
	if err != nil {
		cancel()
		t.Fatalf("raw mod: dial %s: %v", url, err)
	}
	r := &rawMod{t: t, ws: ws, ctx: ctx, cancel: cancel, code: -1}
	go func() {
		for {
			if _, _, err := ws.Read(ctx); err != nil {
				r.mu.Lock()
				r.closed = true
				r.code = int(websocket.CloseStatus(err))
				r.mu.Unlock()
				return
			}
		}
	}()
	t.Cleanup(func() { cancel(); _ = ws.CloseNow() })
	return r
}

func (r *rawMod) write(frame []byte) {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()
	_ = r.ws.Write(ctx, websocket.MessageText, frame)
}

func (r *rawMod) waitClosed(timeout time.Duration) int {
	r.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		r.mu.Lock()
		closed, code := r.closed, r.code
		r.mu.Unlock()
		if closed {
			return code
		}
		if time.Now().After(deadline) {
			r.t.Fatalf("raw mod: the connection was not closed within %s", timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// makePayload builds a canned bb8 blob in the shape contract-a.md §5.3's
// example shows. It is opaque to everything but bb8.Inspect (D4).
func makePayload(entityID int32) string {
	return fmt.Sprintf(`{"transform":{"position":[2000.0,412.77],"rotation":274.11,"scale":0.9312},`+
		`"rb2d":{"px":2000.0,"py":412.77,"vx":6.12,"vy":0.44,"r":274.11},`+
		`"genes":{"names":["a","b"],"values":[0.1,0.2]},`+
		`"body":{"id":{"id":%d},"health":42.0,"energy":11.5},`+
		`"clock":{"simulatedTime":120507.75},`+
		`"brain":{"Nodes":[],"Synapses":[]},"version":"0.6.3.1"}`, entityID)
}
