import Foundation
import Testing
@testable import PostithekaKit

struct ProviderSignInTests {
    let base = URL(string: "http://localhost:8095")!

    /// Answers with a canned response and keeps the request; never a real server.
    final class Recorder: @unchecked Sendable {
        var request: URLRequest?
        let status: Int
        let body: String

        init(_ status: Int, _ body: String) {
            self.status = status
            self.body = body
        }

        func send(_ request: URLRequest) async throws -> (Data, URLResponse) {
            self.request = request
            let response = HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!
            return (Data(body.utf8), response)
        }
    }

    @Test func theHashIsWhatTheServerComputes() {
        // Lowercase hex, as Go's hex.EncodeToString(sha256.Sum256(...)) writes it.
        #expect(ProviderSignIn.hashed("raw-nonce") == "2c5d107938053a2275f022c153c9a71f65ee07754b8bca543ee97a0c3cc66990")
        #expect(ProviderSignIn.hashed("abc") == "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
    }

    @Test func everyNonceIsNew() {
        let a = ProviderSignIn.makeNonce(), b = ProviderSignIn.makeNonce()
        #expect(a != b)
        #expect(a.count >= 40)
        #expect(!a.contains("+") && !a.contains("/") && !a.contains("="))
    }

    @Test func exchangeSendsTheRawNonceAndKeepsTheToken() async throws {
        let stub = Recorder(200, #"{"token":"pk_test","created":true}"#)
        let session = try await ProviderSignIn.exchange(.google, baseURL: base, idToken: "jwt", nonce: "raw", send: stub.send)

        #expect(session == .init(token: "pk_test", created: true))
        let request = try #require(stub.request)
        #expect(request.url?.absoluteString == "http://localhost:8095/auth/google")
        #expect(request.httpMethod == "POST")
        let sent = try JSONDecoder().decode([String: String].self, from: request.httpBody ?? Data())
        #expect(sent == ["id_token": "jwt", "nonce": "raw", "client": "ios"])
    }

    @Test func theServersMessageIsShown() async {
        let stub = Recorder(503, #"{"message":"Sign in with Apple is not set up on this server"}"#)
        await #expect(throws: APIError.message("Sign in with Apple is not set up on this server")) {
            try await ProviderSignIn.exchange(.apple, baseURL: base, idToken: "jwt", nonce: "raw", send: stub.send)
        }
    }

    @Test func noAnswerIsUnreachable() async {
        await #expect(throws: APIError.unreachable) {
            try await ProviderSignIn.exchange(.apple, baseURL: base, idToken: "jwt", nonce: "raw") { _ in throw URLError(.notConnectedToInternet) }
        }
    }
}
