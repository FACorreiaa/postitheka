import Foundation

/// Which accounts the composer shows: every one, or one brand's. A brand is a
/// label the user puts on an account for the product it posts for. It groups
/// accounts and nothing else. Same rules as the web app's brands.ts.
public enum BrandFilter: Hashable, Sendable {
    case all
    case brand(String)
}

/// The brands in use, each once, in alphabetical order whatever their case.
public func brands(of accounts: [Account]) -> [String] {
    Set(accounts.compactMap(\.brand).filter { !$0.isEmpty })
        .sorted { $0.localizedCaseInsensitiveCompare($1) == .orderedAscending }
}

extension BrandFilter {
    public func matches(_ account: Account) -> Bool {
        switch self {
        case .all: true
        case .brand(let name): account.brand == name
        }
    }

    /// The filter to show, given the one last chosen. A brand no account
    /// carries any more (renamed, or its last account disconnected) falls back
    /// to every account, so the composer is never left showing nothing.
    public func resolved(for accounts: [Account]) -> BrandFilter {
        if case .brand(let name) = self, !accounts.contains(where: { $0.brand == name }) { return .all }
        return self
    }
}
