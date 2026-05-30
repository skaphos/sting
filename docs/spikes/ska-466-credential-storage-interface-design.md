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

## Open Design Questions

1. **File format for plaintext fallback**  
   - JSON? (simple, but secrets in cleartext file)  
   - Encrypted with a machine-local key? (adds complexity)  
   - Same format gh uses? (we should investigate `go-gh` credential file format)

2. **Keyring library choice**  
   **Use `github.com/zalando/go-keyring`** (the exact one gh depends on and wraps).  
   We will copy gh's timeout wrapper + mock helpers into `internal/keyring` for robustness and testability.  
   Do **not** use 99designs/keyring or other alternatives — the goal is to follow gh's storage standards as closely as possible.

3. **Migration from existing config**  
   - On first `auth status` or `init`, should we detect legacy `token` / `gitlab_token` and offer to migrate them into the new store?
   - Or treat them as read-only legacy sources forever?

4. **Per-user vs per-host for GHES**  
   GHES often has one hostname but many users. Do we need `(provider, host, username)` as the key, or is `(provider, host)` + active user sufficient? (gh moved to multi-user support; we may want to keep it simpler initially).

5. **Environment variable precedence**  
   Should `STING_TOKEN` / `STING_GITLAB_TOKEN` still win over stored credentials (current viper behavior), or should stored OAuth tokens take priority?

6. **"Bring your own OAuth client" configuration**  
   This is related but slightly separate. We probably need a small sidecar concept for custom `client_id` / `client_secret` per host.

## Recommended Approach: Follow gh Libraries and Standards (No Reinvention)

Per direction, we should **not** invent our own storage mechanism. Instead, closely follow the proven libraries and patterns established by the official GitHub CLI:

### Core Libraries to Depend On

- **`github.com/zalando/go-keyring`** — The exact library gh uses (wrapped with timeouts + mocks in their `internal/keyring`).
  - Add a similar thin wrapper in `internal/keyring` for timeout protection and test mocking (copy the pattern from cli/cli).
- **`github.com/cli/go-gh/v2/pkg/config`** — For reading and writing the configuration file in the exact same format gh uses (`hosts.<host>.oauth_token`, users, etc.).
  - This gives us interoperability potential and follows the "standard" file layout.

### Storage Standard (from gh)

- **Keyring service name**: `"gh:" + hostname` (see `internal/config/config.go:keyringServiceName`).
  - For Sting we can use `"sting:" + hostname` or `"sting:<provider>:<host>"` to namespace cleanly while still being recognizable.
- **Keyring user key**: username (or empty string for the "active" slot).
- **Insecure fallback**: Write under `hosts.<hostname>.oauth_token` (and the multi-user `users.<user>.oauth_token` structure) using go-gh's config package.
- **Login flow logic**: Mirror `AuthConfig.Login` / `ActiveToken` / `Logout` from gh's `internal/config/config.go` and `internal/gh`.

### Benefits of This Approach

- Battle-tested secure + insecure fallback behavior with clear messaging.
- Same UX patterns users already know from `gh` (`keyring` vs `oauth_token` source in status).
- Easy to support reading tokens that were stored by `gh` (or vice versa) in the future.
- We get the multi-user-per-host support almost for free if we follow the same structure.
- Testability via their mock keyring helpers.

### Updated Spike Recommendation

The proposed `Store` interface above is still good as our **Sting-specific abstraction** (it adds the multi-provider + PAT fallback + precedence rules that gh doesn't have).

The **implementation** of that interface should be a thin layer on top of:
- `go-gh` config for the file side
- `zalando/go-keyring` (via our wrapper) for the secure side

This is the "use the libs and standards" path.

Do **not** reach for `99designs/keyring` — stick to zalando to match gh exactly.

## References

- ADR 0007 (OAuth App authentication)
- Previous spike: how `gh` does credential storage (`internal/gh/gh.go`, `AuthConfig.Login`)
- `github.com/cli/go-gh` credential handling
- GitHub best practices for public clients

---

*This document is a living design artifact on `feature/ska-466`. It will be superseded by the implementation and the final ADR once the interface stabilizes.*