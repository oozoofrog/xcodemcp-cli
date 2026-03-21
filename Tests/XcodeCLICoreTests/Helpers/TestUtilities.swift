import Foundation
@testable import XcodeCLICore

/// Write a JSON-RPC envelope to a pipe's writing end.
func writeLine(_ handle: FileHandle, _ json: String) {
    handle.write(Data((json + "\n").utf8))
}

/// Read a JSON-RPC response line from a pipe's reading end.
func readLine(from handle: FileHandle, timeout: TimeInterval = 2.0) -> String? {
    var data = Data()
    let deadline = Date().addingTimeInterval(timeout)
    while Date() < deadline {
        let chunk = handle.availableData
        if chunk.isEmpty {
            Thread.sleep(forTimeInterval: 0.01)
            continue
        }
        data.append(chunk)
        if let str = String(data: data, encoding: .utf8), str.contains("\n") {
            return str.trimmingCharacters(in: .whitespacesAndNewlines)
        }
    }
    return String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines)
}

/// Decode a JSON-RPC envelope from a string.
func decodeEnvelope(_ json: String) throws -> RPCEnvelope {
    try JSONLineCodec.decode(json)
}
