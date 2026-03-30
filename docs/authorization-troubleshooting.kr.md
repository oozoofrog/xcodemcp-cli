# 인증 / 세션 트러블슈팅

이 문서는 `xcodecli`가 Xcode MCP 인증을 어떻게 재사용하는지, "같은 세션"이 정확히 무엇인지, 그리고 왜 Xcode가 다시 인증을 요구할 수 있는지를 빠르게 설명합니다.

> English version: [`authorization-troubleshooting.md`](./authorization-troubleshooting.md)

## 한눈에 보기

- 반복 인증은 보통 같은 pooled session key `{XcodePID, SessionID, DeveloperDir}`를 유지할 때만 가장 잘 줄어듭니다.
- 가장 안전한 기본 운영 경로는: stable installed `xcodecli` binary 1개, default agent mode, default persistent session ID, 불필요한 `MCP_XCODE_PID` / `DEVELOPER_DIR` override 없음입니다.
- 인증이 다시 자주 뜨기 시작하면, agent runtime을 리셋하기 전에 먼저 `doctor --json`과 `agent status --json`부터 확인하세요.

## 빠른 FAQ

### 한 번 Xcode에서 허용하면 이 머신에서는 영구적으로 `mcpbridge`를 쓸 수 있나요?

아니요.

실제로는 인증 재사용을 **하나의 pooled session 안에서만 best-effort로 기대**해야 합니다.

`xcodecli`는 backend `mcpbridge` 프로세스를 아래 pooled session key 기준으로 재사용합니다.

```text
{ XcodePID, SessionID, DeveloperDir }
```

이 키가 바뀌면 다음 요청은 fresh backend session으로 갈 수 있고, Xcode가 다시 인증 프롬프트를 띄울 수 있습니다.

### "같은 세션"은 정확히 무엇인가요?

다음과 같을 때 보통 같은 pooled session으로 봅니다.

- 같은 Xcode 프로세스 (`XcodePID`)
- 같은 session ID (`SessionID`)
- 같은 toolchain / developer directory (`DeveloperDir`)

즉:
- 같은 터미널 창이라고 해서 같은 세션이 아닙니다
- 다른 터미널 창이라고 해서 자동으로 다른 세션도 아닙니다

서로 다른 셸에서도 위 3개 값이 같으면 같은 pooled session을 계속 사용할 수 있습니다.

### 반복 인증을 가장 적게 만들려면 어떻게 해야 하나요?

가장 안전한 운영 방식은 다음 조합입니다.

1. stable installed `xcodecli` path 하나만 계속 사용
2. 기본 agent mode (`xcodecli serve` / `mcp config`) 사용
3. `~/Library/Application Support/xcodecli/session-id` 의 기본 persistent session ID 재사용
4. `MCP_XCODE_PID`, `DEVELOPER_DIR`를 불필요하게 바꾸지 않기

### 어떤 경우에 fresh authorization prompt가 다시 뜰 수 있나요?

대표적인 원인:
- 다른 `xcodecli` binary / checkout path로 바꾸어 실행
- 다른 `--session-id` 사용
- `MCP_XCODE_PID` 변경
- `DEVELOPER_DIR` 변경
- `agent stop` 실행
- `agent uninstall` 실행
- Xcode 재실행으로 PID 변경

### 다른 `--session-id`를 써도 운 좋게 인증창이 안 뜰 수 있나요?

그럴 수는 있습니다. 하지만 운영 규칙상 **다른 `--session-id`는 새 backend session으로 취급해야 한다**고 보는 것이 맞습니다.  
즉, prompt-free 재사용을 기대하면 안 됩니다.

### 새 터미널 창을 열면 자동으로 새로운 인증 세션이 되나요?

아니요.

다음이 그대로면 새 터미널이어도 같은 pooled session을 재사용할 수 있습니다.
- 같은 installed `xcodecli` path
- 같은 persistent default session ID
- 같은 Xcode 인스턴스
- 같은 `DEVELOPER_DIR`

## 권장 운영 규칙

### 평소 사용

- `/opt/homebrew/bin/xcodecli` 또는 `~/.local/bin/xcodecli` 같이 stable installed path를 계속 사용
- raw bridge mode보다 `mcp config` 기본 agent mode 우선
- 기본 persistent session ID 그대로 사용
- `MCP_XCODE_PID`, `DEVELOPER_DIR` 변경 최소화
- `agent stop` / `agent uninstall`는 트러블슈팅 때만 사용

### 의도적으로 분리된 세션이 필요할 때

아래를 바꾸면 새 backend session을 강제로 만들 수 있습니다.

- 다른 `--session-id`
- 다른 `MCP_XCODE_PID`
- 다른 `DEVELOPER_DIR`

단, 이 경우에는 Xcode가 다시 인증을 요구할 수 있다고 가정해야 합니다.

## 반복 인증이 발생할 때 점검 순서

### 1. stable binary를 계속 쓰고 있는지 확인

```bash
./xcodecli doctor --json
./xcodecli agent status --json
```

다음 warning을 확인하세요.
- `LaunchAgent binary registration`
- `effective MCP_XCODE_PID`
- `effective DEVELOPER_DIR`

등록된 binary path와 현재 binary path가 다르면 LaunchAgent backend가 recycle되면서 인증이 다시 뜰 수 있습니다.

### 2. pooled session key를 바꿨는지 확인

아래 질문에 하나라도 Yes면 fresh backend session일 가능성이 있습니다.

- Xcode를 재실행했는가?
- `--session-id`를 줬는가?
- `MCP_XCODE_PID`를 export 했는가?
- `DEVELOPER_DIR`를 export 했는가?
- 다른 `xcodecli` binary로 바꿨는가?

### 3. 기본 persistent session ID를 재사용하고 있는지 확인

기본 위치:

```text
~/Library/Application Support/xcodecli/session-id
```

의도적인 격리가 아니라면 이 값을 override하지 않는 것이 가장 안전합니다.

### 4. 불필요한 agent reset을 피하기

다음 명령은 warm backend session을 버립니다.

```bash
./xcodecli agent stop
./xcodecli agent uninstall
```

정상 운영 중에는 피하고, 복구가 정말 필요할 때만 사용하세요.

### 5. 필요하면 stable path에서 다시 등록

`mcp config`가 현재 executable path가 불안정하다고 경고하면,
먼저 stable location에 설치한 뒤 그 경로에서 MCP 등록을 다시 생성하세요.

## 최소 검증 체크리스트

반복 인증 재사용 가능성을 점검하고 싶다면:

1. read-only tool을 1회 호출
2. 새 셸에서 다른 read-only tool을 다시 호출
3. 같은 default session ID 유지
4. `MCP_XCODE_PID`, `DEVELOPER_DIR`, binary path를 바꾸지 않기

예시:

```bash
./xcodecli tool call XcodeListWindows --json '{}'
./xcodecli tool call XcodeRead --json '{"tabIdentifier":"windowtab1","filePath":"Project/App.swift","limit":5}'
```

의도적으로 새 세션 비교를 하고 싶다면:

```bash
./xcodecli tool call XcodeListWindows --session-id "$(uuidgen | tr '[:upper:]' '[:lower:]')" --json '{}'
```

이 마지막 호출은 fresh backend session 테스트로 봐야 합니다.

## 검증 시나리오 표

| 시나리오 | 예시 변경 | 예상 pooled-session 결과 | 인증 프롬프트 리스크 |
| --- | --- | --- | --- |
| 새 셸이지만 기본 세션 유지 | 같은 installed binary, 같은 Xcode, 같은 default session ID, 같은 `DEVELOPER_DIR` | 보통 같은 warm backend session 유지 | 낮음 |
| 다른 `--session-id` 사용 | 새 UUID 전달 | fresh backend session | 중간~높음 |
| 다른 `MCP_XCODE_PID` 사용 | 다른 Xcode PID 지정 | fresh backend session | 중간~높음 |
| 다른 `DEVELOPER_DIR` 사용 | toolchain override | fresh backend session | 중간~높음 |
| 다른 binary path 사용 | checkout/install path 전환 | LaunchAgent/backend recycle 가능성 큼 | 중간~높음 |
| `agent stop` / `agent uninstall` 후 다음 호출 | warm runtime 초기화 | 다음 요청에서 fresh backend session | 중간~높음 |

이 표는 운영 기대치를 정리한 가이드입니다. 실제 Xcode authorization 동작은 플랫폼 상태에 따라 달라질 수 있으므로 절대 보장으로 해석하면 안 됩니다.
