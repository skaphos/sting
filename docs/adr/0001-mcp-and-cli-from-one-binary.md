# 1. Deliver MCP server and CLI from one binary

## Status

Accepted.

## Context

The driving use case is an agent answering "give me all the commits of `<user>`
in the last week and tell me what they're working on." That calls for an MCP
tool an agent can invoke. A terminal-friendly CLI is also useful for ad-hoc
checks and scripting. Both need the same query logic, configuration, scopes, and
output formats.

## Decision

Ship a single binary, `sting`, with a cobra command tree (`internal/cli`):

- `sting <query flags>` runs the query and prints a report.
- `sting mcp` serves the same query over stdio as an MCP `get_commits` tool.

Shared logic lives in dependency-light packages: `internal/model` (domain
types), `internal/config` (settings, window/time parsing, query resolution),
`internal/ghclient` (GitHub access), and `internal/render` (Markdown/JSON). The
CLI and MCP handlers both build a `config.Request`, resolve it to a
`model.Query`, and call `ghclient.Client.Collect`, so behavior cannot diverge
between the two entry points.

## Consequences

- One artifact to build, install, and register; the installer points runtimes at
  `sting mcp`.
- The MCP tool returns structured commit data plus a Markdown summary, so an
  agent has clean material to describe without re-deriving it.
- A single shared core means a fix or scope change applies to both surfaces at
  once.

## Alternatives Considered

- **Separate `sting` and `sting-mcp` binaries.** Rejected: two artifacts to
  install and keep in sync for no benefit; the mode is a subcommand concern.
- **A `--mcp` flag on the root command** (an earlier iteration). Superseded by
  the `mcp` subcommand, which is the cobra-idiomatic form and is what the
  installer registers.
- **CLI only, no MCP.** Rejected: it fails the primary agent-integration goal.
