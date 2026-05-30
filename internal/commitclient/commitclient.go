// SPDX-License-Identifier: MIT
// Package commitclient selects the provider client for a resolved commit query.
package commitclient

import (
	"context"
	"fmt"
	"net/url"

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

// New builds the provider client selected by provider using cfg.
func New(cfg config.Config, provider model.Provider) (Client, error) {
	if provider == "" {
		provider = cfg.DefaultProvider
	}
	if provider == "" {
		provider = model.ProviderGitHub
	}
	switch provider {
	case model.ProviderGitHub:
		token := resolveGitHubToken(cfg)
		client, err := ghclient.New(token, cfg.BaseURL, cfg.PerPage)
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

// resolveGitHubToken prefers the new credentials store (for OAuth tokens),
// falling back to the legacy config/env token for backward compatibility.
func resolveGitHubToken(cfg config.Config) string {
	// Try the modern credential store first (supports OAuth + PATs stored via `auth` commands)
	if store, err := credentials.New(); err == nil {
		if tok, _, err := store.Load(context.Background(), credentials.ProviderGitHub, githubHost(cfg)); err == nil && tok.AccessToken != "" {
			return tok.AccessToken
		}
	}

	// Legacy fallback (STING_TOKEN, config token, etc.)
	return cfg.Token
}

// resolveGitLabToken prefers the new credentials store (OAuth tokens or PATs
// stored via `sting auth gitlab`), falling back to the legacy gitlab_token
// (config or STING_GITLAB_TOKEN) for backward compatibility.
func resolveGitLabToken(cfg config.Config) string {
	if store, err := credentials.New(); err == nil {
		if tok, _, err := store.Load(context.Background(), credentials.ProviderGitLab, gitlabHost(cfg)); err == nil && tok.AccessToken != "" {
			return tok.AccessToken
		}
	}

	// Legacy fallback
	return cfg.GitLabToken
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
// "https://ghe.example.com/api/v3" and bare hostnames.
func credentialHost(baseURL, defaultHost string) string {
	if baseURL == "" {
		return defaultHost
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
