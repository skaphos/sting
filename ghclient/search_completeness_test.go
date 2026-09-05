// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/skaphos/sting/model"
)

func TestSearchIncompleteOnAnyPageRetainsEvidence(t *testing.T) {
	for _, tc := range []struct {
		name                                         string
		incompletePage, maxCommits, failPage, budget int
		empty                                        bool
		wantCount                                    int
		wantErr                                      bool
	}{
		{name: "first page", incompletePage: 1, wantCount: 2},
		{name: "later page", incompletePage: 2, wantCount: 2},
		{name: "empty", incompletePage: 1, empty: true},
		{name: "user cap", incompletePage: 1, maxCommits: 1, wantCount: 1},
		{name: "later error", incompletePage: 1, failPage: 2, wantCount: 1, wantErr: true},
		{name: "later budget stop", incompletePage: 1, budget: 1, wantCount: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				if page == 0 {
					page = 1
				}
				if page == tc.failPage {
					http.Error(w, "outage", 500)
					return
				}
				if !tc.empty && page == 1 {
					w.Header().Set("Link", fmt.Sprintf("<http://%s%s?page=2>; rel=\"next\"", r.Host, r.URL.Path))
				}
				items := fmt.Sprintf("[{\"sha\":\"page%d\"}]", page)
				if tc.empty {
					items = "[]"
				}
				fmt.Fprintf(w, "{\"total_count\":2,\"incomplete_results\":%t,\"items\":%s}", page == tc.incompletePage, items)
			}))
			defer srv.Close()
			c, err := New("fixture", srv.URL+"/", 100, WithRequestBudget(tc.budget))
			if err != nil {
				t.Fatal(err)
			}
			res, err := c.Collect(context.Background(), model.Query{Author: "alice", Scope: model.ScopeSearch, MaxCommits: tc.maxCommits})
			if (err != nil) != tc.wantErr || !res.Truncated || res.Count != tc.wantCount {
				t.Fatalf("lost evidence or bound: %+v err=%v", res, err)
			}
			found := 0
			for _, d := range res.Disclosures {
				if d.Kind == model.DisclosureSearchIncomplete {
					found++
					if d.Reason == "" || d.NextAction == "" {
						t.Fatal("unexplained incomplete search")
					}
				}
			}
			if found != 1 {
				t.Fatalf("want one incomplete disclosure: %+v", res.Disclosures)
			}
			if tc.budget > 0 && !hasDisclosure(res.Disclosures, model.DisclosureBudgetBounded) {
				t.Fatalf("lost budget disclosure: %+v", res.Disclosures)
			}
		})
	}
}

func TestSearchThousandResultBoundary(t *testing.T) {
	for _, total := range []int{999, 1000, 1001} {
		t.Run(strconv.Itoa(total), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				if page == 0 {
					page = 1
				}
				if page > 10 {
					t.Error("requested beyond search limit")
				}
				start := (page - 1) * 100
				count := min(100, total-start)
				if start+count < total {
					w.Header().Set("Link", fmt.Sprintf("<http://%s%s?page=%d>; rel=\"next\"", r.Host, r.URL.Path, page+1))
				}
				var items []string
				for i := range count {
					items = append(items, fmt.Sprintf("{\"sha\":\"sha%d\"}", start+i))
				}
				fmt.Fprintf(w, "{\"total_count\":%d,\"items\":[%s]}", total, strings.Join(items, ","))
			}))
			defer srv.Close()
			res, err := newTestClient(t, srv.URL, 100).Collect(context.Background(), model.Query{Author: "alice", Scope: model.ScopeSearch})
			if err != nil || res.Count != min(total, 1000) || calls != 10 || res.Truncated != (total > 1000) {
				t.Fatalf("incorrect boundary: count=%d calls=%d truncated=%v err=%v", res.Count, calls, res.Truncated, err)
			}
			if hasDisclosure(res.Disclosures, model.DisclosureSearchCapped) != (total > 1000) {
				t.Fatalf("incorrect cap disclosure: %+v", res.Disclosures)
			}
		})
	}
}
