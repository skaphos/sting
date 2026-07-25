// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

// TestCollectReturnsPartialResultsWhenEnrichmentFails is a regression test for a
// pre-existing Constitution VI violation.
//
// Collect used to abort the whole query when enrichDetails failed
// (`return model.Result{}, err`), discarding every commit already gathered. The
// commits are real evidence whether or not their per-commit detail could be
// fetched, so they are now returned alongside the error with Truncated set.
func TestCollectReturnsPartialResultsWhenEnrichmentFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/commits/"):
			// Per-commit detail fails outright.
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"server error"}`))
		case strings.HasSuffix(r.URL.Path, "/commits"):
			_, _ = w.Write([]byte(`[
				{"sha":"aaa111","html_url":"https://example.com/c/aaa111","author":{"login":"octocat"},
				 "commit":{"message":"first","author":{"name":"Octo","email":"octo@example.com","date":"2026-07-20T10:00:00Z"}}},
				{"sha":"bbb222","html_url":"https://example.com/c/bbb222","author":{"login":"octocat"},
				 "commit":{"message":"second","author":{"name":"Octo","email":"octo@example.com","date":"2026-07-21T10:00:00Z"}}}
			]`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, 100)
	q := model.Query{
		Author:       "octocat",
		Scope:        model.ScopeRepos,
		Repos:        []string{"skaphos/sting"},
		Since:        time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC),
		Until:        time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC),
		IncludeStats: true, // forces the enrichment path
	}

	res, err := c.Collect(context.Background(), q)

	// The error is still reported — the failure is not swallowed.
	if err == nil {
		t.Fatal("expected the enrichment failure to be reported")
	}
	// But the commits already gathered must survive it.
	if res.Count != 2 || len(res.Commits) != 2 {
		t.Fatalf("Count = %d, len(Commits) = %d, want 2/2: gathered commits were discarded",
			res.Count, len(res.Commits))
	}
	if res.Commits[0].SHA != "aaa111" {
		t.Errorf("commit order not preserved: %+v", res.Commits)
	}
	// The result must be visibly incomplete rather than reading as whole.
	if !res.Truncated {
		t.Error("Truncated = false; a result missing its enrichment must say so")
	}
	// The result stays well-formed so a caller can serialize it as evidence.
	if res.SchemaVersion != model.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", res.SchemaVersion, model.SchemaVersion)
	}
	if res.Author != "octocat" || res.Scope != model.ScopeRepos {
		t.Errorf("resolved query not echoed: author=%q scope=%q", res.Author, res.Scope)
	}
}

// TestCollectSucceedsWithoutEnrichment confirms the change did not turn a clean
// run into a reported failure.
func TestCollectSucceedsWithoutEnrichment(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/commits") {
			_, _ = w.Write([]byte(`[{"sha":"aaa111","commit":{"author":{"name":"Octo","date":"2026-07-20T10:00:00Z"}}}]`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, 100)
	res, err := c.Collect(context.Background(), model.Query{
		Author: "octocat",
		Scope:  model.ScopeRepos,
		Repos:  []string{"skaphos/sting"},
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if res.Truncated {
		t.Error("Truncated = true on a complete run")
	}
	if res.Count != 1 {
		t.Errorf("Count = %d, want 1", res.Count)
	}
}
