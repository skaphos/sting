// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/skaphos/sting/internal/apibudget"
)

func TestPRSessionBudget(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-pat" {
			t.Error("lost auth")
		}
		prJSON(t, w, map[string]any{"login": "octocat"})
	})
	c, err := New("test-pat", f.server.URL+"/", 2, WithRequestBudget(1))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		s, err := c.newPRSession(1)
		if err != nil {
			t.Fatal(err)
		}
		call := func() error { _, _, e := s.client.gh.Users.Get(context.Background(), ""); return e }
		if err = s.do("identity", call); err != nil {
			t.Fatal(err)
		}
		if err = s.do("identity", call); !errors.Is(err, apibudget.ErrBudgetExceeded) {
			t.Fatalf("budget error: %v", err)
		}
		if got := s.cost(); got.Consumed != 1 || got.IdentityRequests != 1 || got.Ceiling != 1 {
			t.Fatalf("cost: %+v", got)
		}
	}
	if f.count() != 2 || c.Cost().Consumed != 0 {
		t.Fatal("sessions share budget")
	}
}

func TestPRSessionCanceledAndRedirectCosts(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery == "" {
			http.Redirect(w, r, "/api/v3/user?redirected=1", http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4998")
		prJSON(t, w, map[string]any{"login": "octocat"})
	})
	c, err := New("", f.server.URL+"/", 100)
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.newPRSession(2)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.do("identity", func() error { _, _, e := s.client.gh.Users.Get(context.Background(), ""); return e }); err != nil {
		t.Fatal(err)
	}
	cost := s.cost()
	if cost.Consumed != 2 || cost.IdentityRequests != 2 || cost.Quota == nil || cost.Quota.Remaining != 4998 {
		t.Fatalf("redirect cost: %+v", cost)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fresh, err := c.newPRSession(1)
	if err != nil {
		t.Fatal(err)
	}
	err = fresh.do("discovery", func() error { _, _, e := fresh.client.gh.Users.Get(ctx, ""); return e })
	if !errors.Is(err, context.Canceled) || f.count() != 2 {
		t.Fatalf("cancellation dispatched: %v, %d", err, f.count())
	}
	if fresh.client.gh.Client().Timeout != httpTimeout {
		t.Fatal("lost request timeout")
	}
}
