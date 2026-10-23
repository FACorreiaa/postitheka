import UIKit

/// Presents the system share sheet for a handoff, from UIKit rather than a
/// SwiftUI sheet: UIActivityViewController is itself a sheet, and wrapping it
/// in another leaves an empty one behind it.
enum ShareSheet {
    /// WhatsApp's share extension. Only a share sent through it counts as
    /// handed off; anything else (another app, or cancelling) is not.
    static let whatsAppActivity = "net.whatsapp.WhatsApp.ShareExtension"

    static func present(text: String, files: [URL], completion: @escaping (_ sentToWhatsApp: Bool) -> Void) {
        let items: [Any] = (text.isEmpty ? [] : [text]) + files
        let sheet = UIActivityViewController(activityItems: items, applicationActivities: nil)
        sheet.completionWithItemsHandler = { activity, completed, _, _ in
            completion(completed && activity?.rawValue == whatsAppActivity)
        }
        topViewController()?.present(sheet, animated: true)
    }

    static var isWhatsAppInstalled: Bool {
        UIApplication.shared.canOpenURL(URL(string: "whatsapp://")!)
    }

    private static func topViewController() -> UIViewController? {
        let scene = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first
        var top = scene?.keyWindow?.rootViewController
        while let presented = top?.presentedViewController { top = presented }
        return top
    }
}
