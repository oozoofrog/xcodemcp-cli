#!/usr/bin/env bash
set -euo pipefail

SOURCE_REPO="oozoofrog/xcodecli"
DEFAULT_REF="main"

usage() {
  cat <<'USAGE'
Usage:
  ./scripts/install.sh [--bin-dir PATH] [--ref REF]
  curl -fsSL https://raw.githubusercontent.com/oozoofrog/xcodecli/main/scripts/install.sh | bash
  curl -fsSL https://raw.githubusercontent.com/oozoofrog/xcodecli/main/scripts/install.sh | bash -s -- --ref v0.3.0

Options:
  --bin-dir PATH   Install directory for the xcodecli binary (default: $HOME/.local/bin)
  --ref REF        GitHub branch or tag to install when running outside a local checkout (default: main)
  -h, --help       Show help

Environment:
  BIN_DIR          Same as --bin-dir
  XCODECLI_REF     Same as --ref

Behavior:
  - When run inside a local checkout and no --ref is provided, builds from the current working tree.
  - When run outside a checkout (for example via curl | bash), downloads the requested GitHub ref and builds from source.
  - Requires macOS and Go to be installed.
USAGE
}

log() {
  echo "[install] $*"
}

fail() {
  echo "[install] $*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

resolve_script_dir() {
  local source_path="${BASH_SOURCE[0]:-}"
  [[ -n "$source_path" ]] || return 1
  cd "$(dirname "$source_path")" >/dev/null 2>&1 && pwd
}

resolve_archive_url() {
  local ref="$1"
  local repo_url="https://github.com/${SOURCE_REPO}.git"
  if git ls-remote --exit-code --heads "$repo_url" "$ref" >/dev/null 2>&1; then
    echo "https://github.com/${SOURCE_REPO}/archive/refs/heads/${ref}.tar.gz"
    return 0
  fi
  if git ls-remote --exit-code --tags "$repo_url" "refs/tags/${ref}" >/dev/null 2>&1; then
    echo "https://github.com/${SOURCE_REPO}/archive/refs/tags/${ref}.tar.gz"
    return 0
  fi
  fail "could not resolve ref '${ref}' as a branch or tag in ${SOURCE_REPO}"
}

warn_if_not_on_path() {
  local dir="$1"
  case ":${PATH:-}:" in
    *":${dir}:"*) ;;
    *)
      log "warning: ${dir} is not currently on PATH"
      log "add this to your shell profile if needed:"
      log "  export PATH=\"${dir}:\$PATH\""
      ;;
  esac
}

resolve_path() {
  local path="$1"
  [[ -n "$path" ]] || return 1
  if [[ -d "$path" ]]; then
    (cd "$path" >/dev/null 2>&1 && pwd -P)
    return 0
  fi
  local dir
  dir="$(cd "$(dirname "$path")" >/dev/null 2>&1 && pwd -P)" || return 1
  printf '%s/%s\n' "$dir" "$(basename "$path")"
}

detect_preferred_shell() {
  local shell_path="${SHELL:-}"
  [[ -n "$shell_path" ]] || return 1
  if [[ ! -x "$shell_path" ]]; then
    shell_path="$(command -v "$(basename "$shell_path")" 2>/dev/null || true)"
  fi
  [[ -x "$shell_path" ]] || return 1
  printf '%s\n' "$shell_path"
}

run_shell_path_check() {
  local shell_path="$1"
  local shell_name
  shell_name="$(basename "$shell_path")"
  local marker_start="__XCODECLI_PATH_START__"
  local marker_end="__XCODECLI_PATH_END__"
  local raw_output=""

  case "$shell_name" in
    zsh|bash)
      raw_output="$("$shell_path" -lic "printf '%s\n' '${marker_start}'; command -v xcodecli || true; printf '%s\n' '${marker_end}'" 2>/dev/null | sed -E $'s/\x1b\\[[0-9;]*[[:alpha:]]//g')"
      ;;
    fish)
      raw_output="$("$shell_path" -lc "printf '%s\n' '${marker_start}'; command -v xcodecli; true; printf '%s\n' '${marker_end}'" 2>/dev/null | sed -E $'s/\x1b\\[[0-9;]*[[:alpha:]]//g')"
      ;;
    *)
      return 1
      ;;
  esac

  awk -v start="$marker_start" -v end="$marker_end" '
    $0 == start { capture = 1; next }
    $0 == end { capture = 0; exit }
    capture && NF { print; exit }
  ' <<< "$raw_output"
}

print_path_guidance() {
  local shell_name="$1"
  local dir="$2"
  case "$shell_name" in
    zsh)
      log "add this to ~/.zprofile (or ~/.zshrc):"
      log "  export PATH=\"${dir}:\$PATH\""
      log "then open a new terminal or run:"
      log "  exec zsh -l"
      ;;
    bash)
      log "add this to ~/.bash_profile (or ~/.profile):"
      log "  export PATH=\"${dir}:\$PATH\""
      log "then open a new terminal or run:"
      log "  exec bash -l"
      ;;
    fish)
      log "run this once in fish:"
      log "  fish_add_path ${dir}"
      log "then open a new terminal or run:"
      log "  exec fish"
      ;;
    *)
      warn_if_not_on_path "$dir"
      ;;
  esac
}

report_path_status() {
  local install_bin_dir="$1"
  local install_path="$2"
  local install_resolved
  install_resolved="$(resolve_path "$install_path")"

  log "checking PATH integration"

  local current_hit=""
  current_hit="$(command -v xcodecli 2>/dev/null || true)"
  if [[ -n "$current_hit" ]]; then
    local current_resolved=""
    current_resolved="$(resolve_path "$current_hit" 2>/dev/null || true)"
    if [[ "$current_resolved" == "$install_resolved" ]]; then
      log "current session PATH resolves xcodecli -> ${current_resolved}"
    else
      log "warning: current session PATH resolves xcodecli -> ${current_hit}"
      log "warning: newly installed binary is ${install_resolved}"
    fi
  else
    log "current session PATH does not yet resolve xcodecli"
  fi

  local preferred_shell=""
  preferred_shell="$(detect_preferred_shell 2>/dev/null || true)"
  if [[ -z "$preferred_shell" ]]; then
    log "warning: could not detect a preferred login shell to verify PATH persistence"
    warn_if_not_on_path "$install_bin_dir"
    return 0
  fi

  local shell_name
  shell_name="$(basename "$preferred_shell")"
  local shell_hit=""
  shell_hit="$(run_shell_path_check "$preferred_shell" 2>/dev/null || true)"
  if [[ -n "$shell_hit" ]]; then
    local shell_resolved=""
    shell_resolved="$(resolve_path "$shell_hit" 2>/dev/null || true)"
    if [[ "$shell_resolved" == "$install_resolved" ]]; then
      log "${shell_name} login shell resolves xcodecli -> ${shell_resolved}"
      if [[ -z "$current_hit" ]]; then
        log "open a new terminal window or restart your shell to pick up the updated PATH"
      fi
      return 0
    fi
    log "warning: ${shell_name} login shell resolves xcodecli -> ${shell_hit}"
    log "warning: newly installed binary is ${install_resolved}"
    log "you may need to move ${install_bin_dir} earlier in PATH"
    print_path_guidance "$shell_name" "$install_bin_dir"
    return 0
  fi

  log "warning: ${shell_name} login shell could not find xcodecli on PATH"
  print_path_guidance "$shell_name" "$install_bin_dir"
}

BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
REF="${XCODECLI_REF:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bin-dir)
      [[ $# -ge 2 ]] || fail "--bin-dir requires a path"
      BIN_DIR="$2"
      shift 2
      ;;
    --ref)
      [[ $# -ge 2 ]] || fail "--ref requires a branch or tag"
      REF="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

[[ "$(uname -s)" == "Darwin" ]] || fail "xcodecli only supports macOS"

require_cmd go

SCRIPT_DIR=""
if SCRIPT_DIR="$(resolve_script_dir)"; then
  :
fi
ROOT_DIR=""
if [[ -n "$SCRIPT_DIR" ]]; then
  ROOT_DIR="$(cd "${SCRIPT_DIR}/.." >/dev/null 2>&1 && pwd)"
fi

USE_LOCAL_SOURCE=0
if [[ -n "$ROOT_DIR" && -f "${ROOT_DIR}/go.mod" && -x "${ROOT_DIR}/scripts/build.sh" && -z "$REF" ]]; then
  USE_LOCAL_SOURCE=1
fi

WORK_DIR=""
cleanup() {
  if [[ -n "$WORK_DIR" && -d "$WORK_DIR" ]]; then
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT

if [[ "$USE_LOCAL_SOURCE" -eq 1 ]]; then
  BUILD_ROOT="$ROOT_DIR"
  log "using local checkout at ${BUILD_ROOT}"
else
  require_cmd curl
  require_cmd git
  require_cmd tar
  REF="${REF:-$DEFAULT_REF}"
  ARCHIVE_URL="$(resolve_archive_url "$REF")"
  WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/xcodecli-install-XXXXXX")"
  TARBALL_PATH="${WORK_DIR}/source.tar.gz"
  log "downloading ${ARCHIVE_URL}"
  curl -fsSL "$ARCHIVE_URL" -o "$TARBALL_PATH"
  log "extracting source archive"
  tar -xzf "$TARBALL_PATH" -C "$WORK_DIR"
  BUILD_ROOT="$(find "$WORK_DIR" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
  [[ -n "$BUILD_ROOT" && -f "${BUILD_ROOT}/go.mod" && -x "${BUILD_ROOT}/scripts/build.sh" ]] || fail "downloaded source archive did not contain the expected project layout"
  log "using downloaded source at ${BUILD_ROOT}"
fi

INSTALL_BIN_DIR="$(mkdir -p "$BIN_DIR" && cd "$BIN_DIR" >/dev/null 2>&1 && pwd)"
TEMP_OUTPUT="${WORK_DIR:-${TMPDIR:-/tmp}}/xcodecli"
rm -f "$TEMP_OUTPUT"
log "building xcodecli"
if [[ -n "$REF" && "$REF" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  env VERSION="$REF" "${BUILD_ROOT}/scripts/build.sh" "$TEMP_OUTPUT"
else
  env -u VERSION "${BUILD_ROOT}/scripts/build.sh" "$TEMP_OUTPUT"
fi

INSTALL_PATH="${INSTALL_BIN_DIR}/xcodecli"
log "installing to ${INSTALL_PATH}"
install -m 0755 "$TEMP_OUTPUT" "$INSTALL_PATH"

log "verifying install"
"${INSTALL_PATH}" help >/dev/null
if version_output="$("${INSTALL_PATH}" version 2>/dev/null)"; then
  log "installed version: ${version_output}"
fi

log "installed ${INSTALL_PATH}"
report_path_status "$INSTALL_BIN_DIR" "$INSTALL_PATH"
