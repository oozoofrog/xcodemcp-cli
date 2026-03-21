import Testing
import Foundation

@Suite("CLI Parsing")
struct CLIParsingTests {

    // MARK: - Version & Help

    @Test("version command exits 0 and stdout contains xcodecli")
    func versionCommand() async throws {
        let result = try runCLI(["version"])
        #expect(result.exitCode == 0)
        #expect(result.stdout.contains("xcodecli"))
    }

    @Test("--version flag exits 0 and stdout contains xcodecli")
    func versionFlag() async throws {
        let result = try runCLI(["--version"])
        #expect(result.exitCode == 0)
        #expect(result.stdout.contains("xcodecli"))
    }

    @Test("--help exits 0 and stdout contains USAGE")
    func helpCommand() async throws {
        let result = try runCLI(["--help"])
        #expect(result.exitCode == 0)
        #expect(result.stdout.contains("USAGE"))
    }

    // MARK: - Subcommand Help

    @Test("doctor --help exits 0 and mentions doctor")
    func doctorHelp() async throws {
        let result = try runCLI(["doctor", "--help"])
        #expect(result.exitCode == 0)
        #expect(result.stdout.lowercased().contains("doctor"))
    }

    @Test("serve --help exits 0")
    func serveHelp() async throws {
        let result = try runCLI(["serve", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("tools list --help exits 0")
    func toolsListHelp() async throws {
        let result = try runCLI(["tools", "list", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("tool inspect --help exits 0")
    func toolInspectHelp() async throws {
        let result = try runCLI(["tool", "inspect", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("tool call --help exits 0")
    func toolCallHelp() async throws {
        let result = try runCLI(["tool", "call", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("agent status --help exits 0")
    func agentStatusHelp() async throws {
        let result = try runCLI(["agent", "status", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("agent run --help exits 0")
    func agentRunHelp() async throws {
        let result = try runCLI(["agent", "run", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("agent guide --help exits 0")
    func agentGuideHelp() async throws {
        let result = try runCLI(["agent", "guide", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("agent demo --help exits 0")
    func agentDemoHelp() async throws {
        let result = try runCLI(["agent", "demo", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("mcp config --help exits 0")
    func mcpConfigHelp() async throws {
        let result = try runCLI(["mcp", "config", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("mcp codex --help exits 0")
    func mcpCodexHelp() async throws {
        let result = try runCLI(["mcp", "codex", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("mcp claude --help exits 0")
    func mcpClaudeHelp() async throws {
        let result = try runCLI(["mcp", "claude", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("mcp gemini --help exits 0")
    func mcpGeminiHelp() async throws {
        let result = try runCLI(["mcp", "gemini", "--help"])
        #expect(result.exitCode == 0)
    }

    @Test("update --help exits 0")
    func updateHelp() async throws {
        let result = try runCLI(["update", "--help"])
        #expect(result.exitCode == 0)
    }

    // MARK: - Error Cases

    @Test("unknown command exits non-zero")
    func unknownCommandFails() async throws {
        let result = try runCLI(["nonexistent"])
        #expect(result.exitCode != 0)
    }

    @Test("agent run without --launch-agent exits non-zero")
    func agentRunWithoutFlag() async throws {
        let result = try runCLI(["agent", "run"])
        #expect(result.exitCode != 0)
    }

    @Test("tool call without --json or --json-stdin exits non-zero")
    func toolCallRejectsNoInput() async throws {
        let result = try runCLI(["tool", "call", "TestTool"])
        #expect(result.exitCode != 0)
    }

    // MARK: - Helpers

    private func runCLI(_ arguments: [String]) throws -> CLIRunResult {
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

        return CLIRunResult(
            stdout: String(data: stdoutData, encoding: .utf8) ?? "",
            stderr: String(data: stderrData, encoding: .utf8) ?? "",
            exitCode: process.terminationStatus
        )
    }

    private func productsDirectory() -> URL {
        let thisFile = URL(fileURLWithPath: #filePath)
        let packageRoot = thisFile
            .deletingLastPathComponent() // Tests/XcodeCLITests/
            .deletingLastPathComponent() // Tests/
            .deletingLastPathComponent() // package root
        return packageRoot.appendingPathComponent(".build/debug")
    }
}

private struct CLIRunResult {
    let stdout: String
    let stderr: String
    let exitCode: Int32
}
