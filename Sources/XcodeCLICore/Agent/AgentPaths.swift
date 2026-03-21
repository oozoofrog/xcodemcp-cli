import Foundation

public enum AgentPaths {
    public static let label = "io.oozoofrog.xcodecli"
    public static let supportDirName = "xcodecli"

    public struct Paths: Sendable {
        public let supportDir: String
        public let socketPath: String
        public let pidPath: String
        public let logPath: String
        public let plistPath: String
    }

    public static func defaultPaths() -> Paths {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        return resolvePaths(homeDir: home)
    }

    public static func resolvePaths(homeDir: String) -> Paths {
        resolveNamedPaths(homeDir: homeDir, supportDirName: supportDirName, label: label)
    }

    private static func resolveNamedPaths(homeDir: String, supportDirName: String, label: String) -> Paths {
        let supportDir = (homeDir as NSString).appendingPathComponent("Library/Application Support/\(supportDirName)")
        return Paths(
            supportDir: supportDir,
            socketPath: (supportDir as NSString).appendingPathComponent("daemon.sock"),
            pidPath: (supportDir as NSString).appendingPathComponent("daemon.pid"),
            logPath: (supportDir as NSString).appendingPathComponent("agent.log"),
            plistPath: (homeDir as NSString).appendingPathComponent("Library/LaunchAgents/\(label).plist")
        )
    }

    // MARK: - Convenience accessors (backward-compatible)

    public static func plistPath() -> String {
        defaultPaths().plistPath
    }

    public static func socketPath() -> String {
        defaultPaths().socketPath
    }

    public static func pidPath() -> String {
        defaultPaths().pidPath
    }

    public static func logPath() -> String {
        defaultPaths().logPath
    }

    public static func supportDir() -> String {
        defaultPaths().supportDir
    }
}
