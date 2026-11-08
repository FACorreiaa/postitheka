# Postitheka: feature inventory

Last verified: 2026-11-06, against `postitheka` `main` f8b8285. Production runs nothing: Postitheka is not in `platform/infra` (checked at `origin/main` e838769).

Use this document to pick features, write scripts and plan recordings. Positioning lives in `strategy.md`, and capture tooling lives in `rules/video-playbook.md`.

## How to read this

**Status** would normally describe what a real user can do in production. **Postitheka has no production**, so no feature below is available to anyone outside the owner's machine. Every status is relative to the code on `main` run locally, and every piece of copy must present the product as in development (`strategy.md`, rule 3).

| Status | Meaning |
|---|---|
| `verified` | On `main`, reachable from the UI, covered end to end by tests in the repo (against a stubbed network where one is involved), and marked Done in `BACKLOG.md`. Read in source: **no screen was rendered for this review**, so rehearse before filming. Not live, because nothing is deployed. |
| `partial` | Works, with a limit stated on the feature. Follow the limit in any copy. |
| `experimental` | Built and tested against a stub, but it has never met the real network or a real device, or it needs an owner setup (developer app, device) that does not exist yet. Test before promising. |
| `planned` | Not available. Never market it. See "Do not market" below. |

**Platforms** are Web, iOS, or Both. Networks are named where they apply.

**Priority** reflects customer value and how well it shows on screen, not technical effort.

**Real network** in a recording means the shot needs a real social network account. There is no fake-platform mode: the publishers call each network's real API host, so without a real account a post stops at "Waiting", "Retrying" or "Not sent". Use a dedicated test account, never a public one (`rules/video-playbook.md`).

**Paths** start from the repository root, `~/Work/production/apps/postitheka/`. This is one repo: `internal/` and `cmd/` are the Go API and publish workers, `web/src/` the web composer, `ios/Postitheka/` the iOS views and `ios/PostithekaKit/` the iOS logic.

**Labels differ by platform.** Use the label the viewer will see on screen.

| Concept | Web label | iOS label |
|---|---|---|
| Text field placeholder | "What do you want to say?" | "What's happening?" |
| Destination list | "Send to" | "Stamps" section |
| Add files | "Add photos or video" | "Add photos or videos" |
| Character count | Per stamp, "used / limit" | One total, "N characters", below the text |
| Schedule control | "Publish at" date and time field | "Schedule" toggle, then "Send at" |
| Main button | Publish, or Schedule when a time is set | Publish, or Schedule, in the navigation bar |
| Queued | "Waiting" | "Queued" |
| In flight | "Sending" | "Sending" |
| Landed | A drawn postmark with the network, day and time, and a linked destination name | "Published HH:MM", a seal icon, and an "Open" link |
| Failed | "Not sent", with the reason | "Not sent", with the reason |
| WhatsApp outcome | Postmark plus "Sent from the phone" | "Sent HH:MM" |
| Accounts | "Accounts (n)" button in the header, opening a dialog | Accounts tab |
| Disconnect | Disconnect button | Swipe, then Disconnect |
| Community | "Join us on Discord" in the header | "Join us on Discord" under Community in Accounts |

---

## The five features to lead with

| # | Feature | Why |
|---|---|---|
| 1 | [Checked at the door](#2-checked-at-the-door) | The most visible difference: every stamp counts and objects in its own terms while you type |
| 2 | [One composer, many stamps](#1-one-composer-many-stamps) | The whole promise on one screen |
| 3 | [Sent, with a postmark for every landing](#6-sent-with-a-postmark-for-every-landing) | Turns "it posted somewhere" into proof, with a link |
| 4 | [Never posts twice](#5-never-posts-twice) | A trust claim nobody expects, and true in code |
| 5 | [Schedule a post](#4-schedule-a-post) | Filmable with no network at all, up to the moment it sends |

The WhatsApp hand-off (#11) would be a stronger fifth, because nothing else in this repo's category does it through an official route, but it has never been tried on a device. Promote it to the list after one real send.

---

## Do not market

These appear in old docs, in brand copy or in the code, but a user cannot use them today.

| Item | Actual state | Evidence |
|---|---|---|
| **Anything as "available"** | Not deployed, no hostname, no sign-up, no TestFlight | `platform/infra/argocd/apps` has no Postitheka app; `ios/Config/Release.xcconfig` is empty |
| **X** | Not built. Pay-per-use, billed to the owner | `BACKLOG.md` X-001; no `internal/publisher/x` |
| **Instagram** | Not built | IG-001 |
| **Facebook Pages** | Not built | FB-001 |
| **TikTok** | Not built. Drafts first, then an audit | TT-001, TT-002 |
| **Reddit** | Not built; rollout recommends by hand | RDT-001, RDT-002 |
| **Bluesky, Mastodon** | Deferred | `BACKLOG.md`, Deferred |
| **"Publish everywhere"** as a literal claim | Three networks and a hand-off, none live | `cmd/server/main.go` |
| **LinkedIn Company Pages** | Personal profile only | `internal/publisher/linkedin/linkedin.go` |
| **Discord forum and media channels** | Fail permanently without `thread_name` | DSC-001 |
| **Different text per network** | Decided as premium, not built | `docs/platform-rollout.md` |
| **Free / premium / any price** | No tiers or billing exist | No billing code |
| **Retry a failed post** | No route or button | WEB-001 |
| **Edit, delete or cancel a post** | No route. A scheduled post cannot be cancelled | `api/openapi.yaml` has no DELETE or PATCH on posts |
| **Reconnect warning** | An expired LinkedIn token is not shown until posts fail | ACC-001 |
| **Video length and shape checks** | Only type and size are checked | VID-001 |
| **Video conversion on the phone** | Not built; videos keep their container | IOS-004, `ios/Postitheka/MediaImport.swift` |
| **Share extension** ("share from Photos") | Not built | IOS-005 |
| **Background uploads on iOS** | Not built | IOS-004 |
| **Sign in with Apple** | Not built. Sign-in is a pasted token | AUTH-001 |
| **Engagement analytics** | Deferred | `BACKLOG.md`, Deferred |
| **Teams, shared accounts** | Deferred | `BACKLOG.md`, Deferred |
| **AI writing or rewriting** | None in the code; LLM rewriting is deferred | `BACKLOG.md`, Deferred |
| **Android, iPad, Mac** | No client | `TARGETED_DEVICE_FAMILY = 1` |
| **App Store rating, "rated by…"** | The app asks for a rating but has never been on the store | `ios/PostithekaKit/Sources/PostithekaKit/ReviewPrompter.swift` |
| **Offline web app** | A manifest exists; no service worker was found | `web/public/manifest.json` |

---

## Platform differences

**iOS only:** the WhatsApp hand-off, the Photos picker (photos converted to JPEG on the phone, up to 10 picked at once), pull to refresh in Sent, the rating prompt and its switch.

**Web only:** a character counter on each stamp, the drawn postmark, upload progress bars per file, the Accounts dialog with Connect buttons and the Discord form on one page.

**Server (both clients):** validation at the door, idempotent publish, scheduling, per-destination jobs, retries, rate-limit waits, encryption of credentials, Discord mention suppression.

**Everything else is on both**: composing, stamps with live problems, scheduling, Sent with outcomes and links, connecting Discord by webhook, connecting LinkedIn and Threads by their sign-in page when configured, disconnecting, the community link. Layout and labels differ (table above).

---

# A. Compose and publish

## 1. One composer, many stamps

- **What it does.** One text field and one set of photos or videos. Every connected account appears as a stamp; you choose any number of them and press Publish once.
- **Problem.** Posting one update in several places means opening several apps and doing the same job each time.
- **Benefit.** Write it once, choose where it goes, send it.
- **Platforms.** Both. Destinations today: Discord channels, LinkedIn and Threads when configured, WhatsApp from iOS.
- **Workflow.** 1) Write the post. 2) Add photos or a video. 3) Tap the stamps you want. 4) Press Publish.
- **Location.** Web: the left column of the main page. iOS: Compose tab.
- **Code.** `web/src/Composer.tsx`, `ios/Postitheka/ComposerView.swift`, `ios/PostithekaKit/Sources/PostithekaKit/Composer.swift`, `internal/service/posts.go`.
- **Status.** `verified` on web (FND-004, checked in a browser). `experimental` on iOS (IOS-003 "built, unverified", though `PublishFlowTests` published to a local server). A text, media, or both, is required.
- **Priority.** High. It is the product.
- **Headline.** "Write it once. Choose your stamps."
- **Recording.** Tier A with seeded accounts (no publish), or Tier B with a private Discord test server for the full send. Name stamps after the test server's channels, such as "#announcements" and "#changelog". Never use a real community's channel.
- **Video concept (20 s).** Four phone screens side by side, each an app's own composer, with the same text being typed four times. Cut to Postitheka: one text, three stamps lit gold, one Publish.

## 2. Checked at the door

- **What it does.** While you type, each chosen stamp checks the post against its network's limits and lists what it would refuse. Publish stays disabled until every chosen stamp would accept it. The server checks again, and if any destination would refuse, it answers with every issue and stores nothing.
- **Problem.** You find out a post was too long, or the wrong file type, after one network has already published it.
- **Benefit.** Know it fits everywhere before anything goes out.
- **Platforms.** Both. The web shows a "used / limit" counter on each stamp; iOS shows the problems under each chosen stamp.
- **Workflow.** 1) Choose stamps. 2) Type past a limit: the counter turns red and says "N characters over". 3) Add a video and an image: LinkedIn says "A video must be posted alone". 4) Fix it or deselect that stamp. 5) Publish unlocks.
- **Location.** Web: on the stamps under "Send to". iOS: under each chosen stamp in the Stamps section.
- **Code.** `internal/publisher/rules.go` (`Validate`, `CountText`, `CountLinks`), `web/src/rules.ts` (`problems`, `countText`), `ios/PostithekaKit/Sources/PostithekaKit/Rules.swift`, `internal/service/posts.go` (422 with every issue). Tests: `TestCreatePostReportsEveryIssueAndCreatesNothing`, `rules_test.go`, kit `RulesTests`.
- **Status.** `verified`. Checks text length (Threads counts each emoji by its UTF-8 bytes), links (Threads: 5), file count, type and size, and "video alone" (LinkedIn). **It does not check video duration or aspect ratio** (VID-001). Web messages: "N characters over", "At most N links", "Needs a photo or video", "Takes no media", "At most N files", "Cannot take TYPE", "A video is over N MB", "A video must be posted alone".
- **Priority.** High. The clearest thing nobody else on screen does, and no network is needed to show it.
- **Headline.** "It fits before it goes."
- **Recording.** Tier A. Needs seeded Discord, LinkedIn and Threads accounts and placeholder LinkedIn and Threads app ids so their rules load (`rules/video-playbook.md`). Strong detail: type one emoji and watch the Threads counter jump by 4 while LinkedIn and Discord move by 1. Stop before Publish.
- **Video concept (20 s).** One sentence typed slowly. Three counters tick: "212 / 2,000", "212 / 3,000", "212 / 500". Paste a long paragraph: Threads turns red, "88 characters over". Deselect Threads, Publish lights up. Line: "Each network has its own rules. Postitheka knows them."

## 3. Photos and video

- **What it does.** Attach photos and videos to a post. Each file goes straight from the device to storage with a presigned URL; the server then confirms the size and type in storage before the file can be used.
- **Problem.** Uploading the same files to each network, one by one.
- **Benefit.** Upload once, send to every stamp.
- **Platforms.** Both. Web: a progress bar per file. iOS: the Photos picker, up to 10 at a time; photos are converted to JPEG on the phone.
- **Workflow.** 1) Add photos or video. 2) Watch each upload finish ("Publish unlocks when uploads finish."). 3) Remove one with ×. 4) Publish.
- **Location.** Web: "Add photos or video" under the text. iOS: "Add photos or videos" in the first section.
- **Code.** `internal/service/uploads.go`, `internal/media/store.go`, `web/src/Composer.tsx` (`attach`), `ios/Postitheka/MediaImport.swift`, `ios/PostithekaKit/Sources/PostithekaKit/Uploader.swift`.
- **Status.** `partial`. Web verified with a cross-origin upload (FND-004). The iOS Photos path was not exercised by the UI test. Videos are not converted or checked for length or shape. The server never transcodes, so files must already suit each network: an MP4, H.264/AAC, under 20 MiB fits Discord, LinkedIn and Threads together. Uploads not used in a post are deleted after 24 hours.
- **Priority.** Medium. Necessary, not distinctive.
- **Headline.** "Upload once."
- **Recording.** Tier A on the web against local RustFS, with a JPEG and a short MP4 of your own making. Use 1080×1350 JPEGs; they fit every built network.
- **Video concept (15 s).** Drag three photos in, three bars fill, three stamps stay clear. Cut to the same photos in each destination (Tier B).

## 4. Schedule a post

- **What it does.** Set a time, and the post waits until then. Each destination is queued for that moment.
- **Problem.** The right moment to post is rarely the moment you finish writing.
- **Benefit.** Write it now, send it later.
- **Platforms.** Both. Web: "Publish at" then "Schedule". iOS: "Schedule" toggle, "Send at", then "Schedule".
- **Workflow.** 1) Write and choose stamps. 2) Set the time. 3) Press Schedule. 4) Sent shows "scheduled for …" with each destination "Waiting" (web) or "Queued" (iOS).
- **Location.** Web: bottom of the composer. iOS: the section under Stamps.
- **Code.** `internal/service/posts.go` (`ScheduledAt`, River `InsertOpts.ScheduledAt`), `web/src/Composer.tsx`, `ios/Postitheka/ComposerView.swift`.
- **Status.** `partial`. No test publishes a scheduled post at its time; it relies on River's scheduling. It needs a server that stays up, which does not exist yet. **A scheduled post cannot be edited or cancelled.** WhatsApp cannot be scheduled ("Cannot be scheduled: it is handed over now").
- **Priority.** Medium. Expected in the category, and very filmable.
- **Headline.** "Write it now. Send it Tuesday."
- **Recording.** Tier A up to the Sent row. To show it landing, Tier B: schedule two minutes ahead to the Discord test server and record the postmark arriving. After a Tier A take, stop the server or reset the database, or the job will run later against the seeded accounts.
- **Video concept (15 s).** Set Tuesday 09:00, press Schedule, Sent shows the letter waiting. Time-lapse a clock. The postmark stamps in.

## 5. Never posts twice

- **What it does.** Each press of Publish carries an idempotency key, so pressing it again after a lost connection returns the first post instead of creating a second. If a publish attempt died mid-request, the next attempt marks that destination failed with "check the platform before posting again" rather than risk a duplicate.
- **Problem.** A flaky connection and a second tap can put the same post on a timeline twice.
- **Benefit.** No duplicates in public.
- **Platforms.** Server, used by both clients. Web error copy: "The connection dropped before the server answered. Press Publish again; it will not post twice."
- **Workflow.** Invisible when it works. Seen only when a connection drops (web message above) or an attempt is interrupted (the "Not sent" reason).
- **Location.** Behind Publish.
- **Code.** `internal/service/posts.go`, `migrations/20261022190000_post_idempotency.sql`, `internal/jobs/publish.go` (`failInterrupted`); tests `TestCreatePostIsIdempotent` (including 8 concurrent requests), `TestWorkerDoesNotRepublish`.
- **Status.** `verified` on the server. The web retry path was checked by typecheck only, not in a browser. It prefers a failure to a duplicate, so say "built never to post twice", never "guaranteed delivery".
- **Priority.** High for trust, low for picture. Best as narration over another shot.
- **Headline.** "Tap twice. Post once."
- **Recording.** Not reliably filmable: forcing a dropped connection mid-request is fragile. Use it as a voice-over line over the Publish moment, or as a simple motion graphic. Never fake the error screen.
- **Video concept (10 s).** A thumb double-taps Publish. One postmark per destination, not two. Line: "Tap twice. Post once."

# B. Outcomes

## 6. Sent, with a postmark for every landing

- **What it does.** Every post you sent, newest first, with each destination's outcome. A landed copy gets a postmark (web) or "Published HH:MM" (iOS) and a link to the live post. A failed one says "Not sent" and why. The list refreshes itself every 2 seconds while anything is still on its way.
- **Problem.** After posting in several places you have to go and check each one.
- **Benefit.** Proof of where it landed, in one list.
- **Platforms.** Both.
- **Workflow.** 1) Publish. 2) The post appears in Sent with each destination "Waiting", then "Sending". 3) Postmarks arrive one by one. 4) Click one to open the live post.
- **Location.** Web: the right column, "Sent". iOS: Sent tab.
- **Code.** `web/src/Sent.tsx` (`Postmark`, `TargetRow`), `ios/Postitheka/SentView.swift`, `internal/publisher/discord/discord.go` (message link), `internal/jobs/publish.go`.
- **Status.** `verified` on web against stubs. iOS `experimental` (the UI test saw an outcome arrive from a local server). Shows the 30 most recent posts on the web. Empty state on the web: "Posts you publish appear here, with where each one landed." with the mascot.
- **Priority.** High. The payoff shot of every video.
- **Headline.** "A postmark for every landing."
- **Recording.** Tier B: needs a real destination to reach "Published". The private Discord test server is the only one that needs no developer app. Two webhooks in that server give two postmarks for one post. Then click the link and show the message in the Discord channel.
- **Video concept (20 s).** Publish. The Sent letter appears with two grey pills. One by one they turn into gold-ringed postmarks with cyan cancellation lines. Click one: the message sits in the channel.

## 7. Each destination retries on its own

- **What it does.** Once a post is accepted, each destination is its own job. A network error is retried with backoff (10 attempts, about a day); a rate limit waits exactly as long as the network says; a permanent refusal fails at once with the network's reason. One failing destination never holds up the others.
- **Problem.** One network having a bad minute should not sink the whole post.
- **Benefit.** The ones that can land, land; the rest keep trying or say why not.
- **Platforms.** Server. Shown in Sent as "Retrying" with the last error, then "Published" or "Not sent".
- **Workflow.** Automatic.
- **Location.** Sent.
- **Code.** `internal/jobs/publish.go`, `internal/publisher/publisher.go` (`Permanent`, `RetryAfterError`); tests `TestWorkerOutcomes`, `TestTokenRefresh`.
- **Status.** `partial`. **No Retry button for a failed destination** (WEB-001). The job timeout is 5 minutes, sized for Discord; large videos to LinkedIn or Threads may not fit (rollout, "Job timeout").
- **Priority.** Low on screen, medium in copy.
- **Headline.** "One bad network doesn't sink the post."
- **Recording.** Tier B, carefully: in the Discord test server, delete one of two webhooks, then publish to both. One postmark; one "Not sent" with "the webhook was deleted; reconnect the channel". Only if the owner wants a failure on screen.
- **Video concept (15 s).** Two stamps, one Publish. One postmark, one honest "Not sent" with the reason. Line: "When something fails, it tells you why."

# C. Destinations

## 8. Discord channels

- **What it does.** Connect any Discord channel by pasting its webhook URL. Postitheka checks the URL is one Discord issued, confirms it exists, and names the account after the webhook unless you give a name. Posts go out with mentions disabled, up to 2,000 characters and 10 files of 20 MiB each.
- **Problem.** Announcing in several channels or servers is copy and paste, and a stray @everyone pings hundreds of people.
- **Benefit.** Every channel is a stamp, and posts never ping anyone by accident.
- **Platforms.** Both.
- **Workflow.** 1) In Discord: channel settings, Integrations, Webhooks, New Webhook, Copy Webhook URL. 2) In Postitheka: Accounts, paste it, optionally name it, Connect. 3) The channel appears as a stamp.
- **Location.** Web: Accounts > "Connect a Discord channel". iOS: Accounts > "Connect a Discord channel".
- **Code.** `internal/publisher/discord/discord.go`, `internal/service/accounts.go`; tests `discord_test.go`, `TestConnectDiscordRejectsForeignURLs`.
- **Status.** `experimental` by this document's definition: **never posted to a real channel** (FND-002 caveat). It is the most complete destination and needs no developer app, so one test post would make it `verified`. Forum and media channels fail (DSC-001).
- **Priority.** High. The only destination that can be filmed end to end without the owner creating developer apps.
- **Headline.** "Every channel is a stamp."
- **Recording.** Tier B with a private Discord server created for filming, with no members besides the owner. Never use the community server or any real community. Blur or crop the webhook URL: it is a credential.
- **Video concept (20 s).** Discord's webhook screen, Copy. Paste into Postitheka, Connect, the stamp appears. Publish. Cut to the channel: the message, no pings.

## 9. LinkedIn personal profile

- **What it does.** Connect your LinkedIn profile on LinkedIn's own sign-in page, then post text, up to 20 images, or one video to your feed. Special characters are escaped so the post reads as written.
- **Problem.** LinkedIn's limits and markup differ from everywhere else.
- **Benefit.** Your profile becomes one more stamp.
- **Platforms.** Both, once the server is configured.
- **Workflow.** 1) Accounts > Connect LinkedIn. 2) Sign in on LinkedIn and allow. 3) Back in Postitheka: "LinkedIn connected." 4) The profile is a stamp.
- **Location.** Accounts > "Connect LinkedIn" (appears only when `LINKEDIN_CLIENT_ID` is set).
- **Code.** `internal/publisher/linkedin/linkedin.go`, `internal/service/connect.go`, `internal/httpapi/oauth.go`; tests `linkedin_test.go`, `TestOAuthConnect`.
- **Status.** `experimental`. Stub-tested only. **No LinkedIn developer app exists**, so the button does not appear in any environment. Posts go to the main feed as **PUBLIC**. Personal profile only. Tokens last 60 days with no refresh and nothing warns before they expire (ACC-001).
- **Priority.** Medium, once it has posted for real.
- **Headline.** "Your LinkedIn, one more stamp."
- **Recording.** Stamp and rules only, Tier A (seeded account plus a placeholder client id). A real publish is **blocked under the capture rules**: there is no LinkedIn sandbox, posts are public, and fake profiles are understood to break LinkedIn's terms. Owner decision.
- **Video concept (15 s).** The LinkedIn stamp beside Discord: the same text, "/ 3,000" against "/ 2,000". Add a video and a photo: only LinkedIn objects, "A video must be posted alone". (The markup escaping is visible only in a published post, so it is not a Tier A shot.)

## 10. Threads

- **What it does.** Connect a Threads profile on Threads' own page, then post text, images, a video, or a carousel of up to 20. Counts emoji the way Threads does, and allows at most 5 links.
- **Problem.** Threads counts differently and caps links.
- **Benefit.** One more stamp that knows its own rules.
- **Platforms.** Both, once configured.
- **Workflow.** As LinkedIn, with "Connect Threads".
- **Location.** Accounts > "Connect Threads" (only when `THREADS_APP_ID` is set).
- **Code.** `internal/publisher/threads/threads.go`; tests `threads_test.go`.
- **Status.** `experimental`. Stub-tested only. **No Meta app exists.** Until App Review, only Threads testers on the owner's app can connect. Media is fetched by Meta from a URL, so it cannot work against local storage: from a local server, text only.
- **Priority.** Medium, once it has posted for real.
- **Headline.** "Threads counts emoji differently. So does Postitheka."
- **Recording.** Tier A for the stamp and the emoji counter. A real post would need the owner's Meta app and a dedicated, private Threads test profile (Tier B, owner decision).
- **Video concept (12 s).** Type "Launch day 🚀". Discord reads 12, Threads reads 15.

## 11. WhatsApp from your iPhone

- **What it does.** WhatsApp has no posting API, so the iPhone app hands the post, text and files, to WhatsApp's share sheet. You choose the chat and tap Send. Postitheka records it as sent or not sent; a hand-off the phone never reports fails after an hour with "The phone never reported back. Check WhatsApp before sending it again."
- **Problem.** Groups on WhatsApp get left out of every cross-posting tool.
- **Benefit.** The same post reaches your WhatsApp groups, within WhatsApp's rules.
- **Platforms.** iOS only. Needs WhatsApp installed ("WhatsApp is not installed on this phone" otherwise).
- **Workflow.** 1) Compose. 2) Tap the WhatsApp stamp ("You send it from WhatsApp"). 3) Publish. 4) The share sheet opens. 5) Choose WhatsApp, a chat, Send. 6) Sent shows "Sent HH:MM".
- **Location.** iOS Compose > Stamps > WhatsApp.
- **Code.** `ios/Postitheka/ComposerView.swift` (`WhatsAppStamp`), `ios/Postitheka/ShareSheet.swift`, `internal/service/posts.go` (`assisted`), `internal/jobs/handoffs.go`; tests `TestAssistedHandoff`, kit `aWhatsAppHandoffOpensAfterPublishingAndIsReported`, UI test `testWhatsAppHandoffClosedWithoutSending`.
- **Status.** `experimental`. **Never tried on a device**; the simulator has no WhatsApp, so only the cancel path has run. Cannot be scheduled. Never call it automatic: the person taps Send.
- **Priority.** High once proven. Distinctive.
- **Headline.** "WhatsApp too. You tap Send."
- **Recording.** Tier C: a physical iPhone with WhatsApp, a Debug build pointed at the Mac's `.local` name, sending to the owner's own "Message yourself" chat or a test group with no other members. Blur the contact list in the share sheet.
- **Video concept (20 s).** Discord stamp and WhatsApp stamp chosen. Publish. Discord's postmark arrives; the share sheet slides up; one tap in WhatsApp; back in Sent, both show sent.

## 12. Connect with the network's own sign-in

- **What it does.** For LinkedIn and Threads, Connect opens the network's own consent page; Postitheka never sees your password. Coming back shows "LinkedIn connected." or a plain reason it did not work. Connecting the same profile again renews it rather than adding a second one.
- **Problem.** Handing a password to a third-party tool.
- **Benefit.** You sign in where you always do.
- **Platforms.** Both: web in the same tab, iOS in a system web-authentication sheet.
- **Workflow.** Accounts > Connect [network] > sign in on the network > allow > back with a notice.
- **Location.** Accounts. Copy: "You sign in on the platform's own page; Postitheka never sees your password."
- **Code.** `internal/service/connect.go`, `internal/httpapi/oauth.go`, `migrations/20261023150000_oauth_connect.sql`, `ios/PostithekaKit/Sources/PostithekaKit/Connect.swift`; test `TestOAuthConnect`.
- **Status.** `experimental`. Never run against a real network. The iOS connect screen has not been looked at on a simulator. Discord does not use it: its webhook URL is the credential.
- **Priority.** Medium, as a trust beat.
- **Headline.** "Sign in where you always do."
- **Recording.** The Connect buttons and their line of copy are Tier A with placeholder app ids. The network's consent page is Tier B and shows that network's UI, so follow its brand rules (`rules/brand.md`).
- **Video concept (10 s).** The Connect button, the network's familiar page, one Allow, back to "connected".

## 13. Disconnect, keep the history

- **What it does.** Disconnect an account at any time. Posts already sent stay in Sent; anything still queued for it fails as "the account was disconnected".
- **Problem.** Removing an integration usually removes its history too.
- **Benefit.** Leave a network without losing the record.
- **Platforms.** Both. iOS footer: "Swipe to disconnect. Posts already sent stay in Sent."
- **Workflow.** Accounts > Disconnect (web) or swipe > Disconnect (iOS).
- **Location.** Accounts.
- **Code.** `internal/service/accounts.go`; `post_targets.account_id ON DELETE SET NULL` in `migrations/`; `TestWorkerOutcomes` (disconnected).
- **Status.** `verified`.
- **Priority.** Low.
- **Headline.** "Leave any time. Keep the record."
- **Recording.** Tier A.
- **Video concept.** A trust beat inside a longer cut, not a video of its own.

# D. Apps and trust

## 14. The web composer

- **What it does.** The whole product in one page: composer on the left, Sent on the right, Accounts in a dialog. Served by the same Go binary as the API.
- **Problem.** n/a; it is the surface.
- **Benefit.** Works in any modern browser, at desktop and phone width.
- **Platforms.** Web.
- **Workflow.** Sign in with a token, compose, send.
- **Location.** `localhost:5173` in development, `:8095` from the built binary.
- **Code.** `web/src/App.tsx`, `web/embed.go`, `web/src/styles.css`.
- **Status.** `verified` locally (FND-004, WEB-002: checked at 1280 and 390 px, contrast passes WCAG AA). Not deployed. Sign-in is a pasted API token: developer-only, never present it as sign-up.
- **Priority.** High as the main filming surface.
- **Headline.** n/a.
- **Recording.** 1440×900 for landscape, 390 px wide for a phone-shaped web take. Start already signed in.
- **Video concept.** The base of every web shot.

## 15. The iPhone app

- **What it does.** Compose, Sent and Accounts tabs, in the same night-and-gold design, plus the WhatsApp hand-off and the Photos picker.
- **Problem.** Posting happens away from the desk.
- **Benefit.** The same post from your pocket.
- **Platforms.** iOS 18 or later, iPhone only.
- **Workflow.** Sign in with a token, then the three tabs.
- **Location.** Debug builds only.
- **Code.** `ios/Postitheka/`, `ios/PostithekaKit/`.
- **Status.** `experimental`. Not on TestFlight or the App Store. A Release build shows "No server". Nothing has been tested on a device (P3). It asks for an App Store rating after posts on three different days or after the first post; the `-ui-testing` launch argument suppresses it.
- **Priority.** Medium. Say "iPhone app in development"; never show a store badge.
- **Headline.** n/a.
- **Recording.** Simulator, Debug build, local server (`rules/video-playbook.md`).
- **Video concept.** The vertical base of short-form cuts.

## 16. Credentials sealed on the server

- **What it does.** Every connected account's credentials are encrypted with AES-256-GCM, bound to their row, never logged, never in an API response or URL. The apps hold nothing but your own sign-in token.
- **Problem.** Third-party tools holding keys to your accounts.
- **Benefit.** Your accounts' keys stay on the server, encrypted.
- **Platforms.** Server.
- **Workflow.** Invisible.
- **Location.** n/a.
- **Code.** `internal/secretbox/`; tests `TestCredentialsAreSealedAtRest`, `secretbox_test.go`, `TestErrorsCarryNoSecrets` (Threads).
- **Status.** `verified` in code. No security audit or certification exists; never imply one.
- **Priority.** Low on screen, useful in an objection answer.
- **Headline.** "Your keys stay sealed."
- **Recording.** None. Copy only.
- **Video concept.** None on its own.

## 17. Community Discord

- **What it does.** "Join us on Discord" opens the Postitheka community invite, a separate server from any destination.
- **Problem.** n/a.
- **Benefit.** A place to get help and share ideas (iOS footer copy).
- **Platforms.** Both.
- **Workflow.** Web header link; iOS Accounts > Community.
- **Location.** As above.
- **Code.** `web/src/links.ts`, `ios/Postitheka/AccountsView.swift`.
- **Status.** `verified` that the link exists. Whether the invite is permanent, and whether it should be the call to action, is unconfirmed (`strategy.md`, section 9).
- **Priority.** Medium as a call to action.
- **Headline.** "Join us on Discord."
- **Recording.** Don't film the community server's members or messages.
- **Video concept.** End-card call to action, if approved.

---

# Evidence and assumptions

- **Source.** The working tree at `main` f8b8285 on 2026-11-06. `platform/infra` read at `origin/main` e838769 and local HEAD 66e6429, with a search of every ref: no Postitheka app, values file or secret exists.
- **Not rendered.** No screen, recording or network call was run for this review. Filming sequences and expected labels are read from code and tests, so rehearse them first.
- **Configuration.** The local `.env` sets no LinkedIn, Threads or PostHog keys (values not read). With it as is, `GET /v1/platforms` returns Discord only.
- **Docs disagree with code** in several places, listed in `strategy.md` section 8: the platform list in `CLAUDE.md`, the system diagram in `ARCHITECTURE.md`, the Deferred list in `BACKLOG.md`, and the Threads row in `docs/platform-rollout.md`. Prefer this file and the code.
- **Platform limits** in this file are the values in each `Rules()`, which the repo dates to 2026-10-22 and 2026-10-23. Networks change them; re-check before quoting a number in public.
- **No real post exists.** Every outcome described here was seen in tests against stubs, not on a network.

**Keep this file current.** Per `Claude.md`, when a user-facing feature ships or changes, update its entry here and move anything newly live out of "Do not market". When Postitheka is deployed, rewrite "How to read this" so that status means production again, and record the production SHA in the header.
