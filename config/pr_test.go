// SPDX-License-Identifier: MIT
package config

import (
	"reflect"
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

func TestPRResolution(t *testing.T) {
	now := time.Date(2026, 8, 25, 13, 0, 0, 0, time.UTC)
	cfg := Default()
	cfg.DefaultProvider = model.ProviderGitLab
	q, err := cfg.ResolvePRs(PRRequest{Author: "Octocat"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if q.Provider != model.ProviderGitHub || q.State != "all" || q.TimeBasis != "any" || q.Draft != "all" || q.MaxPRs != 100 || q.MaxRequests != 500 || !q.Since.Equal(now.Add(-7*24*time.Hour)) {
		t.Fatalf("defaults: %+v", q)
	}
	zero := 0
	no := false
	q, err = cfg.ResolvePRs(PRRequest{Scope: "repos", Repos: []string{"ACME/api", "acme/API", "acme/web"}, Since: "2026-08-24T08:00:00-05:00", Until: "2026-08-24T13:00:00Z", Window: "invalid", Draft: &no, MaxPRs: &zero, MaxRequests: &zero}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !q.Since.Equal(q.Until) || q.Since.Location() != time.UTC || q.MaxPRs != 0 || q.Draft != "exclude" || !reflect.DeepEqual(q.Repos, []string{"acme/api", "acme/web"}) {
		t.Fatalf("resolution: %+v", q)
	}
	cfg.DefaultWindow = "invalid"
	cfg.DefaultRepos = []string{"bad:unused"}
	iq, err := cfg.ResolvePRInbox(PRInboxRequest{Scope: "org", Org: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if iq.User != "" || iq.IdentitySource != "authenticated" || iq.State != "open" || len(iq.Relationships) != 3 {
		t.Fatalf("inbox: %+v", iq)
	}
}
func TestPRValidation(t *testing.T) {
	neg := -1
	tests := []PRRequest{{}, {Author: "a is:closed"}, {Author: "a@b.com"}, {Author: "a", Provider: "gitlab"}, {Author: "a", Provider: "bogus"}, {Author: "a", Scope: "bad"}, {Scope: "repos"}, {Scope: "org"}, {Author: "a", Role: "reviewer"}, {Author: "a", State: "bad"}, {Author: "a", TimeBasis: "bad"}, {Author: "a", State: "open", TimeBasis: "merged"}, {Author: "a", MaxPRs: &neg}, {Author: "a", MaxRequests: &neg}, {Author: "a", Repos: []string{"a/b is:closed"}}, {Scope: "repos", Repos: []string{"a/b"}, Org: "a"}, {Scope: "org", Org: "a", Repos: []string{"a/b"}}, {Author: "a", Since: "2026-08-26", Until: "2026-08-25"}}
	for _, req := range tests {
		if _, err := Default().ResolvePRs(req, time.Now()); err == nil {
			t.Errorf("accepted %+v", req)
		}
	}
	for _, req := range []PRInboxRequest{{User: "a@b"}, {User: "a", Provider: "gitlab"}, {Scope: "repos"}, {Scope: "org"}, {MaxPRs: &neg}, {MaxRequests: &neg}} {
		if _, err := Default().ResolvePRInbox(req); err == nil {
			t.Errorf("accepted inbox %+v", req)
		}
	}
}

func TestPRTargetDefaultsAndOverrides(t *testing.T) {
	cfg := Default()
	cfg.DefaultRepos = []string{"Other/Repo"}
	cfg.DefaultOrg = "acme"
	q, err := cfg.ResolvePRs(PRRequest{Author: "octocat", Repos: []string{"acme/api"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if q.Org != "acme" || !reflect.DeepEqual(q.Repos, []string{"acme/api"}) {
		t.Fatalf("intersection: %+v", q)
	}
	yes := true
	iq, err := cfg.ResolvePRInbox(PRInboxRequest{User: "Octocat", Scope: "repos", Draft: &yes})
	if err != nil {
		t.Fatal(err)
	}
	if iq.Org != "" || iq.User != "Octocat" || iq.IdentitySource != "explicit" || iq.Draft != "only" || !reflect.DeepEqual(iq.Repos, []string{"other/repo"}) {
		t.Fatalf("inbox override: %+v", iq)
	}
	cfg.PerPage = 0
	if _, err = cfg.ResolvePRInbox(PRInboxRequest{}); err == nil {
		t.Fatal("invalid page size")
	}
}

// max_prs belongs to the PR workflows: Config.Validate must tolerate it so an
// invalid PR default cannot reject unrelated commit and activity tool calls,
// while both PR resolvers still reject it before any provider access.
func TestNegativeMaxPRsRejectedOnlyByPRResolvers(t *testing.T) {
	cfg := Default()
	cfg.MaxPRs = -1
	if err := cfg.Validate(); err != nil {
		t.Fatalf("PR default rejected an unrelated workflow: %v", err)
	}
	if _, err := cfg.ResolvePRInbox(PRInboxRequest{User: "octocat"}); err == nil {
		t.Fatal("inbox accepted a negative max_prs")
	}
	if _, err := cfg.ResolvePRs(PRRequest{Author: "octocat"}, time.Now()); err == nil {
		t.Fatal("activity accepted a negative max_prs")
	}
}
