import Foundation
import Observation

/// One file attached to the post being written.
public struct Attachment: Identifiable, Sendable, Equatable {
    public let id: UUID
    public let file: URL
    public let contentType: String
    public let size: Int64
    public var progress: Double = 0
    public var mediaID: String?
    public var error: String?
    /// Read from a video file before uploading; nil for anything else.
    public let video: VideoFacts?

    public init(id: UUID = UUID(), file: URL, contentType: String, size: Int64, video: VideoFacts? = nil) {
        self.id = id
        self.file = file
        self.contentType = contentType
        self.size = size
        self.video = video
    }

    public var isUploading: Bool { mediaID == nil && error == nil }
}

/// A post waiting to be handed to an assisted platform's app. Holds its own
/// copy of the text and files, because the composer is cleared as soon as the
/// post is created.
public struct Handoff: Identifiable, Sendable {
    public let id: String // the target's id
    public let platform: Platform
    public let text: String
    public let files: [URL]
}

/// The composer's state and behaviour, kept out of the views so it can be
/// tested without a simulator. Same flow as the web composer: check every
/// chosen destination while typing, upload as files are attached, publish
/// once every destination would accept the post.
@MainActor
@Observable
public final class Composer {
    public var text = ""
    public private(set) var attachments: [Attachment] = []
    public var selected: Set<String> = []
    public var scheduledAt: Date?
    /// What is set for one destination alone: by account id, then option key.
    public var options: [String: [String: String]] = [:]
    /// Hand the post to WhatsApp from this device once it is created.
    public var handToWhatsApp = false
    /// Set after publishing when a share sheet should open; the view presents
    /// it and calls `finishHandoff`.
    public private(set) var pendingHandoff: Handoff?
    public private(set) var serverIssues: [Issue] = []
    public private(set) var error: String?
    public private(set) var busy = false

    public var accounts: [Account] = []
    public var rules: [Platform: Rules] = [:]

    private let api: API
    private let uploader: any Uploader
    // The Idempotency-Key for the post being published, kept while the post is
    // unchanged: pressing Publish again after a lost response finds the post
    // the first press created instead of publishing it twice.
    private var attempt: (signature: String, key: String)?

    public init(api: API, uploader: any Uploader = URLSessionUploader()) {
        self.api = api
        self.uploader = uploader
    }

    public var draft: Draft {
        Draft(text: text, media: attachments.map { .init(contentType: $0.contentType, size: $0.size, video: $0.video) })
    }

    /// The brand last picked. Read `brand` for the one in effect.
    public var brandChoice: BrandFilter = .all
    public var brand: BrandFilter { brandChoice.resolved(for: accounts) }

    /// The accounts on screen: every one, or the chosen brand's.
    public var shown: [Account] { accounts.filter(brand.matches) }

    /// Only what is on screen can be published to. A stamp chosen under one
    /// brand and then hidden by another would send a post somewhere the person
    /// can no longer see.
    ///
    /// Nor can an account whose credentials the platform refused: the server
    /// would reject the post, so it is left out until it is connected again.
    public var chosen: [Account] { shown.filter { selected.contains($0.id) && !$0.needsReconnect } }

    /// What would stop `account`'s platform from taking the post as written.
    public func problems(for account: Account) -> [String] {
        guard let rules = rules[account.platform] else { return [] }
        var draft = draft
        draft.options = options[account.id] ?? [:]
        return PostithekaKit.problems(rules, draft)
    }

    /// What `account`'s platform lets a post set for that destination alone.
    public func fields(for account: Account) -> [PostOption] {
        rules[account.platform]?.options ?? []
    }

    public var isEmpty: Bool {
        text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && attachments.isEmpty
    }

    public var canPublish: Bool {
        !busy && (!chosen.isEmpty || handToWhatsApp) && !isEmpty
            && chosen.allSatisfy { problems(for: $0).isEmpty }
            && !attachments.contains { $0.isUploading || $0.error != nil }
            && !(handToWhatsApp && scheduledAt != nil) // a handoff happens now, on this device
    }

    public func toggle(_ account: Account) {
        guard !account.needsReconnect else { return }
        if selected.contains(account.id) { selected.remove(account.id) } else { selected.insert(account.id) }
    }

    /// Attaches a file and uploads it at once, so Publish is quick.
    public func attach(file: URL, contentType: String, size: Int64, video: VideoFacts? = nil) async {
        let item = Attachment(file: file, contentType: contentType, size: size, video: video)
        attachments.append(item)
        do {
            let upload = try await api.reserveUpload(contentType: contentType, size: size, video: video)
            try await uploader.upload(file: file, to: upload) { [weak self] fraction in
                Task { @MainActor in self?.patch(item.id) { $0.progress = fraction } }
            }
            patch(item.id) {
                $0.mediaID = upload.media_id
                $0.progress = 1
            }
        } catch {
            patch(item.id) { $0.error = error.localizedDescription }
        }
    }

    public func remove(_ id: UUID) {
        attachments.removeAll { $0.id == id }
    }

    /// Publishes, returning the post when it was created. Rejections and
    /// errors are left on the composer for the view to show.
    @discardableResult
    public func publish() async -> Post? {
        guard canPublish else { return nil }
        busy = true
        error = nil
        serverIssues = []
        defer { busy = false }

        let mediaIDs = attachments.compactMap(\.mediaID)
        let accountIDs = chosen.map(\.id)
        let assisted: [Platform] = handToWhatsApp ? [.whatsapp] : []
        // Only for where the post goes: the server refuses options for an
        // account that is not a destination.
        let options = options.filter { accountIDs.contains($0.key) }
        let key = idempotencyKey(mediaIDs: mediaIDs, accountIDs: accountIDs + assisted.map(\.rawValue), options: options)
        do {
            switch try await api.publish(
                text: text, mediaIDs: mediaIDs, accountIDs: accountIDs, options: options, assisted: assisted, scheduledAt: scheduledAt,
                idempotencyKey: key
            ) {
            case .created(let post):
                attempt = nil
                if let target = post.targets.first(where: { $0.platform == .whatsapp && $0.status == .handing_off }) {
                    pendingHandoff = Handoff(id: target.id, platform: .whatsapp, text: text, files: attachments.map(\.file))
                }
                handToWhatsApp = false
                text = ""
                attachments = []
                self.options = [:]
                scheduledAt = nil
                return post
            case .rejected(let issues):
                serverIssues = issues
                return nil
            }
        } catch .unreachable {
            error = "The connection dropped before the server answered. Press Publish again; it will not post twice."
            return nil
        } catch {
            self.error = error.localizedDescription
            return nil
        }
    }

    /// Records how the share sheet ended and returns the updated post. The
    /// handoff is cleared either way: a failure to report leaves the target
    /// waiting on the server, which is shown as such, rather than a sheet that
    /// reopens.
    public func finishHandoff(sent: Bool) async -> Post? {
        guard let handoff = pendingHandoff else { return nil }
        pendingHandoff = nil
        do {
            return try await api.completeHandoff(targetID: handoff.id, sent: sent)
        } catch {
            self.error = error.localizedDescription
            return nil
        }
    }

    /// Issues the server raised for one account, or for the post as a whole
    /// when `account` is nil.
    public func issues(for account: Account?) -> [Issue] {
        serverIssues.filter { $0.account_id == account?.id }
    }

    func idempotencyKey(mediaIDs: [String], accountIDs: [String], options: [String: [String: String]] = [:]) -> String {
        // Sorted: a dictionary's order changes between runs, and the same
        // post must give the same signature.
        let set = options.sorted { $0.key < $1.key }
            .flatMap { account, values in values.sorted { $0.key < $1.key }.map { "\(account)\u{1}\($0.key)\u{1}\($0.value)" } }
        let signature = [text, mediaIDs.joined(separator: ","), accountIDs.joined(separator: ","),
                         scheduledAt.map { String($0.timeIntervalSince1970) } ?? "", set.joined(separator: "\u{2}")].joined(separator: "\u{0}")
        if let attempt, attempt.signature == signature { return attempt.key }
        let key = UUID().uuidString
        attempt = (signature, key)
        return key
    }

    private func patch(_ id: UUID, _ change: (inout Attachment) -> Void) {
        guard let i = attachments.firstIndex(where: { $0.id == id }) else { return }
        change(&attachments[i])
    }
}
