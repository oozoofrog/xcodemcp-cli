# CONTEXT.md

## Scope
- This directory defines the public CLI surface for `xcodecli`.
- It owns command parsing, help text, human-readable output, JSON output, and the agent-oriented workflow layer on top of internal packages.

## Why This Exists
- Most user-visible behavior is concentrated here, but the code is split across large files (`cli.go`, `main.go`, `agent_guide.go`, `agent_demo.go`, `mcp_config.go`).
- Small semantic changes here often require synchronized updates across help text, README examples, and command tests.

## Key Files
- `cli.go`: subcommand tree, flag parsing, help text, aliases, and user-facing usage strings.
- `main.go`: command dispatch, output routing, timeout wiring, and integration with internal packages.
- `agent_guide.go`: read-only workflow tutoring and next-command generation.
- `agent_demo.go`: safe onboarding/demo flow for first-time Xcode MCP discovery.
- `mcp_config.go`: MCP client registration output and optional write-through to client CLIs.
- `*_test.go`: command parsing/output regressions; these are part of the CLI contract.

## Local Rules
- Treat help text as part of the public API. If command semantics change, update help strings in `cli.go`.
- Keep canonical vs shorthand command relationships explicit:
  - `mcp config --client ...` is the canonical form.
  - `mcp codex|claude|gemini` are convenience aliases.
- Preserve the text/JSON output contract:
  - human-readable mode should stay stable and copy/paste-friendly
  - `--json` should remain machine-readable and deterministic
- `agent guide` and `agent demo` must stay non-mutating.
- `tool call` continues to require a JSON object payload, not arbitrary JSON values.

## Change Coupling
- If you change CLI behavior, review all of the following together:
  - command parsing and usage text
  - `../../README.md` command examples
  - `../../docs/agent-quickstart.md` examples
  - affected tests in this directory
- If you change MCP registration behavior, also check:
  - path-stability messaging
  - client-specific help text
  - `../../README.md` registration examples

## Verification Notes
- Run `go test ./...` for any CLI surface change.
- For behavior that affects built binaries or help text, also run:
  - `../../scripts/build.sh .tmp/xcodecli`
  - `../../.tmp/xcodecli version`
- For release-facing CLI examples, confirm documentation still matches the current version string.

## Child Contexts
- None yet. If `agent_*` or `mcp_config.go` grows substantially, split into deeper local contexts rather than expanding this file indefinitely.
