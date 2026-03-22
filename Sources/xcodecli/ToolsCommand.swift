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
            let request = try buildBridgeRequest(
                xcodePID: xcodePID, sessionID: sessionID,
                timeout: TimeInterval(timeout), debug: debug
            )
            let tools = try await AgentClient.listTools(request: request)

            if json {
                try writePrettyJSON(tools)
            } else {
                for tool in tools {
                    let name = toolName(tool)
                    guard !name.isEmpty else { continue }
                    let desc = toolDescription(tool)
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
