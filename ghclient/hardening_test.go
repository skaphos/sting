// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v82/github"
	"github.com/skaphos/sting/model"
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

// TestSkipRepoReason checks which per-repo failures an org scan skips past and
// which global failures it still treats as fatal.
func TestSkipRepoReason(t *testing.T) {
	status := func(code int) error {
		return apiError("op", &github.ErrorResponse{
			Response: &http.Response{StatusCode: code},
			Message:  "x",
		})
	}
	cases := []struct {
		name   string
		err    error
		reason string
		skip   bool
	}{
		{"empty 409", status(http.StatusConflict), "empty repository", true},
		{"not found 404", status(http.StatusNotFound), "not found", true},
		{"gone 410", status(http.StatusGone), "gone", true},
		{"forbidden 403", status(http.StatusForbidden), "access forbidden", true},
		{"legal 451", status(http.StatusUnavailableForLegalReasons), "unavailable for legal reasons", true},
		{"server 500 fatal", status(http.StatusInternalServerError), "", false},
		{"bad request 400 fatal", status(http.StatusBadRequest), "", false},
		{"rate limit fatal", apiError("op", &github.RateLimitError{
			Rate: github.Rate{Reset: github.Timestamp{Time: time.Unix(0, 0)}}, Message: "limit",
		}), "", false},
		{"secondary rate limit fatal", apiError("op", &github.AbuseRateLimitError{Message: "slow down"}), "", false},
		{"non-github error fatal", errors.New("dial tcp: connection refused"), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, skip := skipRepoReason(tc.err)
			if skip != tc.skip || reason != tc.reason {
				t.Errorf("skipRepoReason = (%q, %v), want (%q, %v)", reason, skip, tc.reason, tc.skip)
			}
		})
	}
}

// TestSkipRepoReasonRateLimit403 verifies that a 403 carrying rate-limit
// signals is classified as fatal (so an org scan aborts) rather than skipped as
// a benign per-repo permission denial, even when go-github surfaces it as a
// plain ErrorResponse rather than a RateLimitError/AbuseRateLimitError.
func TestSkipRepoReasonRateLimit403(t *testing.T) {
	forbidden := func(h http.Header, msg string) error {
		return apiError("op", &github.ErrorResponse{
			Response: &http.Response{StatusCode: http.StatusForbidden, Header: h},
			Message:  msg,
		})
	}
	retryAfter := http.Header{}
	retryAfter.Set("Retry-After", "60")
	exhausted := http.Header{}
	exhausted.Set("X-RateLimit-Remaining", "0")

	cases := []struct {
		name   string
		err    error
		reason string
		skip   bool
	}{
		{"secondary limit via Retry-After", forbidden(retryAfter, "slow down"), "", false},
		{"primary limit via X-RateLimit-Remaining", forbidden(exhausted, "api rate limit"), "", false},
		{"secondary limit via message", forbidden(http.Header{}, "You have exceeded a secondary rate limit"), "", false},
		{"genuine permission denial", forbidden(http.Header{}, "Must have push access to view repository collaborators"), "access forbidden", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, skip := skipRepoReason(tc.err)
			if skip != tc.skip || reason != tc.reason {
				t.Errorf("skipRepoReason = (%q, %v), want (%q, %v)", reason, skip, tc.reason, tc.skip)
			}
		})
	}
}

// TestCollectScopeOrgRateLimit403Aborts verifies end to end that a
// secondary-rate-limit 403 during an org scan aborts the whole scan rather than
// being recorded as a per-repo skip, which would silently drop repos.
func TestCollectScopeOrgRateLimit403Aborts(t *testing.T) {
	const twoRepos = `[{"full_name":"skaphos/limited"},{"full_name":"skaphos/sting"}]`
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/orgs/skaphos/repos"):
			_, _ = w.Write([]byte(twoRepos))
		case strings.Contains(r.URL.Path, "/repos/skaphos/limited/commits"):
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"message":"You have exceeded a secondary rate limit"}`, http.StatusForbidden)
		default:
			t.Errorf("unexpected path %q (scan should abort before reaching it)", r.URL.Path)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New("", srv.URL+"/", 50)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Collect(context.Background(), model.Query{
		Author: "octocat", Scope: model.ScopeOrg, Org: "skaphos",
	}); err == nil {
		t.Fatal("Collect: want abort on secondary rate-limit 403; must not be skipped per repo")
	}
}

// TestCollectScopeOrgPermission403Skips verifies the counterpart: a genuine
// permission 403 (no rate-limit signals) is still skipped so one inaccessible
// repo does not abort the scan.
func TestCollectScopeOrgPermission403Skips(t *testing.T) {
	const twoRepos = `[{"full_name":"skaphos/private"},{"full_name":"skaphos/sting"}]`
	const commits = `[{"sha":"z1","html_url":"u","commit":{"message":"m","author":{"name":"A","date":"2026-05-29T00:00:00Z"}}}]`
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/orgs/skaphos/repos"):
			_, _ = w.Write([]byte(twoRepos))
		case strings.Contains(r.URL.Path, "/repos/skaphos/private/commits"):
			http.Error(w, `{"message":"Must have push access to view repository collaborators."}`, http.StatusForbidden)
		case strings.Contains(r.URL.Path, "/repos/skaphos/sting/commits"):
			_, _ = w.Write([]byte(commits))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New("", srv.URL+"/", 50)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat", Scope: model.ScopeOrg, Org: "skaphos",
	})
	if err != nil {
		t.Fatalf("Collect: %v (permission 403 must be skipped, not fatal)", err)
	}
	if res.Count != 1 || len(res.Skipped) != 1 {
		t.Fatalf("Count = %d, Skipped = %+v, want 1 commit and 1 skip", res.Count, res.Skipped)
	}
	if res.Skipped[0].Repo != "skaphos/private" || res.Skipped[0].Reason != "access forbidden" {
		t.Errorf("Skipped[0] = %+v, want {skaphos/private access forbidden}", res.Skipped[0])
	}
}

// TestSearchReposWindowParity verifies the search scope filters the commit
// window at the same second precision as the repos scope: the search qualifier
// must carry full RFC3339 timestamps, matching the since/until the repos path
// sends, so the two scopes cannot return materially different evidence for a
// sub-day window.
func TestSearchReposWindowParity(t *testing.T) {
	since := time.Date(2026, 5, 22, 9, 30, 0, 0, time.UTC)
	until := time.Date(2026, 5, 22, 17, 45, 0, 0, time.UTC)

	var searchQ, repoSince, repoUntil string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "search/commits"):
			searchQ = r.URL.Query().Get("q")
			_, _ = w.Write([]byte(`{"total_count":0,"incomplete_results":false,"items":[]}`))
		case strings.Contains(r.URL.Path, "/repos/o/r/commits"):
			repoSince = r.URL.Query().Get("since")
			repoUntil = r.URL.Query().Get("until")
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New("", srv.URL+"/", 50)
	if err != nil {
		t.Fatal(err)
	}

	sq := model.Query{Author: "octocat", Scope: model.ScopeSearch, Since: since, Until: until}
	if _, err := c.Collect(context.Background(), sq); err != nil {
		t.Fatalf("search Collect: %v", err)
	}
	rq := model.Query{Author: "octocat", Scope: model.ScopeRepos, Repos: []string{"o/r"}, Since: since, Until: until}
	if _, err := c.Collect(context.Background(), rq); err != nil {
		t.Fatalf("repos Collect: %v", err)
	}

	wantSince := since.UTC().Format(time.RFC3339)
	wantUntil := until.UTC().Format(time.RFC3339)
	wantQualifier := "author-date:" + wantSince + ".." + wantUntil
	if !strings.Contains(searchQ, wantQualifier) {
		t.Errorf("search q = %q, want it to contain %q (sub-day precision, not whole days)", searchQ, wantQualifier)
	}
	if repoSince != wantSince || repoUntil != wantUntil {
		t.Errorf("repos since/until = %q/%q, want %q/%q", repoSince, repoUntil, wantSince, wantUntil)
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
