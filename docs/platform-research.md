# Platform research

What each platform allows a third-party app to post, **re-checked against the official developer
docs on 2026-10-23** (the first pass was 2026-09-27, before any code). Platforms change these
rules and prices often: re-check a platform's docs before building against it, and record what
you checked, and when, in its `Rules()` and in `BACKLOG.md`. "Unconfirmed" means the current docs
do not say; never put an unconfirmed number in `Rules()` without checking it.

## Summary (2026-10-23)

| Platform | Posts to | Connect | Before review | Media reaches the platform by | Blocker |
|---|---|---|---|---|---|
| Discord | Server channels | Webhook URL | No review exists | Upload (multipart) | None |
| LinkedIn | The member's own profile | OAuth 2.0, `w_member_social` | Self-serve ("Share on LinkedIn") | Upload (Images, Videos API) | None. Company Pages need the Community Management API (application) |
| Threads | The user's profile | OAuth (threads.com), `threads_basic` + `threads_content_publish` | Threads Testers only | Public URL Meta fetches | App Review for strangers |
| Instagram | Business and Creator accounts | Instagram Login, `instagram_business_basic` + `instagram_business_content_publish` | Role holders only | Public URL (images); upload (video, resumable) | App Review and Business Verification for strangers |
| X | The user's timeline | OAuth 2.0 PKCE | n/a | Upload (v2 chunked, `media.write`) | Pay-per-use credits |
| TikTok | Drafts (inbox), or direct posts | Login Kit OAuth v2 | Direct posts private-only, 5 users a day | Upload (chunked); photos by URL from a verified domain | Audit for public posts |
| Reddit | Subreddits | OAuth 2.0, `submit identity` | Nothing: every client needs approval | Text and links only (no documented media upload) | Approval; commercial agreement |
| Facebook | Pages only | Meta OAuth | Role holders | Public URL or upload | App Review and Business Verification |
| WhatsApp | Nothing: no posting API | — | — | Share sheet on the phone | Assisted publish only |

## Per platform

### LinkedIn
Sources: learn.microsoft.com/linkedin — `shared/authentication/authorization-code-flow`,
`consumer/integrations/self-serve/share-on-linkedin`, `marketing/community-management/shares/`
(`posts-api`, `images-api`, `videos-api`, `multiimage-post-api`), `shared/api-guide/concepts/`
(`rate-limits`, `error-handling`), `marketing/versioning`.

- Authorize `https://www.linkedin.com/oauth/v2/authorization`, token
  `https://www.linkedin.com/oauth/v2/accessToken`. Server apps use the code flow with the client
  secret; PKCE is for native apps on request.
- Access tokens last 60 days. Refresh tokens only "for a limited set of partners"; everyone else
  re-runs the authorize redirect, which skips consent while the member is logged in.
- Every call: `Linkedin-Version: YYYYMM` and `X-Restli-Protocol-Version: 2.0.0`. A version is
  supported at least a year; 202511 sunsets 2026-11-16. A deprecated one answers 426.
- `POST /rest/posts` with `author: urn:li:person:{id}`, `commentary`, `visibility`,
  `distribution`, `lifecycleState: PUBLISHED`. The new post's URN is in `x-restli-id`; it is
  viewed at `https://www.linkedin.com/feed/update/{urn}/`.
- Images: `POST /rest/images?action=initializeUpload` → `uploadUrl` + `urn:li:image:…`, upload,
  then `content.media.id`. JPG, GIF, PNG under 36,152,320 pixels. Multi-image: 2 to 20.
- Video: `initializeUpload` with the size → 4 MB part instructions; PUT each, keep the ETags,
  `finalizeUpload`; post only once processing is done (`MEDIA_ASSET_WAITING_UPLOAD` otherwise).
  MP4, 3 s to 30 min, 75 KB to 500 MB (the page also says 5 GB; it sits under an ads heading).
- Links are not scraped: an article needs `source`, `title`, and optionally a thumbnail.
- Limits: 150 requests per member and 100,000 per app a day, reset midnight UTC, 429 when hit
  (no documented `Retry-After`). 401 for an expired or revoked token.
- Unconfirmed: the `commentary` length (3,000 is folklore; the docs only name
  `FIELD_LENGTH_TOO_LONG`), the image byte limit.

### Threads
Sources: developers.facebook.com/docs/threads — `posts`, `get-started`,
`get-started/get-access-tokens-and-permissions`, `get-started/long-lived-tokens`, `overview`,
`troubleshooting`.

- Authorize `https://threads.com/oauth/authorize` (threads.com, no longer threads.net), token
  `POST https://graph.threads.com/oauth/access_token`. Codes last an hour, single use. Uses the
  Threads app id and secret, not the main Meta app's.
- Long-lived token (`th_exchange_token`) lasts 60 days; refresh with `th_refresh_token` once it is
  24 hours old and before it expires. No refresh token.
- Publish: `POST /{user}/threads` (container) → poll `status` (`IN_PROGRESS`, `FINISHED`, `ERROR`,
  `EXPIRED`, `PUBLISHED`; once a minute, up to 5 minutes) → `POST /{user}/threads_publish`.
  Containers expire after 24 hours. Permalink: `GET /{media}?fields=permalink`.
- 500 characters, emoji counted in UTF-8 bytes. At most 5 links; `link_attachment` only on
  text posts. Carousel 2 to 20. Images JPEG/PNG, 8 MB, width 320–1440, up to 10:1. Video MOV/MP4,
  H.264/HEVC, 5 minutes, 1 GB. 250 posts per 24 hours (`threads_publishing_limit`).
- Re-checked by rendering the pages, 2026-10-23: all of the above, plus no PKCE, `user_id` as a
  JSON number, emoji counted by UTF-8 bytes "for the post character limit", links counted as
  distinct URLs (more than 5 fails container creation with `THREADS_API__LINK_LIMIT_EXCEEDED`
  since 2025-12-22), container `ERROR` reasons (`FAILED_DOWNLOADING_VIDEO`, `INVALID_DURATION`, …)
  in `error_message`, and `PUBLISHED` as the container status once live.
- Unconfirmed: whether Threads needs Business Verification; its error codes (the publisher uses
  the Graph API's); whether an http redirect URI is accepted for local testing.

### Instagram
Sources: developers.facebook.com/docs/instagram-platform — `overview`,
`instagram-api-with-instagram-login` (and `/business-login`), `content-publishing`, reference
`ig-user/media`, `instagram-media`, error codes.

- Instagram API with Instagram Login: no Facebook Page needed, Business and Creator accounts.
  Authorize `https://www.instagram.com/oauth/authorize`, code → short-lived token (1 hour) at
  `https://api.instagram.com/oauth/access_token`, → long-lived (60 days, `ig_exchange_token`) at
  `graph.instagram.com`; refresh with `ig_refresh_token` once 24 hours old.
- Standard Access is for people with a role on the app; serving others needs Advanced Access,
  App Review and Business Verification.
- Same container model: `POST /{ig}/media` → poll `status_code` → `POST /{ig}/media_publish`.
  Images by public URL, **JPEG only**, 8 MB, 4:5 to 1.91:1. Reels MOV/MP4 3 s–15 min, 300 MB, can
  be uploaded (`upload_type=resumable`, rupload.facebook.com). Carousel up to 20, caption on the
  parent, cropped to the first item's ratio. Caption 2,200 characters, 30 hashtags, 20 tags.
- 100 API posts per 24 hours, but the same page says 50 for carousels: read
  `content_publishing_limit`. 400 containers a day. Rate limit error 9 / 2207042.

### X
Sources: docs.x.com — `fundamentals/authentication/oauth-2-0/authorization-code`,
`x-api/getting-started/pricing`, `x-api/posts/create-post`, `x-api/media/quickstart/media-upload-chunked`,
`fundamentals/counting-characters`, `x-api/fundamentals/rate-limits`.

- OAuth 2.0 with PKCE: authorize `https://x.com/i/oauth2/authorize`, token
  `https://api.x.com/2/oauth2/token`. The code expires 30 seconds after issue. Access tokens last
  2 hours; `offline.access` adds a refresh token. Scopes `tweet.read tweet.write users.read
  media.write offline.access`.
- **Pricing is pay-per-use** (no subscription): $0.015 per post, **$0.20 per post with a URL**.
  Free/Basic/Pro are not on the page any more.
- `POST /2/tweets`, up to 4 media ids. Media: `/2/media/upload/initialize`, `/{id}/append`
  (segments ≤ 5 MB), `/{id}/finalize`, poll status. OAuth 2.0 user tokens work for upload.
- 280 weighted characters: URLs 23, CJK and emoji 2, NFC normalised (twitter-text config).
  Images 5 MB, GIF 15 MB, video 20 minutes / 8 GB.
- 100 posts per 15 minutes per user, 10,000 a day per app; `x-rate-limit-reset` is epoch seconds.
- Unconfirmed: the v2 duplicate-post error, the `x.com/i/status/{id}` URL (common, not in docs).

### TikTok
Sources: developers.tiktok.com/doc — `login-kit-web`, `oauth-user-access-token-management`,
`content-posting-api-get-started`, `content-posting-api-media-transfer-guide`,
`content-posting-api-reference-photo-post`, `content-sharing-guidelines`,
`tiktok-api-v2-error-handling` (pages updated August 2026).

- Authorize `https://www.tiktok.com/v2/auth/authorize/` with `client_key` (not `client_id`),
  token and refresh `POST https://open.tiktokapis.com/v2/oauth/token/`. PKCE only for mobile and
  desktop apps. Access 24 hours, refresh token 365 days.
- `video.upload`: drafts in the user's inbox, finished in the app; at most 5 pending a day.
  `video.publish`: direct post, after querying creator info. Unaudited: `SELF_ONLY`, private
  accounts, 5 users a day. Direct posting has required UI (nickname, privacy choice with no
  default, interaction toggles off, commercial-content disclosure).
- Upload: chunks of 5 to 64 MB (last up to 128 MB), in order; or pull from a URL on a domain
  verified with TikTok. Photos are URL-only. Video MP4/WebM/MOV, up to 4 GB, 10 minutes (the
  creator's own limit is in creator info). Images WebP/JPEG, 1080p, 20 MB. Caption 2,200 UTF-16
  units. 6 init requests a minute per token; about 15 direct posts a day per creator.
- Unconfirmed: whether inbox upload needs the audit, the photo count, a public post URL.

### Reddit
Sources: support.reddithelp.com (Responsible Builder Policy, Data API Wiki),
redditinc.com/policies/data-api-terms, reddit.com/dev/api, github.com/reddit-archive/reddit/wiki/OAuth2.

- "You must request access and get explicit approval before accessing any Reddit data" (since
  November 2025). Commercial use needs a separate written agreement; prices are not published.
- OAuth 2.0 code flow, `duration=permanent` for a refresh token, `scope=submit identity`. Access
  tokens last an hour. A descriptive User-Agent is mandatory.
- `POST /api/submit`: `sr`, `title` (300 characters), `kind` `self` or `link`, flair fields.
  Native image, video and gallery upload are **not in the official docs**, so under "official
  APIs only" Reddit is text and links. `GET /api/v1/{sr}/post_requirements` gives a
  subreddit's flair, title and domain rules. 100 queries a minute per client.

### Discord
Webhooks: 2,000 characters, 10 files, 20 MiB per file (checked 2026-10-22). Forum and media
channels need `thread_name`.

### WhatsApp
No posting API: the Business API only messages people who opted in, paid per message. The iOS
app hands the post to WhatsApp through the share sheet and the user taps Send (built).

## Why Go for the backend

Compared with Hummingbird (Swift), considered because the iOS app is Swift:

| | Go | Hummingbird |
|---|---|---|
| OAuth | `golang.org/x/oauth2`, mature | Mostly written by hand |
| Platform libraries | Bluesky's official `indigo`, Mastodon and X clients | Very few |
| Scheduled jobs | `river` (Postgres-backed), `asynq` | Possible, fewer options |
| Deployment | One small static binary | Docker plus a Linux Swift toolchain, slower builds |
| Sharing models with iOS | No; the OpenAPI spec generates both sides instead | Same `Codable` models |

## Suggested order

The current order is in `BACKLOG.md` (P2). Unchanged from the first pass: start Meta Business
Verification and App Review early, since approval takes weeks and needs a live privacy policy, a
demo video and a working build. Bluesky and Mastodon remain free, review-free options not yet on
the list.
