// SPDX-License-Identifier: MIT
package ghclient_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/ghclient"
)

func TestPRPublicClientIndependentBudgets(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("lost read-only dedicated auth")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", fmt.Sprintf("<http://%s/api/v3/repos/acme/api/pulls?page=2>; rel=\"next\"", r.Host))
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()
	client, err := ghclient.New("test-token", server.URL+"/", 100)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	one := 1
	activity, err := cfg.ResolvePRs(config.PRRequest{Scope: "repos", Repos: []string{"acme/api"}, TimeBasis: "created", MaxRequests: &one}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	inbox, err := cfg.ResolvePRInbox(config.PRInboxRequest{Scope: "repos", Repos: []string{"acme/api"}, User: "octocat", MaxRequests: &one})
	if err != nil {
		t.Fatal(err)
	}
	run := func() {
		a, e := client.CollectPRs(context.Background(), activity)
		if e != nil || a.Cost.Consumed != 1 || !a.Truncated {
			t.Errorf("activity budget: %+v %v", a, e)
		}
		b, e := client.CollectPRInbox(context.Background(), inbox)
		if e != nil || b.Cost.Consumed != 1 || !b.Truncated {
			t.Errorf("inbox budget: %+v %v", b, e)
		}
	}
	run()
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(run)
	}
	wg.Wait()
	if calls.Load() != 10 || client.Cost().Consumed != 0 {
		t.Fatalf("shared or missing budget: %d %+v", calls.Load(), client.Cost())
	}
}
