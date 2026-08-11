package peercred

// The credential's own rules, contracts/contract-b-m4.md §3.1 as §22 B22 wrote
// them. The relay-level behaviour — the binding, the grant, the 401 — is tested
// where the relay is; what is here is the four things the credential itself has
// to get right, and each one has a failure mode that would be invisible from the
// wire:
//
//   - THE SPLIT IS ON THE LAST DOT, because §6.1's peerId charset admits a dot.
//     A first-dot split would silently hand the relay a truncated peerId and a
//     secret with a name glued to its front.
//   - THE STORE HOLDS A VERIFIER, so reading peers.json does not hand over every
//     peer's identity.
//   - THE STORE IS DURABLE, because a reservation never expires (D8) and a
//     credential the relay forgets is a peer that can never come back.
//   - A CREDENTIAL IS NOT REPRINTED. The relay cannot: it kept a hash.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitIsOnTheLastDot(t *testing.T) {
	secret := MintSecret()
	for _, peerID := range []string{
		"peer-lan-slot5",
		"peer.with.dots", // §6.1 allows [A-Za-z0-9._-]
		"peer.trailing.dot.x",
		"a",
	} {
		gotPeer, gotSecret, ok := Split(Join(peerID, secret))
		if !ok {
			t.Fatalf("Split(%q.<secret>) failed", peerID)
		}
		if gotPeer != peerID || gotSecret != secret {
			t.Fatalf("Split gave (%q,%q), want (%q,<secret>)", gotPeer, gotSecret, peerID)
		}
	}
	for _, bad := range []string{"", ".", "nodot", ".onlysecret", "onlypeer."} {
		if _, _, ok := Split(bad); ok {
			t.Fatalf("Split(%q) reported a usable credential", bad)
		}
	}
}

func TestSecretShapeMakesTheSplitExact(t *testing.T) {
	if err := ValidSecret(MintSecret()); err != nil {
		t.Fatalf("a freshly minted secret is invalid: %v", err)
	}
	if got := len(MintSecret()); got != 64 {
		t.Fatalf("Mint produced %d characters; §3.1 RECOMMENDS 32 random bytes hex-encoded", got)
	}
	// The no-dot rule is not decoration: it is what makes the last-dot split
	// EXACT rather than merely usual.
	if err := ValidSecret("0123456789abcdef.0123456789abcdef"); err == nil {
		t.Fatal("a secret containing '.' was accepted; the credential separator would be ambiguous")
	}
	if err := ValidSecret("short"); err == nil {
		t.Fatal("a 5-byte secret was accepted; §3.1 wants 32 to 256")
	}
	if err := ValidSecret(strings.Repeat("a", MaxSecretLen+1)); err == nil {
		t.Fatal("an over-long secret was accepted")
	}
	if err := ValidSecret("0123456789abcdef0123456789abcde "); err == nil {
		t.Fatal("a secret with a space was accepted; §3.1 wants printable ASCII with no spaces")
	}
}

func TestStoreKeepsAVerifierAndNotTheSecret(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	secret, err := store.Mint("peer-a", GrantPeer)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, StoreFileName))
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("the store contains the secret; §3.1 requires a salted hash from which the " +
			"join string cannot be recovered")
	}
	// Two peers with the same secret must not produce the same verifier, or the
	// salt is not doing its job and one leaked file compares peers against each
	// other.
	if _, err := store.Mint("peer-b", GrantPeer); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if _, _, ok := store.Verify(Join("peer-b", secret)); ok {
		t.Fatal("peer-a's secret verified as peer-b; the credential is not bound to its peerId")
	}
}

func TestVerifyRefusesEveryWrongShape(t *testing.T) {
	store, err := OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	secret, err := store.Mint("peer-a", GrantPeer)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	peerID, grant, ok := store.Verify(Join("peer-a", secret))
	if !ok || peerID != "peer-a" || grant != GrantPeer {
		t.Fatalf("a valid credential verified as (%q,%q,%v)", peerID, grant, ok)
	}
	for name, credential := range map[string]string{
		"empty":               "",
		"no separator":        "peer-a",
		"wrong secret":        Join("peer-a", MintSecret()),
		"unknown peer":        Join("peer-b", secret),
		"empty secret half":   "peer-a.",
		"empty peer half":     "." + secret,
		"secret as the whole": secret,
	} {
		if _, _, ok := store.Verify(credential); ok {
			t.Fatalf("%s verified", name)
		}
	}
}

func TestStoreSurvivesAReopen(t *testing.T) {
	dir := t.TempDir()
	first, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	peerSecret, err := first.Mint("peer-a", GrantPeer)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	subSecret, err := first.Mint("archive-main", GrantSubscribe)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	second, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, grant, ok := second.Verify(Join("peer-a", peerSecret)); !ok || grant != GrantPeer {
		t.Fatal("the peer credential did not survive a reopen; its slot never expires and it " +
			"could never come back to it")
	}
	if _, grant, ok := second.Verify(Join("archive-main", subSecret)); !ok || grant != GrantSubscribe {
		t.Fatal("the subscribe grant did not survive a reopen")
	}
	if second.Len() != 2 {
		t.Fatalf("the reopened store holds %d credentials, want 2", second.Len())
	}
}

func TestACredentialIsNeverReprinted(t *testing.T) {
	store, err := OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if _, err := store.Mint("peer-a", GrantPeer); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	// Minting over an existing credential is refused, not silently done: the
	// store kept a hash, so a second mint could only invent a NEW secret and
	// strand whichever copy the peer still holds. Recovery is a slot handover
	// with a printed consequence (§3.1, §7.5).
	if _, err := store.Mint("peer-a", GrantPeer); err == nil {
		t.Fatal("Mint silently replaced an existing credential")
	}
	// Remint is the deliberate path, and it keeps the grant.
	fresh, err := store.Remint("peer-a")
	if err != nil {
		t.Fatalf("Remint: %v", err)
	}
	if _, grant, ok := store.Verify(Join("peer-a", fresh)); !ok || grant != GrantPeer {
		t.Fatal("the reminted credential does not verify with its original grant")
	}
	if _, err := store.Remint("peer-nobody"); err == nil {
		t.Fatal("Remint invented a credential for an unknown peer")
	}
	if err := store.Forget("peer-a"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, _, ok := store.Verify(Join("peer-a", fresh)); ok {
		t.Fatal("a forgotten credential still verifies; the identity that gave up a slot could " +
			"go on authenticating against a reservation it no longer holds")
	}
}

func TestGrantsAreDisjoint(t *testing.T) {
	for _, c := range []struct {
		grant, role string
		want        bool
	}{
		{GrantPeer, "peer", true},
		{GrantPeer, "archive", false},
		{GrantSubscribe, "archive", true},
		{GrantSubscribe, "peer", false},
		{GrantAdmin, "peer", false},
		{GrantAdmin, "archive", false},
		{"", "peer", false},
	} {
		if got := GrantAllowsRole(c.grant, c.role); got != c.want {
			t.Fatalf("GrantAllowsRole(%q,%q) = %v, want %v — a compromised subscriber must not "+
				"become a peer and a compromised peer must not read the map's whole traffic",
				c.grant, c.role, got, c.want)
		}
	}
	store, err := OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if _, err := store.Mint("peer-a", "operator"); err == nil {
		t.Fatal("a grant outside the three was accepted")
	}
}

func TestLoadSecretReadsTheFirstLineOnly(t *testing.T) {
	dir := t.TempDir()
	secret := MintSecret()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte(secret+"  \n# a comment somebody added\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := LoadSecret(path)
	if err != nil {
		t.Fatalf("LoadSecret: %v", err)
	}
	if got != secret {
		t.Fatalf("LoadSecret gave %q, want the first line with trailing whitespace stripped", got)
	}

	// The whole credential in the file is the mistake a person actually makes,
	// and the error has to say what to do about it rather than "invalid".
	full := filepath.Join(dir, "full")
	if err := os.WriteFile(full, []byte(Join("peer-a", secret)+"\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err = LoadSecret(full)
	if err == nil {
		t.Fatal("a file holding the whole credential was accepted as a secret")
	}
	if !strings.Contains(err.Error(), "SECRET half only") {
		t.Fatalf("the error is %q; it must name the fix", err)
	}
}

func TestJoinStringSaysWhatTheGrantGrants(t *testing.T) {
	// B27 asks for the subscribe grant to be issued DELIBERATELY, and D24's
	// participant announcement has to be able to say what a participant is
	// agreeing to. A join string that printed a secret and nothing else would
	// make that a document nobody wrote.
	report := JoinString{
		RelayURL: "wss://relay.example.net/contract-b/v4",
		PeerID:   "archive-main", Secret: MintSecret(), Grant: GrantSubscribe,
	}.Report()
	for _, want := range []string{"species census", "save policy", "exclusion list", "PEER_STATUS"} {
		if !strings.Contains(report, want) {
			t.Fatalf("a subscribe join string does not mention %q; granting it must be a decision "+
				"an operator can make with the boundary in front of them (§5.1, B27)", want)
		}
	}
	peerReport := JoinString{
		RelayURL: "wss://relay.example.net/contract-b/v4",
		PeerID:   "peer-lan-slot5", Secret: MintSecret(), Grant: GrantPeer,
	}.Report()
	for _, want := range []string{"--credential-file", "handover", "cannot print this again"} {
		if !strings.Contains(peerReport, want) {
			t.Fatalf("a peer join string does not mention %q; the honest price of NO ACCOUNTS is "+
				"that losing it costs a handover, and the string is where a person meets that", want)
		}
	}
}
