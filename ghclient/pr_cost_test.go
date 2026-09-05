// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/skaphos/sting/config"
)

func TestPRResultCapEvidence(t *testing.T) {
	for _, more := range []bool{false, true} {
		t.Run(fmt.Sprint(more), func(t *testing.T) {
			f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
				items := []any{prPayload(1)}
				if more {
					items = append(items, prPayload(2))
				}
				prJSON(t, w, items)
			})
			c, e := New("", f.server.URL+"/", 100)
			if e != nil {
				t.Fatal(e)
			}
			one := 1
			q, e := config.Default().ResolvePRs(config.PRRequest{Scope: "repos", Repos: []string{"acme/api"}, TimeBasis: "created", MaxPRs: &one}, prClock)
			if e != nil {
				t.Fatal(e)
			}
			got, e := c.CollectPRs(context.Background(), q)
			if e != nil {
				t.Fatal(e)
			}
			if got.Count != 1 || got.Truncated != more || got.Cost.Consumed != 1 {
				t.Fatalf("cap completion: %+v", got)
			}
		})
	}
}
func TestPRHistoryFailureAndUnconfirmedFilters(t *testing.T) {
	for _, status := range []int{200, 403, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/search/issues" {
					p := prPayload(1)
					p["state"] = "closed"
					p["closed_at"] = "2026-08-24T13:00:00Z"
					p["repository_url"] = "https://api.github.com/repos/acme/api"
					p["pull_request"] = map[string]any{}
					prJSON(t, w, map[string]any{"total_count": 1, "items": []any{p}})
					return
				}
				if status != 200 {
					w.WriteHeader(status)
					prJSON(t, w, map[string]any{"message": "failure"})
					return
				}
				prJSON(t, w, []any{prEvent(1, "closed", "2026-08-24T13:00:00Z")})
			})
			c, e := New("", f.server.URL+"/", 100)
			if e != nil {
				t.Fatal(e)
			}
			q, e := config.Default().ResolvePRs(config.PRRequest{Author: "octocat", State: "closed", TimeBasis: "created"}, prClock)
			if e != nil {
				t.Fatal(e)
			}
			got, e := c.CollectPRs(context.Background(), q)
			want := 1
			if status != 200 {
				want = 0
			}
			if got.Count != want || (e != nil) != (status != 200) || got.Cost.HistoryRequests != 1 {
				t.Fatalf("unknown disposition: %+v %v", got, e)
			}
		})
	}
}

func TestPRPaginationSafetyGuard(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", fmt.Sprintf("<http://%s/api/v3/repos/acme/api/pulls?page=1>; rel=\"next\"", r.Host))
		prJSON(t, w, []any{})
	})
	c, e := New("", f.server.URL+"/", 100)
	if e != nil {
		t.Fatal(e)
	}
	zero := 0
	q, e := config.Default().ResolvePRs(config.PRRequest{Scope: "repos", Repos: []string{"acme/api"}, TimeBasis: "created", MaxRequests: &zero}, prClock)
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.CollectPRs(context.Background(), q)
	if e != nil {
		t.Fatal(e)
	}
	if got.Cost.Consumed != maxPages || !got.Truncated {
		t.Fatalf("unbounded pagination: %+v", got)
	}
}
