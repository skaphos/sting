# Changelog

All notable changes to sting are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims
to adhere to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.4](https://github.com/skaphos/sting/compare/v0.0.3...v0.0.4) (2026-06-01)


### Features

* add commit file and diff evidence ([#12](https://github.com/skaphos/sting/issues/12)) ([1c8369b](https://github.com/skaphos/sting/commit/1c8369b4dda6d6cd05792e23050fc2a94b1d5639))
* add commit-query core ([1674079](https://github.com/skaphos/sting/commit/16740799d7bc457135a660ca9b63a9831757438a))
* add GitLab commit provider ([#6](https://github.com/skaphos/sting/issues/6)) ([8e9823a](https://github.com/skaphos/sting/commit/8e9823a568ccb5f07f6881bbf5509d8691a41ac4))
* add MCP server with read-only get_commits tool ([37c3616](https://github.com/skaphos/sting/commit/37c361625c7cc892468aed683a981511f326edff))
* add multi-runtime MCP installer ([48d09bf](https://github.com/skaphos/sting/commit/48d09bfb45fa8fe678f93a79329335b324a29ff7))
* add OAuth App authentication for GitHub and GitLab (SKA-466) ([#13](https://github.com/skaphos/sting/issues/13)) ([7d39af3](https://github.com/skaphos/sting/commit/7d39af385f8937178319c668bf970c222dec9238))
* wire cobra CLI with viper config and the sting binary ([14e4f8b](https://github.com/skaphos/sting/commit/14e4f8b6e2eacc7565c1497a245ad93ecfcde2a8))

## [0.0.3](https://github.com/skaphos/sting/compare/v0.0.2...v0.0.3) (2026-05-31)


### Features

* add commit file and diff evidence ([#12](https://github.com/skaphos/sting/issues/12)) ([87a3fd1](https://github.com/skaphos/sting/commit/87a3fd18340ffd034cad0092c3dd33d1e2c19142))
* add OAuth App authentication for GitHub and GitLab (SKA-466) ([#13](https://github.com/skaphos/sting/issues/13)) ([dd37ea4](https://github.com/skaphos/sting/commit/dd37ea4d0d3f11026c2fa841d85043fa933863e4))

## [0.0.2](https://github.com/skaphos/sting/compare/v0.0.1...v0.0.2) (2026-05-30)


### Features

* add commit-query core ([a1615cd](https://github.com/skaphos/sting/commit/a1615cd8af1e207e8ee7744c0e4b335f935cd73f))
* add GitLab commit provider ([#6](https://github.com/skaphos/sting/issues/6)) ([51acee1](https://github.com/skaphos/sting/commit/51acee163105dd0c3e8437908ff80a6d5edbff44))
* add MCP server with read-only get_commits tool ([6bf285e](https://github.com/skaphos/sting/commit/6bf285e72ca379fd096744c50bd7ab230d7e77e2))
* add multi-runtime MCP installer ([c88615c](https://github.com/skaphos/sting/commit/c88615c631d506229d59e979f93c824ce26de192))
* wire cobra CLI with viper config and the sting binary ([97cd171](https://github.com/skaphos/sting/commit/97cd171c8a7710e9c4a551ecc84adf9deb6c7bff))

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
