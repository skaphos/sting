// SPDX-License-Identifier: MIT
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/skaphos/sting/model"
)

// ActivityRequest is the raw, mostly-string repository-activity input from a
// CLI invocation or an MCP tool call. It mirrors the Request pattern so the CLI
// and MCP share exactly one validation path and cannot drift in what they
// accept or how they phrase a rejection.
type ActivityRequest struct {
	Provider string // github only; empty uses the configured default
	Repo     string // "owner/name"
	Ref      string // branch or tag; empty means the default branch
	Since    string // RFC3339 or YYYY-MM-DD; empty uses Window
	Until    string // RFC3339 or YYYY-MM-DD; empty means now
	Window   string // look-back (e.g. "7d"); used only when Since is empty
	Author   string // optional narrowing filter on the commit listing

	// IncludeDiffs overrides the default when non-nil.
	IncludeDiffs *bool
	// MaxDiffBytes overrides the default when non-nil.
	MaxDiffBytes *int
	// EnrichCommits overrides the default (0) when non-nil.
	EnrichCommits *int
	// MaxRequests overrides the default when non-nil. An explicit 0 is
	// meaningful (uncapped), which is why this is a pointer.
	MaxRequests *int
	// EstimateOnly reports projected cost without gathering evidence.
	EstimateOnly bool
}

// ResolveActivity validates and normalizes an ActivityRequest into an
// ActivityQuery, applying flags > env > file > defaults precedence.
//
// It is the single place the activity window is normalized — once, to UTC — and
// the single place a non-GitHub provider is rejected, so the failure is
// identical whether the request arrived from the CLI or from MCP and is
// impossible to reach with a half-built client behind it. The reference time
// now is injected for testability.
func (cfg Config) ResolveActivity(req ActivityRequest, now time.Time) (model.ActivityQuery, error) {
	provider := model.Provider(req.Provider)
	if provider == "" {
		provider = cfg.DefaultProvider
	}
	if provider == "" {
		provider = model.ProviderGitHub
	}
	if !provider.Valid() {
		return model.ActivityQuery{}, fmt.Errorf("invalid provider %q (want github|gitlab)", provider)
	}
	// Repository activity is GitHub-only. Rejecting here rather than behind a
	// stub client means the gap is stated plainly instead of surfacing as a
	// not-implemented error after credentials have already been resolved.
	if provider != model.ProviderGitHub {
		return model.ActivityQuery{}, fmt.Errorf("provider %q does not support repository activity (github only)", provider)
	}

	repo := strings.TrimSpace(req.Repo)
	if repo == "" {
		return model.ActivityQuery{}, fmt.Errorf("repo is required (owner/name)")
	}
	if !validGitHubRepo(repo) {
		return model.ActivityQuery{}, fmt.Errorf("invalid github repo %q: must be owner/name with no spaces or qualifier characters", req.Repo)
	}

	since, until, err := resolveWindow(req.Since, req.Until, req.Window, cfg.DefaultWindow, now)
	if err != nil {
		return model.ActivityQuery{}, err
	}

	includeDiffs := cfg.IncludeDiffs
	if req.IncludeDiffs != nil {
		includeDiffs = *req.IncludeDiffs
	}

	maxDiffBytes := cfg.MaxDiffBytes
	if maxDiffBytes == 0 {
		maxDiffBytes = model.DefaultMaxDiffBytes
	}
	if req.MaxDiffBytes != nil {
		maxDiffBytes = *req.MaxDiffBytes
	}
	if maxDiffBytes < 0 {
		return model.ActivityQuery{}, fmt.Errorf("max_diff_bytes must be >= 0, got %d", maxDiffBytes)
	}

	enrichCommits := 0
	if req.EnrichCommits != nil {
		enrichCommits = *req.EnrichCommits
	}
	if enrichCommits < 0 {
		return model.ActivityQuery{}, fmt.Errorf("enrich_commits must be >= 0, got %d", enrichCommits)
	}

	maxRequests := cfg.MaxRequests
	if req.MaxRequests != nil {
		maxRequests = *req.MaxRequests
	}
	if maxRequests < 0 {
		return model.ActivityQuery{}, fmt.Errorf("max_requests must be >= 0, got %d", maxRequests)
	}

	// Normalize the window to UTC exactly once, here. Downstream code compares
	// and formats these values without re-deriving them, so an identical
	// request at a fixed now always produces an identical query.
	return model.ActivityQuery{
		Provider:      provider,
		Repo:          repo,
		Ref:           strings.TrimSpace(req.Ref),
		Since:         since,
		Until:         until,
		Author:        strings.TrimSpace(req.Author),
		IncludeDiffs:  includeDiffs,
		MaxDiffBytes:  maxDiffBytes,
		EnrichCommits: enrichCommits,
		MaxRequests:   maxRequests,
		EstimateOnly:  req.EstimateOnly,
	}, nil
}
