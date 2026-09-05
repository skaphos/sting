// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/skaphos/sting/config"
)

func TestPRActivityBudgetPreservesPage(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/issues" {
			t.Fatal("history exceeded budget")
		}
		if !strings.Contains(r.URL.Query().Get("q"), "is:public") {
			t.Error("missing visibility restriction")
		}
		items := []any{}
		for n := 1; n <= 3; n++ {
			p := prPayload(n)
			p["repository_url"] = "https://api.github.com/repos/acme/api"
			p["pull_request"] = map[string]any{}
			items = append(items, p)
		}
		prJSON(t, w, map[string]any{"total_count": 3, "items": items})
	})
	c, err := New("", f.server.URL+"/", 100)
	if err != nil {
		t.Fatal(err)
	}
	one := 1
	q, err := config.Default().ResolvePRs(config.PRRequest{Author: "octocat", MaxRequests: &one}, prClock)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.CollectPRs(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 3 || got.Cost.Consumed != 1 || !got.Truncated {
		t.Fatalf("page evidence lost: %+v", got)
	}
}
func TestPRSearchLimitsAndUpdates(t *testing.T) {
	for _, basis := range []string{"created", "updated", "merged"} {
		t.Run(basis, func(t *testing.T) {
			f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.URL.Query().Get("q"), basis+":") {
					t.Error("missing basis")
				}
				prJSON(t, w, map[string]any{"total_count": 1001, "incomplete_results": true, "items": []any{}})
			})
			c, err := New("", f.server.URL+"/", 100)
			if err != nil {
				t.Fatal(err)
			}
			q, err := config.Default().ResolvePRs(config.PRRequest{Author: "octocat", TimeBasis: basis}, prClock)
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.CollectPRs(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			if got.Count != 0 || !got.Truncated || got.Coverage.DiscoveryComplete {
				t.Fatalf("silent search cap: %+v", got)
			}
		})
	}
}
