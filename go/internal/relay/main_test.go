package relay

// The operator's console: issuance, and the two rules that guard it.
//
// contract-b-m4.md §3.1 calls the issuance flow "the smallest design that
// works", and the whole of it is here — THE RELAY MINTS THE SECRET AT FIRST
// CLAIM AND PRINTS A JOIN STRING, once, for an operator to hand over out of
// band. No accounts, no email, no password reset. What that buys is a milestone
// with no account system in it; what it costs is stated rather than discovered,
// and the cost is that a stranger who loses their join string loses that world's
// identity until an operator hands the slot over by name.
//
// These tests drive Main the way an operator does, because the flow IS the
// interface: a mint that printed a secret into a log, or a reservation that
// quietly did not mint one, would be a working credential system nobody could
// join.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiverse/internal/peercred"
)

func runMain(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Main(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// TestMintCredentialPrintsAJoinStringOnce is §3.1's *Issuance* row end to end.
func TestMintCredentialPrintsAJoinStringOnce(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := runMain(t,
		"--data-dir", dir,
		"--mint-credential", "peer-lan-slot5",
		"--listen", "0.0.0.0:8795",
		"--tls-cert", "/nonexistent-but-never-loaded-for-a-mint",
		"--tls-key", "/nonexistent-but-never-loaded-for-a-mint",
	)
	if code != 0 {
		t.Fatalf("--mint-credential exited %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "JOIN STRING for peer-lan-slot5") {
		t.Fatalf("no join string was printed:\n%s", stdout)
	}
	// The URL a join string names carries the SCHEME the relay will actually
	// serve, and leaves the host a legible placeholder on a wildcard bind —
	// because a relay genuinely does not know which of its addresses a stranger
	// can reach.
	if !strings.Contains(stdout, "wss://<relay-host>:8795/contract-b/v4") {
		t.Fatalf("the join string does not name the wss:// URL a peer must dial:\n%s", stdout)
	}

	secret := secretFrom(t, stdout)
	store, err := peercred.OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if _, grant, ok := store.Verify(peercred.Join("peer-lan-slot5", secret)); !ok || grant != peercred.GrantPeer {
		t.Fatal("the printed join string does not verify against the store it was minted into")
	}
	// The secret is NOT in the store's own file (§3.1), so an operator who backs
	// peers.json up has not backed up every peer's identity.
	raw, err := os.ReadFile(filepath.Join(dir, peercred.StoreFileName))
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("the store holds the secret in recoverable form")
	}

	// A SECOND mint for the same peerId is refused, and the refusal names the
	// recovery path rather than silently replacing a credential the peer may
	// still be holding.
	code, _, stderr = runMain(t, "--data-dir", dir, "--mint-credential", "peer-lan-slot5")
	if code == 0 {
		t.Fatal("a second mint silently replaced an existing credential")
	}
	if !strings.Contains(stderr, "handover-slot") {
		t.Fatalf("the refusal does not name the recovery path:\n%s", stderr)
	}
}

// TestMintIssuesTheSubscribeGrantDeliberately is B27's *It is issued
// deliberately* row: the subscribe grant is issued by the relay OPERATOR, at the
// same console that mints a join string. NO WIRE MESSAGE ASKS FOR THE GRANT AND
// NONE CONFERS IT, so a public map has exactly as many subscribers as its
// operator decided to have.
func TestMintIssuesTheSubscribeGrantDeliberately(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := runMain(t, "--data-dir", dir,
		"--mint-credential", "archive-main", "--grant", "subscribe")
	if code != 0 {
		t.Fatalf("exited %d\n%s\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "WHAT THIS GRANT LETS ITS HOLDER SEE") {
		t.Fatalf("a subscribe join string does not state the visibility boundary, so granting it "+
			"is a default rather than a decision (B27):\n%s", stdout)
	}
	store, err := peercred.OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if grant, _ := store.GrantOf("archive-main"); grant != peercred.GrantSubscribe {
		t.Fatalf("the minted grant is %q, want subscribe", grant)
	}
	if code, _, _ := runMain(t, "--data-dir", dir, "--mint-credential", "x", "--grant", "operator"); code == 0 {
		t.Fatal("a grant outside the three was accepted")
	}
}

// TestReserveSlotMintsTheCredentialWithTheReservation is what "at first claim"
// means in practice. A peer with no credential cannot dial, so there is no
// trust-on-first-use path and no wire moment at which a credential could be
// born: the operator's reservation IS the first claim, and it is where the join
// string comes from.
func TestReserveSlotMintsTheCredentialWithTheReservation(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := runMain(t, "--data-dir", dir,
		"--reserve-slot", "peer-main-slot1@0,0",
		"--reserve-slot", "peer-main-slot2@1,0")
	if code != 0 {
		t.Fatalf("--reserve-slot exited %d\n%s\n%s", code, stdout, stderr)
	}
	for _, id := range []string{"peer-main-slot1", "peer-main-slot2"} {
		if !strings.Contains(stdout, "JOIN STRING for "+id) {
			t.Fatalf("reserving a slot for %s printed no join string; the peer it reserved for "+
				"could never join it:\n%s", id, stdout)
		}
	}
	store, err := peercred.OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if store.Len() != 2 {
		t.Fatalf("the store holds %d credentials after two reservations", store.Len())
	}

	// Re-running the same reservation must NOT remint: the reservation is
	// idempotent (§7.2 rule 1) and the credential is not reprintable, so a
	// repeated command must leave the peer's own copy working.
	code, stdout, _ = runMain(t, "--data-dir", dir, "--reserve-slot", "peer-main-slot1@0,0")
	if code != 0 {
		t.Fatalf("a repeated reservation exited %d", code)
	}
	if strings.Contains(stdout, "JOIN STRING for peer-main-slot1") {
		t.Fatal("a repeated reservation reminted a credential, stranding whichever copy the peer " +
			"already holds")
	}
}

// TestHandoverMintsAFreshCredentialAndDropsTheOld is §3.1's *Recovery is a slot
// handover* row, which is THE ONLY RECOVERY THERE IS. It rebinds the reservation
// to a new peerId with a freshly minted credential, and it drops the old
// identity's — so an identity that gave up a slot cannot go on authenticating
// against a reservation it no longer holds.
func TestHandoverMintsAFreshCredentialAndDropsTheOld(t *testing.T) {
	dir := t.TempDir()
	if code, _, stderr := runMain(t, "--data-dir", dir, "--reserve-slot", "peer-lost-its-string"); code != 0 {
		t.Fatalf("reserve exited %d: %s", code, stderr)
	}
	code, stdout, stderr := runMain(t, "--data-dir", dir,
		"--handover-slot", "1=peer-with-a-new-string", "--yes")
	if code != 0 {
		t.Fatalf("--handover-slot exited %d\n%s\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "JOIN STRING for peer-with-a-new-string") {
		t.Fatalf("a handover printed no join string for the identity it handed the slot to:\n%s", stdout)
	}
	store, err := peercred.OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if _, ok := store.GrantOf("peer-lost-its-string"); ok {
		t.Fatal("the old identity kept its credential after giving up the slot")
	}
	secret := secretFrom(t, stdout)
	if _, _, ok := store.Verify(peercred.Join("peer-with-a-new-string", secret)); !ok {
		t.Fatal("the handover's join string does not verify")
	}
}

// TestInsecureNoTokenRefusesANonLoopbackBind is §3.1's own condition on the
// flag, and it is checked at the BIND rather than at the first connection so
// that the failure is a startup error an operator reads instead of a map that
// quietly admits anybody. §3.1 also states the discipline around it: NO
// INSTALLER, SCRIPT OR DOCUMENT THIS PROJECT SHIPS MAY INSTRUCT A STRANGER TO
// PASS IT.
func TestInsecureNoTokenRefusesANonLoopbackBind(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runMain(t, "--data-dir", dir, "--insecure-no-token", "--listen", "0.0.0.0:0")
	if code == 0 {
		t.Fatal("--insecure-no-token bound a reachable address")
	}
	if !strings.Contains(stderr, "loopback") {
		t.Fatalf("the refusal does not name the rule:\n%s", stderr)
	}
}

// TestARelayWithNoCredentialsRefusesToServe is the startup half of the same
// rule, and the error has to name the command that fixes it — the operator is
// three words from a working map and none of them are guessable.
func TestARelayWithNoCredentialsRefusesToServe(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runMain(t, "--data-dir", dir, "--listen", "127.0.0.1:0")
	if code == 0 {
		t.Fatal("a relay with an empty credential store started and would 401 every peer")
	}
	if !strings.Contains(stderr, "--mint-credential") {
		t.Fatalf("the startup error does not name the remedy:\n%s", stderr)
	}
}

// TestJoinURLNamesWhatAPeerMustDial pins the small function a join string's
// usefulness rests on.
func TestJoinURLNamesWhatAPeerMustDial(t *testing.T) {
	for _, c := range []struct {
		advertise, listen string
		tls               bool
		want              string
	}{
		{"", "0.0.0.0:8795", true, "wss://<relay-host>:8795/contract-b/v4"},
		{"", "127.0.0.1:8795", false, "ws://127.0.0.1:8795/contract-b/v4"},
		{"", "192.168.1.227:8795", true, "wss://192.168.1.227:8795/contract-b/v4"},
		{"", ":8795", true, "wss://<relay-host>:8795/contract-b/v4"},
		{"wss://relay.example.net/contract-b/v4", "0.0.0.0:443", true,
			"wss://relay.example.net/contract-b/v4"},
	} {
		if got := joinURL(c.advertise, c.listen, c.tls); got != c.want {
			t.Fatalf("joinURL(%q,%q,%v) = %q, want %q", c.advertise, c.listen, c.tls, got, c.want)
		}
	}
}

// secretFrom lifts the secret out of a printed join string, which is also a
// small check that the report is legible to something other than a person.
func secretFrom(t *testing.T, report string) string {
	t.Helper()
	for _, line := range strings.Split(report, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "secret" {
			return fields[1]
		}
	}
	t.Fatalf("no secret line in:\n%s", report)
	return ""
}
