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

@Suite("Doctor")
struct DoctorTests {
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
        #expect(json.contains("\"recommendations\""))
    }

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

    @Test("inspector warns when LaunchAgent registered binary path is relative")
    func relativeLaunchAgentBinaryWarns() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcrun mcpbridge --help": ProcessResult(stdout: "ok", stderr: "", exitCode: 0),
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
            "/usr/bin/xcrun mcpbridge": ProcessResult(stdout: "", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { name in name == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [] }
        )

        let report = await inspector.run(opts: DoctorOptions(
            sessionID: UUID().uuidString.lowercased(),
            sessionSource: .persisted,
            sessionPath: "/tmp/session-id",
            agentStatus: AgentStatus(
                plistInstalled: true,
                registeredBinary: "../Cellar/xcodecli/1.0.1/bin/xcodecli",
                currentBinary: "/opt/homebrew/bin/xcodecli",
                binaryPathMatches: false
            )
        ))

        let check = report.checks.first { $0.name == "LaunchAgent binary registration" }
        #expect(check?.status == .warn)
        #expect(check?.detail.contains("relative") == true)
        #expect(check?.detail.contains("Input/output error") == true)
    }

    @Test("inspector warns when explicit MCP_XCODE_PID partitions pooled sessions")
    func explicitPIDWarns() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcrun mcpbridge --help": ProcessResult(stdout: "ok", stderr: "", exitCode: 0),
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
            "/usr/bin/xcrun mcpbridge": ProcessResult(stdout: "", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { name in name == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [XcodeProcess(pid: 123, command: "/Applications/Xcode.app/Contents/MacOS/Xcode")] }
        )

        let report = await inspector.run(opts: DoctorOptions(
            xcodePID: "123",
            sessionID: UUID().uuidString.lowercased(),
            sessionSource: .persisted,
            sessionPath: "/tmp/session-id"
        ))

        let check = report.checks.first { $0.name == "effective MCP_XCODE_PID" }
        #expect(check?.status == .warn)
        #expect(check?.detail.contains("partitions the pooled session key") == true)
    }

    @Test("inspector warns when DEVELOPER_DIR drifts from xcode-select")
    func developerDirDriftWarns() async {
        let runner = MockProcessRunner(results: [
            "/usr/bin/xcrun mcpbridge --help": ProcessResult(stdout: "ok", stderr: "", exitCode: 0),
            "/usr/bin/xcode-select -p": ProcessResult(stdout: "/Applications/Xcode.app/Contents/Developer\n", stderr: "", exitCode: 0),
            "/usr/bin/xcrun mcpbridge": ProcessResult(stdout: "", stderr: "", exitCode: 0),
        ])
        let inspector = DoctorInspector(
            processRunner: runner,
            lookPath: { name in name == "xcrun" ? "/usr/bin/xcrun" : nil },
            listProcesses: { [] }
        )

        let report = await inspector.run(opts: DoctorOptions(
            baseEnv: ["DEVELOPER_DIR": "/Applications/Xcode-beta.app/Contents/Developer"],
            sessionID: UUID().uuidString.lowercased(),
            sessionSource: .persisted,
            sessionPath: "/tmp/session-id"
        ))

        let check = report.checks.first { $0.name == "effective DEVELOPER_DIR" }
        #expect(check?.status == .warn)
        #expect(check?.detail.contains("pooled session key") == true)
    }

    @Test("report recommendations include launchagent remediation")
    func launchAgentRecommendations() {
        let report = DoctorReport(checks: [
            DoctorCheck(name: "LaunchAgent binary registration", status: .warn, detail: "registered=../Cellar/xcodecli | current=/opt/homebrew/bin/xcodecli | match=false | registered binary path is relative")
        ])
        #expect(!report.recommendations.isEmpty)
        #expect(report.recommendations.first?.id == "launchagent-registration")
        #expect(report.textReport.contains("Recommendations:"))
    }
}
