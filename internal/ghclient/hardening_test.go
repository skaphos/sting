// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v82/github"
	"github.com/skaphos/sting/internal/model"
)

func TestAPIErrorClassification(t *testing.T) {
	reset := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	rl := &github.RateLimitError{Rate: github.Rate{Reset: github.Timestamp{Time: reset}}, Message: "limit"}
	if got := apiError("op", rl); !strings.Contains(got.Error(), "rate limit exceeded") ||
		!strings.Contains(got.Error(), "2026-05-30") {
		t.Errorf("rate-limit error = %q", got)
	}

	ab := &github.AbuseRateLimitError{Message: "slow down"}
	if got := apiError("op", ab); !strings.Contains(got.Error(), "secondary rate limit") {
		t.Errorf("abuse error = %q", got)
	}

	plain := github.ErrorResponse{Message: "boom"}
	if got := apiError("op", &plain); !strings.Contains(got.Error(), "op:") {
		t.Errorf("plain error = %q", got)
	}
}

// TestCollectReposMaxCommits verifies the repo scope stops at MaxCommits and
// reports truncation rather than fetching every page first.
func TestCollectReposMaxCommits(t *testing.T) {
	const body = `[
	  {"sha":"a1","html_url":"u1","commit":{"message":"first","author":{"name":"A","date":"2026-05-29T00:00:00Z"}}},
	  {"sha":"a2","html_url":"u2","commit":{"message":"second","author":{"name":"A","date":"2026-05-28T00:00:00Z"}}},
	  {"sha":"a3","html_url":"u3","commit":{"message":"third","author":{"name":"A","date":"2026-05-27T00:00:00Z"}}}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c, err := New("", srv.URL+"/", 100)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Collect(context.Background(), model.Query{
		Author: "a", Scope: model.ScopeRepos, Repos: []string{"o/r"}, MaxCommits: 2,
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if res.Count != 2 {
		t.Errorf("count = %d, want 2", res.Count)
	}
	if !res.Truncated {
		t.Error("Truncated = false, want true when MaxCommits cap is hit")
	}
}
