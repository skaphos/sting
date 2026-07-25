// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// estimateServer serves a per_page=1 probe whose Link header advertises a last
// page. With a page size of 1 that last page number *is* the commit count,
// which is what makes the estimate arithmetic rather than guesswork.
func estimateServer(t *testing.T, commitCount int, probeCalls *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4929")

		switch {
		case strings.Contains(r.URL.Path, "/rate_limit"):
			_, _ = w.Write([]byte(`{"resources":{"core":{"limit":5000,"remaining":4929,"reset":1784000000}}}`))

		case strings.HasSuffix(r.URL.Path, "/commits"):
			*probeCalls++
			if got := r.URL.Query().Get("per_page"); got != "1" {
				t.Errorf("probe per_page = %q, want 1: a bigger page transfers commits for no extra information", got)
			}
			if commitCount > 1 {
				w.Header().Set("Link", fmt.Sprintf(
					`<https://api.github.com/x?page=2&per_page=1>; rel="next", `+
						`<https://api.github.com/x?page=%d&per_page=1>; rel="last"`, commitCount))
			}
			if commitCount == 0 {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte("[" + windowCommit("h1", []string{"b1"},
				"2026-07-20T10:00:00Z", "2026-07-20T10:00:00Z") + "]"))

		default:
			t.Errorf("estimate must not touch %q — it gathers no evidence", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestEstimateActivityExactCount: the Link header's last page equals the exact
// number of commits in the window.
func TestEstimateActivityExactCount(t *testing.T) {
	var probes int
	srv := estimateServer(t, 250, &probes)
	c, err := New("test-token", srv.URL+"/", 100, WithRequestBudget(500))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	q := activityQuery("skaphos/sting")
	q.Ref = "main" // skip the default-branch lookup so the arithmetic is isolated
	report, err := c.EstimateActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("EstimateActivity: %v", err)
	}

	// 1 probe + ceil(250/100)=3 listing pages + 1 comparison = 5.
	if report.Estimated != 5 {
		t.Errorf("Estimated = %d, want 5 (1 probe + 3 pages + 1 comparison)", report.Estimated)
	}
	if probes != 1 {
		t.Errorf("probe requests = %d, want exactly 1", probes)
	}
	if report.QuotaRemaining != 4929 || report.QuotaLimit != 5000 {
		t.Errorf("quota = %d of %d, want 4929 of 5000", report.QuotaRemaining, report.QuotaLimit)
	}
}

// TestEstimateActivityCountsItsOwnProbe keeps the accounting honest: the
// estimate costs one request and must say so rather than hide it.
func TestEstimateActivityCountsItsOwnProbe(t *testing.T) {
	var probes int
	srv := estimateServer(t, 10, &probes)

	c, err := New("test-token", srv.URL+"/", 100, WithRequestBudget(500))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	q := activityQuery("skaphos/sting")
	q.Ref = "main"

	report, err := c.EstimateActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("EstimateActivity: %v", err)
	}
	if report.Consumed != 1 {
		t.Errorf("Consumed = %d, want 1: the probe is real spend and must be reported", report.Consumed)
	}
}

// TestEstimateActivitySinglePage covers the absent-Link case: a window that
// fits on one page carries no rel="last", so the count comes from the payload.
func TestEstimateActivitySinglePage(t *testing.T) {
	var probes int
	srv := estimateServer(t, 1, &probes)
	c := newTestClient(t, srv.URL, 100)

	q := activityQuery("skaphos/sting")
	q.Ref = "main"
	report, err := c.EstimateActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("EstimateActivity: %v", err)
	}
	// 1 probe + 1 listing page + 1 comparison = 3.
	if report.Estimated != 3 {
		t.Errorf("Estimated = %d, want 3 for a single-commit window", report.Estimated)
	}
}

func TestEstimateActivityEmptyWindow(t *testing.T) {
	var probes int
	srv := estimateServer(t, 0, &probes)
	c := newTestClient(t, srv.URL, 100)

	q := activityQuery("skaphos/sting")
	q.Ref = "main"
	report, err := c.EstimateActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("EstimateActivity: %v", err)
	}
	// Nothing to list and nothing to compare: only the probe.
	if report.Estimated != 1 {
		t.Errorf("Estimated = %d, want 1 (the probe alone) for an empty window", report.Estimated)
	}
}

func TestEstimateActivityIncludesEnrichmentAndRefLookup(t *testing.T) {
	var probes int
	srv := estimateServer(t, 50, &probes)
	c := newTestClient(t, srv.URL, 100)

	q := activityQuery("skaphos/sting")
	q.EnrichCommits = 5 // Ref left empty, so a default-branch lookup is needed
	report, err := c.EstimateActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("EstimateActivity: %v", err)
	}
	// 1 probe + 1 ref lookup + 1 page + 1 comparison + 5 enrichment = 9.
	if report.Estimated != 9 {
		t.Errorf("Estimated = %d, want 9 (probe + ref + page + compare + 5 enrichment)", report.Estimated)
	}
}

func TestEstimateActivityEnrichmentClippedToCommitCount(t *testing.T) {
	var probes int
	srv := estimateServer(t, 3, &probes)
	c := newTestClient(t, srv.URL, 100)

	q := activityQuery("skaphos/sting")
	q.Ref = "main"
	q.EnrichCommits = 100 // far more than the window holds
	report, err := c.EstimateActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("EstimateActivity: %v", err)
	}
	// Enrichment cannot exceed the commits that exist: 1 + 1 + 1 + 3 = 6.
	if report.Estimated != 6 {
		t.Errorf("Estimated = %d, want 6: enrichment must be clipped to the commit count", report.Estimated)
	}
}

func TestEstimateActivityInvalidRepo(t *testing.T) {
	c := newTestClient(t, "http://example.invalid", 100)
	if _, err := c.EstimateActivity(context.Background(), activityQuery("bad")); err == nil {
		t.Error("expected an error for a malformed repo")
	}
}

// TestEstimateAccuracy is SC-005: the estimate must land within 20% of what the
// work actually costs.
//
// The estimate predicts *total* spend for the estimate-then-run sequence a user
// actually performs, which is why the probe is one of its terms. So the figure
// it is checked against is the probe plus the real run, not the run alone.
// Because every term is exact arithmetic, the two should match precisely.
func TestEstimateAccuracy(t *testing.T) {
	const (
		commits = 250
		perPage = 100
	)
	q := activityQuery("skaphos/sting")
	q.Ref = "main"

	trEstimate := newCountingTransport(commits, perPage)
	report, err := clientWithTransport(t, trEstimate, perPage).EstimateActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("EstimateActivity: %v", err)
	}
	probeCost, _ := trEstimate.counts()

	trRun := newCountingTransport(commits, perPage)
	if _, err := clientWithTransport(t, trRun, perPage).CollectActivity(context.Background(), q); err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}
	runCost, byKind := trRun.counts()
	// The pre-flight quota check does not count against GitHub's core quota, so
	// it is not part of what the estimate predicts.
	runCost -= byKind["rate-limit"]

	actual := probeCost + runCost
	drift := float64(report.Estimated-actual) / float64(actual)
	if drift < -0.2 || drift > 0.2 {
		t.Errorf("estimate %d vs actual %d = %.0f%% drift, want within 20%% (run breakdown %v)",
			report.Estimated, actual, drift*100, byKind)
	}
	t.Logf("estimated %d; actual %d (probe %d + run %d) %v",
		report.Estimated, actual, probeCost, runCost, byKind)
}
