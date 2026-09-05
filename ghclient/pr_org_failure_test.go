// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"net/http"
	"testing"

	"github.com/skaphos/sting/config"
)

func TestPROrgGlobalFailureStops(t *testing.T) {
	for _, status := range []int{401, 403, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/orgs/acme/repos" {
					prJSON(t, w, []any{map[string]any{"full_name": "acme/a"}, map[string]any{"full_name": "acme/b"}})
					return
				}
				if status == 403 {
					w.Header().Set("Retry-After", "10")
				}
				w.WriteHeader(status)
				prJSON(t, w, map[string]any{"message": "fixture failure"})
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
			if e == nil || got.Count != 0 || got.Cost.Consumed != 2 {
				t.Fatalf("global error skipped: %+v %v", got, e)
			}
		})
	}
}
