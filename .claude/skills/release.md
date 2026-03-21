---
name: release
description: "xcodecli 버전 업데이트 및 Homebrew 배포. '릴리스', 'release', '버전 범프', 'version bump', '배포', 'homebrew 업데이트', '새 버전', 'publish', '태그 생성' 등 릴리스 관련 요청 시 반드시 사용. 사용자가 '0.6.0으로 올려줘' 같은 요청을 할 때도 활성화."
---

# xcodecli Release Skill

이 스킬은 xcodecli의 버전 업데이트부터 Homebrew 배포까지 전체 릴리스 파이프라인을 안내합니다.

## 핵심 원칙

**CLAUDE.md 규칙**: 파괴적 작업은 마지막에, dry-run/check를 먼저 실행합니다. 이 순서를 절대 변경하지 마세요.

## 릴리스 워크플로우

### Phase 1: 버전 범프

1. `Sources/XcodeCLICore/Shared/Version.swift` 의 `source` 상수를 새 버전으로 수정:
   ```swift
   public static let source = "v{NEW_VERSION}"
   ```

2. 커밋:
   ```bash
   git add Sources/XcodeCLICore/Shared/Version.swift
   git commit -m "Bump version to v{NEW_VERSION}"
   ```

### Phase 2: 검증 (파괴적 작업 전에 반드시 실행)

```bash
swift test --no-parallel          # 전체 테스트
swift build                       # 빌드 확인
./scripts/build-swift.sh .tmp/xcodecli  # 릴리스 빌드
.tmp/xcodecli version             # 버전 출력 확인
```

모든 검증이 통과해야 다음 단계로 진행합니다. 하나라도 실패하면 멈추고 원인을 파악하세요.

### Phase 3: Homebrew dry-run (선택, 강력 권장)

```bash
./scripts/release_homebrew.sh v{NEW_VERSION} --tap-dir .tmp/homebrew-tap --dry-run
```

이 단계는 실제 push 없이 formula 생성, `brew audit --strict`, 소스 빌드, 버전 검증을 모두 수행합니다. 실패하면 릴리스하지 마세요.

### Phase 4: 태그 & 릴리스 (파괴적 — 확인 후 실행)

사용자에게 반드시 확인을 받은 후 실행합니다:

```bash
git push origin main              # main 브랜치 push
git tag -a v{NEW_VERSION} -m "Release v{NEW_VERSION}"
git push origin v{NEW_VERSION}    # 태그 push
```

그 후 GitHub Release를 생성합니다:
```bash
gh release create v{NEW_VERSION} --title "v{NEW_VERSION}" --generate-notes
```

### Phase 5: Homebrew 자동 배포

GitHub Release 발행 시 `.github/workflows/homebrew-release.yml` 워크플로우가 자동 실행됩니다:
- 소스 tarball 다운로드 → SHA256 계산
- `Formula/xcodecli.rb` 생성 (version/channel injection via `inreplace`)
- `brew audit --strict` + 소스 빌드 + 스모크 테스트
- `oozoofrog/homebrew-tap` 에 push

워크플로우 실행 상태를 확인합니다:
```bash
gh run list --workflow=homebrew-release.yml --limit 3
```

### Phase 6: 배포 확인

```bash
brew update
brew info oozoofrog/tap/xcodecli  # 새 버전 확인
```

## 수동 Homebrew 복구

CI 워크플로우가 실패한 경우 수동으로 실행할 수 있습니다:
```bash
./scripts/release_homebrew.sh v{NEW_VERSION} --push
```

또는 `workflow_dispatch`로 재실행:
```bash
gh workflow run homebrew-release.yml -f release_tag=v{NEW_VERSION}
```

## 핵심 파일

| 파일 | 역할 |
|------|------|
| `Sources/XcodeCLICore/Shared/Version.swift` | 버전 상수 (`source`) |
| `scripts/build-swift.sh` | 릴리스 빌드 (version/channel injection) |
| `scripts/release_homebrew.sh` | Homebrew formula 생성/검증/push |
| `.github/workflows/homebrew-release.yml` | CI 자동 배포 |
| `docs/releasing.md` | 릴리스 문서 |

## 안전 규칙

- `oozoofrog/homebrew-tap`은 공유 탭 — `Formula/xcodecli.rb`만 수정 가능
- 태그 형식: `vMAJOR.MINOR.PATCH` (예: `v0.6.0`)
- `--dry-run`을 먼저 실행하지 않고 `--push`를 실행하지 마세요
- GitHub Release 생성 전에 모든 테스트가 통과해야 합니다
- `HOMEBREW_TAP_GITHUB_TOKEN` 시크릿이 설정되어 있어야 CI가 동작합니다
