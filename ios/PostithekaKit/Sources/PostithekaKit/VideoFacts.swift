import AVFoundation
import CoreGraphics
import Foundation

/// What a video file says about itself, as the server's upload request takes
/// it (VID-001). The server cannot measure a file, so the client reads these
/// before uploading. A value that could not be read stays nil, and the limit
/// that needs it is then not checked before publishing.
public struct VideoFacts: Sendable, Equatable {
    public var durationMs: Int?
    public var width: Int?
    public var height: Int?

    public init(durationMs: Int? = nil, width: Int? = nil, height: Int? = nil) {
        self.durationMs = durationMs
        self.width = width
        self.height = height
    }

    /// Keeps only values that mean something: an unknown length is not a
    /// number, and a file with no picture is 0 by 0. The size is the one the
    /// video is shown at, so a phone's portrait recording, stored sideways
    /// with a rotation, comes out taller than wide.
    public init(seconds: Double, naturalSize: CGSize, transform: CGAffineTransform) {
        let ms = (seconds * 1000).rounded()
        if ms.isFinite, ms > 0, ms < Double(Int32.max) { durationMs = Int(ms) }
        let shown = CGRect(origin: .zero, size: naturalSize).applying(transform)
        let w = Int(abs(shown.width).rounded()), h = Int(abs(shown.height).rounded())
        if w > 0, h > 0 {
            width = w
            height = h
        }
    }

    /// Reads a video file. Returns nothing known when it cannot be read.
    public static func read(from file: URL) async -> VideoFacts {
        let asset = AVURLAsset(url: file)
        let seconds = (try? await asset.load(.duration))?.seconds ?? .nan
        guard let track = try? await asset.loadTracks(withMediaType: .video).first,
              let (size, transform) = try? await track.load(.naturalSize, .preferredTransform)
        else { return VideoFacts(seconds: seconds, naturalSize: .zero, transform: .identity) }
        return VideoFacts(seconds: seconds, naturalSize: size, transform: transform)
    }
}
