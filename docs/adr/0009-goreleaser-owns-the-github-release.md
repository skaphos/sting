# 9. GoReleaser owns the GitHub release (supersedes part of ADR 0005)

## Status

Accepted. Supersedes the release-object-ownership portion of
[ADR 0005](0005-release-please-owns-release-notes.md).

## Context

[ADR 0005](0005-release-please-owns-release-notes.md) decided that Release
Please would create a **draft** GitHub release with release notes, and that
GoReleaser would attach artifacts to that existing draft
(`release.use_existing_draft: true`, `release.mode: keep-existing`,
`changelog.disable: true`) before publishing it.

The workflows as implemented do not match that decision:

- `.github/workflows/release-please.yml` passes `skip-github-release: true` to
  `googleapis/release-please-action`, so Release Please never creates a GitHub
  release object (draft or otherwise) — it only manages the release PR,
  `.release-please-manifest.json`, `CHANGELOG.md`, and the `vX.Y.Z` tag.
- `.goreleaser.yaml` has `changelog.use: github` (GoReleaser computes its own
  release notes from GitHub commit/PR data, grouped by Conventional Commit
  type) and `release.draft: false` (GoReleaser publishes the release directly;
  there is no existing draft for it to attach to).

In practice, GoReleaser is the tool that creates and publishes the GitHub
release today, using its own changelog rendering rather than Release Please's.

## Decision

Record reality: **GoReleaser owns the GitHub release object and its release
notes.** Release Please's role is limited to the release PR, changelog file,
manifest, and tag.

This ADR does not change behavior — it corrects the documentation to match the
`skip-github-release: true` / `release.draft: false` configuration already in
place. If a maintainer decides the draft-handoff model from ADR 0005 is still
wanted, that is a follow-up config change (remove `skip-github-release`, set
`release.use_existing_draft: true` / `release.mode: keep-existing` /
`changelog.disable: true` in `.goreleaser.yaml`), not a docs fix, and should get
its own ADR once implemented.

## Consequences

- `CONTRIBUTING.md` and `AGENTS.md` should describe GoReleaser, not Release
  Please, as the source of the published GitHub release and its notes.
- Release Please's `CHANGELOG.md` content and GoReleaser's generated release
  notes are produced by two independent changelog renderers and may drift in
  wording (both are driven by Conventional Commits, but with different
  grouping/filtering rules — compare `release-please-config.json` and the
  `changelog:` block in `.goreleaser.yaml`).
- No workflow or `.goreleaser.yaml` changes are made by this ADR; it is a
  documentation-only correction.
