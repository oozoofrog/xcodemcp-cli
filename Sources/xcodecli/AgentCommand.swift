import ArgumentParser
import Foundation
import XcodeCLICore

struct AgentCommand: AsyncParsableCommand {
    static let configuration = CommandConfiguration(
        commandName: "agent",
        abstract: "Inspect or manage the LaunchAgent used by tools commands",
        subcommands: [
            StatusSubcommand.self,
            StopSubcommand.self,
            UninstallSubcommand.self,
            RunSubcommand.self,
            GuideSubcommand.self,
            DemoSubcommand.self,
        ]
    )

    struct RunSubcommand: AsyncParsableCommand {
        static let configuration = CommandConfiguration(
            commandName: "run",
            abstract: "Start the agent daemon (used by LaunchAgent)"
        )

        @Flag(name: .customLong("launch-agent"), help: "Required flag to confirm this is launched by LaunchAgent")
        var launchAgent = false

        @Option(name: .customLong("idle-timeout"), help: "Idle timeout in seconds (default: 86400 = 24h)")
        var idleTimeout: Int = 86400

        @Flag(name: .customLong("debug"), help: "Emit debug logs")
        var debug = false

        func run() async throws {
            guard launchAgent else {
                throw ValidationError("agent run requires --launch-agent flag")
            }

            let env = envDictionary()
            let config = AgentServerConfig(
                idleTimeout: TimeInterval(idleTimeout),
                baseEnv: env,
                debug: debug
            )

            let server = AgentServer(config: config)

            // Handle signals for graceful shutdown
            let sigintSource = DispatchSource.makeSignalSource(signal: SIGINT, queue: .global())
            let sigtermSource = DispatchSource.makeSignalSource(signal: SIGTERM, queue: .global())
            let sighupSource = DispatchSource.makeSignalSource(signal: SIGHUP, queue: .global())

            signal(SIGINT, SIG_IGN)
            signal(SIGTERM, SIG_IGN)
            signal(SIGHUP, SIG_IGN)

            sigintSource.setEventHandler { server.shutdown() }
            sigtermSource.setEventHandler { server.shutdown() }
            sighupSource.setEventHandler { server.shutdown() }

            sigintSource.resume()
            sigtermSource.resume()
            sighupSource.resume()

            try await server.run()

            sigintSource.cancel()
            sigtermSource.cancel()
            sighupSource.cancel()
        }
    }

    struct GuideSubcommand: AsyncParsableCommand {
        static let configuration = CommandConfiguration(
            commandName: "guide",
            abstract: "Show workflow guidance for a given intent"
        )

        @Argument(help: "Intent or workflow name")
        var intent: String = "catalog"

        @Flag(name: .long, help: "Print as JSON")
        var json = false

        @Option(name: .customLong("timeout"), help: "Request timeout in seconds")
        var timeout: Int = 60

        @Option(name: .customLong("xcode-pid"), help: "Override MCP_XCODE_PID")
        var xcodePID: String?

        @Option(name: .customLong("session-id"), help: "Override MCP_XCODE_SESSION_ID")
        var sessionID: String?

        @Flag(name: .customLong("debug"), help: "Emit debug logs")
        var debug = false

        func run() async throws {
            try await runAgentGuide(
                intent: intent, json: json, timeout: timeout,
                xcodePID: xcodePID, sessionID: sessionID, debug: debug
            )
        }
    }

    struct DemoSubcommand: AsyncParsableCommand {
        static let configuration = CommandConfiguration(
            commandName: "demo",
            abstract: "Run a demonstration of agent capabilities"
        )

        @Flag(name: .long, help: "Print as JSON")
        var json = false

        @Option(name: .customLong("timeout"), help: "Request timeout in seconds")
        var timeout: Int = 60

        @Option(name: .customLong("xcode-pid"), help: "Override MCP_XCODE_PID")
        var xcodePID: String?

        @Option(name: .customLong("session-id"), help: "Override MCP_XCODE_SESSION_ID")
        var sessionID: String?

        @Flag(name: .customLong("debug"), help: "Emit debug logs")
        var debug = false

        func run() async throws {
            try await runAgentDemo(
                json: json, timeout: timeout,
                xcodePID: xcodePID, sessionID: sessionID, debug: debug
            )
        }
    }

    struct StatusSubcommand: AsyncParsableCommand {
        static let configuration = CommandConfiguration(
            commandName: "status",
            abstract: "Show LaunchAgent installation and runtime state"
        )

        @Flag(name: .long, help: "Print as pretty JSON")
        var json = false

        func run() async throws {
            let status = try await AgentClient.status()

            if json {
                try writePrettyJSON(status)
            } else {
                print(formatAgentStatus(status))
            }
        }
    }

    struct StopSubcommand: AsyncParsableCommand {
        static let configuration = CommandConfiguration(
            commandName: "stop",
            abstract: "Ask the running LaunchAgent process to stop"
        )

        func run() async throws {
            try await AgentClient.stop()
            print("stopped LaunchAgent process if it was running")
        }
    }

    struct UninstallSubcommand: AsyncParsableCommand {
        static let configuration = CommandConfiguration(
            commandName: "uninstall",
            abstract: "Remove the LaunchAgent plist and local agent runtime files"
        )

        func run() async throws {
            try await AgentClient.uninstall()
            print("removed LaunchAgent plist and local agent runtime files")
        }
    }
}

func formatAgentStatus(_ status: AgentStatus) -> String {
    let binaryLine = status.registeredBinary.isEmpty ? "not installed" : status.registeredBinary
    let matchText: String
    if !status.registeredBinary.isEmpty && !status.currentBinary.isEmpty {
        matchText = status.binaryPathMatches ? "yes" : "no"
    } else {
        matchText = "n/a"
    }
    let runningText = status.running ? "yes" : "no"
    let socketText = status.socketReachable ? "yes" : "no"

    return """
    xcodecli agent

    label: \(status.label)
    plist installed: \(status.plistInstalled)
    plist path: \(status.plistPath)
    registered binary: \(binaryLine)
    current binary: \(status.currentBinary)
    binary matches: \(matchText)
    socket path: \(status.socketPath)
    socket reachable: \(socketText)
    running: \(runningText)
    pid: \(status.pid)
    idle timeout: \(formatTimeoutDuration(ns: status.idleTimeoutNs))
    backend sessions: \(status.backendSessions)
    """
}

func formatTimeoutDuration(ns: Int64) -> String {
    if ns <= 0 { return "0s" }
    let hours = ns / 3_600_000_000_000
    let minutes = ns / 60_000_000_000
    let seconds = ns / 1_000_000_000
    if ns % 3_600_000_000_000 == 0 {
        return "\(hours)h"
    }
    if ns >= 5 * 60_000_000_000 && ns % 60_000_000_000 == 0 {
        return "\(minutes)m"
    }
    if ns % 1_000_000_000 == 0 {
        return "\(seconds)s"
    }
    return "\(ns)ns"
}
