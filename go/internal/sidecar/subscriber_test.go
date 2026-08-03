package sidecar

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contractb"
	"multiverse/internal/lantoken"
	"multiverse/internal/wire"
)

// subscriber is a bare role:"archive" client. It exists so a test can try the
// things contract-b-m3.md §5.1 forbids a subscriber to do, which the real
// archive never attempts.
type subscriber struct {
	t      *testing.T
	ws     *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	frames []wire.Envelope
}

func dialSubscriber(t *testing.T, relayURL, token string) *subscriber {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	ws, _, err := websocket.Dial(dialCtx, relayURL, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
		HTTPHeader:      http.Header{"Authorization": []string{lantoken.Header(token)}},
	})
	dialCancel()
	if err != nil {
		cancel()
		t.Fatalf("subscriber: dial %s: %v", relayURL, err)
	}
	ws.SetReadLimit(wire.MaxFrameBytes)
	s := &subscriber{t: t, ws: ws, ctx: ctx, cancel: cancel}
	go s.readLoop()
	t.Cleanup(func() { cancel(); _ = ws.CloseNow() })

	s.send(contractb.TypeHandshake, contractb.Handshake{
		PeerID:          "archive-probe",
		Role:            contractb.RoleArchive,
		ProtocolVersion: wire.ProtocolB,
		SidecarVersion:  "test",
	})
	return s
}

func (s *subscriber) readLoop() {
	for {
		_, data, err := s.ws.Read(s.ctx)
		if err != nil {
			return
		}
		env, decodeErr := wire.Decode(data)
		if decodeErr != nil {
			continue
		}
		s.mu.Lock()
		s.frames = append(s.frames, env)
		s.mu.Unlock()
	}
}

func (s *subscriber) send(typ string, data any) {
	frame, err := wire.Encode(wire.ProtocolB, typ, time.Now().UnixMilli(), data)
	if err != nil {
		s.t.Fatalf("subscriber: encode %s: %v", typ, err)
	}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	_ = s.ws.Write(ctx, websocket.MessageText, frame)
}

func (s *subscriber) wait(typ string, timeout time.Duration) wire.Envelope {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		for _, env := range s.frames {
			if env.Type == typ {
				s.mu.Unlock()
				return env
			}
		}
		s.mu.Unlock()
		if time.Now().After(deadline) {
			s.t.Fatalf("subscriber: no %s within %s", typ, timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// sendMigrationAndWaitNack tries the one thing §5.1 answers NOT_A_MEMBER.
func (s *subscriber) sendMigrationAndWaitNack(t *testing.T, timeout time.Duration) contractb.MigrationNack {
	t.Helper()
	s.wait(contractb.TypeHandshakeAck, timeout)
	s.send(contractb.TypeMigrationPayload, contractb.MigrationPayload{
		MigrationID:  wire.NewUUID(),
		Kind:         "bibite",
		Body:         contractb.Body{Version: "0.6.3.1", BB8: makePayload(testEntityID)},
		Lineage:      contractb.Lineage{Parents: []contractb.Parent{}},
		SourcePeer:   "archive-probe",
		SourceSlot:   0,
		DestSlot:     1,
		ExitEdge:     "E",
		ExitPosition: 0.5,
		EntityID:     testEntityID,
		Timestamp:    time.Now().UnixMilli(),
	})
	env := s.wait(contractb.TypeMigrationNack, timeout)
	var nack contractb.MigrationNack
	if err := json.Unmarshal(env.Data, &nack); err != nil {
		t.Fatalf("subscriber: decode NACK: %v", err)
	}
	return nack
}

// claimAndWaitGrant tries a SECTOR_CLAIM, which §7.2 rule 5 refuses.
func (s *subscriber) claimAndWaitGrant(t *testing.T, timeout time.Duration) contractb.SectorGrant {
	t.Helper()
	s.send(contractb.TypeSectorClaim, contractb.SectorClaim{
		SimulationSize: 0, ExportEdge: "", BorderEdges: []string{}, ModConnected: false})
	env := s.wait(contractb.TypeSectorGrant, timeout)
	var grant contractb.SectorGrant
	if err := json.Unmarshal(env.Data, &grant); err != nil {
		t.Fatalf("subscriber: decode grant: %v", err)
	}
	return grant
}

// probeRelay does the HTTP upgrade by hand so a test can read the status and
// the WWW-Authenticate header. §3.1: a wrong token gets 401 and no upgrade, so
// there is no WebSocket and no close code to inspect.
func probeRelay(t *testing.T, addr, token string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://"+addr+contractb.ContractBPath, nil)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	if token != "" {
		req.Header.Set("Authorization", lantoken.Header(token))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, strings.TrimSpace(resp.Header.Get("WWW-Authenticate"))
}
