#!/usr/bin/env bash
# Rebuild the social cards from their SVG sources.
#
# The PNGs beside this script are generated, never hand-drawn. Edit home.svg,
# watch.svg or live.svg, run this, and commit both the SVG and the PNG.
#
# Rasterizing is done by a real browser, on purpose. ImageMagick's SVG delegate
# is rsvg-convert, and on a box where that is not installed ImageMagick silently
# falls back to an internal renderer that mangles text, gradients and filters —
# it does not fail, it just produces a wrong card. So this script only prepares
# the wrapper pages; screenshot them at exactly 1200x630 with a browser, save
# each over its social-card*.png, then run this script with --quantize.
#
# Why quantize: a browser screenshot of these cards is ~800KB of 24-bit PNG for
# artwork that uses a few dozen brand colours. The archive's own nginx comment
# names page egress as the largest single cost term in the service, and every
# link share pulls a full card. A 256-colour palette with dithering DISABLED
# takes them to ~130KB with no visible change — dithering is what to avoid here,
# not the palette: it adds visible noise across the large flat dark areas and
# makes the file bigger. Verify by eye after running, especially the soft glow
# on the watch card, which is the first thing that would band.
set -euo pipefail
D="$(cd "$(dirname "$0")" && pwd)"
CARDS=(social-card:home social-card-watch:watch social-card-live:live)

if [ "${1:-}" = "--quantize" ]; then
  for entry in "${CARDS[@]}"; do
    png="$D/${entry%%:*}.png"
    [ -f "$png" ] || { echo "missing $png" >&2; exit 1; }
    convert "$png" -strip +dither -colors 256 -define png:compression-level=9 "$png"
    printf '%-26s %s\n' "$(basename "$png")" "$(du -h "$png" | cut -f1)"
  done
  # The dimensions are the contract the og:image:width/height tags promise, and
  # the Go test asserts them. Catch a bad render here rather than in CI.
  identify -format '%f %wx%h\n' "$D"/social-card*.png
  exit 0
fi

for entry in "${CARDS[@]}"; do
  name="${entry##*:}"
  {
    printf '<!doctype html><meta charset="utf-8"><style>html,body{margin:0;padding:0;overflow:hidden;background:#0b1110;width:1200px;height:630px}svg{display:block}</style>'
    cat "$D/$name.svg"
  } > "$D/$name.html"
done
echo "wrapper pages written. Screenshot each at exactly 1200x630, save over the"
echo "matching social-card*.png, then re-run: $0 --quantize"
