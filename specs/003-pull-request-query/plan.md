# Implementation Plan: PR Activity and Personal Inbox

**Branch**: `feature/127-pull-request-query` | **Date**: 2026-09-05 | **Spec**: [spec.md](spec.md)

**Input**: Clarified specification in `specs/003-pull-request-query/spec.md`.
**Status**: Phase 0 research and Phase 1 design complete; ready for task generation.

## Summary

Add `sting prs` / `get_prs` for opened, merged, and unmerged-closed activity in a UTC window,
and `sting inbox` / `get_pr_inbox` for currently open authored, assigned, or directly
review-requested PRs without an age cutoff. Preserve existing commit and repository-activity
interfaces and the meaning of `include_prs`.

Reuse go-github REST discovery, the public core, and the shared request-budget transport.
Historical activity uses conservative candidate discovery plus budgeted per-PR event pages;
latest timestamps alone cannot recover a closure followed by reopening. Inbox uses paginated
PR metadata, including repository verification of search candidates for direct review requests.
Return independent versioned results with confirmed matches, dimensions of completeness, cost,
and attributable gaps. No new dependency, provider mutation, optional per-PR enrichment, or
persistent evidence cache is proposed.

## Technical Context

**Language/Version**: Go 1.27.1, as pinned in `go.mod`.

**Primary Dependencies**: Existing go-github/v91 v91.0.0, Cobra v1.10.2, Viper v1.21.0,
MCP Go SDK v1.7.0, and standard library. No module/version changes.

**Storage**: Per-call in-memory evidence, candidate IDs, and inbox repository verification;
existing configuration and sting credential store only. Nothing survives query completion.

**Testing**: Standard library testing, `httptest`, external-package API tests, executable/stdio
integration fixtures, race testing and package coverage gates; isolated HOME/USERPROFILE.

**Target Platform**: Existing Go CLI release platforms and stdio MCP runtimes. GitHub public
and configured Enterprise REST API roots; unsupported endpoint behavior is explicit, not fallback.

**Project Type**: Public Go packages plus CLI and MCP server.

**Performance Goals**: Defaults of 100 returned PR identities and 500 actual provider requests;
phase counts sum to consumption. Discovery scales by pages; only selected lifecycle recovery
permits per-PR requests. Keep existing 30-second per-request and two-minute CLI query timeouts.
No unsupported wall-clock latency promise or estimate-only interface is added.

**Constraints**: GET-only access, dedicated credentials, normalized UTC, deterministic serial
collection, explicit partial evidence, no optional diff/review/check enrichment. History cost
can dominate an old repository; cap behavior and search candidate ceilings remain visible.

**Scale/Scope**: Two workflows, each supporting search/repos/org. Public-only unrestricted
search is sting policy. Existing 10,000-page safety guard remains disclosed. Search's candidate
limits can make a narrow historical window incomplete despite a small output set.

## Constitution Check

Pre-research review was recorded before Phase 0; post-design review completed after the linked
artifacts. PASS means the proposed design satisfies the gate, not that implementation/tests have
already run. Constitution v1.0.0 and accepted ADRs govern; no amendment or exception is needed.

| # | Gate | Evidence | Pre-research | Post-design |
| --- | --- | --- | --- | --- |
| I | Read-only | GET-only operation allowlist; both tools in shared definitions, read-only annotation and four-tool drift checks. | PASS | PASS |
| II | Evidence-grade output | Separate schema/query/workflow envelopes, confirmed occurrences/reasons, costs and targeted disclosures. | PASS | PASS |
| III | Deterministic queries | Shared UTC resolver, serial traversal, stable result ordering, isolated budgets, explicit limits. | PASS | PASS |
| IV | Dedicated config/credentials | Typed applicable defaults, request > env > file > defaults; existing sting credential resolver, no ambient tokens. | PASS | PASS |
| V | Public contract | Additive `model/config/ghclient` surfaces; existing schema constants and consumers unchanged (ADR 0004). | PASS | PASS |
| VI | Partial evidence | Reports survive zero/nonzero-match failures; discovery/history/reason gaps are independent of optional metadata. | PASS | PASS |
| VII | No second authority | In-memory query state only; installer ownership and atomic writes unchanged. | PASS | PASS |
| VIII | Honest scope | GitHub only; history ambiguity/index/visibility/current-state limits documented; proposed interfaces labeled as such. | PASS | PASS |
| — | Tests | Direct offline fixture matrix, public API budget tests, concurrent calls, stdio/read-only drift, race and coverage gates. | PASS | PASS |
| — | Governance | Feature branch and PR-only delivery; signed/DCO Conventional Commits; proposed ADR 0012 and planned README changes. | PASS | PASS |

The Skaphos ecosystem assessment contains no sting PR-query adoption verdict. This design adopts
the already-pinned SDK and internal mechanisms rather than building a new provider client.
See [research R1](research.md#r1--reuse-the-existing-core). The proposed
[ADR 0012](../../docs/adr/0012-pr-activity-and-personal-inbox.md) extends existing decisions without
rewriting or superseding their read-only/public-package rules.

## Project Structure

### Documentation (this feature)

```text
specs/003-pull-request-query/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── checklists/requirements.md
└── contracts/
    ├── cli.md
    ├── mcp.md
    ├── go-public-api.md
    └── provider-collection.md
```

`tasks.md` is the next phase's output and is not created by this command.

### Source Code (proposed changes at repository root)

```text
model/pr.go                           # PR metadata, two queries/results, occurrence/reason evidence
config/pr.go                          # two resolvers and small private PR validation helpers
config/config.go                      # MaxPRs, defaults, and applicable validation support
ghclient/pr.go                        # shared normalization and per-call PR sessions
ghclient/pr_activity.go               # activity discovery and lifecycle evaluation
ghclient/pr_history.go                # lifecycle occurrence normalization
ghclient/pr_inbox.go                  # relationship discovery and repository verification
ghclient/ghclient.go                  # retain private immutable constructor settings for sessions
internal/commitclient/pr.go            # narrow GitHub PR factory and interface
internal/cli/prs.go                    # activity flags and adapter
internal/cli/inbox.go                  # inbox flags and adapter
internal/cli/root.go                   # registration and applicable typed config loading
internal/cli/mcp.go                    # MCP loader with shared startup validation
internal/mcpserver/getprs.go           # typed activity tool
internal/mcpserver/getprinbox.go        # typed inbox tool
internal/mcpserver/server.go            # definitions, shared validation, legacy tool guard
internal/mcpserver/getrepoactivity.go   # preserve legacy validation at the tool boundary
internal/render/pr.go                  # shared metadata helpers and activity renderer
internal/render/pr_inbox.go            # inbox renderer
integration/pr_cli_mcp_test.go         # executable and MCP fixtures
```

Adjacent `_test.go` and appropriate `testdata/` fixtures accompany changed packages. Extend
existing registry/installer/integration expectations, README, `config.example.yaml`, and package
examples. No changes to `gitlabclient`, commit behavior, release dependencies or license notices
are needed. REUSE.toml's aggregate MIT annotation covers generated Markdown.

**Structure Decision:** Follow existing `ActivityRequest` → `ActivityQuery` → collector →
result → renderer adapters. Share small normalization helpers, not a generic workflow engine.
Keep activity and inbox request/result schemas separate; share PR metadata and budget mechanism.

## Phase 0 — Research Decisions

Research is complete in [research.md](research.md). Concrete decisions:

1. Historical `any`/`closed` search uses creation-before-window-end candidates; repos/org list
   all states. No latest-update upper bound can silently erase earlier actions.
2. Per-PR issue events recover lifecycle history. Fully paginate; normalize repeat occurrences
   by event ID, with ambiguity disclosed rather than inventing merge/closure classification.
   Unknown merge-disposition filters defer to history rather than prematurely excluding a
   candidate; conflicting record/event merge times cannot produce an invented window match.
3. Inbox repo listing directly evaluates all three relationships. Search streams include
   broad reviewer candidates; repository-level pages confirm personal requests without per-PR
   optional detail calls. Explain mixed discovery and direct-versus-team scope.
4. Every PR call owns its budget, identity lookup, query state and coverage. Reuse existing
   transport enforcement but not its unmetered quota preflight or activity quota-to-success rule.
5. Confirmed evidence survives failures. Optional missing metadata does not itself mark candidate
   discovery incomplete; unfinished histories and relationship checks do.

## Phase 1 — Design and Contracts

- [Data model](data-model.md): two versioned result/query envelopes, shared metadata with explicit
  unknowns, occurrence IDs, reason completeness, cost categories, and coverage invariants.
- [Provider collection](contracts/provider-collection.md): operation allowlist, search/list/history
  algorithms, private visibility, deterministic scheduling, and error/limit behavior.
- [CLI](contracts/cli.md): independent commands and defaults, draft semantics, applicable config,
  stdout/stderr/exit behavior, and examples.
- [MCP](contracts/mcp.md): concrete input/output types, omission semantics, error preservation,
  request isolation and four-tool registration/installer invariants.
- [Public Go API](contracts/go-public-api.md): additive resolvers/collectors, independent budgets
  even when callers omit constructor budget options, and preserved existing contracts.
- [Quickstart](quickstart.md): executable validation procedure and expected fixture evidence.

### Integration details that must not drift

- Add only the new PR result cap to shared config. Do not bind local PR flags globally in a
  way that changes commit flags or leaks settings between MCP calls.
- Inbox loading validates relevant settings, not unused activity windows or commit-provider
  defaults. Activity still validates its window. Existing standalone loaders/public resolvers
  retain behavior. MCP startup decodes typed config and validates shared settings; workflow
  validation happens per tool before collection. Existing MCP tools retain `Config.Validate`
  at their handler boundaries, moving workflow-default failures from startup to the affected
  call. `mcpserver.New` enforces shared validation for embedded callers too; see the
  [MCP contract](contracts/mcp.md). Never mutate shared defaults to bypass validation.
- Inbox repository verification overrides conflicting search hits, removes disproven reasons
  and nonmatching entries, and prevents later search streams from restoring invalidated work.
  Complete-list absence removes a candidate with a gap disclosure, without inventing its state;
  interrupted verification retains uncontradicted proof. Preserve minimal negative/unknown
  verification facts, counts, costs and coverage as specified in the collection contract.
- Preserve credential precedence and host selection via `internal/commitclient`; query self
  resolution is a counted `GET /user`, not I/O inside the config resolver.
- Public collectors create private sessions from construction settings. Existing commit and
  repository-activity counters, options, and SDK clients are not reset or mutated.
- Render versioned partial reports before returning errors. MCP marks them `IsError` while
  retaining structured content; a zero-match failed query is still evidence of collection failure.
- Keep constructor/validation errors separate from initialized collection reports. Sanitize
  errors; never serialize tokens, raw request objects or arbitrary provider body text.

## Verification and Requirement Coverage

| Area | Required evidence | Requirements |
| --- | --- | --- |
| Resolution | all scopes; current state vs actions; defaults/explicit zero/false; date offsets/bounds; injection rejection; GitLab rejection; inbox ignores activity defaults | FR-002–007, 016, 018–019, 021–022 |
| Historical actions | open/merge/close; reopened later; repeated closure IDs; merge/closure ambiguity; pages with post-window context; candidate superset and search caps | FR-005, 008–010, 023–024 |
| Inbox | authored/assigned/direct review union; old drafts; completed/removed/fulfilled relationships; team-only exclusion; multi-stream dedup; explicit contradiction vs complete-list absence vs interrupted verification; no stale re-admission | FR-018–022, 024 |
| Cost and partial evidence | every endpoint/retry counted; own cap vs quota errors; per-entry vs discovery coverage; exact-cap completion; org skips vs explicit failures; no optional per-PR endpoints | FR-011–013, 023–024 |
| Public/transport | fresh per-call budget without constructor option; sequential/concurrent calls; separate version constants; preserved commit/activity behavior | FR-001, 009, 012, 015 |
| CLI/MCP/rendering | stdout JSON on partial failure; correct exit/IsError; all four tools read-only; installer derived names; no facts unique to Markdown; real MCP startup with invalid unused window, activity validation error and subsequent successful inbox call | FR-001, 014–017, 022 |

Use offline fixture tests and meaningful black-box assertions of returned evidence, dispatched
requests, and failure semantics. New tests must not merely reproduce the helper's implementation.
Do not invoke live provider services or inspect real credentials during tests.

Implementation gates: `go -C tools tool task test-race`, `test-integration`, `test-cover-check`,
`lint`, `staticcheck`, `vuln`, and the existing full local `ci` sequence before pushing. The
coverage task is explicit because the current `ci` task does not itself enforce coverage.
Default package coverage remains 80%, CLI 60%, credentials 72%; no floor changes.

## Risks and Recovery

- Broad historical candidates can exhaust search or request limits before finding much window
  activity. Reports identify the stopped work; callers can narrow targets or use repo/org scope.
- Full lifecycle pages may not resolve merge-associated closures or same-time contradictory
  events. Omit ambiguous actions and mark coverage partial; never use ID sort as historical proof.
- Search and listing can observe different current states. Record detected disagreement and
  avoid a transactional snapshot claim; no persistent cache attempts to become authoritative.
- Search inbox verification costs extra pages per repository. It retains direct-review accuracy
  without adding per-PR optional enrichment; report the mixed discovery method and consumed cost.
- New interfaces are additive. Recovery is reverting/disabling new registrations/commands via
  a PR while preserving existing schemas; installer re-install remains the normal update path.

## Complexity Tracking

No constitutional violations. User-authorized lifecycle-history reads supersede issue #127's
initial no-per-PR cost proposal, while the request ceiling and no-optional-enrichment rule remain.
Separate workflow envelopes and dimensioned coverage are necessary to avoid presenting current
attention and historical actions as interchangeable evidence. No generic framework is introduced.

## Implementation reconciliation

The feature branch implements both workflows with the pinned dependencies. The source/test
split follows this plan; executable parity lives in `integration/pr_cli_mcp_test.go`, and the
collector-backed public serialization assertions use external-package
`model/pr_collection_test.go` to avoid an import cycle. Fixture variants are table-driven Go
values in the named test files, with shared GET-only recording in `ghclient/pr_test.go`.

MCP workflow-default validation is isolated as specified. Repository verification retracts
contradicted inbox matches even if later pagination fails, and observed private repositories
cannot enter an unrestricted public-only search result. No optional enrichment, provider
mutation, new dependency, or persistent cache was introduced. See `quickstart.md` for validation.
