import Foundation
import XcodeCLICore

// MARK: - Constants

private let guideWorkflowCatalog = "catalog"
let guideWorkflowOrder = ["build", "test", "read", "search", "edit", "diagnose"]

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

struct GuideWindowEntry: Codable {
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

// MARK: - Window Match Type

struct GuideWindowMatch {
    var matchedEntry: GuideWindowEntry?
    var ambiguous: Bool = false
    var note: String = ""
}

// MARK: - Intent Classification

struct IntentMatch {
    let raw: String
    let workflowID: String
    let confidence: Double
    let alternatives: [String]
    let subject: String
}

func classifyGuideIntent(_ raw: String) -> IntentMatch {
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

// MARK: - Window Matching

func resolveGuideWindowMatch(entries: [GuideWindowEntry], subject: String) -> GuideWindowMatch {
    if entries.isEmpty {
        return GuideWindowMatch(note: "No live Xcode windows were discovered.")
    }
    let tokens = guideWindowMatchTokens(subject)
    if tokens.isEmpty {
        return GuideWindowMatch(note: "No workspace or project hint was detected in the request, so tabIdentifier is still a placeholder.")
    }

    var bestScore = 0
    var bestIndexes: [Int] = []
    for index in entries.indices {
        let score = guideWindowEntryScore(entries[index], tokens: tokens)
        if score > bestScore {
            bestScore = score
            bestIndexes = [index]
        } else if score == bestScore && score > 0 {
            bestIndexes.append(index)
        }
    }
    if bestScore == 0 {
        return GuideWindowMatch(note: "No current Xcode window matched the project hint, so tabIdentifier is still a placeholder.")
    }
    if bestIndexes.count > 1 {
        return GuideWindowMatch(ambiguous: true, note: "More than one Xcode window matched the project hint. Keep tabIdentifier as a placeholder until you choose one.")
    }
    let entry = entries[bestIndexes[0]]
    return GuideWindowMatch(
        matchedEntry: GuideWindowEntry(tabIdentifier: entry.tabIdentifier, workspacePath: entry.workspacePath),
        note: "Matched \(entry.tabIdentifier) to \(entry.workspacePath)."
    )
}

func guideWindowMatchTokens(_ subject: String) -> [String] {
    var normalized = subject.lowercased()
    let replacements: [(String, String)] = [
        (".xcodeproj", " "), (".xcworkspace", " "), (".swift", " "),
        (".m", " "), (".mm", " "), (".plist", " "), (".md", " "),
        ("/", " "), ("-", " "), ("_", " "), (".", " "), (",", " "), (":", " "),
    ]
    for (old, new) in replacements {
        normalized = normalized.replacingOccurrences(of: old, with: new)
    }
    let parts = normalized.split(whereSeparator: { $0.isWhitespace }).map(String.init)
    let stopwords: Set<String> = [
        "build", "compile", "rebuild", "test", "tests", "run", "read", "search", "find",
        "edit", "update", "replace", "write", "create", "modify", "fix", "error", "errors",
        "warning", "warnings", "issue", "issues", "project", "workspace", "file", "source",
        "for", "the", "a", "an", "in", "on", "of", "all", "diagnose", "debug",
    ]
    var seen = Set<String>()
    var tokens: [String] = []
    for part in parts {
        if part.count < 2 { continue }
        if stopwords.contains(part) { continue }
        if seen.contains(part) { continue }
        seen.insert(part)
        tokens.append(part)
    }
    return tokens
}

func guideWindowEntryScore(_ entry: GuideWindowEntry, tokens: [String]) -> Int {
    let pathLower = entry.workspacePath.lowercased()
    let baseLower = (entry.workspacePath as NSString).lastPathComponent.lowercased()
    let ext = (baseLower as NSString).pathExtension
    let stemLower = ext.isEmpty ? baseLower : String(baseLower.dropLast(ext.count + 1))
    let segments = guidePathSegments(entry.workspacePath)

    var best = 0
    for token in tokens {
        if stemLower == token {
            best = max(best, 100)
        } else if stemLower.contains(token) {
            best = max(best, 90)
        } else if baseLower.contains(token) {
            best = max(best, 80)
        } else if segments.contains(where: { $0 == token || $0.contains(token) }) {
            best = max(best, 70)
        } else if pathLower.contains(token) {
            best = max(best, 50)
        }
    }
    return best
}

private func guidePathSegments(_ path: String) -> [String] {
    let raw = path.lowercased().split { c in
        switch c {
        case "/", "-", "_", ".", " ": return true
        default: return false
        }
    }.map(String.init)
    var seen = Set<String>()
    return raw.filter { !$0.isEmpty && seen.insert($0).inserted }
}

private func looksLikeFileHint(_ subject: String) -> Bool {
    let trimmed = subject.trimmingCharacters(in: .whitespaces)
    if trimmed.isEmpty { return false }
    return trimmed.contains(".") || trimmed.contains("/")
}

private func guideGlobPattern(_ subject: String) -> String {
    let trimmed = subject.trimmingCharacters(in: .whitespaces)
    if trimmed.isEmpty { return "**/*" }
    if trimmed.contains("*") { return trimmed }
    if trimmed.hasPrefix("**/") { return trimmed }
    if trimmed.contains("/") { return trimmed }
    if trimmed.contains(".") { return "**/" + trimmed }
    return "**/*" + trimmed + "*"
}

private func guideSearchPattern(_ subject: String) -> String {
    let trimmed = subject.trimmingCharacters(in: .whitespaces)
    if trimmed.isEmpty { return "<search pattern>" }
    return trimmed
}

// MARK: - Shared Workflow Helpers

private func guideListWindowsStep(_ windowMatch: GuideWindowMatch, why: String) -> GuideWorkflowStep {
    GuideWorkflowStep(
        why: why,
        toolName: "XcodeListWindows", argumentsTemplate: [:],
        whenToSkip: guideWindowSkipReason(windowMatch)
    )
}

private func guideSchemaFallback(tools: [String]) -> GuideWorkflowFallback {
    GuideWorkflowFallback(
        title: "If you want schema reassurance",
        description: "Inspect the tool schemas before executing the flow.",
        commands: tools.map { "xcodecli tool inspect \($0) --json" }
    )
}

// MARK: - Workflow Reasoning Helpers

private func guideWindowSkipReason(_ windowMatch: GuideWindowMatch) -> String {
    if let entry = windowMatch.matchedEntry {
        return "Skip because agent guide already matched \(entry.tabIdentifier) from the live Xcode window list."
    }
    return "Skip only if you already know the exact tabIdentifier you want to use."
}

private func guideReasonForIntent(_ windowMatch: GuideWindowMatch, _ base: String) -> String {
    if let entry = windowMatch.matchedEntry {
        return "\(base) Live window matching already suggests \(entry.tabIdentifier)."
    }
    if windowMatch.ambiguous {
        return "\(base) The window match is ambiguous, so keep tabIdentifier as a placeholder until you pick one."
    }
    if !windowMatch.note.isEmpty {
        return "\(base) \(windowMatch.note)"
    }
    return base
}

func guideWorkflowToolChain(_ workflowID: String) -> [String] {
    switch workflowID {
    case "build":
        return ["XcodeListWindows", "BuildProject", "GetBuildLog"]
    case "test":
        return ["XcodeListWindows", "RunAllTests", "GetTestList/RunSomeTests", "GetBuildLog"]
    case "read":
        return ["XcodeListWindows", "XcodeGlob/XcodeLS", "XcodeRead"]
    case "search":
        return ["XcodeListWindows", "XcodeGrep/XcodeGlob"]
    case "edit":
        return ["XcodeListWindows", "XcodeGlob/XcodeLS", "XcodeRead", "XcodeUpdate/XcodeWrite", "XcodeRefreshCodeIssuesInFile"]
    case "diagnose":
        return ["XcodeListWindows", "GetBuildLog/XcodeListNavigatorIssues", "XcodeRead"]
    default:
        return []
    }
}

// MARK: - Format Helpers

private func jsonQuote(_ value: String) -> String {
    if let data = try? JSONEncoder().encode(value), let str = String(data: data, encoding: .utf8) {
        return str
    }
    return "\"\(value)\""
}

private func shellQuote(_ value: String) -> String {
    "'" + value.replacingOccurrences(of: "'", with: "'\\''" ) + "'"
}

private func formatBuildProjectCommand(_ tabIdentifier: String) -> String {
    "xcodecli tool call BuildProject --timeout 30m --json '{\"tabIdentifier\":\(jsonQuote(tabIdentifier))}'"
}

private func formatGetBuildLogCommand(_ tabIdentifier: String, _ severity: String) -> String {
    "xcodecli tool call GetBuildLog --timeout 60s --json '{\"tabIdentifier\":\(jsonQuote(tabIdentifier)),\"severity\":\(jsonQuote(severity))}'"
}

private func formatRunAllTestsCommand(_ tabIdentifier: String) -> String {
    "xcodecli tool call RunAllTests --timeout 30m --json '{\"tabIdentifier\":\(jsonQuote(tabIdentifier))}'"
}

private func formatGetTestListCommand(_ tabIdentifier: String) -> String {
    "xcodecli tool call GetTestList --timeout 60s --json '{\"tabIdentifier\":\(jsonQuote(tabIdentifier))}'"
}

private func formatRunSomeTestsTemplate(_ tabIdentifier: String) -> String {
    "xcodecli tool call RunSomeTests --timeout 30m --json '{\"tabIdentifier\":\(jsonQuote(tabIdentifier)),\"tests\":[{\"targetName\":\"<targetName>\",\"testIdentifier\":\"<identifier>\"}]}'"
}

private func formatXcodeLSCommand(_ tabIdentifier: String, _ path: String) -> String {
    "xcodecli tool call XcodeLS --timeout 60s --json '{\"tabIdentifier\":\(jsonQuote(tabIdentifier)),\"path\":\(jsonQuote(path))}'"
}

private func formatXcodeGlobCommand(_ tabIdentifier: String, _ pattern: String) -> String {
    "xcodecli tool call XcodeGlob --timeout 60s --json '{\"tabIdentifier\":\(jsonQuote(tabIdentifier)),\"pattern\":\(jsonQuote(pattern))}'"
}

private func formatXcodeReadCommand(_ tabIdentifier: String, _ filePath: String) -> String {
    "xcodecli tool call XcodeRead --timeout 60s --json '{\"tabIdentifier\":\(jsonQuote(tabIdentifier)),\"filePath\":\(jsonQuote(filePath))}'"
}

private func formatXcodeGrepCommand(_ tabIdentifier: String, _ pattern: String) -> String {
    "xcodecli tool call XcodeGrep --timeout 60s --json '{\"tabIdentifier\":\(jsonQuote(tabIdentifier)),\"pattern\":\(jsonQuote(pattern)),\"outputMode\":\"content\",\"showLineNumbers\":true}'"
}

private func formatXcodeUpdateTemplate(_ tabIdentifier: String, _ filePath: String) -> String {
    "xcodecli tool call XcodeUpdate --timeout 120s --json '{\"tabIdentifier\":\(jsonQuote(tabIdentifier)),\"filePath\":\(jsonQuote(filePath)),\"oldString\":\"<exact text to replace>\",\"newString\":\"<replacement text>\"}'"
}

private func formatRefreshIssuesCommand(_ tabIdentifier: String, _ filePath: String) -> String {
    "xcodecli tool call XcodeRefreshCodeIssuesInFile --timeout 120s --json '{\"tabIdentifier\":\(jsonQuote(tabIdentifier)),\"filePath\":\(jsonQuote(filePath))}'"
}

private func formatXcodeWriteTemplate(_ tabIdentifier: String, _ filePath: String) -> String {
    "xcodecli tool call XcodeWrite --timeout 120s --json '{\"tabIdentifier\":\(jsonQuote(tabIdentifier)),\"filePath\":\(jsonQuote(filePath)),\"content\":\"<full file contents>\"}'"
}

private func formatMaybeWindowsCommand(_ windowMatch: GuideWindowMatch) -> String {
    if let entry = windowMatch.matchedEntry {
        return "# already matched \(entry.tabIdentifier)"
    }
    return "xcodecli tool call XcodeListWindows --json '{}'"
}

// MARK: - Per-Workflow Command Builders

private func guideCommandsPrefix(_ windowMatch: GuideWindowMatch) -> [String] {
    if windowMatch.matchedEntry == nil {
        return ["xcodecli tool call XcodeListWindows --json '{}'"]
    }
    return []
}

private func buildGuideBuildCommands(_ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> [String] {
    guideCommandsPrefix(windowMatch) + [
        formatBuildProjectCommand(tabIdentifier),
        formatGetBuildLogCommand(tabIdentifier, "error"),
    ]
}

private func buildGuideTestCommands(_ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> [String] {
    guideCommandsPrefix(windowMatch) + [
        formatRunAllTestsCommand(tabIdentifier),
        formatGetBuildLogCommand(tabIdentifier, "error"),
    ]
}

private func buildGuideReadCommands(_ tabIdentifier: String, _ subject: String, _ windowMatch: GuideWindowMatch) -> [String] {
    var commands = guideCommandsPrefix(windowMatch)
    if looksLikeFileHint(subject) {
        commands.append(formatXcodeGlobCommand(tabIdentifier, guideGlobPattern(subject)))
        commands.append(formatXcodeReadCommand(tabIdentifier, "<path from XcodeGlob>"))
    } else {
        commands.append(formatXcodeLSCommand(tabIdentifier, ""))
        commands.append(formatXcodeReadCommand(tabIdentifier, "<path from XcodeLS>"))
    }
    return commands
}

private func buildGuideSearchCommands(_ tabIdentifier: String, _ subject: String, _ windowMatch: GuideWindowMatch) -> [String] {
    var commands = guideCommandsPrefix(windowMatch)
    if looksLikeFileHint(subject) {
        commands.append(formatXcodeGlobCommand(tabIdentifier, guideGlobPattern(subject)))
    } else {
        commands.append(formatXcodeGrepCommand(tabIdentifier, guideSearchPattern(subject)))
    }
    return commands
}

private func buildGuideEditCommands(_ tabIdentifier: String, _ subject: String, _ windowMatch: GuideWindowMatch) -> [String] {
    var commands = guideCommandsPrefix(windowMatch)
    var pathPlaceholder = "<path from XcodeLS>"
    if looksLikeFileHint(subject) {
        commands.append(formatXcodeGlobCommand(tabIdentifier, guideGlobPattern(subject)))
        pathPlaceholder = "<path from XcodeGlob>"
    } else {
        commands.append(formatXcodeLSCommand(tabIdentifier, ""))
    }
    commands.append(formatXcodeReadCommand(tabIdentifier, pathPlaceholder))
    commands.append(formatXcodeUpdateTemplate(tabIdentifier, pathPlaceholder))
    commands.append(formatRefreshIssuesCommand(tabIdentifier, pathPlaceholder))
    return commands
}

private func buildGuideDiagnoseCommands(_ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> [String] {
    guideCommandsPrefix(windowMatch) + [
        formatGetBuildLogCommand(tabIdentifier, "error"),
        formatXcodeReadCommand(tabIdentifier, "<file path from the log or issue navigator>"),
    ]
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
                let msg = extractToolResultMessage(result.result)
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

    let windowMatch = resolveGuideWindowMatch(entries: windows.entries, subject: intentMatch.subject)
    let (workflow, nextCommands) = buildGuideWorkflow(intentMatch, environment, windowMatch)

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
        print(formatAgentGuide(report, windowMatch))
    }
}

// MARK: - Tool Catalog Building

let guideHighlightToolNames = [
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

func findToolByName(_ tools: [JSONValue], _ name: String) -> JSONValue? {
    tools.first { tool in
        if case .object(let obj) = tool, case .string(let n) = obj["name"] { return n == name }
        return false
    }
}

// MARK: - Window Entry Parsing

private func parseGuideWindowEntries(_ result: [String: JSONValue]) -> [GuideWindowEntry] {
    let message = extractToolResultMessage(result)
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


// MARK: - Workflow Building

private func buildGuideWorkflow(_ intent: IntentMatch, _ environment: GuideEnvironment, _ windowMatch: GuideWindowMatch) -> (GuideWorkflowResult, [String]) {
    if intent.workflowID == guideWorkflowCatalog {
        return buildGuideCatalogWorkflow()
    }

    var tabIdentifier = "<tabIdentifier from XcodeListWindows>"
    if let entry = windowMatch.matchedEntry {
        tabIdentifier = entry.tabIdentifier
    }

    switch intent.workflowID {
    case "build":
        return buildGuideBuildWorkflow(intent, tabIdentifier, windowMatch)
    case "test":
        return buildGuideTestWorkflow(intent, tabIdentifier, windowMatch)
    case "read":
        return buildGuideReadWorkflow(intent, tabIdentifier, windowMatch)
    case "search":
        return buildGuideSearchWorkflow(intent, tabIdentifier, windowMatch)
    case "edit":
        return buildGuideEditWorkflow(intent, tabIdentifier, windowMatch)
    case "diagnose":
        return buildGuideDiagnoseWorkflow(intent, tabIdentifier, windowMatch)
    default:
        return buildGuideCatalogWorkflow()
    }
}

private func buildGuideBuildWorkflow(_ intent: IntentMatch, _ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> (GuideWorkflowResult, [String]) {
    let steps: [GuideWorkflowStep] = [
        guideListWindowsStep(windowMatch, why: "Use XcodeListWindows to identify the correct tabIdentifier for the project you want to build."),
        GuideWorkflowStep(
            why: "BuildProject asks Xcode to build the active project or workspace shown in that tab.",
            toolName: "BuildProject",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier)],
            whenToSkip: "Skip only if you decided not to build after checking the window list."
        ),
        GuideWorkflowStep(
            why: "GetBuildLog is the fastest follow-up when BuildProject fails and you need the actionable error summary.",
            toolName: "GetBuildLog",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "severity": .string("error")],
            whenToSkip: "Skip unless the build reports errors or you need error-only output."
        ),
    ]

    let nextCommands = buildGuideBuildCommands(tabIdentifier, windowMatch)
    let fallbacks: [GuideWorkflowFallback] = [
        GuideWorkflowFallback(
            title: "If the window match looks wrong",
            description: "Re-check the live Xcode windows and swap in the exact tabIdentifier yourself.",
            commands: [
                "xcodecli tool call XcodeListWindows --json '{}'",
                formatBuildProjectCommand("<tabIdentifier from above>"),
            ]
        ),
        guideSchemaFallback(tools: ["BuildProject", "GetBuildLog"]),
    ]

    return (GuideWorkflowResult(
        id: "build",
        title: guideWorkflowTitles["build"] ?? "Build a project",
        reason: guideReasonForIntent(windowMatch, "The request is about building, so the shortest safe sequence is window resolution -> build -> build log on failure."),
        steps: steps, fallbacks: fallbacks
    ), nextCommands)
}

private func buildGuideTestWorkflow(_ intent: IntentMatch, _ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> (GuideWorkflowResult, [String]) {
    let steps: [GuideWorkflowStep] = [
        guideListWindowsStep(windowMatch, why: "Use XcodeListWindows first so the test run targets the correct workspace tab."),
        GuideWorkflowStep(
            why: "RunAllTests is the fastest default when the request is to run the current scheme's full test plan.",
            toolName: "RunAllTests",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier)],
            whenToSkip: "Skip this step only if you already know you want a narrower subset of tests."
        ),
        GuideWorkflowStep(
            why: "GetTestList lets you narrow the run to specific tests before using RunSomeTests.",
            toolName: "GetTestList",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier)],
            whenToSkip: "Skip if running the full test plan is acceptable."
        ),
        GuideWorkflowStep(
            why: "GetBuildLog surfaces the underlying failure output if the test run fails early or produces build errors.",
            toolName: "GetBuildLog",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "severity": .string("error")],
            whenToSkip: "Skip unless the test run fails or emits build errors."
        ),
    ]

    let nextCommands = buildGuideTestCommands(tabIdentifier, windowMatch)
    let fallbacks: [GuideWorkflowFallback] = [
        GuideWorkflowFallback(
            title: "If you need to run only some tests",
            description: "Enumerate the available tests first, then switch to RunSomeTests with targetName and testIdentifier values from the list.",
            commands: [
                formatGetTestListCommand(tabIdentifier),
                formatRunSomeTestsTemplate(tabIdentifier),
            ]
        ),
        guideSchemaFallback(tools: ["GetTestList", "RunSomeTests"]),
    ]

    return (GuideWorkflowResult(
        id: "test",
        title: guideWorkflowTitles["test"] ?? "Run tests",
        reason: guideReasonForIntent(windowMatch, "The request is about tests, so the default path is window resolution -> full test run -> narrower test selection only if needed."),
        steps: steps, fallbacks: fallbacks
    ), nextCommands)
}

private func buildGuideReadWorkflow(_ intent: IntentMatch, _ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> (GuideWorkflowResult, [String]) {
    let subject = intent.subject.trimmingCharacters(in: .whitespaces)

    var lookupTool = "XcodeLS"
    var lookupArgs: [String: JSONValue] = ["tabIdentifier": .string(tabIdentifier), "path": .string("")]
    var lookupWhy = "XcodeLS is the simplest starting point when you only need to browse the project tree before opening a file."
    var readPathPlaceholder = "<path from XcodeLS>"
    if looksLikeFileHint(subject) {
        lookupTool = "XcodeGlob"
        lookupArgs = ["tabIdentifier": .string(tabIdentifier), "pattern": .string(guideGlobPattern(subject))]
        lookupWhy = "XcodeGlob is faster when the request already hints at a filename or file extension."
        readPathPlaceholder = "<path from XcodeGlob>"
    }

    let steps: [GuideWorkflowStep] = [
        guideListWindowsStep(windowMatch, why: "Use XcodeListWindows first so the subsequent file operations point at the right workspace tab."),
        GuideWorkflowStep(
            why: lookupWhy,
            toolName: lookupTool,
            argumentsTemplate: lookupArgs,
            whenToSkip: "Skip if you already know the exact project-relative file path."
        ),
        GuideWorkflowStep(
            why: "XcodeRead opens the target source file once you have its project-relative path.",
            toolName: "XcodeRead",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "filePath": .string(readPathPlaceholder)],
            whenToSkip: "Skip only if the earlier lookup already answered the question without opening the file."
        ),
    ]

    let nextCommands = buildGuideReadCommands(tabIdentifier, subject, windowMatch)
    let fallbacks: [GuideWorkflowFallback] = [
        GuideWorkflowFallback(
            title: "If the file path is still unclear",
            description: "Browse the project tree manually before opening the file.",
            commands: [
                formatMaybeWindowsCommand(windowMatch),
                formatXcodeLSCommand(tabIdentifier, ""),
            ]
        ),
        guideSchemaFallback(tools: [lookupTool, "XcodeRead"]),
    ]

    return (GuideWorkflowResult(
        id: "read",
        title: guideWorkflowTitles["read"] ?? "Read a file",
        reason: guideReasonForIntent(windowMatch, "The request is about reading source, so the efficient path is window resolution -> file lookup -> file read."),
        steps: steps, fallbacks: fallbacks
    ), nextCommands)
}

private func buildGuideSearchWorkflow(_ intent: IntentMatch, _ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> (GuideWorkflowResult, [String]) {
    let subject = intent.subject.trimmingCharacters(in: .whitespaces)

    var searchTool = "XcodeGrep"
    var searchArgs: [String: JSONValue] = [
        "tabIdentifier": .string(tabIdentifier),
        "pattern": .string(guideSearchPattern(subject)),
        "outputMode": .string("content"),
        "showLineNumbers": .bool(true),
    ]
    var searchWhy = "XcodeGrep is the best default when the request is to find a symbol or text inside files."
    if looksLikeFileHint(subject) {
        searchTool = "XcodeGlob"
        searchArgs = ["tabIdentifier": .string(tabIdentifier), "pattern": .string(guideGlobPattern(subject))]
        searchWhy = "XcodeGlob is better when the request looks like a filename search instead of a content search."
    }

    let steps: [GuideWorkflowStep] = [
        guideListWindowsStep(windowMatch, why: "Use XcodeListWindows first so the search runs against the right project tab."),
        GuideWorkflowStep(
            why: searchWhy,
            toolName: searchTool,
            argumentsTemplate: searchArgs,
            whenToSkip: "Skip only if you already have the exact file path or symbol location."
        ),
    ]

    let nextCommands = buildGuideSearchCommands(tabIdentifier, subject, windowMatch)
    let fallbacks: [GuideWorkflowFallback] = [
        GuideWorkflowFallback(
            title: "If the first search is too broad",
            description: "Refine the glob, grep pattern, or output mode after you see the initial results.",
            commands: [
                "xcodecli tool inspect XcodeGrep --json",
                "xcodecli tool inspect XcodeGlob --json",
            ]
        ),
    ]

    return (GuideWorkflowResult(
        id: "search",
        title: guideWorkflowTitles["search"] ?? "Search code or files",
        reason: guideReasonForIntent(windowMatch, "The request is about locating code or files, so the shortest safe path is window resolution -> targeted search."),
        steps: steps, fallbacks: fallbacks
    ), nextCommands)
}

private func buildGuideEditWorkflow(_ intent: IntentMatch, _ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> (GuideWorkflowResult, [String]) {
    let subject = intent.subject.trimmingCharacters(in: .whitespaces)

    var lookupTool = "XcodeLS"
    var lookupArgs: [String: JSONValue] = ["tabIdentifier": .string(tabIdentifier), "path": .string("")]
    var pathPlaceholder = "<path from XcodeLS>"
    if looksLikeFileHint(subject) {
        lookupTool = "XcodeGlob"
        lookupArgs = ["tabIdentifier": .string(tabIdentifier), "pattern": .string(guideGlobPattern(subject))]
        pathPlaceholder = "<path from XcodeGlob>"
    }

    let steps: [GuideWorkflowStep] = [
        guideListWindowsStep(windowMatch, why: "Use XcodeListWindows first so the edit applies to the right workspace tab."),
        GuideWorkflowStep(
            why: "Read the target file before changing it so you can compose the smallest safe edit payload.",
            toolName: lookupTool,
            argumentsTemplate: lookupArgs,
            whenToSkip: "Skip if you already know the exact project-relative path."
        ),
        GuideWorkflowStep(
            why: "Open the file contents before deciding between XcodeUpdate and XcodeWrite.",
            toolName: "XcodeRead",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "filePath": .string(pathPlaceholder)],
            whenToSkip: "Skip only if you already have the file contents in hand."
        ),
        GuideWorkflowStep(
            why: "XcodeUpdate is the safer first choice for targeted in-file replacements.",
            toolName: "XcodeUpdate",
            argumentsTemplate: [
                "tabIdentifier": .string(tabIdentifier), "filePath": .string(pathPlaceholder),
                "oldString": .string("<exact text to replace>"), "newString": .string("<replacement text>"),
            ],
            whenToSkip: "Skip this step if the change is a full-file rewrite, in which case XcodeWrite may be simpler."
        ),
        GuideWorkflowStep(
            why: "Refresh diagnostics immediately after the edit so you can verify that the file still parses or compiles cleanly.",
            toolName: "XcodeRefreshCodeIssuesInFile",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "filePath": .string(pathPlaceholder)],
            whenToSkip: "Skip only if you intentionally want to postpone validation."
        ),
    ]

    let nextCommands = buildGuideEditCommands(tabIdentifier, subject, windowMatch)
    let fallbacks: [GuideWorkflowFallback] = [
        GuideWorkflowFallback(
            title: "If the change is a full rewrite",
            description: "Switch from XcodeUpdate to XcodeWrite once you know the entire target file contents.",
            commands: [
                "xcodecli tool inspect XcodeWrite --json",
                formatXcodeWriteTemplate(tabIdentifier, pathPlaceholder),
            ]
        ),
        guideSchemaFallback(tools: ["XcodeUpdate", "XcodeRefreshCodeIssuesInFile"]),
    ]

    return (GuideWorkflowResult(
        id: "edit",
        title: guideWorkflowTitles["edit"] ?? "Edit a file safely",
        reason: guideReasonForIntent(windowMatch, "The request is about changing code, so the safe path is window resolution -> locate/read the file -> small edit -> refresh diagnostics."),
        steps: steps, fallbacks: fallbacks
    ), nextCommands)
}

private func buildGuideDiagnoseWorkflow(_ intent: IntentMatch, _ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> (GuideWorkflowResult, [String]) {
    let steps: [GuideWorkflowStep] = [
        guideListWindowsStep(windowMatch, why: "Use XcodeListWindows first so the diagnostics query targets the right workspace tab."),
        GuideWorkflowStep(
            why: "GetBuildLog is the fastest route to the failing compiler or build messages.",
            toolName: "GetBuildLog",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "severity": .string("error")],
            whenToSkip: "Skip only if you already know the exact failing file or line from somewhere else."
        ),
        GuideWorkflowStep(
            why: "XcodeListNavigatorIssues is a good secondary view when the problem is already visible in Xcode's issue navigator.",
            toolName: "XcodeListNavigatorIssues",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier)],
            whenToSkip: "Skip unless you want the issue navigator perspective in addition to the build log."
        ),
        GuideWorkflowStep(
            why: "Open the failing file after the log points you at a concrete path.",
            toolName: "XcodeRead",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "filePath": .string("<file path from the log or issue navigator>")],
            whenToSkip: "Skip only if the log already tells you everything you need."
        ),
    ]

    let nextCommands = buildGuideDiagnoseCommands(tabIdentifier, windowMatch)
    let fallbacks: [GuideWorkflowFallback] = [
        GuideWorkflowFallback(
            title: "If you need issue navigator context",
            description: "Inspect the issue navigator tool schema before composing a filtered request.",
            commands: [
                "xcodecli tool inspect XcodeListNavigatorIssues --json",
            ]
        ),
        GuideWorkflowFallback(
            title: "If the problem is obviously file-specific",
            description: "Jump straight from the build log to XcodeRead for the failing file.",
            commands: [
                formatXcodeReadCommand(tabIdentifier, "<file path from the log>"),
            ]
        ),
    ]

    return (GuideWorkflowResult(
        id: "diagnose",
        title: guideWorkflowTitles["diagnose"] ?? "Diagnose build or code issues",
        reason: guideReasonForIntent(windowMatch, "The request is about errors or failure analysis, so the efficient path is window resolution -> diagnostics -> open the failing file."),
        steps: steps, fallbacks: fallbacks
    ), nextCommands)
}

private func buildGuideCatalogWorkflow() -> (GuideWorkflowResult, [String]) {
    let steps = guideWorkflowOrder.map { id in
        GuideWorkflowStep(
            why: "Representative request: \"\(guideWorkflowExamples[id] ?? id)\"",
            toolName: guideWorkflowToolChain(id).joined(separator: " -> "),
            argumentsTemplate: [:],
            whenToSkip: "Skip this overview once you know which workflow family matches your request."
        )
    }

    let nextCommands = guideWorkflowOrder.map { id in
        "xcodecli agent guide \(shellQuote(guideWorkflowExamples[id] ?? id))"
    }

    return (GuideWorkflowResult(
        id: guideWorkflowCatalog,
        title: guideWorkflowTitles[guideWorkflowCatalog] ?? "Catalog",
        reason: "No specific intent was provided, so this is a broad overview of the most common workflow families.",
        steps: steps,
        fallbacks: [
            GuideWorkflowFallback(
                title: "If you already know the request",
                description: "Re-run agent guide with the exact user intent to get concrete next commands and, when possible, a real tabIdentifier.",
                commands: nextCommands
            ),
            GuideWorkflowFallback(
                title: "If you want safe live context first",
                description: "Use agent demo to see the live window list and current tool catalog before picking a workflow.",
                commands: ["xcodecli agent demo"]
            ),
        ]
    ), nextCommands)
}

// MARK: - Text Formatting

private func formatArgumentsTemplate(_ arguments: [String: JSONValue]) -> String {
    if arguments.isEmpty { return "{}" }
    if let data = try? JSONEncoder().encode(arguments), let str = String(data: data, encoding: .utf8) {
        return str
    }
    return "{}"
}

private func formatAgentGuide(_ report: AgentGuideReport, _ windowMatch: GuideWindowMatch) -> String {
    var b = ""
    b += "xcodecli agent guide\n\n"

    b += "Intent\n"
    b += "------\n"
    let rawIntent = report.intent.raw.isEmpty ? "(none)" : report.intent.raw
    b += "request: \(rawIntent)\n"
    b += "workflow: \(report.intent.workflowId) (confidence \(String(format: "%.2f", report.intent.confidence)))\n"
    if !report.intent.alternatives.isEmpty {
        b += "alternatives: \(report.intent.alternatives.joined(separator: ", "))\n"
    }

    b += "\nEnvironment\n"
    b += "-----------\n"
    let summary = report.environment.doctor.summary
    b += "doctor: \(report.environment.doctor.success) (\(summary.ok) ok, \(summary.warn) warn, \(summary.fail) fail, \(summary.info) info)\n"
    b += "tool catalog: \(report.environment.toolCatalog.count) tools\n"
    if let status = report.environment.agentStatus {
        b += "launchagent: running=\(status.running) socketReachable=\(status.socketReachable) backendSessions=\(status.backendSessions)\n"
    }
    if report.environment.windows.attempted {
        b += "windows: \(report.environment.windows.entries.count) discovered\n"
    } else {
        b += "windows: not collected\n"
    }
    if !windowMatch.note.isEmpty {
        b += "window match: \(windowMatch.note)\n"
    }
    if !report.errors.isEmpty {
        b += "notes:\n"
        for err in report.errors {
            b += "- \(err.step): \(err.message)\n"
        }
    }

    b += "\nRecommended Workflow\n"
    b += "--------------------\n"
    b += "\(report.workflow.title) — \(report.workflow.reason)\n"
    if report.workflow.id == guideWorkflowCatalog {
        for step in report.workflow.steps {
            b += "- \(step.toolName): \(step.why)\n"
        }
    } else {
        for (index, step) in report.workflow.steps.enumerated() {
            b += "\(index + 1). \(step.toolName)\n"
            b += "   why: \(step.why)\n"
            b += "   args: \(formatArgumentsTemplate(step.argumentsTemplate))\n"
            b += "   skip: \(step.whenToSkip)\n"
        }
    }

    b += "\nExact Next Commands\n"
    b += "-------------------\n"
    for cmd in report.nextCommands {
        b += "- \(cmd)\n"
    }

    b += "\nFallbacks\n"
    b += "---------\n"
    for fallback in report.workflow.fallbacks {
        b += "- \(fallback.title): \(fallback.description)\n"
        for cmd in fallback.commands {
            b += "  \(cmd)\n"
        }
    }

    return b
}
