#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE="${PACKAGE:-./cmd/xcodecli}"
OUTPUT="${1:-${OUTPUT:-${ROOT_DIR}/xcodecli}}"

mkdir -p "$(dirname "$OUTPUT")"

echo "[build] package: ${PACKAGE}"
echo "[build] output:  ${OUTPUT}"

cd "$ROOT_DIR"
go build -o "$OUTPUT" "$PACKAGE"

echo "[build] done"
