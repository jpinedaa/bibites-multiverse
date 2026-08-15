package archive

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWatchPageIsOneSharedReadOnlyBroadcast(t *testing.T) {
	for _, want := range []string{
		`<title>Bibites Multiverse — Watch Broadcast</title>`,
		`href="/watch" aria-current="page">Watch broadcast</a>`,
		"youngest living Bibite",
		"No switching on new births",
		"Death or departure",
		`/stream/bibites/index.m3u8`,
		`/stream/bibites?controls=true`,
		`response.status===429&&loaded`,
		`frame.contentDocument.querySelector("video")`,
		"spectator camera, not a control surface",
	} {
		if !strings.Contains(watchPageHTML, want) {
			t.Fatalf("watch page does not contain %q", want)
		}
	}

	for _, forbidden := range []string{"streamKey", "publish-password", "rtmp://", "rtmps://"} {
		if strings.Contains(watchPageHTML, forbidden) {
			t.Fatalf("watch page exposes broadcast control material %q", forbidden)
		}
	}

	if strings.Contains(watchPageHTML, "allowfullscreen") {
		t.Fatal("watch page contains a redundant allowfullscreen attribute")
	}
}

func TestWatchRouteAndUnknownChild(t *testing.T) {
	a := rigShapedArchive(t)
	h := a.httpHandler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/watch", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /watch status = %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("GET /watch content type = %q", got)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/watch/anything", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET /watch/anything status = %d, want 404", rr.Code)
	}
}
