package sidecar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/bb8"
	"multiverse/internal/contracta"
	"multiverse/internal/modtoken"
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

// alivePopulation is what HEARTBEAT.population reports, which is what
// PEER_STATUS republishes and the status page renders.
func (w *fakeWorld) alivePopulation() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.alive)
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
	// borderEdges are the edges this mod accepts an inbound organism on — all
	// four under the grid. exportEdges are the edges it runs a capture band on:
	// different doors, and one EDGE_STATUS entry per export edge
	// (contract-a.md §15, A18).
	borderEdges []string
	exportEdges []string
	borderWidth float64
	ringSlot    *int
	heartbeat   time.Duration // 0 disables heartbeats entirely
	ackMode     ackMode
	// omitExportEdges sends a CONFIG_UPDATE with no exportEdges at all. A18
	// REMOVED the singular field and its fallback, so this is a close, not a
	// resolution.
	omitExportEdges bool
	// simStep is how much simulated time one HEARTBEAT advances. It is what the
	// pacing test turns down to make a simulated minute pass in a test's
	// lifetime (contract-a.md §7.5).
	simStep float64
	// token is A47's bearer token on the Contract A upgrade. Empty means
	// testContractAToken, which is what every rig in this suite configures its
	// sidecars with; a test that wants the REFUSAL sets its own wrong value, and
	// noToken sends no header at all.
	token   string
	noToken bool
	// lastSave, when set, rides every HEARTBEAT as the save receipt of §15 A21.
	lastSave *contracta.SaveReceipt
	// censusRaw, when set, is spliced into every HEARTBEAT's data object as raw
	// JSON — `"species":[…]` and, when the test wants it, `,"truncated":true`.
	// It is RAW rather than typed on purpose: §5.2's strip table is mostly about
	// shapes no Go type can express (a `species` that is a number, a count that
	// is a string, an entry that is an array), and a test that could only send
	// well-typed censuses could not reach a single row of it (§17, A35).
	censusRaw string
	// settingsRaw, when set, is spliced into every CONFIG_UPDATE's data object
	// as raw JSON — the five OPTIONAL fields of contract-a.md §19, A42. It is
	// RAW for the same reason censusRaw is: A42's strip rule is mostly about
	// shapes no Go type can express (a `migrationExclude` that is a number, a
	// `saveMinutes` that is a string, an entry that is an object), and a test
	// that could only send well-typed settings could not reach a single row of
	// it. An empty string sends NO settings field at all, which is what a mod
	// older than contract-a/2.3 does.
	settingsRaw string
}

// fakeMod is an in-process WebSocket client that speaks Contract A exactly as
// contracts/contract-a.md specifies it.
type fakeMod struct {
	t    *testing.T
	opts fakeModOptions
	ws   *websocket.Conn

	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	frames      []wire.Envelope
	lastEpoch   int64
	edges       map[string]contracta.EdgeState
	edgeHistory []contracta.EdgeStatus
	closeCode   websocket.StatusCode
	closed      bool
	simTick     int64
	simTime     float64
	simStep     float64
	paused      bool
	// hbSuppressed silences HEARTBEATs on the same socket without closing it, so
	// a test can take the mod app-silent and bring it back with no reconnect —
	// the state contract-a.md §8's timeout and the sidecar's delivery-freshness
	// gate both watch for.
	hbSuppressed bool
	// censusRaw is the JSON fragment spliced into every HEARTBEAT. Mutable, so
	// a test can watch a world start reporting a census, change it, and stop.
	censusRaw string
	// settingsRaw is the JSON fragment spliced into every CONFIG_UPDATE.
	// Mutable, so a test can re-handshake with `reason: "settings_changed"` —
	// which is the channel §19 A42 names for a setting that ever becomes
	// mutable, and which already exists.
	settingsRaw string
	ackMode     ackMode
	world       *fakeWorld
	hbStop      chan struct{}
	hbStopped   bool
	wg          sync.WaitGroup
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
	if len(opts.borderEdges) == 0 {
		// All four, since A18. It means "the edges on which this mod will accept an
		// inbound organism", and under D17 it is EQUAL to exportEdges rather than a
		// superset of it (contract-a.md §18, A38).
		opts.borderEdges = []string{contracta.EdgeE, contracta.EdgeN, contracta.EdgeW, contracta.EdgeS}
	}
	if len(opts.exportEdges) == 0 && !opts.omitExportEdges {
		// A conformant mod under two-way lanes declares all four: every declared
		// edge is BOTH an export edge and an entry edge, and "out and in are
		// different doors" is retired (contract-a.md §18, A38). A test that wants
		// the two-edge world of D13 — still legal, and the mixed-rig case §18 A41
		// checks — names it.
		opts.exportEdges = []string{contracta.EdgeE, contracta.EdgeN,
			contracta.EdgeW, contracta.EdgeS}
	}
	if opts.simStep == 0 {
		opts.simStep = 1
	}

	if opts.token == "" {
		opts.token = testContractAToken
	}

	ctx, cancel := context.WithCancel(context.Background())
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	dialOpts := &websocket.DialOptions{CompressionMode: websocket.CompressionDisabled}
	if !opts.noToken {
		// contract-a.md §21 A47: the token rides the HTTP UPGRADE, verified before
		// the WebSocket exists, and never in a frame.
		dialOpts.HTTPHeader = http.Header{
			"Authorization": []string{modtoken.Header(opts.token)},
		}
	}
	ws, _, err := websocket.Dial(dialCtx, opts.url, dialOpts)
	dialCancel()
	if err != nil {
		cancel()
		t.Fatalf("fake mod: dial %s: %v", opts.url, err)
	}
	ws.SetReadLimit(wire.MaxFrameBytes)

	m := &fakeMod{
		t: t, opts: opts, ws: ws, ctx: ctx, cancel: cancel,
		edges: map[string]contracta.EdgeState{}, ackMode: opts.ackMode, world: opts.world,
		simStep: opts.simStep, closeCode: -1, hbStop: make(chan struct{}),
		censusRaw: opts.censusRaw, settingsRaw: opts.settingsRaw,
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
		m.edgeHistory = append(m.edgeHistory, st)
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
	cfg := contracta.ConfigUpdate{
		SessionID:      m.world.sessionID,
		Reason:         reason,
		GameVersion:    m.opts.gameVersion,
		ModVersion:     m.opts.modVersion,
		SimulationSize: &simSize,
		BorderEdges:    m.opts.borderEdges,
		ExportEdges:    m.opts.exportEdges,
		BorderWidth:    &borderWidth,
		RingSlot:       m.opts.ringSlot,
		WorldName:      "M4-Slot",
	}
	m.mu.Lock()
	settings := m.settingsRaw
	m.mu.Unlock()
	if settings == "" {
		m.sendFrame(contracta.TypeConfigUpdate, cfg)
		return
	}
	// The settings go on as RAW BYTES, so a test can send a shape no Go type
	// can hold. Everything else on the frame is built normally.
	body, err := json.Marshal(cfg)
	if err != nil {
		m.t.Errorf("fake mod: encode CONFIG_UPDATE: %v", err)
		return
	}
	spliced := append(append([]byte{}, body[:len(body)-1]...), ',')
	spliced = append(spliced, settings...)
	spliced = append(spliced, '}')
	m.sendFrame(contracta.TypeConfigUpdate, json.RawMessage(spliced))
}

// setSettings changes what rides the next CONFIG_UPDATE and re-handshakes with
// the reason §5.1's enum has carried since M2 for exactly this. It is the whole
// mechanism §19 A42 points at for a setting that ever becomes mutable at
// runtime: no new field, no new message, no new cadence.
func (m *fakeMod) setSettings(raw string) {
	m.mu.Lock()
	m.settingsRaw = raw
	m.mu.Unlock()
	m.sendConfigUpdate("settings_changed")
}

func (m *fakeMod) heartbeatLoop(interval time.Duration) {
	defer m.wg.Done()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.hbStop:
			return
		case <-t.C:
			m.mu.Lock()
			if m.hbSuppressed {
				// App-silent: the socket stays open, but no HEARTBEAT is sent.
				m.mu.Unlock()
				continue
			}
			census := m.censusRaw
			m.simTick++
			tick := m.simTick
			step := m.simStep
			paused := m.paused
			if !paused {
				// A paused world's SIMULATED clock does not advance, which is the
				// whole reason the pacing clock is simulated (contract-a.md §7.5).
				m.simTime += step
			}
			simTime := m.simTime
			m.mu.Unlock()
			pop := m.world.alivePopulation()
			eggs := 3
			scale := 1.0
			if paused {
				scale = 0
			}
			simSize := m.opts.simSize
			targetScale := 10.0
			hb := contracta.Heartbeat{
				SessionID: m.world.sessionID, SimTick: &tick, SimulatedTime: &simTime,
				Population: &pop, EggCount: &eggs, Paused: &paused, TimeScale: &scale,
				TargetTimeScale: &targetScale, SimulationSize: &simSize}
			if m.opts.lastSave != nil {
				save := *m.opts.lastSave
				hb.LastSave = &save
			}
			if census == "" {
				m.sendFrame(contracta.TypeHeartbeat, hb)
				continue
			}
			// The census goes on as RAW BYTES, so a test can send a shape no Go
			// type can hold. Everything else on the frame is built normally.
			body, err := json.Marshal(hb)
			if err != nil {
				m.t.Errorf("fake mod: encode HEARTBEAT: %v", err)
				continue
			}
			spliced := append(append([]byte{}, body[:len(body)-1]...), ',')
			spliced = append(spliced, census...)
			spliced = append(spliced, '}')
			m.sendFrame(contracta.TypeHeartbeat, json.RawMessage(spliced))
		}
	}
}

// setCensus changes what rides the next HEARTBEAT. "" stops sending one at all,
// which is the state a mod older than contract-a/2.2 is permanently in.
func (m *fakeMod) setCensus(raw string) {
	m.mu.Lock()
	m.censusRaw = raw
	m.mu.Unlock()
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
	return m.migrateOutFull(migrationID, entityID, edge, position, payload, nil)
}

// migrateOutParents ships the lineage inputs of contract-a.md §14 A12: the
// parent entity IDs, and one opaque blob for each parent still alive locally.
// The mod never looks inside a parent blob (D4).
func (m *fakeMod) migrateOutParents(entityID int32, edge string, position float64,
	parents []contracta.ParentBlob) string {
	m.t.Helper()
	return m.migrateOutFull(wire.NewUUID(), entityID, edge, position, makePayload(entityID), parents)
}

// migrateOutSpecies ships one migration carrying the OPTIONAL species block of
// contract-a.md §16, A30.
func (m *fakeMod) migrateOutSpecies(entityID int32, edge string, position float64,
	species *contracta.Species) string {
	m.t.Helper()
	return m.migrateOutSpeciesFull(wire.NewUUID(), entityID, edge, position,
		makePayload(entityID), nil, species)
}

func (m *fakeMod) migrateOutFull(migrationID string, entityID int32, edge string, position float64,
	payload string, parents []contracta.ParentBlob) string {
	m.t.Helper()
	return m.migrateOutSpeciesFull(migrationID, entityID, edge, position, payload, parents, nil)
}

func (m *fakeMod) migrateOutSpeciesFull(migrationID string, entityID int32, edge string,
	position float64, payload string, parents []contracta.ParentBlob,
	species *contracta.Species) string {
	m.t.Helper()
	out := m.buildMigrateOut(migrationID, entityID, edge, position, payload, parents, species)
	m.sendFrame(contracta.TypeMigrateOut, out)
	return migrationID
}

// migrateOutRawSpecies ships a migration whose `species` value is the RAW JSON
// given, which is how a test reaches the shapes a Go struct cannot express: a
// missing half, a non-string half, a lone parent field, a block that is not an
// object at all. Every one of them must be STRIPPED, never NACKed (§16, A30).
func (m *fakeMod) migrateOutRawSpecies(entityID int32, edge string, position float64,
	rawSpecies string) string {
	m.t.Helper()
	migrationID := wire.NewUUID()
	out := m.buildMigrateOut(migrationID, entityID, edge, position, makePayload(entityID), nil, nil)
	body, err := json.Marshal(out)
	if err != nil {
		m.t.Fatalf("fake mod: marshal MIGRATE_OUT: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		m.t.Fatalf("fake mod: re-read MIGRATE_OUT: %v", err)
	}
	fields["species"] = json.RawMessage(rawSpecies)
	data, err := json.Marshal(fields)
	if err != nil {
		m.t.Fatalf("fake mod: splice species: %v", err)
	}
	m.sendFrame(contracta.TypeMigrateOut, json.RawMessage(data))
	return migrationID
}

// buildMigrateOut records the in-flight organism the way §6.3 requires — inert,
// keeping its migrationId until the sidecar answers — and returns the frame body.
func (m *fakeMod) buildMigrateOut(migrationID string, entityID int32, edge string,
	position float64, payload string, parents []contracta.ParentBlob,
	species *contracta.Species) contracta.MigrateOut {
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
		Parents:        parents,
		Species:        species,
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
	return out
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

// setSimStep changes how fast the world's SIMULATED clock runs, which is the
// only clock the delivery rate limit reads (contract-a.md §7.5).
func (m *fakeMod) setSimStep(step float64) {
	m.mu.Lock()
	m.simStep = step
	m.mu.Unlock()
}

// setPaused makes the mod report paused: true and timeScale 0, which stops the
// pacing clock — no tokens accrue in the dark, so a paused world does not bank
// a burst.
func (m *fakeMod) setPaused(p bool) {
	m.mu.Lock()
	m.paused = p
	m.mu.Unlock()
}

// suppressHeartbeats stops or resumes HEARTBEATs on the live socket. Unlike
// stopHeartbeats it is reversible, so a test can watch the delivery-freshness
// gate close and reopen without a reconnect (contract-a.md §8).
func (m *fakeMod) suppressHeartbeats(v bool) {
	m.mu.Lock()
	m.hbSuppressed = v
	m.mu.Unlock()
}

// isClosed reports whether the sidecar has closed this connection (e.g. the
// §8 4004), so a delivery-gate test can prove it acted in the window BEFORE that.
func (m *fakeMod) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// simTimeNow is the world's simulated clock, which is the only clock the
// delivery rate limit reads.
func (m *fakeMod) simTimeNow() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.simTime
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

// countType is waitFrom's negative twin: how many frames of one type have
// arrived so far, with no waiting and no fatal. A rule that says a frame MUST
// NOT be sent can only be asserted by looking, never by waiting.
func (m *fakeMod) countType(typ string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, e := range m.frames {
		if e.Type == typ {
			n++
		}
	}
	return n
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

// waitEdgeReason waits for a specific open/reason pair. A closing edge can pass
// through more than one reason on its way — a peer that dies takes its mod with
// it, so peer_mod_absent and no_peer both appear — and a test that cares which
// one it settles on has to say so.
func (m *fakeMod) waitEdgeReason(edge string, open bool, reason string, timeout time.Duration) contracta.EdgeState {
	m.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		m.mu.Lock()
		st, ok := m.edges[edge]
		m.mu.Unlock()
		if ok && st.Open == open && st.Reason == reason {
			return st
		}
		if time.Now().After(deadline) {
			m.t.Fatalf("fake mod: edge %s did not reach open=%v reason=%q within %s (state %+v)",
				edge, open, reason, timeout, st)
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
		HTTPHeader: http.Header{
			"Authorization": []string{modtoken.Header(testContractAToken)},
		},
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

// makePayload builds a canned bb8 blob in the saved dialect of
// contract-a.md §5.3 and genome-hash.md §3. It is opaque to everything but
// bb8.Inspect and the genome projection (D4).
//
// The gene values vary with the entity id, so two organisms have two genome
// hashes and the lineage annex in a test says something.
func makePayload(entityID int32) string {
	n := entityID % 1000
	if n < 0 {
		n = -n
	}
	return fmt.Sprintf(`{"transform":{"position":[2000.0,412.77],"rotation":274.11,"scale":0.9312},`+
		`"rb2d":{"px":2000.0,"py":412.77,"vx":6.12,"vy":0.44,"r":274.11},`+
		`"genes":{"tag":"Cyan","speciesID":41,"gen":37,"parent1":-1180911975,`+
		`"genes":{"SizeRatio":%d.5,"ColorG":0.5,"ColorB":0.30000001192092896}},`+
		`"body":{"id":{"id":%d},"health":42.0,"energy":11.5},`+
		`"clock":{"simulatedTime":120507.75},`+
		`"brain":{"isReady":true,"Nodes":[`+
		`{"Type":0,"baseActivation":0.0,"TypeName":"Input","Index":0,"Inov":0,"Desc":"Constant",`+
		`"Value":1.0,"LastInput":0.0,"LastOutput":1.0},`+
		`{"Type":3,"baseActivation":-0.25,"TypeName":"TanH","Index":48,"Inov":117,"Desc":"Hidden1",`+
		`"Value":0.13,"LastInput":0.9,"LastOutput":0.13}],`+
		`"Synapses":[{"Inov":204,"NodeIn":0,"NodeOut":48,"Weight":-1.5,"En":true}]},`+
		`"version":"0.6.3.1"}`, n, entityID)
}

// genomeHashOf is the hash a conformant sidecar must put in the annex for a
// blob makePayload produced.
func genomeHashOf(t *testing.T, payload string) string {
	t.Helper()
	h, err := bb8.GenomeHash(payload, "0.6.3.1")
	if err != nil {
		t.Fatalf("test payload does not hash: %v", err)
	}
	return h
}

// stopHeartbeats silences the mod without closing its socket, which is the
// state contract-a.md §8's heartbeat timeout exists for.
func (m *fakeMod) stopHeartbeats() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hbStopped {
		return
	}
	m.hbStopped = true
	close(m.hbStop)
}

// edgeEpochNow is the last applied EDGE_STATUS epoch.
func (m *fakeMod) edgeEpochNow() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastEpoch
}

// edgeHistorySince returns every EDGE_STATUS applied after epoch, so a test can
// assert that a lane was never disturbed rather than only that it ended open.
func (m *fakeMod) edgeHistorySince(epoch int64) []contracta.EdgeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]contracta.EdgeStatus, 0, len(m.edgeHistory))
	for _, st := range m.edgeHistory {
		if st.Epoch > epoch {
			out = append(out, st)
		}
	}
	return out
}

func (m *fakeMod) lastEdgeStatus() contracta.EdgeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.edgeHistory) == 0 {
		m.t.Fatal("fake mod: no EDGE_STATUS has arrived")
	}
	return m.edgeHistory[len(m.edgeHistory)-1]
}

// waitTypeStatus waits for the first EDGE_STATUS and returns it.
func (m *fakeMod) waitTypeStatus(timeout time.Duration) contracta.EdgeStatus {
	m.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		m.mu.Lock()
		n := len(m.edgeHistory)
		var st contracta.EdgeStatus
		if n > 0 {
			st = m.edgeHistory[n-1]
		}
		m.mu.Unlock()
		if n > 0 {
			return st
		}
		if time.Now().After(deadline) {
			m.t.Fatalf("fake mod: no EDGE_STATUS within %s", timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (m *fakeMod) edgeNow(edge string) contracta.EdgeState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.edges[edge]
}

// closedNow reports whether the sidecar has closed this connection.
func (r *rawMod) closedNow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}
