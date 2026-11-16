import CryptoKit
import Foundation

/// Sign in with Apple (AUTH-001), the part that is not a view: the nonce, and trading Apple's
/// identity token for a Postitheka API token at `POST /auth/apple`.
///
/// That route is outside `api/openapi.yaml` (auth wraps every spec route, and this is how a
/// person gets their first token), so it is called with plain HTTP, not the generated client.
public enum AppleSignIn {
    /// A fresh random string for one sign-in. Apple is given its hash (`hashed`), the server
    /// the string itself, so a token taken from another sign-in does not verify here.
    public static func makeNonce() -> String {
        var generator = SystemRandomNumberGenerator()
        let bytes = (0 ..< 32).map { _ in UInt8.random(in: .min ... .max, using: &generator) }
        return Data(bytes).base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    /// SHA-256 of the nonce, as lowercase hex: what goes in the Apple request, and what the
    /// server compares the token's `nonce` claim with.
    public static func hashed(_ nonce: String) -> String {
        SHA256.hash(data: Data(nonce.utf8)).map { String(format: "%02x", $0) }.joined()
    }

    /// What the server answered.
    public struct Session: Equatable, Sendable {
        public let token: String
        /// True when this sign-in made the account.
        public let created: Bool
    }

    /// Sends one request and returns the answer; tests pass a stub, never a real server.
    public typealias Send = @Sendable (URLRequest) async throws -> (Data, URLResponse)

    /// Trades Apple's identity token for an API token.
    public static func exchange(
        baseURL: URL,
        identityToken: String,
        nonce: String,
        send: Send = { try await URLSession.shared.data(for: $0) }
    ) async throws(APIError) -> Session {
        var request = URLRequest(url: baseURL.appending(path: "auth/apple"))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try? JSONEncoder().encode(
            ["identity_token": identityToken, "nonce": nonce, "client": "ios"]
        )

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await send(request)
        } catch {
            throw .unreachable
        }
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        if status == 200, let body = try? JSONDecoder().decode(Body.self, from: data), body.token.hasPrefix("pk_") {
            return Session(token: body.token, created: body.created ?? false)
        }
        // The server writes its messages for people; show them as they are.
        if let message = try? JSONDecoder().decode(Message.self, from: data).message, !message.isEmpty {
            throw .message(message)
        }
        throw .message("Sign in with Apple failed (\(status)). Try again.")
    }

    private struct Body: Decodable {
        let token: String
        let created: Bool?
    }

    private struct Message: Decodable {
        let message: String
    }
}
