// SPDX-License-Identifier: MIT
package config

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/skaphos/sting/model"
)

// DefaultMaxPRs is a fixed result ceiling independent of remaining provider quota.
const DefaultMaxPRs = 100

// PRRequest is raw activity input. Pointer fields preserve explicit false and zero.
type PRRequest struct {
	Provider, Scope, Author, Org, Since, Until, Window, State, TimeBasis, Role string
	Repos                                                                      []string
	Draft                                                                      *bool
	MaxPRs, MaxRequests                                                        *int
}

// PRInboxRequest deliberately excludes activity selectors.
type PRInboxRequest struct {
	Provider, Scope, User, Org string
	Repos                      []string
	Draft                      *bool
	MaxPRs, MaxRequests        *int
}

// ResolvePRs normalizes activity input once, before any provider access.
func (cfg Config) ResolvePRs(req PRRequest, now time.Time) (model.PRQuery, error) {
	base, err := cfg.ResolvePRInbox(PRInboxRequest{Provider: req.Provider, Scope: req.Scope, User: req.Author, Org: req.Org, Repos: req.Repos, Draft: req.Draft, MaxPRs: req.MaxPRs, MaxRequests: req.MaxRequests})
	if err != nil {
		return model.PRQuery{}, err
	}
	if base.Scope == model.ScopeSearch && req.Author == "" {
		return model.PRQuery{}, fmt.Errorf("PR search requires an author")
	}
	if req.Role != "" && req.Role != "author" {
		return model.PRQuery{}, fmt.Errorf("PR activity supports only role=author")
	}
	if req.Role != "" && req.Author == "" {
		return model.PRQuery{}, fmt.Errorf("role=author requires an author filter")
	}
	state := model.PRState(req.State)
	if state == "" {
		state = model.PRStateAll
	}
	if !slices.Contains([]model.PRState{model.PRStateAll, model.PRStateOpen, model.PRStateMerged, model.PRStateClosed}, state) {
		return model.PRQuery{}, fmt.Errorf("invalid PR state (want open|merged|closed|all)")
	}
	basis := model.PRTimeBasis(req.TimeBasis)
	if basis == "" {
		basis = model.PRTimeAny
	}
	if !slices.Contains([]model.PRTimeBasis{model.PRTimeCreated, model.PRTimeUpdated, model.PRTimeMerged, model.PRTimeClosed, model.PRTimeAny}, basis) {
		return model.PRQuery{}, fmt.Errorf("invalid PR time basis (want created|updated|merged|closed|any)")
	}
	if basis == model.PRTimeMerged && (state == model.PRStateOpen || state == model.PRStateClosed) {
		return model.PRQuery{}, fmt.Errorf("merged activity contradicts current open/closed-unmerged state")
	}
	since, until, err := resolveWindow(req.Since, req.Until, req.Window, cfg.DefaultWindow, now)
	if err != nil {
		return model.PRQuery{}, err
	}
	role := ""
	if req.Author != "" {
		role = "author"
	}
	return model.PRQuery{Provider: base.Provider, Scope: base.Scope, Repos: base.Repos, Org: base.Org, Author: base.User, Role: role, State: state, Draft: base.Draft, TimeBasis: basis, Since: since, Until: until, MaxPRs: base.MaxPRs, MaxRequests: base.MaxRequests}, nil
}

// ResolvePRInbox resolves applicable typed defaults without reading credentials or windows.
func (cfg Config) ResolvePRInbox(req PRInboxRequest) (model.PRInboxQuery, error) {
	q := model.PRInboxQuery{Provider: model.ProviderGitHub, User: req.User, State: model.PRStateOpen, Draft: model.PRDraftAll, IdentitySource: "authenticated", Relationships: []model.PRRelationship{model.PRAuthored, model.PRAssigned, model.PRReviewRequested}, MaxPRs: cfg.MaxPRs, MaxRequests: cfg.MaxRequests}
	if req.Provider != "" && req.Provider != "github" {
		return q, fmt.Errorf("PR queries support github only; requested provider is unsupported")
	}
	if req.User != "" && !ghLoginRe.MatchString(req.User) {
		return q, fmt.Errorf("PR user/author must be a GitHub login")
	}
	if req.User != "" {
		q.IdentitySource = "explicit"
	}
	q.Scope = model.Scope(req.Scope)
	if q.Scope == "" {
		q.Scope = cfg.DefaultScope
	}
	if q.Scope == "" {
		q.Scope = model.ScopeSearch
	}
	if !q.Scope.Valid() {
		return q, fmt.Errorf("invalid scope (want search|repos|org)")
	}
	repos, org, err := cfg.prTargets(q.Scope, req.Repos, req.Org)
	if err != nil {
		return q, err
	}
	q.Repos = repos
	q.Org = org
	if req.Draft != nil {
		q.Draft = model.PRDraftExclude
		if *req.Draft {
			q.Draft = model.PRDraftOnly
		}
	}
	if req.MaxPRs != nil {
		q.MaxPRs = *req.MaxPRs
	}
	if req.MaxRequests != nil {
		q.MaxRequests = *req.MaxRequests
	}
	if q.MaxPRs < 0 || q.MaxRequests < 0 {
		return q, fmt.Errorf("max_prs and max_requests must be >= 0")
	}
	if cfg.PerPage < 1 || cfg.PerPage > 100 {
		return q, fmt.Errorf("per_page must be 1-100")
	}
	return q, nil
}

func (cfg Config) prTargets(scope model.Scope, requested []string, requestedOrg string) ([]string, string, error) {
	repos, org := requested, requestedOrg
	if scope == model.ScopeRepos && requestedOrg != "" {
		return nil, "", fmt.Errorf("repos scope does not accept org; use search for intersection")
	}
	if scope == model.ScopeOrg && len(requested) > 0 {
		return nil, "", fmt.Errorf("org scope does not accept repos; use search for intersection")
	}
	if scope != model.ScopeOrg && len(repos) == 0 {
		repos = cfg.DefaultRepos
	}
	if scope != model.ScopeRepos && org == "" {
		org = cfg.DefaultOrg
	}
	if scope == model.ScopeOrg {
		repos = nil
	}
	if scope == model.ScopeRepos {
		org = ""
	}
	if org != "" && !ghLoginRe.MatchString(org) {
		return nil, "", fmt.Errorf("invalid GitHub organization")
	}
	out := make([]string, 0, len(repos))
	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		parts := strings.Split(repo, "/")
		if len(parts) != 2 || !ghLoginRe.MatchString(parts[0]) || !ghRepoNameRe.MatchString(parts[1]) || parts[1] == "." || parts[1] == ".." {
			return nil, "", fmt.Errorf("invalid GitHub repository (want owner/name)")
		}
		out = append(out, strings.ToLower(repo))
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if scope == model.ScopeRepos && len(out) == 0 {
		return nil, "", fmt.Errorf("repos scope requires at least one repository")
	}
	if scope == model.ScopeOrg && org == "" {
		return nil, "", fmt.Errorf("org scope requires an organization")
	}
	return out, strings.ToLower(org), nil
}
