import Testing
import Foundation
@testable import XcodeCLICore

@Suite("MCP Server")
struct MCPServerTests {

    // MARK: - Helpers

    /// Build a standard JSON-RPC request string.
    private func jsonRequest(id: Any, method: String, params: [String: Any] = [:]) -> String {
        var dict: [String: Any] = ["jsonrpc": "2.0", "id": id, "method": method]
        if !params.isEmpty {
            dict["params"] = params
        }
        let data = try! JSONSerialization.data(withJSONObject: dict, options: [.sortedKeys])
        return String(data: data, encoding: .utf8)!
    }

    /// Build a JSON-RPC notification string (no id).
    private func jsonNotification(method: String, params: [String: Any] = [:]) -> String {
        var dict: [String: Any] = ["jsonrpc": "2.0", "method": method]
        if !params.isEmpty {
            dict["params"] = params
        }
        let data = try! JSONSerialization.data(withJSONObject: dict, options: [.sortedKeys])
        return String(data: data, encoding: .utf8)!
    }

    /// Create a default handler that returns one tool and echoes call results.
    private func defaultHandler() -> MCPServerHandler {
        MCPServerHandler(
            listTools: { [.object(["name": .string("TestTool")])] },
            callTool: { name, args in
                MCPCallResult(result: ["content": .string("ok"), "echoName": .string(name)])
            }
        )
    }

    /// Start a server task with Pipe-based I/O and return the components.
    private func startServer(
        handler: MCPServerHandler? = nil,
        debug: Bool = false
    ) -> (stdinWrite: FileHandle, stdoutRead: FileHandle, serverTask: Task<Void, any Error>) {
        let stdinPipe = Pipe()
        let stdoutPipe = Pipe()
        let stderrPipe = Pipe()

        let h = handler ?? defaultHandler()
        let serverTask = Task {
            try await serveMCPStdio(
                config: MCPServerConfig(debug: debug),
                handler: h,
                stdin: stdinPipe.fileHandleForReading,
                stdout: stdoutPipe.fileHandleForWriting,
                stderr: stderrPipe.fileHandleForWriting
            )
        }

        return (stdinPipe.fileHandleForWriting, stdoutPipe.fileHandleForReading, serverTask)
    }

    // MARK: - Tests

    @Test("initialize returns protocolVersion and serverInfo")
    func initializeReturnsProtocolVersionAndServerInfo() async throws {
        let (stdinWrite, stdoutRead, serverTask) = startServer()

        writeLine(stdinWrite, jsonRequest(id: 1, method: "initialize", params: [
            "protocolVersion": MCPConstants.requestProtocolVersion,
        ]))

        let responseLine = readLine(from: stdoutRead)
        stdinWrite.closeFile()
        try? await serverTask.value

        let response = try #require(responseLine)
        let envelope = try decodeEnvelope(response)
        #expect(envelope.error == nil)

        guard case .object(let result) = envelope.result else {
            Issue.record("Expected object result")
            return
        }
        guard case .string(let version) = result["protocolVersion"] else {
            Issue.record("Missing protocolVersion in result")
            return
        }
        #expect(version == MCPConstants.requestProtocolVersion)

        guard case .object(let serverInfo) = result["serverInfo"] else {
            Issue.record("Missing serverInfo in result")
            return
        }
        guard case .string(let name) = serverInfo["name"] else {
            Issue.record("Missing name in serverInfo")
            return
        }
        #expect(name == "xcodecli")
    }

    @Test("initialize negotiates supported version 2024-11-05")
    func initializeNegotiatesSupportedVersion() async throws {
        let (stdinWrite, stdoutRead, serverTask) = startServer()

        writeLine(stdinWrite, jsonRequest(id: 1, method: "initialize", params: [
            "protocolVersion": "2024-11-05",
        ]))

        let responseLine = readLine(from: stdoutRead)
        stdinWrite.closeFile()
        try? await serverTask.value

        let response = try #require(responseLine)
        let envelope = try decodeEnvelope(response)
        #expect(envelope.error == nil)

        guard case .object(let result) = envelope.result,
              case .string(let version) = result["protocolVersion"] else {
            Issue.record("Missing protocolVersion in result")
            return
        }
        #expect(version == "2024-11-05")
    }

    @Test("initialize rejects unsupported version")
    func initializeRejectsUnsupportedVersion() async throws {
        let (stdinWrite, stdoutRead, serverTask) = startServer()

        writeLine(stdinWrite, jsonRequest(id: 1, method: "initialize", params: [
            "protocolVersion": "1999-01-01",
        ]))

        let responseLine = readLine(from: stdoutRead)
        stdinWrite.closeFile()
        try? await serverTask.value

        let response = try #require(responseLine)
        let envelope = try decodeEnvelope(response)
        #expect(envelope.error?.code == -32602)

        // Verify error data includes requested and supported
        guard case .object(let data) = envelope.error?.data else {
            Issue.record("Missing error data")
            return
        }
        guard case .string(let requested) = data["requested"] else {
            Issue.record("Missing requested in error data")
            return
        }
        #expect(requested == "1999-01-01")

        guard case .array(let supported) = data["supported"] else {
            Issue.record("Missing supported in error data")
            return
        }
        #expect(supported.count == MCPConstants.supportedProtocolVersions.count)
    }

    @Test("initialize with empty params uses default protocol version")
    func initializeRejectsMissingProtocolVersion() async throws {
        let (stdinWrite, stdoutRead, serverTask) = startServer()

        // Empty params: server defaults to requestProtocolVersion
        writeLine(stdinWrite, jsonRequest(id: 1, method: "initialize", params: [:]))

        let responseLine = readLine(from: stdoutRead)
        stdinWrite.closeFile()
        try? await serverTask.value

        let response = try #require(responseLine)
        let envelope = try decodeEnvelope(response)

        // The Swift server defaults to its own requestProtocolVersion when none specified
        #expect(envelope.error == nil)
        guard case .object(let result) = envelope.result,
              case .string(let version) = result["protocolVersion"] else {
            Issue.record("Missing protocolVersion in result")
            return
        }
        #expect(version == MCPConstants.requestProtocolVersion)
    }

    @Test("ping returns empty result")
    func pingReturnsEmptyResult() async throws {
        let (stdinWrite, stdoutRead, serverTask) = startServer()

        writeLine(stdinWrite, jsonRequest(id: 1, method: "ping"))

        let responseLine = readLine(from: stdoutRead)
        stdinWrite.closeFile()
        try? await serverTask.value

        let response = try #require(responseLine)
        let envelope = try decodeEnvelope(response)
        #expect(envelope.error == nil)

        guard case .object(let result) = envelope.result else {
            Issue.record("Expected object result for ping")
            return
        }
        #expect(result.isEmpty)
    }

    @Test("unknown method returns -32601 Method not found")
    func unknownMethodReturnsMethodNotFound() async throws {
        let (stdinWrite, stdoutRead, serverTask) = startServer()

        writeLine(stdinWrite, jsonRequest(id: 1, method: "bogus/method", params: [:]))

        let responseLine = readLine(from: stdoutRead)
        stdinWrite.closeFile()
        try? await serverTask.value

        let response = try #require(responseLine)
        let envelope = try decodeEnvelope(response)
        #expect(envelope.error?.code == -32601)
    }

    @Test("tools/list returns tools array")
    func toolsListReturnsTools() async throws {
        let (stdinWrite, stdoutRead, serverTask) = startServer()

        // Send initialize first
        writeLine(stdinWrite, jsonRequest(id: 1, method: "initialize", params: [
            "protocolVersion": MCPConstants.requestProtocolVersion,
        ]))
        let _ = readLine(from: stdoutRead)

        // Send tools/list
        writeLine(stdinWrite, jsonRequest(id: 2, method: "tools/list", params: [:]))

        let responseLine = readLine(from: stdoutRead)
        stdinWrite.closeFile()
        try? await serverTask.value

        let response = try #require(responseLine)
        let envelope = try decodeEnvelope(response)
        #expect(envelope.error == nil)

        guard case .object(let result) = envelope.result,
              case .array(let tools) = result["tools"] else {
            Issue.record("Missing tools array in result")
            return
        }
        #expect(tools.count == 1)
        if case .object(let tool) = tools.first,
           case .string(let name) = tool["name"] {
            #expect(name == "TestTool")
        }
    }

    @Test("tools/call returns result with echoed name")
    func toolsCallReturnsResult() async throws {
        let (stdinWrite, stdoutRead, serverTask) = startServer()

        // Initialize
        writeLine(stdinWrite, jsonRequest(id: 1, method: "initialize", params: [
            "protocolVersion": MCPConstants.requestProtocolVersion,
        ]))
        let _ = readLine(from: stdoutRead)

        // Call tool
        writeLine(stdinWrite, jsonRequest(id: 2, method: "tools/call", params: [
            "name": "TestTool",
            "arguments": ["key": "value"] as [String: String],
        ]))

        let responseLine = readLine(from: stdoutRead)
        stdinWrite.closeFile()
        try? await serverTask.value

        let response = try #require(responseLine)
        let envelope = try decodeEnvelope(response)
        #expect(envelope.error == nil)

        guard case .object(let result) = envelope.result else {
            Issue.record("Missing result object")
            return
        }
        guard case .string(let echoName) = result["echoName"] else {
            Issue.record("Missing echoName in result")
            return
        }
        #expect(echoName == "TestTool")
    }

    @Test("cancellation suppresses response for delayed request")
    func cancellationSuppressesResponse() async throws {
        let callStarted = LockedFlag()
        let gate = TestGate()

        let handler = MCPServerHandler(
            listTools: { [.object(["name": .string("TestTool")])] },
            callTool: { _, _ in
                callStarted.set()
                await gate.wait() // Block until test opens the gate
                return MCPCallResult(result: ["content": .string("late")])
            }
        )
        let (stdinWrite, stdoutRead, serverTask) = startServer(handler: handler)

        // Send a slow tools/call
        writeLine(stdinWrite, jsonRequest(id: 1, method: "tools/call", params: [
            "name": "TestTool",
            "arguments": [:] as [String: String],
        ]))

        // Wait for the handler to start
        await callStarted.wait()

        // Cancel it
        writeLine(stdinWrite, jsonNotification(method: "notifications/cancelled", params: [
            "requestId": 1,
            "reason": "test cancel",
        ]))

        // Give the server time to process the cancellation notification
        try await Task.sleep(nanoseconds: 50_000_000) // 50ms

        // Unblock the handler so it can finish (but response should be suppressed)
        gate.open()

        // Send a tools/list that should respond normally
        writeLine(stdinWrite, jsonRequest(id: 2, method: "tools/list", params: [:]))

        let responseLine = readLine(from: stdoutRead)
        stdinWrite.closeFile()
        try? await serverTask.value

        // The first response we get should be for id=2, not id=1
        let response = try #require(responseLine)
        let envelope = try decodeEnvelope(response)
        #expect(envelope.id == .int(2))
    }

    @Test("duplicate request ID returns -32600 error")
    func duplicateRequestIDReturnsError() async throws {
        let callStarted = LockedFlag()
        let gate = TestGate()

        let handler = MCPServerHandler(
            listTools: {
                callStarted.set()
                await gate.wait() // Block until test opens the gate
                return [.object(["name": .string("TestTool")])]
            },
            callTool: { _, _ in MCPCallResult(result: ["content": .string("ok")]) }
        )
        let (stdinWrite, stdoutRead, serverTask) = startServer(handler: handler)

        // Send first tools/list (will block in handler)
        writeLine(stdinWrite, jsonRequest(id: 1, method: "tools/list", params: [:]))

        // Wait for the handler to start
        await callStarted.wait()

        // Send duplicate tools/list with same id
        writeLine(stdinWrite, jsonRequest(id: 1, method: "tools/list", params: [:]))

        // Read the duplicate error response (comes first because it's synchronous)
        let responseLine = readLine(from: stdoutRead)

        // Unblock the first handler so the server can clean up
        gate.open()

        stdinWrite.closeFile()
        try? await serverTask.value

        let response = try #require(responseLine)
        let envelope = try decodeEnvelope(response)
        #expect(envelope.error?.code == -32600)
    }
}

// MARK: - Thread-safe Flag

/// A thread-safe boolean flag for coordinating between tasks in tests.
private final class LockedFlag: @unchecked Sendable {
    private let lock = NSLock()
    private var _value = false
    private var continuation: CheckedContinuation<Void, Never>?

    var value: Bool {
        lock.withLock { _value }
    }

    func set() {
        lock.lock()
        _value = true
        let cont = continuation
        continuation = nil
        lock.unlock()
        cont?.resume()
    }

    /// Wait until the flag is set. Returns immediately if already set.
    func wait() async {
        let alreadySet = lock.withLock { _value }
        if alreadySet { return }
        await withCheckedContinuation { cont in
            let shouldResume = lock.withLock { () -> Bool in
                if _value { return true }
                continuation = cont
                return false
            }
            if shouldResume { cont.resume() }
        }
    }
}

/// A signal that a handler can await, allowing the test to unblock it on demand.
private final class TestGate: @unchecked Sendable {
    private var continuation: CheckedContinuation<Void, Never>?
    private let lock = NSLock()

    /// Called by the handler to wait until the test opens the gate.
    func wait() async {
        await withCheckedContinuation { cont in
            lock.withLock { continuation = cont }
        }
    }

    /// Called by the test to unblock the handler.
    func open() {
        let cont = lock.withLock { () -> CheckedContinuation<Void, Never>? in
            let c = continuation
            continuation = nil
            return c
        }
        cont?.resume()
    }
}
