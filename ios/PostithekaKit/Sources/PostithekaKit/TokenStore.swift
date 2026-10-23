import Foundation
import Security

/// Where the API token lives between launches.
public protocol TokenStore: Sendable {
    func read() -> String?
    func write(_ token: String?)
}

/// The Keychain, readable after the first unlock so a background upload can
/// still authenticate, and never synced: a token belongs to one device.
public struct KeychainTokenStore: TokenStore {
    private let service: String
    private let account = "api-token"

    public init(service: String = "com.fernandocorreia.postitheka") {
        self.service = service
    }

    public func read() -> String? {
        var query = baseQuery
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var item: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &item) == errSecSuccess,
              let data = item as? Data
        else { return nil }
        return String(data: data, encoding: .utf8)
    }

    public func write(_ token: String?) {
        SecItemDelete(baseQuery as CFDictionary)
        guard let token, !token.isEmpty else { return }
        var query = baseQuery
        query[kSecValueData as String] = Data(token.utf8)
        query[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        SecItemAdd(query as CFDictionary, nil)
    }

    private var baseQuery: [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
    }
}

/// A token held in memory only: for tests and previews.
public final class MemoryTokenStore: TokenStore, @unchecked Sendable {
    private let lock = NSLock()
    private var token: String?

    public init(_ token: String? = nil) { self.token = token }

    public func read() -> String? { lock.withLock { token } }
    public func write(_ token: String?) { lock.withLock { self.token = token } }
}
