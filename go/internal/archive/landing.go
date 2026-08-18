package archive

import "strings"

// The public front door and the operator console are deliberately separate.
// The front door answers a first visitor's questions and samples the same
// read-only status endpoint as the console. The console remains the dense,
// self-contained instrument used by participants and operators.
const landingPageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Bibites Multiverse — Evolution has a map</title>
<meta name="description" content="Independent Bibites simulations connected as neighboring ecosystems, with live migrations, species, and evolutionary history.">
<meta name="theme-color" content="#0b1110">
<link rel="canonical" href="https://bibitesmultiverse.com/">
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<meta property="og:type" content="website">
<meta property="og:site_name" content="Bibites Multiverse">
<meta property="og:title" content="Bibites Multiverse — Evolution has a map">
<meta property="og:description" content="Independent worlds. Real migrations. Shared evolutionary history.">
<meta property="og:url" content="https://bibitesmultiverse.com/">
<meta property="og:image" content="https://bibitesmultiverse.com/social-card.png">
<meta property="og:image:secure_url" content="https://bibitesmultiverse.com/social-card.png">
<meta property="og:image:type" content="image/png">
<meta property="og:image:width" content="1200">
<meta property="og:image:height" content="630">
<meta property="og:image:alt" content="A dark map of independent Bibites worlds with migration lanes drawn between them, under the line Evolution has a map.">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="Bibites Multiverse — Evolution has a map">
<meta name="twitter:description" content="Independent worlds. Real migrations. Shared evolutionary history.">
<meta name="twitter:image" content="https://bibitesmultiverse.com/social-card.png">
<meta name="twitter:image:alt" content="A dark map of independent Bibites worlds with migration lanes drawn between them, under the line Evolution has a map.">
<script>
if (/^#(?:map|species|settings|tree)$/.test(location.hash)) {
  location.replace("/live" + location.hash);
}
</script>
<style>
:root{color-scheme:dark;--bg:#0b1110;--surface:#111a18;--surface2:#16221f;
--line:#294038;--text:#eff7f3;--muted:#9aafa7;--green:#66e0ac;--blue:#75bdf2;
--amber:#efbd57;--dark:#e86c76;--ink:#07100d;--max:1180px}
*{box-sizing:border-box}
html{scroll-behavior:smooth}
body{margin:0;background:
radial-gradient(circle at 82% 8%,rgba(67,160,126,.18),transparent 28rem),
radial-gradient(circle at 8% 34%,rgba(62,126,174,.12),transparent 26rem),var(--bg);
color:var(--text);font:16px/1.65 ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}
a{color:inherit}.skip{position:fixed;left:12px;top:-80px;z-index:100;background:var(--text);
color:var(--ink);padding:9px 13px;border-radius:7px}.skip:focus{top:12px}
.shell{width:min(calc(100% - 40px),var(--max));margin-inline:auto}
.sitehd{position:sticky;top:0;z-index:30;background:rgba(11,17,16,.86);
backdrop-filter:blur(16px);border-bottom:1px solid rgba(83,120,107,.28)}
.nav{min-height:72px;display:flex;align-items:center;justify-content:space-between;gap:24px}
.brand{display:inline-flex;align-items:center;gap:10px;text-decoration:none;font-weight:760;
letter-spacing:-.02em}.mark{width:30px;height:22px;color:var(--green);filter:drop-shadow(0 0 9px rgba(102,224,172,.3))}
.links{display:flex;align-items:center;gap:22px;font-size:14px;color:var(--muted)}
.links a{text-decoration:none}.links a:hover,.links a:focus-visible{color:var(--text)}
.links a[aria-current="page"]{color:var(--text);font-weight:700}
.livepill{display:inline-flex;align-items:center;gap:7px!important;padding:7px 11px;border:1px solid var(--line);
border-radius:999px;color:var(--text)!important;background:rgba(22,34,31,.75)}
.dot{width:7px;height:7px;border-radius:50%;background:var(--amber);box-shadow:0 0 0 4px rgba(239,189,87,.1)}
.dot.ok{background:var(--green);box-shadow:0 0 0 4px rgba(102,224,172,.1)}
.hero{min-height:690px;display:grid;grid-template-columns:minmax(0,1.03fr) minmax(360px,.97fr);
align-items:center;gap:72px;padding-block:88px 68px}
.eyebrow{display:flex;align-items:center;gap:10px;margin:0 0 20px;color:var(--green);
font:700 12px/1.2 ui-monospace,SFMono-Regular,Menlo,monospace;letter-spacing:.13em;text-transform:uppercase}
.eyebrow:before{content:"";width:30px;border-top:1px solid currentColor}
h1{max-width:780px;font-size:clamp(54px,7.2vw,94px);line-height:.93;letter-spacing:-.062em;margin:0}
.lede{max-width:660px;color:#bfd0c9;font-size:clamp(18px,2vw,22px);line-height:1.55;margin:28px 0 34px}
.lede a{color:inherit;text-decoration:underline;text-underline-offset:3px;text-decoration-color:var(--green)}
.actions{display:flex;flex-wrap:wrap;gap:12px}.button{display:inline-flex;align-items:center;justify-content:center;
min-height:48px;padding:0 19px;border-radius:8px;text-decoration:none;font-weight:720;font-size:14px;border:1px solid var(--line)}
.button.primary{background:var(--green);border-color:var(--green);color:var(--ink)}
.button.primary:hover{background:#87ebc2}.button.secondary{background:rgba(22,34,31,.6)}
.button.secondary:hover{border-color:var(--muted)}
.promise{margin-top:22px;color:var(--muted);font-size:13px}
.release{display:inline-flex;align-items:center;gap:8px;margin:20px 0 0;padding:7px 13px;border:1px solid var(--line);
border-radius:999px;background:rgba(22,34,31,.6);color:var(--muted);font-size:13px}
.release b{color:var(--text);font:700 13px/1 ui-monospace,SFMono-Regular,Menlo,monospace;letter-spacing:.02em}
.topology{position:relative;min-height:480px;border:1px solid var(--line);border-radius:28px;
background:linear-gradient(145deg,rgba(22,34,31,.92),rgba(10,17,15,.72));overflow:hidden;
box-shadow:0 32px 80px rgba(0,0,0,.26)}
.topology:before{content:"";position:absolute;inset:0;background-image:
linear-gradient(rgba(117,189,242,.045) 1px,transparent 1px),
linear-gradient(90deg,rgba(117,189,242,.045) 1px,transparent 1px);background-size:42px 42px;
mask-image:linear-gradient(to bottom,black,transparent 92%)}
.maplabel{position:absolute;left:26px;top:23px;color:var(--muted);font:12px ui-monospace,SFMono-Regular,Menlo,monospace;
letter-spacing:.11em;text-transform:uppercase}.toposvg{position:absolute;inset:58px 24px 74px;width:calc(100% - 48px);height:calc(100% - 132px)}
.lane{fill:none;stroke:var(--blue);stroke-width:1.3;stroke-dasharray:5 7;opacity:.55}
.lane.hot{stroke:var(--green);stroke-dasharray:none;opacity:.8}.node{fill:#13231f;stroke:var(--green);stroke-width:1.4}
.node.dark{stroke:var(--dark);stroke-dasharray:3 4}.node.hole{stroke:#53625d;stroke-dasharray:4 5}
.tinybib{fill:var(--green);filter:drop-shadow(0 0 7px rgba(102,224,172,.8));offset-path:path("M87 83 C135 83 168 83 222 83");
animation:travel 5s ease-in-out infinite}.topology .caption{position:absolute;left:26px;right:26px;bottom:23px;
display:flex;justify-content:space-between;gap:20px;color:var(--muted);font-size:12px}
@keyframes travel{0%,10%{offset-distance:0%;opacity:0}18%{opacity:1}72%{opacity:1}82%,100%{offset-distance:100%;opacity:0}}
@keyframes statuspulse{0%,45%{box-shadow:0 0 0 5px rgba(102,224,172,.12)}75%,100%{box-shadow:0 0 0 12px rgba(102,224,172,0)}}
.snapshot{position:relative;margin-top:-34px;border:1px solid var(--line);border-radius:16px;background:rgba(17,26,24,.96);
box-shadow:0 20px 55px rgba(0,0,0,.24);display:grid;grid-template-columns:1.2fr repeat(3,1fr);overflow:hidden}
.snapstate,.metric{min-height:118px;padding:22px 24px;display:flex;flex-direction:column;justify-content:center}
.metric{border-left:1px solid var(--line)}.snapk{color:var(--muted);font:11px ui-monospace,SFMono-Regular,Menlo,monospace;
letter-spacing:.1em;text-transform:uppercase}.snapv{margin-top:6px;font-size:28px;font-weight:760;letter-spacing:-.035em}
.snapstate .snapv{font-size:18px;letter-spacing:-.01em}.statusvalue{display:flex;align-items:center;gap:10px}
.statusdot{width:9px;height:9px;flex:0 0 auto;border-radius:50%;background:var(--amber);box-shadow:0 0 0 5px rgba(239,189,87,.1)}
.statusdot.ok{background:var(--green);box-shadow:0 0 0 5px rgba(102,224,172,.12);animation:statuspulse 2.4s ease-out infinite}
.statusdot.down{background:var(--dark);box-shadow:0 0 0 5px rgba(232,108,118,.1)}
.snapnote{margin-top:3px;color:var(--muted);font-size:12px}
.section{padding-block:64px 0}.section:last-child{padding-bottom:64px}.sectionhd{max-width:720px;margin-bottom:32px}.kicker{color:var(--green);
font:700 12px ui-monospace,SFMono-Regular,Menlo,monospace;letter-spacing:.12em;text-transform:uppercase}
h2{font-size:clamp(36px,5vw,60px);line-height:1.03;letter-spacing:-.045em;margin:12px 0 18px}
.sectionhd p{color:var(--muted);font-size:18px;margin:0}.principles{display:grid;grid-template-columns:repeat(3,1fr);gap:16px}
.principle{min-height:220px;padding:28px;border:1px solid var(--line);border-radius:16px;background:linear-gradient(145deg,var(--surface2),var(--surface))}
.principle h3{font-size:22px;margin:0 0 10px}
.principle p{color:var(--muted);margin:0}.flow{display:grid;grid-template-columns:repeat(4,1fr);gap:0;border:1px solid var(--line);border-radius:16px;overflow:hidden}
.flowitem{position:relative;padding:28px 24px;min-height:152px;background:rgba(17,26,24,.72)}
.flowitem+.flowitem{border-left:1px solid var(--line)}.flowitem:not(:last-child):after{content:"→";position:absolute;right:-13px;top:32px;
z-index:2;width:26px;height:26px;border:1px solid var(--line);border-radius:50%;background:var(--bg);color:var(--blue);text-align:center;line-height:24px}
.flowitem b{display:block;margin:0 0 7px}.flowitem span{color:var(--muted);font-size:14px}
.joinbox{display:grid;grid-template-columns:1.05fr .95fr;border:1px solid var(--line);border-radius:22px;overflow:hidden;background:var(--surface)}
.joincopy{padding:50px}.joincopy h2{font-size:clamp(38px,5vw,64px)}.joincopy p{color:var(--muted);max-width:620px}
.trust{padding:50px;border-left:1px solid var(--line);background:linear-gradient(150deg,#172720,#101917)}.trust h3{margin:0 0 20px}.trust ul{margin:0;padding:0;list-style:none}
.trust li{position:relative;padding:12px 0 12px 27px;border-top:1px solid var(--line);color:#bfd0c9}.trust li:before{content:"✓";position:absolute;left:0;color:var(--green)}
.walk h3{margin:0 0 16px;font-size:20px;letter-spacing:-.02em}
.walksteps{margin:0;padding:0;list-style:none;counter-reset:walkstep}
.walksteps li{position:relative;counter-increment:walkstep;padding:0 0 18px 46px;color:var(--muted)}
.walksteps li:last-child{padding-bottom:0}
.walksteps li:before{content:counter(walkstep);position:absolute;left:0;top:0;width:28px;height:28px;
border:1px solid var(--line);border-radius:50%;background:rgba(102,224,172,.1);color:var(--green);
text-align:center;font:700 13px/26px ui-monospace,SFMono-Regular,Menlo,monospace}
.walksteps li:not(:last-child):after{content:"";position:absolute;left:14px;top:34px;bottom:2px;border-left:1px solid var(--line)}
.walksteps b{color:var(--text)}
.walkmenu{margin:11px 0 0;padding:10px 13px;border:1px solid var(--line);border-radius:9px;
background:rgba(7,16,13,.72);color:#bfd0c9;font:12px/1.75 ui-monospace,SFMono-Regular,Menlo,monospace;overflow-x:auto}
.walk code{font:12.5px ui-monospace,SFMono-Regular,Menlo,monospace;color:#bfd0c9}
/* the walkthrough is a full-width second row of the same card rather than a
   tail on the download column: inside that column it made the column 1515px
   tall against 492px of checklist, so the checklist read as an empty panel.
   As its own row the two top columns answer each other and the steps get the
   card's whole width. Source order stays copy, walk, checklist, which is the
   reading order the stacked layout below 900px falls back to. */
#join .joincopy{grid-column:1;grid-row:1}
#join .trust{display:flex;flex-direction:column;justify-content:center;grid-column:2;grid-row:1}
#join .walk{grid-column:1/-1;grid-row:2;padding:34px 50px 50px;border-top:1px solid var(--line)}
#join .walksteps{columns:2;column-gap:44px}
#join .walksteps li{break-inside:avoid}
/* the break is forced instead of balanced so the same step always ends the
   first column, and that step is the one whose connector would otherwise
   dangle into the gutter with nothing under it. */
#join .walksteps li:nth-child(4){break-before:column}
#join .walksteps li:nth-child(3):after{content:none}
.gameshot{margin:0 0 18px}.gameshot img{display:block;width:100%;height:auto;border:1px solid var(--line);border-radius:12px}
.gameshot figcaption{margin-top:8px;font-size:13px;color:var(--muted)}
/* the screenshot column used to set a row 169px taller than the words beside
   it, and the copy just floated in the dead space. The copy is set at the
   page's reading size and the shot is capped, which brings the two columns
   within 22px of each other; the auto margins are what is left of the old
   rule, for whichever column still ends up the shorter one. */
#game .joincopy p{font-size:18px;line-height:1.7}
#game .gameshot{max-width:390px}
#game .joincopy,#game .trust{margin-block:auto}
.faq{display:grid;grid-template-columns:1fr 1fr;align-items:start;gap:12px}.faq details{border:1px solid var(--line);border-radius:12px;background:var(--surface);padding:20px 22px}
.faq summary{cursor:pointer;font-weight:700}.faq p{color:var(--muted);margin:12px 0 2px}.foot{border-top:1px solid var(--line);padding-block:34px;color:var(--muted);font-size:13px}
.footin{display:flex;justify-content:space-between;align-items:center;gap:24px}.footlinks{display:flex;flex-wrap:wrap;gap:18px}.foot a:hover{color:var(--text)}
:focus-visible{outline:3px solid rgba(117,189,242,.75);outline-offset:3px}
@media (prefers-reduced-motion:reduce){html{scroll-behavior:auto}.tinybib{animation:none;offset-distance:65%;opacity:1}.statusdot.ok{animation:none}}
@media(max-width:900px){.hero{grid-template-columns:1fr;gap:48px;padding-top:72px}.topology{min-height:410px}.snapshot{grid-template-columns:1fr 1fr}
.metric:nth-child(3){border-left:0;border-top:1px solid var(--line)}.metric:nth-child(4){border-top:1px solid var(--line)}.principles{grid-template-columns:1fr}
.principle{min-height:0}.flow{grid-template-columns:1fr 1fr}.flowitem:nth-child(3){border-left:0;border-top:1px solid var(--line)}
.flowitem:nth-child(4){border-top:1px solid var(--line)}.joinbox{grid-template-columns:1fr}
/* the card is one column here, so the explicit placement above has to go with
   it: a grid-column:2 against a single-column card would open an implicit
   second one. The steps become a single run again, which takes all three of
   the multi-column rules with it — column-count:1 does NOT cancel a forced
   break, it moves the break into a clipped overflow column and steps 4 and 5
   disappear — and gives step 3 its connector back, since there is no longer a
   gutter for it to dangle in. */
#join .joincopy,#join .trust,#join .walk{grid-column:1;grid-row:auto}
#join .walksteps{columns:1}#join .walksteps li:nth-child(4){break-before:auto}
#join .walksteps li:nth-child(3):after{content:""}
.trust{border-left:0;border-top:1px solid var(--line)}#game .gameshot{max-width:none}}
@media(max-width:860px){.nav{min-height:62px;padding-block:10px;flex-wrap:wrap;gap:10px}.links{width:100%;gap:16px;overflow-x:auto;padding-bottom:3px}.links a{white-space:nowrap}}
@media(max-width:640px){.shell{width:min(calc(100% - 24px),var(--max))}
.links{flex-wrap:wrap;overflow-x:visible;padding-bottom:0}
.hero{min-height:auto;padding-block:60px 54px}.eyebrow{font-size:10px;letter-spacing:.08em;gap:8px}.eyebrow:before{width:20px}h1{font-size:clamp(49px,16vw,72px)}.lede{font-size:18px}.topology{min-height:340px;border-radius:18px}
.snapshot{margin-top:-20px;grid-template-columns:1fr 1fr}.snapstate,.metric{min-height:96px;padding:17px}.snapv{font-size:23px}.snapstate .snapv{font-size:15px}
.snapstate{grid-column:1/-1}.snapshot .metric{border-top:1px solid var(--line)}.snapshot .metric:nth-child(2),.snapshot .metric:nth-child(4){border-left:0}.snapshot .metric:nth-child(3){border-left:1px solid var(--line)}.snapshot .metric:nth-child(4){grid-column:1/-1}
.section{padding-block:44px 0}.section:last-child{padding-bottom:44px}.sectionhd{margin-bottom:26px}.flow{grid-template-columns:1fr}.flowitem+.flowitem{border-left:0;border-top:1px solid var(--line)}
.flowitem:not(:last-child):after{content:"↓";right:24px;top:auto;bottom:-13px}.joincopy,.trust,#join .walk{padding:30px 24px}.faq{grid-template-columns:1fr}.footin{align-items:flex-start;flex-direction:column}}
</style>
</head>
<body>
<a class="skip" href="#main">Skip to content</a>
<header class="sitehd"><div class="shell nav">
  <a class="brand" href="/" aria-label="Bibites Multiverse home">
    <svg class="mark" viewBox="-9 -7 19 14" aria-hidden="true"><path fill="currentColor" d="M4.5-1.26 2.16 0l2.34 1.26C4.14 3.06 1.98 4.14-.54 4.14-3.24 4.14-5.58 2.16-6.66 0-5.58-2.16-3.24-4.14-.54-4.14c2.52 0 4.68 1.08 5.04 2.88Z"/><circle cx="1.5" cy="-1.85" r="1" fill="#07100d"/></svg>
    <span>Bibites Multiverse</span>
  </a>
  <nav class="links" aria-label="Primary navigation">
    <a href="#how">How it works</a><a href="#join">Join</a><a href="/watch">Watch broadcast</a>
    <a href="/announcements/">Announcements</a>
    <a href="https://github.com/jpinedaa/bibites-multiverse">GitHub</a>
    <a class="livepill" href="/live"><i class="dot" id="navdot"></i>Live map</a>
  </nav>
</div></header>
<main id="main">
  <section class="shell hero">
    <div>
      <p class="eyebrow">Public experiment · Aug 14–Nov 14, 2026</p>
      <h1>Evolution has a map.</h1>
      <p class="lede">Bibites Multiverse connects independent copies of <a href="#game"><em>The Bibites</em></a> as neighboring ecosystems. Organisms cross between worlds. Evolution remains local, but its consequences can travel.</p>
      <div class="actions"><a class="button primary" href="/live">Explore the live map&nbsp; →</a><a class="button secondary" href="/watch">Watch broadcast</a><a class="button secondary" href="#join">Run a world</a></div>
      <p class="promise">Every participant owns, runs, and saves a complete local world.</p>
    </div>
    <div class="topology" aria-hidden="true">
      <span class="maplabel">Live evolutionary geography</span>
      <svg class="toposvg" viewBox="0 0 360 270">
        <path class="lane hot" d="M87 83C135 83 168 83 222 83"/><path class="lane" d="M247 83C285 83 305 115 305 151"/>
        <path class="lane" d="M222 186C170 186 138 186 87 186"/><path class="lane" d="M61 105C61 132 61 151 61 164"/>
        <path class="lane" d="M247 186C280 186 305 175 305 151"/><path class="lane" d="M222 104C222 128 222 150 222 164"/>
        <rect class="node" x="37" y="60" width="50" height="45" rx="12"/><rect class="node" x="222" y="60" width="50" height="45" rx="12"/>
        <rect class="node" x="37" y="164" width="50" height="45" rx="12"/><rect class="node dark" x="222" y="164" width="50" height="45" rx="12"/>
        <rect class="node hole" x="280" y="128" width="50" height="45" rx="12"/>
        <path class="tinybib" d="M5-1.4 2.4 0 5 1.4c-.4 2-2.8 3.2-5.6 3.2-3 0-5.6-2.2-6.8-4.6 1.2-2.4 3.8-4.6 6.8-4.6C2.2-4.6 4.6-3.4 5-1.4Z"/>
      </svg>
      <div class="caption"><span>worlds stay independent</span><span>creatures cross the lanes</span></div>
    </div>
  </section>

  <section class="shell snapshot" aria-label="Live map snapshot">
    <div class="snapstate"><span class="snapk">Map status</span><span class="statusvalue"><i class="statusdot" id="statusdot" aria-hidden="true"></i><strong class="snapv" id="status" aria-live="polite">Checking the archive…</strong></span><span class="snapnote" id="age">Live, public, and read-only</span></div>
    <div class="metric"><span class="snapk">Connected worlds</span><strong class="snapv" id="worlds">—</strong><span class="snapnote">independent simulations</span></div>
    <div class="metric"><span class="snapk">Known population</span><strong class="snapv" id="population">—</strong><span class="snapnote">living Bibites</span></div>
    <div class="metric"><span class="snapk">Migrations</span><strong class="snapv" id="migrations">—</strong><span class="snapnote" id="rate">recorded crossings</span></div>
  </section>

  <section class="shell section" id="game">
    <div class="sectionhd"><span class="kicker">The original game</span><h2><em>The Bibites</em></h2><p>Every world on this map runs a copy of the game.</p></div>
    <div class="joinbox">
      <div class="joincopy"><p><em>The Bibites</em> is an artificial-life simulation created by Léo Caussan in 2017 and developed by <a href="https://www.thebibites.com/">Omnia Studios</a>. Every creature in it carries its own genes and a neural-network brain. Nothing scores them. Food, mutation, and natural selection do the work, until behavior like pheromone-trail hunting or stockpiling food appears on its own. Its creator's stated goal is to prove that “life doesn't have to be carbon-based.”</p>
      <p>It has been in open development since 2017, with public devlogs since 2019 and a talk at the ALIFE 2020 conference. Bibites Multiverse only gives it neighbors. The simulation itself is theirs.</p>
      <p>Bibites Multiverse is an independent passion project, built out of an interest in artificial life. It is not affiliated with, endorsed by, or sponsored by Léo Caussan or Omnia Studios.</p></div>
      <aside class="trust"><figure class="gameshot"><img src="/game-screenshot.jpg" width="1280" height="720" alt="A bibite in The Bibites, with its biology panel open and its field of view drawn as a cone." loading="lazy"><figcaption>A world running in <em>The Bibites</em>, captured from the Multiverse broadcast.</figcaption></figure><h3>Get <em>The Bibites</em></h3><p><em>The Bibites: Digital Life</em> is free on itch.io and in Early Access on Steam, for Windows, macOS, and Linux. Get it from the people who made it.</p>
      <div class="actions"><a class="button primary" href="https://store.steampowered.com/app/2736860/The_Bibites_Digital_Life/">Get it on Steam</a><a class="button secondary" href="https://thebibites.itch.io/the-bibites">Free on itch.io</a></div>
      <p class="promise">Official channels: <a href="https://www.thebibites.com/">website</a> · <a href="https://www.youtube.com/@TheBibitesDigitalLife">YouTube</a> · <a href="https://discord.com/invite/rNDMdNjQ2R">Discord</a> · <a href="https://www.patreon.com/thebibites">Patreon</a> · <a href="https://the-bibites.fandom.com/wiki/The_Bibites_Wiki">community wiki</a></p></aside>
    </div>
  </section>

  <section class="shell section" id="about">
    <div class="sectionhd"><span class="kicker">The idea</span><h2>One map. Many evolutionary pressures.</h2><p>This is not one synchronized mega-simulation. Each world develops on its own, and the borders between them become migration routes.</p></div>
    <div class="principles">
      <article class="principle"><h3>Independent worlds</h3><p>Every participant controls a separate game, with its own clock, climate, settings, food, predators, and save files.</p></article>
      <article class="principle"><h3>Real migration</h3><p>A Bibite that crosses an edge can enter another participant's ecosystem and meet a completely different selection pressure.</p></article>
      <article class="principle"><h3>Shared history</h3><p>The public archive follows crossings, population, species, lineages, and changes in brain complexity without controlling any world.</p></article>
    </div>
  </section>

  <section class="shell section" id="how">
    <div class="sectionhd"><span class="kicker">How it works</span><h2>A durable path between local simulations.</h2><p>The migration path is encrypted, retry-safe, and designed to prefer a rare loss over ever duplicating an organism.</p></div>
    <div class="flow" aria-label="System flow">
      <div class="flowitem"><b>The Bibites</b><span>The game itself, running on your machine</span></div>
      <div class="flowitem"><b>Multiverse mod</b><span>Watches the world's borders</span></div>
      <div class="flowitem"><b>Participant sidecar</b><span>Keeps durable custody in transit</span></div>
      <div class="flowitem"><b>Relay + archive</b><span>Routes migrations and records history</span></div>
    </div>
  </section>

  <section class="shell section" id="join">
    <div class="joinbox">
      <div class="joincopy"><span class="kicker">Join the experiment</span><h2>Give your world neighbors.</h2><p>The Windows setup and Linux complete package below are always the newest release. They include <em>The Bibites</em> __HOMEPAGE_GAME_VERSION__, the mod, and the connector. Each installer creates a unique world identity and keeps its secret on your machine. No join string is required. Add-on packages exist for anyone who already owns a supported copy.</p><div class="actions"><a class="button primary" href="__HOMEPAGE_WINDOWS__">Download for Windows</a><a class="button primary" href="__HOMEPAGE_LINUX__">Download for Linux</a><a class="button secondary" href="__HOMEPAGE_TAG__">Checksums and add-ons&nbsp; →</a></div>__HOMEPAGE_RELEASE__</div>
      <div class="walk">
        <h3>On Windows, five steps.</h3>
        <ol class="walksteps">
          <li><b>Download the setup.</b> It is one file. The checksums page beside these buttons lets you verify it first, if you want to.</li>
          <li><b>Run it, and expect the warning.</b> This is a community build with no code-signing certificate, so SmartScreen shows <b>Windows protected your PC</b>. Select <b>More info</b>, then <b>Run anyway</b>.</li>
          <li><b>Press Install.</b> Keep the included copy of <em>The Bibites</em>, or point the setup at a game you already own. It needs no administrator account, no sign-up, and no join code: it gives this world its own identity on the map while it installs, and starts the world when it finishes.</li>
          <li><b>Open Bibites Multiverse, then press Enter.</b> The desktop and Start Menu icon of that name opens the launcher: it says which world it is set to and whether that world is running, above a short menu. Enter starts it, and the world reaches the <a href="/live">live map</a> about a minute later.<pre class="walkmenu">1) Start this world   [Enter]
2) Stop this world</pre></li>
          <li><b>Stop it from the same menu.</b> Option <b>2</b> asks the game to close and waits for its save, so stopping is not losing. Option <b>6</b> creates another world, when you want more than one on this computer.</li>
        </ol>
        <p class="promise">On Linux: unpack the complete package, run <code>./install-bibites-multiverse.sh</code>, then <code>./start-multiverse.sh</code>. The <a href="__HOMEPAGE_DOCS__">install guide</a> has both platforms in full.</p>
      </div>
      <aside class="trust"><h3>Clear boundaries</h3><ul><li>Your world and saves stay on your machine.</li><li>Automatic enrollment creates the secret on your machine.</li><li>The public page is read-only.</li><li>TLS protects traffic to the relay.</li><li>Published world data is explained before you join.</li><li>The shared run ends November 14 unless extended by announcement.</li></ul></aside>
    </div>
  </section>

  <section class="shell section" aria-labelledby="faqtitle">
    <div class="sectionhd"><span class="kicker">Good to know</span><h2 id="faqtitle">Before you cross the border.</h2></div>
    <div class="faq">
      <details><summary>Does the map control my simulation?</summary><p>No. The public page is read-only, and every participant continues to control and save their own local world.</p></details>
      <details><summary>What happens when another world goes offline?</summary><p>The map can route around a dark world when the axis has another live destination. Its reserved place remains waiting for it.</p></details>
      <details><summary>Is the public run permanent?</summary><p>No. The announced run is August 14 through November 14, 2026. It can be extended by announcement and is never silently shortened.</p></details>
      <details><summary>Is this an official Bibites project?</summary><p>No. Bibites Multiverse is an independent passion project, built out of an interest in artificial life. <em>The Bibites</em> was created by Léo Caussan and is developed by Omnia Studios; this project is not affiliated with, endorsed by, or sponsored by either of them.</p></details>
      <details><summary>Is this the game's own multiplayer?</summary><p>No. Omnia Studios has announced its own plans for linked simulation realms. Bibites Multiverse is a separate, independent experiment that runs today, and it is not connected to that work.</p></details>
    </div>
  </section>
</main>
<footer class="foot"><div class="shell footin"><span>Independent community project · Apache-2.0</span><div class="footlinks"><a href="/announcements/">Announcements</a><a href="/watch">Watch broadcast</a><a href="/live">Live map</a><a href="https://github.com/jpinedaa/bibites-multiverse">Source</a><a href="https://github.com/jpinedaa/bibites-multiverse/tree/main/docs">Documentation</a><a href="https://store.steampowered.com/app/2736860/The_Bibites_Digital_Life/">The Bibites on Steam</a><a href="https://thebibites.itch.io/the-bibites">itch.io</a></div></div></footer>
<script>
(function(){
  function n(v){ return v == null ? "unknown" : Number(v).toLocaleString(); }
  function age(ms){ var s=Math.max(0,Math.round(ms/1000)); return s<60?s+" seconds ago":Math.floor(s/60)+" minutes ago"; }
  async function refresh(){
    try{
      var r=await fetch("/api/status",{cache:"no-store"});
      if(!r.ok) throw new Error("status");
      var d=await r.json(), t=d.totals||{}, linked=!!(d.relayConnected&&d.haveStatus);
      document.getElementById("navdot").className="dot"+(linked?" ok":"");
      document.getElementById("statusdot").className="statusdot"+(linked?" ok":!d.relayConnected?" down":"");
      document.getElementById("worlds").textContent=n(t.liveSlots);
      document.getElementById("population").textContent=n(t.population);
      document.getElementById("migrations").textContent=n(t.migrations);
      document.getElementById("rate").textContent=t.perMinute==null?"recorded crossings":Number(t.perMinute).toFixed(1)+" per minute";
      document.getElementById("age").textContent=d.haveStatus?"Updated "+age(d.statusAgeMs):"Waiting for the first relay status";
      document.getElementById("status").textContent=!d.relayConnected?"Archive disconnected":!d.haveStatus?"Waiting for the map":t.liveSlots===0?"Online · no worlds connected":"Online · migrations flowing";
    }catch(e){
      document.getElementById("statusdot").className="statusdot down";
      document.getElementById("status").textContent="Live snapshot unavailable";
      document.getElementById("age").textContent="The map can continue while this page recovers";
    }
  }
  refresh(); setInterval(refresh,15000);
})();
</script>
</body>
</html>`

// landingPageHTML is the page WITH NO RELEASE RESOLVED — the fail-silent
// baseline of the version line below, and the form every reader saw before that
// line existed. It is what the tests assert against, because it is the version
// of this page that must be complete and correct on its own.
var landingPageHTML = renderLandingPage(Config{
	HomepageRepo:        defaultHomepageRepo(),
	HomepageGameVersion: defaultHomepageGameVersion(),
}, "")

// THE DOWNLOAD LINKS NAME NO RELEASE, ON PURPOSE. GitHub answers
// /releases/latest/download/<name> out of whichever release is newest, and
// release/make-release.sh publishes a stable-named copy of each of the two
// packages this page links for exactly that reason. So publishing a release
// moves this page on its own: no rebuild, no host environment value, and no
// archive deployment is part of a release any more.
//
// THE RELEASE NUMBER BESIDE THEM IS NOT A CONTRADICTION OF THAT — it is the
// same rule applied to the label. What the buttons gained in staying current
// they cost the reader in legibility: three buttons that always serve the
// newest release cannot, by themselves, say WHICH release that is, so a visitor
// could not tell what they were about to install or whether it had moved since
// their last visit. `releaseTag` answers that from the SAME authority the
// buttons resolve against, looked up at run time (release.go) rather than
// compiled in, so it stays true across releases with no deployment — which is
// the property the release-independent links were for in the first place. A
// baked-in number would have traded it away and gone quietly stale.
//
// AN EMPTY releaseTag IS A FIRST-CLASS CASE, not a degraded one: the whole
// version line is dropped and the page is byte-identical to the one that
// shipped before it. Nothing else on the page depends on it.
//
// The Windows walkthrough's install-guide link follows the same rule from the
// other side: it names the repository's `main`, never a tag, so the page it
// opens is the guide as it stands rather than the guide a past release shipped.
func renderLandingPage(cfg Config, releaseTag string) string {
	gameVersion := strings.TrimSpace(cfg.HomepageGameVersion)
	if gameVersion == "" {
		gameVersion = defaultHomepageGameVersion()
	}
	repo := normalizeHomepageRepo(cfg.HomepageRepo)
	if repo == "" {
		repo = defaultHomepageRepo()
	}
	latest := "https://github.com/" + repo + "/releases/latest"
	return strings.NewReplacer(
		"__HOMEPAGE_GAME_VERSION__", gameVersion,
		"__HOMEPAGE_RELEASE__", releaseLine(releaseTag),
		"__HOMEPAGE_WINDOWS__", latest+"/download/bibites-multiverse-windows-x64-setup.exe",
		"__HOMEPAGE_LINUX__", latest+"/download/bibites-multiverse-linux-x64-complete.zip",
		"__HOMEPAGE_TAG__", latest,
		"__HOMEPAGE_DOCS__", "https://github.com/"+repo+"/blob/main/docs/participant/install.md",
	).Replace(landingPageTemplate)
}

// releaseLine renders the version line, or nothing at all.
//
// It says "release" and not "version" because the join card already carries a
// second number — the copy above the buttons names the build of *The Bibites*
// each package includes — and the two must never be read as one. The game's
// number is attached to the game's name in the sentence that mentions it; this
// one is attached to the word "release" and sits under the buttons that fetch
// it. The tag is rendered as GitHub publishes it, "v" and all, so that it
// matches what a reader lands on when they follow the third button.
//
// The gate on it is tagLooksLikeRelease and not an HTML escape, for the reason
// given in release.go: the page would rather show nothing than render an
// answer it does not recognise.
func releaseLine(tag string) string {
	tag = strings.TrimSpace(tag)
	if !tagLooksLikeRelease(tag) {
		return ""
	}
	return `<p class="release">Latest release <b>` + tag + `</b></p>`
}

func defaultHomepageRepo() string        { return "jpinedaa/bibites-multiverse" }
func defaultHomepageGameVersion() string { return "0.6.3.1" }

func normalizeHomepageRepo(repo string) string {
	repo = strings.TrimSpace(repo)
	repo = strings.TrimSuffix(repo, "/")
	repo = strings.TrimPrefix(repo, "https://github.com/")
	repo = strings.TrimPrefix(repo, "http://github.com/")
	repo = strings.Trim(repo, "/")
	if repo == "" {
		return defaultHomepageRepo()
	}
	return repo
}

const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
<rect width="64" height="64" rx="14" fill="#0b1110"/>
<path fill="#66e0ac" d="M47 27 39 32l8 5c-2 9-11 14-21 14-11 0-20-8-24-19 4-11 13-19 24-19 10 0 19 5 21 14Z"/>
<circle cx="36" cy="24" r="4" fill="#07100d"/>
</svg>`

const socialCardSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630">
<defs><linearGradient id="bg" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#0b1110"/><stop offset="1" stop-color="#162b24"/></linearGradient><filter id="g"><feGaussianBlur stdDeviation="8"/></filter></defs>
<rect width="1200" height="630" fill="url(#bg)"/><g opacity=".18" stroke="#75bdf2"><path d="M0 110h1200M0 220h1200M0 330h1200M0 440h1200M0 550h1200M170 0v630M340 0v630M510 0v630M680 0v630M850 0v630M1020 0v630"/></g>
<g transform="translate(774 120)"><path d="M90 75C170 75 202 75 282 75M90 225C170 225 202 225 282 225M65 100v100M307 100v100" fill="none" stroke="#75bdf2" stroke-width="5" stroke-dasharray="12 15" opacity=".75"/><rect x="15" y="25" width="100" height="100" rx="24" fill="#13231f" stroke="#66e0ac" stroke-width="5"/><rect x="257" y="25" width="100" height="100" rx="24" fill="#13231f" stroke="#66e0ac" stroke-width="5"/><rect x="15" y="175" width="100" height="100" rx="24" fill="#13231f" stroke="#66e0ac" stroke-width="5"/><rect x="257" y="175" width="100" height="100" rx="24" fill="#13231f" stroke="#e86c76" stroke-width="5" stroke-dasharray="9 9"/><path fill="#66e0ac" filter="url(#g)" d="M204 67l-15 8 15 8c-3 14-18 22-35 22-18 0-33-13-40-30 7-17 22-30 40-30 17 0 32 8 35 22Z"/><path fill="#66e0ac" d="M204 67l-15 8 15 8c-3 14-18 22-35 22-18 0-33-13-40-30 7-17 22-30 40-30 17 0 32 8 35 22Z"/></g>
<text x="74" y="115" fill="#66e0ac" font-family="ui-monospace,monospace" font-size="22" font-weight="700" letter-spacing="3">BIBITES MULTIVERSE</text><text x="70" y="265" fill="#eff7f3" font-family="ui-sans-serif,system-ui,sans-serif" font-size="76" font-weight="760" letter-spacing="-4">Evolution</text><text x="70" y="347" fill="#eff7f3" font-family="ui-sans-serif,system-ui,sans-serif" font-size="76" font-weight="760" letter-spacing="-4">has a map.</text><text x="74" y="430" fill="#9aafa7" font-family="ui-sans-serif,system-ui,sans-serif" font-size="28">Independent worlds. Real migrations.</text><text x="74" y="470" fill="#9aafa7" font-family="ui-sans-serif,system-ui,sans-serif" font-size="28">Shared evolutionary history.</text>
</svg>`

const notFoundPageHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Not found — Bibites Multiverse</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#0b1110;color:#eff7f3;font:16px/1.6 system-ui,sans-serif}main{max-width:560px;padding:36px}p{color:#9aafa7}a{display:inline-block;margin-top:14px;color:#07100d;background:#66e0ac;padding:11px 16px;border-radius:8px;text-decoration:none;font-weight:700}</style></head><body><main><small>404 · UNMAPPED COORDINATE</small><h1>This world is not on the map.</h1><p>The address does not name a public page or archive endpoint.</p><a href="/">Return to the Multiverse</a></main></body></html>`
