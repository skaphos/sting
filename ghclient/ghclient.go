// SPDX-License-Identifier: MIT
// Package ghclient wraps the go-github client with the commit-discovery
// strategies the tool needs and normalizes results into model types.
package ghclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/google/go-github/v82/github"
	"github.com/skaphos/sting/internal/patch"
	"github.com/skaphos/sting/model"
	"golang.org/x/sync/errgroup"
)

// httpTimeout bounds each HTTP request the client makes so a hung connection
// cannot stall a scan indefinitely. It is per-request (each page), not for the
// whole Collect, which the caller bounds via context.
const httpTimeout = 30 * time.Second

// maxPages bounds every pagination loop so a server that always advertises
// another page cannot spin forever with unbounded memory. It is far above any
// realistic page count for a single query.
const maxPages = 10000

// defaultConcurrency bounds how many per-commit detail requests (GetCommit) run
// at once when enriching a result with stats/files/diffs. GitHub has no batch
// endpoint for commit detail, so a large author history otherwise costs one
// serial round-trip per commit; fanning these out is the dominant speed-up for
// "all my commits over a window" queries. The limit stays well under GitHub's
// concurrent-request ceiling so it does not itself provoke secondary rate limits.
const defaultConcurrency = 8

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

// skipRepoReason classifies a per-repo error hit while enumerating an org. It
// returns a short reason and true when the failure is specific to one
// repository, so an org scan can skip it and keep going rather than aborting on
// a single bad repo. The org's repo list was already fetched successfully (that
// is what required org access), so a failure now listing one repo's commits is
// a per-repo condition — an empty repo (409), a repo gone or not visible to the
// token (404/410), one whose access is denied (403), or one withheld for legal
// reasons (451). Global failures — rate limits, server errors, auth, context
// cancellation, network errors — return false so the caller still aborts
// instead of silently dropping every repo behind a transient or systemic fault.
func skipRepoReason(err error) (string, bool) {
	// Rate limits are global, not per-repo: skipping past them would just keep
	// tripping the same limit on every remaining repo, so let the caller abort.
	var rl *github.RateLimitError
	if errors.As(err, &rl) {
		return "", false
	}
	var ab *github.AbuseRateLimitError
	if errors.As(err, &ab) {
		return "", false
	}
	var er *github.ErrorResponse
	if errors.As(err, &er) && er.Response != nil {
		switch er.Response.StatusCode {
		case http.StatusConflict: // 409: "Git Repository is empty"
			return "empty repository", true
		case http.StatusNotFound: // 404: deleted in flight or not visible to the token
			return "not found", true
		case http.StatusGone: // 410: removed
			return "gone", true
		case http.StatusForbidden: // 403: access denied to this repo, OR a rate limit
			// GitHub returns 403 for both a genuine per-repo permission denial and
			// for primary/secondary rate limits, and go-github does not always map
			// the latter to RateLimitError/AbuseRateLimitError. Treating a
			// rate-limit 403 as a per-repo skip would silently drop repos and report
			// a benign "access forbidden" reason, yielding a drastically incomplete
			// evidence set, so classify it as fatal and let the caller abort.
			if isRateLimited(er) {
				return "", false
			}
			return "access forbidden", true
		case http.StatusUnavailableForLegalReasons: // 451: withheld (e.g. DMCA)
			return "unavailable for legal reasons", true
		}
	}
	return "", false
}

// isRateLimited reports whether a 403 ErrorResponse is actually a rate limit
// rather than a permission denial, by inspecting the response headers and
// message that GitHub sets on throttled requests: a Retry-After (secondary
// limit), an exhausted X-RateLimit-Remaining (primary limit), or a message that
// names the (secondary) rate limit.
func isRateLimited(er *github.ErrorResponse) bool {
	if er.Response != nil {
		if er.Response.Header.Get("Retry-After") != "" {
			return true
		}
		if er.Response.Header.Get("X-RateLimit-Remaining") == "0" {
			return true
		}
	}
	return strings.Contains(strings.ToLower(er.Message), "rate limit")
}

// Client retrieves commits from GitHub for an author over a time window.
type Client struct {
	gh          *github.Client
	perPage     int
	concurrency int
}

// New builds a Client. token may be empty (unauthenticated, heavily rate
// limited). baseURL, when set, targets a GitHub Enterprise instance and must be
// the API root (e.g. "https://ghe.example.com/api/v3/"). perPage is clamped to
// the API's 1-100 range.
func New(token, baseURL string, perPage int) (*Client, error) {
	// Use a client with an explicit timeout rather than http.DefaultClient (which
	// go-github's nil default uses) so a stalled request cannot hang a scan and so
	// the global default client is never mutated. WithAuthToken preserves the
	// Timeout while wrapping only the transport.
	gh := github.NewClient(&http.Client{Timeout: httpTimeout})
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
	return &Client{gh: gh, perPage: perPage, concurrency: defaultConcurrency}, nil
}

// Collect runs a query using its scope and returns the normalized result.
func (c *Client) Collect(ctx context.Context, q model.Query) (model.Result, error) {
	until := q.Until
	if until.IsZero() {
		until = time.Now()
	}
	q.Until = until

	// Fetch one commit past the cap so truncation can be distinguished from an
	// exact fit: if the helpers return more than MaxCommits, evidence was dropped
	// (Truncated); returning exactly MaxCommits means nothing was dropped and
	// Truncated must stay false. The final clip below trims the extra probe commit.
	eq := q
	if eq.MaxCommits > 0 {
		eq.MaxCommits = q.MaxCommits + 1
	}

	var (
		commits []model.Commit
		skipped []model.SkippedRepo
		err     error
	)
	switch q.Scope {
	case model.ScopeSearch:
		commits, err = c.searchByAuthor(ctx, eq)
	case model.ScopeRepos:
		commits, err = c.listRepos(ctx, eq)
	case model.ScopeOrg:
		commits, skipped, err = c.listOrg(ctx, eq)
	default:
		return model.Result{}, fmt.Errorf("unsupported scope %q", q.Scope)
	}
	if err != nil {
		return model.Result{}, err
	}

	truncated := false
	if q.MaxCommits > 0 && len(commits) > q.MaxCommits {
		commits = commits[:q.MaxCommits]
		truncated = true
	}

	// Enrich only the commits that survive the cap, and do it concurrently. The
	// scope helpers above return commit metadata without per-commit detail; here
	// each surviving commit gets its stats/files/diffs in a bounded fan-out. This
	// runs after the clip so the extra truncation-probe commit is never fetched.
	if needsDetail(q) {
		if err := c.enrichDetails(ctx, commits, q); err != nil {
			return model.Result{}, err
		}
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
		Skipped:       skipped,
	}, nil
}

// searchByAuthor uses GitHub's global commit search index.
func (c *Client) searchByAuthor(ctx context.Context, q model.Query) ([]model.Commit, error) {
	query := buildSearchQuery(q)
	opts := &github.SearchOptions{
		Sort:        "author-date",
		Order:       "desc",
		ListOptions: github.ListOptions{PerPage: c.resultPageSize(q.MaxCommits)},
	}
	var out []model.Commit
	for page := 1; ; page++ {
		if page > maxPages {
			return nil, fmt.Errorf("search commits: exceeded max pages (%d)", maxPages)
		}
		res, resp, err := c.gh.Search.Commits(ctx, query, opts)
		if err != nil {
			return nil, apiError("search commits", err)
		}
		for _, cr := range res.Commits {
			out = append(out, fromSearchResult(cr))
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
		repoQuery := remainingQuery(q, len(out))
		if repoQuery.MaxCommits == 0 && q.MaxCommits > 0 {
			return out, nil
		}
		commits, err := c.collectRepo(ctx, owner, repo, repoQuery)
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

// listOrg enumerates an org's repositories and lists commits in each. A failure
// listing one repo's commits does not abort the whole scan when it is a
// per-repo condition (e.g. an empty repo, or one the token cannot read): that
// repo is recorded in the returned skip list and enumeration continues. Global
// failures (rate limits, auth, server errors) still abort, since continuing
// would only retrip them on every remaining repo. See skipRepoReason.
func (c *Client) listOrg(ctx context.Context, q model.Query) ([]model.Commit, []model.SkippedRepo, error) {
	if q.Org == "" {
		return nil, nil, fmt.Errorf("scope %q requires an org", model.ScopeOrg)
	}
	opts := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: c.perPage},
	}
	var (
		out     []model.Commit
		skipped []model.SkippedRepo
	)
	for page := 1; ; page++ {
		if page > maxPages {
			return nil, nil, fmt.Errorf("list org repos %s: exceeded max pages (%d)", q.Org, maxPages)
		}
		repos, resp, err := c.gh.Repositories.ListByOrg(ctx, q.Org, opts)
		if err != nil {
			return nil, nil, apiError("list org repos "+q.Org, err)
		}
		for _, r := range repos {
			full := r.GetFullName()
			owner, repo, ok := splitRepo(full)
			if !ok {
				continue
			}
			repoQuery := remainingQuery(q, len(out))
			if repoQuery.MaxCommits == 0 && q.MaxCommits > 0 {
				return out, skipped, nil
			}
			commits, err := c.collectRepo(ctx, owner, repo, repoQuery)
			if err != nil {
				if reason, skip := skipRepoReason(err); skip {
					skipped = append(skipped, model.SkippedRepo{Repo: full, Reason: reason})
					continue
				}
				return nil, nil, err
			}
			out = append(out, commits...)
			if q.MaxCommits > 0 && len(out) >= q.MaxCommits {
				return out, skipped, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return out, skipped, nil
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
		ListOptions: github.ListOptions{PerPage: c.resultPageSize(q.MaxCommits)},
	}
	full := owner + "/" + repo
	var out []model.Commit
	for page := 1; ; page++ {
		if page > maxPages {
			return nil, fmt.Errorf("list commits %s: exceeded max pages (%d)", full, maxPages)
		}
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
	for page := 1; ; page++ {
		if page > maxPages {
			return nil, fmt.Errorf("list pull requests %s: exceeded max pages (%d)", full, maxPages)
		}
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
	opts := &github.ListOptions{PerPage: c.resultPageSize(q.MaxCommits)}
	var out []model.Commit
	for page := 1; ; page++ {
		if page > maxPages {
			return nil, fmt.Errorf("list pr commits %s#%d: exceeded max pages (%d)", full, number, maxPages)
		}
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

func needsDetail(q model.Query) bool {
	return q.IncludeStats || q.IncludeFiles || q.IncludeDiffs
}

func (c *Client) resultPageSize(maxCommits int) int {
	if maxCommits > 0 && maxCommits < c.perPage {
		return maxCommits
	}
	return c.perPage
}

func remainingQuery(q model.Query, have int) model.Query {
	if q.MaxCommits <= 0 {
		return q
	}
	remaining := q.MaxCommits - have
	if remaining < 0 {
		remaining = 0
	}
	q.MaxCommits = remaining
	return q
}

// enrichDetails fills each commit's stats/files/diffs concurrently, bounded by
// c.concurrency. Each worker writes only its own slice element, so the result
// order is unchanged. The first failure cancels the shared context and aborts
// the whole scan, matching the prior sequential behavior — a rate limit or auth
// error is fatal, not something to skip per commit.
func (c *Client) enrichDetails(ctx context.Context, commits []model.Commit, q model.Query) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(c.concurrency)
	for i := range commits {
		cm := &commits[i]
		g.Go(func() error {
			owner, repo, ok := splitRepo(cm.Repo)
			if !ok {
				return fmt.Errorf("invalid repo %q", cm.Repo)
			}
			return c.fillDetails(ctx, owner, repo, cm, q)
		})
	}
	return g.Wait()
}

func (c *Client) fillDetails(ctx context.Context, owner, repo string, cm *model.Commit, q model.Query) error {
	opts := &github.ListOptions{PerPage: c.perPage}
	var files []*github.CommitFile
	needFiles := q.IncludeFiles || q.IncludeDiffs
	for page := 1; ; page++ {
		if page > maxPages {
			return fmt.Errorf("get commit details %s/%s@%s: exceeded max pages (%d)", owner, repo, cm.SHA, maxPages)
		}
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
		return "author-email:" + searchQualifierValue(email)
	}
	return "author:" + searchQualifierValue(author)
}

// searchQualifierValue renders v as a GitHub commit-search qualifier value that
// cannot break out of its qualifier. Values made up only of safe identifier
// characters are emitted verbatim; anything containing a space, colon, quote, or
// other structural character is wrapped in double quotes with embedded
// backslashes and quotes escaped (GitHub's quoted-qualifier syntax). This is
// the defense-in-depth counterpart to config.Resolve's validation: it prevents
// an author of "victim author:attacker" from injecting a second qualifier, and
// matches the injection-safety the GitLab client gets for free from
// URL-encoding.
func searchQualifierValue(v string) string {
	if safeQualifierValue(v) {
		return v
	}
	var b strings.Builder
	b.Grow(len(v) + 2)
	b.WriteByte('"')
	for _, r := range v {
		if r == '\\' || r == '"' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

func safeQualifierValue(v string) bool {
	if v == "" {
		return false
	}
	for _, r := range v {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == '/', r == '@', r == '+':
		default:
			return false
		}
	}
	return true
}

func buildSearchQuery(q model.Query) string {
	parts := []string{authorQualifier(q.Author)}
	if !q.Since.IsZero() || !q.Until.IsZero() {
		// Emit full RFC3339 timestamps rather than whole-day dates so the search
		// scope filters at the same second precision as the repos, PR, and GitLab
		// paths; otherwise the same window returns materially different evidence
		// under scope=search vs scope=repos.
		since := "*"
		if !q.Since.IsZero() {
			since = q.Since.UTC().Format(time.RFC3339)
		}
		until := "*"
		if !q.Until.IsZero() {
			until = q.Until.UTC().Format(time.RFC3339)
		}
		parts = append(parts, fmt.Sprintf("author-date:%s..%s", since, until))
	}
	// Scope qualifiers. A bare global author search returns public repos only;
	// adding org:/repo: qualifiers is what lets the search index reach private
	// repos the authenticated token can access (e.g. an SSO-authorized org).
	if q.Org != "" {
		parts = append(parts, "org:"+searchQualifierValue(q.Org))
	}
	for _, repo := range q.Repos {
		if r := strings.TrimSpace(repo); r != "" {
			parts = append(parts, "repo:"+searchQualifierValue(r))
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
