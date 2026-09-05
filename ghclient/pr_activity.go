// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/go-github/v91/github"
	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/internal/apibudget"
	"github.com/skaphos/sting/model"
)

// CollectPRs gathers historical activity under a private, per-query request budget.
func (c *Client) CollectPRs(ctx context.Context, q model.PRQuery) (model.PRResult, error) {
	if err := validatePRQuery(q, c.perPage); err != nil {
		return model.PRResult{}, err
	}
	s, err := c.newPRSession(q.MaxRequests)
	if err != nil {
		return model.PRResult{}, err
	}
	result := model.PRResult{SchemaVersion: model.PRSchemaVersion, Workflow: "pr-activity", Provider: model.ProviderGitHub, GeneratedAt: time.Now().UTC(), Query: q, PRs: []model.PRActivityEntry{}}
	s.selectionNotes(q.Scope, q.Repos, q.Org)
	s.note("time-basis", "Activity selects "+string(q.TimeBasis)+" in the inclusive resolved window; current state is a separate filter.", "", 0)
	if q.TimeBasis == model.PRTimeUpdated {
		s.note("time-basis", "Updated matching uses only the latest recorded update, not every historical update.", "", 0)
	}
	entries := map[string]model.PRActivityEntry{}
	seen := map[string]bool{}
	process := func(page []model.PullRequest) error { return s.activityPage(ctx, q, page, entries, seen) }
	if q.Scope == model.ScopeSearch {
		var queries []string
		queries, err = activitySearchQueries(q)
		if err == nil {
			err = s.searchActivity(ctx, queries, q, entries, process)
		}
	} else {
		stop := func() bool { return q.MaxPRs > 0 && len(entries) >= q.MaxPRs }
		err = s.walkPRRepos(ctx, q.Scope, q.Repos, q.Org, func(repo string) error { _, e := s.listPRPages(ctx, repo, "all", process, stop); return e }, stop)
	}
	err = s.stop(err, "collect PR activity", "")
	for _, entry := range entries {
		result.PRs = append(result.PRs, entry)
	}
	slices.SortFunc(result.PRs, func(a, b model.PRActivityEntry) int { return comparePR(a.PR, b.PR) })
	s.finish()
	result.Count = len(result.PRs)
	result.Cost = s.cost()
	result.Coverage = s.coverage
	result.Truncated = !s.coverage.DiscoveryComplete || !s.coverage.EvidenceComplete
	result.Disclosures = s.disclosures
	return result, err
}
func validatePRQuery(q model.PRQuery, perPage int) error {
	if !q.Scope.Valid() || !slices.Contains([]model.PRState{model.PRStateAll, model.PRStateOpen, model.PRStateClosed, model.PRStateMerged}, q.State) || !slices.Contains([]model.PRTimeBasis{model.PRTimeAny, model.PRTimeCreated, model.PRTimeUpdated, model.PRTimeClosed, model.PRTimeMerged}, q.TimeBasis) {
		return fmt.Errorf("invalid resolved PR scope, state, or time basis; use config.ResolvePRs")
	}

	if q.Provider != model.ProviderGitHub || q.Since.IsZero() || q.Until.IsZero() {
		return fmt.Errorf("use config.ResolvePRs to supply a GitHub query with explicit window bounds")
	}
	draft, err := prDraftInput(q.Draft)
	if err != nil {
		return err
	}
	cfg := config.Default()
	cfg.PerPage = perPage
	_, err = cfg.ResolvePRs(config.PRRequest{Provider: string(q.Provider), Scope: string(q.Scope), Repos: q.Repos, Org: q.Org, Author: q.Author, Role: q.Role, State: string(q.State), TimeBasis: string(q.TimeBasis), Since: q.Since.Format(time.RFC3339Nano), Until: q.Until.Format(time.RFC3339Nano), Draft: draft, MaxPRs: &q.MaxPRs, MaxRequests: &q.MaxRequests}, q.Until)
	return err
}
func prDraftInput(draft model.PRDraftFilter) (*bool, error) {
	switch draft {
	case model.PRDraftAll:
		return nil, nil
	case model.PRDraftOnly:
		return new(true), nil
	case model.PRDraftExclude:
		return new(false), nil
	default:
		return nil, fmt.Errorf("invalid resolved draft filter")
	}
}
func comparePR(a, b model.PullRequest) int {
	if n := strings.Compare(strings.ToLower(a.Repo), strings.ToLower(b.Repo)); n != 0 {
		return n
	}
	return a.Number - b.Number
}
func (s *prSession) selectionNotes(scope model.Scope, repos []string, org string) {
	s.publicOnly = scope == model.ScopeSearch && len(repos) == 0 && org == ""
	method := "repository listing"
	if scope == model.ScopeSearch {
		method = "search index (at most 1,000 candidates per query and 4,000 matching repositories; inaccessible targets/index omissions cannot be enumerated)"
	}
	visibility := "credential-visible repositories"
	if scope == model.ScopeSearch && len(repos) == 0 && org == "" {
		visibility = "public repositories only"
	}
	s.note("visibility", method+"; "+visibility+".", "", 0)
}
func activitySearchQueries(q model.PRQuery) ([]string, error) {
	base := "is:pr author:" + q.Author + " "
	until := q.Until.UTC().AddDate(0, 0, 1).Format("2006-01-02")
	if q.TimeBasis == model.PRTimeAny || q.TimeBasis == model.PRTimeClosed {
		base += "created:<=" + until
	} else {
		base += string(q.TimeBasis) + ":" + q.Since.UTC().AddDate(0, 0, -1).Format("2006-01-02") + ".." + until
	}
	return prSearchChunks(base, q.Repos, q.Org)
}
func prSearchChunks(base string, repos []string, org string) ([]string, error) {
	if org != "" {
		base += " org:" + org
	}
	if len(repos) == 0 && org == "" {
		base += " is:public"
	}
	if len(base) > 256 {
		return nil, fmt.Errorf("search selectors exceed provider query length; shorten targets")
	}
	if len(repos) == 0 {
		return []string{base}, nil
	}
	out := []string{}
	chunk := base
	for _, repo := range repos {
		token := " repo:" + repo
		if len(base)+len(token) > 256 {
			return nil, fmt.Errorf("repository selector exceeds provider query length")
		}
		if len(chunk)+len(token) > 256 {
			out = append(out, chunk)
			chunk = base
		}
		chunk += token
	}
	return append(out, chunk), nil
}
func (s *prSession) searchPage(ctx context.Context, query string, page int) ([]model.PullRequest, int, error) {
	firstDisclosure := len(s.disclosures)
	defer func() {
		for n := firstDisclosure; n < len(s.disclosures); n++ {
			s.disclosures[n].Stream = query
		}
	}()

	var found *github.IssuesSearchResult
	var resp *github.Response
	perPage := s.client.perPage
	if (page-1)*perPage >= 1000 {
		s.discoveryGap("search-capped", "Search candidate ceiling reached; use repos/org scope.", "")
		return nil, 0, nil
	}
	err := s.do("discovery", func() error {
		var e error
		found, resp, e = s.client.gh.Search.Issues(ctx, query, &github.SearchOptions{Sort: "created", Order: "desc", ListOptions: github.ListOptions{Page: page, PerPage: perPage}})
		return e
	})
	if err != nil {
		kind := "provider-error"
		if errors.Is(err, apibudget.ErrBudgetExceeded) {
			kind = "request-budget"
		}
		s.discoveryGap(kind, "Search stream stopped: "+prErrorReason(err), "")
		return nil, 0, err
	}
	out := []model.PullRequest{}
	if found == nil {
		s.discoveryGap("search-incomplete", "Search returned no result envelope.", "")
		return out, 0, nil
	}
	if found.GetIncompleteResults() {
		s.discoveryGap("search-incomplete", "Provider search reported incomplete results; narrow targets or use listing.", "")
	}
	next := 0
	if resp != nil {
		next = resp.NextPage
	}
	if found.GetTotal() > 1000 || (next > 0 && (next-1)*s.client.perPage >= 1000) {
		s.discoveryGap("search-capped", "Search exposes at most 1,000 candidates; use repos/org scope.", "")
	}
	if next > 0 && (next-1)*s.client.perPage >= 1000 {
		next = 0
	}
	for index, raw := range found.Issues {
		if (page-1)*perPage+index >= 1000 {
			break
		}
		p, ok := fromPRIssue(raw)
		if !ok {
			s.gap("metadata-unavailable", "Search candidate lacks a valid PR identity.", "", 0)
			continue
		}
		if raw.GetState() == "closed" {
			s.closedIssues[prIdentity(p)] = true
		}
		if s.publicOnly && raw.Repository != nil && raw.Repository.GetPrivate() {
			s.privateRepos[strings.ToLower(p.Repo)] = true
			s.discoveryGap("provider-changed", "Private search candidate excluded by public-only selection.", p.Repo)
		}
		if s.publicOnly && s.privateRepos[strings.ToLower(p.Repo)] {
			continue
		}
		out = append(out, p)
	}
	sortPRPage(out)
	return out, next, nil
}
func sortPRPage(page []model.PullRequest) {
	slices.SortStableFunc(page, func(a, b model.PullRequest) int {
		if a.CreatedAt != nil && b.CreatedAt != nil {
			if n := b.CreatedAt.Compare(*a.CreatedAt); n != 0 {
				return n
			}
		}
		return comparePR(a, b)
	})
}
func (s *prSession) searchActivity(ctx context.Context, queries []string, q model.PRQuery, entries map[string]model.PRActivityEntry, process func([]model.PullRequest) error) error {
	for i, query := range queries {
		for page, n := 1, 0; ; n++ {
			if n >= maxPages {
				s.discoveryGap("pagination-limit", "Search pagination safety limit reached.", "")
				return nil
			}
			items, next, err := s.searchPage(ctx, query, page)
			if err != nil {
				return err
			}
			if err = process(items); err != nil {
				return err
			}
			if q.MaxPRs > 0 && len(entries) >= q.MaxPRs {
				if next != 0 || i < len(queries)-1 {
					s.discoveryGap("result-limit", "PR result ceiling reached with unexamined discovery.", "")
				}
				return nil
			}
			if next == 0 {
				break
			}
			page = next
		}
	}
	return nil
}
func (s *prSession) activityPage(ctx context.Context, q model.PRQuery, page []model.PullRequest, entries map[string]model.PRActivityEntry, seen map[string]bool) error {
	candidates := []model.PullRequest{}
	for _, p := range page {
		if !prTargetMatch(p.Repo, q.Repos, q.Org) {
			continue
		}
		id := prIdentity(p)
		if seen[id] {
			continue
		}
		seen[id] = true
		match, known := prFilter(p, q)
		if known && !match {
			continue
		}
		if len(p.MissingFields) > 0 {
			s.note("metadata-unavailable", "Unavailable metadata: "+strings.Join(p.MissingFields, ", "), p.Repo, p.Number)
		}
		candidates = append(candidates, p)
		matches := selectedPRMatches(recordPRMatches(p), q)
		if match && len(matches) > 0 {
			s.admitActivity(q, entries, model.PRActivityEntry{PR: p, Matches: matches, MatchesComplete: !prHistoryNeeded(p, q)})
		}
	}
	for _, p := range candidates {
		id := prIdentity(p)
		_, retained := entries[id]
		if q.MaxPRs > 0 && len(entries) >= q.MaxPRs && !retained {
			s.discoveryGap("result-limit", "PR ceiling leaves candidates unevaluated.", p.Repo)
			continue
		}
		matches := selectedPRMatches(recordPRMatches(p), q)
		complete := true
		var err error
		if prHistoryNeeded(p, q) {
			p, matches, complete, err = s.history(ctx, p, q)
		}
		match, known := prFilter(p, q)
		if !known {
			prMissingMatch(s, p)
			complete = false
		}
		if p.CreatedAt == nil && (q.TimeBasis == model.PRTimeAny || q.TimeBasis == model.PRTimeCreated) {
			prMissingMatch(s, p)
			complete = false
		}
		if p.UpdatedAt == nil && q.TimeBasis == model.PRTimeUpdated {
			prMissingMatch(s, p)
			complete = false
		}
		if match && len(matches) > 0 {
			s.admitActivity(q, entries, model.PRActivityEntry{PR: p, Matches: matches, MatchesComplete: complete})
		} else {
			delete(entries, id)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *prSession) admitActivity(q model.PRQuery, entries map[string]model.PRActivityEntry, entry model.PRActivityEntry) {
	id := prIdentity(entry.PR)
	_, exists := entries[id]
	if !exists && q.MaxPRs > 0 && len(entries) >= q.MaxPRs {
		s.discoveryGap("result-limit", "PR result ceiling reached.", entry.PR.Repo)
		return
	}
	entries[id] = entry
}
