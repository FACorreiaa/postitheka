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
                          media_types: [], requires_text: true, requires_media: false, video_alone: false)
        #expect(problems(rules, Draft(text: "👨‍👩‍👧‍👦")) == ["2 characters over"])
    }

    @Test func aPlatformThatTakesNoMediaSaysSoOnce() {
        let rules = Rules(max_text_length: 280, text_counting: .code_points, max_links: 0, max_media: 0, max_media_bytes: 0, max_video_bytes: 0, max_total_media_bytes: 0,
                          media_types: [], requires_text: true, requires_media: false, video_alone: false)
        let draft = Draft(text: "hi", media: [.init(contentType: "image/png", size: 1), .init(contentType: "image/png", size: 1)])
        #expect(problems(rules, draft) == ["Takes no media"])
    }

    @Test func eachMediaProblemIsReported() {
        let draft = Draft(media: [.init(contentType: "image/heic", size: 30 << 20)])
        #expect(problems(discordRules, draft) == ["Cannot take image/heic", "A file is over 20 MB"])
    }

    @Test func requiredPartsAreReported() {
        let rules = Rules(max_text_length: 100, text_counting: .code_points, max_links: 0, max_media: 1, max_media_bytes: 100, max_video_bytes: 0, max_total_media_bytes: 0,
                          media_types: ["video/mp4"], requires_text: true, requires_media: true, video_alone: false)
        #expect(problems(rules, Draft(media: [.init(contentType: "video/mp4", size: 1)])) == ["Needs text"])
        #expect(problems(rules, Draft(text: "hi")) == ["Needs a photo or video"])
    }

    @Test func videosHaveTheirOwnLimitAndPostAlone() {
        let rules = Rules(max_text_length: 100, text_counting: .code_points, max_links: 0, max_media: 20, max_media_bytes: 5 << 20, max_video_bytes: 500 << 20, max_total_media_bytes: 0,
                          media_types: ["image/png", "video/mp4"], requires_text: false, requires_media: false, video_alone: true)
        #expect(problems(rules, Draft(media: [.init(contentType: "video/mp4", size: 100 << 20)])).isEmpty)
        #expect(problems(rules, Draft(media: [.init(contentType: "image/png", size: 100 << 20)])) == ["A file is over 5 MB"])
        #expect(problems(rules, Draft(media: [.init(contentType: "video/mp4", size: 1), .init(contentType: "image/png", size: 1)]))
            == ["A video must be posted alone"])
    }

    @Test func totalSizeIsChecked() {
        let rules = Rules(max_text_length: 100, text_counting: .code_points, max_links: 0, max_media: 4, max_media_bytes: 100, max_video_bytes: 0, max_total_media_bytes: 150,
                          media_types: ["image/png"], requires_text: false, requires_media: false, video_alone: false)
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
                          requires_media: false, video_alone: false)
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
                          requires_media: false, video_alone: false)
        #expect(problems(rules, Draft(text: "https://a.test https://b.test")) == ["At most 1 link"])
    }
}
