import Foundation
import HTTPTypes
import OpenAPIRuntime
@testable import PostithekaKit

/// Answers API calls with canned responses and records what was sent. Never a
/// real server, never a real token.
final class StubTransport: ClientTransport, @unchecked Sendable {
    struct Sent {
        var request: HTTPRequest
        var body: String
    }

    private let lock = NSLock()
    private var responses: [(status: Int, json: String)]
    private(set) var sent: [Sent] = []
    var failNext = false

    init(_ responses: [(status: Int, json: String)] = []) {
        self.responses = responses
    }

    func enqueue(_ status: Int, _ json: String) {
        lock.withLock { responses.append((status, json)) }
    }

    func send(_ request: HTTPRequest, body: HTTPBody?, baseURL _: URL, operationID _: String) async throws -> (HTTPResponse, HTTPBody?) {
        var text = ""
        if let body { text = try await String(collecting: body, upTo: 1 << 20) }
        let next: (Int, String)? = lock.withLock {
            sent.append(Sent(request: request, body: text))
            if failNext {
                failNext = false
                return nil
            }
            return responses.isEmpty ? nil : responses.removeFirst()
        }
        guard let (status, json) = next else { throw URLError(.networkConnectionLost) }
        var fields = HTTPFields()
        fields[.contentType] = "application/json"
        return (HTTPResponse(status: .init(code: status), headerFields: fields), HTTPBody(json))
    }
}

let postJSON = """
{"id":"p1","text":"hi","media_ids":[],"scheduled_at":"2026-10-23T10:00:00Z","created_at":"2026-10-23T10:00:00Z",
 "targets":[{"id":"t1","account_id":"a1","platform":"discord","status":"pending"}]}
"""

let discordRules = Rules(
    max_text_length: 2000, max_media: 10, max_media_bytes: 20 << 20, max_video_bytes: 0, max_total_media_bytes: 0,
    media_types: ["image/png", "image/jpeg", "video/mp4"], requires_text: false, requires_media: false, video_alone: false
)
