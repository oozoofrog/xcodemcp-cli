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
        let result = try await runner.run("/bin/launchctl", arguments: args)
        if result.exitCode != 0 {
            let text = result.stderr.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                ? result.stdout.trimmingCharacters(in: .whitespacesAndNewlines)
                : result.stderr.trimmingCharacters(in: .whitespacesAndNewlines)
            if text.isEmpty {
                throw XcodeCLIError.agentUnavailable(stage: "launchctl", underlying: "launchctl \(args.joined(separator: " ")): exit \(result.exitCode)")
            }
            throw XcodeCLIError.agentUnavailable(stage: "launchctl", underlying: "launchctl \(args.joined(separator: " ")): \(text)")
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

func recoverBootstrap(
    launchd: any LaunchdInterface,
    label: String,
    plistPath: String,
    initialError: Error,
    alreadyCleaned: Bool
) async throws -> Void {
    if !alreadyCleaned {
        try? await launchd.bootout(target: launchAgentServiceTarget(label: label))
    }
    do {
        try await launchd.bootstrap(
            domainTarget: launchAgentDomainTarget(),
            plistPath: plistPath
        )
    } catch {
        throw XcodeCLIError.agentUnavailable(
            stage: "launchctl",
            underlying:
                "retry launchctl bootstrap after cleanup failed: \(error.localizedDescription) (initial error: \(initialError.localizedDescription))"
        )
    }
}

func ensureLaunchAgentLoaded(
    launchd: any LaunchdInterface,
    label: String,
    plistPath: String,
    forceRestart: Bool,
    plistChanged: Bool
) async throws {
    let serviceTarget = launchAgentServiceTarget(label: label)

    if forceRestart || plistChanged {
        try? await launchd.bootout(target: serviceTarget)
        do {
            try await launchd.bootstrap(
                domainTarget: launchAgentDomainTarget(),
                plistPath: plistPath
            )
        } catch {
            try await recoverBootstrap(
                launchd: launchd,
                label: label,
                plistPath: plistPath,
                initialError: error,
                alreadyCleaned: true
            )
        }
        return
    }

    if (try? await launchd.print(target: serviceTarget)) != nil {
        do {
            try await launchd.kickstart(serviceTarget: serviceTarget)
        } catch {
            try await recoverBootstrap(
                launchd: launchd,
                label: label,
                plistPath: plistPath,
                initialError: error,
                alreadyCleaned: false
            )
        }
        return
    }

    do {
        try await launchd.bootstrap(
            domainTarget: launchAgentDomainTarget(),
            plistPath: plistPath
        )
    } catch {
        try await recoverBootstrap(
            launchd: launchd,
            label: label,
            plistPath: plistPath,
            initialError: error,
            alreadyCleaned: false
        )
    }
}
