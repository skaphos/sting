// SPDX-License-Identifier: MIT
package gitlabclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

// gitlabCommitsBodyN builds a commit-list response with n items, SHAs
// sha0..sha{n-1} in order, stats inline.
func gitlabCommitsBodyN(n int) string {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":"sha%d","short_id":"sha%d","title":"m%d","message":"m%d",`+
			`"author_name":"Octo Cat","author_email":"octo@example.com",`+
			`"authored_date":"2026-05-21T11:00:00Z","web_url":"https://gitlab.example.com/c/sha%d",`+
			`"stats":{"additions":1,"deletions":0,"total":1}}`, i, i, i, i, i)
	}
	b.WriteByte(']')
	return b.String()
}

func isDiffPath(p string) bool {
	return strings.Contains(p, "/repository/commits/") && strings.HasSuffix(p, "/diff")
}

// TestEnrichDiffsConcurrent proves per-commit diff enrichment runs in parallel
// while preserving order.
func TestEnrichDiffsConcurrent(t *testing.T) {
	const n = 6
	var inflight, maxInflight int32
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch p := r.URL.EscapedPath(); {
		case isDiffPath(p):
			cur := atomic.AddInt32(&inflight, 1)
			for {
				old := atomic.LoadInt32(&maxInflight)
				if cur <= old || atomic.CompareAndSwapInt32(&maxInflight, old, cur) {
					break
				}
			}
			time.Sleep(40 * time.Millisecond)
			atomic.AddInt32(&inflight, -1)
			_, _ = w.Write([]byte(gitlabDiffBody))
		case strings.Contains(p, "/repository/commits"):
			_, _ = w.Write([]byte(gitlabCommitsBodyN(n)))
		default:
			t.Errorf("unexpected path %q", p)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, 50)
	res, err := c.Collect(context.Background(), model.Query{
		Author:       "octocat",
		Scope:        model.ScopeRepos,
		Repos:        []string{"skaphos/sting"},
		IncludeDiffs: true,
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if res.Count != n {
		t.Fatalf("Count = %d, want %d", res.Count, n)
	}
	if maxInflight < 2 {
		t.Errorf("max concurrent diff requests = %d, want >= 2 (enrichment should be parallel)", maxInflight)
	}
	for i, cm := range res.Commits {
		if want := fmt.Sprintf("sha%d", i); cm.SHA != want {
			t.Errorf("Commits[%d].SHA = %q, want %q (order must be preserved)", i, cm.SHA, want)
		}
		if len(cm.Files) == 0 {
			t.Errorf("Commits[%d] has no files; diff enrichment missing", i)
		}
	}
}

// TestCollectGroupScopeDiffs covers group-scope diff enrichment: the commit list
// is fetched by numeric project id, but the diff is addressed by the commit's
// URL-encoded project path (cm.Repo), which GitLab accepts identically.
func TestCollectGroupScopeDiffs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch p := r.URL.EscapedPath(); {
		case strings.Contains(p, "/groups/skaphos/projects"):
			_, _ = w.Write([]byte(`[{"id":42,"path_with_namespace":"skaphos/sting"}]`))
		case isDiffPath(p):
			if !strings.Contains(p, "/projects/skaphos%2Fsting/repository/commits/abc123/diff") {
				t.Errorf("group-scope diff path = %q, want project addressed by encoded path", p)
			}
			_, _ = w.Write([]byte(gitlabDiffBody))
		case strings.Contains(p, "/projects/42/repository/commits"):
			_, _ = w.Write([]byte(gitlabCommitsBody))
		default:
			t.Errorf("unexpected path %q", p)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, 50)
	res, err := c.Collect(context.Background(), model.Query{
		Author:       "octocat",
		Scope:        model.ScopeOrg,
		Org:          "skaphos",
		IncludeDiffs: true,
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if res.Count != 1 {
		t.Fatalf("Count = %d, want 1", res.Count)
	}
	if len(res.Commits[0].Files) == 0 {
		t.Fatal("group-scope diff enrichment produced no files")
	}
}

// TestEnrichDiffsAbortsOnError verifies a per-commit diff failure aborts the scan.
func TestEnrichDiffsAbortsOnError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch p := r.URL.EscapedPath(); {
		case isDiffPath(p):
			http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
		case strings.Contains(p, "/repository/commits"):
			_, _ = w.Write([]byte(gitlabCommitsBodyN(3)))
		default:
			t.Errorf("unexpected path %q", p)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL, 50)
	_, err := c.Collect(context.Background(), model.Query{
		Author:       "octocat",
		Scope:        model.ScopeRepos,
		Repos:        []string{"skaphos/sting"},
		IncludeDiffs: true,
	})
	if err == nil {
		t.Fatal("expected error when diff enrichment fails, got nil")
	}
}
