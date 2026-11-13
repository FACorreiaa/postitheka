import PostithekaKit
import RealityKit
import SwiftUI

/// The mascot in 3D on the sign-in screen (BRD-001: a beta experiment; `brand/README.md`
/// "3D mascot"). It rises in, floats on the flat mascot's period and sways a little. The flat
/// mascot shows until the model's first frame, and stays under Reduce Motion, in Low Power
/// Mode, and when the model or its motion cannot be read.
///
/// Debug and Beta builds only: `Release.xcconfig` excludes this file, the model and the motion
/// file, and `ios/tools/release-proof.sh` checks that it did.
struct Mascot3DView: View {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var stage = MascotStage()

    private var draws: Bool {
        !reduceMotion && !ProcessInfo.processInfo.isLowPowerModeEnabled
    }

    var body: some View {
        ZStack {
            Image("Mascot")
                .resizable()
                .scaledToFit()
                .opacity(draws && stage.isLive ? 0 : 1)
            if draws {
                MascotScene(stage: stage)
                    .opacity(stage.isLive ? 1 : 0)
            }
        }
        .aspectRatio(MascotStage.aspect, contentMode: .fit)
        .animation(.easeOut(duration: 0.4), value: stage.isLive)
    }
}

/// The RealityKit view itself: a virtual camera, no AR. Its background is clear, so the mascot
/// stands on whatever SwiftUI draws behind it.
private struct MascotScene: View {
    let stage: MascotStage

    var body: some View {
        RealityView { content in
            content.camera = .virtual
            guard let pivot = await stage.load() else { return }
            content.add(pivot)
            for entity in stage.rig {
                content.add(entity)
            }
            stage.updates = content.subscribe(to: SceneEvents.Update.self) { event in
                stage.step(event.deltaTime)
            }
        }
        .allowsHitTesting(false)
    }
}

/// The loaded mascot and its motion. The model has no rig, so `MascotMotion`'s pose moves it
/// whole, through one pivot. The arithmetic is the web's (`web/src/lib/mascot3d/scene.ts`).
@MainActor @Observable
final class MascotStage {
    /// The flat mascot's shape, so the swap to the model does not move the layout.
    static let aspect: CGFloat = 718 / 646
    /// A frame that took longer (the app coming back) moves the mascot this far and no further.
    private static let longestStep = 0.1

    /// The first frame is drawn: the flat mascot can go.
    private(set) var isLive = false
    @ObservationIgnored var updates: EventSubscription?
    /// The camera and the lights, made with the mascot.
    @ObservationIgnored private(set) var rig: [Entity] = []

    @ObservationIgnored private var motion: MascotMotion?
    @ObservationIgnored private var pivot: Entity?
    @ObservationIgnored private var seconds = 0.0

    /// Reads the model and the motion from the bundle. Nil when either is missing (a build
    /// without `Mascot.usdz`, which is git-ignored): the flat mascot stays.
    func load() async -> Entity? {
        guard let url = Bundle.main.url(forResource: "mascot-motion", withExtension: "json"),
              let data = try? Data(contentsOf: url),
              let motion = try? MascotMotion(json: data),
              let model = try? await Entity(named: "Mascot", in: .main)
        else { return nil }
        // build_mascot3d.py makes the model one unit tall, standing on the origin, facing +Z.
        let pivot = Entity()
        pivot.addChild(model)
        self.motion = motion
        self.pivot = pivot
        rig = [
            camera(),
            light(.accent, intensity: 1500, from: [1.4, 1.8, 2.2]),
            light(.glow, intensity: 1200, from: [-1.6, 1.2, -2]),
            // A soft fill from the camera's side: RealityKit's virtual camera has no environment
            // light like the web's, and without it the front of the body falls into shadow.
            light(.cream, intensity: 700, from: [0, 0.6, 3]),
        ]
        step(0)
        return pivot
    }

    /// One frame: the pose `MascotMotion` gives for the time since the mascot appeared.
    func step(_ elapsed: TimeInterval) {
        guard let motion, let pivot else { return }
        let delta = min(elapsed, Self.longestStep)
        seconds += delta
        let pose = motion.pose(at: seconds)
        pivot.position = [0, Float(pose.translateY), 0]
        // Yaw, then pitch: the order the web composes them in.
        pivot.orientation = turn(pose.yawDegrees, about: [0, 1, 0]) * turn(pose.pitchDegrees, about: [1, 0, 0])
        pivot.scale = SIMD3(repeating: Float(pose.scale))
        if delta > 0 {
            isLive = true
        }
    }

    private func turn(_ degrees: Double, about axis: SIMD3<Float>) -> simd_quatf {
        simd_quatf(angle: Float(degrees * .pi / 180), axis: axis)
    }

    /// The web's camera: the mascot is one unit tall and stands on the origin.
    private func camera() -> Entity {
        let entity = Entity()
        var lens = PerspectiveCameraComponent()
        lens.fieldOfViewInDegrees = 30
        entity.components.set(lens)
        entity.look(at: [0, 0.5, 0], from: [0, 0.55, 2.4], relativeTo: nil)
        return entity
    }

    /// The key art's light: gold from the front, the hologram's cyan on the edge. Colours from
    /// the asset catalog, so they stay the palette's.
    private func light(_ colour: ColorResource, intensity: Float, from position: SIMD3<Float>) -> Entity {
        let entity = Entity()
        entity.components.set(DirectionalLightComponent(color: UIColor(resource: colour), intensity: intensity))
        entity.look(at: [0, 0.5, 0], from: position, relativeTo: nil)
        return entity
    }
}
