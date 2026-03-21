import Testing
import Foundation
@testable import XcodeCLICore

@Suite("MCP Client")
struct MCPClientTests {

    @Test("MCPCallResult with isError true")
    func callResultIsError() {
        let result = MCPCallResult(result: ["content": .string("err")], isError: true)
        #expect(result.isError)
    }

    @Test("MCPCallResult with isError false")
    func callResultSuccess() {
        let result = MCPCallResult(result: ["content": .string("ok")], isError: false)
        #expect(!result.isError)
    }

    @Test("Config has correct defaults")
    func configDefaults() {
        let config = MCPClient.Config()
        #expect(config.command == "/usr/bin/xcrun")
        #expect(config.arguments == ["mcpbridge"])
        #expect(!config.debug)
    }

    @Test("Config accepts custom values")
    func configCustom() {
        let config = MCPClient.Config(
            command: "/usr/local/bin/test",
            arguments: ["--help"],
            environment: ["KEY": "VALUE"],
            debug: true
        )
        #expect(config.command == "/usr/local/bin/test")
        #expect(config.arguments == ["--help"])
        #expect(config.environment["KEY"] == "VALUE")
        #expect(config.debug)
    }

    @Test("MCPCallResult defaults isError to false")
    func mcpCallResultDefaultNotError() {
        let result = MCPCallResult(result: [:])
        #expect(!result.isError)
    }

    @Test("Pagination cursor can be parsed from JSONValue response")
    func paginationCursorParsing() throws {
        let response: JSONValue = .object([
            "tools": .array([.object(["name": .string("Tool1")])]),
            "nextCursor": .string("page2"),
        ])
        guard case .object(let obj) = response,
              case .string(let cursor) = obj["nextCursor"] else {
            Issue.record("Failed to extract nextCursor")
            return
        }
        #expect(cursor == "page2")
    }

    @Test("Empty cursor string signals end of pagination")
    func paginationEmptyCursorStops() throws {
        let response: JSONValue = .object([
            "tools": .array([.object(["name": .string("Tool1")])]),
            "nextCursor": .string(""),
        ])
        guard case .object(let obj) = response,
              case .string(let cursor) = obj["nextCursor"] else {
            Issue.record("Failed to extract nextCursor")
            return
        }
        #expect(cursor.isEmpty)
    }

    @Test("Missing nextCursor signals end of pagination")
    func paginationNoCursorStops() throws {
        let response: JSONValue = .object([
            "tools": .array([.object(["name": .string("Tool1")])]),
        ])
        guard case .object(let obj) = response else {
            Issue.record("Expected object response")
            return
        }
        #expect(obj["nextCursor"] == nil)
    }
}
