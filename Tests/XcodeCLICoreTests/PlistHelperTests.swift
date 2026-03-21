import Foundation
import Testing
@testable import XcodeCLICore

@Suite("PlistHelper")
struct PlistHelperTests {
    private func makeTempPaths(tmpDir: String) -> AgentPaths.Paths {
        AgentPaths.Paths(
            supportDir: tmpDir,
            socketPath: (tmpDir as NSString).appendingPathComponent("daemon.sock"),
            pidPath: (tmpDir as NSString).appendingPathComponent("daemon.pid"),
            logPath: (tmpDir as NSString).appendingPathComponent("agent.log"),
            plistPath: (tmpDir as NSString).appendingPathComponent("io.oozoofrog.xcodecli.plist")
        )
    }

    @Test("renderLaunchAgentPlist contains required keys and arguments")
    func renderContainsRequiredElements() {
        let tmpDir = NSTemporaryDirectory()
        let paths = makeTempPaths(tmpDir: tmpDir)
        let plist = renderLaunchAgentPlist(paths: paths, label: "io.oozoofrog.xcodecli", binaryPath: "/usr/local/bin/xcodecli")

        #expect(plist.contains("Label"))
        #expect(plist.contains("ProgramArguments"))
        #expect(plist.contains("RunAtLoad"))
        #expect(plist.contains("agent"))
        #expect(plist.contains("run"))
        #expect(plist.contains("--launch-agent"))
    }

    @Test("readLaunchAgentBinaryPathFromString extracts binary path from rendered plist")
    func parseExtractsBinaryPath() {
        let tmpDir = NSTemporaryDirectory()
        let paths = makeTempPaths(tmpDir: tmpDir)
        let plist = renderLaunchAgentPlist(paths: paths, label: "io.oozoofrog.xcodecli", binaryPath: "/usr/local/bin/xcodecli")

        let extracted = readLaunchAgentBinaryPathFromString(plist)
        #expect(extracted == "/usr/local/bin/xcodecli")
    }

    @Test("render then parse round-trip preserves binary path")
    func renderParseRoundTrip() {
        let binaryPath = "/usr/local/bin/xcodecli"
        let tmpDir = NSTemporaryDirectory()
        let paths = makeTempPaths(tmpDir: tmpDir)
        let plist = renderLaunchAgentPlist(paths: paths, label: "io.oozoofrog.xcodecli", binaryPath: binaryPath)
        let parsed = readLaunchAgentBinaryPathFromString(plist)

        #expect(parsed == binaryPath)
    }

    @Test("xmlEscape escapes all special XML characters")
    func xmlEscapeSpecialCharacters() {
        let result = xmlEscape("a&b<c>d\"e'f")
        #expect(result == "a&amp;b&lt;c&gt;d&quot;e&apos;f")
    }

    @Test("ensureLaunchAgentPlist returns changed=true on first write, changed=false on identical rewrite")
    func ensureIdempotent() throws {
        let tmpDir = (NSTemporaryDirectory() as NSString).appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(atPath: tmpDir) }
        try FileManager.default.createDirectory(atPath: tmpDir, withIntermediateDirectories: true)

        let paths = makeTempPaths(tmpDir: tmpDir)
        let label = "io.oozoofrog.xcodecli.test"
        let binary = "/usr/local/bin/xcodecli"

        let first = try ensureLaunchAgentPlist(paths: paths, label: label, binaryPath: binary)
        #expect(first.changed == true)

        let second = try ensureLaunchAgentPlist(paths: paths, label: label, binaryPath: binary)
        #expect(second.changed == false)
    }

    @Test("ensureLaunchAgentPlist returns changed=true and old binary when path changes")
    func ensureDetectsChange() throws {
        let tmpDir = (NSTemporaryDirectory() as NSString).appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(atPath: tmpDir) }
        try FileManager.default.createDirectory(atPath: tmpDir, withIntermediateDirectories: true)

        let paths = makeTempPaths(tmpDir: tmpDir)
        let label = "io.oozoofrog.xcodecli.test"
        let oldBinary = "/usr/local/bin/xcodecli-old"
        let newBinary = "/usr/local/bin/xcodecli-new"

        let first = try ensureLaunchAgentPlist(paths: paths, label: label, binaryPath: oldBinary)
        #expect(first.changed == true)

        let second = try ensureLaunchAgentPlist(paths: paths, label: label, binaryPath: newBinary)
        #expect(second.changed == true)
        #expect(second.registeredBinary == oldBinary)
    }
}
