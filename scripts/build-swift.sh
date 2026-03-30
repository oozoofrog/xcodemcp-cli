#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT="${1:-${OUTPUT:-${ROOT_DIR}/.build/release/xcodecli}}"
BUILD_CHANNEL="${BUILD_CHANNEL:-dev}"

bash "${ROOT_DIR}/scripts/check-version-sync.sh"

# Extract source version from Swift source
SOURCE_VERSION="$(sed -n 's/.*static let source = "\(.*\)"/\1/p' "${ROOT_DIR}/Sources/XcodeCLICore/Shared/Version.swift" | head -n 1)"
VERSION="${VERSION:-${SOURCE_VERSION:-v0.0.0}}"

echo "[build-swift] version: ${VERSION}"
echo "[build-swift] channel: ${BUILD_CHANNEL}"
echo "[build-swift] output:  ${OUTPUT}"

# Inject version and channel into Version.swift before building
VERSION_FILE="${ROOT_DIR}/Sources/XcodeCLICore/Shared/Version.swift"
BACKUP_FILE="$(mktemp "${TMPDIR:-/tmp}/xcodecli-version-swift.XXXXXX")"
cp "$VERSION_FILE" "$BACKUP_FILE"

cleanup() {
  if [[ -f "$BACKUP_FILE" ]]; then
    mv "$BACKUP_FILE" "$VERSION_FILE"
  fi
}
trap cleanup EXIT

sed -i '' "s|public static let current: String = source|public static let current: String = \"${VERSION}\"|" "$VERSION_FILE"
sed -i '' "s|public static let buildChannel: String = \"dev\"|public static let buildChannel: String = \"${BUILD_CHANNEL}\"|" "$VERSION_FILE"

cd "$ROOT_DIR"
swift build -c release

# Copy to requested output location if different from default
BUILT_BINARY="${ROOT_DIR}/.build/release/xcodecli"
if [[ "$OUTPUT" != "$BUILT_BINARY" ]]; then
  mkdir -p "$(dirname "$OUTPUT")"
  cp "$BUILT_BINARY" "$OUTPUT"
fi

echo "[build-swift] done"
