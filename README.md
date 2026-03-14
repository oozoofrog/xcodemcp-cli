# xcodemcp

`xcodemcp` is a small Go wrapper around `xcrun mcpbridge` for local macOS use.

## Build

```bash
./scripts/build.sh
./scripts/build.sh .tmp/xcodemcp
```

You can also override the package or output path:

```bash
OUTPUT=.tmp/xcodemcp ./scripts/build.sh
PACKAGE=./cmd/xcodemcp ./scripts/build.sh
```

## Usage

Raw bridge mode is the default. It forwards stdin/stdout/stderr directly to `xcrun mcpbridge`.

```bash
./xcodemcp --xcode-pid 12345
./xcodemcp bridge --session-id 11111111-1111-1111-1111-111111111111
```

Run environment diagnostics:

```bash
./xcodemcp doctor
./xcodemcp doctor --json
MCP_XCODE_PID=12345 ./xcodemcp doctor --json
```

List tools through the MCP bridge:

```bash
./xcodemcp tools list
./xcodemcp tools list --json --timeout 30s
```

Inspect a single tool before calling it:

```bash
./xcodemcp tool inspect XcodeListWindows
./xcodemcp tool inspect XcodeListWindows --json
```

Call a single tool with JSON arguments:

```bash
./xcodemcp tool call XcodeListWindows --json '{}'
./xcodemcp tool call BuildProject --json @/tmp/payload.json
printf '{}' | ./xcodemcp tool call XcodeListWindows --json-stdin
```

Inspect the LaunchAgent used by `tools` commands:

```bash
./xcodemcp agent status
./xcodemcp agent status --json
./xcodemcp agent stop
./xcodemcp agent uninstall
```

## Agent onboarding

- Quick rules for first-time agents: `/Volumes/eyedisk/develop/oozoofrog/xcodemcp-cli/AGENTS.md`
- Detailed walkthrough: `/Volumes/eyedisk/develop/oozoofrog/xcodemcp-cli/docs/agent-quickstart.md`

## Git workflow

- `main`: stable baseline branch
- `codex/*`: implementation branches for agent-driven changes
- Open pull requests from `codex/*` into `main`

## Notes

- `--xcode-pid` overrides `MCP_XCODE_PID`.
- `--session-id` overrides `MCP_XCODE_SESSION_ID`.
- If no `--session-id` flag or `MCP_XCODE_SESSION_ID` environment variable is provided, `xcodemcp` automatically creates and reuses a persistent session ID at `~/Library/Application Support/xcodemcp/session-id`.
- In bridge mode, **stdout is protocol-only**. Wrapper logs and diagnostics go to stderr.
- Convenience commands (`tools list`, `tool inspect`, `tool call`) automatically install and bootstrap a per-user LaunchAgent at `~/Library/LaunchAgents/io.oozoofrog.xcodemcp.plist`.
- The LaunchAgent talks to `xcrun mcpbridge` over a long-lived local Unix socket and shuts itself down after `10m` of idleness by default.
- `tool call` accepts exactly one payload source: inline `--json`, `--json @file`, or `--json-stdin`.
