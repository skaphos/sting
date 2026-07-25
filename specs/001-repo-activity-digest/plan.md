# Implementation Plan: Repository Activity Digest

**Branch**: `feature/repo-activity-digest` | **Date**: 2026-07-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-repo-activity-digest/spec.md`

## Summary

Add a repository-scoped, cost-bounded activity query: list a window's commits cheaply, derive the
window's file-level change set from a single boundary comparison, correlate the two with honestly
labeled attribution, and report what the whole thing cost.

The technical approach rests on three findings from Phase 0 research, each verified against the
code and the pinned dependency:

1. **The base commit is free.** `github.RepositoryCommit.Parents` is populated in the
   *list-commits* response, not only in commit detail. The earliest in-window commit's first
   parent is the comparison base, so FR-008 costs **zero** extra requests. `fromRepoCommit`
   currently discards `Parents`; capturing it is the whole change.
2. **Request accounting belongs in the transport.** A counting `http.RoundTripper` wrapped around
   the existing `&http.Client{Timeout: httpTimeout}` gives exact consumption, ceiling
   enforcement, and rate-header capture in one place — and it covers the existing author-scoped
   paths without touching a single call site, which is most of User Story 4.
3. **An exact cost estimate costs one request.** A `per_page=1` list-commits probe returns
   `Response.LastPage`, which equals the exact commit count in the window. That turns FR-011 from
   a heuristic into arithmetic and makes SC-005 (±20%) trivially satisfiable.

The result is a new public `model.ActivityResult` type, a new `get_repo_activity` MCP tool, a new
`sting activity` CLI command, and a shared budget layer that retrofits cost control onto the
existing query paths. GitHub only; GitLab is rejected explicitly.

## Technical Context

**Language/Version**: Go 1.26.5 (per `go.mod`; toolchain requirement satisfied)

**Primary Dependencies**: `github.com/google/go-github/v82` (provider client),
`github.com/modelcontextprotocol/go-sdk v1.6.1` (MCP), `github.com/spf13/cobra` (CLI),
`github.com/spf13/viper` (config). No new dependencies are required by this feature.

**Storage**: N/A — provider state is authoritative; nothing is persisted (Principle VII)

**Testing**: stdlib `testing` with `net/http/httptest` fixtures; no network access in tests;
`HOME`/`USERPROFILE` isolated for any filesystem-touching test

**Target Platform**: Linux, macOS, and Windows — single binary serving both a local CLI and a
stdio MCP server (ADR 0001)

**Project Type**: Single Go module (CLI + MCP server from one binary)

**Performance Goals**: ≤15 provider requests for a one-week window of ≤500 commits (SC-001);
under 10 s wall clock for the same (SC-004); estimate within ±20% of actual (SC-005)

**Constraints**: Read-only against every provider surface (Principle I); deterministic results
for identical upstream state (FR-023); per-package coverage gate ≥80% (`internal/cli` 60%,
`internal/credentials` 72%, per `scripts/check-coverage.sh`)

**Scale/Scope**: One repository and one reference per activity query; windows of days to weeks;
change sets bounded by the provider's own comparison caps (250 commits / 300 files) with
truncation disclosed

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Derived from `.specify/memory/constitution.md` v1.0.0.

**Initial evaluation (pre-Phase 0)**: all gates PASS. **Post-Phase 1 re-evaluation**: all gates
still PASS; the gates marked below gained sharper evidence from the design.

| # | Gate | Check | Status |
|---|------|-------|--------|
| I | Read-only by design | `get_repo_activity` is `ReadOnlyHint: true`; `ReadOnlyTools()` is refactored to derive from the same tool-definition slice `New()` registers, so a tool cannot be registered without appearing in the auto-approve list | **PASS** |
| II | Evidence-grade output | `ActivityResult` carries provider, repo, reference, resolved boundaries, normalized window, schema version, and generation time; every `Disclosure` and error carries a reason and, where one exists, a next action | **PASS** |
| III | Deterministic queries | Window normalized once in `config.ResolveActivity`; boundaries resolved by ancestry not timestamp (FR-008a); default ceiling is a fixed constant, never quota-derived (FR-012a); **ceiling enforcement is checked before dispatching a concurrent batch, and clipping is by commit order rather than completion order** — see research R3, which closes the one place concurrency could have made truncation nondeterministic | **PASS** |
| IV | Explicit config, dedicated credentials | The new `max_requests` key flows through the existing viper precedence (flags > env > file > defaults); no new credential path and no ambient token read; the budget layer records counts and rate headers only, never tokens | **PASS** |
| V | Public packages are the contract | `model.ActivityResult` is **additive** — `Commit`/`Result` are untouched and `model.SchemaVersion` is **not** bumped, so Wake is unaffected; `ghclient.New` gains a variadic `...Option` (source-compatible); the budget mechanism lives in `internal/`, only its data shape is public; `model` still imports nothing internal | **PASS** |
| VI | Partial results over blindness | Budget and rate-limit stops return a populated `ActivityResult` plus an attributable `Disclosure`; **this also closes an existing gap** — `ghclient.Collect` currently aborts wholesale when `enrichDetails` fails (`return model.Result{}, err`), which Story 4 converts to a partial result | **PASS** |
| VII | Never a second source of truth | Nothing is persisted; the installer gains one additional tool name written through the existing atomic, format-preserving writers, touching only sting's own entry | **PASS** |
| VIII | Technical precision, honest scope | GitLab is rejected with a provider-specific reason and the gap is documented (FR-025); the net-comparison blind spots are disclosed in output rather than glossed (FR-018); attribution is labeled observed or inferred (FR-017) | **PASS** |
| — | Testing non-negotiables | Every new behavior ships tests in the same change, all against `httptest` fixtures; the budget layer is directly unit-testable because it is a `RoundTripper`; new packages must clear the 80% gate | **PASS** |
| — | Governance | PR-only, signed + DCO, Conventional Commits; **ADR 0010 is required** for the single-tool → multi-tool MCP server change (ADR 0001 is immutable and is not edited); `README.md` and `config.example.yaml` updated for the new command, tool, and config key | **PASS** |

## Project Structure

### Documentation (this feature)

```text
specs/001-repo-activity-digest/
├── spec.md              # Feature specification (/speckit-specify, /speckit-clarify)
├── plan.md              # This file (/speckit-plan)
├── research.md          # Phase 0 output (/speckit-plan)
├── data-model.md        # Phase 1 output (/speckit-plan)
├── quickstart.md        # Phase 1 output (/speckit-plan)
├── contracts/           # Phase 1 output (/speckit-plan)
│   ├── go-public-api.md         # model.ActivityResult, ghclient options
│   ├── mcp-get-repo-activity.md # MCP tool input/output schema
│   └── cli-activity.md          # `sting activity` flags and exit behavior
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
model/
├── model.go               # UNCHANGED — Query, Commit, Result, SchemaVersion
└── activity.go            # NEW — ActivityQuery, ActivityResult, ChangeSet, ChangedPath,
                           #       Boundaries, Correlation, CostReport, Disclosure,
                           #       ActivitySchemaVersion

config/
├── config.go              # + MaxRequests field, DefaultMaxRequests const, Defaults() key
├── resolve.go             # + ceiling plumbed into model.Query
└── activity.go            # NEW — ActivityRequest -> model.ActivityQuery; GitHub-only guard

ghclient/
├── ghclient.go            # fromRepoCommit captures Parents; New(...Option); Collect returns
│                          #   partial results instead of aborting on enrich failure
├── activity.go            # NEW — CollectActivity: probe, list, compare, correlate, disclose
└── options.go             # NEW — Option, WithRequestBudget

internal/
├── apibudget/             # NEW — counting/enforcing RoundTripper, rate-header capture,
│                          #       ErrBudgetExceeded, CostReport accumulation
├── activity/              # NEW — provider-agnostic correlation rules + disclosure builders
├── commitclient/          # + activity-capable client resolution; GitLab rejection
├── mcpserver/
│   ├── server.go          # tool registry; ReadOnlyTools() derived from it
│   └── getrepoactivity.go # NEW — get_repo_activity tool handler
├── render/
│   ├── render.go          # UNCHANGED public behavior for model.Result
│   └── activity.go        # NEW — Markdown/JSON view of ActivityResult
└── cli/
    ├── root.go            # + activity command registration
    └── activity.go        # NEW — `sting activity` command

docs/adr/
└── 0010-multi-tool-mcp-server.md   # NEW — supersedes ADR 0001's single-tool framing
```

**Structure Decision**: The existing layout is kept exactly as-is; this feature adds files
alongside their peers rather than introducing a new top-level arrangement. Three placement
decisions are load-bearing:

- **`model/activity.go`, not a new package.** ADR 0004 makes `model` the evidence contract other
  tools import. A sibling file in the same package keeps the public surface in one place while
  keeping `ActivityResult` a *distinct type* from `Result`, which is what the clarification
  decided.
- **`internal/apibudget`, not a public package.** The *data* (`CostReport`) is public because it
  is part of the result contract; the *mechanism* stays internal so Principle V's "minimal public
  API" holds. A public package importing an internal one is legal within the module.
- **`ghclient/activity.go` beside `ghclient.go`.** The activity path shares the client, the error
  classification (`skipRepoReason`, `isRateLimited`), and the pagination discipline already in
  that package. Splitting it out would duplicate all three.

## Phase Status

- [x] Phase 0 — research complete → [research.md](./research.md) (10 decisions, 0 unresolved)
- [x] Phase 1 — design complete → [data-model.md](./data-model.md),
      [contracts/](./contracts/), [quickstart.md](./quickstart.md)
- [ ] Phase 2 — task breakdown (`/speckit-tasks`)

## Complexity Tracking

> Fill ONLY if Constitution Check has violations that must be justified.

**No constitution gate failed**, so no justification is required.

Two decisions carry deliberate, accepted cost. They were taken with the user during
`/speckit-clarify` and are recorded here for review visibility, not as deviations:

| Accepted cost | Why it was chosen | Cheaper alternative and why it was rejected |
|---|---|---|
| A second public result type (`ActivityResult`) rather than extending `Result` | Keeps the change additive: `model.SchemaVersion` is not bumped and Wake needs no coordination | Extending `Result` is one fewer type, but the ADR 0007 precedent means added fields bump the schema version, forcing downstream coordination for no functional gain |
| A second MCP tool rather than a mode of `get_commits` | Two genuinely different result shapes stay behind two honest schemas; `get_commits` keeps its pinned contract | One overloaded tool avoids ADR 0010 and a second installer entry, but returns two incompatible shapes from one schema — exactly the ambiguity FR-017/FR-018 exist to prevent |
