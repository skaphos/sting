// SPDX-License-Identifier: MIT
// Package commitclient selects the provider client for a resolved commit query.
package commitclient

import (
	"context"
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
func NewActivity(cfg config.Config, q model.ActivityQuery) (ActivityClient, error) {
	if q.Provider != "" && q.Provider != model.ProviderGitHub {
		return nil, fmt.Errorf("provider %q does not support repository activity (github only)", q.Provider)
	}
	token := resolveGitHubToken(cfg)
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
func New(cfg config.Config, q model.Query) (Client, error) {
	provider := q.Provider
	switch provider {
	case model.ProviderGitHub:
		token := resolveGitHubToken(cfg)
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
		token := resolveGitLabToken(cfg)
		client, err := gitlabclient.New(token, cfg.GitLabBaseURL, cfg.PerPage)
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
func resolveGitHubToken(cfg config.Config) string {
	if cfg.Token != "" {
		return cfg.Token
	}

	if store, err := credentials.New(); err == nil {
		if tok, _, err := store.Load(context.Background(), credentials.ProviderGitHub, githubHost(cfg)); err == nil && tok.AccessToken != "" {
			return tok.AccessToken
		}
	}

	return ""
}

// resolveGitLabToken mirrors resolveGitHubToken for GitLab: dedicated PAT
// (cfg.GitLabToken) wins, then the OAuth token from `sting auth gitlab`, then
// anonymous. Ambient GITLAB_TOKEN / glab credentials are intentionally never
// consulted (ADR 0002); see ADR 0008.
func resolveGitLabToken(cfg config.Config) string {
	if cfg.GitLabToken != "" {
		return cfg.GitLabToken
	}

	if store, err := credentials.New(); err == nil {
		if tok, _, err := store.Load(context.Background(), credentials.ProviderGitLab, gitlabHost(cfg)); err == nil && tok.AccessToken != "" {
			return tok.AccessToken
		}
	}

	return ""
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
