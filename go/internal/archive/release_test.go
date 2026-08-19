package archive

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// trackerAgainst points a tracker at a test server instead of GitHub. The
// endpoint is a field for exactly this reason: nothing in this package's tests
// reaches the network, and a lookup that could would make the suite depend on
// GitHub's rate limiter.
func trackerAgainst(t *testing.T, h http.Handler) (*releaseTracker, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	tr := newReleaseTracker(defaultHomepageRepo(), testLogger())
	tr.endpoint = ts.URL
	tr.client = ts.Client()
	tr.client.Timeout = releaseFetchTimeout
	return tr, ts
}

func TestReleaseTrackerResolvesTheLatestTag(t *testing.T) {
	var hits int64
	var gotAccept, gotAgent, gotAPIVersion string
	tr, _ := trackerAgainst(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		gotAccept = r.Header.Get("Accept")
		gotAgent = r.Header.Get("User-Agent")
		gotAPIVersion = r.Header.Get("X-GitHub-Api-Version")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","draft":false,"prerelease":false,"name":"9.9.9"}`))
	}))

	if got := tr.Tag(); got != "" {
		t.Fatalf("a tracker that has never looked up answers %q, want the empty string", got)
	}
	tr.refresh(context.Background())
	if got := tr.Tag(); got != "v9.9.9" {
		t.Fatalf("Tag() = %q, want v9.9.9", got)
	}
	if atomic.LoadInt64(&hits) != 1 {
		t.Fatalf("one refresh made %d requests, want 1", atomic.LoadInt64(&hits))
	}
	// GitHub rejects an anonymous request with no User-Agent outright, and the
	// media type is the documented one rather than a guess.
	if gotAccept != "application/vnd.github+json" {
		t.Errorf("Accept = %q, want application/vnd.github+json", gotAccept)
	}
	if !strings.Contains(gotAgent, "bibites-multiverse-archive") {
		t.Errorf("User-Agent = %q, want one naming this program", gotAgent)
	}
	if gotAPIVersion == "" {
		t.Error("the request pins no GitHub API version")
	}
}

// The default endpoint is the public, unauthenticated one for THIS
// repository — the same repository the download buttons resolve against. A
// tracker pointed somewhere else would name a release the buttons do not serve.
func TestReleaseTrackerAddressesTheRepositoryTheButtonsUse(t *testing.T) {
	tr := newReleaseTracker("", nil)
	want := "https://api.github.com/repos/" + defaultHomepageRepo() + "/releases/latest"
	if tr.endpoint != want {
		t.Errorf("default endpoint = %q, want %q", tr.endpoint, want)
	}
	// The same normalization the landing links get, so a repo configured as a
	// URL does not produce a nonsense API path.
	tr = newReleaseTracker("https://github.com/acme/example-bibites/", nil)
	if want := "https://api.github.com/repos/acme/example-bibites/releases/latest"; tr.endpoint != want {
		t.Errorf("configured endpoint = %q, want %q", tr.endpoint, want)
	}
	if tr.client.Timeout <= 0 || tr.client.Timeout > 10*time.Second {
		t.Errorf("lookup timeout = %s, want a short bound", tr.client.Timeout)
	}
}

// STALE BEATS SILENT. Once a tag is known, no later failure may take it away:
// a rate limit, an outage or a garbage body leaves the page naming the release
// it last confirmed, which is at worst one release behind and never wrong about
// having been true.
func TestReleaseTrackerKeepsTheLastGoodTagThroughEveryFailure(t *testing.T) {
	var mode atomic.Value
	mode.Store("ok")
	tr, ts := trackerAgainst(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch mode.Load().(string) {
		case "ok":
			_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
		case "ratelimited":
			w.WriteHeader(http.StatusForbidden)
		case "server-error":
			w.WriteHeader(http.StatusInternalServerError)
		case "notfound":
			w.WriteHeader(http.StatusNotFound)
		case "garbage":
			_, _ = w.Write([]byte("<html>not json at all</html>"))
		case "empty-tag":
			_, _ = w.Write([]byte(`{"tag_name":""}`))
		case "draft":
			_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","draft":true}`))
		case "hostile-tag":
			_, _ = w.Write([]byte(`{"tag_name":"v1\"><script>alert(1)</script>"}`))
		}
	}))

	tr.refresh(context.Background())
	if got := tr.Tag(); got != "v9.9.9" {
		t.Fatalf("Tag() = %q, want v9.9.9", got)
	}
	for _, bad := range []string{"ratelimited", "server-error", "notfound", "garbage", "empty-tag", "draft", "hostile-tag"} {
		mode.Store(bad)
		tr.refresh(context.Background())
		if got := tr.Tag(); got != "v9.9.9" {
			t.Errorf("after a %q answer Tag() = %q, want the last good v9.9.9", bad, got)
		}
	}
	// A dead endpoint — the shape of a host with no route out — is the same.
	ts.Close()
	tr.refresh(context.Background())
	if got := tr.Tag(); got != "v9.9.9" {
		t.Errorf("after a connection failure Tag() = %q, want the last good v9.9.9", got)
	}
}

// ...and a tracker that has NEVER succeeded stays empty rather than inventing
// a placeholder, because "" is what makes the page render as its version-less
// self instead of advertising a guess.
func TestReleaseTrackerThatNeverSucceedsStaysEmpty(t *testing.T) {
	tr, _ := trackerAgainst(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	for i := 0; i < 3; i++ {
		tr.refresh(context.Background())
	}
	if got := tr.Tag(); got != "" {
		t.Errorf("Tag() = %q after only failures, want the empty string", got)
	}
	if got := renderLandingPage(Config{}, tr.Tag()); got != landingPageHTML {
		t.Error("a never-resolved tracker changed the page; it must render the version-less page")
	}
}

// A lookup that hangs must not hang anything. Nothing waits on refresh, so the
// bound is about not accumulating a goroutine and a socket per hour forever.
func TestReleaseLookupIsBoundedByItsOwnTimeout(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	tr, _ := trackerAgainst(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		case <-time.After(30 * time.Second):
		}
	}))
	tr.client.Timeout = 150 * time.Millisecond

	done := make(chan struct{})
	go func() { defer close(done); tr.refresh(context.Background()) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a hanging lookup never returned; the client timeout is not being applied")
	}
	if got := tr.Tag(); got != "" {
		t.Errorf("a timed-out lookup set Tag() to %q", got)
	}
}

// The refresher is a background loop: it resolves once immediately so a restart
// does not leave the page version-less for an hour, and it stops with the
// archive's context rather than outliving it.
func TestReleaseTrackerRefreshesInTheBackgroundAndStopsWithItsContext(t *testing.T) {
	var hits int64
	tr, _ := trackerAgainst(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); tr.run(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	for tr.Tag() == "" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := tr.Tag(); got != "v9.9.9" {
		t.Fatalf("the background loop did not resolve a tag: Tag() = %q", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the release loop outlived its context")
	}
	// One lookup for the run, not a lookup per poll of Tag(): the next one is a
	// whole TTL away, so a page served a thousand times costs GitHub nothing.
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Errorf("the background loop made %d lookups in that window, want 1", got)
	}
	if releaseRefreshInterval < 30*time.Minute || releaseRefreshInterval > 6*time.Hour {
		t.Errorf("the cache TTL is %s; the anonymous API budget is 60 requests an hour", releaseRefreshInterval)
	}
	if releaseRetryInterval < time.Minute || releaseRetryInterval >= releaseRefreshInterval {
		t.Errorf("the pre-first-success retry is %s; it must be shorter than the TTL and still polite", releaseRetryInterval)
	}
}

// SERVING THE PAGE MUST NEVER CALL GITHUB. This is the rule that keeps a third
// party out of the site's critical path, and the only way to hold it is to
// prove the request path makes no request at all.
func TestServingTheLandingPageNeverCallsTheReleaseAPI(t *testing.T) {
	var hits int64
	tr, _ := trackerAgainst(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	tr.refresh(context.Background())

	a := rigShapedArchive(t)
	a.releases = tr
	h := a.httpHandler()
	for i := 0; i < 25; i++ {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("GET / status = %d, want 200", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), `Latest release <b>v9.9.9</b>`) {
			t.Fatal("the served page lost the resolved release")
		}
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Errorf("25 page views made %d release lookups, want the 1 from the background refresh", got)
	}
}

// The lookup is conditional after the first success. A release changes a few
// times a year, so nearly every hourly lookup is a 304: no body to pay for, and
// GitHub does not charge a 304 against the anonymous rate limit at all. What
// matters here is that a 304 is not mistaken for a failure OR for an answer —
// it means "what you have is still current".
func TestReleaseLookupIsConditionalAfterTheFirstSuccess(t *testing.T) {
	const etag = `W/"5e1f0a"`
	var conditional int64
	var unconditional int64
	tr, _ := trackerAgainst(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			atomic.AddInt64(&conditional, 1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		atomic.AddInt64(&unconditional, 1)
		w.Header().Set("ETag", etag)
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))

	tr.refresh(context.Background())
	if got := tr.Tag(); got != "v9.9.9" {
		t.Fatalf("Tag() = %q, want v9.9.9", got)
	}
	if tr.validator() != etag {
		t.Fatalf("the validator was not kept: %q", tr.validator())
	}
	for i := 0; i < 3; i++ {
		tr.refresh(context.Background())
		if got := tr.Tag(); got != "v9.9.9" {
			t.Fatalf("a 304 lost the tag: Tag() = %q", got)
		}
	}
	if got := atomic.LoadInt64(&unconditional); got != 1 {
		t.Errorf("%d lookups sent no validator, want 1 (the first)", got)
	}
	if got := atomic.LoadInt64(&conditional); got != 3 {
		t.Errorf("%d lookups were conditional, want 3", got)
	}
}

// /api/release IS THE NUMBER FOR A PROGRAM, and the reason it exists is that an
// installed launcher must not ask GitHub itself: one host resolves the tag once
// an hour and every launcher asks that host. The two spellings are both here so
// no caller has to know that this project's tags carry a "v" and its version
// constants do not.
func TestReleaseEndpointServesTheResolvedReleaseForAProgram(t *testing.T) {
	a := rigShapedArchive(t)
	h := a.httpHandler()

	get := func() (int, string, string) {
		t.Helper()
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/release", nil))
		return rr.Code, rr.Header().Get("Content-Type"), rr.Body.String()
	}

	// NOT KNOWING IS A 200. Before the first lookup lands, and for ever on a host
	// with no route to GitHub, the answer is an empty document — because "I do
	// not know which release is newest" is the truth, and a caller that reads it
	// as "nothing to say" behaves correctly. An error status here would make a
	// launcher's silent check look like a fault to whoever reads a web log.
	code, contentType, body := get()
	if code != http.StatusOK {
		t.Fatalf("GET /api/release before anything resolved = %d, want 200", code)
	}
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	var unknown ReleaseView
	if err := json.Unmarshal([]byte(body), &unknown); err != nil {
		t.Fatalf("the unresolved answer is not JSON: %v (%q)", err, body)
	}
	if unknown.Tag != "" || unknown.Release != "" {
		t.Fatalf("an unresolved tracker answered %+v", unknown)
	}

	a.releases.setTag("v9.9.9", "")
	code, _, body = get()
	if code != http.StatusOK {
		t.Fatalf("GET /api/release = %d, want 200", code)
	}
	var resolved ReleaseView
	if err := json.Unmarshal([]byte(body), &resolved); err != nil {
		t.Fatalf("the answer is not JSON: %v (%q)", err, body)
	}
	if resolved.Tag != "v9.9.9" {
		t.Errorf("tag = %q, want v9.9.9", resolved.Tag)
	}
	if resolved.Release != "9.9.9" {
		t.Errorf("release = %q, want 9.9.9 - the form a launcher compares against", resolved.Release)
	}

	// A NEW RELEASE MOVES IT WITH NO RESTART, which is the whole reason the tag
	// is resolved rather than compiled in, and is what a launcher installed
	// today will read a year from now.
	a.releases.setTag("v9.9.10", "")
	_, _, body = get()
	if !strings.Contains(body, "9.9.10") {
		t.Errorf("a newly resolved release did not reach the endpoint: %q", body)
	}
}

// The same gate as the page's: this endpoint would rather say nothing than hand
// a program a value it does not recognise as a version.
// It needs a tracker and not a whole archive: the gate is on the value, and
// rigShapedArchive builds a six-world workload this has no use for.
func TestReleaseEndpointRefusesATagItDoesNotRecognise(t *testing.T) {
	tr := newReleaseTracker(defaultHomepageRepo(), testLogger())
	for _, bad := range []string{"", "   ", "latest", "v1 0", "v1<b>"} {
		tr.setTag(bad, "")
		if view := tr.View(); view.Tag != "" || view.Release != "" {
			t.Errorf("a tag of %q was rendered as %+v", bad, view)
		}
	}
	// And a nil tracker — the state a front door must be able to draw — answers
	// the same empty document rather than panicking.
	var none *releaseTracker
	if view := none.View(); view.Tag != "" || view.Release != "" {
		t.Errorf("a tracker that does not exist answered %+v", view)
	}
}

func TestReleaseNumberOf(t *testing.T) {
	// Fictional versions, for the reason the tag in every test above is
	// fictional: release/bump-version.sh refuses a release literal that is not
	// allow-listed, and this function has nothing to do with which release
	// this is.
	tests := []struct{ tag, want string }{
		{"v2.5.4", "2.5.4"},
		{"V2.5.4", "2.5.4"},
		{"2.5.4", "2.5.4"},
		{"v1", "1"},
		{"version9", "version9"},
		{"v", "v"},
	}
	for _, tc := range tests {
		if got := releaseNumberOf(tc.tag); got != tc.want {
			t.Errorf("releaseNumberOf(%q) = %q, want %q", tc.tag, got, tc.want)
		}
	}
}

func TestTagLooksLikeRelease(t *testing.T) {
	for _, ok := range []string{"v9.9.9", "9.9.9", "v1", "v9.9.9-rc1", "2026.08.17", "v9.9.9+build.7", "v9_9_9"} {
		if !tagLooksLikeRelease(ok) {
			t.Errorf("tagLooksLikeRelease(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{
		"", "   ", "latest", "stable", "release",
		`v1"><script>alert(1)</script>`, "v1 0", "v1&amp;", "v1<b>", "v1/../..",
		strings.Repeat("v1.", 20),
	} {
		if tagLooksLikeRelease(bad) {
			t.Errorf("tagLooksLikeRelease(%q) = true, want false", bad)
		}
	}
}
