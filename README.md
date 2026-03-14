# xcodemcp

`xcodemcp` is a small Go wrapper around `xcrun mcpbridge` for local macOS use.

## Build

```bash
go build ./cmd/xcodemcp
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
MCP_XCODE_PID=12345 ./xcodemcp doctor
```

List tools through the MCP bridge:

```bash
./xcodemcp tools list
./xcodemcp tools list --json --timeout 30s
```

Inspect the LaunchAgent used by `tools` commands:

```bash
./xcodemcp agent status
./xcodemcp agent stop
./xcodemcp agent uninstall
```

Call a single tool with JSON arguments:

```bash
./xcodemcp tool call build_sim --json '{"scheme":"Demo"}'
./xcodemcp tool call launch_app_sim --json '{"args":[]}' --timeout 10s
```

## Git workflow

- `main`: stable baseline branch
- `codex/*`: implementation branches for agent-driven changes
- Open pull requests from `codex/*` into `main`

## Notes

- `--xcode-pid` overrides `MCP_XCODE_PID`.
- `--session-id` overrides `MCP_XCODE_SESSION_ID`.
- If no `--session-id` flag or `MCP_XCODE_SESSION_ID` environment variable is provided, `xcodemcp` automatically creates and reuses a persistent session ID at `~/Library/Application Support/xcodemcp/session-id`.
- In bridge mode, **stdout is protocol-only**. Wrapper logs and diagnostics go to stderr.
- Convenience commands (`tools list`, `tool call`) automatically install and bootstrap a per-user LaunchAgent at `~/Library/LaunchAgents/io.oozoofrog.xcodemcp.plist`.
- The LaunchAgent talks to `xcrun mcpbridge` over a long-lived local Unix socket and shuts itself down after `10m` of idleness by default.
- Convenience commands use newline-delimited JSON transport with a default `30s` timeout.
- `tool call --json` currently accepts only an inline JSON object string.
