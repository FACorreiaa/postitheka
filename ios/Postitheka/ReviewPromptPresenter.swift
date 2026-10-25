import PostithekaKit
import StoreKit
import SwiftUI

/// Asks iOS for the rating sheet about 1.5 seconds after a success makes a
/// prompt due, and only while this screen is on screen and the app is active.
/// Attach to the composer: `.reviewPrompt(model.reviewPrompter)`.
///
/// `isPaused` holds a due prompt while something else owns the screen: the
/// WhatsApp share sheet is presented from UIKit, so this view never disappears
/// under it, and the rating sheet would land on top of the handoff. The prompt
/// waits and asks once the share sheet has closed.
struct ReviewPromptPresenter: ViewModifier {
    let prompter: ReviewPrompter
    let isPaused: Bool
    @Environment(\.requestReview) private var requestReview
    @Environment(\.scenePhase) private var scenePhase
    @State private var isVisible = false

    /// Debug builds always show the sheet, which would cover the screen an
    /// XCUITest is driving. PublishFlowTests launches with this argument.
    private static let isUITesting = ProcessInfo.processInfo.arguments.contains("-ui-testing")

    func body(content: Content) -> some View {
        content
            .onAppear {
                isVisible = true
                // A prompt that came due while this screen was away has passed.
                if prompter.isPromptDue, !isPaused { prompter.skipPrompt() }
            }
            .onDisappear { isVisible = false }
            // Restarts (and cancels the wait) whenever due or paused changes.
            .task(id: prompter.isPromptDue && !isPaused) {
                guard prompter.isPromptDue, !isPaused, !Self.isUITesting else { return }
                do {
                    try await Task.sleep(for: .seconds(1.5))
                } catch {
                    return // paused again, or the screen went away; onAppear tidies up
                }
                guard isVisible, scenePhase == .active, prompter.shouldPrompt else {
                    prompter.skipPrompt()
                    return
                }
                prompter.didPrompt()
                requestReview()
            }
    }
}

extension View {
    func reviewPrompt(_ prompter: ReviewPrompter, pausedWhile isPaused: Bool = false) -> some View {
        modifier(ReviewPromptPresenter(prompter: prompter, isPaused: isPaused))
    }
}
