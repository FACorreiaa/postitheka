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

/// Counts text the way the server does for a platform's `text_counting`.
/// `.code_points` counts Unicode scalars: `String.count` counts grapheme
/// clusters and would call a family emoji one character where the server
/// sees several. `.emoji_bytes` counts an emoji as its UTF-8 length (Threads).
public func countText(_ text: String, _ mode: Rules.text_countingPayload = .code_points) -> Int {
    guard mode == .emoji_bytes else { return text.unicodeScalars.count }
    return text.unicodeScalars.reduce(0) { n, s in n + (isEmojiPart(s) ? s.utf8.count : 1) }
}

/// An emoji part as `publisher.isEmojiPart` in Go defines it: any symbol (So)
/// or emoji component. The spec's `text_counting` description is the contract.
func isEmojiPart(_ s: Unicode.Scalar) -> Bool {
    if s.properties.generalCategory == .otherSymbol { return true }
    switch s.value {
    case 0x200D, 0x20E3, 0xFE00...0xFE0F, 0x1F3FB...0x1F3FF, 0xE0020...0xE007F: return true
    default: return false
    }
}

/// Counts distinct links as `publisher.CountLinks` does: a scheme or www., up
/// to whitespace, compared without case or trailing punctuation.
public func countLinks(_ text: String) -> Int {
    let pattern = /(?i)\b(?:https?:\/\/|www\.)[^\s<>"]+/
    var seen = Set<String>()
    for match in text.matches(of: pattern) {
        var link = Substring(match.output)
        while let last = link.last, ".,;:!?)]}'\"".contains(last) { link = link.dropLast() }
        seen.insert(link.lowercased())
    }
    return seen.count
}

/// Mirrors `publisher.Validate` in Go (and `problems` in the web app), so the
/// composer shows what the server would say before Publish is pressed. The
/// server still checks; this is feedback while typing, not trust.
public func problems(_ rules: Rules, _ draft: Draft) -> [String] {
    var out: [String] = []
    let hasText = !draft.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    if !hasText && draft.media.isEmpty { return out } // nothing written yet is not an error

    if rules.requires_text && !hasText { out.append("Needs text") }
    let n = countText(draft.text, rules.text_counting)
    if n > rules.max_text_length { out.append("\(n - rules.max_text_length) characters over") }
    if rules.max_links > 0 && countLinks(draft.text) > rules.max_links { out.append("At most \(rules.max_links) link\(rules.max_links == 1 ? "" : "s")") }

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
        case .threads: "Threads"
        case .x: "X"
        case .instagram: "Instagram"
        case .facebook: "Facebook"
        case .tiktok: "TikTok"
        case .whatsapp: "WhatsApp"
        }
    }
}
