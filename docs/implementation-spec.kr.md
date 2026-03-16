# xcodecli 구현 명세서

> Baseline version: `v0.5.2`
>
> 이 문서는 `xcodecli`의 공개 기능, 내부 구조, 프로토콜, 설치/배포/운영 규칙을 **다른 언어에서도 재구현 가능한 수준**으로 정리한 기술 명세서입니다.
>
> Original English version: [`implementation-spec.md`](./implementation-spec.md)

---

## 문서 목적
- 다른 언어/런타임으로 `xcodecli`를 포팅하려는 엔지니어에게 구현 기준 제공
- CLI/MCP/runtime 동작을 정확히 이해해야 하는 에이전트/자동화 작성자에게 계약 문서 제공
- 설치/배포/운영 흐름을 재현해야 하는 릴리스 담당자에게 운영 스펙 제공

## 목차
- [1. 제품 개요](#1-제품-개요)
- [2. 전역 제약 조건](#2-전역-제약-조건)
- [3. 시스템 아키텍처](#3-시스템-아키텍처)
- [4. 상태 저장과 로컬 경로](#4-상태-저장과-로컬-경로)
- [5. 옵션 및 환경 변수 우선순위](#5-옵션-및-환경-변수-우선순위)
- [6. 공개 CLI 계약](#6-공개-cli-계약)
- [7. 내부 RPC 및 MCP 프로토콜](#7-내부-rpc-및-mcp-프로토콜)
- [8. timeout / cancellation 규칙](#8-timeout--cancellation-규칙)
- [9. 빌드 / 설치 / 릴리스 / 운영](#9-빌드--설치--릴리스--운영)
- [10. 포팅 체크리스트](#10-포팅-체크리스트)
- [11. 권장 포트 구조](#11-권장-포트-구조)

---

## 1. 제품 개요

### 1.1 제품명
- `xcodecli`

### 1.2 제품 유형
- macOS 전용 CLI 도구
- `xcrun mcpbridge`를 감싸는 Go 기반 operator-friendly wrapper

### 1.3 핵심 역할
- raw stdio bridge 제공
- stdio MCP server 제공
- LaunchAgent-backed pooled runtime 제공
- Xcode MCP 도구 discovery / inspect / call 기능 제공
- MCP 클라이언트(Codex/Claude/Gemini) 등록 커맨드 생성/실행 제공
- 환경 진단 및 read-only workflow guidance 제공

### 1.4 비목표
- macOS 외 플랫폼 지원 없음
- 자체 Xcode 도구 구현 없음
- 원격 서버/클라우드 서비스 제공 없음

---

## 2. 전역 제약 조건

### 2.1 플랫폼
- 지원 OS: `darwin` only
- `runtime.GOOS != "darwin"`이면 stderr에 아래 메시지를 출력하고 종료
  - `xcodecli: only macOS (darwin) is supported`
- 종료 코드: `1`

### 2.2 Xcode 전제 조건
- `bridge`, `serve`, `tools`, `tool`, `agent guide`, `agent demo`는 실질적으로 Xcode/MCP 환경 준비를 전제로 함
- `tools` / `tool` 계열은 일반적으로:
  - Xcode 실행 중
  - 최소 하나의 workspace/project window open 상태 필요

### 2.3 stdout / stderr 규칙
- `bridge`: stdout은 프로토콜 전용
- `serve`: stdout은 MCP JSON-RPC 전용
- 사람이 읽는 로그/디버그/진단은 stderr만 사용

### 2.4 파괴적 작업 원칙
- 설치/릴리스/Homebrew push 같은 외부 상태 변경은 마지막 단계에 수행
- release/Homebrew는 dry-run 가능 시 먼저 검증

---

## 3. 시스템 아키텍처

### 3.1 계층 구조
#### Raw bridge 계층
- `bridge`
  - `xcrun mcpbridge`에 대한 raw stdio passthrough
- `serve`
  - `xcodecli` 자체가 stdio MCP server 역할 수행
  - 내부적으로 LaunchAgent-backed pooled runtime 재사용

#### Operator-friendly 계층
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
- `agent run` (내부 전용)

### 3.2 내부 패키지 역할
- `internal/bridge`
  - env 옵션 해석
  - persistent session-id 생성/재사용
  - raw child-process bridge
- `internal/agent`
  - LaunchAgent-backed runtime
  - local Unix socket RPC client/server
  - pooled `mcpbridge` session 관리
- `internal/mcp`
  - MCP stdio client
  - MCP stdio server (`serve` 구현)
- `internal/doctor`
  - 환경 진단 보고서 생성
- `internal/update`
  - Homebrew 및 직접 설치용 자기 업데이트 orchestration

### 3.3 런타임 토폴로지
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

## 4. 상태 저장과 로컬 경로

### 4.1 Persistent session file
- 경로: `~/Library/Application Support/xcodecli/session-id`
- 용도: `MCP_XCODE_SESSION_ID` 재사용
- 생성 권한:
  - 디렉터리: `0700`
  - 파일: `0600`
- 파일 내용:
  - UUID 문자열 1줄 + trailing newline

### 4.2 LaunchAgent 경로
- label: `io.oozoofrog.xcodecli`
- support dir: `~/Library/Application Support/xcodecli`
- socket path: `~/Library/Application Support/xcodecli/daemon.sock`
- pid path: `~/Library/Application Support/xcodecli/daemon.pid`
- log path: `~/Library/Application Support/xcodecli/agent.log`
- plist path: `~/Library/LaunchAgents/io.oozoofrog.xcodecli.plist`

### 4.3 LaunchAgent plist 규칙
- ProgramArguments:
  1. current binary path
  2. `agent`
  3. `run`
  4. `--launch-agent`
- `RunAtLoad = true`
- `StandardOutPath = agent.log`
- `StandardErrorPath = agent.log`

---

## 5. 옵션 및 환경 변수 우선순위

### 5.1 관련 환경 변수
- `MCP_XCODE_PID`
- `MCP_XCODE_SESSION_ID`
- `DEVELOPER_DIR`

### 5.2 우선순위
#### Xcode PID
1. `--xcode-pid`
2. env `MCP_XCODE_PID`

#### Session ID
1. `--session-id`
2. env `MCP_XCODE_SESSION_ID`
3. persistent session file
4. 새 UUID 생성 후 파일 저장

### 5.3 Session source enum
- `explicit`
- `env`
- `persisted`
- `generated`
- `unset`

### 5.4 유효성 규칙
- PID: 양의 정수
- Session ID: UUID 형식

---

## 6. 공개 CLI 계약

### 6.1 루트 파싱 규칙
#### 인자 없음
```bash
xcodecli
```
- root help 출력
- 종료 코드: `0`

#### 첫 토큰이 플래그인 경우
```bash
xcodecli --xcode-pid 123 --session-id ... --debug
```
- `bridge` shorthand로 해석

#### 공통 에러 출력
- stderr prefix: `xcodecli: ...`
- 일반 실패 exit code: `1`

### 6.2 명령 목록
- `version`
- `update`
- `bridge`
- `serve`
- `doctor`
- `mcp`
- `tools`
- `tool`
- `agent`
- 내부 전용: `agent run`

### 6.3 `version`
#### 사용법
```bash
xcodecli version
xcodecli --version
```

#### 출력
- release build: `xcodecli v0.5.2`
- dev build: `xcodecli v0.5.2 (dev)`


### 6.4 `update`
#### 사용법
```bash
xcodecli update
```

#### 플래그
- `-h`, `--help`

#### 동작 알고리즘
1. 현재 실행 중인 `xcodecli` 바이너리 경로를 해석한다.
2. 경로가 임시 Go build 산출물처럼 보이면 실패한다.
3. `brew --prefix oozoofrog/tap/xcodecli`로 Homebrew 관리 설치인지 확인한다.
4. Homebrew 설치면 `brew upgrade oozoofrog/tap/xcodecli`를 실행한다.
5. Homebrew가 아니면 `git ls-remote --refs --tags`로 최신 semantic-version release tag를 찾는다.
6. 해당 tag tarball을 내려받아 release 빌드를 수행한 뒤 현재 실행 파일을 교체한다.
7. 새 바이너리의 `version` 출력을 확인한다.

#### 출력 예
- Homebrew 최신 상태: `xcodecli is already up to date via Homebrew (v0.5.2)`
- 직접 설치 업데이트 완료: `updated xcodecli: v0.5.1 -> v0.5.2`

#### 노트
- Homebrew가 아닌 경로는 모두 직접 설치로 간주한다.
- 직접 설치 업데이트에는 `curl`, `git`, `tar`, `go`가 필요하다.

### 6.5 `bridge`
#### 사용법
```bash
xcodecli bridge [--xcode-pid PID] [--session-id UUID] [--debug]
xcodecli [--xcode-pid PID] [--session-id UUID] [--debug]
```

#### 플래그
- `--xcode-pid PID`
- `--session-id UUID`
- `--debug`
- `-h`, `--help`

#### 동작 알고리즘
1. effective options 해석
2. env override 적용
3. `xcrun mcpbridge` child process 시작
4. stdin/stdout/stderr를 child process와 연결
5. child exit code 반환

#### 출력 계약
- stdout: 프로토콜 전용
- stderr: wrapper error/debug 전용

#### 종료 코드
- child exit code 전달
- wrapper 내부 오류 시 `1`

### 6.6 `serve`
#### 사용법
```bash
xcodecli serve [--xcode-pid PID] [--session-id UUID] [--debug]
```

#### 플래그
- `--xcode-pid PID`
- `--session-id UUID`
- `--debug`
- `-h`, `--help`

#### handler forwarding 규칙
- `ListTools` → `agent.ListTools`
- `CallTool` → `agent.CallTool`
- request forwarding 시 `agent.BuildRequest(..., timeout=0, debug=<cli debug>)`
- 별도 `--timeout` 플래그 없음

#### 지원 MCP 메서드
##### initialize
입력 예:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {"protocolVersion": "2025-06-18"}
}
```

지원 버전:
- `2025-06-18`
- `2025-03-26`
- `2024-11-05`

응답 규칙:
- 지원 버전이면 그 버전을 그대로 `protocolVersion`으로 echo
- 미지원이면 `-32602` + `{requested, supported}` data 포함

##### notifications/initialized
- 응답 없음
- 무시 가능

##### notifications/cancelled
- `requestId`(string 또는 number) 기반 취소
- in-flight request context 취소
- 취소된 요청 결과는 stdout에 쓰지 않음

##### tools/list
응답 payload:
```json
{"tools": [ ... ]}
```

##### tools/call
입력 params:
```json
{
  "name": "BuildProject",
  "arguments": {"tabIdentifier": "..."}
}
```

응답 payload:
- backend result object 그대로 복사
- tool-level failure면 `isError: true` 포함

##### 중복 request id
- 이미 진행 중인 request id가 다시 들어오면 `-32600`

#### 출력 계약
- stdout: MCP JSON-RPC only
- stderr: debug / diagnostics only

#### 종료 코드
- 정상 종료: `0`
- validate/serve runtime 오류: `1`

### 6.7 `doctor`
#### 사용법
```bash
xcodecli doctor [--json] [--xcode-pid PID] [--session-id UUID]
```

#### 진단 항목 예
- `xcrun lookup`
- `xcrun mcpbridge --help`
- `xcode-select -p`
- `running Xcode processes`
- `effective MCP_XCODE_PID`
- `effective MCP_XCODE_SESSION_ID`
- `spawn smoke test`
- optional LaunchAgent status info

#### JSON 출력
```json
{
  "success": true,
  "summary": {"ok":0,"warn":0,"fail":0,"info":0},
  "checks": [
    {"name":"xcrun lookup","status":"ok","detail":"/usr/bin/xcrun"}
  ]
}
```

#### 텍스트 출력
- `[OK]`, `[WARN]`, `[FAIL]`, `[INFO]` 접두 상태 아이콘
- 마지막 summary line 포함

#### 종료 코드
- `success == true` → `0`
- 아니면 `1`

### 6.8 `mcp`
#### 서브커맨드
- `mcp config`
- `mcp codex`
- `mcp claude`
- `mcp gemini`

#### `mcp config` 사용법
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

#### 기본값
- `mode = agent`
- `name = xcodecli`
- Claude scope default = `local`
- Gemini scope default = `user`

#### mode 의미
- `agent` → server command = `xcodecli serve`
- `bridge` → server command = `xcodecli bridge`

#### client별 registration command
##### Codex
```bash
codex mcp add <name> [--env KEY=VALUE ...] -- <xcodecli path> serve|bridge
```

##### Claude
```bash
claude mcp add-json -s <scope> <name> '<json payload>'
```

payload shape:
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

#### `--write` 동작
- Codex/Gemini: 1회 add command 실행
- Claude: add-json 실패 메시지가 `already exists`면 remove 후 retry

#### JSON 출력 스키마
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

#### 중요한 제약
- output-only mode는 persistent session file을 만들지 않음
- temp Go build binary 경로는 거부
- Codex는 `--scope` 미지원

### 6.9 `tools list`
#### 사용법
```bash
xcodecli tools list [--json] [--timeout 60s] [--xcode-pid PID] [--session-id UUID] [--debug]
```

#### 출력
- 텍스트: `<name>\t<description>` 또는 name-only line
- JSON: tool object 배열 그대로

#### 종료 코드
- 성공 `0`
- 실패 `1`

### 6.10 `tool inspect`
#### 사용법
```bash
xcodecli tool inspect <name> [--json] [--timeout 60s] [--xcode-pid PID] [--session-id UUID] [--debug]
```

#### 텍스트 출력
```text
name: <tool>
description: <desc>
inputSchema:
<pretty JSON>
```

#### JSON 출력
- tool object 전체

### 6.11 `tool call`
#### 사용법
```bash
xcodecli tool call <name> (--json '{...}' | --json @payload.json | --json-stdin) [--timeout DURATION] [--xcode-pid PID] [--session-id UUID] [--debug]
```

#### 입력 제약
- payload source는 정확히 1개만 허용
- payload는 반드시 JSON object

#### 기본 timeout 정책
- 60s: list/read/search/log 계열
- 120s: update/write/refresh 계열
- 30m: `BuildProject`, `RunAllTests`, `RunSomeTests`
- 5m: 기타 tool

#### 출력
- 항상 JSON result object
- `result.IsError == true`면 exit code `1`

### 6.12 `agent guide`
#### 목적
요청을 workflow family로 분류하고 라이브 컨텍스트를 반영한 next command를 제안

#### workflow families
- `catalog`
- `build`
- `test`
- `read`
- `search`
- `edit`
- `diagnose`

#### 수집 데이터
- doctor report
- agent status
- tool catalog
- `XcodeListWindows`

#### JSON 출력
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
#### 목적
첫 사용자를 위한 safe onboarding demo

#### 내부 동작
- doctor
- tools list
- agent status
- `XcodeListWindows` safe call

#### 성공 조건
- doctor success
- tools list success
- windows demo attempted
- windows demo ok

### 6.14 `agent status`
#### 목적
LaunchAgent 설치/실행/세션 상태 확인

#### JSON 출력 스키마
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
- stop RPC 전송
- 출력: `stopped LaunchAgent process if it was running`

### 6.16 `agent uninstall`
- plist/socket/pid/log/support dir 제거
- 출력: `removed LaunchAgent plist and local agent runtime files`

### 6.17 `agent run`
- 내부 전용 entrypoint
- `--launch-agent` 필수
- `--idle-timeout` 기본 `24h`

---

## 7. 내부 RPC 및 MCP 프로토콜

### 7.1 Local agent RPC
전송 매체:
- Unix domain socket
- request/response 모두 newline-delimited JSON
- connection당 request 1개

#### request schema
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

#### methods
- `ping`
- `status`
- `stop`
- `tools/list`
- `tools/call`

#### response schema
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
- pagination(`nextCursor`)를 따라 끝까지 이어 붙임

#### tools/call
```json
{"name":"BuildProject","arguments":{...}}
```

#### unsupported server request
- client는 server-initiated request를 지원하지 않음
- `Method not found` error 반환 후 실패

### 7.3 MCP stdio server
#### supported methods
- `initialize`
- `notifications/initialized`
- `notifications/cancelled`
- `tools/list`
- `tools/call`

#### unsupported version response 예
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

#### duplicate request id
- `-32600` / `request id is already in progress`

---

## 8. timeout / cancellation 규칙

### 8.1 request timeout 의미
`--timeout`이 커버하는 범위:
- LaunchAgent startup
- MCP session initialization
- auth prompt 대기

### 8.2 idle timeout
- pooled `mcpbridge` session idle timeout 기본값: `24h`
- active request는 idle timeout으로 끊기지 않음

### 8.3 `serve` 취소 규칙
- `notifications/cancelled` 수신 시 request id 기반 cancel
- in-flight request context 취소
- 취소된 request는 완료되어도 응답 suppression
- stdin EOF 시 in-flight request 전체 취소

### 8.4 `agent` 취소 규칙
- client context cancel → socket deadline 강제 → read/write 탈출
- server connection close 감지 → request context cancel
- long-running request cancel 시 pooled session abort 가능

---

## 9. 빌드 / 설치 / 릴리스 / 운영

### 9.1 빌드
- script: `scripts/build.sh`
- package: `./cmd/xcodecli`
- ldflags:
  - `-X main.cliVersion=<VERSION>`
  - `-X main.cliBuildChannel=<BUILD_CHANNEL>`

### 9.2 설치
#### Homebrew
```bash
brew tap oozoofrog/tap
brew install oozoofrog/tap/xcodecli
```

#### GitHub 직접 설치
```bash
curl -fsSL https://raw.githubusercontent.com/oozoofrog/xcodecli/main/scripts/install.sh | bash
curl -fsSL https://raw.githubusercontent.com/oozoofrog/xcodecli/main/scripts/install.sh | bash -s -- --ref v0.5.2
```

#### 로컬 checkout 설치
```bash
./scripts/install.sh
./scripts/install.sh --bin-dir "$HOME/.local/bin"
```

### 9.3 `scripts/install.sh` 동작
- 로컬 checkout이면 현재 tree 빌드
- checkout 밖이면 GitHub ref tarball 다운로드 후 빌드
- 기본 설치 경로: `$HOME/.local/bin`
- 설치 후 실행 검증
- 셸 PATH 도달성 검증 및 안내 출력

### 9.4 릴리스 흐름
1. `main` merge
2. local verify (`go test`, build, version)
3. annotated tag push (`vX.Y.Z`)
4. GitHub Release draft/publish
5. `release.published` → Homebrew workflow

### 9.5 Homebrew
- tap repo: `oozoofrog/homebrew-tap`
- 작업 대상: `Formula/xcodecli.rb` 한 파일만
- script: `scripts/release_homebrew.sh`
- 작업:
  - tag tarball download
  - sha256 계산
  - formula 생성/갱신
  - `brew audit --strict`
  - `brew install --build-from-source`
  - smoke test + `brew test`
  - dry-run / local commit / push 지원

### 9.6 CI
#### `.github/workflows/ci.yml`
트리거:
- push `main`
- push `codex/**`
- PR

작업:
- gofmt check
- `go test ./...`
- `./scripts/build.sh .tmp/xcodecli`

#### `.github/workflows/homebrew-release.yml`
트리거:
- `release.published`
- `workflow_dispatch`

필수 secret:
- `HOMEBREW_TAP_GITHUB_TOKEN`

---

## 10. 포팅 체크리스트

다른 언어로 재구현 시 반드시 유지해야 할 것:
1. macOS-only gate
2. bridge/serve/tools/tool/agent 명령 체계
3. stdout protocol-only 규칙 (`bridge`, `serve`)
4. session-id precedence 및 persistent file semantics
5. LaunchAgent 경로/label/layout
6. local Unix socket RPC schema와 method set
7. MCP initialize/tools/list/tools/call 계약
8. tool timeout 기본 분류표
9. `mcp config` client별 command generation 규칙
10. doctor / agent guide / agent demo / agent status JSON shape
11. build/install/release/Homebrew/CI 운영 흐름

---

## 11. 권장 포트 구조
- `bridge`
  - env resolution
  - session persistence
  - raw child-process passthrough
- `agent`
  - LaunchAgent equivalent lifecycle
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

이 모듈 구분은 현재 구현의 결합 구조, 테스트 구조, 운영 구조와 가장 잘 맞는다.
