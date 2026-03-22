import Foundation

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
