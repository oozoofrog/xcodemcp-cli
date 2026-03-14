#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE="${PACKAGE:-./cmd/xcodemcp}"
OUTPUT="${1:-${OUTPUT:-${ROOT_DIR}/xcodemcp}}"

mkdir -p "$(dirname "$OUTPUT")"

echo "[build] package: ${PACKAGE}"
echo "[build] output:  ${OUTPUT}"

cd "$ROOT_DIR"
go build -o "$OUTPUT" "$PACKAGE"

echo "[build] done"
