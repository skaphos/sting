# Contributing Guidelines

Thanks for contributing to sting.

## Development Setup

- Go version: see `go.mod` (`go 1.26.3`).
- Run task targets without installing tools globally (Task is pinned in `tools/`):
  - `go -C tools tool task --list`

## Branching and Commits

- Create focused branches from `main`; one logical change per branch.
- Branch naming by change type: `feature/…`, `bug/…`, `chore/…`, `docs/…`,
  `ci/…`, `refactor/…`.
- Every commit must be cryptographically signed **and** carry a DCO sign-off:
  - `git commit -S -s …`
  - Required trailer: `Signed-off-by: Your Name <you@example.com>`
- Use Conventional Commits for what lands on `main`. Release Please uses them
  to infer the next version and release notes:
  - `feat:` → minor
  - `fix:` / `perf:` → patch
  - `docs:`, `test:`, `ci:`, `chore:`, `refactor:` → no bump by default
  - `!` in the type/scope or a `BREAKING CHANGE:` footer → major
- If you squash-merge, the final squash commit message must also be a
  Conventional Commit.

Examples:

- `feat(query): add --until bound`
- `fix(ghclient): stop paging once MaxCommits is reached`

## Coding Standards

- Follow Go conventions and keep code readable.
- Public API surface is `model`, `config`, and `ghclient`; keep it minimal and
  stable. Breaking changes to `Commit`/`Result` must bump `model.SchemaVersion`.
- Keep REUSE metadata valid:
  - Source files must include an `SPDX-License-Identifier` header set to `MIT`.
  - Validate with `reuse lint`.
- Credit shipped dependencies:
  - Regenerate `third_party_licenses/` with `go -C tools tool task notices`
    whenever `go.mod`/`go.sum` changes.
  - Review `THIRD_PARTY_NOTICES.md` before merging dependency updates.
- Format and lint:
  - `go -C tools tool task fmt`
  - `go -C tools tool task lint`

## Testing

Run before opening a PR:

- `go -C tools tool task test`
- `go -C tools tool task test-cover`
- `go -C tools tool task test-cover-check`  (enforces the 80% per-package gate)
- `go -C tools tool task staticcheck`
- `go -C tools tool task vuln`

Or run the full local CI sequence:

- `go -C tools tool task ci`

Tests must not hit the network: use `net/http/httptest` for GitHub calls and
isolate `HOME`/`USERPROFILE` for filesystem-touching tests.

## Pull Requests

PRs should include:

- Summary of what changed
- Why the change is needed
- Testing performed (commands and results)
- Doc updates when behavior changes (`README.md`, `docs/adr/`)

## Safety Expectations

- sting is read-only. Do not add tools or commands that mutate GitHub state.
- Every exposed MCP tool must be read-only; keep `mcpserver.ReadOnlyTools()` the
  single source of truth for the installer's auto-approve list.
- Keep sting's dedicated PAT (`STING_TOKEN`) separate from `GITHUB_TOKEN`.

## Release Process

Releases are cut by Release Please and published by `goreleaser` on the
resulting tag (binaries, checksums, SBOM, cosign signatures, provenance
attestations, and a Homebrew cask). Release Please owns the release PR,
`CHANGELOG.md`, tag, and GitHub release notes; GoReleaser attaches artifacts to
that release. Release automation requires the `RELEASE_BOT_APP_ID` variable and
`RELEASE_BOT_PRIVATE_KEY` secret to be provisioned, with the app granted access
to this repository and `skaphos/homebrew-tools`.
