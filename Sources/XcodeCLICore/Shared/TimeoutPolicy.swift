import Foundation

public enum TimeoutPolicy {
    // Category defaults (in seconds)
    public static let readTimeout: TimeInterval = 60
    public static let writeTimeout: TimeInterval = 120
    public static let longRunningTimeout: TimeInterval = 30 * 60 // 30 minutes
    public static let fallbackTimeout: TimeInterval = 5 * 60 // 5 minutes

    /// Returns the default timeout for a given tool name.
    public static func defaultToolCallTimeout(toolName: String) -> TimeInterval {
        switch toolName {
        case "XcodeListWindows", "XcodeLS", "XcodeGlob", "XcodeRead", "XcodeGrep",
             "GetBuildLog", "GetTestList", "XcodeListNavigatorIssues", "DocumentationSearch":
            return readTimeout
        case "XcodeUpdate", "XcodeWrite", "XcodeRefreshCodeIssuesInFile":
            return writeTimeout
        case "BuildProject", "RunAllTests", "RunSomeTests":
            return longRunningTimeout
        default:
            return fallbackTimeout
        }
    }

    /// Format a timeout duration for display.
    public static func formatDuration(_ seconds: TimeInterval) -> String {
        if seconds <= 0 { return "0s" }
        let totalSeconds = Int(seconds)
        if totalSeconds >= 3600 && totalSeconds % 3600 == 0 {
            return "\(totalSeconds / 3600)h"
        }
        if totalSeconds >= 300 && totalSeconds % 60 == 0 {
            return "\(totalSeconds / 60)m"
        }
        if seconds == Double(totalSeconds) {
            return "\(totalSeconds)s"
        }
        return String(format: "%.1fs", seconds)
    }
}
