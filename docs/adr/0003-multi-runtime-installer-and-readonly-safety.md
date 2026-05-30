# 3. Multi-runtime MCP installer and read-only safety model

## Status

Accepted.

## Context

Registering an MCP server by hand differs per agent runtime: Claude Code and
OpenCode use JSON, Codex and Grok use TOML; the entry shape differs (command +
args vs. an argv array; an explicit `enabled` flag for some). Editing these
files by hand is error-prone and risks clobbering unrelated entries. Separately,
sting only reads from GitHub, so its tool can safely be auto-approved — but only
if "read-only" is asserted in one authoritative place rather than hand-copied
into a permissions list that can drift.

## Decision

Provide `internal/mcpinstall` with a `Runtime` adapter per agent (Claude, Codex,
OpenCode, Grok). Each adapter knows its config path (user and, where supported,
project scope), reads/writes/removes only sting's entry, preserves all other
keys, and writes atomically (temp file + rename, preserving mode). The CLI
exposes `install`, `uninstall`, and `install list`, with `--manual` to print
paste-ready snippets instead of writing.

Mark `get_commits` read-only at the source: `mcp.ToolAnnotations{ReadOnlyHint:
true}` on the registered tool, and `mcpserver.ReadOnlyTools()` as the single
list of read-only tool names. The Claude install snippet derives its
`permissions.allow` block from `ReadOnlyTools()`, so the auto-approve list
cannot drift from what the server actually advertises.

## Consequences

- One command registers sting across detected runtimes; config files keep their
  other servers and formatting.
- `install list` reports `registered` / `registered (stale)` / `not registered`
  / `unsupported`, surfacing path drift after an upgrade.
- The read-only claim has a single source of truth used by both the tool
  annotation and the permissions snippet; a test pins the set.
- Adding a runtime means adding one adapter and a selection flag.

## Alternatives Considered

- **Document manual config edits only.** Rejected: error-prone and easy to get
  subtly wrong per runtime; `--manual` still serves users who prefer it.
- **Full file rewrite on install.** Rejected: would drop unrelated entries and
  reformat user files; adapters do a surgical, atomic merge instead.
- **Hardcode the permissions allow-list.** Rejected: it would silently drift
  from the server's annotations; deriving it from `ReadOnlyTools()` keeps them
  in lockstep.
