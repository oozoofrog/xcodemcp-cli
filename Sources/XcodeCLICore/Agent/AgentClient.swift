import Foundation
#if canImport(Darwin)
import Darwin
#endif

/// Client for communicating with the xcodecli LaunchAgent via Unix socket RPC.
public enum AgentClient {

    // MARK: - Public API

    /// List tools through the agent with autostart.
    public static func listTools(request: AgentRequest) async throws -> [JSONValue] {
        var req = request
        req.method = "tools/list"
        let resp = try await doWithAutostart(req)
        return resp.tools ?? []
    }

    /// Call a tool through the agent with autostart.
    public static func callTool(
        request: AgentRequest,
        name: String,
        arguments: [String: JSONValue]
    ) async throws -> MCPCallResult {
        var req = request
        req.method = "tools/call"
        req.toolName = name
        req.arguments = arguments
        let resp = try await doWithAutostart(req)
        return MCPCallResult(result: resp.result ?? [:], isError: resp.isError ?? false)
    }

    /// Get agent status.
    public static func status() async throws -> AgentStatus {
        let paths = AgentPaths.defaultPaths()
        let label = AgentPaths.label

        var agentStatus = AgentStatus(
            label: label,
            plistPath: paths.plistPath,
            socketPath: paths.socketPath,
            idleTimeoutNs: defaultAgentIdleTimeoutNs
        )

        // Check plist
        if FileManager.default.fileExists(atPath: paths.plistPath) {
            agentStatus.plistInstalled = true
            if let registered = readLaunchAgentBinaryPath(paths.plistPath) {
                agentStatus.registeredBinary = registered
            }
        }

        // Current binary
        if let execPath = try? resolveExecutablePath() {
            agentStatus.currentBinary = execPath
            agentStatus.binaryPathMatches = samePath(execPath, agentStatus.registeredBinary)
        }

        // Try ping
        let pingReq = AgentRequest(method: "status")
        if let resp = try? await doRPC(pingReq) {
            agentStatus.socketReachable = true
            agentStatus.running = true
            if let status = resp.status {
                agentStatus.pid = status.pid
                agentStatus.idleTimeoutNs = status.idleTimeoutMs * 1_000_000 // ms -> ns
                agentStatus.backendSessions = status.backendSessions
            }
        }

        agentStatus.warnings = deriveAgentStatusWarnings(agentStatus)
        agentStatus.nextSteps = deriveAgentStatusNextSteps(agentStatus, warnings: agentStatus.warnings)

        return agentStatus
    }

    /// Stop the agent.
    public static func stop() async throws {
        let req = AgentRequest(method: "stop")
        do {
            _ = try await doRPC(req)
        } catch {
            if !isUnavailableError(error) { throw error }
        }
    }

    /// Uninstall the agent.
    public static func uninstall() async throws {
        var errors: [String] = []
        do { try await stop() } catch { errors.append("stop: \(error.localizedDescription)") }

        let paths = AgentPaths.defaultPaths()
        let label = AgentPaths.label
        let launchd = CommandLaunchd()
        do { try await launchd.bootout(target: launchAgentServiceTarget(label: label)) } catch { errors.append("bootout: \(error.localizedDescription)") }

        let fm = FileManager.default
        for path in [paths.plistPath, paths.socketPath, paths.pidPath, paths.logPath] {
            if fm.fileExists(atPath: path) {
                do { try fm.removeItem(atPath: path) } catch { errors.append("remove \((path as NSString).lastPathComponent): \(error.localizedDescription)") }
            }
        }
        if fm.fileExists(atPath: paths.supportDir) {
            do { try fm.removeItem(atPath: paths.supportDir) } catch { errors.append("remove supportDir: \(error.localizedDescription)") }
        }

        if !errors.isEmpty {
            throw XcodeCLIError.agentUnavailable(stage: "uninstall", underlying: errors.joined(separator: "; "))
        }
    }

    // MARK: - RPC Transport

    private static func doRPC(_ req: AgentRequest) async throws -> AgentResponse {
        let paths = AgentPaths.defaultPaths()
        let socketPath = paths.socketPath

        let fd = socket(AF_UNIX, SOCK_STREAM, 0)
        guard fd >= 0 else {
            throw XcodeCLIError.agentUnavailable(stage: "connect", underlying: "failed to create socket")
        }
        defer { Darwin.close(fd) }

        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        setUnixSocketPath(&addr, to: socketPath)

        let connectResult = withUnsafePointer(to: &addr) { ptr in
            ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) { sockPtr in
                Darwin.connect(fd, sockPtr, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }
        guard connectResult == 0 else {
            throw XcodeCLIError.agentUnavailable(stage: "connect", underlying: "connect to \(socketPath): \(String(cString: strerror(errno)))")
        }

        if let timeoutMS = req.timeoutMS, timeoutMS > 0 {
            var tv = timeval()
            tv.tv_sec = Int(timeoutMS / 1000)
            tv.tv_usec = Int32((timeoutMS % 1000) * 1000)
            if setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size)) != 0 {
                FileHandle.standardError.write(Data("[warn] setsockopt SO_RCVTIMEO failed: \(String(cString: strerror(errno)))\n".utf8))
            }
            if setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size)) != 0 {
                FileHandle.standardError.write(Data("[warn] setsockopt SO_SNDTIMEO failed: \(String(cString: strerror(errno)))\n".utf8))
            }
        }

        var payload = try JSONEncoder().encode(req)
        payload.append(UInt8(ascii: "\n"))
        guard writeAllToFD(fd, payload) else {
            throw XcodeCLIError.agentUnavailable(stage: "write", underlying: "write agent request failed")
        }

        var responseData = Data()
        var buf = [UInt8](repeating: 0, count: 65536)
        while true {
            let n = Darwin.read(fd, &buf, buf.count)
            if n <= 0 { break }
            responseData.append(contentsOf: buf[0..<n])
            if responseData.contains(UInt8(ascii: "\n")) { break }
        }

        guard let lineEnd = responseData.firstIndex(of: UInt8(ascii: "\n")) else {
            throw XcodeCLIError.agentUnavailable(stage: "read", underlying: "read agent response: no complete line")
        }

        let lineData = responseData[responseData.startIndex..<lineEnd]
        let resp = try JSONDecoder().decode(AgentResponse.self, from: lineData)
        if let errMsg = resp.error, !errMsg.isEmpty {
            throw XcodeCLIError.agentServerResponse(message: errMsg)
        }
        return resp
    }

    // MARK: - Autostart Logic

    private static func doWithAutostart(_ req: AgentRequest) async throws -> AgentResponse {
        let paths = AgentPaths.defaultPaths()
        let startTime = ContinuousClock.now

        if let (mismatch, _) = try? launchAgentBinaryMismatch(paths: paths), mismatch {
            try await ensureAgentReady(forceRestart: true)
            return try await doRPC(adjustTimeout(req, since: startTime))
        }

        do {
            return try await doRPC(req)
        } catch let error as XcodeCLIError {
            if case .agentServerResponse = error { throw error }
            guard isUnavailableError(error) else { throw error }
        }

        try await ensureAgentReady(forceRestart: false)
        return try await doRPC(adjustTimeout(req, since: startTime))
    }

    private static func adjustTimeout(_ req: AgentRequest, since startTime: ContinuousClock.Instant) -> AgentRequest {
        guard let timeoutMS = req.timeoutMS, timeoutMS > 0 else { return req }
        let elapsed = ContinuousClock.now - startTime
        let elapsedMS = Int64(elapsed.components.seconds) * 1000 +
                        Int64(elapsed.components.attoseconds / 1_000_000_000_000_000)
        var adjusted = req
        adjusted.timeoutMS = max(timeoutMS - elapsedMS, 1)
        return adjusted
    }

    private static func ensureAgentReady(forceRestart: Bool) async throws {
        let paths = AgentPaths.defaultPaths()
        let label = AgentPaths.label
        let launchd = CommandLaunchd()

        try FileManager.default.createDirectory(atPath: paths.supportDir, withIntermediateDirectories: true, attributes: nil)

        let executablePath = try resolveExecutablePath()
        let (changed, _) = try ensureLaunchAgentPlist(
            paths: paths, label: label, binaryPath: executablePath
        )

        try await ensureLaunchAgentLoaded(
            launchd: launchd,
            label: label,
            plistPath: paths.plistPath,
            forceRestart: forceRestart,
            plistChanged: changed
        )

        try await waitForReady()
    }

    private static func waitForReady() async throws {
        let deadline = Date().addingTimeInterval(5)
        while Date() < deadline {
            let pingReq = AgentRequest(method: "ping")
            if let _ = try? await doRPC(pingReq) {
                return
            }
            try await Task.sleep(nanoseconds: 100_000_000)
        }
        throw XcodeCLIError.agentTimeout(action: "waiting for LaunchAgent to become ready", budgetMS: 5000)
    }

    private static func launchAgentBinaryMismatch(paths: AgentPaths.Paths) throws -> (Bool, String) {
        guard let currentBinary = try? resolveExecutablePath(), !currentBinary.isEmpty else {
            return (false, "")
        }

        if let registered = readLaunchAgentBinaryPath(paths.plistPath),
           !registered.isEmpty,
           !samePath(currentBinary, registered) {
            return (true, "registered binary \(registered) does not match current \(currentBinary)")
        }

        let idPath = binaryIdentityPath(paths)
        guard !idPath.isEmpty else { return (false, "") }

        if let currentIdentity = try? binaryIdentityForExecutable(currentBinary) {
            if let persistedIdentity = try? readBinaryIdentity(idPath) {
                if persistedIdentity != currentIdentity {
                    return (true, "stored identity does not match current binary \(currentBinary)")
                }
            } else if let registered = readLaunchAgentBinaryPath(paths.plistPath), !registered.isEmpty {
                return (true, "binary identity file missing")
            }
        }

        return (false, "")
    }

    // MARK: - Helpers

    private static func resolveExecutablePath() throws -> String {
        if let path = Bundle.main.executablePath {
            let url = URL(fileURLWithPath: path).resolvingSymlinksInPath()
            return url.path
        }
        guard let argv0 = CommandLine.arguments.first, !argv0.isEmpty else {
            throw XcodeCLIError.bridgeSpawnFailed(underlying: "cannot resolve executable path")
        }
        let url = URL(fileURLWithPath: argv0).resolvingSymlinksInPath()
        return url.path
    }

    private static func isUnavailableError(_ error: Error) -> Bool {
        let desc = error.localizedDescription.lowercased()
        return desc.contains("no such file or directory")
            || desc.contains("connection refused")
            || desc.contains("unavailable")
            || desc.contains("connect to")
    }
}

// MARK: - Path Comparison

func samePath(_ left: String, _ right: String) -> Bool {
    guard !left.isEmpty, !right.isEmpty else { return false }
    let l = (left as NSString).standardizingPath
    let r = (right as NSString).standardizingPath
    return l == r
}
