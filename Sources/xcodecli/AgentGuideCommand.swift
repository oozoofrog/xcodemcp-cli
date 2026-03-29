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

private let guideIntentKeywordRules: [(workflowID: String, keywords: [String], value: Int)] = [
    ("diagnose", ["error", "warning", "fail", "failed", "issue", "issues", "log", "diagnostic", "diagnostics"], 5),
    ("build", ["build", "compile", "rebuild", "app", "project"], 3),
    ("test", ["test", "tests", "xctest", "ui test", "uitest"], 4),
    ("read", ["read", "open", "show", "view", "inspect file", "source"], 4),
    ("search", ["find", "search", "grep", "where", "list files"], 4),
    ("edit", ["edit", "change", "update", "replace", "write", "create", "modify"], 4),
]

private let guideIntentBoostRules: [(workflowID: String, keywords: [String], value: Int)] = [
    ("read", [".swift", ".plist", ".xcodeproj", ".xcworkspace"], 2),
    ("diagnose", ["build error", "build failure"], 3),
    ("test", ["run tests", "all tests"], 3),
]

private let guideSubjectPrefixes: [String: [String]] = [
    "build": ["build ", "compile ", "rebuild "],
    "test": ["run all tests for ", "run tests for ", "run test for ", "test ", "tests for ", "run all tests ", "run tests "],
    "read": ["inspect file ", "inspect ", "read ", "open ", "show ", "view ", "source "],
    "search": ["search for ", "search ", "find ", "grep ", "where is ", "where "],
    "edit": ["update ", "edit ", "change ", "replace ", "write ", "create ", "modify "],
    "diagnose": ["diagnose ", "fix ", "investigate ", "debug "],
]

// MARK: - Types

struct GuideIntentResult: Codable {
    let raw: String
    let workflowId: String
    let confidence: Double
    let alternatives: [String]
}

struct GuideWindowEntry: Codable {
    let tabIdentifier: String
    let workspacePath: String
}

struct GuideWindowsResult: Codable {
    var attempted: Bool = false
    var ok: Bool = false
    let toolName: String
    var entries: [GuideWindowEntry] = []
    var error: DemoStepError?
}

struct DemoStepError: Codable {
    let step: String
    let message: String
}

struct GuideEnvironment: Codable {
    let doctor: DoctorJSONReport
    var agentStatus: AgentStatus?
    var toolCatalog: GuideToolCatalog
    var windows: GuideWindowsResult
}

struct GuideToolHighlight: Codable {
    let name: String
    let description: String
    let requiredArgs: [String]
}

struct GuideToolCatalog: Codable {
    let count: Int
    let names: [String]
    let highlights: [GuideToolHighlight]
}

struct GuideWorkflowStep: Codable {
    let why: String
    let toolName: String
    let argumentsTemplate: [String: JSONValue]
    let whenToSkip: String
}

struct GuideWorkflowFallback: Codable {
    let title: String
    let description: String
    let commands: [String]
}

struct GuideWorkflowResult: Codable {
    let id: String
    let title: String
    let reason: String
    let steps: [GuideWorkflowStep]
    let fallbacks: [GuideWorkflowFallback]
}

struct AgentGuideReport: Codable {
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
    var scores = Dictionary(uniqueKeysWithValues: guideWorkflowOrder.map { ($0, 0) })
    applyGuideIntentRules(&scores, text: text, rules: guideIntentKeywordRules)
    applyGuideIntentRules(&scores, text: text, rules: guideIntentBoostRules)

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

func extractGuideSubject(_ raw: String, _ workflowID: String) -> String {
    let lower = raw.lowercased()
    for prefix in guideSubjectPrefixes[workflowID] ?? [] {
        if lower.hasPrefix(prefix) {
            return String(raw.dropFirst(prefix.count)).trimmingCharacters(in: .whitespaces)
        }
    }
    return raw
}

private func applyGuideIntentRules(
    _ scores: inout [String: Int],
    text: String,
    rules: [(workflowID: String, keywords: [String], value: Int)]
) {
    for rule in rules {
        for keyword in rule.keywords where text.contains(keyword) {
            scores[rule.workflowID, default: 0] += rule.value
        }
    }
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

private func guideToolStep(
    why: String,
    toolName: String,
    argumentsTemplate: [String: JSONValue],
    whenToSkip: String
) -> GuideWorkflowStep {
    GuideWorkflowStep(
        why: why,
        toolName: toolName,
        argumentsTemplate: argumentsTemplate,
        whenToSkip: whenToSkip
    )
}

private func guideSchemaFallback(tools: [String]) -> GuideWorkflowFallback {
    GuideWorkflowFallback(
        title: "If you want schema reassurance",
        description: "Inspect the tool schemas before executing the flow.",
        commands: tools.map { "xcodecli tool inspect \($0) --json" }
    )
}

private func makeGuideWorkflowResult(
    id: String,
    windowMatch: GuideWindowMatch,
    baseReason: String,
    steps: [GuideWorkflowStep],
    fallbacks: [GuideWorkflowFallback],
    nextCommands: [String]
) -> (GuideWorkflowResult, [String]) {
    (
        GuideWorkflowResult(
            id: id,
            title: guideWorkflowTitles[id] ?? id,
            reason: guideReasonForIntent(windowMatch, baseReason),
            steps: steps,
            fallbacks: fallbacks
        ),
        nextCommands
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

private func formatMaybeWindowsCommand(_ windowMatch: GuideWindowMatch) -> String {
    if let entry = windowMatch.matchedEntry {
        return "# already matched \(entry.tabIdentifier)"
    }
    return "xcodecli tool call XcodeListWindows --json '{}'"
}

// MARK: - Per-Workflow Command Builders

func guideCommandsPrefix(_ windowMatch: GuideWindowMatch) -> [String] {
    if windowMatch.matchedEntry == nil {
        return ["xcodecli tool call XcodeListWindows --json '{}'"]
    }
    return []
}

func buildGuideBuildCommands(_ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> [String] {
    buildGuideCommands(windowMatch, specs: [
        GuideCommandSpec(
            toolName: "BuildProject",
            timeout: "30m",
            arguments: [GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier))]
        ),
        GuideCommandSpec(
            toolName: "GetBuildLog",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "severity", value: .string("error")),
            ]
        ),
    ])
}

func buildGuideTestCommands(_ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> [String] {
    buildGuideCommands(windowMatch, specs: [
        GuideCommandSpec(
            toolName: "RunAllTests",
            timeout: "30m",
            arguments: [GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier))]
        ),
        GuideCommandSpec(
            toolName: "GetBuildLog",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "severity", value: .string("error")),
            ]
        ),
    ])
}

func buildGuideReadCommands(_ tabIdentifier: String, _ subject: String, _ windowMatch: GuideWindowMatch) -> [String] {
    var specs: [GuideCommandSpec] = []
    if looksLikeFileHint(subject) {
        specs.append(GuideCommandSpec(
            toolName: "XcodeGlob",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "pattern", value: .string(guideGlobPattern(subject))),
            ]
        ))
        specs.append(GuideCommandSpec(
            toolName: "XcodeRead",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "filePath", value: .string("<path from XcodeGlob>")),
            ]
        ))
    } else {
        specs.append(GuideCommandSpec(
            toolName: "XcodeLS",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "path", value: .string("")),
            ]
        ))
        specs.append(GuideCommandSpec(
            toolName: "XcodeRead",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "filePath", value: .string("<path from XcodeLS>")),
            ]
        ))
    }
    return buildGuideCommands(windowMatch, specs: specs)
}

func buildGuideSearchCommands(_ tabIdentifier: String, _ subject: String, _ windowMatch: GuideWindowMatch) -> [String] {
    var specs: [GuideCommandSpec] = []
    if looksLikeFileHint(subject) {
        specs.append(GuideCommandSpec(
            toolName: "XcodeGlob",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "pattern", value: .string(guideGlobPattern(subject))),
            ]
        ))
    } else {
        specs.append(GuideCommandSpec(
            toolName: "XcodeGrep",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "pattern", value: .string(guideSearchPattern(subject))),
                GuideCommandArgument(key: "outputMode", value: .string("content")),
                GuideCommandArgument(key: "showLineNumbers", value: .bool(true)),
            ]
        ))
    }
    return buildGuideCommands(windowMatch, specs: specs)
}

func buildGuideEditCommands(_ tabIdentifier: String, _ subject: String, _ windowMatch: GuideWindowMatch) -> [String] {
    var specs: [GuideCommandSpec] = []
    var pathPlaceholder = "<path from XcodeLS>"
    if looksLikeFileHint(subject) {
        specs.append(GuideCommandSpec(
            toolName: "XcodeGlob",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "pattern", value: .string(guideGlobPattern(subject))),
            ]
        ))
        pathPlaceholder = "<path from XcodeGlob>"
    } else {
        specs.append(GuideCommandSpec(
            toolName: "XcodeLS",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "path", value: .string("")),
            ]
        ))
    }
    specs.append(contentsOf: [
        GuideCommandSpec(
            toolName: "XcodeRead",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "filePath", value: .string(pathPlaceholder)),
            ]
        ),
        GuideCommandSpec(
            toolName: "XcodeUpdate",
            timeout: "120s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "filePath", value: .string(pathPlaceholder)),
                GuideCommandArgument(key: "oldString", value: .string("<exact text to replace>")),
                GuideCommandArgument(key: "newString", value: .string("<replacement text>")),
            ]
        ),
        GuideCommandSpec(
            toolName: "XcodeRefreshCodeIssuesInFile",
            timeout: "120s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "filePath", value: .string(pathPlaceholder)),
            ]
        ),
    ])
    return buildGuideCommands(windowMatch, specs: specs)
}

func buildGuideDiagnoseCommands(_ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> [String] {
    buildGuideCommands(windowMatch, specs: [
        GuideCommandSpec(
            toolName: "GetBuildLog",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "severity", value: .string("error")),
            ]
        ),
        GuideCommandSpec(
            toolName: "XcodeRead",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                GuideCommandArgument(key: "filePath", value: .string("<file path from the log or issue navigator>")),
            ]
        ),
    ])
}

// MARK: - Entry Point

func runAgentGuide(
    intent: String, json: Bool, timeout: Int,
    xcodePID: String?, sessionID: String?, debug: Bool
) async throws {
    let intentMatch = classifyGuideIntent(intent)

    let env = envDictionary()
    let (effective, resolved) = try resolveOptions(env: env, xcodePID: xcodePID, sessionID: sessionID)
    let collected = await collectReadOnlyEnvironment(
        env: env,
        effective: effective,
        resolved: resolved,
        timeout: timeout,
        debug: debug,
        highlightToolNames: guideHighlightToolNames,
        windowsStep: "windows"
    )
    let (report, windowMatch) = buildGuideReport(intentMatch: intentMatch, collected: collected)

    if json {
        try writePrettyJSON(report)
    } else {
        print(formatAgentGuide(report, windowMatch))
    }
}

private func buildGuideReport(
    intentMatch: IntentMatch,
    collected: ReadOnlyEnvironmentCollection
) -> (AgentGuideReport, GuideWindowMatch) {
    let toolCatalog = GuideToolCatalog(
        count: collected.toolCatalog.count,
        names: collected.toolCatalog.names,
        highlights: collected.toolCatalog.highlights.map {
            GuideToolHighlight(name: $0.name, description: $0.description, requiredArgs: $0.requiredArgs)
        }
    )
    let windows = GuideWindowsResult(
        attempted: collected.windowsTool.attempted,
        ok: collected.windowsTool.ok,
        toolName: collected.windowsTool.toolName,
        entries: collected.windowsTool.result.map(parseGuideWindowEntries) ?? [],
        error: collected.windowsTool.error.map { DemoStepError(step: $0.step, message: $0.message) }
    )
    let errors = collected.errors.map { DemoStepError(step: $0.step, message: $0.message) }
    let environment = GuideEnvironment(
        doctor: collected.doctorReport.jsonReport,
        agentStatus: collected.postToolsStatus,
        toolCatalog: toolCatalog,
        windows: windows
    )
    let windowMatch = resolveGuideWindowMatch(entries: windows.entries, subject: intentMatch.subject)
    let (workflow, nextCommands) = buildGuideWorkflow(intentMatch, environment, windowMatch)
    let report = AgentGuideReport(
        success: errors.isEmpty,
        intent: GuideIntentResult(
            raw: intentMatch.raw,
            workflowId: intentMatch.workflowID,
            confidence: intentMatch.confidence,
            alternatives: intentMatch.alternatives
        ),
        environment: environment,
        workflow: workflow,
        nextCommands: nextCommands,
        errors: errors
    )
    return (report, windowMatch)
}

// MARK: - Tool Catalog Building

let guideHighlightToolNames = [
    "XcodeListWindows", "BuildProject", "GetBuildLog", "RunAllTests", "GetTestList",
    "RunSomeTests", "XcodeLS", "XcodeRead", "XcodeGlob", "XcodeGrep",
    "XcodeUpdate", "XcodeWrite", "XcodeRefreshCodeIssuesInFile", "XcodeListNavigatorIssues",
]

private func buildGuideToolCatalog(_ tools: [JSONValue]) -> GuideToolCatalog {
    let catalog = buildToolCatalogData(tools, highlightToolNames: guideHighlightToolNames)
    return GuideToolCatalog(
        count: catalog.count,
        names: catalog.names,
        highlights: catalog.highlights.map {
            GuideToolHighlight(name: $0.name, description: $0.description, requiredArgs: $0.requiredArgs)
        }
    )
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
        guideToolStep(
            why: "BuildProject asks Xcode to build the active project or workspace shown in that tab.",
            toolName: "BuildProject",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier)],
            whenToSkip: "Skip only if you decided not to build after checking the window list."
        ),
        guideToolStep(
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
                formatToolCallCommand(GuideCommandSpec(
                    toolName: "BuildProject",
                    timeout: "30m",
                    arguments: [GuideCommandArgument(key: "tabIdentifier", value: .string("<tabIdentifier from above>"))]
                )),
            ]
        ),
        guideSchemaFallback(tools: ["BuildProject", "GetBuildLog"]),
    ]

    return makeGuideWorkflowResult(
        id: "build",
        windowMatch: windowMatch,
        baseReason: "The request is about building, so the shortest safe sequence is window resolution -> build -> build log on failure.",
        steps: steps,
        fallbacks: fallbacks,
        nextCommands: nextCommands
    )
}

private func buildGuideTestWorkflow(_ intent: IntentMatch, _ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> (GuideWorkflowResult, [String]) {
    let steps: [GuideWorkflowStep] = [
        guideListWindowsStep(windowMatch, why: "Use XcodeListWindows first so the test run targets the correct workspace tab."),
        guideToolStep(
            why: "RunAllTests is the fastest default when the request is to run the current scheme's full test plan.",
            toolName: "RunAllTests",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier)],
            whenToSkip: "Skip this step only if you already know you want a narrower subset of tests."
        ),
        guideToolStep(
            why: "GetTestList lets you narrow the run to specific tests before using RunSomeTests.",
            toolName: "GetTestList",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier)],
            whenToSkip: "Skip if running the full test plan is acceptable."
        ),
        guideToolStep(
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
                formatToolCallCommand(GuideCommandSpec(
                    toolName: "GetTestList",
                    timeout: "60s",
                    arguments: [GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier))]
                )),
                formatToolCallCommand(GuideCommandSpec(
                    toolName: "RunSomeTests",
                    timeout: "30m",
                    arguments: [
                        GuideCommandArgument(
                            key: "tabIdentifier",
                            value: .string(tabIdentifier)
                        ),
                        GuideCommandArgument(
                            key: "tests",
                            value: .array([
                                .object([
                                    "targetName": .string("<targetName>"),
                                    "testIdentifier": .string("<identifier>"),
                                ])
                            ])
                        ),
                    ]
                )),
            ]
        ),
        guideSchemaFallback(tools: ["GetTestList", "RunSomeTests"]),
    ]

    return makeGuideWorkflowResult(
        id: "test",
        windowMatch: windowMatch,
        baseReason: "The request is about tests, so the default path is window resolution -> full test run -> narrower test selection only if needed.",
        steps: steps,
        fallbacks: fallbacks,
        nextCommands: nextCommands
    )
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
        guideToolStep(
            why: lookupWhy,
            toolName: lookupTool,
            argumentsTemplate: lookupArgs,
            whenToSkip: "Skip if you already know the exact project-relative file path."
        ),
        guideToolStep(
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
                formatToolCallCommand(GuideCommandSpec(
                    toolName: "XcodeLS",
                    timeout: "60s",
                    arguments: [
                        GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                        GuideCommandArgument(key: "path", value: .string("")),
                    ]
                )),
            ]
        ),
        guideSchemaFallback(tools: [lookupTool, "XcodeRead"]),
    ]

    return makeGuideWorkflowResult(
        id: "read",
        windowMatch: windowMatch,
        baseReason: "The request is about reading source, so the efficient path is window resolution -> file lookup -> file read.",
        steps: steps,
        fallbacks: fallbacks,
        nextCommands: nextCommands
    )
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
        guideToolStep(
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

    return makeGuideWorkflowResult(
        id: "search",
        windowMatch: windowMatch,
        baseReason: "The request is about locating code or files, so the shortest safe path is window resolution -> targeted search.",
        steps: steps,
        fallbacks: fallbacks,
        nextCommands: nextCommands
    )
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
        guideToolStep(
            why: "Read the target file before changing it so you can compose the smallest safe edit payload.",
            toolName: lookupTool,
            argumentsTemplate: lookupArgs,
            whenToSkip: "Skip if you already know the exact project-relative path."
        ),
        guideToolStep(
            why: "Open the file contents before deciding between XcodeUpdate and XcodeWrite.",
            toolName: "XcodeRead",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "filePath": .string(pathPlaceholder)],
            whenToSkip: "Skip only if you already have the file contents in hand."
        ),
        guideToolStep(
            why: "XcodeUpdate is the safer first choice for targeted in-file replacements.",
            toolName: "XcodeUpdate",
            argumentsTemplate: [
                "tabIdentifier": .string(tabIdentifier), "filePath": .string(pathPlaceholder),
                "oldString": .string("<exact text to replace>"), "newString": .string("<replacement text>"),
            ],
            whenToSkip: "Skip this step if the change is a full-file rewrite, in which case XcodeWrite may be simpler."
        ),
        guideToolStep(
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
                formatToolCallCommand(GuideCommandSpec(
                    toolName: "XcodeWrite",
                    timeout: "120s",
                    arguments: [
                        GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                        GuideCommandArgument(key: "filePath", value: .string(pathPlaceholder)),
                        GuideCommandArgument(key: "content", value: .string("<full file contents>")),
                    ]
                )),
            ]
        ),
        guideSchemaFallback(tools: ["XcodeUpdate", "XcodeRefreshCodeIssuesInFile"]),
    ]

    return makeGuideWorkflowResult(
        id: "edit",
        windowMatch: windowMatch,
        baseReason: "The request is about changing code, so the safe path is window resolution -> locate/read the file -> small edit -> refresh diagnostics.",
        steps: steps,
        fallbacks: fallbacks,
        nextCommands: nextCommands
    )
}

private func buildGuideDiagnoseWorkflow(_ intent: IntentMatch, _ tabIdentifier: String, _ windowMatch: GuideWindowMatch) -> (GuideWorkflowResult, [String]) {
    let steps: [GuideWorkflowStep] = [
        guideListWindowsStep(windowMatch, why: "Use XcodeListWindows first so the diagnostics query targets the right workspace tab."),
        guideToolStep(
            why: "GetBuildLog is the fastest route to the failing compiler or build messages.",
            toolName: "GetBuildLog",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier), "severity": .string("error")],
            whenToSkip: "Skip only if you already know the exact failing file or line from somewhere else."
        ),
        guideToolStep(
            why: "XcodeListNavigatorIssues is a good secondary view when the problem is already visible in Xcode's issue navigator.",
            toolName: "XcodeListNavigatorIssues",
            argumentsTemplate: ["tabIdentifier": .string(tabIdentifier)],
            whenToSkip: "Skip unless you want the issue navigator perspective in addition to the build log."
        ),
        guideToolStep(
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
                formatToolCallCommand(GuideCommandSpec(
                    toolName: "XcodeRead",
                    timeout: "60s",
                    arguments: [
                        GuideCommandArgument(key: "tabIdentifier", value: .string(tabIdentifier)),
                        GuideCommandArgument(key: "filePath", value: .string("<file path from the log>")),
                    ]
                )),
            ]
        ),
    ]

    return makeGuideWorkflowResult(
        id: "diagnose",
        windowMatch: windowMatch,
        baseReason: "The request is about errors or failure analysis, so the efficient path is window resolution -> diagnostics -> open the failing file.",
        steps: steps,
        fallbacks: fallbacks,
        nextCommands: nextCommands
    )
}

func buildGuideCatalogWorkflow() -> (GuideWorkflowResult, [String]) {
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

func formatAgentGuide(_ report: AgentGuideReport, _ windowMatch: GuideWindowMatch) -> String {
    [
        "xcodecli agent guide",
        formatGuideIntentSection(report),
        formatGuideEnvironmentSection(report, windowMatch),
        formatGuideWorkflowSection(report),
        formatGuideNextCommandsSection(report),
        formatGuideFallbacksSection(report),
    ].joined(separator: "\n\n")
}

private func formatGuideIntentSection(_ report: AgentGuideReport) -> String {
    var lines = ["Intent", "------"]
    let rawIntent = report.intent.raw.isEmpty ? "(none)" : report.intent.raw
    lines.append("request: \(rawIntent)")
    lines.append("workflow: \(report.intent.workflowId) (confidence \(String(format: "%.2f", report.intent.confidence)))")
    if !report.intent.alternatives.isEmpty {
        lines.append("alternatives: \(report.intent.alternatives.joined(separator: ", "))")
    }
    return lines.joined(separator: "\n")
}

private func formatGuideEnvironmentSection(_ report: AgentGuideReport, _ windowMatch: GuideWindowMatch) -> String {
    var lines = ["Environment", "-----------"]
    let summary = report.environment.doctor.summary
    lines.append("doctor: \(report.environment.doctor.success) (\(summary.ok) ok, \(summary.warn) warn, \(summary.fail) fail, \(summary.info) info)")
    let notableChecks = report.environment.doctor.checks.filter { $0.status == .warn || $0.status == .fail }
    if !notableChecks.isEmpty {
        lines.append("notable checks:")
        lines.append(contentsOf: notableChecks.map { "- \($0.name) [\($0.status.rawValue)]: \($0.detail)" })
    }
    if !report.environment.doctor.recommendations.isEmpty {
        lines.append("recommendations:")
        for recommendation in report.environment.doctor.recommendations {
            lines.append("- \(recommendation.message)")
            lines.append(contentsOf: recommendation.commands.map { "  \($0)" })
        }
    }
    lines.append("tool catalog: \(report.environment.toolCatalog.count) tools")
    if let status = report.environment.agentStatus {
        lines.append("launchagent: running=\(status.running) socketReachable=\(status.socketReachable) backendSessions=\(status.backendSessions)")
    }
    if report.environment.windows.attempted {
        lines.append("windows: \(report.environment.windows.entries.count) discovered")
    } else {
        lines.append("windows: not collected")
    }
    if !windowMatch.note.isEmpty {
        lines.append("window match: \(windowMatch.note)")
    }
    if !report.errors.isEmpty {
        lines.append("notes:")
        lines.append(contentsOf: report.errors.map { "- \($0.step): \($0.message)" })
    }
    return lines.joined(separator: "\n")
}

private func formatGuideWorkflowSection(_ report: AgentGuideReport) -> String {
    var lines = ["Recommended Workflow", "--------------------", "\(report.workflow.title) — \(report.workflow.reason)"]
    if report.workflow.id == guideWorkflowCatalog {
        lines.append(contentsOf: report.workflow.steps.map { "- \($0.toolName): \($0.why)" })
    } else {
        for (index, step) in report.workflow.steps.enumerated() {
            lines.append("\(index + 1). \(step.toolName)")
            lines.append("   why: \(step.why)")
            lines.append("   args: \(formatArgumentsTemplate(step.argumentsTemplate))")
            lines.append("   skip: \(step.whenToSkip)")
        }
    }
    return lines.joined(separator: "\n")
}

private func formatGuideNextCommandsSection(_ report: AgentGuideReport) -> String {
    let lines = ["Exact Next Commands", "-------------------"] + report.nextCommands.map { "- \($0)" }
    return lines.joined(separator: "\n")
}

private func formatGuideFallbacksSection(_ report: AgentGuideReport) -> String {
    var lines = ["Fallbacks", "---------"]
    for fallback in report.workflow.fallbacks {
        lines.append("- \(fallback.title): \(fallback.description)")
        lines.append(contentsOf: fallback.commands.map { "  \($0)" })
    }
    return lines.joined(separator: "\n")
}
