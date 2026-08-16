package archive

import (
	"bytes"
	"image/jpeg"
	"image/png"
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
		`property="og:image" content="https://bibitesmultiverse.com/social-card.png"`,
		`href="/favicon.svg"`,
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

func TestLandingPageOffersCompletePublicPackages(t *testing.T) {
	defaultLanding := landingPageHTML
	for _, want := range []string{
		"The Windows setup and Linux complete package below are always the newest release.",
		"Each installer creates a unique world identity and keeps its secret on your machine.",
		"No join string is required.",
		`href="https://github.com/jpinedaa/bibites-multiverse/releases/latest/download/bibites-multiverse-windows-x64-setup.exe"`,
		`href="https://github.com/jpinedaa/bibites-multiverse/releases/latest/download/bibites-multiverse-linux-x64-complete.zip"`,
		`href="https://github.com/jpinedaa/bibites-multiverse/releases/latest">Checksums and add-ons`,
		"Automatic enrollment creates the secret on your machine.",
	} {
		if !strings.Contains(defaultLanding, want) {
			t.Errorf("landing page is missing public package detail %q", want)
		}
	}
	if strings.Contains(defaultLanding, "Joining requires a private join string") {
		t.Error("landing page still says that the public map requires a private join string")
	}
}

// The homepage names *The Bibites* throughout, so it also has to say what the
// game is, who made it, and where to buy or download it from the people who
// made it. The absence checks guard the two claims this project cannot make:
// that the game's developer permits or endorses anything here.
func TestLandingPageCreditsTheGameItIsBuiltOn(t *testing.T) {
	for _, want := range []string{
		`id="game"`, "The original game",
		"created by Léo Caussan in 2017 and developed by",
		`href="https://store.steampowered.com/app/2736860/The_Bibites_Digital_Life/"`,
		`href="https://thebibites.itch.io/the-bibites"`,
		`href="https://www.thebibites.com/"`,
		"independent passion project, built out of an interest in artificial life",
		"not affiliated with, endorsed by, or sponsored by Léo Caussan or Omnia Studios",
		"Is this the game's own multiplayer?",
		"The game itself, running on your machine",
	} {
		if !strings.Contains(landingPageHTML, want) {
			t.Errorf("landing page does not credit the game it is built on: missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"developer's permission", "endorsed by <em>The Bibites</em>", "fan project",
	} {
		if strings.Contains(landingPageHTML, forbidden) {
			t.Errorf("landing page makes a claim this project cannot support: %q", forbidden)
		}
	}
}

func TestLandingPageUsesHomepageConfigOverrides(t *testing.T) {
	page := renderLandingPage(Config{
		HomepageRepo:        "acme/example-bibites",
		HomepageGameVersion: "9.9.9",
	})
	for _, want := range []string{
		`<em>The Bibites</em> 9.9.9, the mod, and the connector.`,
		`href="https://github.com/acme/example-bibites/releases/latest/download/bibites-multiverse-windows-x64-setup.exe"`,
		`href="https://github.com/acme/example-bibites/releases/latest/download/bibites-multiverse-linux-x64-complete.zip"`,
		`href="https://github.com/acme/example-bibites/releases/latest">Checksums and add-ons`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("landing page did not apply homepage config: missing %q", want)
		}
	}
}

// The three download links are the whole point of the change that removed the
// release number from this page: they must never name a tag or a versioned
// asset again. GitHub resolves /releases/latest and /releases/latest/download/
// against whichever release is newest, so a release moves this page with no
// rebuild and no deployment — and make-release.sh publishes the two
// stable-named assets these links address.
func TestLandingPageDownloadsNameNoRelease(t *testing.T) {
	for _, want := range []string{
		`href="https://github.com/jpinedaa/bibites-multiverse/releases/latest/download/bibites-multiverse-windows-x64-setup.exe"`,
		`href="https://github.com/jpinedaa/bibites-multiverse/releases/latest/download/bibites-multiverse-linux-x64-complete.zip"`,
		`href="https://github.com/jpinedaa/bibites-multiverse/releases/latest">Checksums and add-ons`,
	} {
		if !strings.Contains(landingPageHTML, want) {
			t.Errorf("landing page is missing the release-independent download link %q", want)
		}
	}
	if strings.Contains(landingPageHTML, "/releases/download/") {
		t.Error("landing page still links a versioned release asset; it must use /releases/latest/download/")
	}
	if strings.Contains(landingPageHTML, "/releases/tag/") {
		t.Error("landing page still links a release tag; it must use /releases/latest")
	}
}

// pngMagic is the eight-byte PNG signature. A card that is served as image/png
// and is not one is worse than no card: the scraper fetches it, fails to decode
// it, and shows the link with a blank frame.
const pngMagic = "\x89PNG\r\n\x1a\n"

// jpegMagic is the three-byte JFIF start-of-image marker, the same kind of
// check as pngMagic for the one raster asset on this site that is not a card.
const jpegMagic = "\xff\xd8\xff"

// The one card size every scraper in the list below crops predictably. The
// og:image:width and og:image:height tags on each page promise exactly this,
// and a card that disagrees with its own tags gets letterboxed or dropped.
const (
	cardWidth  = 1200
	cardHeight = 630
)

// A link preview is read by a scraper, not a browser, and NOT ONE of the
// platforms this project's links travel through — Facebook, WhatsApp, X,
// LinkedIn, Discord, Slack, iMessage — renders SVG. This test is the regression
// guard: every page's card must be raster, must be reachable, and no og: or
// twitter: image tag may point back at a vector.
func TestEveryPageAdvertisesARasterSocialCard(t *testing.T) {
	pages := []struct{ name, html, card, path string }{
		{"landing", landingPageHTML, "https://bibitesmultiverse.com/social-card.png", "/social-card.png"},
		{"watch", watchPageHTML, "https://bibitesmultiverse.com/social-card-watch.png", "/social-card-watch.png"},
		{"live", statusPageHTML, "https://bibitesmultiverse.com/social-card-live.png", "/social-card-live.png"},
	}

	for _, p := range pages {
		for _, want := range []string{
			`<meta property="og:type" content=`,
			`<meta property="og:site_name" content="Bibites Multiverse">`,
			`<meta property="og:title" content=`,
			`<meta property="og:description" content=`,
			`<meta property="og:url" content="https://bibitesmultiverse.com/`,
			`<meta property="og:image" content="` + p.card + `">`,
			`<meta property="og:image:secure_url" content="` + p.card + `">`,
			`<meta property="og:image:type" content="image/png">`,
			`<meta property="og:image:width" content="1200">`,
			`<meta property="og:image:height" content="630">`,
			`<meta property="og:image:alt" content=`,
			`<meta name="twitter:card" content="summary_large_image">`,
			`<meta name="twitter:title" content=`,
			`<meta name="twitter:description" content=`,
			`<meta name="twitter:image" content="` + p.card + `">`,
			`<meta name="twitter:image:alt" content=`,
		} {
			if !strings.Contains(p.html, want) {
				t.Errorf("%s page is missing share metadata %q", p.name, want)
			}
		}
		// Three pages, three cards: a page that borrowed another page's card
		// would make every link look like the same link.
		if got := strings.Count(p.html, p.card); got != 3 {
			t.Errorf("%s page names its own card %d times, want 3 (og:image, og:image:secure_url, twitter:image)",
				p.name, got)
		}
		for _, other := range pages {
			if other.name != p.name && strings.Contains(p.html, other.card) {
				t.Errorf("%s page advertises the %s page's card", p.name, other.name)
			}
		}
		// The regression itself: an image tag pointing at a vector.
		for _, line := range strings.Split(p.html, "\n") {
			if !strings.Contains(line, "og:image") && !strings.Contains(line, "twitter:image") {
				continue
			}
			if strings.Contains(line, ".svg") {
				t.Errorf("%s page still offers a vector to link scrapers: %s", p.name, line)
			}
		}
	}

	a := rigShapedArchive(t)
	h := a.httpHandler()
	for _, p := range pages {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p.path, nil))
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", p.path, rr.Code)
			continue
		}
		if got := rr.Header().Get("Content-Type"); got != "image/png" {
			t.Errorf("GET %s content type = %q, want image/png", p.path, got)
		}
		if got := rr.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=") {
			t.Errorf("GET %s cache control = %q, want a cache lifetime", p.path, got)
		}
		body := rr.Body.Bytes()
		if len(body) == 0 {
			t.Errorf("GET %s served an empty card", p.path)
			continue
		}
		if !bytes.HasPrefix(body, []byte(pngMagic)) {
			t.Errorf("GET %s does not begin with the PNG signature", p.path)
			continue
		}
		// The signature alone is not enough. A 1x1 placeholder is a valid PNG,
		// and the cards were built against placeholders before the artwork
		// existed — so shipping one is a real way for this to silently regress.
		// Every scraper crops to 1.91:1 and most reject anything under 200px a
		// side, so the dimensions are the actual contract, not the bytes.
		cfg, err := png.DecodeConfig(bytes.NewReader(body))
		if err != nil {
			t.Errorf("GET %s did not decode as PNG: %v", p.path, err)
			continue
		}
		if cfg.Width != cardWidth || cfg.Height != cardHeight {
			t.Errorf("GET %s is %dx%d, want %dx%d — a placeholder or a mis-rendered card",
				p.path, cfg.Width, cfg.Height, cardWidth, cardHeight)
		}
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
		{"/social-card.png", "image/png", pngMagic, http.StatusOK},
		{"/social-card-watch.png", "image/png", pngMagic, http.StatusOK},
		{"/social-card-live.png", "image/png", pngMagic, http.StatusOK},
		{"/game-screenshot.jpg", "image/jpeg", jpegMagic, http.StatusOK},
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

// The #game section exists to answer "what am I looking at?" for someone who
// has never heard of the game. It does that with three things that have to
// stay together: a title that names the game's role instead of its history, a
// picture of the game actually running, and a way to get from the hero
// sentence down to the section. Any one of them alone leaves the question
// half-answered, so they are asserted as one contract.
func TestTheGameSectionShowsTheGameAndTheHeroPointsAtIt(t *testing.T) {
	for _, want := range []string{
		"<h2><em>The Bibites</em></h2>",
		"Every world on this map runs a copy of the game.",
		`src="/game-screenshot.jpg"`,
		`href="#game"><em>The Bibites</em></a>`,
	} {
		if !strings.Contains(landingPageHTML, want) {
			t.Errorf("landing page is missing %q", want)
		}
	}
	// The old title told the game's chronology, which reads as a credit line
	// rather than an explanation. It should not come back.
	if strings.Contains(landingPageHTML, "came first") {
		t.Error("landing page still carries the retired chronology title")
	}
	// Its replacement described the game instead of naming it. The section is
	// the game's own, so the title is the game's name and nothing else.
	if strings.Contains(landingPageHTML, "The game behind every world") {
		t.Error("landing page still carries the retired descriptive title")
	}

	a := rigShapedArchive(t)
	rr := httptest.NewRecorder()
	a.httpHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/game-screenshot.jpg", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /game-screenshot.jpg status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("GET /game-screenshot.jpg content type = %q, want image/jpeg", got)
	}
	if got := rr.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=") {
		t.Errorf("GET /game-screenshot.jpg cache control = %q, want a cache lifetime", got)
	}
	// The markup declares 1280x720 so the browser reserves the box before the
	// lazy image lands; a frame of another size would reflow the section.
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(rr.Body.Bytes()))
	if err != nil {
		t.Fatalf("GET /game-screenshot.jpg did not decode as JPEG: %v", err)
	}
	if cfg.Width != 1280 || cfg.Height != 720 {
		t.Errorf("GET /game-screenshot.jpg is %dx%d, want 1280x720 — the size the markup reserves",
			cfg.Width, cfg.Height)
	}
}
