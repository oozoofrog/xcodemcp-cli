import Testing
import Foundation
@testable import xcodecli
import XcodeCLICore

@Suite("Agent Environment Support")
struct AgentEnvironmentSupportTests {
    actor Recorder {
        var statusCalls = 0
        var doctorOptions: DoctorOptions?

        func recordStatusCall() { statusCalls += 1 }
        func recordDoctorOptions(_ options: DoctorOptions) { doctorOptions = options }
        func snapshot() -> (Int, DoctorOptions?) { (statusCalls, doctorOptions) }
    }

    @Test("collectReadOnlyEnvironment uses initial status for doctor and post-tools status for report")
    func collectsStatusesAndDoctorInput() async {
        let recorder = Recorder()
        let initialStatus = AgentStatus(running: false, pid: 111)
        let postStatus = AgentStatus(running: true, pid: 222)
        let statuses = [initialStatus, postStatus]

        final class Box: @unchecked Sendable { var index = 0 }
        let box = Box()

        let result = await collectReadOnlyEnvironment(
            env: [:],
            effective: EnvOptions(sessionID: "11111111-1111-1111-1111-111111111111"),
            resolved: ResolvedOptions(
                envOptions: EnvOptions(sessionID: "11111111-1111-1111-1111-111111111111"),
                sessionSource: .generated,
                sessionPath: "/tmp/session-id"
            ),
            timeout: 60,
            debug: false,
            highlightToolNames: ["XcodeListWindows"],
            windowsStep: "windows",
            statusProvider: {
                await recorder.recordStatusCall()
                let value = statuses[box.index]
                box.index += 1
                return value
            },
            toolsProvider: { _ in
                [
                    .object([
                        "name": .string("XcodeListWindows"),
                        "description": .string("Lists windows"),
                    ])
                ]
            },
            toolCaller: { _, _, _ in
                MCPCallResult(result: [
                    "structuredContent": .object(["message": .string("* tabIdentifier: tab-1, workspacePath: /tmp/App.xcodeproj")])
                ])
            },
            doctorRunner: { options in
                await recorder.recordDoctorOptions(options)
                return DoctorReport(checks: [])
            }
        )

        let snapshot = await recorder.snapshot()
        #expect(snapshot.0 == 2)
        #expect(snapshot.1?.agentStatus?.pid == 111)
        #expect(result.postToolsStatus?.pid == 222)
        #expect(result.errors.isEmpty)
        #expect(result.windowsTool.attempted == true)
        #expect(result.windowsTool.ok == true)
    }

    @Test("collectReadOnlyEnvironment records missing windows tool as a windows-step error")
    func missingWindowsToolProducesError() async {
        let result = await collectReadOnlyEnvironment(
            env: [:],
            effective: EnvOptions(),
            resolved: ResolvedOptions(),
            timeout: 60,
            debug: false,
            highlightToolNames: ["BuildProject"],
            windowsStep: "windows demo",
            statusProvider: { AgentStatus() },
            toolsProvider: { _ in
                [.object(["name": .string("BuildProject")])]
            },
            toolCaller: { _, _, _ in
                Issue.record("toolCaller should not run when windows tool is missing")
                return MCPCallResult(result: [:])
            },
            doctorRunner: { _ in DoctorReport(checks: []) }
        )

        #expect(result.windowsTool.attempted == false)
        #expect(result.windowsTool.error == EnvironmentStepError(step: "windows demo", message: "tool not found: XcodeListWindows"))
        #expect(result.errors.contains(EnvironmentStepError(step: "windows demo", message: "tool not found: XcodeListWindows")))
    }

    @Test("collectReadOnlyEnvironment records tool list failures and skips windows execution")
    func toolsListFailureSkipsWindows() async {
        enum TestError: Error, LocalizedError {
            case failed
            var errorDescription: String? { "tool listing failed" }
        }

        let result = await collectReadOnlyEnvironment(
            env: [:],
            effective: EnvOptions(),
            resolved: ResolvedOptions(),
            timeout: 60,
            debug: false,
            highlightToolNames: ["XcodeListWindows"],
            windowsStep: "windows",
            statusProvider: { AgentStatus() },
            toolsProvider: { _ in throw TestError.failed },
            toolCaller: { _, _, _ in
                Issue.record("toolCaller should not run when tools list fails")
                return MCPCallResult(result: [:])
            },
            doctorRunner: { _ in DoctorReport(checks: []) }
        )

        #expect(result.tools == nil)
        #expect(result.toolCatalog.count == 0)
        #expect(result.windowsTool.attempted == false)
        #expect(result.errors == [EnvironmentStepError(step: "tools list", message: "tool listing failed")])
    }
}
