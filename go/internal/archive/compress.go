package archive

// The archive answers gzip when a client asks for it, and only then.
//
// wp3_hosting_options.md measured the status page as the LARGEST SINGLE EGRESS
// TERM of the hosted service — larger than the game's own migration traffic.
// Per continuously open browser tab: /api/status 19.7 KB every 2 s and
// /api/hops 49.3 KB every 1.5 s, uncompressed, which at real player speeds is
// ~32 GB/month for one person leaving a tab open, and none of it game traffic.
// The same pass measured the gzip ratios — 8.7×, 7.5× and 5.0× — and named the
// fix as one handler wrapper. This is that wrapper.
//
// It is TRANSPORT AND NOTHING ELSE. No payload changes shape, no poll changes
// cadence, no endpoint gains or loses a field: what a reader has after
// decompressing is byte-for-byte what this archive served before. The other
// half of that measurement — whether the two poll intervals should be longer —
// changes what the page FEELS like and is the page owner's design call, not a
// transport one, so nothing here touches it.
//
// Four properties the negotiation owes everyone downstream:
//
//	IDENTITY IS THE DEFAULT. A client that says nothing gets the bytes it always
//	got. curl, a shell health probe, a hand-typed URL, e2e's status_json: all
//	unchanged, because none of them sends Accept-Encoding. Go's own HTTP client
//	sends it for itself and decompresses for itself, so ringstat gets the saving
//	without a line of code (net/http sets resp.Uncompressed and hides both
//	headers from the caller).
//
//	VARY IS ALWAYS SET — on the compressed answer and on the identity one alike.
//	The answer depends on a request header, and a cache that has not been told
//	so will hand gzip to a client that never asked for it. These endpoints are
//	all no-store today, but Vary is a fact about the response, not a hedge
//	against this archive's own caching policy.
//
//	NOTHING IS ENCODED TWICE. A handler that sets its own Content-Encoding owns
//	its body and this wrapper leaves it alone.
//
//	SMALL ANSWERS ARE LEFT ALONE. /healthz is three bytes and gzips to about
//	thirty; a 40-byte error the same. Below one packet compression costs bytes
//	rather than saving them, so the wrapper holds the first writes back and
//	commits to gzip only once the body has proved it is worth compressing.

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// gzipMinBytes is the floor under which a body goes out as it is. One ethernet
// MTU: below a single packet there is no round trip to save, and the header
// this wrapper would have to add is itself twenty bytes. Every endpoint the
// measurement cared about is one to two orders of magnitude above it.
const gzipMinBytes = 1400

// One deflate state is ~260 KB and the status page rebuilds one every two
// seconds per open tab. The archive is the process this project sizes its
// machine on (its replay peak is the whole of wp3's Part 1), so the compressor
// is pooled rather than allocated per request.
var gzipPool = sync.Pool{New: func() any { return gzip.NewWriter(io.Discard) }}

// gzipped wraps a handler in standard Accept-Encoding negotiation.
func gzipped(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Added before the handler runs, so it is on the answer whichever way
		// the negotiation goes and whatever else the handler sets.
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w, status: http.StatusOK}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

// acceptsGzip answers RFC 9110 §12.5.3 for the one coding this archive speaks.
// gzip is acceptable when the client named it with a non-zero quality, or named
// "*" with a non-zero quality and did not separately refuse gzip. An explicit
// entry always decides: "gzip;q=0" is a REFUSAL and outranks any wildcard.
func acceptsGzip(header string) bool {
	wildcard := false
	for _, field := range strings.Split(header, ",") {
		name, params, _ := strings.Cut(field, ";")
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "gzip":
			return qualityIsPositive(params)
		case "*":
			wildcard = qualityIsPositive(params)
		}
	}
	return wildcard
}

// qualityIsPositive reads the q= parameter of one Accept-Encoding entry. An
// entry that states no quality is fully acceptable; one that states an
// unparseable quality is treated as a refusal, because guessing at a client's
// intent is how a client ends up holding bytes it cannot read.
func qualityIsPositive(params string) bool {
	for _, p := range strings.Split(params, ";") {
		k, v, ok := strings.Cut(p, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), "q") {
			continue
		}
		q, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return err == nil && q > 0
	}
	return true
}

// gzipResponseWriter defers the whole decision to the moment the body's size is
// known. Nothing reaches the wire until it commits, which is what lets it set
// Content-Encoding, drop a Content-Length it can no longer honour, and sniff a
// Content-Type off the ORIGINAL bytes rather than off the deflate stream.
type gzipResponseWriter struct {
	http.ResponseWriter
	status     int
	headerSeen bool // the handler has chosen a status code
	committed  bool // the header has gone to the wire; the decision is made
	buf        []byte
	gz         *gzip.Writer // non-nil only when the answer is being compressed
}

var _ http.Flusher = (*gzipResponseWriter)(nil)

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.committed || w.headerSeen {
		return
	}
	if status >= 100 && status <= 199 {
		// An informational response is not the final one. Pass it straight
		// through and keep waiting for the header that carries the body.
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.status = status
	w.headerSeen = true
	if !bodyAllowed(status) {
		_ = w.commit(false)
	}
}

func (w *gzipResponseWriter) Write(p []byte) (int, error) {
	if w.committed {
		if w.gz != nil {
			return w.gz.Write(p)
		}
		return w.ResponseWriter.Write(p)
	}
	w.buf = append(w.buf, p...)
	if len(w.buf) < gzipMinBytes {
		return len(p), nil
	}
	if err := w.commit(true); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Flush commits on whatever the body has reached so far. No endpoint here
// streams — every one of them encodes a value and returns — but a handler that
// flushed into a writer that silently held its bytes would be a trap for
// whoever writes the first streaming endpoint.
func (w *gzipResponseWriter) Flush() {
	if !w.committed {
		_ = w.commit(len(w.buf) >= gzipMinBytes)
	}
	if w.gz != nil {
		_ = w.gz.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap is what http.ResponseController follows to reach the real connection,
// so a deadline or a hijack still finds it through this wrapper.
func (w *gzipResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// commit writes the header and drains everything held back. worthIt is the size
// verdict; the rest of the conditions are the ones a body carries with it.
func (w *gzipResponseWriter) commit(worthIt bool) error {
	w.committed = true
	h := w.ResponseWriter.Header()
	if worthIt && h.Get("Content-Encoding") == "" {
		// The sniff is OURS now: net/http would sniff whatever reaches it,
		// and what reaches it is a deflate stream that sniffs as
		// application/x-gzip. Every endpoint here sets its own type; this is
		// for the next one that forgets to.
		if h.Get("Content-Type") == "" {
			h.Set("Content-Type", http.DetectContentType(w.buf))
		}
		h.Set("Content-Encoding", "gzip")
		// A length measured on the uncompressed body is now a lie. Nobody knows
		// the compressed one yet, so net/http chunks the answer instead.
		h.Del("Content-Length")
		gz := gzipPool.Get().(*gzip.Writer)
		gz.Reset(w.ResponseWriter)
		w.gz = gz
	}
	w.ResponseWriter.WriteHeader(w.status)
	if len(w.buf) == 0 {
		return nil
	}
	buf := w.buf
	w.buf = nil
	if w.gz != nil {
		_, err := w.gz.Write(buf)
		return err
	}
	_, err := w.ResponseWriter.Write(buf)
	return err
}

// close is the end of the request: a body that never reached the floor goes out
// as it is, and the compressor is finished and returned to the pool.
func (w *gzipResponseWriter) close() {
	if !w.committed {
		_ = w.commit(false)
	}
	if w.gz != nil {
		_ = w.gz.Close()
		// Reset before pooling, so a finished request pins neither the
		// connection it wrote to nor its buffers.
		w.gz.Reset(io.Discard)
		gzipPool.Put(w.gz)
		w.gz = nil
	}
}

// bodyAllowed repeats net/http's own rule (RFC 9110 §6.4.1, §15.4.5). A 204 or
// a 304 carries no body, so there is nothing to encode and a Content-Encoding
// on one is a claim about a body that is not there.
func bodyAllowed(status int) bool {
	switch status {
	case http.StatusNoContent, http.StatusNotModified:
		return false
	}
	return status < 100 || status > 199
}
