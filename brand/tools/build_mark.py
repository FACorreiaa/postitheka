"""Rebuild the Postitheka stamp mark as clean geometry, measured from the key art.

Measured on a 3x crop of source/postitheka-key-art.png, so the numbers below are
in that crop's pixels: a perforated gold stamp, a fan of cream postcards leaving
it, five arrows out to five nodes on one circle.
"""
import math
import os
import sys

OUT = sys.argv[1]
os.makedirs(OUT, exist_ok=True)

CX, CY, R = 541, 565, 489  # the circle the nodes sit on, fitted to all four
NODES = [-91.9, -41.3, 2.0, 42.0]  # degrees; 0 is east, negative runs up
ARC_FROM, ARC_TO = -137.0, 82.0
VB = (40, 40, 1040, 1040)


def polar(deg, r=R):
    t = math.radians(deg)
    return CX + r * math.cos(t), CY + r * math.sin(t)


def rounded(points, r):
    """A closed path through points with each corner cut to radius r."""
    d, n = [], len(points)
    for i, (x, y) in enumerate(points):
        px, py = points[i - 1]
        nx, ny = points[(i + 1) % n]
        la = math.hypot(px - x, py - y)
        lb = math.hypot(nx - x, ny - y)
        k = min(r, la / 2, lb / 2)
        a = (x + (px - x) * k / la, y + (py - y) * k / la)
        b = (x + (nx - x) * k / lb, y + (ny - y) * k / lb)
        d.append(f"{'M' if i == 0 else 'L'}{a[0]:.1f} {a[1]:.1f}Q{x:.1f} {y:.1f} {b[0]:.1f} {b[1]:.1f}")
    return "".join(d) + "Z"


def perforated(x0, y0, x1, y1, bite=23, step_x=88.0, step_y=92.0):
    """A stamp's edge: straight runs with half-circle bites taken out of them."""
    d = [f"M{x0} {y0}"]

    def edge(x_from, y_from, x_to, y_to, step):
        length = math.hypot(x_to - x_from, y_to - y_from)
        count = max(1, round(length / step))
        ux, uy = (x_to - x_from) / length, (y_to - y_from) / length
        pitch = length / count
        for i in range(count):
            mid = (i + 0.5) * pitch
            ax, ay = x_from + ux * (mid - bite), y_from + uy * (mid - bite)
            bx, by = x_from + ux * (mid + bite), y_from + uy * (mid + bite)
            d.append(f"L{ax:.1f} {ay:.1f}A{bite} {bite} 0 0 0 {bx:.1f} {by:.1f}")
        d.append(f"L{x_to} {y_to}")

    edge(x0, y0, x1, y0, step_x)
    edge(x1, y0, x1, y1, step_y)
    edge(x1, y1, x0, y1, step_x)
    edge(x0, y1, x0, y0, step_y)
    return "".join(d) + "Z"


def arrow(x0, y0, x1, y1, head=78, width=68, shaft=17):
    """An arrow whose shaft tapers from nothing at the stack to full at the head."""
    length = math.hypot(x1 - x0, y1 - y0)
    ux, uy = (x1 - x0) / length, (y1 - y0) / length
    nx, ny = -uy, ux
    bx, by = x1 - ux * head, y1 - uy * head
    notch = 14
    cx, cy = bx + ux * notch, by + uy * notch
    return (
        f"M{x0 + nx * shaft * 0.2:.1f} {y0 + ny * shaft * 0.2:.1f}"
        f"L{cx + nx * shaft / 2:.1f} {cy + ny * shaft / 2:.1f}"
        f"L{bx + nx * width / 2:.1f} {by + ny * width / 2:.1f}"
        f"L{x1:.1f} {y1:.1f}"
        f"L{bx - nx * width / 2:.1f} {by - ny * width / 2:.1f}"
        f"L{cx - nx * shaft / 2:.1f} {cy - ny * shaft / 2:.1f}"
        f"L{x0 - nx * shaft * 0.2:.1f} {y0 - ny * shaft * 0.2:.1f}Z"
    )


STAMP_OUTER = perforated(55, 308, 582, 962)
STAMP_WINDOW = "M150 368H500Q530 368 530 398V878Q530 908 500 908H150Q120 908 120 878V398Q120 368 150 368Z"
# The stack passes through the stamp's right edge, which is why that edge stops
# short: in the key art the cards break out of the frame, and that is the idea.
STAMP_BREAK = "M520 462H600V858H520Z"

CARD_BACK = rounded([(275, 412), (608, 492), (600, 512), (262, 470)], 8)
CARD_TAN = rounded([(205, 465), (636, 520), (606, 800), (154, 782)], 20)
CARD_MID = rounded([(258, 518), (650, 532), (640, 820), (205, 832)], 20)
CARD_FRONT = rounded([(325, 585), (738, 532), (728, 850), (265, 882)], 24)
CARD_STAMP = rounded([(622, 580), (706, 569), (701, 656), (617, 670)], 5)
LINES = [((362, 690), (568, 656)), ((352, 738), (565, 701)), ((342, 790), (488, 762))]
WAVES = [
    "M540 790C575 742 610 790 650 762S700 738 736 732",
    "M532 822C572 772 610 822 650 794S700 770 736 764",
    "M524 856C566 804 610 854 650 826S700 802 736 796",
]
ARROWS = [
    arrow(575, 312, 575, 130),
    arrow(655, 500, 875, 278),
    arrow(755, 588, 990, 566),
    arrow(755, 750, 892, 870),
    arrow(605, 890, 605, 1068),
]


def art(p):
    """The mark's drawing for one palette; ids are prefixed so inlined copies never collide."""
    k = p["id"]
    defs = (
        "<defs>"
        f'<linearGradient id="{k}g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="{p["gold_hi"]}"/><stop offset="1" stop-color="{p["gold_lo"]}"/></linearGradient>'
        f'<linearGradient id="{k}c" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="{p["card_hi"]}"/><stop offset="1" stop-color="{p["card_lo"]}"/></linearGradient>'
        f'<linearGradient id="{k}s" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="{p["stamp_hi"]}"/><stop offset="1" stop-color="{p["stamp_lo"]}"/></linearGradient>'
        f'<clipPath id="{k}f"><path d="{CARD_FRONT}"/></clipPath>'
        f'<mask id="{k}b" maskUnits="userSpaceOnUse" x="0" y="0" width="1110" height="1110"><rect width="1110" height="1110" fill="#fff"/><path d="{STAMP_BREAK}" fill="#000"/></mask>'
        "</defs>"
    )
    gap = 6.0
    bounds = [ARC_FROM] + [v for n in NODES for v in (n - gap, n + gap)] + [ARC_TO]
    arcs = "".join(
        f'M{polar(a)[0]:.1f} {polar(a)[1]:.1f}A{R} {R} 0 0 1 {polar(b)[0]:.1f} {polar(b)[1]:.1f}'
        for a, b in zip(bounds[::2], bounds[1::2])
    )
    nodes = "".join(
        f'<circle cx="{polar(n)[0]:.1f}" cy="{polar(n)[1]:.1f}" r="30" fill="url(#{k}g)"/>'
        f'<circle cx="{polar(n)[0]:.1f}" cy="{polar(n)[1]:.1f}" r="22" fill="{p["node_gap"]}"/>'
        f'<circle cx="{polar(n)[0]:.1f}" cy="{polar(n)[1]:.1f}" r="16" fill="{p["card_hi"]}"/>'
        for n in NODES
    )
    lines = "".join(
        f'<path d="M{a[0]} {a[1]}L{b[0]} {b[1]}" stroke="{p["ink"]}" stroke-width="12" stroke-linecap="round"/>'
        for a, b in LINES
    )
    waves = "".join(f'<path d="{w}" fill="none" stroke="{p["ink"]}" stroke-width="11"/>' for w in WAVES)
    return (
        defs
        + f'<path d="{arcs}" fill="none" stroke="url(#{k}g)" stroke-width="7" stroke-linecap="round"/>'
        + nodes
        + f'<path d="{"".join(ARROWS)}" fill="{p["arrow"]}"/>'
        + f'<path d="{STAMP_OUTER}{STAMP_WINDOW}" fill="url(#{k}g)" stroke="url(#{k}g)" stroke-width="10" stroke-linejoin="round" fill-rule="evenodd" mask="url(#{k}b)"/>'
        + f'<path d="{CARD_BACK}" fill="{p["card_back"]}"/>'
        + f'<path d="{CARD_TAN}" fill="{p["card_tan"]}"/>'
        + f'<path d="{CARD_MID}" fill="{p["card_mid"]}"/>'
        + f'<path d="{CARD_FRONT}" fill="url(#{k}c)"/>'
        + f'<path d="{CARD_STAMP}" fill="url(#{k}s)"/>'
        + lines
        + f'<g clip-path="url(#{k}f)">{waves}</g>'
    )


def mono():
    gap = 6.0
    bounds = [ARC_FROM] + [v for n in NODES for v in (n - gap, n + gap)] + [ARC_TO]
    arcs = "".join(
        f'M{polar(a)[0]:.1f} {polar(a)[1]:.1f}A{R} {R} 0 0 1 {polar(b)[0]:.1f} {polar(b)[1]:.1f}'
        for a, b in zip(bounds[::2], bounds[1::2])
    )
    nodes = "".join(f'<circle cx="{polar(n)[0]:.1f}" cy="{polar(n)[1]:.1f}" r="26"/>' for n in NODES)
    return (
        f'<path d="{arcs}" fill="none" stroke="currentColor" stroke-width="9" stroke-linecap="round"/>'
        + f'<g fill="currentColor">{nodes}<path d="{"".join(ARROWS)}"/>'
        + f'<path d="{STAMP_OUTER}{STAMP_WINDOW}" fill-rule="evenodd"/><path d="{CARD_FRONT}"/></g>'
    )


def favicon_art(p):
    """At 16 px the arcs and arrows are noise. The stamp and one card carry it."""
    k = p["id"] + "i"
    stamp = perforated(120, 60, 880, 940, bite=36, step_x=127, step_y=125)
    window = "M210 150H790V850H210Z"
    card = rounded([(170, 400), (900, 310), (880, 780), (100, 830)], 40)
    stampette = rounded([(690, 400), (820, 386), (812, 510), (682, 524)], 8)
    return (
        f'<defs><linearGradient id="{k}g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="{p["gold_hi"]}"/><stop offset="1" stop-color="{p["gold_lo"]}"/></linearGradient></defs>'
        f'<path d="{stamp}{window}" fill="url(#{k}g)" stroke="url(#{k}g)" stroke-width="16" stroke-linejoin="round" fill-rule="evenodd"/>'
        f'<path d="{card}" fill="{p["card_hi"]}"/><path d="{stampette}" fill="{p["stamp_lo"]}"/>'
        f'<path d="M240 560L560 520M230 660L470 632" stroke="{p["ink"]}" stroke-width="40" stroke-linecap="round"/>'
    )


DARK = dict(
    id="d", gold_hi="#F4CC94", gold_lo="#B27A3C", card_hi="#FBF0DE", card_lo="#E2C9A6",
    card_back="#6E5D4E", card_tan="#C9A987", card_mid="#E3CBA9",
    stamp_hi="#8A6A42", stamp_lo="#6A4E2E", ink="#2A2118", node_gap="#0B0E14", arrow="#FBF0DE",
)
# On a light page cream cards vanish, so the whole mark steps down a tone.
LIGHT = dict(
    id="l", gold_hi="#C08A4A", gold_lo="#7E5526", card_hi="#E9D2AE", card_lo="#C9A67A",
    card_back="#5B4A3B", card_tan="#A98560", card_mid="#C7A67F",
    stamp_hi="#6F5230", stamp_lo="#523B20", ink="#241B12", node_gap="#FFFFFF", arrow="#7E5526",
)

vb = " ".join(map(str, VB))
for name, p in (("dark", DARK), ("light", LIGHT)):
    with open(f"{OUT}/mark-on-{name}.svg", "w") as f:
        f.write(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb}" role="img" aria-label="Postitheka">{art(p)}</svg>\n')
with open(f"{OUT}/mark-mono.svg", "w") as f:
    f.write(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb}" role="img" aria-label="Postitheka">{mono()}</svg>\n')


def tile(rx, inset):
    """The dark tile the icons stand on; inset keeps the mark inside the mask's safe zone."""
    size = 1024
    scale = (size - 2 * inset) / VB[2]
    tx = inset - VB[0] * scale
    ty = inset - VB[1] * scale
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {size} {size}">'
        '<defs><linearGradient id="t" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#1A1E29"/><stop offset="1" stop-color="#0B0E14"/></linearGradient></defs>'
        f'<rect width="{size}" height="{size}" rx="{rx}" fill="url(#t)"/>'
        f'<g transform="translate({tx:.2f} {ty:.2f}) scale({scale:.4f})">{art(DARK)}</g></svg>\n'
    )


with open(f"{OUT}/icon-rounded.svg", "w") as f:
    f.write(tile(224, 120))
with open(f"{OUT}/icon-square.svg", "w") as f:
    f.write(tile(0, 150))  # inside the 80% maskable safe zone and iOS's corner mask
with open(f"{OUT}/favicon.svg", "w") as f:
    f.write(
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1000 1000">'
        '<rect width="1000" height="1000" rx="180" fill="#0B0E14"/>'
        f"{favicon_art(DARK)}</svg>\n"
    )
print("mark ok")
