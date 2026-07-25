// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-github/v84/github"
	"github.com/skaphos/sting/internal/apibudget"
)

// countingTransport records every request by category. Counting at the
// transport is what makes it possible to assert a *negative* — that no
// per-commit detail request was issued — which an httptest handler cannot prove
// on its own, because it never sees a request that failed before reaching it.
type countingTransport struct {
	mu       sync.Mutex
	total    int
	byKind   map[string]int
	perPage  int
	commits  int
	pageSeen map[int]bool

	// exhaustedQuota makes the rate-limit endpoint report a spent core quota,
	// so the pre-flight check can be exercised.
	exhaustedQuota bool
}

func newCountingTransport(commits, perPage int) *countingTransport {
	return &countingTransport{
		byKind:   map[string]int{},
		perPage:  perPage,
		commits:  commits,
		pageSeen: map[int]bool{},
	}
}

func (c *countingTransport) kindOf(path string) string {
	switch {
	case strings.Contains(path, "/compare/"):
		return "compare"
	case strings.HasSuffix(path, "/commits"):
		return "list-commits"
	case strings.Contains(path, "/commits/"):
		return "commit-detail"
	case strings.Contains(path, "/rate_limit"):
		return "rate-limit"
	default:
		return "repo"
	}
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	kind := c.kindOf(req.URL.Path)

	c.mu.Lock()
	c.total++
	c.byKind[kind]++
	c.mu.Unlock()

	header := http.Header{}
	header.Set("Content-Type", "application/json")
	header.Set("X-RateLimit-Limit", "5000")
	header.Set("X-RateLimit-Remaining", "4900")

	var body string
	switch kind {
	case "repo":
		body = `{"default_branch":"main"}`
	case "list-commits":
		page := 1
		if p := req.URL.Query().Get("page"); p != "" {
			_, _ = fmt.Sscanf(p, "%d", &page)
		}
		// Honor the request's own page size: the estimate probe asks for
		// per_page=1 precisely so that the last page number equals the commit
		// count, and a fixture that ignored it would make the estimate look
		// wrong when it is right.
		perPage := c.perPage
		if p := req.URL.Query().Get("per_page"); p != "" {
			_, _ = fmt.Sscanf(p, "%d", &perPage)
		}
		if perPage < 1 {
			perPage = c.perPage
		}
		totalPages := (c.commits + perPage - 1) / perPage
		start := (page - 1) * perPage
		end := min(start+perPage, c.commits)

		var items []string
		for i := start; i < end; i++ {
			// SHAs descend so the last commit overall is the earliest.
			sha := fmt.Sprintf("sha%04d", c.commits-i)
			parent := fmt.Sprintf("sha%04d", c.commits-i-1)
			items = append(items, windowCommit(sha, []string{parent},
				"2026-07-20T10:00:00Z", "2026-07-20T10:00:00Z"))
		}
		body = "[" + strings.Join(items, ",") + "]"
		if page < totalPages {
			header.Set("Link", fmt.Sprintf(
				`<https://api.github.com/x?page=%d>; rel="next", `+
					`<https://api.github.com/x?page=%d>; rel="last"`, page+1, totalPages))
		}
	case "compare":
		body = compareOK("ahead", fileEntry("a.go", "modified", 1, 1))
	case "rate-limit":
		remaining := 4900
		if c.exhaustedQuota {
			remaining = 0
		}
		body = fmt.Sprintf(`{"resources":{"core":{"limit":5000,"remaining":%d,"reset":1784000000}}}`, remaining)
	case "commit-detail":
		// Return the same path the comparison reports, so enriched commits can
		// actually produce an observed correlation. A fixture with an empty
		// file list would make the attribution invariant vacuously true.
		body = fmt.Sprintf(`{"sha":%q,"files":[{"filename":"a.go","status":"modified","additions":1,"deletions":1}]}`,
			pathBase(req.URL.Path))
	default:
		body = `{}`
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

func (c *countingTransport) counts() (int, map[string]int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]int{}
	for k, v := range c.byKind {
		out[k] = v
	}
	return c.total, out
}

// clientWithTransport builds a Client whose HTTP transport is tr.
//
// It constructs the go-github client directly rather than going through New and
// reassigning: github.Client.Client() returns a *copy* of the HTTP client, so
// mutating that copy would leave the real transport untouched and every request
// would escape to the live API — which the no-network rule forbids and which
// would make the request counts meaningless anyway.
func clientWithTransport(t *testing.T, tr http.RoundTripper, perPage int) *Client {
	t.Helper()
	return &Client{
		gh:          github.NewClient(&http.Client{Transport: tr}),
		perPage:     perPage,
		concurrency: defaultConcurrency,
	}
}

// budgetedClientWithTransport layers the budget transport directly over tr, so
// both observe exactly the same requests.
func budgetedClientWithTransport(t *testing.T, tr http.RoundTripper, perPage, ceiling int) *Client {
	t.Helper()
	budget := apibudget.NewTransport(tr, ceiling)
	return &Client{
		gh:          github.NewClient(&http.Client{Transport: budget}),
		perPage:     perPage,
		concurrency: defaultConcurrency,
		budget:      budget,
	}
}

// TestActivityRequestCount is the executable form of the feature's central
// promise (SC-001, SC-002, FR-006, FR-007).
//
// A 250-commit window must cost a number of requests bounded by commit *pages*,
// not commit count. The same window through the existing per-commit path costs
// ~251. A regression that reintroduces per-commit fetching fails here rather
// than quietly costing users their quota.
func TestActivityRequestCount(t *testing.T) {
	const (
		commits = 250
		perPage = 100
		budget  = 15
	)
	tr := newCountingTransport(commits, perPage)
	c := clientWithTransport(t, tr, perPage)

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}
	if res.Count != commits {
		t.Fatalf("Count = %d, want %d", res.Count, commits)
	}

	total, byKind := tr.counts()
	if total > budget {
		t.Errorf("issued %d requests for a %d-commit window, want <= %d (breakdown: %v)",
			total, commits, budget, byKind)
	}

	// The specific promise: zero per-commit detail requests when enrichment was
	// not asked for. This is the line between a cheap digest and the expensive
	// path the feature exists to replace.
	if got := byKind["commit-detail"]; got != 0 {
		t.Errorf("issued %d per-commit detail requests, want 0 without --enrich-commits", got)
	}
	// Exactly one comparison, independent of commit count.
	if got := byKind["compare"]; got != 1 {
		t.Errorf("issued %d comparison requests, want exactly 1", got)
	}
	// Listing costs one request per page and no more.
	wantPages := (commits + perPage - 1) / perPage
	if got := byKind["list-commits"]; got != wantPages {
		t.Errorf("issued %d list-commits requests, want %d (one per page)", got, wantPages)
	}

	t.Logf("250-commit window cost %d requests: %v", total, byKind)
}

// TestActivityRequestCountScalesWithPagesNotCommits pins the shape of the cost
// curve: ten times the commits must not mean ten times the requests.
func TestActivityRequestCountScalesWithPagesNotCommits(t *testing.T) {
	const perPage = 100

	costFor := func(commits int) int {
		tr := newCountingTransport(commits, perPage)
		c := clientWithTransport(t, tr, perPage)
		if _, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting")); err != nil {
			t.Fatalf("CollectActivity(%d commits): %v", commits, err)
		}
		total, _ := tr.counts()
		return total
	}

	small := costFor(10)
	large := costFor(1000)

	// 100x the commits, but only ~10 more pages: the difference must track
	// pages, not commits.
	if large-small > 12 {
		t.Errorf("cost grew by %d requests going from 10 to 1000 commits (%d -> %d); "+
			"it should track pages, not commit count", large-small, small, large)
	}
	t.Logf("10 commits: %d requests; 1000 commits: %d requests", small, large)
}

// TestActivityCostReportMatchesActualRequests keeps the accounting honest: the
// number the tool reports must be the number it actually spent, or the budget
// is unauditable.
func TestActivityCostReportMatchesActualRequests(t *testing.T) {
	const perPage = 100
	tr := newCountingTransport(150, perPage)

	c := budgetedClientWithTransport(t, tr, perPage, 50)

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}

	total, byKind := tr.counts()
	// The pre-flight rate-limit check is deliberately unmetered: GitHub does not
	// charge core quota for it, so charging it against the caller's ceiling
	// would both overstate the cost and make a small explicit ceiling unusable.
	metered := total - byKind["rate-limit"]
	if res.Cost.Consumed != metered {
		t.Errorf("reported Consumed = %d but %d metered requests were issued (%v)",
			res.Cost.Consumed, metered, byKind)
	}
	if byKind["rate-limit"] == 0 {
		t.Error("no pre-flight quota check was issued, so the exemption was not exercised")
	}
	if res.Cost.Ceiling != 50 {
		t.Errorf("Ceiling = %d, want 50", res.Cost.Ceiling)
	}
	if res.Cost.QuotaLimit != 5000 {
		t.Errorf("QuotaLimit = %d, want 5000 captured from the response headers", res.Cost.QuotaLimit)
	}
}
