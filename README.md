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
- In bridge mode, **stdout is protocol-only**. Wrapper logs and diagnostics go to stderr.
- Convenience commands (`tools list`, `tool call`) use newline-delimited JSON MCP transport with a default `30s` timeout.
- `tool call --json` currently accepts only an inline JSON object string.
