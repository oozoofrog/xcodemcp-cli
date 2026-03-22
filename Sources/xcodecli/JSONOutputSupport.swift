import ArgumentParser
import Foundation
import XcodeCLICore

/// Encode a value as pretty-printed JSON with sorted keys, appending a trailing newline.
func prettyJSONData<T: Encodable>(_ value: T) throws -> Data {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
    var data = try encoder.encode(value)
    data.append(contentsOf: [0x0A]) // newline
    return data
}

/// Write a value as pretty-printed JSON to stdout.
func writePrettyJSON<T: Encodable>(_ value: T) throws {
    let data = try prettyJSONData(value)
    FileHandle.standardOutput.write(data)
}

/// Parse a raw JSON string into a dictionary, requiring the top-level value to be an object.
/// Throws `ValidationError` if the decoded value is not a JSON object.
func parseJSONArguments(_ raw: String) throws -> [String: JSONValue] {
    let data = Data(raw.utf8)
    let value = try JSONDecoder().decode(JSONValue.self, from: data)
    guard case .object(let obj) = value else {
        throw ValidationError("JSON payload must decode to a JSON object")
    }
    return obj
}
