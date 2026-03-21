import Testing
@testable import XcodeCLICore

@Suite("AgentPaths")
struct AgentPathsTests {
    @Test("defaultPaths returns non-empty values for all fields")
    func defaultPathsNonEmpty() {
        let paths = AgentPaths.defaultPaths()
        #expect(!paths.supportDir.isEmpty)
        #expect(!paths.socketPath.isEmpty)
        #expect(!paths.pidPath.isEmpty)
        #expect(!paths.logPath.isEmpty)
        #expect(!paths.plistPath.isEmpty)
    }

    @Test("resolvePaths produces correct paths for a given home directory")
    func resolvePathsCorrectness() {
        let paths = AgentPaths.resolvePaths(homeDir: "/Users/testuser")
        #expect(paths.supportDir.contains("Application Support/xcodecli"))
        #expect(paths.socketPath.hasSuffix("daemon.sock"))
        #expect(paths.pidPath.hasSuffix("daemon.pid"))
        #expect(paths.logPath.hasSuffix("agent.log"))
    }

    @Test("socketPath, pidPath, logPath are all under supportDir")
    func pathsUnderSupportDir() {
        let paths = AgentPaths.resolvePaths(homeDir: "/Users/testuser")
        #expect(paths.socketPath.hasPrefix(paths.supportDir))
        #expect(paths.pidPath.hasPrefix(paths.supportDir))
        #expect(paths.logPath.hasPrefix(paths.supportDir))
    }

    @Test("plistPath is under LaunchAgents and ends with .plist")
    func plistPathLocation() {
        let paths = AgentPaths.resolvePaths(homeDir: "/Users/testuser")
        #expect(paths.plistPath.contains("Library/LaunchAgents"))
        #expect(paths.plistPath.hasSuffix(".plist"))
    }

    @Test("convenience accessors return same values as defaultPaths fields")
    func convenienceAccessorsMatchDefault() {
        let paths = AgentPaths.defaultPaths()
        #expect(AgentPaths.plistPath() == paths.plistPath)
        #expect(AgentPaths.socketPath() == paths.socketPath)
        #expect(AgentPaths.pidPath() == paths.pidPath)
        #expect(AgentPaths.logPath() == paths.logPath)
        #expect(AgentPaths.supportDir() == paths.supportDir)
    }
}
