import Foundation

/// Configuration for the MCP stdio server.
public struct MCPServerConfig: Sendable {
    public let serverName: String
    public let serverVersion: String
    public let debug: Bool

    public init(
        serverName: String = "xcodecli",
        serverVersion: String = Version.current,
        debug: Bool = false
    ) {
        self.serverName = serverName
        self.serverVersion = serverVersion
        self.debug = debug
    }
}

/// Handler callbacks for the MCP server.
public struct MCPServerHandler: Sendable {
    public let listTools: @Sendable () async throws -> [JSONValue]
    public let callTool: @Sendable (String, [String: JSONValue]) async throws -> MCPCallResult

    public init(
        listTools: @escaping @Sendable () async throws -> [JSONValue],
        callTool: @escaping @Sendable (String, [String: JSONValue]) async throws -> MCPCallResult
    ) {
        self.listTools = listTools
        self.callTool = callTool
    }
}

// MARK: - In-flight Request Tracking

struct InFlightRequest {
    let task: Task<Void, Never>
    var cancelled: Bool = false
}

final class InFlightTracker: @unchecked Sendable {
    private let lock = NSLock()
    private var requests: [String: InFlightRequest] = [:]

    /// Register a new in-flight request. Returns false if the key already exists (duplicate).
    func register(key: String, task: Task<Void, Never>) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        if requests[key] != nil {
            return false
        }
        requests[key] = InFlightRequest(task: task)
        return true
    }

    /// Mark a request as finished. Returns true if the request was not cancelled.
    func finish(key: String) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        guard let req = requests.removeValue(forKey: key) else {
            return false
        }
        return !req.cancelled
    }

    /// Cancel a specific request. Returns true if found.
    func cancel(key: String) -> Bool {
        lock.lock()
        let req = requests[key]
        if req != nil {
            requests[key]?.cancelled = true
        }
        lock.unlock()
        // Cancel the task outside the lock
        req?.task.cancel()
        return req != nil
    }

    /// Cancel all in-flight requests.
    func cancelAll() {
        lock.lock()
        let allRequests = requests.values
        for key in requests.keys {
            requests[key]?.cancelled = true
        }
        requests.removeAll()
        lock.unlock()
        for req in allRequests {
            req.task.cancel()
        }
    }
}

// MARK: - Canonical Request Key

func canonicalRequestKey(_ id: JSONValue?) -> String? {
    guard let id else { return nil }
    switch id {
    case .null:
        return nil
    case .int(let v):
        return "\(v)"
    case .double(let v):
        return String(format: "%g", v)
    case .string(let v):
        // JSON-encode the string to get quoted form
        if let data = try? JSONEncoder().encode(v) {
            return String(data: data, encoding: .utf8)
        }
        return "\"\(v)\""
    default:
        return nil
    }
}

func canonicalRequestKeyFromCancelParams(_ params: JSONValue?) -> String? {
    guard case .object(let obj) = params,
          let requestId = obj["requestId"] else {
        return nil
    }
    return canonicalRequestKey(requestId)
}

// MARK: - Server Entry Point

/// Run an MCP stdio server reading from stdin and writing to stdout.
public func serveMCPStdio(
    config: MCPServerConfig,
    handler: MCPServerHandler,
    stdin: FileHandle = .standardInput,
    stdout: FileHandle = .standardOutput,
    stderr: FileHandle = .standardError
) async throws {
    let writer = MCPResponseWriter(handle: stdout, debug: config.debug, errOut: stderr)
    let tracker = InFlightTracker()

    defer { tracker.cancelAll() }

    var lineData = Data()
    while true {
        let byte = stdin.readData(ofLength: 1)
        if byte.isEmpty {
            tracker.cancelAll()
            return
        }
        if byte[0] == UInt8(ascii: "\n") {
            if let line = String(data: lineData, encoding: .utf8) {
                let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
                if !trimmed.isEmpty {
                    if config.debug {
                        stderr.write(Data("[debug] mcp serve recv <- \(trimmed)\n".utf8))
                    }
                    do {
                        let envelope = try JSONLineCodec.decode(trimmed)
                        try handleServerEnvelope(
                            envelope: envelope,
                            config: config,
                            handler: handler,
                            writer: writer,
                            tracker: tracker,
                            errOut: stderr
                        )
                    } catch {
                        if config.debug {
                            stderr.write(Data("[debug] mcp serve error: \(error)\n".utf8))
                        }
                        tracker.cancelAll()
                        return
                    }
                }
            }
            lineData = Data()
        } else {
            lineData.append(byte)
        }
    }
}

// MARK: - Envelope Handling

private func handleServerEnvelope(
    envelope: RPCEnvelope,
    config: MCPServerConfig,
    handler: MCPServerHandler,
    writer: MCPResponseWriter,
    tracker: InFlightTracker,
    errOut: FileHandle
) throws {
    // If no method, it's a response — reject if it has an ID
    guard let method = envelope.method, !method.isEmpty else {
        if envelope.hasID {
            try writer.write(rpcErrorResponse(id: envelope.id, code: -32600, message: "Invalid Request"))
        }
        return
    }

    // Notification (no id)
    guard envelope.hasID else {
        handleNotification(envelope: envelope, config: config, tracker: tracker, errOut: errOut)
        return
    }

    switch method {
    case "initialize":
        let response = buildInitializeResponse(envelope: envelope, config: config)
        try writer.write(response)

    case "ping":
        try writer.write(rpcSuccessResponse(id: envelope.id, result: .object([:])))

    case "tools/list", "tools/call":
        try startAsyncRequest(
            envelope: envelope,
            method: method,
            config: config,
            handler: handler,
            writer: writer,
            tracker: tracker,
            errOut: errOut
        )

    default:
        try writer.write(rpcErrorResponse(id: envelope.id, code: -32601, message: "Method not found"))
    }
}

private func handleNotification(
    envelope: RPCEnvelope,
    config: MCPServerConfig,
    tracker: InFlightTracker,
    errOut: FileHandle
) {
    switch envelope.method {
    case "notifications/cancelled":
        guard let requestKey = canonicalRequestKeyFromCancelParams(envelope.params) else {
            if config.debug {
                errOut.write(Data("[debug] mcp serve ignored malformed cancellation\n".utf8))
            }
            return
        }
        if !tracker.cancel(key: requestKey) && config.debug {
            errOut.write(Data("[debug] mcp serve cancellation ignored for unknown request: \(requestKey)\n".utf8))
        }
    default:
        if config.debug {
            errOut.write(Data("[debug] mcp serve notification ignored: \(envelope.method ?? "")\n".utf8))
        }
    }
}

private func startAsyncRequest(
    envelope: RPCEnvelope,
    method: String,
    config: MCPServerConfig,
    handler: MCPServerHandler,
    writer: MCPResponseWriter,
    tracker: InFlightTracker,
    errOut: FileHandle
) throws {
    guard let requestKey = canonicalRequestKey(envelope.id) else {
        try writer.write(rpcErrorResponse(id: envelope.id, code: -32600, message: "request id must be a JSON string or number"))
        return
    }

    // Create the async task
    let task = Task {
        let response = await processRequest(envelope: envelope, method: method, handler: handler)
        guard tracker.finish(key: requestKey) else {
            // Request was cancelled — suppress response
            return
        }
        guard let response else { return }
        do {
            try writer.write(response)
        } catch {
            if config.debug {
                errOut.write(Data("[debug] mcp serve write failed: \(error)\n".utf8))
            }
        }
    }

    // Register — if duplicate, cancel the task and return error
    if !tracker.register(key: requestKey, task: task) {
        task.cancel()
        try writer.write(rpcErrorResponse(id: envelope.id, code: -32600, message: "request id is already in progress"))
    }
}

private func processRequest(
    envelope: RPCEnvelope,
    method: String,
    handler: MCPServerHandler
) async -> RPCEnvelope? {
    switch method {
    case "tools/list":
        do {
            let tools = try await handler.listTools()
            return rpcSuccessResponse(
                id: envelope.id,
                result: .object(["tools": .array(tools)])
            )
        } catch {
            return rpcErrorResponse(id: envelope.id, code: -32603, message: error.localizedDescription)
        }

    case "tools/call":
        do {
            let (name, arguments) = try decodeToolCallParams(envelope.params)
            let result = try await handler.callTool(name, arguments)
            var payload = result.result
            if result.isError {
                payload["isError"] = .bool(true)
            }
            return rpcSuccessResponse(id: envelope.id, result: .object(payload))
        } catch {
            return rpcErrorResponse(id: envelope.id, code: -32602, message: error.localizedDescription)
        }

    default:
        return rpcErrorResponse(id: envelope.id, code: -32601, message: "Method not found")
    }
}

// MARK: - Initialize Response

private func buildInitializeResponse(envelope: RPCEnvelope, config: MCPServerConfig) -> RPCEnvelope {
    var requestedVersion = MCPConstants.requestProtocolVersion
    if case .object(let params) = envelope.params,
       case .string(let version) = params["protocolVersion"] {
        requestedVersion = version
    }

    guard MCPConstants.isSupportedVersion(requestedVersion) else {
        return rpcErrorResponse(
            id: envelope.id, code: -32602,
            message: "Unsupported protocol version",
            data: .object([
                "requested": .string(requestedVersion),
                "supported": .array(MCPConstants.supportedProtocolVersions.map { .string($0) }),
            ])
        )
    }

    return rpcSuccessResponse(id: envelope.id, result: .object([
        "protocolVersion": .string(requestedVersion),
        "capabilities": .object(["tools": .object([:])]),
        "serverInfo": .object([
            "name": .string(config.serverName),
            "version": .string(config.serverVersion),
        ]),
    ]))
}

// MARK: - Tool Call Params Decoding

func decodeToolCallParams(_ params: JSONValue?) throws -> (String, [String: JSONValue]) {
    guard case .object(let obj) = params else {
        throw XcodeCLIError.mcpRPCError(code: -32602, message: "tools/call params must be a JSON object")
    }
    guard case .string(let name) = obj["name"], !name.trimmingCharacters(in: .whitespaces).isEmpty else {
        throw XcodeCLIError.mcpRPCError(code: -32602, message: "tools/call params require a non-empty name")
    }
    var arguments: [String: JSONValue] = [:]
    if case .object(let args) = obj["arguments"] {
        arguments = args
    }
    return (name, arguments)
}

// MARK: - Response Writer

/// Thread-safe response writer for MCP stdout.
final class MCPResponseWriter: @unchecked Sendable {
    private let handle: FileHandle
    private let debug: Bool
    private let errOut: FileHandle
    private let lock = NSLock()

    init(handle: FileHandle, debug: Bool, errOut: FileHandle) {
        self.handle = handle
        self.debug = debug
        self.errOut = errOut
    }

    func write(_ envelope: RPCEnvelope) throws {
        let line = try JSONLineCodec.encode(envelope)
        if debug {
            errOut.write(Data("[debug] mcp serve send -> \(line.trimmingCharacters(in: .whitespacesAndNewlines))\n".utf8))
        }
        lock.withLock {
            handle.write(Data(line.utf8))
        }
    }
}
