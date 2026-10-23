# Postitheka architecture

How the system fits together and why. `CLAUDE.md` holds the rules; this file explains the
shape they produce. Update it in the same change as any new platform, table, route or process.

## Goals

1. Write once, publish to several platforms, and show per destination what happened.
2. Never post twice, and never lose a post silently: every target ends `published` or `failed`
   with a reason.
3. One deployable: a single Go binary serves the API, the web app and the publish workers.
4. Adding a platform is one package, with no changes to the rest of the system.

## System context

```text
 ┌──────────────┐   ┌──────────────┐
 │ Web composer │   │     iOS      │──share sheet──► WhatsApp app (assisted publish)
 └──────┬───────┘   └──────┬───────┘
        │ /v1 (bearer)     │ /v1 (bearer)
        ▼                  ▼
 ┌─────────────────────────────────────┐        ┌──────────────────────┐
 │ postitheka (one Go process)         │──API──►│ Discord, X, Meta,    │
 │  HTTP: /v1, /healthz, web app       │        │ TikTok (direct)      │
 │  River workers: publish jobs        │        └──────────────────────┘
 └──────┬──────────────────┬───────────┘
        │                  │ presign / stat / read
        ▼                  ▼
 ┌─────────────┐    ┌──────────────┐◄── PUT (presigned) ── clients
 │ PostgreSQL  │    │ R2 / RustFS  │
 │ app + River │    │ media bucket │
 └─────────────┘    └──────────────┘
```

Clients never call a platform's API. The single exception is assisted publish, where the iOS app
opens the platform's own app and the user sends the post. The server still records it, as a
target with no account and no job, which the phone completes with what happened.

## Process

`cmd/server` runs three things in one process:

- the HTTP server (chi): `/healthz` outside the spec, `/v1/*` from the spec behind bearer auth,
  an unknown `/v1/*` path as a JSON 404, and everything else served by the embedded web app;
- the River client, working the `default` queue with `PUBLISH_WORKERS` concurrent jobs, plus an
  hourly sweep of unattached uploads and a ten-minute expiry of unreported handoffs (both also
  run at start);
- on boot in development, migrations and creation of the local bucket with its CORS rule.

Subcommands: `server migrate` applies the goose schema and then River's own migrations, and is
the in-cluster PreSync hook. `server user create EMAIL` mints an API token.

Shutdown stops HTTP first, then River with the remaining budget, so in-flight posts can finish:
a job cut off mid-request leaves its target `publishing`, and the next attempt fails it.

## Packages and boundaries

| Package | Owns | Must not |
| --- | --- | --- |
| `httpapi` | Wire types (generated), auth middleware, mapping errors to status codes | Hold rules or SQL |
| `service` | Accounts, uploads, post creation and validation, tokens | Know any platform's API |
| `publisher` | The `Publisher` interface, `Rules`, `Validate`, error classes, `Registry` | Import a platform package |
| `publisher/<platform>` | One platform: rules, connect, publish, error mapping | Touch the database |
| `jobs` | Running one target: load, decrypt, claim, publish, record. Sweeping unattached uploads | Validate posts (already done at the door) |
| `media` | Storage calls, presigned URLs, stable filenames | Decide what a platform accepts |
| `secretbox` | Sealing and opening credentials | Know what the credentials are |
| `analytics` | Product events to PostHog, queued and dropped rather than blocking | Capture anything a person wrote |
| `db` | Generated queries | Be edited by hand |

## Data model

```text
users ─┬─< api_tokens       token_hash (sha256), last_used_at
       ├─< accounts         platform, display_name, credentials (sealed, AAD = id), remote_id
       ├─< oauth_states     state_hash (sha256), platform, verifier (sealed, AAD = hash), client
       ├─< media            object_key, content_type, size_bytes, verified_at
       └─< posts            text, scheduled_at, idempotency_key + request_hash
              ├─< post_media    (post_id, position) → media
              └─< post_targets  account_id → accounts (SET NULL), platform, status,
                                last_error, remote_id, remote_url, published_at
```

- Everything cascades from `users`: deleting a user deletes all of their data.
- `accounts.id` is generated in Go, because the credentials are sealed to it before the insert.
  A blob copied to another row does not open.
- Disconnecting an account sets `post_targets.account_id` to NULL and keeps the history;
  `platform` is copied onto the target for the same reason. An assisted target (WhatsApp) has
  no account from the start: there is nothing to connect.
- `accounts.remote_id` is the platform's id for what was connected (a LinkedIn member), unique
  per user and platform. Connecting it again replaces the credentials of the same row, so its
  post history stays attached; that is how an expired OAuth token is renewed. NULL for Discord.
- `oauth_states` holds a consent page that is open: the state is stored only as a hash and the
  PKCE verifier sealed to it. Taking a state deletes it; one older than 15 minutes is refused,
  and a user's abandoned ones are cleared when they start another.
- `posts.idempotency_key` is unique per user (partial index, NULLs allowed). `request_hash` is a
  SHA-256 of the request body, so a key reused for a different post is told apart from a retry.
- River's tables (`river_job` and the rest) live in the same database, so a job is inserted in
  the same transaction as its target.

## Post lifecycle

```text
 POST /v1/posts
   │ Idempotency-Key seen before? ── same body ──► 201 with that post (current status)
   │                              └─ other body ─► 422 on Idempotency-Key
   │ validate every target (rules + media verified in storage)
   ├── any issue ──► 422 { issues[] }        nothing stored
   │
   └── tx: posts + post_media + post_targets + river_job per target ── commit ──► 201
                                                    │ (at scheduled_at)
                                                    ▼
 target: pending ──claim──► publishing ──ok──► published (remote_id, remote_url)
            ▲                   │
            └── retryable ──────┤ (River backoff, or RetryAfter → snooze)
                                ├── permanent ──────────────► failed (job cancelled)
                                └── last attempt (10) ──────► failed
 publishing found at job start (previous attempt died) ─────► failed "interrupted"

 assisted target (no job): handing_off ──phone: sent──────► handed_off (published_at)
                               │      └─phone: not sent──► failed
                               └── no report in 1 h ──────► failed "never reported back"
```

An assisted post cannot be scheduled: the handoff happens on the phone, now. The phone reports
through `POST /v1/targets/{id}/handoff`, conditional on `handing_off`, so a repeat of the same
outcome is accepted and anything else is a 409. `expire_handoffs` runs every ten minutes and
fails handoffs older than an hour (the app was killed with the share sheet open); its message
says to check WhatsApp, because the user may have tapped Send before the report was lost.

A repeat with a known key is answered before validation: the first post may have been accepted
under conditions that no longer hold, such as an account since disconnected. Two concurrent
requests with one key both validate, but the second insert waits on the unique index and fails;
its transaction, jobs included, rolls back and it answers with the first post. Keys do not
expire.

The claim (`MarkTargetPublishing`) is conditional on `pending`, so two runs of one job cannot
both publish. Outcomes are recorded with a context that survives the job's own timeout. River
jobs are unique by target id.

Retry budget: 10 attempts with River's default backoff, which spans roughly a day. Job timeout:
5 minutes (ten 20 MiB files streamed to Discord).

## Publishers

```go
type Publisher interface {
    Platform() Platform
    Rules() Rules
    Publish(ctx, credentials []byte, post Post, media MediaSource) (Result, error)
}
```

- `credentials` are opaque bytes the publisher produced when connecting (JSON of its own shape).
- `MediaSource` gives `Open(key)` for platforms that take uploads (Discord) and `PublicURL(key,
  ttl)` for platforms that download media themselves (Instagram, Threads, TikTok pull-from-URL).
- Connecting by webhook is platform-specific (`discord.Connect`). An OAuth platform implements
  `publisher.OAuthConnector` (`AuthorizeURL`, `Exchange`) and shares one flow, below; both end in
  `service.storeAccount`.
- A publisher whose credentials expire implements `publisher.Refresher` (`NeedsRefresh`, no
  I/O; `Refresh`). The worker refreshes before claiming the target, holding `SELECT … FOR
  UPDATE` on the account and asking `NeedsRefresh` again once it has the lock, so a rotating
  refresh token (X) is spent once however many posts go out together. A `Permanent` refresh
  error fails the target with "reconnect the account"; anything else leaves it pending for a
  retry. Nothing was sent to the platform yet, so neither case risks a duplicate.
- `Rules` gives per-file limits (`MaxMediaBytes`, and `MaxVideoBytes` for video when it
  differs), `VideoAlone` for platforms that take a video only on its own, `MaxLinks` for
  platforms that cap distinct links, and `TextCounting` for platforms that count characters
  their own way. `TextCounting` is a named mode (`code_points`, `emoji_bytes`), not a function,
  because the web and iOS clients count too: each mode exists three times (Go `rules.go`,
  `web/src/rules.ts`, the kit's `Rules.swift`), tested against the same cases.
- Registration is one line in `cmd/server/main.go`; `GET /v1/platforms` lists exactly the
  registered set with its rules.

### OAuth connect (built)

```text
 client ── POST /v1/accounts/{platform}/connect {client: web|ios} ──► server
        ◄── {authorize_url} ──  (state + PKCE verifier stored, 15 min)
 browser ──► platform consent page ──► GET /oauth/{platform}/callback?code&state  (no bearer)
                                       take state → Exchange(code) → storeAccount (upsert by remote_id)
        ◄── 303 /?connected=linkedin            (web)
            303 postitheka://connected?platform=linkedin  (iOS: ASWebAuthenticationSession ends)
            failures: connect_error / error = cancelled | expired | failed
```

The callback is outside the spec, like `/healthz`: it is a browser redirect with no bearer
token, and the state is what ties it to a user. It always answers with a redirect to the client
the flow started from, carrying a code, never a platform's message (that is logged). Redirect
URL: `PUBLIC_URL/oauth/{platform}/callback`, which each platform's developer app must list. A
platform is registered only when its client id and secret are set; `GET /v1/platforms` says
`connect: oauth` for it and clients show a Connect button.

### LinkedIn (built, against a stub)

Personal profile through "Share on LinkedIn" (`w_member_social`) and "Sign In with LinkedIn
using OpenID Connect" (`openid profile`, for the member id and name from `/v2/userinfo`). Both
self-serve. Code flow with the client secret; LinkedIn has no PKCE for server apps. Tokens last
60 days with no refresh token, so renewal is reconnecting. Every `/rest` call pins
`Linkedin-Version: 202610`; a sunset version answers 426 and is a code change.

Publish: images through `/rest/images?action=initializeUpload` then a PUT of the bytes; video
through `/rest/videos` (`initializeUpload` with the size, a PUT per byte range LinkedIn chooses,
keeping each ETag, `finalizeUpload`, then poll until `AVAILABLE`, at most 3 minutes); then
`POST /rest/posts` with `content.media` (one file) or `content.multiImage` (2 to 20 images).
The post URN comes back in `x-restli-id`; the link is `linkedin.com/feed/update/{urn}/`.
Commentary is LinkedIn's "little" format, so `| { } @ [ ] ( ) < > # \ * _ ~` are escaped, except
a `#` that starts a hashtag. Upload URLs come from LinkedIn's responses and are refused unless
they are https on a linkedin.com host, so the member's token never leaves LinkedIn. 401 →
permanent ("reconnect"); 429 → `Retry-After`, else wait for the daily reset at midnight UTC;
5xx → retry; other 4xx → permanent, except an upload LinkedIn has not finished taking in.

### Threads (built, against a stub)

The user's own profile through the Threads API (`threads_basic`, `threads_content_publish`),
with the Threads app id and secret. Authorize on `threads.com`, exchange on
`graph.threads.com`; no PKCE. The one-hour token is exchanged at once for a long-lived one (60
days), and `Refresher` renews it a week after each refresh; a refresh that fails while the
token has more than a day left keeps the current token rather than hold up a post. The user id
is a number past 2^53 and is read as `json.Number`.

A post is a container: `TEXT`, `IMAGE` or `VIDEO` with a presigned GET URL (an hour) that Meta
downloads, or a `CAROUSEL` of 2 to 20 item containers. Each container is polled until
`FINISHED` (3 minutes, inside the job's 5) before it is used; `ERROR` is permanent with Meta's
reason, `EXPIRED` and a timeout are retried. `threads_publish` is the only request that puts
anything on the profile, so when it fails without a 4xx answer the container is asked, on a
context of its own: `PUBLISHED` → success, `FINISHED` → retry, no answer → permanent "check
Threads before posting again". Tokens and the app secret travel in query strings and form
bodies where Meta's examples put them, so transport errors are stripped of their URL before
they can reach a log or `last_error`. Graph error codes: 190 → permanent ("reconnect"); 4, 17,
32, 613 or 429 → retry in an hour; 1, 2, 5xx → retry; anything else → permanent.

### Discord (built)

Incoming webhook: the URL is the credential. Connect checks the URL is one Discord issues
(https, a Discord host, `/api/webhooks/{id}/{token}`), then `GET`s it to confirm it exists and
to learn the name, guild and channel. Publish sends `POST ?wait=true` (without `wait`, a rejected
message still answers 204): JSON for text-only posts, otherwise multipart with `payload_json` and
`files[n]`, streamed from storage with an exact `Content-Length`. `allowed_mentions.parse = []`.
429 → `RetryAfter`; 5xx → retry; 401/404 → permanent ("webhook deleted"); other 4xx → permanent.

## Platform matrix

Checked dates matter: re-check a platform's docs before building it.

| Platform | Mode | Status | Notes |
| --- | --- | --- | --- |
| Discord | Direct (webhook) | **Built** | Checked 2026-10-22: 2,000 chars, 10 files, 20 MiB per file. No review. Forum channels need `thread_name` (not yet). |
| LinkedIn | Direct (OAuth 2.0) | **Built** (stub-tested) | Checked 2026-10-23: 3,000 characters, 20 images of 5 MB, or one MP4 up to 500 MB. Self-serve. Needs the owner's developer app to use. |
| Threads | Direct (OAuth, threads.com) | **Built** (stub-tested) | Checked 2026-10-23: 500 characters (emoji counted in UTF-8 bytes), 5 links, carousel of 2–20; JPEG/PNG 8 MB, MP4/MOV 1 GB. Testers before App Review; needs the owner's Threads app. |
| X | Direct (OAuth 2.0 PKCE) | Not started | Pay-per-use ($0.015 a post, $0.20 with a URL, 2026-10-23); owner decision. Weighted `CountText`. |
| Reddit | Direct (OAuth 2.0) | Not started | Needs Reddit's approval first; text and links only. |
| Facebook | Direct (Meta OAuth) | Not started | Pages only. Needs Business Verification + App Review. |
| Instagram | Direct (Instagram Login) | Not started | Business/Creator accounts, no Page needed. JPEG only, by URL (`PublicURL`). 50–100 posts a day. App Review. |
| TikTok | Direct (Content Posting API) | Not started | Until the app passes TikTok's audit, posts are private-only. Upload-to-inbox (drafts) is the low-friction first step. Verify current rules. |
| WhatsApp | Assisted (iOS share sheet) | **Built** | No posting API exists. The server sends nothing; it records the handoff the phone reports. |

## Media flow

1. `POST /v1/media {content_type, size_bytes}`: the server checks that at least one registered
   platform accepts that type and size, writes a `media` row, and returns a 30-minute presigned
   PUT URL plus the headers to send (`Content-Type`). Key: `uploads/{user}/{media}.{ext}`.
2. The client PUTs the file straight to the bucket. The browser needs the bucket's CORS rule.
3. On first use in a post, the server `Stat`s the object: it must exist and its stored
   `Content-Type` must equal the declared one (a presigned PUT pins neither). The real size is
   recorded and `verified_at` set. After that the row is trusted.
4. At publish time the publisher streams the object (`Open`) or hands the platform a presigned
   GET URL (`PublicURL`). The bucket is private.

5. An upload not attached to any post 24 hours after it was reserved is deleted, row and
   object, by the hourly sweep (`jobs.SweepMediaWorker`). It locks candidates `FOR UPDATE SKIP
   LOCKED`, so a row a post is attaching at that moment is skipped and one the sweep holds cannot
   be attached; the delete re-checks that no post references the row. Objects are deleted
   before the rows commit, so a failure leaves rows without objects for the next sweep, never
   objects no row points to.

Not yet handled: a deleted user's objects stay in the bucket (no route deletes users yet).

## Auth

Bearer tokens: `pk_` + 32 random bytes, base64url. Only the SHA-256 is stored; lookup is by hash
and records `last_used_at`. Today tokens are minted with `server user create`. Strangers need a
real sign-in first (AUTH-001). The middleware runs before the request body is decoded.

## API

All routes need `Authorization: Bearer`. The spec is `api/openapi.yaml`. Outside it:
`GET /healthz` and `GET /oauth/{platform}/callback`.

| Route | Does |
| --- | --- |
| `GET /v1/platforms` | Registered platforms and their rules |
| `GET /v1/accounts` | Connected accounts (never credentials) |
| `POST /v1/accounts/discord` | Connect a webhook; 400 if Discord does not recognise it |
| `POST /v1/accounts/{platform}/connect` | Start an OAuth connect: `{client}` → `{authorize_url}`; 404 if the platform does not connect by OAuth here |
| `DELETE /v1/accounts/{id}` | Disconnect; queued targets for it fail as "disconnected" |
| `POST /v1/media` | Reserve an upload, get a presigned PUT |
| `GET /v1/posts?limit=` | Newest first, with targets and media ids |
| `POST /v1/posts` | Validate, store and queue; 422 with every issue. `Idempotency-Key` makes retries safe. `assisted: ["whatsapp"]` adds handoff targets |
| `GET /v1/posts/{id}` | One post with per-target status |
| `POST /v1/targets/{id}/handoff` | The phone reports an assisted target: `{sent}`; 409 unless it is still `handing_off` |

Errors are `{message}`; validation errors are `{message, issues: [{account_id?, platform?,
field, message}]}`.

## Clients

### Web (`web/`, built)

Vite + React + TypeScript, served by the Go binary from `web/dist` through `go:embed`. Types
come from the spec (`npm run api:gen`). Sign-in stores the token in `localStorage`. The composer
mirrors `publisher.Validate` per selected account as the user types, uploads with XHR for
progress, and disables Publish until every chosen destination would accept the post. It sends
an `Idempotency-Key` that stays the same while the post is unchanged, so pressing Publish again
after a dropped connection cannot post twice. The Sent
list polls every 2 seconds only while a target is `pending` or `publishing`.

In development Vite serves on :5173 and proxies `/v1` to :8095.

### iOS (`ios/`, built: composer, Sent, accounts, WhatsApp handoff)

SwiftUI, iOS 18. `PostithekaKit` (a local Swift package) holds everything testable without a
simulator: the client generated from the spec by `swift-openapi-generator` (output committed,
checked by `task ios:check`), the rules mirror, the Keychain token store, uploads and the
`Composer` model. The app target is views only. The base URL comes from the build configuration
(`API_BASE_URL` in `ios/Config/*.xcconfig`); the app holds no secret but the user's token, in
the Keychain.

WhatsApp: after the server accepts a post with `assisted: ["whatsapp"]`, the app opens the share
sheet with the text and media and reports the outcome (`completed` on the WhatsApp activity
means sent) through the handoff route. Still to do: the share extension (start a post from
Photos) and on-device video conversion (`AVAssetExportSession`, H.264/AAC MP4) so the server
never needs ffmpeg.

## Deployment (planned)

The `maat` cluster through `~/Work/production/platform/infra` (ArgoCD auto-syncs `main`):
one Deployment running the image with `GO_ENV=production` and `AUTO_MIGRATE=false`, a PreSync
Job running `server migrate`, a Service on :8095, liveness and readiness on `/healthz`. Secrets
sealed for the app's namespace: `DATABASE_URL`, `TOKEN_ENCRYPTION_KEY`, S3 keys. R2 bucket with
the CORS rule and, later, a lifecycle rule for unattached uploads.

The image is multi-stage: Node builds `web/dist`, Go builds a static binary, Alpine runs it
(about 50 MB).

## Security notes

- Webhook URLs are validated to Discord's own hosts before the server ever requests them, so
  the server cannot be turned into a proxy (SSRF).
- Credentials: AES-256-GCM with the row id as associated data; a version byte allows key
  rotation later. Losing `TOKEN_ENCRYPTION_KEY` orphans every connected account.
- Uploaded type is checked against storage, not trusted from the client.
- Platforms that download media (Threads) get a presigned GET for that one object, valid an
  hour; the bucket stays private.
- OAuth: single-use state, stored hashed, 15 minutes; PKCE verifier sealed; client secrets only in
  the environment; the callback redirects only to the app's own `/` or the `postitheka://`
  scheme, never to a URL taken from the request.
- No rate limiting on the API yet (OPS-002).
- Analytics (`POSTHOG_API_KEY`, empty means off) records four events, keyed by user id:
  `signed_up` (a user row created), `account_connected` (platform), `target_published`
  (platform, attempt) and `target_failed` (platform, a coarse reason such as `rejected` or
  `interrupted`). Never an email, post text, or a platform's error text.
