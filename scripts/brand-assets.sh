#!/usr/bin/env bash
# Regenerates every raster brand asset (favicons, app icons, social images) from
# the Dakiya Mail mark. The vector mark also lives inline in the files listed by
# `git grep -l "$MARK"`; change it there and here together, then rerun this.
# Needs: rsvg-convert (librsvg), sips (macOS), dwebp (libwebp), python3.
set -euo pipefail
cd "$(dirname "$0")/.."

# The mark in its 746x764 design box: an envelope with the flap cut out.
MARK='M146 182H600A56 56 0 0 1 656 238V526A56 56 0 0 1 600 582H146A56 56 0 0 1 90 526V238A56 56 0 0 1 146 182ZM146 232V300L373 468L600 300V232L373 400Z'

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# icon <name> <background-svg-fragment> <mark-fill> <corner-radius> <mark-fraction>
# Writes a 1024x1024 SVG with the mark centered at <mark-fraction> of the width.
icon() {
  local name="$1" bg="$2" fill="$3" rx="$4" frac="$5"
  local w h x y
  w=$(python3 -c "print(1024*$frac)")
  h=$(python3 -c "print(1024*$frac*764/746)")
  x=$(python3 -c "print((1024-1024*$frac)/2)")
  y=$(python3 -c "print((1024-1024*$frac*764/746)/2)")
  cat > "$tmp/$name.svg" <<SVG
<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024">
  <defs>
    <linearGradient id="sky" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="#38bdf8"/><stop offset="1" stop-color="#0284c7"/>
    </linearGradient>
    <clipPath id="round"><rect width="1024" height="1024" rx="$rx"/></clipPath>
  </defs>
  <g clip-path="url(#round)">$bg</g>
  <svg x="$x" y="$y" width="$w" height="$h" viewBox="0 0 746 764"><path d="$MARK" fill="$fill"/></svg>
</svg>
SVG
}

png() { rsvg-convert -w "$2" -h "$2" "$tmp/$1.svg" -o "$3"; }

# ico <out> <png>...: PNG-in-ICO, read by every current browser.
ico() {
  python3 -I - "$@" <<'PY'
import struct, sys
out, pngs = sys.argv[1], sys.argv[2:]
data = [open(p, "rb").read() for p in pngs]
sizes = [struct.unpack(">II", d[16:24]) for d in data]
offset = 6 + 16 * len(data)
head = struct.pack("<HHH", 0, 1, len(data))
for d, (w, h) in zip(data, sizes):
    head += struct.pack("<BBBBHHII", w % 256, h % 256, 0, 0, 1, 32, len(d), offset)
    offset += len(d)
open(out, "wb").write(head + b"".join(data))
PY
}

icon app     '<rect width="1024" height="1024" fill="url(#sky)"/>' '#ffffff' 0   0.62
icon tab     '<rect width="1024" height="1024" fill="url(#sky)"/>' '#ffffff' 224 0.66
icon dark    '<rect width="1024" height="1024" fill="#0b1220"/>'   '#38bdf8' 0   0.62
icon tinted  '<rect width="1024" height="1024" fill="#000000"/>'   '#ffffff' 0   0.62

# Dashboard (web) and marketing site share one icon set.
for dir in web/public site/public; do
  for s in 16 32 48 96; do png tab "$s" "$dir/favicon-${s}x${s}.png"; done
  png app 180 "$dir/apple-touch-icon.png"
  png app 192 "$dir/web-app-manifest-192x192.png"
  png app 512 "$dir/web-app-manifest-512x512.png"
  ico "$dir/favicon.ico" "$dir/favicon-16x16.png" "$dir/favicon-32x32.png" "$dir/favicon-48x48.png"
done

# Admin panel: rendered from its own SVG (mark on slate, admin red).
rsvg-convert -w 16 -h 16 admin/public/admin-icon.svg -o admin/public/favicon-16x16.png
rsvg-convert -w 32 -h 32 admin/public/admin-icon.svg -o admin/public/favicon-32x32.png
rsvg-convert -w 48 -h 48 admin/public/admin-icon.svg -o "$tmp/admin-48.png"
rsvg-convert -w 180 -h 180 admin/public/admin-icon.svg -o admin/public/apple-touch-icon.png
rsvg-convert -w 192 -h 192 admin/public/admin-icon.svg -o admin/public/icon-192.png
rsvg-convert -w 512 -h 512 admin/public/admin-icon.svg -o admin/public/icon-512.png
ico admin/public/favicon.ico admin/public/favicon-16x16.png admin/public/favicon-32x32.png "$tmp/admin-48.png"

# Marketing brand kit.
png app 128 site/public/brand/app-icon.png
png app 1024 "$tmp/avatar.png"
sips -s format jpeg -s formatOptions 90 "$tmp/avatar.png" --out site/public/brand/social-avatar.jpg >/dev/null

# iOS asset-catalog icons (light, dark, tinted). The .icon bundle uses Assets/plane.svg.
ios=ios/DakiyaMail/Assets.xcassets/AppIcon.appiconset
png app 1024 "$ios/AppIcon-Light.png"
png dark 1024 "$ios/AppIcon-Dark.png"
png tinted 1024 "$ios/AppIcon-Tinted.png"

# Open Graph card (1280x640): the site's sky, two of its clouds, and the lockup.
dwebp -quiet site/public/backdrops/cloud-2.webp -o "$tmp/cloud-a.png"
dwebp -quiet site/public/backdrops/cloud-4.webp -o "$tmp/cloud-b.png"
cat > "$tmp/og.svg" <<SVG
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="1280" height="640" viewBox="0 0 1280 640">
  <defs>
    <radialGradient id="sky" cx="0.72" cy="0.25" r="1.1">
      <stop offset="0" stop-color="#7dd3fc"/><stop offset="0.18" stop-color="#38bdf8"/>
      <stop offset="0.36" stop-color="#0ea5e9"/><stop offset="0.58" stop-color="#0284c7"/>
      <stop offset="0.82" stop-color="#075985"/>
    </radialGradient>
  </defs>
  <rect width="1280" height="640" fill="url(#sky)"/>
  <image xlink:href="cloud-a.png" x="-120" y="400" width="760" height="380" opacity="0.85" preserveAspectRatio="xMidYMid meet"/>
  <image xlink:href="cloud-b.png" x="760" y="-40" width="620" height="310" opacity="0.7" preserveAspectRatio="xMidYMid meet"/>
  <svg x="318" y="250" width="132" height="135" viewBox="0 0 746 764"><path d="$MARK" fill="#ffffff"/></svg>
  <text x="480" y="352" font-family="Inter, Helvetica Neue, Helvetica, Arial, sans-serif" font-weight="800" font-size="96" letter-spacing="-2" fill="#ffffff">Dakiya Mail</text>
</svg>
SVG
rsvg-convert "$tmp/og.svg" -o "$tmp/og.png"
sips -s format jpeg -s formatOptions 88 "$tmp/og.png" --out site/public/og-image.jpg >/dev/null

echo "Brand assets regenerated."
