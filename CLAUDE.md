# Postitheka

Postitheka is a cross-poster: write a post once, with text, photos or video, choose where it goes
(Discord, X, Instagram, Facebook, TikTok, WhatsApp), and it is published to each one, with the
outcome shown per destination. The name is post + *-theka*, a collection.

This file is the working contract for anyone, human or agent, changing the repo. Read
`ARCHITECTURE.md` before changing a boundary, and work from `BACKLOG.md` in order.

## Where it stands (2026-10-23)

- Built: the Go API, Discord publishing, the web composer, the iOS app (composer, Sent, accounts,
  WhatsApp handoff). Not deployed. **Strangers: 0.**
- Postitheka is not on the owner's active-projects list (`~/.claude/CLAUDE.md`), but on
  2026-10-23 the owner said to build it out (API, web, iOS) regardless of the strangers count,
  as with Aaru. Do not flag supply-side work here. What waits on the owner is listed in
  `BACKLOG.md` under "Waiting on the owner"; work around it, not on it.
- The owner's global rules apply here: no renames, rebrands or re-architectures until 10
  strangers have used the current version. Agents draft pixels; the owner keeps visual verdicts.

## Product rules

- **All or nothing at the door, independent after.** A post is checked against every chosen
  destination before anything is stored. If any would reject it, the API answers 422 with every
  issue and creates nothing. Once queued, each destination succeeds, retries or fails alone.
- **Never post twice.** Delivery is at least once, so a job that finds its target already
  `publishing` (the previous attempt died mid-request) marks it failed with "check the platform
  before posting again" instead of retrying. A duplicate on someone's timeline is worse than a
  failure they can retry by hand.
- **Official APIs only.** No scraping, no unofficial endpoints, no automating a platform's own
  app or website. A platform with no posting API (WhatsApp) is an **assisted publish**: the iOS
  app hands the post to the platform's app through the share sheet and the user taps Send.
- **Platform limits come from the platform's docs, dated.** Every `Rules()` says when it was
  checked. Never guess a limit: Discord's was remembered as 10 MiB and is 20 MiB per file.
- **No secrets in the repo.** Every secret (database URL, encryption key, storage keys, a
  platform's OAuth client secret, analytics key) comes from the environment: `.env` locally
  (gitignored), sealed secrets in the cluster. `.env.example` holds only blanks and the
  docker-compose dev values. Clients hold none: the web app and the iOS app know the API's
  base URL and nothing else, so every call to a platform goes through the server. CI runs a
  secret scan over the history.
- **Credentials are sealed and never leave the server.** Stored encrypted with AES-256-GCM, bound
  to their account row, never logged, never in an API response, never in a URL.
- **Media bytes never pass through the API.** Clients upload straight to storage with a
  presigned URL; the server checks size and type against storage before a file can be used.
- **Posts never ping people by accident.** Discord posts go out with mentions disabled.
- **A user sees only their own data.** Every query is scoped by `user_id`; tests check it.

## Stack

| Layer | Choice |
| --- | --- |
| API | Go 1.27, `net/http` + chi, generated from `api/openapi.yaml` by oapi-codegen (strict server) |
| Database | PostgreSQL; schema in `migrations/` (goose, embedded); queries in `internal/db/queries.sql` (sqlc, pgx/v5) |
| Jobs | River (Postgres-backed): one job per post target, inserted in the post's own transaction |
| Media | S3 API through minio-go: Cloudflare R2 in production, RustFS locally |
| Credentials | `internal/secretbox`: AES-256-GCM, key from `TOKEN_ENCRYPTION_KEY` |
| Web | `web/`: Vite + React 19 + TypeScript 5.9, `openapi-fetch` with types generated from the spec, embedded in the Go binary |
| iOS | `ios/`: SwiftUI, iOS 18; `PostithekaKit` holds the client generated from the same spec by swift-openapi-generator |
| Deploy | Not set up. Will be the `maat` cluster through `~/Work/production/platform/infra` |

## Repo layout

```text
postitheka/
  api/openapi.yaml          # the contract; Go server and TS types are generated from it
  cmd/server/               # entrypoint: no args serves; migrate | user create EMAIL
  internal/
    config/                 # environment -> Config, fails loudly on anything missing
    database/               # pool, migrations (goose + River), testdb for tests
    db/                     # sqlc output; edit queries.sql, never the .go files
    httpapi/                # api.gen.go (generated) + server.go (wire <-> service)
    jobs/                   # River workers: publish one target, sweep uploads, expire handoffs
    media/                  # object storage, presigned URLs, filenames
    publisher/              # Publisher interface, Rules, Validate, error classes
      discord/              # one package per platform
      linkedin/
      threads/
    secretbox/              # credential encryption
    analytics/              # product events (PostHog); empty key sends nothing
    service/                # application behaviour: accounts, uploads, posts, tokens
  migrations/               # goose SQL; never edit one that has been applied
  web/                      # the composer; web/embed.go serves web/dist
  ios/                      # Postitheka app (views) + PostithekaKit (generated client, model, tests)
  brand/                    # logo, icons, mascot; brand/README.md says which file goes where
  docs/platform-research.md # what each platform allows (dated; re-check before building)
  CLAUDE.md  ARCHITECTURE.md  BACKLOG.md  README.md
```

## Commands

```sh
task db:up                          # Postgres :5452, RustFS :9020/:9021
task dev                            # API + workers on :8095, migrates on boot
task web:dev                        # composer on :5173, proxies /v1 to :8095
task user:create -- you@example.com # prints an API token (stand-in for sign-in)
task test                           # Go tests; DB tests skip without DATABASE_URL
task check                          # the gate: codegen current, gofumpt, vet, typecheck, tests, build
task secrets                        # gitleaks over the history, as CI runs it
task generate                       # api:gen (Go + TS + Swift) and sqlc
task ios:check                      # the iOS gate: Swift client current, kit tests, app builds
task build                          # web build, then one binary with the web app inside
```

## Coding conventions

- **Spec first.** An API change starts in `api/openapi.yaml`, then `task api:gen`, then the code
  the compiler points at. Generated files are committed; `task check` fails if they are stale.
- **SQL lives in `internal/db/queries.sql`.** Run `task sqlc`. No SQL strings in handlers or
  services; a transaction uses the generated queries through `Queries.WithTx`, as `CreatePost`
  does.
- **Handlers translate, services decide.** `internal/httpapi` maps wire types to service calls
  and service errors to status codes. Rules live in `internal/service` and `internal/publisher`.
- **Platform knowledge lives in its publisher package.** Nothing outside
  `internal/publisher/<platform>` names a platform's API, limits or quirks.
- **Classify publisher errors.** `publisher.Permanent` for anything a retry cannot fix,
  `publisher.RetryAfter` when the platform says how long to wait, anything else is retried.
- **Migrations are append-only.** Fix a schema mistake with a new migration, never by editing one
  that has run anywhere, including a developer's machine.
- **Tests use real Postgres** (`testdb.New`, one database per test) and **stub every platform**
  with `httptest` plus a redirecting transport. Never a real token, webhook or API key in a test.
- **Comments say why**, not what: the constraint, the trap, the alternative ruled out. Match the
  density already in the file.
- Go is formatted with gofumpt. Configuration comes from the environment through
  `internal/config`; nothing reads `os.Getenv` elsewhere.
- **Web:** no CSS framework. Tokens on `:root` in `web/src/styles.css`, with dark mode. API calls
  go through `web/src/api.ts`; client-side checks mirror `publisher.Validate` and never replace it.

## Domain language

| Term | Meaning |
| --- | --- |
| **Post** | What the user wrote: text, ordered media, a time to publish. Owned by one user. |
| **Account** | A connected destination: one Discord webhook, one LinkedIn member, one Facebook Page. Holds sealed credentials; OAuth accounts carry the platform's id (`remote_id`), and reconnecting renews the same account. |
| **Target** | One post going to one account. Has its own status: `pending`, `publishing`, `published`, `failed`; an assisted target goes `handing_off` → `handed_off` or `failed`. |
| **Publisher** | The code that knows one platform: its `Rules`, how to connect, how to publish. |
| **Rules** | A platform's limits: text length, media count, size and types. |
| **Media** | An uploaded file. Reserved first, uploaded by the client, verified against storage on first use. |
| **Direct publish** | The server posts through the platform's API. |
| **Assisted publish** | The iOS app hands the post to the platform's app; the user sends it. WhatsApp only, for now. |

In the UI, destinations are **stamps** and a published target gets a **postmark**. Keep the
vocabulary: "Publish" produces "Published"; "Not sent" means failed.

## Traps already hit

- **MinIO images are gone** (Docker Hub repo deleted, quay.io needs auth). Local S3 is RustFS.
- **Browser uploads need bucket CORS** (`PUT`, `Content-Type`, the app's origin). Locally the
  server sets it on boot; for R2 it is infrastructure. A missing rule fails silently at preflight.
- **oapi-codegen's chi server has no per-operation security hook.** Auth wraps every spec route,
  so anything public (`/healthz`) is registered outside the spec in `cmd/server`.
- **River:** `FetchPollInterval` must be at least `FetchCooldown` (100 ms) or the client refuses
  to start.
- **`ON DELETE RESTRICT` is checked immediately**, before sibling cascades run. It broke user
  deletion once; `TestDeletingAUserRemovesEverything` guards it.
- **`openapi-typescript` needs TypeScript 5's JS API.** TypeScript 7 is the native port without
  it; `web/` is pinned to 5.9.
- **`web/dist` is embedded at compile time.** After `npm run build`, rebuild the Go binary or the
  server keeps serving the old app.
- **A Taskfile command containing `: ` must be quoted.** Unquoted, YAML reads it as a mapping
  and Task refuses the whole file. `task check` was broken this way, unnoticed, until CI ran it.
- **Discord forum and media channels** need `thread_name`; without it the webhook answers 400.
- **LinkedIn post text is markup** ("little" format): `| { } @ [ ] ( ) < > # \ * _ ~` must be
  backslash-escaped or the post fails or renders as something else. `escapeText` does it.
- **OAuth redirect URLs must match exactly.** Each platform's developer app lists
  `PUBLIC_URL/oauth/{platform}/callback`; a different host, port or trailing slash is refused.
- **Meta's developer docs render client-side.** A plain fetch gets an empty shell; render the
  page in headless Chrome (CDP, `document.body.innerText`) to read it.
- **Transport errors carry the request URL.** `net/http` writes the full URL, query included,
  into a `*url.Error`. Threads takes tokens and the app secret in query strings, so its `call`
  strips the URL; `TestErrorsCarryNoSecrets` guards it. Do the same for any platform like it.
- **Platform research agents can be wrong.** Limits that came back unconfirmed from research
  (LinkedIn's text length, image size) were checked against the platform's own pages before
  going into `Rules()`.

## Agent working style

- Take the next open item in `BACKLOG.md` whose `Needs` are met. Update its status row when it
  changes. Never renumber IDs. Do not start an item marked **OPEN** or **Owner**: those wait for
  a human decision or action.
- Change the smallest surface that delivers the item. For a feature touching every layer, go in
  this order so the API stays the source of behaviour: spec, migration, queries, service,
  handler, web, then iOS.
- Do not add a platform as an empty stub or an interface "for later". A platform exists when
  its publisher passes tests against a stub of the real API, with dated `Rules`.
- Verify before saying done: `task check` passes; a UI change is looked at in a browser, at
  desktop and phone width; a flow that crosses services is run end to end locally.
- Ask the owner before anything outward-facing or paid: deploying, merging to the infra repo,
  signing up for or paying for a platform API tier, using a real account or webhook, submitting
  for app review.
- Keep this file, `ARCHITECTURE.md` and `BACKLOG.md` true. A new platform, table, route or trap
  updates them in the same change.
