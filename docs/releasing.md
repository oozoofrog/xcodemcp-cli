# Releasing xcodecli

`xcodecli` 릴리스는 GitHub Actions가 아니라 macOS 로컬 환경에서 진행합니다.

## Standard local release flow

1. `origin` 이 `oozoofrog/xcodecli` 를 가리키고, `main` 브랜치의 tracked working tree 가 깨끗한지 확인합니다.
2. 필수 도구가 준비되어 있는지 확인합니다: `git`, `gh`, `swift`, `go`, `brew`, `curl`.
3. `gh auth status -h github.com` 이 통과하고, `git user.name` / `git user.email` 이 설정되어 있는지 확인합니다.
4. `git fetch origin main --tags` 후 `HEAD == origin/main` 인지 확인합니다. 로컬 `main` 이 ahead/behind/diverged 상태면 릴리스를 중단해야 합니다.
5. 릴리스 계획을 먼저 확인합니다:
   ```bash
   ./scripts/release.sh v1.2.1 --dry-run
   ```
6. 실제 릴리스를 실행합니다:
   ```bash
   ./scripts/release.sh v1.2.1
   ```

`./scripts/release.sh <tag>` 는 아래 순서로 작업합니다.

- 공식 `origin` 과 `origin/main` 동기화 상태 검증
- `Sources/XcodeCLICore/Shared/Version.swift` 와 `cmd/xcodecli/version.go` 버전 범프
- `bash ./scripts/check-version-sync.sh`
- `go test ./...`
- `swift test`
- `./scripts/build-swift.sh .tmp/xcodecli`
- `cp .tmp/xcodecli /tmp/xcodecli && /tmp/xcodecli version`
- `git commit -m "Bump version to vX.Y.Z"`
- annotated tag 생성 + `git push --atomic origin main vX.Y.Z`
- `./scripts/release_homebrew.sh vX.Y.Z --push` 로 shared tap 반영
- `gh release create vX.Y.Z --verify-tag --generate-notes`

실패 시 규칙:
- 원격 push 전에 실패하면 로컬 version 변경은 자동으로 되돌립니다.
- atomic main+tag push 가 끝난 뒤 실패하면 자동 롤백하지 않고, 스크립트가 복구 명령을 출력합니다.

## Low-level Homebrew dry-run / recovery

릴리스 전체 대신 tap 반영만 미리 확인하거나 복구하려면 `scripts/release_homebrew.sh` 를 직접 사용합니다.

```bash
./scripts/release_homebrew.sh v1.2.1 --tap-dir .tmp/homebrew-tap --dry-run
```

로컬 tap clone 안에 커밋만 만들고 push 하지 않으려면:

```bash
./scripts/release_homebrew.sh v1.2.1 --tap-dir .tmp/homebrew-tap
```

자동 clone + push 까지 하려면:

```bash
./scripts/release_homebrew.sh v1.2.1 --push
```

선택 사항: HTTPS 인증이 필요하면 토큰을 사용합니다.

```bash
HOMEBREW_TAP_GITHUB_TOKEN=... ./scripts/release_homebrew.sh v1.2.1 --push
```

토큰이 없으면 스크립트는 `gh repo clone` 또는 SSH git remote 를 사용합니다.

## Shared tap safety rules

- `oozoofrog/homebrew-tap` 은 여러 프로젝트가 공유하는 저장소로 취급합니다.
- `xcodecli` 릴리스는 `Formula/xcodecli.rb` 한 파일만 수정해야 합니다.
- tap clone 에 관련 없는 로컬 변경이 있으면 스크립트는 중단해야 합니다.
- tap 커밋을 만들기 전에 `git user.name` 과 `git user.email` 이 설정되어 있어야 합니다.
- 문제가 생기면 `./scripts/release_homebrew.sh <tag> --tap-dir <tap clone> --dry-run` 으로 다시 검증하고 `Formula/xcodecli.rb` diff 만 확인합니다.
