package launcher

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// THE COMPARISON IS THE WHOLE FEATURE. Everything else about the update check is
// "and then nothing happens"; this is the one function whose wrong answer a
// participant sees, and its two failure modes are not symmetric. A false
// negative costs somebody a line on a menu. A false positive tells EVERY
// participant to go and reinstall the version they already have, from a window
// they opened to start a world.
func TestNewerRelease(t *testing.T) {
	// THE VERSIONS BELOW ARE DELIBERATELY FICTIONAL, for the same reason the
	// archive's release tests use a made-up tag: release/bump-version.sh refuses
	// a tree in which the release literal appears anywhere it has not been
	// allow-listed, and a table pinned to whatever this release happens to be
	// would have to be edited by every bump — for a function that has nothing to
	// do with which release this is.
	tests := []struct {
		name      string
		current   string
		published string
		want      string
	}{
		{"a newer patch", "2.5.4", "2.5.5", "2.5.5"},
		{"a newer minor", "2.5.4", "2.6.0", "2.6.0"},
		{"a newer major", "2.5.4", "3.0.0", "3.0.0"},
		{"the tag's v is trimmed on the way in and out", "2.5.4", "v2.5.5", "2.5.5"},
		{"a capital V too", "2.5.4", "V2.5.5", "2.5.5"},
		{"the same release says nothing", "2.5.4", "2.5.4", ""},
		{"the same release wearing a v says nothing", "2.5.4", "v2.5.4", ""},
		{"an older release says nothing", "2.5.4", "2.5.3", ""},
		{"an older minor says nothing", "2.6.0", "2.5.9", ""},
		{"a missing component is a zero, so 2.6 is not newer than 2.6.0", "2.6.0", "2.6", ""},
		{"and 2.6.1 is newer than 2.6", "2.6", "2.6.1", "2.6.1"},
		{"a two-digit component is a number, not a character", "2.9.0", "2.10.0", "2.10.0"},
		{"and the same in the other direction", "2.10.0", "2.9.0", ""},
		{"nothing published says nothing", "2.5.4", "", ""},
		{"whitespace is not a version", "2.5.4", "   ", ""},
		{"a word is not a version", "2.5.4", "latest", ""},
		{"a pre-release suffix is refused rather than guessed at", "2.5.4", "2.5.5-rc1", ""},
		{"so is a build tag", "2.5.4", "2.5.5+build7", ""},
		{"a negative component is refused", "2.5.4", "2.5.-1", ""},
		{"more components than a release has is refused", "2.5.4", "2.5.4.1.1", ""},
		{"a four-part version still compares", "2.5.4", "2.5.4.1", "2.5.4.1"},
		{"a current version this build could not have is refused", "", "9.9.9", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NewerRelease(tc.current, tc.published)
			if tc.want == "" {
				if ok || got != "" {
					t.Fatalf("NewerRelease(%q, %q) = (%q, %v), want no answer",
						tc.current, tc.published, got, ok)
				}
				return
			}
			if !ok || got != tc.want {
				t.Fatalf("NewerRelease(%q, %q) = (%q, %v), want (%q, true)",
					tc.current, tc.published, got, ok, tc.want)
			}
		})
	}
}

// THE RELEASE THIS BUILD IS is one side of every comparison a participant meets,
// so the check must be silent about it. A launcher that nagged about its own
// release is the one bug this feature could ship that everybody would see.
func TestThisReleaseIsNeverNewerThanItself(t *testing.T) {
	for _, published := range []string{Release, "v" + Release, " " + Release + " "} {
		if got, ok := NewerRelease(Release, published); ok {
			t.Fatalf("this build (%s) treats %q as an update to %q", Release, published, got)
		}
	}
}

func TestUpdateNoticeNamesBothReleasesAndTheAddress(t *testing.T) {
	if got := UpdateNotice(""); got != "" {
		t.Fatalf("nothing to say reads %q", got)
	}
	notice := UpdateNotice("9.9.9")
	for _, want := range []string{"9.9.9", Release, HomeURL} {
		if !strings.Contains(notice, want) {
			t.Errorf("the notice %q does not name %q", notice, want)
		}
	}
}

// The address is built from the homepage's own, so the two cannot drift into
// naming different hosts, and it is https because the answer decides what a
// participant is told to install.
func TestTheCheckAddressesTheHomepage(t *testing.T) {
	if !strings.HasPrefix(ReleaseCheckURL, HomeURL+"/") {
		t.Fatalf("the check address %q is not on the homepage %q", ReleaseCheckURL, HomeURL)
	}
	if !strings.HasPrefix(HomeURL, "https://") {
		t.Fatalf("the homepage address %q is not https", HomeURL)
	}
}

// THE REQUEST SAYS NOTHING ABOUT WHO IS ASKING. No query, no body, no cookie,
// and a User-Agent that names the program and NOT its release — a version in it
// would make "how many machines run which release" answerable from a web log,
// which is a thing this project does not collect.
func TestTheLookupDisclosesNothingButThatItIsALauncher(t *testing.T) {
	var gotMethod, gotQuery, gotAgent, gotCookie string
	var gotBody int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotQuery = r.URL.RawQuery
		gotAgent = r.Header.Get("User-Agent")
		gotCookie = r.Header.Get("Cookie")
		gotBody = r.ContentLength
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag":"v9.9.9","release":"9.9.9"}`))
	}))
	defer ts.Close()

	got, ok := fetchPublishedRelease(ts.Client(), ts.URL)
	if !ok || got != "9.9.9" {
		t.Fatalf("the lookup answered (%q, %v), want (\"9.9.9\", true)", got, ok)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("the lookup used %s, want GET", gotMethod)
	}
	if gotQuery != "" {
		t.Errorf("the request carries a query string: %q", gotQuery)
	}
	if gotCookie != "" {
		t.Errorf("the request carries a cookie: %q", gotCookie)
	}
	if gotBody > 0 {
		t.Errorf("the request carries a body of %d bytes", gotBody)
	}
	if !strings.Contains(gotAgent, "bibites-multiverse-launcher") {
		t.Errorf("User-Agent = %q, want one naming this program", gotAgent)
	}
	if strings.Contains(gotAgent, Release) {
		t.Errorf("User-Agent = %q, which carries this machine's release", gotAgent)
	}
}

// The tag alone is enough. The homepage sends both spellings, but a caller that
// only ever saw the tag still compares correctly, because NewerRelease trims it.
func TestTheLookupFallsBackToTheTag(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag":"v9.9.9"}`))
	}))
	defer ts.Close()
	got, ok := fetchPublishedRelease(ts.Client(), ts.URL)
	if !ok || got != "v9.9.9" {
		t.Fatalf("the lookup answered (%q, %v), want (\"v9.9.9\", true)", got, ok)
	}
	if newer, ok := NewerRelease("2.5.4", got); !ok || newer != "9.9.9" {
		t.Fatalf("a tag-only answer compared to (%q, %v)", newer, ok)
	}
}

// EVERY FAILURE IS THE SAME FAILURE, and none of them is ever reported to
// anybody: the participant opened this program to start a world.
func TestEveryFailedLookupIsSilentAndEmpty(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"a server error", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "no", http.StatusInternalServerError)
		}},
		{"a rate limit", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "no", http.StatusTooManyRequests)
		}},
		{"a captive portal answering HTML", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>sign in to this hotel's wifi</html>"))
		}},
		{"an empty answer", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{}`))
		}},
		{"a homepage that has resolved nothing itself", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tag":"","release":""}`))
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.handler)
			defer ts.Close()
			if got, ok := fetchPublishedRelease(ts.Client(), ts.URL); ok || got != "" {
				t.Fatalf("answered (%q, %v), want nothing", got, ok)
			}
		})
	}

	// And the one nobody can serve: an address that answers at all.
	if got, ok := fetchPublishedRelease(&http.Client{Timeout: time.Second},
		"http://127.0.0.1:0/api/release"); ok || got != "" {
		t.Fatalf("an unreachable address answered (%q, %v)", got, ok)
	}
	if got, ok := fetchPublishedRelease(&http.Client{}, "://not a url"); ok || got != "" {
		t.Fatalf("a malformed address answered (%q, %v)", got, ok)
	}
}

// THE OFF SWITCH AND THE TEST SEAM ARE TWO KNOBS, because the environment
// arrives as a plain getenv that cannot tell "set to nothing" from "not set" —
// so an off switch spelled as an empty value would be an off switch that
// silently did nothing.
func TestTheCheckAddressCanBeMovedOrTurnedOff(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"unset is the built-in address", map[string]string{}, ReleaseCheckURL},
		{"an empty value is not an off switch, it is an unset variable",
			map[string]string{ReleaseCheckURLEnv: "", NoUpdateCheckEnv: ""}, ReleaseCheckURL},
		{"set is that address", map[string]string{ReleaseCheckURLEnv: "http://127.0.0.1:9/x"},
			"http://127.0.0.1:9/x"},
		{"the off switch makes no request at all",
			map[string]string{NoUpdateCheckEnv: "1"}, ""},
		{"and it wins over an address", map[string]string{
			NoUpdateCheckEnv: "yes", ReleaseCheckURLEnv: "http://127.0.0.1:9/x"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(name string) string { return tc.env[name] }
			if got := releaseCheckURL(getenv); got != tc.want {
				t.Fatalf("releaseCheckURL = %q, want %q", got, tc.want)
			}
		})
	}
}

// A watch that was never started, and a watch whose lookup has not landed, both
// say nothing — which is what every reader draws. There is no third state and no
// way to ask whether the lookup failed, because there is nothing anybody could
// do with the answer.
func TestAWatchThatAsksNothingSaysNothing(t *testing.T) {
	var nilWatch *updateWatch
	if got := nilWatch.Available(); got != "" {
		t.Fatalf("a watch that does not exist answers %q", got)
	}
	off := startUpdateWatch(func(name string) string {
		if name == NoUpdateCheckEnv {
			return "1"
		}
		return ""
	})
	if got := off.Available(); got != "" {
		t.Fatalf("a watch pointed at nowhere answers %q", got)
	}
	if got := UpdateNotice(off.Available()); got != "" {
		t.Fatalf("and it renders as %q", got)
	}
}

// The watch keeps only what is NEWER. An answer naming this build, or an older
// one, leaves it empty — the console menu and the window both draw whatever is
// there without a second opinion, so the filter has to be here.
func TestAWatchKeepsOnlyANewerRelease(t *testing.T) {
	for _, published := range []string{Release, "0.0.1", "not-a-version", ""} {
		w := &updateWatch{}
		w.record(published)
		if got := w.Available(); got != "" {
			t.Fatalf("a watch told %q answers %q", published, got)
		}
	}
	w := &updateWatch{}
	w.record("v99.0.0")
	if got := w.Available(); got != "99.0.0" {
		t.Fatalf("a watch told v99.0.0 answers %q", got)
	}
}

// THE MENU FRAME. The line is drawn in the frame the menu redraws, not printed
// over whatever the participant is reading — and it is absent, entirely, until
// something is known. A frame that reserved a blank row for it would move every
// choice below it down by one for a state this program is in almost always.
func TestTheMenuFrameCarriesAnUpdateOnlyWhenThereIsOne(t *testing.T) {
	h := newHarness(t)
	p := h.profile("default", "Multiverse", 8787)
	a := &app{
		install: h.install(),
		stdout:  &h.stdout,
		stderr:  &h.stderr,
		now:     h.clock,
		getenv:  h.getenv,
	}

	// Nothing known: this is the state on a machine with no route out, and it is
	// the state every frame is in for the first moments of every run.
	a.renderMenu(p)
	if strings.Contains(h.out(), "is available") {
		t.Fatalf("a frame that knows nothing carries an update line:\n%s", h.out())
	}

	a.updates = &updateWatch{}
	a.updates.record("99.0.0")
	h.stdout.Reset()
	a.renderMenu(p)
	frame := h.out()
	for _, want := range []string{"Version 99.0.0 is available", Release, HomeURL} {
		if !strings.Contains(frame, want) {
			t.Fatalf("the frame does not name %q:\n%s", want, frame)
		}
	}
	// And it has not displaced the menu.
	for _, want := range []string{"1) Start this world", "0) Quit"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("the frame lost %q:\n%s", want, frame)
		}
	}
}

// NOTHING WAITS ON IT. startUpdateWatch returns before the request does, which
// is the property that keeps a slow network off the path between a shortcut and
// a world.
func TestStartingTheWatchDoesNotWaitForTheAnswer(t *testing.T) {
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = w.Write([]byte(`{"release":"99.0.0"}`))
	}))
	defer ts.Close()
	defer close(release)

	start := time.Now()
	watch := startUpdateWatch(func(name string) string {
		if name == ReleaseCheckURLEnv {
			return ts.URL
		}
		return ""
	})
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("starting the watch took %s; it must not wait on the request", elapsed)
	}
	if got := watch.Available(); got != "" {
		t.Fatalf("the watch answered %q before the server did", got)
	}
}
