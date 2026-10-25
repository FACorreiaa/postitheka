# Postitheka backlog

The ordered work list, derived from `CLAUDE.md` and `ARCHITECTURE.md`. It records decisions made
so far and introduces none: anything still undecided is marked **OPEN** and waits for the owner.

## How to read this

- IDs are stable. Never renumber or reuse one. The **Status** table is the record of what is
  done; update an item's row when it changes, with evidence (a commit, a test, a URL).
- Work is grouped into phases, each with an exit **gate**. Inside a phase, order is dependency
  order. Do not start an item whose **Needs** are open.
- **OPEN**: a decision only the owner can make. **Owner**: an action only the owner can take
  (accounts, payments, reviews, submissions). Agents prepare these; they do not do them.
- Size: **S** under a day, **M** one to three days, **L** a week or more (often waiting on a
  third party).
- Every item has a **Done when** that can be checked. "Implemented" is not a condition.
- **Deferred** items exist so nobody reopens them before their phase.

## Decisions recorded 2026-10-22

- **Stack** (owner's proposal, confirmed): Go API, a slim web frontend, SwiftUI for iOS. The web
  app is Vite + React + TypeScript, built to static files and embedded in the Go binary. No
  SvelteKit or other server-rendering runtime: everything is behind a sign-in, so there is no
  SEO need, and a second runtime would be a second deployable.
- **One process, one image.** API, web app and publish workers ship together. Jobs run on River
  in the same Postgres, so a post and its jobs commit in one transaction.
- **The contract is `api/openapi.yaml`.** Go server code, TypeScript types and (later) the Swift
  client are generated from it.
- **Discord connects by webhook, not by bot OAuth.** No review, and the URL is the whole
  credential.
- **The web composer came before X** (owner said "continue"; the agent chose the order). X needs
  a decision on a paid API tier, which is the owner's.
- **WhatsApp is assisted publish** through the iOS share sheet; it has no posting API.
- **Local S3 is RustFS**, because MinIO no longer publishes images.
- **Sign-in is a CLI-minted token until AUTH-001.** That is acceptable for the owner and not for
  strangers, so AUTH-001 is on the path to the P1 gate.
- **Sign-in is Sign in with Apple** (owner, 2026-10-22). Email sign-in was not chosen. App ID
  `com.fernandocorreia.postitheka` (team `84X9WYBF36`) is registered with the capability enabled
  as a primary App ID. The web Services ID waits for the hostname (REL-004); the owner creates
  the Sign in with Apple key, since its private key downloads once and goes straight into a
  sealed secret.
- **The GitHub repo exists** (owner, 2026-10-22). Nothing is pushed yet: pushing waits for the
  owner's word.
- **The public hostname is deferred** (owner, 2026-10-22: "not now").
- **Build it out** (owner, 2026-10-23): API, web client and iOS client, without waiting on the
  strangers count. Owner-blocked items are listed under "Waiting on the owner"; platform
  publishers are built and tested against stubs before the owner's developer apps exist, so a
  `Needs` on an owner account blocks real use, not the code.
- **No secrets in the repo** (owner, 2026-10-23). Enforced by a secret scan in CI.

## Waiting on the owner

Nothing below blocks building; it blocks shipping. Each is an account, a decision or a key that
only the owner can create. Build against stubs and configuration until they arrive.

| What | Unblocks | Notes |
| --- | --- | --- |
| R2 bucket + API token | REL-003, REL-004 | Agent writes the CORS rule |
| Public hostname | REL-004, AUTH-001 (web Services ID) | Deferred 2026-10-22 |
| Sign in with Apple key (.p8) | AUTH-001 | One-time download, straight into a sealed secret |
| PostHog project key | OPS-001 seen working | Sealed secret `POSTHOG_API_KEY` |
| LinkedIn developer app with "Share on LinkedIn" and "Sign In with LinkedIn using OpenID Connect" | LI-001 used for real | Self-serve. `LINKEDIN_CLIENT_ID` / `LINKEDIN_CLIENT_SECRET` in `.env`; redirect `http://localhost:8095/oauth/linkedin/callback` locally, `PUBLIC_URL/oauth/linkedin/callback` in production |
| X developer app + credits | X-001 used for real | Pay-per-use: $0.015 a post, $0.20 with a URL (2026-10-23) |
| TikTok developer app | TT-001 used for real, TT-002 | Photos also need a domain verified with TikTok |
| Reddit API access request | RDT-001 | Approval required since Nov 2025; commercial use needs an agreement |
| Meta app with the Threads API use case, owner as Threads Tester | THR-001 used before review | `THREADS_APP_ID` / `THREADS_APP_SECRET` (the Threads ones, not the main app's) in `.env`; redirect `PUBLIC_URL/oauth/threads/callback`. Meta may refuse a plain-http redirect (not checked); then `PUBLIC_URL` must be an https tunnel to the local server |
| Meta app, Business Verification, App Review | THR-001, IG-001 for strangers; FB-001 at all | Weeks; needs PRV-001 live |
| Approve privacy policy and terms text | PRV-001 | Agent drafts |

## Status (2026-10-23)

| Phase | Done | Gate | What blocks it |
| --- | --- | --- | --- |
| **P0 · Foundations** | 6/6 | **Met** | Nothing |
| **P1 · First stranger** | 4/12 (2 built, unverified) | Not met | Not deployed; no real sign-in |
| **P2 · Platforms** | 0/15 (4 built, unverified) | Not met | Needs P1 deployed; X needs credits, Reddit approval; Meta and TikTok need review |
| **P3 · iOS** | 1/6 (2 built, unverified) | Not met | IOS-002 needs AUTH-001; nothing tested on a device |

Strangers who have published a post: **0**.

### Every item

| ID | Status | Evidence | Caveat |
| --- | --- | --- | --- |
| FND-001 | Done | `internal/`, `cmd/server`; `task check` green | |
| FND-002 | Done | `internal/publisher/discord`; `discord_test.go` | Never posted to a real channel |
| FND-003 | Done | `internal/service/service_test.go` (`TestPostIsPublishedEndToEnd`, `TestWorkerOutcomes`) | |
| FND-004 | Done | `web/`; checked in Chrome at 1400 and 390 px | |
| FND-005 | Done | `migrations/20261022170000_post_media_cascade.sql`; `TestDeletingAUserRemovesEverything` | |
| FND-006 | Done | `Dockerfile`; image built, 50 MB, web app embedded | Never run in the cluster |
| REL-001 | Done | PR #1: CI green, `internal/service` tests ran (3.6 s, none skipped), image built | |
| REL-002 | Done | Image workflow run 39870125984 on `main` (38a1c3a) after PR #1 merged, 2026-10-22 | Package visibility not checked |
| REL-003 | Open | | Owner: Cloudflare account |
| REL-004 | Open | | Hostname deferred by the owner |
| AUTH-001 | Open | App ID `com.fernandocorreia.postitheka` registered | Needs hostname and the key (Owner) |
| API-001 | Done | `migrations/20261022190000_post_idempotency.sql`; `TestCreatePostIsIdempotent` (incl. 8 concurrent requests, one post) | Web retry path checked by typecheck only, not in a browser |
| MED-001 | Done | `internal/jobs/sweep.go`; `TestSweepDeletesOnlyOldUnattachedUploads` in CI; live run against RustFS removed row and object | |
| WEB-001 | Open | | |
| WEB-002 | Built, unverified | `web/src/styles.css` dark tokens | Never looked at in dark mode |
| PRV-001 | Open | | Owner approves the text |
| OPS-001 | Built, unverified | `internal/analytics`; `TestProductEventsCarryNoPersonalData`, events asserted in `TestWorkerOutcomes`; empty key is a no-op (`TestNoAPIKeyMeansNothingIsSent`) | Needs production and a PostHog project key (Owner) to see an event arrive |
| OPS-002 | Open | | |
| OAUTH-001 | Built, unverified | `internal/service/connect.go`, `internal/httpapi/oauth.go`, `migrations/20261023150000_oauth_connect.sql`; `TestOAuthConnect` (start, callback, replay, cancel, expiry, reconnect, two users); web Accounts checked headless at 1400 and 390 px, light and dark; iOS kit `ConnectTests` | Never run against a real platform; iOS connect screen built, not looked at on a simulator |
| X-002 | Built, unverified | `publisher.Refresher`, `PublishWorker.refresh` (row lock, re-check, seal, then publish); `TestTokenRefresh` (three concurrent posts refresh once and publish with the new token, revoked grant fails asking for reconnect, transient failure stays pending); mutation-checked; Threads implements it | Never run against a real platform |
| ACC-001 | Open | | |
| LI-001 | Built, unverified | `internal/publisher/linkedin`; `linkedin_test.go` (connect, text, escaping, one and several images, chunked video with processing, error classes, off-host upload URL); local server offers it with placeholder app values | Needs the owner's LinkedIn app to post for real; video limits partly from an ads section of the docs |
| DSC-001 | Open | | |
| META-001 | Open | | Owner |
| THR-001 | Built, unverified | `internal/publisher/threads`; `threads_test.go` (connect with the long-lived exchange, text, image, video processing, carousel order, `ERROR`/`EXPIRED`/timeout, publish without a clear answer incl. a cancelled request, error classes, refresh, no secrets in errors); mutation-checked; `max_links` and `text_counting` in the spec, mirrored and tested in web and iOS | Needs the owner's Threads app and tester account to post for real |
| X-001 | Open | | OPEN: buy credits |
| TT-001 | Open | | |
| TT-002 | Open | | Owner |
| FB-001 | Open | | Needs META-001 |
| IG-001 | Open | | META-001 for strangers; role holders can use it before |
| RDT-001 | Open | | Owner |
| RDT-002 | Open | | OPEN: title and subreddit |
| VID-001 | Open | | |
| IOS-001 | Done | `ios/`: Xcode project + `PostithekaKit`, client generated by swift-openapi-generator 1.14.0; `task ios:check` green locally and in CI (PR #2, run 39951558861, 2026-10-23) | |
| IOS-002 | Open | | Needs AUTH-001; a pasted token stands in |
| IOS-003 | Built, unverified | `ComposerView`, `Composer` (kit, tested); `PublishFlowTests` UI test published to a local server and saw the outcome in Sent | Photos path not exercised by the UI test |
| IOS-004, IOS-005 | Open | | |
| IOS-006 | Built, unverified | `POST /v1/targets/{id}/handoff`; `TestAssistedHandoff` (incl. expiry after an hour, `internal/jobs/handoffs.go`); kit `aWhatsAppHandoffOpensAfterPublishingAndIsReported`; UI test `testWhatsAppHandoffClosedWithoutSending` recorded the cancel as failed | WhatsApp is not on the simulator: the share sheet opened and was dismissed, but a real send was never tried. Needs a device |

**Known gaps no item owns yet:** deleting a user removes their media rows by cascade but leaves the objects in the bucket. No route deletes a user yet; whichever item adds one must delete the objects too.

---

# P0 · Foundations

**Gate:** a post written in the web app is validated, stored, queued and published to Discord
against a stubbed API, with each failure mode covered by a test. **Met 2026-10-22.**

### FND-001 · Go API skeleton · M
OpenAPI spec, generated chi server, config, Postgres with goose and sqlc, River, S3 storage,
sealed credentials, bearer tokens, `server migrate` and `server user create`.

**Done when:** `task check` passes and `/healthz` answers 200 against the local stack.

### FND-002 · Discord publisher · M
Webhook connect with host validation, text and multipart publish with an exact
`Content-Length`, mentions off, error classification. Rules checked against Discord's docs.

**Done when:** `discord_test.go` covers URL validation, connect, text, media, short files and
every error class against a stub.

### FND-003 · Post pipeline · M
Validate every target at the door (422 with every issue), store and queue in one transaction,
one River job per target, the claim and the interrupted-attempt rule.

**Done when:** a real River client publishes a post end to end in a test, and each worker
outcome (published, permanent, retry, last attempt, interrupted, disconnected) has a test.

### FND-004 · Web composer · M
Sign in by token, compose with uploads, per-destination stamps with live rule checks, Sent list
with postmarks and polling, accounts dialog. Embedded in the binary.

**Done when:** the full flow runs in a browser against the local stack, including a cross-origin
upload to storage.

### FND-005 · User deletion cascades fully · S
`post_media.media_id` was `ON DELETE RESTRICT`, which blocked deleting any user with an
attachment.

**Done when:** deleting a user with posts, media and accounts leaves no rows (test).

### FND-006 · Production image · S
Multi-stage Dockerfile: Node builds the web app, Go builds a static binary, Alpine runs it.

**Done when:** `docker build` succeeds and the binary in the image contains the web assets.

---

# P1 · First stranger

**Gate:** one person who is not the owner signs in on the production URL without help from a
terminal, connects a channel and publishes a post that lands. Measured, not assumed (OPS-001).

### REL-001 · GitHub repo and CI · S
**Needs:** Owner creates the repo (the module path assumes `github.com/FACorreiaa/postitheka`).
A workflow that runs `task check` with a Postgres service container, so database tests run
rather than skip.

**Done when:** a pull request runs the workflow, the service tests report as run (not skipped),
and it is green.

### REL-002 · Image published on merge · S
**Needs:** REL-001. Build the Dockerfile on `main` and push to GHCR tagged with the commit SHA,
following how the other apps on `maat` publish theirs.

**Done when:** a merge to `main` produces `ghcr.io/…/postitheka:<sha>`.

### REL-003 · R2 bucket · S · Owner
**Needs:** Owner creates the bucket and an API token in Cloudflare. The agent writes the CORS
rule (`PUT`, header `Content-Type`, the production origin) and documents it in the infra repo.

**Done when:** a browser on the production origin uploads to the bucket through a presigned URL.

### REL-004 · Deploy on maat · M
**Needs:** REL-002, REL-003. **OPEN:** the public hostname, and how public apps are exposed from
`maat` (read `platform/infra/docs` first). In `platform/infra`: namespace, Deployment
(`AUTO_MIGRATE=false`), PreSync Job running `server migrate`, Service, probes on `/healthz`,
sealed secrets for `DATABASE_URL`, `TOKEN_ENCRYPTION_KEY` and the S3 keys, sealed for this
namespace. Ask the owner before merging to the infra repo: merging deploys.

**Done when:** the production `/healthz` answers 200, and the owner publishes a post from
production to a real Discord channel.

### AUTH-001 · Sign in with Apple · M
**Needs:** the hostname (REL-004) for the web Services ID and its return URL; **Owner** creates
the Sign in with Apple key. Decided 2026-10-22: Sign in with Apple. The App ID is registered.
The server verifies Apple's identity token, finds or creates the user by Apple's stable `sub`,
and issues the same bearer token the API already accepts, so the rest of the system does not
change. The web uses Sign in with Apple JS; iOS uses `AuthenticationServices` (IOS-002).

**Done when:** a new person creates an account and reaches the composer on production without
anyone running a command for them.

### API-001 · Idempotent post creation · S
A network retry of `POST /v1/posts` today creates a second post, and so a second publish.
Accept an `Idempotency-Key` header; a repeat with the same key and user returns the first
response. The web app sends one key per press of Publish.

**Done when:** a test sending the same request twice with one key gets one post and one set of
jobs; with two keys, two.

### MED-001 · Clean up unattached uploads · S
Files reserved and uploaded but never posted stay in the bucket forever. A periodic River job
deletes media rows and objects older than 24 hours that no post references; or an R2 lifecycle
rule if the key layout allows it.

**Done when:** an unattached upload older than the cutoff is gone from the bucket and the
database, and an attached one is untouched (test).

### WEB-001 · Retry a failed target · S
A failed target can only be re-sent by writing the post again. Add a retry route that re-queues
one failed target, and a Retry action in Sent. For an "interrupted" failure the action first says
the post may already be live.

**Done when:** a failed target can be re-sent from the UI, and the route refuses a target that
is not `failed` (test).

### WEB-002 · Dark mode checked · S
The dark tokens exist but have never been looked at.

**Done when:** every screen is checked in dark mode at desktop and phone width, with
screenshots, and contrast passes WCAG AA for text.

### PRV-001 · Privacy policy and terms · S
Public pages served by the app. Needed for strangers and required by Meta's review (META-001).
The agent drafts; the owner approves the text.

**Done when:** both pages are live on production and linked from sign-in.

### OPS-001 · Know when a stranger posts · S
Product events through PostHog, as Seshat does (empty key: nothing sent): sign-up, account
connected, post published, target failed. This is how the P1 gate and the strangers count are
measured.

**Done when:** a post published on production appears as an event, and local development sends
nothing.

### OPS-002 · Rate limits · S
No limit on the API today. Per-user limits on post creation and upload reservation, and per-IP
on unauthenticated requests.

**Done when:** exceeding a limit returns 429 with `Retry-After` (test).

---

# P2 · Platforms

**Gate:** the same post reaches Discord and at least two of LinkedIn, X, Threads, Instagram,
TikTok and Facebook from production, for a user who is not the owner.

What each platform allows, with sources and the date checked, is in
`docs/platform-research.md` (re-checked 2026-10-23). Every platform below except Discord connects
through OAuth, so OAUTH-001 comes first. Order after it: LinkedIn (self-serve, usable as soon as
the owner creates the app), Threads and Instagram (usable by app testers before review), X
(code now, pay-per-use credits to use), TikTok (private-only until audited), Reddit (needs
Reddit's approval first).

### OAUTH-001 · OAuth connect flow · M
Shared by every OAuth platform. `POST /v1/accounts/{platform}/connect` returns the consent URL;
the platform redirects to `GET /oauth/{platform}/callback` (outside the spec: a browser redirect
with no bearer token), which stores the account and redirects to the client the user started
from (`/?connected=` on the web, `postitheka://connected` on iOS). State is single-use, stored
hashed, 15 minutes; the PKCE verifier is sealed. Reconnecting the same platform user renews the
credentials of the existing account (`accounts.remote_id`) instead of adding a second one.
`GET /v1/platforms` says how each platform connects (`webhook` or `oauth`). `PUBLIC_URL` is the
redirect base; client ids and secrets are per-platform environment variables, and a platform
without them is not registered.

**Done when:** start, callback, cancel, expired and replayed state, and reconnect are tested
against a stub platform; the web and iOS Accounts screens open the consent page for an `oauth`
platform and show the outcome.

### X-002 · Token refresh for OAuth platforms · S
**Needs:** OAUTH-001. Refresh before publishing, under a row lock on the account, and store the
result sealed before publishing: X rotates refresh tokens, so two jobs refreshing at once would
lock the account out. Refresh is the publisher's: X and TikTok use a refresh token, Threads and
Instagram refresh the long-lived token itself (only once it is 24 hours old), LinkedIn has none.

**Done when:** a publish with an expired access token refreshes it and succeeds, and two
concurrent publishes for one account refresh once (tests).

### ACC-001 · Accounts that need reconnecting · S
**Needs:** OAUTH-001. A token that has expired and cannot be refreshed (LinkedIn after 60 days,
Threads and Instagram after 60 days unused, any revoked grant) fails every target until the user
reconnects. Mark the account, fail its targets with "reconnect LinkedIn", and show Reconnect on
the account and its stamp.

**Done when:** a 401 from a stubbed platform marks the account, the composer shows the stamp as
needing reconnection, and reconnecting clears it (tests and a browser check).

### LI-001 · LinkedIn publisher · M
**Needs:** OAUTH-001. Personal profile through "Share on LinkedIn" (`w_member_social`,
self-serve, no review). Posts API (`/rest/posts`, pinned `Linkedin-Version`), images through the
Images API (one image, or 2 to 20 as a multi-image post), video through the Videos API (4 MB
parts, finalize, wait for processing). Tokens last 60 days with no refresh, so renewal is
reconnecting (ACC-001). Company Pages need the Community Management API: not in scope.

**Done when:** tests against a stub cover connect, text, one image, several images, video and
error classes; the owner posts to their own profile from a local server once the app exists.

### DSC-001 · Discord forum channels · S
Forum and media channels need `thread_name`; today they fail permanently with a 400. Detect the
channel type when connecting (from the webhook's channel) and send a thread name taken from the
post's first line.

**Done when:** a stubbed forum webhook receives `thread_name`, and a normal channel does not.

### META-001 · Meta Business Verification and App Review · L · Owner
**Needs:** PRV-001 and a deployed build (REL-004). Weeks of waiting, so start as soon as P1
deploys. One review covers the Threads permissions (`threads_basic`,
`threads_content_publish`), Instagram's (`instagram_business_basic`,
`instagram_business_content_publish`) and Pages'. The agent prepares the permissions list, the
use-case text and a screen recording of the flow; the owner submits.

**Done when:** the Meta app is approved for the Threads, Instagram and Pages publishing
permissions.

### THR-001 · Threads publisher · M
**Needs:** OAUTH-001, X-002. Threads API: create a container, poll until `FINISHED`, publish.
Media is fetched by Meta from a URL (presigned GET), never uploaded. 500 characters with emoji
counted in UTF-8 bytes (`Rules.CountText`), at most 5 links, carousels of 2 to 20, JPEG/PNG up to
8 MB, MP4/MOV up to 5 minutes and 1 GB, 250 posts a day. Before review only Threads testers can
connect, which is enough for the owner.

**Done when:** tests against a stub cover connect, the container lifecycle (including `ERROR`,
`EXPIRED` and a publish that timed out), carousels and error classes; the owner posts to their
own account from a local server.

### IG-001 · Instagram publisher · M
**Needs:** OAUTH-001, X-002. Instagram API with Instagram Login: Business and Creator accounts,
no Facebook Page needed. Same container model as Threads. Images are JPEG only and fetched by
URL; video can be uploaded (resumable). The docs give both 50 and 100 posts a day: read
`content_publishing_limit` instead. Captions on carousels go on the parent only. Role holders can
connect before review.

**Done when:** tests against a stub pass, and a photo post lands on the owner's account.

### X-001 · X publisher · M
**OPEN:** whether to buy X API credits. X is pay-per-use as of 2026-10-23: $0.015 per post,
$0.20 for a post containing a URL, no subscription. Publisher with OAuth 2.0 PKCE (the code
lives 30 seconds, so the callback exchanges at once), media through the v2 chunked upload
(`media.write`), and X's weighted character counting as `Rules.CountText`. The code is built
against stubs; only real use waits on the decision.

**Done when:** tests against a stub cover connect, token refresh, text, media and error classes,
and the owner posts to a real X account.

### TT-001 · TikTok upload to inbox · M
**Needs:** OAUTH-001, X-002, VID-001. Upload a video to the user's TikTok drafts
(`video.upload`), which they finish in the app. Chunked upload (5 to 64 MB chunks, in order).
Photos can only be pulled from a domain verified with TikTok, so they wait for a custom domain on
the bucket. At most 5 pending drafts a day. Access tokens last 24 hours, refresh tokens a year.

**Done when:** a stubbed upload-to-inbox flow is tested, and the owner sees a draft in their
TikTok app.

### TT-002 · TikTok audit for direct posting · L · Owner
**Needs:** TT-001. Until audited, direct posts are private-only and at most 5 users a day can
post. Direct posting also requires a set UI (creator info, a privacy choice with no default,
interaction toggles off). The agent prepares the audit material; the owner submits.

**Done when:** TikTok approves direct posting with public visibility.

### RDT-001 · Reddit API access · S · Owner
Since November 2025 Reddit approves every new Data API client, and commercial use needs a
separate written agreement. Native image and video upload is not in Reddit's official API docs,
so under "official APIs only" Reddit is text and link posts. The owner requests access.

**Done when:** Reddit approves the app for posting on users' behalf.

### RDT-002 · Reddit publisher · M
**OPEN:** Reddit posts need a title and a subreddit, which a post does not have. Either an
account is one subreddit and the title is the post's first line, or the composer gets a title
field. **Needs:** RDT-001 for real use, OAUTH-001, X-002. Text and link posts only, checked
against the subreddit's `post_requirements` (flair, title length, banned domains) when the post
is created.

**Done when:** tests against a stub cover connect, a text post, a link post, a subreddit that
requires flair, and error classes.

### FB-001 · Facebook Pages publisher · M
**Needs:** META-001. Pages only: posting to personal profiles is not possible. Connect lists the
user's Pages and creates one account per chosen Page.

**Done when:** tests against a stub pass, and a post lands on a real Page from production.

### VID-001 · Video rules · M
Platforms limit video duration, aspect ratio and codec, which `Rules` cannot express today. Add
the fields, measure duration and dimensions (client-side in web and iOS, checked on the server
where possible), and show the problems on the stamps.

**Done when:** a video too long for one platform blocks only that platform's stamp, with the
reason.

---

# P3 · iOS

**Gate:** the owner installs a TestFlight build, signs in, and publishes a post with a photo
from the share sheet, plus one assisted publish to WhatsApp.

### IOS-001 · Project and generated client · M
SwiftUI app, client generated from `api/openapi.yaml` with `swift-openapi-generator` and
committed; `task check` fails if it is stale.

**Done when:** the app builds in CI and lists `GET /v1/platforms` from a local server.

### IOS-002 · Sign in · S
**Needs:** AUTH-001. The iOS side of whatever sign-in was chosen.

**Done when:** a fresh install signs in and reaches the composer.

### IOS-003 · Composer · M
Same flow as the web: text, media, stamps with rule checks, publish, per-target status.

**Done when:** a post published from the app shows the same outcomes as on the web.

### IOS-004 · Background uploads and on-device video conversion · M
Background `URLSession` uploads to the presigned URL, so they survive the app being closed.
Video is converted on the device (H.264/AAC MP4) to the strictest selected platform's limits,
so the server never needs ffmpeg.

**Done when:** a large video uploads after the app is sent to the background, and the
converted file passes every selected stamp.

### IOS-005 · Share extension · M
Start a post from Photos or any app that shares media.

**Done when:** sharing a photo from Photos opens the composer with the photo attached.

### IOS-006 · WhatsApp assisted publish · S
Hand the text and media to WhatsApp through the share sheet. The server sends nothing; it holds
a target in `handing_off` that the phone completes, so Sent records it as handed off, not as
published. A handoff the phone never reports fails after an hour.

**Done when:** the post opens in WhatsApp ready to send, and Sent shows it as handed off.

---

# Deferred

Not before P2's gate, so nobody reopens them early.

- **Bluesky, Mastodon, Threads, LinkedIn.** Free or review-light, but not on the owner's list.
- **Different text per platform** in one post.
- **Rewriting a post per platform with an LLM.** If it ever comes, it follows the shared Haiku
  trial rules in `~/Work/production/CLAUDE.md`.
- **Editing or deleting a post on the platforms after publishing.**
- **Teams and shared accounts.**
- **Engagement analytics** (likes, views) read back from platforms.
