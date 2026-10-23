import Testing
@testable import PostithekaKit

struct RulesTests {
    @Test func nothingWrittenIsNotAnError() {
        #expect(problems(discordRules, Draft()).isEmpty)
    }

    @Test func textIsCountedInCodePointsLikeTheServer() {
        // One grapheme, seven code points: the server counts seven.
        #expect(countText("👨‍👩‍👧‍👦") == 7)
        let rules = Rules(max_text_length: 5, max_media: 0, max_media_bytes: 0, max_total_media_bytes: 0,
                          media_types: [], requires_text: true, requires_media: false)
        #expect(problems(rules, Draft(text: "👨‍👩‍👧‍👦")) == ["2 characters over"])
    }

    @Test func aPlatformThatTakesNoMediaSaysSoOnce() {
        let rules = Rules(max_text_length: 280, max_media: 0, max_media_bytes: 0, max_total_media_bytes: 0,
                          media_types: [], requires_text: true, requires_media: false)
        let draft = Draft(text: "hi", media: [.init(contentType: "image/png", size: 1), .init(contentType: "image/png", size: 1)])
        #expect(problems(rules, draft) == ["Takes no media"])
    }

    @Test func eachMediaProblemIsReported() {
        let draft = Draft(media: [.init(contentType: "image/heic", size: 30 << 20)])
        #expect(problems(discordRules, draft) == ["Cannot take image/heic", "A file is over 20 MB"])
    }

    @Test func requiredPartsAreReported() {
        let rules = Rules(max_text_length: 100, max_media: 1, max_media_bytes: 100, max_total_media_bytes: 0,
                          media_types: ["video/mp4"], requires_text: true, requires_media: true)
        #expect(problems(rules, Draft(media: [.init(contentType: "video/mp4", size: 1)])) == ["Needs text"])
        #expect(problems(rules, Draft(text: "hi")) == ["Needs a photo or video"])
    }

    @Test func totalSizeIsChecked() {
        let rules = Rules(max_text_length: 100, max_media: 4, max_media_bytes: 100, max_total_media_bytes: 150,
                          media_types: ["image/png"], requires_text: false, requires_media: false)
        let draft = Draft(media: [.init(contentType: "image/png", size: 100), .init(contentType: "image/png", size: 100)])
        #expect(problems(rules, draft) == ["Files total over 150 bytes"])
    }
}
