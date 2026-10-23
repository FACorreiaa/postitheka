// swift-tools-version: 6.0

// Everything in the iOS app that is not a view: the generated API client, the
// rule checks that mirror the server's, and the session that holds the token.
// A package so it is tested with `swift test`, without a simulator.
import PackageDescription

let package = Package(
    name: "PostithekaKit",
    platforms: [.iOS(.v18), .macOS(.v15)],
    products: [
        .library(name: "PostithekaKit", targets: ["PostithekaKit"]),
    ],
    dependencies: [
        .package(url: "https://github.com/apple/swift-openapi-runtime", exact: "1.13.0"),
        .package(url: "https://github.com/apple/swift-openapi-urlsession", exact: "1.3.2"),
    ],
    targets: [
        // Generated from api/openapi.yaml by `task ios:gen`; never edited by hand.
        .target(
            name: "PostithekaAPI",
            dependencies: [.product(name: "OpenAPIRuntime", package: "swift-openapi-runtime")]
        ),
        .target(
            name: "PostithekaKit",
            dependencies: [
                "PostithekaAPI",
                .product(name: "OpenAPIRuntime", package: "swift-openapi-runtime"),
                .product(name: "OpenAPIURLSession", package: "swift-openapi-urlsession"),
            ]
        ),
        .testTarget(name: "PostithekaKitTests", dependencies: ["PostithekaKit"]),
    ]
)
