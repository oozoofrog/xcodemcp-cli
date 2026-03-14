# Changelog

All notable changes to this project will be documented in this file.

The format is inspired by [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses pre-1.0 semantic versioning tags.

## [Unreleased]

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
