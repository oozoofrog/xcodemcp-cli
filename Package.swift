// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "xcodecli",
    platforms: [.macOS(.v15)],
    products: [
        .executable(name: "xcodecli", targets: ["xcodecli"]),
        .library(name: "XcodeCLICore", targets: ["XcodeCLICore"]),
    ],
    dependencies: [
        .package(url: "https://github.com/apple/swift-argument-parser.git", from: "1.5.0"),
    ],
    targets: [
        .executableTarget(
            name: "xcodecli",
            dependencies: [
                "XcodeCLICore",
                .product(name: "ArgumentParser", package: "swift-argument-parser"),
            ]
        ),
        .target(
            name: "XcodeCLICore",
            dependencies: []
        ),
        .target(
            name: "XcodeCLIBridge",
            dependencies: ["XcodeCLICore"]
        ),
        .testTarget(
            name: "XcodeCLICoreTests",
            dependencies: ["XcodeCLICore"]
        ),
        .testTarget(
            name: "XcodeCLITests",
            dependencies: [
                "xcodecli",
                "XcodeCLICore",
                .product(name: "ArgumentParser", package: "swift-argument-parser"),
            ]
        ),
    ]
)
