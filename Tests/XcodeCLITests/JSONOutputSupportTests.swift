import Testing
import Foundation
import ArgumentParser
@testable import xcodecli
import XcodeCLICore

@Suite("JSON Output Support")
struct JSONOutputSupportTests {
    @Test("prettyJSONData produces sorted keys with trailing newline")
    func prettyJSONDataFormat() throws {
        let input = ["zebra": "z", "alpha": "a"]
        let data = try prettyJSONData(input)
        let string = String(data: data, encoding: .utf8)!

        // Keys must be sorted
        let alphaIndex = string.range(of: "alpha")!.lowerBound
        let zebraIndex = string.range(of: "zebra")!.lowerBound
        #expect(alphaIndex < zebraIndex)

        // Must end with newline
        #expect(string.hasSuffix("\n"))

        // Must be pretty-printed (contain newlines inside)
        let lines = string.split(separator: "\n")
        #expect(lines.count > 1)
    }

    @Test("prettyJSONData round-trips through decoder")
    func prettyJSONDataRoundTrip() throws {
        let input = ["key": "value"]
        let data = try prettyJSONData(input)
        let decoded = try JSONDecoder().decode([String: String].self, from: data)
        #expect(decoded == input)
    }

    @Test("prettyJSONData works with nested Encodable types")
    func prettyJSONDataNested() throws {
        struct Nested: Encodable {
            let name: String
            let count: Int
        }
        let input = Nested(name: "test", count: 42)
        let data = try prettyJSONData(input)
        let string = String(data: data, encoding: .utf8)!
        #expect(string.contains("\"count\" : 42"))
        #expect(string.contains("\"name\" : \"test\""))
    }

    @Test("parseJSONArguments decodes a valid JSON object")
    func parseValidObject() throws {
        let result = try parseJSONArguments(#"{"key": "value", "count": 42}"#)
        #expect(result["key"] == .string("value"))
        #expect(result["count"] == .int(42))
    }

    @Test("parseJSONArguments throws ValidationError on non-object JSON")
    func parseNonObject() {
        #expect(throws: ValidationError.self) {
            _ = try parseJSONArguments("[1, 2, 3]")
        }
    }

    @Test("parseJSONArguments throws DecodingError on invalid JSON")
    func parseInvalidJSON() {
        #expect(throws: DecodingError.self) {
            _ = try parseJSONArguments("not json at all")
        }
    }

    @Test("parseJSONArguments handles empty object")
    func parseEmptyObject() throws {
        let result = try parseJSONArguments("{}")
        #expect(result.isEmpty)
    }
}
