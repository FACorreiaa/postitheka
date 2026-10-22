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

## Status (2026-10-22)

| Phase | Done | Gate | What blocks it |
| --- | --- | --- | --- |
| **P0 · Foundations** | 6/6 | **Met** | Nothing |
| **P1 · First stranger** | 1/13 (4 built, unverified) | Not met | Not deployed; no real sign-in; not pushed to GitHub |
| **P2 · Platforms** | 0/8 | Not met | Needs P1 deployed; X needs an owner decision; Meta and TikTok need review |
| **P3 · iOS** | 0/6 | Not met | Not started; needs AUTH-001 |

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
| REL-001 | Built, unverified | `.github/workflows/ci.yml`; `task check` green locally; testdb fails instead of skipping when `CI` is set | Never run on GitHub: push waits for the owner |
| REL-002 | Built, unverified | `.github/workflows/image.yml` | Never run; the GHCR package may need its visibility set on first push |
| REL-003 | Open | | Owner: Cloudflare account |
| REL-004 | Open | | Hostname deferred by the owner |
| AUTH-001 | Open | App ID `com.fernandocorreia.postitheka` registered | Needs hostname and the key (Owner) |
| API-001 | Done | `migrations/20261022190000_post_idempotency.sql`; `TestCreatePostIsIdempotent` (incl. 8 concurrent requests, one post) | Web retry path checked by typecheck only, not in a browser |
| MED-001 | Built, unverified | `internal/jobs/sweep.go`; `TestSweepDeletesOnlyOldUnattachedUploads` passed; live run against RustFS removed row and object | Full suite not re-run after it: local Docker stopped mid-gate |
| WEB-001 | Open | | |
| WEB-002 | Built, unverified | `web/src/styles.css` dark tokens | Never looked at in dark mode |
| DSC-001 | Open | | |
| PRV-001 | Open | | Owner approves the text |
| OPS-001 | Open | | |
| OPS-002 | Open | | |
| META-001 | Open | | Owner |
| X-001 | Open | | OPEN: API tier |
| X-002 | Open | | |
| TT-001 | Open | | |
| TT-002 | Open | | Owner |
| FB-001 | Open | | Needs META-001 |
| IG-001 | Open | | Needs META-001 |
| VID-001 | Open | | |
| IOS-001 … IOS-006 | Open | | |

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

### DSC-001 · Discord forum channels · S
Forum and media channels need `thread_name`; today they fail permanently with a 400. Detect the
channel type when connecting (from the webhook's channel) and send a thread name taken from the
post's first line.

**Done when:** a stubbed forum webhook receives `thread_name`, and a normal channel does not.

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

**Gate:** the same post reaches Discord and at least two of X, TikTok, Facebook and Instagram
from production, for a user who is not the owner.

### META-001 · Meta Business Verification and App Review · L · Owner
**Needs:** PRV-001 and a deployed build (REL-004). Weeks of waiting, so start as soon as P1
deploys. The agent prepares the permissions list, the use-case text and a screen recording of
the flow; the owner submits.

**Done when:** the Meta app is approved for the Pages and Instagram publishing permissions.

### X-001 · X publisher · M
**OPEN:** which X API tier to pay for, if any. Publisher with OAuth 2.0 PKCE connect, text and
media upload, and X's weighted character counting as `Rules.CountText`. Check X's current docs
and pricing first and date them in `Rules()`.

**Done when:** tests against a stub cover connect, token refresh, text, media and error classes,
and the owner posts to a real X account from production.

### X-002 · Token refresh for OAuth platforms · S
**Needs:** X-001. OAuth platforms issue expiring tokens. Refresh them before publishing and
store the new pair, sealed. Shared by every OAuth publisher.

**Done when:** a publish with an expired access token refreshes it and succeeds (test).

### TT-001 · TikTok upload to inbox · M
TikTok's Content Posting API can upload a video to the user's TikTok drafts, which they finish
in the app. This works before TikTok's audit. Check the current docs first.

**Done when:** a stubbed upload-to-inbox flow is tested, and the owner sees a draft in their
TikTok app.

### TT-002 · TikTok audit for direct posting · L · Owner
**Needs:** TT-001. Until audited, direct posts are private-only. The agent prepares the audit
material; the owner submits.

**Done when:** TikTok approves direct posting with public visibility.

### FB-001 · Facebook Pages publisher · M
**Needs:** META-001. Pages only: posting to personal profiles is not possible. Connect lists the
user's Pages and creates one account per chosen Page.

**Done when:** tests against a stub pass, and a post lands on a real Page from production.

### IG-001 · Instagram publisher · M
**Needs:** META-001. Business and Creator accounts only. Instagram downloads media from a URL,
so this is the first user of `MediaSource.PublicURL`; the presigned GET must outlive the
download. Mind the daily publishing limit.

**Done when:** tests against a stub pass, and a photo post lands on a real account from
production.

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
Hand the text and media to WhatsApp through the share sheet. The server is not involved; Sent
records it as handed off, not as published.

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
