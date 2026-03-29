# Changelog

All notable changes to this project will be documented in this file.

The format is inspired by [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [1.1.0] - 2026-03-29
### Added
- `doctor --json` now emits structured `recommendations` so automation can consume remediation guidance directly.
- `agent status --json` now includes `warnings` and `nextSteps` for stale LaunchAgent registration and other stability issues.
- `mcp config --strict-stable-path` to fail fast when the current executable path looks unstable for long-lived MCP registration.

### Changed
- `agent guide` and `agent demo` now surface relevant doctor recommendations inline in their human-readable environment sections.
- `mcp config` now warns on unstable executable paths (for example `.build`, direct `Cellar`, temporary, or external-volume paths) and suggests a stable installed path when available.
- `install.sh` now warns when the selected install directory looks unstable for long-lived MCP registration.

### Fixed
- `doctor`, `agent status`, `agent guide`, and `agent demo` now more consistently explain stale LaunchAgent registration caused by relative or mismatched binary paths.
- `update` now refuses obviously unstable executable paths (Swift build outputs, temporary paths, external-volume paths) before attempting in-place replacement.

## [1.0.0] - 2026-03-21
### Changed
- **Complete Go → Swift rewrite.** The entire CLI has been reimplemented as a Swift Package Manager project (swift-tools-version 6.0, macOS 15+). All Go source code is retained as reference but is no longer built or tested by CI.
- CI workflow replaced: Go CI (`go test`, `gofmt`) removed; Swift CI (`swift build`, `swift test`, release build verification) on macOS 15 runner.
- Homebrew formula now uses `swift build -c release` with `inreplace` version injection instead of `go build` with ldflags.
- `install.sh` requires Swift (Xcode) instead of Go.
- `scripts/build.sh` (Go) removed; `scripts/build-swift.sh` is the sole build script.

### Added
- **Agent RPC server** (`agent run`): Unix domain socket daemon with session pooling, idle timeout (default 24h), PID file, binary identity tracking, and graceful shutdown via SIGINT/SIGTERM.
- **Agent RPC client** with autostart: automatic LaunchAgent bootstrap/kickstart, binary mismatch detection and forced restart, `waitForReady` ping loop, and remaining timeout budget management.
- **MCP server enhancements**: `notifications/cancelled` handler suppresses in-flight responses; duplicate request ID detection returns `-32600`; `tools/list` and `tools/call` dispatched as async Tasks.
- **MCP config rewrite**: per-client invocation builders for Claude (`add-json` with JSON payload + remove/retry), Codex (`mcp add`), and Gemini (`mcp add`); shell quoting; executable path resolution.
- **Agent guide**: workflow catalog (build/test/read/search/edit/diagnose), keyword-based intent classification with confidence scoring, 5-tier fuzzy window matching (100/90/80/70/50), per-workflow fallbacks, 15 format command helpers.
- **Agent demo**: doctor → tools list → agent status → XcodeListWindows onboarding flow with structured JSON/text reports.
- **Tool-specific timeout policy**: read tools 60s, write tools 120s, build/test 30m, fallback 5m. All `tool call`, `tool inspect`, `tools list`, and `serve` commands route through AgentClient.
- **LaunchAgent permission diagnostics** (#29): `doctor` now checks `~/Library/LaunchAgents/` directory ownership and plist writability; `ensureLaunchAgentPlist` wraps permission-denied errors with actionable `sudo chown` guidance.
- **MCP client pagination**: cursor-based `tools/list` loop accumulates tools across pages.
- **Helper modules**: `SocketHelpers` (safe `sockaddr_un` path setter, partial-write loop), `BinaryIdentity` (SHA-256 executable hashing), `PlistHelper` (XML render/parse/ensure), `LaunchdHelper` (launchctl protocol), `TimeoutPolicy`.
- **201 tests** across 26 suites (up from 24), recovering ~96% of Go test coverage. Includes MCP server Pipe-based protocol tests, intent classification tests, session pooling integration tests (env-gated), and Swift-specific concurrency tests.
- Release skill (`.claude/skills/release.md`) for guided version bump and Homebrew deployment.

### Fixed
- `sockaddr_un.sun_path` now uses a safe `withUnsafeMutableBytes` helper instead of fragile `strncpy` pattern.
- Idle timeout unit conversion: server sends milliseconds on the wire (`RuntimeStatus.idleTimeoutMs`), client correctly converts to nanoseconds. Previously multiplied by 10^6 too much.
- `writeResponse` sends a fallback JSON error on encoding failure instead of silently dropping the response.
- Partial writes handled via `writeAllToFD` loop with EINTR retry.
- `getOrCreateClient` TOCTOU race fixed with double-check-under-lock pattern.
- `accept()` loop moved to dedicated Thread to avoid blocking Swift cooperative thread pool.
- `chmod` and `setsockopt` return values now checked.
- `readEnvelope` recursion replaced with loop to prevent stack overflow on many empty lines.
- `MCPClient.request()` loop checks `Task.isCancelled` for cooperative cancellation.
- `stop()` and `uninstall()` collect errors instead of silently swallowing with `try?`.
- `MCPResponseWriter` and `AgentServerConfig` use `@unchecked Sendable` for FileHandle fields.

## [0.5.4] - 2026-03-16
### Changed
- Cut a follow-up patch release to verify the Homebrew upgrade path exercised by `xcodecli update`.

## [0.5.3] - 2026-03-16
### Added
- `xcodecli update` for upgrading an installed `xcodecli` binary in place, using Homebrew when appropriate and otherwise rebuilding the latest GitHub release over the current executable.

## [0.5.2] - 2026-03-16
### Fixed
- Rebuilt release tags to remove personal identity and AI co-author metadata from git history.

## [0.5.0] - 2026-03-16
### Added
- `xcodecli serve`, a stdio MCP server mode that lets MCP clients talk to `xcodecli` directly while reusing the LaunchAgent-backed pooled `mcpbridge` runtime.

### Changed
- `xcodecli mcp config` and the `mcp <client>` aliases now target `xcodecli serve` by default, with `--mode bridge` available for raw passthrough compatibility.

### Fixed
- `serve` now negotiates supported MCP protocol versions instead of always forcing the newest version.
- `serve` and the agent runtime now propagate request cancellation more cleanly so cancelled long-running MCP calls stop without leaking stale work.

## [0.4.1] - 2026-03-15
### Added
- Hierarchical collaboration context docs (`CLAUDE.md` plus local `CONTEXT.md` files) for the CLI surface, internal packages, docs, and scripts.

### Changed
- Tool request timeout defaults are now grouped more explicitly by tool category, including `DocumentationSearch` in the 60-second read/search bucket.
- Agent and quickstart guidance now more clearly distinguish request timeouts from `mcpbridge` session idle timeout behavior.

### Fixed
- LaunchAgent-backed tool requests now retire stale `mcpbridge` sessions during handoff without blocking the next request.
- Same-path rebuilds now detect stale LaunchAgent binaries and restart the daemon before serving new tool requests.
- Cold-start and startup timeout errors now report the remaining request budget instead of the original configured timeout.

## [0.4.0] - 2026-03-15
### Added
- `xcodecli mcp config` for generating or writing MCP registration commands for Claude Code, Codex, and Gemini using `xcodecli bridge`.
- Shorthand aliases `xcodecli mcp codex`, `xcodecli mcp claude`, and `xcodecli mcp gemini`.

### Changed
- README and CLI help now present the shorter `mcp <client>` aliases as the primary user-facing MCP setup flow while retaining `mcp config --client ...` as the advanced/canonical form.

## [0.3.2] - 2026-03-15
### Changed
- `xcodecli` and `xcodecli help` now show the current version at the top of their output.
- Development builds now render the version as `<version> (dev)`, while release builds continue to show the plain tagged version.

## [0.3.1] - 2026-03-15
### Added
- `xcodecli version` and `xcodecli --version` for printing the current CLI version.

### Changed
- Build and release flows now inject tagged versions into release binaries while local builds continue to default to `dev`.

## [0.3.0] - 2026-03-14
### Added
- `agent guide` subcommand for read-only workflow tutoring that maps a request to the recommended xcodecli tool sequence and prints exact next commands.
- `agent demo` subcommand for a safe read-only onboarding flow that runs `doctor`, lists live MCP tools, calls `XcodeListWindows`, and prints suggested next commands.
- `scripts/install.sh` for installing `xcodecli` from a local checkout or directly from GitHub source refs.

### Changed
- Improved first-run onboarding docs and root CLI help with a guide-first path for humans and agents, while keeping `agent demo` as the safe live discovery step.
- Moved installation guidance near the top of the README and documented both direct GitHub installs and Homebrew installs.
- `scripts/install.sh` now verifies PATH reachability for the user's login shell and prints shell-specific next steps when `xcodecli` is not discoverable on PATH.
- Homebrew release automation now publishes `oozoofrog/tap/xcodecli`.

## [0.2.1] - 2026-03-14
### Added
- Homebrew distribution and release automation assets for the shared `oozoofrog/homebrew-tap`.
- Manual/automated Homebrew formula publishing workflow documentation.

## [0.2.0] - 2026-03-14
### Added
- MCP convenience commands: `tools list`, `tool inspect`, and `tool call`.
- LaunchAgent-backed runtime for long-lived `mcpbridge` sessions used by tools commands.
- Persistent `MCP_XCODE_SESSION_ID` reuse across runs.
- First-time agent onboarding docs in `AGENTS.md` and `docs/agent-quickstart.md`.
- JSON output modes for `doctor` and `agent status`.
- `tool call` payload input via inline JSON, `@file`, and `--json-stdin`.
- `scripts/build.sh` for repeatable local builds.

### Changed
- `xcodecli` with no arguments now prints help instead of defaulting to raw bridge execution.
- CLI help now includes richer guidance for both humans and agents.
- Tool command startup and LaunchAgent autostart now honor request timeouts.
- Agent-only commands no longer create a persistent session file as a side effect.

### Fixed
- CI test stability for doctor command coverage.
- MCP startup timeout handling during initialization and agent-backed reuse.

## [0.1.1] - 2026-03-13
### Changed
- Updated GitHub Actions workflow to newer `actions/checkout` and `actions/setup-go` versions.
- Adjusted Go cache configuration to avoid cache warnings when `go.sum` is absent.

## [0.1.0] - 2026-03-13
### Added
- Initial Go-based `xcodecli` CLI scaffold.
- Raw `bridge` mode for passthrough execution of `xcrun mcpbridge`.
- `doctor` command for local environment diagnostics.
- Project metadata, LICENSE, CI, and collaboration templates.
