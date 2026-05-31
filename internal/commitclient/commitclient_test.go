// SPDX-License-Identifier: MIT
package commitclient

import (
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/ghclient"
	"github.com/skaphos/sting/gitlabclient"
	"github.com/skaphos/sting/model"
)

func TestNewSelectsProvider(t *testing.T) {
	cfg := config.Default()

	githubClient, err := New(cfg, model.ProviderGitHub)
	if err != nil {
		t.Fatalf("New(github): %v", err)
	}
	if _, ok := githubClient.(*ghclient.Client); !ok {
		t.Fatalf("New(github) = %T, want *ghclient.Client", githubClient)
	}

	gitlabClient, err := New(cfg, model.ProviderGitLab)
	if err != nil {
		t.Fatalf("New(gitlab): %v", err)
	}
	if _, ok := gitlabClient.(*gitlabclient.Client); !ok {
		t.Fatalf("New(gitlab) = %T, want *gitlabclient.Client", gitlabClient)
	}
}

func TestNewUsesDefaultProvider(t *testing.T) {
	cfg := config.Default()
	cfg.DefaultProvider = model.ProviderGitLab

	client, err := New(cfg, "")
	if err != nil {
		t.Fatalf("New(default gitlab): %v", err)
	}
	if _, ok := client.(*gitlabclient.Client); !ok {
		t.Fatalf("New(default gitlab) = %T, want *gitlabclient.Client", client)
	}
}

func TestNewRejectsUnsupportedProvider(t *testing.T) {
	if _, err := New(config.Default(), model.Provider("bogus")); err == nil {
		t.Fatal("New: want error for unsupported provider")
	}
}

func TestNewWrapsProviderBuildErrors(t *testing.T) {
	cfg := config.Default()
	cfg.BaseURL = "://bad"
	if _, err := New(cfg, model.ProviderGitHub); err == nil {
		t.Fatal("New(github bad URL): want error")
	}

	cfg = config.Default()
	cfg.GitLabBaseURL = "://bad"
	if _, err := New(cfg, model.ProviderGitLab); err == nil {
		t.Fatal("New(gitlab bad URL): want error")
	}
}

func TestResolveGitHubTokenPrefersConfigToken(t *testing.T) {
	cfg := config.Default()
	cfg.Token = "explicit-gh-token"

	if got := resolveGitHubToken(cfg); got != cfg.Token {
		t.Fatalf("resolveGitHubToken=%q, want %q", got, cfg.Token)
	}
}

func TestResolveGitLabTokenPrefersConfigToken(t *testing.T) {
	cfg := config.Default()
	cfg.GitLabToken = "explicit-gl-token"

	if got := resolveGitLabToken(cfg); got != cfg.GitLabToken {
		t.Fatalf("resolveGitLabToken=%q, want %q", got, cfg.GitLabToken)
	}
}
