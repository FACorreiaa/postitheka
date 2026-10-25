import Foundation
import Testing
@testable import PostithekaKit

@MainActor
struct ReviewPrompterTests {
    /// A clock the test moves by hand.
    final class Clock {
        var now = Date(timeIntervalSince1970: 1_800_000_000)
        func advance(days: Double) { now = now.addingTimeInterval(days * 24 * 60 * 60) }
        func advance(hours: Double) { now = now.addingTimeInterval(hours * 60 * 60) }
    }

    let clock = Clock()
    let defaults: UserDefaults
    var calendar = Calendar(identifier: .gregorian)

    init() {
        let suite = "ReviewPrompterTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)!
        defaults.removePersistentDomain(forName: suite)
        calendar.timeZone = TimeZone(identifier: "UTC")!
    }

    func prompter() -> ReviewPrompter {
        let clock = clock
        return ReviewPrompter(defaults: defaults, calendar: calendar, now: { clock.now })
    }

    @Test func asksAfterThreeSuccessesOnDifferentDays() {
        let p = prompter()
        clock.advance(days: 2)
        p.recordSuccess()
        #expect(!p.isPromptDue)
        clock.advance(days: 1)
        p.recordSuccess()
        #expect(!p.isPromptDue)
        clock.advance(days: 1)
        p.recordSuccess()
        #expect(p.successCount == 3)
        #expect(p.isPromptDue)
    }

    @Test func successesOnTheSameDayCountOnce() {
        let p = prompter()
        clock.advance(days: 2)
        p.recordSuccess()
        p.recordSuccess()
        clock.advance(hours: 1)
        p.recordSuccess()
        #expect(p.successCount == 1)
        #expect(!p.isPromptDue)
    }

    @Test func neverAsksInTheFirst24Hours() {
        // 23:00 UTC on install: three calendar days fit in about 25 hours.
        clock.now = calendar.startOfDay(for: clock.now).addingTimeInterval(23 * 60 * 60)
        let p = prompter()
        p.recordSuccess() // day 1
        clock.advance(hours: 2)
        p.recordSuccess() // day 2, 2 hours in
        // Three counted days, still inside the grace: the count alone is not enough.
        defaults.set(3, forKey: "postitheka.review.successCount")
        clock.advance(hours: 20)
        p.recordSuccess() // day 2 again, 22 hours in
        #expect(!p.isPromptDue)
        clock.advance(hours: 3)
        p.recordSuccess() // day 3, 25 hours in
        #expect(p.isPromptDue)
    }

    @Test func aMilestoneAsksOnDayOne() {
        let p = prompter()
        p.recordSuccess()
        p.recordMilestone(once: "firstPost")
        #expect(p.isPromptDue)
        #expect(p.shouldPrompt, "the presenter's re-check lets a milestone through")
    }

    @Test func aMilestoneSurvivesASkippedMoment() {
        let p = prompter()
        p.recordMilestone(once: "firstPost")
        p.skipPrompt()
        #expect(!p.isPromptDue)
        p.recordSuccess()
        #expect(p.isPromptDue, "the next success asks for the milestone that was missed")
    }

    @Test func aOnceMilestoneFiresOnlyOnce() {
        let p = prompter()
        p.recordMilestone(once: "firstPost")
        #expect(p.isPromptDue)
        p.didPrompt()
        clock.advance(days: 200)
        p.recordMilestone(once: "firstPost")
        #expect(!p.isPromptDue)
        #expect(defaults.bool(forKey: "postitheka.review.milestone.firstPost"))

        // A fresh prompter on the same defaults (an app relaunch) remembers it.
        let relaunched = prompter()
        relaunched.recordMilestone(once: "firstPost")
        #expect(!relaunched.isPromptDue)

        // A milestone without a key is not limited.
        relaunched.recordMilestone()
        #expect(relaunched.isPromptDue)
    }

    @Test func theCooldownBlocksSuccessesAndMilestones() {
        let p = prompter()
        p.recordMilestone()
        p.didPrompt()

        for _ in 0..<5 {
            clock.advance(days: 1)
            p.recordSuccess()
        }
        #expect(p.successCount == 5)
        #expect(!p.isPromptDue)
        p.recordMilestone(once: "firstPost")
        #expect(!p.isPromptDue)
        #expect(!p.shouldPrompt)

        clock.advance(days: 120)
        p.recordSuccess()
        #expect(p.isPromptDue, "past the cooldown the count asks again")
    }

    @Test func turningItOffBlocksSuccessesAndMilestones() {
        let p = prompter()
        p.isEnabled = false
        for _ in 0..<3 {
            clock.advance(days: 1)
            p.recordSuccess()
        }
        #expect(!p.isPromptDue)
        p.recordMilestone(once: "firstPost")
        #expect(!p.isPromptDue)
        #expect(!prompter().isEnabled, "the switch is remembered")
    }

    @Test func isOnByDefault() {
        #expect(prompter().isEnabled)
    }

    @Test func didPromptResetsTheCountAndStampsTheDate() {
        let p = prompter()
        for _ in 0..<3 {
            clock.advance(days: 1)
            p.recordSuccess()
        }
        #expect(p.isPromptDue)
        p.didPrompt()
        #expect(!p.isPromptDue)
        #expect(p.successCount == 0)
        #expect(defaults.object(forKey: "postitheka.review.lastPromptAt") as? Date == clock.now)
    }
}
