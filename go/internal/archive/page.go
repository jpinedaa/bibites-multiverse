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
// probe.
func (a *Archive) httpHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		view := a.StatusView()
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
		feed := a.HopFeedView()
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
		view := a.SpeciesIndexView()
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
		_ = json.NewEncoder(w).Encode(h)
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
	return mux
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

/* ---- the species index ---- */
.spctl{display:flex;gap:8px 14px;align-items:center;flex-wrap:wrap;margin-bottom:10px}
.spctl input,.spctl select{font:inherit;font-size:12px;color:var(--text);background:var(--cell);
border:1px solid var(--line);border-radius:4px;padding:5px 8px;min-width:0}
.spctl input{flex:1 1 200px;max-width:340px}
.spctl label{font-size:11px;color:var(--dim);display:flex;gap:6px;align-items:center}
table#sptab{min-width:760px}
tr.sprow{cursor:pointer}
tr.sprow:hover td{background:rgba(90,169,230,.08)}
tr.sprow.open td{background:rgba(90,169,230,.13)}
td.spname{white-space:nowrap;min-width:230px}
/* pre, not nowrap, on the NAME ITSELF: HTML collapses a run of spaces, and a
   doubled space inside a species name is the exact thing contract-a.md §17 A36
   says the page must print as the owning world holds it. It is not a
   curiosity — a measured 13% of the rig's names carry stray whitespace. */
td.spname .nm{font-weight:700;white-space:pre}
td.spname .alt{color:var(--dim);font-size:11px;margin-left:6px;white-space:pre}
.chip.exl{white-space:pre-wrap}
.spglyph{width:15px;height:12px;vertical-align:-2px;margin-right:5px}
.badge{display:inline-block;font-size:10px;letter-spacing:.06em;text-transform:uppercase;
border:1px solid var(--line);border-radius:9px;padding:0 7px;margin-left:6px;vertical-align:1px;
white-space:nowrap}
.badge.exc{color:var(--warn);border-color:var(--warn)}
.badge.every{color:var(--live);border-color:var(--live)}
.badge.endem{color:var(--lane);border-color:var(--lane)}
.chip{display:inline-block;font-size:11px;background:var(--cell);border:1px solid var(--line);
border-radius:4px;padding:1px 6px;margin:0 5px 3px 0;white-space:nowrap}
.chip b{font-variant-numeric:tabular-nums}
.chip.egg{color:var(--dim)}
.chip.exl{color:var(--warn);border-color:var(--warn);white-space:normal;word-break:break-word}
td.spwhere{white-space:normal;min-width:190px}
tr.spdet>td{background:var(--cell);white-space:normal;padding:10px 12px}
/* The detail lives inside a table with a 760px floor, so on a phone it would
   otherwise sit off the right-hand edge of a box the reader has to scroll. It
   sticks to the left of that scroll instead and takes the viewport's width, so
   opening a row on a phone shows the detail rather than hiding it. */
tr.spdet>td>div{position:sticky;left:0;max-width:calc(100vw - 72px)}
.detgrid{display:grid;gap:12px;grid-template-columns:repeat(auto-fill,minmax(min(240px,100%),1fr))}
.detbox{border-left:2px solid var(--line);padding-left:10px;min-width:0}
.detbox h3{margin:0 0 5px;font-size:11px;letter-spacing:.1em;text-transform:uppercase;color:var(--dim);
font-weight:400}
.detbox .big{font-size:19px;font-variant-numeric:tabular-nums}
.detline{font-size:11.5px;color:var(--dim);margin:2px 0}
.detline b{color:var(--text);font-variant-numeric:tabular-nums;font-weight:400}
.detnote{font-size:11px;color:var(--dim);margin-top:8px;font-style:italic}

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
.card .exl{margin-top:4px}
/* The shared creature definition lives in a zero-sized SVG at the top of the
   document rather than inside the map, because THREE tabs draw it now and the
   map's SVG is thrown away and rebuilt whenever the map's shape changes. */
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
    <span class="sub">what is alive, and where</span></button>
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
      <span class="term" data-t="speed">speed</span> is how fast that world runs,
      <span class="term" data-t="pace">pace</span> is arrivals queued over the cap they
      wait behind, per <em>simulated</em> minute of that world</span></h2>
    <div class="tw"><table id="worlds"><thead><tr>
      <th><span class="term" data-t="slot">slot</span></th>
      <th><span class="term" data-t="position">pos</span></th>
      <th><span class="term" data-t="peer">peer</span></th>
      <th>state</th>
      <th class="num"><span class="term" data-t="speed">speed</span></th>
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
      <span class="note muted">every species any world is
        <span class="term" data-t="census">reporting</span> in its census, joined across the
        map &mdash; a species that used to live here and does not now is not on this list, and
        what has <span class="term" data-t="crossings">crossed</span> is an annotation on a row
        the census put there, never a row of its own</span></h2>
    <div class="spctl">
      <input id="spq" type="search" autocomplete="off" spellcheck="false"
             placeholder="search a species…" aria-label="search species">
      <label for="spsort">sort
        <select id="spsort">
          <option value="pop">population</option>
          <option value="crossings">crossings</option>
          <option value="name">name</option>
        </select>
      </label>
      <span class="muted" id="spcount"></span>
    </div>
    <div class="tw"><table id="sptab"><thead><tr>
      <th><span class="term" data-t="species">species</span></th>
      <th class="num"><span class="term" data-t="population">alive</span></th>
      <th><span class="term" data-t="world">worlds</span></th>
      <th class="num"><span class="term" data-t="egg">eggs</span></th>
      <th class="num"><span class="term" data-t="crossings">crossings</span></th>
      <th>last</th>
      <th>first seen</th></tr></thead>
    <tbody id="spbody"></tbody></table></div>
  </section>
</div>

<div class="panel" id="p-settings" role="tabpanel" hidden>
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
  The three views share one poll and each has a link of its own &mdash;
  <code>#map</code>, <code>#species</code>, <code>#settings</code>. JSON:
  <code>/api/status</code> (live), <code>/api/hops</code> (the last minute of crossings,
  bounded in time and in count, and kept out of the durable sample file on purpose),
  <code>/api/species</code> (what is alive, joined to the crossing record),
  <code>/api/species/history?key=</code> and <code>/api/history</code> (downsampled,
  <code>?hours=</code>, <code>?buckets=</code>).
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
 speed:["simulation speed","How fast a world is running, as the game itself reports it: ×5 means five simulated seconds pass for every real second. It is the speed control inside that copy of the game, and each world has its own — one can be racing at ×100 while its neighbour sits at ×1. A paused world reports ×0. A world at a different speed from its neighbours is not a fault; it only means the two experience the traffic between them at different rates, which is why arrivals are paced on the receiving world's OWN clock and not on the wall clock."],
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
 genomegap:["genome gap","A genome the archive knows exists — it has the fingerprint — but has not managed to fetch a copy of yet. It keeps the fingerprint forever, so it can still fetch it tomorrow."],
 flow:["flow window","The per-minute rates on this page are measured over the last few minutes, not over all time, so they describe what is happening now."],
 alive:["alive right now","The species list is built from what the worlds are reporting THIS SECOND, and from nothing else. A kind that lived here yesterday and died out is not on it, however many times it crossed a lane while it was here. That is a deliberate refusal: a list built from crossings would be a list of travellers and their ancestors, which is a different thing from a list of residents and reads as though extinct kinds were still alive."],
 crossings:["crossings","How many times this archive has recorded a creature of this species walking from one world into another, since the archive started keeping records. It is a count of TRIPS, not of creatures: one restless creature crossing ten times counts ten. A species with no crossings is not a species that never moves — it may simply have arisen after the last time anything of its kind travelled."],
 endemic:["endemic","This species lives in exactly one world. It may have been born there and never left, or it may be the last holdout of something that used to be everywhere — the crossing count beside it is the hint. Endemic is not a warning; most new species start this way."],
 everywhere:["everywhere","This species is alive in every world that is currently reporting a census. It only claims that when at least two worlds are reporting: with one world answering, 'everywhere' and 'endemic' are the same sentence, and saying both would dress a single world's list up as a finding about the map."],
 excluded:["never exported","This species is on at least one world's exclusion list, which means that world never lets one of them out through a lane. It explains something otherwise baffling: a world can be full of a kind that never appears on any road out of it. The rule belongs to that world alone and is applied inside its own game — no other world, and nothing on this page, enforces or can enforce it. A species can be excluded by one world and travel freely from another."],
 speciesgenomes:["distinct genomes","How many genuinely different genetic makeups of this species have crossed a lane. Two creatures of one species are rarely identical; this counts the distinct ones the archive has fingerprints for. A big number beside a small population means a kind that is changing fast."],
 parentspecies:["parent species","The species this one was recorded as splitting off from, as the world that named it reported at the time. It is shown and nothing more — there is no family tree here, because the register that gives a species its parent lives inside one copy of the game and only that copy can read it."],
 settings:["settings","What a world was TOLD to do, as opposed to what it is doing. Everything else on this page is a measurement or a receipt; these are the knobs behind them — how often it saves, which species it refuses to export, whether its edges wrap. They are the reason a number elsewhere looks the way it does, and they are read-only here on purpose."],
 readonly:["read-only","This page shows settings and changes none of them. Changing another machine's world from a web page is a much bigger thing than showing one — it needs a login, a rule about who may change what, an answer for what happens when two people change the same thing at once, and a record of who changed it — and none of that is a small extension of showing a value. It is deliberately left for later."],
 savepolicy:["save policy","Three numbers that together answer 'what happens to this world if its machine stops'. How many minutes between saves — where 0 means the timer is OFF, which is a real setting and not a missing one, and is the explanation for a world that never shows a save. How many old saves it keeps beside the live one. And whether it saves when the game is closed."],
 savekeep:["saves kept","How many previous saves this world keeps on disk beside the current one. They rotate: the newest pushes the oldest out. It is the difference between being able to go back one step after something goes wrong and being able to go back several."],
 worldwrap:["world wrapping","Whether that world's own edges join up, so a creature walking off one side reappears on the other instead of piling up against a wall. This system needs it on: the whole map is a doughnut and a world that does not contain its own creatures leaks them into a corner. The world reports this setting and nothing here ever changes it, which is exactly why it is worth showing — it is how you check, from another machine, that nobody turned it off."],
 modversion:["mod version","The version of the small plugin running inside that copy of the game. It is a label, not a promise: what a world can actually report is decided field by field, by whether the field arrives. Use it to tell two machines apart, not to predict what one of them can do."],
 contractaversion:["protocol version","Which version of the language the game plugin and its helper program are speaking. It is the single most useful thing on this page for reading a gap: when a world shows '?' where its neighbours show a number, this says whether that world's plugin is simply too old to have anything to say."],
 simsize:["world size","How big that world is, as its own game reports it — the distance from the middle to an edge. Worlds of different sizes are wired together happily; it only means a creature crosses a small one faster."],
 exportedges:["export edges","The sides of a world that have a capture band running on them: the sides a creature can leave by. Every side can receive an arrival; these are the ones that can send. Which of them actually has a road to somewhere is the map's business, not the world's."]
};

(function buildGlossary(){
  var keys = ["world","slot","position","peer","lane","edge","shuttle","wrap","live","dark","hole",
    "bypass","migration","hopfeed","envelope","population","species","census","alive","egg","unclassed",
    "rawname","endemic","everywhere","excluded","crossings","speciesgenomes","parentspecies",
    "speed","pace","custody","custodyDepth","pacedDepth","held","bounce",
    "settings","readonly","savepolicy","savekeep","lastSave","worldwrap","modversion",
    "contractaversion","simsize","exportedges",
    "unknown","exactlyonce","relay","archive","epoch","genomegap","flow"];
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
   SPEED is the world's own time scale, straight out of the game's heartbeat —
   ×5 is five simulated seconds per real second, ×0 is a world standing still —
   and it is the key to every other number on this page that is counted in
   SIMULATED time. PACE is the arrival rate limit: how many are queued, and the
   cap they are queued behind, per simulated minute of THIS world.

   Both obey §10.1's unknown rule without exception. An older helper program
   publishes no cap, and the cap it would have had is NOT the shipped default —
   that default has moved three times — so it renders "?" and says so in the
   warning colour. A confident 100 there would be a lie about a world running at
   2.0, which is exactly the mistake this rig has already made once in a test. */
function fmtScale(n){
  if (n == null) return "?";
  var r = Math.round(n*10)/10;
  return Math.abs(r - Math.round(r)) < 0.05 ? String(Math.round(r)) : r.toFixed(1);
}
function speedText(v){
  return (v.statsKnown && v.timeScale != null) ? "×"+fmtScale(v.timeScale) : "×?";
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
  var sp = speedText(v), d = paceDepthText(v), r = paceRateText(v);
  part((sp === "×?" ? "chipu" : "chipv") + " term", sp, "speed");
  part(null, " · ", null);
  part("term", "pace ", "pace");
  part(d === "?" ? "chipu" : "chipv", d, null);
  part(null, "/", null);
  part(r === "?" ? "chipu" : "chipv", r, null);
  // 19 + the 8-character unit is 27, which is what fits between the cell's left
  // inset and its right edge at this font. Past that the unit goes and the two
  // numbers stay: a reader who has lost the unit can find it in the tooltip, the
  // glossary and the worlds table, but a number clipped by the world next door
  // is unreadable everywhere.
  if ((sp + " · pace " + d + "/" + r).length <= 19) part(null, "/sim-min", null);
}

function cellTitle(v){
  var s = "slot "+v.slot+" ("+v.position.col+","+v.position.row+")  peer "+v.peerId+"\n"
    + (v.live ? (v.modConnected ? "live" : "connected, but no game attached") : "dark");
  if (!v.live && v.darkForMs != null) s += " for "+ms(v.darkForMs);
  s += "\npopulation " + (v.statsKnown && v.population!=null ? v.population : "unknown");
  if (v.statsKnown){
    s += "\nrunning at " + speedText(v) + " real time"
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
   game. The wire's answer is a byte count, a UTF-8 decode and a cap. THE
   RENDERER'S ANSWER IS ITS OWN, and it is this:

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
   Everything below draws names — species names from six censuses, and, on the
   settings cards, the entries of six exclusion lists. An exclusion entry is the
   same class of text as a census name and arrives on the same unauthenticated
   stats block (contract-b-m4.md §13 item 7, §19 B18), so it is handled inside
   this fence for exactly the same reason, and by the same means: NOTHING HERE
   ASSIGNS MARKUP. Every element is created, every string lands as a text node.

   The consequence is that these two views build their whole DOM by hand rather
   than by templating a string, which is more code and is the point: the safe
   path is the only path, so one careless concatenation cannot appear later. */

var SPX = null, spOpenKey = null, spQuery = "", spSort = "pop", spHist = {};

/* One creature glyph at label size, in this species' own colour — the same
   definition the map draws, so a species reads the same on both tabs. */
function speciesGlyph(colour){
  var svg = document.createElementNS(SVGNS, "svg");
  svg.setAttribute("class", "spglyph");
  svg.setAttribute("viewBox", "-8 -5 16 10");
  svg.setAttribute("aria-hidden", "true");
  var u = document.createElementNS(SVGNS, "use");
  u.setAttribute("href", "#bib");
  if (colour) u.style.fill = colour; else u.setAttribute("class", "bib unclassed");
  svg.appendChild(u);
  return svg;
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

function spBadge(cls, text, term){
  var b = el("span", "badge " + cls, text);
  if (term) b.setAttribute("data-t", term);
  return b;
}

/* An unknown, in the page's own voice: "?" in the warning colour. §10.1's rule
   has no exception on this tab either. */
function unkEl(text){ return el("span", "unknown", text == null ? "?" : text); }

function spMatches(row, q){
  if (!q) return true;
  if (row.key.toLowerCase().indexOf(q) >= 0) return true;
  if (String(row.name).toLowerCase().indexOf(q) >= 0) return true;
  var alt = row.spellings || [];
  for (var i=0;i<alt.length;i++) if (String(alt[i]).toLowerCase().indexOf(q) >= 0) return true;
  return false;
}

function spSorted(list){
  var out = list.slice(0);
  out.sort(function(a,b){
    if (spSort === "name") return a.key < b.key ? -1 : (a.key > b.key ? 1 : 0);
    if (spSort === "crossings"){
      if (b.crossings !== a.crossings) return b.crossings - a.crossings;
      return b.population - a.population;
    }
    if (b.population !== a.population) return b.population - a.population;
    return a.key < b.key ? -1 : (a.key > b.key ? 1 : 0);
  });
  return out;
}

/* renderSpecies paints the whole index. It rebuilds the body every poll, which
   is affordable because the list is bounded by the census that produced it —
   at most 32 species a world — and which keeps the expanded row's own numbers
   as fresh as the row above it. */
function renderSpecies(x){
  SPX = x;
  var body = document.getElementById("spbody");
  if (!body) return;
  while (body.firstChild) body.removeChild(body.firstChild);

  var count = document.getElementById("spcount");
  if (count){
    while (count.firstChild) count.removeChild(count.firstChild);
    if (!x || !x.haveStatus){
      count.appendChild(unkEl("the relay has broadcast no map yet"));
    } else {
      count.appendChild(document.createTextNode(
        x.species.length + " alive across " + x.reportingSlots + " reporting world(s)"));
      if (x.censuslessSlots > 0){
        count.appendChild(document.createTextNode(" · "));
        count.appendChild(unkEl(x.censuslessSlots + " world(s) report no census"));
      }
      if (x.truncatedSlots > 0){
        count.appendChild(document.createTextNode(" · "));
        count.appendChild(unkEl(x.truncatedSlots + " census(es) capped at 32; the rest is unreported"));
      }
    }
  }
  if (!x || !x.species || !x.species.length){
    var tr0 = el("tr"), td0 = el("td", "muted",
      (x && x.haveStatus) ? "no world is reporting a species right now" : "waiting for the map");
    td0.setAttribute("colspan", "7");
    tr0.appendChild(td0); body.appendChild(tr0);
    return;
  }

  var list = spSorted(x.species), shown = 0;
  for (var i=0;i<list.length;i++){
    var row = list[i];
    if (!spMatches(row, spQuery)) continue;
    shown++;
    body.appendChild(speciesRow(row));
    if (spOpenKey === row.key) body.appendChild(speciesDetail(row));
  }
  if (!shown){
    var tr1 = el("tr"), td1 = el("td", "muted", "no species matches that search");
    td1.setAttribute("colspan", "7");
    tr1.appendChild(td1); body.appendChild(tr1);
  }
}

function speciesRow(row){
  var tr = el("tr", "sprow" + (spOpenKey === row.key ? " open" : ""));
  tr.setAttribute("data-k", row.key);

  var name = el("td", "spname");
  var colour = speciesColor(row.key);
  var sw = el("i", "sw");
  sw.style.background = colour;
  name.appendChild(sw);
  name.appendChild(speciesGlyph(colour));
  // THE RAW SPELLING, as the reporting world holds it (contract-a.md §17 A36).
  name.appendChild(el("span", "nm", row.name));
  var alt = row.spellings || [];
  if (alt.length){
    // A second spelling of one species is a real difference between two worlds,
    // not noise to be tidied away, so every one of them is shown.
    name.appendChild(el("span", "alt", "also spelt " + alt.join(" / ")));
  }
  if (row.excluded){
    name.appendChild(spBadge("exc",
      "never exported" + (row.excludedBy && row.excludedBy.length
        ? " by S" + row.excludedBy.join(", S") : ""), "excluded"));
  }
  if (row.everywhere) name.appendChild(spBadge("every", "everywhere", "everywhere"));
  if (row.endemic) name.appendChild(spBadge("endem", "endemic", "endemic"));
  tr.appendChild(name);

  tr.appendChild(el("td", "num", row.population));

  var where = el("td", "spwhere");
  var worlds = row.worlds || [];
  for (var i=0;i<worlds.length;i++){
    var c = el("span", "chip");
    c.appendChild(document.createTextNode("S" + worlds[i].slot + ":"));
    c.appendChild(el("b", null, worlds[i].bibites));
    if (worlds[i].eggs) c.appendChild(el("span", "egg", " +" + worlds[i].eggs + "e"));
    where.appendChild(c);
  }
  tr.appendChild(where);

  tr.appendChild(el("td", "num", row.eggs));
  tr.appendChild(el("td", "num", row.crossings));

  var last = el("td");
  if (row.lastAgeMs == null) last.appendChild(unkEl("never here"));
  else last.appendChild(document.createTextNode(ms(row.lastAgeMs) + " ago"));
  tr.appendChild(last);

  var first = el("td");
  if (!row.firstMs) first.appendChild(unkEl("—"));
  else first.appendChild(document.createTextNode(ms(Date.now() - row.firstMs) + " ago"));
  tr.appendChild(first);
  return tr;
}

/* The detail row. Everything in it comes from the ledger aggregate the index
   already carried, except the sparklines, which are fetched once per species
   and cached: the sample file is read on the server, downsampled there, and
   arrives as buckets — the page never sees the file and never counts a census
   itself. */
function speciesDetail(row){
  var tr = el("tr", "spdet"), td = el("td");
  td.setAttribute("colspan", "7");
  // ONE wrapper, because the sticky rule that keeps this readable on a phone
  // hangs off it: the table it sits in has a 760px floor and scrolls sideways,
  // and a detail pinned to the left of that scroll is a detail a phone reader
  // can actually see.
  var wrap = el("div", "detwrap");
  td.appendChild(wrap);

  var grid = el("div", "detgrid");

  var b1 = el("div", "detbox");
  b1.appendChild(termEl("h3", "speciesgenomes", "distinct genomes"));
  b1.appendChild(el("div", "big", row.genomes + (row.genomesAtLeast ? "+" : "")));
  b1.appendChild(el("div", "detline", "different genetic makeups of this species " +
    "that have crossed a lane"));
  grid.appendChild(b1);

  var b2 = el("div", "detbox");
  b2.appendChild(termEl("h3", "parentspecies", "parent species"));
  if (row.parent){
    var p = el("div", "detline");
    p.appendChild(el("b", null, row.parent));
    b2.appendChild(p);
    b2.appendChild(el("div", "detline", "as the world that named it reported at the time — " +
      "shown, never resolved"));
  } else {
    var pu = el("div", "detline");
    pu.appendChild(unkEl("none recorded"));
    b2.appendChild(pu);
    b2.appendChild(el("div", "detline",
      "no crossing of this species has carried a parent name"));
  }
  grid.appendChild(b2);

  var b3 = el("div", "detbox");
  b3.appendChild(termEl("h3", "crossings", "recent crossings"));
  var rec = row.recent || [];
  if (!rec.length){
    var ru = el("div", "detline");
    ru.appendChild(unkEl("none recorded"));
    b3.appendChild(ru);
  } else {
    for (var i=0;i<rec.length;i++){
      var line = el("div", "detline");
      line.appendChild(el("b", null, "S" + rec[i].fromSlot +
        (rec[i].exitEdge ? " " + rec[i].exitEdge : "") + " → S" + rec[i].toSlot));
      line.appendChild(document.createTextNode("  " + ms(rec[i].ageMs) + " ago"));
      b3.appendChild(line);
    }
    b3.appendChild(el("div", "detline",
      "the newest " + rec.length + " of " + row.crossings + " — a sample, not the whole"));
  }
  grid.appendChild(b3);
  wrap.appendChild(grid);

  var sparks = el("div", "sparks");
  sparks.setAttribute("id", "spspark");
  var h = spHist[row.key];
  if (h) fillSpeciesSparks(sparks, h);
  else sparks.appendChild(el("span", "muted", "loading this species' history…"));
  wrap.appendChild(sparks);

  var note = el("div", "detnote");
  note.appendChild(document.createTextNode(
    "population of this species per world, from the archive's own sample file. " +
    "A gap in a line is a world that reported no census in that bucket — unknown, never a zero — " +
    "and a flat zero is a world that reported and held none. " + speciesHistoryReach(h)));
  wrap.appendChild(note);
  tr.appendChild(td);
  return tr;
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
  var sp = speedText(v);
  setKV(card, "speed", "speed", sp === "×?" ? unkEl("×?") : txt(sp));
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
/* ============================= SPECIES CENSUS — END ======================== */

/* speciesHistoryReach and fillSpeciesSparks are the ONLY parts of the species
   detail that live outside the fence, and they are outside it because they
   build markup strings — which is exactly why NEITHER OF THEM IS EVER HANDED A
   NAME. What they draw is slot numbers, counts and SVG paths; the species they
   describe is identified to them by nothing at all, because the caller has
   already fetched the right series. */
/* HOW SHORT IS SHORT, SAID OUT LOUD. This record began when this archive did,
   which on the running rig is days rather than months, and a 24-hour chart with
   two hours of data in it looks exactly like a species that arrived two hours
   ago. The sentence below is what stops a reader drawing that conclusion.

   It is an UPPER BOUND and it says so: what is known is which BUCKET the oldest
   sample fell in, not where in that bucket it sat, so the honest statement is
   "no further back than", never a precise age this data cannot support. */
function speciesHistoryReach(H){
  if (!H) return "";
  if (!H.samples) return "No sample covers this window at all yet — this record began when the "
    + "archive did, and a chart with nothing on it is a short record, not an absent species.";
  for (var i=0;i<H.total.length;i++){
    if (H.total[i].n > 0){
      var from = Date.now() - H.total[i].tMs;
      return "The sampled record reaches back no further than " + ms(from)
        + " — earlier buckets are empty because nothing was recorded then, not because "
        + "nothing happened.";
    }
  }
  return "";
}

function fillSpeciesSparks(box, H){
  if (!H || !H.slots || !H.slots.length){
    box.innerHTML = '<span class="muted">no per-world history for this species yet</span>';
    return;
  }
  var span = Math.round((H.toMs - H.fromMs)/3600000);
  var spanTxt = span >= 1 ? span+"h" : Math.round((H.toMs-H.fromMs)/60000)+"m";
  var max = Math.max(1, H.max), h = "", i;
  var totMax = 1;
  for (i=0;i<H.total.length;i++) if (H.total[i].v != null && H.total[i].v > totMax) totMax = H.total[i].v;
  var totLast = null;
  for (i=H.total.length-1;i>=0;i--) if (H.total[i].v != null){ totLast = H.total[i].v; break; }
  h += sparkCard(t("population","whole map"), "this species, every world summed, "+spanTxt,
    totLast==null ? '<span class="unknown">unknown</span>' : totLast, H.total, totMax, {wide:true});
  for (i=0;i<H.slots.length;i++){
    var s = H.slots[i];
    var dead = s.points.length ? s.points[s.points.length-1].dark : false;
    h += sparkCard(t("slot","slot "+s.slot), (s.peerId||"")+" · "+spanTxt,
      s.last==null ? '<span class="unknown">unknown</span>' : s.last, s.points, max, {dead:dead});
  }
  box.innerHTML = h;
}

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
   replays the last minute as though it were happening now.

   THE TAB IS IN THE URL HASH, which is what makes "#species" a link somebody
   can send and a reload land where the reader was. */
var TABS = ["map","species","settings"], TAB = "map", lastStatus = null;

function tabFromHash(){
  var h = (location.hash || "").replace(/^#/, "");
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
  if (name === "species"){ if (SPX) renderSpecies(SPX); tickSpecies(); }
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

/* The species tab's own controls. A click on a row opens its detail; a click on
   a term inside that row is a tooltip and nothing else, or every badge would
   collapse the row it explains. */
function openSpecies(key){
  spOpenKey = (spOpenKey === key) ? null : key;
  if (spOpenKey) loadSpeciesHistory(spOpenKey);
  if (SPX) renderSpecies(SPX);
}

async function loadSpeciesHistory(key){
  var cached = spHist[key];
  if (cached && (Date.now() - cached.at) < 60000) return;
  try {
    var r = await fetch("api/species/history?key=" + encodeURIComponent(key)
      + "&hours=24&buckets=60", {cache:"no-store"});
    var H = await r.json();
    H.at = Date.now();
    spHist[key] = H;
  } catch(e){
    spHist[key] = {at: Date.now(), slots: [], total: [], samples: 0, fromMs: 0, toMs: 0};
  }
  if (spOpenKey === key && SPX) renderSpecies(SPX);
}

(function wireSpecies(){
  var body = document.getElementById("spbody");
  if (body) body.addEventListener("click", function(ev){
    if (ev.target.closest && ev.target.closest("[data-t],[data-s]")) return;
    var tr = ev.target.closest ? ev.target.closest("tr.sprow") : null;
    if (tr) openSpecies(tr.getAttribute("data-k"));
  });
  var q = document.getElementById("spq");
  if (q) q.addEventListener("input", function(){
    spQuery = q.value.toLowerCase();
    if (SPX) renderSpecies(SPX);
  });
  var s = document.getElementById("spsort");
  if (s) s.addEventListener("change", function(){
    spSort = s.value;
    if (SPX) renderSpecies(SPX);
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
    var sp = speedText(v);
    var speed = sp === "×?" ? '<span class="unknown">×?</span>' : sp;
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
    + kv("ledger records", d.ledgerRecords);
}

async function tick(){
  try {
    var r = await fetch("api/status", {cache:"no-store"});
    lastStatus = await r.json();
    render(lastStatus);
  } catch(e){
    $("#link").innerHTML = '<span class="bad">status endpoint unreachable</span>';
  }
  // The species index rides the SAME cycle rather than a timer of its own, and
  // it is only asked for while its tab is open: its ledger annotations are
  // derived from a file the browser must never be handed, so they cost the
  // archive a little work and are worth nothing to a tab nobody is looking at.
  if (TAB === "species") await tickSpecies();
}

async function tickSpecies(){
  try {
    var r = await fetch("api/species", {cache:"no-store"});
    renderSpecies(await r.json());
  } catch(e){
    var body = document.getElementById("spbody");
    if (body && !body.firstChild){
      var tr = document.createElement("tr"), td = document.createElement("td");
      td.className = "bad";
      td.setAttribute("colspan", "7");
      td.textContent = "species endpoint unreachable";
      tr.appendChild(td); body.appendChild(tr);
    }
  }
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
</script>
</body>
</html>
`
