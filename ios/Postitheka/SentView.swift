import PostithekaKit
import SwiftUI

/// Every post with what happened at each destination. A published target gets
/// its postmark: the date and a link to where it landed.
struct SentView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        NavigationStack {
            List {
                if let error = model.loadError {
                    Text(error).foregroundStyle(.red)
                }
                ForEach(model.posts, id: \.id) { post in
                    PostRow(post: post)
                }
            }
            .overlay {
                if model.posts.isEmpty {
                    ContentUnavailableView("Nothing sent yet", systemImage: "tray",
                                           description: Text("Posts you publish show up here with each destination's outcome."))
                }
            }
            .navigationTitle("Sent")
            .refreshable { await model.refreshPosts() }
            // Polls only while something is on its way, like the web app.
            .task(id: model.hasPending) {
                while model.hasPending, !Task.isCancelled {
                    try? await Task.sleep(for: .seconds(2))
                    await model.refreshPosts()
                }
            }
        }
    }
}

private struct PostRow: View {
    let post: Post

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if !post.text.isEmpty {
                Text(post.text).lineLimit(4)
            }
            HStack(spacing: 4) {
                if !post.media_ids.isEmpty {
                    Label("\(post.media_ids.count)", systemImage: "photo")
                }
                Text(post.scheduled_at > .now ? "Scheduled for \(post.scheduled_at.formatted(date: .abbreviated, time: .shortened))"
                    : post.created_at.formatted(.relative(presentation: .named)))
            }
            .font(.caption)
            .foregroundStyle(.secondary)
            ForEach(post.targets, id: \.id) { target in
                TargetRow(target: target)
            }
        }
        .padding(.vertical, 4)
    }
}

private struct TargetRow: View {
    let target: Target

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Image(systemName: symbol).foregroundStyle(tint)
            VStack(alignment: .leading, spacing: 4) {
                HStack {
                    Text(target.platform.displayName)
                    Spacer()
                    Text(label).foregroundStyle(tint)
                }
                .font(.subheadline)
                if let error = target.last_error, target.status != .published {
                    Text(error).font(.caption).foregroundStyle(.secondary)
                }
                if let link = target.remote_url.flatMap(URL.init(string:)) {
                    Link("Open", destination: link).font(.caption)
                }
            }
        }
    }

    private var label: String {
        switch target.status {
        case .pending: target.last_error == nil ? "Queued" : "Retrying"
        case .publishing: "Sending"
        case .published: target.published_at.map { "Published \($0.formatted(date: .omitted, time: .shortened))" } ?? "Published"
        case .failed: "Not sent"
        case .handing_off: "On the phone"
        case .handed_off: target.published_at.map { "Sent \($0.formatted(date: .omitted, time: .shortened))" } ?? "Sent"
        }
    }

    private var symbol: String {
        switch target.status {
        case .pending: "clock"
        case .publishing: "paperplane"
        case .published, .handed_off: "checkmark.seal.fill"
        case .failed: "xmark.octagon.fill"
        case .handing_off: "iphone"
        }
    }

    private var tint: Color {
        switch target.status {
        case .pending, .publishing, .handing_off: .secondary
        case .published, .handed_off: .accentColor
        case .failed: .red
        }
    }
}
