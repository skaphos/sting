# 4. Public packages and Wake evidence shape

## Status

Accepted.

## Context

sting began as a self-contained binary with all of its Go code under
`internal/`, which makes none of it importable by other modules. sting is also
a candidate evidence source for [Wake](https://github.com/skaphos/wake), a
repository-forensics system whose layered model is Evidence → Events → Signals →
Interpretation. Wake's `wake-forensics-mcp` extracts commit evidence from
*local* Git; sting contributes the complementary *GitHub-API* view (cross-org
and cross-repo author search, and private-org reach via a token) that local Git
cannot provide.

Wake's service-boundary decision (`wake/DECISIONS/0001`) requires each evidence
component to expose its logic as importable public Go packages plus a thin
`cmd/` entry point, so `wake-cli` can consume the logic in-process rather than
shelling out to a subprocess. As long as sting's collector lived under
`internal/`, the only integration path was spawning `sting mcp` — exactly the
loose, version-fragile coupling that decision rejects.

## Decision

Promote sting's reusable core to public, top-level packages, mirroring the
`wake-forensics-mcp` layout (public domain packages alongside an `internal/`
application layer):

- `model` — domain types (`Commit`, `Query`, `Result`, `Scope`) and the
  `SchemaVersion` contract identifier.
- `config` — `Config`, `Request`, and `Resolve` (request → validated query).
- `ghclient` — the GitHub collector (`New`, `Client.Collect`).

The CLI (`internal/cli`), MCP server wiring (`internal/mcpserver`), installer
(`internal/mcpinstall`), and output rendering (`internal/render`) stay internal:
they are application/serving concerns, not the evidence contract.

Add provenance to `model.Result`: a `SchemaVersion` (`sting.skaphos.io/v1`) and
a `GeneratedAt` timestamp, set by `Collect`. This gives every result an
inspectable, versioned shape consistent with evidence-backed analysis.

Do **not** take a dependency on `wake-core`. sting stays a standalone tool; the
mapping from `model.Result` to `wake-core`'s `evidence.Bundle` lives on the Wake
side (a `wake-adapters` concern), where the consumer owns the translation.

## Consequences

- `wake-cli` (or any consumer) can `import github.com/skaphos/sting/{model,config,ghclient}`
  and run the collector in-process; the public surface is now a supported API.
- The public packages carry a stable import path; breaking changes to `Commit`
  or `Result` must bump `model.SchemaVersion`.
- sting has no build- or release-time coupling to Wake; the evidence mapping is
  Wake's to maintain and version.
- The CLI, MCP server, installer, and renderer remain free to change without
  affecting the public contract.

## Alternatives Considered

- **Keep everything `internal/`; integrate via `sting mcp` subprocess.**
  Rejected by `wake/DECISIONS/0001`: loose version binding and a weaker
  extension surface than importable packages.
- **Depend on `wake-core` and emit `evidence.Bundle` directly from sting.**
  Rejected: couples sting's releases to Wake's schema and inverts the
  dependency direction; the adapter belongs with the consumer (`wake-adapters`).
- **Promote everything (including the CLI and installer) to public.** Rejected:
  needlessly widens the supported API surface; only the evidence contract needs
  to be importable.
