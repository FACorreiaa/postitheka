import AuthenticationServices
import GoogleSignIn
import GoogleSignInSwift
import PostithekaKit
import SwiftUI

/// Sign in with Apple (AUTH-001) or Google (AUTH-002), or paste a token minted with
/// `server user create`: the operator's way in, kept for when both are off on a server.
struct SignInView: View {
    @Environment(AppModel.self) private var model
    /// The raw nonce of the sign-in in flight; the provider gets its hash.
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
                    VStack(spacing: 12) {
                        // Pills, like the rest of the form; the Form's row mask otherwise rounds
                        // the first button's top corners and the last one's bottom ones only.
                        SignInWithAppleButton(.signIn, onRequest: requestApple, onCompletion: finishApple)
                            .signInWithAppleButtonStyle(.white)
                            .frame(height: 48)
                            .clipShape(.capsule)
                        // Google's own button, so its logo and wording follow Google's rules.
                        GoogleSignInButton(scheme: .light, style: .wide, state: busy ? .disabled : .normal, action: signInWithGoogle)
                            .clipShape(.capsule)
                    }
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
        nonce = ProviderSignIn.makeNonce()
        request.requestedScopes = [.email]
        request.nonce = ProviderSignIn.hashed(nonce)
    }

    private func signInWithGoogle() {
        guard !busy, let presenter = Self.topViewController() else { return }
        let nonce = ProviderSignIn.makeNonce()
        Task {
            busy = true
            defer { busy = false }
            do {
                // The default scopes are openid, email and profile: all the server reads.
                let result = try await GIDSignIn.sharedInstance.signIn(
                    withPresenting: presenter, hint: nil, additionalScopes: nil, nonce: ProviderSignIn.hashed(nonce)
                )
                guard let idToken = result.user.idToken?.tokenString else {
                    error = "Google did not return a sign-in. Try again."
                    return
                }
                error = await model.signIn(with: .google, idToken: idToken, nonce: nonce)
                // Postitheka keeps its own token; Google's session is not needed after this.
                GIDSignIn.sharedInstance.signOut()
            } catch let failure as GIDSignInError where failure.code == .canceled {
                // Closing Google's sheet is not an error to show.
            } catch let failure {
                error = failure.localizedDescription
            }
        }
    }

    /// GoogleSignIn presents from a view controller; SwiftUI has none to hand.
    private static func topViewController() -> UIViewController? {
        let scene = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
            .first { $0.activationState == .foregroundActive }
        var top = scene?.keyWindow?.rootViewController
        while let presented = top?.presentedViewController { top = presented }
        return top
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
                error = await model.signIn(with: .apple, idToken: identityToken, nonce: nonce)
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
