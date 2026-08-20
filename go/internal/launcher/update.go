package launcher

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// THE LAUNCHER SAYS WHEN A NEWER RELEASE EXISTS, AND NOTHING ELSE HAPPENS IF IT
// CANNOT FIND OUT.
//
// A participant installs once and then never hears about a fix again: the
// shortcut opens the launcher, the launcher opens the world, and the release
// that repaired the thing they are about to hit is on a web page nobody told
// them to reload. This is the smallest honest answer to that — one GET, on the
// way past, whose entire consequence is a line of text and a button.
//
// FIVE RULES HOLD IT TO "IT CAN ONLY EVER ADD A LINE":
//
//  1. NOTHING WAITS ON IT. The lookup is a goroutine of its own started beside
//     the front door; every reader takes a lock and returns whatever is there,
//     which for the first moments — and for ever on a machine with no route out
//     — is the empty string. A WORLD START NEVER TOUCHES THIS CODE AT ALL.
//  2. A FAILURE IS SILENT. No network, a captive portal, a proxy, a 500, a body
//     that is not JSON: every one of them leaves the value empty and prints
//     nothing. A launcher that complained about its own update check would be
//     reporting a fault in a feature the participant did not ask for, on the one
//     screen they opened to start a world.
//  3. ONLY FORWARD. A published release that is older than this one, equal to
//     it, or not comparable at all says nothing (see NewerRelease). A reinstall
//     of a current build must not be nagged.
//  4. IT ASKS THE HOMEPAGE, NOT GITHUB. The homepage already resolves the newest
//     release once an hour and holds it in memory (internal/archive/release.go);
//     every launcher asking GitHub directly would spend the anonymous rate limit
//     of whatever network the player is on, per machine, for a number one host
//     already knows.
//  5. THE REQUEST SAYS NOTHING ABOUT WHO IS ASKING. A bare GET: no query, no
//     body, no cookie, no identity, and a User-Agent that names the program and
//     NOT its version — see releaseRequest. Nothing about this call is a report,
//     and the answer to "how many people run which release" is deliberately not
//     obtainable from it.
//
// THE LINK IS THIS PROGRAM'S OWN CONSTANT. What comes back is a version string
// and only a version string; the address the button opens is HomeURL, compiled
// in. An endpoint that could hand the launcher a URL to open would be an
// endpoint that could aim it anywhere.

// HomeURL is the project's public page: where a person downloads a release, and
// the one address anything in this program opens in a browser.
const HomeURL = "https://bibitesmultiverse.com"

// ReleaseCheckURL is the one address the update check ever asks. It is served by
// the same process that serves HomeURL (internal/archive, /api/release) and
// answers two fields: the published tag, and that tag as a bare version number.
const ReleaseCheckURL = HomeURL + "/api/release"

// releaseCheckTimeout bounds the whole lookup. Nothing waits on it, so the
// timeout is not about latency — it is about not leaving a goroutine and a
// socket on a captive portal that answers nothing, ever.
const releaseCheckTimeout = 6 * time.Second

// releaseCheckBodyLimit bounds what is read back, for the same reason
// enrollment and the own-slot probe bound theirs: this is a remote document, and
// an unbounded ReadAll of one is a way to be hurt by somebody else's bug.
const releaseCheckBodyLimit = 1 << 16

// TWO VARIABLES, EACH WITH ONE JOB, and they are two rather than one for a
// reason a single knob could not satisfy: the launcher's environment arrives as
// a plain getenv, which cannot tell "set to nothing" from "not set at all". An
// off switch spelled as an empty value would therefore be an off switch that
// silently did nothing on the machine of the one person who wanted it.
const (
	// ReleaseCheckURLEnv moves the check somewhere else. It exists for a test
	// and for a manual rehearsal, which is the only way to see the notice on a
	// machine that is already running the newest release.
	ReleaseCheckURLEnv = "MULTIVERSE_RELEASE_URL"
	// NoUpdateCheckEnv turns the check off. Any non-empty value, and the
	// launcher makes no request at all — not a shorter one, not a cached one,
	// none. It wins over ReleaseCheckURLEnv, because "make no request" is an
	// answer to a question "make it to this address" cannot overrule.
	NoUpdateCheckEnv = "MULTIVERSE_NO_UPDATE_CHECK"
)

// updateWatch is one launcher run's answer to "is there a newer release". The
// zero value is usable and means "nothing to say", which is also what a run that
// never started a lookup reports.
type updateWatch struct {
	mu    sync.Mutex
	newer string
}

// Available is the newer release, or "" when there is none, when the lookup has
// not answered yet, and when it failed. A caller cannot tell those apart on
// purpose: all four mean "say nothing".
func (w *updateWatch) Available() string {
	if w == nil {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.newer
}

func (w *updateWatch) record(published string) {
	if newer, ok := NewerRelease(Release, published); ok {
		w.mu.Lock()
		w.newer = newer
		w.mu.Unlock()
	}
}

// startUpdateWatch begins one lookup in the background and returns immediately.
// The goroutine outlives nothing: it is bounded by releaseCheckTimeout, it holds
// no lock while it waits, and a process that exits first simply takes it with it.
func startUpdateWatch(getenv func(string) string) *updateWatch {
	w := &updateWatch{}
	url := releaseCheckURL(getenv)
	if url == "" {
		return w
	}
	client := &http.Client{Timeout: releaseCheckTimeout}
	go func() {
		if published, ok := fetchPublishedRelease(client, url); ok {
			w.record(published)
		}
	}()
	return w
}

// releaseCheckURL is the address to ask, or "" for no request at all.
func releaseCheckURL(getenv func(string) string) string {
	if getenv == nil {
		return ReleaseCheckURL
	}
	if strings.TrimSpace(getenv(NoUpdateCheckEnv)) != "" {
		return ""
	}
	if moved := strings.TrimSpace(getenv(ReleaseCheckURLEnv)); moved != "" {
		return moved
	}
	return ReleaseCheckURL
}

// releaseAnswer is the endpoint's payload. Both fields are absent when the
// homepage has not resolved a release itself, which is a valid answer and reads
// here as an empty version.
type releaseAnswer struct {
	Tag     string `json:"tag"`
	Release string `json:"release"`
}

// fetchPublishedRelease performs the one request. Every failure is the same
// failure — no version, and no word to anybody — because there is nothing a
// caller could usefully do about any of them.
func fetchPublishedRelease(client *http.Client, url string) (string, bool) {
	req, err := releaseRequest(url)
	if err != nil {
		return "", false
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, releaseCheckBodyLimit))
	if err != nil {
		return "", false
	}
	var answer releaseAnswer
	if err := json.Unmarshal(body, &answer); err != nil {
		return "", false
	}
	// The bare number is the field to compare; the tag is the fallback, because
	// a version with the tag's "v" still on it compares correctly once
	// NewerRelease has trimmed it.
	if answer.Release != "" {
		return answer.Release, true
	}
	return answer.Tag, answer.Tag != ""
}

// releaseRequest builds the GET, and the headers on it are the whole of what
// this program discloses.
//
// THE USER-AGENT CARRIES NO VERSION. It names the program so that an operator
// reading a log can tell a launcher from a browser, and it stops there: a
// version in it would turn a cosmetic lookup into a census of which release each
// address is running, which is a thing this project does not collect and should
// not be able to collect by accident.
func releaseRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "bibites-multiverse-launcher")
	return req, nil
}

// ---------------------------------------------------------------- the compare

// releaseComponentLimit bounds how much of a version string is compared. Four
// components is one more than this project publishes; anything longer is not a
// release of this program and is refused rather than truncated.
const releaseComponentLimit = 4

// NewerRelease answers whether published names a release newer than current,
// and gives back the published version in the form to show a person.
//
// IT IS DELIBERATELY CONSERVATIVE. Every doubt answers false: a component that
// is not a number, a string longer than releaseComponentLimit components, an
// empty side, an equal pair, an older published release. The cost of a false
// negative is that somebody misses a line on a menu; the cost of a false
// positive is a launcher that tells every participant to reinstall the version
// they already have.
//
// A LEADING "v" IS TRIMMED ON BOTH SIDES, because this project publishes tags as
// "v2.5.4" and version constants as "2.5.4", and the two must compare as the
// same release. The example is deliberately not this release: nothing in this
// file may name it, which is what release/bump-version.sh checks.
func NewerRelease(current, published string) (string, bool) {
	shown := strings.TrimSpace(published)
	mine, ok := releaseComponents(current)
	if !ok {
		return "", false
	}
	theirs, ok := releaseComponents(shown)
	if !ok {
		return "", false
	}
	for i := 0; i < releaseComponentLimit; i++ {
		// A missing component is a zero: 0.4 and 0.4.0 are the same release, and
		// 0.4.1 is newer than both.
		var a, b int
		if i < len(mine) {
			a = mine[i]
		}
		if i < len(theirs) {
			b = theirs[i]
		}
		if b > a {
			return trimReleasePrefix(shown), true
		}
		if b < a {
			return "", false
		}
	}
	return "", false
}

// releaseComponents splits a version into its numbers, or refuses. Refusing is
// the answer to anything this project does not publish: a pre-release suffix, a
// build tag, a word, an empty string, or more components than a release has.
func releaseComponents(version string) ([]int, bool) {
	version = trimReleasePrefix(strings.TrimSpace(version))
	if version == "" {
		return nil, false
	}
	parts := strings.Split(version, ".")
	if len(parts) > releaseComponentLimit {
		return nil, false
	}
	numbers := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nil, false
		}
		numbers = append(numbers, n)
	}
	return numbers, true
}

// trimReleasePrefix takes the tag's "v" off a version that carries one, and
// leaves anything else exactly as it is.
func trimReleasePrefix(version string) string {
	if len(version) > 1 && (version[0] == 'v' || version[0] == 'V') &&
		version[1] >= '0' && version[1] <= '9' {
		return version[1:]
	}
	return version
}

// ---------------------------------------------------------------- the words

// UpdateNotice is the ONE sentence every front door says it with, so the console
// menu and the window cannot come to word it differently. "" when there is
// nothing to say, which is the state a caller must be able to draw.
//
// IT NAMES BOTH NUMBERS. "Version <the new one> is available" alone leaves a
// reader asking which one they have, and the answer is one line further up a
// menu they may not be looking at — and is not on the window at all. The
// example is written as a placeholder rather than as a number on purpose: a
// release literal in this file is a release literal outside the bump's
// allowlist, and it fails release/bump-version.sh --check on the release that
// happens to collide with it.
func UpdateNotice(newer string) string {
	if newer == "" {
		return ""
	}
	return "Version " + newer + " is available. This one is " + Release +
		". Download it at " + HomeURL
}
