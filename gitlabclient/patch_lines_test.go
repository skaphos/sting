// SPDX-License-Identifier: MIT
package gitlabclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skaphos/sting/model"
)

func TestPatchContentPrefixes(t *testing.T) {
	for _, diff := range []string{
		"@@ -1 +1 @@\n---old\n+++new\n",
		"--- a/readme\n+++ b/readme\n@@ -1 +1 @@\n----\n++++\n",
		"@@ -1 +1 @@\n--- old\n+++ new\n\\ No newline at end of file\n",
		"diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1 +1 @@\n---old\n+++new\ndiff --git a/b b/b\n--- a/b\n+++ b/b\n",
	} {
		a, d := countPatchLines(diff)
		if a != 1 || d != 1 {
			t.Errorf("patch %q: got +%d/-%d, want +1/-1", diff, a, d)
		}
	}
}

func TestPatchContentPrefixesContributeToCommitTotals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/diff") {
			fmt.Fprint(w, `[{"new_path":"README.md","diff":"@@ -1 +1 @@\n---old\n+++new\n"}]`)
			return
		}
		fmt.Fprint(w, `[{"id":"abc123","author_name":"octocat"}]`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, 50)
	res, err := c.Collect(context.Background(), model.Query{Author: "octocat", Scope: model.ScopeRepos,
		Repos: []string{"a/b"}, IncludeFiles: true})
	if err != nil || len(res.Commits) != 1 {
		t.Fatalf("Collect: %+v %v", res, err)
	}
	cm := res.Commits[0]
	if cm.Additions != 1 || cm.Deletions != 1 || cm.Changes != 2 || len(cm.Files) != 1 || cm.Files[0].Changes != 2 {
		t.Fatalf("incorrect derived totals: %+v", cm)
	}
}
