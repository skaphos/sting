# 7. Commit file and diff evidence

Date: 2026-05-30

## Status

Accepted

## Context

Sting's primary use case is asking an LLM what a person changed over a recent
time window. Commit subjects and messages are useful, but they are often not
enough to explain the real work. The existing `include_stats` option also did
not cover every discovery path consistently, especially when GitHub commits
came from search results.

The public `model.Result` contract is intentionally consumed as evidence by
agents and downstream tools, so adding file and patch data changes the result
shape and must be explicit.

## Decision

Extend `model.Query` and `model.Commit` with optional evidence depth:

- `include_stats` requests additions, deletions, and total changed lines.
- `include_files` requests per-file paths, status, and line counts.
- `include_diffs` requests patch text and implies `include_files`.
- `max_diff_bytes` caps patch text per commit. Truncated files are marked with
  `patch_truncated`.

The default remains commit metadata only. File and diff evidence are opt-in
because they require extra provider API calls and can produce large LLM inputs.

Bump `model.SchemaVersion` to `sting.skaphos.io/v2` because `Commit` gained
`changes` and `files` fields.

## Consequences

- Agents can answer "what did this person actually change?" from file and patch
  evidence rather than commit messages alone.
- GitHub search, repo, and org scopes now share the same commit-detail path
  when stats/files/diffs are requested.
- GitLab uses `with_stats` for commit stats and commit diff APIs for file
  evidence.
- Full diffs are bounded per commit. Large patches may be truncated, and
  callers must treat `patch_truncated` as part of the evidence contract.
- More detailed evidence costs additional read-only API calls. The MCP tool
  remains read-only, so installer auto-approval invariants do not change.
