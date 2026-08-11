package sidecar

// WP2's CLIENT-side tests: what a sidecar does with contract-b-m4.md §22's
// credential (B22) and TLS (B23).
//
// The relay's half of both rules is tested where the relay is. What is here is
// the half a stranger's machine has to get right on its own, and both rules are
// stated in the contract as things a client MUST NOT DO — which is the shape of
// rule a test has to look for deliberately, because nothing fails when it is
// obeyed.
//
//	§3.1: a sidecar whose credential is refused MUST NOT generate a fresh peerId,
//	MUST NOT fall back to an unauthenticated connection, MUST NOT fall back to
//	ws://, and MUST NOT try another peer's credential. It KEEPS ITS JOURNAL,
//	keeps delivering inbound entries to its own mod, and WAITS FOR A PERSON.
//
//	B23: a client MUST verify the chain, the name and the validity window using
//	its platform's trust store; a certificate it cannot verify FAILS THE
//	CONNECTION. It MUST NOT proceed, MUST NOT prompt, MUST NOT offer a flag that
//	skips verification, and MUST NOT pin as a workaround.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"multiverse/internal/archive"
	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
	"multiverse/internal/wire"
)

// TestARefusedSidecarWaitsForAPersonAndKeepsItsJournal is §3.1's *What a refused
// peer MUST NOT do* row, and it is a CUSTODY test wearing an authentication
// costume. Generating a fresh peerId to get past a 401 would strand the slot,
// the journal's destSlot and every organism addressed to it (§7.3 rule 3) — so
// the failure this asserts against is not "the peer did not join", it is "the
// peer joined as somebody else".
func TestARefusedSidecarWaitsForAPersonAndKeepsItsJournal(t *testing.T) {
	rl := startRelay(t)

	// A real, working peer first, so there is a live map to be excluded from.
	good := fastConfig(t, rl, "peer-good")
	goodSide := startSidecar(t, good)
	waitSlot(t, goodSide, 1)

	// The refused one. Its credential is wrong; everything else about it is
	// ordinary, including its mod.
	cfg := fastConfig(t, rl, "peer-refused")
	cfg.Secret = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	refused := startSidecar(t, cfg)
	mod := dialFakeMod(t, fakeModOptions{
		url: refused.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond})

	// Long enough for many ladder steps at the rig's short backoff, so a
	// fall-back would have had every chance to happen.
	time.Sleep(2 * time.Second)

	if got := refused.PeerID(); got != "peer-refused" {
		t.Fatalf("the refused sidecar is now %q; §3.1 forbids minting a fresh identity, and doing "+
			"it would strand this peer's slot and every organism addressed to it", got)
	}
	if got := refused.Slot(); got != 0 {
		t.Fatalf("the refused sidecar holds slot %d", got)
	}
	for _, res := range rl.relay.Snapshot() {
		if res.PeerID != "peer-good" {
			t.Fatalf("the map gained a reservation for %q while its credential was refused; a "+
				"401 must never become a second identity", res.PeerID)
		}
	}
	// ITS MOD IS STILL SERVED. §13 A1's "no mod connection is not an error" has a
	// mirror here: no RELAY connection is not an error either, and the sidecar
	// goes on being the mod's Contract A server throughout.
	if mod.isClosed() {
		t.Fatal("the sidecar dropped its own mod because the RELAY refused it; the two wires are " +
			"independent and the mod's world is not at fault")
	}
	// And the relay URL it is refused on is the one it was given: no downgrade,
	// no port hunt, no second guess.
	if got := refused.RelayURL(); got != rl.url() {
		t.Fatalf("the refused sidecar is now dialling %q, not the %q it was configured with", got, rl.url())
	}

	// The remedy works, and it is the one §3.1 names: a person supplies the right
	// secret. The peer comes back AS ITSELF, to its own slot.
	fixed := fastConfig(t, rl, "peer-refused")
	fixed.DataDir = cfg.DataDir
	fixed.Secret = rl.secret("peer-refused")
	_ = refused.Close()
	repaired := startSidecar(t, fixed)
	dialFakeMod(t, fakeModOptions{
		url: repaired.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond})
	waitSlotAny(t, repaired)
	if repaired.PeerID() != "peer-refused" {
		t.Fatalf("the repaired peer joined as %q", repaired.PeerID())
	}
}

// TestTheSidecarRefusesAnUnverifiableCertificate is B23's client rule, and the
// assertion is deliberately about what does NOT happen: no connection, no
// prompt, no fall-back, and no flag anywhere that would have made it succeed.
//
// The certificate here is self-signed and NOT in this machine's trust store,
// which is exactly the shape of the failure a stranger hits when a relay's
// certificate expires or its operator points a name at the wrong host.
func TestTheSidecarRefusesAnUnverifiableCertificate(t *testing.T) {
	// A TLS server that is not a relay: the point is that the sidecar never gets
	// far enough to send a byte, so what is behind the certificate is irrelevant.
	reached := make(chan struct{}, 1)
	var once sync.Once
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(reached) })
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	rl := startRelay(t) // only for a valid credential to carry
	cfg := fastConfig(t, rl, "peer-tls")
	cfg.RelayURL = "wss" + strings.TrimPrefix(srv.URL, "https") + "/contract-b/v4"
	side := startSidecar(t, cfg)

	time.Sleep(1500 * time.Millisecond)

	select {
	case <-reached:
		t.Fatal("the sidecar completed a TLS handshake against a certificate it cannot verify; " +
			"B23 says it MUST NOT proceed, MUST NOT prompt, and MUST NOT pin as a workaround")
	default:
	}
	if got := side.Slot(); got != 0 {
		t.Fatalf("the sidecar holds slot %d against a relay it could not verify", got)
	}
	// AND IT DID NOT FALL BACK TO ws://. The scheme it dials is the scheme it was
	// configured with, for as long as it keeps failing.
	if got := side.RelayURL(); !strings.HasPrefix(got, "wss://") {
		t.Fatalf("the sidecar is now dialling %q; B23 forbids falling back to ws://", got)
	}
}

// TestTheSidecarVerifiesAgainstThePlatformTrustStore is the other half, and it
// is what keeps the test above from passing against a sidecar that simply cannot
// speak TLS at all. The same kind of self-signed certificate, TRUSTED THE WAY
// B23 SAYS TRUST IS SUPPLIED — through the platform's own store rather than an
// application flag — and the connection completes.
//
// IT RUNS IN A CHILD PROCESS, and the reason is the property itself. Go loads
// the platform trust store ONCE per process, so a test that set SSL_CERT_FILE
// after any earlier TLS work would be configuring nothing. A child started with
// the variable already set is the only honest way to ask the question, and it is
// also exactly what an operator does: the trust is arranged before the process
// starts, by somebody with rights on that machine.
//
// THE ABSENCE OF A CLIENT KNOB IS THE POINT. B23 forbids a flag that skips
// verification and forbids pinning as a workaround, so the sidecar has neither,
// and the remedy for an unverifiable certificate is an operator act at one end
// or the other.
func TestTheSidecarVerifiesAgainstThePlatformTrustStore(t *testing.T) {
	if os.Getenv(wssChildEnv) == "" {
		runWSSChild(t)
		return
	}
	certPath, keyPath := os.Getenv("MULTIVERSE_TEST_TLS_CERT"), os.Getenv("MULTIVERSE_TEST_TLS_KEY")
	if certPath == "" || keyPath == "" {
		t.Fatal("the child was started without a certificate pair")
	}
	rl := startRelayWithTLS(t, certPath, keyPath)
	cfg := fastConfig(t, rl, "peer-wss")
	if !strings.HasPrefix(cfg.RelayURL, "wss://") {
		t.Fatalf("the harness handed the sidecar %q, not a wss:// URL", cfg.RelayURL)
	}
	side := startSidecar(t, cfg)
	dialFakeMod(t, fakeModOptions{
		url: side.URL(), world: newWorld(), heartbeat: 200 * time.Millisecond})
	waitSlotAny(t, side)
	if got := side.Slot(); got == 0 {
		t.Fatal("a sidecar could not join over wss:// with a certificate its platform trusts")
	}
}

// wssChildEnv marks the child process of the test above.
const wssChildEnv = "MULTIVERSE_WSS_CHILD"

// runWSSChild mints the pair, points the CHILD's platform trust store at it
// before the child starts, and re-runs this one test there.
func runWSSChild(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/etc/ssl/certs"); err != nil {
		t.Skip("no Unix trust store to extend; SSL_CERT_FILE is the platform's own mechanism here")
	}
	certPath, keyPath, certPEM := writeLocalhostPair(t)
	bundle := filepath.Join(filepath.Dir(certPath), "ca.pem")
	if err := os.WriteFile(bundle, certPEM, 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	cmd := exec.Command(os.Args[0], "-test.run", "^"+t.Name()+"$", "-test.v")
	cmd.Env = append(os.Environ(),
		wssChildEnv+"=1",
		// The platform's own trust mechanism, set BEFORE the process starts —
		// which is the only moment it can be set, and the moment an operator sets
		// it too.
		"SSL_CERT_FILE="+bundle,
		"MULTIVERSE_TEST_TLS_CERT="+certPath,
		"MULTIVERSE_TEST_TLS_KEY="+keyPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the wss:// child failed (%v):\n%s", err, out)
	}
}

// writeLocalhostPair mints a self-signed certificate for 127.0.0.1.
func writeLocalhostPair(t *testing.T) (certPath, keyPath string, certPEM []byte) {
	t.Helper()
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("certificate: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
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
	return certPath, keyPath, certPEM
}

// TestNothingSendsACredentialInAFrame is §3.1's *Where* row, asserted at the
// level the rule is written at: "Nothing credential-related appears in any
// frame, EVER — not on HANDSHAKE, which is the first place an implementer will
// reach for, because a frame is logged, copied to subscribers and forwarded."
//
// The subscriber is the reason this matters more than tidiness: every frame the
// relay routes is copied byte for byte to an authorised archive (§5.1), so a
// credential on a frame would be a credential in somebody else's ledger.
func TestNothingSendsACredentialInAFrame(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	sub := dialSubscriber(t, g.relay.url(), g.relay)
	sub.wait(contractb.TypeHandshakeAck, 10*time.Second)

	// Make traffic: a claim, a stats ping, a crossing and its answers.
	id := g.node(0).mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "a crossing", func() bool {
		return g.node(1).world.spawnCount(id) == 1
	})
	time.Sleep(500 * time.Millisecond)

	secret := g.relay.secret("peer-slot1")
	sub.mu.Lock()
	frames := append([]wire.Envelope(nil), sub.frames...)
	sub.mu.Unlock()
	for _, env := range frames {
		body := string(env.Data)
		if strings.Contains(body, secret) {
			t.Fatalf("a %s frame carries a peer's secret; §3.1 keeps the credential on the HTTP "+
				"upgrade and nowhere else, and this frame was copied to a subscriber", env.Type)
		}
		for _, forbidden := range []string{"\"secret\"", "\"credential\"", "\"authorization\""} {
			if strings.Contains(strings.ToLower(body), forbidden) {
				t.Fatalf("a %s frame carries %s", env.Type, forbidden)
			}
		}
	}
	// The credential still exists and still works, so this is not passing because
	// nothing authenticated.
	if _, _, ok := g.relay.creds.store.Verify(peercred.Join("peer-slot1", secret)); !ok {
		t.Fatal("the harness's own credential does not verify; this test proves nothing")
	}
}

// TestTheArchiveNeedsTheSubscribeGrant is B27 from the ARCHIVE's end, and it is
// the difference between a rule stated in a contract and one a binary obeys. The
// relay's side of the gate is tested where the relay is; this asserts that the
// real multiverse-archive — the one the rig runs — authenticates as a peerId
// with a grant of its own and gets nothing when the grant is the wrong one.
func TestTheArchiveNeedsTheSubscribeGrant(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})

	// A PEER credential handed to the archive. Under M4 this worked: role
	// "archive" was a self-declaration and the shared token was the only gate, so
	// anyone who could open a socket received a byte-identical copy of every
	// envelope on the map — census, mod version, save policy and exclusion list
	// per world.
	const peerID = "archive-wrong-grant"
	wrong, err := archive.New(archive.Config{
		RelayURL:        g.relay.url(),
		Secret:          g.relay.secret(peerID), // a PEER grant
		PeerID:          peerID,
		DataDir:         t.TempDir(),
		Logger:          testLogger(t),
		RelayBackoffMin: 30 * time.Millisecond,
		RelayBackoffMax: 150 * time.Millisecond,
		TickInterval:    50 * time.Millisecond,
		HTTPListen:      "127.0.0.1:0",
		MetricsInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("archive: new: %v", err)
	}
	if err := wrong.Start(context.Background()); err != nil {
		t.Fatalf("archive: start: %v", err)
	}
	t.Cleanup(func() { _ = wrong.Close() })

	// Traffic it would have recorded if it had been admitted.
	id := g.node(0).mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "a crossing", func() bool {
		return g.node(1).world.spawnCount(id) == 1
	})
	time.Sleep(1500 * time.Millisecond)

	migrations, _, err := wrong.List()
	if err != nil {
		t.Fatalf("archive: list: %v", err)
	}
	if len(migrations) != 0 {
		t.Fatalf("an archive without the SUBSCRIBE grant recorded %d migration(s); B27 makes the "+
			"grant the gate, and the boundary it guards is a fairly complete profile of every "+
			"participant's machine", len(migrations))
	}
	if observers := g.relay.relay.Snapshot(); len(observers) == 0 {
		t.Fatal("the map lost its peers while this test ran")
	}

	// And the archive that HOLDS the grant records normally, so the gate is a
	// gate rather than a wall.
	right := startArchive(t, g.relay)
	second := g.node(0).mod.migrateOut(testEntityID+1, contracta.EdgeE, 0.5)
	waitFor(t, 15*time.Second, "the authorised archive to record a crossing", func() bool {
		records, _, listErr := right.List()
		if listErr != nil {
			return false
		}
		for _, m := range records {
			if m.MigrationID == second {
				return true
			}
		}
		return false
	})
}
