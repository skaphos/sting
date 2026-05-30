# Changelog

All notable changes to sting are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims
to adhere to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- GitLab provider support for repository and group-scoped commit queries via
  `--provider gitlab`, `gitlab_token` / `STING_GITLAB_TOKEN`, and
  `gitlab_base_url` / `STING_GITLAB_BASE_URL`. GitLab `search` scope is not
  supported in this first pass; use `repos` or `org`.
- Query a GitHub user's commits over a time window, as a local CLI
  (`sting --author <user> --window <window>`) or as an MCP server (`sting mcp`)
  exposing a single read-only `get_commits` tool over stdio.
- Three discovery scopes: `search` (commit-search index), `repos` (explicit
  `owner/repo` list), and `org` (enumerate an org's repositories).
- `search` scope accepts `--org` / `--repos` qualifiers, scoping the search
  index into a private org or repo set the token can access.
- Output as Markdown (grouped by repository, newest first) or JSON.
- Flexible time windows (`7d`, `2w`, `48h`, …) and explicit `--since`/`--until`
  bounds (RFC3339 or `YYYY-MM-DD`).
- viper-backed configuration with precedence defaults < config file < env
  (`STING_*`) < flags. A dedicated GitHub PAT (`token` / `STING_TOKEN`) is used,
  kept separate from the ambient `GITHUB_TOKEN`.
- `sting install` / `uninstall` / `install list` to register the MCP server with
  Claude Code, Codex, OpenCode, and Grok, with atomic, format-preserving config
  writes and a `--manual` mode that prints paste-ready snippets.
- `get_commits` is annotated read-only (`readOnlyHint`); the Claude install
  snippet emits a `permissions.allow` block that auto-approves it.

[Unreleased]: https://github.com/skaphos/sting/commits/main
