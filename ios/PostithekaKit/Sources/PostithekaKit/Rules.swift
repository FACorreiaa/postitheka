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
public typealias PostOption = Components.Schemas.PostOption

/// What the composer holds: enough to check against a platform's rules before
/// anything is uploaded or published.
public struct Draft: Sendable, Equatable {
    public struct Media: Sendable, Equatable {
        public var contentType: String
        public var size: Int64
        /// Read from a video file; nil for anything else, or when unknown.
        public var video: VideoFacts?

        public init(contentType: String, size: Int64, video: VideoFacts? = nil) {
            self.contentType = contentType
            self.size = size
            self.video = video
        }
    }

    public var text: String
    public var media: [Media]
    /// What was set for one destination alone, by the keys in its rules' options.
    public var options: [String: String]
    /// The post's first comment (POST-002), the same for every destination.
    public var comment: String

    public init(text: String = "", media: [Media] = [], options: [String: String] = [:], comment: String = "") {
        self.text = text
        self.media = media
        self.options = options
        self.comment = comment
    }
}

/// Counts text the way the server does for a platform's `text_counting`.
/// `.code_points` counts Unicode scalars: `String.count` counts grapheme
/// clusters and would call a family emoji one character where the server
/// sees several. `.emoji_bytes` counts an emoji as its UTF-8 length (Threads).
public func countText(_ text: String, _ mode: Rules.text_countingPayload = .code_points) -> Int {
    if mode == .x_weighted { return countXWeighted(text) }
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

/// Mirrors `countXWeighted` in Go: NFC, a link is 23, a code point in X's
/// short list of ranges is 1 and any other is 2. Like the server it counts an
/// emoji by its code points, where X counts the whole emoji as 2.
func countXWeighted(_ text: String) -> Int {
    let s = text.precomposedStringWithCanonicalMapping
    let pattern = /(?i)\b(?:https?:\/\/|www\.)[^\s<>"]+/
    var n = 0
    var last = s.startIndex
    for match in s.matches(of: pattern) {
        var link = Substring(match.output)
        while let end = link.last, ".,;:!?)]}'\"".contains(end) { link = link.dropLast() }
        n += xWeight(s[last..<match.range.lowerBound]) + 23
        last = link.endIndex
    }
    return n + xWeight(s[last...])
}

func xWeight(_ s: Substring) -> Int {
    s.unicodeScalars.reduce(0) { n, c in
        switch c.value {
        case 0...0x10FF, 0x2000...0x200D, 0x2010...0x201F, 0x2032...0x2037: n + 1
        default: n + 2
        }
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

    // Only where the platform takes a comment: elsewhere the post goes alone.
    let comment = countText(draft.comment.trimmingCharacters(in: .whitespacesAndNewlines), rules.text_counting)
    if rules.max_comment_length > 0 {
        if comment > rules.max_comment_length { out.append("First comment is \(comment - rules.max_comment_length) characters over") }
        if rules.max_links > 0 && countLinks(draft.comment) > rules.max_links {
            out.append("First comment: at most \(rules.max_links) link\(rules.max_links == 1 ? "" : "s")")
        }
    }

    // Mirrors `Rules.optionIssues` in Go, less the unknown key: a form built
    // from the rules cannot send one.
    for o in rules.options {
        let value = (draft.options[o.key] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        let length = countText(value)
        if value.isEmpty && o.required {
            out.append("Needs a \(o.label.lowercased())")
        } else if length > o.max_length {
            out.append("\(o.label) is \(length - o.max_length) characters over")
        }
    }

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
        if video {
            videos += 1
            out += videoProblems(rules, m.video)
        }
        total += m.size
    }
    if rules.video_alone && videos > 0 && draft.media.count > 1 { out.append("A video must be posted alone") }
    if rules.max_total_media_bytes > 0 && total > rules.max_total_media_bytes {
        out.append("Files total over \(formatBytes(rules.max_total_media_bytes))")
    }
    return out
}

/// Mirrors `Rules.videoIssues` in Go: a limit is checked only against a value
/// the file gave up, and 0 means the platform has no such limit.
func videoProblems(_ rules: Rules, _ facts: VideoFacts?) -> [String] {
    var out: [String] = []
    if let ms = facts?.durationMs, ms > 0 {
        if rules.min_video_ms > 0 && ms < rules.min_video_ms { out.append("A video is shorter than \(formatLength(rules.min_video_ms))") }
        if rules.max_video_ms > 0 && ms > rules.max_video_ms { out.append("A video is longer than \(formatLength(rules.max_video_ms))") }
    }
    if let w = facts?.width, let h = facts?.height, w > 0, h > 0 {
        let aspect = Double(w) / Double(h)
        if (rules.min_video_aspect > 0 && aspect < rules.min_video_aspect) || (rules.max_video_aspect > 0 && aspect > rules.max_video_aspect) {
            out.append("A video's shape is not accepted")
        }
        if rules.max_video_width > 0 && w > rules.max_video_width { out.append("A video is wider than \(rules.max_video_width) pixels") }
    }
    return out
}

/// A length limit in words: "3 seconds", "5 minutes", "1:30".
public func formatLength(_ ms: Int) -> String {
    let s = Int((Double(ms) / 1000).rounded())
    if s < 60 { return "\(s) second\(s == 1 ? "" : "s")" }
    if s % 60 == 0 { return "\(s / 60) minute\(s == 60 ? "" : "s")" }
    return "\(s / 60):\(s % 60 < 10 ? "0" : "")\(s % 60)"
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
        case .reddit: "Reddit"
        }
    }
}

/// One line on a published target's first comment (POST-002), nil when it
/// gets none. `failed` lines are shown as failures.
public func commentLine(_ t: Target) -> (text: String, failed: Bool)? {
    guard let status = t.comment_status else { return nil }
    let why = t.comment_error.map { ": \($0)" } ?? ""
    switch status {
    case .posted: return ("First comment posted", false)
    case .pending: return (why.isEmpty ? "First comment waiting" : "First comment retrying\(why)", false)
    case .posting: return ("Posting the first comment", false)
    case .failed: return ("First comment not sent\(why)", true)
    }
}
