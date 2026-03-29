import Foundation

public struct UpdateResult: Sendable {
    public let mode: String          // "homebrew" or "direct"
    public let currentVersion: String
    public let targetVersion: String
    public let alreadyUpToDate: Bool

    public init(mode: String, currentVersion: String, targetVersion: String, alreadyUpToDate: Bool) {
        self.mode = mode
        self.currentVersion = currentVersion
        self.targetVersion = targetVersion
        self.alreadyUpToDate = alreadyUpToDate
    }
}

public enum Updater {
    private static let homebrewFormula = "oozoofrog/tap/xcodecli"

    public static func run(
        currentVersion: String,
        executablePath: String? = nil,
        processRunner: some ProcessRunning
    ) async throws -> UpdateResult {
        let version = currentVersion.trimmingCharacters(in: .whitespaces)
        guard !version.isEmpty else {
            throw XcodeCLIError.mcpInitializationFailed(reason: "current version must not be empty")
        }

        if let executablePath, !executablePath.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            try validateStableUpdateExecutablePath(executablePath)
        }

        // Try Homebrew first
        if let prefix = try? await homebrewPrefix(processRunner: processRunner) {
            return try await runHomebrewUpdate(
                currentVersion: version, prefix: prefix, processRunner: processRunner
            )
        }

        // Fall back to direct update
        return try await runDirectUpdate(currentVersion: version, processRunner: processRunner)
    }

    private static func homebrewPrefix(processRunner: some ProcessRunning) async throws -> String {
        let result = try await processRunner.run("/usr/local/bin/brew", arguments: ["--prefix", homebrewFormula])
        guard result.exitCode == 0 else {
            throw XcodeCLIError.homebrewNotFound
        }
        return result.stdout.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private static func runHomebrewUpdate(
        currentVersion: String, prefix: String, processRunner: some ProcessRunning
    ) async throws -> UpdateResult {
        let result = try await processRunner.run("/usr/local/bin/brew", arguments: ["upgrade", homebrewFormula])
        guard result.exitCode == 0 else {
            throw XcodeCLIError.bridgeSpawnFailed(underlying: "brew upgrade failed: \(result.stderr)")
        }

        let binaryPath = (prefix as NSString).appendingPathComponent("bin/xcodecli")
        let targetVersion = try await inspectVersion(binaryPath: binaryPath, processRunner: processRunner)

        return UpdateResult(
            mode: "homebrew",
            currentVersion: currentVersion,
            targetVersion: targetVersion,
            alreadyUpToDate: currentVersion == targetVersion
        )
    }

    private static func runDirectUpdate(
        currentVersion: String, processRunner: some ProcessRunning
    ) async throws -> UpdateResult {
        // Query latest release tag from GitHub
        let result = try await processRunner.run(
            "/usr/bin/git",
            arguments: ["ls-remote", "--refs", "--tags", "https://github.com/oozoofrog/xcodecli.git"]
        )
        guard result.exitCode == 0 else {
            throw XcodeCLIError.bridgeSpawnFailed(underlying: "git ls-remote failed: \(result.stderr)")
        }

        guard let latestTag = parseLatestTag(result.stdout) else {
            throw XcodeCLIError.mcpInitializationFailed(reason: "no semantic version tags found")
        }

        if currentVersion == latestTag {
            return UpdateResult(
                mode: "direct", currentVersion: currentVersion,
                targetVersion: latestTag, alreadyUpToDate: true
            )
        }

        // TODO: Download, build, and replace binary
        return UpdateResult(
            mode: "direct", currentVersion: currentVersion,
            targetVersion: latestTag, alreadyUpToDate: false
        )
    }

    private static func inspectVersion(
        binaryPath: String, processRunner: some ProcessRunning
    ) async throws -> String {
        let result = try await processRunner.run(binaryPath, arguments: ["version"])
        let fields = result.stdout.trimmingCharacters(in: .whitespacesAndNewlines).split(separator: " ")
        guard fields.count >= 2, fields[0] == "xcodecli" else {
            throw XcodeCLIError.mcpInitializationFailed(reason: "unexpected version output")
        }
        return String(fields[1])
    }

    static func parseLatestTag(_ raw: String) -> String? {
        var versions: [(tag: String, major: Int, minor: Int, patch: Int)] = []

        for line in raw.split(separator: "\n") {
            let fields = line.split(whereSeparator: \.isWhitespace)
            guard fields.count >= 2 else { continue }
            let ref = String(fields.last!)
            guard ref.hasPrefix("refs/tags/v") else { continue }
            guard !ref.hasSuffix("^{}") else { continue }

            let tag = String(ref.dropFirst("refs/tags/".count))
            let parts = tag.dropFirst().split(separator: ".")
            guard parts.count == 3,
                  let major = Int(parts[0]),
                  let minor = Int(parts[1]),
                  let patch = Int(parts[2]) else { continue }

            versions.append((tag: tag, major: major, minor: minor, patch: patch))
        }

        guard !versions.isEmpty else { return nil }

        versions.sort { lhs, rhs in
            if lhs.major != rhs.major { return lhs.major > rhs.major }
            if lhs.minor != rhs.minor { return lhs.minor > rhs.minor }
            return lhs.patch > rhs.patch
        }

        return versions[0].tag
    }

    private static func validateStableUpdateExecutablePath(_ rawPath: String) throws {
        let path = (rawPath as NSString).standardizingPath
        let lower = path.lowercased()

        if !(path as NSString).isAbsolutePath {
            throw XcodeCLIError.mcpInitializationFailed(
                reason: "current executable path must be absolute for `xcodecli update` (\(path)); rerun update from an installed stable path"
            )
        }
        if lower.contains("/.build/") {
            throw XcodeCLIError.mcpInitializationFailed(
                reason: "current executable path looks like a Swift build output (\(path)); rerun `xcodecli update` from an installed stable path"
            )
        }
        if lower.hasPrefix("/tmp/") || lower.hasPrefix("/private/tmp/") {
            throw XcodeCLIError.mcpInitializationFailed(
                reason: "current executable path is in a temporary directory (\(path)); rerun `xcodecli update` from an installed stable path"
            )
        }
        if lower.hasPrefix("/volumes/") {
            throw XcodeCLIError.mcpInitializationFailed(
                reason: "current executable path is on an external volume (\(path)); rerun `xcodecli update` from an installed internal stable path"
            )
        }
    }
}
