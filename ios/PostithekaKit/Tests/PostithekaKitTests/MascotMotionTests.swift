import Foundation
import Testing
@testable import PostithekaKit

/// The same expected numbers as `web/src/lib/mascot-motion.test.ts`: change one, change both.
/// Reads `design/mascot-motion.json` itself, the file both clients' copies must match.
struct MascotMotionTests {
    let motion: MascotMotion

    init() throws {
        let design = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent() // PostithekaKitTests
            .deletingLastPathComponent() // Tests
            .deletingLastPathComponent() // PostithekaKit
            .deletingLastPathComponent() // ios
            .deletingLastPathComponent() // repo
            .appendingPathComponent("design/mascot-motion.json")
        motion = try MascotMotion(json: Data(contentsOf: design))
    }

    private func close(_ a: Double, _ b: Double, _ digits: Double = 6) -> Bool {
        // vitest's toBeCloseTo(b, digits): |a - b| < 10^-digits / 2.
        abs(a - b) < pow(10, -digits) / 2
    }

    @Test func floatsOnTheFlatMascotsFiveSecondHover() {
        let float = motion.idle[0]
        #expect(close(float.value(at: 0), 0))
        #expect(close(float.value(at: 1.25), 0.015))
        #expect(close(float.value(at: 2.5), 0.03))
        #expect(close(float.value(at: 5), 0))
        // Time before the start wraps like any other.
        #expect(close(float.value(at: -1), 0.010365))
    }

    @Test func swaysSlowlyEitherSideOfItsRest() {
        let sway = motion.idle[1]
        #expect(close(sway.value(at: 0), -3))
        #expect(close(sway.value(at: 3), 0))
        #expect(close(sway.value(at: 7.5), 2.12132, 5))
    }

    @Test func risesInOnceAndSettles() {
        #expect(close(motion.entranceRemaining(at: 0), 1))
        #expect(close(motion.entranceRemaining(at: 0.3), 0.125))
        #expect(close(motion.entranceRemaining(at: 0.6), 0))
        #expect(close(motion.entranceRemaining(at: 10), 0))
    }

    @Test func turnsTowardThePointerNoFurtherThanItsLimits() {
        #expect(motion.pointerTarget(x: 1.5, y: -0.5) == .init(yawDegrees: 12, pitchDegrees: 3))
        let other = motion.pointerTarget(x: -0.5, y: 1)
        #expect(close(other.yawDegrees, -6))
        #expect(close(other.pitchDegrees, -6))
    }

    @Test func followsQuicklyAtFirstAndNeverOvershoots() {
        #expect(close(MascotMotion.approach(0, 1, delta: 0.4, timeConstant: 0.4), 0.632121))
        #expect(MascotMotion.approach(0, 1, delta: 100, timeConstant: 0.4) <= 1)
        #expect(MascotMotion.approach(0, 1, delta: 0.1, timeConstant: 0) == 1)
    }

    @Test func addsRestIdleEntranceAndLookIntoOnePose() {
        let pose = motion.pose(at: 0.3)
        #expect(close(pose.translateY, -0.005197))
        #expect(close(pose.yawDegrees, -14.963065))
        #expect(close(pose.pitchDegrees, 0))
        #expect(close(pose.scale, 0.995))

        let looking = motion.pose(at: 3, looking: .init(yawDegrees: 12, pitchDegrees: 3))
        #expect(close(looking.yawDegrees, 0))
        #expect(close(looking.pitchDegrees, 3))
        #expect(close(looking.scale, 1))
    }
}
