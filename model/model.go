// SPDX-License-Identifier: MIT
// Package model holds the domain types shared across commit-query providers.
// It is a leaf package with no internal dependencies so provider clients and
// renderers can depend on it without creating import cycles.
package model

import (
	"strings"
	"time"
)

// SchemaVersion identifies the sting Result contract. It is emitted on every
// Result so downstream consumers (e.g. a Wake evidence adapter) can pin the
// shape they map from and detect drift. Bump it on any breaking change to
// Result or Commit.
const SchemaVersion = "sting.skaphos.io/v2"

// DefaultMaxDiffBytes is the default per-commit patch-text budget used when a
// query requests full diffs but does not set an explicit limit.
const DefaultMaxDiffBytes = 60000

// Provider identifies the source control provider a query targets.
type Provider string

const (
	ProviderGitHub Provider = "github"
	ProviderGitLab Provider = "gitlab"
)

// Valid reports whether p is a recognized provider.
func (p Provider) Valid() bool {
	switch p {
	case ProviderGitHub, ProviderGitLab:
		return true
	default:
		return false
	}
}

// Scope selects how commits are discovered for an author.
type Scope string

const (
	// ScopeSearch uses GitHub's global commit search index (author across all
	// indexed public repositories). Broad but limited to indexed/public repos.
	ScopeSearch Scope = "search"
	// ScopeRepos lists commits within an explicit set of "owner/repo" targets.
	ScopeRepos Scope = "repos"
	// ScopeOrg enumerates an organization's repositories and lists commits in each.
	ScopeOrg Scope = "org"
)

// Valid reports whether s is a recognized scope.
func (s Scope) Valid() bool {
	switch s {
	case ScopeSearch, ScopeRepos, ScopeOrg:
		return true
	default:
		return false
	}
}

// Query describes a single commit-retrieval request.
type Query struct {
	// Provider is the source control provider to query.
	Provider Provider
	// Author is the provider author identifier whose commits are wanted. For
	// GitHub this is a login or an email; in the search scope an email is
	// matched with the author-email: qualifier and a login with author:. For
	// GitLab this is matched against the commit author string.
	Author string
	// Since and Until bound the commit author date, inclusive. A zero Until
	// means "now".
	Since time.Time
	Until time.Time
	// Scope selects the discovery strategy.
	Scope Scope
	// Repos is the list of "owner/repo" targets for ScopeRepos.
	Repos []string
	// Org is the organization login for ScopeOrg.
	Org string
	// IncludeStats requests per-commit additions/deletions. This costs one
	// extra API call per commit, so it is off by default.
	IncludeStats bool
	// IncludeFiles requests per-file change summaries. Providers usually fetch
	// this from the same detail endpoint as stats.
	IncludeFiles bool
	// IncludeDiffs requests patch text for changed files. This implies
	// IncludeFiles and is bounded by MaxDiffBytes.
	IncludeDiffs bool
	// MaxDiffBytes caps patch text per commit when IncludeDiffs is true.
	MaxDiffBytes int
	// MaxCommits caps the number of commits returned (0 = no cap).
	MaxCommits int
	// IncludePullRequests augments repos/org discovery with commits found on
	// open pull-request branches. These commits are not yet on a default branch,
	// so commit search and branch listing miss them; enabling this enumerates
	// open PRs per repo and merges author-matching commits as evidence. It costs
	// extra API calls (one PR list + one commit list per PR), so it is off by
	// default. GitHub only; ignored for GitLab.
	IncludePullRequests bool
}

// Commit is a normalized commit record independent of the GitHub API shape.
type Commit struct {
	SHA        string    `json:"sha"`
	Repo       string    `json:"repo"`             // "owner/repo"
	Author     string    `json:"author,omitempty"` // GitHub login, if known
	AuthorName string    `json:"author_name"`      // git author name
	Email      string    `json:"email,omitempty"`  // git author email
	Date       time.Time `json:"date"`             // git author date
	Message    string    `json:"message"`          // full commit message
	URL        string    `json:"url"`              // html_url
	// Source records how the commit was discovered so a match is auditable:
	// "search" (commit search index), "repo" (default-branch listing), or
	// "pull/<n>" (open pull-request branch). Empty for provider paths that do
	// not tag a source (e.g. GitLab).
	Source    string `json:"source,omitempty"`
	Additions int    `json:"additions,omitempty"`
	Deletions int    `json:"deletions,omitempty"`
	Changes   int    `json:"changes,omitempty"`
	Files     []File `json:"files,omitempty"`
}

// File is a normalized file-level change record for a commit.
type File struct {
	Path           string `json:"path"`
	PreviousPath   string `json:"previous_path,omitempty"`
	Status         string `json:"status,omitempty"`
	Additions      int    `json:"additions,omitempty"`
	Deletions      int    `json:"deletions,omitempty"`
	Changes        int    `json:"changes,omitempty"`
	Patch          string `json:"patch,omitempty"`
	PatchTruncated bool   `json:"patch_truncated,omitempty"`
}

// Summary is the first line of the commit message.
func (c Commit) Summary() string {
	first, _, _ := strings.Cut(c.Message, "\n")
	return first
}

// Result is the outcome of a Query: the matching commits plus the parameters
// that produced them, suitable for direct serialization.
type Result struct {
	// SchemaVersion pins the Result contract (see the package SchemaVersion
	// constant). GeneratedAt records when the query ran, giving the result
	// evidence-style provenance.
	SchemaVersion string    `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	Provider      Provider  `json:"provider,omitempty"`
	Author        string    `json:"author"`
	Scope         Scope     `json:"scope"`
	Since         time.Time `json:"since"`
	Until         time.Time `json:"until"`
	Count         int       `json:"count"`
	Commits       []Commit  `json:"commits"`
	Truncated     bool      `json:"truncated,omitempty"` // true if MaxCommits clipped results
	// Skipped lists repositories an org-scope scan could not list and skipped so
	// that one bad repo (e.g. an empty repo, or one the token cannot read) does
	// not abort the whole scan. Empty/omitted when nothing was skipped.
	Skipped []SkippedRepo `json:"skipped,omitempty"`
}

// SkippedRepo records a repository that an org-scope scan could not list and
// chose to skip rather than abort the whole scan. Reason is a short,
// human-readable cause (e.g. "empty repository", "not found"). Surfacing skips
// keeps a partial result auditable instead of silently dropping the repo.
type SkippedRepo struct {
	Repo   string `json:"repo"`
	Reason string `json:"reason"`
}
