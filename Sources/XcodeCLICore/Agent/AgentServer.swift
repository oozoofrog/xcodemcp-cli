import Foundation
#if canImport(Darwin)
import Darwin
#endif

// MARK: - Server Configuration

public struct AgentServerConfig: Sendable {
    public let paths: AgentPaths.Paths
    public let label: String
    public let idleTimeout: TimeInterval
    public let baseEnv: [String: String]
    public let debug: Bool
    public let errOut: FileHandle

    public init(
        paths: AgentPaths.Paths? = nil,
        label: String = AgentPaths.label,
        idleTimeout: TimeInterval = defaultAgentIdleTimeout,
        baseEnv: [String: String] = [:],
        debug: Bool = false,
        errOut: FileHandle = .standardError
    ) {
        self.paths = paths ?? AgentPaths.defaultPaths()
        self.label = label
        self.idleTimeout = idleTimeout
        self.baseEnv = baseEnv
        self.debug = debug
        self.errOut = errOut
    }
}

// MARK: - Session Key

private struct SessionKey: Hashable {
    let xcodePID: String
    let sessionID: String
    let developerDir: String
}

// MARK: - Pooled Session

private final class PooledSession: @unchecked Sendable {
    let key: SessionKey
    var client: MCPClient?
    let lock = NSLock()
    var inFlight: Int = 0
    var retireWhenIdle: Bool = false

    init(key: SessionKey) {
        self.key = key
    }
}

// MARK: - Runtime Status

struct RuntimeStatus: Codable, Sendable {
    let pid: Int
    let idleTimeoutMs: Int64
    let backendSessions: Int
}

// MARK: - Agent Server

public final class AgentServer: @unchecked Sendable {
    private let cfg: AgentServerConfig

    private let lock = NSLock()
    private var sessions: [SessionKey: PooledSession] = [:]
    private var activeConnections: Int = 0
    private var idleTimer: DispatchSourceTimer?
    private var closed: Bool = false
    private var serverFD: Int32 = -1

    public init(config: AgentServerConfig) {
        self.cfg = config
    }

    // MARK: - Run Server

    public func run() async throws {
        // Create support directory
        try FileManager.default.createDirectory(atPath: cfg.paths.supportDir, withIntermediateDirectories: true, attributes: nil)

        // Write binary identity
        if let execPath = try? resolveExecutablePath() {
            let identity = try binaryIdentityForExecutable(execPath)
            try writeBinaryIdentity(binaryIdentityPath(cfg.paths), identity: identity)
        }

        // Remove stale socket
        let fm = FileManager.default
        if fm.fileExists(atPath: cfg.paths.socketPath) {
            try fm.removeItem(atPath: cfg.paths.socketPath)
        }

        // Create Unix socket
        let fd = socket(AF_UNIX, SOCK_STREAM, 0)
        guard fd >= 0 else {
            throw XcodeCLIError.agentUnavailable(stage: "socket", underlying: "failed to create unix socket: \(String(cString: strerror(errno)))")
        }
        serverFD = fd

        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let socketPath = cfg.paths.socketPath
        withUnsafeMutablePointer(to: &addr.sun_path) { ptr in
            socketPath.withCString { cStr in
                _ = strncpy(UnsafeMutableRawPointer(ptr).assumingMemoryBound(to: CChar.self), cStr, MemoryLayout.size(ofValue: ptr.pointee) - 1)
            }
        }

        let bindResult = withUnsafePointer(to: &addr) { ptr in
            ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) { sockPtr in
                Darwin.bind(fd, sockPtr, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }
        guard bindResult == 0 else {
            Darwin.close(fd)
            throw XcodeCLIError.agentUnavailable(stage: "bind", underlying: "bind \(socketPath): \(String(cString: strerror(errno)))")
        }

        // chmod 0600
        chmod(socketPath, 0o600)

        guard Darwin.listen(fd, 5) == 0 else {
            Darwin.close(fd)
            throw XcodeCLIError.agentUnavailable(stage: "listen", underlying: "listen: \(String(cString: strerror(errno)))")
        }

        // Write PID file
        try "\(ProcessInfo.processInfo.processIdentifier)\n"
            .write(toFile: cfg.paths.pidPath, atomically: true, encoding: .utf8)

        defer {
            shutdown()
            try? fm.removeItem(atPath: cfg.paths.pidPath)
            try? fm.removeItem(atPath: cfg.paths.socketPath)
        }

        startIdleTimer()

        // Accept loop
        while true {
            let clientFD = Darwin.accept(fd, nil, nil)
            if clientFD < 0 {
                if isClosed() { return }
                if errno == EINTR { continue }
                if errno == EBADF || errno == EINVAL { return }
                continue
            }
            connectionStarted()
            Task { [weak self] in
                guard let self else {
                    Darwin.close(clientFD)
                    return
                }
                await self.handleConnection(clientFD)
            }
        }
    }

    // MARK: - Connection Handling

    private func handleConnection(_ fd: Int32) async {
        defer {
            Darwin.close(fd)
            connectionFinished()
        }

        // Read single line request
        var data = Data()
        var buf = [UInt8](repeating: 0, count: 4096)
        outer: while true {
            let n = Darwin.read(fd, &buf, buf.count)
            if n <= 0 { break }
            data.append(contentsOf: buf[0..<n])
            if data.contains(UInt8(ascii: "\n")) { break outer }
        }

        guard let lineEnd = data.firstIndex(of: UInt8(ascii: "\n")) else {
            writeResponse(fd, AgentResponse(error: "read agent request: no newline found"))
            return
        }

        let lineData = data[data.startIndex..<lineEnd]
        guard let req = try? JSONDecoder().decode(AgentRequest.self, from: lineData) else {
            writeResponse(fd, AgentResponse(error: "decode agent request: invalid JSON"))
            return
        }

        let resp = await dispatch(req)
        writeResponse(fd, resp)

        if req.method == "stop" && resp.error == nil {
            Task { [weak self] in self?.shutdown() }
        }
    }

    private func dispatch(_ req: AgentRequest) async -> AgentResponse {
        switch req.method {
        case "ping":
            return AgentResponse(status: runtimeStatus())
        case "status":
            return AgentResponse(status: runtimeStatus())
        case "stop":
            return AgentResponse()
        case "tools/list":
            return await listTools(req)
        case "tools/call":
            return await callTool(req)
        default:
            return AgentResponse(error: "unsupported agent method \"\(req.method)\"")
        }
    }

    // MARK: - Tool Operations

    private func listTools(_ req: AgentRequest) async -> AgentResponse {
        let (pooled, retired) = prepareSession(sessionKeyForRequest(req))
        abortSessionsAsync(retired)

        do {
            let client = try await getOrCreateClient(pooled: pooled, req: req)
            let tools = try await client.listTools()
            finishSession(pooled)
            return AgentResponse(tools: tools)
        } catch {
            discardClient(pooled)
            finishSession(pooled)
            return AgentResponse(error: error.localizedDescription)
        }
    }

    private func callTool(_ req: AgentRequest) async -> AgentResponse {
        let (pooled, retired) = prepareSession(sessionKeyForRequest(req))
        abortSessionsAsync(retired)

        guard let toolName = req.toolName, !toolName.isEmpty else {
            finishSession(pooled)
            return AgentResponse(error: "tools/call requires toolName")
        }

        do {
            let client = try await getOrCreateClient(pooled: pooled, req: req)
            let result = try await client.callTool(name: toolName, arguments: req.arguments ?? [:])
            finishSession(pooled)
            return AgentResponse(result: result.result, isError: result.isError ? true : nil)
        } catch {
            discardClient(pooled)
            finishSession(pooled)
            return AgentResponse(error: error.localizedDescription)
        }
    }

    /// Get existing client or create a new one. Thread-safe via PooledSession lock.
    private func getOrCreateClient(pooled: PooledSession, req: AgentRequest) async throws -> MCPClient {
        // Check for existing client synchronously
        let existing: MCPClient? = pooled.lock.withLock { pooled.client }
        if let existing { return existing }

        // Create new client (async)
        var env = cfg.baseEnv
        if let pid = req.xcodePID, !pid.isEmpty { env["MCP_XCODE_PID"] = pid }
        if let sid = req.sessionID, !sid.isEmpty { env["MCP_XCODE_SESSION_ID"] = sid }
        if let devDir = req.developerDir, !devDir.isEmpty { env["DEVELOPER_DIR"] = devDir }

        let config = MCPClient.Config(environment: env, debug: false, errOut: cfg.errOut)
        let client = try await MCPClient.connect(config: config)

        // Store it
        pooled.lock.withLock { pooled.client = client }
        return client
    }

    private func discardClient(_ pooled: PooledSession) {
        let client: MCPClient? = pooled.lock.withLock {
            let c = pooled.client
            pooled.client = nil
            return c
        }
        if let client {
            Task { await client.abort() }
        }
    }

    // MARK: - Session Management

    private func sessionKeyForRequest(_ req: AgentRequest) -> SessionKey {
        SessionKey(
            xcodePID: req.xcodePID ?? "",
            sessionID: req.sessionID ?? "",
            developerDir: req.developerDir ?? ""
        )
    }

    private func prepareSession(_ key: SessionKey) -> (PooledSession, [PooledSession]) {
        lock.lock()
        defer { lock.unlock() }

        let pooled = sessions[key] ?? {
            let p = PooledSession(key: key)
            sessions[key] = p
            return p
        }()
        pooled.inFlight += 1
        pooled.retireWhenIdle = false

        var retired: [PooledSession] = []
        for (otherKey, other) in sessions where other !== pooled {
            if other.inFlight == 0 {
                sessions.removeValue(forKey: otherKey)
                other.retireWhenIdle = false
                retired.append(other)
            } else {
                other.retireWhenIdle = true
            }
        }

        return (pooled, retired)
    }

    private func finishSession(_ pooled: PooledSession) {
        var shouldRetire = false
        lock.lock()
        if pooled.inFlight > 0 { pooled.inFlight -= 1 }
        if pooled.inFlight == 0 && pooled.retireWhenIdle {
            if sessions[pooled.key] === pooled {
                sessions.removeValue(forKey: pooled.key)
            }
            pooled.retireWhenIdle = false
            shouldRetire = true
        }
        lock.unlock()

        if shouldRetire {
            abortSessionAsync(pooled)
        }
    }

    private func abortSessionsAsync(_ sessions: [PooledSession]) {
        for pooled in sessions {
            abortSessionAsync(pooled)
        }
    }

    private func abortSessionAsync(_ pooled: PooledSession) {
        pooled.lock.lock()
        let client = pooled.client
        pooled.client = nil
        pooled.lock.unlock()
        if let client {
            Task { await client.abort() }
        }
    }

    // MARK: - Idle Timer

    private func startIdleTimer() {
        lock.lock()
        defer { lock.unlock() }
        stopIdleTimerLocked()
        let timer = DispatchSource.makeTimerSource(queue: .global())
        timer.schedule(deadline: .now() + cfg.idleTimeout)
        timer.setEventHandler { [weak self] in self?.shutdown() }
        timer.resume()
        idleTimer = timer
    }

    private func stopIdleTimerLocked() {
        idleTimer?.cancel()
        idleTimer = nil
    }

    private func connectionStarted() {
        lock.lock()
        defer { lock.unlock() }
        activeConnections += 1
        stopIdleTimerLocked()
    }

    private func connectionFinished() {
        lock.lock()
        defer { lock.unlock() }
        if activeConnections > 0 { activeConnections -= 1 }
        if activeConnections == 0 && !closed {
            stopIdleTimerLocked()
            let timer = DispatchSource.makeTimerSource(queue: .global())
            timer.schedule(deadline: .now() + cfg.idleTimeout)
            timer.setEventHandler { [weak self] in self?.shutdown() }
            timer.resume()
            idleTimer = timer
        }
    }

    // MARK: - Status

    private func runtimeStatus() -> AgentStatus {
        lock.lock()
        defer { lock.unlock() }
        var backendSessions = 0
        for (_, pooled) in sessions {
            if pooled.client != nil { backendSessions += 1 }
        }
        return AgentStatus(
            label: cfg.label,
            plistPath: cfg.paths.plistPath,
            plistInstalled: FileManager.default.fileExists(atPath: cfg.paths.plistPath),
            socketPath: cfg.paths.socketPath,
            socketReachable: true,
            running: true,
            pid: Int(ProcessInfo.processInfo.processIdentifier),
            idleTimeoutNs: Int64(cfg.idleTimeout * 1_000_000_000),
            backendSessions: backendSessions
        )
    }

    // MARK: - Shutdown

    public func shutdown() {
        lock.lock()
        if closed {
            lock.unlock()
            return
        }
        closed = true
        stopIdleTimerLocked()
        let fd = serverFD
        serverFD = -1
        let allSessions = Array(sessions.values)
        lock.unlock()

        if fd >= 0 {
            Darwin.close(fd)
        }
        for pooled in allSessions {
            pooled.lock.lock()
            if let client = pooled.client {
                Task { await client.close() }
                pooled.client = nil
            }
            pooled.lock.unlock()
        }
    }

    private func isClosed() -> Bool {
        lock.lock()
        defer { lock.unlock() }
        return closed
    }

    // MARK: - I/O Helpers

    private func writeResponse(_ fd: Int32, _ resp: AgentResponse) {
        guard let data = try? JSONEncoder().encode(resp) else { return }
        var payload = data
        payload.append(UInt8(ascii: "\n"))
        payload.withUnsafeBytes { ptr in
            _ = Darwin.write(fd, ptr.baseAddress, ptr.count)
        }
    }

    private func resolveExecutablePath() throws -> String {
        let path = Bundle.main.executablePath ?? CommandLine.arguments.first ?? ""
        guard !path.isEmpty else {
            throw XcodeCLIError.bridgeSpawnFailed(underlying: "cannot resolve executable path")
        }
        return (path as NSString).standardizingPath
    }
}
