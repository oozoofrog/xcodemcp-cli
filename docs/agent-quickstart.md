# xcodecli agent quickstart

This guide is for a first-time agent or automation that needs to discover and call Xcode MCP tools through `xcodecli`.

## 1. Build the CLI

```bash
cd /path/to/xcodecli
./scripts/build-swift.sh .tmp/xcodecli
```

## 2. Start with the user's intent

If you already know the request, start with `agent guide`:

```bash
./xcodecli agent guide "build Unicody"
./xcodecli agent guide "run tests for Unicody"
./xcodecli agent guide "read KeyboardState.swift"
./xcodecli agent guide "search for AdManager"
./xcodecli agent guide "update KeyboardState.swift"
./xcodecli agent guide "diagnose build errors"
./xcodecli agent guide --json
```

What this does:
- classifies the request into a workflow family (`build`, `test`, `read`, `search`, `edit`, `diagnose`)
- explains the recommended tool order and why that order is correct
- prints exact next commands, often with a real `tabIdentifier` if a live Xcode window match is obvious
- stays read-only while it learns the current environment

## 3. Fastest safe live demo

```bash
./xcodecli agent demo
./xcodecli agent demo --json
```

What this does:
- runs `doctor`
- discovers the live MCP tool catalog
- safely calls `XcodeListWindows`
- prints the next commands for `XcodeLS` and `XcodeRead`

`agent demo` is read-only. It does **not** build, test, write, update, move, or remove project files.

## 4. Check the environment

```bash
./xcodecli doctor --json
./xcodecli agent status --json
```

Look for:
- `success: true` from `doctor`
- a running Xcode PID or at least an open Xcode process
- LaunchAgent socket reachability after the first tools command

## 5. Discover available tools

```bash
./xcodecli tools list
./xcodecli tools list --json --timeout 60s
```

If this is the first `tools` request, `xcodecli` may install and bootstrap a per-user LaunchAgent automatically.

## 6. Inspect one tool

```bash
./xcodecli tool inspect XcodeListWindows
./xcodecli tool inspect XcodeListWindows --json --timeout 60s
```

Use `tool inspect` when you need schema reassurance. For the common workflows above, `agent guide` should usually tell you what to call without making this the first step.

## 7. Call a tool

Inline JSON:

```bash
./xcodecli tool call XcodeListWindows --json '{}'
```

Read the payload from a file:

```bash
cat > /tmp/payload.json <<'JSON'
{"scheme":"Demo"}
JSON
./xcodecli tool call BuildProject --timeout 30m --json @/tmp/payload.json
```

Read the payload from stdin:

```bash
printf '{}' | ./xcodecli tool call XcodeListWindows --json-stdin
```

## 8. End-to-end read-only example

Use the `tabIdentifier` returned by `XcodeListWindows` to continue the flow:

```bash
./xcodecli tool call XcodeListWindows --json '{}'
./xcodecli tool call XcodeLS --json '{"tabIdentifier":"<tabIdentifier from above>","path":""}'
./xcodecli tool call XcodeRead --json '{"tabIdentifier":"<tabIdentifier from above>","filePath":"<path from XcodeLS>"}'
```

If you want the next mutating step after discovery, that is where you would choose something like `BuildProject` or `RunAllTests`.

## Request timeout vs mcpbridge session idle timeout

- `--timeout` is the **request timeout** for a single `tools`, `tool`, `agent guide`, or `agent demo` command.
- The request timeout includes first-use LaunchAgent startup, `mcpbridge` session initialization, and any auth prompts.
- The default **mcpbridge session idle timeout** is `24h`, which controls how long pooled `mcpbridge` sessions stay alive while idle.
- Active requests are **not** interrupted by the `mcpbridge session idle timeout`.

## 9. Troubleshooting

### The tool call times out
- This is the **request timeout**, not the `mcpbridge session idle timeout`.
- Verify Xcode is open and a workspace/project window is visible.
- Retry with a larger `--timeout` (for example `--timeout 30m` for `BuildProject` / `RunAllTests`).
- Retry with `--debug`.
- Re-run `./xcodecli doctor --json`.

### The LaunchAgent looks stale
```bash
./xcodecli agent status --json
./xcodecli agent stop
./xcodecli agent uninstall
```

Then retry `tools list` or `tool inspect`.

### The payload is large or reused often
Prefer `--json @file` over a huge inline string.

## 10. What comes after guide/demo

- `agent guide` solves the learning-curve problem first.
- `agent demo` solves safe live discovery.
- A future higher-level task command may reduce repetitive execution steps further, but it is intentionally **not** part of the current CLI surface yet.
