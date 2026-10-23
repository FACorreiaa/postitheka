import Foundation
import Observation
import PostithekaKit

/// The signed-in session and what every screen shares: platforms and their
/// rules, connected accounts, and the posts sent so far.
@MainActor
@Observable
final class AppModel {
    /// From the build configuration (`API_BASE_URL`), never typed by the user
    /// and never a secret: the app holds no credentials but its own token.
    let baseURL: URL? = {
        guard let raw = Bundle.main.object(forInfoDictionaryKey: "PostithekaAPIBaseURL") as? String,
              !raw.isEmpty
        else { return nil }
        return URL(string: raw)
    }()

    private let tokens: any TokenStore = KeychainTokenStore()
    private(set) var signedIn: Bool
    private(set) var platforms: [PlatformInfo] = []
    private(set) var accounts: [Account] = []
    private(set) var posts: [Post] = []
    private(set) var loadError: String?

    let api: API
    let composer: Composer

    init() {
        let url = baseURL ?? URL(string: "http://invalid")!
        api = API(baseURL: url, tokens: tokens)
        composer = Composer(api: api)
        signedIn = tokens.read() != nil
    }

    /// Stores the token and checks it with the server. Returns why it was
    /// refused, if it was.
    func signIn(token: String) async -> String? {
        let token = token.trimmingCharacters(in: .whitespacesAndNewlines)
        guard token.hasPrefix("pk_") else { return "A token starts with pk_." }
        tokens.write(token)
        do {
            _ = try await api.platforms()
            signedIn = true
            return nil
        } catch {
            tokens.write(nil)
            return error == .unauthorized ? "That token is not valid." : error.localizedDescription
        }
    }

    func signOut() {
        tokens.write(nil)
        signedIn = false
        platforms = []
        accounts = []
        posts = []
    }

    func refresh() async {
        await run {
            async let platforms = api.platforms()
            async let accounts = api.accounts()
            async let posts = api.posts()
            (self.platforms, self.accounts, self.posts) = try await (platforms, accounts, posts)
            composer.accounts = self.accounts
            composer.rules = Dictionary(uniqueKeysWithValues: self.platforms.map { ($0.platform, $0.rules) })
        }
    }

    func refreshPosts() async {
        await run { posts = try await api.posts() }
    }

    func published(_ post: Post) {
        posts.insert(post, at: 0)
    }

    func replace(_ post: Post) {
        if let i = posts.firstIndex(where: { $0.id == post.id }) { posts[i] = post } else { posts.insert(post, at: 0) }
    }

    func connectDiscord(webhookURL: String, name: String) async -> String? {
        do {
            let account = try await api.connectDiscord(webhookURL: webhookURL, displayName: name.isEmpty ? nil : name)
            accounts.append(account)
            composer.accounts = accounts
            return nil
        } catch {
            if error == .unauthorized { signOut() }
            return error.localizedDescription
        }
    }

    /// Connects an account on an OAuth platform. `authenticate` shows the
    /// platform's consent page and returns the URL the server ended on; nil
    /// means the user closed the page, which is not an error to show.
    /// Returns what to tell the user.
    func connect(_ platform: Platform, authenticate: (URL) async throws -> URL?) async -> ConnectOutcome? {
        let consent: URL
        do {
            consent = try await api.startConnect(platform: platform)
        } catch {
            if error == .unauthorized { signOut() }
            return .failed(error.localizedDescription)
        }
        guard let callback = try? await authenticate(consent) else { return nil }
        let outcome = ConnectOutcome(callback: callback)
        if case .connected = outcome { await refresh() }
        return outcome
    }

    func disconnect(_ account: Account) async {
        await run {
            try await api.disconnect(accountID: account.id)
            accounts.removeAll { $0.id == account.id }
            composer.accounts = accounts
            composer.selected.remove(account.id)
        }
    }

    /// True while any target is still on its way, so Sent keeps polling.
    var hasPending: Bool {
        posts.contains { $0.targets.contains { $0.status == .pending || $0.status == .publishing } }
    }

    private func run(_ work: () async throws -> Void) async {
        do {
            try await work()
            loadError = nil
        } catch {
            let error = error as? APIError ?? .unreachable
            if error == .unauthorized { signOut() } else { loadError = error.localizedDescription }
        }
    }
}
