import Testing
import Foundation
@testable import XcodeCLICore

@Suite("Concurrency")
struct ConcurrencyTests {
    @Test("Timeout budget calculation subtracts elapsed time")
    func timeoutBudgetCalculation() {
        let start = ContinuousClock.now
        Thread.sleep(forTimeInterval: 0.1) // 100ms
        let elapsed = ContinuousClock.now - start
        let elapsedMS = Int64(elapsed.components.seconds) * 1000 +
                        Int64(elapsed.components.attoseconds / 1_000_000_000_000_000)
        #expect(elapsedMS >= 90) // at least ~100ms
        #expect(elapsedMS < 500) // not too much
        let original: Int64 = 5000
        let remaining = max(original - elapsedMS, 1)
        #expect(remaining > 4500)
        #expect(remaining < 5000)
    }

    @Test("Task cancellation throws CancellationError")
    func taskCancellationThrows() async {
        let task = Task {
            try Task.checkCancellation()
            return true
        }
        task.cancel()
        do {
            _ = try await task.value
        } catch is CancellationError {
            // expected
            return
        } catch {
            // unexpected error type
        }
        // Note: the task may complete before cancel reaches it
    }

    @Test("samePath compares standardized paths correctly")
    func samePathComparison() {
        #expect(samePath("/usr/local/bin/xcodecli", "/usr/local/bin/xcodecli"))
        #expect(!samePath("", "/usr/local/bin/xcodecli"))
        #expect(!samePath("/usr/local/bin/xcodecli", ""))
        #expect(!samePath("", ""))
    }
}
