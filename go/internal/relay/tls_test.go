package relay

// B23's tests: contract-b-m4.md §22, TLS at the relay's front door.
//
// Three rules are testable at this layer and each one is here:
//
//   - A PLAIN ws:// UPGRADE OFF LOOPBACK IS REFUSED WITH HTTP 426, and with an
//     Upgrade header rather than a redirect, because a redirect to a scheme the
//     client did not ask for is how a downgrade goes unnoticed.
//   - A CERTIFICATE A CLIENT CANNOT VERIFY FAILS THE CONNECTION. The client half
//     of that rule lives with the client (internal/sidecar/credential_test.go);
//     what is asserted here is that the relay presents a real certificate and
//     that nothing on the server side offers a way around one.
//   - A ROTATION COSTS A CONNECTED PEER NOTHING. The relay must be able to load
//     a renewed certificate WITHOUT DROPPING SESSIONS, so an established session
//     survives the swap and the next handshake gets the new certificate.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
	"multiverse/internal/wire"
)

// selfSignedPair writes a certificate and key for 127.0.0.1 into dir, and
// returns the paths plus a pool that trusts it. A test-only CA is the honest
// stand-in for the real thing: B23 requires a client to verify against ITS
// PLATFORM'S TRUST STORE, so a test that wants a verifiable certificate has to
// supply the trust rather than ask the client to skip the check.
func selfSignedPair(t *testing.T, dir, commonName string, notAfter time.Time) (certPath, keyPath string, pool *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName, Organization: []string{"multiverse test"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{commonName},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	pool = x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		t.Fatal("the generated certificate did not parse back")
	}
	return certPath, keyPath, pool
}

// tlsRelay is a relay terminating its own TLS on 127.0.0.1, built through the
// same TLSListener and CertReloader the shipped binary uses.
type tlsRelay struct {
	url      string
	store    *peercred.Store
	reloader *CertReloader
	srv      *http.Server
}

func startTLSRelay(t *testing.T, certPath, keyPath string) *tlsRelay {
	t.Helper()
	store, err := peercred.OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if _, err := store.Mint("bootstrap-peer", peercred.GrantPeer); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	srv, err := New(Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})),
		Credentials:    store,
		PingInterval:   time.Second,
		PeerTimeout:    30 * time.Second,
		StatusCoalesce: 10 * time.Millisecond,
		StatsBroadcast: time.Hour,
	})
	if err != nil {
		t.Fatalf("relay: new: %v", err)
	}
	reloader, err := NewCertReloader(certPath, keyPath)
	if err != nil {
		t.Fatalf("NewCertReloader: %v", err)
	}
	minVersion, err := TLSMinVersion(contractb.RelayTLSMinVersion)
	if err != nil {
		t.Fatalf("TLSMinVersion: %v", err)
	}
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := raw.Addr().String()
	ln := TLSListener(raw, reloader, minVersion)
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = httpSrv.Serve(ln) }()
	t.Cleanup(func() { _ = httpSrv.Close(); srv.Close() })
	return &tlsRelay{
		url:      "wss://" + addr + contractb.ContractBPath,
		store:    store,
		reloader: reloader,
		srv:      httpSrv,
	}
}

func (r *tlsRelay) mint(t *testing.T, peerID string) string {
	t.Helper()
	secret, err := r.store.Mint(peerID, peercred.GrantPeer)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return secret
}

func dialTLS(ctx context.Context, r *tlsRelay, peerID, secret string, pool *x509.CertPool) (*websocket.Conn, error) {
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
	}}
	ws, _, err := websocket.Dial(ctx, r.url, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
		HTTPClient:      client,
		HTTPHeader:      http.Header{"Authorization": []string{peercred.Header(peerID, secret)}},
	})
	return ws, err
}

// TestPlainUpgradeOffLoopbackIsRefusedWith426 is B23's scheme row. The refusal
// is 426 with `Upgrade: TLS/1.2, HTTP/1.1` AND NOT A REDIRECT, and the carve-out
// is loopback — a single-machine rehearsal, and nothing wider.
func TestPlainUpgradeOffLoopbackIsRefusedWith426(t *testing.T) {
	store, err := peercred.OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	secret, err := store.Mint("peer-a", peercred.GrantPeer)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	srv, err := New(Options{
		Logger:      slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})),
		Credentials: store,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.Close()

	// A NON-LOOPBACK bind. 0.0.0.0 is what the LAN rig uses today, and reaching
	// it through a routable address is what makes this connection's LOCAL address
	// non-loopback — which is exactly the test B23 states.
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = httpSrv.Serve(ln) }()
	t.Cleanup(func() { _ = httpSrv.Close() })

	routable := routableAddr(t)
	if routable == "" {
		t.Skip("this host has no non-loopback IPv4 address to reach the wildcard bind through")
	}
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	remote := net.JoinHostPort(routable, port)

	req, err := http.NewRequest(http.MethodGet, "http://"+remote+contractb.ContractBPath, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Authorization", peercred.Header("peer-a", secret))
	// A redirect must never be the answer, so the client is told not to follow
	// one — if the relay ever sent a 30x this test sees it as a status, not as a
	// silently downgraded success.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upgrade attempt: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("a plain ws:// upgrade off loopback got HTTP %d, want 426 UPGRADE REQUIRED "+
			"(contract-b-m4.md §22, B23)", resp.StatusCode)
	}
	if got := resp.Header.Get("Upgrade"); got != "TLS/1.2, HTTP/1.1" {
		t.Fatalf("the 426 carries Upgrade %q, want \"TLS/1.2, HTTP/1.1\"", got)
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		t.Fatalf("the refusal carries a redirect to %q; B23 forbids one, because a redirect to a "+
			"scheme the client did not ask for is how a downgrade goes unnoticed", loc)
	}

	// The loopback carve-out still works on the SAME listener, which is what
	// keeps a single-machine rehearsal runnable.
	local := net.JoinHostPort("127.0.0.1", port)
	loopReq, _ := http.NewRequest(http.MethodGet, "http://"+local+"/healthz", nil)
	loopResp, err := client.Do(loopReq)
	if err != nil {
		t.Fatalf("loopback probe: %v", err)
	}
	defer loopResp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws://"+local+contractb.ContractBPath, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
		HTTPHeader:      http.Header{"Authorization": []string{peercred.Header("peer-a", secret)}},
	})
	if err != nil {
		t.Fatalf("a plain ws:// upgrade over LOOPBACK was refused (%v); B23 keeps it for a "+
			"single-machine rehearsal", err)
	}
	_ = ws.CloseNow()
}

// TestTLSRelayServesTheWireAndVerifies is the happy path, and it is here to keep
// the refusal tests honest: a relay that refused everything would pass them all.
func TestTLSRelayServesTheWireAndVerifies(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, pool := selfSignedPair(t, dir, "localhost", time.Now().Add(24*time.Hour))
	r := startTLSRelay(t, certPath, keyPath)
	secret := r.mint(t, "peer-a")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, err := dialTLS(ctx, r, "peer-a", secret, pool)
	if err != nil {
		t.Fatalf("wss dial: %v", err)
	}
	defer ws.CloseNow()

	frame, err := wire.Encode(wire.ProtocolB, contractb.TypeHandshake, time.Now().UnixMilli(),
		contractb.Handshake{PeerID: "peer-a", Role: contractb.RolePeer,
			ProtocolVersion: wire.ProtocolB, SidecarVersion: "test"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := ws.Write(ctx, websocket.MessageText, frame); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, data, err := ws.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	env, err := wire.Decode(data)
	if err != nil || env.Type != contractb.TypeHandshakeAck {
		t.Fatalf("first frame over TLS was %+v (%v), want HANDSHAKE_ACK", env, err)
	}

	// AND THE SAME RELAY REFUSES A CLIENT THAT CANNOT VERIFY IT. This is the
	// server-side shadow of B23's client rule: there is no server behaviour that
	// lets an unverifying client through, because verification is the client's
	// own and the relay has no say in it.
	strictCtx, strictCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer strictCancel()
	if _, err := dialTLS(strictCtx, r, "peer-a", secret, x509.NewCertPool()); err == nil {
		t.Fatal("a client with an empty trust store completed the TLS handshake")
	}
}

// TestCertificateRotationDoesNotDropSessions is B23's rotation row, and it is
// the row an operator's restart policy depends on: "A rotation replaces the
// certificate the LISTENER presents to the NEXT handshake; an established TLS
// session is unaffected and its WebSocket stays up. THE RELAY MUST BE ABLE TO
// LOAD A RENEWED CERTIFICATE WITHOUT DROPPING SESSIONS."
//
// Where a relay CANNOT do that, the rotation becomes a routine restart and D24's
// restart policy has to tell participants what it looks like — so this test is
// what decides which of those two sentences describes this implementation.
func TestCertificateRotationDoesNotDropSessions(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, firstPool := selfSignedPair(t, dir, "localhost", time.Now().Add(24*time.Hour))
	r := startTLSRelay(t, certPath, keyPath)
	secret := r.mint(t, "peer-a")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	established, err := dialTLS(ctx, r, "peer-a", secret, firstPool)
	if err != nil {
		t.Fatalf("wss dial: %v", err)
	}
	defer established.CloseNow()
	frame, _ := wire.Encode(wire.ProtocolB, contractb.TypeHandshake, time.Now().UnixMilli(),
		contractb.Handshake{PeerID: "peer-a", Role: contractb.RolePeer,
			ProtocolVersion: wire.ProtocolB, SidecarVersion: "test"})
	if err := established.Write(ctx, websocket.MessageText, frame); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := established.Read(ctx); err != nil {
		t.Fatalf("read the ack: %v", err)
	}

	// THE ROTATION. A renewed pair is written over the same paths, which is what
	// an ACME client that renews in place does.
	secondDir := t.TempDir()
	newCert, newKey, secondPool := selfSignedPair(t, secondDir, "localhost", time.Now().Add(72*time.Hour))
	copyOver(t, newCert, certPath)
	copyOver(t, newKey, keyPath)

	// HALF ONE: the established session is untouched. It still carries frames,
	// which is the property a participant experiences as "nothing happened".
	ping, _ := wire.Encode(wire.ProtocolB, contractb.TypePing, time.Now().UnixMilli(),
		contractb.Ping{Nonce: wire.NewUUID()})
	if err := established.Write(ctx, websocket.MessageText, ping); err != nil {
		t.Fatalf("the established session broke across a certificate rotation: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	sawPong := false
	for time.Now().Before(deadline) && !sawPong {
		readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
		_, data, err := established.Read(readCtx)
		readCancel()
		if err != nil {
			t.Fatalf("the established session stopped reading across a certificate rotation: %v", err)
		}
		if env, decodeErr := wire.Decode(data); decodeErr == nil && env.Type == contractb.TypePong {
			sawPong = true
		}
	}
	if !sawPong {
		t.Fatal("the established session did not answer across the rotation")
	}

	// HALF TWO: the NEXT handshake gets the NEW certificate, with no restart, no
	// signal and no reload command.
	nextCtx, nextCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer nextCancel()
	second := r.mint(t, "peer-b")
	ws, err := dialTLS(nextCtx, r, "peer-b", second, secondPool)
	if err != nil {
		t.Fatalf("a new handshake did not get the renewed certificate: %v", err)
	}
	_ = ws.CloseNow()

	reloads, failures, lastErr := r.reloader.Stats()
	if reloads == 0 {
		t.Fatalf("the reloader never reloaded (failures %d, lastErr %v); the certificate on the "+
			"wire would age out with the process", failures, lastErr)
	}

	// AND A HALF-WRITTEN RENEWAL KEEPS SERVING WHAT IS ALREADY LOADED. A cert
	// file newer than its key is the one instant between two writes, and an
	// expired-in-a-month certificate beats no certificate now.
	if err := os.WriteFile(certPath, []byte("-----BEGIN CERTIFICATE-----\nnot a certificate\n-----END CERTIFICATE-----\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	brokenCtx, brokenCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer brokenCancel()
	third := r.mint(t, "peer-c")
	if ws, err := dialTLS(brokenCtx, r, "peer-c", third, secondPool); err != nil {
		t.Fatalf("a half-written renewal took the listener down: %v", err)
	} else {
		_ = ws.CloseNow()
	}
	if _, failures, _ := r.reloader.Stats(); failures == 0 {
		t.Fatal("the reloader did not count the failed reload; an operator needs to see it")
	}
}

// TestTLSMinVersionIsAKnobWithAFloor covers §12's `relayTLSMinVersion`: 1.2 is
// the default and the floor, and a value below it is a startup error rather than
// a listener that quietly admits an obsolete client.
func TestTLSMinVersionIsAKnobWithAFloor(t *testing.T) {
	if v, err := TLSMinVersion(""); err != nil || v != tls.VersionTLS12 {
		t.Fatalf("the default is (%v,%v), want TLS 1.2 (§12, relayTLSMinVersion)", v, err)
	}
	if v, err := TLSMinVersion("1.3"); err != nil || v != tls.VersionTLS13 {
		t.Fatalf("1.3 is (%v,%v), want TLS 1.3", v, err)
	}
	for _, bad := range []string{"1.0", "1.1", "ssl3", "yes"} {
		if _, err := TLSMinVersion(bad); err == nil {
			t.Fatalf("relayTLSMinVersion %q was accepted; the floor is 1.2", bad)
		}
	}
}

// TestBindIsLoopback pins the one predicate two different rules share:
// --insecure-no-token's bind refusal (§3.1) and B23's plain-ws carve-out. A
// WILDCARD BIND IS NOT LOOPBACK even though loopback can reach it, and that
// distinction is the whole point.
func TestBindIsLoopback(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:8795":     true,
		"localhost:8795":     true,
		"[::1]:8795":         true,
		"0.0.0.0:8795":       false,
		":8795":              false,
		"[::]:8795":          false,
		"192.168.1.227:8795": false,
	} {
		if got := bindIsLoopback(addr); got != want {
			t.Fatalf("bindIsLoopback(%q) = %v, want %v", addr, got, want)
		}
	}
}

func copyOver(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatalf("read %s: %v", from, err)
	}
	// The mtime has to move for the reloader to notice, and a filesystem with a
	// one-second timestamp granularity would otherwise make this test flaky.
	if err := os.WriteFile(to, b, 0o600); err != nil {
		t.Fatalf("write %s: %v", to, err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(to, future, future); err != nil {
		t.Fatalf("chtimes %s: %v", to, err)
	}
}

func routableAddr(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			return ip4.String()
		}
	}
	return ""
}
