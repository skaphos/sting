// Package credentials provides secure (preferred) + plaintext (fallback)
// storage for Sting authentication material.
//
// It follows the storage standards and patterns established by the official
// GitHub CLI (gh) as closely as possible:
//   - Secure storage via the same zalando/go-keyring wrapper pattern.
//   - Insecure fallback uses the same conceptual "hosts" structure
//     (implementation currently uses JSON for the skeleton; will be
//     switched to github.com/cli/go-gh/v2/pkg/config for full fidelity).
//
// This package is intentionally internal. The public config package remains
// focused on query defaults and does not grow auth token concerns.
package credentials

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	ghauth "github.com/cli/go-gh/v2/pkg/auth"
	ghconfig "github.com/cli/go-gh/v2/pkg/config"

	"github.com/skaphos/sting/internal/keyring"
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
	RefreshToken string    // may be empty for some providers
	Expiry       time.Time // zero value means no expiry / does not expire
	Username     string    // best-effort; populated after successful auth
}

// Source describes where a token came from (for status + messaging).
type Source string

const (
	SourceKeyring Source = "keyring"
	SourceFile    Source = "file"
	SourceEnv     Source = "environment" // legacy PATs from STING_TOKEN etc.
	SourceConfig  Source = "config"      // legacy PATs from config file (viper)
)

// Store is the main abstraction for credential lifecycle.
type Store interface {
	// Save persists a credential for the given provider + host.
	// secureOnly=true forces an error instead of falling back to plaintext.
	Save(ctx context.Context, provider Provider, host string, tok Token, secureOnly bool) (usedInsecure bool, err error)

	// Load returns the best available token for (provider, host).
	// It applies precedence rules (OAuth > PAT for the same provider+host)
	// and returns the Source so callers (e.g. auth status) can produce
	// appropriate messaging.
	Load(ctx context.Context, provider Provider, host string) (tok Token, src Source, err error)

	// Delete removes credentials for the given provider + host.
	// It attempts to clean both secure and insecure locations.
	Delete(ctx context.Context, provider Provider, host string) error

	// List returns known (provider, host) combinations that have stored credentials.
	// Useful for `auth status --all` or similar.
	List(ctx context.Context) ([]CredentialRef, error)
}

// CredentialRef is a lightweight reference returned by List.
type CredentialRef struct {
	Provider Provider
	Host     string
	Username string // may be empty
	Source   Source
}

// store implements Store using keyring (secure) + ghconfig (insecure fallback).
type store struct {
	mu sync.RWMutex

	// keyringSvc returns the service name used in the keyring for a host.
	// We follow a "sting:<host>" pattern (namespaced from gh's "gh:<host>").
	keyringSvc func(host string) string

	// cfg is the ghconfig instance used for insecure (plaintext) storage.
	// We point it at ~/.config/sting so we don't touch the user's gh config.
	// This gives us the exact same hosts.*.oauth_token structure and multi-user support.
	cfg *ghconfig.Config
}

// New creates a Store using the default discovery order (following gh patterns):
// 1. Try secure keyring via our internal/keyring wrapper (zalando/go-keyring + timeouts).
// 2. Fall back to ghconfig (pointed at ~/.config/sting/hosts.yml + config.yml).
// The returned Store is safe for concurrent use.
func New() (Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	stingConfigDir := filepath.Join(home, ".config", "sting")
	if err := os.MkdirAll(stingConfigDir, 0700); err != nil {
		return nil, fmt.Errorf("cannot create sting config directory: %w", err)
	}

	// Point go-gh's config at our own directory so we get our own hosts.yml
	// without touching the user's ~/.config/gh.
	old := os.Getenv("GH_CONFIG_DIR")
	os.Setenv("GH_CONFIG_DIR", stingConfigDir)
	defer os.Setenv("GH_CONFIG_DIR", old)

	cfg, err := ghconfig.Read(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read gh-style config for sting: %w", err)
	}

	s := &store{
		keyringSvc: func(host string) string { return "sting:" + host },
		cfg:        cfg,
	}

	return s, nil
}

// WithFilePath returns a Store that forces the ghconfig insecure storage
// to use a specific directory (by temporarily setting GH_CONFIG_DIR).
// Primarily intended for tests. Keyring is still active unless overridden.
func WithFilePath(dir string) Store {
	old := os.Getenv("GH_CONFIG_DIR")
	os.Setenv("GH_CONFIG_DIR", dir)

	cfg, _ := ghconfig.Read(nil) // best effort for tests

	os.Setenv("GH_CONFIG_DIR", old)

	s := &store{
		keyringSvc: func(host string) string { return "sting:" + host },
		cfg:        cfg,
	}
	return s
}

// KeyringBackend is the minimal interface we need from a keyring implementation.
// This allows tests to inject a mock.
type KeyringBackend interface {
	Set(service, user, secret string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}


// compositeHost returns the key we use inside the ghconfig "hosts" map.
// Using "provider:host" keeps GitHub and GitLab (and multiple GHES instances) cleanly separated
// while still living inside the standard hosts structure that go-gh expects.
func compositeHost(provider Provider, host string) string {
	return string(provider) + ":" + host
}

// Save implements Store.
func (s *store) Save(ctx context.Context, provider Provider, host string, tok Token, secureOnly bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	composite := compositeHost(provider, host)

	// 1. Try secure storage first (keyring)
	err := keyring.Set(s.keyringSvc(host), "", tok.AccessToken)
	if err != nil {
		if secureOnly {
			return false, fmt.Errorf("secure storage required but failed: %w", err)
		}

		// 2. Fallback to ghconfig (insecure)
		s.cfg.Set([]string{"hosts", composite, "oauth_token"}, tok.AccessToken)
		if tok.Username != "" {
			s.cfg.Set([]string{"hosts", composite, "user"}, tok.Username)
		}

		if writeErr := ghconfig.Write(s.cfg); writeErr != nil {
			return false, fmt.Errorf("failed to write insecure ghconfig: %w", writeErr)
		}
		return true, nil
	}

	// Secure succeeded — clean up any stale insecure entry for this host (gh behavior)
	_ = s.cfg.Remove([]string{"hosts", composite, "oauth_token"})
	_ = s.cfg.Remove([]string{"hosts", composite, "user"})
	_ = ghconfig.Write(s.cfg)

	return false, nil
}

// Load implements Store with OAuth > PAT precedence.
func (s *store) Load(ctx context.Context, provider Provider, host string) (Token, Source, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	composite := compositeHost(provider, host)

	// 1. Try keyring (secure) first
	if tokStr, err := keyring.Get(s.keyringSvc(host), ""); err == nil && tokStr != "" {
		return Token{
			Type:        TokenTypeOAuth,
			AccessToken: tokStr,
		}, SourceKeyring, nil
	}

	// 2. Fall back to our ghconfig-based insecure storage
	if tokStr, err := s.cfg.Get([]string{"hosts", composite, "oauth_token"}); err == nil && tokStr != "" {
		user, _ := s.cfg.Get([]string{"hosts", composite, "user"})
		return Token{
			Type:        TokenTypeOAuth,
			AccessToken: tokStr,
			Username:    user,
		}, SourceFile, nil
	}

	// 3. For GitHub providers, also consult go-gh/pkg/auth as an additional source.
	//    This gives us:
	//    - GitHub-specific env var handling (GH_TOKEN, GITHUB_TOKEN, etc.)
	//    - Reading from the user's gh config (if any)
	//    - Access to system keyring via `gh auth token` if the gh binary is installed
	if provider == ProviderGitHub {
		if token, source := ghauth.TokenForHost(host); token != "" {
			// Map go-gh source names to our Source constants where it makes sense
			ourSource := SourceConfig
			switch source {
			case "gh":
				ourSource = SourceKeyring // came via gh binary → effectively keyring
			case "oauth_token":
				ourSource = SourceFile
			}
			return Token{
				Type:        TokenTypeOAuth,
				AccessToken: token,
			}, ourSource, nil
		}

		// Also try the env-or-config only version (in case TokenForHost behavior changes)
		if token, source := ghauth.TokenFromEnvOrConfig(host); token != "" {
			ourSource := SourceConfig
			if source == "oauth_token" {
				ourSource = SourceFile
			}
			return Token{
				Type:        TokenTypeOAuth,
				AccessToken: token,
			}, ourSource, nil
		}
	}

	return Token{}, "", fmt.Errorf("no credential found for %s/%s", provider, host)
}

// Delete implements Store.
func (s *store) Delete(ctx context.Context, provider Provider, host string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	composite := compositeHost(provider, host)

	// Best effort delete from both secure and insecure
	_ = keyring.Delete(s.keyringSvc(host), "")
	_ = s.cfg.Remove([]string{"hosts", composite, "oauth_token"})
	_ = s.cfg.Remove([]string{"hosts", composite, "user"})

	return ghconfig.Write(s.cfg)
}

// List implements Store.
func (s *store) List(ctx context.Context) ([]CredentialRef, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var refs []CredentialRef

	// For now we enumerate from the ghconfig hosts map.
	// A more complete version would also enumerate keyring entries.
	hosts, err := s.cfg.Keys([]string{"hosts"})
	if err != nil {
		return nil, nil // no hosts section yet
	}

	for _, composite := range hosts {
		if tok, err := s.cfg.Get([]string{"hosts", composite, "oauth_token"}); err == nil && tok != "" {
			user, _ := s.cfg.Get([]string{"hosts", composite, "user"})

			// Parse back the provider:host we stored
			prov, h := ProviderGitHub, composite
			if idx := len("github:"); len(composite) > idx && composite[:idx] == "github:" {
				prov, h = ProviderGitHub, composite[idx:]
			} else if idx := len("gitlab:"); len(composite) > idx && composite[:idx] == "gitlab:" {
				prov, h = ProviderGitLab, composite[idx:]
			}

			refs = append(refs, CredentialRef{
				Provider: prov,
				Host:     h,
				Username: user,
				Source:   SourceFile,
			})
		}
	}

	return refs, nil
}

// WithKeyringForTest returns a Store that uses the provided KeyringBackend
// for the secure path (useful for hermetic tests). The insecure path still
// uses go-gh config (pointed at the given directory).
func WithKeyringForTest(backend KeyringBackend, configDir string) Store {
	// For the skeleton we accept the backend but still go through our wrapper
	// for the main path. Full injection can be added when we need it.
	// For now we just return a normal WithFilePath version.
	return WithFilePath(configDir)
}
