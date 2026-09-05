// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/go-github/v91/github"
	"github.com/skaphos/sting/internal/apibudget"
	"github.com/skaphos/sting/model"
)

type prSession struct {
	publicOnly   bool
	privateRepos map[string]bool
	closedIssues map[string]bool
	client       *Client
	missingMeta  map[string]int
	beforeWindow int
	counts       model.PRCostReport
	coverage     model.PRCoverage
	disclosures  []model.PRDisclosure
}

func (c *Client) newPRSession(limit int) (*prSession, error) {
	if limit < 0 {
		return nil, fmt.Errorf("max_requests must be >= 0")
	}
	client, err := New(c.token, c.baseURL, c.perPage, WithRequestBudget(limit))
	if err != nil {
		return nil, err
	}
	return &prSession{client: client, privateRepos: map[string]bool{}, closedIssues: map[string]bool{}, missingMeta: map[string]int{}, counts: model.PRCostReport{Ceiling: limit}, coverage: model.PRCoverage{DiscoveryComplete: true, EvidenceComplete: true}, disclosures: []model.PRDisclosure{}}, nil
}
func (s *prSession) do(category string, call func() error) error {
	before := s.client.budget.Consumed()
	err := call()
	n := s.client.budget.Consumed() - before
	switch category {
	case "identity":
		s.counts.IdentityRequests += n
	case "history":
		s.counts.HistoryRequests += n
	default:
		s.counts.DiscoveryRequests += n
	}
	return err
}
func (s *prSession) cost() model.PRCostReport {
	out := s.counts
	out.Consumed = s.client.budget.Consumed()
	if s.client.budget.QuotaSeen() {
		r := s.client.Cost()
		out.Quota = &model.PRQuota{Limit: r.QuotaLimit, Remaining: r.QuotaRemaining, Reset: r.QuotaResetsAt}
	}
	return out
}
func (s *prSession) note(kind, reason, repo string, number int) {
	s.disclosures = append(s.disclosures, model.PRDisclosure{Kind: kind, Reason: reason, NextAction: prNextAction(kind), Repo: repo, Number: number})
}

// prNextAction keeps recovery guidance in the structured field rather than in
// prose, so an agent consuming JSON reads the same next step the report prints.
// Selection facts and absent optional metadata have no safe next action.
func prNextAction(kind string) string {
	switch kind {
	case "request-budget":
		return "Raise max_requests or narrow the targets, then re-run."
	case "result-limit":
		return "Raise max_prs or narrow the targets to collect the remaining PRs."
	case "search-capped":
		return "Use repos or org scope, or split the window into shorter ranges."
	case "search-incomplete":
		return "Retry the query, or use repos/org listing for complete discovery."
	case "pagination-limit":
		return "Narrow the targets; the provider offered more pages than sting follows."
	case "repo-skipped":
		return "Verify access to the repository, or exclude it from the query."
	case "history-incomplete":
		return "Raise max_requests and re-run; the actions shown are confirmed but the history is not complete."
	case "history-ambiguous":
		return "Inspect the PR in the provider; sting will not infer an unproven action."
	case "reasons-incomplete":
		return "Re-run with explicit repos or org targets so relationships can be verified."
	case "identity-unresolved":
		return "Run sting auth github, or supply an explicit user."
	case "provider-error":
		return "Check target access and sting's dedicated credentials, then retry."
	case "provider-changed":
		return "Re-run the query; the provider state changed during collection."
	}
	// history-bounded and the selection kinds have no corrective action.
	return ""
}

// missingMetadata defers optional-field gaps so one disclosure covers every
// record sharing a field set. Per-record attribution stays in missing_fields.
func (s *prSession) missingMetadata(p model.PullRequest) {
	if len(p.MissingFields) > 0 {
		s.missingMeta[strings.Join(p.MissingFields, ", ")]++
	}
}
func (s *prSession) gap(kind, reason, repo string, number int) {
	s.coverage.EvidenceComplete = false
	s.note(kind, reason, repo, number)
}
func (s *prSession) discoveryGap(kind, reason, repo string) {
	s.coverage.DiscoveryComplete = false
	s.gap(kind, reason, repo, 0)
}
func (s *prSession) finish() {
	for fields, n := range s.missingMeta {
		s.note("metadata-unavailable", fmt.Sprintf("Discovery did not supply %s for %d PR records; each record lists its own missing_fields.", fields, n), "", 0)
	}
	slices.SortFunc(s.disclosures, func(a, b model.PRDisclosure) int {
		return strings.Compare(fmt.Sprintf("%s/%s/%09d/%s/%s", a.Kind, a.Repo, a.Number, a.Stream, a.Reason), fmt.Sprintf("%s/%s/%09d/%s/%s", b.Kind, b.Repo, b.Number, b.Stream, b.Reason))
	})
	s.disclosures = slices.Compact(s.disclosures)
}

// prFailure keeps errors.Is/As usable without exposing URLs or provider bodies in Error().
type prFailure struct {
	op    string
	cause error
}

func (e *prFailure) Error() string { return e.op + ": " + prErrorReason(e.cause) }
func (e *prFailure) Unwrap() error { return e.cause }
func prErrorReason(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "query canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "query timed out"
	case errors.Is(err, apibudget.ErrBudgetExceeded):
		return "request budget reached"
	}
	var rl *github.RateLimitError
	var ab *github.AbuseRateLimitError
	if errors.As(err, &rl) || errors.As(err, &ab) {
		return "GitHub rate limit; retry after the provider reset"
	}
	var er *github.ErrorResponse
	if errors.As(err, &er) && er.Response != nil {
		if isRateLimited(er) {
			return "GitHub rate limit; retry later"
		}
		return fmt.Sprintf("GitHub HTTP %d; check target access and dedicated credentials", er.Response.StatusCode)
	}
	return "provider request failed; retry or check the configured API connection"
}
func (s *prSession) stop(err error, op, repo string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, apibudget.ErrBudgetExceeded) {
		s.discoveryGap("request-budget", "Request ceiling reached.", repo)
		return nil
	}
	s.discoveryGap("provider-error", op+": "+prErrorReason(err), repo)
	return &prFailure{op: op, cause: err}
}

var prRepoPattern = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)

func validPRRepo(repo string) bool {
	return prRepoPattern.MatchString(repo) && !strings.HasSuffix(repo, "/.") && !strings.HasSuffix(repo, "/..")
}
func prIdentity(p model.PullRequest) string {
	return fmt.Sprintf("%s#%d", strings.ToLower(p.Repo), p.Number)
}
func prTime(t *github.Timestamp) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	v := t.UTC()
	return &v
}
func prLogins(users []*github.User) []string {
	if users == nil {
		return nil
	}
	out := make([]string, 0, len(users))
	for _, u := range users {
		if u.GetLogin() != "" {
			out = append(out, u.GetLogin())
		}
	}
	slices.SortFunc(out, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return slices.CompactFunc(out, strings.EqualFold)
}
func fromPR(raw *github.PullRequest, repo string) (model.PullRequest, bool) {
	if raw == nil {
		return model.PullRequest{}, false
	}
	p := model.PullRequest{Repo: repo, Number: raw.GetNumber(), URL: raw.GetHTMLURL(), Title: raw.GetTitle(), Author: raw.GetUser().GetLogin(), Draft: raw.Draft, CreatedAt: prTime(raw.CreatedAt), UpdatedAt: prTime(raw.UpdatedAt), MergedAt: prTime(raw.MergedAt), ClosedAt: prTime(raw.ClosedAt), Assignees: prLogins(raw.Assignees), RequestedReviewers: prLogins(raw.RequestedReviewers), Additions: raw.Additions, Deletions: raw.Deletions, ChangedFiles: raw.ChangedFiles, MissingFields: []string{}}
	if raw.GetState() == "open" {
		p.State = new(model.PRStateOpen)
	} else if raw.GetState() == "closed" {
		p.State = new(model.PRStateClosed)
	}
	if p.MergedAt != nil {
		p.State = new(model.PRStateMerged)
	}
	if raw.Base != nil {
		p.BaseRef = raw.Base.Ref
	}
	if raw.Head != nil {
		p.HeadRef = raw.Head.Ref
	}
	if raw.Milestone != nil {
		p.Milestone = raw.Milestone.Title
	}
	if raw.Labels != nil {
		p.Labels = []string{}
		for _, l := range raw.Labels {
			p.Labels = append(p.Labels, l.GetName())
		}
		slices.Sort(p.Labels)
	}
	if raw.RequestedTeams != nil {
		p.RequestedTeams = []string{}
		for _, team := range raw.RequestedTeams {
			slug := team.GetSlug()
			if team.Organization != nil {
				slug = team.Organization.GetLogin() + "/" + slug
			}
			p.RequestedTeams = append(p.RequestedTeams, slug)
		}
		slices.Sort(p.RequestedTeams)
	}
	return prMissing(p)
}
func fromPRIssue(raw *github.Issue) (model.PullRequest, bool) {
	if raw == nil {
		return model.PullRequest{}, false
	}
	repo := ""
	if u, err := url.Parse(raw.GetRepositoryURL()); err == nil {
		_, repo, _ = strings.Cut(u.Path, "/repos/")
	}
	if raw.Repository != nil && raw.Repository.GetFullName() != "" {
		repo = raw.Repository.GetFullName()
	}
	pr := &github.PullRequest{Number: raw.Number, HTMLURL: raw.HTMLURL, Title: raw.Title, User: raw.User, State: raw.State, Draft: raw.Draft, CreatedAt: raw.CreatedAt, UpdatedAt: raw.UpdatedAt, ClosedAt: raw.ClosedAt, Assignees: raw.Assignees, Labels: raw.Labels, Milestone: raw.Milestone}
	if raw.PullRequestLinks != nil {
		pr.MergedAt = raw.PullRequestLinks.MergedAt
	}
	p, ok := fromPR(pr, repo)
	if raw.GetState() == "closed" && p.MergedAt == nil {
		p.State = nil
		p.MissingFields = append(p.MissingFields, "state")
	}
	for _, field := range []string{"merged_at", "closed_at", "milestone"} {
		if (field == "merged_at" && p.MergedAt == nil) || (field == "closed_at" && p.ClosedAt == nil) || (field == "milestone" && p.Milestone == nil) {
			p.MissingFields = append(p.MissingFields, field)
		}
	}
	slices.Sort(p.MissingFields)
	p.MissingFields = slices.Compact(p.MissingFields)
	return p, ok
}
func prMissing(p model.PullRequest) (model.PullRequest, bool) {
	u, err := url.Parse(p.URL)
	if !validPRRepo(p.Repo) || p.Number <= 0 || err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return p, false
	}
	fields := map[string]bool{"title": p.Title == "", "author": p.Author == "", "state": p.State == nil, "draft": p.Draft == nil, "created_at": p.CreatedAt == nil, "updated_at": p.UpdatedAt == nil, "assignees": p.Assignees == nil, "requested_reviewers": p.RequestedReviewers == nil, "requested_teams": p.RequestedTeams == nil, "base_ref": p.BaseRef == nil, "head_ref": p.HeadRef == nil, "labels": p.Labels == nil, "additions": p.Additions == nil, "deletions": p.Deletions == nil, "changed_files": p.ChangedFiles == nil}
	for name, missing := range fields {
		if missing {
			p.MissingFields = append(p.MissingFields, name)
		}
	}
	slices.Sort(p.MissingFields)
	return p, true
}

// listPRPages preserves already processed pages on errors; complete means the
// provider's pagination was exhausted, not merely that the result cap was met.
func (s *prSession) listPRPages(ctx context.Context, repo, state string, process func([]model.PullRequest) error, stop func() bool) (bool, error) {
	owner, name, _ := splitRepo(repo)
	opts := &github.PullRequestListOptions{State: state, Sort: "created", Direction: "desc", ListOptions: github.ListOptions{Page: 1, PerPage: s.client.perPage}}
	for pages := 0; ; pages++ {
		if pages >= maxPages {
			s.discoveryGap("pagination-limit", "Repository PR pagination safety limit reached.", repo)
			return false, nil
		}
		var raw []*github.PullRequest
		var resp *github.Response
		err := s.do("discovery", func() error {
			var e error
			raw, resp, e = s.client.gh.PullRequests.List(ctx, owner, name, opts)
			return e
		})
		if err != nil {
			return false, &prFailure{op: "list PRs " + repo, cause: err}
		}
		page := []model.PullRequest{}
		if s.publicOnly && slices.ContainsFunc(raw, func(r *github.PullRequest) bool {
			return r != nil && r.Base != nil && r.Base.Repo != nil && r.Base.Repo.GetPrivate()
		}) {
			s.privateRepos[strings.ToLower(repo)] = true
			s.discoveryGap("provider-changed", "Repository is private during verification and is excluded by public-only selection.", repo)
		}
		for _, r := range raw {
			if s.publicOnly && s.privateRepos[strings.ToLower(repo)] {
				continue
			}

			p, ok := fromPR(r, repo)
			if !ok {
				s.gap("metadata-unavailable", "Listed PR has an invalid identity.", repo, 0)
				continue
			}
			page = append(page, p)
		}
		sortPRPage(page)
		if err = process(page); err != nil {
			return false, err
		}
		if resp == nil || resp.NextPage == 0 {
			return true, nil
		}
		if stop() {
			s.discoveryGap("result-limit", "PR ceiling leaves repository pages unexamined.", repo)
			return false, nil
		}
		opts.Page = resp.NextPage
	}
}
func (s *prSession) walkPRRepos(ctx context.Context, scope model.Scope, repos []string, org string, visit func(string) error, stop func() bool) error {
	if scope == model.ScopeOrg {
		return s.walkPROrg(ctx, org, visit, stop)
	}
	if scope != model.ScopeRepos {
		return fmt.Errorf("invalid PR listing scope")
	}
	for i, repo := range repos {
		if err := visit(repo); err != nil {
			kind := "provider-error"
			if errors.Is(err, apibudget.ErrBudgetExceeded) {
				kind = "request-budget"
			}
			s.discoveryGap(kind, "Repository collection stopped: "+prErrorReason(err), repo)
			return err
		}
		if stop() && i < len(repos)-1 {
			s.discoveryGap("result-limit", "PR ceiling leaves repositories unexamined.", repo)
			return nil
		}
	}
	return nil
}
func prTargetMatch(repo string, repos []string, org string) bool {
	owner, _, _ := strings.Cut(repo, "/")
	if org != "" && !strings.EqualFold(org, owner) {
		return false
	}
	return len(repos) == 0 || slices.ContainsFunc(repos, func(target string) bool { return strings.EqualFold(repo, target) })
}

func (s *prSession) walkPROrg(ctx context.Context, org string, visit func(string) error, stop func() bool) error {
	opts := &github.RepositoryListByOrgOptions{Sort: "full_name", Direction: "asc", ListOptions: github.ListOptions{Page: 1, PerPage: s.client.perPage}}
	seen := map[string]bool{}
	for pages := 0; ; pages++ {
		if pages >= maxPages {
			s.discoveryGap("pagination-limit", "Organization enumeration safety limit reached.", org)
			return nil
		}
		var repos []*github.Repository
		var resp *github.Response
		err := s.do("discovery", func() error {
			var e error
			repos, resp, e = s.client.gh.Repositories.ListByOrg(ctx, org, opts)
			return e
		})
		if err != nil {
			kind := "provider-error"
			if errors.Is(err, apibudget.ErrBudgetExceeded) {
				kind = "request-budget"
			}
			s.discoveryGap(kind, "Organization enumeration failed: "+prErrorReason(err), org)
			return err
		}
		names := []string{}
		for _, repo := range repos {
			name := strings.ToLower(repo.GetFullName())
			if !validPRRepo(name) || !prTargetMatch(name, nil, org) {
				s.discoveryGap("metadata-unavailable", "Organization returned an invalid repository identity.", org)
				continue
			}
			if !seen[name] {
				names = append(names, name)
				seen[name] = true
			}
		}
		slices.Sort(names)
		for i, name := range names {
			if err = visit(name); err != nil {
				if reason, skip := skipRepoReason(err); skip {
					s.discoveryGap("repo-skipped", reason, name)
				} else {
					kind := "provider-error"
					if errors.Is(err, apibudget.ErrBudgetExceeded) {
						kind = "request-budget"
					}
					s.discoveryGap(kind, "Repository collection stopped: "+prErrorReason(err), name)
					return err
				}
			}
			if stop() {
				if i < len(names)-1 || (resp != nil && resp.NextPage != 0) {
					s.discoveryGap("result-limit", "PR ceiling leaves organization repositories unexamined.", org)
				}
				return nil
			}
		}
		if resp == nil || resp.NextPage == 0 {
			return nil
		}
		opts.Page = resp.NextPage
	}
}
