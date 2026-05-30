// SPDX-License-Identifier: MIT

// Package credentials provides secure (preferred) + plaintext (fallback)
// storage for Sting authentication material.
//
// # Isolation Guarantee
//
// This package is deliberately isolated from the user's real GitHub CLI (gh)
// configuration:
//
//   - We never mutate the GH_CONFIG_DIR environment variable.
//   - We never use github.com/cli/go-gh/v2/pkg/config for writing credentials.
//     (That package only supports configuration via GH_CONFIG_DIR, which
//     creates too much risk of accidental leakage or corruption of
//     ~/.config/gh during development, testing, or error conditions.)
//   - All insecure (plaintext) credential storage is written exclusively
//     to ~/.config/sting/hosts.yml using our own minimal implementation.
//
// The design follows the same logical "hosts.<composite>" structure that gh
// uses for familiarity, but everything under Sting's own directory.
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
	"gopkg.in/yaml.v3"

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

// store implements Store using keyring (secure) + our own file-based
// hosts.yml (insecure fallback) under ~/.config/sting.
type store struct {
	mu sync.RWMutex

	// keyringSvc returns the service name used in the keyring for a host.
	keyringSvc func(host string) string

	// insecurePath is the directory we use for plaintext fallback storage.
	// We use ~/.config/sting/hosts.yml with the same logical structure as gh.
	insecurePath string

	// hosts holds the in-memory representation of the hosts section
	// loaded from (or to be written to) hosts.yml.
	hosts map[string]map[string]string // composite -> {oauth_token, user, ...}
}

// New creates a Store using the default discovery order:
// 1. Try secure keyring via our internal/keyring wrapper.
// 2. Fall back to our own ~/.config/sting/hosts.yml (no env var mutation, no risk to gh config).
// The returned Store is safe for concurrent use.
func New() (Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	stingDir := filepath.Join(home, ".config", "sting")
	if err := os.MkdirAll(stingDir, 0700); err != nil {
		return nil, fmt.Errorf("cannot create sting config directory: %w", err)
	}

	s := &store{
		keyringSvc:   func(host string) string { return "sting:" + host },
		insecurePath: stingDir,
	}

	// Load existing insecure hosts (best effort)
	_ = s.loadInsecureHosts()

	return s, nil
}

// WithFilePath returns a Store that uses a specific directory for insecure
// plaintext storage (primarily for hermetic tests). It never touches GH_CONFIG_DIR.
func WithFilePath(dir string) Store {
	s := &store{
		keyringSvc:   func(host string) string { return "sting:" + host },
		insecurePath: dir,
	}
	_ = s.loadInsecureHosts()
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

		// 2. Fallback to our own insecure hosts.yml (strictly under sting dir)
		if s.hosts == nil {
			s.hosts = make(map[string]map[string]string)
		}
		if s.hosts[composite] == nil {
			s.hosts[composite] = make(map[string]string)
		}
		s.hosts[composite]["oauth_token"] = tok.AccessToken
		if tok.Username != "" {
			s.hosts[composite]["user"] = tok.Username
		}

		if writeErr := s.saveInsecureHosts(); writeErr != nil {
			return false, fmt.Errorf("failed to write insecure hosts file: %w", writeErr)
		}
		return true, nil
	}

	// Secure succeeded — clean up any stale insecure entry for this host
	if s.hosts != nil {
		if entry, ok := s.hosts[composite]; ok {
			delete(entry, "oauth_token")
			delete(entry, "user")
			if len(entry) == 0 {
				delete(s.hosts, composite)
			}
			_ = s.saveInsecureHosts()
		}
	}

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

	// 2. Fall back to our own insecure hosts.yml
	if s.hosts != nil {
		if entry, ok := s.hosts[composite]; ok {
			if tokStr := entry["oauth_token"]; tokStr != "" {
				return Token{
					Type:        TokenTypeOAuth,
					AccessToken: tokStr,
					Username:    entry["user"],
				}, SourceFile, nil
			}
		}
	}

	// 3. For GitHub providers, also consult go-gh/pkg/auth as an additional source.
	//    This is read-only and does not involve writing config.
	if provider == ProviderGitHub {
		if token, source := ghauth.TokenForHost(host); token != "" {
			ourSource := SourceConfig
			switch source {
			case "gh":
				ourSource = SourceKeyring
			case "oauth_token":
				ourSource = SourceFile
			}
			return Token{
				Type:        TokenTypeOAuth,
				AccessToken: token,
			}, ourSource, nil
		}

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

	if s.hosts != nil {
		if entry, ok := s.hosts[composite]; ok {
			delete(entry, "oauth_token")
			delete(entry, "user")
			if len(entry) == 0 {
				delete(s.hosts, composite)
			}
			_ = s.saveInsecureHosts()
		}
	}

	return nil
}

// List implements Store.
func (s *store) List(ctx context.Context) ([]CredentialRef, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var refs []CredentialRef

	if s.hosts == nil {
		return nil, nil
	}

	for composite, entry := range s.hosts {
		if tok := entry["oauth_token"]; tok != "" {
			prov, h := ProviderGitHub, composite
			if idx := len("github:"); len(composite) > idx && composite[:idx] == "github:" {
				prov, h = ProviderGitHub, composite[idx:]
			} else if idx := len("gitlab:"); len(composite) > idx && composite[:idx] == "gitlab:" {
				prov, h = ProviderGitLab, composite[idx:]
			}

			refs = append(refs, CredentialRef{
				Provider: prov,
				Host:     h,
				Username: entry["user"],
				Source:   SourceFile,
			})
		}
	}

	return refs, nil
}

// WithKeyringForTest returns a Store that uses the provided KeyringBackend
// for the secure path (useful for hermetic tests). The insecure path still
// uses our own file-based storage under the given directory.
func WithKeyringForTest(backend KeyringBackend, configDir string) Store {
	return WithFilePath(configDir)
}

// --- Insecure hosts.yml handling (strictly scoped to Sting) ---

type hostsFile struct {
	Hosts map[string]map[string]string `yaml:"hosts,omitempty"`
}

func (s *store) insecureHostsPath() string {
	return filepath.Join(s.insecurePath, "hosts.yml")
}

func (s *store) loadInsecureHosts() error {
	path := s.insecureHostsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.hosts = make(map[string]map[string]string)
			return nil
		}
		return err
	}

	var hf hostsFile
	if err := yaml.Unmarshal(data, &hf); err != nil {
		return err
	}

	if hf.Hosts == nil {
		s.hosts = make(map[string]map[string]string)
	} else {
		s.hosts = hf.Hosts
	}
	return nil
}

func (s *store) saveInsecureHosts() error {
	if s.insecurePath == "" {
		return nil
	}

	path := s.insecureHostsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	hf := hostsFile{Hosts: s.hosts}
	if hf.Hosts == nil {
		hf.Hosts = make(map[string]map[string]string)
	}

	data, err := yaml.Marshal(hf)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
