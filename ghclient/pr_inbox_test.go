// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"net/http"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

func TestPRInboxIdentityAndUnion(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			prJSON(t, w, map[string]any{"login": "octocat"})
			return
		}
		if r.URL.Query().Get("state") != "open" {
			t.Error("inbox is not open-only")
		}
		p := prPayload(1)
		p["created_at"] = "2015-01-01T00:00:00Z"
		p["assignees"] = []any{map[string]any{"login": "octocat"}}
		p["requested_reviewers"] = []any{map[string]any{"login": "octocat"}}
		closed := prPayload(2)
		closed["state"] = "closed"
		team := prPayload(3)
		team["user"] = map[string]any{"login": "other"}
		team["requested_teams"] = []any{map[string]any{"slug": "reviewers"}}
		prJSON(t, w, []any{p, closed, team})
	})
	c, e := New("", f.server.URL+"/", 100)
	if e != nil {
		t.Fatal(e)
	}
	cfg := config.Default()
	cfg.DefaultWindow = "invalid"
	q, e := cfg.ResolvePRInbox(config.PRInboxRequest{Scope: "repos", Repos: []string{"acme/api"}})
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.CollectPRInbox(context.Background(), q)
	if e != nil {
		t.Fatal(e)
	}
	if got.Count != 1 || len(got.PRs[0].Reasons) != 3 || got.Query.User != "octocat" || got.Cost.IdentityRequests != 1 || got.Cost.Consumed != 2 || got.Cost.HistoryRequests != 0 || got.Truncated {
		t.Fatalf("inbox union: %+v", got)
	}
	if got.PRs[0].Reasons[0] != model.PRAuthored || got.PRs[0].Reasons[2] != model.PRReviewRequested {
		t.Fatal("reason ordering")
	}
}
func TestPRInboxIdentityFailure(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		prJSON(t, w, map[string]any{"message": "unauthenticated"})
	})
	c, e := New("", f.server.URL+"/", 100)
	if e != nil {
		t.Fatal(e)
	}
	q, e := config.Default().ResolvePRInbox(config.PRInboxRequest{})
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.CollectPRInbox(context.Background(), q)
	if e == nil || got.SchemaVersion == "" || got.Count != 0 || got.Query.User != "" || got.Cost.IdentityRequests != 1 || !got.Truncated {
		t.Fatalf("identity failure: %+v %v", got, e)
	}
}
