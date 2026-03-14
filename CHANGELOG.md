# Changelog

All notable changes to this project will be documented in this file.

The format is inspired by [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses pre-1.0 semantic versioning tags.

## [Unreleased]

## [0.2.1] - 2026-03-14
### Added
- Homebrew distribution and release automation assets for the shared `oozoofrog/homebrew-tap`.
- Manual/automated Homebrew formula publishing workflow documentation.

## [0.2.0] - 2026-03-14
### Added
- MCP convenience commands: `tools list`, `tool inspect`, and `tool call`.
- LaunchAgent-backed runtime for long-lived `mcpbridge` sessions used by tools commands.
- Persistent `MCP_XCODE_SESSION_ID` reuse across runs.
- First-time agent onboarding docs in `/Volumes/eyedisk/develop/oozoofrog/xcodemcp-cli/AGENTS.md` and `/Volumes/eyedisk/develop/oozoofrog/xcodemcp-cli/docs/agent-quickstart.md`.
- JSON output modes for `doctor` and `agent status`.
- `tool call` payload input via inline JSON, `@file`, and `--json-stdin`.
- `scripts/build.sh` for repeatable local builds.

### Changed
- `xcodemcp` with no arguments now prints help instead of defaulting to raw bridge execution.
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
- Initial Go-based `xcodemcp` CLI scaffold.
- Raw `bridge` mode for passthrough execution of `xcrun mcpbridge`.
- `doctor` command for local environment diagnostics.
- Project metadata, LICENSE, CI, and collaboration templates.
