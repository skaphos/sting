# Spike: Credential Storage Interface Design (SKA-466)

**Date**: 2026-05-30  
**Branch**: `feature/ska-466`  
**Status**: In progress (design spike)

> **Important**: Per explicit direction, do **not** reinvent credential storage.  
> We must reuse the libraries (`github.com/zalando/go-keyring`, `github.com/cli/go-gh/v2/pkg/config`) and storage patterns/standards established by the official GitHub CLI (`gh`).  
> See the "Following gh Standards and Libraries" section below for the concrete recommendation.



## Goals

Design a clean, testable internal interface for storing and retrieving authentication credentials (both OAuth tokens and legacy PATs) that:

- Prefers secure OS-backed storage (keyring / credential manager) when available.
- Gracefully falls back to a plaintext file with clear user messaging (consistent with ADR 0002 philosophy and gh behavior).
- Supports multiple providers (`github`, `gitlab`) and multiple hosts per provider (critical for GHES / self-hosted GitLab).
- Makes OAuth the preferred path when both OAuth and PAT credentials exist for the same provider+host.
- Is easy to unit test (isolated HOME, in-memory implementations).
- Keeps the public `config` package surface minimal.

This spike is intentionally narrow: we are designing the **interface and responsibilities**, not a full implementation.

## Requirements (from ADR 0007 + prior decisions)

- Primary happy path: OAuth App tokens (access + refresh + expiry where available).
- Fallback path: Classic PATs (`token` / `gitlab_token` from existing viper/config).
- Clear precedence: When both exist for `(provider, host)`, prefer the OAuth credential.
- Per-host scoping (e.g. `github.com` vs `ghe.example.com`).
- Support for "bring your own" OAuth client credentials (future config surface).
- When falling back to plaintext storage, emit a clear warning (like `gh` does).
- Must work in headless / CI environments (where keyring may not be available).
- Must support `auth status`, `auth logout`, and migration paths.

## Proposed Interface (v0.1)

```go
// Package credentials provides secure (preferred) + plaintext (fallback)
// storage for Sting authentication material.
//
// It is intentionally internal. The public config package remains focused
// on query defaults and does not grow auth token concerns.
package credentials

import (
	"context"
	"time"
)

// Provider identifies a source control system.
type Provider string

const (
	ProviderGitHub Provider = "github"
	ProviderGitLab Provider = "gitlab"
)

// TokenType distinguishes the kind of credential.
type TokenType string

const (
	TokenTypeOAuth TokenType = "oauth"
	TokenTypePAT   TokenType = "pat"
)

// Token represents a stored credential.
// For PATs, only AccessToken is populated.
// For OAuth, the full set may be present.
type Token struct {
	Type         TokenType
	AccessToken  string
	RefreshToken string    // may be empty
	Expiry       time.Time // zero value means no expiry
	Username     string    // best-effort; populated after successful auth
}

// Source describes where a token came from (for status + messaging).
type Source string

const (
	SourceKeyring Source = "keyring"
	SourceFile    Source = "file"
	SourceEnv     Source = "environment" // legacy PATs from STING_TOKEN etc.
	SourceConfig  Source = "config"      // legacy PATs from config file
)

// Store is the main abstraction for credential lifecycle.
type Store interface {
	// Save persists a credential for the given provider + host.
	// secureOnly=true forces an error instead of falling back to plaintext.
	Save(ctx context.Context, provider Provider, host string, tok Token, secureOnly bool) (usedInsecure bool, err error)

	// Load returns the best available token for (provider, host).
	// It applies precedence rules (OAuth > PAT) and returns the Source
	// so callers can produce appropriate status messaging.
	Load(ctx context.Context, provider Provider, host string) (tok Token, src Source, err error)

	// Delete removes credentials for the given provider + host.
	// It should attempt to clean both secure and insecure locations.
	Delete(ctx context.Context, provider Provider, host string) error

	// List returns known (provider, host) combinations that have stored credentials.
	// Useful for `auth status --all`.
	List(ctx context.Context) ([]CredentialRef, error)
}

// CredentialRef is a lightweight reference returned by List.
type CredentialRef struct {
	Provider Provider
	Host     string
	Username string // may be empty
	Source   Source
}

// New creates a Store using the default discovery order (following gh patterns):
// 1. Try secure keyring via zalando/go-keyring (with gh-style timeout wrapper).
// 2. Fall back to plaintext under the gh-style hosts config using go-gh/pkg/config.
// The returned Store is safe for concurrent use.
func New() (Store, error)

// WithFilePath is primarily for testing. It forces the plaintext backend
// to a specific location and disables keyring.
func WithFilePath(path string) Store

// WithKeyring is an escape hatch for tests or advanced users who want
// to inject a specific keyring backend.
func WithKeyring(backend KeyringBackend) Store
```

### Precedence Rules (inside `Load`)

1. If an OAuth token exists for `(provider, host)` → return it (regardless of PAT presence).
2. Else if a PAT exists (from any source: keyring, file, env, config) → return it.
3. Else → not found error.

This ensures that once a user successfully runs `sting auth github`, future operations prefer the OAuth token even if a legacy `token:` still exists in their config.

## Open Design Questions — **LOCKED DECISIONS** (2026-05-30)

All questions have been reviewed and confirmed. These are now final for implementation.

### 1. File format for plaintext fallback — **LOCKED**

**Final Decision:** Use Sting's own `hosts.yml` under Sting's config directory, written through `internal/credentials` with a minimal YAML schema.

- Insecure fallback lives in Sting's config directory, never under the user's `gh` config directory.
- The file uses `hosts.<provider>:<hostname>.oauth_token` and `hosts.<provider>:<hostname>.pat_token` entries so OAuth and PAT credentials remain distinguishable.
- The writer uses `0600` permissions and an atomic temp-file-plus-rename write.
- `github.com/cli/go-gh/v2/pkg/config` remains read-only compatibility context for GitHub credential discovery; it is not used for writing Sting credentials.

### 2. Keyring library choice — **LOCKED**

Use `github.com/zalando/go-keyring` exactly as gh does.
- Copy gh's timeout wrapper + `MockInit` helpers into `internal/keyring`.
- No other keyring libraries.

### 3. Migration from existing config — **LOCKED**

- No auto-migration.
- Legacy `token` / `gitlab_token` (from current viper/config) are treated as **read-only legacy sources**.
- On `auth status` and in messaging, clearly distinguish old config sources vs new credential store.
- Explicit migration can be offered later via `auth` or `init` commands if desired.
- **Context**: This is accurate because there are currently no production installs of Sting.

### 4. Per-user vs per-host for GHES — **LOCKED**

Follow gh's model exactly:
- Primary keying is `(provider, host)` + active user slot.
- Retain the full multi-user structure (`users.<user>`) from day one because we are using go-gh's config format.
- "One active credential per host" is the initial UX; full multi-account switching can be added later.

### 5. Environment variable precedence — **LOCKED**

- `STING_TOKEN` / `STING_GITLAB_TOKEN` (and corresponding flags) retain highest priority for explicit override (preserves CI/automation expectations and ADR 0002).
- Among *stored* credentials, OAuth tokens take precedence over legacy PATs.
- Document clearly in `auth status`.

### 6. "Bring your own OAuth client" configuration — **LOCKED (keys TBD)**

Supported use case (especially important for GHES).
- A small new configuration surface will be added for custom `client_id` / `client_secret` per provider/host.
- Exact key names and structure will be decided by the user during the documentation + implementation loop.
- Noted in ADR 0007 and the registration guide.

---

**All open design questions have now been addressed above with proposed resolutions.**

The implementation follows gh's keyring choice while keeping plaintext writes isolated to Sting's own config directory.

Next step after review: Lock these decisions, update ADR 0007 with any final implications, then begin the first implementation slice (adding the dependencies + the keyring wrapper + a working `credentials.Store` implementation).

This keeps the useful library precedent without mutating the user's GitHub CLI configuration.

Do **not** reach for `99designs/keyring` — stick to zalando to match gh exactly.

## References

- ADR 0007 (OAuth App authentication)
- Previous spike: how `gh` does credential storage (`internal/gh/gh.go`, `AuthConfig.Login`)
- `github.com/cli/go-gh` credential handling
- GitHub best practices for public clients

---

**Status**: All open design questions locked (2026-05-30). Ready for implementation.

---

## Leveraging go-gh Components (Ongoing Strategy)

As we continue implementation, we are deliberately maximizing reuse of `github.com/cli/go-gh/v2` to minimize custom code and third-party dependencies in Sting.

### Current Usage

- **`pkg/config`** — Kept as reference material only. Sting does not write through go-gh config because that would require routing through GitHub CLI configuration state. Plaintext credential writes are handled by `internal/credentials` in Sting's own `hosts.yml`.

### In Progress / Next

- **`pkg/auth`** — Being integrated into `Load()` (at least for GitHub providers).
  - `auth.TokenFromEnvOrConfig(host)` gives us GitHub-aware env var + config file reading (with proper normalization and tenancy handling).
  - `auth.TokenForHost(host)` additionally tries the system keyring by shelling out to `gh auth token --secure-storage` when `gh` is installed.
  - This provides excellent compatibility: if a user is already logged into GitHub via the official `gh` CLI, Sting can discover those tokens without forcing re-authentication in many cases.
  - For GitLab we continue using our own logic (go-gh is GitHub-focused).

### Future Opportunities (When Implementing `auth` Login Commands)

- **`pkg/browser`** — Use instead of directly depending on `github.com/cli/browser`. Gives us free support for `GH_BROWSER` env var and config file settings.
- **`pkg/prompter`** — Use for interactive prompts (`Select`, `Input`, `Password`, `Confirm`) during `sting auth github` / `init`. High-quality wrapper around survey with proper stdio handling.
- **`pkg/api`** — For post-login operations (e.g. fetching the current user after OAuth completes, or making GraphQL queries). The authenticated clients here already know how to attach tokens via the auth package.

### What go-gh Does *Not* Replace

- We still need our own `internal/keyring` wrapper (based on `zalando/go-keyring`) because:
  - We need to *write* tokens during `auth login`.
  - We support GitLab (go-gh is GitHub-only).
  - We want native secure storage without requiring the `gh` binary to be present.
- The core `credentials.Store` abstraction and multi-provider logic remain Sting-specific.

### Guiding Principle Going Forward

When adding new functionality in the auth flow (especially the login commands), default to asking: "Can this be done with something already in go-gh?" before pulling in new direct dependencies or writing custom prompt/browser/auth logic.

This keeps our dependency footprint cleaner and makes Sting feel more consistent with the official GitHub CLI experience where it makes sense.

*This section will be expanded as we adopt more pieces during implementation.*
