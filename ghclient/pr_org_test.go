// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/skaphos/sting/config"
)

func TestPROrgContinuesPastUnreadableRepo(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/orgs/acme/repos" {
			if r.URL.Query().Get("sort") != "full_name" {
				t.Error("unstable org traversal")
			}
			prJSON(t, w, []any{map[string]any{"full_name": "acme/b"}, map[string]any{"full_name": "acme/a"}, map[string]any{"full_name": "acme/c", "private": true}})
			return
		}
		if strings.Contains(r.URL.Path, "/acme/b/") {
			w.WriteHeader(404)
			prJSON(t, w, map[string]any{"message": "hidden"})
			return
		}
		prJSON(t, w, []any{prPayload(1)})
	})
	c, e := New("", f.server.URL+"/", 100)
	if e != nil {
		t.Fatal(e)
	}
	q, e := config.Default().ResolvePRs(config.PRRequest{Scope: "org", Org: "acme", TimeBasis: "created"}, prClock)
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.CollectPRs(context.Background(), q)
	if e != nil {
		t.Fatal(e)
	}
	if got.Count != 2 || !got.Truncated || got.Cost.Consumed != 4 || got.PRs[1].PR.Repo != "acme/c" {
		t.Fatalf("org evidence: %+v", got)
	}
	found := false
	for _, d := range got.Disclosures {
		if d.Kind == "repo-skipped" && d.Repo == "acme/b" {
			found = true
		}
	}
	if !found {
		t.Fatal("skip not attributable")
	}
}
