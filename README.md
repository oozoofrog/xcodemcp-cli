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

## Homebrew

Install from the shared oozoofrog tap:

```bash
brew tap oozoofrog/tap
brew install oozoofrog/tap/xcodemcp
```

Upgrade to the latest published version:

```bash
brew update
brew upgrade oozoofrog/tap/xcodemcp
```

The `oozoofrog/tap` repository is a shared tap that can host multiple formulas and casks. `xcodemcp` is published there as `Formula/xcodemcp.rb`.

If a release needs to be synced manually, see `/Volumes/eyedisk/develop/oozoofrog/xcodemcp-cli/docs/releasing.md` and `./scripts/release_homebrew.sh`.

## Usage

Running `xcodemcp` with no arguments prints help. Use `bridge` for raw passthrough to `xcrun mcpbridge`.

```bash
./xcodemcp
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


## Versioning strategy

Starting after `v0.2.0`, the project will continue to use pre-1.0 semantic versioning tags with the following release policy:

- `v0.2.1`, `v0.2.2`, ...: patch releases for bug fixes, CI/test hardening, documentation corrections, and internal refactors that do not intentionally expand the public CLI surface.
- `v0.3.0`, `v0.4.0`, ...: minor releases for new commands, new flags, new output modes, default-behavior expansions, or materially new LaunchAgent / MCP capabilities.
- Breaking CLI behavior is avoided when possible. Before `v1.0.0`, any unavoidable breaking change should ship in a new minor release and must be called out explicitly in `CHANGELOG.md` and the GitHub Release notes.
- Releases should be cut from `main` only after CI is green.
- Tags should remain annotated `vMAJOR.MINOR.PATCH` tags, and GitHub Releases should continue to use generated notes unless a release needs hand-written upgrade guidance.
- The active maintenance line after this release is `v0.2.x`. Small fixes should prefer the next patch tag on that line before opening a new minor series.

## Notes

- `--xcode-pid` overrides `MCP_XCODE_PID`.
- `--session-id` overrides `MCP_XCODE_SESSION_ID`.
- If no `--session-id` flag or `MCP_XCODE_SESSION_ID` environment variable is provided, `xcodemcp` automatically creates and reuses a persistent session ID at `~/Library/Application Support/xcodemcp/session-id`.
- In bridge mode, **stdout is protocol-only**. Wrapper logs and diagnostics go to stderr.
- Convenience commands (`tools list`, `tool inspect`, `tool call`) automatically install and bootstrap a per-user LaunchAgent at `~/Library/LaunchAgents/io.oozoofrog.xcodemcp.plist`.
- The LaunchAgent talks to `xcrun mcpbridge` over a long-lived local Unix socket and shuts itself down after `10m` of idleness by default.
- `tool call` accepts exactly one payload source: inline `--json`, `--json @file`, or `--json-stdin`.
