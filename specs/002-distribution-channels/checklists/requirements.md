# Specification Quality Checklist: Distribution Channel Conformance

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

## Notes

Two items pass with a stated qualification rather than silently. Recording them here rather
than leaving the reader to infer them, per constitution Principle VIII.

1. **"No implementation details" — named technologies appear, and are requirements rather than
   choices.** This feature's deliverable *is* a set of named distribution channels. `.deb`,
   `.rpm`, the MCP registry, and a container registry are mandated by name in
   `skaphos-resources` DECISIONS/0001; naming them is stating the requirement, not preempting
   the design. Where a genuine design choice exists, the spec stays outcome-level: it says
   "verify the publisher signature over the release's checksum manifest" rather than naming a
   signing tool, "atomically replaced" rather than naming a replacement strategy, and "the
   build's own recorded metadata" rather than naming an API. Two references to concrete current
   state — `internal/cli/version.go:14` and `go install ...@latest` — appear only in the Problem
   section, as evidence of the defect being fixed.

2. **Stakeholder audience is the maintainer and the tool's users, both technical.** sting is a
   developer CLI and an MCP server; there is no non-technical consumer of a packaging decision.
   The spec is written to be readable without knowing sting's code, which is the reachable form
   of that criterion here.

3. **`SC-002` and `SC-008` name a distribution family and the absence of a language toolchain.**
   Both describe the *user's environment*, which is what makes the criterion verifiable, not an
   implementation of sting.

No [NEEDS CLARIFICATION] markers were needed. The one question with material scope impact —
whether `winget` and `scoop` are in this feature — is resolved in the Assumptions section from
the issue's own follow-up comment, which recommends sequencing Windows Authenticode signing
ahead of those channels. That resolution is recorded as a scope decision, not a silent drop:
both channels are *recommended* rather than required under ADR-0001, and both are listed in
Out of Scope with the reason.

Validation run: 1 iteration, all items pass.

## Re-validation after clarification session 2026-07-25

Five clarifications were accepted and integrated. All 16 items re-evaluated: **16/16 → 16/16**,
no state changes, no regressions.

The clarifications made the spec materially more concrete, which is worth recording against
item 1 above rather than letting the qualification quietly widen. Newly named in the spec are
`io.skaphos/sting` and its DNS TXT ownership proof, the pinned signer identity (sting's release
workflow and the GitHub Actions OIDC issuer), Authenticode as the gate on Windows self-update,
and rename-then-replace as the mechanism where a running executable cannot be overwritten. Each
is a *decision that was asked and answered*, not a design the spec is preempting — the namespace
and the trust anchor are externally visible identities that cannot be deferred to planning, and
the Windows mechanism is the only one the platform permits. Item 1 therefore still passes under
its stated qualification, with a wider surface than at first pass.

Item "Success criteria are technology-agnostic" likewise still passes: SC-012, SC-013 and SC-014
were added, and each describes the user's environment or an observable outcome — a machine with
no extra tooling, a rejected signature, a binary that was not replaced — rather than how sting
achieves it.

Two factual corrections were made during integration, unrelated to any clarification: the
Problem section's channel count (now stated precisely as two channels today and six after this
feature) and SC-009, which had claimed the container image would appear in the release checksum
manifest. Images are not checksummed into that manifest; SC-009 now states the provenance
appropriate to each artifact form.
