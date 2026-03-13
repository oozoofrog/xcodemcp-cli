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

## Git workflow

- `main`: stable baseline branch
- `codex/*`: implementation branches for agent-driven changes
- Open pull requests from `codex/*` into `main`

## Notes

- `--xcode-pid` overrides `MCP_XCODE_PID`.
- `--session-id` overrides `MCP_XCODE_SESSION_ID`.
- In bridge mode, **stdout is protocol-only**. Wrapper logs and diagnostics go to stderr.
- v1 only covers raw bridge passthrough and `doctor` diagnostics.
