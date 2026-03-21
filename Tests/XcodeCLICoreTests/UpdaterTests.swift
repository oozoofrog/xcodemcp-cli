import Testing
import Foundation
@testable import XcodeCLICore

@Suite("Updater")
struct UpdaterTests {

    // MARK: - parseLatestTag

    @Test("parseLatestTag selects newest semantic version")
    func parseLatestTagSelectsNewest() {
        let raw = [
            "abc\trefs/tags/not-a-version",
            "def\trefs/tags/v0.5.2",
            "ghi\trefs/tags/v0.5.10",
            "jkl\trefs/tags/v0.6.0^{}",
            "mno\trefs/tags/v0.6.0",
        ].joined(separator: "\n")

        let result = Updater.parseLatestTag(raw)
        #expect(result == "v0.6.0")
    }

    @Test("parseLatestTag returns nil when no semantic version tags exist")
    func parseLatestTagNoVersions() {
        let result = Updater.parseLatestTag("abc refs/tags/not-semver")
        #expect(result == nil)
    }

    @Test("parseLatestTag returns nil for empty input")
    func parseLatestTagEmpty() {
        let result = Updater.parseLatestTag("")
        #expect(result == nil)
    }

    @Test("parseLatestTag ignores annotated tag derefs (^{})")
    func parseLatestTagIgnoresDerefs() {
        let raw = [
            "abc\trefs/tags/v1.0.0^{}",
        ].joined(separator: "\n")

        let result = Updater.parseLatestTag(raw)
        #expect(result == nil)
    }

    @Test("parseLatestTag sorts by major, minor, patch correctly")
    func parseLatestTagSortOrder() {
        let raw = [
            "a\trefs/tags/v0.1.9",
            "b\trefs/tags/v0.2.0",
            "c\trefs/tags/v1.0.0",
            "d\trefs/tags/v0.99.99",
        ].joined(separator: "\n")

        let result = Updater.parseLatestTag(raw)
        #expect(result == "v1.0.0")
    }

    @Test("parseLatestTag handles single valid tag")
    func parseLatestTagSingleTag() {
        let raw = "abc\trefs/tags/v0.5.4"
        let result = Updater.parseLatestTag(raw)
        #expect(result == "v0.5.4")
    }

    @Test("parseLatestTag skips tags not starting with refs/tags/v")
    func parseLatestTagSkipsNonVersionTags() {
        let raw = [
            "abc\trefs/tags/release-1.0",
            "def\trefs/tags/v0.3.1",
            "ghi\trefs/heads/v1.0.0",
        ].joined(separator: "\n")

        let result = Updater.parseLatestTag(raw)
        #expect(result == "v0.3.1")
    }

    @Test("parseLatestTag skips tags with non-numeric parts")
    func parseLatestTagSkipsNonNumeric() {
        let raw = [
            "abc\trefs/tags/v0.1.beta",
            "def\trefs/tags/v0.2.0",
        ].joined(separator: "\n")

        let result = Updater.parseLatestTag(raw)
        #expect(result == "v0.2.0")
    }

    @Test("parseLatestTag skips tags with wrong number of version components")
    func parseLatestTagSkipsWrongComponents() {
        let raw = [
            "abc\trefs/tags/v1.0",
            "def\trefs/tags/v1.0.0.0",
            "ghi\trefs/tags/v2.0.0",
        ].joined(separator: "\n")

        let result = Updater.parseLatestTag(raw)
        #expect(result == "v2.0.0")
    }

    // MARK: - UpdateResult

    @Test("UpdateResult stores all fields correctly")
    func updateResultFields() {
        let result = UpdateResult(
            mode: "homebrew",
            currentVersion: "v0.5.2",
            targetVersion: "v0.5.3",
            alreadyUpToDate: false
        )
        #expect(result.mode == "homebrew")
        #expect(result.currentVersion == "v0.5.2")
        #expect(result.targetVersion == "v0.5.3")
        #expect(!result.alreadyUpToDate)
    }

    // MARK: - run() with mock process runner

    @Test("run rejects empty version string")
    func runRejectsEmptyVersion() async {
        let runner = MockProcessRunner()
        await #expect(throws: XcodeCLIError.self) {
            _ = try await Updater.run(currentVersion: "", processRunner: runner)
        }
    }

    @Test("run rejects whitespace-only version string")
    func runRejectsWhitespaceVersion() async {
        let runner = MockProcessRunner()
        await #expect(throws: XcodeCLIError.self) {
            _ = try await Updater.run(currentVersion: "   ", processRunner: runner)
        }
    }

    @Test("run trims version whitespace before processing")
    func runTrimsVersion() async throws {
        // When homebrew is not found, falls back to direct update with git ls-remote.
        // Provide git ls-remote output that matches the trimmed version.
        var runner = MockProcessRunner()
        runner.results["/usr/local/bin/brew --prefix oozoofrog/tap/xcodecli"] =
            ProcessResult(stdout: "", stderr: "not found", exitCode: 1)
        runner.results["/usr/bin/git ls-remote --refs --tags https://github.com/oozoofrog/xcodecli.git"] =
            ProcessResult(stdout: "abc\trefs/tags/v0.5.2", stderr: "", exitCode: 0)

        let result = try await Updater.run(currentVersion: "  v0.5.2  ", processRunner: runner)
        #expect(result.currentVersion == "v0.5.2")
        #expect(result.alreadyUpToDate)
    }

    @Test("run uses homebrew mode when brew prefix succeeds")
    func runHomebrewMode() async throws {
        let prefix = "/opt/homebrew/Cellar/xcodecli/0.5.2"
        let binaryPath = "\(prefix)/bin/xcodecli"

        var runner = MockProcessRunner()
        runner.results["/usr/local/bin/brew --prefix oozoofrog/tap/xcodecli"] =
            ProcessResult(stdout: prefix, stderr: "", exitCode: 0)
        runner.results["/usr/local/bin/brew upgrade oozoofrog/tap/xcodecli"] =
            ProcessResult(stdout: "Warning: already installed", stderr: "", exitCode: 0)
        runner.results["\(binaryPath) version"] =
            ProcessResult(stdout: "xcodecli v0.5.2", stderr: "", exitCode: 0)

        let result = try await Updater.run(currentVersion: "v0.5.2", processRunner: runner)
        #expect(result.mode == "homebrew")
        #expect(result.alreadyUpToDate)
        #expect(result.targetVersion == "v0.5.2")
    }

    @Test("run homebrew mode detects upgrade available")
    func runHomebrewUpgrade() async throws {
        let prefix = "/opt/homebrew/Cellar/xcodecli/0.5.3"
        let binaryPath = "\(prefix)/bin/xcodecli"

        var runner = MockProcessRunner()
        runner.results["/usr/local/bin/brew --prefix oozoofrog/tap/xcodecli"] =
            ProcessResult(stdout: prefix, stderr: "", exitCode: 0)
        runner.results["/usr/local/bin/brew upgrade oozoofrog/tap/xcodecli"] =
            ProcessResult(stdout: "Upgrading xcodecli", stderr: "", exitCode: 0)
        runner.results["\(binaryPath) version"] =
            ProcessResult(stdout: "xcodecli v0.5.3", stderr: "", exitCode: 0)

        let result = try await Updater.run(currentVersion: "v0.5.2", processRunner: runner)
        #expect(result.mode == "homebrew")
        #expect(!result.alreadyUpToDate)
        #expect(result.targetVersion == "v0.5.3")
        #expect(result.currentVersion == "v0.5.2")
    }

    @Test("run falls back to direct mode when brew prefix fails")
    func runDirectFallback() async throws {
        var runner = MockProcessRunner()
        runner.results["/usr/local/bin/brew --prefix oozoofrog/tap/xcodecli"] =
            ProcessResult(stdout: "", stderr: "not found", exitCode: 1)
        runner.results["/usr/bin/git ls-remote --refs --tags https://github.com/oozoofrog/xcodecli.git"] =
            ProcessResult(stdout: "abc\trefs/tags/v0.5.2", stderr: "", exitCode: 0)

        let result = try await Updater.run(currentVersion: "v0.5.2", processRunner: runner)
        #expect(result.mode == "direct")
        #expect(result.alreadyUpToDate)
        #expect(result.targetVersion == "v0.5.2")
    }

    @Test("run direct mode reports update available when newer tag exists")
    func runDirectUpdateAvailable() async throws {
        var runner = MockProcessRunner()
        runner.results["/usr/local/bin/brew --prefix oozoofrog/tap/xcodecli"] =
            ProcessResult(stdout: "", stderr: "not found", exitCode: 1)
        runner.results["/usr/bin/git ls-remote --refs --tags https://github.com/oozoofrog/xcodecli.git"] =
            ProcessResult(
                stdout: "abc\trefs/tags/v0.5.2\ndef\trefs/tags/v0.5.3",
                stderr: "",
                exitCode: 0
            )

        let result = try await Updater.run(currentVersion: "v0.5.2", processRunner: runner)
        #expect(result.mode == "direct")
        #expect(!result.alreadyUpToDate)
        #expect(result.targetVersion == "v0.5.3")
    }

    @Test("run direct mode throws when git ls-remote fails")
    func runDirectGitFailure() async {
        var runner = MockProcessRunner()
        runner.results["/usr/local/bin/brew --prefix oozoofrog/tap/xcodecli"] =
            ProcessResult(stdout: "", stderr: "not found", exitCode: 1)
        runner.results["/usr/bin/git ls-remote --refs --tags https://github.com/oozoofrog/xcodecli.git"] =
            ProcessResult(stdout: "", stderr: "network error", exitCode: 128)

        await #expect(throws: XcodeCLIError.self) {
            _ = try await Updater.run(currentVersion: "v0.5.2", processRunner: runner)
        }
    }

    @Test("run direct mode throws when no semantic version tags found")
    func runDirectNoTags() async {
        var runner = MockProcessRunner()
        runner.results["/usr/local/bin/brew --prefix oozoofrog/tap/xcodecli"] =
            ProcessResult(stdout: "", stderr: "not found", exitCode: 1)
        runner.results["/usr/bin/git ls-remote --refs --tags https://github.com/oozoofrog/xcodecli.git"] =
            ProcessResult(stdout: "abc\trefs/tags/not-semver", stderr: "", exitCode: 0)

        await #expect(throws: XcodeCLIError.self) {
            _ = try await Updater.run(currentVersion: "v0.5.2", processRunner: runner)
        }
    }

    @Test("run homebrew mode throws when brew upgrade fails")
    func runHomebrewUpgradeFails() async {
        var runner = MockProcessRunner()
        runner.results["/usr/local/bin/brew --prefix oozoofrog/tap/xcodecli"] =
            ProcessResult(stdout: "/opt/homebrew/Cellar/xcodecli/0.5.2", stderr: "", exitCode: 0)
        runner.results["/usr/local/bin/brew upgrade oozoofrog/tap/xcodecli"] =
            ProcessResult(stdout: "", stderr: "upgrade failed", exitCode: 1)

        await #expect(throws: XcodeCLIError.self) {
            _ = try await Updater.run(currentVersion: "v0.5.2", processRunner: runner)
        }
    }
}
