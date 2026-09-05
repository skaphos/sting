// SPDX-License-Identifier: MIT
package ghclient_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/ghclient"
)

func ExampleClient_CollectPRs() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "read-only", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"total_count":0,"items":[]}`)
	}))
	defer server.Close()
	cfg := config.Default()
	q, err := cfg.ResolvePRs(config.PRRequest{Author: "octocat", TimeBasis: "created"}, time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC))
	if err != nil {
		panic(err)
	}
	client, err := ghclient.New("", server.URL+"/", 100)
	if err != nil {
		panic(err)
	}
	result, err := client.CollectPRs(context.Background(), q)
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Workflow, result.Count)
	// Output: pr-activity 0
}
func ExampleClient_CollectPRInbox() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "read-only", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()
	cfg := config.Default()
	q, err := cfg.ResolvePRInbox(config.PRInboxRequest{User: "octocat", Scope: "repos", Repos: []string{"acme/api"}})
	if err != nil {
		panic(err)
	}
	client, err := ghclient.New("", server.URL+"/", 100)
	if err != nil {
		panic(err)
	}
	result, err := client.CollectPRInbox(context.Background(), q)
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Workflow, result.Count)
	// Output: pr-inbox 0
}
