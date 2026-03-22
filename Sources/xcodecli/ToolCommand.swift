import ArgumentParser
import Foundation
import XcodeCLICore

struct ToolCommand: AsyncParsableCommand {
    static let configuration = CommandConfiguration(
        commandName: "tool",
        abstract: "Convenience commands for inspecting or calling a tool",
        subcommands: [InspectSubcommand.self, CallSubcommand.self]
    )

    struct InspectSubcommand: AsyncParsableCommand {
        static let configuration = CommandConfiguration(
            commandName: "inspect",
            abstract: "Show tool description and input schema"
        )

        @Argument(help: "Name of the tool to inspect")
        var name: String

        @Flag(name: .long, help: "Print the raw tool object as pretty JSON")
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

            guard let tool = findToolByName(tools, name) else {
                throw ValidationError("tool not found: \(name)")
            }

            if json {
                try writePrettyJSON(tool)
            } else if case .object(let obj) = tool {
                print("name: \(toolName(tool))")
                print("description: \(toolDescription(tool))")
                print("inputSchema:")
                if let schema = obj["inputSchema"] {
                    try writePrettyJSON(schema)
                }
            }
        }
    }

    struct CallSubcommand: AsyncParsableCommand {
        static let configuration = CommandConfiguration(
            commandName: "call",
            abstract: "Call a single MCP tool with JSON object arguments"
        )

        @Argument(help: "Name of the tool to call")
        var name: String

        @Option(name: .long, help: "JSON object passed as tool arguments")
        var json: String?

        @Flag(name: .customLong("json-stdin"), help: "Read the JSON object payload from stdin")
        var jsonStdin = false

        @Option(name: .customLong("timeout"), help: "Override the request timeout")
        var timeout: Int?

        @Option(name: .customLong("xcode-pid"), help: "Override MCP_XCODE_PID")
        var xcodePID: String?

        @Option(name: .customLong("session-id"), help: "Override MCP_XCODE_SESSION_ID")
        var sessionID: String?

        @Flag(name: .customLong("debug"), help: "Emit debug logs to stderr")
        var debug = false

        func run() async throws {
            guard json != nil || jsonStdin else {
                throw ValidationError("tool call requires exactly one of --json or --json-stdin")
            }
            guard !(json != nil && jsonStdin) else {
                throw ValidationError("tool call accepts exactly one of --json or --json-stdin")
            }

            let jsonPayload: String
            if jsonStdin {
                let data = FileHandle.standardInput.readDataToEndOfFile()
                jsonPayload = String(data: data, encoding: .utf8) ?? "{}"
            } else if let raw = json {
                jsonPayload = raw.hasPrefix("@")
                    ? try String(contentsOfFile: String(raw.dropFirst()), encoding: .utf8)
                    : raw
            } else {
                jsonPayload = "{}"
            }

            let arguments = try parseJSONArguments(jsonPayload)

            // Apply tool-specific default timeout if not explicitly set
            let effectiveTimeout = timeout ?? Int(TimeoutPolicy.defaultToolCallTimeout(toolName: name))

            let request = try buildBridgeRequest(
                xcodePID: xcodePID, sessionID: sessionID,
                timeout: TimeInterval(effectiveTimeout), debug: debug
            )
            let result = try await AgentClient.callTool(
                request: request, name: name, arguments: arguments
            )

            try writePrettyJSON(result.result)

            if result.isError {
                throw ExitCode(1)
            }
        }

    }
}
