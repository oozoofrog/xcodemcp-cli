import ArgumentParser
import Foundation
import XcodeCLICore

struct MCPCommand: AsyncParsableCommand {
    static let configuration = CommandConfiguration(
        commandName: "mcp",
        abstract: "Print or write MCP client configuration",
        subcommands: [
            ConfigSubcommand.self,
            CodexAlias.self,
            ClaudeAlias.self,
            GeminiAlias.self,
        ]
    )

    struct ConfigSubcommand: AsyncParsableCommand {
        static let configuration = CommandConfiguration(
            commandName: "config",
            abstract: "Print or write a client-specific MCP registration command"
        )

        @Option(name: .long, help: "Target client preset: claude, codex, or gemini")
        var client: String

        @Option(name: .long, help: "MCP server mode: agent or bridge")
        var mode: String = "agent"

        @Option(name: .long, help: "Registered MCP server name")
        var name: String = "xcodecli"

        @Option(name: .long, help: "Scope: local, user, or project")
        var scope: String?

        @Flag(name: .long, help: "Execute the generated registration command")
        var write = false

        @Flag(name: .long, help: "Print a machine-readable plan/result object")
        var json = false

        @Flag(name: .customLong("strict-stable-path"), help: "Fail if the current xcodecli executable path looks unstable for long-lived MCP registration")
        var strictStablePath = false

        @Option(name: .customLong("xcode-pid"), help: "Include an explicit MCP_XCODE_PID override")
        var xcodePID: String?

        @Option(name: .customLong("session-id"), help: "Include an explicit MCP_XCODE_SESSION_ID override")
        var sessionID: String?

        func run() async throws {
            let executablePath = try resolveCurrentExecutablePath()
            var result = try buildMCPConfigResult(
                client: client, mode: mode, name: name,
                scope: resolveScope(client: client, scope: scope),
                xcodePID: xcodePID, sessionID: sessionID,
                executablePath: executablePath
            )

            try validateMCPConfigExecutablePath(
                executablePath: executablePath,
                strictStablePath: strictStablePath,
                advisory: MCPExecutableAdvisory(
                    warnings: result.warnings,
                    suggestedExecutablePath: result.suggestedExecutablePath
                )
            )

            if write {
                result.write = await performMCPConfigWrite(
                    client: client, name: name,
                    scope: result.scope ?? "",
                    result: result
                )
            }

            if json {
                try writePrettyJSON(result)
            } else {
                print(formatMCPConfigResult(result))
            }

            if write && (!result.write.executed || result.write.exitCode != 0) {
                throw ExitCode(1)
            }
        }
    }

    struct CodexAlias: AsyncParsableCommand {
        static let configuration = CommandConfiguration(
            commandName: "codex",
            abstract: "Alias for mcp config --client codex"
        )

        @Option(name: .long, help: "MCP server mode") var mode: String = "agent"
        @Option(name: .long, help: "Server name") var name: String = "xcodecli"
        @Flag(name: .long, help: "Execute command") var write = false
        @Flag(name: .long, help: "JSON output") var json = false
        @Flag(name: .customLong("strict-stable-path")) var strictStablePath = false
        @Option(name: .customLong("xcode-pid")) var xcodePID: String?
        @Option(name: .customLong("session-id")) var sessionID: String?

        func run() async throws {
            var cmd = ConfigSubcommand()
            cmd.client = "codex"
            cmd.mode = mode
            cmd.name = name
            cmd.write = write
            cmd.json = json
            cmd.strictStablePath = strictStablePath
            cmd.xcodePID = xcodePID
            cmd.sessionID = sessionID
            try await cmd.run()
        }
    }

    struct ClaudeAlias: AsyncParsableCommand {
        static let configuration = CommandConfiguration(
            commandName: "claude",
            abstract: "Alias for mcp config --client claude"
        )

        @Option(name: .long, help: "MCP server mode") var mode: String = "agent"
        @Option(name: .long, help: "Server name") var name: String = "xcodecli"
        @Option(name: .long, help: "Scope") var scope: String?
        @Flag(name: .long, help: "Execute command") var write = false
        @Flag(name: .long, help: "JSON output") var json = false
        @Flag(name: .customLong("strict-stable-path")) var strictStablePath = false
        @Option(name: .customLong("xcode-pid")) var xcodePID: String?
        @Option(name: .customLong("session-id")) var sessionID: String?

        func run() async throws {
            var cmd = ConfigSubcommand()
            cmd.client = "claude"
            cmd.mode = mode
            cmd.name = name
            cmd.scope = scope
            cmd.write = write
            cmd.json = json
            cmd.strictStablePath = strictStablePath
            cmd.xcodePID = xcodePID
            cmd.sessionID = sessionID
            try await cmd.run()
        }
    }

    struct GeminiAlias: AsyncParsableCommand {
        static let configuration = CommandConfiguration(
            commandName: "gemini",
            abstract: "Alias for mcp config --client gemini"
        )

        @Option(name: .long, help: "MCP server mode") var mode: String = "agent"
        @Option(name: .long, help: "Server name") var name: String = "xcodecli"
        @Option(name: .long, help: "Scope") var scope: String?
        @Flag(name: .long, help: "Execute command") var write = false
        @Flag(name: .long, help: "JSON output") var json = false
        @Flag(name: .customLong("strict-stable-path")) var strictStablePath = false
        @Option(name: .customLong("xcode-pid")) var xcodePID: String?
        @Option(name: .customLong("session-id")) var sessionID: String?

        func run() async throws {
            var cmd = ConfigSubcommand()
            cmd.client = "gemini"
            cmd.mode = mode
            cmd.name = name
            cmd.scope = scope
            cmd.write = write
            cmd.json = json
            cmd.strictStablePath = strictStablePath
            cmd.xcodePID = xcodePID
            cmd.sessionID = sessionID
            try await cmd.run()
        }
    }
}

// MARK: - Types

struct MCPConfigServerSpec: Codable {
    let command: String
    let args: [String]
    let env: [String: String]
}

struct MCPConfigWriteResult: Codable {
    var requested: Bool
    var executed: Bool
    var exitCode: Int
    var stdout: String
    var stderr: String

    static let notRequested = MCPConfigWriteResult(
        requested: false, executed: false, exitCode: 0, stdout: "", stderr: ""
    )
}

struct MCPConfigResult: Codable {
    let client: String
    let mode: String
    let name: String
    let scope: String?
    let server: MCPConfigServerSpec
    let command: [String]
    let displayCommand: String
    let warnings: [String]
    let suggestedExecutablePath: String?
    var write: MCPConfigWriteResult
}

// MARK: - Config Building

private func resolveScope(client: String, scope: String?) -> String {
    switch client.lowercased() {
    case "codex": return ""
    case "claude": return scope ?? "local"
    case "gemini": return scope ?? "user"
    default: return scope ?? ""
    }
}

private func resolveCurrentExecutablePath() throws -> String {
    // Try CommandLine.arguments[0] first
    let argv0 = CommandLine.arguments.first ?? ""
    if !argv0.isEmpty {
        let path: String
        if (argv0 as NSString).isAbsolutePath {
            path = (argv0 as NSString).standardizingPath
        } else if argv0.contains("/") {
            let cwd = FileManager.default.currentDirectoryPath
            path = ((cwd as NSString).appendingPathComponent(argv0) as NSString).standardizingPath
        } else {
            // Search PATH
            let pathDirs = (ProcessInfo.processInfo.environment["PATH"] ?? "").split(separator: ":").map(String.init)
            let found = pathDirs.first { dir in
                let full = (dir as NSString).appendingPathComponent(argv0)
                return FileManager.default.isExecutableFile(atPath: full)
            }
            if let found {
                path = (found as NSString).appendingPathComponent(argv0)
            } else {
                path = argv0
            }
        }
        return path
    }
    // Fallback to Bundle.main.executablePath
    if let execPath = Bundle.main.executablePath {
        return (execPath as NSString).standardizingPath
    }
    throw ValidationError("cannot resolve current executable path")
}

private func explicitMCPConfigEnv(xcodePID: String?, sessionID: String?) -> [String: String] {
    var env: [String: String] = [:]
    if let pid = xcodePID, !pid.isEmpty {
        env["MCP_XCODE_PID"] = pid
    }
    if let sid = sessionID, !sid.isEmpty {
        env["MCP_XCODE_SESSION_ID"] = sid
    }
    return env
}

private func buildMCPConfigResult(
    client: String, mode: String, name: String, scope: String,
    xcodePID: String?, sessionID: String?,
    executablePath: String
) throws -> MCPConfigResult {
    let advisory = mcpConfigExecutableAdvisory(executablePath: executablePath)
    let serverArgs = mode == "bridge" ? ["bridge"] : ["serve"]
    let server = MCPConfigServerSpec(
        command: executablePath,
        args: serverArgs,
        env: explicitMCPConfigEnv(xcodePID: xcodePID, sessionID: sessionID)
    )

    let invocation = try buildMCPConfigInvocation(
        client: client, name: name, scope: scope, server: server
    )
    let command = [invocation.name] + invocation.args

    return MCPConfigResult(
        client: client,
        mode: mode,
        name: name,
        scope: scope.isEmpty ? nil : scope,
        server: server,
        command: command,
        displayCommand: shellQuoteCommand(command),
        warnings: advisory.warnings,
        suggestedExecutablePath: advisory.suggestedExecutablePath,
        write: MCPConfigWriteResult(requested: false, executed: false, exitCode: 0, stdout: "", stderr: "")
    )
}

// MARK: - Per-Client Invocation Building

private struct CommandInvocation {
    let name: String
    let args: [String]
}

private struct MCPExecutableAdvisory {
    let warnings: [String]
    let suggestedExecutablePath: String?
}

private func validateMCPConfigExecutablePath(
    executablePath: String,
    strictStablePath: Bool,
    advisory: MCPExecutableAdvisory
) throws {
    guard strictStablePath, !advisory.warnings.isEmpty else { return }

    var message = "current executable path looks unstable for long-lived MCP registration (\(executablePath))."
    for warning in advisory.warnings {
        message += "\n- \(warning)"
    }
    if let suggested = advisory.suggestedExecutablePath, !suggested.isEmpty {
        message += "\nSuggested stable path: \(suggested)"
    }
    throw ValidationError(message)
}

private func buildMCPConfigInvocation(
    client: String, name: String, scope: String, server: MCPConfigServerSpec
) throws -> CommandInvocation {
    switch client.lowercased() {
    case "codex":
        var args = ["mcp", "add", name]
        args += envArgs(flagName: "--env", env: server.env)
        args += ["--", server.command]
        args += server.args
        return CommandInvocation(name: "codex", args: args)

    case "claude":
        let payload = try buildClaudeJSONPayload(server: server)
        return CommandInvocation(
            name: "claude",
            args: ["mcp", "add-json", "-s", scope, name, payload]
        )

    case "gemini":
        var args = ["mcp", "add", "-s", scope]
        args += envArgs(flagName: "-e", env: server.env)
        args += [name, server.command]
        args += server.args
        return CommandInvocation(name: "gemini", args: args)

    default:
        throw ValidationError("unsupported MCP client: \(client)")
    }
}

private func buildClaudeJSONPayload(server: MCPConfigServerSpec) throws -> String {
    struct ClaudePayload: Codable {
        let type: String
        let command: String
        let args: [String]
        let env: [String: String]?
    }
    let payload = ClaudePayload(
        type: "stdio",
        command: server.command,
        args: server.args,
        env: server.env.isEmpty ? nil : server.env
    )
    let encoder = JSONEncoder()
    encoder.outputFormatting = .sortedKeys
    let data = try encoder.encode(payload)
    return String(data: data, encoding: .utf8) ?? "{}"
}

private func envArgs(flagName: String, env: [String: String]) -> [String] {
    guard !env.isEmpty else { return [] }
    let sorted = env.keys.sorted()
    var args: [String] = []
    for key in sorted {
        args.append(flagName)
        args.append("\(key)=\(env[key]!)")
    }
    return args
}

// MARK: - Write Execution

private func performMCPConfigWrite(
    client: String, name: String, scope: String, result: MCPConfigResult
) async -> MCPConfigWriteResult {
    var writeResult = MCPConfigWriteResult(
        requested: true, executed: false, exitCode: 0, stdout: "", stderr: ""
    )

    let invocation: CommandInvocation
    do {
        invocation = try buildMCPConfigInvocation(
            client: client, name: name, scope: scope, server: result.server
        )
    } catch {
        writeResult.stderr = error.localizedDescription
        return writeResult
    }

    switch client.lowercased() {
    case "claude":
        // Try add-json, if "already exists" then remove + retry
        let (firstResult, firstErr) = await runInvocation(invocation)
        mergeWriteResult(&writeResult, run: firstResult, error: firstErr)
        if firstErr == nil && firstResult.exitCode == 0 { return writeResult }
        guard claudeAlreadyExists(run: firstResult, error: firstErr) else { return writeResult }

        let removeInvocation = CommandInvocation(
            name: "claude", args: ["mcp", "remove", "-s", scope, name]
        )
        let (removeResult, removeErr) = await runInvocation(removeInvocation)
        mergeWriteResult(&writeResult, run: removeResult, error: removeErr)
        if removeErr != nil { return writeResult }
        if removeResult.exitCode != 0 && !claudeRemoveNotFound(run: removeResult) { return writeResult }

        let (retryResult, retryErr) = await runInvocation(invocation)
        mergeWriteResult(&writeResult, run: retryResult, error: retryErr)
        return writeResult

    default:
        let (runResult, runErr) = await runInvocation(invocation)
        mergeWriteResult(&writeResult, run: runResult, error: runErr)
        return writeResult
    }
}

private struct ExternalCommandResult {
    var exitCode: Int
    var stdout: String
    var stderr: String
}

private func runInvocation(_ invocation: CommandInvocation) async -> (ExternalCommandResult, Error?) {
    let runner = SystemProcessRunner()
    do {
        // Look up the command on PATH
        let pathDirs = (ProcessInfo.processInfo.environment["PATH"] ?? "").split(separator: ":").map(String.init)
        let resolved = pathDirs
            .map { ($0 as NSString).appendingPathComponent(invocation.name) }
            .first { FileManager.default.isExecutableFile(atPath: $0) }
        guard let cmdPath = resolved else {
            return (ExternalCommandResult(exitCode: 1, stdout: "", stderr: "\(invocation.name) CLI not found on PATH"),
                    ValidationError("\(invocation.name) CLI not found on PATH"))
        }
        let result = try await runner.run(cmdPath, arguments: invocation.args)
        return (ExternalCommandResult(
            exitCode: Int(result.exitCode),
            stdout: result.stdout.trimmingCharacters(in: .whitespacesAndNewlines),
            stderr: result.stderr.trimmingCharacters(in: .whitespacesAndNewlines)
        ), nil)
    } catch {
        return (ExternalCommandResult(exitCode: 1, stdout: "", stderr: error.localizedDescription), error)
    }
}

private func mergeWriteResult(_ target: inout MCPConfigWriteResult, run: ExternalCommandResult, error: Error?) {
    if let error {
        target.stderr = joinOutput(target.stderr, error.localizedDescription)
        return
    }
    target.executed = true
    target.exitCode = run.exitCode
    target.stdout = joinOutput(target.stdout, run.stdout)
    target.stderr = joinOutput(target.stderr, run.stderr)
}

private func claudeAlreadyExists(run: ExternalCommandResult, error: Error?) -> Bool {
    if error != nil || run.exitCode == 0 { return false }
    let combined = (run.stdout + "\n" + run.stderr).lowercased()
    return combined.contains("already exists")
}

private func claudeRemoveNotFound(run: ExternalCommandResult) -> Bool {
    if run.exitCode == 0 { return false }
    let combined = (run.stdout + "\n" + run.stderr).lowercased()
    return combined.contains("no mcp server found")
}

private func joinOutput(_ existing: String, _ next: String) -> String {
    let a = existing.trimmingCharacters(in: .whitespacesAndNewlines)
    let b = next.trimmingCharacters(in: .whitespacesAndNewlines)
    if a.isEmpty { return b }
    if b.isEmpty { return a }
    return a + "\n" + b
}

// MARK: - Shell Quoting

private func shellQuoteCommand(_ argv: [String]) -> String {
    argv.map(mcpShellQuote).joined(separator: " ")
}

private func mcpShellQuote(_ value: String) -> String {
    if value.isEmpty { return "''" }
    let safe = value.allSatisfy { c in
        switch c {
        case "a"..."z", "A"..."Z", "0"..."9", "-", "_", ".", "/", ":", "=":
            return true
        default:
            return false
        }
    }
    if safe { return value }
    return "'" + value.replacingOccurrences(of: "'", with: "'\\''") + "'"
}

// MARK: - Text Formatting

private func formatMCPConfigResult(_ result: MCPConfigResult) -> String {
    var lines: [String] = []
    lines.append("client: \(result.client)")
    lines.append("mode: \(result.mode)")
    lines.append("name: \(result.name)")
    if let scope = result.scope, !scope.isEmpty {
        lines.append("scope: \(scope)")
    }
    lines.append("server: \(result.server.command) \(result.server.args.joined(separator: " "))")
    if result.server.env.isEmpty {
        lines.append("env: none")
    } else {
        lines.append("env:")
        for key in result.server.env.keys.sorted() {
            lines.append("  \(key)=\(result.server.env[key]!)")
        }
    }
    lines.append("command:")
    lines.append("  \(result.displayCommand)")
    if !result.warnings.isEmpty {
        lines.append("warnings:")
        lines.append(contentsOf: result.warnings.map { "  - \($0)" })
        if let suggested = result.suggestedExecutablePath, !suggested.isEmpty {
            lines.append("suggested executable path: \(suggested)")
        }
    }
    lines.append("write requested: \(result.write.requested)")
    if result.write.requested {
        lines.append("write executed: \(result.write.executed)")
        lines.append("write exit code: \(result.write.exitCode)")
        if !result.write.stdout.isEmpty {
            lines.append("write stdout:")
            for line in result.write.stdout.split(separator: "\n", omittingEmptySubsequences: false) {
                lines.append("  \(line)")
            }
        }
        if !result.write.stderr.isEmpty {
            lines.append("write stderr:")
            for line in result.write.stderr.split(separator: "\n", omittingEmptySubsequences: false) {
                lines.append("  \(line)")
            }
        }
    }
    return lines.joined(separator: "\n")
}

private func mcpConfigExecutableAdvisory(executablePath: String) -> MCPExecutableAdvisory {
    let standardized = (executablePath as NSString).standardizingPath
    let lower = standardized.lowercased()
    var warnings: [String] = []

    if !(standardized as NSString).isAbsolutePath {
        warnings.append("current executable path is relative; long-lived MCP registration is safer from an absolute installed path")
    }
    if lower.contains("/.build/") {
        warnings.append("current executable path looks like a Swift build output; rebuilds or clean operations can invalidate LaunchAgent registration")
    }
    if lower.contains("/cellar/") {
        warnings.append("current executable path points inside Homebrew Cellar; prefer the stable symlink path on PATH instead of a versioned Cellar path")
    }
    if lower.hasPrefix("/tmp/") || lower.hasPrefix("/private/tmp/") {
        warnings.append("current executable path is in a temporary directory; temporary paths are poor targets for long-lived MCP registration")
    }
    if lower.hasPrefix("/volumes/") {
        warnings.append("current executable path is on an external volume; for long-lived MCP usage prefer an internal stable install path")
    }

    let deduped = Array(NSOrderedSet(array: warnings)) as? [String] ?? warnings
    return MCPExecutableAdvisory(
        warnings: deduped,
        suggestedExecutablePath: deduped.isEmpty ? nil : suggestedStableExecutablePath(currentPath: standardized)
    )
}

private func suggestedStableExecutablePath(currentPath: String) -> String? {
    let candidates = [
        resolveExecutableOnPATH(named: "xcodecli"),
        "/opt/homebrew/bin/xcodecli",
        "/usr/local/bin/xcodecli",
        "\(NSHomeDirectory())/.local/bin/xcodecli",
    ]

    for candidate in candidates.compactMap({ $0 }) {
        let standardized = (candidate as NSString).standardizingPath
        if standardized == currentPath { continue }
        guard FileManager.default.isExecutableFile(atPath: standardized) else { continue }
        guard isPreferredStableExecutablePath(standardized) else { continue }
        return standardized
    }
    return nil
}

private func resolveExecutableOnPATH(named name: String) -> String? {
    let pathDirs = (ProcessInfo.processInfo.environment["PATH"] ?? "").split(separator: ":").map(String.init)
    for dir in pathDirs {
        let full = (dir as NSString).appendingPathComponent(name)
        if FileManager.default.isExecutableFile(atPath: full) {
            return (full as NSString).standardizingPath
        }
    }
    return nil
}

private func isPreferredStableExecutablePath(_ path: String) -> Bool {
    let lower = path.lowercased()
    if !(path as NSString).isAbsolutePath { return false }
    if lower.contains("/.build/") { return false }
    if lower.contains("/cellar/") { return false }
    if lower.hasPrefix("/tmp/") || lower.hasPrefix("/private/tmp/") { return false }
    if lower.hasPrefix("/volumes/") { return false }
    return true
}
