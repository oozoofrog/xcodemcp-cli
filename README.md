# xcodecli

`xcodecli` is a macOS CLI wrapper around `xcrun mcpbridge` for local use.

## Install

### Homebrew

Install from the shared `oozoofrog/tap` formula:

```bash
brew tap oozoofrog/tap
brew install oozoofrog/tap/xcodecli
```

Upgrade later with:

```bash
brew update
brew upgrade oozoofrog/tap/xcodecli
```

### Direct install from GitHub

Install the current `main` branch directly from GitHub:

```bash
curl -fsSL https://raw.githubusercontent.com/oozoofrog/xcodecli/main/scripts/install.sh | bash
```

Install a specific tag or branch:

```bash
curl -fsSL https://raw.githubusercontent.com/oozoofrog/xcodecli/main/scripts/install.sh | bash -s -- --ref v1.0.0
curl -fsSL https://raw.githubusercontent.com/oozoofrog/xcodecli/main/scripts/install.sh | bash -s -- --ref main
```

Install into a custom directory:

```bash
curl -fsSL https://raw.githubusercontent.com/oozoofrog/xcodecli/main/scripts/install.sh | bash -s -- --bin-dir "$HOME/.local/bin"
```

### Install from a local checkout

Build and install from the checked-out repository:

```bash
./scripts/install.sh
./scripts/install.sh --bin-dir "$HOME/.local/bin"
```

The install script:
- builds from the current checkout when run locally
- downloads and builds the requested GitHub ref when run via `curl | bash`
- installs `xcodecli` into `$HOME/.local/bin` by default
- verifies that the installed binary runs successfully
- checks whether your login shell can find `xcodecli` on `PATH` and prints shell-specific guidance if it cannot

Upgrade an existing install in place:

```bash
xcodecli update
```

`xcodecli update` delegates to `brew upgrade oozoofrog/tap/xcodecli` for Homebrew installs. Other installs download the latest GitHub release, rebuild it, and replace the current executable.

The shared `oozoofrog/tap` repository can host multiple formulas and casks. `xcodecli` is published there as `Formula/xcodecli.rb`.

If a release needs to be synced manually, see `docs/releasing.md` and `./scripts/release_homebrew.sh`.

## Build from source

```bash
swift build
swift test
./scripts/build-swift.sh .tmp/xcodecli
```

Override version or build channel:

```bash
VERSION=v1.0.0 BUILD_CHANNEL=release ./scripts/build-swift.sh .tmp/xcodecli
```

## Usage

Running `xcodecli` with no arguments prints help. Use `bridge` for raw passthrough to `xcrun mcpbridge`, or `serve` when an MCP client should talk to `xcodecli` directly while reusing the LaunchAgent-backed runtime.

```bash
./xcodecli
./xcodecli version
./xcodecli --xcode-pid 12345
./xcodecli bridge --session-id 11111111-1111-1111-1111-111111111111
./xcodecli serve --session-id 11111111-1111-1111-1111-111111111111
```

Fastest workflow tutor for a real request:

```bash
./xcodecli agent guide "build Unicody"
./xcodecli agent guide "read KeyboardState.swift"
./xcodecli agent guide --json
```

`agent guide` is read-only. It maps a user request to the recommended tool workflow, shows why that order is correct, and prints the exact next commands to run.

Fastest safe live onboarding demo:

```bash
./xcodecli agent demo
./xcodecli agent demo --json
```

`agent demo` is read-only. It reuses `doctor`, discovers the live tool catalog, safely calls `XcodeListWindows`, and prints the next commands to try.

Run environment diagnostics:

```bash
./xcodecli doctor
./xcodecli doctor --json
MCP_XCODE_PID=12345 ./xcodecli doctor --json
```

Generate MCP registration commands for supported clients:

```bash
./xcodecli mcp codex
./xcodecli mcp claude
./xcodecli mcp gemini
./xcodecli mcp codex --write
./xcodecli mcp claude --write --json
```

Notes:
- `mcp config` targets `xcodecli serve` by default so MCP clients reuse the LaunchAgent-backed pooled runtime.
- Use `--mode bridge` if you explicitly want raw `xcodecli bridge` passthrough instead.
- `xcodecli mcp codex|claude|gemini` are shorthand aliases for `xcodecli mcp config --client ...`.
- `mcp config` warns when the current `xcodecli` executable path looks unstable for long-lived MCP registration (for example `.build`, `Cellar`, `/tmp`, or external-volume paths).
- Add `--strict-stable-path` if you want `mcp config` to fail instead of warn when the current executable path looks unstable.
- Output-only mode prints a ready-to-paste registration command and does **not** create or reuse `xcodecli`'s persistent session file.
- The first actual `serve` run creates or reuses `xcodecli`'s persistent session ID at runtime if you did not pass `--session-id`.
- `--write` delegates registration to the target client CLI instead of editing that client's config files directly.
- Gemini defaults to `--scope user` so it does not write `.gemini/settings.json` into the current project unless you explicitly choose `--scope project`.
- For long-lived MCP usage, register a **stable xcodecli path** (for example `/opt/homebrew/bin/xcodecli` or `~/.local/bin/xcodecli`). Switching between different binaries or checkout paths forces the LaunchAgent to recycle its backend session, which can surface fresh Xcode authorization prompts.
- Avoid changing `MCP_XCODE_PID` or `DEVELOPER_DIR` between runs unless you intentionally want a separate pooled session.

List tools through the MCP bridge:

```bash
./xcodecli tools list
./xcodecli tools list --json --timeout 60s
```

Inspect a single tool before calling it:

```bash
./xcodecli tool inspect XcodeListWindows
./xcodecli tool inspect XcodeListWindows --json --timeout 60s
```

Call a single tool with JSON arguments:

```bash
./xcodecli tool call XcodeListWindows --json '{}'
./xcodecli tool call BuildProject --timeout 30m --json @/tmp/payload.json
printf '{}' | ./xcodecli tool call XcodeListWindows --json-stdin
```

Inspect the LaunchAgent used by `tools` commands:

```bash
./xcodecli agent guide "build Unicody"
./xcodecli agent demo
./xcodecli agent status
./xcodecli agent status --json
./xcodecli agent stop
./xcodecli agent uninstall
```

## LLM agent workflow playbook

Start here when you already know the task:

```bash
./xcodecli agent guide "build Unicody"
./xcodecli agent guide "run tests for Unicody"
./xcodecli agent guide "read KeyboardState.swift"
./xcodecli agent guide "search for AdManager"
./xcodecli agent guide "update KeyboardState.swift"
./xcodecli agent guide "diagnose build errors"
```

Notes:
- `agent guide` is the fastest way to learn the right tool sequence for a concrete request.
- `agent demo` is the safest way to discover live windows and tool availability before you pick a workflow.
- Many Xcode MCP tools require a `tabIdentifier`; both `agent guide` and `agent demo` help you understand when and why `XcodeListWindows` comes first.
- `tool inspect` is still available, but it should usually be a fallback for schema reassurance rather than the first step.

## Manual low-level flow

If you want the raw building blocks instead of guidance:

```bash
./xcodecli tools list
./xcodecli tool inspect XcodeListWindows --json
./xcodecli tool call XcodeListWindows --json '{}'
./xcodecli tool call BuildProject --json '{"tabIdentifier":"<tabIdentifier from above>"}'
```

After `agent guide` and `agent demo`, the next likely usability improvement is a higher-level task command. This repository does **not** add that abstraction yet.

## Agent onboarding

- Quick rules for first-time agents: `AGENTS.md`
- Detailed walkthrough: `docs/agent-quickstart.md`

## Git workflow

- `main`: stable baseline branch
- `codex/*`: implementation branches for agent-driven changes
- Open pull requests from `codex/*` into `main`


## Versioning strategy

The project now uses stable semantic versioning tags with the following release policy:

- `v1.0.1`, `v1.0.2`, ...: patch releases for bug fixes, CI/test hardening, documentation corrections, and internal refactors that do not intentionally expand the public CLI surface.
- `v1.1.0`, `v1.2.0`, ...: minor releases for new commands, new flags, new output modes, default-behavior expansions, or materially new LaunchAgent / MCP capabilities.
- Breaking CLI behavior is avoided when possible. Any unavoidable breaking change should ship in a new major release and must be called out explicitly in `CHANGELOG.md` and the GitHub Release notes.
- Releases should be cut from `main` only after CI is green.
- Tags should remain annotated `vMAJOR.MINOR.PATCH` tags, and GitHub Releases should continue to use generated notes unless a release needs hand-written upgrade guidance.
- The active maintenance line is `v1.0.x`. Small fixes should prefer the next patch tag on that line before opening a new minor series.

## Notes

- `--xcode-pid` overrides `MCP_XCODE_PID`.
- `--session-id` overrides `MCP_XCODE_SESSION_ID`.
- If no `--session-id` flag or `MCP_XCODE_SESSION_ID` environment variable is provided, `xcodecli` automatically creates and reuses a persistent session ID at `~/Library/Application Support/xcodecli/session-id`.
- In bridge mode, **stdout is protocol-only**. Wrapper logs and diagnostics go to stderr.
- Convenience commands (`tools list`, `tool inspect`, `tool call`) automatically install and bootstrap a per-user LaunchAgent at `~/Library/LaunchAgents/io.oozoofrog.xcodecli.plist`.
- `--timeout` is the **request timeout**. It includes first-use LaunchAgent startup, `mcpbridge` session initialization, and any auth prompts.
- The default **mcpbridge session idle timeout** is `24h`. It controls how long pooled `mcpbridge` sessions stay alive while idle.
- Active requests are **not** interrupted by the `mcpbridge session idle timeout`.
- `doctor`, `agent demo`, and `agent guide` will warn when the registered LaunchAgent binary path is relative, missing, or differs from the current binary because those drifts are common causes of LaunchAgent bootstrap failures and unexpected re-authorization churn.
- `agent status` surfaces the same stale-registration warnings in human-readable mode so you can triage LaunchAgent drift without running the full doctor flow first.
- `doctor --json` now includes structured `recommendations` alongside raw checks so automation can act on common remediation paths directly.
- `agent guide` and `agent demo` now echo relevant doctor recommendations in their human-readable environment sections so first-run triage does not require a separate doctor pass.
- `update` now refuses obviously unstable executable paths (for example Swift build outputs, temporary directories, and external-volume paths) so in-place upgrades only happen from stable install locations.
- Default request timeouts are `60s` for `tools list`, `tool inspect`, `agent guide`, and `agent demo`; `tool call` uses tool-specific defaults (`60s` list/read/search/log, `120s` update/write/refresh, `30m` build/test, `5m` fallback).
- `tool call` accepts exactly one payload source: inline `--json`, `--json @file`, or `--json-stdin`.
