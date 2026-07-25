# Specification Quality Checklist: Repository Activity Digest

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-07-25
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Constitution Alignment

- [x] Read-only invariant preserved (Principle I) — FR-022, FR-026a
- [x] Output is evidence-grade and re-derivable (Principle II) — FR-014, FR-015, FR-017, FR-020
- [x] Queries are deterministic; bounds are explicit and disclosed (Principle III) — FR-004,
      FR-008, FR-008a, FR-010, FR-012a, FR-019, FR-023
- [x] Public contract changes carry a schema version bump (Principle V) — FR-024
- [x] Partial results over blindness (Principle VI) — FR-013, FR-015
- [x] Honest scope; provider coverage differences documented (Principle VIII) — FR-018, FR-021,
      FR-025
- [x] Cites accepted ADRs rather than re-deriving; proposes no silent supersession — see
      "Governing Decisions"

## Validation Notes

**Iteration 1 (2026-07-25)** — three items failed on first pass and were corrected:

1. *Requirements are testable and unambiguous* — the original correlation requirement asked the
   system to "associate each delta with the commits that produced it", which the cheap path
   physically cannot do: a boundary comparison yields the set of changed paths but not which
   commit changed which path. Corrected to FR-017, which requires every association to be labeled
   observed or inferred and forbids presenting inference as observation.
2. *Scope is clearly bounded* — "build a story" was ambiguous between emitting correlated evidence
   and generating prose. Resolved by FR-021 (no narrative generation inside the tool) plus an
   explicit Out of Scope entry, grounded in Principle III (determinism) and Principle VIII
   (honest scope).
3. *Success criteria are measurable* — initial cost criteria were stated as "fewer API calls".
   Replaced with SC-001 through SC-005, which bound request counts, query repetition within a
   quota period, latency, and estimate accuracy.

**Terminology note**: "provider request", "quota", "reference", and "boundary comparison" are
domain vocabulary for a developer tool that queries source-control providers, not implementation
choices. No specific API, endpoint, library, or language is named anywhere in the spec.

**Iteration 2 (2026-07-25, `/speckit-clarify`)** — no checkbox changed state; all 23 items passed
before and after. Five ambiguities that would have surfaced as rework during planning were
resolved with the user and recorded in the spec's Clarifications section:

1. *Default request ceiling* — the spec previously specified only an opt-in ceiling, leaving the
   default path able to exhaust the quota, which is the failure the feature exists to prevent.
   Now FR-012 mandates a fixed, documented, overridable default; FR-012a forbids deriving it from
   remaining quota, which would have broken FR-023.
2. *Agent-facing surface* — FR-026 now specifies a second, distinct read-only tool rather than
   leaving "reachable from the agent-facing tool surface" open. This surfaced a governance
   obligation the spec had missed: ADR 0001 frames the server as single-tool, so a new ADR is
   required. Recorded under Governing Decisions.
3. *Result contract* — FR-024 now specifies a new sibling result type. This changed the
   Principle V analysis: the feature is purely additive, the existing schema version is not
   bumped, and no downstream coordination with Wake is needed.
4. *Window base commit* — FR-008 now anchors the comparison at the parent of the earliest
   in-window commit. This closed a silent off-by-one: anchoring at the first in-window commit
   would have absorbed that commit's own work into the baseline and under-reported every window.
   FR-008a fixes resolution by ancestry rather than timestamp, and FR-008b covers the root-commit
   case.
5. *GitLab scope* — FR-025 previously permitted either delivering GitLab or failing cleanly,
   which left task decomposition undecided. Now GitHub-only with an explicit provider-specific
   rejection and a documented gap.

**Behavior-change note**: the default ceiling applies to existing author-scoped queries, so an
unusually large query that previously ran to completion may now return a labeled partial result.
This is intended and disclosed (Edge Cases, Assumptions) rather than silent.

**Open decisions**: none blocking. Remaining choices are plan-level — the concrete default
ceiling value, the deterministic rule selecting which commits receive opt-in per-commit detail
(FR-010), and whether conditional revalidation is used to reduce quota consumption (currently out
of scope).
