import PostithekaKit
import SwiftUI

/// Every post with what happened at each destination. A published target gets
/// its postmark: the date and a link to where it landed.
struct SentView: View {
    @Environment(AppModel.self) private var model
    @State private var cancelling: Post?
    @State private var moving: Post?
    @State private var actionError: String?

    var body: some View {
        NavigationStack {
            List {
                if let error = model.loadError {
                    Text(error).foregroundStyle(.red).brandRow()
                }
                ForEach(model.posts, id: \.id) { post in
                    PostRow(post: post).brandRow()
                        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                            // Only while nothing has gone out; the server refuses otherwise.
                            if post.isUnsent {
                                Button("Cancel", systemImage: "trash", role: .destructive) { cancelling = post }
                                Button("Reschedule", systemImage: "calendar") { moving = post }
                                    .tint(Brand.glow)
                            }
                        }
                }
            }
            .brandBackdrop()
            .overlay {
                if model.posts.isEmpty {
                    ContentUnavailableView {
                        VStack(spacing: 16) {
                            Image("Mascot")
                                .resizable()
                                .scaledToFit()
                                .frame(height: 136)
                                .accessibilityHidden(true)
                            Text("Nothing sent yet")
                        }
                    } description: {
                        Text("Posts you publish show up here with each destination's outcome.")
                    }
                }
            }
            .navigationTitle("Sent")
            .confirmationDialog(
                "Cancel this post?", isPresented: .init(get: { cancelling != nil }, set: { if !$0 { cancelling = nil } }),
                titleVisibility: .visible, presenting: cancelling
            ) { post in
                Button("Cancel Post", role: .destructive) {
                    Task { actionError = await model.cancel(post) }
                }
                Button("Keep Post", role: .cancel) {}
            } message: { _ in
                Text("This removes the post. It will not be sent.")
            }
            .sheet(isPresented: .init(get: { moving != nil }, set: { if !$0 { moving = nil } })) {
                if let post = moving {
                    RescheduleSheet(post: post) { date in await model.reschedule(post, to: date) }
                }
            }
            .alert("That did not work", isPresented: .init(get: { actionError != nil }, set: { if !$0 { actionError = nil } })) {
                Button("OK", role: .cancel) {}
            } message: {
                Text(actionError ?? "")
            }
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

/// Picks a new time for a post that has not started sending.
private struct RescheduleSheet: View {
    let post: Post
    /// Returns what went wrong, or nil when the post moved.
    let save: (Date) async -> String?

    @Environment(\.dismiss) private var dismiss
    @State private var date: Date
    @State private var saving = false
    @State private var error: String?

    init(post: Post, save: @escaping (Date) async -> String?) {
        self.post = post
        self.save = save
        _date = State(initialValue: max(post.scheduled_at, .now))
    }

    var body: some View {
        NavigationStack {
            Form {
                DatePicker("Publish at", selection: $date, in: Date.now...)
                if let error {
                    Text(error).foregroundStyle(.red)
                }
            }
            .navigationTitle("Reschedule")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Keep Time") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        Task {
                            saving = true
                            error = await save(date)
                            saving = false
                            if error == nil { dismiss() }
                        }
                    }
                    .disabled(saving)
                }
            }
        }
        .presentationDetents([.medium])
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
                if target.status == .drafted {
                    // Nothing is public until its author posts it from the platform's app.
                    Text("Open the notification in the \(target.platform.displayName) app to add a caption and post it.")
                        .font(.caption).foregroundStyle(.secondary)
                }
                if let link = target.remote_url.flatMap(URL.init(string:)) {
                    Link("Open", destination: link).font(.caption)
                }
                if let line = commentLine(target) {
                    if target.comment_status == .posted, let link = target.comment_url.flatMap(URL.init(string:)) {
                        Link(line.text, destination: link).font(.caption)
                    } else {
                        Text(line.text).font(.caption).foregroundStyle(line.failed ? Color.red : Color.secondary)
                    }
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
        case .drafted: target.published_at.map { "Draft \($0.formatted(date: .omitted, time: .shortened))" } ?? "Draft"
        }
    }

    private var symbol: String {
        switch target.status {
        case .pending: "clock"
        case .publishing: "paperplane"
        case .published, .handed_off: "checkmark.seal.fill"
        case .failed: "xmark.octagon.fill"
        case .handing_off: "iphone"
        case .drafted: "tray.and.arrow.down"
        }
    }

    private var tint: Color {
        switch target.status {
        case .pending, .publishing, .handing_off: .secondary
        case .published, .handed_off: Brand.glow
        case .drafted: Brand.cream
        case .failed: .red
        }
    }
}
