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
	"time"

	"github.com/google/go-github/v82/github"
	"github.com/skaphos/sting/internal/apibudget"
	"github.com/skaphos/sting/model"
)

// queryTransport serves the author-scoped discovery paths (search, repo
// listing, org listing, per-commit detail) and counts what it is asked for.
type queryTransport struct {
	mu      sync.Mutex
	byKind  map[string]int
	commits int
	perPage int
}

func newQueryTransport(commits, perPage int) *queryTransport {
	return &queryTransport{byKind: map[string]int{}, commits: commits, perPage: perPage}
}

func (q *queryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p := req.URL.Path
	var kind string
	switch {
	case strings.Contains(p, "search/commits"):
		kind = "search"
	case strings.Contains(p, "/orgs/"):
		kind = "org-repos"
	case strings.Contains(p, "/commits/"):
		kind = "commit-detail"
	case strings.HasSuffix(p, "/commits"):
		kind = "list-commits"
	default:
		kind = "other"
	}

	q.mu.Lock()
	q.byKind[kind]++
	q.mu.Unlock()

	header := http.Header{}
	header.Set("Content-Type", "application/json")
	header.Set("X-RateLimit-Limit", "5000")
	header.Set("X-RateLimit-Remaining", "4900")

	page := 1
	if v := req.URL.Query().Get("page"); v != "" {
		_, _ = fmt.Sscanf(v, "%d", &page)
	}
	totalPages := (q.commits + q.perPage - 1) / q.perPage

	var body string
	switch kind {
	case "search":
		var items []string
		start := (page - 1) * q.perPage
		end := min(start+q.perPage, q.commits)
		for i := start; i < end; i++ {
			items = append(items, fmt.Sprintf(
				`{"sha":"sha%04d","repository":{"full_name":"skaphos/sting"},`+
					`"commit":{"message":"m","author":{"name":"Octo","email":"octo@example.com","date":"2026-07-20T10:00:00Z"}}}`, i))
		}
		body = fmt.Sprintf(`{"total_count":%d,"incomplete_results":false,"items":[%s]}`,
			q.commits, strings.Join(items, ","))
		if page < totalPages {
			header.Set("Link", fmt.Sprintf(`<https://api.github.com/x?page=%d>; rel="next"`, page+1))
		}
	case "org-repos":
		body = `[{"full_name":"skaphos/sting"}]`
	case "list-commits":
		var items []string
		start := (page - 1) * q.perPage
		end := min(start+q.perPage, q.commits)
		for i := start; i < end; i++ {
			items = append(items, fmt.Sprintf(
				`{"sha":"sha%04d","commit":{"author":{"name":"Octo","email":"octo@example.com","date":"2026-07-20T10:00:00Z"}}}`, i))
		}
		body = "[" + strings.Join(items, ",") + "]"
		if page < totalPages {
			header.Set("Link", fmt.Sprintf(`<https://api.github.com/x?page=%d>; rel="next"`, page+1))
		}
	case "commit-detail":
		body = `{"sha":"x","stats":{"additions":1,"deletions":1,"total":2},"files":[]}`
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

func (q *queryTransport) counts() map[string]int {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := map[string]int{}
	for k, v := range q.byKind {
		out[k] = v
	}
	return out
}

func budgetedQueryClient(tr http.RoundTripper, perPage, ceiling int) *Client {
	budget := apibudget.NewTransport(tr, ceiling)
	return &Client{
		gh:          github.NewClient(&http.Client{Transport: budget}),
		perPage:     perPage,
		concurrency: defaultConcurrency,
		budget:      budget,
	}
}

func authorQuery(scope model.Scope) model.Query {
	q := model.Query{
		Author: "octocat",
		Scope:  scope,
		Since:  time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC),
		Until:  time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC),
	}
	switch scope {
	case model.ScopeRepos:
		q.Repos = []string{"skaphos/sting"}
	case model.ScopeOrg:
		q.Org = "skaphos"
	}
	return q
}

// TestQueryCeilingStopsEachScope covers User Story 4: the ceiling applies to
// every author-scoped discovery path, and reaching it yields the results
// gathered so far rather than an abort.
func TestQueryCeilingStopsEachScope(t *testing.T) {
	for _, scope := range []model.Scope{model.ScopeSearch, model.ScopeRepos, model.ScopeOrg} {
		t.Run(string(scope), func(t *testing.T) {
			const (
				commits = 1000
				perPage = 10
				ceiling = 3
			)
			tr := newQueryTransport(commits, perPage)
			c := budgetedQueryClient(tr, perPage, ceiling)

			res, err := c.Collect(context.Background(), authorQuery(scope))

			// The ceiling surfaces as an error on this path (Result has no
			// disclosure channel), but the gathered commits must survive it.
			if err == nil {
				t.Logf("scope %s completed within the ceiling", scope)
			}
			if res.Count == 0 && err != nil {
				t.Errorf("scope %s: ceiling discarded every gathered commit", scope)
			}

			consumed := c.Cost().Consumed
			if consumed > ceiling {
				t.Errorf("scope %s: consumed %d requests, exceeding the ceiling of %d",
					scope, consumed, ceiling)
			}
			t.Logf("scope %s: %d commits, %d requests, %v", scope, res.Count, consumed, tr.counts())
		})
	}
}

// TestDefaultCeilingDoesNotClipDefaultConfiguration is the guarantee behind
// research R5's choice of 500: the default ceiling must not break a query that
// works today.
//
// The worst-case default-configuration query is max_commits = 100 with
// enrichment on, which costs roughly 110 requests — comfortably inside 500.
func TestDefaultCeilingDoesNotClipDefaultConfiguration(t *testing.T) {
	const (
		commits = 100 // config.DefaultMaxCommits
		perPage = 100
	)
	tr := newQueryTransport(commits, perPage)
	c := budgetedQueryClient(tr, perPage, model.DefaultMaxRequests)

	q := authorQuery(model.ScopeSearch)
	q.MaxCommits = commits
	q.IncludeStats = true // the expensive default-configuration shape
	q.IncludeFiles = true

	res, err := c.Collect(context.Background(), q)
	if err != nil {
		t.Fatalf("the default ceiling broke a default-configuration query: %v", err)
	}
	if res.Count != commits {
		t.Errorf("Count = %d, want %d", res.Count, commits)
	}

	consumed := c.Cost().Consumed
	if consumed >= model.DefaultMaxRequests {
		t.Errorf("consumed %d requests against a default ceiling of %d — no headroom",
			consumed, model.DefaultMaxRequests)
	}
	// ~110 requests expected: 1 search page + 100 detail fetches.
	t.Logf("default-configuration query cost %d of %d (%v)",
		consumed, model.DefaultMaxRequests, tr.counts())
}

// TestQueryUncappedCeilingIsStillAccounted: 0 means uncapped, not unaccounted.
func TestQueryUncappedCeilingIsStillAccounted(t *testing.T) {
	tr := newQueryTransport(50, 100)
	c := budgetedQueryClient(tr, 100, 0)

	if _, err := c.Collect(context.Background(), authorQuery(model.ScopeSearch)); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := c.Cost().Consumed; got == 0 {
		t.Error("Consumed = 0 on an uncapped run; accounting must stay on")
	}
	if got := c.Cost().Ceiling; got != 0 {
		t.Errorf("Ceiling = %d, want 0", got)
	}
}

// TestQueryCeilingCoversEnrichment confirms the transport-level budget reaches
// the per-commit fan-out too, which is where the original rate-limit pain was
// actually felt.
func TestQueryCeilingCoversEnrichment(t *testing.T) {
	const (
		commits = 200
		perPage = 100
		ceiling = 20
	)
	tr := newQueryTransport(commits, perPage)
	c := budgetedQueryClient(tr, perPage, ceiling)

	q := authorQuery(model.ScopeSearch)
	q.IncludeStats = true

	res, _ := c.Collect(context.Background(), q)

	consumed := c.Cost().Consumed
	if consumed > ceiling {
		t.Errorf("consumed %d, exceeding the ceiling of %d during enrichment", consumed, ceiling)
	}
	counts := tr.counts()
	if counts["commit-detail"] > ceiling {
		t.Errorf("issued %d detail requests past a ceiling of %d", counts["commit-detail"], ceiling)
	}
	// The commits themselves survive even though enrichment was cut short.
	if res.Count == 0 {
		t.Error("the ceiling discarded every gathered commit")
	}
	t.Logf("enrichment under a ceiling of %d: %d commits, %d requests, %v",
		ceiling, res.Count, consumed, counts)
}
