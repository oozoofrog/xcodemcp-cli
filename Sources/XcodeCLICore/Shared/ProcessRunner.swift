import Foundation

/// Result of running an external process.
public struct ProcessResult: Sendable {
    public let stdout: String
    public let stderr: String
    public let exitCode: Int32

    public init(stdout: String, stderr: String, exitCode: Int32) {
        self.stdout = stdout
        self.stderr = stderr
        self.exitCode = exitCode
    }
}

/// Abstraction over external process execution for testability.
public protocol ProcessRunning: Sendable {
    func run(
        _ command: String,
        arguments: [String],
        environment: [String: String]?,
        workingDirectory: String?,
        stdinData: Data?
    ) async throws -> ProcessResult
}

extension ProcessRunning {
    public func run(
        _ command: String,
        arguments: [String] = [],
        environment: [String: String]? = nil,
        workingDirectory: String? = nil
    ) async throws -> ProcessResult {
        try await run(command, arguments: arguments, environment: environment,
                      workingDirectory: workingDirectory, stdinData: nil)
    }
}

/// Default implementation using Foundation.Process.
public struct SystemProcessRunner: ProcessRunning {
    public init() {}

    public func run(
        _ command: String,
        arguments: [String],
        environment: [String: String]?,
        workingDirectory: String?,
        stdinData: Data?
    ) async throws -> ProcessResult {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: command)
        process.arguments = arguments

        if let environment {
            process.environment = environment
        }
        if let workingDirectory {
            process.currentDirectoryURL = URL(fileURLWithPath: workingDirectory)
        }

        let stdoutPipe = Pipe()
        let stderrPipe = Pipe()
        process.standardOutput = stdoutPipe
        process.standardError = stderrPipe

        if let stdinData {
            let stdinPipe = Pipe()
            process.standardInput = stdinPipe
            stdinPipe.fileHandleForWriting.write(stdinData)
            stdinPipe.fileHandleForWriting.closeFile()
        }

        try process.run()

        // Read pipes concurrently BEFORE waitUntilExit to avoid deadlock.
        // If the subprocess fills the pipe buffer (~64KB) while the parent
        // is blocked on waitUntilExit, both processes deadlock.
        async let stdoutData = Self.readAll(from: stdoutPipe.fileHandleForReading)
        async let stderrData = Self.readAll(from: stderrPipe.fileHandleForReading)

        let exitCode = await withCheckedContinuation { continuation in
            process.terminationHandler = { proc in
                continuation.resume(returning: proc.terminationStatus)
            }
        }

        let out = await stdoutData
        let err = await stderrData

        return ProcessResult(
            stdout: String(data: out, encoding: .utf8) ?? "",
            stderr: String(data: err, encoding: .utf8) ?? "",
            exitCode: exitCode
        )
    }

    private static func readAll(from handle: FileHandle) async -> Data {
        await withCheckedContinuation { continuation in
            DispatchQueue.global().async {
                let data = handle.readDataToEndOfFile()
                continuation.resume(returning: data)
            }
        }
    }
}
