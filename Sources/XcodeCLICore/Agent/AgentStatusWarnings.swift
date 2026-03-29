import Foundation

public func deriveAgentStatusWarnings(_ status: AgentStatus) -> [String] {
    var warnings: [String] = []

    if !status.registeredBinary.isEmpty && !(status.registeredBinary as NSString).isAbsolutePath {
        warnings.append("registered LaunchAgent binary path is relative; stale older installs can make launchctl bootstrap fail with Input/output error")
    }

    if !status.registeredBinary.isEmpty && !FileManager.default.isExecutableFile(atPath: status.registeredBinary) {
        warnings.append("registered LaunchAgent binary is missing or not executable; the next LaunchAgent bootstrap may fail until the plist is rewritten")
    }

    if !status.registeredBinary.isEmpty && !status.currentBinary.isEmpty && !status.binaryPathMatches {
        warnings.append("registered LaunchAgent binary differs from the current binary; switching binaries recycles the backend session and can surface fresh Xcode authorization prompts")
    }

    return Array(NSOrderedSet(array: warnings)) as? [String] ?? warnings
}

public func deriveAgentStatusNextSteps(_ status: AgentStatus, warnings: [String]? = nil) -> [String] {
    let effectiveWarnings = warnings ?? deriveAgentStatusWarnings(status)
    var steps: [String] = []

    if !effectiveWarnings.isEmpty {
        steps.append("if the registration looks stale, run `xcodecli agent uninstall` and then re-register from one stable xcodecli path")
    }

    if status.plistInstalled && !status.running && !status.socketReachable {
        steps.append("run `xcodecli agent demo` or `xcodecli tools list --json` to bootstrap the LaunchAgent again")
    }

    return steps
}
