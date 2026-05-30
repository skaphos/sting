# 5. Release Please owns release notes

## Status

Accepted.

## Context

sting needs a release gate before publishing binaries. The previous workflow
used `skaphos/actions/release-pr` to bump `.release-please-manifest.json` and
`skaphos/actions/release-tag` to push `vX.Y.Z`; GoReleaser then created or
replaced the GitHub release body from its own changelog settings.

That split gave GoReleaser ownership of the release object, but it also meant
the repository carried release-please state without using release-please for the
part it is best at: maintaining the release PR, changelog, tag, and release
notes together.

## Decision

Use `googleapis/release-please-action@v5` as the release gate. Release Please
opens and updates the release PR, bumps `.release-please-manifest.json`, updates
`CHANGELOG.md`, creates the `vX.Y.Z` tag, and creates the GitHub release with
the release notes.

The workflow uses the `skaphos-release-bot` GitHub App token rather than the
default `GITHUB_TOKEN` so the tag created by Release Please triggers the
tag-driven GoReleaser workflow.

GoReleaser remains responsible for building and uploading artifacts, SBOMs,
signatures, attestations, and the Homebrew cask. It is configured with
`release.mode: keep-existing` and `changelog.disable: true` so it attaches to
the Release Please release without replacing its notes.

## Consequences

- Release Please is the single source of truth for release PRs, changelog
  content, tags, and GitHub release notes.
- GoReleaser no longer computes user-facing release notes. Its release job is
  artifact publication for the existing tag/release.
- The release flow depends on the release bot app having `contents: write`,
  `pull-requests: write`, and enough repository access for app-created tag
  pushes to trigger downstream workflows.

## Alternatives Considered

- **Keep the custom skaphos/actions gate.** Rejected. It duplicates the release
  PR and tag behavior Release Please already provides while leaving
  release-please manifest state in the repository.
- **Let GoReleaser replace the release body.** Rejected. That recreates the
  two-tools-own-the-release conflict; release notes should come from one tool.
