# CLAUDE.md

## Repository Commands
- Swift package build: `swift build`
- Swift package tests: `swift test`
- Release build script: `./scripts/build-swift.sh`
- Go implementation tests: `go test ./...` when touching `./cmd/xcodecli/` or `./internal/`
- Agent onboarding:
  - `./xcodecli agent guide "<intent>"`
  - `./xcodecli agent demo --json`
  - `./xcodecli doctor --json`
- MCP client registration:
  - `./xcodecli mcp codex`
  - `./xcodecli mcp claude`
  - `./xcodecli mcp gemini`
- Release basics:
  - follow `./docs/releasing.md`
  - use `./scripts/release_homebrew.sh <tag> --dry-run` before push/publish paths

## Global Architecture Decisions
- `xcodecli` is a macOS CLI wrapper around `xcrun mcpbridge`.
- The repository currently contains two code trees:
  - the Swift package in `./Package.swift` and `./Sources/`
  - the Go CLI/runtime tree in `./cmd/xcodecli/` and `./internal/`
- The operator-friendly workflows (`doctor`, `mcp`, `tools`, `tool`, `agent`) build on top of the raw bridge/runtime layer across these implementations.

## Environment Constraints
- macOS only.
- Xcode must be running before using bridge-backed commands.
- At least one Xcode project/workspace window should be open before using `tools` or mutating Xcode MCP tools.
- In bridge mode, stdout is protocol-only. Human-readable logs belong on stderr.
- For release and Homebrew work, destructive steps come last and dry-run/check steps come first.

## Context Tree Entry Points
- `./AGENTS.md`: portable collaboration and triage rules.
- `./cmd/xcodecli/CONTEXT.md`: CLI parsing, help/output contracts, and command-level change coupling.
- `./internal/CONTEXT.md`: package boundaries for `agent`, `bridge`, `doctor`, `mcp`, and `update`.
- `./scripts/CONTEXT.md`: build/install/release script responsibilities and safety rules.
- `./docs/CONTEXT.md`: canonical long-form user documentation and example-sync rules.

## Context Loading Order
1. Read this file for global constraints and command anchors.
2. Read `./AGENTS.md` for collaboration rules.
3. Read the closest `CONTEXT.md` to the files you are changing.
4. Read canonical user docs (`./README.md`, `./docs/*.md`) only when you need detailed examples or release/install procedures.

## Override Rules
- The closest `CONTEXT.md` is more specific than this file.
- Subsystem documents may refine workflow details, but they should not override:
  - macOS/Xcode constraints
  - bridge stdout safety
  - destructive steps last
  - release/Homebrew dry-run-first policy

## Documentation Rules
- Keep global rules here short and stable.
- Do not duplicate long `./README.md` sections here; link downward instead.
- If a repeated mistake reveals a missing rule, update the nearest relevant context document rather than only fixing code.
