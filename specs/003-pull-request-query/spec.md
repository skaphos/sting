# Feature Specification: PR Activity and Personal Inbox

**Feature Branch**: `feature/127-pull-request-query`

**Created**: 2026-09-05

**Status**: Draft

**Input**: User description: "Lets work on Issue #127 create a branch, $speckit-specify the feature"

**Source**: [Issue #127 — First-class pull-request evidence](https://github.com/skaphos/sting/issues/127),
expanded by the user on 2026-09-05 to include a separate personal PR inbox.

## Problem

Commit queries miss work that has not landed on a queried branch. Standup, release, and team
report consumers need to discover pull requests (PRs) opened, merged, or closed during the
requested window, including drafts and closed-unmerged work. Merely remaining open does not
qualify a PR for inclusion. The existing `include_prs` option discovers commits on open PR
branches; it does not return PR records. This feature specifies a separate, bounded PR query
with enough metadata to summarize the work and explain what the result can and cannot show.
Users also need a separate view of current work requiring their attention: open PRs they
created, are assigned to, or have been requested to review, regardless of age. This personal
inbox has its own command and agent tool, with inclusion reasons explaining why each PR appears. Both workflows remain
read-only: managing attention means finding and summarizing work, not assigning, reviewing,
closing, or merging PRs.

The behavior below is proposed, not a description of an already shipped capability.

## Clarifications

### Session 2026-09-05

- Q: Should an open-PR query without explicit dates include stale open PRs? → A: No. The query
  reports activity during the resolved time window: opened, merged, and closed actions.
  Default selection includes closed-unmerged activity; remaining open is not a qualifying
  action, and general updates are not part of the default action selection.
- Scope expansion: The user also requested a separate command for currently open PRs they
  authored or are assigned to. Include this personal workflow in the same specification,
  separate from the activity query; the inbox is not restricted to the activity window.
- Q: Should the personal inbox also include PRs requesting the user's review? → A: Yes. Include
  all three relationships: authored by the user, assigned to the user, and requesting their
  review. Label every applicable relationship on each deduplicated PR.
- Q: May activity collection make additional history requests within `max_requests` to recover
  actions such as closure during the window followed by reopening later? → A: Yes. Recover
  lifecycle actions within the request budget and return explicitly partial evidence if that
  budget prevents completion. Per-PR requests are permitted for lifecycle history only; this
  supersedes the issue's blanket prohibition on per-PR requests.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Find a person's PR activity during a window (Priority: P1)

A developer preparing a standup asks which PRs a person authored that were opened, merged,
or closed during a window, including PRs whose commits never reached a default branch.

**Why this priority**: This is the evidence gap that motivated issue #127.

**Independent Test**: Query a controlled set containing an open draft, an older PR merged in
the window, a PR matching several clocks, a closed-unmerged PR, and an unrelated author's PR.
Verify the selected records, their matching clocks, and the resolved query.

**Acceptance Scenarios**:

1. **Given** an author's open draft created in the window and an older PR merged in the window,
   **When** the author is queried with default state and time filters, **Then** both appear,
   labeled with their current state, draft flag, and the clocks that match the window.
2. **Given** one PR opened and merged in the window, **When** the query uses time basis `any`,
   **Then** it appears once with both opening and merging recorded as qualifying actions.
3. **Given** another author's PR and the requested author's PR closed without merge in the
   window, **When** the default author query runs, **Then** the closure is included and the
   other author's PR is excluded.
4. **Given** equivalent explicit windows expressed with different timezone offsets, **When**
   they are queried against unchanged provider state, **Then** they yield the same ordered
   evidence and normalized boundaries, apart from report-generation metadata.
5. **Given** a search query without an author, **When** it is submitted, **Then** it fails
   validation before collection and explains that search requires an author.
6. **Given** an older PR still open, with only a general update during the window, **When** the
   default query runs, **Then** it is excluded. An explicit `time-basis=updated` query may
   include it with its update basis stated.
7. **Given** a PR opened within the window and closed afterward, **When** the default query
   runs, **Then** it is included for its opening, with current state reported separately.
8. **Given** an older PR closed during the window and reopened afterward, **When** the default
   query or a `time-basis=closed` query runs with sufficient budget and accessible history,
   **Then** the PR appears for its historical closure even though its current state is open.
9. **Given** a PR closed, reopened, and closed again during the window, **When** activity is
   queried, **Then** the PR appears once with both closure occurrences and their timestamps;
   reopening alone does not qualify under the default action selection.

---

### User Story 2 - Inspect repository PR activity (Priority: P1)

A maintainer asks for PRs in named repositories without having to select an author, then
filters by state or draft status to identify open work, shipped changes, or abandoned work.

**Why this priority**: Repository questions are a distinct primary query shape in the issue.

**Independent Test**: Query two named repositories with known open, draft, merged, and
closed-unmerged records, exercising each state and each draft selection.

**Acceptance Scenarios**:

1. **Given** PRs from several authors in two named repositories, **When** the maintainer uses
   `scope=repos` with `state=open`, **Then** matching open PRs from all authors are returned,
   including drafts unless excluded explicitly.
2. **Given** merged and closed-unmerged PRs, **When** `state=closed` is selected, **Then** only
   closed-unmerged PRs are returned; `state=merged` selects only merged PRs.
3. **Given** PRs merged inside and outside the window, **When** `state=merged` and
   `time-basis=merged` are selected, **Then** only those actually merged within the window appear.
4. **Given** an explicit author or draft selection, **When** the repository query runs,
   **Then** the selected records satisfy that filter as well as state and time filters.
5. **Given** an open PR last changed before the configured default window, **When** no dates
   are supplied, **Then** it is excluded and the report states the resolved window; the report
   does not claim to be an all-time queue.

---

### User Story 3 - Gather a team digest with honest private visibility (Priority: P1)

A team lead queries an organization, optionally filtering by author or state, and can tell
which repositories were covered and which could not be read.

**Why this priority**: Private organization access and attributable gaps are required for
credible team evidence.

**Independent Test**: Query an organization containing accessible public and private
repositories and a repository with an access failure, using controlled provider responses.

**Acceptance Scenarios**:

1. **Given** accessible public and private repositories, **When** `scope=org` is requested,
   **Then** matching PRs across those repositories appear with repository identities and
   organization coverage recorded, without requiring an author.
2. **Given** one inaccessible repository and one accessible repository, **When** the
   organization query runs, **Then** available evidence is returned with the skipped repository
   and reason identified in structured and human-readable output.
3. **Given** author search without an organization or repository restriction, **When** it runs,
   **Then** discovery is public-only and that limitation is stated. Explicitly restricted search
   permits private results visible to sting's credential and states its search-based coverage.
4. **Given** a caller-named repository that cannot be read, **When** a `repos` query runs,
   **Then** it reports a query failure with that repository and reason rather than treating it
   as an ordinary organization skip, while preserving any evidence already gathered.

---

### User Story 4 - Consume bounded evidence through CLI and agents (Priority: P2)

A human or agent runs a PR query with predictable limits and can distinguish an empty result,
a complete result, and a partial result. Additional lifecycle-history work is allowed within
the budget; unrelated detail enrichment must not silently increase the cost.

**Why this priority**: Reliable cost and completeness reporting make the primary queries
usable in repeated standup and reporting workflows.

**Independent Test**: Submit equivalent CLI and MCP queries, compare structured evidence,
and repeat with small result/request caps and injected provider failures.

**Acceptance Scenarios**:

1. **Given** the same resolved query and unchanged provider state, **When** `sting prs` and
   `get_prs` run, **Then** they return equivalent PR evidence and disclosures; Markdown groups
   records by organization/repository and identifies drafts, pending review requests, and merges.
2. **Given** more matching records or more remaining collection work than the configured cap,
   **When** that cap stops collection, **Then** gathered evidence is returned with truncation,
   the stopping reason, and actual request use; a run stopped only by its own caps exits
   successfully through the CLI.
3. **Given** different PR counts fitting within the same discovery pages and requiring no
   additional lifecycle-history work, **When** the query runs, **Then** request count does not
   grow once per PR. No PR diffs or optional per-PR detail enrichment are fetched.
   When lifecycle recovery requires more requests, those requests count against the same cap
   and the cost report identifies that history work.
4. **Given** missing optional metadata, **When** evidence is rendered, **Then** unavailable
   values are distinguishable from known empty lists, false values, and zero counts.
5. **Given** an incomplete search or a global failure after evidence was gathered, **When**
   collection ends, **Then** the report retains evidence and identifies the missing coverage
   and failure; it does not present a successful complete result.
6. **Given** GitLab or an unsupported role is selected, **When** the query is submitted,
   **Then** it returns an explicit unsupported-selection error before provider collection.
7. **Given** some confirmed actions and more lifecycle history to collect, **When** the request
   budget expires, **Then** confirmed actions remain in the report, history coverage is marked
   incomplete with the affected PRs or scopes identified, and the CLI exits successfully for
   that budget stop. PRs without a confirmed match are not presented as matching evidence.

---

### User Story 5 - Find current PRs needing my attention (Priority: P1)

A developer opens a personal inbox to see PRs they authored, are currently assigned to, or
have been requested to review, including older work that still needs attention. Each item
explains the relationship to the user, allowing them to follow its link and act in the provider.

**Why this priority**: The user explicitly requested current workflow management alongside
windowed activity; age alone must not hide outstanding work.

**Independent Test**: Query a controlled dataset containing an old authored draft, an old
assigned PR, an old PR requesting their review, a PR matching several relationships,
unrelated open PRs, and previously assigned or completed PRs. Compare the command and agent
results.

**Acceptance Scenarios**:

1. **Given** an open PR authored by the user months ago and another currently assigned to the
   user, **When** `sting inbox` or `get_pr_inbox` runs, **Then** both appear regardless of age,
   with `authored` and `assigned` inclusion reasons respectively.
2. **Given** a PR both authored by and assigned to the user, **When** the inbox runs, **Then**
   it appears once with both reasons and counts once toward the PR cap.
3. **Given** a merged PR, a closed-unmerged PR, and an open PR whose only relationship was an
   assignment since removed, **When** the inbox runs, **Then** none appears. Unrelated open
   PRs are also excluded.
4. **Given** an authored draft, **When** the default inbox runs, **Then** it appears labeled as
   a draft; an explicit non-draft selection excludes it.
5. **Given** explicit repository or organization restrictions, **When** the inbox runs,
   **Then** each returned PR satisfies both the restriction and an inbox relationship. Omitted
   restrictions use the configured scope/targets and disclose their visibility limits.
6. **Given** a default seven-day activity window and an older assigned PR, **When** the inbox
   runs, **Then** the activity window does not apply; the report explicitly identifies its
   current-work selection and resolved user.
7. **Given** identical visible provider state, **When** the inbox is run before and after an
   activity query, **Then** its selection is unchanged. Inbox requests do not inherit activity
   time/state filters, and neither workflow modifies provider state.
8. **Given** no qualifying open work or a collection cap/failure, **When** the inbox runs,
   **Then** it distinguishes an empty complete inbox from partial or failed collection, retains
   gathered evidence, and reports costs and coverage gaps under the same rules as activity.
9. **Given** an old open PR requesting the user's review, **When** the inbox runs, **Then** it
   appears with `review-requested` as its inclusion reason even if the user neither authored it
   nor is assigned to it. A removed or fulfilled request no longer qualifies through that reason,
   but another current relationship can keep the PR in the inbox.
10. **Given** a PR matching all three relationships, **When** the inbox runs, **Then** it appears
    once with `authored`, `assigned`, and `review-requested` reasons.
11. **Given** an invalid configured activity window, **When** the agent server starts and the
    inbox is queried, **Then** the inbox remains usable. An activity call relying on that
    invalid default fails locally without preventing a subsequent inbox call. Malformed
    configuration and invalid shared settings still prevent startup.
12. **Given** search evidence contradicted by repository verification, **When** inbox results
    are reconciled, **Then** disproven reasons are removed; a PR with no remaining confirmed
    relationship, an explicit selection mismatch, or absence from a complete open-PR listing
    is excluded. The report discloses the disagreement without inventing why the PR disappeared.
    Later search hits cannot restore contradicted matches during that query. Interrupted
    verification retains uncontradicted proof and discloses incomplete coverage.

### Edge Cases

- An empty successful result contains the resolved query, zero records, cost, and coverage
  disclosures; failed or incomplete discovery returning zero records is distinguishable.
- Timestamps exactly at either boundary match; equal boundaries are valid. Reversed boundaries,
  malformed dates, and invalid windows fail validation. Explicit `since` overrides `window`.
- An older PR merged or closed in the window can match `any`; merely remaining open or
  receiving a general update cannot. The explicit `updated` basis uses the latest update
  timestamp and does not prove every historical update.
- An unmerged PR has no merge timestamp and cannot match `merged`. Missing timestamps must not
  be invented or inferred from the close time; inability to establish a requested match is
  disclosed rather than silently represented as complete coverage.
- `time-basis=merged` with `state=open` or `state=closed` is contradictory and fails validation.
- A PR discovered through several actions or overlapping repository restrictions counts once
  toward the result cap. Its evidence preserves distinct qualifying action occurrences, while
  duplicate discovery of the same occurrence must not duplicate it.
- `closed` activity means closure without merging at that occurrence. A merge is labeled
  `merged`, not counted again as a separate closure. An earlier unmerged closure remains a
  `closed` action even if the PR is later reopened and merged.
- Later updates, reopening, or another closure must not erase an earlier qualifying action.
  Current state and latest timestamps cannot substitute for lifecycle history. If that history
  cannot be fully discovered or read within provider/budget limits, disclose incomplete coverage.
- `time-basis=closed` with `state=open` is valid: a reopened PR can have a historical closure.
  `created` means the original opening; reopening is not a second creation action.
- Conflicting draft selections, unknown states/roles/time bases/scopes, missing scope targets,
  malformed repository identifiers, and negative caps fail validation before collection.
- A search ceiling or provider-declared incomplete response marks the result incomplete even
  when no records are returned. The disclosure identifies which discovery work was affected.
- Authentication failures, rate limits, and provider-wide errors stop collection rather than
  being repeatedly treated as repository-specific skips; accumulated evidence is retained.
- Private repositories invisible to the credential cannot be enumerated as known skips. The
  result states that coverage is limited to credential-visible repositories.
- Pending review requests are metadata, not proof that a review was submitted or approved.
- Inbox authorship, current assignment, and pending review requests are a union, not an
  intersection. A removed assignment or removed/fulfilled review request no longer qualifies
  unless another supported relationship still does. A submitted review alone is not a pending
  request; a new review request can qualify the PR again.
- The inbox must never guess who “me” is from an ambient provider token or local Git author.
  If the selected user cannot be resolved, fail with an actionable explanation.
- Inbox time-basis, window, or non-open state inputs must be rejected as inapplicable, directing
  the caller to `sting prs` / `get_prs` for activity queries; configured activity defaults do not
  cause inbox validation errors or silently filter its records.

## Requirements *(mandatory)*

### Functional Requirements

FR-002 through FR-007 define activity-query selection. FR-008 through FR-017 apply to both
workflows where applicable; inbox selection is defined separately by FR-018 through FR-022.
FR-023 defines activity history recovery; FR-024 defines evidence completeness for both.

- **FR-001**: Expose `sting prs` / `get_prs` for windowed PR activity and `sting inbox` /
  `get_pr_inbox` for the personal PR inbox. Each workflow MUST have an explicit, versioned
  query/result contract that makes its selection semantics identifiable. Existing commit and
  repository-activity queries, commit result schema/version, and `include_prs` semantics MUST
  remain compatible. In MCP, invalid workflow-specific defaults MUST reject affected tool
  calls before collection rather than prevent unrelated tools from starting; shared startup
  validation and existing standalone command validation remain enforced.
- **FR-002**: Support GitHub `search`, `repos`, and `org` scopes. Search MUST require an author;
  repository and organization queries MUST allow any author or an explicit author filter.
  Reuse applicable configured scope, repository, and organization defaults and validate the
  resolved selection. Explicit request values override configuration.
- **FR-003**: Support `state=open|merged|closed|all`. `closed` MUST mean closed without merge.
  An omitted state MUST resolve to `all`, so current state does not hide qualifying opened,
  merged, or closed actions. An explicit state filter selects current provider state at
  collection time in addition to the window/action filter; it does not select an action.
- **FR-004**: Support draft-only, non-draft-only, and unrestricted draft selection, independently
  of state. Omission MUST include both drafts and non-drafts.
- **FR-005**: Support `time-basis=created|updated|merged|closed|any`, defaulting to `any`.
  `created` denotes opening; `any` selects the union of opened, merged, and closed actions
  within the inclusive resolved window. General updates qualify only when `updated` is
  explicitly selected. A record MUST have at least one confirmed match, and identify each
  qualifying action and its timestamp separately from current PR state. Duplicates MUST be
  combined by repository and PR number. The result MUST state its selected basis and the
  limitations of latest-update matching.
- **FR-006**: Resolve `since`, `until`, and `window` using the existing precedence and UTC window
  semantics: explicit `since` wins over `window`; omitted `until` uses one captured request time;
  an omitted window uses the configured default (built-in seven days). All query shapes MUST
  disclose these bounds, including state-only queries.
- **FR-007**: Support only `role=author` in the MVP, defaulting to that role when an author is
  present. Results MUST distinguish an author filter from an unfiltered repository/org query.
  Unsupported role selections MUST fail explicitly rather than silently substituting authorship.
- **FR-008**: Each PR MUST carry repository identity, number, title, URL, current state, draft
  status, author, current-record created/updated/merged/closed timestamps, and workflow-specific
  matching evidence when available. Activity records MUST also preserve distinct qualifying
  lifecycle occurrences, including historical closures absent from current-record timestamps. Also return assignees, requested reviewers (people or teams), base/head refs,
  labels, milestone name, and additions/deletions/changed-file counts when supplied by discovery.
  Unknown values MUST be distinguishable from known empty/zero values; missing metadata and its
  effect on matching or completeness MUST be disclosed. No unavailable fact may be fabricated.
- **FR-009**: Every report MUST contain provider, schema version, generation time, the resolved
  workflow and scope/targets/filters/limits, PR records, cost, truncation/completeness information,
  and attributable disclosures/errors. Activity reports MUST include the resolved window and
  actions; inbox reports MUST identify the resolved user, relationships, and absence of a time
  restriction. Evidence MUST have deterministic ordering for unchanged upstream state;
  generation metadata is not part of evidence equality.
- **FR-010**: Search without an org/repository restriction MUST be public-only. Explicitly scoped
  search and repository/org listing MUST respect sting's credential visibility. Reports MUST
  identify search versus listing discovery and applicable coverage limits, including search's
  1,000-hit ceiling and incomplete-result indication where applicable. They MUST NOT transfer
  commit search's default-branch restriction to PR search.
- **FR-011**: Organization enumeration MUST continue past repository-specific unreadability,
  recording each skipped repository and reason. Explicit repository failures and global failures
  MUST be surfaced as errors and stop collection, retaining any prior evidence. Rate-limit and
  authentication failures MUST not be misclassified as skippable access failures.
- **FR-012**: Enforce `max_prs` (built-in default 100) and the existing `max_requests` setting
  (built-in default 500). Zero MUST explicitly disable that particular cap; negative values MUST
  be rejected. All provider calls made for the query MUST count toward its request ceiling.
  Discovery and inbox cost MUST scale with discovery/enumeration pages, not optional detail
  fetches per PR. Activity collection MAY make additional per-PR lifecycle-history requests
  needed to establish qualifying actions; these MUST share the request ceiling and be reported
  as history cost. This exception MUST NOT permit per-PR diffs or optional metadata enrichment.
- **FR-013**: When caps or provider discovery limits prevent completion, return available
  evidence with truncation and an attributable reason. Reaching sting's own caps alone MUST be
  successful, with CLI exit status 0. Provider failures MUST remain distinguishable from planned
  budget stops. A fully exhausted result exactly equal to a cap MUST not be called incomplete
  solely because its count equals that cap.
- **FR-014**: JSON MUST be the complete structured record. Markdown MUST be a view of that record,
  grouped by organization/repository, with draft, pending-review-request, and merged indicators,
  plus visible selection criteria, costs, and coverage gaps. Activity rendering MUST show its
  window and qualifying actions; inbox rendering MUST show the selected user and each PR's
  inclusion reasons. Equivalent CLI/MCP requests MUST yield equivalent evidence.
- **FR-015**: Collection MUST be read-only and use sting's dedicated credentials and existing
  configuration precedence; it MUST neither use ambient provider tokens nor expose secrets.
  Both new MCP tools MUST carry the read-only annotation and participate in the existing
  derived registration/installer approval contract, with the read-only drift check covering
  all tools.
- **FR-016**: GitHub MUST be both workflows' default provider. An explicit GitLab selection MUST
  fail with a clear unsupported-provider explanation before collection, as activity does.
- **FR-017**: README and query help MUST explain activity's three scope shapes and the personal
  inbox, defaults, windows, state/action/relationship distinctions, costs, private visibility,
  unavailable metadata, history-recovery costs and completeness, and what is not covered.
  They MUST describe `include_prs` as commit
  discovery only, GitLab as unsupported here, and detailed enrichment as outside this MVP.
  Examples MUST distinguish “what happened during this window” from “what needs my attention
  now,” including older open inbox work and closed-unmerged activity.
- **FR-018**: The personal inbox MUST select currently open PRs authored by, currently assigned
  to, OR currently requesting review from the resolved user. These three relationships MUST be
  combined as a union. It MUST include drafts by default, support explicit draft selection,
  and exclude completed work. Age MUST NOT disqualify an otherwise matching PR.
- **FR-019**: The inbox MUST default to the user identified by sting's dedicated GitHub
  credential, with an explicit user override permitted. It MUST record the resolved login;
  identity resolution MUST use read-only access and count against the request budget. Failure
  to resolve identity MUST be explicit rather than silently selecting a different person.
- **FR-020**: Inbox results MUST label each PR with every confirmed inclusion reason:
  `authored`, `assigned`, and `review-requested` where applicable, and deduplicate by repository
  and PR number. Current assignments and pending review requests MUST be distinguishable from
  past assignments and removed/fulfilled requests. Unavailable relationship evidence MUST not
  be interpreted as proof of a match or a complete absence of matching work.
- **FR-021**: The inbox MUST support the existing `search`, `repos`, and `org` scope vocabulary,
  configured scope/target defaults, and explicit target overrides. Its resolved user replaces
  activity search's required author selector; relationship selection MUST remain a union in
  every scope. Unrestricted search remains public-only, and private visibility MUST be disclosed
  under FR-010. Default limits remain 100 PRs and 500 requests unless configured otherwise.
- **FR-022**: Inbox queries MUST have no time restriction and MUST not inherit activity's
  time/state/role filters. Inapplicable explicit inputs MUST fail with guidance to use the
  activity query. Both workflows MUST remain read-only discovery and summary surfaces; no
  assignment, review submission, merge, close, or other provider mutation is included.
- **FR-023**: Activity queries MUST recover qualifying opened/merged/closed occurrences even
  when later PR changes remove or replace the corresponding current-record timestamps.
  Discovery MUST account for this history, not exclude candidates solely because their latest
  update or closure is outside the window. Within accessible history and sufficient budget,
  preserve each matching occurrence with its action and timestamp. If discovery or history
  retrieval cannot complete, retain confirmed matches and disclose the affected coverage.
- **FR-024**: A returned PR MUST have at least one confirmed activity action/time match or inbox
  relationship, as appropriate. Unconfirmed candidates MUST NOT be presented as matches; gaps
  preventing their evaluation MUST be disclosed. Missing optional metadata MUST be labeled
  unavailable, without alone implying that PR discovery is incomplete. Unexamined candidates,
  missing qualifying history, or unverified inclusion reasons MUST be reported separately from
  optional metadata gaps; a report MUST NOT claim complete matching evidence in those cases.

### Key Entities *(include if feature involves data)*

- **PR activity query**: The resolved provider, scope, repository/org targets, optional author
  and role, effective state and draft filters, selected actions/time basis, UTC boundaries,
  and collection limits.
- **Lifecycle occurrence**: One provider-evidenced opening, merge, or unmerged closure of a PR,
  with its action, timestamp, and identity sufficient to distinguish repeat closures from
  duplicate discovery. Occurrences remain distinct from the PR's current state and timestamps.
- **PR inbox query**: The resolved provider, user, scope/targets, current relationships, draft
  selection, and collection limits; no activity window.
- **Inbox inclusion reason**: A confirmed current relationship connecting one PR to the selected
  user: authorship, current assignment, or a pending review request. One PR can have several
  reasons.
- **Pull request evidence**: One provider-owned PR identified by repository and number, its
  available metadata, current state, timestamps, and workflow-specific evidence: qualifying
  actions for activity or current inclusion reasons for the inbox.
- **PR report**: The versioned query and its ordered evidence, generation time, collection cost,
  completeness, and disclosures; independent of the existing commit result contract.
- **Disclosure or collection error**: An attributable visibility limit, unavailable evidence,
  skipped repository, provider failure, or budget stop, with a reason and safe next action
  where one exists.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In controlled complete datasets below all caps, all three scopes for both activity
  and inbox return 100% of matching PRs, zero nonmatching PRs, and zero duplicate PR identities.
- **SC-002**: Standup consumers can distinguish open, draft, merged, and closed-unmerged work
  and explain every returned PR's qualifying opened/merged/closed actions directly from the
  report, without another lookup for facts already supplied by discovery. Default reports
  include zero PRs solely because they remain open or have a general update in the window.
- **SC-003**: Every intentionally bounded or incomplete test run identifies its stopping reason
  or coverage gap, retains gathered evidence, and never exceeds its nonzero request/result cap.
- **SC-004**: With a fixed set of discovery pages and no additional lifecycle-history work,
  increasing record count adds zero per-PR detail requests. In history-recovery fixtures,
  100% of additional lifecycle requests are included in both reported cost and budget use;
  zero requests fetch diffs or optional per-PR metadata enrichment.
- **SC-005**: Equivalent CLI and agent queries produce identical normalized evidence across
  all acceptance fixtures; human-readable reports introduce zero facts absent from that evidence.
- **SC-006**: All existing commit/activity compatibility checks and MCP read-only drift checks
  continue to pass, and every new behavior has direct tests without live provider access.
- **SC-007**: In complete inbox fixtures, users can identify every returned PR's relationship
  from the report alone; 100% of qualifying older open work remains eligible regardless of the
  configured activity window, and zero completed or unrelated PRs appear.
- **SC-008**: With accessible complete history and sufficient budget, fixtures with later
  reopening, repeated closures, and updates after the window retain 100% of qualifying
  lifecycle occurrences, with zero duplicated occurrences and zero stale-open-only matches.

## Assumptions

- The user's clarification supersedes the issue's proposed defaults in every scope: omitted
  state means `all`, and omitted time basis selects opened, merged, and closed actions. No
  explicit window means the existing configured window. Selecting `state=open` or `state=all`
  does not disable activity time filtering. The separate inbox deliberately has no time cutoff.
- Current state is report context, distinct from qualifying actions during the window. An
  explicit state filter is an additional restriction; author filtering means the PR's author,
  not necessarily the person who performed its merge or closure.
- Optional metadata is limited to evidence supplied by discovery or already-required history
  reads. Planning must verify discovery and history coverage, field availability, occurrence
  identity, and time precision. The user-authorized lifecycle exception permits additional
  requests for qualifying activity history; it does not permit separate reviewer-metadata,
  stats, diff, or other optional enrichment calls.
- `closed` actions describe unmerged closures at the time they occurred. Merge is its own action.
  Reopening can explain later state changes but is not itself a default qualifying action.
- A capped report promises a deterministic gathered subset with explicit coverage gaps, not
  the globally newest PRs across all repositories or a complete action list for every retained
  PR. Planning must define stable traversal and ordering consistent with this contract.
- GitLab merge requests; non-author role filtering for the activity query; base-branch
  filtering; files/diffs; review bodies; check-run rollups; general event history beyond the
  selected PR lifecycle actions; and provider mutation are outside the MVP. Recovery of
  qualifying lifecycle occurrences is in scope and subject to the shared request budget. Current assignee and pending review-request matching are in
  scope for the separate inbox. Requested-reviewer metadata remains in scope where available.
  Direct requests to the selected user are included; requests to a team are shown as team
  metadata and do not imply a personal request without confirmed membership evidence.
- `sting inbox` and `get_pr_inbox` are the proposed separate entrypoint names. Inbox identity
  defaults to sting's authenticated user and permits an explicit user override, avoiding an
  implicit dependency on local Git identity or an ambient provider session.
- sting retains its existing configuration/credential model and stores no durable PR cache or
  consumer-specific interpretation of the evidence.

### Dependencies and Binding Decisions

- [Constitution](../../.specify/memory/constitution.md), especially read-only collection,
  reconstructible queries, dedicated credentials, partial evidence, and the public contract.
- [ADR 0010](../../docs/adr/0010-multi-tool-mcp-server.md): distinct questions/result shapes use
  sibling read-only tools and a derived tool-registration/approval list; activity and inbox
  therefore receive distinct tool entrypoints.
- [ADR 0004](../../docs/adr/0004-public-packages-and-wake-evidence.md): the evidence contract is
  public and independently consumable; PR evidence must not break the commit contract or depend
  on Wake or another consumer.
- [ADR 0006](../../docs/adr/0006-gitlab-provider-support.md): provider differences must be explicit;
  a new GitHub capability does not imply matching GitLab coverage.
- The Skaphos [ecosystem assessment](https://github.com/skaphos/skaphos-resources/blob/main/tools/ECOSYSTEM.md)
  was checked in the sibling checkout: it has no sting/PR-query adoption verdict. The existing
  ADR 0004 rationale governs this extension; the plan must assess relevant reusable components
  before proposing new ones.
- The plan's Constitution Check must pass before implementation. This specification does not
  supersede any ADR; concrete design and provider-contract verification belong to planning.
