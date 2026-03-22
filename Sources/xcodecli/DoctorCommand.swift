import ArgumentParser
import Foundation
import XcodeCLICore

struct DoctorCommand: AsyncParsableCommand {
    static let configuration = CommandConfiguration(
        commandName: "doctor",
        abstract: "Run environment diagnostics"
    )

    @Flag(name: .long, help: "Print the diagnostic report as pretty JSON")
    var json = false

    @Option(name: .customLong("xcode-pid"), help: "Diagnose the effective MCP_XCODE_PID value")
    var xcodePID: String?

    @Option(name: .customLong("session-id"), help: "Diagnose the effective MCP_XCODE_SESSION_ID value")
    var sessionID: String?

    func run() async throws {
        let env = envDictionary()
        let sessionPath = (try? PathUtilities.sessionFilePath()) ?? ""

        let overrides = EnvOptions(
            xcodePID: xcodePID ?? "",
            sessionID: sessionID ?? ""
        )

        let resolved: ResolvedOptions
        do {
            resolved = try SessionManager.resolve(
                baseEnv: env, overrides: overrides, sessionPath: sessionPath
            )
        } catch {
            resolved = ResolvedOptions(
                envOptions: EnvOptions.effective(baseEnv: env, overrides: overrides),
                sessionSource: .unset,
                sessionPath: sessionPath
            )
        }

        let opts = DoctorOptions(
            baseEnv: env,
            xcodePID: resolved.envOptions.xcodePID,
            sessionID: resolved.envOptions.sessionID,
            sessionSource: resolved.sessionSource,
            sessionPath: resolved.sessionPath
        )

        let inspector = DoctorInspector(processRunner: SystemProcessRunner())
        let report = await inspector.run(opts: opts)

        if json {
            try writePrettyJSON(report.jsonReport)
        } else {
            print(report.textReport, terminator: "")
        }

        if !report.isSuccess {
            throw ExitCode(1)
        }
    }
}

/// Convert ProcessInfo.processInfo.environment to a dictionary.
func envDictionary() -> [String: String] {
    ProcessInfo.processInfo.environment
}
