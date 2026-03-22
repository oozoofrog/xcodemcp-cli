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
    let environment = await collectReadOnlyEnvironment(
        env: env,
        effective: effective,
        resolved: resolved,
        timeout: timeout,
        debug: debug,
        highlightToolNames: demoHighlightToolNames,
        windowsStep: "windows demo"
    )

    let toolCatalog = DemoToolCatalog(
        count: environment.toolCatalog.count,
        names: environment.toolCatalog.names,
        highlights: environment.toolCatalog.highlights.map {
            DemoToolHighlight(name: $0.name, description: $0.description, requiredArgs: $0.requiredArgs)
        }
    )
    let windowsDemo = DemoWindowsResult(
        attempted: environment.windowsTool.attempted,
        ok: environment.windowsTool.ok,
        toolName: environment.windowsTool.toolName,
        arguments: [:],
        result: environment.windowsTool.result,
        error: environment.windowsTool.error.map { DemoDemoStepError(step: $0.step, message: $0.message) }
    )
    let errors = environment.errors.map { DemoDemoStepError(step: $0.step, message: $0.message) }
    let success = environment.doctorReport.isSuccess && environment.tools != nil && windowsDemo.attempted && windowsDemo.ok

    let report = AgentDemoReport(
        success: success,
        doctor: environment.doctorReport.jsonReport,
        agentStatus: environment.postToolsStatus,
        toolCatalog: toolCatalog,
        windowsDemo: windowsDemo,
        nextCommands: agentDemoNextCommands(),
        errors: errors
    )

    if json {
        try writePrettyJSON(report)
    } else {
        print(formatAgentDemo(report))
    }

    if !success {
        throw ExitCode(1)
    }
}

// MARK: - Tool Catalog

private func buildDemoToolCatalog(_ tools: [JSONValue]) -> DemoToolCatalog {
    let catalog = buildToolCatalogData(tools, highlightToolNames: demoHighlightToolNames)
    return DemoToolCatalog(
        count: catalog.count,
        names: catalog.names,
        highlights: catalog.highlights.map {
            DemoToolHighlight(name: $0.name, description: $0.description, requiredArgs: $0.requiredArgs)
        }
    )
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
