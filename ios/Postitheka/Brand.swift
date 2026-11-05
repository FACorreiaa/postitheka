import SwiftUI

/// The key art's palette; `brand/README.md` has the values and what each is for.
/// Gold is the asset catalog's accent colour, so it needs no name here. The app
/// is dark only (`UIUserInterfaceStyle`), so these have no light variants.
enum Brand {
    static let night = Color("Night")
    static let sheet = Color("Sheet")
    static let cream = Color("Cream")
    static let onGold = Color("OnGold")
    /// The hologram's cyan. One meaning, as on the web: on its way or landed.
    static let glow = Color("Glow")
}

extension View {
    /// A Form or List on the night page, instead of the system's grey.
    func brandBackdrop() -> some View {
        scrollContentBackground(.hidden).background(Brand.night)
    }

    /// `listRowBackground` does not inherit from the Form, so each Section asks for it.
    func brandRow() -> some View {
        listRowBackground(Brand.sheet)
    }
}
