import PostithekaKit
import UIKit

/// Presents the system share sheet for a handoff, from UIKit rather than a
/// SwiftUI sheet: UIActivityViewController is itself a sheet, and wrapping it
/// in another leaves an empty one behind it.
enum ShareSheet {
    /// The platforms a post can be handed to, in the order their stamps show.
    static let platforms: [Platform] = [.whatsapp, .reddit]

    /// WhatsApp's share extension.
    static let whatsAppActivity = "net.whatsapp.WhatsApp.ShareExtension"
    /// Reddit's app is com.reddit.Reddit (App Store lookup, 2026-11-15). An
    /// extension's identifier starts with its app's, so the prefix is matched:
    /// the extension's own name has not been read off a device.
    static let redditActivityPrefix = "com.reddit.Reddit."

    /// Whether a finished share went to `platform`'s own app. Only that counts
    /// as handed off; another app, or cancelling, does not.
    static func went(to platform: Platform, activity: String?, completed: Bool) -> Bool {
        guard completed, let activity else { return false }
        switch platform {
        case .whatsapp: return activity == whatsAppActivity
        case .reddit: return activity.hasPrefix(redditActivityPrefix)
        default: return false
        }
    }

    static func present(_ handoff: Handoff, completion: @escaping (_ sent: Bool) -> Void) {
        let items: [Any] = (handoff.text.isEmpty ? [] : [handoff.text]) + handoff.files
        let sheet = UIActivityViewController(activityItems: items, applicationActivities: nil)
        sheet.completionWithItemsHandler = { activity, completed, _, _ in
            completion(went(to: handoff.platform, activity: activity?.rawValue, completed: completed))
        }
        guard let top = topViewController() else { return completion(false) }
        // A second handoff becomes due the moment the first sheet reports, and
        // that sheet may still be sliding away: presenting over it fails
        // without a word, so wait for it to finish.
        if let leaving = top.presentedViewController, let transition = leaving.transitionCoordinator {
            transition.animate(alongsideTransition: nil) { _ in top.present(sheet, animated: true) }
        } else {
            top.present(sheet, animated: true)
        }
    }

    static func isInstalled(_ platform: Platform) -> Bool {
        let scheme = switch platform {
        case .whatsapp: "whatsapp://"
        case .reddit: "reddit://"
        default: ""
        }
        guard let url = URL(string: scheme) else { return false }
        return UIApplication.shared.canOpenURL(url)
    }

    /// The controller on top, not counting one that is on its way out.
    private static func topViewController() -> UIViewController? {
        let scene = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first
        var top = scene?.keyWindow?.rootViewController
        while let presented = top?.presentedViewController, !presented.isBeingDismissed { top = presented }
        return top
    }
}
