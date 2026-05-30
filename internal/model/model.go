// Package model holds the domain types shared across the GitHub commit-query
// tool. It is a leaf package with no internal dependencies so both the GitHub
// client and the renderers can depend on it without creating import cycles.
package model

import "time"

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
	// Author is the GitHub login (or, for search, may be an email) whose
	// commits are wanted.
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
	// MaxCommits caps the number of commits returned (0 = no cap).
	MaxCommits int
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
	Additions  int       `json:"additions,omitempty"`
	Deletions  int       `json:"deletions,omitempty"`
}

// Summary is the first line of the commit message.
func (c Commit) Summary() string {
	for i := 0; i < len(c.Message); i++ {
		if c.Message[i] == '\n' {
			return c.Message[:i]
		}
	}
	return c.Message
}

// Result is the outcome of a Query: the matching commits plus the parameters
// that produced them, suitable for direct serialization.
type Result struct {
	Author    string    `json:"author"`
	Scope     Scope     `json:"scope"`
	Since     time.Time `json:"since"`
	Until     time.Time `json:"until"`
	Count     int       `json:"count"`
	Commits   []Commit  `json:"commits"`
	Truncated bool      `json:"truncated,omitempty"` // true if MaxCommits clipped results
}
