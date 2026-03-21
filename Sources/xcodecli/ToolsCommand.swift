import ArgumentParser
import Foundation
import XcodeCLICore

struct ToolsCommand: AsyncParsableCommand {
    static let configuration = CommandConfiguration(
        commandName: "tools",
        abstract: "Convenience commands for listing tools",
        subcommands: [ListSubcommand.self]
    )

    struct ListSubcommand: AsyncParsableCommand {
        static let configuration = CommandConfiguration(
            commandName: "list",
            abstract: "List MCP tools exposed through xcrun mcpbridge"
        )

        @Flag(name: .long, help: "Print the flattened tools array as pretty JSON")
        var json = false

        @Option(name: .customLong("timeout"), help: "Override the request timeout")
        var timeout: Int = 60

        @Option(name: .customLong("xcode-pid"), help: "Override MCP_XCODE_PID")
        var xcodePID: String?

        @Option(name: .customLong("session-id"), help: "Override MCP_XCODE_SESSION_ID")
        var sessionID: String?

        @Flag(name: .customLong("debug"), help: "Emit debug logs to stderr")
        var debug = false

        func run() async throws {
            let env = envDictionary()
            let (effective, _) = try resolveOptions(env: env, xcodePID: xcodePID, sessionID: sessionID)
            let bridgeEnv = EnvOptions.applyOverrides(baseEnv: env, opts: effective)

            let request = buildAgentRequest(
                env: bridgeEnv, effective: effective,
                timeout: TimeInterval(timeout), debug: debug
            )
            let tools = try await AgentClient.listTools(request: request)

            if json {
                let encoder = JSONEncoder()
                encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
                let data = try encoder.encode(tools)
                FileHandle.standardOutput.write(data)
                FileHandle.standardOutput.write(Data("\n".utf8))
            } else {
                for tool in tools {
                    if case .object(let obj) = tool {
                        let name = obj["name"].flatMap { if case .string(let s) = $0 { return s } else { return nil } } ?? ""
                        let desc = obj["description"].flatMap { if case .string(let s) = $0 { return s } else { return nil } } ?? ""
                        if desc.isEmpty {
                            print(name)
                        } else {
                            print("\(name)\t\(desc)")
                        }
                    }
                }
            }
        }
    }
}
