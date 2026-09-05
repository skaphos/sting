// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

// Records last updated before the window cannot hold an in-window occurrence,
// so the default any basis must not spend a lifecycle request proving that.
// Regression: the backlog previously exhausted the budget and hid the one PR
// merged inside the window, returning zero matches for the motivating query.
func TestPRActivitySkipsHistoryForRecordsBeforeWindow(t *testing.T) {
	const total = 600
	events := 0
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pulls") {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page == 0 {
				page = 1
			}
			items := []any{}
			for i := 0; i < 100; i++ {
				n := (page-1)*100 + i + 1
				p := prPayload(n)
				p["created_at"] = fmt.Sprintf("2024-01-01T00:%02d:00Z", 59-(n%60))
				p["updated_at"] = "2024-02-01T00:00:00Z"
				if n == total { // oldest by creation, merged inside the window
					p["created_at"] = "2023-01-01T00:00:00Z"
					p["state"] = "closed"
					p["merged_at"] = "2026-08-20T12:00:00Z"
					p["closed_at"] = "2026-08-20T12:00:00Z"
					p["updated_at"] = "2026-08-20T12:00:00Z"
				}
				items = append(items, p)
			}
			if page*100 < total {
				w.Header().Set("Link", fmt.Sprintf("<http://%s/repos/acme/api/pulls?page=%d>; rel=\"next\"", r.Host, page+1))
			}
			prJSON(t, w, items)
			return
		}
		events++
		prJSON(t, w, []any{prEvent(1, "merged", "2026-08-20T12:00:00Z"), prEvent(2, "closed", "2026-08-20T12:00:00Z")})
	})
	c, err := New("", f.server.URL+"/", 100)
	if err != nil {
		t.Fatal(err)
	}
	q, err := config.Default().ResolvePRs(config.PRRequest{Scope: "repos", Repos: []string{"acme/api"}}, prClock)
	if err != nil {
		t.Fatal(err)
	}
	if q.TimeBasis != model.PRTimeAny {
		t.Fatalf("default basis: %s", q.TimeBasis)
	}
	got, err := c.CollectPRs(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 1 || got.PRs[0].PR.Number != total {
		t.Fatalf("lost the PR merged in the window: count=%d %+v", got.Count, got.PRs)
	}
	if got.Truncated || !got.Coverage.DiscoveryComplete || !got.Coverage.EvidenceComplete {
		t.Fatalf("complete collection reported as partial: %+v", got.Coverage)
	}
	// Six listing pages plus one lifecycle read for the single live candidate.
	if events != 1 || got.Cost.HistoryRequests != 1 || got.Cost.Consumed != 7 {
		t.Fatalf("stale records cost lifecycle requests: events=%d cost=%+v", events, got.Cost)
	}
	// The bound rests on an inference about update timestamps, so the report
	// states that it was applied and to how many records (research R2).
	bounded := 0
	for _, d := range got.Disclosures {
		if d.Kind == "history-bounded" {
			bounded++
			if !strings.Contains(d.Reason, fmt.Sprintf("%d record(s)", total-1)) {
				t.Errorf("history bound not quantified: %q", d.Reason)
			}
		}
	}
	if bounded != 1 {
		t.Fatalf("history bound applied without disclosing it: %+v", got.Disclosures)
	}
}

// A run that skips nothing must not claim a bound it never applied.
func TestPRActivityOmitsHistoryBoundWhenNothingSkipped(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pulls") {
			p := prPayload(1)
			p["created_at"] = "2026-08-24T13:00:00Z"
			p["updated_at"] = "2026-08-24T13:00:00Z"
			prJSON(t, w, []any{p})
			return
		}
		prJSON(t, w, []any{})
	})
	c, err := New("", f.server.URL+"/", 100)
	if err != nil {
		t.Fatal(err)
	}
	q, err := config.Default().ResolvePRs(config.PRRequest{Scope: "repos", Repos: []string{"acme/api"}}, prClock)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.CollectPRs(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range got.Disclosures {
		if d.Kind == "history-bounded" {
			t.Fatalf("disclosed a bound that was never applied: %q", d.Reason)
		}
	}
	if got.Count != 1 || got.Truncated {
		t.Fatalf("in-window record lost: count=%d truncated=%v", got.Count, got.Truncated)
	}
}

// A record updated after the window keeps its history read, because a closure
// inside the window can be replaced by a later reopening (FR-023).
func TestPRActivityKeepsHistoryForRecordsUpdatedAfterWindow(t *testing.T) {
	events := 0
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pulls") {
			p := prPayload(1)
			p["created_at"] = "2026-01-01T00:00:00Z"
			p["updated_at"] = "2026-08-25T12:00:00Z" // after the window closure
			prJSON(t, w, []any{p})
			return
		}
		events++
		prJSON(t, w, []any{
			prEvent(1, "closed", "2026-08-20T12:00:00Z"),
			prEvent(2, "reopened", "2026-08-24T12:00:00Z"),
		})
	})
	c, err := New("", f.server.URL+"/", 100)
	if err != nil {
		t.Fatal(err)
	}
	q, err := config.Default().ResolvePRs(config.PRRequest{Scope: "repos", Repos: []string{"acme/api"}}, prClock)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.CollectPRs(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if events != 1 || got.Count != 1 {
		t.Fatalf("dropped recoverable closure: events=%d count=%d", events, got.Count)
	}
	if len(got.PRs[0].Matches) != 1 || got.PRs[0].Matches[0].Kind != "closed" {
		t.Fatalf("historical closure not preserved: %+v", got.PRs[0].Matches)
	}
}

// Disclosures carry recovery guidance in next_action, and optional-metadata
// gaps collapse into one entry per field set instead of one entry per PR.
func TestPRDisclosureNextActionAndAggregatedMetadata(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		items := []any{}
		for n := 1; n <= 3; n++ {
			items = append(items, prPayload(n))
		}
		prJSON(t, w, items)
	})
	c, err := New("", f.server.URL+"/", 100)
	if err != nil {
		t.Fatal(err)
	}
	one := 1
	q, err := config.Default().ResolvePRs(config.PRRequest{Scope: "repos", Repos: []string{"acme/api"}, TimeBasis: "created", MaxPRs: &one}, prClock)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.CollectPRs(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	metadata, roles, limits := 0, 0, 0
	for _, d := range got.Disclosures {
		switch d.Kind {
		case "metadata-unavailable":
			metadata++
			if !strings.Contains(d.Reason, "3 PR records") {
				t.Errorf("metadata disclosure lost its count: %q", d.Reason)
			}
		case "role":
			roles++
		case "result-limit":
			limits++
			if d.NextAction == "" {
				t.Error("result-limit disclosure has no next action")
			}
		}
		if strings.Contains(d.Reason, "increase max_requests") {
			t.Errorf("guidance left in reason prose: %q", d.Reason)
		}
	}
	if metadata != 1 || roles != 1 || limits == 0 {
		t.Fatalf("disclosure shape: metadata=%d role=%d limit=%d", metadata, roles, limits)
	}
}

// The role disclosure distinguishes an author filter from an unfiltered query.
func TestPRRoleDisclosureDistinguishesAuthorFilter(t *testing.T) {
	for _, author := range []string{"", "octocat"} {
		f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) { prJSON(t, w, []any{}) })
		c, err := New("", f.server.URL+"/", 100)
		if err != nil {
			t.Fatal(err)
		}
		q, err := config.Default().ResolvePRs(config.PRRequest{Scope: "repos", Repos: []string{"acme/api"}, Author: author, TimeBasis: "created"}, prClock)
		if err != nil {
			t.Fatal(err)
		}
		got, err := c.CollectPRs(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		want := "No author filter"
		if author != "" {
			want = "role=author"
		}
		found := false
		for _, d := range got.Disclosures {
			if d.Kind == "role" && strings.Contains(d.Reason, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("author=%q missing role disclosure %q: %+v", author, want, got.Disclosures)
		}
	}
}
