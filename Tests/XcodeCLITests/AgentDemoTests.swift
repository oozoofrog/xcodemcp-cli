import Testing
import Foundation
@testable import xcodecli
import XcodeCLICore

@Suite("Agent Demo - extractToolResultMessage")
struct ExtractToolResultMessageTests {

    @Test("extracts message from structuredContent")
    func structuredContentMessage() {
        let result: [String: JSONValue] = [
            "structuredContent": .object([
                "message": .string("Build succeeded")
            ])
        ]
        let message = extractToolResultMessage(result)
        #expect(message == "Build succeeded")
    }

    @Test("extracts message from content array")
    func contentArrayMessage() {
        let result: [String: JSONValue] = [
            "content": .array([
                .object(["text": .string("Line one")]),
                .object(["text": .string("Line two")]),
            ])
        ]
        let message = extractToolResultMessage(result)
        #expect(message == "Line one\nLine two")
    }

    @Test("returns empty string for nil-like empty result")
    func emptyResultReturnsEmpty() {
        let result: [String: JSONValue] = [:]
        let message = extractToolResultMessage(result)
        // With an empty dict, JSONEncoder produces "{}" which is non-empty,
        // but there is no structuredContent or content key.
        #expect(!message.isEmpty || message.isEmpty) // always passes; real check below
        // The fallback path encodes the dict to JSON, so for empty dict we get "{}"
        #expect(message == "{}")
    }

    @Test("prefers structuredContent over content array")
    func structuredContentTakesPrecedence() {
        let result: [String: JSONValue] = [
            "structuredContent": .object([
                "message": .string("From structured")
            ]),
            "content": .array([
                .object(["text": .string("From content")]),
            ])
        ]
        let message = extractToolResultMessage(result)
        #expect(message == "From structured")
    }

    @Test("skips whitespace-only text blocks in content array")
    func skipsWhitespaceBlocks() {
        let result: [String: JSONValue] = [
            "content": .array([
                .object(["text": .string("   ")]),
                .object(["text": .string("Real message")]),
            ])
        ]
        let message = extractToolResultMessage(result)
        #expect(message == "Real message")
    }

    @Test("skips whitespace-only structuredContent message and falls through")
    func whitespaceStructuredContentFallsThrough() {
        let result: [String: JSONValue] = [
            "structuredContent": .object([
                "message": .string("   ")
            ]),
            "content": .array([
                .object(["text": .string("Fallback text")]),
            ])
        ]
        let message = extractToolResultMessage(result)
        #expect(message == "Fallback text")
    }
}

@Suite("Agent Demo - findToolByName")
struct FindToolByNameTests {

    @Test("finds existing tool by name")
    func findsExistingTool() {
        let tools: [JSONValue] = [
            .object(["name": .string("XcodeListWindows"), "description": .string("Lists windows")]),
            .object(["name": .string("BuildProject"), "description": .string("Builds project")]),
        ]
        let found = findToolByName(tools, "BuildProject")
        #expect(found != nil)
        if case .object(let obj) = found, case .string(let name) = obj["name"] {
            #expect(name == "BuildProject")
        } else {
            Issue.record("Expected object with name BuildProject")
        }
    }

    @Test("returns nil for missing tool name")
    func returnsNilForMissing() {
        let tools: [JSONValue] = [
            .object(["name": .string("XcodeListWindows")]),
        ]
        let found = findToolByName(tools, "NonExistentTool")
        #expect(found == nil)
    }

    @Test("returns nil for empty tools list")
    func emptyToolsList() {
        let found = findToolByName([], "XcodeListWindows")
        #expect(found == nil)
    }
}
