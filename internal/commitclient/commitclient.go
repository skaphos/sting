// SPDX-License-Identifier: MIT
// Package commitclient selects the provider client for a resolved commit query.
package commitclient

import (
	"context"
	"fmt"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/internal/credentials"
	"github.com/skaphos/sting/ghclient"
	"github.com/skaphos/sting/gitlabclient"
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
		client, err := gitlabclient.New(cfg.GitLabToken, cfg.GitLabBaseURL, cfg.PerPage)
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
		if tok, _, err := store.Load(context.Background(), credentials.ProviderGitHub, "github.com"); err == nil && tok.AccessToken != "" {
			return tok.AccessToken
		}
	}

	// Legacy fallback (STING_TOKEN, config token, etc.)
	return cfg.Token
}
