import Foundation

/// Protocol for interacting with launchctl.
public protocol LaunchdInterface: Sendable {
    func print(target: String) async throws -> String
    func bootstrap(domainTarget: String, plistPath: String) async throws
    func kickstart(serviceTarget: String) async throws
    func bootout(target: String) async throws
}

/// Concrete launchctl implementation.
public struct CommandLaunchd: LaunchdInterface, Sendable {
    private let runner: any ProcessRunning

    public init(runner: any ProcessRunning = SystemProcessRunner()) {
        self.runner = runner
    }

    public func print(target: String) async throws -> String {
        try await runLaunchctl("print", target)
    }

    public func bootstrap(domainTarget: String, plistPath: String) async throws {
        _ = try await runLaunchctl("bootstrap", domainTarget, plistPath)
    }

    public func kickstart(serviceTarget: String) async throws {
        _ = try await runLaunchctl("kickstart", serviceTarget)
    }

    public func bootout(target: String) async throws {
        _ = try await runLaunchctl("bootout", target)
    }

    @discardableResult
    private func runLaunchctl(_ args: String...) async throws -> String {
        let result = try await runner.run("launchctl", arguments: args)
        if result.exitCode != 0 {
            let text = result.stderr.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                ? result.stdout.trimmingCharacters(in: .whitespacesAndNewlines)
                : result.stderr.trimmingCharacters(in: .whitespacesAndNewlines)
            if text.isEmpty {
                throw XcodeCLIError.bridgeSpawnFailed(underlying: "launchctl \(args.joined(separator: " ")): exit \(result.exitCode)")
            }
            throw XcodeCLIError.bridgeSpawnFailed(underlying: "launchctl \(args.joined(separator: " ")): \(text)")
        }
        return result.stdout
    }
}

// MARK: - Target Helpers

public func launchAgentDomainTarget() -> String {
    "gui/\(getuid())"
}

public func launchAgentServiceTarget(label: String) -> String {
    "\(launchAgentDomainTarget())/\(label)"
}
