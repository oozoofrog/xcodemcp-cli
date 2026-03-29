import Testing
import Foundation
import XcodeCLICore

/// JSON-decodable mirror of the MCPConfigResult type from the xcodecli executable.
/// Used to validate `mcp config --json` output without needing @testable import.
private struct MCPConfigOutput: Decodable {
    let client: String
    let mode: String
    let name: String
    let scope: String?
    let server: ServerSpec
    let command: [String]
    let displayCommand: String
    let warnings: [String]?
    let suggestedExecutablePath: String?
    let write: WriteResult

    struct ServerSpec: Decodable {
        let command: String
        let args: [String]
        let env: [String: String]
    }

    struct WriteResult: Decodable {
        let requested: Bool
        let executed: Bool
        let exitCode: Int
        let stdout: String
        let stderr: String
    }
}

@Suite("MCP Config")
struct MCPConfigTests {

    // MARK: - Config Subcommand Parsing

    @Test("mcp config --client codex --json produces valid JSON")
    func codexConfigJSON() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "codex", "--json"])
        #expect(result.exitCode == 0)
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.client == "codex")
        #expect(output.mode == "agent")
        #expect(output.name == "xcodecli")
        #expect((output.warnings ?? []).contains { $0.contains("Swift build output") || $0.contains("external volume") })
    }

    @Test("mcp config --client claude --json produces valid JSON")
    func claudeConfigJSON() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "claude", "--json"])
        #expect(result.exitCode == 0)
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.client == "claude")
        #expect(output.mode == "agent")
        #expect(output.name == "xcodecli")
    }

    @Test("mcp config --client gemini --json produces valid JSON")
    func geminiConfigJSON() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "gemini", "--json"])
        #expect(result.exitCode == 0)
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.client == "gemini")
        #expect(output.mode == "agent")
        #expect(output.name == "xcodecli")
    }

    // MARK: - Alias Commands

    @Test("mcp codex --json parses as codex alias (known broken: ConfigSubcommand.client has no default)")
    func codexAliasJSON() async throws {
        // CodexAlias creates ConfigSubcommand() then sets cmd.client = "codex",
        // but @Option without a default crashes on read before assignment.
        // This test documents the current broken behavior.
        let result = try await runCLI(["mcp", "codex", "--json"])
        #expect(result.exitCode != 0)
    }

    @Test("mcp claude --json parses as claude alias")
    func claudeAliasJSON() async throws {
        let result = try await runCLI(["mcp", "claude", "--json"])
        #expect(result.exitCode == 0)
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.client == "claude")
    }

    @Test("mcp gemini --json parses as gemini alias")
    func geminiAliasJSON() async throws {
        let result = try await runCLI(["mcp", "gemini", "--json"])
        #expect(result.exitCode == 0)
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.client == "gemini")
    }

    // MARK: - Default Scopes

    @Test("codex config has nil scope by default")
    func codexDefaultScope() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "codex", "--json"])
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.scope == nil)
    }

    @Test("claude config defaults to local scope")
    func claudeDefaultScope() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "claude", "--json"])
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.scope == "local")
    }

    @Test("gemini config defaults to user scope")
    func geminiDefaultScope() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "gemini", "--json"])
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.scope == "user")
    }

    // MARK: - Custom Options

    @Test("claude config with --scope user overrides default")
    func claudeScopeOverride() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "claude", "--scope", "user", "--json"])
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.scope == "user")
    }

    @Test("config with --name custom sets server name")
    func customServerName() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "codex", "--name", "myserver", "--json"])
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.name == "myserver")
    }

    @Test("config with --mode bridge uses bridge server args")
    func bridgeModeServerArgs() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "codex", "--mode", "bridge", "--json"])
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.mode == "bridge")
        #expect(output.server.args == ["bridge"])
    }

    @Test("config with --mode agent uses serve server args")
    func agentModeServerArgs() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "codex", "--mode", "agent", "--json"])
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.mode == "agent")
        #expect(output.server.args == ["serve"])
    }

    // MARK: - Environment Variables

    @Test("config with --xcode-pid sets MCP_XCODE_PID in env")
    func xcodePIDEnv() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "codex", "--xcode-pid", "12345", "--json"])
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.server.env["MCP_XCODE_PID"] == "12345")
    }

    @Test("config with --session-id sets MCP_XCODE_SESSION_ID in env")
    func sessionIDEnv() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "codex", "--session-id", "abc-123", "--json"])
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.server.env["MCP_XCODE_SESSION_ID"] == "abc-123")
    }

    @Test("config with no pid/session has empty env")
    func emptyEnvByDefault() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "codex", "--json"])
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.server.env.isEmpty)
    }

    @Test("config from debug build emits stable-path warnings")
    func unstableExecutableWarnings() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "codex", "--json"])
        let output = try decodeMCPConfig(result.stdout)
        let warnings = output.warnings ?? []
        #expect(!warnings.isEmpty)
        #expect(warnings.contains { $0.contains("Swift build output") || $0.contains("external volume") })
    }

    @Test("strict stable path fails from debug build")
    func strictStablePathFails() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "codex", "--strict-stable-path", "--json"])
        #expect(result.exitCode != 0)
        #expect(result.stderr.contains("unstable"))
    }

    // MARK: - Error Cases

    @Test("mcp config without --client fails")
    func missingClientFails() async throws {
        let result = try await runCLI(["mcp", "config", "--json"])
        #expect(result.exitCode != 0)
    }

    @Test("write result reports not requested when --write is absent")
    func writeNotRequestedByDefault() async throws {
        let result = try await runCLI(["mcp", "config", "--client", "codex", "--json"])
        let output = try decodeMCPConfig(result.stdout)
        #expect(output.write.requested == false)
        #expect(output.write.executed == false)
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
        let thisFile = URL(fileURLWithPath: #filePath)
        let packageRoot = thisFile
            .deletingLastPathComponent() // Tests/XcodeCLITests/
            .deletingLastPathComponent() // Tests/
            .deletingLastPathComponent() // package root
        return packageRoot.appendingPathComponent(".build/debug")
    }

    private func decodeMCPConfig(_ jsonString: String) throws -> MCPConfigOutput {
        let data = Data(jsonString.utf8)
        return try JSONDecoder().decode(MCPConfigOutput.self, from: data)
    }
}

private struct CLIResult {
    let stdout: String
    let stderr: String
    let exitCode: Int32
}
