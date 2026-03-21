import Testing
import Foundation
@testable import XcodeCLICore

@Suite("InFlightTracker")
struct InFlightTrackerTests {
    @Test("register returns true on first call, finish returns true")
    func registerAndFinish() {
        let tracker = InFlightTracker()
        let task = Task<Void, Never> {}
        #expect(tracker.register(key: "req-1", task: task))
        #expect(tracker.finish(key: "req-1"))
    }

    @Test("register same key twice returns false (duplicate detection)")
    func duplicateRegistration() {
        let tracker = InFlightTracker()
        let task1 = Task<Void, Never> {}
        let task2 = Task<Void, Never> {}
        #expect(tracker.register(key: "dup", task: task1))
        #expect(!tracker.register(key: "dup", task: task2))
    }

    @Test("cancel sets cancelled; subsequent finish returns false")
    func cancelSuppressesFinish() {
        let tracker = InFlightTracker()
        let task = Task<Void, Never> {}
        #expect(tracker.register(key: "c1", task: task))
        #expect(tracker.cancel(key: "c1"))
        // finish should return false because the request was cancelled
        #expect(!tracker.finish(key: "c1"))
    }

    @Test("cancelAll cancels all tracked tasks and clears")
    func cancelAll() {
        let tracker = InFlightTracker()
        let task1 = Task<Void, Never> {}
        let task2 = Task<Void, Never> {}
        _ = tracker.register(key: "a", task: task1)
        _ = tracker.register(key: "b", task: task2)
        tracker.cancelAll()
        // After cancelAll, finish should return false for both (removed)
        #expect(!tracker.finish(key: "a"))
        #expect(!tracker.finish(key: "b"))
    }

    @Test("canonicalRequestKey for .int(42) returns \"42\"")
    func canonicalKeyInt() {
        let result = canonicalRequestKey(.int(42))
        #expect(result == "42")
    }

    @Test("canonicalRequestKey for .string(\"abc\") returns quoted string")
    func canonicalKeyString() {
        let result = canonicalRequestKey(.string("abc"))
        #expect(result == "\"abc\"")
    }
}
