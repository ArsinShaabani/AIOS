#!/usr/bin/env bash
# Quality gate: total statement coverage >= 80% and >= 80% for core packages.
# See docs/DESIGN.md §7. The profile must come from scripts/coverage.sh (merged,
# de-duplicated) — a raw multi-package `go test -coverprofile` double-counts.
set -euo pipefail
cd "$(dirname "$0")/.."

TOTAL_MIN="${TOTAL_MIN:-80}"
CORE_MIN="${CORE_MIN:-80}"

if [[ ! -f coverage.out ]]; then
  echo "coverage.out not found; run ./scripts/coverage.sh first" >&2
  exit 1
fi

exec python3 scripts/coverage-check.py coverage.out "$TOTAL_MIN" "$CORE_MIN"
