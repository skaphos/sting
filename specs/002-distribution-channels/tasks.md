---

description: "Task list for distribution channel conformance"
---

# Tasks: Distribution Channel Conformance

**Input**: Design documents from `/specs/002-distribution-channels/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/](./contracts/)

**Tests**: REQUIRED. Per `.specify/memory/constitution.md`, new behavior ships with direct test
coverage in the same change. Tests must not touch the network (`net/http/httptest`) and must
isolate `HOME`/`USERPROFILE` when touching the filesystem. Per-package coverage gate: 80% default,
`internal/cli` 60%.

**Organization**: Grouped by user story so each is independently implementable and testable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1–US5)
- Exact file paths in every description

## Path Conventions

Single Go module at repository root. Go convention: tests live beside the code they test
(`internal/selfupdate/verify_test.go`), not in a separate `tests/` tree.

---

## Phase 1: Setup

**Purpose**: Establish a known-good baseline before any change, so regressions are attributable.

- [X] T001 Run `go -C tools tool task ci` and record the baseline result (lint, staticcheck, vuln, race tests, coverage gate) before any change
- [X] T002 [P] Confirm GoReleaser config validity with `goreleaser check` against the current `.goreleaser.yaml`
- [X] T003 [P] Create `.dockerignore` at repository root covering `.git/`, `dist/`, `specs/`, `graphify-out/`, `*.log`, `.env*` (required once a `Dockerfile` exists — US5)

---

## Phase 2: Foundational

**Purpose**: Cross-story prerequisites.

**This feature has almost none, and that is a finding rather than an omission.** Four of the five
obligations (US3, US4, US5, and the cross-cutting release work) are independent configuration
surfaces. The only real dependency edge is US1 → US2: the update command cannot decide whether it
is out of date until the binary knows its own version. That edge is expressed by phase ordering,
not by a shared foundation.

- [X] T004 Verify `internal/cli.Version`/`Commit`/`Date` remain the ldflags target named in `.goreleaser.yaml` so release stamping is unaffected by US1 (FR-003)

**Checkpoint**: Baseline green; user story work can begin.

---

## Phase 3: User Story 1 — Version identity (Priority: P1) 🎯 MVP

**Goal**: Every build reports honestly what it knows — module version from `go install`, revision
from a local build, an explicit "unavailable" when nothing was recorded. Never `dev`, never
`(devel)`.

**Independent Test**: Build without release-time stamping and confirm a real version/revision is
reported; build with stamping and confirm stamped values win.

### Tests for User Story 1 (REQUIRED) ⚠️

> Write these first and confirm they fail before implementing.

- [X] T005 [P] [US1] Table test for the resolution precedence matrix (ldflags > build info > unknown) in `internal/buildinfo/buildinfo_test.go`, covering all four rows of data-model.md §1
- [X] T006 [P] [US1] Test that `"(devel)"` is never surfaced as a version and maps to `SourceUnknown` in `internal/buildinfo/buildinfo_test.go` (FR-004)
- [X] T007 [P] [US1] Test that `vcs.modified` produces a dirty marker in rendered output in `internal/cli/version_test.go` (FR-002)

### Implementation for User Story 1

- [X] T008 [US1] Create `internal/buildinfo/buildinfo.go` with `Info`, `Source`, and `Resolve()` implementing the precedence table in data-model.md §1 (FR-001, FR-003, FR-004)
- [X] T009 [US1] Make `Resolve()` the single version source consumed by both the version command and the updater in `internal/buildinfo/buildinfo.go` (FR-005)
- [X] T010 [US1] Rewrite `internal/cli/version.go` to render from `internal/buildinfo`, keeping the exported ldflags vars as the stamping target and matching the output shape in contracts/cli-update.md
- [X] T011 [US1] Update `README.md` so the `go install` instruction no longer implies an unversioned binary

**Checkpoint**: `go install`-ed and locally built binaries both report honestly; release output unchanged.

---

## Phase 4: User Story 2 — `sting update` (Priority: P2)

**Goal**: Upgrade in place when sting owns the binary; refuse with the correct command when a
package manager does. Verify before replacing, always.

**Independent Test**: Point the updater at a tampered release and confirm the binary is untouched
and exit is non-zero; place a binary under a simulated package-manager path and confirm the right
command is printed and nothing is written.

### Setup for User Story 2

- [X] T012 [US2] Add `github.com/sigstore/sigstore-go` to `go.mod` and run `go mod tidy`
- [X] T013 [US2] Regenerate third-party notices with `go -C tools tool task notices` (constitution: required whenever `go.mod`/`go.sum` changes)

### Tests for User Story 2 (REQUIRED) ⚠️

- [X] T014 [P] [US2] Ownership classification tests for Homebrew, RPM, dpkg, Go toolchain, unmanaged, and undeterminable in `internal/selfupdate/ownership_test.go`, using an injected executable resolver and stubbed command runner (data-model.md §2)
- [X] T015 [P] [US2] Symlink-resolution test proving a cellar-linked binary classifies as Homebrew, not unmanaged, in `internal/selfupdate/ownership_test.go` (spec edge case)
- [X] T016 [P] [US2] **Negative test: valid signature, wrong signer identity is rejected** in `internal/selfupdate/verify_test.go` (FR-010, SC-013) — the single most important test in this feature
- [X] T017 [P] [US2] Checksum-mismatch test asserting the installed binary is byte-identical afterwards in `internal/selfupdate/verify_test.go` (FR-011)
- [X] T018 [P] [US2] Test that no flag or environment variable can skip verification in `internal/selfupdate/verify_test.go` (FR-012)
- [X] T019 [P] [US2] Release-resolution tests over `httptest` covering latest, explicit tag, draft/pre-release exclusion, tag-not-found, assets-not-yet-published, and no-asset-for-platform in `internal/selfupdate/release_test.go` (FR-020, spec edge cases)
- [X] T020 [P] [US2] Atomic-replace tests in `t.TempDir()` covering success, interrupted replace leaving exactly one complete binary, and unwritable directory in `internal/selfupdate/replace_test.go` (FR-013, FR-018)
- [X] T021 [P] [US2] Exit-code contract tests for every row of the table in contracts/cli-update.md in `internal/cli/update_test.go`
- [X] T022 [P] [US2] Test that no command other than `update` contacts a release or version endpoint in `internal/cli/update_test.go` (FR-007, SC-005)
- [X] T023 [P] [US2] Test that the update path sends no credential and does not read `GITHUB_TOKEN`/`GH_TOKEN`/`STING_TOKEN` in `internal/selfupdate/release_test.go` (Principle IV, FR-019)

### Implementation for User Story 2

- [X] T024 [P] [US2] Implement release resolution and asset download in `internal/selfupdate/release.go` with an injectable HTTP client (FR-020, data-model.md §3)
- [X] T025 [P] [US2] Implement ownership classification in `internal/selfupdate/ownership.go` per the ordered table in data-model.md §2, with injectable executable resolver and command runner (FR-008)
- [X] T026 [US2] Implement in-process Sigstore verification in `internal/selfupdate/verify.go` using `sigstore-go`, pinning issuer `https://token.actions.githubusercontent.com` and the release-workflow SAN pattern (FR-009, FR-010; research §2)
- [X] T027 [US2] Implement the four-step verification order — bundle over `checksums.txt`, identity pin, digest lookup, artifact hash — in `internal/selfupdate/verify.go` (data-model.md §4)
- [X] T028 [US2] Implement atomic replacement in `internal/selfupdate/replace.go` (temp file in the target directory, then rename) (FR-013)
- [X] T029 [US2] Implement the platform-gated variant in `internal/selfupdate/replace_windows.go`: rename-aside plus later cleanup, disabled pending Authenticode signing (FR-006, FR-013)
- [X] T030 [US2] Implement `Plan` resolution and the `Action` enum in `internal/selfupdate/plan.go` so `--check` and a real run share one decision path (FR-014, data-model.md §3)
- [X] T031 [US2] Create `internal/cli/update.go` as a thin Cobra command with `--check` and `--version`, mapping each `Action` to its exit code per contracts/cli-update.md (FR-006, FR-014, FR-016)
- [X] T032 [US2] Register `updateCmd` in `internal/cli/root.go`
- [X] T033 [US2] Write `docs/adr/0011-self-update-trust-model.md` recording verify-before-replace, the pinned signer identity, defer-to-package-manager, and the Windows gate (constitution: ADR for architecturally significant decisions)
- [X] T034 [US2] Document the upgrade path per install channel in `README.md` (FR-043)

**Checkpoint**: `sting update` verifies, defers, or refuses correctly on linux and darwin; gated on Windows.

---

## Phase 5: User Story 3 — Linux packages (Priority: P3)

**Goal**: A `.deb` and `.rpm` per architecture on every release, carrying the same supply-chain
guarantees as the archives.

**Independent Test**: Install each package on a matching container image, confirm the binary runs
and reports the release version, then remove it and confirm nothing is left behind.

### Tests for User Story 3 (REQUIRED) ⚠️

- [ ] T035 [P] [US3] Add a `goreleaser check` step to `.github/workflows/ci.yml` so `.goreleaser.yaml` is validated on every change
- [ ] T036 [P] [US3] Document the snapshot install/remove validation for both package formats in `specs/002-distribution-channels/quickstart.md` (already drafted — verify it runs as written)

### Implementation for User Story 3

- [ ] T037 [US3] Add an `nfpms:` block to `.goreleaser.yaml` producing `deb` and `rpm` for amd64 and arm64, with binary at `/usr/bin/sting` and license plus notice files at the packaging-conventional path (FR-021, FR-022)
- [ ] T038 [US3] Declare maintainer, homepage, license (MIT), and description in the `nfpms:` block (FR-023)
- [ ] T039 [US3] Extend the `sboms:` block in `.goreleaser.yaml` with an `artifacts: package` entry so packages carry SBOMs (research §6 — the one guarantee packages do *not* inherit for free)
- [ ] T040 [US3] Add a Linux packages section to `README.md` stating plainly that there is no hosted apt/yum repository and that upgrades mean downloading the next release (FR-025)

**Checkpoint**: A snapshot build emits 4 packages, all listed in `checksums.txt`.

---

## Phase 6: User Story 4 — MCP registry entry (Priority: P4)

**Goal**: sting is discoverable by name in the registry MCP clients read.

**Independent Test**: Validate `server.json` against the published schema in CI; confirm a release
publishes an entry whose version matches the tag.

### Tests for User Story 4 (REQUIRED) ⚠️

- [ ] T041 [P] [US4] Add a `server.json` schema-validation step to `.github/workflows/ci.yml` against `https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json` (FR-027)
- [ ] T042 [P] [US4] Add a drift test asserting `server.json` describes exactly the tools `mcpserver.ReadOnlyTools()` advertises in `internal/mcpserver/server_test.go` (FR-030)

### Implementation for User Story 4

- [ ] T043 [US4] Create `server.json` at repository root per contracts/server-json.md, named `io.skaphos/sting` with an OCI package entry, stdio transport, and the `STING_TOKEN`/`STING_GITLAB_TOKEN` environment variables (FR-026)
- [ ] T044 [US4] Add an `mcp-publisher login dns` + `publish` step to `.github/workflows/release.yml`, authenticating with the org-scoped `MCP_REGISTRY_KEY` secret (FR-028)
- [ ] T045 [US4] Make the registry publish step non-blocking but loudly reported — `continue-on-error` plus a workflow annotation — so a failure is visible without invalidating the release (FR-029)
- [ ] T046 [US4] Document the `skaphos.io` TXT record (`v=MCPv1; k=ed25519; p=<base64>`) and the `MCP_REGISTRY_KEY` secret as release prerequisites in `docs/adr/0011-self-update-trust-model.md` or a sibling operations note

**Checkpoint**: `server.json` validates in CI; publishing is wired and non-blocking.

---

## Phase 7: User Story 5 — Container image (Priority: P5)

**Goal**: A multi-arch image on GHCR that runs the MCP server over stdio with no local toolchain.

**Independent Test**: Pull on both architectures and drive the containerized server with an MCP
client, passing a credential through the environment.

### Tests for User Story 5 (REQUIRED) ⚠️

- [ ] T047 [P] [US5] Extend the `goreleaser check` CI step to cover the new `dockers_v2` block (shares the step added in T035)
- [ ] T048 [P] [US5] Add a snapshot-build validation step to `specs/002-distribution-channels/quickstart.md` confirming both platform-suffixed images build locally (snapshot mode does not push, so there is no manifest to inspect)

### Implementation for User Story 5

- [ ] T049 [US5] Create `Dockerfile` at repository root that copies the GoReleaser-built binary from the per-platform build context, runs as a non-root user, includes CA certificates, and defaults to `sting mcp` (FR-031, FR-032, FR-033)
- [ ] T050 [US5] Add a `dockers_v2:` block to `.goreleaser.yaml` for `ghcr.io/skaphos/sting`, platforms `linux/amd64` and `linux/arm64`, tagged with the version and `latest` (FR-031; research §3)
- [ ] T051 [US5] Add buildx setup, binfmt, and GHCR login steps to `.github/workflows/release.yml` before the GoReleaser step, all release-blocking on failure (research §3, FR-037)
- [ ] T052 [US5] Ensure the image carries an SBOM, signature, and provenance attestation equivalent to the archives (FR-035)
- [ ] T053 [US5] Add a container-based MCP client configuration to `README.md` alongside the local-binary one (FR-036)

**Checkpoint**: A snapshot build produces per-platform images; a real release produces a multi-arch manifest.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: The release-coherence work that asserts on everything the prior phases produce. Last
because it depends on all of them.

- [ ] T054 **Change existing behavior**: make an unusable Homebrew credential fail the release in `.github/workflows/release.yml`, replacing the current `--skip=homebrew` + `::warning::` path (FR-038) — the highest-value change in this feature relative to the documented failure mode
- [ ] T055 Add a post-release `verify` job to `.github/workflows/release.yml`, gated on the release job, asserting the **version each channel serves** for release assets, the Homebrew cask, and the container image (FR-039, FR-040)
- [ ] T056 Include the MCP registry in the verify job as a reported, non-blocking channel (FR-029, FR-040)
- [ ] T057 Add retry-with-backoff to the verify job so third-party propagation latency is not reported as a failed release (FR-041)
- [ ] T058 [P] Add an upgrade-path matrix to `README.md` mapping each install channel to its correct upgrade command (FR-043)
- [ ] T059 [P] Update `AGENTS.md` if any development workflow changed
- [ ] T060 Verify the per-package coverage gate passes, including the new `internal/buildinfo` and `internal/selfupdate` packages at the 80% default (`go -C tools tool task test` plus `scripts/check-coverage.sh`)
- [ ] T061 Run `go -C tools tool task ci` and confirm it passes end to end
- [ ] T062 Run the [quickstart.md](./quickstart.md) validation for every user story
- [ ] T063 Confirm `reuse lint` still passes for all new files (`REUSE.toml` uses an aggregate `**` annotation, so new files are covered — verify rather than assume)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: minimal by design — see the note in that phase
- **US1 (Phase 3)**: after Phase 2. **Blocks US2.**
- **US2 (Phase 4)**: after US1 — the updater must be able to resolve its own version (FR-005, FR-017)
- **US3 (Phase 5)**, **US4 (Phase 6)**, **US5 (Phase 7)**: after Phase 2; mutually independent and independent of US1/US2
- **Polish (Phase 8)**: after US3, US4, and US5, because the verify job asserts on their artifacts

### The only hard edge

```text
US1 ──► US2
US3 ──┐
US4 ──┼──► Phase 8
US5 ──┘
```

US3, US4 and US5 touch `.goreleaser.yaml` and `release.yml` in different blocks; if worked
concurrently, coordinate on those two files.

### Within Each User Story

- Tests written and failing before implementation
- `internal/` domain logic before the Cobra command that calls it
- Configuration before the CI step that validates it

### Parallel Opportunities

- T002, T003 in Setup
- All US1 tests (T005–T007)
- All US2 tests (T014–T023) — ten independent test files/cases
- T024 and T025 (different files, no shared state)
- US3, US4 and US5 entire phases, subject to the shared-file caveat above

---

## Parallel Example: User Story 2

```bash
# Tests first, all independent:
Task: "Ownership classification tests in internal/selfupdate/ownership_test.go"
Task: "Wrong-signer-identity rejection test in internal/selfupdate/verify_test.go"
Task: "Release resolution tests over httptest in internal/selfupdate/release_test.go"
Task: "Atomic replace tests in internal/selfupdate/replace_test.go"
Task: "Exit-code contract tests in internal/cli/update_test.go"

# Then the two independent implementation files:
Task: "Release resolution in internal/selfupdate/release.go"
Task: "Ownership classification in internal/selfupdate/ownership.go"
```

---

## Implementation Strategy

### MVP (User Story 1 only)

Phases 1–3. Delivers the fix for the defect issue #121 names concretely — `sting version` printing
`dev` for the install path the README recommends. Independently valuable and shippable.

### Incremental delivery

1. Phases 1–3 → US1, the MVP
2. Phase 4 → US2, the upgrade path (needs US1)
3. Phases 5–7 → US3/US4/US5, the three remaining channels, in any order
4. Phase 8 → release coherence, which needs all channels present to assert on

### Scope note

`winget`, `scoop`, Windows Authenticode signing, and enabling Windows self-replacement are **out of
scope** — see spec.md Out of Scope. T029 ships the gated refusal path, not the replacement path.

---

## Notes

- 63 tasks: 3 setup, 1 foundational, 7 US1, 23 US2, 6 US3, 6 US4, 7 US5, 10 polish
- US2 carries a third of the work; the other four obligations are largely configuration
- Commit after each task or logical group; all commits `-S -s` as `shawn@skaphos.io` (skaphos org)
- Verify tests fail before implementing
