# sting

Query a GitHub user's commits over a time window and hand them to an LLM agent
(or a terminal) in a consumable form.

`sting` is a single binary with subcommands:

- **`sting mcp`** — runs an MCP server over stdio exposing a single, read-only
  `get_commits` tool, so an agent like Claude Code can answer *"give me all the
  commits of `mendedlink` in the last week and tell me what he's working on."*
- **`sting <query flags>`** — prints a Markdown or JSON report locally.
- **`sting install` / `uninstall` / `install list`** — register the MCP server
  with your agent runtimes (Claude Code, Codex, OpenCode, Grok).

Configuration is resolved with [viper] (`defaults < config file < env < flags`),
so a dedicated read-only PAT can live in sting's own config instead of relying
on the ambient `GITHUB_TOKEN`.

## Install

```sh
go install github.com/skaphos/sting/cmd/sting@latest
# or, in this repo:
task build      # -> ./bin/sting
```

## Authentication

sting uses its **own** token key, deliberately separate from `GITHUB_TOKEN` so a
dedicated read-only PAT does not collide with other tools. Set it in the config
file (`token:`) or via `STING_TOKEN`:

```sh
# config file (recommended): ~/.config/sting/config.yaml
token: ghp_xxx

# or environment
export STING_TOKEN=ghp_xxx
```

Unauthenticated calls work for public data but are heavily rate limited (global
commit search is ~10 requests/min without a token). A classic PAT needs no scopes
for public repos; add read `repo` access to include private repos.

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

> Give me all the commits of `mendedlink` in the last week and tell me what he's
> working on.

The tool returns structured commit data **plus** a Markdown summary, so the
agent has clean material to describe the work.

## CLI usage

```sh
# Last week of an author's commits, anywhere GitHub has indexed (Markdown).
sting --author mendedlink --window 7d

# JSON for piping into other tools.
sting --author mendedlink --window 7d -o json

# Within specific repos (most complete; works on private repos with a token).
sting --author mendedlink --scope repos --repos skaphos/sting,skaphos/other

# Across every repo in an org.
sting --author mendedlink --scope org --org skaphos --window 2w

# Explicit bounds and per-commit line stats.
sting --author mendedlink --since 2026-05-01 --until 2026-05-15 --stats
```

Run `sting --help` (or `sting <command> --help`) for the full flag list.

### Scopes

| scope    | how it finds commits                                              | notes                                              |
|----------|------------------------------------------------------------------|----------------------------------------------------|
| `search` | GitHub commit search by `author:`                                | global (public-only) unless scoped; 1000-result cap |
| `repos`  | lists commits in each `owner/repo` you name, filtered by author  | most complete; supports private repos with a token |
| `org`    | enumerates an org's repos, then lists commits in each            | needs org read access for private repos            |

#### Searching private orgs

A bare `search` is a global author query, which GitHub limits to **public**
repos. To reach a **private** org via the search index, scope the query by
combining `search` with `--org` (or `--repos`):

```sh
# Adds `org:Alaska-Airlines-Shared` to the search query.
sting --author mendedlink --scope search --org Alaska-Airlines-Shared --window 7d
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

| key              | env                   | flag           | default    | meaning                                  |
|------------------|-----------------------|----------------|------------|------------------------------------------|
| `token`          | `STING_TOKEN`         | `--token`      | —          | dedicated GitHub PAT                      |
| `base_url`       | `STING_BASE_URL`      | `--base-url`   | github.com | GitHub Enterprise API root               |
| `per_page`       | `STING_PER_PAGE`      | `--per-page`   | `100`      | API page size (1–100)                    |
| `max_commits`    | `STING_MAX_COMMITS`   | `--max-commits`| `0`        | cap on returned commits (0 = unlimited)  |
| `default_scope`  | `STING_DEFAULT_SCOPE` | (`--scope`)    | `search`   | scope when unspecified                   |
| `default_window` | `STING_DEFAULT_WINDOW`| (`--window`)   | `7d`       | look-back when `since` unspecified       |
| `default_repos`  | `STING_DEFAULT_REPOS` | (`--repos`)    | —          | `owner/repo` list for `repos` scope      |
| `default_org`    | `STING_DEFAULT_ORG`   | (`--org`)      | —          | org for `org` scope                      |
| `default_format` | `STING_DEFAULT_FORMAT`| (`-o`)         | `markdown` | CLI output format                        |
| `include_stats`  | `STING_INCLUDE_STATS` | (`--stats`)    | `false`    | fetch additions/deletions per commit     |

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
```

Application layer (internal):

```
cmd/sting/            thin entrypoint -> internal/cli
internal/cli/         cobra command tree + viper wiring
internal/render/      JSON + Markdown rendering
internal/mcpserver/   MCP server; read-only get_commits tool
internal/mcpinstall/  runtime adapters (Claude, Codex, OpenCode, Grok)
```

[viper]: https://github.com/spf13/viper
