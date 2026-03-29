import Foundation

// MARK: - Types

public enum CheckStatus: String, Codable, Sendable {
    case ok = "ok"
    case warn = "warn"
    case fail = "fail"
    case info = "info"
}

public struct DoctorCheck: Codable, Sendable {
    public let name: String
    public let status: CheckStatus
    public let detail: String

    public init(name: String, status: CheckStatus, detail: String) {
        self.name = name
        self.status = status
        self.detail = detail
    }
}

public struct DoctorSummary: Codable, Sendable {
    public let ok: Int
    public let warn: Int
    public let fail: Int
    public let info: Int
}

public struct DoctorRecommendation: Codable, Sendable {
    public let id: String
    public let message: String
    public let commands: [String]

    public init(id: String, message: String, commands: [String] = []) {
        self.id = id
        self.message = message
        self.commands = commands
    }
}

public struct DoctorJSONReport: Codable, Sendable {
    public let success: Bool
    public let summary: DoctorSummary
    public let checks: [DoctorCheck]
    public let recommendations: [DoctorRecommendation]
}

public struct DoctorReport: Sendable {
    public let checks: [DoctorCheck]

    public init(checks: [DoctorCheck]) {
        self.checks = checks
    }

    public var isSuccess: Bool {
        !checks.contains { $0.status == .fail }
    }

    public var summary: DoctorSummary {
        var ok = 0, warn = 0, fail = 0, info = 0
        for check in checks {
            switch check.status {
            case .ok: ok += 1
            case .warn: warn += 1
            case .fail: fail += 1
            case .info: info += 1
            }
        }
        return DoctorSummary(ok: ok, warn: warn, fail: fail, info: info)
    }

    public var jsonReport: DoctorJSONReport {
        DoctorJSONReport(success: isSuccess, summary: summary, checks: checks, recommendations: recommendations)
    }

    public var textReport: String {
        let s = summary
        var lines = ["xcodecli doctor", ""]
        for check in checks {
            lines.append("\(statusIcon(check.status)) \(check.name): \(check.detail)")
        }
        if !recommendations.isEmpty {
            lines.append("")
            lines.append("Recommendations:")
            for recommendation in recommendations {
                lines.append("- \(recommendation.message)")
                lines.append(contentsOf: recommendation.commands.map { "  \($0)" })
            }
        }
        lines.append("")
        lines.append("Summary: \(s.ok) ok, \(s.warn) warn, \(s.fail) fail, \(s.info) info")
        return lines.joined(separator: "\n") + "\n"
    }

    public var recommendations: [DoctorRecommendation] {
        var recommendations: [DoctorRecommendation] = []

        if let check = checks.first(where: { $0.name == "LaunchAgent binary registration" && $0.status == .warn }) {
            recommendations.append(DoctorRecommendation(
                id: "launchagent-registration",
                message: "LaunchAgent registration looks stale or unstable. Re-register xcodecli from one stable installed path.",
                commands: [
                    "xcodecli agent uninstall",
                    "xcodecli mcp codex",
                ]
            ))
            if check.detail.contains("relative") {
                recommendations.append(DoctorRecommendation(
                    id: "launchagent-relative-path",
                    message: "Avoid relative or versioned binary paths in LaunchAgent ProgramArguments; prefer an absolute stable path such as /opt/homebrew/bin/xcodecli."
                ))
            }
        }

        if checks.contains(where: { $0.name == "effective MCP_XCODE_PID" && $0.status == .warn }) {
            recommendations.append(DoctorRecommendation(
                id: "session-key-xcode-pid",
                message: "Drop explicit MCP_XCODE_PID unless you intentionally want a separate pooled session."
            ))
        }

        if checks.contains(where: { $0.name == "effective DEVELOPER_DIR" && $0.status == .warn }) {
            recommendations.append(DoctorRecommendation(
                id: "session-key-developer-dir",
                message: "Keep DEVELOPER_DIR aligned with xcode-select -p across runs unless you intentionally need a separate pooled session."
            ))
        }

        if checks.contains(where: { $0.name == "running Xcode processes" && $0.status == .warn }) {
            recommendations.append(DoctorRecommendation(
                id: "xcode-not-running",
                message: "Open Xcode with the target workspace visible before using bridge-backed commands."
            ))
        }

        return recommendations
    }

    private func statusIcon(_ status: CheckStatus) -> String {
        switch status {
        case .ok: return "[OK]"
        case .warn: return "[WARN]"
        case .fail: return "[FAIL]"
        case .info: return "[INFO]"
        }
    }
}

// MARK: - Process Info

public struct XcodeProcess: Sendable {
    public let pid: Int
    public let command: String

    public init(pid: Int, command: String) {
        self.pid = pid
        self.command = command
    }

    public var looksLikeXcode: Bool {
        let firstToken = command.split(separator: " ").first.map(String.init) ?? command
        let base = (firstToken as NSString).lastPathComponent
        return firstToken.contains("/Contents/MacOS/Xcode") || base == "Xcode"
    }
}

// MARK: - Agent Status (forward declaration for doctor)

public struct AgentStatus: Codable, Sendable {
    public var label: String
    public var plistPath: String
    public var plistInstalled: Bool
    public var registeredBinary: String
    public var currentBinary: String
    public var binaryPathMatches: Bool
    public var socketPath: String
    public var socketReachable: Bool
    public var running: Bool
    public var pid: Int
    public var idleTimeoutNs: Int64 // nanoseconds (Go time.Duration compatible)
    public var backendSessions: Int
    public var warnings: [String]
    public var nextSteps: [String]

    public init(
        label: String = "", plistPath: String = "", plistInstalled: Bool = false,
        registeredBinary: String = "", currentBinary: String = "",
        binaryPathMatches: Bool = false, socketPath: String = "",
        socketReachable: Bool = false, running: Bool = false, pid: Int = 0,
        idleTimeoutNs: Int64 = 0, backendSessions: Int = 0,
        warnings: [String] = [], nextSteps: [String] = []
    ) {
        self.label = label
        self.plistPath = plistPath
        self.plistInstalled = plistInstalled
        self.registeredBinary = registeredBinary
        self.currentBinary = currentBinary
        self.binaryPathMatches = binaryPathMatches
        self.socketPath = socketPath
        self.socketReachable = socketReachable
        self.running = running
        self.pid = pid
        self.idleTimeoutNs = idleTimeoutNs
        self.backendSessions = backendSessions
        self.warnings = warnings
        self.nextSteps = nextSteps
    }
}

// MARK: - Doctor Options

public struct DoctorOptions: Sendable {
    public var baseEnv: [String: String]
    public var xcodePID: String
    public var sessionID: String
    public var sessionSource: SessionSource
    public var sessionPath: String
    public var agentStatus: AgentStatus?
    public var agentStatusError: String?

    public init(
        baseEnv: [String: String] = [:],
        xcodePID: String = "",
        sessionID: String = "",
        sessionSource: SessionSource = .unset,
        sessionPath: String = "",
        agentStatus: AgentStatus? = nil,
        agentStatusError: String? = nil
    ) {
        self.baseEnv = baseEnv
        self.xcodePID = xcodePID
        self.sessionID = sessionID
        self.sessionSource = sessionSource
        self.sessionPath = sessionPath
        self.agentStatus = agentStatus
        self.agentStatusError = agentStatusError
    }
}

// MARK: - Inspector

public struct DoctorInspector: Sendable {
    private let processRunner: any ProcessRunning
    private let lookPath: @Sendable (String) async -> String?
    private let listProcesses: @Sendable () async throws -> [XcodeProcess]

    public init(processRunner: any ProcessRunning) {
        self.processRunner = processRunner
        self.lookPath = { @Sendable command in
            let paths = systemPATH()
            for dir in paths {
                let fullPath = (dir as NSString).appendingPathComponent(command)
                if FileManager.default.isExecutableFile(atPath: fullPath) {
                    return fullPath
                }
            }
            return nil
        }
        self.listProcesses = { @Sendable in
            let result = try await processRunner.run(
                "/bin/ps", arguments: ["-axo", "pid=,command="]
            )
            return parseProcessList(result.stdout)
        }
    }

    /// Testable initializer with all dependencies injectable.
    public init(
        processRunner: any ProcessRunning,
        lookPath: @escaping @Sendable (String) async -> String?,
        listProcesses: @escaping @Sendable () async throws -> [XcodeProcess]
    ) {
        self.processRunner = processRunner
        self.lookPath = lookPath
        self.listProcesses = listProcesses
    }

    public func run(opts: DoctorOptions) async -> DoctorReport {
        var checks: [DoctorCheck] = []
        var xcodeSelectPath = ""

        // 1. xcrun lookup
        let xcrunPath = await lookPath("xcrun")
        let xcrunAvailable = xcrunPath != nil
        if let path = xcrunPath {
            checks.append(DoctorCheck(name: "xcrun lookup", status: .ok, detail: path))
        } else {
            checks.append(DoctorCheck(name: "xcrun lookup", status: .fail, detail: "xcrun not found on PATH"))
        }

        // 2. xcrun mcpbridge --help
        if xcrunAvailable, let path = xcrunPath {
            do {
                let result = try await processRunner.run(path, arguments: ["mcpbridge", "--help"])
                if result.exitCode == 0 {
                    checks.append(DoctorCheck(
                        name: "xcrun mcpbridge --help", status: .ok,
                        detail: "exit 0 (\(result.stdout.count) bytes stdout)"
                    ))
                } else {
                    checks.append(DoctorCheck(
                        name: "xcrun mcpbridge --help", status: .fail,
                        detail: formatCommandFailure(exitCode: result.exitCode, stderr: result.stderr, stdout: result.stdout)
                    ))
                }
            } catch {
                checks.append(DoctorCheck(
                    name: "xcrun mcpbridge --help", status: .fail, detail: error.localizedDescription
                ))
            }
        } else {
            checks.append(DoctorCheck(
                name: "xcrun mcpbridge --help", status: .info,
                detail: "skipped because xcrun is unavailable"
            ))
        }

        // 3. xcode-select -p
        do {
            let result = try await processRunner.run("/usr/bin/xcode-select", arguments: ["-p"])
            if result.exitCode == 0 {
                xcodeSelectPath = result.stdout.trimmingCharacters(in: .whitespacesAndNewlines)
                checks.append(DoctorCheck(
                    name: "xcode-select -p", status: .ok,
                    detail: xcodeSelectPath
                ))
            } else {
                checks.append(DoctorCheck(
                    name: "xcode-select -p", status: .fail,
                    detail: formatCommandFailure(exitCode: result.exitCode, stderr: result.stderr, stdout: result.stdout)
                ))
            }
        } catch {
            checks.append(DoctorCheck(
                name: "xcode-select -p", status: .fail, detail: error.localizedDescription
            ))
        }

        // 4. Running Xcode processes
        var allProcesses: [XcodeProcess] = []
        var xcodeCandidates: [XcodeProcess]
        var procError: Error? = nil
        do {
            allProcesses = try await listProcesses()
            xcodeCandidates = allProcesses
                .filter { $0.looksLikeXcode }
                .sorted { $0.pid < $1.pid }
            if xcodeCandidates.isEmpty {
                checks.append(DoctorCheck(
                    name: "running Xcode processes", status: .warn,
                    detail: "no Xcode.app process detected"
                ))
            } else {
                let summary = xcodeCandidates.map { "\($0.pid) \($0.command)" }.joined(separator: " | ")
                checks.append(DoctorCheck(
                    name: "running Xcode processes", status: .ok, detail: summary
                ))
            }
        } catch {
            procError = error
            checks.append(DoctorCheck(
                name: "running Xcode processes", status: .fail, detail: error.localizedDescription
            ))
        }

        // 5. Effective MCP_XCODE_PID
        var pidValid = true
        if opts.xcodePID.isEmpty {
            checks.append(DoctorCheck(
                name: "effective MCP_XCODE_PID", status: .info, detail: "not set"
            ))
        } else {
            do {
                let pid = try EnvOptions.parsePID(opts.xcodePID)
                if procError != nil {
                    pidValid = false
                    checks.append(DoctorCheck(
                        name: "effective MCP_XCODE_PID", status: .fail,
                        detail: "cannot validate PID because process listing failed"
                    ))
                } else if let proc = allProcesses.first(where: { $0.pid == pid }) {
                    if proc.looksLikeXcode {
                        checks.append(DoctorCheck(
                            name: "effective MCP_XCODE_PID", status: .warn,
                            detail: "PID \(pid) -> \(proc.command) (explicit MCP_XCODE_PID partitions the pooled session key; changing it between runs can trigger a fresh mcpbridge session)"
                        ))
                    } else {
                        pidValid = false
                        checks.append(DoctorCheck(
                            name: "effective MCP_XCODE_PID", status: .fail,
                            detail: "PID \(pid) does not look like an Xcode.app process (\(proc.command))"
                        ))
                    }
                } else {
                    pidValid = false
                    checks.append(DoctorCheck(
                        name: "effective MCP_XCODE_PID", status: .fail,
                        detail: "PID \(pid) was not found"
                    ))
                }
            } catch {
                pidValid = false
                checks.append(DoctorCheck(
                    name: "effective MCP_XCODE_PID", status: .fail,
                    detail: error.localizedDescription
                ))
            }
        }

        // 6. Effective MCP_XCODE_SESSION_ID
        var sessionValid = true
        if opts.sessionID.isEmpty {
            checks.append(DoctorCheck(
                name: "effective MCP_XCODE_SESSION_ID", status: .info, detail: "not set"
            ))
        } else if !EnvOptions.isValidUUID(opts.sessionID) {
            sessionValid = false
            checks.append(DoctorCheck(
                name: "effective MCP_XCODE_SESSION_ID", status: .fail,
                detail: "MCP_XCODE_SESSION_ID must be a UUID"
            ))
        } else {
            checks.append(DoctorCheck(
                name: "effective MCP_XCODE_SESSION_ID", status: .ok,
                detail: formatSessionDetail(opts: opts)
            ))
        }

        // 7. Effective DEVELOPER_DIR
        let developerDir = opts.baseEnv["DEVELOPER_DIR"]?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if developerDir.isEmpty {
            checks.append(DoctorCheck(
                name: "effective DEVELOPER_DIR", status: .info,
                detail: "not set (using xcode-select -p)"
            ))
        } else if !xcodeSelectPath.isEmpty && developerDir != xcodeSelectPath {
            checks.append(DoctorCheck(
                name: "effective DEVELOPER_DIR", status: .warn,
                detail: "\(developerDir) (overrides xcode-select -p \(xcodeSelectPath); DEVELOPER_DIR is part of the pooled session key, so changing it can trigger a fresh mcpbridge session)"
            ))
        } else if !xcodeSelectPath.isEmpty {
            checks.append(DoctorCheck(
                name: "effective DEVELOPER_DIR", status: .ok,
                detail: "\(developerDir) (matches xcode-select -p)"
            ))
        } else {
            checks.append(DoctorCheck(
                name: "effective DEVELOPER_DIR", status: .info,
                detail: developerDir
            ))
        }

        // 8. Spawn smoke test
        if !xcrunAvailable {
            checks.append(DoctorCheck(
                name: "spawn smoke test", status: .info,
                detail: "skipped because xcrun is unavailable"
            ))
        } else if !pidValid || !sessionValid {
            checks.append(DoctorCheck(
                name: "spawn smoke test", status: .info,
                detail: "skipped because explicit overrides failed validation"
            ))
        } else if let path = xcrunPath {
            let smokeEnv = EnvOptions.applyOverrides(
                baseEnv: opts.baseEnv,
                opts: EnvOptions(xcodePID: opts.xcodePID, sessionID: opts.sessionID)
            )
            let startedAt = ContinuousClock.now
            do {
                let result = try await processRunner.run(
                    path, arguments: ["mcpbridge"],
                    environment: smokeEnv,
                    workingDirectory: nil,
                    stdinData: Data() // empty stdin, closes immediately
                )
                let elapsed = ContinuousClock.now - startedAt
                if result.exitCode == 0 {
                    checks.append(DoctorCheck(
                        name: "spawn smoke test", status: .ok,
                        detail: "exit 0 in \(formatDuration(elapsed))"
                    ))
                } else {
                    checks.append(DoctorCheck(
                        name: "spawn smoke test", status: .fail,
                        detail: formatCommandFailure(exitCode: result.exitCode, stderr: result.stderr, stdout: result.stdout)
                    ))
                }
            } catch {
                checks.append(DoctorCheck(
                    name: "spawn smoke test", status: .fail, detail: error.localizedDescription
                ))
            }
        }

        // 9. LaunchAgents directory permissions
        let launchAgentsDir = (AgentPaths.plistPath() as NSString).deletingLastPathComponent
        if FileManager.default.fileExists(atPath: launchAgentsDir) {
            if FileManager.default.isWritableFile(atPath: launchAgentsDir) {
                checks.append(DoctorCheck(
                    name: "LaunchAgents directory", status: .ok,
                    detail: launchAgentsDir
                ))
            } else {
                var hint = "not writable"
                if let attrs = try? FileManager.default.attributesOfItem(atPath: launchAgentsDir),
                   let ownerID = attrs[.ownerAccountID] as? UInt,
                   ownerID != getuid() {
                    let ownerName = attrs[.ownerAccountName] as? String ?? "uid \(ownerID)"
                    hint = "owned by \(ownerName), not writable. Fix: sudo chown $(whoami) \(launchAgentsDir)"
                }
                checks.append(DoctorCheck(
                    name: "LaunchAgents directory", status: .warn,
                    detail: "\(launchAgentsDir): \(hint)"
                ))
            }
        } else {
            checks.append(DoctorCheck(
                name: "LaunchAgents directory", status: .info,
                detail: "\(launchAgentsDir) does not exist (will be created on first agent use)"
            ))
        }

        // 10. Plist file permissions
        let plistPath = AgentPaths.plistPath()
        if FileManager.default.fileExists(atPath: plistPath) {
            if FileManager.default.isWritableFile(atPath: plistPath) {
                checks.append(DoctorCheck(
                    name: "LaunchAgent plist writable", status: .ok,
                    detail: plistPath
                ))
            } else {
                var hint = "not writable"
                if let attrs = try? FileManager.default.attributesOfItem(atPath: plistPath),
                   let ownerID = attrs[.ownerAccountID] as? UInt,
                   ownerID != getuid() {
                    let ownerName = attrs[.ownerAccountName] as? String ?? "uid \(ownerID)"
                    hint = "owned by \(ownerName). Fix: sudo chown $(whoami) \(plistPath)"
                }
                checks.append(DoctorCheck(
                    name: "LaunchAgent plist writable", status: .warn,
                    detail: "\(plistPath): \(hint)"
                ))
            }
        }

        // 11-13. LaunchAgent status checks
        if let errMsg = opts.agentStatusError {
            checks.append(DoctorCheck(
                name: "LaunchAgent status", status: .info,
                detail: "unavailable: \(errMsg)"
            ))
        } else if let status = opts.agentStatus {
            checks.append(DoctorCheck(
                name: "LaunchAgent plist", status: .info,
                detail: "installed=\(status.plistInstalled) path=\(status.plistPath)"
            ))
            checks.append(DoctorCheck(
                name: "LaunchAgent socket", status: .info,
                detail: "reachable=\(status.socketReachable) path=\(status.socketPath)"
            ))
            if !status.registeredBinary.isEmpty || !status.currentBinary.isEmpty {
                checks.append(launchAgentBinaryRegistrationCheck(status))
            }
        }

        return DoctorReport(checks: checks)
    }

    // MARK: - Formatting

    private func formatCommandFailure(exitCode: Int32, stderr: String, stdout: String) -> String {
        var parts: [String] = []
        if exitCode != 0 {
            parts.append("exit \(exitCode)")
        }
        let text = (stderr + " " + stdout).trimmingCharacters(in: .whitespacesAndNewlines)
        if !text.isEmpty {
            parts.append(text)
        }
        return parts.joined(separator: "; ")
    }

    private func formatSessionDetail(opts: DoctorOptions) -> String {
        switch opts.sessionSource {
        case .persisted where !opts.sessionPath.isEmpty:
            return "\(opts.sessionID) (persisted at \(opts.sessionPath))"
        case .generated where !opts.sessionPath.isEmpty:
            return "\(opts.sessionID) (generated and saved to \(opts.sessionPath))"
        case .env:
            return "\(opts.sessionID) (from environment)"
        case .explicit:
            return "\(opts.sessionID) (from --session-id)"
        default:
            return opts.sessionID
        }
    }

    private func formatDuration(_ duration: Duration) -> String {
        let ms = duration.components.attoseconds / 1_000_000_000_000_000
        let totalMs = Int(duration.components.seconds) * 1000 + Int(ms)
        let roundedMs = (totalMs / 10) * 10
        if roundedMs >= 1000 {
            return String(format: "%.2fs", Double(roundedMs) / 1000.0)
        }
        return "\(roundedMs)ms"
    }

    private func launchAgentBinaryRegistrationCheck(_ status: AgentStatus) -> DoctorCheck {
        let detailPrefix = "registered=\(status.registeredBinary) | current=\(status.currentBinary) | match=\(status.binaryPathMatches)"

        if !status.registeredBinary.isEmpty && !(status.registeredBinary as NSString).isAbsolutePath {
            return DoctorCheck(
                name: "LaunchAgent binary registration",
                status: .warn,
                detail: "\(detailPrefix) | registered binary path is relative; stale older installs can make launchctl bootstrap fail with Input/output error. Rewrite the plist with the current binary or run `xcodecli agent uninstall` before retrying."
            )
        }

        if !status.registeredBinary.isEmpty && !FileManager.default.isExecutableFile(atPath: status.registeredBinary) {
            return DoctorCheck(
                name: "LaunchAgent binary registration",
                status: .warn,
                detail: "\(detailPrefix) | registered binary is missing or not executable; the next LaunchAgent bootstrap may fail until the plist is rewritten."
            )
        }

        if !status.registeredBinary.isEmpty && !status.currentBinary.isEmpty && !status.binaryPathMatches {
            return DoctorCheck(
                name: "LaunchAgent binary registration",
                status: .warn,
                detail: "\(detailPrefix) | switching binaries recycles the LaunchAgent backend and can surface new Xcode authorization prompts. Keep one stable xcodecli path for long-lived MCP use."
            )
        }

        return DoctorCheck(
            name: "LaunchAgent binary registration",
            status: .info,
            detail: detailPrefix
        )
    }
}

// MARK: - Helpers

private func systemPATH() -> [String] {
    (Foundation.ProcessInfo.processInfo.environment["PATH"] ?? "")
        .split(separator: ":").map(String.init)
}

public func parseProcessList(_ output: String) -> [XcodeProcess] {
    output.split(separator: "\n").compactMap { line in
        let trimmed = line.trimmingCharacters(in: .whitespaces)
        guard let spaceIndex = trimmed.firstIndex(where: { $0.isWhitespace }) else { return nil }
        let pidStr = String(trimmed[trimmed.startIndex..<spaceIndex]).trimmingCharacters(in: .whitespaces)
        let cmd = String(trimmed[spaceIndex...]).trimmingCharacters(in: .whitespaces)
        guard let pid = Int(pidStr), !cmd.isEmpty else { return nil }
        return XcodeProcess(pid: pid, command: cmd)
    }
}
