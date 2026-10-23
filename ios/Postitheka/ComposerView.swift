import PhotosUI
import PostithekaKit
import SwiftUI

/// Write once, choose the stamps, publish. Each stamp is a connected account;
/// it shows what its platform would object to while the post is written.
struct ComposerView: View {
    @Environment(AppModel.self) private var model
    @State private var picked: [PhotosPickerItem] = []
    @State private var scheduling = false
    @FocusState private var editing: Bool

    var body: some View {
        @Bindable var composer = model.composer
        NavigationStack {
            Form {
                Section {
                    TextField("What's happening?", text: $composer.text, axis: .vertical)
                        .lineLimit(5...12)
                        .focused($editing)
                    if !composer.attachments.isEmpty {
                        AttachmentStrip(composer: composer)
                    }
                    PhotosPicker(selection: $picked, maxSelectionCount: 10, matching: .any(of: [.images, .videos])) {
                        Label("Add photos or videos", systemImage: "photo.on.rectangle")
                    }
                } footer: {
                    Text("\(countText(composer.text)) characters")
                        .contentTransition(.numericText())
                }

                Section("Stamps") {
                    if model.accounts.isEmpty {
                        Text("Connect an account under Accounts to choose where this goes.")
                            .foregroundStyle(.secondary)
                    }
                    ForEach(model.accounts, id: \.id) { account in
                        StampRow(account: account, composer: composer)
                    }
                    WhatsAppStamp(composer: composer)
                }

                Section {
                    Toggle("Schedule", isOn: $scheduling.animation())
                    if scheduling {
                        DatePicker("Send at", selection: Binding(
                            get: { composer.scheduledAt ?? .now.addingTimeInterval(3600) },
                            set: { composer.scheduledAt = $0 }
                        ), in: Date.now...)
                    }
                }

                if let error = composer.error {
                    Section { Text(error).foregroundStyle(.red) }
                }
                ForEach(composer.issues(for: nil), id: \.self) { issue in
                    Text(issue.message).foregroundStyle(.red)
                }
            }
            .navigationTitle("Compose")
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button(scheduling ? "Schedule" : "Publish") {
                        editing = false
                        Task {
                            if let post = await composer.publish() {
                                model.published(post)
                                scheduling = false
                            }
                        }
                    }
                    .disabled(!composer.canPublish)
                }
            }
            .onChange(of: scheduling) { _, on in composer.scheduledAt = on ? .now.addingTimeInterval(3600) : nil }
            .onChange(of: composer.pendingHandoff?.id) { _, id in
                guard id != nil, let handoff = composer.pendingHandoff else { return }
                ShareSheet.present(text: handoff.text, files: handoff.files) { sent in
                    Task {
                        if let post = await composer.finishHandoff(sent: sent) { model.replace(post) }
                    }
                }
            }
            .onChange(of: picked) { _, items in
                guard !items.isEmpty else { return }
                picked = []
                Task {
                    for item in items {
                        if let media = try? await MediaImport.load(item) {
                            await composer.attach(file: media.file, contentType: media.contentType, size: media.size)
                        }
                    }
                }
            }
        }
    }
}

private struct StampRow: View {
    let account: Account
    let composer: Composer

    var body: some View {
        let chosen = composer.selected.contains(account.id)
        let problems = composer.problems(for: account) + composer.issues(for: account).map(\.message)
        Button {
            composer.toggle(account)
        } label: {
            HStack(spacing: 12) {
                Image(systemName: chosen ? "checkmark.circle.fill" : "circle")
                    .foregroundStyle(chosen ? Color.accentColor : .secondary)
                VStack(alignment: .leading, spacing: 4) {
                    Text(account.display_name).foregroundStyle(.primary)
                    Text(account.platform.displayName).font(.caption).foregroundStyle(.secondary)
                    if chosen {
                        ForEach(problems, id: \.self) { Text($0).font(.caption).foregroundStyle(.red) }
                    }
                }
            }
        }
        .accessibilityAddTraits(chosen ? .isSelected : [])
    }
}

/// WhatsApp has no posting API, so its stamp is not an account: choosing it
/// opens the share sheet after Publish, and the user sends it from WhatsApp.
private struct WhatsAppStamp: View {
    @Bindable var composer: Composer

    var body: some View {
        Button {
            composer.handToWhatsApp.toggle()
        } label: {
            HStack(spacing: 12) {
                Image(systemName: composer.handToWhatsApp ? "checkmark.circle.fill" : "circle")
                    .foregroundStyle(composer.handToWhatsApp ? Color.accentColor : .secondary)
                VStack(alignment: .leading, spacing: 4) {
                    Text("WhatsApp").foregroundStyle(.primary)
                    Text(ShareSheet.isWhatsAppInstalled ? "You send it from WhatsApp" : "WhatsApp is not installed on this phone")
                        .font(.caption).foregroundStyle(.secondary)
                    if composer.handToWhatsApp && composer.scheduledAt != nil {
                        Text("Cannot be scheduled: it is handed over now").font(.caption).foregroundStyle(.red)
                    }
                }
            }
        }
        .accessibilityAddTraits(composer.handToWhatsApp ? .isSelected : [])
    }
}

private struct AttachmentStrip: View {
    let composer: Composer

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(composer.attachments) { item in
                    ZStack(alignment: .topTrailing) {
                        Thumbnail(file: item.file, isVideo: item.contentType.hasPrefix("video/"))
                            .frame(width: 72, height: 72)
                            .clipShape(.rect(cornerRadius: 8))
                            .overlay {
                                if item.isUploading {
                                    ProgressView(value: item.progress).progressViewStyle(.circular)
                                } else if item.error != nil {
                                    Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.red)
                                }
                            }
                        Button { composer.remove(item.id) } label: {
                            Image(systemName: "xmark.circle.fill").symbolRenderingMode(.palette)
                                .foregroundStyle(.white, .black.opacity(0.6))
                        }
                        .buttonStyle(.plain)
                        .padding(4)
                        .accessibilityLabel("Remove")
                    }
                }
            }
        }
    }
}

private struct Thumbnail: View {
    let file: URL
    let isVideo: Bool

    var body: some View {
        if !isVideo, let image = UIImage(contentsOfFile: file.path) {
            Image(uiImage: image).resizable().scaledToFill()
        } else {
            Color(.secondarySystemBackground).overlay { Image(systemName: "video").foregroundStyle(.secondary) }
        }
    }
}
