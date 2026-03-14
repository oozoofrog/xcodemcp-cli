#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE="${PACKAGE:-./cmd/xcodecli}"
OUTPUT="${1:-${OUTPUT:-${ROOT_DIR}/xcodecli}}"
SOURCE_VERSION="$(sed -n 's/^const sourceVersion = "\(.*\)"$/\1/p' "${ROOT_DIR}/cmd/xcodecli/version.go" | head -n 1)"
VERSION="${VERSION:-${SOURCE_VERSION:-v0.0.0}}"
BUILD_CHANNEL="${BUILD_CHANNEL:-dev}"
GO_LDFLAGS="${GO_LDFLAGS:-}"

if [[ -n "$GO_LDFLAGS" ]]; then
  GO_LDFLAGS="${GO_LDFLAGS} "
fi
GO_LDFLAGS="${GO_LDFLAGS}-X main.cliVersion=${VERSION} -X main.cliBuildChannel=${BUILD_CHANNEL}"

mkdir -p "$(dirname "$OUTPUT")"

echo "[build] package: ${PACKAGE}"
echo "[build] output:  ${OUTPUT}"
echo "[build] version: ${VERSION}"
echo "[build] channel: ${BUILD_CHANNEL}"

cd "$ROOT_DIR"
go build -ldflags "$GO_LDFLAGS" -o "$OUTPUT" "$PACKAGE"

echo "[build] done"
