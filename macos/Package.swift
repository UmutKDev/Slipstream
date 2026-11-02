// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "Gecit",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(
            name: "Gecit",
            path: "Sources/Gecit"
        )
    ]
)
