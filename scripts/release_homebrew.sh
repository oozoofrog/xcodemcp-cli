#!/usr/bin/env bash
set -euo pipefail

SOURCE_REPO="oozoofrog/xcodecli"
export HOMEBREW_NO_AUTO_UPDATE=1
export HOMEBREW_NO_INSTALL_CLEANUP=1

TAP_REPO="oozoofrog/homebrew-tap"
FORMULA_NAME="xcodecli"
DEFAULT_CLONE_ROOT="${TMPDIR:-/tmp}"

usage() {
  cat <<USAGE
Usage:
  ./scripts/release_homebrew.sh <tag> [--tap-dir PATH] [--push] [--dry-run]

Examples:
  ./scripts/release_homebrew.sh v0.5.2 --tap-dir .tmp/homebrew-tap --dry-run
  ./scripts/release_homebrew.sh v0.5.2 --push

Behavior:
  - Downloads the GitHub source tarball for the given tag
  - Computes sha256 and writes Formula/xcodecli.rb in the shared tap repo
  - Runs Homebrew audit and build-from-source validation
  - Commits only the xcodecli formula change locally unless --dry-run is set
  - Pushes the tap commit only when --push is set

Environment:
  HOMEBREW_TAP_GITHUB_TOKEN  Optional. Used for cloning/pushing the tap over HTTPS.
USAGE
}

fail() {
  echo "[homebrew-release] $*" >&2
  exit 1
}

log() {
  echo "[homebrew-release] $*"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

clone_tap_repo() {
  local target_dir="$1"
  if [[ -n "${HOMEBREW_TAP_GITHUB_TOKEN:-}" ]]; then
    git clone "https://x-access-token:${HOMEBREW_TAP_GITHUB_TOKEN}@github.com/${TAP_REPO}.git" "$target_dir"
  elif command -v gh >/dev/null 2>&1; then
    gh repo clone "$TAP_REPO" "$target_dir"
  else
    git clone "git@github.com:${TAP_REPO}.git" "$target_dir"
  fi
}

ensure_git_identity() {
  local tap_dir="$1"
  if ! git -C "$tap_dir" config user.name >/dev/null; then
    git -C "$tap_dir" config user.name "github-actions[bot]"
  fi
  if ! git -C "$tap_dir" config user.email >/dev/null; then
    git -C "$tap_dir" config user.email "41898282+github-actions[bot]@users.noreply.github.com"
  fi
}

ensure_tap_repo_safe() {
  local tap_dir="$1"
  local formula_rel="Formula/${FORMULA_NAME}.rb"
  local status
  status="$(git -C "$tap_dir" status --short)"
  if [[ -z "$status" ]]; then
    return 0
  fi

  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    local path_part="${line:3}"
    if [[ "$path_part" != "$formula_rel" ]]; then
      fail "tap repo has unrelated local changes: $line"
    fi
  done <<< "$status"
}

render_formula() {
  local version="$1"
  local sha256="$2"
  cat <<FORMULA
class Xcodecli < Formula
  desc "macOS CLI wrapper around xcrun mcpbridge"
  homepage "https://github.com/${SOURCE_REPO}"
  url "https://github.com/${SOURCE_REPO}/archive/refs/tags/v${version}.tar.gz"
  sha256 "${sha256}"
  license "MIT"

  depends_on xcode: ["16.0", :build]
  depends_on :macos

  def install
    # Inject release version and channel into Version.swift before building
    version_file = "Sources/XcodeCLICore/Shared/Version.swift"
    inreplace version_file, 'public static let current: String = source',
                            "public static let current: String = \\"v#{version}\\""
    inreplace version_file, 'public static let buildChannel: String = "dev"',
                            'public static let buildChannel: String = "release"'
    system "swift", "build", "-c", "release", "--disable-sandbox"
    bin.install ".build/release/xcodecli"
  end

  test do
    assert_match "v#{version}", shell_output("#{bin}/xcodecli version")
  end
end
FORMULA
}

TAG=""
TAP_DIR=""
PUSH=0
DRY_RUN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tap-dir)
      [[ $# -ge 2 ]] || fail "--tap-dir requires a path"
      TAP_DIR="$2"
      shift 2
      ;;
    --push)
      PUSH=1
      shift
      ;;
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
VERSION="${TAG#v}"

require_cmd curl
require_cmd git
require_cmd shasum
require_cmd brew

TAP_NAME="oozoofrog/tap"
AUTO_CLONED=0
if [[ -z "$TAP_DIR" ]]; then
  TAP_DIR="$(mktemp -d "${DEFAULT_CLONE_ROOT%/}/xcodecli-homebrew-tap-XXXXXX")"
  AUTO_CLONED=1
  log "cloning ${TAP_REPO} into ${TAP_DIR}"
  clone_tap_repo "$TAP_DIR"
fi

[[ -d "$TAP_DIR/.git" ]] || fail "tap directory is not a git repository: $TAP_DIR"
ensure_tap_repo_safe "$TAP_DIR"
mkdir -p "$TAP_DIR/Formula"
FORMULA_PATH="$TAP_DIR/Formula/${FORMULA_NAME}.rb"
TARBALL_URL="https://github.com/${SOURCE_REPO}/archive/refs/tags/${TAG}.tar.gz"

log "computing sha256 for ${TARBALL_URL}"
SHA256="$(curl -fsSL "$TARBALL_URL" | shasum -a 256 | awk '{print $1}')"
[[ -n "$SHA256" ]] || fail "failed to compute sha256"

log "writing ${FORMULA_PATH} inside shared tap repo ${TAP_DIR}"
render_formula "$VERSION" "$SHA256" > "$FORMULA_PATH"

VALIDATION_TAP_ADDED=0
if ! brew tap | grep -qx "$TAP_NAME"; then
  log "tapping ${TAP_NAME} for validation"
  brew tap "$TAP_NAME"
  VALIDATION_TAP_ADDED=1
fi

VALIDATION_TAP_REPO="$(brew --repo "$TAP_NAME")"
VALIDATION_FORMULA_DIR="$VALIDATION_TAP_REPO/Formula"
log "validation tap repo: ${VALIDATION_TAP_REPO}"
VALIDATION_FORMULA_PATH="$VALIDATION_FORMULA_DIR/${FORMULA_NAME}.rb"
BACKUP_FORMULA_PATH=""
mkdir -p "$VALIDATION_FORMULA_DIR"
if [[ -f "$VALIDATION_FORMULA_PATH" ]]; then
  BACKUP_FORMULA_PATH="$(mktemp "${DEFAULT_CLONE_ROOT%/}/xcodecli-formula-backup-XXXXXX.rb")"
  cp "$VALIDATION_FORMULA_PATH" "$BACKUP_FORMULA_PATH"
fi
log "temporarily validating only ${VALIDATION_FORMULA_PATH}"
cp "$FORMULA_PATH" "$VALIDATION_FORMULA_PATH"

cleanup_validation_tap() {
  log "restoring validation copy of ${VALIDATION_FORMULA_PATH}"
  if [[ -n "$BACKUP_FORMULA_PATH" && -f "$BACKUP_FORMULA_PATH" ]]; then
    cp "$BACKUP_FORMULA_PATH" "$VALIDATION_FORMULA_PATH"
    rm -f "$BACKUP_FORMULA_PATH"
  else
    rm -f "$VALIDATION_FORMULA_PATH"
  fi
  if [[ "$VALIDATION_TAP_ADDED" -eq 1 ]]; then
    brew untap "$TAP_NAME" >/dev/null 2>&1 || true
  fi
}
trap cleanup_validation_tap EXIT

log "running brew audit"
brew audit --strict "$TAP_NAME/$FORMULA_NAME"

log "running brew install --build-from-source"
WAS_INSTALLED=0
if brew list --formula "$FORMULA_NAME" >/dev/null 2>&1; then
  WAS_INSTALLED=1
  brew reinstall --build-from-source "$TAP_NAME/$FORMULA_NAME"
else
  brew install --build-from-source "$TAP_NAME/$FORMULA_NAME"
fi

log "running formula smoke test"
"$(brew --prefix)/bin/${FORMULA_NAME}" --help >/dev/null
version_output="$("$(brew --prefix)/bin/${FORMULA_NAME}" version | tr -d '\r')"
[[ "$version_output" == "${FORMULA_NAME} ${TAG}" ]] || fail "unexpected version output from Homebrew install: ${version_output}"
brew test "$TAP_NAME/$FORMULA_NAME"

if [[ "$WAS_INSTALLED" -eq 0 ]]; then
  log "cleaning up temporary Homebrew install"
  brew uninstall --force "$FORMULA_NAME"
fi

if [[ -z "$(git -C "$TAP_DIR" status --short -- "Formula/${FORMULA_NAME}.rb")" ]]; then
  log "formula already up to date"
else
  if [[ "$DRY_RUN" -eq 1 ]]; then
    log "dry-run enabled; leaving formula changes uncommitted in ${TAP_DIR}"
  else
    ensure_git_identity "$TAP_DIR"
    ensure_tap_repo_safe "$TAP_DIR"
    git -C "$TAP_DIR" add "Formula/${FORMULA_NAME}.rb"
    git -C "$TAP_DIR" commit -m "${FORMULA_NAME} ${VERSION}"
    if [[ "$PUSH" -eq 1 ]]; then
      log "rebasing tap branch before push"
      git -C "$TAP_DIR" pull --rebase origin main
      log "pushing tap update"
      git -C "$TAP_DIR" push origin HEAD:main
    else
      log "tap commit created locally (not pushed)"
    fi
  fi
fi

log "done"
log "tap directory: ${TAP_DIR}"
if [[ "$AUTO_CLONED" -eq 1 && "$DRY_RUN" -eq 1 ]]; then
  log "auto-cloned tap directory was kept for inspection because --dry-run was used"
fi
