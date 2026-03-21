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
        runner.results["launchctl print gui/501/io.test.svc"] = ProcessResult(
            stdout: "service info here", stderr: "", exitCode: 0
        )
        let launchd = CommandLaunchd(runner: runner)
        let output = try await launchd.print(target: "gui/501/io.test.svc")
        #expect(output == "service info here")
    }

    @Test("CommandLaunchd with exit 1 and stderr throws agentUnavailable")
    func commandLaunchdFailure() async {
        var runner = MockProcessRunner()
        runner.results["launchctl print gui/501/bad"] = ProcessResult(
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
}
