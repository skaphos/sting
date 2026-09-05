// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-github/v91/github"
	"github.com/skaphos/sting/config"
)

func TestPRRepositoryFailurePreservesEvidence(t *testing.T) {
	for _, status := range []int{403, 404, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/repos/acme/b/pulls" {
					w.WriteHeader(status)
					prJSON(t, w, map[string]any{"message": "fixture failure"})
					return
				}
				prJSON(t, w, []any{prPayload(1)})
			})
			c, e := New("", f.server.URL+"/", 100)
			if e != nil {
				t.Fatal(e)
			}
			q, e := config.Default().ResolvePRs(config.PRRequest{Scope: "repos", Repos: []string{"acme/a", "acme/b"}, TimeBasis: "created"}, prClock)
			if e != nil {
				t.Fatal(e)
			}
			got, e := c.CollectPRs(context.Background(), q)
			if e == nil || got.Count != 1 || !got.Truncated || got.Cost.Consumed != 2 {
				t.Fatalf("partial failure: %+v %v", got, e)
			}
			var er *github.ErrorResponse
			var ab *github.AbuseRateLimitError
			if !errors.As(e, &er) && !errors.As(e, &ab) {
				t.Errorf("lost provider error type: %T", e)
			}
		})
	}
}
