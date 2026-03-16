# xcodecli Implementation Specification

> Baseline version: `v0.5.2`
>
> This document describes `xcodecli` at a level detailed enough to reimplement it in another language, including its public CLI contract, internal architecture, protocols, persistence rules, installation flow, release flow, and operational assumptions.
>
> Korean version: [`implementation-spec.kr.md`](./implementation-spec.kr.md)

---

## Purpose
- Provide a full implementation contract for engineers porting `xcodecli` to another language/runtime
- Provide an exact behavioral reference for automation and agent authors
- Provide an operational reference for installation, release, and Homebrew distribution

## Table of Contents
- [1. Product Overview](#1-product-overview)
- [2. Global Constraints](#2-global-constraints)
- [3. System Architecture](#3-system-architecture)
- [4. Persistence and Local Paths](#4-persistence-and-local-paths)
- [5. Options and Environment Variable Precedence](#5-options-and-environment-variable-precedence)
- [6. Public CLI Contract](#6-public-cli-contract)
- [7. Internal RPC and MCP Protocol](#7-internal-rpc-and-mcp-protocol)
- [8. Timeout and Cancellation Rules](#8-timeout-and-cancellation-rules)
- [9. Build / Install / Release / Operations](#9-build--install--release--operations)
- [10. Porting Checklist](#10-porting-checklist)
- [11. Recommended Port Structure](#11-recommended-port-structure)

---

## 1. Product Overview

### 1.1 Name
- `xcodecli`

### 1.2 Product Type
- macOS-only CLI tool
- A Go-based operator-friendly wrapper around `xcrun mcpbridge`

### 1.3 Core Responsibilities
- Provide a raw stdio bridge to `xcrun mcpbridge`
- Provide a stdio MCP server (`serve`)
- Provide a LaunchAgent-backed pooled runtime
- Expose Xcode MCP tool discovery / inspect / call flows
- Generate and optionally execute MCP registration commands for Codex / Claude / Gemini
- Provide environment diagnostics and read-only workflow guidance

### 1.4 Non-goals
- No non-macOS support
- No custom Xcode tool implementation
- No remote hosted service / SaaS behavior

---

## 2. Global Constraints

### 2.1 Platform
- Supported OS: `darwin` only
- If `runtime.GOOS != "darwin"`, the process must exit with:
  - stderr: `xcodecli: only macOS (darwin) is supported`
  - exit code: `1`

### 2.2 Xcode Assumptions
- `bridge`, `serve`, `tools`, `tool`, `agent guide`, and `agent demo` all assume a valid local Xcode/MCP environment
- `tools` / `tool` flows generally assume:
  - Xcode is running
  - At least one workspace/project window is open

### 2.3 stdout / stderr Rules
- `bridge`: stdout is protocol-only
- `serve`: stdout is MCP JSON-RPC only
- Human-readable logs, debug output, and diagnostics go to stderr only

### 2.4 Destructive Operations
- Installation, release publication, and Homebrew push are last-step operations
- Release/Homebrew flows should prefer dry-run or local validation first whenever possible

---

## 3. System Architecture

### 3.1 Layering
#### Raw bridge layer
- `bridge`
  - Raw stdio passthrough to `xcrun mcpbridge`
- `serve`
  - `xcodecli` acts as a stdio MCP server itself
  - Reuses the LaunchAgent-backed pooled runtime internally

#### Operator-friendly layer
- `doctor`
- `mcp config` / `mcp <client>`
- `tools list`
- `tool inspect`
- `tool call`
- `agent guide`
- `agent demo`
- `agent status`
- `agent stop`
- `agent uninstall`
- `agent run` (internal-only)

### 3.2 Internal Package Responsibilities
- `internal/bridge`
  - Environment option resolution
  - Persistent session-id creation/reuse
  - Raw child-process bridge execution
- `internal/agent`
  - LaunchAgent-backed runtime
  - Local Unix socket RPC client/server
  - Pooled `mcpbridge` session management
- `internal/mcp`
  - MCP stdio client
  - MCP stdio server (`serve` implementation)
- `internal/doctor`
  - Environment diagnostics report generation
- `internal/update`
  - Self-update orchestration for Homebrew and direct installs

### 3.3 Runtime Topologies
#### `bridge`
```text
stdin/stdout/stderr
   ↕
 xcodecli bridge
   ↕ raw passthrough
 xcrun mcpbridge
   ↕
 Xcode MCP tools
```

#### `serve`
```text
MCP client
   ↕ stdio JSON-RPC
 xcodecli serve
   ↕ local agent RPC (unix socket)
 LaunchAgent runtime (xcodecli agent run)
   ↕ pooled MCP stdio sessions
 xcrun mcpbridge
   ↕
 Xcode MCP tools
```

#### `tools` / `tool`
```text
xcodecli tools/tool command
   ↕ local agent RPC
 LaunchAgent runtime
   ↕ pooled MCP stdio sessions
 xcrun mcpbridge
```

---

## 4. Persistence and Local Paths

### 4.1 Persistent Session File
- Path: `~/Library/Application Support/xcodecli/session-id`
- Purpose: reuse `MCP_XCODE_SESSION_ID`
- Permissions:
  - directory: `0700`
  - file: `0600`
- Contents:
  - one UUID line plus a trailing newline

### 4.2 LaunchAgent Paths
- label: `io.oozoofrog.xcodecli`
- support dir: `~/Library/Application Support/xcodecli`
- socket path: `~/Library/Application Support/xcodecli/daemon.sock`
- pid path: `~/Library/Application Support/xcodecli/daemon.pid`
- log path: `~/Library/Application Support/xcodecli/agent.log`
- plist path: `~/Library/LaunchAgents/io.oozoofrog.xcodecli.plist`

### 4.3 LaunchAgent plist Rules
- ProgramArguments:
  1. current binary path
  2. `agent`
  3. `run`
  4. `--launch-agent`
- `RunAtLoad = true`
- `StandardOutPath = agent.log`
- `StandardErrorPath = agent.log`

---

## 5. Options and Environment Variable Precedence

### 5.1 Relevant Environment Variables
- `MCP_XCODE_PID`
- `MCP_XCODE_SESSION_ID`
- `DEVELOPER_DIR`

### 5.2 Precedence
#### Xcode PID
1. `--xcode-pid`
2. env `MCP_XCODE_PID`

#### Session ID
1. `--session-id`
2. env `MCP_XCODE_SESSION_ID`
3. persistent session file
4. generate a new UUID and persist it

### 5.3 Session Source Enum
- `explicit`
- `env`
- `persisted`
- `generated`
- `unset`

### 5.4 Validation Rules
- PID must be a positive integer
- Session ID must be a UUID

---

## 6. Public CLI Contract

### 6.1 Root Parsing Rules
#### No arguments
```bash
xcodecli
```
- Print root help
- Exit code: `0`

#### First token is a flag
```bash
xcodecli --xcode-pid 123 --session-id ... --debug
```
- Interpreted as the `bridge` shorthand form

#### Common error output
- stderr prefix: `xcodecli: ...`
- general failure exit code: `1`

### 6.2 Command List
- `version`
- `update`
- `bridge`
- `serve`
- `doctor`
- `mcp`
- `tools`
- `tool`
- `agent`
- internal only: `agent run`

### 6.3 `version`
#### Usage
```bash
xcodecli version
xcodecli --version
```

#### Output
- release build: `xcodecli v0.5.2`
- dev build: `xcodecli v0.5.2 (dev)`


### 6.4 `update`
#### Usage
```bash
xcodecli update
```

#### Flags
- `-h`, `--help`

#### Algorithm
1. Resolve the current `xcodecli` executable path.
2. Fail if the path looks like a temporary Go build output.
3. Detect whether the binary is Homebrew-managed via `brew --prefix oozoofrog/tap/xcodecli`.
4. If Homebrew-managed, run `brew upgrade oozoofrog/tap/xcodecli`.
5. Otherwise query the latest semantic-version release tag with `git ls-remote --refs --tags`.
6. Download that tag tarball, build a release binary, and replace the current executable.
7. Verify the new binary with `version` output.

#### Output examples
- already current via Homebrew: `xcodecli is already up to date via Homebrew (v0.5.2)`
- updated direct install: `updated xcodecli: v0.5.1 -> v0.5.2`

#### Notes
- Any non-Homebrew path is treated as a direct install.
- Direct-install updates require `curl`, `git`, `tar`, and `go` on PATH.

### 6.5 `bridge`
#### Usage
```bash
xcodecli bridge [--xcode-pid PID] [--session-id UUID] [--debug]
xcodecli [--xcode-pid PID] [--session-id UUID] [--debug]
```

#### Flags
- `--xcode-pid PID`
- `--session-id UUID`
- `--debug`
- `-h`, `--help`

#### Algorithm
1. Resolve effective options
2. Apply environment overrides
3. Launch `xcrun mcpbridge`
4. Connect stdin/stdout/stderr directly to the child process
5. Return the child exit code

#### Output contract
- stdout: protocol-only
- stderr: wrapper error/debug only

#### Exit codes
- child exit code is propagated
- internal wrapper failures return `1`

### 6.6 `serve`
#### Usage
```bash
xcodecli serve [--xcode-pid PID] [--session-id UUID] [--debug]
```

#### Flags
- `--xcode-pid PID`
- `--session-id UUID`
- `--debug`
- `-h`, `--help`

#### Handler forwarding rules
- `ListTools` → `agent.ListTools`
- `CallTool` → `agent.CallTool`
- Request forwarding uses `agent.BuildRequest(..., timeout=0, debug=<cli debug>)`
- There is no explicit `--timeout` flag for `serve`

#### Supported MCP methods
##### initialize
Input example:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {"protocolVersion": "2025-06-18"}
}
```

Supported versions:
- `2025-06-18`
- `2025-03-26`
- `2024-11-05`

Response rules:
- If the version is supported, echo it back as `protocolVersion`
- If unsupported, return `-32602` with `{requested, supported}` in `data`

##### notifications/initialized
- No response
- Can be ignored

##### notifications/cancelled
- Cancels an in-flight request by `requestId` (string or number)
- Cancels the in-flight request context
- Suppresses the response if the cancelled request eventually completes

##### tools/list
Response payload:
```json
{"tools": [ ... ]}
```

##### tools/call
Input params:
```json
{
  "name": "BuildProject",
  "arguments": {"tabIdentifier": "..."}
}
```

Response payload:
- Copy the backend result object verbatim
- Include `isError: true` when the tool-level result is an error

##### Duplicate request IDs
- If the same request ID is already in progress, return `-32600`

#### Output contract
- stdout: MCP JSON-RPC only
- stderr: debug / diagnostics only

#### Exit codes
- normal server termination: `0`
- validation or serve runtime failure: `1`

### 6.7 `doctor`
#### Usage
```bash
xcodecli doctor [--json] [--xcode-pid PID] [--session-id UUID]
```

#### Diagnostic checks (representative)
- `xcrun lookup`
- `xcrun mcpbridge --help`
- `xcode-select -p`
- `running Xcode processes`
- `effective MCP_XCODE_PID`
- `effective MCP_XCODE_SESSION_ID`
- `spawn smoke test`
- optional LaunchAgent status checks

#### JSON output
```json
{
  "success": true,
  "summary": {"ok":0,"warn":0,"fail":0,"info":0},
  "checks": [
    {"name":"xcrun lookup","status":"ok","detail":"/usr/bin/xcrun"}
  ]
}
```

#### Text output
- `[OK]`, `[WARN]`, `[FAIL]`, `[INFO]` status prefixes
- Final summary line included

#### Exit codes
- `success == true` → `0`
- otherwise `1`

### 6.8 `mcp`
#### Subcommands
- `mcp config`
- `mcp codex`
- `mcp claude`
- `mcp gemini`

#### `mcp config` usage
```bash
xcodecli mcp config \
  --client <claude|codex|gemini> \
  [--mode <agent|bridge>] \
  [--name xcodecli] \
  [--scope SCOPE] \
  [--write] \
  [--json] \
  [--xcode-pid PID] \
  [--session-id UUID]
```

#### Defaults
- `mode = agent`
- `name = xcodecli`
- Claude scope default = `local`
- Gemini scope default = `user`

#### Mode semantics
- `agent` → server command = `xcodecli serve`
- `bridge` → server command = `xcodecli bridge`

#### Client-specific registration commands
##### Codex
```bash
codex mcp add <name> [--env KEY=VALUE ...] -- <xcodecli path> serve|bridge
```

##### Claude
```bash
claude mcp add-json -s <scope> <name> '<json payload>'
```

Payload shape:
```json
{
  "type": "stdio",
  "command": "/abs/path/to/xcodecli",
  "args": ["serve"],
  "env": {"MCP_XCODE_PID": "123"}
}
```

##### Gemini
```bash
gemini mcp add -s <scope> <name> <xcodecli path> serve|bridge
```

#### `--write` behavior
- Codex / Gemini: execute one add command
- Claude: if add-json fails with `already exists`, remove and retry

#### JSON output schema
```json
{
  "client": "codex",
  "mode": "agent",
  "name": "xcodecli",
  "scope": "local",
  "server": {
    "command": "/abs/path/to/xcodecli",
    "args": ["serve"],
    "env": {"MCP_XCODE_PID": "123"}
  },
  "command": ["codex","mcp","add",...],
  "displayCommand": "codex mcp add ...",
  "write": {
    "requested": true,
    "executed": true,
    "exitCode": 0,
    "stdout": "...",
    "stderr": "..."
  }
}
```

#### Important constraints
- Output-only mode must not create the persistent session file
- Temporary Go build binaries must be rejected as registration targets
- Codex does not support `--scope`

### 6.9 `tools list`
#### Usage
```bash
xcodecli tools list [--json] [--timeout 60s] [--xcode-pid PID] [--session-id UUID] [--debug]
```

#### Output
- Text: `<name>\t<description>` or name-only lines
- JSON: raw tool object array

#### Exit codes
- success `0`
- failure `1`

### 6.10 `tool inspect`
#### Usage
```bash
xcodecli tool inspect <name> [--json] [--timeout 60s] [--xcode-pid PID] [--session-id UUID] [--debug]
```

#### Text output
```text
name: <tool>
description: <desc>
inputSchema:
<pretty JSON>
```

#### JSON output
- Full tool object

### 6.11 `tool call`
#### Usage
```bash
xcodecli tool call <name> (--json '{...}' | --json @payload.json | --json-stdin) [--timeout DURATION] [--xcode-pid PID] [--session-id UUID] [--debug]
```

#### Input constraints
- Exactly one payload source is allowed
- Payload must be a JSON object

#### Default timeout policy
- 60s: list/read/search/log tools
- 120s: update/write/refresh tools
- 30m: `BuildProject`, `RunAllTests`, `RunSomeTests`
- 5m: all other tools

#### Output
- Always writes a JSON result object
- Exit code becomes `1` when `result.IsError == true`

### 6.12 `agent guide`
#### Purpose
Classify a user request into a workflow family and produce next commands using live context

#### Workflow families
- `catalog`
- `build`
- `test`
- `read`
- `search`
- `edit`
- `diagnose`

#### Collected inputs
- doctor report
- agent status
- tool catalog
- `XcodeListWindows`

#### JSON output
`agentGuideReport`
```json
{
  "success": true,
  "intent": {...},
  "environment": {...},
  "workflow": {...},
  "nextCommands": ["xcodecli ..."],
  "errors": [{"step":"tools list","message":"..."}]
}
```

### 6.13 `agent demo`
#### Purpose
Safe onboarding demo for first-time use

#### Internal actions
- doctor
- tools list
- agent status
- safe `XcodeListWindows` call

#### Success condition
- doctor success
- tools list success
- windows demo attempted
- windows demo ok

### 6.14 `agent status`
#### Purpose
Inspect LaunchAgent installation / runtime / session state

#### JSON output schema
`agent.Status`
```json
{
  "label": "io.oozoofrog.xcodecli",
  "plistPath": "...",
  "plistInstalled": true,
  "registeredBinary": "...",
  "currentBinary": "...",
  "binaryPathMatches": true,
  "socketPath": "...",
  "socketReachable": true,
  "running": true,
  "pid": 123,
  "idleTimeout": 86400000000000,
  "backendSessions": 1
}
```

### 6.15 `agent stop`
- Send stop RPC
- Output: `stopped LaunchAgent process if it was running`

### 6.16 `agent uninstall`
- Remove plist/socket/pid/log/support dir
- Output: `removed LaunchAgent plist and local agent runtime files`

### 6.17 `agent run`
- Internal-only entrypoint
- Requires `--launch-agent`
- Default `--idle-timeout` is `24h`

---

## 7. Internal RPC and MCP Protocol

### 7.1 Local agent RPC
Transport:
- Unix domain socket
- Request/response are newline-delimited JSON
- One request per connection

#### Request schema
```json
{
  "method": "tools/call",
  "xcodePid": "123",
  "sessionId": "uuid",
  "developerDir": "/Applications/Xcode.app/Contents/Developer",
  "timeoutMs": 60000,
  "debug": true,
  "toolName": "BuildProject",
  "arguments": {"tabIdentifier":"..."}
}
```

#### Methods
- `ping`
- `status`
- `stop`
- `tools/list`
- `tools/call`

#### Response schema
```json
{
  "error": "...",
  "tools": [ ... ],
  "result": { ... },
  "isError": false,
  "status": {
    "pid": 123,
    "idleTimeoutMs": 86400000,
    "backendSessions": 1
  }
}
```

### 7.2 MCP stdio client
#### initialize request
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2025-06-18",
    "capabilities": {},
    "clientInfo": {"name":"xcodecli","version":"dev"}
  }
}
```

#### post-initialize
```json
{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}
```

#### tools/list
- Follows pagination until `nextCursor` is empty

#### tools/call
```json
{"name":"BuildProject","arguments":{...}}
```

#### Unsupported server request behavior
- The client does not support server-initiated requests
- Returns `Method not found` and fails the call

### 7.3 MCP stdio server
#### Supported methods
- `initialize`
- `notifications/initialized`
- `notifications/cancelled`
- `tools/list`
- `tools/call`

#### Unsupported version response example
```json
{
  "jsonrpc":"2.0",
  "id":1,
  "error": {
    "code": -32602,
    "message": "Unsupported protocol version",
    "data": {
      "requested": "2099-01-01",
      "supported": ["2025-06-18","2025-03-26","2024-11-05"]
    }
  }
}
```

#### Duplicate request IDs
- `-32600` / `request id is already in progress`

---

## 8. Timeout and Cancellation Rules

### 8.1 Meaning of request timeout
`--timeout` covers:
- LaunchAgent startup
- MCP session initialization
- Authentication prompt wait time

### 8.2 Idle timeout
- Default pooled `mcpbridge` session idle timeout: `24h`
- Active requests are not interrupted by the idle timeout

### 8.3 `serve` cancellation
- `notifications/cancelled` cancels in-flight requests by request ID
- Cancels the in-flight request context
- Suppresses responses for requests that were cancelled
- EOF on stdin cancels all in-flight requests

### 8.4 `agent` cancellation
- Client context cancellation forces socket deadline expiry so reads/writes unblock
- Server connection close cancels the request context
- Cancelling a long-running request may abort the pooled session to avoid stale work accumulation

---

## 9. Build / Install / Release / Operations

### 9.1 Build
- Script: `scripts/build.sh`
- Package: `./cmd/xcodecli`
- ldflags:
  - `-X main.cliVersion=<VERSION>`
  - `-X main.cliBuildChannel=<BUILD_CHANNEL>`

### 9.2 Install
#### Homebrew
```bash
brew tap oozoofrog/tap
brew install oozoofrog/tap/xcodecli
```

#### Direct GitHub install
```bash
curl -fsSL https://raw.githubusercontent.com/oozoofrog/xcodecli/main/scripts/install.sh | bash
curl -fsSL https://raw.githubusercontent.com/oozoofrog/xcodecli/main/scripts/install.sh | bash -s -- --ref v0.5.2
```

#### Local checkout install
```bash
./scripts/install.sh
./scripts/install.sh --bin-dir "$HOME/.local/bin"
```

### 9.3 `scripts/install.sh` behavior
- If run from a local checkout, build the current tree
- If run outside a checkout, download the GitHub ref tarball and build it
- Default install path: `$HOME/.local/bin`
- Validate that the installed binary runs
- Check login-shell PATH reachability and print shell-specific guidance

### 9.4 Release flow
1. Merge into `main`
2. Run local verification (`go test`, build, version)
3. Push an annotated tag (`vX.Y.Z`)
4. Create/publish a GitHub Release
5. `release.published` triggers the Homebrew workflow

### 9.5 Homebrew
- Tap repo: `oozoofrog/homebrew-tap`
- Only `Formula/xcodecli.rb` may be modified
- Script: `scripts/release_homebrew.sh`
- Operations:
  - download tag tarball
  - compute sha256
  - generate/update formula
  - `brew audit --strict`
  - `brew install --build-from-source`
  - smoke test + `brew test`
  - support dry-run / local commit / push

### 9.6 CI
#### `.github/workflows/ci.yml`
Triggers:
- push to `main`
- push to `codex/**`
- pull requests

Jobs:
- gofmt check
- `go test ./...`
- `./scripts/build.sh .tmp/xcodecli`

#### `.github/workflows/homebrew-release.yml`
Triggers:
- `release.published`
- `workflow_dispatch`

Required secret:
- `HOMEBREW_TAP_GITHUB_TOKEN`

---

## 10. Porting Checklist

Any reimplementation must preserve at least the following:
1. macOS-only platform gate
2. The bridge / serve / tools / tool / agent command model
3. Protocol-only stdout rules for `bridge` and `serve`
4. Session-ID precedence and persistent file semantics
5. LaunchAgent label/path/layout
6. Local Unix socket RPC schema and method set
7. MCP initialize / tools/list / tools/call contracts
8. Default tool timeout categories
9. `mcp config` client-specific command generation rules
10. doctor / agent guide / agent demo / agent status JSON shapes
11. Build / install / release / Homebrew / CI operational flow

---

## 11. Recommended Port Structure
- `bridge`
  - environment resolution
  - session persistence
  - raw child-process passthrough
- `agent`
  - LaunchAgent-equivalent lifecycle
  - local RPC server/client
  - pooled backend session manager
- `mcp`
  - stdio JSON-RPC client/server
- `doctor`
  - environment inspector
- `cli`
  - command parsing
  - help text
  - text/JSON rendering

This modular split best matches the current implementation’s coupling, tests, and operational structure.
