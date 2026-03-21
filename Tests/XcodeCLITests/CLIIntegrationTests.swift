import Testing
import Foundation

@Suite("CLI Integration")
struct CLIIntegrationTests {
    @Test("xcodecli version prints version line")
    func versionCommand() async throws {
        let result = try await runCLI(["version"])
        #expect(result.stdout.contains("xcodecli v"))
        #expect(result.exitCode == 0)
    }

    @Test("xcodecli --version prints version line")
    func versionFlag() async throws {
        let result = try await runCLI(["--version"])
        #expect(result.stdout.contains("xcodecli v"))
        #expect(result.exitCode == 0)
    }

    @Test("xcodecli --help exits 0")
    func helpExitsZero() async throws {
        let result = try await runCLI(["--help"])
        #expect(result.exitCode == 0)
    }

    @Test("xcodecli doctor --help exits 0")
    func doctorHelpExitsZero() async throws {
        let result = try await runCLI(["doctor", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("xcodecli agent status --help exits 0")
    func agentStatusHelpExitsZero() async throws {
        let result = try await runCLI(["agent", "status", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("xcodecli with unknown subcommand exits non-zero")
    func unknownExitsNonZero() async throws {
        let result = try await runCLI(["zzz-no-such-command"])
        #expect(result.exitCode != 0)
    }

    @Test("xcodecli bridge --help exits 0")
    func bridgeHelpExitsZero() async throws {
        let result = try await runCLI(["bridge", "--help"])
        #expect(result.exitCode == 0)
    }

    // MARK: - Helpers

    private func runCLI(_ arguments: [String]) async throws -> CLIResult {
        let binaryURL = productsDirectory().appendingPathComponent("xcodecli")
        let process = Process()
        process.executableURL = binaryURL
        process.arguments = arguments

        let stdoutPipe = Pipe()
        let stderrPipe = Pipe()
        process.standardOutput = stdoutPipe
        process.standardError = stderrPipe

        try process.run()
        process.waitUntilExit()

        let stdoutData = stdoutPipe.fileHandleForReading.readDataToEndOfFile()
        let stderrData = stderrPipe.fileHandleForReading.readDataToEndOfFile()

        return CLIResult(
            stdout: String(data: stdoutData, encoding: .utf8) ?? "",
            stderr: String(data: stderrData, encoding: .utf8) ?? "",
            exitCode: process.terminationStatus
        )
    }

    private func productsDirectory() -> URL {
        // SPM builds products into .build/debug/ or .build/release/
        // During `swift test`, the binary is at .build/debug/
        let thisFile = URL(fileURLWithPath: #filePath)
        let packageRoot = thisFile
            .deletingLastPathComponent() // Tests/XcodeCLITests/
            .deletingLastPathComponent() // Tests/
            .deletingLastPathComponent() // package root
        return packageRoot.appendingPathComponent(".build/debug")
    }
}

private struct CLIResult {
    let stdout: String
    let stderr: String
    let exitCode: Int32
}
