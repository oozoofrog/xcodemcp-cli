import Testing
import Foundation
@testable import XcodeCLICore

@Suite("SystemProcessRunner")
struct ProcessRunnerTests {

    @Test("handles large stdout without deadlocking")
    func largeStdoutDoesNotDeadlock() async throws {
        // Generate >64KB output to exceed typical pipe buffer size.
        // If the implementation calls waitUntilExit() before reading pipes,
        // the subprocess will block on write and this test will hang.
        let runner = SystemProcessRunner()
        let result = try await runner.run(
            "/usr/bin/python3",
            arguments: ["-c", "import sys; sys.stdout.write('A' * 200000)"],
            environment: nil,
            workingDirectory: nil,
            stdinData: nil
        )
        #expect(result.exitCode == 0)
        #expect(result.stdout.count == 200_000, "expected 200KB stdout, got \(result.stdout.count) bytes")
    }

    @Test("handles large stderr without deadlocking")
    func largeStderrDoesNotDeadlock() async throws {
        let runner = SystemProcessRunner()
        let result = try await runner.run(
            "/usr/bin/python3",
            arguments: ["-c", "import sys; sys.stderr.write('B' * 200000)"],
            environment: nil,
            workingDirectory: nil,
            stdinData: nil
        )
        #expect(result.exitCode == 0)
        #expect(result.stderr.count == 200_000, "expected 200KB stderr, got \(result.stderr.count) bytes")
    }

    @Test("handles large stdout and stderr simultaneously without deadlocking")
    func largeStdoutAndStderrDoesNotDeadlock() async throws {
        let runner = SystemProcessRunner()
        let result = try await runner.run(
            "/usr/bin/python3",
            arguments: ["-c", """
                import sys
                for _ in range(1000):
                    sys.stdout.write('O' * 100)
                    sys.stderr.write('E' * 100)
                sys.stdout.flush()
                sys.stderr.flush()
                """],
            environment: nil,
            workingDirectory: nil,
            stdinData: nil
        )
        #expect(result.exitCode == 0)
        #expect(result.stdout.count == 100_000)
        #expect(result.stderr.count == 100_000)
    }

    @Test("passes stdin data to subprocess")
    func stdinDataPassed() async throws {
        let runner = SystemProcessRunner()
        let input = "hello from stdin"
        let result = try await runner.run(
            "/bin/cat",
            arguments: [],
            environment: nil,
            workingDirectory: nil,
            stdinData: input.data(using: .utf8)
        )
        #expect(result.exitCode == 0)
        #expect(result.stdout == input)
    }

    @Test("reports non-zero exit code")
    func nonZeroExitCode() async throws {
        let runner = SystemProcessRunner()
        let result = try await runner.run(
            "/bin/sh",
            arguments: ["-c", "exit 42"],
            environment: nil,
            workingDirectory: nil,
            stdinData: nil
        )
        #expect(result.exitCode == 42)
    }
}
