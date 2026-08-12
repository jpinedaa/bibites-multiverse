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
		h, err := a.HistoryView(window, buckets)
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
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(statusPageHTML))
	})
	return gzipped(mux)
}

// historyParams reads ?hours= and ?buckets=, and clamps both. A reader may ask
// for a different window; it may not ask the archive to read an unbounded file
// or to build an unbounded answer.
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
<title>multiverse map</title>
<style>
:root{color-scheme:dark;--bg:#101215;--panel:#191d22;--line:#2a3038;--text:#e6e9ee;
--dim:#8b95a3;--live:#4ec9a0;--dark:#e05561;--hole:#3a424c;--warn:#e2b93b;--lane:#5aa9e6;
--flash:#7de3c0;--hot:#f4fffb;--cell:#14181d}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);
font:14px/1.45 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
header{padding:12px 18px;border-bottom:1px solid var(--line);display:flex;
gap:8px 18px;align-items:baseline;flex-wrap:wrap;position:sticky;top:0;background:var(--bg);z-index:20}
h1{font-size:15px;margin:0;letter-spacing:.08em;text-transform:uppercase;white-space:nowrap}
.muted{color:var(--dim)}
.hdr b{font-weight:700;color:var(--text);font-variant-numeric:tabular-nums}
/* minmax(0,...) and min-width:0 all the way down: a grid or flex item defaults to
   min-width:auto, and one wide child then pushes the whole page sideways. The
   map and the tables scroll INSIDE their own box; the body never does. */
main{padding:18px;display:grid;gap:18px;grid-template-columns:minmax(0,1fr)}
.panel{display:grid;gap:18px;grid-template-columns:minmax(0,1fr);min-width:0}
.panel[hidden]{display:none}
section{background:var(--panel);border:1px solid var(--line);border-radius:6px;padding:14px;
min-width:0;overflow:hidden}

/* ---- the tab bar ----
   Three views over ONE poll. It is sticky under the header because the header
   carries the numbers that are true on every tab, and it scrolls sideways
   rather than wrapping so a narrow phone gets three full-width tap targets
   instead of two lines of half-height ones. */
.tabs{position:sticky;top:0;z-index:19;display:flex;gap:0;background:var(--bg);
border-bottom:1px solid var(--line);overflow-x:auto;-webkit-overflow-scrolling:touch}
.tabs .tab{flex:1 1 0;min-width:104px;font:inherit;font-size:12px;letter-spacing:.12em;
text-transform:uppercase;color:var(--dim);background:none;border:0;border-bottom:2px solid transparent;
padding:12px 14px;cursor:pointer;white-space:nowrap}
.tabs .tab:hover{color:var(--text)}
.tabs .tab[aria-selected="true"]{color:var(--text);border-bottom-color:var(--lane)}
.tabs .tab .sub{display:block;font-size:10px;letter-spacing:.04em;text-transform:none;
color:var(--dim);margin-top:2px}
/* On a phone the three tap targets matter and the subtitles do not: keeping
   them turns a clean bar into a sideways-scrolling one. */
@media (max-width:560px){.tabs .tab .sub{display:none}.tabs .tab{padding:13px 8px}}

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
border:1px solid var(--line);border-radius:4px;padding:5px 8px;min-width:0}
.spctl input{flex:1 1 200px;max-width:340px}
.spctl label{font-size:11px;color:var(--dim);display:flex;gap:6px;align-items:center}
.chip{display:inline-block;font-size:11px;background:var(--cell);border:1px solid var(--line);
border-radius:4px;padding:1px 6px;margin:0 5px 3px 0;white-space:nowrap}
.chip b{font-variant-numeric:tabular-nums}
.chip.exl{color:var(--warn);border-color:var(--warn);white-space:pre-wrap;word-break:break-word}
.lifewrap{overflow:auto;max-height:min(78vh,1000px);border:1px solid var(--line);
border-radius:4px;background:var(--cell)}
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
/* The brain ring sits at the end of the bar and is never filled: it is a size,
   not a quantity of anything you could add up. */
svg.life .brain{fill:none;stroke:var(--lane);stroke-width:1.3;cursor:help}
svg.life .link{fill:none;stroke:var(--line);stroke-width:1.5}
svg.life .chain{fill:none;stroke:var(--dim);stroke-width:1.2;stroke-dasharray:2 3}
svg.life .wdot{stroke-width:1.1}
svg.life .wdot.on{fill:var(--text);stroke:none}
svg.life .wdot.off{fill:none;stroke:var(--line)}
svg.life .wdot.unk{fill:none;stroke:var(--warn);stroke-dasharray:1.5 1.5}
svg.life .trend{fill:none;stroke:var(--live);stroke-width:1.4;stroke-linejoin:round}
svg.life .trendbase{stroke:var(--line);stroke-width:1}
svg.life .grid{stroke:var(--line);stroke-width:1;opacity:.5}
svg.life .tick,svg.life .colhd{fill:var(--dim);font-size:10px;letter-spacing:.04em}
svg.life .nowline{stroke:var(--live);stroke-width:1;opacity:.55}
/* The record's floor: everything left of it is crossings that named no parent at
   all, which is why no edge in the drawing can start there. */
svg.life .floor{stroke:var(--warn);stroke-width:1.2;stroke-dasharray:4 3;opacity:.8}
svg.life .floorlbl{fill:var(--warn);font-size:9.5px;cursor:help}
svg.life .prefloor{fill:var(--hole);opacity:.10}
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
.treelegend i.doti{display:inline-block;width:7px;height:7px;background:var(--text);
border-radius:50%;vertical-align:0;margin-right:5px}
.treestat{display:flex;gap:8px 16px;flex-wrap:wrap;font-size:11px;margin-bottom:10px}
.treestat span b{color:var(--text);font-variant-numeric:tabular-nums;font-weight:400}
/* The one control on this line. It undoes a filter the view applied on its own,
   so it has to look like something you can press rather than like more prose. */
.treestat .seedbtn{font:inherit;font-size:11px;color:var(--dim);background:var(--cell);
border:1px solid var(--line);border-radius:4px;padding:1px 8px;margin-left:2px;cursor:pointer}
.treestat .seedbtn:hover{color:var(--text);border-color:var(--dim)}
.detline{font-size:11.5px;color:var(--dim);margin:2px 0}
.detline b{color:var(--text);font-variant-numeric:tabular-nums;font-weight:400}

/* ---- the settings cards ---- */
.cards{display:grid;gap:12px;grid-template-columns:repeat(auto-fill,minmax(min(300px,100%),1fr))}
.card{border:1px solid var(--line);border-radius:5px;background:var(--cell);padding:10px 12px;
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
h2{font-size:12px;margin:0 0 10px;letter-spacing:.12em;text-transform:uppercase;color:var(--dim);
display:flex;gap:8px 14px;align-items:center;flex-wrap:wrap;min-width:0}
h2 .note{text-transform:none;letter-spacing:0;font-size:11px}
.unknown{color:var(--warn);font-style:italic}
.bad{color:var(--dark)}
.ok{color:var(--live)}

/* ---- the map ---- */
.mapwrap{overflow-x:auto;overflow-y:hidden;max-width:100%}
#map{display:block;width:100%;height:auto;min-width:700px}
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

.laneunder{fill:none;stroke:var(--panel);stroke-width:9;stroke-linecap:round}
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
.sparks{display:grid;gap:10px;grid-template-columns:repeat(auto-fill,minmax(min(210px,100%),1fr))}
.spark{border:1px solid var(--line);border-radius:5px;padding:8px 10px 6px;background:var(--cell);
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
th,td{text-align:left;padding:4px 8px;border-bottom:1px solid var(--line);white-space:nowrap}
th{color:var(--dim);font-weight:400;letter-spacing:.06em}
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
border-bottom:1px solid var(--line);padding:3px 0}
.kv span:last-child{color:var(--text);font-variant-numeric:tabular-nums}

/* ---- glossary + tooltips ---- */
.term{border-bottom:1px dotted var(--dim);cursor:help}
text.term,tspan.term{text-decoration:underline dotted;cursor:help}
#tip{position:fixed;z-index:60;max-width:320px;background:#0b0e11;border:1px solid var(--line);
border-radius:6px;padding:9px 11px;font-size:12px;line-height:1.55;color:var(--text);
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
footer{padding:12px 18px;color:var(--dim);font-size:11px;border-top:1px solid var(--line)}

/* ---- the motion switch ----
   Small, and in the footer rather than the header, because it is a preference
   and not a reading. It is still a CONTROL: the pressed state has to be legible
   at a glance, or a reader cannot tell which of the three is in force. */
.motion{margin-top:10px;display:flex;gap:7px;align-items:baseline;flex-wrap:wrap}
.motion .mbtn{font:inherit;font-size:11px;color:var(--dim);background:var(--cell);
border:1px solid var(--line);border-radius:4px;padding:2px 9px;cursor:pointer}
.motion .mbtn:hover{color:var(--text);border-color:var(--dim)}
.motion .mbtn[aria-pressed="true"]{color:var(--bg);background:var(--lane);border-color:var(--lane)}
.motion .mwhy{font-size:11px;color:var(--dim)}
@media (max-width:640px){main{padding:10px;gap:10px}section{padding:10px}header{padding:10px}}
</style>
</head>
<body>
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
<header>
  <h1>multiverse</h1>
  <span class="hdr muted" id="shape">&hellip;</span>
  <span class="hdr muted"><span class="term" data-t="population">population</span> <b id="hpop">&mdash;</b></span>
  <span class="hdr muted"><span class="term" data-t="migration">migrations</span> <b id="hmig">&mdash;</b></span>
  <span class="hdr muted" id="link"></span>
  <span class="hdr muted" id="age"></span>
</header>
<nav class="tabs" id="tabs" role="tablist" aria-label="views">
  <button type="button" class="tab" role="tab" data-tab="map" aria-selected="true">map
    <span class="sub">worlds, lanes, crossings</span></button>
  <button type="button" class="tab" role="tab" data-tab="species" aria-selected="false">species
    <span class="sub">what is alive, where, and where it came from</span></button>
  <button type="button" class="tab" role="tab" data-tab="settings" aria-selected="false">settings
    <span class="sub">what each world was told to do</span></button>
</nav>
<main>
<div class="panel" id="p-map" role="tabpanel">
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
    <div class="mapwrap"><div id="mapbox"></div></div>
  </section>

  <section>
    <h2>history <span class="note muted">population per world, last 24 hours, from the
      archive&rsquo;s own sample file &mdash; a gap in a line is
      <span class="term" data-t="unknown">unknown</span>, never a zero</span></h2>
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

<div class="panel" id="p-species" role="tabpanel" hidden>
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
        <span class="term" data-t="collapsed">collapses to one dotted edge</span> that says how
        many generations it stood for. Nothing here is resolved against any world&rsquo;s own
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
      <span><i class="bari"></i>first recorded crossing &rarr; last, or &rarr; now while alive</span>
      <span><i class="chaini"></i>&plus;<b>n</b> &mdash; extinct generations collapsed; the
        dotted length counts <span class="term" data-t="collapsed">generations, not time</span></span>
      <span><i class="ringi braini"></i><span class="term" data-t="brainsize">brain size</span>
        &mdash; bigger ring, more neurons and synapses</span>
      <span><i class="doti"></i><span class="term" data-t="minimap">where it lives</span>,
        on the map&rsquo;s own grid</span>
      <span><span class="term" data-t="trend">the 24 h line</span> &mdash; its population
        across the map</span>
      <span>click a row for its worlds, its record and its parent</span>
    </p>
    <div class="lifewrap" id="lfbox"></div>
  </section>
</div>

<div class="panel" id="p-settings" role="tabpanel" hidden>
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
 collapsed:["+n generations","A run of ancestors that all died out, with no living branch anywhere along it, drawn as ONE dotted edge with the number of generations it stood for. Drawing all of them would be a column of names nothing alive belongs to; leaving the number off would make a distant cousin look like a sibling. So the number is the difference between the two. The dotted run has a LENGTH, and that length counts generations and not time: everything else on this drawing is measured against the clock along the top, and this one mark is not, which is why it is dotted and why the number is printed beside it."],
 lifespan:["the bar, and what it is not","A species' bar starts at the first crossing THIS ARCHIVE RECORDED of it and ends at the last one, or at the right-hand edge while the species is still alive somewhere. It is not a lifespan and this page never calls it one. A kind can have lived for days before anything of it walked into another world, and a kind whose bar stopped last Tuesday may be alive and simply staying at home — what stopped is the record, and the only honest thing a record can draw is itself. The one place the drawing goes beyond that is an ancestor no crossing of its own was ever recorded for: its bar begins at its earliest recorded DESCENDANT, because that descendant's crossing named it as a parent, so the record does support 'it existed by then' and supports nothing earlier. Those bars are drawn hollow."],
 brainsize:["brain size","The ring at the end of a species' bar. Every creature carries a brain — a little network of neurons wired together by synapses — and this is how big the newest one of that species the archive has a copy of actually is: a bigger ring is more neurons and more synapses. It is drawn from ONE genome per species, the latest the crossing record named, and it is read out of the copy in the archive's own store. WHERE THERE IS NO RING THERE IS NO ANSWER, which is not the same as a small brain: the copy may never have arrived, or may have been deleted once it was older than the retention horizon. The record of the crossing stays forever either way; the genetic material is the part that ages out."],
 minimap:["where it lives","The little grid of dots beside a species, laid out the way the map itself is laid out — this map is three worlds wide and two high, so it is three dots by two, and a different map would draw a different grid. A filled dot is a world where this species is alive right now. A hollow dot is a world that sent its census and did not name it. A dashed amber dot is a world reporting no census at all, which is UNKNOWN and never 'not there'. A seat nobody has claimed is drawn as nothing."],
 trend:["the 24-hour line","A small line of this species' population across the whole map over the last day, from the archive's own sample file — the shape, not the numbers. A gap in the line is a stretch where no world reported a census, which is unknown and never a zero. Every one of these lines comes from a single answer covering every living species at once, so the column costs one request rather than one per species. This record began when the archive did, so a short line is a short record and not a young species."],
 recordfloor:["the record begins here","A species drawn with nothing above it, and ancestors above it all the same. The number beside the label is how many generations of them the record holds: every one is extinct with no other living line, so the whole run is collapsed into the row you can see rather than drawn as a column of names nothing alive belongs to. Above the top of that run the record simply stops: ancestry here is only ever carried by a crossing, and the date on the tab is the earliest crossing this archive holds that named a parent at all — anything older than it is a crossing that named none. So the top of a family here is the edge of the record, not the first creature of its kind. That is why the game's starting species is not the root of this tree: its descendants are all here, but the links back to it were never recorded, and a link nobody recorded is not one this page will draw."],
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
    "genealogy","lifespan","branchpoint","collapsed","noancestry","recordfloor",
    "minimap","trend","brainsize",
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
  var w = Math.max(d.map.width,1), h = Math.max(d.map.height,1);
  var W = 2*MG + w*CW + (w-1)*GX, H = 2*MG + h*CH + (h-1)*GY;
  var byslot = {}, at = {};
  for (var i=0;i<d.slots.length;i++){
    byslot[d.slots[i].slot] = d.slots[i];
    at[d.slots[i].position.col+","+d.slots[i].position.row] = d.slots[i];
  }

  var s = '<svg id="map" viewBox="0 0 '+W+' '+H+'" role="img" '
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
/* The shared trend answer, keyed by species, and what it says about its own
   reach. Both are null until the first slow poll lands, and a row with no
   entry draws no line — never a flat one, which would be a claim. */
var LFTREND = null, LFTRENDMETA = null;

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
/* A collapsed run of generations, drawn as a dotted lead-in whose LENGTH COUNTS
   GENERATIONS AND NOT TIME. Five pixels a generation, floored so one generation
   is still visible and capped so forty do not cross the whole picture; the
   number is printed beside it either way, because a length a reader has to
   measure is not a number. */
var LF_GENPX = 5, LF_CHAINMIN = 12, LF_CHAINMAX = 130;
function lfChainLen(gens){
  return Math.max(LF_CHAINMIN, Math.min(LF_CHAINMAX, gens * LF_GENPX));
}
/* THE BRAIN, as a ring at the end of the bar — the end, because the stats come
   from the LATEST genome of that species the record named, so they describe what
   it is now and not what it was when it first crossed.

   A ring and not a thickness: thickness has to have a value for every bar, and
   most of the honest answers here are ABSENT — a genome pruned past the
   retention horizon, or never fetched, has no brain to draw. An absent ring is
   nothing at all, which cannot be misread as a small brain. */
var LF_BRAIN_R0 = 2.4, LF_BRAIN_R1 = 6.4, LF_BRAIN_FULL = 600;
function lfBrainR(n){
  var w = (n.neurons || 0) + (n.synapses || 0);
  if (w <= 0) return 0;
  var f = Math.log(1 + w) / Math.log(1 + LF_BRAIN_FULL);
  if (f > 1) f = 1;
  return LF_BRAIN_R0 + f * (LF_BRAIN_R1 - LF_BRAIN_R0);
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
    lines.push("Brain: " + n.neurons + " neurons and " + n.synapses + " synapses, from the " +
      "latest genome of it this archive holds a copy of. The ring at the end of the bar is " +
      "that size.");
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
    lines.push("The dotted lead-in above it stands for " + n.collapsed +
      " extinct generation(s) with no living branch on them. Its LENGTH counts generations, " +
      "not time.");
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
  // THE BRAIN, and its absence. An absent one says WHY rather than printing a
  // zero: the archive keeps the fingerprint of every genome forever and the copy
  // only while the retention horizon allows.
  var brain = [{t: "brain", c: "dk", term: "brainsize"}];
  if (n.neurons){
    brain.push({t: "  " + n.neurons + " neurons · " + n.synapses + " synapses"});
    brain.push({t: "  (from the latest genome of it this archive holds)", c: "dk"});
  } else {
    brain.push({t: "  no copy of its latest genome is held here", c: "unk"});
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

/* One species' bar, and the brain ring at the end of it. */
function lfBar(g, n, top, sc){
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
  if (x1 - x0 < 2.5) x1 = x0 + 2.5;
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
    ring.setAttribute("data-t", "brainsize");
    g.appendChild(ring);
  }
}

/* The join to the parent: a drop from the parent's row to this one AT THIS
   SPECIES' FIRST-SEEN POINT, which is the instant the record first says this
   kind exists. Where the edge stands for a run of collapsed generations, a
   dotted lead-in runs back from that point and the number is printed on it. */
function lfEdge(links, n, rowY, cols, sc){
  var py = rowY[n.parent], cy = rowY[n.key];
  if (py == null || cy == null || !n.spanFromMs) return;
  var jx = sc.x(n.spanFromMs);
  var drop = svgEl("path", "link");
  drop.setAttribute("d", "M" + jx.toFixed(1) + " " + py + " V" + cy);
  links.appendChild(drop);
  if (n.collapsed > 0){
    var from = Math.max(cols.plot - 18, jx - lfChainLen(n.collapsed));
    var chain = svgEl("path", "chain");
    chain.setAttribute("d", "M" + from.toFixed(1) + " " + cy + " H" + jx.toFixed(1));
    links.appendChild(chain);
    var gt = svgEl("text", "gen");
    gt.setAttribute("x", from.toFixed(1));
    gt.setAttribute("y", String(cy - 4));
    gt.setAttribute("data-t", "collapsed");
    gt.textContent = "+" + n.collapsed;
    links.appendChild(gt);
  }
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
  var g = svgEl("g", "lfrow" + (n.alive ? "" : " anc") +
                     (lfOpenKey === n.key ? " open" : ""));
  var tk = "lf" + i;
  // data-s registers the row's tooltip; the key is generated here and the NAME
  // never becomes part of a selector.
  SP[tk] = lfTip(n);
  g.setAttribute("data-s", tk);
  g.setAttribute("data-k", n.key);
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

  if (n.alive){
    // The same creature glyph the map draws, in the same per-species colour, and
    // it is THE ONLY COLOURED THING ON THE ROW.
    var u = svgEl("use");
    u.setAttribute("href", "#bib");
    u.setAttribute("transform",
      "translate(" + LF_GLYPHX + " " + (top + 12) + ") scale(0.85)");
    u.style.fill = speciesColor(n.key);
    g.appendChild(u);
  } else {
    var ring = svgEl("circle", "ring");
    ring.setAttribute("cx", String(LF_GLYPHX));
    ring.setAttribute("cy", String(top + 12));
    ring.setAttribute("r", "4");
    g.appendChild(ring);
  }

  var text = svgEl("text", "nm");
  // Depth as a small indent, so the family reads in the label column too. It is
  // capped: forty generations of indentation would push every name off the left
  // of its own column.
  var indent = joined ? Math.min(n.depth || 0, 6) * 9 : 0;
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
  lfBar(g, n, top, sc);
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
  for (var tms = first; tms <= sc.t1; tms += step){
    var px = sc.x(tms);
    var line = svgEl("line", "grid");
    line.setAttribute("x1", String(px)); line.setAttribute("x2", String(px));
    line.setAttribute("y1", String(LF_PADT - 16)); line.setAttribute("y2", String(height - 6));
    g.appendChild(line);
    var lbl = svgEl("text", "tick");
    lbl.setAttribute("x", String(px));
    lbl.setAttribute("y", String(LF_PADT - 22));
    lbl.setAttribute("text-anchor", "middle");
    lbl.textContent = step >= 86400000 ? trDay(tms).slice(5) : trClock(tms);
    g.appendChild(lbl);
  }
  // THE RECORD'S FLOOR, as a boundary rather than a footnote: everything left of
  // this line is crossings that carried no parent at all, so no edge in this
  // drawing can begin there. It is the reason a root is a root.
  //
  // THE TEST IS >= AND THE DIFFERENCE IS THE WHOLE MARK. The server clamps the
  // axis down to the floor so the floor is always inside the picture — so on
  // every map whose oldest drawn bar is younger than the floor, the two are
  // EXACTLY EQUAL, which is the running rig's own case. A strict > drew the
  // boundary on no map at all: it appeared only when something was older than the
  // floor, which is precisely when it matters least. At equality the line sits on
  // the axis's left edge and the shaded run before it is empty, which is the
  // truth: nothing in this picture predates the record's ancestry.
  if (x.ancestrySinceMs && x.ancestrySinceMs >= sc.t0){
    var fx = sc.x(x.ancestrySinceMs);
    var shade = svgEl("rect", "prefloor");
    shade.setAttribute("x", String(cols.plot));
    shade.setAttribute("y", String(LF_PADT - 16));
    shade.setAttribute("width", String(Math.max(0, fx - cols.plot)));
    shade.setAttribute("height", String(height - 6 - (LF_PADT - 16)));
    g.appendChild(shade);
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
  return g;
}

/* renderLife paints the whole view. It rebuilds every poll, which is affordable
   because the node set is bounded by the census that produced it — at most 32
   species a world — and which keeps an expanded row's numbers as fresh as the
   row above it. */
function renderLife(x){
  LFX = x;
  // THE ROWS ARE CHOSEN FIRST, because the counts line describes THIS drawing:
  // how many rows the seed filter took out of it, and how many seed rows are on
  // it anyway.
  var pick = lfRows(x), list = pick.list, i;
  lfCount(x);
  lfStats(x, pick.hid, pick.seed);
  var host = document.getElementById("lfbox");
  if (!host) return;
  while (host.firstChild) host.removeChild(host.firstChild);
  // This view's tooltips are rebuilt with it. They share SP with the map's
  // species runs and are prefixed so neither can clear the other's entries.
  for (var old in SP){ if (old.indexOf("lf") === 0) delete SP[old]; }

  if (!x || !x.haveStatus){
    host.appendChild(el("div", "muted", "waiting for the map"));
    return;
  }
  if (!list.length){
    host.appendChild(el("div", "muted", lfQuery
      ? "no species matches that search"
      : "no world is reporting a species right now, so there is nothing to relate"));
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
  var rowY = {};
  for (i=0;i<list.length;i++) rowY[list[i].key] = tops[i] + LF_ROWH / 2 - 1;
  if (pick.joined){
    for (i=0;i<list.length;i++){
      if (list[i].parent) lfEdge(links, list[i], rowY, cols, sc);
    }
  }
  for (i=0;i<list.length;i++){
    svg.appendChild(lfRow(x, list[i], tops[i], cols, sc, i, pick.joined));
  }
  host.appendChild(svg);
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
function sparkPath(points, max, w, h){
  var line = "", area = "", open = false, n = points.length, last = null;
  for (var i=0;i<n;i++){
    var p = points[i];
    if (p.v == null){
      if (open){ area += " L "+xAt(i-1,n,w)+" "+h+" Z"; open = false; }
      continue;
    }
    var x = xAt(i,n,w), y = h - 3 - (max>0 ? (p.v/max)*(h-7) : 0);
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

function sparkCard(title, sub, valueTxt, points, max, opts){
  var W = 220, H = 54;
  var dead = opts && opts.dead;
  var g = sparkPath(points, max, W, H);
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
    + '<div class="sparkft"><span>'+esc(sub)+'</span><span>0&ndash;'+max+'</span></div></div>';
}

function renderHistory(H){
  var box = $("#spark");
  if (!H || !H.slots){ box.innerHTML = '<span class="muted">no history yet</span>'; return; }
  var hours = Math.round((H.toMs - H.fromMs)/3600000);
  var span = hours >= 1 ? hours+"h" : Math.round((H.toMs-H.fromMs)/60000)+"m";
  var max = Math.max(1, H.maxPopulation);
  var h = "";

  var totMax = 1, i;
  for (i=0;i<H.total.length;i++) if (H.total[i].v != null && H.total[i].v > totMax) totMax = H.total[i].v;
  var totLast = null;
  for (i=H.total.length-1;i>=0;i--) if (H.total[i].v != null){ totLast = H.total[i].v; break; }
  h += sparkCard(t("population","whole map"), "every world summed, "+span,
    totLast==null ? '<span class="unknown">unknown</span>' : totLast, H.total, totMax, {wide:true});

  var flowMax = 1, flowSum = 0;
  for (i=0;i<H.flow.length;i++) if (H.flow[i].v != null){
    if (H.flow[i].v > flowMax) flowMax = H.flow[i].v;
    flowSum += H.flow[i].v;
  }
  h += sparkCard(t("migration","migrations"), "per "+Math.round(H.bucketMs/60000)+" min, "+span,
    flowSum, H.flow, flowMax, {bars:true, wide:true});

  for (i=0;i<H.slots.length;i++){
    var s = H.slots[i];
    var dead = s.points.length ? s.points[s.points.length-1].dark : false;
    h += sparkCard(t("slot","slot "+s.slot), (s.peerId||"")+" · "+span,
      s.last==null ? '<span class="unknown">unknown</span>' : s.last,
      s.points, max, {dead:dead});
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
    bs[i].setAttribute("aria-selected",
      bs[i].getAttribute("data-tab") === name ? "true" : "false");
  }
  document.title = "multiverse " + name;
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
  if (name === "species"){ if (LFX) renderLife(LFX); tickLife(); tickTrends(); }
  if (name === "map") tickHistory();
  if (push && location.hash !== "#"+name) location.hash = "#"+name;
}

(function wireTabs(){
  var bar = document.getElementById("tabs");
  if (bar) bar.addEventListener("click", function(ev){
    var b = ev.target.closest ? ev.target.closest(".tab") : null;
    if (b) showTab(b.getAttribute("data-tab"), true);
  });
  window.addEventListener("hashchange", function(){
    var want = tabFromHash();
    if (want !== TAB) showTab(want, false);
  });
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
  if (box) box.addEventListener("click", function(ev){
    var g = ev.target.closest ? ev.target.closest(".lfrow") : null;
    if (g) openLife(g.getAttribute("data-k"));
  });
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
async function tickHistory(){
  if (TAB !== "map") return;
  try {
    var r = await fetch("api/history?hours=24&buckets=120", {cache:"no-store"});
    renderHistory(await r.json());
  } catch(e){
    $("#spark").innerHTML = '<span class="bad">history endpoint unreachable</span>';
  }
}

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
</script>
</body>
</html>
`
