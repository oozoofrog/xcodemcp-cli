import Testing
import Foundation
@testable import XcodeCLICore

// MARK: - Mock Process Runner

struct MockProcessRunner: ProcessRunning {
    var results: [String: ProcessResult] = [:]
    var defaultResult = ProcessResult(stdout: "", stderr: "", exitCode: 0)

    func run(
        _ command: String,
        arguments: [String],
        environment: [String: String]?,
        workingDirectory: String?,
        stdinData: Data?
    ) async throws -> ProcessResult {
        let key = ([command] + arguments).joined(separator: " ")
        return results[key] ?? defaultResult
    }
}

// MARK: - Capturing Process Runner (records calls for verification)

final class CapturingProcessRunner: ProcessRunning, @unchecked Sendable {
    struct Call: Sendable {
        let command: String
        let arguments: [String]
        let environment: [String: String]?
        let stdinData: Data?
    }

    private var _calls: [Call] = []
    var calls: [Call] { _calls }

    var results: [String: ProcessResult] = [:]
    var defaultResult = ProcessResult(stdout: "", stderr: "", exitCode: 0)

    init(results: [String: ProcessResult] = [:]) {
        self.results = results
    }

    func run(
        _ command: String,
        arguments: [String],
        environment: [String: String]?,
        workingDirectory: String?,
        stdinData: Data?
    ) async throws -> ProcessResult {
        let call = Call(command: command, arguments: arguments, environment: environment, stdinData: stdinData)
        _calls.append(call)
        let key = ([command] + arguments).joined(separator: " ")
        return results[key] ?? defaultResult
    }
}

@Suite("Doctor")
struct DoctorTests {

    // MARK: - Report model tests

    @Test("report with all checks OK is successful")
    func successfulReport() {
        let checks = [
            DoctorCheck(name: "test1", status: .ok, detail: "good"),
            DoctorCheck(name: "test2", status: .info, detail: "fyi"),
        ]
        let report = DoctorReport(checks: checks)
        #expect(report.isSuccess)
    }

    @Test("report with a fail check is not successful")
    func failedReport() {
        let checks = [
            DoctorCheck(name: "test1", status: .ok, detail: "good"),
            DoctorCheck(name: "test2", status: .fail, detail: "bad"),
        ]
        let report = DoctorReport(checks: checks)
        #expect(!report.isSuccess)
    }

    @Test("summary counts statuses correctly")
    func summaryCounts() {
        let checks = [
            DoctorCheck(name: "a", status: .ok, detail: ""),
            DoctorCheck(name: "b", status: .ok, detail: ""),
            DoctorCheck(name: "c", status: .warn, detail: ""),
            DoctorCheck(name: "d", status: .fail, detail: ""),
            DoctorCheck(name: "e", status: .info, detail: ""),
            DoctorCheck(name: "f", status: .info, detail: ""),
        ]
        let report = DoctorReport(checks: checks)
        let s = report.summary
        #expect(s.ok == 2)
        #expect(s.warn == 1)
        #expect(s.fail == 1)
        #expect(s.info == 2)
    }

    @Test("text report includes status icons")
    func textReportFormat() {
        let checks = [
            DoctorCheck(name: "xcrun lookup", status: .ok, detail: "/usr/bin/xcrun"),
        ]
        let report = DoctorReport(checks: checks)
        let text = report.textReport
        #expect(text.contains("[OK] xcrun lookup: /usr/bin/xcrun"))
        #expect(text.contains("xcodecli doctor"))
    }

    @Test("JSON report encodes correctly")
    func jsonReportEncoding() throws {
        let checks = [
            DoctorCheck(name: "test", status: .ok, detail: "fine"),
        ]
        let report = DoctorReport(checks: checks)
        let encoder = JSONEncoder()
        let data = try encoder.encode(report.jsonReport)
        let json = String(data: data, encoding: .utf8)!
        #expect(json.contains("\"success\":true"))
        #expect(json.contains("\"ok\":1"))
    }

    // MARK: - Inspector: xcrun lookup

    @Test("inspector handles xcrun not found")
    func xcrunNotFound() async {
        let runner = MockProcessRunner()
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { _ in nil },
            listProcesses: { [] }
        )
        let report = await inspector.run(opts: DoctorOptions())
        let xcrunCheck = report.checks.first { $0.name == "xcrun lookup" }
        #expect(xcrunCheck?.status == .fail)
    }

    @Test("inspector skips mcpbridge check when xcrun unavailable")
    func mcpbridgeSkippedWithoutXcrun() async {
        let runner = MockProcessRunner()
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { _ in nil },
            listProcesses: { [] }
        )
        let report = await inspector.run(opts: DoctorOptions())
        let bridgeCheck = report.checks.first { $0.name == "xcrun mcpbridge --help" }
        #expect(bridgeCheck?.status == .info)
        #expect(bridgeCheck?.detail.contains("skipped") == true)
    }

    // MARK: - Inspector: full success path

    @Test("inspector full success produces all expected checks")
    func inspectorFullSuccess() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcrun mcpbridge --help": ProcessResult(stdout: "help text", stderr: "", exitCode: 0),
            "/usr/bin/xcrun mcpbridge": ProcessResult(stdout: "", stderr: "", exitCode: 0),
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [XcodeProcess(pid: 101, command: "/Applications/Xcode.app/Contents/MacOS/Xcode")] }
        )
        let report = await inspector.run(opts: DoctorOptions(
            xcodePID: "101",
            sessionID: "11111111-1111-1111-1111-111111111111"
        ))
        #expect(report.isSuccess, "expected success report, got: \(report.textReport)")

        // Verify all core checks are present
        let checkNames = report.checks.map { $0.name }
        let expectedNames = [
            "xcrun lookup",
            "xcrun mcpbridge --help",
            "xcode-select -p",
            "running Xcode processes",
            "effective MCP_XCODE_PID",
            "effective MCP_XCODE_SESSION_ID",
            "spawn smoke test",
        ]
        for name in expectedNames {
            #expect(checkNames.contains(name), "missing check: \(name)")
        }

        // All core checks should be ok
        for name in expectedNames {
            let check = report.checks.first { $0.name == name }
            #expect(check?.status == .ok, "\(name) should be ok, got \(check?.status.rawValue ?? "nil"): \(check?.detail ?? "")")
        }
    }

    // MARK: - Inspector: PID validation

    @Test("inspector validates PID against running processes")
    func pidValidation() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])

        // PID not found in process list
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [XcodeProcess(pid: 200, command: "/Applications/Xcode.app/Contents/MacOS/Xcode")] }
        )
        let report = await inspector.run(opts: DoctorOptions(xcodePID: "999"))
        let pidCheck = report.checks.first { $0.name == "effective MCP_XCODE_PID" }
        #expect(pidCheck?.status == .fail)
        #expect(pidCheck?.detail.contains("999") == true)
        #expect(pidCheck?.detail.contains("not found") == true)
    }

    @Test("inspector rejects PID that does not look like Xcode")
    func pidNotXcode() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [XcodeProcess(pid: 42, command: "/usr/bin/vim")] }
        )
        let report = await inspector.run(opts: DoctorOptions(xcodePID: "42"))
        let pidCheck = report.checks.first { $0.name == "effective MCP_XCODE_PID" }
        #expect(pidCheck?.status == .fail)
        #expect(pidCheck?.detail.contains("does not look like") == true)
    }

    @Test("inspector rejects invalid PID format")
    func pidInvalidFormat() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [] }
        )
        let report = await inspector.run(opts: DoctorOptions(xcodePID: "notanumber"))
        let pidCheck = report.checks.first { $0.name == "effective MCP_XCODE_PID" }
        #expect(pidCheck?.status == .fail)
    }

    @Test("inspector shows info when PID not set")
    func pidNotSet() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [] }
        )
        let report = await inspector.run(opts: DoctorOptions())
        let pidCheck = report.checks.first { $0.name == "effective MCP_XCODE_PID" }
        #expect(pidCheck?.status == .info)
        #expect(pidCheck?.detail == "not set")
    }

    // MARK: - Inspector: session validation

    @Test("inspector rejects invalid session UUID")
    func sessionInvalidUUID() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [] }
        )
        let report = await inspector.run(opts: DoctorOptions(sessionID: "not-a-uuid"))
        let sessionCheck = report.checks.first { $0.name == "effective MCP_XCODE_SESSION_ID" }
        #expect(sessionCheck?.status == .fail)
        #expect(sessionCheck?.detail.contains("UUID") == true)
    }

    @Test("inspector shows info when session not set")
    func sessionNotSet() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [] }
        )
        let report = await inspector.run(opts: DoctorOptions())
        let sessionCheck = report.checks.first { $0.name == "effective MCP_XCODE_SESSION_ID" }
        #expect(sessionCheck?.status == .info)
        #expect(sessionCheck?.detail == "not set")
    }

    // MARK: - Inspector: smoke test skip

    @Test("inspector skips smoke test when PID override is invalid")
    func smokeTestSkippedForInvalidPID() async {
        let capturer = CapturingProcessRunner(results: [
            "/usr/bin/xcrun mcpbridge --help": ProcessResult(stdout: "help", stderr: "", exitCode: 0),
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: capturer,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [XcodeProcess(pid: 101, command: "/Applications/Xcode.app/Contents/MacOS/Xcode")] }
        )
        // PID 0 is invalid (must be positive)
        let report = await inspector.run(opts: DoctorOptions(xcodePID: "0"))
        let smokeCheck = report.checks.first { $0.name == "spawn smoke test" }
        #expect(smokeCheck?.status == .info)
        #expect(smokeCheck?.detail.contains("skipped") == true)

        // Verify mcpbridge was NOT called for smoke test
        let smokeCalls = capturer.calls.filter { $0.arguments == ["mcpbridge"] }
        #expect(smokeCalls.isEmpty, "smoke test should not have been called")
    }

    @Test("inspector skips smoke test when session override is invalid")
    func smokeTestSkippedForInvalidSession() async {
        let capturer = CapturingProcessRunner(results: [
            "/usr/bin/xcrun mcpbridge --help": ProcessResult(stdout: "help", stderr: "", exitCode: 0),
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: capturer,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [XcodeProcess(pid: 101, command: "/Applications/Xcode.app/Contents/MacOS/Xcode")] }
        )
        let report = await inspector.run(opts: DoctorOptions(sessionID: "bad-uuid"))
        let smokeCheck = report.checks.first { $0.name == "spawn smoke test" }
        #expect(smokeCheck?.status == .info)
        #expect(smokeCheck?.detail.contains("skipped") == true)
    }

    // MARK: - Inspector: smoke test env overrides

    @Test("smoke test passes PID and session env overrides")
    func smokeTestEnvOverrides() async {
        let capturer = CapturingProcessRunner(results: [
            "/usr/bin/xcrun mcpbridge --help": ProcessResult(stdout: "help", stderr: "", exitCode: 0),
            "/usr/bin/xcrun mcpbridge": ProcessResult(stdout: "", stderr: "", exitCode: 0),
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: capturer,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [XcodeProcess(pid: 101, command: "/Applications/Xcode.app/Contents/MacOS/Xcode")] }
        )
        let _ = await inspector.run(opts: DoctorOptions(
            baseEnv: ["HOME": "/tmp"],
            xcodePID: "101",
            sessionID: "11111111-1111-1111-1111-111111111111"
        ))

        let smokeCalls = capturer.calls.filter { $0.arguments == ["mcpbridge"] }
        #expect(smokeCalls.count == 1)
        let env = smokeCalls[0].environment ?? [:]
        #expect(env["MCP_XCODE_PID"] == "101")
        #expect(env["MCP_XCODE_SESSION_ID"] == "11111111-1111-1111-1111-111111111111")
    }

    // MARK: - Inspector: agent status

    @Test("inspector includes agent status info checks")
    func agentStatusIncluded() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [XcodeProcess(pid: 101, command: "/Applications/Xcode.app/Contents/MacOS/Xcode")] }
        )
        let report = await inspector.run(opts: DoctorOptions(
            agentStatus: AgentStatus(
                plistPath: "/tmp/io.oozoofrog.xcodecli.plist",
                plistInstalled: true,
                registeredBinary: "/tmp/xcodecli",
                currentBinary: "/tmp/xcodecli",
                binaryPathMatches: true,
                socketPath: "/tmp/daemon.sock",
                socketReachable: true
            )
        ))
        let checkNames = report.checks.map { $0.name }
        #expect(checkNames.contains("LaunchAgent plist"), "missing LaunchAgent plist check")
        #expect(checkNames.contains("LaunchAgent socket"), "missing LaunchAgent socket check")
        #expect(checkNames.contains("LaunchAgent binary registration"), "missing binary registration check")
    }

    @Test("inspector shows agent status error when available")
    func agentStatusError() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [] }
        )
        let report = await inspector.run(opts: DoctorOptions(
            agentStatusError: "connection refused"
        ))
        let statusCheck = report.checks.first { $0.name == "LaunchAgent status" }
        #expect(statusCheck?.status == .info)
        #expect(statusCheck?.detail.contains("connection refused") == true)
    }

    // MARK: - Inspector: Xcode process detection

    @Test("inspector warns when no Xcode process detected")
    func noXcodeProcess() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [XcodeProcess(pid: 1, command: "/sbin/launchd")] }
        )
        let report = await inspector.run(opts: DoctorOptions())
        let processCheck = report.checks.first { $0.name == "running Xcode processes" }
        #expect(processCheck?.status == .warn)
        #expect(processCheck?.detail.contains("no Xcode.app process") == true)
    }

    @Test("inspector handles process list failure")
    func processListFailure() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { cmd in cmd == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { throw XcodeCLIError.bridgeSpawnFailed(underlying: "ps failed") }
        )
        let report = await inspector.run(opts: DoctorOptions())
        let processCheck = report.checks.first { $0.name == "running Xcode processes" }
        #expect(processCheck?.status == .fail)
    }

    // MARK: - parseProcessList

    @Test("parseProcessList parses standard ps output")
    func parseProcessListBasic() {
        let output = """
          1 /sbin/launchd
        456 /Applications/Xcode.app/Contents/MacOS/Xcode
        789 /usr/sbin/sshd
        """
        let procs = parseProcessList(output)
        #expect(procs.count == 3)
        #expect(procs[0].pid == 1)
        #expect(procs[0].command == "/sbin/launchd")
        #expect(procs[1].pid == 456)
        #expect(procs[1].looksLikeXcode)
        #expect(!procs[2].looksLikeXcode)
    }

    @Test("parseProcessList handles empty input")
    func parseProcessListEmpty() {
        #expect(parseProcessList("").isEmpty)
        #expect(parseProcessList("\n\n").isEmpty)
    }

    // MARK: - XcodeProcess.looksLikeXcode

    @Test("looksLikeXcode detects Xcode processes")
    func looksLikeXcodeDetection() {
        #expect(XcodeProcess(pid: 1, command: "/Applications/Xcode.app/Contents/MacOS/Xcode").looksLikeXcode)
        #expect(XcodeProcess(pid: 1, command: "/Applications/Xcode-16.0.app/Contents/MacOS/Xcode -something").looksLikeXcode)
        #expect(XcodeProcess(pid: 1, command: "Xcode").looksLikeXcode)
        #expect(!XcodeProcess(pid: 1, command: "/usr/bin/xcodebuild").looksLikeXcode)
        #expect(!XcodeProcess(pid: 1, command: "/usr/bin/vim").looksLikeXcode)
    }
}
