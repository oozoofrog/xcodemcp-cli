import Foundation
import Testing
@testable import XcodeCLICore

@Suite("BinaryIdentity")
struct BinaryIdentityTests {
    @Test("binaryIdentityForExecutable returns sha256 hash for existing file")
    func existingFileReturnsSHA256() throws {
        let tmpFile = (NSTemporaryDirectory() as NSString).appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(atPath: tmpFile) }
        try "hello world\n".write(toFile: tmpFile, atomically: true, encoding: .utf8)

        let identity = try binaryIdentityForExecutable(tmpFile)
        #expect(identity.hasPrefix("sha256:"))
        let hex = String(identity.dropFirst("sha256:".count))
        #expect(hex.count == 64)
        #expect(hex.allSatisfy { $0.isHexDigit })
    }

    @Test("binaryIdentityForExecutable returns path fallback for nonexistent file")
    func nonexistentFileReturnsPath() throws {
        let fakePath = "/tmp/nonexistent-\(UUID().uuidString)"
        let identity = try binaryIdentityForExecutable(fakePath)
        #expect(identity.hasPrefix("path:"))
        #expect(identity.contains(fakePath))
    }

    @Test("binaryIdentityForExecutable throws for empty string")
    func emptyPathThrows() {
        #expect(throws: XcodeCLIError.self) {
            try binaryIdentityForExecutable("")
        }
    }

    @Test("writeBinaryIdentity and readBinaryIdentity round-trip preserves identity")
    func writeReadRoundTrip() throws {
        let tmpFile = (NSTemporaryDirectory() as NSString).appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(atPath: tmpFile) }

        let identity = "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
        try writeBinaryIdentity(tmpFile, identity: identity)
        let read = try readBinaryIdentity(tmpFile)
        #expect(read == identity)
    }

    @Test("binaryIdentityPath returns supportDir joined with binary-id")
    func identityPathConstruction() {
        let paths = AgentPaths.Paths(
            supportDir: "/tmp/test-support",
            socketPath: "/tmp/test-support/daemon.sock",
            pidPath: "/tmp/test-support/daemon.pid",
            logPath: "/tmp/test-support/agent.log",
            plistPath: "/tmp/io.oozoofrog.xcodecli.plist"
        )
        let result = binaryIdentityPath(paths)
        #expect(result == "/tmp/test-support/binary-id")
    }
}
