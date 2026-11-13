import Foundation

/// How the 3D mascot moves (BRD-001). The numbers are `design/mascot-motion.json`, bundled by
/// the app; this type only turns a time into values. It knows no renderer: the app's
/// `Mascot3DView` applies what it returns, and `web/src/lib/mascot-motion.ts` is the same
/// arithmetic for the web. A change here is a change there, and both tests hold the same
/// expected numbers.
public struct MascotMotion: Decodable, Sendable {
    public enum Easing: String, Decodable, Sendable {
        case sine, linear

        /// 0 to 1 along one span between two keyframes.
        public func callAsFunction(_ progress: Double) -> Double {
            switch self {
            case .sine: 0.5 - 0.5 * cos(.pi * progress)
            case .linear: progress
            }
        }
    }

    public struct Keyframe: Decodable, Sendable {
        /// Fraction of the track's period, 0 to 1.
        public let t: Double
        public let v: Double
    }

    public enum Property: String, Decodable, Sendable {
        case translateY, rotateYDegrees
    }

    public struct Track: Decodable, Sendable {
        public let property: Property
        public let periodSeconds: Double
        public let easing: Easing
        public let keyframes: [Keyframe]

        /// The looping track's value at `seconds`.
        public func value(at seconds: Double) -> Double {
            guard let last = keyframes.last else { return 0 }
            let wrapped = seconds.truncatingRemainder(dividingBy: periodSeconds)
            let phase = (wrapped < 0 ? wrapped + periodSeconds : wrapped) / periodSeconds
            for (from, to) in zip(keyframes, keyframes.dropFirst()) where phase <= to.t {
                let span = to.t - from.t
                let progress = span > 0 ? (phase - from.t) / span : 1
                return from.v + (to.v - from.v) * easing(progress)
            }
            return last.v
        }
    }

    public struct Rest: Decodable, Sendable {
        public let rotateYDegrees: Double
    }

    /// Where the mascot starts, as offsets from rest, and how long it takes to get there.
    public struct Entrance: Decodable, Sendable {
        public let durationSeconds: Double
        public let translateY: Double
        public let scale: Double
    }

    /// The furthest it turns toward a pointer, and how quickly it follows. The web uses it; iOS
    /// has no pointer on the sign-in screen and keeps to the idle sway.
    public struct Pointer: Decodable, Sendable {
        public let yawDegrees: Double
        public let pitchDegrees: Double
        public let timeConstantSeconds: Double
    }

    /// Where the mascot looks, in degrees: yaw toward +X positive, pitch about X.
    public struct Look: Equatable, Sendable {
        public var yawDegrees = 0.0
        public var pitchDegrees = 0.0

        public init(yawDegrees: Double = 0, pitchDegrees: Double = 0) {
            self.yawDegrees = yawDegrees
            self.pitchDegrees = pitchDegrees
        }
    }

    /// The whole model's transform for one frame.
    public struct Pose: Equatable, Sendable {
        public let translateY: Double
        public let yawDegrees: Double
        public let pitchDegrees: Double
        public let scale: Double
    }

    public let version: Int
    public let rest: Rest
    public let idle: [Track]
    public let entrance: Entrance
    public let pointer: Pointer

    public init(json: Data) throws {
        self = try JSONDecoder().decode(MascotMotion.self, from: json)
    }

    /// How much of the entrance is still to go at `seconds`: 1 at the start, 0 once it has
    /// settled. Ease-out cubic, so it arrives quickly and lands softly.
    public func entranceRemaining(at seconds: Double) -> Double {
        let progress = min(1, max(0, seconds / entrance.durationSeconds))
        return pow(1 - progress, 3)
    }

    /// Where to look for a pointer at (`x`, `y`): each from -1 to 1 across the screen, right and
    /// up positive, held inside that range. Looking up is a negative pitch.
    public func pointerTarget(x: Double, y: Double) -> Look {
        let cx = min(1, max(-1, x))
        let cy = min(1, max(-1, y))
        return Look(yawDegrees: cx * pointer.yawDegrees, pitchDegrees: -cy * pointer.pitchDegrees)
    }

    /// One frame's step from `current` toward `target`: quick at first, never past it.
    public static func approach(_ current: Double, _ target: Double, delta: Double, timeConstant: Double) -> Double {
        guard timeConstant > 0 else { return target }
        return current + (target - current) * (1 - exp(-delta / timeConstant))
    }

    /// The pose `seconds` after the mascot appeared, looking at `look`: rest, plus the idle
    /// loops, plus what is left of the entrance, plus the look.
    public func pose(at seconds: Double, looking look: Look = Look()) -> Pose {
        var lift = 0.0
        var yaw = rest.rotateYDegrees + look.yawDegrees
        for track in idle {
            let value = track.value(at: seconds)
            switch track.property {
            case .translateY: lift += value
            case .rotateYDegrees: yaw += value
            }
        }
        let left = entranceRemaining(at: seconds)
        return Pose(
            translateY: lift + entrance.translateY * left,
            yawDegrees: yaw,
            pitchDegrees: look.pitchDegrees,
            scale: 1 + (entrance.scale - 1) * left
        )
    }
}
