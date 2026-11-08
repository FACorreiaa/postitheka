# Postitheka: product marketing strategy

Last verified: 2026-11-06, against `postitheka` `main` f8b8285 (2026-11-05; the local `initial-build` branch is the same commit), and `platform/infra` `origin/main` e838769 and local HEAD 66e6429. **Production runs nothing.** Postitheka has no entry in `platform/infra` on any branch, so there is no production SHA to check against.

Related documents:

- `features.md`: the feature inventory, with what can and cannot be shown.
- `rules/brand.md`: colours, type, logo rules, third-party logos and voice.
- `rules/video-playbook.md`: how to capture and produce videos.
- `rules/content-calendar.md`: what is planned and approved.

There is no older marketing plan for this app. Positioning here is built from the code, `README.md`, `CLAUDE.md`, `ARCHITECTURE.md`, `BACKLOG.md`, `docs/platform-rollout.md`, `docs/platform-research.md` and `brand/`.

## Rules for anyone writing from this document

1. The product is **Postitheka**: one word, capital P, the name from "post" + *-theka*, a collection. There is no other public or internal name.
2. Every claim must trace to `features.md`. If a feature has a status other than `verified`, follow the limits written on that feature.
3. **Nothing is live.** Postitheka is not deployed, has no public address, no sign-up, and is not on TestFlight or the App Store. Never write "available now", "try it today", "download" or "sign up". Until that changes, any material is a preview of software in development, and must say so.
4. **Name only the networks that are built.** Built: Discord (channels, by webhook), LinkedIn (personal profile), Threads, and WhatsApp as a hand-off from the iPhone app. Not built: X, Instagram, Facebook, TikTok, Reddit. No publisher has posted to a real platform yet: every one has run only against a stub in tests.
5. Never invent users, testimonials, statistics or results. The repository records **0 strangers** and no usage data.
6. Postitheka has no AI in it. Never say it writes, rewrites or optimises posts. Rewriting per platform with an LLM is listed as deferred, not planned.
7. Items marked **Needs confirmation** are open decisions. Do not resolve them by guessing.

---

## 1. Product overview

| | |
|---|---|
| **Product name** | Postitheka |
| **Website** | **None.** The public hostname was deferred by the owner on 2026-10-22 ("not now") and is decision 6 in `docs/platform-rollout.md` |
| **Tagline** | "Write once. Publish everywhere." It is on the key art and on both sign-in screens. See section 8: "everywhere" over-claims today |
| **One sentence** | Postitheka lets you write a post once, with text, photos or video, pick where it goes, and see per destination whether it landed. |
| **Category** | Social media cross-posting and scheduling tool. No App Store category is set: the app has never been submitted |
| **Operator** | Fernando Correia (GitHub `FACorreiaa`, Apple developer team `84X9WYBF36`, bundle `com.fernandocorreia.postitheka`). Whether this is an individual or a company is not stated anywhere in the repo: no privacy policy or terms exist yet (PRV-001 open) |
| **Price today** | None, because nobody can use it. No billing code exists. Free and premium tiers are decided in principle; see section 6 |

**Elevator pitch.** Posting the same update to several networks means rewriting it for each one's limits, uploading the same files again, and checking afterwards which ones actually went out. Postitheka takes the post once, checks it against every chosen network before anything is stored, publishes to each one through that network's official API, and shows where each copy landed, with a link. It is built never to post twice.

### Supported platforms

**Client platforms**

| Surface | State on 2026-11-06 | Notes |
|---|---|---|
| **Web composer** | **Local only** | Built and embedded in the Go binary. Runs at `localhost:5173` (dev) or `:8095` (built). Dark only. Has a web app manifest. Not deployed anywhere |
| **iPhone app** | **Development builds only** | SwiftUI, iOS 18 or later, iPhone only (`TARGETED_DEVICE_FAMILY = 1`), version 0.1 build 1. Not on TestFlight or the App Store. The Release configuration has an empty server address, so a Release build opens on "No server" |
| **iPad, Mac** | None | iPhone only |
| **Android** | None | An adaptive icon exists in `brand/icons/`, nothing else |

**Target social networks**

| Network | Built | Usable today | What it takes | Evidence |
|---|---|---|---|---|
| **Discord** | Yes: server channels through an incoming webhook | From a local server, with any Discord channel webhook | Nothing beyond a webhook URL. Never posted to a real channel | `internal/publisher/discord/` |
| **LinkedIn** | Yes: the member's own **personal profile**, posted with `visibility: PUBLIC` | No. Offered only when `LINKEDIN_CLIENT_ID` is set, and no developer app exists in any environment | The owner's LinkedIn developer app. Tokens last 60 days with no refresh | `internal/publisher/linkedin/` |
| **Threads** | Yes: the user's own profile | No. Offered only when `THREADS_APP_ID` is set, and no Meta app exists | The owner's Meta app with the Threads use case; testers only before App Review. Media needs a public bucket, so it cannot work against local storage | `internal/publisher/threads/` |
| **WhatsApp** | Yes, as an **assisted hand-off**: the iPhone app opens WhatsApp's share sheet and you tap Send | Not proven. Never tried on a device; the simulator has no WhatsApp | A real iPhone with WhatsApp. iPhone only. Cannot be scheduled | `ios/Postitheka/ShareSheet.swift`, `internal/jobs/handoffs.go` |
| **X** | **No** | No | Developer app plus pay-per-use credits ($0.015 a post, $0.20 with a URL, billed to the owner) | `BACKLOG.md` X-001 |
| **Instagram** | **No** | No | Meta app, Professional account, App Review for strangers | IG-001 |
| **Facebook Pages** | **No** | No | Meta app, Business Verification and App Review | FB-001 |
| **TikTok** | **No** | No | Developer app and audit; drafts first | TT-001, TT-002 |
| **Reddit** | **No** | No | Reddit's approval; the rollout recommends doing Reddit by hand | RDT-001, RDT-002 |
| **Bluesky, Mastodon** | **No** | No | Deferred in `BACKLOG.md` | |

`GET /v1/platforms` lists exactly the publishers registered in `cmd/server/main.go`: Discord always, LinkedIn and Threads only when their app credentials are set. WhatsApp is not in that list; the iPhone composer adds its stamp itself.

---

## 2. Target audience

**Needs confirmation.** The audience below is inferred from the product and from the owner's 2026-11-05 decision. The repository contains no research, interviews or usage data, and the strangers count is 0. Replace these personas with evidence once real users exist.

**Primary user, decided.** The owner, promoting his own apps (`docs/platform-rollout.md`, "Decided 2026-11-05"). The product is also to be offered to the public.

**Secondary users, inferred.**

- Indie developers and small founders who announce releases in their Discord community and on their own LinkedIn and Threads profiles.
- Community managers who post the same announcement into several Discord channels or servers. This is the one case that works end to end on today's code without any developer app.
- Organisers who post to a Discord server and also send the same update to a WhatsApp group from their phone.

### Personas (hypotheses)

| Persona | Situation | Frustration | What Postitheka offers them |
|---|---|---|---|
| **The indie maker** | Ships several small apps, announces each release | Rewrites one update for each network, re-uploads the same screenshots, forgets one | One composer, every chosen stamp checked as they type, one Publish, a postmark per destination |
| **The Discord community manager** | Runs announcements in several servers or channels | Pasting into each channel, and pinging everyone by accident | Several webhooks as stamps, mentions always disabled, a link to each message |
| **The founder on LinkedIn and Threads** | Posts launch notes on a personal profile | Different limits: 3,000 characters on one, 500 on the other, emoji counted differently | Live counters per stamp that use each network's own counting |
| **The group organiser** | Runs a Discord server and a WhatsApp group for the same people | WhatsApp has no posting tool at all | The iPhone app hands the post, files included, to WhatsApp, and records whether it was sent |

**Who Postitheka is not for, today.** Anyone who needs X, Instagram, Facebook, TikTok or Reddit. Teams and agencies (shared accounts are deferred). People who want engagement analytics (deferred), editing or deleting posts after publishing (deferred), a different text per network (premium, not built), or AI-written posts (none). LinkedIn Company Pages (not in scope). Android users. Anyone who cannot run a local server, until it is deployed.

---

## 3. Problem and solution

**Main problem.** Every network has its own limits, upload flow and failure modes. Posting the same thing to four places is four jobs, each of which can fail quietly, and a retry after a dropped connection can leave a duplicate on someone's timeline.

**Why it matters.** For a small team the announcement is the work that gets skipped. A missed network means a missed audience; a duplicate or a broken post looks careless in public.

**How Postitheka solves it.**

1. **Checked at the door.** As you type, each chosen stamp shows its own counter and what it would object to. When you press Publish, the server checks every destination first; if any would refuse, it answers with every issue and stores nothing (`internal/publisher/rules.go`, `internal/service/posts.go`).
2. **Independent after.** Once accepted, each destination is its own job: it publishes, waits exactly as long as a rate limit says, retries on its own, or fails with a reason (`internal/jobs/publish.go`).
3. **Never twice.** A press of Publish carries an idempotency key, and a job that finds its previous attempt died mid-request fails with "check the platform before posting again" instead of risking a duplicate.
4. **Shown per destination.** Sent lists every post with a postmark and a link for each place it landed, and a plain reason for each place it did not.

**Primary value proposition.** Write it once; Postitheka checks it for every network before it goes, and tells you exactly where it landed.

### Key user benefits

1. **One draft, many destinations.** One text and one set of files, sent to every stamp you choose.
2. **Know before you press Publish.** Each stamp counts the way its network counts and names the problem: "N characters over", "A video must be posted alone", "Cannot take image/gif".
3. **See where it landed.** A postmark and a link for each published copy; "Not sent" with a reason for each failure.
4. **No duplicates.** Built so that a retry never posts the same thing twice.
5. **Official routes only.** No scraping and no automating a network's own app. You sign in on the network's own page, and stored credentials are encrypted and never leave the server.

---

## 4. Positioning and differentiation

**Positioning statement (draft).** For makers and community managers who post the same update to several places, Postitheka is the cross-poster that checks the post against every network before it goes, and shows exactly where each copy landed. It uses only official APIs, and it is built never to post twice.

**The chosen claim (proposal, Needs confirmation).** Certainty: "know it fits, know it landed". The product's vocabulary already carries it: stamps you choose, a postmark when it lands, "Not sent" when it does not. Do not sell a list of networks; the list is short today.

### Differentiators, with evidence and limits

| Claim | Evidence | Limit you must respect |
|---|---|---|
| Checks every destination before storing anything | `Validate` in `internal/publisher/rules.go`; 422 with every issue in `internal/service/posts.go` | Checks text length, links, file count, type and size. **Not video duration or aspect ratio** (VID-001 open) |
| Counts the way each network counts | `TextCounting` modes `code_points` and `emoji_bytes`, mirrored in Go, `web/src/rules.ts` and the kit's `Rules.swift` | Only Threads counts differently today. X's weighted counting is not built |
| Never posts twice | `Idempotency-Key` (API-001, `TestCreatePostIsIdempotent`), the interrupted-attempt rule in `internal/jobs/publish.go` | It prefers a failure to a duplicate. Say "built never to post twice", not "guaranteed delivery" |
| Each destination succeeds or fails alone | One River job per target | There is **no Retry button** yet (WEB-001). A failed copy is re-sent by writing the post again |
| Shows where each copy landed | Postmark with `remote_url` in `web/src/Sent.tsx`, "Open" link in `ios/Postitheka/SentView.swift` | WhatsApp has no link: it shows "Sent from the phone" |
| Official APIs only | Product rule in `CLAUDE.md`; WhatsApp is a hand-off because it has no posting API | WhatsApp needs your tap on the phone. Do not call it automatic |
| Your password never reaches Postitheka | OAuth on the platform's own page (`internal/service/connect.go`); copy in `Accounts.tsx` | Applies to LinkedIn and Threads. Discord connects with a webhook URL, which is itself the credential |
| Mentions never fire by accident | `allowed_mentions.parse = []` in the Discord publisher | Discord only |
| Scheduling | `scheduled_at` on posts, River jobs scheduled at that time | Needs a server that stays up, which does not exist yet. A scheduled post cannot be edited or cancelled from the UI. WhatsApp cannot be scheduled |

### Reasons someone would choose Postitheka (once it is live)

- They post to Discord plus LinkedIn or Threads, and want one place to do it.
- They have been burned by a duplicate or by a post that silently failed.
- They want to see, per network, what went out.
- They want a tool that stays within each network's official API.

### Alternatives

No feature-by-feature comparison has been done, so **make no comparative claims**.

- Established social schedulers exist (for example Buffer and Hootsuite). This is general market knowledge, not research in this repo. Do not compare features or prices.
- Posting by hand in each app is probably the real competitor. This is inferred.
- The networks themselves are destinations, not rivals. Do not show them in a negative light.

---

## 5. Marketing messaging

All of this is draft. There is no live landing page, store listing or approved copy to inherit from.

**Main headline (draft).** Write it once. Know where it landed.

**Supporting headline (draft).** One post, checked against every network before it goes, with a postmark for each place it lands.

**Brand tagline.** "Write once. Publish everywhere." Use it only as the brand line next to the logo, and always with the real list of networks nearby, or not at all until the owner decides (section 9).

**Elevator pitch (30 seconds).**

> Posting one update to Discord, LinkedIn and Threads usually means three apps, three sets of limits and three uploads, and then checking which ones actually went out. Postitheka takes the post once. As you type, each destination shows whether it fits. When you publish, it goes to each one through the network's official API, and you get a postmark and a link for every place it landed. It is built never to post twice. It is in development, and not open to the public yet.

**Short description (49 words).**

> Postitheka is a cross-poster in development. Write a post once, with text, photos or video, and choose where it goes. Each destination is checked as you type, and published through its official API. You see a postmark and a link wherever it landed. Built never to post twice.

**Long description (167 words).**

> Postitheka is for people who post the same update in several places.
>
> Write your post once and add photos or a video. Choose the destinations you want, which Postitheka calls stamps. Each stamp checks the post as you type, with its own character count and its own limits, so you know before you press Publish that it fits. If any destination would refuse it, nothing is sent until you fix it or deselect that stamp.
>
> Then each destination goes on its own. When one lands, it gets a postmark with a link to the live post. When one fails, it says why. A dropped connection never turns into a duplicate post.
>
> Postitheka connects to Discord channels, to your LinkedIn profile and to Threads, using each network's official API. On iPhone it can also hand a post to WhatsApp for you to send. You can schedule a post for later.
>
> Postitheka is in development and not yet open to the public.

When a deployed version exists, remove the last line and check every network named against `features.md`.

### Five benefit messages

1. **It fits before it goes.** Every stamp counts the way its network counts.
2. **One press, every destination.** One text, one set of files, each chosen network.
3. **A postmark for every landing.** Where it went, when, and a link to see it.
4. **Never twice.** A retry never puts the same post on a timeline twice.
5. **The front door, not the back.** Official APIs only. You sign in on the network's own page.

### Brand voice

There is no written voice guide. This is inferred from the UI copy, which is plain, specific and honest about failure: "Press Publish again; it will not post twice.", "The phone never reported back. Check WhatsApp before sending it again.", "Publish unlocks when uploads finish." See `rules/brand.md`.

| Do | Don't |
|---|---|
| Specific: "Threads counts an emoji as its bytes" | Hype: "supercharge your reach", "go viral", "10x" |
| Admit limits: "Not sent: the webhook was deleted" | Promises about engagement, reach or growth |
| The product's own words: stamp, postmark, Publish, Published, Not sent | "Blast", "spray", "automate your social media" |
| "Discord, LinkedIn and Threads" | "Every platform", "all your socials", logos of networks that are not built |
| "Built never to post twice" | "Guaranteed", "100% reliable" |

---

## 6. Conversion

**Primary call to action, today.** **Needs confirmation.** The only live destination a viewer can reach is the community Discord linked from the web header and the iPhone Accounts tab: "Join us on Discord", `https://discord.gg/nYhWNMRJC` (`web/src/links.ts`, `ios/Postitheka/AccountsView.swift`). Whether the owner wants it as the marketing call to action, and whether the invite is permanent, is unconfirmed.

**Secondary call to action, later.** "Join the iPhone beta" once a TestFlight build exists, and a sign-up link once the hostname exists. Do not write either before then.

**Pricing.** Verified: no billing code exists on any platform, and nobody can use the product. Decided on 2026-11-05: free users write one text for all networks; premium users can write a different text per network. Still open: whether X is premium, and how a user pays (StoreKit on iOS, something on the web). **Never mention a price, a tier name, "free" or "premium" in public material** until the owner sets them.

**X costs the owner money per post.** Every stranger's post to X would bill the owner's developer account. This is why the X decision gates any public offer that includes X.

### Suggested landing page messaging (for when a landing page exists)

1. **Hero.** "Write it once. Know where it landed." A real recording of stamps checking a post, then postmarks arriving.
2. **It fits before it goes.** Counters and issues per stamp.
3. **One press, every destination.** The Publish moment and the Sent list filling.
4. **Never twice.** One plain paragraph on the idempotency rule.
5. **Your phone, for WhatsApp.** Only after the hand-off is proven on a device.
6. **The networks.** The built list only, with a dated note for what is next. No "coming soon" logos.
7. **Your accounts, your keys.** Official sign-in pages, encrypted credentials, disconnect any time and history stays.
8. **Pricing and call to action.** Once decided.

### Objections

| Objection | Honest answer |
|---|---|
| "Does it post to X / Instagram / TikTok?" | Not yet. Today: Discord, LinkedIn and Threads, plus WhatsApp from the iPhone. Others are in the backlog, each blocked on that network's review or pricing |
| "Will it post twice if my connection drops?" | No. Publish is idempotent, and a job that cannot tell whether it went out fails and says "check the platform" rather than post again |
| "What do you do with my accounts?" | You sign in on the network's own page. Credentials are encrypted with AES-256-GCM, never logged, never sent to the apps. Disconnecting keeps your history |
| "Can I write a different text for each network?" | Not yet. It is decided as a premium feature, not built |
| "Can I edit or delete a post afterwards?" | No, and a scheduled post cannot be cancelled from the app yet |
| "Can I retry a failed one?" | Not with a button yet (WEB-001). You write the post again for that destination |
| "Does it use AI?" | No |
| "Who is behind this?" | Fernando Correia. No privacy policy or terms are published yet, and there is no company or audit to point to. Say so |
| "I'm on Android" | There is no Android app. The web composer will work once it is deployed |

---

## 7. Measurement

**Activation (proposal, from the P1 gate in `BACKLOG.md`).** A person who is not the owner signs in on production without help from a terminal, connects a channel, and publishes a post that lands.

**Leading indicator (proposal).** An account connected (`account_connected`).

**Events that exist.** `internal/analytics` sends four events to PostHog, keyed by user id: `signed_up`, `account_connected` (platform), `target_published` (platform, attempt), `target_failed` (platform, coarse reason). Never an email or post text. **The PostHog key has not been provided**, so nothing is sent anywhere today (OPS-001: built, unverified). The iPhone app has no analytics SDK; iOS activity is visible only through these server events.

**Kill criteria.** None are recorded. **Needs confirmation:** the owner sets them before any paid promotion. A starting point, in the shape other apps here use: ten conversations and no sign-ups means the pitch is wrong; five sign-ups and no published post means connecting is too hard; no conversations means distribution is wrong.

**Current numbers.** Strangers who have published a post: **0** (`BACKLOG.md`).

---

## 8. What must be fixed before spending on promotion

These were found during this review. None was changed, because the brief excluded application code.

**Nothing can be used by a stranger**

| Gap | Backlog | Who |
|---|---|---|
| Not deployed: no R2 bucket, no namespace, no hostname, nothing in `platform/infra` | REL-003, REL-004 | Owner decides the hostname and approves the infra merge |
| No sign-up: a token is minted on the command line and pasted into an "API token" field | AUTH-001, IOS-002 | Owner creates the Sign in with Apple key |
| No privacy policy or terms | PRV-001 | Agent drafts, owner approves |
| No rate limits | OPS-002 | Agent |
| No analytics key | OPS-001 | Owner |
| No TestFlight build. The Release configuration has no server address | P3 | Owner |
| No developer app for LinkedIn or Threads, so neither can be connected anywhere | LI-001, THR-001 | Owner |
| No publisher has ever posted to a real platform | Rollout step 0 | Owner, with a test account |
| Meta App Review and Business Verification, needed for strangers on Threads | META-001 | Owner |

**Would hurt early users**

- LinkedIn tokens die after 60 days and nothing shows it (ACC-001 open). Every post to that stamp then fails until the user reconnects.
- No Retry for a failed destination (WEB-001).
- Discord forum and media channels fail permanently (DSC-001).
- Video duration and aspect ratio are not checked (VID-001), so a video can pass the door and be refused by the network later.

**Copy that is false or misleading today**

| Where | Says | Reality |
|---|---|---|
| `CLAUDE.md`, first paragraph | Choose where it goes "(Discord, X, Instagram, Facebook, TikTok, WhatsApp)" | X, Instagram, Facebook and TikTok are not built. LinkedIn and Threads, which are built, are missing from the list |
| Key art, both sign-in screens | "Write once. Publish everywhere." | Three networks plus a WhatsApp hand-off, none live |
| `ARCHITECTURE.md`, system context diagram | Server publishes to "Discord, X, Meta, TikTok (direct)" | X and TikTok are not built; of Meta, only Threads |
| `BACKLOG.md`, Deferred | "Bluesky, Mastodon, Threads, LinkedIn" | Threads and LinkedIn are built |
| `docs/platform-rollout.md`, platform table and Threads section | Threads "Built, not on `main`"; "merge first" | Threads is on `main` (step 1 is marked done in the same file) |
| `CLAUDE.md`, "Where it stands (2026-10-23)" | Built: "Discord publishing" | Also LinkedIn, Threads, OAuth connect and token refresh |
| Web and iOS sign-in | "API token", "Mint one with `task user:create`…" | Developer-only. Never film it as a sign-up screen |
| Web composer, no accounts | Button "Connect Discord" | Opens the whole Accounts dialog, which may also offer LinkedIn and Threads |
| iPhone Accounts | "Ask me to rate Postitheka", and a rating request after posts | The app is not on the App Store |

**Missing for production work**

- No demo environment, no seed script, and no fake-platform mode. A post cannot reach "Published" without a real network account (`rules/video-playbook.md`).
- No brand voice document and no landing page.

---

## 9. Evidence and assumptions

### How this was checked

Source code and documents were read on the working tree at `main` f8b8285, which matches `origin/main` and `initial-build`. Production state was read from `platform/infra` (`argocd/apps`, every branch ref, a text search for the name and its variants). The local `.env` was inspected for which keys are set, without reading their values.

**Not done.** Nobody ran the product for this review. No screen was rendered, no build was made, no request was sent to any network, and the cluster was not queried. "Verified" in `features.md` means the code exists on `main`, is reachable from the UI and is covered by tests, not that someone watched it work. **No screen was rendered for this review; rehearse before filming.**

### Verified

- Built publishers: Discord, LinkedIn, Threads (`internal/publisher/*`), registered in `cmd/server/main.go`, LinkedIn and Threads only when configured.
- WhatsApp is an assisted hand-off in the iPhone app only (`publisher.IsAssisted`).
- Postitheka is absent from `platform/infra` on every ref.
- The local `.env` sets no LinkedIn, Threads or PostHog keys.
- iOS: iOS 18.0 minimum, iPhone only, version 0.1, Release `API_BASE_URL` empty.
- No billing code anywhere. No AI or LLM calls anywhere.
- Platform limits as coded: Discord 2,000 characters, 10 files, 20 MiB each; LinkedIn 3,000 characters, 20 files, 5 MiB images, 500 MiB video, video alone; Threads 500 characters with emoji counted in UTF-8 bytes, 5 links, 20 files, 8 MB images, 1 GB video.
- The community Discord invite in both clients.
- 0 strangers, as recorded in `BACKLOG.md`.

### Inferred, so confirm before relying on it

| Item | Basis | How to confirm |
|---|---|---|
| A GHCR image exists for the latest `main` | `.github/workflows/image.yml`; REL-002 evidence for an earlier SHA | GitHub Packages for `facorreiaa/postitheka` |
| Personas and audience | The product surface and the owner's 2026-11-05 decision | Real user conversations |
| Brand voice | UI copy | Owner |
| The key art was generated, not drawn | `brand/README.md` calls the mascot "a cutout of a 1672 px render" | Owner |
| The Discord invite is permanent | It is hard-coded | Discord server settings |
| A Discord post from a local server works | Stub tests only | One post to a private test server |

### Needs confirmation (decisions for the owner)

1. **Can anything be marketed before deployment?** If yes, as what: a "building in public" preview, or only to the owner's existing audiences?
2. **Lead audience.** The owner's own promotion, indie makers, or Discord community managers?
3. **The tagline.** Keep "Publish everywhere" while three networks are built, qualify it, or replace it in marketing?
4. **Call to action.** Is the community Discord the call to action until there is a sign-up?
5. **Public hostname** (rollout decision 6).
6. **Tiers and payment** (decision 5), and whether X is premium.
7. **Which networks to name as "next"**, if any, and with what timing.
8. **Test accounts.** A private Discord test server for filming, and whether a dedicated LinkedIn or Threads test profile is acceptable (LinkedIn's terms are understood to forbid fake profiles; confirm).
9. **Founder on camera.** The only source of honest first-person claims.
10. **The operator's legal identity** for privacy, terms and ad accounts.
11. **Key art licence**, if it was generated, before paid use.
12. **Stranger count** once there is something to count.
