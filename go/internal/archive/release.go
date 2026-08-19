package archive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// THE HOMEPAGE SAYS WHICH RELEASE IT IS HANDING OUT, WITHOUT A DEPLOYMENT PER
// RELEASE. Those two halves pull against each other, and this file is where
// they are reconciled.
//
// The download buttons address GitHub's /releases/latest, so publishing a
// release moves the page on its own (landing.go). The cost of that was that the
// page could not name what it served: a visitor saw three buttons and no way to
// tell what came down them, or whether anything had changed since last week. A
// number baked into the binary would have answered that and given the property
// back — every release would then need a homepage redeploy to stay honest, and
// a missed one would leave the page CONFIDENTLY WRONG, which is worse than the
// silence it replaced.
//
// So the number is resolved at RUN TIME from the same source the buttons
// resolve against: GitHub's own "latest release" endpoint for this repository.
// One background lookup an hour, cached in this process, and the request path
// only ever reads the cached string.
//
// FOUR RULES HOLD IT TO "NEVER BREAKS THE PAGE":
//
//  1. NO REQUEST EVER WAITS ON GITHUB. The refresh is a goroutine of its own;
//     Tag() takes a read lock and returns. A GitHub outage cannot add a
//     millisecond to a page load, because the page load never asks.
//  2. NOT KNOWING IS A VALID STATE. Before the first success, and forever if
//     every lookup fails, the tag is "" and the page renders exactly as it did
//     before this file existed — the version line is simply not there. Nothing
//     about the page is conditional on the lookup having worked.
//  3. STALE BEATS SILENT. A failed refresh keeps the last good value rather
//     than clearing it. A tag can only be wrong here by being one release
//     behind for as long as GitHub is unreachable, and the buttons beside it
//     are resolved by GitHub itself, so they stay right regardless.
//  4. ONLY A TAG-SHAPED ANSWER IS RENDERED. See tagLooksLikeRelease: the value
//     is written into HTML, and a field from a remote JSON document does not
//     get to carry markup into this page on the strength of who serves it.
//
// UNAUTHENTICATED ON PURPOSE. The endpoint is public and anonymous callers get
// 60 requests an hour per IP; this asks for one an hour once it is working, and
// one every few minutes while it is not (releaseRetryInterval), which is inside
// that budget by more than an order of magnitude. A token would be a secret to
// deploy, rotate and leak for a number that is already public.
const (
	// releaseRefreshInterval is the cache TTL: how long a resolved tag is
	// reused before the next lookup. A release is a manual, rare event and the
	// page is not a release announcement, so an hour of lag is invisible.
	releaseRefreshInterval = time.Hour
	// releaseRetryInterval is the shorter cadence used while NOTHING has ever
	// resolved. A box that starts before its network does should not go an hour
	// with a version-less page for the sake of one missed lookup.
	releaseRetryInterval = 5 * time.Minute
	// releaseFetchTimeout bounds one lookup end to end. Nothing waits on it, so
	// the timeout is not about latency — it is about not leaking a goroutine and
	// a socket into a black hole every hour.
	releaseFetchTimeout = 5 * time.Second
	// releaseTagMaxLen is a sanity bound on the rendered string. Real tags are
	// "v1.2.3"; anything remotely near this length is not a version.
	releaseTagMaxLen = 32
)

// releaseTracker is the in-process cache of the newest published release tag.
// The zero value is not usable; construct it with newReleaseTracker.
type releaseTracker struct {
	// endpoint is the full API URL. It is a field rather than a constant so a
	// test can point the tracker at an httptest server — the same seam the rest
	// of this package uses for anything that would otherwise reach the network.
	endpoint string
	client   *http.Client
	log      *slog.Logger

	mu  sync.RWMutex
	tag string
	// etag is the validator from the last successful answer, replayed on the
	// next lookup. GitHub answers an unchanged release with 304 and an empty
	// body, which costs this host ~200 bytes instead of ~20KB an hour and,
	// unlike a 200, is not charged against the anonymous rate limit at all. A
	// release changes a few times a year; nearly every lookup is a 304.
	etag string
}

func newReleaseTracker(repo string, log *slog.Logger) *releaseTracker {
	repo = normalizeHomepageRepo(repo)
	if log == nil {
		log = slog.Default()
	}
	return &releaseTracker{
		endpoint: "https://api.github.com/repos/" + repo + "/releases/latest",
		client:   &http.Client{Timeout: releaseFetchTimeout},
		log:      log,
	}
}

// Tag is what the renderer asks for, and it is the whole of the request path's
// involvement: a read lock and a string. "" means "not known", which the
// landing page renders as the page that has always existed.
func (t *releaseTracker) Tag() string {
	if t == nil {
		return ""
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.tag
}

// ReleaseView is what /api/release answers with. It exists because the HOMEPAGE
// ALREADY KNOWS the newest release and an installed launcher does not, and the
// alternative — every player's machine asking GitHub directly — spends the
// anonymous rate limit of whatever network that player is on, on a number this
// host resolved an hour ago and is holding in memory.
//
// TWO SPELLINGS OF ONE FACT, and both are here on purpose. Tag is what GitHub
// publishes and what the page renders, "v" and all, so a reader who follows the
// release link recognises what they land on. Release is the same value with the
// tag's "v" taken off — the form that compares against a launcher's own
// launcher.Release — so no consumer has to know that this project's tags carry a
// prefix and its version constants do not.
//
// NOT KNOWING IS A VALID ANSWER, exactly as it is for the page (release.go rule
// 2): before the first successful lookup, and forever on a host with no route to
// GitHub, both fields are absent and the answer is `{}`. That is a 200 and not
// an error, because "I do not know which release is newest" is the truth and a
// caller that treats it as "nothing to say" behaves correctly.
type ReleaseView struct {
	// Tag is GitHub's own tag, e.g. "v2.5.4". Absent when nothing has resolved.
	// The example is deliberately not this release: nothing in this file may
	// name the release, which is the property release/bump-version.sh checks
	// and the reason the number is resolved at run time at all.
	Tag string `json:"tag,omitempty"`
	// Release is the tag without its "v", e.g. "2.5.4" — the form a launcher
	// compares against its own release constant. Absent for the same reason.
	Release string `json:"release,omitempty"`
}

// View is the whole of the release endpoint's involvement with this tracker: a
// read lock and two strings, so a caller waits on nothing.
func (t *releaseTracker) View() ReleaseView {
	tag := t.Tag()
	if !tagLooksLikeRelease(tag) {
		return ReleaseView{}
	}
	return ReleaseView{Tag: tag, Release: releaseNumberOf(tag)}
}

// releaseNumberOf strips the tag prefix this project publishes under. It is a
// prefix strip and nothing more: a tag that does not carry one is already the
// number, and the value has passed tagLooksLikeRelease before it gets here.
func releaseNumberOf(tag string) string {
	if len(tag) > 1 && (tag[0] == 'v' || tag[0] == 'V') && tag[1] >= '0' && tag[1] <= '9' {
		return tag[1:]
	}
	return tag
}

// validator is the ETag to replay, or "" when nothing has been fetched yet.
func (t *releaseTracker) validator() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.etag
}

func (t *releaseTracker) setTag(tag, etag string) {
	t.mu.Lock()
	changed := t.tag != tag
	t.tag = tag
	t.etag = etag
	t.mu.Unlock()
	if changed {
		// Rare and useful: this line dates the moment the public page started
		// advertising a different release, which is the one thing an operator
		// would want to correlate a support question against.
		t.log.Info("archive: latest release resolved", "tag", tag, "source", t.endpoint)
	}
}

// run is the background refresher. It does one lookup immediately — a restart
// should not leave the page version-less for an hour — and then sleeps for the
// TTL, or for the shorter retry interval while it has never succeeded.
func (t *releaseTracker) run(ctx context.Context) {
	for {
		t.refresh(ctx)
		wait := releaseRefreshInterval
		if t.Tag() == "" {
			wait = releaseRetryInterval
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// refresh performs one lookup. Every failure path is the same failure: the
// cached value is left exactly as it was, and the caller is told nothing,
// because there is nothing a caller could usefully do about it.
func (t *releaseTracker) refresh(ctx context.Context) {
	tag, etag, err := t.fetch(ctx)
	if err != nil {
		// Debug, not Warn: this is a cosmetic lookup against a third party, it
		// is expected to fail sometimes, and the page is designed to be correct
		// when it does. An hourly warning about a working system trains an
		// operator to ignore the log.
		t.log.Debug("archive: latest-release lookup failed", "err", err, "source", t.endpoint)
		return
	}
	if tag == "" {
		// A 304: the release we already named is still the newest one. Nothing
		// to write, and the validator we sent is still the current one.
		return
	}
	t.setTag(tag, etag)
}

// fetch returns the resolved tag and the validator to replay next time. An
// empty tag with a nil error is the "not modified" answer — what we hold is
// still current — and is the ONLY case where no error means no new value.
func (t *releaseTracker) fetch(ctx context.Context) (tag, etag string, err error) {
	ctx, cancel := context.WithTimeout(ctx, releaseFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.endpoint, nil)
	if err != nil {
		return "", "", err
	}
	// The documented media type, and a User-Agent because GitHub rejects a
	// request without one outright.
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "bibites-multiverse-archive/"+Version)
	if v := t.validator(); v != "" {
		req.Header.Set("If-None-Match", v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return "", "", nil
	}
	if resp.StatusCode != http.StatusOK {
		// 403/429 is the rate limiter, 404 is a repository with no published
		// release yet, 5xx is GitHub. All three mean the same thing here.
		return "", "", fmt.Errorf("latest release endpoint answered HTTP %d", resp.StatusCode)
	}
	// A bound on the body, because this is a remote document and an unbounded
	// ReadAll of one is a way to be hurt by a stranger's bug. A release payload
	// is a few kilobytes; a megabyte is generous.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", err
	}
	var doc struct {
		TagName string `json:"tag_name"`
		Draft   bool   `json:"draft"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", "", err
	}
	tag = strings.TrimSpace(doc.TagName)
	// /releases/latest never answers with a draft or a prerelease — it is the
	// newest FULL release, which is exactly the one the download buttons
	// resolve to — but a draft here would mean the two disagree, so it is
	// refused rather than rendered.
	if doc.Draft || !tagLooksLikeRelease(tag) {
		return "", "", errUnusableReleaseTag
	}
	return tag, resp.Header.Get("ETag"), nil
}

var errUnusableReleaseTag = errors.New("the latest release carries no usable tag_name")

// tagLooksLikeRelease is the gate between a remote JSON field and this site's
// HTML. It is deliberately an ALLOW LIST rather than an escape: escaping would
// make a hostile tag safe to render, and this page does not want to render a
// hostile tag safely — it wants to not render it. Everything a real tag is made
// of passes; a space, a bracket, an ampersand or a quote does not.
func tagLooksLikeRelease(tag string) bool {
	if tag == "" || len(tag) > releaseTagMaxLen {
		return false
	}
	digits := false
	for _, r := range tag {
		switch {
		case r >= '0' && r <= '9':
			digits = true
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r == '.', r == '-', r == '_', r == '+':
		default:
			return false
		}
	}
	// A version names a number somewhere. "latest", "stable" and "" are not
	// answers to "which release is this".
	return digits
}
