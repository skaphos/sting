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
				"author": {"login": "mendedlink"},
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
		Author: "mendedlink",
		Scope:  "search",
	})
	if err != nil {
		t.Fatalf("getCommits returned error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected non-error result, got %+v", res)
	}
	if mr.Author != "mendedlink" {
		t.Errorf("Author = %q, want %q", mr.Author, "mendedlink")
	}
	if mr.Count != 1 {
		t.Errorf("Count = %d, want 1", mr.Count)
	}
	txt := firstText(res)
	if txt == "" {
		t.Fatal("expected Markdown TextContent in result")
	}
	if !strings.Contains(txt, "mendedlink") {
		t.Errorf("rendered Markdown missing author; got:\n%s", txt)
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
		Author:   "mendedlink",
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
	if txt := firstText(res); !strings.Contains(txt, "mendedlink") {
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
		Author: "mendedlink",
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
