# Implementation Plan: Distribution Channel Conformance

**Branch**: `feature/distribution-channels` | **Date**: 2026-07-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-distribution-channels/spec.md`

> **Status update — self-update was dropped after implementation.** This plan was written
> before `sting update` was built and measured. It was built, it worked, and it was then
> removed: verifying in-process cost +122% binary size and 62 extra modules to serve one
> command. See [ADR 0011](../../docs/adr/0011-no-self-update-subcommand.md).
>
> The plan below is kept as the record of what was planned and why. Everything it says about
> `internal/selfupdate`, `sting update`, and the `sigstore-go` dependency describes work that
> was done and then reverted — those files do not exist, and `go.mod` is byte-identical to
> `main`. The other four obligations shipped as planned.

## Summary

Adopt `skaphos-resources` DECISIONS/0001 for sting, which is both a Shape 2 CLI and a Shape 3 MCP
server and so inherits both channel sets. Five required obligations are unmet: version-stamped
source installs, a self-update subcommand, Linux `.deb`/`.rpm`, an MCP registry entry, and a
multi-arch container image.

The technical approach is deliberately lopsided. Four of the five are **configuration** — an
`nfpms` block, a `dockers_v2` block, a checked-in `server.json`, and release-workflow wiring — and
carry almost no Go code. The fifth, `sting update`, is the only real engineering: it needs to
learn its own version, work out who owns the binary on disk, verify a Sigstore bundle in-process
against a pinned signer identity, and replace itself atomically. That asymmetry drives the whole
plan: two new internal packages (`buildinfo`, `selfupdate`) plus one thin Cobra command, against a
larger surface of release-pipeline configuration.

Alongside the additions, one **change to existing behavior**: `release.yml` currently drops the
Homebrew cask with a `::warning::` when the app token cannot reach the tap. FR-038 makes that a
hard failure. It is the single change most directly aimed at the silent-staleness failure
ADR-0001 and repokeeper ADR-0007 both document.

## Technical Context

**Language/Version**: Go 1.26.5 (per `go.mod`)

**Primary Dependencies**: existing only — Cobra, viper, `modelcontextprotocol/go-sdk` v1.6.1,
`google/go-github/v84`. **No dependency is added**: `go.mod` and `go.sum` are byte-identical to
`main`. `sigstore-go` was added for in-process signature verification and then removed with the
self-update command it existed to serve (ADR 0011).

**Storage**: N/A. The update command writes only sting's own binary; no persisted state is added.

**Testing**: standard library `testing`, `net/http/httptest` for the release-fetch path, `t.TempDir`
plus isolated `HOME`/`USERPROFILE` for filesystem paths. No network in tests.

**Target Platform**: linux, darwin, windows × amd64, arm64 (unchanged). Self-replacement is
enabled on linux and darwin; gated off on windows pending Authenticode signing (FR-006).

**Project Type**: single Go module — CLI plus MCP server from one binary (ADR 0001).

**Performance Goals**: N/A. `sting update` is user-initiated and network-bound; no throughput or
latency target applies.

**Constraints**: verification is mandatory and unskippable (FR-012) and must require no tooling on
the user's machine (FR-009); no command other than `update` may contact a release endpoint
(FR-007); the update path must not read ambient provider tokens (Principle IV).

**Scale/Scope**: ~2 new internal packages, 1 new CLI command, 1 new ADR, 4 release-pipeline
config surfaces (`.goreleaser.yaml`, `Dockerfile`, `server.json`, `release.yml`), README and docs
updates. 43 functional requirements, 14 success criteria.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Derived from `.specify/memory/constitution.md` v1.0.0.

| # | Gate | Check | Status |
|---|------|-------|--------|
| I | Read-only by design | No command, MCP tool, or code path writes to provider state; any new MCP tool is `ReadOnlyHint: true` and registered via `mcpserver.ReadOnlyTools()` | **PASS** — no new MCP tool; `update` writes only sting's own binary on local disk and reads public release assets. FR-042 states this as a requirement. |
| II | Evidence-grade output | Results carry provider, scope, resolved query, and schema version; every failure states a reason and a next safe action | **PASS** — `update` reports resolved current/target version, install ownership, and which verification step failed (FR-004, FR-011, FR-018); package-manager refusal prints the correct command as the next safe action. |
| III | Deterministic queries | Same request + same upstream state → same result; time/window parsing normalized at the boundary; bounds and truncation disclosed | **PASS** — update behavior is a function of the resolved release contents, the pinned signer identity, and the install location. No "now"-dependence beyond which tag is latest, and an explicit target version removes even that (FR-016). |
| IV | Explicit config, dedicated credentials | Typed/validated config with flags > env > file > defaults; sting's own PAT keys only (no `GITHUB_TOKEN`/`GITLAB_TOKEN`); no secrets in logs, output, or errors | **PASS** — `update` needs no credentials at all (public release assets) and MUST NOT borrow ambient provider tokens. FR-019 additionally forbids transmitting identifying information. |
| V | Public packages are the contract | Public API surface stays minimal; breaking `Commit`/`Result` changes bump `model.SchemaVersion`; no dependency on consumers; inward-only layering | **PASS** — `buildinfo` and `selfupdate` are `internal/`. No change to `model`, `config`, `ghclient`, `gitlabclient`; no `SchemaVersion` bump. |
| VI | Partial results over blindness | Per-scope/per-repo failures return partial results with an attributable error | **N/A** — no query surface is added. The analogous obligation for this feature is that a failed channel is attributable rather than silent, which FR-039/FR-040 cover at the release level. |
| VII | Never a second source of truth | Nothing persisted that can diverge from provider state; files written for other runtimes are atomic, format-preserving, and touch only sting's entry | **PASS** — atomic replacement (FR-013); files owned by another installer are never overwritten (FR-008); no install-provenance marker file is written (see research §5). |
| VIII | Technical precision, honest scope | Docs describe verified behavior and state limits plainly; no marketing language | **PASS** — FR-025 requires stating there is no hosted apt/yum repo; the Windows gate and unsigned Windows binaries are documented, not elided (FR-006, FR-043). |
| — | Testing non-negotiables | New behavior has direct tests in the same change; bugfixes ship a regression test; no network in tests; `HOME`/`USERPROFILE` isolated; per-package coverage gate holds (80% default) | **PASS with design constraint** — `selfupdate` must be built around injectable seams (HTTP client, filesystem root, executable-path resolver, command runner) to reach the 80% floor without network. `internal/cli` keeps its documented 60% floor; `internal/buildinfo` and `internal/selfupdate` are new packages and take the 80% default. |
| — | Governance | PR-only, signed + DCO (`git commit -S -s`), Conventional Commits; ADR for architecturally significant decisions; `README.md` updated for user-visible changes | **PASS** — work lands on `feature/distribution-channels` via PR #122. A new ADR records the self-update trust model (pinned identity, verify-before-replace, defer-to-package-manager); README gains the new install and upgrade paths. |

**Gate result: PASS.** One dependency addition requires justification — recorded in Complexity
Tracking. Re-checked after Phase 1 design: unchanged, see [Post-design re-check](#post-design-constitution-re-check).

## Project Structure

### Documentation (this feature)

```text
specs/002-distribution-channels/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── cli-update.md
│   ├── server-json.md
│   └── release-artifacts.md
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── buildinfo/                 # NEW — version identity resolution (FR-001..FR-005)
│   ├── buildinfo.go           #   ldflags > debug.ReadBuildInfo > unknown
│   └── buildinfo_test.go
└── cli/
    └── version.go             # CHANGED — resolve through internal/buildinfo

# Planned and then reverted (ADR 0011): internal/selfupdate/*, internal/cli/update.go

server.json                    # NEW — MCP registry entry, io.skaphos/sting
Dockerfile                     # NEW — copies the GoReleaser-built binary; non-root; stdio

.goreleaser.yaml               # CHANGED — nfpms, dockers_v2, sboms for packages
.github/workflows/
├── release.yml                # CHANGED — buildx+binfmt+GHCR login, mcp-publisher,
│                              #           post-release verify job, hard-fail on
│                              #           unusable Homebrew credential (FR-038)
└── ci.yml                     # CHANGED — validate server.json against its schema (FR-027)

docs/adr/0011-no-self-update-subcommand.md # NEW — deviation record
README.md                                   # CHANGED — install + upgrade paths per channel
```

**Structure Decision**: The existing single-module layout is kept unchanged. Two new `internal/`
packages are added rather than extending `internal/cli`, for two reasons. First, `internal/cli`
carries a documented 60% coverage floor because of its interactive surface; update logic that is
genuinely unit-testable should not inherit that floor, and putting it in its own package holds it
to the 80% default. Second, Principle V's inward-only layering — `selfupdate` is domain logic and
must not be reachable from, or entangled with, the Cobra command shell beyond a thin call.
`internal/cli/update.go` stays a flag-parsing wrapper.

## Phase 0: Research

Complete — see [research.md](./research.md). Five unknowns resolved: the `debug.ReadBuildInfo()`
fallback matrix; the `sigstore-go` verification API and the exact identity to pin; `dockers_v2` as
the supported multi-arch path in GoReleaser v2.17; the MCP registry's `server.json` shape and the
`io.skaphos` DNS proof; and install-ownership detection ordering. It also records what the new
artifacts inherit from the existing supply chain versus what needs wiring, and the design of the
post-release verification job.

No NEEDS CLARIFICATION markers remain.

## Phase 1: Design & Contracts

Complete. Artifacts:

- [data-model.md](./data-model.md) — the four entities from the spec, as concrete types with
  fields, validation rules, and the ownership-classification state transitions.
- [contracts/cli-update.md](./contracts/cli-update.md) — `sting update` flags, output shape, and
  the exit-code contract per outcome.
- [contracts/server-json.md](./contracts/server-json.md) — the registry entry, field by field.
- [contracts/release-artifacts.md](./contracts/release-artifacts.md) — the exact artifact set a
  release must produce, per channel, and what verification asserts about each.
- [quickstart.md](./quickstart.md) — runnable validation of every user story.

### Implementation sequencing

The spec's story priorities and the issue's own suggested order agree, and the dependency edge is
real: self-update cannot work until the binary knows its version.

1. **US1 — version identity** (`internal/buildinfo`, `internal/cli/version.go`). *Shipped.*
2. **US2 — `sting update`** — *built, measured, and dropped; see ADR 0011.*
3. **US3 — Linux packages** (`nfpms`, SBOM coverage, README). Independent of 1 and 2.
4. **US4 — MCP registry** (`server.json`, CI schema validation, `mcp-publisher` in release).
5. **US5 — container image** (`Dockerfile`, `dockers_v2`, buildx wiring).
6. **Cross-cutting** — post-release verification job and the FR-038 hard-fail change. Sequenced
   last because it asserts on the artifacts every prior step produces.

Steps 3–5 are mutually independent and independent of 1–2; only 1→2 and (3,4,5)→6 are ordered.

### Post-design Constitution re-check

Re-evaluated after the design above. **No gate changes status.** Two points worth recording:

- The `selfupdate` seam design (injectable HTTP client, filesystem root, executable resolver,
  command runner) is what keeps the testing gate at PASS rather than aspirational. It is a design
  constraint, not an implementation detail, so it is stated in the Structure Decision and carried
  into `data-model.md`.
- Gate IV strengthens on inspection: because `update` uses only public release assets, it needs no
  credential at all. The requirement is therefore not merely "don't read `GITHUB_TOKEN`" but "send
  no credential" — an unauthenticated fetch is subject to stricter rate limits, which is a
  legitimate failure mode the command must report clearly rather than a reason to authenticate.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Dropping a **required** channel: no `sting update` subcommand | Verification cannot be skipped, and verifying in-process (the only way not to require `cosign` on the user's machine) cost +122% binary size and 62 extra modules to serve one command. | Shipping it anyway (rejected on cost). Shelling out to `cosign` (rejected: unusable for nearly every end user). Skipping verification (rejected: DECISIONS/0001 says that is strictly worse than shipping nothing). **Recorded as a deviation in [ADR 0011](../../docs/adr/0011-no-self-update-subcommand.md), which DECISIONS/0001 requires for a dropped required channel.** |
| One new `internal/` package rather than extending `internal/cli` | Holds `buildinfo` to the 80% coverage default instead of `internal/cli`'s documented 60% floor, and keeps Principle V's inward-only layering intact. | Adding to `internal/cli` (rejected: inherits the lower floor and entangles domain logic with the Cobra shell). |

## Deferred, and why

Recorded so absence is not read as oversight:

- **`winget` / `scoop`** and **Windows Authenticode signing** — out of scope per the spec; the
  issue's follow-up comment sequences signing ahead of those channels.
- **Enabling Windows self-replacement** — specified (FR-006, FR-013), implementation gated on the
  same signing work. This feature ships the gated refusal path.
- **Promoting `MACOS_*` secrets to org scope** — an org-administration change tracked in
  `skaphos-resources`, not here.
- **The `notarize` silent-skip hazard** — ADR-0001 records that unset signing secrets skip signing
  rather than failing. FR-038 addresses the general class (missing credential must fail the
  release); whether the `isEnvSet` gate itself should become a hard failure touches macOS signing
  policy and is left to the follow-up that owns it.
