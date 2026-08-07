// Command fakemod is a Contract A mod-side client with no game behind it.
//
// WHY IT EXISTS, AND WHAT IT IS NOT FOR. The M4 rig needs six deliverable slots
// and this install of BepInEx supports only five modded game instances: its
// DiskLogListener walks LogOutput.log, then LogOutput.log.1 .. .4, and then
// gives up — and an instance that gets no log file never completes the mod's
// startup path, so it sits at the main menu and answers nothing. That is a
// property of the logging host, not of memory or CPU; six games run comfortably
// at about 550 MB each.
//
// So one slot of the local rehearsal is driven by this program instead. It is a
// RIG TOOL, in the same family as worldstat and ringstat, and it is deliberately
// NOT a simulator:
//
//   - It has no world, no organisms of its own and no geometry. It never
//     invents a bb8 blob, because a synthetic genome would pollute the archive's
//     lineage graph with an organism that never lived.
//   - It is a STORE-AND-FORWARD peer. Every organism it exports is one it
//     received: the same payload bytes, the same entity, a new migrationId,
//     through whichever export edge EDGE_STATUS has open — all four of them
//     under D17. Custody, dedup, lineage and exactly-once are therefore
//     exercised for real in every direction that slot can send.
//   - It answers MIGRATE_IN with MIGRATE_IN_ACK only after it has recorded the
//     organism, which is the same custody gate a real mod's spawn is.
//
// Its held set is written to <state-file> after every change, so the rig can
// count organisms in this slot exactly as it counts them in a real world.
//
// On the LAN phase this program is replaced by a real game on the second
// computer, which has its own BepInEx and therefore its own log file.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contracta"
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

type held struct {
	MigrationID string  `json:"migrationId"`
	EntityID    int32   `json:"entityId"`
	Payload     string  `json:"-"`
	GameVersion string  `json:"gameVersion"`
	EntryEdge   string  `json:"entryEdge"`
	Position    float64 `json:"entryPosition"`
	Velocity    contracta.Vec
	Heading     float64
	// Species is the block this arrival carried, kept so the onward hop carries
	// the same one (contract-a.md §16, A30). A real mod re-reads it from the live
	// Species record every hop; this program has no registry and no world, so
	// carrying the arrival's own block verbatim is the honest analogue — and it
	// is what lets a species name cross a synthetic peer in a LAN rig instead of
	// dying at it.
	Species   *contracta.Species `json:"species,omitempty"`
	ArrivedAt time.Time          `json:"arrivedAt"`
}

type stateFile struct {
	PeerID      string  `json:"peerId"`
	RingSlot    int     `json:"ringSlot"`
	Population  int     `json:"population"`
	EntityIDs   []int32 `json:"entityIds"`
	Arrivals    int     `json:"arrivals"`
	Departures  int     `json:"departures"`
	Duplicates  int     `json:"duplicates"`
	OpenEdges   string  `json:"openEdges"`
	UpdatedAtMs int64   `json:"updatedAtMs"`
}

type fakeMod struct {
	log       *slog.Logger
	sessionID string
	simSize   float64
	edgesOut  []string
	edgesAll  []string
	ringSlot  int
	statePath string
	holdFor   time.Duration
	timeScale float64
	ackDelay  time.Duration
	// census is the species census this peer reports on every HEARTBEAT, or nil
	// when the operator asked for none. It is a FIXED list from a flag, not a
	// measurement: this program has no world, so the honest thing it can do is
	// report exactly what it was told to report, and nil rather than [] when it
	// was told nothing (contract-a.md §17, A35).
	census *wire.Census

	conn *wsutil.Conn

	mu         sync.Mutex
	openEdges  map[string]bool
	lastEpoch  int64
	holding    map[string]*held
	byEntity   map[int32]string
	inFlight   map[string]*held
	nextEdge   int
	simTick    int64
	simTime    float64
	arrivals   int
	departures int
	duplicates int
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("fakemod", flag.ContinueOnError)
	url := fs.String("url", "ws://127.0.0.1:8792"+contracta.ContractAPath, "the sidecar's Contract A address")
	ringSlot := fs.Int("ring-slot", 0, "the advisory ringSlot this peer reports; a disagreement closes 4001")
	// Four, under D17 (contract-a.md §18, A38): every declared edge is both an
	// export edge and an entry edge, and a conformant mod declares all four. A
	// peer that declares fewer is still conformant — the field declares GEOMETRY,
	// "I run a capture band on these edges", and never topology.
	exportEdges := fs.String("export-edges", "E,N,W,S", "the export edges this peer declares")
	simSize := fs.Float64("sim-size", 2000, "S, the playable half-extent it reports")
	borderWidth := fs.Float64("border-width", 40, "W, the strip width it reports")
	statePath := fs.String("state-file", "", "where to write the held set, for the rig's census")
	holdFor := fs.Duration("hold", 45*time.Second, "how long an arrival is held before it is forwarded on")
	ackDelay := fs.Duration("ack-delay", 0,
		"wait this long between journaling an arrival and answering MIGRATE_IN_ACK. A real "+
			"game answers in a tick; a rig that needs to kill a peer AFTER it took custody and "+
			"BEFORE its acknowledgement left — contract-b-m4.md §9.3's dangerous case — needs "+
			"that window to be wide enough to aim at")
	tickEvery := fs.Duration("tick", time.Second, "HEARTBEAT cadence, in wall time (contract-a.md §10: 1000 ms)")
	timeScale := fs.Float64("time-scale", 1,
		"how fast this peer's SIMULATED clock runs against the wall clock. It has to match the "+
			"rig's game instances: the sidecar paces inbound delivery per simulated minute of the "+
			"RECEIVING world (§7.5), so a synthetic peer left at 1x beside worlds at 5x throttles "+
			"itself five times harder than they do and grows a backlog that never drains")
	speciesJSON := fs.String("species", "",
		"a species census to report on every HEARTBEAT, as the raw JSON array of "+
			"contract-a.md §5.2 — e.g. '[{\"genericName\":\"Izus \",\"specificName\":"+
			"\"copedylanus\",\"bibites\":96,\"eggs\":14}]'. Empty sends NO field, which is what "+
			"a mod older than contract-a/2.2 does and reads as UNKNOWN on the page; '[]' is the "+
			"different, stronger statement that this world has nothing alive. Names are sent "+
			"RAW, edge whitespace and all, because that is what this field carries (§17, A36)")
	logLevel := fs.String("log-level", "info", "debug, info, warn or error")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(*logLevel)); err != nil {
		lvl = slog.LevelInfo
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))

	out := splitEdges(*exportEdges)
	if len(out) == 0 {
		log.Error("fakemod: --export-edges names no edge")
		return 1
	}
	all := make([]string, 0, 4)
	seen := map[string]bool{}
	for _, e := range out {
		if !contracta.ValidEdge(e) {
			log.Error("fakemod: not an edge", "edge", e)
			return 1
		}
		add(&all, seen, e)
		// The entry edges are DERIVED, exactly as the mod derives them: the
		// opposite of each declared export edge (contract-a.md §15 A18).
		if opp, ok := contracta.Opposite(e); ok {
			add(&all, seen, opp)
		}
	}

	// The census is parsed ONCE, here, so a typo is a startup error instead of a
	// silently stripped field every second for a week. Absent stays absent: an
	// unset flag sends no `species` key at all, which is what the page must read
	// as unknown rather than as an empty world (contract-a.md §17, A35).
	var census *wire.Census
	if *speciesJSON != "" {
		var c wire.Census
		if err := json.Unmarshal([]byte(*speciesJSON), &c); err != nil {
			log.Error("fakemod: --species is not JSON", "err", err)
			return 1
		}
		carried, _, why := wire.CarryCensus(&c, false)
		if carried == nil {
			log.Error("fakemod: --species is not an array of census entries", "reason", why)
			return 1
		}
		if len(why) > 0 {
			log.Error("fakemod: --species holds entries a sidecar would strip", "reasons", why)
			return 1
		}
		census = carried
	}

	m := &fakeMod{
		log: log, sessionID: wire.NewUUID(), simSize: *simSize, timeScale: *timeScale,
		edgesOut: out, edgesAll: all, ringSlot: *ringSlot,
		statePath: *statePath, holdFor: *holdFor, ackDelay: *ackDelay,
		openEdges: map[string]bool{}, holding: map[string]*held{},
		byEntity: map[int32]string{}, inFlight: map[string]*held{},
		census: census,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for ctx.Err() == nil {
		if err := m.session(ctx, *url, *borderWidth, *tickEvery); err != nil && ctx.Err() == nil {
			log.Warn("fakemod: session ended; reconnecting", "err", err)
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
	}
	return 0
}

func add(list *[]string, seen map[string]bool, e string) {
	if seen[e] {
		return
	}
	seen[e] = true
	*list = append(*list, e)
}

func splitEdges(v string) []string {
	out := []string{}
	cur := ""
	flush := func() {
		if cur != "" {
			out = append(out, cur)
			cur = ""
		}
	}
	for _, r := range v {
		switch r {
		case ',', ';', ' ', '\t':
			flush()
		default:
			if r >= 'a' && r <= 'z' {
				r -= 32
			}
			cur += string(r)
		}
	}
	flush()
	return out
}

func (m *fakeMod) session(ctx context.Context, url string, borderWidth float64, tick time.Duration) error {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	ws, _, err := websocket.Dial(dialCtx, url, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	cancel()
	if err != nil {
		return err
	}
	ws.SetReadLimit(wire.MaxFrameBytes)
	m.conn = wsutil.New(ws, 64)
	defer m.conn.CloseNow()

	// §6.2: a new connection is a new session, and the sidecar reasserts custody
	// against a sessionId it has not seen (§7.4).
	m.mu.Lock()
	m.sessionID = wire.NewUUID()
	m.lastEpoch = 0
	m.openEdges = map[string]bool{}
	m.mu.Unlock()

	cfg := contracta.ConfigUpdate{
		SessionID: m.sessionID, Reason: "connect",
		GameVersion: "0.6.3.1", ModVersion: "fakemod/1.0",
		SimulationSize: &m.simSize, BorderEdges: m.edgesAll, ExportEdges: m.edgesOut,
		BorderWidth: &borderWidth, WorldName: "fakemod",
	}
	if m.ringSlot > 0 {
		slot := m.ringSlot
		cfg.RingSlot = &slot
	}
	if err := m.send(contracta.TypeConfigUpdate, cfg); err != nil {
		return err
	}
	m.log.Info("fakemod: connected", "url", url, "sessionId", m.sessionID,
		"exportEdges", m.edgesOut, "borderEdges", m.edgesAll, "ringSlot", m.ringSlot,
		"timeScale", m.timeScale)
	// THIS PEER'S WORLD IS IN MEMORY AND DIES WITH THE PROCESS, unlike a game's
	// save. Rewriting the state file at every handshake is what stops a stale
	// snapshot from being read as a living population after a kill — which
	// double-counts against the sidecar's own journal row for the same organism.
	m.writeState()

	sessCtx, sessCancel := context.WithCancel(ctx)
	defer sessCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); m.beat(sessCtx, tick) }()

	err = m.readLoop(sessCtx)
	sessCancel()
	wg.Wait()
	return err
}

func (m *fakeMod) send(typ string, data any) error {
	frame, err := wire.Encode(wire.ProtocolA, typ, time.Now().UnixMilli(), data)
	if err != nil {
		return err
	}
	return m.conn.Send(frame)
}

// beat drives the heartbeat AND the forwarding. Pacing at the sidecar is
// measured against the RECEIVING world's simulated clock (§7.5), and this is
// that clock: one tick of wall time is one second of simulated time at 1x.
func (m *fakeMod) beat(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		m.mu.Lock()
		m.simTick++
		m.simTime += every.Seconds() * m.timeScale
		pop := len(m.holding)
		inFlight := len(m.inFlight)
		tick, simTime := m.simTick, m.simTime
		m.mu.Unlock()

		paused := false
		scale := m.timeScale
		hb := contracta.Heartbeat{
			SessionID: m.sessionID, SimTick: &tick, SimulatedTime: &simTime,
			Population: &pop, Paused: &paused, TimeScale: &scale,
			SimulationSize: &m.simSize, InFlightOut: &inFlight,
			Species: m.census,
		}
		if err := m.send(contracta.TypeHeartbeat, hb); err != nil {
			return
		}
		m.forwardOne()
	}
}

// forwardOne exports the oldest organism whose hold has expired, through the
// next open export edge in rotation. It never invents a payload: the bytes are
// the ones that arrived.
func (m *fakeMod) forwardOne() {
	m.mu.Lock()
	var pick *held
	var pickID string
	for id, h := range m.holding {
		if time.Since(h.ArrivedAt) < m.holdFor {
			continue
		}
		if pick == nil || h.ArrivedAt.Before(pick.ArrivedAt) {
			pick, pickID = h, id
		}
	}
	if pick == nil {
		m.mu.Unlock()
		return
	}
	edge := ""
	for i := 0; i < len(m.edgesOut); i++ {
		cand := m.edgesOut[(m.nextEdge+i)%len(m.edgesOut)]
		if m.openEdges[cand] {
			edge = cand
			m.nextEdge = (m.nextEdge + i + 1) % len(m.edgesOut)
			break
		}
	}
	if edge == "" {
		m.mu.Unlock()
		return
	}
	delete(m.holding, pickID)
	delete(m.byEntity, pick.EntityID)
	migrationID := wire.NewUUID()
	m.inFlight[migrationID] = pick
	tick := m.simTick
	m.mu.Unlock()

	// exitPosition mirrors where it came in, clamped, which keeps the transverse
	// continuity D3 buys without this program having any geometry of its own.
	pos := pick.Position
	if pos < 0 {
		pos = 0
	}
	if pos > 1 {
		pos = 1
	}
	vel := pick.Velocity
	heading := pick.Heading
	entity := pick.EntityID
	out := contracta.MigrateOut{
		MigrationID: migrationID, EntityID: &entity, Kind: contracta.KindBibite,
		GameVersion: pick.GameVersion, Payload: pick.Payload, Species: pick.Species,
		ExitEdge: edge, ExitPosition: &pos, Velocity: &vel, Heading: &heading,
		SimulationSize: &m.simSize, SimTick: &tick,
	}
	if err := m.send(contracta.TypeMigrateOut, out); err != nil {
		m.log.Warn("fakemod: MIGRATE_OUT failed", "err", err)
		return
	}
	m.log.Info("fakemod: MIGRATE_OUT", "migrationId", migrationID, "entityId", entity,
		"exitEdge", edge, "wasMigrationId", pickID)
	m.writeState()
}

func (m *fakeMod) readLoop(ctx context.Context) error {
	for {
		raw, err := m.conn.Read(ctx)
		if err != nil {
			return err
		}
		env, err := wire.Decode(raw)
		if err != nil {
			m.log.Warn("fakemod: undecodable frame", "err", err)
			continue
		}
		switch env.Type {
		case contracta.TypeEdgeStatus:
			m.onEdgeStatus(env)
		case contracta.TypeMigrateIn:
			m.onMigrateIn(env)
		case contracta.TypeMigrateOutAck:
			m.onMigrateOutAck(env)
		case contracta.TypeMigrateOutNack:
			m.onMigrateOutNack(env)
		}
	}
}

func (m *fakeMod) onEdgeStatus(env wire.Envelope) {
	var st contracta.EdgeStatus
	if json.Unmarshal(env.Data, &st) != nil {
		return
	}
	m.mu.Lock()
	if st.Epoch <= m.lastEpoch {
		m.mu.Unlock()
		return
	}
	m.lastEpoch = st.Epoch
	// §5.4: the frame is the WHOLE state, not a delta. A declared edge with no
	// entry is closed.
	next := map[string]bool{}
	for _, e := range st.Edges {
		next[e.Edge] = e.Open
	}
	m.openEdges = next
	open := m.describeEdgesLocked()
	m.mu.Unlock()
	m.log.Info("fakemod: EDGE_STATUS", "epoch", st.Epoch, "entries", len(st.Edges), "edges", open)
	m.writeState()
}

func (m *fakeMod) describeEdgesLocked() string {
	s := ""
	for _, e := range m.edgesOut {
		if s != "" {
			s += " "
		}
		if m.openEdges[e] {
			s += e + ":open"
		} else {
			s += e + ":closed"
		}
	}
	return s
}

func (m *fakeMod) onMigrateIn(env wire.Envelope) {
	var in contracta.MigrateIn
	if json.Unmarshal(env.Data, &in) != nil {
		return
	}
	if in.MigrationID == "" {
		// §13 A2: no answer channel, so drop it with one logged error.
		m.log.Error("fakemod: MIGRATE_IN with no migrationId — dropped")
		return
	}
	m.mu.Lock()
	duplicate := false
	// §7.3: entityId is the DURABLE dedup key, migrationId the transport one.
	if _, ok := m.holding[in.MigrationID]; ok {
		duplicate = true
	} else if _, ok := m.byEntity[in.EntityID]; ok {
		duplicate = true
	}
	if !duplicate {
		m.holding[in.MigrationID] = &held{
			MigrationID: in.MigrationID, EntityID: in.EntityID, Payload: in.Payload,
			GameVersion: in.GameVersion, EntryEdge: in.EntryEdge, Position: in.EntryPosition,
			Velocity: in.Velocity, Heading: in.Heading, Species: in.Species, ArrivedAt: time.Now(),
		}
		m.byEntity[in.EntityID] = in.MigrationID
		m.arrivals++
	} else {
		m.duplicates++
	}
	tick := m.simTick
	m.mu.Unlock()

	entity := in.EntityID
	dup := duplicate
	ack := func() {
		if err := m.send(contracta.TypeMigrateInAck, contracta.MigrateInAck{
			MigrationID: in.MigrationID, EntityID: &entity, Duplicate: &dup, SimTick: &tick,
		}); err != nil {
			m.log.Warn("fakemod: MIGRATE_IN_ACK failed", "err", err)
		}
	}
	if m.ackDelay > 0 {
		// The organism is already recorded, so custody HAS moved here. Only the
		// statement of it is delayed, which is exactly §9.3's dangerous case.
		m.log.Info("fakemod: custody taken, acknowledgement delayed",
			"migrationId", in.MigrationID, "entityId", in.EntityID, "delay", m.ackDelay)
		go func() { time.Sleep(m.ackDelay); ack() }()
		m.writeState()
		return
	}
	ack()
	m.log.Info("fakemod: MIGRATE_IN accepted", "migrationId", in.MigrationID,
		"entityId", in.EntityID, "entryEdge", in.EntryEdge, "bounceBack", in.BounceBack,
		"duplicate", duplicate)
	m.writeState()
}

func (m *fakeMod) onMigrateOutAck(env wire.Envelope) {
	var ack struct {
		MigrationID string `json:"migrationId"`
	}
	if json.Unmarshal(env.Data, &ack) != nil {
		return
	}
	m.mu.Lock()
	h := m.inFlight[ack.MigrationID]
	delete(m.inFlight, ack.MigrationID)
	if h != nil {
		m.departures++
	}
	m.mu.Unlock()
	if h != nil {
		m.log.Info("fakemod: MIGRATE_OUT_ACK — custody released", "migrationId", ack.MigrationID,
			"entityId", h.EntityID)
	}
	m.writeState()
}

// onMigrateOutNack puts the organism back, because a NACK means the sidecar took
// no custody (§5.6). The mod is the only place it exists at that moment.
func (m *fakeMod) onMigrateOutNack(env wire.Envelope) {
	var nack struct {
		MigrationID string `json:"migrationId"`
		Code        string `json:"code"`
		Message     string `json:"message"`
	}
	if json.Unmarshal(env.Data, &nack) != nil {
		return
	}
	m.mu.Lock()
	h := m.inFlight[nack.MigrationID]
	delete(m.inFlight, nack.MigrationID)
	if h != nil {
		h.ArrivedAt = time.Now()
		m.holding[h.MigrationID] = h
		m.byEntity[h.EntityID] = h.MigrationID
	}
	m.mu.Unlock()
	m.log.Info("fakemod: MIGRATE_OUT_NACK — the organism stays here", "migrationId", nack.MigrationID,
		"code", nack.Code, "message", nack.Message)
	m.writeState()
}

func (m *fakeMod) writeState() {
	if m.statePath == "" {
		return
	}
	m.mu.Lock()
	st := stateFile{
		PeerID: "fakemod", RingSlot: m.ringSlot, Population: len(m.holding),
		Arrivals: m.arrivals, Departures: m.departures, Duplicates: m.duplicates,
		OpenEdges: m.describeEdgesLocked(), UpdatedAtMs: time.Now().UnixMilli(),
	}
	for _, h := range m.holding {
		st.EntityIDs = append(st.EntityIDs, h.EntityID)
	}
	m.mu.Unlock()

	body, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	tmp := m.statePath + ".tmp"
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0o755); err != nil {
		return
	}
	if err := os.WriteFile(tmp, append(body, '\n'), 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, m.statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "fakemod: state rename failed: %v\n", err)
	}
}
