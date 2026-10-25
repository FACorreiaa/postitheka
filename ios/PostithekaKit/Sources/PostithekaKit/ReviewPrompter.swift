import Foundation
import Observation

/// Decides when to ask for an App Store rating.
///
/// A success is a post the server accepted (or a WhatsApp handoff the user
/// sent), counted at most once per calendar day, so the ask comes after posts
/// on three different days. It never comes in the first 24 hours, more than
/// once in 120 days, or after the person turned it off under Accounts.
///
/// A milestone (the first post) asks right away instead, even on day 1: it
/// skips the count and the 24 hours, but not the cooldown or the switch.
///
/// iOS still decides whether the system sheet actually shows; the app's
/// `ReviewPromptPresenter` makes the request. Logic only, kept per device in
/// `defaults`, with the clock injected so the rules are testable without
/// waiting days.
@MainActor
@Observable
public final class ReviewPrompter {
    public static let successesBeforePrompt = 3
    public static let firstSessionGrace: TimeInterval = 24 * 60 * 60
    public static let cooldown: TimeInterval = 120 * 24 * 60 * 60

    /// "Ask me to rate Postitheka". On by default.
    public var isEnabled: Bool {
        didSet { defaults.set(isEnabled, forKey: Self.enabledKey) }
    }

    /// Set when a success or milestone makes a prompt due; the presenter clears it.
    public private(set) var isPromptDue = false

    /// A milestone that has not been asked for yet. Kept in memory past
    /// `skipPrompt()`, so a first post whose moment passed (app backgrounded,
    /// tab switched) is asked for on the next success instead of being lost:
    /// its `once` key is already spent.
    private var milestonePending = false

    @ObservationIgnored private let defaults: UserDefaults
    @ObservationIgnored private let calendar: Calendar
    @ObservationIgnored private let now: () -> Date

    private static let enabledKey = "postitheka.review.enabled"
    private static let firstSeenKey = "postitheka.review.firstSeenAt"
    private static let successCountKey = "postitheka.review.successCount"
    private static let lastCountedDayKey = "postitheka.review.lastCountedDay"
    private static let lastPromptKey = "postitheka.review.lastPromptAt"
    private static let milestoneKeyPrefix = "postitheka.review.milestone."

    public init(defaults: UserDefaults = .standard, calendar: Calendar = .current, now: @escaping () -> Date = Date.init) {
        self.defaults = defaults
        self.calendar = calendar
        self.now = now
        self.isEnabled = defaults.object(forKey: Self.enabledKey) as? Bool ?? true
        if defaults.object(forKey: Self.firstSeenKey) == nil {
            defaults.set(now(), forKey: Self.firstSeenKey)
        }
    }

    public var successCount: Int { defaults.integer(forKey: Self.successCountKey) }

    /// Something worked. Counts once per calendar day.
    public func recordSuccess() {
        let today = calendar.startOfDay(for: now())
        let lastCounted = defaults.object(forKey: Self.lastCountedDayKey) as? Date
        if lastCounted.map({ !calendar.isDate($0, inSameDayAs: today) }) ?? true {
            defaults.set(today, forKey: Self.lastCountedDayKey)
            defaults.set(successCount + 1, forKey: Self.successCountKey)
        }
        isPromptDue = shouldPrompt
    }

    /// A first-time or celebratory moment: asks without waiting for the count or
    /// the first day. Cooldown and the Settings switch still apply. With `once`,
    /// it fires at most one time per install; the key is spent when the moment
    /// happens, even if the switch or the cooldown kept it from asking.
    public func recordMilestone(once key: String? = nil) {
        if let key {
            let storageKey = Self.milestoneKeyPrefix + key
            guard !defaults.bool(forKey: storageKey) else { return }
            defaults.set(true, forKey: storageKey)
        }
        if isEnabled, !isCoolingDown { milestonePending = true }
        isPromptDue = shouldPrompt
    }

    /// The presenter's last check before asking. A pending milestone passes it
    /// without the count or the 24 hours, so the re-check cannot veto it.
    public var shouldPrompt: Bool {
        guard isEnabled, !isCoolingDown else { return false }
        if milestonePending { return true }
        guard successCount >= Self.successesBeforePrompt else { return false }
        if let firstSeen = defaults.object(forKey: Self.firstSeenKey) as? Date,
           now().timeIntervalSince(firstSeen) < Self.firstSessionGrace {
            return false
        }
        return true
    }

    private var isCoolingDown: Bool {
        guard let lastPrompt = defaults.object(forKey: Self.lastPromptKey) as? Date else { return false }
        return now().timeIntervalSince(lastPrompt) < Self.cooldown
    }

    /// The review was requested: start counting again from zero.
    public func didPrompt() {
        defaults.set(0, forKey: Self.successCountKey)
        defaults.set(now(), forKey: Self.lastPromptKey)
        milestonePending = false
        isPromptDue = false
    }

    /// The moment passed (screen left, app backgrounded): wait for the next success.
    public func skipPrompt() {
        isPromptDue = false
    }
}
