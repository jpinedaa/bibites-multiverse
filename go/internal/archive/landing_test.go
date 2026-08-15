package archive

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicFrontDoorAndLiveConsoleHaveSeparateJobs(t *testing.T) {
	for _, want := range []string{
		"Evolution has a map.", `href="/live"`, `fetch("/api/status"`,
		"Aug 14–Nov 14, 2026", "Independent worlds", "Real migration", "Shared history",
		`rel="canonical" href="https://bibitesmultiverse.com/"`,
		`property="og:image"`, `href="/favicon.svg"`,
	} {
		if !strings.Contains(landingPageHTML, want) {
			t.Fatalf("the public front door is missing %q", want)
		}
	}
	// Old links were fragments on /. A server never receives a fragment, so the
	// landing document itself carries the compatibility handoff to /live.
	if !strings.Contains(landingPageHTML, `location.replace("/live" + location.hash)`) {
		t.Fatal("old #map/#species/#settings links no longer reach the console")
	}
	// The landing page may link out, but it still owns its rendering: no CDN,
	// remote font, stylesheet, or executable script is needed to introduce the
	// project or show the live snapshot.
	for _, forbidden := range []string{`<script src=`, `<link rel="stylesheet"`, "@import", "//cdn"} {
		if strings.Contains(landingPageHTML, forbidden) {
			t.Fatalf("the public front door depends on an external rendering asset: %q", forbidden)
		}
	}

	for _, want := range []string{
		`<title>Bibites Multiverse — Live Map</title>`, `href="/" aria-label="Bibites Multiverse home"`,
		`id="tab-map"`, `aria-controls="p-map"`, `aria-labelledby="tab-map"`,
		`ev.key === "ArrowRight"`, "The map is online and waiting for worlds.",
	} {
		if !strings.Contains(statusPageHTML, want) {
			t.Fatalf("the live console is missing %q", want)
		}
	}
}

func TestPublicLiveSurfacesShareOneVisualLanguage(t *testing.T) {
	pages := map[string]string{
		"landing": landingPageHTML,
		"map":     statusPageHTML,
		"watch":   watchPageHTML,
	}
	for name, page := range pages {
		for _, want := range []string{
			`--bg:#0b1110`, `--line:#294038`, `--text:#eff7f3`,
			`#66e0ac`, `#75bdf2`, `#e86c76`,
			`class="brand" href="/" aria-label="Bibites Multiverse home"`,
			`<span>Bibites Multiverse</span>`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("%s page is missing shared visual element %q", name, want)
			}
		}
		for _, want := range []string{
			`>How it works</a>`, `>Join</a>`, `>Watch broadcast</a>`,
			`href="https://github.com/jpinedaa/bibites-multiverse">GitHub</a>`, `>Live map`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("%s page is missing shared navigation link %q", name, want)
			}
		}
		for _, old := range []string{`>Watch live</a>`, `>About</a>`, `>Species &amp; lineages</a>`} {
			if strings.Contains(page, old) {
				t.Errorf("%s page still contains retired navigation label %q", name, old)
			}
		}
	}

	for _, want := range []string{
		`class="console-summary"`, `aria-label="Current map status"`,
		`href="/watch">Watch broadcast</a>`, `aria-current="page"`,
	} {
		if !strings.Contains(statusPageHTML, want) {
			t.Errorf("live map is missing integrated navigation or status element %q", want)
		}
	}
	for _, want := range []string{
		`id="statusdot"`, `@keyframes statuspulse`,
		`className="statusdot"+(linked?" ok"`, `className="statusdot down"`,
	} {
		if !strings.Contains(landingPageHTML, want) {
			t.Errorf("landing page is missing live status indicator %q", want)
		}
	}
	for _, old := range []string{`--bg:#101215`, `--panel:#191d22`, `--line:#2a3038`} {
		if strings.Contains(statusPageHTML, old) {
			t.Errorf("live map still contains its retired standalone palette %q", old)
		}
	}
}

func TestLandingCardLayoutKeepsRelatedContentTogether(t *testing.T) {
	for _, want := range []string{
		`.section{padding-block:64px 0}`,
		`.section:last-child{padding-bottom:64px}`,
		`.principle h3{font-size:22px;margin:0 0 10px}`,
		`.flowitem b{display:block;margin:0 0 7px}`,
		`.faq{display:grid;grid-template-columns:1fr 1fr;align-items:start`,
	} {
		if !strings.Contains(landingPageHTML, want) {
			t.Errorf("landing page is missing compact independent card layout %q", want)
		}
	}
	if strings.Contains(landingPageHTML, `class="num"`) {
		t.Error("landing cards still contain decorative numbers")
	}
}

func TestPublicWebsiteRoutesAndAssets(t *testing.T) {
	a := rigShapedArchive(t)
	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)

	tests := []struct {
		path, contentType, contains string
		status                      int
	}{
		{"/", "text/html", "Evolution has a map.", http.StatusOK},
		{"/live", "text/html", "species", http.StatusOK},
		{"/watch", "text/html", "Follow a life in progress.", http.StatusOK},
		{"/favicon.svg", "image/svg+xml", "#66e0ac", http.StatusOK},
		{"/social-card.svg", "image/svg+xml", "Evolution", http.StatusOK},
		{"/robots.txt", "text/plain", "Sitemap: https://bibitesmultiverse.com/sitemap.xml", http.StatusOK},
		{"/sitemap.xml", "application/xml", "https://bibitesmultiverse.com/live", http.StatusOK},
		{"/nothing-here", "text/html", "This world is not on the map.", http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			resp, err := http.Get(ts.URL + tc.path)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.status)
			}
			if !strings.Contains(resp.Header.Get("Content-Type"), tc.contentType) {
				t.Fatalf("Content-Type = %q, want %q", resp.Header.Get("Content-Type"), tc.contentType)
			}
			if !strings.Contains(string(body), tc.contains) {
				t.Fatalf("body does not contain %q", tc.contains)
			}
		})
	}

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(ts.URL + "/map")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "/live" {
		t.Fatalf("legacy /map = HTTP %d to %q, want 301 to /live",
			resp.StatusCode, resp.Header.Get("Location"))
	}
}
