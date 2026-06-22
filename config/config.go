// SPDX-License-Identifier: MIT
// Package config defines the tool's settings and the flexible time-window
// parsing used to bound a query. Loading and source precedence (config file,
// environment, flags) is handled by viper in the CLI layer; this package stays
// dependency-light so provider clients and renderers can rely on it.
package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/skaphos/sting/model"
)

// Config holds all tunable settings. The mapstructure keys are the canonical
// configuration keys: they are the YAML/JSON config-file keys, the viper keys
// bound to flags, and (uppercased, STING_-prefixed) the environment variables.
type Config struct {
	// DefaultProvider is used when a query omits a provider.
	DefaultProvider model.Provider `mapstructure:"provider"`
	// Token is the GitHub personal access token sting authenticates with. It is
	// deliberately sting's own key (config-file "token" or env STING_TOKEN), kept
	// separate from the ambient GITHUB_TOKEN so a dedicated read-only PAT can be
	// the default without colliding with other tools' credentials.
	Token string `mapstructure:"token"`
	// BaseURL points at a GitHub Enterprise API root
	// ("https://ghe.example.com/api/v3/"). Empty means public github.com.
	BaseURL string `mapstructure:"base_url"`
	// GitLabToken is the GitLab personal access token sting authenticates with.
	// It is kept separate from both GITHUB_TOKEN and ambient GitLab env vars such
	// as GITLAB_TOKEN.
	GitLabToken string `mapstructure:"gitlab_token"`
	// GitLabBaseURL points at a GitLab API v4 root
	// ("https://gitlab.example.com/api/v4/"). Empty means GitLab.com.
	GitLabBaseURL string `mapstructure:"gitlab_base_url"`
	// DefaultScope is used when a query omits a scope.
	DefaultScope model.Scope `mapstructure:"default_scope"`
	// DefaultWindow is the look-back window when a query omits since/until.
	DefaultWindow string `mapstructure:"default_window"`
	// DefaultRepos seeds ScopeRepos queries that omit repos.
	DefaultRepos []string `mapstructure:"default_repos"`
	// DefaultOrg seeds ScopeOrg queries that omit org.
	DefaultOrg string `mapstructure:"default_org"`
	// DefaultFormat is the CLI render format ("markdown" or "json").
	DefaultFormat string `mapstructure:"default_format"`
	// PerPage is the API page size (1-100).
	PerPage int `mapstructure:"per_page"`
	// MaxCommits caps results per query (0 = unlimited).
	MaxCommits int `mapstructure:"max_commits"`
	// IncludeStats fetches per-commit line stats by default.
	IncludeStats bool `mapstructure:"include_stats"`
	// IncludeFiles fetches per-file change summaries by default.
	IncludeFiles bool `mapstructure:"include_files"`
	// IncludeDiffs fetches bounded patch text by default.
	IncludeDiffs bool `mapstructure:"include_diffs"`
	// MaxDiffBytes caps patch text per commit when diffs are requested.
	MaxDiffBytes int `mapstructure:"max_diff_bytes"`
	// IncludePullRequests augments repos/org discovery with open-PR branch
	// commits by default (GitHub only).
	IncludePullRequests bool `mapstructure:"include_prs"`
}

// Defaults are the built-in configuration values, keyed by their canonical
// config key. The CLI seeds viper with these so they participate uniformly in
// precedence resolution.
func Defaults() map[string]any {
	return map[string]any{
		"provider":        string(model.ProviderGitHub),
		"token":           "",
		"base_url":        "",
		"gitlab_token":    "",
		"gitlab_base_url": "",
		"default_scope":   string(model.ScopeSearch),
		"default_window":  "7d",
		"default_repos":   []string{},
		"default_org":     "",
		"default_format":  "markdown",
		"per_page":        100,
		"max_commits":     0,
		"include_stats":   false,
		"include_files":   false,
		"include_diffs":   false,
		"max_diff_bytes":  model.DefaultMaxDiffBytes,
		"include_prs":     false,
	}
}

// Default returns the built-in configuration as a Config value.
func Default() Config {
	return Config{
		DefaultProvider: model.ProviderGitHub,
		DefaultScope:    model.ScopeSearch,
		DefaultWindow:   "7d",
		DefaultFormat:   "markdown",
		PerPage:         100,
		MaxCommits:      0,
		IncludeStats:    false,
		IncludeFiles:    false,
		IncludeDiffs:    false,
		MaxDiffBytes:    model.DefaultMaxDiffBytes,
	}
}

// Validate checks that the resolved configuration is internally consistent.
func (cfg Config) Validate() error {
	if cfg.DefaultProvider != "" && !cfg.DefaultProvider.Valid() {
		return fmt.Errorf("invalid provider %q (want github|gitlab)", cfg.DefaultProvider)
	}
	if cfg.DefaultScope != "" && !cfg.DefaultScope.Valid() {
		return fmt.Errorf("invalid default_scope %q (want search|repos|org)", cfg.DefaultScope)
	}
	if cfg.PerPage < 1 || cfg.PerPage > 100 {
		return fmt.Errorf("per_page must be 1-100, got %d", cfg.PerPage)
	}
	if cfg.MaxDiffBytes < 0 {
		return fmt.Errorf("max_diff_bytes must be >= 0, got %d", cfg.MaxDiffBytes)
	}
	if cfg.DefaultWindow != "" {
		if _, err := ParseWindow(cfg.DefaultWindow); err != nil {
			return fmt.Errorf("invalid default_window: %w", err)
		}
	}
	return nil
}

// ParseWindow turns a look-back string into a duration. It accepts Go durations
// ("48h", "30m") plus the day/week suffixes "d" and "w" (e.g. "7d", "2w").
func ParseWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty window")
	}
	switch unit := s[len(s)-1]; unit {
	case 'd', 'w':
		n, err := strconv.Atoi(strings.TrimSpace(s[:len(s)-1]))
		if err != nil {
			return 0, fmt.Errorf("invalid window %q: %w", s, err)
		}
		if n < 0 {
			return 0, fmt.Errorf("negative window %q", s)
		}
		day := 24 * time.Hour
		if unit == 'w' {
			return time.Duration(n) * 7 * day, nil
		}
		return time.Duration(n) * day, nil
	default:
		d, err := time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("invalid window %q: %w", s, err)
		}
		if d < 0 {
			return 0, fmt.Errorf("negative window %q", s)
		}
		return d, nil
	}
}

// ParseTime parses a since/until bound. It accepts RFC3339 timestamps and the
// date form "2006-01-02" (interpreted in UTC).
func ParseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q (want RFC3339 or YYYY-MM-DD)", s)
}
