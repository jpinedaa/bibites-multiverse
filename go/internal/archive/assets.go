package archive

// The social cards are the ONE part of this website that is not read by a
// browser. Every other byte the archive serves is HTML, JSON, or an SVG a
// browser draws; a link preview is read by a scraper — Facebook, WhatsApp, X,
// LinkedIn, Discord, Slack, iMessage — and NOT ONE OF THEM RENDERS SVG. A
// vector card in og:image is the same as no card at all, which is what the
// homepage shipped with until these files existed.
//
// So the cards are raster, they are 1200x630 (the one size every scraper above
// crops predictably), and they are EMBEDDED rather than read from disk. The
// archive is a single binary that has to keep serving its page when it is the
// only thing still running, from whatever directory an operator started it in;
// an asset it has to find at runtime is an asset it can fail to find. The rest
// of the site's assets are Go constants for the same reason — these are byte
// slices instead only because a PNG is not text.
//
// The three cards are separate images because the three pages make three
// different promises: the front door introduces the project, /watch is one live
// camera, /live is the map. A shared card would make every link look like the
// homepage.
//
// EDITING A CARD. The PNGs are generated, not drawn by hand. assets/home.svg,
// assets/watch.svg and assets/live.svg are the sources; assets/render.sh builds
// a 1200x630 wrapper page for each one, which is then screenshotted at exactly
// that viewport to produce the PNG beside it. Change the SVG, re-render, and
// commit both. Do not rasterize with ImageMagick: its SVG delegate is
// rsvg-convert, and where that is missing it silently falls back to an internal
// renderer that mangles text and gradients.
//
// The cards are palette-quantized (render.sh --quantize) because they are paid
// for by the byte. A browser screenshot of this artwork is ~800KB of 24-bit PNG
// for a few dozen brand colours; 256 colours without dithering renders
// identically at ~130KB, and every link share pulls a whole card. The nginx
// front door names page egress as the largest single cost term in this service,
// so a 6x saving on an asset built to be fetched by strangers is worth the one
// extra step. Note the archive gzips anything over 1400 bytes on the way out,
// which buys nothing on an already-compressed PNG — the quantize step is where
// the actual saving comes from.

import _ "embed"

//go:embed assets/social-card.png
var socialCardPNG []byte

//go:embed assets/social-card-watch.png
var socialCardWatchPNG []byte

//go:embed assets/social-card-live.png
var socialCardLivePNG []byte
