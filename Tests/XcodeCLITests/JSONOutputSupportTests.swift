import Testing
import Foundation
@testable import xcodecli

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
}
