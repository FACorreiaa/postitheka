# Brand assets

Cut from the key art in `source/postitheka-key-art.png`. Draft: the owner keeps the visual verdict.

## What to use where

| Need | File |
| --- | --- |
| Web header, docs, README | `svg/lockup-horizontal-on-{light,dark}.svg` |
| Splash, About, marketing | `svg/lockup-stacked-on-{light,dark}.svg`, `mascot/mascot.png` |
| Mark alone (nav, loaders) | `svg/mark-on-{light,dark}.svg`; `svg/mark-mono.svg` takes `currentColor` |
| Wordmark alone | `svg/wordmark-on-{light,dark}.svg`, `svg/wordmark-mono.svg` |
| Browser tab | `svg/favicon.svg` with `icons/favicon.ico` as fallback |
| PWA manifest | `icons/icon-{192,512}.png` (`any`), `icons/icon-maskable-{192,512}.png` (`maskable`) |
| iOS home screen (web) | `icons/apple-touch-icon.png` (180, no alpha) |
| iOS app icon | `icons/ios-appicon-1024.png` (no alpha, square: iOS applies the mask) |
| Android adaptive icon | `icons/android-adaptive-foreground.png` (432 px) on background `#0E1520` |
| Mascot in UI (empty states, onboarding) | `mascot/mascot-{256,512}.{png,webp}`, transparent |
| Mascot as avatar | `mascot/mascot-avatar{,-256}.png`, circular |

`favicon.svg` draws the mark larger than `icon-*.svg`: at 16 px the bands vanish at the app-icon
size. The mark is rebuilt as geometry (one slope, one band thickness, one gap) rather than traced,
so it stays crisp at every size. The wordmark is a potrace trace of the key art's lettering.

The mascot is a cutout of a 1254 px render, so `mascot.png` (530x759) is the largest it gets
without upscaling. It holds a tablet showing the mark; the platform logos around it in the key
art are left out, since they are other companies' trademarks.

## Colours

| Token | Value |
| --- | --- |
| Mark light | `#F5F8FC` to `#BFCADB` |
| Mark dark (on light) | `#26364A` to `#121B27` |
| Accent bar | `#4FC3FF` to `#2E86F0` |
| Icon tile | `#1E2C40` to `#0A0F17` |

## Rebuilding

Needs `rsvg-convert` and Python with Pillow and numpy; the mascot cutout also used `rembg`
(BiRefNet-general model) and is kept as `source/mascot-cutout-raw.png`, so it need not rerun.

```sh
python tools/build_mark.py svg
rsvg-convert -w 3000 -h 510 tools/wordmark-trace.svg -o /tmp/word.png
python tools/build_word.py tools/wordmark-trace.svg /tmp/word.png svg
python tools/build_raster.py source/mascot-cutout-raw.png .
```
