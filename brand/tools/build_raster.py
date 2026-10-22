"""Raster exports: mascot cutouts and the icon sizes each platform asks for."""
import os, subprocess, sys
from PIL import Image, ImageDraw, ImageFilter

CUT, OUT = sys.argv[1:3]
os.makedirs(f"{OUT}/mascot", exist_ok=True)
os.makedirs(f"{OUT}/icons", exist_ok=True)


def svg(src, dst, w=None, h=None):
    cmd = ["rsvg-convert"] + (["-w", str(w)] if w else []) + (["-h", str(h)] if h else []) + [src, "-o", dst]
    subprocess.run(cmd, check=True)


# --- mascot ------------------------------------------------------------------
m = Image.open(CUT).convert("RGBA")
a = m.getchannel("A")
# drop the faint matte the model leaves around glows, keep the soft edge
a = a.point(lambda v: 0 if v < 12 else v)
m.putalpha(a)
m = m.crop(a.getbbox())
pad = 24
full = Image.new("RGBA", (m.width + 2 * pad, m.height + 2 * pad))
full.paste(m, (pad, pad))
full.save(f"{OUT}/mascot/mascot.png", optimize=True)
full.save(f"{OUT}/mascot/mascot.webp", quality=90, method=6)
for h in (512, 256):
    r = full.resize((round(full.width * h / full.height), h), Image.LANCZOS)
    r.save(f"{OUT}/mascot/mascot-{h}.png", optimize=True)
    r.save(f"{OUT}/mascot/mascot-{h}.webp", quality=90, method=6)

# avatar: head and shoulders in a circle on the brand dark, for profile/empty states
S = 1024
grad = Image.radial_gradient("L").resize((S, S))  # 0 centre -> 255 edge
top, bot = (30, 44, 64), (10, 15, 23)
bg = Image.composite(Image.new("RGBA", (S, S), bot + (255,)), Image.new("RGBA", (S, S), top + (255,)), grad)
head = m.crop((0, 0, m.width, round(m.height * 0.72)))  # runs past the circle, which does the clipping
hs = S * 0.86 / head.width
head = head.resize((round(head.width * hs), round(head.height * hs)), Image.LANCZOS)
bg.alpha_composite(head, ((S - head.width) // 2, round(S * 0.14)))
mask = Image.new("L", (S * 4, S * 4))
ImageDraw.Draw(mask).ellipse((0, 0, S * 4 - 1, S * 4 - 1), fill=255)
bg.putalpha(mask.resize((S, S), Image.LANCZOS))
bg.save(f"{OUT}/mascot/mascot-avatar.png", optimize=True)
bg.resize((256, 256), Image.LANCZOS).save(f"{OUT}/mascot/mascot-avatar-256.png", optimize=True)

# --- icons -------------------------------------------------------------------
B = f"{OUT}/svg"
I = f"{OUT}/icons"
for s in (16, 32, 48):
    svg(f"{B}/favicon.svg", f"{I}/favicon-{s}.png", s)
ico = Image.open(f"{I}/favicon-48.png")
ico.save(f"{I}/favicon.ico", sizes=[(16, 16), (32, 32), (48, 48)],
         append_images=[Image.open(f"{I}/favicon-{s}.png") for s in (16, 32)])
for s in (192, 512):
    svg(f"{B}/icon-rounded.svg", f"{I}/icon-{s}.png", s)
    svg(f"{B}/icon-square.svg", f"{I}/icon-maskable-{s}.png", s)  # mark sits inside the 80% safe zone


def flat(src, dst, s):  # iOS rejects alpha in app icons
    svg(src, dst, s)
    Image.open(dst).convert("RGB").save(dst, optimize=True)


flat(f"{B}/icon-square.svg", f"{I}/apple-touch-icon.png", 180)
flat(f"{B}/icon-square.svg", f"{I}/ios-appicon-1024.png", 1024)

# Android adaptive icon: 108dp canvas, foreground must fit the 66dp safe circle
fg = 432
svg(f"{B}/mark-on-dark.svg", f"{I}/_m.png", h=round(fg * 0.50))
mk = Image.open(f"{I}/_m.png")
layer = Image.new("RGBA", (fg, fg))
layer.alpha_composite(mk, ((fg - mk.width) // 2, (fg - mk.height) // 2))
layer.save(f"{I}/android-adaptive-foreground.png", optimize=True)
os.remove(f"{I}/_m.png")
print("ok", full.size)
