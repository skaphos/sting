# 8. OAuth App authentication and multi-provider credential storage

## Status

Proposed.

## Context

sting currently authenticates exclusively via long-lived Personal Access Tokens (PATs) configured as `token` / `STING_TOKEN` (GitHub) and `gitlab_token` / `STING_GITLAB_TOKEN` (GitLab). This model is documented and enforced by [ADR 0002](0002-dedicated-pat-via-viper.md): dedicated sting credentials are resolved via viper with strict separation from ambient provider tokens.

PAT-based authentication will continue to be supported, but it will be repositioned as the legacy fallback path once OAuth flows are available.

While this approach has served the initial GitHub-only and early GitLab support phases well, it has known limitations:

- Poor first-time user experience (manual token creation and configuration).
- Weak support for enterprise SSO / GitHub Enterprise Server (GHES) and GitLab self-hosted instances.
- No natural support for token refresh or scoped, revocable credentials.
- Difficult per-user authentication story when sting runs as an MCP server inside agent runtimes.

The goal (tracked in Linear SKA-466 and the detailed command design in SKA-467) is to add first-class OAuth App authentication flows modeled on the official CLIs:

- GitHub: similar to `gh auth login` (web + device flow via the `cli/oauth` library).
- GitLab: similar to `glab auth login --device`.

The design calls for **explicit per-provider subcommands** (`sting auth github`, `sting auth gitlab`, plus `auth login <provider>` aliases) rather than a generic provider picker at the top level. `sting init` (and `init <provider>`) should provide guided onboarding.

A critical requirement that emerged during discovery:

- Proper GitHub OAuth (especially the web application flow with localhost callback and reliable device flow) requires a registered OAuth App.
- Because CLI tools are "public clients," the Client ID and Client Secret must be embedded in the binary. GitHub officially documents this pattern and notes that public clients should prefer Authorization Code + PKCE (or device flow).
- The official `gh` CLI ships exactly this way, embedding the credentials for its publicly registered "GitHub CLI" OAuth App (with a comment that the secret is "safe to be embedded in version control").
- For GitHub Enterprise Server, OAuth Apps must be registered **on the GHES instance itself**. Client credentials differ per enterprise deployment.

Additionally, many organizations will want (or need) to register and use their own OAuth Apps (for custom branding, specific scopes, audit requirements, or GHES).

## Decision

OAuth App authentication is the primary and recommended method for users authenticating with sting.

Support for classic Personal Access Tokens (PATs) is retained, but **strictly as a legacy fallback**. PATs must be clearly documented and presented as the non-preferred, advanced/automation-oriented option rather than the default path.

**High-level changes:**

- Add a new `auth` command group (`internal/cli`) with explicit subcommands:
  - `sting auth github` / `sting auth login github`
  - `sting auth gitlab` / `sting auth login gitlab`
  - `sting auth status`
  - `sting auth logout [github|gitlab]`
- Add `sting init` (and provider-specific variants) that can drive the new auth flows.
- Introduce an internal credential storage layer (`internal/credentials` or similar) responsible for:
  - Secure storage (OS keyring / credential store when available).
  - Graceful fallback to plaintext (under `~/.config/sting/` or equivalent) with clear user messaging.
  - Storage of both legacy PATs and OAuth tokens (access + refresh where applicable).
  - Per-provider, per-host (for Enterprise) scoping.
- Use `github.com/cli/oauth` (the same library used by `gh`) for GitHub flows on github.com and GHES. This library provides `DetectFlow()` which intelligently chooses between device flow and web application flow (localhost callback).
- For GitHub.com we will publish and embed credentials for an official "Sting CLI" (or "Skaphos Sting") OAuth App.
- Support "bring your own" OAuth App credentials, especially important for:
  - GitHub Enterprise Server (where apps are registered per instance).
  - Organizations that prefer their own registered app on GitHub.com or GitLab.
- A small amount of new configuration surface will be added later for custom `client_id` / `client_secret` per provider/host (see open question 6 in the credential storage spike).
- Update `internal/commitclient` and the MCP server path to obtain tokens via the new credential layer. The layer must support both OAuth tokens and legacy PATs, with clear precedence rules that treat OAuth as preferred when both are present for the same provider/host.
- Document the full registration process for both GitHub.com / GitLab.com and self-hosted GHES / GitLab instances so teams can use their own apps.
- Evolve (but do not break) the dedicated-credential philosophy from ADR 0002. OAuth tokens become the primary "sting's own" credentials. Legacy PAT configuration remains fully functional as a fallback but must be presented and documented as such.
- All user-facing surfaces (CLI help text, `auth status` output, `sting init` guidance, README, error messages, and migration notes) must present OAuth flows as the happy path and label PAT usage explicitly as the legacy fallback option.

The `get_commits` MCP tool remains the single read-only tool. Authentication configuration for the MCP server will continue to come from the same resolved configuration / credential store used by the CLI.

## Consequences

- Significantly improved onboarding experience via `sting init` and the explicit `auth` commands, with OAuth as the primary happy path.
- Better enterprise / self-hosted support through hostname prompting and bring-your-own app support.
- Clear, documented path for organizations to register and use their own OAuth Apps (GitHub.com, GHES, GitLab.com, self-hosted GitLab).
- We publish one official Skaphos OAuth App (with registration guide) for convenience, following the same embeddable-credentials pattern as `gh`.
- New internal credential storage abstraction; the public `config` package surface remains minimal and focused on query defaults (consistent with ADR 0004 public package guidelines).
- Legacy PAT support is preserved for backward compatibility, automation, and constrained environments, but is explicitly positioned and documented as the fallback method.
- All documentation, help text, status output, and onboarding flows must consistently present OAuth as the recommended path and PATs as the legacy/advanced fallback. This is a deliberate documentation and UX requirement.
- Requires a new ADR-level decision (this record) and updates to README, config.example.yaml, installation documentation, and command help text.
- Adds a dependency on `github.com/cli/oauth` (and transitively any secure storage library chosen for the credential layer).
- `model.SchemaVersion` is not expected to change for this feature (additive capability only).

## Alternatives Considered

- **Continue with PATs only + better documentation.** Rejected: does not address the stated UX, enterprise SSO, and MCP per-user goals in SKA-466.
- **Use GitHub Apps instead of OAuth Apps.** Considered for future work (better fine-grained permissions and installation model). For the initial user-authentication flow we follow the `gh` / `glab` precedent of OAuth Apps + device/web flows.
- **Always require users to bring their own OAuth App (no published Skaphos app).** Rejected for usability. We will publish a convenient default while fully documenting the bring-your-own path.
- **Hide all credential storage details behind `config.Config`.** Rejected: would bloat the public package and violate the minimal API surface goals. New logic belongs in `internal/`.
- **Single generic `auth login` command that prompts for provider.** Rejected per the explicit subcommand requirement in the SKA-467 design.
- **Support only device flow (no web fallback).** Rejected: the `cli/oauth` library's combined approach (with localhost callback) provides the best experience across environments, matching what users expect from `gh`.
- **Treat PATs as a co-equal primary authentication method alongside OAuth.** Rejected: this would dilute the UX improvements that are the main goal of SKA-466. PATs are retained only for compatibility and specific constrained use cases; they must be documented as the fallback.

## References

- Linear: SKA-466 (parent), SKA-467 (detailed command design)
- GitHub CLI source (primary reference implementation): `pkg/cmd/auth/...`, `internal/authflow/flow.go`, `internal/gh/gh.go`
- `github.com/cli/oauth` library
- GitHub docs: Best practices for public clients / OAuth Apps, GHES OAuth App registration
- ADR 0002 (Dedicated PAT via viper)
- ADR 0004 (Public packages)
- Draft registration guide: `docs/oauth-app-registration.md`
- Credential storage interface design spike: `docs/spikes/ska-466-credential-storage-interface-design.md`
