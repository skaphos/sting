---
description: "Task list for Repository Activity Digest"
---

# Tasks: Repository Activity Digest

**Input**: Design documents from `/specs/001-repo-activity-digest/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/](./contracts/), [quickstart.md](./quickstart.md)

**Tests**: REQUIRED. Per `.specify/memory/constitution.md`, new behavior ships with direct test
coverage in the same change and every bugfix ships a regression test. Tests MUST NOT touch the
network (use `net/http/httptest`) and MUST isolate `HOME`/`USERPROFILE` when touching the
filesystem. The per-package coverage gate (80% default; `internal/cli` 60%) must pass.

**Organization**: Grouped by user story so each is independently implementable and testable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel — different files, no dependency on incomplete work
- **[Story]**: `[US1]`–`[US4]`, mapping to the user stories in spec.md
- Every task names an exact file path

## Path Conventions

This is a **single Go module**. Go tests are **co-located** with the code they exercise
(`foo_test.go` beside `foo.go`) — there is no `tests/` directory. Public packages sit at the
repository root (`model/`, `config/`, `ghclient/`, `gitlabclient/`); everything else lives under
`internal/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Record the governance decision and close the one open research question before code
depends on either.

- [X] T001 Write ADR 0010 recording the single-tool → multi-tool MCP server decision in `docs/adr/0010-multi-tool-mcp-server.md`, following the format of `docs/adr/0007-commit-file-and-diff-evidence.md` (Status: Accepted; Context: ADR 0001 framed the server as exposing one `get_commits` tool; Decision: register a second read-only tool and derive `ReadOnlyTools()` from the registration list; Consequences: installer auto-approve blocks gain an entry, read-only invariant unchanged). Do NOT edit ADR 0001 — ADRs are immutable.
- [X] T002 [P] Add the ADR 0010 entry to the index list in `docs/adr/README.md`, matching the existing `- [NNNN — Title](file.md)` format.
- [X] T003 [P] Resolve research R2 in `specs/001-repo-activity-digest/research.md`: confirm against the live GitHub API whether list-commits `since`/`until` filter on committer date or author date, and replace the verification-task paragraph with the verified answer. This determines the value of `ActivityResult.WindowDateBasis`; it changes a label only, not the design.

**Checkpoint**: The architectural decision is recorded and the one unresolved provider-semantics
question is answered.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The type contract, the budget mechanism, request resolution, and the tool registry.
Every user story depends on these.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T004 [P] Create all activity types in `model/activity.go` per [data-model.md](./data-model.md): `ActivitySchemaVersion`, `DefaultMaxRequests = 500`, `ActivityQuery`, `ActivityResult`, `ActivityCommit` (with `AuthorDate`, `CommitterDate`, `ParentSHAs`, `Enriched`), `Boundaries`, `ChangeSet`, `ChangedPath`, `Correlation`, `CostReport`, `Disclosure`, and `ActivityCommit.Summary()`. Do NOT modify `model/model.go` — `Result`, `Commit`, and `SchemaVersion` stay untouched.
- [X] T005 [P] Write type tests in `model/activity_test.go`: `Summary()` first-line extraction including empty and single-line messages, JSON round-trip preserving `omitempty` behavior, and an assertion that `SchemaVersion` (the existing constant) is still `sting.skaphos.io/v2` so an accidental bump fails the build.
- [X] T006 [P] Implement the counting/enforcing transport in `internal/apibudget/budget.go`: a `RoundTripper` wrapping a delegate, an atomic request counter, `ErrBudgetExceeded` as a sentinel, capture of `X-RateLimit-Limit`/`Remaining`/`Reset` from every response, a `Remaining()` accessor for check-before-dispatch (research R3), and a `Report()` returning `model.CostReport`.
- [X] T007 [P] Write transport tests in `internal/apibudget/budget_test.go` using a stub `RoundTripper`: exact counting across N calls, `ErrBudgetExceeded` returned at the ceiling and not before, ceiling of 0 meaning uncapped-but-still-counted, rate headers parsed including absent and malformed values, and concurrent-safety under `-race`.
- [X] T008 Add `MaxRequests int` to `Config` in `config/config.go`, a `DefaultMaxRequests` wiring to `model.DefaultMaxRequests` in `Default()`, the `max_requests` key in `Defaults()`, and a `>= 0` check in `Validate()`.
- [X] T009 Implement `ActivityRequest` and `(Config) ResolveActivity` in `config/activity.go` per [contracts/go-public-api.md](./contracts/go-public-api.md): normalize the window once to UTC, reuse the existing `validGitHubRepo` validator, apply flags > env > file > defaults, and reject any non-GitHub provider with `provider %q does not support repository activity (github only)`. Depends on T004, T008.
- [X] T010 Write resolution tests in `config/activity_test.go` as a table: valid window forms (`--window`, `since`/`until`, defaults), missing repo, malformed repo, `since` after `until`, negative `max_requests`, GitLab rejection, and that identical requests at a fixed `now` normalize identically. Depends on T009.
- [X] T011 Add `Option` and `WithRequestBudget(ceiling int) Option` in `ghclient/options.go`, and widen `New` in `ghclient/ghclient.go` to `New(token, baseURL string, perPage int, opts ...Option)`, installing the `apibudget` transport around the existing `&http.Client{Timeout: httpTimeout}`. The variadic keeps all existing three-argument call sites source-compatible. Depends on T006.
- [X] T012 [P] Write option tests in `ghclient/options_test.go`: existing three-argument `New` calls still compile and behave identically, `WithRequestBudget` installs a transport that counts, and enterprise base URL handling is unaffected by the wrapper.
- [X] T013 Refactor `internal/mcpserver/server.go` so tool definitions live in one slice that both `New()` registration and `ReadOnlyTools()` read from, replacing the hardcoded `[]string{"get_commits"}` literal (research R6). Behavior for the existing tool must not change.
- [X] T014 Add a drift test in `internal/mcpserver/server_test.go` asserting that every tool the registry defines appears in `ReadOnlyTools()` and carries `ReadOnlyHint: true` — the test that makes Constitution Principle I mechanical rather than conventional. Depends on T013.

**Checkpoint**: Types, budget mechanism, request resolution, and tool registry are in place. User
story work can begin.

---

## Phase 3: User Story 1 — Ask what happened in a repository over a window (Priority: P1) 🎯 MVP

**Goal**: A developer names a repository and a window and gets the commits plus the aggregate
change set, without naming an author and without paying a per-commit cost.

**Independent Test**: `sting activity --repo owner/name --window 7d` returns commits with full
messages and a change set; the run issues a number of requests bounded by commit *pages*, not
commit count; and `sting query --window 7d` still fails with "author is required", proving the new
path is genuinely author-free.

### Tests for User Story 1 (REQUIRED — constitution: testing non-negotiables) ⚠️

> Write these first and confirm they fail before implementing.

- [X] T015 [P] [US1] Write provider-behavior tests in `ghclient/activity_test.go` using `net/http/httptest` fixtures covering: a happy-path window, `parents` present on list responses, an empty window (base == head, `identical` status), a root-commit window with no parent, a `diverged` comparison status, a comparison truncated at the 300-file cap, and a 403 that is a permission denial versus a 403 that is a rate limit (`isRateLimited` already distinguishes them).
- [X] T016 [P] [US1] Write the request-count test in `ghclient/activity_cost_test.go` using a fake `RoundTripper`: a 250-commit window must issue ≤ 15 requests (SC-001) and must issue **zero** per-commit detail requests when enrichment is not requested (FR-006). This is the executable form of the feature's central promise — a regression that reintroduces per-commit fetching fails here.
- [X] T017 [P] [US1] Write rendering tests in `internal/render/activity_test.go`: Markdown includes resolved query, both boundary SHAs, commits, change set, and a visible disclosures section; JSON output is `model.ActivityResult` verbatim; and `ChangeSet.Paths` renders in sorted order.
- [X] T018 [P] [US1] Write CLI tests in `internal/cli/activity_test.go` with `HOME`/`USERPROFILE` isolated: flag parsing into `ActivityRequest`, `--repo` required, `--format json` emitting the contract, and exit code 0 on a successful run.
- [X] T019 [P] [US1] Write MCP tool tests in `internal/mcpserver/getrepoactivity_test.go`: input schema maps to `ActivityRequest`, a resolution error is returned as the handler's error value (never a zero-value structured payload), and a panic in the collect chain is recovered into a tool-level error result rather than killing the server.

### Implementation for User Story 1

- [X] T020 [US1] Capture `Parents` and the committer date in `fromRepoCommit` in `ghclient/ghclient.go` (research R1). This is the change that makes the comparison base free — the data is already in the list response and is currently discarded. Keep `model.Commit` unchanged; surface the new fields on `model.ActivityCommit` only.
- [X] T021 [US1] Implement window commit listing in `ghclient/activity.go`: paginate list-commits with the normalized window and optional author filter, honor the existing `maxPages` guard, and preserve provider ordering (newest first).
- [X] T022 [US1] Implement boundary resolution in `ghclient/activity.go`: head is the latest in-window commit; base is `ParentSHAs[0]` of the earliest in-window commit (`BaseSource = "parent-of-earliest"`); a root commit with no parent yields `BaseSource = "repository-root"` (FR-008b); an empty window yields base == head with `identical` status. Resolve by ancestry, never by timestamp proximity (FR-008a). Depends on T020, T021.
- [X] T023 [US1] Implement the change set in `ghclient/activity.go`: one `Repositories.CompareCommits` call with `&ListOptions{PerPage: 1}` (paginates commits away, leaves `Files` intact — research R7), map `Files` to `[]ChangedPath` including renames via `PreviousPath`, sort `Paths` lexicographically for determinism, and set `Truncated` when the provider's file cap is hit. When `Status == "diverged"`, leave the change set empty. Depends on T022.
- [X] T024 [US1] Implement disclosure construction in `internal/activity/disclose.go`: `reference-scoped` and `net-comparison-blindspot` emitted **unconditionally** whenever a change set is produced (FR-018 — a clean result still carries them), plus `ancestry-diverged`, `provider-capped`, `patch-truncated`, and `author-filter-not-applied`. Each carries a reason and, where one exists, a next action.
- [X] T025 [US1] Implement `CollectActivity` in `ghclient/activity.go`, assembling `ActivityResult` with schema version, generation time, resolved query echo, boundaries, commits, change set, cost report, and disclosures. It must return a populated result — never a bare error — on every non-fatal path. Depends on T021–T024.
- [X] T026 [US1] Implement the `ActivityResult` view in `internal/render/activity.go`: Markdown in the order resolved query → boundaries → commits → change set → cost → disclosures, plus JSON passthrough. Render disclosures as a visible section, not a footnote — an agent that misses the blind-spot disclosure will overstate the evidence. Reuse the existing `codeSpan`/`codeFence` helpers from `internal/render/render.go`.
- [X] T027 [US1] Add activity client resolution in `internal/commitclient/commitclient.go`: reuse the existing GitHub token resolution and construct the client with `WithRequestBudget`. Do not add a GitLab path — rejection happens in `ResolveActivity` (research R9).
- [X] T028 [US1] Implement the `sting activity` command in `internal/cli/activity.go` per [contracts/cli-activity.md](./contracts/cli-activity.md) with flags `--repo`, `--ref`, `--window`, `--since`, `--until`, `--author`, `--include-diffs`, `--max-diff-bytes`, `--format`, and register it in `internal/cli/root.go`. Keep the command thin — flag parsing and request construction only — so the testable logic stays in `config` and `ghclient` (`internal/cli` carries a 60% coverage floor).
- [X] T029 [US1] Implement the `get_repo_activity` tool in `internal/mcpserver/getrepoactivity.go` per [contracts/mcp-get-repo-activity.md](./contracts/mcp-get-repo-activity.md): `GetRepoActivityInput`, `ReadOnlyHint: true`, deferred `recover()` matching the `getCommits` pattern, structured `ActivityResult` output plus a Markdown text view. Register it in the T013 tool slice. Omit any `provider` field — the tool is GitHub-only. Depends on T013, T025, T026.

**Checkpoint**: User Story 1 is fully functional and independently testable. This is the MVP.

---

## Phase 4: User Story 2 — Know and cap what a query will cost (Priority: P2)

**Goal**: See a query's projected cost before running it, cap what it may consume, and get
labeled partial results instead of an abort when a limit is reached.

**Independent Test**: `--estimate` reports a projected request count and remaining quota without
gathering evidence; `--max-requests 2` on a large window exits 0 with a non-empty commit list and
a `budget-bounded` disclosure carrying a reason and next action.

### Tests for User Story 2 (REQUIRED) ⚠️

- [X] T030 [P] [US2] Write estimation tests in `ghclient/estimate_test.go`: a `per_page=1` probe response carrying a `Link` header with `rel="last"` yields the exact commit count, the arithmetic estimate matches the documented formula, `EstimateActivity` issues exactly one evidence-path request, and an absent `Link` header (single-page result) is handled.
- [X] T031 [P] [US2] Write budget-stop tests in `ghclient/activity_budget_test.go`: a ceiling reached mid-listing returns a populated `ActivityResult` with the commits gathered so far, a `budget-bounded` disclosure, a `CostReport` reflecting actual consumption, and **no** Go error. Add the equivalent for a rate-limit stop producing `quota-exhausted`.
- [X] T032 [US2] Extend `internal/cli/activity_test.go` with `--estimate` output assertions and an exit-code-0 assertion for a budget-bounded run — a bounded result is a result, not a failure. Same file as T018, so not parallel.

### Implementation for User Story 2

- [X] T033 [US2] Implement `EstimateActivity` in `ghclient/activity.go` per research R4: issue one `per_page=1` list-commits probe, read `Response.LastPage` as the exact in-window commit count, and compute `1 + ceil(commits/per_page) + 1 + enrichment_subset`. Count the probe in the reported cost — the accounting must stay honest.
- [X] T034 [US2] Add the pre-flight quota check in `ghclient/activity.go`: query the rate-limit endpoint (which does not count against the core quota) and, when quota is already exhausted, return that fact with the reset time before any evidence gathering begins (FR-016).
- [X] T035 [US2] Wire budget-stop semantics into `CollectActivity` in `ghclient/activity.go`: on `ErrBudgetExceeded` or a rate-limit error, stop gathering and return the populated result plus the matching disclosure rather than propagating the error. Depends on T025, T031.
- [X] T036 [US2] Add `--estimate` and `--max-requests` flags to `internal/cli/activity.go`, and render estimate-only output as the projected count plus quota with an explicit "no evidence was gathered" line.
- [X] T037 [US2] Add `estimate_only` and `max_requests` to `GetRepoActivityInput` in `internal/mcpserver/getrepoactivity.go`, using the `*int` pointer convention so an explicit `0` (uncapped) is distinguishable from an omitted field.
- [X] T038 [US2] Add the cost section to `internal/render/activity.go`: requests consumed against ceiling, quota remaining against limit, and reset time — on every result, including bounded ones (FR-014).

**Checkpoint**: User Stories 1 and 2 both work independently.

---

## Phase 5: User Story 3 — Correlate the change set with the commit narrative (Priority: P3)

**Goal**: Pair changed paths with the commits that plausibly produced them, with every
association labeled observed or inferred so the tool never overstates what it knows.

**Independent Test**: Without `--enrich-commits`, every correlation is `inferred` and none is
`observed`. With `--enrich-commits 5`, at most 5 distinct SHAs carry `observed`.

### Tests for User Story 3 (REQUIRED) ⚠️

- [X] T039 [P] [US3] Write correlation-rule tests in `internal/activity/correlate_test.go` as a table over the three rules from research R8: `observed` only when per-commit file data exists, `inferred:path-mention` on a verbatim path in the message body, `inferred:scope-match` on a Conventional Commit scope matching a leading path segment, and no attribution when no rule matches. Assert first-match-wins ordering and byte-identical output across repeated runs (no map-iteration dependence).
- [X] T040 [P] [US3] Write the attribution-invariant test in `ghclient/enrich_subset_test.go`: `Correlation.Basis == "observed"` requires `Enriched == true` on every SHA it names. This is the highest-value test in the feature — it is what stops inference being presented as observation (FR-017).

### Implementation for User Story 3

- [X] T041 [P] [US3] Implement the three deterministic correlation rules in `internal/activity/correlate.go`, applied in declared order with first match winning, each result tagged with its `Basis` and `Rule`. Paths matching no rule are left unattributed — an honest gap beats a fabricated link.
- [X] T042 [US3] Implement deterministic enrichment-subset selection in `ghclient/activity.go`: take the first N commits in the result's existing order, ask the budget for remaining capacity and dispatch only a batch it can fully afford (**check before dispatch**, not fire-and-fail), and clip by commit order rather than completion order. This is the mitigation for the concurrency-versus-determinism hazard in research R3 — without it, the same query can return different results run to run and FR-023 breaks.
- [X] T043 [US3] Wire correlations into `CollectActivity` in `ghclient/activity.go` and emit an `enrichment-partial` disclosure when the subset delivered is smaller than requested. Depends on T041, T042.
- [X] T044 [US3] Add `--enrich-commits` to `internal/cli/activity.go`.
- [X] T045 [US3] Add `enrich_commits` to `GetRepoActivityInput` in `internal/mcpserver/getrepoactivity.go`, documenting in its jsonschema description that it costs one request per commit and is what enables observed attribution.
- [X] T046 [US3] Render correlations in `internal/render/activity.go`, showing each association's basis so a reader can tell observation from inference at a glance.

**Checkpoint**: All three primary user stories are independently functional.

---

## Phase 6: User Story 4 — Apply the same cost discipline to author-scoped queries (Priority: P4)

**Goal**: The existing "what did I do this week" query gains estimation, a ceiling, partial
results, and consumption reporting — closing the loop on the path where the rate-limit problem
was originally hit.

**Independent Test**: `sting query --author X --window 30d --include-diffs --max-requests 5`
returns partial results with consumption reported and exit code 0, rather than aborting.

### Tests for User Story 4 (REQUIRED) ⚠️

- [X] T047 [P] [US4] Write a regression test in `ghclient/collect_partial_test.go` proving `Collect` returns the commits already gathered when enrichment fails partway, instead of discarding them. This covers a pre-existing Constitution VI violation at `ghclient/ghclient.go` (`return model.Result{}, err` in the `needsDetail` branch), so it is a bugfix regression test as the constitution requires.
- [X] T048 [P] [US4] Write ceiling tests for the author path in `ghclient/budget_query_test.go`: search, repos, and org scopes each stop at the ceiling and return partial results, and the default ceiling of 500 does **not** clip a default-configuration query (`max_commits = 100` with enrichment ≈ 110 requests — research R5).

### Implementation for User Story 4

- [X] T049 [US4] Change the `needsDetail` branch of `Collect` in `ghclient/ghclient.go` to return partial results with an attributable error rather than `model.Result{}`, preserving the existing `Result` shape and reusing `Truncated` to signal clipping. Depends on T047.
- [X] T050 [US4] Add `MaxRequests` to `model.Query` in `model/model.go` and plumb it through `config.Resolve` in `config/resolve.go`. This is additive to `Query`, which is a request type and not part of the serialized evidence contract — `model.SchemaVersion` must **not** be bumped.
- [X] T051 [US4] Add `--max-requests` to `internal/cli/query.go` via `registerQueryFlags`.
- [X] T052 [US4] Add `max_requests` to `GetCommitsInput` in `internal/mcpserver/server.go` using the existing `*int` pointer convention, and plumb it into `config.Request`.
- [X] T053 [US4] Pass the budget option through GitHub client construction in `internal/commitclient/commitclient.go` so every author-scoped path is accounted, not just the activity path.

**Checkpoint**: All four user stories complete. The original complaint is addressed on both the
new and the pre-existing query paths.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T054 [P] Document `sting activity` in `README.md`: add it to the subcommand list near the top, add a usage section after "CLI usage", and state plainly what the digest does not cover (net-comparison blind spots, single reference, GitHub only) — Principle VIII requires stating limits, not just capabilities.
- [X] T055 [P] Add the `max_requests` key with its default and a comment to `config.example.yaml`.
- [X] T056 [P] Update `AGENTS.md` and `.github/copilot-instructions.md` if either enumerates commands or MCP tools, so agent-facing guidance lists both tools.
- [X] T057 Run `go -C tools tool task ci` and confirm lint, staticcheck, govulncheck, race-enabled tests, and the coverage gate all pass. `internal/apibudget` and `internal/activity` are pure logic and must clear the standard 80% gate — neither warrants a lower floor.
- [X] T058 Execute every scenario in [quickstart.md](./quickstart.md) and confirm the Definition of Done checklist, paying particular attention to scenario 4 (boundary off-by-one), scenario 6 (no unearned `observed` attribution), and scenario 11 (determinism).
- [X] T059 Verify the read-only invariant end to end: `sting install list` shows both tools in the auto-approve block, `tools/list` over stdio reports `readOnlyHint: true` for each, and no code path issues a non-GET request to a provider.
- [ ] T060 Request Copilot review on the PR via the API (`POST pulls/{n}/requested_reviewers` with `copilot-pull-request-reviewer[bot]`) — the `gh --add-reviewer @copilot` form does not attach it.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately. T003 feeds `WindowDateBasis` in T004.
- **Foundational (Phase 2)**: Depends on Setup. **Blocks every user story.**
- **User Stories (Phases 3–6)**: All depend on Foundational.
- **Polish (Phase 7)**: Depends on the user stories you intend to ship.

### User Story Dependencies

- **US1 (P1)**: Depends only on Foundational. **No dependency on any other story.**
- **US2 (P2)**: Depends on Foundational. T035 extends `CollectActivity` from T025, so US2's
  implementation lands after US1's — but US2 is separately testable and separately valuable.
- **US3 (P3)**: Depends on Foundational. T042/T043 extend `CollectActivity`; the correlation rules
  themselves (T041) are pure and can be built in parallel with anything.
- **US4 (P4)**: Depends on Foundational and on `apibudget` only. **Independent of US1–US3** — it
  touches the existing query path and could ship first if the author-path rate-limit pain is more
  urgent than the new capability.

### Within Each User Story

- Tests are written first and must fail before implementation.
- Types → budget/transport → resolution → client logic → rendering → surfaces (CLI, MCP).
- `ghclient/activity.go` is touched by T021–T025, T033–T035, T042–T043. Those are **sequential**
  within their phases; only cross-file work is marked `[P]`.

### Parallel Opportunities

- **Phase 1**: T002 and T003 in parallel.
- **Phase 2**: T004+T005 (model), T006+T007 (apibudget), and T013+T014 (registry) are three
  independent tracks. T008–T012 follow T004/T006.
- **Phase 3**: All five test tasks (T015–T019) run in parallel — different files.
- **Phase 5**: T041 (pure correlation rules) is independent of everything else in the phase.
- **Phase 6**: Entirely independent of Phases 3–5; a second developer can take US4 in parallel
  with US1 once Foundational is done.
- **Phase 7**: T054, T055, T056 in parallel.

---

## Parallel Example: User Story 1 Tests

```bash
# Launch all five US1 test tasks together — each touches a different file:
Task: "Provider-behavior fixtures in ghclient/activity_test.go"
Task: "Request-count assertions in ghclient/activity_cost_test.go"
Task: "Rendering tests in internal/render/activity_test.go"
Task: "CLI flag tests in internal/cli/activity_test.go"
Task: "MCP tool tests in internal/mcpserver/getrepoactivity_test.go"
```

## Parallel Example: Foundational Tracks

```bash
# Three independent tracks after Phase 1:
Track A: T004 -> T005    # model/activity.go + tests
Track B: T006 -> T007    # internal/apibudget + tests
Track C: T013 -> T014    # mcpserver tool registry + drift test
# Then converge: T008 -> T009 -> T010 (config), T011 -> T012 (ghclient options)
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1: Setup (T001–T003)
2. Phase 2: Foundational (T004–T014) — **blocks everything**
3. Phase 3: User Story 1 (T015–T029)
4. **STOP and VALIDATE**: run quickstart scenarios 1–4. Scenario 2 (request count) and scenario 4
   (boundary off-by-one) are the two that prove the MVP actually delivers what it claims.
5. Ship it — a repository activity query that is cheap and honest is independently valuable even
   with no budgeting, correlation, or author-path retrofit.

### Incremental Delivery

1. Setup + Foundational → foundation ready
2. + US1 → **MVP**: repository activity, cost-bounded by construction
3. + US2 → cost is visible and capped; partial results replace aborts
4. + US3 → attribution, honestly labeled
5. + US4 → the original author-path complaint is closed

### Parallel Team Strategy

With two developers, after Foundational:

- Developer A: US1 → US2 → US3 (the new capability, in dependency order)
- Developer B: US4 (the existing-path retrofit — genuinely independent)

US4 touches `ghclient/ghclient.go`, `config/resolve.go`, `internal/cli/query.go`, and
`internal/mcpserver/server.go`; US1–US3 touch `ghclient/activity.go`, `internal/cli/activity.go`,
and `internal/mcpserver/getrepoactivity.go`. The only shared file is `internal/mcpserver/server.go`
(T013 registry, T052 input) — sequence those two or expect a small merge.

---

## Notes

- `[P]` means different files and no dependency on incomplete work.
- Go tests are co-located; there is no `tests/` directory.
- Commit after each task or logical group, signed and DCO signed off (`git commit -S -s`),
  Conventional Commits, PR-only — never to `main`.
- Every task that adds behavior adds tests in the **same** change; the coverage gate is not a
  separate cleanup step.
- The three highest-risk tasks, worth extra review attention: **T020** (parents capture — if it
  regresses, the base is wrong and every window under-reports), **T042** (check-before-dispatch —
  if it regresses, results stop being deterministic), and **T024** (unconditional disclosures — if
  they become conditional, the tool starts overstating its evidence).
