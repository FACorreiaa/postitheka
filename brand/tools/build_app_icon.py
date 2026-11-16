"""App and browser icons from the emblem the owner chose on 2026-11-16.

Usage: uv run --with pillow python3 brand/tools/build_app_icon.py \
           brand/source/postitheka-app-icon-emblem.png brand

The emblem (1254 px) is a finished icon: a rounded navy tile with an orange rim on black. Two
cuts are made from it:

- Full bleed, for platforms that apply their own mask (the iOS app icon, apple-touch-icon,
  maskable PWA icons). The square is cut INSET px in from every side, past the rim, so iOS's
  smaller corner radius hides the emblem's black corners.
- The tile as drawn, with the black outside it made transparent, for places that show the
  picture as it is (the browser tab, PWA `any` icons, the link preview).

It writes over brand/icons/ (ios-appicon-1024, apple-touch-icon, icon-*, favicon-*, favicon.ico)
and does not touch anything else.
"""
import os, sys
from PIL import Image, ImageDraw

SRC, OUT = sys.argv[1:3]
I = f"{OUT}/icons"
os.makedirs(I, exist_ok=True)

# Measured on the 1254 px emblem: the rim starts 16 px in; the corner radius is about 355 px.
INSET = 100
EDGE, RADIUS = 12, 359

src = Image.open(SRC).convert("RGB")
W = src.width
assert src.size == (W, W), "the emblem must be square"
k = W / 1254

bleed = src.crop((round(INSET * k), round(INSET * k), W - round(INSET * k), W - round(INSET * k)))

# Transparent outside the tile, drawn at 4x and scaled down for a smooth edge.
S = 4
mask = Image.new("L", (W * S, W * S), 0)
e, r = round(EDGE * k * S), round(RADIUS * k * S)
ImageDraw.Draw(mask).rounded_rectangle((e, e, W * S - e, W * S - e), radius=r, fill=255)
tile = src.convert("RGBA")
tile.putalpha(mask.resize((W, W), Image.LANCZOS))


def flat(img, path, size):
    img.resize((size, size), Image.LANCZOS).convert("RGB").save(path, optimize=True)


def clear(img, path, size):
    img.resize((size, size), Image.LANCZOS).save(path, optimize=True)


flat(bleed, f"{I}/ios-appicon-1024.png", 1024)
flat(bleed, f"{I}/apple-touch-icon.png", 180)
for s in (192, 512):
    flat(bleed, f"{I}/icon-maskable-{s}.png", s)
    clear(tile, f"{I}/icon-{s}.png", s)
for s in (16, 32, 48):
    clear(tile, f"{I}/favicon-{s}.png", s)
tile.resize((256, 256), Image.LANCZOS).save(f"{I}/favicon.ico", sizes=[(16, 16), (32, 32), (48, 48)])
print("icons written to", I)
