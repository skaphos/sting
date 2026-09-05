# Data Model: PR Activity and Personal Inbox

This is the implemented v1 contract. Names below are public Go types in `model`; JSON names use
snake_case. Preserve existing `Commit`, `Result`, `ActivityResult`, and their schema constants.

## Queries

| Type | Fields and invariants |
| --- | --- |
| `PRQuery` | `provider=github`, `scope`, normalized `repos`, `org`, optional `author`, `role=author` when filtered (otherwise empty), `state=all` default, `draft=all`, `time_basis=any`, inclusive UTC `since`/`until`, `max_prs`, `max_requests` |
| `PRInboxQuery` | `provider=github`, `scope`, normalized `repos`, `org`, `user` (empty before authenticated resolution), `identity_source=explicit|authenticated`, fixed ordered relationships `[authored, assigned, review-requested]`, `state=open`, `draft=all`, `max_prs`, `max_requests`; no date fields |

- `PRState`: `open|merged|closed`; query-only `all` means no current-state restriction.
- `PRTimeBasis`: `created|updated|merged|closed|any`. `any` means original opening, merging,
  and unmerged closure occurrences. `updated` is explicit latest-update matching.
- `PRDraftFilter`: `all|only|exclude`; CLI maps `--draft` / `--no-draft` to the latter two.
- Empty provider selects GitHub regardless of the commit provider default; explicit GitLab fails.
- Reuse scope/default target resolution with validated GitHub login/org/repo tokens. PR users
  are GitHub logins, not commit-author email strings. Case-fold identities for comparison;
  preserve provider spelling for display. Normalize, deduplicate, and sort target repositories.
- Search requires an activity author. Inbox uses its resolved user instead. Repos requires a
  nonempty target list; org requires an org. Simultaneous org and repo search restrictions are
  intersected; reject conflicting or irrelevant explicit scope targets, never silently ignore them.
- Request overrides use pointers for caps and draft choice; zero disables a cap. Default
  `Config.MaxPRs=100`; reuse `MaxRequests=500`. Negative caps fail before provider access.
- Existing `resolveWindow` supplies UTC and precedence; no client-side `now` for selection.
  Inbox ignores configured activity windows and rejects explicit date/action inputs.

## PR metadata and availability

`PullRequest` is shared by `PRActivityEntry` and `PRInboxEntry`.

| JSON field | Go shape / meaning |
| --- | --- |
| `repo`, `number`, `url` | Required repository full name, positive PR number, provider URL; identity is provider + case-folded repo + number |
| `title`, `author` | Strings when supplied; unavailable facts also listed in `missing_fields` |
| `state` | `*PRState`; current state, never state at the historical action |
| `draft` | `*bool`; nil is unknown, false is known non-draft |
| `created_at`, `updated_at`, `merged_at`, `closed_at` | `*time.Time`, UTC; current-record timestamps, distinct from historical occurrence timestamps |
| `assignees`, `requested_reviewers` | `[]string`, sorted logins; nil is unknown, non-nil empty is known empty |
| `requested_teams` | `[]string`, sorted org/team identifiers; metadata only, not a personal inclusion reason |
| `base_ref`, `head_ref` | `*string`; missing or deleted refs remain unknown when not supplied |
| `labels` | `[]string`, sorted names with nil/empty distinction |
| `milestone` | `*string`, name only |
| `additions`, `deletions`, `changed_files` | `*int`; nil unknown, zero known zero; no detail request to populate them |
| `missing_fields` | Sorted `[]string` of unavailable metadata field names, always an array |

Emit metadata fields without `omitempty` so JSON preserves null versus known empty/zero.
For nullable facts that can be absent by meaning (e.g. no milestone, never merged), null with
no `missing_fields` entry means known absent; null with that entry means unavailable.
Search may lack a way to distinguish absent from omitted; use unavailable conservatively.
A closed search issue without confirmed merge disposition has unknown current state until
lifecycle/list evidence settles it. Defer an unknown merge-disposition filter to necessary
history reads, including for created/updated queries; reject only known mismatches before those
reads. Never apply an explicit state filter based on a guess.

Required identity failures exclude the candidate and produce a coverage disclosure; optional
metadata gaps alone do not make discovered PR membership incomplete. Do not put raw provider
payloads, bodies, credentials, or arbitrary HTTP error dumps into these types.

## Matching evidence

`PRActivityEntry`: `pr PullRequest`, `matches []PRMatch`, `matches_complete bool`.
A returned entry must have at least one confirmed selected match. The boolean means all selected
matching evidence for this PR has been evaluated; it is not a claim about all provider history.

`PRMatch` fields:

- `id string`: stable within the PR; `created` for the original opening, `event:<provider-id>`
  for lifecycle events, `merged` for one merge when known only from discovery, and
  `updated:<UTC timestamp>` for explicit latest-update evidence.
- `kind`: `opened|merged|closed|updated`; `at time.Time`; `source=record|issue-event`.
- `event_id *int64`: provider ID when supplied. No actor field in v1; author filtering concerns
  the PR author, not necessarily the actor who merged or closed it.

A provider merge event replaces an equivalent record-derived merge match rather than adding
another action. Conflicting record/event merge times invalidate that merge's window match;
retain other confirmed actions, remove any now-unmatched provisional PR, and disclose incomplete
history. Original creation is one match. Distinct closure event IDs remain distinct,
even at identical timestamps; duplicate pages cannot duplicate one event. Sort by `at`, fixed
kind order `opened, closed, merged, updated`, then ID; this tie-break is display order only.
Reopened events are internal classification evidence, not default matches. Ambiguous closed
versus merged classification is omitted with `matches_complete=false` and a targeted disclosure.

`PRInboxEntry`: `pr PullRequest`, `reasons []PRRelationship`, `reasons_complete bool`.
Reasons are fixed-order `authored, assigned, review-requested`. Every reason is confirmed;
team-only search matches do not establish `review-requested`. A partial verification can retain
an author/assignee match with `reasons_complete=false`. Nonmatching candidates are not entries.

## Results

`PRResult` uses `PRSchemaVersion = "sting.prs.skaphos.io/v1"` and `workflow=pr-activity`.
`PRInboxResult` uses `PRInboxSchemaVersion = "sting.pr-inbox.skaphos.io/v1"` and `workflow=pr-inbox`.
They have separate concrete `query` and `prs` types; no public mode-switched result union.

Shared top-level fields:

| Field | Meaning |
| --- | --- |
| `schema_version`, `workflow`, `provider`, `generated_at` | Provenance, with UTC generation timestamp |
| `query` | Resolved workflow query; unresolved self identity remains visibly unresolved only on an identity failure report |
| `prs`, `count` | Ordered entries and exact array length; empty arrays are `[]` |
| `coverage` | `PRCoverage`: booleans `discovery_complete`, `evidence_complete` for the selected query |
| `truncated` | True when either coverage dimension is false; optional missing metadata alone leaves it false |
| `cost` | `PRCostReport`, defined below |
| `disclosures` | `[]PRDisclosure`, including informational selection semantics and attributable gaps |

Complete is relative to the declared discovery surface and credential visibility, never a claim
about invisible private resources or a transactionally frozen provider snapshot. `evidence_complete`
is false for unexamined candidate matches, incomplete histories or reason checks, including when
no PRs could be returned. Per-entry completeness covers the retained entry only.

`PRDisclosure`: `kind`, `reason`, optional `next_action`, optional `repo`, `number`, `stream`.
Kinds include `visibility`, `time-basis`, `role`, `search-capped`, `search-incomplete`,
`request-budget`, `result-limit`, `pagination-limit`, `repo-skipped`, `history-incomplete`,
`history-ambiguous`, `reasons-incomplete`, `metadata-unavailable`, `provider-error`,
`identity-unresolved`, `provider-changed`. Stable sorting/deduplication preserves attribution.
Errors are sanitized summaries; do not copy credential-bearing request URLs or response bodies.

`PRCostReport`: `consumed`, `ceiling`, `identity_requests`, `discovery_requests`, `history_requests`.
The three categories sum to consumed actual dispatched HTTP requests, including redirects and
retries; rejected over-budget dispatches count zero. Inbox repo verification is discovery.
Optional `quota` contains the last observed remaining/limit/reset (not a global quota across
search and core resources); omit it when headers were not observed. No estimate endpoint or
extra quota preflight is part of this feature.

## Lifecycle and completeness rules

1. Resolve/validate locally; initialize the versioned report before any collection/identity call.
2. Admit only confirmed matches; retain them immediately even if later collection fails.
3. Later evidence may add reasons/actions or correct current metadata. Contradictory observations
   produce `provider-changed`; repository listing takes precedence over search, including later
   search hits. Remove disproven inbox reasons and entries with no remaining confirmed reason
   or an explicit state/draft mismatch. A complete open list also invalidates absent search
   candidates, without asserting why they disappeared; incomplete listing never proves absence.
   Unknown fields do not negate prior proof. Follow the [reconciliation contract](contracts/provider-collection.md)
   and mark overall evidence incomplete for contradictions, even when no entries remain.
4. A limit or failed next page alone never erases uncontradicted confirmed evidence; corrections
   from already-fetched contradictory evidence still apply. Only sting's own caps return a
   nil collection error; provider errors/cancellation retain a report plus an error.
5. Hitting exactly N entries is not alone proof of truncation. If there is no remaining discovery
   or evidence work, completion is true; otherwise stop and mark the appropriate dimension false.
6. Output PRs sort by case-folded repo then number. Selection is a deterministic gathered subset,
   not global newest-N activity. Unknowns, limits, and generation metadata remain auditable.
