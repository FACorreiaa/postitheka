import PostithekaKit
import SwiftUI

@main
struct PostithekaApp: App {
    @UIApplicationDelegateAdaptor private var appDelegate: AppDelegate
    @State private var model = AppModel()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(model)
        }
    }
}

final class AppDelegate: NSObject, UIApplicationDelegate {
    /// Navigation titles are drawn by UIKit, out of reach of SwiftUI's styles. Here and
    /// not in `App.init`: touching UIKit that early loses the catalog's accent colour,
    /// and every control in the app comes out system blue.
    func application(_ application: UIApplication,
                     didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool {
        let cream = UIColor(named: "Cream") ?? .label
        UINavigationBar.appearance().largeTitleTextAttributes = [.foregroundColor: cream]
        UINavigationBar.appearance().titleTextAttributes = [.foregroundColor: cream]
        return true
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
