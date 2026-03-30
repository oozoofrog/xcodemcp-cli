# Authorization and session troubleshooting

This page explains how `xcodecli` reuses Xcode MCP authorization, what "same session" means, and what to check when Xcode prompts for authorization again.

## At a glance

- Repeated Xcode authorization is usually minimized only while you keep using the same pooled session key: `{XcodePID, SessionID, DeveloperDir}`.
- The safest normal path is: one stable installed `xcodecli` binary, default agent mode, default persistent session ID, and no unnecessary `MCP_XCODE_PID` / `DEVELOPER_DIR` overrides.
- If prompts start recurring, first check `doctor --json` and `agent status --json` before resetting the agent runtime.

## Quick FAQ

### Does one Xcode authorization unlock `mcpbridge` forever on this machine?

No. In practice, authorization reuse is only **best-effort within one pooled session**.

`xcodecli` reuses backend `mcpbridge` processes by a pooled session key:

```text
{ XcodePID, SessionID, DeveloperDir }
```

If that key changes, the next request may land on a fresh backend session and Xcode may show a fresh authorization prompt.

### What counts as "the same session"?

Not a terminal tab, not a shell process, and not "everything on the machine forever".

Two calls are typically on the same pooled session only when all of these stay the same:
- the same Xcode process (`XcodePID`)
- the same session ID (`SessionID`)
- the same toolchain / developer directory (`DeveloperDir`)

Different shells can still be on the same pooled session if those three values do not change.

### What is the safest setup if I want fewer repeated prompts?

Use all of the following:
1. one stable installed `xcodecli` path
2. default agent mode (`xcodecli serve` via `mcp config`)
3. the default persistent session ID from `~/Library/Application Support/xcodecli/session-id`
4. no explicit `MCP_XCODE_PID` or `DEVELOPER_DIR` unless you intentionally need them

### What usually causes a fresh authorization prompt?

Common causes:
- switching between different `xcodecli` binaries or checkout paths
- passing a different `--session-id`
- changing `MCP_XCODE_PID`
- changing `DEVELOPER_DIR`
- running `agent stop`
- running `agent uninstall`
- restarting Xcode so the PID changes

### Can I use different `--session-id` values without re-authorization?

Sometimes you might still get lucky if Xcode already trusts the new backend quickly, but **you should treat a different `--session-id` as a new backend session**. Do not assume prompt-free reuse across different session IDs.

### Does a new terminal window create a new authorization session?

Not by itself.

If the new terminal still uses the same:
- installed `xcodecli` path
- persistent default session ID
- Xcode instance
- `DEVELOPER_DIR`

then it can still reuse the same pooled backend session.

## Recommended operating rules

### Normal day-to-day usage

- Keep using one installed `xcodecli` path, such as `/opt/homebrew/bin/xcodecli` or `~/.local/bin/xcodecli`
- Prefer `mcp config` default agent mode over raw bridge mode
- Let `xcodecli` reuse its default persistent session ID
- Avoid changing `MCP_XCODE_PID` or `DEVELOPER_DIR`
- Avoid `agent stop` / `agent uninstall` unless you are troubleshooting

### When you intentionally want isolation

Use a different `--session-id`, different `MCP_XCODE_PID`, or different `DEVELOPER_DIR` only when you explicitly want a separate backend session and accept that Xcode may ask again.

## Troubleshooting repeated prompts

### 1. Check whether you are still on a stable installed binary

```bash
./xcodecli doctor --json
./xcodecli agent status --json
```

Look for warnings about:
- `LaunchAgent binary registration`
- `effective MCP_XCODE_PID`
- `effective DEVELOPER_DIR`

If the registered binary path differs from the current binary path, the LaunchAgent backend may recycle and Xcode may ask again.

### 2. Check whether you changed the pooled session key

Ask:
- Did Xcode restart?
- Did I pass `--session-id`?
- Did I export `MCP_XCODE_PID`?
- Did I export `DEVELOPER_DIR`?
- Did I switch from one `xcodecli` binary to another?

If yes, treat the next request as a possible fresh authorization event.

### 3. Reuse the default persistent session ID

Default location:

```text
~/Library/Application Support/xcodecli/session-id
```

Avoid overriding it unless you intentionally need isolation.

### 4. Avoid unnecessary agent resets

These commands are useful for recovery, but they also discard the warm backend session:

```bash
./xcodecli agent stop
./xcodecli agent uninstall
```

Do not use them as part of normal daily operation.

### 5. Re-register from a stable path if needed

If `mcp config` warns that the current executable path looks unstable, install or copy `xcodecli` to a stable location first, then regenerate your MCP registration.

## Minimal verification checklist

If you want to test whether authorization reuse is likely to work:

1. call a read-only tool once
2. call another read-only tool from a new shell
3. keep the same default session ID
4. do not change `MCP_XCODE_PID`, `DEVELOPER_DIR`, or binary path

Representative commands:

```bash
./xcodecli tool call XcodeListWindows --json '{}'
./xcodecli tool call XcodeRead --json '{"tabIdentifier":"windowtab1","filePath":"Project/App.swift","limit":5}'
```

Then compare with a deliberate new-session call:

```bash
./xcodecli tool call XcodeListWindows --session-id "$(uuidgen | tr '[:upper:]' '[:lower:]')" --json '{}'
```

That last command should be treated as a fresh backend session test.

## Verification scenario table

| Scenario | Example change | Expected pooled-session result | Prompt risk |
| --- | --- | --- | --- |
| Same default session in a new shell | Same installed binary, same Xcode, same default session ID, same `DEVELOPER_DIR` | Usually stays on the same warm backend session | Low |
| Different `--session-id` | Pass a new UUID on the command line | Fresh backend session | Medium to high |
| Different `MCP_XCODE_PID` | Target a different Xcode PID | Fresh backend session | Medium to high |
| Different `DEVELOPER_DIR` | Override the selected toolchain | Fresh backend session | Medium to high |
| Different binary path | Switch from one checkout/install path to another | LaunchAgent/backend recycle likely | Medium to high |
| `agent stop` or `agent uninstall` before the next call | Reset the warm runtime | Fresh backend session on the next request | Medium to high |

Use this table as an operational expectation guide, not as a cryptographic guarantee. Xcode authorization behavior is ultimately owned by the platform and may still vary based on current machine state.
