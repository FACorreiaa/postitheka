import PostithekaKit
import SwiftUI

/// Connected destinations. Disconnecting keeps the history of what was sent.
struct AccountsView: View {
    @Environment(AppModel.self) private var model
    @State private var connecting = false

    var body: some View {
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

                Section {
                    Button("Connect a Discord channel") { connecting = true }
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
}

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
