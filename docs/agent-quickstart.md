# xcodemcp agent quickstart

This guide is for a first-time agent or automation that needs to discover and call Xcode MCP tools through `xcodemcp`.

## 1. Build the CLI

```bash
cd /Volumes/eyedisk/develop/oozoofrog/xcodemcp-cli
./scripts/build.sh
```

## 2. Check the environment

```bash
./xcodemcp doctor --json
./xcodemcp agent status --json
```

Look for:
- `success: true` from `doctor`
- a running Xcode PID or at least an open Xcode process
- LaunchAgent socket reachability after the first tools command

## 3. Discover available tools

```bash
./xcodemcp tools list --json
```

If this is the first `tools` request, `xcodemcp` may install and bootstrap a per-user LaunchAgent automatically.

## 4. Inspect one tool

```bash
./xcodemcp tool inspect XcodeListWindows
./xcodemcp tool inspect XcodeListWindows --json
```

Use `tool inspect` to confirm the tool name, description, and `inputSchema` before calling it.

## 5. Call a tool

Inline JSON:

```bash
./xcodemcp tool call XcodeListWindows --json '{}'
```

Read the payload from a file:

```bash
cat > /tmp/payload.json <<'JSON'
{"scheme":"Demo"}
JSON
./xcodemcp tool call BuildProject --json @/tmp/payload.json
```

Read the payload from stdin:

```bash
printf '{}' | ./xcodemcp tool call XcodeListWindows --json-stdin
```

## 6. Troubleshooting

### The tool call times out
- Verify Xcode is open and a workspace/project window is visible.
- Retry with `--debug`.
- Re-run `./xcodemcp doctor --json`.

### The LaunchAgent looks stale
```bash
./xcodemcp agent status --json
./xcodemcp agent stop
./xcodemcp agent uninstall
```

Then retry `tools list` or `tool inspect`.

### The payload is large or reused often
Prefer `--json @file` over a huge inline string.
