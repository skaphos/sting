// SPDX-License-Identifier: MIT
package gitlabclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

func newTestClient(t *testing.T, serverURL string, perPage int) *Client {
	t.Helper()
	c, err := New("test-token", serverURL+"/api/v4/", perPage)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

const gitlabCommitsBody = `[
  {
    "id": "abc123",
    "short_id": "abc123",
    "title": "repo commit message",
    "message": "repo commit message\n\nbody",
    "author_name": "Octo Cat",
    "author_email": "octo@example.com",
    "authored_date": "2026-05-21T11:00:00Z",
    "web_url": "https://gitlab.example.com/skaphos/sting/-/commit/abc123",
    "stats": {"additions": 42, "deletions": 7, "total": 49}
  }
]`

func TestCollectScopeRepos(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.EscapedPath(); !strings.Contains(got, "/projects/skaphos%2Fsting/repository/commits") {
			t.Errorf("unexpected path %q", got)
		}
		if got := r.URL.Query().Get("author"); got != "octocat" {
			t.Errorf("author query = %q, want octocat", got)
		}
		if got := r.URL.Query().Get("with_stats"); got != "true" {
			t.Errorf("with_stats = %q, want true", got)
		}
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "test-token" {
			t.Errorf("PRIVATE-TOKEN = %q, want test-token", got)
		}
		_, _ = w.Write([]byte(gitlabCommitsBody))
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
	if res.Provider != model.ProviderGitLab {
		t.Errorf("Provider = %q, want gitlab", res.Provider)
	}
	if res.Count != 1 {
		t.Fatalf("Count = %d, want 1", res.Count)
	}
	cm := res.Commits[0]
	if cm.SHA != "abc123" {
		t.Errorf("SHA = %q, want abc123", cm.SHA)
	}
	if cm.Repo != "skaphos/sting" {
		t.Errorf("Repo = %q, want skaphos/sting", cm.Repo)
	}
	if cm.AuthorName != "Octo Cat" {
		t.Errorf("AuthorName = %q, want Octo Cat", cm.AuthorName)
	}
	if cm.Email != "octo@example.com" {
		t.Errorf("Email = %q, want octo@example.com", cm.Email)
	}
	if cm.Additions != 42 || cm.Deletions != 7 {
		t.Errorf("Additions/Deletions = %d/%d, want 42/7", cm.Additions, cm.Deletions)
	}
	wantDate := time.Date(2026, 5, 21, 11, 0, 0, 0, time.UTC)
	if !cm.Date.Equal(wantDate) {
		t.Errorf("Date = %v, want %v", cm.Date, wantDate)
	}
}

func TestCollectScopeOrg(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case strings.Contains(path, "/groups/skaphos/projects"):
			if got := r.URL.Query().Get("include_subgroups"); got != "true" {
				t.Errorf("include_subgroups = %q, want true", got)
			}
			_, _ = w.Write([]byte(`[{"id":42,"path_with_namespace":"skaphos/sting"}]`))
		case strings.Contains(path, "/projects/42/repository/commits"):
			_, _ = w.Write([]byte(gitlabCommitsBody))
		default:
			t.Errorf("unexpected path %q", path)
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
	if res.Commits[0].Repo != "skaphos/sting" {
		t.Errorf("Repo = %q, want skaphos/sting", res.Commits[0].Repo)
	}
}

func TestCollectPaginationAndMaxCommits(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "1":
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[{"id":"one","message":"m1","authored_date":"2026-05-21T11:00:00Z"}]`))
		case "2":
			_, _ = w.Write([]byte(`[{"id":"two","message":"m2","authored_date":"2026-05-22T11:00:00Z"}]`))
		default:
			t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, 50)
	res, err := c.Collect(context.Background(), model.Query{
		Author:     "octocat",
		Scope:      model.ScopeRepos,
		Repos:      []string{"skaphos/sting"},
		MaxCommits: 2,
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !res.Truncated {
		t.Error("Truncated = false, want true")
	}
	if res.Count != 2 {
		t.Fatalf("Count = %d, want 2", res.Count)
	}
}

func TestCollectUnsupportedAndInvalidInputs(t *testing.T) {
	c, err := New("", "", 10)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Collect(context.Background(), model.Query{Scope: model.ScopeSearch}); err == nil {
		t.Fatal("expected error for search scope")
	}
	if _, err := c.Collect(context.Background(), model.Query{Scope: model.ScopeRepos}); err == nil {
		t.Fatal("expected error for empty repos")
	}
	if _, err := c.Collect(context.Background(), model.Query{Scope: model.ScopeOrg}); err == nil {
		t.Fatal("expected error for empty org")
	}
}

func TestCollectAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, 50)
	_, err := c.Collect(context.Background(), model.Query{
		Author: "octocat",
		Scope:  model.ScopeRepos,
		Repos:  []string{"skaphos/sting"},
	})
	if err == nil {
		t.Fatal("Collect: want error")
	}
	if !strings.Contains(err.Error(), "gitlab api status 429") {
		t.Fatalf("Collect error = %q, want status context", err)
	}
}

func TestNewMalformedBaseURL(t *testing.T) {
	if _, err := New("", "://bad", 10); err == nil {
		t.Fatal("expected error for malformed baseURL")
	}
	if _, err := New("", "gitlab.example.com/api/v4", 10); err == nil {
		t.Fatal("expected error for missing scheme")
	}
}

func TestNewPerPageClamping(t *testing.T) {
	for _, pp := range []int{0, -5, 200} {
		c, err := New("", "", pp)
		if err != nil {
			t.Fatalf("New(perPage=%d): %v", pp, err)
		}
		if c.perPage != 100 {
			t.Errorf("New(perPage=%d).perPage = %d, want 100", pp, c.perPage)
		}
	}
	c, err := New("", "", 50)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.perPage != 50 {
		t.Errorf("perPage = %d, want 50", c.perPage)
	}
}
