# xcodecli agent quickstart

This guide is for a first-time agent or automation that needs to discover and call Xcode MCP tools through `xcodecli`.

If you specifically need guidance on repeated Xcode authorization prompts, pooled session reuse, or recovery steps, also read:
- English: `docs/authorization-troubleshooting.md`
- Korean: `docs/authorization-troubleshooting.kr.md`

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
- any warnings about `LaunchAgent binary registration`, `effective MCP_XCODE_PID`, or `effective DEVELOPER_DIR`

To minimize repeated Xcode authorization prompts across sessions:
- prefer `xcodecli serve` / `mcp config` default agent mode
- keep using the **same installed xcodecli path** instead of alternating between a checkout binary and a Homebrew or copied binary
- avoid changing `MCP_XCODE_PID` or `DEVELOPER_DIR` unless you intentionally want a different pooled session
- if `mcp config` warns that the current executable path looks unstable, re-run it from a stable installed path before registering the server with your MCP client
- if you want policy enforcement instead of advisory output, add `--strict-stable-path` to `mcp config`

What counts as "the same session" for authorization reuse:
- `xcodecli` reuses backend `mcpbridge` state by a pooled session key: `{XcodePID, SessionID, DeveloperDir}`.
- Different terminal windows or new CLI processes can still stay on the **same** pooled session if those three values remain unchanged.
- By default, `xcodecli` reuses the persistent session ID stored at `~/Library/Application Support/xcodecli/session-id`, which is the safest choice if you want repeated calls from different shells to stay on one pooled session.
- Passing a different `--session-id`, changing `MCP_XCODE_PID`, changing `DEVELOPER_DIR`, switching binaries, or forcing `agent stop` / `agent uninstall` can move the next request onto a fresh backend session and may surface a fresh Xcode authorization prompt.

Practical operating rules:
1. Register one stable installed `xcodecli` path and keep using that exact path.
2. Prefer default agent mode over raw bridge mode for long-lived MCP usage.
3. Reuse the default persistent session ID; only pass `--session-id` when you intentionally want isolation.
4. Do not set `MCP_XCODE_PID` or `DEVELOPER_DIR` unless you are intentionally targeting a different Xcode instance/toolchain.
5. Avoid `agent stop` / `agent uninstall` during normal use; reserve them for recovery and troubleshooting.
6. Treat authorization reuse as valid **within one pooled session**, not as a global one-time grant across every possible session configuration.

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

If `doctor` reports that the registered LaunchAgent binary path is relative, missing, or mismatched, treat that as a stale registration. Re-run the current binary once after `agent uninstall`, then keep using that same binary path for future MCP registration.
`agent status` now surfaces the same stale-registration hints in text mode, so use it as the fastest first triage command before escalating to `doctor`.
Likewise, run `xcodecli update` only from a stable installed path. The command now rejects obvious Swift build outputs, temporary paths, and external-volume paths instead of trying to replace them in place.
For automation, prefer `doctor --json`: it now includes a structured `recommendations` array in addition to raw checks.
For interactive use, `agent guide` and `agent demo` now surface those recommendations inline when the environment already suggests a likely next fix.

### The payload is large or reused often
Prefer `--json @file` over a huge inline string.

## 10. What comes after guide/demo

- `agent guide` solves the learning-curve problem first.
- `agent demo` solves safe live discovery.
- A future higher-level task command may reduce repetitive execution steps further, but it is intentionally **not** part of the current CLI surface yet.
