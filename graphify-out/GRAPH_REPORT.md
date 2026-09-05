# Graph Report - sting  (2026-09-04)

## Corpus Check
- 178 files · ~164,354 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 1650 nodes · 3741 edges · 107 communities (92 shown, 9 thin omitted)
- Extraction: 87% EXTRACTED · 13% INFERRED · 0% AMBIGUOUS · INFERRED: 482 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `26cf951a`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- ActivityResult
- context.Context
- credentials.go
- NewTransport
- testing.T
- newCmd
- cli_test.go
- serverjson_test.go
- tomledit.go
- Tasks: Distribution Channel Conformance
- Disclosure
- common.sh
- activityQuery
- resolve
- Tasks: Repository Activity Digest
- isolateHome
- New
- Tasks: [FEATURE NAME]
- Provider
- newTestClient
- newTestClient
- ghclient/activity_test.go
- sting
- Feature Specification: Repository Activity Digest
- Config
- Entry
- Feature Specification: Distribution Channel Conformance
- Changelog
- Open Design Questions — **LOCKED DECISIONS** (2026-05-30)
- newActivityCmd
- ByName
- github.com/spf13/cobra.Command
- Default
- Registering OAuth Apps for Sting
- runtime.go
- buildSearchQuery
- getcommits_test.go
- Core Principles
- adapters_error_test.go
- codexAdapter
- swapCollectActivity
- Quickstart: Validating Repository Activity Digest
- install.go
- Feature Specification: [FEATURE NAME]
- Implementation Plan: Distribution Channel Conformance
- adr/README.md
- Phase 1 Data Model: Repository Activity Digest
- Phase 0 Research: Repository Activity Digest
- Repository Guidelines
- budgetedQueryClient
- grokAdapter
- Snippet
- model/activity_test.go
- Core Principles
- GitHub Copilot Instructions for sting
- 11. No self-update subcommand (deviation from DECISIONS/0001)
- estimateServer
- runAuthGitLab
- server.json
- Implementation Plan: [FEATURE]
- Implementation Plan: Repository Activity Digest
- Quickstart: Validating Distribution Channel Conformance
- Phase 0 Research: Distribution Channel Conformance
- Contributing Guidelines
- runtime_more_test.go
- Contract: `sting activity` CLI Command
- Contract: Go Public API
- Contract: Release artifact set and channel verification
- Data Model: Distribution Channel Conformance
- writeInstallListTable
- config/activity_test.go
- 1. Deliver MCP server and CLI from one binary
- 3. Multi-runtime MCP installer and read-only safety model
- 6. GitLab provider support
- 8. OAuth App authentication and multi-provider credential storage
- opencode.go
- Specification Quality Checklist: Repository Activity Digest
- Specification Quality Checklist: Distribution Channel Conformance
- Contract: `server.json` — MCP registry entry
- 2. Dedicated GitHub PAT via viper, separate from GITHUB_TOKEN
- 4. Public packages and Wake evidence shape
- 5. Release Please owns release notes
- 7. Commit file and diff evidence
- 10. Multi-tool MCP server
- keyring_test.go
- Contract: `get_repo_activity` MCP Tool
- Contract: `sting update` — NOT SHIPPED
- 9. GoReleaser owns the GitHub release (supersedes part of ADR 0005)
- runAuthLogout
- model_test.go
- run_generation
- [CHECKLIST TYPE] Checklist: [FEATURE NAME]
- main
- failingKeyring
- succeedingKeyring
- Invoke-NoticeGeneration
- lockedKeyring
- check-coverage.sh script
- Third-Party Notices
- github.com/skaphos/sting
- github.com/skaphos/sting/tools

## God Nodes (most connected - your core abstractions)
1. `Default()` - 44 edges
2. `newCmd()` - 43 edges
3. `activityQuery()` - 39 edges
4. `newTestClient()` - 39 edges
5. `isolateHome()` - 37 edges
6. `ByName()` - 36 edges
7. `New()` - 30 edges
8. `WithFilePath()` - 28 edges
9. `Query` - 28 edges
10. `NewTransport()` - 23 edges

## Surprising Connections (you probably didn't know these)
- `TestGetCommitsIncludePRs()` --calls--> `boolPtr()`  [INFERRED]
  internal/mcpserver/getcommits_test.go → config/activity_test.go
- `main()` --calls--> `Execute()`  [EXTRACTED]
  cmd/sting/main.go → internal/cli/root.go
- `applyActivityCostFlags()` --references--> `ActivityRequest`  [EXTRACTED]
  internal/cli/activity.go → config/activity.go
- `Config` --references--> `Provider`  [EXTRACTED]
  config/config.go → model/model.go
- `Config` --references--> `Scope`  [EXTRACTED]
  config/config.go → model/model.go

## Import Cycles
- None detected.

## Communities (107 total, 9 thin omitted)

### Community 0 - "ActivityResult"
Cohesion: 0.05
Nodes (81): github.com/spf13/pflag.FlagSet, strings.Builder, conventionalScope(), Correlate(), correlatePath(), leadingSegment(), matchingSHAs(), observedSHAs() (+73 more)

### Community 1 - "context.Context"
Cohesion: 0.06
Nodes (50): anyPatchTruncated(), changeSetFromFiles(), estimateRequests(), Client, isRateLimitError(), resolveBoundaries(), TestCollectUnsupportedScope(), apiError() (+42 more)

### Community 2 - "credentials.go"
Cohesion: 0.09
Nodes (30): CredentialRef, defaultKeyring, hostsFile, KeyringBackend, Source, Token, TokenType, sync.RWMutex (+22 more)

### Community 3 - "NewTransport"
Cohesion: 0.12
Nodes (30): rateObservation, stubTransport, unmeteredKey, net/http.Header, net/http.Request, net/http.Response, sync/atomic.Int64, Transport (+22 more)

### Community 4 - "testing.T"
Cohesion: 0.13
Nodes (38): testing.T, TestCombinedKeyringAndFile(), TestConcurrentSaveLoad(), TestDefaultStingDirUsesXDGConfigHome(), TestDelete(), TestDeleteCleansEmptyComposite(), TestDeleteRemovesFromInsecureBackend(), TestDeleteSurfacesKeyringFailure() (+30 more)

### Community 5 - "newCmd"
Cohesion: 0.13
Nodes (38): runAuthStatus(), installClaude(), isolateHome(), newCmd(), registerInstallFlags(), registerUninstallFlags(), TestAuthStatusOutput_NoCredentials(), TestAuthStatusOutput_VariousStates() (+30 more)

### Community 6 - "cli_test.go"
Cohesion: 0.10
Nodes (33): TestConfigMaxRequestsDefaultAndValidation(), Defaults(), registerActivityFlags(), contains(), seedValidConfig(), TestAuthLoginVerboseFormWired(), TestConfigMissing(), TestConfigMissingExplicitConfigFile() (+25 more)

### Community 7 - "serverjson_test.go"
Cohesion: 0.11
Nodes (28): github.com/modelcontextprotocol/go-sdk/mcp.CallToolRequest, github.com/modelcontextprotocol/go-sdk/mcp.CallToolResult, github.com/modelcontextprotocol/go-sdk/mcp.Server, github.com/modelcontextprotocol/go-sdk/mcp.Tool, handler, boolPtr(), errorResult(), handler (+20 more)

### Community 8 - "tomledit.go"
Cohesion: 0.13
Nodes (30): io/fs.FileMode, TestResolveTargetDanglingSymlink(), TestResolveTargetMissing(), TestWriteAtomicNewFile(), TestWriteAtomicOverwritePreservesMode(), TestWriteAtomicParentMissing(), TestWriteAtomicRenameOntoDir(), TestWriteAtomicThroughSymlink() (+22 more)

### Community 9 - "Tasks: Distribution Channel Conformance"
Cohesion: 0.06
Nodes (33): Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation for User Story 4, Implementation for User Story 5, Implementation Strategy (+25 more)

### Community 10 - "Disclosure"
Cohesion: 0.18
Nodes (29): DisclosureInput, AncestryDiverged(), AuthorFilterNotApplied(), BudgetBounded(), Build(), EnrichmentPartial(), NetComparisonBlindspot(), PatchTruncated() (+21 more)

### Community 11 - "common.sh"
Cohesion: 0.13
Nodes (25): check-prerequisites.sh script, check_dir(), check_file(), find_specify_root(), format_speckit_command(), get_current_branch(), get_feature_paths(), get_invoke_separator() (+17 more)

### Community 12 - "activityQuery"
Cohesion: 0.17
Nodes (26): TestCollectActivityBudgetStopImmediately(), TestCollectActivityBudgetStopReturnsPartialResult(), TestCollectActivityGenerousCeilingCompletes(), TestCollectActivityUncappedRunIsNotStopped(), TestEstimateOnlyGathersNothing(), TestPreflightQuotaExhausted(), budgetedClientWithTransport(), clientWithTransport() (+18 more)

### Community 13 - "resolve"
Cohesion: 0.13
Nodes (24): Source, runtime/debug.BuildInfo, cleanSentinel(), Info, Resolve(), resolve(), stamped(), bi() (+16 more)

### Community 14 - "Tasks: Repository Activity Digest"
Cohesion: 0.07
Nodes (30): Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation for User Story 4, Implementation Strategy, Incremental Delivery (+22 more)

### Community 15 - "isolateHome"
Cohesion: 0.14
Nodes (26): TestGrokDetectStaleEnvVar(), TestOpencodeDetectStaleEnvVar(), mustDetect(), TestClaudeDetect(), TestClaudeDetectDir(), TestCodexDetect(), TestConfigPathGrokEnvOverride(), TestConfigPathOpencodeEnvOverride() (+18 more)

### Community 16 - "New"
Cohesion: 0.13
Nodes (24): TestListOrgEmpty(), TestListReposEmpty(), TestListReposInvalidTarget(), TestNewMalformedBaseURL(), TestNewPerPageClamping(), New(), TestAPIErrorClassification(), TestCollectReposMaxCommits() (+16 more)

### Community 17 - "Tasks: [FEATURE NAME]"
Cohesion: 0.07
Nodes (26): Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation Strategy, Incremental Delivery, MVP First (User Story 1 Only) (+18 more)

### Community 18 - "Provider"
Cohesion: 0.10
Nodes (19): ActivityRequest, Config, ParseTime(), ParseWindow(), TestParseTime(), TestParseWindow(), TestParseWindowOverflow(), TestValidateGitLabSearchScopeIncompatible() (+11 more)

### Community 19 - "newTestClient"
Cohesion: 0.14
Nodes (22): TestCollectActivityInvalidRepo(), TestCollectActivityPermissionDeniedIsFatal(), TestCollectReturnsPartialResultsWhenEnrichmentFails(), TestCollectSucceedsWithoutEnrichment(), Client, newTestClient(), TestCollectExactMaxCommitsNotTruncated(), TestCollectIncludeFilesAndDiffs() (+14 more)

### Community 20 - "newTestClient"
Cohesion: 0.15
Nodes (23): gitlabCommitsBodyN(), isDiffPath(), TestCollectGroupScopeDiffs(), TestEnrichDiffsAbortsOnError(), TestEnrichDiffsConcurrent(), New(), skipProjectReason(), Client (+15 more)

### Community 21 - "ghclient/activity_test.go"
Cohesion: 0.30
Nodes (20): compareOK(), disclosureKinds(), fileEntry(), hasDisclosure(), pathBase(), splitCompareRange(), TestCollectActivityAuthorFilterDisclosed(), TestCollectActivityDeterministic() (+12 more)

### Community 22 - "sting"
Cohesion: 0.09
Nodes (23): Agent integration (the main use case), Authentication (recommended), CLI usage, Configuration, Development, Documentation, Evidence depth, First-time setup (+15 more)

### Community 23 - "Feature Specification: Repository Activity Digest"
Cohesion: 0.09
Nodes (23): Assumptions, Clarifications, Contract, safety, and parity, Cost-bounded evidence gathering, Cost visibility and budgeting, Edge Cases, Feature Specification: Repository Activity Digest, Functional Requirements (+15 more)

### Community 24 - "Config"
Cohesion: 0.19
Nodes (20): ActivityClient, Client, Config, credentialHost(), githubHost(), gitlabHost(), New(), NewActivity() (+12 more)

### Community 25 - "Entry"
Cohesion: 0.20
Nodes (10): decodeJSONInto(), Scope, jsonObjectAt(), readJSONDoc(), writeJSONDoc(), checkJsonc(), Entry, claudeAdapter (+2 more)

### Community 26 - "Feature Specification: Distribution Channel Conformance"
Cohesion: 0.10
Nodes (21): Assumptions, Clarifications, Constitution Alignment, Dependencies, Edge Cases, Feature Specification: Distribution Channel Conformance, Functional Requirements, Key Entities (+13 more)

### Community 27 - "Changelog"
Cohesion: 0.10
Nodes (20): [0.0.2](https://github.com/skaphos/sting/compare/v0.0.1...v0.0.2) (2026-05-30), [0.0.3](https://github.com/skaphos/sting/compare/v0.0.2...v0.0.3) (2026-05-31), [0.0.4](https://github.com/skaphos/sting/compare/v0.0.3...v0.0.4) (2026-06-22), [0.0.5](https://github.com/skaphos/sting/compare/v0.0.4...v0.0.5) (2026-06-30), [0.0.6](https://github.com/skaphos/sting/compare/v0.0.5...v0.0.6) (2026-07-11), [0.0.7](https://github.com/skaphos/sting/compare/v0.0.6...v0.0.7) (2026-07-12), [1.1.0](https://github.com/skaphos/sting/compare/v1.0.0...v1.1.0) (2026-07-26), Added (+12 more)

### Community 28 - "Open Design Questions — **LOCKED DECISIONS** (2026-05-30)"
Cohesion: 0.10
Nodes (19): 1. File format for plaintext fallback — **LOCKED**, 2. Keyring library choice — **LOCKED**, 3. Migration from existing config — **LOCKED**, 4. Per-user vs per-host for GHES — **LOCKED**, 5. Environment variable precedence — **LOCKED**, 6. "Bring your own OAuth client" configuration — **LOCKED (keys TBD)**, Current Usage, Future Opportunities (When Implementing `auth` Login Commands) (+11 more)

### Community 29 - "newActivityCmd"
Cohesion: 0.33
Nodes (18): bytes.Buffer, activityTestServer(), newActivityCmd(), runActivityCmd(), TestActivityCommandBadFormat(), TestActivityCommandBadWindow(), TestActivityCommandBudgetBoundedExitsZero(), TestActivityCommandEstimateJSON() (+10 more)

### Community 30 - "ByName"
Cohesion: 0.30
Nodes (17): TestClaudeCreatesPrivateFile(), TestClaudeLargeIntegerRoundTrip(), TestClaudeNullMcpServers(), TestClaudeWritePreservesUserKeys(), TestOpencodeWritePreservesUserKeys(), ByName(), readFile(), TestCodexAppendPreservesExisting() (+9 more)

### Community 31 - "github.com/spf13/cobra.Command"
Cohesion: 0.31
Nodes (16): bufio.Reader, github.com/spf13/cobra.Command, io.Writer, ensureDefaultProvider(), offerInstall(), printFinalSummary(), prompt(), runGitHubAuthWizard() (+8 more)

### Community 32 - "Default"
Cohesion: 0.22
Nodes (16): Default(), TestResolveAcceptsValidGitHubAuthors(), TestResolveDefaults(), TestResolveDiffsImplyFiles(), TestResolveExplicitProvider(), TestResolveExplicitSinceUntil(), TestResolveExplicitWindow(), TestResolveGitLabSearchUnsupported() (+8 more)

### Community 33 - "Registering OAuth Apps for Sting"
Cohesion: 0.12
Nodes (17): GitHub (github.com and GitHub Enterprise Server), GitLab (gitlab.com and self-hosted), Migration and Fallbacks, Next Steps / Status, Public App vs. Your Own App: Trust and Governance, Recommended Settings, Recommended Settings, Registering OAuth Apps for Sting (+9 more)

### Community 34 - "runtime.go"
Cohesion: 0.19
Nodes (12): uninstallTarget, collectUninstallTargets(), init(), init(), init(), All(), Runtime, Scope (+4 more)

### Community 35 - "buildSearchQuery"
Cohesion: 0.20
Nodes (15): authorQualifier(), buildSearchQuery(), safeQualifierValue(), searchQualifierValue(), TestAuthorMatches(), TestAuthorQualifier(), TestBuildSearchQuery(), TestBuildSearchQueryEmail() (+7 more)

### Community 36 - "getcommits_test.go"
Cohesion: 0.22
Nodes (15): net/http/httptest.Server, firstText(), handler, isZeroResult(), newTestHandler(), TestErrorResult(), TestGetCommitsCollectError(), TestGetCommitsDefaultsDiffsOff() (+7 more)

### Community 37 - "Core Principles"
Cohesion: 0.13
Nodes (14): Core Principles, Development Workflow and Quality Gates, Engineering Constraints, Governance, I. Read-Only by Design (NON-NEGOTIABLE), II. Evidence-Grade, Explainable Output, III. Deterministic, Reconstructible Queries, IV. Explicit Configuration, Dedicated Credentials (+6 more)

### Community 38 - "adapters_error_test.go"
Cohesion: 0.20
Nodes (13): runReadEntryCases(), TestClaudeReadEntry(), TestCodexReadEntry(), TestGrokReadEntry(), TestGrokReadEntryEnabledFlag(), TestOpencodeCheckJsoncNonJSON(), TestOpencodeJsoncRefusal(), TestOpencodeReadEntry() (+5 more)

### Community 39 - "codexAdapter"
Cohesion: 0.20
Nodes (6): decodeTOMLInto(), Scope, readTOMLDoc(), tomlTableAt(), codexAdapter, codexServer

### Community 40 - "swapCollectActivity"
Cohesion: 0.33
Nodes (12): activityHandler(), handler, swapCollectActivity(), TestGetRepoActivityBudgetStopIsSuccess(), TestGetRepoActivityCollectErrorIsTheErrorValue(), TestGetRepoActivityDiffsDefaultOff(), TestGetRepoActivityExplicitZeroMaxRequests(), TestGetRepoActivityMapsInput() (+4 more)

### Community 41 - "Quickstart: Validating Repository Activity Digest"
Cohesion: 0.14
Nodes (14): 10. MCP surface (FR-026, FR-026a), 11. Determinism (FR-023, SC-009), 1. Offline gates (run first — these must pass before any live check), 2. The cost promise (SC-001, SC-002, FR-006, FR-007), 3. Repository activity, live (User Story 1, FR-001 – FR-005), 4. Boundary correctness (FR-008, FR-008a — the off-by-one), 5. Cost visibility and budgeting (User Story 2, FR-011 – FR-016), 6. Honest attribution (User Story 3, FR-017) (+6 more)

### Community 42 - "install.go"
Cohesion: 0.19
Nodes (12): TestDesiredInstallEntry(), TestParseInstallScope(), TestPrintClaudePermissionsBlock(), TestPrintManualSnippets(), TestRuntimeSelection(), addRuntimeFlags(), desiredInstallEntry(), init() (+4 more)

### Community 43 - "Feature Specification: [FEATURE NAME]"
Cohesion: 0.15
Nodes (12): Assumptions, Edge Cases, Feature Specification: [FEATURE NAME], Functional Requirements, Key Entities *(include if feature involves data)*, Measurable Outcomes, Requirements *(mandatory)*, Success Criteria *(mandatory)* (+4 more)

### Community 44 - "Implementation Plan: Distribution Channel Conformance"
Cohesion: 0.15
Nodes (13): Complexity Tracking, Constitution Check, Deferred, and why, Documentation (this feature), Implementation Plan: Distribution Channel Conformance, Implementation sequencing, Phase 0: Research, Phase 1: Design & Contracts (+5 more)

### Community 46 - "Phase 1 Data Model: Repository Activity Digest"
Cohesion: 0.17
Nodes (12): ActivityCommit, ActivityQuery, ActivityResult, Boundaries, ChangeSet and ChangedPath, Correlation, CostReport, Disclosure (+4 more)

### Community 47 - "Phase 0 Research: Repository Activity Digest"
Cohesion: 0.17
Nodes (12): Phase 0 Research: Repository Activity Digest, R10 — Testing strategy: fixtures for shape, a fake transport for cost, R1 — The comparison base costs zero extra requests, R2 — Window filtering is committer-date-based upstream; sting reports author date, R3 — Request accounting and ceiling enforcement live in an `http.RoundTripper`, R4 — An exact cost estimate costs exactly one request, R5 — The default ceiling is 500 requests, R6 — `ReadOnlyTools()` becomes derived, not hand-maintained (+4 more)

### Community 48 - "Repository Guidelines"
Cohesion: 0.18
Nodes (11): Agent Resources, Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Release and compliance tooling (+3 more)

### Community 49 - "budgetedQueryClient"
Cohesion: 0.47
Nodes (9): authorQuery(), budgetedQueryClient(), Client, newQueryTransport(), TestDefaultCeilingDoesNotClipDefaultConfiguration(), TestQueryCeilingCoversEnrichment(), TestQueryCeilingStopsEachScope(), TestQueryUncappedCeilingIsStillAccounted() (+1 more)

### Community 50 - "grokAdapter"
Cohesion: 0.22
Nodes (5): Scope, grokDir(), init(), grokAdapter, grokServer

### Community 51 - "Snippet"
Cohesion: 0.31
Nodes (9): ClaudePermissionsSnippet(), ClaudePermissionToolName(), TestClaudePermissionsSnippetSorted(), TestClaudePermissionToolName(), TestSnippetPerRuntime(), TestSnippetUnknownRuntime(), renderJSON(), renderTOML() (+1 more)

### Community 52 - "model/activity_test.go"
Cohesion: 0.18
Nodes (10): TestActivityCommitOmitEmpty(), TestActivityCommitSummary(), TestActivityResultJSONRoundTrip(), TestActivityResultOmitEmpty(), TestActivitySchemaVersion(), TestChangedPathOmitEmpty(), TestDefaultMaxRequests(), TestDisclosureKindsDistinct() (+2 more)

### Community 53 - "Core Principles"
Cohesion: 0.18
Nodes (10): Core Principles, Governance, [PRINCIPLE_1_NAME], [PRINCIPLE_2_NAME], [PRINCIPLE_3_NAME], [PRINCIPLE_4_NAME], [PRINCIPLE_5_NAME], [PROJECT_NAME] Constitution (+2 more)

### Community 55 - "GitHub Copilot Instructions for sting"
Cohesion: 0.20
Nodes (9): Codebase Shape, Commit and Branch Guidance, GitHub Copilot Instructions for sting, Go and Repository Conventions, Pull Request Instructions, Safety Rules (read-only by design), Testing Expectations, What Good Changes Look Like (+1 more)

### Community 56 - "11. No self-update subcommand (deviation from DECISIONS/0001)"
Cohesion: 0.22
Nodes (9): 11. No self-update subcommand (deviation from DECISIONS/0001), Consequences, Context, Decision, Follow-up, Negative, Positive, Status (+1 more)

### Community 57 - "estimateServer"
Cohesion: 0.39
Nodes (8): estimateServer(), TestEstimateActivityCountsItsOwnProbe(), TestEstimateActivityEmptyWindow(), TestEstimateActivityEnrichmentClippedToCommitCount(), TestEstimateActivityExactCount(), TestEstimateActivityIncludesEnrichmentAndRefLookup(), TestEstimateActivityInvalidRepo(), TestEstimateActivitySinglePage()

### Community 58 - "runAuthGitLab"
Cohesion: 0.28
Nodes (8): addAuthGitLabFlags(), fetchGitLabUsername(), init(), runAuthGitLab(), TestFetchGitLabUsername(), TestRunAuthGitLab_SelfHostedRequiresOwnApp(), TestRunAuthGitLab_WithToken(), TestRunAuthGitLab_WithToken_EmptyInput()

### Community 59 - "server.json"
Cohesion: 0.22
Nodes (8): description, name, packages, repository, source, url, $schema, version

### Community 60 - "Implementation Plan: [FEATURE]"
Cohesion: 0.22
Nodes (8): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Plan: [FEATURE], Project Structure, Source Code (repository root), Summary, Technical Context

### Community 62 - "Implementation Plan: Repository Activity Digest"
Cohesion: 0.22
Nodes (9): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Plan: Repository Activity Digest, Phase Status, Project Structure, Source Code (repository root), Summary (+1 more)

### Community 63 - "Quickstart: Validating Distribution Channel Conformance"
Cohesion: 0.22
Nodes (9): Cross-cutting — release coherence, Prerequisites, Quickstart: Validating Distribution Channel Conformance, Traceability, US1 — Version identity (P1), US2 — `sting update` (P2) — NOT SHIPPED, US3 — Linux packages (P3), US4 — MCP registry entry (P4) (+1 more)

### Community 64 - "Phase 0 Research: Distribution Channel Conformance"
Cohesion: 0.22
Nodes (9): 1. Version identity without release-time stamping, 2. In-process Sigstore verification, 3. Multi-arch container image inside the single release invocation, 4. MCP registry entry and the `io.skaphos` namespace, 5. Install provenance detection, 6. Release supply-chain coverage for the new artifacts, 7. Post-release verification, Phase 0 Research: Distribution Channel Conformance (+1 more)

### Community 65 - "Contributing Guidelines"
Cohesion: 0.25
Nodes (8): Branching and Commits, Coding Standards, Contributing Guidelines, Development Setup, Pull Requests, Release Process, Safety Expectations, Testing

### Community 66 - "runtime_more_test.go"
Cohesion: 0.32
Nodes (7): names(), TestAllSortedAndComplete(), TestByNameHitAndMiss(), TestSelectionFromFlags(), TestSelectionResolveEmptyDetect(), TestSelectionResolveExplicit(), TestSelectionResolveExplicitUnknown()

### Community 67 - "Contract: `sting activity` CLI Command"
Cohesion: 0.25
Nodes (8): Contract: `sting activity` CLI Command, Estimate mode, Exit codes, Flags, Output, Synopsis, Testing notes, Worked example

### Community 68 - "Contract: Go Public API"
Cohesion: 0.25
Nodes (8): Added: `config` package, Added: `ghclient` package, Added: `model` package, Behavior change to `Collect` (User Story 4), Compatibility guarantee, Contract: Go Public API, Error contract for `ResolveActivity`, Invariants for implementation and review

### Community 69 - "Contract: Release artifact set and channel verification"
Cohesion: 0.25
Nodes (8): Artifact set per release, Container image, Contract: Release artifact set and channel verification, Credential handling — a change to existing behavior, Linux packages, Post-release verification, Single-invocation rule, Supply-chain coverage

### Community 70 - "Data Model: Distribution Channel Conformance"
Cohesion: 0.25
Nodes (8): 1. Version identity, 2. Install provenance, 3. Update plan and outcome, 4. Verification material, 5. Server description (`server.json`), 6. Release artifact set, Data Model: Distribution Channel Conformance, Testability seams

### Community 71 - "writeInstallListTable"
Cohesion: 0.38
Nodes (5): listRow, TestDash(), TestWriteInstallListTable(), dash(), writeInstallListTable()

### Community 72 - "config/activity_test.go"
Cohesion: 0.52
Nodes (6): boolPtr(), fixedNow(), intPtr(), TestResolveActivity(), TestResolveActivityDeterministic(), TestResolveActivityNormalizesToUTC()

### Community 73 - "1. Deliver MCP server and CLI from one binary"
Cohesion: 0.29
Nodes (6): 1. Deliver MCP server and CLI from one binary, Alternatives Considered, Consequences, Context, Decision, Status

### Community 74 - "3. Multi-runtime MCP installer and read-only safety model"
Cohesion: 0.29
Nodes (6): 3. Multi-runtime MCP installer and read-only safety model, Alternatives Considered, Consequences, Context, Decision, Status

### Community 75 - "6. GitLab provider support"
Cohesion: 0.29
Nodes (6): 6. GitLab provider support, Alternatives Considered, Consequences, Context, Decision, Status

### Community 76 - "8. OAuth App authentication and multi-provider credential storage"
Cohesion: 0.29
Nodes (7): 8. OAuth App authentication and multi-provider credential storage, Alternatives Considered, Consequences, Context, Decision, References, Status

### Community 77 - "opencode.go"
Cohesion: 0.29
Nodes (4): Scope, init(), opencodeDir(), opencodeServer

### Community 78 - "Specification Quality Checklist: Repository Activity Digest"
Cohesion: 0.29
Nodes (6): Constitution Alignment, Content Quality, Feature Readiness, Requirement Completeness, Specification Quality Checklist: Repository Activity Digest, Validation Notes

### Community 79 - "Specification Quality Checklist: Distribution Channel Conformance"
Cohesion: 0.29
Nodes (6): Content Quality, Feature Readiness, Notes, Re-validation after clarification session 2026-07-25, Requirement Completeness, Specification Quality Checklist: Distribution Channel Conformance

### Community 80 - "Contract: `server.json` — MCP registry entry"
Cohesion: 0.29
Nodes (7): Capability drift, Contract: `server.json` — MCP registry entry, Field rules, Identity, Publishing, Shape, Validation

### Community 81 - "2. Dedicated GitHub PAT via viper, separate from GITHUB_TOKEN"
Cohesion: 0.33
Nodes (6): 2. Dedicated GitHub PAT via viper, separate from GITHUB_TOKEN, Alternatives Considered, Consequences, Context, Decision, Status

### Community 82 - "4. Public packages and Wake evidence shape"
Cohesion: 0.33
Nodes (6): 4. Public packages and Wake evidence shape, Alternatives Considered, Consequences, Context, Decision, Status

### Community 83 - "5. Release Please owns release notes"
Cohesion: 0.33
Nodes (6): 5. Release Please owns release notes, Alternatives Considered, Consequences, Context, Decision, Status

### Community 84 - "7. Commit file and diff evidence"
Cohesion: 0.33
Nodes (5): 7. Commit file and diff evidence, Consequences, Context, Decision, Status

### Community 85 - "10. Multi-tool MCP server"
Cohesion: 0.33
Nodes (5): 10. Multi-tool MCP server, Consequences, Context, Decision, Status

### Community 86 - "keyring_test.go"
Cohesion: 0.53
Nodes (5): MockInit(), MockInitWithError(), TestGetNotFound(), TestMockInitWithError(), TestSetGetDelete()

### Community 87 - "Contract: `get_repo_activity` MCP Tool"
Cohesion: 0.33
Nodes (6): Contract: `get_repo_activity` MCP Tool, Error handling, Input schema, Installer impact, Output, Registration

### Community 88 - "Contract: `sting update` — NOT SHIPPED"
Cohesion: 0.33
Nodes (6): Behavioral requirements, Changes to `sting version`, Contract: `sting update` — NOT SHIPPED, Exit codes, Output, Synopsis

### Community 89 - "9. GoReleaser owns the GitHub release (supersedes part of ADR 0005)"
Cohesion: 0.40
Nodes (5): 9. GoReleaser owns the GitHub release (supersedes part of ADR 0005), Consequences, Context, Decision, Status

### Community 90 - "runAuthLogout"
Cohesion: 0.40
Nodes (3): runAuthLogout(), TestRunAuthLogout_Idempotent(), TestRunAuthLogout_SpecificProvider()

### Community 91 - "model_test.go"
Cohesion: 0.40
Nodes (4): TestCommitSummary(), TestProviderValid(), TestSchemaVersion(), TestScopeValid()

### Community 92 - "run_generation"
Cohesion: 0.70
Nodes (4): rewrite_report_rows(), run_generation(), save_exception_license(), generate-notices.sh script

### Community 93 - "[CHECKLIST TYPE] Checklist: [FEATURE NAME]"
Cohesion: 0.40
Nodes (4): [Category 1], [Category 2], [CHECKLIST TYPE] Checklist: [FEATURE NAME], Notes

### Community 97 - "Invoke-NoticeGeneration"
Cohesion: 0.83
Nodes (3): Invoke-NoticeGeneration(), Save-ExceptionLicense(), Update-ReportRows()

## Knowledge Gaps
- **413 isolated node(s):** `common.sh script`, `Config`, `Config`, `gitlabProject`, `github.com/skaphos/sting` (+408 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 489 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **9 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `TestScannerHandlesArrayTables()` connect `tomledit.go` to `testing.T`, `ByName`?**
  _High betweenness centrality (0.022) - this node is a cross-community bridge._
- **Why does `swapCollectActivity()` connect `swapCollectActivity` to `Config`, `context.Context`, `testing.T`, `ActivityResult`?**
  _High betweenness centrality (0.020) - this node is a cross-community bridge._
- **Why does `newActivityCmd()` connect `newActivityCmd` to `testing.T`, `newCmd`, `cli_test.go`, `github.com/spf13/cobra.Command`?**
  _High betweenness centrality (0.016) - this node is a cross-community bridge._
- **Are the 19 inferred relationships involving `Default()` (e.g. with `TestValidateGitLabSearchScopeIncompatible()` and `TestValidateMaxCommits()`) actually correct?**
  _`Default()` has 19 INFERRED edges - model-reasoned connections that need verification._
- **Are the 4 inferred relationships involving `newCmd()` (e.g. with `newActivityCmd()` and `TestRunInstallAggregatesErrors()`) actually correct?**
  _`newCmd()` has 4 INFERRED edges - model-reasoned connections that need verification._
- **Are the 22 inferred relationships involving `activityQuery()` (e.g. with `TestCollectActivityBudgetStopImmediately()` and `TestCollectActivityBudgetStopReturnsPartialResult()`) actually correct?**
  _`activityQuery()` has 22 INFERRED edges - model-reasoned connections that need verification._
- **Are the 23 inferred relationships involving `newTestClient()` (e.g. with `TestCollectActivityAuthorFilterDisclosed()` and `TestCollectActivityDeterministic()`) actually correct?**
  _`newTestClient()` has 23 INFERRED edges - model-reasoned connections that need verification._