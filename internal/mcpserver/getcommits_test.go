// SPDX-License-Identifier: MIT
package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

// firstText returns the text of the first TextContent in res, or "" if none.
func firstText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// newTestHandler builds a handler whose client points at srv.
func newTestHandler(t *testing.T, srv *httptest.Server) *handler {
	t.Helper()
	cfg := config.Default()
	cfg.BaseURL = srv.URL + "/"
	return &handler{cfg: cfg}
}

// TestGetCommitsResolveError covers the resolve-failure branch (and errorResult):
// an empty Author fails config.Resolve, so getCommits returns an IsError result
// with non-empty text and a zero model.Result, and no Go error.
func TestGetCommitsResolveError(t *testing.T) {
	h := &handler{cfg: config.Default()}

	res, mr, err := h.getCommits(context.Background(), nil, GetCommitsInput{Author: ""})
	if err != nil {
		t.Fatalf("getCommits returned error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError result, got %+v", res)
	}
	if txt := firstText(res); txt == "" {
		t.Fatal("expected non-empty error text")
	}
	if !isZeroResult(mr) {
		t.Errorf("expected zero model.Result, got %+v", mr)
	}
}

// isZeroResult reports whether r carries no data (the value returned alongside
// an error result).
func isZeroResult(r model.Result) bool {
	return r.Author == "" && r.Count == 0 && len(r.Commits) == 0 &&
		r.Scope == "" && r.Since.IsZero() && r.Until.IsZero()
}

// TestGetCommitsSuccess covers the collect + render branch: the test server
// returns a minimal valid commit-search payload, so Collect succeeds and
// getCommits returns a non-error result carrying the rendered Markdown and the
// expected author/count.
func TestGetCommitsSuccess(t *testing.T) {
	const payload = `{
		"total_count": 1,
		"incomplete_results": false,
		"items": [
			{
				"sha": "abc123",
				"html_url": "https://github.com/skaphos/sting/commit/abc123",
				"author": {"login": "mfacenet"},
				"repository": {"full_name": "skaphos/sting"},
				"commit": {
					"message": "feat: add thing",
					"author": {
						"name": "Mended Link",
						"email": "mended@example.com",
						"date": "2026-05-29T12:00:00Z"
					}
				}
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/search/commits") {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	h := newTestHandler(t, srv)

	res, mr, err := h.getCommits(context.Background(), nil, GetCommitsInput{
		Author: "mfacenet",
		Scope:  "search",
	})
	if err != nil {
		t.Fatalf("getCommits returned error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected non-error result, got %+v", res)
	}
	if mr.Author != "mfacenet" {
		t.Errorf("Author = %q, want %q", mr.Author, "mfacenet")
	}
	if mr.Count != 1 {
		t.Errorf("Count = %d, want 1", mr.Count)
	}
	txt := firstText(res)
	if txt == "" {
		t.Fatal("expected Markdown TextContent in result")
	}
	if !strings.Contains(txt, "mfacenet") {
		t.Errorf("rendered Markdown missing author; got:\n%s", txt)
	}
}

// TestGetCommitsIncludePRs covers the include_prs input branch: with the flag
// set on a repos-scope query, the handler discovers a commit on an open PR
// branch in addition to the default-branch commit.
func TestGetCommitsIncludePRs(t *testing.T) {
	const repoCommits = `[{"sha":"def456","author":{"login":"octocat"},"commit":{"message":"merged work","author":{"name":"Octo","email":"octo@example.com","date":"2026-05-21T11:00:00Z"}}}]`
	const openPRs = `[{"number":5,"state":"open"}]`
	const prCommits = `[{"sha":"pr789","html_url":"https://example.com/c/pr789","author":{"login":"octocat"},"commit":{"message":"wip on branch","author":{"name":"Octo","email":"octo@example.com","date":"2026-05-22T09:00:00Z"}}}]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/pulls/5/commits"):
			_, _ = w.Write([]byte(prCommits))
		case strings.Contains(r.URL.Path, "/pulls"):
			_, _ = w.Write([]byte(openPRs))
		case strings.Contains(r.URL.Path, "/commits"):
			_, _ = w.Write([]byte(repoCommits))
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	h := newTestHandler(t, srv)

	res, mr, err := h.getCommits(context.Background(), nil, GetCommitsInput{
		Author:     "octocat",
		Scope:      "repos",
		Repos:      []string{"skaphos/sting"},
		Since:      "2026-05-01",
		Until:      "2026-05-31",
		IncludePRs: true,
	})
	if err != nil {
		t.Fatalf("getCommits returned error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected non-error result, got %+v", res)
	}
	if mr.Count != 2 {
		t.Fatalf("Count = %d, want 2 (default-branch + PR-branch)", mr.Count)
	}
	var foundPR bool
	for _, cm := range mr.Commits {
		if cm.SHA == "pr789" && cm.Source == "pull/5" {
			foundPR = true
		}
	}
	if !foundPR {
		t.Errorf("expected PR-branch commit pr789 tagged source pull/5; got %+v", mr.Commits)
	}
}

func TestGetCommitsGitLabSuccess(t *testing.T) {
	const payload = `[
		{
			"id": "abc123",
			"message": "feat: add gitlab",
			"author_name": "Mended Link",
			"author_email": "mended@example.com",
			"authored_date": "2026-05-29T12:00:00Z",
			"web_url": "https://gitlab.example.com/skaphos/sting/-/commit/abc123"
		}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.EscapedPath(), "/projects/skaphos%2Fsting/repository/commits") {
			http.Error(w, "unexpected path "+r.URL.EscapedPath(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.GitLabBaseURL = srv.URL + "/api/v4/"
	h := &handler{cfg: cfg}

	res, mr, err := h.getCommits(context.Background(), nil, GetCommitsInput{
		Provider: "gitlab",
		Author:   "mfacenet",
		Scope:    "repos",
		Repos:    []string{"skaphos/sting"},
	})
	if err != nil {
		t.Fatalf("getCommits returned error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected non-error result, got %+v", res)
	}
	if mr.Provider != model.ProviderGitLab {
		t.Errorf("Provider = %q, want gitlab", mr.Provider)
	}
	if mr.Count != 1 {
		t.Errorf("Count = %d, want 1", mr.Count)
	}
	if txt := firstText(res); !strings.Contains(txt, "mfacenet") {
		t.Errorf("rendered Markdown missing author; got:\n%s", txt)
	}
}

// TestGetCommitsCollectError covers the collect-failure branch: the search
// endpoint returns HTTP 500, so Collect errors and getCommits returns an
// IsError result with a zero model.Result and no Go error.
func TestGetCommitsCollectError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := newTestHandler(t, srv)

	res, mr, err := h.getCommits(context.Background(), nil, GetCommitsInput{
		Author: "mfacenet",
		Scope:  "search",
	})
	if err != nil {
		t.Fatalf("getCommits returned error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError result, got %+v", res)
	}
	if firstText(res) == "" {
		t.Fatal("expected non-empty error text")
	}
	if !isZeroResult(mr) {
		t.Errorf("expected zero model.Result, got %+v", mr)
	}
}

// TestErrorResult exercises errorResult directly: it marks the result as an
// error and surfaces the error text to the agent.
func TestErrorResult(t *testing.T) {
	res := errorResult(errors.New("something failed"))
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError result, got %+v", res)
	}
	if got := firstText(res); got != "something failed" {
		t.Errorf("text = %q, want %q", got, "something failed")
	}
}
