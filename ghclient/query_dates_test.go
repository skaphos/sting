// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

func TestQueryDatesPreserveSourceSemantics(t *testing.T) {
	for _, tc := range []struct {
		scope model.Scope
		prs   bool
		basis string
		count int
	}{
		{model.ScopeSearch, false, model.WindowDateBasisAuthor, 1},
		{model.ScopeRepos, false, model.WindowDateBasisCommitter, 1},
		{model.ScopeOrg, false, model.WindowDateBasisCommitter, 1},
		{model.ScopeRepos, true, model.WindowDateBasisMixed, 2},
		{model.ScopeOrg, true, model.WindowDateBasisMixed, 2},
	} {
		t.Run(fmt.Sprintf("%s/prs=%v", tc.scope, tc.prs), func(t *testing.T) {
			rebased := windowCommit("rebased", []string{"base"}, "2026-07-01T00:00:00Z", "2026-07-20T00:00:00Z")
			cherry := windowCommit("cherry", []string{"base"}, "2026-07-20T00:00:00Z", "2026-07-30T00:00:00Z")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/search/commits"):
					if !strings.Contains(r.URL.Query().Get("q"), "author-date:2026-07-18T00:00:00Z..2026-07-25T00:00:00Z") {
						t.Errorf("search lost author-date window: %s", r.URL.RawQuery)
					}
					fmt.Fprintf(w, "{\"total_count\":1,\"items\":[%s]}", cherry)
				case strings.Contains(r.URL.Path, "/orgs/"):
					fmt.Fprint(w, "[{\"full_name\":\"a/b\"}]")
				case strings.HasSuffix(r.URL.Path, "/pulls"):
					fmt.Fprint(w, "[{\"number\":1}]")
				case strings.Contains(r.URL.Path, "/pulls/1/commits"):
					// Unlike the repository listing, this endpoint is not
					// time-filtered. Sting must select by author date here.
					fmt.Fprintf(w, "[%s,%s]", rebased, cherry)
				default:
					if r.URL.Query().Get("since") != "2026-07-18T00:00:00Z" || r.URL.Query().Get("until") != "2026-07-25T00:00:00Z" {
						t.Errorf("listing window changed: %s", r.URL.RawQuery)
					}
					fmt.Fprintf(w, "[%s]", rebased)
				}
			}))
			defer srv.Close()
			since, until := activityWindow()
			res, err := newTestClient(t, srv.URL, 100).Collect(context.Background(), model.Query{
				Author: "octocat", Scope: tc.scope, Repos: []string{"a/b"}, Org: "a",
				Since: since, Until: until, IncludePullRequests: tc.prs,
			})
			if err != nil || res.Count != tc.count || res.WindowDateBasis != tc.basis || res.SchemaVersion != model.SchemaVersion {
				t.Fatalf("incorrect query evidence: %+v err=%v", res, err)
			}
			for _, cm := range res.Commits {
				wantAuthor, wantCommitter, wantBasis := "2026-07-01T00:00:00Z", "2026-07-20T00:00:00Z", model.WindowDateBasisCommitter
				if cm.SHA == "cherry" {
					wantAuthor, wantCommitter, wantBasis = "2026-07-20T00:00:00Z", "2026-07-30T00:00:00Z", model.WindowDateBasisAuthor
				}
				if cm.Date.Format(time.RFC3339) != wantAuthor || cm.CommitterDate.Format(time.RFC3339) != wantCommitter || cm.WindowDateBasis != wantBasis {
					t.Fatalf("timestamps conflated or filtered incorrectly: %+v", cm)
				}
			}
		})
	}
}

func TestQueryDateBasisSurvivesEmptyAndFailedDiscovery(t *testing.T) {
	for _, status := range []int{200, 500} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			fmt.Fprint(w, "[]")
		}))
		res, err := newTestClient(t, srv.URL, 100).Collect(context.Background(), model.Query{
			Scope: model.ScopeRepos, Repos: []string{"a/b"}, Author: "octocat",
		})
		srv.Close()
		if (err != nil) != (status == 500) || res.WindowDateBasis != model.WindowDateBasisCommitter {
			t.Fatalf("missing query basis on empty/failure result: %+v %v", res, err)
		}
	}
}
