# GitHub Copilot Instructions for sting

sting queries a GitHub user's commits over a time window, as a local CLI or as
an MCP server exposing a single read-only `get_commits` tool over stdio. It is
read-only by design and authenticates with a dedicated GitHub PAT kept separate
from the ambient `GITHUB_TOKEN`.

## What Good Changes Look Like

- Prefer small, focused pull requests with one logical change.
- Keep code straightforward: small functions, clear names, early returns, simple
  control flow.
- Follow the Go naming and layout conventions already used in this repository.
- Keep the public packages (`model`, `config`, `ghclient`) a minimal, stable API
  surface; breaking changes to `Commit`/`Result` must bump `model.SchemaVersion`.

## Safety Rules (read-only by design)

- sting only reads from GitHub. Do not add tools or commands that mutate
  repositories, issues, pull requests, or any remote state.
- Every exposed MCP tool must be read-only. `get_commits` is annotated
  `ReadOnlyHint: true`, and `mcpserver.ReadOnlyTools()` is the single source of
  truth the installer's Claude auto-approve snippet derives from — keep them in
  lockstep.
- Authentication uses sting's own PAT (`token` / `STING_TOKEN`), deliberately
  separate from `GITHUB_TOKEN`. Do not start reading `GITHUB_TOKEN`.
- The installer writes only sting's own entry, preserves unrelated keys, and
  writes atomically. Do not introduce whole-file config rewrites.

## Codebase Shape

- Public, importable packages: `model/` (domain types + `SchemaVersion`),
  `config/` (settings, window/time parsing, `Resolve`), `ghclient/` (go-github
  wrapper + scope dispatch + normalization).
- Application layer under `internal/`: `cli/` (Cobra + viper), `mcpserver/`
  (MCP `get_commits`), `mcpinstall/` (runtime adapters), `render/` (JSON/Markdown).
- `cmd/sting/` is the thin entrypoint. User docs live in `README.md`; decisions
  in `docs/adr/`.

## Testing Expectations

Prefer the task-based checks (Task is pinned in `tools/`):

- `go -C tools tool task fmt`
- `go -C tools tool task lint`
- `go -C tools tool task test`
- `go -C tools tool task test-cover`
- `go -C tools tool task test-cover-check`
- `go -C tools tool task staticcheck`
- `go -C tools tool task vuln`

New behavior should include direct test coverage. CI enforces a strict 80%
per-package coverage gate. Tests must not hit the network — use
`net/http/httptest` for GitHub calls and isolate `HOME`/`USERPROFILE` for
filesystem tests.

## Go and Repository Conventions

- Use the Go version declared in `go.mod`.
- Keep files `gofmt`/`goimports` clean.
- Maintain REUSE/SPDX metadata: new source files must include an
  `SPDX-License-Identifier` header set to `MIT`; validate with `reuse lint`.
- Tests use the standard library `testing` package; match the existing
  table-driven style.

## Pull Request Instructions

- Explain what changed and why.
- Summarize user-visible or behavior changes clearly.
- List the exact tests and checks run, with outcomes.
- Call out doc updates (`README.md`, `docs/adr/`) when behavior changed.
- Note residual risks, limitations, or follow-up work.

## Commit and Branch Guidance

- Never commit directly to `main`; changes land through pull requests.
- Use focused branch names: `feature/…`, `bug/…`, `chore/…`, `docs/…`, `ci/…`,
  `refactor/…`.
- Use Conventional Commit subjects (`feat:`, `fix:`, `docs:`, `test:`, `ci:`,
  `chore:`, `refactor:`).
- Commits are expected to be cryptographically signed and include a DCO sign-off.

## When Unsure

- Choose the safer, read-only behavior.
- Avoid expanding scope beyond the requested change.
- Match existing command patterns, test style, and output conventions rather
  than inventing new ones.
