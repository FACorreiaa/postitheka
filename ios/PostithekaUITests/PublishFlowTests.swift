import XCTest

/// Drives the app against a local server: sign in, see the connected
/// account, publish, and watch the outcome arrive in Sent. Needs a running
/// server and a token in POSTITHEKA_TOKEN (pass TEST_RUNNER_POSTITHEKA_TOKEN to
/// xcodebuild); without one it skips. The token is a local test token, never
/// committed.
final class PublishFlowTests: XCTestCase {
    @MainActor
    func testPublishFromTheComposer() throws {
        let env = ProcessInfo.processInfo.environment
        guard let token = env["POSTITHEKA_TOKEN"], !token.isEmpty else {
            throw XCTSkip("set TEST_RUNNER_POSTITHEKA_TOKEN to a local server token")
        }
        let account = env["POSTITHEKA_ACCOUNT"] ?? "UI test"
        let text = "Hello from the iOS composer \(Int(Date().timeIntervalSince1970))"

        let app = XCUIApplication()
        app.launchArguments = ["-ui-testing"] // no rating sheet over the flow
        app.launch()

        let field = app.secureTextFields.firstMatch
        if field.waitForExistence(timeout: 5) {
            snapshot(app, "1-sign-in")
            field.tap()
            field.typeText(token)
            app.buttons["Sign in"].tap()
        }

        XCTAssertTrue(app.tabBars.buttons["Accounts"].waitForExistence(timeout: 10), "did not reach the app")
        app.tabBars.buttons["Accounts"].tap()
        XCTAssertTrue(app.staticTexts[account].waitForExistence(timeout: 10), "account \(account) not listed")
        snapshot(app, "2-accounts")

        app.tabBars.buttons["Compose"].tap()
        let editor = app.textFields["What's happening?"].exists ? app.textFields["What's happening?"] : app.textViews.firstMatch
        XCTAssertTrue(editor.waitForExistence(timeout: 5))
        editor.tap()
        editor.typeText(text)
        app.buttons.containing(NSPredicate(format: "label CONTAINS %@", account)).firstMatch.tap()
        snapshot(app, "3-compose")
        let publish = app.navigationBars.buttons["Publish"]
        XCTAssertTrue(publish.isEnabled, "Publish is disabled")
        publish.tap()

        app.tabBars.buttons["Sent"].tap()
        XCTAssertTrue(app.staticTexts[text].waitForExistence(timeout: 10), "the post is not in Sent")
        let outcome = NSPredicate(format: "label BEGINSWITH 'Published' OR label == 'Not sent'")
        XCTAssertTrue(app.staticTexts.containing(outcome).firstMatch.waitForExistence(timeout: 30), "no outcome arrived")
        snapshot(app, "4-sent")
    }

    /// The simulator has no WhatsApp, so this closes the share sheet and checks
    /// the handoff is recorded as not sent: the honest outcome of a cancelled
    /// sheet.
    @MainActor
    func testWhatsAppHandoffClosedWithoutSending() throws {
        guard let token = ProcessInfo.processInfo.environment["POSTITHEKA_TOKEN"], !token.isEmpty else {
            throw XCTSkip("set TEST_RUNNER_POSTITHEKA_TOKEN to a local server token")
        }
        let text = "For the group \(Int(Date().timeIntervalSince1970))"
        let app = XCUIApplication()
        app.launchArguments = ["-ui-testing"] // no rating sheet over the flow
        app.launch()
        let field = app.secureTextFields.firstMatch
        if field.waitForExistence(timeout: 5) {
            field.tap()
            field.typeText(token)
            app.buttons["Sign in"].tap()
        }

        XCTAssertTrue(app.tabBars.buttons["Compose"].waitForExistence(timeout: 10))
        app.tabBars.buttons["Compose"].tap()
        let editor = app.textFields["What's happening?"].exists ? app.textFields["What's happening?"] : app.textViews.firstMatch
        XCTAssertTrue(editor.waitForExistence(timeout: 5))
        editor.tap()
        editor.typeText(text)
        app.buttons.containing(NSPredicate(format: "label CONTAINS 'WhatsApp'")).firstMatch.tap()
        snapshot(app, "5-whatsapp-stamp")
        app.navigationBars.buttons["Publish"].tap()

        // The share sheet's contents run in another process and iOS 26 gives it
        // no Close button, so it is dismissed the way a person would: a tap
        // above it. iOS reports that to the app as not completed.
        let gone = NSPredicate(format: "isHittable == false")
        expectation(for: gone, evaluatedWith: app.navigationBars.buttons["Publish"])
        waitForExpectations(timeout: 10)
        sleep(1)
        snapshot(app, "6-share-sheet")
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.12)).tap()

        app.tabBars.buttons["Sent"].tap()
        XCTAssertTrue(app.staticTexts[text].waitForExistence(timeout: 10))
        XCTAssertTrue(app.staticTexts["Not sent"].waitForExistence(timeout: 10), "the cancelled handoff was not recorded")
        snapshot(app, "7-handoff-not-sent")
    }

    @MainActor
    private func snapshot(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
