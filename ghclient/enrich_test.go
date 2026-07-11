// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

// searchBodyN builds a commit-search response with n items, SHAs sha0..sha{n-1}
// in order, all in one repo.
func searchBodyN(n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, `{"total_count":%d,"incomplete_results":false,"items":[`, n)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"sha":"sha%d","html_url":"https://example.com/c/sha%d",`+
			`"author":{"login":"octocat"},"repository":{"full_name":"skaphos/sting"},`+
			`"commit":{"message":"m%d","author":{"name":"Octo Cat","email":"octo@example.com","date":"2026-05-20T10:00:00Z"}}}`,
			i, i, i)
	}
	b.WriteString(`]}`)
	return b.String()
}

// TestEnrichDetailsConcurrent proves per-commit detail enrichment runs in
// parallel (the fix for slow "all my commits" queries) while preserving order.
func TestEnrichDetailsConcurrent(t *testing.T) {
	const n = 6
	var inflight, maxInflight int32
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch p := r.URL.Path; {
		case strings.Contains(p, "search/commits"):
			_, _ = w.Write([]byte(searchBodyN(n)))
		case strings.Contains(p, "/commits/"): // GetCommit detail
			cur := atomic.AddInt32(&inflight, 1)
			for {
				old := atomic.LoadInt32(&maxInflight)
				if cur <= old || atomic.CompareAndSwapInt32(&maxInflight, old, cur) {
					break
				}
			}
			time.Sleep(40 * time.Millisecond) // widen the overlap window
			atomic.AddInt32(&inflight, -1)
			fmt.Fprintf(w, `{"sha":%q,"stats":{"additions":1,"deletions":0,"total":1}}`, path.Base(p))
		default:
			t.Errorf("unexpected path %q", p)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, 50)
	res, err := c.Collect(context.Background(), model.Query{
		Author:       "octocat",
		Scope:        model.ScopeSearch,
		IncludeStats: true,
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if res.Count != n {
		t.Fatalf("Count = %d, want %d", res.Count, n)
	}
	if maxInflight < 2 {
		t.Errorf("max concurrent detail requests = %d, want >= 2 (enrichment should be parallel)", maxInflight)
	}
	for i, cm := range res.Commits {
		if want := fmt.Sprintf("sha%d", i); cm.SHA != want {
			t.Errorf("Commits[%d].SHA = %q, want %q (order must be preserved)", i, cm.SHA, want)
		}
		if cm.Additions != 1 {
			t.Errorf("Commits[%d].Additions = %d, want 1 (stats must be filled)", i, cm.Additions)
		}
	}
}

// TestEnrichDetailsAbortsOnError verifies a per-commit detail failure aborts the
// whole scan rather than being silently dropped.
func TestEnrichDetailsAbortsOnError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch p := r.URL.Path; {
		case strings.Contains(p, "search/commits"):
			_, _ = w.Write([]byte(searchBodyN(3)))
		case strings.Contains(p, "/commits/"):
			http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
		default:
			t.Errorf("unexpected path %q", p)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, 50)
	_, err := c.Collect(context.Background(), model.Query{
		Author:       "octocat",
		Scope:        model.ScopeSearch,
		IncludeStats: true,
	})
	if err == nil {
		t.Fatal("expected error when detail enrichment fails, got nil")
	}
}
