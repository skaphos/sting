# Feature Specification: Credential Source Visibility

**Feature Branch**: `feature/45-credential-source-visibility`

**Created**: 2026-10-11

**Status**: Implemented

**Input**: User description: "Credential source visibility (scoped slice of GitHub issue skaphos/sting#45). Surface the selected credential source + provider + host (never the secret) on stderr for CLI commands and every MCP tool call, at an opt-in verbosity so default output is unchanged; document MCP-server mixed-auth precedence. Out of scope: TokenSource/Authenticator interface refactor, OAuth token refresh/expiry handling."

**Source**: [Issue #45 — Token resolution follow-ups](https://github.com/skaphos/sting/issues/45),
the "active-credential logging" and "document MCP-server mixed-auth behavior" items. The
`TokenSource` interface and token refresh/expiry items stay out of scope and are tracked
separately.

## Problem

For every query, sting picks one credential per provider and host using the "most explicit wins"
precedence settled in [ADR 0008](../../docs/adr/0008-oauth-app-authentication.md):

1. the dedicated PAT (`--token` / `STING_TOKEN` / config `token`; the GitLab equivalents);
2. the OAuth token saved by `sting auth <provider>` for that host (system keyring or
   sting's credential file);
3. anonymous.

By default, ambient provider tokens (`GITHUB_TOKEN`, `GITLAB_TOKEN`, `gh`/`glab` stored auth)
are not consulted ([ADR 0002](../../docs/adr/0002-dedicated-pat-via-viper.md)). One exception
exists, and it is opt-in: when `STING_ALLOW_AMBIENT_GITHUB_TOKEN` is set, the credential layer
falls back to the ambient GitHub token in place of a stored sting credential. Today that switch
is invisible. The README does not document it, and this feature only reports it, without
changing it.

That choice is never reported. `sting auth status` lists the credentials that are *stored*, but not
the one a given run actually *used*. The failure that matters most is a silent drop to
anonymous, which can happen when an OAuth login targets a different host, the keyring is
unavailable, or a stale PAT shadows a fresh OAuth login. The query still succeeds, but private
repositories are missing from the result, and the user has no signal to explain the gap. The
problem is worse inside an MCP runtime: the server's environment is configured by the installer,
not by the user's shell, so the user cannot inspect which credential the server holds.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See which credential a CLI run used (Priority: P1)

A developer runs `sting query` (or `activity`, `prs`, `inbox`) and gets fewer results than
expected. They re-run with verbose output turned on. sting prints which kind of credential it
selected, plus the provider and host, so the developer can tell whether the run was
authenticated, and how.

**Why this priority**: This is the core diagnostic gap from issue #45, and the cheapest way to
explain a silently incomplete result.

**Independent Test**: Run each CLI query command with verbose output enabled under each
credential situation (dedicated PAT set; only an OAuth login stored; nothing configured).
Confirm that one line on stderr names the expected source, provider, and host, and that stdout
is byte-identical to a non-verbose run.

**Acceptance Scenarios**:

1. **Given** a dedicated GitHub PAT is configured and an OAuth login also exists for
   github.com, **When** the user runs `sting query` with verbose output, **Then** stderr reports
   the dedicated PAT as the source for GitHub at `github.com`.
2. **Given** no PAT is configured and an OAuth login for github.com is stored in the system
   keyring, **When** the user runs a query with verbose output, **Then** stderr reports the
   stored OAuth credential from the keyring.
3. **Given** no PAT is configured and the OAuth login is stored in sting's credential file,
   **When** the user runs a query with verbose output, **Then** stderr reports the stored OAuth
   credential from the file.
4. **Given** no credential of any kind exists for the target host, **When** the user runs a
   query with verbose output, **Then** stderr reports anonymous access for that provider and
   host.
5. **Given** the user targets a self-hosted instance (GHES base URL or self-managed GitLab base
   URL), **When** they run a query with verbose output, **Then** the reported host is that
   instance's hostname, not the public default.
6. **Given** verbose output is *not* enabled, **When** the user runs any query command,
   **Then** stdout and stderr are exactly what they were before this feature.

---

### User Story 2 - See which credential an MCP tool call used (Priority: P2)

A user has sting registered as an MCP server in an agent runtime. An agent's `get_commits` call
comes back without private-repository commits. The user turns on verbose output in the server's
configuration and inspects the runtime's MCP server log. For each tool call, they find which
credential source, provider, and host the server used.

**Why this priority**: The MCP path is sting's main use case and is the hardest place to inspect
credentials. It depends on the same capability as Story 1, applied to a long-running server.

**Independent Test**: Start the MCP server with verbose output enabled under a known credential
situation, invoke each of the four tools, and confirm one stderr line per call names the source.
Confirm that the stdio protocol stream is unaffected.

**Acceptance Scenarios**:

1. **Given** the MCP server runs with verbose output enabled, **When** any of `get_commits`,
   `get_repo_activity`, `get_prs`, or `get_pr_inbox` is called, **Then** the server writes the
   selected credential source, provider, and host to stderr for that call.
2. **Given** the MCP server runs with verbose output enabled, **When** tools are called, **Then**
   no diagnostic text appears on stdout, and the protocol exchange is unchanged.
3. **Given** the MCP server runs without verbose output, **When** tools are called, **Then**
   nothing new is written to stderr.

---

### User Story 3 - Understand MCP mixed-auth precedence from the docs (Priority: P3)

A user who has run `sting auth github` and also has `STING_TOKEN` forwarded into their agent
runtime wants to know which credential the MCP tools will use, and how to make the server use
their OAuth login instead.

**Why this priority**: Documentation only. It prevents the confusion that Stories 1 and 2
diagnose, but it changes no behavior.

**Independent Test**: A reader can answer "which credential does the MCP server use when both a
PAT and an OAuth login exist?" and "how do I make it use OAuth?" from the README alone.

**Acceptance Scenarios**:

1. **Given** the README's agent-integration section, **When** a reader looks up credential
   resolution for the MCP server, **Then** it states that every tool uses the same precedence as
   the CLI (dedicated PAT, then stored OAuth login for the target host, then anonymous) and that
   ambient provider tokens are not used by default. They are used only under the existing
   `STING_ALLOW_AMBIENT_GITHUB_TOKEN` opt-in, which the README must name (consistent with
   FR-002 and FR-010).
2. **Given** the same section, **When** a reader looks up server-wide PATs, **Then** it explains
   that a PAT reaching the server through the runtime's environment (as the installer
   configures) or through sting's config file applies to every tool call, and overrides the
   user's OAuth login for that provider. It also explains that leaving the variable unset or
   empty lets the OAuth login apply.
3. **Given** the same section, **When** a reader wants to confirm which credential a running
   server uses, **Then** it points to the verbose output from Story 2.

### Edge Cases

- **Stored-credential lookup fails** (keyring locked or unavailable, credential file
  unreadable): resolution behaves exactly as today and falls back to the next source. With
  verbose output on, the report says that the stored-credential lookup failed, as distinct from
  "no stored credential", so a silent anonymous fallback is explained. The failure reason must
  not include secret material.
- **Empty PAT value** (for example `${STING_TOKEN:-}` expanding to an empty string in a runtime
  config): this counts as "not configured", as it does today, and the report names whichever
  source was actually used.
- **One run resolves more than one provider or host**: each resolution is reported once, so the
  number of lines matches the number of credentials resolved.
- **Credential resolution happens before the run fails** (for example, invalid query flags
  reject the request): no report is required for a resolution that never happened. A report that
  was already written stays accurate.
- **Self-hosted base URL with an API path** (for example `https://ghe.example.com/api/v3`): the
  reported host is the bare hostname used as the credential-storage key.
- **Ambient opt-in enabled** (`STING_ALLOW_AMBIENT_GITHUB_TOKEN` set): the credential layer
  already falls back to `GH_TOKEN` / `GITHUB_TOKEN` or the `gh` config file when no sting
  credential is stored. When that happens, the report MUST say so explicitly. This is exactly
  the kind of identity switch the report exists to expose.
- **Secrets**: no token value, prefix, suffix, length, or hash appears in any report.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: sting MUST be able to report, for each credential resolution, the selected source,
  the provider, and the host the credential was resolved for.
- **FR-002**: The reported source MUST be exactly one of: dedicated PAT; stored credential from
  the system keyring; stored credential from sting's credential file; ambient GitHub token
  (opt-in, from the environment or the `gh` config file); anonymous. A stored credential MUST
  also report its kind (OAuth token or PAT), because `sting auth gitlab --with-token` stores a PAT.
  The ambient source MUST be labelled as opt-in, so it is never mistaken for a sting-owned
  credential.
- **FR-003**: Reporting MUST be off by default and enabled by an explicit verbosity setting.
  With it off, sting's output on every stream MUST be unchanged from current behavior.
- **FR-004**: The verbosity setting MUST be controllable through sting's normal configuration
  precedence (flag, `STING_`-prefixed environment variable, config file), so it can be enabled
  for an MCP server whose arguments and environment are fixed by an agent runtime's
  configuration.
- **FR-005**: Reports MUST be written to stderr only. They MUST NOT change stdout, the JSON or
  Markdown evidence, the structured result, or the MCP stdio protocol stream.
- **FR-006**: Reporting MUST cover the CLI commands `query` (including the root command's query
  form), `activity`, `prs`, and `inbox`, and the MCP tools `get_commits`, `get_repo_activity`,
  `get_prs`, and `get_pr_inbox`.
- **FR-007**: In the MCP server, reporting MUST happen per tool call, so each call's credential
  is visible.
- **FR-008**: When a stored-credential lookup fails for a reason other than "no credential
  stored", the report MUST say that the lookup failed before naming the source that was
  actually used.
- **FR-009**: Reports MUST NOT contain any secret material or anything derived from it.
- **FR-010**: This feature MUST NOT change which credential is selected. The ADR 0008
  precedence, the ADR 0002 default exclusion of ambient tokens, and the existing
  `STING_ALLOW_AMBIENT_GITHUB_TOKEN` opt-in all stay exactly as they are.
- **FR-011**: The README MUST document MCP-server credential precedence, how server-wide PATs
  (runtime environment or config file) interact with a stored OAuth login, how to make the
  server use the OAuth login, and how to enable verbose output for the server.
- **FR-012**: The README MUST document the verbosity setting and the meaning of each reported
  source.

### Key Entities

- **Credential resolution report**: A diagnostic record of one credential selection. It holds
  the provider (GitHub or GitLab), the host, the selected source (one of the FR-002 values),
  the stored credential's kind where applicable, and,
  where it applies, an indication that a stored-credential lookup failed. It never includes the
  credential itself, and it is never part of the evidence result.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For every credential situation (dedicated PAT, keyring-stored, file-stored, opt-in
  ambient, none), a user
  can name the credential a run used from one verbose run, without reading source code or
  `auth status`.
- **SC-002**: With verbose output off, every existing CLI and MCP test's output is unchanged
  (zero output regressions).
- **SC-003**: With verbose output on, stdout and the MCP protocol stream are byte-identical to a
  non-verbose run of the same request.
- **SC-004**: No report produced in testing contains any part of a configured or stored token
  value.
- **SC-005**: All 8 query surfaces (4 CLI commands, 4 MCP tools) produce a report when verbose
  output is on.
- **SC-006**: A reader can answer both MCP mixed-auth questions in User Story 3 from the README
  alone.

## Assumptions

- "Dedicated PAT" is reported without saying whether it came from a flag, the environment, or
  the config file. sting's configuration layer merges these into one value, and ADR 0008 treats
  them as one precedence tier. Telling them apart would be a separate enhancement.
- The report is a diagnostic aid, not evidence. Adding the credential source to the structured
  result would change the evidence contract (and possibly `model.SchemaVersion`), so it is a
  separate decision outside this feature.
- MCP runtimes capture or show a server's stderr in their own logs. sting does not add its own
  log file or log destination.
- One verbosity level (on/off) is enough for this feature. Finer-grained levels are not needed
  until there is more diagnostic output to tier.
- The `TokenSource`/`Authenticator` interface refactor and OAuth token refresh/expiry handling
  (the rest of issue #45) are out of scope. This feature must not block either one.
