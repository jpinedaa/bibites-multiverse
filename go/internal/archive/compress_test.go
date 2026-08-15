package archive

// compress.go's tests: the negotiation, the headers, and the one property that
// makes a transport change safe to make at all — THE BYTES ARE THE SAME BYTES.
// wp3_hosting_options.md's egress finding is what these exist for, so the ratio
// is asserted here too: a wrapper that negotiated correctly and saved nothing
// would pass every other test in this file.

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/wire"
)

// rigShapedArchive builds the shape wp3 measured on the living deployment: six
// live worlds in a 3×2 map, each reporting a census of several species, a
// minute of crossings in the hop feed, and enough samples in the metrics file
// for both history endpoints to answer. The point is a payload of realistic
// SIZE and realistic REPETITION — an endpoint's compression ratio is a fact
// about the shape of its JSON, and a one-slot fixture would not have it.
func rigShapedArchive(t *testing.T) *Archive {
	t.Helper()
	generic := []string{"Izus", "Cyanea", "Copedylanus", "Vulpes", "Achlys", "Nereus"}
	specific := []string{"velox", "copedylanus", "borealis", "profunda", "gracilis", "obscura"}

	var slots []contractb.SlotInfo
	for i := 0; i < 6; i++ {
		var entries []contractb.CensusEntry
		for j := 0; j < 8; j++ {
			entries = append(entries, entry(
				generic[(i+j)%len(generic)],
				specific[(i*j+j)%len(specific)]+string(rune('a'+j)),
				11+i*3+j, j%3))
		}
		st := census(60+i*7, 4+i, entries...)
		st.CustodyDepth = contractb.IntPtr(i % 3)
		st.PacedDepth = contractb.IntPtr(0)
		st.HeldDepth = contractb.IntPtr(0)
		st.BouncedTimeoutTotal = contractb.IntPtr(0)
		st.MigrationExclude = &wire.ExcludeList{Names: []string{"Nereus obscura"}}
		slots = append(slots, slot(i+1, i%3, i/3, true, st))
	}
	status := contractb.PeerStatus{
		Epoch: 41, Map: contractb.MapShape{Width: 3, Height: 2}, SlotCount: 6, Slots: slots,
	}
	a := newViewFixture(t, status, time.Second)

	now := time.Now().UnixMilli()
	a.mu.Lock()
	for i := 0; i < 40; i++ {
		a.observeHopLocked(Hop{
			MigrationID: "m" + strings.Repeat("0", 3) + string(rune('a'+i%26)),
			AtMs:        now - int64(i)*1_000,
			FromSlot:    i%6 + 1, ToSlot: (i+1)%6 + 1,
			ExitEdge: contracta.EdgeE,
			Species: &wire.Species{
				GenericName:  generic[i%len(generic)],
				SpecificName: specific[i%len(specific)],
			},
		})
	}
	a.mu.Unlock()

	pops := []*int{ip(60), ip(67), ip(74), ip(81), ip(88), ip(95)}
	live := []bool{true, true, true, true, true, true}
	for i := 0; i < 8; i++ {
		s := sample(now-int64(7-i)*60_000, 900+i*40, pops, live)
		if err := a.metrics.Append(s); err != nil {
			t.Fatalf("metrics.Append: %v", err)
		}
	}
	return a
}

// rawGet makes ONE request with exactly the Accept-Encoding asked for — no
// header at all when it is empty — and hands back the bytes as they arrived on
// the wire. DisableCompression is the whole point: net/http would otherwise
// negotiate for itself and decompress before this test could see either header.
func rawGet(t *testing.T, url, acceptEncoding string) (*http.Response, []byte) {
	t.Helper()
	tr := &http.Transport{DisableCompression: true}
	t.Cleanup(tr.CloseIdleConnections)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request %s: %v", url, err)
	}
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s answered HTTP %d: %s", url, resp.StatusCode, body)
	}
	return resp, body
}

// sameAnswer compares two readings of a LIVE endpoint. Every run of digits
// collapses to one mark first, because the two readings are milliseconds apart
// and a frame that carries its own generatedAtMs is supposed to differ there.
// Everything else — every key, every name, every byte of structure and order —
// has to be identical. The numbers themselves are covered byte for byte by the
// two tests below, which compress a SINGLE payload rather than two readings.
var digitRun = regexp.MustCompile(`[0-9]+`)

func sameAnswer(a, b []byte) bool {
	return string(digitRun.ReplaceAll(a, []byte("#"))) ==
		string(digitRun.ReplaceAll(b, []byte("#")))
}

func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("the body is labelled gzip and is not gzip: %v", err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("the gzip stream is truncated or corrupt: %v", err)
	}
	if err := zr.Close(); err != nil {
		t.Fatalf("the gzip stream does not close cleanly: %v", err)
	}
	return out
}

// TestEveryServedSurfaceIsGzippedExactlyWhenAsked is the wrapper's contract over
// the whole mux at once: the page, the five JSON endpoints, and nothing left
// out. For each surface it asserts both halves of a negotiation —
//
//	ASKED: Content-Encoding: gzip, a body that really is a gzip stream, the
//	handler's own Content-Type and Cache-Control untouched, and fewer bytes on
//	the wire than the identity answer.
//
//	NOT ASKED: no Content-Encoding at all, and the identical bytes this archive
//	served before the wrapper existed.
//
// Vary is asserted on BOTH, because the header is a statement about how the
// answer was chosen and not about which way the choice went.
func TestEveryServedSurfaceIsGzippedExactlyWhenAsked(t *testing.T) {
	a := rigShapedArchive(t)
	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)

	for _, path := range []string{"/", "/live", "/api/status", "/api/hops", "/api/species",
		"/api/species/tree", "/api/species/history?key=Izus+velox", "/api/history"} {
		t.Run(path, func(t *testing.T) {
			plainResp, plain := rawGet(t, ts.URL+path, "")
			if enc := plainResp.Header.Get("Content-Encoding"); enc != "" {
				t.Fatalf("a client that asked for nothing was sent %q; identity is what a "+
					"reader that says nothing gets, or curl and every shell probe in e2e/ "+
					"holds bytes it cannot read", enc)
			}
			if !strings.Contains(plainResp.Header.Get("Vary"), "Accept-Encoding") {
				t.Fatal("the identity answer carries no Vary: a cache that stores it will " +
					"replay it to a client that did ask, and the other way round")
			}

			zipResp, zipped := rawGet(t, ts.URL+path, "gzip")
			if zipResp.Header.Get("Content-Encoding") != "gzip" {
				t.Fatalf("a client that asked for gzip was sent %q",
					zipResp.Header.Get("Content-Encoding"))
			}
			if !strings.Contains(zipResp.Header.Get("Vary"), "Accept-Encoding") {
				t.Fatal("the compressed answer carries no Vary")
			}
			// The negotiation is TRANSPORT. Everything the handler said about
			// its own body still holds.
			if got, want := zipResp.Header.Get("Content-Type"),
				plainResp.Header.Get("Content-Type"); got != want {
				t.Fatalf("compression changed the content type: %q, want %q", got, want)
			}
			if !strings.Contains(zipResp.Header.Get("Cache-Control"), "no-store") {
				t.Fatalf("compression lost Cache-Control: %q",
					zipResp.Header.Get("Cache-Control"))
			}
			// Content-Length may not survive: it measured the body before this.
			if cl := zipResp.Header.Get("Content-Length"); cl != "" &&
				cl != strconv.Itoa(len(zipped)) {
				t.Fatalf("Content-Length %s describes the uncompressed body, not the %d "+
					"bytes actually sent", cl, len(zipped))
			}

			if len(zipped) >= len(plain) {
				t.Fatalf("gzip made %s bigger, not smaller: %d bytes against %d",
					path, len(zipped), len(plain))
			}
			// And it is still the same answer.
			body := gunzip(t, zipped)
			if !sameAnswer(body, plain) {
				t.Fatalf("the decompressed body is not the answer the identity request "+
					"got (%d bytes against %d)", len(body), len(plain))
			}
			if strings.HasPrefix(path, "/api/") {
				var v any
				if err := json.Unmarshal(body, &v); err != nil {
					t.Fatalf("the decompressed body is not the JSON that went in: %v", err)
				}
			}
			t.Logf("%s: %d B identity, %d B gzip (%.1f×)", path, len(plain), len(zipped),
				float64(len(plain))/float64(len(zipped)))
		})
	}
}

// TestTheServedPageSurvivesCompressionByteForByte is the exactness half, on the
// one surface where exactness is free to assert: the page is a constant, so the
// bytes a browser reconstructs can be compared against the constant itself
// rather than against a second reading of a living system.
//
// It is also the biggest single body this archive serves — 140 KB of HTML, CSS
// and JavaScript on every first load — and the one nobody counted, because the
// measurement was aimed at the polls.
func TestTheServedPageSurvivesCompressionByteForByte(t *testing.T) {
	a := rigShapedArchive(t)
	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)

	_, zipped := rawGet(t, ts.URL+"/live", "gzip")
	if got := string(gunzip(t, zipped)); got != statusPageHTML {
		t.Fatalf("the page a browser reconstructs is not the page this archive holds "+
			"(%d bytes against %d)", len(got), len(statusPageHTML))
	}
	// THE FLOOR IS AIMED AT THE FAILURE MODE, NOT AT THE PAGE'S PROSE. What it
	// catches is a body that stopped compressing at all — one that turned binary,
	// or went out double-encoded — and that lands at or below 1.0×. The page's own
	// ratio is not a constant to pin: markup, CSS and identifiers compress far
	// better than English, and this page explains itself in English, so every
	// commentary a change adds walks the average DOWN. Measured: the page was at
	// 3.011× with about 800 B of room left under a floor of 3, and the focus fix's
	// commentary alone spent it. A floor with no headroom is a tripwire on the next
	// paragraph somebody writes rather than a test of the wrapper.
	ratio := float64(len(statusPageHTML)) / float64(len(zipped))
	t.Logf("the page: %d B identity, %d B gzip (%.2f×)", len(statusPageHTML), len(zipped), ratio)
	if ratio < 2 {
		t.Fatalf("the page compresses %.2f×; a document this repetitive that will not "+
			"compress means the wrapper is shipping something other than the page", ratio)
	}
}

// TestTheStatusFrameCompressesAsMeasured is wp3_hosting_options.md's number,
// pinned. The rig measured /api/status at 19,705 B and 2,254 B gzipped — 8.7×,
// with gzip -9 — and that ratio is the whole argument for this wrapper: it is
// what turns ~32 GB/month per open tab into ~4 GB.
//
// The floor is deliberately well under the measurement rather than at it. This
// fixture is a smaller map than the rig's and the wrapper compresses at the
// default level rather than -9, so an exact number here would be a test of the
// fixture. A frame that stopped compressing at all — a payload that turned
// binary, or a wrapper that started double-encoding — is what this catches.
//
// The exact bytes are checked on the SAME body: the frame is read off the live
// endpoint once and served back through the wrapper, so what is compared is one
// payload before and after, with no second reading of a clock in between.
func TestTheStatusFrameCompressesAsMeasured(t *testing.T) {
	a := rigShapedArchive(t)
	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)

	_, frame := rawGet(t, ts.URL+"/api/status", "")
	if len(frame) < gzipMinBytes {
		t.Fatalf("the fixture's status frame is %d bytes, under the %d-byte floor; it is "+
			"not the shape wp3 measured and this test would prove nothing", len(frame),
			gzipMinBytes)
	}

	echo := httptest.NewServer(gzipped(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(frame)
		})))
	t.Cleanup(echo.Close)

	_, zipped := rawGet(t, echo.URL, "gzip")
	if !bytes.Equal(gunzip(t, zipped), frame) {
		t.Fatal("a status frame does not survive the round trip byte for byte")
	}
	ratio := float64(len(frame)) / float64(len(zipped))
	if ratio < 4 {
		t.Fatalf("the status frame compressed only %.1f×; the rig measured 8.7× and the "+
			"hosting arithmetic is built on it", ratio)
	}
	t.Logf("status frame: %d B → %d B (%.1f×)", len(frame), len(zipped), ratio)
}

// TestAcceptEncodingIsNegotiatedAndNotAssumed walks the header field itself.
// Every one of these is something a real client sends, and two of them are the
// ones a naive strings.Contains(header, "gzip") gets WRONG: "gzip;q=0" is a
// refusal, and a client that offers only brotli must not be handed a coding it
// never named.
func TestAcceptEncodingIsNegotiatedAndNotAssumed(t *testing.T) {
	a := rigShapedArchive(t)
	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)

	for _, tc := range []struct {
		header string
		want   bool
	}{
		{"", false},
		{"gzip", true},
		{"gzip, deflate", true},
		{"gzip, deflate, br, zstd", true}, // what every browser actually sends
		{"deflate, gzip;q=0.5", true},
		{" GZIP ", true}, // the token is case-insensitive
		{"gzip;q=0", false},
		{"gzip;q=0.0", false},
		{"br", false},
		{"identity", false},
		{"*", true},
		{"*;q=0", false},
		{"*, gzip;q=0", false}, // the explicit refusal beats the wildcard
		{"deflate;q=1.0, *;q=0.5", true},
	} {
		resp, body := rawGet(t, ts.URL+"/api/status", tc.header)
		got := resp.Header.Get("Content-Encoding") == "gzip"
		if got != tc.want {
			t.Fatalf("Accept-Encoding %q: compressed = %v, want %v", tc.header, got, tc.want)
		}
		if !got {
			if !json.Valid(body) {
				t.Fatalf("Accept-Encoding %q: the identity answer is not JSON", tc.header)
			}
			continue
		}
		if !json.Valid(gunzip(t, body)) {
			t.Fatalf("Accept-Encoding %q: the compressed answer is not JSON", tc.header)
		}
	}
}

// TestSmallAnswersAreLeftAlone covers the floor. /healthz is three bytes: gzip
// would make it thirty and add a header to say so. A probe that reads "ok" is
// also the last thing that should be handed a coding to undo.
func TestSmallAnswersAreLeftAlone(t *testing.T) {
	a := rigShapedArchive(t)
	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)

	resp, body := rawGet(t, ts.URL+"/healthz", "gzip")
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("/healthz was compressed (%q); it is three bytes and gzip costs it "+
			"twenty-seven more", enc)
	}
	if string(body) != "ok\n" {
		t.Fatalf("/healthz answered %q", body)
	}
	// A 404 is a handful of bytes too, and it is still a well-formed answer.
	tr := &http.Transport{DisableCompression: true}
	t.Cleanup(tr.CloseIdleConnections)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/nothing-here", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	r404, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("GET /nothing-here: %v", err)
	}
	defer r404.Body.Close()
	small, _ := io.ReadAll(r404.Body)
	if r404.StatusCode != http.StatusNotFound {
		t.Fatalf("an unknown path answered HTTP %d", r404.StatusCode)
	}
	if enc := r404.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("a 19-byte 404 was compressed (%q)", enc)
	}
	if !strings.Contains(string(small), "404") {
		t.Fatalf("the 404 body was mangled: %q", small)
	}
}

// TestTheArchivesOwnReadersAreUnaffected is the consumer check, as a test. Two
// kinds of reader exist for these endpoints and the wrapper has to be invisible
// to both:
//
//	ringstat, which is Go. net/http asks for gzip on its own behalf and
//	decompresses on its own behalf, so the terminal tool gets the saving with no
//	change to a line of it — and resp.Uncompressed is how that is visible.
//
//	e2e's shell, which is curl. curl sends no Accept-Encoding unless it is told
//	to, so every status_json, rig-check.sh and wait_healthy in e2e/ keeps
//	receiving the identity bytes it has always parsed.
func TestTheArchivesOwnReadersAreUnaffected(t *testing.T) {
	a := rigShapedArchive(t)
	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)

	// ---- ringstat's own two entry points, unchanged.
	status, err := FetchStatus(ts.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("FetchStatus: %v", err)
	}
	if status.SlotCount != 6 || status.Totals.Population == nil {
		t.Fatalf("FetchStatus read a different map: %+v", status.Map)
	}
	index, err := FetchSpecies(ts.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("FetchSpecies: %v", err)
	}
	if len(index.Species) == 0 {
		t.Fatal("FetchSpecies read an empty index from a map of six censusing worlds")
	}
	// And it really did travel compressed: net/http negotiated, and says so.
	resp, err := http.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatalf("GET /api/status: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if !resp.Uncompressed {
		t.Fatal("a stock Go client did not get a compressed answer; every reader in this " +
			"project that is not a shell is one of these, and the egress saving is theirs")
	}

	// ---- the shell's reader: no Accept-Encoding, identity bytes, still JSON.
	_, body := rawGet(t, ts.URL+"/api/status", "")
	var frame Status
	if err := json.Unmarshal(body, &frame); err != nil {
		t.Fatalf("curl's view of /api/status stopped being JSON: %v", err)
	}
	if frame.SlotCount != 6 {
		t.Fatalf("curl's view reads %d slots", frame.SlotCount)
	}
}
