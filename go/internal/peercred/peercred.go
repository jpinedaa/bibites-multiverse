// Package peercred implements the per-peer credential of
// contracts/contract-b-m4.md §3.1, as §22 B22 rewrote it for `contract-b/4.0`.
//
// It replaces the shared LAN token outright, and that replacement is the whole
// reason contract-b/4 is a major (§22, B32). The token authenticated a NETWORK:
// any holder could present any `peerId`, including one that already held a slot,
// and §3.2's 4006 rule would then evict the legitimate peer with one frame. The
// credential authenticates an IDENTITY, because it is BOUND to the `peerId`:
//
//	Authorization: Bearer <peerId>.<secret>
//
// split on the LAST ".", because §6.1's peerId charset admits a dot and the
// secret's does not. The relay verifies the secret against the verifier it holds
// for that peerId and then holds the name until HANDSHAKE arrives; a frame that
// claims a different peerId is refused there (§3.1, and relay.handshake).
//
// THE STORE HOLDS A VERIFIER, NEVER A SECRET (§3.1, B22). A salted digest, from
// which the join string cannot be recovered: a relay whose peers.json is read
// must not thereby hand over every peer's identity. The digest is a single
// SHA-256 over salt and secret rather than a password KDF, and that is a
// deliberate choice with a reason: the secret is 32 bytes minted by
// crypto/rand — 256 bits of entropy — so there is no dictionary to run and no
// work factor worth paying. A KDF here would buy nothing and cost a verification
// on every upgrade.
//
// THERE IS NO FLAG THAT TAKES A SECRET LITERALLY, on any binary. §3.1 states it
// as a rule and the reason is the process listing.
package peercred

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"multiverse/internal/fsutil"
)

// The three grants of §22, B22. THEY ARE DISJOINT: one credential carries
// exactly one of them, a credential that holds one does not hold another, and
// the relay decides which `role` a connection may take from the grant alone
// (§5.1 B27, §7.5 B28). That disjointness is what stops a compromised
// subscriber claiming a slot and a compromised peer reading the map's whole
// traffic.
const (
	// GrantPeer owns a world and a slot: role "peer" (§6.1).
	GrantPeer = "peer"
	// GrantSubscribe is the archive's: role "archive", read-only (§5.1, B27).
	GrantSubscribe = "subscribe"
	// GrantAdmin reaches the authenticated admin path of B28 and NOTHING on this
	// wire. A connection presenting it is refused both roles, which is what
	// "disjoint" means when the third surface does not exist yet: B28 is WP4, and
	// until it lands an admin credential authenticates nothing at all.
	GrantAdmin = "admin"
)

// SecretEnvVar is the client-side source of the secret half. The other source is
// --credential-file; there is no third and there is no literal flag (§3.1).
const SecretEnvVar = "MULTIVERSE_PEER_SECRET"

// StoreFileName is `credentialVerifierStore` (§12): <data-dir>/peers.json. It is
// the THIRD file an operator must back up, beside ring.json and the archive's
// durable set (D24, DQ2).
const StoreFileName = "peers.json"

// Secret shape (§3.1): 32 to 256 bytes of printable ASCII containing no ".",
// and the RECOMMENDED value is 32 random bytes hex-encoded, which is what Mint
// produces.
const (
	MinSecretLen = 32
	MaxSecretLen = 256
	mintBytes    = 32
)

var (
	// ErrNoSecret means neither MULTIVERSE_PEER_SECRET nor --credential-file
	// supplied one.
	ErrNoSecret = errors.New("peercred: no peer secret configured")
	// ErrUnknownPeer and ErrExists are the two store-level answers an operator
	// command has to tell apart.
	ErrUnknownPeer = errors.New("peercred: no credential for this peerId")
	ErrExists      = errors.New("peercred: this peerId already holds a credential")
)

// ValidGrant reports whether g is one of the three.
func ValidGrant(g string) bool {
	return g == GrantPeer || g == GrantSubscribe || g == GrantAdmin
}

// GrantAllowsRole is the §6.1 / B27 rule in one place: the credential's grant
// decides which `role` values are legal on this connection, and a role the grant
// does not carry is refused with 4003.
func GrantAllowsRole(grant, role string) bool {
	switch role {
	case "peer":
		return grant == GrantPeer
	case "archive":
		return grant == GrantSubscribe
	}
	return false
}

// Split takes a credential apart at its LAST ".", which is the only unambiguous
// split: §6.1 allows a dot inside a peerId and ValidSecret forbids one inside a
// secret.
func Split(credential string) (peerID, secret string, ok bool) {
	i := strings.LastIndex(credential, ".")
	if i <= 0 || i == len(credential)-1 {
		return "", "", false
	}
	return credential[:i], credential[i+1:], true
}

// Join is the inverse of Split.
func Join(peerID, secret string) string { return peerID + "." + secret }

// Header is the value a client puts on its upgrade request. Nothing
// credential-related ever appears in a frame (§3.1).
func Header(peerID, secret string) string { return "Bearer " + Join(peerID, secret) }

// FromRequest pulls the credential out of an HTTP upgrade request.
func FromRequest(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// ValidSecret enforces §3.1's shape. The no-dot rule is what makes Split's
// last-dot rule exact rather than merely usual.
func ValidSecret(secret string) error {
	if secret == "" {
		return ErrNoSecret
	}
	if len(secret) < MinSecretLen || len(secret) > MaxSecretLen {
		return fmt.Errorf("peercred: secret is %d bytes, want %d to %d",
			len(secret), MinSecretLen, MaxSecretLen)
	}
	for i := 0; i < len(secret); i++ {
		c := secret[i]
		if c < 0x21 || c > 0x7e {
			return errors.New("peercred: secret must be printable ASCII with no spaces")
		}
		if c == '.' {
			return errors.New("peercred: secret must not contain '.', which is the credential separator " +
				"(a credential file holds the SECRET half only; the peerId comes from <data-dir>/peer-id)")
		}
	}
	return nil
}

// MintSecret is §3.1's RECOMMENDED value: 32 random bytes, hex-encoded.
func MintSecret() string {
	var b [mintBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("peercred: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// LoadSecret reads the client's own secret: --credential-file first, then
// MULTIVERSE_PEER_SECRET. The file's FIRST LINE is used with trailing
// whitespace stripped (§3.1).
func LoadSecret(credentialFile string) (string, error) {
	if credentialFile != "" {
		b, err := os.ReadFile(credentialFile)
		if err != nil {
			return "", fmt.Errorf("peercred: read %s: %w", credentialFile, err)
		}
		line := string(b)
		if i := strings.IndexAny(line, "\r\n"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimRight(line, " \t")
		if err := ValidSecret(line); err != nil {
			return "", err
		}
		return line, nil
	}
	if v := strings.TrimSpace(os.Getenv(SecretEnvVar)); v != "" {
		if err := ValidSecret(v); err != nil {
			return "", err
		}
		return v, nil
	}
	return "", ErrNoSecret
}

// ---------------------------------------------------------------- the store

// record is one peer's verifier. THE SECRET IS NOT HERE and cannot be derived
// from what is.
type record struct {
	PeerID    string `json:"peerId"`
	Grant     string `json:"grant"`
	Salt      string `json:"salt"`
	Verifier  string `json:"verifier"`
	CreatedAt int64  `json:"createdAt"`
}

type storeFile struct {
	Version int      `json:"version"`
	Peers   []record `json:"peers"`
}

// Store is `credentialVerifierStore` (§12): the per-peer verifiers and their
// grants, durable across a relay restart. It is not safe for concurrent use by
// itself; the relay holds it behind its own lock and only ever reads it on the
// serving path.
//
// DURABILITY IS THE POINT AND IT IS NOT INCIDENTAL. A credential the relay
// forgets on restart is a peer that can never come back, against a reservation
// that never expires (D8) and a journal full of entries addressed to its slot.
// The file is written the way ring.json is: rename into place, then fsync the
// directory (fsutil.SyncDir), so a crash leaves either the old set or the new
// one and never a half of either.
type Store struct {
	// mu makes the store safe for a mint that happens while the relay is
	// serving, which B28's admin path will do and the handover command already
	// wants. Verify takes the read side, so an upgrade never waits on another
	// upgrade.
	mu    sync.RWMutex
	path  string
	peers map[string]record

	// decoySalt makes an unknown peerId cost the same work as a known one
	// (§3.1's constant-time row): the relay hashes against this instead of
	// returning early, so response timing does not enumerate the map's
	// membership.
	decoySalt []byte
}

// OpenStore reads <dir>/peers.json, or returns an empty in-memory store when
// dir is empty — which is what a test rig wants and what a hosted relay must
// never do.
func OpenStore(dir string) (*Store, error) {
	s := &Store{peers: map[string]record{}, decoySalt: newSalt()}
	if dir == "" {
		return s, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s.path = filepath.Join(dir, StoreFileName)
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var f storeFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("peercred: %s is unreadable: %w", s.path, err)
	}
	for _, rec := range f.Peers {
		if rec.PeerID == "" {
			continue
		}
		s.peers[rec.PeerID] = rec
	}
	return s, nil
}

// Path is the store's file, or "" for an in-memory one.
func (s *Store) Path() string { return s.path }

// Len is how many credentials exist. A relay with none can admit nobody, which
// is why New refuses to start on it (§3.1).
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.peers)
}

// GrantOf reports the grant held for peerID.
func (s *Store) GrantOf(peerID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.peers[peerID]
	if !ok {
		return "", false
	}
	return rec.Grant, true
}

// PeerIDs lists every peerId with a credential, sorted. Secrets are not
// recoverable, so this leaks nothing a status page does not already publish.
func (s *Store) PeerIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.peerIDsLocked()
}

func (s *Store) peerIDsLocked() []string {
	out := make([]string, 0, len(s.peers))
	for id := range s.peers {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Mint creates a credential for peerID with grant and returns the SECRET, once.
// The store keeps only the verifier, so this is the only moment the secret
// exists anywhere but the peer's own machine — which is why §3.1 says the join
// string is printed once (B22).
//
// It refuses to overwrite: rebinding an identity is §7.5's handover, a
// deliberate act with a printed consequence, not a side effect of a repeated
// command.
func (s *Store) Mint(peerID, grant string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mintLocked(peerID, grant)
}

func (s *Store) mintLocked(peerID, grant string) (string, error) {
	if peerID == "" {
		return "", errors.New("peercred: mint needs a peerId")
	}
	if !ValidGrant(grant) {
		return "", fmt.Errorf("peercred: grant %q is not peer/subscribe/admin", grant)
	}
	if _, ok := s.peers[peerID]; ok {
		return "", fmt.Errorf("%w: %s", ErrExists, peerID)
	}
	secret := MintSecret()
	salt := newSalt()
	s.peers[peerID] = record{
		PeerID:    peerID,
		Grant:     grant,
		Salt:      hex.EncodeToString(salt),
		Verifier:  hex.EncodeToString(digest(salt, secret)),
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := s.saveLocked(); err != nil {
		delete(s.peers, peerID)
		return "", err
	}
	return secret, nil
}

// Remint replaces an existing credential with a fresh secret, keeping the grant.
// It is the credential half of §7.5's handover: the new peerId gets a freshly
// minted credential, and the old identity's is dropped by Forget.
func (s *Store) Remint(peerID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.peers[peerID]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownPeer, peerID)
	}
	grant := rec.Grant
	delete(s.peers, peerID)
	secret, err := s.mintLocked(peerID, grant)
	if err != nil {
		s.peers[peerID] = rec
		return "", err
	}
	return secret, nil
}

// Forget drops a credential. The relay uses it on a handover, so the identity
// that gave up the slot cannot go on authenticating against a reservation it no
// longer holds.
func (s *Store) Forget(peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.peers[peerID]
	if !ok {
		return nil
	}
	delete(s.peers, peerID)
	if err := s.saveLocked(); err != nil {
		s.peers[peerID] = rec
		return err
	}
	return nil
}

// Verify is the relay's whole authentication step. It returns the peerId the
// credential NAMES and its grant; the binding to HANDSHAKE.peerId is checked one
// layer up, where the frame is (§3.1, §6.1).
//
// EVERY PATH DOES THE SAME WORK. A malformed credential, an unknown peerId and a
// wrong secret all reach the same digest and the same constant-time compare, so
// the answer's timing says nothing about which peerIds exist.
func (s *Store) Verify(credential string) (peerID, grant string, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	name, secret, split := Split(credential)
	rec, known := s.peers[name]
	salt := s.decoySalt
	want := digest(s.decoySalt, "")
	if known {
		if b, err := hex.DecodeString(rec.Salt); err == nil {
			salt = b
		}
		if b, err := hex.DecodeString(rec.Verifier); err == nil {
			want = b
		}
	}
	got := digest(salt, secret)
	match := subtle.ConstantTimeCompare(got, want) == 1
	if !split || !known || !match {
		return "", "", false
	}
	return name, rec.Grant, true
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	f := storeFile{Version: 1}
	for _, id := range s.peerIDsLocked() {
		f.Peers = append(f.Peers, s.peers[id])
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	dir := filepath.Dir(s.path)
	tmp := s.path + ".tmp"
	// 0600: the file holds no recoverable secret, and it still names every
	// participant and their grant. It is not world-readable for the same reason
	// a token file never was.
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return fsutil.SyncDir(dir)
}

func newSalt() []byte {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("peercred: crypto/rand failed: " + err.Error())
	}
	return b[:]
}

func digest(salt []byte, secret string) []byte {
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte{0})
	h.Write([]byte(secret))
	return h.Sum(nil)
}

// ---------------------------------------------------------------- join string

// JoinString is what an operator hands over out of band (§3.1, §22 B22): the
// relay URL, the peerId and the secret, printed ONCE on the relay's own console.
// No accounts, no email, no password reset — and no recovery, which §3.1 states
// as the honest price: a stranger who loses this loses that world's identity
// until an operator hands the slot over by name.
type JoinString struct {
	RelayURL string
	PeerID   string
	Secret   string
	Grant    string
}

// Line is the one-line form, for copying into a message. It carries exactly the
// three things §3.1 names and a version tag so a future format is legible.
func (j JoinString) Line() string {
	return fmt.Sprintf("multiverse-join/1 %s %s", j.RelayURL, Join(j.PeerID, j.Secret))
}

// Report is what the console prints. It is deliberately a procedure rather than
// a blob: the receiving operator has to put the secret in a file and pass two
// flags, and a join string that does not say so is a support conversation.
func (j JoinString) Report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nJOIN STRING for %s — printed ONCE. Hand it over out of band.\n", j.PeerID)
	fmt.Fprintf(&b, "  relay       %s\n", j.RelayURL)
	fmt.Fprintf(&b, "  peerId      %s\n", j.PeerID)
	fmt.Fprintf(&b, "  secret      %s\n", j.Secret)
	fmt.Fprintf(&b, "  grant       %s\n", j.Grant)
	fmt.Fprintf(&b, "  one line    %s\n", j.Line())
	fmt.Fprintf(&b, "\n  This relay keeps a VERIFIER, not the secret: it cannot print this again.\n")
	fmt.Fprintf(&b, "  If it is lost, the only recovery is an operator slot handover by name\n")
	fmt.Fprintf(&b, "  (multiverse-relay --handover-slot <n>=<newPeerId>), which mints a fresh one.\n")
	switch j.Grant {
	case GrantPeer:
		fmt.Fprintf(&b, "\n  On %s's own machine:\n", j.PeerID)
		fmt.Fprintf(&b, "      umask 077; printf '%%s\\n' '<secret>' > ~/.multiverse-peer-secret\n")
		fmt.Fprintf(&b, "      multiverse-sidecar --relay %s \\\n", j.RelayURL)
		fmt.Fprintf(&b, "          --peer-id %s --credential-file ~/.multiverse-peer-secret\n", j.PeerID)
	case GrantSubscribe:
		fmt.Fprintf(&b, "\n  On the machine that runs the archive:\n")
		fmt.Fprintf(&b, "      umask 077; printf '%%s\\n' '<secret>' > ~/.multiverse-archive-secret\n")
		fmt.Fprintf(&b, "      multiverse-archive --relay %s \\\n", j.RelayURL)
		fmt.Fprintf(&b, "          --peer-id %s --credential-file ~/.multiverse-archive-secret\n", j.PeerID)
		fmt.Fprintf(&b, "\n  WHAT THIS GRANT LETS ITS HOLDER SEE (§5.1, B27), because granting it is a\n")
		fmt.Fprintf(&b, "  decision rather than a default. Its holder receives:\n")
		fmt.Fprintf(&b, "    - every MIGRATION_PAYLOAD, MIGRATION_ACK and MIGRATION_NACK the relay\n")
		fmt.Fprintf(&b, "      routes or generates, byte for byte;\n")
		fmt.Fprintf(&b, "    - every PEER_STATUS, and with it, per world: the species census,\n")
		fmt.Fprintf(&b, "      the mod version, the contract-a version, the save policy,\n")
		fmt.Fprintf(&b, "      the exclusion list, populations, queue depths and speeds.\n")
		fmt.Fprintf(&b, "  That is a fairly complete profile of every participant's machine. It is what\n")
		fmt.Fprintf(&b, "  a participant must be told before they join, and it is NOT a privileged view:\n")
		fmt.Fprintf(&b, "  every field here is one the relay already broadcasts to every peer.\n")
	case GrantAdmin:
		fmt.Fprintf(&b, "\n  This grant reaches the authenticated admin path and NOTHING on the Contract B\n")
		fmt.Fprintf(&b, "  wire: a connection presenting it is refused role \"peer\" and role \"archive\"\n")
		fmt.Fprintf(&b, "  alike, because the three grants are disjoint (§22, B22).\n")
	}
	return b.String()
}
