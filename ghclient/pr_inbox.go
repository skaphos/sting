// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/go-github/v91/github"
	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

type prVerification struct {
	facts    map[string]model.PullRequest
	complete bool
}
type prInboxCollector struct {
	s        *prSession
	q        model.PRInboxQuery
	entries  map[string]model.PRInboxEntry
	verified map[string]*prVerification
}

// CollectPRInbox gathers current personal work without an activity window.
func (c *Client) CollectPRInbox(ctx context.Context, q model.PRInboxQuery) (model.PRInboxResult, error) {
	if err := validatePRInbox(q, c.perPage); err != nil {
		return model.PRInboxResult{}, err
	}
	s, err := c.newPRSession(q.MaxRequests)
	if err != nil {
		return model.PRInboxResult{}, err
	}
	result := model.PRInboxResult{SchemaVersion: model.PRInboxSchemaVersion, Workflow: "pr-inbox", Provider: model.ProviderGitHub, GeneratedAt: time.Now().UTC(), Query: q, PRs: []model.PRInboxEntry{}}
	s.selectionNotes(q.Scope, q.Repos, q.Org)
	if q.User == "" {
		var user *github.User
		err = s.do("identity", func() error { var e error; user, _, e = s.client.gh.Users.Get(ctx, ""); return e })
		if err == nil {
			if user == nil || user.GetLogin() == "" {
				err = fmt.Errorf("authenticated identity response lacks a login")
			} else {
				q.User = user.GetLogin()
				if e := validatePRInbox(q, c.perPage); e != nil {
					q.User = ""
					err = e
				}
			}
		}
		if err != nil {
			s.gap("identity-unresolved", "Unable to resolve the user; configure a dedicated GitHub credential or supply an explicit user.", "", 0)
		}
	}
	result.Query = q
	collector := &prInboxCollector{s: s, q: q, entries: map[string]model.PRInboxEntry{}, verified: map[string]*prVerification{}}
	if err == nil {
		if q.Scope == model.ScopeSearch {
			err = collector.search(ctx)
		} else {
			err = s.walkPRRepos(ctx, q.Scope, q.Repos, q.Org, func(repo string) error {
				_, e := s.listPRPages(ctx, repo, "open", func(page []model.PullRequest) error {
					for _, p := range page {
						collector.admit(p, false)
					}
					return nil
				}, collector.full)
				return e
			}, collector.full)
		}
	}
	err = s.stop(err, "collect PR inbox", "")
	for _, entry := range collector.entries {
		if !entry.ReasonsComplete {
			s.gap("reasons-incomplete", "Personal relationships could not all be verified.", entry.PR.Repo, entry.PR.Number)
		}
		result.PRs = append(result.PRs, entry)
	}
	slices.SortFunc(result.PRs, func(a, b model.PRInboxEntry) int { return comparePR(a.PR, b.PR) })
	s.finish()
	result.Count = len(result.PRs)
	result.Coverage = s.coverage
	result.Cost = s.cost()
	result.Truncated = !s.coverage.DiscoveryComplete || !s.coverage.EvidenceComplete
	result.Disclosures = s.disclosures
	return result, err
}
func validatePRInbox(q model.PRInboxQuery, perPage int) error {
	if !q.Scope.Valid() || q.Provider != model.ProviderGitHub || q.State != model.PRStateOpen || !slices.Equal(q.Relationships, []model.PRRelationship{model.PRAuthored, model.PRAssigned, model.PRReviewRequested}) {
		return fmt.Errorf("use config.ResolvePRInbox to supply a GitHub open-PR union query")
	}
	if q.IdentitySource != "explicit" && q.IdentitySource != "authenticated" {
		return fmt.Errorf("invalid inbox identity source")
	}
	if q.IdentitySource == "explicit" && q.User == "" {
		return fmt.Errorf("explicit inbox identity requires a user")
	}
	draft, err := prDraftInput(q.Draft)
	if err != nil {
		return err
	}
	cfg := config.Default()
	cfg.PerPage = perPage
	_, err = cfg.ResolvePRInbox(config.PRInboxRequest{Provider: string(q.Provider), Scope: string(q.Scope), User: q.User, Repos: q.Repos, Org: q.Org, Draft: draft, MaxPRs: &q.MaxPRs, MaxRequests: &q.MaxRequests})
	return err
}
func (i *prInboxCollector) full() bool { return i.q.MaxPRs > 0 && len(i.entries) >= i.q.MaxPRs }
func inboxReasons(p model.PullRequest, user string) ([]model.PRRelationship, bool) {
	reasons := []model.PRRelationship{}
	if strings.EqualFold(p.Author, user) {
		reasons = append(reasons, model.PRAuthored)
	}
	if slices.ContainsFunc(p.Assignees, func(v string) bool { return strings.EqualFold(v, user) }) {
		reasons = append(reasons, model.PRAssigned)
	}
	if slices.ContainsFunc(p.RequestedReviewers, func(v string) bool { return strings.EqualFold(v, user) }) {
		reasons = append(reasons, model.PRReviewRequested)
	}
	return reasons, p.Author != "" && p.Assignees != nil && p.RequestedReviewers != nil
}
func (i *prInboxCollector) admit(p model.PullRequest, search bool) {
	if !prTargetMatch(p.Repo, i.q.Repos, i.q.Org) {
		return
	}
	id := prIdentity(p)
	if p.State != nil && *p.State != model.PRStateOpen {
		delete(i.entries, id)
		return
	}
	if i.q.Draft != model.PRDraftAll && p.Draft != nil && (*p.Draft != (i.q.Draft == model.PRDraftOnly)) {
		delete(i.entries, id)
		return
	}
	reasons, complete := inboxReasons(p, i.q.User)
	known := p.State != nil && (i.q.Draft == model.PRDraftAll || p.Draft != nil)
	if !known {
		i.s.gap("reasons-incomplete", "Current state or draft selection is unverified.", p.Repo, p.Number)
		return
	}
	if len(reasons) == 0 {
		if !complete {
			i.s.gap("reasons-incomplete", "Candidate relationships are unavailable.", p.Repo, p.Number)
		}
		delete(i.entries, id)
		return
	}
	if _, exists := i.entries[id]; !exists && i.full() {
		i.s.discoveryGap("result-limit", "PR ceiling leaves matching personal work uncollected.", p.Repo)
		return
	}
	i.entries[id] = model.PRInboxEntry{PR: p, Reasons: reasons, ReasonsComplete: complete && !search}
	i.s.missingMetadata(p)
}
func (i *prInboxCollector) changed(p model.PullRequest) {
	i.s.gap("provider-changed", "Repository verification contradicts search evidence; no transactional snapshot is claimed.", p.Repo, p.Number)
}
func (i *prInboxCollector) reconcile(p model.PullRequest) {
	id := prIdentity(p)
	old, exists := i.entries[id]
	if exists {
		before := old.Reasons
		// A missing field is not negative evidence. Keep only uncontradicted proof.
		merged := p
		if merged.Author == "" {
			merged.Author = old.PR.Author
		}
		if merged.Assignees == nil {
			merged.Assignees = old.PR.Assignees
		}
		if merged.RequestedReviewers == nil {
			merged.RequestedReviewers = old.PR.RequestedReviewers
		}
		if merged.State == nil {
			merged.State = old.PR.State
		}
		if merged.Draft == nil {
			merged.Draft = old.PR.Draft
		}
		after, _ := inboxReasons(merged, i.q.User)
		contradiction := slices.ContainsFunc(before, func(r model.PRRelationship) bool { return !slices.Contains(after, r) }) || (merged.State != nil && *merged.State != model.PRStateOpen) || (i.q.Draft != model.PRDraftAll && merged.Draft != nil && (*merged.Draft != (i.q.Draft == model.PRDraftOnly)))
		if contradiction {
			i.changed(p)
		}
		i.admit(merged, false)
		_, complete := inboxReasons(p, i.q.User)
		if entry, ok := i.entries[id]; ok && !complete {
			entry.ReasonsComplete = false
			i.entries[id] = entry
		}
		return
	}
	i.admit(p, false)
}
func verificationFacts(p model.PullRequest) model.PullRequest {
	return model.PullRequest{Repo: p.Repo, Number: p.Number, Author: p.Author, State: p.State, Draft: p.Draft, Assignees: p.Assignees, RequestedReviewers: p.RequestedReviewers}
}
func (i *prInboxCollector) verify(ctx context.Context, repo string) error {
	v := &prVerification{facts: map[string]model.PullRequest{}}
	i.verified[strings.ToLower(repo)] = v
	i.s.note("visibility", "Discovery method: search+repo-listing; repository verification may establish additional personal matches.", repo, 0)
	complete, err := i.s.listPRPages(ctx, repo, "open", func(page []model.PullRequest) error {
		for _, p := range page {
			v.facts[prIdentity(p)] = verificationFacts(p)
			i.reconcile(p)
		}
		return nil
	}, i.full)
	v.complete = complete && err == nil
	if i.s.publicOnly && i.s.privateRepos[strings.ToLower(repo)] {
		for id, entry := range i.entries {
			if strings.EqualFold(entry.PR.Repo, repo) {
				i.changed(entry.PR)
				delete(i.entries, id)
			}
		}
	}
	if v.complete {
		for id, entry := range i.entries {
			if strings.EqualFold(entry.PR.Repo, repo) {
				if _, seen := v.facts[id]; !seen {
					i.changed(entry.PR)
					delete(i.entries, id)
				}
			}
		}
	} else {
		i.s.gap("reasons-incomplete", "Repository verification did not finish; absence is not proof of removal.", repo, 0)
		for id, entry := range i.entries {
			if strings.EqualFold(entry.PR.Repo, repo) {
				if _, seen := v.facts[id]; !seen {
					entry.ReasonsComplete = false
					i.entries[id] = entry
				}
			}
		}
	}
	return err
}
func (i *prInboxCollector) searchHit(p model.PullRequest) {
	v, verified := i.verified[strings.ToLower(p.Repo)]
	if !verified {
		i.admit(p, true)
		return
	}
	id := prIdentity(p)
	fact, seen := v.facts[id]
	if !seen {
		if v.complete {
			i.changed(p)
			delete(i.entries, id)
		} else {
			i.admit(p, true)
		}
		return
	}
	before, _ := inboxReasons(p, i.q.User)
	if fact.Author != "" {
		p.Author = fact.Author
	}
	if fact.Assignees != nil {
		p.Assignees = fact.Assignees
	}
	if fact.RequestedReviewers != nil {
		p.RequestedReviewers = fact.RequestedReviewers
	}
	if fact.State != nil {
		p.State = fact.State
	}
	if fact.Draft != nil {
		p.Draft = fact.Draft
	}
	after, _ := inboxReasons(p, i.q.User)
	if slices.ContainsFunc(before, func(r model.PRRelationship) bool { return !slices.Contains(after, r) }) {
		i.changed(p)
	}
	if _, exists := i.entries[id]; !exists {
		i.admit(p, false)
		_, known := inboxReasons(fact, i.q.User)
		if entry, ok := i.entries[id]; ok && !known {
			entry.ReasonsComplete = false
			i.entries[id] = entry
		}
	}
}
func (i *prInboxCollector) search(ctx context.Context) error {
	type stream struct {
		query            string
		page, iterations int
		done             bool
	}
	streams := []stream{}
	for _, role := range []string{"author", "assignee", "review-requested"} {
		queries, err := prSearchChunks("is:pr is:open "+role+":"+i.q.User, i.q.Repos, i.q.Org)
		if err != nil {
			return err
		}
		for _, query := range queries {
			streams = append(streams, stream{query: query, page: 1})
		}
	}
	for {
		pending := false
		for n := range streams {
			st := &streams[n]
			if st.done {
				continue
			}
			pending = true
			if st.iterations >= maxPages {
				i.s.discoveryGap("pagination-limit", "Inbox search pagination safety limit reached.", "")
				return nil
			}
			st.iterations++
			page, next, err := i.s.searchPage(ctx, st.query, st.page)
			if err != nil {
				return err
			}
			st.page = next
			st.done = next == 0
			repos := []string{}
			for _, p := range page {
				if !prTargetMatch(p.Repo, i.q.Repos, i.q.Org) {
					continue
				}
				i.searchHit(p)
				repo := strings.ToLower(p.Repo)
				if _, ok := i.verified[repo]; !ok && !slices.Contains(repos, repo) {
					repos = append(repos, repo)
				}
			}
			for _, repo := range repos {
				if err = i.verify(ctx, repo); err != nil {
					return err
				}
			}
			if i.full() {
				if slices.ContainsFunc(streams, func(s stream) bool { return !s.done }) {
					i.s.discoveryGap("result-limit", "PR ceiling leaves relationship search streams unexamined.", "")
				}
				return nil
			}
		}
		if !pending {
			return nil
		}
	}
}
