"""Rebuild the Postitheka P mark as clean geometry, measured from the source render.

Every diagonal shares one slope K, bands share one thickness, gaps share one width,
so the mark stays crisp at 16 px and at 1024 px.
"""
import sys, os

OUT = sys.argv[1]
K = 0.44          # slope of every diagonal (~24 deg)
X0, XS, XI, XR = 46, 145, 430, 510   # left edge, stem right, inner right, outer right
AX, AY = 290, 25  # apex
T = 86            # vertical thickness of the diagonal bands
YBR = 265         # outer bottom-right corner


def up(x0, y0, x):  # line rising to the right through (x0, y0)
    return y0 - K * (x - x0)


def down(x0, y0, x):  # line falling to the right
    return y0 + K * (x - x0)


body = [
    (X0, AY + K * (AX - X0)),                   # outer top-left
    (AX, AY),                                   # apex
    (XR, down(AX, AY, XR)),                     # outer top-right
    (XR, YBR),                                  # outer bottom-right
    (XS, YBR + K * (XR - XS)),                  # stem inner corner
    (XS, YBR + K * (XR - XS) + 87),             # stem bottom-right
    (X0, YBR + K * (XR - XS) + 87 - K * (XS - X0)),  # stem bottom-left
    (X0, YBR - T + K * (XR - X0)),              # lower band top-left
    (XI, YBR - T + K * (XR - XI)),              # inner bottom-right
    (XI, down(AX, AY + T, XI)),                 # inner top-right
    (AX, AY + T),                               # inner apex
    (X0, AY + T + K * (AX - X0)),               # top band bottom-left
]

# blue bar: centred in the gap between the two bands, right end at x=278
BAR_TOP_L, BAR_H, BAR_XR = 264, 72, 278
bar = [
    (X0, BAR_TOP_L),
    (BAR_XR, BAR_TOP_L - K * (BAR_XR - X0)),
    (BAR_XR, BAR_TOP_L - K * (BAR_XR - X0) + BAR_H),
    (X0, BAR_TOP_L + BAR_H),
]

# foot: falls to the right, same slope
FX0, FX1, FY, FH = 185, 275, 455, 77
foot = [(FX0, FY), (FX1, FY + K * (FX1 - FX0)), (FX1, FY + K * (FX1 - FX0) + FH), (FX0, FY + FH)]

pts = body + bar + foot
minx = min(p[0] for p in pts); maxx = max(p[0] for p in pts)
miny = min(p[1] for p in pts); maxy = max(p[1] for p in pts)
R = 9  # corner radius of the render's soft outer corners


def inset(poly, r):
    """Offset every edge inward by r. Stroking the result 2r wide with round joins
    rounds the convex corners and restores the outline, so gaps keep their width."""
    n = len(poly)
    area = sum(poly[i][0] * poly[(i + 1) % n][1] - poly[(i + 1) % n][0] * poly[i][1] for i in range(n))
    sgn = 1 if area > 0 else -1
    lines = []
    for i in range(n):
        (x1, y1), (x2, y2) = poly[i], poly[(i + 1) % n]
        dx, dy = x2 - x1, y2 - y1
        ln = (dx * dx + dy * dy) ** 0.5
        nx, ny = -dy / ln * sgn, dx / ln * sgn  # inward normal
        lines.append(((x1 + nx * r, y1 + ny * r), (dx, dy)))
    out = []
    for i in range(n):
        (p, d1), (q, d2) = lines[i - 1], lines[i]
        den = d1[0] * d2[1] - d1[1] * d2[0]
        t = ((q[0] - p[0]) * d2[1] - (q[1] - p[1]) * d2[0]) / den
        out.append((p[0] + d1[0] * t, p[1] + d1[1] * t))
    return out


pad = 0
W = maxx - minx + 2 * pad
H = maxy - miny + 2 * pad


def d(poly):
    return "M" + " L".join(f"{x - minx + pad:.1f} {y - miny + pad:.1f}" for x, y in poly) + " Z"


PATHS = {k: d(inset(v, R)) for k, v in {"body": body, "bar": bar, "foot": foot}.items()}


def svg(body_fill, bar_fill, defs="", vb=None, extra_bar=""):
    vbx = vb or f"0 0 {W:.1f} {H:.1f}"
    s = lambda f: f'fill="{f}" stroke="{f}" stroke-width="{2 * R}" stroke-linejoin="round"'
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vbx}" role="img" aria-label="Postitheka">'
        f"{defs}"
        f'<path d="{PATHS["body"]}" {s(body_fill)}/>'
        f'<path d="{PATHS["foot"]}" {s(body_fill)}/>'
        f'<path d="{PATHS["bar"]}" {s(bar_fill)}{extra_bar}/>'
        "</svg>\n"
    )


GRAD = (
    "<defs>"
    '<linearGradient id="pk-body" x1="0" y1="0" x2="0.35" y2="1">'
    '<stop offset="0" stop-color="#F5F8FC"/><stop offset="1" stop-color="#BFCADB"/></linearGradient>'
    '<linearGradient id="pk-bar" x1="0" y1="1" x2="1" y2="0">'
    '<stop offset="0" stop-color="#4FC3FF"/><stop offset="1" stop-color="#2E86F0"/></linearGradient>'
    "</defs>"
)
GRAD_LIGHT = (
    "<defs>"
    '<linearGradient id="pk-body" x1="0" y1="0" x2="0.35" y2="1">'
    '<stop offset="0" stop-color="#26364A"/><stop offset="1" stop-color="#121B27"/></linearGradient>'
    '<linearGradient id="pk-bar" x1="0" y1="1" x2="1" y2="0">'
    '<stop offset="0" stop-color="#3FB4F5"/><stop offset="1" stop-color="#2273DC"/></linearGradient>'
    "</defs>"
)

os.makedirs(OUT, exist_ok=True)
open(f"{OUT}/mark-on-dark.svg", "w").write(svg("url(#pk-body)", "url(#pk-bar)", GRAD))
open(f"{OUT}/mark-on-light.svg", "w").write(svg("url(#pk-body)", "url(#pk-bar)", GRAD_LIGHT))
open(f"{OUT}/mark-mono.svg", "w").write(svg("currentColor", "currentColor"))

# app icon / favicon: mark centred on a dark rounded tile. 1024 grid, mark ~62% tall.
S = 1024


def tile(rx, fill=0.62):
    scale = S * fill / H
    tx, ty = (S - W * scale) / 2, (S - H * scale) / 2
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {S} {S}" role="img" aria-label="Postitheka">'
        + GRAD.replace("</defs>", "")
        + '<radialGradient id="pk-bg" cx="0.3" cy="0.2" r="1.1">'
        '<stop offset="0" stop-color="#1E2C40"/><stop offset="1" stop-color="#0A0F17"/></radialGradient>'
        '<filter id="pk-glow" x="-30%" y="-60%" width="160%" height="220%">'
        '<feGaussianBlur stdDeviation="14" result="b"/><feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge></filter>'
        "</defs>"
        f'<rect width="{S}" height="{S}" rx="{rx}" fill="url(#pk-bg)"/>'
        f'<g transform="translate({tx:.1f} {ty:.1f}) scale({scale:.4f})">'
        + svg("url(#pk-body)", "url(#pk-bar)", extra_bar=' filter="url(#pk-glow)"')
        .split(">", 1)[1]
        .rsplit("</svg>", 1)[0]
        + "</g></svg>\n"
    )


open(f"{OUT}/icon-rounded.svg", "w").write(tile(224))  # web/PWA/Android: rounded
open(f"{OUT}/icon-square.svg", "w").write(tile(0))      # iOS: the OS applies its own mask
open(f"{OUT}/favicon.svg", "w").write(tile(200, 0.80))  # 16-32 px: bigger mark, thin bands vanish otherwise
print(f"mark {W:.1f}x{H:.1f}")
