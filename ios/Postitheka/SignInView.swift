import SwiftUI

/// Until Sign in with Apple (AUTH-001), a token minted with
/// `server user create` is pasted here, as on the web.
struct SignInView: View {
    @Environment(AppModel.self) private var model
    @State private var token = ""
    @State private var error: String?
    @State private var busy = false
    @FocusState private var typing: Bool

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    BrandHero(showsMascot: !typing)
                }
                .listRowBackground(Color.clear)
                .listRowSeparator(.hidden)

                Section {
                    SecureField("pk_…", text: $token)
                        .textContentType(.password)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                        .focused($typing)
                        .submitLabel(.go)
                        .onSubmit(signIn)
                } header: {
                    Text("API token")
                } footer: {
                    if let error {
                        Text(error).foregroundStyle(.red)
                    } else {
                        Text("Create one with server user create.")
                    }
                }
                .brandRow()

                Button(action: signIn) {
                    Group {
                        if busy { ProgressView() } else { Text("Sign in").fontWeight(.semibold) }
                    }
                    .frame(maxWidth: .infinity, minHeight: 32)
                }
                .buttonStyle(.borderedProminent)
                // The dark label is for the gold fill; disabled, the fill is grey.
                .foregroundStyle(token.isEmpty || busy ? Color.secondary : Brand.onGold)
                .disabled(token.isEmpty || busy)
                .listRowBackground(Color.clear)
                .listRowInsets(EdgeInsets())
            }
            .brandBackdrop()
            .scrollDismissesKeyboard(.interactively)
            .animation(.easeOut(duration: 0.2), value: typing)
        }
    }

    private func signIn() {
        guard !token.isEmpty, !busy else { return }
        Task {
            busy = true
            error = await model.signIn(token: token)
            busy = false
        }
    }
}

/// The key art, rebuilt: the mascot over its ring, the lockup, the tagline.
private struct BrandHero: View {
    /// Off while typing: with it, the keyboard covers the Sign in button.
    let showsMascot: Bool

    var body: some View {
        VStack(spacing: 16) {
            if showsMascot {
                VStack(spacing: 8) {
                    Image("Mascot")
                        .resizable()
                        .scaledToFit()
                        .frame(height: 176)
                        .accessibilityHidden(true)
                    FloorRing()
                }
                .transition(.opacity)
            }
            Image("Lockup")
                .resizable()
                .scaledToFit()
                .frame(height: 112)
                .accessibilityLabel("Postitheka")
            Text("Write once. Publish everywhere.")
                .italic()
                .font(.subheadline)
        }
        .frame(maxWidth: .infinity)
    }
}

/// The cyan ring the mascot hovers over on the key art's floor.
private struct FloorRing: View {
    var body: some View {
        Ellipse()
            .stroke(Brand.glow, lineWidth: 2)
            .frame(width: 88, height: 12)
            .shadow(color: Brand.glow.opacity(0.6), radius: 8)
            .accessibilityHidden(true)
    }
}
