# Quickstart: Validating Repository Activity Digest

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Contracts**:
[contracts/](./contracts/)

How to prove this feature works end to end. Each scenario names the requirement it validates and
the success criterion it feeds. Written to be run after `/speckit-implement`.

## Prerequisites

```sh
go version                       # expect >= 1.26.5, matching go.mod
go -C tools tool task build      # produces ./sting
sting auth github                # or export STING_TOKEN=<read-only PAT>
```

A GitHub token is required for the live scenarios. The offline suite needs nothing.

## 1. Offline gates (run first — these must pass before any live check)

```sh
go -C tools tool task ci
```

Runs lint, staticcheck, govulncheck, race-enabled tests, and the coverage gate. Expected: all
pass, with `model`, `config`, `ghclient`, `internal/apibudget`, and `internal/activity` each at or
above 80%.

Targeted runs while iterating:

```sh
go test ./model/... ./config/... ./ghclient/... ./internal/apibudget/... ./internal/activity/...
go test -run TestCollectActivity ./ghclient/ -v
```

## 2. The cost promise (SC-001, SC-002, FR-006, FR-007)

**This is the feature's central claim and the most important test.** It is offline — a fake
`RoundTripper` counts requests (research R10).

```sh
go test -run TestActivityRequestCount ./ghclient/ -v
```

Expected: a 250-commit window issues **≤ 15** requests — one probe, three listing pages, one
comparison, plus overhead. The same window through the existing per-commit path issues ~251. Any
regression that reintroduces per-commit fetching fails this test rather than quietly costing
users their quota.

## 3. Repository activity, live (User Story 1, FR-001 – FR-005)

```sh
sting activity --repo skaphos/sting --window 7d
```

Expected: commits with full messages, an aggregate change set, and a footer reporting cost and
disclosures. Verify the header echoes repository, reference, normalized window, and both boundary
SHAs (FR-020).

Confirm no author was needed — the gap that motivated the feature:

```sh
sting query --window 7d          # fails: "author is required"
sting activity --repo skaphos/sting --window 7d   # succeeds
```

## 4. Boundary correctness (FR-008, FR-008a — the off-by-one)

The highest-risk correctness check. Pick a window whose earliest commit is known:

```sh
sting activity --repo skaphos/sting --since 2026-07-24 --until 2026-07-25 --format json \
  | jq '{base: .boundaries.base_sha, head: .boundaries.head_sha, source: .boundaries.base_source}'
```

Expected: `base_sha` is the **parent** of the earliest in-window commit, not that commit itself.
Verify the first in-window commit's own changes appear in the change set:

```sh
git log --format=%H -1 --before=2026-07-24   # should equal base_sha
```

If `base_sha` equals the earliest in-window commit, the off-by-one described in R1 has regressed
and every window is under-reported.

## 5. Cost visibility and budgeting (User Story 2, FR-011 – FR-016)

```sh
sting activity --repo skaphos/sting --window 30d --estimate
```

Expected: a projected request count and remaining quota; no evidence gathered. Compare against a
real run to confirm SC-005 (±20%):

```sh
sting activity --repo skaphos/sting --window 30d --format json | jq '.cost'
```

Force a budget stop and confirm partial results, not an error:

```sh
sting activity --repo skaphos/sting --window 90d --max-requests 2 --format json \
  | jq '{count, consumed: .cost.consumed, disclosures: [.disclosures[].kind]}'
echo "exit=$?"
```

Expected: `exit=0`, a non-empty commit list, and a `budget-bounded` disclosure carrying a reason
and a next action. An error or an empty result here is a Constitution VI violation.

## 6. Honest attribution (User Story 3, FR-017)

```sh
sting activity --repo skaphos/sting --window 7d --format json \
  | jq '[.correlations[] | .basis] | group_by(.) | map({basis: .[0], n: length})'
```

Expected without `--enrich-commits`: **every** correlation is `inferred`. Zero `observed` — no
per-commit data was fetched, so none can be claimed. Then:

```sh
sting activity --repo skaphos/sting --window 7d --enrich-commits 5 --format json \
  | jq '[.correlations[] | select(.basis=="observed") | .shas[]] | unique | length'
```

Expected: at most 5 distinct SHAs carry `observed`. More than that means the invariant in
[contracts/go-public-api.md](./contracts/go-public-api.md) has broken and the tool is presenting
inference as observation.

## 7. Blind-spot disclosure (FR-018)

```sh
sting activity --repo skaphos/sting --window 7d --format json \
  | jq '[.disclosures[].kind] | contains(["reference-scoped","net-comparison-blindspot"])'
```

Expected: `true` on **every** run that produces a change set, including clean ones. These two are
unconditional.

## 8. Divergence and empty windows (FR-009, edge cases)

```sh
# A window with no commits
sting activity --repo skaphos/sting --since 2020-01-01 --until 2020-01-02 --format json \
  | jq '{count, status: .boundaries.status, paths: (.change_set.paths | length)}'
```

Expected: `count: 0`, `status: "identical"`, zero paths, exit `0` — an empty report, not an error.

Divergence (force-push inside the window) is covered offline by an `httptest` fixture returning
`"status": "diverged"`; expect an empty change set plus an `ancestry-diverged` disclosure.

## 9. GitLab rejection (FR-025)

```sh
sting activity --repo group/project --provider gitlab 2>&1
```

Expected: `provider "gitlab" does not support repository activity (github only)`. Confirm existing
GitLab commit queries still work:

```sh
sting query --provider gitlab --author someone --scope repos --repos group/project --window 7d
```

## 10. MCP surface (FR-026, FR-026a)

```sh
sting install list                       # get_repo_activity appears in the auto-approve block
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | sting mcp
```

Expected: both `get_commits` and `get_repo_activity` listed, each with `readOnlyHint: true`. The
installer snippet must contain both without anyone having hand-edited a list (research R6).

## 11. Determinism (FR-023, SC-009)

```sh
sting activity --repo skaphos/sting --since 2026-07-01 --until 2026-07-08 --format json \
  | jq 'del(.generated_at)' > /tmp/run1.json
sting activity --repo skaphos/sting --since 2026-07-01 --until 2026-07-08 --format json \
  | jq 'del(.generated_at)' > /tmp/run2.json
diff /tmp/run1.json /tmp/run2.json && echo "deterministic"
```

Expected: no diff. A difference points at unsorted paths or completion-order-dependent enrichment
— the concurrency hazard described in research R3.

## Definition of done

- [ ] `go -C tools tool task ci` passes, coverage gate included
- [ ] Scenario 2 proves the request count bound (SC-001, SC-002)
- [ ] Scenario 4 proves boundary correctness (FR-008)
- [ ] Scenario 5 proves partial results on a budget stop, exit `0` (FR-013, Constitution VI)
- [ ] Scenario 6 proves no unearned `observed` attribution (FR-017)
- [ ] Scenario 11 proves determinism (FR-023)
- [ ] `README.md` and `config.example.yaml` document the command, tool, and `max_requests`
- [ ] ADR 0010 recorded for the multi-tool MCP server
- [ ] Research R2's committer-date assumption verified against the live API
