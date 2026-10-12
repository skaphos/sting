# Implementation Plan: Credential Source Visibility

**Branch**: `feature/45-credential-source-visibility` | **Date**: 2026-10-11 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/004-credential-source-visibility/spec.md`

## Summary

Credential resolution in `internal/commitclient` returns a secret-free `Resolution` record
(provider, host, source, kind, lookup error) along with the token. When a `--verbose` /
`STING_VERBOSE` / `verbose:` setting is on, each client constructor writes one
`sting: credential: …` line to stderr. The writer is injected through a variadic functional
option, so default behavior and every existing call site stay unchanged. The CLI passes its
stderr when verbose, and the MCP server receives the same writer and threads it into every
tool call. A `credentials.ErrNotFound` sentinel separates "nothing stored" from "lookup
failed", so a silent fallback to anonymous becomes visible. README documents the setting and
the MCP mixed-auth precedence. Which credential gets selected does not change.

## Technical Context

**Language/Version**: Go 1.27.2 (per `go.mod`)

**Primary Dependencies**: Existing only: Cobra, viper, `modelcontextprotocol/go-sdk`. No new
modules, so `third_party_licenses/` stays unchanged.

**Storage**: N/A. The feature reads the existing credential store and persists nothing.

**Testing**: stdlib `testing`; `bytes.Buffer` writers; a fake `credentials.Store` behind a
package-level seam; isolated `HOME`/`USERPROFILE`; no network.

**Target Platform**: The CLI and the stdio MCP server on macOS, Linux, and Windows.

**Project Type**: CLI and MCP server (single Go module).

**Performance Goals**: No measurable cost. Credential lookup already happens; the feature adds
one formatted line when verbose is on.

**Constraints**: stdout and the MCP protocol stream must stay byte-identical; no secret
material in any report; default output unchanged.

**Scale/Scope**: About 6 Go files changed in `internal/commitclient`, `internal/credentials`,
`internal/mcpserver`, and `internal/cli`, plus README and CHANGELOG-adjacent docs. No public
package changes.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Gate | Check | Status |
|---|------|-------|--------|
| I | Read-only by design | No new tool or mutation path; the tool set and `ReadOnlyTools()` are untouched | PASS |
| II | Evidence-grade output | Results are unchanged. The report is stderr-only diagnostics, and putting the source into `Result` is explicitly deferred (spec Assumptions) | PASS |
| III | Deterministic queries | No change to query resolution, windows, or bounds | PASS |
| IV | Explicit config, dedicated credentials | The `verbose` key goes through viper's flags > env > file precedence; credential precedence is unchanged; `resolution` has no token field and tests assert the token never appears (FR-009). This change adds no ambient-token reading. It does surface a **pre-existing** deviation, the `STING_ALLOW_AMBIENT_GITHUB_TOKEN` opt-in (see Complexity Tracking) | PASS for this change; pre-existing deviation recorded |
| V | Public packages are the contract | No change to `model`, `config`, `ghclient`, or `gitlabclient`; verbosity stays in `internal/cli` (research R3); `SchemaVersion` unchanged | PASS |
| VI | Partial results over blindness | Strengthened: a failed stored-credential lookup that degrades to anonymous becomes reportable instead of silent | PASS |
| VII | Never a second source of truth | Nothing persisted; installer untouched | PASS |
| VIII | Technical precision, honest scope | README documents verified precedence, including the existing ambient opt-in, which is undocumented today; the report line is declared non-contractual | PASS |
| — | Testing non-negotiables | Unit tests for every `Source`, the lookup-failure path, and the silent default; CLI and MCP wiring tests; no network; isolated HOME; coverage gates checked with `test-cover-check` | PASS |
| — | Governance | PR-only, `git commit -S -s`, `feat:` Conventional Commit; README updated; no ADR needed (an additive diagnostic, no architectural decision; ADR 0008 precedence is unchanged) | PASS |

**Post-design re-check (after Phase 1)**: All gates still PASS. The design adds no public API,
no dependency, and no persisted state.

## Project Structure

### Documentation (this feature)

```text
specs/004-credential-source-visibility/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/credential-report.md
├── checklists/requirements.md
└── tasks.md             # /speckit-tasks
```

### Source Code (repository root)

```text
internal/credentials/
└── credentials.go       # + ErrNotFound sentinel wrapped by Load's final miss

internal/commitclient/
├── commitclient.go      # Resolution, Source, Option/WithCredentialReport, openCredentialStore seam;
│                        # resolve* return (token, Resolution); New/NewActivity emit
├── pr.go                # NewPR(cfg, opts...) emits
└── report_test.go       # new: source matrix, lookup failure, silence, no-secret

internal/mcpserver/
├── server.go            # New(cfg, opts...), WithDiagnostics; handler.diag; collectCommits threads opts
├── getrepoactivity.go   # pass commitclient.WithCredentialReport(h.diag)
├── getprs.go            # same
└── getprinbox.go        # same

internal/cli/
├── root.go              # --verbose persistent flag bound to viper "verbose"; diagnostics(cmd) helper
├── query.go, activity.go, prs.go, inbox.go   # pass the option
└── mcp.go               # mcpserver.WithDiagnostics(diagnostics(cmd))

README.md                # verbosity setting, source labels, MCP mixed-auth precedence
```

**Structure Decision**: Existing single-module layout. All changes are internal packages plus
README.

## Complexity Tracking

This feature introduces no constitution violations. It does make one **pre-existing**
deviation visible, recorded here so the gate above is not read as clearing it:

| Deviation | Origin | Status |
|-----------|--------|--------|
| `internal/credentials` reads `GH_TOKEN` / `GITHUB_TOKEN` or the `gh` config file when `STING_ALLOW_AMBIENT_GITHUB_TOKEN` is set. That conflicts with Principle IV ("MUST NOT read ambient provider tokens"), ADR 0002, and ADR 0008 ("intentionally never consulted"). | PR #32 narrowed a previously *unconditional* ambient fallback to this opt-in as the safer default, and deferred the matching ADR update to a separate docs PR that never landed. | Unresolved by this feature. It needs a maintainer decision: remove the opt-in, or supersede ADR 0002/0008 and amend Principle IV to sanction it. This feature only labels the path honestly in reports and docs; it neither extends nor endorses it. |
