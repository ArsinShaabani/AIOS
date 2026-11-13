#!/usr/bin/env bash
# Move the staged GitHub Actions workflows into .github/workflows and push them.
# Requires a credential with the `workflows` permission (GitHub App "Workflows:
# Read and write", or a PAT with the `workflow` scope). See deploy/ci/README.md.
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ ! -d deploy/ci ]]; then
  echo "deploy/ci not found; workflows may already be enabled" >&2
  exit 1
fi

mkdir -p .github/workflows
git mv deploy/ci/ci.yml .github/workflows/ci.yml
git mv deploy/ci/release.yml .github/workflows/release.yml
git add .github/workflows deploy/ci

git commit -m "ci: enable GitHub Actions workflows" || {
  echo "nothing to commit"
}
git push origin HEAD:main

echo "CI workflows enabled and pushed."
