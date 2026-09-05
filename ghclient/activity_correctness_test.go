// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/go-github/v91/github"
	"github.com/skaphos/sting/model"
)

func TestActivityRetainsFailedStages(t *testing.T) {
	for _, stage := range []string{"listing", "comparison", "enrichment", "cancellation"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				p := r.URL.Path
				switch {
				case strings.HasSuffix(p, "/rate_limit"):
					fmt.Fprint(w, `{"resources":{"core":{"remaining":4999,"limit":5000}}}`)
				case strings.Contains(p, "/compare/"):
					if stage == "comparison" {
						http.Error(w, "outage", 500)
						return
					}
					fmt.Fprint(w, `{"status":"ahead","files":[{"filename":"a.go"}]}`)
				case strings.HasSuffix(p, "/commits/head"):
					if r.URL.Query().Get("page") == "2" {
						http.Error(w, "detail outage", 500)
						return
					}
					w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2>; rel="next"`, r.Host, p))
					fmt.Fprint(w, `{"files":[{"filename":"a.go"}]}`)
				default:
					if r.URL.Query().Get("page") == "2" {
						if stage == "cancellation" {
							cancel()
						}
						http.Error(w, "listing outage", 500)
						return
					}
					if stage == "listing" || stage == "cancellation" {
						w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2>; rel="next"`, r.Host, p))
					}
					fmt.Fprintf(w, "[%s]", windowCommit("head", []string{"base"}, "2026-07-20T00:00:00Z", "2026-07-20T00:00:00Z"))
				}
			}))
			defer srv.Close()
			c, err := New("fixture", srv.URL+"/", 100, WithRequestBudget(20))
			if err != nil {
				t.Fatal(err)
			}
			q := activityQuery("a/b")
			q.Ref, q.EnrichCommits = "main", 1
			res, err := c.CollectActivity(ctx, q)
			if err == nil || res.Count != 1 || res.Cost.Consumed < 2 || !hasDisclosure(res.Disclosures, model.DisclosureCollectionFailed) {
				t.Fatalf("lost partial evidence: result=%+v err=%v", res, err)
			}
			if stage == "cancellation" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
			if res.CommitsCollected != (stage == "comparison" || stage == "enrichment") {
				t.Fatalf("incorrect listing completion: %+v", res)
			}
			if stage == "enrichment" {
				if !res.ChangeSetCollected || len(res.ChangeSet.Paths) != 1 || len(res.Commits[0].Files) != 1 || res.Commits[0].Enriched {
					t.Fatalf("lost comparison or partial file page: %+v", res)
				}
			}
		})
	}
}

func TestActivityAuthorDoesNotNarrowBoundaries(t *testing.T) {
	for _, author := range []string{"middle", "first", "last", "absent"} {
		t.Run(author, func(t *testing.T) {
			var comparison string
			listed := 0
			commit := func(sha, parent string) string {
				return windowCommit(sha, []string{parent}, "2026-07-20T00:00:00Z", "2026-07-20T00:00:00Z")
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/rate_limit"):
					fmt.Fprint(w, `{"resources":{"core":{"remaining":4999,"limit":5000}}}`)
				case strings.Contains(r.URL.Path, "/compare/"):
					comparison = r.URL.Path
					fmt.Fprint(w, `{"status":"ahead","files":[{"filename":"all-authors.go"}]}`)
				default:
					listed++
					filter := r.URL.Query().Get("author")
					switch filter {
					case "":
						fmt.Fprintf(w, "[%s,%s,%s]", commit("last", "middle"), commit("middle", "first"), commit("first", "before"))
					case "absent":
						fmt.Fprint(w, "[]")
					default:
						fmt.Fprintf(w, "[%s]", commit(filter, "filtered-parent"))
					}
				}
			}))
			defer srv.Close()
			c, err := New("fixture", srv.URL+"/", 100, WithRequestBudget(20))
			if err != nil {
				t.Fatal(err)
			}
			q := activityQuery("a/b")
			q.Ref, q.Author = "main", author
			res, err := c.CollectActivity(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(comparison, "/before...last") || listed != 2 || res.Cost.Consumed != 3 {
				t.Fatalf("boundaries or cost wrong: comparison=%s listed=%d cost=%+v", comparison, listed, res.Cost)
			}
			want := 1
			if author == "absent" {
				want = 0
			}
			if res.Count != want || !res.CommitsCollected || !res.ChangeSetCollected || !hasDisclosure(res.Disclosures, model.DisclosureAuthorFilterNotApplied) {
				t.Fatalf("incorrect author view: %+v", res)
			}
			if want == 1 && res.Commits[0].SHA != author {
				t.Fatalf("unfiltered commit leaked: %+v", res.Commits)
			}
		})
	}
}

func TestActivityPaginatedEnrichmentCannotStarveComparison(t *testing.T) {
	for _, concurrency := range []int{1, 8} {
		t.Run(fmt.Sprint(concurrency), func(t *testing.T) {
			var calls []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				p := r.URL.Path
				switch {
				case strings.HasSuffix(p, "/rate_limit"):
					fmt.Fprint(w, `{"resources":{"core":{"remaining":4999,"limit":5000}}}`)
				case strings.Contains(p, "/compare/"):
					calls = append(calls, "compare")
					fmt.Fprint(w, `{"status":"ahead","files":[{"filename":"a.go"}]}`)
				case strings.HasSuffix(p, "/commits/head"):
					calls = append(calls, "head:"+r.URL.Query().Get("page"))
					w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2>; rel="next"`, r.Host, p))
					fmt.Fprint(w, `{"files":[{"filename":"a.go"}]}`)
				case strings.HasSuffix(p, "/commits"):
					calls = append(calls, "list")
					fmt.Fprintf(w, "[%s,%s]", windowCommit("head", []string{"older"}, "2026-07-21T00:00:00Z", "2026-07-21T00:00:00Z"), windowCommit("older", []string{"base"}, "2026-07-20T00:00:00Z", "2026-07-20T00:00:00Z"))
				default:
					t.Errorf("unexpected request %s", p)
				}
			}))
			defer srv.Close()
			c, err := New("fixture", srv.URL+"/", 1, WithRequestBudget(3))
			if err != nil {
				t.Fatal(err)
			}
			c.concurrency = concurrency
			q := activityQuery("a/b")
			q.Ref, q.EnrichCommits, q.MaxRequests = "main", 2, 3
			res, err := c.CollectActivity(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, []string{"list", "compare", "head:"}) || res.Cost.Consumed != 3 {
				t.Fatalf("unexpected budget allocation: %v cost=%+v", calls, res.Cost)
			}
			if !res.ChangeSetCollected || len(res.Commits[0].Files) != 1 || res.Commits[0].Enriched || res.Commits[1].Enriched || !hasDisclosure(res.Disclosures, model.DisclosureEnrichmentPartial) {
				t.Fatalf("incorrect partial evidence: %+v", res)
			}
		})
	}
}

func TestActivityPatchBudgetIgnoresProviderOrdering(t *testing.T) {
	a := &github.CommitFile{Filename: new("a"), Patch: new("éé"), Additions: new(2)}
	b := &github.CommitFile{Filename: new("b"), Patch: new("bbb"), Deletions: new(3)}
	for _, budget := range []int{1, 3, 4, 5} {
		q := model.ActivityQuery{IncludeDiffs: true, MaxDiffBytes: budget}
		input := []*github.CommitFile{b, a}
		x, y := changeSetFromFiles([]*github.CommitFile{a, b}, q), changeSetFromFiles(input, q)
		if !reflect.DeepEqual(x, y) || input[0] != b || x.TotalAdditions != 2 || x.TotalDeletions != 3 {
			t.Fatalf("budget=%d: nondeterministic evidence or input mutation: %+v vs %+v", budget, x, y)
		}
		for _, p := range x.Paths {
			if !utf8.ValidString(p.Patch) {
				t.Fatalf("invalid UTF-8: %q", p.Patch)
			}
		}
	}
}

func TestActivityEstimateAccountsForBothAuthorListings(t *testing.T) {
	for _, ceiling := range []int{1, 10} {
		t.Run(fmt.Sprint(ceiling), func(t *testing.T) {
			var authors []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/rate_limit") {
					fmt.Fprint(w, `{"resources":{"core":{"remaining":4999,"limit":5000}}}`)
					return
				}
				author := r.URL.Query().Get("author")
				authors = append(authors, author)
				count := 250
				if author != "" {
					count = 2
				}
				w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=%d>; rel="last"`, r.Host, r.URL.Path, count))
				fmt.Fprint(w, `[{"sha":"probe"}]`)
			}))
			defer srv.Close()
			c, err := New("fixture", srv.URL+"/", 100, WithRequestBudget(ceiling))
			if err != nil {
				t.Fatal(err)
			}
			q := activityQuery("a/b")
			q.Ref, q.Author, q.EnrichCommits, q.EstimateOnly, q.MaxRequests = "main", "alice", 5, true, ceiling
			res, err := c.CollectActivity(context.Background(), q)
			if err != nil || !res.EstimateOnly || res.CommitsCollected || res.ChangeSetCollected {
				t.Fatalf("incorrect estimate: %+v %v", res, err)
			}
			if ceiling == 1 {
				if res.Cost.Consumed != 1 || !hasDisclosure(res.Disclosures, model.DisclosureBudgetBounded) {
					t.Fatalf("lost probe cost/stop: %+v", res)
				}
			} else if res.Cost.Estimated != 9 || res.Cost.Consumed != 2 || !reflect.DeepEqual(authors, []string{"", "alice"}) {
				t.Fatalf("want 2 probes + 3 window pages + 1 author page + compare + 2 details: %+v authors=%v", res.Cost, authors)
			}
		})
	}
}

func TestActivityRetainsCompletedEnrichmentOnLaterFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/rate_limit"):
			fmt.Fprint(w, `{"resources":{"core":{"remaining":4999,"limit":5000}}}`)
		case strings.Contains(r.URL.Path, "/compare/"):
			fmt.Fprint(w, `{"status":"ahead","files":[{"filename":"a.go"}]}`)
		case strings.HasSuffix(r.URL.Path, "/commits/head"):
			fmt.Fprint(w, `{"files":[{"filename":"a.go"}]}`)
		case strings.HasSuffix(r.URL.Path, "/commits/older"):
			http.Error(w, "later detail outage", 500)
		default:
			fmt.Fprintf(w, "[%s,%s]", windowCommit("head", []string{"older"}, "2026-07-21T00:00:00Z", "2026-07-21T00:00:00Z"), windowCommit("older", []string{"base"}, "2026-07-20T00:00:00Z", "2026-07-20T00:00:00Z"))
		}
	}))
	defer srv.Close()
	q := activityQuery("a/b")
	q.Ref, q.EnrichCommits = "main", 2
	res, err := newTestClient(t, srv.URL, 100).CollectActivity(context.Background(), q)
	if err == nil || len(res.Commits) != 2 || !res.Commits[0].Enriched || res.Commits[1].Enriched || !res.ChangeSetCollected {
		t.Fatalf("completed enrichment lost: %+v %v", res, err)
	}
	if len(res.Correlations) != 1 || res.Correlations[0].Basis != model.BasisObserved || !reflect.DeepEqual(res.Correlations[0].SHAs, []string{"head"}) {
		t.Fatalf("completed observed attribution lost: %+v", res.Correlations)
	}
}
