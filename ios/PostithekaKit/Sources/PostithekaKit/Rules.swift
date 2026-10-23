import Foundation
import PostithekaAPI

public typealias Rules = Components.Schemas.Rules
public typealias Platform = Components.Schemas.Platform
public typealias PlatformInfo = Components.Schemas.PlatformInfo
public typealias Account = Components.Schemas.Account
public typealias Post = Components.Schemas.Post
public typealias Target = Components.Schemas.Target
public typealias Issue = Components.Schemas.Issue
public typealias Upload = Components.Schemas.Upload

/// What the composer holds: enough to check against a platform's rules before
/// anything is uploaded or published.
public struct Draft: Sendable, Equatable {
    public struct Media: Sendable, Equatable {
        public var contentType: String
        public var size: Int64

        public init(contentType: String, size: Int64) {
            self.contentType = contentType
            self.size = size
        }
    }

    public var text: String
    public var media: [Media]

    public init(text: String = "", media: [Media] = []) {
        self.text = text
        self.media = media
    }
}

/// Counts text the way the server does for platforms without their own
/// counter: Unicode code points. `String.count` counts grapheme clusters and
/// would call a family emoji one character where the server sees several.
public func countText(_ text: String) -> Int {
    text.unicodeScalars.count
}

/// Mirrors `publisher.Validate` in Go (and `problems` in the web app), so the
/// composer shows what the server would say before Publish is pressed. The
/// server still checks; this is feedback while typing, not trust.
public func problems(_ rules: Rules, _ draft: Draft) -> [String] {
    var out: [String] = []
    let hasText = !draft.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    if !hasText && draft.media.isEmpty { return out } // nothing written yet is not an error

    if rules.requires_text && !hasText { out.append("Needs text") }
    let n = countText(draft.text)
    if n > rules.max_text_length { out.append("\(n - rules.max_text_length) characters over") }

    if rules.requires_media && draft.media.isEmpty { out.append("Needs a photo or video") }
    if rules.max_media == 0 && !draft.media.isEmpty {
        out.append("Takes no media")
        return out
    }
    if draft.media.count > rules.max_media { out.append("At most \(rules.max_media) files") }

    var total: Int64 = 0
    var videos = 0
    for m in draft.media {
        if !rules.media_types.contains(m.contentType) { out.append("Cannot take \(m.contentType)") }
        let video = m.contentType.hasPrefix("video/")
        let limit = video && rules.max_video_bytes > 0 ? rules.max_video_bytes : rules.max_media_bytes
        if m.size > limit { out.append("A \(video ? "video" : "file") is over \(formatBytes(limit))") }
        if video { videos += 1 }
        total += m.size
    }
    if rules.video_alone && videos > 0 && draft.media.count > 1 { out.append("A video must be posted alone") }
    if rules.max_total_media_bytes > 0 && total > rules.max_total_media_bytes {
        out.append("Files total over \(formatBytes(rules.max_total_media_bytes))")
    }
    return out
}

public func formatBytes(_ n: Int64) -> String {
    if n >= 1 << 20 { return "\(Int((Double(n) / Double(1 << 20)).rounded())) MB" }
    if n >= 1 << 10 { return "\(Int((Double(n) / Double(1 << 10)).rounded())) KB" }
    return "\(n) bytes"
}

extension Platform {
    /// The platform's own name, as the UI shows it.
    public var displayName: String {
        switch self {
        case .discord: "Discord"
        case .linkedin: "LinkedIn"
        case .x: "X"
        case .instagram: "Instagram"
        case .facebook: "Facebook"
        case .tiktok: "TikTok"
        case .whatsapp: "WhatsApp"
        }
    }
}
