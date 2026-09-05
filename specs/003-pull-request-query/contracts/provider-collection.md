# Contract: GitHub PR Collection

The algorithms below are design choices derived in [research.md](../research.md), including
conservative behavior where the provider does not guarantee an ordering or historical shortcut.
Use the pinned go-github REST client. Every request is GET and uses the configured API host.

## Allowed collection operations

| SDK operation | Purpose | Cost category |
| --- | --- | --- |
| `Users.Get(ctx, "")` | Resolve inbox self identity when user omitted | identity |
| `Repositories.ListByOrg` | Enumerate credential-visible org repositories | discovery |
| `Search.Issues` with `is:pr` | PR candidate or relationship streams | discovery |
| `PullRequests.List` | Repos/org PR discovery and search-inbox relationship verification | discovery |
| `Issues.ListIssueEvents` | Recover historical lifecycle occurrences | history |

No commit-list augmentation, per-PR detail/review endpoint, timeline bodies, diffs, check runs,
GraphQL, or unmetered quota request. Requests use typed/validated targets, not provider-supplied
arbitrary URLs; follow page numbers through the SDK while retaining the configured host.

## Budget/session and ordering

Each call owns a fresh transport budget, even on a reused public `ghclient.Client`. Preserve
base URL, authentication and timeout while creating the per-call session; do not wrap a previous
session's exhausted counter or mutate a shared SDK client. Reuse `apibudget.Transport` and keep
phase counters derived from actual transport consumption before/after serial operations.
Public query `MaxRequests` is authoritative; omission of a constructor budget option cannot
silently remove it. Test successive and concurrent calls with isolated caps.

Use existing per-request 30-second timeout, caller cancellation, and CLI two-minute query
context. Do not retry automatically outside the caller context. Preserve the existing 10,000-page
loop guard, with a `pagination-limit` disclosure; `max_requests=0` disables only that cap.

Explicit repos are normalized and lexically sorted. Enumerate org repository pages using
`sort=full_name&direction=asc`, then process names in sorted page order, deduplicating names.
Incomplete enumeration remains a scope-level gap. Candidate PR pages use `sort=created`,
`direction=desc`; normalize equal-time candidates by repo/number within each page. Do not
claim stable selection if the provider changes ordering/state mid-query; disclose detected races.

## Activity discovery

For search, always include `is:pr author:<login>` and normalized target restrictions. Add
`is:public` only when neither org nor repos restrict the search. Split long repository target
lists into deterministic bounded query chunks; org plus repos is an intersection. Use conservative
calendar-day widening for provider date qualifiers, then apply exact inclusive UTC bounds locally.
Never truncate a query string or silently drop targets to satisfy provider query limits.

| Time basis | Search candidate qualifier | Evidence evaluation |
| --- | --- | --- |
| `created` | `created:<widened window>` | Original creation timestamp |
| `updated` | `updated:<widened window>` | Latest update timestamp only |
| `merged` | `merged:<widened window>` | Actual merge timestamp; event reads if unavailable |
| `closed`, `any` | `created:<=<widened until>` | Original creation plus fully paginated lifecycle histories as selected |

Apply current state/draft filters locally; do not filter `closed`/`any` discovery by current
closed state. For repos/org, list `state=all`, discard creation after `until`, and evaluate
selected current-state/author/draft filters before applicable history work. Filtering is
three-valued: reject known mismatches, accept known matches, and defer unknown merge disposition
to permitted lifecycle reads before applying an explicit state filter. This applies even to
`created`/`updated` queries with `state=closed|merged`. Unknown state must not discard a candidate
before the history that can resolve it. If resolution cannot complete, disclose the gap and do
not return that candidate under an unproven filter. Confirmed openings may be retained while
waiting for history only when all explicit selection filters are already satisfied.

Process a discovery page's candidates in order, completing one candidate's required history
before moving on. Process all fetched page metadata first so already-confirmed openings/merges
are retained if history then consumes the budget. Read only histories necessary for selected
matches; `any`/`closed` generally requires each eligible candidate's full event pagination.
No early history stop based on timestamp, ID, or assumed provider event order.

Each search stream/chunk tracks total hits, incomplete flag and remaining pages. Never request
beyond its 1,000-candidate bound. Record the cap even with zero returned matching PRs. State
search's 4,000-repository coverage bound and inaccessible-target/indexing limitations. Repos/org
listing avoids the search cap but still has credential, request, and pagination boundaries.

## Lifecycle normalization

- Original `created_at` proves one opening. Actual `merged_at` or a `merged` event proves a
  single merge; prefer the event ID if both are obtained.
- If record and event merge timestamps disagree, do not emit two merges or silently replace
  one timestamp. Remove the conflicting merge match, retain other confirmed actions, and mark
  `history-ambiguous` with incomplete evidence. A provisional entry with no remaining confirmed
  selected match is removed from `prs`; its evaluation gap remains in the report. Agreement on
  merged disposition can still settle current state even when merge timing is uncertain.
- Retain only lifecycle event fields needed for matching/classification: ID, kind, time, and
  any supplied linkage evidence. Do not retain unrelated event bodies.
- Classify closures only with adequate lifecycle evidence. A closure followed by an unambiguous
  reopening before a later merge is an unmerged closure cycle. With complete consistent history
  and no merge, a closure is unmerged even when current state is open after reopening.
- A final closure coincident with merging must not be emitted as an additional unmerged action.
  Distinct timestamps/events that cannot be safely paired remain ambiguous: omit only the
  uncertain closure, retain the merge/other matches, and mark history evidence incomplete.
- Equal-time IDs are not chronological proof. Preserve distinct confirmed closure IDs, never
  combine them by timestamp. Missing IDs/times or inconsistent duplicate payloads are targeted
  gaps, not fabricated occurrences.
- Reopening is classification context; it does not qualify by itself. Full event pages may
  contain events after `until`, which can establish the meaning of an earlier closure.

## Inbox discovery and direct-review verification

Resolve a missing user first under the same budget. Repos/org list open PRs; evaluate author,
assignees, and direct `requested_reviewers` as a union. All known reasons travel with one entry.
Missing relationship arrays remain unknown. Requested teams do not substitute for direct users.

Search uses three distinct streams, in round-robin page order:

1. `is:pr is:open author:<user>`
2. `is:pr is:open assignee:<user>`
3. `is:pr is:open review-requested:<user>` (candidate only until direct review is verified)

All streams share visibility/target restrictions and one result/request budget. Author/assignee
hits with confirmed open state and satisfied draft selection may be retained immediately, with
incomplete reasons until verification. Broad reviewer provenance cannot alone prove a direct
request. Do not return a review-only candidate until personal requested-reviewer evidence exists.

For each newly encountered candidate repository, paginate its open PRs once. Retain output
metadata only for matching PRs, plus minimal per-PR verification facts for nonmatches/unknowns
and repository pagination status needed to reconcile earlier and later search hits. This discovery
may also establish additional matching PRs beyond search hits: record the mixed
`search+repo-listing` discovery method. Every added item must still satisfy the original explicit
target and visibility restrictions. Reuse this in-memory verification for later stream hits;
no persistent cache and no per-PR reviewer calls. Do not call a complete list again within the
same query. Page budget stops retain uncontradicted verified reasons and disclose unfinished
verification. Repository listing evidence takes precedence over search evidence, even if another
search stream returns the conflicting hit later in the same query.

Reconcile every listed candidate before discarding nonmatching metadata:

- Known relationship values replace earlier reasons for that relationship; a known empty list
  removes its earlier reason. Keep the entry only if at least one confirmed reason remains and
  its known state/draft still satisfies selection. Unknown fields do not prove removal; retain
  uncontradicted prior proof with incomplete reason coverage. Explicit state/draft mismatches
  remove the entry even if authorship still matches.
- After a successful, fully paginated open-PR list, remove search candidates absent from that
  list. Record `provider-changed` and incomplete evidence, without inventing a closed/merged
  state or a reason for absence. Later search hits cannot restore a candidate contradicted by
  that repository's verification in this query.
- An interrupted, failed, skipped, or incomplete list provides no proof from absence: retain
  uncontradicted authored/assigned matches with `reasons_complete=false`. Apply explicit
  contradictions already observed on fetched pages even when subsequent pagination fails.
- Every contradiction has an attributable `provider-changed` disclosure and makes overall
  `evidence_complete=false`, including when all entries are removed. Recompute reasons/count
  after removal; retain costs and disclosures. Discovery completion still follows exhausted
  traversal, not the resulting count. Removal does not restart stopped traversal or reset caps.

Inspect all three relationships for each listed PR. A team-only review candidate with no other
relationship is excluded. Pending direct requests removed or fulfilled no longer qualify; later
re-requested work qualifies again. A search/list disagreement is `provider-changed`, not evidence
of a transactional snapshot. Complete declared search plus list coverage is required before
claiming discovery complete. Missing optional stats do not invalidate known relationship coverage.

## Limits, errors, and partial evidence

At `max_prs`, do not add new identities; continue normalization of evidence already fetched.
Do not issue additional requests solely to prove that a reached cap was exact. If unexamined
work remains, report partial discovery/evidence. If everything was exhausted naturally at the
cap, report complete. Count distinct PRs, not actions, stream hits, or inclusion reasons.

| Outcome | Result | CLI / MCP |
| --- | --- | --- |
| Validation failure | No fabricated evidence report | exit 1 / tool error |
| Successful empty or complete query | Versioned report, arrays present | exit 0 / success |
| Own result/request cap or pagination guard | Confirmed evidence + affected coverage disclosures | exit 0 / success |
| Search incomplete/capped | Confirmed evidence + incomplete discovery | exit 0 / success, never a complete claim |
| Org-specific unreadability | Retain evidence, skip repo with reason; continue where useful | exit 0 / disclosed partial success |
| Explicit repo, identity, global/provider/rate-limit error or cancellation | Initialized report + attributable error; stop | exit 1 / `IsError=true` with structured report |
| Ambiguous history or missing optional metadata | Retain confirmed evidence; mark only affected coverage | exit 0 / disclosed result |

Reuse `skipRepoReason` for classification, checking rate limits before access-denial skips.
For an org history access failure, retain that repo's known PR evidence and disclose history
coverage; do not pretend all prior evidence vanished. Unavailable explicit search targets must
be disclosed as a search visibility limitation unless an actual access error identifies a target.

Transport errors are wrapped with operation/target context while preserving `errors.Is/As`;
rendered errors use sanitized classifications and safe next steps. Do not reuse activity's
quota-to-success `classifyStop` unchanged. No provider error becomes a silent empty success.
