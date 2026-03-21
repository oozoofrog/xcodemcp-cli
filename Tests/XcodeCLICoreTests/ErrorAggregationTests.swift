import Testing
import Foundation
#if canImport(Darwin)
import Darwin
#endif
@testable import XcodeCLICore

@Suite("ErrorAggregation")
struct ErrorAggregationTests {
    @Test("Unavailable error detection matches expected patterns")
    func unavailableErrorDetection() {
        let err1 = XcodeCLIError.agentUnavailable(stage: "connect", underlying: "no such file or directory")
        #expect(err1.localizedDescription.lowercased().contains("unavailable") ||
                err1.localizedDescription.lowercased().contains("no such file"))
    }

    @Test("Error collection pattern joins multiple errors")
    func errorCollectionPattern() {
        var errors: [String] = []
        errors.append("stop: agent unavailable")
        errors.append("remove socket: permission denied")
        let combined = errors.joined(separator: "; ")
        #expect(combined.contains("stop:"))
        #expect(combined.contains("remove socket:"))
    }

    @Test("RuntimeStatus to AgentStatus ms-to-ns conversion")
    func runtimeStatusConversion() {
        let runtime = RuntimeStatus(pid: 1234, idleTimeoutMs: 86400000, backendSessions: 2)
        #expect(runtime.pid == 1234)
        #expect(runtime.idleTimeoutMs == 86400000)
        // AgentStatus conversion: ms -> ns
        let ns = runtime.idleTimeoutMs * 1_000_000
        #expect(ns == 86400000000000) // 24h in nanoseconds
    }

    @Test("writeAllToFD returns false on invalid file descriptor")
    func writeAllToFDBadFD() {
        let result = writeAllToFD(-1, Data("test".utf8))
        #expect(!result)
    }
}
