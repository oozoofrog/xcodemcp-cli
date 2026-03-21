import Foundation
import Testing
@testable import XcodeCLICore

@Suite("BridgeEnv")
struct BridgeEnvTests {
    @Test("effective options prefer CLI overrides over base env")
    func effectiveOptionsOverrides() {
        let baseEnv = [BridgeEnvKey.xcodePID: "100"]
        let overrides = EnvOptions(xcodePID: "200")
        let result = EnvOptions.effective(baseEnv: baseEnv, overrides: overrides)
        #expect(result.xcodePID == "200")
    }

    @Test("applyOverrides merges PID and sessionID into env dict")
    func applyOverridesMerges() {
        let baseEnv = ["FOO": "bar"]
        let opts = EnvOptions(xcodePID: "42", sessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
        let merged = EnvOptions.applyOverrides(baseEnv: baseEnv, opts: opts)
        #expect(merged["FOO"] == "bar")
        #expect(merged[BridgeEnvKey.xcodePID] == "42")
        #expect(merged[BridgeEnvKey.sessionID] == "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
    }

    @Test("validate rejects non-numeric PID")
    func validateRejectsInvalidPID() {
        let opts = EnvOptions(xcodePID: "abc")
        #expect(throws: XcodeCLIError.self) {
            try opts.validate()
        }
    }

    @Test("validate rejects invalid UUID")
    func validateRejectsInvalidUUID() {
        let opts = EnvOptions(sessionID: "not-a-uuid")
        #expect(throws: XcodeCLIError.self) {
            try opts.validate()
        }
    }

    @Test("validate accepts valid PID and UUID")
    func validateAcceptsValidOptions() throws {
        let opts = EnvOptions(xcodePID: "123", sessionID: UUID().uuidString)
        try opts.validate()
    }

    @Test("parsePID rejects negative value")
    func parsePIDRejectsNegative() {
        #expect(throws: XcodeCLIError.self) {
            _ = try EnvOptions.parsePID("-1")
        }
    }

    @Test("parsePID rejects zero")
    func parsePIDRejectsZero() {
        #expect(throws: XcodeCLIError.self) {
            _ = try EnvOptions.parsePID("0")
        }
    }

    @Test("isValidUUID accepts standard UUID")
    func isValidUUIDAcceptsStandard() {
        #expect(EnvOptions.isValidUUID(UUID().uuidString) == true)
    }

    @Test("isValidUUID rejects garbage string")
    func isValidUUIDRejectsGarbage() {
        #expect(EnvOptions.isValidUUID("xyz") == false)
    }

    @Test("resolve creates and persists a new UUID when no session exists")
    func resolveCreatesAndPersistsUUID() throws {
        let tmpDir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString).path
        try FileManager.default.createDirectory(atPath: tmpDir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(atPath: tmpDir) }
        let sessionPath = (tmpDir as NSString).appendingPathComponent("session-id")

        let resolved = try SessionManager.resolve(
            baseEnv: [:],
            overrides: EnvOptions(),
            sessionPath: sessionPath
        )

        #expect(resolved.sessionSource == .generated)
        #expect(EnvOptions.isValidUUID(resolved.envOptions.sessionID))

        let fileExists = FileManager.default.fileExists(atPath: sessionPath)
        #expect(fileExists)

        let content = try String(contentsOfFile: sessionPath, encoding: .utf8)
            .trimmingCharacters(in: .whitespacesAndNewlines)
        #expect(EnvOptions.isValidUUID(content))
        #expect(content == resolved.envOptions.sessionID)
    }
}
