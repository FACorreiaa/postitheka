import Foundation
import Testing
@testable import PostithekaKit

@MainActor
struct ComposerTests {
    let account = Account(id: "a1", platform: .discord, display_name: "#general", created_at: .now)

    func composer(_ stub: StubTransport) -> Composer {
        let c = Composer(api: API(baseURL: URL(string: "http://localhost:8095")!, tokens: MemoryTokenStore("pk_test"), transport: stub))
        c.accounts = [account]
        c.rules = [.discord: discordRules]
        c.selected = ["a1"]
        return c
    }

    func keys(_ stub: StubTransport) -> [String?] {
        stub.sent.map { $0.request.headerFields[.init("Idempotency-Key")!] }
    }

    @Test func publishingAgainAfterADroppedConnectionReusesTheKey() async {
        let stub = StubTransport()
        stub.failNext = true
        let c = composer(stub)
        c.text = "hello"

        #expect(await c.publish() == nil)
        #expect(c.error?.contains("will not post twice") == true)

        stub.enqueue(201, postJSON)
        #expect(await c.publish()?.id == "p1")
        #expect(keys(stub)[0] == keys(stub)[1])
        #expect(c.text.isEmpty, "a published post clears the composer")
    }

    @Test func aChangedPostGetsANewKey() async {
        let stub = StubTransport()
        stub.failNext = true
        let c = composer(stub)
        c.text = "hello"
        _ = await c.publish()

        c.text = "hello, edited"
        stub.enqueue(201, postJSON)
        _ = await c.publish()

        #expect(keys(stub)[0] != keys(stub)[1])
    }

    @Test func aRejectionIsShownPerAccount() async {
        let stub = StubTransport([(422, #"{"message":"rejected","issues":[{"account_id":"a1","platform":"discord","field":"text","message":"too long"}]}"#)])
        let c = composer(stub)
        c.text = "hello"

        #expect(await c.publish() == nil)
        #expect(c.issues(for: account).map(\.message) == ["too long"])
        #expect(c.issues(for: nil).isEmpty)
        #expect(c.text == "hello", "a rejected post stays in the composer")
    }

    @Test func publishWaitsForEveryDestinationToAccept() {
        let c = composer(StubTransport())
        #expect(!c.canPublish, "nothing written")
        c.text = String(repeating: "a", count: 2001)
        #expect(!c.canPublish, "over Discord's limit")
        #expect(c.problems(for: account) == ["1 characters over"])
        c.text = "fine"
        #expect(c.canPublish)
        c.selected = []
        #expect(!c.canPublish, "no destination")
    }

    @Test func aWhatsAppHandoffOpensAfterPublishingAndIsReported() async {
        let created = #"{"id":"p2","text":"group","media_ids":[],"scheduled_at":"2026-10-23T10:00:00.5Z","created_at":"2026-10-23T10:00:00.5Z","targets":[{"id":"t9","platform":"whatsapp","status":"handing_off"}]}"#
        let handed = #"{"id":"p2","text":"group","media_ids":[],"scheduled_at":"2026-10-23T10:00:00Z","created_at":"2026-10-23T10:00:00Z","targets":[{"id":"t9","platform":"whatsapp","status":"handed_off","published_at":"2026-10-23T10:00:05.25+01:00"}]}"#
        let stub = StubTransport([(201, created), (200, handed)])
        let c = composer(stub)
        c.selected = []
        c.text = "group"
        #expect(!c.canPublish, "no destination yet")
        c.handTo = [.whatsapp]
        #expect(c.canPublish, "WhatsApp alone is a destination")

        _ = await c.publish()

        let sentBody = try? JSONSerialization.jsonObject(with: Data(stub.sent[0].body.utf8)) as? [String: Any]
        #expect(sentBody?["assisted"] as? [String] == ["whatsapp"])
        #expect(c.pendingHandoff?.id == "t9")
        #expect(c.pendingHandoff?.text == "group", "the handoff keeps the text after the composer clears")
        #expect(c.text.isEmpty)

        let post = await c.finishHandoff(sent: true)
        #expect(post?.targets.first?.status == .handed_off)
        #expect(stub.sent[1].request.path == "/v1/targets/t9/handoff")
        #expect(stub.sent[1].body.contains("true"))
        #expect(c.pendingHandoff == nil)
    }

    @Test func redditAndWhatsAppAreHandedOverOneAfterTheOther() async {
        let created = #"{"id":"p3","text":"both","media_ids":[],"scheduled_at":"2026-11-15T10:00:00Z","created_at":"2026-11-15T10:00:00Z","targets":[{"id":"t1","platform":"reddit","status":"handing_off"},{"id":"t2","platform":"whatsapp","status":"handing_off"}]}"#
        let afterReddit = #"{"id":"p3","text":"both","media_ids":[],"scheduled_at":"2026-11-15T10:00:00Z","created_at":"2026-11-15T10:00:00Z","targets":[{"id":"t1","platform":"reddit","status":"handed_off","published_at":"2026-11-15T10:00:05Z"},{"id":"t2","platform":"whatsapp","status":"handing_off"}]}"#
        let afterBoth = #"{"id":"p3","text":"both","media_ids":[],"scheduled_at":"2026-11-15T10:00:00Z","created_at":"2026-11-15T10:00:00Z","targets":[{"id":"t1","platform":"reddit","status":"handed_off","published_at":"2026-11-15T10:00:05Z"},{"id":"t2","platform":"whatsapp","status":"failed","last_error":"the share sheet closed without sending"}]}"#
        let stub = StubTransport([(201, created), (200, afterReddit), (200, afterBoth)])
        let c = composer(stub)
        c.selected = []
        c.text = "both"
        c.handTo = [.whatsapp, .reddit]

        _ = await c.publish()

        let sentBody = try? JSONSerialization.jsonObject(with: Data(stub.sent[0].body.utf8)) as? [String: Any]
        #expect(sentBody?["assisted"] as? [String] == ["reddit", "whatsapp"])
        #expect(c.handTo.isEmpty)
        #expect(c.pendingHandoff?.platform == .reddit)

        _ = await c.finishHandoff(sent: true)
        #expect(stub.sent[1].request.path == "/v1/targets/t1/handoff")
        #expect(c.pendingHandoff?.platform == .whatsapp, "the next share sheet is due")
        #expect(c.pendingHandoff?.text == "both")

        _ = await c.finishHandoff(sent: false)
        #expect(stub.sent[2].request.path == "/v1/targets/t2/handoff")
        #expect(stub.sent[2].body.contains("false"))
        #expect(c.pendingHandoff == nil)
    }

    @Test func saysWhatPublishIsWaitingFor() {
        let c = composer(StubTransport())
        c.selected = []
        #expect(c.waitingFor == "Write something or add a photo or video.")
        c.text = "hello"
        #expect(c.waitingFor == "Tap a stamp to choose where it goes.")
        c.selected = [account.id]
        #expect(c.canPublish)
        #expect(c.waitingFor == nil)
        c.text = String(repeating: "a", count: 2001)
        #expect(c.waitingFor == "Fix what a chosen stamp objects to.")
    }

    @Test func aHandoffCannotBeScheduled() {
        let c = composer(StubTransport())
        c.text = "x"
        c.handTo = [.whatsapp]
        c.scheduledAt = .now.addingTimeInterval(3600)
        #expect(!c.canPublish)
    }

    @Test func optionsGoOutOnlyForChosenDestinations() async {
        let stub = StubTransport([(201, postJSON)])
        let c = composer(stub)
        var rules = discordRules
        rules.options = [.init(key: "title", label: "Forum post title", max_length: 100, required: false)]
        c.rules = [.discord: rules]
        c.text = "hi"
        c.options = ["a1": ["title": "Release notes"], "gone": ["title": "not a destination"]]

        #expect(c.fields(for: account).map(\.key) == ["title"])
        #expect(await c.publish() != nil)
        // The body is pretty-printed; spacing is not what is being checked.
        let body = stub.sent[0].body.filter { !$0.isWhitespace }
        #expect(body.contains("\"options\":{\"a1\":{\"title\":\"Releasenotes\"}}"))
        #expect(!body.contains("gone"))
        #expect(c.options.isEmpty)
    }

    @Test func theFirstCommentGoesOutWithThePost() async {
        let stub = StubTransport([(201, postJSON)])
        let c = composer(stub)
        var rules = discordRules
        rules.max_comment_length = 10
        c.rules = [.discord: rules]
        c.text = "hi"
        c.firstComment = "#tags"
        #expect(c.note(for: account) == nil)
        #expect(await c.publish() != nil)
        let body = stub.sent[0].body.filter { !$0.isWhitespace }
        #expect(body.contains("\"first_comment\":\"#tags\""))
        #expect(c.firstComment.isEmpty, "a published post clears the comment too")
    }

    @Test func aBlankCommentIsNotSent() async {
        let stub = StubTransport([(201, postJSON)])
        let c = composer(stub)
        c.text = "hi"
        c.firstComment = "   "
        #expect(await c.publish() != nil)
        #expect(!stub.sent[0].body.contains("first_comment"))
    }

    @Test func aCommentOverItsLimitHoldsPublish() {
        let c = composer(StubTransport([]))
        var rules = discordRules
        rules.max_comment_length = 3
        c.rules = [.discord: rules]
        c.text = "hi"
        c.firstComment = "#launch"
        #expect(c.problems(for: account) == ["First comment is 4 characters over"])
        #expect(!c.canPublish)
    }

    @Test func aPlatformWithoutCommentsSaysThePostGoesAlone() {
        let c = composer(StubTransport([]))
        c.text = "hi"
        #expect(c.note(for: account) == nil, "no comment, nothing to say")
        c.firstComment = String(repeating: "#", count: 5000)
        #expect(c.note(for: account) == "Takes no first comment; the post goes alone.")
        #expect(c.problems(for: account).isEmpty, "a comment it will not take is no reason to hold the post")
    }

    @Test func aChangedCommentGetsANewKey() async {
        let stub = StubTransport()
        stub.failNext = true
        let c = composer(stub)
        c.text = "hello"
        c.firstComment = "#a"
        #expect(await c.publish() == nil)
        c.firstComment = "#b"
        stub.enqueue(201, postJSON)
        #expect(await c.publish() != nil)
        #expect(keys(stub)[0] != keys(stub)[1])
    }

    @Test func aTargetSaysHowItsCommentWent() throws {
        func target(_ json: String) throws -> Target {
            try JSONDecoder().decode(Target.self, from: Data(#"{"id":"t1","platform":"discord","status":"published"\#(json)}"#.utf8))
        }
        #expect(commentLine(try target("")) == nil)
        #expect(commentLine(try target(#","comment_status":"posted""#))?.text == "First comment posted")
        #expect(commentLine(try target(#","comment_status":"pending","comment_error":"discord: 502""#))?.text == "First comment retrying: discord: 502")
        let failed = commentLine(try target(#","comment_status":"failed","comment_error":"discord: 400""#))
        #expect(failed?.text == "First comment not sent: discord: 400" && failed?.failed == true)
    }

    @Test func anOptionOverItsLimitHoldsPublish() {
        let c = composer(StubTransport([]))
        var rules = discordRules
        rules.options = [.init(key: "title", label: "Forum post title", max_length: 5, required: false)]
        c.rules = [.discord: rules]
        c.text = "hi"
        c.options = ["a1": ["title": "too long"]]
        #expect(c.problems(for: account) == ["Forum post title is 3 characters over"])
        #expect(!c.canPublish)
    }

    @Test func changingAnOptionMakesItANewPost() {
        let c = composer(StubTransport([]))
        c.text = "hi"
        let plain = c.idempotencyKey(mediaIDs: [], accountIDs: ["a1"])
        let titled = c.idempotencyKey(mediaIDs: [], accountIDs: ["a1"], options: ["a1": ["title": "A"]])
        #expect(plain != titled)
        #expect(titled == c.idempotencyKey(mediaIDs: [], accountIDs: ["a1"], options: ["a1": ["title": "A"]]))
    }
}
