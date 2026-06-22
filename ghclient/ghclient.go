// SPDX-License-Identifier: MIT
// Package ghclient wraps the go-github client with the commit-discovery
// strategies the tool needs and normalizes results into model types.
package ghclient

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/go-github/v82/github"
	"github.com/skaphos/sting/internal/patch"
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
		Provider:      model.ProviderGitHub,
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
			cm := fromSearchResult(cr)
			if needsDetail(q) {
				owner, repo, ok := splitRepo(cm.Repo)
				if !ok {
					return nil, fmt.Errorf("invalid repo %q from search result", cm.Repo)
				}
				if err := c.fillDetails(ctx, owner, repo, &cm, q); err != nil {
					return nil, err
				}
			}
			out = append(out, cm)
			if q.MaxCommits > 0 && len(out) >= q.MaxCommits {
				break
			}
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
		commits, err := c.collectRepo(ctx, owner, repo, q)
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
		commits, err := c.collectRepo(ctx, owner, repo, q)
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

// collectRepo gathers a repo's default-branch commits and, when the query opts
// into pull-request discovery, the author-matching commits on open PR branches
// that are not yet reachable from the default branch. The seen set dedups SHAs
// within this one repository — across the default-branch listing and every open
// PR (a commit can appear in multiple PRs and may already be on the default
// branch). It is intentionally per-repo: a SHA is only unique within a repo, so
// deduping across repos (e.g. forks with shared history) would drop legitimate
// evidence.
func (c *Client) collectRepo(ctx context.Context, owner, repo string, q model.Query) ([]model.Commit, error) {
	seen := map[string]bool{}
	commits, err := c.listRepoCommits(ctx, owner, repo, q, seen)
	if err != nil {
		return nil, err
	}
	if q.IncludePullRequests {
		// Keep PR discovery within the per-repo cap: the default-branch listing
		// may have already filled it, in which case skip PR enumeration entirely
		// (no wasted API calls), otherwise only fetch the remaining budget so the
		// combined per-repo result never exceeds MaxCommits and the final clip in
		// Collect cannot drop PR-branch evidence that was within budget.
		prQuery := q
		if q.MaxCommits > 0 {
			remaining := q.MaxCommits - len(commits)
			if remaining <= 0 {
				return commits, nil
			}
			prQuery.MaxCommits = remaining
		}
		prCommits, err := c.pullRequestCommits(ctx, owner, repo, prQuery, seen)
		if err != nil {
			return nil, err
		}
		commits = append(commits, prCommits...)
	}
	return commits, nil
}

func (c *Client) listRepoCommits(ctx context.Context, owner, repo string, q model.Query, seen map[string]bool) ([]model.Commit, error) {
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
			if seen[cm.SHA] {
				continue
			}
			seen[cm.SHA] = true
			if needsDetail(q) {
				if err := c.fillDetails(ctx, owner, repo, &cm, q); err != nil {
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

// pullRequestCommits enumerates a repo's open pull requests and returns the
// author-matching commits on their branches within the query window. These
// commits are evidence that commit search and default-branch listing miss
// because the work is not yet merged. Each commit is tagged with its PR source.
func (c *Client) pullRequestCommits(ctx context.Context, owner, repo string, q model.Query, seen map[string]bool) ([]model.Commit, error) {
	full := owner + "/" + repo
	prOpts := &github.PullRequestListOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: c.perPage},
	}
	var out []model.Commit
	for {
		prs, resp, err := c.gh.PullRequests.List(ctx, owner, repo, prOpts)
		if err != nil {
			return nil, apiError("list pull requests "+full, err)
		}
		for _, pr := range prs {
			commits, err := c.prBranchCommits(ctx, owner, repo, pr.GetNumber(), q, seen)
			if err != nil {
				return nil, err
			}
			out = append(out, commits...)
			if q.MaxCommits > 0 && len(out) >= q.MaxCommits {
				return out, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		prOpts.Page = resp.NextPage
	}
	return out, nil
}

// prBranchCommits returns the author-matching, in-window, not-yet-seen commits
// on a single pull request, tagged with the PR as their discovery source.
func (c *Client) prBranchCommits(ctx context.Context, owner, repo string, number int, q model.Query, seen map[string]bool) ([]model.Commit, error) {
	full := owner + "/" + repo
	source := fmt.Sprintf("pull/%d", number)
	opts := &github.ListOptions{PerPage: c.perPage}
	var out []model.Commit
	for {
		commits, resp, err := c.gh.PullRequests.ListCommits(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, apiError(fmt.Sprintf("list pr commits %s#%d", full, number), err)
		}
		for _, rc := range commits {
			cm := fromRepoCommit(full, rc)
			cm.Source = source
			if !authorMatches(cm, q.Author) || !inWindow(cm.Date, q.Since, q.Until) {
				continue
			}
			if seen[cm.SHA] {
				continue
			}
			seen[cm.SHA] = true
			if needsDetail(q) {
				if err := c.fillDetails(ctx, owner, repo, &cm, q); err != nil {
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

// authorEmail returns the bare email address when author is an email — including
// the "Name <user@example.com>" form that mail.ParseAddress accepts — and
// whether it was an email at all. A GitHub login cannot contain "@", so its
// presence is the signal. It is the single normalization point shared by the
// search qualifier and PR-branch matching so the two cannot diverge.
func authorEmail(author string) (string, bool) {
	if addr, err := mail.ParseAddress(author); err == nil && strings.Contains(author, "@") {
		return addr.Address, true
	}
	return author, false
}

// authorMatches reports whether a commit was authored by the queried identity.
// The query author may be a GitHub login (matched against the attributed login)
// or a commit email (matched against the raw author email), mirroring the
// login/email distinction the search path makes. Email input is normalized to
// the bare address so the "Name <addr>" form matches just as it does in search.
func authorMatches(cm model.Commit, author string) bool {
	target := author
	if email, ok := authorEmail(author); ok {
		target = email
	}
	a := strings.ToLower(strings.TrimSpace(target))
	if a == "" {
		return false
	}
	return strings.ToLower(cm.Author) == a || strings.ToLower(cm.Email) == a
}

// inWindow reports whether t falls within [since, until]. A zero bound is open.
func inWindow(t, since, until time.Time) bool {
	if !since.IsZero() && t.Before(since) {
		return false
	}
	if !until.IsZero() && t.After(until) {
		return false
	}
	return true
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

func needsDetail(q model.Query) bool {
	return q.IncludeStats || q.IncludeFiles || q.IncludeDiffs
}

func (c *Client) fillDetails(ctx context.Context, owner, repo string, cm *model.Commit, q model.Query) error {
	opts := &github.ListOptions{PerPage: c.perPage}
	var files []*github.CommitFile
	needFiles := q.IncludeFiles || q.IncludeDiffs
	for {
		rc, resp, err := c.gh.Repositories.GetCommit(ctx, owner, repo, cm.SHA, opts)
		if err != nil {
			return apiError(fmt.Sprintf("get commit details %s/%s@%s", owner, repo, cm.SHA), err)
		}
		if s := rc.GetStats(); s != nil {
			cm.Additions = s.GetAdditions()
			cm.Deletions = s.GetDeletions()
			cm.Changes = s.GetTotal()
		}
		if needFiles {
			files = append(files, rc.Files...)
		}
		if resp.NextPage == 0 || !needFiles {
			break
		}
		opts.Page = resp.NextPage
	}
	if needFiles {
		cm.Files = githubFiles(files, q)
	}
	if cm.Changes == 0 && (cm.Additions != 0 || cm.Deletions != 0) {
		cm.Changes = cm.Additions + cm.Deletions
	}
	return nil
}

func githubFiles(files []*github.CommitFile, q model.Query) []model.File {
	out := make([]model.File, 0, len(files))
	budget := q.MaxDiffBytes
	if budget <= 0 {
		budget = model.DefaultMaxDiffBytes
	}
	for _, f := range files {
		mf := model.File{
			Path:         f.GetFilename(),
			PreviousPath: f.GetPreviousFilename(),
			Status:       f.GetStatus(),
			Additions:    f.GetAdditions(),
			Deletions:    f.GetDeletions(),
			Changes:      f.GetChanges(),
		}
		if q.IncludeDiffs {
			mf.Patch, mf.PatchTruncated, budget = patch.ConsumePatchBudget(f.GetPatch(), budget)
		}
		out = append(out, mf)
	}
	return out
}

// authorQualifier picks the GitHub commit-search qualifier for the author
// input. A raw commit email must use author-email: — GitHub's commit search
// does not match an email against the author: qualifier, so emitting author:
// for an email silently returns zero results. It uses the bare parsed address
// (via authorEmail), so the "Name <user@example.com>" form yields a valid
// qualifier rather than one with spaces/angle brackets.
func authorQualifier(author string) string {
	if email, ok := authorEmail(author); ok {
		return "author-email:" + email
	}
	return "author:" + author
}

func buildSearchQuery(q model.Query) string {
	parts := []string{authorQualifier(q.Author)}
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
		Source:  "search",
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
		Source:  "repo",
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
