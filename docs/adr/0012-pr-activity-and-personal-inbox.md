# 12. PR lifecycle activity and personal inbox are separate evidence workflows

Date: 2026-09-05

## Status

Proposed. Design accompanies [issue #127](https://github.com/skaphos/sting/issues/127) and its
[clarified specification](../../specs/003-pull-request-query/spec.md); the feature branch implements this decision, which remains proposed pending review.

## Context

PR activity asks which PRs were opened, merged, or closed during a window. A personal inbox
asks which currently open PRs the user authored, is assigned to, or is requested to review.
Current state alone cannot answer the first question, and an activity window would hide older
work from the second. A PR closed during the window and reopened later must retain its earlier
closure as evidence.

During clarification, the user authorized additional lifecycle-history requests within the
request ceiling and expanded scope to include all three inbox relationships. This replaces the
issue's initial no-per-PR-request proposal for lifecycle collection only.

## Decision

Extend [ADR 0010](0010-multi-tool-mcp-server.md)'s separate-tool pattern:

- `sting prs` / `get_prs` return historical PR activity. Default action selection includes
  opening, merging and unmerged closure, with no current-state filter. General latest-update
  matching is explicit; remaining open never qualifies by itself.
- `sting inbox` / `get_pr_inbox` return current open work with the union of authored, assigned,
  and direct pending-review reasons, without a time cutoff. Each PR appears once with all
  confirmed reasons. Team-only requests are not inferred to be personal assignments.
- Give the two workflows independent versioned public query/result envelopes with shared PR
  metadata, following [ADR 0004](0004-public-packages-and-wake-evidence.md). Existing commit and
  repository-activity contracts and `include_prs` behavior remain unchanged.
- Use GET-only paginated discovery and per-PR lifecycle history as needed. All calls, including
  self identity and inbox verification, consume one isolated per-query budget. No optional
  per-PR diff, reviewer-detail, stats, or check enrichment is added.
- Preserve confirmed action occurrences independently of current-record timestamps. Disclose
  ambiguous or unavailable history and incomplete discovery/reason evaluation separately from
  unavailable optional metadata. A partial result must never claim complete matching evidence.
- Both new MCP tools use the existing single definition source and read-only annotations;
  installer permissions remain derived and format-preserving. Neither workflow mutates PRs.
- MCP startup validates typed shared settings; each tool validates its workflow before provider
  access. Legacy workflow configuration checks move to their existing tool handlers, so invalid
  activity defaults cannot block the inbox. Standalone CLI validation remains unchanged.
- For the inbox, repository verification overrides search evidence. Remove disproven reasons
  and candidates absent from complete open-PR listings, while disclosing disagreement without
  inventing a completion state. Interrupted verification retains uncontradicted proof. Keep
  only per-query verification state; later search hits cannot restore contradicted matches.

## Consequences

Historical queries may scan a broad candidate set and spend most of their budget recovering
history. A narrow window does not imply cheap search or freedom from search's candidate cap.
Inbox reviewer search may require repository-level verification to distinguish direct requests.
These costs and limitations are explicit in structured output.

An invalid workflow-specific MCP default now fails the affected tool call instead of server
startup; syntax/type errors and invalid shared settings still fail startup. This isolates tools
without weakening their query validation. Inbox search/list disagreement can remove initially
retained matches; its attributable coverage gap remains even when no entries survive.

Callers can choose the workflow that matches their question without interpreting a mode-dependent
schema. Independent versions avoid forcing commit consumers to migrate. There is no persistent
PR cache, consumer dependency, or constitutional deviation.

## Alternatives

- Overload `include_prs` or `get_commits`: violates their commit evidence meaning.
- Reuse one result with a mode: makes historical occurrences and current relationships ambiguous.
- Filter only latest closure/update dates: loses historical actions after later state changes.
- Fetch full PR details/review bodies by default: adds unrelated cost and scope.
- Infer all reviewer-search hits as direct requests: can turn team candidates into false personal
  evidence. Verify direct relationships through paginated PR metadata instead.

Provider findings and exact design choices are recorded in
[research](../../specs/003-pull-request-query/research.md) and
[collection contract](../../specs/003-pull-request-query/contracts/provider-collection.md).
Existing accepted ADRs remain unchanged.
