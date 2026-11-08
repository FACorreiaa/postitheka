# Postitheka video playbook

How to produce Postitheka videos without showing anything false. Pair this with `../features.md` (what each feature does and its limits), `../strategy.md` (messages) and `brand.md` (look and voice). Checked on 2026-11-06 at `main` f8b8285. **No capture in this document has been run yet**, and no screen was rendered for this review, so rehearse before filming every sequence.

## Rules that never bend

1. **Product functionality is shown with real screens.** Higgsfield is for supporting scenes and presenters. Never use it to generate or fake app UI, network UI, postmarks or published posts.
2. **Never post to a real public account during capture.** Every publish in a recording goes to a dedicated test destination that nobody else reads. Which networks have one:

   | Network | Safe test destination | Status |
   |---|---|---|
   | Discord | A private server created for filming, owner as its only member, with throwaway channels and webhooks | **Usable now.** Free, needs no developer app |
   | WhatsApp | The owner's own "Message yourself" chat, or a group with no other members | Usable on a physical iPhone; never tried |
   | Threads | A dedicated test profile, set to private, added as a Threads Tester on the owner's Meta app | **Blocked:** no Meta app. Whether a test profile is acceptable is an owner decision |
   | LinkedIn | **None.** No sandbox; posts go out `PUBLIC`; fake profiles are understood to break LinkedIn's terms | **Blocked.** Film the stamp only |
   | X, Instagram, Facebook, TikTok, Reddit | n/a | Not built. Never film |

   Never use the Postitheka community server, any real community's channel, or the owner's public profiles.
3. **Outcomes on screen must be real.** A postmark, "Published" or a link must come from a real run against a test destination. Never edit a "Not sent" into a postmark, and never fake a network's page. Seeded data is fine for stamps, counters and composer states, as long as nothing on screen claims it was sent.
4. **No fake customers.** An AI presenter is a presenter. It must not claim to be a user or describe results ("my reach doubled"). No invented testimonials or statistics. Follow each platform's rules on labelling AI-generated people.
5. **Synthetic data only.** No real names, emails, servers, channels or contacts. Use a demo email such as `demo@example.com` when minting a token. Webhook URLs and tokens are credentials: never let one appear in frame.
6. **Spend needs approval.** Get approval before Higgsfield credits. No paid network API is involved today; if X is ever built, every test post costs the owner money and needs approval too.
7. **Nothing is published without explicit authorization.**
8. **Only `verified` features go in as working**, and the whole product is presented as in development. Check the "Do not market" table in `features.md` first.

**No retired assets exist** for this app. The key art is current; its holographic cards are illustration, never product.

## What's missing before a first shoot

| Gap | Why it matters | Smallest fix |
|---|---|---|
| Whether to market before launch | Nothing is deployed, so every video is a preview (`strategy.md`, section 9) | Owner decision |
| A private Discord test server | The only destination that can reach "Published" today | Owner creates it and two webhooks (free) |
| A seed script | Stamps need accounts, and connecting even Discord calls discord.com. There is no seed and no fake-platform mode | A small Go program using `internal/service` and `secretbox` to create a demo user and accounts with dummy sealed credentials. Tier A only: such accounts cannot publish |
| Placeholder LinkedIn and Threads app ids for Tier A | Without them their rules are not served and their stamps show no checks | Set `LINKEDIN_CLIENT_ID`, `LINKEDIN_CLIENT_SECRET`, `THREADS_APP_ID`, `THREADS_APP_SECRET` to dummy values in a separate `.env` for filming. Never click Connect with them |
| A first real post on any network | No publisher has ever met a real network (rollout step 0) | One Discord test post, rehearsed before filming |
| WhatsApp on a device | The hand-off has never been tried on a phone | One rehearsal on the owner's iPhone |
| Sign-in screen | Shows "API token" and a terminal command, which is developer-only | Start every take already signed in, or crop to the hero |
| A call to action | No site, no sign-up, no TestFlight | Owner confirms the community Discord, or none |

## Capture environments

| Need | Use | Notes |
|---|---|---|
| Web, no network (Tier A) | `task db:up` (Postgres `127.0.0.1:5452`, RustFS S3 `127.0.0.1:9020`, console `:9021`), `task dev` (API and workers on `:8095`), `task web:dev` (composer on `:5173`, proxies `/v1`). Or `task build` and run `./server` on `:8095` | Needs `TOKEN_ENCRYPTION_KEY` in `.env` (`openssl rand -base64 32`). Mint a token with `task user:create -- demo@example.com`. The server sets the bucket's CORS for `:5173` and `:8095` on boot |
| Web, real destination (Tier B) | Same stack, plus the Discord test server's webhooks | Discord media uploads stream from local storage, so they work locally. Threads media does not: Meta must download it from a public URL |
| iOS simulator | Debug build, `API_BASE_URL = http://localhost:8095` (`ios/Config/Debug.xcconfig`) | The simulator reaches the Mac's localhost. Shows as "Postitheka" (no separate dev name) |
| iOS on a device (Tier C) | Debug build with `API_BASE_URL` set to the Mac's `.local` name, same network | `NSAllowsLocalNetworking` allows plain http to `.local`. A Release build shows "No server" |

**iOS minimum is 18.0, iPhone only.** The UI tests target an "iPhone 17" simulator, and a comment in them refers to iOS 26's share sheet.

To reset between takes: `task db:reset` (it prompts), then mint a new token and reseed. A scheduled post from an earlier take will otherwise publish, or try to, at its time.

## Recording recipes

**Web**

1. No Playwright or other browser automation exists in this repo. A capture script would be new tooling: ask before adding it, and keep it outside `web/` source.
2. Viewport 1440×900 for landscape; 390 px wide for a phone-shaped web take (the layout was checked at 1280 and 390 px).
3. Sign in once by pasting the token, so the take starts on the workspace. The token stays in `localStorage`.
4. Load the page once first so Schibsted Grotesk is cached.
5. Slow down: type at a human pace, pause on each counter and each postmark.

**iOS simulator**

1. `xcrun simctl status_bar booted override --time 9:41 --batteryState discharged --batteryLevel 100` for a clean status bar.
2. `xcrun simctl io booted recordVideo out.mp4`.
3. Run the existing UI test with `task ios:uitest TOKEN=pk_…` (it runs `PostithekaUITests` on "iPhone 17"). It needs a connected account named "UI test" unless `POSTITHEKA_ACCOUNT` is set. It is too fast for viewers, so add a film-only test class with pauses, or drive the simulator by hand.
4. Launch argument `-ui-testing` suppresses the App Store rating sheet. Use it for every take.
5. A UI test's `app.screenshot()` leaves the keyboard out as a black area; use `XCUIScreen.main.screenshot()` for stills (`CLAUDE.md`, traps).
6. Photos for the picker: drag your own synthetic JPEGs onto the simulator first.

**Output sizes.** Vertical 1080×1920 for short-form, landscape 1920×1080 where useful, square or 4:5 only if a platform calls for it. iPhone captures are naturally vertical; web shots need a framed or cropped treatment for vertical. Posts you make in a demo should use 1080×1350 JPEGs and short H.264 MP4s, which fit every built network.

## Clear these before every recording

- [ ] Signed in already; no token or "API token" field in frame.
- [ ] Demo user only (`demo@example.com` or similar); no real email anywhere.
- [ ] Accounts named for the demo ("#announcements", "#changelog", "Test profile"), not real servers or people.
- [ ] No webhook URL in frame: close the Accounts dialog's form before recording, or blur it.
- [ ] Destinations for any real publish are the test server or self-chat only (rule 2).
- [ ] Dark appearance (it is the only one). Reduced motion off.
- [ ] Rating prompt suppressed (`-ui-testing`) on iOS.
- [ ] Nothing from an earlier take is still scheduled.
- [ ] Avoid on camera: "Connect Discord" in the empty composer (it opens the whole dialog), the LinkedIn and Threads Connect buttons when they carry placeholder ids (clicking them opens a real consent page that will refuse), "No server", "WhatsApp is not installed on this phone".
- [ ] No network logo you have not cleared (`brand.md`).

## Shot bank

Feature numbers refer to `features.md`. **Tier A** is local, with no external network, and can be recorded first. **Tier B** needs a real social network account (a test one). No paid API is involved today. **Tier C** needs a physical device.

| ID | Shot | # | Platform | Length | Tier |
|---|---|---|---|---|---|
| A1 | Sign-in hero: mascot hovering over the cyan ring, lockup, tagline. Crop out the token form | 14 | Web | 6 s | A |
| A2 | iOS sign-in hero, same crop | 15 | iOS sim | 5 s | A |
| A3 | Empty Sent with the mascot: "Posts you publish appear here, with where each one landed." | 6 | Web | 4 s | A |
| A4 | Three stamps chosen, counters ticking "/ 2,000", "/ 3,000", "/ 500" as text is typed | 2 | Web | 10 s | A (seed + placeholder ids) |
| A5 | Paste a long paragraph: Threads turns red, "N characters over", Publish disabled; deselect Threads, Publish lights | 2 | Web | 10 s | A (seed + placeholder ids) |
| A6 | Type one emoji: Threads jumps by 4, the others by 1 | 2, 10 | Web | 6 s | A (seed + placeholder ids) |
| A7 | Add a video and a photo: LinkedIn says "A video must be posted alone"; a GIF: Threads says "Cannot take image/gif" | 2, 3 | Web | 10 s | A (seed + placeholder ids) |
| A8 | Drag in photos, upload bars fill against local storage, thumbnails settle | 3 | Web | 8 s | A |
| A9 | Set "Publish at", button becomes Schedule, Sent shows "scheduled for …" with "Waiting" pills | 4 | Web | 12 s | A (seed; reset afterwards) |
| A10 | iOS Compose: Stamps section, problems under a chosen stamp, Schedule toggle and "Send at" | 1, 2, 4 | iOS sim | 12 s | A (seed + placeholder ids) |
| A11 | Accounts dialog: "Connect a Discord channel" steps and "You sign in on the platform's own page; Postitheka never sees your password." Don't click Connect | 8, 12 | Web | 8 s | A |
| B1 | Paste a test-server webhook, Connect, the stamp appears (webhook blurred) | 8 | Web | 12 s | B |
| B2 | One post to two test channels: two postmarks arrive; click one; the message in Discord, no pings | 1, 6, 8 | Web and Discord | 20 s | B |
| B3 | Schedule two minutes ahead, cut, the postmark lands | 4, 6 | Web | 15 s | B |
| B4 | One webhook deleted: one postmark, one "Not sent" with the reason | 7 | Web | 12 s | B (owner decides whether to show a failure) |
| B5 | iOS sim: Photos picker, upload, publish to the test server, "Published HH:MM", Open | 3, 6, 15 | iOS sim and Discord | 20 s | B |
| B6 | Threads test profile: connect, publish text, postmark | 10, 12 | Web | 15 s | B (blocked: no Meta app; owner decision) |
| B7 | LinkedIn real publish | 9 | n/a | n/a | **Not filmable** under rule 2 |
| C1 | WhatsApp: stamp, Publish, share sheet, send to self-chat, back to Sent "Sent HH:MM" | 11 | iPhone | 20 s | C (never tried; rehearse) |
| C2 | Discord and WhatsApp together from the phone: postmark and hand-off side by side | 6, 8, 11 | iPhone and Discord | 20 s | B + C |

**Cheapest first cut:** a silent 30-second reel from A1, A4, A5, A6 and A9. No account, no network, no spend. It needs the seed script and placeholder ids. The first cut that shows a real landing adds B2, which needs only the free Discord test server.

## Formats

Every format needs a script, storyboard and shot list approved before generation. These are starting points, not approved scripts. Every end card carries the in-development line from `brand.md` until the owner says otherwise.

### 1. Full application showcase (45–60 s)

| Time | Beat | Picture | Words |
|---|---|---|---|
| 0–4 | Hook | Night background, lockup | "One update. Four apps. Four sets of rules." |
| 4–12 | Write once | A4 | "Postitheka takes the post once, and you choose where it goes." |
| 12–24 | It fits before it goes | A5, then A6 | "Each stamp checks it the way its network counts. If one would refuse it, you know before you press Publish." |
| 24–36 | It lands | B2 | "Then each one goes on its own, through the network's official API, and you get a postmark for every place it landed." |
| 36–44 | Later, too | A9, then B3 | "Write it now. Send it later." |
| 44–52 | Never twice | B2 close-up of the postmarks | "And it's built never to post twice." |
| 52–60 | End | Stacked lockup, tagline, CTA if approved | "Postitheka. Write once." In-development line on screen. |

Add C1 ("On iPhone, it hands the post to WhatsApp for you to send") only after the hand-off has been proven on a device.

### 2. AI presenter / UGC advertisement (30–45 s)

The presenter speaks in the second person about the viewer's situation, and real screen recordings show the product. Draft:

> You finished the update. Now you paste it into Discord, then LinkedIn, then Threads, and one of them says it's too long.
>
> Postitheka is a cross-poster being built for exactly that. You write the post once, and every destination checks it as you type.
>
> When it fits everywhere, you press Publish once. Each one goes out through the network's own API, and you get a postmark for every place it landed.
>
> And if your connection drops, pressing Publish again won't post it twice.
>
> It's in development. Follow along on our Discord.

Picture: a presenter at a desk, cutting to A5, B2 and A9. Generated scenes must contain no fake phone or laptop UI: composite the real recordings instead. Drop the last line if the Discord call to action is not approved. Stop if the presenter gains a personal backstory or results. Never name X, Instagram, TikTok or Facebook.

The owner on camera, building it in public, would be stronger and more honest than any generated presenter, if willing (`strategy.md`, section 9).

### 3. Individual feature demos (15–30 s)

Use the "Video concept" on each feature in `features.md`. Suggested order: Checked at the door (2), Sent with postmarks (6), Discord channels (8), Schedule (4), Threads emoji counting (10), then WhatsApp (11) once proven.

Each demo covers one problem and one benefit, and ends on the call to action, or on the lockup and the in-development line if there is none.

### 4. Short teaser (10–15 s)

Option A, no network: A6. One emoji typed; the Threads counter jumps by 4, the others by 1. Text: "Every network counts differently. Postitheka knows how." End card.

Option B, Discord test server: B2 close-up. Publish, two postmarks stamp in with their cyan cancellation lines. Text: "A postmark for every landing." End card.

## Using Higgsfield

Fits: presenter footage, a desk or post-office mood shot, transitions, thumbnails, and motion built from the key art's world. Doesn't fit: anything that shows the Postitheka interface, a network's interface, or a post being received. Keep to the palette in `brand.md` and name it in the prompt:

> Dark night palette, background `#0B0E14`, surfaces `#151923`, warm gold `#E0AE6E` accents, cream `#F3E3CB` highlights, a single thin cyan glow `#7FE3F2` used only for the moment something arrives. Calm, cinematic, a quiet post office after dark. No neon, no purple or blue gradients, no confetti, no social network logos, no readable screens.

Prompt out network logos, readable text and device screens. Never generate the mascot in a new pose or as a speaking character without the owner's approval. The Higgsfield CLI's login state for this app was not checked; logging in is the owner's to do. Use the `higgsfield-generate` skill if it is available, and quote the credit cost before spending.

## Asset manifest

Keep it at `assets/manifest.md`, with final assets in `assets/`. Create it with the first real asset. Large recordings stay out of Git. One row per file:

| File | Video or shot ID | Feature # | Platform | App commit | Data source (seed, test server) | Test destination used | Posted to a public account? (must be No) | Licences and credits | Approved by, date |
|---|---|---|---|---|---|---|---|---|---|

Re-check a clip whenever the feature changes, and whenever a network's limits in `Rules()` change.

## Quality control

Run this before asking for approval:

- [ ] Every demonstrated feature is `verified` in `features.md`, or the stated limit is respected.
- [ ] Every postmark, "Published" and link came from a real run to a test destination, and the manifest says which.
- [ ] Nothing was posted to a real public account.
- [ ] Only built networks are named or shown: Discord, LinkedIn, Threads, WhatsApp. No X, Instagram, Facebook, TikTok or Reddit.
- [ ] Network logos, if any, are official, unaltered and cleared (`brand.md`).
- [ ] No token, webhook URL, real email, real server or real contact in frame.
- [ ] UI text is current, with the product's own words: stamp, postmark, Publish, Published, Not sent.
- [ ] Words match the limits: "built never to post twice", "you tap Send" for WhatsApp, no "everywhere" as a literal claim.
- [ ] The product is presented as in development; no "download", "sign up" or "available now"; no store badge.
- [ ] Audio, captions and visuals are in sync. Captions are in the video's language.
- [ ] The in-development line and any credits are present.
- [ ] Social package ready: hook, caption, CTA, hashtags, vertical and landscape cuts, thumbnail idea, calendar row.
- [ ] Owner approved it. **Published only with explicit authorization.**
