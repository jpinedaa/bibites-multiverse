package sidecar

// The tests of the participant's own-slot view, written against DQ8's bar: "a
// participant can read their own slot's liveness, lanes, paced depth and last
// save without an operator" (m5_tracking.md, WP7).

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contracta"
)

// TestTheOwnSlotViewAnswersTheFourThingsAParticipantMustReadAlone is the
// deliverable itself. Each of the four is a thing that today lives on the
// operator's page, and each has to be readable from the participant's own
// machine with nobody else involved.
func TestTheOwnSlotViewAnswersTheFourThingsAParticipantMustReadAlone(t *testing.T) {
	g := healthyGrid(t)
	nd := g.node(0)

	res := fetchOwnSlot(nd.cfg.DataDir, 3*time.Second)
	if !res.OK {
		t.Fatalf("the view could not be read: %s", res.Why)
	}
	v := res.View

	// 1. LIVENESS — this world's own place on the map, and whether the map says
	//    it is live.
	if v.Slot.Slot == 0 {
		t.Error("the view reports no slot")
	}
	if !v.Relay.Connected {
		t.Error("the view reports the map link down on a rig where it is up")
	}
	if !v.Slot.GrantSeen || !v.Slot.Granted {
		t.Error("the view does not carry the map's own answer to this world's claim")
	}
	// 2. LANES — one entry per declared export edge, with its reason.
	if len(v.Edges) != 4 {
		t.Errorf("the view carries %d lanes, want one per declared edge", len(v.Edges))
	}
	open := 0
	for _, e := range v.Edges {
		if e.Open {
			open++
			if e.PeerSlot == 0 {
				t.Errorf("the %s lane is open and names no peer", e.Edge)
			}
		} else if e.Reason == "" {
			t.Errorf("the %s lane is closed and gives no reason", e.Edge)
		}
	}
	if open == 0 {
		t.Error("no lane is open on a 2x2 map where every axis exists")
	}
	// 3. PACED DEPTH, against the rate it is queued behind — a queue is only
	//    deep against the cap it is queued behind.
	if v.Custody.InboundRatePerSimMinute <= 0 {
		t.Error("the view reports a paced depth with no delivery rate beside it")
	}
	// 4. LAST SAVE, and the speed, which the participant documentation's own
	//    table promises beside it.
	if !v.Mod.Connected {
		t.Fatal("the view reports no game connected on a rig where one is")
	}
	if v.Mod.TimeScale == nil {
		t.Error("the view reports no applied speed")
	}
	if v.Mod.GameVersion == "" || v.Mod.ModVersion == "" {
		t.Error("the view does not carry the versions a support conversation asks for")
	}
	// And the neighbours, which is the half no participant could see before.
	if len(v.Peers) != 4 {
		t.Errorf("the view carries %d worlds, want the whole map", len(v.Peers))
	}
	me := 0
	for _, p := range v.Peers {
		if p.Me {
			me++
		}
	}
	if me != 1 {
		t.Errorf("%d rows are marked as this world; exactly one must be", me)
	}
}

// TestTheOwnSlotViewAsksTheMapForNothing is D1's constraint on this work: the
// sidecar already knows everything from PEER_STATUS, so the view adds no wire
// message and no request. Reading it a hundred times must cost the map nothing.
func TestTheOwnSlotViewAsksTheMapForNothing(t *testing.T) {
	g := healthyGrid(t)
	nd := g.node(0)

	nd.side.mu.Lock()
	before := nd.side.sent.totalFrames
	nd.side.mu.Unlock()

	for i := 0; i < 100; i++ {
		if res := fetchOwnSlot(nd.cfg.DataDir, 3*time.Second); !res.OK {
			t.Fatalf("read %d failed: %s", i, res.Why)
		}
	}

	nd.side.mu.Lock()
	after := nd.side.sent.totalFrames
	nd.side.mu.Unlock()
	// The sidecar's own timers keep sending while this runs, so the bar is not
	// zero: it is that a HUNDRED reads did not put a hundred frames on the wire.
	if after-before > 20 {
		t.Fatalf("100 reads of the view cost %d frames on the map wire; the view is supposed to "+
			"be derived from state this process already had", after-before)
	}
}

// TestTheOwnSlotEndpointIsReadOnlyAndCarriesNoSecret is the surface's own
// contract: it answers GET, it refuses everything else, and it prints the PATH
// of each secret and never a secret.
func TestTheOwnSlotEndpointIsReadOnlyAndCarriesNoSecret(t *testing.T) {
	g := healthyGrid(t)
	nd := g.node(0)
	addr, err := readListenAddr(nd.cfg.DataDir)
	if err != nil {
		t.Fatalf("listen.addr: %v", err)
	}
	url := "http://" + addr + OwnSlotPath

	resp, err := http.Post(url, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST answered HTTP %d, want 405: this endpoint changes nothing and must not "+
			"look as though it could", resp.StatusCode)
	}
	if strings.Contains(string(body), nd.cfg.Secret) {
		t.Fatal("the refusal carries the credential")
	}

	get, err := http.Get(url)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	raw, _ := io.ReadAll(get.Body)
	get.Body.Close()
	if get.StatusCode != http.StatusOK {
		t.Fatalf("GET answered HTTP %d", get.StatusCode)
	}
	var v OwnSlot
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("the endpoint did not answer a readable view: %v", err)
	}
	if v.Schema != OwnSlotSchema {
		t.Fatalf("schema %q, want %q", v.Schema, OwnSlotSchema)
	}
	if strings.Contains(string(raw), nd.cfg.Secret) ||
		strings.Contains(string(raw), nd.cfg.ContractAToken) {
		t.Fatal("the view carries a secret")
	}
	if v.ContractATokenFile == "" {
		t.Error("the view does not name the token file's path, which is the fact a participant " +
			"needs and is not the secret")
	}
	if !v.CredentialConfigured {
		t.Error("the view does not say whether a credential is configured at all")
	}
}

// TestTheOwnSlotViewNeverRendersTextItDidNotAuthorAsMarkup is B30's escaping
// obligation on the surface WP7 adds. A peer id, a game version and a
// lastRefusal all arrive from somewhere else, and this one renders on the
// participant's own terminal.
func TestTheOwnSlotViewNeverRendersTextItDidNotAuthorAsMarkup(t *testing.T) {
	hostile := "slot-\x1b]0;pwned\x07evil\x1b[2J"
	v := OwnSlot{
		Schema:     OwnSlotSchema,
		ObservedAt: time.Now().UnixMilli(),
		PeerID:     hostile,
		RelayURL:   "wss://relay.example\x1b[31m",
		Slot: SlotState{Slot: 1, MapWidth: 2, MapHeight: 1,
			LastRefusal: "capacity: \x1b[1mmaxFramesPerSecond\x1b[0m"},
		Mod: ModState{Connected: true, GameVersion: "0.6.\x1b3.1", ModVersion: "0.2.0"},
		Edges: []EdgeState{
			{Edge: contracta.EdgeE, Open: false, Reason: "no_peer\x07", Usable: true},
		},
		Peers: []PeerState{
			{Slot: 1, Me: true, Live: true, ModConnected: true, GameVersion: "0.6.3.1"},
			{Slot: 2, Live: true, GameVersion: "0.6.\x1b[5m4.0"},
		},
	}
	var buf bytes.Buffer
	RenderOwnSlot(&buf, v)
	out := buf.String()
	for _, bad := range []string{"\x1b", "\x07"} {
		if strings.Contains(out, bad) {
			t.Fatalf("the rendered view carries a control character; a terminal's markup IS the "+
				"control character:\n%q", out)
		}
	}
	// It replaces rather than drops, so an operator comparing two surfaces can
	// see that something was there.
	if !strings.Contains(out, "�") {
		t.Fatal("the sanitiser dropped the bytes instead of replacing them")
	}
}

// TestTheOwnSlotViewMeasuresTheAchievedSpeedItself is the reading no wire field
// carries. The applied scale is copied and never computed; the achieved rate is
// Δ simulated time over Δ wall, and the gap between them is the news.
func TestTheOwnSlotViewMeasuresTheAchievedSpeedItself(t *testing.T) {
	var r achievedRate
	start := time.Now()
	// A world advancing five simulated seconds per wall second, sampled every
	// half second for fifteen seconds of wall time.
	for i := 0; i <= 30; i++ {
		at := start.Add(time.Duration(i) * 500 * time.Millisecond)
		r.observe("session-1", at, float64(i)*2.5)
	}
	rate, span, ok := r.rate(start.Add(15 * time.Second))
	if !ok {
		t.Fatal("a fifteen-second window of continuous heartbeats produced no rate")
	}
	if rate < 4.9 || rate > 5.1 {
		t.Fatalf("achieved rate %v, want about 5", rate)
	}
	if span < achievedMinSpan {
		t.Fatalf("the rate was measured over %s, under the minimum span", span)
	}
	// A span too short to smooth the jitter is UNKNOWN and not a number.
	var short achievedRate
	short.observe("session-1", start, 0)
	short.observe("session-1", start.Add(time.Second), 5)
	if _, _, ok := short.rate(start.Add(time.Second)); ok {
		t.Fatal("a one-second span produced a rate; the guard exists because a 100 ms wobble in " +
			"when the mod read its clock is a 10% error over one second")
	}
	// A different Contract A session is a different world, whatever the clocks
	// read.
	r.observe("session-2", start.Add(16*time.Second), 0)
	if _, _, ok := r.rate(start.Add(16 * time.Second)); ok {
		t.Fatal("the window survived a new session; nothing before it belongs to it")
	}
}

// TestTheProcessRecordTellsALiveSidecarFromAStaleRecord is what stale-process
// reads, and the reason it needs two facts rather than one: pid numbers are
// reused after a reboot, so a record alone is never evidence.
func TestTheProcessRecordTellsALiveSidecarFromAStaleRecord(t *testing.T) {
	g := healthyGrid(t)
	nd := g.node(0)

	rec, ok := readProcessRecord(nd.cfg.DataDir)
	if !ok {
		t.Fatal("a running sidecar wrote no process record")
	}
	if rec.PID != os.Getpid() {
		t.Fatalf("the record names pid %d and this process is %d", rec.PID, os.Getpid())
	}
	if rec.PeerID != nd.cfg.PeerID || rec.Listen == "" {
		t.Fatalf("the record does not name what it is a record OF: %+v", rec)
	}
	rep := diagnoseOn(t, nd, nil)
	mustVerdict(t, rep, "stale-process", VerdictPass)

	// A record naming a process that is gone is a WARNING and never a failure:
	// it is left behind by a kill, and the next start overwrites it.
	stale := t.TempDir()
	if err := os.WriteFile(filepath.Join(stale, processRecordName),
		[]byte(`{"pid":999999,"startedAtMs":1,"peerId":"ghost","listen":"127.0.0.1:1"}`),
		0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rep = Diagnose(DiagnoseOptions{DataDir: stale, RelayURL: "ws://127.0.0.1:1",
		Timeout: 300 * time.Millisecond})
	c := mustVerdict(t, rep, "stale-process", VerdictWarn)
	if c.Actor != ActorNobody {
		t.Fatalf("actor %q, want nobody: the next start clears it", c.Actor)
	}
	if len(c.Taxonomy) == 0 {
		t.Fatal("the warning carries no taxonomy id, and rule 5 says every one must")
	}
}

// TestAStoppedSidecarLeavesNoProcessRecordBehind: a CLEAN shutdown removes its
// own record, so the warning above means what it says.
func TestAStoppedSidecarLeavesNoProcessRecordBehind(t *testing.T) {
	rl := startRelay(t)
	cfg := fastConfig(t, rl, "peer-stopper")
	side := startSidecar(t, cfg)
	if _, ok := readProcessRecord(cfg.DataDir); !ok {
		t.Fatal("a started sidecar wrote no process record")
	}
	if err := side.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, ok := readProcessRecord(cfg.DataDir); ok {
		t.Fatal("a cleanly stopped sidecar left its process record behind, which would make " +
			"every later diagnosis claim it is running")
	}
}

// TestTheOwnSlotCommandSaysWhatToDoWhenThereIsNothingToRead. A participant who
// runs this with their sidecar stopped must be sent somewhere useful rather than
// told nothing.
func TestTheOwnSlotCommandSaysWhatToDoWhenThereIsNothingToRead(t *testing.T) {
	dir := t.TempDir()
	var out, errBuf bytes.Buffer
	code := mySlotCommand(dir, time.Second, false, &out, &errBuf)
	if code != ExitFail {
		t.Fatalf("exit %d, want %d", code, ExitFail)
	}
	msg := errBuf.String()
	if !strings.Contains(msg, "--diagnose") {
		t.Fatalf("the failure does not send the reader at the command that answers without a "+
			"running sidecar:\n%s", msg)
	}
	if !strings.Contains(msg, "no live view") {
		t.Fatalf("the failure does not say what is missing:\n%s", msg)
	}
}
