import Foundation
import Testing
@testable import PostithekaKit

struct APITests {
    let base = URL(string: "http://localhost:8095")!

    @Test func everyRequestCarriesTheCurrentToken() async throws {
        let stub = StubTransport([(200, "[]"), (200, "[]")])
        let tokens = MemoryTokenStore("pk_test")
        let api = API(baseURL: base, tokens: tokens, transport: stub)

        _ = try await api.accounts()
        tokens.write("pk_other")
        _ = try await api.accounts()

        #expect(stub.sent.map { $0.request.headerFields[.authorization] } == ["Bearer pk_test", "Bearer pk_other"])
    }

    @Test func publishSendsTheIdempotencyKey() async throws {
        let stub = StubTransport([(201, postJSON)])
        let api = API(baseURL: base, tokens: MemoryTokenStore("pk_test"), transport: stub)

        let outcome = try await api.publish(text: "hi", mediaIDs: [], accountIDs: ["a1"], scheduledAt: nil, idempotencyKey: "k1")

        guard case .created(let post) = outcome else { Issue.record("not created"); return }
        #expect(post.id == "p1")
        #expect(stub.sent[0].request.headerFields[.init("Idempotency-Key")!] == "k1")
        #expect(stub.sent[0].body.contains("\"account_ids\""))
    }

    @Test func aRejectedPostReturnsEveryIssue() async throws {
        let stub = StubTransport([(422, #"{"message":"rejected","issues":[{"account_id":"a1","platform":"discord","field":"text","message":"too long"},{"field":"Idempotency-Key","message":"reused"}]}"#)])
        let api = API(baseURL: base, tokens: MemoryTokenStore("pk_test"), transport: stub)

        let outcome = try await api.publish(text: "hi", mediaIDs: [], accountIDs: ["a1"], scheduledAt: nil, idempotencyKey: "k1")

        guard case .rejected(let issues) = outcome else { Issue.record("not rejected"); return }
        #expect(issues.map(\.field) == ["text", "Idempotency-Key"])
    }

    /// The server's own format: Go's RFC 3339 with a fraction, and without
    /// one when it is zero.
    @Test func timestampsDecodeWithAndWithoutFractions() async throws {
        let stub = StubTransport([(200, """
        [{"created_at":"2026-10-23T00:15:18.429157+01:00","display_name":"a","id":"1","platform":"discord"},
         {"created_at":"2026-10-23T00:15:18+01:00","display_name":"b","id":"2","platform":"discord"}]
        """)])
        let api = API(baseURL: base, tokens: MemoryTokenStore("pk_test"), transport: stub)

        let accounts = try await api.accounts()

        #expect(accounts.count == 2)
        #expect(abs(accounts[0].created_at.timeIntervalSince(accounts[1].created_at) - 0.429157) < 0.001)
    }

    @Test func errorsAreTypedForTheApp() async {
        let stub = StubTransport([(401, #"{"message":"no"}"#), (400, #"{"message":"not a Discord webhook"}"#)])
        let api = API(baseURL: base, tokens: MemoryTokenStore(), transport: stub)

        await #expect(throws: APIError.unauthorized) { try await api.platforms() }
        await #expect(throws: APIError.message("not a Discord webhook")) {
            try await api.connectDiscord(webhookURL: "https://example.com", displayName: nil)
        }
        await #expect(throws: APIError.unreachable) { try await api.accounts() } // stub has nothing left
    }

    @Test func anUnreadableResponseIsNotCalledUnreachable() async {
        let stub = StubTransport([(200, #"[{"created_at":"yesterday","display_name":"a","id":"1","platform":"discord"}]"#)])
        let api = API(baseURL: base, tokens: MemoryTokenStore("pk_test"), transport: stub)

        await #expect(throws: APIError.message("The server sent something this version of the app cannot read.")) {
            try await api.accounts()
        }
    }
}
