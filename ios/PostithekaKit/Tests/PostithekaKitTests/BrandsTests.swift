import Foundation
import Testing
@testable import PostithekaKit

// The same cases as the web app's brands.test.ts: change one, change both.
@MainActor
struct BrandsTests {
    static func account(_ id: String, _ brand: String? = nil) -> Account {
        Account(id: id, platform: .discord, display_name: id, brand: brand, created_at: .now)
    }

    let accounts = [account("1", "norviq"), account("2", "Aaru"), account("3"), account("4", "Aaru")]

    @Test func eachBrandIsListedOnceAlphabeticallyWhateverItsCase() {
        #expect(brands(of: accounts) == ["Aaru", "norviq"])
        #expect(brands(of: [Self.account("1")]).isEmpty)
    }

    @Test func allShowsEveryAccountAndABrandOnlyItsOwn() {
        #expect(accounts.filter(BrandFilter.all.matches).map(\.id) == ["1", "2", "3", "4"])
        #expect(accounts.filter(BrandFilter.brand("Aaru").matches).map(\.id) == ["2", "4"])
    }

    @Test func aBrandThatIsGoneFallsBackToAll() {
        #expect(BrandFilter.brand("Loci").resolved(for: accounts) == .all)
        #expect(BrandFilter.brand("Aaru").resolved(for: accounts) == .brand("Aaru"))
    }

    @Test func aStampHiddenByTheBrandFilterIsNotPublishedTo() {
        let c = Composer(api: API(baseURL: URL(string: "http://localhost:8095")!, tokens: MemoryTokenStore("pk_test"), transport: StubTransport()))
        c.accounts = accounts
        c.selected = ["1", "2"]
        #expect(c.chosen.map(\.id) == ["1", "2"])

        c.brandChoice = .brand("Aaru")
        #expect(c.shown.map(\.id) == ["2", "4"])
        #expect(c.chosen.map(\.id) == ["2"], "norviq's stamp is off screen, so it is not a destination")
    }

    @Test func anAccountThatNeedsReconnectingIsNotPublishedTo() {
        let c = Composer(api: API(baseURL: URL(string: "http://localhost:8095")!, tokens: MemoryTokenStore("pk_test"), transport: StubTransport()))
        let stale = Account(id: "9", platform: .linkedin, display_name: "Ada", created_at: .now, needs_reconnect: true)
        c.accounts = accounts + [stale]

        c.toggle(stale)
        #expect(c.selected.isEmpty, "its stamp cannot be chosen")

        // Chosen while it still worked, refused since.
        c.selected = ["2", "9"]
        #expect(c.chosen.map(\.id) == ["2"])
        #expect(!Self.account("1").needsReconnect)
    }

    @Test func settingABrandSendsItAndReturnsTheAccount() async throws {
        let stub = StubTransport([(200, ##"{"id":"a1","platform":"discord","display_name":"#general","brand":"Aaru","created_at":"2026-11-14T10:00:00Z"}"##)])
        let api = API(baseURL: URL(string: "http://localhost:8095")!, tokens: MemoryTokenStore("pk_test"), transport: stub)

        let updated = try await api.setBrand(accountID: "a1", brand: "Aaru")

        #expect(updated.brand == "Aaru")
        #expect(stub.sent[0].request.method == .patch)
        #expect(stub.sent[0].request.path == "/v1/accounts/a1")
        // Decoded, not matched as text: the client pretty-prints its JSON.
        let sent = try JSONDecoder().decode([String: String].self, from: Data(stub.sent[0].body.utf8))
        #expect(sent == ["brand": "Aaru"])
    }
}
