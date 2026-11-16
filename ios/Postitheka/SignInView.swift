import AuthenticationServices
import PostithekaKit
import SwiftUI

/// Sign in with Apple (AUTH-001), or paste a token minted with `server user create`: the
/// operator's way in, kept for when Apple sign-in is off on a server.
struct SignInView: View {
    @Environment(AppModel.self) private var model
    /// The raw nonce of the Apple sign-in in flight; Apple gets its hash.
    @State private var nonce = ""
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
                    SignInWithAppleButton(.signIn, onRequest: requestApple, onCompletion: finishApple)
                        .signInWithAppleButtonStyle(.white)
                        .frame(height: 48)
                        .disabled(busy)
                }
                .listRowBackground(Color.clear)
                .listRowInsets(EdgeInsets())

                Section {
                    SecureField("pk_…", text: $token)
                        .textContentType(.password)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                        .focused($typing)
                        .submitLabel(.go)
                        .onSubmit(signIn)
                } header: {
                    Text("Or an API token")
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

                // The pages are served by the same server the app talks to (PRV-001).
                if let base = model.baseURL {
                    HStack(spacing: 20) {
                        Link("Privacy", destination: base.appending(path: "privacy/"))
                        Link("Terms", destination: base.appending(path: "terms/"))
                    }
                    .font(.footnote)
                    .tint(Color.secondary)
                    .frame(maxWidth: .infinity)
                    .listRowBackground(Color.clear)
                }
            }
            .brandBackdrop()
            .scrollDismissesKeyboard(.interactively)
            .animation(.easeOut(duration: 0.2), value: typing)
        }
    }

    private func requestApple(_ request: ASAuthorizationAppleIDRequest) {
        nonce = AppleSignIn.makeNonce()
        request.requestedScopes = [.email]
        request.nonce = AppleSignIn.hashed(nonce)
    }

    private func finishApple(_ result: Result<ASAuthorization, any Error>) {
        switch result {
        case .success(let authorization):
            guard let credential = authorization.credential as? ASAuthorizationAppleIDCredential,
                  let data = credential.identityToken,
                  let identityToken = String(data: data, encoding: .utf8)
            else {
                error = "Apple did not return a sign-in. Try again."
                return
            }
            Task {
                busy = true
                error = await model.signInWithApple(identityToken: identityToken, nonce: nonce)
                busy = false
            }
        case .failure(let failure):
            // Closing Apple's sheet is not an error to show.
            if (failure as? ASAuthorizationError)?.code != .canceled {
                error = failure.localizedDescription
            }
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
                    mascot
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

    /// The 3D mascot on Debug and Beta builds (BRD-001), the flat one on the App Store's, where
    /// `Mascot3DView` is not compiled at all.
    @ViewBuilder private var mascot: some View {
        #if DEBUG || BETA
            Mascot3DView()
        #else
            Image("Mascot").resizable().scaledToFit()
        #endif
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
