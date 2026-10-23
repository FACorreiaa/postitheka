import SwiftUI

/// Until Sign in with Apple (AUTH-001), a token minted with
/// `server user create` is pasted here, as on the web.
struct SignInView: View {
    @Environment(AppModel.self) private var model
    @State private var token = ""
    @State private var error: String?
    @State private var busy = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    SecureField("pk_…", text: $token)
                        .textContentType(.password)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                } header: {
                    Text("API token")
                } footer: {
                    if let error {
                        Text(error).foregroundStyle(.red)
                    } else {
                        Text("Create one with server user create.")
                    }
                }

                Button {
                    Task {
                        busy = true
                        error = await model.signIn(token: token)
                        busy = false
                    }
                } label: {
                    if busy { ProgressView() } else { Text("Sign in") }
                }
                .disabled(token.isEmpty || busy)
            }
            .navigationTitle("Postitheka")
        }
    }
}
