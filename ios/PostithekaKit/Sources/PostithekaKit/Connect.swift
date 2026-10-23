import Foundation

/// The scheme the server redirects to when a connect started on iOS is done.
/// A web authentication session ends when it sees it; no app registration of
/// the scheme is needed for that.
public let connectCallbackScheme = "postitheka"

/// How connecting an account on an OAuth platform ended, read from the URL
/// the server redirected to: `postitheka://connected?platform=linkedin`, or
/// `?error=cancelled|expired|failed`. Mirrors `readConnectOutcome` on the web.
public enum ConnectOutcome: Equatable, Sendable {
    case connected(Platform)
    case failed(String)

    public init(callback url: URL) {
        let items = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems ?? []
        let value = { (name: String) in items.first { $0.name == name }?.value }
        if let raw = value("platform"), let platform = Platform(rawValue: raw) {
            self = .connected(platform)
            return
        }
        // The redirect names no platform on failure: the server only knows the
        // outcome, and the platform's own message never travels in a URL.
        switch value("error") {
        case "cancelled": self = .failed("Not connected: access was not granted on the platform’s page.")
        case "expired": self = .failed("Not connected: the sign-in page was open too long. Try again.")
        default: self = .failed("Not connected: the platform did not accept the sign-in. Try again.")
        }
    }
}
