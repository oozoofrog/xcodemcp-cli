import Foundation
import XcodeCLICore

struct EnvironmentStepError: Equatable, Sendable {
    let step: String
    let message: String
}

struct WindowsToolExecutionData: Sendable {
    let toolName: String
    var attempted: Bool = false
    var ok: Bool = false
    var result: [String: JSONValue]?
    var error: EnvironmentStepError?
}

struct ReadOnlyEnvironmentCollection: Sendable {
    let doctorReport: DoctorReport
    let postToolsStatus: AgentStatus?
    let tools: [JSONValue]?
    let toolCatalog: ToolCatalogData
    let windowsTool: WindowsToolExecutionData
    let errors: [EnvironmentStepError]
}

typealias AgentStatusProvider = @Sendable () async throws -> AgentStatus
typealias AgentToolsProvider = @Sendable (AgentRequest) async throws -> [JSONValue]
typealias AgentToolCaller = @Sendable (AgentRequest, String, [String: JSONValue]) async throws -> MCPCallResult
typealias DoctorRunner = @Sendable (DoctorOptions) async -> DoctorReport

func collectReadOnlyEnvironment(
    env: [String: String],
    effective: EnvOptions,
    resolved: ResolvedOptions,
    timeout: Int,
    debug: Bool,
    highlightToolNames: [String],
    windowsToolName: String = "XcodeListWindows",
    windowsStep: String,
    statusProvider: AgentStatusProvider = { try await AgentClient.status() },
    toolsProvider: AgentToolsProvider = { try await AgentClient.listTools(request: $0) },
    toolCaller: AgentToolCaller = { try await AgentClient.callTool(request: $0, name: $1, arguments: $2) },
    doctorRunner: DoctorRunner = { opts in
        let doctorInspector = DoctorInspector(processRunner: SystemProcessRunner())
        return await doctorInspector.run(opts: opts)
    }
) async -> ReadOnlyEnvironmentCollection {
    let initialStatus = try? await statusProvider()

    let doctorReport = await doctorRunner(DoctorOptions(
        baseEnv: env,
        xcodePID: effective.xcodePID,
        sessionID: effective.sessionID,
        sessionSource: resolved.sessionSource,
        sessionPath: resolved.sessionPath,
        agentStatus: initialStatus
    ))

    let bridgeEnv = EnvOptions.applyOverrides(baseEnv: env, opts: effective)
    let request = buildAgentRequest(
        env: bridgeEnv,
        effective: effective,
        timeout: TimeInterval(timeout),
        debug: debug
    )

    var errors: [EnvironmentStepError] = []
    var tools: [JSONValue]?
    var toolCatalog = ToolCatalogData(names: [], highlights: [])

    do {
        let listedTools = try await toolsProvider(request)
        tools = listedTools
        toolCatalog = buildToolCatalogData(listedTools, highlightToolNames: highlightToolNames)
    } catch {
        errors.append(EnvironmentStepError(step: "tools list", message: error.localizedDescription))
    }

    let postToolsStatus = try? await statusProvider()

    var windowsTool = WindowsToolExecutionData(toolName: windowsToolName)
    if let tools {
        if findToolByName(tools, windowsToolName) == nil {
            let error = EnvironmentStepError(step: windowsStep, message: "tool not found: \(windowsToolName)")
            windowsTool.error = error
            errors.append(error)
        } else {
            windowsTool.attempted = true
            do {
                let result = try await toolCaller(request, windowsToolName, [:])
                windowsTool.result = result.result
                windowsTool.ok = !result.isError
                if result.isError {
                    let message = extractToolResultMessage(result.result)
                    let error = EnvironmentStepError(
                        step: windowsStep,
                        message: message.isEmpty ? "tool returned isError=true" : message
                    )
                    windowsTool.error = error
                    errors.append(error)
                }
            } catch {
                let error = EnvironmentStepError(step: windowsStep, message: error.localizedDescription)
                windowsTool.error = error
                errors.append(error)
            }
        }
    }

    return ReadOnlyEnvironmentCollection(
        doctorReport: doctorReport,
        postToolsStatus: postToolsStatus,
        tools: tools,
        toolCatalog: toolCatalog,
        windowsTool: windowsTool,
        errors: errors
    )
}
