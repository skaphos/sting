# Repository Guidelines

sting queries a GitHub or GitLab user's commits over a time window, as a local
CLI or as an MCP server exposing a single read-only `get_commits` tool. It is
read-only by design and uses dedicated provider PATs kept separate from ambient
provider tokens.

## Project Structure & Module Organization

Public packages (importable; the evidence contract — see
[ADR 0004](docs/adr/0004-public-packages-and-wake-evidence.md)):

- `model/`: domain types (`Commit`, `Provider`, `Query`, `Result`, `Scope`) and the
  `Result` `SchemaVersion`. Leaf package, no internal deps.
- `config/`: `Config`, viper keys, window/time parsing, and `Resolve`
  (request → validated `model.Query`).
- `ghclient/`: go-github wrapper, scope dispatch (search/repos/org), and
  normalization into `model` types.
- `gitlabclient/`: GitLab REST wrapper, scope dispatch (repos/org), and
  normalization into `model` types.

Application layer (internal):

- `cmd/sting/`: thin entrypoint that boots `internal/cli`.
- `internal/cli/`: Cobra command tree (`query`, `mcp`, `install`, `uninstall`,
  `version`) and viper wiring.
- `internal/commitclient/`: provider client selection shared by CLI and MCP.
- `internal/mcpserver/`: MCP server; the read-only `get_commits` tool.
- `internal/mcpinstall/`: per-runtime install adapters (Claude, Codex, OpenCode,
  Grok) with atomic, format-preserving config writes.
- `internal/render/`: JSON and Markdown rendering.

Docs: `README.md` (usage), `docs/adr/` (architecture decisions),
`CHANGELOG.md`, `config.example.yaml`.

## Build, Test, and Development Commands

Tasks run without globally installing tools (Task is pinned in `tools/`):

- `go -C tools tool task --list`: list tasks.
- `go -C tools tool task build`: build the local binary.
- `go -C tools tool task test`: run the test suite (stdlib `testing`).
- `go -C tools tool task test-cover`: tests with coverage output.
- `go -C tools tool task test-cover-check`: enforce the per-package coverage gate.
- `go -C tools tool task lint`: golangci-lint (gofmt/goimports + static checks).
- `go -C tools tool task staticcheck` / `vuln`: staticcheck and govulncheck.
- `go -C tools tool task build-ci`: goreleaser snapshot build for all platforms.
- `go -C tools tool task ci`: run the full local CI sequence.

## Coding Style & Naming Conventions

- Go version: see `go.mod` (`go 1.26.3`).
- Formatting: `gofmt` and `goimports`, enforced via `golangci-lint`.
- Naming: standard Go conventions (exported `PascalCase`, unexported `camelCase`).
- Tests: filename suffix `_test.go`; keep fixtures under `testdata/`.

## Testing Guidelines

- Framework: the standard library `testing` package; table-driven tests where natural.
- New behavior must ship with direct test coverage in the same change.
- CI enforces a strict 80% per-package coverage gate (`scripts/check-coverage.sh`);
  run `go -C tools tool task test-cover-check` locally before pushing.
- No network in tests: use `net/http/httptest` for GitHub calls and isolate
  `HOME`/`USERPROFILE` for filesystem-touching tests.

## Engineering Guardrails

- Keep cognitive load low: small functions, clear names, early returns, simple
  control flow over clever abstractions.
- Comment the why (invariants, edge cases, non-obvious tradeoffs), not the what.
- Keep the public packages (`model`, `config`, `ghclient`, `gitlabclient`) a
  deliberate, minimal API surface; breaking changes to `Commit`/`Result` must bump
  `model.SchemaVersion`.

## Safety Notes (read-only by design)

- sting only reads from GitHub and GitLab. Do not add tools or commands that mutate
  repositories, issues, or any remote state.
- `get_commits` is annotated `ReadOnlyHint: true`; `mcpserver.ReadOnlyTools()`
  is the single source of truth the installer's auto-approve list derives from.
  Keep that invariant — every exposed tool must be read-only.
- Authentication uses sting's own PAT keys (`token` / `STING_TOKEN` for GitHub,
  `gitlab_token` / `STING_GITLAB_TOKEN` for GitLab), intentionally separate
  from ambient provider tokens. Do not start reading `GITHUB_TOKEN` or
  `GITLAB_TOKEN`.
- The installer writes only sting's entry, preserves other keys, and writes
  atomically. Do not add behavior that rewrites whole config files.

## Commit & Pull Request Guidelines

- **All changes land via pull request. Never commit directly to `main`.**
- Branch naming by change type: `feature/…`, `bug/…`, `chore/…`, `docs/…`,
  `ci/…`, `refactor/…`. One logical change per PR.
- **Commits must be cryptographically signed AND carry a DCO sign-off** (pass
  both `-S` and `-s`, e.g. `git commit -S -s …`).
- Use Conventional Commits so `skaphos/actions/release-pr` can infer the bump
  via `svu`:
  - `feat:` → minor; `fix:` / `perf:` → patch.
  - `docs:`, `test:`, `ci:`, `chore:`, `refactor:` → no bump by default.
  - `!` in type/scope or a `BREAKING CHANGE:` footer → major.
  - Squash-merge subjects must also be Conventional Commits.
- PRs should include: summary, why, testing performed (commands + results), and
  doc updates when behavior changes (`README.md`, `docs/adr/`).

## Documentation Expectations

- Update `README.md` for user-visible behavior changes.
- Add a new ADR under `docs/adr/` for architecturally significant decisions;
  ADRs are immutable — supersede rather than rewrite.
- Regenerate `third_party_licenses/` with `go -C tools tool task notices`
  whenever `go.mod`/`go.sum` changes, and review `THIRD_PARTY_NOTICES.md`.
