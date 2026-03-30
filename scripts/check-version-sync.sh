#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_VERSION_FILE="${ROOT_DIR}/cmd/xcodecli/version.go"
SWIFT_VERSION_FILE="${ROOT_DIR}/Sources/XcodeCLICore/Shared/Version.swift"

extract_go_version() {
  sed -n 's/^const sourceVersion = "\(.*\)"$/\1/p' "$GO_VERSION_FILE" | head -n 1
}

extract_swift_version() {
  sed -n 's/^.*public static let source = "\(.*\)".*$/\1/p' "$SWIFT_VERSION_FILE" | head -n 1
}

GO_VERSION="$(extract_go_version)"
SWIFT_VERSION="$(extract_swift_version)"

if [[ -z "$GO_VERSION" ]]; then
  echo "[version-sync] failed to extract Go sourceVersion from ${GO_VERSION_FILE}" >&2
  exit 1
fi

if [[ -z "$SWIFT_VERSION" ]]; then
  echo "[version-sync] failed to extract Swift Version.source from ${SWIFT_VERSION_FILE}" >&2
  exit 1
fi

if [[ "$GO_VERSION" != "$SWIFT_VERSION" ]]; then
  echo "[version-sync] mismatch detected" >&2
  echo "  Go:    ${GO_VERSION}" >&2
  echo "  Swift: ${SWIFT_VERSION}" >&2
  exit 1
fi

echo "[version-sync] ok: ${GO_VERSION}"
