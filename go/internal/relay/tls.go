package relay

// TLS at the relay's front door — contracts/contract-b-m4.md §3 and §22, B23.
//
// §13 item 1 named the reason TLS and the per-peer credential ship together and
// the reason is worth carrying at the top of the file that implements one half:
// SPLITTING THEM PRODUCES A HALF-SECURED RELAY THAT READS AS SECURED. TLS
// without credentials encrypts a wire on which any participant can impersonate
// any other; credentials without TLS put the credential on the wire in the
// clear. Neither half is a milestone; the pair is (m5_considerations.md, DQ1,
// Risk 1).
//
// WHERE IT TERMINATES IS AN OPERATIONAL CHOICE AND NOT THIS FILE'S (B23): the
// relay itself, or a fronting proxy that terminates for it. What the contract
// specifies is the wire-visible behaviour, and it is identical either way — which
// is why the only server-side rules here are the listener, the certificate, and
// a reload that does not drop sessions. The name, the ACME client and the
// renewal are WP3's.

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

// TLSMinVersion maps `relayTLSMinVersion` (§12) onto crypto/tls. 1.2 is the
// default and the floor: this contract has no reason to admit anything older,
// and a knob that can be turned below the floor is a knob that will be.
func TLSMinVersion(v string) (uint16, error) {
	switch v {
	case "", "1.2":
		return tls.VersionTLS12, nil
	case "1.3":
		return tls.VersionTLS13, nil
	}
	return 0, fmt.Errorf("relay: relayTLSMinVersion %q is not 1.2 or 1.3", v)
}

// CertReloader holds the certificate the LISTENER presents and re-reads it from
// disk when the files change. It is B23's rotation row, which is a rule about
// what a rotation costs a CONNECTED PEER and the answer is NOTHING:
//
//	"A rotation replaces the certificate the listener presents to the NEXT
//	 handshake; an established TLS session is unaffected and its WebSocket stays
//	 up. A peer sees a rotation only if the rotation restarts the process — and
//	 then it sees an ordinary disconnect, reconnects on the backoff ladder, and
//	 rejoins with reason: "reclaimed". THE RELAY MUST BE ABLE TO LOAD A RENEWED
//	 CERTIFICATE WITHOUT DROPPING SESSIONS."
//
// The mechanism is deliberately the smallest one that satisfies that sentence:
// GetCertificate is called once per TLS handshake, so a stat of two files there
// is free at any rate this relay will ever see, and a renewed pair is picked up
// by the next handshake with no signal, no restart and no reload command. An
// ACME client that renews in place needs to do nothing at all; one that cannot
// renew in place makes the rotation a ROUTINE RESTART, and D24's restart policy
// is what tells participants what that looks like (DQ2).
//
// A renewal that lands HALF-WRITTEN is the case worth naming: the cert file is
// new and the key file is not, for the instant between two writes. Load fails,
// and this keeps serving THE CERTIFICATE IT ALREADY HAS rather than failing the
// handshake — an expired-in-a-month certificate beats no certificate now, and
// the next handshake tries again.
type CertReloader struct {
	certFile string
	keyFile  string

	mu       sync.Mutex
	cert     *tls.Certificate
	certMod  time.Time
	keyMod   time.Time
	lastErr  error
	reloads  int
	failures int
}

// NewCertReloader loads the pair once, so a relay with an unreadable certificate
// fails at startup rather than at a stranger's first dial.
func NewCertReloader(certFile, keyFile string) (*CertReloader, error) {
	if certFile == "" || keyFile == "" {
		return nil, errors.New("relay: TLS needs both --tls-cert and --tls-key")
	}
	r := &CertReloader{certFile: certFile, keyFile: keyFile}
	if err := r.reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// GetCertificate is the tls.Config hook. It re-reads the pair when either file's
// modification time has moved and otherwise answers from the cached copy.
func (r *CertReloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	certMod, keyMod, err := r.mtimes()
	// ANY CHANGE, not a later one. A renewal that preserves or restores a
	// timestamp — a copy with -p, a restore from backup, a clock that stepped
	// back — is still a renewal, and a reloader that only ever moved forwards
	// would serve the old certificate until the process restarted.
	if err == nil && (!certMod.Equal(r.certMod) || !keyMod.Equal(r.keyMod)) {
		if reloadErr := r.reloadLocked(certMod, keyMod); reloadErr != nil {
			// Keep serving what we have. See the type comment: a half-written
			// renewal must not take the listener down.
			r.failures++
			r.lastErr = reloadErr
		} else {
			r.reloads++
			r.lastErr = nil
		}
	}
	if r.cert == nil {
		return nil, r.lastErr
	}
	return r.cert, nil
}

// Stats reports how many times the pair was reloaded and how many reloads
// failed, for an operator log line and for the rotation test.
func (r *CertReloader) Stats() (reloads, failures int, lastErr error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reloads, r.failures, r.lastErr
}

func (r *CertReloader) reload() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	certMod, keyMod, err := r.mtimes()
	if err != nil {
		return err
	}
	return r.reloadLocked(certMod, keyMod)
}

func (r *CertReloader) reloadLocked(certMod, keyMod time.Time) error {
	cert, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return fmt.Errorf("relay: load TLS key pair: %w", err)
	}
	r.cert = &cert
	r.certMod, r.keyMod = certMod, keyMod
	return nil
}

func (r *CertReloader) mtimes() (time.Time, time.Time, error) {
	cs, err := os.Stat(r.certFile)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	ks, err := os.Stat(r.keyFile)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return cs.ModTime(), ks.ModTime(), nil
}

// TLSListener wraps ln so the relay terminates its own TLS. A relay behind a
// fronting proxy does not call this: the proxy owns the certificate and reaches
// the relay over loopback, which allowPlainUpgrade admits for exactly that
// reason.
func TLSListener(ln net.Listener, r *CertReloader, minVersion uint16) net.Listener {
	return tls.NewListener(ln, &tls.Config{
		GetCertificate: r.GetCertificate,
		MinVersion:     minVersion,
	})
}
