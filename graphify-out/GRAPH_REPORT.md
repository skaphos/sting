# Graph Report - sting  (2026-07-25)

## Corpus Check
- 133 files · ~104,599 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 1229 nodes · 2580 edges · 68 communities (55 shown, 13 thin omitted)
- Extraction: 81% EXTRACTED · 19% INFERRED · 0% AMBIGUOUS · INFERRED: 500 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `7a1783aa`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- contains
- Default
- ByName
- Query
- credentials_test.go
- grokAdapter
- Tasks: [FEATURE NAME]
- speckit-analyze/SKILL.md
- sting
- .String
- Open Design Questions — **LOCKED DECISIONS** (2026-05-30)
- main (sting entrypoint)
- common.sh
- render.Render
- getcommits_test.go
- collect_test.go
- Registering OAuth Apps for Sting
- init.go
- newTestClient
- Changelog
- Execution Steps
- Core Principles
- Feature Specification: [FEATURE NAME]
- speckit-plan/SKILL.md
- 8. OAuth App authentication and multi-provider credential storage
- 4. Public packages and Wake evidence shape
- 6. GitLab provider support
- 5. Release Please owns release notes
- 2. Dedicated GitHub PAT via viper, separate from GITHUB_TOKEN
- 3. Multi-runtime MCP installer and read-only safety model
- 1. Deliver MCP server and CLI from one binary
- speckit-specify/SKILL.md
- speckit-tasks/SKILL.md
- keyring_test.go
- keyring.go
- 7. Commit file and diff evidence
- 9. GoReleaser owns the GitHub release (supersedes part of ADR 0005)
- Core Principles
- Regenerating notices
- runtime.go
- adr/README.md
- config.Default (Config)
- GitHub Copilot Instructions for sting
- Repository Guidelines
- create-new-feature.sh
- Implementation Plan: [FEATURE]
- mcpinstall.Entry
- mcpinstall.Scope
- mcpinstall.Selection
- speckit-checklist/SKILL.md
- Contributing Guidelines
- speckit-clarify/SKILL.md
- speckit-implement/SKILL.md
- speckit-constitution/SKILL.md
- speckit-taskstoissues/SKILL.md
- [CHECKLIST TYPE] Checklist: [FEATURE NAME]
- check-coverage.sh
- run_generation
- check-prerequisites.sh
- setup-plan.sh
- setup-tasks.sh
- github.com/skaphos/sting
- github.com/skaphos/sting/tools

## God Nodes (most connected - your core abstractions)
1. `contains()` - 99 edges
2. `newCmd()` - 42 edges
3. `ByName` - 37 edges
4. `Default()` - 36 edges
5. `isolateHome()` - 36 edges
6. `readFile()` - 31 edges
7. `WithFilePath()` - 28 edges
8. `Query` - 27 edges
9. `Commit` - 24 edges
10. `isolateHome()` - 23 edges

## Surprising Connections (you probably didn't know these)
- `ADR 0003: Multi-runtime installer and read-only safety` --rationale_for--> `cli.runInstall`  [INFERRED]
  docs/adr/0003-multi-runtime-installer-and-readonly-safety.md → internal/cli/install.go
- `Read-only claim from single source (ReadOnlyTools)` --rationale_for--> `cli.printClaudePermissionsBlock`  [INFERRED]
  docs/adr/0003-multi-runtime-installer-and-readonly-safety.md → internal/cli/install.go
- `ADR 0002: Dedicated PAT via viper` --rationale_for--> `cli.initConfig`  [INFERRED]
  docs/adr/0002-dedicated-pat-via-viper.md → internal/cli/root.go
- `sting Taskfile` --references--> `cli.Execute`  [INFERRED]
  Taskfile.yml → internal/cli/root.go
- `ADR 0001: MCP and CLI from one binary` --rationale_for--> `cli.mcpCmd`  [INFERRED]
  docs/adr/0001-mcp-and-cli-from-one-binary.md → internal/cli/mcp.go

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **MCP install runtime adapters implement Runtime** — mcpinstall_claudeadapter, mcpinstall_codexadapter, mcpinstall_opencodeadapter, mcpinstall_grokadapter, internal_mcpinstall_runtime_runtime [EXTRACTED 0.95]
- **get_commits tool request flow** — internal_mcpserver_server_getcommits, internal_config_resolve_resolve, internal_ghclient_ghclient_collect, internal_config_resolve_request [EXTRACTED 0.90]
- **Codex and Grok share TOML doc helpers** — internal_mcpinstall_codex_readtomldoc, internal_mcpinstall_codex_writetomldoc, internal_mcpinstall_codex_tomltableat, mcpinstall_grokadapter [EXTRACTED 0.85]
- **Cobra command tree assembly** — internal_cli_root_rootcmd, internal_cli_query_runquery, internal_cli_mcp_mcpcmd, internal_cli_install_installcmd, internal_cli_uninstall_uninstallcmd [EXTRACTED 0.90]
- **Result rendering flow** — internal_model_model_result, internal_render_render, internal_render_render_tomarkdown, internal_render_render_tojson [EXTRACTED 0.85]
- **Read-only safety single source of truth** — adr0003_readonly_single_source, internal_cli_install_printclaudepermissionsblock, changelog_sting [INFERRED 0.75]

## Communities (68 total, 13 thin omitted)

### Community 0 - "contains"
Cohesion: 0.05
Nodes (113): ADR 0001: MCP and CLI from one binary, Buffer, listRow, Command, runAuthLogout(), Command, runAuthStatus(), contains() (+105 more)

### Community 1 - "Default"
Cohesion: 0.07
Nodes (55): Client, Default(), Defaults(), Config, Provider, Scope, Time, ParseTime() (+47 more)

### Community 2 - "ByName"
Cohesion: 0.09
Nodes (63): T, runReadEntryCases(), TestClaudeReadEntry(), TestCodexReadEntry(), TestGrokReadEntry(), TestGrokReadEntryEnabledFlag(), TestOpencodeCheckJsoncNonJSON(), TestOpencodeJsoncRefusal() (+55 more)

### Community 3 - "Query"
Cohesion: 0.06
Nodes (62): CommitFile, CommitResult, Client, apiError(), authorEmail(), authorMatches(), authorQualifier(), buildSearchQuery() (+54 more)

### Community 4 - "credentials_test.go"
Cohesion: 0.06
Nodes (65): CredentialRef, defaultKeyring, failingKeyring, hostsFile, KeyringBackend, lockedKeyring, Provider, Source (+57 more)

### Community 5 - "grokAdapter"
Cohesion: 0.07
Nodes (42): decodeJSONInto, FileMode, Scope, init(), jsonObjectAt, readJSONDoc, writeJSONDoc, decodeTOMLInto (+34 more)

### Community 6 - "Tasks: [FEATURE NAME]"
Cohesion: 0.07
Nodes (26): Dependencies & Execution Order, Format: `[ID] [P?] [Story] Description`, Implementation for User Story 1, Implementation for User Story 2, Implementation for User Story 3, Implementation Strategy, Incremental Delivery, MVP First (User Story 1 Only) (+18 more)

### Community 7 - "speckit-analyze/SKILL.md"
Cohesion: 0.08
Nodes (25): 1. Initialize Analysis Context, 2. Load Artifacts (Progressive Disclosure), 3. Build Semantic Models, 4. Detection Passes (Token-Efficient Analysis), 5. Severity Assignment, 6. Produce Compact Analysis Report, 7. Provide Next Actions, 8. Offer Remediation (+17 more)

### Community 8 - "sting"
Cohesion: 0.06
Nodes (41): ADR 0002: Dedicated PAT via viper, GITHUB_TOKEN intentionally not read, ADR 0003: Multi-runtime installer and read-only safety, Read-only claim from single source (ReadOnlyTools), sting Changelog, sting config.example.yaml, ADR index, Agent integration (the main use case) (+33 more)

### Community 9 - ".String"
Cohesion: 0.15
Nodes (30): FileMode, T, TestResolveTargetDanglingSymlink(), TestResolveTargetMissing(), TestWriteAtomicNewFile(), TestWriteAtomicOverwritePreservesMode(), TestWriteAtomicParentMissing(), TestWriteAtomicRenameOntoDir() (+22 more)

### Community 10 - "Open Design Questions — **LOCKED DECISIONS** (2026-05-30)"
Cohesion: 0.10
Nodes (19): 1. File format for plaintext fallback — **LOCKED**, 2. Keyring library choice — **LOCKED**, 3. Migration from existing config — **LOCKED**, 4. Per-user vs per-host for GHES — **LOCKED**, 5. Environment variable precedence — **LOCKED**, 6. "Bring your own OAuth client" configuration — **LOCKED (keys TBD)**, Current Usage, Future Opportunities (When Implementing `auth` Login Commands) (+11 more)

### Community 11 - "main (sting entrypoint)"
Cohesion: 0.40
Nodes (3): main (sting entrypoint), T, TestMain_Version()

### Community 12 - "common.sh"
Cohesion: 0.13
Nodes (5): get_feature_paths(), get_repo_root(), _persist_feature_json(), resolve_specify_init_dir(), common.sh script

### Community 13 - "render.Render"
Cohesion: 0.12
Nodes (36): Shared dependency-light core (model/config/ghclient/render), Builder, hasPrefix(), model.Commit, model.Commit.Summary, model.Query, model.Result, model.Scope (+28 more)

### Community 14 - "getcommits_test.go"
Cohesion: 0.06
Nodes (52): CallToolRequest, config.Config, ParseTime, ParseWindow, Dedicated STING_TOKEN separate from GITHUB_TOKEN, Config.Validate, config.Request, Config.Resolve (+44 more)

### Community 15 - "collect_test.go"
Cohesion: 0.11
Nodes (38): ErrorResponse, Client, T, newTestClient(), TestCollectExactMaxCommitsNotTruncated(), TestCollectIncludeFilesAndDiffs(), TestCollectIncludeStats(), TestCollectMaxCommitsTruncation() (+30 more)

### Community 16 - "Registering OAuth Apps for Sting"
Cohesion: 0.11
Nodes (19): code:bash (sting auth gitlab --client-id <YOUR_APPLICATION_ID>), code:bash (# Read the token from a file so it never lands in shell hist), GitHub (github.com and GitHub Enterprise Server), GitLab (gitlab.com and self-hosted), Migration and Fallbacks, Next Steps / Status, Public App vs. Your Own App: Trust and Governance, Recommended Settings (+11 more)

### Community 17 - "init.go"
Cohesion: 0.14
Nodes (27): addAuthGitHubFlags(), Command, init(), runAuthGitHub(), addAuthGitLabFlags(), fetchGitLabUsername(), Command, init() (+19 more)

### Community 18 - "newTestClient"
Cohesion: 0.18
Nodes (25): gitlabCommitsBodyN(), T, isDiffPath(), TestCollectGroupScopeDiffs(), TestEnrichDiffsAbortsOnError(), TestEnrichDiffsConcurrent(), New(), skipProjectReason() (+17 more)

### Community 19 - "Changelog"
Cohesion: 0.09
Nodes (22): [0.0.2](https://github.com/skaphos/sting/compare/v0.0.1...v0.0.2) (2026-05-30), [0.0.3](https://github.com/skaphos/sting/compare/v0.0.2...v0.0.3) (2026-05-31), [0.0.4](https://github.com/skaphos/sting/compare/v0.0.3...v0.0.4) (2026-06-22), [0.0.5](https://github.com/skaphos/sting/compare/v0.0.4...v0.0.5) (2026-06-30), [0.0.6](https://github.com/skaphos/sting/compare/v0.0.5...v0.0.6) (2026-07-11), [0.0.7](https://github.com/skaphos/sting/compare/v0.0.6...v0.0.7) (2026-07-12), Added, Bug Fixes (+14 more)

### Community 20 - "Execution Steps"
Cohesion: 0.12
Nodes (15): 1. Initialize Convergence Context, 2. Load Artifacts (Progressive Disclosure), 3. Build the Intent Inventory, 4. Assess the Codebase and Classify Findings, 5. Assign Severity, 6. Present the In-Session Findings Summary, 7. Append Convergence Tasks (or report converged), 8. Provide Next Actions (Handoff) (+7 more)

### Community 21 - "Core Principles"
Cohesion: 0.13
Nodes (14): Core Principles, Development Workflow and Quality Gates, Engineering Constraints, Governance, I. Read-Only by Design (NON-NEGOTIABLE), II. Evidence-Grade, Explainable Output, III. Deterministic, Reconstructible Queries, IV. Explicit Configuration, Dedicated Credentials (+6 more)

### Community 22 - "Feature Specification: [FEATURE NAME]"
Cohesion: 0.15
Nodes (12): Assumptions, Edge Cases, Feature Specification: [FEATURE NAME], Functional Requirements, Key Entities *(include if feature involves data)*, Measurable Outcomes, Requirements *(mandatory)*, Success Criteria *(mandatory)* (+4 more)

### Community 23 - "speckit-plan/SKILL.md"
Cohesion: 0.18
Nodes (10): Completion Report, Done When, Key rules, Mandatory Post-Execution Hooks, Outline, Phase 0: Outline & Research, Phase 1: Design & Contracts, Phases (+2 more)

### Community 24 - "8. OAuth App authentication and multi-provider credential storage"
Cohesion: 0.29
Nodes (7): 8. OAuth App authentication and multi-provider credential storage, Alternatives Considered, Consequences, Context, Decision, References, Status

### Community 25 - "4. Public packages and Wake evidence shape"
Cohesion: 0.18
Nodes (6): 4. Public packages and Wake evidence shape, Alternatives Considered, Consequences, Context, Decision, Status

### Community 26 - "6. GitLab provider support"
Cohesion: 0.29
Nodes (6): 6. GitLab provider support, Alternatives Considered, Consequences, Context, Decision, Status

### Community 27 - "5. Release Please owns release notes"
Cohesion: 0.33
Nodes (6): 5. Release Please owns release notes, Alternatives Considered, Consequences, Context, Decision, Status

### Community 28 - "2. Dedicated GitHub PAT via viper, separate from GITHUB_TOKEN"
Cohesion: 0.33
Nodes (6): 2. Dedicated GitHub PAT via viper, separate from GITHUB_TOKEN, Alternatives Considered, Consequences, Context, Decision, Status

### Community 29 - "3. Multi-runtime MCP installer and read-only safety model"
Cohesion: 0.29
Nodes (6): 3. Multi-runtime MCP installer and read-only safety model, Alternatives Considered, Consequences, Context, Decision, Status

### Community 30 - "1. Deliver MCP server and CLI from one binary"
Cohesion: 0.29
Nodes (6): 1. Deliver MCP server and CLI from one binary, Alternatives Considered, Consequences, Context, Decision, Status

### Community 31 - "speckit-specify/SKILL.md"
Cohesion: 0.18
Nodes (10): Completion Report, Done When, For AI Generation, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, Quick Guidelines, Section Requirements (+2 more)

### Community 32 - "speckit-tasks/SKILL.md"
Cohesion: 0.18
Nodes (10): Checklist Format (REQUIRED), Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Phase Structure, Pre-Execution Checks, Task Generation Rules (+2 more)

### Community 33 - "keyring_test.go"
Cohesion: 0.52
Nodes (6): T, MockInit(), MockInitWithError(), TestGetNotFound(), TestMockInitWithError(), TestSetGetDelete()

### Community 35 - "7. Commit file and diff evidence"
Cohesion: 0.33
Nodes (5): 7. Commit file and diff evidence, Consequences, Context, Decision, Status

### Community 36 - "9. GoReleaser owns the GitHub release (supersedes part of ADR 0005)"
Cohesion: 0.40
Nodes (5): 9. GoReleaser owns the GitHub release (supersedes part of ADR 0005), Consequences, Context, Decision, Status

### Community 37 - "Core Principles"
Cohesion: 0.18
Nodes (10): Core Principles, Governance, [PRINCIPLE_1_NAME], [PRINCIPLE_2_NAME], [PRINCIPLE_3_NAME], [PRINCIPLE_4_NAME], [PRINCIPLE_5_NAME], [PROJECT_NAME] Constitution (+2 more)

### Community 38 - "Regenerating notices"
Cohesion: 0.50
Nodes (3): code:bash (go -C tools tool task notices), Regenerating notices, Third-Party Notices

### Community 39 - "runtime.go"
Cohesion: 0.15
Nodes (19): uninstallTarget, cli.collectUninstallTargets, Scope, init(), All, T, names(), TestAllSortedAndComplete() (+11 more)

### Community 44 - "GitHub Copilot Instructions for sting"
Cohesion: 0.20
Nodes (9): Codebase Shape, Commit and Branch Guidance, GitHub Copilot Instructions for sting, Go and Repository Conventions, Pull Request Instructions, Safety Rules (read-only by design), Testing Expectations, What Good Changes Look Like (+1 more)

### Community 45 - "Repository Guidelines"
Cohesion: 0.22
Nodes (9): Build, Test, and Development Commands, Coding Style & Naming Conventions, Commit & Pull Request Guidelines, Documentation Expectations, Engineering Guardrails, Project Structure & Module Organization, Repository Guidelines, Safety Notes (read-only by design) (+1 more)

### Community 46 - "create-new-feature.sh"
Cohesion: 0.31
Nodes (4): get_highest_from_specs(), is_feature_number_in_range(), create-new-feature.sh script, spec_prefix_exists()

### Community 47 - "Implementation Plan: [FEATURE]"
Cohesion: 0.22
Nodes (8): Complexity Tracking, Constitution Check, Documentation (this feature), Implementation Plan: [FEATURE], Project Structure, Source Code (repository root), Summary, Technical Context

### Community 51 - "speckit-checklist/SKILL.md"
Cohesion: 0.25
Nodes (7): Anti-Examples: What NOT To Do, Checklist Purpose: "Unit Tests for English", Example Checklist Types & Sample Items, Execution Steps, Post-Execution Checks, Pre-Execution Checks, User Input

### Community 52 - "Contributing Guidelines"
Cohesion: 0.25
Nodes (8): Branching and Commits, Coding Standards, Contributing Guidelines, Development Setup, Pull Requests, Release Process, Safety Expectations, Testing

### Community 53 - "speckit-clarify/SKILL.md"
Cohesion: 0.29
Nodes (6): Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, User Input

### Community 54 - "speckit-implement/SKILL.md"
Cohesion: 0.29
Nodes (6): Completion Report, Done When, Mandatory Post-Execution Hooks, Outline, Pre-Execution Checks, User Input

### Community 55 - "speckit-constitution/SKILL.md"
Cohesion: 0.33
Nodes (5): Outline, Post-Execution Checks, Pre-Execution Checks, Scope Guard, User Input

### Community 56 - "speckit-taskstoissues/SKILL.md"
Cohesion: 0.40
Nodes (4): Outline, Post-Execution Checks, Pre-Execution Checks, User Input

### Community 57 - "[CHECKLIST TYPE] Checklist: [FEATURE NAME]"
Cohesion: 0.40
Nodes (4): [Category 1], [Category 2], [CHECKLIST TYPE] Checklist: [FEATURE NAME], Notes

## Knowledge Gaps
- **296 isolated node(s):** `check-prerequisites.sh script`, `common.sh script`, `setup-plan.sh script`, `setup-tasks.sh script`, `gitlabProject` (+291 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **13 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `contains()` connect `contains` to `Default`, `ByName`, `Query`, `credentials_test.go`, `grokAdapter`, `render.Render`, `getcommits_test.go`, `collect_test.go`, `newTestClient`?**
  _High betweenness centrality (0.258) - this node is a cross-community bridge._
- **Why does `sting` connect `sting` to `contains`, `4. Public packages and Wake evidence shape`?**
  _High betweenness centrality (0.157) - this node is a cross-community bridge._
- **Why does `readFile()` connect `ByName` to `contains`, `credentials_test.go`, `grokAdapter`, `.String`, `init.go`?**
  _High betweenness centrality (0.069) - this node is a cross-community bridge._
- **Are the 67 inferred relationships involving `contains()` (e.g. with `TestValidateGitLabSearchScopeIncompatible()` and `TestValidateProvider()`) actually correct?**
  _`contains()` has 67 INFERRED edges - model-reasoned connections that need verification._
- **Are the 3 inferred relationships involving `newCmd()` (e.g. with `TestRunInstallAggregatesErrors()` and `TestRunInstallPreservesDisabled()`) actually correct?**
  _`newCmd()` has 3 INFERRED edges - model-reasoned connections that need verification._
- **Are the 33 inferred relationships involving `ByName` (e.g. with `runReadEntryCases()` and `TestGrokReadEntryEnabledFlag()`) actually correct?**
  _`ByName` has 33 INFERRED edges - model-reasoned connections that need verification._
- **Are the 32 inferred relationships involving `Default()` (e.g. with `TestValidateGitLabSearchScopeIncompatible()` and `TestValidateMaxCommits()`) actually correct?**
  _`Default()` has 32 INFERRED edges - model-reasoned connections that need verification._