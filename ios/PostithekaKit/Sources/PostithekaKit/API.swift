import Foundation
import HTTPTypes
import OpenAPIRuntime
import OpenAPIURLSession
import PostithekaAPI

/// Why a call failed, in words the app can show. A server's message is shown
/// as it is; the server writes them for people.
public enum APIError: LocalizedError, Equatable {
    /// The token is missing or no longer valid: the app goes back to sign-in.
    case unauthorized
    case message(String)
    /// No answer at all: offline, or the server is down.
    case unreachable

    public var errorDescription: String? {
        switch self {
        case .unauthorized: "Signed out. Paste a token to sign in again."
        case .message(let m): m
        case .unreachable: "The server did not respond. Check your connection."
        }
    }
}

/// What publishing a post came to.
public enum PublishOutcome: Sendable {
    case created(Post)
    /// Some destination would reject the post. Nothing was created.
    case rejected([Issue])
}

/// The API as the app uses it: typed results instead of the generated
/// response enums, and one error type.
public struct API: Sendable {
    let client: Client

    /// - Parameters:
    ///   - baseURL: the server, e.g. `http://localhost:8095`. Paths in the spec
    ///     start with `/v1`, so this is the bare origin.
    ///   - tokens: read on every request, so signing in or out takes effect
    ///     without rebuilding the client.
    public init(baseURL: URL, tokens: any TokenStore, transport: any ClientTransport = URLSessionTransport()) {
        client = Client(
            serverURL: baseURL,
            configuration: Configuration(dateTranscoder: RFC3339DateTranscoder()),
            transport: transport,
            middlewares: [BearerMiddleware(tokens: tokens)]
        )
    }

    public func platforms() async throws(APIError) -> [PlatformInfo] {
        let out = try await call { try await client.listPlatforms() }
        switch out {
        case .ok(let r): return try body { try r.body.json }
        case .unauthorized: throw .unauthorized
        case .undocumented(let code, _): throw Self.unexpected(code)
        }
    }

    public func accounts() async throws(APIError) -> [Account] {
        let out = try await call { try await client.listAccounts() }
        switch out {
        case .ok(let r): return try body { try r.body.json }
        case .unauthorized: throw .unauthorized
        case .undocumented(let code, _): throw Self.unexpected(code)
        }
    }

    public func connectDiscord(webhookURL: String, displayName: String?) async throws(APIError) -> Account {
        let out = try await call {
            try await client.connectDiscord(body: .json(.init(webhook_url: webhookURL, display_name: displayName)))
        }
        switch out {
        case .created(let r): return try body { try r.body.json }
        case .badRequest(let r): throw .message(try body { try r.body.json.message })
        case .unauthorized: throw .unauthorized
        case .undocumented(let code, _): throw Self.unexpected(code)
        }
    }

    /// The consent page for connecting an account on an OAuth platform. Open
    /// it in a web authentication session that ends on `connectCallbackScheme`;
    /// the server sends the user there with the outcome.
    public func startConnect(platform: Platform) async throws(APIError) -> URL {
        let out = try await call {
            try await client.startConnect(path: .init(platform: platform), body: .json(.init(client: .ios)))
        }
        switch out {
        case .ok(let r):
            let raw = try body { try r.body.json.authorize_url }
            guard let url = URL(string: raw), url.scheme == "https" else {
                throw .message("The server sent a sign-in address that cannot be opened.")
            }
            return url
        case .badRequest(let r): throw .message(try body { try r.body.json.message })
        case .notFound(let r): throw .message(try body { try r.body.json.message })
        case .unauthorized: throw .unauthorized
        case .undocumented(let code, _): throw Self.unexpected(code)
        }
    }

    public func disconnect(accountID: String) async throws(APIError) {
        let out = try await call { try await client.deleteAccount(path: .init(accountId: accountID)) }
        switch out {
        case .noContent, .notFound: return // already gone is what was asked for
        case .unauthorized: throw .unauthorized
        case .undocumented(let code, _): throw Self.unexpected(code)
        }
    }

    public func reserveUpload(contentType: String, size: Int64) async throws(APIError) -> Upload {
        let out = try await call {
            try await client.createUpload(body: .json(.init(content_type: contentType, size_bytes: size)))
        }
        switch out {
        case .created(let r): return try body { try r.body.json }
        case .badRequest(let r): throw .message(try body { try r.body.json.message })
        case .unauthorized: throw .unauthorized
        case .undocumented(let code, _): throw Self.unexpected(code)
        }
    }

    /// Creates the post. `idempotencyKey` makes a retry safe: the same key
    /// with the same post returns the post the first call created.
    public func publish(
        text: String, mediaIDs: [String], accountIDs: [String], assisted: [Platform] = [], scheduledAt: Date?,
        idempotencyKey: String
    ) async throws(APIError) -> PublishOutcome {
        let out = try await call {
            try await client.createPost(
                headers: .init(Idempotency_hyphen_Key: idempotencyKey),
                body: .json(.init(
                    text: text, media_ids: mediaIDs, account_ids: accountIDs,
                    assisted: assisted.isEmpty ? nil : assisted, scheduled_at: scheduledAt
                ))
            )
        }
        switch out {
        case .created(let r): return .created(try body { try r.body.json })
        case .unprocessableContent(let r): return .rejected(try body { try r.body.json.issues })
        case .badRequest(let r): throw .message(try body { try r.body.json.message })
        case .unauthorized: throw .unauthorized
        case .undocumented(let code, _): throw Self.unexpected(code)
        }
    }

    /// Reports how the share sheet for an assisted target ended.
    public func completeHandoff(targetID: String, sent: Bool) async throws(APIError) -> Post {
        let out = try await call {
            try await client.completeHandoff(path: .init(targetId: targetID), body: .json(.init(sent: sent)))
        }
        switch out {
        case .ok(let r): return try body { try r.body.json }
        case .notFound: throw .message("That post no longer exists.")
        // The server stops waiting after an hour and marks the handoff
        // failed; only a report arriving after that lands here from the app.
        case .conflict: throw .message("Postitheka stopped waiting for this one. Check WhatsApp to see whether it was sent.")
        case .unauthorized: throw .unauthorized
        case .undocumented(let code, _): throw Self.unexpected(code)
        }
    }

    public func posts(limit: Int = 20) async throws(APIError) -> [Post] {
        let out = try await call { try await client.listPosts(query: .init(limit: limit)) }
        switch out {
        case .ok(let r): return try body { try r.body.json }
        case .unauthorized: throw .unauthorized
        case .undocumented(let code, _): throw Self.unexpected(code)
        }
    }

    public func post(id: String) async throws(APIError) -> Post {
        let out = try await call { try await client.getPost(path: .init(postId: id)) }
        switch out {
        case .ok(let r): return try body { try r.body.json }
        case .notFound: throw .message("That post no longer exists.")
        case .unauthorized: throw .unauthorized
        case .undocumented(let code, _): throw Self.unexpected(code)
        }
    }

    // MARK: - Plumbing

    /// Only a transport failure is "unreachable". A response that arrived but
    /// could not be decoded is a different problem, and saying the server did
    /// not respond would send someone checking the wrong thing.
    private func call<T>(_ op: () async throws -> T) async throws(APIError) -> T {
        do {
            return try await op()
        } catch let error as ClientError where !(error.underlyingError is URLError) {
            throw .message("The server sent something this version of the app cannot read.")
        } catch {
            throw .unreachable
        }
    }

    private func body<T>(_ read: () throws -> T) throws(APIError) -> T {
        do {
            return try read()
        } catch {
            throw .message("The server sent something this version of the app cannot read.")
        }
    }

    private static func unexpected(_ code: Int) -> APIError {
        .message("The server answered \(code).")
    }
}

/// Go writes times as RFC 3339 with fractional seconds, and leaves the fraction
/// out when it is zero. The runtime's default ISO 8601 transcoder accepts only
/// the second form, so every response with a timestamp failed to decode.
struct RFC3339DateTranscoder: DateTranscoder {
    func encode(_ date: Date) throws -> String {
        date.formatted(.iso8601.year().month().day().time(includingFractionalSeconds: true).timeZone(separator: .omitted))
    }

    func decode(_ string: String) throws -> Date {
        if let date = try? Date(string, strategy: Self.fractional) { return date }
        return try Date(string, strategy: Self.whole)
    }

    private static let fractional = Date.ISO8601FormatStyle(includingFractionalSeconds: true)
    private static let whole = Date.ISO8601FormatStyle()
}

/// Adds the bearer token, read fresh for every request.
struct BearerMiddleware: ClientMiddleware {
    let tokens: any TokenStore

    func intercept(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL: URL,
        operationID: String,
        next: (HTTPRequest, HTTPBody?, URL) async throws -> (HTTPResponse, HTTPBody?)
    ) async throws -> (HTTPResponse, HTTPBody?) {
        var request = request
        if let token = tokens.read() {
            request.headerFields[.authorization] = "Bearer \(token)"
        }
        return try await next(request, body, baseURL)
    }
}
