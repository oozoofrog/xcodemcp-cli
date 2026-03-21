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
            let env = envDictionary()
            let (effective, _) = try resolveOptions(env: env, xcodePID: xcodePID, sessionID: sessionID)
            let bridgeEnv = EnvOptions.applyOverrides(baseEnv: env, opts: effective)

            let request = buildAgentRequest(
                env: bridgeEnv, effective: effective,
                timeout: TimeInterval(timeout), debug: debug
            )
            let tools = try await AgentClient.listTools(request: request)

            guard let tool = tools.first(where: {
                if case .object(let obj) = $0, case .string(let n) = obj["name"] { return n == name }
                return false
            }) else {
                throw ValidationError("tool not found: \(name)")
            }

            if json {
                let encoder = JSONEncoder()
                encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
                let data = try encoder.encode(tool)
                FileHandle.standardOutput.write(data)
                FileHandle.standardOutput.write(Data("\n".utf8))
            } else if case .object(let obj) = tool {
                let toolName = obj["name"].flatMap { if case .string(let s) = $0 { return s } else { return nil } } ?? ""
                let desc = obj["description"].flatMap { if case .string(let s) = $0 { return s } else { return nil } } ?? ""
                print("name: \(toolName)")
                print("description: \(desc)")
                print("inputSchema:")
                if let schema = obj["inputSchema"] {
                    let encoder = JSONEncoder()
                    encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
                    let data = try encoder.encode(schema)
                    FileHandle.standardOutput.write(data)
                    FileHandle.standardOutput.write(Data("\n".utf8))
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

            let env = envDictionary()
            let (effective, _) = try resolveOptions(env: env, xcodePID: xcodePID, sessionID: sessionID)
            let bridgeEnv = EnvOptions.applyOverrides(baseEnv: env, opts: effective)

            let request = buildAgentRequest(
                env: bridgeEnv, effective: effective,
                timeout: TimeInterval(effectiveTimeout), debug: debug
            )
            let result = try await AgentClient.callTool(
                request: request, name: name, arguments: arguments
            )

            let encoder = JSONEncoder()
            encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
            let data = try encoder.encode(result.result)
            FileHandle.standardOutput.write(data)
            FileHandle.standardOutput.write(Data("\n".utf8))

            if result.isError {
                throw ExitCode(1)
            }
        }

        private func parseJSONArguments(_ raw: String) throws -> [String: JSONValue] {
            let data = Data(raw.utf8)
            let value = try JSONDecoder().decode(JSONValue.self, from: data)
            guard case .object(let obj) = value else {
                throw ValidationError("JSON payload must decode to a JSON object")
            }
            return obj
        }
    }
}
