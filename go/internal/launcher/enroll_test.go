package launcher

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// enrollFixture is one public map served over TLS, plus a launcher pointed at
// it. TLS is not decoration: the packaged map is only accepted over https, and
// the test has to exercise that rule rather than route around it.
type enrollFixture struct {
	t        *testing.T
	server   *httptest.Server
	app      *app
	dataRoot string
	bodies   [][]byte
	relayURL string
}

func newEnrollFixture(t *testing.T, handler func(f *enrollFixture, w http.ResponseWriter, body []byte)) *enrollFixture {
	t.Helper()
	base := t.TempDir()
	f := &enrollFixture{
		t:        t,
		dataRoot: filepath.Join(base, "worlds", "second"),
		relayURL: testRelayURL,
	}
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body := new(bytes.Buffer)
		body.ReadFrom(r.Body)
		f.bodies = append(f.bodies, body.Bytes())
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type is %q, want application/json", got)
		}
		handler(f, w, body.Bytes())
	}))
	t.Cleanup(f.server.Close)

	root := filepath.Join(base, "install")
	mkdir(t, root)
	writeFile(t, filepath.Join(root, publicMapName),
		`{"format":"`+publicMapFormat+`","enrollmentUrl":"`+f.server.URL+
			`","relayUrl":"`+f.relayURL+`"}`, 0o644)

	f.app = &app{
		install: Install{Root: root},
		stdin:   bufio.NewReader(strings.NewReader("")),
		stdout:  new(bytes.Buffer),
		stderr:  new(bytes.Buffer),
		now:     func() time.Time { return time.Date(2026, 8, 16, 9, 41, 7, 0, time.UTC) },
		getenv:  func(string) string { return "" },
		client:  f.server.Client(),
	}
	return f
}

func (f *enrollFixture) pendingPath() string {
	return filepath.Join(f.dataRoot, pendingFileName)
}

func (f *enrollFixture) credentialPath() string {
	return filepath.Join(f.dataRoot, credentialFileName)
}

// pending reads the record the launcher keeps between attempts.
func (f *enrollFixture) pending() pendingEnrollment {
	f.t.Helper()
	var record pendingEnrollment
	raw, err := os.ReadFile(f.pendingPath())
	if err != nil {
		f.t.Fatalf("the pending record is missing: %v", err)
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		f.t.Fatalf("the pending record is not JSON: %v", err)
	}
	return record
}

// grant answers the way contracts/public-enrollment.md says a new credential is
// answered.
func grant(status int) func(*enrollFixture, http.ResponseWriter, []byte) {
	return func(f *enrollFixture, w http.ResponseWriter, body []byte) {
		var request enrollmentRequest
		if err := json.Unmarshal(body, &request); err != nil {
			f.t.Fatalf("the request is not JSON: %v", err)
		}
		peerID := "public-" + strings.ReplaceAll(request.InstallID, "-", "")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(enrollmentResponse{
			Format:   enrollResponseFormat,
			RelayURL: f.relayURL,
			PeerID:   peerID,
			Created:  status == http.StatusCreated,
		})
	}
}

func TestEnrollmentHappyPath(t *testing.T) {
	f := newEnrollFixture(t, grant(http.StatusCreated))

	id, err := f.app.enrollPublicMap(f.dataRoot, testRelayURL)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}

	// The request body is exactly the documented shape, in the documented order.
	if len(f.bodies) != 1 {
		t.Fatalf("the map was asked %d times, want 1", len(f.bodies))
	}
	pending := f.pending()
	wantBody := `{"format":"` + enrollRequestFormat + `","installId":"` + pending.InstallID +
		`","secret":"` + pending.Secret + `","release":"` + Release + `"}`
	if got := string(f.bodies[0]); got != wantBody {
		t.Fatalf("the request body drifted.\n got: %s\nwant: %s", got, wantBody)
	}
	if !uuidPattern.MatchString(pending.InstallID) {
		t.Fatalf("the installation id is not a UUID: %s", pending.InstallID)
	}
	if !hexSecretPattern.MatchString(pending.Secret) {
		t.Fatalf("the secret is not 64 lower-case hexadecimal characters: %s", pending.Secret)
	}
	if want := "public-" + strings.ReplaceAll(pending.InstallID, "-", ""); id.PeerID != want {
		t.Fatalf("the identity is %s, want %s", id.PeerID, want)
	}
	if id.RelayURL != testRelayURL {
		t.Fatalf("the relay is %s, want %s", id.RelayURL, testRelayURL)
	}

	// The secret half is on disk, owner-only, and the pending record is still
	// there: it is removed only after the profile exists.
	if got := readFile(t, f.credentialPath()); got != pending.Secret+"\n" {
		t.Fatalf("the credential file holds %q", got)
	}
	info, err := os.Stat(f.credentialPath())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("the credential is mode %v, want 0600", info.Mode().Perm())
	}
	if !fileExists(f.pendingPath()) {
		t.Fatal("the pending record was removed before the profile existed")
	}
	finishEnrollment(id)
	if fileExists(f.pendingPath()) {
		t.Fatal("the pending record survived a completed enrollment")
	}
}

// TestEnrollmentRetry proves a lost answer is a retry rather than a second
// identity: the same body goes out again, byte for byte.
func TestEnrollmentRetry(t *testing.T) {
	attempt := 0
	f := newEnrollFixture(t, func(f *enrollFixture, w http.ResponseWriter, body []byte) {
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		grant(http.StatusOK)(f, w, body)
	})

	if _, err := f.app.enrollPublicMap(f.dataRoot, testRelayURL); err == nil {
		t.Fatal("a 503 was accepted")
	}
	first := f.pending()
	if !fileExists(f.pendingPath()) {
		t.Fatal("the pending record did not survive a failure")
	}

	id, err := f.app.enrollPublicMap(f.dataRoot, testRelayURL)
	if err != nil {
		t.Fatalf("the retry failed: %v", err)
	}
	second := f.pending()
	if first != second {
		t.Fatalf("the retry used another identity:\n%+v\n%+v", first, second)
	}
	if len(f.bodies) != 2 || !bytes.Equal(f.bodies[0], f.bodies[1]) {
		t.Fatalf("the retry was not byte-identical:\n%s\n%s", f.bodies[0], f.bodies[1])
	}
	if want := "public-" + strings.ReplaceAll(first.InstallID, "-", ""); id.PeerID != want {
		t.Fatalf("the identity is %s, want %s", id.PeerID, want)
	}
	mustContain(t, "the output", f.app.stdout.(*bytes.Buffer).String(),
		"retrying the pending public-map enrollment")
}

func TestEnrollmentPeerIdMismatch(t *testing.T) {
	f := newEnrollFixture(t, func(f *enrollFixture, w http.ResponseWriter, body []byte) {
		json.NewEncoder(w).Encode(enrollmentResponse{
			Format:   enrollResponseFormat,
			RelayURL: f.relayURL,
			PeerID:   "public-00000000000000000000000000000000",
		})
	})
	_, err := f.app.enrollPublicMap(f.dataRoot, testRelayURL)
	if err == nil {
		t.Fatal("an answer for another identity was accepted")
	}
	mustContain(t, "the refusal", err.Error(), "a different identity")
	if !fileExists(f.pendingPath()) {
		t.Fatal("the pending record did not survive the refusal")
	}
	if fileExists(f.credentialPath()) {
		t.Fatal("a credential was written for an identity the map did not confirm")
	}
}

func TestEnrollmentRelayMismatch(t *testing.T) {
	f := newEnrollFixture(t, func(f *enrollFixture, w http.ResponseWriter, body []byte) {
		var request enrollmentRequest
		json.Unmarshal(body, &request)
		json.NewEncoder(w).Encode(enrollmentResponse{
			Format:   enrollResponseFormat,
			RelayURL: "wss://somewhere.else/contract-b/v4",
			PeerID:   "public-" + strings.ReplaceAll(request.InstallID, "-", ""),
		})
	})
	_, err := f.app.enrollPublicMap(f.dataRoot, testRelayURL)
	if err == nil {
		t.Fatal("an answer naming another relay was accepted")
	}
	mustContain(t, "the refusal", err.Error(), "or relay")
	if !fileExists(f.pendingPath()) {
		t.Fatal("the pending record did not survive the refusal")
	}
	if fileExists(f.credentialPath()) {
		t.Fatal("a credential was written for a relay the launcher does not know")
	}
}

func TestEnrollment429(t *testing.T) {
	f := newEnrollFixture(t, func(f *enrollFixture, w http.ResponseWriter, body []byte) {
		w.Header().Set("Retry-After", "600")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	_, err := f.app.enrollPublicMap(f.dataRoot, testRelayURL)
	if err == nil {
		t.Fatal("a 429 was accepted")
	}
	mustContain(t, "the refusal", err.Error(), "per-address enrollment limit")
	mustContain(t, "the refusal", err.Error(), "Retry-After: 600")
	if !fileExists(f.pendingPath()) {
		t.Fatal("the pending record did not survive a 429")
	}
}

func TestEnrollment409(t *testing.T) {
	f := newEnrollFixture(t, func(f *enrollFixture, w http.ResponseWriter, body []byte) {
		w.WriteHeader(http.StatusConflict)
	})
	_, err := f.app.enrollPublicMap(f.dataRoot, testRelayURL)
	if err == nil {
		t.Fatal("a 409 was accepted")
	}
	mustContain(t, "the refusal", err.Error(), "already has another credential")
	mustContain(t, "the refusal", err.Error(), "slot handover")
	if !fileExists(f.pendingPath()) {
		t.Fatal("the pending record did not survive a 409")
	}
}

// TestEnrollmentRefusesOverAnExistingWorld is the INS-ENROLL rule: never create
// a replacement over a world that already has an identity.
func TestEnrollmentRefusesOverAnExistingWorld(t *testing.T) {
	f := newEnrollFixture(t, grant(http.StatusCreated))
	writeFile(t, f.credentialPath(), "already here\n", 0o600)
	_, err := f.app.enrollPublicMap(f.dataRoot, testRelayURL)
	if err == nil {
		t.Fatal("a replacement identity was created over an existing world")
	}
	mustContain(t, "the refusal", err.Error(), "INS-ENROLL")
	if len(f.bodies) != 0 {
		t.Fatal("the map was contacted before the refusal")
	}
}

// TestEnrollmentRefusesOnAPrivateMap: an install on a private map cannot mint an
// identity on the packaged public one.
func TestEnrollmentRefusesOnAPrivateMap(t *testing.T) {
	f := newEnrollFixture(t, grant(http.StatusCreated))
	_, err := f.app.enrollPublicMap(f.dataRoot, "wss://private.example/contract-b/v4")
	if err == nil {
		t.Fatal("a public-map identity was minted for a private-map install")
	}
	mustContain(t, "the refusal", err.Error(), "--join-string-file")
	if len(f.bodies) != 0 {
		t.Fatal("the map was contacted before the refusal")
	}
}

func TestJoinStringParsing(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	cases := []struct {
		name      string
		line      string
		relayFlag string
		peerID    string
		secret    string
		relayURL  string
		wantError string
	}{
		{
			name:     "the three-field one line",
			line:     joinStringPrefix + " wss://relay.example/contract-b/v4 my-world." + secret,
			peerID:   "my-world",
			secret:   secret,
			relayURL: "wss://relay.example/contract-b/v4",
		},
		{
			name:      "the identity half alone with --relay-url",
			line:      "my-world." + secret,
			relayFlag: "wss://relay.example/contract-b/v4",
			peerID:    "my-world",
			secret:    secret,
			relayURL:  "wss://relay.example/contract-b/v4",
		},
		{
			name:      "the identity half alone with no relay",
			line:      "my-world." + secret,
			wantError: "multiverse-join/1",
		},
		{
			name:      "an unencrypted relay",
			line:      joinStringPrefix + " ws://relay.example/contract-b/v4 my-world." + secret,
			wantError: "there is no plain fallback",
		},
		{
			name:      "a relay that is not a URL",
			line:      joinStringPrefix + " relay.example my-world." + secret,
			wantError: "not a wss:// URL",
		},
		{
			// The halves split on the LAST dot: an identity may contain one and
			// a secret may not.
			name:     "an identity holding a dot",
			line:     joinStringPrefix + " wss://relay.example/contract-b/v4 my.world." + secret,
			peerID:   "my.world",
			secret:   secret,
			relayURL: "wss://relay.example/contract-b/v4",
		},
		{
			name:      "no dot at all",
			line:      joinStringPrefix + " wss://relay.example/contract-b/v4 myworld",
			wantError: "joined by a dot",
		},
		{
			name:      "a dot at the end",
			line:      joinStringPrefix + " wss://relay.example/contract-b/v4 myworld.",
			wantError: "joined by a dot",
		},
		{
			name:      "a secret that is too short",
			line:      joinStringPrefix + " wss://relay.example/contract-b/v4 my-world.tooshort",
			wantError: "32 to 256",
		},
		{
			name: "a secret that is too long",
			line: joinStringPrefix + " wss://relay.example/contract-b/v4 my-world." +
				strings.Repeat("a", 257),
			wantError: "32 to 256",
		},
		{
			name: "a secret holding a character that is not printable ASCII",
			line: joinStringPrefix + " wss://relay.example/contract-b/v4 my-world." +
				strings.Repeat("é", 32),
			wantError: "printable ASCII",
		},
		{
			name:      "two fields",
			line:      "one two",
			wantError: "not a join string this launcher can read",
		},
		{
			// --relay-url beside a join string that already names a relay was
			// silently ignored, so a user who retyped the address was never told
			// which one was used.
			name:      "--relay-url beside a full join string",
			line:      joinStringPrefix + " wss://relay.example/contract-b/v4 my-world." + secret,
			relayFlag: "wss://somewhere.else/contract-b/v4",
			wantError: "Remove --relay-url",
		},
		{
			name:      "--relay-url repeating the join string's own relay",
			line:      joinStringPrefix + " wss://relay.example/contract-b/v4 my-world." + secret,
			relayFlag: "wss://relay.example/contract-b/v4",
			peerID:    "my-world",
			secret:    secret,
			relayURL:  "wss://relay.example/contract-b/v4",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			peerID, secret, relayURL, err := parseJoinString(test.line, test.relayFlag)
			if test.wantError != "" {
				if err == nil {
					t.Fatalf("%q was accepted and must not be", test.line)
				}
				mustContain(t, "the refusal", err.Error(), test.wantError)
				return
			}
			if err != nil {
				t.Fatalf("%q was refused: %v", test.line, err)
			}
			if peerID != test.peerID || secret != test.secret || relayURL != test.relayURL {
				t.Fatalf("got %q %q %q, want %q %q %q",
					peerID, secret, relayURL, test.peerID, test.secret, test.relayURL)
			}
		})
	}

	// The join FILE takes the first line that is neither empty nor a comment.
	path := filepath.Join(t.TempDir(), "join.txt")
	writeFile(t, path, "# the operator's note\n\n"+joinStringPrefix+
		" wss://relay.example/contract-b/v4 my-world."+secret+"\n", 0o600)
	line, err := readJoinString(path)
	if err != nil {
		t.Fatalf("readJoinString: %v", err)
	}
	if !strings.HasPrefix(line, joinStringPrefix) {
		t.Fatalf("the join string read as %q", line)
	}
	if _, err := readJoinString(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Fatal("a missing join file was accepted")
	}
}

// TestEnrollmentRefusesARedirect is the credential-leak regression. Go replays
// a POST body verbatim on 307 and 308, and http.Client's default CheckRedirect
// follows one, so an enrollment address that redirected to plain http would
// re-send the only recoverable secret in the system in clear.
func TestEnrollmentRefusesARedirect(t *testing.T) {
	leaked := make(chan string, 1)
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		body.ReadFrom(r.Body)
		leaked <- body.String()
		w.WriteHeader(http.StatusOK)
	}))
	defer plain.Close()

	f := newEnrollFixture(t, func(f *enrollFixture, w http.ResponseWriter, body []byte) {
		http.Redirect(w, &http.Request{}, plain.URL+"/api/enroll", http.StatusTemporaryRedirect)
	})
	// The fixture's client is the test server's, so give it the launcher's own
	// redirect policy: that policy is what is under test.
	f.app.client.CheckRedirect = enrollmentClient().CheckRedirect

	_, err := f.app.enrollPublicMap(f.dataRoot, testRelayURL)
	// The leak is the headline: assert it before anything about wording.
	select {
	case body := <-leaked:
		t.Fatalf("the secret was re-sent over plain http: %s", body)
	default:
	}
	if err == nil {
		t.Fatal("a redirect was followed")
	}
	mustContain(t, "the refusal", err.Error(), "does not follow a redirect")
	if fileExists(f.credentialPath()) {
		t.Fatal("a credential was written for a redirect that was refused")
	}
	if !fileExists(f.pendingPath()) {
		t.Fatal("the pending record did not survive the refusal, so a retry is not safe")
	}
}

// TestEnrollmentUnreadablePendingRecordAborts: an existing pending record that
// cannot be read must STOP the run. Minting a fresh identity would consume
// another public-map identity and another per-address entry, which is what
// "a lost response is a retry, not a second identity" forbids.
func TestEnrollmentUnreadablePendingRecordAborts(t *testing.T) {
	f := newEnrollFixture(t, grant(http.StatusCreated))
	if err := os.MkdirAll(f.dataRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A directory where the file should be is a read error that is not
	// "missing", which is the shape of a scanner holding the file on Windows.
	if err := os.MkdirAll(f.pendingPath(), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := f.app.enrollPublicMap(f.dataRoot, testRelayURL)
	if err == nil {
		t.Fatal("an unreadable pending record was replaced")
	}
	mustContain(t, "the refusal", err.Error(), "exists but could not be read")
	mustContain(t, "the refusal", err.Error(), "no new identity was created")
	if len(f.bodies) != 0 {
		t.Fatal("the map was contacted with a fresh identity")
	}
}
