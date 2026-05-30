// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

// newTestClient builds a Client pointed at the given test server.
func newTestClient(t *testing.T, serverURL string, perPage int) *Client {
	t.Helper()
	c, err := New("test-token", serverURL+"/", perPage)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// searchResponse is one item's worth of a commit-search response.
const searchBody = `{
  "total_count": 1,
  "incomplete_results": false,
  "items": [
    {
      "sha": "abc123",
      "html_url": "https://example.com/c/abc123",
      "author": {"login": "octocat"},
      "repository": {"full_name": "skaphos/sting"},
      "commit": {
        "message": "search commit message",
        "author": {"name": "Octo Cat", "email": "octo@example.com", "date": "2026-05-20T10:00:00Z"}
      }
    }
  ]
}`

// repoCommitsBody is a list of repository commits for one repo.
const repoCommitsBody = `[
  {
    "sha": "def456",
    "html_url": "https://example.com/c/def456",
    "author": {"login": "octocat"},
    "commit": {
      "message": "repo commit message",
      "author": {"name": "Octo Cat", "email": "octo@example.com", "date": "2026-05-21T11:00:00Z"}
    }
  }
]`

const orgReposBody = `[{"full_name": "skaphos/sting"}]`

const getCommitStatsBody = `{
  "sha": "def456",
  "stats": {"additions": 42, "deletions": 7}
}`

func TestCollectScopeSearch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "search/commits") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(searchBody))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, 50)
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat",
		Scope:  model.ScopeSearch,
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if res.Count != 1 || len(res.Commits) != 1 {
		t.Fatalf("Count = %d, len(Commits) = %d, want 1/1", res.Count, len(res.Commits))
	}
	cm := res.Commits[0]
	if cm.SHA != "abc123" {
		t.Errorf("SHA = %q, want abc123", cm.SHA)
	}
	if cm.Repo != "skaphos/sting" {
		t.Errorf("Repo = %q, want skaphos/sting", cm.Repo)
	}
	if cm.Author != "octocat" {
		t.Errorf("Author = %q, want octocat", cm.Author)
	}
	if cm.Message != "search commit message" {
		t.Errorf("Message = %q", cm.Message)
	}
	want := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	if !cm.Date.Equal(want) {
		t.Errorf("Date = %v, want %v", cm.Date, want)
	}
	if res.Scope != model.ScopeSearch {
		t.Errorf("Scope = %q", res.Scope)
	}
	if res.Until.IsZero() {
		t.Error("Until should be defaulted to now, got zero")
	}
}

func TestCollectScopeRepos(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/repos/skaphos/sting/commits") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(repoCommitsBody))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, 50)
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat",
		Scope:  model.ScopeRepos,
		Repos:  []string{"skaphos/sting"},
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if res.Count != 1 {
		t.Fatalf("Count = %d, want 1", res.Count)
	}
	cm := res.Commits[0]
	if cm.SHA != "def456" {
		t.Errorf("SHA = %q, want def456", cm.SHA)
	}
	if cm.Repo != "skaphos/sting" {
		t.Errorf("Repo = %q, want skaphos/sting", cm.Repo)
	}
	if cm.Author != "octocat" {
		t.Errorf("Author = %q, want octocat", cm.Author)
	}
	if cm.Message != "repo commit message" {
		t.Errorf("Message = %q", cm.Message)
	}
	want := time.Date(2026, 5, 21, 11, 0, 0, 0, time.UTC)
	if !cm.Date.Equal(want) {
		t.Errorf("Date = %v, want %v", cm.Date, want)
	}
}

func TestCollectScopeOrg(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/orgs/skaphos/repos"):
			_, _ = w.Write([]byte(orgReposBody))
		case strings.Contains(r.URL.Path, "/repos/skaphos/sting/commits"):
			_, _ = w.Write([]byte(repoCommitsBody))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, 50)
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat",
		Scope:  model.ScopeOrg,
		Org:    "skaphos",
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if res.Count != 1 {
		t.Fatalf("Count = %d, want 1", res.Count)
	}
	if res.Commits[0].SHA != "def456" {
		t.Errorf("SHA = %q, want def456", res.Commits[0].SHA)
	}
}

func TestCollectUnsupportedScope(t *testing.T) {
	c, err := New("", "", 10)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Collect(context.Background(), model.Query{Scope: model.Scope("bogus")}); err == nil {
		t.Fatal("expected error for unsupported scope")
	}
	if _, err := c.Collect(context.Background(), model.Query{Scope: model.Scope("")}); err == nil {
		t.Fatal("expected error for empty scope")
	}
}

func TestListReposEmpty(t *testing.T) {
	c, err := New("", "", 10)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Collect(context.Background(), model.Query{Scope: model.ScopeRepos}); err == nil {
		t.Fatal("expected error for empty repos")
	}
}

func TestListReposInvalidTarget(t *testing.T) {
	c, err := New("", "", 10)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Collect(context.Background(), model.Query{
		Scope: model.ScopeRepos,
		Repos: []string{"noslash"},
	}); err == nil {
		t.Fatal("expected error for invalid repo target")
	}
}

func TestListOrgEmpty(t *testing.T) {
	c, err := New("", "", 10)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Collect(context.Background(), model.Query{Scope: model.ScopeOrg}); err == nil {
		t.Fatal("expected error for empty org")
	}
}

func TestCollectMaxCommitsTruncation(t *testing.T) {
	// Search returns two items; MaxCommits=1 should clip and mark truncated.
	const twoItems = `{
      "total_count": 2,
      "incomplete_results": false,
      "items": [
        {"sha": "one", "repository": {"full_name": "o/r"}, "commit": {"message": "m1", "author": {"date": "2026-05-20T10:00:00Z"}}},
        {"sha": "two", "repository": {"full_name": "o/r"}, "commit": {"message": "m2", "author": {"date": "2026-05-20T11:00:00Z"}}}
      ]
    }`
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(twoItems))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, 50)
	res, err := c.Collect(context.Background(), model.Query{
		Author:     "octocat",
		Scope:      model.ScopeSearch,
		MaxCommits: 1,
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !res.Truncated {
		t.Error("Truncated = false, want true")
	}
	if res.Count != 1 || len(res.Commits) != 1 {
		t.Fatalf("Count = %d, len = %d, want 1/1", res.Count, len(res.Commits))
	}
	if res.Commits[0].SHA != "one" {
		t.Errorf("SHA = %q, want one", res.Commits[0].SHA)
	}
}

func TestCollectIncludeStats(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/repos/skaphos/sting/commits/def456"):
			_, _ = w.Write([]byte(getCommitStatsBody))
		case strings.Contains(r.URL.Path, "/repos/skaphos/sting/commits"):
			_, _ = w.Write([]byte(repoCommitsBody))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, 50)
	res, err := c.Collect(context.Background(), model.Query{
		Author:       "octocat",
		Scope:        model.ScopeRepos,
		Repos:        []string{"skaphos/sting"},
		IncludeStats: true,
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if res.Count != 1 {
		t.Fatalf("Count = %d, want 1", res.Count)
	}
	cm := res.Commits[0]
	if cm.Additions != 42 || cm.Deletions != 7 {
		t.Errorf("Additions/Deletions = %d/%d, want 42/7", cm.Additions, cm.Deletions)
	}
}

func TestNewMalformedBaseURL(t *testing.T) {
	if _, err := New("", "://bad", 10); err == nil {
		t.Fatal("expected error for malformed baseURL")
	}
}

func TestNewPerPageClamping(t *testing.T) {
	// perPage < 1 and > 100 should both clamp without error.
	for _, pp := range []int{0, -5, 200} {
		c, err := New("", "", pp)
		if err != nil {
			t.Fatalf("New(perPage=%d): %v", pp, err)
		}
		if c.perPage != 100 {
			t.Errorf("New(perPage=%d).perPage = %d, want 100", pp, c.perPage)
		}
	}
	// In-range value is preserved.
	c, err := New("", "", 50)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.perPage != 50 {
		t.Errorf("perPage = %d, want 50", c.perPage)
	}
}
