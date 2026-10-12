# Tasks: Credential Source Visibility

**Input**: Design documents from `/specs/004-credential-source-visibility/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/credential-report.md, quickstart.md

**Tests**: REQUIRED (constitution, testing non-negotiables). No network; isolated
`HOME`/`USERPROFILE`; the per-package coverage gate must hold.

## Phase 1: Setup

No setup tasks: the feature uses the existing module, dependencies, and layout.

## Phase 2: Foundational (blocks all stories)

- [ ] T001 Add the exported sentinel `ErrNotFound` and wrap the final "no credential found" return of `store.Load` with it (`%w`), keeping the message text, in internal/credentials/credentials.go
- [ ] T002 Add a regression test asserting that `errors.Is(err, ErrNotFound)` holds for a genuine miss and does NOT hold for the "keyring unavailable" errors, in internal/credentials/credentials_test.go
- [ ] T003 Define the `Source` enum, the `Resolution` struct (Provider, Host, Source, Kind, LookupErr; no token field), and `Resolution.String()` producing exactly the contract line format of contracts/credential-report.md, in internal/commitclient/report.go
- [ ] T004 Add the `openCredentialStore = credentials.New` package seam, and change `resolveGitHubToken`/`resolveGitLabToken` to return `(string, Resolution)` mapping dedicated / keyring / file / ambient-env / ambient-gh / anonymous plus LookupErr (non-ErrNotFound `Load` errors and `credentials.New` errors), with precedence unchanged, in internal/commitclient/commitclient.go
- [ ] T005 Add `Option` / `WithCredentialReport(w io.Writer)`, and make `New`, `NewActivity`, and `NewPR` accept `...Option` and write `Resolution.String()+"\n"` to the writer when non-nil, in internal/commitclient/commitclient.go and internal/commitclient/pr.go
- [ ] T006 Update existing resolve tests to the new return signature in internal/commitclient/commitclient_test.go

**Checkpoint**: The package builds, existing tests pass, and nothing is written without the option.

## Phase 3: User Story 1 - See which credential a CLI run used (P1) 🎯 MVP

**Goal**: `--verbose` / `STING_VERBOSE` / `verbose:` makes `query`, `activity`, `prs`, and
`inbox` report the credential on stderr.

**Independent Test**: Run each command verbose and non-verbose under PAT, stored, and none;
check the stderr line, and check that stdout is identical.

- [ ] T007 [P] [US1] Table test over a fake `credentials.Store` covering every source and kind, a GHES/self-managed host derived from a base URL with an API path, the keyring-unavailable lookup failure, a `credentials.New` failure, and an empty PAT; assert each exact line and that the token string never appears, in internal/commitclient/report_test.go
- [ ] T008 [P] [US1] Test that `New`, `NewActivity`, and `NewPR` write exactly one line with the option and nothing without it (nil writer or no option), in internal/commitclient/report_test.go
- [ ] T009 [US1] Add the persistent `--verbose` flag bound to viper key `verbose`, and a `diagnostics(cmd *cobra.Command) io.Writer` helper (stderr when on, nil when off), in internal/cli/root.go
- [ ] T010 [US1] Pass `commitclient.WithCredentialReport(diagnostics(cmd))` at the call sites in internal/cli/query.go, internal/cli/activity.go, internal/cli/prs.go, and internal/cli/inbox.go
- [ ] T011 [US1] CLI test: with an isolated HOME, `--verbose` and `STING_VERBOSE=true` each produce the report on the command's stderr; with verbose off, stderr has no `sting: credential:` line; stdout carries no report, in internal/cli/verbose_test.go

**Checkpoint**: The MVP works on the CLI.

## Phase 4: User Story 2 - See which credential an MCP tool call used (P2)

**Goal**: The MCP server reports once per tool call on stderr.

**Independent Test**: Build the server with a diagnostics buffer, call each of the four tools,
and expect four lines; build it without diagnostics and expect none.

- [ ] T012 [US2] Add `Option` / `WithDiagnostics(w io.Writer)` to `mcpserver.New(cfg, opts...)`, store the writer on `handler`, and thread `commitclient.WithCredentialReport(h.diag)` through `collectCommits` and the activity, PR, and inbox handlers, in internal/mcpserver/server.go, internal/mcpserver/getrepoactivity.go, internal/mcpserver/getprs.go, internal/mcpserver/getprinbox.go
- [ ] T013 [US2] Wire `mcpserver.WithDiagnostics(diagnostics(cmd))` into internal/cli/mcp.go
- [ ] T014 [US2] Test that each of the four tools writes exactly one report line per call to the diagnostics writer, and none without the option, using httptest-backed base URLs, in internal/mcpserver/diagnostics_test.go

## Phase 5: User Story 3 - Understand MCP mixed-auth precedence from the docs (P3)

**Goal**: The README answers which credential the MCP server uses and how to change it.

**Independent Test**: A reader answers both questions in User Story 3 from the README alone.

- [ ] T015 [P] [US3] Document credential precedence (including the existing `STING_ALLOW_AMBIENT_GITHUB_TOKEN` opt-in), the `verbose` setting, and the report labels, in README.md (Authentication / Configuration)
- [ ] T016 [P] [US3] Document MCP mixed-auth: the same precedence per tool call; forwarded `STING_TOKEN` and config-file PATs are server-wide and override the OAuth login; leaving the variable unset or empty lets OAuth apply; enabling `STING_VERBOSE` for the server, in README.md (Agent integration)

## Phase 6: Polish & Cross-Cutting

- [ ] T017 Run `go -C tools tool task lint`, `test`, and `test-cover-check`, and fix any findings
- [ ] T018 Run the quickstart.md CLI scenarios 1, 2, 4, 5, and 6 against an isolated HOME (scenario 3 needs an interactive OAuth login and is covered by T007)
- [ ] T019 Mark the delivered items in specs/004-credential-source-visibility/tasks.md and set the spec Status to Implemented

## Dependencies

- Phase 2 → all stories. US1 (T009) provides the `diagnostics` helper that US2 (T013) reuses.
  US3 is independent (docs only).
- Within Phase 2: T001 → T004; T003 → T004 → T005 → T006.

## Parallel Opportunities

- T007 and T008 run together (same new file, independent cases written together).
- T015 and T016 can be written alongside any code phase.
- T002 runs in parallel with T003.

## Implementation Strategy

MVP = Phase 2 + US1. US2 is a small extension through the same option. US3 lands in the same
PR so the documented behavior matches the shipped behavior (Principle VIII). The whole
feature ships as a single `feat:` PR for the next minor release.
