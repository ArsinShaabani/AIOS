#!/usr/bin/env bash
# Generate a correct merged coverage profile.
#
# Why not a single `go test -coverprofile ./...`? Go writes one block per test
# binary, so blocks are duplicated many times and coverage is understated.
# Here every package is tested separately (instrumenting all of ./internal/...),
# then profiles are merged by summing counts for unique blocks.
set -euo pipefail
cd "$(dirname "$0")/.."

OUT="${1:-coverage.out}"
PARTS="$(mktemp -d)"
trap 'rm -rf "$PARTS"' EXIT

packages=$(go list ./internal/... ./tests/e2e/...)
failed=0
for pkg in $packages; do
  name=$(echo "$pkg" | tr '/.' '__')
  echo "testing $pkg"
  if ! go test -count=1 -covermode=atomic -coverpkg=./internal/... \
      -coverprofile="$PARTS/$name.out" "$pkg" >/tmp/aios-cov-one.log 2>&1; then
    echo "FAILED: $pkg" >&2
    tail -20 /tmp/aios-cov-one.log >&2
    failed=1
  fi
done

if [[ "$failed" -ne 0 ]]; then
  echo "test failures detected; coverage profile is incomplete" >&2
  exit 1
fi

python3 scripts/merge-coverage.py "$OUT" "$PARTS"/*.out
go tool cover -func="$OUT" | tail -1
