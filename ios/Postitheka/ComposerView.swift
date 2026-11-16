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
                .brandRow()

                Section("Stamps") {
                    if model.accounts.isEmpty {
                        Text("Connect an account under Accounts to choose where this goes.")
                            .foregroundStyle(.secondary)
                    }
                    let brands = PostithekaKit.brands(of: composer.accounts)
                    if !brands.isEmpty {
                        Picker("Brand", selection: Binding(
                            get: { composer.brand },
                            set: { composer.brandChoice = $0 }
                        )) {
                            Text("All").tag(BrandFilter.all)
                            ForEach(brands, id: \.self) { Text($0).tag(BrandFilter.brand($0)) }
                        }
                        .tint(.accentColor)
                    }
                    ForEach(composer.shown, id: \.id) { account in
                        StampRow(account: account, composer: composer)
                        if composer.chosen.contains(where: { $0.id == account.id }) {
                            ForEach(composer.fields(for: account), id: \.key) { field in
                                OptionRow(field: field, accountID: account.id, composer: composer)
                            }
                        }
                    }
                    ForEach(ShareSheet.platforms, id: \.self) { platform in
                        AssistedStamp(platform: platform, composer: composer)
                    }
                }
                .brandRow()

                Section {
                    Toggle("Schedule", isOn: $scheduling.animation())
                        .tint(.accentColor)
                    if scheduling {
                        DatePicker("Send at", selection: Binding(
                            get: { composer.scheduledAt ?? .now.addingTimeInterval(3600) },
                            set: { composer.scheduledAt = $0 }
                        ), in: Date.now...)
                    }
                }
                .brandRow()

                if let reason = composer.waitingFor {
                    Section { Text(reason).font(.callout).foregroundStyle(.secondary) }.brandRow()
                }
                if let error = composer.error {
                    Section { Text(error).foregroundStyle(.red) }.brandRow()
                }
                ForEach(composer.issues(for: nil), id: \.self) { issue in
                    Text(issue.message).foregroundStyle(.red).brandRow()
                }
            }
            .brandBackdrop()
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
                ShareSheet.present(handoff) { sent in
                    Task {
                        if let post = await composer.finishHandoff(sent: sent) {
                            model.replace(post)
                            if sent { model.reviewPrompter.recordSuccess() }
                        }
                    }
                }
            }
            .reviewPrompt(model.reviewPrompter, pausedWhile: composer.pendingHandoff != nil)
            .onChange(of: picked) { _, items in
                guard !items.isEmpty else { return }
                picked = []
                Task {
                    for item in items {
                        if let media = try? await MediaImport.load(item) {
                            await composer.attach(file: media.file, contentType: media.contentType, size: media.size, video: media.video)
                        }
                    }
                }
            }
        }
    }
}

/// One thing a post can set for a single destination, under its stamp.
private struct OptionRow: View {
    let field: PostOption
    let accountID: String
    @Bindable var composer: Composer

    var body: some View {
        TextField(field.required ? field.label : "\(field.label) (optional)", text: Binding(
            get: { composer.options[accountID]?[field.key] ?? "" },
            set: { composer.options[accountID, default: [:]][field.key] = $0 }
        ))
        .font(.callout)
        .padding(.leading, 36) // under the stamp's name, past its checkmark
    }
}

private struct StampRow: View {
    let account: Account
    let composer: Composer

    var body: some View {
        let stale = account.needsReconnect
        let chosen = composer.selected.contains(account.id) && !stale
        let problems = composer.problems(for: account) + composer.issues(for: account).map(\.message)
        Button {
            composer.toggle(account)
        } label: {
            HStack(spacing: 12) {
                Image(systemName: chosen ? "checkmark.circle.fill" : "circle")
                    .foregroundStyle(chosen ? Color.accentColor : .secondary)
                VStack(alignment: .leading, spacing: 4) {
                    Text(account.display_name).foregroundStyle(Brand.cream)
                    Text(account.platform.displayName).font(.caption).foregroundStyle(Color.secondary)
                    if stale {
                        Text("Needs reconnecting. Open Accounts to connect it again.").font(.caption).foregroundStyle(.red)
                    }
                    if chosen {
                        ForEach(problems, id: \.self) { Text($0).font(.caption).foregroundStyle(.red) }
                    }
                }
            }
        }
        .disabled(stale)
        .accessibilityAddTraits(chosen ? .isSelected : [])
    }
}

/// An assisted platform's stamp is not an account: choosing it opens the share
/// sheet after Publish, and the user sends the post from that platform's app.
/// WhatsApp has no posting API; Reddit is by hand by choice (RDT-002).
private struct AssistedStamp: View {
    let platform: Platform
    @Bindable var composer: Composer

    var body: some View {
        let name = platform.displayName
        let chosen = composer.handTo.contains(platform)
        Button {
            if chosen { composer.handTo.remove(platform) } else { composer.handTo.insert(platform) }
        } label: {
            HStack(spacing: 12) {
                Image(systemName: chosen ? "checkmark.circle.fill" : "circle")
                    .foregroundStyle(chosen ? Color.accentColor : .secondary)
                VStack(alignment: .leading, spacing: 4) {
                    // Explicit colours: inside a Form's Button, .primary and .secondary
                    // resolve against the tint and every line would turn gold.
                    Text(name).foregroundStyle(Brand.cream)
                    Text(ShareSheet.isInstalled(platform) ? hint : "\(name) is not installed on this phone")
                        .font(.caption).foregroundStyle(Color.secondary)
                    if chosen && composer.scheduledAt != nil {
                        Text("Cannot be scheduled: it is handed over now").font(.caption).foregroundStyle(.red)
                    }
                }
            }
        }
        .accessibilityAddTraits(chosen ? .isSelected : [])
    }

    private var hint: String {
        switch platform {
        case .reddit: "You choose the community and title in Reddit"
        default: "You send it from \(platform.displayName)"
        }
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
            Brand.sheet.overlay { Image(systemName: "video").foregroundStyle(.secondary) }
        }
    }
}
