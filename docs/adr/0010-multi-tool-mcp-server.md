# 10. Multi-tool MCP server

Date: 2026-07-25

## Status

Accepted

## Context

ADR 0001 framed sting's MCP server as exposing a single read-only `get_commits`
tool, and the code follows that framing literally: `mcpserver.ReadOnlyTools()`
returns a hardcoded `[]string{"get_commits"}` while its own doc comment calls
itself "the single source of truth ... so the install permissions snippet cannot
drift". With exactly one tool that claim held by coincidence rather than by
construction.

The repository activity digest adds a second tool, `get_repo_activity`, which
answers a different question (what happened in a repository over a window)
returning a different shape (`model.ActivityResult`). Folding it into
`get_commits` as a mode would return two incompatible shapes from one schema —
the ambiguity the evidence contract exists to prevent.

Two hand-synchronized lists — the registrations in `New()` and the names in
`ReadOnlyTools()` — would make Constitution Principle I conventional rather than
mechanical. The failure mode is silent: a tool registered but absent from the
list simply stops being auto-approved, which reads as a runtime quirk rather
than a bug.

ADRs are immutable, so ADR 0001 is not edited; this record supersedes its
single-tool framing only.

## Decision

sting's MCP server exposes multiple read-only tools, and the read-only tool list
is **derived** rather than maintained:

- Tool definitions live in one slice inside `internal/mcpserver`. Both `New()`
  (registration) and `ReadOnlyTools()` (the installer auto-approve list) read
  from that slice.
- Every tool in the slice is annotated `ReadOnlyHint: true`. A test asserts that
  every tool the registry defines appears in `ReadOnlyTools()` and carries the
  hint, so registering a tool without read-only annotation fails the build
  rather than degrading auto-approval at runtime.
- `get_repo_activity` is the second tool. `get_commits` keeps its existing
  schema and behavior unchanged.

The read-only invariant itself is unchanged: sting still issues no non-GET
request to any provider.

## Consequences

- The installer's Claude permissions snippet gains a `get_repo_activity` entry.
  Users who installed before this change pick it up on re-install; until then
  the tool still works, just prompted rather than auto-approved.
- Principle I becomes mechanical: the auto-approve list cannot drift from what
  the server actually registers, because there is only one list.
- Adding a third tool is now a one-entry change to the definition slice, with
  the drift test covering the invariant automatically.
- ADR 0001's "single read-only tool" framing is superseded. Its core decision —
  one binary serving both the CLI and the stdio MCP server — is untouched.
