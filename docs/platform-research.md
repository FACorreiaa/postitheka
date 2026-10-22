# Platform research

Notes from 2026-09-27, before any code existed, on what each platform allows a
third-party app to post. **Platforms change these rules and prices often:
check each platform's developer docs before building against it**, and record
what you checked, and when, in that platform's `Rules()` and in `BACKLOG.md`.

Corrections since: Discord's upload limit is **20 MiB per file**, not 10
(checked 2026-10-22). TikTok was not covered here; see `ARCHITECTURE.md`
§"Platform matrix".

## What each platform allows

| Platform | What you can post to | Login and approval | Main difficulty |
|---|---|---|---|
| **Discord** | Server channels | Webhooks, or OAuth with the `webhook.incoming` scope. **No app review.** | None really. 2,000-character limit. |
| **X** | The user's own timeline | OAuth 2.0 (PKCE) | **Cost.** The free tier is very limited; real use means paying. |
| **LinkedIn** | Personal profile through "Share on LinkedIn" (`w_member_social`) | OAuth 2.0 | Company pages need the Community Management API, which requires approval. Tokens expire after about 60 days. |
| **Facebook** | **Pages only.** Posting to personal profiles was removed in 2018. | Meta OAuth + App Review + Business Verification | Users with only a personal profile cannot post through an app. |
| **Instagram** | **Business or Creator accounts only** | Meta (Instagram Login) + App Review | Media must be at a public URL Instagram downloads. Daily posting limit. |
| **Threads** | Personal accounts | Threads API (a Meta app) + App Review | Media at a public URL. 500-character limit. |
| **WhatsApp** | Nothing. There is no API for Status or Channels. | — | The Business API only messages people who opted in, paid per message. That is not posting. |

**WhatsApp workaround:** hand the post to WhatsApp from the iOS app with the
share sheet or the `whatsapp://` link. The user taps Send; the server is not
involved. The same handoff works for any platform whose review has not passed.

So there are two ways to publish:

- **Direct publish** through the server: Discord, X, Facebook Pages, Instagram,
  Threads, LinkedIn.
- **Assisted publish** through the iOS share sheet: WhatsApp, and a fallback
  for the others.

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

1. Discord, X and LinkedIn personal profiles: no approval needed to start.
2. Meta (Facebook Pages, Instagram, Threads): **start Business Verification and
   App Review early**; approval takes weeks and needs a live privacy policy, a
   demo video and a working build.
3. WhatsApp through the share-sheet handoff, from the first iOS build.
4. Optionally Bluesky and Mastodon: free, no review; Bluesky has an official Go
   library (`indigo`).
