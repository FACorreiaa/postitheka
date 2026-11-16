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
- **Reddit is assisted publish too** (owner, 2026-11-15): its API needs approval, commercial use
  needs an agreement, and its documented part takes no media. The API route is dropped.
- **Local S3 is RustFS**, because MinIO no longer publishes images.
- **Sign-in is a CLI-minted token until AUTH-001.** That is acceptable for the owner and not for
  strangers, so AUTH-001 is on the path to the P1 gate.
- **Sign in with Apple needs no key** (owner, 2026-11-16: "Khepri way, no key"). The server
  checks Apple's identity token against Apple's public keys; a .p8 is only for exchanging codes
  and revoking tokens. One is needed when in-app account deletion is built, since Apple asks
  apps to revoke the user's token then.
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

## Decisions recorded 2026-11-05

- **Public product** (owner). Postitheka promotes the owner's apps and is also offered to strangers,
  so AUTH-001, OPS-002 and META-001 stay on the path, and the TikTok audit (TT-002) is worth applying
  for.
- **X credits are bought once the app is on TestFlight and ready to test** (owner). X posts go out
  without URLs by default ($0.015 against $0.20). Build X-001 against stubs before then.
- **Free and premium tiers** (owner). Free users write one text for all platforms; premium users can
  write a different text per platform.
- **OPEN:** whether X is premium too, and how a user pays. X posts bill the owner's developer account
  for every user, so public X access needs an answer here.

## Decisions recorded 2026-11-13

- **A 3D mascot is tried on beta only** (owner). The supplied kit is a different robot from the
  approved postman; the owner chose to use it recoloured to the brand palette, on both sign-in
  screens, floating, swaying, rising in and turning toward the pointer, with the built model
  git-ignored and a CDN decided when deploy lands. BRD-001. `brand/README.md` records the
  exception.

## Decisions recorded 2026-11-16

- **Each person connects their own accounts; the platform apps are Postitheka's** (owner
  question, recorded as the design already built). The LinkedIn, Meta, X and TikTok developer
  apps belong to Postitheka, and their client ids and secrets are server configuration. A person
  presses a stamp, signs in on that platform, and Postitheka keeps that account's token sealed
  (OAUTH-001); Discord takes the person's own webhook. Nobody brings an API key. A post is text,
  images and/or video, sent to the accounts the person chose. What still stops a stranger: no
  sign-up (AUTH-001, IOS-002), Meta apps in development mode until review (META-001, needs
  PRV-001), TikTok private-only until its audit (TT-002). X bills the owner's developer account
  for every user's posts, which needs a decision before X opens to strangers (OPEN).
- **Spaces and a first comment are part of the MVP** (owner): BRAND-002 and POST-002 are on
  P2's gate. Flagged that neither brings a stranger in while the strangers count is 0; the owner
  decided they belong in the MVP anyway.

## Waiting on the owner

Nothing below blocks building; it blocks shipping. Each is an account, a decision or a key that
only the owner can create. Build against stubs and configuration until they arrive.

`docs/platform-rollout.md` (2026-11-05) orders this work for the owner's own use, promoting their
apps, and lists the decisions still open. Its recommendations are not decisions.

| What | Unblocks | Notes |
| --- | --- | --- |
| R2 bucket + API token | REL-003, REL-004 | Agent writes the CORS rule |
| Public hostname | REL-004, AUTH-001 (web Services ID) | Deferred 2026-10-22 |
| Sign in with Apple web Services ID | AUTH-001 on the web | Identifier (e.g. `com.fernandocorreia.postitheka.web`), domain `postitheka.facorreia.com`, return URL `https://postitheka.facorreia.com/`; its identifier goes in `APPLE_WEB_CLIENT_ID`. Not a secret. Until set, the web shows no Apple button |
| Sign in with Apple key (.p8) | Token revocation on account deletion (later) | Not needed for sign-in (2026-11-16). One-time download, straight into a sealed secret |
| PostHog project key | OPS-001 seen working | Sealed secret `POSTHOG_API_KEY` |
| LinkedIn developer app with "Share on LinkedIn" and "Sign In with LinkedIn using OpenID Connect" | LI-001 used for real | **To do (owner, 2026-11-16).** Self-serve. `LINKEDIN_CLIENT_ID` / `LINKEDIN_CLIENT_SECRET` in `.env`; redirect `http://localhost:8095/oauth/linkedin/callback` locally, `PUBLIC_URL/oauth/linkedin/callback` in production |
| X developer app + credits | X-001 used for real | **To do (owner, 2026-11-16).** Pay-per-use: $0.015 a post, $0.20 with a URL (2026-10-23) |
| TikTok developer app | TT-001 used for real, TT-002 | **To do (owner, 2026-11-16).** Photos also need a domain verified with TikTok |
| Meta app with the Threads API use case, owner as Threads Tester | THR-001 used before review | **To do (owner, 2026-11-16).** `THREADS_APP_ID` / `THREADS_APP_SECRET` (the Threads ones, not the main app's) in `.env`; redirect `PUBLIC_URL/oauth/threads/callback`. Instagram is the same app with the Instagram use case: `INSTAGRAM_APP_ID` / `INSTAGRAM_APP_SECRET` from "API setup with Instagram login", redirect `PUBLIC_URL/oauth/instagram/callback`, and a professional account with a role on the app. Facebook Pages use the Meta app's own id and secret: `FACEBOOK_APP_ID` / `FACEBOOK_APP_SECRET`, redirect `PUBLIC_URL/oauth/facebook/callback` under Facebook Login. Meta may refuse a plain-http redirect (not checked); then `PUBLIC_URL` must be an https tunnel to the local server |
| Meta app, Business Verification, App Review | THR-001, IG-001, FB-001 for strangers | **To do (owner, 2026-11-16).** Weeks; needs PRV-001 live. Not needed for accounts with a role on the app (Standard Access, checked 2026-11-05) |
| Approve privacy policy and terms text | PRV-001 | Done | Live 2026-11-16 at https://postitheka.facorreia.com/privacy/ and /terms/ (PR #34, promote LuminaVaultInfra #663, image `fe53fde`), linked from web and iOS sign-in; deletion instructions at `/privacy/#delete`. Operator Fernando Correia, contact fernando@facorreia.com, Portuguese law (owner) | |

## Status (2026-10-23)

| Phase | Done | Gate | What blocks it |
| --- | --- | --- | --- |
| **P0 · Foundations** | 6/6 | **Met** | Nothing |
| **P1 · First stranger** | 4/12 (2 built, unverified) | Not met | Not deployed; no real sign-in |
| **P2 · Platforms** | 0/17 (13 built, unverified; RDT-001 dropped; BRAND-002 and POST-002 open) | Not met | Needs P1 deployed; X needs credits; Meta and TikTok need review; Spaces and first comment not built |
| **P3 · iOS** | 1/6 (2 built, unverified) | Not met | IOS-002 needs AUTH-001; nothing tested on a device |

Strangers who have published a post: **0**.

### Every item

| ID | Status | Evidence | Caveat |
| --- | --- | --- | --- |
| FND-001 | Done | `internal/`, `cmd/server`; `task check` green | |
| FND-002 | Done | `internal/publisher/discord`; `discord_test.go`. First real post 2026-11-16 from production: text, one PNG, a forum post title set, published first try with a message URL | |
| FND-003 | Done | `internal/service/service_test.go` (`TestPostIsPublishedEndToEnd`, `TestWorkerOutcomes`) | |
| FND-004 | Done | `web/`; checked in Chrome at 1400 and 390 px | |
| FND-005 | Done | `migrations/20261022170000_post_media_cascade.sql`; `TestDeletingAUserRemovesEverything` | |
| FND-006 | Done | `Dockerfile`; image built, 50 MB, web app embedded | Never run in the cluster |
| REL-001 | Done | PR #1: CI green, `internal/service` tests ran (3.6 s, none skipped), image built | |
| REL-002 | Done | Image workflow run 39870125984 on `main` (38a1c3a) after PR #1 merged, 2026-10-22 | Package visibility not checked |
| REL-003 | Done | R2 bucket `postitheka-production` (Western Europe, private) with the upload CORS rule; account token scoped to that bucket, sealed into `postitheka-env` (LuminaVaultInfra #651, #656), 2026-11-16 | |
| REL-004 | Done | Live at https://postitheka.facorreia.com since 2026-11-16: promote by hand (LuminaVaultInfra #654, image `990fdcb`), migrations applied, `/healthz` `/health/live` `/health/ready` 200, TLS from Let's Encrypt, R2 preflight 204 from the app origin and 403 from others. Promotes are manual by the owner's choice; `INFRA_TOKEN` is not set. The owner published a post with an image to a real Discord channel from production on 2026-11-16 | |
| AUTH-001 | Built, unverified | `internal/appleid` (token verified against Apple's keys: signature, iss, aud, exp, nonce), `Service.SignInWithApple`, `POST /auth/apple`, `migrations/20261116150000_users_apple_sub.sql`; `appleid_test.go` (stub key server), `TestSignInWithApple*`, `TestAppleSignInAnswers`; web button seen in a browser with a placeholder Services ID | Never signed in with a real Apple ID. Production needs `APPLE_BUNDLE_IDS` in the infra values; the web needs the Services ID (Owner) |
| API-001 | Done | `migrations/20261022190000_post_idempotency.sql`; `TestCreatePostIsIdempotent` (incl. 8 concurrent requests, one post) | Web retry path checked by typecheck only, not in a browser |
| MED-002 | Built, unverified | `Service.DeleteUser`, `media.Store.DeletePrefix`, `server user delete EMAIL` (`task user:delete`); `TestDeleteUserRemovesTheirUploadsFromStorage` (objects with and without a row, another user's untouched, storage down leaves the user in place, queued job posts nothing); live run against RustFS removed two objects and left another prefix alone | Not run against R2. No route or button yet: a signed-in user cannot delete their own account until AUTH-001. An upload URL issued before the deletion still works for its 30 minutes |
| MED-001 | Done | `internal/jobs/sweep.go`; `TestSweepDeletesOnlyOldUnattachedUploads` in CI; live run against RustFS removed row and object | |
| WEB-001 | Built, unverified | `POST /v1/targets/{id}/retry`, `migrations/20261113190000_target_retry.sql` (`interrupted`, `retries`), Send again in Sent with a may-be-live warning; `TestRetryTargetPublishesAgain` (real River client, first job completed), `TestRetryTargetRefusesWhatCannotBeSentAgain` (pending, published, assisted, disconnected, another user's) | Checked in a browser against a local server with seeded targets; never against a real platform |
| WEB-002 | Done | Dark only since 2026-11-05, so there is one theme to check: every screen looked at in a browser at 1280 and 390 px, no sideways scroll; palette contrast passes WCAG AA for text (lowest 6.5:1) | |
| PRV-001 | Drafted, waiting on the owner's merge | `web/public/privacy/`, `web/public/terms/` (static HTML, served at `/privacy/` and `/terms/`), linked from web and iOS sign-in; seen at desktop and 375 px | Operator Fernando Correia, contact fernando@facorreia.com, Portuguese law (owner, 2026-11-16). Done when live on production |
| OPS-001 | Built, unverified | `internal/analytics`; `TestProductEventsCarryNoPersonalData`, events asserted in `TestWorkerOutcomes`; empty key is a no-op (`TestNoAPIKeyMeansNothingIsSent`) | Needs production and a PostHog project key (Owner) to see an event arrive |
| OPS-002 | Built, unverified | `internal/ratelimit`, `internal/httpapi/limits.go`; `TestCreatingPostsPastTheLimitIsRefused`, `TestReservingUploadsPastTheLimitIsRefused`, `TestFailedAuthenticationIsLimitedByAddress`, `TestAddressLimitOnPublicRoutes`; every limit hit over HTTP against a local server | Not run behind the cluster's ingress: whether Traefik passes the real client address in `X-Forwarded-For` is unchecked, and if it does not, the per-address limits are shared by everyone |
| OAUTH-001 | Built, unverified | `internal/service/connect.go`, `internal/httpapi/oauth.go`, `migrations/20261023150000_oauth_connect.sql`; `TestOAuthConnect` (start, callback, replay, cancel, expiry, reconnect, two users); web Accounts checked headless at 1400 and 390 px, light and dark; iOS kit `ConnectTests` | Never run against a real platform; iOS connect screen built, not looked at on a simulator |
| X-002 | Built, unverified | `publisher.Refresher`, `PublishWorker.refresh` (row lock, re-check, seal, then publish); `TestTokenRefresh` (three concurrent posts refresh once and publish with the new token, revoked grant fails asking for reconnect, transient failure stays pending); mutation-checked; Threads implements it | Never run against a real platform |
| ACC-001 | Built, unverified | `migrations/20261114150000_account_needs_reconnect.sql`, `publisher.Reconnect`, `needs_reconnect` on Account; `TestRefusedCredentialsMarkTheAccount` (a 401 marks only that account, a rejected post does not, queued targets fail without a request, a new post is refused at the door), revoked refresh in `TestTokenRefresh`, reconnect clears it in `TestOAuthConnect`; web stamp and Accounts looked at in a browser at desktop and 375 px with seeded accounts; iOS kit test | Never against a real platform: which answers mean dead credentials is read from each platform's docs. Marks after the first refusal; nothing warns before a token expires. iOS views built, not looked at on a simulator. Web Reconnect button seen, not pressed (it leaves for the platform) |
| POST-001 | Built, unverified | `PATCH` and `DELETE /v1/posts/{id}`; `TestCancelPostRemovesAnUnsentPost`, `TestReschedulePostMovesAnUnsentPost` (the old job posts nothing, the new one posts once), `TestAPostThatStartedSendingCannotBeCancelledOrMoved`; web Sent checked in a browser at desktop and 375 px against a local server with seeded posts; iOS kit `APITests` | iOS swipe actions and the reschedule sheet built, not looked at on a simulator. Not run against a job firing at the same moment outside a test |
| POST-002 | Open | Research 2026-11-16 in `docs/platform-research.md` | LinkedIn and X each need one real test call |
| BRAND-001 | Built, unverified | `migrations/20261114120000_account_brand.sql`, `PATCH /v1/accounts/{id}`; `TestAccountBrand`; web `brands.test.ts`, kit `BrandsTests`; web looked at in a browser at desktop and 375 px against a local server | iOS built and kit-tested, not looked at in a simulator |
| BRAND-002 | Open | | Name and storage are OPEN (owner) |
| LI-001 | Built, unverified | `internal/publisher/linkedin`; `linkedin_test.go` (connect, text, escaping, one and several images, chunked video with processing, error classes, off-host upload URL); local server offers it with placeholder app values | Needs the owner's LinkedIn app to post for real; video limits partly from an ads section of the docs |
| DSC-001 | Built, unverified | `internal/publisher/discord`: a refusal with Discord's error 220001 is answered by sending the message again with `thread_name`; `TestPublishToAForumChannelNamesTheThread`, `TestPublishMediaToAForumChannelSendsTheFilesAgain`, `TestANormalChannelGetsNoThreadName`, `TestOtherRefusalsAreNotSentAgain`, `TestThreadName` | Never posted to a real forum channel: the error code and the 100-character name limit are from Discord's docs. A forum that requires a tag still fails (40067), with Discord's message |
| META-001 | Open | | Owner |
| THR-001 | Built, unverified | `internal/publisher/threads`; `threads_test.go` (connect with the long-lived exchange, text, image, video processing, carousel order, `ERROR`/`EXPIRED`/timeout, publish without a clear answer incl. a cancelled request, error classes, refresh, no secrets in errors); mutation-checked; `max_links` and `text_counting` in the spec, mirrored and tested in web and iOS | Needs the owner's Threads app and tester account to post for real |
| X-001 | Built, unverified | `internal/publisher/x`; `x_test.go` (connect with PKCE and Basic client auth, refresh keeps the rotated token, a revoked grant is permanent, text, photos, a video in segments with the processing wait, error classes, no secrets in errors); `TestCountXWeighted` and the same cases in `web/src/rules.test.ts` and the kit's `RulesTests.swift` | Never run against X: needs the owner's developer app and credits. The post link's form is not in X's reference |
| TT-001 | Built, unverified | `internal/publisher/tiktok`; `tiktok_test.go` (connect, an error in a 200 token answer, refresh keeps a new refresh token and keeps the old one when none comes, a revoked grant is permanent, a small video whole, a large one in chunks with the rest in the last, the wait while TikTok processes, a video TikTok refuses, a lost answer to the last chunk is never retried, error classes, no secrets in errors, rules); `TestADraftIsNotRecordedAsPublished` | Never run against TikTok: needs the owner's developer app approved for `video.upload`, and an https `PUBLIC_URL`. The draft row was not seen on a device. Not built: photos, the shorter-side and frame-rate limits |
| TT-002 | Open | | Owner |
| FB-001 | Built, unverified | `internal/publisher/facebook`, `publisher.MultiConnector`; `facebook_test.go` (connect: long-lived exchange, Pages across result pages, only Pages the person can post to; text, photo, several photos as one post, video; a publish without a clear answer is never retried; error classes; no secrets in errors; rules), `TestOAuthConnectStoresEveryAccountGranted` | Needs the owner's Meta app with Facebook Login and a Page to post for real. Never run against Facebook. Text length, photo count and video limits are our own caps: Meta's reference states none. META-001 for strangers |
| IG-001 | Built, unverified | `internal/publisher/instagram`; `instagram_test.go` (connect with the long-lived exchange and the account id from the profile, both answer shapes; image, reel, carousel order; `ERROR`/`EXPIRED`/timeout; publish without a clear answer incl. a cancelled request; error classes by code and subcode; refresh; no secrets in errors; rules) | Needs the owner's Meta app with the Instagram use case and a professional account with a role on it to post for real. Never run against Instagram. META-001 for strangers |
| RDT-001 | Dropped 2026-11-15 | | Reddit is posted to by hand, so no API access is needed |
| RDT-002 | Built, unverified | `publisher.Reddit` in `IsAssisted`; `TestAssistedHandoff` ("reddit and whatsapp each get a handoff"); kit `redditAndWhatsAppAreHandedOverOneAfterTheOther`; `ShareSheet.swift`, `AssistedStamp` | Never tried on a device: what Reddit's share extension accepts, its activity identifier (matched by the prefix `com.reddit.Reddit.`), and two share sheets in a row are all unseen |
| VID-001 | Built, unverified | Length, shape and width limits in `Rules`, checked against what the client read from the file: `video_rules_test.go`, `web/src/rules.test.ts`, the kit's `RulesTests.swift` (the same cases three times), `TestReserveUploadKeepsWhatTheClientReadFromAVideo`. No codec check. Not seen with a real video file in a browser or on a device | |
| OPT-001 | Built, unverified | `options` on `post_targets`, `Rules.Options`, `POST /v1/posts` `options` by account id; first use is Discord's forum post title. `options_test.go`, `TestTargetOptionsReachThePublisher`, `TestCreatePostRefusesOptionsItCannotUse`, `TestOptionsChangeTheIdempotencyDigestOnlyWhenSet`, `TestAForumPostTakesTheTitleTheAuthorGaveIt`, `web/src/rules.test.ts`, the kit's `RulesTests.swift` and `ComposerTests.swift`. Web field seen in a browser; iOS field built, not looked at on a device; never sent to a real forum channel | |
| IOS-001 | Done | `ios/`: Xcode project + `PostithekaKit`, client generated by swift-openapi-generator 1.14.0; `task ios:check` green locally and in CI (PR #2, run 39951558861, 2026-10-23) | |
| IOS-002 | Built, unverified | `SignInWithAppleButton` in `SignInView`, `Postitheka.entitlements`, kit `AppleSignIn` (`AppleSignInTests`); the pasted token stays as the operator's way in | Not tried on a device or TestFlight build |
| IOS-003 | Built, unverified | `ComposerView`, `Composer` (kit, tested); `PublishFlowTests` UI test published to a local server and saw the outcome in Sent | Photos path not exercised by the UI test |
| IOS-004, IOS-005 | Open | | |
| BRD-001 | Gate 1 passed, waiting on the device check | Branch `brd-001-mascot-3d`. `build_mascot3d.py` builds `mascot.glb` and `Mascot.usdz`; since 2026-11-15 from the tablet model with a drawn smile (1544 KB, 876 KB gzip, 20 draw calls, 129k triangles; `Mascot.usdz` 1640 KB, passes `usdchecker --arkit`), before that without the tablet (1047 KB, 616 KB gzip, 15 draw calls, 87k triangles; 1144 KB). Seen rendering in Chrome (model in ~1.4 s from the dev server, canvas swaps in, turns and floats) and in the iOS 26 simulator on a Beta build; flat mascot kept with a broken URL. Headless Chrome (software WebGL, dev and production builds, 2026-11-13): live in 0.9–1.3 s; one WebGL context left (the probe's is released); 15 draws a frame, none while the tab is hidden or the mascot is scrolled off, resumes after; reduced motion and Save-Data send no request and keep the flat mascot; no WebGL fetches the config only, never three. In the first ~5 runs the scene chunk's fetch was aborted (`net::ERR_ABORTED`, cause not found) and the flat mascot stayed, as designed; not seen again in 36 runs. Both drivers' tests, `task check` and `task ios:check` (Release proof with the model present) green | Entry budget raised 1 KiB to 80896 B (owner, 2026-11-13: Vite's preload wrapper). Gate 1 (the recolour) passed: owner, 2026-11-13. Waiting on the owner's device check: fps, memory, Low Power Mode and Reduce Motion on iOS, a real GPU |
| IOS-006 | Built, unverified | `POST /v1/targets/{id}/handoff`; `TestAssistedHandoff` (incl. expiry after an hour, `internal/jobs/handoffs.go`); kit `aWhatsAppHandoffOpensAfterPublishingAndIsReported`; UI test `testWhatsAppHandoffClosedWithoutSending` recorded the cancel as failed | WhatsApp is not on the simulator: the share sheet opened and was dismissed, but a real send was never tried. Needs a device |
| IOS-007 | Done | Release/Beta `API_BASE_URL`, shared scheme, `PrivacyInfo.xcprivacy`, `ios/tools/testflight.sh`; `task ios:check` green. Build 20261116.1046 uploaded by `task ios:testflight` on 2026-11-16 (PR #29); marked done by the owner the same day | |

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
(`AUTO_MIGRATE=false`), PreSync Job running `server migrate`, Service, probes on `/health/ready`
and `/health/live` (the shared chart's defaults; `/healthz` stays for people and scripts),
sealed secrets for `DATABASE_URL`, `TOKEN_ENCRYPTION_KEY` and the S3 keys, sealed for this
namespace. Ask the owner before merging to the infra repo: merging deploys.

**Done when:** the production `/healthz` answers 200, and the owner publishes a post from
production to a real Discord channel.

### AUTH-001 · Sign in with Apple · M
**Needs:** the hostname (REL-004) for the web Services ID and its return URL; **Owner** creates
the Services ID. Decided 2026-10-22: Sign in with Apple. The App ID is registered. No .p8 key
(2026-11-16): the identity token is checked against Apple's public keys.
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

### MED-002 · Deleting a user removes their uploads · S
Deleting a user cascades through the database (FND-005) but left their files in the bucket.
`DeleteUser` removes everything under the user's key prefix, then the user, then the prefix
again for anything that landed in between. By prefix rather than by media row, so a file with no
row goes too. The operator runs it as `server user delete EMAIL`; the in-app route comes with
sign-in (AUTH-001), which the App Store requires for account deletion.

**Done when:** deleting a user leaves none of their objects in the bucket and another user's
untouched (test, and a run against local storage).

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
TikTok and Facebook from production, for a user who is not the owner. The post is written in a
Space (BRAND-002) and carries a first comment, which lands under the post on every one of those
platforms that allows one (POST-002). Added to the gate by the owner 2026-11-16: part of the MVP.

What each platform allows, with sources and the date checked, is in
`docs/platform-research.md` (re-checked 2026-10-23). Every platform below except Discord connects
through OAuth, so OAUTH-001 comes first. Order after it: LinkedIn (self-serve, usable as soon as
the owner creates the app), Threads and Instagram (usable by app testers before review), X
(code now, pay-per-use credits to use), TikTok (private-only until audited). Reddit is by hand
(RDT-002).

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

### BRAND-001 · Brand label on accounts · S
Decided by the owner 2026-11-14: Postitheka promotes several of the owner's apps from one login.
An account carries an optional brand ("Aaru", "Norviq"), free text up to 40 characters, set in
Accounts. The composer shows a brand filter when any account has one, and publishes only to the
stamps on screen. A label, not a tenant: nothing is scoped by it, and "Teams and shared
accounts" stays out of scope.

**Done when:** a brand set on an account filters the composer on web and iOS, a stamp hidden by
the filter is not published to, and another user's account cannot be labelled (tests and a
browser check).

### BRAND-002 · Spaces: a brand's own posts and defaults · M
On P2's gate (MVP, owner 2026-11-16). **Needs:** BRAND-001 verified. Asked by the owner 2026-11-16: a Space for each thing promoted
(Khepri, LuminaVault, Norviq, AI), holding that subject's posts. BRAND-001 already labels
accounts and filters the composer by brand; this takes the label from accounts to posts:

- A post records the brand it was written under. Sent filters by brand the way the composer does.
- A brand can hold defaults applied when composing under it: the first comment (POST-002), and
  later hashtags or a link.
- Still a label, not a tenant: no sharing, no members ("Teams and shared accounts" stays deferred).

**OPEN:** the name. The owner calls these Spaces; the code, API and screens say brand. The rule
in `~/.claude/CLAUDE.md` holds renames until 10 strangers have used the current version, so the
screens keep "brand" until then unless the owner decides otherwise.

**OPEN:** whether a brand becomes its own record (a table with defaults) or stays free text on
accounts with defaults keyed by the text. Defaults need somewhere to live, which points to a
record; decide before the migration.

**Done when:** a post written under a brand shows only under that brand in Sent on web and iOS,
and a brand's default first comment is filled in when composing under it (tests and a browser
check at desktop and phone width).

### ACC-001 · Accounts that need reconnecting · S
**Needs:** OAUTH-001. A token that has expired and cannot be refreshed (LinkedIn after 60 days,
Threads and Instagram after 60 days unused, any revoked grant) fails every target until the user
reconnects. Mark the account, fail its targets with "reconnect LinkedIn", and show Reconnect on
the account and its stamp.

**Done when:** a 401 from a stubbed platform marks the account, the composer shows the stamp as
needing reconnection, and reconnecting clears it (tests and a browser check).

### POST-001 · Cancel or move a scheduled post · S
A post that has not started sending can be cancelled or given another time, from Sent on the web
and on iOS. "Not started" means every target is still `pending`; once one is sending, sent or
failed the answer is 409, because part of the post may already be public. Cancelling removes the
post. Moving it replaces its targets and queues them for the new time: a publish job is unique by
its target, so the job already queued is left to find its target gone and stop. Editing the text
or media of a scheduled post is not part of this.

**Done when:** a cancelled post's job posts nothing, a moved post is sent once at the new time,
and both are refused once sending has started (tests and a browser check).

### POST-002 · First comment: hashtags and links under the post · M
On P2's gate (MVP, owner 2026-11-16). **Needs:** research first. Asked by the owner 2026-11-16: the post carries the text, images and
video; a reply posted under it right after carries the hashtags and the link to the app, so the
post itself stays clean.

Research done 2026-11-16: "First comment" in `docs/platform-research.md`.
- **Discord:** yes, as a second webhook message (into the forum thread by `thread_id`). No new
  permission. Build first.
- **Threads, Instagram, Facebook:** yes, each with one new permission (`threads_manage_replies`,
  `instagram_business_manage_comments`, `pages_manage_engagement`) that joins META-001's App
  Review. Works now for accounts with a role on the app; reconnecting is needed to grant it.
- **LinkedIn:** the endpoint exists; whether `w_member_social` is enough is unconfirmed. One real
  call with the owner's account decides it before code. If it needs the Community Management API,
  that is a separate vetted app (OPEN).
- **X:** self-serve accounts may reply only when "summoned"; whether one's own post counts is
  unconfirmed. One real call decides it. A reply with a link likely costs $0.20.
- **TikTok:** no comment API, and drafts are not published. Not offered; the stamp says so.
- **WhatsApp, Reddit:** handed off by hand, so the comment goes with the post for the person to
  paste.

Then, per platform that allows it: a per-post "first comment" field (a per-target option,
OPT-001), posted after the post succeeds, as its own step with its own retry, so a failed
comment never re-sends the post. Sent shows the comment's own outcome.

**Done when:** a post with a first comment publishes to Discord and one OAuth platform from
production, and the comment appears under each; a platform that cannot take one says so on its
stamp before publishing.

### LI-001 · LinkedIn publisher · M
**Needs:** OAUTH-001. Personal profile through "Share on LinkedIn" (`w_member_social`,
self-serve, no review). Posts API (`/rest/posts`, pinned `Linkedin-Version`), images through the
Images API (one image, or 2 to 20 as a multi-image post), video through the Videos API (4 MB
parts, finalize, wait for processing). Tokens last 60 days with no refresh, so renewal is
reconnecting (ACC-001). Company Pages need the Community Management API: not in scope.

**Done when:** tests against a stub cover connect, text, one image, several images, video and
error classes; the owner posts to their own profile from a local server once the app exists.

### DSC-001 · Discord forum channels · S
Forum and media channels need `thread_name`; without it they fail permanently with a 400. The
channel type cannot be read when connecting: the webhook object does not carry it, and reading
the channel needs a bot. So the channel says so itself. Discord refuses the plain message with
error 220001, and the message is sent once more with a thread name taken from the post's first
line (100 characters at most; "1 file" or "3 files" when there is no text). The refusal saved
nothing, so this cannot post twice; it does upload a forum post's files twice.

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

**Built 2026-11-14, against a stub only.** What the docs said on that day, where it differs from
the above: video also goes by URL (the resumable upload is for apps on Facebook Login only); a
single video is a reel (`media_type=REELS`) and a video inside a carousel is `VIDEO`; the code
exchange is on `api.instagram.com` and everything else on `graph.instagram.com`; the id
containers are created under is `user_id` from `/me`, not the one the code exchange returns.
The daily limit is not read ahead of time: Instagram's refusal (subcode 2207042) is retried
hourly. Left for later: aspect ratio and duration checks (VID-001), the 30 hashtag and 20
@-tag limits and alt text. A refused token (error 190) marks the account for reconnecting
(`publisher.Reconnect`, since 2026-11-15).

### X-001 · X publisher · M
**Owner:** buys X API credits once the app is on TestFlight and ready to test (decided 2026-11-05).
X is pay-per-use, re-checked 2026-11-05: $0.015 per post, $0.20 for a post containing a URL, no
subscription, billed to the app's developer account. Publisher with OAuth 2.0 PKCE (the code
lives 30 seconds, so the callback exchanges at once), media through the v2 chunked upload
(`media.write`), and X's weighted character counting as `Rules.CountText`. The code is built
against stubs; only real use waits on the decision.

**Done when:** tests against a stub cover connect, token refresh, text, media and error classes,
and the owner posts to a real X account.

**Built 2026-11-15** against a stub; the real post waits on the owner's app and credits.

- OAuth 2.0 with PKCE as a confidential client (`X_CLIENT_ID`, `X_CLIENT_SECRET`). A token is
  refreshed when it has under 40 minutes left, since a publish may run for 30; X issues a new
  refresh token each time and the new one is kept (X-002 stores it before publishing).
- Media goes through the chunked upload in 4 MB segments, then waits while X processes it. Every
  upload step is retried on X's own errors. Creating the post is not: X has no idempotency key,
  so a failure X did not answer with a refusal ends as "check the account before posting again".
- Rules (X's pages, 2026-11-15): 280 weighted characters; 4 photos or 1 video; JPG, PNG, WEBP up
  to 5 MB; a video 0.5 seconds to 20 minutes and between 1:3 and 3:1. Those are the limits of an
  account without Premium.
- **Counting differs from X in two known ways.** An emoji made of several code points (a flag, a
  skin tone, a family) is counted per code point, so it counts more than X's 2: a post near the
  limit may be refused here that X would take. A bare domain (`example.com`) is not counted as
  a link, as X does: such a post may pass here and be refused by X.
- **Left out:** GIF (X takes one, alone, up to 15 MB, which `Rules` cannot say); a limit on a
  video's width and height (X's page contradicts itself); the cost hint on the stamp.
- **Our own cap, not X's:** a video of 1 GiB, because every byte passes through the server.
- **Not from X's reference:** the link to a published post (`x.com/{username}/status/{id}`).
  Confirm it with the first real post.
- Unconfirmed, as before: whether an upload is billed, and whether a failed request is.

### TT-001 · TikTok upload to inbox · M
**Needs:** OAUTH-001, X-002, VID-001. Upload a video to the user's TikTok drafts
(`video.upload`), which they finish in the app. Chunked upload (5 to 64 MB chunks, in order).
Photos can only be pulled from a domain verified with TikTok, so they wait for a custom domain on
the bucket. At most 5 pending drafts a day. Access tokens last 2 hours, refresh tokens 180 days
(TikTok's pages, 2026-11-15; an earlier note here said 24 hours and a year, which is what the
example answer on the refresh page shows).

**Done when:** a stubbed upload-to-inbox flow is tested, and the owner sees a draft in their
TikTok app.

### TT-002 · TikTok audit for direct posting · L · Owner
**Needs:** TT-001. Until audited, direct posts are private-only and at most 5 users a day can
post. Direct posting also requires a set UI (creator info, a privacy choice with no default,
interaction toggles off). The agent prepares the audit material; the owner submits.

**Done when:** TikTok approves direct posting with public visibility.

### RDT-001 · Reddit API access · S · Owner · Dropped
Dropped 2026-11-15: the owner chose to post to Reddit by hand (RDT-002), so nothing is requested.
Kept for the record of why. Since November 2025 Reddit approves every new Data API client, and commercial use needs a
separate written agreement. Native image and video upload is not in Reddit's official API docs,
so under "official APIs only" Reddit is text and link posts. The owner requests access.

**Done when:** Reddit approves the app for posting on users' behalf.

### RDT-002 · Reddit by hand · S
Decided by the owner on 2026-11-15: Reddit is an assisted publish, like WhatsApp. The iOS app
hands the text and media to Reddit through the share sheet; the author picks the community and
the title there, which settles the old open question of where a title and subreddit come from.
The server sends nothing and stores no Reddit credentials. A post going to both WhatsApp and
Reddit gets one share sheet after the other, each reported alone. iOS only; the web shows the
outcome. Cannot be scheduled, like any handoff.

**Done when:** the post opens in Reddit ready to send from a real iPhone, and Sent shows it as
handed off.

### FB-001 · Facebook Pages publisher · M
**Needs:** OAUTH-001; META-001 for strangers, since role holders on the app can use Standard Access
before it. Pages only: posting to personal profiles is not possible. Connect lists the
user's Pages and creates one account per chosen Page.

**Done when:** tests against a stub pass, and a post lands on a real Page from production.

**Built 2026-11-15, against a stub only.** How it differs from the plan above:
- **No Page picker of ours.** Facebook's consent dialog asks which Pages to share, so every Page
  shared that the person can create content on becomes an account (`publisher.MultiConnector`,
  `ExchangeAll`). To add or drop a Page, connect again and change the choice there. A login that
  shares no usable Page ends as `connect_error=failed`, with no message of its own yet.
- **Video goes by URL** (`file_url` on `graph-video.facebook.com`), not through the Resumable
  Upload API. Whether Facebook answers before or after it has fetched the file is not documented;
  the request gets 4 minutes.
- **Scopes:** `pages_show_list`, `pages_read_engagement`, `pages_manage_posts`, the set the photo
  and video references ask for. The Pages guide lists more; they were left out.
- **Limits are ours where Meta states none** (checked 2026-11-15): 5,000 characters, 10 photos,
  video MP4 only and 300 MB. From the reference: photos of 10 MB. Raise them against a Meta page
  that says so, or after a real post.
- **Never retried once the publishing request is sent** unless Facebook clearly refused it: there
  is no idempotency key and nothing to ask afterwards. The user sees "check the Page before
  posting again". Uploading the photos of a multi-photo post is retried freely.
- A video post is saved with no link: Meta documents the address of a Page post, not of a video.
- Left for later: links as link posts (`link`), scheduled publishing on Facebook's side, Reels.

### VID-001 · Video rules · M
Platforms limit video duration, aspect ratio and codec, which `Rules` cannot express today. Add
the fields, measure duration and dimensions (client-side in web and iOS, checked on the server
where possible), and show the problems on the stamps.

**Done when:** a video too long for one platform blocks only that platform's stamp, with the
reason.

**Built 2026-11-15.** How it works, and what it leaves out:

- The server cannot measure a file: there is no ffmpeg in the cluster. The client reads a
  video's length and shown size (after rotation) and declares them with the upload
  (`duration_ms`, `width`, `height` on `POST /v1/media`); they are stored on the `media` row.
  The web app reads them from a `<video>` element, the iOS app with AVFoundation.
- `Rules` gained `MinVideoLength`, `MaxVideoLength`, `MinVideoAspect`, `MaxVideoAspect` and
  `MaxVideoWidth`; zero means the platform states no limit. `Validate` and both clients' copies
  check them, so only the stamps of the platforms that refuse the video are blocked.
- Limits set, each from the platform's own page: Instagram 3 seconds to 15 minutes, Threads up
  to 5 minutes, both at most 1920 pixels wide and 0.01:1 to 10:1 (checked 2026-11-15);
  LinkedIn 3 seconds to 30 minutes. Discord and Facebook state none.
- **A declared value is not verified.** A client that declares nothing, or lies, passes the
  check, and the platform then refuses the video itself. This is feedback for an honest client,
  not a gate.
- **When the client cannot read the file, nothing is checked.** A browser that does not play
  the codec (HEVC in a MOV, on some browsers) reads nothing.
- **The 1920-pixel width blocks 4K video** on Instagram and Threads, as their pages state it.
  Whether Meta refuses such a file or scales it down has not been tried with a real account.
- Not built: codec, frame rate and bitrate checks; image shape (Instagram's 4:5 to 1.91:1).

### OPT-001 · Options per destination · S
Some platforms need more than the post: TikTok a privacy level, Reddit a subreddit and a title.
A post has no place for them, and they differ per destination.

**Done when:** a value set for one destination reaches that platform's publisher, and one the
platform does not take is refused with the rest of the post's issues.

**Built 2026-11-15.**

- A platform lists what it takes in `Rules.Options` (key, label, length limit, required), so
  both composers build the fields from `GET /v1/platforms` and a new option needs no client
  change. `POST /v1/posts` takes `options` keyed by account id; they are checked by `Validate`,
  stored on the target (`post_targets.options`, jsonb), returned on the target, kept when a post
  is rescheduled, and handed to `Publish` in `Post.Options`.
- Every option is one line of text. **A choice from a list (TikTok's privacy level) is not
  built:** TikTok's choices come from the creator's account, not from the platform's rules, so
  that shape belongs with TT-001.
- First use: Discord's "Forum post title" (100 characters, Discord's thread name limit), which
  replaces the post's first line as the thread name (DSC-001). A webhook cannot tell what kind
  of channel it posts to, so the field shows under every Discord stamp and is used only in a
  forum or media channel.
- RDT-002's open question (title and subreddit) is not decided by this; it only makes the
  "composer gets a title field" answer cheap.

---

# P3 · iOS

**Gate:** the owner installs a TestFlight build, signs in, and publishes a post with a photo
from the share sheet, plus one assisted publish to WhatsApp.

### IOS-001 · Project and generated client · M
SwiftUI app, client generated from `api/openapi.yaml` with `swift-openapi-generator` and
committed; `task check` fails if it is stale.

**Done when:** the app builds in CI and lists `GET /v1/platforms` from a local server.

### IOS-002 · Sign in · S
**Needs:** AUTH-001. The iOS side of whatever sign-in was chosen: `SignInWithAppleButton`, a
nonce per sign-in, and the token traded at `POST /auth/apple`.

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

### IOS-007 · TestFlight builds · S
Beta and Release talk to production (`Release.xcconfig`; Beta inherits it). The shared
`Postitheka` scheme archives Beta. `ios/tools/testflight.sh` (`task ios:archive`,
`task ios:testflight`) archives for devices with automatic signing, numbers the build from the
UTC time, and exports with `ios/tools/ExportOptions.plist` straight to App Store Connect. Upload
signs in with an App Store Connect API key when `ASC_KEY_ID`, `ASC_ISSUER_ID` and `ASC_KEY_PATH`
are set, otherwise with the Apple ID in Xcode. `PrivacyInfo.xcprivacy` declares no tracking,
UserDefaults (reason CA92.1), and the post text and photos or videos the person sends as
collected for the app's function.

**Owner:** the App Store Connect app record for `com.fernandocorreia.postitheka`, and upload
access (Xcode signed in, or an API key). External testers also need a privacy policy URL
(PRV-001) and Beta App Review.

**Done when:** a build uploaded by `task ios:testflight` installs from TestFlight on the owner's
phone and talks to production.

---

# Brand

Experiments the owner judges before they reach production. Not on any phase's gate.

### BRD-001 · 3D mascot on beta sign-in (web + iOS) · M
**Needs:** the owner's go to copy `Postitheka_Mascot_Web_NoTablet.glb` from the kit into an
empty source directory (Gate 1 is the owner's look at the recolour, before client work is
judged).

The supplied robot, recoloured to the palette and without its tablet, in 3D on the web's SignIn
and the iOS SignInView, on beta only. Rules in `brand/README.md` "3D mascot". Shape copied from
Aaru's BRD-011.

- `brand/tools/build_mascot3d.py` (stdlib, run with `python3 -I`) builds `out/mascot3d/mascot.glb`
  and `Mascot.usdz` from the source GLB.
- `design/mascot-motion.json` is the motion; `web/src/lib/mascot-motion.ts` and PostithekaKit's
  `MascotMotion` turn a time into values, and both tests assert the same numbers.
- Web: `MASCOT_3D_MODEL_URL` (empty means off) reaches the page through `GET /client-config`;
  three is imported only by `web/src/lib/mascot3d/scene.ts`, after first paint.
- iOS: a Beta build configuration; Release excludes the 3D view, model and motion file.

**Done when:**
- With `MASCOT_3D_MODEL_URL` set, web SignIn shows the flat mascot first and the 3D one once a
  frame is drawn, at desktop and phone width; empty, reduced motion, Save-Data, no WebGL or a
  broken URL keep the flat one. A hidden tab stops drawing, and the page holds one WebGL context.
- A Debug or Beta iOS build shows the 3D mascot on SignInView; Reduce Motion and Low Power Mode
  keep the flat one.
- `ios/tools/release-proof.sh` passes: a Release build holds no model file, no motion file and
  no `Mascot3DView` symbol.
- `scripts/size-budget.mjs` passes with the entry budget unchanged; only the lazy total grew.
- `scripts/brand-guard.sh` (motion copies match, no hex literal in the 3D code) and both
  drivers' tests pass in CI.
- The owner's go/no-go is recorded here with lazy JS KB, model KB, time to first 3D frame and
  frame rate, from a device.

---

# Deferred

Not before P2's gate, so nobody reopens them early.

- **Bluesky, Mastodon, Threads, LinkedIn.** Free or review-light, but not on the owner's list.
- **Different text per platform** in one post. Decided 2026-11-05 to be premium only, so it waits for
  the tiers.
- **Rewriting a post per platform with an LLM.** If it ever comes, it follows the shared Haiku
  trial rules in `~/Work/production/CLAUDE.md`.
- **Editing or deleting a post on the platforms after publishing.**
- **Teams and shared accounts.**
- **Engagement analytics** (likes, views) read back from platforms.
