<!--
Sync Impact Report
- Version change: (template, unversioned) → 1.0.0
- Source: derived from skaphos-resources/standards/constitution.md v1.1.0 (2026-07-23)
- Modified principles: all template placeholders replaced with concrete sting principles
  - [PRINCIPLE_1_NAME] → I. Read-Only by Design (NON-NEGOTIABLE)
  - [PRINCIPLE_2_NAME] → II. Evidence-Grade, Explainable Output
  - [PRINCIPLE_3_NAME] → III. Deterministic, Reconstructible Queries
  - [PRINCIPLE_4_NAME] → IV. Explicit Configuration, Dedicated Credentials
  - [PRINCIPLE_5_NAME] → V. Compose, Don't Trap: the Public Packages Are the Contract
  - (added) VI. Partial Results Over Blindness
  - (added) VII. sting Is Never a Second Source of Truth
  - (added) VIII. Technical Precision, Honest Scope
- Added sections: Upstream Principle Applicability; Engineering Constraints;
  Development Workflow and Quality Gates
- Removed sections: none ([SECTION_2]/[SECTION_3] template slots filled)
- Templates requiring updates:
  - ✅ .specify/templates/plan-template.md (Constitution Check gate table filled)
  - ✅ .specify/templates/tasks-template.md (tests changed from OPTIONAL to REQUIRED; coverage-gate
       task added — the stock template contradicted the testing non-negotiables)
  - ✅ .claude/skills/speckit-tasks/SKILL.md (same "tests are optional" contradiction, overridden)
  - ✅ .specify/templates/spec-template.md (reviewed; no constitution-driven change needed)
  - ✅ .specify/templates/checklist-template.md (reviewed; no constitution-driven change needed)
  - ✅ Remaining .claude/skills/speckit-*/SKILL.md (reviewed; agent-generic, no stale references)
  - ✅ AGENTS.md / README.md (reviewed; consistent with the principles below)
- Follow-up TODOs: none
-->

# sting Constitution

sting queries a GitHub or GitLab user's commits over a time window, as a local CLI or as an
MCP server exposing a single read-only `get_commits` tool. It is an *evidence source*: its
output is consumed by humans and by other Skaphos tools (notably Wake) as fact about what
happened in a repository.

This constitution is derived from the Skaphos constitution
(`skaphos-resources/standards/constitution.md`, v1.1.0). It adds sting-specific principles
and constraints; it does not weaken or contradict anything upstream.

## Core Principles

### I. Read-Only by Design (NON-NEGOTIABLE)

sting MUST NOT mutate remote state. No command, MCP tool, or code path may create, update,
delete, or otherwise write to repositories, issues, pull requests, or any other provider-side
object. Every exposed MCP tool MUST be annotated `ReadOnlyHint: true`, and
`mcpserver.ReadOnlyTools()` MUST remain the single source of truth from which the installer's
auto-approve list derives. A change that adds a non-read-only surface is not a feature
request — it is a constitutional amendment.

*Rationale: sting's value as evidence depends on it being incapable of altering what it
reports on. Auto-approval in MCP runtimes is only safe while the read-only invariant holds
mechanically rather than by convention.*

### II. Evidence-Grade, Explainable Output

Every result MUST carry enough context to be re-derived and audited: provider, scope, the
resolved query (user, window boundaries, repositories or organization), and the result schema
version. Failures MUST state the reason and, where one exists, the next safe action —
"failed" with no reason is a defect. Structured (JSON) output is the contract; the
human-readable rendering is a view of it, never a superset.

*Rationale: sting is consumed as an audit and evidence input. Output that cannot be
reproduced or explained is not evidence.*

### III. Deterministic, Reconstructible Queries

The same request against the same upstream state MUST produce the same result. Window and
time parsing MUST be explicit and normalized once at the boundary (`config.Resolve`:
request → validated `model.Query`); no implicit local-timezone or "now"-dependent behavior
leaks into client code. Bounds and truncation MUST be explicit and disclosed in the output
rather than silently applied.

*Rationale: determinism is what makes a commit query citable. A silently bounded result that
reads as complete is a correctness bug, not a performance tradeoff.*

### IV. Explicit Configuration, Dedicated Credentials

Configuration loads at startup into typed, validated structs with explicit precedence
(flags > env > file > defaults); no scattered `os.Getenv`. sting MUST authenticate with its
own PAT keys (`token` / `STING_TOKEN` for GitHub, `gitlab_token` / `STING_GITLAB_TOKEN` for
GitLab) and MUST NOT read ambient provider tokens such as `GITHUB_TOKEN` or `GITLAB_TOKEN`.
Secrets MUST never be logged, rendered, or included in error messages.

*Rationale: an evidence tool that silently borrows whatever credential is in the environment
cannot be reasoned about, scoped, or revoked independently.*

### V. Compose, Don't Trap: the Public Packages Are the Contract

`model`, `config`, `ghclient`, and `gitlabclient` are a deliberate, minimal public API
(see ADR 0004) — the evidence contract other tools import. Breaking changes to `Commit` or
`Result` MUST bump `model.SchemaVersion` in the same change. sting MUST remain independently
useful standalone and MUST NOT take a dependency on the control planes or consumers that read
its output. Internal layering stays inward-only: `model` imports nothing internal; domain code
never imports transport packages.

*Rationale: sting is a primitive. A primitive that depends on its consumers stops being
adoptable and starts being a trap.*

### VI. Partial Results Over Blindness

When one scope, repository, or provider call fails, sting MUST return what it could gather
alongside an explicit, attributable error for what it could not — never a silent empty result
and never an all-or-nothing failure where partial evidence was available. Degradation is
reported in the structured output, not only on stderr.

*Rationale: the upstream principle is read-only degradation over blindness. For a query tool
that means partial, labeled results beat a bare error the caller cannot act on.*

### VII. sting Is Never a Second Source of Truth

Provider state is authoritative; sting persists nothing that could diverge from it beyond its
own configuration and credentials. Files sting writes on behalf of other runtimes (MCP install
targets) MUST be written atomically, MUST preserve keys and formatting sting does not own, and
MUST touch only sting's own entry. Whole-file rewrites of another tool's configuration are
forbidden.

*Rationale: a tool that quietly becomes an authority — or that clobbers a config it does not
own — breaks the durable-state boundary the suite depends on.*

### VIII. Technical Precision, Honest Scope

Documentation and specifications MUST describe verified behavior, not intent, and MUST state
plainly what sting is not: it is not a code-review tool, not a metrics platform, not a mutation
surface, and not a general provider API client. Provider coverage differences (GitHub search
scope vs. GitLab REST scopes) MUST be documented rather than papered over. Marketing language
and exaggerated claims are forbidden in repository content.

*Rationale: operational credibility is the product; a tool that overclaims is worse than one
that does less.*

## Upstream Principle Applicability

The Skaphos constitution's principles bind sting except where scope makes them inapplicable.
This is recorded explicitly so that omission is never read as silent divergence:

- Upstream I (Explicit State Over Implicit Behavior) → Principles III and IV.
- Upstream II (Git Is the Durable Desired-State Boundary) → Principle VII; sting reads Git
  history and declares no desired state of its own.
- Upstream III (Deterministic, Reconstructible Operation) → Principle III.
- Upstream IV (Kubernetes-Native, Never Obscured) → **not applicable**: sting neither serves
  CRDs nor reconciles cluster state. If sting ever grows a controller or CRD surface, this
  exemption lapses and the upstream principle applies verbatim.
- Upstream V (Compose, Don't Trap) → Principle V.
- Upstream VI (Explainable Reconciliation, Evidence-Grade Audit) → Principle II.
- Upstream VII (Read-Only Degradation Over Blindness) → Principles I and VI.
- Upstream VIII (Topology Is Deployment State) → **not applicable**: sting models no
  deployment topology. The same lapse condition as Upstream IV applies.
- Upstream IX (Technical Precision, Honest Scope) → Principle VIII.

## Engineering Constraints

- **Stack**: Go (version per `go.mod`), Cobra for the CLI, viper for configuration, and the
  standard library `testing` package for tests. External dependencies are minimized and
  justified.
- **Go engineering**: per `skaphos-resources/standards/go-engineering-standard.md` — KISS and
  YAGNI, errors wrapped with operation context and branched with `errors.Is`/`errors.As`,
  `ctx` as the first parameter and propagated end-to-end, interfaces defined where they are
  consumed, no catch-all package names, and a thin `cmd/sting/main.go`.
- **Testing (non-negotiable)**: new behavior ships with direct test coverage in the same
  change, and every bugfix ships a regression test. Tests MUST NOT touch the network — use
  `net/http/httptest` — and MUST isolate `HOME`/`USERPROFILE` when touching the filesystem.
  CI enforces a per-package coverage gate of 80% by default, with documented lower floors for
  packages with heavy interactive or external-integration surface (`internal/cli` 60%,
  `internal/credentials` 72%; see `scripts/check-coverage.sh`). Lowering a floor requires the
  same justification as any other deviation.
- **CI gates**: `lint` (golangci-lint, gofmt/goimports), `staticcheck`, `vuln` (govulncheck),
  race-enabled tests, and the coverage gate. `go -C tools tool task ci` runs the local
  equivalent and is expected to pass before pushing.
- **Documentation**: per `skaphos-resources/standards/documentation-standard.md`. `README.md`
  is updated for user-visible behavior changes; ADRs live under `docs/adr/` and are immutable —
  superseded, never rewritten. `third_party_licenses/` is regenerated
  (`go -C tools tool task notices`) whenever `go.mod`/`go.sum` changes.
- **Repository governance**: per `skaphos-resources/standards/repository-governance.md`. All
  changes land via pull request; never commit directly to `main`. Commits are cryptographically
  signed **and** DCO signed off (`git commit -S -s`). Conventional Commits gate the
  release-please version bump (see ADR 0005). Branch names follow the change type
  (`feature/`, `bug/`, `chore/`, `docs/`, `ci/`, `refactor/`).

## Development Workflow and Quality Gates

- Feature work follows the spec-driven flow — `/speckit-specify` → `/speckit-plan` →
  `/speckit-tasks` → `/speckit-implement` — with each artifact checked against this
  constitution.
- Specs MUST cite existing ADRs and `skaphos-resources` findings rather than re-deriving
  settled questions, and MUST NOT contradict an accepted ADR without proposing its
  supersession.
- **Adopt before build**: where `skaphos-resources/tools/ECOSYSTEM.md` records mature prior
  art, a plan that builds instead of adopting MUST document why the verdict does not apply.
- The plan's Constitution Check gate MUST be evaluated before Phase 0 research and re-checked
  after Phase 1 design. A failed gate blocks implementation until the design changes or the
  deviation is justified in the plan's Complexity Tracking table.
- Pull requests include: summary, why, testing performed (commands and results), and doc
  updates when behavior changes.

## Governance

This constitution derives from the Skaphos constitution and is subordinate to it. It MAY add
sting-specific principles and constraints; it MUST NOT weaken or contradict the upstream
document. When the upstream changes, this file is re-synced — propose upstream first, mirror
here second.

**Amendment**: amendments land by pull request against this file, with the rationale in the PR
description. Version semantics: MAJOR for removing or redefining a principle, MINOR for adding
a principle or section, PATCH for clarifications that change no requirement. An amendment that
would relax Principle I (read-only) requires an ADR in addition to a version bump.

**Compliance**: specs, plans, and pull request review are gated against this constitution. A
deviation is either (a) justified in writing in the plan's Complexity Tracking table, or (b) a
proposed amendment — silent divergence is not an option. `AGENTS.md` remains the runtime
development guidance for day-to-day work; where it and this document disagree, this document is
authoritative and `AGENTS.md` gets fixed.

**Version**: 1.0.0 | **Ratified**: 2026-07-25 | **Last Amended**: 2026-07-25
