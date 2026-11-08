# Postitheka brand rules for marketing

Sources of truth: `brand/README.md`, `brand/source/postitheka-key-art.png`, `web/src/styles.css`, `ios/Postitheka/Brand.swift` and the colour sets in `ios/Postitheka/Assets.xcassets/`. If this file disagrees with them, they win. Checked on 2026-11-06 at `main` f8b8285.

The owner chose the key art on 2026-11-05 to replace an earlier blue set, and the apps follow it. There is **no written brand guide, voice guide or design-verdict record** beyond `brand/README.md`. The owner's rule is that agents draft pixels and the owner keeps visual verdicts, and that there are no renames or rebrands until 10 strangers have used the current version. **Agents draft, the owner keeps the verdict.**

## Name and words

- **Postitheka** is the only name: one word, capital P only. Never "PostiTheka", "Posti", "Postitheka app" in a title, or a translation.
- **Origin:** post + *-theka*, a collection (`CLAUDE.md`). Fine to explain once; don't build a campaign on it without the owner.
- **Tagline:** "Write once. Publish everywhere." From the key art, the web sign-in (`web/src/App.tsx`) and the iOS sign-in (`ios/Postitheka/SignInView.swift`, set in italics). It over-claims today (`strategy.md`, section 8): use it only beside the logo, with the real network list nearby, until the owner decides.
- **Product vocabulary, keep it exact** (`CLAUDE.md`, "Domain language"):
  - A destination is a **stamp**. A landed copy gets a **postmark**.
  - **Publish** produces **Published**. A failure is **Not sent**. In flight is **Sending**. A WhatsApp hand-off in progress is **On the phone**, and done is **Sent from the phone** (web) or **Sent** (iOS).
  - Connected destinations are **accounts**. A post is a **post**, never a "blast" or "campaign".
- Network names are written as each network writes them: Discord, LinkedIn, Threads, WhatsApp. Say "X", not "Twitter", if X is ever mentioned.

## Personality

No voice document exists. **Inferred from the UI copy, to be confirmed by the owner:** a careful post office. Plain, specific, calm, and honest about failure. It says what happened and what to do next, and it never celebrates for you.

Evidence of the voice in the product:

- "Press Publish again; it will not post twice."
- "The phone never reported back. Check WhatsApp before sending it again."
- "Publish unlocks when uploads finish."
- "You sign in on the platform's own page; Postitheka never sees your password."
- "Posts you publish appear here, with where each one landed."

**Never:**

- Growth-hack hype: "go viral", "10x your reach", "dominate the feed", "explode your audience".
- Engagement or reach promises of any kind. The product doesn't measure either.
- "Automate your social media", "set and forget", "bot". Postitheka posts what you wrote, when you said, through official APIs.
- "AI-powered" anything. There is no AI in the product.
- "Every platform", "all your socials", or a wall of network logos.
- Confetti, fireworks, or a celebration for every Publish. The postmark is the celebration.
- A failure shown in brand colours. Red is for failure because it is outside the palette.

## Colour

From `brand/README.md`, matching `web/src/styles.css` `:root` and the iOS colour sets. **Dark only:** the apps have no light theme, because the art has none. Film dark.

| Name | Hex | Web token | iOS | Role |
|---|---|---|---|---|
| Night | `#0B0E14` | `--paper` | `Night` | The page, the launch screen, the PWA background |
| Sheet | `#151923` | `--sheet` | `Sheet` | Cards, fields, a stamp's window, Form rows |
| Stamp | `#2A2F3C` | `--stamp` | n/a | An unchosen stamp's perforated frame |
| Rule | `#2F2E33` | `--rule` | n/a | Dividers (web only; not in `brand/README.md`) |
| Cream | `#F3E3CB` | `--ink` | `Cream` | Text, the wordmark |
| Muted | `#A89D8E` | `--muted` | system secondary | Secondary text |
| Gold | `#E0AE6E` | `--gold` | `AccentColor` | Actions, a chosen stamp |
| Gold bright | `#F4CC94` | `--gold-bright`, `--focus` | n/a | Hover, focus |
| On gold | `#17120B` | `--on-gold` | `OnGold` | Text on a gold button |
| Glow | `#7FE3F2` | `--postmark` | `Glow` | **One meaning only: on its way or landed.** The postmark, "Sending", "connected" |
| Cancel | `#F0837A` | `--cancel` | system red | "Not sent", errors. Deliberately not in the art |

**Mark gradients** (logo only): stamp and arc `#F4CC94` to `#B27A3C`; front card `#FBF0DE` to `#E2C9A6`; icon tile `#1A1E29` to `#0B0E14`.

**Rules for video.**

- Gold means "act" or "chosen". One gold focus per frame is the rule of thumb: the Publish button or the chosen stamps, not both lit and a gold title.
- Cyan glow means "landed". Save it for the postmark moment, so the viewer learns it. Never use cyan for a title or a background.
- Red only for a real failure on screen. Never as decoration.
- Proportions are **to be confirmed**. The key art reads as mostly night and dark neutrals, with gold and cream in the logo and cyan only in the hologram; don't quote a ratio.

**Off-brand, do not use:** the earlier blue set, which the key art replaced on 2026-11-05 (its files are no longer in `brand/`); any light theme; network brand colours as Postitheka colours.

## Type

- **Web:** Schibsted Grotesk Variable, with its italic, loaded from `@fontsource-variable/schibsted-grotesk` 5.3.0 (`web/src/main.tsx`), falling back to `system-ui`. Base 17 px, a 1.25 scale (`--s-1` to `--s3` in `styles.css`). The font's licence is believed to be OFL; **to be confirmed** from the package's licence file before paid use.
- **iOS:** the system font. No custom font is bundled.
- **The wordmark is not a font.** It is a trace of the key art's lettering (`brand/README.md`). Never retype "Postitheka" in a font as a stand-in for the logo; use the SVG.
- Captions and end cards: Schibsted Grotesk, to match the web. A separate display face is **to be confirmed**; don't introduce one.

## Logo, mark and icons

All in `brand/`. `brand/README.md` says which file goes where.

| Need | File |
|---|---|
| Horizontal lockup | `svg/lockup-horizontal-on-dark.svg`, `svg/lockup-horizontal-on-light.svg` |
| Stacked lockup (end cards, splash) | `svg/lockup-stacked-on-dark.svg`, `svg/lockup-stacked-on-light.svg` |
| Mark alone | `svg/mark-on-dark.svg`, `svg/mark-on-light.svg`, `svg/mark-mono.svg` (`currentColor`) |
| Wordmark alone | `svg/wordmark-on-dark.svg`, `svg/wordmark-on-light.svg`, `svg/wordmark-mono.svg` |
| App icon | `icons/ios-appicon-1024.png`, `svg/icon-rounded.svg`, `svg/icon-square.svg` |
| Web icons | `svg/favicon.svg`, `icons/favicon.ico`, `icons/icon-{192,512}.png`, `icons/icon-maskable-{192,512}.png`, `icons/apple-touch-icon.png` |
| Key art (landscape, 1672×941) | `source/postitheka-key-art.png` |

**Rules** (from `brand/README.md` where stated, otherwise **to be confirmed** by the owner):

- On Night use the `-on-dark` files; on a white page use `-on-light`.
- The favicon is a simplified stamp and one postcard, not the mark shrunk. Don't use the full mark below favicon size.
- Never recolour, stretch, rotate or add a glow to the mark. Clear space and minimum sizes are **to be confirmed**; until then, leave at least the stamp's width clear around the lockup.
- No App Store or Google Play badge anywhere: the app is on neither.
- The key art's holographic cards are **not product UI**. Never present them, or anything like them, as what the app shows.

## Mascot

A small robot postman: navy and cream armour with gold trim, a postman's cap and scarf with a stamp on them, cyan smiling eyes, carrying a stack of letters, hovering over a cyan ring. It has **no name** in the repo; don't give it one without the owner.

- Files: `mascot/mascot.{png,webp}` (718×646, the largest it gets), `mascot/mascot-{256,512}.{png,webp}`, `mascot/mascot-avatar{,-256}.png` (circular).
- **One pose exists**, cut from a render of the key art. Don't scale it beyond 718 px wide; it was not made larger.
- In the product it appears on sign-in (hovering on the web, still on iOS, both over the cyan ring), and in empty states (Sent on both, Accounts on iOS).
- Any new pose, or an animated version, needs the owner's approval. Don't make it a talking presenter, and don't give it lines.

## Motion

From the code. No motion guide exists.

| Kind | Value | Where |
|---|---|---|
| Mascot hover | 5 s ease-in-out loop, rises 0.6 rem | Web sign-in, `.hero img` |
| Stamp chosen | 160 ms transform | Web `.stamp` |
| Hero fade while typing | 0.2 s ease-out | iOS sign-in |
| Character counter | Numeric text transition | iOS composer footer |

Keep edits unhurried: holds and slow pushes, not whip cuts. The natural beat is the stamp: choose, and later the postmark lands. Record with reduced motion **off**, or the mascot stops and stamps stop moving (`@media (prefers-reduced-motion: reduce)`).

## Required credits

| Asset | Licence | What to do |
|---|---|---|
| Schibsted Grotesk | Believed OFL, **to be confirmed** | No credit needed under OFL |
| Key art and mascot | **Unknown.** `brand/README.md` calls the mascot a cutout of a render; how it was made is not recorded | Owner confirms origin and rights before paid use |
| SF Symbols in iOS screens | Apple's terms | Fine inside real iOS screen recordings; don't lift them into graphics |
| Network names and logos | Each network's own rules, below | Check before showing a logo |

## Third-party social network logos

Postitheka's own screens show network **names as text**, never logos (`platformNames` in `web/src/rules.ts`, `displayName` in the iOS kit). Marketing should do the same by default. Any logo needs the checks below.

**Rules for every network:**

1. **Only networks that are built** may appear at all: Discord, LinkedIn, Threads, WhatsApp. Never show X, Instagram, Facebook, TikTok, Reddit, Bluesky or Mastodon logos, not even as "coming soon".
2. Use the network's **official, current asset**, downloaded from its brand page, unaltered: no recolouring to gold, no outlines, no stamps or postmarks drawn over it, no animation of the logo itself.
3. **No implied endorsement or partnership.** Never write "official partner", "approved by" or "integrated with" in a way that suggests the network chose Postitheka.
4. The network's mark must be smaller and less prominent than Postitheka's.
5. Real network UI (a Discord channel, a LinkedIn consent page) may appear only as genuine screen recordings from the test accounts, unedited.
6. Re-read the guidelines **at the time of use**. The summaries below are from general knowledge, not from pages read for this review, and are **to be confirmed** against each page.

| Network | Where its rules live | Points to check |
|---|---|---|
| Discord | Discord's branding page (discord.com/branding) | Use the official logo and its own colours; don't modify; no implied partnership |
| LinkedIn | LinkedIn's brand guidelines (brand.linkedin.com) | The "in" logo unaltered; don't use "LinkedIn" as a verb; no implied endorsement |
| Threads | Meta's Brand Resource Center | Official Threads mark only; Meta's rules on using its logos in video and ads |
| WhatsApp | Meta's Brand Resource Center (WhatsApp section) | Official logo and green only; rules on showing WhatsApp UI in marketing; don't imply WhatsApp integration beyond the share sheet |

## Standard disclaimer

Until Postitheka is deployed and open, every public piece needs a line such as:

> In development. Not yet open to the public.

Use it in the caption or on the end card. When a network is shown being posted to, the post shown must be on a test account, and the caption must not imply that network is available to the viewer. The exact wording is **to be confirmed** by the owner.
