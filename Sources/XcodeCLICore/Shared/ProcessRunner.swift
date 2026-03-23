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

        // Read pipe data concurrently with process execution to avoid deadlock.
        // If the child produces more output than the pipe buffer (64 KB on macOS),
        // waitUntilExit() and the child's write() would deadlock.
        // Drain pipes on background threads to prevent deadlock when output
        // exceeds the pipe buffer (64 KB on macOS).
        let stdoutReader = Task.detached { stdoutPipe.fileHandleForReading.readDataToEndOfFile() }
        let stderrReader = Task.detached { stderrPipe.fileHandleForReading.readDataToEndOfFile() }

        process.waitUntilExit()

        let stdoutData = await stdoutReader.value
        let stderrData = await stderrReader.value

        return ProcessResult(
            stdout: String(data: stdoutData, encoding: .utf8) ?? "",
            stderr: String(data: stderrData, encoding: .utf8) ?? "",
            exitCode: process.terminationStatus
        )
    }
}
