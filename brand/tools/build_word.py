"""Wordmark from the potrace output, plus mark+wordmark lockups."""
import re, sys
import numpy as np
from PIL import Image

TRACE, MASK, OUT = sys.argv[1:4]
svg = open(TRACE).read()
g = re.search(r'<g transform="([^"]+)"[^>]*>(.*?)</g>', svg, re.S)
transform, paths = g.group(1), re.sub(r"\s+", " ", g.group(2)).strip()

# tight box from the trace rendered on its own pixel grid; the thresholded mask
# undershoots, because potrace's curves bulge past the pixel corners
m = np.asarray(Image.open(MASK).convert("RGBA"))[..., 3] > 0
ys, xs = np.where(m)
P = 16  # breathing room so no renderer clips an antialiased edge
x0, x1, y0, y1 = xs.min() - P, xs.max() + 1 + P, ys.min() - P, ys.max() + 1 + P
WW, WH = x1 - x0, y1 - y0


def inner(fill):
    return f'<g fill="{fill}" transform="translate({-x0} {-y0}) {transform}">{paths}</g>'


def wordmark(fill):
    return f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {WW} {WH}" role="img" aria-label="Postitheka">{inner(fill)}</svg>\n'


INK = {"dark": "#EEF3F9", "light": "#1A2838"}
for k, f in INK.items():
    open(f"{OUT}/wordmark-on-{k}.svg", "w").write(wordmark(f))
open(f"{OUT}/wordmark-mono.svg", "w").write(wordmark("currentColor"))

mark_vb = {}
for k in INK:
    s = open(f"{OUT}/mark-on-{k}.svg").read()
    vb = re.search(r'viewBox="([^"]+)"', s).group(1)
    mark_vb[k] = (vb, s.split(">", 1)[1].rsplit("</svg>", 1)[0])
MW, MH = map(float, mark_vb["dark"][0].split()[2:])


def nest(x, y, w, h, vb, body):
    return f'<svg x="{x:.1f}" y="{y:.1f}" width="{w:.1f}" height="{h:.1f}" viewBox="{vb}">{body}</svg>'


for k, f in INK.items():
    vb, body = mark_vb[k]
    # horizontal: mark height = 2.2x wordmark cap height, wordmark centred on it
    mh = WH * 2.2
    mw = MW * mh / MH
    gap = WH * 0.7
    W, H = mw + gap + WW, mh
    s = (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W:.1f} {H:.1f}" role="img" aria-label="Postitheka">'
         + nest(0, 0, mw, mh, vb, body)
         + nest(mw + gap, (H - WH) / 2, WW, WH, f"0 0 {WW} {WH}", inner(f)) + "</svg>\n")
    open(f"{OUT}/lockup-horizontal-on-{k}.svg", "w").write(s)
    # stacked, as in the source art: mark above, wordmark ~2x mark width
    mw = WW / 2.0
    mh = MH * mw / MW
    gap = WH * 0.8
    W, H = WW, mh + gap + WH
    s = (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W:.1f} {H:.1f}" role="img" aria-label="Postitheka">'
         + nest((W - mw) / 2, 0, mw, mh, vb, body)
         + nest(0, mh + gap, WW, WH, f"0 0 {WW} {WH}", inner(f)) + "</svg>\n")
    open(f"{OUT}/lockup-stacked-on-{k}.svg", "w").write(s)
print("wordmark", WW, WH)
