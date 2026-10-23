// swift-tools-version: 6.0

// Runs swift-openapi-generator at a pinned version, so the Swift client is
// generated the same way on every machine (`task ios:gen`). The output is
// committed, as the Go and TypeScript code is: building the app needs no
// build plugin and no plugin trust prompt.
import PackageDescription

let package = Package(
    name: "openapi-tools",
    platforms: [.macOS(.v15)],
    dependencies: [
        .package(url: "https://github.com/apple/swift-openapi-generator", exact: "1.14.0"),
    ]
)
