import CoreTransferable
import PhotosUI
import SwiftUI
import UniformTypeIdentifiers

/// A picked file, copied to the app's temporary directory so it can be
/// uploaded after the picker has gone.
nonisolated struct ImportedMedia: Sendable {
    let file: URL
    let contentType: String
    let size: Int64
}

nonisolated enum MediaImport {
    /// Photos come out as JPEG: the library's HEIC is not something most
    /// platforms accept. Videos keep their container; on-device conversion to
    /// each platform's limits is IOS-004.
    static func load(_ item: PhotosPickerItem) async throws -> ImportedMedia? {
        if item.supportedContentTypes.contains(where: { $0.conforms(to: .movie) }) {
            guard let movie = try await item.loadTransferable(type: PickedMovie.self) else { return nil }
            let type = UTType(filenameExtension: movie.url.pathExtension)?.preferredMIMEType ?? "video/mp4"
            return ImportedMedia(file: movie.url, contentType: type, size: fileSize(movie.url))
        }
        guard let data = try await item.loadTransferable(type: Data.self),
              let jpeg = UIImage(data: data)?.jpegData(compressionQuality: 0.9)
        else { return nil }
        let url = temporaryURL(extension: "jpg")
        try jpeg.write(to: url)
        return ImportedMedia(file: url, contentType: "image/jpeg", size: Int64(jpeg.count))
    }

    static func temporaryURL(extension ext: String) -> URL {
        FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString).appendingPathExtension(ext)
    }

    static func fileSize(_ url: URL) -> Int64 {
        (try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize).map(Int64.init) ?? 0
    }
}

nonisolated private struct PickedMovie: Transferable {
    let url: URL

    static var transferRepresentation: some TransferRepresentation {
        FileRepresentation(contentType: .movie) { movie in
            SentTransferredFile(movie.url)
        } importing: { received in
            let url = MediaImport.temporaryURL(extension: received.file.pathExtension)
            try FileManager.default.copyItem(at: received.file, to: url)
            return Self(url: url)
        }
    }
}
