package archive

// The production-health page is self-contained for the same reason the live
// map is: no CDN, build step, font service or client framework can become a new
// dependency of the surface used to decide whether dependencies are healthy.
const healthPageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Bibites Multiverse — Production Health</title>
<meta name="description" content="Live production checks, world health, service-host performance, archive integrity, and observability coverage for Bibites Multiverse.">
<meta name="theme-color" content="#0b1110">
<link rel="canonical" href="https://bibitesmultiverse.com/health">
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<style>
:root{color-scheme:dark;--bg:#0b1110;--surface:#111a18;--surface2:#16221f;--surface3:#1a2925;
--line:#294038;--text:#eff7f3;--muted:#9aafa7;--green:#66e0ac;--blue:#75bdf2;
--amber:#efbd57;--red:#e86c76;--ink:#07100d;--max:1240px}
*{box-sizing:border-box}html{scroll-behavior:smooth}body{margin:0;background:
radial-gradient(circle at 88% 3%,rgba(67,160,126,.16),transparent 30rem),
radial-gradient(circle at 5% 26%,rgba(62,126,174,.1),transparent 28rem),var(--bg);
color:var(--text);font:15px/1.55 ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}
a{color:inherit}.skip{position:fixed;left:12px;top:-80px;z-index:100;background:var(--text);
color:var(--ink);padding:9px 13px;border-radius:7px}.skip:focus{top:12px}.shell{width:min(calc(100% - 40px),var(--max));margin-inline:auto}
.sitehd{position:sticky;top:0;z-index:30;background:rgba(11,17,16,.88);backdrop-filter:blur(16px);
border-bottom:1px solid rgba(83,120,107,.28)}.nav{min-height:70px;display:flex;align-items:center;justify-content:space-between;gap:24px}
.brand{display:inline-flex;align-items:center;gap:10px;text-decoration:none;font-weight:760;letter-spacing:-.02em}.mark{width:30px;height:22px;color:var(--green)}
.links{display:flex;align-items:center;gap:20px;color:var(--muted);font-size:14px}.links a{text-decoration:none}.links a:hover,.links a:focus-visible{color:var(--text)}
.livepill{display:inline-flex;align-items:center;gap:7px!important;padding:7px 11px;border:1px solid var(--line);border-radius:999px;color:var(--text)!important;background:rgba(22,34,31,.75)}
.dot{display:inline-block;width:8px;height:8px;border-radius:50%;background:var(--muted);box-shadow:0 0 0 4px rgba(154,175,167,.08)}
.dot.pass{background:var(--green);box-shadow:0 0 0 4px rgba(102,224,172,.1)}.dot.warning{background:var(--amber);box-shadow:0 0 0 4px rgba(239,189,87,.1)}
.dot.critical{background:var(--red);box-shadow:0 0 0 4px rgba(232,108,118,.1)}
.hero{padding:70px 0 32px;display:grid;grid-template-columns:minmax(0,1fr) minmax(340px,.72fr);gap:72px;align-items:end}
.eyebrow{margin:0 0 14px;color:var(--green);font:700 12px/1.2 ui-monospace,SFMono-Regular,Menlo,monospace;letter-spacing:.13em;text-transform:uppercase}
h1{font-size:clamp(48px,7vw,82px);line-height:.95;letter-spacing:-.055em;margin:0}.lede{max-width:760px;color:#bfd0c9;font-size:18px;margin:24px 0 0}
.overall{border:1px solid var(--line);border-radius:18px;background:linear-gradient(145deg,rgba(22,34,31,.96),rgba(11,18,16,.92));padding:24px;box-shadow:0 24px 60px rgba(0,0,0,.2)}
.overalltop{display:flex;align-items:center;gap:12px}.overalltop .dot{width:11px;height:11px}.overall h2{margin:0;font-size:23px;letter-spacing:-.025em}.overall p{color:var(--muted);margin:10px 0 0}
.summary{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;padding:26px 0 8px}.stat{border:1px solid var(--line);border-radius:13px;background:rgba(17,26,24,.86);padding:18px}
.stat span{display:block;color:var(--muted);font-size:12px;text-transform:uppercase;letter-spacing:.08em}.stat b{display:block;margin-top:7px;font-size:24px;letter-spacing:-.035em}.stat small{display:block;color:var(--muted);margin-top:3px}
.section{padding:46px 0 0}.section:last-of-type{padding-bottom:64px}.sectionhd{display:flex;justify-content:space-between;align-items:end;gap:24px;margin-bottom:18px}
.sectionhd h2{font-size:30px;letter-spacing:-.035em;margin:0}.sectionhd p{max-width:620px;color:var(--muted);margin:0;text-align:right}.grid{display:grid;grid-template-columns:1fr 1fr;gap:16px}
.card{border:1px solid var(--line);border-radius:16px;background:rgba(17,26,24,.9);padding:22px;min-width:0}.card.full{grid-column:1/-1}.card h3{margin:0;font-size:19px;letter-spacing:-.02em}
.sub{color:var(--muted);font-size:13px;margin:5px 0 0}.checks{display:grid;grid-template-columns:repeat(3,1fr);gap:12px}.checkgroup{border:1px solid rgba(83,120,107,.28);border-radius:12px;background:rgba(22,34,31,.56);padding:15px}
.checkgroup h3{font-size:12px;text-transform:uppercase;letter-spacing:.1em;color:var(--muted);margin:0 0 8px}.check{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:8px 0;border-top:1px solid rgba(83,120,107,.2)}
.check:first-of-type{border-top:0}.checkname{display:flex;align-items:center;gap:9px;min-width:0}.checkname span:last-child{overflow:hidden;text-overflow:ellipsis}.pill{display:inline-flex;align-items:center;justify-content:center;border:1px solid var(--line);border-radius:999px;padding:3px 8px;color:var(--muted);font:700 10px/1.3 ui-monospace,SFMono-Regular,Menlo,monospace;text-transform:uppercase;letter-spacing:.06em}
.pill.pass{color:var(--green);border-color:rgba(102,224,172,.35)}.pill.warning{color:var(--amber);border-color:rgba(239,189,87,.35)}.pill.critical{color:var(--red);border-color:rgba(232,108,118,.4)}
.metrics{display:grid;grid-template-columns:repeat(4,1fr);gap:10px;margin-top:18px}.metric{padding:13px;border-radius:10px;background:var(--surface2);border:1px solid rgba(83,120,107,.24)}
.metric span{display:block;color:var(--muted);font-size:11px;text-transform:uppercase;letter-spacing:.07em}.metric b{display:block;font-size:19px;margin-top:5px}.charts{display:grid;grid-template-columns:repeat(3,1fr);gap:12px;margin-top:16px}
.chart{border:1px solid rgba(83,120,107,.28);border-radius:12px;background:#0d1513;padding:13px;min-height:174px}.charthead{display:flex;justify-content:space-between;gap:10px}.charthead b{font-size:13px}.charthead span{color:var(--muted);font-size:12px}.chart svg{display:block;width:100%;height:112px;margin-top:8px}.chart .empty{display:grid;place-items:center;height:112px;color:var(--muted);font-size:13px}
.worlds,.units{margin-top:14px}.row{display:grid;grid-template-columns:minmax(120px,1fr) repeat(3,minmax(86px,.55fr));gap:12px;align-items:center;padding:11px 0;border-top:1px solid rgba(83,120,107,.22)}
.row.head{color:var(--muted);font-size:11px;text-transform:uppercase;letter-spacing:.07em}.row b{font-size:14px}.row span:not(.pill){color:#c5d4cf}.units .row{grid-template-columns:minmax(120px,1fr) repeat(5,minmax(76px,.62fr))}
.coverage{width:100%;border-collapse:collapse;margin-top:8px}.coverage th,.coverage td{text-align:left;padding:12px 10px;border-top:1px solid rgba(83,120,107,.25);vertical-align:top}.coverage th{color:var(--muted);font-size:11px;text-transform:uppercase;letter-spacing:.07em}.coverage td:last-child{color:var(--muted)}
.truth{border-left:3px solid var(--blue);background:rgba(117,189,242,.07);padding:15px 17px;border-radius:0 10px 10px 0;color:#c5d4cf;margin-top:16px}.truth strong{color:var(--text)}
footer{border-top:1px solid var(--line);color:var(--muted)}.foot{min-height:92px;display:flex;align-items:center;justify-content:space-between;gap:24px}.footlinks{display:flex;flex-wrap:wrap;gap:18px}.foot a{text-decoration:none}.foot a:hover{color:var(--text)}
@media(max-width:980px){.hero{grid-template-columns:1fr;gap:28px}.summary{grid-template-columns:1fr 1fr}.checks{grid-template-columns:1fr 1fr}.charts{grid-template-columns:1fr}.links a:not(.livepill){display:none}}
@media(max-width:680px){.shell{width:min(calc(100% - 24px),var(--max))}.hero{padding-top:46px}h1{font-size:49px}.summary,.grid,.checks,.metrics{grid-template-columns:1fr}.sectionhd{display:block}.sectionhd p{text-align:left;margin-top:8px}.card{padding:17px}.row,.units .row{grid-template-columns:1fr 1fr}.row.head{display:none}.coverage{display:block;overflow-x:auto}.foot{align-items:flex-start;flex-direction:column;padding:25px 0}.card.full{grid-column:auto}}
</style>
</head>
<body>
<a class="skip" href="#main">Skip to health results</a>
<header class="sitehd"><div class="shell nav">
  <a class="brand" href="/" aria-label="Bibites Multiverse home"><svg class="mark" viewBox="-9 -7 19 14" aria-hidden="true"><path fill="currentColor" d="M4.5-1.26 2.16 0l2.34 1.26C4.14 3.06 1.98 4.14-.54 4.14-3.24 4.14-5.58 2.16-6.66 0-5.58-2.16-3.24-4.14-.54-4.14c2.52 0 4.68 1.08 5.04 2.88Z"/><circle cx="1.5" cy="-1.85" r="1" fill="#07100d"/></svg><span>Bibites Multiverse</span></a>
  <nav class="links" aria-label="Primary navigation"><a href="/#how">How it works</a><a href="/#join">Join</a><a href="/watch">Watch broadcast</a><a href="/announcements/">Announcements</a><a href="https://github.com/jpinedaa/bibites-multiverse">GitHub</a><a class="livepill" href="/health" aria-current="page"><i class="dot" id="navdot"></i>Health</a></nav>
</div></header>
<main id="main">
  <section class="shell hero">
    <div><p class="eyebrow">Live production evidence</p><h1>System health,<br>without the guesswork.</h1><p class="lede">Current automated checks, every world&rsquo;s three-state connection, two hours of service-host performance, archive integrity, and the gaps we still cannot see from one place.</p></div>
    <aside class="overall" aria-live="polite"><div class="overalltop"><i class="dot" id="overall-dot"></i><h2 id="overall-title">Loading live results&hellip;</h2></div><p id="overall-copy">Missing and stale measurements stay unknown. They never become a green zero.</p></aside>
  </section>
  <section class="shell summary" aria-label="Health summary">
    <div class="stat"><span>Automated checks</span><b id="sum-checks">&hellip;</b><small id="sum-checks-note">five-minute cadence</small></div>
    <div class="stat"><span>Worlds running</span><b id="sum-worlds">&hellip;</b><small id="sum-worlds-note">sidecar + game + fresh stats</small></div>
    <div class="stat"><span>Service-host sample</span><b id="sum-host">&hellip;</b><small>one-minute cadence</small></div>
    <div class="stat"><span>Archive record</span><b id="sum-ledger">&hellip;</b><small id="sum-ledger-note">durable crossings</small></div>
  </section>

  <section class="shell section" aria-labelledby="checks-title"><div class="sectionhd"><div><p class="eyebrow">Alerting layer</p><h2 id="checks-title">Automated service checks</h2></div><p id="checks-age">Waiting for the scheduled monitor&rsquo;s completed-pass marker.</p></div><div class="card full"><div id="checks" class="checks"><p class="sub">No completed check pass is available yet.</p></div></div></section>

  <section class="shell section" aria-labelledby="world-title"><div class="sectionhd"><div><p class="eyebrow">Service layer</p><h2 id="world-title">Worlds, routing, and archive</h2></div><p>Live, sidecar-only, dark, and unknown remain separate states. Structural holes are shown beside bypassed lanes.</p></div>
    <div class="grid"><article class="card"><h3>World processes</h3><p class="sub" id="world-note">Reading the current relay status.</p><div class="metrics"><div class="metric"><span>Live sidecars</span><b id="m-live">&hellip;</b></div><div class="metric"><span>Games attached</span><b id="m-mods">&hellip;</b></div><div class="metric"><span>Fresh stats</span><b id="m-known">&hellip;</b></div><div class="metric"><span>Dark slots</span><b id="m-dark">&hellip;</b></div></div><div id="worlds" class="worlds"></div></article>
      <article class="card"><h3>Routing and record</h3><p class="sub">Current map state and the archive&rsquo;s measured restart and retention layer.</p><div class="metrics"><div class="metric"><span>Bypassed lanes</span><b id="m-bypass">&hellip;</b></div><div class="metric"><span>Structural holes</span><b id="m-holes">&hellip;</b></div><div class="metric"><span>Genome gaps</span><b id="m-gaps">&hellip;</b></div><div class="metric"><span>Cold-copy wait</span><b id="m-cold">&hellip;</b></div><div class="metric"><span>Replay last start</span><b id="m-replay">&hellip;</b></div><div class="metric"><span>Roll-up age</span><b id="m-rollup">&hellip;</b></div><div class="metric"><span>Roll-up covered</span><b id="m-covered">&hellip;</b></div><div class="metric"><span>Duplicates refused</span><b id="m-duplicates">&hellip;</b></div><div class="metric"><span>Raw record</span><b id="m-raw">&hellip;</b></div><div class="metric"><span>Closed segments</span><b id="m-segments">&hellip;</b></div><div class="metric"><span>Retired segments</span><b id="m-retired">&hellip;</b></div><div class="metric"><span>Skipped lines</span><b id="m-skipped">&hellip;</b></div></div></article>
    </div>
  </section>

  <section class="shell section" aria-labelledby="host-title"><div class="sectionhd"><div><p class="eyebrow">Performance layer</p><h2 id="host-title">Service-host profile</h2></div><p id="host-age">The host sampler records once a minute. Charts show at most the latest two hours.</p></div>
    <div class="card full"><div class="metrics"><div class="metric"><span>CPU busy</span><b id="h-cpu">&hellip;</b></div><div class="metric"><span>Memory available</span><b id="h-memory">&hellip;</b></div><div class="metric"><span>Disk used</span><b id="h-disk">&hellip;</b></div><div class="metric"><span>Load 1 / 5 / 15</span><b id="h-load">&hellip;</b></div><div class="metric"><span>Swap used</span><b id="h-swap">&hellip;</b></div><div class="metric"><span>Connections</span><b id="h-connections">&hellip;</b></div><div class="metric"><span>TCP resets in window</span><b id="h-resets">&hellip;</b></div><div class="metric"><span>Retransmits in window</span><b id="h-retrans">&hellip;</b></div><div class="metric"><span>Connection failures</span><b id="h-attempts">&hellip;</b></div><div class="metric"><span>Listen drops / overflows</span><b id="h-listen">&hellip;</b></div><div class="metric"><span>SYN retransmits</span><b id="h-syn">&hellip;</b></div><div class="metric"><span>Conntrack used</span><b id="h-conntrack">&hellip;</b></div></div>
      <div class="charts"><div class="chart"><div class="charthead"><b>CPU busy</b><span id="chart-cpu-label">unknown</span></div><div id="chart-cpu"></div></div><div class="chart"><div class="charthead"><b>Memory available</b><span id="chart-memory-label">unknown</span></div><div id="chart-memory"></div></div><div class="chart"><div class="charthead"><b>Archive anonymous memory</b><span id="chart-anon-label">unknown</span></div><div id="chart-anon"></div></div></div>
      <div id="units" class="units"></div>
    </div>
  </section>

  <section class="shell section" aria-labelledby="broadcast-title"><div class="sectionhd"><div><p class="eyebrow">Broadcast path</p><h2 id="broadcast-title">Audience and stream signal</h2></div><p>The audience signal decides whether the remote publisher runs. It is a control measurement, not a synthetic playback test.</p></div><div class="grid"><article class="card"><h3 id="viewer-title">Checking viewer presence</h3><p class="sub" id="viewer-copy">Reading the ten-second presence document.</p><div class="metrics"><div class="metric"><span>HLS sessions</span><b id="v-sessions">&hellip;</b></div><div class="metric"><span>Last request</span><b id="v-request">&hellip;</b></div></div></article><article class="card"><h3>What this result does not prove</h3><p class="sub">An idle room and a publisher that cannot start both have zero HLS sessions. A viewer request exercises the start control, but there is no continuous external probe that waits for playable video.</p><div class="truth"><strong>Honest boundary:</strong> current audience and map state are centralized; end-to-end start-to-play success is not.</div></article></div></section>

  <section class="shell section" aria-labelledby="coverage-title"><div class="sectionhd"><div><p class="eyebrow">Coverage map</p><h2 id="coverage-title">What is and is not centralized</h2></div><p>A dashboard is useful only if it names the evidence it cannot show.</p></div><div class="card full"><table class="coverage"><thead><tr><th>Instrument</th><th>Cadence</th><th>Dashboard state</th><th>Boundary</th></tr></thead><tbody>
    <tr><td>World and archive status</td><td>live poll</td><td><span class="pill" id="cov-status">unknown</span></td><td>Current state and durable archive counters are centralized.</td></tr>
    <tr><td>Scheduled service checks</td><td>5 minutes</td><td><span class="pill" id="cov-monitor">unknown</span></td><td>Verdicts are public; alert text, costs, host paths, and identities are not.</td></tr>
    <tr><td>Service-host performance</td><td>1 minute</td><td><span class="pill" id="cov-host">unknown</span></td><td>The latest two hours are shown. The retained JSONL remains on the host.</td></tr>
    <tr><td>Broadcast audience signal</td><td>10 seconds</td><td><span class="pill" id="cov-viewers">unknown</span></td><td>Presence is centralized; successful publisher startup and playback are not synthesized.</td></tr>
    <tr><td>Off-host production verification</td><td>daily and per deploy</td><td><span class="pill">not ingested</span></td><td>The private operations workflow verifies deployed state. Its result does not feed this public host.</td></tr>
    <tr><td>Deployment health windows</td><td>on demand</td><td><span class="pill">receipt only</span></td><td>Numeric before-and-after samples live with each deployment receipt, not in a continuous feed.</td></tr>
    <tr><td>Provider metrics and cost audit</td><td>5 minutes to daily</td><td><span class="pill">verdict only</span></td><td>The monitor publishes safe severities. Resource identity, costs, and raw provider readings remain private.</td></tr>
    <tr><td>World-host performance and PSI</td><td>5 minutes</td><td><span class="pill">local only</span></td><td>Recorded on the world host. No live feed reaches this service yet.</td></tr>
    <tr><td>Service-host pressure stalls</td><td>not sampled</td><td><span class="pill">missing</span></td><td>CPU, memory, disk, units, and TCP counters are sampled here; PSI is not.</td></tr>
    <tr><td>External front-door path probe</td><td>not installed</td><td><span class="pill">missing</span></td><td>No off-host dead-man or continuous TLS path series exists yet.</td></tr>
    <tr><td>Continuous CPU / heap profiling</td><td>not installed</td><td><span class="pill">missing</span></td><td>There is sampled utilization, memory, pressure on the world host, and counters; no continuous profiler.</td></tr>
  </tbody></table><div class="truth"><strong>Freshness is part of every result.</strong> A stale collector becomes unknown even when its last recorded verdict was green. This page is a read-only projection and does not replace the off-host evidence store still called for by the observability standard.</div></div></section>
</main>
<footer><div class="shell foot"><span>Independent community project · Apache-2.0</span><div class="footlinks"><a href="/announcements/">Announcements</a><a href="/watch">Watch broadcast</a><a href="/live">Live map</a><a href="/health">Production health</a><a href="https://github.com/jpinedaa/bibites-multiverse">Source</a></div></div></footer>
<script>
(function(){
  var HEALTH=null, STATUS=null, VIEWERS=null, FAIL={health:false,status:false,viewers:false};
  function by(id){return document.getElementById(id)}
  function text(id,v){by(id).textContent=v}
  function num(v){return v==null?"unknown":Number(v).toLocaleString()}
  function fixed(v,n){return v==null?"unknown":Number(v).toFixed(n)}
  function bytes(v){if(v==null)return "unknown";var u=["B","KiB","MiB","GiB","TiB"],i=0,x=Number(v);while(x>=1024&&i<u.length-1){x/=1024;i++}return (x>=100||i===0?x.toFixed(0):x.toFixed(1))+" "+u[i]}
  function duration(s){if(s==null)return "unknown";s=Math.max(0,Math.round(s));if(s<60)return s+"s";if(s<3600)return Math.floor(s/60)+"m";if(s<86400)return (s/3600).toFixed(1)+"h";return (s/86400).toFixed(1)+"d"}
  function isoAge(v){var at=v&&Date.parse(v);if(!isFinite(at))return "unknown";return duration((Date.now()-at)/1000)+" ago"}
  function viewersFresh(){var at=VIEWERS&&Date.parse(VIEWERS.asOf),age=Date.now()-at;return isFinite(at)&&age>=-30000&&age<30000}
  function pill(node,state,label){node.className="pill"+(state?" "+state:"");node.textContent=label||state||"unknown"}
  function dot(node,state){node.className="dot"+(state?" "+state:"")}
  async function get(url){var r=await fetch(url,{cache:"no-store"});if(!r.ok)throw new Error(url+" "+r.status);return r.json()}
  function resultState(){
    if(FAIL.health||FAIL.status||FAIL.viewers||!HEALTH||!STATUS||!VIEWERS)return "unknown";
    var m=HEALTH.monitor,h=HEALTH.serviceHost,t=STATUS.totals||{};
    if(!STATUS.relayConnected||(m.freshness==="fresh"&&m.overall==="critical"))return "critical";
    if(!STATUS.haveStatus||STATUS.statusAgeMs>30000||!m.available||m.freshness!=="fresh"||!h.available||h.freshness!=="fresh"||!viewersFresh())return "unknown";
    var attached=(STATUS.slots||[]).filter(function(s){return s.live&&s.modConnected}).length;
    if(m.overall==="warning"||(t.darkSlots||0)>0||attached<(t.liveSlots||0))return "warning";
    return m.overall==="pass"?"pass":"unknown";
  }
  function renderOverall(){
    var state=resultState(),titles={pass:"All centralized signals are healthy",warning:"Production has active warnings",critical:"Production has a critical result",unknown:"Production health is incomplete"};
    dot(by("overall-dot"),state);dot(by("navdot"),state);text("overall-title",titles[state]);
    var copy=state==="unknown"?"One or more required feeds are missing or stale. The last green result is not being reused.":state==="pass"?"The scheduled monitor passed, host telemetry is fresh, and current worlds have no degraded state.":"Open the failing checks and world states below. Threshold decisions come from the monitor, not from this page.";
    text("overall-copy",copy);
  }
  function renderMonitor(){
    var m=HEALTH&&HEALTH.monitor;if(!m||!m.available){text("sum-checks","unknown");text("checks-age","No completed scheduled monitor pass is available.");by("checks").innerHTML='<p class="sub">The monitor result is unavailable. No previous green result is substituted.</p>';pill(by("cov-monitor"),"unknown");renderOverall();return}
    var counts={pass:0,warning:0,critical:0,unknown:0};(m.checks||[]).forEach(function(c){counts[c.severity]=(counts[c.severity]||0)+1});
    text("sum-checks",counts.pass+" / "+m.checks.length+" pass");text("sum-checks-note",counts.warning+" warning · "+counts.critical+" critical · "+counts.unknown+" unknown");
    text("checks-age",(m.freshness==="stale"?"Stale result from ":"Completed ")+isoAge(m.asOf)+" · five-minute cadence");pill(by("cov-monitor"),m.freshness==="fresh"?m.overall:"unknown",m.freshness);
    var groups={},order=[];(m.checks||[]).forEach(function(c){if(!groups[c.group]){groups[c.group]=[];order.push(c.group)}groups[c.group].push(c)});
    var root=by("checks");root.textContent="";order.forEach(function(g){var box=document.createElement("section");box.className="checkgroup";var h=document.createElement("h3");h.textContent=g;box.appendChild(h);groups[g].forEach(function(c){var row=document.createElement("div");row.className="check";var name=document.createElement("div");name.className="checkname";var d=document.createElement("i");dot(d,c.severity);var label=document.createElement("span");label.textContent=c.label;name.appendChild(d);name.appendChild(label);var p=document.createElement("span");pill(p,c.severity);row.appendChild(name);row.appendChild(p);box.appendChild(row)});root.appendChild(box)});
    renderOverall();
  }
  function renderStatus(){
    if(!STATUS){text("sum-worlds","unknown");text("sum-ledger","unknown");pill(by("cov-status"),"unknown");renderOverall();return}
    var slots=STATUS.slots||[],t=STATUS.totals||{},mods=slots.filter(function(s){return s.live&&s.modConnected}).length,known=slots.filter(function(s){return s.statsKnown}).length,running=slots.filter(function(s){return s.live&&s.modConnected&&s.statsKnown}).length;
    text("sum-worlds",running+" / "+slots.length);text("sum-worlds-note",mods+" games attached · "+known+" fresh stats");text("sum-ledger",num(STATUS.ledgerRecords));text("sum-ledger-note",STATUS.ledgerSkippedLines?num(STATUS.ledgerSkippedLines)+" skipped lines":"no skipped lines reported");
    text("m-live",num(t.liveSlots));text("m-mods",num(mods));text("m-known",num(known));text("m-dark",num(t.darkSlots));text("world-note",STATUS.haveStatus?"Map status "+duration(STATUS.statusAgeMs/1000)+" old · relay "+(STATUS.relayConnected?"connected":"disconnected"):"Waiting for the first relay status.");
    text("m-bypass",num((STATUS.lanes||[]).filter(function(l){return !l.open}).length));text("m-holes",num(t.holes));text("m-gaps",num(STATUS.genomeGaps));text("m-cold",num(STATUS.ledgerSegmentsAwaitingColdCopy));text("m-replay",STATUS.replayRawSeconds==null?"unknown":fixed(STATUS.replayRawSeconds,1)+"s");text("m-rollup",STATUS.rollupSavedAtMs?duration((Date.now()-STATUS.rollupSavedAtMs)/1000):"not saved yet");text("m-covered",num(STATUS.rollupCoveredRecords));text("m-duplicates",num(STATUS.duplicatesRefused));text("m-raw",bytes(STATUS.ledgerRawBytes));text("m-segments",num(STATUS.ledgerSegments));text("m-retired",num(STATUS.ledgerRetiredTotal));text("m-skipped",num(STATUS.ledgerSkippedLines||0));
    var root=by("worlds");root.textContent="";var head=document.createElement("div");head.className="row head";["World","State","Achieved speed","Stats age"].forEach(function(v){var s=document.createElement("span");s.textContent=v;head.appendChild(s)});root.appendChild(head);
    slots.forEach(function(s){var state=!s.live?"critical":!s.modConnected?"warning":!s.statsKnown?"unknown":"pass",label=!s.live?"dark":!s.modConnected?"sidecar only":!s.statsKnown?"stats unknown":"running";var row=document.createElement("div");row.className="row";var name=document.createElement("b");name.textContent="World "+s.slot+" · ("+s.position.col+","+s.position.row+")";var p=document.createElement("span");pill(p,state,label);var speed=document.createElement("span");speed.textContent=s.achievedTimeScale==null?"unknown":"×"+fixed(s.achievedTimeScale,1);var age=document.createElement("span");age.textContent=s.statsKnown?duration(s.statsAgeMs/1000):"unknown";row.appendChild(name);row.appendChild(p);row.appendChild(speed);row.appendChild(age);root.appendChild(row)});
    var state=!STATUS.relayConnected?"critical":!STATUS.haveStatus?"unknown":((t.darkSlots||0)>0||mods<(t.liveSlots||0)?"warning":"pass");pill(by("cov-status"),state);renderOverall();
  }
  function counterDelta(history,key){var vals=(history||[]).map(function(p){return p[key]}).filter(function(v){return v!=null});if(vals.length<2)return null;var d=vals[vals.length-1]-vals[0];return d<0?null:d}
  function chart(id,points,read,format,color,labelID){var vals=[];(points||[]).forEach(function(p){var v=read(p);if(v!=null&&isFinite(v))vals.push({at:p.at,v:Number(v)})});var root=by(id);root.textContent="";if(vals.length<2){root.innerHTML='<div class="empty">Not enough fresh samples</div>';text(labelID,"unknown");return}var min=Math.min.apply(null,vals.map(function(p){return p.v})),max=Math.max.apply(null,vals.map(function(p){return p.v}));if(min===max){min-=1;max+=1}var d=vals.map(function(p,i){var x=8+i*(464/(vals.length-1)),y=108-(p.v-min)*(96/(max-min));return(i?"L":"M")+x.toFixed(1)+" "+y.toFixed(1)}).join(" ");root.innerHTML='<svg viewBox="0 0 480 120" role="img" aria-label="recent metric trend"><path d="'+d+'" fill="none" stroke="'+color+'" stroke-width="2.5" vector-effect="non-scaling-stroke"/><line x1="8" y1="108" x2="472" y2="108" stroke="#294038"/></svg>';text(labelID,format(vals[vals.length-1].v)+" now · "+vals.length+" samples")}
  function renderHost(){
    var h=HEALTH&&HEALTH.serviceHost;if(!h||!h.available||!h.latest){text("sum-host","unknown");text("host-age","No valid service-host sample is available.");pill(by("cov-host"),"unknown");["h-cpu","h-memory","h-disk","h-load","h-swap","h-connections","h-resets","h-retrans","h-attempts","h-listen","h-syn","h-conntrack"].forEach(function(id){text(id,"unknown")});renderOverall();return}
    var l=h.latest,m=l.memory||{},tcp=l.tcp||{},ct=l.conntrack||{},hist=h.history||[],resets=counterDelta(hist,"estabResets"),outResets=counterDelta(hist,"outResets"),drops=counterDelta(hist,"listenDrops"),overflows=counterDelta(hist,"listenOverflows");
    text("sum-host",h.freshness);text("host-age",(h.freshness==="stale"?"Stale sample from ":"Latest sample ")+isoAge(h.asOf)+" · "+h.sampleCount+" samples across "+duration(h.windowSeconds));text("h-cpu",l.cpuBusyPct==null?"unknown":fixed(l.cpuBusyPct,1)+"%");text("h-memory",m.availablePct==null?"unknown":fixed(m.availablePct,1)+"%");text("h-disk",l.disk.dataUsedPct==null?"unknown":fixed(l.disk.dataUsedPct,1)+"%");text("h-load",[l.load.one,l.load.five,l.load.fifteen].map(function(v){return fixed(v,2)}).join(" / "));text("h-swap",m.swapUsedPct==null?"unknown":fixed(m.swapUsedPct,1)+"%");text("h-connections",num(tcp.currEstab));text("h-resets",resets==null||outResets==null?"unknown":num(resets+outResets));text("h-retrans",num(counterDelta(hist,"retransSegs")));text("h-attempts",num(counterDelta(hist,"attemptFails")));text("h-listen",drops==null||overflows==null?"unknown":num(drops)+" / "+num(overflows));text("h-syn",num(counterDelta(hist,"synRetrans")));text("h-conntrack",ct.usedPct==null?"unknown":fixed(ct.usedPct,1)+"%");
    chart("chart-cpu",hist,function(p){return p.cpuBusyPct},function(v){return fixed(v,1)+"%"},"#75bdf2","chart-cpu-label");chart("chart-memory",hist,function(p){return p.memoryAvailablePct},function(v){return fixed(v,1)+"%"},"#66e0ac","chart-memory-label");chart("chart-anon",hist,function(p){return p.archiveAnonBytes==null?null:p.archiveAnonBytes/1073741824},function(v){return fixed(v,2)+" GiB"},"#efbd57","chart-anon-label");
    var root=by("units");root.textContent="";var head=document.createElement("div");head.className="row head";["Service","State","Memory","Anonymous","Peak","Restarts"].forEach(function(v){var s=document.createElement("span");s.textContent=v;head.appendChild(s)});root.appendChild(head);(l.units||[]).forEach(function(u){var row=document.createElement("div");row.className="row";var name=document.createElement("b");name.textContent=u.name;var state=document.createElement("span");pill(state,u.activeState==="active"?"pass":u.activeState?"critical":"unknown",u.activeState||"unknown");var memory=document.createElement("span");memory.textContent=bytes(u.memoryBytes);var anon=document.createElement("span");anon.textContent=bytes(u.anonBytes);var peak=document.createElement("span");peak.textContent=bytes(u.mainVmHwmBytes);var restarts=document.createElement("span");restarts.textContent=num(u.restarts);row.appendChild(name);row.appendChild(state);row.appendChild(memory);row.appendChild(anon);row.appendChild(peak);row.appendChild(restarts);root.appendChild(row)});
    pill(by("cov-host"),h.freshness==="fresh"?"pass":"unknown",h.freshness);renderOverall();
  }
  function renderViewers(){if(!VIEWERS){text("viewer-title","Viewer signal unavailable");text("viewer-copy","The presence document could not be read. This is unknown, not an empty room.");text("v-sessions","unknown");text("v-request","unknown");pill(by("cov-viewers"),"unknown");renderOverall();return}text("viewer-title",VIEWERS.watching?"An audience is present":"Broadcast is idle");text("viewer-copy","Presence document "+isoAge(VIEWERS.asOf)+". Idle does not prove that the publisher can start.");text("v-sessions",num(VIEWERS.hlsSessions));text("v-request",VIEWERS.lastViewerRequestAgeSec==null?"none in window":duration(VIEWERS.lastViewerRequestAgeSec)+" ago");var fresh=viewersFresh();pill(by("cov-viewers"),fresh?"pass":"unknown",fresh?"fresh":"stale");renderOverall()}
  async function refreshHealth(){try{HEALTH=await get("/api/health");FAIL.health=false}catch(e){FAIL.health=true}renderMonitor();renderHost()}
  async function refreshStatus(){try{STATUS=await get("/api/status");FAIL.status=false}catch(e){STATUS=null;FAIL.status=true}renderStatus()}
  async function refreshViewers(){try{VIEWERS=await get("/api/viewers");FAIL.viewers=false}catch(e){VIEWERS=null;FAIL.viewers=true}renderViewers()}
  refreshHealth();refreshStatus();refreshViewers();setInterval(refreshHealth,60000);setInterval(refreshStatus,15000);setInterval(refreshViewers,15000);
})();
</script>
</body>
</html>`
