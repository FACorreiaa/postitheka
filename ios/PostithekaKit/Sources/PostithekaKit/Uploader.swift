import Foundation

/// Sends a file straight to storage with the presigned URL the API returned.
/// The bytes never pass through the API.
public protocol Uploader: Sendable {
    func upload(file: URL, to upload: Upload, progress: @escaping @Sendable (Double) -> Void) async throws
}

public struct URLSessionUploader: Uploader {
    private let session: URLSession

    public init(session: URLSession = .shared) {
        self.session = session
    }

    public func upload(file: URL, to upload: Upload, progress: @escaping @Sendable (Double) -> Void) async throws {
        guard let url = URL(string: upload.upload_url) else { throw APIError.message("The upload address is not valid.") }
        var request = URLRequest(url: url)
        request.httpMethod = upload.method.rawValue
        for (name, value) in upload.headers.additionalProperties {
            request.setValue(value, forHTTPHeaderField: name)
        }
        let (_, response) = try await session.upload(for: request, fromFile: file, delegate: ProgressDelegate(progress))
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            let code = (response as? HTTPURLResponse)?.statusCode ?? 0
            throw APIError.message("Upload failed (\(code)).")
        }
        progress(1)
    }
}

private final class ProgressDelegate: NSObject, URLSessionTaskDelegate, Sendable {
    let progress: @Sendable (Double) -> Void

    init(_ progress: @escaping @Sendable (Double) -> Void) {
        self.progress = progress
    }

    func urlSession(_: URLSession, task _: URLSessionTask, didSendBodyData _: Int64, totalBytesSent: Int64, totalBytesExpectedToSend: Int64) {
        guard totalBytesExpectedToSend > 0 else { return }
        progress(Double(totalBytesSent) / Double(totalBytesExpectedToSend))
    }
}
