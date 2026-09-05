// SPDX-License-Identifier: MIT
package gitlabclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

func TestRequestBudgetStopsPaginationWithDisclosure(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Query().Get("page") != "1" {
			t.Errorf("request reached page %q after the ceiling", r.URL.Query().Get("page"))
		}
		w.Header().Set("X-Next-Page", "2")
		_, _ = w.Write([]byte(gitlabCommitsBody))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, 100)
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat", Scope: model.ScopeRepos, Repos: []string{"a/b"}, MaxRequests: 1,
	})
	if err != nil {
		t.Fatalf("a request-budget stop must be a bounded result: %v", err)
	}
	if hits != 1 {
		t.Fatalf("dispatched %d requests, want 1", hits)
	}
	if res.Count != 1 || res.Commits[0].SHA != "abc123" {
		t.Fatalf("partial commits = %+v, want the first page retained", res.Commits)
	}
	if !res.Truncated {
		t.Error("Truncated = false after the request ceiling stopped pagination")
	}
	if res.Cost.Consumed != 1 || res.Cost.Ceiling != 1 {
		t.Errorf("Cost = %+v, want consumed=1 ceiling=1", res.Cost)
	}
	if len(res.Disclosures) != 1 || res.Disclosures[0].Kind != model.DisclosureBudgetBounded {
		t.Fatalf("Disclosures = %+v, want budget-bounded", res.Disclosures)
	}
}

func TestRequestBudgetCoversGroupDiscovery(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if !strings.Contains(r.URL.Path, "/groups/team/projects") {
			t.Errorf("unexpected dispatched path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":1,"path_with_namespace":"team/repo"}]`))
	}))
	defer srv.Close()

	c, err := New("token", srv.URL+"/api/v4/", 100, WithRequestBudget(1))
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat", Scope: model.ScopeOrg, Org: "team", MaxRequests: 1,
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if hits != 1 {
		t.Fatalf("dispatched %d requests, want 1", hits)
	}
	if len(res.Disclosures) != 1 || res.Disclosures[0].Kind != model.DisclosureBudgetBounded {
		t.Fatalf("Disclosures = %+v, want budget-bounded", res.Disclosures)
	}
}

func TestRequestBudgetCoversDiffPagination(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if isDiffPath(r.URL.EscapedPath()) {
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(gitlabDiffBody))
			return
		}
		_, _ = w.Write([]byte(gitlabCommitsBody))
	}))
	defer srv.Close()

	c, err := New("token", srv.URL+"/api/v4/", 100, WithRequestBudget(2))
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat", Scope: model.ScopeRepos, Repos: []string{"a/b"},
		IncludeFiles: true, MaxRequests: 2,
	})
	if err != nil {
		t.Fatalf("a diff budget stop must be a bounded result: %v", err)
	}
	if hits != 2 {
		t.Fatalf("dispatched %d requests, want list + one diff page", hits)
	}
	if len(res.Commits) != 1 || len(res.Commits[0].Files) != 2 {
		t.Fatalf("partial diff evidence = %+v, want the successful page retained", res.Commits)
	}
	if len(res.Disclosures) != 1 || res.Disclosures[0].Kind != model.DisclosureBudgetBounded {
		t.Fatalf("Disclosures = %+v, want budget-bounded", res.Disclosures)
	}
}

func TestUncappedRequestsAreStillAccounted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(gitlabCommitsBody))
	}))
	defer srv.Close()

	c, err := New("token", srv.URL+"/api/v4/", 100, WithRequestBudget(0))
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat", Scope: model.ScopeRepos, Repos: []string{"a/b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cost.Consumed != 1 || res.Cost.Ceiling != 0 {
		t.Errorf("Cost = %+v, want one request with no ceiling", res.Cost)
	}
}

func TestLaterCommitPageFailureRetainsEarlierPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			http.Error(w, "outage", http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Next-Page", "2")
		_, _ = w.Write([]byte(gitlabCommitsBody))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, 100)
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat", Scope: model.ScopeRepos, Repos: []string{"a/b"}, Until: time.Now(),
	})
	if err == nil {
		t.Fatal("expected the later-page provider error")
	}
	if res.Count != 1 || res.Commits[0].SHA != "abc123" {
		t.Fatalf("partial result = %+v, want first page retained", res)
	}
	if !res.Truncated {
		t.Error("Truncated = false on an incomplete result")
	}
}

func TestLaterRepoFailureRetainsEarlierRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.EscapedPath(), "projects/a%2Fgood"):
			_, _ = w.Write([]byte(gitlabCommitsBody))
		case strings.Contains(r.URL.EscapedPath(), "projects/b%2Fbad"):
			http.Error(w, "outage", http.StatusInternalServerError)
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, 100)
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat", Scope: model.ScopeRepos, Repos: []string{"a/good", "b/bad"},
	})
	if err == nil {
		t.Fatal("expected the second repository error")
	}
	if res.Count != 1 || res.Commits[0].Repo != "a/good" {
		t.Fatalf("partial commits = %+v, want the successful repository retained", res.Commits)
	}
}

func TestLaterInvalidRepoRetainsEarlierRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(gitlabCommitsBody))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, 100)
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat", Scope: model.ScopeRepos, Repos: []string{"a/good", " "},
	})
	if err == nil {
		t.Fatal("expected the invalid repository error")
	}
	if res.Count != 1 || res.Commits[0].Repo != "a/good" {
		t.Fatalf("partial commits = %+v, want the successful repository retained", res.Commits)
	}
}

func TestLaterGroupPageFailureRetainsEarlierProjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.EscapedPath(), "/groups/team/projects"):
			if r.URL.Query().Get("page") == "2" {
				http.Error(w, "outage", http.StatusInternalServerError)
				return
			}
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[{"id":1,"path_with_namespace":"team/good"}]`))
		case strings.Contains(r.URL.EscapedPath(), "/projects/1/repository/commits"):
			_, _ = w.Write([]byte(gitlabCommitsBody))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, 100)
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat", Scope: model.ScopeOrg, Org: "team",
	})
	if err == nil {
		t.Fatal("expected the later group-page error")
	}
	if res.Count != 1 || res.Commits[0].Repo != "team/good" {
		t.Fatalf("partial commits = %+v, want the successful project retained", res.Commits)
	}
}

func TestLaterDiffPageFailureRetainsEarlierDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isDiffPath(r.URL.EscapedPath()) {
			if r.URL.Query().Get("page") == "2" {
				http.Error(w, "outage", http.StatusInternalServerError)
				return
			}
			w.Header().Set("X-Next-Page", "2")
			_, _ = fmt.Fprintf(w, `[{"old_path":"a.go","new_path":"a.go","diff":%q}]`, "+line\n")
			return
		}
		_, _ = w.Write([]byte(gitlabCommitsBody))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, 100)
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat", Scope: model.ScopeRepos, Repos: []string{"a/b"}, IncludeFiles: true,
	})
	if err == nil {
		t.Fatal("expected the later diff-page error")
	}
	if res.Count != 1 || len(res.Commits[0].Files) != 1 {
		t.Fatalf("partial enrichment = %+v, want first diff page retained", res.Commits)
	}
}
