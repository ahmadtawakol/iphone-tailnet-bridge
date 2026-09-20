// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "iPhoneTailnetBridge",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "Tailbridge", targets: ["TailbridgeApp"]),
    ],
    targets: [
        .executableTarget(
            name: "TailbridgeApp",
            path: "macapp/Sources/TailbridgeApp"
        ),
    ]
)
