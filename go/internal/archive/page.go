package archive

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"multiverse/internal/wire"
)

// The status page is SELF-CONTAINED HTML with inline JS and SVG that polls two
// JSON endpoints. No CDN, no build step, no framework, no external asset of any
// kind: the page has to work on a LAN with no internet, opened from a browser on
// either machine or from a phone, and it has to keep working when the archive is
// the only thing still running.
//
// It is a MAP, not a dashboard. The grid is drawn at its real coordinates, each
// world's population is drawn as the organisms it is, and the lanes are drawn as
// the arrows they are — including the torus wrap and, above all, the BYPASS: a
// curved arrow reaching past a dark world to the next live one. The bypass is
// the one thing a table of numbers cannot show and the one thing Risk 5 says an
// operator must not miss.
//
// It also EXPLAINS ITSELF. The people who open it did not build the system, so
// every piece of jargon carries a tooltip and the glossary at the bottom says
// the same thing at length. Tooltip and glossary read from one table, so the two
// can never drift apart.

// httpHandler serves the operator surface: the page, its JSON, and a health
// probe. Every one of them goes out through gzipped (compress.go), which is
// negotiation and nothing else: a client that asks for gzip gets it, a client
// that does not gets the same bytes it always got, and no payload on this mux
// changes shape either way.
func (a *Archive) httpHandler() http.Handler {
	mux := http.NewServeMux()
	// DQ7's deny list is applied HERE, at the serving boundary, and on every one
	// of these four endpoints (§22, B30). Two properties come out of the
	// placement and neither is incidental:
	//
	//	THE PAGE AND ringstat CANNOT DISAGREE. They render the same JSON, so a
	//	name suppressed for one is suppressed for the other by construction
	//	rather than by two lists kept in step.
	//
	//	THE RECORD IS UNTOUCHED. StatusView's own value is what MetricsLog
	//	serializes and the ledger already holds what crossed. Suppression is a
	//	fact about the view, and D11's never-evict rule is not bent by it: M5
	//	promises removal from the view and explicitly does not promise removal
	//	from the record.
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		view := a.deny.ApplyStatus(a.StatusView())
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(view)
	})
	// The recent-hops feed rides its OWN endpoint and is deliberately absent from
	// /api/status (§17, B14). /api/status is what MetricsLog.Append serializes
	// verbatim into the durable sample file once a minute; a per-organism feed
	// hung off it would be written to disk forever, beside a ledger that already
	// holds every one of those hops. See hops.go.
	mux.HandleFunc("/api/hops", func(w http.ResponseWriter, r *http.Request) {
		feed := a.deny.ApplyHopFeed(a.HopFeedView())
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(feed)
	})
	// The species index rides its OWN endpoint for the same reason the hop feed
	// does (§17, B14): /api/status is what MetricsLog.Append serializes verbatim
	// into the durable sample file once a minute, and the ledger annotations
	// here — crossings ever, distinct genomes, recent lanes — are DERIVED from a
	// file that already holds every one of those facts. Writing them into a
	// second file forever would be the same mistake twice.
	//
	// It is also why the ALIVE UNION is computed here rather than on the page:
	// the union is cheap, but joining it to the ledger is not something a
	// browser can do at all without being handed the ledger, and one join in one
	// place is what stops the terminal tool and the page disagreeing about which
	// species are endemic.
	mux.HandleFunc("/api/species", func(w http.ResponseWriter, r *http.Request) {
		view := a.deny.ApplySpeciesIndex(a.SpeciesIndexView())
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(view)
	})
	// The genealogy rides its OWN endpoint, for the third time and the same
	// reason (§17, B14): /api/status is what MetricsLog.Append serializes
	// verbatim into the durable sample file once a minute, and this view is
	// DERIVED — every edge in it is already in the ledger, and every leaf is
	// already in the census /api/status carries. Hanging it off the status
	// payload would write a recomputable tree to disk once a minute forever and
	// widen the one file whose width is a disk budget (§20, B20).
	//
	// IT IS ALSO THE SPECIES VIEW NOW, which is why it carries the census facts
	// the flat index carries: the page draws ONE tab from it (tree.go's header).
	// That is a payload the page fetches instead of two, not in addition to them
	// — and /api/species below is untouched, because ringstat reads it and a
	// terminal table is a different answer to a different question.
	//
	// The reduction happens on THIS side (tree.go rule 2), so the page and
	// ringstat cannot disagree about who is related to whom.
	mux.HandleFunc("/api/species/tree", func(w http.ResponseWriter, r *http.Request) {
		view := a.deny.ApplySpeciesTree(a.SpeciesTreeView())
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(view)
	})
	// The shared trend answer: every living species' recent shape in ONE bounded
	// read of the sample file. It rides its own endpoint rather than the tree's
	// because the two have different costs and different cadences — the tree is
	// an in-memory derivation polled every two seconds, and this reads a file and
	// is polled once a minute. Hanging it off the tree would put a disk read on
	// the two-second cadence for a line that changes once a minute.
	mux.HandleFunc("/api/species/trends", func(w http.ResponseWriter, r *http.Request) {
		window, buckets := historyParams(r)
		tr, err := a.SpeciesTrendsView(window, buckets)
		if err != nil {
			http.Error(w, `{"error":"trends unavailable"}`, http.StatusInternalServerError)
			a.log.Warn("archive: species trends read failed", "err", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(a.deny.ApplySpeciesTrends(tr))
	})
	// The brain-complexity series rides its OWN endpoint, for the fourth time and
	// the same reason (§17, B14): /api/status is what MetricsLog.Append
	// serializes verbatim into the durable sample file once a minute, and this is
	// a series over a maintained aggregate with a durable file of its own. Hanging
	// it off the status payload would write the same history twice, in two
	// resolutions, forever.
	//
	// IT TAKES A WINDOW RATHER THAN AN HOUR COUNT, unlike every other history
	// endpoint here, and that is what lets the panel share the genealogy's axis.
	// That axis RE-FITS to the drawn set (tree.go, SpanStartMs/SpanStartSeedMs) —
	// a search, a revealed seed row or a species leaving the census moves its left
	// edge — so a series at a fixed resolution ending at now could not be drawn
	// against it. The page sends the two edges it is actually drawing and gets the
	// series re-aggregated onto them.
	//
	// NOTHING HERE CARRIES A SPECIES NAME, so the deny list has nothing to apply:
	// the answer is bucket times, counts and distributions. That is a property of
	// the payload rather than an exemption, and it is why this endpoint is the one
	// on this mux that does not pass through d.Apply*.
	mux.HandleFunc("/api/species/brains", func(w http.ResponseWriter, r *http.Request) {
		from, to, buckets := brainParams(r)
		view := a.BrainHistoryView(from, to, buckets)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(view)
	})
	mux.HandleFunc("/api/species/history", func(w http.ResponseWriter, r *http.Request) {
		key, ok := speciesKeyParam(r)
		if !ok {
			http.Error(w, `{"error":"key is missing or too long"}`, http.StatusBadRequest)
			return
		}
		window, buckets := historyParams(r)
		h, err := a.SpeciesHistoryView(key, window, buckets)
		if err != nil {
			http.Error(w, `{"error":"history unavailable"}`, http.StatusInternalServerError)
			a.log.Warn("archive: species history read failed", "err", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(a.deny.ApplySpeciesHistory(h))
	})
	mux.HandleFunc("/api/history", func(w http.ResponseWriter, r *http.Request) {
		window, buckets := historyParams(r)
		var h History
		var err error
		if r.URL.Query().Get("range") == "all" {
			h, err = a.HistoryAllView(buckets)
		} else {
			h, err = a.HistoryView(window, buckets)
		}
		if err != nil {
			http.Error(w, `{"error":"history unavailable"}`, http.StatusInternalServerError)
			a.log.Warn("archive: history read failed", "err", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(h)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/live" {
			serveNotFound(w)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(statusPageHTML))
	})
	mux.HandleFunc("/watch", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/watch" {
			serveNotFound(w)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(watchPageHTML))
	})
	mux.HandleFunc("/map", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/live", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/favicon.svg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write([]byte(faviconSVG))
	})
	mux.HandleFunc("/social-card.svg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write([]byte(socialCardSVG))
	})
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write([]byte("User-agent: *\nAllow: /\nSitemap: https://bibitesmultiverse.com/sitemap.xml\n"))
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://bibitesmultiverse.com/</loc><priority>1.0</priority></url>
  <url><loc>https://bibitesmultiverse.com/live</loc><priority>0.8</priority></url>
  <url><loc>https://bibitesmultiverse.com/watch</loc><priority>0.8</priority></url>
</urlset>
`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			serveNotFound(w)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(landingPageHTML))
	})
	return gzipped(mux)
}

func serveNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(notFoundPageHTML))
}

// historyParams reads ?hours= and ?buckets=, and clamps both. The all-record
// route uses only the bucket count from this result and applies its own fixed
// read bound.
func historyParams(r *http.Request) (time.Duration, int) {
	window := HistoryDefaultWindow
	if v := r.URL.Query().Get("hours"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			window = time.Duration(f * float64(time.Hour))
		}
	}
	if window < HistoryMinWindow {
		window = HistoryMinWindow
	}
	if window > HistoryMaxWindow {
		window = HistoryMaxWindow
	}
	buckets := HistoryDefaultBuckets
	if v := r.URL.Query().Get("buckets"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			buckets = n
		}
	}
	if buckets < HistoryMinBuckets {
		buckets = HistoryMinBuckets
	}
	if buckets > HistoryMaxBuckets {
		buckets = HistoryMaxBuckets
	}
	return window, buckets
}

// brainParams reads ?from=, ?to= and ?buckets= for /api/species/brains, and
// clamps all three. It takes two ABSOLUTE EDGES rather than an hour count
// because the panel shares an axis that re-fits: the caller sends the window it
// is actually drawing, not a duration ending at now.
//
// An absent or nonsensical window falls back to the aggregate's own default —
// the last day — rather than to nothing, so a request with no query string is
// still an answer somebody can look at.
func brainParams(r *http.Request) (fromMs, toMs int64, buckets int) {
	q := r.URL.Query()
	toMs = time.Now().UnixMilli()
	if v := q.Get("to"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			toMs = n
		}
	}
	fromMs = toMs - HistoryDefaultWindow.Milliseconds()
	if v := q.Get("from"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			fromMs = n
		}
	}
	if fromMs >= toMs {
		fromMs = toMs - HistoryDefaultWindow.Milliseconds()
	}
	if max := BrainMaxWindow.Milliseconds(); toMs-fromMs > max {
		fromMs = toMs - max
	}
	buckets = HistoryDefaultBuckets
	if v := q.Get("buckets"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			buckets = n
		}
	}
	if buckets < BrainMinBuckets {
		buckets = BrainMinBuckets
	}
	if buckets > BrainMaxBuckets {
		buckets = BrainMaxBuckets
	}
	return fromMs, toMs, buckets
}

// maxSpeciesKeyBytes bounds ?key=. Two 64-byte halves and a joining space is
// 129 (contract-a.md §19, A42); the slack above it is for a caller that passed
// a raw spelling with whitespace this end normalizes away.
const maxSpeciesKeyBytes = 200

// speciesKeyParam reads ?key= and NORMALIZES IT — the one place a caller's
// spelling is repaired, and only because the key is a comparison key and never
// a label. A page that already holds the normalized key sends it back
// unchanged; a human typing a raw name into the URL still gets an answer.
func speciesKeyParam(r *http.Request) (string, bool) {
	raw := r.URL.Query().Get("key")
	if raw == "" || len(raw) > maxSpeciesKeyBytes {
		return "", false
	}
	key := wire.NormalizeSpeciesName(raw)
	if key == "" {
		return "", false
	}
	return key, true
}

func (a *Archive) serveHTTP() {
	if a.httpSrv == nil {
		return
	}
	_ = a.httpSrv.Serve(a.httpLn)
}

// HTTPAddr is the resolved status-page address, or "" when none was asked for.
func (a *Archive) HTTPAddr() string {
	if a.httpLn == nil {
		return ""
	}
	return a.httpLn.Addr().String()
}

const statusPageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Bibites Multiverse — Live Map</title>
<meta name="description" content="The live Bibites Multiverse map: connected worlds, migrations, living species, lineages, and reported settings.">
<meta name="theme-color" content="#0b1110">
<link rel="canonical" href="https://bibitesmultiverse.com/live">
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<style>
:root{color-scheme:dark;--bg:#0b1110;--panel:#111a18;--panel2:#16221f;--line:#294038;
--text:#eff7f3;--dim:#9aafa7;--live:#66e0ac;--dark:#e86c76;--hole:#53625d;
--warn:#efbd57;--lane:#75bdf2;--flash:#87ebc2;--hot:#f4fffb;--cell:#0e1714;
--ink:#07100d;--max:1440px}
*{box-sizing:border-box}
html{scroll-behavior:smooth}
body{margin:0;background:
radial-gradient(circle at 82% 2%,rgba(67,160,126,.17),transparent 30rem),
radial-gradient(circle at 5% 24%,rgba(62,126,174,.11),transparent 28rem),var(--bg);
color:var(--text);font:14px/1.55 ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}
a{color:inherit}.skiplink{position:fixed;left:12px;top:-80px;z-index:100;background:var(--text);
color:var(--ink);padding:9px 13px;border-radius:7px;text-decoration:none}.skiplink:focus{top:12px}
.consolehead{border-bottom:1px solid rgba(83,120,107,.34);background:rgba(11,17,16,.72)}
.console-nav,.console-summary,.tabs,main,footer{width:min(calc(100% - 40px),var(--max));margin-inline:auto}
.console-nav{min-height:72px;display:flex;align-items:center;justify-content:space-between;gap:24px}
.brand{display:inline-flex;align-items:center;gap:10px;text-decoration:none;font-weight:760;
letter-spacing:-.02em}.mark{width:30px;height:22px;color:var(--live);filter:drop-shadow(0 0 9px rgba(102,224,172,.3))}
.consolelinks{display:flex;align-items:center;gap:22px;font-size:14px;color:var(--dim)}
.consolelinks a{text-decoration:none}.consolelinks a:hover,.consolelinks a:focus-visible{color:var(--text)}
.consolelinks a[aria-current="page"]{color:var(--text);font-weight:700}
.livepill{display:inline-flex;align-items:center;gap:7px;padding:7px 11px;border:1px solid var(--line);
border-radius:999px;color:var(--text)!important;background:rgba(22,34,31,.75)}
.navdot{width:7px;height:7px;border-radius:50%;background:var(--live);box-shadow:0 0 0 4px rgba(102,224,172,.1)}
.console-summary{display:grid;grid-template-columns:minmax(280px,.85fr) minmax(520px,1.15fr);
gap:46px;align-items:end;padding-block:54px 48px}
.console-title .eyebrow{display:flex;align-items:center;gap:9px;margin:0 0 12px;color:var(--live);
font:700 11px/1.2 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;letter-spacing:.13em;text-transform:uppercase}
.console-title .eyebrow:before{content:"";width:26px;border-top:1px solid currentColor}
h1{font-size:clamp(38px,5vw,64px);line-height:.98;letter-spacing:-.045em;margin:0}
.summarycopy{max-width:560px;margin:17px 0 0;color:#bfd0c9;font-size:16px}
.statusgrid{display:grid;grid-template-columns:1.35fr repeat(2,1fr);border:1px solid var(--line);
border-radius:16px;overflow:hidden;background:rgba(17,26,24,.94);box-shadow:0 18px 55px rgba(0,0,0,.2)}
.statusitem{min-width:0;min-height:92px;padding:18px 20px;display:flex;flex-direction:column;
justify-content:center;border-left:1px solid var(--line)}
.statusitem:first-child,.statusitem:nth-child(4){border-left:0}.statusitem:nth-child(n+4){border-top:1px solid var(--line)}
.statusitem.age{grid-column:span 2}.statuskey{color:var(--dim);font:10px/1.3 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
letter-spacing:.1em;text-transform:uppercase;margin-bottom:6px}.hdr{min-width:0;color:var(--dim);font:12px/1.45 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
overflow-wrap:anywhere}.hdr b{font-size:20px;line-height:1.15;font-weight:740;color:var(--text);font-variant-numeric:tabular-nums;letter-spacing:-.03em}
.statusitem:first-child .hdr{font-size:11px}
.muted{color:var(--dim)}
/* minmax(0,...) and min-width:0 all the way down: a grid or flex item defaults to
   min-width:auto, and one wide child then pushes the whole page sideways. The
   map and the tables scroll INSIDE their own box; the body never does. */
main{padding-block:28px 70px;display:grid;gap:20px;grid-template-columns:minmax(0,1fr)}
.panel{display:grid;gap:18px;grid-template-columns:minmax(0,1fr);min-width:0}
.panel[hidden]{display:none}
section{background:linear-gradient(145deg,rgba(22,34,31,.93),rgba(17,26,24,.94));
border:1px solid var(--line);border-radius:16px;padding:20px;min-width:0;overflow:hidden;
box-shadow:0 16px 44px rgba(0,0,0,.13)}

/* ---- the tab bar ----
   Three views over ONE poll. It stays visible after the page introduction and
   scrolls sideways rather than wrapping, so a narrow phone keeps three useful
   tap targets instead of two lines of half-height ones. */
.tabs{position:sticky;top:0;z-index:19;display:flex;gap:6px;padding:8px;
border:1px solid rgba(83,120,107,.38);border-radius:14px;background:rgba(11,17,16,.9);
backdrop-filter:blur(16px);overflow-x:auto;-webkit-overflow-scrolling:touch;
box-shadow:0 10px 34px rgba(0,0,0,.2);transform:translateY(-17px);margin-bottom:-17px}
.tabs .tab{flex:1 1 0;min-width:104px;font:700 11px/1.3 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
letter-spacing:.1em;text-transform:uppercase;color:var(--dim);background:transparent;border:1px solid transparent;
border-radius:8px;padding:10px 14px;cursor:pointer;white-space:nowrap}
.tabs .tab:hover{color:var(--text);background:rgba(22,34,31,.6)}
.tabs .tab[aria-selected="true"]{color:var(--ink);border-color:var(--live);background:var(--live)}
.tabs .tab .sub{display:block;font-size:10px;letter-spacing:.04em;text-transform:none;
color:inherit;opacity:.68;margin-top:3px;font-weight:500}
/* On smaller screens the three tap targets matter and the subtitles do not:
   keeping them makes their text collide before the bar needs to scroll. */
@media (max-width:900px){.tabs .tab .sub{display:none}.tabs .tab{padding:13px 8px}}

/* ---- the species view: one drawing, on a time axis ----
   ONE ROW PER SPECIES, and the row is the unit of everything: the label column
   is fixed and clipped, the mini-map and the count and the trend line sit in
   their own columns, and the plot on the right carries the bar. A row per node
   is what makes label placement a non-problem at any species count — a classic
   dendrogram centres an ancestor between its children and then has nowhere to
   put that ancestor's name.

   It scrolls INSIDE its own box, like the map and the tables, because a long
   species name is 64 bytes a peer chose and must not be able to push the page
   sideways. */
.spctl{display:flex;gap:8px 14px;align-items:center;flex-wrap:wrap;margin-bottom:10px}
.spctl input,.spctl select{font:inherit;font-size:12px;color:var(--text);background:var(--cell);
border:1px solid var(--line);border-radius:8px;padding:7px 9px;min-width:0}
.spctl input{flex:1 1 200px;max-width:340px}
.spctl label{font-size:11px;color:var(--dim);display:flex;gap:6px;align-items:center}
.chip{display:inline-block;font-size:11px;background:var(--cell);border:1px solid var(--line);
border-radius:999px;padding:2px 7px;margin:0 5px 3px 0;white-space:nowrap}
.chip b{font-variant-numeric:tabular-nums}
.chip.exl{color:var(--warn);border-color:var(--warn);white-space:pre-wrap;word-break:break-word}
.lifewrap{overflow:auto;max-height:min(78vh,1000px);border:1px solid var(--line);
border-radius:10px;background:var(--cell)}
svg.life{display:block}
svg.life .hit{fill:transparent}
svg.life .lfrow{cursor:pointer}
svg.life .lfrow:hover .hit{fill:rgba(90,169,230,.08)}
svg.life .lfrow.open .hit{fill:rgba(90,169,230,.10)}
svg.life .lfrow:hover .nm{fill:var(--hot)}
/* white-space:pre on the NAME, for contract-a.md §17 A36's reason: SVG text
   collapses a run of spaces exactly as HTML does, and a doubled space inside a
   species name is the owning world's spelling, not noise.
   The class selectors are deliberately NOT element-qualified: the name is a
   <tspan> inside a <text> and so is every annotation beside it, so "text.meta"
   would match nothing. One <text> per row lets the annotations FLOW after a name
   whose width nothing here measures. */
svg.life text{font:12px/1 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
svg.life text.nm{fill:var(--text);white-space:pre}
svg.life .anc text.nm{fill:var(--dim);font-size:11px}
svg.life .meta{fill:var(--dim);font-size:10.5px}
svg.life .gen{fill:var(--dim);font-size:9.5px}
svg.life text.pop{fill:var(--text);font-size:11.5px;font-variant-numeric:tabular-nums}
svg.life .eggs{fill:var(--dim);font-size:9.5px}
/* The badge line under a name. It is its own text run, outside the name's clip,
   because the badges used to ride inside it and were cut off there. */
svg.life text.bdg{fill:var(--dim);font-size:9.5px}
svg.life .tbadge{font-size:9.5px;letter-spacing:.06em}
svg.life .tbadge.warn{fill:var(--warn)}
svg.life .tbadge.lane{fill:var(--lane)}
svg.life .tbadge.live{fill:var(--live)}
/* The root badge is DELIBERATELY NOT the warning colour the two standing-alone
   badges wear: "the record reaches 31 generations above this and stops" is a
   statement about the record's own edge, not a gap in it, and a reader who sees
   amber twice will read the two as the same kind of thing. */
svg.life .tbadge.rec{fill:var(--dim)}
svg.life .ring{fill:none;stroke:var(--dim);stroke-width:1.5}
/* THE BAR IS NOT COLOURED BY SPECIES. The glyph carries the colour and nothing
   else does: a bar tinted to match it would repeat one fact twice and leave a
   reader looking for a second meaning in it. Alive is bright, an ancestor
   nothing is alive of is dim and thinner, and a bar whose start was inferred
   from a descendant is outlined rather than filled. */
svg.life .lfbar.live{fill:var(--text);opacity:.82}
svg.life .lfbar.ext{fill:var(--dim);opacity:.55}
svg.life .lfbar.derived{fill:none;stroke:var(--dim);stroke-width:1;stroke-dasharray:3 2}
svg.life .undated{fill:none;stroke:var(--warn);stroke-width:1.4}
/* The brain ring sits at the end of the bar and is never filled: it is a place on
   a scale, not a quantity of anything you could add up. pointer-events:all is what
   makes it answerable — an unfilled circle is only hittable on its own stroke, and
   the smallest ring here is 2.4 px of radius, so the whole disc has to be the
   target for the tooltip carrying its real numbers to be reachable at all. */
svg.life .brain{fill:none;stroke:var(--lane);stroke-width:1.3;cursor:help;pointer-events:all}
svg.life .link{fill:none;stroke:var(--line);stroke-width:1.5}
/* The collapsed run. It is the SAME line as .link, dotted, because it is the same
   relationship drawn across a stretch whose species are not on the picture — and
   it is now on the clock like every other horizontal distance here, so the dots
   say "nobody drawn held this" rather than "this length is not time". */
svg.life .chain{fill:none;stroke:var(--dim);stroke-width:1.2;stroke-dasharray:2 3}
/* A LINK THAT RUNS BACKWARDS, and the ring where it left its parent. Amber for
   the reason everything amber on this page is amber: it is a mark to read twice
   rather than a mark to read. The two are wider-dashed than the collapsed run so
   the two dotted things on one drawing cannot be taken for each other. */
svg.life .rev{fill:none;stroke:var(--warn);stroke-width:1.3;stroke-dasharray:4 2;cursor:help}
svg.life .revmark{fill:none;stroke:var(--warn);stroke-width:1.3;cursor:help}
svg.life .wdot{stroke-width:1.1}
svg.life .wdot.on{fill:var(--text);stroke:none}
svg.life .wdot.off{fill:none;stroke:var(--line)}
svg.life .wdot.unk{fill:none;stroke:var(--warn);stroke-dasharray:1.5 1.5}
svg.life .trend{fill:none;stroke:var(--live);stroke-width:1.4;stroke-linejoin:round}
svg.life .trendbase{stroke:var(--line);stroke-width:1}
svg.life .grid{stroke:var(--line);stroke-width:1;opacity:.5}
svg.life .tick,svg.life .colhd{fill:var(--dim);font-size:10px;letter-spacing:.04em}
svg.life .nowline{stroke:var(--live);stroke-width:1;opacity:.55}
/* The record's floor. IT IS OFF THE LEFT EDGE ON AN ORDINARY MAP — the axis is
   fitted to the oldest DRAWN bar and the floor is a fixed date days older — so it
   is a CAPTION at the margin, in the quiet colour the ticks wear, and it costs no
   pixels. The dashed line and its amber label are for the case where the floor
   really does fall inside the drawn span, where it is a boundary a reader can
   see: left of it, no edge in this drawing can begin. */
svg.life .floor{stroke:var(--warn);stroke-width:1.2;stroke-dasharray:4 3;opacity:.8}
svg.life .floorlbl{fill:var(--warn);font-size:9.5px;cursor:help}
svg.life .floorcap{fill:var(--dim);font-size:9.5px;cursor:help}
/* ---- THE FAMILY, DRAWN IN THE LABEL COLUMN.
   A bracket per parent-child link: straight down from under the parent's own
   glyph, then a stub right into the child's. Siblings lay their rails on the same
   x, so a parent with three children reads as one rail with three stubs — and the
   last stub is where the rail stops, which is the whole of what a "└" says. It is
   the same --line the plot's drop is drawn in: one relationship, one colour, two
   places. */
svg.life .tw{fill:none;stroke:var(--line);stroke-width:1.2;stroke-linejoin:round}
/* ---- ONE LINE, LIT. Hovering or tab-focusing a row dims every row that is not
   its ancestor or its descendant, which is the fastest honest answer to "how is
   this one related". Dimming rather than hiding: the rest of the drawing keeps
   its place, so nothing moves under the reader's eye. */
svg.life.lit .lfrow{opacity:.2}
svg.life.lit .lfrow.kin{opacity:1}
svg.life.lit .ln{opacity:.12}
svg.life.lit .ln.kin{opacity:1}
/* The collapsed run lights with the rest of its link: on a link that crosses a
   gap it IS the link, and a lineage lit with one of its edges left dim would be
   the lighting answering a different question on that row than on every other. */
svg.life .ln.kin .link,svg.life .ln.kin .tw,svg.life .ln.kin .chain{stroke:var(--hot);stroke-width:1.8}
svg.life .lfrow.self .nm{fill:var(--hot)}
/* The keyboard's own mark. A row is focusable, so a reader with no mouse can walk
   the tree and read the same lit line; the focus ring is the row's own tint
   rather than a browser outline around an SVG group nobody can see the edges of. */
svg.life .lfrow:focus{outline:none}
/* The box holds the focus only as a landing place, when the focused row stops
   being drawn; a ring around the whole drawing would say far more than "you are
   still here". The next Tab answers, by walking in at the first row. */
.lifewrap:focus{outline:none}
svg.life .lfrow:focus .hit{fill:rgba(90,169,230,.14)}
svg.life .lfrow:focus .nm{fill:var(--hot)}
svg.life .detbg{fill:var(--bg);opacity:.55}
svg.life .det{fill:var(--text);font-size:11px;white-space:pre}
svg.life .det .dk{fill:var(--dim)}
svg.life .det .unk{fill:var(--warn);font-style:italic}
.treelegend{display:flex;gap:8px 16px;flex-wrap:wrap;font-size:11px;color:var(--dim);
margin:0 0 10px}
.treelegend i.ringi{display:inline-block;width:9px;height:9px;border:1.5px solid var(--dim);
border-radius:50%;vertical-align:-1px;margin-right:5px}
.treelegend i.ringi.braini{border-color:var(--lane)}
.treelegend i.bari{display:inline-block;width:22px;height:7px;background:var(--text);opacity:.82;
border-radius:2px;vertical-align:-1px;margin-right:5px}
.treelegend i.chaini{display:inline-block;width:22px;height:0;border-top:1.2px dotted var(--dim);
vertical-align:middle;margin-right:5px}
.treelegend i.revi{display:inline-block;width:22px;height:0;border-top:1.3px dashed var(--warn);
vertical-align:middle;margin-right:5px}
/* The bracket the label column draws between a parent row and a child row. */
.treelegend i.twi{display:inline-block;width:9px;height:9px;border-left:1.2px solid var(--dim);
border-bottom:1.2px solid var(--dim);vertical-align:1px;margin-right:5px}
.treelegend i.doti{display:inline-block;width:7px;height:7px;background:var(--text);
border-radius:50%;vertical-align:0;margin-right:5px}
/* ---- THE BRAIN PANEL, under the drawing and sharing its x axis.
   Its own box, directly below the tree's, with no border on top: the two read as
   one picture with one clock and are two boxes only because the tree scrolls
   vertically and this must not scroll away with it. Its horizontal scroll is
   slaved to the tree's, so the shared axis stays shared even on a window too
   narrow for the drawing. */
.brainwrap{overflow-x:auto;overflow-y:hidden;border:1px solid var(--line);border-top:0;
border-radius:0 0 10px 10px;background:var(--cell)}
.lifewrap{border-radius:10px 10px 0 0}
svg.brainp{display:block}
svg.brainp text{font:10px/1 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;fill:var(--dim)}
svg.brainp .grid{stroke:var(--line);stroke-width:1;opacity:.5}
svg.brainp .nowline{stroke:var(--live);stroke-width:1;opacity:.55}
svg.brainp .base{stroke:var(--line);stroke-width:1}
/* The two series. Synapses wear the lane blue the brain ring already wears, so
   the ring on a row and the line under it are plainly the same subject; hidden
   neurons wear the live green. Neither is the warning colour: a rising line here
   is not a fault. */
svg.brainp .syn{fill:none;stroke:var(--lane);stroke-width:1.6;stroke-linejoin:round}
svg.brainp .synband{fill:var(--lane);opacity:.16;stroke:none}
svg.brainp .hid{fill:none;stroke:var(--live);stroke-width:1.6;stroke-linejoin:round}
svg.brainp .hidband{fill:var(--live);opacity:.16;stroke:none}
svg.brainp .plbl{font-size:10px;letter-spacing:.04em}
svg.brainp .plbl.syn{fill:var(--lane);stroke:none}
svg.brainp .plbl.hid{fill:var(--live);stroke:none}
svg.brainp .yrange{fill:var(--dim);font-size:9.5px;font-variant-numeric:tabular-nums}
/* Coverage. A filled column is the share of that slice's genomes this archive
   could read; an amber tick at the floor is the OTHER absence — the record holds
   crossings there and not one of their genomes was ever measured — and an empty
   column is no crossing at all. Three different facts, three different marks. */
svg.brainp .cov{fill:var(--dim);opacity:.55}
svg.brainp .covnone{fill:var(--warn);opacity:.85}
svg.brainp .covbase{stroke:var(--line);stroke-width:1}
/* WAITING IS NOT ABSENCE, and it wears neither of the two absences' clothes. The
   drawn window can move — the seed stock revealed pulls its left edge back four
   days — before the answer for the new window has arrived, and a stretch left
   blank there is the strip's own mark for "no crossing was recorded here": a
   claim about the map, made out of a request still in flight. So it is washed,
   edged where the answer starts, and captioned. Dim, never the amber the two
   real absences wear, and under everything: where there is an answer there is no
   wash. */
svg.brainp .pending{fill:var(--dim);opacity:.09}
svg.brainp .pendedge{stroke:var(--dim);stroke-width:1;stroke-dasharray:2 3;opacity:.55}
svg.brainp .pendlbl{fill:var(--dim);font-size:9.5px;font-style:italic;cursor:help}
svg.brainp .hit{fill:transparent;cursor:help;pointer-events:all}
svg.brainp .havemark{stroke:var(--warn);stroke-width:1.2;stroke-dasharray:4 3;opacity:.8}
svg.brainp .havelbl{fill:var(--warn);font-size:9.5px;cursor:help}
svg.brainp .none{fill:var(--dim);font-size:11px}
.brainlegend{margin-top:8px}
.treelegend i.bsyn{display:inline-block;width:22px;height:0;border-top:1.6px solid var(--lane);
vertical-align:middle;margin-right:5px}
.treelegend i.bhid{display:inline-block;width:22px;height:0;border-top:1.6px solid var(--live);
vertical-align:middle;margin-right:5px}
.treelegend i.bbandi{display:inline-block;width:22px;height:8px;background:var(--lane);
opacity:.28;vertical-align:-1px;margin-right:5px}
.treelegend i.bcovi{display:inline-block;width:5px;height:9px;background:var(--dim);
opacity:.55;vertical-align:-1px;margin-right:5px}
.treestat{display:flex;gap:8px 16px;flex-wrap:wrap;font-size:11px;margin-bottom:10px}
.treestat span b{color:var(--text);font-variant-numeric:tabular-nums;font-weight:400}
/* The one control on this line. It undoes a filter the view applied on its own,
   so it has to look like something you can press rather than like more prose. */
.treestat .seedbtn{font:inherit;font-size:11px;color:var(--dim);background:var(--cell);
border:1px solid var(--line);border-radius:999px;padding:2px 9px;margin-left:2px;cursor:pointer}
.treestat .seedbtn:hover{color:var(--text);border-color:var(--dim)}
.detline{font-size:11.5px;color:var(--dim);margin:2px 0}
.detline b{color:var(--text);font-variant-numeric:tabular-nums;font-weight:400}

/* ---- the settings cards ---- */
.cards{display:grid;gap:12px;grid-template-columns:repeat(auto-fill,minmax(min(300px,100%),1fr))}
.card{border:1px solid var(--line);border-radius:12px;background:rgba(14,23,20,.82);padding:15px 16px;
min-width:0;overflow:hidden}
.cardhd{display:flex;justify-content:space-between;align-items:baseline;gap:8px;
border-bottom:1px solid var(--line);padding-bottom:6px;margin-bottom:6px;flex-wrap:wrap}
.cardhd .slot{font-size:15px;font-weight:700}
.cardhd .peer{color:var(--dim);font-size:11px}
.cardsub{font-size:11px;color:var(--dim);margin:6px 0 3px;letter-spacing:.08em;text-transform:uppercase}
.card .kv{border-bottom:1px dotted var(--line)}
.card .kv .u{color:var(--warn);font-style:italic}
/* Two quiet annotations on a ceiling's row: the wire's own name for it, beside
   the plain-English one, because that is the string a disconnection message
   quotes back — "maxFramesPerSecond 50 exceeded" — and a reader should not need
   a translation table; and the human unit beside a byte count, which never
   replaces the exact number that message quotes. */
.card .kv .lk,.card .kv .lu{font-size:10px;letter-spacing:.03em;opacity:.6}
.card .exl{margin-top:4px}
/* The shared creature definition lives in a zero-sized SVG at the top of the
   document rather than inside the map, because TWO tabs draw it — the map's
   cells and hop glyphs, and every living row of the species view — and the map's
   SVG is thrown away and rebuilt whenever the map's shape changes. */
.glyphdefs{position:absolute;width:0;height:0;overflow:hidden}
h2{font-size:16px;line-height:1.35;margin:0 0 16px;letter-spacing:-.01em;color:var(--text);
display:flex;gap:8px 14px;align-items:flex-start;flex-wrap:wrap;min-width:0}
h2:before{content:"";width:22px;margin-top:.67em;border-top:1px solid var(--live);flex:0 0 auto}
h2 .note{flex:1 1 420px;text-transform:none;letter-spacing:0;font:11px/1.55 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
.unknown{color:var(--warn);font-style:italic}
.bad{color:var(--dark)}
.ok{color:var(--live)}

/* ---- the map ---- */
.mapstage{position:relative}
.maptools{display:flex;justify-content:flex-end;margin:0 0 8px}
.mapfull{display:inline-flex;align-items:center;gap:7px;padding:7px 10px;color:var(--dim);
background:var(--cell);border:1px solid var(--line);border-radius:8px;font:700 10px/1.2
ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;letter-spacing:.08em;text-transform:uppercase;
cursor:pointer}.mapfull:hover,.mapfull:focus-visible{color:var(--text);border-color:var(--lane)}
.mapfull:focus-visible{outline:2px solid var(--lane);outline-offset:2px}.mapfull[hidden]{display:none}
.mapfull svg{width:14px;height:14px;fill:none;stroke:currentColor;stroke-width:1.8;
stroke-linecap:round;stroke-linejoin:round}.mapfull .contract{display:none}
.mapwrap{overflow-x:auto;overflow-y:hidden;max-width:100%}
#mapbox{min-height:390px}
#map{display:block;width:100%;height:auto;min-width:700px}
#map text{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
.mapstage.isfullscreen{width:100vw;height:100vh;padding:12px;background:var(--bg);
display:grid;grid-template-rows:auto minmax(0,1fr)}
.mapstage.isfullscreen .maptools{margin-bottom:8px}
.mapstage.isfullscreen .mapfull .expand{display:none}
.mapstage.isfullscreen .mapfull .contract{display:block}
.mapstage.isfullscreen .mapwrap{min-width:0;min-height:0;overflow:hidden;display:grid;place-items:center}
.mapstage.isfullscreen #mapbox{width:100%;height:100%;min-height:0;display:grid;place-items:center}
.mapstage.isfullscreen #map{width:100%;height:100%;min-width:0;max-width:100%;max-height:100%}
.mapstage.isfullscreen .mapempty{width:100%;height:100%;min-height:0}
.mapempty{min-height:390px;display:grid;place-items:center;text-align:center;border:1px dashed var(--hole);
border-radius:12px;background:var(--cell);padding:34px}.mapempty .emptybib{display:block;width:54px;height:38px;
margin:0 auto 20px;color:var(--dim)}.mapempty b{display:block;font-size:18px;color:var(--text);margin-bottom:8px}
.mapempty p{max-width:510px;margin:0 auto 18px;color:var(--dim);font-family:ui-sans-serif,system-ui,sans-serif}
.mapempty a{display:inline-block;padding:8px 12px;border:1px solid var(--line);border-radius:8px;color:var(--text);text-decoration:none}
.mapempty a:hover{border-color:var(--lane)}
.cellbg{fill:var(--cell);stroke:var(--line);stroke-width:1.5}
.cell.live .cellbg{stroke:var(--live)}
.cell.dark .cellbg{stroke:var(--dark)}
.cell.hole .cellbg{fill:none;stroke:var(--hole);stroke-dasharray:6 6}
.cell .slotno{fill:var(--text);font-size:19px;font-weight:700}
.cell .pos{fill:var(--dim);font-size:11.5px}
.cell .popnum{fill:var(--text);font-size:27px;font-weight:700;font-variant-numeric:tabular-nums}
.cell.dark .popnum{fill:var(--dark)}
.cell .popnum.unk{fill:var(--warn);font-size:16px;font-style:italic}
.cell .note{fill:var(--dim);font-size:11px}
.cell .note.warnt{fill:var(--warn)}
.cell .note.badt{fill:var(--dark)}
/* The two SETTINGS a cell reports, on their own line: how fast the world runs
   and the cap on how fast organisms are let into it. They are settings rather
   than readings, so they are drawn quieter than the population and louder than
   nothing — and an unreported one is a warning colour, because a cap nobody
   states is exactly the thing this page must not fill in for itself. */
.cell .chips{fill:var(--dim);font-size:11px}
.cell .chips .chipv{fill:var(--text)}
.cell .chips .chipu{fill:var(--warn);font-style:italic}
.cell .statelbl{font-size:11px;letter-spacing:.06em}
.cell.live .statelbl,.cell.live .statedot{fill:var(--live)}
.cell.dark .statelbl,.cell.dark .statedot{fill:var(--dark)}
.cell.hole .statelbl,.cell.hole .statedot{fill:var(--hole)}
/* The arrival flash. It is a PURE OPACITY FADE — nothing moves, nothing is
   scaled, nothing slides — so it is deliberately kept when the page is told to
   reduce motion. With travel switched off it is half of what tells a reader a
   creature landed here, and suppressing it (which this page used to do) bought
   nothing a person asking for less motion actually wanted. */
.cellhit{fill:none;stroke:var(--flash);stroke-width:3;opacity:0;pointer-events:none}
.cell.hit .cellhit{animation:hit .8s ease-out}
@keyframes hit{0%{opacity:.95}100%{opacity:0}}

/* ---- the creatures in a cell, grouped by species ----
   One glyph is one creature and its colour is its species, so a cell shows what
   lives there and not only how much. The body path takes its fill from the
   <use> that draws it, which is how 70 creatures cost one path definition; the
   eye sets its own so it reads on every hue. */
.bibeye{fill:#0b0e12}
.bibrun{cursor:help}
.bib.neutral{fill:var(--live)}
.cell.dark .bib.neutral{fill:var(--dark)}
.bib.unclassed{fill:var(--hole)}
.egg{fill:none;stroke-width:1.5}
.egg.neutral{stroke:var(--live)}
.cell.dark .egg.neutral{stroke:var(--dark)}
.egg.unclassed{stroke:var(--hole)}
.cell.dark .bibs{opacity:.6}

.laneunder{fill:none;stroke:var(--cell);stroke-width:9;stroke-linecap:round}
.lane{fill:none;stroke-width:2.4;stroke-linecap:round}
.lane.open{stroke:var(--lane)}
.lane.bypass{stroke:var(--warn);stroke-dasharray:10 7}
.lane.closed{stroke:var(--dark);stroke-dasharray:2 6}
.lanehit{fill:none;stroke:transparent;stroke-width:20;cursor:help}
g.lg:hover .lane{stroke-width:4}
.lanelbl{font-size:10.5px;fill:var(--dim);text-anchor:middle}
.lanelbl.bypasslbl{fill:var(--warn)}
.lanelbl.closedlbl{fill:var(--dark)}
.edgebar{stroke:var(--hole);stroke-width:2.5;stroke-linecap:round}
.wraplbl{font-size:10px;fill:var(--dim)}
/* ---- a hop: one named creature crossing one lane, right now ----
   The lane's own label says how FAST it runs. This says WHO just crossed it,
   and it has to win the eye against a field of 420 resident glyphs: it is
   bigger, it is stroked, and it drops a shadow the cell glyphs do not have.
   Neutral is the colour of a hop whose envelope carried no species block —
   never a guessed name, and never omitted. */
.hopg{pointer-events:none}
.hopbib{stroke:var(--hot);stroke-width:.9;filter:drop-shadow(0 0 5px rgba(255,255,255,.55))}
.hopbib.neutral{fill:var(--dim)}
.hopring{fill:none;stroke:var(--hot);stroke-width:1.1;opacity:.5}
/* The SAME event under reduced motion. The glyph is placed where a travelling
   one would have ended — the far end of the lane, at the edge of the world it
   reached — and it fades there. It never travels, so there is no movement to
   object to, and the crossing is still shown rather than silently dropped. */
.hopstill{animation:hopstill 1.1s ease-out forwards}
@keyframes hopstill{0%{opacity:0}16%{opacity:1}60%{opacity:1}100%{opacity:0}}
.axis{fill:var(--dim);font-size:11px}

/* ---- legend ---- */
.legend{display:flex;gap:6px 16px;flex-wrap:wrap;font-size:11px;color:var(--dim);
text-transform:none;letter-spacing:0;min-width:0}
.legend>span{white-space:nowrap}
.legend i{display:inline-block;width:22px;height:0;border-top-width:2.4px;
border-top-style:solid;vertical-align:middle;margin-right:5px}
.legend i.open{border-color:var(--lane)}
.legend i.bypass{border-color:var(--warn);border-top-style:dashed}
.legend i.closed{border-color:var(--dark);border-top-style:dotted}
.legend i.box{height:11px;border:1.5px solid;border-top-width:1.5px;border-radius:2px;width:14px}
.legend i.box.live{border-color:var(--live)}
.legend i.box.dark{border-color:var(--dark)}
.legend i.box.hole{border-color:var(--hole);border-style:dashed}
.legend i.bibi{width:12px;height:9px;border:0;background:var(--dim);
border-radius:62% 38% 38% 62%/50%}
.legend i.bibi.unc{background:var(--hole)}
.legend i.bibi.hopi{background:var(--hot);box-shadow:0 0 5px rgba(255,255,255,.55)}
.legend i.eggi{width:9px;height:9px;border:1.5px solid var(--dim);border-radius:50%;
background:none}

/* ---- history strip ---- */
.historyhead{display:flex;align-items:flex-start;justify-content:space-between;gap:12px 20px;
flex-wrap:wrap;margin-bottom:16px}.historyhead h2{margin:0;flex:1 1 520px}
.historyrange{display:inline-flex;gap:4px;padding:3px;border:1px solid var(--line);border-radius:9px;
background:var(--cell)}.historyrange button{font:700 10px/1.2 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
letter-spacing:.06em;text-transform:uppercase;color:var(--dim);background:transparent;border:1px solid transparent;
border-radius:6px;padding:7px 10px;cursor:pointer;white-space:nowrap}.historyrange button:hover{color:var(--text)}
.historyrange button[aria-pressed="true"]{color:var(--ink);background:var(--live);border-color:var(--live)}
.sparks{display:grid;gap:10px;grid-template-columns:repeat(auto-fill,minmax(min(210px,100%),1fr))}
.spark{border:1px solid var(--line);border-radius:11px;padding:11px 12px 9px;background:rgba(14,23,20,.82);
min-width:0;overflow:hidden}
.spark.wide{grid-column:span 2}
@media (max-width:520px){.spark.wide{grid-column:span 1}}
.sparkhd{display:flex;justify-content:space-between;align-items:baseline;gap:8px;font-size:11px;
color:var(--dim)}
.sparkhd b{color:var(--text);font-size:15px;font-variant-numeric:tabular-nums}
.spark svg{display:block;width:100%;height:auto;margin:4px 0 2px}
.sparkft{font-size:10px;color:var(--dim);display:flex;justify-content:space-between}
.sline{fill:none;stroke:var(--live);stroke-width:2;stroke-linejoin:round;stroke-linecap:round}
.sline.deadline{stroke:var(--dark)}
.sarea{fill:var(--live);opacity:.13}
.sarea.deadarea{fill:var(--dark)}
.sbar{fill:var(--lane);opacity:.75}
.sdark{fill:var(--dark);opacity:.16}
.sdotend{fill:var(--live)}
.sdotend.deadend{fill:var(--dark)}
.sbase{stroke:var(--line);stroke-width:1}

/* ---- tables ---- */
.tw{overflow-x:auto;max-width:100%}
table{width:100%;border-collapse:collapse;font-size:12px;min-width:520px}
th,td{text-align:left;padding:7px 9px;border-bottom:1px solid var(--line);white-space:nowrap}
th{color:var(--dim);font:700 10px/1.3 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
letter-spacing:.08em;text-transform:uppercase}
td{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
tbody tr:hover{background:rgba(117,189,242,.045)}
td.num{text-align:right;font-variant-numeric:tabular-nums}
.closed{color:var(--dark)}
.open{color:var(--lane)}
.skip{color:var(--warn);font-size:11px;white-space:normal}
td.spx{white-space:normal;min-width:180px}
.spitem{display:inline-block;margin-right:11px;white-space:nowrap}
.sw{display:inline-block;width:10px;height:8px;margin-right:4px;vertical-align:middle;
border-radius:62% 38% 38% 62%/50%}
.spmore{color:var(--dim)}
.kv{display:flex;justify-content:space-between;gap:8px;font-size:12px;
border-bottom:1px solid var(--line);padding:5px 0}
.kv span:last-child{color:var(--text);font-variant-numeric:tabular-nums}

/* ---- glossary + tooltips ---- */
.term{border-bottom:1px dotted var(--dim);cursor:help}
text.term,tspan.term{text-decoration:underline dotted;cursor:help}
#tip{position:fixed;z-index:60;max-width:320px;background:#07100d;border:1px solid var(--line);
border-radius:10px;padding:11px 13px;font-size:12px;line-height:1.55;color:var(--text);
box-shadow:0 10px 28px rgba(0,0,0,.6);display:none}
#tip b{display:block;color:var(--lane);font-size:10.5px;letter-spacing:.1em;
text-transform:uppercase;margin-bottom:4px;word-break:break-word}
#tip .tipbody{white-space:pre-line}
details.gloss summary{cursor:pointer;color:var(--dim);font-size:12px;letter-spacing:.12em;
text-transform:uppercase;list-style:none}
details.gloss summary::-webkit-details-marker{display:none}
details.gloss summary:before{content:"\25B8 ";color:var(--lane)}
details.gloss[open] summary:before{content:"\25BE ";color:var(--lane)}
.glosslist{margin:12px 0 0;display:grid;gap:10px;
grid-template-columns:repeat(auto-fill,minmax(280px,1fr))}
.glossitem{border-left:2px solid var(--line);padding-left:10px}
.glossitem b{display:block;color:var(--lane);font-size:11px;letter-spacing:.08em;
text-transform:uppercase;margin-bottom:3px}
.glossitem span{color:var(--dim);font-size:12px;line-height:1.6}
footer{padding-block:28px 38px;color:var(--dim);font-size:11px;border-top:1px solid var(--line);
font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}

/* ---- the motion switch ----
   Small, and in the footer rather than the header, because it is a preference
   and not a reading. It is still a CONTROL: the pressed state has to be legible
   at a glance, or a reader cannot tell which of the three is in force. */
.motion{margin-top:10px;display:flex;gap:7px;align-items:baseline;flex-wrap:wrap}
.motion .mbtn{font:inherit;font-size:11px;color:var(--dim);background:var(--cell);
border:1px solid var(--line);border-radius:999px;padding:3px 10px;cursor:pointer}
.motion .mbtn:hover{color:var(--text);border-color:var(--dim)}
.motion .mbtn[aria-pressed="true"]{color:var(--ink);background:var(--lane);border-color:var(--lane)}
.motion .mwhy{font-size:11px;color:var(--dim)}
:focus-visible{outline:3px solid rgba(117,189,242,.75);outline-offset:3px}
@media (prefers-reduced-motion:reduce){html{scroll-behavior:auto}}
@media (max-width:980px){.console-summary{grid-template-columns:1fr;gap:28px}.statusgrid{grid-template-columns:repeat(3,1fr)}}
@media (max-width:860px){.console-nav{min-height:62px;padding-block:10px;flex-wrap:wrap;gap:10px}.consolelinks{width:100%;gap:16px;overflow-x:auto;padding-bottom:3px}.consolelinks a{white-space:nowrap}}
@media (max-width:640px){.console-nav,.console-summary,.tabs,main,footer{width:min(calc(100% - 24px),var(--max))}
.consolelinks{flex-wrap:wrap;overflow-x:visible;padding-bottom:0}.console-summary{padding-block:42px 35px}
.summarycopy{font-size:14px}.statusgrid{grid-template-columns:1fr 1fr;border-radius:12px}.statusitem{min-height:82px;padding:14px}
.statusitem:nth-child(n){border-left:0;border-top:1px solid var(--line)}.statusitem:nth-child(odd){border-left:1px solid var(--line)}
.statusitem:first-child{border-top:0;border-left:0;grid-column:1/-1}.statusitem:nth-child(2){border-top:1px solid var(--line)}
.statusitem.age{grid-column:span 1}.tabs{width:100%;border-left:0;border-right:0;border-radius:0;transform:none;margin-bottom:0;padding-inline:6px}
main{padding-block:12px 48px;gap:12px}.panel{gap:12px}section{padding:14px;border-radius:12px}footer{padding-block:24px 32px}}
</style>
</head>
<body>
<a class="skiplink" href="#content">Skip to live map content</a>
<!-- One creature, defined once for the whole document and drawn by reference by
     every tab: a teardrop body with a mouth notch bitten out of the nose and an
     eye. It faces EAST, which is the way organisms travel. The body sets no
     fill, so each <use> colours it by species; the eye sets its own, so it
     reads on every hue. -->
<svg class="glyphdefs" aria-hidden="true" focusable="false"><defs>
<g id="bib">
<path d="M 4.5 -1.26 L 2.16 0 L 4.5 1.26 C 4.14 3.06, 1.98 4.14, -0.54 4.14 C -3.24 4.14, -5.58 2.16, -6.66 0 C -5.58 -2.16, -3.24 -4.14, -0.54 -4.14 C 1.98 -4.14, 4.14 -3.06, 4.5 -1.26 Z"/>
<circle class="bibeye" cx="1.5" cy="-1.85" r="1"/>
</g>
</defs></svg>
<header class="consolehead">
  <div class="console-nav">
    <a class="brand" href="/" aria-label="Bibites Multiverse home">
      <svg class="mark" viewBox="-9 -7 19 14" aria-hidden="true"><path fill="currentColor" d="M4.5-1.26 2.16 0l2.34 1.26C4.14 3.06 1.98 4.14-.54 4.14-3.24 4.14-5.58 2.16-6.66 0-5.58-2.16-3.24-4.14-.54-4.14c2.52 0 4.68 1.08 5.04 2.88Z"/><circle cx="1.5" cy="-1.85" r="1" fill="#07100d"/></svg>
      <span>Bibites Multiverse</span>
    </a>
    <nav class="consolelinks" aria-label="Primary navigation">
      <a href="/#how">How it works</a><a href="/#join">Join</a><a href="/watch">Watch broadcast</a>
      <a href="https://github.com/jpinedaa/bibites-multiverse">GitHub</a>
      <a class="livepill" href="/live" aria-current="page"><i class="navdot"></i>Live map</a>
    </nav>
  </div>
  <div class="console-summary">
    <div class="console-title">
      <p class="eyebrow">Live evolutionary geography</p>
      <h1>Explore the Multiverse.</h1>
      <p class="summarycopy">See every connected world, follow real migrations, and trace the species and lineages evolving across the map.</p>
    </div>
    <div class="statusgrid" aria-label="Current map status">
      <div class="statusitem"><span class="statuskey">Map shape</span><span class="hdr" id="shape">&hellip;</span></div>
      <div class="statusitem"><span class="statuskey"><span class="term" data-t="population">Population</span></span><span class="hdr"><b id="hpop">&mdash;</b></span></div>
      <div class="statusitem"><span class="statuskey"><span class="term" data-t="migration">Migrations</span></span><span class="hdr"><b id="hmig">&mdash;</b></span></div>
      <div class="statusitem"><span class="statuskey">Connection</span><span class="hdr" id="link"></span></div>
      <div class="statusitem age"><span class="statuskey">Freshness</span><span class="hdr" id="age"></span></div>
    </div>
  </div>
</header>
<nav class="tabs" id="tabs" role="tablist" aria-label="views">
  <button type="button" class="tab" role="tab" id="tab-map" data-tab="map" aria-controls="p-map" aria-selected="true" tabindex="0">live map
    <span class="sub">worlds, lanes, crossings</span></button>
  <button type="button" class="tab" role="tab" id="tab-species" data-tab="species" aria-controls="p-species" aria-selected="false" tabindex="-1">species
    <span class="sub">what is alive, where, and where it came from</span></button>
  <button type="button" class="tab" role="tab" id="tab-settings" data-tab="settings" aria-controls="p-settings" aria-selected="false" tabindex="-1">settings
    <span class="sub">what each world was told to do</span></button>
</nav>
<main id="content" tabindex="-1">
<div class="panel" id="p-map" role="tabpanel" aria-labelledby="tab-map">
  <section>
    <h2>the map
      <span class="legend">
        <span><i class="box live"></i><span class="term" data-t="live">live</span></span>
        <span><i class="box dark"></i><span class="term" data-t="dark">dark</span></span>
        <span><i class="box hole"></i><span class="term" data-t="hole">hole</span></span>
        <span><i class="open"></i><span class="term" data-t="lane">lane open</span></span>
        <span><i class="bypass"></i><span class="term" data-t="bypass">bypass</span></span>
        <span><i class="closed"></i>lane closed</span>
        <span><i class="bibi hopi"></i><span class="term" data-t="hopfeed">a creature crossing, just now</span></span>
        <span><span class="term" data-t="shuttle">two lanes, one each way</span></span>
        <span><span class="term" data-t="wrap">wrap-around</span></span>
        <span><i class="bibi"></i><span class="term" data-t="species">one creature, coloured by species</span></span>
        <span><i class="eggi"></i><span class="term" data-t="egg">an egg</span></span>
        <span><i class="bibi unc"></i><span class="term" data-t="unclassed">no species record</span></span>
      </span>
    </h2>
    <div class="mapstage" id="mapstage">
      <div class="maptools">
        <button type="button" class="mapfull" id="mapfull" aria-pressed="false"
                aria-label="Show the live map in fullscreen">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path class="expand" d="M8 3H3v5M16 3h5v5M8 21H3v-5M16 21h5v-5"/>
            <path class="contract" d="M3 8h5V3M21 8h-5V3M3 16h5v5M21 16h-5v5"/>
          </svg>
          <span id="mapfulltext">fullscreen</span>
        </button>
      </div>
      <div class="mapwrap"><div id="mapbox"></div></div>
    </div>
  </section>

  <section>
    <div class="historyhead">
      <h2>history <span class="note muted">population per world, <span id="historyscope">all recorded history</span>, from the
        archive&rsquo;s own sample file &mdash; each graph fits its visible minimum and maximum;
        a gap is <span class="term" data-t="unknown">unknown</span>, never a zero</span></h2>
      <div class="historyrange" id="historyrange" role="group" aria-label="Population history range">
        <button type="button" data-history-range="all" aria-pressed="true">All time</button>
        <button type="button" data-history-range="24h" aria-pressed="false">Last 24 hours</button>
      </div>
    </div>
    <div class="sparks" id="spark"><span class="muted">loading&hellip;</span></div>
  </section>

  <section><h2>effective <span class="term" data-t="lane">lanes</span>
    <span class="note muted">recomputed here from what the relay broadcasts, for display</span></h2>
    <div class="tw"><table id="lanes"><thead><tr>
      <th>from</th><th><span class="term" data-t="edge">edge</span></th><th>to</th><th>state</th>
      <th class="num"><span class="term" data-t="envelope">envelopes</span></th><th class="num">/min</th>
      <th><span class="term" data-t="bypass">bypassing</span></th></tr></thead>
    <tbody></tbody></table></div></section>

  <section><h2><span class="term" data-t="world">worlds</span>
    <span class="note muted">the detail behind every cell &mdash;
      <span class="term" data-t="speed">speed</span> is how fast that world says it is
      running and, after the arrow,
      <span class="term" data-t="achieved">how fast it actually ran</span>;
      <span class="term" data-t="pace">pace</span> is arrivals queued over the cap they
      wait behind, per <em>simulated</em> minute of that world</span></h2>
    <div class="tw"><table id="worlds"><thead><tr>
      <th><span class="term" data-t="slot">slot</span></th>
      <th><span class="term" data-t="position">pos</span></th>
      <th><span class="term" data-t="peer">peer</span></th>
      <th>state</th>
      <th class="num"><span class="term" data-t="speed">speed</span>
        <span class="muted">&rarr;</span>
        <span class="term" data-t="achieved">got</span></th>
      <th class="num"><span class="term" data-t="population">pop</span></th>
      <th><span class="term" data-t="census">species</span></th>
      <th class="num"><span class="term" data-t="custodyDepth">custody</span></th>
      <th class="num"><span class="term" data-t="pace">pace</span></th>
      <th class="num"><span class="term" data-t="held">held</span></th>
      <th class="num"><span class="term" data-t="bounce">bounces</span></th>
      <th><span class="term" data-t="lastSave">last save</span></th>
      <th>note</th></tr></thead>
    <tbody></tbody></table></div></section>

  <section><h2>totals</h2><div id="totals"></div></section>
</div>

<div class="panel" id="p-species" role="tabpanel" aria-labelledby="tab-species" hidden>
  <section>
    <h2><span class="term" data-t="alive">species alive right now</span>
      <span class="note muted">Every species any world is
        <span class="term" data-t="census">reporting</span> in its census, joined across the
        map, drawn against TIME &mdash; a bar runs from the first
        <span class="term" data-t="crossings">crossing</span> this archive recorded of that
        species to the last, or to the right-hand edge while it is still alive. It is the span
        of the RECORD and <span class="term" data-t="lifespan">not a lifespan</span>. A species
        that used to live here and does not now is not a row of its own: it is drawn only where
        two living lines <span class="term" data-t="branchpoint">part at it</span>.
        The ancestry is what the migration record shows &mdash; when a creature crosses, the
        world it left names its species <em>and that species&rsquo; parent species</em>, and
        this is one generation of that per crossing, chained. So a lineage
        that has never crossed a lane has nothing here to connect it &mdash;
        <span class="term" data-t="noancestry">that is stated per species, never guessed</span>.
        A run of extinct ancestors with no living branch on it
        <span class="term" data-t="collapsed">collapses to one dotted edge</span>, drawn across
        the stretch of time it held the line and labelled with how many generations that was.
        Nothing here is resolved against any world&rsquo;s own
        registry &mdash; this is what the record says.</span></h2>
    <div class="spctl">
      <input id="lfq" type="search" autocomplete="off" spellcheck="false"
             placeholder="search a species…" aria-label="search species">
      <label for="lfsort">order
        <select id="lfsort">
          <option value="family">family</option>
          <option value="pop">population</option>
        </select>
      </label>
      <span class="muted" id="lfcount"></span>
    </div>
    <div class="treestat" id="lfstat"></div>
    <p class="treelegend">
      <span><i class="bibi"></i>alive &mdash; a leaf, or a living ancestor; the glyph&rsquo;s
        colour is the species and nothing else here is coloured</span>
      <span><i class="ringi"></i>an ancestor alive nowhere that is reporting</span>
      <span><i class="bari"></i><span class="term" data-t="lifespan">a bar</span> &mdash; first
        recorded crossing &rarr; last, or &rarr; now while alive</span>
      <span><i class="twi"></i><span class="term" data-t="descends">descended from</span>
        &mdash; the bracket in the name column, and the line dropping onto a bar where the
        record first names that species</span>
      <span><span class="term" data-t="timeaxis">the clock along the top</span> &mdash; left is
        older, the right-hand edge is now</span>
      <span><span class="term" data-t="lineage">hover a row, or tab to it</span> &mdash;
        everything not in its family dims</span>
      <span><i class="chaini"></i>&plus;<b>n</b> &mdash; extinct generations collapsed, drawn
        <span class="term" data-t="collapsed">across the stretch they held the line</span>;
        the number is there because a count is not recoverable from a duration</span>
      <span><i class="revi"></i><span class="term" data-t="beforeparent">recorded before its
        parent</span> &mdash; the link runs backwards to where the child's own record starts</span>
      <span><i class="ringi braini"></i><span class="term" data-t="brainsize">brain size</span>
        &mdash; bigger ring, more neurons and synapses <em>than the other rows drawn</em>;
        the newest genome of it this archive <em>ever read</em>, which for an extinct line may
        be days old &mdash; hover one for the numbers and its age</span>
      <span><i class="doti"></i><span class="term" data-t="minimap">where it lives</span>,
        on the map&rsquo;s own grid</span>
      <span><span class="term" data-t="trend">the 24 h line</span> &mdash; its population
        across the map</span>
      <span>click a row &mdash; or tab to it and press Enter &mdash; for its worlds, its record
        and its parent</span>
    </p>
    <!-- tabindex="-1" is not a tab stop: it is where the focus lands when the
         row that had it is no longer drawn, so a species leaving the census does
         not send the reader back to the top of the page. -->
    <div class="lifewrap" id="lfbox" tabindex="-1"></div>
    <!-- THE BRAIN PANEL. It is BELOW the drawing and not a row in it: an
         aggregate over every genome the archive could read is not a species, and
         giving it a row would give it a row's affordances — a name, a lineage, a
         click that opens a detail about a creature. It shares the drawing's exact
         left and right edges and its tick positions, and nothing else. -->
    <div class="brainwrap" id="lfbrain"></div>
    <p class="treelegend brainlegend" id="lfbrainlgd">
      <span><i class="bsyn"></i><span class="term" data-t="braintrend">median synapses</span>
        &mdash; per genome, in each slice of time</span>
      <span><i class="bhid"></i><span class="term" data-t="hiddenneurons">median hidden
        neurons</span> &mdash; the count ABOVE the fixed 48 every brain is born with</span>
      <span><i class="bbandi"></i>the shaded band &mdash; the middle half of the genomes in
        that slice, a quarter above the line and a quarter below</span>
      <span>each graph uses its own visible minimum and maximum, printed beside the line</span>
      <span><i class="bcovi"></i><span class="term" data-t="braincoverage">how much of it was
        measured</span> &mdash; taller is more of that slice&rsquo;s genomes read, on a
        square-root scale, and never shorter than the amber tick for none of them</span>
      <span>a break in a line is a <span class="term" data-t="braingap">gap</span>, never a
        zero</span>
      <span>a washed stretch is <span class="term" data-t="brainwaiting">waiting</span>, never
        empty</span>
    </p>
  </section>
</div>

<div class="panel" id="p-settings" role="tabpanel" aria-labelledby="tab-settings" hidden>
  <section>
    <h2><span class="term" data-t="ceilings">what the map itself is running with</span>
      <span class="note muted">The <span class="term" data-t="relay">relay</span>&rsquo;s own
        configuration, as it publishes it on every broadcast: the
        <span class="term" data-t="ceilings">ceilings</span> every world here is measured
        against and the oldest helper version this map
        <span class="term" data-t="floor">admits</span>. Everything else on this tab is what a
        world SAYS about itself; these two are the relay&rsquo;s own numbers, so they are
        authoritative &mdash; and a map that publishes no ceilings at all is
        <span class="unknown">unknown</span>, never a map without any.</span></h2>
    <div class="cards" id="mapcard"><span class="muted">loading&hellip;</span></div>
  </section>
  <section>
    <h2><span class="term" data-t="settings">what each world was told to do</span>
      <span class="note muted"><b>Read-only.</b> This page renders what each world reports
        about its own configuration and offers no way to change any of it; a control surface
        is separate, later work. A field no world has told us shows
        <span class="unknown">?</span> &mdash;
        <span class="term" data-t="unknown">never</span> the value the game ships with.</span></h2>
    <div class="cards" id="setcards"><span class="muted">loading&hellip;</span></div>
  </section>
</div>

  <section><details class="gloss" id="glossbox">
    <summary>What am I looking at?</summary>
    <div class="glosslist" id="glosslist"></div>
  </details></section>
</main>
<footer>
  One source, no polling of anybody: everything here comes from the broadcasts this
  <span class="term" data-t="archive">archive</span> receives as a read-only subscriber and from
  the <span class="term" data-t="envelope">envelope</span> copies it records. The
  <span class="term" data-t="relay">relay</span> is the authority for what a world actually
  routes; these lanes are recomputed for display. An
  <span class="unknown">unknown</span> is a world that reported nothing, or reported it too long
  ago &mdash; never a zero. Every creature in transit is in exactly one side&rsquo;s
  <span class="term" data-t="custody">custody</span>, which is how the system keeps its
  <span class="term" data-t="exactlyonce">exactly-once</span> promise. The
  <span class="term" data-t="rawname">species names</span> are printed exactly as the world that
  owns them holds them, spacing and all, and nothing here tidies one. Rates are measured over a
  short <span class="term" data-t="flow">flow window</span>, not over all time. The
  <span class="term" data-t="settings">settings</span> a world reports are
  <span class="term" data-t="readonly">read-only</span> here: this page changes nothing, anywhere.
  The <span class="term" data-t="genealogy">family tree</span> is what the migration record
  shows and nothing else: lineages that have never crossed a lane are not connected here, and
  the count of those is printed on the tab rather than left for a reader to notice. A
  <span class="term" data-t="lifespan">bar</span> is the span of that record and not a lifespan.
  The three views share one poll and each has a link of its own &mdash;
  <code>#map</code>, <code>#species</code>, <code>#settings</code>; <code>#tree</code> was the
  genealogy&rsquo;s own tab before it and the census list became one drawing, and it still lands
  there. JSON:
  <code>/api/status</code> (live), <code>/api/hops</code> (the last minute of crossings,
  bounded in time and in count, and kept out of the durable sample file on purpose),
  <code>/api/species/tree</code> (what is alive, joined to the crossing record and reduced to its
  branch points &mdash; its own endpoint so a derived tree is not written to disk once a minute),
  <code>/api/species</code> (the same alive set as a flat index, which is what
  <code>ringstat</code> reads),
  <code>/api/species/trends</code> (every living species&rsquo; recent population shape in one
  answer, which is what the trend column is drawn from),
  <code>/api/species/brains?from=&amp;to=&amp;buckets=</code> (how complicated the brains
  crossing this map have been getting, over the held sample, re-aggregated onto whatever window
  is asked for &mdash; which is what lets the panel share the family tree&rsquo;s own clock),
  <code>/api/species/history?key=</code> (one species, split per world) and
  <code>/api/history</code> (downsampled, <code>?hours=</code>, <code>?buckets=</code>).
  <div class="motion" id="motion">
    <span class="term" data-t="motion">motion</span>
    <button type="button" class="mbtn" data-m="auto">auto</button>
    <button type="button" class="mbtn" data-m="on">on</button>
    <button type="button" class="mbtn" data-m="off">off</button>
    <span class="mwhy" id="motionwhy"></span>
  </div>
</footer>
<div id="tip"></div>
<script>
"use strict";
var $ = function(s){ return document.querySelector(s); };
var SVGNS = "http://www.w3.org/2000/svg";

/* ------------------------------------------------------------------ glossary
   One table feeds BOTH the hover tooltips and the section at the bottom, so the
   short answer and the long answer can never drift apart. Plain English, for a
   curious player rather than an operator. */
var G = {
 world:["world","One running copy of the game — a whole little ecosystem with its own creatures, food and weather. Six of them are wired together here, and creatures can walk out of one and into the next."],
 slot:["slot","A world's permanent seat on the grid. Slot 3 is always slot 3: it can be moved to a different square, and it can go offline for a day, and it is still slot 3 when it comes back."],
 position:["position","Where a seat sits on the map, written (column, row). Column 0 is the far left and row 0 is the bottom row."],
 peer:["peer","The name the little helper program beside each world answers to on the network — 'slot-4', for instance. One peer per world; the worlds never talk to each other directly."],
 lane:["lane","A road from one world to the next, with a direction. Every side of a world is now both an exit and a door: a creature can leave east and another can arrive from the east on the same side at the same time. So each pair of neighbours is joined by TWO lanes drawn side by side, one arrow each way, and each of the two carries its own traffic and can be open while the other is not."],
 edge:["edge","The side of a world a creature leaves by, and also the side arrivals come in on. E is east (right), N is north (up), W is west (left), S is south (down). All four are exits and all four are doors."],
 shuttle:["two-lane pair","When an axis has only two worlds on it, going one way and going the other way land you in the same place — so both lanes of that axis join the same two worlds, and they carry roughly twice the traffic of a single lane. On this map every COLUMN is like that. It also means one death closes both: if the world above you is also the world below you, losing it closes north and south together."],
 wrap:["wrap-around","The map is a doughnut, not a sheet of paper. Walk east off the right-hand column and you arrive in the left-hand one; walk north off the top row and you arrive at the bottom. Nothing ever falls off an edge. On the map those lanes are drawn leaving one side and re-entering the opposite side, with the map edge marked."],
 live:["live","This world is connected right now: it can send creatures and it can receive them."],
 dark:["dark","This world is not connected right now — the game is closed, the machine is off, or the network dropped. Its seat stays on the map, and the worlds pointing at it reach past it instead."],
 hole:["hole","An empty seat: a square inside the map that no world has claimed. Lanes step straight over it in both directions."],
 bypass:["bypass / route-around","When a world goes dark, the lane that pointed at it does not shut down — it reaches over that world to the next one that is actually there, and the current keeps flowing. That is the good news and the danger at once: everything can look healthy while a world has been dead since yesterday. So a bypass is always drawn as a warning, with what it is skipping and for how long."],
 custody:["custody","Exactly one side is holding a travelling creature at any instant, and it is written to disk before it is handed over. The sender keeps it until the receiver confirms the creature arrived; only then does the sender let go."],
 custodyDepth:["custody depth","How many creatures this world is holding mid-journey right now — sent but not yet confirmed, plus arrived but not yet let into the world."],
 pacedDepth:["paced depth","Arrivals are let into a world at a capped rate so a burst cannot flood it. This is how many are queued up waiting their turn. A queue that never drains means the cap is set too low."],
 speed:["simulation speed","How fast a world is running, as the game itself reports it: ×5 means the game is set to pass five simulated seconds for every real second. It is the speed control inside that copy of the game, and each world has its own — one can be asking for ×100 while its neighbour sits at ×1. A paused world reports ×0. A world at a different speed from its neighbours is not a fault; it only means the two experience the traffic between them at different rates, which is why arrivals are paced on the receiving world's OWN clock and not on the wall clock. This is what the game is TRYING to do; the number after the arrow is what it managed."],
 achieved:["speed actually delivered","The second half of a cell's speed, written ×100 → ~×12: the world is set to run a hundred times real time and is really running about twelve. This one is not reported by anybody — it is measured here, by watching that world's own clock against the wall clock for the last minute, so it is the only figure on this page that says what a world genuinely did. A machine can only run so many worlds so fast: each drawing of the screen advances the simulation by one small step, so however high the speed is set, the real rate is capped by how fast that computer can draw. Setting ×100 on a machine that can deliver ×12 buys nothing, and before this number existed the page said ×100 and looked healthy. The two agreeing means a world is keeping up. It shows nothing at all until it has watched long enough to be sure — just after this program starts, or just after a world comes back — because a rate measured over two seconds would jump about and mean nothing."],
 pace:["pace","Two numbers about arrivals into this world: how many are queued waiting to be let in, and the cap they are queued behind. The cap counts per SIMULATED minute of this world — so a world at ×10 gets through its allowance ten times faster in real time, and at the same rate as the world itself experiences it. Queued 0 against any cap is a world keeping up. A queue that never drains means the cap is set too low. A world whose helper program is too old to report its cap shows a ? there: unknown, never the shipped default, which has been changed three times."],
 held:["held","A creature whose destination went dark while it was travelling. It waits, quietly retrying; if the destination stays gone long enough it is sent back where it came from rather than lost."],
 bounce:["bounce","A migration that gave up and returned to the world it started in. It is a thing you get told about, never a silent repair."],
 migration:["migration / hop","One creature's trip from one world to the next. Every hop is copied to the archive as it happens, which is where all the counts on this page come from. On the map a lane carries a number — how many crossings a minute it has been measuring — and when a hop actually happens, that creature itself, drawn in its own species' colour, sets off along the lane and travels to the far world. Nothing else moves on a lane: what you see travelling is always a real creature that really crossed."],
 hopfeed:["hops just now","The last minute of crossings, kept in memory and nowhere else. It is a record of who CROSSED, which is a different question from who LIVES in a world: the creatures drawn inside a cell come from that world's own census, and these come from the migrations the archive was copied on. Never add the two together. A hop whose message carried no species name travels as a plain grey creature — unknown, never guessed."],
 motion:["motion","Whether a crossing TRAVELS across this map or simply appears where it landed. On 'auto' the page follows your system's reduce-motion setting — Windows calls it 'Animation effects' and macOS calls it 'Reduce motion' — and a great many machines have that switched off for reasons that have nothing to do with this page. Either way a crossing is still drawn and still counted: with motion reduced, the creature appears for a moment at the world it reached, in its own species colour, and fades. 'on' and 'off' override the system setting in both directions, and this browser remembers which you chose."],
 lastSave:["last save","Every world saves itself to disk every few minutes and sends back a receipt — when it saved, how big the file was, how long it took. This is the age of the newest receipt from that world."],
 population:["population","How many living creatures the world holds, as its own game reported it. Each little creature drawn inside a cell is one of them."],
 species:["species","A kind of creature, with a two-part name like 'Cyanea velox'. Every creature in a cell is drawn in its species' colour, and the same species is the same colour in every world, so you can see a kind spreading across the map. Hover one to see how many there are and where else it lives."],
 census:["species census","Once a second each world lists which species are living in it and how many of each, and that list travels here with everything else it reports. It is a picture of that world right now — not a record of what has crossed between worlds, which is a different question with a different, wrong-looking answer. The list is capped at the 32 most numerous species; when a world has more, the page says so."],
 egg:["egg","An unhatched egg, drawn as a small hollow ring beside the creatures of its species. Eggs are counted separately from creatures everywhere on this page, because a world's population count does not include them."],
 unclassed:["no species record","Creatures the world counted but did not file under any species. It is an ordinary state, not a fault — and it is drawn in grey so the gap is visible rather than quietly rolled into whichever species happens to be nearby."],
 rawname:["as the world spells it","A species name is copied here exactly as the owning world holds it, including stray or doubled spaces. Nothing on the way tidies one, because a tidied name is a name the player cannot find in their own game — and two spellings a world keeps apart really are two records in it."],
 unknown:["unknown","A number that is missing, or older than the freshness rule allows, is shown as unknown — never as zero. A world that has told us nothing is unknown, not empty. An honest gap beats a confident zero."],
 exactlyonce:["exactly-once","The promise the whole thing is built on: a creature is never duplicated. It is in one world, or in transit, or in the next world — never in two places at once. Very rarely one can be lost instead, and a lost creature simply reads as a natural death."],
 relay:["relay","The small server in the middle. Every world's helper connects to it and to nothing else, so there is exactly one map and one set of rules."],
 archive:["archive","The program serving this page. It listens to everything the relay broadcasts, keeps a permanent record of every migration and a copy of every genome, and never asks a world for anything — so watching can never slow the traffic down."],
 envelope:["envelope","One message carrying one creature between two worlds. The counts here are the envelopes this archive was copied on."],
 epoch:["epoch","A counter the relay bumps every time the map itself changes. If it jumps, a world joined, left or moved."],
 genomegap:["genome gap","A genome the archive knows exists — it has the fingerprint — but has not managed to fetch a copy of yet. It keeps the fingerprint forever. Whether it can still fetch a copy tomorrow depends on the retention horizon below: with no horizon set, yes, indefinitely; with one set, the asking stops once the crossing is older than the horizon, because a copy fetched after that would be deleted again on the next sweep."],
 horizon:["retention horizon","How long a copy of a genome is kept after it was last stored or last read. THE RECORD OF WHAT HAPPENED IS KEPT FOREVER AND IS NEVER AFFECTED BY THIS — every crossing, every fingerprint, every family link stays. What ages out is the genetic material itself, which is the bulky part, and only if nobody asked for it inside the horizon. This row is absent entirely when no horizon is set, which is the default: then nothing is ever deleted."],
 flow:["flow window","The per-minute rates on this page are measured over the last few minutes, not over all time, so they describe what is happening now."],
 alive:["alive right now","The species list is built from what the worlds are reporting THIS SECOND, and from nothing else. A kind that lived here yesterday and died out is not on it, however many times it crossed a lane while it was here. That is a deliberate refusal: a list built from crossings would be a list of travellers and their ancestors, which is a different thing from a list of residents and reads as though extinct kinds were still alive."],
 crossings:["crossings","How many times this archive has recorded a creature of this species walking from one world into another, since the archive started keeping records. It is a count of TRIPS, not of creatures: one restless creature crossing ten times counts ten. A species with no crossings is not a species that never moves — it may simply have arisen after the last time anything of its kind travelled."],
 endemic:["endemic","This species lives in exactly one world. It may have been born there and never left, or it may be the last holdout of something that used to be everywhere — the crossing count beside it is the hint. Endemic is not a warning; most new species start this way."],
 everywhere:["everywhere","This species is alive in every world that is currently reporting a census. It only claims that when at least two worlds are reporting: with one world answering, 'everywhere' and 'endemic' are the same sentence, and saying both would dress a single world's list up as a finding about the map."],
 excluded:["never exported","This species is on at least one world's exclusion list, which means that world never lets one of them out through a lane. It explains something otherwise baffling: a world can be full of a kind that never appears on any road out of it. The rule belongs to that world alone and is applied inside its own game — no other world, and nothing on this page, enforces or can enforce it. A species can be excluded by one world and travel freely from another."],
 seedstock:["seed stock","The kind a world was seeded with, and the one kind on this map that goes nowhere: EVERY world it lives in has it on the list of species that world refuses to export, so while that stays true nothing of it can leave anywhere. That is a stronger fact than 'never exported', which only says SOME world refuses it — a species one world holds back can still travel freely out of another. Its bar would run the full width of the drawing from the moment the record starts and say nothing about the map, so the rows leave it out by default and the timeline is measured without it; the line above says how many are out and offers to show them, and a search for one finds it whether or not they are shown. It is a rule and not a name: any species every one of its worlds refuses to export reads the same way, and a species alive nowhere is never seed stock — the ancestors this drawing keeps are kept."],
 speciesgenomes:["distinct genomes","How many genuinely different genetic makeups of this species have crossed a lane. Two creatures of one species are rarely identical; this counts the distinct ones the archive has fingerprints for. A big number beside a small population means a kind that is changing fast."],
 parentspecies:["parent species","The species this one was recorded as splitting off from, as the world that named it reported at the time. One crossing carries one generation of this, and the drawing on this tab is what happens when you chain them: if this species' parent has a parent of its own, recorded by some other crossing, the record joins them up, and the line dropping onto this species' bar is that join. Nothing is resolved against any world's own register — the register lives inside one copy of the game and only that copy can read it — so what a family tree here says is what the record says, which is a smaller and more honest claim."],
 genealogy:["the family tree","Every species alive right now, arranged by who came from whom. The information comes from one place: when a creature walks out of a world, that world names the creature's species AND the species it split off from. One crossing tells you one generation. Thousands of crossings, chained together, tell you the shape of the family — and on this map that shape runs about forty generations deep. What is drawn is only the part that still matters: the living species, and the ancestors where two or more living lines part company. Everything else is left out, and where a whole run of ancestors is left out the edge says how many."],
 branchpoint:["a branch point","An ancestor that is drawn even though nothing of its kind is alive anywhere, because two or more living species descend from it by different children. It is the answer to 'how are these two related' — the most recent point their lines were the same. An ancestor with only ONE living line below it is not a branch point and is not drawn: it would be a step in a corridor with no doors."],
 timeaxis:["the clock along the top","This drawing is a CALENDAR, not a ranking. Every position across it is a real moment in UTC, the dates and times along the top are that clock, and the right-hand edge is NOW — which is why every living species' bar reaches it. The left-hand edge is the OLDEST BAR ON THE PICTURE and nothing more: it is not the beginning of the map, of the record, or of anything else, and the axis re-fits itself as the rows change, so revealing a hidden row can widen it. The record itself usually reaches further back than the left edge does, and the caption at the left margin says how far. EVERY horizontal distance on this drawing is time and there is no exception to that any more: the dotted run that stands for collapsed generations is drawn across the real stretch those generations held the line, and the number printed on it is there only because a count cannot be recovered from a duration. Vertical distance, by contrast, means nothing at all — it is the order the rows are laid out in."],
 descends:["descended from","Who came from whom, drawn twice, because the two halves answer different questions. IN THE NAME COLUMN a bracket joins a parent row to each of its children and the children are indented one step, so the shape of the family reads straight down the page even when the bars themselves are far apart. IN THE PLOT a line lands on the child's bar at the moment the record FIRST NAMES that child — that is the one horizontal position the join can honestly have. Where it LEAVES the parent is the other half of the same honesty: the last moment the parent's own record supports, which for most links is that same instant and is then a plain drop. Where the parent's record stopped first, the link travels along the child's row across the stretch between them, and where the child's record started FIRST it runs backwards in amber instead. A row with no bracket has no parent on this drawing: it is a root, and the badge on it says whether the record holds ancestors above it anyway."],
 lineage:["one family, lit","Point at a row — or move to it with the Tab key — and everything that is not related to it fades: what stays lit is the line of ancestors above it, all the way to its root, and every species below it. It is the fastest answer to 'how is this one related to the rest', and it needs no click, changes nothing, and moves nothing on the page. Pressing Enter or Space on a focused row opens its detail, exactly as clicking it does."],
 collapsed:["+n generations","A run of ancestors that all died out, with no living branch anywhere along it, drawn as ONE dotted edge with the number of generations it stood for. Drawing all of them would be a column of names nothing alive belongs to; leaving the number off would make a distant cousin look like a sibling. THE DOTTED RUN IS ON THE CLOCK, exactly like every other horizontal distance here: it starts where the ancestor's own record stops and ends where the descendant's record starts, so where it sits and how long it is are the real stretch of time in which this line was carried only by species this drawing does not draw. That stretch used to be left blank — on this map it is forty hours wide — with a short mark beside it whose length counted generations instead, which put two different scales on one drawing. The number is still printed because a count of generations cannot be recovered from a duration: forty generations and one generation can cross the same forty hours. That is now the only thing the number is for. Where there is no such stretch — the ancestor is still alive, or the descendant's first crossing falls inside the ancestor's own span — nothing is drawn across, and the number stands on its own: the record says those generations sat somewhere inside a stretch the ancestor itself occupies, and it does not say where."],
 beforeparent:["recorded before its parent","A species whose first recorded crossing is EARLIER than its parent's own. The relationship is real and both dates are the record's: ancestry here is a by-product of TRAVEL, so a species is first seen when it happens to walk into another world and not when it arose — and the younger of two kinds can easily be the first to make that trip. So the parent's bar can start hours after the child's, and the honest drawing of that is not a line dropping onto a stretch of the parent's row where the parent has no bar. This page will not invent a bar to tidy it up. Instead the link leaves the parent at the earliest moment the parent's record supports, which is the left end of its bar, marks that point with a ring, and runs BACKWARDS in amber to where the child's own record begins. Amber and backwards for the same reason: it is not ordinary descent, and it is the one link on this drawing that must be read twice."],
 lifespan:["the bar, and what it is not","A species' bar starts at the first crossing THIS ARCHIVE RECORDED of it and ends at the last one, or at the right-hand edge while the species is still alive somewhere. It is not a lifespan and this page never calls it one. A kind can have lived for days before anything of it walked into another world, and a kind whose bar stopped last Tuesday may be alive and simply staying at home — what stopped is the record, and the only honest thing a record can draw is itself. The one place the drawing goes beyond that is an ancestor no crossing of its own was ever recorded for: its bar begins at its earliest recorded DESCENDANT, because that descendant's crossing named it as a parent, so the record does support 'it existed by then' and supports nothing earlier. Those bars are drawn hollow."],
 brainsize:["brain size","The ring at the end of a species' bar. Every creature carries a brain \u2014 a little network of neurons wired together by synapses \u2014 and this is how big THE NEWEST GENOME OF THAT SPECIES THIS ARCHIVE EVER READ is, COMPARED WITH THE OTHER SPECIES ON THIS DRAWING. That comparison is the whole of what the size means, and it is worth being plain about: the smallest brain among the rows drawn right now is the smallest ring, the largest is the largest, and everything else is placed between them. It is not an absolute size, and two rings of the same size on two different days are not the same brain. The scale re-fits whenever the drawn set changes \u2014 a search, a revealed row, a species leaving the census \u2014 exactly as the clock along the top re-fits to the bars it holds. It is drawn that way because the alternative was measured and said nothing: against a fixed scale the species on this map differed by a seventh of a pixel, so a kind carrying a third more brain than its neighbour drew the same ring. HOVER A RING FOR THE REAL NUMBERS \u2014 the counts are what the size stopped carrying, and they are on the row's own tooltip too. EVER READ, NOT STILL HELD, and the difference matters most on the rows you would otherwise never see a ring on. The measurement is written down the moment a genome is read and is kept afterwards, so it outlives the copy it came from: an ancestor nothing is alive of keeps the last brain this archive managed to see of it, forever, even though nothing is fetching that species\u2019 genomes any more and every copy has since been deleted. The cost is that a ring can be OLD. Hover it \u2014 it says when the genome it was read from crossed, and a species extinct for three days carries a three-day-old reading drawn beside a living species\u2019 current one. Comparing them is fair (it is what a trend through time looks like on the tree itself) as long as you know which is which. WHERE THERE IS NO RING THERE IS NO ANSWER, which is not the same as a small brain: this archive has never once managed to read a genome of that species. The record of the crossing stays forever either way."],
 braintrend:["brains over time","The panel under the drawing. It is the same clock as the tree above it \u2014 same left edge, same right edge, same tick marks \u2014 and it shows how complicated the brains crossing this map have been getting. The top line is the MEDIAN NUMBER OF SYNAPSES in one genome: the middle genome of that slice of time, half above and half below. The shaded band around it is the middle half of that slice \u2014 a quarter of the genomes above the line, a quarter below \u2014 so a widening band is a population spreading out and a narrow one is a population agreeing with itself. WHAT IT IS MEASURED OVER: every distinct genome this archive holds a copy of whose crossing falls in that slice, counted ONCE each however many times that same creature travelled. Which genomes the archive happens to hold is not chosen by anything about the creature \u2014 the queue that fetches them walks a list ordered by the genome\u2019s own fingerprint, and the most travelled species on this map is over-represented among the copies held by about one part in a hundred \u2014 so this is a fair sample of what crossed and not a sample of what happened to be interesting. It is not a sample of what LIVES here: only creatures that travelled are in it."],
 hiddenneurons:["hidden neurons","The neurons a brain has BEYOND the ones every bibite is born with. Every brain on this map starts with the same fixed set of 48 \u2014 the senses it reads the world with and the muscles it acts with \u2014 and that 48 never varies: it is the smallest count in every one of the tens of thousands of genomes this archive has read. So the raw neuron number barely moves even while brains change enormously, and reading growth off it understates what happened by about sevenfold. This is the count above that floor, which is the part that is actually being invented: the interior of the brain."],
 braincoverage:["how much was measured","The strip of little columns along the bottom of the brain panel: for each slice of time, how much of what crossed this archive was able to look inside. A tall column is most of it; a short one is a few. THE HEIGHTS ARE ON A SQUARE-ROOT SCALE and the shortest column is three pixels tall, which is two decisions worth knowing about. Coverage on this map runs from under half a per cent to about a third, so on a straight scale the whole range that actually happens would live in the bottom four pixels and the difference between a slice read once and a slice read a third of the way through would be invisible; the square root spends the strip on the range that occurs. And the floor keeps every column that means WE READ SOME OF IT taller than the amber tick that means we read none, which a straight scale did not: below about a fifth the mark for 'some' was drawing shorter than the mark for 'none'. Full height still means all of it, and the exact share is in the tooltip. AN AMBER TICK IS NOT A SHORT COLUMN \u2014 it means the record holds crossings in that slice and not one of their genomes was ever read, so there is no line there at all. An empty space is a slice with no crossing recorded in it. IT IS WORST AT THE RIGHT-HAND EDGE, which is the part you look at hardest, and that is not decay: a genome is asked for after its crossing is recorded and the answers arrive over the following days, so the newest slices are the ones still filling in. Measured on this map: about 42% of a slice is readable in its first six hours and about 97% after five days. The line over a thinly-measured slice rests on less evidence, and this is where you can see that rather than having to assume it."],
 brainwaiting:["waiting, not empty","A washed stretch with a dashed edge on the brain panel, and a state of the REQUEST rather than a fact about the map. The panel asks for exactly the window the drawing above it is showing, so whenever that window moves — you reveal the seed stock and the left edge jumps back four days, you resize the box across a bucket — there is a stretch of clock that nothing has been measured for yet. It lasts about as long as one request. It is drawn because the alternative is worse: left blank, that stretch would be wearing the strip's own mark for a slice with no crossing recorded in it, and this panel would be asserting an absence in the record that it has simply not asked about. On this map that mistake was measured at 56% of the plot, over a stretch really holding 74 measured points and 433,627 crossings."],
 braingap:["a gap, not a zero","A break in either line means this archive measured no genome at all in that stretch, and it is drawn as a break on purpose. A zero would say the creatures on this map had no brains, which is a statement about the world made out of a hole in the record. Two quite different things make a hole: nothing crossed (the map was down \u2014 45 of this record\u2019s first 183 hours had no crossing at all, including one stretch of a whole day), or things crossed and none of their genomes were ever read. The strip below the lines tells you which."],
 minimap:["where it lives","The little grid of dots beside a species, laid out the way the map itself is laid out — this map is three worlds wide and two high, so it is three dots by two, and a different map would draw a different grid. A filled dot is a world where this species is alive right now. A hollow dot is a world that sent its census and did not name it. A dashed amber dot is a world reporting no census at all, which is UNKNOWN and never 'not there'. A seat nobody has claimed is drawn as nothing."],
 trend:["the 24-hour line","A small line of this species' population across the whole map over the last day, from the archive's own sample file — the shape, not the numbers. A gap in the line is a stretch where no world reported a census, which is unknown and never a zero. Every one of these lines comes from a single answer covering every living species at once, so the column costs one request rather than one per species. This record began when the archive did, so a short line is a short record and not a young species."],
 recordfloor:["the record begins here","A species drawn with nothing above it, and ancestors above it all the same. The number beside the label is how many generations of them the record holds: every one is extinct with no other living line, so the whole run is collapsed into the row you can see rather than drawn as a column of names nothing alive belongs to. Above the top of that run the record simply stops: ancestry here is only ever carried by a crossing, and the date on the tab is the earliest crossing this archive holds that named a parent at all — anything older than it is a crossing that named none. So the top of a family here is the edge of the record, not the first creature of its kind. That is why the game's starting species is not the root of this tree: its descendants are all here, but the links back to it were never recorded, and a link nobody recorded is not one this page will draw. THE DATE IS USUALLY OLDER THAN THE PICTURE, which is why it is printed at the left margin of the clock rather than drawn on it: the drawing is fitted to the oldest bar it actually holds, and reserving the space back to this date would leave most of the plot empty and more of it empty every hour. Where the date does fall inside the picture it is drawn there, as a dashed line, because then it separates a stretch where no family line can begin from one where they can."],
 noancestry:["no recorded ancestry","This species is alive, and no crossing of it has ever named a parent species — so the record cannot say where it came from, and it is drawn on its own. That is not a fault in the species and not a fault in the map: ancestry here is a by-product of TRAVEL, and a kind that has stayed home has left no record to carry it. A separate note is used for a species whose ancestry IS recorded but whose whole family has died out; those two are different facts and are never given the same label."],
 settings:["settings","What a world was TOLD to do, as opposed to what it is doing. Everything else on this page is a measurement or a receipt; these are the knobs behind them — how often it saves, which species it refuses to export, whether its edges wrap. They are the reason a number elsewhere looks the way it does, and they are read-only here on purpose."],
 readonly:["read-only","This page shows settings and changes none of them. Changing another machine's world from a web page is a much bigger thing than showing one — it needs a login, a rule about who may change what, an answer for what happens when two people change the same thing at once, and a record of who changed it — and none of that is a small extension of showing a value. It is deliberately left for later."],
 savepolicy:["save policy","Three numbers that together answer 'what happens to this world if its machine stops'. How many minutes between saves — where 0 means the timer is OFF, which is a real setting and not a missing one, and is the explanation for a world that never shows a save. How many old saves it keeps beside the live one. And whether it saves when the game is closed."],
 savekeep:["saves kept","How many previous saves this world keeps on disk beside the current one. They rotate: the newest pushes the oldest out. It is the difference between being able to go back one step after something goes wrong and being able to go back several."],
 worldwrap:["world wrapping","Whether that world's own edges join up, so a creature walking off one side reappears on the other instead of piling up against a wall. This system needs it on: the whole map is a doughnut and a world that does not contain its own creatures leaks them into a corner. The world reports this setting and nothing here ever changes it, which is exactly why it is worth showing — it is how you check, from another machine, that nobody turned it off."],
 modversion:["mod version","The version of the small plugin running inside that copy of the game. It is a label, not a promise: what a world can actually report is decided field by field, by whether the field arrives. Use it to tell two machines apart, not to predict what one of them can do."],
 contractaversion:["protocol version","Which version of the language the game plugin and its helper program are speaking. It is the single most useful thing on this page for reading a gap: when a world shows '?' where its neighbours show a number, this says whether that world's plugin is simply too old to have anything to say."],
 simsize:["world size","How big that world is, as its own game reports it — the distance from the middle to an edge. Worlds of different sizes are wired together happily; it only means a creature crosses a small one faster."],
 exportedges:["export edges","The sides of a world that have a capture band running on them: the sides a creature can leave by. Every side can receive an arrival; these are the ones that can send. Which of them actually has a road to somewhere is the map's business, not the world's."],
 ceilings:["the map's ceilings","How much traffic the relay in the middle lets one world put on the wire: messages a second, bytes a second, the size of a single message, how often a world may claim its seat, how many genomes it may ask a neighbour for in a minute, and how many watchers like this page may listen at once. THE RELAY PUBLISHES THE NUMBERS IT IS ACTUALLY RUNNING WITH — every one of them is a knob its operator can turn, so these are the real ceilings and not the values this page was built with. A world that goes over one is disconnected on its own and nothing else changes: its neighbours reach past it exactly as they do for any world that goes dark, and the message it is disconnected with names which ceiling it crossed and by how much. A map that publishes none is running a relay older than the table itself — unknown, and never 'there are no limits'."],
 floor:["oldest version admitted","The oldest version of the helper program this map still lets in. One below it is refused at the door and told both numbers, so it reads as an out-of-date machine rather than as a dead world. Most maps set none, and none is a real answer rather than a missing one: every compatible version is admitted."]
};

(function buildGlossary(){
  var keys = ["world","slot","position","peer","lane","edge","shuttle","wrap","live","dark","hole",
    "bypass","migration","hopfeed","envelope","population","species","census","alive","egg","unclassed",
    "rawname","endemic","everywhere","excluded","seedstock","crossings","speciesgenomes",
    "parentspecies",
    "genealogy","lifespan","timeaxis","descends","lineage",
    "branchpoint","collapsed","beforeparent","noancestry","recordfloor",
    "minimap","trend","brainsize","braintrend","hiddenneurons","braincoverage","braingap",
    "brainwaiting",
    "speed","achieved","pace","custody","custodyDepth","pacedDepth","held","bounce",
    "settings","readonly","savepolicy","savekeep","lastSave","worldwrap","modversion",
    "contractaversion","simsize","exportedges","ceilings","floor",
    "unknown","exactlyonce","relay","archive","epoch","genomegap","horizon","flow"];
  var h = "";
  for (var i=0;i<keys.length;i++){
    var g = G[keys[i]];
    if (!g) continue;
    h += '<div class="glossitem"><b>'+esc(g[0])+'</b><span>'+esc(g[1])+'</span></div>';
  }
  $("#glosslist").innerHTML = h;
})();

/* Tooltips: hover on a pointer, tap to toggle on a phone. Delegated, so terms
   drawn later into the SVG work without being wired up one by one.

   TWO registries, ONE element and ONE set of dismissal rules. G above is the
   fixed glossary, keyed by data-t. SP below is DYNAMIC, rebuilt from every poll
   and keyed by data-s — one entry per species run drawn on the map. A species
   entry's text is ATTACKER-CHOOSABLE (contract-b-m4.md §13 item 7), so the tip
   is filled by BUILDING NODES AND SETTING textContent, never by assigning
   markup. The glossary goes down the same path: one code path cannot rot into
   two, and the safe one is the only one. */
var tip = $("#tip"), tipFor = null, tipBody = null, tipHead = null, SP = {};
function tipContent(el){
  var k = el.getAttribute("data-s");
  if (k && SP[k]) return SP[k];
  var g = G[el.getAttribute("data-t")];
  return g ? {title:g[0], body:g[1]} : null;
}
function showTip(el, x, y){
  var c = tipContent(el);
  if (!c) return;
  if (!tipHead){
    tipHead = document.createElement("b");
    tipBody = document.createElement("div");
    tipBody.className = "tipbody";
    tip.appendChild(tipHead); tip.appendChild(tipBody);
  }
  tipHead.textContent = c.title;
  tipBody.textContent = c.body;
  tip.style.display = "block";
  var w = tip.offsetWidth, h = tip.offsetHeight;
  var left = Math.min(Math.max(8, x - w/2), window.innerWidth - w - 8);
  var top = y - h - 14;
  if (top < 8) top = y + 20;
  tip.style.left = left+"px"; tip.style.top = top+"px";
  tipFor = el;
}
function hideTip(){ tip.style.display = "none"; tipFor = null; }
var TIPSEL = "[data-t],[data-s]";
document.addEventListener("mouseover", function(e){
  var t = e.target.closest ? e.target.closest(TIPSEL) : null;
  if (t) showTip(t, e.clientX, e.clientY);
});
document.addEventListener("mouseout", function(e){
  var t = e.target.closest ? e.target.closest(TIPSEL) : null;
  if (t && t === tipFor) hideTip();
});
document.addEventListener("click", function(e){
  var t = e.target.closest ? e.target.closest(TIPSEL) : null;
  if (!t){ hideTip(); return; }
  if (t === tipFor){ hideTip(); return; }
  showTip(t, e.clientX, e.clientY);
});
window.addEventListener("scroll", hideTip, {passive:true});

/* ------------------------------------------------------------------ helpers */
function esc(s){
  return String(s==null?"":s).replace(/[&<>"']/g, function(c){
    return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]; });
}
function ms(v){
  if (v==null) return "unknown";
  var s = Math.floor(v/1000); if (s<60) return s+"s";
  var m = Math.floor(s/60); if (m<60) return m+"m "+(s%60)+"s";
  var h = Math.floor(m/60); return h+"h "+(m%60)+"m";
}
function num(v){ return v==null ? '<span class="unknown">unknown</span>' : v; }
function kv(k,v){ return '<div class="kv"><span class="muted">'+k+'</span><span>'+v+'</span></div>'; }
function t(term, label){ return '<span class="term" data-t="'+term+'">'+esc(label)+'</span>'; }
function rate(v){ return (v>=10 ? v.toFixed(0) : v.toFixed(1)); }

/* ------------------------------------------------------------------ the map */
/* CW/CH is a cell, GX/GY the gutter a lane runs through, MG the outer margin the
   wrap lanes live in: WOUT is where a wrapping arrow stops, WBAR is the bar that
   draws the edge of the map it went through. */
/* CH carries one line more than the creature field needs: the settings line
   (speed and pace) sits between the field and the note, and neither of those two
   may be pushed into the other. */
var CW=200, CH=166, GX=88, GY=80, MG=96, WOUT=40, WBAR=58;
/* The creature field inside a cell: BCOLS by BROWS glyphs on a 12 by 13 pitch,
   laid out in one group translated to the field's top-left corner. BCAP is what
   caps the drawing, never the truth: past it one glyph stands for several and
   the cell says so. */
var BCOLS=14, BROWS=5, BCAP=BCOLS*BROWS, BPX=12, BPY=13;
var mapSig = "", anim = [], reduced = false, prevMig = {}, rafId = 0;
/* The hop animation's state (§17, B14). hopLayer sits ABOVE the cells;
   everything else is bounded on purpose and the bounds are named at HOPMS. */
var hopLayer = null, hopSeen = {}, hopSeenQ = [], hopQ = [], hopsLive = [],
    nextHopAt = 0, hopsPrimed = false, hopFeedOK = false;

/* LSEP is how far a DIRECTED lane sits off its axis centreline.
   Two-way lanes (D17) put two arrows in every gutter, and the choice made here
   is TWO SEPARATE DIRECTED ARROWS ON PARALLEL RAILS rather than one line with a
   head at each end. On the 3x2 rig that is 12 lane pairs = 24 directions, and a
   double-headed line cannot carry any of the three things a direction owns:
     - its own rate label (the two directions of a pair carry different flows),
     - its own state (skip lists are PER WALK — east and west cross one row from
       opposite ends and legitimately bypass different slots, so one direction
       can be a bypass while the other is direct),
     - its own travelling hop glyphs.
   The forward lane of an axis (E, N) takes one rail and the reverse (W, S) the
   other, and a bypass arc bows to the same side its rail sits on, so the two
   arcs of a pair are concentric and never cross. */
var LSEP = 13;
function cellX(c){ return MG + c*(CW+GX); }
function cellY(r,h){ return MG + (h-1-r)*(CH+GY); }
/* laneY/laneX are the centreline; laneOff pushes one direction off it. */
function laneY(r,h){ return cellY(r,h) + CH*0.58; }
function laneX(c){ return cellX(c) + CW*0.5; }
/* A horizontal bypass bows UP (-y) and a vertical one bows RIGHT (+x), which is
   what the one-way map already did. The FORWARD lane of each axis takes the rail
   on the bow's side and the taller bow with it; the reverse lane takes the other
   rail and the shorter bow, so the two arcs of a pair nest instead of crossing. */
function laneOff(edge){
  var bowSide = (edge === "E" || edge === "W") ? -LSEP : LSEP;
  return (edge === "E" || edge === "N") ? bowSide : -bowSide;
}
function laneBow(edge){ return (edge === "E" || edge === "N") ? 2*LSEP : 0; }

function signature(d){
  var s = d.map.width+"x"+d.map.height+"|"+(d.haveStatus?1:0)+"|";
  for (var i=0;i<d.slots.length;i++){
    var v = d.slots[i];
    s += v.slot+","+v.position.col+","+v.position.row+","+(v.live?1:0)+","
       + (v.modConnected?1:0)+","+(v.statsKnown?1:0)+","+esc(v.peerId)+";";
  }
  s += "|";
  for (var j=0;j<d.lanes.length;j++){
    var l = d.lanes[j];
    s += l.fromSlot+l.edge+l.toSlot+(l.open?1:0)+(l.skipped?l.skipped.length:0)+l.reason+";";
  }
  return s;
}

/* One lane's geometry. Everything the drawing needs is derived here, from the
   walk the server already did: how many positions were skipped, and whether the
   path crosses the map edge. */
/* Four edges, two axes, two directions per axis. The reverse pair is the
   forward pair with the step negated (contract-b-m4.md §17, B13), and the
   drawing mirrors that exactly: same wrap handling, same bypass bow, same
   arrowhead, read backwards along the axis. */
function laneGeom(l, d, byslot){
  var from = byslot[l.fromSlot];
  if (!from) return null;
  var w = d.map.width, h = d.map.height;
  var e = l.edge;
  var vert = (e === "N" || e === "S");
  var back = (e === "W" || e === "S");
  var off = laneOff(e), bow = laneBow(e);
  var steps = (l.skipped ? l.skipped.length : 0) + 1;
  var c = from.position.col, r = from.position.row;
  var segs = [];
  if (!l.open){
    // A closed lane is a stub with no destination: it goes nowhere, and it says
    // so rather than pointing at a world it cannot reach. It leaves by its own
    // side of the cell, so a closed W stub and an open E lane are never confused.
    var sx, sy, ex, ey;
    if (vert){
      sx = laneX(c) + off; ex = sx;
      sy = back ? cellY(r,h) + CH : cellY(r,h);
      ey = back ? sy + 46 : sy - 46;
    } else {
      sy = laneY(r,h) + off; ey = sy;
      sx = back ? cellX(c) : cellX(c) + CW;
      ex = back ? sx - 46 : sx + 46;
    }
    return {state:"closed", segs:[{d:"M "+sx+" "+sy+" L "+ex+" "+ey, head:false}],
            stopAt:{x:ex,y:ey}, wrap:false, steps:steps, vertical:vert, back:back,
            edgeAt:null};
  }
  var to = byslot[l.toSlot];
  if (!to) return null;
  var state = steps > 1 ? "bypass" : "open";
  var tc = to.position.col, tr = to.position.row;
  if (vert){
    var x = laneX(c) + off;
    var topY = cellY(h-1,h), botY = cellY(0,h) + CH;
    if (!back){
      // NORTH: out of the top of the cell, up the column, in at the bottom of
      // the target.
      if ((r + steps) < h){
        segs.push(arcSeg(x, cellY(r,h), x, cellY(tr,h)+CH, steps-1, true, bow));
        return {state:state, segs:segs, wrap:false, steps:steps, vertical:true,
                back:back, edgeAt:null};
      }
      // The torus: it leaves through the top of the map and comes back in at
      // the bottom. Two segments, each with its own arrowhead, and a bar on the
      // edge it passed through — so the reader can see it did not stop.
      segs.push(arcSeg(x, cellY(r,h), x, topY - WOUT, (h-1-r), true, bow));
      segs.push(arcSeg(x, botY + WOUT, x, cellY(tr,h)+CH, tr, true, bow));
      return {state:state, segs:segs, wrap:true, steps:steps, vertical:true, back:back,
              edgeAt:[{x:x, y:topY - WBAR, horiz:true, lbl:"to slot "+l.toSlot, dy:-12},
                      {x:x, y:botY + WBAR, horiz:true, lbl:"from slot "+l.fromSlot, dy:16}]};
    }
    // SOUTH: the same walk with the step negated. Out of the BOTTOM, down the
    // column, in at the TOP of the target, and it wraps through the bottom edge.
    if ((r - steps) >= 0){
      segs.push(arcSeg(x, cellY(r,h)+CH, x, cellY(tr,h), steps-1, true, bow));
      return {state:state, segs:segs, wrap:false, steps:steps, vertical:true,
              back:back, edgeAt:null};
    }
    segs.push(arcSeg(x, cellY(r,h)+CH, x, botY + WOUT, r, true, bow));
    segs.push(arcSeg(x, topY - WOUT, x, cellY(tr,h), (h-1-tr), true, bow));
    // The reverse direction's wrap labels are pushed one line further out. Both
    // directions of a column cross the SAME map edge, and a 13-unit rail offset
    // is not enough to keep two "to slot N" strings off each other.
    return {state:state, segs:segs, wrap:true, steps:steps, vertical:true, back:back,
            edgeAt:[{x:x, y:botY + WBAR, horiz:true, lbl:"to slot "+l.toSlot, dy:30},
                    {x:x, y:topY - WBAR, horiz:true, lbl:"from slot "+l.fromSlot, dy:-26}]};
  }
  var y = laneY(r,h) + off;
  var rightX = cellX(w-1) + CW, leftX = cellX(0);
  if (!back){
    // EAST.
    if ((c + steps) < w){
      segs.push(arcSeg(cellX(c)+CW, y, cellX(tc), y, steps-1, false, bow));
      return {state:state, segs:segs, wrap:false, steps:steps, vertical:false,
              back:back, edgeAt:null};
    }
    segs.push(arcSeg(cellX(c)+CW, y, rightX + WOUT, y, (w-1-c), false, bow));
    segs.push(arcSeg(leftX - WOUT, y, cellX(tc), y, tc, false, bow));
    // Below the line: the rate label sits above it, and the two must not collide.
    return {state:state, segs:segs, wrap:true, steps:steps, vertical:false, back:back,
            edgeAt:[{x:rightX + WBAR, y:y, horiz:false, lbl:"to slot "+l.toSlot, dy:26},
                    {x:leftX - WBAR, y:y, horiz:false, lbl:"from slot "+l.fromSlot, dy:26}]};
  }
  // WEST: out of the LEFT of the cell, along the row, in at the RIGHT of the
  // target, wrapping through the left edge of the map.
  if ((c - steps) >= 0){
    segs.push(arcSeg(cellX(c), y, cellX(tc)+CW, y, steps-1, false, bow));
    return {state:state, segs:segs, wrap:false, steps:steps, vertical:false,
            back:back, edgeAt:null};
  }
  segs.push(arcSeg(cellX(c), y, leftX - WOUT, y, c, false, bow));
  segs.push(arcSeg(rightX + WOUT, y, cellX(tc)+CW, y, (w-1-tc), false, bow));
  return {state:state, segs:segs, wrap:true, steps:steps, vertical:false, back:back,
          edgeAt:[{x:leftX - WBAR, y:y, horiz:false, lbl:"to slot "+l.toSlot, dy:26},
                  {x:rightX + WBAR, y:y, horiz:false, lbl:"from slot "+l.fromSlot, dy:26}]};
}

/* arcSeg draws one segment of a lane. It BOWS when the segment has to pass over
   cells, which is exactly the bypass — the single thing a table cannot show. A
   segment that passes over nothing is a straight arrow. extra is the rail's own
   bow allowance, which is what keeps the two arcs of a lane pair nested. */
function arcSeg(x1,y1,x2,y2,over,vertical,extra){
  if (over < 1) return {d:"M "+x1+" "+y1+" L "+x2+" "+y2, head:true};
  var lift = (extra||0) + 30 + 14*(over-1);
  if (vertical){
    var bx = x1 + (CW*0.5 + lift);
    var cx = x1 + (bx - x1)/0.75;
    return {d:"M "+x1+" "+y1+" C "+cx+" "+y1+", "+cx+" "+y2+", "+x2+" "+y2, head:true};
  }
  var by = y1 - (CH*0.58 + lift);
  var cy = y1 + (by - y1)/0.75;
  return {d:"M "+x1+" "+y1+" C "+x1+" "+cy+", "+x2+" "+cy+", "+x2+" "+y2, head:true};
}

function laneKey(l){ return l.fromSlot+l.edge; }

function buildMap(d){
  if (!d.haveStatus || !d.map || d.map.width < 1 || d.map.height < 1){
    var emptyTitle = d.relayConnected
      ? "The map is online and waiting for worlds."
      : "The archive is online and waiting for the relay.";
    var emptyBody = d.relayConnected
      ? "No participant has claimed a visible seat yet. Worlds will appear here when they connect; this is an empty beginning, not a broken map."
      : "The live map is temporarily unavailable. Participant worlds can continue running while this read-only archive reconnects.";
    $("#mapbox").innerHTML = '<div class="mapempty" role="status">'
      + '<div><svg class="emptybib" viewBox="-9 -7 19 14" aria-hidden="true">'
      + '<path fill="currentColor" d="M4.5-1.26 2.16 0l2.34 1.26C4.14 3.06 1.98 4.14-.54 4.14-3.24 4.14-5.58 2.16-6.66 0-5.58-2.16-3.24-4.14-.54-4.14c2.52 0 4.68 1.08 5.04 2.88Z"/>'
      + '<circle cx="1.5" cy="-1.85" r="1" fill="var(--cell)"/></svg>'
      + '<b>'+emptyTitle+'</b>'
      + '<p>'+emptyBody+'</p>'
      + '<a href="/">Learn how the Multiverse works</a></div></div>';
    mapSig = signature(d);
    return;
  }
  var w = Math.max(d.map.width,1), h = Math.max(d.map.height,1);
  var W = 2*MG + w*CW + (w-1)*GX, H = 2*MG + h*CH + (h-1)*GY;
  var byslot = {}, at = {};
  for (var i=0;i<d.slots.length;i++){
    byslot[d.slots[i].slot] = d.slots[i];
    at[d.slots[i].position.col+","+d.slots[i].position.row] = d.slots[i];
  }

  var s = '<svg id="map" viewBox="0 0 '+W+' '+H+'" preserveAspectRatio="xMidYMid meet" role="img" '
    + 'aria-label="the multiverse map: '+w+' by '+h+' grid of worlds and the lanes between them">'
    + '<defs>'
    + marker("mOpen","var(--lane)") + marker("mBypass","var(--warn)")
    + marker("mClosed","var(--dark)")
    // The creature itself is NOT defined here. It is defined once for the whole
    // document, in the zero-sized SVG at the top of the body, and drawn by
    // reference up to 420 times here and again on the species tab. It moved out
    // of this string when the second tab needed it: this SVG is thrown away and
    // rebuilt whenever the map's shape changes, and a definition that vanishes
    // with it is a definition the other tabs cannot rely on.
    + '</defs>';

  // Which way is north, before a reader has to work it out from the arrows.
  s += '<text class="axis" x="8" y="15">north &uarr; · east &rarr;</text>';

  // Lanes first, cells over them: an arrow must never cover a number.
  var vertical = {}, lstate = {}, lback = {};
  s += '<g id="lanelayer">';
  for (var j=0;j<d.lanes.length;j++){
    var l = d.lanes[j], g = laneGeom(l, d, byslot);
    if (!g) continue;
    var key = laneKey(l), cls = g.state;
    var head = "mOpen";
    if (cls === "bypass") head = "mBypass";
    if (cls === "closed") head = "mClosed";
    var title = laneTitle(l, g);
    s += '<g class="lg" id="lg-'+esc(key)+'">';
    for (var k=0;k<g.segs.length;k++){
      var seg = g.segs[k];
      s += '<path class="laneunder" d="'+seg.d+'"/>';
    }
    for (k=0;k<g.segs.length;k++){
      seg = g.segs[k];
      s += '<path class="lane '+cls+'" id="lp-'+esc(key)+'-'+k+'" d="'+seg.d+'"'
         + (seg.head ? ' marker-end="url(#'+head+')"' : '') + '/>';
    }
    if (g.state === "closed"){
      s += '<g class="closedx" transform="translate('+g.stopAt.x+','+g.stopAt.y+')">'
         + '<path class="lane closed" style="stroke-dasharray:none" d="M -6 -6 L 6 6"/>'
         + '<path class="lane closed" style="stroke-dasharray:none" d="M 6 -6 L -6 6"/></g>';
    }
    if (g.edgeAt){
      for (k=0;k<g.edgeAt.length;k++){
        var e = g.edgeAt[k];
        s += e.horiz
          ? '<path class="edgebar" d="M '+(e.x-16)+' '+e.y+' L '+(e.x+16)+' '+e.y+'"/>'
          : '<path class="edgebar" d="M '+e.x+' '+(e.y-16)+' L '+e.x+' '+(e.y+16)+'"/>';
        s += '<text class="wraplbl term" data-t="wrap" text-anchor="middle" x="'+e.x
          + '" y="'+(e.y + e.dy)+'">'+esc(e.lbl)+'</text>';
      }
    }
    // Each direction labels itself on its OWN side of the pair, or the two rate
    // labels of a lane pair land on top of each other.
    s += '<text class="lanelbl'+(cls==="bypass"?" bypasslbl":(cls==="closed"?" closedlbl":""))
       + '" id="ll-'+esc(key)+'" x="0" y="0" text-anchor="'
       + (g.vertical ? (g.back?"end":"start") : "middle")+'"></text>';
    vertical[key] = !!g.vertical;
    lback[key] = !!g.back;
    lstate[key] = g.state;
    for (k=0;k<g.segs.length;k++){
      s += '<path class="lanehit" d="'+g.segs[k].d+'"><title>'+esc(title)+'</title></path>';
    }
    s += '</g>';
  }
  // Cells over the lanes, hops over the cells. A hop is the event the page
  // exists to show and must never be hidden by the cell it is arriving at.
  s += '</g><g id="celllayer">';

  for (var row = h-1; row >= 0; row--){
    for (var col = 0; col < w; col++){
      var v = at[col+","+row], x = cellX(col), y = cellY(row,h);
      if (!v){
        s += '<g class="cell hole"><rect class="cellbg" x="'+x+'" y="'+y+'" width="'+CW
          + '" height="'+CH+'" rx="10"/>'
          + '<text class="slotno term" data-t="hole" style="fill:var(--hole)" x="'+(x+16)+'" y="'+(y+30)+'">hole</text>'
          + '<text class="pos" x="'+(x+16)+'" y="'+(y+48)+'">('+col+','+row+')</text>'
          + '<text class="note" x="'+(x+16)+'" y="'+(y+CH-14)+'">no world claims this seat;</text>'
          + '<text class="note" x="'+(x+16)+'" y="'+(y+CH-2)+'">both axes step over it</text></g>';
        continue;
      }
      var state = v.live ? "live" : "dark";
      var lbl = v.live ? (v.modConnected ? "live" : "no game") : "dark";
      // The <title> is EMPTY here and filled by paintMap with textContent. It
      // carries species names, which are attacker-choosable text (§13 item 7),
      // and this string becomes innerHTML — so no name may ever reach it. It is
      // also the phone path: a tap on a cell shows exactly this.
      s += '<g class="cell '+state+'" id="c-'+v.slot+'">'
        + '<rect class="cellbg" x="'+x+'" y="'+y+'" width="'+CW+'" height="'+CH+'" rx="10">'
        + '<title id="ctitle-'+v.slot+'"></title></rect>'
        + '<rect class="cellhit" x="'+(x+2)+'" y="'+(y+2)+'" width="'+(CW-4)+'" height="'+(CH-4)
        + '" rx="9"/>'
        + '<text class="slotno term" data-t="slot" x="'+(x+16)+'" y="'+(y+30)+'">slot '+v.slot+'</text>'
        + '<text class="pos" x="'+(x+16)+'" y="'+(y+48)+'">('+v.position.col+','+v.position.row
        + ') <tspan class="term" data-t="peer">'+esc(v.peerId)+'</tspan></text>'
        + '<text class="popnum" id="cpop-'+v.slot+'" text-anchor="end" x="'+(x+CW-16)+'" y="'
        + (y+32)+'"></text>'
        + '<circle class="statedot" cx="'+(x+CW-16-7*lbl.length-9)+'" cy="'+(y+44)+'" r="3.4"/>'
        + '<text class="statelbl" text-anchor="end" x="'+(x+CW-16)+'" y="'+(y+48)+'">'+lbl+'</text>'
        // The creature field is empty markup translated into place; every glyph
        // in it is built by paintSpecies with createElementNS, for the same
        // reason the title above is empty.
        + '<g class="bibs" id="cspec-'+v.slot+'" transform="translate('+(x+16)+','+(y+60)+')"></g>'
        // The settings line: how fast this world runs, and the cap on how fast
        // organisms are let into it. Empty markup filled by paintChips, which
        // builds tspans rather than concatenating a string, for the same reason
        // the title above is empty — one code path for the DOM, and the safe one.
        + '<text class="chips" id="cchip-'+v.slot+'" x="'+(x+16)+'" y="'+(y+CH-27)+'"></text>'
        + '<text class="note" id="cnote-'+v.slot+'" x="'+(x+16)+'" y="'+(y+CH-11)+'"></text>'
        + '</g>';
    }
  }
  s += '</g><g id="hops"></g></svg>';
  $("#mapbox").innerHTML = s;

  // The labels sit on the paths, which only exist once the SVG is in the DOM.
  anim = [];
  hopLayer = document.getElementById("hops");
  // Every live hop animation refers to a path that has just been thrown away.
  killHops();
  for (j=0;j<d.lanes.length;j++){
    l = d.lanes[j];
    var kk = laneKey(l), tracks = [], total = 0;
    for (k=0;;k++){
      var p = document.getElementById("lp-"+kk+"-"+k);
      if (!p) break;
      var len = p.getTotalLength();
      tracks.push({el:p, len:len});
      total += len;
    }
    if (!tracks.length) continue;
    var lab = document.getElementById("ll-"+kk);
    if (lab){
      var best = tracks[0];
      for (k=1;k<tracks.length;k++) if (tracks[k].len > best.len) best = tracks[k];
      // A bypass arc peaks exactly where the crossing lane's own label sits, so
      // it is labelled a third of the way along instead of at the middle.
      var frac = (lstate[kk] === "bypass" && !vertical[kk]) ? 0.33 : 0.5;
      var mp = best.el.getPointAtLength(best.len*frac);
      // Perpendicular to the lane, and on this DIRECTION's own side of the pair,
      // or the label lands on the arrow it describes or on its opposite number.
      var bk = lback[kk];
      lab.setAttribute("x", mp.x + (vertical[kk] ? (bk ? -18 : 18) : 0));
      lab.setAttribute("y", mp.y + (vertical[kk] ? 4 : (bk ? 16 : -11)));
    }
    if (l.open) anim.push({key:kk, tracks:tracks, total:total});
  }
  mapSig = signature(d);
}

/* The lane anim record for one directed lane, or null. Lanes are keyed
   fromSlot+edge everywhere on this surface, which is what makes the two
   directions of a pair two separate things to animate. */
function animFor(key){
  for (var i=0;i<anim.length;i++) if (anim[i].key === key) return anim[i];
  return null;
}

function marker(id,color){
  return '<marker id="'+id+'" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6.5" '
    + 'markerHeight="6.5" orient="auto-start-reverse">'
    + '<path d="M 0 0 L 10 5 L 0 10 z" fill="'+color+'"/></marker>';
}

/* ---------------------------------------------- the two settings a cell shows
   SPEED is the world's own time scale, and it is TWO numbers because it has
   always been two questions. The APPLIED scale is straight out of the game's
   heartbeat — ×5 is the game saying it is running five simulated seconds per
   real second, ×0 is a world standing still. The ACHIEVED rate is what its
   clock actually produced, measured here by the archive over the last minute.
   They agree on a host that is keeping up and come apart completely on one that
   is not: every world on this rig reports ×100 and several deliver ×5, because
   a frame can only advance one simulation tick however high the number is set.
   Written "×100 → ~×12": asked of it, then delivered.

   PACE is the arrival rate limit: how many are queued, and the cap they are
   queued behind, per simulated minute of THIS world.

   All three obey §10.1's unknown rule without exception. An older helper
   program publishes no cap, and the cap it would have had is NOT the shipped
   default — that default has moved three times — so it renders "?" and says so
   in the warning colour. A confident 100 there would be a lie about a world
   running at 2.0, which is exactly the mistake this rig has already made once
   in a test. The achieved rate is the same rule in its quieter form: until the
   archive has watched a world for long enough to measure it, the cell shows the
   applied value ALONE rather than a first guess that would move under the
   reader. */
function fmtScale(n){
  if (n == null) return "?";
  var r = Math.round(n*10)/10;
  return Math.abs(r - Math.round(r)) < 0.05 ? String(Math.round(r)) : r.toFixed(1);
}
function speedText(v){
  return (v.statsKnown && v.timeScale != null) ? "×"+fmtScale(v.timeScale) : "×?";
}
/* The measured half, or null — which every caller renders as "nothing here",
   never as a "?" beside the applied value. An unknown cap is a peer refusing to
   say; an unmeasured rate is this archive not having looked long enough yet,
   and dressing the second up as the first would cry wolf on every restart. */
function achievedText(v){
  return (v.statsKnown && v.achievedTimeScale != null)
    ? "~×"+fmtScale(v.achievedTimeScale) : null;
}
function paceRateText(v){
  return (v.statsKnown && v.inboundRatePerSimMinute != null)
    ? fmtScale(v.inboundRatePerSimMinute) : "?";
}
function paceDepthText(v){
  return (v.statsKnown && v.pacedDepth != null) ? String(v.pacedDepth) : "?";
}

/* The cell's settings line, built as nodes so the unknown halves can carry
   their own colour and each half its own tooltip. The unit is dropped when the
   numbers are long enough to reach the world next door; the tooltip, the
   glossary and the worlds table all still carry it. */
function paintChips(v){
  var host = document.getElementById("cchip-"+v.slot);
  if (!host) return;
  while (host.firstChild) host.removeChild(host.firstChild);
  function part(cls, s, term){
    var el = document.createElementNS(SVGNS, "tspan");
    if (cls) el.setAttribute("class", cls);
    if (term) el.setAttribute("data-t", term);
    el.textContent = s;
    host.appendChild(el);
  }
  var sp = speedText(v), ac = achievedText(v), d = paceDepthText(v), r = paceRateText(v);
  part((sp === "×?" ? "chipu" : "chipv") + " term", sp, "speed");
  // The arrow is drawn in the line's own dim colour and the two scales in the
  // value colour, so what reads as a pair is the pair: asked for, then got.
  if (ac){
    part(null, " → ", null);
    part("chipv term", ac, "achieved");
  }
  part(null, " · ", null);
  part("term", "pace ", "pace");
  part(d === "?" ? "chipu" : "chipv", d, null);
  part(null, "/", null);
  part(r === "?" ? "chipu" : "chipv", r, null);
  // 19 + the 8-character unit is 27, which is what fits between the cell's left
  // inset and its right edge at this font. Past that the unit goes and the two
  // numbers stay: a reader who has lost the unit can find it in the tooltip, the
  // glossary and the worlds table, but a number clipped by the world next door
  // is unreadable everywhere. The measured rate spends that budget too — it is
  // why a cell showing both scales drops the unit — and it is worth it: the unit
  // is written in three other places and the gap between the two scales is
  // written nowhere else on this map.
  if ((sp + (ac ? " → " + ac : "") + " · pace " + d + "/" + r).length <= 19) {
    part(null, "/sim-min", null);
  }
}

function cellTitle(v){
  var s = "slot "+v.slot+" ("+v.position.col+","+v.position.row+")  peer "+v.peerId+"\n"
    + (v.live ? (v.modConnected ? "live" : "connected, but no game attached") : "dark");
  if (!v.live && v.darkForMs != null) s += " for "+ms(v.darkForMs);
  s += "\npopulation " + (v.statsKnown && v.population!=null ? v.population : "unknown");
  if (v.statsKnown){
    var acT = achievedText(v);
    s += "\nspeed: the game reports " + speedText(v) + " real time"
      + (acT ? ", and is measured delivering " + acT + " over the last "
               + ms(v.achievedSpanMs)
             : "; the delivered rate is not measured yet")
      + "\narrivals: " + paceDepthText(v) + " queued, cap " + paceRateText(v)
      + " per simulated minute";
    s += "\ncustody "+(v.custodyDepth==null?"unknown":v.custodyDepth)
      +", held "+(v.heldDepth==null?"unknown":v.heldDepth);
    if (v.lastSave) s += "\nlast save "+ms(v.lastSaveAgeMs)+" ago";
    s += "\n" + censusLines(v);
  } else {
    s += "\nthis world has reported nothing recently"
      + (v.statsAgeMs ? " (last heard "+ms(v.statsAgeMs)+" ago)" : "");
  }
  return s;
}

var EDGEWORD = {E:"east", N:"north", W:"west", S:"south"};

function laneTitle(l, g){
  var s = "slot "+l.fromSlot+" "+(EDGEWORD[l.edge]||l.edge)+" lane\n";
  if (!l.open) return s + "closed: "+l.reason + "\nnothing on this axis can be delivered to.";
  s += "carries organisms to slot "+l.toSlot;
  if (g.wrap) s += ", around the edge of the map";
  if (g.steps > 1){
    s += "\nBYPASS: it reaches over "+(g.steps-1)+" position(s) that cannot take an organism";
    var parts = [];
    for (var i=0;i<(l.skipped||[]).length;i++){
      var k = l.skipped[i];
      parts.push(k.slot==null ? "hole ("+k.position.col+","+k.position.row+")"
                              : "slot "+k.slot+": "+k.reason);
    }
    if (parts.length) s += "\nskipping "+parts.join(", ");
  }
  s += "\n"+l.migrations+" migration(s) recorded, "+l.perMinute.toFixed(2)+"/min recently";
  return s;
}

/* Everything between the two SPECIES CENSUS markers below handles ATTACKER-
   CHOOSABLE TEXT, and it is fenced off so that fact cannot be forgotten.

   A census name is up to 64 bytes a peer chose, 32 entries a peer, arriving on
   an unauthenticated stats block the relay copies verbatim into a broadcast
   every client reads (contract-b-m4.md §13 item 7). It is NOT sanitized
   upstream and it never will be: contract-a.md §17 A36 guarantees the opposite,
   because a repaired label names a species the player cannot find in their own
   game. The wire's answer is a byte count, a UTF-8 decode and a cap.

   THREE FIELDS ARRIVE THIS WAY AND THE THIRD IS EASY TO FORGET. Beside the
   census names and the exclusion entries, an ANCESTOR'S NAME on the family tree
   is a "parentGenericName" off a migration envelope (contract-a.md §16 A30) —
   the same 64 attacker-chosen bytes, arriving by a different route, and the one
   name on this page that no census ever vouches for. It is drawn inside this
   fence for that reason and by the same means.

   THE RENDERER'S ANSWER IS ITS OWN, and it is this:

     NO NAME EVER BECOMES MARKUP. Names reach the DOM through textContent and
     through createElementNS + setAttribute, and through nothing else. The
     fenced region assigns no innerHTML at all — the page test asserts that the
     token does not occur between the markers, which is the only form of this
     rule a Go test can enforce — and no name is ever concatenated into one of
     the HTML strings the rest of this file builds.

   The other rule this region keeps is contract-a.md §17 A36's: NOTHING HERE
   REPAIRS A NAME. What is drawn and what is printed is the raw spelling the
   owning world holds. Whitespace is normalized in exactly one place, normName,
   FOR COMPARISON ONLY — colour and cross-world presence — and the result is
   never displayed and never written back. */

/* =========================== SPECIES CENSUS — BEGIN ======================= */

/* The world's own display name for a species: the two raw halves with one space
   between them, which is the game's own Species.name. */
function censusName(e){ return String(e.genericName) + " " + String(e.specificName); }

/* normName is A34's repair — trim, collapse internal runs — applied FOR
   COMPARISON ONLY. It answers two questions and no others: what colour is this
   species, and which other worlds report it. It is never shown to anybody and
   nothing is ever rewritten from it, because the same Species record legitimately
   reads "Izus" on a migration and "Izus " on a census, and two spellings a world
   keeps apart are two records in that world. */
function normName(s){ return String(s).replace(/\s+/g, " ").replace(/^ | $/g, ""); }

/* A stable colour per species. FNV-1a over the compared name, then a hue, so a
   species is the same colour in every world and across every reload, with no
   table to keep and nothing to assign. Lightness stays high enough to read on
   the dark background at every hue. */
function hashStr(s){
  var h = 2166136261 >>> 0;
  for (var i=0;i<s.length;i++){ h ^= s.charCodeAt(i); h = Math.imul(h, 16777619) >>> 0; }
  return h >>> 0;
}
function speciesColor(name){
  var h = hashStr(name);
  return "hsl(" + (h % 360) + "," + (58 + (h >>> 9) % 22) + "%," + (56 + (h >>> 17) % 13) + "%)";
}

/* XW is the LIVE cross-world index: compared name -> the slots reporting it,
   rebuilt from every poll so "also in slot 5" is as fresh as the map. The join
   is done HERE, in the client, from the censuses /api/status already carries —
   the server publishes each world's census exactly as its stats block held it
   and derives nothing, so there is one statement of the census and the page is
   the only thing that has an opinion about how to relate six of them. */
var XW = {};
function buildCrossWorld(d){
  XW = {};
  for (var i=0;i<d.slots.length;i++){
    var v = d.slots[i];
    if (!v.statsKnown || !v.speciesKnown || !v.species) continue;
    for (var j=0;j<v.species.length;j++){
      var k = normName(censusName(v.species[j]));
      if (!XW[k]) XW[k] = [];
      if (XW[k].indexOf(v.slot) < 0) XW[k].push(v.slot);
    }
  }
}

/* censusRuns turns one slot into the contiguous runs the cell draws, IN THE
   CENSUS'S OWN ORDER — which is the mod's, sorted by bibites + eggs descending.
   Nothing here re-sorts, merges or de-duplicates: two entries whose names differ
   only in whitespace are two Species records in that world and stay two runs.

   The last run is the honest slack contract-a.md §17 A35 defines: Sum bibites is
   allowed to fall short of population, and the shortfall is organisms with no
   Species record. It is drawn in grey rather than folded into a species, because
   folding it in would invent an abundance the world never reported. */
function censusRuns(v){
  var runs = [], sumB = 0, sumE = 0;
  var known = !!(v.statsKnown && v.speciesKnown);
  var list = (known && v.species) ? v.species : [];
  for (var i=0;i<list.length;i++){
    var e = list[i], nm = censusName(e), cmp = normName(nm);
    sumB += e.bibites; sumE += e.eggs;
    runs.push({key:"sp"+v.slot+"-"+i, name:nm, cmp:cmp, color:speciesColor(cmp),
               bibites:e.bibites, eggs:e.eggs});
  }
  var pop = (v.statsKnown && v.population != null) ? v.population : null;
  var eggs = (v.statsKnown && v.eggCount != null) ? v.eggCount : null;
  var unB = (known && pop != null && pop > sumB) ? pop - sumB : (known ? 0 : (pop || 0));
  var unE = (known && eggs != null && eggs > sumE) ? eggs - sumE : (known ? 0 : (eggs || 0));
  return {known:known, runs:runs, sumB:sumB, sumE:sumE, unB:unB, unE:unE, pop:pop, eggCount:eggs};
}

/* censusLines is the census as text, for the cell's <title> — the phone path,
   where there is no hover and a tap has to say everything at once. It is set
   with textContent by its caller and is never markup. */
function censusLines(v){
  if (!v.speciesKnown) return "species: unknown (this world reports no census)";
  if (!v.species || !v.species.length) return "species: none — this world reports nothing alive";
  var c = censusRuns(v), out = [];
  out.push("species (" + c.runs.length + (v.speciesTruncated ? ", the 32 most numerous; the rest is UNREPORTED" : "") + "):");
  for (var i=0;i<c.runs.length;i++){
    var r = c.runs[i];
    out.push("  " + r.name + " — " + r.bibites + " alive" + (r.eggs ? ", " + r.eggs + " egg(s)" : ""));
  }
  if (c.unB > 0) out.push("  (" + c.unB + " with no species record)");
  return out.join("\n");
}

/* One species run's tooltip, registered under its data-s key. Both halves of it
   are text: SP holds strings and showTip sets them with textContent. */
function speciesTip(v, r, total){
  var body = r.bibites + " alive" + (r.eggs ? " and " + r.eggs + " egg(s)" : " here")
    + " in slot " + v.slot;
  if (v.population != null && v.population > 0){
    body += "\n" + Math.round(1000 * r.bibites / v.population) / 10
      + "% of this world's population";
  } else if (total > 0){
    body += "\n" + Math.round(1000 * (r.bibites + r.eggs) / total) / 10 + "% of its census";
  }
  var where = XW[r.cmp] || [], others = [];
  for (var i=0;i<where.length;i++) if (where[i] !== v.slot) others.push("slot " + where[i]);
  body += "\n" + (others.length ? "also in " + others.join(", ")
                                : "no other world reports it right now");
  body += "\nname as this world spells it — nothing here tidies it";
  return {title:r.name, body:body};
}

/* paintSpecies draws one cell's creature field. Every element is CREATED, never
   parsed: no name reaches a markup string, and the only thing that carries a
   name is a textContent or a tooltip registry entry keyed by a generated id. */
function paintSpecies(v){
  var host = document.getElementById("cspec-"+v.slot);
  if (!host) return null;
  while (host.firstChild) host.removeChild(host.firstChild);
  var c = censusRuns(v);
  var total = c.sumB + c.sumE + c.unB + c.unE;
  if (!v.statsKnown || total <= 0) return c;

  // One glyph is one creature until there are more creatures than glyphs; past
  // that, one glyph stands for several and the cell's note says how many.
  var unit = Math.max(1, Math.ceil(total / BCAP)), left = BCAP, idx = 0;
  function budget(n){
    if (n <= 0) return 0;
    var g = Math.max(1, Math.round(n / unit));
    if (g > left) g = left;
    left -= g;
    return g;
  }
  function place(parent, kind, colour, cls){
    var col = idx % BCOLS, row = Math.floor(idx / BCOLS);
    idx++;
    var cx = 6 + col*BPX, cy = 6 + row*BPY;
    var el;
    if (kind === "egg"){
      el = document.createElementNS(SVGNS, "circle");
      el.setAttribute("class", "egg" + cls);
      el.setAttribute("cx", cx); el.setAttribute("cy", cy); el.setAttribute("r", 3);
      if (colour) el.style.stroke = colour;
    } else {
      el = document.createElementNS(SVGNS, "use");
      el.setAttribute("href", "#bib");
      el.setAttribute("class", "bib" + cls);
      el.setAttribute("x", cx); el.setAttribute("y", cy);
      if (colour) el.style.fill = colour;
    }
    parent.appendChild(el);
  }
  function run(cls, colour, nBib, nEgg, key, tipObj){
    var gb = budget(nBib), ge = budget(nEgg);
    if (!gb && !ge) return;
    var g = document.createElementNS(SVGNS, "g");
    if (key){
      g.setAttribute("class", "bibrun");
      g.setAttribute("data-s", key);
      SP[key] = tipObj;
    }
    for (var i=0;i<gb;i++) place(g, "bib", colour, cls);
    for (var j=0;j<ge;j++) place(g, "egg", colour, cls);
    host.appendChild(g);
  }

  if (!c.known){
    // ABSENT CENSUS: unknown, never zero and never an empty world. The
    // creatures are drawn exactly as they were before there was a census, in
    // the neutral colour, and the cell's note says the species are unknown.
    run(" neutral", null, c.unB, c.unE, "sp"+v.slot+"-unk",
        {title:"species unknown", body:"This world reports a population but no species census — "
         + "an older game mod, or one that does not report species yet.\nUnknown is not zero: "
         + "these creatures are here, their kinds are not being told to us."});
    return c;
  }
  for (var i=0;i<c.runs.length && left>0;i++){
    var r = c.runs[i];
    run("", r.color, r.bibites, r.eggs, r.key, speciesTip(v, r, c.sumB + c.sumE));
  }
  if (left > 0 && (c.unB > 0 || c.unE > 0)){
    run(" unclassed", null, c.unB, c.unE, "sp"+v.slot+"-unc",
        {title:"no species record", body:c.unB + " creature(s)"
         + (c.unE ? " and " + c.unE + " egg(s)" : "") + " in slot " + v.slot
         + " that the world counted but filed under no species.\nThat is an ordinary state, "
         + "not a fault, and it is drawn grey rather than added to a species so the gap stays "
         + "visible."});
  }
  c.unit = unit;
  return c;
}

/* The species cell of the worlds table, filled the same way and for the same
   reason: a swatch element, then the name as a text node. */
function paintSpeciesCell(v){
  var cell = document.getElementById("wsp-"+v.slot);
  if (!cell) return;
  cell.textContent = "";
  function plain(cls, s){
    var el = document.createElement("span");
    if (cls) el.className = cls;
    el.textContent = s;
    cell.appendChild(el);
  }
  if (!v.statsKnown || !v.speciesKnown){ plain("unknown", "unknown"); return; }
  if (!v.species || !v.species.length){ plain("unknown", "none alive"); return; }
  var shown = Math.min(4, v.species.length);
  for (var i=0;i<shown;i++){
    var e = v.species[i], nm = censusName(e);
    var item = document.createElement("span");
    item.className = "spitem";
    var sw = document.createElement("i");
    sw.className = "sw";
    sw.style.background = speciesColor(normName(nm));
    item.appendChild(sw);
    item.appendChild(document.createTextNode(nm + " ×" + (e.bibites + e.eggs)));
    cell.appendChild(item);
  }
  var rest = v.species.length - shown;
  if (rest > 0) plain("spmore", "+" + rest + " more");
  if (v.speciesTruncated) plain("unknown", "· 32 shown, the rest unreported");
}
/* A HOP'S species name is the same untrusted text arriving on a different
   message, and it is handled inside this fence for exactly that reason
   (contract-b-m4.md §17, B14: "a census name and a ledger name are equally
   untrusted"). The two functions below are the only ones that touch it.

   hopName returns "" when the envelope carried NO species block, and "" is what
   the neutral glyph is drawn from. It is never the string "unknown": unknown is
   the absence of a name, not a name. */
function hopName(hp){
  var sp = hp && hp.species;
  if (!sp || !sp.genericName || !sp.specificName) return "";
  return censusName(sp);
}

/* buildHopGlyph creates the travelling creature. Every node is CREATED and the
   name lands in a <title> as textContent — nothing here is parsed as markup and
   nothing here assigns a colour from anything but a hash of the compared
   spelling. It is drawn larger than a resident glyph, stroked and shadowed,
   because it has to win the eye against a field of 420 of them. */
function buildHopGlyph(name, toSlot){
  var g = document.createElementNS(SVGNS, "g");
  g.setAttribute("class", "hopg");
  var ring = document.createElementNS(SVGNS, "circle");
  ring.setAttribute("class", "hopring");
  ring.setAttribute("r", 12);
  g.appendChild(ring);
  var u = document.createElementNS(SVGNS, "use");
  u.setAttribute("href", "#bib");
  u.setAttribute("class", name ? "hopbib" : "hopbib neutral");
  u.setAttribute("transform", "scale(2.2)");
  if (name) u.style.fill = speciesColor(normName(name));
  g.appendChild(u);
  var ttl = document.createElementNS(SVGNS, "title");
  ttl.textContent = (name ? name : "a creature of no reported species")
    + " — crossing to slot " + toSlot;
  g.appendChild(ttl);
  return g;
}

/* ------------------------------------------------------------ the SPECIES tab
   Everything below draws names — species names from six censuses, the names of
   their recorded ancestors, and, on the settings cards, the entries of six
   exclusion lists. An ancestor's label is not a census name: no world is
   reporting the species, so the only spelling the archive has is a
   parentGenericName off a migration envelope (contract-a.md §16 A30), which is
   the same 64 attacker-chosen bytes arriving by another route. An exclusion entry
   is the same class of text again (contract-b-m4.md §13 item 7, §19 B18). All
   three are handled inside this fence for the same reason and by the same means:
   NOTHING HERE ASSIGNS MARKUP. Every element is created, every string lands as a
   text node.

   The consequence is that these views build their whole DOM by hand rather than
   by templating a string, which is more code and is the point: the safe path is
   the only path, so one careless concatenation cannot appear later.

   ONE VIEW, NOT TWO. This tab was a flat census table and a separate family
   tree. They listed the same species from the same census in two orders, and a
   reader who wanted "how many of this one, and where did it come from" had to
   hold one tab in their head while reading the other. They are one drawing now:

     TIME RUNS LEFT TO RIGHT. A species is a BAR from the first crossing this
     archive recorded of it to the last, or to the right-hand edge while it is
     alive. That is deliberately NOT a lifespan — it is the span of the RECORD,
     and every label here says so.

     THE CENSUS FACTS SIT ON THE BAR. Population, the worlds holding it and how
     many in each, eggs, the three badges: the things the flat table said, said
     on the row whose shape says when and from whom.

     THE GLYPH IS THE ONLY COLOURED THING. One species is one colour, and it is
     worn by the creature and by nothing else — no swatch, no coloured chip, no
     tinted bar repeating what the glyph already said.

     THE LABELS AND THE BADGES ARE TWO LINES, not one. A name is 64 bytes
     somebody else chose, so it is clipped to its column; the badges are the
     page's own words and are drawn under it with room to be read. They shared
     the name's clip once and were cut off by it, which made a row with something
     to say look like a row with nothing.

     AND THE PLOT IS ELASTIC. Everything left of it is text and dots with the
     widths they have; the timeline takes what the box leaves, because the right
     edge of it is NOW and that is the one mark nobody should have to scroll to.

     ONE SET OF ROWS IS LEFT OUT BY DEFAULT: the seed stock, which is a species
     every world holding it refuses to export. It is marked by the server, it is
     counted on the tab, and the notice that counts it will put it back. */

var LFX = null, lfOpenKey = null, lfQuery = "", lfSort = "family";
/* THE ANSWER'S OWN NODES BY KEY, rebuilt with every paint. A link and a row's
   tooltip are both about a PAIR of species now — where the parent's record stops
   against where the child's starts — and neither can be drawn from the child
   alone. It indexes every node the answer carries and not only the drawn ones,
   because the server's reduction already rewrote a child's parent to the nearest
   DRAWN ancestor, so a lookup by n.parent is a lookup of a row. */
var LFBY = {};
/* The shared trend answer, keyed by species, and what it says about its own
   reach. Both are null until the first slow poll lands, and a row with no
   entry draws no line — never a flat one, which would be a claim. */
var LFTREND = null, LFTRENDMETA = null;

/* ---- WHAT THE LAST PAINT PUT ON THE SCREEN, so one lineage can be lit without
   repainting anything.

   LFEL and LFLN hold the drawn rows and the drawn parent-child links by species
   key, each with the class string it was built with — the BASE, which lighting
   adds to and unlighting restores. Keeping the base beside the element is what
   lets a row be lit and unlit without the code having to remember whether that
   row was also open, also an ancestor, also anything else.

   LFPAR and LFKID are the drawn tree itself: parent by key, children by key.
   They are rebuilt from the rows this paint actually produced, never from the
   answer, because a search draws a subset and the lineage a reader sees lit must
   be the lineage in front of them.

   lfFocusKey is the row the keyboard is on, carried across EVERY paint and not
   only the one that opening a row causes: this view repaints about every two
   seconds and each paint replaces the whole drawing, so the element holding the
   focus is destroyed under the reader's hands thirty times a minute. It is set
   wherever the focus ARRIVES on a row — Tab, a click, the key that opens one, or
   the paint putting it back — and cleared only when the focus genuinely leaves.

   lfPainting is how that difference is told. A row losing the focus mid-paint is
   this code taking its element away, not the reader walking off, and browsers
   disagree about whether that even reports itself — so the paint says so rather
   than the handler guessing.

   LFJOINED IS THE WHOLE AFFORDANCE'S GATE. In the population order this drawing
   draws no edges at all, because rows in abundance order are not in family order
   (lfRows). Lighting "the line" there would light one row and dim every other,
   which a reader would read as "this species is related to nothing" — a claim
   about the record made out of a sort order. So in that order nothing lights. */
var LFSVG = null, LFEL = {}, LFLN = {}, LFPAR = {}, LFKID = {}, LFJOINED = false,
    lfFocusKey = null, lfPainting = false;

/* One species' whole line: every ancestor above it up to its root, and its whole
   subtree below. Both walks are guarded — the server's own tree is acyclic and
   guarded (tree.go rule 4), and this walks what the PAGE built, which is one more
   place a loop must not become a hang. */
function lfKin(key){
  var on = {}, cur = key, guard = 0;
  while (cur && !on[cur] && guard++ < 600){ on[cur] = true; cur = LFPAR[cur]; }
  var stack = [key];
  while (stack.length && guard++ < 4000){
    var kids = LFKID[stack.pop()] || [];
    for (var i=0;i<kids.length;i++){
      if (!on[kids[i]]){ on[kids[i]] = true; stack.push(kids[i]); }
    }
  }
  return on;
}

/* Light one lineage and dim the rest. It is a class change on elements already on
   the screen: no request, no rebuild, nothing moves. THE DIMMING IS THE POINT —
   hiding the other rows would reflow the drawing under the reader's eye and
   answer a different question. */
function lfLight(key){
  if (!LFSVG || !LFJOINED || !LFEL[key]) return;
  var on = lfKin(key), k;
  LFSVG.setAttribute("class", "life lit");
  for (k in LFEL){
    LFEL[k].g.setAttribute("class", LFEL[k].base +
      (k === key ? " kin self" : (on[k] ? " kin" : "")));
  }
  for (k in LFLN){
    LFLN[k].g.setAttribute("class", LFLN[k].base + (on[k] ? " kin" : ""));
  }
}
function lfDark(){
  if (!LFSVG) return;
  LFSVG.setAttribute("class", "life");
  for (var k in LFEL) LFEL[k].g.setAttribute("class", LFEL[k].base);
  for (var j in LFLN) LFLN[j].g.setAttribute("class", LFLN[j].base);
}
/* WHAT THE DRAWING LOOKS LIKE WITH NO POINTER ON IT: the keyboard's row if a row
   has the focus, dark if none does. The pointer leaving is not the focus leaving,
   and a reader who tabbed to a row and then moved the mouse across the page must
   not watch that row's family go out while the row still wears the focus ring. */
function lfRest(){
  if (lfFocusKey && LFEL[lfFocusKey]) lfLight(lfFocusKey); else lfDark();
}

function el(tag, cls, text){
  var e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = String(text);
  return e;
}

/* A term-marked element, built rather than templated so it can carry a name
   beside it without either becoming markup. */
function termEl(tag, term, text){
  var e = el(tag, "term", text);
  e.setAttribute("data-t", term);
  return e;
}

/* An unknown, in the page's own voice: "?" in the warning colour. §10.1's rule
   has no exception on this tab either. */
function unkEl(text){ return el("span", "unknown", text == null ? "?" : text); }

/* svgEl and trSpan keep the two safe primitives in one place: an element is
   created, never parsed, and a string becomes a text node on a <tspan>. */
function svgEl(tag, cls){
  var e = document.createElementNS(SVGNS, tag);
  if (cls) e.setAttribute("class", cls);
  return e;
}
/* trDay is a calendar day from an epoch-millisecond stamp, in UTC and in ISO
   order. Ages — "4h 12m ago" — are how everything about NOW is written here;
   the time axis and the record's floor are fixed points weeks back, where an age
   in hours is unreadable and a local rendering would put two readers a day apart
   on the same fact. */
function trDay(msv){ return new Date(msv).toISOString().slice(0,10); }
function trClock(msv){ return new Date(msv).toISOString().slice(5,16).replace("T", " "); }

function trSpan(parent, cls, text, dx){
  var s = svgEl("tspan", cls);
  if (dx) s.setAttribute("dx", String(dx));
  s.textContent = String(text);
  parent.appendChild(s);
  return s;
}

/* ---- the drawing's geometry.

   COLUMNS, then a plot. The label column is fixed and CLIPPED, because a species
   name is 64 bytes somebody else chose and a name allowed to run into the plot
   would draw itself over the timeline. The full name is always in the row's
   tooltip, which cannot be overflowed.

   The whole thing scrolls inside its own box, like the map and the tables. */
var LF_ROWH = 24, LF_PADT = 44, LF_DETL = 15, LF_DETPAD = 12;
var LF_GLYPHX = 14, LF_NAMEX = 30, LF_NAMEW = 366, LF_MINIX = 408;
/* LF_PLOTW is the plot's width when nothing can be measured — a hidden panel has
   no width to fit. When the box CAN be measured the plot takes whatever it
   leaves, down to LF_PLOTMIN, because the most important pixel on this drawing is
   the right-hand edge: it is NOW, and every living bar ends there. A fixed 700
   put it 147px past the right edge of a 1280-wide window with the box scrolled to
   0, so the one mark a reader is looking for was the one mark off the screen. */
var LF_DOT = 9, LF_SPARKW = 84, LF_PLOTW = 700, LF_PLOTMIN = 280, LF_PLOTPAD = 30;
/* Room kept for the box's own vertical scrollbar. The width is measured on a box
   that has not been filled yet, so the scrollbar that the fill puts there is not
   in the measurement — and a drawing sized to the whole box then overflows it by
   exactly that much and grows a horizontal scrollbar to say so. */
var LF_SCROLLW = 16;
/* The badge line's own height and its left inset. A row that carries no badge is
   not made taller for one. */
var LF_BADGEH = 13, LF_BADGEX = 6;
/* The bar: thin for an ancestor nothing is alive of, thicker for a living
   species, and the same thickness for every species of a kind. THICKNESS IS NOT
   THE BRAIN — see lfBrainR. */
var LF_BARH = 8, LF_ANCH = 4;
/* ---- THE FAMILY, READ DOWN THE LABEL COLUMN.

   The drawing said who descends from whom in ONE place: a line dropping onto a
   bar somewhere out in the plot. With fourteen rows and bars scattered across
   days of axis, two related rows can be a hand's width apart and the drop between
   them crosses everything in between — so the structure was there and unreadable.

   So it is said TWICE, and the second saying is the one a reader scans: every row
   is indented by its depth, and a bracket joins a parent's row down to each of its
   children — one rail per parent, one stub per child, the last stub ending the
   rail. That is the shape of a printed tree, and it works no matter where the bars
   are, because it uses no horizontal position that means anything.

   THE INDENT IS CAPPED. Forty generations of it would push every name off its own
   column, so past LF_MAXD the rows stop stepping right; the bracket is still drawn
   between the true parent and the true child, so what a reader sees is a family
   that stops indenting rather than a family that changes shape. */
var LF_INDENT = 13, LF_MAXD = 6;
function lfIndent(n, joined){
  return joined ? Math.min(n.depth || 0, LF_MAXD) * LF_INDENT : 0;
}
/* ---- A LINK IS ON THE CLOCK, ALL OF IT.

   A collapsed run used to be drawn as a dotted lead-in immediately left of the
   child's bar whose LENGTH COUNTED GENERATIONS — five pixels each — and the page
   said so in four places because it was the one mark on this drawing that was not
   on the time axis. It was still wrong. The generations it stood for lived
   somewhere, and where they lived was the blank stretch between the end of the
   ancestor's bar and the start of the child's: on the running rig, 40.1 h of empty
   plot with a 50 px mark beside it claiming the same run on a different scale. A
   drawing that puts one distance on two scales cannot be read.

   So there is no exception any more. Every horizontal distance here is time, and a
   link is drawn across the real interval it spans:

     IT LEAVES THE PARENT at the latest moment the parent's OWN record supports
     that is not after the child's first-seen — the end of its bar when it is
     extinct, and the right-hand edge when it is alive, which is to say the child's
     own first-seen and no run at all.

     IT LANDS at the child's first-seen point, which is the one horizontal
     position a join can honestly have and is where it always landed.

   THE NUMBER STAYS, and it is now the only thing it is for: a count of
   generations is not recoverable from a duration. Forty generations and one
   generation can cross the same forty hours.

   lfLink is that geometry as DATA — a function of the two nodes' published spans
   and the scale, and of nothing else, so what it answers is the record's. Three
   shapes come out of it and lfJoin draws them. */
/* The smallest run this drawing will draw. Ten generations across a four-minute
   gap is a fraction of a pixel on three days of axis, and a mark nobody can see is
   a mark that is not there — so a positive run is widened to 2.5 px. That is the
   SAME tolerance, for the same reason, as the shortest bar (lfBar): it overstates
   the interval by at most those pixels, and it is the only place either mark is
   allowed to. */
var LF_RUNMIN = 2.5;
/* How far above the child's bar an INVERTED link runs back — see lfLink's third
   case. It cannot run along the bar's own centreline the way a forward run does,
   because the stretch it crosses is a stretch that bar already occupies. */
var LF_REVY = 8;
function lfLink(n, p, sc){
  var jx = sc.x(n.spanFromMs);
  var out = {jx: jx, lx: jx, run: 0, rev: false};
  // NO PARENT BAR TO LEAVE. A parent no crossing of its own was ever recorded for
  // is drawn as a mark at now and nothing else, so there is no stretch of its row
  // for the link to contradict: it drops where it lands, as it always did.
  if (!(p && p.spanFromMs)) return out;
  if (n.spanFromMs < p.spanFromMs){
    // THE RECORD SAW THE CHILD FIRST. Its own earliest crossing is earlier than
    // its parent's, so a drop at the child's first-seen would come down on a
    // stretch of the parent's row that has no bar in it. The earliest moment the
    // parent's record supports is the start of its bar, so that is where the link
    // leaves — and it then runs BACKWARDS, which is a fact about the record and is
    // drawn as one.
    out.rev = true;
    out.lx = sc.x(p.spanFromMs);
    out.run = out.lx - jx;
    if (out.run < LF_RUNMIN){ out.run = LF_RUNMIN; out.lx = jx + LF_RUNMIN; }
    return out;
  }
  // A LIVING PARENT HAS NO LAST MOMENT: its bar runs to the right-hand edge, so
  // every child of it starts inside its span and there is no stretch to cross.
  var lastMs = p.alive ? 0 : (p.spanToMs || p.spanFromMs);
  if (lastMs && n.spanFromMs > lastMs){
    out.lx = sc.x(lastMs);
    out.run = jx - out.lx;
    if (out.run < LF_RUNMIN){ out.run = LF_RUNMIN; out.lx = jx - LF_RUNMIN; }
  }
  return out;
}
/* THE BRAIN, as a ring at the end of the bar — the end, because the stats come
   from the LATEST genome of that species the record named, so they describe what
   it is now and not what it was when it first crossed.

   A ring and not a thickness: thickness has to have a value for every bar, and
   most of the honest answers here are ABSENT — a genome pruned past the
   retention horizon, or never fetched, has no brain to draw. An absent ring is
   nothing at all, which cannot be misread as a small brain. Nine of sixteen
   species on the running rig have no ring for exactly that reason.

   ITS SCALE IS FITTED TO WHAT IS DRAWN, and that is a change with a cost worth
   stating. The ring used to be an ABSOLUTE size: (neurons + synapses) log-scaled
   against a fixed 600, which is a range this data does not occupy. Measured on
   the live tree, the seven species that have stats at all run 88 to 113 — 5.21 px
   of radius to 5.36 px. A species carrying 29% more brain than another drew a ring
   0.15 px bigger, which is to say the mark encoded nothing at all.

   So the smallest brain among the species DRAWN RIGHT NOW is the smallest ring
   and the largest is the largest, and everything between is placed on that span.
   What is bought is a mark that varies with the thing it stands for. What is paid
   is that a ring no longer means a size — it means a PLACE AMONG THE ROWS IN FRONT
   OF YOU, and it re-fits when the drawn set turns over, exactly as the time axis
   already does. That is said in the glossary, in the legend and on the ring's own
   tooltip, which carries the real counts so the absolute answer is never more than
   a hover away.

   IT IS STILL LOGARITHMIC INSIDE THE FITTED RANGE. One species ten times the rest
   would otherwise press every other ring flat against the floor, which is the same
   failure this change exists to undo, one range in. */
var LF_BRAIN_R0 = 2.4, LF_BRAIN_R1 = 6.4;
/* The range the last paint fitted, and how many species it was fitted over. */
var LFBRAIN = {lo: 0, hi: 0, n: 0};
function lfBrainW(n){ return (n.neurons || 0) + (n.synapses || 0); }
function lfBrainRange(list){
  var out = {lo: 0, hi: 0, n: 0};
  for (var i=0;i<list.length;i++){
    var w = lfBrainW(list[i]);
    if (w <= 0) continue;
    if (!out.n || w < out.lo) out.lo = w;
    if (!out.n || w > out.hi) out.hi = w;
    out.n++;
  }
  return out;
}
function lfBrainR(n){
  var w = lfBrainW(n);
  // NO COPY OF ITS GENOME, NO RING. Absence is drawn as nothing, never as the
  // smallest ring on the drawing — the two answers are not the same answer.
  if (w <= 0) return 0;
  var b = LFBRAIN, mid = (LF_BRAIN_R0 + LF_BRAIN_R1) / 2;
  // ONE MEASUREMENT IS NOT A COMPARISON, and neither is a set that is all one
  // number. Both get the MIDDLE of the span: the largest ring would claim a
  // biggest that nothing was measured against, the smallest would claim the
  // reverse, and the middle claims neither. It is also what keeps the divisor off
  // zero, which is the same case seen from the arithmetic.
  if (b.n < 2 || !(b.hi > b.lo)) return mid;
  var f = (Math.log(1 + w) - Math.log(1 + b.lo)) /
          (Math.log(1 + b.hi) - Math.log(1 + b.lo));
  if (f < 0) f = 0; else if (f > 1) f = 1;
  return LF_BRAIN_R0 + f * (LF_BRAIN_R1 - LF_BRAIN_R0);
}
/* The ring's own tooltip: the REAL numbers, which the drawing itself cannot carry
   now that the mark is a comparison. It names where the numbers came from, and it
   says the one thing the ring's size no longer says. */
function lfBrainAge(n){
  if (!n.brainAtMs) return "";
  var age = (LFX && LFX.generatedAtMs ? LFX.generatedAtMs : Date.now()) - n.brainAtMs;
  if (age < 0) age = 0;
  return ms(age);
}
function lfBrainTip(n){
  var b = LFBRAIN, w = lfBrainW(n), age = lfBrainAge(n), body =
    n.neurons + " neurons and " + n.synapses + " synapses, from the newest genome of this " +
    "species this archive EVER READ" +
    // AND WHEN. The measurement is kept after the copy it came from is gone, so
    // it can be days old — and an old reading drawn beside a current one must say
    // which it is, or the row invites a comparison of two different instants
    // presented as one.
    (age ? " — a genome that crossed " + age + " ago" +
      (n.alive ? "." : ", and nothing of this species is alive anywhere reporting, so no " +
        "newer one can arrive.")
        : ".");
  if (b.n < 2 || !(b.hi > b.lo)){
    body += " It is the only brain measured on this drawing" +
      (b.n > 1 ? " size — every species here that has a genome carries the same total" : "") +
      ", so the ring is drawn mid-scale rather than claiming a biggest or a smallest.";
  } else {
    body += " The ring is sized against the species drawn RIGHT NOW — " + b.lo + " to " +
      b.hi + " across " + b.n + " of them — so it says where this one sits among the rows in " +
      "front of you and not how big a brain is." +
      (w >= b.hi ? " It is the largest on this drawing."
                 : (w <= b.lo ? " It is the smallest on this drawing." : "")) +
      " Reveal a row, search, or let a species leave the census and the scale re-fits, the " +
      "way the clock along the top does.";
  }
  return {title: "brain size", body: body};
}

/* The column x positions, derived from the MAP'S OWN SHAPE rather than from a
   constant: this rig is 3 wide and 2 high and the next one is not, so the
   mini-map's width is the map's width and everything right of it moves. */
function lfCols(x){
  var m = (x && x.map) || {};
  // THE MAP'S REAL WIDTH, with no cap of its own: a wider map makes a wider
  // column and a wider drawing, which scrolls. Clamping it here would drop the
  // right-hand worlds off every species' mini-map without saying so, which is
  // the one thing this page does not do to a reader.
  var cols = Math.max(1, m.width || 1);
  var miniW = Math.max(30, cols * LF_DOT);
  var pop = LF_MINIX + miniW + 52;
  var spark = pop + 16;
  var plot = spark + LF_SPARKW + 24;
  // THE PLOT FILLS WHAT THE BOX LEAVES. The columns to its left are text and
  // dots and have the widths they have; the timeline is the elastic one, so the
  // drawing is as wide as its container and the now edge is on the screen
  // without anybody scrolling to it. A box that cannot be measured — a panel
  // still hidden — gets the default, and the resize that follows repaints it.
  var host = document.getElementById("lfbox");
  var avail = host ? host.clientWidth : 0;
  var plotw = avail > 0
    ? Math.max(LF_PLOTMIN, avail - plot - LF_PLOTPAD - LF_SCROLLW)
    : LF_PLOTW;
  return {mini: LF_MINIX, miniW: miniW, pop: pop, spark: spark, plot: plot,
          plotw: plotw, w: plot + plotw + LF_PLOTPAD};
}

/* The time axis: a linear scale from the earliest thing the record dates (or the
   record's own ancestry floor, whichever is older — the server publishes the
   answer so the floor is always inside the picture) to now.

   TWO PUBLISHED LEFT EDGES, and the drawing picks the one that fits what it is
   actually drawing: spanStartMs is the axis without the seed stock, which is the
   default set of rows, and spanStartSeedMs is the axis with it. Revealing a seed
   species STRETCHES the axis instead of clamping its bar against the left edge —
   and it does that from the answer already in hand, with no second request. */
function lfScale(x, cols, seed){
  var t0 = (seed ? (x.spanStartSeedMs || x.spanStartMs) : x.spanStartMs) || 0;
  var t1 = x.spanEndMs || x.generatedAtMs || Date.now();
  if (!(t1 > t0)) t1 = t0 + 1;
  return {t0: t0, t1: t1, x: function(msv){
    var f = (msv - t0) / (t1 - t0);
    if (f < 0) f = 0; else if (f > 1) f = 1;
    return cols.plot + f * cols.plotw;
  }};
}
var LF_STEPS = [3600000, 10800000, 21600000, 43200000, 86400000, 172800000,
                604800000, 1209600000, 2592000000];
function lfStep(span){
  for (var i=0;i<LF_STEPS.length;i++) if (span / LF_STEPS[i] <= 7) return LF_STEPS[i];
  return LF_STEPS[LF_STEPS.length-1];
}

/* ---- THE SEED STOCK, and why a row is not there.

   A SEED SPECIES IS ONE EVERY WORLD HOLDING IT REFUSES TO EXPORT. The server
   evaluates that against the exclusion lists it holds and marks the node
   (species.go, tree.go) — the policy is the archive's data and re-deriving it
   here would be a second opinion waiting to drift. On this map it is the game's
   own starting template: a bar the full width of the picture, running since the
   record began, with no living descendant and no part in anything the rest of the
   drawing is about. It took over the timeline and answered nothing.

   SO IT IS LEFT OUT OF THE ROWS AND OUT OF THE AXIS, and SAID. A filter a reader
   cannot see is a filter that makes the view wrong, so the count is on the stat
   line with the reason beside it and a control that undoes it. The state is this
   variable and nothing else: no request, no storage, no query string — a reload
   is back to the default.

   A SEARCH BEATS THE FILTER. Typing a name that matches a seed species produces
   the row. Answering "no species matches that search" about a species this view
   is holding back would be the view lying about its own contents, and that is a
   worse surprise than a row appearing. */
var lfSeedShown = false;
function lfSeedHidden(n){
  if (!n.seedStock || lfSeedShown) return false;
  return !(lfQuery && lfMatches(n));
}

function lfMatches(n){
  if (!lfQuery) return true;
  if (String(n.key).toLowerCase().indexOf(lfQuery) >= 0) return true;
  if (String(n.name).toLowerCase().indexOf(lfQuery) >= 0) return true;
  var alt = n.spellings || [];
  for (var i=0;i<alt.length;i++){
    if (String(alt[i]).toLowerCase().indexOf(lfQuery) >= 0) return true;
  }
  return false;
}

/* Which rows are drawn, in which order, and whether the joins between them mean
   anything.

   FAMILY is the server's own DFS pre-order, so a parent is always above its
   children and one pass places every edge. BY POPULATION is a flat ranking of
   what is ALIVE — the answer the old census table gave — and it draws no edges
   at all, because rows in abundance order are not in family order and a line
   between two of them would cross half the drawing to say nothing.

   A SEARCH KEEPS THE FAMILY. A matching row brings its ancestors with it, so a
   filtered tree is still a tree rather than a set of orphans.

   THE SEED FILTER RUNS IN BOTH ORDERS, and the two counts it returns are what
   the stat line reports: how many rows it took out, and how many seed rows are
   drawn anyway — by the toggle, or because the reader searched for one. They are
   counted from the rows this call actually produced rather than from the
   server's total, because the notice describes THIS drawing.

   IT TAKES THE ROW OUT AND NOTHING ELSE. Should a hidden seed species ever be
   some drawn row's parent — it is not on this map, where nothing descends from
   the starting template in the record, but the rule is a rule — that child keeps
   its place and its indentation and simply has no line dropping onto it, which is
   what every row in the population order looks like anyway. Reparenting it onto a
   grandparent it never descended from would be a lie about the record told to
   tidy a picture, and this file does not tell those. */
function lfRows(x){
  var nodes = (x && x.nodes) || [], i, hid = 0, seed = 0;
  function drop(n){
    if (!lfSeedHidden(n)) { if (n.seedStock) seed++; return false; }
    hid++;
    return true;
  }
  if (lfSort === "pop"){
    var flat = [];
    for (i=0;i<nodes.length;i++){
      if (!nodes[i].alive || !lfMatches(nodes[i])) continue;
      if (drop(nodes[i])) continue;
      flat.push(nodes[i]);
    }
    flat.sort(function(a,b){
      if (b.population !== a.population) return b.population - a.population;
      return a.key < b.key ? -1 : (a.key > b.key ? 1 : 0);
    });
    return {list: flat, joined: false, hid: hid, seed: seed};
  }
  var byKey = {}, keep = {}, out = [];
  if (!lfQuery){
    for (i=0;i<nodes.length;i++){
      if (drop(nodes[i])) continue;
      out.push(nodes[i]);
    }
    return {list: out, joined: true, hid: hid, seed: seed};
  }
  for (i=0;i<nodes.length;i++) byKey[nodes[i].key] = nodes[i];
  for (i=0;i<nodes.length;i++){
    if (!lfMatches(nodes[i])) continue;
    var cur = nodes[i], guard = 0;
    while (cur && !keep[cur.key] && guard++ < 600){
      keep[cur.key] = true;
      cur = cur.parent ? byKey[cur.parent] : null;
    }
  }
  for (i=0;i<nodes.length;i++){
    if (!keep[nodes[i].key]) continue;
    // A seed species dragged in as some matching row's ANCESTOR is still hidden:
    // lfSeedHidden spares the row the search itself matched and no other.
    if (drop(nodes[i])) continue;
    out.push(nodes[i]);
  }
  return {list: out, joined: true, hid: hid, seed: seed};
}

/* The two counts lines. Neither carries a name, and both are built out of nodes
   anyway, because the rule for this region is that markup is never assigned in
   it — one exception is how a region stops being a fence. */
function lfCount(x){
  var host = document.getElementById("lfcount");
  if (!host) return;
  while (host.firstChild) host.removeChild(host.firstChild);
  if (!x || !x.haveStatus){
    host.appendChild(unkEl("the relay has broadcast no map yet"));
    return;
  }
  host.appendChild(document.createTextNode(
    x.alive + " alive across " + x.reportingSlots + " reporting world(s)"));
  if (x.censuslessSlots > 0){
    host.appendChild(document.createTextNode(" · "));
    host.appendChild(unkEl(x.censuslessSlots + " world(s) report no census"));
  }
  if (x.truncatedSlots > 0){
    host.appendChild(document.createTextNode(" · "));
    host.appendChild(unkEl(x.truncatedSlots + " census(es) capped at 32; the rest is unreported"));
  }
  if (x.ledgerOverflow > 0){
    host.appendChild(document.createTextNode(" · "));
    host.appendChild(unkEl(x.ledgerOverflow + " species past the aggregate's bound, untracked"));
  }
  // HOW SHORT THE TREND IS, SAID OUT LOUD. The sampled record began when this
  // archive did, which on the running rig is days rather than months, and a
  // 24-hour trend with two hours of samples in it looks exactly like a species
  // that arrived two hours ago. An empty column says so rather than reading as
  // "nothing happened".
  if (LFTRENDMETA && !LFTRENDMETA.samples){
    host.appendChild(document.createTextNode(" · "));
    host.appendChild(unkEl("no sample covers the last day yet, so no trend is drawn — " +
      "this record began when the archive did"));
  }
}

function lfStats(x, hid, seed){
  var host = document.getElementById("lfstat");
  if (!host) return;
  // THIS LINE IS REBUILT EVERY POLL TOO, and the one control on it is a real
  // button — so the keyboard loses it the way the rows lose the focus, and the
  // reader who just pressed it is the one most likely to press it again. Same
  // rule as the drawing: the paint puts the focus back only where it took it.
  var ae = document.activeElement,
      keepBtn = !!(ae && ae.closest && ae.closest(".seedbtn") && host.contains(ae)),
      newBtn = null;
  while (host.firstChild) host.removeChild(host.firstChild);
  if (!x || !x.haveStatus){
    host.appendChild(unkEl("the relay has broadcast no map yet"));
    return;
  }
  function stat(term, label, value, extra){
    var s = el("span", "muted");
    s.appendChild(term ? termEl("span", term, label) : document.createTextNode(label));
    s.appendChild(document.createTextNode(" "));
    s.appendChild(el("b", null, value));
    if (extra) s.appendChild(document.createTextNode(extra));
    host.appendChild(s);
  }
  stat("alive", "alive", x.alive);
  stat(null, "joined by the record", x.connected);
  if (x.isolated > 0){
    var s = el("span", "muted");
    s.appendChild(termEl("span", "noancestry", "standing alone"));
    s.appendChild(document.createTextNode(" "));
    s.appendChild(el("b", null, x.isolated));
    // THE TWO REASONS A LEAF STANDS ALONE, split, because they are different
    // facts: no ancestry recorded at all, versus a recorded family that is
    // extinct. Reporting one number for both would let a reader conclude the
    // record knows less than it does.
    s.appendChild(document.createTextNode(" (" + x.unrecorded +
      " with no ancestry recorded, " + (x.isolated - x.unrecorded) +
      " whose recorded family is gone)"));
    host.appendChild(s);
  }
  // THE ONE SET OF ROWS THIS VIEW LEAVES OUT ON PURPOSE, counted out loud and
  // undoable. x.seedStock is the server's own count of living seed species and
  // gates the whole clause: no seed stock in the answer, no notice, whatever any
  // filter here believes. The numbers in the sentence are this drawing's — how
  // many rows went, or how many are up — because that is what the reader is
  // looking at. NOTHING IN IT IS A NAME: static words and integers only, which is
  // why the badge line and this line can both be built without escaping anything.
  if (x.seedStock > 0 && (hid > 0 || seed > 0)){
    var sd = el("span", "muted");
    sd.appendChild(el("b", null, hid > 0 ? hid : seed));
    sd.appendChild(document.createTextNode(" "));
    sd.appendChild(termEl("span", "seedstock",
      hid > 0 ? "seed species hidden"
              : (lfSeedShown ? "seed species shown" : "seed species shown by your search")));
    sd.appendChild(document.createTextNode(
      " — excluded from migration on every world where it lives"));
    if (hid > 0 || lfSeedShown){
      sd.appendChild(document.createTextNode(" · "));
      var btn = el("button", "seedbtn", hid > 0 ? "show" : "hide");
      btn.setAttribute("type", "button");
      sd.appendChild(btn);
      newBtn = btn;
    }
    host.appendChild(sd);
  }
  stat("branchpoint", "branch points drawn", x.ancestors);
  if (x.collapsed > 0) stat("collapsed", "generations collapsed", x.collapsed);
  stat(null, "the record's deepest line", x.maxDepth, " generation(s)");
  // The reach of the whole record behind the reduced tree, so the tab is honest
  // about being a small view of a big thing.
  var r = el("span", "muted");
  r.appendChild(document.createTextNode("the crossing record holds "));
  r.appendChild(el("b", null, x.ledgerSpecies));
  r.appendChild(document.createTextNode(" species, "));
  r.appendChild(el("b", null, x.ledgerEdges));
  r.appendChild(document.createTextNode(" of them with a parent named — almost all extinct, " +
    "which is why only what is alive is drawn"));
  host.appendChild(r);
  // THE RECORD'S OWN FLOOR, which is the answer to the question every root
  // provokes. It is a maintained timestamp and not a scan (species.go
  // edgeFirstMs), and it is absent rather than zero when no record has ever
  // named a parent. The drawing marks the same instant as a line.
  if (x.ancestrySinceMs){
    var f = el("span", "muted");
    f.appendChild(termEl("span", "recordfloor", "ancestry recorded since"));
    f.appendChild(document.createTextNode(" "));
    f.appendChild(el("b", null, trDay(x.ancestrySinceMs)));
    f.appendChild(document.createTextNode(
      " UTC — the oldest crossing kept here that names a parent"));
    host.appendChild(f);
  }
  if (x.cycleGuard > 0 || x.walkCapped > 0 || x.nodesCapped){
    var g = el("span");
    g.appendChild(unkEl("the record holds " +
      (x.cycleGuard > 0 ? x.cycleGuard + " ancestry loop(s) " : "") +
      (x.walkCapped > 0 ? x.walkCapped + " over-long line(s) " : "") +
      (x.nodesCapped ? "more nodes than this view draws " : "") +
      "— guarded, and drawn as far as it was safe to"));
    host.appendChild(g);
  }
  if (keepBtn && newBtn && newBtn.focus) newBtn.focus();
}

/* lfTip is one row's tooltip, as strings in the SP registry — the same registry
   the map's species runs use, filled by showTip with textContent. */
function lfTip(n){
  var lines = [];
  if (n.alive){
    var where = [];
    var worlds = n.worlds || [];
    for (var i=0;i<worlds.length;i++){
      where.push("S" + worlds[i].slot + ": " + worlds[i].bibites +
        (worlds[i].eggs ? " (+" + worlds[i].eggs + " egg(s))" : ""));
    }
    lines.push(n.population + " alive right now" + (n.eggs ? ", " + n.eggs + " egg(s)" : "") +
      " in " + worlds.length + " world(s) — " + where.join(", "));
  } else {
    // §10.1's rule, on the one node type that could most easily be misread as a
    // resident: it is NOT alive, and the sentence says so before anything else.
    lines.push("Not alive in any world that is reporting a census. It is drawn because " +
      n.leaves + " living species descend from it by different lines — it is where those " +
      "lines part.");
  }
  if (n.alive && n.leaves > 1){
    lines.push("It is also an ancestor here: " + (n.leaves - 1) +
      " other living species descend from it.");
  }
  // WHY THIS ROW IS NORMALLY NOT HERE, said on the row itself: a reader who
  // revealed it, or who found it by searching, arrives at it without having read
  // the notice on the tab.
  if (n.seedStock){
    lines.push("Seed stock: every world it lives in refuses to export it, so nothing of it can " +
      "leave anywhere while that holds. This view leaves such a species out of its rows and out " +
      "of the time axis by default, and says on the tab that it did.");
  }
  // THE BAR, said in words, because a bar on a time axis is the one mark here a
  // reader will otherwise read as a lifespan.
  if (n.spanFromMs){
    lines.push("The bar runs from the first crossing this archive recorded of it — " +
      trClock(n.spanFromMs) + " UTC, " + ms(Date.now() - n.spanFromMs) + " ago" +
      (n.spanDerived ? ", which is its earliest recorded DESCENDANT rather than itself: no " +
        "crossing of this species was ever recorded" : "") + " — " +
      (n.alive ? "to now, because it is alive."
               : "to the last one, " + trClock(n.spanToMs || n.spanFromMs) + " UTC. That is " +
                 "when the RECORD of it stops, not when it died."));
  } else {
    lines.push("No crossing of it has ever been recorded, so the drawing can date neither " +
      "end of it.");
  }
  if (n.neurons){
    var bage = lfBrainAge(n);
    lines.push("Brain: " + n.neurons + " neurons and " + n.synapses + " synapses, from the " +
      "newest genome of it this archive ever read" +
      (bage ? ", which crossed " + bage + " ago" : "") + ". The reading is kept after the copy " +
      "it came from is gone, so an extinct line keeps the last brain that was ever seen of it. " +
      "The ring at the end of the bar is where that sits AMONG THE SPECIES DRAWN RIGHT NOW " +
      "rather than an absolute size, and it re-scales as those change — hover the ring itself " +
      "for the range it was fitted to.");
  }
  // WHO IT CAME FROM, WITHOUT OPENING THE ROW. The name of the parent species was
  // only ever in the expanded detail, so the drawing's own subject — descent —
  // was the one thing a reader had to click for. It is safe here for the reason
  // every name on this view is safe: a tip is filled with textContent (showTip),
  // never with markup.
  //
  // AND THE THREE CASES ARE THREE SENTENCES. A row whose bracket goes to its
  // actual recorded parent is not the same as one whose bracket skips a run of
  // extinct generations, and neither is the same as one whose parent is not on
  // this drawing at all.
  if (n.parentName){
    if (n.collapsed > 0){
      lines.push("The record names " + n.parentName + " as its parent species. Nothing of " +
        "that one is alive and no other living line runs through it, so it is not a row here: " +
        "the bracket beside the name reaches past it to the nearest ancestor that IS drawn, " +
        n.collapsed + " generation(s) further up, and the +" + n.collapsed + " on the link " +
        "counts the ones in between.");
    } else if (n.parent){
      lines.push("Descends from " + n.parentName + ", which is the row the bracket beside the " +
        "name joins it to. The line landing on its bar is that same link, and it lands at the " +
        "moment the record first named this species.");
    } else {
      lines.push("The record names " + n.parentName + " as its parent species. Nothing of that " +
        "line is alive on this map, so there is no row above this one to join it to.");
    }
  }
  if (n.ancestryKnown){
    lines.push("The record traces it back " + n.ancestryDepth + " generation(s).");
    // WHAT THE ROOT BADGE MEANS, on the node that wears it: those generations
    // are recorded and not drawn, and the chain stops at the record's own edge
    // rather than at the beginning of anything.
    if (!n.parent){
      lines.push("Nothing is drawn above it because not one of those " + n.ancestryDepth +
        " ancestors has another living branch on this map, so the whole run is folded into " +
        "this row — and the run ends where the RECORD ends rather than where the family did: " +
        "no crossing of the species at the top of it ever named a parent, and the earliest " +
        "crossing here that named one at all is dated on the tab.");
    }
  } else {
    lines.push("No crossing of it has ever named a parent species, so the record cannot say " +
      "where it came from.");
  }
  if (n.isolated){
    lines.push(n.ancestryKnown
      ? "Nothing else alive on this map descends from any of those ancestors, so it stands on " +
        "its own here — its family is recorded and its family is extinct."
      : "It stands on its own here. Ancestry on this map is a by-product of travel, and a " +
        "kind that has stayed home leaves no record to carry it.");
  }
  if (n.collapsed > 0){
    lines.push("The +" + n.collapsed + " on its link counts extinct generation(s) with no " +
      "living branch on them. It is printed because a count cannot be read off a duration: " +
      "one generation and forty of them can cross the same stretch of clock.");
  }
  // THE SHAPE OF ITS OWN LINK, IN WORDS, from the two dates the drawing itself
  // used. Every horizontal distance on this view is time, so a mark that crosses a
  // stretch can name the stretch — and where no mark is drawn, this says that
  // rather than describing one that is not there.
  var pn = n.parent ? LFBY[n.parent] : null;
  if (pn && pn.spanFromMs && n.spanFromMs){
    var pend = pn.spanToMs || pn.spanFromMs;
    if (n.spanFromMs < pn.spanFromMs){
      lines.push("ITS OWN RECORD STARTS BEFORE ITS PARENT'S, by " +
        ms(pn.spanFromMs - n.spanFromMs) + ". The descent is real and both dates are the " +
        "record's: a kind is first seen here when it happens to walk into another world, not " +
        "when it arose, and this one made that trip first. So its link is drawn leaving the " +
        "earliest point its parent's record supports — the left end of that bar, with a ring " +
        "on it — and running BACKWARDS in amber to where this one's record begins, rather " +
        "than dropping onto a stretch of its parent's row that has no bar in it.");
    } else if (!pn.alive && n.spanFromMs > pend){
      lines.push("The record of the row above stops " + ms(n.spanFromMs - pend) +
        " before this one's begins, and the link is drawn across exactly that stretch" +
        (n.collapsed > 0
          ? " — dotted, because it is the collapsed generations that carried the line through it."
          : "."));
    } else if (n.collapsed > 0){
      lines.push("Nothing is drawn across for those generations: the record of the row above " +
        "still runs where this one's begins, so it does not say which part of that span they " +
        "sat in, and the number is the whole of what is known.");
    }
  }
  lines.push(n.crossings + " recorded crossing(s)" +
    (n.genomes ? ", " + n.genomes + " distinct genome(s)" : "") + ".");
  lines.push(n.nameFrom === "census"
    ? "Spelled as the world holding it spells it now."
    : "Spelled as its descendants' crossing records named it — no census names it, because " +
      "nothing of its kind is alive.");
  return {title: String(n.name), body: lines.join(" ")};
}

/* The expanded row: what the flat census table used to put in a detail panel.
   One array of lines, built once, so the height the layout reserves and the
   lines the drawing emits cannot disagree. Each line is a list of segments, and
   a segment carrying a NAME is a segment like any other — it becomes a tspan. */
function lfDetailLines(n){
  var out = [], i;
  if (n.alive){
    var worlds = n.worlds || [], seg = [{t: "worlds", c: "dk", term: "world"}];
    if (!worlds.length) seg.push({t: "  none reporting", c: "unk"});
    for (i=0;i<worlds.length;i++){
      seg.push({t: "  S" + worlds[i].slot + " " + worlds[i].bibites +
        (worlds[i].eggs ? " +" + worlds[i].eggs + "e" : "")});
    }
    out.push(seg);
    var alt = n.spellings || [];
    if (alt.length){
      // A second spelling of one species is a real difference between two
      // worlds, not noise to be tidied away, so every one of them is shown.
      var sp = [{t: "also spelt", c: "dk", term: "rawname"}];
      for (i=0;i<alt.length;i++) sp.push({t: "  " + alt[i]});
      out.push(sp);
    }
  }
  var rec = [{t: "record", c: "dk", term: "crossings"},
             {t: "  " + n.crossings + " crossing(s)"}];
  if (n.genomes){
    rec.push({t: " · "});
    rec.push({t: n.genomes + " distinct genome(s)", term: "speciesgenomes"});
  }
  if (n.spanFromMs){
    rec.push({t: " · first " + ms(Date.now() - n.spanFromMs) + " ago"});
    if (!n.alive && n.spanToMs){
      rec.push({t: " · last " + ms(Date.now() - n.spanToMs) + " ago"});
    }
  }
  out.push(rec);
  // THE BRAIN, its AGE, and its absence. The reading outlives the copy it was
  // taken from, so it carries its own date: an ancestor extinct for days shows a
  // days-old measurement, and a row that showed it without saying so would put two
  // different instants side by side as if they were one. An absent one says WHY
  // rather than printing a zero — this archive has never once read a genome of it.
  var brain = [{t: "brain", c: "dk", term: "brainsize"}];
  if (n.neurons){
    var dage = lfBrainAge(n);
    brain.push({t: "  " + n.neurons + " neurons · " + n.synapses + " synapses"});
    brain.push({t: "  (the newest genome of it ever read here" +
      (dage ? ", " + dage + " old" : "") + ")", c: "dk"});
  } else {
    brain.push({t: "  no genome of it has ever been read here", c: "unk"});
  }
  out.push(brain);
  var par = [{t: "parent species", c: "dk", term: "parentspecies"}];
  if (n.parentName){
    par.push({t: "  " + n.parentName});
    par.push({t: "  as the world that named it reported at the time — shown, never resolved",
              c: "dk"});
  } else {
    par.push({t: "  none recorded", c: "unk"});
  }
  out.push(par);
  var recent = n.recent || [];
  for (i=0;i<recent.length && i<4;i++){
    out.push([{t: i === 0 ? "recent lanes" : "", c: "dk", term: i === 0 ? "crossings" : null},
              {t: "  S" + recent[i].fromSlot + (recent[i].exitEdge ? " " + recent[i].exitEdge : "") +
                  " → S" + recent[i].toSlot + "  " + ms(recent[i].ageMs) + " ago"}]);
  }
  if (recent.length){
    out.push([{t: "", c: "dk"},
              {t: "  the newest " + recent.length + " of " + n.crossings +
                  " — a sample, not the whole", c: "dk"}]);
  }
  return out;
}
function lfDetailHeight(n){
  return lfDetailLines(n).length * LF_DETL + LF_DETPAD;
}

/* The per-species MINI-MAP: one dot per seat, in the map's own arrangement,
   filled where this species lives. THE GRID IS THE MAP'S, published by the
   server from the same status frame the rows came from — six dots in two rows
   because THIS map is three by two, never because six is how many worlds there
   are.

   Three states and they are three different facts (§10.1): filled is alive
   there, hollow is a world that reported a census without it, and the warning
   ring is a world reporting no census at all — unknown, never absence. A seat
   nobody claimed is a hole and is drawn as nothing. */
function lfMini(g, x, n, top, cols){
  var m = (x && x.map) || {}, cells = m.cells || [];
  if (!cells.length || !n.alive) return;
  var rows = Math.max(1, m.height || 1);
  var pitch = Math.min(LF_DOT - 1, Math.max(4, (LF_ROWH - 8) / rows));
  var have = {}, worlds = n.worlds || [], i;
  for (i=0;i<worlds.length;i++) have[worlds[i].slot] = worlds[i];
  for (i=0;i<cells.length;i++){
    var c = cells[i];
    // A seat whose column is outside the map's own published width is a
    // malformed frame, not a world this drawing has room for.
    if (c.col * LF_DOT >= cols.miniW) continue;
    var d = svgEl("circle", have[c.slot] ? "wdot on"
                                         : (c.reporting ? "wdot off" : "wdot unk"));
    d.setAttribute("cx", String(cols.mini + c.col * LF_DOT + LF_DOT / 2));
    // Row 0 is the BOTTOM row of the map, and it is the bottom row here too.
    d.setAttribute("cy", String(top + 8 + (rows - 1 - c.row) * pitch));
    d.setAttribute("r", "2.6");
    g.appendChild(d);
  }
}

/* The trend line: this species' population across the whole map over the last
   day, from the ONE answer that carries every living species (/api/species/trends).
   It is a SHAPE and carries no numbers, so it carries no name either. */
function lfSpark(g, n, top, cols){
  if (!n.alive || !LFTREND) return;
  var tr = LFTREND[n.key];
  if (!tr || !tr.points || !tr.points.length) return;
  var pts = tr.points, max = Math.max(1, tr.max), n2 = pts.length;
  var base = top + LF_ROWH - 7, h = 13, d = "", open = false;
  for (var i=0;i<n2;i++){
    if (pts[i] == null){ open = false; continue; }
    var px = cols.spark + (n2 < 2 ? LF_SPARKW / 2 : (i / (n2 - 1)) * LF_SPARKW);
    var py = base - (pts[i] / max) * h;
    d += (open ? " L " : " M ") + px.toFixed(1) + " " + py.toFixed(1);
    open = true;
  }
  if (!d) return;
  var line = svgEl("path", "trend");
  line.setAttribute("d", d.trim());
  g.appendChild(line);
  var b = svgEl("line", "trendbase");
  b.setAttribute("x1", String(cols.spark)); b.setAttribute("x2", String(cols.spark + LF_SPARKW));
  b.setAttribute("y1", String(base + 1)); b.setAttribute("y2", String(base + 1));
  g.appendChild(b);
}

/* One species' bar, and the brain ring at the end of it. The row's own index is
   carried in so the ring can register a tooltip of its own without a species name
   ever becoming part of a key. */
function lfBar(g, n, top, sc, i){
  var cy = top + LF_ROWH / 2 - 1;
  if (!n.spanFromMs){
    // Nothing the record dates. A living species still gets a mark, at now,
    // because it IS here — what is missing is when it started, not whether.
    if (!n.alive) return;
    var dot = svgEl("circle", "undated");
    dot.setAttribute("cx", String(sc.x(sc.t1) - 3));
    dot.setAttribute("cy", String(cy));
    dot.setAttribute("r", "3.2");
    g.appendChild(dot);
    return;
  }
  var x0 = sc.x(n.spanFromMs);
  var x1 = n.alive ? sc.x(sc.t1) : sc.x(n.spanToMs || n.spanFromMs);
  // A span too short to see still gets a 2.5 px mark, and the mark is SLID LEFT
  // rather than grown past the axis — a bar crossing the now line would claim the
  // record reaches into a future it cannot have. Sliding keeps the mark visible
  // where shrinking it to the edge would erase the newest rows entirely; it costs
  // at most 2.5 px of left edge, on a width that is already a minimum and not a
  // measurement.
  if (x1 - x0 < 2.5){
    x1 = x0 + 2.5;
    var xend = sc.x(sc.t1);
    if (x1 > xend){ x1 = xend; x0 = xend - 2.5; }
  }
  var h = n.alive ? LF_BARH : LF_ANCH;
  var bar = svgEl("rect", "lfbar" + (n.alive ? " live" : " ext") +
                          (n.spanDerived ? " derived" : ""));
  bar.setAttribute("x", String(x0));
  bar.setAttribute("y", String(cy - h / 2));
  bar.setAttribute("width", String(x1 - x0));
  bar.setAttribute("height", String(h));
  bar.setAttribute("rx", "1.5");
  g.appendChild(bar);
  var r = lfBrainR(n);
  if (r > 0){
    var ring = svgEl("circle", "brain");
    ring.setAttribute("cx", String(x1 + r + 2));
    ring.setAttribute("cy", String(cy));
    ring.setAttribute("r", r.toFixed(1));
    // THE RING ANSWERS WITH THE NUMBERS ITS SIZE STOPPED CARRYING. Its own key is
    // generated from the row index, like the row's; the glossary term stays behind
    // it, so a paint that has not filled SP yet still explains the mark rather than
    // saying nothing.
    var bk = "lfb" + i;
    SP[bk] = lfBrainTip(n);
    ring.setAttribute("data-s", bk);
    ring.setAttribute("data-t", "brainsize");
    g.appendChild(ring);
  }
}

/* ONE PARENT-CHILD LINK, DRAWN IN BOTH PLACES, in one group of its own.

   THE BRACKET, in the label column: down from under the parent's glyph to the
   child's row, then a stub into the child's glyph. Siblings lay their rails on
   the same x, so a parent with three children reads as one rail with three stubs
   and the last stub ends it. It is drawn for EVERY link, including one whose
   child the record has never dated — the relationship is known even when the
   moment is not, and that is exactly the row a reader is most likely to wonder
   about.

   THE LINK, in the plot: it leaves the parent and lands on the child's bar AT
   THIS SPECIES' FIRST-SEEN POINT, which is the instant the record first says this
   kind exists — the one horizontal position the join can honestly have. It turns
   with a small elbow rather than crossing anything, so at a glance it flows into
   the bar it belongs to instead of competing with it.

   WHERE IT LEAVES IS lfLink'S ANSWER, and the three shapes are three different
   readings of the record:

     NO RUN — the child's first crossing falls inside the parent's own span, which
     is every child of a living parent and most children of an extinct one. The
     link is a plain drop at that point, exactly as it always was.

     A FORWARD RUN — the parent's record stops before the child's begins. The link
     leaves at the end of the parent's bar and travels along the child's row into
     the start of its bar, across the real interval in which this line was carried
     by nothing on the drawing. It is DOTTED when collapsed generations carried it,
     and plain when the link is direct and it is only the record that is silent.

     A BACKWARD RUN — the child's first crossing is EARLIER than its parent's. It
     is drawn in the warning colour, above the child's bar rather than along it,
     with a ring where it left the parent, because it is not ordinary descent and
     must not be read as any.

   THE +N GOES WITH THE SHAPE: centred over a run where there is one, and tucked
   against the drop where there is not.

   THE GROUP IS THE UNIT OF LIGHTING. Every mark carries the child's key, so
   lighting one lineage lights the whole link and never half of it. */
function lfJoin(links, n, rowY, rowInd, sc){
  var py = rowY[n.parent], cy = rowY[n.key];
  if (py == null || cy == null) return null;
  var g = svgEl("g", "ln");
  g.setAttribute("data-k", n.key);
  var gxp = LF_GLYPHX + (rowInd[n.parent] || 0), gx = LF_GLYPHX + (rowInd[n.key] || 0);
  var tw = svgEl("path", "tw");
  tw.setAttribute("d", "M" + gxp + " " + (py + 7) + " V" + cy + " H" + (gx - 5));
  g.appendChild(tw);
  if (n.spanFromMs){
    var lay = lfLink(n, LFBY[n.parent], sc);
    var jx = lay.jx, lx = lay.lx, lbx = jx - 4, lba = "end", lby = cy - 4;
    if (lay.rev){
      var ry = cy - LF_REVY;
      var rv = svgEl("path", "rev");
      rv.setAttribute("d", "M" + lx.toFixed(1) + " " + py + " V" + ry +
        " H" + jx.toFixed(1) + " V" + (cy - 2));
      rv.setAttribute("data-t", "beforeparent");
      g.appendChild(rv);
      var mk = svgEl("circle", "revmark");
      mk.setAttribute("cx", lx.toFixed(1));
      mk.setAttribute("cy", String(py));
      mk.setAttribute("r", "3");
      mk.setAttribute("data-t", "beforeparent");
      g.appendChild(mk);
      lbx = (lx + jx) / 2; lba = "middle"; lby = ry - 4;
    } else if (lay.run > 0){
      // The elbow never overshoots the run it turns into: a run at the 2.5 px
      // floor would otherwise be drawn as a hook doubling back on itself.
      var er = Math.min(5, lay.run);
      var run = svgEl("path", n.collapsed > 0 ? "chain" : "link");
      run.setAttribute("d", "M" + lx.toFixed(1) + " " + py + " V" + (cy - er).toFixed(1) +
        " Q" + lx.toFixed(1) + " " + cy + " " + (lx + er).toFixed(1) + " " + cy +
        " H" + jx.toFixed(1));
      g.appendChild(run);
      lbx = (lx + jx) / 2; lba = "middle";
    } else {
      var drop = svgEl("path", "link");
      drop.setAttribute("d", "M" + jx.toFixed(1) + " " + py + " V" + (cy - 5) +
        " Q" + jx.toFixed(1) + " " + cy + " " + (jx + 5).toFixed(1) + " " + cy);
      g.appendChild(drop);
    }
    if (n.collapsed > 0){
      var gt = svgEl("text", "gen");
      gt.setAttribute("x", lbx.toFixed(1));
      gt.setAttribute("y", String(lby));
      gt.setAttribute("text-anchor", lba);
      gt.setAttribute("data-t", "collapsed");
      gt.textContent = "+" + n.collapsed;
      g.appendChild(gt);
    }
  }
  links.appendChild(g);
  return g;
}

/* THE BADGES, as a list, drawn on a LINE OF THEIR OWN under the name.

   THEY USED TO RIDE THE NAME'S OWN TEXT RUN — inside the clip that stops a
   64-byte name somebody else chose from reaching the plot — so the clip cut the
   badges off too. Measured on the running rig: "NO RECORDED ANCESTRY" painted as
   "NO R", "THE RECORD BEGINS HERE · 31 GENERATIONS ABOVE" was invisible in its
   entirety, and every "· extinct here · n living lines below" lost its tail. A
   label a reader cannot read is worse than no label, because the row then looks
   unremarkable rather than unexplained.

   So the name keeps the clip and the badges get their own run, their own clip
   with the whole label region to spread across, and their own 13 pixels of row —
   which a row carrying no badge is not charged for.

   EVERY ENTRY IS STATIC WORDS AND INTEGERS. No name enters this list, which is
   what lets it be drawn outside the name's clip at all, and it is the same
   structural property the root badge always had rather than a new promise. */
function lfBadges(n, joined){
  var out = [];
  if (!n.alive){
    out.push({t: "· extinct here · " + n.leaves + " living lines below", c: "meta"});
  } else {
    var by = (n.excludedBy && n.excludedBy.length) ? " S" + n.excludedBy.join(" S") : "";
    // SEED STOCK SUBSUMES THE EXCLUSION BADGE rather than sitting beside it:
    // "excluded somewhere" is the weaker half of what this row already says, and
    // printing both would read as two findings about one policy.
    if (n.seedStock){
      out.push({t: "SEED STOCK · NEVER EXPORTED" + by, c: "tbadge warn", term: "seedstock"});
    } else if (n.excluded){
      out.push({t: "NEVER EXPORTED" + by, c: "tbadge warn", term: "excluded"});
    }
    if (n.everywhere) out.push({t: "EVERYWHERE", c: "tbadge live", term: "everywhere"});
    if (n.endemic) out.push({t: "ENDEMIC", c: "tbadge lane", term: "endemic"});
  }
  if (n.isolated){
    // The two reasons, never conflated — and the label a reader sees is the one
    // that is true of THIS species.
    out.push({t: n.ancestryKnown ? "NO LIVING RELATIVE" : "NO RECORDED ANCESTRY",
              c: "tbadge warn", term: n.ancestryKnown ? "genealogy" : "noancestry"});
  } else if (n.alive && n.leaves > 1){
    out.push({t: "ALSO AN ANCESTOR", c: "tbadge lane"});
  }
  // A ROOT WITH ANCESTORS ABOVE IT. Nothing is drawn above this row, and without
  // a label that reads as "the family starts here" — which for most roots on this
  // map is false by dozens of generations. The record traces them; the reduction
  // collapsed them because not one has a living branch.
  if (joined && !n.parent && n.ancestryKnown && n.ancestryDepth > 0){
    out.push({t: "THE RECORD BEGINS HERE · " + n.ancestryDepth +
      (n.ancestryDepth === 1 ? " GENERATION ABOVE" : " GENERATIONS ABOVE"),
      c: "tbadge rec", term: "recordfloor"});
  }
  return out;
}
function lfBadgeH(n, joined){ return lfBadges(n, joined).length ? LF_BADGEH : 0; }

/* One row's whole height: the row itself, the badge line when it has one, and the
   detail when it is open. It is one function so the height the layout reserves
   and the marks the drawing emits cannot disagree. */
function lfRowH(n, joined, open){
  return LF_ROWH + lfBadgeH(n, joined) + (open ? lfDetailHeight(n) : 0);
}

/* One row: the label column, the mini-map, the count, the trend and the bar. */
function lfRow(x, n, top, cols, sc, i, joined){
  var base = "lfrow" + (n.alive ? "" : " anc") + (lfOpenKey === n.key ? " open" : "");
  var g = svgEl("g", base);
  var tk = "lf" + i;
  // data-s registers the row's tooltip; the key is generated here and the NAME
  // never becomes part of a selector.
  SP[tk] = lfTip(n);
  g.setAttribute("data-s", tk);
  g.setAttribute("data-k", n.key);
  // A ROW IS A CONTROL, so it is one for the keyboard too: focusable, announced
  // by its <title>, opened with Enter or Space. Lighting its lineage happens on
  // focus exactly as it happens on hover, which is the point — the affordance
  // that answers "how is this one related" must not need a pointing device.
  g.setAttribute("tabindex", "0");
  g.setAttribute("role", "button");
  LFEL[n.key] = {g: g, base: base};
  // THE FULL NAME, AS THE ROW'S OWN TITLE. The label below is clipped — a name is
  // 64 bytes somebody else chose — and the promise that "the full name is always
  // in the row's tooltip" used to mean this page's hover tip and nothing a
  // browser, a keyboard or a screen reader could reach. An SVG <title> is that
  // promise kept: it is the accessible name of the group, and it is a TEXT NODE
  // on a created element like every other name in this region.
  var ttl = svgEl("title");
  ttl.textContent = String(n.name);
  g.appendChild(ttl);

  var open = lfOpenKey === n.key;
  var badges = lfBadges(n, joined);
  var badgeH = badges.length ? LF_BADGEH : 0;
  var hit = svgEl("rect", "hit");
  hit.setAttribute("x", "0");
  hit.setAttribute("y", String(top));
  hit.setAttribute("width", String(cols.w));
  hit.setAttribute("height", String(LF_ROWH - 1 + badgeH + (open ? lfDetailHeight(n) : 0)));
  g.appendChild(hit);

  // THE WHOLE ROW STEPS RIGHT WITH ITS DEPTH, glyph included. It used to be the
  // name alone, with every glyph in one column: that reads as a list with ragged
  // text, not as a tree, and it left the brackets nothing to end on.
  var indent = lfIndent(n, joined);
  if (n.alive){
    // The same creature glyph the map draws, in the same per-species colour, and
    // it is THE ONLY COLOURED THING ON THE ROW.
    var u = svgEl("use");
    u.setAttribute("href", "#bib");
    u.setAttribute("transform",
      "translate(" + (LF_GLYPHX + indent) + " " + (top + 12) + ") scale(0.85)");
    u.style.fill = speciesColor(n.key);
    g.appendChild(u);
  } else {
    var ring = svgEl("circle", "ring");
    ring.setAttribute("cx", String(LF_GLYPHX + indent));
    ring.setAttribute("cy", String(top + 12));
    ring.setAttribute("r", "4");
    g.appendChild(ring);
  }

  var text = svgEl("text", "nm");
  text.setAttribute("x", String(LF_NAMEX + indent));
  text.setAttribute("y", String(top + 16));
  text.setAttribute("clip-path", "url(#lfclip)");
  // THE RAW SPELLING, as its source holds it (contract-a.md §17 A36 for a census
  // name, §16 A34 for a record's). CSS keeps its spaces.
  trSpan(text, null, n.name);
  g.appendChild(text);

  // The badge line, under the name, in its own clip and its own run.
  if (badges.length){
    var bt = svgEl("text", "bdg");
    bt.setAttribute("x", String(LF_NAMEX + indent + LF_BADGEX));
    bt.setAttribute("y", String(top + LF_ROWH + 9));
    bt.setAttribute("clip-path", "url(#lfbclip)");
    for (var bi=0;bi<badges.length;bi++){
      var s = trSpan(bt, badges[bi].c || null, badges[bi].t, bi ? 10 : 0);
      if (badges[bi].term) s.setAttribute("data-t", badges[bi].term);
    }
    g.appendChild(bt);
  }

  lfMini(g, x, n, top, cols);

  if (n.alive){
    var pop = svgEl("text", "pop");
    pop.setAttribute("x", String(cols.pop));
    pop.setAttribute("y", String(top + 16));
    pop.setAttribute("text-anchor", "end");
    trSpan(pop, null, String(n.population));
    if (n.eggs) trSpan(pop, "eggs", "+" + n.eggs + "e", 3);
    g.appendChild(pop);
  }

  lfSpark(g, n, top, cols);
  lfBar(g, n, top, sc, i);
  if (open) lfDetail(g, n, top + LF_ROWH + badgeH, cols);
  return g;
}

function lfDetail(g, n, top, cols){
  var lines = lfDetailLines(n), i, j;
  var box = svgEl("rect", "detbg");
  box.setAttribute("x", "0");
  box.setAttribute("y", String(top - 2));
  box.setAttribute("width", String(cols.w));
  box.setAttribute("height", String(lines.length * LF_DETL + LF_DETPAD - 4));
  g.appendChild(box);
  for (i=0;i<lines.length;i++){
    var text = svgEl("text", "det");
    text.setAttribute("x", String(LF_NAMEX + 8));
    text.setAttribute("y", String(top + 11 + i * LF_DETL));
    for (j=0;j<lines[i].length;j++){
      var seg = lines[i][j];
      if (!seg.t) continue;
      var s = trSpan(text, seg.c || null, seg.t);
      if (seg.term) s.setAttribute("data-t", seg.term);
    }
    g.appendChild(text);
  }
}

/* The axis, the record's floor and the column headings — everything that is
   drawn once for the whole picture rather than per row. */
function lfAxis(x, cols, sc, height){
  var g = svgEl("g", "axis");
  var span = sc.t1 - sc.t0, step = lfStep(span);
  var first = Math.ceil(sc.t0 / step) * step;
  // The caption below shares the ticks' own row, right of everything and left of
  // the plot. capRight is where it ends, so the one tick label whose left overhang
  // would land on it can give way — see the anchor below. A number printed over
  // another number is two facts a reader can read as neither.
  var capRight = x.ancestrySinceMs && !(x.ancestrySinceMs > sc.t0) ? cols.plot - 6 : -1;
  for (var tms = first; tms <= sc.t1; tms += step){
    var px = sc.x(tms);
    var line = svgEl("line", "grid");
    line.setAttribute("x1", String(px)); line.setAttribute("x2", String(px));
    line.setAttribute("y1", String(LF_PADT - 16)); line.setAttribute("y2", String(height - 6));
    g.appendChild(line);
    var tlbl = step >= 86400000 ? trDay(tms).slice(5) : trClock(tms);
    var lbl = svgEl("text", "tick");
    lbl.setAttribute("x", String(px));
    lbl.setAttribute("y", String(LF_PADT - 22));
    // A label straddles its own grid line, EXCEPT where straddling it would reach
    // back over the floor's caption: then it stands to the right of the line
    // instead. Half its width is estimated from its length, which is exact enough
    // for a monospace face and needs no laid-out node to measure. THE DATE IS NOT
    // DROPPED — the leftmost tick is the one a reader looks at to find out where
    // the axis begins, and the line under it is still the moment itself.
    lbl.setAttribute("text-anchor",
      px - tlbl.length * 3 < capRight + 4 ? "start" : "middle");
    lbl.textContent = tlbl;
    g.appendChild(lbl);
  }
  // THE RECORD'S FLOOR, WHICH IS A FACT AND NOT A WIDTH.
  //
  // It used to be a boundary inside the picture, kept there by the server
  // clamping the axis down to it. Measured on the running rig that left about two
  // thirds of the plot empty and the empty share grew every hour — the floor is a
  // fixed date and the bars are not — so the axis is fitted to the oldest DRAWN
  // bar now (tree.go, SpanStartMs) and the floor is usually OLDER than the left
  // edge. That is the ordinary case, and its honest drawing is a CAPTION at the
  // left margin: the date, in the ticks' own quiet colour, in the row the ticks
  // occupy, reading into the axis it precedes. It carries the same glossary term
  // the badge and the stat line carry, so the fact is one fact in three places.
  //
  // AND WHERE IT REALLY IS INSIDE THE PICTURE it is still the boundary it was: a
  // species whose first crossing named no parent can predate the oldest crossing
  // that named one, and then everything left of the line is a stretch where no
  // family line in this drawing can begin. The test is a STRICT > for that
  // reason — at equality the line would sit exactly on the axis's left edge and
  // mark nothing, and the caption says the same thing in words.
  if (x.ancestrySinceMs && x.ancestrySinceMs > sc.t0){
    var fx = sc.x(x.ancestrySinceMs);
    var fl = svgEl("line", "floor");
    fl.setAttribute("x1", String(fx)); fl.setAttribute("x2", String(fx));
    fl.setAttribute("y1", String(LF_PADT - 16)); fl.setAttribute("y2", String(height - 6));
    g.appendChild(fl);
    var flt = svgEl("text", "floorlbl");
    flt.setAttribute("x", String(fx + 4));
    flt.setAttribute("y", String(LF_PADT - 6));
    flt.setAttribute("data-t", "recordfloor");
    flt.textContent = "ancestry recorded from here";
    g.appendChild(flt);
  } else if (x.ancestrySinceMs){
    var fcap = svgEl("text", "floorcap");
    fcap.setAttribute("x", String(capRight));
    fcap.setAttribute("y", String(LF_PADT - 22));
    fcap.setAttribute("text-anchor", "end");
    fcap.setAttribute("data-t", "recordfloor");
    fcap.textContent = "the record reaches back to " + trDay(x.ancestrySinceMs) + " ·";
    g.appendChild(fcap);
  }
  var now = svgEl("line", "nowline");
  now.setAttribute("x1", String(cols.plot + cols.plotw));
  now.setAttribute("x2", String(cols.plot + cols.plotw));
  now.setAttribute("y1", String(LF_PADT - 16)); now.setAttribute("y2", String(height - 6));
  g.appendChild(now);
  var nowt = svgEl("text", "tick");
  nowt.setAttribute("x", String(cols.plot + cols.plotw));
  nowt.setAttribute("y", String(LF_PADT - 22));
  nowt.setAttribute("text-anchor", "end");
  nowt.textContent = "now";
  g.appendChild(nowt);

  function head(px, label, anchor, term){
    var h = svgEl("text", "colhd");
    h.setAttribute("x", String(px));
    h.setAttribute("y", String(LF_PADT - 6));
    if (anchor) h.setAttribute("text-anchor", anchor);
    if (term) h.setAttribute("data-t", term);
    h.textContent = label;
    return g.appendChild(h);
  }
  head(LF_NAMEX, "species", null, "species");
  head(cols.mini, "where", null, "world");
  head(cols.pop, "alive", "end", "population");
  head(cols.spark, "24 h", null, "trend");
  // THE PLOT'S OWN HEADING, which the drawing went without: the ticks say WHICH
  // moments and nothing said that the direction is time at all. A first-time
  // reader looking at a row of bars has to be told that much before anything else
  // on this tab means what it says.
  head(cols.plot, "time, left to right", null, "timeaxis");
  return g;
}

/* ------------------------------------------- BRAINS OVER TIME, UNDER THE TREE

   WHAT IT IS. One panel, below the drawing, sharing the drawing's exact left and
   right edges and its tick positions and NOTHING else: its own y scales, its own
   box, its own answer from its own endpoint. It says how complicated the brains
   crossing this map have been getting, which is the one question the tree itself
   cannot answer — a tree draws WHO, and this draws WHAT THEY ARE MADE OF.

   WHY IT IS NOT A ROW IN THE TREE. An aggregate over every genome the archive
   could read is not a species. Given a row it would inherit a row's affordances —
   a name, a place in a family, a lineage that lights, a click that opens a detail
   about a creature — and every one of those would be a lie about what it is.

   WHY IT SHARES THE AXIS AND HOW. The tree's clock RE-FITS to what is drawn
   (tree.go: spanStartMs, spanStartSeedMs), so a series at a fixed resolution
   ending at now could not be laid against it. The panel is drawn from lfScale —
   the SAME function, the same cols, the same instance — and the series is fetched
   for the two edges that scale actually has, re-aggregated server-side onto them.
   Every x here is sc.x of a real millisecond, which is the property the geometry
   is tested on.

   THE TWO SERIES, AND WHY THEY ARE STACKED RATHER THAN OVERLAID. Median synapses
   per genome and median HIDDEN neurons — the count above the fixed 48 every brain
   is born with. They are the two that move: against time, the synapse median runs
   8.1 to 42.1 over this record and the hidden-neuron median 1.0 to 8.6, while the
   RAW neuron count runs 49.0 to 56.6 because 48 of it never varies at all. Two
   series in two units cannot honestly share one y scale — the smaller would be
   pressed flat — and putting two scales on one plot invites a reader to read
   meaning into where the lines cross, which would be meaning that is not there.
   So they are two small multiples, one above the other, each fitted to its own
   range, over one shared clock. Nothing crosses anything.

   EACH RANGE FITS THE SHADED VALUES IN THIS WINDOW. Starting every plot at zero
   compressed the changing part of a line into a few pixels. The lower edge is
   the smallest lower quartile and the upper edge is the largest upper quartile,
   so the complete band stays visible while changes in its median remain clear.
   Both edges are printed on the plot. A constant series has one centered value.

   THE BAND IS THE MIDDLE HALF, not the extremes, and that is a choice with a
   reason. A slice holds a few hundred genomes; its minimum hidden-neuron count is
   0 in almost every slice (some genome is at the floor) and its maximum is one
   mutant, so a min-to-max band would draw a flat floor and a spiky ceiling and
   would encode THE PRESENCE OF OUTLIERS rather than the spread of the population.
   The quartile band moves with the population, which is what makes a widening or
   a narrowing legible. The extremes are not lost: they are in the tooltip.

   COVERAGE IS ITS OWN STRIP, not an opacity ramp on the lines. The archive holds
   about 42% of a slice's genomes in that slice's first six hours and about 97%
   after five days — the fetch backlog draining backwards — so the WORST-covered
   part of this picture is its right-hand edge, which is the part a reader looks
   at hardest. An opacity ramp would therefore make the newest and least certain
   stretch the faintest thing on the panel, hiding the problem in the shape of
   admitting it, and it would conflate "less certain" with "smaller" on a plot
   whose whole subject is size. A strip states it instead, in its own row, with
   the numbers in the tooltip. */
var LFB = null, LFSCALE = null, LFCOLS = null;
/* WHAT THE HELD ANSWER WAS ASKED FOR, and what is being asked for right now —
   two questions, not two timestamps, because this panel re-asks on a change of
   QUESTION and never on the clock moving. lfBrainAsk explains the rule. */
var LFBFOR = null, LFBASK = null, LFBSEQ = 0, LFBT = 0, LFB_REASK = 200;
var LFB_PADT = 18, LFB_SERH = 46, LFB_GAP = 15, LFB_COVH = 11, LFB_PADB = 16;
/* THE TWO MARKS ON THE COVERAGE STRIP, AS ONE RELATION. The amber tick means
   "crossings here and not one of them read"; a filled column means "some of it
   was read". The filled column is therefore NEVER allowed to be shorter than the
   tick, and the only way to keep that true is to write the two heights down
   beside each other. lfbCovH is the rest of the argument. */
var LFB_COVNONE = 2, LFB_COVMIN = 3;
function lfBrainH(){
  return LFB_PADT + LFB_SERH*2 + LFB_GAP*2 + LFB_COVH + LFB_PADB;
}
/* A MINIMUM SIZE IS A FLOOR ON VISIBILITY, NOT A MEASUREMENT — and a floor
   applied AFTER the scale has already clamped a mark to the axis end pushes that
   mark straight back out past it, which is the one thing the clamp exists to
   prevent. So every minimum in this panel goes through here, and here does what
   the tree's own shortest bar does (lfBar): grow right if there is room, SLIDE
   LEFT if there is not. Nothing this returns can end right of xend, at any width.
   A mark past the now line would claim the record reaches into a future it
   cannot have. */
function lfbFloor(x0, x1, min, xend){
  if (x1 - x0 < min) x1 = x0 + min;
  if (x1 > xend){ x1 = xend; x0 = xend - min; }
  return [x0, x1];
}
/* The requested resolution: about one bucket per four pixels of plot, inside the
   endpoint's own bounds. Finer than the aggregate's five minutes buys nothing —
   the server cannot invent detail the fold never kept — and the answer says what
   resolution it actually holds. */
function lfBrainBuckets(cols){
  var n = Math.round((cols && cols.plotw ? cols.plotw : LF_PLOTW) / 4);
  if (n < 8) n = 8;
  if (n > 720) n = 720;
  return n;
}
function lfbY(top, h, v, minv, maxv){
  // One value is a level and not a range. Put it in the middle instead of
  // inventing a lower or upper bound that the answer did not publish.
  if (!(maxv > minv)) return top + h/2;
  var f = (v - minv) / (maxv - minv);
  if (f < 0) f = 0; else if (f > 1) f = 1;
  return top + h - f*h;
}
/* A RUN IS A STRETCH WITH A READING IN EVERY BUCKET. Runs are drawn and the
   spaces between them are not: a gap in the record is a break in the line, never
   a segment joining the two readings either side of it. Joining them would draw a
   straight ascent across the 24-hour outage this record holds and invite it to be
   read as a measurement. */
function lfbRuns(pts, medk){
  var runs = [], cur = null;
  for (var i=0;i<pts.length;i++){
    if (pts[i][medk] == null){ cur = null; continue; }
    if (!cur){ cur = []; runs.push(cur); }
    cur.push(pts[i]);
  }
  return runs;
}
function lfbSeries(g, pts, B, sc, top, keys, cls, minv, maxv){
  var runs = lfbRuns(pts || [], keys.med), r, i, p, x, d;
  var half = (B.bucketMs || 0) / 2;
  for (r=0;r<runs.length;r++){
    var run = runs[r];
    // The band first, under the line.
    d = "";
    for (i=0;i<run.length;i++){
      p = run[i];
      if (p[keys.hi] == null) { d = ""; break; }
      x = sc.x(p.tMs + half);
      d += (i ? "L" : "M") + x.toFixed(2) + "," +
        lfbY(top, LFB_SERH, p[keys.hi], minv, maxv).toFixed(2);
    }
    if (d){
      for (i=run.length-1;i>=0;i--){
        p = run[i];
        var lo = p[keys.lo] == null ? p[keys.med] : p[keys.lo];
        x = sc.x(p.tMs + half);
        d += "L" + x.toFixed(2) + "," +
          lfbY(top, LFB_SERH, lo, minv, maxv).toFixed(2);
      }
      var band = svgEl("path", cls + "band");
      band.setAttribute("d", d + "Z");
      g.appendChild(band);
    }
    // Then the median. A ONE-BUCKET RUN IS STILL DRAWN — as a two-pixel stub,
    // because a lone reading between two outages is a reading and a polyline of
    // one point renders as nothing at all.
    d = "";
    for (i=0;i<run.length;i++){
      p = run[i];
      x = sc.x(p.tMs + half);
      d += (i ? "L" : "M") + x.toFixed(2) + "," +
        lfbY(top, LFB_SERH, p[keys.med], minv, maxv).toFixed(2);
    }
    if (run.length === 1){
      // AND IT IS SLID LEFT RATHER THAN GROWN PAST THE AXIS. Two pixels is a
      // floor on visibility and not a measurement, so a lone reading in the
      // newest bucket — whose x is already clamped to the now line — must not
      // buy its visibility with two pixels of a future the record cannot have.
      p = run[0];
      var sy = lfbY(top, LFB_SERH, p[keys.med], minv, maxv).toFixed(2);
      var sx = lfbFloor(sc.x(p.tMs + half), sc.x(p.tMs + half), 2, sc.x(sc.t1));
      d = "M" + sx[0].toFixed(2) + "," + sy + "L" + sx[1].toFixed(2) + "," + sy;
    }
    var line = svgEl("path", cls);
    line.setAttribute("d", d);
    g.appendChild(line);
  }
}
/* THE HEIGHT OF A FILLED COVERAGE COLUMN, and why it is not the fraction times
   the strip. Two things were wrong with h = f × 11 on this record.

   AN ORDERING FAILURE, first. The "crossings here and not one read" tick is 2 px
   tall, so every slice under 18.2% drew a SHORTER mark for "we read some of it"
   than the mark for "we read none of it" — 21 of 23 slices on the live map —
   and the strip's own heights said the opposite of what the glossary teaches.
   Nothing but colour separated the two facts.

   A RESOLUTION FAILURE, second. Coverage here runs from under half a per cent to
   about 37%, because a genome is asked for after its crossing is recorded and the
   answers arrive over the following days. A linear map spends four of its eleven
   pixels on the whole range that occurs and seven on a range nothing reaches, so
   the difference between a slice read once and a slice a third read is a pixel.

   So the column STARTS ABOVE THE TICK and carries the fraction as its square
   root: 3 px for the first genome read, 11 px for all of them, and the thinly
   measured stretch this map lives in gets most of the room instead of a pixel of
   it. It stays monotone — more coverage is always a taller column — and full
   height still means all of it. The strip is for reading the shape; the exact
   number is in the tooltip, which is where a number belongs. */
function lfbCovH(f){
  if (!(f > 0)) return LFB_COVMIN;
  if (f > 1) f = 1;
  return LFB_COVMIN + Math.sqrt(f) * (LFB_COVH - LFB_COVMIN);
}
/* AND A COUNT THAT IS NOT ZERO NEVER PRINTS AS ZERO PER CENT. Math.round said
   "0% of them" about 3 of 23 slices that plainly had readings — which is exactly
   the claim the amber tick is reserved for, made in words about a slice that had
   just been drawn as measured — and the same rounding would call a slice missing
   three genomes in a thousand complete. Both ends are named in words instead, so
   the sentence can never contradict the mark above it. */
function lfbShare(n, seen){
  if (!(seen > 0) || !(n > 0)) return "none";
  if (n >= seen) return "100%";
  var f = 100 * n / seen;
  if (f < 0.5) return "less than 1%";
  if (f > 99.5) return "almost all";
  return Math.round(f) + "%";
}
/* The tooltip for one slice: what it is, what it rests on, and the numbers the
   drawing itself cannot carry. It names the sampling rule and the blindness of
   the missingness in words, because a reader who has just been shown a coverage
   of 43% deserves to be told in the same breath why the 43% is still a fair
   sample of the 100%. */
function lfbTip(p, B){
  var when = trClock(p.tMs), mins = Math.round((B.bucketMs||0)/60000);
  var body = when + "Z, the " + (mins >= 60
    ? (mins/60 >= 2 ? Math.round(mins/60) + " hours" : "hour")
    : mins + " minutes") + " starting there.\n";
  if (p.medSyn == null && p.medHid == null){
    body += p.seen
      ? "The record holds " + p.seen + " genome" + (p.seen === 1 ? "" : "s") +
        " crossing in it and this archive has never managed to read one of them, " +
        "so there is no line here. That is a gap and not a zero."
      : "No crossing at all was recorded in it — the map was down, or nothing " +
        "travelled. That is a gap and not a zero.";
    return {title: "brains over time", body: body};
  }
  body += "Median synapses " + p.medSyn + (p.loSyn != null
    ? " (middle half " + p.loSyn + "–" + p.hiSyn + ", range " + p.minSyn + "–" + p.maxSyn + ")" : "") + ".\n";
  body += "Median hidden neurons " + p.medHid + (p.loHid != null
    ? " (middle half " + p.loHid + "–" + p.hiHid + ", range " + p.minHid + "–" + p.maxHid + ")" : "") +
    " — above the fixed " + (B.neuronFloor || 48) + " every brain is born with.\n";
  body += "Measured over " + p.n + " distinct genome" + (p.n === 1 ? "" : "s") +
    ", each counted once however often that creature travelled";
  if (p.seen > 0){
    body += ", out of " + p.seen + " the record says crossed — " +
      lfbShare(p.n, p.seen) + " of them";
  }
  body += ". Which genomes this archive holds is decided by a queue ordered on the " +
    "genome's own fingerprint, so what is missing is missing for reasons that have " +
    "nothing to do with the creature: the most travelled species here is " +
    "over-represented among the held copies by about one part in a hundred.";
  if (p.binned) body += " One reading in this slice was outside the range this panel keeps in detail.";
  return {title: "brains over time", body: body};
}
/* HOW FAR THE DRAWN WINDOW MAY RUN PAST THE HELD ANSWER BEFORE IT MATTERS —
   one drawn bucket, and never less than the five minutes the fold itself keeps.
   It is one number because it settles two questions that must not be allowed to
   disagree: whether a stretch is drawn as unanswered, and whether the panel asks
   again. A start that has moved by less than a bucket cannot move a mark by a
   whole bucket, and the right-hand edge is NOW, which moves on every paint and
   is the sixty-second poll's business rather than a new question. */
function lfbTol(B){
  var b = (B && B.bucketMs) || 0;
  return b > 300000 ? b : 300000;
}
/* THE STRETCH NOTHING HAS BEEN ASKED ABOUT YET, which is not the same fact as
   "the record holds no crossing here" and must never be drawn with the strip's
   mark for it. The drawing's window moves — the seed stock revealed pulls the
   left edge back 92 hours on this map — and for as long as one request takes,
   more than half of this plot can be a question in flight. Left blank it reads
   as the one absence the strip reserves for a stretch where nothing crossed at
   all, over a stretch that in fact held 74 measured points; so it is drawn as
   itself instead. No held answer at all is the whole plot pending. */
function lfbPending(B, sc, plotL, plotR){
  if (!B || !B.points || !B.points.length) return [[plotL, plotR]];
  // An answer that does not overlap the drawing at all is the same state as no
  // answer: the whole plot, once, rather than two washes over each other.
  if (B.fromMs && B.toMs && (B.fromMs >= sc.t1 || B.toMs <= sc.t0)) return [[plotL, plotR]];
  var out = [], tol = lfbTol(B);
  if (B.fromMs && B.fromMs > sc.t0 + tol) out.push([plotL, sc.x(B.fromMs)]);
  if (B.toMs && B.toMs < sc.t1 - tol) out.push([sc.x(B.toMs), plotR]);
  return out;
}
/* AND THE SLICES THAT ACTUALLY FALL ON THIS CLOCK. A held answer whose window is
   wider than the drawing's — the seed stock hidden again, a narrower box — has
   slices off both ends, and sc.x clamps every one of them onto the plot edge:
   dozens of columns stacked in a pixel, each claiming a coverage for a moment
   that is not drawn. A slice that does not overlap what is drawn is not drawn. */
function lfbVisible(B, sc){
  var pts = (B && B.points) || [], out = [], bw = (B && B.bucketMs) || 0, i;
  for (i=0;i<pts.length;i++){
    if (pts[i].tMs + bw <= sc.t0 || pts[i].tMs >= sc.t1) continue;
    out.push(pts[i]);
  }
  return out;
}
/* The panel itself. It is handed the SAME cols and the SAME scale the tree above
   it was drawn with, which is what makes "shares the axis" a fact rather than an
   intention. */
function lfBrainPanel(x, cols, sc){
  var host = document.getElementById("lfbrain");
  if (!host) return;
  while (host.firstChild) host.removeChild(host.firstChild);
  // "lfbp", NOT "lfb": the brain RING's tooltips are keyed "lfb"+row and are
  // written by the paint that has just finished. A prefix that swallowed them
  // would empty every ring's numbers the instant this panel drew.
  for (var old in SP){ if (old.indexOf("lfbp") === 0) delete SP[old]; }
  var B = LFB;
  var height = lfBrainH();
  var svg = svgEl("svg", "brainp");
  svg.setAttribute("width", String(cols.w));
  svg.setAttribute("height", String(height));

  // THE SAME TICKS, from the same step over the same span, so a vertical line
  // through both pictures means one moment.
  var span = sc.t1 - sc.t0, step = lfStep(span);
  var first = Math.ceil(sc.t0 / step) * step, tms, px;
  for (tms = first; tms <= sc.t1; tms += step){
    px = sc.x(tms);
    var gl = svgEl("line", "grid");
    gl.setAttribute("x1", String(px)); gl.setAttribute("x2", String(px));
    gl.setAttribute("y1", "0"); gl.setAttribute("y2", String(height - LFB_PADB + 4));
    svg.appendChild(gl);
  }
  var nw = svgEl("line", "nowline");
  nw.setAttribute("x1", String(cols.plot + cols.plotw));
  nw.setAttribute("x2", String(cols.plot + cols.plotw));
  nw.setAttribute("y1", "0"); nw.setAttribute("y2", String(height - LFB_PADB + 4));
  svg.appendChild(nw);

  // ---- WAITING IS NOT ABSENCE, AND IT IS DRAWN. Every stretch of this plot the
  // held answer does not reach is washed, edged and captioned, and carries a
  // tooltip that says a request is in flight — because leaving it blank would
  // spend the strip's own mark for "nothing crossed here" on a question that has
  // simply not come back yet. It is drawn UNDER everything else: where there is
  // an answer there is no wash, so nothing ever sits on top of a measurement.
  var plotL = cols.plot, plotR = cols.plot + cols.plotw;
  var held = !!(B && B.points && B.points.length);
  var pend = lfbPending(B, sc, plotL, plotR), pi, pw;
  for (pi=0;pi<pend.length;pi++){
    pw = pend[pi][1] - pend[pi][0];
    if (!(pw > 0.5)) continue;
    var pr = svgEl("rect", "pending");
    pr.setAttribute("x", pend[pi][0].toFixed(2)); pr.setAttribute("y", "0");
    pr.setAttribute("width", pw.toFixed(2));
    pr.setAttribute("height", String(height - LFB_PADB + 4));
    svg.appendChild(pr);
    // The dashed edge is where the answer starts, so a reader can see the
    // boundary rather than infer it from a wash. A pending stretch that is the
    // WHOLE plot has no boundary to draw and gets none.
    if (held){
      var pe = svgEl("line", "pendedge");
      var ex = pend[pi][0] <= plotL + 0.01 ? pend[pi][1] : pend[pi][0];
      pe.setAttribute("x1", ex.toFixed(2)); pe.setAttribute("x2", ex.toFixed(2));
      pe.setAttribute("y1", "0"); pe.setAttribute("y2", String(height - LFB_PADB + 4));
      svg.appendChild(pe);
    }
    var ph = svgEl("rect", "hit");
    ph.setAttribute("x", pend[pi][0].toFixed(2)); ph.setAttribute("y", "0");
    ph.setAttribute("width", pw.toFixed(2)); ph.setAttribute("height", String(height));
    ph.setAttribute("data-s", "lfbpwait" + pi);
    SP["lfbpwait" + pi] = {title: "brains over time", body: held
      ? "Nothing has been measured for this stretch YET. The drawing's window " +
        "moved — the seed stock revealed, or the box resized across a bucket — " +
        "and the answer for the new window is still on its way. THIS IS NOT AN " +
        "EMPTY STRETCH: an empty one on the strip below means the record holds no " +
        "crossing at all there, which is a fact about the map, and this is a fact " +
        "about a request. What is here will appear when the answer lands."
      : "The first measurement for this window has not arrived yet. Nothing here " +
        "is a statement about the record: the panel is drawing its clock and " +
        "waiting for its answer."};
    svg.appendChild(ph);
    if (held && pw >= 90){
      var pl = svgEl("text", "pendlbl");
      pl.setAttribute("x", (pend[pi][0] + 4).toFixed(2));
      pl.setAttribute("y", String(height - LFB_PADB + 14));
      pl.setAttribute("data-t", "brainwaiting");
      pl.textContent = "measuring this stretch — not asked for yet, not empty";
      svg.appendChild(pl);
    }
  }

  if (!B || !B.points || !B.points.length){
    var w8 = svgEl("text", "none");
    w8.setAttribute("x", String(LF_NAMEX));
    w8.setAttribute("y", String(LFB_PADT + 14));
    w8.textContent = "brains over time: waiting for the measurement";
    svg.appendChild(w8);
    host.appendChild(svg);
    return;
  }

  var synTop = LFB_PADT, hidTop = LFB_PADT + LFB_SERH + LFB_GAP;
  var covTop = hidTop + LFB_SERH + LFB_GAP;
  var minSyn = typeof B.minSyn === "number" ? B.minSyn : 0;
  var maxSyn = typeof B.maxSyn === "number" ? B.maxSyn : minSyn;
  var minHid = typeof B.minHid === "number" ? B.minHid : 0;
  var maxHid = typeof B.maxHid === "number" ? B.maxHid : minHid;

  function baseline(top){
    var b = svgEl("line", "base");
    b.setAttribute("x1", String(cols.plot)); b.setAttribute("x2", String(cols.plot + cols.plotw));
    b.setAttribute("y1", String(top + LFB_SERH)); b.setAttribute("y2", String(top + LFB_SERH));
    svg.appendChild(b);
  }
  function label(top, cls, term, text, ymin, ymax){
    var lb = svgEl("text", "plbl " + cls);
    lb.setAttribute("x", String(LF_NAMEX));
    lb.setAttribute("y", String(top + 9));
    lb.setAttribute("data-t", term);
    lb.textContent = text;
    svg.appendChild(lb);
    var mx = svgEl("text", "yrange");
    mx.setAttribute("x", String(cols.plot - 6));
    mx.setAttribute("text-anchor", "end");
    mx.textContent = String(ymax);
    // A constant series has one value, not two equal endpoints. Put its only
    // label beside its middle line. A real range labels both visible edges.
    mx.setAttribute("y", String(ymax > ymin ? top + 8 : top + LFB_SERH/2 + 3));
    svg.appendChild(mx);
    if (ymax > ymin){
      var mn = svgEl("text", "yrange");
      mn.setAttribute("x", String(cols.plot - 6));
      mn.setAttribute("y", String(top + LFB_SERH));
      mn.setAttribute("text-anchor", "end");
      mn.textContent = String(ymin);
      svg.appendChild(mn);
    }
  }
  baseline(synTop); baseline(hidTop);
  // ONE SET OF SLICES FOR BOTH PICTURES: the lines, the strip and the tooltips
  // are all the slices that fall on this clock, so a slice cannot be a hole in
  // one of them and a mark in another.
  var pts = lfbVisible(B, sc);
  lfbSeries(svg, pts, B, sc, synTop, {med:"medSyn", lo:"loSyn", hi:"hiSyn"},
    "syn", minSyn, maxSyn);
  lfbSeries(svg, pts, B, sc, hidTop, {med:"medHid", lo:"loHid", hi:"hiHid"},
    "hid", minHid, maxHid);
  label(synTop, "syn", "braintrend", "median synapses per genome", minSyn, maxSyn);
  // THE FLOOR IS ON THE PANEL, not only in the glossary. A reader who sees
  // "neurons" and a line near the bottom has to be told, in the picture, that 48
  // of every count here is fixed and is not drawn — otherwise the second series
  // reads as a species with almost no brain.
  label(hidTop, "hid", "hiddenneurons",
    "median hidden neurons (every brain also has the same fixed " +
    (B.neuronFloor || 48) + ")", minHid, maxHid);

  // ---- THE COVERAGE STRIP.
  var cb = svgEl("line", "covbase");
  cb.setAttribute("x1", String(cols.plot)); cb.setAttribute("x2", String(cols.plot + cols.plotw));
  cb.setAttribute("y1", String(covTop + LFB_COVH)); cb.setAttribute("y2", String(covTop + LFB_COVH));
  svg.appendChild(cb);
  var clbl = svgEl("text", "plbl");
  clbl.setAttribute("x", String(LF_NAMEX));
  clbl.setAttribute("y", String(covTop + LFB_COVH));
  clbl.setAttribute("data-t", "braincoverage");
  clbl.textContent = "how much of it was measured (heights on a square-root scale)";
  svg.appendChild(clbl);

  // EVERY MINIMUM HERE GOES THROUGH lfbFloor. The newest column is a bucket the
  // scale has already clamped to the now line, so a width floored afterwards ran
  // up to a pixel past it — the same defect as the stub above, in the other
  // mark. The half pixel is the gap between neighbouring columns.
  var xend = sc.x(sc.t1), i, p, x0, x1, cx, hx;
  for (i=0;i<pts.length;i++){
    p = pts[i];
    x0 = sc.x(p.tMs); x1 = sc.x(p.tMs + B.bucketMs);
    cx = lfbFloor(x0, x1 - 0.5, 1, xend);
    if (p.n > 0 && p.seen > 0){
      var h = lfbCovH(p.n / p.seen);
      var bar = svgEl("rect", "cov");
      bar.setAttribute("x", cx[0].toFixed(2)); bar.setAttribute("y", (covTop + LFB_COVH - h).toFixed(2));
      bar.setAttribute("width", (cx[1] - cx[0]).toFixed(2)); bar.setAttribute("height", h.toFixed(2));
      svg.appendChild(bar);
    } else if (p.seen > 0){
      // CROSSINGS, AND NOT ONE OF THEM READ. A different absence from "nothing
      // crossed", and it gets a different mark rather than the same emptiness —
      // and a SHORTER one than any filled column, which is what lfbCovH's floor
      // is there to guarantee.
      var tick = svgEl("rect", "covnone");
      tick.setAttribute("x", cx[0].toFixed(2));
      tick.setAttribute("y", String(covTop + LFB_COVH - LFB_COVNONE));
      tick.setAttribute("width", (cx[1] - cx[0]).toFixed(2));
      tick.setAttribute("height", String(LFB_COVNONE));
      svg.appendChild(tick);
    }
    // One hit target per slice, over the whole panel, so a reader gets the
    // numbers from anywhere in the column rather than having to find the line.
    hx = lfbFloor(x0, x1, 1, xend);
    var hit = svgEl("rect", "hit");
    hit.setAttribute("x", hx[0].toFixed(2)); hit.setAttribute("y", "0");
    hit.setAttribute("width", (hx[1] - hx[0]).toFixed(2));
    hit.setAttribute("height", String(height));
    hit.setAttribute("data-s", "lfbp" + i);
    SP["lfbp" + i] = lfbTip(p, B);
    svg.appendChild(hit);
  }

  // ---- WHERE THE MEASUREMENT BEGINS, drawn the way the genealogy draws its own
  // floor. It is usually off the left edge and costs nothing; when a sidecar has
  // been lost it is recent, and then it is the most important mark on the panel.
  if (B.haveFromMs && B.haveFromMs > sc.t0 && B.haveFromMs < sc.t1){
    var hx = sc.x(B.haveFromMs);
    var hl = svgEl("line", "havemark");
    hl.setAttribute("x1", String(hx)); hl.setAttribute("x2", String(hx));
    hl.setAttribute("y1", "0"); hl.setAttribute("y2", String(height - LFB_PADB + 4));
    svg.appendChild(hl);
    var ht = svgEl("text", "havelbl");
    ht.setAttribute("x", String(hx + 4));
    ht.setAttribute("y", String(height - LFB_PADB + 14));
    ht.setAttribute("data-t", "braingap");
    ht.textContent = B.lost
      ? "brains measured from here (the earlier record was lost)"
      : "brains measured from here";
    svg.appendChild(ht);
  }
  host.appendChild(svg);
}

/* THE KEYBOARD KEEPS ITS PLACE ACROSS A PAINT — every paint, not only the one
   that opening a row causes. The paint that just finished replaced the element
   the focus was on with an equivalent one, so the focus goes back on the row with
   the same species key and its family is lit again, exactly as the reader left
   it. The caller's "mine" is renderLife's decision, taken before the tear-down,
   about whether the focus was in this drawing at all.

   WHEN THE ROW IS GONE. The drawn set genuinely turns over — a species stops
   being reported and its row is not in the next paint — and then there is no
   equivalent element to go back to. Restoring nothing drops the focus on <body>,
   which is the defect this fixes wearing a different hat: the reader is sent to
   the top of the document for having read this tab. Moving to some OTHER row
   would be worse still, because it would light another family and read as if the
   reader had walked there. So the focus lands on the drawing's own box: no row is
   focused, nothing is lit, and the reader's next Tab walks into the drawing at
   its first row instead of starting the page again. */
function lfRefocus(host, mine){
  lfPainting = false;
  if (!mine) return;
  var ent = LFEL[lfFocusKey];
  if (ent && ent.g.focus){
    ent.g.focus();
    // focus() is expected to fire focusin, which lights the line. The paint lights
    // it itself as well: a browser that considers the row focused already fires
    // nothing, and a focus ring on an unlit family is worse than one redundant
    // pass over a set the census bounds.
    lfLight(lfFocusKey);
    return;
  }
  lfFocusKey = null;
  lfDark();
  if (host.focus) host.focus();
}

/* renderLife paints the whole view. It rebuilds every poll, which is affordable
   because the node set is bounded by the census that produced it — at most 32
   species a world — and which keeps an expanded row's numbers as fresh as the
   row above it. */
function renderLife(x){
  LFX = x;
  // The pair index, before anything reads it: a link's geometry and a row's
  // tooltip both need the parent node, and a stale one would date a link against
  // a species that is no longer in the answer.
  LFBY = {};
  var all = (x && x.nodes) || [];
  for (var bi=0;bi<all.length;bi++) LFBY[all[bi].key] = all[bi];
  // THE ROWS ARE CHOSEN FIRST, because the counts line describes THIS drawing:
  // how many rows the seed filter took out of it, and how many seed rows are on
  // it anyway.
  var pick = lfRows(x), list = pick.list, i;
  // THE BRAIN RING'S SCALE, fitted to the rows this paint is about to draw and to
  // nothing else — so it follows a search, the seed reveal and the census the same
  // way the time axis does, and it is computed once rather than per row.
  LFBRAIN = lfBrainRange(list);
  lfCount(x);
  lfStats(x, pick.hid, pick.seed);
  var host = document.getElementById("lfbox");
  if (!host) return;
  // WHOSE FOCUS THIS IS, decided BEFORE the drawing is torn down. The paint may
  // put the focus back only if the paint is what took it: a reader who has tabbed
  // on to the search box, or clicked away entirely, must never be dragged back
  // into the picture by a poll they did not ask for. No activeElement to report is
  // read as "still here", which is the state this paint is about to rebuild.
  var ae = document.activeElement,
      mine = !!lfFocusKey && (!ae || ae === document.body || host.contains(ae));
  lfPainting = true;
  while (host.firstChild) host.removeChild(host.firstChild);
  // This view's tooltips are rebuilt with it. They share SP with the map's
  // species runs and are prefixed so neither can clear the other's entries.
  for (var old in SP){ if (old.indexOf("lf") === 0) delete SP[old]; }
  // And so is everything the lighting holds: the elements this paint is about to
  // replace are gone, and a stale entry would light a row that is no longer on
  // the screen.
  LFSVG = null; LFEL = {}; LFLN = {}; LFPAR = {}; LFKID = {}; LFJOINED = false;

  if (!x || !x.haveStatus){
    host.appendChild(el("div", "muted", "waiting for the map"));
    lfBrainClear();
    lfRefocus(host, mine);
    return;
  }
  if (!list.length){
    host.appendChild(el("div", "muted", lfQuery
      ? "no species matches that search"
      : "no world is reporting a species right now, so there is nothing to relate"));
    lfBrainClear();
    lfRefocus(host, mine);
    return;
  }

  var cols = lfCols(x);
  var tops = [], y = LF_PADT;
  for (i=0;i<list.length;i++){
    tops.push(y);
    y += lfRowH(list[i], pick.joined, lfOpenKey === list[i].key);
  }
  var height = y + 16;

  var svg = svgEl("svg", "life");
  svg.setAttribute("width", String(cols.w));
  svg.setAttribute("height", String(height));
  // ONE clip for every label, bounding X only: a name may be as long as its
  // author made it and still cannot reach the plot.
  var defs = svgEl("defs");
  var clip = svgEl("clipPath");
  clip.setAttribute("id", "lfclip");
  var cr = svgEl("rect");
  cr.setAttribute("x", String(LF_NAMEX - 4));
  cr.setAttribute("y", "0");
  cr.setAttribute("width", String(LF_NAMEW));
  cr.setAttribute("height", String(height));
  clip.appendChild(cr);
  defs.appendChild(clip);
  // A SECOND CLIP FOR THE BADGE LINE, wider: it has the whole label region to
  // spread across and still may not reach the plot.
  var bclip = svgEl("clipPath");
  bclip.setAttribute("id", "lfbclip");
  var br = svgEl("rect");
  br.setAttribute("x", String(LF_NAMEX - 4));
  br.setAttribute("y", "0");
  br.setAttribute("width", String(Math.max(60, cols.plot - LF_NAMEX - 10)));
  br.setAttribute("height", String(height));
  bclip.appendChild(br);
  defs.appendChild(bclip);
  svg.appendChild(defs);

  // THE AXIS FITS WHAT IS DRAWN. A revealed seed species stretches it to the
  // wider published edge; with none on the picture it stays fitted to the rows
  // that are.
  var sc = lfScale(x, cols, pick.seed > 0);
  svg.appendChild(lfAxis(x, cols, sc, height));

  var links = svgEl("g", "links");
  svg.appendChild(links);
  var rowY = {}, rowInd = {};
  for (i=0;i<list.length;i++){
    rowY[list[i].key] = tops[i] + LF_ROWH / 2 - 1;
    rowInd[list[i].key] = lfIndent(list[i], pick.joined);
  }
  if (pick.joined){
    for (i=0;i<list.length;i++){
      var n = list[i];
      // A LINK IS A LINK ONLY IF BOTH ENDS ARE ON THIS DRAWING. The parent of a
      // drawn row can be absent — a hidden seed species, or a row a search left
      // out — and then the child keeps its place and its indent and simply has
      // no bracket, which is what a root looks like anyway.
      if (!n.parent || rowY[n.parent] == null) continue;
      var lg = lfJoin(links, n, rowY, rowInd, sc);
      if (!lg) continue;
      LFLN[n.key] = {g: lg, base: "ln"};
      LFPAR[n.key] = n.parent;
      (LFKID[n.parent] = LFKID[n.parent] || []).push(n.key);
    }
  }
  for (i=0;i<list.length;i++){
    svg.appendChild(lfRow(x, list[i], tops[i], cols, sc, i, pick.joined));
  }
  host.appendChild(svg);
  LFSVG = svg;
  LFJOINED = pick.joined;
  // THE PANEL IS PAINTED FROM THE SAME cols AND THE SAME sc, in the same pass, so
  // it cannot be drawn against a clock the tree above it is no longer using. They
  // are also what the panel's own fetch asks for its window, which is why they are
  // recorded here rather than recomputed there.
  LFCOLS = cols; LFSCALE = sc;
  lfBrainPanel(x, cols, sc);
  // AND THE PAINT IS WHAT TRIGGERS THE PANEL'S FETCH. The window this panel asks
  // for does not exist until a drawing has been laid out, so the paint is the
  // only thing that can know the question has changed — a timer can only know
  // that time has passed.
  lfBrainAsk();
  lfRefocus(host, mine);
}

/* The panel when there is no drawing to sit under. It is emptied rather than
   left holding the last picture: a clock with no tree above it is a clock a
   reader has no way to read. */
function lfBrainClear(){
  var host = document.getElementById("lfbrain");
  if (!host) return;
  while (host.firstChild) host.removeChild(host.firstChild);
  for (var old in SP){ if (old.indexOf("lfbp") === 0) delete SP[old]; }
}

/* WHAT THIS PANEL IS ASKING, as an object rather than a moment: the window's
   START and the resolution. Those two are the whole of the question — the END is
   NOW and is the same on every paint no matter what is drawn. */
function lfBrainWant(sc, cols){
  return {t0: Math.round(sc.t0), buckets: lfBrainBuckets(cols)};
}
function lfBrainSame(req, want){
  return !!req && req.buckets === want.buckets &&
    Math.abs(req.t0 - want.t0) <= lfbTol(LFB);
}
/* THE RE-FETCH RULE, AND WHY IT IS NOT "THE WINDOW CHANGED".

   The drawing repaints about every two seconds and its right-hand edge IS now,
   so "the drawn window changed" is TRUE ON EVERY PAINT and would be a fetch every
   two seconds — a bounded read of the whole aggregate, thirty times a minute, to
   move one edge by a pixel. What makes it a different QUESTION is one of two
   things: the window's START moving, which happens when the seed stock is
   revealed (92 hours on this map), when the oldest drawn species changes, or
   when a search changes what is drawn; and the RESOLUTION changing, which
   happens when the box is resized across a bucket boundary. Both are events a
   reader caused. NOW ticking is not, and it is left to the sixty-second poll,
   which is what that poll is for.

   The tolerance on the start is lfbTol — one drawn bucket, floored at the fold's
   own five minutes — so a start that drifts by less than the resolution it would
   be drawn at does not re-ask; it is the SAME number the pending mark uses, so
   the panel can never be waiting for something it has decided not to ask for.

   AND IT IS DEBOUNCED, because a drag across a screen changes the bucket count
   by the hundred: the trailing timer is restarted by every paint that still
   wants a different answer and fires once the window has settled. Two hundred
   milliseconds against the sixty seconds this replaces, and against the ~60,070
   ms a cold load measured before the load-time call had a scale to ask with. */
function lfBrainAsk(){
  var sc = LFSCALE, cols = LFCOLS;
  if (!sc || !cols) return;
  var want = lfBrainWant(sc, cols);
  if (lfBrainSame(LFBFOR, want) || lfBrainSame(LFBASK, want)){
    if (LFBT){ clearTimeout(LFBT); LFBT = 0; }
    return;
  }
  if (LFBT) clearTimeout(LFBT);
  LFBT = setTimeout(function(){ LFBT = 0; tickBrains(); }, LFB_REASK);
}

/* ----------------------------------------------------------- the SETTINGS tab
   One card per world. Every value is what that world reported about its own
   configuration, and an absent one renders "?" — NEVER the value the game ships
   with, which for saveMinutes would claim a world is being saved when its timer
   may be off (contract-b-m4.md §19, B19). */
function setKV(card, term, label, valueNode){
  var row = el("div", "kv");
  row.appendChild(term ? termEl("span", term, label) : el("span", "muted", label));
  var v = el("span");
  v.appendChild(valueNode);
  row.appendChild(v);
  card.appendChild(row);
}

function txt(s){ return document.createTextNode(String(s)); }

function fmtBool(v, yes, no){
  if (v == null) return unkEl();
  return txt(v ? yes : no);
}

function renderSettings(d){
  // The map's own card first: a world's numbers are only readable against the
  // ceilings they are measured on, and the relay is the only party that knows
  // what those are.
  var mhost = document.getElementById("mapcard");
  if (mhost){
    while (mhost.firstChild) mhost.removeChild(mhost.firstChild);
    mhost.appendChild(relayCard(d || {}));
  }
  var host = document.getElementById("setcards");
  if (!host) return;
  while (host.firstChild) host.removeChild(host.firstChild);
  if (!d || !d.slots || !d.slots.length){
    host.appendChild(el("span", "muted", "no slots reserved yet"));
    return;
  }
  for (var i=0;i<d.slots.length;i++) host.appendChild(settingsCard(d.slots[i]));
}

function settingsCard(v){
  var card = el("div", "card");

  var hd = el("div", "cardhd");
  var left = el("span");
  left.appendChild(el("span", "slot", "slot " + v.slot));
  left.appendChild(txt(" "));
  // A peer id is another peer's chosen string; it is a text node here for the
  // same reason a species name is.
  left.appendChild(el("span", "peer", v.peerId));
  hd.appendChild(left);
  var state = el("span", v.live ? (v.modConnected ? "ok" : "bad") : "bad",
    v.live ? (v.modConnected ? "live" : "no game") : "dark " + ms(v.darkForMs));
  hd.appendChild(state);
  card.appendChild(hd);

  if (!v.statsKnown){
    // §10.1 rule 3: a block older than the freshness rule is history, not state,
    // and settings age with the block that carried them.
    var gap = el("div", "detline");
    gap.appendChild(unkEl("this world has reported nothing" +
      (v.statsAgeMs ? " for " + ms(v.statsAgeMs) : "") + " — every setting below is unknown"));
    card.appendChild(gap);
  }

  card.appendChild(el("div", "cardsub", "identity"));
  setKV(card, "modversion", "mod version", v.modVersion ? txt(v.modVersion) : unkEl());
  setKV(card, "contractaversion", "protocol", v.contractAVersion ? txt(v.contractAVersion) : unkEl());
  setKV(card, null, "game version", v.gameVersion ? txt(v.gameVersion) : unkEl());
  setKV(card, "simsize", "world size", v.simulationSize ? txt(v.simulationSize) : unkEl());
  var edges = el("span");
  if (v.exportEdges && v.exportEdges.length) edges.appendChild(txt(v.exportEdges.join(" ")));
  else edges.appendChild(unkEl("none declared"));
  setKV(card, "exportedges", "export edges", edges);

  card.appendChild(el("div", "cardsub", "running"));
  // Both halves of the speed, and the span behind the measured one, because the
  // card is where a reader comes to find out whether a number is trustworthy.
  var sp = speedText(v), ac = achievedText(v), spv = el("span");
  spv.appendChild(sp === "×?" ? unkEl("×?") : txt(sp));
  if (ac){
    spv.appendChild(txt(" → "));
    spv.appendChild(txt(ac));
    spv.appendChild(el("span", "muted", " measured over " + ms(v.achievedSpanMs)));
  } else if (v.statsKnown){
    spv.appendChild(el("span", "muted", " — delivered rate not measured yet"));
  }
  setKV(card, "speed", "speed", spv);
  var dep = paceDepthText(v), cap = paceRateText(v), pace = el("span");
  pace.appendChild(dep === "?" ? unkEl() : txt(dep));
  pace.appendChild(txt(" queued / cap "));
  pace.appendChild(cap === "?" ? unkEl() : txt(cap));
  setKV(card, "pace", "arrivals", pace);
  setKV(card, null, "burst", (v.statsKnown && v.inboundRateBurst != null)
    ? txt(fmtScale(v.inboundRateBurst)) : unkEl());
  setKV(card, "population", "population", (v.statsKnown && v.population != null)
    ? txt(v.population) : unkEl());
  setKV(card, "egg", "eggs", (v.statsKnown && v.eggCount != null) ? txt(v.eggCount) : unkEl());

  card.appendChild(el("div", "cardsub", "if the machine stops"));
  var save = el("span");
  if (v.saveMinutes == null) save.appendChild(unkEl());
  else if (v.saveMinutes === 0){
    // 0 IS A READING AND A LOUD ONE: the save timer is off, and it is the
    // explanation for a world that never shows a save receipt.
    save.appendChild(el("span", "bad", "OFF"));
  } else save.appendChild(txt("every " + fmtScale(v.saveMinutes) + " min"));
  setKV(card, "savepolicy", "saves", save);
  setKV(card, "savekeep", "saves kept", v.saveKeep == null ? unkEl() : txt(v.saveKeep));
  setKV(card, null, "saves on quit", fmtBool(v.saveOnQuit, "yes", "no"));
  setKV(card, "lastSave", "last save", (v.statsKnown && v.lastSave)
    ? txt(ms(v.lastSaveAgeMs) + " ago") : unkEl("none yet"));
  var wrap = el("span");
  if (v.worldWrapping == null) wrap.appendChild(unkEl());
  else if (v.worldWrapping) wrap.appendChild(txt("on"));
  else wrap.appendChild(el("span", "bad", "OFF — this world is not containing its own creatures"));
  setKV(card, "worldwrap", "world wrapping", wrap);

  card.appendChild(el("div", "cardsub", "never exported"));
  var exl = el("div", "exl");
  if (!v.migrationExcludeKnown){
    exl.appendChild(unkEl("this world has not told us"));
  } else if (!v.migrationExclude || !v.migrationExclude.length){
    // A PRESENT EMPTY LIST is a stronger, different statement than absence: the
    // policy is switched off, and this world exports every species it holds.
    exl.appendChild(el("span", "muted", "nothing — the exclusion policy is off"));
  } else {
    for (var i=0;i<v.migrationExclude.length;i++){
      // Another world's configured name, chosen by whoever set that world up.
      exl.appendChild(el("span", "chip exl", v.migrationExclude[i]));
    }
  }
  card.appendChild(exl);
  return card;
}

/* ------------------------------------------------- what the MAP is running with
   The relay's own two published values, off the same broadcast every world's
   stats ride (contract-b-m4.md §6.5; §22, B24 and B25). They are drawn on the
   settings tab because that is where this page keeps what something was TOLD to
   do rather than what it is doing — and beside the world cards on purpose: a
   world's message rate is only readable against the ceiling it is measured on.

   They are the one thing on this tab that is AUTHORITATIVE rather than reported.
   A world's card is that world's claim about itself; these are the relay's own
   configuration, published as the values it is RUNNING with, and this page never
   substitutes a default for one — the numbers below are knobs an operator may
   have turned, and a shipped default drawn in their place would be the only
   figure here nobody could check.

   THE TWO ABSENCES ARE DIFFERENT FACTS AND ARE DRAWN DIFFERENTLY. No table at
   all is a relay older than the table, which is UNKNOWN and never "no ceilings".
   No floor is the relay's real answer, which is that it admits every compatible
   version.

   It is built inside the fence and out of nodes, like every other card here:
   a published key is the relay's string, and the safe path is the only one. */
var LIMITS = [
  ["maxConnectionsPerPeer", "connections one world may hold"],
  ["maxConnectionsPerAddress", "connections from one machine"],
  ["maxFramesPerSecond", "messages a second"],
  ["maxFrameBytes", "biggest single message"],
  ["maxBytesPerSecond", "bytes a second"],
  ["maxClaimsPerMinute", "seat claims a minute"],
  ["maxGenomeRequestsPerMinute", "genome requests a minute"],
  ["maxSubscribers", "watchers like this page"]
];

/* A byte ceiling in the unit a person reads, beside the exact number a
   disconnection message quotes. Neither replaces the other. */
function limitValue(key, n){
  var v = el("span", null, String(n));
  if (key.indexOf("Bytes") >= 0 && n >= 1048576){
    var mib = n/1048576;
    v.appendChild(el("span", "lu",
      "  (" + (mib === Math.round(mib) ? String(mib) : mib.toFixed(1)) + " MiB)"));
  }
  return v;
}

function limitKV(card, label, key, valueNode){
  var row = el("div", "kv");
  // A ceiling this page has no plain-English name for is drawn as its wire name
  // alone: an invented label would be this page guessing at what it means.
  var k = el("span", "muted", label ? label + " " : "");
  k.appendChild(el("code", "lk", key));
  row.appendChild(k);
  var v = el("span");
  v.appendChild(valueNode);
  row.appendChild(v);
  card.appendChild(row);
}

function relayCard(d){
  var card = el("div", "card");
  var hd = el("div", "cardhd");
  var left = el("span");
  left.appendChild(el("span", "slot", "the map"));
  hd.appendChild(left);
  hd.appendChild(el("span", d.relayConnected ? "ok" : "bad",
    d.relayConnected ? "relay linked" : "relay link DOWN"));
  card.appendChild(hd);

  card.appendChild(el("div", "cardsub", "helpers this map admits"));
  var floor = el("span");
  if (d.minContractVersion) floor.appendChild(txt(d.minContractVersion));
  // ABSENT IS THE ANSWER, NOT A GAP: a map with no floor admits every compatible
  // version, and that is a decision rather than something nobody has said yet.
  else floor.appendChild(el("span", "muted", "no minimum — any compatible version"));
  setKV(card, "floor", "oldest version", floor);

  card.appendChild(el("div", "cardsub", "ceilings every world here is measured against"));
  var lim = d.limits, keys = lim ? Object.keys(lim) : [];
  if (!keys.length){
    var u = el("div", "detline");
    u.appendChild(unkEl("this map publishes no ceilings — its relay is older than the " +
      "published table. Unknown, and never “this map has no limits”"));
    card.appendChild(u);
    return card;
  }
  var seen = {};
  for (var i=0;i<LIMITS.length;i++){
    var k = LIMITS[i][0];
    seen[k] = true;
    // A table with a hole in it is not a table: a key this map did not publish is
    // an unknown ceiling, never the value this page ships knowing.
    limitKV(card, LIMITS[i][1], k,
      (lim[k] == null) ? unkEl() : limitValue(k, lim[k]));
  }
  // A relay may publish a ceiling this page has never heard of. Dropping it would
  // be this page deciding what the table contains, which is exactly the thing the
  // published table exists to stop.
  keys.sort();
  for (var j=0;j<keys.length;j++){
    if (seen[keys[j]]) continue;
    limitKV(card, "", keys[j], limitValue(keys[j], lim[keys[j]]));
  }
  var note = el("div", "detline",
    "these are the relay's own numbers, not this page's — every one is a knob its " +
    "operator can turn, and a world over one is disconnected on its own while the rest " +
    "of the map runs on");
  card.appendChild(note);
  return card;
}
/* ============================= SPECIES CENSUS — END ======================== */

/* Per-poll paint: only the things that actually change. Rebuilding the SVG every
   two seconds would restart every animation, and the flow would never be seen. */
function paintMap(d){
  for (var i=0;i<d.slots.length;i++){
    var v = d.slots[i];
    var pop = document.getElementById("cpop-"+v.slot);
    var note = document.getElementById("cnote-"+v.slot);
    if (!pop) continue;
    var n = (v.statsKnown && v.population!=null) ? v.population : null;
    if (n==null){ pop.textContent = "unknown"; pop.setAttribute("class","popnum unk"); }
    else { pop.textContent = String(n); pop.setAttribute("class","popnum"); }
    // The creature field, grouped by species, and the cell's tap text. Both are
    // built as nodes: see the fenced region above for why.
    var c = paintSpecies(v);
    paintChips(v);
    var ct = document.getElementById("ctitle-"+v.slot);
    if (ct) ct.textContent = cellTitle(v);
    var unit = (c && c.unit) ? c.unit : 1;
    var txt = "", cls = "note";
    if (!v.live && v.darkForMs != null){ txt = "dark for "+ms(v.darkForMs)+" — bypassed"; cls += " badt"; }
    else if (v.live && !v.modConnected){ txt = "no game attached"; cls += " badt"; }
    else if (!v.statsKnown){
      txt = "reported nothing" + (v.statsAgeMs ? " for "+ms(v.statsAgeMs) : ""); cls += " warnt";
    } else if (!v.speciesKnown){
      // §10.1: absent renders as unknown species — never "no species", never an
      // empty list, never a zero.
      txt = "species unknown"; cls += " warnt";
    } else if (v.speciesTruncated){
      // §10.1: a truncated census names the 32 most abundant and the page MUST
      // say the rest is unreported rather than present it as the whole list.
      txt = "32 species · rest unreported"; cls += " warnt";
    } else if (!v.species || !v.species.length){
      txt = "reporting: nothing alive"; cls += " warnt";
    } else if (v.lastSave){
      txt = v.species.length+" species · saved "+ms(v.lastSaveAgeMs);
    } else { txt = v.species.length+" species · no save yet"; cls += " warnt"; }
    // The scale hint is the first thing to go: it is a nicety, and the line it
    // shares is carrying a rule the page MUST state.
    if (unit > 1 && txt.length <= 22) txt += " · 1×"+unit;
    if (v.lastRefusal){ txt = v.lastRefusal; cls = "note badt"; }
    // The cell is 200 units wide; anything longer belongs in the tooltip and the
    // table below, not spilling over the world next door.
    if (txt.length > 29) txt = txt.slice(0,28)+"…";
    note.textContent = txt;
    note.setAttribute("class", cls);
  }

  for (i=0;i<d.lanes.length;i++){
    var l = d.lanes[i], key = laneKey(l);
    var lab = document.getElementById("ll-"+key);
    if (lab){
      // The lane's rate is a NUMBER and only a number. It used to also be a
      // stream of ambient dots walking the arrow, and that was wallpaper: it
      // moved constantly, it said nothing the label did not say, and beside the
      // hop glyphs — which are real, individual creatures — it invited the
      // reader to count animation as traffic. The dots are gone; the
      // measurement they paced is right here.
      if (!l.open) lab.textContent = "closed: "+l.reason;
      else if ((l.skipped||[]).length) lab.textContent = "bypass ×"+l.skipped.length
        + " · " + rate(l.perMinute) + "/min";
      else lab.textContent = rate(l.perMinute)+"/min";
    }
    // POLL-DIFFERENCING IS THE FALLBACK, and it is only a fallback. When
    // /api/hops is answering, that arrival is already travelling the lane as its
    // own species glyph (§17, B14) and flashing the cell here too would say one
    // crossing twice. When the feed is unreachable, this is all that is left of
    // "something arrived" — a pure opacity fade at the destination, which needs
    // no frame loop and is therefore the same signal under reduced motion.
    var was = prevMig[key];
    if (was != null && l.migrations > was && !hopFeedOK) flashCell(l.toSlot);
    prevMig[key] = l.migrations;
  }
}

function flashCell(slot){
  var g = document.getElementById("c-"+slot);
  if (!g) return;
  g.classList.remove("hit");
  void g.getBoundingClientRect();
  g.classList.add("hit");
  setTimeout(function(){ g.classList.remove("hit"); }, 850);
}

/* ---------------------------------------------------------- the hop animation
   contract-b-m4.md §17, B14 (D19). /api/hops is a bounded feed of the last
   minute of crossings — lane, endpoints, timestamp and the species block the
   envelope carried. Every entry the page has not seen before sends THAT
   SPECIES' OWN GLYPH down that lane, from the source world to the destination.

   Four rules, three inherited and one this loop adds:

     NEUTRAL WHEN UNKNOWN. An envelope with no species block travels as the grey
     glyph. Never a guessed name, never omitted, and never "unknown" rendered as
     though it were a species value.

     NAMES ARE UNTRUSTED TEXT. The name reaches the DOM through textContent on a
     <title> built with createElementNS and through nothing else; see the fenced
     region above, where hopName and buildHopGlyph live for that reason.

     LEDGER, NOT CENSUS. A hop glyph says "this one crossed". The glyphs inside a
     cell say "these live here". Two facts, two sources, never summed.

     AND BOUNDED, WHICH IS THIS LOOP'S OWN. A dam releasing hundreds of arrivals
     must not put hundreds of creatures on the map: at most HOPMAX travel at
     once, they launch on a stagger, the pending queue is capped at HOPQMAX and
     the rest are simply not drawn. The feed is a sample of what is happening,
     never an obligation to draw all of it.

   REDUCED MOTION DEGRADES THIS, IT DOES NOT DELETE IT. See stillHop: the same
   creature in the same colour is shown arriving, it just does not walk there. */
var HOPMS = 1700, HOPMAX = 12, HOPSTAGGER = 130, HOPQMAX = 40, HOPSEENMAX = 800;
/* The reduced-motion form is bounded on the same axes: how long one stays up,
   and how many may be up at once. A burst must not stack forty glyphs on one
   cell edge either. */
var HOPSTILLMS = 1100, HOPSTILLMAX = 8, hopStills = 0;

/* A bounded seen-set. A migrationId is animated once, ever, and the feed
   re-serves the same 60 seconds on every poll — so without this the same
   creature would set off twice a second for a minute. */
function hopMarkSeen(id){
  if (!id) return true;
  if (hopSeen[id]) return true;
  hopSeen[id] = true;
  hopSeenQ.push(id);
  if (hopSeenQ.length > HOPSEENMAX) delete hopSeen[hopSeenQ.shift()];
  return false;
}

function onHops(f){
  var list = (f && f.hops) || [], i;
  // The FIRST poll only PRIMES the seen-set. A page opened at 04:12 must not
  // replay the previous minute all at once as though it were happening now.
  if (!hopsPrimed){
    for (i=0;i<list.length;i++) hopMarkSeen(list[i].migrationId);
    hopsPrimed = true;
    return;
  }
  for (i=0;i<list.length;i++){
    var hp = list[i];
    if (hopMarkSeen(hp.migrationId)) continue;
    // Lanes are keyed fromSlot+edge, which is what makes the two directions of a
    // pair two things to animate. A hop whose lane is closed or not drawn — it
    // crossed, then the map changed — is dropped rather than guessed onto
    // another arrow.
    var a = animFor(hp.fromSlot + hp.exitEdge);
    if (!a) continue;
    // Reduced motion takes the OTHER form of the same event, and takes it here
    // rather than by dropping the hop on the floor. The queue and its cap belong
    // to the travelling form alone: a still glyph is placed at once and has its
    // own bound.
    if (reduced){ stillHop(a, hopName(hp), hp.toSlot); continue; }
    if (hopQ.length >= HOPQMAX) continue;
    hopQ.push({a:a, name:hopName(hp), to:hp.toSlot});
  }
}

/* killHops drops every travelling glyph. buildMap throws the whole SVG away, so
   every path a live hop is walking stops existing at that instant. */
function killHops(){
  for (var i=0;i<hopsLive.length;i++){
    if (hopsLive[i].el.parentNode) hopsLive[i].el.parentNode.removeChild(hopsLive[i].el);
  }
  hopsLive = []; hopQ = [];
}

function launchHop(q){
  if (!hopLayer) return;
  var g = buildHopGlyph(q.name, q.to);
  hopLayer.appendChild(g);
  // Speed is set from the WHOLE lane's length, so every hop takes about HOPMS
  // whatever the lane's length — a short hop and a long wrap read as one event.
  hopsLive.push({el:g, a:q.a, ti:0, dist:0, speed:q.a.total/HOPMS, to:q.to});
}

/* stillHop is the reduced-motion arrival, and it is the whole of this fix's
   first half. It puts the glyph exactly where a travelling one would have
   ENDED — the last point of that lane's own path, at the edge of the world it
   reached — so the two forms agree about where a crossing lands, and then lets
   CSS fade it. No travel, no rotation, and no frame loop is needed to run it.

   Reading the endpoint off the lane path rather than off the cell geometry is
   deliberate: a bypass and a wrap end somewhere a cell's own coordinates do not
   predict, and the arrow the reader is looking at is the authority for where
   its traffic arrives. */
function stillHop(a, name, to){
  if (!hopLayer || hopStills >= HOPSTILLMAX) return;
  var tr = a.tracks.length ? a.tracks[a.tracks.length-1] : null;
  if (!tr) return;
  var pt = tr.el.getPointAtLength(tr.len);
  var g = buildHopGlyph(name, to);
  g.setAttribute("class", "hopg hopstill");
  g.setAttribute("transform", "translate("+pt.x.toFixed(1)+","+pt.y.toFixed(1)+")");
  hopLayer.appendChild(g);
  hopStills++;
  setTimeout(function(){
    hopStills--;
    if (g.parentNode) g.parentNode.removeChild(g);
  }, HOPSTILLMS);
  // The destination cell says so too, with the same fade the travelling form
  // fires on arrival.
  flashCell(to);
}

/* stepHops advances every travelling glyph one frame, along its lane's own
   track list. It ROTATES to the path's tangent, because the glyph
   faces east by construction and a west-bound creature that faces east looks
   like a drawing bug rather than a direction. */
function stepHops(ts, dt){
  if (hopQ.length && ts >= nextHopAt && hopsLive.length < HOPMAX){
    launchHop(hopQ.shift());
    nextHopAt = ts + HOPSTAGGER;
  }
  for (var i=hopsLive.length-1;i>=0;i--){
    var q = hopsLive[i];
    q.dist += q.speed*dt;
    var tr = q.a.tracks[q.ti];
    while (tr && q.dist > tr.len){ q.dist -= tr.len; q.ti++; tr = q.a.tracks[q.ti]; }
    if (!tr){
      if (q.el.parentNode) q.el.parentNode.removeChild(q.el);
      hopsLive.splice(i,1);
      // It ARRIVED. The destination cell says so, which is the same flash the
      // poll-differencing fallback fires.
      flashCell(q.to);
      continue;
    }
    var pt = tr.el.getPointAtLength(q.dist);
    var nx = tr.el.getPointAtLength(Math.min(q.dist + 2, tr.len));
    var ang = Math.atan2(nx.y - pt.y, nx.x - pt.x) * 180 / Math.PI;
    q.el.setAttribute("transform", "translate(" + pt.x.toFixed(1) + "," + pt.y.toFixed(1)
      + ") rotate(" + ang.toFixed(1) + ")");
  }
}

/* THE FRAME LOOP MOVES ONE KIND OF THING, AND EVERY ONE OF THEM IS A REAL
   CREATURE THAT REALLY CROSSED. It used to also walk an ambient dot down every
   open lane at that lane's measured rate — decorative traffic, indistinguishable
   at a glance from the hop glyphs beside it, and saying nothing the lane's own
   "/min" label did not already say in a number a reader can compare. It is
   gone. What moves on this map is evidence. */
var lastTs = 0;
function frame(ts){
  rafId = requestAnimationFrame(frame);
  if (document.hidden) { lastTs = ts; return; }
  var dt = lastTs ? Math.min(ts - lastTs, 80) : 16;
  lastTs = ts;
  stepHops(ts, dt);
}

/* ------------------------------------------------------------- the sparklines */
function valueRange(points){
  var min = null, max = null;
  for (var i=0;i<points.length;i++) if (points[i].v != null){
    if (min == null || points[i].v < min) min = points[i].v;
    if (max == null || points[i].v > max) max = points[i].v;
  }
  return {known:min != null, min:min == null ? 0 : min, max:max == null ? 0 : max};
}
function sparkPath(points, min, max, w, h){
  var line = "", area = "", open = false, n = points.length, last = null;
  for (var i=0;i<n;i++){
    var p = points[i];
    if (p.v == null){
      if (open){ area += " L "+xAt(i-1,n,w)+" "+h+" Z"; open = false; }
      continue;
    }
    var x = xAt(i,n,w), y = max>min
      ? h - 3 - ((p.v-min)/(max-min))*(h-7) : h/2;
    if (!open){ line += " M "+x.toFixed(1)+" "+y.toFixed(1);
                area += " M "+x.toFixed(1)+" "+h+" L "+x.toFixed(1)+" "+y.toFixed(1); open = true; }
    else { line += " L "+x.toFixed(1)+" "+y.toFixed(1); area += " L "+x.toFixed(1)+" "+y.toFixed(1); }
    last = {x:x, y:y};
  }
  if (open) area += " L "+(last?last.x:0)+" "+h+" Z";
  return {line:line.trim(), area:area.trim(), last:last};
}
function xAt(i,n,w){ return n<2 ? w/2 : (i/(n-1))*w; }

function darkBands(points, w, h){
  var out = "", n = points.length;
  for (var i=0;i<n;i++){
    if (!points[i].dark) continue;
    var x0 = xAt(i,n,w) - (w/Math.max(n-1,1))/2;
    out += '<rect class="sdark" x="'+Math.max(0,x0).toFixed(1)+'" y="0" width="'
      + (w/Math.max(n-1,1)).toFixed(1)+'" height="'+h+'"/>';
  }
  return out;
}

function sparkCard(title, sub, valueTxt, points, min, max, opts){
  var W = 220, H = 54;
  var dead = opts && opts.dead;
  var g = sparkPath(points, min, max, W, H);
  var body = darkBands(points, W, H);
  if (opts && opts.bars){
    var n = points.length, bw = Math.max(1.2, W/n - 0.8);
    for (var i=0;i<n;i++){
      var p = points[i];
      if (p.v == null || p.v <= 0) continue;
      var bh = max>0 ? Math.max(1, (p.v/max)*(H-6)) : 1;
      body += '<rect class="sbar" x="'+((i/n)*W).toFixed(1)+'" y="'+(H-bh).toFixed(1)
        + '" width="'+bw.toFixed(1)+'" height="'+bh.toFixed(1)+'"/>';
    }
  } else {
    body += '<path class="sarea'+(dead?" deadarea":"")+'" d="'+g.area+'"/>'
      + '<path class="sline'+(dead?" deadline":"")+'" d="'+g.line+'"/>';
    if (g.last) body += '<circle class="sdotend'+(dead?" deadend":"")+'" cx="'+g.last.x.toFixed(1)
      + '" cy="'+g.last.y.toFixed(1)+'" r="2.6"/>';
  }
  return '<div class="spark'+(opts&&opts.wide?" wide":"")+'">'
    + '<div class="sparkhd"><span>'+title+'</span><b>'+valueTxt+'</b></div>'
    + '<svg viewBox="0 0 '+W+' '+H+'" aria-hidden="true">'
    + '<line class="sbase" x1="0" y1="'+(H-0.5)+'" x2="'+W+'" y2="'+(H-0.5)+'"/>'
    + body + '</svg>'
    + '<div class="sparkft"><span>'+esc(sub)+'</span><span>'
    + (opts&&opts.bars ? '0&ndash;'+max : opts&&opts.known===false ? 'unknown' : min+'&ndash;'+max)
    + '</span></div></div>';
}

function renderHistory(H){
  var box = $("#spark");
  if (!H || !H.slots){ box.innerHTML = '<span class="muted">no history yet</span>'; return; }
  var hours = Math.round((H.toMs - H.fromMs)/3600000);
  var span = hours >= 1 ? hours+"h" : Math.round((H.toMs-H.fromMs)/60000)+"m";
  var h = "";
  var scope = $("#historyscope");
  if (scope) scope.textContent = HISTORY_RANGE === "all"
    ? (H.truncated ? "available history; older samples were trimmed" : "all recorded history")
    : "the last 24 hours";

  var i, totRange = valueRange(H.total);
  var totLast = null;
  for (i=H.total.length-1;i>=0;i--) if (H.total[i].v != null){ totLast = H.total[i].v; break; }
  h += sparkCard(t("population","whole map"), "every world summed, "+span,
    totLast==null ? '<span class="unknown">unknown</span>' : totLast, H.total,
    totRange.min, totRange.max, {wide:true, known:totRange.known});

  var flowMax = 1, flowSum = 0;
  for (i=0;i<H.flow.length;i++) if (H.flow[i].v != null){
    if (H.flow[i].v > flowMax) flowMax = H.flow[i].v;
    flowSum += H.flow[i].v;
  }
  h += sparkCard(t("migration","migrations"), "per "+Math.round(H.bucketMs/60000)+" min, "+span,
    flowSum, H.flow, 0, flowMax, {bars:true, wide:true});

  for (i=0;i<H.slots.length;i++){
    var s = H.slots[i];
    var dead = s.points.length ? s.points[s.points.length-1].dark : false;
    var range = valueRange(s.points);
    h += sparkCard(t("slot","slot "+s.slot), (s.peerId||"")+" · "+span,
      s.last==null ? '<span class="unknown">unknown</span>' : s.last,
      s.points, range.min, range.max, {dead:dead, known:range.known});
  }
  if (H.truncated) h += '<div class="spark"><div class="sparkhd"><span>note</span></div>'
    + '<div class="sparkft">the sample file is longer than one read; older buckets are cut off'
    + '</div></div>';
  box.innerHTML = h;
}

/* How many reserved slots have stats but no census: the honest gap, counted, so
   an operator can see a rig go half-blind rather than discover it one cell at a
   time. A slot with no stats at all is already counted as unknown. */
function censusless(d){
  var n = 0;
  for (var i=0;i<d.slots.length;i++) if (d.slots[i].statsKnown && !d.slots[i].speciesKnown) n++;
  return n;
}

/* ------------------------------------------------------------ tabs and polling
   THREE VIEWS OVER ONE POLL. The status endpoint is fetched once every two
   seconds whatever tab is open, because the header's numbers are true on all
   three and because a second timer per tab would ask the archive for the same
   frame three times. What varies is only what is DRAWN from it: the map paints
   an SVG, the settings tab paints six cards, and the species tab hangs one
   extra request off the same tick because its ledger annotations cannot be
   derived from the status frame at all.

   The two map-only feeds — the hop feed and the history strip — are gated on
   the map being visible. A hidden panel has no laid-out geometry to animate
   along, and a crossing drawn into a box nobody is looking at is work spent to
   be invisible; the seen-set is re-primed on the way back so a tab switch never
   replays the last minute as though it were happening now. The species view's
   trend column is gated the same way and polled a great deal more slowly: it is
   the one fetch on this tab that reads the durable sample file, and it changes
   about once a minute.

   THE TAB IS IN THE URL HASH, which is what makes "#species" a link somebody
   can send and a reload land where the reader was. */
var TABS = ["map","species","settings"], TAB = "map", lastStatus = null;

function tabFromHash(){
  var h = (location.hash || "").replace(/^#/, "");
  // #tree was the genealogy's own tab before the two views merged. It is still a
  // link somebody sent, so it lands where the genealogy went rather than
  // silently on the map.
  if (h === "tree") return "species";
  return TABS.indexOf(h) >= 0 ? h : "map";
}

function showTab(name, push){
  if (TABS.indexOf(name) < 0) name = "map";
  TAB = name;
  hideTip();
  var i;
  for (i=0;i<TABS.length;i++){
    var p = document.getElementById("p-"+TABS[i]);
    if (p) p.hidden = (TABS[i] !== name);
  }
  var bs = document.querySelectorAll("#tabs .tab");
  for (i=0;i<bs.length;i++){
    var selected = bs[i].getAttribute("data-tab") === name;
    bs[i].setAttribute("aria-selected", selected ? "true" : "false");
    bs[i].setAttribute("tabindex", selected ? "0" : "-1");
  }
  document.title = "Bibites Multiverse — "
    + (name === "map" ? "Live Map" : name === "species" ? "Species & Lineages" : "World Settings");
  if (name === "map"){
    // An SVG in a hidden panel has no geometry to measure — getTotalLength on
    // its lane paths is what every label position and every hop animation is
    // built from — so the map is REBUILT when it comes back into view rather
    // than painted into a box nothing has laid out.
    mapSig = "";
    hopsPrimed = false;
  }
  if (lastStatus) render(lastStatus);
  // The species view is redrawn from the answer it already has so the tab is
  // never blank while its fetch is in flight, then refreshed. Its geometry needs
  // no laid-out box — every coordinate is computed, not measured — so unlike the
  // map it does not have to be rebuilt on becoming visible.
  if (name === "species"){ if (LFX) renderLife(LFX); tickLife(); tickTrends(); tickBrains(); }
  if (name === "map") tickHistory();
  if (push && location.hash !== "#"+name) location.hash = "#"+name;
}

(function wireTabs(){
  var bar = document.getElementById("tabs");
  if (bar) bar.addEventListener("click", function(ev){
    var b = ev.target.closest ? ev.target.closest(".tab") : null;
    if (b) showTab(b.getAttribute("data-tab"), true);
  });
  if (bar) bar.addEventListener("keydown", function(ev){
    var b = ev.target.closest ? ev.target.closest(".tab") : null;
    if (!b) return;
    var bs = Array.prototype.slice.call(bar.querySelectorAll(".tab"));
    var at = bs.indexOf(b), next = at;
    if (ev.key === "ArrowRight") next = (at + 1) % bs.length;
    else if (ev.key === "ArrowLeft") next = (at + bs.length - 1) % bs.length;
    else if (ev.key === "Home") next = 0;
    else if (ev.key === "End") next = bs.length - 1;
    else return;
    ev.preventDefault();
    bs[next].focus();
    showTab(bs[next].getAttribute("data-tab"), true);
  });
  window.addEventListener("hashchange", function(){
    var want = tabFromHash();
    if (want !== TAB) showTab(want, false);
  });
})();

/* ------------------------------------------------------ fullscreen live map
   The map is already one SVG with a viewBox, so fullscreen changes no map
   geometry and fetches no data. The fullscreen stage gives that SVG the whole
   viewport, removes the normal narrow-screen minimum width, and lets the SVG's
   preserveAspectRatio fit every world and lane inside the available rectangle.
   The button stays inside the fullscreen element so it remains available as an
   exit. Escape and the button both pass through the same state sync. */
function mapFullscreenElement(){
  return document.fullscreenElement || document.webkitFullscreenElement || null;
}

function syncMapFullscreen(){
  var stage = document.getElementById("mapstage");
  var button = document.getElementById("mapfull");
  if (!stage || !button) return;
  var active = mapFullscreenElement() === stage;
  stage.classList.toggle("isfullscreen", active);
  button.setAttribute("aria-pressed", active ? "true" : "false");
  button.setAttribute("aria-label", active
    ? "Exit fullscreen live map" : "Show the live map in fullscreen");
  var text = document.getElementById("mapfulltext");
  if (text) text.textContent = active ? "exit fullscreen" : "fullscreen";
}

function toggleMapFullscreen(){
  var stage = document.getElementById("mapstage");
  if (!stage) return;
  var action;
  if (mapFullscreenElement() === stage){
    action = document.exitFullscreen || document.webkitExitFullscreen;
    if (action) action = action.call(document);
  } else {
    action = stage.requestFullscreen || stage.webkitRequestFullscreen;
    if (action) action = action.call(stage);
  }
  // A rejected request must not leave the toggle claiming a state the browser
  // refused. Fullscreen failures are otherwise non-fatal: the map stays usable.
  if (action && action.catch) action.catch(syncMapFullscreen);
}

(function wireMapFullscreen(){
  var stage = document.getElementById("mapstage");
  var button = document.getElementById("mapfull");
  if (!stage || !button) return;
  if (!stage.requestFullscreen && !stage.webkitRequestFullscreen){
    button.hidden = true;
    return;
  }
  button.addEventListener("click", toggleMapFullscreen);
  document.addEventListener("fullscreenchange", syncMapFullscreen);
  document.addEventListener("webkitfullscreenchange", syncMapFullscreen);
  syncMapFullscreen();
})();

/* The species view's own controls. A click on a row opens its detail; the row
   also carries the tooltip, so opening one dismisses the tip that was over it
   rather than leaving it pointing at an element this redraw has replaced. */
function openLife(key){
  lfOpenKey = (lfOpenKey === key) ? null : key;
  hideTip();
  if (LFX) renderLife(LFX);
}

/* The seed-stock reveal. IT IS A REDRAW AND NOTHING ELSE: the rows are already in
   the answer this page is holding — the server marks them and sends them — so
   showing them costs one repaint and a wider axis, and never a request. Nothing
   is stored either: the state is a variable, and a reload is back to the default.
   That is deliberate rather than unfinished. A hidden row is the view's own
   opinion about one poll, not a preference worth outliving the page. */
function toggleSeed(){
  lfSeedShown = !lfSeedShown;
  hideTip();
  if (LFX) renderLife(LFX);
}

var lfResizeT = 0;
(function wireLife(){
  var box = document.getElementById("lfbox");
  function rowOf(t){ return t && t.closest ? t.closest(".lfrow") : null; }
  if (box) box.addEventListener("click", function(ev){
    var g = rowOf(ev.target);
    if (g) openLife(g.getAttribute("data-k"));
  });
  // ---- ONE LINEAGE, LIT. Delegated on the box, so it survives every repaint
  // without a listener being attached per row — and it costs one class change per
  // drawn row, on a set bounded by the census.
  if (box){
    box.addEventListener("mouseover", function(ev){
      var g = rowOf(ev.target);
      if (g) lfLight(g.getAttribute("data-k")); else lfRest();
    });
    box.addEventListener("mouseleave", lfRest);
    // THE KEYBOARD GETS THE SAME ANSWER. focusin/focusout bubble where focus and
    // blur do not, which is what lets one listener on the box serve every row.
    //
    // AND THE ROW THE FOCUS LANDS ON IS RECORDED HERE, not in the key that opens
    // one: every way in is the same way in — Tab, a click, the paint putting the
    // focus back — and the next paint is what needs to know.
    box.addEventListener("focusin", function(ev){
      var g = rowOf(ev.target);
      if (!g) return;
      lfFocusKey = g.getAttribute("data-k");
      lfLight(lfFocusKey);
    });
    // A REPAINT IS NOT A BLUR. Only the reader leaving the drawing clears the
    // keyboard's place; a row losing the focus because the paint took its element
    // away is the paint's doing and lfRefocus is already putting it back. The
    // second guard is the same event arriving after the removal instead of during
    // it, which browsers do not agree about.
    box.addEventListener("focusout", function(ev){
      var g = rowOf(ev.target);
      if (!g || lfPainting || g.isConnected === false) return;
      // Row to row is not leaving either: the focusin that follows relights, and
      // darkening in between only makes the drawing blink between two rows.
      if (ev.relatedTarget && rowOf(ev.relatedTarget)) return;
      lfFocusKey = null;
      lfDark();
    });
    box.addEventListener("keydown", function(ev){
      if (ev.key !== "Enter" && ev.key !== " " && ev.key !== "Spacebar") return;
      var g = rowOf(ev.target);
      if (!g) return;
      // Space scrolls a page and Enter submits things; a row that has taken the
      // focus has taken the key too.
      if (ev.preventDefault) ev.preventDefault();
      lfFocusKey = g.getAttribute("data-k");
      openLife(lfFocusKey);
    });
  }
  var q = document.getElementById("lfq");
  if (q) q.addEventListener("input", function(){
    lfQuery = q.value.toLowerCase();
    if (LFX) renderLife(LFX);
  });
  var s = document.getElementById("lfsort");
  if (s) s.addEventListener("change", function(){
    lfSort = s.value;
    if (LFX) renderLife(LFX);
  });
  // The reveal control is rebuilt with the stat line every poll, so the listener
  // is on the line rather than on the button.
  var st = document.getElementById("lfstat");
  if (st) st.addEventListener("click", function(ev){
    var b = ev.target.closest ? ev.target.closest(".seedbtn") : null;
    if (b) toggleSeed();
  });
  // TWO BOXES, ONE CLOCK. The drawing and the panel under it are separate boxes
  // only because the drawing scrolls vertically and the panel must not scroll away
  // with it. On a window too narrow for the drawing they can both scroll
  // sideways, and two clocks at two offsets would not be one clock — so the
  // panel's horizontal offset follows the drawing's. It is one-way: the drawing is
  // where a reader is working.
  var bp = document.getElementById("lfbrain");
  if (box && bp) box.addEventListener("scroll", function(){
    if (bp.scrollLeft !== box.scrollLeft) bp.scrollLeft = box.scrollLeft;
  }, {passive:true});
  // THE DRAWING IS AS WIDE AS ITS BOX, so a resize changes the geometry without
  // changing a single fact. It repaints from the answer already held — no poll is
  // pulled forward and none is needed — and it is coalesced, because a drag
  // across a screen fires this by the hundred.
  window.addEventListener("resize", function(){
    if (lfResizeT) clearTimeout(lfResizeT);
    lfResizeT = setTimeout(function(){
      lfResizeT = 0;
      if (LFX && TAB === "species") renderLife(LFX);
    }, 150);
  });
})();

function render(d){
  renderHeader(d);
  if (TAB === "map") renderMap(d);
  else if (TAB === "settings") renderSettings(d);
}

function renderHeader(d){
  $("#shape").innerHTML = d.haveStatus
    ? d.map.width+"×"+d.map.height+"  ·  "+d.slotCount+" "+t("slot","slots")+"  ·  "
      + d.holes.length+" "+t("hole","hole(s)")+"  ·  "+t("epoch","epoch")+" "+d.epoch
    : '<span class="unknown">the relay has broadcast no map yet</span>';
  $("#hpop").innerHTML = num(d.totals.population);
  $("#hmig").innerHTML = d.totals.migrations + ' <span class="muted">('
    + rate(d.totals.perMinute) + "/min)</span>";
  $("#link").innerHTML = d.relayConnected
    ? '<span class="ok">'+t("relay","relay")+' linked</span>'
    : '<span class="bad">'+t("relay","relay")+' link DOWN</span>';
  // §10.1's unknown rule, applied to the view itself: a frame this page cannot
  // date, or dated too long ago, is not state.
  var age = $("#age");
  if (!d.haveStatus){ age.innerHTML = '<span class="unknown">state unknown</span>'; }
  else if (d.statusAgeMs > 30000){
    age.innerHTML = '<span class="unknown">state '+ms(d.statusAgeMs)
      + " old — STALE, treat every number as unknown</span>";
  } else { age.innerHTML = '<span class="muted">state '+ms(d.statusAgeMs)+" old</span>"; }
}

function renderMap(d){
  // The cross-world join is rebuilt from every poll, before anything draws, so
  // "also in slot 5" is exactly as fresh as the map beside it. SP goes with it:
  // a species that left a world must not keep a tooltip.
  SP = {};
  buildCrossWorld(d);

  if (signature(d) !== mapSig) buildMap(d);
  paintMap(d);

  $("#lanes tbody").innerHTML = d.lanes.map(function(l){
    var skips = (l.skipped||[]).map(function(k){
      return k.slot==null ? "hole ("+k.position.col+","+k.position.row+")"
                          : "slot "+k.slot+": "+k.reason; }).join(", ");
    return "<tr><td>slot "+l.fromSlot+"</td><td>"+l.edge+"</td>"
      + "<td>"+(l.open ? "slot "+l.toSlot : "—")+"</td>"
      + '<td class="'+(l.open?"open":"closed")+'">'+(l.open?"open":"closed: "+l.reason)+"</td>"
      + '<td class="num">'+l.migrations+'</td>'
      + '<td class="num">'+l.perMinute.toFixed(2)+'</td>'
      + '<td class="skip">'+(skips||"—")+"</td></tr>";
  }).join("") || '<tr><td colspan="7" class="muted">no declared export edges yet</td></tr>';

  $("#worlds tbody").innerHTML = d.slots.map(function(v){
    var state = v.live ? (v.modConnected ? '<span class="ok">live</span>'
                                         : '<span class="bad">no game</span>')
                       : '<span class="bad">dark '+ms(v.darkForMs)+"</span>";
    var save = (v.statsKnown && v.lastSave) ? ms(v.lastSaveAgeMs)+" ago"
      : '<span class="unknown">unknown</span>';
    var note = v.lastRefusal ? '<span class="bad">'+esc(v.lastRefusal)+"</span>"
      : (v.statsKnown ? "" : '<span class="unknown">reported nothing'
          + (v.statsAgeMs? " for "+ms(v.statsAgeMs) : "")+"</span>");
    // The species cell is left EMPTY here and filled by paintSpeciesCell: this
    // string is assigned to innerHTML, and no census name may reach it.
    // Both settings columns render every unknown half as an unknown, never as a
    // zero and never as the shipped default: "0/?" is a world queueing nothing
    // behind a cap nobody has told us.
    // Both scales in one column, as on the map: the applied one the game
    // reports and the rate this archive measured. Both are numbers built here,
    // so neither can carry attacker-chosen text into this innerHTML.
    var sp = speedText(v), ac = achievedText(v);
    var speed = (sp === "×?" ? '<span class="unknown">×?</span>' : sp)
      + (ac ? ' <span class="muted">→</span> ' + ac : "");
    var dep = paceDepthText(v), cap = paceRateText(v);
    var pace = (dep === "?" ? '<span class="unknown">?</span>' : dep) + "/"
      + (cap === "?" ? '<span class="unknown">?</span>' : cap);
    return "<tr><td>"+v.slot+"</td><td>("+v.position.col+","+v.position.row+")</td>"
      + "<td>"+esc(v.peerId)+"</td><td>"+state+"</td>"
      + '<td class="num">'+speed+'</td>'
      + '<td class="num">'+num(v.population)+'</td>'
      + '<td class="spx" id="wsp-'+v.slot+'"></td>'
      + '<td class="num">'+num(v.custodyDepth)+'</td>'
      + '<td class="num">'+pace+'</td>'
      + '<td class="num">'+num(v.heldDepth)+'</td>'
      + '<td class="num">'+num(v.bouncedTimeoutTotal)+'</td>'
      + "<td>"+save+"</td><td>"+note+"</td></tr>";
  }).join("") || '<tr><td colspan="13" class="muted">no slots reserved yet</td></tr>';
  for (var si=0; si<d.slots.length; si++) paintSpeciesCell(d.slots[si]);

  var tt = d.totals;
  $("#totals").innerHTML =
      kv(t("live","live")+" / "+t("dark","dark")+" / "+t("hole","holes"),
         tt.liveSlots+" / "+tt.darkSlots+" / "+tt.holes)
    + kv(t("population","population")+" (known worlds)", num(tt.population))
    + kv(t("custodyDepth","custody depth"), num(tt.custodyDepth))
    + kv(t("pacedDepth","paced depth"), num(tt.pacedDepth))
    + kv(t("held","held depth"), num(tt.heldDepth))
    + kv(t("bounce","bounces a hold timeout caused"), num(tt.timeoutBounces))
    + kv("worlds reporting nothing", tt.unknownSlots)
    // Counted over the COMPARED spelling, and labelled as such: two worlds
    // spelling one species differently would otherwise read as two.
    + kv(t("census","species across the map"),
         Object.keys(XW).length + ' <span class="muted">(by compared spelling)</span>')
    + kv("worlds reporting no species", censusless(d) === 0
         ? "0" : '<span class="unknown">'+censusless(d)+"</span>")
    + kv(t("envelope","envelopes recorded"),
         tt.migrations+"  ("+tt.perMinute.toFixed(2)+"/min over the last "
         + Math.round((d.flowWindowMs||300000)/60000)+" min)")
    + kv(t("genomegap","genome gaps"), d.genomeGaps)
    + kv("ledger records", d.ledgerRecords)
    // Absent entirely unless a retention horizon is set, which is the default:
    // an absent row is how a reader knows nothing is being pruned, and 0 would
    // read as "a horizon that deletes nothing" instead. The ledger line beside
    // it is not decoration — it is the whole of what this feature does not do.
    + (d.genomeHorizonMs
         ? kv(t("horizon","genome retention horizon"),
              (d.genomeHorizonMs/86400000).toFixed(1)+" days"
              + ' <span class="muted">(the ledger is kept forever)</span>')
           + kv("genome copies pruned past it",
                (d.genomesEvicted||0)+" ("
                + ((d.genomesEvictedBytes||0)/1048576).toFixed(1)+" MiB)")
           + kv("gaps retired past it", d.genomeGapsExpired||0)
         : "")
    + (d.ledgerSkippedLines
         ? kv("ledger lines unreadable and skipped",
              '<span class="bad">'+d.ledgerSkippedLines+"</span>")
         : "");
}

async function tick(){
  try {
    var r = await fetch("api/status", {cache:"no-store"});
    lastStatus = await r.json();
    render(lastStatus);
  } catch(e){
    $("#link").innerHTML = '<span class="bad">status endpoint unreachable</span>';
  }
  // The species view rides the SAME cycle rather than a timer of its own, and
  // it is only asked for while its tab is open: it is derived from a ledger the
  // browser must never be handed, so it costs the archive a little work and is
  // worth nothing to a tab nobody is looking at.
  if (TAB === "species") await tickLife();
}

async function tickLife(){
  try {
    var r = await fetch("api/species/tree", {cache:"no-store"});
    renderLife(await r.json());
  } catch(e){
    var host = document.getElementById("lfbox");
    if (host && !host.firstChild){
      host.appendChild(el("div", "bad", "species endpoint unreachable"));
    }
  }
}

/* The trend column rides its OWN and much slower timer, and asks for ONE answer
   covering every living species. It is the only fetch on this page that reads
   the durable sample file for this tab, and it changes about once a minute — so
   polling it on the two-second cycle would be a bounded file read every two
   seconds for a line that has not moved. A failure leaves the trend column
   empty and every other thing on the row exactly as it was. */
async function tickTrends(){
  if (TAB !== "species") return;
  try {
    var r = await fetch("api/species/trends?hours=24&buckets=32", {cache:"no-store"});
    var T = await r.json();
    var byKey = {}, list = T.species || [];
    for (var i=0;i<list.length;i++) byKey[list[i].key] = list[i];
    LFTREND = byKey;
    LFTRENDMETA = {samples: T.samples, truncated: T.truncated, buckets: T.buckets,
                   fromMs: T.fromMs, toMs: T.toMs};
  } catch(e){
    LFTREND = null;
    LFTRENDMETA = null;
  }
  if (LFX && TAB === "species") renderLife(LFX);
}
/* The brain panel rides its own slow timer too, and for a third reason on top of
   the trend column's two: its window is the DRAWING'S OWN two edges, so it can
   only be asked for once the drawing has been laid out at least once. A failure
   leaves the panel holding whatever it last drew and every other thing on this
   tab exactly as it was.

   THE TIMER IS NOT THE TRIGGER, THOUGH — lfBrainAsk is, off the paint that lays
   the window out. This is the refresh for the one edge that moves on its own:
   NOW. What it must never be again is the FIRST ask, which is how a cold load
   spent sixty seconds saying "waiting for the measurement" against an answer
   that was one request away. */
async function tickBrains(){
  if (TAB !== "species") return;
  var sc = LFSCALE, cols = LFCOLS;
  if (!sc || !cols) return;
  var want = lfBrainWant(sc, cols);
  LFBASK = want;
  var mine = ++LFBSEQ;
  try {
    var r = await fetch("api/species/brains?from=" + want.t0 +
      "&to=" + Math.round(sc.t1) + "&buckets=" + want.buckets, {cache:"no-store"});
    var B = await r.json();
    // AN OVERTAKEN ANSWER IS DROPPED. A reply for a window the reader has already
    // left would put the panel back on the wrong clock and set off another
    // re-ask; the newest question is the only one worth an answer.
    if (mine !== LFBSEQ) return;
    LFB = B; LFBFOR = want;
  } catch(e){
    if (mine === LFBSEQ) LFBASK = null;
    return;
  }
  LFBASK = null;
  if (LFX && TAB === "species") renderLife(LFX);
}
/* The hop feed is polled SEPARATELY from the status view, which is the shape of
   B14's decision and not an accident: /api/status is what the archive
   serializes verbatim into its durable metrics file once a minute, and a
   per-organism feed does not belong in a file that is never rewritten. A page
   whose hop endpoint fails keeps its map and every number on it, and falls back
   to flashing the destination cell when the migration counters move. */
async function tickHops(){
  // Map-only: the glyphs travel the map's own lane paths, and a hidden panel
  // has none to travel. hopsPrimed is reset on the way back in, so returning to
  // the map never replays the last minute as if it were happening now.
  if (TAB !== "map") return;
  try {
    var r = await fetch("api/hops", {cache:"no-store"});
    onHops(await r.json());
    hopFeedOK = true;
  } catch(e){
    hopFeedOK = false;
  }
}
var HISTORY_RANGE = "all", HISTORY_SEQ = 0;
function setHistoryRange(range){
  if (range !== "all" && range !== "24h") return;
  HISTORY_RANGE = range;
  var buttons = document.querySelectorAll("#historyrange button");
  for (var i=0;i<buttons.length;i++) buttons[i].setAttribute("aria-pressed",
    buttons[i].getAttribute("data-history-range") === range ? "true" : "false");
  var scope = $("#historyscope");
  if (scope) scope.textContent = range === "all" ? "all recorded history" : "the last 24 hours";
  $("#spark").innerHTML = '<span class="muted">loading&hellip;</span>';
  tickHistory();
}
async function tickHistory(){
  if (TAB !== "map") return;
  var wanted = HISTORY_RANGE, mine = ++HISTORY_SEQ;
  try {
    var query = wanted === "all" ? "range=all&buckets=120" : "hours=24&buckets=120";
    var r = await fetch("api/history?"+query, {cache:"no-store"});
    if (!r.ok) throw new Error("history unavailable");
    var answer = await r.json();
    if (mine !== HISTORY_SEQ || wanted !== HISTORY_RANGE) return;
    renderHistory(answer);
  } catch(e){
    if (mine !== HISTORY_SEQ) return;
    $("#spark").innerHTML = '<span class="bad">history endpoint unreachable</span>';
  }
}

(function wireHistoryRange(){
  var box = document.getElementById("historyrange");
  if (box) box.addEventListener("click", function(ev){
    var button = ev.target.closest ? ev.target.closest("button") : null;
    if (button) setHistoryRange(button.getAttribute("data-history-range"));
  });
})();

/* ------------------------------------------------------ motion, and its switch
   This page had ONE rule for prefers-reduced-motion and it was the wrong one:
   it never polled the hop feed and never started the frame loop, so a reader
   whose system asks for reduced motion saw no crossings at all — no travelling
   creature, not even an arrival flash — and was told nothing about
   why. On Windows the setting behind that query is "Animation effects", which
   is off on a great many machines for reasons that have nothing to do with this
   page, and the failure was invisible: the map still drew, the numbers still
   moved, and the traffic simply never appeared.

   The rule now is DEGRADE, NEVER SUPPRESS. Reduced motion stops the TRAVEL —
   the frame loop and the glyph walking its lane — and keeps the FACT: the
   arriving creature appears at the end of the lane it came down, in its own
   species colour, and fades (stillHop). Nothing moves; nothing is hidden.

   And it is OVERRIDABLE IN BOTH DIRECTIONS, because a media query is a guess
   about a person and this one is wrong often. The three-way choice is kept in
   this browser and beats the query either way: auto follows the system, on
   animates regardless, off stops the travel on a system that never asked. */
var MOTIONKEY = "multiverse.motion", motionPref = "auto", mqReduced = false;

function readMotionPref(){
  try {
    var v = localStorage.getItem(MOTIONKEY);
    if (v === "auto" || v === "on" || v === "off") return v;
  } catch(e){}
  return "auto";
}

/* applyMotion is the ONE place the effective answer is computed, and it is
   idempotent: it is called at startup, on every press of the switch, and when
   the system setting itself changes under a page that is already open. */
function applyMotion(){
  reduced = (motionPref === "off") || (motionPref === "auto" && mqReduced);
  var bs = document.querySelectorAll("#motion .mbtn");
  for (var i=0;i<bs.length;i++){
    bs[i].setAttribute("aria-pressed",
      bs[i].getAttribute("data-m") === motionPref ? "true" : "false");
  }
  // It SAYS which way it went and why, so the reader who wondered where the
  // creatures went has the answer on the page instead of in a settings dialog.
  var why = $("#motionwhy");
  if (why){
    why.textContent = reduced
      ? (motionPref === "auto"
          ? "your system asks for reduced motion — a crossing appears at the world it reached instead of travelling there"
          : "a crossing appears at the world it reached instead of travelling there")
      : (mqReduced
          ? "animating anyway, over the reduced-motion setting your system reports"
          : "creatures travel their lane as they cross");
  }
  if (reduced){
    // Without the loop a travelling glyph would stall mid-lane with nothing to
    // move it, so switching motion off has to sweep what is already in flight.
    if (rafId){ cancelAnimationFrame(rafId); rafId = 0; }
    killHops();
  } else if (!rafId){
    lastTs = 0;
    rafId = requestAnimationFrame(frame);
  }
}

function setMotionPref(v){
  motionPref = v;
  try { localStorage.setItem(MOTIONKEY, v); } catch(e){}
  applyMotion();
}

(function wireMotion(){
  var box = document.getElementById("motion");
  if (box) box.addEventListener("click", function(ev){
    var b = ev.target.closest ? ev.target.closest(".mbtn") : null;
    if (b) setMotionPref(b.getAttribute("data-m"));
  });
  try {
    var mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    mqReduced = !!mq.matches;
    if (mq.addEventListener){
      mq.addEventListener("change", function(e){ mqReduced = !!e.matches; applyMotion(); });
    }
  } catch(e){}
  motionPref = readMotionPref();
  applyMotion();
})();

showTab(tabFromHash(), false);
tick(); setInterval(tick, 2000);
// Faster than the status poll: the feed is bounded at 60 seconds, and a hop
// should set off close to when it happened rather than up to two seconds late.
// It is polled WHATEVER the motion setting is: reduced motion changes how a
// crossing is DRAWN, never whether this page knows one happened.
tickHops(); setInterval(tickHops, 1500);
tickHistory(); setInterval(tickHistory, 60000);
// The trend column, on the same slow cadence and for the same reason: one
// bounded read of the sample file, feeding every row at once.
tickTrends(); setInterval(tickTrends, 60000);
// And the brain panel — where THE TIMER IS THE REFRESH AND NOT THE TRIGGER. Its
// window comes from a laid-out drawing, so the paint asks for it (lfBrainAsk,
// off renderLife) the moment there is one and again whenever the question
// changes; a load-time call here could only ever find no scale and return, which
// is what made a cold load wait a whole minute for an answer one request away.
// This timer moves the one edge that moves by itself, which is NOW.
setInterval(tickBrains, 60000);
</script>
</body>
</html>
`
