package archive

// watchPageHTML is the public wrapper around the single shared game broadcast.
// The media player itself is served from /stream/ by the same nginx front door;
// no stream key, ingest address, or control surface is present in this page.
const watchPageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Bibites Multiverse — Watch Broadcast</title>
<meta name="description" content="Watch one live Bibites Multiverse world. The shared camera follows the youngest living Bibite until it dies or leaves the world.">
<meta name="theme-color" content="#0b1110">
<link rel="canonical" href="https://bibitesmultiverse.com/watch">
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<meta property="og:type" content="website">
<meta property="og:site_name" content="Bibites Multiverse">
<meta property="og:title" content="Bibites Multiverse — Watch Broadcast">
<meta property="og:description" content="Follow a life in progress. The shared camera stays on the youngest living Bibite until it dies or leaves the world.">
<meta property="og:url" content="https://bibitesmultiverse.com/watch">
<meta property="og:image" content="https://bibitesmultiverse.com/social-card-watch.png">
<meta property="og:image:secure_url" content="https://bibitesmultiverse.com/social-card-watch.png">
<meta property="og:image:type" content="image/png">
<meta property="og:image:width" content="1200">
<meta property="og:image:height" content="630">
<meta property="og:image:alt" content="The shared broadcast camera held on one young Bibite swimming through its living world.">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="Bibites Multiverse — Watch Broadcast">
<meta name="twitter:description" content="Follow a life in progress. The shared camera stays on the youngest living Bibite until it dies or leaves the world.">
<meta name="twitter:image" content="https://bibitesmultiverse.com/social-card-watch.png">
<meta name="twitter:image:alt" content="The shared broadcast camera held on one young Bibite swimming through its living world.">
<style>
:root{color-scheme:dark;--bg:#0b1110;--panel:#111a18;--panel2:#16221f;--line:#294038;
--text:#eff7f3;--dim:#9aafa7;--green:#66e0ac;--blue:#75bdf2;--gold:#efbd57;
--red:#e86c76;--ink:#07100d;--max:1180px}
*{box-sizing:border-box}html{scroll-behavior:smooth}body{margin:0;background:var(--bg);color:var(--text);
font:16px/1.55 ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;
background:radial-gradient(circle at 82% 8%,rgba(67,160,126,.18),transparent 28rem),
radial-gradient(circle at 8% 34%,rgba(62,126,174,.12),transparent 26rem),var(--bg)}
a{color:inherit}.skip{position:fixed;left:12px;top:-80px;z-index:30;background:var(--text);color:var(--ink);
padding:9px 12px;border-radius:6px;text-decoration:none}.skip:focus{top:12px}.shell{width:min(calc(100% - 36px),var(--max));margin:auto}
header{position:sticky;top:0;z-index:20;border-bottom:1px solid rgba(83,120,107,.28);background:rgba(11,17,16,.86);
backdrop-filter:blur(16px)}.nav{min-height:72px;display:flex;align-items:center;
justify-content:space-between;gap:20px}.brand{display:flex;align-items:center;gap:10px;text-decoration:none;font-weight:720;letter-spacing:.01em}
.mark{width:30px;color:var(--green);filter:drop-shadow(0 0 9px rgba(102,224,172,.3))}nav{display:flex;align-items:center;gap:24px}nav a{font-size:14px;color:var(--dim);text-decoration:none}
nav a:hover{color:var(--text)}nav a[aria-current="page"]{color:var(--text);font-weight:700}
.maplink{padding:8px 13px;border:1px solid var(--line);border-radius:999px;color:var(--text)}
main{padding:64px 0 90px}.eyebrow{margin:0 0 13px;color:var(--green);font-size:12px;font-weight:760;letter-spacing:.14em;text-transform:uppercase}
h1{font-size:clamp(42px,7vw,76px);line-height:.98;letter-spacing:-.045em;margin:0;max-width:850px}.lede{max-width:760px;color:var(--dim);
font-size:clamp(17px,2vw,21px);margin:24px 0 36px}.player{position:relative;aspect-ratio:16/9;border:1px solid var(--line);border-radius:18px;
overflow:hidden;background:#020504;box-shadow:0 28px 90px rgba(0,0,0,.36)}.player iframe{display:block;border:0;width:100%;height:100%;background:#020504}
.offline{position:absolute;inset:0;display:grid;place-items:center;text-align:center;padding:30px;background:radial-gradient(circle at 50% 45%,#12261e,#020504 70%)}
.offline[hidden]{display:none}.offline svg{width:65px;color:var(--dim);margin-bottom:17px}.offline b{display:block;font-size:21px;margin-bottom:7px}
.offline span{display:block;color:var(--dim);max-width:510px}.statebar{display:flex;align-items:center;justify-content:space-between;gap:20px;padding:15px 4px 0;
color:var(--dim);font-size:13px}.state{display:flex;align-items:center;gap:8px}.dot{width:8px;height:8px;border-radius:50%;background:var(--gold);box-shadow:0 0 0 4px rgba(239,189,87,.12)}
.dot.online{background:var(--green);box-shadow:0 0 0 4px rgba(102,224,172,.12)}.dot.offline-dot{background:var(--red);box-shadow:0 0 0 4px rgba(232,108,118,.12)}
.worldbar{display:flex;align-items:center;gap:20px;flex-wrap:wrap;margin-top:26px;padding:19px 22px;
border:1px solid var(--line);border-radius:14px;background:linear-gradient(145deg,var(--panel2),var(--panel))}
.gridsym{flex:none;display:block}.gridsym .gcell{fill:none;stroke:var(--line);stroke-width:1.5;rx:3}
.gridsym .gcell.filled{stroke:var(--dim)}.gridsym .gcell.hole{stroke-dasharray:3 3}
.gridsym .gcell.here{fill:rgba(102,224,172,.16);stroke:var(--green);stroke-width:2}
.gridsym .gno{fill:var(--dim);font:600 9px/1 ui-monospace,SFMono-Regular,Menlo,monospace;text-anchor:middle}
.gridsym .gno.here{fill:var(--green)}
.wmeta{min-width:0}.wname{display:block;font-size:19px;font-weight:700}
.wpeer{display:block;margin-top:3px;color:var(--dim);font:12px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace;
word-break:break-all}.wpos{display:block;margin-top:5px;color:var(--dim);font-size:14px}
.wlink{margin-left:auto;flex:none;padding:9px 15px;border:1px solid var(--line);border-radius:999px;
color:var(--text);text-decoration:none;font-size:14px}.wlink:hover,.wlink:focus{border-color:var(--green)}
.wunk{color:var(--gold);font-style:italic}
.facts{display:grid;grid-template-columns:repeat(3,1fr);margin-top:68px;border:1px solid var(--line);border-radius:16px;overflow:hidden}
.fact{padding:27px 25px;min-height:160px;background:linear-gradient(145deg,var(--panel2),var(--panel))}.fact+.fact{border-left:1px solid var(--line)}.fact b{display:block;margin:8px 0 9px;font-size:18px}
.fact p{margin:0;color:var(--dim);font-size:14px}.num{color:var(--green);font:700 11px/1 ui-monospace,SFMono-Regular,Menlo,monospace;letter-spacing:.12em}
.note{margin-top:35px;color:var(--dim);font-size:14px;max-width:820px}.note strong{color:var(--text)}
footer{border-top:1px solid var(--line);padding:28px 0;color:var(--dim);font-size:13px}.foot{display:flex;justify-content:space-between;gap:20px;flex-wrap:wrap}
.footlinks{display:flex;gap:18px}.foot a{text-decoration:none}.foot a:hover{color:var(--text)}
@media(max-width:860px){.nav{min-height:62px;padding-block:10px;flex-wrap:wrap;gap:10px}nav{width:100%;gap:16px;overflow-x:auto;padding-bottom:3px}nav a{white-space:nowrap}}
@media(max-width:720px){.shell{width:min(calc(100% - 24px),var(--max))}
nav{flex-wrap:wrap;overflow-x:visible;padding-bottom:0}main{padding-top:46px}.player{border-radius:11px}.facts{grid-template-columns:1fr}.fact+.fact{border-left:0;border-top:1px solid var(--line)}
.statebar{align-items:flex-start;flex-direction:column;gap:5px}}
</style>
</head>
<body>
<a class="skip" href="#broadcast">Skip to broadcast</a>
<header><div class="shell nav">
  <a class="brand" href="/" aria-label="Bibites Multiverse home">
    <svg class="mark" viewBox="-9 -7 19 14" aria-hidden="true"><path fill="currentColor" d="M4.5-1.26 2.16 0l2.34 1.26C4.14 3.06 1.98 4.14-.54 4.14-3.24 4.14-5.58 2.16-6.66 0-5.58-2.16-3.24-4.14-.54-4.14c2.52 0 4.68 1.08 5.04 2.88Z"/><circle cx="1.5" cy="-1.85" r="1" fill="#07100d"/></svg>
    <span>Bibites Multiverse</span>
  </a>
  <nav aria-label="Primary navigation"><a href="/#how">How it works</a><a href="/#join">Join</a><a href="/watch" aria-current="page">Watch broadcast</a><a href="https://github.com/jpinedaa/bibites-multiverse">GitHub</a><a class="maplink" href="/live">Live map&nbsp; →</a></nav>
</div></header>
<main id="broadcast" class="shell" tabindex="-1">
  <p class="eyebrow">One world · one shared camera · live</p>
  <h1>Follow a life in progress.</h1>
  <p class="lede">Everyone on this page watches the same Multiverse world. The camera chooses its youngest living Bibite, stays with it for the rest of its life here, then chooses again.</p>
  <div class="player">
    <iframe id="player" title="Live Bibites game broadcast" allow="autoplay; fullscreen; picture-in-picture"></iframe>
    <div class="offline" id="offline" role="status" aria-live="polite">
      <div><svg viewBox="-9 -7 19 14" aria-hidden="true"><path fill="currentColor" d="M4.5-1.26 2.16 0l2.34 1.26C4.14 3.06 1.98 4.14-.54 4.14-3.24 4.14-5.58 2.16-6.66 0-5.58-2.16-3.24-4.14-.54-4.14c2.52 0 4.68 1.08 5.04 2.88Z"/></svg>
      <b id="offtitle">Checking the broadcast…</b><span id="offcopy">The map continues running while this player connects.</span></div>
    </div>
  </div>
  <div class="statebar"><span class="state"><i class="dot" id="dot"></i><span id="status">Checking stream</span></span><span>Approximately 6–15 seconds behind the simulation</span></div>
  <div class="worldbar" id="worldbar">
    <div class="wmeta" id="wmeta"><span class="wname" id="wname">Finding the world on camera&hellip;</span></div>
  </div>
  <section class="facts" aria-label="How the broadcast works">
    <article class="fact"><span class="num">01 · CHOOSE</span><b>The youngest living Bibite</b><p>Age means simulated age in this world. Entity ID breaks an exact tie.</p></article>
    <article class="fact"><span class="num">02 · STAY</span><b>No switching on new births</b><p>A younger birth does not steal the camera. The chosen life gets its whole turn.</p></article>
    <article class="fact"><span class="num">03 · REPEAT</span><b>Death or departure</b><p>Death triggers a new choice. Migration does too, because the creature has left this particular world.</p></article>
  </section>
  <p class="note"><strong>This is a spectator camera, not a control surface.</strong> Watching cannot pause, select, move, feed, kill, or otherwise change the simulation. A brief outage can occur when the broadcast host restarts; the public map and every other world continue independently.</p>
</main>
<footer><div class="shell foot"><span>Independent community project · Apache-2.0</span><span class="footlinks"><a href="/#how">How it works</a><a href="/#join">Join</a><a href="/live">Live map</a><a href="https://github.com/jpinedaa/bibites-multiverse">Source</a></span></div></footer>
<script>
(function(){
  var frame=document.getElementById("player"), cover=document.getElementById("offline");
  var dot=document.getElementById("dot"), status=document.getElementById("status");
  var title=document.getElementById("offtitle"), copy=document.getElementById("offcopy");
  var loaded=false, failures=0;
  function reconnecting(){
    cover.hidden=false;dot.className="dot offline-dot";status.textContent="Broadcast reconnecting";
    title.textContent="The camera is between sessions.";
    copy.textContent="This page will reconnect automatically. The Multiverse itself continues running.";
  }
  frame.addEventListener("load",function(){
    setTimeout(function(){
      try{
        if(frame.contentDocument&&frame.contentDocument.querySelector("video")) return;
        loaded=false;failures=2;reconnecting();
      }catch(e){}
    },600);
  });
  async function check(){
    try{
      var response=await fetch("/stream/bibites/index.m3u8",{cache:"no-store"});
      if(response.status===429&&loaded) return;
      if(!response.ok) throw new Error("offline");
      failures=0;
      if(!loaded){frame.src="/stream/bibites?controls=true&muted=true&autoplay=true&playsInline=true";loaded=true;}
      cover.hidden=true;dot.className="dot online";status.textContent="Broadcast online";
    }catch(e){
      failures++;
      if(failures<2) return;
      reconnecting();
    }
  }
  check();setInterval(check,5000);
})();
/* ------------------------------------------------------- which world is this?
   The camera shows ONE world of the map, and until now this page never said
   which. The archive is TOLD which peer that is (no frame on either wire says a
   world is being filmed), publishes it as broadcastPeerId, and everything drawn
   below is read off the same /api/status the live map reads.

   Every failure here is UNKNOWN and never a guess: an unnamed world, a named
   world absent from the map, and an unreachable archive each say so plainly.
   Naming the wrong world would be worse than naming none. */
(function(){
  var bar=document.getElementById("worldbar");
  var SVG="http://www.w3.org/2000/svg", CELL=19, GAP=5;
  function el(tag,cls,text){
    var e=document.createElement(tag);
    if(cls) e.className=cls;
    if(text!=null) e.textContent=String(text);
    return e;
  }
  function svgEl(tag,cls){
    var e=document.createElementNS(SVG,tag);
    if(cls) e.setAttribute("class",cls);
    return e;
  }
  function clear(){ while(bar.firstChild) bar.removeChild(bar.firstChild); }
  function unknown(text){
    clear();
    var meta=el("div","wmeta");
    meta.appendChild(el("span","wname wunk",text));
    bar.appendChild(meta);
  }
  /* The map rectangle at a glance, with this world's cell lit. Same (col,row)
     the live map prints, so the two pages can be read against each other. */
  function gridSymbol(shape,slots,here){
    var w=Math.max(1,shape.width||1), h=Math.max(1,shape.height||1);
    var at={};
    for(var i=0;i<slots.length;i++){
      var p=slots[i].position||{};
      at[p.col+","+p.row]=slots[i];
    }
    var svg=svgEl("svg","gridsym");
    svg.setAttribute("width",String(w*CELL+(w-1)*GAP));
    svg.setAttribute("height",String(h*CELL+(h-1)*GAP));
    svg.setAttribute("role","img");
    svg.setAttribute("aria-label","this world sits at column "+((here.position.col|0)+1)+
      " row "+((here.position.row|0)+1)+" of a "+w+" by "+h+" map");
    for(var row=0;row<h;row++){
      for(var col=0;col<w;col++){
        var slot=at[col+","+row], mine=slot&&slot.slot===here.slot;
        var kind=mine?"here":(slot?"filled":"hole");
        var r=svgEl("rect","gcell "+kind);
        r.setAttribute("x",String(col*(CELL+GAP)));
        r.setAttribute("y",String(row*(CELL+GAP)));
        r.setAttribute("width",String(CELL));
        r.setAttribute("height",String(CELL));
        r.setAttribute("rx","3");
        svg.appendChild(r);
        if(slot){
          var n=svgEl("text","gno"+(mine?" here":""));
          n.setAttribute("x",String(col*(CELL+GAP)+CELL/2));
          n.setAttribute("y",String(row*(CELL+GAP)+CELL/2+3.5));
          n.textContent=String(slot.slot);
          svg.appendChild(n);
        }
      }
    }
    return svg;
  }
  function render(d){
    var named=d&&d.broadcastPeerId;
    if(!named){
      unknown("This deployment has not named the world on camera.");
      return;
    }
    var slots=(d&&d.slots)||[], here=null;
    for(var i=0;i<slots.length;i++) if(slots[i].peerId===named) here=slots[i];
    if(!here||!here.position){
      unknown("The world on camera is not on the map right now.");
      return;
    }
    clear();
    bar.appendChild(gridSymbol(d.map||{},slots,here));
    var m=el("div","wmeta");
    m.appendChild(el("span","wname","slot "+here.slot+" of "+(d.slotCount||slots.length)));
    /* A peer id is another party's chosen string, so it goes in as text. */
    m.appendChild(el("span","wpeer",here.peerId));
    var p=here.position, shape=d.map||{};
    var where="("+p.col+","+p.row+")";
    if(shape.width&&shape.height){
      where+=" — column "+((p.col|0)+1)+" of "+shape.width+", row "+((p.row|0)+1)+" of "+shape.height;
    }
    m.appendChild(el("span","wpos",where));
    bar.appendChild(m);
    var link=el("a","wlink","See it on the live map  →");
    link.href="/live#settings";
    bar.appendChild(link);
  }
  async function readWorld(){
    try{
      var response=await fetch("/api/status",{cache:"no-store"});
      if(!response.ok) throw new Error("unavailable");
      render(await response.json());
    }catch(e){
      unknown("The map is not answering, so this page cannot name the world.");
    }
  }
  readWorld();setInterval(readWorld,30000);
})();
</script>
</body>
</html>`
