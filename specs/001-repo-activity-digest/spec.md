# Feature Specification: Repository Activity Digest

**Feature Branch**: `feature/repo-activity-digest`

**Created**: 2026-07-25

**Status**: Draft

**Input**: User description: "As a developer I want to be able to query a repository to see what's happened in a period of time, however, like the issue we're running into with users, we may need to 1. get a commit list without diffs and then 2. run a diff on the head between commit at start and finish time to detect the deltas, correlate that with commit messages to build a story, the concern is that we wipe out the github api rate limit just asking what I've done in the last week (granted I'm a bit heavier than the average developer)"

## Problem

sting can already answer "what did *this person* commit in this window", but two gaps block the
question the user actually asks:

1. **There is no repository-scoped question.** Every query today requires an author — an empty
   author is rejected at request resolution. "What happened in this repo last week?" cannot be
   asked at all.
2. **Evidence depth costs one provider request per commit.** Asking for stats, files, or diffs
   triggers a per-commit detail fetch. A heavy week — hundreds of commits — turns a single
   question into hundreds of requests, and a handful of such questions exhausts the hourly quota.
   The caller has no way to know the cost before paying it, and no way to cap it.

The consequence is that the evidence sting exists to provide is the evidence a user cannot
afford to ask for. This feature makes window-scoped activity affordable: discover commits
cheaply, derive the window's file-level change set with a request count that does **not** grow
per commit, and disclose both the cost and the fidelity tradeoff in the result.

## Clarifications

### Session 2026-07-25

- Q: Should a repository activity query enforce a default provider-request ceiling, or only cap
  when the caller explicitly asks? → A: A generous default ceiling is always in force, sized so a
  typical one-week query completes within it, and is explicitly overridable. The default is a
  fixed value, not derived from remaining quota, so determinism holds.
- Q: How should the repository activity capability be exposed on the agent-facing (MCP) surface?
  → A: A new, second read-only tool alongside the existing commit-query tool, leaving that tool's
  contract unchanged.
- Q: What shape should the activity digest result take, given Wake consumes the commit-query
  result as a pinned contract? → A: A new sibling result type with its own schema version. The
  existing result contract does not change and its schema version is not bumped.
- Q: Which commit anchors the start of the window comparison? → A: The parent of the earliest
  in-window commit — the repository state immediately before the window — resolved by ancestry
  rather than by timestamp proximity.
- Q: Is GitLab in scope for this feature? → A: No. GitHub only; a GitLab repository-activity
  request fails with a clear, provider-specific reason, and the coverage gap is documented.
  Existing GitLab commit queries are unaffected.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ask what happened in a repository over a window (Priority: P1)

A developer names a repository and a time window and receives the commits in that window
together with the aggregate file-level change set for the window — without naming an author and
without paying a per-commit cost. The result is complete enough to hand to an agent and ask
"summarize what shipped here last week".

**Why this priority**: This is the missing capability. Without it there is no repository-scoped
question at all, and every other story in this spec is a refinement of it.

**Independent Test**: Run a window query against a repository with a known commit history and a
known start/end state; verify the returned commit list matches the window, the returned change
set matches the difference between the window's boundary states, and the number of provider
requests issued is bounded by the number of commit *pages* rather than the number of commits.

**Acceptance Scenarios**:

1. **Given** a repository with 250 commits in the last 7 days, **When** the developer requests
   that repository's activity for the last 7 days, **Then** the result contains all 250 commits
   with full messages, plus the window's aggregate per-path change set, and the run issues an
   order-of-magnitude fewer provider requests than one-per-commit.
2. **Given** a repository with no commits in the requested window, **When** the developer
   requests that window, **Then** the result is an explicit empty activity report — zero
   commits, an empty change set, resolved boundaries stated — and not an error.
3. **Given** a repository the caller's credential cannot read, **When** the developer requests
   its activity, **Then** the result states the repository and the reason it could not be read,
   and does not present an empty result as though the repository were quiet.
4. **Given** a window query, **When** the result is returned, **Then** it echoes the resolved
   query — repository, reference, normalized window boundaries, the boundary commits used, and
   the schema version — so the result can be re-derived and audited.

---

### User Story 2 - Know and cap what a query will cost (Priority: P2)

Before running a query, the developer can see what it is estimated to cost in provider requests
and how much quota remains. The developer can set a ceiling; when the ceiling or the provider's
own limit is reached, the query stops and returns what it gathered, labeled, rather than
aborting or silently blowing the budget.

**Why this priority**: This is the stated concern. Story 1 makes the question affordable in the
common case; this story makes the cost *visible and bounded* so a heavy user can trust the tool
in the uncommon case. It is independently valuable — cost estimation and budgeting apply to
every existing query path, not only the new one.

**Independent Test**: Run a query in estimate-only mode and confirm no evidence-gathering
requests are issued and an estimate is returned; then run the same query with a ceiling set
below the estimate and confirm the result is partial, explicitly labeled as budget-stopped, and
names what was not gathered.

**Acceptance Scenarios**:

1. **Given** a window query, **When** the developer asks for an estimate instead of a result,
   **Then** the tool reports the estimated provider request count and the remaining quota
   without issuing evidence-gathering requests.
2. **Given** a request ceiling lower than what the query needs, **When** the query runs, **Then**
   the result contains the evidence gathered up to the ceiling, is marked as bounded by budget,
   states which repositories or window segments were not covered, and states the next safe
   action.
3. **Given** the provider's quota is exhausted mid-query, **When** the query is running, **Then**
   the result contains everything gathered before exhaustion together with an attributable error
   naming the limit and its reset time — never a silent empty result and never an all-or-nothing
   failure.
4. **Given** any completed query, **When** the result is returned, **Then** it reports the
   provider requests actually consumed, the ceiling that was in force, and the quota remaining at
   completion.
5. **Given** a developer who sets no ceiling at all, **When** a query runs, **Then** the
   documented default ceiling applies, is reported in the result, and is the same value on every
   run regardless of how much quota remains.

---

### User Story 3 - Correlate the change set with the commit narrative (Priority: P3)

The developer receives the window's change set alongside the ordered commit messages, with any
path-to-commit association explicitly labeled as observed or inferred, so an agent can construct
an accurate account of the window's work without the tool overstating what it actually knows.

**Why this priority**: This turns a commit list plus a diff into a usable story. It depends on
Stories 1 and 2 for its inputs, and it is the part most at risk of quietly manufacturing
confidence — so it ships after the cheap, honest foundation is in place.

**Independent Test**: Run a window query against a repository where the per-commit file
attribution is known; confirm every path-to-commit association in the output is tagged with how
it was determined, and that no association is presented as observed unless per-commit file data
was actually retrieved for that commit.

**Acceptance Scenarios**:

1. **Given** a window query run without per-commit file retrieval, **When** the result associates
   a changed path with a commit, **Then** that association is labeled inferred and records the
   basis for the inference.
2. **Given** a window query where per-commit file data was retrieved for a bounded subset of
   commits, **When** the result is returned, **Then** associations for that subset are labeled
   observed, associations outside it are labeled inferred, and the result states how the subset
   was chosen.
3. **Given** a change set derived from comparing the window's boundary states, **When** a file
   was created and deleted entirely inside the window, **Then** the result discloses that
   boundary comparison cannot see such churn, so the absence of that path is not read as evidence
   that no work touched it.

---

### User Story 4 - Apply the same cost discipline to author-scoped queries (Priority: P4)

The existing "what did I do this week" query gains the same estimate, ceiling, partial-result,
and consumption-reporting behavior, so the rate-limit concern is addressed on the path where it
was first hit rather than only on the new repository-scoped path.

**Why this priority**: The original complaint ("we wipe out the rate limit just asking what I've
done in the last week") is on the existing path. It ships last only because the mechanism must
be proven on the new, simpler path first; it is not optional.

**Independent Test**: Run an existing author-scoped query with evidence depth requested and a
ceiling set below its cost; confirm it returns partial, labeled results with consumption
reported, matching the behavior specified in Story 2.

**Acceptance Scenarios**:

1. **Given** an author-scoped query requesting per-commit evidence across many commits, **When**
   an estimate is requested, **Then** the estimate reflects the per-commit cost of that request
   shape.
2. **Given** an author-scoped query with a ceiling, **When** the ceiling is reached, **Then**
   partial results are returned with the same labeling and disclosure as Story 2.

---

### Edge Cases

- **Empty window**: no commits between the boundaries — boundary commits resolve to the same
  commit, the change set is empty, and the result says so explicitly rather than erroring.
- **Window opens at repository inception**: the earliest in-window commit is the root commit and
  has no parent, so the start boundary resolves to the empty repository state (FR-008b); the
  change set is everything added since inception, and the result reports the boundary as such.
- **History rewritten inside the window** (force-push, rebase, squash-merge of the branch
  itself): the boundary commits no longer share ancestry. The result must report divergence and
  must not present a comparison between unrelated histories as the window's change set.
- **Boundary commit no longer reachable** (deleted branch, garbage-collected ref): reported as an
  unresolvable boundary with the reason, not as an empty window.
- **Provider comparison caps**: provider comparison responses impose their own limits on returned
  commits and files. When the window's change set exceeds them, the result is marked truncated
  and states what was clipped.
- **Churn invisible to boundary comparison**: a file added then removed, or edited then reverted,
  inside the window nets to nothing. The result must disclose this limitation.
- **Renames**: boundary comparison may report a rename where per-commit history shows an add plus
  a delete. The result records which representation it used.
- **Activity outside the compared reference**: work on branches, forks, or unmerged pull requests
  is not on the compared reference. The result states which reference was compared so the reader
  does not mistake it for all activity.
- **Merge-heavy windows**: merge commits appear in the commit list but contribute no unique
  change of their own to the window's net change set; counts must not double-count them.
- **Very large single changes** (vendored dependencies, generated files, lockfiles): patch volume
  must stay bounded and any bounding must be disclosed.
- **Quota already exhausted before the query starts**: the query reports this up front with the
  reset time rather than failing partway with an opaque error.
- **Window boundaries and time zones**: boundaries are normalized once and echoed in the result;
  the same wall-clock request must not resolve differently depending on the caller's environment.
- **Repository unreadable or empty**: named explicitly with a reason, in the same way a skipped
  repository is reported today.
- **Repository activity requested against GitLab**: rejected with a provider-specific reason
  naming the unsupported capability; the caller is not left to infer it from an empty result, and
  existing GitLab commit queries continue to work.
- **Default ceiling reached on a query that used to complete**: because the default applies to
  existing author-scoped queries too, a previously-complete query may now stop short. The result
  must say the default ceiling stopped it and how to raise or disable it — the stop is never
  silent.

## Requirements *(mandatory)*

### Functional Requirements

#### Repository-scoped window queries

- **FR-001**: Users MUST be able to request a repository's activity for a time window without
  supplying an author.
- **FR-002**: Users MUST be able to optionally narrow a repository window query to one author,
  without that narrowing changing the cost characteristics of the query.
- **FR-003**: Users MUST be able to specify the window as an explicit start/end pair or as a
  duration ending at the query time, using the same window syntax the existing query surface
  accepts.
- **FR-004**: The system MUST normalize window boundaries once, at the request boundary, and echo
  the normalized boundaries in the result.
- **FR-005**: Users MUST be able to name the reference (branch or tag) to examine; when omitted
  the repository's default reference is used, and the reference actually used MUST appear in the
  result.

#### Cost-bounded evidence gathering

- **FR-006**: The default repository window query MUST NOT issue per-commit detail requests. Its
  request count MUST scale with the number of commit *result pages*, not with the number of
  commits.
- **FR-007**: The system MUST derive the window's aggregate per-path change set using a number of
  requests that is independent of the number of commits in the window.
- **FR-008**: The system MUST resolve the window's start boundary to the **parent of the earliest
  in-window commit** on the compared reference — the repository state immediately before the
  window — so that every in-window commit's work appears in the change set. The end boundary is
  the latest in-window commit on that reference.
- **FR-008a**: Boundaries MUST be resolved by ancestry, not by timestamp proximity, so that
  rebases, cherry-picks, and author/committer date disagreement do not shift them. Both resolved
  boundary identifiers, and how each was resolved, MUST be reported in the result.
- **FR-008b**: When the earliest in-window commit is the repository's root commit and therefore
  has no parent, the start boundary MUST resolve to the empty repository state, and the result
  MUST report that the window opens at repository inception.
- **FR-009**: The system MUST detect when the resolved boundaries do not share ancestry and MUST
  report the divergence instead of emitting a change set derived from unrelated histories.
- **FR-010**: The system MUST support opt-in per-commit evidence for a bounded subset of the
  window's commits, MUST select that subset deterministically, and MUST state in the result both
  the size of the subset and the rule that selected it.

#### Cost visibility and budgeting

- **FR-011**: Users MUST be able to obtain an estimate of a query's provider request cost without
  issuing the query's evidence-gathering requests.
- **FR-012**: A default provider-request ceiling MUST be in force for every query — including
  existing author-scoped queries — sized so that a typical one-week query completes well within
  it (see SC-001). Users MUST be able to raise, lower, or disable it explicitly.
- **FR-012a**: The default ceiling MUST be a fixed, documented value. It MUST NOT be derived from
  remaining quota or any other run-varying state, so that FR-023 (identical request, identical
  result) holds.
- **FR-013**: When a ceiling is reached, the system MUST return the evidence gathered so far,
  MUST mark the result as bounded by budget, and MUST name what was not covered.
- **FR-014**: Every result MUST report the provider requests consumed, the ceiling in force, and
  the caller's remaining quota, including the quota reset time when the provider supplies one.
- **FR-015**: When a provider rate limit is hit mid-query, the system MUST return the evidence
  gathered so far together with an attributable error stating the limit, its reset time, and the
  next safe action — never a silent empty result and never an all-or-nothing failure.
- **FR-016**: The system MUST report a pre-existing exhausted quota before beginning evidence
  gathering, rather than discovering it partway through.

#### Honest, auditable output

- **FR-017**: Every association between a changed path and a commit MUST be labeled as observed
  (derived from retrieved per-commit file data) or inferred (derived from any other basis), and
  inferred associations MUST record the basis used. The system MUST NOT present an inferred
  association as observed.
- **FR-018**: The system MUST disclose in the result that a boundary-comparison change set cannot
  represent churn that nets to nothing within the window, and cannot represent activity outside
  the compared reference.
- **FR-019**: Every bound the system applies — commit cap, patch-size cap, provider comparison
  cap, request ceiling — MUST be disclosed in the structured result when it takes effect, not
  only on the human-readable output.
- **FR-020**: The result MUST carry enough context to be re-derived: provider, repository,
  reference, resolved boundaries, resolved window, schema version, and generation time.
- **FR-021**: The system MUST NOT generate narrative prose about the work. It emits the commit
  messages and the correlated change set as structured evidence; constructing the story is the
  consumer's job.

#### Contract, safety, and parity

- **FR-022**: All new capability MUST be read-only. No path may create, update, or delete any
  provider-side object. (Tool-level read-only annotation and auto-approval are specified in
  FR-026a.)
- **FR-023**: An identical request against identical upstream state MUST produce an identical
  result, excluding the generation timestamp.
- **FR-024**: The activity result MUST be a new public result type, distinct from the existing
  commit-query result and carrying its own schema version. The existing commit-query result MUST
  NOT change shape, and this feature MUST NOT bump its schema version. Any later change to the
  new type's shape MUST bump the new type's schema version in the same change.
- **FR-025**: The capability MUST be delivered for GitHub. GitLab is out of scope for this
  feature: a repository-activity request against GitLab MUST fail with a clear, provider-specific
  reason naming the unsupported capability, and the coverage gap MUST be stated in user-facing
  documentation. Existing GitLab commit-query behavior MUST be unaffected.
- **FR-026**: The capability MUST be reachable from the local CLI and from a new, second
  agent-facing tool, distinct from the existing commit-query tool. The existing tool's request
  and result contract MUST NOT change. The structured output is the contract; any human-readable
  rendering is a view of it, never a superset.
- **FR-026a**: The new agent-facing tool MUST be annotated read-only and MUST derive its
  auto-approval status from the existing read-only tool registry, so that every runtime's
  installer permission block gains the tool mechanically rather than by hand-editing.

### Key Entities *(include if feature involves data)*

- **Activity Result**: The top-level container returned by this feature — a new public type,
  sibling to the existing commit-query result, carrying its own schema version and generation
  time. It holds the request echo, resolved boundaries, commit records, change set, correlations,
  cost report, and disclosures below.
- **Activity Window Request**: What the caller asked for — provider, repository, reference,
  normalized window boundaries, optional author narrowing, evidence depth, request ceiling.
- **Resolved Boundaries**: The two commits that define the window's start and end state on the
  compared reference — the start being the parent of the earliest in-window commit — plus how
  each was resolved and whether they share ancestry.
- **Commit Record**: A commit in the window — identity, author, date, full message, and link.
  Reuses the existing commit contract.
- **Window Change Set**: The aggregate per-path difference between the resolved boundaries — path,
  change status, line counts, optional bounded patch text — with truncation flags.
- **Correlation**: A link between a changed path and one or more commits, carrying its
  determination basis (observed or inferred) and, for inferred links, the rule that produced it.
- **Cost Report**: Estimated requests, requests consumed, remaining quota, quota reset time, and
  the ceiling in force.
- **Disclosure**: A machine-readable statement of a limitation that applied to this result —
  budget-bounded, provider-capped, patch-truncated, ancestry-diverged, reference-scoped,
  net-comparison blind spot — each with a reason and, where one exists, a next action.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A one-week activity query against a repository with up to 500 commits in the window
  consumes no more than 15 provider requests.
- **SC-002**: For a window of 200 commits, the default repository activity query consumes at least
  90% fewer provider requests than the existing per-commit evidence path for the same window.
- **SC-003**: A developer can run at least 20 one-week activity queries within a single quota
  period without exhausting their quota.
- **SC-004**: A one-week activity query against a repository with up to 500 commits in the window
  returns in under 10 seconds on a normal connection.
- **SC-005**: The pre-run cost estimate is within ±20% of the requests actually consumed for at
  least 95% of queries.
- **SC-006**: 100% of results that were bounded, truncated, or degraded for any reason carry a
  machine-readable disclosure naming the reason; zero results present a bounded outcome as
  complete.
- **SC-007**: 100% of results in which a rate limit or budget ceiling was hit contain the evidence
  gathered before the stop plus a stated reason and next action; zero such results are empty
  without explanation.
- **SC-008**: 100% of path-to-commit associations in output carry an observed/inferred label.
- **SC-009**: Repeating an identical query against unchanged upstream state produces identical
  structured output apart from the generation timestamp, in 100% of runs.
- **SC-010**: A developer who has never used the tool can obtain a week of repository activity and
  correctly state, from the result alone, what the result does and does not cover.

## Assumptions

These are the defaults chosen where the request did not specify. Each can be reversed in
`/speckit-clarify` or `/speckit-plan`.

- **Both scopes are in scope, repository-scoped first.** The request describes a repository
  question but names the author-scoped weekly query as where the rate-limit pain appears. The
  cost-control mechanism is therefore specified as general (Stories 2 and 4), with the new
  repository-scoped question as the P1 delivery.
- **Boundary comparison is the default fidelity; per-commit detail stays opt-in and bounded.**
  This is the cheap path the request describes. It cannot attribute paths to individual commits
  and cannot see churn that nets to nothing — both are specified as mandatory disclosures rather
  than hidden. The comparison runs from the parent of the earliest in-window commit (FR-008), so
  the first commit in the window is included rather than absorbed into the baseline.
- **The activity result is a new type, not an extension of the existing one** (clarified
  2026-07-25). This keeps the change purely additive for downstream consumers that pin the
  existing commit-query schema version, at the cost of a second public result type to maintain.
- **sting emits evidence, not prose.** "Build a story" is satisfied by emitting the commit
  messages correlated with the change set in a form an agent can narrate from. Generating the
  narrative inside sting would be non-deterministic and outside its stated scope, so FR-021
  excludes it.
- **The compared reference defaults to the repository's default branch.** Branch, fork, and
  unmerged pull-request activity are outside the compared reference and are disclosed as such
  rather than silently merged in.
- **GitHub only for this feature** (clarified 2026-07-25). GitLab repository-activity requests
  fail with an explicit provider-specific reason and the gap is documented. This is a deliberate,
  stated coverage difference rather than an implied parity, which is what Principle VIII requires.
- **Existing query behavior is preserved except for the default ceiling** (clarified 2026-07-25).
  Author-scoped queries keep their existing shape, output contract, and schema version. The one
  behavior change is that the default request ceiling (FR-012) now applies to them, so an
  unusually large query that previously ran to completion may now stop and return a labeled
  partial result. This is the intended effect — it is the failure mode the feature exists to
  prevent — and it is disclosed rather than silent. No existing default becomes more expensive.
- **The existing window syntax, configuration precedence, and credential model are reused
  unchanged.** This feature introduces no new authentication path and reads no ambient provider
  tokens.
- **Response caching and conditional revalidation are out of scope for this specification.** They
  are a legitimate way to further reduce quota consumption and may be considered during planning,
  but no requirement here depends on them.

## Out of Scope

- Any write, mutation, or approval action against a provider.
- Code review, quality scoring, or judgement about the changes found.
- Prose narrative generation, summarization, or any model-generated content inside the tool.
- Metrics, dashboards, trend lines, or productivity measurement over time.
- Persisting query results as a durable record; provider state remains authoritative.
- Organization-wide or cross-repository sweeps as a new capability. Existing org and multi-repo
  scopes continue to work and inherit the cost controls, but no new fan-out surface is added.
- GitLab support for repository activity. Existing GitLab commit queries are unaffected; the
  new capability rejects GitLab explicitly (FR-025) and is left to a follow-up feature.
- Changing the existing commit-query tool or its result contract. This feature adds a second
  tool and a sibling result type; it modifies neither existing contract.
- Local Git repository analysis. This feature queries providers.

## Governing Decisions

This specification is constrained by, and does not contradict, the following accepted records:

- **Constitution I (Read-Only by Design)** → FR-022, FR-026a.
- **Constitution II (Evidence-Grade, Explainable Output)** → FR-014, FR-015, FR-017, FR-020.
- **Constitution III (Deterministic, Reconstructible Queries)** → FR-004, FR-008, FR-008a,
  FR-010, FR-012a, FR-019, FR-023.
- **Constitution V (Compose, Don't Trap)** and **ADR 0004 (Public packages and Wake evidence
  shape)** → FR-024, FR-026. The sibling-type decision keeps this feature additive for Wake,
  which pins the existing commit-query schema version; no downstream coordination is required.
- **Constitution VI (Partial Results Over Blindness)** → FR-013, FR-015.
- **Constitution VIII (Technical Precision, Honest Scope)** → FR-018, FR-021, FR-025.
- **ADR 0007 (Commit file and diff evidence)** established that evidence depth is opt-in because
  it costs extra provider calls. This feature does not reverse that decision — it adds a
  bounded-cost path to the same evidence and makes the cost visible. No supersession is proposed.
- **ADR 0003 (Multi-runtime installer and read-only safety)** → the second tool derives its
  auto-approval status from the existing read-only tool registry (FR-026a). The read-only safety
  model is unchanged; only the number of tools it covers grows.

- **ADR 0001 (Deliver MCP server and CLI from one binary)** — **a new ADR is required.** ADR 0001
  describes a single `get_commits` MCP tool. FR-026 adds a second tool, so this feature MUST land
  a new decision record covering the move from a single-tool to a multi-tool server and the
  read-only invariant that continues to govern it. ADRs are immutable, so ADR 0001 is not
  edited — the new record supersedes only its single-tool framing. Producing that ADR is in scope
  for `/speckit-plan`.
