# Postitheka

Write once, post to many platforms. One Go process serves the API, the web
composer and the publish workers; clients (web, iOS) are generated from
`api/openapi.yaml`.

Working rules are in `CLAUDE.md`, the design in `ARCHITECTURE.md`, the work list in
`BACKLOG.md`, and what each platform allows in `docs/platform-research.md`.

## Run it

```sh
task db:up                               # Postgres :5452, RustFS (S3) :9020
cp .env.example .env                     # then set TOKEN_ENCRYPTION_KEY=$(openssl rand -base64 32)
task dev                                 # API on :8095, migrates on boot
task user:create -- you@example.com      # prints an API token
task web:dev                             # composer on :5173, hot reload
```

Open localhost:5173, paste the token, connect a Discord channel under
Accounts, and post. `task build` produces one binary with the composer inside,
served at :8095.

`task test` runs everything; database tests need the stack up and skip without
`DATABASE_URL`. `task check` is the full gate.

## How a post moves

1. `POST /v1/media` reserves a media id and returns a presigned PUT URL. The
   client uploads straight to storage; the API never touches the bytes.
2. `POST /v1/posts` with text, media ids and account ids. Every target's rules
   are checked first (each upload is confirmed in storage with the size and type
   it was declared with); if any target would reject the post, the response is
   a 422 listing every issue, and nothing is created.
   With an `Idempotency-Key` header, a retry returns the post the first
   request created rather than making a second one.
3. The post, one `post_targets` row per account, and one River job per target
   commit in a single transaction.
4. Each job publishes to one platform and records the outcome on its target:
   `published` with a link, back to `pending` with the error for a retry, or
   `failed`. Rate limits wait exactly as long as the platform says.
5. Clients poll `GET /v1/posts/{id}` for per-target status.

Delivery is at least once. If a process dies with a request in flight, the
next attempt marks the target failed ("check the platform before posting
again") rather than risk a duplicate post.

## Layout

| Path | What |
|---|---|
| `api/openapi.yaml` | The contract. `task api:gen` regenerates `internal/httpapi/api.gen.go` |
| `cmd/server` | Entrypoint: `server`, `server migrate`, `server user create EMAIL` |
| `internal/publisher` | `Publisher` interface, `Rules`, validation, error classes |
| `internal/publisher/discord` | Discord webhooks: the first platform |
| `internal/service` | Application behaviour: accounts, uploads, posts, tokens |
| `internal/jobs` | The River worker that publishes one target |
| `internal/secretbox` | AES-256-GCM for credentials at rest, bound to the row id |
| `internal/media` | S3-compatible storage (R2 in production) |
| `internal/db` | sqlc output from `internal/db/queries.sql` (`task sqlc`) |
| `migrations` | goose SQL, embedded; River migrates its own tables |
| `web/` | The composer: Vite + React + TypeScript, embedded by `web/embed.go`. Types in `src/schema.d.ts` come from the spec |

## Adding a platform

1. A package under `internal/publisher/<name>` implementing `Publisher`, with
   its `Rules` taken from the platform's current docs, with the date checked.
2. A connect flow: a service method that produces a `publisher.Connection` and
   calls `storeAccount`, plus its route in the spec (OAuth platforms need an
   authorize and a callback route).
3. Register it in `cmd/server/main.go`. `GET /v1/platforms` picks it up.

Tests stub the platform's HTTP API; see `discord_test.go`. Never a real token.

## Not done yet

- **Sign in with Apple.** Tokens come from `server user create` until then.
- **X, TikTok, Instagram, Facebook.** Start Meta Business Verification and the
  TikTok audit early; both take weeks.
- **WhatsApp** is a share-sheet handoff in the iOS app. It has no API for posting.
- **Discord forum channels** need `thread_name`; they fail permanently today.
- **iOS app.** The web composer covers the same flow; iOS adds the share
  sheet, which is also how WhatsApp gets posts.
- **Deployment.** Nothing is in `platform/infra` yet. When it is: R2 bucket,
  sealed secrets for `TOKEN_ENCRYPTION_KEY` and the S3 keys (sealed for the
  right namespace), and a PreSync hook running `server migrate`. The R2 bucket
  needs a CORS rule allowing `PUT` with `Content-Type` from the app's origin,
  or browser uploads fail at the preflight; locally the server sets this up.
# postitheka
