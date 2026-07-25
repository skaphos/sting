// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skaphos/sting/internal/apibudget"
	"github.com/skaphos/sting/model"
)

// TestNewThreeArgumentCallSitesStillCompile is the source-compatibility promise
// behind making New variadic: every existing caller passes three arguments and
// must keep working unchanged, with no budget transport installed.
func TestNewThreeArgumentCallSitesStillCompile(t *testing.T) {
	c, err := New("token", "", 50)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.perPage != 50 {
		t.Errorf("perPage = %d, want 50", c.perPage)
	}
	if c.concurrency != defaultConcurrency {
		t.Errorf("concurrency = %d, want %d", c.concurrency, defaultConcurrency)
	}
	if c.budget != nil {
		t.Error("a three-argument New must not install a budget transport")
	}
	if got := c.Cost(); got != (model.CostReport{}) {
		t.Errorf("Cost() = %+v, want a zero report without WithRequestBudget", got)
	}
}

func TestNewPerPageClampingUnaffectedByOptions(t *testing.T) {
	tests := []struct {
		in   int
		want int
	}{{0, 100}, {-5, 100}, {101, 100}, {1, 1}, {100, 100}, {42, 42}}
	for _, tt := range tests {
		c, err := New("token", "", tt.in, WithRequestBudget(10))
		if err != nil {
			t.Fatalf("New(perPage=%d): %v", tt.in, err)
		}
		if c.perPage != tt.want {
			t.Errorf("New(perPage=%d).perPage = %d, want %d", tt.in, c.perPage, tt.want)
		}
	}
}

func TestWithRequestBudgetInstallsTransport(t *testing.T) {
	c, err := New("token", "", 100, WithRequestBudget(25))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.budget == nil {
		t.Fatal("WithRequestBudget did not install a budget transport")
	}
	if got := c.budget.Ceiling(); got != 25 {
		t.Errorf("ceiling = %d, want 25", got)
	}
	if got := c.budgetRemaining(); got != 25 {
		t.Errorf("budgetRemaining() = %d, want 25", got)
	}
}

func TestWithRequestBudgetZeroIsUncappedButAccounted(t *testing.T) {
	c, err := New("token", "", 100, WithRequestBudget(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.budget == nil {
		t.Fatal("WithRequestBudget(0) must still install accounting")
	}
	if got := c.Cost().Ceiling; got != 0 {
		t.Errorf("Cost().Ceiling = %d, want 0", got)
	}
}

func TestWithRequestBudgetNegativeClampedToZero(t *testing.T) {
	c, err := New("token", "", 100, WithRequestBudget(-7))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := c.budget.Ceiling(); got != 0 {
		t.Errorf("ceiling = %d, want 0 for a negative request", got)
	}
}

func TestNilOptionIgnored(t *testing.T) {
	if _, err := New("token", "", 100, nil); err != nil {
		t.Fatalf("New with a nil option: %v", err)
	}
}

// TestBudgetTransportCountsRealRequests proves the transport is actually in the
// request path — including for an authenticated client, where go-github wraps
// the transport with its own auth layer. If the layering were inverted, the
// counter would silently report zero for every authenticated query.
func TestBudgetTransportCountsRealRequests(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if got := r.Header.Get("Authorization"); !strings.Contains(got, "test-token") {
			t.Errorf("Authorization header = %q, want it to carry the token", got)
		}
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4990")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c, err := New("test-token", srv.URL+"/", 100, WithRequestBudget(10))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	q := model.Query{
		Author: "octocat",
		Scope:  model.ScopeRepos,
		Repos:  []string{"skaphos/sting"},
		Since:  time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		Until:  time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
	}
	if _, err := c.Collect(context.Background(), q); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	cost := c.Cost()
	if cost.Consumed == 0 {
		t.Fatal("Cost().Consumed = 0; the budget transport is not in the request path")
	}
	if int64(cost.Consumed) != hits.Load() {
		t.Errorf("Consumed = %d but the server saw %d requests", cost.Consumed, hits.Load())
	}
	if cost.Ceiling != 10 {
		t.Errorf("Ceiling = %d, want 10", cost.Ceiling)
	}
	if cost.QuotaLimit != 5000 || cost.QuotaRemaining != 4990 {
		t.Errorf("quota = %d of %d, want 4990 of 5000 captured from the response headers",
			cost.QuotaRemaining, cost.QuotaLimit)
	}
}

// TestBudgetCeilingStopsRealRequests confirms enforcement reaches the wire: once
// the ceiling is spent the server must stop seeing requests, and the error must
// still be recognizable as a budget stop after go-github has wrapped it.
func TestBudgetCeilingStopsRealRequests(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		// Always advertise another page so pagination would run forever if the
		// ceiling were not enforced.
		w.Header().Set("Link", `<`+"http://example.invalid/x?page=99"+`>; rel="next"`)
		_, _ = w.Write([]byte(`[{"sha":"a","commit":{"author":{"name":"n","date":"2026-07-02T00:00:00Z"}}}]`))
	}))
	defer srv.Close()

	const ceiling = 3
	c, err := New("test-token", srv.URL+"/", 100, WithRequestBudget(ceiling))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	q := model.Query{
		Author: "octocat",
		Scope:  model.ScopeRepos,
		Repos:  []string{"skaphos/sting"},
		Since:  time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		Until:  time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
	}
	_, err = c.Collect(context.Background(), q)
	if err == nil {
		t.Fatal("Collect succeeded past the ceiling")
	}
	if !errors.Is(err, apibudget.ErrBudgetExceeded) {
		t.Errorf("error = %v, want it to unwrap to ErrBudgetExceeded", err)
	}
	if got := hits.Load(); got != ceiling {
		t.Errorf("server saw %d requests, want exactly %d", got, ceiling)
	}
	if got := c.Cost().Consumed; got != ceiling {
		t.Errorf("Consumed = %d, want %d", got, ceiling)
	}
}

// TestEnterpriseBaseURLUnaffectedByBudget guards the wrapper against breaking
// GitHub Enterprise support: the base URL rewriting happens on the go-github
// client, which is constructed after the transport is installed.
func TestEnterpriseBaseURLUnaffectedByBudget(t *testing.T) {
	const base = "https://ghe.example.com/api/v3/"

	plain, err := New("token", base, 100)
	if err != nil {
		t.Fatalf("New without budget: %v", err)
	}
	budgeted, err := New("token", base, 100, WithRequestBudget(50))
	if err != nil {
		t.Fatalf("New with budget: %v", err)
	}

	if plain.gh.BaseURL.String() != budgeted.gh.BaseURL.String() {
		t.Errorf("BaseURL differs with the budget wrapper: %q vs %q",
			plain.gh.BaseURL, budgeted.gh.BaseURL)
	}
	if !strings.Contains(budgeted.gh.BaseURL.String(), "ghe.example.com") {
		t.Errorf("BaseURL = %q, want the enterprise host preserved", budgeted.gh.BaseURL)
	}
}

func TestEnterpriseBaseURLErrorStillReported(t *testing.T) {
	// A control character makes url.Parse fail inside WithEnterpriseURLs.
	if _, err := New("token", "http://\x7f/", 100, WithRequestBudget(1)); err == nil {
		t.Error("New with an unparseable enterprise URL = nil error, want a failure")
	}
}

func TestBudgetRemainingWithoutBudgetIsUnbounded(t *testing.T) {
	c, err := New("token", "", 100)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := c.budgetRemaining(); got <= 1_000_000 {
		t.Errorf("budgetRemaining() = %d, want an effectively unbounded value", got)
	}
}
