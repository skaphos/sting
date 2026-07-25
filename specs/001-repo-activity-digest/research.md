# Phase 0 Research: Repository Activity Digest

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-07-25

All Technical Context unknowns are resolved. Ten decisions follow. Each records what was chosen,
why, and what was rejected. Claims about the codebase were verified by reading it; claims about
the dependency were verified against `go-github v82` in the module cache. One item (R2) depends on
provider behavior that cannot be verified offline and carries an explicit verification task.

---

## R1 — The comparison base costs zero extra requests

**Decision**: Resolve the window's base commit from `RepositoryCommit.Parents[0].SHA` on the
earliest in-window commit, captured during the ordinary list-commits pagination.

**Rationale**: `go-github`'s `RepositoryCommit` (`github/repos_commits.go:19`) declares
`Parents []*Commit`, and GitHub populates it on the **list** endpoint, not only on commit detail.
sting's `fromRepoCommit` (`ghclient/ghclient.go:731`) currently maps SHA, URL, author, repo,
message, and date — and discards `Parents`. Capturing the first parent of the oldest in-window
commit therefore satisfies FR-008 for free, which is what makes SC-001 (≤15 requests) achievable.

First-parent is the correct choice when the earliest in-window commit is a merge: the first parent
follows the mainline of the compared reference, so the base is the reference's state before the
window rather than the tip of a merged side branch.

**Alternatives considered**:

- *A separate `GetCommit` call on the earliest commit* — one extra request per query for data
  already in hand.
- *A second list-commits call with `until` set to the window start, taking the newest result* —
  one extra request, and it reintroduces the timestamp-versus-ancestry problem FR-008a exists to
  avoid.
- *`merge_base_commit` from the comparison response* — only available after the comparison, which
  needs the base to be issued in the first place. Circular.

---

## R2 — Window filtering is committer-date-based upstream; sting reports author date

**Decision**: Apply the window server-side via list-commits `since`/`until` as today, but record
**both** dates on each commit record and state in the result which date the window was applied to.
Boundary resolution uses the provider's own ordering, so the base is always the parent of whatever
the provider considers the oldest in-window commit.

**Rationale**: `model.Query` documents `Since`/`Until` as bounding the **author** date, and
`fromRepoCommit` sets `Commit.Date` from `commit.author.date`. GitHub's list-commits `since`/
`until` parameters filter on the **committer** date. For ordinary commits the two are identical,
so this has been invisible; they diverge after a rebase, cherry-pick, or amend. For the activity
digest this is load-bearing: "the earliest in-window commit" must mean the same thing to sting and
to the provider, or the base commit is wrong and the entire change set shifts.

Recording both dates and disclosing which one bounded the window is the Principle VIII response —
state the provider's semantics rather than paper over them.

**Verification task (carry into implementation)**: confirm against the live API that list-commits
`since`/`until` filter on committer date. `httptest` fixtures cannot verify provider semantics —
they only prove sting handles a given response shape. If the behavior turns out to be author-date
based, the only change needed is the label in the disclosure, not the design.

**Alternatives considered**:

- *Client-side re-filtering on author date* — would require fetching beyond the window to find
  commits whose author date falls inside it, defeating the cost goal, and could still miss
  commits arbitrarily far outside.
- *Silently keeping the existing mismatch* — cheapest, but it makes the boundary undefined in
  exactly the rebase-heavy repositories where the digest matters most.

---

## R3 — Request accounting and ceiling enforcement live in an `http.RoundTripper`

**Decision**: Introduce `internal/apibudget` providing a counting, enforcing `RoundTripper` that
wraps the client's existing transport. It counts requests, captures the rate-limit headers from
every response, and returns a sentinel `ErrBudgetExceeded` once the ceiling is reached.

**Rationale**: `ghclient.New` already constructs `github.NewClient(&http.Client{Timeout:
httpTimeout})` (`ghclient/ghclient.go:131`), so there is a single, clean injection point. Putting
accounting in the transport gives three things at once:

- **Exact counts** including pagination and any retry, which call-site counting would miss.
- **Coverage of the existing paths for free.** `searchByAuthor`, `listRepos`, `listOrg`,
  `pullRequestCommits`, and `enrichDetails` all go through the same transport, so User Story 4 —
  cost discipline on author-scoped queries — needs no changes in any of them.
- **Rate headers in one place.** `go-github`'s `Response.Rate` is per-response; the transport
  accumulates the latest observation for the `CostReport` without threading it through call
  chains.

**Determinism risk and its mitigation (this is the one real subtlety).** `enrichDetails` fans out
across `defaultConcurrency = 8` goroutines. If the ceiling were enforced purely inside the
transport, *which* eight requests won the race would decide which commits got enriched — the same
query against unchanged upstream state could return different results, violating FR-023 and
Constitution III. Two rules close this:

1. **Check before dispatch.** The enrichment loop asks the budget for remaining capacity and
   dispatches only a batch it can fully afford, rather than firing requests and letting them fail.
2. **Clip by commit order, not completion order.** When capacity is short, enrich the first *N*
   commits in the result's existing deterministic order and mark the remainder unenriched in a
   `Disclosure`.

The transport-level check remains as a backstop for any path that forgets rule 1, but it should
never be what stops a well-behaved query.

**Alternatives considered**:

- *Counting at each call site* — invasive, easy to miss a path, and blind to retries.
- *A `context.Context` value carrying a counter* — implicit dependency, and go-github does not
  expose a hook to decrement it.
- *A rate limiter rather than a counter* — solves throughput, not the total-consumption problem
  the user actually described.

---

## R4 — An exact cost estimate costs exactly one request

**Decision**: Estimate by issuing a single `per_page=1` list-commits request scoped to the window
and reading `Response.LastPage`, which equals the exact number of commits in the window. Derive
the estimate arithmetically from that count.

**Rationale**: `go-github`'s `Response` exposes `LastPage` parsed from the `Link` header
(`github/github.go:673,778`). With `per_page=1`, the last page number *is* the commit count. The
estimate is then:

```text
estimate = 1 (probe)
         + ceil(commits / per_page)   (commit listing)
         + 1                          (boundary comparison)
         + enrichment_subset_size     (0 unless per-commit detail is requested)
```

Every term is exact, so SC-005's ±20% tolerance has enormous headroom — the only source of drift
is upstream state changing between the estimate and the run.

FR-011 requires an estimate "without issuing the query's evidence-gathering requests". A
`per_page=1` probe gathers no evidence — it is discarded and only its `Link` header is read — so
this satisfies the requirement as written. The probe is counted in the reported cost so the
accounting stays honest.

Separately, GitHub's `/rate_limit` endpoint does not count against the core quota, which gives
FR-016 (report a pre-exhausted quota before starting) a genuinely free pre-flight check.

**Alternatives considered**:

- *A heuristic from window length times an assumed commit rate* — zero requests, but wildly
  inaccurate for exactly the heavy user this feature targets, and would fail SC-005.
- *Estimating from a full first page* — same request count as the probe but transfers 100 commits
  instead of 1 for no additional information.

---

## R5 — The default ceiling is 500 requests

**Decision**: `DefaultMaxRequests = 500`, a fixed constant, overridable via the `max_requests`
config key, the `--max-requests` flag, and the MCP tool input. `0` disables the ceiling
explicitly.

**Rationale**: The value is derived from what existing default configuration actually costs, so
that FR-012's default cannot clip a query that works today:

| Query shape | Approximate requests | Headroom at 500 |
|---|---|---|
| Repository activity, one week, ≤500 commits | ~8 | 60× |
| Author query, default config (`max_commits = 100`), enrichment on | ~110 | 4.5× |
| Author query, org scope over a mid-sized org | ~150–250 | ~2× |
| Exhaustive scan (`max_commits = 0`) with diffs over a large org | thousands | **stopped** |

`config.DefaultMaxCommits = 100` (`config/config.go:19`) is what makes this tractable: the
enrichment path is already capped at 100 commits by default, so 500 leaves roughly 4× headroom on
the most expensive default-configuration query while still stopping the unbounded scan that
motivated the feature.

At GitHub's authenticated 5000/hour core quota, 500 is 10% — so at minimum 10 worst-case queries
per hour, and roughly 600 activity queries per hour, far exceeding SC-003's ≥20.

Per FR-012a the value is a constant, never derived from remaining quota. A quota-derived ceiling
would make the same request return different results depending on unrelated prior activity.

**Alternatives considered**:

- *100* — clean, but would clip the default author query with enrichment, breaking existing
  behavior for no safety gain.
- *A per-query-shape default* — better fitted, but a ceiling whose value depends on the request
  shape is harder to explain and to document than a single number.

---

## R6 — `ReadOnlyTools()` becomes derived, not hand-maintained

**Decision**: Refactor `internal/mcpserver` so tool definitions live in one slice that both
`New()` (registration) and `ReadOnlyTools()` (auto-approve list) read from.

**Rationale**: `ReadOnlyTools()` currently returns a hardcoded `[]string{"get_commits"}`
(`internal/mcpserver/server.go:76`) while its own doc comment calls it "the single source of truth
… so the install permissions snippet cannot drift". With one tool that claim held by luck. Adding
a second makes it two hand-synchronized lists, and the failure mode is silent: a tool registered
but absent from the list simply stops being auto-approved, which looks like a runtime quirk rather
than a bug. `internal/cli/install.go:187` is the sole consumer.

Deriving both from one slice makes Constitution Principle I mechanical rather than conventional,
which is precisely what the principle demands.

**Alternatives considered**:

- *Appending `"get_repo_activity"` to the literal* — two lines, and it preserves a latent drift
  bug the constitution explicitly says must be mechanical.
- *Reflecting over the registered server* — the go-sdk offers no stable enumeration API, and it
  would make the auto-approve list a runtime rather than compile-time fact.

---

## R7 — The change set comes from one comparison call, with commits paginated away

**Decision**: Call `Repositories.CompareCommits(ctx, owner, repo, base, head, &ListOptions{PerPage:
1})` and read only `Files`. Ignore the returned `Commits` — the commit list already came from the
cheaper listing path.

**Rationale**: `CompareCommits` (`github/repos_commits.go:240`) returns a `CommitsComparison`
carrying `Status`, `AheadBy`, `BehindBy`, `TotalCommits`, `Commits`, and `Files`. The `ListOptions`
parameter paginates the **commits**, not the files, so `PerPage: 1` minimizes payload while leaving
the file list intact — one request, independent of commit count, satisfying FR-007.

`Status` directly serves FR-009: a value of `diverged` means the boundaries do not share ancestry,
which happens after a force-push or rebase inside the window. That case must be reported as a
divergence disclosure rather than rendered as a change set.

GitHub caps the comparison response at 250 commits and 300 files. The commit cap is irrelevant
here (commits come from the listing path), but the file cap is not: a window touching more than
300 files yields a truncated `Files` array, which must raise a truncation disclosure per FR-019.

**Alternatives considered**:

- *`CompareCommitsRaw` with the diff media type* — returns a unified diff that would need parsing
  to recover per-path counts, and the endpoint's own caps still apply.
- *Summing per-commit file lists* — exactly the per-commit cost the feature exists to eliminate.

---

## R8 — Correlation uses three deterministic, declared rules

**Decision**: Attribute changed paths to commits with an ordered, deterministic rule set, tagging
every result with the rule that produced it:

| Basis | Rule | Confidence |
|---|---|---|
| `observed` | Per-commit file data was actually fetched for this commit and lists this path | Certain |
| `inferred:path-mention` | The commit message body contains the path verbatim | Strong |
| `inferred:scope-match` | A Conventional Commit scope, `feat(render):`, matches a leading path segment | Weak |
| *(none)* | No rule matched — the path appears in the change set with no commit attributed | — |

**Rationale**: FR-017 forbids presenting inference as observation, and FR-023 requires
determinism, which rules out any scoring that depends on iteration order or map traversal. Rules
are applied in the order above, first match wins, and every correlation records its basis so a
consumer can filter to `observed` only. Paths that match no rule are left unattributed rather than
guessed — an honest gap is worth more than a fabricated link, which is the whole point of
Principle VIII.

The conventional-commit rule is deliberately marked weak: the repository uses Conventional Commits
for release-please (ADR 0005), so scopes are reliably present, but a scope names a component, not
a path, and the two only usually coincide.

**Alternatives considered**:

- *Similarity scoring between message text and path tokens* — better recall, but tuning thresholds
  makes the output hard to audit and risks presenting noise as evidence.
- *No correlation at all, emitting the two lists side by side* — maximally honest and much
  simpler, but it pushes the entire join onto the consumer and leaves User Story 3 unbuilt.

---

## R9 — GitLab is rejected at request resolution, not at the client

**Decision**: `config.ResolveActivity` rejects any provider other than GitHub with a message naming
the capability and the provider. No GitLab activity code path is written.

**Rationale**: `config.Resolve` already validates provider/scope compatibility and returns
messages in the form `provider %q does not support scope %q (use repos or org)`
(`config/resolve.go:81`). Rejecting at the same boundary keeps validation in one place, means the
failure is impossible to reach with a half-built client behind it, and gives an identical error
whether the request arrived from the CLI or from MCP.

Existing GitLab commit queries are untouched — `gitlabclient` gains nothing and loses nothing.

**Alternatives considered**:

- *A GitLab client returning a not-implemented error* — a stub whose only behavior is to fail,
  reachable only after client construction has already resolved credentials.
- *Silently returning an empty result* — forbidden by Principle VI; an empty result is
  indistinguishable from a quiet repository.

---

## R10 — Testing strategy: fixtures for shape, a fake transport for cost

**Decision**: Two complementary layers, both offline.

1. **Provider behavior** via `net/http/httptest`, following the existing pattern in
   `ghclient/collect_test.go` and `enrich_test.go`. Fixtures must cover: parents present on list
   responses, a `diverged` comparison status, a comparison truncated at the file cap, an empty
   window, a root-commit window, and a 403 that is a rate limit versus a 403 that is a permission
   denial (`isRateLimited` already distinguishes these).
2. **Cost accounting** via a fake `RoundTripper` asserting exact request counts. This is what
   turns SC-001 and SC-002 into executable tests rather than aspirations — a test can assert "this
   query issued exactly 8 requests" and fail if a regression reintroduces per-commit fetching.

**Rationale**: The constitution forbids network access in tests, and `httptest` alone cannot prove
a *negative* like "no per-commit detail request was issued". Counting at the transport can, and it
directly encodes the feature's central promise.

Coverage: `internal/apibudget` and `internal/activity` are pure logic with no interactive surface,
so both must clear the standard 80% gate; neither warrants a documented lower floor.

**Alternatives considered**:

- *Recorded live cassettes* — higher fidelity to real provider behavior, but they drift silently
  and would need regeneration against a live account, which the no-network rule forbids in CI.
- *Asserting request counts only through httptest handler hit counts* — workable, but it cannot
  observe requests that fail before reaching the handler, and it conflates transport retries with
  distinct logical calls.

---

## Unresolved

None. R2 carries a live-API verification task, but its outcome changes only a disclosure label,
not the design — so it does not block Phase 1 or implementation.
