# sting

Query a GitHub or GitLab user's commits over a time window and hand them to an
LLM agent (or a terminal) in a consumable form.

`sting` is a single binary with subcommands:

- **`sting mcp`** — runs an MCP server over stdio exposing a single, read-only
  `get_commits` tool, so an agent like Claude Code can answer *"give me all the
  commits of `mfacenet` in the last week and tell me what he's working on."*
- **`sting <query flags>`** — prints a Markdown or JSON report locally.
- **`sting install` / `uninstall` / `install list`** — register the MCP server
  with your agent runtimes (Claude Code, Codex, OpenCode, Grok).

Configuration is resolved with [viper] (`defaults < config file < env < flags`),
so dedicated read-only PATs can live in sting's own config instead of relying on
ambient provider tokens.

## Install

```sh
brew tap skaphos/tools https://github.com/skaphos/homebrew-tools
brew install --cask skaphos/tools/sting
```

Or install from source:

```sh
go install github.com/skaphos/sting/cmd/sting@latest
```

Or build from this repo:

```sh
go -C tools tool task build      # -> ./bin/sting
```

## Authentication

### GitHub

sting uses its **own** token key, deliberately separate from `GITHUB_TOKEN` so a
dedicated read-only GitHub PAT does not collide with other tools. Set it in the
config file (`token:`) or via `STING_TOKEN`:

```sh
# config file (recommended): ~/.config/sting/config.yaml
token: ghp_xxx

# or environment
export STING_TOKEN=ghp_xxx
```

Unauthenticated calls work for public data but are heavily rate limited (global
commit search is ~10 requests/min without a token). A classic PAT needs no scopes
for public repos; add read `repo` access to include private repos.

### GitLab

GitLab uses a separate token key, deliberately separate from both the GitHub
token and ambient GitLab environment variables. Set it in the config file
(`gitlab_token:`) or via `STING_GITLAB_TOKEN`:

```sh
# config file (recommended): ~/.config/sting/config.yaml
gitlab_token: glpat_xxx

# or environment
export STING_GITLAB_TOKEN=glpat_xxx
```

GitLab.com is the default. For self-managed GitLab, point `gitlab_base_url` (or
`STING_GITLAB_BASE_URL`) at the API v4 root, for example:

```yaml
gitlab_base_url: https://gitlab.example.com/api/v4/
```

## Agent integration (the main use case)

Register the MCP server with every detected runtime:

```sh
sting install                      # auto-detect Claude, Codex, OpenCode, Grok
sting install --claude             # just one runtime
sting install --scope project      # write project-scoped config in CWD
sting install --manual             # print snippets instead of writing
sting install list                 # show registration state per runtime
sting uninstall                    # remove entries (prompts unless --yes)
```

`install` writes a `sting mcp` entry pointing at the current executable. Because
`get_commits` is **read-only** (advertised via the MCP `readOnlyHint`
annotation), the Claude snippet also prints a paste-ready `permissions.allow`
block that auto-approves the tool — safe to accept without per-call prompts.

Then ask the agent naturally:

> Give me all the commits of `mfacenet` in the last week and tell me what he's
> working on.

The tool returns structured commit data **plus** a Markdown summary, so the
agent has clean material to describe the work.

## CLI usage

```sh
# Last week of an author's commits, anywhere GitHub has indexed (Markdown).
sting --author mfacenet --window 7d

# JSON for piping into other tools.
sting --author mfacenet --window 7d -o json

# Within specific repos (most complete; works on private repos with a token).
sting --author mfacenet --scope repos --repos skaphos/sting,skaphos/other

# Across every repo in an org.
sting --author mfacenet --scope org --org skaphos --window 2w

# GitLab project commits.
sting --provider gitlab --author mfacenet --scope repos --repos skaphos/sting

# GitLab group commits, including projects in subgroups.
sting --provider gitlab --author mfacenet --scope org --org skaphos --window 2w

# Explicit bounds and per-commit line stats.
sting --author mfacenet --since 2026-05-01 --until 2026-05-15 --stats

# File-level evidence without full patches.
sting --author mfacenet --scope repos --repos skaphos/sting --window 7d --files

# Full bounded diffs for LLM analysis. This implies --files.
sting --author mfacenet --scope repos --repos skaphos/sting --window 7d --diffs --max-diff-bytes 60000
```

Run `sting --help` (or `sting <command> --help`) for the full flag list.

### Evidence depth

The default query returns commit metadata and messages only. Use the evidence
flags when you want an agent to explain the actual code changes:

- `--stats` adds per-commit additions, deletions, and total changed lines.
- `--files` adds changed file paths, statuses, and per-file line counts (and also populates the per-commit totals).
- `--diffs` adds patch text for each changed file and implies `--files`.
- `--max-diff-bytes` caps patch text per commit; truncated files are marked in
  JSON and Markdown.

GitHub fetches this evidence from per-commit detail calls. GitLab uses
`with_stats` for line stats and commit diff calls for file evidence. Keep full
diffs explicit because they cost extra API calls and can be token-heavy for an
LLM context.

### Scopes

| provider | scope    | how it finds commits                                             | notes                                               |
|----------|----------|------------------------------------------------------------------|-----------------------------------------------------|
| GitHub   | `search` | GitHub commit search by `author:`                                | global (public-only) unless scoped; 1000-result cap |
| GitHub   | `repos`  | lists commits in each `owner/repo` you name, filtered by author  | most complete; supports private repos with a token  |
| GitHub   | `org`    | enumerates an org's repos, then lists commits in each            | needs org read access for private repos             |
| GitLab   | `repos`  | lists commits in each `group/project` or project ID              | supports nested group paths; GitLab search not used |
| GitLab   | `org`    | treats `org` as a GitLab group and includes subgroup projects     | needs group/project read access for private data    |

GitLab `search` scope is not supported yet. GitLab's search API does not map
cleanly to sting's date-bounded author query contract, so use `repos` or `org`
with `--provider gitlab`.

#### Searching private orgs

A bare `search` is a global author query, which GitHub limits to **public**
repos. To reach a **private** org via the search index, scope the query by
combining `search` with `--org` (or `--repos`):

```sh
# Adds `org:Alaska-Airlines-Shared` to the search query.
sting --author mfacenet --scope search --org Alaska-Airlines-Shared --window 7d
```

Two requirements for any private-org result:

- **Token access** — the PAT must have read access to the private repos. If the
  org enforces SAML/SSO, the token must be **SSO-authorized** for it (classic
  PAT) or approved with repo/contents read (fine-grained PAT); otherwise private
  results are silently excluded.
- **Indexing** — commit search only covers default branches and skips commits
  attributed to an email not linked to the GitHub account.

For the **most complete** private-org coverage (not limited by the search
index), prefer `--scope org --org Alaska-Airlines-Shared`, which lists commits
per repo directly. `search --org` is faster (one query vs. per-repo listings)
but subject to the indexing caveats above.

### Time window

`--window` accepts `7d`, `2w`, `48h`, `30m`, etc. `--since`/`--until` accept
`YYYY-MM-DD` or RFC3339. `--since` overrides `--window`; `--until` defaults to now.

## Configuration

Resolved in increasing precedence: built-in defaults → config file → environment
(`STING_*`) → flags. The config file is discovered as `config.yaml` under
`$XDG_CONFIG_HOME/sting`, `~/.config/sting`, `~/.sting`, or the current
directory, or pointed at explicitly with `--config path.yaml`.

| key                | env                     | flag                 | default    | meaning                                  |
|--------------------|-------------------------|----------------------|------------|------------------------------------------|
| `provider`         | `STING_PROVIDER`        | (`--provider`)       | `github`   | provider when unspecified                |
| `token`            | `STING_TOKEN`           | `--token`            | —          | dedicated GitHub PAT                     |
| `base_url`         | `STING_BASE_URL`        | `--base-url`         | github.com | GitHub Enterprise API root               |
| `gitlab_token`     | `STING_GITLAB_TOKEN`    | `--gitlab-token`     | —          | dedicated GitLab PAT                     |
| `gitlab_base_url`  | `STING_GITLAB_BASE_URL` | `--gitlab-base-url`  | GitLab.com | GitLab API v4 root                       |
| `per_page`         | `STING_PER_PAGE`        | `--per-page`         | `100`      | API page size (1–100)                    |
| `max_commits`      | `STING_MAX_COMMITS`     | `--max-commits`      | `0`        | cap on returned commits (0 = unlimited)  |
| `default_scope`    | `STING_DEFAULT_SCOPE`   | (`--scope`)          | `search`   | scope when unspecified                   |
| `default_window`   | `STING_DEFAULT_WINDOW`  | (`--window`)         | `7d`       | look-back when `since` unspecified       |
| `default_repos`    | `STING_DEFAULT_REPOS`   | (`--repos`)          | —          | repo/project list for `repos` scope      |
| `default_org`      | `STING_DEFAULT_ORG`     | (`--org`)            | —          | org/group for `org` scope                |
| `default_format`   | `STING_DEFAULT_FORMAT`  | (`-o`)               | `markdown` | CLI output format                        |
| `include_stats`    | `STING_INCLUDE_STATS`   | (`--stats`)          | `false`    | fetch additions/deletions per commit     |
| `include_files`    | `STING_INCLUDE_FILES`   | (`--files`)          | `false`    | fetch changed file summaries             |
| `include_diffs`    | `STING_INCLUDE_DIFFS`   | (`--diffs`)          | `false`    | fetch bounded patch text                 |
| `max_diff_bytes`   | `STING_MAX_DIFF_BYTES`  | (`--max-diff-bytes`) | `60000`    | per-commit patch byte cap                |

Keys in parentheses are per-query request flags that override the resolved
default for a single invocation. See `config.example.yaml`.

## Development

```sh
task check        # tidy + vet + test
task test:race    # tests under the race detector
task run -- --author octocat --scope repos --repos octocat/Hello-World --since 2008-01-01
```

Package and API reference (godoc):

```sh
go doc ./...                # synopsis of every package
go doc ./ghclient Client    # a specific type
go doc ./gitlabclient Client
```

## Documentation

- [CHANGELOG.md](CHANGELOG.md) — notable changes.
- [Architecture Decision Records](docs/adr/) — why the tool is shaped the way it
  is (single-binary MCP+CLI, dedicated PAT, multi-runtime installer, release
  ownership).
- Per-package godoc — see the `go doc` commands above.

## Layout

Public packages (importable; the evidence contract — see
[ADR 0004](docs/adr/0004-public-packages-and-wake-evidence.md)):

```
model/                domain types (leaf) + Result SchemaVersion
config/               Config, viper keys, window/time parsing, query resolution
ghclient/             go-github wrapper + scope dispatch + normalization
gitlabclient/         GitLab REST wrapper + scope dispatch + normalization
```

Application layer (internal):

```
cmd/sting/            thin entrypoint -> internal/cli
internal/cli/         cobra command tree + viper wiring
internal/commitclient/ provider client selection
internal/render/      JSON + Markdown rendering
internal/mcpserver/   MCP server; read-only get_commits tool
internal/mcpinstall/  runtime adapters (Claude, Codex, OpenCode, Grok)
```

[viper]: https://github.com/spf13/viper
