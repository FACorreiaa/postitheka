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
        c.handToWhatsApp = true
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

    @Test func aHandoffCannotBeScheduled() {
        let c = composer(StubTransport())
        c.text = "x"
        c.handToWhatsApp = true
        c.scheduledAt = .now.addingTimeInterval(3600)
        #expect(!c.canPublish)
    }
}
