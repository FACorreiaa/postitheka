# Brand assets

Cut from the key art in `source/postitheka-key-art.png`, which the owner chose on 2026-11-05 to
replace the earlier blue set: stamp logo, postman mascot, gold on night. The apps follow it, and
the owner keeps the visual verdict.

## What to use where

| Need | File |
| --- | --- |
| Web header, docs, README | `svg/lockup-horizontal-on-dark.svg`; `-on-light` for a white page such as a README |
| Splash, About, marketing | `svg/lockup-stacked-on-{light,dark}.svg`, `mascot/mascot.png` |
| Mark alone (nav, loaders) | `svg/mark-on-{light,dark}.svg`; `svg/mark-mono.svg` takes `currentColor` |
| Wordmark alone | `svg/wordmark-on-{light,dark}.svg`, `svg/wordmark-mono.svg` |
| Browser tab | `svg/favicon.svg` with `icons/favicon.ico` as fallback |
| PWA manifest | `icons/icon-{192,512}.png` (`any`), `icons/icon-maskable-{192,512}.png` (`maskable`) |
| iOS home screen (web) | `icons/apple-touch-icon.png` (180, no alpha) |
| iOS app icon | `icons/ios-appicon-1024.png` (no alpha, square: iOS applies the mask) |
| iOS sign-in lockup | `Lockup.imageset`: `svg/lockup-stacked-on-dark.svg` rendered to PNG at 3x, since the catalog's SVG support is not trusted with this mark's masks |
| Android adaptive icon | `icons/android-adaptive-foreground.png` (432 px) on background `#0B0E14` |
| Mascot in UI (empty states, onboarding) | `mascot/mascot-{256,512}.{png,webp}`, transparent |
| Mascot as avatar | `mascot/mascot-avatar{,-256}.png`, circular |

`favicon.svg` is not the mark shrunk: at 16 px the arc and arrows are noise, so it is the stamp and
one postcard. The mark is rebuilt as geometry measured from the art rather than traced, so it stays
crisp at every size. The wordmark is a potrace trace of the key art's lettering.

The mascot is a cutout of a 1672 px render, so `mascot.png` (718x646) is the largest it gets without
upscaling. The holographic cards it hands out in the art are left out.

## Colours

The apps are dark only, because the art is. `web/src/styles.css` and the iOS asset catalog hold the
same values.

| Token | Value | For |
| --- | --- | --- |
| Night | `#0B0E14` | The page |
| Sheet | `#151923` | Cards, fields, a stamp's window |
| Stamp | `#2A2F3C` | An unchosen stamp's perforated frame |
| Cream | `#F3E3CB` | Text, the wordmark |
| Muted | `#A89D8E` | Secondary text on the web; iOS uses the system's |
| Gold | `#E0AE6E` | Actions, a chosen stamp; the iOS accent colour |
| Gold bright | `#F4CC94` | Hover, focus |
| On gold | `#17120B` | Text on a gold button |
| Glow | `#7FE3F2` | One meaning: on its way or landed. The postmark, "Sending" |
| Cancel | `#F0837A` | "Not sent", errors. Not in the art: a failure must not read as brand |

## Mark gradients

| Part | Value |
| --- | --- |
| Stamp, arc | `#F4CC94` to `#B27A3C` |
| Front card | `#FBF0DE` to `#E2C9A6` |
| Icon tile | `#1A1E29` to `#0B0E14` |

## Rebuilding

Needs `rsvg-convert` and Python with Pillow and numpy; the mascot cutout also used `rembg`
(BiRefNet-general model) and is kept as `source/mascot-cutout-raw.png`, so it need not rerun.

```sh
# From brand/. Only if the art changes: the lettering is thresholded from a 6x upscale
# of (100,497)-(722,604) in the key art, and the result is committed.
python tools/build_mark.py svg
rsvg-convert -w 3732 -h 642 tools/wordmark-trace.svg -o /tmp/word.png
python tools/build_word.py tools/wordmark-trace.svg /tmp/word.png svg
python tools/build_raster.py source/mascot-cutout-raw.png .
```

## 3D mascot (beta only, BRD-001)

An experiment the owner judges on a device before anything reaches production. It is the one
exception to "the mascot is the postman": the owner supplied a 3D kit of a different robot
(white and blue, holding a tablet with a P-arrow mark) and decided on 2026-11-13 to try it
**recoloured to this palette, on beta only**. It is not the approved mascot, and it does not
replace any file above.

- **Never in production.** The web shows it only when `MASCOT_3D_MODEL_URL` is set, and only a
  beta or local environment sets it. The iOS app compiles it into Debug and Beta builds only;
  `Release.xcconfig` excludes the view, the model and the motion file, and
  `ios/tools/release-proof.sh` fails if a Release build contains any of them.
- **The tablet is removed.** Its mark is not the stamp, and a recolour cannot change a shape.
- **Colours are tokens.** The build script paints each material with a token read from
  `web/src/styles.css`, and the scene's lights are tokens too (gold key light, glow rim). No
  hex literal in the 3D code; `scripts/brand-guard.sh` checks.
- **Motion is data:** `design/mascot-motion.json`, applied by one pure driver per client
  (`web/src/lib/mascot-motion.ts`, `PostithekaKit` `MascotMotion`). It floats on the flat
  mascot's period, sways slightly, rises in once, and on the web turns a little toward the
  pointer. No eyes move: the model has no rig.
- **The flat mascot is the first paint and the fallback.** It stays under reduced motion,
  Save-Data, Low Power Mode, without WebGL, or when the model does not load.

Rebuild, from the repo root, after any change to the paint or the source:

```sh
python3 -I brand/tools/build_mascot3d.py \
    --src ~/Work/production/apps/postitheka-3d-src/Postitheka_Mascot_Web_NoTablet.glb \
    --out out/mascot3d --ios ios/Postitheka/Mascot3D
```

Locally, set `MASCOT_3D_MODEL_URL=/mascot3d/mascot.glb` in `.env` and run `task dev` with
`task web:dev`; Vite serves `out/mascot3d/`. A Debug or Beta iOS build picks up the copied
`Mascot.usdz`.

The source kit stays outside the repo and is treated as untrusted: only its
`Postitheka_Mascot_Web_NoTablet.glb` is copied, into an empty directory of its own, and none of
its scripts or its USDZ are used. The build output (`out/mascot3d/`, and the copy in
`ios/Postitheka/Mascot3D/`) is git-ignored.
