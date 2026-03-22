import Testing
import Foundation
@testable import xcodecli
import XcodeCLICore

@Suite("Agent Guide - Intent Classification")
struct AgentGuideIntentTests {

    // MARK: - classifyGuideIntent

    @Test("empty intent returns catalog workflow with full confidence")
    func emptyIntentReturnsCatalog() {
        let result = classifyGuideIntent("")
        #expect(result.workflowID == "catalog")
        #expect(result.confidence == 1.0)
        #expect(result.raw == "")
    }

    @Test("whitespace-only intent returns catalog workflow")
    func whitespaceIntentReturnsCatalog() {
        let result = classifyGuideIntent("   ")
        #expect(result.workflowID == "catalog")
        #expect(result.confidence == 1.0)
    }

    @Test("build intent is classified as build workflow")
    func buildIntent() {
        let result = classifyGuideIntent("build Unicody")
        #expect(result.workflowID == "build")
        #expect(result.confidence > 0.35)
        #expect(result.subject == "Unicody")
    }

    @Test("test intent is classified as test workflow")
    func testIntent() {
        let result = classifyGuideIntent("run tests for Unicody")
        #expect(result.workflowID == "test")
        #expect(result.confidence > 0.35)
    }

    @Test("read intent is classified as read workflow")
    func readIntent() {
        let result = classifyGuideIntent("read KeyboardState.swift")
        #expect(result.workflowID == "read")
        #expect(result.confidence > 0.35)
    }

    @Test("search intent is classified as search workflow")
    func searchIntent() {
        let result = classifyGuideIntent("search for AdManager")
        #expect(result.workflowID == "search")
        #expect(result.confidence > 0.35)
    }

    @Test("edit intent is classified as edit workflow")
    func editIntent() {
        let result = classifyGuideIntent("update KeyboardState.swift")
        #expect(result.workflowID == "edit")
        #expect(result.confidence > 0.35)
    }

    @Test("diagnose intent is classified as diagnose workflow")
    func diagnoseIntent() {
        let result = classifyGuideIntent("diagnose build errors")
        #expect(result.workflowID == "diagnose")
        #expect(result.confidence > 0.35)
    }

    @Test("confidence is clamped below 1.0")
    func confidenceRange() {
        // Even with many keywords the confidence should stay below 1.0
        let result = classifyGuideIntent("build compile rebuild project app")
        #expect(result.confidence >= 0.35)
        #expect(result.confidence <= 0.99)
    }

    @Test("unrecognized intent defaults to search workflow")
    func unrecognizedDefaultsToSearch() {
        let result = classifyGuideIntent("xyzzy something random")
        #expect(result.workflowID == "search")
        // Minimum confidence for a positive-scored match
        #expect(result.confidence >= 0.35)
    }

    @Test("alternatives contain at most two entries")
    func alternativesCount() {
        let result = classifyGuideIntent("build Unicody")
        #expect(result.alternatives.count <= 2)
    }
}

@Suite("Agent Guide - Window Matching")
struct AgentGuideWindowTests {

    @Test("empty entries returns note about no windows")
    func emptyEntries() {
        let match = resolveGuideWindowMatch(entries: [], subject: "Unicody")
        #expect(match.matchedEntry == nil)
        #expect(match.note.contains("No live Xcode windows"))
    }

    @Test("empty subject returns note about no hint")
    func emptySubject() {
        let entries = [
            GuideWindowEntry(tabIdentifier: "tab-1", workspacePath: "/Users/dev/Unicody/Unicody.xcodeproj"),
        ]
        let match = resolveGuideWindowMatch(entries: entries, subject: "")
        #expect(match.matchedEntry == nil)
        #expect(match.note.contains("No workspace or project hint"))
    }

    @Test("exact project name matches the correct window")
    func exactNameMatch() {
        let entries = [
            GuideWindowEntry(tabIdentifier: "tab-1", workspacePath: "/Users/dev/Unicody/Unicody.xcodeproj"),
            GuideWindowEntry(tabIdentifier: "tab-2", workspacePath: "/Users/dev/OtherApp/OtherApp.xcodeproj"),
        ]
        let match = resolveGuideWindowMatch(entries: entries, subject: "Unicody")
        #expect(match.matchedEntry?.tabIdentifier == "tab-1")
        #expect(match.ambiguous == false)
    }

    @Test("no matching entry returns placeholder note")
    func noMatch() {
        let entries = [
            GuideWindowEntry(tabIdentifier: "tab-1", workspacePath: "/Users/dev/Alpha/Alpha.xcodeproj"),
        ]
        let match = resolveGuideWindowMatch(entries: entries, subject: "ZetaProject")
        #expect(match.matchedEntry == nil)
        #expect(match.note.contains("No current Xcode window matched"))
    }

    @Test("guideWindowEntryScore returns 100 for exact stem match")
    func exactStemScore() {
        let entry = GuideWindowEntry(tabIdentifier: "tab-1", workspacePath: "/Users/dev/unicody/unicody.xcodeproj")
        let score = guideWindowEntryScore(entry, tokens: ["unicody"])
        #expect(score == 100)
    }

    @Test("guideWindowEntryScore returns 0 when no token matches")
    func noTokenMatch() {
        let entry = GuideWindowEntry(tabIdentifier: "tab-1", workspacePath: "/Users/dev/Alpha/Alpha.xcodeproj")
        let score = guideWindowEntryScore(entry, tokens: ["zetaproject"])
        #expect(score == 0)
    }
}

@Suite("Agent Guide - Workflow Structure")
struct AgentGuideWorkflowTests {

    @Test("guideWorkflowOrder contains exactly six entries")
    func workflowOrderCount() {
        #expect(guideWorkflowOrder.count == 6)
        #expect(guideWorkflowOrder == ["build", "test", "read", "search", "edit", "diagnose"])
    }

    @Test("guideWorkflowToolChain returns non-empty chains for all workflow IDs")
    func toolChainsNonEmpty() {
        for id in guideWorkflowOrder {
            let chain = guideWorkflowToolChain(id)
            #expect(!chain.isEmpty, "Tool chain for \(id) should not be empty")
        }
    }

    @Test("guideHighlightToolNames includes core tools")
    func highlightToolNamesContainCoreTools() {
        #expect(guideHighlightToolNames.contains("XcodeListWindows"))
        #expect(guideHighlightToolNames.contains("BuildProject"))
        #expect(guideHighlightToolNames.contains("XcodeRead"))
    }

    @Test("guideWindowMatchTokens filters stopwords and short tokens")
    func windowMatchTokensFiltering() {
        // "for" and "the" are stopwords; single-char tokens are dropped
        let tokens = guideWindowMatchTokens("build the Unicody project for release")
        #expect(!tokens.contains("for"))
        #expect(!tokens.contains("the"))
        #expect(!tokens.contains("build"))
        #expect(!tokens.contains("project"))
        #expect(tokens.contains("unicody"))
        #expect(tokens.contains("release"))
    }

    @Test("formatToolCallCommand renders ordered JSON arguments")
    func formatToolCallCommandOutput() {
        let command = formatToolCallCommand(GuideCommandSpec(
            toolName: "XcodeGrep",
            timeout: "60s",
            arguments: [
                GuideCommandArgument(key: "tabIdentifier", value: .string("tab-1")),
                GuideCommandArgument(key: "pattern", value: .string("AdManager")),
                GuideCommandArgument(key: "showLineNumbers", value: .bool(true)),
            ]
        ))

        #expect(command == #"xcodecli tool call XcodeGrep --timeout 60s --json '{"tabIdentifier":"tab-1","pattern":"AdManager","showLineNumbers":true}'"#)
    }

    @Test("buildGuideBuildCommands prefixes XcodeListWindows only when no window is matched")
    func buildCommandsPrefixBehavior() {
        let unmatched = buildGuideBuildCommands("tab-1", GuideWindowMatch())
        #expect(unmatched.first == "xcodecli tool call XcodeListWindows --json '{}'")
        #expect(unmatched.count == 3)

        let matched = buildGuideBuildCommands(
            "tab-1",
            GuideWindowMatch(matchedEntry: GuideWindowEntry(tabIdentifier: "tab-1", workspacePath: "/tmp/App.xcodeproj"))
        )
        #expect(matched.first != "xcodecli tool call XcodeListWindows --json '{}'")
        #expect(matched.count == 2)
    }

    @Test("buildGuideReadCommands switches between glob and ls based on file hint")
    func readCommandBranching() {
        let withFileHint = buildGuideReadCommands("tab-1", "KeyboardState.swift", GuideWindowMatch())
        #expect(withFileHint[1].contains("XcodeGlob"))
        #expect(withFileHint[2].contains(#""filePath":"<path from XcodeGlob>""#))

        let withoutFileHint = buildGuideReadCommands("tab-1", "keyboard state", GuideWindowMatch())
        #expect(withoutFileHint[1].contains("XcodeLS"))
        #expect(withoutFileHint[2].contains(#""filePath":"<path from XcodeLS>""#))
    }

    @Test("buildGuideEditCommands uses update and refresh commands with the selected placeholder path")
    func editCommandSequence() {
        let commands = buildGuideEditCommands("tab-1", "KeyboardState.swift", GuideWindowMatch())
        #expect(commands[1].contains("XcodeGlob"))
        #expect(commands[2].contains("XcodeRead"))
        #expect(commands[3].contains("XcodeUpdate"))
        #expect(commands[4].contains("XcodeRefreshCodeIssuesInFile"))
        #expect(commands[3].contains(#""filePath":"<path from XcodeGlob>""#))
    }
}
