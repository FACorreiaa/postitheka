# Platform rollout: what promoting our own apps needs

Written 2026-11-05 for a later phase. The purpose: the owner uses Postitheka to promote their own
apps, posting text, images and video, each optional, to X, Instagram, LinkedIn, Facebook, TikTok,
Reddit and Discord, and it is also offered to the public.

Facts come from the platforms' docs, checked on the date above unless a row says otherwise. Limits
and sources are in `docs/platform-research.md`. **Unconfirmed** means no official page settled it,
so test it or re-read the docs before relying on it. No platform has received a real post yet:
every publisher has only run against stubs.

## Decided 2026-11-05

Postitheka is a **public product**, so strangers connect their own accounts. That means:

- **Meta:** App Review and Business Verification (META-001) are required for strangers. Role holders
  need neither, so the owner can test first under Standard Access, subject to the visibility check
  in step 3.
- **Sign-in:** Sign in with Apple (AUTH-001, IOS-002) is required, and so are rate limits (OPS-002).
- **TikTok:** the audit is open to us, since its guidelines require an app "intended for a wide
  audience". Until it passes, direct posts are private-only, so drafts come first.
- **X:** the owner buys credits once the app is on TestFlight and ready to test. The publisher is
  built against stubs before then. Set a spending limit, and post without URLs by default.
- **Facebook and the job timeout:** the changes under Code work are accepted. Neither is built yet.

**X credits are the owner's money.** Pay-per-use bills the developer account behind the app, so every
stranger's post to X costs the owner $0.015, or $0.20 with a URL. Public X access needs a gate, which
is the likely first use of the free and premium check below.

## Still open

| # | Decision | Recommendation | Why |
| --- | --- | --- | --- |
| 3 | TikTok after the audit opens: when to apply? | Ship drafts first, then apply (TT-002) | Review wants a working, public app |
| 4 | Reddit: API or by hand? | By hand | Approval is doubtful, the API has no media, and subreddit rules decide what survives |
| 5 | Free and premium: what does each tier get? | See below | The owner proposed a check for free and premium users; no billing exists yet |
| 6 | Public hostname (deferred 2026-10-22) | Choose one when starting step 2 | OAuth callbacks, TestFlight and scheduled posts need a server that stays up |
| 7 | LinkedIn: personal profile or Company Page? | Personal profile first | Company Pages need an application for the Community Management API |

**Decision 5.** A post has one text, so selecting X caps it at 280 characters, and X cannot drop a
link while LinkedIn keeps it. Different text per platform removes both limits. It is deferred today.
A tier check could make it, and X itself, premium features. How a user pays (StoreKit on iOS, the web
equivalent) is undecided and is not in any backlog item.

## What each platform takes

| Platform | Text only | Images | Video | Image + video together | Cost | Status |
| --- | --- | --- | --- | --- | --- | --- |
| Discord | Yes, 2,000 | 10 files, 20 MiB each | 20 MiB per file | Yes | Free | Built |
| LinkedIn | Yes, 3,000 | 1 to 20, 5 MB | One MP4, 3 s to 30 min, 500 MB | No | Free | Built |
| Threads | Yes, 500 | Up to 20, JPEG/PNG, 8 MB | MP4/MOV, 5 min, 1 GB | Carousel: not checked | Free | Built, not on `main` |
| X | Yes, 280 weighted | Up to 4, 5 MB | 20 min, 8 GB | Unconfirmed | $0.015 a post, $0.20 with a URL | Not built |
| Instagram | **No** | Up to 20, **JPEG only**, 8 MB, 4:5 to 1.91:1 | Reels, MP4/MOV, 3 s to 15 min, 300 MB | Carousel, cropped to the first item's ratio | Free | Not built |
| Facebook | Yes, limit unconfirmed | One by URL; several unconfirmed | Resumable upload; limits unconfirmed | Unconfirmed | Free | Not built |
| TikTok | **No** | By URL from a verified domain, JPEG/WebP | MP4/WebM/MOV, 10 min, 4 GB | Not checked | Free | Not built |
| Reddit | Yes, plus a required title | **None through the official API** | **None through the official API** | n/a | Unpublished if commercial | Not built |

The API already treats text and media as optional: a post needs one or the other. Each platform's
own requirements then apply, and because of "all or nothing at the door", one failing stamp gets
the whole post refused with a 422 until that stamp is deselected. So a text-only post cannot include
Instagram or TikTok, and a post with media cannot include Reddit.

### What to export so one post fits everywhere

| Asset | Export | Reason |
| --- | --- | --- |
| Image | JPEG, under 5 MB, 1080×1350 (4:5) | LinkedIn and X cap at 5 MB, Instagram takes only JPEG and 4:5 to 1.91:1. **PNG screenshots block Instagram** |
| Video | MP4, H.264/AAC, 3 to 60 s, about 2.5 Mbps | 60 s at 2.5 Mbps is about 18 MiB, under Discord's 20 MiB. Instagram also needs 3 s or more |
| Video shape | 9:16 vertical | Intended for Reels and TikTok; ratio limits not re-checked, and nothing validates them yet (VID-001) |
| Text | 280 weighted characters if X is selected | X counts URLs as 23, and CJK and emoji as 2. Threads counts emoji by UTF-8 bytes |

The server does not transcode, and the cluster has no ffmpeg. Files must be right when uploaded.
HEIC is on no platform's list, so reserving an upload for one is refused.

## Per platform

### Discord: ready to use
- **Owner:** create a server channel and a webhook, then paste the URL into Accounts.
- **Code:** nothing. Forum and media channels still fail until DSC-001.

### LinkedIn: ready to use from a local server
- **Owner:** create a developer app with "Share on LinkedIn" and "Sign In with LinkedIn using OpenID
  Connect". Set `LINKEDIN_CLIENT_ID` and `LINKEDIN_CLIENT_SECRET`. List the redirect
  `PUBLIC_URL/oauth/linkedin/callback`.
- **Watch:** tokens last 60 days with no refresh, so reconnect every two months (ACC-001 shows this).
  A Company Page needs a separate application.

### Instagram: needs step 2, then code
- **Owner:** a Professional (Business or Creator) account; no Facebook Page is needed. Create the
  Meta app and give the Instagram account a role on it.
- **Code:** IG-001. Same container model as Threads, so that package is the template.
- **Watch:** images must be JPEG. Links in captions are commonly reported as not clickable (not
  re-checked), so plan on a link in the profile. The limit is 100 API posts in 24 hours and a
  carousel counts as one; read `content_publishing_limit` rather than hard-coding it.

### Threads: needs step 2
- **Owner:** the same Meta app with the Threads use case, and the Threads account added as a tester.
  Set `THREADS_APP_ID` and `THREADS_APP_SECRET`, the Threads values rather than the main app's.
- **Code:** merge first (step 1). It is on `initial-build`, not `main`.

### Facebook: needs step 2, then code
- **Owner:** a Facebook **Page**, because personal profiles cannot be posted to. The same Meta app
  with Facebook Login, and the owner must be able to create content on the Page.
- **Code:** FB-001. Text and links go to `/{page}/feed`, a photo to `/{page}/photos` by URL, and a
  video to `/{page}/videos` with a handle from the Resumable Upload API, all with a Page access
  token (Graph v27.0 in the examples).
- **Interface change:** FB-001 calls for one account per Page, but `OAuthConnector.Exchange` returns
  a single `Connection`. It needs a change or a Page-picking step.
- **Check first:** Meta's pages disagree on permissions. The Pages guide lists `pages_manage_posts`,
  `pages_manage_engagement`, `pages_read_engagement`, `pages_read_user_engagement` and
  `publish_video`; the Video guide lists `pages_show_list`, `pages_read_engagement` and
  `pages_manage_posts`. Request the smallest set that works. Also unconfirmed: text length,
  multi-photo posts, video limits.

### X: code can start now, real posts need credits
- **Owner:** a developer account and app (OAuth 2.0, with the callback listed), plus credits. Set a
  **spending limit** in the console: requests stop when it is reached, which is what we want.
- **Free credits:** $20 after the first eligible card's first credit purchase, and a match of the
  first auto-recharge up to $50. They expire after 3 months. Prepaid cards do not qualify.
- **Cost:** 30 posts a month is $0.45 without URLs and $6.00 with them. The $20 credit covers about
  1,300 posts without URLs or 100 with them.
- **Code:** X-001, including a third `TextCounting` mode written in Go, web and Swift.
- **Unconfirmed:** whether media upload is billed (the price list shows only "Media Metadata" at
  $0.005), what counts as "with URL" (our `CountLinks` skips bare domains like `example.com`, and X
  may not), and whether a failed request is charged.

### TikTok: drafts, then the audit
| Route | What happens | Fit |
| --- | --- | --- |
| Direct post, unaudited | Posts are `SELF_ONLY`, **and the account must be private when posting**. A public account gets a 403 | Unusable for promotion |
| Direct post, audited | Public posts | **The goal.** The guidelines require an app "intended for a wide audience, not limited to internal groups/private use", which a public product can claim |
| Upload to inbox (`video.upload`) | Draft in the user's inbox; they finish it in the TikTok app. At most 5 pending in 24 hours | **First step.** Needs the app approved for the scope |
| By hand | No integration | The fallback |

- **Owner:** a TikTok developer app with the Content Posting API, submitted for review. TikTok says
  several days to two weeks. Photos also need a domain verified with TikTok.
- **Code:** TT-001, which needs VID-001. A draft is not a publish, so it should not show "Published";
  it needs an outcome of its own in the target lifecycle.
- **Direct Post UI (TT-002):** it requires a commercial-content toggle. Choosing "Your Brand" labels
  the post "Promotional content", which is what promoting our own apps needs. It also forbids adding
  promotional watermarks and logos through the client.

### Reddit: recommend by hand
- Every new API client needs approval, and commercial use needs a separate agreement (official text
  read 2026-10-23; Reddit's page refused automated reads on 2026-11-05).
- Third-party guides report personal bots are usually refused, and one says requests stop on
  2026-10-31. **Neither is confirmed.** The official route to test is the access request itself.
- The official API has text and links only, and every post needs a title and subreddit (RDT-002).
- Our own judgement, not a platform rule: subreddits often restrict self-promotion, so each post
  needs reading that community's rules. That does not suit cross-posting.

### Alternative for TikTok and Reddit: assisted publish
The iOS share sheet already hands WhatsApp posts to the app, and the same flow could hand a post to
the TikTok or Reddit app, media included. It needs `IsAssisted` extended and the product rule
changed, since that rule reserves assisted publish for platforms with no posting API. So it is the
owner's call. Unverified: what those apps accept from the share sheet. The WhatsApp handoff has not
been tried on a device either.

## Order of work

**Step 0: first real posts, no deploy.** Local server, Discord webhook, LinkedIn app. This is the
first time any publisher meets a real platform, so it is the cheapest way to find what stubs hid.

**Step 1: reconcile the branches.** Done 2026-11-05 through PR #3: `main` and `initial-build` hold the
same work.

**Step 2: deploy.**
| Need | Why | Who |
| --- | --- | --- |
| R2 bucket and API token (REL-003) | Meta downloads media by URL, and local RustFS is unreachable from outside | Owner creates, agent writes the CORS rule |
| Hostname and deploy on `maat` (REL-004) | OAuth callbacks, and scheduled posts need a server that stays up | Owner approves: merging to infra deploys |
| Sealed secrets | `DATABASE_URL`, `TOKEN_ENCRYPTION_KEY`, S3 keys, and each platform's client id and secret | Owner provides, agent seals |
| Privacy and terms pages (PRV-001) | Meta's Live mode requires a Terms of Service URL, and platform reviews expect both | Agent drafts, owner approves |

A local server with R2 and an https tunnel can do first real Meta posts, but scheduled posts stop
when the laptop sleeps, so it is only a stopgap.

**Step 3: Meta (Threads, Instagram, Facebook).** One Meta app, role holders first, then META-001 for
strangers. **Test this first:**
post once, then open the URL in a logged-out browser. A consultant's blog claims posts from an app in
Development mode are hidden from the public until it goes Live; Meta's docs do not confirm it. If it
is true, switch to Live with Standard Access, which needs a contact email, Terms URL, icon, category
and app purpose.

**Step 4: X.** Build against stubs now; real posts wait for credits, bought once on TestFlight.

**Step 5: TikTok drafts,** then the audit (decision 3).

### Code work, in order

1. **ACC-001:** show when an account needs reconnecting. Every OAuth platform needs it, LinkedIn
   within 60 days.
2. **Job timeout:** `PublishWorker.Timeout` is 5 minutes, sized for ten 20 MiB Discord files. Large
   Reels, X and TikTok uploads plus processing polls may not fit. Measure, then make it fit.
3. **IG-001,** then **FB-001,** then **X-001.**
4. **VID-001:** duration and aspect ratio on stamps. Needed by TikTok and useful for Reels.
5. **Different text per platform:** deferred today until P2's gate, and tied to decision 5.
6. **Cost hint on the X stamp,** once what counts as "with URL" is known: show $0.20 before Publish.
7. **TT-001,** with the draft outcome.

## Done when

The same post, with one image, reaches Discord, LinkedIn, Threads, Instagram and the Facebook Page
from production, each link opens in a logged-out browser, and Sent shows the correct outcome for each.
Then the same with a vertical video. X joins once credits are bought, and TikTok when a draft arrives
in the owner's inbox. The owner's own use comes first; the P2 gate then repeats this for a stranger.

## Sources (checked 2026-11-05)

Official:
- [X API pricing](https://docs.x.com/x-api/getting-started/pricing), [X free credits](https://docs.x.com/x-api/getting-started/free-credits)
- [TikTok Content Sharing Guidelines](https://developers.tiktok.com/doc/content-sharing-guidelines), [Get Started: Direct Post](https://developers.tiktok.com/doc/content-posting-api-get-started), [Get Started: Upload](https://developers.tiktok.com/doc/content-posting-api-get-started-upload-content), [Upload reference](https://developers.tiktok.com/doc/content-posting-api-reference-upload-video), [App Review FAQ](https://developers.tiktok.com/doc/getting-started-faq)
- [Meta Access Levels](https://developers.facebook.com/docs/graph-api/overview/access-levels), [Pages API posts](https://developers.facebook.com/documentation/pages-api/posts), [Video API publishing](https://developers.facebook.com/docs/video-api/guides/publishing), [Instagram content publishing](https://developers.facebook.com/docs/instagram-platform/content-publishing), [App basic settings](https://developers.facebook.com/documentation/development/create-an-app/app-dashboard/basic-settings)

Secondary, not to be relied on alone:
- [Activepieces on Reddit API access](https://www.activepieces.com/blog/reddit-api-access-in-2026-how-to-get-started), [Reddit data API 2026](https://www.socialcrawl.dev/blog/reddit-data-api-2026.md)
- [bundle.social on Facebook permissions](https://bundle.social/blog/facebook-api-permissions), [timetopost on the Page API](https://timetopost.co/blog/how-to-post-to-facebook-page-api/)
- [Consultant's note on Development mode](https://singhamandeep.com/?p=972)
