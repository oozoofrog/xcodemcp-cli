import Foundation

/// Default idle timeout for agent sessions (24 hours, in nanoseconds for Go compatibility).
public let defaultAgentIdleTimeoutNs: Int64 = 24 * 60 * 60 * 1_000_000_000

/// Default idle timeout for agent sessions (24 hours).
public let defaultAgentIdleTimeout: TimeInterval = 24 * 60 * 60

/// Agent RPC request for communicating with the LaunchAgent.
public struct AgentRequest: Codable, Sendable {
    public var method: String
    public var toolName: String?
    public var arguments: [String: JSONValue]?
    public var timeoutMS: Int64?
    public var xcodePID: String?
    public var sessionID: String?
    public var developerDir: String?
    public var debug: Bool?

    public init(
        method: String,
        toolName: String? = nil,
        arguments: [String: JSONValue]? = nil,
        timeoutMS: Int64? = nil,
        xcodePID: String? = nil,
        sessionID: String? = nil,
        developerDir: String? = nil,
        debug: Bool? = nil
    ) {
        self.method = method
        self.toolName = toolName
        self.arguments = arguments
        self.timeoutMS = timeoutMS
        self.xcodePID = xcodePID
        self.sessionID = sessionID
        self.developerDir = developerDir
        self.debug = debug
    }
}

/// Agent RPC response.
public struct AgentResponse: Codable, Sendable {
    public var tools: [JSONValue]?
    public var result: [String: JSONValue]?
    public var isError: Bool?
    public var error: String?
    public var status: AgentStatus?

    public init(
        tools: [JSONValue]? = nil,
        result: [String: JSONValue]? = nil,
        isError: Bool? = nil,
        error: String? = nil,
        status: AgentStatus? = nil
    ) {
        self.tools = tools
        self.result = result
        self.isError = isError
        self.error = error
        self.status = status
    }
}

/// Build an agent request from resolved environment options.
public func buildAgentRequest(
    env: [String: String],
    effective: EnvOptions,
    timeout: TimeInterval,
    debug: Bool
) -> AgentRequest {
    AgentRequest(
        method: "",
        timeoutMS: Int64(timeout * 1000),
        xcodePID: effective.xcodePID.isEmpty ? nil : effective.xcodePID,
        sessionID: effective.sessionID.isEmpty ? nil : effective.sessionID,
        developerDir: env["DEVELOPER_DIR"],
        debug: debug
    )
}
