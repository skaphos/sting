// SPDX-License-Identifier: MIT
// Package commitclient selects the provider client for a resolved commit query.
package commitclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/ghclient"
	"github.com/skaphos/sting/gitlabclient"
	"github.com/skaphos/sting/internal/credentials"
	"github.com/skaphos/sting/model"
)

// Client is the common behavior shared by provider-specific commit clients.
type Client interface {
	Collect(context.Context, model.Query) (model.Result, error)
}

// ActivityClient gathers a repository's activity over a window. It is a
// separate interface from Client because only GitHub implements it: a GitLab
// activity path is not stubbed out, since config.ResolveActivity rejects
// non-GitHub providers before a client is ever constructed.
type ActivityClient interface {
	CollectActivity(context.Context, model.ActivityQuery) (model.ActivityResult, error)
}

// NewActivity builds the GitHub client for a repository-activity query, wiring
// in the request budget so every provider call is accounted and bounded.
//
// It takes no provider argument: repository activity is GitHub-only, and the
// rejection lives in config.ResolveActivity so the failure is identical whether
// the request came from the CLI or from MCP, and is unreachable with a
// half-built client behind it.
func NewActivity(cfg config.Config, q model.ActivityQuery, opts ...Option) (ActivityClient, error) {
	if q.Provider != "" && q.Provider != model.ProviderGitHub {
		return nil, fmt.Errorf("provider %q does not support repository activity (github only)", q.Provider)
	}
	token, r := resolveGitHubToken(cfg)
	applyOptions(opts).emit(r)
	client, err := ghclient.New(token, cfg.BaseURL, cfg.PerPage, ghclient.WithRequestBudget(q.MaxRequests))
	if err != nil {
		return nil, fmt.Errorf("build github client: %w", err)
	}
	return client, nil
}

// New builds the provider client for a resolved query. q.Provider is expected
// to be an already-resolved, non-empty value: config.Resolve applies the
// empty->default->github fallback and rejects invalid providers, so New does not
// repeat that defaulting. The switch's default branch is kept as a guard so the
// exported constructor never returns a nil client for an unexpected value.
//
// It takes the whole query rather than just the provider so the per-call request
// ceiling reaches the client; a ceiling resolved into the query but never handed
// to the transport would be silently ignored.
func New(cfg config.Config, q model.Query, opts ...Option) (Client, error) {
	o := applyOptions(opts)
	provider := q.Provider
	switch provider {
	case model.ProviderGitHub:
		token, r := resolveGitHubToken(cfg)
		o.emit(r)
		// Account for every author-scoped path too, not just the activity one:
		// the transport-level budget covers search, repos, org, PR discovery,
		// and enrichment without any of them knowing about it.
		client, err := ghclient.New(token, cfg.BaseURL, cfg.PerPage,
			ghclient.WithRequestBudget(q.MaxRequests))
		if err != nil {
			return nil, fmt.Errorf("build github client: %w", err)
		}
		return client, nil
	case model.ProviderGitLab:
		token, r := resolveGitLabToken(cfg)
		o.emit(r)
		client, err := gitlabclient.New(token, cfg.GitLabBaseURL, cfg.PerPage,
			gitlabclient.WithRequestBudget(q.MaxRequests))
		if err != nil {
			return nil, fmt.Errorf("build gitlab client: %w", err)
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", provider)
	}
}

// resolveGitHubToken resolves the GitHub token by "most explicit wins": sting's
// dedicated PAT (cfg.Token — the --token flag, STING_TOKEN, or config-file
// "token", all collapsed by viper) takes precedence, then the OAuth token saved
// by `sting auth github`, then anonymous. Ambient tokens (GITHUB_TOKEN, the gh
// CLI's stored auth, etc.) are intentionally never consulted, preserving the
// strict separation of ADR 0002; do not add such a fallback here. See ADR 0008
// for the full precedence rationale.
func resolveGitHubToken(cfg config.Config) (string, resolution) {
	return resolveToken(cfg.Token, model.ProviderGitHub, credentials.ProviderGitHub, githubHost(cfg))
}

// resolveGitLabToken mirrors resolveGitHubToken for GitLab: dedicated PAT
// (cfg.GitLabToken) wins, then the OAuth token from `sting auth gitlab`, then
// anonymous. Ambient GITLAB_TOKEN / glab credentials are intentionally never
// consulted (ADR 0002); see ADR 0008.
func resolveGitLabToken(cfg config.Config) (string, resolution) {
	return resolveToken(cfg.GitLabToken, model.ProviderGitLab, credentials.ProviderGitLab, gitlabHost(cfg))
}

// openCredentialStore is a package variable so tests can substitute a fake
// store instead of reaching the real system keyring.
var openCredentialStore = credentials.New

// resolveToken applies the shared precedence and records which tier won. A
// failed stored-credential lookup still falls through to anonymous exactly as
// before; the resolution only makes that fallback explainable.
func resolveToken(dedicated string, provider model.Provider, credProvider credentials.Provider, host string) (string, resolution) {
	r := resolution{provider: provider, host: host, source: sourceAnonymous}
	if dedicated != "" {
		r.source = sourceDedicated
		return dedicated, r
	}

	store, err := openCredentialStore()
	if err != nil {
		// The underlying error can quote credentials-file contents (a YAML
		// parse error echoes the offending line), so it is never reported.
		r.lookupReason = "credentials file could not be read"
		return "", r
	}
	tok, src, err := store.Load(context.Background(), credProvider, host)
	switch {
	case err == nil && tok.AccessToken != "":
		r.source = storedSource(src)
		r.kind = tok.Type
		return tok.AccessToken, r
	case err != nil && !errors.Is(err, credentials.ErrNotFound):
		// Load's own errors name only provider, host, and the keyring's error.
		r.lookupReason = singleLine(err.Error())
	}
	return "", r
}

// githubHost returns the hostname to use when looking up GitHub credentials.
// It derives the host from cfg.BaseURL when targeting GitHub Enterprise Server,
// falling back to "github.com".
func githubHost(cfg config.Config) string {
	return credentialHost(cfg.BaseURL, "github.com")
}

// gitlabHost returns the hostname to use when looking up GitLab credentials.
// It derives the host from cfg.GitLabBaseURL when targeting a self-hosted
// GitLab instance, falling back to "gitlab.com".
func gitlabHost(cfg config.Config) string {
	return credentialHost(cfg.GitLabBaseURL, "gitlab.com")
}

// credentialHost extracts a bare hostname from a base URL (for credential
// storage keys). It is resilient to full API paths like
// "https://ghe.example.com/api/v3" as well as schemeless inputs like
// "ghe.example.com" or "ghe.example.com/api/v3".
func credentialHost(baseURL, defaultHost string) string {
	if baseURL == "" {
		return defaultHost
	}
	// url.Parse treats a schemeless input as a path, which leaves Hostname
	// empty, so add a scheme when one is missing before parsing.
	if !strings.Contains(baseURL, "://") {
		baseURL = "https://" + baseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return defaultHost
	}
	if h := u.Hostname(); h != "" {
		return h
	}
	return defaultHost
}
