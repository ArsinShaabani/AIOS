# CI workflows (staged)

GitHub Actions definitions live here **temporarily**. They are the same files
that belong in `.github/workflows/`, but the automated push used to publish this
repository runs as a GitHub App without the `workflows` permission, and GitHub
rejects any push that creates or edits `.github/workflows/*` in that case.

To activate CI on GitHub:

```bash
./scripts/enable-ci.sh
```

The script moves `deploy/ci/*.yml` back to `.github/workflows/`, commits with a
Conventional Commit message and pushes. Run it from a machine/token that has the
`workflows` permission (a normal user PAT with `workflow` scope, or the GitHub
App installed with Workflows: Read and write).

Until then, all checks are reproducible locally:

```bash
make lint     # gofmt + go vet
make test     # unit + e2e
make cover    # coverage profile (atomic, -coverpkg)
make release  # cross-compiled linux amd64/arm64/armv7 + checksums
```

The workflows themselves are unchanged and cover: format/vet, race tests with
coverage gates, amd64/arm64 builds, multi-arch Docker build, tagged releases with
checksums, and GHCR image publishing.
