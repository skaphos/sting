// SPDX-License-Identifier: MIT
// Package ghclient wraps the go-github client with the commit-discovery
// strategies the tool needs and normalizes results into model types.
package ghclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v82/github"
	"github.com/skaphos/sting/model"
)

// apiError wraps a go-github error, giving rate-limit failures a clearer,
// agent-actionable message (an MCP client can decide to back off) while
// preserving the original error for errors.As/Is callers.
func apiError(op string, err error) error {
	var rl *github.RateLimitError
	if errors.As(err, &rl) {
		return fmt.Errorf("%s: github rate limit exceeded (resets %s): %w",
			op, rl.Rate.Reset.UTC().Format(time.RFC3339), err)
	}
	var ab *github.AbuseRateLimitError
	if errors.As(err, &ab) {
		return fmt.Errorf("%s: github secondary rate limit, retry later: %w", op, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}

// Client retrieves commits from GitHub for an author over a time window.
type Client struct {
	gh      *github.Client
	perPage int
}

// New builds a Client. token may be empty (unauthenticated, heavily rate
// limited). baseURL, when set, targets a GitHub Enterprise instance and must be
// the API root (e.g. "https://ghe.example.com/api/v3/"). perPage is clamped to
// the API's 1-100 range.
func New(token, baseURL string, perPage int) (*Client, error) {
	gh := github.NewClient(nil)
	if token != "" {
		gh = gh.WithAuthToken(token)
	}
	if baseURL != "" {
		var err error
		gh, err = gh.WithEnterpriseURLs(baseURL, baseURL)
		if err != nil {
			return nil, fmt.Errorf("configure enterprise URL: %w", err)
		}
	}
	if perPage < 1 {
		perPage = 100
	}
	if perPage > 100 {
		perPage = 100
	}
	return &Client{gh: gh, perPage: perPage}, nil
}

// Collect runs a query using its scope and returns the normalized result.
func (c *Client) Collect(ctx context.Context, q model.Query) (model.Result, error) {
	until := q.Until
	if until.IsZero() {
		until = time.Now()
	}
	q.Until = until

	var (
		commits []model.Commit
		err     error
	)
	switch q.Scope {
	case model.ScopeSearch:
		commits, err = c.searchByAuthor(ctx, q)
	case model.ScopeRepos:
		commits, err = c.listRepos(ctx, q)
	case model.ScopeOrg:
		commits, err = c.listOrg(ctx, q)
	default:
		return model.Result{}, fmt.Errorf("unsupported scope %q", q.Scope)
	}
	if err != nil {
		return model.Result{}, err
	}

	// The scope helpers stop fetching once they have MaxCommits, so reaching the
	// cap means more commits may exist upstream; clip to the cap and flag it.
	truncated := false
	if q.MaxCommits > 0 && len(commits) >= q.MaxCommits {
		commits = commits[:q.MaxCommits]
		truncated = true
	}

	return model.Result{
		SchemaVersion: model.SchemaVersion,
		GeneratedAt:   time.Now(),
		Author:        q.Author,
		Scope:         q.Scope,
		Since:         q.Since,
		Until:         q.Until,
		Count:         len(commits),
		Commits:       commits,
		Truncated:     truncated,
	}, nil
}

// searchByAuthor uses GitHub's global commit search index.
func (c *Client) searchByAuthor(ctx context.Context, q model.Query) ([]model.Commit, error) {
	query := buildSearchQuery(q)
	opts := &github.SearchOptions{
		Sort:        "author-date",
		Order:       "desc",
		ListOptions: github.ListOptions{PerPage: c.perPage},
	}
	var out []model.Commit
	for {
		res, resp, err := c.gh.Search.Commits(ctx, query, opts)
		if err != nil {
			return nil, apiError("search commits", err)
		}
		for _, cr := range res.Commits {
			out = append(out, fromSearchResult(cr))
		}
		if q.MaxCommits > 0 && len(out) >= q.MaxCommits {
			break
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return out, nil
}

// listRepos lists commits authored by q.Author across each "owner/repo" target.
func (c *Client) listRepos(ctx context.Context, q model.Query) ([]model.Commit, error) {
	if len(q.Repos) == 0 {
		return nil, fmt.Errorf("scope %q requires at least one repo", model.ScopeRepos)
	}
	var out []model.Commit
	for _, target := range q.Repos {
		owner, repo, ok := splitRepo(target)
		if !ok {
			return nil, fmt.Errorf("invalid repo %q (want owner/repo)", target)
		}
		commits, err := c.listRepoCommits(ctx, owner, repo, q)
		if err != nil {
			return nil, err
		}
		out = append(out, commits...)
		if q.MaxCommits > 0 && len(out) >= q.MaxCommits {
			break
		}
	}
	return out, nil
}

// listOrg enumerates an org's repositories and lists commits in each.
func (c *Client) listOrg(ctx context.Context, q model.Query) ([]model.Commit, error) {
	if q.Org == "" {
		return nil, fmt.Errorf("scope %q requires an org", model.ScopeOrg)
	}
	repos, err := c.orgRepos(ctx, q.Org)
	if err != nil {
		return nil, err
	}
	var out []model.Commit
	for _, full := range repos {
		owner, repo, ok := splitRepo(full)
		if !ok {
			continue
		}
		commits, err := c.listRepoCommits(ctx, owner, repo, q)
		if err != nil {
			return nil, err
		}
		out = append(out, commits...)
		if q.MaxCommits > 0 && len(out) >= q.MaxCommits {
			break
		}
	}
	return out, nil
}

func (c *Client) listRepoCommits(ctx context.Context, owner, repo string, q model.Query) ([]model.Commit, error) {
	opts := &github.CommitsListOptions{
		Author:      q.Author,
		Since:       q.Since,
		Until:       q.Until,
		ListOptions: github.ListOptions{PerPage: c.perPage},
	}
	full := owner + "/" + repo
	var out []model.Commit
	for {
		commits, resp, err := c.gh.Repositories.ListCommits(ctx, owner, repo, opts)
		if err != nil {
			return nil, apiError("list commits "+full, err)
		}
		for _, rc := range commits {
			cm := fromRepoCommit(full, rc)
			if q.IncludeStats {
				if err := c.fillStats(ctx, owner, repo, &cm); err != nil {
					return nil, err
				}
			}
			out = append(out, cm)
			if q.MaxCommits > 0 && len(out) >= q.MaxCommits {
				return out, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return out, nil
}

func (c *Client) orgRepos(ctx context.Context, org string) ([]string, error) {
	opts := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: c.perPage},
	}
	var out []string
	for {
		repos, resp, err := c.gh.Repositories.ListByOrg(ctx, org, opts)
		if err != nil {
			return nil, apiError("list org repos "+org, err)
		}
		for _, r := range repos {
			out = append(out, r.GetFullName())
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return out, nil
}

func (c *Client) fillStats(ctx context.Context, owner, repo string, cm *model.Commit) error {
	rc, _, err := c.gh.Repositories.GetCommit(ctx, owner, repo, cm.SHA, nil)
	if err != nil {
		return apiError(fmt.Sprintf("get commit stats %s/%s@%s", owner, repo, cm.SHA), err)
	}
	if s := rc.GetStats(); s != nil {
		cm.Additions = s.GetAdditions()
		cm.Deletions = s.GetDeletions()
	}
	return nil
}

func buildSearchQuery(q model.Query) string {
	parts := []string{"author:" + q.Author}
	if !q.Since.IsZero() || !q.Until.IsZero() {
		since := "*"
		if !q.Since.IsZero() {
			since = q.Since.UTC().Format("2006-01-02")
		}
		until := "*"
		if !q.Until.IsZero() {
			until = q.Until.UTC().Format("2006-01-02")
		}
		parts = append(parts, fmt.Sprintf("author-date:%s..%s", since, until))
	}
	// Scope qualifiers. A bare global author search returns public repos only;
	// adding org:/repo: qualifiers is what lets the search index reach private
	// repos the authenticated token can access (e.g. an SSO-authorized org).
	if q.Org != "" {
		parts = append(parts, "org:"+q.Org)
	}
	for _, repo := range q.Repos {
		if r := strings.TrimSpace(repo); r != "" {
			parts = append(parts, "repo:"+r)
		}
	}
	return strings.Join(parts, " ")
}

func fromSearchResult(cr *github.CommitResult) model.Commit {
	cm := model.Commit{
		SHA:     cr.GetSHA(),
		URL:     cr.GetHTMLURL(),
		Author:  cr.GetAuthor().GetLogin(),
		Repo:    cr.GetRepository().GetFullName(),
		Message: cr.GetCommit().GetMessage(),
	}
	if a := cr.GetCommit().GetAuthor(); a != nil {
		cm.AuthorName = a.GetName()
		cm.Email = a.GetEmail()
		cm.Date = a.GetDate().Time
	}
	return cm
}

func fromRepoCommit(repoFull string, rc *github.RepositoryCommit) model.Commit {
	cm := model.Commit{
		SHA:     rc.GetSHA(),
		URL:     rc.GetHTMLURL(),
		Author:  rc.GetAuthor().GetLogin(),
		Repo:    repoFull,
		Message: rc.GetCommit().GetMessage(),
	}
	if a := rc.GetCommit().GetAuthor(); a != nil {
		cm.AuthorName = a.GetName()
		cm.Email = a.GetEmail()
		cm.Date = a.GetDate().Time
	}
	return cm
}

func splitRepo(s string) (owner, repo string, ok bool) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
