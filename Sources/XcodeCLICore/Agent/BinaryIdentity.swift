import Foundation
import CryptoKit

private let binaryIdentityFileName = "binary-id"

/// Compute the path to the binary identity file.
public func binaryIdentityPath(_ paths: AgentPaths.Paths) -> String {
    guard !paths.supportDir.trimmingCharacters(in: .whitespaces).isEmpty else { return "" }
    return (paths.supportDir as NSString).appendingPathComponent(binaryIdentityFileName)
}

/// Compute the identity string for an executable.
/// Returns "sha256:<hex>" if the file exists, "path:<absolute_path>" otherwise.
public func binaryIdentityForExecutable(_ path: String) throws -> String {
    let cleaned = (path.trimmingCharacters(in: .whitespaces) as NSString).standardizingPath
    guard !cleaned.isEmpty else {
        throw XcodeCLIError.bridgeSpawnFailed(underlying: "missing executable path")
    }

    guard let data = FileManager.default.contents(atPath: cleaned) else {
        if !FileManager.default.fileExists(atPath: cleaned) {
            return "path:\(cleaned)"
        }
        throw XcodeCLIError.bridgeSpawnFailed(underlying: "cannot read executable \(cleaned)")
    }

    let hash = SHA256.hash(data: data)
    let hex = hash.map { String(format: "%02x", $0) }.joined()
    return "sha256:\(hex)"
}

/// Read the persisted binary identity.
public func readBinaryIdentity(_ path: String) throws -> String {
    try String(contentsOfFile: path, encoding: .utf8).trimmingCharacters(in: .whitespacesAndNewlines)
}

/// Write a binary identity string to file.
public func writeBinaryIdentity(_ path: String, identity: String) throws {
    guard !path.trimmingCharacters(in: .whitespaces).isEmpty else {
        throw XcodeCLIError.bridgeSpawnFailed(underlying: "missing binary identity path")
    }
    try (identity.trimmingCharacters(in: .whitespaces) + "\n")
        .write(toFile: path, atomically: true, encoding: .utf8)
}
