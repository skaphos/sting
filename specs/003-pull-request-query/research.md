# Research: PR Activity and Personal Inbox

Date: 2026-09-05. Baseline: `4c6013a58aa504e1345f3350ba5e18773c020769`.
The tracked graph report targets `26cf951a` and was treated as stale. Findings combine local
source inspection with independent lifecycle and inbox research required by `speckit-plan`.
No live authenticated provider queries were used. Documentation and the pinned client establish
the design; controlled integration fixtures must validate normalization and transport behavior.

## R1 — Reuse the existing core

**Decision:** Extend `model`, `config`, and `ghclient`; add thin CLI/MCP/rendering adapters.
Reuse go-github/v91 and `internal/apibudget`. Add no dependency. Keep commit/repository-activity
contracts untouched and use two independently versioned PR result envelopes.

**Rationale:** [ADR 0004](../../docs/adr/0004-public-packages-and-wake-evidence.md) and
[ADR 0010](../../docs/adr/0010-multi-tool-mcp-server.md) already decide package composition and
sibling read-only tools. The sibling Skaphos `tools/ECOSYSTEM.md` has no PR-query adoption
verdict. Adopt the existing provider SDK and transport; sting supplies its own evidence contract.

**Alternatives:** Shelling out to `gh` would introduce ambient credential and process coupling.
GraphQL is unnecessary and its usual POST transport conflicts with ADR 0010's GET-only rule.
A generic provider-query framework or new public provider package adds no needed capability.

**Local evidence:** `go.mod`; `ghclient/{ghclient,options,activity}.go`;
`internal/apibudget/budget.go`; `internal/commitclient/commitclient.go`.

## R2 — Discover historical candidates conservatively

**Decision:** For `any` and `closed`, search PRs created no later than the window end, then
read lifecycle history. Do not bound *discovery* by latest update/closure or require candidates
to be currently closed. Repos/org use PR listing with `state=all`; apply exact filters locally.
Specific `created`, `updated`, and `merged` bases can use their matching discovery qualifiers.

**Revised during implementation:** history *reads* are bounded below by the window start. A
candidate whose `updated_at` precedes `since` is dropped without a lifecycle request. Discovery
is unchanged, so the candidate set is still the conservative superset above.

**Rationale:** Current-record date filters cannot establish an earlier closure erased by later
reopening, so discovery must stay wide. Reading history for every candidate, however, made the
default `any` query unusable: on a 600-PR repository it spent 495 of 500 requests on records
last touched 18 months earlier, exhausted the budget before reaching the one PR merged inside
the window, and returned zero matches. Opening, merging, closing, and reopening each advance
`updated_at` — the behaviour GitHub's own incremental polling relies on, via the documented
`since` parameter on issue listing and the `updated:` search qualifier — so a record updated
before `since` holds no qualifying occurrence.

**Limits:** GitHub documents `updated_at` for change detection but publishes no per-event
guarantee, so this remains an inference rather than a contract. It is therefore disclosed
rather than assumed: a run that skipped records emits a `history-bounded` disclosure naming the
count and the invariant. Coverage stays complete, because the bound is a selection rule applied
to every candidate alike and not an interrupted read; a consumer that rejects the inference can
see exactly how many records it affected. If the invariant were ever violated, the failure mode
is a missed occurrence on a record with a stale timestamp — which the disclosure makes visible.

**Alternatives:** `updated:since..until` as a *discovery* bound still loses later-changed PRs
and is still rejected; `closed:window` loses earlier closures. Reading every candidate's
history was the original choice and is rejected above on measured cost. Disabling the bound
behind a flag was considered and rejected as MVP surface area. No automatic time-splitting of
capped search is proposed.

Source: [GitHub PR search qualifiers](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests).

## R3 — Recover lifecycle occurrences, preserving ambiguity

**Decision:** Use paginated per-PR issue events for `any`/`closed`, and when a selected merge
cannot be confirmed from discovery. Preserve provider event IDs and original opening identity.
Fully read eligible histories; the endpoint supplies no documented time-order shortcut.

**Rationale:** Per-PR events expose the necessary lifecycle evidence and accept either Issues
read or Pull requests read permissions. Repository-wide issue events require Issues read and
would widen permission assumptions. Timeline adds irrelevant bodies.

Source: [Issue event endpoints](https://docs.github.com/en/rest/issues/events#list-issue-events).

**Classification decision:** Keep distinct closure IDs. A merge occurrence is not a second
unmerged closure. Establish earlier closure cycles using confirmed reopening/merge chronology;
do not treat event ID order or a nonempty `commit_id` as proof of lifecycle order or merging.
Where merge-associated closure pairing or same-time events cannot be classified safely, retain
confirmed actions and disclose an ambiguous-history gap. This conservative policy is our design;
GitHub does not document a universal merge/closure pairing key.

Source: [Lifecycle event types](https://docs.github.com/en/rest/using-the-rest-api/issue-event-types).

## R4 — Verify inbox relationships through paginated discovery

**Decision:** Repos/org list open PRs and evaluate author, assignees, and direct requested
reviewers locally. Search uses three streams: `author`, `assignee`, and `review-requested`.
For candidate repositories, paginate open PRs once per repository to establish all current
relationships. Search-confirmed author/assignee evidence can survive an interrupted verification
pass; review-only candidates require direct-review proof before being returned.

**Rationale:** The pinned SDK's issue-search shape lacks requested-reviewer arrays and refs;
its PR-list shape includes them. Ordinary `review-requested:LOGIN` can include team requests.
Direct requests must therefore be verified from `requested_reviewers`, without per-PR detail
calls. Requests to teams alone are metadata and excluded from this MVP's personal relationship.

**Alternatives:** `involves` includes unrelated interactions. `user-review-requested:@me` is
explicitly documented, but arbitrary-login use was not verified; the plan does not depend on it.
Using separate per-PR reviewer endpoints would violate the agreed inbox cost shape.

Sources: [Reviewer search semantics](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests#search-by-pull-request-review-status-and-reviewer),
[PR listing](https://docs.github.com/en/rest/pulls/pulls#list-pull-requests).
Local SDK: `github/{issues,pulls,search}.go` in go-github/v91 v91.0.0.

## R5 — Visibility and completeness are dimensions

**Decision:** Unrestricted search adds `is:public` as sting policy. Explicit org/repo search
can see private resources allowed by the credential. Disclose search indexing and its limits
separately from consumed requests, returned PR caps, unfinished histories, and missing metadata.

**Rationale:** Search limits apply to candidates, not qualifying lifecycle occurrences: a short
window can encounter the 1,000-hit ceiling while returning few PRs. Search also limits matching
repositories to 4,000 and can report incomplete results. Multi-target access omissions cannot
be mistaken for known repository skips. Recommend repos/org listing for more complete coverage.

**Alternatives:** Calling every empty result complete, treating unknown metadata as empty, or
claiming a global newest-N ordering would overstate the evidence.

Source: [GitHub search limits](https://docs.github.com/en/rest/search/search#search-issues-and-pull-requests).

## R6 — Resolve identity with sting credentials

**Decision:** An explicit validated inbox user requires no identity lookup. Otherwise use
`Users.Get(ctx, "")` / `GET /user` under the same request ceiling, recording the resolved login.
Do not infer identity from local Git or another tool's credentials. Retain the existing dedicated
PAT → sting credential-store → anonymous credential precedence; anonymous self lookup fails.

**Rationale:** The authenticated-user endpoint supplies the login without fetching private
profile extras. The existing credential resolver already owns host-specific sting tokens.

Source: [Authenticated user](https://docs.github.com/en/rest/users/users#get-the-authenticated-user).
Local evidence: `internal/commitclient/commitclient.go`; pinned `github/users.go`.

## R7 — Isolate budgets and preserve failed reports

**Decision:** Give each PR collection call a fresh budget/session, including public library
calls. Process discovery/history serially for predictable consumption. Track identity,
discovery (including inbox verification), and history request counts. Avoid the existing
unmetered rate-limit preflight; FR-012 counts every query request. Do not reuse activity's
`classifyStop` unchanged: it turns provider quota errors into success, whereas only sting's
own caps are successful PR stops. Preserve initialized reports on failures even at zero PRs.

**Rationale:** Existing `apibudget.Transport` counts dispatched requests and retries. Existing
CLI/MCP activity adapters preserve structured failure reports based on schema initialization.
New collectors must avoid shared counters across concurrent MCP calls and must not silently
ignore `q.MaxRequests` when library callers omit `WithRequestBudget`.

**Alternatives:** Per-function counters miss redirects/retries; parallel history fetches make
budget-winner selection unstable; a blanket error discards collected evidence.

## Resolution

All Phase 0 questions are resolved into concrete designs and explicit evidence limits.
Provider ordering, live-state races, and ambiguous lifecycle payloads are handled as reportable
limitations, not assumptions of completeness. No constitution deviation is required.
