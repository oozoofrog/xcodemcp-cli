import ArgumentParser
import Foundation
import XcodeCLICore

// MARK: - Constants

private let demoWindowsToolName = "XcodeListWindows"
private let demoHighlightToolNames = [
    demoWindowsToolName, "XcodeLS", "XcodeRead", "BuildProject", "RunAllTests",
]

// MARK: - Types

private struct DemoDemoStepError: Codable {
    let step: String
    let message: String
}

private struct DemoToolHighlight: Codable {
    let name: String
    let description: String
    let requiredArgs: [String]
}

private struct DemoToolCatalog: Codable {
    let count: Int
    let names: [String]
    let highlights: [DemoToolHighlight]
}

private struct DemoWindowsResult: Codable {
    var attempted: Bool = false
    var ok: Bool = false
    let toolName: String
    var arguments: [String: JSONValue] = [:]
    var result: [String: JSONValue]?
    var error: DemoDemoStepError?
}

private struct AgentDemoReport: Codable {
    var success: Bool
    let doctor: DoctorJSONReport
    var agentStatus: AgentStatus?
    var toolCatalog: DemoToolCatalog
    var windowsDemo: DemoWindowsResult
    let nextCommands: [String]
    var errors: [DemoDemoStepError]
}

// MARK: - Entry Point

func runAgentDemo(
    json: Bool, timeout: Int,
    xcodePID: String?, sessionID: String?, debug: Bool
) async throws {
    let env = envDictionary()
    let (effective, resolved) = try resolveOptions(env: env, xcodePID: xcodePID, sessionID: sessionID)

    let initialStatus = try? await AgentClient.status()

    let doctorInspector = DoctorInspector(processRunner: SystemProcessRunner())
    let doctorReport = await doctorInspector.run(opts: DoctorOptions(
        baseEnv: env, xcodePID: effective.xcodePID, sessionID: effective.sessionID,
        sessionSource: resolved.sessionSource, sessionPath: resolved.sessionPath,
        agentStatus: initialStatus
    ))

    var errors: [DemoDemoStepError] = []
    let bridgeEnv = EnvOptions.applyOverrides(baseEnv: env, opts: effective)
    let request = buildAgentRequest(env: bridgeEnv, effective: effective, timeout: TimeInterval(timeout), debug: debug)

    // List tools
    var toolCatalog = DemoToolCatalog(count: 0, names: [], highlights: [])
    var tools: [JSONValue]?
    do {
        let t = try await AgentClient.listTools(request: request)
        tools = t
        toolCatalog = buildDemoToolCatalog(t)
    } catch {
        errors.append(DemoDemoStepError(step: "tools list", message: error.localizedDescription))
    }

    // Post-tools agent status
    let postStatus = try? await AgentClient.status()

    // Windows demo
    var windowsDemo = DemoWindowsResult(toolName: demoWindowsToolName, arguments: [:])
    if let tools {
        if findToolByName(tools, demoWindowsToolName) == nil {
            windowsDemo.error = DemoDemoStepError(step: "windows demo", message: "tool not found: XcodeListWindows")
            errors.append(windowsDemo.error!)
        } else {
            windowsDemo.attempted = true
            do {
                let result = try await AgentClient.callTool(request: request, name: demoWindowsToolName, arguments: [:])
                windowsDemo.result = result.result
                windowsDemo.ok = !result.isError
                if result.isError {
                    let msg = extractToolResultMessage(result.result)
                    windowsDemo.error = DemoDemoStepError(step: "windows demo", message: msg.isEmpty ? "tool returned isError=true" : msg)
                    errors.append(windowsDemo.error!)
                }
            } catch {
                windowsDemo.error = DemoDemoStepError(step: "windows demo", message: error.localizedDescription)
                errors.append(windowsDemo.error!)
            }
        }
    }

    let success = doctorReport.isSuccess && tools != nil && windowsDemo.attempted && windowsDemo.ok

    let report = AgentDemoReport(
        success: success,
        doctor: doctorReport.jsonReport,
        agentStatus: postStatus,
        toolCatalog: toolCatalog,
        windowsDemo: windowsDemo,
        nextCommands: agentDemoNextCommands(),
        errors: errors
    )

    if json {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        let data = try encoder.encode(report)
        FileHandle.standardOutput.write(data)
        FileHandle.standardOutput.write(Data("\n".utf8))
    } else {
        print(formatAgentDemo(report))
    }

    if !success {
        throw ExitCode(1)
    }
}

// MARK: - Tool Catalog

private func buildDemoToolCatalog(_ tools: [JSONValue]) -> DemoToolCatalog {
    var names: [String] = []
    for tool in tools {
        if case .object(let obj) = tool, case .string(let name) = obj["name"] {
            names.append(name)
        }
    }

    var highlights: [DemoToolHighlight] = []
    for name in demoHighlightToolNames {
        guard let tool = findToolByName(tools, name) else { continue }

        var desc = ""
        var requiredArgs: [String] = []
        if case .object(let obj) = tool {
            if case .string(let d) = obj["description"] { desc = d }
            if case .object(let schema) = obj["inputSchema"],
               case .array(let req) = schema["required"] {
                requiredArgs = req.compactMap { if case .string(let s) = $0 { return s } else { return nil } }
            }
        }
        highlights.append(DemoToolHighlight(name: name, description: desc, requiredArgs: requiredArgs))
    }

    return DemoToolCatalog(count: names.count, names: names, highlights: highlights)
}

// MARK: - Helpers

private func agentDemoNextCommands() -> [String] {
    [
        "xcodecli tool inspect XcodeRead --json --timeout 60",
        "xcodecli tool call XcodeLS --timeout 60 --json '{\"tabIdentifier\":\"<tabIdentifier from above>\",\"path\":\"\"}'",
        "xcodecli tool call XcodeRead --timeout 60 --json '{\"tabIdentifier\":\"<tabIdentifier from above>\",\"filePath\":\"<path from XcodeLS>\"}'",
    ]
}

func extractToolResultMessage(_ result: [String: JSONValue]) -> String {
    if case .object(let structured) = result["structuredContent"],
       case .string(let msg) = structured["message"],
       !msg.trimmingCharacters(in: .whitespaces).isEmpty {
        return msg
    }
    if case .array(let content) = result["content"] {
        let messages = content.compactMap { item -> String? in
            if case .object(let block) = item, case .string(let text) = block["text"],
               !text.trimmingCharacters(in: .whitespaces).isEmpty { return text }
            return nil
        }
        if !messages.isEmpty { return messages.joined(separator: "\n") }
    }
    if let data = try? JSONEncoder().encode(result), let text = String(data: data, encoding: .utf8) {
        return text
    }
    return ""
}

// MARK: - Text Formatting

private func formatAgentDemo(_ report: AgentDemoReport) -> String {
    var b = ""
    b += "xcodecli agent demo\n\n"

    // Environment
    b += "Environment\n-----------\n"
    let doctorState = report.doctor.success ? "ok" : "needs attention"
    let s = report.doctor.summary
    b += "doctor: \(doctorState) (\(s.ok) ok, \(s.warn) warn, \(s.fail) fail, \(s.info) info)\n"

    // Notable checks
    let notable = report.doctor.checks.filter { $0.status != .ok }
    if !notable.isEmpty {
        b += "notable checks:\n"
        for check in notable {
            b += "- \(check.name) [\(check.status.rawValue)]: \(check.detail)\n"
        }
    }

    if let status = report.agentStatus {
        b += "launchagent after tools discovery: running=\(status.running) socketReachable=\(status.socketReachable) backendSessions=\(status.backendSessions)\n"
    } else {
        b += "launchagent after tools discovery: unavailable\n"
    }

    // Tool Catalog
    b += "\nTool Catalog\n------------\n"
    b += "count: \(report.toolCatalog.count)\n"
    if !report.toolCatalog.names.isEmpty {
        b += "names: \(report.toolCatalog.names.joined(separator: ", "))\n"
    } else {
        b += "names: unavailable\n"
    }
    if !report.toolCatalog.highlights.isEmpty {
        b += "highlights:\n"
        for h in report.toolCatalog.highlights {
            let required = h.requiredArgs.isEmpty ? "none" : h.requiredArgs.joined(separator: ", ")
            b += "- \(h.name) (required: \(required)): \(h.description)\n"
        }
    }

    // Safe Live Demo
    b += "\nSafe Live Demo\n--------------\n"
    b += "tool: \(report.windowsDemo.toolName) --json '{}'\n"
    if report.windowsDemo.ok {
        b += "status: ok\n"
        if let result = report.windowsDemo.result {
            let msg = extractToolResultMessage(result)
            if !msg.isEmpty {
                b += "output:\n"
                for line in msg.split(separator: "\n") {
                    b += "  \(line)\n"
                }
            }
        }
    } else if report.windowsDemo.attempted {
        b += "status: failed\n"
        if let err = report.windowsDemo.error {
            b += "error: \(err.message)\n"
        }
    } else {
        b += "status: skipped\n"
        if let err = report.windowsDemo.error {
            b += "reason: \(err.message)\n"
        }
    }

    // Next Commands
    b += "\nNext Commands\n-------------\n"
    for cmd in report.nextCommands {
        b += "- \(cmd)\n"
    }

    return b
}
