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

// Default returns the built-in configuration as a Config value. This is the
// single source of truth for default values; Defaults() derives its map
// representation from it so the two cannot drift apart.
//
// Default is intentional public API consumed by the sibling wake project
// (see ADR 0004) in addition to Defaults(), which is what the sting CLI
// itself seeds viper with.
func Default() Config {
	return Config{
		DefaultProvider: model.ProviderGitHub,
		DefaultScope:    model.ScopeSearch,
		DefaultWindow:   "7d",
		DefaultRepos:    []string{},
		DefaultFormat:   "markdown",
		PerPage:         100,
		MaxCommits:      0,
		IncludeStats:    false,
		IncludeFiles:    false,
		IncludeDiffs:    false,
		MaxDiffBytes:    model.DefaultMaxDiffBytes,
	}
}

// Defaults are the built-in configuration values, keyed by their canonical
// config key. The CLI seeds viper with these so they participate uniformly in
// precedence resolution. It is derived from Default() so the map and struct
// representations of the built-in configuration stay in sync.
func Defaults() map[string]any {
	d := Default()
	return map[string]any{
		"provider":        string(d.DefaultProvider),
		"token":           d.Token,
		"base_url":        d.BaseURL,
		"gitlab_token":    d.GitLabToken,
		"gitlab_base_url": d.GitLabBaseURL,
		"default_scope":   string(d.DefaultScope),
		"default_window":  d.DefaultWindow,
		"default_repos":   d.DefaultRepos,
		"default_org":     d.DefaultOrg,
		"default_format":  d.DefaultFormat,
		"per_page":        d.PerPage,
		"max_commits":     d.MaxCommits,
		"include_stats":   d.IncludeStats,
		"include_files":   d.IncludeFiles,
		"include_diffs":   d.IncludeDiffs,
		"max_diff_bytes":  d.MaxDiffBytes,
		"include_prs":     d.IncludePullRequests,
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
	if cfg.DefaultProvider == model.ProviderGitLab && cfg.DefaultScope == model.ScopeSearch {
		return fmt.Errorf("provider %q does not support default_scope %q (use repos or org)", cfg.DefaultProvider, cfg.DefaultScope)
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
		multiplier := int64(1)
		if unit == 'w' {
			multiplier = 7
		}
		// Detect overflow before it happens: n * multiplier * day must stay
		// within int64 and non-negative, or a huge window (e.g. "20000w")
		// silently wraps into a negative duration that later passes
		// Validate and fails every query with a misleading error.
		const maxWindow = 100 * 365 * 24 * time.Hour // a sane cap: 100 years
		if n != 0 && int64(n) > int64(maxWindow)/(multiplier*int64(day)) {
			return 0, fmt.Errorf("window %q exceeds maximum supported duration", s)
		}
		return time.Duration(n) * time.Duration(multiplier) * day, nil
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
