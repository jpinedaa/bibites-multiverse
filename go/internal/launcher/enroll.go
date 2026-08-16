package launcher

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Enrollment gives one new world its own identity on a map. It is a faithful
// port of step 6 of release/kit/Install-BibitesMultiverse.ps1 and of
// contracts/public-enrollment.md, because a second world created by the
// launcher must be indistinguishable, to the relay, from a first world created
// by the installer.

const (
	publicMapFormat      = "bibites-multiverse/public-map/1"
	pendingFormat        = "bibites-multiverse/enrollment-pending/1"
	enrollRequestFormat  = "bibites-multiverse/enrollment-request/1"
	enrollResponseFormat = "bibites-multiverse/enrollment-response/1"

	joinStringPrefix = "multiverse-join/1"

	minSecretLength = 32
	maxSecretLength = 256
)

// enrollmentBodyLimit bounds what is read from the enrollment endpoint. The
// documented response is a few hundred bytes.
const enrollmentBodyLimit = 64 << 10

var (
	hexSecretPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	uuidPattern      = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// publicMap is the packaged, secret-free join configuration.
type publicMap struct {
	Format        string `json:"format"`
	EnrollmentURL string `json:"enrollmentUrl"`
	RelayURL      string `json:"relayUrl"`
}

// pendingEnrollment is the protected record written before the first request,
// so a lost response is a retry rather than a second identity.
type pendingEnrollment struct {
	Format    string `json:"format"`
	InstallID string `json:"installId"`
	Secret    string `json:"secret"`
}

type enrollmentRequest struct {
	Format    string `json:"format"`
	InstallID string `json:"installId"`
	Secret    string `json:"secret"`
	Release   string `json:"release"`
}

type enrollmentResponse struct {
	Format   string `json:"format"`
	RelayURL string `json:"relayUrl"`
	PeerID   string `json:"peerId"`
	Created  bool   `json:"created"`
}

// identity is what a new world got: the half that is not a secret, plus the
// pending record that may only be removed once the profile exists on disk.
type identity struct {
	PeerID      string
	RelayURL    string
	pendingPath string
}

// readPublicMap loads and checks the packaged public-map.json.
func readPublicMap(path string) (publicMap, error) {
	var m publicMap
	raw, err := os.ReadFile(path)
	if err != nil {
		return m, fmt.Errorf("this installation has no %s, so it cannot enroll a world on the public "+
			"map. Use --join-string-file for a private map (%s)", publicMapName, path)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("%s is not valid JSON", path)
	}
	if m.Format != publicMapFormat {
		return m, fmt.Errorf("%s has an unsupported format", path)
	}
	if !enrollURLPattern.MatchString(m.EnrollmentURL) {
		return m, fmt.Errorf("%s does not contain a secure HTTPS enrollment address", path)
	}
	if !relayURLPattern.MatchString(m.RelayURL) {
		return m, fmt.Errorf("%s does not contain a secure WSS relay address", path)
	}
	return m, nil
}

// enrollPublicMap creates one new identity on the map this installation is
// already on, and writes the secret half into the new world's data root.
func (a *app) enrollPublicMap(dataRoot, activeRelay string) (identity, error) {
	var id identity
	m, err := readPublicMap(a.install.PublicMapPath())
	if err != nil {
		return id, err
	}
	// An install on a private map cannot mint an identity on the public one:
	// the two maps do not know each other, and the operator hands out identity
	// on a private map by hand.
	if activeRelay != "" && activeRelay != m.RelayURL {
		return id, fmt.Errorf("this installation is on the map at %s, and the packaged public map is "+
			"%s. A new world on a private map takes a join string from that map's operator: pass "+
			"--join-string-file", activeRelay, m.RelayURL)
	}

	credentialPath := filepath.Join(dataRoot, credentialFileName)
	pendingPath := filepath.Join(dataRoot, pendingFileName)
	if marker := worldMarker(dataRoot); marker != "" {
		return id, fmt.Errorf("%s already holds a world: %s is in it. A new world never goes over an "+
			"existing one - two identities over one journal strand both. Create this world with "+
			"another --data-root; the default gives every world its own. To put THAT world back on "+
			"this computer instead, run Install-BibitesMultiverse.ps1 -DataRoot %q, which reuses the "+
			"identity already there (INS-ENROLL)",
			dataRoot, marker, dataRoot)
	}
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		return id, err
	}

	pending, err := a.loadOrCreatePending(pendingPath)
	if err != nil {
		return id, err
	}
	expectedPeerID := "public-" + strings.ReplaceAll(strings.ToLower(pending.InstallID), "-", "")

	a.say("requesting a unique identity from %s", m.EnrollmentURL)
	response, err := a.postEnrollment(m.EnrollmentURL, enrollmentRequest{
		Format:    enrollRequestFormat,
		InstallID: pending.InstallID,
		Secret:    pending.Secret,
		Release:   Release,
	})
	if err != nil {
		// The pending record is deliberately kept: running this again is a safe
		// retry that sends a byte-identical body.
		return id, err
	}
	if response.Format != enrollResponseFormat || response.RelayURL != m.RelayURL || response.PeerID != expectedPeerID {
		return id, fmt.Errorf("the public map returned an enrollment response for a different identity " +
			"or relay. Nothing was changed; the pending identity was kept")
	}

	if err := writeSecretFile(credentialPath, pending.Secret); err != nil {
		return id, err
	}
	if err := protectUserFile(credentialPath); err != nil {
		a.warnUnprotectedCredential(credentialPath, err)
	}
	return identity{PeerID: expectedPeerID, RelayURL: m.RelayURL, pendingPath: pendingPath}, nil
}

// loadOrCreatePending reuses a pending record when there is one, and mints a
// fresh identity and secret when there is not. A protection failure on a fresh
// record is fatal and nothing is contacted.
func (a *app) loadOrCreatePending(path string) (pendingEnrollment, error) {
	var pending pendingEnrollment
	raw, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		// The record EXISTS and could not be read - a scanner holding it, an
		// ACL from another account. Minting a fresh identity here would consume
		// another public-map identity and another per-address entry, which is
		// exactly what "a lost response is a retry, not a second identity" (
		// contracts/public-enrollment.md) forbids.
		return pending, fmt.Errorf("%s exists but could not be read (%w). It holds this world's "+
			"pending identity, so nothing was contacted and no new identity was created. Fix the "+
			"file's permissions and run this again", path, readErr)
	}
	if readErr == nil {
		if err := json.Unmarshal(raw, &pending); err != nil {
			return pending, fmt.Errorf("%s is not valid JSON. Remove it only if you want a different "+
				"map identity", path)
		}
		if pending.Format != pendingFormat || !uuidPattern.MatchString(pending.InstallID) ||
			!hexSecretPattern.MatchString(pending.Secret) {
			return pending, fmt.Errorf("%s is not an enrollment record this launcher can use. Remove "+
				"it only if you want a different map identity", path)
		}
		a.say("retrying the pending public-map enrollment")
		return pending, nil
	}

	installID, err := newUUIDv4()
	if err != nil {
		return pending, err
	}
	secret, err := newSecretHex()
	if err != nil {
		return pending, err
	}
	pending = pendingEnrollment{Format: pendingFormat, InstallID: installID, Secret: secret}
	encoded, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return pending, err
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		return pending, err
	}
	if err := protectUserFile(path); err != nil {
		os.Remove(path)
		return pendingEnrollment{}, fmt.Errorf("the launcher could not protect its pending map "+
			"credential. It did not contact the map: %w", err)
	}
	return pending, nil
}

// postEnrollment sends the documented request and maps the documented statuses
// to something a participant can act on.
func (a *app) postEnrollment(url string, request enrollmentRequest) (enrollmentResponse, error) {
	var parsed enrollmentResponse
	body, err := json.Marshal(request)
	if err != nil {
		return parsed, err
	}
	httpRequest, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return parsed, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(httpRequest)
	if err != nil {
		return parsed, fmt.Errorf("the public map did not create this world's identity: %w. The "+
			"pending identity was kept, so running this again is a safe retry", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, enrollmentBodyLimit))
	if err != nil {
		return parsed, fmt.Errorf("the public map's answer could not be read: %w. The pending "+
			"identity was kept, so running this again is a safe retry", err)
	}

	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated:
	case http.StatusBadRequest:
		return parsed, fmt.Errorf("the public map refused this request as malformed (400). The " +
			"pending identity was kept; report this, because the launcher composed the request")
	case http.StatusConflict:
		return parsed, fmt.Errorf("this installation id already has another credential on the map " +
			"(409). Enrollment never replaces an existing credential: ask the operator for a slot " +
			"handover, or create the world in a data folder that has never enrolled")
	case http.StatusTooManyRequests:
		retry := response.Header.Get("Retry-After")
		if retry == "" {
			retry = "unknown"
		}
		return parsed, fmt.Errorf("the map's per-address enrollment limit was reached (429). "+
			"Retry-After: %s seconds. The pending identity was kept, so running this again after "+
			"that is a safe retry", retry)
	case http.StatusServiceUnavailable:
		return parsed, fmt.Errorf("the map is not enrolling new worlds right now (503): either " +
			"enrollment is closed or the automatic-credential limit is full. The pending identity " +
			"was kept, so running this again later is a safe retry")
	default:
		return parsed, fmt.Errorf("the public map answered HTTP %d. The pending identity was kept, "+
			"so running this again is a safe retry", response.StatusCode)
	}

	if err := json.Unmarshal(raw, &parsed); err != nil {
		return parsed, fmt.Errorf("the public map returned an answer that is not JSON. The pending " +
			"identity was kept")
	}
	if parsed.Format == "" || parsed.RelayURL == "" || parsed.PeerID == "" {
		return parsed, fmt.Errorf("the public map returned an incomplete enrollment response. The " +
			"pending identity was kept")
	}
	return parsed, nil
}

// finishEnrollment removes the pending record. It runs only after the
// credential and the profile both exist, which is what makes a lost response a
// retry rather than a second identity.
func finishEnrollment(id identity) {
	if id.pendingPath != "" {
		os.Remove(id.pendingPath)
	}
}

// worldMarker names the first file that says this data root already belongs to a
// world, or "" when nothing does. It is the launcher's half of the installers'
// rule that an identity belongs to a data root: the credential and the install
// record are what a live world leaves, and <data root>/data/peer-id is what an
// UNINSTALLED one leaves - the uninstall keeps the world's data, so a folder with
// a journal and a name in it is still somebody's world and never a place to put a
// new one.
func worldMarker(dataRoot string) string {
	for _, rel := range []string{
		credentialFileName,
		recordName,
		filepath.Join(dataDirName, peerIDFileName),
	} {
		if fileExists(filepath.Join(dataRoot, rel)) {
			return rel
		}
	}
	return ""
}

// identityFromJoinFile is the private-map path: the operator's one-line join
// string, read from a file so the secret never reaches a command line.
func (a *app) identityFromJoinFile(joinFile, relayFlag, dataRoot string) (identity, error) {
	var id identity
	line, err := readJoinString(joinFile)
	if err != nil {
		return id, err
	}
	peerID, secret, relayURL, err := parseJoinString(line, relayFlag)
	if err != nil {
		return id, err
	}
	credentialPath := filepath.Join(dataRoot, credentialFileName)
	if marker := worldMarker(dataRoot); marker != "" {
		return id, fmt.Errorf("%s already holds a world: %s is in it. A new world never goes over an "+
			"existing one - two identities over one journal strand both. Create this world with "+
			"another --data-root. To put THAT world back on this computer instead, run "+
			"Install-BibitesMultiverse.ps1 -DataRoot %q with its own join string, which reuses the "+
			"identity already there (INS-ENROLL)",
			dataRoot, marker, dataRoot)
	}
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		return id, err
	}
	if err := writeSecretFile(credentialPath, secret); err != nil {
		return id, err
	}
	if err := protectUserFile(credentialPath); err != nil {
		a.warnUnprotectedCredential(credentialPath, err)
	}
	a.say("delete %s now. It still holds the secret in clear text, and the launcher will not "+
		"delete a file you gave it.", joinFile)
	return identity{PeerID: peerID, RelayURL: relayURL}, nil
}

// readJoinString returns the first non-empty, non-comment line of the file.
func readJoinString(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("there is no join string file at %s", path)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		candidate := strings.TrimSpace(line)
		if candidate != "" && !strings.HasPrefix(candidate, "#") {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s holds no join string. Its first non-empty line must be the one your "+
		"operator sent", path)
}

// parseJoinString splits the operator's line into the relay address and the two
// halves of the credential. The halves split on the LAST dot: an identity may
// legally contain one and a secret may not (contract-b-m4.md §3.1).
func parseJoinString(line, relayFlag string) (peerID, secret, relayURL string, err error) {
	fields := strings.Fields(line)
	var credential string
	switch {
	case len(fields) == 3 && fields[0] == joinStringPrefix:
		if strings.TrimSpace(relayFlag) != "" && !strings.EqualFold(strings.TrimSpace(relayFlag), fields[1]) {
			return "", "", "", fmt.Errorf("the join string already names the relay %s, and "+
				"--relay-url says %s. Remove --relay-url, or use it with a file that holds the "+
				"identity half alone", fields[1], strings.TrimSpace(relayFlag))
		}
		relayURL = fields[1]
		credential = fields[2]
	case len(fields) == 1:
		credential = fields[0]
		relayURL = strings.TrimSpace(relayFlag)
		if relayURL == "" {
			return "", "", "", fmt.Errorf("that is the identity half on its own, with no relay " +
				"address. Either use the whole one-line join string - it starts with " +
				"'multiverse-join/1' - or add --relay-url wss://<relay-host>/contract-b/v4")
		}
	default:
		return "", "", "", fmt.Errorf("that is not a join string this launcher can read. The " +
			"one-line form is three parts: 'multiverse-join/1', the wss:// relay address, and your " +
			"identity and secret joined by a dot")
	}
	if err := validateRelayURL(relayURL); err != nil {
		return "", "", "", err
	}
	dot := strings.LastIndexByte(credential, '.')
	if dot <= 0 || dot == len(credential)-1 {
		return "", "", "", fmt.Errorf("the identity half and the secret half are joined by a dot, " +
			"and this value has no usable one. Use the whole 'one line' your operator sent")
	}
	peerID = credential[:dot]
	secret = credential[dot+1:]
	if len(secret) < minSecretLength || len(secret) > maxSecretLength {
		return "", "", "", fmt.Errorf("the secret half is %d characters. It must be %d to %d. Nothing "+
			"was written; ask your operator to re-send the join string exactly as their relay printed it",
			len(secret), minSecretLength, maxSecretLength)
	}
	if !printableASCII.MatchString(secret) {
		return "", "", "", fmt.Errorf("the secret half must be printable ASCII with no spaces. " +
			"Nothing was written")
	}
	if !printableASCII.MatchString(peerID) {
		return "", "", "", fmt.Errorf("the identity half must be printable ASCII with no spaces. " +
			"Nothing was written")
	}
	return peerID, secret, relayURL, nil
}

// writeSecretFile writes the secret half and nothing else, owner-only from the
// first byte. It is written fresh every time, because re-writing over an
// already protected file needs a privilege an ordinary account does not have.
func writeSecretFile(path, secret string) error {
	os.Remove(path)
	return os.WriteFile(path, []byte(secret+"\n"), 0o600)
}

// newUUIDv4 mints the installation id the peer id is derived from.
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("this machine could not produce randomness for a new identity: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	text := hex.EncodeToString(b[:])
	return text[0:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:32], nil
}

// newSecretHex mints the client half of the credential: 32 random bytes as 64
// lower-case hexadecimal characters, which is what both installers do.
func newSecretHex() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("this machine could not produce randomness for a new credential: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// publicMapNote is printed once after a world is enrolled on the public map,
// because a per-address limit is the thing a participant creating several
// worlds in a row will meet and will not expect.
const publicMapNote = `  Every world you add takes another identity on the public map, and the map applies
  a per-address enrollment limit, so several worlds created quickly can be refused
  for a while.`

// leavingNote is printed after every create, whichever map it was: deleting a
// world here is not the same act as leaving the map.
const leavingNote = `  Deleting a world here is not leaving the map - see docs/participant/leave.md.`

// warnUnprotectedCredential says what is true about WHERE the credential is.
// The installer can promise "inside your own profile" because its data root
// defaults under it; the launcher lets --data-root be anywhere, so the promise
// is only made when it holds.
func (a *app) warnUnprotectedCredential(path string, cause error) {
	a.warn("the map credential is in %s, and its permissions could not be tightened: %v", path, cause)
	if home, err := os.UserHomeDir(); err == nil && home != "" && pathWithin(path, home) {
		a.warn("it is inside your own profile, which other accounts cannot read by default.")
		return
	}
	a.warn("it is NOT inside your own profile, so other accounts on this machine may be able to "+
		"read it. Tighten the permissions on %s yourself, or move this world's data folder under "+
		"your own profile.", path)
}
