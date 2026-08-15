package archive

// watchPageHTML is the public wrapper around the single shared game broadcast.
// The media player itself is served from /stream/ by the same nginx front door;
// no stream key, ingest address, or control surface is present in this page.
const watchPageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Bibites Multiverse — Watch Live</title>
<meta name="description" content="Watch one live Bibites Multiverse world. The shared camera follows the youngest living Bibite until it dies or leaves the world.">
<meta name="theme-color" content="#07100d">
<link rel="canonical" href="https://bibitesmultiverse.com/watch">
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<style>
:root{color-scheme:dark;--bg:#07100d;--panel:#0d1915;--line:#20352d;--text:#edf7f2;
--dim:#91a69c;--green:#62e6a7;--gold:#e2b96f;--red:#ef7b79;--max:1180px}
*{box-sizing:border-box}html{scroll-behavior:smooth}body{margin:0;background:var(--bg);color:var(--text);
font:16px/1.55 ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
a{color:inherit}.skip{position:fixed;left:12px;top:-80px;z-index:30;background:var(--text);color:var(--bg);
padding:9px 12px;border-radius:6px;text-decoration:none}.skip:focus{top:12px}.shell{width:min(calc(100% - 36px),var(--max));margin:auto}
header{border-bottom:1px solid var(--line);background:rgba(7,16,13,.94)}.nav{min-height:70px;display:flex;align-items:center;
justify-content:space-between;gap:20px}.brand{display:flex;align-items:center;gap:10px;text-decoration:none;font-weight:720;letter-spacing:.01em}
.mark{width:28px;color:var(--green)}nav{display:flex;align-items:center;gap:24px}nav a{font-size:14px;color:var(--dim);text-decoration:none}
nav a:hover{color:var(--text)}.maplink{padding:8px 13px;border:1px solid var(--line);border-radius:999px;color:var(--text)}
main{padding:64px 0 90px}.eyebrow{margin:0 0 13px;color:var(--green);font-size:12px;font-weight:760;letter-spacing:.14em;text-transform:uppercase}
h1{font-size:clamp(42px,7vw,76px);line-height:.98;letter-spacing:-.045em;margin:0;max-width:850px}.lede{max-width:760px;color:var(--dim);
font-size:clamp(17px,2vw,21px);margin:24px 0 36px}.player{position:relative;aspect-ratio:16/9;border:1px solid var(--line);border-radius:18px;
overflow:hidden;background:#020504;box-shadow:0 28px 90px rgba(0,0,0,.36)}.player iframe{display:block;border:0;width:100%;height:100%;background:#020504}
.offline{position:absolute;inset:0;display:grid;place-items:center;text-align:center;padding:30px;background:radial-gradient(circle at 50% 45%,#12261e,#020504 70%)}
.offline[hidden]{display:none}.offline svg{width:65px;color:var(--dim);margin-bottom:17px}.offline b{display:block;font-size:21px;margin-bottom:7px}
.offline span{display:block;color:var(--dim);max-width:510px}.statebar{display:flex;align-items:center;justify-content:space-between;gap:20px;padding:15px 4px 0;
color:var(--dim);font-size:13px}.state{display:flex;align-items:center;gap:8px}.dot{width:8px;height:8px;border-radius:50%;background:var(--gold);box-shadow:0 0 0 4px rgba(226,185,111,.12)}
.dot.online{background:var(--green);box-shadow:0 0 0 4px rgba(98,230,167,.12)}.dot.offline-dot{background:var(--red);box-shadow:0 0 0 4px rgba(239,123,121,.12)}
.facts{display:grid;grid-template-columns:repeat(3,1fr);margin-top:68px;border:1px solid var(--line);border-radius:15px;overflow:hidden}
.fact{padding:27px 25px;min-height:160px;background:var(--panel)}.fact+.fact{border-left:1px solid var(--line)}.fact b{display:block;margin:8px 0 9px;font-size:18px}
.fact p{margin:0;color:var(--dim);font-size:14px}.num{color:var(--green);font:700 11px/1 ui-monospace,SFMono-Regular,Menlo,monospace;letter-spacing:.12em}
.note{margin-top:35px;color:var(--dim);font-size:14px;max-width:820px}.note strong{color:var(--text)}
footer{border-top:1px solid var(--line);padding:28px 0;color:var(--dim);font-size:13px}.foot{display:flex;justify-content:space-between;gap:20px;flex-wrap:wrap}
.footlinks{display:flex;gap:18px}.foot a{text-decoration:none}.foot a:hover{color:var(--text)}
@media(max-width:720px){.shell{width:min(calc(100% - 24px),var(--max))}.nav{min-height:62px}nav a:not(.maplink){display:none}
main{padding-top:46px}.player{border-radius:11px}.facts{grid-template-columns:1fr}.fact+.fact{border-left:0;border-top:1px solid var(--line)}
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
  <nav aria-label="Primary navigation"><a href="/">About</a><a href="/live">Species &amp; lineages</a><a class="maplink" href="/live">Live map&nbsp; →</a></nav>
</div></header>
<main id="broadcast" class="shell" tabindex="-1">
  <p class="eyebrow">One world · one shared camera · live</p>
  <h1>Follow a life in progress.</h1>
  <p class="lede">Everyone on this page watches the same Multiverse world. The camera chooses its youngest living Bibite, stays with it for the rest of its life here, then chooses again.</p>
  <div class="player">
    <iframe id="player" title="Live Bibites game broadcast" allow="autoplay; fullscreen; picture-in-picture" allowfullscreen></iframe>
    <div class="offline" id="offline" role="status" aria-live="polite">
      <div><svg viewBox="-9 -7 19 14" aria-hidden="true"><path fill="currentColor" d="M4.5-1.26 2.16 0l2.34 1.26C4.14 3.06 1.98 4.14-.54 4.14-3.24 4.14-5.58 2.16-6.66 0-5.58-2.16-3.24-4.14-.54-4.14c2.52 0 4.68 1.08 5.04 2.88Z"/></svg>
      <b id="offtitle">Checking the broadcast…</b><span id="offcopy">The map continues running while this player connects.</span></div>
    </div>
  </div>
  <div class="statebar"><span class="state"><i class="dot" id="dot"></i><span id="status">Checking stream</span></span><span>Approximately 6–15 seconds behind the simulation</span></div>
  <section class="facts" aria-label="How the broadcast works">
    <article class="fact"><span class="num">01 · CHOOSE</span><b>The youngest living Bibite</b><p>Age means simulated age in this world. Entity ID breaks an exact tie.</p></article>
    <article class="fact"><span class="num">02 · STAY</span><b>No switching on new births</b><p>A younger birth does not steal the camera. The chosen life gets its whole turn.</p></article>
    <article class="fact"><span class="num">03 · REPEAT</span><b>Death or departure</b><p>Death triggers a new choice. Migration does too, because the creature has left this particular world.</p></article>
  </section>
  <p class="note"><strong>This is a spectator camera, not a control surface.</strong> Watching cannot pause, select, move, feed, kill, or otherwise change the simulation. A brief outage can occur when the broadcast host restarts; the public map and every other world continue independently.</p>
</main>
<footer><div class="shell foot"><span>Independent community project · Apache-2.0</span><span class="footlinks"><a href="/">About</a><a href="/live">Live map</a><a href="https://github.com/jpinedaa/bibites-multiverse">Source</a></span></div></footer>
<script>
(function(){
  var frame=document.getElementById("player"), cover=document.getElementById("offline");
  var dot=document.getElementById("dot"), status=document.getElementById("status");
  var title=document.getElementById("offtitle"), copy=document.getElementById("offcopy");
  var loaded=false, failures=0;
  async function check(){
    try{
      var response=await fetch("/stream/bibites/index.m3u8",{cache:"no-store"});
      if(!response.ok) throw new Error("offline");
      failures=0;
      if(!loaded){frame.src="/stream/bibites?controls=true&muted=true&autoplay=true&playsInline=true";loaded=true;}
      cover.hidden=true;dot.className="dot online";status.textContent="Broadcast online";
    }catch(e){
      failures++;
      if(failures<2) return;
      cover.hidden=false;dot.className="dot offline-dot";status.textContent="Broadcast reconnecting";
      title.textContent="The camera is between sessions.";
      copy.textContent="This page will reconnect automatically. The Multiverse itself continues running.";
    }
  }
  check();setInterval(check,5000);
})();
</script>
</body>
</html>`
