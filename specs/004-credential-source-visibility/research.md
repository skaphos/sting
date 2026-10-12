# Research: Credential Source Visibility

Phase 0 output for [plan.md](plan.md). The Technical Context had no open NEEDS CLARIFICATION
items. The decisions below settle the design questions raised by the current code.

## R1. Where the report is produced

- **Decision**: In `internal/commitclient`, where `resolveGitHubToken` / `resolveGitLabToken`
  already run for all eight query surfaces. These functions return a resolution record along
  with the token, and the three constructors (`New`, `NewActivity`, `NewPR`) emit it.
- **Rationale**: Every CLI command and MCP tool already funnels through these three
  constructors (`internal/cli/{query,activity,prs,inbox}.go`, and
  `internal/mcpserver/{server,getrepoactivity,getprs,getprinbox}.go`). Putting the report there
  covers FR-006 by construction, and FR-007 (per MCP call) as well, because each tool handler
  builds a fresh client per call.
- **Alternatives considered**: Logging in each caller would mean 8 call sites that can drift.
  Logging inside `internal/credentials` would miss the dedicated-PAT and anonymous outcomes,
  which never reach the store.

## R2. How the report reaches stderr

- **Decision**: A variadic functional option, `commitclient.WithCredentialReport(w io.Writer)`.
  A nil writer, or no option at all, is silent. `mcpserver.New` gains a matching variadic
  `mcpserver.WithDiagnostics(w io.Writer)` that the handler threads into every constructor.
- **Rationale**: Existing call sites and tests compile unchanged (FR-003, SC-002). The writer
  is injected, so tests assert on a `bytes.Buffer` without capturing the process's stderr.
  Variadic options already have precedent in this codebase (`ghclient.WithRequestBudget`).
- **Alternatives considered**: `log/slog`. sting has no structured logging, and every existing
  diagnostic is a `sting: warning: …` line written with `fmt`, so a logger adds a dependency
  surface for a single line (KISS/YAGNI). A package-level writer is global mutable state and
  breaks test isolation.

## R3. The verbosity setting

- **Decision**: A persistent boolean flag `--verbose` (no shorthand), bound to the viper key
  `verbose`. It is therefore also `STING_VERBOSE` and config-file `verbose: true` (FR-004). The
  CLI reads it in one helper that returns `cmd.ErrOrStderr()` when on and `nil` when off.
- **Rationale**: The flag/env/file precedence comes from the existing viper wiring at no extra
  cost. An MCP runtime can turn it on without changing the server's arguments, through either
  `STING_VERBOSE` in the runtime's env block or `verbose: true` in sting's config. No shorthand,
  because `-v` is conventionally read as "version" and keeping it free avoids ambiguity.
- **Alternatives considered**: Adding `Verbose` to the public `config.Config`. Rejected: ADR 0004
  keeps the public `config` package focused on query defaults (the `credentials` package
  documentation makes the same point), and verbosity is an application concern. Leveled
  verbosity (`-vv`) is not needed for a single line (spec Assumptions).
- **Installer impact**: None. The installer is not changed to forward `STING_VERBOSE`. It is a
  diagnostic toggle users set on purpose, and the README documents both ways to enable it.

## R4. Telling "not stored" apart from "lookup failed"

- **Decision**: Add an exported sentinel, `credentials.ErrNotFound`, and wrap the store's final
  "no credential found" error with it. `commitclient` reports every other `Load` error, and any
  `credentials.New()` error (an unreadable hosts file), as a failed lookup (FR-008).
  Resolution itself does not change: a failed lookup still falls through to anonymous (FR-010).
- **Rationale**: Today both cases come back as an opaque error and collapse into anonymous. The
  "a stored credential … exists but the keyring is unavailable" case is the store's own
  "fail loudly" path, and `commitclient` silently discards it. That is the most valuable thing
  to expose.
- **Secret safety**: The store's error messages contain provider, host, file path, and the
  keyring or OS error text. None of these contain token material. A test asserts that the
  report never contains the token value (FR-009, SC-004).

## R5. Source vocabulary

- **Decision**: The report uses the `credentials.Source` and `credentials.TokenType` values the
  store already returns, plus two outcomes that `commitclient` adds itself: `dedicated PAT` and
  `anonymous`. Ambient sources (`SourceEnv` / `SourceConfig`, reachable only when
  `STING_ALLOW_AMBIENT_GITHUB_TOKEN` is set) get an explicit "ambient … opt-in" label.
- **Rationale**: Reusing the store's enum avoids a second vocabulary that could drift from it.
  The ambient path can only be reached through the opt-in, so labelling it "opt-in" is
  accurate, and it keeps that path visibly separate from sting-owned credentials (Principle IV).

## R6. Test seam for stored credentials

- **Decision**: A package-level `openCredentialStore = credentials.New` in `commitclient`,
  which tests replace with an in-test fake `credentials.Store`.
- **Rationale**: `credentials.New()` uses the real system keyring. The existing
  `credentials.WithKeyringForTest` seam lives in an `_test.go` file and cannot be reached from
  another package. A fake `Store` covers every `Source`, every token type, and every error path
  deterministically, without touching the keyring or the network. This follows the existing
  `collectCommits` package-variable seam in `mcpserver`.
