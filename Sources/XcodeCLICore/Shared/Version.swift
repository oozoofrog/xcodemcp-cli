public enum Version {
    public static let source = "v1.0.2"

    /// Replaced at build time by scripts/build-swift.sh via sed.
    /// When building with `swift build` directly, stays equal to `source`.
    public static let current: String = source

    /// Build channel: "dev" for local builds, "release" for distribution.
    /// Replaced at build time by scripts/build-swift.sh via sed.
    public static let buildChannel: String = "dev"

    public static var isDev: Bool {
        buildChannel.lowercased().trimmingCharacters(in: .whitespaces) == "dev"
    }

    public static var line: String {
        var result = "xcodecli \(current)"
        if isDev {
            result += " (dev)"
        }
        return result
    }
}
