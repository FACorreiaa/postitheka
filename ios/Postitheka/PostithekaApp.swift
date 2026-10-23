import PostithekaKit
import SwiftUI

@main
struct PostithekaApp: App {
    @State private var model = AppModel()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(model)
        }
    }
}

struct RootView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        if model.baseURL == nil {
            ContentUnavailableView(
                "No server",
                systemImage: "network.slash",
                description: Text("This build has no server address. Set API_BASE_URL in its configuration.")
            )
        } else if model.signedIn {
            TabView {
                Tab("Compose", systemImage: "square.and.pencil") { ComposerView() }
                Tab("Sent", systemImage: "tray.full") { SentView() }
                Tab("Accounts", systemImage: "person.2") { AccountsView() }
            }
            .task { await model.refresh() }
        } else {
            SignInView()
        }
    }
}
