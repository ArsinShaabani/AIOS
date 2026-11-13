#!/usr/bin/env bash
# Quality gates. The project target is 80% statement coverage; the current
# enforced floor is deliberately honest (see docs/ARCHITECTURE.md §11).
set -euo pipefail
cd "$(dirname "$0")/.."

TOTAL_MIN="${TOTAL_MIN:-35}"
CORE_MIN="${CORE_MIN:-35}"

if [[ ! -f coverage.out ]]; then
  echo "coverage.out not found; run 'make cover' first" >&2
  exit 1
fi

total=$(go tool cover -func=coverage.out | awk '/^total:/ {gsub("%","",$3); print $3}')
echo "total coverage: ${total}% (minimum ${TOTAL_MIN}%)"
awk -v c="$total" -v m="$TOTAL_MIN" 'BEGIN { exit (c+0 >= m+0) ? 0 : 1 }' || {
  echo "FAIL: total coverage ${total}% is below ${TOTAL_MIN}%" >&2
  exit 1
}

# Per-package floors for packages with dedicated unit suites.
core_packages=(
  "./internal/platform"
  "./internal/security"
  "./internal/voice"
  "./internal/config"
  "./internal/adapters/vector"
)

failed=0
for pkg in "${core_packages[@]}"; do
  out=$(go test -cover "$pkg" 2>/dev/null | tail -1)
  pct=$(echo "$out" | sed -n 's/.*coverage: \([0-9.]*\)%.*/\1/p')
  if [[ -z "$pct" ]]; then
    echo "WARN: no coverage for $pkg"
    continue
  fi
  if awk -v c="$pct" -v m="$CORE_MIN" 'BEGIN { exit (c+0 >= m+0) ? 0 : 1 }'; then
    echo "ok   ${pkg} ${pct}%"
  else
    echo "FAIL ${pkg} ${pct}% < ${CORE_MIN}%" >&2
    failed=1
  fi
done

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi
echo "coverage gates passed"
