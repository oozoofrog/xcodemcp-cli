import Foundation
import XcodeCLICore

// MARK: - Constants

private let guideWorkflowCatalog = "catalog"
private let guideWorkflowOrder = ["build", "test", "read", "search", "edit", "diagnose"]

private let guideWorkflowTitles: [String: String] = [
    guideWorkflowCatalog: "Workflow catalog overview",
    "build": "Build a project",
    "test": "Run tests",
    "read": "Read a file",
    "search": "Search code or files",
    "edit": "Edit a file safely",
    "diagnose": "Diagnose build or code issues",
]

private let guideWorkflowExamples: [String: String] = [
    "build": "build Unicody",
    "test": "run tests for Unicody",
    "read": "read KeyboardState.swift",
    "search": "search for AdManager",
    "edit": "update KeyboardState.swift",
    "diagnose": "diagnose build errors",
]

private let guideRelatedWorkflows: [String: [String]] = [
    "build": ["diagnose", "test"],
    "test": ["build", "diagnose"],
    "read": ["search", "edit"],
    "search": ["read", "diagnose"],
    "edit": ["read", "diagnose"],
    "diagnose": ["build", "search"],
]

// MARK: - Types

private struct GuideIntentResult: Codable {
    let raw: String
    let workflowId: String
    let confidence: Double
    let alternatives: [String]
}

private struct GuideWindowEntry: Codable {
    let tabIdentifier: String
    let workspacePath: String
}

private struct GuideWindowsResult: Codable {
    var attempted: Bool = false
    var ok: Bool = false
    let toolName: String
    var entries: [GuideWindowEntry] = []
    var error: DemoStepError?
}

private struct DemoStepError: Codable {
    let step: String
    let message: String
}

private struct GuideEnvironment: Codable {
    let doctor: DoctorJSONReport
    var agentStatus: AgentStatus?
    var toolCatalog: GuideToolCatalog
    var windows: GuideWindowsResult
}

private struct GuideToolHighlight: Codable {
    let name: String
    let description: String
    let requiredArgs: [String]
}

private struct GuideToolCatalog: Codable {
    let count: Int
    let names: [String]
    let highlights: [GuideToolHighlight]
}

private struct GuideWorkflowStep: Codable {
    let why: String
    let toolName: String
    let argumentsTemplate: [String: JSONValue]
    let whenToSkip: String
}

private struct GuideWorkflowFallback: Codable {
    let title: String
    let description: String
    let commands: [String]
}

private struct GuideWorkflowResult: Codable {
    let id: String
    let title: String
    let reason: String
    let steps: [GuideWorkflowStep]
    let fallbacks: [GuideWorkflowFallback]
}

private struct AgentGuideReport: Codable {
    let success: Bool
    let intent: GuideIntentResult
    let environment: GuideEnvironment
    let workflow: GuideWorkflowResult
    let nextCommands: [String]
    let errors: [DemoStepError]
}

// MARK: - Intent Classification

private struct IntentMatch {
    let raw: String
    let workflowID: String
    let confidence: Double
    let alternatives: [String]
    let subject: String
}

private func classifyGuideIntent(_ raw: String) -> IntentMatch {
    let trimmed = raw.trimmingCharacters(in: .whitespaces)
    if trimmed.isEmpty {
        return IntentMatch(raw: "", workflowID: guideWorkflowCatalog, confidence: 1, alternatives: guideWorkflowOrder, subject: "")
    }

    let text = trimmed.lowercased()
    var scores: [String: Int] = [:]
    for id in guideWorkflowOrder { scores[id] = 0 }

    func addScore(_ wf: String, _ keywords: [String], _ value: Int) {
        for kw in keywords {
            if text.contains(kw) { scores[wf, default: 0] += value }
        }
    }

    addScore("diagnose", ["error", "warning", "fail", "failed", "issue", "issues", "log", "diagnostic", "diagnostics"], 5)
    addScore("build", ["build", "compile", "rebuild", "app", "project"], 3)
    addScore("test", ["test", "tests", "xctest", "ui test", "uitest"], 4)
    addScore("read", ["read", "open", "show", "view", "inspect file", "source"], 4)
    addScore("search", ["find", "search", "grep", "where", "list files"], 4)
    addScore("edit", ["edit", "change", "update", "replace", "write", "create", "modify"], 4)

    if text.contains(".swift") || text.contains(".plist") || text.contains(".xcodeproj") || text.contains(".xcworkspace") {
        scores["read", default: 0] += 2
    }
    if text.contains("build error") || text.contains("build failure") { scores["diagnose", default: 0] += 3 }
    if text.contains("run tests") || text.contains("all tests") { scores["test", default: 0] += 3 }

    var bestWorkflow = "search"
    var bestScore = 0
    for id in guideWorkflowOrder {
        if (scores[id] ?? 0) > bestScore {
            bestWorkflow = id
            bestScore = scores[id] ?? 0
        }
    }
    if bestScore <= 0 { bestWorkflow = "search"; bestScore = 1 }

    // Alternatives: top 2 non-best workflows with positive scores
    let candidates = scores
        .filter { $0.key != bestWorkflow && $0.value > 0 }
        .sorted { $0.value != $1.value ? $0.value > $1.value : $0.key < $1.key }
    var alternatives = candidates.prefix(2).map(\.key)
    if alternatives.count < 2 {
        for related in guideRelatedWorkflows[bestWorkflow] ?? [] {
            if related != bestWorkflow && !alternatives.contains(related) {
                alternatives.append(related)
                if alternatives.count >= 2 { break }
            }
        }
    }

    let confidence = min(0.35 + Double(bestScore) * 0.1, 0.99)

    return IntentMatch(
        raw: trimmed, workflowID: bestWorkflow, confidence: confidence,
        alternatives: alternatives, subject: extractGuideSubject(trimmed, bestWorkflow)
    )
}

private func extractGuideSubject(_ raw: String, _ workflowID: String) -> String {
    let lower = raw.lowercased()
    let prefixes: [String: [String]] = [
        "build": ["build ", "compile ", "rebuild "],
        "test": ["run all tests for ", "run tests for ", "run test for ", "test ", "tests for ", "run all tests ", "run tests "],
        "read": ["inspect file ", "inspect ", "read ", "open ", "show ", "view ", "source "],
        "search": ["search for ", "search ", "find ", "grep ", "where is ", "where "],
        "edit": ["update ", "edit ", "change ", "replace ", "write ", "create ", "modify "],
        "diagnose": ["diagnose ", "fix ", "investigate ", "debug "],
    ]
    for prefix in prefixes[workflowID] ?? [] {
        if lower.hasPrefix(prefix) {
            return String(raw.dropFirst(prefix.count)).trimmingCharacters(in: .whitespaces)
        }
    }
    return raw
}

// MARK: - Entry Point

func runAgentGuide(
    intent: String, json: Bool, timeout: Int,
    xcodePID: String?, sessionID: String?, debug: Bool
) async throws {
    let intentMatch = classifyGuideIntent(intent)

    // Collect environment
    let env = envDictionary()
    let (effective, resolved) = try resolveOptions(env: env, xcodePID: xcodePID, sessionID: sessionID)

    var errors: [DemoStepError] = []
    let agentStatus = try? await AgentClient.status()

    let doctorInspector = DoctorInspector(processRunner: SystemProcessRunner())
    let doctorReport = await doctorInspector.run(opts: DoctorOptions(
        baseEnv: env, xcodePID: effective.xcodePID, sessionID: effective.sessionID,
        sessionSource: resolved.sessionSource, sessionPath: resolved.sessionPath,
        agentStatus: agentStatus
    ))

    // Build agent request
    let bridgeEnv = EnvOptions.applyOverrides(baseEnv: env, opts: effective)
    let request = buildAgentRequest(env: bridgeEnv, effective: effective, timeout: TimeInterval(timeout), debug: debug)

    // List tools
    var toolCatalog = GuideToolCatalog(count: 0, names: [], highlights: [])
    var tools: [JSONValue]?
    do {
        let t = try await AgentClient.listTools(request: request)
        tools = t
        toolCatalog = buildGuideToolCatalog(t)
    } catch {
        errors.append(DemoStepError(step: "tools list", message: error.localizedDescription))
    }

    // Agent status post-tools
    let postStatus = try? await AgentClient.status()

    // Windows
    var windows = GuideWindowsResult(toolName: "XcodeListWindows")
    if let tools, findToolByName(tools, "XcodeListWindows") != nil {
        windows.attempted = true
        do {
            var windowReq = request
            windowReq.method = "tools/call"
            let result = try await AgentClient.callTool(request: request, name: "XcodeListWindows", arguments: [:])
            windows.ok = !result.isError
            windows.entries = parseGuideWindowEntries(result.result)
            if result.isError {
                let msg = extractDemoToolMessage(result.result)
                windows.error = DemoStepError(step: "windows", message: msg.isEmpty ? "tool returned isError=true" : msg)
                errors.append(windows.error!)
            }
        } catch {
            windows.error = DemoStepError(step: "windows", message: error.localizedDescription)
            errors.append(windows.error!)
        }
    } else if tools != nil {
        windows.error = DemoStepError(step: "windows", message: "tool not found: XcodeListWindows")
        errors.append(windows.error!)
    }

    let environment = GuideEnvironment(
        doctor: doctorReport.jsonReport,
        agentStatus: postStatus,
        toolCatalog: toolCatalog,
        windows: windows
    )

    let (workflow, nextCommands) = buildGuideWorkflow(intentMatch, environment, windows.entries)

    let report = AgentGuideReport(
        success: errors.isEmpty,
        intent: GuideIntentResult(
            raw: intentMatch.raw, workflowId: intentMatch.workflowID,
            confidence: intentMatch.confidence, alternatives: intentMatch.alternatives
        ),
        environment: environment,
        workflow: workflow,
        nextCommands: nextCommands,
        errors: errors
    )

    if json {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        let data = try encoder.encode(report)
        FileHandle.standardOutput.write(data)
        FileHandle.standardOutput.write(Data("\n".utf8))
    } else {
        print(formatAgentGuide(report))
    }
}

// MARK: - Tool Catalog Building

private let guideHighlightToolNames = [
    "XcodeListWindows", "BuildProject", "GetBuildLog", "RunAllTests", "GetTestList",
    "RunSomeTests", "XcodeLS", "XcodeRead", "XcodeGlob", "XcodeGrep",
    "XcodeUpdate", "XcodeWrite", "XcodeRefreshCodeIssuesInFile", "XcodeListNavigatorIssues",
]

private func buildGuideToolCatalog(_ tools: [JSONValue]) -> GuideToolCatalog {
    var names: [String] = []
    for tool in tools {
        if case .object(let obj) = tool, case .string(let name) = obj["name"] {
            names.append(name)
        }
    }

    var highlights: [GuideToolHighlight] = []
    for name in guideHighlightToolNames {
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
        highlights.append(GuideToolHighlight(name: name, description: desc, requiredArgs: requiredArgs))
    }

    return GuideToolCatalog(count: names.count, names: names, highlights: highlights)
}

private func findToolByName(_ tools: [JSONValue], _ name: String) -> JSONValue? {
    tools.first { tool in
        if case .object(let obj) = tool, case .string(let n) = obj["name"] { return n == name }
        return false
    }
}

// MARK: - Window Entry Parsing

private func parseGuideWindowEntries(_ result: [String: JSONValue]) -> [GuideWindowEntry] {
    let message = extractDemoToolMessage(result)
    guard !message.trimmingCharacters(in: .whitespaces).isEmpty else { return [] }

    return message.split(separator: "\n").compactMap { rawLine in
        var line = rawLine.trimmingCharacters(in: .whitespaces)
        if line.isEmpty { return nil }
        line = line.hasPrefix("* ") ? String(line.dropFirst(2)) : line
        let tabPrefix = "tabIdentifier: "
        let middle = ", workspacePath: "
        guard line.hasPrefix(tabPrefix) else { return nil }
        let rest = String(line.dropFirst(tabPrefix.count))
        guard let middleRange = rest.range(of: middle) else { return nil }
        let tabId = rest[rest.startIndex..<middleRange.lowerBound].trimmingCharacters(in: .whitespaces)
        let wsPath = rest[middleRange.upperBound...].trimmingCharacters(in: .whitespaces)
        guard !tabId.isEmpty, !wsPath.isEmpty else { return nil }
        return GuideWindowEntry(tabIdentifier: String(tabId), workspacePath: String(wsPath))
    }
}

private func extractDemoToolMessage(_ result: [String: JSONValue]) -> String {
    if case .object(let structured) = result["structuredContent"],
       case .string(let msg) = structured["message"], !msg.trimmingCharacters(in: .whitespaces).isEmpty {
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

// MARK: - Workflow Building

private func buildGuideWorkflow(_ intent: IntentMatch, _ environment: GuideEnvironment, _ entries: [GuideWindowEntry]) -> (GuideWorkflowResult, [String]) {
    if intent.workflowID == guideWorkflowCatalog {
        return buildGuideCatalogWorkflow()
    }

    let tabIdentifier = "<tabIdentifier from XcodeListWindows>"

    let steps: [GuideWorkflowStep]
    let title = guideWorkflowTitles[intent.workflowID] ?? intent.workflowID
    let reason: String

    switch intent.workflowID {
    case "build":
        steps = [
            GuideWorkflowStep(why: "Use XcodeListWindows to identify the correct tabIdentifier.", toolName: "XcodeListWindows", argumentsTemplate: [:], whenToSkip: "Skip if you already have the tabIdentifier."),
            GuideWorkflowStep(why: "BuildProject asks Xcode to build the active project.", toolName: "BuildProject", argumentsTemplate: ["tabIdentifier": .string(tabIdentifier)], whenToSkip: ""),
            GuideWorkflowStep(why: "GetBuildLog retrieves error details when the build fails.", toolName: "GetBuildLog", argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "severity": .string("error")], whenToSkip: "Skip unless the build reports errors."),
        ]
        reason = "The request is about building. Sequence: window resolution -> build -> build log on failure."
    case "test":
        steps = [
            GuideWorkflowStep(why: "Use XcodeListWindows first.", toolName: "XcodeListWindows", argumentsTemplate: [:], whenToSkip: ""),
            GuideWorkflowStep(why: "RunAllTests runs the current scheme's full test plan.", toolName: "RunAllTests", argumentsTemplate: ["tabIdentifier": .string(tabIdentifier)], whenToSkip: "Skip if you want a subset."),
            GuideWorkflowStep(why: "GetTestList lets you narrow with RunSomeTests.", toolName: "GetTestList", argumentsTemplate: ["tabIdentifier": .string(tabIdentifier)], whenToSkip: "Skip if running all tests is fine."),
        ]
        reason = "The request is about tests. Path: window resolution -> full test run -> narrow if needed."
    case "read":
        steps = [
            GuideWorkflowStep(why: "Use XcodeListWindows first.", toolName: "XcodeListWindows", argumentsTemplate: [:], whenToSkip: ""),
            GuideWorkflowStep(why: "XcodeLS browses the project tree.", toolName: "XcodeLS", argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "path": .string("")], whenToSkip: "Skip if you know the path."),
            GuideWorkflowStep(why: "XcodeRead opens the file.", toolName: "XcodeRead", argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "filePath": .string("<path from XcodeLS>")], whenToSkip: ""),
        ]
        reason = "The request is about reading. Path: window resolution -> file lookup -> file read."
    case "search":
        steps = [
            GuideWorkflowStep(why: "Use XcodeListWindows first.", toolName: "XcodeListWindows", argumentsTemplate: [:], whenToSkip: ""),
            GuideWorkflowStep(why: "XcodeGrep searches file contents.", toolName: "XcodeGrep", argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "pattern": .string(intent.subject), "outputMode": .string("content")], whenToSkip: ""),
        ]
        reason = "The request is about locating code. Path: window resolution -> search."
    case "edit":
        steps = [
            GuideWorkflowStep(why: "Use XcodeListWindows first.", toolName: "XcodeListWindows", argumentsTemplate: [:], whenToSkip: ""),
            GuideWorkflowStep(why: "Read the file before editing.", toolName: "XcodeRead", argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "filePath": .string("<path>")], whenToSkip: "Skip if you have the contents."),
            GuideWorkflowStep(why: "XcodeUpdate for targeted replacements.", toolName: "XcodeUpdate", argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "filePath": .string("<path>"), "oldString": .string("<exact text>"), "newString": .string("<replacement>")], whenToSkip: "Use XcodeWrite for full rewrites."),
            GuideWorkflowStep(why: "Refresh diagnostics after editing.", toolName: "XcodeRefreshCodeIssuesInFile", argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "filePath": .string("<path>")], whenToSkip: ""),
        ]
        reason = "The request is about editing. Path: window -> read -> edit -> refresh diagnostics."
    case "diagnose":
        steps = [
            GuideWorkflowStep(why: "Use XcodeListWindows first.", toolName: "XcodeListWindows", argumentsTemplate: [:], whenToSkip: ""),
            GuideWorkflowStep(why: "GetBuildLog retrieves failing messages.", toolName: "GetBuildLog", argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "severity": .string("error")], whenToSkip: ""),
            GuideWorkflowStep(why: "XcodeListNavigatorIssues provides a secondary view.", toolName: "XcodeListNavigatorIssues", argumentsTemplate: ["tabIdentifier": .string(tabIdentifier)], whenToSkip: "Optional secondary view."),
        ]
        reason = "The request is about diagnosing issues. Path: window -> build log -> navigator issues."
    default:
        return buildGuideCatalogWorkflow()
    }

    let nextCommands = steps.compactMap { step -> String? in
        guard !step.toolName.isEmpty else { return nil }
        if step.argumentsTemplate.isEmpty {
            return "xcodecli tool call \(step.toolName) --json '{}'"
        }
        if let data = try? JSONEncoder().encode(step.argumentsTemplate),
           let jsonStr = String(data: data, encoding: .utf8) {
            return "xcodecli tool call \(step.toolName) --json '\(jsonStr)'"
        }
        return nil
    }

    return (GuideWorkflowResult(
        id: intent.workflowID, title: title, reason: reason,
        steps: steps, fallbacks: []
    ), nextCommands)
}

private func buildGuideCatalogWorkflow() -> (GuideWorkflowResult, [String]) {
    let steps = guideWorkflowOrder.map { id in
        GuideWorkflowStep(
            why: "Representative request: \"\(guideWorkflowExamples[id] ?? id)\"",
            toolName: id,
            argumentsTemplate: [:],
            whenToSkip: "Skip this overview once you know which workflow matches your request."
        )
    }

    let nextCommands = guideWorkflowOrder.map { id in
        "xcodecli agent guide '\(guideWorkflowExamples[id] ?? id)'"
    }

    return (GuideWorkflowResult(
        id: guideWorkflowCatalog,
        title: guideWorkflowTitles[guideWorkflowCatalog] ?? "Catalog",
        reason: "No specific intent was provided, so this is a broad overview.",
        steps: steps,
        fallbacks: [
            GuideWorkflowFallback(
                title: "If you already know the request",
                description: "Re-run agent guide with the exact user intent.",
                commands: nextCommands
            ),
            GuideWorkflowFallback(
                title: "If you want safe live context first",
                description: "Use agent demo to see the live window list.",
                commands: ["xcodecli agent demo"]
            ),
        ]
    ), nextCommands)
}

// MARK: - Text Formatting

private func formatAgentGuide(_ report: AgentGuideReport) -> String {
    var b = ""
    b += "xcodecli agent guide\n\n"
    b += "Intent\n------\n"
    b += "raw: \(report.intent.raw.isEmpty ? "(empty)" : report.intent.raw)\n"
    b += "workflow: \(report.intent.workflowId)\n"
    b += "confidence: \(String(format: "%.2f", report.intent.confidence))\n"
    if !report.intent.alternatives.isEmpty {
        b += "alternatives: \(report.intent.alternatives.joined(separator: ", "))\n"
    }

    b += "\nWorkflow: \(report.workflow.title)\n"
    b += String(repeating: "-", count: report.workflow.title.count + 10) + "\n"
    b += "reason: \(report.workflow.reason)\n"
    if !report.workflow.steps.isEmpty {
        b += "steps:\n"
        for (i, step) in report.workflow.steps.enumerated() {
            b += "  \(i + 1). \(step.toolName): \(step.why)\n"
        }
    }

    if !report.nextCommands.isEmpty {
        b += "\nNext Commands\n-------------\n"
        for cmd in report.nextCommands {
            b += "- \(cmd)\n"
        }
    }

    if !report.errors.isEmpty {
        b += "\nErrors\n------\n"
        for err in report.errors {
            b += "- [\(err.step)] \(err.message)\n"
        }
    }

    return b
}
