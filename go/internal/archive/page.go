package archive

import (
	"encoding/json"
	"net/http"
)

// The status page is SELF-CONTAINED HTML with inline JS that polls one JSON
// endpoint. No CDN, no build step, no framework: the page has to work on a LAN
// with no internet, opened from a browser on either machine, and it has to keep
// working when the archive is the only thing still running.

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
--dim:#8b95a3;--live:#4ec9a0;--dark:#e05561;--hole:#3a424c;--warn:#e2b93b;--lane:#5aa9e6}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);
font:14px/1.45 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
header{padding:14px 18px;border-bottom:1px solid var(--line);display:flex;
gap:18px;align-items:baseline;flex-wrap:wrap}
h1{font-size:15px;margin:0;letter-spacing:.08em;text-transform:uppercase}
.muted{color:var(--dim)}
main{padding:18px;display:grid;gap:18px}
section{background:var(--panel);border:1px solid var(--line);border-radius:6px;padding:14px}
h2{font-size:12px;margin:0 0 10px;letter-spacing:.12em;text-transform:uppercase;color:var(--dim)}
#grid{display:grid;gap:10px}
.cell{border:1px solid var(--line);border-radius:5px;padding:10px;min-height:104px;
background:#14181d;position:relative}
.cell.live{border-color:var(--live)}
.cell.dark{border-color:var(--dark)}
.cell.hole{border-style:dashed;color:var(--dim);background:transparent}
.slotno{font-size:17px;font-weight:700}
.pos{color:var(--dim);font-size:11px}
.pill{display:inline-block;font-size:10px;padding:1px 6px;border-radius:9px;
border:1px solid var(--line);margin-left:6px;vertical-align:middle}
.pill.live{color:var(--live);border-color:var(--live)}
.pill.dark{color:var(--dark);border-color:var(--dark)}
.kv{display:flex;justify-content:space-between;gap:8px;font-size:12px}
.kv span:last-child{color:var(--text)}
.unknown{color:var(--warn);font-style:italic}
table{width:100%;border-collapse:collapse;font-size:12px}
th,td{text-align:left;padding:4px 8px;border-bottom:1px solid var(--line)}
th{color:var(--dim);font-weight:400;letter-spacing:.06em}
td.num{text-align:right;font-variant-numeric:tabular-nums}
.closed{color:var(--dark)}
.open{color:var(--lane)}
.skip{color:var(--warn);font-size:11px}
.bad{color:var(--dark)}
footer{padding:10px 18px;color:var(--dim);font-size:11px;border-top:1px solid var(--line)}
</style>
</head>
<body>
<header>
  <h1>multiverse map</h1>
  <span class="muted" id="shape">&hellip;</span>
  <span class="muted" id="link"></span>
  <span class="muted" id="age"></span>
</header>
<main>
  <section><h2>the map</h2><div id="grid"></div></section>
  <section><h2>effective lanes &mdash; derived from PEER_STATUS by the walk of &sect;8, for display</h2>
    <table id="lanes"><thead><tr><th>from</th><th>edge</th><th>to</th><th>state</th>
    <th class="num">envelopes</th><th class="num">/min</th><th>bypassing</th></tr></thead>
    <tbody></tbody></table></section>
  <section><h2>totals</h2><div id="totals"></div></section>
</main>
<footer>
  One source, no polling of anybody: everything here comes from the PEER_STATUS
  broadcasts this archive receives as a read-only subscriber and from the envelope
  copies it records. The relay's SECTOR_GRANT is the authority for a peer's actual
  routing; these lanes are recomputed for display. An <span class="unknown">unknown</span>
  is a slot that reported nothing, or reported it too long ago &mdash; never a zero.
</footer>
<script>
const $ = s => document.querySelector(s);
function ms(v){ if(v==null) return "unknown";
  const s = Math.floor(v/1000); if (s<60) return s+"s";
  const m = Math.floor(s/60); if (m<60) return m+"m "+(s%60)+"s";
  const h = Math.floor(m/60); return h+"h "+(m%60)+"m"; }
function num(v){ return v==null ? '<span class="unknown">unknown</span>' : v; }
function kv(k,v){ return '<div class="kv"><span class="muted">'+k+'</span><span>'+v+'</span></div>'; }

function render(d){
  $("#shape").textContent = d.haveStatus
    ? d.map.width+"×"+d.map.height+"  ·  "+d.slotCount+" slots  ·  "
      +d.holes.length+" hole(s)  ·  epoch "+d.epoch
    : "no PEER_STATUS yet";
  $("#link").innerHTML = d.relayConnected
    ? '<span style="color:var(--live)">relay linked</span>'
    : '<span class="bad">relay link DOWN</span>';
  $("#age").textContent = d.haveStatus ? "state "+ms(d.statusAgeMs)+" old" : "";

  const g = $("#grid");
  g.style.gridTemplateColumns = "repeat("+Math.max(d.map.width,1)+",minmax(150px,1fr))";
  const at = {}; d.slots.forEach(s => at[s.position.col+","+s.position.row] = s);
  let cells = "";
  for (let row = d.map.height-1; row >= 0; row--){
    for (let col = 0; col < d.map.width; col++){
      const s = at[col+","+row];
      if (!s){ cells += '<div class="cell hole"><div class="slotno">hole</div>'
        +'<div class="pos">('+col+','+row+')</div>'
        +'<div class="muted" style="font-size:11px">both axes route around it</div></div>'; continue; }
      let body = '<div class="slotno">slot '+s.slot
        +'<span class="pill '+(s.live?'live':'dark')+'">'+(s.live?'live':'dark')+'</span></div>'
        +'<div class="pos">('+s.position.col+','+s.position.row+') '+s.peerId+'</div>';
      if (!s.live && s.darkForMs != null)
        body += '<div class="bad" style="font-size:11px">bypassed for '+ms(s.darkForMs)+'</div>';
      if (!s.modConnected && s.live)
        body += '<div class="bad" style="font-size:11px">no mod: not deliverable</div>';
      if (s.lastRefusal) body += '<div class="bad" style="font-size:11px">'+s.lastRefusal+'</div>';
      if (!s.statsKnown){
        body += '<div class="unknown" style="font-size:11px">stats unknown'
          + (s.statsAgeMs? " ("+ms(s.statsAgeMs)+" old)" : "") + '</div>';
      } else {
        body += kv("population", num(s.population))
          + kv("custody", num(s.custodyDepth))
          + kv("paced", num(s.pacedDepth))
          + kv("held", num(s.heldDepth))
          + kv("timeout bounces", num(s.bouncedTimeoutTotal))
          + kv("last save", s.lastSave ? ms(s.lastSaveAgeMs)+" ago" :
              '<span class="unknown">unknown</span>');
      }
      cells += '<div class="cell '+(s.live?'live':'dark')+'">'+body+'</div>';
    }
  }
  g.innerHTML = cells || '<div class="muted">the map is empty</div>';

  $("#lanes tbody").innerHTML = d.lanes.map(l => {
    const skips = (l.skipped||[]).map(k =>
      k.slot==null ? "hole("+k.position.col+","+k.position.row+")"
                   : "slot "+k.slot+":"+k.reason).join(", ");
    return "<tr><td>slot "+l.fromSlot+"</td><td>"+l.edge+"</td>"
      + "<td>"+(l.open ? "slot "+l.toSlot : "—")+"</td>"
      + '<td class="'+(l.open?"open":"closed")+'">'+(l.open?"open":"closed: "+l.reason)+"</td>"
      + '<td class="num">'+l.migrations+'</td>'
      + '<td class="num">'+l.perMinute.toFixed(2)+'</td>'
      + '<td class="skip">'+(skips||"—")+"</td></tr>";
  }).join("") || '<tr><td colspan="7" class="muted">no declared export edges yet</td></tr>';

  const t = d.totals;
  $("#totals").innerHTML =
      kv("live / dark / holes", t.liveSlots+" / "+t.darkSlots+" / "+t.holes)
    + kv("population (known slots)", num(t.population))
    + kv("custody depth", num(t.custodyDepth))
    + kv("paced depth", num(t.pacedDepth))
    + kv("held depth", num(t.heldDepth))
    + kv("bounces a hold timeout caused", num(t.timeoutBounces))
    + kv("slots reporting nothing", t.unknownSlots)
    + kv("envelopes recorded", t.migrations+"  ("+t.perMinute.toFixed(2)+"/min)")
    + kv("genome gaps", d.genomeGaps);
}

async function tick(){
  try { render(await (await fetch("api/status",{cache:"no-store"})).json()); }
  catch(e){ $("#link").innerHTML = '<span class="bad">status endpoint unreachable</span>'; }
}
tick(); setInterval(tick, 2000);
</script>
</body>
</html>
`
