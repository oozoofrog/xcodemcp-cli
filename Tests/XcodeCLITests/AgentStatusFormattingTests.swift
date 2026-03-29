import Testing
import Foundation
@testable import xcodecli
import XcodeCLICore

@Suite("Agent Status Formatting")
struct AgentStatusFormattingTests {

    @Test("formatAgentStatus includes warnings for relative registered binary paths")
    func relativeRegisteredBinaryWarns() {
        let status = AgentStatus(
            label: "io.oozoofrog.xcodecli",
            plistPath: "/tmp/io.oozoofrog.xcodecli.plist",
            plistInstalled: true,
            registeredBinary: "../Cellar/xcodecli/1.0.1/bin/xcodecli",
            currentBinary: "/opt/homebrew/bin/xcodecli",
            binaryPathMatches: false,
            socketPath: "/tmp/daemon.sock",
            socketReachable: false,
            running: false,
            pid: 0,
            idleTimeoutNs: 86_400_000_000_000,
            backendSessions: 0
        )

        let text = formatAgentStatus(status)
        #expect(text.contains("warnings:"))
        #expect(text.contains("relative"))
        #expect(text.contains("next steps:"))
        #expect(text.contains("agent uninstall"))
    }

    @Test("formatAgentStatus omits warning section when registration looks healthy")
    func healthyRegistrationOmitsWarnings() {
        let tempBinary = (NSTemporaryDirectory() as NSString).appendingPathComponent(UUID().uuidString)
        try? "binary".write(toFile: tempBinary, atomically: true, encoding: .utf8)
        try? FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: tempBinary)
        defer { try? FileManager.default.removeItem(atPath: tempBinary) }

        let status = AgentStatus(
            label: "io.oozoofrog.xcodecli",
            plistPath: "/tmp/io.oozoofrog.xcodecli.plist",
            plistInstalled: true,
            registeredBinary: tempBinary,
            currentBinary: tempBinary,
            binaryPathMatches: true,
            socketPath: "/tmp/daemon.sock",
            socketReachable: true,
            running: true,
            pid: 123,
            idleTimeoutNs: 86_400_000_000_000,
            backendSessions: 1
        )

        let text = formatAgentStatus(status)
        #expect(!text.contains("warnings:"))
    }
}
