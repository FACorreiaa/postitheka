import AuthenticationServices
import PostithekaKit
import SwiftUI

/// Connected destinations. Disconnecting keeps the history of what was sent.
struct AccountsView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.webAuthenticationSession) private var webAuthentication
    @State private var connecting = false
    @State private var opening: Platform?
    @State private var outcome: ConnectOutcome?

    var body: some View {
        @Bindable var prompter = model.reviewPrompter
        NavigationStack {
            List {
                if let error = model.loadError {
                    Section { Text(error).foregroundStyle(.red) }
                }
                Section {
                    ForEach(model.accounts, id: \.id) { account in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(account.display_name)
                            Text(account.platform.displayName).font(.caption).foregroundStyle(.secondary)
                        }
                        .swipeActions {
                            Button("Disconnect", role: .destructive) {
                                Task { await model.disconnect(account) }
                            }
                        }
                    }
                } footer: {
                    if !model.accounts.isEmpty { Text("Swipe to disconnect. Posts already sent stay in Sent.") }
                }

                if let outcome {
                    Section {
                        switch outcome {
                        case .connected(let platform): Text("\(platform.displayName) connected.").foregroundStyle(.tint)
                        case .failed(let message): Text(message).foregroundStyle(.red)
                        }
                    }
                }

                Section {
                    ForEach(model.platforms.filter { $0.connect == .oauth }, id: \.platform) { info in
                        Button(opening == info.platform ? "Opening…" : "Connect \(info.platform.displayName)") {
                            Task { await connect(info.platform) }
                        }
                        .disabled(opening != nil)
                    }
                    Button("Connect a Discord channel") { connecting = true }
                } footer: {
                    if model.platforms.contains(where: { $0.connect == .oauth }) {
                        Text("You sign in on the platform’s own page; Postitheka never sees your password.")
                    }
                }

                // Our own server, not a destination: the header and footer keep it
                // apart from "Connect a Discord channel" above.
                Section {
                    Link(destination: discordURL) {
                        Label("Join us on Discord", systemImage: "bubble.left.and.bubble.right")
                    }
                } header: {
                    Text("Community")
                } footer: {
                    Text("Get help and share ideas")
                }

                Section {
                    Toggle("Ask me to rate Postitheka", isOn: $prompter.isEnabled)
                }

                Section {
                    Button("Sign out", role: .destructive) { model.signOut() }
                }
            }
            .overlay {
                if model.accounts.isEmpty && model.loadError == nil {
                    ContentUnavailableView("No accounts", systemImage: "person.2.slash",
                                           description: Text("Connect a destination to publish to it."))
                        .allowsHitTesting(false)
                }
            }
            .navigationTitle("Accounts")
            .sheet(isPresented: $connecting) { ConnectDiscordView() }
        }
    }

    private func connect(_ platform: Platform) async {
        opening = platform
        outcome = nil
        outcome = await model.connect(platform) { url in
            // Closing the page throws canceledLogin: the user changed their
            // mind, which needs no message.
            try? await webAuthentication.authenticate(using: url, callbackURLScheme: connectCallbackScheme)
        }
        opening = nil
    }
}

/// Invite from the #postitheka channel of our community server.
private let discordURL = URL(string: "https://discord.gg/nYhWNMRJC")!

private struct ConnectDiscordView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var webhookURL = ""
    @State private var name = ""
    @State private var error: String?
    @State private var busy = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("https://discord.com/api/webhooks/…", text: $webhookURL)
                        .keyboardType(.URL)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                } header: {
                    Text("Webhook URL")
                } footer: {
                    Text("In Discord: channel settings, Integrations, Webhooks, New Webhook, Copy Webhook URL.")
                }
                Section("Name (optional)") {
                    TextField("#announcements", text: $name)
                }
                if let error {
                    Section { Text(error).foregroundStyle(.red) }
                }
            }
            .navigationTitle("Connect Discord")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Connect") {
                        Task {
                            busy = true
                            error = await model.connectDiscord(webhookURL: webhookURL, name: name)
                            busy = false
                            if error == nil { dismiss() }
                        }
                    }
                    .disabled(webhookURL.isEmpty || busy)
                }
            }
        }
    }
}
