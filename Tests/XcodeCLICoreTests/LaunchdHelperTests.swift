import Testing
import Foundation
@testable import XcodeCLICore

@Suite("LaunchdHelper")
struct LaunchdHelperTests {
    @Test("launchAgentDomainTarget returns gui/<current uid>")
    func domainTarget() {
        let expected = "gui/\(getuid())"
        #expect(launchAgentDomainTarget() == expected)
    }

    @Test("launchAgentServiceTarget returns gui/<uid>/<label>")
    func serviceTarget() {
        let uid = getuid()
        let label = "io.test.app"
        #expect(launchAgentServiceTarget(label: label) == "gui/\(uid)/\(label)")
    }

    @Test("CommandLaunchd with exit 0 passes correct launchctl args")
    func commandLaunchdSuccess() async throws {
        var runner = MockProcessRunner()
        runner.results["/bin/launchctl print gui/501/io.test.svc"] = ProcessResult(
            stdout: "service info here", stderr: "", exitCode: 0
        )
        let launchd = CommandLaunchd(runner: runner)
        let output = try await launchd.print(target: "gui/501/io.test.svc")
        #expect(output == "service info here")
    }

    @Test("CommandLaunchd with exit 1 and stderr throws agentUnavailable")
    func commandLaunchdFailure() async {
        var runner = MockProcessRunner()
        runner.results["/bin/launchctl print gui/501/bad"] = ProcessResult(
            stdout: "", stderr: "service not found", exitCode: 1
        )
        let launchd = CommandLaunchd(runner: runner)
        do {
            _ = try await launchd.print(target: "gui/501/bad")
            Issue.record("Expected XcodeCLIError.agentUnavailable to be thrown")
        } catch let error as XcodeCLIError {
            switch error {
            case .agentUnavailable(let stage, let underlying):
                #expect(stage == "launchctl")
                #expect(underlying.contains("service not found"))
            default:
                Issue.record("Wrong error case: \(error)")
            }
        } catch {
            Issue.record("Unexpected error type: \(error)")
        }
    }

    @Test("ensureLaunchAgentLoaded retries bootstrap after cleanup when bootstrap fails once")
    func ensureLaunchAgentLoadedRetriesBootstrap() async throws {
        let launchd = FakeLaunchd(
            printError: XcodeCLIError.agentUnavailable(stage: "launchctl", underlying: "not loaded"),
            bootstrapFailures: 1
        )

        try await ensureLaunchAgentLoaded(
            launchd: launchd,
            label: "io.test.retry",
            plistPath: "/tmp/io.test.retry.plist",
            forceRestart: false,
            plistChanged: false
        )

        let state = await launchd.state()
        #expect(state.bootstrapCalls == 2)
        #expect(state.bootoutCalls == 1)
        #expect(state.kickstartCalls == 0)
    }

    @Test("ensureLaunchAgentLoaded rebootstraps after kickstart failure")
    func ensureLaunchAgentLoadedRecoversFromKickstartFailure() async throws {
        let launchd = FakeLaunchd(
            printError: nil,
            bootstrapFailures: 0,
            kickstartFailures: 1
        )

        try await ensureLaunchAgentLoaded(
            launchd: launchd,
            label: "io.test.kickstart",
            plistPath: "/tmp/io.test.kickstart.plist",
            forceRestart: false,
            plistChanged: false
        )

        let state = await launchd.state()
        #expect(state.kickstartCalls == 1)
        #expect(state.bootoutCalls == 1)
        #expect(state.bootstrapCalls == 1)
    }
}

private actor FakeLaunchd: LaunchdInterface {
    struct State: Sendable {
        var bootstrapCalls = 0
        var kickstartCalls = 0
        var bootoutCalls = 0
    }

    private var stateValue = State()
    private let printError: XcodeCLIError?
    private var remainingBootstrapFailures: Int
    private var remainingKickstartFailures: Int

    init(
        printError: XcodeCLIError?,
        bootstrapFailures: Int = 0,
        kickstartFailures: Int = 0
    ) {
        self.printError = printError
        self.remainingBootstrapFailures = bootstrapFailures
        self.remainingKickstartFailures = kickstartFailures
    }

    func print(target: String) async throws -> String {
        if let printError {
            throw printError
        }
        return target
    }

    func bootstrap(domainTarget: String, plistPath: String) async throws {
        stateValue.bootstrapCalls += 1
        if remainingBootstrapFailures > 0 {
            remainingBootstrapFailures -= 1
            throw XcodeCLIError.agentUnavailable(stage: "launchctl", underlying: "Bootstrap failed: 5: Input/output error")
        }
    }

    func kickstart(serviceTarget: String) async throws {
        stateValue.kickstartCalls += 1
        if remainingKickstartFailures > 0 {
            remainingKickstartFailures -= 1
            throw XcodeCLIError.agentUnavailable(stage: "launchctl", underlying: "kickstart failed")
        }
    }

    func bootout(target: String) async throws {
        stateValue.bootoutCalls += 1
    }

    func state() -> State {
        stateValue
    }
}
