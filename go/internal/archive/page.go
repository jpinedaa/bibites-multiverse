package archive

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
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
--pulse:#7de3c0;--hot:#f4fffb;--cell:#14181d}
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
section{background:var(--panel);border:1px solid var(--line);border-radius:6px;padding:14px;
min-width:0;overflow:hidden}
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
.cell .statelbl{font-size:11px;letter-spacing:.06em}
.cell.live .statelbl,.cell.live .statedot{fill:var(--live)}
.cell.dark .statelbl,.cell.dark .statedot{fill:var(--dark)}
.cell.hole .statelbl,.cell.hole .statedot{fill:var(--hole)}
.dot{fill:var(--live)}
.cell.dark .dot{fill:var(--dark);opacity:.55}
.cellhit{fill:none;stroke:var(--pulse);stroke-width:3;opacity:0;pointer-events:none}
.cell.hit .cellhit{animation:hit .8s ease-out}
@keyframes hit{0%{opacity:.95}100%{opacity:0}}

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
.pulse{fill:var(--pulse)}
.pulse.hot{fill:var(--hot)}
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
.legend i.dot{width:9px;height:9px;border-radius:50%;background:var(--pulse);border:0}

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
text-transform:uppercase;margin-bottom:4px}
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
@media (max-width:640px){main{padding:10px;gap:10px}section{padding:10px}header{padding:10px}}
@media (prefers-reduced-motion:reduce){.cell.hit .cellhit{animation:none}}
</style>
</head>
<body>
<header>
  <h1>multiverse map</h1>
  <span class="hdr muted" id="shape">&hellip;</span>
  <span class="hdr muted"><span class="term" data-t="population">population</span> <b id="hpop">&mdash;</b></span>
  <span class="hdr muted"><span class="term" data-t="migration">migrations</span> <b id="hmig">&mdash;</b></span>
  <span class="hdr muted" id="link"></span>
  <span class="hdr muted" id="age"></span>
</header>
<main>
  <section>
    <h2>the map
      <span class="legend">
        <span><i class="box live"></i><span class="term" data-t="live">live</span></span>
        <span><i class="box dark"></i><span class="term" data-t="dark">dark</span></span>
        <span><i class="box hole"></i><span class="term" data-t="hole">hole</span></span>
        <span><i class="open"></i><span class="term" data-t="lane">lane open</span></span>
        <span><i class="bypass"></i><span class="term" data-t="bypass">bypass</span></span>
        <span><i class="closed"></i>lane closed</span>
        <span><i class="dot"></i><span class="term" data-t="migration">a migration</span></span>
        <span><span class="term" data-t="wrap">wrap-around</span></span>
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
    <span class="note muted">the detail behind every cell</span></h2>
    <div class="tw"><table id="worlds"><thead><tr>
      <th><span class="term" data-t="slot">slot</span></th>
      <th><span class="term" data-t="position">pos</span></th>
      <th><span class="term" data-t="peer">peer</span></th>
      <th>state</th>
      <th class="num"><span class="term" data-t="population">pop</span></th>
      <th class="num"><span class="term" data-t="custodyDepth">custody</span></th>
      <th class="num"><span class="term" data-t="pacedDepth">paced</span></th>
      <th class="num"><span class="term" data-t="held">held</span></th>
      <th class="num"><span class="term" data-t="bounce">bounces</span></th>
      <th><span class="term" data-t="lastSave">last save</span></th>
      <th>note</th></tr></thead>
    <tbody></tbody></table></div></section>

  <section><h2>totals</h2><div id="totals"></div></section>

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
  <span class="term" data-t="exactlyonce">exactly-once</span> promise. Rates are measured over a
  short <span class="term" data-t="flow">flow window</span>, not over all time. JSON:
  <code>/api/status</code> (live) and <code>/api/history</code> (downsampled,
  <code>?hours=</code>, <code>?buckets=</code>).
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
 lane:["lane","A one-way road from one world to the next. Creatures only ever travel east and north, so the west and south sides of a world are where arrivals come in, never where they leave."],
 edge:["edge","The side of a world a creature leaves by. E is east, to the right. N is north, upwards. Those two are the only exits."],
 wrap:["wrap-around","The map is a doughnut, not a sheet of paper. Walk east off the right-hand column and you arrive in the left-hand one; walk north off the top row and you arrive at the bottom. Nothing ever falls off an edge. On the map those lanes are drawn leaving one side and re-entering the opposite side, with the map edge marked."],
 live:["live","This world is connected right now: it can send creatures and it can receive them."],
 dark:["dark","This world is not connected right now — the game is closed, the machine is off, or the network dropped. Its seat stays on the map, and the worlds pointing at it reach past it instead."],
 hole:["hole","An empty seat: a square inside the map that no world has claimed. Lanes step straight over it in both directions."],
 bypass:["bypass / route-around","When a world goes dark, the lane that pointed at it does not shut down — it reaches over that world to the next one that is actually there, and the current keeps flowing. That is the good news and the danger at once: everything can look healthy while a world has been dead since yesterday. So a bypass is always drawn as a warning, with what it is skipping and for how long."],
 custody:["custody","Exactly one side is holding a travelling creature at any instant, and it is written to disk before it is handed over. The sender keeps it until the receiver confirms the creature arrived; only then does the sender let go."],
 custodyDepth:["custody depth","How many creatures this world is holding mid-journey right now — sent but not yet confirmed, plus arrived but not yet let into the world."],
 pacedDepth:["paced depth","Arrivals are let into a world at a capped rate so a burst cannot flood it. This is how many are queued up waiting their turn. A queue that never drains means the cap is set too low."],
 held:["held","A creature whose destination went dark while it was travelling. It waits, quietly retrying; if the destination stays gone long enough it is sent back where it came from rather than lost."],
 bounce:["bounce","A migration that gave up and returned to the world it started in. It is a thing you get told about, never a silent repair."],
 migration:["migration / hop","One creature's trip from one world to the next. Every hop is copied to the archive as it happens, which is where all the counts on this page come from. On the map each hop is a moving dot travelling along its lane."],
 lastSave:["last save","Every world saves itself to disk every few minutes and sends back a receipt — when it saved, how big the file was, how long it took. This is the age of the newest receipt from that world."],
 population:["population","How many living creatures the world holds, as its own game reported it. Each dot inside a cell is one creature."],
 unknown:["unknown","A number that is missing, or older than the freshness rule allows, is shown as unknown — never as zero. A world that has told us nothing is unknown, not empty. An honest gap beats a confident zero."],
 exactlyonce:["exactly-once","The promise the whole thing is built on: a creature is never duplicated. It is in one world, or in transit, or in the next world — never in two places at once. Very rarely one can be lost instead, and a lost creature simply reads as a natural death."],
 relay:["relay","The small server in the middle. Every world's helper connects to it and to nothing else, so there is exactly one map and one set of rules."],
 archive:["archive","The program serving this page. It listens to everything the relay broadcasts, keeps a permanent record of every migration and a copy of every genome, and never asks a world for anything — so watching can never slow the traffic down."],
 envelope:["envelope","One message carrying one creature between two worlds. The counts here are the envelopes this archive was copied on."],
 epoch:["epoch","A counter the relay bumps every time the map itself changes. If it jumps, a world joined, left or moved."],
 genomegap:["genome gap","A genome the archive knows exists — it has the fingerprint — but has not managed to fetch a copy of yet. It keeps the fingerprint forever, so it can still fetch it tomorrow."],
 flow:["flow window","The per-minute rates on this page are measured over the last few minutes, not over all time, so they describe what is happening now."]
};

(function buildGlossary(){
  var keys = ["world","slot","position","peer","lane","edge","wrap","live","dark","hole","bypass",
    "migration","envelope","population","custody","custodyDepth","pacedDepth","held","bounce",
    "lastSave","unknown","exactlyonce","relay","archive","epoch","genomegap","flow"];
  var h = "";
  for (var i=0;i<keys.length;i++){
    var g = G[keys[i]];
    if (!g) continue;
    h += '<div class="glossitem"><b>'+esc(g[0])+'</b><span>'+esc(g[1])+'</span></div>';
  }
  $("#glosslist").innerHTML = h;
})();

/* Tooltips: hover on a pointer, tap to toggle on a phone. Delegated, so terms
   drawn later into the SVG work without being wired up one by one. */
var tip = $("#tip"), tipFor = null;
function showTip(el, x, y){
  var g = G[el.getAttribute("data-t")];
  if (!g) return;
  tip.innerHTML = "<b>"+esc(g[0])+"</b>"+esc(g[1]);
  tip.style.display = "block";
  var w = tip.offsetWidth, h = tip.offsetHeight;
  var left = Math.min(Math.max(8, x - w/2), window.innerWidth - w - 8);
  var top = y - h - 14;
  if (top < 8) top = y + 20;
  tip.style.left = left+"px"; tip.style.top = top+"px";
  tipFor = el;
}
function hideTip(){ tip.style.display = "none"; tipFor = null; }
document.addEventListener("mouseover", function(e){
  var t = e.target.closest ? e.target.closest("[data-t]") : null;
  if (t) showTip(t, e.clientX, e.clientY);
});
document.addEventListener("mouseout", function(e){
  var t = e.target.closest ? e.target.closest("[data-t]") : null;
  if (t && t === tipFor) hideTip();
});
document.addEventListener("click", function(e){
  var t = e.target.closest ? e.target.closest("[data-t]") : null;
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
var CW=200, CH=150, GX=88, GY=80, MG=96, WOUT=40, WBAR=58;
var DOTCOLS=14, DOTROWS=5, DOTCAP=DOTCOLS*DOTROWS;
var mapSig = "", anim = [], pulseLayer = null, reduced = false, prevMig = {}, rafId = 0;

function cellX(c){ return MG + c*(CW+GX); }
function cellY(r,h){ return MG + (h-1-r)*(CH+GY); }
function laneY(r,h){ return cellY(r,h) + CH*0.58; }
function laneX(c){ return cellX(c) + CW*0.5; }

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
function laneGeom(l, d, byslot){
  var from = byslot[l.fromSlot];
  if (!from) return null;
  var w = d.map.width, h = d.map.height;
  var north = (l.edge === "N");
  var steps = (l.skipped ? l.skipped.length : 0) + 1;
  var c = from.position.col, r = from.position.row;
  var segs = [];
  if (!l.open){
    // A closed lane is a stub with no destination: it goes nowhere, and it says
    // so rather than pointing at a world it cannot reach.
    var sx, sy, ex, ey;
    if (north){ sx = laneX(c); sy = cellY(r,h); ex = sx; ey = sy - 46; }
    else { sx = cellX(c)+CW; sy = laneY(r,h); ex = sx + 46; ey = sy; }
    return {state:"closed", segs:[{d:"M "+sx+" "+sy+" L "+ex+" "+ey, head:false}],
            stopAt:{x:ex,y:ey}, wrap:false, steps:steps, vertical:north, edgeAt:null};
  }
  var to = byslot[l.toSlot];
  if (!to) return null;
  var state = steps > 1 ? "bypass" : "open";
  var tc = to.position.col, tr = to.position.row;
  if (north){
    var wrapN = (r + steps) >= h;
    var x = laneX(c);
    if (!wrapN){
      segs.push(arcSeg(x, cellY(r,h), x, cellY(tr,h)+CH, steps-1, true));
      return {state:state, segs:segs, wrap:false, steps:steps, vertical:true, edgeAt:null};
    }
    // The torus: it leaves through the top of the map and comes back in at the
    // bottom. Two segments, each with its own arrowhead, and a bar on the edge
    // it passed through — so the reader can see it did not stop.
    var topY = cellY(h-1,h), botY = cellY(0,h) + CH;
    segs.push(arcSeg(x, cellY(r,h), x, topY - WOUT, (h-1-r), true));
    segs.push(arcSeg(x, botY + WOUT, x, cellY(tr,h)+CH, tr, true));
    return {state:state, segs:segs, wrap:true, steps:steps, vertical:true,
            edgeAt:[{x:x, y:topY - WBAR, horiz:true, lbl:"to slot "+l.toSlot, dy:-12},
                    {x:x, y:botY + WBAR, horiz:true, lbl:"from slot "+l.fromSlot, dy:16}]};
  }
  var wrapE = (c + steps) >= w;
  var y = laneY(r,h);
  if (!wrapE){
    segs.push(arcSeg(cellX(c)+CW, y, cellX(tc), y, steps-1, false));
    return {state:state, segs:segs, wrap:false, steps:steps, vertical:false, edgeAt:null};
  }
  var rightX = cellX(w-1) + CW, leftX = cellX(0);
  segs.push(arcSeg(cellX(c)+CW, y, rightX + WOUT, y, (w-1-c), false));
  segs.push(arcSeg(leftX - WOUT, y, cellX(tc), y, tc, false));
  // Below the line: the rate label sits above it, and the two must not collide.
  return {state:state, segs:segs, wrap:true, steps:steps, vertical:false,
          edgeAt:[{x:rightX + WBAR, y:y, horiz:false, lbl:"to slot "+l.toSlot, dy:26},
                  {x:leftX - WBAR, y:y, horiz:false, lbl:"from slot "+l.fromSlot, dy:26}]};
}

/* arcSeg draws one segment of a lane. It BOWS when the segment has to pass over
   cells, which is exactly the bypass — the single thing a table cannot show. A
   segment that passes over nothing is a straight arrow. */
function arcSeg(x1,y1,x2,y2,over,vertical){
  if (over < 1) return {d:"M "+x1+" "+y1+" L "+x2+" "+y2, head:true};
  var lift = 30 + 14*(over-1);
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
    + '</defs>';

  // Which way is north, before a reader has to work it out from the arrows.
  s += '<text class="axis" x="8" y="15">north &uarr; · east &rarr;</text>';

  // Lanes first, cells over them: an arrow must never cover a number.
  var vertical = {}, lstate = {};
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
    s += '<text class="lanelbl'+(cls==="bypass"?" bypasslbl":(cls==="closed"?" closedlbl":""))
       + '" id="ll-'+esc(key)+'" x="0" y="0" text-anchor="'
       + (g.vertical?"start":"middle")+'"></text>';
    vertical[key] = !!g.vertical;
    lstate[key] = g.state;
    for (k=0;k<g.segs.length;k++){
      s += '<path class="lanehit" d="'+g.segs[k].d+'"><title>'+esc(title)+'</title></path>';
    }
    s += '</g>';
  }
  s += '</g><g id="pulses"></g><g id="celllayer">';

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
      s += '<g class="cell '+state+'" id="c-'+v.slot+'">'
        + '<rect class="cellbg" x="'+x+'" y="'+y+'" width="'+CW+'" height="'+CH+'" rx="10">'
        + '<title>'+esc(cellTitle(v))+'</title></rect>'
        + '<rect class="cellhit" x="'+(x+2)+'" y="'+(y+2)+'" width="'+(CW-4)+'" height="'+(CH-4)
        + '" rx="9"/>'
        + '<text class="slotno term" data-t="slot" x="'+(x+16)+'" y="'+(y+30)+'">slot '+v.slot+'</text>'
        + '<text class="pos" x="'+(x+16)+'" y="'+(y+48)+'">('+v.position.col+','+v.position.row
        + ') <tspan class="term" data-t="peer">'+esc(v.peerId)+'</tspan></text>'
        + '<text class="popnum" id="cpop-'+v.slot+'" text-anchor="end" x="'+(x+CW-16)+'" y="'
        + (y+32)+'"></text>'
        + '<circle class="statedot" cx="'+(x+CW-16-7*lbl.length-9)+'" cy="'+(y+44)+'" r="3.4"/>'
        + '<text class="statelbl" text-anchor="end" x="'+(x+CW-16)+'" y="'+(y+48)+'">'+lbl+'</text>'
        + '<g id="cdots-'+v.slot+'">';
      for (var di=0; di<DOTCAP; di++){
        var dx = x + 16 + (di % DOTCOLS)*12 + 5.5;
        var dy = y + 62 + Math.floor(di / DOTCOLS)*12 + 5.5;
        s += '<circle class="dot" id="cd-'+v.slot+'-'+di+'" cx="'+dx+'" cy="'+dy
          + '" r="4.2" style="display:none"/>';
      }
      s += '</g>'
        + '<text class="note" id="cnote-'+v.slot+'" x="'+(x+16)+'" y="'+(y+CH-11)+'"></text>'
        + '</g>';
    }
  }
  s += '</g></svg>';
  $("#mapbox").innerHTML = s;

  // The labels sit on the paths, which only exist once the SVG is in the DOM.
  anim = [];
  pulseLayer = document.getElementById("pulses");
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
      // Perpendicular to the lane, or the label lands on the arrow it describes.
      lab.setAttribute("x", mp.x + (vertical[kk] ? 22 : 0));
      lab.setAttribute("y", mp.y + (vertical[kk] ? 4 : -11));
    }
    if (l.open) anim.push({key:kk, tracks:tracks, total:total, rate:0, next:0, pulses:[]});
  }
  mapSig = signature(d);
}

function marker(id,color){
  return '<marker id="'+id+'" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6.5" '
    + 'markerHeight="6.5" orient="auto-start-reverse">'
    + '<path d="M 0 0 L 10 5 L 0 10 z" fill="'+color+'"/></marker>';
}

function cellTitle(v){
  var s = "slot "+v.slot+" ("+v.position.col+","+v.position.row+")  peer "+v.peerId+"\n"
    + (v.live ? (v.modConnected ? "live" : "connected, but no game attached") : "dark");
  if (!v.live && v.darkForMs != null) s += " for "+ms(v.darkForMs);
  s += "\npopulation " + (v.statsKnown && v.population!=null ? v.population : "unknown");
  if (v.statsKnown){
    s += "\ncustody "+(v.custodyDepth==null?"unknown":v.custodyDepth)
      +", paced "+(v.pacedDepth==null?"unknown":v.pacedDepth)
      +", held "+(v.heldDepth==null?"unknown":v.heldDepth);
    if (v.lastSave) s += "\nlast save "+ms(v.lastSaveAgeMs)+" ago";
  } else {
    s += "\nthis world has reported nothing recently"
      + (v.statsAgeMs ? " (last heard "+ms(v.statsAgeMs)+" ago)" : "");
  }
  return s;
}

function laneTitle(l, g){
  var s = "slot "+l.fromSlot+" "+(l.edge==="N"?"north":"east")+" lane\n";
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
    // One dot is one organism, until there are more organisms than dots.
    var unit = 1, shown = 0;
    if (n != null){
      unit = Math.max(1, Math.ceil(n / DOTCAP));
      shown = Math.min(DOTCAP, Math.ceil(n / unit));
    }
    for (var di=0; di<DOTCAP; di++){
      var c = document.getElementById("cd-"+v.slot+"-"+di);
      if (c) c.style.display = di < shown ? "" : "none";
    }
    var txt = "", cls = "note";
    if (!v.live && v.darkForMs != null){ txt = "dark for "+ms(v.darkForMs)+" — bypassed"; cls += " badt"; }
    else if (v.live && !v.modConnected){ txt = "no game attached"; cls += " badt"; }
    else if (!v.statsKnown){
      txt = "reported nothing" + (v.statsAgeMs ? " for "+ms(v.statsAgeMs) : ""); cls += " warnt";
    } else if (v.lastSave){
      txt = "saved "+ms(v.lastSaveAgeMs)+" ago";
      if (unit > 1) txt += " · 1 dot = "+unit;
    } else { txt = "no save receipt yet"; cls += " warnt"; }
    if (v.lastRefusal){ txt = v.lastRefusal; cls = "note badt"; }
    // The cell is 200 units wide; anything longer belongs in the tooltip and the
    // table below, not spilling over the world next door.
    if (txt.length > 29) txt = txt.slice(0,28)+"…";
    note.textContent = txt;
    note.setAttribute("class", cls);
  }

  var winMin = (d.flowWindowMs || 300000) / 60000;
  for (i=0;i<d.lanes.length;i++){
    var l = d.lanes[i], key = laneKey(l);
    var lab = document.getElementById("ll-"+key);
    if (lab){
      if (!l.open) lab.textContent = "closed: "+l.reason;
      else if ((l.skipped||[]).length) lab.textContent = "bypass ×"+l.skipped.length
        + " · " + rate(l.perMinute) + "/min";
      else lab.textContent = rate(l.perMinute)+"/min";
    }
    var a = null;
    for (var j=0;j<anim.length;j++) if (anim[j].key === key) a = anim[j];
    if (a){
      // One pulse is one organism, and the interval between pulses is the lane's
      // measured interval between hops. No floor that flatters a quiet lane: a
      // lane carrying one organism a minute shows one pulse a minute.
      var perMin = (l.recentHops!=null ? l.recentHops/winMin : l.perMinute) || 0;
      a.rate = perMin;
      a.gap = perMin > 0 ? Math.min(120000, Math.max(200, 60000/perMin)) : 0;
    }
    // A hop that ARRIVED between two polls gets its own, brighter pulse — and
    // pushes the steady one back, so the same organism is not drawn twice.
    var was = prevMig[key];
    if (was != null && l.migrations > was && a && !reduced){
      var extra = Math.min(4, l.migrations - was);
      for (var e=0;e<extra;e++) fireHot(a, e*130);
      if (a.gap > 0) a.next = Math.max(a.next, performance.now() + a.gap*extra);
      flashCell(l.toSlot);
    }
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

function spawn(a, hot){
  if (!pulseLayer || a.pulses.length > 7) return;
  var c = document.createElementNS(SVGNS, "circle");
  c.setAttribute("class", hot ? "pulse hot" : "pulse");
  c.setAttribute("r", hot ? 6.5 : 4.2);
  c.setAttribute("cx", -50); c.setAttribute("cy", -50);
  pulseLayer.appendChild(c);
  a.pulses.push({el:c, ti:0, dist:0, speed:hot?0.34:0.22});
}
function fireHot(a, delay){ setTimeout(function(){ spawn(a, true); }, delay); }

var lastTs = 0;
function frame(ts){
  rafId = requestAnimationFrame(frame);
  if (document.hidden) { lastTs = ts; return; }
  var dt = lastTs ? Math.min(ts - lastTs, 80) : 16;
  lastTs = ts;
  for (var i=0;i<anim.length;i++){
    var a = anim[i];
    if (a.rate > 0 && ts >= a.next){
      spawn(a, false);
      a.next = ts + a.gap*(0.65 + 0.7*Math.random());
    }
    for (var j=a.pulses.length-1;j>=0;j--){
      var p = a.pulses[j];
      p.dist += p.speed*dt;
      var tr = a.tracks[p.ti];
      while (tr && p.dist > tr.len){ p.dist -= tr.len; p.ti++; tr = a.tracks[p.ti]; }
      if (!tr){ if (p.el.parentNode) p.el.parentNode.removeChild(p.el); a.pulses.splice(j,1); continue; }
      var pt = tr.el.getPointAtLength(p.dist);
      p.el.setAttribute("cx", pt.x); p.el.setAttribute("cy", pt.y);
    }
  }
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

/* ------------------------------------------------------------------- polling */
function render(d){
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
    return "<tr><td>"+v.slot+"</td><td>("+v.position.col+","+v.position.row+")</td>"
      + "<td>"+esc(v.peerId)+"</td><td>"+state+"</td>"
      + '<td class="num">'+num(v.population)+'</td>'
      + '<td class="num">'+num(v.custodyDepth)+'</td>'
      + '<td class="num">'+num(v.pacedDepth)+'</td>'
      + '<td class="num">'+num(v.heldDepth)+'</td>'
      + '<td class="num">'+num(v.bouncedTimeoutTotal)+'</td>'
      + "<td>"+save+"</td><td>"+note+"</td></tr>";
  }).join("") || '<tr><td colspan="11" class="muted">no slots reserved yet</td></tr>';

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
    + kv(t("envelope","envelopes recorded"),
         tt.migrations+"  ("+tt.perMinute.toFixed(2)+"/min over the last "
         + Math.round((d.flowWindowMs||300000)/60000)+" min)")
    + kv(t("genomegap","genome gaps"), d.genomeGaps)
    + kv("ledger records", d.ledgerRecords);
}

async function tick(){
  try {
    var r = await fetch("api/status", {cache:"no-store"});
    render(await r.json());
  } catch(e){
    $("#link").innerHTML = '<span class="bad">status endpoint unreachable</span>';
  }
}
async function tickHistory(){
  try {
    var r = await fetch("api/history?hours=24&buckets=120", {cache:"no-store"});
    renderHistory(await r.json());
  } catch(e){
    $("#spark").innerHTML = '<span class="bad">history endpoint unreachable</span>';
  }
}

try { reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches; } catch(e){}
tick(); setInterval(tick, 2000);
tickHistory(); setInterval(tickHistory, 60000);
if (!reduced) rafId = requestAnimationFrame(frame);
</script>
</body>
</html>
`
