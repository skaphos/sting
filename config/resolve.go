// SPDX-License-Identifier: MIT
package config

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/skaphos/sting/model"
)

// GitHub identity charsets. A GitHub login or organization is ASCII
// alphanumerics and hyphens; a repository is "owner/name" where the name may
// also contain dots and underscores; an email is a single token with no
// whitespace, colon, quote, or angle bracket. These are validated before the
// values are concatenated into a GitHub commit-search query (see
// ghclient.buildSearchQuery) so an attacker cannot inject an extra qualifier —
// e.g. an author of "victim author:attacker" would otherwise silently retarget
// the search. GitLab values are not validated here because the GitLab client
// URL-encodes every value, making structural injection impossible.
var (
	ghLoginRe    = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	ghRepoNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	ghEmailRe    = regexp.MustCompile(`^[^\s:"<>]+@[^\s:"<>]+$`)
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
	// MaxCommits overrides the default when non-nil.
	MaxCommits *int
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

	// GitHub author/org/repo values become bare tokens in a commit-search query,
	// so reject anything that could break out of a qualifier before it reaches
	// the client. GitLab is not validated here (its client URL-encodes values).
	if provider == model.ProviderGitHub {
		if err := validateGitHubIdentifiers(req.Author, org, repos); err != nil {
			return model.Query{}, err
		}
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
	maxCommits := cfg.MaxCommits
	if req.MaxCommits != nil {
		maxCommits = *req.MaxCommits
	}
	if maxCommits < 0 {
		return model.Query{}, fmt.Errorf("max_commits must be >= 0, got %d", maxCommits)
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
		MaxCommits:          maxCommits,
		IncludePullRequests: includePRs,
	}, nil
}

// validateGitHubIdentifiers rejects author/org/repo values that could break out
// of a GitHub commit-search qualifier. author may be a login or an email
// (including the "Name <addr>" form, matching ghclient.authorQualifier); org
// must be a GitHub login/org; each repo must be "owner/name".
func validateGitHubIdentifiers(author, org string, repos []string) error {
	if !validGitHubAuthor(author) {
		return fmt.Errorf("invalid github author %q: must be a login ([A-Za-z0-9-]) or an email", author)
	}
	if org != "" && !ghLoginRe.MatchString(org) {
		return fmt.Errorf("invalid github org %q: must match [A-Za-z0-9-]", org)
	}
	for _, repo := range repos {
		r := strings.TrimSpace(repo)
		if r == "" {
			continue
		}
		if !validGitHubRepo(r) {
			return fmt.Errorf("invalid github repo %q: must be owner/name with no spaces or qualifier characters", repo)
		}
	}
	return nil
}

func validGitHubAuthor(author string) bool {
	// An email (including "Name <addr>") is validated as its bare address, which
	// mirrors how the search qualifier uses it. Parsing is not sufficient on its
	// own: a quoted local part can carry spaces, so the address is re-checked
	// against a whitespace/qualifier-free charset.
	if addr, err := mail.ParseAddress(author); err == nil && strings.Contains(author, "@") {
		return ghEmailRe.MatchString(addr.Address)
	}
	return ghLoginRe.MatchString(author)
}

func validGitHubRepo(repo string) bool {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return false
	}
	return ghLoginRe.MatchString(owner) && ghRepoNameRe.MatchString(name)
}
