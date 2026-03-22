# CONTEXT.md

## Scope
- This directory holds the reusable implementation layers behind the CLI.
- The boundary here is package-oriented rather than command-oriented.

## Why This Exists
- `cmd/xcodecli` exposes one CLI surface, but the actual runtime concerns are split across distinct internal packages.
- Changes that look local often cross package boundaries: session handling affects agent runtime behavior, bridge env resolution affects doctor output, and MCP client behavior affects both bridge and agent flows.

## Key Files
- `./agent/client.go`: LaunchAgent client, autostart, and socket RPC entrypoints.
- `./agent/server.go`: long-lived runtime, session pooling, and request dispatch.
- `./bridge/session.go`: persistent session ID resolution and storage.
- `./mcp/client.go`: direct MCP client transport over `xcrun mcpbridge`.

## Package Responsibilities
- `agent`
  - LaunchAgent-backed local runtime
  - socket RPC client/server behavior
  - agent status / stop / uninstall / autostart lifecycle
- `bridge`
  - raw environment option handling
  - persistent session ID creation/reuse
  - direct `xcrun mcpbridge` process execution
- `doctor`
  - environment inspection and human/JSON diagnostics
- `mcp`
  - MCP protocol client behavior and persistent client reuse for direct bridge communication

## Local Rules
- Keep package boundaries explicit:
  - `bridge` should stay focused on raw bridge/env/process concerns
  - `agent` should own the long-lived runtime and LaunchAgent lifecycle
  - `doctor` should inspect and report, not mutate runtime state
  - `mcp` should remain transport/protocol-focused
- When moving logic between these packages, document why the boundary is changing instead of silently expanding one package’s scope.
- Do not introduce CLI wording or doc-string policy here; that belongs in `cmd/xcodecli`.

## Change Coupling
- Session behavior changes usually require reviewing:
  - `./bridge/session.go`
  - `./agent/client.go`
  - `./agent/server.go`
  - CLI-facing messages in `../cmd/xcodecli/`
- LaunchAgent/runtime changes usually require reviewing:
  - `./agent/client.go`
  - `./agent/server.go`
  - `./agent/plist.go`
  - status/reporting paths in `../cmd/xcodecli/`
- MCP transport changes usually require reviewing:
  - `./mcp/client.go`
  - `./mcp/server.go`
  - `./agent/client.go`
  - `tool inspect` / `tool call` behavior in `../cmd/xcodecli/`

## Verification Notes
- Package-level tests matter here. Keep `go test ./...` green after any boundary shift.
- If behavior spans packages, do not rely on a single package test; read the adjacent tests as part of the change review.

## Child Contexts
- None yet. `internal/agent` is the first likely split candidate if LaunchAgent/runtime logic grows beyond this document’s scope.
