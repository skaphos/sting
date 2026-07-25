// SPDX-License-Identifier: MIT
package commitclient

import (
	"strings"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/ghclient"
	"github.com/skaphos/sting/gitlabclient"
	"github.com/skaphos/sting/model"
)

func TestNewSelectsProvider(t *testing.T) {
	cfg := config.Default()

	githubClient, err := New(cfg, model.Query{Provider: model.ProviderGitHub})
	if err != nil {
		t.Fatalf("New(github): %v", err)
	}
	if _, ok := githubClient.(*ghclient.Client); !ok {
		t.Fatalf("New(github) = %T, want *ghclient.Client", githubClient)
	}

	gitlabClient, err := New(cfg, model.Query{Provider: model.ProviderGitLab})
	if err != nil {
		t.Fatalf("New(gitlab): %v", err)
	}
	if _, ok := gitlabClient.(*gitlabclient.Client); !ok {
		t.Fatalf("New(gitlab) = %T, want *gitlabclient.Client", gitlabClient)
	}
}

// TestNewRejectsEmptyProvider documents that New no longer defaults an empty
// provider: provider defaulting (empty->default->github) is centralized in
// config.Resolve, and New expects an already-resolved value. The switch's
// default branch guards against an empty or unknown provider rather than
// silently returning a nil client.
func TestNewRejectsEmptyProvider(t *testing.T) {
	if _, err := New(config.Default(), model.Query{}); err == nil {
		t.Fatal("New(empty provider): want error; defaulting lives in config.Resolve")
	}
}

func TestNewRejectsUnsupportedProvider(t *testing.T) {
	if _, err := New(config.Default(), model.Query{Provider: model.Provider("bogus")}); err == nil {
		t.Fatal("New: want error for unsupported provider")
	}
}

func TestNewWrapsProviderBuildErrors(t *testing.T) {
	cfg := config.Default()
	cfg.BaseURL = "://bad"
	if _, err := New(cfg, model.Query{Provider: model.ProviderGitHub}); err == nil {
		t.Fatal("New(github bad URL): want error")
	}

	cfg = config.Default()
	cfg.GitLabBaseURL = "://bad"
	if _, err := New(cfg, model.Query{Provider: model.ProviderGitLab}); err == nil {
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

func TestNewActivityBuildsGitHubClient(t *testing.T) {
	cfg := config.Default()
	cfg.Token = "test-token"

	client, err := NewActivity(cfg, model.ActivityQuery{
		Provider:    model.ProviderGitHub,
		Repo:        "skaphos/sting",
		MaxRequests: 50,
	})
	if err != nil {
		t.Fatalf("NewActivity: %v", err)
	}
	if _, ok := client.(*ghclient.Client); !ok {
		t.Fatalf("NewActivity = %T, want *ghclient.Client", client)
	}
}

// TestNewActivityAppliesTheQueryCeiling: a ceiling resolved into the query but
// never handed to the transport would be silently ignored.
func TestNewActivityAppliesTheQueryCeiling(t *testing.T) {
	cfg := config.Default()
	client, err := NewActivity(cfg, model.ActivityQuery{
		Provider:    model.ProviderGitHub,
		Repo:        "skaphos/sting",
		MaxRequests: 42,
	})
	if err != nil {
		t.Fatalf("NewActivity: %v", err)
	}
	gh, ok := client.(*ghclient.Client)
	if !ok {
		t.Fatalf("NewActivity = %T, want *ghclient.Client", client)
	}
	if got := gh.Cost().Ceiling; got != 42 {
		t.Errorf("ceiling = %d, want 42 — the query's ceiling did not reach the client", got)
	}
}

// TestNewActivityRejectsNonGitHub keeps the GitHub-only limit reachable even if
// a caller bypasses config.ResolveActivity.
func TestNewActivityRejectsNonGitHub(t *testing.T) {
	_, err := NewActivity(config.Default(), model.ActivityQuery{
		Provider: model.ProviderGitLab,
		Repo:     "group/project",
	})
	if err == nil {
		t.Fatal("NewActivity(gitlab): want an error")
	}
	if !strings.Contains(err.Error(), "does not support repository activity") {
		t.Errorf("error = %v, want the GitHub-only rejection", err)
	}
}

func TestNewActivityMalformedBaseURL(t *testing.T) {
	cfg := config.Default()
	cfg.BaseURL = "http://\x7f/"
	if _, err := NewActivity(cfg, model.ActivityQuery{
		Provider: model.ProviderGitHub, Repo: "o/r",
	}); err == nil {
		t.Fatal("NewActivity with an unparseable base URL: want an error")
	}
}

func TestCredentialHostVariants(t *testing.T) {
	tests := []struct {
		baseURL, fallback, want string
	}{
		{"", "github.com", "github.com"},
		{"https://ghe.example.com/api/v3/", "github.com", "ghe.example.com"},
		{"ghe.example.com", "github.com", "ghe.example.com"},
		{"ghe.example.com/api/v3", "github.com", "ghe.example.com"},
		{"https://gitlab.example.com/api/v4", "gitlab.com", "gitlab.example.com"},
		{"://nonsense", "github.com", "github.com"},
	}
	for _, tt := range tests {
		if got := credentialHost(tt.baseURL, tt.fallback); got != tt.want {
			t.Errorf("credentialHost(%q, %q) = %q, want %q", tt.baseURL, tt.fallback, got, tt.want)
		}
	}
}
