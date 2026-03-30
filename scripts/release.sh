#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SWIFT_VERSION_FILE="${ROOT_DIR}/Sources/XcodeCLICore/Shared/Version.swift"
GO_VERSION_FILE="${ROOT_DIR}/cmd/xcodecli/version.go"
SOURCE_REPO="oozoofrog/xcodecli"

TAG=""
DRY_RUN=0
BACKUP_DIR=""
FILES_BUMPED=0
LOCAL_COMMIT_CREATED=0
LOCAL_TAG_CREATED=0
REMOTE_MUTATED=0
CURRENT_VERSION=""

usage() {
  cat <<USAGE
Usage:
  ./scripts/release.sh <tag> [--dry-run]

Examples:
  ./scripts/release.sh v1.2.1 --dry-run
  ./scripts/release.sh v1.2.1

Behavior:
  - Bumps the source version in Swift and Go
  - Runs local verification commands
  - Creates and atomically pushes a release commit and annotated tag to the official origin
  - Updates the shared Homebrew tap locally via scripts/release_homebrew.sh
  - Creates a GitHub Release with generated notes

Notes:
  - Releases must be cut from a clean tracked working tree on main
  - origin must point to ${SOURCE_REPO}, and HEAD must exactly match origin/main before the version bump
  - If a failure happens before any push, local version changes are rolled back automatically
  - After the atomic main+tag push, follow the printed recovery commands instead of expecting an auto-rollback
USAGE
}

fail() {
  echo "[release] $*" >&2
  exit 1
}

log() {
  echo "[release] $*"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

restore_bumped_files() {
  if [[ -n "$BACKUP_DIR" && -d "$BACKUP_DIR" ]]; then
    cp "${BACKUP_DIR}/Version.swift" "$SWIFT_VERSION_FILE"
    cp "${BACKUP_DIR}/version.go" "$GO_VERSION_FILE"
  fi
}

print_remote_recovery() {
  cat >&2 <<RECOVERY
[release] release failed after the atomic remote push completed.
[release] manual recovery:
[release]   - inspect remote state: git ls-remote --heads --tags origin main "refs/tags/${TAG}"
[release]   - rerun Homebrew sync: ./scripts/release_homebrew.sh ${TAG} --push
[release]   - create the GitHub Release manually if needed: gh release create ${TAG} --verify-tag --generate-notes
[release]   - if the release should be withdrawn, revert the pushed release commit and delete the remote tag together with explicit git commands
RECOVERY
}

cleanup() {
  local status=$?
  trap - EXIT

  if [[ $status -ne 0 && "$DRY_RUN" -eq 0 ]]; then
    if [[ "$REMOTE_MUTATED" -eq 0 ]]; then
      if [[ "$LOCAL_TAG_CREATED" -eq 1 ]]; then
        git -C "$ROOT_DIR" tag -d "$TAG" >/dev/null 2>&1 || true
      fi
      if [[ "$LOCAL_COMMIT_CREATED" -eq 1 ]]; then
        git -C "$ROOT_DIR" reset --hard HEAD~1 >/dev/null 2>&1 || true
      elif [[ "$FILES_BUMPED" -eq 1 ]]; then
        restore_bumped_files
      fi
      echo "[release] restored local release state because the failure happened before any push" >&2
    else
      print_remote_recovery
    fi
  fi

  if [[ -n "$BACKUP_DIR" && -d "$BACKUP_DIR" ]]; then
    rm -rf "$BACKUP_DIR"
  fi

  exit "$status"
}
trap cleanup EXIT

run_cmd() {
  local description="$1"
  shift
  log "$description"
  "$@"
}

extract_swift_version() {
  sed -n 's/^.*public static let source = "\(.*\)".*$/\1/p' "$SWIFT_VERSION_FILE" | head -n 1
}

extract_go_version() {
  sed -n 's/^const sourceVersion = "\(.*\)"$/\1/p' "$GO_VERSION_FILE" | head -n 1
}

normalize_github_slug() {
  python3 - "$1" <<'PY'
import sys

url = sys.argv[1].strip()
prefixes = [
    "git@github.com:",
    "ssh://git@github.com/",
    "https://github.com/",
    "http://github.com/",
    "git://github.com/",
]

slug = None
for prefix in prefixes:
    if url.startswith(prefix):
        slug = url[len(prefix):]
        break

if not slug:
    raise SystemExit(1)

slug = slug.strip("/")
if slug.endswith(".git"):
    slug = slug[:-4]
parts = [part for part in slug.split("/") if part]
if len(parts) != 2:
    raise SystemExit(1)

print("/".join(parts))
PY
}

ensure_clean_tracked_tree() {
  local status
  status="$(git -C "$ROOT_DIR" status --porcelain --untracked-files=no)"
  [[ -z "$status" ]] || fail "tracked working tree must be clean before releasing"
}

ensure_main_branch() {
  local branch
  branch="$(git -C "$ROOT_DIR" symbolic-ref --quiet --short HEAD || true)"
  [[ -n "$branch" ]] || fail "releases must be created from a branch checkout, not detached HEAD"
  [[ "$branch" == "main" ]] || fail "releases must be cut from main (current branch: ${branch})"
}

ensure_official_origin_remote() {
  local origin_url origin_slug
  origin_url="$(git -C "$ROOT_DIR" config --get remote.origin.url 2>/dev/null || true)"
  [[ -n "$origin_url" ]] || fail "git remote 'origin' is required"

  origin_slug="$(normalize_github_slug "$origin_url" 2>/dev/null || true)"
  [[ -n "$origin_slug" ]] || fail "origin must point to ${SOURCE_REPO}; unsupported GitHub URL: ${origin_url}"
  [[ "$origin_slug" == "$SOURCE_REPO" ]] || fail "origin must point to ${SOURCE_REPO} before release (current: ${origin_url})"
}

fetch_release_refs() {
  log "fetching origin/main and tags"
  git -C "$ROOT_DIR" fetch --quiet origin main --tags
}

ensure_head_matches_origin_main() {
  local head_sha origin_sha behind ahead
  git -C "$ROOT_DIR" rev-parse --verify --quiet refs/remotes/origin/main >/dev/null || fail "origin/main is unavailable after fetch"

  head_sha="$(git -C "$ROOT_DIR" rev-parse HEAD)"
  origin_sha="$(git -C "$ROOT_DIR" rev-parse refs/remotes/origin/main)"
  read -r behind ahead <<<"$(git -C "$ROOT_DIR" rev-list --left-right --count refs/remotes/origin/main...HEAD)"

  [[ "$head_sha" == "$origin_sha" ]] || fail "HEAD must exactly match origin/main before release (behind=${behind}, ahead=${ahead}); fetch/rebase or drop unpublished local commits first"
}

ensure_tag_not_present() {
  git -C "$ROOT_DIR" rev-parse --verify --quiet "refs/tags/${TAG}" >/dev/null && fail "tag already exists locally: ${TAG}"

  local remote_tag
  remote_tag="$(git -C "$ROOT_DIR" ls-remote --refs --tags origin "refs/tags/${TAG}" 2>/dev/null)" || fail "failed to query origin tags"
  [[ -z "$remote_tag" ]] || fail "tag already exists on origin: ${TAG}"
}

ensure_gh_auth() {
  gh auth status -h github.com >/dev/null 2>&1 || fail "gh CLI must be authenticated for GitHub release creation"
}

ensure_git_identity() {
  local name email
  name="$(git -C "$ROOT_DIR" config user.name || true)"
  email="$(git -C "$ROOT_DIR" config user.email || true)"
  [[ -n "$name" ]] || fail "git user.name is required to create the release commit"
  [[ -n "$email" ]] || fail "git user.email is required to create the release commit"
}

prepare_backups() {
  BACKUP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/xcodecli-release-XXXXXX")"
  cp "$SWIFT_VERSION_FILE" "${BACKUP_DIR}/Version.swift"
  cp "$GO_VERSION_FILE" "${BACKUP_DIR}/version.go"
}

bump_versions() {
  local swift_version go_version
  swift_version="$(extract_swift_version)"
  go_version="$(extract_go_version)"
  [[ -n "$swift_version" ]] || fail "failed to read Swift source version"
  [[ -n "$go_version" ]] || fail "failed to read Go source version"
  [[ "$swift_version" == "$go_version" ]] || fail "Swift and Go source versions differ before release: ${swift_version} vs ${go_version}"
  [[ "$swift_version" != "$TAG" ]] || fail "source version already matches ${TAG}; choose a new release tag"

  CURRENT_VERSION="$swift_version"
  prepare_backups

  python3 - "$GO_VERSION_FILE" "$SWIFT_VERSION_FILE" "$TAG" <<'PY'
from pathlib import Path
import re
import sys

go_path = Path(sys.argv[1])
swift_path = Path(sys.argv[2])
tag = sys.argv[3]

go_text = go_path.read_text()
swift_text = swift_path.read_text()

go_text, go_count = re.subn(r'^const sourceVersion = ".*"$', f'const sourceVersion = "{tag}"', go_text, count=1, flags=re.MULTILINE)
swift_text, swift_count = re.subn(r'^    public static let source = ".*"$', f'    public static let source = "{tag}"', swift_text, count=1, flags=re.MULTILINE)

if go_count != 1:
    raise SystemExit(f'failed to update Go version file: {go_path}')
if swift_count != 1:
    raise SystemExit(f'failed to update Swift version file: {swift_path}')

go_path.write_text(go_text)
swift_path.write_text(swift_text)
PY

  FILES_BUMPED=1
}

print_dry_run_plan() {
  cat <<PLAN
[release] dry-run only; no files were modified.
[release] current version: ${CURRENT_VERSION}
[release] target version:  ${TAG}
[release] planned commands:
[release]   git fetch origin main --tags
[release]   bash ./scripts/check-version-sync.sh
[release]   go test ./...
[release]   swift test
[release]   ./scripts/build-swift.sh .tmp/xcodecli
[release]   cp .tmp/xcodecli /tmp/xcodecli && /tmp/xcodecli version
[release]   git add Sources/XcodeCLICore/Shared/Version.swift cmd/xcodecli/version.go
[release]   git commit -m 'Bump version to ${TAG}'
[release]   git tag -a '${TAG}' -m 'Release ${TAG}'
[release]   git push --atomic origin main '${TAG}'
[release]   ./scripts/release_homebrew.sh ${TAG} --push
[release]   gh release create ${TAG} --verify-tag --generate-notes
PLAN
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -* )
      fail "unknown flag: $1"
      ;;
    *)
      if [[ -n "$TAG" ]]; then
        fail "multiple tags provided"
      fi
      TAG="$1"
      shift
      ;;
  esac
done

[[ -n "$TAG" ]] || { usage; fail "missing required tag"; }
[[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "tag must look like vMAJOR.MINOR.PATCH"

require_cmd git
require_cmd gh
require_cmd swift
require_cmd go
require_cmd brew
require_cmd curl
require_cmd python3

ensure_official_origin_remote
ensure_main_branch
ensure_clean_tracked_tree
fetch_release_refs
ensure_head_matches_origin_main
ensure_tag_not_present
CURRENT_VERSION="$(extract_swift_version)"
[[ -n "$CURRENT_VERSION" ]] || fail "failed to read current source version"

if [[ "$DRY_RUN" -eq 1 ]]; then
  CURRENT_GO_VERSION="$(extract_go_version)"
  [[ "$CURRENT_VERSION" == "$CURRENT_GO_VERSION" ]] || fail "Swift and Go source versions differ before release: ${CURRENT_VERSION} vs ${CURRENT_GO_VERSION}"
  [[ "$CURRENT_VERSION" != "$TAG" ]] || fail "source version already matches ${TAG}; choose a new release tag"
  print_dry_run_plan
  exit 0
fi

ensure_gh_auth
ensure_git_identity
cd "$ROOT_DIR"

bump_versions
run_cmd "checking version sync" bash ./scripts/check-version-sync.sh
run_cmd "running Go tests" go test ./...
run_cmd "running Swift tests" swift test
run_cmd "building release binary" ./scripts/build-swift.sh .tmp/xcodecli
run_cmd "verifying release binary via /tmp" bash -lc 'cp .tmp/xcodecli /tmp/xcodecli && /tmp/xcodecli version'

run_cmd "creating release commit" git add Sources/XcodeCLICore/Shared/Version.swift cmd/xcodecli/version.go
run_cmd "committing source version bump" git commit -m "Bump version to ${TAG}"
LOCAL_COMMIT_CREATED=1
run_cmd "creating annotated tag ${TAG}" git tag -a "${TAG}" -m "Release ${TAG}"
LOCAL_TAG_CREATED=1

run_cmd "atomically pushing main and tag ${TAG}" git push --atomic origin main "${TAG}"
REMOTE_MUTATED=1
run_cmd "updating shared Homebrew tap" ./scripts/release_homebrew.sh "${TAG}" --push
run_cmd "creating GitHub Release" gh release create "${TAG}" --verify-tag --generate-notes

log "release completed"
log "from ${CURRENT_VERSION} to ${TAG}"
