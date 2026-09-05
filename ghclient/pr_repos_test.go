// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"net/http"
	"testing"

	"github.com/skaphos/sting/config"
)

func TestPRRepositoryActivity(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "all" {
			t.Error("current state constrained discovery")
		}
		p := prPayload(1)
		if r.URL.Path == "/repos/acme/web/pulls" {
			p["user"] = map[string]any{"login": "another"}
			p["draft"] = true
		}
		prJSON(t, w, []any{p})
	})
	c, err := New("", f.server.URL+"/", 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, author := range []string{"", "octocat"} {
		q, e := config.Default().ResolvePRs(config.PRRequest{Scope: "repos", Repos: []string{"acme/web", "acme/api"}, TimeBasis: "created", Author: author}, prClock)
		if e != nil {
			t.Fatal(e)
		}
		got, e := c.CollectPRs(context.Background(), q)
		if e != nil {
			t.Fatal(e)
		}
		want := 2
		if author != "" {
			want = 1
		}
		if got.Count != want || got.Truncated || got.Cost.Consumed != 2 || got.PRs[0].PR.Repo != "acme/api" {
			t.Fatalf("repo result: %+v", got)
		}
	}
}
