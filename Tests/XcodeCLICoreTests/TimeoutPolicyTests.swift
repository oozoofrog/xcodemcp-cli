import Testing
@testable import XcodeCLICore

@Suite("TimeoutPolicy")
struct TimeoutPolicyTests {
    @Test("BuildProject timeout is 1800 seconds (30 min)")
    func buildProjectTimeout() {
        #expect(TimeoutPolicy.defaultToolCallTimeout(toolName: "BuildProject") == 1800)
    }

    @Test("XcodeRead timeout is 60 seconds")
    func xcodeReadTimeout() {
        #expect(TimeoutPolicy.defaultToolCallTimeout(toolName: "XcodeRead") == 60)
    }

    @Test("XcodeWrite timeout is 120 seconds")
    func xcodeWriteTimeout() {
        #expect(TimeoutPolicy.defaultToolCallTimeout(toolName: "XcodeWrite") == 120)
    }

    @Test("RunAllTests timeout is 1800 seconds (30 min)")
    func runAllTestsTimeout() {
        #expect(TimeoutPolicy.defaultToolCallTimeout(toolName: "RunAllTests") == 1800)
    }

    @Test("UnknownTool falls back to 300 seconds (5 min)")
    func unknownToolFallback() {
        #expect(TimeoutPolicy.defaultToolCallTimeout(toolName: "UnknownTool") == 300)
    }

    @Test("formatDuration renders 3600 as 1h")
    func formatHours() {
        #expect(TimeoutPolicy.formatDuration(3600) == "1h")
    }

    @Test("formatDuration renders 300 as 5m")
    func formatMinutes() {
        #expect(TimeoutPolicy.formatDuration(300) == "5m")
    }

    @Test("formatDuration renders 0 as 0s")
    func formatZero() {
        #expect(TimeoutPolicy.formatDuration(0) == "0s")
    }
}
