// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/skaphos/sting/config"
)

func TestPRInboxSearchReconciliation(t *testing.T) {
	for _, mode := range []string{"removed", "absent", "interrupted"} {
		t.Run(mode, func(t *testing.T) {
			f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
				p := prPayload(1)
				p["user"] = map[string]any{"login": "other"}
				p["assignees"] = []any{map[string]any{"login": "octocat"}}
				if r.URL.Path == "/search/issues" {
					p["repository_url"] = "https://api.github.com/repos/acme/api"
					p["pull_request"] = map[string]any{}
					prJSON(t, w, map[string]any{"total_count": 1, "items": []any{p}})
					return
				}
				if mode == "interrupted" {
					w.Header().Set("Link", fmt.Sprintf("<http://%s/api/v3/repos/acme/api/pulls?page=2>; rel=\"next\"", r.Host))
					prJSON(t, w, []any{})
					return
				}
				if mode == "absent" {
					prJSON(t, w, []any{})
					return
				}
				p["assignees"] = []any{}
				another := prPayload(2)
				prJSON(t, w, []any{p, another})
			})
			c, e := New("", f.server.URL+"/", 100)
			if e != nil {
				t.Fatal(e)
			}
			limit := 10
			if mode == "interrupted" {
				limit = 2
			}
			q, e := config.Default().ResolvePRInbox(config.PRInboxRequest{User: "octocat", MaxRequests: &limit})
			if e != nil {
				t.Fatal(e)
			}
			got, e := c.CollectPRInbox(context.Background(), q)
			if e != nil {
				t.Fatal(e)
			}
			want := 1
			if mode == "absent" {
				want = 0
			}
			if got.Count != want || !got.Truncated {
				t.Fatalf("reconciliation %s: %+v", mode, got)
			}
			if mode == "removed" && got.PRs[0].PR.Number != 2 {
				t.Fatal("stale search restored invalid assignment")
			}
			if mode == "interrupted" && (got.PRs[0].ReasonsComplete || got.Cost.Consumed != 2) {
				t.Fatal("incomplete absence treated as proof")
			}
			if mode != "interrupted" && got.Cost.Consumed != 4 {
				t.Fatalf("verification repeated: %+v", got.Cost)
			}
		})
	}
}

func TestPRInboxVerificationPreservesPublicRestriction(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		p := prPayload(1)
		if r.URL.Path == "/search/issues" {
			p["repository_url"] = "https://api.github.com/repos/acme/api"
			p["pull_request"] = map[string]any{}
			prJSON(t, w, map[string]any{"total_count": 1, "items": []any{p}})
			return
		}
		p["base"] = map[string]any{"ref": "main", "repo": map[string]any{"full_name": "acme/api", "private": true}}
		prJSON(t, w, []any{p})
	})
	c, e := New("", f.server.URL+"/", 100)
	if e != nil {
		t.Fatal(e)
	}
	q, e := config.Default().ResolvePRInbox(config.PRInboxRequest{User: "octocat"})
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.CollectPRInbox(context.Background(), q)
	if e != nil {
		t.Fatal(e)
	}
	if got.Count != 0 || !got.Truncated {
		t.Fatalf("private listing escaped public-only selection: %+v", got)
	}
}

func TestPRInboxUnknownAssignmentRetainsProof(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		p := prPayload(1)
		p["user"] = map[string]any{"login": "other"}
		if r.URL.Path == "/search/issues" {
			p["assignees"] = []any{map[string]any{"login": "octocat"}}
			p["repository_url"] = "https://api.github.com/repos/acme/api"
			p["pull_request"] = map[string]any{}
			prJSON(t, w, map[string]any{"total_count": 1, "items": []any{p}})
			return
		}
		delete(p, "assignees")
		prJSON(t, w, []any{p})
	})
	c, e := New("", f.server.URL+"/", 100)
	if e != nil {
		t.Fatal(e)
	}
	q, e := config.Default().ResolvePRInbox(config.PRInboxRequest{User: "octocat"})
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.CollectPRInbox(context.Background(), q)
	if e != nil {
		t.Fatal(e)
	}
	if got.Count != 1 || !got.Truncated || got.PRs[0].ReasonsComplete {
		t.Fatalf("unknown assignment became known absent/complete: %+v", got)
	}
}
