import CoreGraphics
import Testing
@testable import PostithekaKit

struct RulesTests {
    @Test func nothingWrittenIsNotAnError() {
        #expect(problems(discordRules, Draft()).isEmpty)
    }

    @Test func textIsCountedInCodePointsLikeTheServer() {
        // One grapheme, seven code points: the server counts seven.
        #expect(countText("👨‍👩‍👧‍👦") == 7)
        let rules = Rules(max_text_length: 5, text_counting: .code_points, max_links: 0, max_media: 0, max_media_bytes: 0, max_video_bytes: 0, max_total_media_bytes: 0,
                          media_types: [], requires_text: true, requires_media: false, video_alone: false, min_video_ms: 0, max_video_ms: 0, min_video_aspect: 0, max_video_aspect: 0, max_video_width: 0, max_comment_length: 0, options: [])
        #expect(problems(rules, Draft(text: "👨‍👩‍👧‍👦")) == ["2 characters over"])
    }

    @Test func aPlatformThatTakesNoMediaSaysSoOnce() {
        let rules = Rules(max_text_length: 280, text_counting: .code_points, max_links: 0, max_media: 0, max_media_bytes: 0, max_video_bytes: 0, max_total_media_bytes: 0,
                          media_types: [], requires_text: true, requires_media: false, video_alone: false, min_video_ms: 0, max_video_ms: 0, min_video_aspect: 0, max_video_aspect: 0, max_video_width: 0, max_comment_length: 0, options: [])
        let draft = Draft(text: "hi", media: [.init(contentType: "image/png", size: 1), .init(contentType: "image/png", size: 1)])
        #expect(problems(rules, draft) == ["Takes no media"])
    }

    @Test func eachMediaProblemIsReported() {
        let draft = Draft(media: [.init(contentType: "image/heic", size: 30 << 20)])
        #expect(problems(discordRules, draft) == ["Cannot take image/heic", "A file is over 20 MB"])
    }

    @Test func requiredPartsAreReported() {
        let rules = Rules(max_text_length: 100, text_counting: .code_points, max_links: 0, max_media: 1, max_media_bytes: 100, max_video_bytes: 0, max_total_media_bytes: 0,
                          media_types: ["video/mp4"], requires_text: true, requires_media: true, video_alone: false, min_video_ms: 0, max_video_ms: 0, min_video_aspect: 0, max_video_aspect: 0, max_video_width: 0, max_comment_length: 0, options: [])
        #expect(problems(rules, Draft(media: [.init(contentType: "video/mp4", size: 1)])) == ["Needs text"])
        #expect(problems(rules, Draft(text: "hi")) == ["Needs a photo or video"])
    }

    @Test func videosHaveTheirOwnLimitAndPostAlone() {
        let rules = Rules(max_text_length: 100, text_counting: .code_points, max_links: 0, max_media: 20, max_media_bytes: 5 << 20, max_video_bytes: 500 << 20, max_total_media_bytes: 0,
                          media_types: ["image/png", "video/mp4"], requires_text: false, requires_media: false, video_alone: true, min_video_ms: 0, max_video_ms: 0, min_video_aspect: 0, max_video_aspect: 0, max_video_width: 0, max_comment_length: 0, options: [])
        #expect(problems(rules, Draft(media: [.init(contentType: "video/mp4", size: 100 << 20)])).isEmpty)
        #expect(problems(rules, Draft(media: [.init(contentType: "image/png", size: 100 << 20)])) == ["A file is over 5 MB"])
        #expect(problems(rules, Draft(media: [.init(contentType: "video/mp4", size: 1), .init(contentType: "image/png", size: 1)]))
            == ["A video must be posted alone"])
    }

    @Test func totalSizeIsChecked() {
        let rules = Rules(max_text_length: 100, text_counting: .code_points, max_links: 0, max_media: 4, max_media_bytes: 100, max_video_bytes: 0, max_total_media_bytes: 150,
                          media_types: ["image/png"], requires_text: false, requires_media: false, video_alone: false, min_video_ms: 0, max_video_ms: 0, min_video_aspect: 0, max_video_aspect: 0, max_video_width: 0, max_comment_length: 0, options: [])
        let draft = Draft(media: [.init(contentType: "image/png", size: 100), .init(contentType: "image/png", size: 100)])
        #expect(problems(rules, draft) == ["Files total over 150 bytes"])
    }

    @Test func emojiCountAsTheirBytesWhereThePlatformSaysSo() {
        // The same cases as TestCountText in Go: the three counters must agree.
        let cases: [(String, Int)] = [("hello", 5), ("olá", 3), ("日本", 2), ("👍", 4), ("👍🏽", 8), ("❤️", 6),
                                      ("1️⃣", 7), ("👨‍👩‍👧", 18), ("🇵🇹", 8), ("a © b", 6)]
        for (text, want) in cases {
            #expect(countText(text, .emoji_bytes) == want, "\(text)")
        }
        let rules = Rules(max_text_length: 4, text_counting: .emoji_bytes, max_links: 0, max_media: 0, max_media_bytes: 0,
                          max_video_bytes: 0, max_total_media_bytes: 0, media_types: [], requires_text: false,
                          requires_media: false, video_alone: false, min_video_ms: 0, max_video_ms: 0, min_video_aspect: 0, max_video_aspect: 0, max_video_width: 0, max_comment_length: 0, options: [])
        #expect(problems(rules, Draft(text: "ab👍")) == ["2 characters over"])
    }

    @Test func linksAreCountedOnceEach() {
        let cases: [(String, Int)] = [("no links here", 0), ("see https://example.com/a.", 1),
                                      ("https://example.com and https://example.com again", 1),
                                      ("HTTPS://Example.com and https://example.com", 1),
                                      ("www.threads.com, http://a.test (https://b.test)", 3), ("example.com is not counted", 0)]
        for (text, want) in cases {
            #expect(countLinks(text) == want, "\(text)")
        }
        let rules = Rules(max_text_length: 100, text_counting: .code_points, max_links: 1, max_media: 0, max_media_bytes: 0,
                          max_video_bytes: 0, max_total_media_bytes: 0, media_types: [], requires_text: false,
                          requires_media: false, video_alone: false, min_video_ms: 0, max_video_ms: 0, min_video_aspect: 0, max_video_aspect: 0, max_video_width: 0, max_comment_length: 0, options: [])
        #expect(problems(rules, Draft(text: "https://a.test https://b.test")) == ["At most 1 link"])
    }

    // VID-001. The same cases as publisher/video_rules_test.go and web/src/rules.test.ts.
    private var videoRules: Rules {
        Rules(max_text_length: 100, text_counting: .code_points, max_links: 0, max_media: 4, max_media_bytes: 1 << 30,
              max_video_bytes: 0, max_total_media_bytes: 0, media_types: ["video/mp4", "image/jpeg"], requires_text: false,
              requires_media: false, video_alone: false, min_video_ms: 3000, max_video_ms: 300_000, min_video_aspect: 0.5,
              max_video_aspect: 2, max_video_width: 1920, max_comment_length: 0, options: [])
    }

    private func video(_ ms: Int? = nil, _ w: Int? = nil, _ h: Int? = nil) -> [String] {
        problems(videoRules, Draft(media: [.init(contentType: "video/mp4", size: 1, video: VideoFacts(durationMs: ms, width: w, height: h))]))
    }

    @Test func aVideoWithinEveryLimitPasses() {
        #expect(video(60000, 1080, 1920).isEmpty)
        #expect(video(3000, 1080, 1920).isEmpty)
        #expect(video(300_000, 1920, 1080).isEmpty)
    }

    @Test func theLimitAVideoBreaksIsNamed() {
        #expect(video(2500, 1080, 1920) == ["A video is shorter than 3 seconds"])
        #expect(video(372_000, 1080, 1920) == ["A video is longer than 5 minutes"])
        #expect(video(60000, 400, 1000) == ["A video's shape is not accepted"])
        #expect(video(60000, 3840, 2160) == ["A video is wider than 1920 pixels"])
    }

    @Test func whatTheFileDidNotGiveUpIsNotChecked() {
        #expect(video().isEmpty)
        #expect(video(nil, 4000, nil).isEmpty)
        #expect(video(600_000) == ["A video is longer than 5 minutes"])
        #expect(problems(videoRules, Draft(media: [.init(contentType: "video/mp4", size: 1)])).isEmpty)
    }

    @Test func anImageIsNeverMeasuredAsAVideo() {
        let draft = Draft(media: [.init(contentType: "image/jpeg", size: 1, video: VideoFacts(durationMs: 9_000_000, width: 9000, height: 10))])
        #expect(problems(videoRules, draft).isEmpty)
    }

    @Test func lengthLimitsAreWrittenInWords() {
        #expect(formatLength(3000) == "3 seconds")
        #expect(formatLength(1000) == "1 second")
        #expect(formatLength(60000) == "1 minute")
        #expect(formatLength(900_000) == "15 minutes")
        #expect(formatLength(90000) == "1:30")
    }

    @Test func videoFactsKeepOnlyWhatMeansSomething() {
        let upright = VideoFacts(seconds: 12.3456, naturalSize: CGSize(width: 1920, height: 1080), transform: CGAffineTransform(rotationAngle: .pi / 2))
        #expect(upright == VideoFacts(durationMs: 12346, width: 1080, height: 1920))
        #expect(VideoFacts(seconds: .nan, naturalSize: .zero, transform: .identity) == VideoFacts())
        #expect(VideoFacts(seconds: .infinity, naturalSize: CGSize(width: 640, height: 0), transform: .identity) == VideoFacts())
    }

    @Test func optionsAreCheckedAgainstTheirRules() {
        var rules = discordRules
        rules.options = [
            .init(key: "title", label: "Title", max_length: 5, required: true),
            .init(key: "flair", label: "Flair", max_length: 5, required: false),
        ]
        #expect(problems(rules, Draft(text: "x", options: ["title": "Hello", "flair": "news"])).isEmpty)
        #expect(problems(rules, Draft(text: "x", options: ["title": " 😀😀😀😀😀 "])).isEmpty)
        #expect(problems(rules, Draft(text: "x")) == ["Needs a title"])
        #expect(problems(rules, Draft(text: "x", options: ["title": "   "])) == ["Needs a title"])
        #expect(problems(rules, Draft(text: "x", options: ["title": "Hello!", "flair": "breaking"]))
            == ["Title is 1 characters over", "Flair is 3 characters over"])
    }

    /// The same cases as `TestCountXWeighted` in Go and the web app's rules.test.ts.
    @Test func xWeightedCounting() {
        let cases: [(String, Int)] = [
            ("", 0), ("Hello, world!", 13), ("café", 4), ("cafe\u{301}", 4), ("日本語", 6), ("👋", 2),
            ("“quoted” – dash", 15), ("https://example.com", 23),
            ("https://example.com/a/very/long/path?with=query", 23), ("see https://example.com.", 28),
            ("www.example.com and http://a.test", 51), ("🙋🏽", 4),
        ]
        for (text, want) in cases {
            #expect(countText(text, .x_weighted) == want, "\(text)")
        }
    }
}
