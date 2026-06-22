// SPDX-License-Identifier: MIT
package config

import (
	"fmt"
	"time"

	"github.com/skaphos/sting/model"
)

// Request is the raw, mostly-string input from a CLI invocation or an MCP tool
// call. Empty fields fall back to configuration defaults during Resolve.
type Request struct {
	Provider string // github|gitlab; empty uses default
	Author   string
	Since    string // RFC3339 or YYYY-MM-DD; empty uses Window
	Until    string // RFC3339 or YYYY-MM-DD; empty means now
	Window   string // look-back (e.g. "7d"); used only when Since is empty
	Scope    string // search|repos|org; empty uses default
	Repos    []string
	Org      string

	// IncludeStats overrides the default when non-nil.
	IncludeStats *bool
	// IncludeFiles overrides the default when non-nil.
	IncludeFiles *bool
	// IncludeDiffs overrides the default when non-nil.
	IncludeDiffs *bool
	// MaxDiffBytes overrides the default when non-nil.
	MaxDiffBytes *int
	// IncludePullRequests overrides the default when non-nil.
	IncludePullRequests *bool
}

// Resolve turns a Request into a validated model.Query, applying defaults from
// cfg. The reference time now is injected for testability.
func (cfg Config) Resolve(req Request, now time.Time) (model.Query, error) {
	if req.Author == "" {
		return model.Query{}, fmt.Errorf("author is required")
	}

	provider := model.Provider(req.Provider)
	if provider == "" {
		provider = cfg.DefaultProvider
	}
	if provider == "" {
		provider = model.ProviderGitHub
	}
	if !provider.Valid() {
		return model.Query{}, fmt.Errorf("invalid provider %q (want github|gitlab)", provider)
	}

	scope := model.Scope(req.Scope)
	if scope == "" {
		scope = cfg.DefaultScope
	}
	if !scope.Valid() {
		return model.Query{}, fmt.Errorf("invalid scope %q (want search|repos|org)", scope)
	}
	if provider == model.ProviderGitLab && scope == model.ScopeSearch {
		return model.Query{}, fmt.Errorf("provider %q does not support scope %q (use repos or org)", provider, scope)
	}

	until := now
	if req.Until != "" {
		t, err := ParseTime(req.Until)
		if err != nil {
			return model.Query{}, fmt.Errorf("until: %w", err)
		}
		until = t
	}

	var since time.Time
	switch {
	case req.Since != "":
		t, err := ParseTime(req.Since)
		if err != nil {
			return model.Query{}, fmt.Errorf("since: %w", err)
		}
		since = t
	default:
		window := req.Window
		if window == "" {
			window = cfg.DefaultWindow
		}
		d, err := ParseWindow(window)
		if err != nil {
			return model.Query{}, fmt.Errorf("window: %w", err)
		}
		since = until.Add(-d)
	}

	if since.After(until) {
		return model.Query{}, fmt.Errorf("since (%s) is after until (%s)",
			since.Format(time.RFC3339), until.Format(time.RFC3339))
	}

	repos := req.Repos
	if len(repos) == 0 {
		repos = cfg.DefaultRepos
	}
	org := req.Org
	if org == "" {
		org = cfg.DefaultOrg
	}

	includeStats := cfg.IncludeStats
	if req.IncludeStats != nil {
		includeStats = *req.IncludeStats
	}
	includeFiles := cfg.IncludeFiles
	if req.IncludeFiles != nil {
		includeFiles = *req.IncludeFiles
	}
	includeDiffs := cfg.IncludeDiffs
	if req.IncludeDiffs != nil {
		includeDiffs = *req.IncludeDiffs
	}
	if includeDiffs {
		includeFiles = true
	}
	maxDiffBytes := cfg.MaxDiffBytes
	if maxDiffBytes == 0 {
		maxDiffBytes = model.DefaultMaxDiffBytes
	}
	if req.MaxDiffBytes != nil {
		maxDiffBytes = *req.MaxDiffBytes
	}
	if maxDiffBytes < 0 {
		return model.Query{}, fmt.Errorf("max_diff_bytes must be >= 0, got %d", maxDiffBytes)
	}

	includePRs := cfg.IncludePullRequests
	if req.IncludePullRequests != nil {
		includePRs = *req.IncludePullRequests
	}

	return model.Query{
		Provider:            provider,
		Author:              req.Author,
		Since:               since,
		Until:               until,
		Scope:               scope,
		Repos:               repos,
		Org:                 org,
		IncludeStats:        includeStats,
		IncludeFiles:        includeFiles,
		IncludeDiffs:        includeDiffs,
		MaxDiffBytes:        maxDiffBytes,
		MaxCommits:          cfg.MaxCommits,
		IncludePullRequests: includePRs,
	}, nil
}
